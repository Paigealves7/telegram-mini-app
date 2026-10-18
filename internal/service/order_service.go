package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
)

type OrderService struct {
	repo         *repository.Repository
	salesService *SalesService
	botService   *BotService // Добавили сервис уведомлений
}

func NewOrderService(repo *repository.Repository, salesService *SalesService, botService *BotService) *OrderService {
	return &OrderService{repo: repo, salesService: salesService, botService: botService}
}

func (s *OrderService) CreateOrder(ctx context.Context, order *domain.Order, items []domain.OrderItem) (*domain.Order, error) {
	createdOrder, err := s.repo.CreateOrder(ctx, order, items)
	if err != nil {
		return nil, err
	}

	// Асинхронно отправляем уведомление мастеру, чтобы не тормозить HTTP ответ
	go func() {
		msg := fmt.Sprintf("🔔 <b>Новая заявка #%d</b>\n\nНа вас назначен новый заказ. Пожалуйста, подготовьте материалы к отгрузке.", createdOrder.ID)
		_ = s.botService.NotifyUser(context.Background(), createdOrder.MasterID, msg)
	}()

	return createdOrder, nil
}

func (s *OrderService) CompleteOrder(ctx context.Context, orderID int64, userID int64) error {
	// Вся работа (проверка статуса, продажа, смена статуса) выполняется одной транзакцией в репозитории.
	order, err := s.repo.CompleteOrder(ctx, orderID, userID)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrOrderNotFound),
			errors.Is(err, repository.ErrOrderClosed),
			errors.Is(err, repository.ErrOrderEmpty),
			errors.Is(err, repository.ErrInsufficientStock),
			errors.Is(err, repository.ErrBoardNotFound):
			return err
		default:
			log.Printf("COMPLETE ORDER #%d: %v", orderID, err)
			return errors.New("не удалось завершить заказ, попробуйте ещё раз")
		}
	}

	// Уведомляем мастера об успешном завершении
	go func() {
		msg := fmt.Sprintf("✅ <b>Заявка #%d завершена</b>\n\nМатериалы списаны, продажа проведена.", orderID)
		_ = s.botService.NotifyUser(context.Background(), order.MasterID, msg)
	}()

	return nil
}
