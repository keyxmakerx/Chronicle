package campaigns

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	// guestCodeLife is how long a guest code stays usable.
	guestCodeLife = 7 * 24 * time.Hour
	// guestCodeListWindow is how far back the owner's list reaches.
	guestCodeListWindow = 30 * 24 * time.Hour
	// guestCodeMaxLive caps a campaign's open codes, so a runaway form can't
	// fill the table.
	guestCodeMaxLive = 20
	// guestCodeAlphabet leaves out letters and digits read aloud wrong
	// (0/O, 1/I/L). 31^8 codes, single use, behind a rate limit.
	guestCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	guestCodeLen      = 8
)

// GuestCode is one code an owner made. The code itself is never stored.
type GuestCode struct {
	ID         string
	CampaignID string
	Note       string
	CreatedBy  string
	ExpiresAt  time.Time
	UsedBy     *string
	UsedAt     *time.Time
	CreatedAt  time.Time
	UsedByName string
}

// Status is what the owner's list shows for the code.
func (g GuestCode) Status(now time.Time) string {
	switch {
	case g.UsedAt != nil:
		return "used"
	case !now.Before(g.ExpiresAt):
		return "expired"
	default:
		return "open"
	}
}

// NewGuestCode is a freshly made code, shown to the owner once.
type NewGuestCode struct {
	GuestCode
	Code string
}

// GuestCodeService makes and redeems guest codes. It is also auth's
// GuestCodes adapter.
type GuestCodeService struct {
	repo      GuestCodeRepository
	campaigns CampaignRepository
	now       func() time.Time
}

// NewGuestCodeService returns the guest code service.
func NewGuestCodeService(repo GuestCodeRepository, campaigns CampaignRepository) *GuestCodeService {
	return &GuestCodeService{repo: repo, campaigns: campaigns, now: func() time.Time { return time.Now().UTC() }}
}

// normalizeGuestCode accepts the code however it was typed.
func normalizeGuestCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hashGuestCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeGuestCode(code)))
	return hex.EncodeToString(sum[:])
}

// randomGuestCode returns a code like "K7QM-3WHT".
func randomGuestCode() (string, error) {
	size := big.NewInt(int64(len(guestCodeAlphabet)))
	b := make([]byte, 0, guestCodeLen+1)
	for i := 0; i < guestCodeLen; i++ {
		if i == guestCodeLen/2 {
			b = append(b, '-')
		}
		n, err := rand.Int(rand.Reader, size)
		if err != nil {
			return "", err
		}
		b = append(b, guestCodeAlphabet[n.Int64()])
	}
	return string(b), nil
}

// Create makes a one-use code for a campaign.
func (s *GuestCodeService) Create(ctx context.Context, campaignID, createdBy, note string) (*NewGuestCode, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 100 {
		return nil, apperror.NewBadRequest("keep the note to 100 characters")
	}
	now := s.now()
	live, err := s.repo.CountLive(ctx, campaignID, now)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if live >= guestCodeMaxLive {
		return nil, apperror.NewBadRequest(fmt.Sprintf("this campaign already has %d open codes; remove some first", guestCodeMaxLive))
	}
	code, err := randomGuestCode()
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("making guest code: %w", err))
	}
	gc := GuestCode{
		ID: generateUUID(), CampaignID: campaignID, Note: note, CreatedBy: createdBy,
		ExpiresAt: now.Add(guestCodeLife), CreatedAt: now,
	}
	if err := s.repo.Create(ctx, &gc, hashGuestCode(code)); err != nil {
		return nil, apperror.NewInternal(err)
	}
	slog.Info("guest code made", slog.String("campaign_id", campaignID), slog.String("code_id", gc.ID))
	return &NewGuestCode{GuestCode: gc, Code: code}, nil
}

// List returns the campaign's recent codes.
func (s *GuestCodeService) List(ctx context.Context, campaignID string) ([]GuestCode, error) {
	return s.repo.ListByCampaign(ctx, campaignID, s.now().Add(-guestCodeListWindow))
}

// Remove deletes an unused code.
func (s *GuestCodeService) Remove(ctx context.Context, campaignID, id string) error {
	return s.repo.DeleteUnused(ctx, campaignID, id)
}

// Now is the service's clock, for the list's status labels.
func (s *GuestCodeService) Now() time.Time { return s.now() }

// ClaimGuestCode takes a code for one join. An archived campaign takes no
// one, so its codes are released untouched.
func (s *GuestCodeService) ClaimGuestCode(ctx context.Context, code string) (string, string, error) {
	if len(normalizeGuestCode(code)) != guestCodeLen {
		return "", "", apperror.NewNotFound("that code isn't right, or it was already used or has expired")
	}
	id, campaignID, err := s.repo.Claim(ctx, hashGuestCode(code), s.now())
	if err != nil {
		return "", "", err
	}
	c, err := s.campaigns.FindByID(ctx, campaignID)
	if err != nil || c.IsArchived() {
		s.ReleaseGuestCode(ctx, id)
		return "", "", apperror.NewBadRequest("that campaign isn't taking new players")
	}
	return id, campaignID, nil
}

// ReleaseGuestCode puts a claimed code back.
func (s *GuestCodeService) ReleaseGuestCode(ctx context.Context, codeID string) {
	if err := s.repo.Release(ctx, codeID); err != nil {
		slog.Warn("releasing guest code", slog.String("code_id", codeID), slog.Any("error", err))
	}
}

// AdmitWithGuestCode makes the user a player in the campaign.
func (s *GuestCodeService) AdmitWithGuestCode(ctx context.Context, codeID, campaignID, userID string) error {
	if _, err := s.campaigns.FindMember(ctx, campaignID, userID); err == nil {
		return apperror.NewConflict("you're already in that campaign")
	}
	if err := s.campaigns.AddMember(ctx, &CampaignMember{
		CampaignID: campaignID, UserID: userID, Role: RolePlayer, JoinedAt: s.now(),
	}); err != nil {
		return apperror.NewInternal(fmt.Errorf("adding player: %w", err))
	}
	if err := s.repo.SetUsedBy(ctx, codeID, userID); err != nil {
		slog.Warn("recording guest code use", slog.String("code_id", codeID), slog.Any("error", err))
	}
	slog.Info("joined with a guest code", slog.String("campaign_id", campaignID), slog.String("user_id", userID))
	return nil
}

// MoveGuestMembership is the campaign step of merging a guest.
func (s *GuestCodeService) MoveGuestMembership(ctx context.Context, campaignID, guestID, targetID string) error {
	return s.repo.MoveMembership(ctx, campaignID, guestID, targetID)
}
