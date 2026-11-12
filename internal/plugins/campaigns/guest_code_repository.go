package campaigns

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// GuestCodeRepository stores guest codes, keyed by the code's hash.
type GuestCodeRepository interface {
	Create(ctx context.Context, gc *GuestCode, codeHash string) error
	ListByCampaign(ctx context.Context, campaignID string, since time.Time) ([]GuestCode, error)
	CountLive(ctx context.Context, campaignID string, now time.Time) (int, error)
	DeleteUnused(ctx context.Context, campaignID, id string) error
	// Claim marks a live code used and returns it; a code that is unknown,
	// used or expired is NotFound.
	Claim(ctx context.Context, codeHash string, now time.Time) (id, campaignID string, err error)
	Release(ctx context.Context, id string) error
	SetUsedBy(ctx context.Context, id, userID string) error
	// FindByID loads one of a campaign's codes; another campaign's is NotFound.
	FindByID(ctx context.Context, campaignID, id string) (*GuestCode, error)
	// VisitDays is the code's look-around length, zero for a player's code.
	VisitDays(ctx context.Context, id string) (int, error)
	// StartVisit records who used a look-around pass and when it ends.
	StartVisit(ctx context.Context, id, userID string, until time.Time) error
	// FindVisit is when the user's latest look-around pass for the campaign
	// ends, or nil when they have none.
	FindVisit(ctx context.Context, campaignID, userID string) (*time.Time, error)
	// EndVisit moves a running visit's end to now; an ended one is NotFound.
	EndVisit(ctx context.Context, campaignID, id string, now time.Time) error
	// ClearVisit removes the pass's end, once its visitor is a player.
	ClearVisit(ctx context.Context, id string) error
	// MoveMembership hands a guest's place in a campaign to another account,
	// keeping that account's role when it is already a member.
	MoveMembership(ctx context.Context, campaignID, fromUserID, toUserID string) error
}

type guestCodeRepository struct{ db *sql.DB }

// NewGuestCodeRepository returns the MariaDB guest code store.
func NewGuestCodeRepository(db *sql.DB) GuestCodeRepository {
	return &guestCodeRepository{db: db}
}

// Create inserts a code. Only its hash is stored.
func (r *guestCodeRepository) Create(ctx context.Context, gc *GuestCode, codeHash string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_guest_codes (id, campaign_id, code_hash, note, created_by, expires_at, created_at, visit_days)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		gc.ID, gc.CampaignID, codeHash, gc.Note, gc.CreatedBy, gc.ExpiresAt, gc.CreatedAt, nullVisitDays(gc.VisitDays))
	if err != nil {
		return fmt.Errorf("inserting guest code: %w", err)
	}
	return nil
}

