package demoaccounts

import (
	"context"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

func TestProcessDayAccruesDailyInterestOnEveryAccount(t *testing.T) {
	p := New(3, 0.0365, 1_000_00) // 1,000.00 at 3.65% → 10p a day
	res, err := p.ProcessDay(context.Background(), wf.NewBusinessDate(2026, time.January, 1))
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
	res, _ = p.ProcessDay(context.Background(), wf.NewBusinessDate(2026, time.January, 2))
	if res.Accounts[0].OpeningBalance != 1_000_10 {
		t.Fatalf("day 2 opening = %d, want 100010", res.Accounts[0].OpeningBalance)
	}
}

func TestProcessDayFailsOnTheConfiguredDate(t *testing.T) {
	bad := wf.NewBusinessDate(2026, time.January, 3)
	p := New(1, 0.0365, 1_000_00)
	p.FailOn(bad)
	if _, err := p.ProcessDay(context.Background(), wf.NewBusinessDate(2026, time.January, 2)); err != nil {
		t.Fatal("day before should succeed")
	}
	if _, err := p.ProcessDay(context.Background(), bad); err == nil {
		t.Fatal("configured date should fail")
	}
	if _, err := p.ProcessDay(context.Background(), bad); err != nil {
		t.Fatal("a retry of the failed date should succeed")
	}
}

func TestDebitTakesTheAmountFromTheAccount(t *testing.T) {
	p := New(2, 0.035, 1_000_000)
	if err := p.Debit("acct-0001", 30_000); err != nil {
		t.Fatal(err)
	}
	if got, ok := p.Balance("acct-0001"); !ok || got != 970_000 {
		t.Fatalf("balance = %d, %v; want 970000", got, ok)
	}
	if got, _ := p.Balance("acct-0000"); got != 1_000_000 {
		t.Fatalf("other account touched: %d", got)
	}
}

func TestDebitRefusesMoreThanTheBalanceOrAnUnknownAccount(t *testing.T) {
	p := New(1, 0.035, 1_000)
	if err := p.Debit("acct-0000", 1_001); err == nil {
		t.Fatal("overdrawing should fail")
	}
	if got, _ := p.Balance("acct-0000"); got != 1_000 {
		t.Fatalf("a refused debit changed the balance: %d", got)
	}
	if err := p.Debit("acct-0009", 1); err == nil {
		t.Fatal("unknown account should fail")
	}
	if _, ok := p.Balance("acct-0009"); ok {
		t.Fatal("unknown account reported a balance")
	}
}
