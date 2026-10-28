package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func newTestServer(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	app, err := NewApp(Options{Accounts: 3})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	srv := httptest.NewServer(app.Handler("/"))
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return srv, client
}

func get(t *testing.T, c *http.Client, u string) (int, string) {
	t.Helper()
	res, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func post(t *testing.T, c *http.Client, u string, form url.Values) *http.Response {
	t.Helper()
	res, err := c.PostForm(u, form)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res
}

func mustContain(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Fatalf("body missing %q\n---\n%s", w, body)
		}
	}
}

func TestAboutIsTheHomePageAndCarriesTheMenu(t *testing.T) {
	srv, c := newTestServer(t)
	for _, path := range []string{"/", "/about"} {
		code, body := get(t, c, srv.URL+path)
		if code != 200 {
			t.Fatalf("%s: status %d", path, code)
		}
		mustContain(t, body, "gobank-workflow", `href="workflows/instances"`, `href="workflows/definitions"`, `href="simulation"`, `href="internal/explorer"`, `href="about"`)
	}
}

func TestInstancesStartEmpty(t *testing.T) {
	srv, c := newTestServer(t)
	_, body := get(t, c, srv.URL+"/workflows/instances")
	mustContain(t, body, "No instances yet")
}

func TestAdvancingTheSimulationCreatesTimeOrderedInstances(t *testing.T) {
	srv, c := newTestServer(t)
	res := post(t, c, srv.URL+"/simulation/advance", url.Values{"days": {"3"}})
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/simulation" {
		t.Fatalf("advance: status %d location %q", res.StatusCode, res.Header.Get("Location"))
	}
	_, body := get(t, c, srv.URL+"/workflows/instances")
	// Most recent first.
	i1, i2, i3 := strings.Index(body, "2026-01-03"), strings.Index(body, "2026-01-02"), strings.Index(body, "2026-01-01")
	if !(i1 > 0 && i1 < i2 && i2 < i3) {
		t.Fatalf("instances not most-recent-first: %d %d %d", i1, i2, i3)
	}
	if n := strings.Count(body, `</strong></a></td>`); n != 3 {
		t.Fatalf("want 3 instance rows, got %d", n)
	}
	_, sim := get(t, c, srv.URL+"/simulation")
	mustContain(t, sim, "2026-01-04") // next business date to process
}

func TestInstanceDetailShowsStateAndSteps(t *testing.T) {
	srv, c := newTestServer(t)
	post(t, c, srv.URL+"/simulation/advance", url.Values{"days": {"1"}})
	_, list := get(t, c, srv.URL+"/workflows/instances")
	m := regexp.MustCompile(`href="(workflows/instances/[^"]+)"`).FindStringSubmatch(list)
	if m == nil {
		t.Fatal("no instance link")
	}
	code, body := get(t, c, srv.URL+"/"+m[1])
	if code != 200 {
		t.Fatalf("detail status %d", code)
	}
	mustContain(t, body, "daily_update", "2026-01-01", "completed", "acct-0000", "acct-0002", "internal/explorer/workflow_steps?filter=run_id")
	if code, _ := get(t, c, srv.URL+"/workflows/instances/nope"); code != 404 {
		t.Fatalf("unknown instance: status %d, want 404", code)
	}
}

func TestDefinitionsExplorerShowsCountsAndInstances(t *testing.T) {
	srv, c := newTestServer(t)
	post(t, c, srv.URL+"/simulation/advance", url.Values{"days": {"2"}})
	_, body := get(t, c, srv.URL+"/workflows/definitions")
	mustContain(t, body, "daily_update", `href="workflows/definitions/daily_update"`)
	code, detail := get(t, c, srv.URL+"/workflows/definitions/daily_update")
	if code != 200 {
		t.Fatalf("definition status %d", code)
	}
	mustContain(t, detail, "End-of-day processing", "2026-01-01", "2026-01-02")
	if code, _ := get(t, c, srv.URL+"/workflows/definitions/nope"); code != 404 {
		t.Fatalf("unknown definition: status %d, want 404", code)
	}
}

