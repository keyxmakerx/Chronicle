package campaigns

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// transferTokenBytes is the number of random bytes in a transfer token.
const transferTokenBytes = 32

// transferExpiryHours is how long a transfer token remains valid.
const transferExpiryHours = 72

// CampaignService handles business logic for campaign operations.
// It owns slug generation, membership rules, and ownership transfers.
type CampaignService interface {
	// Campaign CRUD
	Create(ctx context.Context, userID string, input CreateCampaignInput) (*Campaign, error)
	GetByID(ctx context.Context, id string) (*Campaign, error)
	GetBySlug(ctx context.Context, slug string) (*Campaign, error)
	List(ctx context.Context, userID string, opts ListOptions) ([]Campaign, int, error)
	ListAll(ctx context.Context, opts ListOptions) ([]Campaign, int, error)
	ListPublic(ctx context.Context, limit int) ([]Campaign, error)
	Update(ctx context.Context, campaignID string, input UpdateCampaignInput) (*Campaign, error)
	Delete(ctx context.Context, campaignID string) error
	CountAll(ctx context.Context) (int, error)

	// Membership
	GetMember(ctx context.Context, campaignID, userID string) (*CampaignMember, error)
	AddMember(ctx context.Context, campaignID, email string, role Role) error
	RemoveMember(ctx context.Context, campaignID, userID string) error
	UpdateMemberRole(ctx context.Context, campaignID, userID string, role Role) error
	UpdateMemberCharacter(ctx context.Context, campaignID, userID string, characterEntityID *string) error
	ListMembers(ctx context.Context, campaignID string) ([]CampaignMember, error)

	// Ownership transfer
	InitiateTransfer(ctx context.Context, campaignID, ownerID, targetEmail string) (*OwnershipTransfer, error)
	AcceptTransfer(ctx context.Context, token string, acceptingUserID string) error
	CancelTransfer(ctx context.Context, campaignID string) error
	GetPendingTransfer(ctx context.Context, campaignID string) (*OwnershipTransfer, error)

	// Backdrop and branding
	UpdateBackdropPath(ctx context.Context, campaignID string, path *string) error
	UpdateAccentColor(ctx context.Context, campaignID string, color string) error
	// UpdateBranding sets the campaign's custom brand name and logo path.
	UpdateBranding(ctx context.Context, campaignID, brandName, brandLogo string) error
	// UpdateTopbarStyle sets the campaign's topbar visual customization.
	UpdateTopbarStyle(ctx context.Context, campaignID string, style *TopbarStyle) error
	UpdateDmGrants(ctx context.Context, campaignID string, userIDs []string) error
	// UpdateFontFamily sets the campaign's body font family.
	UpdateFontFamily(ctx context.Context, campaignID, fontFamily string) error
	// UpdateWelcomeMessage sets the campaign's MOTD banner message.
	UpdateWelcomeMessage(ctx context.Context, campaignID, message string) error
	// UpdateDefaultVisibility sets the default visibility for new entities.
	UpdateDefaultVisibility(ctx context.Context, campaignID, visibility string) error

	// Sidebar configuration
	UpdateSidebarConfig(ctx context.Context, campaignID string, config SidebarConfig) error
	GetSidebarConfig(ctx context.Context, campaignID string) (*SidebarConfig, error)

	// Dashboard layout
	UpdateDashboardLayout(ctx context.Context, campaignID string, layout *DashboardLayout) error
	UpdateDashboardLayoutRaw(ctx context.Context, campaignID string, layoutJSON *string) error
	GetDashboardLayout(ctx context.Context, campaignID string) (*DashboardLayout, error)
	ResetDashboardLayout(ctx context.Context, campaignID string) error

	// Owner dashboard layout
	UpdateOwnerDashboardLayout(ctx context.Context, campaignID string, layout *DashboardLayout) error
	GetOwnerDashboardLayout(ctx context.Context, campaignID string) (*DashboardLayout, error)
	ResetOwnerDashboardLayout(ctx context.Context, campaignID string) error

	// Admin operations
	ForceTransferOwnership(ctx context.Context, campaignID, newOwnerID string) error
	AdminAddMember(ctx context.Context, campaignID, userID string, role Role) error

	// Archive
	ArchiveCampaign(ctx context.Context, campaignID string) error
	UnarchiveCampaign(ctx context.Context, campaignID string) error

	// Shareable invite link
	GenerateJoinCode(ctx context.Context, campaignID string) (string, error)
	RevokeJoinCode(ctx context.Context, campaignID string) error
	JoinByCode(ctx context.Context, code, userID string) error

	// Game system
	UpdateSystemID(ctx context.Context, campaignID, systemID string) error

	// Lifecycle hooks — set after construction to avoid circular initialization.
	SetContentTemplateSeeder(seeder ContentTemplateSeeder)
	SetWorldbuildingPromptSeeder(seeder WorldbuildingPromptSeeder)
	SetLayoutPresetSeeder(seeder LayoutPresetSeeder)
	SetMediaCleaner(cleaner MediaCleaner)
	SetHookDispatcher(dispatcher CampaignHookDispatcher)
}

// GroupService handles business logic for campaign group operations.
type GroupService interface {
	CreateGroup(ctx context.Context, campaignID, name string, description *string) (*CampaignGroup, error)
	ListGroups(ctx context.Context, campaignID string) ([]CampaignGroup, error)
	GetGroup(ctx context.Context, groupID int) (*CampaignGroup, error)
	UpdateGroup(ctx context.Context, groupID int, name string, description *string) error
	DeleteGroup(ctx context.Context, groupID int) error
	AddGroupMember(ctx context.Context, groupID int, userID string) error
	RemoveGroupMember(ctx context.Context, groupID int, userID string) error
	ListGroupMembers(ctx context.Context, groupID int) ([]GroupMemberInfo, error)
}

