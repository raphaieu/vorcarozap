GO ?= $(shell which go 2>/dev/null || echo /home/raphael/.local/go-1.27/bin/go)
TEMPL ?= $(shell which templ 2>/dev/null || echo /home/raphael/.local/bin/templ)

.PHONY: all templ fmt vet build run migrate test compose-config compose-build compose-up compose-proxy clean

all: templ fmt vet build

templ:
	@echo "==> Gerando templates com templ..."
	@$(TEMPL) generate

fmt:
	@echo "==> Formatando código Go..."
	@$(GO) fmt ./...

vet:
	@echo "==> Executando go vet..."
	@$(GO) vet ./...

build: templ
	@echo "==> Compilando binário vorcarozap..."
	@mkdir -p bin
	@$(GO) build -o bin/vorcarozap ./cmd/vorcarozap

run: build
	@echo "==> Iniciando vorcarozap serve..."
	@./bin/vorcarozap serve

migrate: build
	@echo "==> Executando migrations..."
	@./bin/vorcarozap migrate

test:
	@echo "==> Executando testes..."
	@$(GO) test -v ./...

compose-config:
	@echo "==> Validando configuração do Docker Compose..."
	@docker compose config

compose-build:
	@echo "==> Construindo imagens do Compose..."
	@docker compose build

compose-up:
	@echo "==> Subindo serviço app via Compose..."
	@docker compose up -d app

compose-proxy:
	@echo "==> Subindo app + proxy Caddy via Compose..."
	@docker compose --profile proxy up -d

clean:
	@echo "==> Limpando artefatos..."
	@rm -rf bin/ data/
