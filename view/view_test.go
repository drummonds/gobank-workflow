package view_test

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/view"
)

// store is a fixed set of runs and steps, in the order a store returned them.
type store struct {
	runs  []wf.RunRecord
	steps map[string][]wf.StepResult
}

func (s *store) GetRun(_ context.Context, id string) (*wf.RunRecord, error) {
	for _, r := range s.runs {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, nil
}

func (s *store) ListSteps(_ context.Context, runID string) ([]wf.StepResult, error) {
	return s.steps[runID], nil
}

// ListRuns filters as the memory store does.
func (s *store) ListRuns(ctx context.Context, f wf.RunFilter) ([]wf.RunRecord, error) {
	m := wf.NewMemStore()
	for _, r := range s.runs {
		_ = m.CreateRun(ctx, &r)
	}
	return m.ListRuns(ctx, f)
}

// CountMatching counts as the memory store does.
func (s *store) CountMatching(ctx context.Context, f wf.RunFilter) (int, error) {
	m := wf.NewMemStore()
	for _, r := range s.runs {
		_ = m.CreateRun(ctx, &r)
	}
	return m.CountMatching(ctx, f)
}

func (s *store) CountRuns(context.Context) (map[wf.WorkflowType]wf.RunCounts, error) {
	counts := map[wf.WorkflowType]wf.RunCounts{}
	for _, r := range s.runs {
		if counts[r.WorkflowType] == nil {
			counts[r.WorkflowType] = wf.RunCounts{}
		}
		counts[r.WorkflowType][r.Status]++
	}
	return counts, nil
}

var (
	t0       = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	shipping = wf.Definition{
		Type: "ship", Purpose: wf.Purpose{Name: "release", Description: "Get a version out."},
		Description: "Ship one version.", InstanceKey: "version",
		Steps: []wf.Step{{Name: "build", Description: "Compile it."}, {Name: "tag", Description: "Tag it.", Source: "https://example.org/tag.go"}, {Name: "publish", Description: "Put it out."}},
	}
	accrual = wf.Definition{
		Type: "accrue", Purpose: wf.Purpose{Name: "end of day", Description: "Close the books."},
		Description: "Accrue interest.", InstanceKey: "business date",
		Steps: []wf.Step{{Name: "accrue"}},
	}
)

func run(id string, def wf.Definition, key string, status wf.RunStatus, started time.Time) wf.RunRecord {
	return wf.RunRecord{ID: id, WorkflowType: def.Type, Key: key, BusinessDate: wf.NewBusinessDate(2026, time.March, 1),
		Revision: def.Revision(), Status: status, StartedAt: started, CreatedAt: started}
}

func stage(runID, name string, status wf.RunStatus, at time.Time) wf.StepResult {
	return wf.StepResult{ID: runID + name + at.String(), RunID: runID, StepName: name, Status: status, DurationNs: int64(2 * time.Second), CreatedAt: at}
}

// page is what the views handed the host's frame.
type page struct {
	status int
	active string
	body   string
}

func serve(t *testing.T, v *view.Views, path string) page {
	t.Helper()
	var p page
	v.Frame = func(w http.ResponseWriter, r *http.Request, pg view.Page) {
		p.active = pg.Active
		io.WriteString(w, string(pg.HTML))
	}
	rec := httptest.NewRecorder()
	http.StripPrefix("/workflows", v.Handler()).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	p.status, p.body = rec.Code, rec.Body.String()
	return p
}

func views(s *store) *view.Views {
	return &view.Views{Store: s, Definitions: []wf.Definition{shipping, accrual}, Base: "/workflows/"}
}

func mustContain(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Fatalf("body missing %q\n---\n%s", w, body)
		}
	}
}

// queue is a store with active work: two purposes, every status, and a
// child instance under the waiting one.
func queue() *store {
	day := t0.Add(-24 * time.Hour)
	child := run("w-child", shipping, "v1.2.0/1", wf.StatusRunning, t0.Add(2*time.Hour+time.Minute))
	child.ParentID = "w"
	return &store{runs: []wf.RunRecord{
		run("c-old", shipping, "v1.0.0", wf.StatusCompleted, day),
		run("f", shipping, "v1.1.0", wf.StatusFailed, t0.Add(time.Hour)),
		run("w", shipping, "v1.2.0", wf.StatusWaiting, t0.Add(2*time.Hour)),
		child,
		run("c-new", shipping, "v1.3.0", wf.StatusCompleted, t0.Add(3*time.Hour)),
		run("eod", accrual, "", wf.StatusRunning, t0.Add(4*time.Hour)),
	}}
}

