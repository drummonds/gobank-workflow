//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"strings"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

func (s *WasmState) BuildDashboardHTML() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var b strings.Builder

	// Header.
	b.WriteString(`<div class="box">`)
	b.WriteString(`<h2 class="title is-4">Workflow Runner Demo</h2>`)
	fmt.Fprintf(&b, `<p><strong>Accounts:</strong> %d</p>`, numAccounts)
	fmt.Fprintf(&b, `<p><strong>Start date:</strong> %s</p>`, s.start)
	fmt.Fprintf(&b, `<p><strong>Current date:</strong> %s</p>`, s.current)
	fmt.Fprintf(&b, `<p><strong>Days processed:</strong> %d</p>`, len(s.timings))

	if s.err != "" {
		fmt.Fprintf(&b, `<p class="has-text-danger"><strong>Error:</strong> %s</p>`, s.err)
	}
	b.WriteString(`</div>`)

	// Workflow definitions with the instances recorded in the store.
	counts, _ := s.store.CountRuns(context.Background())
	b.WriteString(`<div class="box">`)
	fmt.Fprintf(&b, `<h3 class="title is-5">Workflow definitions <span class="tag is-light">schema v%d</span></h3>`, wf.SchemaVersion)
	b.WriteString(`<table class="table is-striped is-fullwidth is-narrow">`)
	b.WriteString(`<thead><tr><th>Type</th><th>Description</th><th>Pending</th><th>Running</th><th>Completed</th><th>Failed</th><th>Instances</th></tr></thead><tbody>`)
	for _, d := range wf.Definitions {
		c := counts[d.Type]
		fmt.Fprintf(&b, `<tr><td><code>%s</code></td><td>%s</td><td>%d</td><td>%d</td><td class="has-text-success">%d</td><td class="has-text-danger">%d</td><td><strong>%d</strong></td></tr>`,
			d.Type, d.Description, c[wf.StatusPending], c[wf.StatusRunning], c[wf.StatusCompleted], c[wf.StatusFailed], c.Total())
	}
	b.WriteString(`</tbody></table></div>`)

	// Timing table.
	if len(s.timings) > 0 {
		b.WriteString(`<div class="box">`)
		b.WriteString(`<h3 class="title is-5">Daily Processing Times</h3>`)
		b.WriteString(`<table class="table is-striped is-fullwidth is-narrow">`)
		b.WriteString(`<thead><tr><th>Date</th><th>Duration</th><th>Accounts</th></tr></thead><tbody>`)

		// Show last 20 rows.
		start := 0
		if len(s.timings) > 20 {
			start = len(s.timings) - 20
		}
		for _, t := range s.timings[start:] {
			fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%d</td></tr>`,
				t.Date, t.Duration.Round(1), t.Accounts)
		}
		b.WriteString(`</tbody></table>`)
		b.WriteString(`</div>`)

		// Timing data for chart (JSON array).
		b.WriteString(`<div class="box">`)
		b.WriteString(`<h3 class="title is-5">Processing Time Chart</h3>`)
		b.WriteString(`<canvas id="timing-chart" width="600" height="200"></canvas>`)
		b.WriteString(`<script>`)
		b.WriteString(`window._timingData = [`)
		for i, t := range s.timings {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"date":"%s","us":%d}`, t.Date, t.Duration.Microseconds())
		}
		b.WriteString(`];`)
		b.WriteString(`if(window.drawChart) window.drawChart();`)
		b.WriteString(`</script>`)
		b.WriteString(`</div>`)
	}

	// Run history from store.
	ctx := context.Background()
	runs, _ := s.store.ListRuns(ctx, 10)
	if len(runs) > 0 {
		b.WriteString(`<div class="box">`)
		b.WriteString(`<h3 class="title is-5">Recent Runs</h3>`)
		b.WriteString(`<table class="table is-striped is-fullwidth is-narrow">`)
		b.WriteString(`<thead><tr><th>Date</th><th>Status</th></tr></thead><tbody>`)
		for _, r := range runs {
			statusClass := ""
			switch r.Status {
			case wf.StatusCompleted:
				statusClass = "has-text-success"
			case wf.StatusFailed:
				statusClass = "has-text-danger"
			}
			fmt.Fprintf(&b, `<tr><td>%s</td><td class="%s">%s</td></tr>`,
				r.BusinessDate, statusClass, r.Status)
		}
		b.WriteString(`</tbody></table>`)
		b.WriteString(`</div>`)
	}

	return b.String()
}
