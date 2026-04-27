package service

import (
	"testing"

	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/domain"
)

// mockPublisher implements Publisher for tests.
type mockPublisher struct {
	full    bool
	queued  int
}

func (m *mockPublisher) Enqueue(_, _ []byte) bool {
	if m.full {
		return false
	}
	m.queued++
	return true
}

func TestIngest(t *testing.T) {
	t.Run("valid event is accepted", func(t *testing.T) {
		pub := &mockPublisher{}
		svc := NewIngestService(pub)

		e := &domain.Event{EventType: "click", UserID: "u1"}
		if err := svc.Ingest(e); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pub.queued != 1 {
			t.Fatalf("expected 1 enqueued record, got %d", pub.queued)
		}
		if e.EventID == "" {
			t.Fatal("Normalise should have set EventID")
		}
	})

	t.Run("invalid event returns validation error", func(t *testing.T) {
		pub := &mockPublisher{}
		svc := NewIngestService(pub)

		e := &domain.Event{EventType: "click"} // missing user_id
		if err := svc.Ingest(e); err == nil {
			t.Fatal("expected validation error")
		}
		if pub.queued != 0 {
			t.Fatal("invalid event should not be enqueued")
		}
	})

	t.Run("full buffer returns ErrBufferFull", func(t *testing.T) {
		pub := &mockPublisher{full: true}
		svc := NewIngestService(pub)

		e := &domain.Event{EventType: "click", UserID: "u1"}
		err := svc.Ingest(e)
		if err != ErrBufferFull {
			t.Fatalf("expected ErrBufferFull, got %v", err)
		}
	})
}

func TestIngestBatch(t *testing.T) {
	t.Run("all valid events accepted", func(t *testing.T) {
		pub := &mockPublisher{}
		svc := NewIngestService(pub)

		events := []domain.Event{
			{EventType: "click", UserID: "u1"},
			{EventType: "page_view", UserID: "u2"},
		}
		result := svc.IngestBatch(events)
		if result.Accepted != 2 || result.Dropped != 0 {
			t.Fatalf("expected 2 accepted 0 dropped, got %+v", result)
		}
	})

	t.Run("mixed valid and invalid", func(t *testing.T) {
		pub := &mockPublisher{}
		svc := NewIngestService(pub)

		events := []domain.Event{
			{EventType: "click", UserID: "u1"},   // valid
			{EventType: "click"},                  // invalid: missing user_id
			{EventType: "purchase", UserID: "u3"}, // valid
		}
		result := svc.IngestBatch(events)
		if result.Accepted != 2 || result.Dropped != 1 {
			t.Fatalf("expected 2 accepted 1 dropped, got %+v", result)
		}
	})

	t.Run("full buffer drops everything", func(t *testing.T) {
		pub := &mockPublisher{full: true}
		svc := NewIngestService(pub)

		events := []domain.Event{
			{EventType: "click", UserID: "u1"},
			{EventType: "click", UserID: "u2"},
		}
		result := svc.IngestBatch(events)
		if result.Accepted != 0 || result.Dropped != 2 {
			t.Fatalf("expected 0 accepted 2 dropped, got %+v", result)
		}
	})
}
