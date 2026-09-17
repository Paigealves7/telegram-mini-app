package repository

import (
	"context"
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

// GetContractors — получение всех контрагентов (Раздел 4: GET /contractors)
func (r *Repository) GetContractors(ctx context.Context) ([]domain.Contractor, error) {
	query := `SELECT id, name FROM contractors ORDER BY id DESC`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	contractors, err := pgx.CollectRows(rows, pgx.RowToStructByName[domain.Contractor])
	if err != nil {
		return nil, err
	}

	return contractors, nil
}

// CreateContractor — создание контрагента (Раздел 4: POST /contractors)
func (r *Repository) CreateContractor(ctx context.Context, name string) (*domain.Contractor, error) {
	query := `INSERT INTO contractors (name) VALUES ($1) RETURNING id, name`

	var c domain.Contractor
	err := r.db.QueryRow(ctx, query, name).Scan(&c.ID, &c.Name)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

// CreateUser — сохранение нового пользователя
func (r *Repository) CreateUser(ctx context.Context, username, passwordHash string, role domain.Role) (*domain.User, error) {
	query := `
		INSERT INTO users (username, password_hash, role) 
		VALUES ($1, $2, $3) 
		RETURNING id, username, role, created_at`

	var user domain.User
	err := r.db.QueryRow(ctx, query, username, passwordHash, role).Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByUsername — поиск пользователя для логина
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	query := `SELECT id, username, password_hash, role, created_at FROM users WHERE username = $1`

	var user domain.User
	err := r.db.QueryRow(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// CreateLogArrival — сохранение поступления леса и списка бревен в транзакции
func (r *Repository) CreateLogArrival(ctx context.Context, arrival *domain.LogArrival, items []domain.LogItem) (*domain.LogArrival, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. Вставляем шапку прихода в log_arrivals
	arrivalQuery := `
		INSERT INTO log_arrivals (supplier_id, carrier_id, truck_number, arrival_date, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	err = tx.QueryRow(ctx, arrivalQuery,
		arrival.SupplierID,
		arrival.CarrierID,
		arrival.TruckNumber,
		arrival.ArrivalDate,
		arrival.CreatedBy,
	).Scan(&arrival.ID, &arrival.CreatedAt)
	if err != nil {
		return nil, err
	}

	// 2. Вставляем позицию каждого бревна в log_items
	itemQuery := `
		INSERT INTO log_items (arrival_id, length_mm, diameter_mm, species, count, volume_m3)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	for i := range items {
		items[i].ArrivalID = arrival.ID
		err := tx.QueryRow(ctx, itemQuery,
			arrival.ID,
			items[i].LengthMM,
			items[i].DiameterMM,
			items[i].Species,
			items[i].Count,
			items[i].VolumeM3,
		).Scan(&items[i].ID)
		if err != nil {
			return nil, err
		}
	}

	// Фиксируем транзакцию
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	arrival.Items = items
	return arrival, nil
}

// CreateSawingOperation — атомарное проведение распила бревен и прихода досок
func (r *Repository) CreateSawingOperation(
	ctx context.Context,
	op *domain.SawingOperation,
	sawedLogs []domain.SawedLog,
	producedBoards []domain.Board,
) (*domain.SawingOperation, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. Создаем шапку операции
	sawingQuery := `
		INSERT INTO sawing_operations (worker_id, date)
		VALUES ($1, $2)
		RETURNING id, created_at`

	err = tx.QueryRow(ctx, sawingQuery, op.WorkerID, op.Date).Scan(&op.ID, &op.CreatedAt)
	if err != nil {
		return nil, err
	}

	// 2. Записываем списываемые бревна
	logQuery := `
		INSERT INTO sawed_logs (sawing_id, log_id, volume_m3)
		VALUES ($1, $2, $3)
		RETURNING id`

	for i := range sawedLogs {
		sawedLogs[i].SawingID = op.ID
		err := tx.QueryRow(ctx, logQuery, op.ID, sawedLogs[i].LogID, sawedLogs[i].VolumeM3).Scan(&sawedLogs[i].ID)
		if err != nil {
			return nil, err
		}
	}

	// 3. Записываем полученные доски на склад пиломатериалов
	boardQuery := `
		INSERT INTO boards (height_mm, width_mm, length_mm, species, grade, count, volume_m3, price_per_m3)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`

	for i := range producedBoards {
		err := tx.QueryRow(ctx, boardQuery,
			producedBoards[i].HeightMM,
			producedBoards[i].WidthMM,
			producedBoards[i].LengthMM,
			producedBoards[i].Species,
			producedBoards[i].Grade,
			producedBoards[i].Count,
			producedBoards[i].VolumeM3,
			producedBoards[i].PricePerM3,
		).Scan(&producedBoards[i].ID)
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

// CreateSale — сохранение продажи и списание досок в транзакции
func (r *Repository) CreateSale(ctx context.Context, sale *domain.Sale, items []domain.SaleItem) (*domain.Sale, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. Создаем запись продажи
	saleQuery := `
		INSERT INTO sales (buyer_id, total_amount, sale_date, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	err = tx.QueryRow(ctx, saleQuery,
		sale.BuyerID,
		sale.TotalAmount,
		sale.SaleDate,
		sale.CreatedBy,
	).Scan(&sale.ID, &sale.CreatedAt)
	if err != nil {
		return nil, err
	}

	// 2. Вставляем позиции продажи
	itemQuery := `
		INSERT INTO sale_items (sale_id, board_id, count, price, volume_m3)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	// 3. Обновляем (уменьшаем) остаток на складе boards
	updateBoardQuery := `
		UPDATE boards 
		SET count = count - $1 
		WHERE id = $2 AND count >= $1`

	for i := range items {
		items[i].SaleID = sale.ID
		err := tx.QueryRow(ctx, itemQuery,
			sale.ID,
			items[i].BoardID,
			items[i].Count,
			items[i].Price,
			items[i].VolumeM3,
		).Scan(&items[i].ID)
		if err != nil {
			return nil, err
		}

		res, err := tx.Exec(ctx, updateBoardQuery, items[i].Count, items[i].BoardID)
		if err != nil {
			return nil, err
		}
		if res.RowsAffected() == 0 {
			return nil, pgx.ErrNoRows // Недостаточно досок на складе
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

	orderQuery := `
		INSERT INTO orders (customer_id, status, total_amount)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`

	err = tx.QueryRow(ctx, orderQuery, order.CustomerID, order.Status, order.TotalAmount).Scan(
		&order.ID,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	itemQuery := `
		INSERT INTO order_items (order_id, height_mm, width_mm, length_mm, species, grade, count, volume_m3, price_per_m3)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	for i := range items {
		items[i].OrderID = order.ID
		err := tx.QueryRow(ctx, itemQuery,
			order.ID,
			items[i].HeightMM,
			items[i].WidthMM,
			items[i].LengthMM,
			items[i].Species,
			items[i].Grade,
			items[i].Count,
			items[i].VolumeM3,
			items[i].PricePerM3,
		).Scan(&items[i].ID)
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

// GetOrders — получение всех заказов
func (r *Repository) GetOrders(ctx context.Context) ([]domain.Order, error) {
	query := `SELECT id, customer_id, status, total_amount, created_at, updated_at FROM orders ORDER BY id DESC`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []domain.Order
	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(&o.ID, &o.CustomerID, &o.Status, &o.TotalAmount, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}

	return orders, nil
}
