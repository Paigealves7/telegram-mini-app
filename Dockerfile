# 1. Этап сборки (Builder)
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Кэшируем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем бинарник с отключением CGO
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /sawmill-backend ./cmd/main.go

# 2. Финальный образ
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

COPY --from=builder /sawmill-backend .

ENV PORT=8080

EXPOSE 8080

CMD ["./sawmill-backend"]