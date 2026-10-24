//go:build js && wasm

package main

import (
	"context"
	"sync"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/internal/demoaccounts"
)

const numAccounts = 1000

// The browser demo runs the end of day and nothing else.
func init() {
	wf.Definitions = append(wf.Definitions, wf.DailyUpdate)
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

	proc := demoaccounts.New(numAccounts, 0.035, 1000000) // 10,000.00 at 3.5%
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