// queueIDs is every instance in queue(), top level and child.
var queueIDs = []string{"eod", "w", "w-child", "f", "c-new", "c-old"}

// inOrder fails unless each want appears in body, each after the last.
func inOrder(t *testing.T, body string, wants ...string) {
	t.Helper()
	at := 0
	for _, w := range wants {
		i := strings.Index(body[at:], w)
		if i < 0 {
			t.Fatalf("%q missing, or not after %q\n---\n%s", w, body[:at], body)
		}
		at += i + len(w)
	}
}

func TestTheQueueIsOneTableOfCurrentInstancesBeforeTheNewestCompleted(t *testing.T) {
	p := serve(t, views(queue()), "/workflows/instances")
	if p.status != 200 || p.active != "instances" {
		t.Fatalf("status %d active %q", p.status, p.active)
	}
	queued := p.body[strings.Index(p.body, "</form>"):]
	// Current instances come before completed ones, each newest first,
	// whatever their purpose; a row names its purpose, leading to that
	// purpose's queue.
	inOrder(t, queued, `href="/workflows/instances/eod"`, `href="/workflows/instances?purpose=end+of+day">end of day</a>`,
		`href="/workflows/instances/w"`, `href="/workflows/instances?purpose=release">release</a>`,
		`href="/workflows/instances/f"`, `href="/workflows/instances/c-new"`, `href="/workflows/instances/c-old"`)
	if strings.Count(queued, "<table") != 1 || strings.Contains(queued, "<h2") {
		t.Fatalf("the queue should be one table with no group headings:\n%s", queued)
	}
	if root := serve(t, views(queue()), "/workflows/"); root.body != p.body {
		t.Fatal("the base itself should be the queue")
	}
	mustContain(t, serve(t, views(&store{}), "/workflows/instances").body, "No instances yet")
}

// pageNav is the queue's page navigation as its words: Previous, the page
// numbers and ellipses, Next.
func pageNav(t *testing.T, body string) []string {
	t.Helper()
	i, j := strings.Index(body, "<nav"), strings.LastIndex(body, "</nav>")
	if i < 0 || j < 0 {
		return nil
	}
	nav := body[i:j]
	if strings.Count(nav, "<ul") != 1 || strings.Index(nav, "<ul") > strings.Index(nav, "<a") {
		t.Fatalf("the page navigation should be one group:\n%s", nav)
	}
	return strings.Fields(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(nav, " "))
}

func TestTheQueuePagesNAtATime(t *testing.T) {
	for _, c := range []struct {
		query string
		shown []string // instance rows, in order
		nav   []string // the page navigation, as words
	}{
		{"n=2", []string{"eod", "w"}, []string{"1", "2", "3", "Next"}},
		{"n=2&page=2", []string{"f", "c-new"}, []string{"Previous", "1", "2", "3", "Next"}},
		{"n=2&page=3", []string{"c-old"}, []string{"Previous", "1", "2", "3"}},
		{"n=5", []string{"eod", "w", "f", "c-new", "c-old"}, nil},
	} {
		body := serve(t, views(queue()), "/workflows/instances?"+c.query).body
		var rows []string
		for _, id := range c.shown {
			rows = append(rows, `href="/workflows/instances/`+id+`"><strong>`)
		}
		inOrder(t, body, rows...)
		if n := strings.Count(body, `</strong></a></td>`); n != len(c.shown) {
			t.Errorf("%s: %d rows, want %d", c.query, n, len(c.shown))
		}
		if nav := pageNav(t, body); !slices.Equal(nav, c.nav) {
			t.Errorf("%s: page navigation %q, want %q", c.query, nav, c.nav)
		}
	}
	page2 := serve(t, views(queue()), "/workflows/instances?n=2&page=2").body
	mustContain(t, page2, `href="/workflows/instances?n=2&amp;page=1">Previous</a>`, `href="/workflows/instances?n=2&amp;page=3">3</a>`,
		`href="/workflows/instances?n=2&amp;page=3">Next</a>`, `aria-current="page">2</a>`)
	mustContain(t, serve(t, views(queue()), "/workflows/instances?n=2&page=9").body, "No instances on page 9")
}

