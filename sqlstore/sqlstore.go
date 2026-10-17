package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	db "git.bytestone.uk/hum3/gobank-db"
	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// Store implements wf.Store using a *sql.DB (go-postgres / pglike compatible).
type Store struct {
	db *sql.DB
}

// New brings the database up to wf.SchemaVersion (recorded in gobank-db's
// schema_migrations table) and returns a Store.
func New(d *sql.DB) (*Store, error) {
	if _, err := db.Apply(context.Background(), d, Migrations); err != nil {
		return nil, err
	}
	return &Store{db: d}, nil
}

// SchemaVersion reports the highest migration version applied to the
// database, 0 if none.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	applied, err := db.AppliedVersions(ctx, s.db)
	if err != nil {
		return 0, err
	}
	latest := 0
	for v := range applied {
		if v > latest {
			latest = v
		}
	}
	return latest, nil
}

func (s *Store) CountRuns(ctx context.Context) (map[wf.WorkflowType]wf.RunCounts, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workflow_type, status, COUNT(*) FROM workflow_runs GROUP BY workflow_type, status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[wf.WorkflowType]wf.RunCounts{}
	for rows.Next() {
		var wfType, status string
		var n int
		if err := rows.Scan(&wfType, &status, &n); err != nil {
			return nil, err
		}
		if counts[wf.WorkflowType(wfType)] == nil {
			counts[wf.WorkflowType(wfType)] = wf.RunCounts{}
		}
		counts[wf.WorkflowType(wfType)][wf.RunStatus(status)] = n
	}
	return counts, rows.Err()
}

var _ wf.Store = (*Store)(nil)

func (s *Store) CreateRun(ctx context.Context, run *wf.RunRecord) error {
	// Check for existing completed run (idempotency guard).
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workflow_runs WHERE workflow_type = ? AND business_date = ? AND instance_key = ? AND status = 'completed'`,
		string(run.WorkflowType), run.BusinessDate.String(), run.Key,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return wf.ErrAlreadyCompleted
	}

	// One instance per (type, date): an unfinished attempt is replaced by
	// this one, steps first because of the foreign key.
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM workflow_steps WHERE run_id IN (SELECT id FROM workflow_runs WHERE workflow_type = ? AND business_date = ? AND instance_key = ?)`,
		string(run.WorkflowType), run.BusinessDate.String(), run.Key); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM workflow_runs WHERE workflow_type = ? AND business_date = ? AND instance_key = ?`,
		string(run.WorkflowType), run.BusinessDate.String(), run.Key); err != nil {
		return err
	}

	stepNames, err := encodeSteps(run.Revision)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO workflow_runs (id, workflow_type, business_date, instance_key, revision, step_names, status, started_at, error_message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, string(run.WorkflowType), run.BusinessDate.String(), run.Key, run.Revision.ID, stepNames,
		string(run.Status), run.StartedAt.UTC().Format(time.RFC3339Nano),
		run.Error, run.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) GetRun(ctx context.Context, id string) (*wf.RunRecord, error) {
	return s.scanRun(ctx,
		`SELECT id, workflow_type, business_date, instance_key, revision, step_names, status, started_at, completed_at, error_message, created_at
		 FROM workflow_runs WHERE id = ?`, id)
}

func (s *Store) FindRun(ctx context.Context, wfType wf.WorkflowType, date wf.BusinessDate) (*wf.RunRecord, error) {
	return s.scanRun(ctx,
		`SELECT id, workflow_type, business_date, instance_key, revision, step_names, status, started_at, completed_at, error_message, created_at
		 FROM workflow_runs WHERE workflow_type = ? AND business_date = ? AND instance_key = ''`,
		string(wfType), date.String())
}

func (s *Store) FindRunByKey(ctx context.Context, wfType wf.WorkflowType, key string) (*wf.RunRecord, error) {
	return s.scanRun(ctx,
		`SELECT id, workflow_type, business_date, instance_key, revision, step_names, status, started_at, completed_at, error_message, created_at
		 FROM workflow_runs WHERE workflow_type = ? AND instance_key = ?`,
		string(wfType), key)
}

func (s *Store) RestartRun(ctx context.Context, id string, rev wf.Revision) error {
	stepNames, err := encodeSteps(rev)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx,
		`UPDATE workflow_runs SET status = 'running', started_at = ?, completed_at = NULL, error_message = '', revision = ?, step_names = ? WHERE id = ?`,
		now, rev.ID, stepNames, id)
	return err
}

// encodeSteps is a revision's step names as stored: a JSON array, or
// empty when the run has no revision.
func encodeSteps(rev wf.Revision) (string, error) {
	if len(rev.Steps) == 0 {
		return "", nil
	}
	b, err := json.Marshal(rev.Steps)
	return string(b), err
}

func (s *Store) WaitRun(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_runs SET status = 'waiting', error_message = '' WHERE id = ?`, id)
	return err
}

