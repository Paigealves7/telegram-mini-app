package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"telegram-mini-app/internal/domain"

	"github.com/jackc/pgx/v5"
)

var (
	ErrOperationInUse = errors.New("запись связана с другими операциями")
	ErrOrderSaleLink  = errors.New("для завершённого заказа не удалось однозначно определить связанную продажу")
)

var ErrResetNotDemo = errors.New("сброс тестовых данных и нумерации доступен только для демонстрационных организаций")

func (r *Repository) ResetTestData(ctx context.Context, orgID int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		org, err := r.lockOrgExclusive(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if err := clearDemoOperations(ctx, tx, org); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE organizations SET last_order_number = 0 WHERE id = $1`, orgID)
		return err
	})
}

func clearDemoOperations(ctx context.Context, tx pgx.Tx, org *domain.Organization) error {
	if !org.IsDemo {
		return ErrResetNotDemo
	}
	if err := lockFirewood(ctx, tx, org.ID); err != nil {
		return err
	}
	tables := []string{
		"order_payments", "order_boards", "order_firewood", "sale_boards", "sales", "orders",
		"sawed_logs", "boards", "sawing_operations", "log_items", "log_arrivals",
		"firewood_processing", "firewood_arrivals", "stock_adjustments", "firewood_stock_adjustments",
	}
	for _, table := range tables {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE organization_id = $1", org.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) SeedDemoData(ctx context.Context, orgID, actorID int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		org, err := r.lockOrgExclusive(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if err := clearDemoOperations(ctx, tx, org); err != nil {
			return err
		}

		supplier := domain.Contractor{Name: "ООО «СеверЛесЗаготовка»", IsSupplier: true}
		carrier := domain.Contractor{Name: "ИП Морозов А.В. (Лесовозы)", IsCarrier: true}
		builder := domain.Contractor{Name: "СК «УютСтрой»", IsClient: true}
		store := domain.Contractor{Name: "База стройматериалов «Кедр»", IsClient: true}
		for _, contractor := range []*domain.Contractor{&supplier, &carrier, &builder, &store} {
			err := tx.QueryRow(ctx, `INSERT INTO contractors (organization_id, name, is_supplier, is_carrier, is_client)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (organization_id, name) DO UPDATE SET
					is_supplier = contractors.is_supplier OR EXCLUDED.is_supplier,
					is_carrier = contractors.is_carrier OR EXCLUDED.is_carrier,
					is_client = contractors.is_client OR EXCLUDED.is_client,
					is_active = TRUE
				RETURNING id`, orgID, contractor.Name, contractor.IsSupplier, contractor.IsCarrier, contractor.IsClient).Scan(&contractor.ID)
			if err != nil {
				return err
			}
		}

		boards, err := seedDemoProduction(ctx, tx, orgID, actorID, supplier.ID, carrier.ID)
		if err != nil {
			return err
		}
		first := domain.Order{OrderNumber: 1, MasterID: actorID, ContractorID: builder.ID, DeliveryPrice: 2500,
			Status: domain.OrderStatusInProgress, FirewoodItems: []domain.FirewoodOrderItem{{Kind: "split", VolumeM3: 4, Price: 3200}}}
		if err := createOrderTx(ctx, tx, orgID, &first, []domain.OrderItem{{BoardID: boards[0].ID, Count: 20, Price: *boards[0].PricePerM3}}); err != nil {
			return err
		}
		if _, err := r.completeOrderTx(ctx, tx, orgID, first.ID, actorID); err != nil {
			return err
		}
		if _, err := addPaymentTx(ctx, tx, orgID, first.ID, 15000, "Аванс по безналу", actorID); err != nil {
			return err
		}

		second := domain.Order{OrderNumber: 2, MasterID: actorID, ContractorID: store.ID, Status: domain.OrderStatusInProgress}
		if err := createOrderTx(ctx, tx, orgID, &second, []domain.OrderItem{
			{BoardID: boards[1].ID, Count: 30, Price: *boards[1].PricePerM3},
			{BoardID: boards[2].ID, Count: 5, Price: *boards[2].PricePerM3},
		}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE organizations SET last_order_number = $2 WHERE id = $1`, orgID, second.OrderNumber)
		return err
	})
}