func TestInjectedFailureThenRetryLeavesOneInstanceForTheDate(t *testing.T) {
	srv, c := newTestServer(t)
	post(t, c, srv.URL+"/simulation/advance", url.Values{"days": {"1"}})
	post(t, c, srv.URL+"/simulation/fail", nil)
	post(t, c, srv.URL+"/simulation/advance", url.Values{"days": {"1"}})
	_, list := get(t, c, srv.URL+"/workflows/instances")
	mustContain(t, list, "failed", "2026-01-02")
	_, sim := get(t, c, srv.URL+"/simulation")
	mustContain(t, sim, "2026-01-02") // the failed date is still the one to process
	post(t, c, srv.URL+"/simulation/advance", url.Values{"days": {"1"}})
	_, list = get(t, c, srv.URL+"/workflows/instances")
	if strings.Contains(list, `class="tag is-danger"`) {
		t.Fatalf("retry should have replaced the failed instance:\n%s", list)
	}
	if n := strings.Count(list, "<strong>2026-01-02</strong>"); n != 1 {
		t.Fatalf("want one instance for 2026-01-02, got %d", n)
	}
}

func TestExplorerShowsTheSchema(t *testing.T) {
	srv, c := newTestServer(t)
	post(t, c, srv.URL+"/simulation/advance", url.Values{"days": {"1"}})
	_, index := get(t, c, srv.URL+"/internal/explorer")
	mustContain(t, index, "workflow_runs", "workflow_steps", "schema_migrations")
	_, table := get(t, c, srv.URL+"/internal/explorer/workflow_runs")
	mustContain(t, table, "daily_update")
}

func TestResetForgetsAllInstances(t *testing.T) {
	srv, c := newTestServer(t)
	post(t, c, srv.URL+"/simulation/advance", url.Values{"days": {"2"}})
	post(t, c, srv.URL+"/simulation/reset", nil)
	_, body := get(t, c, srv.URL+"/workflows/instances")
	mustContain(t, body, "No instances yet")
}

func TestDefinitionPagesDescribeStepsNotJustInstances(t *testing.T) {
	srv, c := newTestServer(t)
	_, list := get(t, c, srv.URL+"/workflows/definitions")
	mustContain(t, list, "release", `href="workflows/definitions/release"`, "Steps", "Instance key")
	_, detail := get(t, c, srv.URL+"/workflows/definitions/release")
	mustContain(t, detail, "check", "verify install", "deploy docs", "version", `href="https://git.bytestone.uk/hum3/task-plus/src/branch/main/internal/changelog/changelog.go"`)
	_, daily := get(t, c, srv.URL+"/workflows/definitions/daily_update")
	mustContain(t, daily, "end_of_day", "business date")
}

func TestReleasingThroughThePipelineCreatesAKeyedInstanceWithStageSteps(t *testing.T) {
	srv, c := newTestServer(t)
	res := post(t, c, srv.URL+"/simulation/release", nil)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("release: status %d", res.StatusCode)
	}
	_, list := get(t, c, srv.URL+"/workflows/instances")
	mustContain(t, list, "release", "v0.1.3", "completed")
	m := regexp.MustCompile(`href="(workflows/instances/[^"]+)"`).FindStringSubmatch(list)
	_, detail := get(t, c, srv.URL+"/"+m[1])
	mustContain(t, detail, "v0.1.3", "check", "push mirror", "deploy docs")
	if strings.Contains(detail, "acct-") {
		t.Fatal("a release instance has stage steps, not account steps")
	}
	_, sim := get(t, c, srv.URL+"/simulation")
	mustContain(t, sim, "v0.1.4") // next version to ship
}

func TestFailedStageThenRetryResumesTheSameReleaseInstance(t *testing.T) {
	srv, c := newTestServer(t)
	post(t, c, srv.URL+"/simulation/release", url.Values{"fail_stage": {"push mirror"}})
	_, list := get(t, c, srv.URL+"/workflows/instances")
	mustContain(t, list, "v0.1.3", "failed", "push mirror")
	m := regexp.MustCompile(`href="(workflows/instances/[^"]+)"`).FindStringSubmatch(list)
	_, detail := get(t, c, srv.URL+"/"+m[1])
	mustContain(t, detail, "5 of 8 stages completed", "not run")
	post(t, c, srv.URL+"/simulation/release", nil) // retries v0.1.3
	_, detail = get(t, c, srv.URL+"/"+m[1])
	mustContain(t, detail, "8 of 8 stages completed")
	if strings.Contains(detail, "not run") || strings.Contains(detail, ">failed<") {
		t.Fatalf("after the retry every stage should be completed:\n%s", detail)
	}
	_, list = get(t, c, srv.URL+"/workflows/instances")
	if n := strings.Count(list, "<strong>v0.1.3</strong>"); n != 1 {
		t.Fatalf("want one instance for v0.1.3, got %d", n)
	}
	_, defs := get(t, c, srv.URL+"/workflows/definitions/release")
	mustContain(t, defs, "v0.1.3")
}

