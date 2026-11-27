// store_prefs.go reads the person's own choice to put the top bar's Store
// icon away in one campaign, for the owner's desk row that brings it back.
// It mirrors the brush's choice (customize_brush.go): the campaigns plugin
// reads display choices through the auth service, never its tables.

package campaigns

import (
	"context"
	"log/slog"
)

// StorePrefs reads one person's choice to hide the top bar's Store icon in
// one campaign.
type StorePrefs interface {
	StoreHidden(ctx context.Context, userID, campaignID string) (bool, error)
}

// SetStorePrefs wires the person's Store icon choice.
func (h *Handler) SetStorePrefs(p StorePrefs) { h.storePrefs = p }

// storeHidden reports the person's choice in one campaign; one that cannot
// be read leaves the icon shown, its default.
func (h *Handler) storeHidden(ctx context.Context, userID, campaignID string) bool {
	if h.storePrefs == nil || userID == "" {
		return false
	}
	hidden, err := h.storePrefs.StoreHidden(ctx, userID, campaignID)
	if err != nil {
		slog.Warn("store choice not read", slog.String("user_id", userID), slog.Any("error", err))
		return false
	}
	return hidden
}