// MediaCleaner handles bulk media file cleanup during campaign deletion.
// Implemented by the media service.
type MediaCleaner interface {
	DeleteCampaignFiles(ctx context.Context, campaignID string) (int, error)
}

// CampaignHookDispatcher fires lifecycle events for WASM plugin notification.
// Implemented by the extensions HookDispatcher.
type CampaignHookDispatcher interface {
	DispatchCampaignDeleted(ctx context.Context, campaignID string)
}

// campaignService implements CampaignService.
type campaignService struct {
	repo           CampaignRepository
	users          UserFinder
	mail           MailService            // May be nil if SMTP is not configured.
	seeder           EntityTypeSeeder       // Seeds default entity types on campaign creation. May be nil.
	templateSeeder   ContentTemplateSeeder  // Seeds default content templates on campaign creation. May be nil.
	promptSeeder     WorldbuildingPromptSeeder // Seeds default worldbuilding prompts on campaign creation. May be nil.
	layoutSeeder     LayoutPresetSeeder     // Seeds default layout presets on campaign creation. May be nil.
	mediaCleaner     MediaCleaner           // Cleans up media files on campaign delete. May be nil.
	hookDispatcher   CampaignHookDispatcher // Dispatches WASM lifecycle events. May be nil.
	baseURL          string
}

// NewCampaignService creates a new campaign service with the given dependencies.
// The mail and seeder parameters may be nil if the corresponding plugins are not yet wired.
func NewCampaignService(repo CampaignRepository, users UserFinder, mail MailService, seeder EntityTypeSeeder, baseURL string) CampaignService {
	return &campaignService{
		repo:    repo,
		users:   users,
		mail:    mail,
		seeder:  seeder,
		baseURL: baseURL,
	}
}

// SetContentTemplateSeeder sets the seeder for default content templates.
// Called after all plugins are wired to avoid initialization order issues.
func (s *campaignService) SetContentTemplateSeeder(seeder ContentTemplateSeeder) {
	s.templateSeeder = seeder
}

// SetWorldbuildingPromptSeeder sets the seeder for default worldbuilding prompts.
func (s *campaignService) SetWorldbuildingPromptSeeder(seeder WorldbuildingPromptSeeder) {
	s.promptSeeder = seeder
}

// SetLayoutPresetSeeder sets the seeder for default layout presets.
// Called after all plugins are wired to avoid initialization order issues.
func (s *campaignService) SetLayoutPresetSeeder(seeder LayoutPresetSeeder) {
	s.layoutSeeder = seeder
}

// SetMediaCleaner sets the media cleaner for campaign deletion cleanup.
// Called after all plugins are wired to avoid initialization order issues.
func (s *campaignService) SetMediaCleaner(cleaner MediaCleaner) {
	s.mediaCleaner = cleaner
}

// SetHookDispatcher sets the WASM hook dispatcher for campaign lifecycle events.
// Called after all plugins are wired to avoid initialization order issues.
func (s *campaignService) SetHookDispatcher(dispatcher CampaignHookDispatcher) {
	s.hookDispatcher = dispatcher
}

// --- Campaign CRUD ---

// Create creates a new campaign and automatically adds the creator as Owner.
func (s *campaignService) Create(ctx context.Context, userID string, input CreateCampaignInput) (*Campaign, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("campaign name is required")
	}
	if len(name) > 200 {
		return nil, apperror.NewBadRequest("campaign name must be at most 200 characters")
	}

	desc := strings.TrimSpace(input.Description)
	if len(desc) > 5000 {
		return nil, apperror.NewBadRequest("description must be at most 5000 characters")
	}

	// Generate a unique slug from the name.
	slug, err := s.generateSlug(ctx, name)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("generating slug: %w", err))
	}

	now := time.Now().UTC()
	var descPtr *string
	if desc != "" {
		descPtr = &desc
	}

	campaign := &Campaign{
		ID:            generateUUID(),
		Name:          name,
		Slug:          slug,
		Description:   descPtr,
		Settings:      "{}",
		SidebarConfig: "{}",
		CreatedBy:     userID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.repo.Create(ctx, campaign); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating campaign: %w", err))
	}

	// Auto-add the creator as Owner.
	member := &CampaignMember{
		CampaignID: campaign.ID,
		UserID:     userID,
		Role:       RoleOwner,
		JoinedAt:   now,
	}
	if err := s.repo.AddMember(ctx, member); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("adding owner member: %w", err))
	}

	// Seed default entity types for the new campaign.
	if s.seeder != nil {
		if err := s.seeder.SeedDefaults(ctx, campaign.ID); err != nil {
			// Non-fatal: campaign is still usable without default types.
			slog.Warn("failed to seed default entity types",
				slog.String("campaign_id", campaign.ID),
				slog.Any("error", err),
			)
		}
	}

	// Seed default content templates for the new campaign.
	if s.templateSeeder != nil {
		if err := s.templateSeeder.SeedDefaults(ctx, campaign.ID); err != nil {
			slog.Warn("failed to seed default content templates",
				slog.String("campaign_id", campaign.ID),
				slog.Any("error", err),
			)
		}
	}

	// Seed default worldbuilding prompts for the new campaign.
	if s.promptSeeder != nil {
		if err := s.promptSeeder.SeedDefaults(ctx, campaign.ID); err != nil {
			slog.Warn("failed to seed default worldbuilding prompts",
				slog.String("campaign_id", campaign.ID),
				slog.Any("error", err),
			)
		}
	}

	// Seed default layout presets for the new campaign.
	if s.layoutSeeder != nil {
		if err := s.layoutSeeder.SeedDefaults(ctx, campaign.ID); err != nil {
			slog.Warn("failed to seed default layout presets",
				slog.String("campaign_id", campaign.ID),
				slog.Any("error", err),
			)
		}
	}

	slog.Info("campaign created",
		slog.String("campaign_id", campaign.ID),
		slog.String("slug", campaign.Slug),
		slog.String("user_id", userID),
	)

	return campaign, nil
}

