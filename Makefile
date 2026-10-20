BIN     := bin/pgex
COMPOSE := ./scripts/compose.sh -f db/compose.yaml

.PHONY: build test lint run db-up db-down db-reset check-clubdata

build:
	go build -o $(BIN) ./cmd/pgex

test:
	go test -race ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

run: build
	./$(BIN)

check-clubdata:
	@test -f db/clubdata.sql || { \
	  echo "error: db/clubdata.sql is missing."; \
	  echo "Download the country club dataset from pgexercises.com and save it as db/clubdata.sql, then rerun 'make db-up'."; \
	  exit 1; }

db-up: check-clubdata
	$(COMPOSE) up -d

db-down:
	$(COMPOSE) down

# Drop the volume so init scripts run again on next start.
db-reset: check-clubdata
	$(COMPOSE) down -v
	$(COMPOSE) up -d
