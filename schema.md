# Database schema

The tables of a database migrated by `sqlstore.New` to the current schema version. The DDL is in `sqlstore/schema.go`; this page is the readable view of it, and `sqlstore`'s tests fail if a column here is missing, extra or out of order.

![workflow_runs, workflow_steps and schema_migrations, and the key from steps to runs](schema.svg)

Columns are listed in the order the database has them: those added by a later migration come last, with the version that added them. Types are as declared; under pglike (SQLite) every `TIMESTAMPTZ` is stored as text, written fixed-width in UTC (`2006-01-02T15:04:05.000000000Z`) so that text order is time order.

### `workflow_runs`

One row per workflow instance. An instance is identified by (`workflow_type`, `business_date`, `instance_key`), unique together: `instance_key` is empty for a date-keyed workflow such as `daily_update`, and the version for a release. A retry of an unfinished instance replaces its row.

| Column | Type | Since | Meaning |
|---|---|---|---|
| `id` | `TEXT` primary key | v1 | the run's id |
| `workflow_type` | `TEXT` | v1 | the definition this is an instance of |
| `business_date` | `DATE` | v1 | the business date it processes |
| `status` | `TEXT` | v1 | `pending`, `running`, `waiting`, `completed` or `failed` |
| `started_at` | `TIMESTAMPTZ` | v1 | when the latest attempt started |
| `completed_at` | `TIMESTAMPTZ` | v1 | when it completed or failed; null while in hand |
| `error_message` | `TEXT` | v1 | why it failed; empty otherwise |
| `created_at` | `TIMESTAMPTZ` | v1 | when the instance was created; `ListRuns` orders by it, newest first |
| `instance_key` | `VARCHAR(100)` | v2 | distinguishes instances of one type on one date |
| `revision` | `VARCHAR(16)` | v4 | id of the revision (step list) the latest attempt ran; empty before v4 |
| `step_names` | `TEXT` | v4 | that revision's step names as a JSON array, so the run renders without the code that ran it |

Indexes: unique on (`workflow_type`, `business_date`, `instance_key`); on `created_at`.

### `workflow_steps`

One row per step recorded for a run: an account's end-of-day processing (`account_id` set), or a stage of a pipeline (`account_id` empty). Money is an integer amount scaled by `exponent`: 100004 at exponent −2 is 1000.04.

| Column | Type | Since | Meaning |
|---|---|---|---|
| `id` | `TEXT` primary key | v1 | the step's id |
| `run_id` | `TEXT` → `workflow_runs.id` | v1 | the run it belongs to |
| `account_id` | `TEXT` | v1 | the account processed; empty for a pipeline stage |
| `step_name` | `TEXT` | v1 | the step or stage |
| `opening_balance` | `INTEGER` | v1 | balance before the step |
| `closing_balance` | `INTEGER` | v1 | balance after it |
| `interest_amount` | `INTEGER` | v1 | interest the step accrued |
| `exponent` | `INTEGER` | v1 | decimal exponent of the three amounts (default −2) |
| `duration_ns` | `INTEGER` | v1 | how long the step took, in nanoseconds |
| `created_at` | `TIMESTAMPTZ` | v1 | when it was recorded; `ListSteps` orders by it |
| `status` | `VARCHAR(20)` | v2 | `completed`, `failed` or `waiting` |
| `error_message` | `TEXT` | v2 | why the step failed; empty otherwise |

Indexes: on `run_id`; on `created_at`.

### `schema_migrations`

Owned by [gobank-db](https://gobank-db.docs.bytestone.uk/): one row per migration applied. The highest `version` is the version the database is at (`sqlstore.Store.SchemaVersion`).

| Column | Type | Since | Meaning |
|---|---|---|---|
| `version` | `INTEGER` primary key | — | the migration's version |
| `name` | `VARCHAR(200)` | — | its name |
| `phase` | `VARCHAR(20)` | — | `expand`, `cutover` or `contract` |
| `applied_at` | `TIMESTAMP` | — | when it was applied |
