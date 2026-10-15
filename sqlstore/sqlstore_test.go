package sqlstore

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	db "git.bytestone.uk/hum3/gobank-db"
	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/storetest"
	_ "github.com/ncruces/go-sqlite3/embed"
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
	store, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	date := wf.NewBusinessDate(2026, time.January, 15)
	run := &wf.RunRecord{
		ID:           "run-1",
		WorkflowType: wf.WorkflowDailyUpdate,
		BusinessDate: date,
		Status:       wf.StatusRunning,
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
	if got == nil || got.Status != wf.StatusRunning {
		t.Fatalf("expected running run, got %v", got)
	}

	if err := store.CompleteRun(ctx, "run-1"); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	got, _ = store.GetRun(ctx, "run-1")
	if got.Status != wf.StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}

func TestSQLStore_IdempotencyGuard(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	date := wf.NewBusinessDate(2026, time.January, 15)
	run := &wf.RunRecord{
		ID:           "run-1",
		WorkflowType: wf.WorkflowDailyUpdate,
		BusinessDate: date,
		Status:       wf.StatusRunning,
		StartedAt:    time.Now(),
		CreatedAt:    time.Now(),
	}
	store.CreateRun(ctx, run)
	store.CompleteRun(ctx, "run-1")

	run2 := &wf.RunRecord{
		ID:           "run-2",
		WorkflowType: wf.WorkflowDailyUpdate,
		BusinessDate: date,
		Status:       wf.StatusPending,
		StartedAt:    time.Now(),
		CreatedAt:    time.Now(),
	}
	err = store.CreateRun(ctx, run2)
	if err != wf.ErrAlreadyCompleted {
		t.Fatalf("expected wf.ErrAlreadyCompleted, got %v", err)
	}
}

func TestSQLStore_Steps(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Need a run first for FK.
	run := &wf.RunRecord{
		ID:           "run-1",
		WorkflowType: wf.WorkflowDailyUpdate,
		BusinessDate: wf.NewBusinessDate(2026, time.January, 15),
		Status:       wf.StatusRunning,
		StartedAt:    time.Now(),
		CreatedAt:    time.Now(),
	}
	store.CreateRun(ctx, run)

	store.RecordStep(ctx, &wf.StepResult{
		ID: "s1", RunID: "run-1", AccountID: "acct-1",
		StepName: "end_of_day", OpeningBalance: 100000, ClosingBalance: 100004,
		InterestAmount: 4, Exponent: -2, CreatedAt: time.Now(),
	})
	store.RecordStep(ctx, &wf.StepResult{
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
	store, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// No runs.
	last, err := store.LastCompletedDate(ctx, wf.WorkflowDailyUpdate)
	if err != nil {
		t.Fatalf("LastCompletedDate: %v", err)
	}
	if !last.IsZero() {
		t.Fatalf("expected zero, got %s", last)
	}

	// Add runs.
	for i, d := range []int{10, 15, 20} {
		run := &wf.RunRecord{
			ID:           string(rune('a' + i)),
			WorkflowType: wf.WorkflowDailyUpdate,
			BusinessDate: wf.NewBusinessDate(2026, time.January, d),
			Status:       wf.StatusRunning,
			StartedAt:    time.Now(),
			CreatedAt:    time.Now(),
		}
		store.CreateRun(ctx, run)
		store.CompleteRun(ctx, run.ID)
	}

	last, _ = store.LastCompletedDate(ctx, wf.WorkflowDailyUpdate)
	expected := wf.NewBusinessDate(2026, time.January, 20)
	if !last.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected, last)
	}
}

func TestSQLStore_SchemaIdempotent(t *testing.T) {
	db := newTestDB(t)
	_, err := New(db)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	_, err = New(db)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
}

