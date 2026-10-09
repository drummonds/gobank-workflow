package workflow_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/runnertest"
)

// promotion is the worked example of docs/writing-workflows.md: build,
// wait for an approval, promote. The approval is a store write made
// elsewhere; approved stands in for reading it.
func promotion(approved *bool, log func(string)) []wf.Stage {
	return []wf.Stage{
		{Name: "build", Run: func(context.Context) error { log("building"); return nil }},
		{Name: "approve", Until: func(context.Context) (bool, error) { return *approved, nil }},
		{Name: "promote", Run: func(context.Context) error { log("promoting"); return nil }},
	}
}

// The host ticks the pipeline until the run is no longer waiting. Each
// tick re-enters the same attempt: build is not repeated, and the
// approval step is recorded once, when the wait began.
func ExampleRunner_RunPipeline() {
	ctx := context.Background()
	store := wf.NewMemStore()
	approved := false
	stages := promotion(&approved, func(s string) { fmt.Println(s) })
	runner := wf.NewRunner(store, nil)
	date := wf.NewBusinessDate(2026, time.October, 8)
	for tick := 1; ; tick++ {
		rec, err := runner.RunPipeline(ctx, "promotion", "v0.3.0", date, stages)
		if err != nil {
			fmt.Println("failed:", err)
			return
		}
		fmt.Printf("tick %d: %s\n", tick, rec.Status)
		if rec.Status != wf.StatusWaiting {
			break
		}
		approved = tick == 2 // the approval arrives between the second and third ticks
	}
	// Output:
	// building
	// tick 1: waiting
	// tick 2: waiting
	// promoting
	// tick 3: completed
}

// A workflow's test drives one key and reads the store back through
// runnertest: the one instance, and each step's status.
func TestPromotionRecordsEveryStep(t *testing.T) {
	ctx := context.Background()
	store := wf.NewMemStore()
	approved := false
	stages := promotion(&approved, func(string) {})
	runner := wf.NewRunner(store, nil)
	date := wf.NewBusinessDate(2026, time.October, 8)

	if _, err := runner.RunPipeline(ctx, "promotion", "v0.3.0", date, stages); err != nil {
		t.Fatal(err)
	}
	run := runnertest.OnlyRun(t, store)
	if run.Status != wf.StatusWaiting || runnertest.Steps(t, store, run.ID)["approve"] != wf.StatusWaiting {
		t.Fatalf("run %s, steps %v", run.Status, runnertest.Steps(t, store, run.ID))
	}

	approved = true
	if _, err := runner.RunPipeline(ctx, "promotion", "v0.3.0", date, stages); err != nil {
		t.Fatal(err)
	}
	run = runnertest.OnlyRun(t, store)
	steps := runnertest.Steps(t, store, run.ID)
	if run.Status != wf.StatusCompleted || len(steps) != 3 {
		t.Fatalf("run %s, steps %v", run.Status, steps)
	}
	for _, name := range []string{"build", "approve", "promote"} {
		if steps[name] != wf.StatusCompleted {
			t.Fatalf("step %s = %s", name, steps[name])
		}
	}
}
