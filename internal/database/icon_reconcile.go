// icon_reconcile.go — boot-time repair of stored icon names.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// IconColumn names one stored icon column and the value that replaces an
// unusable entry in it. Each owning plugin declares its own columns; the
// boot sequence passes them all to ReconcileIconColumns.
type IconColumn struct {
	Table   string
	Column  string
	Default string
}

// ReconcileIconColumns replaces every stored icon that fails
// sanitize.IsIconName with its column's default and logs the old value, so
// no page is ever built from an unchecked icon. It is a data fix, so it
// lives here rather than in a migration.
//
// Idempotent: it only touches rows that fail the check, and a replaced row
// holds a valid default, so a second run changes nothing. NULLs are left
// alone. A table that doesn't exist (plugin not migrated) is skipped. The
// check runs in Go rather than SQL so it can't drift from the write-path
// rule. A column that fails is reported and the rest still run. Returns the
// number of rows changed.
func ReconcileIconColumns(ctx context.Context, db *sql.DB, cols []IconColumn) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("ReconcileIconColumns: nil db handle")
	}
	total := 0
	var errs []error
	for _, col := range cols {
		n, err := reconcileIconColumn(ctx, db, col)
		total += n
		if err != nil {
			errs = append(errs, fmt.Errorf("reconciling %s.%s icons: %w", col.Table, col.Column, err))
		}
	}
	return total, errors.Join(errs...)
}

func reconcileIconColumn(ctx context.Context, db *sql.DB, col IconColumn) (int, error) {
	if !sanitize.IsIconName(col.Default) {
		return 0, fmt.Errorf("default %q is not a valid icon name", col.Default)
	}
	table, err := SafeIdent(col.Table)
	if err != nil {
		return 0, err
	}
	column, err := SafeIdent(col.Column)
	if err != nil {
		return 0, err
	}

	var present int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
		col.Table, col.Column,
	).Scan(&present); err != nil {
		return 0, fmt.Errorf("checking column exists: %w", err)
	}
	if present == 0 {
		return 0, nil
	}

	// Collect first, then update, so no UPDATE runs while the result set is
	// still open on the same connection pool.
	type badRow struct{ id, icon string }
	var bad []badRow
	rows, err := db.QueryContext(ctx,
		"SELECT id, "+column+" FROM "+table+" WHERE "+column+" IS NOT NULL")
	if err != nil {
		return 0, fmt.Errorf("listing icons: %w", err)
	}
	for rows.Next() {
		var r badRow
		if err := rows.Scan(&r.id, &r.icon); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning icon row: %w", err)
		}
		if !sanitize.IsIconName(r.icon) {
			bad = append(bad, r)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterating icons: %w", err)
	}
	rows.Close()

	changed := 0
	for _, r := range bad {
		// BINARY match so a concurrent edit to a different (valid) value is
		// never overwritten under the case-insensitive collation.
		res, err := db.ExecContext(ctx,
			"UPDATE "+table+" SET "+column+" = ? WHERE id = ? AND BINARY "+column+" = ?",
			col.Default, r.id, r.icon)
		if err != nil {
			return changed, fmt.Errorf("resetting icon on row %s: %w", r.id, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed += int(n)
			slog.Warn("icon reconcile: replaced invalid stored icon",
				slog.String("table", col.Table),
				slog.String("id", r.id),
				slog.String("old_icon", r.icon),
				slog.String("new_icon", col.Default),
			)
		}
	}
	return changed, nil
}