// ListByCampaign lists codes made since a time, newest first, with the name
// of whoever used each.
func (r *guestCodeRepository) ListByCampaign(ctx context.Context, campaignID string, since time.Time) ([]GuestCode, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+guestCodeColumns+`
		   FROM campaign_guest_codes g
		   LEFT JOIN users u ON u.id = g.used_by
		  WHERE g.campaign_id = ? AND g.created_at >= ?
		  ORDER BY g.created_at DESC`, campaignID, since)
	if err != nil {
		return nil, fmt.Errorf("listing guest codes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []GuestCode
	for rows.Next() {
		g, err := scanGuestCode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

// guestCodeColumns is what scanGuestCode reads, from campaign_guest_codes g
// LEFT JOIN users u on whoever used the code.
const guestCodeColumns = `g.id, g.campaign_id, g.note, g.created_by, g.expires_at, g.used_by, g.used_at, g.created_at,
		        COALESCE(u.display_name, ''), COALESCE(g.visit_days, 0), g.visit_until`

func scanGuestCode(row interface{ Scan(...any) error }) (*GuestCode, error) {
	var g GuestCode
	if err := row.Scan(&g.ID, &g.CampaignID, &g.Note, &g.CreatedBy, &g.ExpiresAt,
		&g.UsedBy, &g.UsedAt, &g.CreatedAt, &g.UsedByName, &g.VisitDays, &g.VisitUntil); err != nil {
		return nil, err
	}
	return &g, nil
}

// nullVisitDays stores a player's code as NULL, so older rows and new ones
// read the same.
func nullVisitDays(days int) any {
	if days <= 0 {
		return nil
	}
	return days
}

// FindByID loads one of the campaign's codes.
func (r *guestCodeRepository) FindByID(ctx context.Context, campaignID, id string) (*GuestCode, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+guestCodeColumns+`
		   FROM campaign_guest_codes g
		   LEFT JOIN users u ON u.id = g.used_by
		  WHERE g.id = ? AND g.campaign_id = ?`, id, campaignID)
	g, err := scanGuestCode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("that code is gone")
	}
	if err != nil {
		return nil, fmt.Errorf("reading guest code: %w", err)
	}
	return g, nil
}

// VisitDays reads a code's look-around length.
func (r *guestCodeRepository) VisitDays(ctx context.Context, id string) (int, error) {
	var days int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(visit_days, 0) FROM campaign_guest_codes WHERE id = ?`, id).Scan(&days)
	if err != nil {
		return 0, fmt.Errorf("reading guest code kind: %w", err)
	}
	return days, nil
}

// StartVisit records the visitor and the end of their visit.
func (r *guestCodeRepository) StartVisit(ctx context.Context, id, userID string, until time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE campaign_guest_codes SET used_by = ?, visit_until = ? WHERE id = ?`, userID, until, id)
	if err != nil {
		return fmt.Errorf("starting visit: %w", err)
	}
	return nil
}

// FindVisit takes the latest end, so a second pass for the same person
// extends their visit rather than being shadowed by an older one.
func (r *guestCodeRepository) FindVisit(ctx context.Context, campaignID, userID string) (*time.Time, error) {
	var until sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(visit_until) FROM campaign_guest_codes
		  WHERE campaign_id = ? AND used_by = ? AND visit_until IS NOT NULL`, campaignID, userID).Scan(&until)
	if err != nil {
		return nil, fmt.Errorf("reading visit: %w", err)
	}
	if !until.Valid {
		return nil, nil
	}
	return &until.Time, nil
}

// EndVisit ends a running visit now.
func (r *guestCodeRepository) EndVisit(ctx context.Context, campaignID, id string, now time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE campaign_guest_codes SET visit_until = ?
		  WHERE id = ? AND campaign_id = ? AND visit_until > ?`, now, id, campaignID, now)
	if err != nil {
		return fmt.Errorf("ending visit: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("that visit has already ended")
	}
	return nil
}

// ClearVisit removes a pass's end once its visitor became a player.
func (r *guestCodeRepository) ClearVisit(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE campaign_guest_codes SET visit_until = NULL WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("clearing visit: %w", err)
	}
	return nil
}

// CountLive counts a campaign's unused, unexpired codes.
func (r *guestCodeRepository) CountLive(ctx context.Context, campaignID string, now time.Time) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM campaign_guest_codes
		  WHERE campaign_id = ? AND used_at IS NULL AND expires_at > ?`, campaignID, now).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting guest codes: %w", err)
	}
	return n, nil
}

// DeleteUnused removes a code nobody has used. A used code stays as the
// record of who joined with it.
func (r *guestCodeRepository) DeleteUnused(ctx context.Context, campaignID, id string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM campaign_guest_codes WHERE id = ? AND campaign_id = ? AND used_at IS NULL`, id, campaignID)
	if err != nil {
		return fmt.Errorf("deleting guest code: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("that code is already used or gone")
	}
	return nil
}

