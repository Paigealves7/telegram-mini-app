package repository

import (
	"context"
	"os"
	"telegram-mini-app/internal/domain"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateLogArrival_Transaction(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("Пропуск интеграционного теста: TEST_DATABASE_URL не задан")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("не удалось подключиться к тестовой БД: %v", err)
	}
	defer pool.Close()

	repo := NewRepository(pool)

	supplier, err := repo.CreateContractor(ctx, "Тестовый Поставщик")
	if err != nil {
		t.Fatalf("ошибка создания тестового поставщика: %v", err)
	}

	carrier, err := repo.CreateContractor(ctx, "Тестовый Перевозчик")
	if err != nil {
		t.Fatalf("ошибка создания тестового перевозчика: %v", err)
	}

	user, err := repo.CreateUser(ctx, "test_master", "password_hash", domain.RoleMaster)
	if err != nil {
		t.Fatalf("ошибка создания тестового пользователя: %v", err)
	}

	//тест с реальными ID из базы
	arrival := &domain.LogArrival{
		SupplierID:  supplier.ID,
		CarrierID:   carrier.ID,
		TruckNumber: "A123BC777",
		ArrivalDate: "2026-09-18",
		CreatedBy:   user.ID,
	}

	items := []domain.LogItem{
		{LengthMM: 6000, DiameterMM: 240, Species: "Сосна", Count: 10, VolumeM3: 2.5},
	}

	result, err := repo.CreateLogArrival(ctx, arrival, items)
	if err != nil {
		t.Fatalf("ошибка выполнения транзакции: %v", err)
	}

	if result.ID == 0 {
		t.Error("ожидался валидный ID созданного поступления")
	}
	if len(result.Items) != 1 || result.Items[0].ID == 0 {
		t.Error("позиции бревен не были сохранены в транзакции")
	}
}
