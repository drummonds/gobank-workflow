// Package demoaccounts is a DayProcessor for demos: a book of identical
// easy-access savings accounts accruing simple daily interest. It has no
// place in production; gobank-products owns real product logic.
package demoaccounts

import (
	"context"
	"fmt"
	"math"
	"sync"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

type account struct {
	id      string
	balance int64 // minor units
}

// Processor implements wf.DayProcessor.
type Processor struct {
	mu       sync.Mutex
	rate     float64 // annual rate
	accounts []account
	failOn   map[wf.BusinessDate]bool
}

// New returns a Processor with n accounts, each opened with balance minor
// units at the given annual rate.
func New(n int, rate float64, balance int64) *Processor {
	p := &Processor{rate: rate, accounts: make([]account, n), failOn: map[wf.BusinessDate]bool{}}
	for i := range n {
		p.accounts[i] = account{id: fmt.Sprintf("acct-%04d", i), balance: balance}
	}
	return p
}

// FailOn makes the next ProcessDay for date return an error, so a demo
// can show a failed instance and its retry. The failure fires once.
func (p *Processor) FailOn(date wf.BusinessDate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failOn[date] = true
}

// ProcessDay accrues one day of interest on every account.
func (p *Processor) ProcessDay(_ context.Context, date wf.BusinessDate) (wf.DayResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.failOn[date] {
		delete(p.failOn, date)
		return wf.DayResult{}, fmt.Errorf("demoaccounts: rate feed unavailable for %s (injected)", date)
	}

	results := make([]wf.AccountResult, len(p.accounts))
	for i := range p.accounts {
		a := &p.accounts[i]
		interest := int64(math.Round(float64(a.balance) * p.rate / 365))
		results[i] = wf.AccountResult{
			AccountID:      a.id,
			OpeningBalance: a.balance,
			ClosingBalance: a.balance + interest,
			InterestAmount: interest,
			Exponent:       -2,
		}
		a.balance += interest
	}
	return wf.DayResult{Date: date, Accounts: results}, nil
}
