package workflow

import (
	"context"
	"database/sql"
	"time"
)

// SQLStore implements Store using a *sql.DB (go-postgres / pglike compatible).
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore creates the workflow tables if needed and returns a SQLStore.
func NewSQLStore(db *sql.DB) (*SQLStore, error) {
	if err := execStatements(db, SchemaSQL); err != nil {
		return nil, err
	}
	return &SQLStore{db: db}, nil
}

var _ Store = (*SQLStore)(nil)

func (s *SQLStore) CreateRun(ctx context.Context, run *RunRecord) error {
	// Check for existing completed run (idempotency guard).
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workflow_runs WHERE workflow_type = ? AND business_date = ? AND status = 'completed'`,
		string(run.WorkflowType), run.BusinessDate.String(),
	).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrAlreadyCompleted
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO workflow_runs (id, workflow_type, business_date, status, started_at, error_message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run.ID, string(run.WorkflowType), run.BusinessDate.String(),
		string(run.Status), run.StartedAt.UTC().Format(time.RFC3339Nano),
		run.Error, run.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLStore) GetRun(ctx context.Context, id string) (*RunRecord, error) {
	return s.scanRun(ctx,
		`SELECT id, workflow_type, business_date, status, started_at, completed_at, error_message, created_at
		 FROM workflow_runs WHERE id = ?`, id)
}

func (s *SQLStore) FindRun(ctx context.Context, wfType WorkflowType, date BusinessDate) (*RunRecord, error) {
	return s.scanRun(ctx,
		`SELECT id, workflow_type, business_date, status, started_at, completed_at, error_message, created_at
		 FROM workflow_runs WHERE workflow_type = ? AND business_date = ?`,
		string(wfType), date.String())
}

func (s *SQLStore) CompleteRun(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE workflow_runs SET status = 'completed', completed_at = ? WHERE id = ?`,
		now, id)
	return err
}

func (s *SQLStore) FailRun(ctx context.Context, id string, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE workflow_runs SET status = 'failed', completed_at = ?, error_message = ? WHERE id = ?`,
		now, errMsg, id)
	return err
}

func (s *SQLStore) RecordStep(ctx context.Context, step *StepResult) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workflow_steps (id, run_id, account_id, step_name, opening_balance, closing_balance, interest_amount, exponent, duration_ns, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		step.ID, step.RunID, step.AccountID, step.StepName,
		step.OpeningBalance, step.ClosingBalance, step.InterestAmount,
		step.Exponent, step.DurationNs,
		step.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLStore) ListSteps(ctx context.Context, runID string) ([]StepResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, account_id, step_name, opening_balance, closing_balance, interest_amount, exponent, duration_ns, created_at
		 FROM workflow_steps WHERE run_id = ? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []StepResult
	for rows.Next() {
		var step StepResult
		var createdAt string
		if err := rows.Scan(&step.ID, &step.RunID, &step.AccountID, &step.StepName,
			&step.OpeningBalance, &step.ClosingBalance, &step.InterestAmount,
			&step.Exponent, &step.DurationNs, &createdAt); err != nil {
			return nil, err
		}
		step.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		results = append(results, step)
	}
	return results, rows.Err()
}

func (s *SQLStore) ListRuns(ctx context.Context, limit int) ([]RunRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workflow_type, business_date, status, started_at, completed_at, error_message, created_at
		 FROM workflow_runs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []RunRecord
	for rows.Next() {
		r, err := scanRunRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *r)
	}
	return results, rows.Err()
}

func (s *SQLStore) LastCompletedDate(ctx context.Context, wfType WorkflowType) (BusinessDate, error) {
	var dateStr sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(business_date) FROM workflow_runs WHERE workflow_type = ? AND status = 'completed'`,
		string(wfType),
	).Scan(&dateStr)
	if err != nil || !dateStr.Valid {
		return BusinessDate{}, err
	}
	t, err := time.Parse("2006-01-02", dateStr.String)
	if err != nil {
		return BusinessDate{}, err
	}
	return BusinessDateFrom(t), nil
}

func (s *SQLStore) scanRun(ctx context.Context, query string, args ...any) (*RunRecord, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	r, err := scanRunFromRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRunFromRow(row scannable) (*RunRecord, error) {
	var r RunRecord
	var wfType, dateStr, status string
	var startedAt, completedAt sql.NullString
	var createdAt string
	var errMsg string

	err := row.Scan(&r.ID, &wfType, &dateStr, &status, &startedAt, &completedAt, &errMsg, &createdAt)
	if err != nil {
		return nil, err
	}

	r.WorkflowType = WorkflowType(wfType)
	r.Status = RunStatus(status)
	r.Error = errMsg

	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		r.BusinessDate = BusinessDateFrom(t)
	}
	if startedAt.Valid {
		r.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt.String)
	}
	if completedAt.Valid {
		r.CompletedAt, _ = time.Parse(time.RFC3339Nano, completedAt.String)
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)

	return &r, nil
}

func scanRunRow(rows *sql.Rows) (*RunRecord, error) {
	return scanRunFromRow(rows)
}
