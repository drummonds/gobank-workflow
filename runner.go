package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Runner orchestrates daily workflow processing.
type Runner struct {
	store     Store
	processor DayProcessor
}

// NewRunner creates a Runner.
func NewRunner(store Store, processor DayProcessor) *Runner {
	return &Runner{store: store, processor: processor}
}

// dailyRevision is RunDaily's step list: one end_of_day step per account.
var dailyRevision = NewRevision([]string{"end_of_day"})

// RunDaily processes a single business date idempotently.
// Returns nil, nil if the date has already been completed or cancelled.
func (r *Runner) RunDaily(ctx context.Context, date BusinessDate) (*RunRecord, error) {
	existing, err := r.store.FindRun(ctx, WorkflowDailyUpdate, date)
	if err != nil {
		return nil, fmt.Errorf("checking existing run: %w", err)
	}
	if existing != nil && !existing.Status.InHand() {
		return nil, nil
	}

	var run *RunRecord
	if existing != nil {
		// A new attempt at the same instance: every account is reprocessed.
		run = existing
		if err := r.store.RestartRun(ctx, run.ID, dailyRevision); err != nil {
			return nil, fmt.Errorf("restarting run: %w", err)
		}
		if err := r.store.DeleteSteps(ctx, run.ID); err != nil {
			return nil, fmt.Errorf("clearing steps: %w", err)
		}
	} else {
		now := time.Now()
		run = &RunRecord{
			ID:           uuid.New().String(),
			WorkflowType: WorkflowDailyUpdate,
			BusinessDate: date,
			Revision:     dailyRevision,
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

	result, procErr := r.processor.ProcessDay(ctx, date)
	if procErr != nil {
		_ = r.store.FailRun(ctx, run.ID, procErr.Error())
		return nil, fmt.Errorf("processing %s: %w", date, procErr)
	}

	// Record step results.
	for _, ar := range result.Accounts {
		step := &StepResult{
			ID:             uuid.New().String(),
			RunID:          run.ID,
			AccountID:      ar.AccountID,
			StepName:       dailyRevision.Steps[0],
			Status:         StatusCompleted,
			OpeningBalance: ar.OpeningBalance,
			ClosingBalance: ar.ClosingBalance,
			InterestAmount: ar.InterestAmount,
			Exponent:       ar.Exponent,
			CreatedAt:      time.Now(),
		}
		if err := r.store.RecordStep(ctx, step); err != nil {
			return nil, fmt.Errorf("recording step for %s: %w", ar.AccountID, err)
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

// CatchUp processes all unprocessed dates from the given start date
// (or last completed date + 1, whichever is later) up to and including target.
func (r *Runner) CatchUp(ctx context.Context, from, target BusinessDate) ([]RunRecord, error) {
	last, err := r.store.LastCompletedDate(ctx, WorkflowDailyUpdate)
	if err != nil {
		return nil, fmt.Errorf("finding last completed date: %w", err)
	}

	start := from
	if !last.IsZero() && last.Next().After(from) {
		start = last.Next()
	}

	var results []RunRecord
	for d := start; !d.After(target); d = d.Next() {
		rec, err := r.RunDaily(ctx, d)
		if err != nil {
			return results, fmt.Errorf("running %s: %w", d, err)
		}
		if rec != nil {
			results = append(results, *rec)
		}
	}
	return results, nil
}

// LastProcessedDate returns the most recent completed daily_update date.
func (r *Runner) LastProcessedDate(ctx context.Context) (BusinessDate, error) {
	return r.store.LastCompletedDate(ctx, WorkflowDailyUpdate)
}
