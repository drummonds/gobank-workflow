// Package view is the workflow pages an app shows over a store: the
// instances as a work queue, each instance laid over the steps it ran, the definitions and
// each definition with its own instances. It renders HTML fragments
// (Bulma classes) and serves its own routes; the host wraps each page in
// its own layout through Frame, and adds what only it knows about a
// workflow type through Detail.
package view

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/diagram"
)

// Reader is the read side of a wf.Store the pages need.
type Reader interface {
	ListRuns(ctx context.Context, f wf.RunFilter) ([]wf.RunRecord, error)
	CountRuns(ctx context.Context) (map[wf.WorkflowType]wf.RunCounts, error)
	GetRun(ctx context.Context, id string) (*wf.RunRecord, error)
	ListSteps(ctx context.Context, runID string) ([]wf.StepResult, error)
}

// Page is one rendered page as the host's Frame receives it.
type Page struct {
	Active string        // "instances" or "definitions", for the host's menu
	HTML   template.HTML // the page content
}

// Views serves the workflow pages. Mount Handler under Base with the
// prefix stripped, e.g. http.StripPrefix("/workflows", v.Handler()) for
// Base "/workflows/". Base itself is the instances page: the queue of
// work in hand, filtered by its GET form.
type Views struct {
	Store       Reader
	Definitions []wf.Definition
	// Base prefixes every link the pages make; it ends in "/". It may be
	// relative when the host sets <base href>.
	Base string
	// Diagrams holds a picture of each definition as <type>.svg; nil, or a
	// missing file, means no picture.
	Diagrams fs.FS
	// Detail, if set, adds content for one instance below the steps: what
	// the host alone knows about its workflow type. href is the instance's
	// own link, for paging and the like.
	Detail func(r *http.Request, run wf.RunRecord, steps []wf.StepResult, href string) template.HTML
	// Cancel, if set, ends an instance cancelled (wf.Store's CancelRun):
	// the queue then offers it on every instance in hand but running.
	Cancel func(ctx context.Context, id string) error
	// Frame, if set, writes a page inside the host's layout; otherwise the
	// bare fragment is written.
	Frame func(w http.ResponseWriter, r *http.Request, p Page)
}

// Handler serves the pages, the definition pictures and each instance as d2.
func (v *Views) Handler() http.Handler {
	mux := http.NewServeMux()
	instances := func(w http.ResponseWriter, r *http.Request) {
		v.frame(w, r, Page{Active: "instances", HTML: template.HTML(v.instancesHTML(r))})
	}
	mux.HandleFunc("GET /{$}", instances)
	mux.HandleFunc("GET /instances", instances)
	mux.HandleFunc("GET /instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		html, ok := v.instanceHTML(r, r.PathValue("id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		v.frame(w, r, Page{Active: "instances", HTML: html})
	})
	if v.Cancel != nil {
		mux.HandleFunc("POST /instances/{id}/cancel", v.cancel)
	}
	mux.HandleFunc("GET /instances/{id}/diagram.d2", func(w http.ResponseWriter, r *http.Request) {
		run, steps, ok := v.instance(r.Context(), r.PathValue("id"))
		def := v.definition(run.WorkflowType)
		if !ok || def == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprint(w, diagram.Instance(*def, run, latest(steps)))
	})
	mux.HandleFunc("GET /definitions", func(w http.ResponseWriter, r *http.Request) {
		v.frame(w, r, Page{Active: "definitions", HTML: v.definitionsHTML(r.Context())})
	})
	mux.HandleFunc("GET /definitions/{type}", func(w http.ResponseWriter, r *http.Request) {
		html, ok := v.definitionHTML(r.Context(), wf.WorkflowType(r.PathValue("type")))
		if !ok {
			http.NotFound(w, r)
			return
		}
		v.frame(w, r, Page{Active: "definitions", HTML: html})
	})
	mux.HandleFunc("GET /definitions/{type}/diagram.svg", func(w http.ResponseWriter, r *http.Request) {
		svg, ok := v.diagram(wf.WorkflowType(r.PathValue("type")))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(svg)
	})
	return mux
}

// cancel ends the instance and returns to the page the operator was on,
// or to the queue when that page is not this site's.
func (v *Views) cancel(w http.ResponseWriter, r *http.Request) {
	run, err := v.Store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil || run == nil {
		http.NotFound(w, r)
		return
	}
	if !v.cancellable(*run) {
		http.Error(w, fmt.Sprintf("a %s instance cannot be cancelled", run.Status), http.StatusConflict)
		return
	}
	if err := v.Cancel(r.Context(), run.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Relative to /…/instances/{id}/cancel, so the browser resolves it
	// under wherever the views are mounted; http.Redirect would resolve it
	// against the path with the prefix stripped.
	back := "../../"
	if ref, err := url.Parse(r.Referer()); err == nil && ref.Host == r.Host {
		back = ref.String()
	}
	w.Header().Set("Location", back)
	w.WriteHeader(http.StatusSeeOther)
}

// cancellable reports whether the queue offers to cancel run: the host
// can, and the run is in hand but not running, since a runner is on it.
func (v *Views) cancellable(run wf.RunRecord) bool {
	return v.Cancel != nil && run.Status.InHand() && run.Status != wf.StatusRunning
}

// InstanceHref is the link to one instance's page.
func (v *Views) InstanceHref(id string) string { return v.Base + "instances/" + id }

// DefinitionHref is the link to one definition's page.
func (v *Views) DefinitionHref(t wf.WorkflowType) string { return v.Base + "definitions/" + string(t) }

func (v *Views) frame(w http.ResponseWriter, r *http.Request, p Page) {
	if v.Frame != nil {
		v.Frame(w, r, p)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, p.HTML)
}

func (v *Views) definition(t wf.WorkflowType) *wf.Definition {
	for i := range v.Definitions {
		if v.Definitions[i].Type == t {
			return &v.Definitions[i]
		}
	}
	return nil
}

func (v *Views) diagram(t wf.WorkflowType) ([]byte, bool) {
	if v.Diagrams == nil {
		return nil, false
	}
	svg, err := fs.ReadFile(v.Diagrams, string(t)+".svg")
	return svg, err == nil
}

// instance is the run id names with its step records; false when there is none.
func (v *Views) instance(ctx context.Context, id string) (wf.RunRecord, []wf.StepResult, bool) {
	run, err := v.Store.GetRun(ctx, id)
	if err != nil || run == nil {
		return wf.RunRecord{}, nil, false
	}
	steps, _ := v.Store.ListSteps(ctx, id)
	return *run, steps, true
}

// latest keeps, for each step name, its most recent record by time: a
// retried stage reads as its retry, whatever order the store listed them in.
func latest(steps []wf.StepResult) []wf.StepResult {
	at := map[string]int{}
	var out []wf.StepResult
	for _, s := range steps {
		i, seen := at[s.StepName]
		switch {
		case !seen:
			at[s.StepName] = len(out)
			out = append(out, s)
		case s.CreatedAt.After(out[i].CreatedAt):
			out[i] = s
		}
	}
	return out
}
