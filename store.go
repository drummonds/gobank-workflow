package workflow

import "context"

// Store persists workflow run history and step results.
type Store interface {
	// CreateRun inserts a new run record. Returns ErrAlreadyCompleted if
	// a completed run already exists for this (workflow_type, business_date).
	CreateRun(ctx context.Context, run *RunRecord) error

	// GetRun retrieves a run by ID.
	GetRun(ctx context.Context, id string) (*RunRecord, error)

	// FindRun looks up a date-keyed run by (workflow_type, business_date).
	// Returns nil, nil if not found.
	FindRun(ctx context.Context, wfType WorkflowType, date BusinessDate) (*RunRecord, error)

	// FindRunByKey looks up a keyed run by (workflow_type, key).
	// Returns nil, nil if not found.
	FindRunByKey(ctx context.Context, wfType WorkflowType, key string) (*RunRecord, error)

	// RestartRun puts a failed run back to running for a new attempt:
	// status running, started now, error cleared, and the revision the
	// attempt runs recorded.
	RestartRun(ctx context.Context, id string, rev Revision) error

	// WaitRun parks a run on a stage's condition: status waiting, the
	// attempt's StartedAt kept.
	WaitRun(ctx context.Context, id string) error

	// ResumeRun continues a waiting run's attempt: status running,
	// error cleared, StartedAt kept. RestartRun is the new-attempt
	// counterpart.
	ResumeRun(ctx context.Context, id string) error

	// CompleteRun marks a run as completed with the current time.
	CompleteRun(ctx context.Context, id string) error

	// FailRun marks a run as failed with an error message.
	FailRun(ctx context.Context, id string, errMsg string) error

	// RecordStep inserts a step result.
	RecordStep(ctx context.Context, step *StepResult) error

	// ListSteps returns all steps for a run, ordered by created_at.
	ListSteps(ctx context.Context, runID string) ([]StepResult, error)

	// DeleteSteps removes a run's steps in the given statuses, or all of
	// them when no status is given.
	DeleteSteps(ctx context.Context, runID string, statuses ...RunStatus) error

	// DeleteStep removes a run's steps with the given name: a stage that
	// was undone, so the next attempt runs it again.
	DeleteStep(ctx context.Context, runID, stepName string) error

	// ListRuns returns recent runs, most recent first.
	ListRuns(ctx context.Context, limit int) ([]RunRecord, error)

	// CountRuns returns, for every workflow type that has run, how many
	// runs (instances) exist in each status.
	CountRuns(ctx context.Context) (map[WorkflowType]RunCounts, error)

	// LastCompletedDate returns the most recent business date with a
	// completed run of the given workflow type. Returns zero BusinessDate if none.
	LastCompletedDate(ctx context.Context, wfType WorkflowType) (BusinessDate, error)
}
