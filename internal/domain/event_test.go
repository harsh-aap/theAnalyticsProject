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
			name:  "valid anonymous event",
			event: Event{EventType: "page_view", AnonymousID: "anon-1"},
		},
		{
			name:  "valid authenticated event",
			event: Event{EventType: "purchase", AnonymousID: "anon-1", UserID: "user-1", Price: 9.99},
		},
		{
			name:    "missing event_type",
			event:   Event{AnonymousID: "anon-1"},
			wantErr: "event_type is required",
		},
		{
			name:    "event_type with spaces rejected",
			event:   Event{EventType: "page view", AnonymousID: "anon-1"},
			wantErr: "event_type must be lowercase",
		},
		{
			name:    "event_type with capitals rejected",
			event:   Event{EventType: "PageView", AnonymousID: "anon-1"},
			wantErr: "event_type must be lowercase",
		},
		{
			name:  "new event_type not in schema passes through",
			event: Event{EventType: "video_played", AnonymousID: "anon-1"},
		},
		{
			name:    "missing anonymous_id",
			event:   Event{EventType: "click"},
			wantErr: "anonymous_id is required",
		},
		{
			name:    "negative price",
			event:   Event{EventType: "purchase", AnonymousID: "anon-1", Price: -1},
			wantErr: "price must be non-negative",
		},
		{
			name:  "zero price is fine",
			event: Event{EventType: "click", AnonymousID: "anon-1", Price: 0},
		},
		{
			name:  "valid identify event",
			event: Event{EventType: "identify", AnonymousID: "anon-uuid-1", UserID: "real-user-1"},
		},
		{
			name:    "custom event missing custom_event_name",
			event:   Event{EventType: "custom", AnonymousID: "anon-1"},
			wantErr: "custom_event_name is required for custom events",
		},
		{
			name:  "valid custom event",
			event: Event{EventType: "custom", AnonymousID: "anon-1", CustomEventName: "video_played"},
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
	t.Run("sets event_ts_ms when zero", func(t *testing.T) {
		e := Event{EventType: "click", AnonymousID: "anon-1"}
		e.Normalise()
		if e.EventTsMs == 0 {
			t.Fatal("expected EventTsMs to be set")
		}
	})

	t.Run("preserves existing event_ts_ms", func(t *testing.T) {
		e := Event{EventType: "click", AnonymousID: "anon-1", EventTsMs: 999}
		e.Normalise()
		if e.EventTsMs != 999 {
			t.Fatalf("expected EventTsMs=999, got %d", e.EventTsMs)
		}
	})

	t.Run("generates event_id", func(t *testing.T) {
		e := Event{EventType: "click", AnonymousID: "anon-1"}
		e.Normalise()
		if e.EventID == "" {
			t.Fatal("expected EventID to be set")
		}
	})

	t.Run("preserves caller-supplied event_id", func(t *testing.T) {
		e := Event{EventType: "click", AnonymousID: "anon-1", EventID: "my-id"}
		e.Normalise()
		if e.EventID != "my-id" {
			t.Fatalf("expected EventID=my-id, got %q", e.EventID)
		}
	})

	t.Run("two events same ms get different ids", func(t *testing.T) {
		e1 := Event{EventType: "click", AnonymousID: "anon-1", EventTsMs: 1000}
		e2 := Event{EventType: "click", AnonymousID: "anon-2", EventTsMs: 1000}
		e1.Normalise()
		e2.Normalise()
		if e1.EventID == e2.EventID {
			t.Fatal("different anonymous IDs should produce different event IDs")
		}
	})
}

func TestJSON(t *testing.T) {
	e := Event{EventType: "purchase", AnonymousID: "anon-1", UserID: "u1", Price: 9.99}
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
	if !strings.Contains(string(b), `"event_ts_ms"`) {
		t.Fatalf("JSON missing event_ts_ms: %s", b)
	}
	if !strings.Contains(string(b), `"anonymous_id"`) {
		t.Fatalf("JSON missing anonymous_id: %s", b)
	}
}
