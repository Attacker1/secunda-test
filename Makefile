.PHONY: build run test test-unit test-integration cover vet tidy up down docker-build

build:
	go build -o bin/api ./cmd/api

run:
	go run ./cmd/api -config config.yaml

vet:
	go vet ./...

tidy:
	go mod tidy

# Только unit-тесты (без Docker/testcontainers).
test-unit:
	go test ./internal/service/... -count=1

# Интеграционные тесты поднимают MySQL через testcontainers (нужен Docker).
test-integration:
	go test -tags=integration ./internal/repository/... -count=1

# Все тесты.
test:
	go test -tags=integration ./... -count=1

# Покрытие критичных методов (service-слой).
cover:
	go test ./internal/service/... -coverprofile=coverage.out -count=1
	go tool cover -func=coverage.out | tail -n 1

up:
	docker compose up --build

down:
	docker compose down -v

docker-build:
	docker build -t task-service:latest .