// GetByID retrieves a campaign by ID.
func (s *campaignService) GetByID(ctx context.Context, id string) (*Campaign, error) {
	return s.repo.FindByID(ctx, id)
}

// GetBySlug retrieves a campaign by its URL slug.
func (s *campaignService) GetBySlug(ctx context.Context, slug string) (*Campaign, error) {
	return s.repo.FindBySlug(ctx, slug)
}

// List returns campaigns the user is a member of.
func (s *campaignService) List(ctx context.Context, userID string, opts ListOptions) ([]Campaign, int, error) {
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 24
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	return s.repo.ListByUser(ctx, userID, opts)
}

// ListAll returns all campaigns. Admin only.
func (s *campaignService) ListAll(ctx context.Context, opts ListOptions) ([]Campaign, int, error) {
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 24
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	return s.repo.ListAll(ctx, opts)
}

// ListPublic returns public campaigns for the landing page. Clamps the limit
// to a sane range to prevent abuse via URL parameter manipulation.
func (s *campaignService) ListPublic(ctx context.Context, limit int) ([]Campaign, error) {
	if limit < 1 || limit > 50 {
		limit = 12
	}
	return s.repo.ListPublic(ctx, limit)
}

// Update modifies a campaign's name and description.
func (s *campaignService) Update(ctx context.Context, campaignID string, input UpdateCampaignInput) (*Campaign, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("campaign name is required")
	}
	if len(name) > 200 {
		return nil, apperror.NewBadRequest("campaign name must be at most 200 characters")
	}

	desc := strings.TrimSpace(input.Description)
	if len(desc) > 5000 {
		return nil, apperror.NewBadRequest("description must be at most 5000 characters")
	}

	// Regenerate slug if name changed.
	if name != campaign.Name {
		slug, err := s.generateSlug(ctx, name)
		if err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("generating slug: %w", err))
		}
		campaign.Slug = slug
	}

	campaign.Name = name
	if desc != "" {
		campaign.Description = &desc
	} else {
		campaign.Description = nil
	}
	campaign.IsPublic = input.IsPublic

	if err := s.repo.Update(ctx, campaign); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("updating campaign: %w", err))
	}

	return campaign, nil
}

// Delete removes a campaign and all its data. Performs multi-step cleanup:
// 1. Delete media files from disk (before SQL CASCADE nullifies campaign_id)
// 2. Dispatch campaign.deleted hook to WASM plugins for cache cleanup
// 3. SQL DELETE with FK CASCADE handles remaining database rows
func (s *campaignService) Delete(ctx context.Context, campaignID string) error {
	// Step 1: Clean up media files from disk before the SQL DELETE.
	// The media_files FK uses ON DELETE SET NULL, so we must delete files
	// while we still know which campaign they belong to.
	if s.mediaCleaner != nil {
		deleted, err := s.mediaCleaner.DeleteCampaignFiles(ctx, campaignID)
		if err != nil {
			// Log but don't fail — orphaned files are preferable to a stuck campaign.
			slog.Warn("media cleanup failed during campaign deletion",
				slog.String("campaign_id", campaignID),
				slog.Any("error", err),
			)
		} else if deleted > 0 {
			slog.Info("cleaned up campaign media files",
				slog.String("campaign_id", campaignID),
				slog.Int("deleted", deleted),
			)
		}
	}

	// Step 2: Notify WASM plugins so they can clean up in-memory state.
	if s.hookDispatcher != nil {
		s.hookDispatcher.DispatchCampaignDeleted(ctx, campaignID)
	}

	// Step 3: SQL DELETE — FK CASCADE handles all remaining DB rows.
	if err := s.repo.Delete(ctx, campaignID); err != nil {
		return err
	}

	slog.Info("campaign deleted", slog.String("campaign_id", campaignID))
	return nil
}

// CountAll returns total number of campaigns. Used for admin dashboard.
func (s *campaignService) CountAll(ctx context.Context) (int, error) {
	return s.repo.CountAll(ctx)
}

// --- Membership ---

// GetMember retrieves a user's membership in a campaign.
func (s *campaignService) GetMember(ctx context.Context, campaignID, userID string) (*CampaignMember, error) {
	return s.repo.FindMember(ctx, campaignID, userID)
}

