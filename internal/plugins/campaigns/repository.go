package campaigns

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// CampaignRepository defines the data access contract for campaign operations.
// All SQL lives in the concrete implementation -- no SQL leaks out.
type CampaignRepository interface {
	// Campaign CRUD
	Create(ctx context.Context, campaign *Campaign) error
	FindByID(ctx context.Context, id string) (*Campaign, error)
	FindBySlug(ctx context.Context, slug string) (*Campaign, error)
	ListByUser(ctx context.Context, userID string, opts ListOptions) ([]Campaign, int, error)
	ListAll(ctx context.Context, opts ListOptions) ([]Campaign, int, error)
	ListPublic(ctx context.Context, limit int) ([]Campaign, error)
	Update(ctx context.Context, campaign *Campaign) error
	Delete(ctx context.Context, id string) error
	SlugExists(ctx context.Context, slug string) (bool, error)
	CountAll(ctx context.Context) (int, error)

	// Membership
	AddMember(ctx context.Context, member *CampaignMember) error
	RemoveMember(ctx context.Context, campaignID, userID string) error
	FindMember(ctx context.Context, campaignID, userID string) (*CampaignMember, error)
	ListMembers(ctx context.Context, campaignID string) ([]CampaignMember, error)
	UpdateMemberRole(ctx context.Context, campaignID, userID string, role Role) error
	UpdateMemberCharacter(ctx context.Context, campaignID, userID string, characterEntityID *string) error
	FindOwnerMember(ctx context.Context, campaignID string) (*CampaignMember, error)

	// Ownership transfer
	CreateTransfer(ctx context.Context, transfer *OwnershipTransfer) error
	FindTransferByToken(ctx context.Context, token string) (*OwnershipTransfer, error)
	FindTransferByCampaign(ctx context.Context, campaignID string) (*OwnershipTransfer, error)
	DeleteTransfer(ctx context.Context, id string) error

	// UpdateBackdropPath updates only the backdrop_path column.
	UpdateBackdropPath(ctx context.Context, campaignID string, path *string) error

	// UpdateSettings updates only the settings JSON column.
	UpdateSettings(ctx context.Context, campaignID, settingsJSON string) error

	// UpdateSidebarConfig updates only the sidebar_config JSON column.
	UpdateSidebarConfig(ctx context.Context, campaignID, configJSON string) error

	// UpdateDashboardLayout updates only the dashboard_layout JSON column.
	// Pass nil to revert to the hardcoded default dashboard.
	UpdateDashboardLayout(ctx context.Context, campaignID string, layoutJSON *string) error

	// UpdateOwnerDashboardLayout updates only the owner_dashboard_layout JSON column.
	// Pass nil to revert to the hardcoded default owner dashboard.
	UpdateOwnerDashboardLayout(ctx context.Context, campaignID string, layoutJSON *string) error

	// TransferOwnership atomically transfers campaign ownership from one user
	// to another within a database transaction.
	TransferOwnership(ctx context.Context, campaignID, fromUserID, toUserID string) error

	// ForceTransferOwnership is used by admins to take ownership. Atomically
	// demotes current owner to Scribe and sets the new owner.
	ForceTransferOwnership(ctx context.Context, campaignID, newOwnerID string) error

	// ArchiveCampaign sets the archived_at timestamp, making the campaign read-only.
	ArchiveCampaign(ctx context.Context, campaignID string) error

	// UnarchiveCampaign clears the archived_at timestamp, restoring write access.
	UnarchiveCampaign(ctx context.Context, campaignID string) error

	// SetJoinCode stores a shareable invite code for the campaign.
	SetJoinCode(ctx context.Context, campaignID, code string) error

	// ClearJoinCode removes the shareable invite code.
	ClearJoinCode(ctx context.Context, campaignID string) error

	// FindByJoinCode looks up a campaign by its shareable invite code.
	FindByJoinCode(ctx context.Context, code string) (*Campaign, error)
}

