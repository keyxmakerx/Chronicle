package app

// armory_give_adapters.go wires "give an item or a map to a character" to the
// maps, entities and sessions plugins. The armory plugin sees only
// armory.HandoutStore and armory.GiveNotifier; every cross-plugin call goes
// through these adapters.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

// handoutMarkerField is the fields_data key that marks an entity as a map's
// handout. It is not a field of any entity type, so no form can show or type
// it; its value is the map id.
const handoutMarkerField = "chronicle_map_handout"

// handoutScanPages bounds the walk for an earlier handout (100 per page); a
// campaign with more items than that simply makes a fresh handout.
const handoutScanPages = 20

// armoryHandoutAdapter implements armory.HandoutStore. A map is handed out as
// an item entity that points at it, so the character holds a normal "Has Item"
// line and Foundry sees an ordinary item whose description links the map.
type armoryHandoutAdapter struct {
	maps maps.MapService
	svc  entities.EntityService
	dir  *armoryStashDirectoryAdapter
}

var _ armory.HandoutStore = (*armoryHandoutAdapter)(nil)

func (a *armoryHandoutAdapter) ListMaps(ctx context.Context, campaignID string) ([]armory.NamedRef, error) {
	list, err := a.maps.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]armory.NamedRef, 0, len(list))
	for _, m := range list {
		out = append(out, armory.NamedRef{ID: m.ID, Name: m.Name})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// FindMap treats a missing map and one from another campaign alike.
func (a *armoryHandoutAdapter) FindMap(ctx context.Context, campaignID, mapID string) (*armory.NamedRef, error) {
	m, err := a.maps.GetMap(ctx, mapID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	if m == nil || m.CampaignID != campaignID {
		return nil, nil
	}
	return &armory.NamedRef{ID: m.ID, Name: m.Name}, nil
}

// pickHandout chooses the marked handout for the map among the candidates:
// the same name, the same assigned map and the marker, so a player's or
// Scribe's own item that happens to share the name is never taken over. With
// several, the oldest wins, so the choice does not move around.
func pickHandout(list []entities.Entity, name, mapID string) *entities.Entity {
	var best *entities.Entity
	for i := range list {
		e := &list[i]
		if e.Name != name || e.MapID == nil || *e.MapID != mapID || e.FieldsData[handoutMarkerField] != mapID {
			continue
		}
		if best == nil || e.CreatedAt.Before(best.CreatedAt) || (e.CreatedAt.Equal(best.CreatedAt) && e.ID < best.ID) {
			best = e
		}
	}
	return best
}

func (a *armoryHandoutAdapter) FindHandout(ctx context.Context, campaignID, name, mapID string) (*armory.EntityRef, error) {
	ti, err := a.dir.types(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	var found []entities.Entity
	for _, tid := range ti.itemIDs {
		for page := 1; page <= handoutScanPages; page++ {
			list, total, err := a.svc.List(ctx, campaignID, tid, permissions.RoleOwner, "", entities.ListOptions{Page: page, PerPage: 100, Sort: "name"})
			if err != nil {
				return nil, err
			}
			for _, e := range list {
				if ti.item[e.EntityTypeID] {
					found = append(found, e)
				}
			}
			if page*100 >= total || len(list) == 0 {
				break
			}
		}
	}
	if e := pickHandout(found, name, mapID); e != nil {
		r := a.dir.ref(ti, e)
		return &r, nil
	}
	return nil, nil
}

// handoutEntry is the handout's entry: one line linking the map's page. Foundry
// shows the entry as the item's description, so the link is how a player there
// finds the map.
func handoutEntry(campaignID string, m armory.NamedRef) (entryJSON, entryHTML string) {
	href := "/campaigns/" + campaignID + "/maps/" + m.ID
	doc := map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{
			map[string]any{"type": "text", "text": "Map: "},
			map[string]any{"type": "text", "text": m.Name, "marks": []any{
				map[string]any{"type": "link", "attrs": map[string]any{"href": href}},
			}},
		}},
	}}
	b, _ := json.Marshal(doc)
	return string(b), fmt.Sprintf(`<p>Map: <a href="%s">%s</a></p>`, html.EscapeString(href), html.EscapeString(m.Name))
}

