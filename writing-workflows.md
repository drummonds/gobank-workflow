# Writing workflows

The authoring contract: what a workflow on this engine is, what the engine
guarantees, what a workflow must supply in return, and how to test and
draw one. A human or an agent should be able to write the next workflow
from this page alone. The worked example at the end is `example_test.go`,
compiled and run by `go test`, so it cannot drift from the engine.

## Vocabulary

| Term | Meaning | In code |
|---|---|---|
| purpose | what a workflow achieves; its workflows are the ways of achieving it, one purpose per workflow | `Purpose`, on `Definition` |
| workflow | a kind of work the engine can run, declared once in code | `Definition`, registered in `Definitions` |
| revision | a workflow's step list as the code had it for an attempt; the same names in the same order are the same revision | `Revision`, recorded on `RunRecord` |
| instance | one run of a workflow, identified within its type by its key | `RunRecord`; `Key` for keyed workflows, `BusinessDate` for date-keyed |
| step | one unit recorded against an instance: a stage, or one account's end of day | `StepResult` |
| stage | a step of a pipeline as code: a name, a `Run`, an `Until` or both, an `Undo` when its work must not outlive a failed run, and `Final` when its work must never be redone | `Stage` |
| attempt | one call of the runner for an instance; a retry is a new attempt at the same instance, never a new instance | `StartedAt` is the attempt's start |

An instance renders from the revision it recorded, not from the live
definition, so it stays right after the code changes: `Definition.StepsFor`
is its step list, described by the definition where a name still exists.

## Declaring a workflow

A definition is the workflow as the registry, the pages and the diagrams
see it. Declare it next to the code that runs it and append it to
`Definitions` in an `init`, as gobank-deploy does.

| Field | What it is | Rule |
|---|---|---|
| `Type` | the workflow's name | lower snake case; unique across every program sharing a store |
| `Purpose` | what the workflow achieves: a `Name` and a `Description` | share one `Purpose` value between the workflows that are ways of achieving the same thing |
| `Description` | what one instance achieves and what a retry does | one or two sentences |
| `InstanceKey` | what identifies one instance, as prose | "version", "environment@expiry", "business date" |
| `Steps` | every step the runner will record, in order | `Name` matches the stage's `Name` exactly; `Description` says what it does; `Source` is an `https://` link to the code |

`definition_test.go` checks the registry: every definition has a type and a
description, every step links to its code. The instance key is the one
idempotency rule, so choose it from the system under control, not from
the clock: a demo environment is keyed by its expiry, which the server
carries, so a restart finds the same instance.

## Stages

A pipeline is `Runner.RunPipeline(ctx, type, key, date, stages)`. Each
stage runs in order and is recorded as a step with the stage's name. A
stage has a `Run`, an `Until`, or both: `Until` is a condition the stage
waits on, and `Run` executes once it holds. `Undo` reverses the stage's
work when a later stage fails.

| `Until` | `Run` | Step recorded | Run status | `RunPipeline` returns |
|---|---|---|---|---|
| unset | returns nil | completed | continues | |
| unset | returns error | failed, with the error | failed | nil, the error |
| false | not called | waiting, once, when the wait began | waiting | the run, nil |
| true | unset | completed | continues | |
| true | returns nil | completed | continues | |
| true | returns error | failed | failed | nil, the error |
| error | not called | failed, with the error | failed | nil, the error |
| completed in an earlier attempt | not called | kept | continues | |
| after the last stage | | | completed | the run, nil |
| key already completed | not called | | | nil, nil |

When a stage fails, the completed stages before it are undone in reverse
order, nearest first, each by its own `Undo`; a stage without one keeps
its step and its work.

| Completed stage before the failure | `Undo` | Step | The undoing |
|---|---|---|---|
| has no `Undo` | not called | kept | passes over it |
| `Undo` returns nil | called | removed | continues to the one before |
| `Undo` returns error | called | kept, its work may still stand | stops; the error is reported with the failure |

The run's error names the failed stage, then what was undone and any
undo that failed: `days: the demo stopped answering (undone: add,
create); undo add: delete server: forbidden`. Give a stage an `Undo` when
its work bills or blocks while the run sits failed: a server created, a
lock taken. A tag pushed or a figure recorded is left to stand.

A waiting run is parked, not held: `RunPipeline` returns, and the host
calls it again on its own tick. The cadence belongs to the host, so there
is no interval on the stage. An approval, a scheme response or a server
coming up is a store write or an observation whose condition is "is it
there yet". Do not sleep and poll inside a stage; put the condition in
`Until` and let the tick do the polling, so the wait survives a restart.

## Resume

A failed instance is retried by calling the runner again with the same
key: the failed step is cleared, the run restarts, and the stages run
again from the first one not completed. A waiting instance is re-entered
the same way, but it continues the same attempt: nothing is cleared and
`StartedAt` is kept.

| Call for an existing instance | Steps cleared | Run | Stages run |
|---|---|---|---|
| after a failure or a crash (`failed`, `running`, `pending`) | failed, running, pending; the undone ones went when they were undone | `RestartRun`: new attempt, `StartedAt` now, the stages' revision recorded | the first not completed, onwards |
| while waiting | none | `ResumeRun` once a condition holds, same attempt | the waiting one, onwards |
| after completion | | | none; the call is a no-op |

This is what resume demands of a stage:

