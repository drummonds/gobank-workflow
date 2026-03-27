# gobank-workflow

Banking workflow engine -- idempotent daily processing, business date management, and persistent run tracking.

## Pages

- [WASM Demo](demo/index.html) -- interactive simulation of daily workflow processing with timing chart
- [Changelog](CHANGELOG.html)
- [Roadmap](ROADMAP.html)

## Overview

gobank-workflow is the timing and orchestration layer for the [gobank](https://codeberg.org/hum3/gobank) ecosystem. It wraps [gobank-products](https://codeberg.org/hum3/gobank-products)' simulation engine with:

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

## Demo

The [WASM demo](demo/index.html) runs a simulated daily workflow with 1,000 Easy Access savings accounts (3.5% annual rate, 10,000.00 each). Click the buttons to advance time and observe processing duration per day.
