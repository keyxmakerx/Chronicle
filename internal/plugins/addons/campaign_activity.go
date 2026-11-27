package addons

import (
	"context"
	"log/slog"
)

// CampaignActivityLogger is the slice of a campaign's own activity log this
// plugin writes to when a feature is switched on or off for that campaign.
// Declared here rather than imported from the audit plugin so the plugins
// stay independent; app wiring supplies the adapter. Site-wide add-on changes
// go to the admin change log instead (ActivityRecorder).
type CampaignActivityLogger interface {
	LogCampaignActivity(ctx context.Context, campaignID, userID, action, subject string) error
}

// Campaign activity actions for a feature switch. The audit plugin gives them
// words ("switched on", "switched off"); subject is the feature's name.
const (
	campaignActionAddonEnabled  = "addon.enabled"
	campaignActionAddonDisabled = "addon.disabled"
)

// SetCampaignActivityLogger wires the campaign activity log. Optional: nil
// disables it.
func (h *Handler) SetCampaignActivityLogger(l CampaignActivityLogger) {
	h.campaignActivity = l
}

// logCampaignToggle records a campaign feature switch after it succeeded.
// Best-effort: a failed write is logged and never fails the toggle.
func (h *Handler) logCampaignToggle(ctx context.Context, campaignID, userID string, enabled bool, feature string) {
	if h.campaignActivity == nil {
		return
	}
	action := campaignActionAddonDisabled
	if enabled {
		action = campaignActionAddonEnabled
	}
	if err := h.campaignActivity.LogCampaignActivity(ctx, campaignID, userID, action, feature); err != nil {
		slog.Warn("campaign activity log failed", slog.String("action", action), slog.Any("error", err))
	}
}
