package app

// armory_stash_adapters.go wires the armory stash service to the entities,
// relations and campaigns plugins. The armory plugin sees only its own small
// interfaces; every cross-plugin call goes through these adapters.

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// moneyFieldKeys is the order in which a character's money field is looked
// for: the first numeric field whose key is one of these.
var moneyFieldKeys = []string{"gp", "coins", "gold", "money", "wealth"}

// moneyFieldKey picks the money field of an entity type, or "" when it has none.
func moneyFieldKey(fields []entities.FieldDefinition) string {
	for _, want := range moneyFieldKeys {
		for _, f := range fields {
			if f.Key == want && f.Type == "number" {
				return want
			}
		}
	}
	return ""
}

// stashTypeInfo is what the directory derives from a campaign's entity types.
type stashTypeInfo struct {
	expires   time.Time
	character map[int]bool
	item      map[int]bool
	money     map[int]string
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
		ti.money[et.ID] = moneyFieldKey(et.Fields)
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

func (a *armoryEntityFieldsAdapter) UpdateEntityFields(ctx context.Context, entityID string, fields map[string]any) error {
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
