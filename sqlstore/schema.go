package sqlstore

import db "git.bytestone.uk/hum3/gobank-db"

// Migrations is the versioned history of the gobank-workflow schema, applied
// by New through gobank-db. Append a new Migration to change the schema and
// raise wf.SchemaVersion to match; never edit one that has shipped.
var Migrations = []db.Migration{
	{Version: 1, Name: "workflow runs and steps", Phase: db.Expand, SQL: SchemaSQL},
	{Version: 2, Name: "instance keys and step status", Phase: db.Expand, SQL: keysAndStepStatusSQL},
	{Version: 3, Name: "drop date-only uniqueness", Phase: db.Contract, SQL: dropDateOnlyIndexSQL},
}

// keysAndStepStatusSQL (v2, expand) adds the instance key so a keyed
// workflow such as a release can have several instances on one business
// date, and gives steps a status and error so pipeline stages can be
// recorded. The new unique index sits alongside the old one until v3.
const keysAndStepStatusSQL = `
ALTER TABLE workflow_runs ADD COLUMN instance_key VARCHAR(100) NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_runs_type_date_key
    ON workflow_runs(workflow_type, business_date, instance_key);
ALTER TABLE workflow_steps ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'completed';
ALTER TABLE workflow_steps ADD COLUMN error_message TEXT NOT NULL DEFAULT '';
`

// dropDateOnlyIndexSQL (v3, contract) removes the (type, date) uniqueness
// that v2 superseded.
const dropDateOnlyIndexSQL = `
DROP INDEX IF EXISTS idx_workflow_runs_type_date;
`

// SchemaSQL is the DDL of schema version 1: workflow_runs and workflow_steps.
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
