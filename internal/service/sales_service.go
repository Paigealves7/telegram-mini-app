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

	// Объём и суммы считает репозиторий внутри транзакции по данным склада
	// (цена позиции = за 1 м³), значения от клиента не используются.

	return s.repo.CreateSale(ctx, sale, items)
}
