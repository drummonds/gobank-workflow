package workflow

import "time"

// BusinessDate represents a bank processing date (date only, no time component).
type BusinessDate struct {
	Year  int
	Month time.Month
	Day   int
}

// NewBusinessDate creates a BusinessDate from year, month, day.
func NewBusinessDate(year int, month time.Month, day int) BusinessDate {
	return BusinessDate{Year: year, Month: month, Day: day}
}

// BusinessDateFrom extracts a BusinessDate from a time.Time.
func BusinessDateFrom(t time.Time) BusinessDate {
	return BusinessDate{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// Time returns the BusinessDate as a time.Time at midnight UTC.
func (d BusinessDate) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// String returns the date in YYYY-MM-DD format.
func (d BusinessDate) String() string {
	return d.Time().Format("2006-01-02")
}

// IsZero reports whether d is the zero value.
func (d BusinessDate) IsZero() bool {
	return d.Year == 0 && d.Month == 0 && d.Day == 0
}

// Before reports whether d is before other.
func (d BusinessDate) Before(other BusinessDate) bool {
	return d.Time().Before(other.Time())
}

// After reports whether d is after other.
func (d BusinessDate) After(other BusinessDate) bool {
	return d.Time().After(other.Time())
}

// Equal reports whether d equals other.
func (d BusinessDate) Equal(other BusinessDate) bool {
	return d.Year == other.Year && d.Month == other.Month && d.Day == other.Day
}

// Next returns the following calendar day.
func (d BusinessDate) Next() BusinessDate {
	return BusinessDateFrom(d.Time().AddDate(0, 0, 1))
}

// RunRecord tracks one execution of a workflow for a business date.
type RunRecord struct {
	ID           string
	WorkflowType WorkflowType
	BusinessDate BusinessDate
	Status       RunStatus
	StartedAt    time.Time
	CompletedAt  time.Time
	Error        string
	CreatedAt    time.Time
}

// StepResult records one account's processing within a run.
type StepResult struct {
	ID             string
	RunID          string
	AccountID      string
	StepName       string
	OpeningBalance int64
	ClosingBalance int64
	InterestAmount int64
	Exponent       int
	DurationNs     int64
	CreatedAt      time.Time
}
