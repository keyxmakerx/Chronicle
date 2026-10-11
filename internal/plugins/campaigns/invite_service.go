package campaigns

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// InviteMailer sends invite emails. Subset of smtp.MailService to avoid
// importing the smtp package directly.
type InviteMailer interface {
	SendHTMLMail(ctx context.Context, to []string, subject, plainBody, htmlBody string) error
	IsConfigured(ctx context.Context) bool
}

// InviteService handles business logic for campaign invites.
type InviteService interface {
	CreateInvite(ctx context.Context, campaignID, createdBy string, input CreateInviteInput) (*Invite, error)
	ListInvites(ctx context.Context, campaignID string) ([]Invite, error)
	RevokeInvite(ctx context.Context, inviteID string) error
	AcceptInvite(ctx context.Context, token string, userID string) (*Invite, error)
	GetInviteByToken(ctx context.Context, token string) (*Invite, error)
}

// inviteService implements InviteService.
type inviteService struct {
	repo        InviteRepository
	campaigns   CampaignRepository
	mailer      InviteMailer
	baseURL     string
}

// NewInviteService creates a new invite service.
func NewInviteService(repo InviteRepository, campaigns CampaignRepository, mailer InviteMailer, baseURL string) InviteService {
	return &inviteService{
		repo:      repo,
		campaigns: campaigns,
		mailer:    mailer,
		baseURL:   strings.TrimRight(baseURL, "/"),
	}
}

// CreateInvite creates a new invitation and sends an email to the recipient.
func (s *inviteService) CreateInvite(ctx context.Context, campaignID, createdBy string, input CreateInviteInput) (*Invite, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" {
		return nil, apperror.NewValidation("email is required")
	}

	// Validate role.
	role := strings.ToLower(strings.TrimSpace(input.Role))
	if role == "" {
		role = "player"
	}
	if role != "player" && role != "scribe" {
		return nil, apperror.NewValidation("role must be 'player' or 'scribe'")
	}

	// Verify campaign exists.
	campaign, err := s.campaigns.FindByID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("fetching campaign: %w", err)
	}

	// Check for existing pending invite to same email.
	existing, err := s.repo.GetByEmailAndCampaign(ctx, email, campaignID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// Real error vs "not found" — sql.ErrNoRows is wrapped, so check message.
		if !strings.Contains(err.Error(), "no rows") {
			return nil, fmt.Errorf("checking existing invite: %w", err)
		}
	}
	if existing != nil && existing.IsPending() {
		return nil, apperror.NewValidation("an active invite already exists for this email")
	}

	// Clean up expired invites while we're here.
	if cleaned, err := s.repo.DeleteExpired(ctx, campaignID); err == nil && cleaned > 0 {
		slog.Debug("cleaned expired invites", slog.Int64("count", cleaned))
	}

	// Generate secure token.
	tokenBytes := make([]byte, inviteTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generating invite token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	invite := &Invite{
		ID:         uuid.New().String(),
		CampaignID: campaignID,
		Email:      email,
		Role:       role,
		Token:      token,
		CreatedBy:  createdBy,
		ExpiresAt:  time.Now().UTC().Add(time.Duration(inviteExpiryDays) * 24 * time.Hour),
	}

	if err := s.repo.Create(ctx, invite); err != nil {
		return nil, err
	}

	// Send invite email if SMTP is configured.
	if s.mailer != nil && s.mailer.IsConfigured(ctx) {
		acceptURL := fmt.Sprintf("%s/invites/accept?token=%s", s.baseURL, token)
		roleName := "Player"
		if role == "scribe" {
			roleName = "Scribe"
		}

		subject := fmt.Sprintf("You're invited to %s — Chronicle", campaign.Name)

		plainBody := fmt.Sprintf(
			"You've been invited to join \"%s\" as a %s on Chronicle.\n\n"+
				"Click the link below to accept:\n%s\n\n"+
				"This invitation expires in %d days.\n\n"+
				"If you don't have an account, you'll be able to create one when you accept.",
			campaign.Name, roleName, acceptURL, inviteExpiryDays)

		htmlBody := fmt.Sprintf(`<div style="font-family:sans-serif;max-width:600px;margin:0 auto;padding:20px">
<h2 style="color:#111827">You're invited to join "%s"</h2>
<p style="color:#374151;font-size:16px">You've been invited as a <strong>%s</strong>.</p>
<p style="margin:24px 0">
  <a href="%s" style="display:inline-block;padding:12px 24px;background:#6366f1;color:#fff;text-decoration:none;border-radius:8px;font-weight:600">Accept Invitation</a>
</p>
<p style="color:#6b7280;font-size:14px">This invitation expires in %d days.</p>
<p style="color:#6b7280;font-size:14px">If you don't have an account, you'll be able to create one when you accept.</p>
</div>`, campaign.Name, roleName, acceptURL, inviteExpiryDays)

		if err := s.mailer.SendHTMLMail(ctx, []string{email}, subject, plainBody, htmlBody); err != nil {
			slog.Warn("failed to send invite email",
				slog.String("email", email),
				slog.String("campaign", campaignID),
				slog.String("error", err.Error()))
			// Don't fail the invite creation if email fails.
		}
	}

	slog.Info("campaign invite created",
		slog.String("campaign_id", campaignID),
		slog.String("email", email),
		slog.String("role", role))

	return invite, nil
}

