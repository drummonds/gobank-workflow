package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Stage is one step of a pipeline workflow: a named unit of work. Until,
// when set, is a condition the stage waits on: while it is false the run
// parks as waiting and RunPipeline returns, and the caller's next call
// re-evaluates it; once it holds, Run (if any) executes and the stage
// completes. An approval or an external event is a store write whose
// condition is "is it recorded". The polling cadence is the caller's.
//
// Undo, when set, reverses the stage's work once a later stage has
// failed: a server created is deleted. The undone stage's step is
// removed, so the next attempt runs it again. A stage without Undo keeps
// its step and its work, and the next attempt resumes after it.
//
// Final marks a stage whose work must never be redone, such as an archive
// written: once it has completed, Rewind refuses to go back to it or any
// stage before it.
type Stage struct {
	Name  string
	Until func(ctx context.Context) (bool, error)
	Run   func(ctx context.Context) error
	Undo  func(ctx context.Context) error
	Final bool
}

// RunPipeline runs stages in order as one instance of wfType identified by
// key (for a release, the version). A completed or cancelled key is a no-op. A stage
// that fails marks its step and the run failed and stops the run; the next
// call for the same key restarts that instance and resumes from the failed
// stage, never repeating a completed one. A stage whose Until is false
// leaves the run waiting and returns it with no error; calling again
// continues the same attempt from that stage.
//
// When a stage fails, the completed stages before it that have an Undo
// are undone in reverse order, nearest first, and their steps removed,
// so the next attempt starts from the first stage whose work no longer
// stands. An Undo that fails is reported with the failure, keeps its step
// since its work may still stand, and stops the undoing there. The run's
// error names the failed stage, what was undone and any undo that failed.
func (r *Runner) RunPipeline(ctx context.Context, wfType WorkflowType, key string, date BusinessDate, stages []Stage) (*RunRecord, error) {
	existing, err := r.store.FindRunByKey(ctx, wfType, key)
	if err != nil {
		return nil, fmt.Errorf("checking existing run: %w", err)
	}
	if existing != nil && !existing.Status.InHand() {
		return nil, nil
	}

	names := make([]string, len(stages))
	for i, st := range stages {
		names[i] = st.Name
	}
	rev := NewRevision(names)

	done := map[string]bool{}
	waiting := map[string]bool{}
	var run *RunRecord
	if existing != nil {
		run = existing
		steps, err := r.store.ListSteps(ctx, run.ID)
		if err != nil {
			return nil, fmt.Errorf("reading steps: %w", err)
		}
		for _, s := range steps {
			switch s.Status {
			case StatusCompleted:
				done[s.StepName] = true
			case StatusWaiting:
				waiting[s.StepName] = true
			}
		}
		if run.Status != StatusWaiting {
			// A new attempt after a failure or a crash.
			if err := r.store.DeleteSteps(ctx, run.ID, StatusFailed, StatusRunning, StatusPending); err != nil {
				return nil, fmt.Errorf("clearing failed steps: %w", err)
			}
			if err := r.store.RestartRun(ctx, run.ID, rev); err != nil {
				return nil, fmt.Errorf("restarting run: %w", err)
			}
			run.Status = StatusRunning
			run.Revision = rev
		}
	} else {
		now := time.Now()
		run = &RunRecord{
			ID:           uuid.New().String(),
			WorkflowType: wfType,
			BusinessDate: date,
			Key:          key,
			Revision:     rev,
			Status:       StatusRunning,
			StartedAt:    now,
			CreatedAt:    now,
		}
		if err := r.store.CreateRun(ctx, run); err != nil {
			if err == ErrAlreadyCompleted {
				return nil, nil
			}
			return nil, fmt.Errorf("creating run: %w", err)
		}
	}

	for i, st := range stages {
		if done[st.Name] {
			continue
		}
		t0 := time.Now()
		var stageErr error
		if st.Until != nil {
			holds, err := st.Until(ctx)
			switch {
			case err != nil:
				stageErr = err
			case !holds:
				return r.park(ctx, run, st.Name, waiting[st.Name])
			case waiting[st.Name]:
				// The condition now holds: the waiting step gives way to the real one.
				if err := r.store.DeleteSteps(ctx, run.ID, StatusWaiting); err != nil {
					return nil, fmt.Errorf("clearing waiting step: %w", err)
				}
			}
		}
		if run.Status == StatusWaiting {
			if err := r.store.ResumeRun(ctx, run.ID); err != nil {
				return nil, fmt.Errorf("resuming run: %w", err)
			}
			run.Status = StatusRunning
		}
		if stageErr == nil && st.Run != nil {
			stageErr = st.Run(ctx)
		}
		step := &StepResult{
			ID:         uuid.New().String(),
			RunID:      run.ID,
			StepName:   st.Name,
			Status:     StatusCompleted,
			DurationNs: int64(time.Since(t0)),
			CreatedAt:  time.Now(),
		}
		if stageErr != nil {
			step.Status = StatusFailed
			step.Error = stageErr.Error()
		}
		if err := r.store.RecordStep(ctx, step); err != nil {
			return nil, fmt.Errorf("recording step %s: %w", st.Name, err)
		}
		if stageErr != nil {
			msg := fmt.Sprintf("%s: %v", st.Name, stageErr)
			msg += r.undo(ctx, run.ID, stages[:i], done)
			_ = r.store.FailRun(ctx, run.ID, msg)
			return nil, fmt.Errorf("%s %s: %s", wfType, key, msg)
		}
		done[st.Name] = true
	}

	if err := r.store.CompleteRun(ctx, run.ID); err != nil {
		return nil, fmt.Errorf("completing run: %w", err)
	}
	completed, _ := r.store.GetRun(ctx, run.ID)
	if completed != nil {
		return completed, nil
	}
	run.Status = StatusCompleted
	return run, nil
}

