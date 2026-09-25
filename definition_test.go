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
