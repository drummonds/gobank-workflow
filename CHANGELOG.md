# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

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
