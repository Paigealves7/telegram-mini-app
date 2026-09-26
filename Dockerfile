FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

COPY sawmill-backend .
RUN chmod +x ./sawmill-backend

ENV PORT=8080
EXPOSE 8080

CMD ["./sawmill-backend"]