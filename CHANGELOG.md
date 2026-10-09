# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added
- Docs: writing workflows (`writing-workflows.md`), the authoring contract: vocabulary, declaring a definition, the stage outcome table, resume and what it demands, what the host supplies, testing, drawing, a worked example compiled as `example_test.go`, and a checklist
- `runnertest` package: `OnlyRun` and `Steps`, the store checks a workflow's own tests make, extracted from gobank-deploy's three flow harnesses

## [0.2.0] - 2026-10-08

 - Release prep

### Added
- Waiting stages: `Stage.Until` is a condition the stage waits on. While it is false the run is parked as `waiting` (a new run and step status) and `RunPipeline` returns; the caller's next call re-evaluates it and continues the same attempt, so a wait survives a restart and only the status is persisted. Once it holds, `Run` executes if set. `Store.WaitRun` and `Store.ResumeRun` are the transitions; no schema change
- Docs: comparison with other workflow engines (`research/comparison.md`): Go libraries and the larger systems set against this one, with feature matrices, the closest relatives and a table of gaps and candidate additions

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

## [0.1.2] - 2026-09-25

 - Schema versions, workflow definitions with instance counts by state

### Added
- Schema versioning through gobank-db migrations: `sqlstore.Migrations`, the `SchemaVersion` constant and `sqlstore.Store.SchemaVersion(ctx)`; `sqlstore.New` records applied versions in `schema_migrations`
- `Definitions` registry of workflow types and `Store.CountRuns`, the number of instances of each definition in every state; shown on the daily processing demo with the schema version
- Docs: workflow definitions, instance states and schema versions on the landing page

### Changed
- SQL store moved to the `sqlstore` package (`sqlstore.New`, `sqlstore.Store`); the core package no longer links database drivers, which keeps the WASM demo small

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
