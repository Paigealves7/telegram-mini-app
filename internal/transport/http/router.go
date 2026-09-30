package http

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(db *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"POST", "GET", "DELETE", "PUT", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	repo := repository.NewRepository(db)
	dictHandler := NewDictHandler(repo)
	authHandler := NewAuthHandler(repo)
	arrivalHandler := NewArrivalHandler(repo)
	sawingHandler := NewSawingHandler(repo)
	salesHandler := NewSalesHandler(repo)
	orderHandler := NewOrderHandler(repo)
	tgAuthHandler := NewTelegramAuthHandler(repo, os.Getenv("TELEGRAM_BOT_TOKEN"))
	boardHandler := NewBoardHandler(repo)
	logHandler := NewLogHandler(repo) // Инициализируем новый хендлер

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			http.Error(w, "База данных недоступна", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK! Сервер и БД работают."))
	})

	workDir, err := os.Getwd()
	if err != nil {
		log.Printf("ошибка получения рабочей директории: %v", err)
	}
	webDir := filepath.Join(workDir, "web")
	fileServer := http.FileServer(http.Dir(webDir))

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		filePath := filepath.Join(webDir, filepath.Clean(r.URL.Path))
		info, err := os.Stat(filePath)

		if err != nil || info.IsDir() {
			http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	r.Route("/api/v1", func(r chi.Router) {

		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/telegram", tgAuthHandler.LoginViaTelegram)
		})

		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware)

			// Доступно ВСЕМ авторизованным
			r.Get("/boards", boardHandler.GetList)
			r.Get("/logs", logHandler.GetList) // <-- Добавлен маршрут для сырья
			r.Get("/contractors", dictHandler.GetContractors)

			r.Group(func(r chi.Router) {
				r.Use(RequireRoles(domain.RoleManager, domain.RoleMaster))
				r.Post("/contractors", dictHandler.CreateContractor)
				r.Post("/arrivals", arrivalHandler.CreateArrival)
				r.Post("/sales", salesHandler.CreateSale)
				r.Get("/orders", orderHandler.GetOrders)
				r.Post("/orders", orderHandler.CreateOrder)
			})

			r.Group(func(r chi.Router) {
				r.Use(RequireRoles(domain.RoleManager, domain.RoleWorker))
				r.Post("/sawing", sawingHandler.CreateSawing)
			})
		})
	})

	return r
}
