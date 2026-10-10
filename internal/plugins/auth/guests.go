package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// guestEmailDomain is the placeholder domain of a guest's account. .invalid
// never resolves, so no mail is ever sent to a guest by accident.
const guestEmailDomain = "@guest.invalid"

// GuestCodes is the campaign side of guest codes, implemented by an adapter
// over the campaigns service so auth imports no other plugin.
type GuestCodes interface {
	// ClaimGuestCode marks a live code used before anyone is made, so two
	// people racing for one code can't both get in.
	ClaimGuestCode(ctx context.Context, code string) (codeID, campaignID string, err error)
	// ReleaseGuestCode undoes a claim whose join then failed.
	ReleaseGuestCode(ctx context.Context, codeID string)
	// AdmitWithGuestCode makes the user a player in the code's campaign and
	// records them as the one who used it.
	AdmitWithGuestCode(ctx context.Context, codeID, campaignID, userID string) error
}

// ConfigureGuests wires guest codes. Without it the join page refuses.
func ConfigureGuests(svc AuthService, codes GuestCodes) {
	if s, ok := svc.(*authService); ok {
		s.guestCodes = codes
	}
}

// OnGuestMerged registers fn to move what a guest made in their campaign to
// the account they merge into. Steps must be safe to run twice: a failed
// merge is retried from the start.
func OnGuestMerged(svc AuthService, fn func(ctx context.Context, campaignID, guestID, targetID string) error) {
	if s, ok := svc.(*authService); ok && fn != nil {
		s.onGuestMerged = append(s.onGuestMerged, fn)
	}
}

// JoinInput is what the join page sends. SessionUserID is set when someone
// already signed in uses a code, which only adds them to the campaign.
type JoinInput struct {
	Code          string
	Name          string
	SessionUserID string
	IP            string
	UserAgent     string
}

// JoinResult says where the person landed. SessionToken is set only when a
// new guest was made.
type JoinResult struct {
	CampaignID   string
	SessionToken string
	User         *User
}

// KeepGuestInput is the "Keep my account" form.
type KeepGuestInput struct {
	Email    string
	Password string
}

// MergeGuestInput is the "I already have an account" form.
type MergeGuestInput struct {
	Email    string
	Password string
	Code     string
}

// normalizeGuestCode accepts the code however it was typed: any case, with
// or without the dash or spaces.
func normalizeGuestCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// JoinWithGuestCode lets someone into a campaign with a guest code.
func (s *authService) JoinWithGuestCode(ctx context.Context, in JoinInput) (*JoinResult, error) {
	if s.guestCodes == nil {
		return nil, apperror.NewBadRequest("guest codes aren't available on this site")
	}
	code := normalizeGuestCode(in.Code)
	if code == "" {
		return nil, apperror.NewBadRequest("type the code your game master gave you")
	}

	if in.SessionUserID != "" {
		return s.joinSignedIn(ctx, code, in.SessionUserID)
	}

	name := strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(name); n < 2 || n > 100 {
		return nil, apperror.NewBadRequest("type a name of 2 to 100 characters")
	}
	// A closed site takes no new people, guests included.
	if mode, err := s.registrationMode(ctx); err != nil || mode == registrationClosed {
		return nil, apperror.NewForbidden("this site isn't taking new people right now")
	}

	codeID, campaignID, err := s.guestCodes.ClaimGuestCode(ctx, code)
	if err != nil {
		return nil, err
	}
	id := generateUUID()
	user := &User{
		ID:              id,
		Email:           "guest+" + id + guestEmailDomain,
		DisplayName:     name,
		PasswordHash:    "!guest!" + randomHex(32),
		CreatedAt:       time.Now().UTC(),
		GuestCampaignID: &campaignID,
	}
	if err := s.repo.Create(ctx, user); err != nil {
		s.guestCodes.ReleaseGuestCode(ctx, codeID)
		return nil, apperror.NewInternal(fmt.Errorf("creating guest: %w", err))
	}
	if err := s.guestCodes.AdmitWithGuestCode(ctx, codeID, campaignID, id); err != nil {
		s.guestCodes.ReleaseGuestCode(ctx, codeID)
		s.endGuest(ctx, user)
		return nil, err
	}
	token, err := s.createSession(ctx, user, in.IP, in.UserAgent)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating session: %w", err))
	}
	slog.Info("guest joined with a code", slog.String("user_id", id), slog.String("campaign_id", campaignID))
	return &JoinResult{CampaignID: campaignID, SessionToken: token, User: user}, nil
}

// joinSignedIn adds an existing account to the code's campaign as a player.
func (s *authService) joinSignedIn(ctx context.Context, code, userID string) (*JoinResult, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.GuestCampaignID != nil {
		return nil, apperror.NewConflict("you're a guest in one campaign; keep your account first, then you can join others")
	}
	codeID, campaignID, err := s.guestCodes.ClaimGuestCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if err := s.guestCodes.AdmitWithGuestCode(ctx, codeID, campaignID, userID); err != nil {
		s.guestCodes.ReleaseGuestCode(ctx, codeID)
		return nil, err
	}
	return &JoinResult{CampaignID: campaignID, User: user}, nil
}

// guestUser loads the signed-in person and checks they are a guest.
func (s *authService) guestUser(ctx context.Context, userID string) (*User, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.GuestCampaignID == nil {
		return nil, apperror.NewBadRequest("this is already a full account")
	}
	return user, nil
}