func (s *Store) ResumeRun(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_runs SET status = 'running', error_message = '' WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteSteps(ctx context.Context, runID string, statuses ...wf.RunStatus) error {
	if len(statuses) == 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM workflow_steps WHERE run_id = ?`, runID)
		return err
	}
	for _, st := range statuses {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM workflow_steps WHERE run_id = ? AND status = ?`, runID, string(st)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteStep(ctx context.Context, runID, stepName string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workflow_steps WHERE run_id = ? AND step_name = ?`, runID, stepName)
	return err
}

func (s *Store) CompleteRun(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE workflow_runs SET status = 'completed', completed_at = ? WHERE id = ?`,
		now, id)
	return err
}

func (s *Store) FailRun(ctx context.Context, id string, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE workflow_runs SET status = 'failed', completed_at = ?, error_message = ? WHERE id = ?`,
		now, errMsg, id)
	return err
}

func (s *Store) RecordStep(ctx context.Context, step *wf.StepResult) error {
	status := step.Status
	if status == "" {
		status = wf.StatusCompleted
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workflow_steps (id, run_id, account_id, step_name, status, error_message, opening_balance, closing_balance, interest_amount, exponent, duration_ns, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		step.ID, step.RunID, step.AccountID, step.StepName, string(status), step.Error,
		step.OpeningBalance, step.ClosingBalance, step.InterestAmount,
		step.Exponent, step.DurationNs,
		step.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) ListSteps(ctx context.Context, runID string) ([]wf.StepResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, account_id, step_name, status, error_message, opening_balance, closing_balance, interest_amount, exponent, duration_ns, created_at
		 FROM workflow_steps WHERE run_id = ? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []wf.StepResult
	for rows.Next() {
		var step wf.StepResult
		var createdAt, status string
		if err := rows.Scan(&step.ID, &step.RunID, &step.AccountID, &step.StepName, &status, &step.Error,
			&step.OpeningBalance, &step.ClosingBalance, &step.InterestAmount,
			&step.Exponent, &step.DurationNs, &createdAt); err != nil {
			return nil, err
		}
		step.Status = wf.RunStatus(status)
		step.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		results = append(results, step)
	}
	return results, rows.Err()
}

func (s *Store) ListRuns(ctx context.Context, f wf.RunFilter) ([]wf.RunRecord, error) {
	where, args := runFilterSQL(f)
	query := `SELECT id, workflow_type, business_date, instance_key, revision, step_names, status, started_at, completed_at, error_message, created_at
		 FROM workflow_runs` + where + ` ORDER BY `
	if f.InHandFirst {
		query += `CASE WHEN status = 'completed' THEN 1 ELSE 0 END, `
	}
	query += `created_at DESC`
	skip := f.Offset
	if f.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, f.Limit, f.Offset)
		skip = 0
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []wf.RunRecord
	for rows.Next() {
		r, err := scanRunRow(rows)
		if err != nil {
			return nil, err
		}
		// SQLite has no OFFSET without LIMIT, nor Postgres LIMIT -1: an
		// offset alone is skipped here.
		if skip > 0 {
			skip--
			continue
		}
		results = append(results, *r)
	}
	return results, rows.Err()
}

// runFilterSQL is the WHERE clause, if any, for a filter and its arguments.
func runFilterSQL(f wf.RunFilter) (string, []any) {
	var conds []string
	var args []any
	in := func(col string, vals []string) {
		conds = append(conds, col+` IN (?`+strings.Repeat(`, ?`, len(vals)-1)+`)`)
		for _, v := range vals {
			args = append(args, v)
		}
	}
	if len(f.Types) > 0 {
		vals := make([]string, len(f.Types))
		for i, t := range f.Types {
			vals[i] = string(t)
		}
		in(`workflow_type`, vals)
	}
	if len(f.Statuses) > 0 {
		vals := make([]string, len(f.Statuses))
		for i, st := range f.Statuses {
			vals[i] = string(st)
		}
		in(`status`, vals)
	}
	if !f.Since.IsZero() {
		// created_at is RFC 3339 text, whose fraction and zone do not sort;
		// the bound to the second without them is a prefix of every
		// stamp in that second, so it compares as a time.
		conds = append(conds, `created_at >= ?`)
		args = append(args, f.Since.UTC().Format("2006-01-02T15:04:05"))
	}
	if f.KeyContains != "" {
		conds = append(conds, `instance_key ILIKE ? ESCAPE '\'`)
		args = append(args, "%"+likeEscaper.Replace(f.KeyContains)+"%")
	}
	if f.Revision != "" {
		conds = append(conds, `revision = ?`)
		args = append(args, f.Revision)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return ` WHERE ` + strings.Join(conds, ` AND `), args
}

// likeEscaper makes a string match itself in a LIKE pattern escaped by \.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (s *Store) LastCompletedDate(ctx context.Context, wfType wf.WorkflowType) (wf.BusinessDate, error) {
	var dateStr sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(business_date) FROM workflow_runs WHERE workflow_type = ? AND status = 'completed'`,
		string(wfType),
	).Scan(&dateStr)
	if err != nil || !dateStr.Valid {
		return wf.BusinessDate{}, err
	}
	t, err := time.Parse("2006-01-02", dateStr.String)
	if err != nil {
		return wf.BusinessDate{}, err
	}
	return wf.BusinessDateFrom(t), nil
}

func (s *Store) scanRun(ctx context.Context, query string, args ...any) (*wf.RunRecord, error) {
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

func scanRunFromRow(row scannable) (*wf.RunRecord, error) {
	var r wf.RunRecord
	var wfType, dateStr, status string
	var startedAt, completedAt sql.NullString
	var createdAt string
	var errMsg, stepNames string

	err := row.Scan(&r.ID, &wfType, &dateStr, &r.Key, &r.Revision.ID, &stepNames, &status, &startedAt, &completedAt, &errMsg, &createdAt)
	if err != nil {
		return nil, err
	}

	r.WorkflowType = wf.WorkflowType(wfType)
	r.Status = wf.RunStatus(status)
	r.Error = errMsg
	if stepNames != "" {
		if err := json.Unmarshal([]byte(stepNames), &r.Revision.Steps); err != nil {
			return nil, err
		}
	}

	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		r.BusinessDate = wf.BusinessDateFrom(t)
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

func scanRunRow(rows *sql.Rows) (*wf.RunRecord, error) {
	return scanRunFromRow(rows)
}