// campaignRepository implements CampaignRepository with MariaDB queries.
type campaignRepository struct {
	db *sql.DB
}

// NewCampaignRepository creates a new repository backed by the given DB pool.
func NewCampaignRepository(db *sql.DB) CampaignRepository {
	return &campaignRepository{db: db}
}

// --- Campaign CRUD ---

// Create inserts a new campaign row.
func (r *campaignRepository) Create(ctx context.Context, campaign *Campaign) error {
	query := `INSERT INTO campaigns (id, name, slug, description, is_public, settings, backdrop_path, sidebar_config, dashboard_layout, owner_dashboard_layout, created_by, created_at, updated_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		campaign.ID, campaign.Name, campaign.Slug, campaign.Description, campaign.IsPublic,
		campaign.Settings, campaign.BackdropPath, campaign.SidebarConfig, campaign.DashboardLayout, campaign.OwnerDashboardLayout,
		campaign.CreatedBy, campaign.CreatedAt, campaign.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting campaign: %w", err)
	}
	return nil
}

// FindByID retrieves a campaign by its UUID.
func (r *campaignRepository) FindByID(ctx context.Context, id string) (*Campaign, error) {
	query := `SELECT id, name, slug, description, is_public, settings, backdrop_path, sidebar_config, dashboard_layout, owner_dashboard_layout, created_by, created_at, updated_at, archived_at, join_code
	          FROM campaigns WHERE id = ?`

	c := &Campaign{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsPublic,
		&c.Settings, &c.BackdropPath, &c.SidebarConfig, &c.DashboardLayout, &c.OwnerDashboardLayout,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.JoinCode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("campaign not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying campaign by id: %w", err)
	}
	return c, nil
}

// FindBySlug retrieves a campaign by its URL slug.
func (r *campaignRepository) FindBySlug(ctx context.Context, slug string) (*Campaign, error) {
	query := `SELECT id, name, slug, description, is_public, settings, backdrop_path, sidebar_config, dashboard_layout, owner_dashboard_layout, created_by, created_at, updated_at, archived_at, join_code
	          FROM campaigns WHERE slug = ?`

	c := &Campaign{}
	err := r.db.QueryRowContext(ctx, query, slug).Scan(
		&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsPublic,
		&c.Settings, &c.BackdropPath, &c.SidebarConfig, &c.DashboardLayout, &c.OwnerDashboardLayout,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.JoinCode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("campaign not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying campaign by slug: %w", err)
	}
	return c, nil
}

// ListByUser returns campaigns the user is a member of, ordered by most
// recently updated. Returns the campaigns and total count for pagination.
func (r *campaignRepository) ListByUser(ctx context.Context, userID string, opts ListOptions) ([]Campaign, int, error) {
	// Count total for pagination.
	countQuery := `SELECT COUNT(*) FROM campaigns c
	               INNER JOIN campaign_members cm ON cm.campaign_id = c.id
	               WHERE cm.user_id = ? AND c.archived_at IS NULL`
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting user campaigns: %w", err)
	}

	query := `SELECT c.id, c.name, c.slug, c.description, c.is_public,
	                 c.settings, c.backdrop_path, c.sidebar_config, c.dashboard_layout,
	                 c.owner_dashboard_layout, c.created_by, c.created_at, c.updated_at,
	                 c.archived_at, c.join_code
	          FROM campaigns c
	          INNER JOIN campaign_members cm ON cm.campaign_id = c.id
	          WHERE cm.user_id = ? AND c.archived_at IS NULL
	          ORDER BY c.updated_at DESC
	          LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, userID, opts.PerPage, opts.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listing user campaigns: %w", err)
	}
	defer rows.Close()

	var campaigns []Campaign
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsPublic,
			&c.Settings, &c.BackdropPath, &c.SidebarConfig, &c.DashboardLayout, &c.OwnerDashboardLayout,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.JoinCode,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning campaign row: %w", err)
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, total, rows.Err()
}