- **Idempotent.** A stage may run again after a crash that lost its step. Make its effect safe to repeat: create only when absent, push only when not already pushed.
- **State on the system under control, not in the run.** The run records that a stage completed, nothing more. What the stage made (a server, a tag, a figure) lives where it belongs and is found again from there.
- **Resumable from any stage.** A later stage cannot assume an earlier one ran in this process. Anything it needs, it reads back.
- **Undone when it must not outlive a failure.** A stage whose work bills or blocks while the run sits failed has an `Undo` that removes it; the next attempt then makes it again. Its `Undo` is as idempotent as its `Run`: deleting what is already gone is not an error.
- **Its own record, if it has one.** A workflow with domain figures to keep (a drill's readings, a performance run's rates) keeps them in its own table against the run's id, found through `FindRunByKey`.

## Rewind

An operator can send an instance back to an earlier stage with
`Runner.Rewind(ctx, type, key, from, stages)`: when a check finds a problem
that an earlier step must fix. The stage named `from` and every later one
lose their steps, and the run starts a new attempt; the host's next call to
`RunPipeline` runs them again, and the stages before `from` are not
repeated. Rewind does not run anything itself.

| Instance | `from` | Rewind | Next `RunPipeline` |
|---|---|---|---|
| waiting, failed or completed | a stage, no `Final` stage completed at or after it | steps from `from` on removed; run running, new attempt | runs from `from` |
| any | a stage at or before a completed `Final` stage | refused | unchanged |
| any | not a stage of the pipeline | refused | unchanged |
| none for the key | — | refused | — |

Mark a stage `Final` when its work must never be redone, such as an
archive written or money moved. Rewinding after it is allowed. The steps a
rewind removes are not kept as history yet (#5).

## What the store records and what the host supplies

The store records runs and steps: `workflow_runs` and `workflow_steps`,
versioned through `sqlstore.Migrations`. Use `sqlstore.New` on a pglike
database for real, `NewMemStore` in tests. A step of a pipeline carries
its name, status, error and duration; the daily step carries the account's
balances and interest.

The host supplies:

| | What | Why |
|---|---|---|
| a store | `sqlstore.New(db)` or `NewMemStore()` | the engine keeps no state of its own |
| the tick | a loop that calls the runner until the run is not waiting | the engine never holds a goroutine |
| a log per step | an `io.Writer` the stages write to | the step record keeps only name, status, error and duration |
| the pages | instance and definition pages; the demo app and gobank-deploy are two | the engine renders nothing |
| the clock, injectable | `Now` and `Sleep` as fields with real defaults | tests drive time |

## Testing a workflow

Drive one key against `NewMemStore()` with fakes for the system under
control and an injected clock, then read the store back. `runnertest`
has the two checks every workflow test makes: `OnlyRun` is the single
instance, `Steps` is each step's status by name. A failure test arms one
fake to fail, checks the run is failed with the failed step, then calls
again and checks the same instance completes with the earlier stages not
repeated. A waiting test flips the condition between calls.

A new store implementation runs `storetest`'s contract checks, as
`sqlstore` and `MemStore` do.

## Drawing a workflow

`cmd/defd2` writes one d2 file per definition from the registry, and
`task docs:d2` renders them: boxes are the steps, each linked to its
source. `diagram.Instance` draws one instance with its steps coloured by
state; the demo app serves it at `workflows/instances/<id>/diagram.d2`.
Nothing is drawn by hand, so the pictures cannot drift from the code.

## Worked example

A promotion: build, wait for an approval, promote. From `example_test.go`.

```go
func promotion(approved *bool, log func(string)) []wf.Stage {
	return []wf.Stage{
		{Name: "build", Run: func(context.Context) error { log("building"); return nil }},
		{Name: "approve", Until: func(context.Context) (bool, error) { return *approved, nil }},
		{Name: "promote", Run: func(context.Context) error { log("promoting"); return nil }},
	}
}

func ExampleRunner_RunPipeline() {
	ctx := context.Background()
	store := wf.NewMemStore()
	approved := false
	stages := promotion(&approved, func(s string) { fmt.Println(s) })
	runner := wf.NewRunner(store, nil)
	date := wf.NewBusinessDate(2026, time.October, 8)
	for tick := 1; ; tick++ {
		rec, err := runner.RunPipeline(ctx, "promotion", "v0.3.0", date, stages)
		if err != nil {
			fmt.Println("failed:", err)
			return
		}
		fmt.Printf("tick %d: %s\n", tick, rec.Status)
		if rec.Status != wf.StatusWaiting {
			break
		}
		approved = tick == 2 // the approval arrives between the second and third ticks
	}
	// Output:
	// building
	// tick 1: waiting
	// tick 2: waiting
	// promoting
	// tick 3: completed
}
```

The test beside it, `TestPromotionRecordsEveryStep`, is the testing
pattern above: one key, the store read back through `runnertest`.

## Checklist

- [ ] A `Definition` with type, description, instance key and every step named, described and linked to its code, appended to `Definitions`
- [ ] The instance key comes from the system under control
- [ ] Each stage is idempotent and reads back what it needs
- [ ] Waits are `Until` conditions, not sleeps
- [ ] A stage whose work bills or blocks has an `Undo`
- [ ] A stage whose work must never be redone is `Final`
- [ ] `Now` and `Sleep` are injectable with real defaults
- [ ] A test for the happy path, one for a failure resumed, one per waiting stage, one for a failure undone where a stage has an `Undo`
- [ ] Domain figures in the workflow's own table against the run id
- [ ] The host ticks waiting runs and shows the pages