func seedDemoProduction(ctx context.Context, tx pgx.Tx, orgID, actorID, supplierID, carrierID int64) ([]domain.Board, error) {
	now := time.Now()
	today, yesterday := now.Format(time.DateOnly), now.AddDate(0, 0, -1).Format(time.DateOnly)
	pine := domain.LogArrival{SupplierID: supplierID, CarrierID: carrierID, TruckNumber: "А450КХ196", ArrivalDate: yesterday, CreatedBy: actorID}
	pineLogs := []domain.LogItem{
		{LengthMM: 6000, DiameterMM: 220, Species: "Сосна", Count: 20, VolumeM3: 5.600},
		{LengthMM: 6000, DiameterMM: 260, Species: "Сосна", Count: 15, VolumeM3: 5.850},
	}
	if err := createLogArrivalTx(ctx, tx, orgID, &pine, pineLogs); err != nil {
		return nil, err
	}
	spruce := domain.LogArrival{SupplierID: supplierID, CarrierID: carrierID, TruckNumber: "В712МН196", ArrivalDate: today, CreatedBy: actorID}
	if err := createLogArrivalTx(ctx, tx, orgID, &spruce, []domain.LogItem{
		{LengthMM: 6000, DiameterMM: 240, Species: "Ель", Count: 12, VolumeM3: 3.960},
	}); err != nil {
		return nil, err
	}

	if err := ensureFirewoodSettings(ctx, tx, orgID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO firewood_arrivals (organization_id, supplier_id, carrier_id, truck_number, volume_m3, arrival_date, created_by)
		VALUES ($1, $2, $3, 'К805ОР196', 18, $4::date, $5)`, orgID, supplierID, carrierID, yesterday, actorID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO firewood_processing (organization_id, kind, volume_m3, coefficient, raw_m3, performer, date, comment, created_by)
		SELECT s.organization_id, s.kind, p.volume, s.coefficient, p.volume * s.coefficient, 'Бригада переработки', $2::date, 'Берёзовые дрова', $3
		FROM firewood_settings s JOIN (VALUES ('split', 10.000), ('round', 6.000)) AS p(kind, volume) USING (kind)
		WHERE s.organization_id = $1`, orgID, today, actorID); err != nil {
		return nil, err
	}

	widePrice, narrowPrice, beamPrice := 18500.0, 16000.0, 19000.0
	boards := []domain.Board{
		{HeightMM: 50, WidthMM: 150, LengthMM: 6000, Species: "Сосна", Grade: "1", Count: 50, VolumeM3: 2.250, PricePerM3: &widePrice},
		{HeightMM: 25, WidthMM: 100, LengthMM: 6000, Species: "Сосна", Grade: "1", Count: 80, VolumeM3: 1.200, PricePerM3: &narrowPrice},
		{HeightMM: 100, WidthMM: 100, LengthMM: 6000, Species: "Сосна", Grade: "1", Count: 10, VolumeM3: 0.600, PricePerM3: &beamPrice},
	}
	sawing := domain.SawingOperation{WorkerID: actorID, Date: today}
	if err := createSawingOperationTx(ctx, tx, orgID, &sawing, []domain.SawedLog{
		{LogID: &pineLogs[0].ID, VolumeM3: 5.600},
	}, boards); err != nil {
		return nil, err
	}
	return boards, nil
}

