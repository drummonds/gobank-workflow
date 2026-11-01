package view

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"slices"
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
	case wf.StatusCancelled:
		class = "is-dark"
	}
	return fmt.Sprintf(`<span class="tag %s">%s</span>`, class, s)
}

// stamp is a moment with its date, since runs can span days, and its
// milliseconds, since stages can be shorter than a second.
func stamp(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04:05.000")
}

// took is a duration rounded to its scale: seconds for minutes and more,
// milliseconds for seconds, microseconds below that.
func took(d time.Duration) string {
	switch {
	case d >= time.Minute:
		d = d.Round(time.Second)
	case d >= time.Second:
		d = d.Round(time.Millisecond)
	case d >= time.Millisecond:
		d = d.Round(time.Microsecond)
	}
	return d.String()
}

func runTook(r wf.RunRecord) string {
	if r.CompletedAt.IsZero() {
		return "—"
	}
	return took(r.CompletedAt.Sub(r.StartedAt))
}

// instanceName is what identifies an instance to a reader: its key when
// the definition has one, otherwise its business date.
func instanceName(r wf.RunRecord) string {
	if r.Key != "" {
		return r.Key
	}
	return r.BusinessDate.String()
}

func sourceLink(st wf.Step) string {
	if st.Source == "" {
		return ""
	}
	return fmt.Sprintf(`<a href="%s" title="%s">code</a>`, html.EscapeString(st.Source), html.EscapeString(st.Source))
}

// stepLabel is a step's name, marked ⊞ when it is a sub-process, as the
// diagrams draw it.
func stepLabel(st wf.Step) string {
	if st.Child != "" {
		return st.Name + " ⊞"
	}
	return st.Name
}

// subProcessNote says what a sub-process step starts, how, what it gives
// its children and which step waits for them, linking to the child
// definition; on the step that waits, whose children it waits for; empty
// for a plain step.
func (v *Views) subProcessNote(d wf.Definition, st wf.Step) string {
	if st.Joins != "" {
		return fmt.Sprintf(` <span class="has-text-grey">— waits for every <em>%s</em> child to be through</span>`, html.EscapeString(st.Joins))
	}
	if st.Child == "" {
		return ""
	}
	child := fmt.Sprintf(`<a href="%s"><code>%s</code></a>`, v.DefinitionHref(st.Child), html.EscapeString(string(st.Child)))
	joins := ""
	for _, other := range d.Steps {
		if other.Joins == st.Name {
			joins = fmt.Sprintf("; <em>%s</em> waits for all", html.EscapeString(other.Name))
		}
	}
	var starts, warning string
	switch {
	case st.Mode == wf.OneAtATime && joins == "":
		starts = fmt.Sprintf("starts %s children one at a time and waits for each", child)
	case st.Mode == wf.OneAtATime:
		starts = fmt.Sprintf("starts %s children one at a time%s", child, joins)
	case st.Mode == wf.AllAtOnce && joins == "":
		starts = fmt.Sprintf("starts a %s child per item, all at once", child)
		warning = ` <span class="tag is-warning is-light">nothing waits for the children</span>`
	case st.Mode == wf.AllAtOnce:
		starts = fmt.Sprintf("starts a %s child per item, all at once%s", child, joins)
	default:
		starts = fmt.Sprintf("starts one %s%s", child, joins)
	}
	return fmt.Sprintf(` <span class="has-text-grey">— %s%s</span>%s`, starts, given(st), warning)
}

// given is what a child-starting step gives its children, as a clause;
// empty when the step says nothing.
func given(st wf.Step) string {
	if st.Given == "" {
		return ""
	}
	return ", given " + html.EscapeString(st.Given)
}

