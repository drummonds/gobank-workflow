package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/cmd/demo/definitions"
	"github.com/google/uuid"
)

// paymentsPerBatch is the size of every batch file the simulation receives.
const paymentsPerBatch = 5

// paymentsSQL is the host's own table for the batch file: step data, in
// the same database as the runs and keyed to them, so a payment resumes
// from the database alone. The batch's receive stage writes a row per
// payment; start payments keys each row to the child run it starts; the
// payment's post stage stamps it posted. Nothing about a payment lives
// only in the process.
const paymentsSQL = `
CREATE TABLE IF NOT EXISTS payments (
    id TEXT PRIMARY KEY,
    batch_run_id TEXT NOT NULL REFERENCES workflow_runs(id),
    batch_ref VARCHAR(20) NOT NULL,
    position INTEGER NOT NULL,
    payment_key VARCHAR(40) NOT NULL UNIQUE,
    account VARCHAR(20) NOT NULL,
    amount INTEGER NOT NULL,
    run_id TEXT REFERENCES workflow_runs(id),
    posted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_payments_batch ON payments(batch_run_id);
CREATE INDEX IF NOT EXISTS idx_payments_run ON payments(run_id);
`

// payment is one row of the payments table: one line of a batch file.
type payment struct {
	ID       string
	Batch    string
	N        int
	Key      string
	Account  string
	Amount   int64
	RunID    string // the payment instance, once started
	PostedAt string // when post succeeded, empty until then
}

// batchRef names the n-th batch paid.
func batchRef(n int) string { return fmt.Sprintf("PAY-%04d", n) }

// batchFile is the file the simulation receives for a batch: position n
// (from 1) pays n × 100.00 out of the n-th account, wrapping round the
// book. It stands in for the network; receive is what makes it data.
func (a *App) batchFile(ref string) []payment {
	ps := make([]payment, paymentsPerBatch)
	for i := range ps {
		n := i + 1
		ps[i] = payment{Batch: ref, N: n, Key: fmt.Sprintf("%s/%d", ref, n), Account: fmt.Sprintf("acct-%04d", (n-1)%a.opts.Accounts), Amount: int64(n) * 10_000}
	}
	return ps
}

// receive writes the batch file to the payments table under the batch's
// run. A line already received is left as it is, so a receive that is
// run again after a crash is harmless.
func (a *App) receive(ctx context.Context, batchRunID string, ps []payment) error {
	for _, p := range ps {
		_, err := a.db.ExecContext(ctx, `INSERT INTO payments (id, batch_run_id, batch_ref, position, payment_key, account, amount)
			VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (payment_key) DO NOTHING`,
			uuid.New().String(), batchRunID, p.Batch, p.N, p.Key, p.Account, p.Amount)
		if err != nil {
			return fmt.Errorf("receiving %s: %w", p.Key, err)
		}
	}
	return nil
}

const paymentColumns = `id, batch_ref, position, payment_key, account, amount, run_id, posted_at`

func scanPayment(row interface{ Scan(...any) error }) (*payment, error) {
	var p payment
	var runID, postedAt sql.NullString
	if err := row.Scan(&p.ID, &p.Batch, &p.N, &p.Key, &p.Account, &p.Amount, &runID, &postedAt); err != nil {
		return nil, err
	}
	p.RunID, p.PostedAt = runID.String, postedAt.String
	return &p, nil
}

// paymentsOf lists the payments of the batch with that reference, in file
// order. Caller holds a.mu.
func (a *App) paymentsOf(ctx context.Context, ref string) ([]payment, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT `+paymentColumns+` FROM payments WHERE batch_ref = $1 ORDER BY position`, ref)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ps []payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		ps = append(ps, *p)
	}
	return ps, rows.Err()
}

// paymentByRun is the payment a payment instance is about, nil if the run
// is not a payment's. Caller holds a.mu.
func (a *App) paymentByRun(ctx context.Context, runID string) (*payment, error) {
	p, err := scanPayment(a.db.QueryRowContext(ctx, `SELECT `+paymentColumns+` FROM payments WHERE run_id = $1`, runID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
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

// payPayments runs every payment of the batch that is still to do, each
// from its row in the payments table.
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
		p, err := a.paymentByRun(ctx, c.ID)
		if err != nil || p == nil {
			a.logf("payment %s has no row in payments: %v", c.Key, err)
			continue
		}
		if _, err := a.runner.RunPipeline(ctx, definitions.PaymentType, p.Key, a.current, a.paymentStages(*p, p.N == failPayment)); err != nil {
			a.logf("payment %s failed: %v", p.Key, err)
		}
	}
}

// batchStages is the batch as RunPipeline runs it: receive the file into
// the payments table, start one payment child per row and return, settle
// once none is active.
func (a *App) batchStages(ref string) []wf.Stage {
	ctxRun := func(ctx context.Context) (*wf.RunRecord, error) {
		run, err := a.store.FindRunByKey(ctx, definitions.BatchType, ref)
		if err == nil && run == nil {
			err = fmt.Errorf("batch %s has no run", ref)
		}
		return run, err
	}
	return []wf.Stage{
		{Name: "receive", Run: func(ctx context.Context) error {
			run, err := ctxRun(ctx)
			if err != nil {
				return err
			}
			file := a.batchFile(ref)
			if err := a.receive(ctx, run.ID, file); err != nil {
				return err
			}
			var total int64
			for _, p := range file {
				total += p.Amount
			}
			a.logf("batch %s received: %d payments, %s", ref, len(file), money(total, -2))
			return nil
		}},
		{Name: "start payments", Run: func(ctx context.Context) error {
			run, err := ctxRun(ctx)
			if err != nil {
				return err
			}
			ps, err := a.paymentsOf(ctx, ref)
			if err != nil {
				return err
			}
			for _, p := range ps {
				child, err := a.runner.StartChild(ctx, run.ID, definitions.PaymentType, p.Key, a.current)
				if err != nil {
					return err
				}
				if _, err := a.db.ExecContext(ctx, `UPDATE payments SET run_id = $1 WHERE id = $2`, child.ID, p.ID); err != nil {
					return fmt.Errorf("keying %s to its run: %w", p.Key, err)
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

// paymentStages is one payment as RunPipeline runs it, from its row. fail
// makes the post stage fail this attempt, once.
func (a *App) paymentStages(p payment, fail bool) []wf.Stage {
	return []wf.Stage{
		{Name: "check funds", Run: func(ctx context.Context) error {
			balance, err := a.proc.Balance(ctx, p.Account)
			if err != nil {
				return err
			}
			if balance < p.Amount {
				return fmt.Errorf("%s: %s exceeds balance %s", p.Account, money(p.Amount, -2), money(balance, -2))
			}
			return nil
		}},
		{Name: "post", Run: func(ctx context.Context) error {
			time.Sleep(time.Millisecond) // a visible, if token, duration
			if fail {
				return fmt.Errorf("posting %s failed (injected)", p.Key)
			}
			if err := a.proc.Debit(ctx, p.Account, p.Amount); err != nil {
				return err
			}
			_, err := a.db.ExecContext(ctx, `UPDATE payments SET posted_at = $1 WHERE id = $2`, time.Now().UTC().Format(time.RFC3339Nano), p.ID)
			return err
		}},
	}
}
