# Roadmap

The workflow engine for the gobank family: definitions drawn from the
code, instances keyed by business date or version, step results that are
domain figures, state in pglike under the bank's own versioned schema.
`research/comparison.md` sets it against the other engines; its gaps
table is the backlog of engine features, and the items below are the
ones with a consumer now.

The roadmap is a set of named epics, each a list of items, and every
item an open issue labelled `roadmap` on the Forgejo (task-plus
`tp roadmap`). The forge says what exists and what is done; this file
groups and describes it. An item leaves the file when its issue is
closed, landing in the changelog under the version that finished it.
Epic order is file order, not a sequence.

The engine is its own runner on pglike, not a go-workflows integration
as planned for v0.1, so that its state is bank data under the
contract-view rule, which a replay engine's opaque history would not
be. The banking workflows themselves (payment settlement, account
opening and closing, regulatory reporting, daily interest accrual) are
gobank's and its product libraries' items; this roadmap is the engine
they run on.

## The workflow model

What a workflow is and what a run records of it.

- #4 Purpose and revision — two levels around the workflow, so the
  hierarchy is purpose → workflow → revision → instance → step. A
  `Purpose` (name, description) on each definition is what is
  achieved, registry only; the instance key stays per workflow until
  #16. A revision is the step list an attempt ran, recorded on the
  run, so an instance that outlives its code still renders from what
  it ran
- #5 Attempt history (comparison, "now") — an `attempt` number on
  `workflow_steps` kept across retries, instead of deleting the failed
  steps; the instance page shows the last attempt by default
- #6 Retention purge — `Definition.Retention` (zero means keep, the
  default, because end-of-day runs are bank records) and
  `Store.PurgeRuns`, which deletes a (workflow, revision) group of
  runs and their steps only when the revision is absent from the
  registry, every instance in the group is completed, and the
  newest completed-at is older than the retention. One running,
  waiting or failed instance keeps the whole revision; failed is not
  resolved until retried or abandoned, and abandoning is a status
  we do not have
- #16 Instance key per purpose (deferred from #4) — the key moves from
  `Definition` to `Purpose`, so a key completed by any of a purpose's
  workflows is done for all of them. Waits for a purpose with two
  workflows; runner-enforced only, so #7's guard follows it

## Running

How runs are started, guarded, retried and resumed.

- #7 Concurrent runner guard (comparison, "now") — `ErrAlreadyRunning`
  from `RestartRun` unless the run is stale, judged by a
  `heartbeat_at` column: one running instance per workflow and key,
  per purpose and key once #16 lands. Needed once several processes
  share a store
- #8 Scheduling — a `schedules` table and a tick, next due date, skip
  while the previous run is still running; `CatchUp` is the backfill.
  The v0.1 "configurable scheduling" item, now shaped
- #9 Per-step retry with backoff, when a network stage hurts
- #10 Resume mid-pass for the daily run, with gobank's stage 3d
- #14 Abandoned status — `abandoned`, an operator's terminal decision
  that a failed instance will not be finished, with who and why;
  failed stays the retryable observation. The queue (#11) drops
  abandoned runs and #6's purge can treat them as resolved. Abandon
  does not compensate: `Undo` stays a separate action
- #15 Panic recovery — a panicking stage or processor fails its run
  (`panic: <value>` and the stack as the error, then compensation)
  instead of killing the process and leaving the run `running`. No
  `panicked` status: the cause carries it. A process that dies
  outright is #7's heartbeat

## Pages

What the host shows of the engine and what an operator does through it.

- #11 Workflows page as a work queue — the default view is the current
  instances, running, waiting and failed first, then the newest
  completed, capped at n and grouped by purpose with each purpose's
  newest instance as its headline ("upgrade: 0.4.2 running via
  blue-green"). Filter is a GET form (purpose, workflow, status,
  since, key contains, n), served and in WASM alike, backed by a
  `RunFilter` on `Store.ListRuns` in place of the bare limit so the
  filtering is SQL. Drill-down is purpose → workflow (revisions as a
  filter) → instance → steps. Replaces the flat 500-row Instances
  page and the per-type list repeated on Definitions
- #12 Panels from native code — `Definition.Status(run, steps) string`
  returns HTML for the instance page through `lofigui.HTML`,
  freeform because its content is domain figures; the engine stays
  free of net/http and the host owns the route. An `Input` is
  declarative: the waiting step it completes, its fields, and an
  `Apply` that does the store write; the host renders the Bulma
  form and the waiting stage's `Until` sees the write on the next
  tick. Inputs are the comparison's "human task" row and follow
  waiting stages; status panels can come first. `cmd/defd2` draws
  inputs as entry points