// startedBy says who starts a definition's instances: each registry step
// that names it as a child, with its mode and what it gives. Nothing for
// a definition nobody in the registry starts.
func (v *Views) startedBy(b *strings.Builder, t wf.WorkflowType) {
	for _, d := range v.Definitions {
		for _, st := range d.Steps {
			if st.Child != t {
				continue
			}
			mode := ""
			if st.Mode != "" {
				mode = ", " + string(st.Mode)
			}
			fmt.Fprintf(b, `<p><strong>Started by:</strong> <a href="%s"><code>%s</code></a>&#39;s step <em>%s</em>%s%s.</p>`,
				v.DefinitionHref(d.Type), html.EscapeString(string(d.Type)), html.EscapeString(st.Name), mode, given(st))
		}
	}
}

// parentLink names a run's parent and links up to it; the bare id when
// the parent is not in the store.
func (v *Views) parentLink(ctx context.Context, id string) string {
	parent, err := v.Store.GetRun(ctx, id)
	if err != nil || parent == nil {
		return fmt.Sprintf(`<code>%s</code>`, html.EscapeString(id))
	}
	return fmt.Sprintf(`<a href="%s">%s %s</a>`, v.InstanceHref(parent.ID), parent.WorkflowType, html.EscapeString(instanceName(*parent)))
}

// revisionLink is the run's revision, leading to the queue of the
// instances that ran it.
func (v *Views) revisionLink(run wf.RunRecord) string {
	return fmt.Sprintf(`<a href="%s"><code>%s</code></a>`,
		html.EscapeString(v.queueHref(queueFilter{Workflow: string(run.WorkflowType), Revision: run.Revision.ID})), html.EscapeString(run.Revision.ID))
}

