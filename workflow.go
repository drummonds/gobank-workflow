// Package workflow provides banking workflow orchestration — business date
// management, idempotent daily processing, and persistent run tracking.
//
// It wraps gobank-products' Simulation engine with scheduling, state
// persistence, and observability. gobank-products owns product definitions
// and feature handlers (stateless business logic); this package owns timing.
package workflow

import "errors"

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
