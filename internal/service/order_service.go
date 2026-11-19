package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"strings"
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

func (s *OrderService) CreateOrder(ctx context.Context, orgID int64, order *domain.Order, items []domain.OrderItem) (*domain.Order, error) {
	createdOrder, err := s.repo.CreateOrder(ctx, orgID, order, items)
	if err != nil {
		return nil, err
	}

	client, clientErr := s.repo.GetContractorName(ctx, orgID, createdOrder.ContractorID)
	lines, linesErr := s.repo.GetOrderLines(ctx, orgID, createdOrder.ID)
	if clientErr != nil || strings.TrimSpace(client) == "" {
		client = fmt.Sprintf("Клиент №%d", createdOrder.ContractorID)
	}
	var msg strings.Builder
	fmt.Fprintf(&msg, "<b>Новая заявка №%d</b>\nПокупатель: %s\n\n<b>Состав заказа:</b>\n", createdOrder.OrderNumber, html.EscapeString(client))
	if linesErr != nil {
		log.Printf("ORDER %d: не удалось загрузить позиции уведомления: %v", createdOrder.ID, linesErr)
		msg.WriteString("Состав временно недоступен.")
	} else {
		total := domain.Round2(createdOrder.DeliveryPrice + createdOrder.ExtraPrice)
		for _, line := range lines {
			total += domain.LineTotal(line.Price, line.VolumeM3)
			count := ""
			if line.FirewoodKind == "" {
				count = formatInt(line.Count) + " шт "
			}
			fmt.Fprintf(&msg, "• %s — %s(%s м³)\n", html.EscapeString(itemTitle(line)), count, formatVolume(line.VolumeM3))
		}
		fmt.Fprintf(&msg, "\n<b>Сумма:</b> %s ₽", formatMoney(domain.Round2(total)))
	}
	s.botService.NotifyAsync(createdOrder.MasterID, msg.String())

	return createdOrder, nil
}

func (s *OrderService) CompleteOrder(ctx context.Context, orgID, orderID, userID int64) error {
	// Вся работа (проверка статуса, продажа, смена статуса) выполняется одной транзакцией в репозитории.
	order, err := s.repo.CompleteOrder(ctx, orgID, orderID, userID)
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
	msg := fmt.Sprintf("<b>Заявка №%d завершена</b>\nМатериалы списаны со склада, продажа проведена.", order.OrderNumber)
	s.botService.NotifyAsync(order.MasterID, msg)

	return nil
}
