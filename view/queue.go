package view

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

const (
	queueDefault = 50  // instances the queue shows unless n says otherwise
	queueMax     = 500 // the most n may ask for
)

// queueFilter is the instances page's GET form: what the operator asked
// to see, as written in the query string.
type queueFilter struct {
	Purpose, Workflow, Status, Since, Key, Revision string
	N, Page                                         int
}

func parseQueueFilter(r *http.Request) queueFilter {
	q := queueFilter{
		Purpose: r.FormValue("purpose"), Workflow: r.FormValue("workflow"), Status: r.FormValue("status"),
		Since: r.FormValue("since"), Key: r.FormValue("key"), Revision: r.FormValue("revision"),
	}
	q.N, _ = strconv.Atoi(r.FormValue("n"))
	q.Page, _ = strconv.Atoi(r.FormValue("page"))
	q.Page = max(q.Page, 1)
	return q
}

func (q queueFilter) n() int {
	if q.N <= 0 {
		return queueDefault
	}
	return min(q.N, queueMax)
}

func (q queueFilter) narrowed() bool {
	return q.Purpose != "" || q.Workflow != "" || q.Status != "" || q.Since != "" || q.Key != "" || q.Revision != ""
}

// queueHref is the queue under q.
func (v *Views) queueHref(q queueFilter) string {
	vals := url.Values{}
	for k, s := range map[string]string{"purpose": q.Purpose, "workflow": q.Workflow, "status": q.Status, "since": q.Since, "key": q.Key, "revision": q.Revision} {
		if s != "" {
			vals.Set(k, s)
		}
	}
	if q.N > 0 {
		vals.Set("n", strconv.Itoa(q.N))
	}
	if q.Page > 0 {
		vals.Set("page", strconv.Itoa(q.Page))
	}
	if len(vals) == 0 {
		return v.Base + "instances"
	}
	return v.Base + "instances?" + vals.Encode()
}

// storeFilter is the filter as the store takes it, without the status and
// limit the queue sets per query; false when nothing can match, as for a
// purpose no definition serves. The queue lists the top-level instances,
// a child being read on its parent's page, unless the filter names a
// purpose, workflow, revision or key: then the operator has asked for
// children by name and they are listed too.
func (v *Views) storeFilter(q queueFilter) (wf.RunFilter, bool) {
	f := wf.RunFilter{KeyContains: q.Key, Revision: q.Revision,
		TopLevel: q.Purpose == "" && q.Workflow == "" && q.Revision == "" && q.Key == ""}
	if q.Purpose != "" {
		f.Types = []wf.WorkflowType{} // set, so an unserved purpose matches nothing
		for _, d := range v.Definitions {
			if d.Purpose.Name == q.Purpose {
				f.Types = append(f.Types, d.Type)
			}
		}
	}
	if q.Workflow != "" {
		t := wf.WorkflowType(q.Workflow)
		if f.Types == nil || slices.Contains(f.Types, t) {
			f.Types = []wf.WorkflowType{t}
		} else {
			f.Types = []wf.WorkflowType{}
		}
	}
	if q.Since != "" {
		if since, err := time.ParseInLocation("2006-01-02", q.Since, time.Local); err == nil {
			f.Since = since
		}
	}
	return f, f.Types == nil || len(f.Types) > 0
}

// queueRuns is one page of the queue: the active instances, then the
// completed, newest first in each; or, when a status is asked for, the
// newest in it. more says a later page has instances.
func (v *Views) queueRuns(ctx context.Context, q queueFilter) (runs []wf.RunRecord, more bool, err error) {
	f, ok := v.storeFilter(q)
	if !ok {
		return nil, false, nil
	}
	if q.Status != "" {
		f.Statuses = []wf.RunStatus{wf.RunStatus(q.Status)}
	}
	f.ActiveFirst = q.Status == ""
	f.Offset, f.Limit = (max(q.Page, 1)-1)*q.n(), q.n()+1
	runs, err = v.Store.ListRuns(ctx, f)
	if len(runs) > q.n() {
		return runs[:q.n()], true, err
	}
	return runs, false, err
}

