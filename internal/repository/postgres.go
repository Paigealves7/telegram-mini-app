package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"telegram-mini-app/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetContractors(ctx context.Context, orgID int64) ([]domain.Contractor, error) {
	query := `SELECT id, name, is_supplier, is_carrier, is_client, is_active FROM contractors WHERE organization_id = $1 ORDER BY id DESC`
	rows, err := r.db.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByName[domain.Contractor])
}

// CreateContractor добавляет контрагента без разбора по типам (подходит всем ролям).
func (r *Repository) CreateContractor(ctx context.Context, orgID int64, name string) (*domain.Contractor, error) {
	return r.CreateContractorTyped(ctx, orgID, name, true, true, true)
}

func (r *Repository) CreateContractorTyped(ctx context.Context, orgID int64, name string, supplier, carrier, client bool) (*domain.Contractor, error) {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	query := `INSERT INTO contractors (organization_id, name, is_supplier, is_carrier, is_client) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, is_supplier, is_carrier, is_client, is_active`
	var c domain.Contractor
	err = tx.QueryRow(ctx, query, orgID, name, supplier, carrier, client).
		Scan(&c.ID, &c.Name, &c.IsSupplier, &c.IsCarrier, &c.IsClient, &c.IsActive)
	if err != nil {
		return nil, err
	}
	return &c, tx.Commit(ctx)
}

