# ============================================
# Stage 1: BUILDER — собираем бинарник
# ============================================
FROM golang:1.26.0-alpine AS builder

WORKDIR /app

# git нужен для go mod download
RUN apk add --no-cache git ca-certificates

# Кэшируем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходники
COPY . .

# Собираем бинарник под AMD64 (сервер)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /sawmill-backend \
    ./cmd/main.go

# ============================================
# Stage 2: FINAL — минимальный образ
# ============================================
FROM alpine:3.20

# Сертификаты, таймзоны, wget (для healthcheck)
RUN apk --no-cache add ca-certificates tzdata wget

# Непривилегированный пользователь
RUN adduser -D -u 1000 appuser

WORKDIR /app

# Копируем бинарник из builder
COPY --from=builder /sawmill-backend .
RUN chmod +x ./sawmill-backend

# Копируем миграции
COPY --from=builder /app/migrations ./migrations

# Копируем веб-файлы
COPY --from=builder /app/web ./web

# Владелец — appuser
RUN chown -R appuser:appuser /app

USER appuser

ENV PORT=8080
EXPOSE 8080

# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./sawmill-backend"]
