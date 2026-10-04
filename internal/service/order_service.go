package service

import (
	"context"
	"errors"
	"fmt"
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
	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Status == domain.OrderStatusCompleted || order.Status == domain.OrderStatusCanceled {
		return errors.New("заказ уже завершен или отменен")
	}

	items, err := s.repo.GetOrderItems(ctx, orderID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return errors.New("невозможно завершить пустой заказ")
	}

	sale := &domain.Sale{
		ContractorID:  order.ContractorID,
		TruckNumber:   "Доставка по заявке",
		DeliveryPrice: order.DeliveryPrice,
		ExtraPrice:    order.ExtraPrice,
		CreatedBy:     userID,
	}

	saleItems := make([]domain.SaleItem, len(items))
	for i, item := range items {
		saleItems[i] = domain.SaleItem{
			BoardID:  item.BoardID,
			Count:    item.Count,
			Price:    item.Price,
			VolumeM3: item.VolumeM3,
		}
	}

	_, err = s.salesService.CreateSale(ctx, sale, saleItems)
	if err != nil {
		return errors.New("ошибка конвертации заявки в продажу: " + err.Error())
	}

	err = s.repo.UpdateOrderStatus(ctx, orderID, domain.OrderStatusCompleted)

	// Уведомляем мастера об успешном завершении
	if err == nil {
		go func() {
			msg := fmt.Sprintf("✅ <b>Заявка #%d завершена</b>\n\nМатериалы списаны, продажа проведена.", orderID)
			_ = s.botService.NotifyUser(context.Background(), order.MasterID, msg)
		}()
	}

	return err
}
