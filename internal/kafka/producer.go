package kafka

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Producer wraps franz-go with fully async produce.
//
// How backpressure works:
//   sem is a buffered channel of size maxInFlight.
//   Enqueue() tries a non-blocking send to sem before calling Produce().
//   The callback fires when Kafka acks (or errors) and releases the slot.
//   If sem is full, Enqueue returns false → handler returns HTTP 503.
//
// This means HTTP handler latency = semaphore try (~ns) + client.Produce() (~ns).
// It is completely decoupled from Kafka round-trip time (~2–5 ms).
type Producer struct {
	client   *kgo.Client
	topic    string
	dlqTopic string
	sem      chan struct{}
	wg       sync.WaitGroup
}

func New(brokers []string, topic, dlqTopic string, maxInFlight int) (*Producer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
		kgo.ProducerBatchMaxBytes(1<<20),
		// At 10k req/s, a 1ms linger window batches ~10 records per Kafka request.
		// This cuts broker round-trips by 10x with zero impact on HTTP latency
		// (produce is async — the HTTP response is already sent before this fires).
		kgo.ProducerLinger(time.Millisecond),
		kgo.RecordRetries(3),
	)
	if err != nil {
		return nil, err
	}
	p := &Producer{
		client:   client,
		topic:    topic,
		dlqTopic: dlqTopic,
		sem:      make(chan struct{}, maxInFlight),
	}

	// Register a gauge that reports live buffer utilisation.
	// Ignore "already registered" errors so tests calling New() multiple times don't panic.
	_ = prometheus.Register(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "ingestion_producer_buffer_in_flight",
			Help: "Number of Kafka records currently in the producer buffer.",
		},
		func() float64 { return float64(len(p.sem)) },
	))

	return p, nil
}

// Enqueue is non-blocking. Returns false when maxInFlight limit is hit (backpressure signal).
func (p *Producer) Enqueue(key, value []byte) bool {
	// Try to acquire a slot — non-blocking.
	select {
	case p.sem <- struct{}{}:
	default:
		metrics.EventsDropped.Inc()
		return false
	}

	p.wg.Add(1)
	rec := &kgo.Record{
		Topic: p.topic,
		Key:   key,
		Value: value,
	}
	// Produce is async; the callback fires on ack or error.
	p.client.Produce(context.Background(), rec, func(r *kgo.Record, err error) {
		<-p.sem     // release the slot
		p.wg.Done() // signal drain
		if err != nil {
			slog.Error("kafka produce error", "error", err, "topic", r.Topic)
			metrics.EventsFailed.Inc()
			p.sendToDLQ(r)
			return
		}
		metrics.EventsProduced.Inc()
	})
	return true
}

// sendToDLQ forwards a failed record to the dead-letter topic.
// It is fire-and-forget: DLQ errors are logged but never retried to avoid cascading loops.
func (p *Producer) sendToDLQ(r *kgo.Record) {
	dlq := &kgo.Record{
		Topic: p.dlqTopic,
		Key:   r.Key,
		Value: r.Value,
	}
	p.wg.Add(1)
	p.client.Produce(context.Background(), dlq, func(_ *kgo.Record, err error) {
		p.wg.Done()
		if err != nil {
			slog.Error("kafka dlq produce error", "error", err, "topic", p.dlqTopic)
			return
		}
		metrics.EventsDLQ.Inc()
	})
}

// Ping checks whether the Kafka cluster is reachable. Used by the /ready endpoint.
func (p *Producer) Ping(ctx context.Context) error {
	return p.client.Ping(ctx)
}

// Close waits for all in-flight callbacks to complete, flushes, then closes.
// Must be called only after the HTTP server is fully shut down.
func (p *Producer) Close() {
	p.wg.Wait()
	if err := p.client.Flush(context.Background()); err != nil {
		slog.Error("kafka flush error on shutdown", "error", err)
	}
	p.client.Close()
}
