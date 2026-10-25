package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"telegram-mini-app/connection"
	httptransport "telegram-mini-app/internal/transport/http"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbPool, err := connection.Connect(ctx)
	if err != nil {
		log.Fatalf("Ошибка подключения к PostgreSQL: %v", err)
	}
	defer dbPool.Close()
	log.Println("Подключение к PostgreSQL установлено")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Таймауты защищают от «медленных» соединений, которые иначе держат ресурсы бесконечно.
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           httptransport.NewRouter(dbPool),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second, // с запасом на сборку и отправку PDF
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Сервер запущен на порту %s", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		log.Fatalf("Ошибка работы сервера: %v", err)
	case <-ctx.Done():
		log.Println("Получен сигнал остановки, завершаем работу...")
	}

	// Даём текущим запросам дозавершиться (docker stop ждёт 30 секунд, см. stop_grace_period).
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Ошибка при остановке сервера: %v", err)
	}
	log.Println("Сервер остановлен")
}
