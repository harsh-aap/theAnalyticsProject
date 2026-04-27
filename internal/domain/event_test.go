package domain

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		wantErr string
	}{
		{
			name:  "valid event",
			event: Event{EventType: "page_view", UserID: "u1"},
		},
		{
			name:    "missing event_type",
			event:   Event{UserID: "u1"},
			wantErr: "event_type is required",
		},
		{
			name:    "unknown event_type",
			event:   Event{EventType: "checkout", UserID: "u1"},
			wantErr: "unknown event_type",
		},
		{
			name:    "missing user_id",
			event:   Event{EventType: "click"},
			wantErr: "user_id is required",
		},
		{
			name:    "negative price",
			event:   Event{EventType: "purchase", UserID: "u1", Price: -1},
			wantErr: "price must be non-negative",
		},
		{
			name:  "zero price is fine",
			event: Event{EventType: "click", UserID: "u1", Price: 0},
		},
		{
			name:  "valid identify event",
			event: Event{EventType: "identify", UserID: "real-user-1", AnonymousID: "anon-uuid-1"},
		},
		{
			name:    "identify missing anonymous_id",
			event:   Event{EventType: "identify", UserID: "real-user-1"},
			wantErr: "anonymous_id is required for identify events",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.event.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestNormalise(t *testing.T) {
	t.Run("sets timestamp when zero", func(t *testing.T) {
		e := Event{EventType: "click", UserID: "u1"}
		e.Normalise()
		if e.Timestamp == 0 {
			t.Fatal("expected Timestamp to be set")
		}
	})

	t.Run("preserves existing timestamp", func(t *testing.T) {
		e := Event{EventType: "click", UserID: "u1", Timestamp: 999}
		e.Normalise()
		if e.Timestamp != 999 {
			t.Fatalf("expected Timestamp=999, got %d", e.Timestamp)
		}
	})

	t.Run("generates event_id", func(t *testing.T) {
		e := Event{EventType: "click", UserID: "u1"}
		e.Normalise()
		if e.EventID == "" {
			t.Fatal("expected EventID to be set")
		}
	})

	t.Run("preserves caller-supplied event_id", func(t *testing.T) {
		e := Event{EventType: "click", UserID: "u1", EventID: "my-id"}
		e.Normalise()
		if e.EventID != "my-id" {
			t.Fatalf("expected EventID=my-id, got %q", e.EventID)
		}
	})

	t.Run("two events same ms get different ids", func(t *testing.T) {
		e1 := Event{EventType: "click", UserID: "u1", Timestamp: 1000}
		e2 := Event{EventType: "click", UserID: "u2", Timestamp: 1000}
		e1.Normalise()
		e2.Normalise()
		if e1.EventID == e2.EventID {
			t.Fatal("different users should produce different event IDs")
		}
	})
}

func TestJSON(t *testing.T) {
	e := Event{EventType: "purchase", UserID: "u1", Price: 9.99}
	e.Normalise()

	b, err := e.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("expected non-empty JSON")
	}
	if !strings.Contains(string(b), `"purchase"`) {
		t.Fatalf("JSON missing event_type: %s", b)
	}
}