func (a *armoryHandoutAdapter) CreateHandout(ctx context.Context, campaignID, createdBy, name string, m armory.NamedRef) (*armory.EntityRef, error) {
	ti, err := a.dir.types(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if len(ti.itemIDs) == 0 {
		return nil, apperror.NewBadRequest("This campaign has no item category to hold a map. Add one first.")
	}
	// Hidden from players until a give adds the holder to the allow list.
	e, err := a.svc.Create(ctx, campaignID, createdBy, entities.CreateEntityInput{
		Name: name, EntityTypeID: ti.itemIDs[0], IsPrivate: true,
		FieldsData: map[string]any{handoutMarkerField: m.ID},
	})
	if err != nil {
		return nil, err
	}
	entryJSON, entryHTML := handoutEntry(campaignID, m)
	if _, err := a.svc.AssignMap(ctx, e.ID, &m.ID); err != nil {
		a.discard(ctx, e.ID)
		return nil, err
	}
	if err := a.svc.UpdateEntry(ctx, e.ID, entryJSON, entryHTML); err != nil {
		a.discard(ctx, e.ID)
		return nil, err
	}
	r := a.dir.ref(ti, e)
	return &r, nil
}

// discard removes a handout that could not be finished, so the next give does
// not find a half-made item and a retry does not pile up copies.
func (a *armoryHandoutAdapter) discard(ctx context.Context, entityID string) {
	if err := a.svc.Delete(ctx, entityID); err != nil {
		slog.Warn("give: could not remove an unfinished map handout", slog.String("entity_id", entityID), slog.Any("error", err))
	}
}

// viewGrants returns the grants that make the entity visible to the users
// besides the GM, the users it added, and whether anything changed. A custom
// entity keeps every grant it has; a private default one starts from "Scribe
// and up", the same people a private entity is open to. A public default
// entity is already open to everyone and is left alone. A user who already
// holds a grant of their own is not added again.
func viewGrants(e *entities.Entity, existing []entities.EntityPermission, userIDs []string) (grants []entities.PermissionGrant, added []string, changed bool) {
	if e.Visibility != entities.VisibilityCustom {
		if !e.IsPrivate {
			return nil, nil, false
		}
		grants = []entities.PermissionGrant{{
			SubjectType: entities.SubjectRole, SubjectID: strconv.Itoa(permissions.RoleScribe), Permission: entities.PermEdit,
		}}
		changed = true
	} else {
		for _, p := range existing {
			grants = append(grants, entities.PermissionGrant{SubjectType: p.SubjectType, SubjectID: p.SubjectID, Permission: p.Permission})
		}
	}
	have := map[string]bool{}
	for _, g := range grants {
		if g.SubjectType == entities.SubjectUser {
			have[g.SubjectID] = true
		}
	}
	for _, id := range userIDs {
		if id == "" || have[id] {
			continue
		}
		have[id] = true
		grants = append(grants, entities.PermissionGrant{SubjectType: entities.SubjectUser, SubjectID: id, Permission: entities.PermView})
		added = append(added, id)
		changed = true
	}
	return grants, added, changed
}

func (a *armoryHandoutAdapter) AllowViewers(ctx context.Context, campaignID, entityID string, userIDs []string) ([]string, error) {
	e, existing, err := a.permissionsOf(ctx, campaignID, entityID)
	if err != nil {
		return nil, err
	}
	grants, added, changed := viewGrants(e, existing, userIDs)
	if !changed {
		return nil, nil
	}
	if err := a.svc.SetEntityPermissions(ctx, entityID, entities.SetPermissionsInput{
		Visibility: entities.VisibilityCustom, Permissions: grants,
	}); err != nil {
		return nil, err
	}
	return added, nil
}

// withoutViewers returns the custom grants minus the users' plain view
// grants, and whether any went. Role and group grants, and a user grant
// above view, are someone's deliberate choice and stay.
func withoutViewers(existing []entities.EntityPermission, userIDs []string) ([]entities.PermissionGrant, bool) {
	drop := map[string]bool{}
	for _, id := range userIDs {
		drop[id] = true
	}
	var grants []entities.PermissionGrant
	changed := false
	for _, p := range existing {
		if p.SubjectType == entities.SubjectUser && p.Permission == entities.PermView && drop[p.SubjectID] {
			changed = true
			continue
		}
		grants = append(grants, entities.PermissionGrant{SubjectType: p.SubjectType, SubjectID: p.SubjectID, Permission: p.Permission})
	}
	return grants, changed
}

// RevokeViewers only ever narrows a custom allow list; an entity that is not
// custom has no user grants to take back.
func (a *armoryHandoutAdapter) RevokeViewers(ctx context.Context, campaignID, entityID string, userIDs []string) error {
	e, existing, err := a.permissionsOf(ctx, campaignID, entityID)
	if err != nil {
		return err
	}
	if e.Visibility != entities.VisibilityCustom {
		return nil
	}
	grants, changed := withoutViewers(existing, userIDs)
	if !changed {
		return nil
	}
	return a.svc.SetEntityPermissions(ctx, entityID, entities.SetPermissionsInput{
		Visibility: entities.VisibilityCustom, Permissions: grants,
	})
}

// permissionsOf loads the entity and its grants, treating one from another
// campaign as missing.
func (a *armoryHandoutAdapter) permissionsOf(ctx context.Context, campaignID, entityID string) (*entities.Entity, []entities.EntityPermission, error) {
	e, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		return nil, nil, err
	}
	if e.CampaignID != campaignID {
		return nil, nil, apperror.NewNotFound("item")
	}
	existing, err := a.svc.GetEntityPermissions(ctx, entityID)
	if err != nil {
		return nil, nil, err
	}
	return e, existing, nil
}

// armoryGiveNotifierAdapter implements armory.GiveNotifier on the sessions
// plugin's notification store, which any feature may write through NotifyUsers.
type armoryGiveNotifierAdapter struct {
	svc sessions.SessionService
}

var _ armory.GiveNotifier = (*armoryGiveNotifierAdapter)(nil)

func (a *armoryGiveNotifierAdapter) ItemGiven(ctx context.Context, campaignID string, userIDs []string, message, detail, link string) error {
	return a.svc.NotifyUsersWithDetail(ctx, userIDs, campaignID, armory.NotifItemGiven, message, detail, link)
}

// armoryShareAuditAdapter writes item shares to the campaign's activity log.
// The audit service is built after the stash service, so it is bound late;
// until then (never, once the app serves) a share simply goes unlogged.
type armoryShareAuditAdapter struct {
	svc audit.AuditService
}

var _ armory.ShareAuditor = (*armoryShareAuditAdapter)(nil)

func (a *armoryShareAuditAdapter) LogEvent(ctx context.Context, campaignID, userID, action string, details map[string]any) error {
	if a.svc == nil {
		return nil
	}
	return a.svc.Log(ctx, &audit.AuditEntry{
		CampaignID: campaignID, UserID: userID, Action: action, EntityType: "entity",
		EntityID: fmt.Sprint(details["item_id"]), EntityName: fmt.Sprint(details["item_name"]), Details: details,
	})
}