func TestThePageNavShowsUpToFivePagesAroundTheCurrent(t *testing.T) {
	s := &store{}
	for i := range 12 {
		s.runs = append(s.runs, run(fmt.Sprintf("r%02d", i), shipping, fmt.Sprintf("v0.%d.0", i), wf.StatusCompleted, t0.Add(time.Duration(i)*time.Minute)))
	}
	for _, c := range []struct {
		page int
		nav  []string
	}{
		{1, []string{"1", "2", "…", "12", "Next"}},
		{2, []string{"Previous", "1", "2", "3", "…", "12", "Next"}},
		{6, []string{"Previous", "1", "…", "5", "6", "7", "…", "12", "Next"}},
		{11, []string{"Previous", "1", "…", "10", "11", "12", "Next"}},
		{12, []string{"Previous", "1", "…", "11", "12"}},
	} {
		body := serve(t, views(s), fmt.Sprintf("/workflows/instances?n=1&page=%d", c.page)).body
		if nav := pageNav(t, body); !slices.Equal(nav, c.nav) {
			t.Errorf("page %d: navigation %q, want %q", c.page, nav, c.nav)
		}
	}
}

func TestTheFilterNarrowsTheQueue(t *testing.T) {
	for _, c := range []struct {
		query string
		shown []string // instance ids listed
		echo  string   // the form keeps the filter
	}{
		{"", []string{"eod", "w", "f", "c-new", "c-old"}, ""},
		{"purpose=release", []string{"w", "w-child", "f", "c-new", "c-old"}, `<option value="release" selected>`},
		{"workflow=accrue", []string{"eod"}, `<option value="accrue" selected>`},
		{"workflow=ship", []string{"w", "w-child", "f", "c-new", "c-old"}, `<option value="ship" selected>`},
		{"status=failed", []string{"f"}, `<option value="failed" selected>`},
		{"status=running", []string{"eod"}, `<option value="running" selected>`},
		{"since=2026-03-01", []string{"eod", "w", "f", "c-new"}, `value="2026-03-01"`},
		{"key=1.3", []string{"c-new"}, `value="1.3"`},
		{"key=1.2", []string{"w", "w-child"}, `value="1.2"`},
		{"workflow=ship&revision=" + shipping.Revision().ID, []string{"w", "w-child", "f", "c-new", "c-old"}, shipping.Revision().ID},
		{"purpose=release&workflow=accrue", nil, ""},
		{"purpose=nobody", nil, ""},
	} {
		body := serve(t, views(queue()), "/workflows/instances?"+c.query).body
		for _, id := range queueIDs {
			if listed := strings.Contains(body, `href="/workflows/instances/`+id+`"`); listed != slices.Contains(c.shown, id) {
				t.Errorf("%s: %s listed %v, want %v", c.query, id, listed, !listed)
			}
		}
		mustContain(t, body, c.echo)
		if len(c.shown) == 0 {
			mustContain(t, body, "No instances match")
		}
	}
}

func TestTheQueueDrillsDownFromPurposeToWorkflowToRevision(t *testing.T) {
	body := serve(t, views(queue()), "/workflows/instances").body
	mustContain(t, body, `href="/workflows/instances?purpose=release"`, `href="/workflows/definitions/ship"`)
	// A purpose's queue leads to each workflow's definition, and that to the workflow's queue.
	mustContain(t, serve(t, views(queue()), "/workflows/definitions/ship").body, `href="/workflows/instances?workflow=ship"`)
	// An instance's revision leads to the queue of that revision's instances.
	mustContain(t, serve(t, views(queue()), "/workflows/instances/w").body,
		`href="/workflows/instances?revision=`+shipping.Revision().ID+`&amp;workflow=ship"`)
	// A workflow's queue leads on to its definition.
	mustContain(t, serve(t, views(queue()), "/workflows/instances?workflow=ship").body, `href="/workflows/definitions/ship"`)
}

func TestARunOfAnUnknownDefinitionIsQueuedUnderItsType(t *testing.T) {
	r := wf.RunRecord{ID: "x1", WorkflowType: "retired", Key: "k", Status: wf.StatusFailed, StartedAt: t0, CreatedAt: t0}
	body := serve(t, views(&store{runs: []wf.RunRecord{r}}), "/workflows/instances").body
	mustContain(t, body, "retired", `href="/workflows/instances/x1"`)
}

