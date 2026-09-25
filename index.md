# gobank-workflow

Banking workflow engine -- idempotent daily processing, business date management, and persistent run tracking.

## Pages

- [Daily processing demo](demo/index.html) -- interactive simulation of daily workflow processing with timing chart
- [Release pipeline demo](demo/deploy.html) -- step through shipping a new gobank version; inject a failure and watch the resume retry only the failed stage
- [Changelog](CHANGELOG.html)
- [Roadmap](ROADMAP.html)

## Overview

gobank-workflow is the timing and orchestration layer for the [gobank](https://git.bytestone.uk/hum3/gobank) ecosystem. It wraps [gobank-products](https://git.bytestone.uk/hum3/gobank-products)' simulation engine with:

- **Idempotent daily processing** -- `Runner.RunDaily(date)` is a no-op if already completed
- **Business date management** -- decouples processing date from calendar date
- **Persistent run tracking** -- SQL-backed store records every run and per-account step results
- **Catch-up** -- `Runner.CatchUp(from, to)` fills gaps from the last completed date

## Architecture

```
Runner.RunDaily(date)
  -> Store.FindRun(daily_update, date)  -- idempotency check
  -> if completed: return nil           -- no-op
  -> Store.CreateRun(pending)
  -> sim.AdvanceToDate(date)            -- gobank-products handles business logic
  -> Observer writes StepResults
  -> Store.CompleteRun()
```

## Workflow definitions

Each definition is a kind of workflow the engine can run; each run recorded in the store is an instance of one. `Definitions` is the registry in code, and `Store.CountRuns` reports how many instances each definition has in every state. The daily processing demo shows both.

| Type | Description | Instance key |
|---|---|---|
| `daily_update` | End-of-day processing for one business date: runs the `DayProcessor` over every account and records a step per account | (`workflow_type`, `business_date`), unique |

An instance moves through these states:

| State | Meaning | Set by |
|---|---|---|
| `pending` | created, not yet started | `Store.CreateRun` |
| `running` | processing the date | `Runner.RunDaily` on start |
| `completed` | every step recorded; the date will never be re-run | `Store.CompleteRun` |
| `failed` | the `DayProcessor` returned an error; the date is eligible for a retry | `Store.FailRun` |

## Schema

The SQL schema is versioned with [gobank-db](https://gobank-db.docs.bytestone.uk/) expand/contract migrations. The SQL adapter lives in the `sqlstore` package so the core package stays free of database drivers. `sqlstore.New` applies `sqlstore.Migrations` in order and records each in `schema_migrations`; `SchemaVersion` in the core package is the version the code expects, and `sqlstore.Store.SchemaVersion(ctx)` is the version a database is at.

| Version | Name | Phase | Tables |
|---|---|---|---|
| 1 | workflow runs and steps | expand | `workflow_runs` (one row per instance, unique on type and business date), `workflow_steps` (one row per account per run) |

## Demos

Both demos are Go compiled to WASM, rendered with [lofigui](https://git.bytestone.uk/hum3/lofigui).

The [daily processing demo](demo/index.html) runs a simulated daily workflow with 1,000 Easy Access savings accounts (3.5% annual rate, 10,000.00 each). Click the buttons to advance time and observe processing duration per day.

The [release pipeline demo](demo/deploy.html) models the pipeline that ships a new gobank version -- check, bump, changelog, tag, push origin, push mirror, verify install, deploy docs. The stages, and the task-plus code that runs each one for real under `tp release`, are shown below. Run it a stage at a time or to the end. Arm *Fail next stage* to inject a failure: the release halts at that stage, and resuming retries only that stage. Completed stages are never repeated, which is the same resume property the daily runner has for business dates.

![Release pipeline stages and the code that runs them](release-pipeline.svg)

Source: `release-pipeline.d2`.
