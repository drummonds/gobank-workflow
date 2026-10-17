package view_test

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
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

func (s *store) ListRuns(_ context.Context, limit int) ([]wf.RunRecord, error) {
	return s.runs[:min(limit, len(s.runs))], nil
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

func TestInstancesAreListedNewestFirstUnderTheBase(t *testing.T) {
	s := &store{runs: []wf.RunRecord{
		run("r2", shipping, "v1.2.0", wf.StatusFailed, t0.Add(time.Hour)),
		run("r1", shipping, "v1.1.0", wf.StatusCompleted, t0),
	}}
	p := serve(t, views(s), "/workflows/instances")
	if p.status != 200 || p.active != "instances" {
		t.Fatalf("status %d active %q", p.status, p.active)
	}
	mustContain(t, p.body, `href="/workflows/instances/r2"`, `href="/workflows/definitions/ship"`, `class="tag is-danger"`)
	if strings.Index(p.body, "v1.2.0") > strings.Index(p.body, "v1.1.0") {
		t.Fatal("instances should be newest first, as the store lists them")
	}
	if root := serve(t, views(s), "/workflows/"); root.body != p.body {
		t.Fatal("the base itself should be the instances list")
	}
	mustContain(t, serve(t, views(&store{}), "/workflows/instances").body, "No instances yet")
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
	mustContain(t, one.body, "Ship one version.", "one instance per version", "Compile it.", `href="/workflows/instances/r1"`, `href="/workflows/instances/r2"`)
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

func TestTimesCarryTheDateAndDurationsRoundToTheirScale(t *testing.T) {
	long := run("l1", shipping, "v2.0.0", wf.StatusCompleted, t0)
	long.CompletedAt = t0.Add(3*time.Hour + 12*time.Minute + 5*time.Second + 123*time.Millisecond)
	short := run("s1", shipping, "v2.0.1", wf.StatusCompleted, t0)
	short.CompletedAt = t0.Add(1234567 * time.Nanosecond)
	s := &store{runs: []wf.RunRecord{long, short}}
	body := serve(t, views(s), "/workflows/instances").body
	mustContain(t, body, t0.Local().Format("2006-01-02 15:04:05"), "3h12m5s", "1.235ms")
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
