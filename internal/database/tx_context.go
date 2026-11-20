package database

import (
	"context"
	"database/sql"
)

// A multi-write commit across services (a page and its listings) runs inside
// one SQL transaction carried by the context. A repository that takes part
// asks Q for its executor instead of using its *sql.DB directly: with a
// transaction in the context it gets that, otherwise the plain handle. Writes
// to other tables stay on their own connections, so anything that must not
// show before the commit goes through AfterCommit.

// Querier is what *sql.DB and *sql.Tx share.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type txKey struct{}

type txState struct {
	tx    *sql.Tx
	after []func()
}

// Q returns the transaction in ctx, or db when there is none.
func Q(ctx context.Context, db *sql.DB) Querier {
	if st, ok := ctx.Value(txKey{}).(*txState); ok {
		return st.tx
	}
	return db
}

// AfterCommit runs fn once the surrounding transaction commits, or at once
// when there is none. A rolled-back transaction never runs it, so events and
// history never describe rows that were not kept.
func AfterCommit(ctx context.Context, fn func()) {
	if st, ok := ctx.Value(txKey{}).(*txState); ok {
		st.after = append(st.after, fn)
		return
	}
	fn()
}

// RunInTx runs fn in one transaction: commit when it returns nil, roll back
// otherwise. Inside an existing transaction it just runs fn, so the outer
// one decides.
func RunInTx(ctx context.Context, db *sql.DB, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(*txState); ok {
		return fn(ctx)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	st := &txState{tx: tx}
	if err := fn(context.WithValue(ctx, txKey{}, st)); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && rbErr != sql.ErrTxDone {
			return &RollbackError{Cause: err, Rollback: rbErr}
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, f := range st.after {
		f()
	}
	return nil
}

// RollbackError is a failure whose rollback failed too, so rows may remain.
type RollbackError struct {
	Cause    error
	Rollback error
}

func (e *RollbackError) Error() string {
	return e.Cause.Error() + " (and the rollback failed: " + e.Rollback.Error() + ")"
}

func (e *RollbackError) Unwrap() error { return e.Cause }
