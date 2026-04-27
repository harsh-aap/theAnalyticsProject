package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// EventsProduced counts events successfully handed off to the Kafka producer.
	EventsProduced = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ingestion_events_produced_total",
		Help: "Events successfully handed off to the Kafka producer.",
	})

	// EventsDropped counts events rejected at the producer due to backpressure (buffer full → HTTP 503).
	EventsDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ingestion_events_dropped_total",
		Help: "Events dropped due to producer backpressure (buffer full).",
	})

	// EventsFailed counts events that exhausted Kafka retries and could not be delivered.
	EventsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ingestion_events_failed_total",
		Help: "Events that failed to produce to Kafka after all retries.",
	})

	// EventsDLQ counts events forwarded to the dead-letter topic after produce failure.
	EventsDLQ = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ingestion_events_dlq_total",
		Help: "Events forwarded to the dead-letter topic after produce failure.",
	})

	// HTTPRequestDuration tracks HTTP handler latency broken down by method, route, and status.
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ingestion_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"method", "path", "status"},
	)
)

func init() {
	prometheus.MustRegister(
		EventsProduced,
		EventsDropped,
		EventsFailed,
		EventsDLQ,
		HTTPRequestDuration,
	)
}
