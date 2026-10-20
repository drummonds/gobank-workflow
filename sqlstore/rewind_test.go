package sqlstore

import (
	"context"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// Rewind reopens a completed run; the database must let it complete again.
func TestRewindACompletedRun(t *testing.T) {
	ctx := context.Background()
	s, err := New(newTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	r := wf.NewRunner(s, nil)
	runs := map[string]int{}
	stage := func(name string) wf.Stage {
		return wf.Stage{Name: name, Run: func(context.Context) error { runs[name]++; return nil }}
	}
	stages := []wf.Stage{stage("scan"), stage("check")}
	day := wf.NewBusinessDate(2026, time.October, 20)
	if rec, err := r.RunPipeline(ctx, "batch", "b1", day, stages); err != nil || rec.Status != wf.StatusCompleted {
		t.Fatalf("first run: %+v, %v", rec, err)
	}
	if _, err := r.Rewind(ctx, "batch", "b1", "check", stages); err != nil {
		t.Fatal(err)
	}
	rec, err := r.RunPipeline(ctx, "batch", "b1", day, stages)
	if err != nil || rec.Status != wf.StatusCompleted {
		t.Fatalf("after rewind: %+v, %v", rec, err)
	}
	if runs["scan"] != 1 || runs["check"] != 2 {
		t.Errorf("runs %v, want scan once, check twice", runs)
	}
	steps, _ := s.ListSteps(ctx, rec.ID)
	if len(steps) != 2 {
		t.Errorf("%d steps, want 2", len(steps))
	}
}
