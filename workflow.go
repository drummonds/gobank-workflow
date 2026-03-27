// Package workflow provides banking workflow orchestration — business date
// management, idempotent daily processing, and persistent run tracking.
//
// It defines a DayProcessor interface that callers implement with their
// own domain logic (e.g. gobank-products). This package owns timing,
// scheduling, and state persistence — not business rules.
package workflow

import (
	"context"
	"errors"
	"time"
)

// WorkflowType identifies a kind of workflow.
type WorkflowType string

const (
	WorkflowDailyUpdate WorkflowType = "daily_update"
)

// RunStatus is the lifecycle state of a workflow run.
type RunStatus string

const (
	StatusPending   RunStatus = "pending"
	StatusRunning   RunStatus = "running"
	StatusCompleted RunStatus = "completed"
	StatusFailed    RunStatus = "failed"
)

// ErrAlreadyCompleted is returned when a completed run already exists
// for the given (workflow_type, business_date) pair.
var ErrAlreadyCompleted = errors.New("workflow: run already completed for this date")

// Clock provides the current time, allowing simulation to control time.
type Clock interface {
	Now() time.Time
}

// WallClock uses real system time.
type WallClock struct{}

func (WallClock) Now() time.Time { return time.Now() }

// DayProcessor processes all accounts for a single business day.
// Implementations contain the domain logic (interest accrual, EOD, etc.).
type DayProcessor interface {
	ProcessDay(ctx context.Context, date BusinessDate) (DayResult, error)
}

// DayResult captures what happened during a day's processing.
type DayResult struct {
	Date     BusinessDate
	Accounts []AccountResult
}

// AccountResult captures one account's processing for a day.
type AccountResult struct {
	AccountID      string
	OpeningBalance int64
	ClosingBalance int64
	InterestAmount int64
	Exponent       int
}
