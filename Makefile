COMPOSE ?= docker compose
DEV_RUN := $(COMPOSE) --profile dev run --rm dev

.PHONY: all templ fmt tidy test test-race vet build shell run migrate compose-config compose-build compose-up compose-proxy clean

all: templ fmt tidy test vet build

templ:
	$(DEV_RUN) go tool templ generate

sqlc:
	$(DEV_RUN) go tool sqlc generate

fmt:
	$(DEV_RUN) gofmt -w .

tidy:
	$(DEV_RUN) go mod tidy

test:
	$(DEV_RUN) go test ./...

test-race:
	$(DEV_RUN) go test -race ./...

vet:
	$(DEV_RUN) go vet ./...

build:
	$(DEV_RUN) go build ./...

shell:
	$(DEV_RUN) sh

run:
	$(COMPOSE) up --build app

migrate:
	$(COMPOSE) run --rm app migrate

compose-config:
	$(COMPOSE) config

compose-build:
	$(COMPOSE) build

compose-up:
	$(COMPOSE) up -d app

compose-proxy:
	$(COMPOSE) --profile proxy up -d

clean:
	rm -rf bin/ data/
