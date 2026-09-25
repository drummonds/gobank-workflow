//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

const numAccounts = 1000

// simpleAccount is a minimal account model for the demo.
type simpleAccount struct {
	ID      string
	Balance int64   // minor units (pence)
	Rate    float64 // annual rate
}

// simpleProcessor implements wf.DayProcessor with basic daily interest.
type simpleProcessor struct {
	accounts []simpleAccount
}

func newSimpleProcessor(n int, rate float64, initialBalance int64) *simpleProcessor {
	p := &simpleProcessor{accounts: make([]simpleAccount, n)}
	for i := range n {
		p.accounts[i] = simpleAccount{
			ID:      fmt.Sprintf("acct-%04d", i),
			Balance: initialBalance,
			Rate:    rate,
		}
	}
	return p
}

func (p *simpleProcessor) ProcessDay(_ context.Context, date wf.BusinessDate) (wf.DayResult, error) {
	results := make([]wf.AccountResult, len(p.accounts))
	for i := range p.accounts {
		a := &p.accounts[i]
		interest := int64(float64(a.Balance) * a.Rate / 365)
		results[i] = wf.AccountResult{
			AccountID:      a.ID,
			OpeningBalance: a.Balance,
			ClosingBalance: a.Balance + interest,
			InterestAmount: interest,
			Exponent:       -2,
		}
		a.Balance += interest
	}
	return wf.DayResult{Date: date, Accounts: results}, nil
}

// DayTiming records how long each day's processing took.
type DayTiming struct {
	Date     wf.BusinessDate
	Duration time.Duration
	Accounts int
}

// WasmState holds the demo state.
type WasmState struct {
	mu      sync.Mutex
	runner  *wf.Runner
	store   *wf.MemStore
	timings []DayTiming
	start   wf.BusinessDate
	current wf.BusinessDate
	err     string
}

func NewWasmState() *WasmState {
	s := &WasmState{}
	s.reset()
	return s
}

func (s *WasmState) reset() {
	s.start = wf.NewBusinessDate(2026, time.January, 1)
	s.current = s.start

	proc := newSimpleProcessor(numAccounts, 0.035, 1000000) // 10,000.00 at 3.5%
	s.store = wf.NewMemStore()
	s.runner = wf.NewRunner(s.store, proc)
	s.timings = nil
	s.err = ""
}

func (s *WasmState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reset()
}

func (s *WasmState) AdvanceDays(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	for range n {
		t0 := time.Now()
		rec, err := s.runner.RunDaily(ctx, s.current)
		dur := time.Since(t0)

		if err != nil {
			s.err = err.Error()
			return
		}

		accounts := 0
		if rec != nil {
			steps, _ := s.store.ListSteps(ctx, rec.ID)
			accounts = len(steps)
		}

		s.timings = append(s.timings, DayTiming{
			Date:     s.current,
			Duration: dur,
			Accounts: accounts,
		})
		s.current = s.current.Next()
	}
	s.err = ""
}
