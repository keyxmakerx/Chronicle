package foundry_vtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Settings KV keys for the auto-pin banner (C-FMC-8).
//
// LatestAutoPinSummaryKey holds the JSON-serialized AutoPinSummary
// of the most recent install that auto-pinned campaigns. Overwritten
// on every install (only the latest summary is surfaced; older
// summaries are still queryable via the security_events audit log).
//
// AutoPinBannerDismissedAtKey holds a Unix-second timestamp string
// of the last time the admin dismissed the banner. The banner shows
// iff the latest summary's timestamp is strictly greater than the
// dismissal timestamp.
const (
	LatestAutoPinSummaryKey       = "foundry_vtt.latest_autopin_summary"
	AutoPinBannerDismissedAtKey   = "foundry_vtt.autopin_banner_dismissed_at"
)

// AutoPinSummary is the renderable bundle the admin banner displays.
// Populated by AutoPinOnInstall + serialized to the settings KV;
// read back by GetUnreadAutoPinSummary.
type AutoPinSummary struct {
	// PreviousVersion is the version campaigns were effectively
	// running before this install. The banner phrases it as the
	// version campaigns are now pinned TO.
	PreviousVersion string `json:"previous_version"`
	// NewVersion is the version that just got installed. The banner
	// says "you installed N; M campaigns were auto-pinned to <prev>
	// — bump them to <new> via..." linking back to admin actions.
	NewVersion string `json:"new_version"`
	// Affected is the count of campaigns auto-pinned by this install.
	Affected int `json:"affected"`
	// Timestamp is when the install fired the auto-pin (Unix seconds).
	// Drives the unread/dismissed comparison.
	Timestamp int64 `json:"timestamp"`
}

// storeAutoPinSummary serializes summary to the settings KV under
// LatestAutoPinSummaryKey. Called by AutoPinOnInstall after the
// per-campaign fan-out completes. Soft-fails (logs but doesn't
// abort the install) if kv is nil or the write errors — the
// summary is supplementary; missing it doesn't break installs.
func (s *service) storeAutoPinSummary(ctx context.Context, summary AutoPinSummary) error {
	if s.kv == nil {
		return nil // KV not wired (tests); skip silently
	}
	bytes, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal autopin summary: %w", err)
	}
	return s.kv.Set(ctx, LatestAutoPinSummaryKey, string(bytes))
}

// GetUnreadAutoPinSummary returns the latest summary if unread,
// nil if no summary exists or the admin has already dismissed it.
//
// Read-only: doesn't touch the dismissal key. The banner handler
// re-renders empty when this returns nil; the dismiss handler is
// what updates the dismissal timestamp.
func (s *service) GetUnreadAutoPinSummary(ctx context.Context) (*AutoPinSummary, error) {
	if s.kv == nil {
		return nil, nil
	}
	raw, err := s.kv.Get(ctx, LatestAutoPinSummaryKey)
	if err != nil || raw == "" {
		// Missing key returns an apperror.NotFound-shaped error from
		// settings.Get. Treat any error as "no summary to surface" —
		// the banner is supplementary, not load-bearing, so we don't
		// abort the page render over a KV read issue.
		return nil, nil
	}
	var summary AutoPinSummary
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return nil, fmt.Errorf("parse stored autopin summary: %w", err)
	}

	// Check dismissal. A summary timestamp <= dismissed_at means
	// the admin has already acknowledged this install.
	dismissedRaw, _ := s.kv.Get(ctx, AutoPinBannerDismissedAtKey)
	if dismissedRaw != "" {
		dismissed, parseErr := strconv.ParseInt(dismissedRaw, 10, 64)
		if parseErr == nil && summary.Timestamp <= dismissed {
			return nil, nil
		}
	}
	return &summary, nil
}

// DismissAutoPinBanner stamps the current Unix timestamp into the
// dismissal settings key. Subsequent calls to GetUnreadAutoPinSummary
// return nil until a new install produces a summary with a fresh
// timestamp.
func (s *service) DismissAutoPinBanner(ctx context.Context) error {
	if s.kv == nil {
		return errors.New("settings KV not configured; banner state can't persist")
	}
	now := strconv.FormatInt(time.Now().Unix(), 10)
	if err := s.kv.Set(ctx, AutoPinBannerDismissedAtKey, now); err != nil {
		return fmt.Errorf("set dismissal timestamp: %w", err)
	}
	return nil
}
