package workflow

import (
	"context"
	"testing"
	"time"
)

func TestMemStore_RunLifecycle(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()
	date := NewBusinessDate(2026, time.January, 15)

	run := &RunRecord{
		ID:           "run-1",
		WorkflowType: WorkflowDailyUpdate,
		BusinessDate: date,
		Status:       StatusPending,
		StartedAt:    time.Now(),
	}

	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	got, err := s.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got == nil || got.Status != StatusPending {
		t.Fatalf("expected pending run, got %v", got)
	}

	got, err = s.FindRun(ctx, WorkflowDailyUpdate, date)
	if err != nil {
		t.Fatalf("FindRun: %v", err)
	}
	if got == nil || got.ID != "run-1" {
		t.Fatalf("FindRun returned wrong run: %v", got)
	}

	if err := s.CompleteRun(ctx, "run-1"); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	got, err = s.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetRun after complete: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}

func TestMemStore_IdempotencyGuard(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()
	date := NewBusinessDate(2026, time.January, 15)

	run1 := &RunRecord{
		ID:           "run-1",
		WorkflowType: WorkflowDailyUpdate,
		BusinessDate: date,
		Status:       StatusPending,
	}
	if err := s.CreateRun(ctx, run1); err != nil {
		t.Fatalf("first CreateRun: %v", err)
	}
	if err := s.CompleteRun(ctx, "run-1"); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	run2 := &RunRecord{
		ID:           "run-2",
		WorkflowType: WorkflowDailyUpdate,
		BusinessDate: date,
		Status:       StatusPending,
	}
	err := s.CreateRun(ctx, run2)
	if err != ErrAlreadyCompleted {
		t.Fatalf("expected ErrAlreadyCompleted, got %v", err)
	}
}

func TestMemStore_FailRun(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	run := &RunRecord{
		ID:           "run-1",
		WorkflowType: WorkflowDailyUpdate,
		BusinessDate: NewBusinessDate(2026, time.January, 15),
		Status:       StatusRunning,
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := s.FailRun(ctx, "run-1", "something broke"); err != nil {
		t.Fatalf("FailRun: %v", err)
	}

	got, _ := s.GetRun(ctx, "run-1")
	if got.Status != StatusFailed || got.Error != "something broke" {
		t.Fatalf("expected failed run with error, got %+v", got)
	}
}

func TestMemStore_Steps(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	s.RecordStep(ctx, &StepResult{ID: "s1", RunID: "run-1", AccountID: "acct-1", StepName: "end_of_day"})
	s.RecordStep(ctx, &StepResult{ID: "s2", RunID: "run-1", AccountID: "acct-2", StepName: "end_of_day"})
	s.RecordStep(ctx, &StepResult{ID: "s3", RunID: "run-2", AccountID: "acct-1", StepName: "end_of_day"})

	steps, err := s.ListSteps(ctx, "run-1")
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
}

func TestMemStore_LastCompletedDate(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	// No runs yet.
	last, err := s.LastCompletedDate(ctx, WorkflowDailyUpdate)
	if err != nil {
		t.Fatalf("LastCompletedDate: %v", err)
	}
	if !last.IsZero() {
		t.Fatalf("expected zero date, got %s", last)
	}

	// Add two completed runs.
	for i, d := range []int{10, 15} {
		run := &RunRecord{
			ID:           string(rune('a' + i)),
			WorkflowType: WorkflowDailyUpdate,
			BusinessDate: NewBusinessDate(2026, time.January, d),
			Status:       StatusPending,
		}
		s.CreateRun(ctx, run)
		s.CompleteRun(ctx, run.ID)
	}

	last, err = s.LastCompletedDate(ctx, WorkflowDailyUpdate)
	if err != nil {
		t.Fatalf("LastCompletedDate: %v", err)
	}
	expected := NewBusinessDate(2026, time.January, 15)
	if !last.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected, last)
	}
}

func TestMemStore_ListRuns(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	for i := 1; i <= 5; i++ {
		s.CreateRun(ctx, &RunRecord{
			ID:           string(rune('a' + i)),
			WorkflowType: WorkflowDailyUpdate,
			BusinessDate: NewBusinessDate(2026, time.January, i),
			Status:       StatusCompleted,
		})
	}

	runs, err := s.ListRuns(ctx, RunFilter{Limit: 3})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
}

func TestMemStore_FindRunNotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	got, err := s.FindRun(ctx, WorkflowDailyUpdate, NewBusinessDate(2026, time.January, 1))
	if err != nil {
		t.Fatalf("FindRun: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}