// ListAll returns all campaigns ordered by most recently updated. Admin only.
func (r *campaignRepository) ListAll(ctx context.Context, opts ListOptions) ([]Campaign, int, error) {
	countQuery := `SELECT COUNT(*) FROM campaigns`
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting all campaigns: %w", err)
	}

	query := `SELECT id, name, slug, description, is_public, settings, backdrop_path, sidebar_config, dashboard_layout, owner_dashboard_layout, created_by, created_at, updated_at, archived_at, join_code
	          FROM campaigns ORDER BY updated_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, opts.PerPage, opts.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("listing all campaigns: %w", err)
	}
	defer rows.Close()

	var campaigns []Campaign
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsPublic,
			&c.Settings, &c.BackdropPath, &c.SidebarConfig, &c.DashboardLayout, &c.OwnerDashboardLayout,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.JoinCode,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning campaign row: %w", err)
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, total, rows.Err()
}

// ListPublic returns public campaigns ordered by most recently updated.
// Used for the public landing page to showcase discoverable campaigns.
func (r *campaignRepository) ListPublic(ctx context.Context, limit int) ([]Campaign, error) {
	query := `SELECT id, name, slug, description, is_public, settings, backdrop_path, sidebar_config, dashboard_layout, owner_dashboard_layout, created_by, created_at, updated_at, archived_at, join_code
	          FROM campaigns WHERE is_public = 1 AND archived_at IS NULL
	          ORDER BY updated_at DESC LIMIT ?`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("listing public campaigns: %w", err)
	}
	defer rows.Close()

	var campaigns []Campaign
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsPublic,
			&c.Settings, &c.BackdropPath, &c.SidebarConfig, &c.DashboardLayout, &c.OwnerDashboardLayout,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.JoinCode,
		); err != nil {
			return nil, fmt.Errorf("scanning public campaign row: %w", err)
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, rows.Err()
}

// Update modifies an existing campaign's name, description, settings, and sidebar config.
func (r *campaignRepository) Update(ctx context.Context, campaign *Campaign) error {
	query := `UPDATE campaigns SET name = ?, slug = ?, description = ?, is_public = ?,
	          settings = ?, sidebar_config = ?, updated_at = NOW()
	          WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		campaign.Name, campaign.Slug, campaign.Description, campaign.IsPublic,
		campaign.Settings, campaign.SidebarConfig, campaign.ID,
	)
	if err != nil {
		return fmt.Errorf("updating campaign: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}

// Delete removes a campaign. FK CASCADE handles member and transfer cleanup.
func (r *campaignRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM campaigns WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting campaign: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}

// SlugExists returns true if a campaign with the given slug already exists.
func (r *campaignRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM campaigns WHERE slug = ?)`, slug,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking slug existence: %w", err)
	}
	return exists, nil
}

// CountAll returns the total number of campaigns. Used for admin dashboard.
func (r *campaignRepository) CountAll(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaigns`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting campaigns: %w", err)
	}
	return count, nil
}