// UpdateContractor меняет название, типы и признак «скрыт». Возвращает false, если контрагента нет.
func (r *Repository) UpdateContractor(ctx context.Context, orgID int64, id int64, name string, supplier, carrier, client, active bool) (bool, error) {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE contractors SET name = $3, is_supplier = $4, is_carrier = $5, is_client = $6, is_active = $7 WHERE organization_id = $1 AND id = $2`,
		orgID, id, name, supplier, carrier, client, active)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, tx.Commit(ctx)
}

var ErrContractorInUse = errors.New("у контрагента есть приходы, продажи или заказы: скройте его вместо удаления")

// DeleteContractor удаляет контрагента, только если на него нет ссылок. Возвращает false, если его нет.
func (r *Repository) DeleteContractor(ctx context.Context, orgID int64, id int64) (bool, error) {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM contractors WHERE organization_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return false, ErrContractorInUse
		}
		return false, err
	}
	return tag.RowsAffected() > 0, tx.Commit(ctx)
}

var ErrDeletedUser = errors.New("этот аккаунт удалён")

type userRowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func nextUserID(ctx context.Context, db userRowQuerier) (int64, error) {
	var id int64
	for {
		if err := db.QueryRow(ctx, `SELECT nextval(pg_get_serial_sequence('users','id'))`).Scan(&id); err != nil {
			return 0, err
		}
		// Новый аккаунт не должен случайно получить зарезервированный ID.
		if !(domain.User{ID: id}).IsSuperAdmin() {
			return id, nil
		}
	}
}

func (r *Repository) CreateUser(ctx context.Context, username, passwordHash string, role domain.Role) (*domain.User, error) {
	id, err := nextUserID(ctx, r.db)
	if err != nil {
		return nil, err
	}
	query := `
		INSERT INTO users (id, username, password_hash, role)
		SELECT $1::bigint, $2::text, $3::text, $4::role
		RETURNING id, username, role, created_at`
	var user domain.User
	err = r.db.QueryRow(ctx, query, id, username, passwordHash, role).Scan(&user.ID, &user.Username, &user.Role, &user.CreatedAt)
	return &user, err
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	query := `SELECT id, username, password_hash, role, created_at FROM users WHERE username = $1`
	var user domain.User
	err := r.db.QueryRow(ctx, query, username).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	return &user, err
}

// journalSearchPattern возвращает шаблон для поиска буквального фрагмента.
func journalSearchPattern(search []string) string {
	if len(search) == 0 {
		return ""
	}
	q := strings.TrimSpace(search[0])
	if q == "" {
		return ""
	}
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
}

// journalBoardDimensions используется в запросах, где boards имеет псевдоним b.
const journalBoardDimensions = `(
	b.height_mm::text ILIKE $4 OR b.width_mm::text ILIKE $4 OR b.length_mm::text ILIKE $4
	OR concat_ws('×', b.height_mm, b.width_mm, b.length_mm) ILIKE translate($4, 'xXхХ ', '××××')
)`

// GetLogArrivals returns log arrivals and their tally items.
func (r *Repository) GetLogArrivals(ctx context.Context, orgID int64, limit, offset int, search ...string) ([]domain.LogArrival, error) {
	rows, err := r.db.Query(ctx, `
		SELECT la.id, COALESCE(la.supplier_id, 0), COALESCE(la.carrier_id, 0), la.truck_number, la.arrival_date::text, COALESCE(la.created_by, 0), la.created_at
		FROM log_arrivals la
		WHERE la.organization_id = $1
			AND ($4 = '' OR ('Поступление №' || la.id::text) ILIKE $4 OR la.truck_number ILIKE $4
				OR EXISTS (
					SELECT 1 FROM contractors c
					WHERE c.organization_id = la.organization_id
						AND c.id IN (la.supplier_id, la.carrier_id) AND c.name ILIKE $4
				)
				OR EXISTS (
					SELECT 1 FROM log_items li WHERE li.organization_id = la.organization_id AND li.arrival_id = la.id
						AND (li.species ILIKE $4 OR li.diameter_mm::text ILIKE $4
							OR ('Ø' || li.diameter_mm::text) ILIKE $4)
				))
		ORDER BY la.arrival_date DESC, la.created_at DESC, la.id DESC LIMIT $2 OFFSET $3`, orgID, limit, offset, journalSearchPattern(search))
	if err != nil {
		return nil, err
	}
	arrivals := make([]domain.LogArrival, 0)
	for rows.Next() {
		var arrival domain.LogArrival
		if err := rows.Scan(&arrival.ID, &arrival.SupplierID, &arrival.CarrierID, &arrival.TruckNumber, &arrival.ArrivalDate, &arrival.CreatedBy, &arrival.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		arrivals = append(arrivals, arrival)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for i := range arrivals {
		items, err := r.db.Query(ctx, `
			SELECT id, arrival_id, length_mm, diameter_mm, species, count, volume_m3
			FROM log_items WHERE organization_id = $1 AND arrival_id = $2 ORDER BY id`, orgID, arrivals[i].ID)
		if err != nil {
			return nil, err
		}
		arrivals[i].Items = make([]domain.LogItem, 0)
		for items.Next() {
			var item domain.LogItem
			if err := items.Scan(&item.ID, &item.ArrivalID, &item.LengthMM, &item.DiameterMM, &item.Species, &item.Count, &item.VolumeM3); err != nil {
				items.Close()
				return nil, err
			}
			arrivals[i].Items = append(arrivals[i].Items, item)
		}
		if err := items.Err(); err != nil {
			items.Close()
			return nil, err
		}
		items.Close()
	}
	return arrivals, nil
}

func (r *Repository) CreateLogArrival(ctx context.Context, orgID int64, arrival *domain.LogArrival, items []domain.LogItem) (*domain.LogArrival, error) {
	type logDimensions struct {
		species    string
		lengthMM   int
		diameterMM int
	}
	grouped := make([]domain.LogItem, 0, len(items))
	groupIndex := make(map[logDimensions]int, len(items))
	for _, item := range items {
		item.Species = strings.TrimSpace(item.Species)
		key := logDimensions{species: item.Species, lengthMM: item.LengthMM, diameterMM: item.DiameterMM}
		if i, ok := groupIndex[key]; ok {
			grouped[i].Count += item.Count
			grouped[i].VolumeM3 += item.VolumeM3
			continue
		}
		groupIndex[key] = len(grouped)
		grouped = append(grouped, item)
	}

	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := createLogArrivalTx(ctx, tx, orgID, arrival, grouped); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	arrival.Items = grouped
	return arrival, nil
}

func createLogArrivalTx(ctx context.Context, tx pgx.Tx, orgID int64, arrival *domain.LogArrival, items []domain.LogItem) error {
	arrivalQuery := `
		INSERT INTO log_arrivals (organization_id, supplier_id, carrier_id, truck_number, arrival_date, created_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`
	if err := tx.QueryRow(ctx, arrivalQuery, orgID, arrival.SupplierID, arrival.CarrierID, arrival.TruckNumber, arrival.ArrivalDate, arrival.CreatedBy).Scan(&arrival.ID, &arrival.CreatedAt); err != nil {
		return err
	}

	itemQuery := `INSERT INTO log_items (organization_id, arrival_id, length_mm, diameter_mm, species, count, volume_m3) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
	for i := range items {
		item := &items[i]
		item.ArrivalID = arrival.ID
		item.VolumeM3 = round3(item.VolumeM3)
		if err := tx.QueryRow(ctx, itemQuery, orgID, arrival.ID, item.LengthMM, item.DiameterMM, item.Species, item.Count, item.VolumeM3).Scan(&item.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) CreateSawingOperation(ctx context.Context, orgID int64, op *domain.SawingOperation, sawedLogs []domain.SawedLog, producedBoards []domain.Board) (*domain.SawingOperation, error) {
	var inputVolume, outputVolume float64
	for _, log := range sawedLogs {
		inputVolume += log.VolumeM3
	}
	for _, board := range producedBoards {
		outputVolume += board.VolumeM3
	}
	if err := validateSawingYield(inputVolume, outputVolume); err != nil {
		return nil, err
	}

	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := createSawingOperationTx(ctx, tx, orgID, op, sawedLogs, producedBoards); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	op.Logs = sawedLogs
	op.Boards = producedBoards
	return op, nil
}

func createSawingOperationTx(ctx context.Context, tx pgx.Tx, orgID int64, op *domain.SawingOperation, sawedLogs []domain.SawedLog, producedBoards []domain.Board) error {
	sawingQuery := `INSERT INTO sawing_operations (organization_id, worker_id, date) VALUES ($1, $2, $3) RETURNING id, created_at`
	if err := tx.QueryRow(ctx, sawingQuery, orgID, op.WorkerID, op.Date).Scan(&op.ID, &op.CreatedAt); err != nil {
		return err
	}

	logQuery := `INSERT INTO sawed_logs (organization_id, sawing_id, log_id, volume_m3, consumed_count, consumed_volume_m3)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	logIDs := make([]int64, 0, len(sawedLogs))
	for _, item := range sawedLogs {
		if item.LogID == nil {
			return ErrLogNotFound
		}
		logIDs = append(logIDs, *item.LogID)
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM log_items WHERE organization_id = $1 AND id = ANY($2) ORDER BY id FOR UPDATE`, orgID, logIDs); err != nil {
		return err
	}
	for i := range sawedLogs {
		logID := *sawedLogs[i].LogID

		var remVol float64
		var remCount int
		err := tx.QueryRow(ctx, `SELECT remaining_volume_m3, remaining_count FROM log_items WHERE organization_id = $1 AND id = $2`, orgID, logID).Scan(&remVol, &remCount)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrLogNotFound
			}
			return err
		}
		newVol, newCount, err := consumeLog(remVol, remCount, sawedLogs[i].VolumeM3)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE log_items SET remaining_volume_m3 = $2, remaining_count = $3 WHERE organization_id = $1 AND id = $4`, orgID, newVol, newCount, logID); err != nil {
			return err
		}

		sawedLogs[i].SawingID = op.ID
		err = tx.QueryRow(ctx, logQuery, orgID, op.ID, logID, sawedLogs[i].VolumeM3, remCount-newCount, round3(remVol-newVol)).Scan(&sawedLogs[i].ID)
		if err != nil {
			return err
		}
	}

	boardQuery := `INSERT INTO boards (organization_id, height_mm, width_mm, length_mm, species, grade, count, volume_m3, price_per_m3, sawing_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`
	for i := range producedBoards {
		err := tx.QueryRow(ctx, boardQuery, orgID, producedBoards[i].HeightMM, producedBoards[i].WidthMM, producedBoards[i].LengthMM, producedBoards[i].Species, producedBoards[i].Grade, producedBoards[i].Count, producedBoards[i].VolumeM3, producedBoards[i].PricePerM3, op.ID).Scan(&producedBoards[i].ID)
		if err != nil {
			return err
		}
	}
	return nil
}

const sawingVolumeTolerance = 0.0005 // Допуск на округление объёма до трёх знаков.

var (
	ErrSawingYieldExceedsInput = errors.New("объём досок превышает объём распиленного сырья")
	ErrInsufficientStock       = errors.New("недостаточно товара на складе")
	ErrBoardNotFound           = errors.New("доска не найдена на складе")
	ErrBoardEmpty              = errors.New("этой позиции нет на складе")
	ErrOrderNotFound           = errors.New("заказ не найден")
	ErrOrderClosed             = errors.New("заказ уже завершен или отменен")
	ErrOrderHasPayments        = errors.New("Сначала удалите принятые оплаты по заказу")
	ErrInvalidOrderStatus      = errors.New("допустимые статусы: «В работе» и «Отменён»")
	ErrOrderEmpty              = errors.New("невозможно завершить пустой заказ")
)

func validateSawingYield(inputVolume, outputVolume float64) error {
	if outputVolume > inputVolume+sawingVolumeTolerance {
		return ErrSawingYieldExceedsInput
	}
	return nil
}

// GetSales returns sales history with their line items.
func (r *Repository) GetSales(ctx context.Context, orgID int64, limit, offset int, search ...string) ([]domain.Sale, error) {
	rows, err := r.db.Query(ctx, `
		SELECT s.id, s.contractor_id, s.truck_number, s.delivery_price, s.extra_price, s.total_volume,
		       s.total_price, COALESCE(s.created_by, 0), s.created_at, s.order_id,
		       (s.order_id IS NOT NULL OR s.legacy_order_sale), COALESCE(o.order_number, 0)
		FROM sales s
		LEFT JOIN orders o ON o.organization_id = s.organization_id AND o.id = s.order_id
		WHERE s.organization_id = $1
			AND ($4 = '' OR ('Продажа №' || s.id::text) ILIKE $4 OR s.truck_number ILIKE $4
				OR ('Заказ №' || o.order_number::text) ILIKE $4
				OR EXISTS (
					SELECT 1 FROM contractors c
					WHERE c.organization_id = s.organization_id AND c.id = s.contractor_id AND c.name ILIKE $4
				)
				OR EXISTS (
					SELECT 1 FROM sale_boards sb JOIN boards b
						ON b.organization_id = sb.organization_id AND b.id = sb.board_id
					WHERE sb.organization_id = s.organization_id AND sb.sale_id = s.id
						AND (b.species ILIKE $4 OR `+journalBoardDimensions+`)
				))
		ORDER BY s.created_at DESC, s.id DESC LIMIT $2 OFFSET $3`, orgID, limit, offset, journalSearchPattern(search))
	if err != nil {
		return nil, err
	}
	sales := make([]domain.Sale, 0)
	for rows.Next() {
		var sale domain.Sale
		if err := rows.Scan(&sale.ID, &sale.ContractorID, &sale.TruckNumber, &sale.DeliveryPrice, &sale.ExtraPrice, &sale.TotalVolume, &sale.TotalPrice, &sale.CreatedBy, &sale.CreatedAt, &sale.OrderID, &sale.IsOrderSale, &sale.OrderNumber); err != nil {
			rows.Close()
			return nil, err
		}
		sales = append(sales, sale)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range sales {
		items, err := r.db.Query(ctx, `
			SELECT sb.id, sb.sale_id, sb.board_id, sb.count, sb.volume_m3, sb.price,
				b.species, b.grade, b.height_mm, b.width_mm, b.length_mm
			FROM sale_boards sb JOIN boards b ON b.organization_id = sb.organization_id AND b.id = sb.board_id
			WHERE sb.organization_id = $1 AND sb.sale_id = $2 ORDER BY sb.id`, orgID, sales[i].ID)
		if err != nil {
			return nil, err
		}
		sales[i].Items = make([]domain.SaleItem, 0)
		for items.Next() {
			var item domain.SaleItem
			if err := items.Scan(&item.ID, &item.SaleID, &item.BoardID, &item.Count, &item.VolumeM3, &item.Price,
				&item.Species, &item.Grade, &item.HeightMM, &item.WidthMM, &item.LengthMM); err != nil {
				items.Close()
				return nil, err
			}
			sales[i].Items = append(sales[i].Items, item)
		}
		if err := items.Err(); err != nil {
			items.Close()
			return nil, err
		}
		items.Close()
	}
	return sales, nil
}

func (r *Repository) CreateSale(ctx context.Context, orgID int64, sale *domain.Sale, items []domain.SaleItem) (*domain.Sale, error) {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := r.createSaleTx(ctx, tx, orgID, sale, items); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return sale, nil
}

// createSaleTx проводит продажу внутри уже открытой транзакции (без Commit),
// чтобы её можно было объединить с другими действиями, например с завершением заказа.
func (r *Repository) createSaleTx(ctx context.Context, tx pgx.Tx, orgID int64, sale *domain.Sale, items []domain.SaleItem) error {
	saleQuery := `
		INSERT INTO sales (organization_id, contractor_id, truck_number, delivery_price, extra_price, total_volume, total_price, created_by, order_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`

	err := tx.QueryRow(ctx, saleQuery, orgID, sale.ContractorID, sale.TruckNumber, sale.DeliveryPrice, sale.ExtraPrice, sale.TotalVolume, sale.TotalPrice, sale.CreatedBy, sale.OrderID).Scan(&sale.ID, &sale.CreatedAt)
	if err != nil {
		return err
	}

	itemQuery := `
		INSERT INTO sale_boards (organization_id, sale_id, board_id, count, volume_m3, price)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	boardIDs := make([]int64, 0, len(items))
	for _, item := range items {
		boardIDs = append(boardIDs, item.BoardID)
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM boards WHERE organization_id = $1 AND id = ANY($2) ORDER BY id FOR UPDATE`, orgID, boardIDs); err != nil {
		return err
	}
	var totalVolume, totalPrice float64
	for i := range items {
		var currentCount int
		var currentVolume float64
		err := tx.QueryRow(ctx, `SELECT count, volume_m3 FROM boards WHERE organization_id = $1 AND id = $2`, orgID, items[i].BoardID).Scan(&currentCount, &currentVolume)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrBoardNotFound
			}
			return err
		}

		if currentCount < items[i].Count {
			return ErrInsufficientStock
		}

		newCount := currentCount - items[i].Count
		// Объём считаем по складу, а не доверяем клиенту. Цена у позиции = за 1 м³.
		items[i].VolumeM3 = domain.ItemVolume(currentCount, currentVolume, items[i].Count)
		totalVolume += items[i].VolumeM3
		totalPrice += domain.LineTotal(items[i].Price, items[i].VolumeM3)
		newVolume := currentVolume - items[i].VolumeM3
		// Объём приходит от клиента и может не сойтись с остатком: в минус не уходим,
		// а при нулевом остатке по штукам объём тоже ноль.
		if newCount == 0 || newVolume < 0 {
			newVolume = 0
		}

		// 2. Строку НЕ удаляем: на неё ссылаются sale_boards и order_boards (внешние ключи),
		// из-за DELETE продажа всей позиции падала. Нулевые позиции скрывает фильтр
		// count > 0 в GetAllBoards.
		_, err = tx.Exec(ctx, `UPDATE boards SET count = $2, volume_m3 = $3 WHERE organization_id = $1 AND id = $4`, orgID, newCount, newVolume, items[i].BoardID)
		if err != nil {
			return err
		}

		// 3. Записываем позицию чека
		items[i].SaleID = sale.ID
		err = tx.QueryRow(ctx, itemQuery, orgID, sale.ID, items[i].BoardID, items[i].Count, items[i].VolumeM3, items[i].Price).Scan(&items[i].ID)
		if err != nil {
			return err
		}
	}

	sale.TotalVolume = totalVolume
	sale.TotalPrice = domain.Round2(totalPrice + sale.DeliveryPrice + sale.ExtraPrice)
	if _, err := tx.Exec(ctx, `UPDATE sales SET total_volume = $2, total_price = $3 WHERE organization_id = $1 AND id = $4`, orgID, sale.TotalVolume, sale.TotalPrice, sale.ID); err != nil {
		return err
	}

	sale.Items = items
	return nil
}

// CompleteOrder в ОДНОЙ транзакции: блокирует заказ, проводит продажу по его позициям
// и ставит статус completed. Двойной клик или параллельный запрос второй раз не спишут склад,
// а при любой ошибке не остаётся ни продажи, ни смены статуса.
func (r *Repository) CompleteOrder(ctx context.Context, orgID int64, orderID, userID int64) (*domain.Order, error) {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	order, err := r.completeOrderTx(ctx, tx, orgID, orderID, userID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return order, nil
}

func (r *Repository) completeOrderTx(ctx context.Context, tx pgx.Tx, orgID, orderID, userID int64) (*domain.Order, error) {
	var o domain.Order
	err := tx.QueryRow(ctx, `SELECT id, order_number, COALESCE(master_id,0), contractor_id, COALESCE(notes,''), delivery_price, extra_price, status, created_at, updated_at FROM orders WHERE organization_id = $1 AND id = $2 FOR UPDATE`, orgID, orderID).
		Scan(&o.ID, &o.OrderNumber, &o.MasterID, &o.ContractorID, &o.Notes, &o.DeliveryPrice, &o.ExtraPrice, &o.Status, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	if o.Status == domain.OrderStatusCompleted || o.Status == domain.OrderStatusCanceled {
		return nil, ErrOrderClosed
	}

	rows, err := tx.Query(ctx, `SELECT board_id, count, volume_m3, price FROM order_boards WHERE organization_id = $1 AND order_id = $2 ORDER BY id`, orgID, orderID)
	if err != nil {
		return nil, err
	}
	var saleItems []domain.SaleItem
	for rows.Next() {
		var it domain.SaleItem
		if err := rows.Scan(&it.BoardID, &it.Count, &it.VolumeM3, &it.Price); err != nil {
			rows.Close()
			return nil, err
		}
		saleItems = append(saleItems, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err = lockFirewood(ctx, tx, orgID); err != nil {
		return nil, err
	}
	fwItems, err := firewoodOrderItems(ctx, tx, orgID, orderID)
	if err != nil {
		return nil, err
	}
	if err = checkFirewoodOrderStock(ctx, tx, orgID, fwItems); err != nil {
		return nil, err
	}
	if len(saleItems) == 0 && len(fwItems) == 0 {
		return nil, ErrOrderEmpty
	}

	sale := &domain.Sale{
		OrderID:       &orderID,
		ContractorID:  o.ContractorID,
		TruckNumber:   "Доставка по заявке",
		DeliveryPrice: o.DeliveryPrice,
		ExtraPrice:    o.ExtraPrice,
		CreatedBy:     userID,
	}
	if err := r.createSaleTx(ctx, tx, orgID, sale, saleItems); err != nil {
		return nil, err
	}

	var fwVolume, fwPrice float64
	for _, p := range fwItems {
		fwVolume += p.VolumeM3
		fwPrice += domain.Round2(p.VolumeM3 * p.Price)
	}
	if _, err = tx.Exec(ctx, `UPDATE sales SET total_volume=total_volume+$2,total_price=total_price+$3 WHERE organization_id = $1 AND id=$4`, orgID, fwVolume, fwPrice, sale.ID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET status = $2, updated_at = NOW() WHERE organization_id = $1 AND id = $3`, orgID, domain.OrderStatusCompleted, orderID); err != nil {
		return nil, err
	}
	o.Status = domain.OrderStatusCompleted
	return &o, nil
}

type boardStock struct {
	count  int
	volume float64
}

func requestedOrderCounts(items []domain.OrderItem) map[int64]int {
	requested := make(map[int64]int, len(items))
	for _, item := range items {
		requested[item.BoardID] += item.Count
	}
	return requested
}

func validateOrderStock(requested map[int64]int, stocks map[int64]boardStock) error {
	for boardID, count := range requested {
		stock, ok := stocks[boardID]
		if !ok {
			return ErrBoardNotFound
		}
		if stock.count <= 0 {
			return ErrBoardEmpty
		}
		if count > stock.count {
			return ErrInsufficientStock
		}
	}
	return nil
}

func (r *Repository) CreateOrder(ctx context.Context, orgID int64, order *domain.Order, items []domain.OrderItem) (*domain.Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `UPDATE organizations SET last_order_number = last_order_number + 1 WHERE id = $1 AND is_active = TRUE RETURNING last_order_number`, orgID).Scan(&order.OrderNumber); err != nil {
		return nil, organizationError(err)
	}
	if len(order.FirewoodItems) > 0 {
		if err := lockFirewood(ctx, tx, orgID); err != nil {
			return nil, err
		}
	}

	if err := createOrderTx(ctx, tx, orgID, order, items); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	order.Items = items
	return order, nil
}

func createOrderTx(ctx context.Context, tx pgx.Tx, orgID int64, order *domain.Order, items []domain.OrderItem) error {
	orderQuery := `INSERT INTO orders (organization_id, order_number, master_id, contractor_id, notes, delivery_price, extra_price, status) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id, created_at, updated_at`
	if err := tx.QueryRow(ctx, orderQuery, orgID, order.OrderNumber, order.MasterID, order.ContractorID, order.Notes, order.DeliveryPrice, order.ExtraPrice, order.Status).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt); err != nil {
		return err
	}

	itemQuery := `INSERT INTO order_boards (organization_id, order_id, board_id, count, volume_m3, price) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	requestedByBoard := requestedOrderCounts(items)
	boardStocks := make(map[int64]boardStock, len(requestedByBoard))
	for boardID := range requestedByBoard {
		var stock boardStock
		err := tx.QueryRow(ctx, `SELECT count, volume_m3 FROM boards WHERE organization_id = $1 AND id = $2`, orgID, boardID).Scan(&stock.count, &stock.volume)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrBoardNotFound
			}
			return err
		}
		boardStocks[boardID] = stock
	}
	if err := validateOrderStock(requestedByBoard, boardStocks); err != nil {
		return err
	}

	for i := range items {
		// Объём считаем по складу, а не доверяем клиенту.
		stock := boardStocks[items[i].BoardID]
		items[i].VolumeM3 = domain.ItemVolume(stock.count, stock.volume, items[i].Count)

		items[i].OrderID = order.ID
		if err := tx.QueryRow(ctx, itemQuery, orgID, order.ID, items[i].BoardID, items[i].Count, items[i].VolumeM3, items[i].Price).Scan(&items[i].ID); err != nil {
			return err
		}
	}
	if err := checkFirewoodOrderStock(ctx, tx, orgID, order.FirewoodItems); err != nil {
		return err
	}
	for i := range order.FirewoodItems {
		p := &order.FirewoodItems[i]
		p.Price = domain.Round2(p.Price)
		if _, err := tx.Exec(ctx, `INSERT INTO order_firewood(organization_id, order_id,kind,volume_m3,price) VALUES($1, $2,$3,$4,$5)`, orgID, order.ID, p.Kind, p.VolumeM3, p.Price); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) GetOrders(ctx context.Context, orgID int64, limit, offset int) ([]domain.Order, error) {
	query := `SELECT o.id, o.order_number, COALESCE(o.master_id,0), o.contractor_id, COALESCE(o.notes,''), o.delivery_price, o.extra_price, o.status, o.created_at, o.updated_at,
		` + orderTotalExpr + `, ` + orderPaidExpr + `
		FROM orders o WHERE o.organization_id = $1 ORDER BY o.id DESC LIMIT $2 OFFSET $3`
	rows, err := r.db.Query(ctx, query, orgID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := make([]domain.Order, 0)
	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(&o.ID, &o.OrderNumber, &o.MasterID, &o.ContractorID, &o.Notes, &o.DeliveryPrice, &o.ExtraPrice, &o.Status, &o.CreatedAt, &o.UpdatedAt, &o.TotalPrice, &o.PaidAmount); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range orders {
		items, err := r.GetOrderItems(ctx, orgID, orders[i].ID)
		if err != nil {
			return nil, err
		}
		orders[i].Items = items
		fr, err := r.db.Query(ctx, `SELECT kind,volume_m3,price FROM order_firewood WHERE organization_id = $1 AND order_id=$2 ORDER BY id`, orgID, orders[i].ID)
		if err != nil {
			return nil, err
		}
		for fr.Next() {
			var p domain.FirewoodOrderItem
			if err = fr.Scan(&p.Kind, &p.VolumeM3, &p.Price); err != nil {
				fr.Close()
				return nil, err
			}
			orders[i].FirewoodItems = append(orders[i].FirewoodItems, p)
		}
		err = fr.Err()
		fr.Close()
		if err != nil {
			return nil, err
		}
	}
	return orders, nil
}

// GetSawingOperations returns recent production operations and their input/output records.
func (r *Repository) GetSawingOperations(ctx context.Context, orgID int64, limit, offset int, search ...string) ([]domain.SawingOperation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT so.id, COALESCE(so.worker_id, 0), to_char(so.date, 'YYYY-MM-DD'), so.created_at,
			COALESCE(NULLIF(u.display_name, ''), u.username, '')
		FROM sawing_operations so LEFT JOIN users u ON u.id = so.worker_id
		WHERE so.organization_id = $1
			AND ($4 = '' OR ('Распил №' || so.id::text) ILIKE $4 OR to_char(so.date, 'YYYY-MM-DD') ILIKE $4
				OR to_char(so.date, 'DD.MM.YYYY') ILIKE $4
				OR u.display_name ILIKE $4 OR u.username ILIKE $4 OR u.tg_username ILIKE $4
				OR EXISTS (
					SELECT 1 FROM boards b WHERE b.organization_id = so.organization_id AND b.sawing_id = so.id
						AND (b.species ILIKE $4 OR `+journalBoardDimensions+`)
				))
		ORDER BY so.date DESC, so.id DESC LIMIT $2 OFFSET $3`, orgID, limit, offset, journalSearchPattern(search))
	if err != nil {
		return nil, err
	}
	ops := make([]domain.SawingOperation, 0)
	for rows.Next() {
		var op domain.SawingOperation
		if err := rows.Scan(&op.ID, &op.WorkerID, &op.Date, &op.CreatedAt, &op.WorkerName); err != nil {
			rows.Close()
			return nil, err
		}
		ops = append(ops, op)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range ops {
		logs, err := r.db.Query(ctx, `SELECT id, sawing_id, log_id, volume_m3 FROM sawed_logs WHERE organization_id = $1 AND sawing_id = $2 ORDER BY id`, orgID, ops[i].ID)
		if err != nil {
			return nil, err
		}
		ops[i].Logs = make([]domain.SawedLog, 0)
		for logs.Next() {
			var item domain.SawedLog
			if err := logs.Scan(&item.ID, &item.SawingID, &item.LogID, &item.VolumeM3); err != nil {
				logs.Close()
				return nil, err
			}
			ops[i].Logs = append(ops[i].Logs, item)
		}
		if err := logs.Err(); err != nil {
			logs.Close()
			return nil, err
		}
		logs.Close()
		boards, err := r.db.Query(ctx, `SELECT id, height_mm, width_mm, length_mm, species, grade, count, volume_m3, price_per_m3 FROM boards WHERE organization_id = $1 AND sawing_id = $2 ORDER BY id`, orgID, ops[i].ID)
		if err != nil {
			return nil, err
		}
		ops[i].Boards = make([]domain.Board, 0)
		for boards.Next() {
			var board domain.Board
			var price *float64
			if err := boards.Scan(&board.ID, &board.HeightMM, &board.WidthMM, &board.LengthMM, &board.Species, &board.Grade, &board.Count, &board.VolumeM3, &price); err != nil {
				boards.Close()
				return nil, err
			}
			board.PricePerM3 = price
			ops[i].Boards = append(ops[i].Boards, board)
		}
		if err := boards.Err(); err != nil {
			boards.Close()
			return nil, err
		}
		boards.Close()
	}
	return ops, nil
}

func (r *Repository) GetAllBoards(ctx context.Context, orgID int64, filter domain.BoardFilter, limit, offset int) ([]domain.Board, error) {
	query := `SELECT id, height_mm, width_mm, length_mm, species, grade, count, volume_m3, price_per_m3 FROM boards WHERE organization_id = $1 AND count > 0`

	args := []interface{}{orgID}
	argID := 2

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

func (r *Repository) GetAllLogs(ctx context.Context, orgID int64, filter domain.LogFilter, limit, offset int) ([]domain.LogItem, error) {
	query := `SELECT id, arrival_id, length_mm, diameter_mm, species, remaining_count, remaining_volume_m3 FROM log_items WHERE organization_id = $1 AND remaining_volume_m3 > 0`

	args := []interface{}{orgID}
	argID := 2

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

func (r *Repository) GetStatisticsSummary(ctx context.Context, orgID int64, startDate, endDate string, comparePrevious ...bool) (*domain.StatisticsSummary, error) {
	stats := &domain.StatisticsSummary{}
	var previous domain.StatisticsComparison
	withPrevious := len(comparePrevious) > 0 && comparePrevious[0]

	// boards хранит текущий остаток. Восстанавливаем выпуск по продажам
	// и всем корректировкам партий, напиленных в выбранном периоде.
	query := `WITH bounds AS (
		SELECT $2::date AS start_date, $3::date AS end_date
	), periods AS (
		SELECT FALSE AS is_previous, start_date, end_date FROM bounds
		UNION ALL
		SELECT TRUE, start_date - (end_date - start_date + 1), start_date - 1 FROM bounds WHERE $4::boolean
	), totals AS (
		SELECT p.is_previous, arrivals.volume AS logs_arrived, production.sawed, production.produced,
			sales.volume AS sales_volume, sales.amount AS sales_amount
		FROM periods p
		CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(li.volume_m3), 0) AS volume
			FROM log_arrivals la JOIN log_items li ON li.organization_id = la.organization_id AND li.arrival_id = la.id
			WHERE la.organization_id = $1 AND la.arrival_date >= p.start_date AND la.arrival_date <= p.end_date
		) arrivals
		CROSS JOIN LATERAL (
			WITH operations AS (
				SELECT id FROM sawing_operations WHERE organization_id = $1 AND date >= p.start_date AND date <= p.end_date
			), produced AS (
				SELECT b.id, b.volume_m3 FROM boards b JOIN operations so ON b.sawing_id = so.id WHERE b.organization_id = $1
			)
			SELECT (SELECT COALESCE(SUM(sl.volume_m3), 0) FROM sawed_logs sl JOIN operations so ON sl.sawing_id = so.id WHERE sl.organization_id = $1) AS sawed,
				CASE WHEN NOT p.is_previous THEN
					(SELECT COALESCE(SUM(volume_m3), 0) FROM produced)
					+ (SELECT COALESCE(SUM(sb.volume_m3), 0) FROM sale_boards sb JOIN produced b ON b.id = sb.board_id WHERE sb.organization_id = $1)
					- (SELECT COALESCE(SUM(a.delta_volume), 0) FROM stock_adjustments a JOIN produced b ON b.id = a.item_id WHERE a.organization_id = $1 AND a.kind = 'board')
				END AS produced
		) production
		CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(s.total_volume), 0) AS volume, COALESCE(SUM(s.total_price), 0) AS amount
			FROM sales s WHERE s.organization_id = $1 AND s.created_at >= p.start_date AND s.created_at < p.end_date + 1
		) sales
	), firewood_arrived AS (
		SELECT COALESCE(SUM(volume_m3), 0) AS volume FROM firewood_arrivals
		WHERE organization_id = $1 AND arrival_date >= $2 AND arrival_date <= $3
	), firewood_produced AS (
		SELECT COALESCE(SUM(volume_m3) FILTER (WHERE kind = 'split'), 0) AS split,
			COALESCE(SUM(volume_m3) FILTER (WHERE kind = 'round'), 0) AS round,
			COALESCE(SUM(raw_m3), 0) AS raw
		FROM firewood_processing WHERE organization_id = $1 AND date >= $2 AND date <= $3
	)
	SELECT t.logs_arrived, t.sawed, t.produced, t.sales_volume, t.sales_amount,
		a.volume, f.split, f.round, f.raw,
		COALESCE(prev.logs_arrived, 0), COALESCE(prev.sawed, 0), COALESCE(prev.sales_volume, 0), COALESCE(prev.sales_amount, 0)
	FROM totals t LEFT JOIN totals prev ON prev.is_previous
	CROSS JOIN firewood_arrived a CROSS JOIN firewood_produced f
	WHERE NOT t.is_previous`
	err := r.db.QueryRow(ctx, query, orgID, startDate, endDate, withPrevious).Scan(
		&stats.TotalLogsArrivedVolume, &stats.TotalLogsSawedVolume, &stats.TotalBoardsProducedVolume,
		&stats.TotalSalesVolume, &stats.TotalSalesAmount,
		&stats.FirewoodArrivedVolume, &stats.FirewoodSplitProducedVolume, &stats.FirewoodRoundProducedVolume, &stats.FirewoodRawConsumedVolume,
		&previous.TotalLogsArrivedVolume, &previous.TotalLogsSawedVolume, &previous.TotalSalesVolume, &previous.TotalSalesAmount)
	if err != nil {
		return nil, err
	}

	stock, err := readFirewoodStock(ctx, orgID, r.db)
	if err != nil {
		return nil, err
	}
	stats.FirewoodStock = *stock
	if withPrevious {
		stats.Previous = &previous
	}
	return stats, nil
}

func (r *Repository) GetOrderByID(ctx context.Context, orgID int64, id int64) (*domain.Order, error) {
	query := `SELECT id, order_number, COALESCE(master_id,0), contractor_id, COALESCE(notes,''), delivery_price, extra_price, status, created_at, updated_at FROM orders WHERE organization_id = $1 AND id = $2`
	var o domain.Order
	err := r.db.QueryRow(ctx, query, orgID, id).Scan(&o.ID, &o.OrderNumber, &o.MasterID, &o.ContractorID, &o.Notes, &o.DeliveryPrice, &o.ExtraPrice, &o.Status, &o.CreatedAt, &o.UpdatedAt)
	return &o, err
}

func (r *Repository) GetOrderItems(ctx context.Context, orgID int64, orderID int64) ([]domain.OrderItem, error) {
	query := `SELECT ob.id, ob.order_id, ob.board_id, ob.count, ob.volume_m3, ob.price, COALESCE(b.species, ''), COALESCE(b.grade, ''), COALESCE(b.height_mm, 0), COALESCE(b.width_mm, 0), COALESCE(b.length_mm, 0) FROM order_boards ob LEFT JOIN boards b ON b.id = ob.board_id AND b.organization_id = ob.organization_id WHERE ob.organization_id = $1 AND ob.order_id = $2 ORDER BY ob.id`
	rows, err := r.db.Query(ctx, query, orgID, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.OrderItem, 0)
	for rows.Next() {
		var i domain.OrderItem
		if err := rows.Scan(&i.ID, &i.OrderID, &i.BoardID, &i.Count, &i.VolumeM3, &i.Price, &i.Species, &i.Grade, &i.HeightMM, &i.WidthMM, &i.LengthMM); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) UpdateOrderStatus(ctx context.Context, orgID int64, id int64, status domain.OrderStatus) error {
	if status != domain.OrderStatusInProgress && status != domain.OrderStatusCanceled {
		return ErrInvalidOrderStatus
	}
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// AddPayment и CompleteOrder блокируют ту же строку заказа.
	var current domain.OrderStatus
	if err := tx.QueryRow(ctx, `SELECT status FROM orders WHERE organization_id = $1 AND id = $2 FOR UPDATE`, orgID, id).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrderNotFound
		}
		return err
	}
	if current != domain.OrderStatusNew && current != domain.OrderStatusInProgress {
		return ErrOrderClosed
	}
	if status == domain.OrderStatusCanceled {
		var paid float64
		if err := tx.QueryRow(ctx, `SELECT `+orderPaidExpr+` FROM orders o WHERE o.organization_id = $1 AND o.id = $2`, orgID, id).Scan(&paid); err != nil {
			return err
		}
		if paid > 0 {
			return ErrOrderHasPayments
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET status = $2, updated_at = NOW() WHERE organization_id = $1 AND id = $3`, orgID, status, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (*domain.User, error) {
	query := `SELECT id, username, password_hash, role, created_at, COALESCE(display_name, ''), COALESCE(tg_username, '') FROM users WHERE id = $1`
	var user domain.User
	err := r.db.QueryRow(ctx, query, id).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.DisplayName, &user.TGUsername)
	return &user, err
}

// ListUsers возвращает сотрудников организации с их ролями.
func (r *Repository) ListUsers(ctx context.Context, orgID int64) ([]domain.User, error) {
	rows, err := r.db.Query(ctx, `SELECT u.id, u.username, u.password_hash, om.role, u.created_at, u.display_name, u.tg_username
		FROM users u JOIN organization_members om ON om.user_id = u.id
		WHERE om.organization_id = $1 ORDER BY u.id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]domain.User, 0)
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.DisplayName, &u.TGUsername); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *Repository) UpdateUserRole(ctx context.Context, orgID, id int64, role domain.Role, allowManager bool) (bool, error) {
	tx, err := r.beginOrgWrite(ctx, orgID)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `UPDATE organization_members SET role = $3::role
		WHERE organization_id = $1 AND user_id = $2
		AND ($4 OR role <> 'manager' AND $3::role <> 'manager')`, orgID, id, role, allowManager)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, tx.Commit(ctx)
}

