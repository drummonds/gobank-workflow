package sqlstore

import (
	"context"
	"database/sql"
	"slices"
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

	runs, err := store.ListRuns(ctx, wf.RunFilter{Limit: 3})
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
	runs, _ := store.ListRuns(ctx, wf.RunFilter{Limit: 10})
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
	storetest.Revisions(t, store)
}

func TestFilteredRuns(t *testing.T) {
	store, err := New(newTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	storetest.FilteredRuns(t, store)
}

func TestTimeOrder(t *testing.T) {
	store, err := New(newTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	storetest.TimeOrder(t, store)
}

// Stamps written before v5 by time.RFC3339Nano, trailing zeros dropped,
// sort in time order once v5 has padded them.
func TestV5PadsEarlierStamps(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)
	if _, err := db.Apply(ctx, d, Migrations[:4]); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ id, key, at string }{
		{"r1", "a", "2026-04-01T09:00:05.5Z"},
		{"r2", "b", "2026-04-01T09:00:05.52Z"},
		{"r0", "c", "2026-04-01T09:00:05Z"},
	} {
		if _, err := d.ExecContext(ctx,
			`INSERT INTO workflow_runs (id, workflow_type, business_date, instance_key, status, started_at, created_at) VALUES (?, 'ship', '2026-04-01', ?, 'running', ?, ?)`,
			r.id, r.key, r.at, r.at); err != nil {
			t.Fatal(err)
		}
	}
	for _, st := range []struct{ id, at string }{
		{"s1", "2026-04-01T09:00:05.5Z"},
		{"s2", "2026-04-01T09:00:05.52Z"},
		{"s0", "2026-04-01T09:00:05Z"},
	} {
		if _, err := d.ExecContext(ctx,
			`INSERT INTO workflow_steps (id, run_id, account_id, step_name, created_at) VALUES (?, 'r1', '', 'build', ?)`,
			st.id, st.at); err != nil {
			t.Fatal(err)
		}
	}

	store, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.ListRuns(ctx, wf.RunFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var gotRuns []string
	for _, r := range runs {
		gotRuns = append(gotRuns, r.ID)
	}
	if want := []string{"r2", "r1", "r0"}; !slices.Equal(gotRuns, want) {
		t.Errorf("runs: got %v, want %v", gotRuns, want)
	}
	if want := time.Date(2026, time.April, 1, 9, 0, 5, 520_000_000, time.UTC); !runs[0].CreatedAt.Equal(want) {
		t.Errorf("r2 created at %v, want %v", runs[0].CreatedAt, want)
	}
	steps, err := store.ListSteps(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	var gotSteps []string
	for _, st := range steps {
		gotSteps = append(gotSteps, st.ID)
	}
	if want := []string{"s0", "s1", "s2"}; !slices.Equal(gotSteps, want) {
		t.Errorf("steps: got %v, want %v", gotSteps, want)
	}
}

func TestSchemaHistoryIsSixVersions(t *testing.T) {
	want := []db.Phase{db.Expand, db.Expand, db.Contract, db.Expand, db.Expand, db.Expand}
	if len(Migrations) != len(want) {
		t.Fatalf("expected %d migrations, got %+v", len(want), Migrations)
	}
	for i, m := range Migrations {
		if m.Phase != want[i] {
			t.Fatalf("v%d is %v, want %v", m.Version, m.Phase, want[i])
		}
	}
}

func TestCancelled(t *testing.T) {
	store, err := New(newTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	storetest.Cancelled(t, store)
}

func TestChildren(t *testing.T) {
	store, err := New(newTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	storetest.Children(t, store)
}