func TestDefinitionDiagramIsServedWhenShippedAndInstanceD2IsGenerated(t *testing.T) {
	app, err := NewApp(Options{Accounts: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	app.diagrams = fstest.MapFS{"release.svg": {Data: []byte("<svg>release</svg>")}}
	srv := httptest.NewServer(app.Handler("/"))
	defer srv.Close()
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	code, body := get(t, c, srv.URL+"/workflows/definitions/release/diagram.svg")
	if code != 200 || body != "<svg>release</svg>" {
		t.Fatalf("svg: %d %q", code, body)
	}
	if code, _ := get(t, c, srv.URL+"/workflows/definitions/daily_update/diagram.svg"); code != 404 {
		t.Fatalf("missing diagram should 404, got %d", code)
	}
	_, page := get(t, c, srv.URL+"/workflows/definitions/release")
	mustContain(t, page, `src="workflows/definitions/release/diagram.svg"`)
	_, daily := get(t, c, srv.URL+"/workflows/definitions/daily_update")
	if strings.Contains(daily, ".svg") {
		t.Fatal("no diagram shipped for daily_update, page should not embed one")
	}

	post(t, c, srv.URL+"/simulation/release", url.Values{"fail_stage": {"tag"}})
	_, list := get(t, c, srv.URL+"/workflows/instances")
	id := regexp.MustCompile(`workflows/instances/([^"/]+)"`).FindStringSubmatch(list)[1]
	code, d2 := get(t, c, srv.URL+"/workflows/instances/"+id+"/diagram.d2")
	if code != 200 {
		t.Fatalf("d2: status %d", code)
	}
	mustContain(t, d2, "tag.class: failed", "check.class: completed", "deploy_docs.class: not_run", "v0.1.3")
	_, detail := get(t, c, srv.URL+"/workflows/instances/"+id)
	mustContain(t, detail, `href="workflows/instances/`+id+`/diagram.d2"`, `src="workflows/definitions/release/diagram.svg"`)
}

func TestDefinitionsShowTheirPurpose(t *testing.T) {
	srv, c := newTestServer(t)
	_, body := get(t, c, srv.URL+"/workflows/definitions")
	mustContain(t, body, "end of day", "Close the bank&#39;s books", "release", "Ship a new version")
	_, detail := get(t, c, srv.URL+"/workflows/definitions/release")
	mustContain(t, detail, "<strong>Purpose:</strong> release")
}

func TestInstanceStepsArePagedInAccountOrder(t *testing.T) {
	app, err := NewApp(Options{Accounts: 120})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	srv := httptest.NewServer(app.Handler("/"))
	t.Cleanup(srv.Close)
	c := srv.Client()
	post(t, c, srv.URL+"/simulation/advance", url.Values{"days": {"1"}})
	_, list := get(t, c, srv.URL+"/workflows/instances")
	path := regexp.MustCompile(`href="(workflows/instances/[^"]+)"`).FindStringSubmatch(list)[1]

	_, first := get(t, c, srv.URL+"/"+path)
	mustContain(t, first, "acct-0000", "acct-0049", "steps 1–50 of 120", `href="`+path+`?page=2"`)
	if strings.Contains(first, "acct-0050") {
		t.Fatal("page 1 shows a page-2 step")
	}

	_, last := get(t, c, srv.URL+"/"+path+"?page=3")
	mustContain(t, last, "acct-0100", "acct-0119", "steps 101–120 of 120", `href="`+path+`?page=2"`)
	if strings.Contains(last, "acct-0099") || strings.Contains(last, "?page=4") {
		t.Fatal("page 3 shows a page-2 step or a next link")
	}

	// Out of range clamps to the last page rather than showing nothing.
	_, beyond := get(t, c, srv.URL+"/"+path+"?page=9")
	mustContain(t, beyond, "steps 101–120 of 120")
}
