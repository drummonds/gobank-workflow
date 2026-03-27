package workflow

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockProcessor is a simple DayProcessor for testing.
type mockProcessor struct {
	numAccounts int
	rate        float64 // annual rate
	balances    map[string]int64
}

func newMockProcessor(n int, rate float64, initialBalance int64) *mockProcessor {
	m := &mockProcessor{
		numAccounts: n,
		rate:        rate,
		balances:    make(map[string]int64, n),
	}
	for i := range n {
		m.balances[fmt.Sprintf("acct-%04d", i)] = initialBalance
	}
	return m
}

func (m *mockProcessor) ProcessDay(_ context.Context, date BusinessDate) (DayResult, error) {
	var results []AccountResult
	for id, bal := range m.balances {
		interest := int64(float64(bal) * m.rate / 365)
		newBal := bal + interest
		results = append(results, AccountResult{
			AccountID:      id,
			OpeningBalance: bal,
			ClosingBalance: newBal,
			InterestAmount: interest,
			Exponent:       -2,
		})
		m.balances[id] = newBal
	}
	return DayResult{Date: date, Accounts: results}, nil
}

func TestRunner_RunDaily(t *testing.T) {
	ctx := context.Background()
	proc := newMockProcessor(3, 0.035, 1000000)
	store := NewMemStore()
	runner := NewRunner(store, proc)

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

	steps, err := store.ListSteps(ctx, rec.ID)
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
}

func TestRunner_Idempotent(t *testing.T) {
	ctx := context.Background()
	proc := newMockProcessor(1, 0.035, 1000000)
	store := NewMemStore()
	runner := NewRunner(store, proc)

	date := NewBusinessDate(2026, time.January, 1)

	rec1, err := runner.RunDaily(ctx, date)
	if err != nil {
		t.Fatalf("first RunDaily: %v", err)
	}
	if rec1 == nil {
		t.Fatal("first run should return record")
	}

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
	proc := newMockProcessor(2, 0.035, 1000000)
	store := NewMemStore()
	runner := NewRunner(store, proc)

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
	proc := newMockProcessor(2, 0.035, 1000000)
	store := NewMemStore()
	runner := NewRunner(store, proc)

	from := NewBusinessDate(2026, time.January, 1)
	_, err := runner.CatchUp(ctx, from, NewBusinessDate(2026, time.January, 3))
	if err != nil {
		t.Fatalf("first CatchUp: %v", err)
	}

	recs, err := runner.CatchUp(ctx, from, NewBusinessDate(2026, time.January, 5))
	if err != nil {
		t.Fatalf("second CatchUp: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 new runs (Jan 4-5), got %d", len(recs))
	}
}

func TestRunner_StepInterestAccumulates(t *testing.T) {
	ctx := context.Background()
	proc := newMockProcessor(1, 0.035, 1000000) // 10,000.00 at 3.5%
	store := NewMemStore()
	runner := NewRunner(store, proc)

	from := NewBusinessDate(2026, time.January, 1)
	recs, err := runner.CatchUp(ctx, from, NewBusinessDate(2026, time.January, 3))
	if err != nil {
		t.Fatalf("CatchUp: %v", err)
	}

	// Check that day 2's opening balance reflects day 1's interest.
	steps1, _ := store.ListSteps(ctx, recs[0].ID)
	steps2, _ := store.ListSteps(ctx, recs[1].ID)
	if steps2[0].OpeningBalance != steps1[0].ClosingBalance {
		t.Fatalf("day 2 opening (%d) should equal day 1 closing (%d)",
			steps2[0].OpeningBalance, steps1[0].ClosingBalance)
	}
}
