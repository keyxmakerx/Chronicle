package campaigns

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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
	Update(ctx context.Context, campaignID string, input UpdateCampaignInput) (*Campaign, error)
	Delete(ctx context.Context, campaignID string) error
	CountAll(ctx context.Context) (int, error)

	// Membership
	GetMember(ctx context.Context, campaignID, userID string) (*CampaignMember, error)
	AddMember(ctx context.Context, campaignID, email string, role Role) error
	RemoveMember(ctx context.Context, campaignID, userID string) error
	UpdateMemberRole(ctx context.Context, campaignID, userID string, role Role) error
	ListMembers(ctx context.Context, campaignID string) ([]CampaignMember, error)

	// Ownership transfer
	InitiateTransfer(ctx context.Context, campaignID, ownerID, targetEmail string) (*OwnershipTransfer, error)
	AcceptTransfer(ctx context.Context, token string, acceptingUserID string) error
	CancelTransfer(ctx context.Context, campaignID string) error
	GetPendingTransfer(ctx context.Context, campaignID string) (*OwnershipTransfer, error)

	// Admin operations
	ForceTransferOwnership(ctx context.Context, campaignID, newOwnerID string) error
	AdminAddMember(ctx context.Context, campaignID, userID string, role Role) error
}

// campaignService implements CampaignService.
type campaignService struct {
	repo    CampaignRepository
	users   UserFinder
	mail    MailService // May be nil if SMTP is not configured.
	baseURL string
}

// NewCampaignService creates a new campaign service with the given dependencies.
// The mail parameter may be nil if SMTP is not configured.
func NewCampaignService(repo CampaignRepository, users UserFinder, mail MailService, baseURL string) CampaignService {
	return &campaignService{
		repo:    repo,
		users:   users,
		mail:    mail,
		baseURL: baseURL,
	}
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
		ID:          generateUUID(),
		Name:        name,
		Slug:        slug,
		Description: descPtr,
		Settings:    "{}",
		CreatedBy:   userID,
		CreatedAt:   now,
		UpdatedAt:   now,
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

	if err := s.repo.Update(ctx, campaign); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("updating campaign: %w", err))
	}

	return campaign, nil
}

// Delete removes a campaign and all its data (via FK CASCADE).
func (s *campaignService) Delete(ctx context.Context, campaignID string) error {
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

// ListMembers returns all members of a campaign.
func (s *campaignService) ListMembers(ctx context.Context, campaignID string) ([]CampaignMember, error) {
	return s.repo.ListMembers(ctx, campaignID)
}

// --- Ownership Transfer ---

// InitiateTransfer starts an ownership transfer. Generates a token and
// optionally sends an email if SMTP is configured.
func (s *campaignService) InitiateTransfer(ctx context.Context, campaignID, ownerID, targetEmail string) (*OwnershipTransfer, error) {
	email := strings.ToLower(strings.TrimSpace(targetEmail))

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

// generateSlug creates a unique slug for a campaign. If the base slug is
// taken, appends -2, -3, etc. until a unique one is found.
func (s *campaignService) generateSlug(ctx context.Context, name string) (string, error) {
	base := Slugify(name)
	slug := base

	for i := 2; ; i++ {
		exists, err := s.repo.SlugExists(ctx, slug)
		if err != nil {
			return "", fmt.Errorf("checking slug: %w", err)
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

// generateUUID creates a new v4 UUID string using crypto/rand.
func generateUUID() string {
	uuid := make([]byte, 16)
	_, _ = rand.Read(uuid)
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