func (r *Repository) DeleteOrder(ctx context.Context, orgID int64, id int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockOrgShare(ctx, tx, orgID); err != nil {
			return err
		}
		var status domain.OrderStatus
		if err := tx.QueryRow(ctx, `SELECT status FROM orders WHERE organization_id = $1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&status); err != nil {
			return err
		}
		if status == domain.OrderStatusCompleted {
			// По тому же ключу синхронизируются отгрузки и изменение остатка дров.
			if err := lockFirewood(ctx, tx, orgID); err != nil {
				return err
			}
			var saleID int64
			err := tx.QueryRow(ctx, `SELECT id FROM sales WHERE organization_id = $1 AND order_id=$2 FOR UPDATE`, orgID, id).Scan(&saleID)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrOrderSaleLink
			}
			if err != nil {
				return err
			}
			if err := reverseSale(ctx, tx, orgID, saleID); err != nil {
				return err
			}
		}
		// Вместе с заказом каскадно удаляются позиции, оплаты и списание готовых дров.
		_, err := tx.Exec(ctx, `DELETE FROM orders WHERE organization_id = $1 AND id=$2`, orgID, id)
		return err
	})
}

func (r *Repository) DeleteSale(ctx context.Context, orgID int64, id int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockOrgShare(ctx, tx, orgID); err != nil {
			return err
		}
		var orderSale bool
		if err := tx.QueryRow(ctx, `SELECT order_id IS NOT NULL OR legacy_order_sale FROM sales WHERE organization_id = $1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&orderSale); err != nil {
			return err
		}
		if orderSale {
			return fmt.Errorf("%w: удалите заказ, чтобы отменить связанную продажу", ErrOperationInUse)
		}
		return reverseSale(ctx, tx, orgID, id)
	})
}

func reverseSale(ctx context.Context, tx pgx.Tx, orgID int64, id int64) error {
	// Для возврата берём фактически списанные объёмы, сохранённые в чеке.
	if _, err := tx.Exec(ctx, `SELECT b.id FROM boards b
		WHERE b.organization_id = $1 AND b.id IN (SELECT board_id FROM sale_boards WHERE organization_id = $1 AND sale_id=$2)
		ORDER BY b.id FOR UPDATE`, orgID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE boards b SET count=b.count+s.count, volume_m3=b.volume_m3+s.volume
		FROM (SELECT board_id, SUM(count) AS count, SUM(volume_m3) AS volume
		      FROM sale_boards WHERE organization_id = $1 AND sale_id=$2 GROUP BY board_id) s WHERE b.organization_id = $1 AND b.id=s.board_id`, orgID, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM sales WHERE organization_id = $1 AND id=$2`, orgID, id)
	return err
}

func (r *Repository) DeleteLogArrival(ctx context.Context, orgID int64, id int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockOrgShare(ctx, tx, orgID); err != nil {
			return err
		}
		var arrivalID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM log_arrivals WHERE organization_id = $1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&arrivalID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM log_items WHERE organization_id = $1 AND arrival_id=$2 ORDER BY id FOR UPDATE`, orgID, id); err != nil {
			return err
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM log_items li WHERE li.organization_id = $1 AND li.arrival_id=$2 AND (
			    li.remaining_count <> li.count OR li.remaining_volume_m3 <> li.volume_m3
			    OR EXISTS (SELECT 1 FROM sawed_logs sl WHERE sl.organization_id = $1 AND sl.log_id=li.id))
		)`, orgID, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return fmt.Errorf("%w: часть брёвен из прихода уже израсходована", ErrOperationInUse)
		}
		_, err := tx.Exec(ctx, `DELETE FROM log_arrivals WHERE organization_id = $1 AND id=$2`, orgID, id)
		return err
	})
}