// KeepGuestAccount gives a guest an email and password, lifting the fence.
// Every session is replaced so none keeps the old guest limits or lack of
// them; the returned token is the new one.
func (s *authService) KeepGuestAccount(ctx context.Context, userID string, in KeepGuestInput, ip, userAgent string) (string, error) {
	user, err := s.guestUser(ctx, userID)
	if err != nil {
		return "", err
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if _, err := mail.ParseAddress(email); err != nil || len(email) > 255 || strings.HasSuffix(email, ".invalid") {
		return "", apperror.NewBadRequest("type a real email address")
	}
	if n := len(in.Password); n < 8 || n > 128 {
		return "", apperror.NewBadRequest("use a password of 8 to 128 characters")
	}
	if mode, err := s.registrationMode(ctx); err != nil || mode == registrationClosed {
		return "", apperror.NewForbidden("this site isn't taking new accounts right now; you can keep playing as a guest")
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return "", apperror.NewInternal(fmt.Errorf("hashing password: %w", err))
	}
	if err := s.repo.KeepGuestAccount(ctx, user.ID, email, hash); err != nil {
		return "", err
	}
	user.Email, user.PasswordHash, user.GuestCampaignID = email, hash, nil
	if _, err := s.DestroyAllUserSessions(ctx, user.ID); err != nil {
		slog.Warn("ending guest sessions", slog.String("user_id", user.ID), slog.Any("error", err))
	}
	token, err := s.createSession(ctx, user, ip, userAgent)
	if err != nil {
		return "", apperror.NewInternal(fmt.Errorf("creating session: %w", err))
	}
	slog.Info("guest kept their account", slog.String("user_id", user.ID))
	return token, nil
}

// MergeGuest moves a guest's place in their campaign, characters and notes
// to an account they already have, then ends the guest. The other account's
// password (and code, when it has two-factor) is checked like a sign-in.
func (s *authService) MergeGuest(ctx context.Context, guestID string, in MergeGuestInput, ip, userAgent string) (string, *User, error) {
	guest, err := s.guestUser(ctx, guestID)
	if err != nil {
		return "", nil, err
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if delay := s.loginThrottleDelay(ctx, email); delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", nil, ctx.Err()
		}
	}
	target, err := s.repo.FindByEmail(ctx, email)
	if err != nil || !verifyPassword(in.Password, target.PasswordHash) {
		s.recordLoginFailure(ctx, email)
		return "", nil, apperror.NewUnauthorized("that email and password don't match an account")
	}
	if target.IsDisabled {
		return "", nil, apperror.NewForbidden("that account has been disabled")
	}
	if target.GuestCampaignID != nil || target.ID == guest.ID {
		return "", nil, apperror.NewBadRequest("that's a guest account too; merge into a full account")
	}
	if err := s.verifySecondFactorIfOn(ctx, target, in.Code); err != nil {
		return "", nil, err
	}
	s.clearLoginFailures(ctx, email)

	campaignID := *guest.GuestCampaignID
	for _, fn := range s.onGuestMerged {
		if err := fn(ctx, campaignID, guest.ID, target.ID); err != nil {
			return "", nil, apperror.NewInternal(fmt.Errorf("moving guest's things: %w", err))
		}
	}
	s.endGuest(ctx, guest)
	token, err := s.createSession(ctx, target, ip, userAgent)
	if err != nil {
		return "", nil, apperror.NewInternal(fmt.Errorf("creating session: %w", err))
	}
	slog.Info("guest merged into an account", slog.String("guest_id", guest.ID), slog.String("user_id", target.ID))
	return token, target, nil
}

// EndGuest removes a guest's account once they have no campaign: removed by
// the owner, merged, or their campaign deleted. Full accounts are untouched.
func (s *authService) EndGuest(ctx context.Context, userID string) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil || user.GuestCampaignID == nil {
		return
	}
	s.endGuest(ctx, user)
}

// EndCampaignGuests ends every guest of a deleted campaign.
func (s *authService) EndCampaignGuests(ctx context.Context, campaignID string) {
	ids, err := s.repo.ListCampaignGuests(ctx, campaignID)
	if err != nil {
		slog.Warn("listing guests of deleted campaign", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return
	}
	for _, id := range ids {
		s.EndGuest(ctx, id)
	}
}

// endGuest empties the guest's row the way a deleted account is emptied,
// so anything they wrote keeps a "Former member" signature.
func (s *authService) endGuest(ctx context.Context, user *User) {
	junk := make([]byte, 32)
	_, _ = rand.Read(junk)
	email := "deleted+" + user.ID + "@deleted.invalid"
	if err := s.repo.AnonymizeUser(ctx, user.ID, email, DeletedDisplayName, "!deleted!"+hex.EncodeToString(junk)); err != nil {
		slog.Warn("ending guest account", slog.String("user_id", user.ID), slog.Any("error", err))
		return
	}
	if _, err := s.DestroyAllUserSessions(ctx, user.ID); err != nil {
		slog.Warn("ending guest sessions", slog.String("user_id", user.ID), slog.Any("error", err))
	}
	for _, fn := range s.onAccountDeleted {
		fn(ctx, user.ID)
	}
	slog.Info("guest account ended", slog.String("user_id", user.ID))
}

// IsGuestEmail reports whether an address is a guest's placeholder.
func IsGuestEmail(email string) bool {
	return strings.HasSuffix(strings.ToLower(email), guestEmailDomain)
}
