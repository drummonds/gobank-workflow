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

const (
	WorkflowDailyUpdate WorkflowType = "daily_update"
	WorkflowRelease     WorkflowType = "release"
)

// SchemaVersion is the SQL schema version this code expects. The
// sqlstore package's migration history must end at it; the memory store
// has no schema.
const SchemaVersion = 4

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

const (
	srcWorkflow = "https://git.bytestone.uk/hum3/gobank-workflow/src/branch/main/"
	srcTaskPlus = "https://git.bytestone.uk/hum3/task-plus/src/branch/main/"
)

// ReleaseStages are the step names of the release pipeline, in order.
var ReleaseStages = []string{"check", "bump", "changelog", "tag", "push origin", "push mirror", "verify install", "deploy docs"}

// Definitions lists every workflow type the engine knows.
var Definitions = []Definition{
	{
		Type:        WorkflowDailyUpdate,
		Purpose:     Purpose{"end of day", "Close the bank's books for one business date."},
		Description: "End-of-day processing for one business date: the DayProcessor runs over every account. A completed date is never re-run.",
		InstanceKey: "business date",
		Steps:       []Step{{"end_of_day", "one per account: the DayProcessor accrues the day's interest; the Runner records opening and closing balances", srcWorkflow + "runner.go"}},
	},
	{
		Type:        WorkflowRelease,
		Purpose:     Purpose{"release", "Ship a new version of the code."},
		Description: "Ship a new version through the release pipeline. A failed stage stops the release; the next run resumes from that stage.",
		InstanceKey: "version",
		Steps: []Step{
			{"check", "fmt, vet, test", srcTaskPlus + "internal/workflow/workflow.go"},
			{"bump", "choose the next semantic version", srcTaskPlus + "internal/workflow/gather.go"},
			{"changelog", "move [Unreleased] under the new version heading", srcTaskPlus + "internal/changelog/changelog.go"},
			{"tag", "commit and create the annotated git tag", srcTaskPlus + "internal/workflow/execute.go"},
			{"push origin", "push commit and tag to origin", srcTaskPlus + "internal/git/git.go"},
			{"push mirror", "push commit and tag to the GitHub mirror", srcTaskPlus + "internal/git/git.go"},
			{"verify install", "go install the tag from the module proxy", srcTaskPlus + "internal/workflow/execute.go"},
			{"deploy docs", "build and rsync the docs site", srcTaskPlus + "internal/deploy/rsync.go"},
		},
	},
}

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
