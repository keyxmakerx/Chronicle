package app

// map_grants_adapter.go tells the maps plugin which maps have been given to
// whom. A map is given as its handout item (armory's Give, see
// armory_give_adapters.go): an item page named "Map: <name>" that points at
// the map and carries the handout marker. A player sees a map while a
// character they own holds that item on a "Has Item" line the player can see
// and the Director made: the Owner or a co-DM, as Give does. Anyone who can
// edit a character can add a Has Item line by hand, so a line made by anyone
// else gives nothing, and a player who re-adds a line the GM took back gets
// nothing either.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// holdersTTL is how long a map's holders are remembered for live messages,
// which can come many times a second while a token is dragged. Access itself
// is never cached: GivenMapIDs reads fresh on every request.
const holdersTTL = 5 * time.Second

type mapGrantsAdapter struct {
	entities  entities.EntityService
	relations relations.RelationService
	members   interface {
		ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
		IsUserDmGranted(ctx context.Context, campaignID, userID string) (bool, error)
	}

	mu      sync.Mutex
	holders map[string]holdersEntry
}

type holdersEntry struct {
	users []string
	at    time.Time
}

var _ maps.MapGrants = (*mapGrantsAdapter)(nil)

// handoutMapOf returns the map a held item is the handout of, or "". The
// marker is checked as well as the assigned map, so an item someone merely
// pointed at a map grants nothing.
func handoutMapOf(e *entities.Entity) string {
	if e == nil || e.MapID == nil || *e.MapID == "" {
		return ""
	}
	if v, _ := e.FieldsData[handoutMarkerField].(string); v != *e.MapID {
		return ""
	}
	return *e.MapID
}

func (a *mapGrantsAdapter) GivenMapIDs(ctx context.Context, campaignID, userID string) (map[string]bool, error) {
	out := map[string]bool{}
	if userID == "" {
		return out, nil
	}
	owned, err := a.entities.ListByOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	var directors func(string) (bool, error)
	for _, c := range owned {
		rels, err := a.relations.ListByEntity(ctx, campaignID, c.ID)
		if err != nil {
			return nil, err
		}
		for _, r := range rels {
			// Only a handout's name is looked up, so ordinary items cost
			// nothing; a Director-only line is one the player can't see.
			if r.RelationType != hasItemType || r.DmOnly || !strings.HasPrefix(r.TargetEntityName, armory.HandoutPrefix) {
				continue
			}
			// A line held at zero (mid-move, before it is dropped) holds nothing.
			if carriedQuantity(r.Metadata) < 1 {
				continue
			}
			if directors == nil {
				if directors, err = a.directorCheck(ctx, campaignID); err != nil {
					return nil, err
				}
			}
			ok, err := directors(r.CreatedBy)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			e, err := a.entities.GetByID(ctx, r.TargetEntityID)
			if err != nil {
				continue
			}
			if e.CampaignID != campaignID {
				continue
			}
			if id := handoutMapOf(e); id != "" {
				out[id] = true
			}
		}
	}
	return out, nil
}

// carriedQuantity reads a Has Item line's quantity as armory does: absent
// means one, anything unreadable counts as none.
func carriedQuantity(meta []byte) float64 {
	if len(meta) == 0 {
		return 1
	}
	var m struct {
		Quantity *float64 `json:"quantity"`
	}
	if err := json.Unmarshal(meta, &m); err != nil {
		return 0
	}
	if m.Quantity == nil {
		return 1
	}
	return *m.Quantity
}

// directorCheck answers, for this request, whether a user is the campaign's
// Owner or holds a co-DM grant now. A creator who has left, or was demoted,
// no longer counts.
func (a *mapGrantsAdapter) directorCheck(ctx context.Context, campaignID string) (func(string) (bool, error), error) {
	members, err := a.members.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	roles := make(map[string]campaigns.Role, len(members))
	for _, m := range members {
		roles[m.UserID] = m.Role
	}
	seen := map[string]bool{}
	return func(userID string) (bool, error) {
		role, member := roles[userID]
		if userID == "" || !member {
			return false, nil
		}
		if role >= campaigns.RoleOwner {
			return true, nil
		}
		if ok, done := seen[userID]; done {
			return ok, nil
		}
		ok, err := a.members.IsUserDmGranted(ctx, campaignID, userID)
		if err != nil {
			return false, err
		}
		seen[userID] = ok
		return ok, nil
	}, nil
}

func (a *mapGrantsAdapter) MapHolders(ctx context.Context, campaignID, mapID string) ([]string, error) {
	key := campaignID + "/" + mapID
	a.mu.Lock()
	if h, ok := a.holders[key]; ok && time.Since(h.at) < holdersTTL {
		a.mu.Unlock()
		return h.users, nil
	}
	a.mu.Unlock()

	members, err := a.members.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	users := []string{}
	for _, m := range members {
		if maps.SeesAllMaps(int(m.Role)) || int(m.Role) < permissions.RolePlayer {
			continue
		}
		given, err := a.GivenMapIDs(ctx, campaignID, m.UserID)
		if err != nil {
			return nil, err
		}
		if given[mapID] {
			users = append(users, m.UserID)
		}
	}
	a.mu.Lock()
	if a.holders == nil {
		a.holders = map[string]holdersEntry{}
	}
	a.holders[key] = holdersEntry{users: users, at: time.Now()}
	a.mu.Unlock()
	return users, nil
}
