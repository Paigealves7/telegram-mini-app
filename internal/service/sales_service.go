package service

import (
	"context"
	"errors"
	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"
)

type SalesService struct {
	repo *repository.Repository
}

func NewSalesService(repo *repository.Repository) *SalesService {
	return &SalesService{repo: repo}
}

func (s *SalesService) CreateSale(ctx context.Context, sale *domain.Sale, items []domain.SaleItem) (*domain.Sale, error) {
	if len(items) == 0 {
		return nil, errors.New("позиции продажи не могут быть пустыми")
	}

	var totalVolume float64
	var totalPrice float64

	// Пересчет объемов и сумм на сервере (защита от фейковых данных клиента)
	for _, item := range items {
		totalVolume += item.VolumeM3
		totalPrice += item.Price // предполагается, что клиент присылает итоговую цену за эту позицию
	}

	sale.TotalVolume = totalVolume
	sale.TotalPrice = totalPrice + sale.DeliveryPrice + sale.ExtraPrice

	return s.repo.CreateSale(ctx, sale, items)
}
