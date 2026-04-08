.PHONY: dev build test lint migrate-up migrate-down docker-build

dev:
	docker compose up -d postgres redis
	air

build:
	go build -o bin/goalden-api ./cmd/server

test:
	go test ./... -v -race

lint:
	golangci-lint run ./...

migrate-up:
	migrate -path internal/database/migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path internal/database/migrations -database "$(DATABASE_URL)" down 1

docker-build:
	docker build -t goalden-api .

docker-up:
	docker compose up -d

docker-down:
	docker compose down
