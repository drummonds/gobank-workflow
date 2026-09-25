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

## Demos

Both demos are Go compiled to WASM, rendered with [lofigui](https://git.bytestone.uk/hum3/lofigui).

The [daily processing demo](demo/index.html) runs a simulated daily workflow with 1,000 Easy Access savings accounts (3.5% annual rate, 10,000.00 each). Click the buttons to advance time and observe processing duration per day.

The [release pipeline demo](demo/deploy.html) models the pipeline that ships a new gobank version -- check, bump, changelog, tag, push origin, push mirror, verify install, deploy docs. The stages, and the task-plus code that runs each one for real under `tp release`, are shown below. Run it a stage at a time or to the end. Arm *Fail next stage* to inject a failure: the release halts at that stage, and resuming retries only that stage. Completed stages are never repeated, which is the same resume property the daily runner has for business dates.

![Release pipeline stages and the code that runs them](release-pipeline.svg)

Source: `release-pipeline.d2`.