func TestAnInstanceShowsEachStageInItsLatestRecordedState(t *testing.T) {
	r := run("r1", shipping, "v1.1.0", wf.StatusCompleted, t0)
	// The store lists the retry's record before the failure it replaced:
	// the state shown is the latest by time, not by listing order.
	s := &store{runs: []wf.RunRecord{r}, steps: map[string][]wf.StepResult{"r1": {
		stage("r1", "build", wf.StatusCompleted, t0.Add(time.Second)),
		stage("r1", "tag", wf.StatusCompleted, t0.Add(time.Minute)),
		stage("r1", "tag", wf.StatusFailed, t0.Add(2*time.Second)),
	}}}
	p := serve(t, views(s), "/workflows/instances/r1")
	if p.status != 200 || p.active != "instances" {
		t.Fatalf("status %d active %q", p.status, p.active)
	}
	mustContain(t, p.body, "v1.1.0", "<code>r1</code>", "2 of 3 stages completed", "not run", "Tag it.", `href="https://example.org/tag.go"`,
		`href="/workflows/instances/r1/diagram.d2"`, "the current revision")
	if strings.Contains(p.body, "is-danger") {
		t.Fatalf("tag was retried to completion; nothing should read failed:\n%s", p.body)
	}
	if p := serve(t, views(s), "/workflows/instances/nope"); p.status != 404 {
		t.Fatalf("unknown instance: status %d", p.status)
	}
}

func TestAnInstanceRendersFromTheRevisionItRan(t *testing.T) {
	r := run("old", shipping, "v0.9.0", wf.StatusCompleted, t0)
	r.Revision = wf.NewRevision([]string{"build", "publish"})
	s := &store{runs: []wf.RunRecord{r}, steps: map[string][]wf.StepResult{"old": {
		stage("old", "build", wf.StatusCompleted, t0), stage("old", "publish", wf.StatusCompleted, t0.Add(time.Second)),
	}}}
	p := serve(t, views(s), "/workflows/instances/old")
	mustContain(t, p.body, "2 of 2 stages completed", r.Revision.ID, "not the current revision")
	if strings.Contains(p.body, "Tag it.") {
		t.Fatalf("a stage the run's revision did not have should not be listed:\n%s", p.body)
	}
}

func TestAccountStepsAreLeftToTheHostsDetail(t *testing.T) {
	r := run("d1", accrual, "", wf.StatusCompleted, t0)
	steps := []wf.StepResult{{RunID: "d1", AccountID: "acct-1", StepName: "accrue", Status: wf.StatusCompleted}}
	s := &store{runs: []wf.RunRecord{r}, steps: map[string][]wf.StepResult{"d1": steps}}
	v := views(s)
	v.Detail = func(req *http.Request, got wf.RunRecord, gotSteps []wf.StepResult, href string) template.HTML {
		return template.HTML(fmt.Sprintf("<p>detail %s %d page=%s at %s</p>", got.ID, len(gotSteps), req.FormValue("page"), href))
	}
	p := serve(t, v, "/workflows/instances/d1?page=2")
	mustContain(t, p.body, "2026-03-01", "<p>detail d1 1 page=2 at /workflows/instances/d1</p>")
	if strings.Contains(p.body, "stages completed") {
		t.Fatalf("account steps are not stages:\n%s", p.body)
	}
}

func TestARunOfAnUnknownDefinitionListsItsStepsBare(t *testing.T) {
	r := wf.RunRecord{ID: "x1", WorkflowType: "retired", Key: "k", Status: wf.StatusFailed, StartedAt: t0, Error: "boom"}
	s := &store{runs: []wf.RunRecord{r}, steps: map[string][]wf.StepResult{"x1": {stage("x1", "old step", wf.StatusFailed, t0)}}}
	p := serve(t, views(s), "/workflows/instances/x1")
	mustContain(t, p.body, "retired", "old step", "boom")
}

