# Этап 1: Компиляция свежего кода
FROM golang:alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Компилируем бинарник прямо внутри Докера
RUN go build -o sawmill-backend ./cmd/main.go

# Этап 2: Запуск
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
# Забираем свежий бинарник и файлы фронтенда
COPY --from=builder /app/sawmill-backend .
COPY --from=builder /app/web ./web

ENV PORT=8080
EXPOSE 8080

CMD ["./sawmill-backend"]