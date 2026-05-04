package service

import (
	"errors"

	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/domain"
)

// Publisher is the only thing the service needs from the Kafka layer.
// Keeping this interface here means the service package has zero knowledge of Kafka.
type Publisher interface {
	Enqueue(key, value []byte) bool
}

var ErrBufferFull = errors.New("publisher buffer full")

type IngestService struct {
	pub Publisher
}

func NewIngestService(p Publisher) *IngestService {
	return &IngestService{pub: p}
}

func (s *IngestService) Ingest(e *domain.Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	e.Normalise()

	payload, err := e.JSON()
	if err != nil {
		return err
	}

	// Partition by user_id when available so post-login events for the same
	// user are co-partitioned regardless of device. Fall back to anonymous_id
	// so pre-auth events from the same device stay ordered.
	key := e.AnonymousID
	if e.UserID != "" {
		key = e.UserID
	}
	if !s.pub.Enqueue([]byte(key), payload) {
		return ErrBufferFull
	}
	return nil
}

type BatchResult struct {
	Accepted int
	Dropped  int
}

func (s *IngestService) IngestBatch(events []domain.Event) BatchResult {
	var result BatchResult
	for i := range events {
		if err := s.Ingest(&events[i]); err != nil {
			result.Dropped++
		} else {
			result.Accepted++
		}
	}
	return result
}
