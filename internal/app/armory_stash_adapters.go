package app

// armory_stash_adapters.go wires the armory stash service to the entities,
// relations and campaigns plugins. The armory plugin sees only its own small
// interfaces; every cross-plugin call goes through these adapters.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// moneyField picks the money field of an entity type, or empty strings when
// it has none. The key order is the plugin's, shared with its history recorder.
func moneyField(fields []entities.FieldDefinition) (key, label string) {
	for _, want := range armory.MoneyFieldKeys {
		for _, f := range fields {
			if f.Key == want && f.Type == "number" {
				return want, f.Label
			}
		}
	}
	return "", ""
}

// stashTypeInfo is what the directory derives from a campaign's entity types.
type stashTypeInfo struct {
	expires   time.Time
	character map[int]bool
	item      map[int]bool
	money     map[int]string
	moneyName map[int]string
	charIDs   []int
	itemIDs   []int
}

// armoryStashDirectoryAdapter implements armory.StashDirectory. Entity types
// change rarely and a history page resolves many names, so the type facts are
// cached per campaign for a few seconds.
type armoryStashDirectoryAdapter struct {
	svc   entities.EntityService
	mu    sync.Mutex
	cache map[string]*stashTypeInfo
}

func (a *armoryStashDirectoryAdapter) types(ctx context.Context, campaignID string) (*stashTypeInfo, error) {
	a.mu.Lock()
	if ti, ok := a.cache[campaignID]; ok && time.Now().Before(ti.expires) {
		a.mu.Unlock()
		return ti, nil
	}
	a.mu.Unlock()

	all, err := a.svc.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	items, err := a.svc.GetEntityTypesByPresetCategory(ctx, campaignID, "item")
	if err != nil {
		return nil, err
	}
	ti := &stashTypeInfo{
		expires:   time.Now().Add(5 * time.Second),
		character: map[int]bool{},
		item:      map[int]bool{},
		money:     map[int]string{},
		moneyName: map[int]string{},
	}
	ti.charIDs = characterFamilyTypeIDs(all, true)
	for _, id := range ti.charIDs {
		ti.character[id] = true
	}
	for _, et := range items {
		ti.item[et.ID] = true
		ti.itemIDs = append(ti.itemIDs, et.ID)
	}
	for _, et := range all {
		ti.money[et.ID], ti.moneyName[et.ID] = moneyField(et.Fields)
	}
	a.mu.Lock()
	if a.cache == nil {
		a.cache = map[string]*stashTypeInfo{}
	}
	a.cache[campaignID] = ti
	a.mu.Unlock()
	return ti, nil
}

func (a *armoryStashDirectoryAdapter) ref(ti *stashTypeInfo, e *entities.Entity) armory.EntityRef {
	r := armory.EntityRef{
		ID: e.ID, Name: e.Name, TypeID: e.EntityTypeID,
		IsCharacter: ti.character[e.EntityTypeID],
		IsItem:      ti.item[e.EntityTypeID],
	}
	if e.OwnerUserID != nil {
		r.OwnerUserID = *e.OwnerUserID
	}
	if r.IsCharacter {
		r.MoneyKey = ti.money[e.EntityTypeID]
		r.MoneyLabel = ti.moneyName[e.EntityTypeID]
	}
	return r
}