// UpdateBackdropPath updates only the backdrop_path for a campaign.
func (r *campaignRepository) UpdateBackdropPath(ctx context.Context, campaignID string, path *string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaigns SET backdrop_path = ?, updated_at = NOW() WHERE id = ?`,
		path, campaignID,
	)
	if err != nil {
		return fmt.Errorf("updating backdrop path: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}

// UpdateSettings updates only the settings JSON for a campaign.
func (r *campaignRepository) UpdateSettings(ctx context.Context, campaignID, settingsJSON string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaigns SET settings = ?, updated_at = NOW() WHERE id = ?`,
		settingsJSON, campaignID,
	)
	if err != nil {
		return fmt.Errorf("updating settings: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}

// UpdateSidebarConfig updates only the sidebar_config JSON for a campaign.
func (r *campaignRepository) UpdateSidebarConfig(ctx context.Context, campaignID, configJSON string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaigns SET sidebar_config = ?, updated_at = NOW() WHERE id = ?`,
		configJSON, campaignID,
	)
	if err != nil {
		return fmt.Errorf("updating sidebar config: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}

// UpdateDashboardLayout updates only the dashboard_layout JSON for a campaign.
// Pass nil to revert to the hardcoded default dashboard.
func (r *campaignRepository) UpdateDashboardLayout(ctx context.Context, campaignID string, layoutJSON *string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaigns SET dashboard_layout = ?, updated_at = NOW() WHERE id = ?`,
		layoutJSON, campaignID,
	)
	if err != nil {
		return fmt.Errorf("updating dashboard layout: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}

// UpdateOwnerDashboardLayout updates only the owner_dashboard_layout JSON for a campaign.
// Pass nil to revert to the hardcoded default owner dashboard.
func (r *campaignRepository) UpdateOwnerDashboardLayout(ctx context.Context, campaignID string, layoutJSON *string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaigns SET owner_dashboard_layout = ?, updated_at = NOW() WHERE id = ?`,
		layoutJSON, campaignID,
	)
	if err != nil {
		return fmt.Errorf("updating owner dashboard layout: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}

// --- Archive & Join Code ---

// ArchiveCampaign sets the archived_at timestamp, making the campaign read-only.
func (r *campaignRepository) ArchiveCampaign(ctx context.Context, campaignID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaigns SET archived_at = NOW(), updated_at = NOW() WHERE id = ? AND archived_at IS NULL`,
		campaignID,
	)
	if err != nil {
		return fmt.Errorf("archiving campaign: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found or already archived")
	}
	return nil
}

// UnarchiveCampaign clears the archived_at timestamp, restoring write access.
func (r *campaignRepository) UnarchiveCampaign(ctx context.Context, campaignID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaigns SET archived_at = NULL, updated_at = NOW() WHERE id = ? AND archived_at IS NOT NULL`,
		campaignID,
	)
	if err != nil {
		return fmt.Errorf("unarchiving campaign: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found or not archived")
	}
	return nil
}

// SetJoinCode stores a shareable invite code for the campaign.
func (r *campaignRepository) SetJoinCode(ctx context.Context, campaignID, code string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaigns SET join_code = ?, updated_at = NOW() WHERE id = ?`,
		code, campaignID,
	)
	if err != nil {
		return fmt.Errorf("setting join code: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}

// ClearJoinCode removes the shareable invite code.
func (r *campaignRepository) ClearJoinCode(ctx context.Context, campaignID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaigns SET join_code = NULL, updated_at = NOW() WHERE id = ?`,
		campaignID,
	)
	if err != nil {
		return fmt.Errorf("clearing join code: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}

// FindByJoinCode looks up a campaign by its shareable invite code.
func (r *campaignRepository) FindByJoinCode(ctx context.Context, code string) (*Campaign, error) {
	query := `SELECT id, name, slug, description, is_public, settings, backdrop_path, sidebar_config, dashboard_layout, owner_dashboard_layout, created_by, created_at, updated_at, archived_at, join_code
	          FROM campaigns WHERE join_code = ?`

	c := &Campaign{}
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsPublic,
		&c.Settings, &c.BackdropPath, &c.SidebarConfig, &c.DashboardLayout, &c.OwnerDashboardLayout,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.JoinCode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("invalid or expired invite code")
	}
	if err != nil {
		return nil, fmt.Errorf("querying campaign by join code: %w", err)
	}
	return c, nil
}

// --- Membership ---

// AddMember inserts a new campaign membership row.
func (r *campaignRepository) AddMember(ctx context.Context, member *CampaignMember) error {
	query := `INSERT INTO campaign_members (campaign_id, user_id, role, joined_at)
	          VALUES (?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		member.CampaignID, member.UserID, member.Role.String(), member.JoinedAt,
	)
	if err != nil {
		return fmt.Errorf("adding campaign member: %w", err)
	}
	return nil
}

// RemoveMember deletes a campaign membership row.
func (r *campaignRepository) RemoveMember(ctx context.Context, campaignID, userID string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM campaign_members WHERE campaign_id = ? AND user_id = ?`,
		campaignID, userID,
	)
	if err != nil {
		return fmt.Errorf("removing campaign member: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("member not found")
	}
	return nil
}

// FindMember retrieves a user's membership with their display info.
func (r *campaignRepository) FindMember(ctx context.Context, campaignID, userID string) (*CampaignMember, error) {
	query := `SELECT cm.campaign_id, cm.user_id, cm.role, cm.character_entity_id, cm.joined_at,
	                 u.display_name, u.email, u.avatar_path
	          FROM campaign_members cm
	          INNER JOIN users u ON u.id = cm.user_id
	          WHERE cm.campaign_id = ? AND cm.user_id = ?`

	m := &CampaignMember{}
	var roleStr string
	err := r.db.QueryRowContext(ctx, query, campaignID, userID).Scan(
		&m.CampaignID, &m.UserID, &roleStr, &m.CharacterEntityID, &m.JoinedAt,
		&m.DisplayName, &m.Email, &m.AvatarPath,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("member not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding campaign member: %w", err)
	}
	m.Role = RoleFromString(roleStr)
	return m, nil
}

// ListMembers returns all members of a campaign with their display info.
func (r *campaignRepository) ListMembers(ctx context.Context, campaignID string) ([]CampaignMember, error) {
	query := `SELECT cm.campaign_id, cm.user_id, cm.role, cm.character_entity_id, cm.joined_at,
	                 u.display_name, u.email, u.avatar_path,
	                 e.name
	          FROM campaign_members cm
	          INNER JOIN users u ON u.id = cm.user_id
	          LEFT JOIN entities e ON e.id = cm.character_entity_id
	          WHERE cm.campaign_id = ?
	          ORDER BY FIELD(cm.role, 'owner', 'scribe', 'player'), u.display_name`

	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing campaign members: %w", err)
	}
	defer rows.Close()

	var members []CampaignMember
	for rows.Next() {
		var m CampaignMember
		var roleStr string
		if err := rows.Scan(
			&m.CampaignID, &m.UserID, &roleStr, &m.CharacterEntityID, &m.JoinedAt,
			&m.DisplayName, &m.Email, &m.AvatarPath,
			&m.CharacterName,
		); err != nil {
			return nil, fmt.Errorf("scanning member row: %w", err)
		}
		m.Role = RoleFromString(roleStr)
		members = append(members, m)
	}
	return members, rows.Err()
}

// UpdateMemberRole changes a member's role within a campaign.
func (r *campaignRepository) UpdateMemberRole(ctx context.Context, campaignID, userID string, role Role) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_members SET role = ? WHERE campaign_id = ? AND user_id = ?`,
		role.String(), campaignID, userID,
	)
	if err != nil {
		return fmt.Errorf("updating member role: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("member not found")
	}
	return nil
}

// UpdateMemberCharacter sets or clears the character entity assignment for a member.
func (r *campaignRepository) UpdateMemberCharacter(ctx context.Context, campaignID, userID string, characterEntityID *string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_members SET character_entity_id = ? WHERE campaign_id = ? AND user_id = ?`,
		characterEntityID, campaignID, userID,
	)
	if err != nil {
		return fmt.Errorf("updating member character: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("member not found")
	}
	return nil
}

// FindOwnerMember returns the member with role='owner' for a campaign.
func (r *campaignRepository) FindOwnerMember(ctx context.Context, campaignID string) (*CampaignMember, error) {
	query := `SELECT cm.campaign_id, cm.user_id, cm.role, cm.character_entity_id, cm.joined_at,
	                 u.display_name, u.email, u.avatar_path
	          FROM campaign_members cm
	          INNER JOIN users u ON u.id = cm.user_id
	          WHERE cm.campaign_id = ? AND cm.role = 'owner'`

	m := &CampaignMember{}
	var roleStr string
	err := r.db.QueryRowContext(ctx, query, campaignID).Scan(
		&m.CampaignID, &m.UserID, &roleStr, &m.CharacterEntityID, &m.JoinedAt,
		&m.DisplayName, &m.Email, &m.AvatarPath,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("owner not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding campaign owner: %w", err)
	}
	m.Role = RoleFromString(roleStr)
	return m, nil
}

// --- Ownership Transfer ---

// CreateTransfer inserts a new pending ownership transfer.
func (r *campaignRepository) CreateTransfer(ctx context.Context, transfer *OwnershipTransfer) error {
	query := `INSERT INTO ownership_transfers (id, campaign_id, from_user_id, to_user_id, token, expires_at, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		transfer.ID, transfer.CampaignID, transfer.FromUserID,
		transfer.ToUserID, transfer.Token, transfer.ExpiresAt, transfer.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating ownership transfer: %w", err)
	}
	return nil
}

// FindTransferByToken retrieves a pending transfer by its verification token.
func (r *campaignRepository) FindTransferByToken(ctx context.Context, token string) (*OwnershipTransfer, error) {
	query := `SELECT id, campaign_id, from_user_id, to_user_id, token, expires_at, created_at
	          FROM ownership_transfers WHERE token = ?`

	t := &OwnershipTransfer{}
	err := r.db.QueryRowContext(ctx, query, token).Scan(
		&t.ID, &t.CampaignID, &t.FromUserID, &t.ToUserID,
		&t.Token, &t.ExpiresAt, &t.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("transfer not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding transfer by token: %w", err)
	}
	return t, nil
}

// FindTransferByCampaign retrieves the pending transfer for a campaign (if any).
func (r *campaignRepository) FindTransferByCampaign(ctx context.Context, campaignID string) (*OwnershipTransfer, error) {
	query := `SELECT id, campaign_id, from_user_id, to_user_id, token, expires_at, created_at
	          FROM ownership_transfers WHERE campaign_id = ?`

	t := &OwnershipTransfer{}
	err := r.db.QueryRowContext(ctx, query, campaignID).Scan(
		&t.ID, &t.CampaignID, &t.FromUserID, &t.ToUserID,
		&t.Token, &t.ExpiresAt, &t.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // No pending transfer is not an error.
	}
	if err != nil {
		return nil, fmt.Errorf("finding transfer by campaign: %w", err)
	}
	return t, nil
}

// DeleteTransfer removes a pending transfer.
func (r *campaignRepository) DeleteTransfer(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM ownership_transfers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting transfer: %w", err)
	}
	return nil
}

// TransferOwnership atomically transfers campaign ownership from one user to
// another. Updates campaigns.created_by, swaps member roles, and removes the
// pending transfer row -- all within a single transaction.
func (r *campaignRepository) TransferOwnership(ctx context.Context, campaignID, fromUserID, toUserID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transfer tx: %w", err)
	}
	defer tx.Rollback()

	// Update campaign ownership.
	if _, err := tx.ExecContext(ctx,
		`UPDATE campaigns SET created_by = ? WHERE id = ?`,
		toUserID, campaignID,
	); err != nil {
		return fmt.Errorf("updating campaign owner: %w", err)
	}

	// Demote old owner to Scribe.
	if _, err := tx.ExecContext(ctx,
		`UPDATE campaign_members SET role = 'scribe' WHERE campaign_id = ? AND user_id = ?`,
		campaignID, fromUserID,
	); err != nil {
		return fmt.Errorf("demoting old owner: %w", err)
	}

	// Promote new owner. If they're already a member, update their role.
	// If not, insert a new membership row.
	res, err := tx.ExecContext(ctx,
		`UPDATE campaign_members SET role = 'owner' WHERE campaign_id = ? AND user_id = ?`,
		campaignID, toUserID,
	)
	if err != nil {
		return fmt.Errorf("promoting new owner: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// New owner wasn't a member -- add them.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO campaign_members (campaign_id, user_id, role, joined_at) VALUES (?, ?, 'owner', NOW())`,
			campaignID, toUserID,
		); err != nil {
			return fmt.Errorf("inserting new owner as member: %w", err)
		}
	}

	// Remove the pending transfer row.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM ownership_transfers WHERE campaign_id = ?`,
		campaignID,
	); err != nil {
		return fmt.Errorf("cleaning up transfer: %w", err)
	}

	return tx.Commit()
}

// ForceTransferOwnership is used by admins to take ownership of a campaign.
// Atomically demotes the current owner to Scribe and sets the admin as the
// new owner. If the admin is already a member, their role is updated; if not,
// a new membership row is created.
func (r *campaignRepository) ForceTransferOwnership(ctx context.Context, campaignID, newOwnerID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning force transfer tx: %w", err)
	}
	defer tx.Rollback()

	// Demote current owner to Scribe.
	if _, err := tx.ExecContext(ctx,
		`UPDATE campaign_members SET role = 'scribe' WHERE campaign_id = ? AND role = 'owner'`,
		campaignID,
	); err != nil {
		return fmt.Errorf("demoting current owner: %w", err)
	}

	// Set new owner on campaign.
	if _, err := tx.ExecContext(ctx,
		`UPDATE campaigns SET created_by = ? WHERE id = ?`,
		newOwnerID, campaignID,
	); err != nil {
		return fmt.Errorf("updating campaign created_by: %w", err)
	}

	// Upsert the new owner's membership.
	res, err := tx.ExecContext(ctx,
		`UPDATE campaign_members SET role = 'owner' WHERE campaign_id = ? AND user_id = ?`,
		campaignID, newOwnerID,
	)
	if err != nil {
		return fmt.Errorf("setting new owner role: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO campaign_members (campaign_id, user_id, role, joined_at) VALUES (?, ?, 'owner', NOW())`,
			campaignID, newOwnerID,
		); err != nil {
			return fmt.Errorf("inserting new owner member: %w", err)
		}
	}

	// Clean up any pending transfers since ownership was forcefully changed.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM ownership_transfers WHERE campaign_id = ?`,
		campaignID,
	); err != nil {
		return fmt.Errorf("cleaning up transfers: %w", err)
	}

	return tx.Commit()
}