func TestDefinitionsDescribeTheirStepsAndCountTheirInstances(t *testing.T) {
	s := &store{runs: []wf.RunRecord{
		run("r2", shipping, "v1.2.0", wf.StatusFailed, t0.Add(time.Hour)),
		run("r1", shipping, "v1.1.0", wf.StatusCompleted, t0),
	}}
	list := serve(t, views(s), "/workflows/definitions")
	if list.active != "definitions" {
		t.Fatalf("active %q", list.active)
	}
	mustContain(t, list.body, "release", "Get a version out.", `href="/workflows/definitions/ship"`, "build → tag → publish", "2 <span", "1 completed, 1 failed")

	one := serve(t, views(s), "/workflows/definitions/ship")
	mustContain(t, one.body, "Ship one version.", "one instance per version", "Compile it.", `href="/workflows/instances/r1"`, `href="/workflows/instances/r2"`,
		`href="/workflows/instances?workflow=ship"`)
	inOrder(t, one.body, `<strong>v1.2.0</strong>`, `<strong>v1.1.0</strong>`)
	if strings.Contains(serve(t, views(s), "/workflows/definitions/accrue").body, "v1.1.0") {
		t.Fatal("a definition lists only its own instances")
	}
	if p := serve(t, views(s), "/workflows/definitions/nope"); p.status != 404 {
		t.Fatalf("unknown definition: status %d", p.status)
	}
}

func TestDiagramsAreServedAndEmbeddedWhenShipped(t *testing.T) {
	s := &store{runs: []wf.RunRecord{run("r1", shipping, "v1.1.0", wf.StatusCompleted, t0)}}
	v := views(s)
	v.Diagrams = fstest.MapFS{"ship.svg": {Data: []byte("<svg>ship</svg>")}}

	svg := serve(t, v, "/workflows/definitions/ship/diagram.svg")
	if svg.status != 200 || svg.body != "<svg>ship</svg>" {
		t.Fatalf("svg: %d %q", svg.status, svg.body)
	}
	if p := serve(t, v, "/workflows/definitions/accrue/diagram.svg"); p.status != 404 {
		t.Fatalf("missing diagram: status %d", p.status)
	}
	mustContain(t, serve(t, v, "/workflows/definitions/ship").body, `src="/workflows/definitions/ship/diagram.svg"`)
	mustContain(t, serve(t, v, "/workflows/instances/r1").body, `src="/workflows/definitions/ship/diagram.svg"`)
	if strings.Contains(serve(t, v, "/workflows/definitions/accrue").body, ".svg") {
		t.Fatal("no diagram shipped for accrue; its page should not embed one")
	}
}

func TestAnInstanceIsServedAsD2(t *testing.T) {
	s := &store{runs: []wf.RunRecord{run("r1", shipping, "v1.1.0", wf.StatusFailed, t0)},
		steps: map[string][]wf.StepResult{"r1": {stage("r1", "build", wf.StatusCompleted, t0), stage("r1", "tag", wf.StatusFailed, t0.Add(time.Second))}}}
	p := serve(t, views(s), "/workflows/instances/r1/diagram.d2")
	if p.status != 200 {
		t.Fatalf("status %d", p.status)
	}
	mustContain(t, p.body, "build.class: completed", "tag.class: failed", "publish.class: not_run")
}

func TestTimesCarryTheDateToTheMillisecondAndDurationsRoundToTheirScale(t *testing.T) {
	long := run("l1", shipping, "v2.0.0", wf.StatusCompleted, t0)
	long.CompletedAt = t0.Add(3*time.Hour + 12*time.Minute + 5*time.Second + 123*time.Millisecond)
	short := run("s1", shipping, "v2.0.1", wf.StatusCompleted, t0)
	short.CompletedAt = t0.Add(1234567 * time.Nanosecond)
	s := &store{runs: []wf.RunRecord{long, short}}
	body := serve(t, views(s), "/workflows/instances").body
	mustContain(t, body, t0.Local().Format("2006-01-02 15:04:05.000"), "3h12m5s", "1.235ms")
	if strings.Contains(body, "3h12m5.123s") {
		t.Fatal("a run of hours should not be timed to the millisecond")
	}
}

func TestADefinitionWithoutAPurposeStillLists(t *testing.T) {
	bare := wf.Definition{Type: "bare", Description: "No purpose given.", InstanceKey: "date", Steps: []wf.Step{{Name: "only"}}}
	v := &view.Views{Store: &store{}, Definitions: []wf.Definition{bare}, Base: "/workflows/"}
	body := serve(t, v, "/workflows/definitions").body
	mustContain(t, body, `href="/workflows/definitions/bare"`, "No purpose given.")
	if strings.Contains(body, "<strong></strong>") {
		t.Fatalf("an empty purpose should not leave an empty heading:\n%s", body)
	}
}

