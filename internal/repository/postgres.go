package repository

import (
	"context"
	"errors"
	"fmt"
	"telegram-mini-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetContractors(ctx context.Context) ([]domain.Contractor, error) {
	query := `SELECT id, name FROM contractors ORDER BY id DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByName[domain.Contractor])
}

func (r *Repository) CreateContractor(ctx context.Context, name string) (*domain.Contractor, error) {
	query := `INSERT INTO contractors (name) VALUES ($1) RETURNING id, name`
	var c domain.Contractor
	err := r.db.QueryRow(ctx, query, name).Scan(&c.ID, &c.Name)
	return &c, err
}

func (r *Repository) CreateUser(ctx context.Context, username, passwordHash string, role domain.Role) (*domain.User, error) {
	query := `
		INSERT INTO users (username, password_hash, role) 
		VALUES ($1, $2, $3) 
		RETURNING id, username, role, created_at`
	var user domain.User
	err := r.db.QueryRow(ctx, query, username, passwordHash, role).Scan(&user.ID, &user.Username, &user.Role, &user.CreatedAt)
	return &user, err
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	query := `SELECT id, username, password_hash, role, created_at FROM users WHERE username = $1`
	var user domain.User
	err := r.db.QueryRow(ctx, query, username).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	return &user, err
}

func (r *Repository) CreateLogArrival(ctx context.Context, arrival *domain.LogArrival, items []domain.LogItem) (*domain.LogArrival, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	arrivalQuery := `
		INSERT INTO log_arrivals (supplier_id, carrier_id, truck_number, arrival_date, created_by)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`
	err = tx.QueryRow(ctx, arrivalQuery, arrival.SupplierID, arrival.CarrierID, arrival.TruckNumber, arrival.ArrivalDate, arrival.CreatedBy).Scan(&arrival.ID, &arrival.CreatedAt)
	if err != nil {
		return nil, err
	}

	itemQuery := `INSERT INTO log_items (arrival_id, length_mm, diameter_mm, species, count, volume_m3) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	for i := range items {
		items[i].ArrivalID = arrival.ID
		err := tx.QueryRow(ctx, itemQuery, arrival.ID, items[i].LengthMM, items[i].DiameterMM, items[i].Species, items[i].Count, items[i].VolumeM3).Scan(&items[i].ID)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	arrival.Items = items
	return arrival, nil
}

func (r *Repository) CreateSawingOperation(ctx context.Context, op *domain.SawingOperation, sawedLogs []domain.SawedLog, producedBoards []domain.Board) (*domain.SawingOperation, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	sawingQuery := `INSERT INTO sawing_operations (worker_id, date) VALUES ($1, $2) RETURNING id, created_at`
	err = tx.QueryRow(ctx, sawingQuery, op.WorkerID, op.Date).Scan(&op.ID, &op.CreatedAt)
	if err != nil {
		return nil, err
	}

	logQuery := `INSERT INTO sawed_logs (sawing_id, log_id, volume_m3) VALUES ($1, $2, $3) RETURNING id`
	for i := range sawedLogs {
		sawedLogs[i].SawingID = op.ID
		err := tx.QueryRow(ctx, logQuery, op.ID, sawedLogs[i].LogID, sawedLogs[i].VolumeM3).Scan(&sawedLogs[i].ID)
		if err != nil {
			return nil, err
		}
	}

	boardQuery := `INSERT INTO boards (height_mm, width_mm, length_mm, species, grade, count, volume_m3, price_per_m3) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`
	for i := range producedBoards {
		err := tx.QueryRow(ctx, boardQuery, producedBoards[i].HeightMM, producedBoards[i].WidthMM, producedBoards[i].LengthMM, producedBoards[i].Species, producedBoards[i].Grade, producedBoards[i].Count, producedBoards[i].VolumeM3, producedBoards[i].PricePerM3).Scan(&producedBoards[i].ID)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	op.Logs = sawedLogs
	op.Boards = producedBoards
	return op, nil
}

func (r *Repository) CreateSale(ctx context.Context, sale *domain.Sale, items []domain.SaleItem) (*domain.Sale, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	saleQuery := `
		INSERT INTO sales (contractor_id, truck_number, delivery_price, extra_price, total_volume, total_price, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`

	err = tx.QueryRow(ctx, saleQuery, sale.ContractorID, sale.TruckNumber, sale.DeliveryPrice, sale.ExtraPrice, sale.TotalVolume, sale.TotalPrice, sale.CreatedBy).Scan(&sale.ID, &sale.CreatedAt)
	if err != nil {
		return nil, err
	}

	itemQuery := `
		INSERT INTO sale_boards (sale_id, board_id, count, volume_m3, price)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	for i := range items {
		// 1. FOR UPDATE Блокирует строку для других транзакций
		var currentCount int
		var currentVolume float64
		err := tx.QueryRow(ctx, `SELECT count, volume_m3 FROM boards WHERE id = $1 FOR UPDATE`, items[i].BoardID).Scan(&currentCount, &currentVolume)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errors.New("доска не найдена на складе")
			}
			return nil, err
		}

		if currentCount < items[i].Count {
			return nil, errors.New("недостаточно товара на складе")
		}

		newCount := currentCount - items[i].Count
		newVolume := currentVolume - items[i].VolumeM3

		// 2. Избегаем нулевого мусора на складе
		if newCount == 0 {
			_, err = tx.Exec(ctx, `DELETE FROM boards WHERE id = $1`, items[i].BoardID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE boards SET count = $1, volume_m3 = $2 WHERE id = $3`, newCount, newVolume, items[i].BoardID)
		}
		if err != nil {
			return nil, err
		}

		// 3. Записываем позицию чека
		items[i].SaleID = sale.ID
		err = tx.QueryRow(ctx, itemQuery, sale.ID, items[i].BoardID, items[i].Count, items[i].VolumeM3, items[i].Price).Scan(&items[i].ID)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	sale.Items = items
	return sale, nil
}

