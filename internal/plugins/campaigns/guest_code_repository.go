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
		`INSERT INTO campaign_guest_codes (id, campaign_id, code_hash, note, created_by, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		gc.ID, gc.CampaignID, codeHash, gc.Note, gc.CreatedBy, gc.ExpiresAt, gc.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting guest code: %w", err)
	}
	return nil
}

// ListByCampaign lists codes made since a time, newest first, with the name
// of whoever used each.
func (r *guestCodeRepository) ListByCampaign(ctx context.Context, campaignID string, since time.Time) ([]GuestCode, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT g.id, g.campaign_id, g.note, g.created_by, g.expires_at, g.used_by, g.used_at, g.created_at,
		        COALESCE(u.display_name, '')
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
		var g GuestCode
		if err := rows.Scan(&g.ID, &g.CampaignID, &g.Note, &g.CreatedBy, &g.ExpiresAt,
			&g.UsedBy, &g.UsedAt, &g.CreatedAt, &g.UsedByName); err != nil {
			return nil, fmt.Errorf("scanning guest code: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
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
// is nothing left to move.
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
		return nil
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
	if _, err := tx.ExecContext(ctx,
		`UPDATE campaign_guest_codes SET used_by = ? WHERE campaign_id = ? AND used_by = ?`,
		toUserID, campaignID, fromUserID); err != nil {
		return fmt.Errorf("moving guest code record: %w", err)
	}
	return tx.Commit()
}
