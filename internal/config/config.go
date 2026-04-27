package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port            string
	KafkaBrokers    []string
	KafkaTopic      string
	KafkaDLQTopic   string        // dead-letter topic for events that fail after all retries
	MaxInFlight     int           // max concurrent in-flight Kafka records; acts as backpressure limit
	ShutdownTimeout time.Duration
	APIKeys         []string      // valid bearer tokens; empty = auth disabled (local dev)
	DebugSecret     string        // secret for the hidden key-fetch endpoint; empty = endpoint not registered
	LogLevel        string        // slog level: debug | info | warn | error (default: info)
	RateLimitRPS    int           // global request rate limit per second; 0 = disabled
	RateLimitBurst  int           // burst size for rate limiter
}

func Load() *Config {
	cfg := &Config{
		Port:            getEnv("PORT", "8080"),
		KafkaBrokers:    strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
		KafkaTopic:      getEnv("KAFKA_TOPIC", "events"),
		MaxInFlight:     getEnvInt("MAX_IN_FLIGHT", 4096),
		ShutdownTimeout: time.Duration(getEnvInt("SHUTDOWN_TIMEOUT_SEC", 15)) * time.Second,
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		RateLimitRPS:    getEnvInt("RATE_LIMIT_RPS", 10000),
		RateLimitBurst:  getEnvInt("RATE_LIMIT_BURST", 1000),
	}
	cfg.KafkaDLQTopic = getEnv("KAFKA_DLQ_TOPIC", cfg.KafkaTopic+".dlq")
	if raw := getEnv("API_KEYS", ""); raw != "" {
		cfg.APIKeys = strings.Split(raw, ",")
	}
	cfg.DebugSecret = getEnv("DEBUG_SECRET", "")
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