// --- Campaign Group Repository ---

// GroupRepository manages campaign groups and their memberships.
type GroupRepository interface {
	// CreateGroup creates a new campaign group. Returns the created group with its ID.
	CreateGroup(ctx context.Context, campaignID, name string, description *string) (*CampaignGroup, error)
	// ListGroups returns all groups for a campaign, ordered by name.
	ListGroups(ctx context.Context, campaignID string) ([]CampaignGroup, error)
	// GetGroup returns a single group by ID.
	GetGroup(ctx context.Context, groupID int) (*CampaignGroup, error)
	// UpdateGroup updates a group's name and description.
	UpdateGroup(ctx context.Context, groupID int, name string, description *string) error
	// DeleteGroup deletes a group and its memberships (cascade).
	DeleteGroup(ctx context.Context, groupID int) error
	// AddGroupMember adds a user to a group.
	AddGroupMember(ctx context.Context, groupID int, userID string) error
	// RemoveGroupMember removes a user from a group.
	RemoveGroupMember(ctx context.Context, groupID int, userID string) error
	// ListGroupMembers returns all members of a group with display info.
	ListGroupMembers(ctx context.Context, groupID int) ([]GroupMemberInfo, error)
}

// groupRepository implements GroupRepository with MariaDB queries.
type groupRepository struct {
	db *sql.DB
}

