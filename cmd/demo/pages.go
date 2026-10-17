package main

import (
	"context"
	"fmt"
	"html"
	"runtime"
	"strings"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

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
