# Roadmap

The workflow engine for the gobank family: definitions drawn from the
code, instances keyed by business date or version, step results that are
domain figures, state in pglike under the bank's own versioned schema.
`research/comparison.md` sets it against the other engines; its gaps
table is the backlog of engine features, and the rows below are the ones
with a consumer now.

## Now

- [x] (unreleased) Compensation: `Stage.Undo` reverses a completed stage
      when a later one fails, the undone steps go so the next attempt
      starts from the first stage whose work no longer stands, and the
      run's error names what was undone. The "only if observed" item
      from Later, observed not as a half-shipped release but as a
      gobank-deploy perf box billing while its failed run sat (#3)
- [x] (unreleased) Waiting stages: a stage may wait on a condition
      (`Until func(ctx) (bool, error)`) before running. A false
      condition marks the step and run `waiting` and returns; a caller's
      tick re-enters waiting runs, so the wait survives a restart and
      the condition function is never persisted, only the status. An
      event (an approval, a scheme response) is a store write whose
      condition is "is it recorded", so the comparison's "durable wait"
      and "human task" rows are one story. Pulled forward from "with
      payments" by gobank-deploy: it replaces the hand-rolled sleep/poll
      in its demo `serve` and drill `prepare`, and is the promote gate of
      its release pipeline (gobank-deploy Story 1m). Decided: one
      mechanism, always return; the polling cadence is the caller's, so
      there is no `Every` on the stage. Re-entry continues the same
      attempt (`ResumeRun`), where a retry starts a new one
      (`RestartRun`)
- [x] (unreleased) Writing workflows: a documentation section that is the authoring
      contract, for a human or an agent writing the next workflow from it
      alone. What a definition is (name, instance key, steps with
      descriptions and source links); what a stage is (`Run`, or `Until`
      once waiting stages land); how resume works and what that demands
      of a stage (idempotent, resumable from any step, state on the
      system under control rather than in the run); what the store
      records and what the host must supply (a store, the tick, a log
      per step, the pages); how to test one (fakes for clock, sleep and
      poll, the `storetest` contract, a runner test kit to extract from
      gobank-deploy's three flows); how to draw it (`cmd/defd2`). A
      worked example and a checklist, rendered by `docs:build`, and the
      consuming projects' `CLAUDE.md` point at it so an agent loads it
      before writing a flow. Decided: the worked example is neither the
      `release` demo nor the drill but a promotion pipeline compiled as
      `example_test.go`, so it cannot drift; the drill is referenced.
      Still to do: gobank-deploy's `CLAUDE.md` pointer, and its three
      harnesses onto `runnertest` once this ships
- [ ] Purpose and revision: two levels around the workflow, so the
      hierarchy is purpose → workflow → revision → instance → step. A
      `Purpose` (name, description, instance key) is what is achieved;
      its workflows are the ways of achieving it (upgrade: redeploy,
      drill, blue-green, in-place), one purpose per workflow. The
      instance key moves from `Definition` to `Purpose`, so "is 0.4.2
      deployed" is answered across all its workflows, and the
      concurrent runner guard is scoped to the purpose. A revision is
      the step list as the code had it, a hash of the step names
      recorded on each run, so an instance that outlives its code still
      renders from what it recorded (the instance page reads the live
      release stages today). No store column for purpose, one for
      revision. Open: whether a drill is the upgrade purpose or a
      "rehearse upgrade" purpose reusing the same workflows, decided by
      whether its key is the target version or the drill date
- [ ] Workflows page as a work queue: the default view is the current
      instances, running, waiting and failed first, then the newest
      completed, capped at n and grouped by purpose with each purpose's
      newest instance as its headline ("upgrade: 0.4.2 running via
      blue-green"). Filter is a GET form (purpose, workflow, status,
      since, key contains, n), served and in WASM alike, backed by a
      `RunFilter` on `Store.ListRuns` in place of the bare limit so the
      filtering is SQL. Drill-down is purpose → workflow (revisions as a
      filter) → instance → steps. Replaces the flat 500-row Instances
      page and the per-type list repeated on Definitions

## Next

- [ ] Panels from native code: `Definition.Status(run, steps) string`
      returns HTML for the instance page through `lofigui.HTML`,
      freeform because its content is domain figures; the engine stays
      free of net/http and the host owns the route. An `Input` is
      declarative: the waiting step it completes, its fields, and an
      `Apply` that does the store write; the host renders the Bulma
      form and the waiting stage's `Until` sees the write on the next
      tick. Inputs are the comparison's "human task" row and follow
      waiting stages; status panels can come first. `cmd/defd2` draws
      inputs as entry points
- [ ] Retention purge: `Definition.Retention` (zero means keep, the
      default, because end-of-day runs are bank records) and
      `Store.PurgeRuns`, which deletes a (workflow, revision) group of
      runs and their steps only when the revision is absent from the
      registry, every instance in the group is completed, and the
      newest completed-at is older than the retention. One running,
      waiting or failed instance keeps the whole revision; failed is not
      resolved until retried or abandoned, and abandoning is a status
      we do not have
- [ ] Concurrent runner guard (comparison, "now"): `ErrAlreadyRunning`
      from `RestartRun` unless the run is stale, judged by a
      `heartbeat_at` column, scoped to the purpose: one running instance
      per purpose and key. Needed once several processes share a store
- [ ] Attempt history (comparison, "now"): an `attempt` number on
      `workflow_steps` kept across retries, instead of deleting the failed
      steps; the instance page shows the last attempt by default
- [ ] Scheduling: a `schedules` table and a tick, next due date, skip
      while the previous run is still running; `CatchUp` is the backfill.
      The v0.1 "configurable scheduling" item, now shaped

## Later

- Payment settlement workflows, on waiting stages (awaiting a scheme
  response)
- Account opening / closing lifecycle
- Regulatory reporting generation
- Per-step retry with backoff, when a network stage hurts
- Resume mid-pass for the daily run, with gobank's stage 3d

## Done

- [x] (v0.1.2) End-of-day processing: `RunDaily` idempotent per business
      date, `CatchUp` over a gap, the SQL store with its schema versioned
      through gobank-db migrations, the `Definitions` registry
- [x] (v0.1.3) Pipelines: `RunPipeline`, an instance keyed by version
      whose steps are the stages, resumed from the failed stage; d2 from
      the registry (`cmd/defd2`); the demo app with instances,
      definitions and a simulation driver
- Dropped: the go-workflows integration planned for v0.1. The engine is
  its own runner on pglike so that its state is bank data under the
  contract-view rule, which a replay engine's opaque history would not
  be
- Delegated: daily interest accrual is the `DayProcessor`'s work, in
  gobank-products and the demo accounts, not the engine's
