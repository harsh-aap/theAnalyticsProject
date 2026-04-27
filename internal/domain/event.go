package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	json "github.com/goccy/go-json" // 2-3x faster than encoding/json; already a transitive dep via gin
	"github.com/google/uuid"
)

type Event struct {
	EventID     string  `json:"event_id"`
	EventType   string  `json:"event_type"`
	UserID      string  `json:"user_id"`
	AnonymousID string  `json:"anonymous_id,omitempty"` // device-generated UUID; present on all events, required for identify
	SessionID   string  `json:"session_id,omitempty"`
	Timestamp   int64   `json:"timestamp"` // Unix ms; set server-side if zero
	ProductID   string  `json:"product_id,omitempty"`
	Price       float64 `json:"price,omitempty"`
	Source      string  `json:"source,omitempty"`
}

var validEventTypes = map[string]bool{
	"page_view":     true,
	"click":         true,
	"add_to_cart":   true,
	"purchase":      true,
	"search":        true,
	"session_start": true,
	"session_end":   true,
	"identify":      true, // maps anonymous_id → user_id for identity stitching
}

func (e *Event) Validate() error {
	if strings.TrimSpace(e.EventType) == "" {
		return errors.New("event_type is required")
	}
	if !validEventTypes[e.EventType] {
		return fmt.Errorf("unknown event_type: %s", e.EventType)
	}
	if strings.TrimSpace(e.UserID) == "" {
		return errors.New("user_id is required")
	}
	if e.Price < 0 {
		return errors.New("price must be non-negative")
	}
	if e.EventType == "identify" && strings.TrimSpace(e.AnonymousID) == "" {
		return errors.New("anonymous_id is required for identify events")
	}
	return nil
}

// Normalise fills server-side defaults. UUID v7 embeds a millisecond timestamp
// in its first 48 bits, so ORDER BY event_id is equivalent to ORDER BY ingestion_time.
func (e *Event) Normalise() {
	if e.Timestamp == 0 {
		e.Timestamp = time.Now().UnixMilli()
	}
	// Only generate an ID if the client didn't supply one.
	// UUID v7: time-ordered — safe for Redshift sort keys and cursor pagination.
	if e.EventID == "" {
		if id, err := uuid.NewV7(); err == nil {
			e.EventID = id.String()
		} else {
			// fallback: uuid v4 is never used for ordering, only as a unique key
			e.EventID = uuid.New().String()
		}
	}
}

func (e *Event) JSON() ([]byte, error) {
	return json.Marshal(e)
}
