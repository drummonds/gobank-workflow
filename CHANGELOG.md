# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

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
