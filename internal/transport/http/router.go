package http

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
	"telegram-mini-app/internal/service"
	"telegram-mini-app/pkg/telegram"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(db *pgxpool.Pool) http.Handler {
	// 1. Читаем секреты ОДИН раз при старте
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("Критическая ошибка: JWT_SECRET не задан в переменных окружения")
	}

	botToken := strings.Trim(strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")), "\"'")
	if botToken == "" {
		log.Fatal("Критическая ошибка: TELEGRAM_BOT_TOKEN не задан в переменных окружения")
	}

	// Диагностика: какому боту принадлежит токен. Сравни с ботом, через который открываешь Mini App.
	if username, err := telegram.BotUsername(context.Background(), botToken); err != nil {
		log.Printf("ВНИМАНИЕ: токен бота не прошёл проверку getMe: %v", err)
	} else {
		log.Printf("Telegram: токен принадлежит боту @%s (длина токена: %d)", username, len(botToken))
	}

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

	// Подключаем слой Service
	botService := service.NewBotService(botToken, repo)
	salesService := service.NewSalesService(repo)
	orderService := service.NewOrderService(repo, salesService, botService)
	pdfService := service.NewPDFService(repo, jwtSecret)

	// 2. Прокидываем секреты и зависимости в хендлеры
	dictHandler := NewDictHandler(repo)
	authHandler := NewAuthHandler(repo, jwtSecret)
	arrivalHandler := NewArrivalHandler(repo)
	sawingHandler := NewSawingHandler(repo)

	// В SalesHandler теперь передаем Service, а не Repo
	salesHandler := NewSalesHandler(salesService)

	orderHandler := NewOrderHandler(repo, orderService, pdfService, botService)
	tgAuthHandler := NewTelegramAuthHandler(repo, botToken, jwtSecret)
	authLimiter := NewRateLimiter(5) // 5 запросов в минуту
	authLimiter.StartCleanup()
	boardHandler := NewBoardHandler(repo)
	logHandler := NewLogHandler(repo)
	statsHandler := NewStatsHandler(repo)

	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
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

	// Публичные ссылки на накладные (подписанные, живут 10 минут): без них файл не скачать в Telegram.
	r.Get("/files/pdf/{token}/{name}", orderHandler.ServePDF)
	r.Head("/files/pdf/{token}/{name}", orderHandler.ServePDF)

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// Telegram WebView агрессивно кеширует страницу. no-cache заставляет его сверяться
		// с сервером (ETag/Last-Modified) при каждом открытии, а не показывать старую версию.
		w.Header().Set("Cache-Control", "no-cache")
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
			r.Post("/login", authHandler.Login)
			r.Post("/telegram", authLimiter.Middleware(tgAuthHandler.LoginViaTelegram))
		})

		r.Group(func(r chi.Router) {
			// 3. Прокидываем секрет в Middleware
			r.Use(AuthMiddleware(jwtSecret))

			r.Get("/boards", boardHandler.GetList)
			r.Get("/logs", logHandler.GetList)
			r.Get("/contractors", dictHandler.GetContractors)

			r.Group(func(r chi.Router) {
				r.Use(RequireRoles(domain.RoleManager, domain.RoleMaster))
				r.Post("/contractors", dictHandler.CreateContractor)
				r.Post("/arrivals", arrivalHandler.CreateArrival)
				r.Post("/sales", salesHandler.CreateSale)
				r.Get("/orders", orderHandler.GetOrders)
				r.Post("/orders", orderHandler.CreateOrder)
				r.Put("/orders/{id}/complete", orderHandler.CompleteOrder)
				r.Get("/orders/{id}/pdf", orderHandler.DownloadPDF)
				r.Post("/orders/{id}/pdf/link", orderHandler.PDFLink)
				r.Post("/orders/{id}/pdf/send", orderHandler.PDFSend)
			})

			r.Group(func(r chi.Router) {
				r.Use(RequireRoles(domain.RoleManager, domain.RoleWorker))
				r.Post("/sawing", sawingHandler.CreateSawing)
			})

			r.Group(func(r chi.Router) {
				r.Use(RequireRoles(domain.RoleManager))
				r.Get("/statistics", statsHandler.GetSummary)
				r.Post("/users", authHandler.Register) // создание сотрудников: только менеджер
			})
		})
	})

	return r
}
