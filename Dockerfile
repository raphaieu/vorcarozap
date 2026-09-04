# Stage 1: Build da aplicação Go
FROM golang:1.27-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src

# Dependências do Go
COPY go.mod go.sum ./
RUN go mod download

# Código fonte
COPY . .

# Geração de templates templ
RUN go tool templ generate

# Compilação do binário estático (sem CGO)
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/vorcarozap ./cmd/vorcarozap

# Stage 2: Imagem final enxuta
FROM alpine:3.21

# Instala certificados SSL e curl para healthcheck
RUN apk add --no-cache ca-certificates curl tzdata

# Criação de usuário e grupo não-root
RUN addgroup -g 10001 -S appgroup && \
    adduser -u 10001 -S appuser -G appgroup

# Criação do diretório de dados persistentes do SQLite
RUN mkdir -p /data && chown -R appuser:appgroup /data

# Cópia do binário compilado
COPY --from=builder /bin/vorcarozap /usr/local/bin/vorcarozap

USER appuser

ENV APP_PORT=8080 \
    APP_ENV=production \
    DB_PATH=/data/vorcarozap.db

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --retries=3 \
    CMD curl -f http://localhost:8080/health/live || exit 1

ENTRYPOINT ["/usr/local/bin/vorcarozap"]
CMD ["serve"]
