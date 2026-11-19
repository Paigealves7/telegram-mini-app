package repository

import (
	"context"
	"errors"
	"fmt"

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
		if !org.IsDemo {
			return ErrResetNotDemo
		}
		if err := lockFirewood(ctx, tx, orgID); err != nil {
			return err
		}
		tables := []string{
			"order_payments", "order_boards", "order_firewood", "sale_boards", "sales", "orders",
			"sawed_logs", "boards", "sawing_operations", "log_items", "log_arrivals",
			"firewood_processing", "firewood_arrivals", "stock_adjustments", "firewood_stock_adjustments",
		}
		for _, table := range tables {
			if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE organization_id = $1", orgID); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE organizations SET last_order_number = 0 WHERE id = $1`, orgID)
		return err
	})
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
