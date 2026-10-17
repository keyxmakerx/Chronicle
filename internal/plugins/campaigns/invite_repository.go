package campaigns

import (
	"context"
	"database/sql"
	"fmt"
)

// InviteRepository handles persistence for campaign invites.
type InviteRepository interface {
	Create(ctx context.Context, invite *Invite) error
	GetByToken(ctx context.Context, token string) (*Invite, error)
	ListByCampaign(ctx context.Context, campaignID string) ([]Invite, error)
	MarkAccepted(ctx context.Context, id string) error
	// Delete removes an invite of the given campaign and reports whether a row
	// was deleted, so a foreign or unknown ID is distinguishable from success.
	Delete(ctx context.Context, campaignID, id string) (bool, error)
	DeleteExpired(ctx context.Context, campaignID string) (int64, error)
	GetByEmailAndCampaign(ctx context.Context, email, campaignID string) (*Invite, error)
}

// inviteRepository implements InviteRepository using MariaDB.
type inviteRepository struct {
	db *sql.DB
}

// NewInviteRepository creates a new invite repository.
func NewInviteRepository(db *sql.DB) InviteRepository {
	return &inviteRepository{db: db}
}

// Create inserts a new campaign invite.
func (r *inviteRepository) Create(ctx context.Context, invite *Invite) error {
	query := `INSERT INTO campaign_invites (id, campaign_id, email, role, token, created_by, expires_at)
	           VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query,
		invite.ID, invite.CampaignID, invite.Email, invite.Role,
		invite.Token, invite.CreatedBy, invite.ExpiresAt)
	if err != nil {
		return fmt.Errorf("inserting invite: %w", err)
	}
	return nil
}

// GetByToken retrieves an invite by its unique token.
func (r *inviteRepository) GetByToken(ctx context.Context, token string) (*Invite, error) {
	query := `SELECT i.id, i.campaign_id, i.email, i.role, i.token, i.created_by,
	                  i.created_at, i.expires_at, i.accepted_at
	           FROM campaign_invites i
	           WHERE i.token = ?`
	var invite Invite
	err := r.db.QueryRowContext(ctx, query, token).Scan(
		&invite.ID, &invite.CampaignID, &invite.Email, &invite.Role,
		&invite.Token, &invite.CreatedBy, &invite.CreatedAt, &invite.ExpiresAt,
		&invite.AcceptedAt)
	if err != nil {
		return nil, fmt.Errorf("fetching invite by token: %w", err)
	}
	return &invite, nil
}

// ListByCampaign returns all invites for a campaign, newest first.
// Includes the creator's display name from the users table.
func (r *inviteRepository) ListByCampaign(ctx context.Context, campaignID string) ([]Invite, error) {
	query := `SELECT i.id, i.campaign_id, i.email, i.role, i.created_by,
	                  i.created_at, i.expires_at, i.accepted_at,
	                  COALESCE(u.display_name, u.email) AS created_by_name
	           FROM campaign_invites i
	           LEFT JOIN users u ON u.id = i.created_by
	           WHERE i.campaign_id = ?
	           ORDER BY i.created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing invites: %w", err)
	}
	defer rows.Close()

	var invites []Invite
	for rows.Next() {
		var inv Invite
		if err := rows.Scan(
			&inv.ID, &inv.CampaignID, &inv.Email, &inv.Role,
			&inv.CreatedBy, &inv.CreatedAt, &inv.ExpiresAt, &inv.AcceptedAt,
			&inv.CreatedByName); err != nil {
			return nil, fmt.Errorf("scanning invite: %w", err)
		}
		invites = append(invites, inv)
	}
	return invites, rows.Err()
}

// MarkAccepted sets the accepted_at timestamp on an invite.
func (r *inviteRepository) MarkAccepted(ctx context.Context, id string) error {
	query := `UPDATE campaign_invites SET accepted_at = NOW() WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("marking invite accepted: %w", err)
	}
	return nil
}

// Delete removes an invite by ID within a campaign; the campaign condition
// keeps one campaign's owner from revoking another campaign's invite.
func (r *inviteRepository) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	query := `DELETE FROM campaign_invites WHERE id = ? AND campaign_id = ?`
	result, err := r.db.ExecContext(ctx, query, id, campaignID)
	if err != nil {
		return false, fmt.Errorf("deleting invite: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("deleting invite: %w", err)
	}
	return n > 0, nil
}

// DeleteExpired removes all expired invites for a campaign.
func (r *inviteRepository) DeleteExpired(ctx context.Context, campaignID string) (int64, error) {
	query := `DELETE FROM campaign_invites WHERE campaign_id = ? AND expires_at < NOW() AND accepted_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, campaignID)
	if err != nil {
		return 0, fmt.Errorf("deleting expired invites: %w", err)
	}
	return result.RowsAffected()
}

// GetByEmailAndCampaign retrieves a pending invite for a specific email and campaign.
func (r *inviteRepository) GetByEmailAndCampaign(ctx context.Context, email, campaignID string) (*Invite, error) {
	query := `SELECT i.id, i.campaign_id, i.email, i.role, i.token, i.created_by,
	                  i.created_at, i.expires_at, i.accepted_at
	           FROM campaign_invites i
	           WHERE i.email = ? AND i.campaign_id = ? AND i.accepted_at IS NULL AND i.expires_at > NOW()
	           ORDER BY i.created_at DESC LIMIT 1`
	var invite Invite
	err := r.db.QueryRowContext(ctx, query, email, campaignID).Scan(
		&invite.ID, &invite.CampaignID, &invite.Email, &invite.Role,
		&invite.Token, &invite.CreatedBy, &invite.CreatedAt, &invite.ExpiresAt,
		&invite.AcceptedAt)
	if err != nil {
		return nil, fmt.Errorf("fetching invite by email: %w", err)
	}
	return &invite, nil
}
