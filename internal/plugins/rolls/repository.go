package rolls

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Repository owns the campaign_rolls and campaign_roll_settings tables. Which
// rolls a viewer may read is decided in the SQL of List, so a read for a
// player never selects another person's hidden roll.
type Repository interface {
	// Insert adds a roll. A roll whose origin id is already recorded is not
	// added again: it returns that roll's id and duplicate=true.
	Insert(ctx context.Context, r Roll) (id string, duplicate bool, err error)
	// List returns up to limit rolls the viewer may see, newest first, made
	// before the roll before (all when empty).
	List(ctx context.Context, campaignID string, v Viewer, before string, limit int) ([]Roll, error)
	// Look returns the table's dice look, or "" when none was set.
	Look(ctx context.Context, campaignID string) (string, error)
	// SetLook saves the table's dice look.
	SetLook(ctx context.Context, campaignID, look string) error
}

type repository struct {
	db *sql.DB
}

// NewRepository creates the MariaDB repository for rolls.
func NewRepository(db *sql.DB) Repository {
	return &repository{db: db}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *repository) Insert(ctx context.Context, x Roll) (string, bool, error) {
	dice, err := json.Marshal(x.Dice)
	if err != nil {
		return "", false, apperror.NewInternal(fmt.Errorf("encode dice: %w", err))
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO campaign_rolls
		(id, campaign_id, origin, origin_id, source, roller_id, roller_name, label, formula, dice, modifier, total, outcome, why, from_where, mode, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		x.ID, x.CampaignID, x.Origin, nullable(x.OriginID), x.Source, nullable(x.RollerID), x.RollerName, x.Label, x.Formula,
		string(dice), x.Modifier, x.Total, x.Outcome, x.Why, x.From, x.Mode, x.CreatedAt)
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 && x.OriginID != "" {
		var id string
		if err := r.db.QueryRowContext(ctx, `SELECT id FROM campaign_rolls WHERE campaign_id = ? AND origin = ? AND origin_id = ?`,
			x.CampaignID, x.Origin, x.OriginID).Scan(&id); err != nil {
			return "", false, apperror.NewInternal(fmt.Errorf("find recorded roll: %w", err))
		}
		return id, true, nil
	}
	if err != nil {
		return "", false, apperror.NewInternal(fmt.Errorf("insert roll: %w", err))
	}
	return x.ID, false, nil
}

// visibleTo is the audience rule as SQL, mirroring Service's audience: public
// rolls are everyone's; a Director also sees Director-only and blind rolls;
// the roller sees their own rolls of any mode (a blind one as a stub, which
// the service makes). Nobody else ever sees someone's self roll.
const visibleTo = `(mode = 'public' OR roller_id = ? OR (? AND mode IN ('gm', 'blind')))`

func (r *repository) List(ctx context.Context, campaignID string, v Viewer, before string, limit int) ([]Roll, error) {
	args := []any{campaignID, v.UserID, v.Director}
	q := `SELECT id, origin, COALESCE(origin_id, ''), source, COALESCE(roller_id, ''), roller_name, label, formula, dice,
		modifier, total, outcome, why, from_where, mode, created_at
		FROM campaign_rolls WHERE campaign_id = ? AND ` + visibleTo
	if before != "" {
		q += ` AND (created_at, id) < (SELECT created_at, id FROM campaign_rolls WHERE campaign_id = ? AND id = ?)`
		args = append(args, campaignID, before)
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("list rolls: %w", err))
	}
	defer func() { _ = rows.Close() }()
	var out []Roll
	for rows.Next() {
		x := Roll{CampaignID: campaignID}
		var dice string
		if err := rows.Scan(&x.ID, &x.Origin, &x.OriginID, &x.Source, &x.RollerID, &x.RollerName, &x.Label, &x.Formula, &dice,
			&x.Modifier, &x.Total, &x.Outcome, &x.Why, &x.From, &x.Mode, &x.CreatedAt); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scan roll: %w", err))
		}
		// A row whose dice can't be read still shows its total.
		_ = json.Unmarshal([]byte(dice), &x.Dice)
		x.CreatedAt = x.CreatedAt.UTC()
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("list rolls: %w", err))
	}
	return out, nil
}

func (r *repository) Look(ctx context.Context, campaignID string) (string, error) {
	var look string
	err := r.db.QueryRowContext(ctx, `SELECT look FROM campaign_roll_settings WHERE campaign_id = ?`, campaignID).Scan(&look)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", apperror.NewInternal(fmt.Errorf("read roll settings: %w", err))
	}
	return look, nil
}

func (r *repository) SetLook(ctx context.Context, campaignID, look string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO campaign_roll_settings (campaign_id, look, updated_at) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE look = VALUES(look), updated_at = VALUES(updated_at)`, campaignID, look, time.Now().UTC())
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("save roll settings: %w", err))
	}
	return nil
}