// headline is a group's newest instance under the filter, on whatever page.
func (v *Views) headline(ctx context.Context, q queueFilter, group queueFilter, fallback wf.RunRecord) wf.RunRecord {
	if group.Purpose != "" {
		q.Purpose = group.Purpose
	} else {
		q.Workflow = group.Workflow
	}
	f, ok := v.storeFilter(q)
	if !ok {
		return fallback
	}
	if q.Status != "" {
		f.Statuses = []wf.RunStatus{wf.RunStatus(q.Status)}
	}
	f.Limit = 1
	if runs, err := v.Store.ListRuns(ctx, f); err == nil && len(runs) == 1 {
		return runs[0]
	}
	return fallback
}

// purposeOf is the group a run is queued under: its definition's purpose,
// or its type when the definition has none or is not in this build.
func (v *Views) purposeOf(r wf.RunRecord) (name string, filter queueFilter) {
	if d := v.definition(r.WorkflowType); d != nil && d.Purpose.Name != "" {
		return d.Purpose.Name, queueFilter{Purpose: d.Purpose.Name}
	}
	return string(r.WorkflowType), queueFilter{Workflow: string(r.WorkflowType)}
}

func (v *Views) instancesHTML(r *http.Request) string {
	q := parseQueueFilter(r)
	var b strings.Builder
	b.WriteString(`<h1 class="title">Workflow instances</h1><p class="subtitle">The active work — running, waiting and failed — then the newest completed, by purpose; a child instance is on its parent's page unless the filter names its purpose, workflow or key</p>`)
	v.filterForm(&b, q)
	if q.Workflow != "" {
		if def := v.definition(wf.WorkflowType(q.Workflow)); def != nil {
			fmt.Fprintf(&b, `<p>Definition: <a href="%s"><code>%s</code></a> — %s</p>`, v.DefinitionHref(def.Type), def.Type, html.EscapeString(def.Description))
		}
	}
	v.queueHTML(r.Context(), &b, q)
	return b.String()
}

// filterForm is the GET form over the queue, holding the filter it shows.
func (v *Views) filterForm(b *strings.Builder, q queueFilter) {
	option := func(value, label, chosen string) {
		sel := ""
		if value == chosen {
			sel = " selected"
		}
		fmt.Fprintf(b, `<option value="%s"%s>%s</option>`, html.EscapeString(value), sel, html.EscapeString(label))
	}
	field := func(label string) {
		fmt.Fprintf(b, `<div class="field mb-0"><label class="label is-small">%s</label><div class="control">`, label)
	}
	end := `</div></div>`
	fmt.Fprintf(b, `<form method="get" action="%sinstances" class="box"><div class="is-flex is-flex-wrap-wrap is-align-items-flex-end" style="gap:.75rem">`, v.Base)
	field("Purpose")
	b.WriteString(`<div class="select is-small"><select name="purpose">`)
	option("", "any", q.Purpose)
	var seen []string
	for _, d := range v.Definitions {
		if n := d.Purpose.Name; n != "" && !slices.Contains(seen, n) {
			seen = append(seen, n)
			option(n, n, q.Purpose)
		}
	}
	b.WriteString(`</select></div>` + end)
	field("Workflow")
	b.WriteString(`<div class="select is-small"><select name="workflow">`)
	option("", "any", q.Workflow)
	for _, d := range v.Definitions {
		option(string(d.Type), string(d.Type), q.Workflow)
	}
	b.WriteString(`</select></div>` + end)
	field("Status")
	b.WriteString(`<div class="select is-small"><select name="status">`)
	option("", "active, then completed", q.Status)
	for _, s := range []wf.RunStatus{wf.StatusPending, wf.StatusRunning, wf.StatusWaiting, wf.StatusFailed, wf.StatusCompleted, wf.StatusCancelled} {
		option(string(s), string(s), q.Status)
	}
	b.WriteString(`</select></div>` + end)
	field("Since")
	fmt.Fprintf(b, `<input class="input is-small" type="date" name="since" value="%s">`+end, html.EscapeString(q.Since))
	field("Key contains")
	fmt.Fprintf(b, `<input class="input is-small" type="text" name="key" value="%s" size="12">`+end, html.EscapeString(q.Key))
	field("n")
	fmt.Fprintf(b, `<input class="input is-small" type="number" name="n" min="1" max="%d" value="%d" style="width:5rem">`+end, queueMax, q.n())
	if q.Revision != "" {
		fmt.Fprintf(b, `<input type="hidden" name="revision" value="%s">`, html.EscapeString(q.Revision))
	}
	b.WriteString(`<div class="field mb-0"><div class="control"><button class="button is-small is-link">Filter</button></div></div>`)
	if q.narrowed() {
		fmt.Fprintf(b, `<div class="field mb-0"><div class="control"><a class="button is-small is-light" href="%sinstances">Clear</a></div></div>`, v.Base)
	}
	b.WriteString(`</div>`)
	if q.Revision != "" {
		without := q
		without.Revision = ""
		fmt.Fprintf(b, `<p class="mt-2"><span class="tags has-addons"><span class="tag">revision <code>%s</code></span><a class="tag is-delete" href="%s" aria-label="any revision"></a></span></p>`,
			html.EscapeString(q.Revision), html.EscapeString(v.queueHref(without)))
	}
	b.WriteString(`</form>`)
}

