package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RouterConfig holds all tunable options for NewRouter.
type RouterConfig struct {
	APIKeys        []string
	DebugSecret    string
	RateLimitRPS   int // 0 = disabled
	RateLimitBurst int
}

// Pinger is satisfied by *kafka.Producer. Extracted so the router has no kafka import.
type Pinger interface {
	Ping(ctx context.Context) error
}

func NewRouter(h *Handler, cfg RouterConfig, pinger Pinger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(middlewareRecovery(), middlewareBodyLimit(), middlewareRequestID(), middlewareLogger())
	r.Use(middlewareRateLimit(cfg.RateLimitRPS, cfg.RateLimitBurst))

	// ── Infra endpoints (no auth) ──────────────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/ready", func(c *gin.Context) {
		if pinger == nil {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := pinger.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Hidden debug endpoint — only registered when DebugSecret is set.
	// Returns the first configured API key for quick local testing.
	if cfg.DebugSecret != "" {
		r.GET("/___api_key_get", func(c *gin.Context) {
			if c.GetHeader("X-Debug-Secret") != cfg.DebugSecret {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid secret"})
				return
			}
			if len(cfg.APIKeys) == 0 {
				c.JSON(http.StatusOK, gin.H{"api_key": "", "note": "no API keys configured"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"api_key": cfg.APIKeys[0]})
		})
	}

	// ── Authenticated API ──────────────────────────────────────────────────────
	v1 := r.Group("/v1", middlewareAuth(cfg.APIKeys))
	{
		v1.POST("/events", h.Ingest)
		v1.POST("/events/batch", h.IngestBatch)
	}

	return r
}