// AddMember adds a user to a campaign by their email address.
func (s *campaignService) AddMember(ctx context.Context, campaignID, email string, role Role) error {
	if !isValidEmail(email) {
		return apperror.NewBadRequest("invalid email address")
	}
	if !role.IsValid() {
		return apperror.NewBadRequest("invalid role")
	}
	// Only Owner and admin can add members, but Owner role can't be assigned
	// through regular member addition -- only through ownership transfer.
	if role == RoleOwner {
		return apperror.NewBadRequest("cannot add a member as owner; use ownership transfer instead")
	}

	// Look up the user by email.
	user, err := s.users.FindUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return apperror.NewBadRequest("no user found with that email")
	}

	// Check if already a member.
	_, err = s.repo.FindMember(ctx, campaignID, user.ID)
	if err == nil {
		return apperror.NewConflict("user is already a member of this campaign")
	}

	member := &CampaignMember{
		CampaignID: campaignID,
		UserID:     user.ID,
		Role:       role,
		JoinedAt:   time.Now().UTC(),
	}

	if err := s.repo.AddMember(ctx, member); err != nil {
		return apperror.NewInternal(fmt.Errorf("adding member: %w", err))
	}

	slog.Info("member added to campaign",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", user.ID),
		slog.String("role", role.String()),
	)
	return nil
}

// RemoveMember removes a user from a campaign. The owner cannot be removed.
func (s *campaignService) RemoveMember(ctx context.Context, campaignID, userID string) error {
	member, err := s.repo.FindMember(ctx, campaignID, userID)
	if err != nil {
		return err
	}

	// Owners must transfer ownership before they can be removed.
	if member.Role == RoleOwner {
		return apperror.NewBadRequest("cannot remove the campaign owner; transfer ownership first")
	}

	if err := s.repo.RemoveMember(ctx, campaignID, userID); err != nil {
		return apperror.NewInternal(fmt.Errorf("removing member: %w", err))
	}

	slog.Info("member removed from campaign",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
	)
	return nil
}

// UpdateMemberRole changes a member's role. The owner's role cannot be changed
// through this method -- use ownership transfer instead.
func (s *campaignService) UpdateMemberRole(ctx context.Context, campaignID, userID string, role Role) error {
	if !role.IsValid() {
		return apperror.NewBadRequest("invalid role")
	}
	if role == RoleOwner {
		return apperror.NewBadRequest("cannot promote to owner; use ownership transfer instead")
	}

	member, err := s.repo.FindMember(ctx, campaignID, userID)
	if err != nil {
		return err
	}

	// Can't change the owner's role.
	if member.Role == RoleOwner {
		return apperror.NewBadRequest("cannot change the owner's role; transfer ownership first")
	}

	if err := s.repo.UpdateMemberRole(ctx, campaignID, userID, role); err != nil {
		return apperror.NewInternal(fmt.Errorf("updating role: %w", err))
	}

	slog.Info("member role updated",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
		slog.String("new_role", role.String()),
	)
	return nil
}

// UpdateMemberCharacter sets a member's character entity assignment.
func (s *campaignService) UpdateMemberCharacter(ctx context.Context, campaignID, userID string, characterEntityID *string) error {
	return s.repo.UpdateMemberCharacter(ctx, campaignID, userID, characterEntityID)
}

// ListMembers returns all members of a campaign.
func (s *campaignService) ListMembers(ctx context.Context, campaignID string) ([]CampaignMember, error) {
	return s.repo.ListMembers(ctx, campaignID)
}

// --- Ownership Transfer ---

// InitiateTransfer starts an ownership transfer. Generates a token and
// optionally sends an email if SMTP is configured.
func (s *campaignService) InitiateTransfer(ctx context.Context, campaignID, ownerID, targetEmail string) (*OwnershipTransfer, error) {
	email := strings.ToLower(strings.TrimSpace(targetEmail))
	if !isValidEmail(email) {
		return nil, apperror.NewBadRequest("invalid email address")
	}

	// Verify the target user exists.
	targetUser, err := s.users.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, apperror.NewBadRequest("no user found with that email")
	}

	// Can't transfer to yourself.
	if targetUser.ID == ownerID {
		return nil, apperror.NewBadRequest("cannot transfer ownership to yourself")
	}

	// Check for existing pending transfer.
	existing, err := s.repo.FindTransferByCampaign(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("checking existing transfer: %w", err))
	}
	if existing != nil {
		return nil, apperror.NewConflict("a transfer is already pending for this campaign; cancel it first")
	}

	// Generate a random token.
	token, err := generateToken()
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("generating transfer token: %w", err))
	}

	now := time.Now().UTC()
	transfer := &OwnershipTransfer{
		ID:         generateUUID(),
		CampaignID: campaignID,
		FromUserID: ownerID,
		ToUserID:   targetUser.ID,
		Token:      token,
		ExpiresAt:  now.Add(transferExpiryHours * time.Hour),
		CreatedAt:  now,
	}

	if err := s.repo.CreateTransfer(ctx, transfer); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating transfer: %w", err))
	}

	// Send email if SMTP is configured.
	if s.mail != nil && s.mail.IsConfigured(ctx) {
		campaign, _ := s.repo.FindByID(ctx, campaignID)
		campaignName := "your campaign"
		if campaign != nil {
			campaignName = campaign.Name
		}

		link := fmt.Sprintf("%s/campaigns/%s/accept-transfer?token=%s", s.baseURL, campaignID, token)
		body := fmt.Sprintf(
			"You have been offered ownership of the campaign \"%s\" on Chronicle.\n\n"+
				"Click the link below to accept (you must be logged in):\n%s\n\n"+
				"This link expires in %d hours. If you did not expect this, you can ignore it.",
			campaignName, link, transferExpiryHours,
		)

		if err := s.mail.SendMail(ctx, []string{email}, "Campaign Ownership Transfer", body); err != nil {
			// Log but don't fail -- the transfer is still created and can be
			// accepted via the campaign settings page.
			slog.Warn("failed to send transfer email",
				slog.String("campaign_id", campaignID),
				slog.String("to", email),
				slog.Any("error", err),
			)
		}
	}

	slog.Info("ownership transfer initiated",
		slog.String("campaign_id", campaignID),
		slog.String("from", ownerID),
		slog.String("to", targetUser.ID),
	)

	return transfer, nil
}

