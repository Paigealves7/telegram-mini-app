.PHONY: build run test docker-build clean

APP_NAME = sawmill-backend
CMD_PATH = ./cmd/main.go

# Локальная сборка бинарника для текущей ОС
build:
	go build -o bin/$(APP_NAME) $(CMD_PATH)

# Кросс-компиляция для Linux (если деплоишь на VPS / Ubuntu)
build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/$(APP_NAME)-linux $(CMD_PATH)

run: build
	./bin/$(APP_NAME)

test:
	go test -v ./...

clean:
	rm -rf bin/