func (r *Repository) CreateOrder(ctx context.Context, order *domain.Order, items []domain.OrderItem) (*domain.Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	orderQuery := `INSERT INTO orders (master_id, contractor_id, notes, delivery_price, extra_price, status) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at, updated_at`
	err = tx.QueryRow(ctx, orderQuery, order.MasterID, order.ContractorID, order.Notes, order.DeliveryPrice, order.ExtraPrice, order.Status).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return nil, err
	}

	itemQuery := `INSERT INTO order_boards (order_id, board_id, count, volume_m3, price) VALUES ($1, $2, $3, $4, $5) RETURNING id`
	for i := range items {
		items[i].OrderID = order.ID
		err := tx.QueryRow(ctx, itemQuery, order.ID, items[i].BoardID, items[i].Count, items[i].VolumeM3, items[i].Price).Scan(&items[i].ID)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	order.Items = items
	return order, nil
}

func (r *Repository) GetOrders(ctx context.Context, limit, offset int) ([]domain.Order, error) {
	query := `SELECT id, master_id, contractor_id, notes, delivery_price, extra_price, status, created_at, updated_at FROM orders ORDER BY id DESC LIMIT $1 OFFSET $2`
	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []domain.Order
	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(&o.ID, &o.MasterID, &o.ContractorID, &o.Notes, &o.DeliveryPrice, &o.ExtraPrice, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, nil
}

func (r *Repository) GetAllBoards(ctx context.Context, filter domain.BoardFilter, limit, offset int) ([]domain.Board, error) {
	query := `SELECT id, height_mm, width_mm, length_mm, species, grade, count, volume_m3, price_per_m3 FROM boards WHERE 1=1`

	args := []interface{}{}
	argID := 1

	if filter.Species != "" {
		query += fmt.Sprintf(" AND species = $%d", argID)
		args = append(args, filter.Species)
		argID++
	}
	if filter.Grade != "" {
		query += fmt.Sprintf(" AND grade = $%d", argID)
		args = append(args, filter.Grade)
		argID++
	}
	if filter.HeightMM > 0 {
		query += fmt.Sprintf(" AND height_mm = $%d", argID)
		args = append(args, filter.HeightMM)
		argID++
	}
	if filter.WidthMM > 0 {
		query += fmt.Sprintf(" AND width_mm = $%d", argID)
		args = append(args, filter.WidthMM)
		argID++
	}
	if filter.LengthMM > 0 {
		query += fmt.Sprintf(" AND length_mm = $%d", argID)
		args = append(args, filter.LengthMM)
		argID++
	}

	query += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", argID, argID+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	boards := make([]domain.Board, 0)
	for rows.Next() {
		var b domain.Board
		var price *float64

		if err := rows.Scan(&b.ID, &b.HeightMM, &b.WidthMM, &b.LengthMM, &b.Species, &b.Grade, &b.Count, &b.VolumeM3, &price); err != nil {
			return nil, err
		}
		b.PricePerM3 = price
		boards = append(boards, b)
	}
	return boards, rows.Err()
}

func (r *Repository) GetAllLogs(ctx context.Context, filter domain.LogFilter, limit, offset int) ([]domain.LogItem, error) {
	query := `SELECT id, arrival_id, length_mm, diameter_mm, species, count, volume_m3 FROM log_items WHERE 1=1`

	args := []interface{}{}
	argID := 1

	if filter.Species != "" {
		query += fmt.Sprintf(" AND species = $%d", argID)
		args = append(args, filter.Species)
		argID++
	}
	if filter.LengthMM > 0 {
		query += fmt.Sprintf(" AND length_mm = $%d", argID)
		args = append(args, filter.LengthMM)
		argID++
	}
	if filter.DiameterMM > 0 {
		query += fmt.Sprintf(" AND diameter_mm = $%d", argID)
		args = append(args, filter.DiameterMM)
		argID++
	}

	query += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", argID, argID+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]domain.LogItem, 0)
	for rows.Next() {
		var l domain.LogItem
		if err := rows.Scan(&l.ID, &l.ArrivalID, &l.LengthMM, &l.DiameterMM, &l.Species, &l.Count, &l.VolumeM3); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (r *Repository) GetStatisticsSummary(ctx context.Context, startDate, endDate string) (*domain.StatisticsSummary, error) {
	stats := &domain.StatisticsSummary{}
	q1 := `SELECT COALESCE(SUM(li.volume_m3), 0) FROM log_items li JOIN log_arrivals la ON li.arrival_id = la.id WHERE la.arrival_date >= $1 AND la.arrival_date <= $2`
	if err := r.db.QueryRow(ctx, q1, startDate, endDate).Scan(&stats.TotalLogsArrivedVolume); err != nil {
		return nil, err
	}

	q2 := `SELECT COALESCE(SUM(sl.volume_m3), 0) FROM sawed_logs sl JOIN sawing_operations so ON sl.sawing_id = so.id WHERE so.date >= $1 AND so.date <= $2`
	if err := r.db.QueryRow(ctx, q2, startDate, endDate).Scan(&stats.TotalLogsSawedVolume); err != nil {
		return nil, err
	}

	q3 := `SELECT COALESCE(SUM(total_price), 0) FROM sales WHERE created_at >= $1::timestamp AND created_at <= $2::timestamp + interval '23 hours 59 minutes 59 seconds'`
	if err := r.db.QueryRow(ctx, q3, startDate, endDate).Scan(&stats.TotalSalesAmount); err != nil {
		return nil, err
	}

	q4 := `SELECT COALESCE(SUM(sb.volume_m3), 0) FROM sale_boards sb JOIN sales s ON sb.sale_id = s.id WHERE s.created_at >= $1::timestamp AND s.created_at <= $2::timestamp + interval '23 hours 59 minutes 59 seconds'`
	if err := r.db.QueryRow(ctx, q4, startDate, endDate).Scan(&stats.TotalSalesVolume); err != nil {
		return nil, err
	}

	return stats, nil
}

func (r *Repository) GetOrderByID(ctx context.Context, id int64) (*domain.Order, error) {
	query := `SELECT id, master_id, contractor_id, notes, delivery_price, extra_price, status, created_at, updated_at FROM orders WHERE id = $1`
	var o domain.Order
	err := r.db.QueryRow(ctx, query, id).Scan(&o.ID, &o.MasterID, &o.ContractorID, &o.Notes, &o.DeliveryPrice, &o.ExtraPrice, &o.Status, &o.CreatedAt, &o.UpdatedAt)
	return &o, err
}

func (r *Repository) GetOrderItems(ctx context.Context, orderID int64) ([]domain.OrderItem, error) {
	query := `SELECT id, order_id, board_id, count, volume_m3, price FROM order_boards WHERE order_id = $1`
	rows, err := r.db.Query(ctx, query, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.OrderItem
	for rows.Next() {
		var i domain.OrderItem
		if err := rows.Scan(&i.ID, &i.OrderID, &i.BoardID, &i.Count, &i.VolumeM3, &i.Price); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, nil
}

func (r *Repository) UpdateOrderStatus(ctx context.Context, id int64, status domain.OrderStatus) error {
	_, err := r.db.Exec(ctx, `UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	return err
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (*domain.User, error) {
	query := `SELECT id, username, password_hash, role, created_at FROM users WHERE id = $1`
	var user domain.User
	err := r.db.QueryRow(ctx, query, id).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	return &user, err
}
