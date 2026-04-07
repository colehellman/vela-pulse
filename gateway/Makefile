BINARY     := bin/server
MIGRATIONS := ../infra/migrations
DATABASE_URL ?= $(shell echo $$DATABASE_URL)
GOOSE      := $(shell go env GOPATH)/bin/goose

.PHONY: run build migrate migrate-status migrate-down lint test sqlc-gen clean

run: build
	./$(BINARY)

build:
	go build -o $(BINARY) ./cmd/server

migrate:
	$(GOOSE) -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" up

migrate-status:
	$(GOOSE) -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" status

migrate-down:
	$(GOOSE) -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" down

lint:
	golangci-lint run ./...

test:
	go test ./... -race -count=1

sqlc-gen:
	sqlc generate

clean:
	rm -rf $(BINARY)