func TestADefinitionsQueuePagesOnToTheInstancesPage(t *testing.T) {
	s := &store{}
	for i := range queueDefault + 1 {
		s.runs = append(s.runs, run(fmt.Sprintf("r%02d", i), shipping, fmt.Sprintf("v0.%d.0", i), wf.StatusCompleted, t0.Add(time.Duration(i)*time.Minute)))
	}
	body := serve(t, views(s), "/workflows/definitions/ship").body
	mustContain(t, body, `href="/workflows/instances?page=2&amp;workflow=ship">Next`)
}

// queueDefault is the view's default n.
const queueDefault = 50

// cancelling is the queue's views with a Cancel hook that records each id.
func cancelling(s *store, cancelled *[]string) *view.Views {
	v := views(s)
	v.Cancel = func(_ context.Context, id string) error {
		*cancelled = append(*cancelled, id)
		return nil
	}
	return v
}

func post(v *view.Views, path, referer string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, nil)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	http.StripPrefix("/workflows", v.Handler()).ServeHTTP(rec, req)
	return rec
}

func TestTheQueueOffersCancelOnlyWhenTheHostDoesAndOnlyForWorkNotRunning(t *testing.T) {
	if body := serve(t, views(queue()), "/workflows/instances").body; strings.Contains(body, "/cancel") {
		t.Fatal("no Cancel hook, no cancel button")
	}
	var cancelled []string
	body := serve(t, cancelling(queue(), &cancelled), "/workflows/instances").body
	mustContain(t, body, `action="/workflows/instances/f/cancel"`, `action="/workflows/instances/w/cancel"`)
	for _, id := range []string{"eod", "c-new", "c-old"} {
		if strings.Contains(body, `action="/workflows/instances/`+id+`/cancel"`) {
			t.Fatalf("%s is running or ended, so it should have no cancel button", id)
		}
	}
	mustContain(t, body, `<option value="cancelled">cancelled</option>`)
}

