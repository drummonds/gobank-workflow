package pipeline

import (
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

func TestNewPipelineHasNoReleaseInProgress(t *testing.T) {
	p := New()
	if p.Current() != nil {
		t.Fatal("expected no release in progress")
	}
	if got := p.NextVersion(); got != "v0.1.1" {
		t.Fatalf("NextVersion = %q, want v0.1.1", got)
	}
}

func TestStartCreatesReleaseWithAllStagesPending(t *testing.T) {
	p := New()
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	r := p.Current()
	if r == nil {
		t.Fatal("expected a release in progress")
	}
	if r.Version != "v0.1.1" {
		t.Fatalf("Version = %q, want v0.1.1", r.Version)
	}
	if r.Status != wf.StatusRunning {
		t.Fatalf("Status = %q, want running", r.Status)
	}
	if len(r.Stages) != len(Stages) {
		t.Fatalf("got %d stages, want %d", len(r.Stages), len(Stages))
	}
	for _, s := range r.Stages {
		if s.Status != wf.StatusPending {
			t.Fatalf("stage %s status = %q, want pending", s.Stage.Name, s.Status)
		}
	}
}

func TestStartWhileInProgressIsRejected(t *testing.T) {
	p := New()
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(); err == nil {
		t.Fatal("expected error starting a second release while one is in progress")
	}
}

func TestNextRunsStagesInOrderUntilComplete(t *testing.T) {
	p := New()
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	for i := range Stages {
		if err := p.Next(); err != nil {
			t.Fatalf("Next %d: %v", i, err)
		}
		r := p.Current()
		if i < len(Stages)-1 {
			if r == nil {
				t.Fatalf("after stage %d release should still be in progress", i)
			}
			if r.Stages[i].Status != wf.StatusCompleted {
				t.Fatalf("stage %d status = %q, want completed", i, r.Stages[i].Status)
			}
			if r.Stages[i+1].Status != wf.StatusPending {
				t.Fatalf("stage %d status = %q, want pending", i+1, r.Stages[i+1].Status)
			}
		}
	}
	if p.Current() != nil {
		t.Fatal("release should have finished")
	}
	h := p.History()
	if len(h) != 1 || h[0].Status != wf.StatusCompleted || h[0].Version != "v0.1.1" {
		t.Fatalf("history = %+v, want one completed v0.1.1", h)
	}
	if got := p.NextVersion(); got != "v0.1.2" {
		t.Fatalf("NextVersion = %q, want v0.1.2", got)
	}
}

func TestNextWithoutReleaseIsRejected(t *testing.T) {
	p := New()
	if err := p.Next(); err == nil {
		t.Fatal("expected error")
	}
}

func TestInjectedFailureStopsTheReleaseAtThatStage(t *testing.T) {
	p := New()
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	if err := p.Next(); err != nil { // check
		t.Fatal(err)
	}
	p.SetFailNext(true)
	if err := p.Next(); err != nil { // bump fails
		t.Fatal(err)
	}
	r := p.Current()
	if r == nil {
		t.Fatal("failed release should remain in progress so it can be resumed")
	}
	if r.Status != wf.StatusFailed {
		t.Fatalf("Status = %q, want failed", r.Status)
	}
	if r.Stages[1].Status != wf.StatusFailed || r.Stages[1].Error == "" {
		t.Fatalf("stage bump = %+v, want failed with an error", r.Stages[1])
	}
	if r.Stages[2].Status != wf.StatusPending {
		t.Fatalf("stage after failure = %q, want pending", r.Stages[2].Status)
	}
	if p.FailNext() {
		t.Fatal("fail-next should clear once consumed")
	}
}

func TestResumeRerunsOnlyTheFailedStage(t *testing.T) {
	p := New()
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	if err := p.Next(); err != nil {
		t.Fatal(err)
	}
	p.SetFailNext(true)
	if err := p.Next(); err != nil {
		t.Fatal(err)
	}
	if err := p.Next(); err != nil { // retry bump
		t.Fatal(err)
	}
	r := p.Current()
	if r.Status != wf.StatusRunning {
		t.Fatalf("Status = %q, want running after successful retry", r.Status)
	}
	if r.Stages[0].Attempts != 1 {
		t.Fatalf("check attempts = %d, want 1 (completed stages are not re-run)", r.Stages[0].Attempts)
	}
	if r.Stages[1].Attempts != 2 || r.Stages[1].Status != wf.StatusCompleted {
		t.Fatalf("bump = %+v, want completed on attempt 2", r.Stages[1])
	}
}

func TestRunToEndCompletesOrStopsAtFailure(t *testing.T) {
	p := New()
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	p.SetFailNext(true)
	if err := p.RunToEnd(); err != nil {
		t.Fatal(err)
	}
	if r := p.Current(); r == nil || r.Status != wf.StatusFailed || r.Stages[0].Status != wf.StatusFailed {
		t.Fatalf("expected release stopped at first stage, got %+v", r)
	}
	if err := p.RunToEnd(); err != nil {
		t.Fatal(err)
	}
	if p.Current() != nil {
		t.Fatal("release should have completed on resume")
	}
}

func TestResetForgetsEverything(t *testing.T) {
	p := New()
	_ = p.Start()
	_ = p.RunToEnd()
	_ = p.Start()
	p.Reset()
	if p.Current() != nil || len(p.History()) != 0 || p.NextVersion() != "v0.1.1" {
		t.Fatal("reset should return to the initial state")
	}
}

func TestEveryStageNamesItsImplementation(t *testing.T) {
	for _, s := range Stages {
		if s.Name == "" || s.Does == "" || s.Code == "" {
			t.Fatalf("stage %+v must have a name, what it does, and the code that runs it", s)
		}
	}
}
