package http

import (
	"net/http"
	"os"
	"telegram-mini-app/internal/repository"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(db *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CleanPath)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{},
		AllowedMethods:   []string{"POST", "GET", "DELETE", "PUT", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// 1. Создаем слой репозитория и хэндлеров (Инициализация)
	repo := repository.NewRepository(db)
	dictHandler := NewDictHandler(repo) // из dict_handler.go
	authHandler := NewAuthHandler(repo) // Из auth_handler.go
	arrivalHandler := NewArrivalHandler(repo)
	sawingHandler := NewSawingHandler(repo) // Из sawing_handler.go
	salesHandler := NewSalesHandler(repo)   // Инициализация sales_handler.go
	orderHandler := NewOrderHandler(repo)   // Из order_handler.go
	tgAuthHandler := NewTelegramAuthHandler(repo, os.Getenv("TELEGRAM_BOT_TOKEN"))

	// 3. Эндпоинт проверки здоровья (Health check)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			http.Error(w, "База данных недоступна", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK! Сервер и БД работают."))
	})

	// 4. Защита маршрутов
	r.Route("/api/v1", func(r chi.Router) {

		// Публичные ручки
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/telegram", tgAuthHandler.LoginViaTelegram)
		})

		// Защищенные ручки (требуют JWT)
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware)

			r.Get("/contractors", dictHandler.GetContractors)
			r.Post("/contractors", dictHandler.CreateContractor)

			r.Post("/arrivals", arrivalHandler.CreateArrival)
			r.Post("/sawing", sawingHandler.CreateSawing)
			r.Post("/sales", salesHandler.CreateSale)

			r.Get("/orders", orderHandler.GetOrders)
			r.Post("/orders", orderHandler.CreateOrder)
		})
	})

	return r

}