func TestCancelEndsAnInstanceAndReturnsToTheQueue(t *testing.T) {
	var cancelled []string
	v := cancelling(queue(), &cancelled)
	rec := post(v, "/workflows/instances/w/cancel", "http://example.com/workflows/instances?status=waiting")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "http://example.com/workflows/instances?status=waiting" {
		t.Fatalf("cancel should return to the page it came from: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if !slices.Equal(cancelled, []string{"w"}) {
		t.Fatalf("cancelled %v, want [w]", cancelled)
	}
	if rec := post(v, "/workflows/instances/f/cancel", "http://elsewhere.org/"); rec.Header().Get("Location") != "../../" {
		t.Fatalf("a referer from another site is not followed; back to the queue instead, got %q", rec.Header().Get("Location"))
	}
	if rec := post(v, "/workflows/instances/eod/cancel", ""); rec.Code != http.StatusConflict {
		t.Fatalf("a running instance cannot be cancelled, got %d", rec.Code)
	}
	if rec := post(v, "/workflows/instances/nope/cancel", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown instance: %d", rec.Code)
	}
	if rec := post(views(queue()), "/workflows/instances/w/cancel", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("no Cancel hook, no cancel route: %d", rec.Code)
	}
	if !slices.Equal(cancelled, []string{"w", "f"}) {
		t.Fatalf("cancelled %v, want [w f]", cancelled)
	}
}

func TestACancelledInstanceIsTaggedAsEnded(t *testing.T) {
	s := &store{runs: []wf.RunRecord{run("x", shipping, "v9", wf.StatusCancelled, t0)}}
	mustContain(t, serve(t, views(s), "/workflows/instances/x").body, `<span class="tag is-dark">cancelled</span>`)
}

var (
	batching = wf.Definition{Type: "batch", Purpose: wf.Purpose{Name: "scan", Description: "Scan a batch."}, Description: "One batch of prints.", InstanceKey: "batch id",
		Steps: []wf.Step{{Name: "describe"}, {Name: "loads", Description: "a load per load of the bed", Child: "load", Many: true}, {Name: "close"}}}
	loading = wf.Definition{Type: "load", Purpose: wf.Purpose{Name: "scan", Description: "Scan a batch."}, Description: "One load of the bed.", InstanceKey: "batch id/n",
		Steps: []wf.Step{{Name: "preview"}, {Name: "photos", Child: "photo"}}}
)

func TestAnInstanceListsItsChildrenAndLinksUpToItsParent(t *testing.T) {
	batch := run("b1", batching, "B-1", wf.StatusRunning, t0)
	first := run("l1", loading, "B-1/1", wf.StatusCompleted, t0.Add(time.Hour))
	first.ParentID = "b1"
	second := run("l2", loading, "B-1/2", wf.StatusPending, t0.Add(2*time.Hour))
	second.ParentID = "b1"
	second.StartedAt = time.Time{}
	v := &view.Views{Store: &store{runs: []wf.RunRecord{batch, first, second}}, Definitions: []wf.Definition{batching, loading}, Base: "/workflows/"}

	parent := serve(t, v, "/workflows/instances/b1").body
	mustContain(t, parent, "Children", `href="/workflows/instances/l1"`, `href="/workflows/instances/l2"`, "B-1/2", "pending")
	inOrder(t, parent[strings.Index(parent, "Children"):], "B-1/1", "B-1/2") // oldest first: the order they were started
	if strings.Contains(parent, "Parent") {
		t.Fatal("a top-level instance has no parent")
	}
	child := serve(t, v, "/workflows/instances/l1").body
	mustContain(t, child, "Parent", `href="/workflows/instances/b1">batch B-1</a>`)
	if strings.Contains(child, "Children") {
		t.Fatal("an instance without children has no Children section")
	}
}

func TestADefinitionPageNamesTheChildOfASubProcessStep(t *testing.T) {
	v := &view.Views{Store: &store{}, Definitions: []wf.Definition{batching, loading}, Base: "/workflows/"}
	body := serve(t, v, "/workflows/definitions/batch").body
	mustContain(t, body, "loads ⊞", `starts many <a href="/workflows/definitions/load"><code>load</code></a>`)
	mustContain(t, serve(t, v, "/workflows/definitions/load").body, `starts one <a href="/workflows/definitions/photo"><code>photo</code></a>`)
	mustContain(t, serve(t, v, "/workflows/definitions").body, "describe → loads ⊞ → close")
}

// A queue row says what ran as the workflow and the last two characters
// of its revision, ship-3f, linked to the definition; the revision in full
// is on the instance. A run of no known definition is named bare.
func TestTheQueueNamesWhatRanLinkedToItsDefinition(t *testing.T) {
	body := serve(t, views(queue()), "/workflows/instances").body
	rev := shipping.Revision().ID
	mustContain(t, body, "<th>What</th>", `<a href="/workflows/definitions/ship"><code>ship-`+rev[len(rev)-2:]+`</code></a>`)
	if strings.Contains(body, "<th>Revision</th>") || strings.Contains(body, rev) {
		t.Error("the queue should not show revisions in full")
	}
	r := wf.RunRecord{ID: "x1", WorkflowType: "retired", Key: "k", Status: wf.StatusFailed, StartedAt: t0, CreatedAt: t0}
	mustContain(t, serve(t, views(&store{runs: []wf.RunRecord{r}}), "/workflows/instances").body, `<td><code>retired</code></td>`)
}

// The host's own page for an instance, when it has one, is preferred
// wherever an instance is linked: the queue row and a child on its
// parent's page; the generic instance page stays a step away. Without
// one, the instance page is the link.
func TestTheHostsOwnPageIsPreferredWhenItHasOne(t *testing.T) {
	v := views(queue())
	v.Specific = func(r wf.RunRecord) string {
		if r.Key == "v1.2.0" || r.Key == "v1.3.0" {
			return "/releases/" + r.Key
		}
		return ""
	}
	body := serve(t, v, "/workflows/instances").body
	mustContain(t, body, `<a href="/releases/v1.3.0"><strong>v1.3.0</strong></a>`,
		`<a href="/releases/v1.2.0"><strong>v1.2.0</strong></a>`, `href="/workflows/instances/w">Instance</a>`,
		`<a href="/workflows/instances/f"><strong>v1.1.0</strong></a>`)
	if strings.Contains(body, `href="/workflows/instances/f">Instance`) {
		t.Error("an instance with no page of its own should not be linked twice")
	}

	s := queue()
	child := run("k", shipping, "v1.2.0", wf.StatusWaiting, t0)
	child.ParentID = "c-new"
	s.runs = append(s.runs, child)
	v.Store = s
	parent := serve(t, v, "/workflows/instances/c-new").body
	mustContain(t, parent, `<a href="/releases/v1.2.0"><strong>v1.2.0</strong></a>`, `href="/workflows/instances/k">Instance</a>`)
}