// AcceptTransfer completes a pending ownership transfer. The accepting user
// must match the transfer's to_user_id and the token must not be expired.
func (s *campaignService) AcceptTransfer(ctx context.Context, token string, acceptingUserID string) error {
	transfer, err := s.repo.FindTransferByToken(ctx, token)
	if err != nil {
		return apperror.NewBadRequest("invalid or expired transfer link")
	}

	// Verify the token hasn't expired.
	if time.Now().UTC().After(transfer.ExpiresAt) {
		// Clean up the expired transfer.
		_ = s.repo.DeleteTransfer(ctx, transfer.ID)
		return apperror.NewBadRequest("this transfer link has expired")
	}

	// Verify the accepting user is the intended recipient.
	if transfer.ToUserID != acceptingUserID {
		return apperror.NewForbidden("this transfer is not for your account")
	}

	// Perform the atomic transfer.
	if err := s.repo.TransferOwnership(ctx, transfer.CampaignID, transfer.FromUserID, transfer.ToUserID); err != nil {
		return apperror.NewInternal(fmt.Errorf("transferring ownership: %w", err))
	}

	slog.Info("ownership transfer completed",
		slog.String("campaign_id", transfer.CampaignID),
		slog.String("from", transfer.FromUserID),
		slog.String("to", transfer.ToUserID),
	)

	return nil
}

// CancelTransfer removes a pending ownership transfer.
func (s *campaignService) CancelTransfer(ctx context.Context, campaignID string) error {
	transfer, err := s.repo.FindTransferByCampaign(ctx, campaignID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("finding transfer: %w", err))
	}
	if transfer == nil {
		return apperror.NewNotFound("no pending transfer for this campaign")
	}

	if err := s.repo.DeleteTransfer(ctx, transfer.ID); err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting transfer: %w", err))
	}

	slog.Info("ownership transfer cancelled", slog.String("campaign_id", campaignID))
	return nil
}

// GetPendingTransfer returns the pending transfer for a campaign, or nil.
func (s *campaignService) GetPendingTransfer(ctx context.Context, campaignID string) (*OwnershipTransfer, error) {
	return s.repo.FindTransferByCampaign(ctx, campaignID)
}

// --- Sidebar Configuration ---

// maxSidebarConfigEntries caps the number of entries in sidebar config arrays
// to prevent abuse via oversized JSON payloads.
const maxSidebarConfigEntries = 100

// UpdateSidebarConfig updates the campaign's sidebar configuration. Validates
// UpdateBackdropPath sets or clears the campaign's backdrop image path.
func (s *campaignService) UpdateBackdropPath(ctx context.Context, campaignID string, path *string) error {
	return s.repo.UpdateBackdropPath(ctx, campaignID, path)
}

