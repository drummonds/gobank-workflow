package workflow

import (
	"database/sql"
	"strings"
)

// SchemaSQL is the DDL for gobank-workflow tables.
const SchemaSQL = `
CREATE TABLE IF NOT EXISTS workflow_runs (
    id TEXT PRIMARY KEY,
    workflow_type TEXT NOT NULL,
    business_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_runs_type_date
    ON workflow_runs(workflow_type, business_date);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_created
    ON workflow_runs(created_at);

CREATE TABLE IF NOT EXISTS workflow_steps (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES workflow_runs(id),
    account_id TEXT NOT NULL,
    step_name TEXT NOT NULL,
    opening_balance INTEGER NOT NULL DEFAULT 0,
    closing_balance INTEGER NOT NULL DEFAULT 0,
    interest_amount INTEGER NOT NULL DEFAULT 0,
    exponent INTEGER NOT NULL DEFAULT -2,
    duration_ns INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_run
    ON workflow_steps(run_id);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_created
    ON workflow_steps(created_at);
`

func execStatements(db *sql.DB, ddl string) error {
	for stmt := range strings.SplitSeq(ddl, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
