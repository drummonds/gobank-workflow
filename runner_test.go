package workflow

import (
	"context"
	"testing"
	"time"

	luca "codeberg.org/hum3/go-luca"
	gbp "codeberg.org/hum3/gobank-products"
)

func newTestSim(t *testing.T, startDate time.Time) (*gbp.Simulation, *gbp.SimClock) {
	t.Helper()
	ledger, err := luca.NewLedger(":memory:")
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	t.Cleanup(func() { ledger.Close() })

	clock := gbp.NewSimClock(startDate)
	sim, err := gbp.NewSimulation(ledger, clock)
	if err != nil {
		t.Fatalf("NewSimulation: %v", err)
	}
	sim.RegisterProduct(gbp.EasyAccess())

	// Create a cash account for deposits/withdrawals.
	cash, err := sim.Ledger.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err != nil {
		t.Fatalf("CreateAccount(cash): %v", err)
	}

	ma, err := sim.OpenAccount("easy-access", "Liability:Savings:alice", "GBP", -2, map[string]string{
		"annual_rate": "0.015",
	})
	if err != nil {
		t.Fatalf("OpenAccount: %v", err)
	}

	err = sim.Deposit(ma.Account.ID, 100000, cash.ID, luca.CodeBookTransfer)
	if err != nil {
		t.Fatalf("Deposit: %v", err)
	}
	return sim, clock
}

func TestRunner_RunDaily(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sim, clock := newTestSim(t, start)
	store := NewMemStore()
	runner := NewRunner(store, sim, clock)

	date := NewBusinessDate(2026, time.January, 1)
	rec, err := runner.RunDaily(ctx, date)
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if rec == nil {
		t.Fatal("expected RunRecord, got nil")
	}
	if rec.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", rec.Status)
	}

	// Verify steps were recorded.
	steps, err := store.ListSteps(ctx, rec.ID)
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(steps) == 0 {
		t.Fatal("expected at least one step result")
	}
}

func TestRunner_Idempotent(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sim, clock := newTestSim(t, start)
	store := NewMemStore()
	runner := NewRunner(store, sim, clock)

	date := NewBusinessDate(2026, time.January, 1)

	rec1, err := runner.RunDaily(ctx, date)
	if err != nil {
		t.Fatalf("first RunDaily: %v", err)
	}
	if rec1 == nil {
		t.Fatal("first run should return record")
	}

	// Second call for same date returns nil (no-op).
	rec2, err := runner.RunDaily(ctx, date)
	if err != nil {
		t.Fatalf("second RunDaily: %v", err)
	}
	if rec2 != nil {
		t.Fatal("expected nil for idempotent re-run")
	}
}

func TestRunner_CatchUp(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sim, clock := newTestSim(t, start)
	store := NewMemStore()
	runner := NewRunner(store, sim, clock)

	from := NewBusinessDate(2026, time.January, 1)
	target := NewBusinessDate(2026, time.January, 5)
	recs, err := runner.CatchUp(ctx, from, target)
	if err != nil {
		t.Fatalf("CatchUp: %v", err)
	}
	if len(recs) != 5 {
		t.Fatalf("expected 5 runs (Jan 1-5), got %d", len(recs))
	}

	last, err := runner.LastProcessedDate(ctx)
	if err != nil {
		t.Fatalf("LastProcessedDate: %v", err)
	}
	if !last.Equal(target) {
		t.Fatalf("expected last processed %s, got %s", target, last)
	}
}

func TestRunner_CatchUpFromExisting(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sim, clock := newTestSim(t, start)
	store := NewMemStore()
	runner := NewRunner(store, sim, clock)

	// Process first 3 days.
	from := NewBusinessDate(2026, time.January, 1)
	_, err := runner.CatchUp(ctx, from, NewBusinessDate(2026, time.January, 3))
	if err != nil {
		t.Fatalf("first CatchUp: %v", err)
	}

	// Catch up to day 5 — should only process days 4 and 5.
	recs, err := runner.CatchUp(ctx, from, NewBusinessDate(2026, time.January, 5))
	if err != nil {
		t.Fatalf("second CatchUp: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 new runs (Jan 4-5), got %d", len(recs))
	}
}
