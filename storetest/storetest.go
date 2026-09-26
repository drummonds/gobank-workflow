// Package storetest holds the Store contract checks shared by every store
// implementation's tests.
package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// KeyedInstances checks instance keys: several keyed runs may share a
// business date, FindRunByKey finds them, and a completed key is final.
func KeyedInstances(t *testing.T, s wf.Store) {
	t.Helper()
	ctx := context.Background()
	d := wf.NewBusinessDate(2026, time.May, 1)
	a := &wf.RunRecord{ID: "rel-a", WorkflowType: wf.WorkflowRelease, BusinessDate: d, Key: "v0.1.3", Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
	b := &wf.RunRecord{ID: "rel-b", WorkflowType: wf.WorkflowRelease, BusinessDate: d, Key: "v0.1.4", Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := s.CreateRun(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRun(ctx, b); err != nil {
		t.Fatalf("two releases on one date must both be accepted: %v", err)
	}
	got, err := s.FindRunByKey(ctx, wf.WorkflowRelease, "v0.1.4")
	if err != nil || got == nil || got.ID != "rel-b" || got.Key != "v0.1.4" {
		t.Fatalf("FindRunByKey = %+v, %v", got, err)
	}
	if got, _ := s.FindRunByKey(ctx, wf.WorkflowRelease, "v9.9.9"); got != nil {
		t.Fatalf("unknown key should be nil, got %+v", got)
	}
	if err := s.CompleteRun(ctx, "rel-a"); err != nil {
		t.Fatal(err)
	}
	again := &wf.RunRecord{ID: "rel-a2", WorkflowType: wf.WorkflowRelease, BusinessDate: d, Key: "v0.1.3", Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := s.CreateRun(ctx, again); !errors.Is(err, wf.ErrAlreadyCompleted) {
		t.Fatalf("a completed key is final, got %v", err)
	}
}

// RestartAndStepStatus checks step status round-trips, DeleteSteps by
// status, and RestartRun.
func RestartAndStepStatus(t *testing.T, s wf.Store) {
	t.Helper()
	ctx := context.Background()
	d := wf.NewBusinessDate(2026, time.May, 2)
	run := &wf.RunRecord{ID: "r1", WorkflowType: wf.WorkflowRelease, BusinessDate: d, Key: "v1", Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	for _, st := range []struct {
		id     string
		status wf.RunStatus
		errMsg string
	}{{"s1", wf.StatusCompleted, ""}, {"s2", wf.StatusFailed, "boom"}} {
		if err := s.RecordStep(ctx, &wf.StepResult{ID: st.id, RunID: "r1", StepName: st.id, Status: st.status, Error: st.errMsg, DurationNs: 5, CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	steps, _ := s.ListSteps(ctx, "r1")
	if len(steps) != 2 || steps[0].Status != wf.StatusCompleted || steps[1].Status != wf.StatusFailed || steps[1].Error != "boom" {
		t.Fatalf("step status not round-tripped: %+v", steps)
	}
	if err := s.FailRun(ctx, "r1", "stage failed"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSteps(ctx, "r1", wf.StatusFailed); err != nil {
		t.Fatal(err)
	}
	steps, _ = s.ListSteps(ctx, "r1")
	if len(steps) != 1 || steps[0].ID != "s1" {
		t.Fatalf("only the failed step should be deleted, got %+v", steps)
	}
	if err := s.RestartRun(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRun(ctx, "r1")
	if got.Status != wf.StatusRunning || got.Error != "" || !got.CompletedAt.IsZero() {
		t.Fatalf("RestartRun should put the run back to running with no error, got %+v", got)
	}
	if err := s.DeleteSteps(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	if steps, _ = s.ListSteps(ctx, "r1"); len(steps) != 0 {
		t.Fatalf("DeleteSteps with no statuses should clear all, got %d", len(steps))
	}
}
