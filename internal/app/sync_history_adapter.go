package app

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
)

// syncHistoryEditorAdapter answers "who last changed this page" for the sync
// history from the campaign's change log, so a change Foundry applied names
// the person who made it in Chronicle.
type syncHistoryEditorAdapter struct {
	audit audit.AuditService
}

// LastEditor returns the newest change-log author at or before the given
// time. The log is newest first and campaign-scoped.
func (a syncHistoryEditorAdapter) LastEditor(ctx context.Context, campaignID, entityID string, before time.Time) (string, bool) {
	entries, err := a.audit.GetEntityHistory(ctx, entityID, campaignID)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.UserID != "" && !e.CreatedAt.After(before) {
			return e.UserID, true
		}
	}
	return "", false
}
