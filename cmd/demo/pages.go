package main

import (
	"context"
	"fmt"
	"html"
	"io/fs"
	"runtime"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

func statusTag(s wf.RunStatus) string {
	class := "is-light"
	switch s {
	case wf.StatusRunning:
		class = "is-info"
	case wf.StatusWaiting:
		class = "is-warning"
	case wf.StatusCompleted:
		class = "is-success"
	case wf.StatusFailed:
		class = "is-danger"
	}
	return fmt.Sprintf(`<span class="tag %s">%s</span>`, class, s)
}

// money renders minor units with the given exponent, e.g. 1000000,-2 → 10,000.00.
func money(v int64, exp int) string {
	if exp >= 0 {
		return fmt.Sprintf("%d", v)
	}
	scale := int64(1)
	for range -exp {
		scale *= 10
	}
	whole, frac := v/scale, v%scale
	if frac < 0 {
		frac = -frac
	}
	s := fmt.Sprintf("%d", whole)
	for i := len(s) - 3; i > 0; i -= 3 {
		if s[i-1] != '-' {
			s = s[:i] + "," + s[i:]
		}
	}
	return fmt.Sprintf("%s.%0*d", s, -exp, frac)
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("15:04:05.000")
}

func (a *App) aboutHTML() string {
	a.mu.Lock()
	applied, _ := a.store.SchemaVersion(context.Background())
	a.mu.Unlock()
	var b strings.Builder
	b.WriteString(`<h1 class="title">gobank-workflow</h1>`)
	b.WriteString(`<p class="subtitle">Demo of the banking workflow engine: idempotent daily processing, persistent run tracking, one instance per business date.</p>`)
	b.WriteString(`<div class="content"><p>Use <strong>Simulation</strong> to make business days happen, then browse the <strong>Workflows</strong> menu: instances are the runs recorded in the store, definitions are the kinds of workflow the engine knows. <strong>DB Explorer</strong> shows the tables behind them.</p></div>`)
	b.WriteString(`<table class="table is-narrow"><tbody>`)
	fmt.Fprintf(&b, `<tr><th>App version</th><td>%s</td></tr>`, html.EscapeString(version))
	fmt.Fprintf(&b, `<tr><th>Schema version</th><td>code expects v%d, database is at v%d</td></tr>`, wf.SchemaVersion, applied)
	fmt.Fprintf(&b, `<tr><th>Runtime</th><td>%s/%s, %s</td></tr>`, runtime.GOOS, runtime.GOARCH, runtime.Version())
	fmt.Fprintf(&b, `<tr><th>Store</th><td>pglike (go-postgres) in memory; reset by the simulation</td></tr>`)
	b.WriteString(`</tbody></table>`)
	b.WriteString(`<div class="content"><ul><li><a href="https://gobank-workflow.docs.bytestone.uk/">Documentation</a></li><li><a href="https://git.bytestone.uk/hum3/gobank-workflow">Source</a></li></ul></div>`)
	return b.String()
}

func (a *App) instancesHTML(ctx context.Context) string {
	a.mu.Lock()
	runs, err := a.store.ListRuns(ctx, 500)
	a.mu.Unlock()
	var b strings.Builder
	b.WriteString(`<h1 class="title">Workflow instances</h1><p class="subtitle">Most recent first</p>`)
	if err != nil {
		fmt.Fprintf(&b, `<div class="notification is-danger">%s</div>`, html.EscapeString(err.Error()))
		return b.String()
	}
	if len(runs) == 0 {
		b.WriteString(`<div class="notification">No instances yet. <a href="simulation">Run the simulation</a> to process some business days.</div>`)
		return b.String()
	}
	b.WriteString(`<div class="table-container"><table class="table is-striped is-hoverable is-fullwidth is-narrow">`)
	b.WriteString(`<thead><tr><th>Instance</th><th>Definition</th><th>Business date</th><th>Status</th><th>Started</th><th>Completed</th><th>Duration</th><th>Error</th><th></th></tr></thead><tbody>`)
	for _, r := range runs {
		dur := "—"
		if !r.CompletedAt.IsZero() {
			dur = r.CompletedAt.Sub(r.StartedAt).Round(time.Microsecond).String()
		}
		fmt.Fprintf(&b, `<tr><td><strong>%s</strong></td><td><a href="workflows/definitions/%s"><code>%s</code></a></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td class="has-text-danger">%s</td><td><a class="button is-small is-link is-light" href="workflows/instances/%s">View</a></td></tr>`,
			instanceName(r), r.WorkflowType, r.WorkflowType, r.BusinessDate, statusTag(r.Status), stamp(r.StartedAt), stamp(r.CompletedAt), dur, html.EscapeString(r.Error), r.ID)
	}
	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

const stepsShown = 50

// instanceName is what identifies an instance to a reader: its key when
// the definition has one, otherwise its business date.
func instanceName(r wf.RunRecord) string {
	if r.Key != "" {
		return r.Key
	}
	return r.BusinessDate.String()
}

func definitionOf(t wf.WorkflowType) *wf.Definition {
	for i := range wf.Definitions {
		if wf.Definitions[i].Type == t {
			return &wf.Definitions[i]
		}
	}
	return nil
}

func (a *App) instanceHTML(ctx context.Context, id string) (string, bool) {
	a.mu.Lock()
	run, err := a.store.GetRun(ctx, id)
	var steps []wf.StepResult
	if err == nil && run != nil {
		steps, _ = a.store.ListSteps(ctx, id)
	}
	a.mu.Unlock()
	if err != nil || run == nil {
		return "", false
	}
	var b strings.Builder
	name := instanceName(*run)
	fmt.Fprintf(&b, `<nav class="breadcrumb"><ul><li><a href="workflows/instances">Instances</a></li><li class="is-active"><a>%s %s</a></li></ul></nav>`, run.WorkflowType, name)
	fmt.Fprintf(&b, `<h1 class="title">%s %s %s</h1>`, run.WorkflowType, name, statusTag(run.Status))
	b.WriteString(`<table class="table is-narrow"><tbody>`)
	fmt.Fprintf(&b, `<tr><th>Instance id</th><td><code>%s</code></td></tr>`, run.ID)
	fmt.Fprintf(&b, `<tr><th>Definition</th><td><a href="workflows/definitions/%s"><code>%s</code></a></td></tr>`, run.WorkflowType, run.WorkflowType)
	if run.Key != "" {
		fmt.Fprintf(&b, `<tr><th>Key</th><td>%s</td></tr>`, html.EscapeString(run.Key))
	}
	fmt.Fprintf(&b, `<tr><th>Business date</th><td>%s</td></tr>`, run.BusinessDate)
	fmt.Fprintf(&b, `<tr><th>Status</th><td>%s</td></tr>`, statusTag(run.Status))
	fmt.Fprintf(&b, `<tr><th>Started</th><td>%s</td></tr>`, stamp(run.StartedAt))
	fmt.Fprintf(&b, `<tr><th>Completed</th><td>%s</td></tr>`, stamp(run.CompletedAt))
	if run.Error != "" {
		fmt.Fprintf(&b, `<tr><th>Error</th><td class="has-text-danger">%s</td></tr>`, html.EscapeString(run.Error))
	}
	stageSteps := len(steps) > 0 && steps[0].AccountID == ""
	if stageSteps {
		completed := 0
		for _, s := range steps {
			if s.Status == wf.StatusCompleted {
				completed++
			}
		}
		fmt.Fprintf(&b, `<tr><th>Steps</th><td>%d of %d stages completed</td></tr>`, completed, len(releaseDefinition().Steps))
	} else {
		var interest int64
		for _, s := range steps {
			interest += s.InterestAmount
		}
		fmt.Fprintf(&b, `<tr><th>Steps</th><td>%d accounts processed, %s interest accrued</td></tr>`, len(steps), money(interest, -2))
	}
	b.WriteString(`</tbody></table>`)

	if stageSteps {
		b.WriteString(`<h2 class="subtitle">Stages</h2>`)
		b.WriteString(`<table class="table is-striped is-fullwidth is-narrow"><thead><tr><th>#</th><th>Stage</th><th>Status</th><th>Duration</th><th>Error</th></tr></thead><tbody>`)
		byName := map[string]wf.StepResult{}
		for _, s := range steps {
			byName[s.StepName] = s
		}
		for i, def := range releaseDefinition().Steps {
			st, ran := byName[def.Name]
			status, dur, errMsg := `<span class="tag is-light">not run</span>`, "—", ""
			if ran {
				status, dur, errMsg = statusTag(st.Status), time.Duration(st.DurationNs).Round(time.Microsecond).String(), st.Error
			}
			fmt.Fprintf(&b, `<tr><td>%d</td><td><strong>%s</strong> <span class="has-text-grey">%s</span> %s</td><td>%s</td><td>%s</td><td class="has-text-danger">%s</td></tr>`,
				i+1, def.Name, html.EscapeString(def.Description), sourceLink(def), status, dur, html.EscapeString(errMsg))
		}
		b.WriteString(`</tbody></table>`)
	} else if len(steps) > 0 {
		fmt.Fprintf(&b, `<h2 class="subtitle">Steps <span class="has-text-grey">(first %d of %d)</span></h2>`, min(stepsShown, len(steps)), len(steps))
		b.WriteString(`<div class="table-container"><table class="table is-striped is-fullwidth is-narrow">`)
		b.WriteString(`<thead><tr><th>Account</th><th>Step</th><th class="has-text-right">Opening</th><th class="has-text-right">Interest</th><th class="has-text-right">Closing</th></tr></thead><tbody>`)
		for _, s := range steps[:min(stepsShown, len(steps))] {
			fmt.Fprintf(&b, `<tr><td><code>%s</code></td><td>%s</td><td class="has-text-right">%s</td><td class="has-text-right">%s</td><td class="has-text-right">%s</td></tr>`,
				s.AccountID, s.StepName, money(s.OpeningBalance, s.Exponent), money(s.InterestAmount, s.Exponent), money(s.ClosingBalance, s.Exponent))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	a.diagramHTML(&b, run.WorkflowType)
	fmt.Fprintf(&b, `<p><a href="workflows/instances/%s/diagram.d2">This instance as d2</a> — steps coloured by state; render with <code>d2 diagram.d2 out.svg</code>. <a href="internal/explorer/workflow_steps?filter=run_id&amp;value=%s">All steps in the DB explorer</a>.</p>`, run.ID, run.ID)
	return b.String(), true
}

func (a *App) definitionsHTML(ctx context.Context) string {
	a.mu.Lock()
	counts, _ := a.store.CountRuns(ctx)
	a.mu.Unlock()
	var b strings.Builder
	b.WriteString(`<h1 class="title">Workflow definitions</h1><p class="subtitle">What the engine can run: each definition is a sequence of steps, and each instance is one run of it</p>`)
	b.WriteString(`<div class="table-container"><table class="table is-striped is-hoverable is-fullwidth">`)
	b.WriteString(`<thead><tr><th>Type</th><th>Description</th><th>Instance key</th><th>Steps</th><th>Instances</th></tr></thead><tbody>`)
	for _, d := range wf.Definitions {
		c := counts[d.Type]
		var names []string
		for _, st := range d.Steps {
			names = append(names, st.Name)
		}
		fmt.Fprintf(&b, `<tr><td><a href="workflows/definitions/%s"><code>%s</code></a></td><td>%s</td><td>%s</td><td>%s</td><td>%d <span class="has-text-grey">(%d completed, %d failed)</span></td></tr>`,
			d.Type, d.Type, html.EscapeString(d.Description), d.InstanceKey, html.EscapeString(strings.Join(names, " → ")), c.Total(), c[wf.StatusCompleted], c[wf.StatusFailed])
	}
	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

func (a *App) definitionHTML(ctx context.Context, t wf.WorkflowType) (string, bool) {
	def := definitionOf(t)
	if def == nil {
		return "", false
	}
	a.mu.Lock()
	counts, _ := a.store.CountRuns(ctx)
	runs, _ := a.store.ListRuns(ctx, 500)
	a.mu.Unlock()
	c := counts[t]
	var b strings.Builder
	fmt.Fprintf(&b, `<nav class="breadcrumb"><ul><li><a href="workflows/definitions">Definitions</a></li><li class="is-active"><a>%s</a></li></ul></nav>`, t)
	fmt.Fprintf(&b, `<h1 class="title"><code>%s</code></h1><p class="subtitle">%s</p>`, t, html.EscapeString(def.Description))
	fmt.Fprintf(&b, `<p><strong>Instance key:</strong> %s — one instance per %s.</p>`, def.InstanceKey, def.InstanceKey)
	b.WriteString(`<h2 class="subtitle mt-4">Steps</h2>`)
	b.WriteString(`<table class="table is-narrow is-striped"><thead><tr><th>#</th><th>Step</th><th>Does</th><th>Code</th></tr></thead><tbody>`)
	for i, st := range def.Steps {
		fmt.Fprintf(&b, `<tr><td>%d</td><td><strong>%s</strong></td><td>%s</td><td>%s</td></tr>`, i+1, st.Name, html.EscapeString(st.Description), sourceLink(st))
	}
	b.WriteString(`</tbody></table>`)
	a.diagramHTML(&b, t)
	b.WriteString(`<h2 class="subtitle mt-4">Instances <span class="has-text-grey">(runs of this definition)</span></h2>`)
	b.WriteString(`<div class="tags are-medium">`)
	for _, st := range []wf.RunStatus{wf.StatusPending, wf.StatusRunning, wf.StatusWaiting, wf.StatusCompleted, wf.StatusFailed} {
		fmt.Fprintf(&b, `<span class="tag">%s&nbsp;<strong>%d</strong></span>`, st, c[st])
	}
	fmt.Fprintf(&b, `<span class="tag is-dark">instances&nbsp;<strong>%d</strong></span></div>`, c.Total())
	b.WriteString(`<table class="table is-striped is-hoverable is-fullwidth is-narrow"><thead><tr><th>Instance</th><th>Business date</th><th>Status</th><th>Started</th><th>Completed</th><th></th></tr></thead><tbody>`)
	n := 0
	for _, r := range runs {
		if r.WorkflowType != t {
			continue
		}
		n++
		fmt.Fprintf(&b, `<tr><td><strong>%s</strong></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td><a class="button is-small is-link is-light" href="workflows/instances/%s">View</a></td></tr>`,
			instanceName(r), r.BusinessDate, statusTag(r.Status), stamp(r.StartedAt), stamp(r.CompletedAt), r.ID)
	}
	b.WriteString(`</tbody></table>`)
	if n == 0 {
		b.WriteString(`<p class="has-text-grey">No instances yet.</p>`)
	}
	return b.String(), true
}

func (a *App) simulationHTML(ctx context.Context) string {
	a.mu.Lock()
	counts, _ := a.store.CountRuns(ctx)
	current, failNext := a.current, a.failNext
	next := a.nextVersion()
	log := append([]string(nil), a.log...)
	opts := a.opts
	a.mu.Unlock()
	c := counts[wf.WorkflowDailyUpdate]
	var b strings.Builder
	b.WriteString(`<h1 class="title">Simulation driver</h1><p class="subtitle">Make business days happen and watch instances appear</p>`)
	b.WriteString(`<div class="columns"><div class="column is-half">`)
	b.WriteString(`<table class="table is-narrow"><tbody>`)
	fmt.Fprintf(&b, `<tr><th>Book</th><td>%d accounts at %.2f%% p.a., %s each at start</td></tr>`, opts.Accounts, opts.Rate*100, money(opts.Balance, -2))
	fmt.Fprintf(&b, `<tr><th>Next business date</th><td><strong>%s</strong></td></tr>`, current)
	fmt.Fprintf(&b, `<tr><th>Instances</th><td>%d completed, %d failed</td></tr>`, c[wf.StatusCompleted], c[wf.StatusFailed])
	if failNext {
		b.WriteString(`<tr><th>Next day</th><td><span class="tag is-danger">will fail</span></td></tr>`)
	}
	b.WriteString(`</tbody></table>`)
	b.WriteString(`<div class="buttons">`)
	for _, d := range []int{1, 7, 30} {
		fmt.Fprintf(&b, `<form method="post" action="simulation/advance"><input type="hidden" name="days" value="%d"><button class="button is-primary">+%d day%s</button></form>`, d, d, map[bool]string{true: "s"}[d > 1])
	}
	failClass := "is-danger is-outlined"
	if failNext {
		failClass = "is-danger"
	}
	fmt.Fprintf(&b, `<form method="post" action="simulation/fail"><button class="button %s">Fail next day</button></form>`, failClass)
	b.WriteString(`<form method="post" action="simulation/reset"><button class="button is-warning">Reset</button></form>`)
	b.WriteString(`</div>`)
	b.WriteString(`<p>A failed day stays current: the next advance retries the same instance. See <a href="workflows/instances">instances</a>.</p>`)

	rc := counts[wf.WorkflowRelease]
	b.WriteString(`<h2 class="subtitle mt-5">Release pipeline</h2>`)
	fmt.Fprintf(&b, `<p>Next version to ship: <strong>%s</strong>. %d released, %d failed.</p>`, next, rc[wf.StatusCompleted], rc[wf.StatusFailed])
	b.WriteString(`<form method="post" action="simulation/release" class="field has-addons">`)
	b.WriteString(`<div class="control"><div class="select"><select name="fail_stage"><option value="">every stage succeeds</option>`)
	for _, st := range releaseDefinition().Steps {
		fmt.Fprintf(&b, `<option value="%s">fail at %s</option>`, html.EscapeString(st.Name), html.EscapeString(st.Name))
	}
	fmt.Fprintf(&b, `</select></div></div><div class="control"><button class="button is-primary">Ship %s</button></div></form>`, next)
	b.WriteString(`<p>A failed release keeps its version: shipping again resumes that instance from the failed stage.</p>`)
	b.WriteString(`</div><div class="column">`)
	b.WriteString(`<h2 class="subtitle">Log</h2><pre class="log">`)
	start := max(0, len(log)-20)
	for _, line := range log[start:] {
		b.WriteString(html.EscapeString(line))
		b.WriteString("\n")
	}
	b.WriteString(`</pre></div></div>`)
	return b.String()
}

// diagramHTML embeds the definition's diagram when the build shipped one.
func (a *App) diagramHTML(b *strings.Builder, t wf.WorkflowType) {
	if _, err := fs.Stat(a.diagrams, string(t)+".svg"); err != nil {
		return
	}
	fmt.Fprintf(b, `<figure class="image my-4"><img src="workflows/definitions/%s/diagram.svg" alt="%s steps" style="max-width:100%%"></figure>`, t, t)
}

// sourceLink is the step's link to its code, or nothing.
func sourceLink(st wf.Step) string {
	if st.Source == "" {
		return ""
	}
	return fmt.Sprintf(`<a href="%s" title="%s">code</a>`, html.EscapeString(st.Source), html.EscapeString(st.Source))
}
