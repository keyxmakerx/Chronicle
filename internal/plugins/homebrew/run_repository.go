package homebrew

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type sqlRunStore struct{ db *sql.DB }

// NewRunStore returns the MariaDB store of rule runs.
func NewRunStore(db *sql.DB) RunStore { return &sqlRunStore{db: db} }

const runCols = `id, campaign_id, recipe_id, recipe_name, what, rule_json, COALESCE(scope, ''), COALESCE(payload, ''),
	status, COALESCE(outcome, ''), COALESCE(created_by, ''), created_at, COALESCE(done_by, ''), done_at`

func scanRun(sc interface{ Scan(...any) error }) (Run, error) {
	var r Run
	var rule, payload, lines string
	var doneAt sql.NullTime
	if err := sc.Scan(&r.ID, &r.CampaignID, &r.RecipeID, &r.RecipeName, &r.What, &rule, &r.Scope, &payload,
		&r.Status, &lines, &r.CreatedBy, &r.CreatedAt, &r.DoneBy, &doneAt); err != nil {
		return r, err
	}
	if err := json.Unmarshal([]byte(rule), &r.Rule); err != nil {
		return r, fmt.Errorf("decode run %s: %w", r.ID, err)
	}
	if payload != "" {
		r.Payload = json.RawMessage(payload)
	}
	if lines != "" {
		if err := json.Unmarshal([]byte(lines), &r.Lines); err != nil {
			return r, fmt.Errorf("decode run %s lines: %w", r.ID, err)
		}
	}
	if doneAt.Valid {
		t := doneAt.Time
		r.DoneAt = &t
	}
	return r, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func linesJSON(lines []string) (any, error) {
	if lines == nil {
		return nil, nil
	}
	b, err := json.Marshal(lines)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s *sqlRunStore) CreateRun(ctx context.Context, r Run) error {
	rule, err := json.Marshal(r.Rule)
	if err != nil {
		return fmt.Errorf("encode run rule: %w", err)
	}
	lines, err := linesJSON(r.Lines)
	if err != nil {
		return fmt.Errorf("encode run lines: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO homebrew_runs
		(id, campaign_id, recipe_id, recipe_name, what, rule_json, scope, payload, status, outcome, created_by, created_at, done_by, done_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.CampaignID, r.RecipeID, r.RecipeName, r.What, string(rule), nullable(r.Scope), nullable(string(r.Payload)),
		r.Status, lines, nullable(r.CreatedBy), r.CreatedAt, nullable(r.DoneBy), r.DoneAt)
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	return nil
}

func (s *sqlRunStore) GetRun(ctx context.Context, campaignID, id string) (Run, bool, error) {
	r, err := scanRun(s.db.QueryRowContext(ctx,
		`SELECT `+runCols+` FROM homebrew_runs WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if err == sql.ErrNoRows {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, fmt.Errorf("get run: %w", err)
	}
	return r, true, nil
}

func (s *sqlRunStore) FinishRun(ctx context.Context, r Run, from string) (bool, error) {
	lines, err := linesJSON(r.Lines)
	if err != nil {
		return false, fmt.Errorf("encode run lines: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE homebrew_runs SET status = ?, outcome = ?, done_by = ?, done_at = ?
		WHERE campaign_id = ? AND id = ? AND status = ?`,
		r.Status, lines, nullable(r.DoneBy), r.DoneAt, r.CampaignID, r.ID, from)
	if err != nil {
		return false, fmt.Errorf("finish run: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("finish run: %w", err)
	}
	return n == 1, nil
}

func (s *sqlRunStore) ListRuns(ctx context.Context, campaignID string, held bool, limit int) ([]Run, error) {
	op := "<>"
	if held {
		op = "="
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+runCols+` FROM homebrew_runs
		WHERE campaign_id = ? AND status `+op+` ? ORDER BY created_at DESC, id DESC LIMIT ?`, campaignID, RunHeld, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list runs: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneRuns keeps the newest keep finished runs. MariaDB can't LIMIT a
// subquery under IN, so the cut-off row is found first.
func (s *sqlRunStore) PruneRuns(ctx context.Context, campaignID string, keep int) error {
	var cut sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT created_at FROM homebrew_runs
		WHERE campaign_id = ? AND status <> ? ORDER BY created_at DESC LIMIT 1 OFFSET ?`, campaignID, RunHeld, keep).Scan(&cut)
	if err == sql.ErrNoRows || (err == nil && !cut.Valid) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("prune runs: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM homebrew_runs
		WHERE campaign_id = ? AND status <> ? AND created_at <= ?`, campaignID, RunHeld, cut.Time); err != nil {
		return fmt.Errorf("prune runs: %w", err)
	}
	return nil
}
