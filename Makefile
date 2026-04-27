.PHONY: run build test test-v lint tidy kafka-up kafka-down load-test

run:
	go run ./cmd/server/...

build:
	go build -o bin/ingestion-server ./cmd/server/...

# Unit + handler tests (no Kafka needed)
test:
	go test -race -count=1 ./...

test-v:
	go test -race -count=1 -v ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

# Start single-node Kafka locally for integration testing
kafka-up:
	docker compose up -d
	docker compose wait kafka

kafka-down:
	docker compose down

# Load test against a running server (install k6 first: brew install k6)
# Usage: make load-test BASE_URL=http://localhost:8080 API_KEY=your-key
load-test:
	k6 run -e BASE_URL=$(or $(BASE_URL),http://localhost:8080) -e API_KEY=$(or $(API_KEY),dev-key-change-me-in-prod) k6/load_test.js
