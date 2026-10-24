package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// release is the arbitrary pipeline type the tests run.
const release WorkflowType = "release"

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
	runs, _ := store.ListRuns(ctx, RunFilter{Limit: 10})
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
	rec, err := NewRunner(store, nil).RunPipeline(ctx, release, "v0.1.3", NewBusinessDate(2026, time.June, 2), stagesWithFailure("", calls, &fail))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != StatusCompleted || rec.Key != "v0.1.3" || rec.WorkflowType != release {
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
	again, err := NewRunner(store, nil).RunPipeline(ctx, release, "v0.1.3", NewBusinessDate(2026, time.June, 3), stagesWithFailure("", calls, &fail))
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
	_, err := r.RunPipeline(ctx, release, "v0.1.3", d, stagesWithFailure("tag", calls, &fail))
	if err == nil {
		t.Fatal("expected the tag stage to fail")
	}
	failed, _ := store.FindRunByKey(ctx, release, "v0.1.3")
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

	rec, err := r.RunPipeline(ctx, release, "v0.1.3", d, stagesWithFailure("tag", calls, &fail))
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

// --- Runner: waiting stages -----------------------------------------------

func TestRunPipelineWaitsOnAConditionUntilItHolds(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	calls := map[string]int{}
	approved := false
	stages := []Stage{
		{Name: "check", Run: func(context.Context) error { calls["check"]++; return nil }},
		{Name: "approve", Until: func(context.Context) (bool, error) { calls["approve"]++; return approved, nil }},
		{Name: "push", Run: func(context.Context) error { calls["push"]++; return nil }},
	}
	r := NewRunner(store, nil)
	d := NewBusinessDate(2026, time.June, 2)

	rec, err := r.RunPipeline(ctx, release, "v0.1.4", d, stages)
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil || rec.Status != StatusWaiting {
		t.Fatalf("a false condition should leave the run waiting, got %+v", rec)
	}
	if calls["push"] != 0 {
		t.Fatal("stages after the waiting one must not run")
	}
	steps, _ := store.ListSteps(ctx, rec.ID)
	if len(steps) != 2 || steps[0].Status != StatusCompleted || steps[1].StepName != "approve" || steps[1].Status != StatusWaiting {
		t.Fatalf("steps while waiting = %+v", steps)
	}
	waitingSince := steps[1].CreatedAt
	startedAt := rec.StartedAt

	// A tick while the condition still fails: same instance, same attempt, no new step.
	again, err := r.RunPipeline(ctx, release, "v0.1.4", d, stages)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != rec.ID || again.Status != StatusWaiting || !again.StartedAt.Equal(startedAt) {
		t.Fatalf("re-entering a waiting run must continue the same attempt, got %+v", again)
	}
	if calls["check"] != 1 || calls["approve"] != 2 {
		t.Fatalf("only the condition should be re-evaluated: %v", calls)
	}
	steps, _ = store.ListSteps(ctx, rec.ID)
	if len(steps) != 2 || !steps[1].CreatedAt.Equal(waitingSince) {
		t.Fatalf("the waiting step should be kept, not re-recorded: %+v", steps)
	}

	approved = true
	done, err := r.RunPipeline(ctx, release, "v0.1.4", d, stages)
	if err != nil {
		t.Fatal(err)
	}
	if done.ID != rec.ID || done.Status != StatusCompleted || !done.StartedAt.Equal(startedAt) {
		t.Fatalf("once the condition holds the same attempt completes, got %+v", done)
	}
	if calls["check"] != 1 || calls["approve"] != 3 || calls["push"] != 1 {
		t.Fatalf("calls after completion = %v", calls)
	}
	steps, _ = store.ListSteps(ctx, rec.ID)
	if len(steps) != 3 {
		t.Fatalf("want 3 steps, got %+v", steps)
	}
	for _, s := range steps {
		if s.Status != StatusCompleted {
			t.Fatalf("every step should be completed, got %+v", s)
		}
	}
}

func TestRunPipelineConditionGatesTheStagesRun(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	ran := 0
	ready := false
	stages := []Stage{{Name: "promote",
		Until: func(context.Context) (bool, error) { return ready, nil },
		Run:   func(context.Context) error { ran++; return nil },
	}}
	r := NewRunner(store, nil)
	d := NewBusinessDate(2026, time.June, 2)
	if rec, err := r.RunPipeline(ctx, release, "v1", d, stages); err != nil || rec.Status != StatusWaiting || ran != 0 {
		t.Fatalf("Run must not execute before the condition holds: %+v, %v, ran %d", rec, err, ran)
	}
	ready = true
	if rec, err := r.RunPipeline(ctx, release, "v1", d, stages); err != nil || rec.Status != StatusCompleted || ran != 1 {
		t.Fatalf("Run should execute once the condition holds: %+v, %v, ran %d", rec, err, ran)
	}
}

func TestRunPipelineConditionErrorFailsTheStage(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	stages := []Stage{{Name: "approve", Until: func(context.Context) (bool, error) { return false, errors.New("approvals down") }}}
	_, err := NewRunner(store, nil).RunPipeline(ctx, release, "v1", NewBusinessDate(2026, time.June, 2), stages)
	if err == nil || !strings.Contains(err.Error(), "approvals down") {
		t.Fatalf("a condition error should fail the run, got %v", err)
	}
	run, _ := store.FindRunByKey(ctx, release, "v1")
	if run == nil || run.Status != StatusFailed {
		t.Fatalf("run = %+v", run)
	}
	steps, _ := store.ListSteps(ctx, run.ID)
	if len(steps) != 1 || steps[0].Status != StatusFailed || steps[0].Error != "approvals down" {
		t.Fatalf("steps = %+v", steps)
	}
}

// --- Runner: compensation -------------------------------------------------

// undoStages is create, add, days and remove: create and add can be undone
// and record it in order; days fails while *fail is set.
func undoStages(calls map[string]int, undone *[]string, fail *bool, undoErr error) []Stage {
	record := func(name string) func(context.Context) error {
		return func(context.Context) error { calls[name]++; return nil }
	}
	undo := func(name string) func(context.Context) error {
		return func(context.Context) error {
			*undone = append(*undone, name)
			if name == "add" && undoErr != nil {
				return undoErr
			}
			return nil
		}
	}
	return []Stage{
		{Name: "create", Run: record("create"), Undo: undo("create")},
		{Name: "add", Run: record("add"), Undo: undo("add")},
		{Name: "days", Run: func(context.Context) error {
			calls["days"]++
			if *fail {
				*fail = false
				return errors.New("the demo stopped answering")
			}
			return nil
		}},
		{Name: "remove", Run: record("remove")},
	}
}

// When a stage fails, every completed stage that can be undone is undone
// in reverse, and its step goes with it, so the next attempt starts from
// the first stage whose work no longer stands. The run's error says what
// failed and what was undone.
func TestRunPipelineUndoesCompletedStagesOnFailure(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	calls, fail := map[string]int{}, true
	var undone []string
	r := NewRunner(store, nil)
	d := NewBusinessDate(2026, time.October, 15)
	_, err := r.RunPipeline(ctx, release, "perf large", d, undoStages(calls, &undone, &fail, nil))
	if err == nil || !strings.Contains(err.Error(), "days: the demo stopped answering") || !strings.Contains(err.Error(), "undone: add, create") {
		t.Fatalf("err = %v; want the failure and what was undone", err)
	}
	if strings.Join(undone, " ") != "add create" {
		t.Errorf("undone = %v; want the completed stages in reverse", undone)
	}
	failed, _ := store.FindRunByKey(ctx, release, "perf large")
	if failed == nil || failed.Status != StatusFailed || !strings.Contains(failed.Error, "undone: add, create") {
		t.Fatalf("run = %+v; want failed, saying what was undone", failed)
	}
	steps, _ := store.ListSteps(ctx, failed.ID)
	if len(steps) != 1 || steps[0].StepName != "days" || steps[0].Status != StatusFailed {
		t.Fatalf("steps after failure = %+v; want only the failed step, the undone ones gone", steps)
	}
	if calls["remove"] != 0 {
		t.Error("stages after the failure must not run")
	}

	rec, err := r.RunPipeline(ctx, release, "perf large", d, undoStages(calls, &undone, &fail, nil))
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != failed.ID || rec.Status != StatusCompleted {
		t.Fatalf("retry should complete the same instance, got %+v", rec)
	}
	if calls["create"] != 2 || calls["add"] != 2 || calls["days"] != 2 || calls["remove"] != 1 {
		t.Errorf("the next attempt starts from the first undone stage: %v", calls)
	}
	steps, _ = store.ListSteps(ctx, rec.ID)
	if len(steps) != 4 {
		t.Errorf("steps after retry = %+v; want every stage recorded once", steps)
	}
}

// A stage without an Undo keeps its step, as before: the next attempt
// resumes after it, and the undoing of later stages does not reach past
// it in what is reported.
func TestRunPipelineKeepsStagesWithoutUndo(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	calls, fail := map[string]int{}, true
	var undone []string
	stages := undoStages(calls, &undone, &fail, nil)
	stages[0].Undo = nil // create is kept
	r := NewRunner(store, nil)
	d := NewBusinessDate(2026, time.October, 15)
	_, err := r.RunPipeline(ctx, release, "perf large", d, stages)
	if err == nil || !strings.Contains(err.Error(), "undone: add") || strings.Contains(err.Error(), "create") {
		t.Fatalf("err = %v; want add undone and create not mentioned", err)
	}
	failed, _ := store.FindRunByKey(ctx, release, "perf large")
	steps, _ := store.ListSteps(ctx, failed.ID)
	if len(steps) != 2 || steps[0].StepName != "create" || steps[0].Status != StatusCompleted || steps[1].StepName != "days" {
		t.Fatalf("steps after failure = %+v; want create kept and days failed", steps)
	}
	stages = undoStages(calls, &undone, &fail, nil)
	stages[0].Undo = nil
	if _, err := r.RunPipeline(ctx, release, "perf large", d, stages); err != nil {
		t.Fatal(err)
	}
	if calls["create"] != 1 || calls["add"] != 2 {
		t.Errorf("the next attempt resumes after the kept stage: %v", calls)
	}
}

// An undo that fails is reported with the failure, keeps its step since
// its work may still stand, and stops the undoing there: the stages
// before it are built on by it and are not touched.
func TestRunPipelineReportsAnUndoThatFails(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	calls, fail := map[string]int{}, true
	var undone []string
	r := NewRunner(store, nil)
	d := NewBusinessDate(2026, time.October, 15)
	_, err := r.RunPipeline(ctx, release, "perf large", d, undoStages(calls, &undone, &fail, errors.New("delete server: forbidden")))
	if err == nil || !strings.Contains(err.Error(), "days: the demo stopped answering") || !strings.Contains(err.Error(), "undo add: delete server: forbidden") {
		t.Fatalf("err = %v; want the failure and the undo's error", err)
	}
	if strings.Join(undone, " ") != "add" {
		t.Errorf("undone = %v; want the undoing stopped at the one that failed", undone)
	}
	failed, _ := store.FindRunByKey(ctx, release, "perf large")
	steps, _ := store.ListSteps(ctx, failed.ID)
	if len(steps) != 3 || steps[0].StepName != "create" || steps[1].StepName != "add" || steps[1].Status != StatusCompleted || steps[2].Status != StatusFailed {
		t.Fatalf("steps after failure = %+v; want create and add kept, days failed", steps)
	}
}

// --- Runner: a cancelled instance ------------------------------------------

func TestACancelledInstanceIsNotRunAgain(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	calls := map[string]int{}
	fail := true
	r := NewRunner(store, nil)
	d := NewBusinessDate(2026, time.June, 2)
	if _, err := r.RunPipeline(ctx, release, "v0.1.3", d, stagesWithFailure("tag", calls, &fail)); err == nil {
		t.Fatal("expected the tag stage to fail")
	}
	failed, _ := store.FindRunByKey(ctx, release, "v0.1.3")
	if err := store.CancelRun(ctx, failed.ID); err != nil {
		t.Fatal(err)
	}
	again, err := r.RunPipeline(ctx, release, "v0.1.3", d, stagesWithFailure("tag", calls, &fail))
	if err != nil || again != nil {
		t.Fatalf("a cancelled key should be a no-op: %+v, %v", again, err)
	}
	if calls["check"] != 1 || calls["tag"] != 1 {
		t.Fatalf("stages must not re-run for a cancelled key: %v", calls)
	}

	proc := &failOnceProcessor{}
	daily := NewRunner(store, proc)
	if _, err := daily.RunDaily(ctx, d); err == nil {
		t.Fatal("first attempt should fail")
	}
	day, _ := store.FindRun(ctx, WorkflowDailyUpdate, d)
	if err := store.CancelRun(ctx, day.ID); err != nil {
		t.Fatal(err)
	}
	if rec, err := daily.RunDaily(ctx, d); err != nil || rec != nil || proc.calls != 1 {
		t.Fatalf("a cancelled date should be a no-op: %+v, %v, %d calls", rec, err, proc.calls)
	}
}

// --- Child runs: a sub-process stage starts them, the tick runs them ------

func TestStartChildRecordsAPendingRunUnderItsParentForTheTickToRun(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	r := NewRunner(store, nil)
	d := NewBusinessDate(2026, time.October, 24)
	// The sub-process stage reads back its own run and starts two loads
	// under it; it does not wait for them.
	loads := Stage{Name: "loads", Run: func(ctx context.Context) error {
		batch, _ := store.FindRunByKey(ctx, "batch", "B-1")
		for _, key := range []string{"B-1/1", "B-1/2"} {
			if _, err := r.StartChild(ctx, batch.ID, "load", key, d); err != nil {
				return err
			}
		}
		return nil
	}}
	batch, err := r.RunPipeline(ctx, "batch", "B-1", d, []Stage{{Name: "describe"}, loads})
	if err != nil || batch.Status != StatusCompleted {
		t.Fatalf("the parent should complete without waiting for its children: %+v, %v", batch, err)
	}
	children, _ := store.ListRuns(ctx, RunFilter{ParentID: batch.ID})
	if len(children) != 2 {
		t.Fatalf("children = %+v, want two", children)
	}
	for _, c := range children {
		if c.WorkflowType != "load" || c.ParentID != batch.ID || c.Status != StatusPending || !c.StartedAt.IsZero() {
			t.Fatalf("a started child is pending under its parent, not yet attempted: %+v", c)
		}
	}
	// The host's tick runs a child by its key: the same instance, still the parent's.
	load, err := r.RunPipeline(ctx, "load", "B-1/1", d, []Stage{{Name: "preview"}})
	if err != nil || load.Status != StatusCompleted || load.ParentID != batch.ID {
		t.Fatalf("running the child: %+v, %v", load, err)
	}
	if load.ID != children[0].ID && load.ID != children[1].ID {
		t.Fatalf("RunPipeline should run the started child, not make a new one: %s", load.ID)
	}
	// Starting it again, as a stage run again would, leaves it as it is.
	again, err := r.StartChild(ctx, batch.ID, "load", "B-1/1", d)
	if err != nil || again.ID != load.ID || again.Status != StatusCompleted {
		t.Fatalf("StartChild on an existing key should return it unchanged: %+v, %v", again, err)
	}
	counts, _ := store.CountDescendants(ctx, batch.ID)
	if counts[StatusCompleted] != 1 || counts[StatusPending] != 1 || counts.Total() != 2 {
		t.Fatalf("descendants = %v", counts)
	}
	if _, err := r.StartChild(ctx, "no-such-run", "load", "B-9/1", d); err == nil {
		t.Fatal("a child needs a parent that exists")
	}
}