// Claim marks the code used in one statement, so of two people racing for
// it exactly one gets it. A campaign in the Trash takes no new guests.
func (r *guestCodeRepository) Claim(ctx context.Context, codeHash string, now time.Time) (string, string, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE campaign_guest_codes SET used_at = ?
		  WHERE code_hash = ? AND used_at IS NULL AND expires_at > ?
		    AND campaign_id IN (SELECT id FROM campaigns WHERE deleted_at IS NULL)`, now, codeHash, now)
	if err != nil {
		return "", "", fmt.Errorf("claiming guest code: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", "", apperror.NewNotFound("that code isn't right, or it was already used or has expired")
	}
	var id, campaignID string
	err = r.db.QueryRowContext(ctx,
		`SELECT id, campaign_id FROM campaign_guest_codes WHERE code_hash = ?`, codeHash).Scan(&id, &campaignID)
	if err != nil {
		return "", "", fmt.Errorf("reading claimed guest code: %w", err)
	}
	return id, campaignID, nil
}

// Release makes a claimed code usable again after the join failed.
func (r *guestCodeRepository) Release(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE campaign_guest_codes SET used_at = NULL, used_by = NULL WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("releasing guest code: %w", err)
	}
	return nil
}

// SetUsedBy records who joined with a code.
func (r *guestCodeRepository) SetUsedBy(ctx context.Context, id, userID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE campaign_guest_codes SET used_by = ? WHERE id = ?`, userID, id)
	if err != nil {
		return fmt.Errorf("recording guest: %w", err)
	}
	return nil
}

// MoveMembership is safe to run twice: once the guest's row is gone there
// is nothing left to move. A guest who is only visiting has no row, but
// their pass still moves to the account.
func (r *guestCodeRepository) MoveMembership(ctx context.Context, campaignID, fromUserID, toUserID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting membership move: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var guestChar sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT character_entity_id FROM campaign_members WHERE campaign_id = ? AND user_id = ? FOR UPDATE`,
		campaignID, fromUserID).Scan(&guestChar)
	if errors.Is(err, sql.ErrNoRows) {
		if err := moveGuestCodeRecord(ctx, tx, campaignID, fromUserID, toUserID); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("reading guest membership: %w", err)
	}
	var exists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM campaign_members WHERE campaign_id = ? AND user_id = ?)`,
		campaignID, toUserID).Scan(&exists); err != nil {
		return fmt.Errorf("checking membership: %w", err)
	}
	if exists {
		// The account keeps its own role; it takes the guest's character only
		// when it has none of its own. The guest's row goes first so the
		// character is never linked twice.
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM campaign_members WHERE campaign_id = ? AND user_id = ?`, campaignID, fromUserID); err != nil {
			return fmt.Errorf("removing guest membership: %w", err)
		}
		if guestChar.Valid {
			if _, err := tx.ExecContext(ctx,
				`UPDATE campaign_members SET character_entity_id = ?
				  WHERE campaign_id = ? AND user_id = ? AND character_entity_id IS NULL`,
				guestChar.String, campaignID, toUserID); err != nil {
				return fmt.Errorf("moving character link: %w", err)
			}
		}
	} else if _, err := tx.ExecContext(ctx,
		`UPDATE campaign_members SET user_id = ? WHERE campaign_id = ? AND user_id = ?`,
		toUserID, campaignID, fromUserID); err != nil {
		return fmt.Errorf("moving membership: %w", err)
	}
	if err := moveGuestCodeRecord(ctx, tx, campaignID, fromUserID, toUserID); err != nil {
		return err
	}
	return tx.Commit()
}

// moveGuestCodeRecord hands the guest's codes, look-around passes included,
// to the account they merged into.
func moveGuestCodeRecord(ctx context.Context, tx *sql.Tx, campaignID, fromUserID, toUserID string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE campaign_guest_codes SET used_by = ? WHERE campaign_id = ? AND used_by = ?`,
		toUserID, campaignID, fromUserID); err != nil {
		return fmt.Errorf("moving guest code record: %w", err)
	}
	return nil
}
