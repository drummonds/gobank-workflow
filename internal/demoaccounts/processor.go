// Package demoaccounts is a DayProcessor for demos: a book of identical
// easy-access savings accounts accruing simple daily interest. It has no
// place in production; gobank-products owns real product logic.
package demoaccounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sync"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"github.com/google/uuid"
)

// ErrNoAccount is returned for an account the book does not have.
var ErrNoAccount = errors.New("demoaccounts: no such account")

// accountsSQL is the book: the host's own table in the same database as
// the runs, so a restart from the database alone finds the balances the
// last day and the payments left. The daily step's records on
// workflow_steps are what happened to a balance; this row is the balance.
const accountsSQL = `
CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY,
    account_id VARCHAR(20) NOT NULL UNIQUE,
    balance INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);
`

// Processor implements wf.DayProcessor over the accounts table.
type Processor struct {
	db   *sql.DB
	rate float64 // annual rate

	mu     sync.Mutex
	failOn map[wf.BusinessDate]bool
}

// Open returns a Processor over the accounts table in db at the given
// annual rate, creating the table if the database lacks it. An empty
// book opens n accounts with balance minor units each; a book with
// accounts is left as the last process left it.
func Open(ctx context.Context, db *sql.DB, n int, rate float64, balance int64) (*Processor, error) {
	if _, err := db.ExecContext(ctx, accountsSQL); err != nil {
		return nil, fmt.Errorf("demoaccounts: accounts table: %w", err)
	}
	var have int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM accounts`).Scan(&have); err != nil {
		return nil, fmt.Errorf("demoaccounts: counting accounts: %w", err)
	}
	if have == 0 {
		for i := range n {
			if _, err := db.ExecContext(ctx, `INSERT INTO accounts (id, account_id, balance) VALUES ($1, $2, $3)`,
				uuid.New().String(), fmt.Sprintf("acct-%04d", i), balance); err != nil {
				return nil, fmt.Errorf("demoaccounts: opening account %d: %w", i, err)
			}
		}
	}
	return &Processor{db: db, rate: rate, failOn: map[wf.BusinessDate]bool{}}, nil
}

// FailOn makes the next ProcessDay for date return an error, so a demo
// can show a failed instance and its retry. The failure fires once.
func (p *Processor) FailOn(date wf.BusinessDate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failOn[date] = true
}

// injected reports, and disarms, a failure armed for date.
func (p *Processor) injected(date wf.BusinessDate) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.failOn[date] {
		return false
	}
	delete(p.failOn, date)
	return true
}

// ProcessDay accrues one day of interest on every account, in account
// order, as one transaction: the day is on the book whole or not at all.
func (p *Processor) ProcessDay(ctx context.Context, date wf.BusinessDate) (wf.DayResult, error) {
	if p.injected(date) {
		return wf.DayResult{}, fmt.Errorf("demoaccounts: rate feed unavailable for %s (injected)", date)
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return wf.DayResult{}, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT account_id, balance FROM accounts ORDER BY account_id`)
	if err != nil {
		return wf.DayResult{}, err
	}
	var results []wf.AccountResult
	for rows.Next() {
		var id string
		var opening int64
		if err := rows.Scan(&id, &opening); err != nil {
			rows.Close()
			return wf.DayResult{}, err
		}
		interest := int64(math.Round(float64(opening) * p.rate / 365))
		results = append(results, wf.AccountResult{
			AccountID:      id,
			OpeningBalance: opening,
			ClosingBalance: opening + interest,
			InterestAmount: interest,
			Exponent:       -2,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return wf.DayResult{}, err
	}
	for _, r := range results {
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET balance = $1 WHERE account_id = $2`, r.ClosingBalance, r.AccountID); err != nil {
			return wf.DayResult{}, fmt.Errorf("demoaccounts: %s: %w", r.AccountID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return wf.DayResult{}, err
	}
	return wf.DayResult{Date: date, Accounts: results}, nil
}

// Balance is the account's current balance in minor units; ErrNoAccount
// for an account the book does not have.
func (p *Processor) Balance(ctx context.Context, id string) (int64, error) {
	var balance int64
	err := p.db.QueryRowContext(ctx, `SELECT balance FROM accounts WHERE account_id = $1`, id).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: %s", ErrNoAccount, id)
	}
	return balance, err
}

// Debit takes amount from the account. It refuses an unknown account and
// more than the balance, leaving the balance as it was.
func (p *Processor) Debit(ctx context.Context, id string, amount int64) error {
	balance, err := p.Balance(ctx, id)
	if err != nil {
		return err
	}
	if amount > balance {
		return fmt.Errorf("demoaccounts: %s: %d exceeds balance %d", id, amount, balance)
	}
	_, err = p.db.ExecContext(ctx, `UPDATE accounts SET balance = balance - $1 WHERE account_id = $2`, amount, id)
	return err
}
