// Command demo is the gobank-workflow demo app: an About page, a Workflows
// menu (instances and definitions), a simulation driver that makes days
// happen, and a DB explorer over the live schema. The same mux serves the
// HTTP build and the browser (service worker) build.
package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	dbexplorer "git.bytestone.uk/hum3/go-dbexplorer"
	_ "git.bytestone.uk/hum3/go-postgres"
	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/diagram"
	"git.bytestone.uk/hum3/gobank-workflow/internal/demoaccounts"
	"git.bytestone.uk/hum3/gobank-workflow/sqlstore"
	"git.bytestone.uk/hum3/lofigui"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// version is set by the build (-ldflags "-X main.version=...").
var version = "dev"

// assetFS holds the definition diagrams the docs build renders with d2
// (task docs:d2); the app serves them as workflows/definitions/<type>.svg.
//
//go:embed assets
var assetFS embed.FS

// Options configures the simulated bank behind the app.
type Options struct {
	Accounts int             // savings accounts in the book (default 250)
	Rate     float64         // annual interest rate (default 3.5%)
	Balance  int64           // opening balance in minor units (default 10,000.00)
	Start    wf.BusinessDate // first business date (default 2026-01-01)
}

func (o Options) withDefaults() Options {
	if o.Accounts == 0 {
		o.Accounts = 250
	}
	if o.Rate == 0 {
		o.Rate = 0.035
	}
	if o.Balance == 0 {
		o.Balance = 1_000_000
	}
	if o.Start.IsZero() {
		o.Start = wf.NewBusinessDate(2026, time.January, 1)
	}
	return o
}

// App holds the store, the runner and the simulation driver's position.
type App struct {
	opts Options

	mu       sync.Mutex
	db       *sql.DB
	store    *sqlstore.Store
	proc     *demoaccounts.Processor
	runner   *wf.Runner
	current  wf.BusinessDate // next business date to process
	failNext bool
	shipped  [3]int // last version released through the pipeline
	log      []string

	page     *template.Template
	diagrams fs.FS // <type>.svg per definition; missing files mean no picture
}

// NewApp opens an in-memory pglike database at the current schema version.
func NewApp(opts Options) (*App, error) {
	assets, _ := fs.Sub(assetFS, "assets")
	a := &App{opts: opts.withDefaults(), page: template.Must(template.New("layout").Parse(layoutHTML)), diagrams: assets}
	if err := a.reset(); err != nil {
		return nil, err
	}
	return a, nil
}

// Close releases the database.
func (a *App) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

// reset discards the database and starts the simulation again.
func (a *App) reset() error {
	if a.db != nil {
		_ = a.db.Close()
	}
	d, err := sql.Open("pglike", ":memory:")
	if err != nil {
		return err
	}
	store, err := sqlstore.New(d)
	if err != nil {
		d.Close()
		return err
	}
	a.db = d
	a.store = store
	a.proc = demoaccounts.New(a.opts.Accounts, a.opts.Rate, a.opts.Balance)
	a.runner = wf.NewRunner(store, a.proc)
	a.current = a.opts.Start
	a.failNext = false
	a.shipped = [3]int{0, 1, 2}
	a.log = nil
	a.logf("simulation reset: %d accounts, %.2f%% p.a., starting %s", a.opts.Accounts, a.opts.Rate*100, a.current)
	return nil
}

// Advance processes up to n business days. A failed day stops the advance
// and stays current, so the next Advance retries it.
func (a *App) Advance(ctx context.Context, n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for range n {
		if a.failNext {
			a.proc.FailOn(a.current)
			a.failNext = false
		}
		t0 := time.Now()
		rec, err := a.runner.RunDaily(ctx, a.current)
		if err != nil {
			a.logf("%s failed: %v", a.current, err)
			return
		}
		if rec == nil {
			a.logf("%s already completed, skipped", a.current)
		} else {
			a.logf("%s completed in %s", a.current, time.Since(t0).Round(time.Microsecond))
		}
		a.current = a.current.Next()
	}
}

// NextVersion is the version the next release will ship: a patch bump on
// the last one shipped, or the same one again while it is failed.
func (a *App) NextVersion() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.nextVersion()
}

func (a *App) nextVersion() string {
	return fmt.Sprintf("v%d.%d.%d", a.shipped[0], a.shipped[1], a.shipped[2]+1)
}

// Release runs the release pipeline for the next version on the current
// business date. failStage, if set, names the stage that will fail this
// attempt. A failed release keeps its version, so the next Release
// resumes it from the failed stage.
func (a *App) Release(ctx context.Context, failStage string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	version := a.nextVersion()
	var stages []wf.Stage
	for _, step := range releaseDefinition().Steps {
		name := step.Name
		stages = append(stages, wf.Stage{Name: name, Run: func(context.Context) error {
			time.Sleep(2 * time.Millisecond) // a visible, if token, duration
			if name == failStage {
				return fmt.Errorf("%s failed (injected)", name)
			}
			return nil
		}})
	}
	rec, err := a.runner.RunPipeline(ctx, wf.WorkflowRelease, version, a.current, stages)
	switch {
	case err != nil:
		a.logf("release %s failed: %v", version, err)
	case rec == nil:
		a.logf("release %s already shipped", version)
	default:
		a.shipped[2]++
		a.logf("release %s shipped", version)
	}
}

