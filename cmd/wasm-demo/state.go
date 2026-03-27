//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	luca "codeberg.org/hum3/go-luca"
	gbp "codeberg.org/hum3/gobank-products"
	wf "codeberg.org/hum3/gobank-workflow"
)

const numAccounts = 1000

// DayTiming records how long each day's processing took.
type DayTiming struct {
	Date     wf.BusinessDate
	Duration time.Duration
	Accounts int
}

// WasmState holds the in-memory simulation and workflow runner.
type WasmState struct {
	mu      sync.Mutex
	runner  *wf.Runner
	store   *wf.MemStore
	sim     *gbp.Simulation
	clock   *gbp.SimClock
	ledger  luca.Ledger
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
	s.ledger = luca.NewMemLedger()
	s.start = wf.NewBusinessDate(2026, time.January, 1)
	s.current = s.start

	s.clock = gbp.NewSimClock(s.start.Time())
	sim, err := gbp.NewSimulation(s.ledger, s.clock)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.sim = sim
	s.sim.RegisterProduct(gbp.EasyAccess())

	// Create cash account and open savings accounts.
	cash, err := s.ledger.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err != nil {
		s.err = err.Error()
		return
	}

	for i := range numAccounts {
		path := fmt.Sprintf("Liability:Savings:cust%04d", i)
		ma, err := s.sim.OpenAccount("easy-access", path, "GBP", -2, map[string]string{
			"annual_rate": "0.035",
		})
		if err != nil {
			s.err = err.Error()
			return
		}
		// Deposit 10,000.00 into each account.
		if err := s.sim.Deposit(ma.Account.ID, 1000000, cash.ID, luca.CodeBookTransfer); err != nil {
			s.err = err.Error()
			return
		}
	}

	s.store = wf.NewMemStore()
	s.runner = wf.NewRunner(s.store, s.sim, s.clock)
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

func (s *WasmState) Timings() []DayTiming {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]DayTiming, len(s.timings))
	copy(out, s.timings)
	return out
}
