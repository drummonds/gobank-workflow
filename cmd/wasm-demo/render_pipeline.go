//go:build js && wasm

package main

import (
	"fmt"
	"html"
	"strings"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/cmd/wasm-demo/pipeline"
)

func statusTag(s wf.RunStatus) string {
	class := "is-light"
	switch s {
	case wf.StatusRunning:
		class = "is-info"
	case wf.StatusCompleted:
		class = "is-success"
	case wf.StatusFailed:
		class = "is-danger"
	}
	return fmt.Sprintf(`<span class="tag %s">%s</span>`, class, s)
}

func button(action, label, class string, disabled bool) string {
	dis := ""
	if disabled {
		dis = " disabled"
	}
	return fmt.Sprintf(`<button class="button %s" onclick="act('%s')"%s>%s</button>`, class, action, dis, label)
}

// BuildPipelineHTML renders the release pipeline demo.
func BuildPipelineHTML(p *pipeline.Pipeline) string {
	var b strings.Builder
	cur := p.Current()
	inProgress := cur != nil
	failed := inProgress && cur.Status == wf.StatusFailed

	b.WriteString(`<div class="box">`)
	b.WriteString(`<h2 class="title is-4">Release pipeline</h2>`)
	fmt.Fprintf(&b, `<p>Next version to ship: <strong>%s</strong>.</p>`, p.NextVersion())
	b.WriteString(`<p class="mb-3">Arm <em>Fail next stage</em>, run, then run again: only the failed stage is retried — completed stages are never repeated.</p>`)
	b.WriteString(`<div class="buttons">`)
	b.WriteString(button("start", "Start release "+p.NextVersion(), "is-primary", inProgress))
	next := "Run next stage"
	if failed {
		next = "Retry failed stage"
	}
	b.WriteString(button("next", next, "is-info", !inProgress))
	b.WriteString(button("end", "Run to end", "is-link", !inProgress))
	failClass := "is-danger is-outlined"
	if p.FailNext() {
		failClass = "is-danger"
	}
	b.WriteString(button("fail", "Fail next stage", failClass, false))
	b.WriteString(button("reset", "Reset", "is-warning", false))
	b.WriteString(`</div></div>`)

	if inProgress {
		b.WriteString(`<div class="box">`)
		fmt.Fprintf(&b, `<h3 class="title is-5">Release %s %s</h3>`, cur.Version, statusTag(cur.Status))
		b.WriteString(`<table class="table is-striped is-fullwidth is-narrow">`)
		b.WriteString(`<thead><tr><th>#</th><th>Stage</th><th>Does</th><th>Code</th><th>Status</th><th>Attempts</th><th>Error</th></tr></thead><tbody>`)
		for i, s := range cur.Stages {
			fmt.Fprintf(&b, `<tr><td>%d</td><td><strong>%s</strong></td><td>%s</td><td><code>%s</code></td><td>%s</td><td>%d</td><td class="has-text-danger">%s</td></tr>`,
				i+1, s.Stage.Name, html.EscapeString(s.Stage.Does), html.EscapeString(s.Stage.Code), statusTag(s.Status), s.Attempts, html.EscapeString(s.Error))
		}
		b.WriteString(`</tbody></table></div>`)
	} else {
		b.WriteString(`<div class="box">`)
		b.WriteString(`<h3 class="title is-5">Stages</h3>`)
		b.WriteString(`<table class="table is-striped is-fullwidth is-narrow">`)
		b.WriteString(`<thead><tr><th>#</th><th>Stage</th><th>Does</th><th>Code</th></tr></thead><tbody>`)
		for i, s := range pipeline.Stages {
			fmt.Fprintf(&b, `<tr><td>%d</td><td><strong>%s</strong></td><td>%s</td><td><code>%s</code></td></tr>`, i+1, s.Name, html.EscapeString(s.Does), html.EscapeString(s.Code))
		}
		b.WriteString(`</tbody></table></div>`)
	}

	if h := p.History(); len(h) > 0 {
		b.WriteString(`<div class="box">`)
		b.WriteString(`<h3 class="title is-5">Shipped</h3>`)
		b.WriteString(`<table class="table is-striped is-fullwidth is-narrow">`)
		b.WriteString(`<thead><tr><th>Version</th><th>Status</th><th>Retried stages</th></tr></thead><tbody>`)
		for _, r := range h {
			var retried []string
			for _, s := range r.Stages {
				if s.Attempts > 1 {
					retried = append(retried, fmt.Sprintf("%s ×%d", s.Stage.Name, s.Attempts))
				}
			}
			fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td></tr>`, r.Version, statusTag(r.Status), strings.Join(retried, ", "))
		}
		b.WriteString(`</tbody></table></div>`)
	}

	if log := p.Log(); len(log) > 0 {
		b.WriteString(`<div class="box">`)
		b.WriteString(`<h3 class="title is-5">Log</h3><pre>`)
		start := 0
		if len(log) > 15 {
			start = len(log) - 15
		}
		for _, line := range log[start:] {
			b.WriteString(html.EscapeString(line))
			b.WriteString("\n")
		}
		b.WriteString(`</pre></div>`)
	}
	return b.String()
}