func releaseDefinition() wf.Definition {
	for _, d := range wf.Definitions {
		if d.Type == wf.WorkflowRelease {
			return d
		}
	}
	panic("release definition missing")
}

// ToggleFailNext arms or disarms an injected failure for the next day.
func (a *App) ToggleFailNext() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failNext = !a.failNext
}

func (a *App) logf(format string, args ...any) {
	a.log = append(a.log, time.Now().Format("15:04:05")+"  "+fmt.Sprintf(format, args...))
}

func (a *App) explorer(base string) *dbexplorer.Explorer {
	return &dbexplorer.Explorer{
		DB:         a.db,
		BasePath:   strings.TrimSuffix(base, "/") + "/internal/explorer",
		UUIDLen:    8,
		TimeFormat: "2006-01-02 15:04:05",
	}
}

// Handler builds the mux. base is "/" for the server and the service-worker
// scope for the browser build; every in-app href is relative to it.
func (a *App) Handler(base string) http.Handler {
	mux := http.NewServeMux()
	redirectSim := func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base+"simulation", http.StatusSeeOther)
	}

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { a.render(w, base, "about", a.aboutHTML()) })
	mux.HandleFunc("GET /about", func(w http.ResponseWriter, r *http.Request) { a.render(w, base, "about", a.aboutHTML()) })

	mux.HandleFunc("GET /workflows/instances", func(w http.ResponseWriter, r *http.Request) {
		a.render(w, base, "instances", a.instancesHTML(r.Context()))
	})
	mux.HandleFunc("GET /workflows/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.FormValue("page"))
		html, ok := a.instanceHTML(r.Context(), r.PathValue("id"), page)
		if !ok {
			http.NotFound(w, r)
			return
		}
		a.render(w, base, "instances", html)
	})
	mux.HandleFunc("GET /workflows/definitions", func(w http.ResponseWriter, r *http.Request) {
		a.render(w, base, "definitions", a.definitionsHTML(r.Context()))
	})
	mux.HandleFunc("GET /workflows/definitions/{type}/diagram.svg", func(w http.ResponseWriter, r *http.Request) {
		svg, err := fs.ReadFile(a.diagrams, r.PathValue("type")+".svg")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(svg)
	})
	mux.HandleFunc("GET /workflows/instances/{id}/diagram.d2", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		run, err := a.store.GetRun(r.Context(), r.PathValue("id"))
		var steps []wf.StepResult
		if err == nil && run != nil {
			steps, _ = a.store.ListSteps(r.Context(), run.ID)
		}
		a.mu.Unlock()
		if err != nil || run == nil || definitionOf(run.WorkflowType) == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprint(w, diagram.Instance(*definitionOf(run.WorkflowType), *run, steps))
	})
	mux.HandleFunc("GET /workflows/definitions/{type}", func(w http.ResponseWriter, r *http.Request) {
		html, ok := a.definitionHTML(r.Context(), wf.WorkflowType(r.PathValue("type")))
		if !ok {
			http.NotFound(w, r)
			return
		}
		a.render(w, base, "definitions", html)
	})

	mux.HandleFunc("GET /simulation", func(w http.ResponseWriter, r *http.Request) {
		a.render(w, base, "simulation", a.simulationHTML(r.Context()))
	})
	mux.HandleFunc("POST /simulation/advance", func(w http.ResponseWriter, r *http.Request) {
		days, _ := strconv.Atoi(r.FormValue("days"))
		if days < 1 {
			days = 1
		}
		a.Advance(r.Context(), days)
		redirectSim(w, r)
	})
	mux.HandleFunc("POST /simulation/release", func(w http.ResponseWriter, r *http.Request) {
		a.Release(r.Context(), r.FormValue("fail_stage"))
		redirectSim(w, r)
	})
	mux.HandleFunc("POST /simulation/fail", func(w http.ResponseWriter, r *http.Request) {
		a.ToggleFailNext()
		redirectSim(w, r)
	})
	mux.HandleFunc("POST /simulation/reset", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		err := a.reset()
		a.mu.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		redirectSim(w, r)
	})

	explore := func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		ex := a.explorer(base)
		rest := strings.TrimPrefix(r.URL.RequestURI(), "/internal/explorer")
		html := ex.Render(ex.BasePath + rest)
		a.mu.Unlock()
		a.render(w, base, "explorer", html)
	}
	mux.HandleFunc("GET /internal/explorer", explore)
	mux.HandleFunc("GET /internal/explorer/", explore)

	mux.HandleFunc("GET /bulma.min.css", lofigui.ServeBulma)
	mux.HandleFunc("GET /favicon.ico", lofigui.ServeFavicon)
	return mux
}

func (a *App) render(w http.ResponseWriter, base, active, content string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = a.page.Execute(w, map[string]any{
		"base":    base,
		"active":  active,
		"content": template.HTML(content),
		"version": version,
	})
}