func TestSQLStore_FindRunNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store, _ := New(db)

	got, err := store.FindRun(ctx, wf.WorkflowDailyUpdate, wf.NewBusinessDate(2026, time.January, 1))
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
	store, _ := New(db)

	for i := 1; i <= 5; i++ {
		store.CreateRun(ctx, &wf.RunRecord{
			ID:           string(rune('a' + i)),
			WorkflowType: wf.WorkflowDailyUpdate,
			BusinessDate: wf.NewBusinessDate(2026, time.January, i),
			Status:       wf.StatusCompleted,
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

func TestMigrationsEndAtTheSchemaVersionTheCodeExpects(t *testing.T) {
	if err := db.Validate(Migrations); err != nil {
		t.Fatalf("Migrations invalid: %v", err)
	}
	if got := Migrations[len(Migrations)-1].Version; got != wf.SchemaVersion {
		t.Fatalf("last migration is v%d but wf.SchemaVersion is %d", got, wf.SchemaVersion)
	}
}

func TestNewRecordsTheSchemaVersion(t *testing.T) {
	store, err := New(newTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != wf.SchemaVersion {
		t.Fatalf("applied schema version = %d, want %d", got, wf.SchemaVersion)
	}
}

func TestCountRuns(t *testing.T) {
	ctx := context.Background()
	store, err := New(newTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	counts, err := store.CountRuns(ctx)
	if err != nil || len(counts) != 0 {
		t.Fatalf("empty store: counts=%v err=%v", counts, err)
	}
	d := wf.NewBusinessDate(2026, time.March, 1)
	for _, st := range []wf.RunStatus{wf.StatusCompleted, wf.StatusCompleted, wf.StatusFailed, wf.StatusRunning} {
		if err := store.CreateRun(ctx, &wf.RunRecord{
			ID: "run-" + d.String(), WorkflowType: wf.WorkflowDailyUpdate,
			BusinessDate: d, Status: st, StartedAt: time.Now(), CreatedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
		d = d.Next()
	}
	counts, err = store.CountRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	daily := counts[wf.WorkflowDailyUpdate]
	if daily[wf.StatusCompleted] != 2 || daily[wf.StatusFailed] != 1 || daily[wf.StatusRunning] != 1 || daily.Total() != 4 {
		t.Fatalf("daily_update counts = %v", daily)
	}
}

func TestRetryReplacesFailedRun(t *testing.T) {
	ctx := context.Background()
	store, err := New(newTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	d := wf.NewBusinessDate(2026, time.April, 1)
	first := &wf.RunRecord{ID: "attempt-1", WorkflowType: wf.WorkflowDailyUpdate, BusinessDate: d, Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := store.CreateRun(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordStep(ctx, &wf.StepResult{ID: "s1", RunID: first.ID, AccountID: "a", StepName: "end_of_day", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := store.FailRun(ctx, first.ID, "boom"); err != nil {
		t.Fatal(err)
	}
	second := &wf.RunRecord{ID: "attempt-2", WorkflowType: wf.WorkflowDailyUpdate, BusinessDate: d, Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now().Add(time.Millisecond)}
	if err := store.CreateRun(ctx, second); err != nil {
		t.Fatalf("retry after failure must be accepted: %v", err)
	}
	runs, _ := store.ListRuns(ctx, 10)
	if len(runs) != 1 || runs[0].ID != "attempt-2" {
		t.Fatalf("want exactly the retry attempt, got %+v", runs)
	}
	if steps, _ := store.ListSteps(ctx, "attempt-1"); len(steps) != 0 {
		t.Fatalf("steps of the replaced attempt should be gone, got %d", len(steps))
	}
}

func TestKeyedInstancesAndRestart(t *testing.T) {
	store, err := New(newTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	storetest.KeyedInstances(t, store)
	storetest.RestartAndStepStatus(t, store)
	storetest.WaitAndResume(t, store)
	storetest.UndoneStep(t, store)
}

func TestSchemaHistoryIsThreeVersions(t *testing.T) {
	if len(Migrations) != 3 || Migrations[1].Phase != db.Expand || Migrations[2].Phase != db.Contract {
		t.Fatalf("expected v1 expand, v2 expand, v3 contract; got %+v", Migrations)
	}
}
