// Package workflow provides banking workflow orchestration — business date
// management, idempotent daily processing, and persistent run tracking.
//
// It defines a DayProcessor interface that callers implement with their
// own domain logic (e.g. gobank-products). This package owns timing,
// scheduling, and state persistence — not business rules.
package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// WorkflowType identifies a kind of workflow.
type WorkflowType string

// WorkflowDailyUpdate is the type of DailyUpdate, the one workflow this
// package runs itself (Runner.RunDaily).
const WorkflowDailyUpdate WorkflowType = "daily_update"

// SchemaVersion is the SQL schema version this code expects. The
// sqlstore package's migration history must end at it; the memory store
// has no schema.
const SchemaVersion = 5

// Purpose is what a workflow achieves; its workflows are the ways of
// achieving it. It lives in the registry only: a run's purpose is its
// definition's.
type Purpose struct {
	Name        string
	Description string
}

// Definition describes a kind of workflow the engine can run. Definitions
// is the registry; each run in the Store is an instance of one of them,
// and each step recorded against a run is one of the definition's Steps.
type Definition struct {
	Type        WorkflowType
	Purpose     Purpose
	Description string
	InstanceKey string // what identifies one instance
	Steps       []Step
}

// Revision is the definition's current step list as a Revision.
func (d Definition) Revision() Revision {
	names := make([]string, len(d.Steps))
	for i, s := range d.Steps {
		names[i] = s.Name
	}
	return NewRevision(names)
}

// StepsFor is the step list a run of this definition ran, given its
// revision: each step described by the definition where its name still
// exists there, bare otherwise. A run recorded before revisions has none
// and gets the definition's steps.
func (d Definition) StepsFor(rev Revision) []Step {
	if len(rev.Steps) == 0 {
		return d.Steps
	}
	steps := make([]Step, len(rev.Steps))
	for i, name := range rev.Steps {
		steps[i] = Step{Name: name}
		for _, s := range d.Steps {
			if s.Name == name {
				steps[i] = s
				break
			}
		}
	}
	return steps
}

// Revision is a workflow's step list as the code had it for one attempt,
// recorded on the run so that an instance which outlives its code still
// renders from what it ran. ID identifies the list: the same names in
// the same order are the same revision.
type Revision struct {
	ID    string
	Steps []string
}

// NewRevision makes the revision of a step list.
func NewRevision(steps []string) Revision {
	sum := sha256.Sum256([]byte(strings.Join(steps, "\n")))
	return Revision{ID: hex.EncodeToString(sum[:8]), Steps: steps}
}

// Step is one named unit of work in a Definition. Source links to the
// code that performs it.
type Step struct {
	Name        string
	Description string
	Source      string
}

// Definitions is the registry: every workflow the host program runs. It
// starts empty; a host appends its own definitions in an init next to the
// code that runs them, so its pages show only what it runs. gobank
// registers DailyUpdate; the demo app registers it and a release.
var Definitions []Definition

// RunCounts is the number of runs of one workflow type in each status.
type RunCounts map[RunStatus]int

// Total is the number of runs across all statuses.
func (c RunCounts) Total() int {
	n := 0
	for _, v := range c {
		n += v
	}
	return n
}

// RunStatus is the lifecycle state of a workflow run.
type RunStatus string

const (
	StatusPending   RunStatus = "pending"
	StatusRunning   RunStatus = "running"
	StatusWaiting   RunStatus = "waiting" // parked on a stage's condition; a tick re-enters it
	StatusCompleted RunStatus = "completed"
	StatusFailed    RunStatus = "failed"
	StatusCancelled RunStatus = "cancelled" // ended by an operator; only a rewind revives it
)

// InHand reports whether a run in status s is still work to do: anything
// but completed or cancelled.
func (s RunStatus) InHand() bool {
	return s != StatusCompleted && s != StatusCancelled
}

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
