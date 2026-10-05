package app

// quests_adapters.go bridges the quests plugin to the entities, maps and
// campaigns services. The quests plugin owns the narrow interfaces; these
// adapters live here so it never imports another plugin.

import (
	"context"
	"errors"
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
)

// questEntityAdapter implements quests.EntityDirectory over the entities service.
type questEntityAdapter struct {
	svc entities.EntityService
}

func entityInfo(e *entities.Entity) quests.EntityInfo {
	info := quests.EntityInfo{ID: e.ID, Name: e.Name, TypeName: e.TypeName, TypeSlug: e.TypeSlug}
	if e.ImagePath != nil {
		info.ImagePath = *e.ImagePath
	}
	return info
}

// Entities skips a missing page and one in another campaign (a clean absence,
// not an error), so a foreign id can never be resolved through this plugin.
func (a *questEntityAdapter) Entities(ctx context.Context, campaignID string, ids []string) (map[string]quests.EntityInfo, error) {
	out := make(map[string]quests.EntityInfo, len(ids))
	for _, id := range ids {
		if _, done := out[id]; done || id == "" {
			continue
		}
		e, err := a.svc.GetByID(ctx, id)
		if err != nil {
			var appErr *apperror.AppError
			if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
				continue
			}
			return nil, err
		}
		if e.CampaignID == campaignID {
			out[id] = entityInfo(e)
		}
	}
	return out, nil
}

// FilterViewable delegates to the entities plugin's one visibility predicate.
func (a *questEntityAdapter) FilterViewable(ctx context.Context, campaignID string, ids []string, role int, userID string) (map[string]bool, error) {
	return a.svc.FilterViewableEntityIDs(ctx, campaignID, ids, role, userID)
}

// Search reuses the entity name search (all types).
func (a *questEntityAdapter) Search(ctx context.Context, campaignID, query string, role int, userID string, limit int) ([]quests.EntityInfo, error) {
	opts := entities.DefaultListOptions()
	opts.PerPage = limit
	found, _, err := a.svc.Search(ctx, campaignID, query, 0, role, userID, opts)
	if err != nil {
		return nil, err
	}
	out := make([]quests.EntityInfo, 0, len(found))
	for i := range found {
		out = append(out, entityInfo(&found[i]))
	}
	return out, nil
}

// questMapAdapter implements quests.MapDirectory over the maps service.
type questMapAdapter struct {
	svc maps.MapService
}

func (a *questMapAdapter) Maps(ctx context.Context, campaignID string, ids []string) (map[string]quests.MapInfo, error) {
	out := make(map[string]quests.MapInfo, len(ids))
	for _, id := range ids {
		if _, done := out[id]; done || id == "" {
			continue
		}
		m, err := a.svc.GetMap(ctx, id)
		if err != nil {
			var appErr *apperror.AppError
			if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
				continue
			}
			return nil, err
		}
		if m.CampaignID == campaignID {
			out[id] = quests.MapInfo{ID: m.ID, Name: m.Name}
		}
	}
	return out, nil
}

func (a *questMapAdapter) ListMaps(ctx context.Context, campaignID string) ([]quests.MapInfo, error) {
	ms, err := a.svc.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]quests.MapInfo, 0, len(ms))
	for _, m := range ms {
		out = append(out, quests.MapInfo{ID: m.ID, Name: m.Name})
	}
	return out, nil
}

// questMemberNamesAdapter implements quests.MemberNames from the member list.
type questMemberNamesAdapter struct {
	svc campaigns.CampaignService
}

func (a *questMemberNamesAdapter) DisplayNames(ctx context.Context, campaignID string, _ []string) (map[string]string, error) {
	members, err := a.svc.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(members))
	for _, m := range members {
		out[m.UserID] = m.DisplayName
	}
	return out, nil
}

// questCharacterAdapter implements quests.CharacterDirectory over the Armory's
// character listing (the same family the Armory's own give box offers), with
// the claiming member's display name.
type questCharacterAdapter struct {
	dir   *armoryStashDirectoryAdapter
	names *questMemberNamesAdapter
}

func (a *questCharacterAdapter) ListCharacters(ctx context.Context, campaignID string, role int, userID string) ([]quests.CharacterInfo, error) {
	chars, err := a.dir.ListCharacters(ctx, campaignID, role, userID)
	if err != nil {
		return nil, err
	}
	names, err := a.names.DisplayNames(ctx, campaignID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]quests.CharacterInfo, 0, len(chars))
	for _, c := range chars {
		info := quests.CharacterInfo{ID: c.ID, Name: c.Name}
		if c.OwnerUserID != "" {
			info.Player = names[c.OwnerUserID]
			if info.Player == "" {
				info.Player = "a player"
			}
		}
		out = append(out, info)
	}
	return out, nil
}
