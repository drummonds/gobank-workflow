package workflow

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "codeberg.org/hum3/go-postgres"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLStore_RunLifecycle(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store, err := NewSQLStore(db)
	if err != nil {
		t.Fatalf("NewSQLStore: %v", err)
	}

	date := NewBusinessDate(2026, time.January, 15)
	run := &RunRecord{
		ID:           "run-1",
		WorkflowType: WorkflowDailyUpdate,
		BusinessDate: date,
		Status:       StatusRunning,
		StartedAt:    time.Now(),
		CreatedAt:    time.Now(),
	}

	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	got, err := store.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got == nil || got.Status != StatusRunning {
		t.Fatalf("expected running run, got %v", got)
	}

	if err := store.CompleteRun(ctx, "run-1"); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	got, _ = store.GetRun(ctx, "run-1")
	if got.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}

func TestSQLStore_IdempotencyGuard(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store, err := NewSQLStore(db)
	if err != nil {
		t.Fatalf("NewSQLStore: %v", err)
	}

	date := NewBusinessDate(2026, time.January, 15)
	run := &RunRecord{
		ID:           "run-1",
		WorkflowType: WorkflowDailyUpdate,
		BusinessDate: date,
		Status:       StatusRunning,
		StartedAt:    time.Now(),
		CreatedAt:    time.Now(),
	}
	store.CreateRun(ctx, run)
	store.CompleteRun(ctx, "run-1")

	run2 := &RunRecord{
		ID:           "run-2",
		WorkflowType: WorkflowDailyUpdate,
		BusinessDate: date,
		Status:       StatusPending,
		StartedAt:    time.Now(),
		CreatedAt:    time.Now(),
	}
	err = store.CreateRun(ctx, run2)
	if err != ErrAlreadyCompleted {
		t.Fatalf("expected ErrAlreadyCompleted, got %v", err)
	}
}

func TestSQLStore_Steps(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store, err := NewSQLStore(db)
	if err != nil {
		t.Fatalf("NewSQLStore: %v", err)
	}

	// Need a run first for FK.
	run := &RunRecord{
		ID:           "run-1",
		WorkflowType: WorkflowDailyUpdate,
		BusinessDate: NewBusinessDate(2026, time.January, 15),
		Status:       StatusRunning,
		StartedAt:    time.Now(),
		CreatedAt:    time.Now(),
	}
	store.CreateRun(ctx, run)

	store.RecordStep(ctx, &StepResult{
		ID: "s1", RunID: "run-1", AccountID: "acct-1",
		StepName: "end_of_day", OpeningBalance: 100000, ClosingBalance: 100004,
		InterestAmount: 4, Exponent: -2, CreatedAt: time.Now(),
	})
	store.RecordStep(ctx, &StepResult{
		ID: "s2", RunID: "run-1", AccountID: "acct-2",
		StepName: "end_of_day", CreatedAt: time.Now(),
	})

	steps, err := store.ListSteps(ctx, "run-1")
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].OpeningBalance != 100000 {
		t.Fatalf("expected opening balance 100000, got %d", steps[0].OpeningBalance)
	}
}

func TestSQLStore_LastCompletedDate(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store, err := NewSQLStore(db)
	if err != nil {
		t.Fatalf("NewSQLStore: %v", err)
	}

	// No runs.
	last, err := store.LastCompletedDate(ctx, WorkflowDailyUpdate)
	if err != nil {
		t.Fatalf("LastCompletedDate: %v", err)
	}
	if !last.IsZero() {
		t.Fatalf("expected zero, got %s", last)
	}

	// Add runs.
	for i, d := range []int{10, 15, 20} {
		run := &RunRecord{
			ID:           string(rune('a' + i)),
			WorkflowType: WorkflowDailyUpdate,
			BusinessDate: NewBusinessDate(2026, time.January, d),
			Status:       StatusRunning,
			StartedAt:    time.Now(),
			CreatedAt:    time.Now(),
		}
		store.CreateRun(ctx, run)
		store.CompleteRun(ctx, run.ID)
	}

	last, _ = store.LastCompletedDate(ctx, WorkflowDailyUpdate)
	expected := NewBusinessDate(2026, time.January, 20)
	if !last.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected, last)
	}
}

func TestSQLStore_SchemaIdempotent(t *testing.T) {
	db := newTestDB(t)
	_, err := NewSQLStore(db)
	if err != nil {
		t.Fatalf("first NewSQLStore: %v", err)
	}
	_, err = NewSQLStore(db)
	if err != nil {
		t.Fatalf("second NewSQLStore: %v", err)
	}
}

func TestSQLStore_FindRunNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store, _ := NewSQLStore(db)

	got, err := store.FindRun(ctx, WorkflowDailyUpdate, NewBusinessDate(2026, time.January, 1))
	if err != nil {
		t.Fatalf("FindRun: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestSQLStore_ListRuns(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store, _ := NewSQLStore(db)

	for i := 1; i <= 5; i++ {
		store.CreateRun(ctx, &RunRecord{
			ID:           string(rune('a' + i)),
			WorkflowType: WorkflowDailyUpdate,
			BusinessDate: NewBusinessDate(2026, time.January, i),
			Status:       StatusCompleted,
			StartedAt:    time.Now(),
			CreatedAt:    time.Now(),
		})
	}

	runs, err := store.ListRuns(ctx, 3)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
}
