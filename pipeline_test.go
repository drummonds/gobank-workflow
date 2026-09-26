package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// --- Definitions describe their steps -------------------------------------

func TestDefinitionsDescribeTheirSteps(t *testing.T) {
	byType := map[WorkflowType]Definition{}
	for _, d := range Definitions {
		byType[d.Type] = d
	}
	daily := byType[WorkflowDailyUpdate]
	if len(daily.Steps) != 1 || daily.Steps[0].Name != "end_of_day" {
		t.Fatalf("daily_update steps = %+v", daily.Steps)
	}
	rel, ok := byType[WorkflowRelease]
	if !ok {
		t.Fatal("release definition missing")
	}
	if len(rel.Steps) != len(ReleaseStages) {
		t.Fatalf("release definition has %d steps, want %d", len(rel.Steps), len(ReleaseStages))
	}
	for i, s := range rel.Steps {
		if s.Name == "" || s.Description == "" || s.Name != ReleaseStages[i] {
			t.Fatalf("release step %d = %+v", i, s)
		}
	}
	for _, d := range Definitions {
		for _, s := range d.Steps {
			if !strings.HasPrefix(s.Source, "https://") {
				t.Fatalf("%s step %s must link to its code, got %q", d.Type, s.Name, s.Source)
			}
		}
	}
}

// --- Runner: retry keeps the instance -------------------------------------

func TestRunDailyRetryKeepsTheInstanceID(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	proc := &failOnceProcessor{}
	r := NewRunner(store, proc)
	d := NewBusinessDate(2026, time.June, 1)
	if _, err := r.RunDaily(ctx, d); err == nil {
		t.Fatal("first attempt should fail")
	}
	failed, _ := store.FindRun(ctx, WorkflowDailyUpdate, d)
	if failed == nil || failed.Status != StatusFailed {
		t.Fatalf("expected a failed instance, got %+v", failed)
	}
	rec, err := r.RunDaily(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != failed.ID || rec.Status != StatusCompleted {
		t.Fatalf("retry should complete the same instance: was %s, got %+v", failed.ID, rec)
	}
	runs, _ := store.ListRuns(ctx, 10)
	if len(runs) != 1 {
		t.Fatalf("want one instance, got %d", len(runs))
	}
	steps, _ := store.ListSteps(ctx, rec.ID)
	if len(steps) != 1 || steps[0].Status != StatusCompleted {
		t.Fatalf("steps after retry = %+v", steps)
	}
}

type failOnceProcessor struct{ calls int }

func (p *failOnceProcessor) ProcessDay(_ context.Context, date BusinessDate) (DayResult, error) {
	p.calls++
	if p.calls == 1 {
		return DayResult{}, errors.New("first attempt fails")
	}
	return DayResult{Date: date, Accounts: []AccountResult{{AccountID: "a", OpeningBalance: 1, ClosingBalance: 1, Exponent: -2}}}, nil
}

// --- Runner: pipelines ----------------------------------------------------

func stagesWithFailure(failAt string, calls map[string]int, fail *bool) []Stage {
	var stages []Stage
	for _, name := range []string{"check", "tag", "push"} {
		stages = append(stages, Stage{Name: name, Run: func(context.Context) error {
			calls[name]++
			if name == failAt && *fail {
				*fail = false
				return errors.New(name + " broke")
			}
			return nil
		}})
	}
	return stages
}

func TestRunPipelineRecordsEveryStage(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	calls := map[string]int{}
	fail := false
	rec, err := NewRunner(store, nil).RunPipeline(ctx, WorkflowRelease, "v0.1.3", NewBusinessDate(2026, time.June, 2), stagesWithFailure("", calls, &fail))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != StatusCompleted || rec.Key != "v0.1.3" || rec.WorkflowType != WorkflowRelease {
		t.Fatalf("run = %+v", rec)
	}
	steps, _ := store.ListSteps(ctx, rec.ID)
	if len(steps) != 3 {
		t.Fatalf("want 3 steps, got %d", len(steps))
	}
	for i, name := range []string{"check", "tag", "push"} {
		if steps[i].StepName != name || steps[i].Status != StatusCompleted || steps[i].DurationNs < 0 {
			t.Fatalf("step %d = %+v", i, steps[i])
		}
	}
	// A completed key is a no-op afterwards.
	again, err := NewRunner(store, nil).RunPipeline(ctx, WorkflowRelease, "v0.1.3", NewBusinessDate(2026, time.June, 3), stagesWithFailure("", calls, &fail))
	if err != nil || again != nil {
		t.Fatalf("second run of a completed key: %+v, %v", again, err)
	}
	if calls["check"] != 1 {
		t.Fatalf("stages must not re-run for a completed key, check ran %d times", calls["check"])
	}
}

func TestRunPipelineStopsAtFailureAndResumesFromIt(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	calls := map[string]int{}
	fail := true
	r := NewRunner(store, nil)
	d := NewBusinessDate(2026, time.June, 2)
	_, err := r.RunPipeline(ctx, WorkflowRelease, "v0.1.3", d, stagesWithFailure("tag", calls, &fail))
	if err == nil {
		t.Fatal("expected the tag stage to fail")
	}
	failed, _ := store.FindRunByKey(ctx, WorkflowRelease, "v0.1.3")
	if failed == nil || failed.Status != StatusFailed || failed.Error == "" {
		t.Fatalf("expected a failed instance with an error, got %+v", failed)
	}
	steps, _ := store.ListSteps(ctx, failed.ID)
	if len(steps) != 2 || steps[0].Status != StatusCompleted || steps[1].StepName != "tag" || steps[1].Status != StatusFailed || steps[1].Error == "" {
		t.Fatalf("steps after failure = %+v", steps)
	}
	if calls["push"] != 0 {
		t.Fatal("stages after the failure must not run")
	}

	rec, err := r.RunPipeline(ctx, WorkflowRelease, "v0.1.3", d, stagesWithFailure("tag", calls, &fail))
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != failed.ID || rec.Status != StatusCompleted {
		t.Fatalf("retry should complete the same instance, got %+v", rec)
	}
	if calls["check"] != 1 || calls["tag"] != 2 || calls["push"] != 1 {
		t.Fatalf("resume must re-run only the failed stage onwards: %v", calls)
	}
	steps, _ = store.ListSteps(ctx, rec.ID)
	if len(steps) != 3 || steps[1].Status != StatusCompleted {
		t.Fatalf("steps after retry = %+v", steps)
	}
}
