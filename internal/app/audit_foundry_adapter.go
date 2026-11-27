package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// auditFoundryAdapter gives the campaign's Recent Activity the sync history
// rows a person would call a change: what Foundry wrote into Chronicle, the
// world connecting, linking and unlinking, and each conflict the module
// settled. Reads, failures and Chronicle's own pushes stay on the Sync
// history page, where they are diagnosis rather than activity.
type auditFoundryAdapter struct {
	repo syncapi.SyncHistoryRepository
}

func (a auditFoundryAdapter) FoundryActivity(ctx context.Context, campaignID string, limit int) ([]audit.FoundryEvent, error) {
	evs, err := a.repo.List(ctx, campaignID, syncapi.SyncHistoryFilter{Limit: limit})
	if err != nil {
		return nil, err
	}
	var out []audit.FoundryEvent
	var add func(e syncapi.SyncEvent)
	add = func(e syncapi.SyncEvent) {
		if foundryActivityRow(e) {
			out = append(out, audit.FoundryEvent{At: e.OccurredAt, Kind: e.Kind, ResourceID: e.ResourceID, Name: e.ResourceName,
				Action: e.Action, Direction: e.Direction, Status: e.Status, Message: e.Message})
		}
		for _, c := range e.Children {
			add(c)
		}
	}
	for _, e := range evs {
		add(e)
	}
	return out, nil
}

// foundryActivityRow picks the rows Recent Activity lists.
func foundryActivityRow(e syncapi.SyncEvent) bool {
	switch {
	case e.Status == "conflict":
		return true
	case !e.OK:
		return false
	case e.Direction == syncapi.DirLink:
		return e.Kind == "connection" || e.Kind == "link"
	case e.Direction == syncapi.DirToChronicle:
		return e.RecordedByChronicle()
	}
	return false
}
