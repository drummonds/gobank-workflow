package workflow

import (
	"context"
	"fmt"
	"slices"
)

// Rewind sends the instance of wfType identified by key back to the stage
// named from: that stage's step and every later one are removed and the
// run starts a new attempt, so the next RunPipeline runs them again and
// the earlier stages are not repeated. It works on a waiting, failed or
// completed instance. It refuses when a Final stage at or after from has
// completed, since that work must not be redone. stages are the pipeline
// as RunPipeline is given it.
func (r *Runner) Rewind(ctx context.Context, wfType WorkflowType, key, from string, stages []Stage) (*RunRecord, error) {
	run, err := r.store.FindRunByKey(ctx, wfType, key)
	if err != nil {
		return nil, fmt.Errorf("finding %s %s: %w", wfType, key, err)
	}
	if run == nil {
		return nil, fmt.Errorf("%s %s: no instance to rewind", wfType, key)
	}
	at := slices.IndexFunc(stages, func(st Stage) bool { return st.Name == from })
	if at < 0 {
		return nil, fmt.Errorf("%s %s: no step %q to rewind to", wfType, key, from)
	}
	steps, err := r.store.ListSteps(ctx, run.ID)
	if err != nil {
		return nil, fmt.Errorf("reading steps: %w", err)
	}
	done := map[string]bool{}
	for _, s := range steps {
		if s.Status == StatusCompleted {
			done[s.StepName] = true
		}
	}
	for _, st := range stages[at:] {
		if st.Final && done[st.Name] {
			return nil, fmt.Errorf("%s %s: %s is final and done, so nothing from %s on can be redone", wfType, key, st.Name, from)
		}
	}
	for _, st := range stages[at:] {
		if err := r.store.DeleteStep(ctx, run.ID, st.Name); err != nil {
			return nil, fmt.Errorf("clearing step %s: %w", st.Name, err)
		}
	}
	names := make([]string, len(stages))
	for i, st := range stages {
		names[i] = st.Name
	}
	if err := r.store.RestartRun(ctx, run.ID, NewRevision(names)); err != nil {
		return nil, fmt.Errorf("restarting run: %w", err)
	}
	return r.store.GetRun(ctx, run.ID)
}
