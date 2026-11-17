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

func (r *Repository) ResetTestData(ctx context.Context) error {
	// Один TRUNCATE атомарно очищает операции и сбрасывает их последовательности.
	_, err := r.db.Exec(ctx, `TRUNCATE TABLE order_payments, order_boards, order_firewood, orders,
		sale_boards, sales, sawed_logs, boards, sawing_operations, log_items, log_arrivals,
		firewood_processing, firewood_arrivals, stock_adjustments, firewood_stock_adjustments
		RESTART IDENTITY CASCADE`)
	return err
}

func (r *Repository) DeleteOrder(ctx context.Context, id int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var status domain.OrderStatus
		if err := tx.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
			return err
		}
		if status == domain.OrderStatusCompleted {
			// По тому же ключу синхронизируются отгрузки и изменение остатка дров.
			if err := lockFirewood(ctx, tx); err != nil {
				return err
			}
			var saleID int64
			err := tx.QueryRow(ctx, `SELECT id FROM sales WHERE order_id=$1 FOR UPDATE`, id).Scan(&saleID)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrOrderSaleLink
			}
			if err != nil {
				return err
			}
			if err := reverseSale(ctx, tx, saleID); err != nil {
				return err
			}
		}
		// Вместе с заказом каскадно удаляются позиции, оплаты и списание готовых дров.
		_, err := tx.Exec(ctx, `DELETE FROM orders WHERE id=$1`, id)
		return err
	})
}

func (r *Repository) DeleteSale(ctx context.Context, id int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var orderSale bool
		if err := tx.QueryRow(ctx, `SELECT order_id IS NOT NULL OR legacy_order_sale FROM sales WHERE id=$1 FOR UPDATE`, id).Scan(&orderSale); err != nil {
			return err
		}
		if orderSale {
			return fmt.Errorf("%w: удалите заказ, чтобы отменить связанную продажу", ErrOperationInUse)
		}
		return reverseSale(ctx, tx, id)
	})
}

func reverseSale(ctx context.Context, tx pgx.Tx, id int64) error {
	// Для возврата берём фактически списанные объёмы, сохранённые в чеке.
	if _, err := tx.Exec(ctx, `SELECT b.id FROM boards b
		WHERE b.id IN (SELECT board_id FROM sale_boards WHERE sale_id=$1)
		ORDER BY b.id FOR UPDATE`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE boards b SET count=b.count+s.count, volume_m3=b.volume_m3+s.volume
		FROM (SELECT board_id, SUM(count) AS count, SUM(volume_m3) AS volume
		      FROM sale_boards WHERE sale_id=$1 GROUP BY board_id) s WHERE b.id=s.board_id`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM sales WHERE id=$1`, id)
	return err
}

func (r *Repository) DeleteLogArrival(ctx context.Context, id int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var arrivalID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM log_arrivals WHERE id=$1 FOR UPDATE`, id).Scan(&arrivalID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM log_items WHERE arrival_id=$1 ORDER BY id FOR UPDATE`, id); err != nil {
			return err
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM log_items li WHERE li.arrival_id=$1 AND (
			    li.remaining_count <> li.count OR li.remaining_volume_m3 <> li.volume_m3
			    OR EXISTS (SELECT 1 FROM sawed_logs sl WHERE sl.log_id=li.id))
		)`, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return fmt.Errorf("%w: часть брёвен из прихода уже израсходована", ErrOperationInUse)
		}
		_, err := tx.Exec(ctx, `DELETE FROM log_arrivals WHERE id=$1`, id)
		return err
	})
}

func (r *Repository) DeleteSawing(ctx context.Context, id int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var operationID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM sawing_operations WHERE id=$1 FOR UPDATE`, id).Scan(&operationID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM boards WHERE sawing_id=$1 ORDER BY id FOR UPDATE`, id); err != nil {
			return err
		}
		var hasBoards, used, hasUnlinkedLogs bool
		err := tx.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM boards WHERE sawing_id=$1),
			EXISTS (SELECT 1 FROM boards b WHERE b.sawing_id=$1 AND (
				EXISTS (SELECT 1 FROM sale_boards WHERE board_id=b.id)
				OR EXISTS (SELECT 1 FROM order_boards WHERE board_id=b.id))),
			EXISTS (SELECT 1 FROM sawed_logs WHERE sawing_id=$1 AND log_id IS NULL)`, id).
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
			WHERE li.id IN (SELECT log_id FROM sawed_logs WHERE sawing_id=$1) ORDER BY li.id FOR UPDATE`, id); err != nil {
			return err
		}
		// У старых распилов точное число списанных брёвен не сохранилось.
		// Восстанавливаем его по доле объёма, как в миграции учёта остатков.
		if _, err := tx.Exec(ctx, `WITH restored AS (
			SELECT log_id, SUM(COALESCE(consumed_volume_m3, volume_m3)) AS volume,
				COALESCE(SUM(consumed_count),0) AS pieces,
				COALESCE(SUM(volume_m3) FILTER (WHERE consumed_count IS NULL),0) AS legacy_volume
			FROM sawed_logs WHERE sawing_id=$1 GROUP BY log_id
		)
		UPDATE log_items li SET
			remaining_volume_m3=LEAST(li.volume_m3,li.remaining_volume_m3+r.volume),
			remaining_count=CASE WHEN li.remaining_volume_m3+r.volume <= 0 THEN 0
				WHEN li.remaining_volume_m3+r.volume >= li.volume_m3 THEN li.count
				ELSE LEAST(li.count, GREATEST(1, li.remaining_count+r.pieces+
					ROUND(li.count*r.legacy_volume/NULLIF(li.volume_m3,0)))) END
		FROM restored r WHERE li.id=r.log_id`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM boards WHERE sawing_id=$1`, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM sawing_operations WHERE id=$1`, id)
		return err
	})
}

func (r *Repository) UpdateBoard(ctx context.Context, b domain.Board, actorID int64) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var oldCount int
		var oldVolume float64
		if err := tx.QueryRow(ctx, `SELECT count, volume_m3 FROM boards WHERE id=$1 FOR UPDATE`, b.ID).Scan(&oldCount, &oldVolume); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE boards SET species=$2, grade=$3, height_mm=$4, width_mm=$5,
			length_mm=$6, count=$7, volume_m3=$8, price_per_m3=$9 WHERE id=$1`,
			b.ID, b.Species, b.Grade, b.HeightMM, b.WidthMM, b.LengthMM, b.Count, b.VolumeM3, b.PricePerM3); err != nil {
			return err
		}
		deltaCount, deltaVolume := b.Count-oldCount, round3(b.VolumeM3-oldVolume)
		if deltaCount == 0 && deltaVolume == 0 {
			return nil
		}
		label := fmt.Sprintf("%s · доска %d×%d · %s", b.Species, b.HeightMM, b.WidthMM, lenLabel(b.LengthMM))
		_, err := tx.Exec(ctx, `INSERT INTO stock_adjustments
			(kind, item_id, item_label, delta_count, delta_volume, reason, note, created_by)
			VALUES ('board',$1,$2,$3,$4,'other','Прямое редактирование позиции',$5)`,
			b.ID, label, deltaCount, deltaVolume, actorID)
		return err
	})
}
