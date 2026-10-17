// Package runnertest holds the checks a workflow's own tests make against
// the store after a run: the instance it produced and the status of each
// step. It is the counterpart of storetest for code built on the Runner.
package runnertest

import (
	"context"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// OnlyRun is the single instance in the store. A workflow test drives one
// key, so anything but exactly one run is a failure.
func OnlyRun(t testing.TB, s wf.Store) wf.RunRecord {
	t.Helper()
	runs, err := s.ListRuns(context.Background(), wf.RunFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("want exactly one run, got %d", len(runs))
	}
	return runs[0]
}

// Steps is the status of each step of a run by step name.
func Steps(t testing.TB, s wf.Store, runID string) map[string]wf.RunStatus {
	t.Helper()
	steps, err := s.ListSteps(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]wf.RunStatus{}
	for _, st := range steps {
		out[st.StepName] = st.Status
	}
	return out
}
