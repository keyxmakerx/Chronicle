package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

// leave_adapters.go joins a member leaving a campaign to the plugins that
// hold what they leave behind, so campaigns never import them.

// leaveCharacterStore is the entity repository's release of a claim.
type leaveCharacterStore interface {
	ReleaseOwned(ctx context.Context, campaignID, userID string) ([]string, error)
	ClaimedNames(ctx context.Context, userID string, campaignIDs []string) (map[string][]string, error)
}

// leaveCharacterAdapter implements campaigns.CharacterReleaser.
type leaveCharacterAdapter struct {
	store leaveCharacterStore
}

var _ campaigns.CharacterReleaser = (*leaveCharacterAdapter)(nil)

func (a *leaveCharacterAdapter) ReleaseCharacters(ctx context.Context, campaignID, userID string) ([]string, error) {
	return a.store.ReleaseOwned(ctx, campaignID, userID)
}

func (a *leaveCharacterAdapter) ClaimedCharacters(ctx context.Context, userID string, campaignIDs []string) (map[string][]string, error) {
	return a.store.ClaimedNames(ctx, userID, campaignIDs)
}

// leaveNotifierAdapter implements campaigns.LeaveNotifier on the sessions
// plugin's notification store.
type leaveNotifierAdapter struct {
	svc sessions.SessionService
}

var _ campaigns.LeaveNotifier = (*leaveNotifierAdapter)(nil)

func (a *leaveNotifierAdapter) MemberLeft(ctx context.Context, campaignID string, userIDs []string, message, detail, action, link string) error {
	return a.svc.NotifyUsersWithAction(ctx, userIDs, campaignID, campaigns.NotifMemberLeft, message, detail, action, link)
}
