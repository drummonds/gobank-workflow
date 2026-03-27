package workflow

import (
	"context"
	"time"

	gbp "codeberg.org/hum3/gobank-products"
	"github.com/google/uuid"
)

// RunObserver captures DailyUpdate events and writes StepResults to a Store.
type RunObserver struct {
	store Store
	runID string
}

// NewRunObserver creates an observer bound to a specific run.
func NewRunObserver(store Store, runID string) *RunObserver {
	return &RunObserver{store: store, runID: runID}
}

// Handler returns a gbp.DailyUpdateHandler that records step results.
func (o *RunObserver) Handler() gbp.DailyUpdateHandler {
	return func(update gbp.DailyUpdate) {
		ctx := context.Background()
		for _, au := range update.Accounts {
			step := &StepResult{
				ID:             uuid.New().String(),
				RunID:          o.runID,
				AccountID:      au.Account.Account.ID,
				StepName:       "end_of_day",
				OpeningBalance: int64(au.OpeningBalance),
				ClosingBalance: int64(au.ClosingBalance),
				InterestAmount: int64(au.InterestAmount),
				Exponent:       au.Exponent,
				CreatedAt:      time.Now(),
			}
			_ = o.store.RecordStep(ctx, step)
		}
	}
}
