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

// guestVisitDays are the look-around lengths an owner can pick. Zero means
// the code makes a player, as before.
var guestVisitDays = []int{1, 3, 7}

// validVisitDays reports whether days is a length the card offers.
func validVisitDays(days int) bool {
	if days == 0 {
		return true
	}
	for _, d := range guestVisitDays {
		if d == days {
			return true
		}
	}
	return false
}

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
	// VisitDays is the look-around length; zero for a code that makes a player.
	VisitDays int
	// VisitUntil is when the visitor's access ends. Nil before the code is
	// used, and again once the owner made them a player.
	VisitUntil *time.Time
}

// IsVisit reports whether the code is a look-around pass.
func (g GuestCode) IsVisit() bool { return g.VisitDays > 0 }

// Status is what the owner's list shows for the code: open, expired, used,
// or for a used look-around pass visiting or visit_ended.
func (g GuestCode) Status(now time.Time) string {
	switch {
	case g.UsedAt != nil && g.IsVisit() && g.VisitUntil != nil && now.Before(*g.VisitUntil):
		return "visiting"
	case g.UsedAt != nil && g.IsVisit() && g.VisitUntil != nil:
		return "visit_ended"
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

// Create makes a one-use code for a campaign. visitDays above zero makes a
// look-around pass of that many days instead of a player's code.
func (s *GuestCodeService) Create(ctx context.Context, campaignID, createdBy, note string, visitDays int) (*NewGuestCode, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 100 {
		return nil, apperror.NewBadRequest("keep the note to 100 characters")
	}
	if !validVisitDays(visitDays) {
		return nil, apperror.NewBadRequest("pick 1, 3 or 7 days to look around")
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
		ExpiresAt: now.Add(guestCodeLife), CreatedAt: now, VisitDays: visitDays,
	}
	if err := s.repo.Create(ctx, &gc, hashGuestCode(code)); err != nil {
		return nil, apperror.NewInternal(err)
	}
	slog.Info("guest code made", slog.String("campaign_id", campaignID), slog.String("code_id", gc.ID), slog.Int("visit_days", visitDays))
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

// AdmitWithGuestCode makes the user a player in the campaign, or for a
// look-around pass starts their visit; a visitor is never made a member.
func (s *GuestCodeService) AdmitWithGuestCode(ctx context.Context, codeID, campaignID, userID string) error {
	if _, err := s.campaigns.FindMember(ctx, campaignID, userID); err == nil {
		return apperror.NewConflict("you're already in that campaign")
	}
	days, err := s.repo.VisitDays(ctx, codeID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if days > 0 {
		until := s.now().Add(time.Duration(days) * 24 * time.Hour)
		if err := s.repo.StartVisit(ctx, codeID, userID, until); err != nil {
			return apperror.NewInternal(err)
		}
		slog.Info("visiting with a guest code", slog.String("campaign_id", campaignID), slog.String("user_id", userID))
		return nil
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

// VisitPass reports a non-member's look-around pass for the campaign: when
// it ends, and whether it is still running. found is false when they never
// had one, or the owner has since made them a player.
func (s *GuestCodeService) VisitPass(ctx context.Context, campaignID, userID string) (until time.Time, live, found bool, err error) {
	u, err := s.repo.FindVisit(ctx, campaignID, userID)
	if err != nil || u == nil {
		return time.Time{}, false, false, err
	}
	return *u, s.now().Before(*u), true, nil
}

// visitCode loads a used look-around pass of this campaign, for the
// owner's buttons on it.
func (s *GuestCodeService) visitCode(ctx context.Context, campaignID, id string) (*GuestCode, error) {
	gc, err := s.repo.FindByID(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if !gc.IsVisit() || gc.UsedBy == nil || gc.VisitUntil == nil {
		return nil, apperror.NewNotFound("nobody is looking around with that code")
	}
	return gc, nil
}

// MakeVisitorPlayer turns a visitor, current or ended, into a player. The
// membership comes first: if clearing the pass then fails, membership
// already outranks it.
func (s *GuestCodeService) MakeVisitorPlayer(ctx context.Context, campaignID, id string) error {
	gc, err := s.visitCode(ctx, campaignID, id)
	if err != nil {
		return err
	}
	if _, err := s.campaigns.FindMember(ctx, campaignID, *gc.UsedBy); err != nil {
		if err := s.campaigns.AddMember(ctx, &CampaignMember{
			CampaignID: campaignID, UserID: *gc.UsedBy, Role: RolePlayer, JoinedAt: s.now(),
		}); err != nil {
			return apperror.NewInternal(fmt.Errorf("adding player: %w", err))
		}
	}
	if err := s.repo.ClearVisit(ctx, gc.ID); err != nil {
		return apperror.NewInternal(err)
	}
	slog.Info("visitor made a player", slog.String("campaign_id", campaignID), slog.String("user_id", *gc.UsedBy))
	return nil
}

// EndVisit ends a running visit now. The pass stays listed, so the owner
// can still make them a player later.
func (s *GuestCodeService) EndVisit(ctx context.Context, campaignID, id string) error {
	if _, err := s.visitCode(ctx, campaignID, id); err != nil {
		return err
	}
	return s.repo.EndVisit(ctx, campaignID, id, s.now())
}

// MoveGuestMembership is the campaign step of merging a guest.
func (s *GuestCodeService) MoveGuestMembership(ctx context.Context, campaignID, guestID, targetID string) error {
	return s.repo.MoveMembership(ctx, campaignID, guestID, targetID)
}
