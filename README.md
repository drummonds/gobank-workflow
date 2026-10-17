# gobank-workflow

Workflow engine for the gobank family: definitions drawn from the code, instances keyed by business date or version, step results that are domain figures, state in pglike under the bank's own versioned schema.

Part of the [gobank](https://git.bytestone.uk/hum3/gobank) family of libraries.

## Overview

gobank-workflow provides:

- **Daily Tasks** — interest accrual, end-of-day processing, balance snapshots
- **Scheduled Jobs** — recurring tasks on configurable schedules
- **Workflow Orchestration** — multi-step banking processes (account opening, payments settlement)
- **Workflow pages** (`view`) — instances and definitions as HTML an app mounts under its own path and wraps in its own layout; a `Detail` hook adds what only the app knows about a workflow type

## Status

Early development. See [issues](https://git.bytestone.uk/hum3/gobank-workflow/issues) for planned work.

## Links

- **Documentation:** https://gobank-workflow.docs.bytestone.uk/
- **Source:** https://git.bytestone.uk/hum3/gobank-workflow
- **Mirror:** https://github.com/drummonds/gobank-workflow
