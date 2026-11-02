//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"math"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// book is the browser demo's DayProcessor: identical easy-access savings
// accounts accruing simple daily interest, held in memory. The demo times
// the engine over a thousand accounts and keeps nothing between loads, so
// it has no database; the demo app's demoaccounts keeps its book in one.
type book struct {
	rate     float64 // annual rate
	balances []int64 // minor units, by account index
}

func newBook(n int, rate float64, balance int64) *book {
	b := &book{rate: rate, balances: make([]int64, n)}
	for i := range b.balances {
		b.balances[i] = balance
	}
	return b
}

// ProcessDay accrues one day of interest on every account.
func (b *book) ProcessDay(_ context.Context, date wf.BusinessDate) (wf.DayResult, error) {
	results := make([]wf.AccountResult, len(b.balances))
	for i, opening := range b.balances {
		interest := int64(math.Round(float64(opening) * b.rate / 365))
		results[i] = wf.AccountResult{
			AccountID:      fmt.Sprintf("acct-%04d", i),
			OpeningBalance: opening,
			ClosingBalance: opening + interest,
			InterestAmount: interest,
			Exponent:       -2,
		}
		b.balances[i] += interest
	}
	return wf.DayResult{Date: date, Accounts: results}, nil
}
