package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// aiFoundryScenesAdapter implements records.FoundryScenesAPI over the sync
// API's scene store. The lookup itself keeps the list to Directors.
type aiFoundryScenesAdapter struct {
	repo syncapi.FoundrySceneRepository
}

var _ records.FoundryScenesAPI = aiFoundryScenesAdapter{}

func (a aiFoundryScenesAdapter) FoundryScenes(ctx context.Context, campaignID string) (*records.FoundryScenes, error) {
	rep, err := a.repo.Get(ctx, campaignID)
	if err != nil || rep == nil {
		return nil, err
	}
	out := &records.FoundryScenes{Truncated: rep.Truncated, ReportedAt: rep.ReportedAt,
		Scenes: make([]records.FoundrySceneRef, 0, len(rep.Scenes))}
	for _, s := range rep.Scenes {
		out.Scenes = append(out.Scenes, records.FoundrySceneRef{Name: s.Name, Folder: s.Folder, Active: s.Active, Nav: s.InNavigation})
	}
	return out, nil
}
