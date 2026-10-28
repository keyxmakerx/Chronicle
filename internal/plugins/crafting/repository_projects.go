package crafting

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ProjectStore keeps projects, respites and rolls. Every statement filters
// on campaign_id, as the recipe repository's do.
type ProjectStore interface {
	CreateProject(ctx context.Context, p Project) error
	GetProject(ctx context.Context, campaignID, id string) (Project, bool, error)
	// ListProjects lists projects in one status, for one character or (with
	// characterID "") the whole campaign: in progress oldest first,
	// finished ones newest first. limit 0 means no limit.
	ListProjects(ctx context.Context, campaignID, characterID, status string, limit int) ([]Project, error)
	CountActive(ctx context.Context, campaignID, characterID string) (int, error)
	// UpdateProject writes the points, rolls ready, status, delivered flag
	// and finish time if the stored version is still expected; ok is false
	// on a lost race.
	UpdateProject(ctx context.Context, p Project, expected int) (ok bool, err error)
	AddRoll(ctx context.Context, campaignID, projectID, rolledBy, dice, terms string, total, from, to int) error
	// LogRespite gives every project in progress one more roll and records
	// the respite, in one transaction, and says how many projects it
	// reached.
	LogRespite(ctx context.Context, campaignID, loggedBy, dayLabel string) (int, error)
	LastRespite(ctx context.Context, campaignID string) (*Respite, error)
}

const projectColumns = `id, campaign_id, character_id, COALESCE(recipe_id, ''), recipe_name, goal, stat, COALESCE(result_item_id, ''),
	result_qty, points, rolls_ready, status, delivered, took, COALESCE(started_by, ''), started_at, finished_at, version`

func scanProject(s rowScanner) (Project, error) {
	var (
		p        Project
		finished sql.NullTime
	)
	err := s.Scan(&p.ID, &p.CampaignID, &p.CharacterID, &p.RecipeID, &p.RecipeName, &p.Goal, &p.Stat, &p.ResultItemID,
		&p.ResultQty, &p.Points, &p.RollsReady, &p.Status, &p.Delivered, &p.Took, &p.StartedBy, &p.StartedAt, &finished, &p.Version)
	if finished.Valid {
		t := finished.Time
		p.FinishedAt = &t
	}
	return p, err
}

func (r *sqlRepository) CreateProject(ctx context.Context, p Project) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO crafting_projects (id, campaign_id, character_id, recipe_id, recipe_name, goal, stat, result_item_id, result_qty,
		 points, rolls_ready, status, took, started_by, started_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?)`,
		p.ID, p.CampaignID, p.CharacterID, nullString(p.RecipeID), p.RecipeName, p.Goal, p.Stat, nullString(p.ResultItemID), p.ResultQty,
		ProjectActive, p.Took, nullString(p.StartedBy), p.StartedAt)
	if err != nil {
		return fmt.Errorf("create crafting project: %w", err)
	}
	return nil
}

func (r *sqlRepository) GetProject(ctx context.Context, campaignID, id string) (Project, bool, error) {
	p, err := scanProject(r.db.QueryRowContext(ctx,
		`SELECT `+projectColumns+` FROM crafting_projects WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, false, nil
	}
	if err != nil {
		return Project{}, false, fmt.Errorf("get crafting project: %w", err)
	}
	return p, true, nil
}

func (r *sqlRepository) ListProjects(ctx context.Context, campaignID, characterID, status string, limit int) ([]Project, error) {
	q := `SELECT ` + projectColumns + ` FROM crafting_projects WHERE campaign_id = ? AND status = ?`
	args := []any{campaignID, status}
	if characterID != "" {
		q += ` AND character_id = ?`
		args = append(args, characterID)
	}
	if status == ProjectActive {
		q += ` ORDER BY started_at, id`
	} else {
		q += ` ORDER BY finished_at DESC, id`
	}
	if limit > 0 {
		q += fmt.Sprintf(` LIMIT %d`, limit)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list crafting projects: %w", err)
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("scan crafting project: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list crafting projects: %w", err)
	}
	return out, nil
}

func (r *sqlRepository) CountActive(ctx context.Context, campaignID, characterID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM crafting_projects WHERE campaign_id = ? AND character_id = ? AND status = ?`,
		campaignID, characterID, ProjectActive).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count crafting projects: %w", err)
	}
	return n, nil
}

func (r *sqlRepository) UpdateProject(ctx context.Context, p Project, expected int) (bool, error) {
	var finished any
	if p.FinishedAt != nil {
		finished = *p.FinishedAt
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE crafting_projects SET points = ?, rolls_ready = ?, status = ?, delivered = ?, finished_at = ?, version = version + 1
		 WHERE campaign_id = ? AND id = ? AND version = ?`,
		p.Points, p.RollsReady, p.Status, p.Delivered, finished, p.CampaignID, p.ID, expected)
	if err != nil {
		return false, fmt.Errorf("update crafting project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update crafting project: %w", err)
	}
	return n == 1, nil
}

func (r *sqlRepository) AddRoll(ctx context.Context, campaignID, projectID, rolledBy, dice, terms string, total, from, to int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO crafting_rolls (project_id, campaign_id, rolled_by, dice, terms, total, points_from, points_to) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		projectID, campaignID, nullString(rolledBy), dice, terms, total, from, to)
	if err != nil {
		return fmt.Errorf("add crafting roll: %w", err)
	}
	return nil
}

func (r *sqlRepository) LogRespite(ctx context.Context, campaignID, loggedBy, dayLabel string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("log respite: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`UPDATE crafting_projects SET rolls_ready = LEAST(rolls_ready + 1, ?), version = version + 1 WHERE campaign_id = ? AND status = ?`,
		maxRollsReady, campaignID, ProjectActive)
	if err != nil {
		return 0, fmt.Errorf("log respite: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("log respite: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO crafting_respites (campaign_id, logged_by, day_label, projects) VALUES (?, ?, ?, ?)`,
		campaignID, nullString(loggedBy), clipText(dayLabel, 80), n); err != nil {
		return 0, fmt.Errorf("log respite: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("log respite: %w", err)
	}
	return int(n), nil
}

func (r *sqlRepository) LastRespite(ctx context.Context, campaignID string) (*Respite, error) {
	var (
		out Respite
		at  time.Time
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT day_label, projects, created_at FROM crafting_respites WHERE campaign_id = ? ORDER BY id DESC LIMIT 1`, campaignID,
	).Scan(&out.DayLabel, &out.Projects, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("last respite: %w", err)
	}
	out.At = at
	return &out, nil
}

// clipText keeps s within max characters, so a long calendar label or a
// long list of what a project took fits its column.
func clipText(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
