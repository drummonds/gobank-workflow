# gobank-workflow

Banking workflow engine -- idempotent daily processing, business date management, and persistent run tracking.

## Pages

- [Demo app](app/) -- the workflow engine as an app, running in your browser: Workflows menu with instances and definitions, a simulation driver to make days happen, and a DB explorer over the live schema
- [Daily processing demo](demo/index.html) -- interactive simulation of daily workflow processing with timing chart
- [Release pipeline demo](demo/deploy.html) -- step through shipping a new gobank version; inject a failure and watch the resume retry only the failed stage
- [Writing workflows](writing-workflows.html) -- the authoring contract: definitions, stages and waiting, resume and what it demands of a stage, what the host supplies, testing, drawing, a worked example and a checklist
- [Database schema](schema.html) -- the tables, their keys and every column, with the migration that added it
- [Comparison with other workflow engines](comparison.html) -- Go libraries and the larger systems (Temporal, Camunda, Flowable, Spring Batch) set against this one: feature matrices, the closest relatives, and the gaps worth closing
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

Each definition is a kind of workflow the engine can run; each run recorded in the store is an instance of one. `Definitions` is the registry in code: it starts empty and each host program appends the workflows it runs, so its pages show those and no others. `Store.CountRuns` reports how many instances each definition has in every state. The demo app registers these four:

| Type | Description | Instance key | Steps |
|---|---|---|---|
| `daily_update` | End-of-day processing for one business date: the `DayProcessor` runs over every account | business date | `end_of_day`, one per account |
| `release` | Ship a new version through the release pipeline; a failed stage stops the release and the next run resumes from it | version | check → bump → changelog → tag → push origin → push mirror → verify install → deploy docs |
| `payment_batch` | Pay a file of five payments; each payment is a child instance under the batch, which settles once they have all finished and fails if any failed | batch reference (`PAY-0001`) | receive → start payments (sub-process, one `payment` per payment) → settle |
| `payment` | Pay one payment of a batch out of the paying account | batch reference/position (`PAY-0001/3`) | check funds → post |

The batch and its payments are a master–detail pair: the batch's instance page lists its payments as children with their state, and each payment's page links up to its batch.

The diagrams below show only how the steps flow; they are generated from the registry by `cmd/defd2` and rendered with d2 at docs build time, so they cannot drift from the code. Each step is a link to the code that performs it. The demo app shows the same pictures, colours an instance's steps by state, and carries the detail in tables.

![daily_update definition](definitions/daily_update.svg)

![release definition](definitions/release.svg)

![payment_batch definition](definitions/payment_batch.svg)

![payment definition](definitions/payment.svg)

A definition is a sequence of steps; an instance is one run of it, identified within its type by the instance key, and each step recorded against the instance is one of the definition's steps. A retry is a new attempt at the same instance, not a new instance: `Runner.RunDaily` reprocesses every account, `Runner.RunPipeline` resumes from the failed stage.

An instance moves through these states:

| State | Meaning | Set by |
|---|---|---|
| `pending` | created, not yet started | `Store.CreateRun` |
| `running` | processing the date, or a pipeline stage | `Runner.RunDaily` on start; `Store.ResumeRun` when a waited-on condition holds |
| `waiting` | parked on a stage's `Until` condition; the host's next call re-enters the same attempt | `Store.WaitRun` |
| `completed` | every step recorded; the date will never be re-run | `Store.CompleteRun` |
| `failed` | the `DayProcessor` or a pipeline stage returned an error; the instance is eligible for a retry | `Store.FailRun` |

[Writing workflows](writing-workflows.html) is the authoring contract for the next one.

## Schema

The SQL schema is versioned with [gobank-db](https://gobank-db.docs.bytestone.uk/) expand/contract migrations. The SQL adapter lives in the `sqlstore` package so the core package stays free of database drivers. `sqlstore.New` applies `sqlstore.Migrations` in order and records each in `schema_migrations`; `SchemaVersion` in the core package is the version the code expects, and `sqlstore.Store.SchemaVersion(ctx)` is the version a database is at.

| Version | Name | Phase | Change |
|---|---|---|---|
| 1 | workflow runs and steps | expand | `workflow_runs` (one row per instance), `workflow_steps` (one row per step of a run) |
| 2 | instance keys and step status | expand | `workflow_runs.instance_key` with a unique index on (type, date, key) alongside the old one; `workflow_steps.status` and `error_message` |
| 3 | drop date-only uniqueness | contract | drops the (type, date) unique index that v2 superseded |
| 4 | run revisions | expand | `workflow_runs.revision` and `step_names`: the revision the latest attempt ran |
| 5 | fixed-width created_at | expand | pads `created_at` stamps written before v5 to the fixed-width layout, so text order is time order |

Versions 2 and 3 are one expand/contract pair: gobank-db's lint refuses a `DROP INDEX` in an expand migration, so the replacement index is added first and the old one removed in a separate contract step.

The tables and their columns are on the [database schema](schema.html) page.

## Demo app

The [demo app](app/) is the same Go program as `task demo` serves on a workstation, compiled to WASM and run inside a service worker with [lofigui](https://git.bytestone.uk/hum3/lofigui): one `http.ServeMux`, two entry points. Its database is pglike in the browser's memory, migrated to the current schema version at start.

| Menu | Page | Shows |
|---|---|---|
| Workflows → Instances | `workflows/instances` | the top-level runs in the store, active first then most recent; a child run is on its parent's page unless the filter names its purpose, workflow or key; each opens its state and the per-account steps |
| Workflows → Definitions | `workflows/definitions` | the `Definitions` registry with instance counts by state; each opens its instances |
| Simulation | `simulation` | the driver: advance 1, 7 or 30 business days, arm a failure for the next day, ship the next version through the release pipeline (optionally failing at a chosen stage), pay the next payment batch (optionally with one payment failing to post), reset |
| DB Explorer | `internal/explorer` | tables, columns, indexes and rows of the live schema, including `schema_migrations` |
| About | `about` | versions, schema version expected and applied, runtime |

A failed day stays current, a failed release keeps its version and a failed batch keeps its reference: the next advance, ship or pay retries the same instance, so there is always one instance per business date, one per version and one per batch. Paying a batch again retries its failed payments first, then settles the batch.

## Demos

Both demos are Go compiled to WASM, rendered with [lofigui](https://git.bytestone.uk/hum3/lofigui).

The [daily processing demo](demo/index.html) runs a simulated daily workflow with 1,000 Easy Access savings accounts (3.5% annual rate, 10,000.00 each). Click the buttons to advance time and observe processing duration per day.

The [release pipeline demo](demo/deploy.html) models the pipeline that ships a new gobank version -- check, bump, changelog, tag, push origin, push mirror, verify install, deploy docs. The stages, and the task-plus code that runs each one for real under `tp release`, are shown below. Run it a stage at a time or to the end. Arm *Fail next stage* to inject a failure: the release halts at that stage, and resuming retries only that stage. Completed stages are never repeated, which is the same resume property the daily runner has for business dates.

![Release pipeline stages and the code that runs them](release-pipeline.svg)

Source: `release-pipeline.d2`.
