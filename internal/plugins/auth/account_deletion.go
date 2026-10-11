package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// DeletedDisplayName is what a deleted account is shown as wherever its
// pages, posts and shared notes stay behind.
const DeletedDisplayName = "Former member"

// deleteConfirmWord is what the person types to confirm. The page checks it
// too, but the server is the one that counts.
const deleteConfirmWord = "DELETE"

// OwnedCampaignRef is a campaign that blocks deletion until it is handed
// over or deleted.
type OwnedCampaignRef struct {
	ID          string
	Name        string
	MemberCount int
}

// AccountDeletionHooks are the campaign-side steps of deleting an account,
// implemented by an adapter over the campaigns service.
type AccountDeletionHooks interface {
	// OwnedCampaigns lists campaigns the user owns, archived ones included.
	OwnedCampaigns(ctx context.Context, userID string) ([]OwnedCampaignRef, error)
	// LeaveAllCampaigns removes the user from every campaign the way an
	// owner's Remove does. It refuses while the user still owns one.
	LeaveAllCampaigns(ctx context.Context, userID string) error
}

// ConfigureAccountDeletion wires the campaign-side steps. Without them
// deletion is refused, so an account can never be removed while still
// holding campaigns.
func ConfigureAccountDeletion(svc AuthService, hooks AccountDeletionHooks) {
	if s, ok := svc.(*authService); ok {
		s.accountDeletion = hooks
	}
}

// OnAccountDeleted registers fn to run once an account is gone, for data
// other plugins hold about the person (character links, private notes,
// API keys). A failing step is logged; the account stays deleted.
func OnAccountDeleted(svc AuthService, fn func(ctx context.Context, userID string)) {
	if s, ok := svc.(*authService); ok && fn != nil {
		s.onAccountDeleted = append(s.onAccountDeleted, fn)
	}
}

// DeleteAccountInput is what the Delete account form sends.
type DeleteAccountInput struct {
	Password string `json:"password"`
	Confirm  string `json:"confirm"`
	// Code is asked for only when two-factor is on.
	Code string `json:"code"`
}

// OwnedCampaigns lists what blocks this person from deleting their account.
func (s *authService) OwnedCampaigns(ctx context.Context, userID string) ([]OwnedCampaignRef, error) {
	if s.accountDeletion == nil {
		return nil, nil
	}
	return s.accountDeletion.OwnedCampaigns(ctx, userID)
}

// DeleteOwnAccount removes the signed-in person's account.
//
// The users row is kept and emptied rather than deleted: pages, relations
// and uploads point at it with foreign keys that would either block the
// delete or cascade away content other members rely on. What stays is a
// disabled row named "Former member" with no email, password or profile,
// so their shared work keeps a signature and nothing personal remains.
func (s *authService) DeleteOwnAccount(ctx context.Context, userID string, in DeleteAccountInput) error {
	if strings.TrimSpace(in.Confirm) != deleteConfirmWord {
		return apperror.NewBadRequest("type DELETE to confirm")
	}
	if s.accountDeletion == nil {
		return apperror.NewInternal(fmt.Errorf("account deletion is not wired"))
	}
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !verifyPassword(in.Password, user.PasswordHash) {
		return apperror.NewBadRequest("that password isn't right")
	}
	if err := s.verifySecondFactorIfOn(ctx, user, in.Code); err != nil {
		return err
	}
	if user.IsAdmin {
		n, err := s.repo.CountAdmins(ctx)
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("counting admins: %w", err))
		}
		if n <= 1 {
			return apperror.NewConflict("you're the only site admin; make someone else an admin first")
		}
	}
	owned, err := s.accountDeletion.OwnedCampaigns(ctx, userID)
	if err != nil {
		return err
	}
	if len(owned) > 0 {
		return apperror.NewConflict("hand over or delete the campaigns you own first")
	}
	if err := s.accountDeletion.LeaveAllCampaigns(ctx, userID); err != nil {
		return err
	}
	if user.AvatarPath != nil && *user.AvatarPath != "" {
		if err := s.ClearAvatar(ctx, userID); err != nil {
			slog.Warn("clearing avatar of deleted account", slog.String("user_id", userID), slog.Any("error", err))
		}
	}

	// An unguessable hash no password can match, so the row can never be
	// signed into even if is_disabled were cleared by hand.
	junk := make([]byte, 32)
	if _, err := rand.Read(junk); err != nil {
		return apperror.NewInternal(fmt.Errorf("generating placeholder: %w", err))
	}
	email := "deleted+" + userID + "@deleted.invalid"
	if err := s.repo.AnonymizeUser(ctx, userID, email, DeletedDisplayName, "!deleted!"+hex.EncodeToString(junk)); err != nil {
		return apperror.NewInternal(fmt.Errorf("removing account: %w", err))
	}
	if _, err := s.DestroyAllUserSessions(ctx, userID); err != nil {
		slog.Warn("ending sessions of deleted account", slog.String("user_id", userID), slog.Any("error", err))
	}
	for _, fn := range s.onAccountDeleted {
		fn(ctx, userID)
	}
	slog.Info("account deleted by its owner", slog.String("user_id", userID))
	return nil
}