// queueHTML writes the queue under q, one table per purpose, each headed by
// the purpose's newest instance.
func (v *Views) queueHTML(ctx context.Context, b *strings.Builder, q queueFilter) {
	q.Page = max(q.Page, 1)
	runs, more, err := v.queueRuns(ctx, q)
	if err != nil {
		errorNote(b, err)
		return
	}
	if len(runs) == 0 {
		if q.Page > 1 {
			first := q
			first.Page = 0
			fmt.Fprintf(b, `<div class="notification">No instances on page %d. <a href="%s">First page</a></div>`, q.Page, html.EscapeString(v.queueHref(first)))
		} else if q.narrowed() {
			b.WriteString(`<div class="notification">No instances match this filter.</div>`)
		} else {
			b.WriteString(`<div class="notification">No instances yet.</div>`)
		}
		return
	}
	var order []string
	groups := map[string][]wf.RunRecord{}
	for _, r := range runs {
		name, _ := v.purposeOf(r)
		if groups[name] == nil {
			order = append(order, name)
		}
		groups[name] = append(groups[name], r)
	}
	for _, name := range order {
		rs := groups[name]
		_, filter := v.purposeOf(rs[0])
		newest := v.headline(ctx, q, filter, rs[0])
		fmt.Fprintf(b, `<h2 class="subtitle mt-5 mb-2"><a href="%s"><strong>%s</strong></a>: <a href="%s">%s</a> %s via <a href="%s"><code>%s</code></a></h2>`,
			html.EscapeString(v.queueHref(filter)), html.EscapeString(name), v.preferredHref(newest), html.EscapeString(instanceName(newest)), statusTag(newest.Status),
			html.EscapeString(v.queueHref(queueFilter{Workflow: string(newest.WorkflowType)})), newest.WorkflowType)
		b.WriteString(`<div class="table-container"><table class="table is-striped is-hoverable is-fullwidth is-narrow">`)
		b.WriteString(`<thead><tr><th>Instance</th><th>What</th><th>Business date</th><th>Status</th><th>Started</th><th>Completed</th><th>Took</th><th>Error</th><th></th></tr></thead><tbody>`)
		for _, r := range rs {
			fmt.Fprintf(b, `<tr><td><a href="%s"><strong>%s</strong></a></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td class="has-text-danger">%s</td><td><div class="buttons is-flex-wrap-nowrap">%s%s</div></td></tr>`,
				v.preferredHref(r), html.EscapeString(instanceName(r)), v.what(r), r.BusinessDate,
				statusTag(r.Status), stamp(r.StartedAt), stamp(r.CompletedAt), runTook(r), html.EscapeString(r.Error), v.instanceLink(r), v.cancelButton(r))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	if q.Page > 1 || more {
		at := func(page int) string {
			p := q
			p.Page = page
			return html.EscapeString(v.queueHref(p))
		}
		b.WriteString(`<nav class="pagination is-small mt-4" role="navigation" aria-label="queue pages">`)
		if q.Page > 1 {
			fmt.Fprintf(b, `<a class="pagination-previous" href="%s">Previous</a>`, at(q.Page-1))
		}
		if more {
			fmt.Fprintf(b, `<a class="pagination-next" href="%s">Next</a>`, at(q.Page+1))
		}
		fmt.Fprintf(b, `<ul class="pagination-list"><li><a class="pagination-link is-current" aria-current="page">%d</a></li></ul></nav>`, q.Page)
	}
}

// cancelButton is the queue row's Cancel, a POST, when the run can be cancelled.
func (v *Views) cancelButton(r wf.RunRecord) string {
	if !v.cancellable(r) {
		return ""
	}
	return fmt.Sprintf(`<form method="post" action="%s/cancel"><button class="button is-small is-danger is-light">Cancel</button></form>`, v.InstanceHref(r.ID))
}
