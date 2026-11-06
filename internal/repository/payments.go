package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"telegram-mini-app/internal/domain"

	"github.com/jackc/pgx/v5"
)

// Сумма заказа = сумма позиций (цена за м³ × объём, по копейкам) + доставка + допы. Совпадает с продажей и накладной.
// Запросы используют алиас таблицы заказов o.
const (
	orderTotalExpr = `(COALESCE((SELECT SUM(ROUND(ob.price * ob.volume_m3, 2)) FROM order_boards ob WHERE ob.order_id = o.id), 0)
		+ COALESCE(o.delivery_price, 0) + COALESCE(o.extra_price, 0))`
	orderPaidExpr = `COALESCE((SELECT SUM(p.amount) FROM order_payments p WHERE p.order_id = o.id), 0)`
)

var (
	ErrPaymentTooLarge  = errors.New("оплата больше остатка по заказу")
	ErrOrderCanceledPay = errors.New("заказ отменён: оплату принять нельзя")
)

func money(v float64) string {
	return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
}

// AddPayment записывает оплату. Переплатить заказ нельзя: так ловятся опечатки в сумме.
func (r *Repository) AddPayment(ctx context.Context, orderID int64, amount float64, note string, userID int64) (*domain.Payment, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status domain.OrderStatus
	if err := tx.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	if status == domain.OrderStatusCanceled {
		return nil, ErrOrderCanceledPay
	}

	var total, paid float64
	if err := tx.QueryRow(ctx, `SELECT `+orderTotalExpr+`, `+orderPaidExpr+` FROM orders o WHERE o.id = $1`, orderID).Scan(&total, &paid); err != nil {
		return nil, err
	}
	amount = domain.Round2(amount)
	remaining := domain.Round2(total - paid)
	if remaining <= 0 {
		return nil, fmt.Errorf("%w: заказ уже оплачен полностью", ErrPaymentTooLarge)
	}
	if amount > remaining+0.005 {
		return nil, fmt.Errorf("%w: осталось %s ₽", ErrPaymentTooLarge, money(remaining))
	}

	p := domain.Payment{OrderID: orderID, Amount: amount, Note: note, CreatedBy: userID}
	err = tx.QueryRow(ctx, `INSERT INTO order_payments (order_id, amount, note, created_by) VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		orderID, amount, note, userID).Scan(&p.ID, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetOrderPayments возвращает итоги по заказу и историю оплат.
func (r *Repository) GetOrderPayments(ctx context.Context, orderID int64) (*domain.OrderPayments, error) {
	res := domain.OrderPayments{OrderID: orderID, Payments: []domain.Payment{}}
	err := r.db.QueryRow(ctx, `SELECT o.contractor_id, o.status, `+orderTotalExpr+`, `+orderPaidExpr+` FROM orders o WHERE o.id = $1`, orderID).
		Scan(&res.ContractorID, &res.Status, &res.TotalPrice, &res.PaidAmount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	rows, err := r.db.Query(ctx, `SELECT id, order_id, amount, note, COALESCE(created_by, 0), created_at FROM order_payments WHERE order_id = $1 ORDER BY id`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p domain.Payment
		if err := rows.Scan(&p.ID, &p.OrderID, &p.Amount, &p.Note, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, err
		}
		res.Payments = append(res.Payments, p)
	}
	return &res, rows.Err()
}

// DeletePayment удаляет ошибочную оплату. Возвращает false, если её нет.
func (r *Repository) DeletePayment(ctx context.Context, id int64) (bool, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM order_payments WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// GetDebts возвращает завершённые заказы с неоплаченным остатком.
func (r *Repository) GetDebts(ctx context.Context) ([]domain.DebtOrder, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, contractor_id, created_at, total, paid FROM (
			SELECT o.id, o.contractor_id, o.created_at, `+orderTotalExpr+` AS total, `+orderPaidExpr+` AS paid
			FROM orders o WHERE o.status = 'completed'
		) t WHERE total - paid > 0.005 ORDER BY contractor_id, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.DebtOrder, 0)
	for rows.Next() {
		var d domain.DebtOrder
		if err := rows.Scan(&d.OrderID, &d.ContractorID, &d.CreatedAt, &d.Total, &d.Paid); err != nil {
			return nil, err
		}
		d.Due = domain.Round2(d.Total - d.Paid)
		out = append(out, d)
	}
	return out, rows.Err()
}