// NewGroupRepository creates a new group repository backed by the given DB pool.
func NewGroupRepository(db *sql.DB) GroupRepository {
	return &groupRepository{db: db}
}

// CreateGroup inserts a new group and returns it with the auto-generated ID.
func (r *groupRepository) CreateGroup(ctx context.Context, campaignID, name string, description *string) (*CampaignGroup, error) {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_groups (campaign_id, name, description) VALUES (?, ?, ?)`,
		campaignID, name, description,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting campaign group: %w", err)
	}

	id, _ := result.LastInsertId()
	return &CampaignGroup{
		ID:          int(id),
		CampaignID:  campaignID,
		Name:        name,
		Description: description,
	}, nil
}

// ListGroups returns all groups for a campaign, ordered by name.
func (r *groupRepository) ListGroups(ctx context.Context, campaignID string) ([]CampaignGroup, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, campaign_id, name, description, created_at, updated_at
		 FROM campaign_groups WHERE campaign_id = ? ORDER BY name`,
		campaignID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing campaign groups: %w", err)
	}
	defer rows.Close()

	var groups []CampaignGroup
	for rows.Next() {
		var g CampaignGroup
		if err := rows.Scan(&g.ID, &g.CampaignID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning campaign group: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GetGroup returns a single group by ID.
func (r *groupRepository) GetGroup(ctx context.Context, groupID int) (*CampaignGroup, error) {
	var g CampaignGroup
	err := r.db.QueryRowContext(ctx,
		`SELECT id, campaign_id, name, description, created_at, updated_at
		 FROM campaign_groups WHERE id = ?`, groupID,
	).Scan(&g.ID, &g.CampaignID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("finding campaign group: %w", err)
	}
	return &g, nil
}

// UpdateGroup updates a group's name and description.
func (r *groupRepository) UpdateGroup(ctx context.Context, groupID int, name string, description *string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_groups SET name = ?, description = ? WHERE id = ?`,
		name, description, groupID,
	)
	if err != nil {
		return fmt.Errorf("updating campaign group: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("group not found")
	}
	return nil
}

// DeleteGroup deletes a group. Memberships cascade via FK.
func (r *groupRepository) DeleteGroup(ctx context.Context, groupID int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM campaign_groups WHERE id = ?`, groupID)
	if err != nil {
		return fmt.Errorf("deleting campaign group: %w", err)
	}
	return nil
}

// AddGroupMember adds a user to a group. Ignores duplicates.
func (r *groupRepository) AddGroupMember(ctx context.Context, groupID int, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT IGNORE INTO campaign_group_members (group_id, user_id) VALUES (?, ?)`,
		groupID, userID,
	)
	if err != nil {
		return fmt.Errorf("adding group member: %w", err)
	}
	return nil
}

// RemoveGroupMember removes a user from a group.
func (r *groupRepository) RemoveGroupMember(ctx context.Context, groupID int, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM campaign_group_members WHERE group_id = ? AND user_id = ?`,
		groupID, userID,
	)
	if err != nil {
		return fmt.Errorf("removing group member: %w", err)
	}
	return nil
}

// ListGroupMembers returns all members of a group with display info from users table.
func (r *groupRepository) ListGroupMembers(ctx context.Context, groupID int) ([]GroupMemberInfo, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT cgm.user_id, u.display_name, u.email, cm.role, u.avatar_path
		 FROM campaign_group_members cgm
		 JOIN users u ON u.id = cgm.user_id
		 JOIN campaign_groups cg ON cg.id = cgm.group_id
		 JOIN campaign_members cm ON cm.campaign_id = cg.campaign_id AND cm.user_id = cgm.user_id
		 WHERE cgm.group_id = ?
		 ORDER BY u.display_name`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing group members: %w", err)
	}
	defer rows.Close()

	var members []GroupMemberInfo
	for rows.Next() {
		var m GroupMemberInfo
		if err := rows.Scan(&m.UserID, &m.DisplayName, &m.Email, &m.Role, &m.AvatarPath); err != nil {
			return nil, fmt.Errorf("scanning group member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}
