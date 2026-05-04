package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"

	json "github.com/goccy/go-json" // 2-3x faster than encoding/json; already a transitive dep via gin
	"github.com/google/uuid"
)

type Event struct {
	// ── Core — every event ───────────────────────────────────────
	EventID     string `json:"event_id"`
	EventType   string `json:"event_type"`
	UserID      string `json:"user_id,omitempty"`  // empty for anonymous events; populated after login/signup
	AnonymousID string `json:"anonymous_id"`        // always required — client SDK generates on first touch
	SessionID   string `json:"session_id,omitempty"`
	Brand       string `json:"brand,omitempty"`     // which brand this event belongs to
	Platform    string `json:"platform,omitempty"`  // web | mobile | shopify | saas | csv
	Source      string `json:"source,omitempty"`
	EventTsMs   int64  `json:"event_ts_ms"`         // Unix ms; set server-side if zero

	// ── Web platform ─────────────────────────────────────────────
	PageURL          string `json:"page_url,omitempty"`
	Referrer         string `json:"referrer,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	Browser          string `json:"browser,omitempty"`
	BrowserVersion   string `json:"browser_version,omitempty"`
	OS               string `json:"os,omitempty"`
	ScreenResolution string `json:"screen_resolution,omitempty"`
	ViewportSize     string `json:"viewport_size,omitempty"`
	UTMSource        string `json:"utm_source,omitempty"`
	UTMMedium        string `json:"utm_medium,omitempty"`
	UTMCampaign      string `json:"utm_campaign,omitempty"`
	UTMTerm          string `json:"utm_term,omitempty"`

	// ── Mobile platform ──────────────────────────────────────────
	ScreenName    string `json:"screen_name,omitempty"`
	DeviceModel   string `json:"device_model,omitempty"`
	OSVersion     string `json:"os_version,omitempty"`
	AppVersion    string `json:"app_version,omitempty"`
	NetworkType   string `json:"network_type,omitempty"`
	Carrier       string `json:"carrier,omitempty"`
	Locale        string `json:"locale,omitempty"`
	AdvertisingID string `json:"advertising_id,omitempty"`

	// ── Shopify platform ─────────────────────────────────────────
	ShopDomain        string `json:"shop_domain,omitempty"`
	ShopifyOrderID    string `json:"shopify_order_id,omitempty"`
	ShopifyCustomerID string `json:"shopify_customer_id,omitempty"`
	VariantID         string `json:"variant_id,omitempty"`
	FulfillmentStatus string `json:"fulfillment_status,omitempty"`
	ShopifyEventType  string `json:"shopify_event_type,omitempty"`
	Tags              string `json:"tags,omitempty"`

	// ── SaaS platform ────────────────────────────────────────────
	OrgID              string `json:"org_id,omitempty"`
	WorkspaceID        string `json:"workspace_id,omitempty"`
	PlanType           string `json:"plan_type,omitempty"`
	FeatureName        string `json:"feature_name,omitempty"`
	SubscriptionID     string `json:"subscription_id,omitempty"`
	APIVersion         string `json:"api_version,omitempty"`
	TrialDaysRemaining int    `json:"trial_days_remaining,omitempty"`

	// ── Navigation events ────────────────────────────────────────
	ElementID   string `json:"element_id,omitempty"`
	ElementType string `json:"element_type,omitempty"`

	// ── Commerce events ──────────────────────────────────────────
	ProductID     string  `json:"product_id,omitempty"`
	ProductName   string  `json:"product_name,omitempty"`
	Category      string  `json:"category,omitempty"`
	Price         float64 `json:"price,omitempty"`
	Currency      string  `json:"currency,omitempty"`
	Quantity      int     `json:"quantity,omitempty"`
	CartID        string  `json:"cart_id,omitempty"`
	OrderID       string  `json:"order_id,omitempty"`
	Discount      float64 `json:"discount,omitempty"`
	PaymentMethod string  `json:"payment_method,omitempty"`

	// ── Search events ────────────────────────────────────────────
	Query        string `json:"query,omitempty"`
	ResultsCount int    `json:"results_count,omitempty"`

	// ── Custom events ────────────────────────────────────────────
	CustomEventName  string                 `json:"custom_event_name,omitempty"`
	CustomProperties map[string]interface{} `json:"custom_properties,omitempty"`
}

// eventTypeFormat accepts lowercase letters, digits, and underscores — max 64 chars.
// The Go service validates format only. Redshift's event_types table is the
// source of truth for which values are semantically meaningful.
// Adding a new event type to Redshift never requires a Go service change.
var eventTypeFormat = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func (e *Event) Validate() error {
	if strings.TrimSpace(e.EventType) == "" {
		return errors.New("event_type is required")
	}
	if !eventTypeFormat.MatchString(e.EventType) {
		return errors.New("event_type must be lowercase letters, digits, and underscores only (e.g. page_view)")
	}
	if strings.TrimSpace(e.AnonymousID) == "" {
		return errors.New("anonymous_id is required")
	}
	if e.Price < 0 {
		return errors.New("price must be non-negative")
	}
	if e.EventType == "custom" && strings.TrimSpace(e.CustomEventName) == "" {
		return errors.New("custom_event_name is required for custom events")
	}
	return nil
}

// Normalise fills server-side defaults. UUID v7 embeds a millisecond timestamp
// in its first 48 bits, so ORDER BY event_id is equivalent to ORDER BY ingestion_time.
func (e *Event) Normalise() {
	if e.EventTsMs == 0 {
		e.EventTsMs = time.Now().UnixMilli()
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
