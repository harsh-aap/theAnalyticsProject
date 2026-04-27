# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Cache dependency downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o bin/ingestion-server \
    ./cmd/server/...

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.21

# ca-certificates: needed for TLS connections to Kafka brokers over the internet.
# tzdata: ensures time.LoadLocation works if event timestamps need timezone handling.
RUN apk --no-cache add ca-certificates tzdata

# Run as a non-root user — principle of least privilege.
RUN addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder /app/bin/ingestion-server .

USER app

EXPOSE 8080

ENTRYPOINT ["./ingestion-server"]
