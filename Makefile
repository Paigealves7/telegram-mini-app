.PHONY: build run test docker-build clean

APP_NAME = sawmill-backend
CMD_PATH = ./cmd/main.go # Укажи точный путь к main.go

# Локальная сборка бинарника для текущей ОС
build:
	go build -o bin/$(APP_NAME) $(CMD_PATH)

# Кросс-компиляция для Linux (если деплоишь на VPS / Ubuntu)
build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/$(APP_NAME)-linux $(CMD_PATH)

# Запуск приложения
run: build
	./bin/$(APP_NAME)

# Запуск всех тестов
test:
	go test -v ./...

# Очистка за собой
clean:
	rm -rf bin/