package main

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"slices"
	"strconv"
	"strings"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/cmd/demo/definitions"
)

// lockedStore is the store as the workflow views read it: under the app's
// lock, since the simulation writes and reset replaces it.
type lockedStore struct{ a *App }

func (s lockedStore) ListRuns(ctx context.Context, f wf.RunFilter) ([]wf.RunRecord, error) {
	s.a.mu.Lock()
	defer s.a.mu.Unlock()
	return s.a.store.ListRuns(ctx, f)
}

func (s lockedStore) CountMatching(ctx context.Context, f wf.RunFilter) (int, error) {
	s.a.mu.Lock()
	defer s.a.mu.Unlock()
	return s.a.store.CountMatching(ctx, f)
}

func (s lockedStore) CountRuns(ctx context.Context) (map[wf.WorkflowType]wf.RunCounts, error) {
	s.a.mu.Lock()
	defer s.a.mu.Unlock()
	return s.a.store.CountRuns(ctx)
}

func (s lockedStore) GetRun(ctx context.Context, id string) (*wf.RunRecord, error) {
	s.a.mu.Lock()
	defer s.a.mu.Unlock()
	return s.a.store.GetRun(ctx, id)
}

func (s lockedStore) ListSteps(ctx context.Context, runID string) ([]wf.StepResult, error) {
	s.a.mu.Lock()
	defer s.a.mu.Unlock()
	return s.a.store.ListSteps(ctx, runID)
}

// stepsPerPage is how many account steps one page of an instance shows.
const stepsPerPage = 50

// detail is what the demo adds to an instance page: what a batch or a
// payment is about, the accounts an end-of-day run processed, a page at
// a time (?page=N, clamped), and the link to its step rows in the DB
// explorer.
func (a *App) detail(r *http.Request, run wf.RunRecord, steps []wf.StepResult, href string) template.HTML {
	var b strings.Builder
	switch run.WorkflowType {
	case definitions.BatchType:
		var total int64
		for _, p := range a.paymentsOf(run.Key) {
			total += p.Amount
		}
		fmt.Fprintf(&b, `<p>%d payments, %s in total.</p>`, paymentsPerBatch, money(total, -2))
	case definitions.PaymentType:
		if p, ok := a.paymentOf(run.Key); ok {
			fmt.Fprintf(&b, `<p>Payment %d of %s: %s from <code>%s</code>.</p>`, p.N, html.EscapeString(p.Batch), money(p.Amount, -2), html.EscapeString(p.Account))
		}
	}
	var accounts []wf.StepResult
	for _, s := range steps {
		if s.AccountID != "" {
			accounts = append(accounts, s)
		}
	}
	if len(accounts) > 0 {
		var interest int64
		for _, s := range accounts {
			interest += s.InterestAmount
		}
		fmt.Fprintf(&b, `<p>%d accounts processed, %s interest accrued.</p>`, len(accounts), money(interest, -2))
		// Account order, not record order: it is what a reader scans by,
		// and it does not depend on the clock's resolution.
		slices.SortFunc(accounts, func(x, y wf.StepResult) int { return strings.Compare(x.AccountID, y.AccountID) })
		page, _ := strconv.Atoi(r.FormValue("page"))
		pages := (len(accounts) + stepsPerPage - 1) / stepsPerPage
		page = min(max(page, 1), pages)
		lo, hi := (page-1)*stepsPerPage, min(page*stepsPerPage, len(accounts))
		fmt.Fprintf(&b, `<h2 class="subtitle">Accounts <span class="has-text-grey">(steps %d–%d of %d)</span></h2>`, lo+1, hi, len(accounts))
		b.WriteString(`<div class="table-container"><table class="table is-striped is-fullwidth is-narrow">`)
		b.WriteString(`<thead><tr><th>Account</th><th>Step</th><th class="has-text-right">Opening</th><th class="has-text-right">Interest</th><th class="has-text-right">Closing</th></tr></thead><tbody>`)
		for _, s := range accounts[lo:hi] {
			fmt.Fprintf(&b, `<tr><td><code>%s</code></td><td>%s</td><td class="has-text-right">%s</td><td class="has-text-right">%s</td><td class="has-text-right">%s</td></tr>`,
				html.EscapeString(s.AccountID), html.EscapeString(s.StepName), money(s.OpeningBalance, s.Exponent), money(s.InterestAmount, s.Exponent), money(s.ClosingBalance, s.Exponent))
		}
		b.WriteString(`</tbody></table></div>`)
		if pages > 1 {
			b.WriteString(`<nav class="pagination is-small" role="navigation" aria-label="account pages">`)
			if page > 1 {
				fmt.Fprintf(&b, `<a class="pagination-previous" href="%s?page=%d">Previous</a>`, href, page-1)
			}
			if page < pages {
				fmt.Fprintf(&b, `<a class="pagination-next" href="%s?page=%d">Next</a>`, href, page+1)
			}
			b.WriteString(`<ul class="pagination-list">`)
			for p := 1; p <= pages; p++ {
				if p == page {
					fmt.Fprintf(&b, `<li><a class="pagination-link is-current" aria-current="page">%d</a></li>`, p)
				} else {
					fmt.Fprintf(&b, `<li><a class="pagination-link" href="%s?page=%d">%d</a></li>`, href, p, p)
				}
			}
			b.WriteString(`</ul></nav>`)
		}
	}
	fmt.Fprintf(&b, `<p><a href="internal/explorer/workflow_steps?filter=run_id&amp;value=%s">All steps in the DB explorer</a>.</p>`, html.EscapeString(run.ID))
	return template.HTML(b.String())
}