// park leaves the run waiting on stage: the waiting step is recorded the
// first time only, so its CreatedAt is when the wait began, and a run
// already waiting is not rewritten.
func (r *Runner) park(ctx context.Context, run *RunRecord, stage string, alreadyWaiting bool) (*RunRecord, error) {
	if !alreadyWaiting {
		step := &StepResult{ID: uuid.New().String(), RunID: run.ID, StepName: stage, Status: StatusWaiting, CreatedAt: time.Now()}
		if err := r.store.RecordStep(ctx, step); err != nil {
			return nil, fmt.Errorf("recording step %s: %w", stage, err)
		}
	}
	if run.Status != StatusWaiting {
		if err := r.store.WaitRun(ctx, run.ID); err != nil {
			return nil, fmt.Errorf("parking run: %w", err)
		}
		run.Status = StatusWaiting
	}
	return run, nil
}

// undo reverses the completed stages among before that have an Undo, in
// reverse order, removing each undone stage's step. It stops at the
// first Undo that fails. What it did is returned as a suffix for the
// run's error: " (undone: add, create)" and "; undo add: <error>".
func (r *Runner) undo(ctx context.Context, runID string, before []Stage, done map[string]bool) string {
	var undone []string
	var failed error
	for i := len(before) - 1; i >= 0; i-- {
		st := before[i]
		if !done[st.Name] || st.Undo == nil {
			continue
		}
		if err := st.Undo(ctx); err != nil {
			failed = fmt.Errorf("undo %s: %w", st.Name, err)
			break
		}
		if err := r.store.DeleteStep(ctx, runID, st.Name); err != nil {
			failed = fmt.Errorf("undo %s: removing its step: %w", st.Name, err)
			break
		}
		undone = append(undone, st.Name)
	}
	suffix := ""
	if len(undone) > 0 {
		suffix += " (undone: " + strings.Join(undone, ", ") + ")"
	}
	if failed != nil {
		suffix += "; " + failed.Error()
	}
	return suffix
}