// ListInvites returns all invites for a campaign (pending, accepted, expired).
func (s *inviteService) ListInvites(ctx context.Context, campaignID string) ([]Invite, error) {
	return s.repo.ListByCampaign(ctx, campaignID)
}

// RevokeInvite deletes a pending invite.
func (s *inviteService) RevokeInvite(ctx context.Context, inviteID string) error {
	return s.repo.Delete(ctx, inviteID)
}

// AcceptInvite validates the token and adds the user to the campaign.
func (s *inviteService) AcceptInvite(ctx context.Context, token string, userID string) (*Invite, error) {
	invite, err := s.repo.GetByToken(ctx, token)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, apperror.NewNotFound("invite not found")
		}
		return nil, err
	}

	if invite.AcceptedAt != nil {
		return nil, apperror.NewValidation("this invitation has already been accepted")
	}

	if invite.IsExpired() {
		return nil, apperror.NewValidation("this invitation has expired")
	}

	// A campaign in the site Trash is not found: nobody may be added to it.
	if _, err := s.campaigns.FindByID(ctx, invite.CampaignID); err != nil {
		return nil, err
	}

	// Check if user is already a member.
	_, err = s.campaigns.FindMember(ctx, invite.CampaignID, userID)
	if err == nil {
		// Already a member — mark invite accepted and return.
		_ = s.repo.MarkAccepted(ctx, invite.ID)
		return invite, apperror.NewValidation("you are already a member of this campaign")
	}

	// Add user to campaign with the invited role.
	role := RoleFromString(invite.Role)
	member := &CampaignMember{
		CampaignID: invite.CampaignID,
		UserID:     userID,
		Role:       role,
		JoinedAt:   time.Now().UTC(),
	}
	if err := s.campaigns.AddMember(ctx, member); err != nil {
		return nil, fmt.Errorf("adding member: %w", err)
	}

	// Mark invite as accepted.
	if err := s.repo.MarkAccepted(ctx, invite.ID); err != nil {
		return nil, fmt.Errorf("marking invite accepted: %w", err)
	}

	slog.Info("campaign invite accepted",
		slog.String("campaign_id", invite.CampaignID),
		slog.String("user_id", userID),
		slog.String("role", invite.Role))

	return invite, nil
}

// GetInviteByToken retrieves an invite by its token for display purposes.
func (s *inviteService) GetInviteByToken(ctx context.Context, token string) (*Invite, error) {
	invite, err := s.repo.GetByToken(ctx, token)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, apperror.NewNotFound("invite not found")
		}
		return nil, err
	}
	return invite, nil
}
