package systems

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// cleanItemText runs a list item's text through the same HTML policy as an
// entry's main description, but keeps the text as typed whenever the policy
// would only escape it. Item text is mostly plain prose that the rules page
// prints as text, so storing the escaped form would show "it&#39;s"; only
// text the policy actually changes (a disallowed tag or attribute) is
// replaced. The second return says whether it was.
func cleanItemText(s string) (string, bool) {
	if !strings.Contains(s, "<") {
		return s, false
	}
	clean := sanitize.HTML(s)
	if clean == s || html.UnescapeString(clean) == s {
		return s, false
	}
	return clean, true
}

// cleanItemTexts applies cleanItemText to the "description" of every item in
// every list of named items, in place, and says whether anything changed.
func cleanItemTexts(p map[string]any) bool {
	changed := false
	for _, v := range p {
		items, ok := v.([]any)
		if !ok {
			continue
		}
		for _, raw := range items {
			it, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			d, ok := it["description"].(string)
			if !ok {
				continue
			}
			if clean, did := cleanItemText(d); did {
				it["description"] = clean
				changed = true
			}
		}
	}
	return changed
}

// ReconcileEntryItemTexts applies the write path's item-text cleaning to
// campaign entries saved before that path cleaned them. It is a data fix, so
// it runs at boot rather than as a migration.
//
// Idempotent: only rows whose item texts still change are rewritten, and a
// cleaned text is left alone by the next pass. A missing table (systems not
// migrated) is skipped. Returns the number of rows changed.
func ReconcileEntryItemTexts(ctx context.Context, db *sql.DB) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("ReconcileEntryItemTexts: nil db handle")
	}
	var present int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.TABLES
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'campaign_system_entries'`,
	).Scan(&present); err != nil {
		return 0, fmt.Errorf("checking table exists: %w", err)
	}
	if present == 0 {
		return 0, nil
	}

	// Collect first, then update, so no UPDATE runs while the result set is
	// still open on the same pool.
	type row struct {
		id       int64
		old, new string
	}
	var fix []row
	// encoding/json writes "<" as \u003c, so look for both spellings.
	rows, err := db.QueryContext(ctx,
		`SELECT id, properties FROM campaign_system_entries
		 WHERE INSTR(properties, '<') > 0 OR INSTR(properties, ?) > 0`, `\u003c`)
	if err != nil {
		return 0, fmt.Errorf("listing rows: %w", err)
	}
	for rows.Next() {
		var r row
		var raw sql.NullString
		if err := rows.Scan(&r.id, &raw); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning row: %w", err)
		}
		// UseNumber keeps large whole numbers exact through the rewrite.
		var p map[string]any
		dec := json.NewDecoder(strings.NewReader(raw.String))
		dec.UseNumber()
		if dec.Decode(&p) != nil || !cleanItemTexts(p) {
			continue
		}
		out, err := json.Marshal(p)
		if err != nil || string(out) == raw.String {
			continue
		}
		r.old, r.new = raw.String, string(out)
		fix = append(fix, r)
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
			`UPDATE campaign_system_entries SET properties = ? WHERE id = ? AND BINARY properties = BINARY ?`,
			r.new, r.id, r.old)
		if err != nil {
			return changed, fmt.Errorf("updating entry %d: %w", r.id, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed++
		}
	}
	return changed, nil
}
