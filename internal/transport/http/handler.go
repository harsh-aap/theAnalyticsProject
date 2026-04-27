package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/domain"
	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/service"
)

const maxBatchSize = 500

// ingester is satisfied by *service.IngestService.
// Extracted as an interface so handler tests can inject a mock.
type ingester interface {
	Ingest(e *domain.Event) error
	IngestBatch(events []domain.Event) service.BatchResult
}

type Handler struct {
	svc ingester
}

func NewHandler(svc ingester) *Handler {
	return &Handler{svc: svc}
}

// POST /v1/events
func (h *Handler) Ingest(c *gin.Context) {
	var e domain.Event
	if err := c.ShouldBindJSON(&e); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.Ingest(&e); err != nil {
		if errors.Is(err, service.ErrBufferFull) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "producer buffer full, retry later"})
			return
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"event_id": e.EventID})
}

// POST /v1/events/batch
func (h *Handler) IngestBatch(c *gin.Context) {
	var events []domain.Event
	if err := c.ShouldBindJSON(&events); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(events) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "batch must not be empty"})
		return
	}
	if len(events) > maxBatchSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": "batch exceeds maximum size of 500",
		})
		return
	}

	result := h.svc.IngestBatch(events)

	status := http.StatusAccepted
	if result.Accepted == 0 {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{"accepted": result.Accepted, "dropped": result.Dropped})
}
