package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/config"
	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/kafka"
	"github.com/harsh-aap/theAnalyticsProject/ingestion/internal/service"
	httpTransport "github.com/harsh-aap/theAnalyticsProject/ingestion/internal/transport/http"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env if present — silently ignored in prod where real env vars are set.
	_ = godotenv.Load()

	cfg := config.Load()

	// Structured JSON logging — parseable by Datadog, Loki, CloudWatch, etc.
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	producer, err := kafka.New(cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaDLQTopic, cfg.KafkaAuthType, cfg.MaxInFlight)
	if err != nil {
		slog.Error("failed to create kafka producer", "error", err)
		os.Exit(1)
	}

	svc := service.NewIngestService(producer)
	handler := httpTransport.NewHandler(svc)
	router := httpTransport.NewRouter(handler, httpTransport.RouterConfig{
		APIKeys:        cfg.APIKeys,
		DebugSecret:    cfg.DebugSecret,
		RateLimitRPS:   cfg.RateLimitRPS,
		RateLimitBurst: cfg.RateLimitBurst,
	}, producer)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,

		// Timeouts prevent slow-client head-of-line blocking.
		// ReadHeaderTimeout is the most important: it stops slow-loris attacks
		// and frees goroutines that are stuck waiting on headers.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10, // 64 KB
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("http server shutdown error", "error", err)
	}

	// Drain Kafka only after HTTP is fully closed — no new Enqueue() calls possible.
	producer.Close()
	slog.Info("shutdown complete")
}
