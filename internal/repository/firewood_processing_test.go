package repository

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"os"
	"telegram-mini-app/internal/domain"
	"testing"
)

func TestFirewoodLifecycle(t *testing.T) {
	url := os.Getenv("FIREWOOD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("isolated firewood database required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewRepository(db)
	var uid, cid int64
	if err = db.QueryRow(ctx, `INSERT INTO users(username,password_hash,role) VALUES('firewood-test','test','manager') RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO contractors(name) VALUES('firewood-test') RETURNING id`).Scan(&cid); err != nil {
		t.Fatal(err)
	}
	if _, err = r.CreateFirewoodArrival(ctx, cid, cid, "test", 10, "2026-11-14", uid); err != nil {
		t.Fatal(err)
	}
	p := domain.FirewoodProcessing{Kind: "split", VolumeM3: 3, Performer: "Рабочий", Date: "2026-11-14"}
	if err = r.CreateFirewoodProcessing(ctx, &p, uid); err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.RawM3-1.65) > 1e-9 {
		t.Fatalf("raw=%v", p.RawM3)
	}
	q := p
	q.Kind = "round"
	if err = r.CreateFirewoodProcessing(ctx, &q, uid); err != nil {
		t.Fatal(err)
	}
	if math.Abs(q.RawM3-1.8) > 1e-9 {
		t.Fatalf("raw=%v", q.RawM3)
	}
	if err = r.UpdateFirewoodSettings(ctx, .7, .8); err != nil {
		t.Fatal(err)
	}
	list, err := r.FirewoodProcessing(ctx, 100, 0)
	if err != nil || len(list) != 2 || list[1].Coefficient != .55 {
		t.Fatalf("history: %v %v", list, err)
	}
	s, err := r.GetFirewoodStock(ctx)
	if err != nil || math.Abs(s.RawM3-6.55) > 1e-9 || s.SplitM3 != 3 || s.RoundM3 != 3 {
		t.Fatalf("stock: %+v %v", s, err)
	}
	q.VolumeM3 = 100
	if err = r.CreateFirewoodProcessing(ctx, &q, uid); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("overspend: %v", err)
	}
	if err = r.DeleteFirewoodProcessing(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	s, err = r.GetFirewoodStock(ctx)
	if err != nil || math.Abs(s.RawM3-8.2) > 1e-9 || s.SplitM3 != 0 {
		t.Fatalf("delete: %+v %v", s, err)
	}
	order, err := r.CreateOrder(ctx, &domain.Order{MasterID: uid, ContractorID: cid, Status: domain.OrderStatusNew, FirewoodItems: []domain.FirewoodOrderItem{{Kind: "round", VolumeM3: 2, Price: 1500}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.AddPayment(ctx, order.ID, 1000, "аванс", uid); err != nil {
		t.Fatal(err)
	}
	if _, err = r.CompleteOrder(ctx, order.ID, uid); err != nil {
		t.Fatal(err)
	}
	if _, err = r.CompleteOrder(ctx, order.ID, uid); !errors.Is(err, ErrOrderClosed) {
		t.Fatalf("double complete: %v", err)
	}
	s, err = r.GetFirewoodStock(ctx)
	if err != nil || s.RoundM3 != 1 {
		t.Fatalf("completed stock: %+v %v", s, err)
	}
	if err = r.DeleteFirewoodProcessing(ctx, list[0].ID); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("delete sold production: %v", err)
	}
	payments, err := r.GetOrderPayments(ctx, order.ID)
	if err != nil || payments.TotalPrice != 3000 || payments.PaidAmount != 1000 {
		t.Fatalf("payments: %+v %v", payments, err)
	}
	debts, err := r.GetDebts(ctx)
	if err != nil || len(debts) != 1 || debts[0].Due != 2000 {
		t.Fatalf("debts: %+v %v", debts, err)
	}
	lines, err := r.GetOrderLines(ctx, order.ID)
	if err != nil || len(lines) != 1 || lines[0].FirewoodKind != "round" || lines[0].VolumeM3 != 2 {
		t.Fatalf("PDF lines: %+v %v", lines, err)
	}
	if _, err = r.AddPayment(ctx, order.ID, 2000, "остаток", uid); err != nil {
		t.Fatal(err)
	}
	debts, err = r.GetDebts(ctx)
	if err != nil || len(debts) != 0 {
		t.Fatalf("paid debts: %+v %v", debts, err)
	}
}
