package workflow

import (
	"context"
	"testing"
	"time"
)

func TestEveryWorkflowTypeHasADefinition(t *testing.T) {
	found := map[WorkflowType]bool{}
	for _, d := range Definitions {
		if d.Type == "" || d.Description == "" {
			t.Fatalf("definition %+v needs a type and a description", d)
		}
		found[d.Type] = true
	}
	if !found[WorkflowDailyUpdate] {
		t.Fatalf("daily_update has no definition")
	}
}

// seedRunsInEveryState inserts runs for daily_update: two completed, one
// failed, one running.
func seedRunsInEveryState(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	base := NewBusinessDate(2026, time.March, 1)
	for i, st := range []RunStatus{StatusCompleted, StatusCompleted, StatusFailed, StatusRunning} {
		d := base
		for range i {
			d = d.Next()
		}
		if err := s.CreateRun(ctx, &RunRecord{
			ID: "run-" + d.String(), WorkflowType: WorkflowDailyUpdate,
			BusinessDate: d, Status: st, StartedAt: time.Now(), CreatedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func checkCounts(t *testing.T, s Store) {
	t.Helper()
	counts, err := s.CountRuns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 0 {
		t.Fatalf("empty store should report no workflow types, got %v", counts)
	}
	seedRunsInEveryState(t, s)
	counts, err = s.CountRuns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	daily := counts[WorkflowDailyUpdate]
	want := RunCounts{StatusCompleted: 2, StatusFailed: 1, StatusRunning: 1}
	for st, n := range want {
		if daily[st] != n {
			t.Fatalf("daily_update %s = %d, want %d (all: %v)", st, daily[st], n, daily)
		}
	}
	if daily[StatusPending] != 0 {
		t.Fatalf("pending = %d, want 0", daily[StatusPending])
	}
	if daily.Total() != 4 {
		t.Fatalf("Total() = %d, want 4", daily.Total())
	}
}

func TestMemStore_CountRuns(t *testing.T) { checkCounts(t, NewMemStore()) }

// A retry after a failure is a new attempt at the same instance, not a
// second instance: the failed run is replaced.
func checkRetryReplacesFailedRun(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	d := NewBusinessDate(2026, time.April, 1)
	first := &RunRecord{ID: "attempt-1", WorkflowType: WorkflowDailyUpdate, BusinessDate: d, Status: StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := s.CreateRun(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := s.FailRun(ctx, first.ID, "boom"); err != nil {
		t.Fatal(err)
	}
	second := &RunRecord{ID: "attempt-2", WorkflowType: WorkflowDailyUpdate, BusinessDate: d, Status: StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now().Add(time.Millisecond)}
	if err := s.CreateRun(ctx, second); err != nil {
		t.Fatalf("retry after failure must be accepted: %v", err)
	}
	runs, err := s.ListRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "attempt-2" {
		t.Fatalf("want exactly the retry attempt, got %+v", runs)
	}
	if got, _ := s.GetRun(ctx, "attempt-1"); got != nil {
		t.Fatalf("failed attempt should be gone, got %+v", got)
	}
	counts, _ := s.CountRuns(ctx)
	if counts[WorkflowDailyUpdate][StatusFailed] != 0 || counts[WorkflowDailyUpdate].Total() != 1 {
		t.Fatalf("counts after retry = %v", counts[WorkflowDailyUpdate])
	}
}

func TestMemStore_RetryReplacesFailedRun(t *testing.T) { checkRetryReplacesFailedRun(t, NewMemStore()) }
