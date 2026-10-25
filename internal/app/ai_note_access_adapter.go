package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// aiNoteAccess tells AI Import which shared notes the owner lets it change:
// the switches from the plugin's store, and who a note's writer is from the
// campaign's members. A writer it can't place counts as a player, whose
// notes are never changed.
type aiNoteAccess struct {
	store     ai_workspace.NoteAccessStore
	campaigns campaigns.CampaignService
}

func (a aiNoteAccess) NoteAccess(ctx context.Context, campaignID string) (records.NoteAccess, error) {
	return a.store.Get(ctx, campaignID)
}

func (a aiNoteAccess) NoteWriter(ctx context.Context, campaignID, userID string) (records.NoteWriter, error) {
	m, err := a.campaigns.GetMember(ctx, campaignID, userID)
	if err != nil || m == nil {
		return records.NoteWriter{Kind: records.WriterPlayer}, nil
	}
	w := records.NoteWriter{Name: m.DisplayName, Kind: records.WriterPlayer}
	granted, err := a.campaigns.IsUserDmGranted(ctx, campaignID, userID)
	switch {
	case err == nil && granted:
		w.Kind = records.WriterCoDM
	case m.Role == campaigns.RoleScribe:
		w.Kind = records.WriterScribe
	}
	return w, nil
}
