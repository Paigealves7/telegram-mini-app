package router

import (
	"net/http"
	"telegram-mini-app/internal/repository"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	arrivalhttp "telegram-mini-app/internal/transport/http"
	authhttp "telegram-mini-app/internal/transport/http"
	dicthttp "telegram-mini-app/internal/transport/http"
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
	dictHandler := dicthttp.NewDictHandler(repo) // из dict_handler.go
	authHandler := authhttp.NewAuthHandler(repo) // Из auth_handler.go
	arrivalHandler := arrivalhttp.NewArrivalHandler(repo)

	// 3. Эндпоинт проверки здоровья (Health check)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			http.Error(w, "База данных недоступна", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK! Сервер и БД работают."))
	})

	// 4. Группировка API по верстке из вашего ТЗ
	r.Route("/api/v1", func(r chi.Router) {

		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register) // POST /api/v1/auth/register
			r.Post("/login", authHandler.Login)       // POST /api/v1/auth/login
		})

		// Справочники
		r.Get("/contractors", dictHandler.GetContractors)    // GET  /api/v1/contractors
		r.Post("/contractors", dictHandler.CreateContractor) // POST /api/v1/contractors

		// Поступления
		r.Post("/arrivals", arrivalHandler.CreateArrival) // POST /api/v1/arrivals

		// Распил
		r.Post("/sawing", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Операция распила"))
		})
	})

	return r

}
