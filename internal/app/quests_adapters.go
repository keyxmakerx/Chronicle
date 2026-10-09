package app

// quests_adapters.go bridges the quests plugin to the entities, maps and
// campaigns services. The quests plugin owns the narrow interfaces; these
// adapters live here so it never imports another plugin.

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// questEntityAdapter implements quests.EntityDirectory over the entities
// service, with names read in one query per load through PageCards.
type questEntityAdapter struct {
	svc   entities.EntityService
	cards entities.PageCards
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
	cards, err := a.cards.InCampaign(ctx, campaignID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]quests.EntityInfo, len(cards))
	for id, c := range cards {
		out[id] = quests.EntityInfo{ID: c.ID, Name: c.Name, TypeName: c.TypeName, TypeSlug: c.TypeSlug, ImagePath: c.ImagePath}
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

// questMapAdapter implements quests.MapDirectory over the maps service. With
// the maps addon off it finds no maps, so quests cannot pin or link one and
// never points at the addon's routes.
type questMapAdapter struct {
	svc    maps.MapService
	addons questAddonChecker
}

// questAddonChecker is the slice of the addons service the map adapter needs.
type questAddonChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
}

func (a *questMapAdapter) Enabled(ctx context.Context, campaignID string) (bool, error) {
	return a.addons.IsEnabledForCampaign(ctx, campaignID, "maps")
}

// Maps reads the campaign's map list once and picks the requested ones, so a
// board with many map pins costs one lookup, and a foreign id is never found.
func (a *questMapAdapter) Maps(ctx context.Context, campaignID string, ids []string) (map[string]quests.MapInfo, error) {
	out := make(map[string]quests.MapInfo, len(ids))
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			want[id] = true
		}
	}
	if len(want) == 0 {
		return out, nil
	}
	ms, err := a.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		if want[m.ID] {
			out[m.ID] = quests.MapInfo{ID: m.ID, Name: m.Name}
		}
	}
	return out, nil
}

func (a *questMapAdapter) ListMaps(ctx context.Context, campaignID string) ([]quests.MapInfo, error) {
	on, err := a.Enabled(ctx, campaignID)
	if err != nil || !on {
		return nil, err
	}
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

// questTypeAdapter implements quests.TypeDirectory over the entities service.
type questTypeAdapter struct {
	svc entities.EntityService
}

// TypeInCampaign treats a missing type and one of another campaign alike, as
// not found, so a foreign category id cannot be probed through the boards.
func (a *questTypeAdapter) TypeInCampaign(ctx context.Context, campaignID string, typeID int) (bool, error) {
	et, err := a.svc.GetEntityTypeByID(ctx, typeID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return et.CampaignID == campaignID, nil
}

// questAnnouncerAdapter implements quests.Announcer over the websocket bus.
type questAnnouncerAdapter struct {
	bus ws.EventBus
}

func (a *questAnnouncerAdapter) QuestChanged(campaignID, entityID string, version int, dmOnly bool) {
	msg := ws.NewMessage(ws.MsgQuestUpdated, campaignID, entityID, map[string]int{"version": version})
	msg.RequiresDM = dmOnly
	a.bus.Publish(msg)
}

func (a *questAnnouncerAdapter) BoardsChanged(campaignID string, h quests.Home, dmOnly bool) {
	id, home := h.EntityID, "page"
	if h.IsType() {
		id, home = strconv.Itoa(h.TypeID), "category"
	}
	msg := ws.NewMessage(ws.MsgNoticeBoardsUpdated, campaignID, id, map[string]string{"home": home})
	msg.RequiresDM = dmOnly
	a.bus.Publish(msg)
}
