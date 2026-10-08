# Comparison with other workflow engines

gobank-workflow is small by design: a run record per instance, a step
record per unit of work, two runners, a definitions registry and a
versioned schema. This page sets it against the Go workflow libraries and
the larger orchestration systems as they stood on 2026-10-04, to show what
is deliberately absent and what might be worth adding. Versions and
licences change; the figures are a snapshot and each system's own docs are
the source of truth.

## Why this stays a component

In the gobank model bank every table belongs to one component and other
code reads it through that component's contract view
([ADR-0001](https://git.bytestone.uk/hum3/gobank/src/branch/main/adr/0001-contract-views.md)).
The workflow runs and steps are bank data: the DB explorer shows them
beside accounts and postings, the step results carry opening and closing
balances in the ledger's own units, and the schema moves with the rest of
the bank through gobank-db expand/contract migrations. Every engine below
keeps instance state somewhere else, in its own history store, in a
Kafka log, in a server's RocksDB, or in Postgres tables shaped by the
engine, and none compiles to WASM, so none could run inside the browser
demo on pglike. Adopting one would move the workflow component's data
outside the bank's database or outside the contract-view rule. That is
the reason to stay separate, and it is also the reason the comparison is
worth keeping: the engines below are where the missing features have
already been designed once.

## The landscape

The systems fall into three groups by what you run and how a workflow is
written.

| System | Group | Workflow is written as | You run | Instance state lives in | Licence | Go |
|---|---|---|---|---|---|---|
| gobank-workflow | library | Go: a `Definition` with named steps; `RunDaily`, `RunPipeline` | nothing extra | your database (pglike, Postgres, SQLite) | MIT | native |
| [go-workflows](https://github.com/cschleiden/go-workflows) | library | Go: deterministic function replayed from history | nothing extra | SQLite, MySQL, Postgres, Redis | MIT | native |
| [DBOS Transact Go](https://github.com/dbos-inc/dbos-transact-golang) | library | Go: function with checkpointed steps | nothing extra (paid console optional) | Postgres, SQLite | MIT | native |
| [luno/workflow](https://github.com/luno/workflow) | library | Go: typed status machine, step per transition | nothing extra | SQL record store + Kafka or in-memory stream | BSD-3 | native |
| [River](https://riverqueue.com) | library | Go: job kinds; DAG workflows are River Pro (closed source, paid) | nothing extra | Postgres, SQLite | MPL-2.0 core | native |
| [asynq](https://github.com/hibiken/asynq) | library | Go: tasks; no workflow layer | Redis | Redis | MIT | native |
| [Temporal](https://temporal.io) | server, code | Go: deterministic function replayed from history | server + Postgres/MySQL/Cassandra (+ Elasticsearch at scale) + UI | server's history store | MIT | first-class SDK |
| [Cadence](https://cadenceworkflow.io) | server, code | as Temporal | four services + Cassandra/SQL | server's store | Apache-2.0 (CNCF) | first-class SDK |
| [Restate](https://restate.dev) | server, code | Go: journaled handlers; services, keyed objects, workflows | one server binary | server's RocksDB | BSL-1.1 server, MIT SDK | first-class SDK |
| [Inngest](https://www.inngest.com) | server, code | Go: event-triggered function with memoised steps | server or cloud | server (SQLite/Postgres + Redis) | SSPL server, Apache-2.0 SDK | first-class SDK |
| [Hatchet](https://hatchet.run) | server, code | Go: tasks and DAGs; durable tasks checkpoint waits | engine + Postgres | engine's Postgres | MIT | first-class SDK |
| [Dapr Workflow](https://docs.dapr.io/developing-applications/building-blocks/workflow/) | server, code | Go: replayed orchestration (durabletask) | Dapr sidecar + state store | Dapr state store | Apache-2.0 | first-class SDK |
| [LittleHorse](https://littlehorse.io) | server, graph | Go DSL compiled to a node graph on the server | JVM server + Kafka | Kafka + RocksDB | AGPL server, Apache-2.0 SDK | first-class SDK |
| [Conductor](https://github.com/conductor-oss/conductor) | server, declarative | JSON definition; Go builder available | JVM + Redis/Postgres + Elasticsearch/OpenSearch | server's store | Apache-2.0 | SDK (Orkes) |
| [Argo Workflows](https://argoproj.github.io/workflows/) | server, declarative | YAML CRD; one pod per step | Kubernetes | Kubernetes API (+ archive DB) | Apache-2.0 | typed client |
| [Camunda 8 / Zeebe](https://camunda.com) | server, BPMN | BPMN 2.0; job workers in code | 3+ brokers + Elasticsearch/OpenSearch + Identity | brokers' Raft log | source-available, paid in production | REST or community gRPC client |
| [Flowable](https://www.flowable.com/open-source) | library (Java), BPMN | BPMN 2.0, CMMN, DMN | JVM + RDBMS | your RDBMS, engine's tables | Apache-2.0 engine | REST only |
| [Spring Batch](https://spring.io/projects/spring-batch) | library (Java) | Java: job of chunked or tasklet steps | JVM | your RDBMS, `BATCH_*` tables | Apache-2.0 | none |
| [Kestra](https://kestra.io) | server, declarative | YAML flow; Go only as a script task | JVM + Postgres | server's DB | Apache-2.0 core | script runner |
| [Windmill](https://www.windmill.dev) | server, declarative | visual or JSON flow of scripts; Go as a script language | Rust server + Postgres | server's Postgres | AGPL + proprietary | script runner |

Two execution models dominate. **Replay** engines (go-workflows, DBOS,
Temporal, Cadence, Restate, Inngest, Dapr) record the result of each
side-effecting step and re-run the workflow function from the top on
recovery, which lets ordinary code sleep for a month or wait for an
event, at the price of a determinism rule on the workflow body.
**Recorded-step** engines (gobank-workflow, luno/workflow, Spring Batch,
the BPMN engines, the declarative orchestrators) keep an explicit record
of where an instance is and resume from it; the code has no determinism
rule but every wait has to be modelled as a stored state. gobank-workflow
is in the second camp, and the closest relatives are the three in the
next section.

## Feature matrix: Go libraries

Y means built in, P means partial or a documented pattern, N means absent,
U means not confirmed from the project's own documentation.

| Feature | gobank-workflow | go-workflows | DBOS Go | luno/workflow | River (core) |
|---|---|---|---|---|---|
| Instance keyed by business key, duplicate refused | Y, (type, date, key) unique | Y, caller-chosen instance id | Y, workflow id | Y, foreign id | Y, unique jobs |
| Completed instance is never re-run | Y | Y | Y | Y | P, unique by state |
| Retry of a failed instance keeps its identity | Y, restart in place | Y | Y | Y | Y, same job |
| Resume from the failed step, not the start | Y, pipeline only; daily reprocesses every account | Y, replay | Y, step checkpoints | Y, status machine | N |
| Per-step retry with backoff | N, caller re-runs | Y | Y | Y | Y |
| Durable timer or sleep | N | Y | Y | Y, timeout steps | P, scheduled jobs |
| External event into a running instance | N | Y, signals | Y, send/recv, events | Y, callbacks, connectors | Pro only |
| Parallel steps, fan-out | N | Y | Y | Y, parallel consumers | Y, queues; DAG is Pro |
| Child workflows | N | Y | Y | N | Pro only |
| Compensation (saga) | N | N | N | N | N |
| Change definition while instances in flight | N, steps matched by name | N | Y, version, patch, fork | P | n/a |
| Schedule, cron | N, caller drives | N | Y | Y | Y, periodic jobs |
| Catch-up over missed periods | Y, `CatchUp` | N | N | N | N |
| Attempt history kept | N, failed steps deleted on retry | Y, full history | Y | Y, event log | Y, errors list |
| Definition visible as data | Y, registry, d2 diagrams | P, diagnostics UI | N | Y, Mermaid | N |
| UI | Y, demo app pages | Y, diagnostics | paid console | Y | Y, River UI |
| Metrics, tracing | N | Y, OTel | U | Y, Prometheus | Y, OTel |
| Test support | Y, memory store, `storetest` contract | Y, mocks, timer skipping | P | Y, testing package | Y, rivertest |
| Schema versioned with the host database | Y, gobank-db migrations | N, own schema | N, own schema | N, own schema | N, own migrations |
| Runs in the browser (WASM, pglike) | Y | N | N | N | N |

## Feature matrix: larger systems

| Feature | gobank-workflow | Temporal | Restate | Camunda 8 | Flowable | Conductor | LittleHorse | Spring Batch |
|---|---|---|---|---|---|---|---|---|
| Instance keyed by business key, duplicate refused | Y | Y, id reuse and conflict policies | Y, workflow key | P, message correlation | P, business key not unique | Y in Orkes; U in OSS | Y, unique run id | Y, identifying job parameters |
| Completed instance is never re-run | Y | Y, `RejectDuplicate` or `AllowDuplicateFailedOnly` | Y | P | P | P | Y | Y, `JobInstanceAlreadyComplete` |
| Resume from the failed step | Y, pipeline | Y | Y | Y | Y | Y | Y | Y, last committed chunk |
| Per-step retry with backoff | N | Y | Y | Y | P, fixed count | Y | Y | Y, item level |
| Durable timer | N | Y | Y | Y, timer events | Y | Y, wait task | Y | N |
| External event into a running instance | N | Y, signal, update | Y, awakeables, promises | Y, messages | Y | Y | Y, external events, interrupts | N |
| Parallel steps | N | Y | Y | Y, gateways, multi-instance | Y | Y, fork/join | Y, threads | Y, split, partitioning |
| Child workflows | N | Y | Y | Y, call activities | Y | Y | Y | P, job step |
| Compensation (saga) | N | P, coded | Y, documented pattern | Y, BPMN compensation | Y | P, failure workflow | Y, failure handlers | N |
| Change definition while instances in flight | N | Y, patching, worker versioning | Y, deployment pinning | Y, instance migration | Y, migration service | Y, versioned definitions | Y, versions, migration plans | N |
| Schedule, cron | N | Y, Schedules with overlap and catch-up policy | N | P, timer start | P, timer start | Y | U | N, external |
| Human task | N | P, wait for signal | P | Y, Tasklist | Y, OSS has no UI | P | Y | N |
| Attempt history kept | N | Y | Y | Y | Y | Y | Y | Y, one `JobExecution` per attempt |
| UI | Y, demo app | Y | Y | Y | commercial only since v7 | Y | Y | N |
| Metrics, tracing | N | Y | Y | Y | P | P | Y | Y, Micrometer |
| Test support | Y | Y, time skipping, replayer | Y, Testcontainers | Y, Java only | Y, Java only | P | U | Y, Java only |
| Smallest production footprint | one process, one database | server + DB + UI | one binary | 3 brokers + search cluster | JVM + RDBMS | JVM + Redis + search | JVM + Kafka | JVM + RDBMS |

Where finance use is documented: Temporal names ANZ, JPMorgan Chase,
Macquarie, Coinbase, Klarna and Checkout.com; Flowable publishes tier-one
bank case-management studies; Camunda markets to banking; Spring Batch's
reference manual uses an end-of-day job keyed by business date as its
worked example.

## The three closest relatives

**Spring Batch** is the direct ancestor of `RunDaily`. Its `JobInstance`
is a job plus its identifying parameters, and the manual's example is an
`EndOfDayJob` with `schedule.Date` as that parameter: one instance per
business date, exactly our (type, date) key. Each attempt is a separate
`JobExecution`, a completed instance cannot be restarted, and a failed one
restarts from the last committed chunk because readers persist their
position in an `ExecutionContext`. The two things it has that we do not
are the attempt record and the chunk-level resume; the first matters for
audit, the second is the mechanism the gobank roadmap's start-of-day
workflow needs when it says the pass over every account "resumes after a
restart from the rows already written".

**Temporal** solves the idempotency question we solved with a unique
index by policy on the workflow id. `AllowDuplicateFailedOnly` is
precisely `RunDaily`'s contract: a completed id is refused, a failed one
may be run again. Its conflict policy for an id that is still running
(`Fail`, `UseExisting`, `TerminateExisting`) names a case we have not
handled, and its Schedules object (overlap policy, catch-up window,
backfill) is the mature form of `CatchUp`. Everything else it offers
comes with the replay model and a server, which is the part we are not
buying.

**luno/workflow** is the nearest Go library in spirit: no replay, a typed
status machine over a SQL record store, idempotent on a foreign id, with
timeouts, callbacks, cron and a Mermaid diagram of the definition. It
reached v1.0 on 2026-09-30. It shows what a recorded-step Go library
grows into if it keeps going, and its event-stream dependency is the
point at which it stops fitting a one-database component.

## Gaps and candidates

Each row is a feature the engines above have and this one does not. The
decision is whether the bank's own workflows need it; the candidate is
the smallest shape that fits the current model.

| Gap | Who has it well | Needed by | Candidate shape | Priority |
|---|---|---|---|---|
| A running instance can be restarted by a second runner: `RunDaily` restarts any non-completed run, including `running` | Temporal conflict policy, Spring Batch `JobExecutionAlreadyRunning` | stage 8 of the gobank roadmap, several processes and one workflow runner | `ErrAlreadyRunning` from `RestartRun` unless the run is stale; a `heartbeat_at` column to judge stale | now |
| Attempt history is lost: a retry deletes the failed steps | Spring Batch `JobExecution`, every replay engine | audit of a failed end-of-day | an `attempt` number on `workflow_steps`, kept across retries, with the instance page showing the last attempt by default | now |
| Daily run reprocesses every account; pipeline resumes but daily does not | Spring Batch chunk restart, DBOS steps | roadmap stage 3d, resume mid-pass | record each account's step as it completes and skip completed ones on retry; the pipeline already does this by step name | with stage 3 |
| No schedule or overlap policy; the caller decides when to run | Temporal Schedules, Argo CronWorkflow | production runner | a `schedules` table and a tick: next due date, skip if the previous run is still running; `CatchUp` is already the backfill | with stage 8 |
| No per-step retry with backoff | all | network stages of the release pipeline | `Stage.Retry` with attempts and backoff, recorded as attempts of the step | when it hurts |
| No durable wait for an external event | Temporal signals, Restate awakeables, BPMN messages | payment settlement, awaiting a scheme response | a `waiting` run status and a `Store.Signal` that records the event and wakes the run | with payments |
| No parallel steps | all | pacing the account pass, not throughput, is the current need | leave; the roadmap paces the pass over the day | not planned |
| No compensation | Camunda, LittleHorse, Restate | a release that fails after `push origin` | an optional `Stage.Undo` run in reverse for completed stages; worth it only if a half-shipped release is observed | not planned |
| No definition versioning | Temporal, LittleHorse, BPMN engines | instances that outlive their code: the instance page, and purging old runs | a revision (hash of the step names) recorded on the run, so an old instance renders from what it recorded; no migration (roadmap "Purpose and revision") | now |
| No human task | Camunda, Flowable, LittleHorse | four-eyes approval on a payment or release | a step whose completion is a signal from the staff UI, once signals exist | after signals |
| No metrics | all servers | operations | counters per definition and status, exported where the bank exports the rest | with stage 8 |

What this engine has that the others do not is the other half of the
decision: its state is bank data under the contract-view rule, its schema
is versioned with the bank's, its step results are domain figures rather
than opaque payloads, its definitions are drawn from the code they
describe, and the whole thing runs in a browser on pglike. Those are the
properties any addition from the table above has to keep.

## Sources

Each system's figures were read from its own repository, releases and
documentation on 2026-10-04. The load-bearing pages:

- Temporal: [Workflow Id reuse and conflict policies](https://docs.temporal.io/workflow-execution/workflowid-runid), [Schedules](https://docs.temporal.io/schedule), [Worker Versioning](https://docs.temporal.io/production-deployment/worker-deployments/worker-versioning), [self-hosted deployment](https://docs.temporal.io/self-hosted-guide/deployment)
- Spring Batch: [domain language](https://docs.spring.io/spring-batch/reference/domain.html), [what's new in 6.0](https://docs.spring.io/spring-batch/reference/whatsnew.html)
- go-workflows: [README](https://github.com/cschleiden/go-workflows), [docs](https://cschleiden.github.io/go-workflows/)
- DBOS Go: [workflow tutorial](https://docs.dbos.dev/golang/tutorials/workflow-tutorial), [workflow management](https://docs.dbos.dev/golang/tutorials/workflow-management)
- luno/workflow: [README](https://github.com/luno/workflow)
- River: [Pro workflows](https://riverqueue.com/docs/pro/workflows), [pricing](https://riverqueue.com/pricing)
- Restate: [versioning](https://docs.restate.dev/operate/versioning), [sagas](https://docs.restate.dev/guides/sagas), [server licence](https://github.com/restatedev/restate/blob/main/LICENSE)
- Camunda 8: [licences](https://docs.camunda.io/docs/reference/licenses/), [Go client deprecation](https://camunda.com/blog/2024/09/deprecating-zbctl-and-go-clients/), [process instance migration](https://docs.camunda.io/docs/components/concepts/process-instance-migration/)
- Flowable: [open source](https://www.flowable.com/open-source), [process instance migration](https://www.flowable.com/open-source/docs/bpmn/ch08-ProcessInstanceMigration)
- Conductor: [conductor-oss](https://github.com/conductor-oss/conductor), [task definitions](https://conductor-oss.github.io/conductor/documentation/configuration/taskdef.html)
- LittleHorse: [WfSpec versioning](https://littlehorse.io/docs/server/concepts/advanced/wfspec-versioning), [external events](https://littlehorse.io/docs/server/concepts/external-events), [user tasks](https://littlehorse.io/docs/server/concepts/user-tasks)
- Argo Workflows: [CronWorkflow](https://argo-workflows.readthedocs.io/en/latest/cron-workflows/), [retries](https://argo-workflows.readthedocs.io/en/latest/retries/)
- Kestra: [architecture](https://kestra.io/docs/architecture), [Go script plugin](https://kestra.io/plugins/plugin-script-go)
- Windmill: [Go scripts](https://www.windmill.dev/docs/getting_started/scripts_quickstart/go), [workflows as code](https://www.windmill.dev/docs/core_concepts/workflows_as_code)
- Dapr / durabletask-go: [workflow features](https://docs.dapr.io/developing-applications/building-blocks/workflow/workflow-features-concepts/), [durabletask-go](https://github.com/microsoft/durabletask-go)
- Inngest: [steps and workflows](https://www.inngest.com/docs/features/inngest-functions/steps-workflows); Hatchet: [durable execution](https://docs.hatchet.run/home/durable-execution)