// GetEntity returns nil for a missing entity and for one in another campaign.
func (a *armoryStashDirectoryAdapter) GetEntity(ctx context.Context, campaignID, entityID string) (*armory.EntityRef, error) {
	e, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	if e.CampaignID != campaignID {
		return nil, nil
	}
	ti, err := a.types(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	r := a.ref(ti, e)
	return &r, nil
}

// listByTypes pages through every viewable entity of the given types.
func (a *armoryStashDirectoryAdapter) listByTypes(ctx context.Context, ti *stashTypeInfo, campaignID string, typeIDs []int, role int, userID string, limit int) ([]armory.EntityRef, error) {
	seen := map[string]bool{}
	var out []armory.EntityRef
	for _, tid := range typeIDs {
		for page := 1; page <= 20; page++ {
			list, total, err := a.svc.List(ctx, campaignID, tid, role, userID, entities.ListOptions{Page: page, PerPage: 100, Sort: "name"})
			if err != nil {
				return nil, err
			}
			for i := range list {
				if seen[list[i].ID] {
					continue
				}
				seen[list[i].ID] = true
				out = append(out, a.ref(ti, &list[i]))
				if limit > 0 && len(out) >= limit {
					return out, nil
				}
			}
			if page*100 >= total || len(list) == 0 {
				break
			}
		}
	}
	return out, nil
}

func (a *armoryStashDirectoryAdapter) ListCharacters(ctx context.Context, campaignID string, role int, userID string) ([]armory.EntityRef, error) {
	ti, err := a.types(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	all, err := a.listByTypes(ctx, ti, campaignID, ti.charIDs, role, userID, 0)
	if err != nil {
		return nil, err
	}
	// Listing a parent type also returns its sub-types, so keep only entities
	// whose own type is in the family.
	out := all[:0]
	for _, r := range all {
		if r.IsCharacter {
			out = append(out, r)
		}
	}
	return out, nil
}

func (a *armoryStashDirectoryAdapter) ListItems(ctx context.Context, campaignID string, role int, userID string, limit int) ([]armory.EntityRef, error) {
	ti, err := a.types(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return a.listByTypes(ctx, ti, campaignID, ti.itemIDs, role, userID, limit)
}

func (a *armoryStashDirectoryAdapter) OwnedCharacterIDs(ctx context.Context, campaignID, userID string) (map[string]bool, error) {
	list, err := a.svc.ListByOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(list))
	for _, e := range list {
		out[e.ID] = true
	}
	return out, nil
}

// armoryCharacterActorAdapter implements armory.BuyerAccessChecker for moves:
// a user may act as a character they have claimed, or when they are Owner or
// Scribe, or when the entity's own permissions grant them edit. It differs
// from armoryBuyerAccessAdapter, which needs edit access alone and so refuses
// a player on their own claimed character whenever the character is simply
// visible to everyone.
type armoryCharacterActorAdapter struct {
	svc entities.EntityService
}

func (a *armoryCharacterActorAdapter) CanUserActAsBuyer(ctx context.Context, campaignID, entityID, userID string, role int) (bool, error) {
	e, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	if e.CampaignID != campaignID {
		return false, nil
	}
	if role >= permissions.RoleScribe {
		return true, nil
	}
	if e.OwnerUserID != nil && userID != "" && *e.OwnerUserID == userID {
		return true, nil
	}
	perm, err := a.svc.CheckEntityAccess(ctx, entityID, role, userID)
	if err != nil {
		return false, err
	}
	return perm != nil && perm.CanEdit, nil
}

// armoryEntityFieldsAdapter implements armory.EntityFieldUpdater. Updates are
// merged into the stored fields, so only the money key changes.
type armoryEntityFieldsAdapter struct {
	svc entities.EntityService
}

func (a *armoryEntityFieldsAdapter) GetEntityFields(ctx context.Context, entityID string) (map[string]any, error) {
	e, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(e.FieldsData))
	for k, v := range e.FieldsData {
		out[k] = v
	}
	return out, nil
}

// UpdateEntityFields marks the write as a stash move's, so the money history
// does not log a second line for a change the move already records.
func (a *armoryEntityFieldsAdapter) UpdateEntityFields(ctx context.Context, entityID string, fields map[string]any) error {
	ctx = changesource.With(ctx, changesource.Source{Kind: changesource.KindStash})
	return a.svc.MergeFields(ctx, entityID, fields)
}

// armoryHasItemAdapter implements armory.HasItemStore over the relations
// service, using the same "Has Item" / "In Inventory Of" pair as the inventory
// widget.
type armoryHasItemAdapter struct {
	svc relations.RelationService
}

const (
	hasItemType       = "Has Item"
	inInventoryOfType = "In Inventory Of"
)

func (a *armoryHasItemAdapter) ListByCharacter(ctx context.Context, campaignID, characterID string) ([]armory.HasItemRelation, error) {
	rels, err := a.svc.ListByEntity(ctx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	var out []armory.HasItemRelation
	for _, r := range rels {
		if r.RelationType != hasItemType {
			continue
		}
		out = append(out, armory.HasItemRelation{
			ID: r.ID, ItemEntityID: r.TargetEntityID, ItemName: r.TargetEntityName,
			Metadata: r.Metadata, DmOnly: r.DmOnly,
		})
	}
	return out, nil
}

func (a *armoryHasItemAdapter) Create(ctx context.Context, campaignID, characterID, itemID, createdBy string, metadata []byte) (int, error) {
	rel, err := a.svc.Create(ctx, campaignID, characterID, itemID, hasItemType, inInventoryOfType, createdBy, metadata)
	if err != nil {
		return 0, err
	}
	return rel.ID, nil
}

func (a *armoryHasItemAdapter) UpdateMetadata(ctx context.Context, id int, metadata []byte) error {
	return a.svc.UpdateMetadata(ctx, id, metadata)
}

func (a *armoryHasItemAdapter) UpdateMetadataIf(ctx context.Context, id int, expected, metadata []byte) (bool, error) {
	return a.svc.UpdateMetadataIf(ctx, id, expected, metadata)
}

func (a *armoryHasItemAdapter) Delete(ctx context.Context, id int) error {
	return a.svc.Delete(ctx, id)
}

// armoryMemberNamesAdapter implements armory.UserNamer from the member list.
type armoryMemberNamesAdapter struct {
	svc campaigns.CampaignService
}

func (a *armoryMemberNamesAdapter) DisplayNames(ctx context.Context, campaignID string, userIDs []string) (map[string]string, error) {
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

// armoryStashEventAdapter publishes the stash plugin's events on the
// websocket bus. The bus is bound after construction, once it exists.
type armoryStashEventAdapter struct {
	bus ws.EventBus
}

// stashEventTypes is the set of event types the stash plugin may publish.
var stashEventTypes = map[string]ws.MessageType{
	armory.EventStashMoved:        ws.MsgStashMoved,
	armory.EventStashRequested:    ws.MsgStashRequested,
	armory.EventStashSettled:      ws.MsgStashSettled,
	armory.EventStashMoneyChanged: ws.MsgStashMoneyChanged,
	armory.EventDowntimeChanged:   ws.MsgDowntimeChanged,
}

// PublishStashEvent sends the event to DM-equivalent sockets only: the ids in
// it can belong to hidden characters or stashes, and the only consumer is the
// GM's Foundry client, which relays for players.
func (a *armoryStashEventAdapter) PublishStashEvent(eventType, campaignID, resourceID string, payload map[string]any) {
	t, ok := stashEventTypes[eventType]
	if !ok || a.bus == nil || campaignID == "" {
		return
	}
	msg := ws.NewMessage(t, campaignID, resourceID, payload)
	msg.RequiresDM = true
	a.bus.Publish(msg)
}

// armoryMemberDirectoryAdapter implements armory.MemberDirectory over the
// campaigns service.
type armoryMemberDirectoryAdapter struct {
	svc campaigns.CampaignService
}

func (a *armoryMemberDirectoryAdapter) MemberRole(ctx context.Context, campaignID, userID string) (int, bool, error) {
	m, err := a.svc.GetMember(ctx, campaignID, userID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return 0, false, nil
		}
		return 0, false, err
	}
	if m == nil {
		return 0, false, nil
	}
	return int(m.Role), true, nil
}

func (a *armoryMemberDirectoryAdapter) IsDmGranted(ctx context.Context, campaignID, userID string) (bool, error) {
	return a.svc.IsUserDmGranted(ctx, campaignID, userID)
}

// armoryMoneyObserver implements entities.FieldChangeObserver: it hands sheet
// money edits to the stash plugin's history recorder.
type armoryMoneyObserver struct {
	history *armory.MoneyHistory
}

func (o *armoryMoneyObserver) FieldsChanged(ctx context.Context, e *entities.Entity, oldFields, newFields map[string]any) error {
	return o.history.RecordFieldChange(ctx, e.CampaignID, e.ID, oldFields, newFields)
}

// syncStashAPIAdapter implements syncapi.StashAPIService over the stash
// plugin's sync API facade, translating request shapes between the two.
type syncStashAPIAdapter struct {
	api *armory.StashAPI
}

var _ syncapi.StashAPIService = (*syncStashAPIAdapter)(nil)

func (a *syncStashAPIAdapter) View(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string) (any, error) {
	v, err := a.api.View(ctx, campaignID, keyUserID, actingUserID, characterID)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (a *syncStashAPIAdapter) Move(ctx context.Context, campaignID, keyUserID string, req syncapi.StashMoveRequest) (any, error) {
	v, err := a.api.Move(ctx, campaignID, keyUserID, armory.APIMoveRequest{
		ActingUserID: req.ActingUserID, Kind: req.Kind, ItemID: req.ItemID,
		Quantity: req.Quantity, Amount: req.Amount,
		From: armory.APIEndpoint{Kind: req.From.Kind, ID: req.From.ID},
		To:   armory.APIEndpoint{Kind: req.To.Kind, ID: req.To.ID},
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (a *syncStashAPIAdapter) History(ctx context.Context, campaignID, keyUserID, actingUserID, characterID, stashID string) (any, error) {
	v, err := a.api.History(ctx, campaignID, keyUserID, actingUserID, characterID, stashID)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (a *syncStashAPIAdapter) Requests(ctx context.Context, campaignID, keyUserID, actingUserID string) (any, error) {
	v, err := a.api.Requests(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (a *syncStashAPIAdapter) Approve(ctx context.Context, campaignID, keyUserID, actingUserID string, moveID int64) (any, error) {
	v, err := a.api.Approve(ctx, campaignID, keyUserID, actingUserID, moveID)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (a *syncStashAPIAdapter) Decline(ctx context.Context, campaignID, keyUserID, actingUserID string, moveID int64) (any, error) {
	v, err := a.api.Decline(ctx, campaignID, keyUserID, actingUserID, moveID)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (a *syncStashAPIAdapter) Downtime(ctx context.Context, campaignID string) (any, error) {
	v, err := a.api.Downtime(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (a *syncStashAPIAdapter) SetDowntime(ctx context.Context, campaignID, keyUserID, actingUserID string, open bool) (any, error) {
	v, err := a.api.SetDowntime(ctx, campaignID, keyUserID, actingUserID, open)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// syncShopBuyAPIAdapter implements syncapi.ShopBuyAPIService over the shop
// buy service, acting as the member the call names. Who the call runs as is
// decided by StashAPI.ActorFor, the same rule the stash calls use, so a key
// can only ever buy as itself or, when its holder is Owner or co-DM, as a
// current member under that member's own rights.
type syncShopBuyAPIAdapter struct {
	actors *armory.StashAPI
	buy    armory.ShopBuyService
}

var _ syncapi.ShopBuyAPIService = (*syncShopBuyAPIAdapter)(nil)

func (a *syncShopBuyAPIAdapter) Buyers(ctx context.Context, campaignID, keyUserID, actingUserID, shopEntityID string) (any, error) {
	actor, err := a.actors.ActorFor(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	v, err := a.buy.Buyers(ctx, campaignID, shopEntityID, actor)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (a *syncShopBuyAPIAdapter) Buy(ctx context.Context, campaignID, keyUserID, actingUserID, shopEntityID string, body json.RawMessage) (any, error) {
	actor, err := a.actors.ActorFor(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	var in armory.BuyInput
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, apperror.NewBadRequest("invalid JSON body")
	}
	v, err := a.buy.Buy(ctx, campaignID, shopEntityID, actor, in)
	if err != nil {
		return nil, err
	}
	return v, nil
}
