package telegramminiapp

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"telegram-mini-app/connection"
	"telegram-mini-app/internal/router"
	"time"
)

func main() {

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbPool, err := connection.Connect(ctx)
	if err != nil {
		log.Fatal("Ошибка подключение к PostgreSQL", err)
	}
	defer dbPool.Close()

	log.Println("Сервер успешно подключен!")

	// 3. Инициализация chi-роутера из нашего нового пакета
	router := router.NewRouter(dbPool)

	// 4. Запуск сервера
	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	go func() {
		log.Println("Сервер запущен на http://localhost:8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ошибка работы сервера: %v", err)
		}
	}()

	// 5. Ожидание сигнала остановки (Ctrl+C)
	<-ctx.Done()
	log.Println("Остановка сервера...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Ошибка при выключении сервера: %v", err)
	}

	log.Println("Сервер успешно остановлен.")

}