// UpdateAccentColor sets the campaign's accent color. Accepts a hex color
// string (e.g. "#6366f1") or empty string to reset to default.
func (s *campaignService) UpdateAccentColor(ctx context.Context, campaignID string, color string) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.AccentColor = color

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// UpdateBranding sets the campaign's custom brand name and logo path.
// Brand name max 40 chars. Empty strings clear the respective fields.
func (s *campaignService) UpdateBranding(ctx context.Context, campaignID, brandName, brandLogo string) error {
	if len(brandName) > 40 {
		return apperror.NewBadRequest("brand name must be 40 characters or fewer")
	}
	if len(brandLogo) > 255 {
		return apperror.NewBadRequest("brand logo path too long")
	}

	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.BrandName = brandName
	settings.BrandLogo = brandLogo

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// UpdateTopbarStyle sets the campaign's topbar visual customization.
// Nil style clears the customization (reverts to default).
func (s *campaignService) UpdateTopbarStyle(ctx context.Context, campaignID string, style *TopbarStyle) error {
	if style != nil {
		// Validate mode.
		switch style.Mode {
		case "solid", "gradient", "image", "":
			// ok
		default:
			return apperror.NewBadRequest("invalid topbar mode, expected solid, gradient, or image")
		}
		// Validate color values are valid hex (e.g. "#6366f1") or empty.
		for _, color := range []string{style.Color, style.GradientFrom, style.GradientTo} {
			if color != "" && !isValidHexColor(color) {
				return apperror.NewBadRequest("invalid color format, expected #RRGGBB")
			}
		}
		// Validate gradient direction.
		if style.GradientDir != "" {
			switch style.GradientDir {
			case "to-r", "to-br", "to-b":
				// ok
			default:
				return apperror.NewBadRequest("invalid gradient direction")
			}
		}
	}

	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.TopbarStyle = style

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// isValidHexColor checks that s is a 7-character hex color (#RRGGBB).
func isValidHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// UpdateDmGrants sets which users are granted dm_only content visibility.
// Only campaign Owners may call this. The granted users can see dm_only
// content but cannot create or toggle dm_only flags.
func (s *campaignService) UpdateDmGrants(ctx context.Context, campaignID string, userIDs []string) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.DmGrantIDs = userIDs

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// validFontFamilies defines the allowed font families for campaigns.
var validFontFamilies = map[string]bool{
	"":              true, // default
	"serif":         true,
	"sans-serif":    true,
	"monospace":     true,
	"georgia":       true,
	"merriweather":  true,
}

// UpdateFontFamily sets the campaign's body font family.
// Empty string resets to the default font.
func (s *campaignService) UpdateFontFamily(ctx context.Context, campaignID, fontFamily string) error {
	if !validFontFamilies[fontFamily] {
		return apperror.NewBadRequest("invalid font family")
	}

	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.FontFamily = fontFamily

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// UpdateWelcomeMessage sets the campaign's MOTD banner message.
// Empty string clears the message.
func (s *campaignService) UpdateWelcomeMessage(ctx context.Context, campaignID, message string) error {
	if len(message) > 500 {
		return apperror.NewBadRequest("welcome message must be 500 characters or fewer")
	}

	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.WelcomeMessage = message

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// validDefaultVisibilities defines the allowed default visibility values.
var validDefaultVisibilities = map[string]bool{
	"":        true, // everyone (default)
	"dm_only": true,
	"private": true,
}

// UpdateDefaultVisibility sets the campaign's default visibility for new entities.
// Empty string means "everyone" (the platform default).
func (s *campaignService) UpdateDefaultVisibility(ctx context.Context, campaignID, visibility string) error {
	if !validDefaultVisibilities[visibility] {
		return apperror.NewBadRequest("invalid visibility, expected empty, dm_only, or private")
	}

	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.DefaultVisibility = visibility

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// array sizes and persists as JSON.
func (s *campaignService) UpdateSidebarConfig(ctx context.Context, campaignID string, config SidebarConfig) error {
	if len(config.EntityTypeOrder) > maxSidebarConfigEntries {
		return apperror.NewBadRequest("entity type order list is too long")
	}
	if len(config.HiddenTypeIDs) > maxSidebarConfigEntries {
		return apperror.NewBadRequest("hidden type list is too long")
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling sidebar config: %w", err))
	}

	if err := s.repo.UpdateSidebarConfig(ctx, campaignID, string(configJSON)); err != nil {
		return err
	}

	slog.Info("sidebar config updated", slog.String("campaign_id", campaignID))
	return nil
}

// GetSidebarConfig returns the parsed sidebar configuration for a campaign.
func (s *campaignService) GetSidebarConfig(ctx context.Context, campaignID string) (*SidebarConfig, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	cfg := campaign.ParseSidebarConfig()
	return &cfg, nil
}

// --- Dashboard Layout ---

// maxDashboardRows caps the number of rows in a dashboard layout to prevent
// abuse via oversized JSON payloads.
const maxDashboardRows = 50

// maxDashboardBlocksPerRow caps the total number of blocks per row.
const maxDashboardBlocksPerRow = 20

// validateDashboardLayout validates block types, column widths, and sanitizes
// text_block content. Shared by both campaign and owner dashboard layouts.
func validateDashboardLayout(layout *DashboardLayout) error {
	if len(layout.Rows) > maxDashboardRows {
		return apperror.NewBadRequest("dashboard layout has too many rows")
	}

	for _, row := range layout.Rows {
		totalWidth := 0
		blockCount := 0
		for _, col := range row.Columns {
			if col.Width < 1 || col.Width > 12 {
				return apperror.NewBadRequest("column width must be between 1 and 12")
			}
			totalWidth += col.Width
			blockCount += len(col.Blocks)
			for i, block := range col.Blocks {
				if !ValidBlockTypes[block.Type] {
					return apperror.NewBadRequest(fmt.Sprintf("unsupported block type: %s", block.Type))
				}
				if block.Type == "text_block" {
					if content, ok := block.Config["content"].(string); ok {
						col.Blocks[i].Config["content"] = sanitize.HTML(content)
					}
				}
			}
		}
		if totalWidth > 12 {
			return apperror.NewBadRequest("row column widths exceed 12")
		}
		if blockCount > maxDashboardBlocksPerRow {
			return apperror.NewBadRequest("too many blocks in a single row")
		}
	}
	return nil
}

// marshalLayout marshals a DashboardLayout to a JSON string pointer.
func marshalLayout(layout *DashboardLayout) (*string, error) {
	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("marshaling dashboard layout: %w", err))
	}
	s := string(layoutJSON)
	return &s, nil
}

// UpdateDashboardLayout validates and saves a dashboard layout for a campaign.
func (s *campaignService) UpdateDashboardLayout(ctx context.Context, campaignID string, layout *DashboardLayout) error {
	if layout == nil {
		return s.repo.UpdateDashboardLayout(ctx, campaignID, nil)
	}
	if err := validateDashboardLayout(layout); err != nil {
		return err
	}
	layoutStr, err := marshalLayout(layout)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateDashboardLayout(ctx, campaignID, layoutStr); err != nil {
		return err
	}
	slog.Info("dashboard layout updated", slog.String("campaign_id", campaignID))
	return nil
}

// UpdateDashboardLayoutRaw saves a pre-marshaled dashboard layout JSON string.
// Used by the handler when merging role-specific layouts.
func (s *campaignService) UpdateDashboardLayoutRaw(ctx context.Context, campaignID string, layoutJSON *string) error {
	if err := s.repo.UpdateDashboardLayout(ctx, campaignID, layoutJSON); err != nil {
		return err
	}
	slog.Info("dashboard layout updated (raw)", slog.String("campaign_id", campaignID))
	return nil
}

// GetDashboardLayout returns the parsed dashboard layout for a campaign.
// Returns nil if no custom layout is set (use default).
func (s *campaignService) GetDashboardLayout(ctx context.Context, campaignID string) (*DashboardLayout, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return campaign.ParseDashboardLayout(), nil
}

// ResetDashboardLayout removes the custom dashboard layout, reverting to default.
func (s *campaignService) ResetDashboardLayout(ctx context.Context, campaignID string) error {
	if err := s.repo.UpdateDashboardLayout(ctx, campaignID, nil); err != nil {
		return err
	}
	slog.Info("dashboard layout reset to default", slog.String("campaign_id", campaignID))
	return nil
}

// UpdateOwnerDashboardLayout validates and saves the owner dashboard layout.
func (s *campaignService) UpdateOwnerDashboardLayout(ctx context.Context, campaignID string, layout *DashboardLayout) error {
	if layout == nil {
		return s.repo.UpdateOwnerDashboardLayout(ctx, campaignID, nil)
	}
	if err := validateDashboardLayout(layout); err != nil {
		return err
	}
	layoutStr, err := marshalLayout(layout)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateOwnerDashboardLayout(ctx, campaignID, layoutStr); err != nil {
		return err
	}
	slog.Info("owner dashboard layout updated", slog.String("campaign_id", campaignID))
	return nil
}

// GetOwnerDashboardLayout returns the parsed owner dashboard layout.
// Returns nil if no custom layout is set (use default).
func (s *campaignService) GetOwnerDashboardLayout(ctx context.Context, campaignID string) (*DashboardLayout, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return campaign.ParseOwnerDashboardLayout(), nil
}

// ResetOwnerDashboardLayout removes the custom owner dashboard layout.
func (s *campaignService) ResetOwnerDashboardLayout(ctx context.Context, campaignID string) error {
	if err := s.repo.UpdateOwnerDashboardLayout(ctx, campaignID, nil); err != nil {
		return err
	}
	slog.Info("owner dashboard layout reset to default", slog.String("campaign_id", campaignID))
	return nil
}

// --- Admin Operations ---

// ForceTransferOwnership is used by admins to take ownership of a campaign.
// No email confirmation needed — this is an administrative action.
func (s *campaignService) ForceTransferOwnership(ctx context.Context, campaignID, newOwnerID string) error {
	if err := s.repo.ForceTransferOwnership(ctx, campaignID, newOwnerID); err != nil {
		return apperror.NewInternal(fmt.Errorf("force transferring ownership: %w", err))
	}

	slog.Info("admin force-transferred campaign ownership",
		slog.String("campaign_id", campaignID),
		slog.String("new_owner", newOwnerID),
	)
	return nil
}

// AdminAddMember adds a user to a campaign by their user ID. Used by admins
// to add themselves. When adding as Owner, triggers a force transfer.
func (s *campaignService) AdminAddMember(ctx context.Context, campaignID, userID string, role Role) error {
	if !role.IsValid() {
		return apperror.NewBadRequest("invalid role")
	}

	// Check if already a member.
	existing, err := s.repo.FindMember(ctx, campaignID, userID)
	if err == nil {
		// Already a member -- update their role if different.
		if existing.Role == role {
			return nil // No change needed.
		}

		// If promoting to Owner, use force transfer.
		if role == RoleOwner {
			return s.ForceTransferOwnership(ctx, campaignID, userID)
		}

		// Otherwise just update the role.
		return s.repo.UpdateMemberRole(ctx, campaignID, userID, role)
	}

	// Not a member -- add them. If joining as Owner, force-transfer.
	if role == RoleOwner {
		return s.ForceTransferOwnership(ctx, campaignID, userID)
	}

	member := &CampaignMember{
		CampaignID: campaignID,
		UserID:     userID,
		Role:       role,
		JoinedAt:   time.Now().UTC(),
	}

	if err := s.repo.AddMember(ctx, member); err != nil {
		return apperror.NewInternal(fmt.Errorf("admin adding member: %w", err))
	}

	slog.Info("admin added member to campaign",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
		slog.String("role", role.String()),
	)
	return nil
}

// --- Helpers ---

// maxSlugAttempts caps slug deduplication iterations to prevent DoS from
// adversarial name collisions (e.g., creating "test", "test-2" ... "test-N").
const maxSlugAttempts = 100

// generateSlug creates a unique slug for a campaign. If the base slug is
// taken, appends -2, -3, etc. After maxSlugAttempts, falls back to a random suffix.
func (s *campaignService) generateSlug(ctx context.Context, name string) (string, error) {
	base := Slugify(name)
	slug := base

	for i := 2; i < maxSlugAttempts+2; i++ {
		exists, err := s.repo.SlugExists(ctx, slug)
		if err != nil {
			return "", fmt.Errorf("checking slug: %w", err)
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}

	// Fallback: append random suffix to guarantee uniqueness.
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random slug suffix: %w", err)
	}
	return fmt.Sprintf("%s-%s", base, hex.EncodeToString(b)), nil
}

// generateUUID creates a new v4 UUID string using crypto/rand.
// Panics if the system entropy source fails, as this indicates a
// catastrophic system problem that would compromise all security.
func generateUUID() string {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// generateToken creates a cryptographically random hex-encoded token.
func generateToken() (string, error) {
	b := make([]byte, transferTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- Archive & Join Code ---

// ArchiveCampaign soft-archives a campaign, making it read-only.
func (s *campaignService) ArchiveCampaign(ctx context.Context, campaignID string) error {
	return s.repo.ArchiveCampaign(ctx, campaignID)
}

// UnarchiveCampaign restores write access to a soft-archived campaign.
func (s *campaignService) UnarchiveCampaign(ctx context.Context, campaignID string) error {
	return s.repo.UnarchiveCampaign(ctx, campaignID)
}

// GenerateJoinCode creates a shareable invite code for the campaign.
// Returns the generated code.
func (s *campaignService) GenerateJoinCode(ctx context.Context, campaignID string) (string, error) {
	b := make([]byte, 6) // 12 hex chars
	if _, err := rand.Read(b); err != nil {
		return "", apperror.NewInternal(fmt.Errorf("generating join code: %w", err))
	}
	code := hex.EncodeToString(b)

	if err := s.repo.SetJoinCode(ctx, campaignID, code); err != nil {
		return "", err
	}

	slog.Info("join code generated",
		slog.String("campaign_id", campaignID),
	)
	return code, nil
}

// RevokeJoinCode removes the shareable invite code for the campaign.
func (s *campaignService) RevokeJoinCode(ctx context.Context, campaignID string) error {
	return s.repo.ClearJoinCode(ctx, campaignID)
}

// JoinByCode joins a user to a campaign via its shareable invite code.
func (s *campaignService) JoinByCode(ctx context.Context, code, userID string) error {
	campaign, err := s.repo.FindByJoinCode(ctx, code)
	if err != nil {
		return err
	}

	if campaign.IsArchived() {
		return apperror.NewBadRequest("campaign is archived and not accepting new members")
	}

	// Check if already a member.
	_, err = s.repo.FindMember(ctx, campaign.ID, userID)
	if err == nil {
		return apperror.NewConflict("you are already a member of this campaign")
	}

	member := &CampaignMember{
		CampaignID: campaign.ID,
		UserID:     userID,
		Role:       RolePlayer,
		JoinedAt:   time.Now().UTC(),
	}

	if err := s.repo.AddMember(ctx, member); err != nil {
		return apperror.NewInternal(fmt.Errorf("joining campaign: %w", err))
	}

	slog.Info("user joined campaign via invite code",
		slog.String("campaign_id", campaign.ID),
		slog.String("user_id", userID),
	)
	return nil
}

// UpdateSystemID sets the game system for a campaign. Stores in the
// settings JSON column. Accepts a system registry ID or "custom:<url>".
func (s *campaignService) UpdateSystemID(ctx context.Context, campaignID, systemID string) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.SystemID = systemID

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// isValidEmail performs a basic format check on an email address.
func isValidEmail(email string) bool {
	// Simple regex: something@something.something
	return len(email) <= 254 && len(email) >= 5 && strings.Contains(email, "@") && strings.Contains(email, ".")
}

// --- Campaign Group Service ---

// groupService implements GroupService.
type groupService struct {
	repo GroupRepository
}

// NewGroupService creates a new group service.
func NewGroupService(repo GroupRepository) GroupService {
	return &groupService{repo: repo}
}

// CreateGroup creates a new campaign group with validation.
func (s *groupService) CreateGroup(ctx context.Context, campaignID, name string, description *string) (*CampaignGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperror.NewBadRequest("group name is required")
	}
	if len(name) > 100 {
		return nil, apperror.NewBadRequest("group name must be 100 characters or less")
	}

	group, err := s.repo.CreateGroup(ctx, campaignID, name, description)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, apperror.NewBadRequest("a group with this name already exists")
		}
		return nil, apperror.NewInternal(err)
	}
	return group, nil
}

// ListGroups returns all groups for a campaign with their members.
func (s *groupService) ListGroups(ctx context.Context, campaignID string) ([]CampaignGroup, error) {
	groups, err := s.repo.ListGroups(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	// Populate members for each group.
	for i := range groups {
		members, err := s.repo.ListGroupMembers(ctx, groups[i].ID)
		if err != nil {
			slog.Error("failed to list group members", slog.Int("group_id", groups[i].ID), slog.Any("error", err))
			continue
		}
		groups[i].Members = members
	}
	return groups, nil
}

// GetGroup returns a single group by ID with its members.
func (s *groupService) GetGroup(ctx context.Context, groupID int) (*CampaignGroup, error) {
	group, err := s.repo.GetGroup(ctx, groupID)
	if err != nil {
		return nil, apperror.NewNotFound("group not found")
	}
	members, _ := s.repo.ListGroupMembers(ctx, groupID)
	group.Members = members
	return group, nil
}

// UpdateGroup updates a group's name and description.
func (s *groupService) UpdateGroup(ctx context.Context, groupID int, name string, description *string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return apperror.NewBadRequest("group name is required")
	}
	if err := s.repo.UpdateGroup(ctx, groupID, name, description); err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return apperror.NewBadRequest("a group with this name already exists")
		}
		return apperror.NewInternal(err)
	}
	return nil
}

// DeleteGroup deletes a group.
func (s *groupService) DeleteGroup(ctx context.Context, groupID int) error {
	if err := s.repo.DeleteGroup(ctx, groupID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// AddGroupMember adds a user to a group.
func (s *groupService) AddGroupMember(ctx context.Context, groupID int, userID string) error {
	if err := s.repo.AddGroupMember(ctx, groupID, userID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// RemoveGroupMember removes a user from a group.
func (s *groupService) RemoveGroupMember(ctx context.Context, groupID int, userID string) error {
	if err := s.repo.RemoveGroupMember(ctx, groupID, userID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// ListGroupMembers returns all members of a group.
func (s *groupService) ListGroupMembers(ctx context.Context, groupID int) ([]GroupMemberInfo, error) {
	members, err := s.repo.ListGroupMembers(ctx, groupID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return members, nil
}
