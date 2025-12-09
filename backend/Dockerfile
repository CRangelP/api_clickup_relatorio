# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Instala dependências de build
RUN apk add --no-cache git ca-certificates

# Copia go.mod e go.sum primeiro (cache de dependências)
COPY go.mod go.sum ./
RUN go mod download

# Copia código fonte
COPY . .

# Build do binário
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /api \
    ./cmd/api

# Runtime stage
FROM alpine:3.19

# Instala tzdata para suporte a timezone e ca-certificates para HTTPS
RUN apk add --no-cache tzdata ca-certificates

# Configura timezone
ENV TZ=America/Sao_Paulo

# Cria usuário não-root
RUN adduser -D -g '' appuser

WORKDIR /app

# Cria diretório de dados para arquivos temporários (pode ser montado como volume)
RUN mkdir -p /app/data && chown appuser:appuser /app/data
VOLUME /app/data

# Variável de ambiente para diretório de dados
ENV DATA_DIR=/app/data

# Copia binário do builder
COPY --from=builder /api .

# Usa usuário não-root
USER appuser

# Expõe porta
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Executa
CMD ["./api"]
