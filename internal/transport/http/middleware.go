package http

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/metrics"
	"golang.org/x/time/rate"
)

// maxBodyBytes is the hard cap on request body size (1 MB).
// Matches ProducerBatchMaxBytes on the Kafka side — no point accepting
// a payload we couldn't forward in a single batch anyway.
const maxBodyBytes = 1 << 20 // 1 MB

const requestIDHeader = "X-Request-ID"

func middlewareRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}

func middlewareLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		dur := time.Since(start)
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", dur.Milliseconds(),
			"request_id", c.GetString("request_id"),
			"client_ip", c.ClientIP(),
		)
		metrics.HTTPRequestDuration.WithLabelValues(
			c.Request.Method,
			c.FullPath(),
			strconv.Itoa(c.Writer.Status()),
		).Observe(dur.Seconds())
	}
}

func middlewareRecovery() gin.HandlerFunc {
	return gin.RecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		slog.Error("panic recovered", "error", recovered, "path", c.Request.URL.Path)
		c.AbortWithStatusJSON(500, gin.H{"error": "internal server error"})
	})
}

// middlewareBodyLimit caps the request body at maxBodyBytes.
// Requests that exceed the limit get a 413 before the body is fully read.
func middlewareBodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
		c.Next()
	}
}

// middlewareRateLimit enforces a global token-bucket rate limit.
// rps=0 disables the middleware entirely (local dev / tests).
func middlewareRateLimit(rps, burst int) gin.HandlerFunc {
	if rps == 0 {
		return func(c *gin.Context) { c.Next() }
	}
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	return func(c *gin.Context) {
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded, slow down"})
			return
		}
		c.Next()
	}
}

// middlewareAuth validates Bearer tokens against the provided key set.
// If keys is empty, auth is disabled (local dev / no-auth mode).
func middlewareAuth(keys []string) gin.HandlerFunc {
	if len(keys) == 0 {
		return func(c *gin.Context) { c.Next() }
	}

	valid := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		valid[k] = struct{}{}
	}

	return func(c *gin.Context) {
		token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing Authorization header"})
			return
		}
		if _, ok := valid[token]; !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			return
		}
		c.Next()
	}
}
