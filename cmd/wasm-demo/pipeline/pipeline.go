// Package pipeline models the release pipeline that ships a new version of
// gobank, for the workflow demo. It is a self-contained state machine:
// stages run in order, a failure halts the release at that stage, and
// resuming re-runs only the failed stage — completed stages are never
// repeated. It borrows the engine's run-status vocabulary so the model can
// be lifted into gobank-workflow once multi-step orchestration lands.
package pipeline

import (
	"errors"
	"fmt"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// Stage is one step in releasing a new gobank version.
type Stage struct {
	Name string
	Does string // what the stage does, shown in the UI
	Code string // the task-plus code that runs it for real under `tp release`
}

// Stages is the release pipeline in order, as `tp release` runs it
// (task-plus internal/workflow). release-pipeline.d2 draws the same list.
var Stages = []Stage{
	{"check", "task check — fmt, vet, test", "workflow.Run → runCmds(cfg.Check)"},
	{"bump", "choose the next semantic version", "workflow.Gather → version.BumpPastRetracted"},
	{"changelog", "move [Unreleased] under the new version heading", "changelog.Update"},
	{"tag", "commit and create the annotated git tag", "git.Commit → git.Tag"},
	{"push origin", "push commit and tag to origin (git.bytestone.uk)", `git.PushTo(dir, "origin")`},
	{"push mirror", "push commit and tag to GitHub", `git.PushTo(dir, "github")`},
	{"verify install", "go install the tag from the module proxy", "GOPROXY=direct go install → pokeGoProxy"},
	{"deploy docs", "task docs:build, then rsync docs/ to docs.bytestone.uk", "runCmds(pages_build) → deploy.Rsync.Deploy"},
}

// StageRun is the record of one stage within a release.
type StageRun struct {
	Stage    Stage
	Status   wf.RunStatus
	Error    string
	Attempts int
}

// Release is one run of the pipeline for a version.
type Release struct {
	Version string
	Status  wf.RunStatus
	Stages  []StageRun
}

// nextStage returns the index of the first stage that is not completed,
// or -1 when every stage is done.
func (r *Release) nextStage() int {
	for i, s := range r.Stages {
		if s.Status != wf.StatusCompleted {
			return i
		}
	}
	return -1
}

// ErrInProgress is returned by Start while a release is still in progress.
var ErrInProgress = errors.New("pipeline: a release is already in progress")

// ErrNoRelease is returned by Next when nothing has been started.
var ErrNoRelease = errors.New("pipeline: no release in progress")

// Pipeline holds the demo state: the release in progress, the ones that
// have finished, and the next version to ship.
type Pipeline struct {
	major, minor, patch int
	current             *Release
	history             []Release
	failNext            bool
	log                 []string
}

// New returns a pipeline whose last shipped version is v0.1.0.
func New() *Pipeline {
	p := &Pipeline{}
	p.Reset()
	return p
}

// Reset returns to the initial state.
func (p *Pipeline) Reset() {
	p.major, p.minor, p.patch = 0, 1, 0
	p.current = nil
	p.history = nil
	p.failNext = false
	p.log = nil
}

// NextVersion is the version the next release will ship.
func (p *Pipeline) NextVersion() string {
	return fmt.Sprintf("v%d.%d.%d", p.major, p.minor, p.patch+1)
}

// Current is the release in progress, or nil.
func (p *Pipeline) Current() *Release { return p.current }

// History lists finished releases, most recent first.
func (p *Pipeline) History() []Release { return p.history }

// Log lists events, oldest first.
func (p *Pipeline) Log() []string { return p.log }

// FailNext reports whether the next stage to run has been set to fail.
func (p *Pipeline) FailNext() bool { return p.failNext }

// SetFailNext arms (or disarms) an injected failure for the next stage run.
func (p *Pipeline) SetFailNext(fail bool) { p.failNext = fail }

// Start begins a release of the next version.
func (p *Pipeline) Start() error {
	if p.current != nil {
		return ErrInProgress
	}
	r := &Release{Version: p.NextVersion(), Status: wf.StatusRunning}
	for _, s := range Stages {
		r.Stages = append(r.Stages, StageRun{Stage: s, Status: wf.StatusPending})
	}
	p.current = r
	p.logf("%s: release started", r.Version)
	return nil
}

// Next runs the first stage that has not completed. After a failure that
// is the failed stage itself, so Next doubles as retry.
func (p *Pipeline) Next() error {
	r := p.current
	if r == nil {
		return ErrNoRelease
	}
	i := r.nextStage()
	s := &r.Stages[i]
	s.Attempts++
	r.Status = wf.StatusRunning

	if p.failNext {
		p.failNext = false
		s.Status = wf.StatusFailed
		s.Error = fmt.Sprintf("%s failed (injected) on attempt %d", s.Stage.Name, s.Attempts)
		r.Status = wf.StatusFailed
		p.logf("%s: %s failed — %s", r.Version, s.Stage.Name, s.Error)
		return nil
	}

	s.Status = wf.StatusCompleted
	s.Error = ""
	p.logf("%s: %s completed (attempt %d)", r.Version, s.Stage.Name, s.Attempts)

	if r.nextStage() == -1 {
		r.Status = wf.StatusCompleted
		p.history = append([]Release{*r}, p.history...)
		p.current = nil
		p.patch++
		p.logf("%s: released", r.Version)
	}
	return nil
}

// RunToEnd runs stages until the release completes or a stage fails.
func (p *Pipeline) RunToEnd() error {
	if p.current == nil {
		return ErrNoRelease
	}
	for {
		if err := p.Next(); err != nil {
			return err
		}
		if p.current == nil || p.current.Status == wf.StatusFailed {
			return nil
		}
	}
}

func (p *Pipeline) logf(format string, args ...any) {
	p.log = append(p.log, fmt.Sprintf(format, args...))
}
