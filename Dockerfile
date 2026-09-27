FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Копируем бинарный файл
COPY sawmill-backend .
RUN chmod +x ./sawmill-backend

# Копируем папку с фронтендом (index.html)
COPY web ./web

ENV PORT=8080
EXPOSE 8080

CMD ["./sawmill-backend"]