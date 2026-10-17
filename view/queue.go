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

// current are the statuses of instances still in hand, which the queue
// lists before the completed ones.
var current = []wf.RunStatus{wf.StatusPending, wf.StatusRunning, wf.StatusWaiting, wf.StatusFailed}

// queueFilter is the instances page's GET form: what the operator asked
// to see, as written in the query string.
type queueFilter struct {
	Purpose, Workflow, Status, Since, Key, Revision string
	N                                               int
}

func parseQueueFilter(r *http.Request) queueFilter {
	q := queueFilter{
		Purpose: r.FormValue("purpose"), Workflow: r.FormValue("workflow"), Status: r.FormValue("status"),
		Since: r.FormValue("since"), Key: r.FormValue("key"), Revision: r.FormValue("revision"),
	}
	q.N, _ = strconv.Atoi(r.FormValue("n"))
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
	if len(vals) == 0 {
		return v.Base + "instances"
	}
	return v.Base + "instances?" + vals.Encode()
}

// storeFilter is the filter as the store takes it, without the status and
// limit the queue sets per query; false when nothing can match, as for a
// purpose no definition serves.
func (v *Views) storeFilter(q queueFilter) (wf.RunFilter, bool) {
	f := wf.RunFilter{KeyContains: q.Key, Revision: q.Revision}
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

// queueRuns is the queue: the current instances, then the newest
// completed, n in all; or, when a status is asked for, the newest n in it.
func (v *Views) queueRuns(ctx context.Context, q queueFilter) ([]wf.RunRecord, error) {
	f, ok := v.storeFilter(q)
	if !ok {
		return nil, nil
	}
	f.Limit = q.n()
	if q.Status != "" {
		f.Statuses = []wf.RunStatus{wf.RunStatus(q.Status)}
		return v.Store.ListRuns(ctx, f)
	}
	f.Statuses = current
	runs, err := v.Store.ListRuns(ctx, f)
	if err != nil || len(runs) == f.Limit {
		return runs, err
	}
	f.Statuses, f.Limit = []wf.RunStatus{wf.StatusCompleted}, f.Limit-len(runs)
	done, err := v.Store.ListRuns(ctx, f)
	return append(runs, done...), err
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
	b.WriteString(`<h1 class="title">Workflow instances</h1><p class="subtitle">The work in hand — running, waiting and failed — then the newest completed, by purpose</p>`)
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
		fmt.Fprintf(b, `<div class="field"><label class="label is-small">%s</label><div class="control">`, label)
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
	option("", "in hand, then completed", q.Status)
	for _, s := range []wf.RunStatus{wf.StatusPending, wf.StatusRunning, wf.StatusWaiting, wf.StatusFailed, wf.StatusCompleted} {
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
	b.WriteString(`<div class="field"><div class="control"><button class="button is-small is-link">Filter</button></div></div>`)
	if q.narrowed() {
		fmt.Fprintf(b, `<div class="field"><div class="control"><a class="button is-small is-light" href="%sinstances">Clear</a></div></div>`, v.Base)
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
	runs, err := v.queueRuns(ctx, q)
	if err != nil {
		errorNote(b, err)
		return
	}
	if len(runs) == 0 {
		if q.narrowed() {
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
		newest := rs[0]
		for _, r := range rs {
			if r.CreatedAt.After(newest.CreatedAt) {
				newest = r
			}
		}
		_, filter := v.purposeOf(newest)
		fmt.Fprintf(b, `<h2 class="subtitle mt-5 mb-2"><a href="%s"><strong>%s</strong></a>: <a href="%s">%s</a> %s via <code>%s</code></h2>`,
			html.EscapeString(v.queueHref(filter)), html.EscapeString(name), v.InstanceHref(newest.ID), html.EscapeString(instanceName(newest)), statusTag(newest.Status), newest.WorkflowType)
		b.WriteString(`<div class="table-container"><table class="table is-striped is-hoverable is-fullwidth is-narrow">`)
		b.WriteString(`<thead><tr><th>Instance</th><th>Workflow</th><th>Revision</th><th>Business date</th><th>Status</th><th>Started</th><th>Completed</th><th>Took</th><th>Error</th><th></th></tr></thead><tbody>`)
		for _, r := range rs {
			rev := "—"
			if r.Revision.ID != "" {
				rev = fmt.Sprintf(`<a href="%s"><code>%s</code></a>`, html.EscapeString(v.queueHref(queueFilter{Workflow: string(r.WorkflowType), Revision: r.Revision.ID})), r.Revision.ID)
			}
			fmt.Fprintf(b, `<tr><td><strong>%s</strong></td><td><a href="%s"><code>%s</code></a></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td class="has-text-danger">%s</td><td><a class="button is-small is-link is-light" href="%s">View</a></td></tr>`,
				html.EscapeString(instanceName(r)), html.EscapeString(v.queueHref(queueFilter{Workflow: string(r.WorkflowType)})), r.WorkflowType, rev, r.BusinessDate,
				statusTag(r.Status), stamp(r.StartedAt), stamp(r.CompletedAt), runTook(r), html.EscapeString(r.Error), v.InstanceHref(r.ID))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	if len(runs) == q.n() {
		fmt.Fprintf(b, `<p class="has-text-grey">Showing the first %d; raise n to see more.</p>`, q.n())
	}
}
