package workflow

import (
	"context"
	"strings"
	"testing"
	"time"
)

// counted is a pipeline whose stages count their runs; gate, when set,
// is the condition the "approve" stage waits on, and failAt names a
// stage that fails.
type counted struct {
	runs   map[string]int
	gate   bool
	failAt string
}

func (c *counted) stages() []Stage {
	if c.runs == nil {
		c.runs = map[string]int{}
	}
	run := func(name string) func(context.Context) error {
		return func(context.Context) error {
			c.runs[name]++
			if name == c.failAt {
				return errFail
			}
			return nil
		}
	}
	return []Stage{
		{Name: "scan", Run: run("scan")},
		{Name: "approve", Until: func(context.Context) (bool, error) { return c.gate, nil }, Run: run("approve")},
		{Name: "check", Run: run("check")},
		{Name: "bag", Run: run("bag"), Final: true},
		{Name: "tidy", Run: run("tidy")},
	}
}

type failure string

func (f failure) Error() string { return string(f) }

const errFail = failure("stage failed")

var day = NewBusinessDate(2026, time.October, 20)

func TestRewindReRunsTheStepAndLaterOnes(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	r := NewRunner(store, nil)
	c := &counted{gate: true}
	c.failAt = "check"
	if _, err := r.RunPipeline(ctx, "batch", "b1", day, c.stages()); err == nil {
		t.Fatal("check should fail")
	}
	c.failAt = ""
	if _, err := r.Rewind(ctx, "batch", "b1", "scan", c.stages()); err != nil {
		t.Fatal(err)
	}
	rec, err := r.RunPipeline(ctx, "batch", "b1", day, c.stages())
	if err != nil || rec.Status != StatusCompleted {
		t.Fatalf("after rewind: %+v, %v", rec, err)
	}
	if c.runs["scan"] != 2 || c.runs["approve"] != 2 || c.runs["check"] != 2 || c.runs["bag"] != 1 {
		t.Errorf("runs %v, want scan, approve and check twice, bag once", c.runs)
	}
}

func TestRewindAWaitingRun(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	r := NewRunner(store, nil)
	c := &counted{}
	rec, _ := r.RunPipeline(ctx, "batch", "b1", day, c.stages())
	if rec.Status != StatusWaiting {
		t.Fatalf("status %s, want waiting", rec.Status)
	}
	rec, err := r.Rewind(ctx, "batch", "b1", "scan", c.stages())
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status == StatusWaiting || rec.Status == StatusCompleted {
		t.Errorf("rewound run is %s", rec.Status)
	}
	if steps := stepNames(t, store, rec.ID); len(steps) != 0 {
		t.Errorf("steps left after rewinding to the first: %v", steps)
	}
	rec, _ = r.RunPipeline(ctx, "batch", "b1", day, c.stages())
	if rec.Status != StatusWaiting || c.runs["scan"] != 2 {
		t.Errorf("after rewind: %s, scan run %d times; want waiting, twice", rec.Status, c.runs["scan"])
	}
	if steps := stepNames(t, store, rec.ID); strings.Join(steps, ",") != "scan,approve" {
		t.Errorf("steps %v, want scan completed and approve waiting, recorded once each", steps)
	}
}

func TestRewindACompletedRunBeforeItsFinalStage(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	r := NewRunner(store, nil)
	c := &counted{gate: true}
	stages := c.stages()
	stages[3].Final = false // nothing final, so a completed run may be rewound
	if rec, _ := r.RunPipeline(ctx, "batch", "b1", day, stages); rec.Status != StatusCompleted {
		t.Fatalf("status %s", rec.Status)
	}
	first, _ := store.FindRunByKey(ctx, "batch", "b1")

	if _, err := r.Rewind(ctx, "batch", "b1", "check", stages); err != nil {
		t.Fatal(err)
	}
	again, _ := store.FindRunByKey(ctx, "batch", "b1")
	if again.ID != first.ID {
		t.Error("rewind made a new instance")
	}
	if again.Status == StatusCompleted {
		t.Error("rewound run still completed")
	}
	rec, err := r.RunPipeline(ctx, "batch", "b1", day, stages)
	if err != nil || rec.Status != StatusCompleted {
		t.Fatalf("after rewind: %+v, %v", rec, err)
	}
	if c.runs["scan"] != 1 || c.runs["check"] != 2 || c.runs["bag"] != 2 || c.runs["tidy"] != 2 {
		t.Errorf("runs %v, want scan once and check, bag, tidy twice", c.runs)
	}
}

// A final stage's work must not be redone: once it has completed, no step
// at or before it can be rewound to.
func TestRewindRefusals(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	r := NewRunner(store, nil)
	c := &counted{gate: true}
	r.RunPipeline(ctx, "batch", "b1", day, c.stages())

	cases := []struct {
		key, from, want string
	}{
		{"b1", "check", "final"},
		{"b1", "bag", "final"},
		{"b1", "wash", "no step"},
		{"b2", "scan", "no instance"},
	}
	for _, tc := range cases {
		_, err := r.Rewind(ctx, "batch", tc.key, tc.from, c.stages())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("rewind %s to %s: %v, want an error mentioning %q", tc.key, tc.from, err, tc.want)
		}
	}
	if _, err := r.Rewind(ctx, "batch", "b1", "tidy", c.stages()); err != nil {
		t.Errorf("rewind after the final stage: %v", err)
	}
}

func stepNames(t *testing.T, s Store, runID string) []string {
	t.Helper()
	steps, err := s.ListSteps(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, st := range steps {
		names = append(names, st.StepName)
	}
	return names
}
