package demoaccounts

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	wf "git.bytestone.uk/hum3/gobank-workflow"
)

var ctx = context.Background()

// book is a Processor over a fresh in-memory database.
func book(t *testing.T, n int, rate float64, balance int64) (*Processor, *sql.DB) {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	p, err := Open(ctx, db, n, rate, balance)
	if err != nil {
		t.Fatal(err)
	}
	return p, db
}

func balance(t *testing.T, p *Processor, id string) int64 {
	t.Helper()
	got, err := p.Balance(ctx, id)
	if err != nil {
		t.Fatalf("Balance(%s): %v", id, err)
	}
	return got
}

func TestProcessDayAccruesDailyInterestOnEveryAccount(t *testing.T) {
	p, _ := book(t, 3, 0.0365, 1_000_00) // 1,000.00 at 3.65% → 10p a day
	res, err := p.ProcessDay(ctx, wf.NewBusinessDate(2026, time.January, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Accounts) != 3 {
		t.Fatalf("got %d account results, want 3", len(res.Accounts))
	}
	for _, a := range res.Accounts {
		if a.OpeningBalance != 1_000_00 || a.InterestAmount != 10 || a.ClosingBalance != 1_000_10 || a.Exponent != -2 {
			t.Fatalf("unexpected result %+v", a)
		}
	}
	// Balances carry forward.
	res, _ = p.ProcessDay(ctx, wf.NewBusinessDate(2026, time.January, 2))
	if res.Accounts[0].OpeningBalance != 1_000_10 {
		t.Fatalf("day 2 opening = %d, want 100010", res.Accounts[0].OpeningBalance)
	}
}

func TestProcessDayFailsOnTheConfiguredDate(t *testing.T) {
	bad := wf.NewBusinessDate(2026, time.January, 3)
	p, _ := book(t, 1, 0.0365, 1_000_00)
	p.FailOn(bad)
	if _, err := p.ProcessDay(ctx, wf.NewBusinessDate(2026, time.January, 2)); err != nil {
		t.Fatal("day before should succeed")
	}
	if _, err := p.ProcessDay(ctx, bad); err == nil {
		t.Fatal("configured date should fail")
	}
	if _, err := p.ProcessDay(ctx, bad); err != nil {
		t.Fatal("a retry of the failed date should succeed")
	}
}

func TestDebitTakesTheAmountFromTheAccount(t *testing.T) {
	p, _ := book(t, 2, 0.035, 1_000_000)
	if err := p.Debit(ctx, "acct-0001", 30_000); err != nil {
		t.Fatal(err)
	}
	if got := balance(t, p, "acct-0001"); got != 970_000 {
		t.Fatalf("balance = %d, want 970000", got)
	}
	if got := balance(t, p, "acct-0000"); got != 1_000_000 {
		t.Fatalf("other account touched: %d", got)
	}
}

func TestDebitRefusesMoreThanTheBalanceOrAnUnknownAccount(t *testing.T) {
	p, _ := book(t, 1, 0.035, 1_000)
	if err := p.Debit(ctx, "acct-0000", 1_001); err == nil {
		t.Fatal("overdrawing should fail")
	}
	if got := balance(t, p, "acct-0000"); got != 1_000 {
		t.Fatalf("a refused debit changed the balance: %d", got)
	}
	if err := p.Debit(ctx, "acct-0009", 1); !errors.Is(err, ErrNoAccount) {
		t.Fatalf("unknown account: %v, want ErrNoAccount", err)
	}
	if _, err := p.Balance(ctx, "acct-0009"); !errors.Is(err, ErrNoAccount) {
		t.Fatalf("unknown account reported: %v, want ErrNoAccount", err)
	}
}

// The book is a table, not a slice: a second Processor opened over the
// same database sees the balances the first one left, and does not open
// the accounts again.
func TestASecondOpenOverTheSameDatabaseSeesTheBalancesTheFirstLeft(t *testing.T) {
	first, db := book(t, 2, 0.0365, 1_000_00)
	if _, err := first.ProcessDay(ctx, wf.NewBusinessDate(2026, time.January, 1)); err != nil {
		t.Fatal(err)
	}
	if err := first.Debit(ctx, "acct-0001", 500_00); err != nil {
		t.Fatal(err)
	}

	second, err := Open(ctx, db, 2, 0.0365, 1_000_00)
	if err != nil {
		t.Fatal(err)
	}
	if got := balance(t, second, "acct-0000"); got != 1_000_10 {
		t.Fatalf("acct-0000 after restart = %d, want 100010", got)
	}
	if got := balance(t, second, "acct-0001"); got != 500_10 {
		t.Fatalf("acct-0001 after restart = %d, want 50010", got)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("accounts rows = %d, %v; want 2", n, err)
	}
}
