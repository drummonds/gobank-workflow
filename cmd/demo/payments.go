package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/cmd/demo/definitions"
)

// paymentsPerBatch is the size of every batch the simulation pays.
const paymentsPerBatch = 5

// payment is one line of a batch file: position n (from 1) pays n × 100.00
// out of the n-th account, wrapping round the book. A payment is a function
// of its key, so nothing is stored beyond the runs.
type payment struct {
	Batch   string
	N       int
	Account string
	Amount  int64
}

func (p payment) Key() string { return fmt.Sprintf("%s/%d", p.Batch, p.N) }

// batchRef names the n-th batch paid.
func batchRef(n int) string { return fmt.Sprintf("PAY-%04d", n) }

// paymentsOf lists the payments of the batch with that reference.
func (a *App) paymentsOf(batch string) []payment {
	ps := make([]payment, paymentsPerBatch)
	for i := range ps {
		n := i + 1
		ps[i] = payment{Batch: batch, N: n, Account: fmt.Sprintf("acct-%04d", (n-1)%a.opts.Accounts), Amount: int64(n) * 10_000}
	}
	return ps
}

// paymentOf is the payment a payment run is about, from its key.
func (a *App) paymentOf(key string) (payment, bool) {
	batch, pos, ok := strings.Cut(key, "/")
	n, err := strconv.Atoi(pos)
	if !ok || err != nil || n < 1 || n > paymentsPerBatch {
		return payment{}, false
	}
	return a.paymentsOf(batch)[n-1], true
}

// NextBatch is the reference the next Pay will pay: the one after the
// last settled, or the same again while it is failed.
func (a *App) NextBatch() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.nextBatch()
}

func (a *App) nextBatch() string { return batchRef(a.paid + 1) }

// Pay runs the payment batch for the next reference on the current
// business date, with its payments as child instances. failPayment, if
// not zero, is the position of the payment that fails to post this
// attempt. A failed batch keeps its reference, so the next Pay retries
// the failed payments and settles it.
//
// One Pay is the host's tick twice over: the batch runs until it parks
// waiting for payments it has just started, the payments run, and the
// batch runs again to settle. On a retry the payments go first, so a
// failed one has been retried by the time the batch settles.
func (a *App) Pay(ctx context.Context, failPayment int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ref := a.nextBatch()
	var rec *wf.RunRecord
	var err error
	for range 2 {
		a.payPayments(ctx, ref, failPayment)
		rec, err = a.runner.RunPipeline(ctx, definitions.BatchType, ref, a.current, a.batchStages(ref))
		if err != nil || rec == nil || rec.Status != wf.StatusWaiting {
			break
		}
	}
	switch {
	case err != nil:
		a.logf("batch %s failed: %v", ref, err)
	case rec == nil:
		a.logf("batch %s already paid", ref)
	case rec.Status == wf.StatusWaiting:
		a.logf("batch %s still waiting for its payments", ref)
	default:
		a.paid++
		a.logf("batch %s settled: %d payments", ref, paymentsPerBatch)
	}
}

// payPayments runs every payment of the batch that is still to do.
func (a *App) payPayments(ctx context.Context, ref string, failPayment int) {
	batch, err := a.store.FindRunByKey(ctx, definitions.BatchType, ref)
	if err != nil || batch == nil {
		return
	}
	children, err := a.store.ListRuns(ctx, wf.RunFilter{ParentID: batch.ID})
	if err != nil {
		a.logf("batch %s: listing payments: %v", ref, err)
		return
	}
	for _, c := range children {
		if !c.Status.Active() {
			continue
		}
		p, ok := a.paymentOf(c.Key)
		if !ok {
			continue
		}
		if _, err := a.runner.RunPipeline(ctx, definitions.PaymentType, p.Key(), a.current, a.paymentStages(p, p.N == failPayment)); err != nil {
			a.logf("payment %s failed: %v", p.Key(), err)
		}
	}
}

// batchStages is the batch as RunPipeline runs it: receive, start one
// payment child per payment and return, settle once none is active.
func (a *App) batchStages(ref string) []wf.Stage {
	ctxRun := func(ctx context.Context) (*wf.RunRecord, error) {
		run, err := a.store.FindRunByKey(ctx, definitions.BatchType, ref)
		if err == nil && run == nil {
			err = fmt.Errorf("batch %s has no run", ref)
		}
		return run, err
	}
	var total int64
	for _, p := range a.paymentsOf(ref) {
		total += p.Amount
	}
	return []wf.Stage{
		{Name: "receive", Run: func(context.Context) error {
			a.logf("batch %s received: %d payments, %s", ref, paymentsPerBatch, money(total, -2))
			return nil
		}},
		{Name: "start payments", Run: func(ctx context.Context) error {
			run, err := ctxRun(ctx)
			if err != nil {
				return err
			}
			for _, p := range a.paymentsOf(ref) {
				if _, err := a.runner.StartChild(ctx, run.ID, definitions.PaymentType, p.Key(), a.current); err != nil {
					return err
				}
			}
			return nil
		}},
		{Name: "settle",
			Until: func(ctx context.Context) (bool, error) {
				run, err := ctxRun(ctx)
				if err != nil {
					return false, err
				}
				counts, err := a.store.CountDescendants(ctx, run.ID)
				if err != nil {
					return false, err
				}
				return counts[wf.StatusPending]+counts[wf.StatusRunning]+counts[wf.StatusWaiting] == 0, nil
			},
			Run: func(ctx context.Context) error {
				run, err := ctxRun(ctx)
				if err != nil {
					return err
				}
				counts, err := a.store.CountDescendants(ctx, run.ID)
				if err != nil {
					return err
				}
				if n := counts[wf.StatusFailed]; n > 0 {
					return fmt.Errorf("%d of %d payments failed", n, paymentsPerBatch)
				}
				return nil
			}},
	}
}

// paymentStages is one payment as RunPipeline runs it. fail makes the
// post stage fail this attempt, once.
func (a *App) paymentStages(p payment, fail bool) []wf.Stage {
	return []wf.Stage{
		{Name: "check funds", Run: func(context.Context) error {
			balance, ok := a.proc.Balance(p.Account)
			if !ok {
				return fmt.Errorf("no account %s", p.Account)
			}
			if balance < p.Amount {
				return fmt.Errorf("%s: %s exceeds balance %s", p.Account, money(p.Amount, -2), money(balance, -2))
			}
			return nil
		}},
		{Name: "post", Run: func(context.Context) error {
			time.Sleep(time.Millisecond) // a visible, if token, duration
			if fail {
				return fmt.Errorf("posting %s failed (injected)", p.Key())
			}
			return a.proc.Debit(p.Account, p.Amount)
		}},
	}
}
