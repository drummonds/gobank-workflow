// Package storetest holds the Store contract checks shared by every store
// implementation's tests.
package storetest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// release is the arbitrary keyed type the checks run.
const release wf.WorkflowType = "release"

// KeyedInstances checks instance keys: several keyed runs may share a
// business date, FindRunByKey finds them, and a completed key is final.
func KeyedInstances(t *testing.T, s wf.Store) {
	t.Helper()
	ctx := context.Background()
	d := wf.NewBusinessDate(2026, time.May, 1)
	a := &wf.RunRecord{ID: "rel-a", WorkflowType: release, BusinessDate: d, Key: "v0.1.3", Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
	b := &wf.RunRecord{ID: "rel-b", WorkflowType: release, BusinessDate: d, Key: "v0.1.4", Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := s.CreateRun(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRun(ctx, b); err != nil {
		t.Fatalf("two releases on one date must both be accepted: %v", err)
	}
	got, err := s.FindRunByKey(ctx, release, "v0.1.4")
	if err != nil || got == nil || got.ID != "rel-b" || got.Key != "v0.1.4" {
		t.Fatalf("FindRunByKey = %+v, %v", got, err)
	}
	if got, _ := s.FindRunByKey(ctx, release, "v9.9.9"); got != nil {
		t.Fatalf("unknown key should be nil, got %+v", got)
	}
	if err := s.CompleteRun(ctx, "rel-a"); err != nil {
		t.Fatal(err)
	}
	again := &wf.RunRecord{ID: "rel-a2", WorkflowType: release, BusinessDate: d, Key: "v0.1.3", Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
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
	run := &wf.RunRecord{ID: "r1", WorkflowType: release, BusinessDate: d, Key: "v1", Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
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
	if err := s.RestartRun(ctx, "r1", wf.NewRevision([]string{"s1", "s2"})); err != nil {
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

// UndoneStep checks DeleteStep: a run's steps with that name go, the
// rest stay, and another run's step of the same name is untouched.
func UndoneStep(t *testing.T, s wf.Store) {
	t.Helper()
	ctx := context.Background()
	d := wf.NewBusinessDate(2026, time.October, 15)
	for _, id := range []string{"u1", "u2"} {
		run := &wf.RunRecord{ID: id, WorkflowType: release, BusinessDate: d, Key: id, Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
		if err := s.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"create", "add"} {
			if err := s.RecordStep(ctx, &wf.StepResult{ID: id + "-" + name, RunID: id, StepName: name, Status: wf.StatusCompleted, CreatedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := s.DeleteStep(ctx, "u1", "create"); err != nil {
		t.Fatal(err)
	}
	if steps, _ := s.ListSteps(ctx, "u1"); len(steps) != 1 || steps[0].StepName != "add" {
		t.Fatalf("u1 after DeleteStep create = %+v; want add only", steps)
	}
	if steps, _ := s.ListSteps(ctx, "u2"); len(steps) != 2 {
		t.Fatalf("u2 should be untouched, got %+v", steps)
	}
}

// WaitAndResume checks the waiting transitions: WaitRun parks a run,
// ResumeRun continues the same attempt, so StartedAt is kept where
// RestartRun would reset it.
func WaitAndResume(t *testing.T, s wf.Store) {
	t.Helper()
	ctx := context.Background()
	started := time.Date(2026, time.May, 3, 9, 0, 0, 0, time.UTC)
	run := &wf.RunRecord{ID: "w1", WorkflowType: release, BusinessDate: wf.NewBusinessDate(2026, time.May, 3), Key: "v2", Status: wf.StatusRunning, StartedAt: started, CreatedAt: started}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := s.WaitRun(ctx, "w1"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRun(ctx, "w1")
	if got.Status != wf.StatusWaiting || !got.CompletedAt.IsZero() || !got.StartedAt.Equal(started) {
		t.Fatalf("WaitRun should park the run, got %+v", got)
	}
	counts, _ := s.CountRuns(ctx)
	if counts[release][wf.StatusWaiting] != 1 {
		t.Fatalf("waiting runs should be counted, got %+v", counts)
	}
	if err := s.ResumeRun(ctx, "w1"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetRun(ctx, "w1")
	if got.Status != wf.StatusRunning || got.Error != "" || !got.StartedAt.Equal(started) {
		t.Fatalf("ResumeRun should continue the same attempt, got %+v", got)
	}
}

// Revisions checks that a run's revision round-trips through every read,
// and that RestartRun replaces it with the new attempt's.
func Revisions(t *testing.T, s wf.Store) {
	t.Helper()
	ctx := context.Background()
	first := wf.NewRevision([]string{"check", "push origin", "tag"})
	run := &wf.RunRecord{ID: "rev1", WorkflowType: release, BusinessDate: wf.NewBusinessDate(2026, time.October, 16), Key: "v3", Revision: first, Status: wf.StatusRunning, StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	same := func(where string, got wf.Revision, want wf.Revision) {
		t.Helper()
		if got.ID != want.ID || !slices.Equal(got.Steps, want.Steps) {
			t.Fatalf("%s revision = %+v, want %+v", where, got, want)
		}
	}
	got, _ := s.GetRun(ctx, "rev1")
	same("GetRun", got.Revision, first)
	got, _ = s.FindRunByKey(ctx, release, "v3")
	same("FindRunByKey", got.Revision, first)
	runs, _ := s.ListRuns(ctx, wf.RunFilter{Limit: 10})
	for _, r := range runs {
		if r.ID == "rev1" {
			same("ListRuns", r.Revision, first)
		}
	}
	if err := s.FailRun(ctx, "rev1", "tag: boom"); err != nil {
		t.Fatal(err)
	}
	next := wf.NewRevision([]string{"check", "changelog", "tag"})
	if err := s.RestartRun(ctx, "rev1", next); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetRun(ctx, "rev1")
	same("after RestartRun", got.Revision, next)
}

// FilteredRuns checks ListRuns under a RunFilter: each field narrows the
// runs, fields combine, the newest come first (active before completed
// when asked), and Offset and Limit page them. s must
// start empty.
func FilteredRuns(t *testing.T, s wf.Store) {
	t.Helper()
	ctx := context.Background()
	t0 := time.Date(2026, time.March, 1, 9, 0, 0, 0, time.UTC)
	revA, revB := wf.NewRevision([]string{"build"}), wf.NewRevision([]string{"build", "tag"})
	for _, r := range []wf.RunRecord{
		{ID: "f1", WorkflowType: "ship", Key: "v1.0.0", Revision: revA, Status: wf.StatusCompleted, CreatedAt: t0},
		{ID: "f5", WorkflowType: "close", Status: wf.StatusCompleted, CreatedAt: t0.Add(48 * time.Hour)},
		{ID: "f2", WorkflowType: "ship", Key: "V1.1.0", Revision: revB, Status: wf.StatusFailed, CreatedAt: t0.Add(time.Hour)},
		{ID: "f3", WorkflowType: "close", Status: wf.StatusRunning, CreatedAt: t0.Add(2 * time.Hour)},
		{ID: "f4", WorkflowType: "ship", Key: "v2.0.0_rc1", Revision: revB, Status: wf.StatusWaiting, CreatedAt: t0.Add(24 * time.Hour)},
	} {
		r.BusinessDate = wf.BusinessDateFrom(r.CreatedAt)
		r.StartedAt = r.CreatedAt
		if err := s.CreateRun(ctx, &r); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name string
		f    wf.RunFilter
		want []string
	}{
		{"no filter is every run, newest first", wf.RunFilter{}, []string{"f5", "f4", "f3", "f2", "f1"}},
		{"limit keeps the newest", wf.RunFilter{Limit: 2}, []string{"f5", "f4"}},
		{"types", wf.RunFilter{Types: []wf.WorkflowType{"ship"}}, []string{"f4", "f2", "f1"}},
		{"statuses", wf.RunFilter{Statuses: []wf.RunStatus{wf.StatusFailed, wf.StatusWaiting}}, []string{"f4", "f2"}},
		{"since is created at or after", wf.RunFilter{Since: t0.Add(time.Hour)}, []string{"f5", "f4", "f3", "f2"}},
		{"key contains, ignoring case", wf.RunFilter{KeyContains: "v1."}, []string{"f2", "f1"}},
		{"key contains takes _ literally", wf.RunFilter{KeyContains: "0_r"}, []string{"f4"}},
		{"key contains takes % literally", wf.RunFilter{KeyContains: "%"}, nil},
		{"revision", wf.RunFilter{Revision: revB.ID}, []string{"f4", "f2"}},
		{"offset skips the newest", wf.RunFilter{Offset: 1, Limit: 2}, []string{"f4", "f3"}},
		{"offset without limit", wf.RunFilter{Offset: 3}, []string{"f2", "f1"}},
		{"active first, then completed, each newest first", wf.RunFilter{ActiveFirst: true}, []string{"f4", "f3", "f2", "f5", "f1"}},
		{"active first pages across the completed", wf.RunFilter{ActiveFirst: true, Types: []wf.WorkflowType{"ship"}, Offset: 1, Limit: 2}, []string{"f2", "f1"}},
		{"fields combine", wf.RunFilter{Types: []wf.WorkflowType{"ship"}, Statuses: []wf.RunStatus{wf.StatusCompleted, wf.StatusWaiting}, Limit: 1}, []string{"f4"}},
	} {
		runs, err := s.ListRuns(ctx, c.f)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var got []string
		for _, r := range runs {
			got = append(got, r.ID)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// TimeOrder checks that ListRuns and ListSteps order by the time a record
// was created, not by how its stamp happens to print: 05.5 is before 05.52.
func TimeOrder(t *testing.T, s wf.Store) {
	t.Helper()
	ctx := context.Background()
	t0 := time.Date(2026, time.April, 1, 9, 0, 5, 0, time.UTC)
	for _, r := range []wf.RunRecord{
		{ID: "o1", WorkflowType: "ship", Key: "a", CreatedAt: t0.Add(500 * time.Millisecond)},
		{ID: "o2", WorkflowType: "ship", Key: "b", CreatedAt: t0.Add(520 * time.Millisecond)},
		{ID: "o0", WorkflowType: "ship", Key: "c", CreatedAt: t0},
	} {
		r.BusinessDate = wf.BusinessDateFrom(r.CreatedAt)
		r.Status = wf.StatusRunning
		r.StartedAt = r.CreatedAt
		if err := s.CreateRun(ctx, &r); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := s.ListRuns(ctx, wf.RunFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var gotRuns []string
	for _, r := range runs {
		gotRuns = append(gotRuns, r.ID)
	}
	if want := []string{"o2", "o1", "o0"}; !slices.Equal(gotRuns, want) {
		t.Errorf("runs: got %v, want %v", gotRuns, want)
	}

	for _, st := range []wf.StepResult{
		{ID: "s1", StepName: "build", CreatedAt: t0.Add(500 * time.Millisecond)},
		{ID: "s2", StepName: "tag", CreatedAt: t0.Add(520 * time.Millisecond)},
		{ID: "s0", StepName: "check", CreatedAt: t0},
	} {
		st.RunID = "o1"
		if err := s.RecordStep(ctx, &st); err != nil {
			t.Fatal(err)
		}
	}
	steps, err := s.ListSteps(ctx, "o1")
	if err != nil {
		t.Fatal(err)
	}
	var gotSteps []string
	for _, st := range steps {
		gotSteps = append(gotSteps, st.ID)
	}
	if want := []string{"s0", "s1", "s2"}; !slices.Equal(gotSteps, want) {
		t.Errorf("steps: got %v, want %v", gotSteps, want)
	}
}

// Cancelled checks CancelRun: the run ends cancelled, the transitions a
// runner makes leave it so, RestartRun (a rewind) revives it, and
// ActiveFirst lists it with the completed. s must start empty.
func Cancelled(t *testing.T, s wf.Store) {
	t.Helper()
	ctx := context.Background()
	t0 := time.Date(2026, time.October, 21, 9, 0, 0, 0, time.UTC)
	for _, r := range []wf.RunRecord{
		{ID: "x1", WorkflowType: "ship", Key: "v1", Status: wf.StatusFailed, Error: "tag: boom", CreatedAt: t0},
		{ID: "x2", WorkflowType: "ship", Key: "v2", Status: wf.StatusCompleted, CreatedAt: t0.Add(time.Hour)},
		{ID: "x3", WorkflowType: "ship", Key: "v3", Status: wf.StatusWaiting, CreatedAt: t0.Add(2 * time.Hour)},
	} {
		r.BusinessDate = wf.BusinessDateFrom(r.CreatedAt)
		r.StartedAt = r.CreatedAt
		if err := s.CreateRun(ctx, &r); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CancelRun(ctx, "x1"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRun(ctx, "x1")
	if got.Status != wf.StatusCancelled || got.CompletedAt.IsZero() || got.Error != "tag: boom" {
		t.Fatalf("CancelRun should end the run cancelled, keeping its error, got %+v", got)
	}
	runs, _ := s.ListRuns(ctx, wf.RunFilter{ActiveFirst: true})
	var order []string
	for _, r := range runs {
		order = append(order, r.ID)
	}
	if want := []string{"x3", "x2", "x1"}; !slices.Equal(order, want) {
		t.Fatalf("active first should list cancelled with completed: got %v, want %v", order, want)
	}
	for name, transition := range map[string]func() error{
		"CompleteRun": func() error { return s.CompleteRun(ctx, "x1") },
		"FailRun":     func() error { return s.FailRun(ctx, "x1", "late") },
		"WaitRun":     func() error { return s.WaitRun(ctx, "x1") },
		"ResumeRun":   func() error { return s.ResumeRun(ctx, "x1") },
	} {
		if err := transition(); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.GetRun(ctx, "x1"); got.Status != wf.StatusCancelled || got.Error != "tag: boom" {
			t.Fatalf("%s should leave a cancelled run as it is, got %+v", name, got)
		}
	}
	if err := s.RestartRun(ctx, "x1", wf.NewRevision([]string{"tag"})); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetRun(ctx, "x1"); got.Status != wf.StatusRunning {
		t.Fatalf("RestartRun should revive a cancelled run, got %+v", got)
	}
}

// Children checks the parent link: ParentID round-trips, ListRuns by
// ParentID is a run's direct children, and CountDescendants counts every
// run under it, at any depth, by status. s must start empty.
func Children(t *testing.T, s wf.Store) {
	t.Helper()
	ctx := context.Background()
	t0 := time.Date(2026, time.October, 24, 9, 0, 0, 0, time.UTC)
	for i, r := range []wf.RunRecord{
		{ID: "batch", WorkflowType: "batch", Key: "B-1", Status: wf.StatusRunning},
		{ID: "load1", WorkflowType: "load", Key: "B-1/1", ParentID: "batch", Status: wf.StatusCompleted},
		{ID: "load2", WorkflowType: "load", Key: "B-1/2", ParentID: "batch", Status: wf.StatusRunning},
		{ID: "photo1", WorkflowType: "photo", Key: "P-1", ParentID: "load1", Status: wf.StatusCompleted},
		{ID: "photo2", WorkflowType: "photo", Key: "P-2", ParentID: "load1", Status: wf.StatusFailed},
		{ID: "other", WorkflowType: "batch", Key: "B-2", Status: wf.StatusRunning},
	} {
		r.CreatedAt = t0.Add(time.Duration(i) * time.Minute)
		r.StartedAt = r.CreatedAt
		r.BusinessDate = wf.BusinessDateFrom(t0)
		if err := s.CreateRun(ctx, &r); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := s.GetRun(ctx, "load1")
	if got.ParentID != "batch" {
		t.Fatalf("ParentID not round-tripped: %+v", got)
	}
	ids := func(f wf.RunFilter) []string {
		runs, err := s.ListRuns(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range runs {
			out = append(out, r.ID)
			if r.ParentID != f.ParentID {
				t.Fatalf("ListRuns(%+v) returned %+v", f, r)
			}
		}
		return out
	}
	if got := ids(wf.RunFilter{ParentID: "batch"}); !slices.Equal(got, []string{"load2", "load1"}) {
		t.Fatalf("children of batch = %v, want the direct children, newest first", got)
	}
	if got := ids(wf.RunFilter{ParentID: "batch", Statuses: []wf.RunStatus{wf.StatusRunning}}); !slices.Equal(got, []string{"load2"}) {
		t.Fatalf("running children of batch = %v", got)
	}
	if got := ids(wf.RunFilter{ParentID: "other"}); len(got) != 0 {
		t.Fatalf("other has no children, got %v", got)
	}
	for id, want := range map[string]wf.RunCounts{
		"batch": {wf.StatusCompleted: 2, wf.StatusRunning: 1, wf.StatusFailed: 1},
		"load1": {wf.StatusCompleted: 1, wf.StatusFailed: 1},
		"other": {},
	} {
		got, err := s.CountDescendants(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Total() != want.Total() {
			t.Fatalf("descendants of %s = %v, want %v", id, got, want)
		}
		for st, n := range want {
			if got[st] != n {
				t.Fatalf("descendants of %s = %v, want %v", id, got, want)
			}
		}
	}
}