func (r *Repository) DeleteSawing(ctx context.Context, orgID int64, id int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockOrgShare(ctx, tx, orgID); err != nil {
			return err
		}
		var operationID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM sawing_operations WHERE organization_id = $1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&operationID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM boards WHERE organization_id = $1 AND sawing_id=$2 ORDER BY id FOR UPDATE`, orgID, id); err != nil {
			return err
		}
		var hasBoards, used, hasUnlinkedLogs bool
		err := tx.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM boards WHERE organization_id = $1 AND sawing_id=$2),
			EXISTS (SELECT 1 FROM boards b WHERE b.organization_id = $1 AND b.sawing_id=$2 AND (
				EXISTS (SELECT 1 FROM sale_boards WHERE organization_id = $1 AND board_id=b.id)
				OR EXISTS (SELECT 1 FROM order_boards WHERE organization_id = $1 AND board_id=b.id))),
			EXISTS (SELECT 1 FROM sawed_logs WHERE organization_id = $1 AND sawing_id=$2 AND log_id IS NULL)`, orgID, id).
			Scan(&hasBoards, &used, &hasUnlinkedLogs)
		if err != nil {
			return err
		}
		if !hasBoards || hasUnlinkedLogs {
			return fmt.Errorf("%w: у распила нет связи с выпущенными досками или партией сырья", ErrOperationInUse)
		}
		if used {
			return fmt.Errorf("%w: доски из распила есть в заказах или продажах", ErrOperationInUse)
		}
		if _, err := tx.Exec(ctx, `SELECT li.id FROM log_items li
			WHERE li.organization_id = $1 AND li.id IN (SELECT log_id FROM sawed_logs WHERE organization_id = $1 AND sawing_id=$2) ORDER BY li.id FOR UPDATE`, orgID, id); err != nil {
			return err
		}
		// У старых распилов точное число списанных брёвен не сохранилось.
		// Восстанавливаем его по доле объёма, как в миграции учёта остатков.
		if _, err := tx.Exec(ctx, `WITH restored AS (
			SELECT log_id, SUM(COALESCE(consumed_volume_m3, volume_m3)) AS volume,
				COALESCE(SUM(consumed_count),0) AS pieces,
				COALESCE(SUM(volume_m3) FILTER (WHERE consumed_count IS NULL),0) AS legacy_volume
			FROM sawed_logs WHERE organization_id = $1 AND sawing_id=$2 GROUP BY log_id
		)
		UPDATE log_items li SET
			remaining_volume_m3=LEAST(li.volume_m3,li.remaining_volume_m3+r.volume),
			remaining_count=CASE WHEN li.remaining_volume_m3+r.volume <= 0 THEN 0
				WHEN li.remaining_volume_m3+r.volume >= li.volume_m3 THEN li.count
				ELSE LEAST(li.count, GREATEST(1, li.remaining_count+r.pieces+
					ROUND(li.count*r.legacy_volume/NULLIF(li.volume_m3,0)))) END
		FROM restored r WHERE li.organization_id = $1 AND li.id=r.log_id`, orgID, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM boards WHERE organization_id = $1 AND sawing_id=$2`, orgID, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM sawing_operations WHERE organization_id = $1 AND id=$2`, orgID, id)
		return err
	})
}

func (r *Repository) UpdateBoard(ctx context.Context, orgID int64, b domain.Board, actorID int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := lockOrgShare(ctx, tx, orgID); err != nil {
			return err
		}
		var oldCount int
		var oldVolume float64
		if err := tx.QueryRow(ctx, `SELECT count, volume_m3 FROM boards WHERE organization_id = $1 AND id=$2 FOR UPDATE`, orgID, b.ID).Scan(&oldCount, &oldVolume); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE boards SET species=$3, grade=$4, height_mm=$5, width_mm=$6,
			length_mm=$7, count=$8, volume_m3=$9, price_per_m3=$10 WHERE organization_id = $1 AND id=$2`,
			orgID, b.ID, b.Species, b.Grade, b.HeightMM, b.WidthMM, b.LengthMM, b.Count, b.VolumeM3, b.PricePerM3); err != nil {
			return err
		}
		deltaCount, deltaVolume := b.Count-oldCount, round3(b.VolumeM3-oldVolume)
		if deltaCount == 0 && deltaVolume == 0 {
			return nil
		}
		label := fmt.Sprintf("%s · доска %d×%d · %s", b.Species, b.HeightMM, b.WidthMM, lenLabel(b.LengthMM))
		_, err := tx.Exec(ctx, `INSERT INTO stock_adjustments
			(organization_id, kind, item_id, item_label, delta_count, delta_volume, reason, note, created_by)
			VALUES ($1, 'board',$2,$3,$4,$5,'other','Прямое редактирование позиции',$6)`,
			orgID, b.ID, label, deltaCount, deltaVolume, actorID)
		return err
	})
}