// childrenHTML lists the runs started under this one, oldest first: the
// order they were started. Nothing is written for a run with none.
func (v *Views) childrenHTML(ctx context.Context, b *strings.Builder, id string) {
	children, err := v.Store.ListRuns(ctx, wf.RunFilter{ParentID: id})
	if err != nil {
		errorNote(b, err)
		return
	}
	if len(children) == 0 {
		return
	}
	slices.Reverse(children)
	b.WriteString(`<h2 class="subtitle">Children</h2>`)
	b.WriteString(`<table class="table is-striped is-fullwidth is-narrow"><thead><tr><th>Instance</th><th>What</th><th>Status</th><th>Started</th><th>Took</th><th></th></tr></thead><tbody>`)
	for _, c := range children {
		fmt.Fprintf(b, `<tr><td><a href="%s"><strong>%s</strong></a></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			v.preferredHref(c), html.EscapeString(instanceName(c)), v.what(c), statusTag(c.Status), stamp(c.StartedAt), runTook(c), v.instanceLink(c))
	}
	b.WriteString(`</tbody></table>`)
}

// diagramHTML embeds the definition's picture drawn across, with the one
// drawn down in its place on a narrow screen when the host shipped it.
func (v *Views) diagramHTML(b *strings.Builder, t wf.WorkflowType) {
	if _, ok := v.diagram(t, ""); !ok {
		return
	}
	img := fmt.Sprintf(`<img src="%s/diagram.svg" alt="%s steps" style="max-width:100%%">`, v.DefinitionHref(t), t)
	if _, ok := v.diagram(t, ".down"); ok {
		img = fmt.Sprintf(`<picture><source media="(max-width: 768px)" srcset="%s/diagram.down.svg">%s</picture>`, v.DefinitionHref(t), img)
	}
	fmt.Fprintf(b, `<figure class="image my-4">%s</figure>`, img)
}

func errorNote(b *strings.Builder, err error) {
	fmt.Fprintf(b, `<div class="notification is-danger">%s</div>`, html.EscapeString(err.Error()))
}

// instanceHTML is one instance: what it is, its steps laid over the
// revision it ran when they are stages, then the host's Detail.
func (v *Views) instanceHTML(r *http.Request, id string) (template.HTML, bool) {
	run, steps, ok := v.instance(r.Context(), id)
	if !ok {
		return "", false
	}
	def := v.definition(run.WorkflowType)
	// Stage records name a step of the definition and nothing else; records
	// with an account are the host's to show.
	staged := def != nil
	for _, s := range steps {
		if s.AccountID != "" {
			staged = false
		}
	}
	var b strings.Builder
	name := html.EscapeString(instanceName(run))
	fmt.Fprintf(&b, `<nav class="breadcrumb"><ul><li><a href="%sinstances">Instances</a></li><li class="is-active"><a>%s %s</a></li></ul></nav>`, v.Base, run.WorkflowType, name)
	fmt.Fprintf(&b, `<h1 class="title">%s %s %s</h1>`, run.WorkflowType, name, statusTag(run.Status))
	b.WriteString(`<table class="table is-narrow"><tbody>`)
	fmt.Fprintf(&b, `<tr><th>Instance id</th><td><code>%s</code></td></tr>`, html.EscapeString(run.ID))
	if def != nil {
		fmt.Fprintf(&b, `<tr><th>Definition</th><td><a href="%s"><code>%s</code></a></td></tr>`, v.DefinitionHref(run.WorkflowType), run.WorkflowType)
	} else {
		fmt.Fprintf(&b, `<tr><th>Definition</th><td><code>%s</code> <span class="has-text-grey">(not defined in this build)</span></td></tr>`, run.WorkflowType)
	}
	if run.Key != "" {
		fmt.Fprintf(&b, `<tr><th>Key</th><td>%s</td></tr>`, name)
	}
	fmt.Fprintf(&b, `<tr><th>Business date</th><td>%s</td></tr>`, run.BusinessDate)
	if run.ParentID != "" {
		b.WriteString(`<tr><th>Parent</th><td>` + v.parentLink(r.Context(), run.ParentID) + `</td></tr>`)
	}
	fmt.Fprintf(&b, `<tr><th>Status</th><td>%s</td></tr>`, statusTag(run.Status))
	fmt.Fprintf(&b, `<tr><th>Started</th><td>%s</td></tr>`, stamp(run.StartedAt))
	fmt.Fprintf(&b, `<tr><th>Completed</th><td>%s</td></tr>`, stamp(run.CompletedAt))
	fmt.Fprintf(&b, `<tr><th>Took</th><td>%s</td></tr>`, runTook(run))
	if run.Error != "" {
		fmt.Fprintf(&b, `<tr><th>Error</th><td class="has-text-danger">%s</td></tr>`, html.EscapeString(run.Error))
	}
	current := def == nil || run.Revision.ID == "" || run.Revision.ID == def.Revision().ID
	switch {
	case run.Revision.ID == "":
		b.WriteString(`<tr><th>Revision</th><td>— <span class="has-text-grey">(recorded before revisions)</span></td></tr>`)
	case current:
		fmt.Fprintf(&b, `<tr><th>Revision</th><td>%s <span class="has-text-grey">(the current revision)</span></td></tr>`, v.revisionLink(run))
	default:
		fmt.Fprintf(&b, `<tr><th>Revision</th><td>%s <span class="tag is-warning is-light">not the current revision</span> <span class="has-text-grey">the code now runs a different step list</span></td></tr>`, v.revisionLink(run))
	}
	var ran []wf.Step
	if def != nil {
		ran = def.StepsFor(run.Revision)
	}
	stages := latest(steps)
	if staged {
		completed := 0
		for _, s := range stages {
			if s.Status == wf.StatusCompleted {
				completed++
			}
		}
		fmt.Fprintf(&b, `<tr><th>Steps</th><td>%d of %d stages completed</td></tr>`, completed, len(ran))
	} else {
		fmt.Fprintf(&b, `<tr><th>Steps</th><td>%d recorded</td></tr>`, len(steps))
	}
	b.WriteString(`</tbody></table>`)

	switch {
	case staged:
		byName := map[string]wf.StepResult{}
		for _, s := range stages {
			byName[s.StepName] = s
		}
		b.WriteString(`<h2 class="subtitle">Stages</h2>`)
		b.WriteString(`<table class="table is-striped is-fullwidth is-narrow"><thead><tr><th>#</th><th>Stage</th><th>Status</th><th>Took</th><th>Error</th></tr></thead><tbody>`)
		for i, stage := range ran {
			st, recorded := byName[stage.Name]
			status, dur, errMsg := `<span class="tag is-light">not run</span>`, "—", ""
			if recorded {
				status, dur, errMsg = statusTag(st.Status), took(time.Duration(st.DurationNs)), st.Error
			}
			fmt.Fprintf(&b, `<tr><td>%d</td><td><strong>%s</strong> <span class="has-text-grey">%s</span> %s%s</td><td>%s</td><td>%s</td><td class="has-text-danger">%s</td></tr>`,
				i+1, html.EscapeString(stepLabel(stage)), html.EscapeString(stage.Description), sourceLink(stage), v.subProcessNote(*def, stage), status, dur, html.EscapeString(errMsg))
		}
		b.WriteString(`</tbody></table>`)
	case def == nil && len(steps) > 0:
		b.WriteString(`<h2 class="subtitle">Steps</h2>`)
		b.WriteString(`<table class="table is-striped is-fullwidth is-narrow"><thead><tr><th>Step</th><th>Status</th><th>Took</th><th>Error</th></tr></thead><tbody>`)
		for _, s := range stages {
			fmt.Fprintf(&b, `<tr><td><strong>%s</strong></td><td>%s</td><td>%s</td><td class="has-text-danger">%s</td></tr>`,
				html.EscapeString(s.StepName), statusTag(s.Status), took(time.Duration(s.DurationNs)), html.EscapeString(s.Error))
		}
		b.WriteString(`</tbody></table>`)
	}
	v.childrenHTML(r.Context(), &b, run.ID)
	if v.Detail != nil {
		b.WriteString(string(v.Detail(r, run, steps, v.InstanceHref(run.ID))))
	}
	if def != nil && current {
		// The shipped picture is of the current code.
		v.diagramHTML(&b, run.WorkflowType)
	}
	if def != nil {
		fmt.Fprintf(&b, `<p><a href="%s/diagram.d2">This instance as d2</a> — steps coloured by state; render with <code>d2 diagram.d2 out.svg</code>.</p>`, v.InstanceHref(run.ID))
	}
	return template.HTML(b.String()), true
}

// role is a definition's place in a hierarchy, read from the registry: a
// parent when one of its steps starts children, a child when another
// definition's step starts it, both or neither.
func (v *Views) role(d wf.Definition) string {
	var roles []string
	if slices.ContainsFunc(d.Steps, func(st wf.Step) bool { return st.Child != "" }) {
		roles = append(roles, "parent")
	}
	for _, other := range v.Definitions {
		if slices.ContainsFunc(other.Steps, func(st wf.Step) bool { return st.Child == d.Type }) {
			roles = append(roles, "child")
			break
		}
	}
	return strings.Join(roles, ", ")
}

// shownSteps is how many of a definition's steps the list shows before
// counting the rest.
const shownSteps = 4

// firstSteps is a step list as one line: every step when there are few,
// else the first few and a count.
func firstSteps(steps []wf.Step) string {
	var names []string
	for _, st := range steps {
		names = append(names, stepLabel(st))
	}
	if len(names) <= shownSteps {
		return html.EscapeString(strings.Join(names, " → "))
	}
	return fmt.Sprintf(`%s … <span class="has-text-grey">(%d steps)</span>`, html.EscapeString(strings.Join(names[:shownSteps], " → ")), len(names))
}

func (v *Views) definitionsHTML(ctx context.Context) template.HTML {
	counts, err := v.Store.CountRuns(ctx)
	var b strings.Builder
	b.WriteString(`<h1 class="title">Workflow definitions</h1><p class="subtitle">What the engine can run: each definition is a sequence of steps, and each instance is one run of it</p>`)
	if err != nil {
		errorNote(&b, err)
	}
	b.WriteString(`<div class="table-container"><table class="table is-striped is-hoverable is-fullwidth">`)
	b.WriteString(`<thead><tr><th>Purpose</th><th>Type</th><th>Role</th><th>Description</th><th>Instance key</th><th>Steps</th><th>Instances</th></tr></thead><tbody>`)
	for _, d := range v.Definitions {
		c := counts[d.Type]
		purpose := "—"
		if d.Purpose.Name != "" {
			purpose = fmt.Sprintf(`<strong>%s</strong><br><span class="has-text-grey">%s</span>`, html.EscapeString(d.Purpose.Name), html.EscapeString(d.Purpose.Description))
		}
		fmt.Fprintf(&b, `<tr><td>%s</td><td><a href="%s"><code>%s</code></a></td><td class="role">%s</td><td>%s</td><td>%s</td><td>%s</td><td>%d <span class="has-text-grey">(%d completed, %d failed)</span></td></tr>`,
			purpose, v.DefinitionHref(d.Type), d.Type, v.role(d), html.EscapeString(d.Description), html.EscapeString(d.InstanceKey), firstSteps(d.Steps), c.Total(), c[wf.StatusCompleted], c[wf.StatusFailed])
	}
	b.WriteString(`</tbody></table></div>`)
	return template.HTML(b.String())
}

func (v *Views) definitionHTML(ctx context.Context, t wf.WorkflowType) (template.HTML, bool) {
	def := v.definition(t)
	if def == nil {
		return "", false
	}
	counts, err := v.Store.CountRuns(ctx)
	c := counts[t]
	var b strings.Builder
	fmt.Fprintf(&b, `<nav class="breadcrumb"><ul><li><a href="%sdefinitions">Definitions</a></li><li class="is-active"><a>%s</a></li></ul></nav>`, v.Base, t)
	fmt.Fprintf(&b, `<h1 class="title"><code>%s</code></h1><p class="subtitle">%s</p>`, t, html.EscapeString(def.Description))
	if def.Purpose.Name != "" {
		fmt.Fprintf(&b, `<p><strong>Purpose:</strong> %s — %s</p>`, html.EscapeString(def.Purpose.Name), html.EscapeString(def.Purpose.Description))
	}
	fmt.Fprintf(&b, `<p><strong>Instance key:</strong> %s — one instance per %s.</p>`, html.EscapeString(def.InstanceKey), html.EscapeString(def.InstanceKey))
	v.startedBy(&b, t)
	b.WriteString(`<h2 class="subtitle mt-4">Steps</h2>`)
	b.WriteString(`<table class="table is-narrow is-striped"><thead><tr><th>#</th><th>Step</th><th>Does</th><th>Code</th></tr></thead><tbody>`)
	for i, st := range def.Steps {
		fmt.Fprintf(&b, `<tr><td>%d</td><td><strong>%s</strong></td><td>%s%s</td><td>%s</td></tr>`, i+1, html.EscapeString(stepLabel(st)), html.EscapeString(st.Description), v.subProcessNote(*def, st), sourceLink(st))
	}
	b.WriteString(`</tbody></table>`)
	v.diagramHTML(&b, t)
	b.WriteString(`<h2 class="subtitle mt-4">Instances <span class="has-text-grey">(runs of this definition)</span></h2>`)
	if err != nil {
		errorNote(&b, err)
	}
	b.WriteString(`<div class="tags are-medium">`)
	for _, st := range []wf.RunStatus{wf.StatusPending, wf.StatusRunning, wf.StatusWaiting, wf.StatusCompleted, wf.StatusFailed, wf.StatusCancelled} {
		fmt.Fprintf(&b, `<span class="tag">%s&nbsp;<strong>%d</strong></span>`, st, c[st])
	}
	fmt.Fprintf(&b, `<span class="tag is-dark">instances&nbsp;<strong>%d</strong></span></div>`, c.Total())
	fmt.Fprintf(&b, `<p><a href="%s">Filter these instances</a></p>`, html.EscapeString(v.queueHref(queueFilter{Workflow: string(t)})))
	v.queueHTML(ctx, &b, queueFilter{Workflow: string(t)})
	return template.HTML(b.String()), true
}
