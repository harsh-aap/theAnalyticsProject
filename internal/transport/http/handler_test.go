package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/domain"
	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/service"
)

var _ ingester = (*mockIngester)(nil) // compile-time interface check

func init() {
	gin.SetMode(gin.TestMode)
}

// mockIngester implements the ingester interface.
type mockIngester struct {
	ingestErr   error
	batchResult service.BatchResult
}

func (m *mockIngester) Ingest(e *domain.Event) error {
	if m.ingestErr != nil {
		return m.ingestErr
	}
	e.EventID = "test-id"
	return nil
}

func (m *mockIngester) IngestBatch(events []domain.Event) service.BatchResult {
	return m.batchResult
}

func newTestRouter(svc ingester) *gin.Engine {
	return NewRouter(NewHandler(svc), RouterConfig{}, nil)
}

func newTestRouterWithKeys(svc ingester, keys []string) *gin.Engine {
	return NewRouter(NewHandler(svc), RouterConfig{APIKeys: keys}, nil)
}

func newTestRouterWithDebug(svc ingester, keys []string, debugSecret string) *gin.Engine {
	return NewRouter(NewHandler(svc), RouterConfig{APIKeys: keys, DebugSecret: debugSecret}, nil)
}

func post(router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func postWithKey(router *gin.Engine, path string, body any, apiKey string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// --- /v1/events ---

func TestIngest_ValidEvent(t *testing.T) {
	r := newTestRouter(&mockIngester{})
	w := post(r, "/v1/events", map[string]any{"event_type": "click", "user_id": "u1"})

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIngest_BadJSON(t *testing.T) {
	r := newTestRouter(&mockIngester{})
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestIngest_ValidationError(t *testing.T) {
	svc := &mockIngester{ingestErr: errors.New("user_id is required")}
	r := newTestRouter(svc)
	w := post(r, "/v1/events", map[string]any{"event_type": "click"})

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIngest_BufferFull(t *testing.T) {
	svc := &mockIngester{ingestErr: service.ErrBufferFull}
	r := newTestRouter(svc)
	w := post(r, "/v1/events", map[string]any{"event_type": "click", "user_id": "u1"})

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// --- /v1/events/batch ---

func TestIngestBatch_Valid(t *testing.T) {
	svc := &mockIngester{batchResult: service.BatchResult{Accepted: 2, Dropped: 0}}
	r := newTestRouter(svc)
	w := post(r, "/v1/events/batch", []map[string]any{
		{"event_type": "click", "user_id": "u1"},
		{"event_type": "page_view", "user_id": "u2"},
	})

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIngestBatch_EmptyArray(t *testing.T) {
	r := newTestRouter(&mockIngester{})
	w := post(r, "/v1/events/batch", []map[string]any{})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestIngestBatch_TooLarge(t *testing.T) {
	events := make([]map[string]any, 501)
	for i := range events {
		events[i] = map[string]any{"event_type": "click", "user_id": "u1"}
	}
	r := newTestRouter(&mockIngester{})
	w := post(r, "/v1/events/batch", events)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}

func TestIngestBatch_AllDropped(t *testing.T) {
	svc := &mockIngester{batchResult: service.BatchResult{Accepted: 0, Dropped: 2}}
	r := newTestRouter(svc)
	w := post(r, "/v1/events/batch", []map[string]any{
		{"event_type": "click", "user_id": "u1"},
		{"event_type": "click", "user_id": "u2"},
	})

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHealth(t *testing.T) {
	r := newTestRouter(&mockIngester{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// --- auth middleware ---

func TestAuth_NoKeyConfigured_AllowsThrough(t *testing.T) {
	r := newTestRouter(&mockIngester{}) // nil keys = auth disabled
	w := post(r, "/v1/events", map[string]any{"event_type": "click", "user_id": "u1"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 with auth disabled, got %d", w.Code)
	}
}

func TestAuth_MissingHeader_Returns401(t *testing.T) {
	r := newTestRouterWithKeys(&mockIngester{}, []string{"secret-key"})
	w := post(r, "/v1/events", map[string]any{"event_type": "click", "user_id": "u1"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_WrongKey_Returns401(t *testing.T) {
	r := newTestRouterWithKeys(&mockIngester{}, []string{"secret-key"})
	w := postWithKey(r, "/v1/events", map[string]any{"event_type": "click", "user_id": "u1"}, "wrong-key")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_ValidKey_Returns202(t *testing.T) {
	r := newTestRouterWithKeys(&mockIngester{}, []string{"secret-key"})
	w := postWithKey(r, "/v1/events", map[string]any{"event_type": "click", "user_id": "u1"}, "secret-key")
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

func TestAuth_HealthBypassesAuth(t *testing.T) {
	r := newTestRouterWithKeys(&mockIngester{}, []string{"secret-key"})
	req := httptest.NewRequest(http.MethodGet, "/health", nil) // no auth header
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /health without auth, got %d", w.Code)
	}
}

// --- debug key endpoint ---

func debugGET(router *gin.Engine, secret string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/___api_key_get", nil)
	if secret != "" {
		req.Header.Set("X-Debug-Secret", secret)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestDebugKeyEndpoint_NotRegisteredWithoutSecret(t *testing.T) {
	r := newTestRouterWithDebug(&mockIngester{}, []string{"my-api-key"}, "") // no debug secret
	w := debugGET(r, "anything")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when debug secret not configured, got %d", w.Code)
	}
}

func TestDebugKeyEndpoint_WrongSecret_Returns401(t *testing.T) {
	r := newTestRouterWithDebug(&mockIngester{}, []string{"my-api-key"}, "correct-secret")
	w := debugGET(r, "wrong-secret")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong secret, got %d", w.Code)
	}
}

func TestDebugKeyEndpoint_MissingHeader_Returns401(t *testing.T) {
	r := newTestRouterWithDebug(&mockIngester{}, []string{"my-api-key"}, "correct-secret")
	w := debugGET(r, "") // no header sent
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with missing header, got %d", w.Code)
	}
}

func TestDebugKeyEndpoint_CorrectSecret_ReturnsKey(t *testing.T) {
	r := newTestRouterWithDebug(&mockIngester{}, []string{"my-api-key"}, "correct-secret")
	w := debugGET(r, "correct-secret")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "my-api-key") {
		t.Fatalf("expected api_key in response, got: %s", w.Body.String())
	}
}

func TestDebugKeyEndpoint_NoKeysConfigured(t *testing.T) {
	r := newTestRouterWithDebug(&mockIngester{}, nil, "correct-secret") // debug on, but no API keys
	w := debugGET(r, "correct-secret")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"api_key":""`) && !strings.Contains(w.Body.String(), `"api_key": ""`) {
		t.Fatalf("expected empty api_key, got: %s", w.Body.String())
	}
}
