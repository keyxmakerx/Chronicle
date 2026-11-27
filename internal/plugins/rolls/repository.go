package rolls

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Repository owns the campaign_rolls, campaign_roll_settings and
// campaign_roll_member_dice tables. Which rolls a viewer may read is decided
// in the SQL of List, so a read for a player never selects another person's
// hidden roll.
type Repository interface {
	// DicePick returns a member's stored dice pick in a campaign, or the zero
	// pick when they have none.
	DicePick(ctx context.Context, campaignID, userID string) (DicePick, error)
	// SaveDicePick stores a member's pick; the zero pick removes the row. The
	// key is campaign and user together, so the write is only ever the
	// caller's own.
	SaveDicePick(ctx context.Context, campaignID, userID string, p DicePick) error
	// Insert adds a roll. A roll whose origin id is already recorded is not
	// added again: it returns that roll's id and duplicate=true.
	Insert(ctx context.Context, r Roll) (id string, duplicate bool, err error)
	// List returns up to limit rolls the viewer may see, newest first, made
	// before the roll before (all when empty).
	List(ctx context.Context, campaignID string, v Viewer, before string, limit int) ([]Roll, error)
	// Settings returns the campaign's dice settings: the stored row, or the
	// defaults (empty names, players may pick) when none was saved. Look is ""
	// until a Director sets it.
	Settings(ctx context.Context, campaignID string) (Settings, error)
	// SaveSettings writes s, but on an existing row only the named settings
	// (keys of the Settings JSON, e.g. "diceSet"), so concurrent changes to
	// different settings both land.
	SaveSettings(ctx context.Context, campaignID string, s Settings, changed []string) error
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
	look := ""
	if x.RollerDice != nil {
		b, err := json.Marshal(x.RollerDice)
		if err != nil {
			return "", false, apperror.NewInternal(fmt.Errorf("encode roller dice: %w", err))
		}
		look = string(b)
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO campaign_rolls
		(id, campaign_id, origin, origin_id, source, roller_id, roller_name, label, formula, dice, modifier, total, outcome, why, from_where, mode, roller_dice, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		x.ID, x.CampaignID, x.Origin, nullable(x.OriginID), x.Source, nullable(x.RollerID), x.RollerName, x.Label, x.Formula,
		string(dice), x.Modifier, x.Total, x.Outcome, x.Why, x.From, x.Mode, look, x.CreatedAt)
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
		modifier, total, outcome, why, from_where, mode, roller_dice, created_at
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
		var dice, look string
		if err := rows.Scan(&x.ID, &x.Origin, &x.OriginID, &x.Source, &x.RollerID, &x.RollerName, &x.Label, &x.Formula, &dice,
			&x.Modifier, &x.Total, &x.Outcome, &x.Why, &x.From, &x.Mode, &look, &x.CreatedAt); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scan roll: %w", err))
		}
		// A row whose dice can't be read still shows its total.
		_ = json.Unmarshal([]byte(dice), &x.Dice)
		// An unreadable look is thrown in the campaign's dice, like an old row.
		if look != "" {
			var l DiceLook
			if json.Unmarshal([]byte(look), &l) == nil {
				x.RollerDice = &l
			}
		}
		x.CreatedAt = x.CreatedAt.UTC()
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("list rolls: %w", err))
	}
	return out, nil
}

// settingsColumns maps each changeable setting to its column. SaveSettings
// builds its SQL only from this table, never from a caller's string.
var settingsColumns = map[string]string{
	"look": "look", "diceSet": "dice_set", "diceColour": "dice_colour",
	"critSucc": "crit_succ", "critFail": "crit_fail", "quality": "quality", "playersPick": "players_pick",
	"diceOffered": "dice_offered",
}

func (r *repository) Settings(ctx context.Context, campaignID string) (Settings, error) {
	s := Settings{PlayersPick: true, DiceOffered: []string{}}
	var offered string
	err := r.db.QueryRowContext(ctx, `SELECT look, dice_set, dice_colour, crit_succ, crit_fail, quality, players_pick, dice_offered
		FROM campaign_roll_settings WHERE campaign_id = ?`, campaignID).
		Scan(&s.Look, &s.DiceSet, &s.DiceColour, &s.CritSucc, &s.CritFail, &s.Quality, &s.PlayersPick, &offered)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{PlayersPick: true, DiceOffered: []string{}}, nil
	}
	if err != nil {
		return Settings{}, apperror.NewInternal(fmt.Errorf("read roll settings: %w", err))
	}
	if offered != "" {
		s.DiceOffered = strings.Split(offered, ",")
	}
	return s, nil
}

func (r *repository) SaveSettings(ctx context.Context, campaignID string, s Settings, changed []string) error {
	// The row is created from the whole merged value, but a conflict updates
	// only the columns this call changed, so two Directors changing different
	// settings at once don't undo each other.
	var updates []string
	for _, k := range changed {
		col, ok := settingsColumns[k]
		if !ok {
			return apperror.NewInternal(fmt.Errorf("save roll settings: unknown setting %q", k))
		}
		updates = append(updates, col+" = VALUES("+col+")")
	}
	if len(updates) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO campaign_roll_settings
		(campaign_id, look, dice_set, dice_colour, crit_succ, crit_fail, quality, players_pick, dice_offered, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE `+strings.Join(updates, ", ")+`, updated_at = VALUES(updated_at)`,
		campaignID, s.Look, s.DiceSet, s.DiceColour, s.CritSucc, s.CritFail, s.Quality, s.PlayersPick, strings.Join(s.DiceOffered, ","), time.Now().UTC())
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("save roll settings: %w", err))
	}
	return nil
}

func (r *repository) DicePick(ctx context.Context, campaignID, userID string) (DicePick, error) {
	var p DicePick
	err := r.db.QueryRowContext(ctx, `SELECT dice_set, dice_colour, crit_succ, crit_fail
		FROM campaign_roll_member_dice WHERE campaign_id = ? AND user_id = ?`, campaignID, userID).
		Scan(&p.Set, &p.Colour, &p.CritSucc, &p.CritFail)
	if errors.Is(err, sql.ErrNoRows) {
		return DicePick{}, nil
	}
	if err != nil {
		return DicePick{}, apperror.NewInternal(fmt.Errorf("read dice pick: %w", err))
	}
	return p, nil
}

func (r *repository) SaveDicePick(ctx context.Context, campaignID, userID string, p DicePick) error {
	if p.IsZero() {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM campaign_roll_member_dice WHERE campaign_id = ? AND user_id = ?`,
			campaignID, userID); err != nil {
			return apperror.NewInternal(fmt.Errorf("clear dice pick: %w", err))
		}
		return nil
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO campaign_roll_member_dice
		(campaign_id, user_id, dice_set, dice_colour, crit_succ, crit_fail) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE dice_set = VALUES(dice_set), dice_colour = VALUES(dice_colour),
		crit_succ = VALUES(crit_succ), crit_fail = VALUES(crit_fail)`,
		campaignID, userID, p.Set, p.Colour, p.CritSucc, p.CritFail); err != nil {
		return apperror.NewInternal(fmt.Errorf("save dice pick: %w", err))
	}
	return nil
}
