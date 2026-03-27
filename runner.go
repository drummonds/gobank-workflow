package workflow

import (
	"context"
	"fmt"
	"time"

	gbp "codeberg.org/hum3/gobank-products"
	"github.com/google/uuid"
)

// Runner orchestrates daily workflow processing.
type Runner struct {
	store Store
	sim   *gbp.Simulation
	clock gbp.Clock
}

// NewRunner creates a Runner.
func NewRunner(store Store, sim *gbp.Simulation, clock gbp.Clock) *Runner {
	return &Runner{store: store, sim: sim, clock: clock}
}

// RunDaily processes a single business date idempotently.
// Returns nil, nil if the date has already been completed.
func (r *Runner) RunDaily(ctx context.Context, date BusinessDate) (*RunRecord, error) {
	existing, err := r.store.FindRun(ctx, WorkflowDailyUpdate, date)
	if err != nil {
		return nil, fmt.Errorf("checking existing run: %w", err)
	}
	if existing != nil && existing.Status == StatusCompleted {
		return nil, nil
	}

	now := time.Now()
	run := &RunRecord{
		ID:           uuid.New().String(),
		WorkflowType: WorkflowDailyUpdate,
		BusinessDate: date,
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

	obs := NewRunObserver(r.store, run.ID)
	r.sim.OnDailyUpdate(obs.Handler())

	_, simErr := r.sim.AdvanceToDate(date.Time())
	if simErr != nil {
		_ = r.store.FailRun(ctx, run.ID, simErr.Error())
		return nil, fmt.Errorf("simulation advance to %s: %w", date, simErr)
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
