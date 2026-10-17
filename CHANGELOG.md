# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added
- `RunFilter` (#11): `Store.ListRuns` takes a filter — workflow types,
  statuses, created since (to the second), key contains (ignoring case,
  `%` and `_` literal), revision, limit — and the SQL store filters in
  the query. `storetest.FilteredRuns` is the contract.

### Changed
- `Store.ListRuns(ctx, limit)` is now `ListRuns(ctx, RunFilter)`
  (breaking; `RunFilter{Limit: n}` is the old call), and so is
  `view.Reader`'s.
- Instances page is a work queue (#11): the instances in hand (pending,
  running, waiting, failed) first, then the newest completed, n in all
  (default 50, at most 500), grouped by purpose with the purpose's
  newest instance as its headline. A GET form filters by purpose,
  workflow, status, since, key contains and n, on the server and in
  the WASM build alike. Drill-down is by link: purpose → workflow →
  revision → instance. A definition's page shows the same queue for
  its workflow in place of its own 500-run list.

## [0.6.0] - 2026-10-17

 - view package: reusable workflow pages (#21); demo account paging (#18)

### Added
- `view` package (#21): the workflow pages (instances, an instance laid
  over the steps it ran, definitions, a definition with its instances,
  definition pictures and each instance as d2) as one `http.Handler` over a
  read-only store. The host wraps pages in its layout via `Frame` and adds
  type-specific content via `Detail`. A stage shows its latest record by
  time, whatever order the store lists them in. Timestamps carry the date
  to the millisecond, and durations round to their scale, so runs of hours read as well as
  runs of milliseconds.

### Changed
- Demo app uses `view`; its account table and paging are its `Detail`.
- Demo app: an instance's account steps are paged, 50 per page with
  Previous/Next and page links (`?page=N`, clamped), in account order (#18).
  Previously only the first 50 were shown, in record order.

## [0.5.0] - 2026-10-16

 - Purpose and revision (#4): runs record and render the step list they ran

### Added
- Purpose and revision (#4). A `Purpose` (name, description) on each
  `Definition` says what the workflow achieves; it lives in the registry
  only, and the demo's definitions pages show it. A `Revision` is the
  step list an attempt ran: `RunPipeline` and `RunDaily` record it on the
  run at creation and again at each restart, so a retry under changed
  code carries the new list. Instance pages and `diagram.Instance` render
  from the recorded revision through `Definition.StepsFor`, and the demo
  flags an instance whose revision is not the current one. Schema v4
  (expand) adds `revision` and `step_names` to `workflow_runs`; runs from
  before it render from the live definition as before.

### Changed
- `Store.RestartRun` takes the new attempt's `Revision` (breaking for
  `Store` implementations outside this module).

## [0.4.0] - 2026-10-15

 - Compensation: Stage.Undo reverses a completed stage when a later one fails (#3)

### Added
- Compensation (#3): `Stage.Undo` reverses a completed stage's work when
  a later stage fails. The runner undoes the completed stages before the
  failure in reverse order, nearest first, removes each undone stage's
  step so the next attempt runs it again, and names what was undone in
  the run's error; an `Undo` that fails is reported, keeps its step and
  stops the undoing there. A stage without `Undo` keeps its step and its
  work as before. `Store.DeleteStep` removes a run's steps by name, in
  the memory and SQL stores and the `storetest` contract; no schema
  change. Written for gobank-deploy's perf and scenario runs, whose
  created server bills while a failed run sits.

### Roadmap
- Compensation (#3): `Stage.Undo` reverses a completed stage when a
  later one fails, so the next attempt starts from the first stage whose
  work no longer stands

## [0.3.0] - 2026-10-09

 - update documentation

### Added
- Docs: writing workflows (`writing-workflows.md`), the authoring contract: vocabulary, declaring a definition, the stage outcome table, resume and what it demands, what the host supplies, testing, drawing, a worked example compiled as `example_test.go`, and a checklist
- `runnertest` package: `OnlyRun` and `Steps`, the store checks a workflow's own tests make, extracted from gobank-deploy's three flow harnesses

### Roadmap
- Writing workflows: the authoring contract as a documentation section,
  with a worked example compiled as `example_test.go` and the
  `runnertest` kit

## [0.2.0] - 2026-10-08

 - Release prep

### Added
- Waiting stages: `Stage.Until` is a condition the stage waits on. While it is false the run is parked as `waiting` (a new run and step status) and `RunPipeline` returns; the caller's next call re-evaluates it and continues the same attempt, so a wait survives a restart and only the status is persisted. Once it holds, `Run` executes if set. `Store.WaitRun` and `Store.ResumeRun` are the transitions; no schema change
- Docs: comparison with other workflow engines (`research/comparison.md`): Go libraries and the larger systems set against this one, with feature matrices, the closest relatives and a table of gaps and candidate additions

### Roadmap
- Waiting stages: `Stage.Until`, one mechanism for durable waits and
  human tasks; the caller's tick re-enters waiting runs

## [0.1.3] - 2026-09-27

 - Release pipeline runner, keyed instances, step status, definition diagrams, demo app

### Added
- Demo app (`cmd/demo`, `task demo`, and in the docs as a WASM service-worker build): About, Workflows menu with time-ordered instances and a definitions explorer, instance pages showing state and steps, a simulation driver, and go-dbexplorer over the live schema
- `internal/demoaccounts`: the shared demo DayProcessor, with an injectable one-off failure

- `release` workflow definition and `Runner.RunPipeline`: a keyed (version) multi-step instance whose steps are the pipeline stages; a failed stage stops the run and the next run resumes from it. Available from the demo app's simulation page with a choice of stage to fail
- `Definition.Steps` and `InstanceKey`: definitions describe their steps; the app's definition pages lead with them
- `RunRecord.Key`, `StepResult.Status` and `Error`; `Store.FindRunByKey`, `RestartRun`, `DeleteSteps`; `storetest` package with the Store contract checks
- `diagram` package and `cmd/defd2`: d2 source generated from the definitions registry, one diagram per definition, and an instance diagram with steps coloured by state; rendered into the docs and shown in the demo app, where an instance's d2 is downloadable
- Schema v2 (expand: instance key, step status) and v3 (contract: drop the date-only unique index)

### Changed
- A retry is a new attempt at the same instance: `RunDaily` restarts the failed run in place instead of creating another, so the instance id is stable

### Fixed
- A retry after a failed run no longer leaves two instances for one date in the memory store, and is no longer rejected by the SQL store's unique index

### Roadmap
- Pipelines: `RunPipeline`, an instance keyed by version whose steps
  are the stages, resumed from the failed stage; d2 from the registry
  (`cmd/defd2`); the demo app

## [0.1.2] - 2026-09-25

 - Schema versions, workflow definitions with instance counts by state

### Added
- Schema versioning through gobank-db migrations: `sqlstore.Migrations`, the `SchemaVersion` constant and `sqlstore.Store.SchemaVersion(ctx)`; `sqlstore.New` records applied versions in `schema_migrations`
- `Definitions` registry of workflow types and `Store.CountRuns`, the number of instances of each definition in every state; shown on the daily processing demo with the schema version
- Docs: workflow definitions, instance states and schema versions on the landing page

### Changed
- SQL store moved to the `sqlstore` package (`sqlstore.New`, `sqlstore.Store`); the core package no longer links database drivers, which keeps the WASM demo small

### Roadmap
- End-of-day processing: `RunDaily` idempotent per business date,
  `CatchUp` over a gap, the SQL store with its versioned schema, the
  `Definitions` registry

## [0.1.1] - 2026-09-25

 - Release pipeline demo, docs site, module path moved to git.bytestone.uk

### Added
- Release pipeline demo (`demo/deploy.html`): steps through shipping a new gobank version, with injected failure and resume that retries only the failed stage
- Release pipeline diagram (`release-pipeline.d2`, rendered by `task docs:d2`): the eight stages and the task-plus code that runs each under `tp release`
- Docs published at https://gobank-workflow.docs.bytestone.uk/

### Changed
- Module path is now `git.bytestone.uk/hum3/gobank-workflow` (was codeberg.org); go-postgres dependency moved to `git.bytestone.uk/hum3/go-postgres` v0.5.13

## [0.1.0] - 2026-03-27

 - Release prep

### Added
- Initial project scaffold