func (r *Repository) DeleteUser(ctx context.Context, orgID, id int64) error {
	return r.deleteOrganizationMember(ctx, orgID, id, false)
}

func (r *Repository) DeleteOrganizationMember(ctx context.Context, orgID, id int64) error {
	return r.deleteOrganizationMember(ctx, orgID, id, true)
}

func (r *Repository) deleteOrganizationMember(ctx context.Context, orgID, id int64, platformAdmin bool) error {
	tx, err := r.beginOrganizationWrite(ctx, orgID, !platformAdmin)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var username string
	if err := tx.QueryRow(ctx, `SELECT username FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&username); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM organization_members WHERE organization_id = $1 AND user_id = $2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	if platformAdmin && (domain.User{ID: id, Username: username}).IsSuperAdmin() {
		return ErrWebUserProtected
	}
	_, err = tx.Exec(ctx, `INSERT INTO deleted_telegram_users (organization_id, username)
		SELECT $1, $2::text WHERE $2::text ~ '^tg_[0-9]+$' ON CONFLICT DO NOTHING`, orgID, username)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpdateUserPassword задаёт новый пароль. Telegram-аккаунты (хеш начинается с tg_) не трогает.
func (r *Repository) UpdateUserPassword(ctx context.Context, id int64, passwordHash string) (bool, error) {
	tag, err := r.db.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1 AND password_hash NOT LIKE 'tg\_%'`, id, passwordHash)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// UpdateTelegramProfile сохраняет имя и @username из Telegram (обновляется при каждом входе, чтобы переименования не терялись).
func (r *Repository) UpdateTelegramProfile(ctx context.Context, id int64, displayName, tgUsername string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET display_name = $2, tg_username = $3 WHERE id = $1`, id, displayName, tgUsername)
	return err
}
