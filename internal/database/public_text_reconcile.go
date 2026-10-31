// public_text_reconcile.go — boot-time removal of GM-only parts from stored
// text that every reader is served as stored.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// PublicTextColumn names one stored rich-text column whose readers are not
// split by role, so it must never hold GM-only parts: the row's own
// visibility decides who reads it. JSON marks ProseMirror JSON; otherwise the
// column holds HTML. Each owning plugin declares its own columns.
type PublicTextColumn struct {
	Table  string
	Column string
	JSON   bool
}

// ReconcilePublicTextColumns removes GM-only parts (secret passages, GM-only
// pictures and diagrams, table rollers) from rows written before the write
// paths stripped them, using the same sanitize rules as those writes. It is a
// data fix, so it lives here rather than in a migration.
//
// Idempotent: only rows that still hold GM-only parts are rewritten, and a
// rewrite holds none. A table or column that doesn't exist (plugin not
// migrated) is skipped. A column that fails is reported and the rest still
// run. Returns the number of rows changed.
func ReconcilePublicTextColumns(ctx context.Context, db *sql.DB, cols []PublicTextColumn) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("ReconcilePublicTextColumns: nil db handle")
	}
	total := 0
	var errs []error
	for _, col := range cols {
		n, err := reconcilePublicTextColumn(ctx, db, col)
		total += n
		if err != nil {
			errs = append(errs, fmt.Errorf("reconciling %s.%s: %w", col.Table, col.Column, err))
		}
	}
	return total, errors.Join(errs...)
}

// publicTextStripped is the column value with its GM-only parts removed, and
// whether there were any.
func publicTextStripped(col PublicTextColumn, v string) (string, bool) {
	if col.JSON {
		if !sanitize.HasSecrets(v, "") {
			return v, false
		}
		return sanitize.StripSecretsJSON(v), true
	}
	out := sanitize.StripSecretsHTML(v)
	return out, out != v
}

func reconcilePublicTextColumn(ctx context.Context, db *sql.DB, col PublicTextColumn) (int, error) {
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

	// The markers narrow the scan to rows that can hold GM-only parts; the
	// Go check decides. Collect first, then update, so no UPDATE runs while
	// the result set is still open on the same connection pool.
	markers := `(` + column + ` LIKE '%data-secret%' OR ` + column + ` LIKE '%ce-img--gm%' OR ` +
		column + ` LIKE '%ce-diagram--gm%' OR ` + column + ` LIKE '%ce-roll%')`
	if col.JSON {
		markers = `(` + column + ` LIKE '%"secret"%' OR ` + column + ` LIKE '%gmOnly%' OR ` + column + ` LIKE '%rollTable%')`
	}
	type row struct{ id, old, fixed string }
	var fix []row
	rows, err := db.QueryContext(ctx, "SELECT id, "+column+" FROM "+table+" WHERE "+markers)
	if err != nil {
		return 0, fmt.Errorf("listing rows: %w", err)
	}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.old); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning row: %w", err)
		}
		var changed bool
		if r.fixed, changed = publicTextStripped(col, r.old); changed {
			fix = append(fix, r)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterating rows: %w", err)
	}
	rows.Close()

	changed := 0
	for _, r := range fix {
		// BINARY match so a concurrent edit is never overwritten.
		res, err := db.ExecContext(ctx,
			"UPDATE "+table+" SET "+column+" = ? WHERE id = ? AND BINARY "+column+" = ?",
			r.fixed, r.id, r.old)
		if err != nil {
			return changed, fmt.Errorf("rewriting row %s: %w", r.id, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed += int(n)
			slog.Warn("public text reconcile: removed GM-only parts",
				slog.String("table", col.Table), slog.String("column", col.Column), slog.String("id", r.id))
		}
	}
	return changed, nil
}
