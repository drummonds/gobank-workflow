package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Stage is one step of a pipeline workflow: a named unit of work.
type Stage struct {
	Name string
	Run  func(ctx context.Context) error
}

// RunPipeline runs stages in order as one instance of wfType identified by
// key (for a release, the version). A completed key is a no-op. A stage
// that fails marks its step and the run failed and stops the run; the next
// call for the same key restarts that instance and resumes from the failed
// stage, never repeating a completed one.
func (r *Runner) RunPipeline(ctx context.Context, wfType WorkflowType, key string, date BusinessDate, stages []Stage) (*RunRecord, error) {
	existing, err := r.store.FindRunByKey(ctx, wfType, key)
	if err != nil {
		return nil, fmt.Errorf("checking existing run: %w", err)
	}
	if existing != nil && existing.Status == StatusCompleted {
		return nil, nil
	}

	done := map[string]bool{}
	var run *RunRecord
	if existing != nil {
		run = existing
		steps, err := r.store.ListSteps(ctx, run.ID)
		if err != nil {
			return nil, fmt.Errorf("reading steps: %w", err)
		}
		for _, s := range steps {
			if s.Status == StatusCompleted {
				done[s.StepName] = true
			}
		}
		if err := r.store.DeleteSteps(ctx, run.ID, StatusFailed, StatusRunning, StatusPending); err != nil {
			return nil, fmt.Errorf("clearing failed steps: %w", err)
		}
		if err := r.store.RestartRun(ctx, run.ID); err != nil {
			return nil, fmt.Errorf("restarting run: %w", err)
		}
	} else {
		now := time.Now()
		run = &RunRecord{
			ID:           uuid.New().String(),
			WorkflowType: wfType,
			BusinessDate: date,
			Key:          key,
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

	for _, st := range stages {
		if done[st.Name] {
			continue
		}
		t0 := time.Now()
		stageErr := st.Run(ctx)
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
			_ = r.store.FailRun(ctx, run.ID, msg)
			return nil, fmt.Errorf("%s %s: %s", wfType, key, msg)
		}
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
