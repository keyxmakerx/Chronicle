package records

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// EntityLookup finds a campaign page by name. GetBySlug is campaign-scoped.
type EntityLookup interface {
	GetBySlug(ctx context.Context, campaignID, slug string) (*entities.Entity, error)
}

// RelationsAPI is the slice of relations.RelationService shop stock and
// carried items need. Relations are only ever reached through
// ListByEntity(campaignID, …), so an id from another campaign never reaches
// UpdateMetadata or Delete.
type RelationsAPI interface {
	Create(ctx context.Context, campaignID, sourceEntityID, targetEntityID, relationType, reverseRelationType, createdBy string, metadata json.RawMessage, dmOnly ...bool) (*relations.Relation, error)
	ListByEntity(ctx context.Context, campaignID, entityID string) ([]relations.Relation, error)
	UpdateMetadata(ctx context.Context, id int, metadata json.RawMessage) error
	Delete(ctx context.Context, id int) error
}

// linkKind is a page-to-page relation with numeric metadata: a shop's stock
// ("sells") or what a character carries ("Has Item"). The relation types
// and metadata keys are the ones the armory and shop widgets read.
type linkKind struct {
	name, label, doc     string
	ownerKey             string // front-matter key naming the source page
	relType, reverseType string
	numbers              []string // metadata keys copied from front matter
	strs                 []string
	Entities             EntityLookup
	Rels                 RelationsAPI
}

// ShopStockKind is what a shop sells and for how much.
func ShopStockKind(e EntityLookup, r RelationsAPI) Kind {
	return &linkKind{
		name: "shop-stock", label: "Shop stock", ownerKey: "shop",
		relType: "sells", reverseType: "sold by",
		numbers: []string{"price", "quantity"}, strs: []string{"currency"},
		Entities: e, Rels: r,
		doc: "An item a shop sells. Keys: `shop` (the shop's page name), `item` (the item's page name), `price` (number), `currency` (cp, sp, gp…; default gp), `quantity` (leave out for unlimited, 0 = sold out). Both pages must exist or be created in the same paste. `action: delete` stops selling it.\n\n```\n---\nkind: shop-stock\nshop: The Rusty Anchor\nitem: Rope (50 ft)\nprice: 1\nquantity: 5\n---\n```",
	}
}

// CarriedItemKind is an item in a character's inventory.
func CarriedItemKind(e EntityLookup, r RelationsAPI) Kind {
	return &linkKind{
		name: "carried-item", label: "Carried item", ownerKey: "character",
		relType: "Has Item", reverseType: "In Inventory Of",
		numbers: []string{"quantity"}, strs: []string{"notes"},
		Entities: e, Rels: r,
		doc: "An item in a character's inventory. Keys: `character` (the character's page name), `item` (the item's page name), `quantity` (default 1), `notes`. `action: delete` takes it away.\n\n```\n---\nkind: carried-item\ncharacter: Ser Aldric\nitem: Potion of Healing\nquantity: 2\n---\n```",
	}
}

func (k *linkKind) Name() string  { return k.name }
func (k *linkKind) Label() string { return k.label }
func (k *linkKind) Doc() string   { return k.doc }

func (k *linkKind) page(ctx context.Context, campaignID, name string) *entities.Entity {
	if name == "" {
		return nil
	}
	e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID {
		return nil
	}
	return e
}

// resolve finds both pages and any existing link between them.
func (k *linkKind) resolve(ctx context.Context, campaignID string, r Record) (src, item *entities.Entity, link *relations.Relation, err error) {
	src = k.page(ctx, campaignID, r.Str(k.ownerKey))
	item = k.page(ctx, campaignID, r.Str("item"))
	if src == nil || item == nil {
		return src, item, nil, nil
	}
	rels, err := k.Rels.ListByEntity(ctx, campaignID, src.ID)
	if err != nil {
		return src, item, nil, badRequestf("could not read %s's links", src.Name)
	}
	for i := range rels {
		if rels[i].SourceEntityID == src.ID && rels[i].TargetEntityID == item.ID && rels[i].RelationType == k.relType {
			return src, item, &rels[i], nil
		}
	}
	return src, item, nil, nil
}

func (k *linkKind) summary(r Record) string {
	s := r.Str("item") + " · " + r.Str(k.ownerKey)
	if k.name == "shop-stock" && r.Has("price") {
		cur := r.Str("currency")
		if cur == "" {
			cur = "gp"
		}
		s += " · " + r.Str("price") + " " + cur
	}
	if r.Has("quantity") {
		s += " · " + r.Str("quantity")
	}
	return s
}

func (k *linkKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if r.Str(k.ownerKey) == "" || r.Str("item") == "" {
		return Plan{Error: fmt.Sprintf("needs `%s` and `item`", k.ownerKey)}
	}
	for _, n := range k.numbers {
		if f, ok, err := r.Float(n); err != nil {
			return Plan{Error: err.Error()}
		} else if ok && f < 0 {
			return Plan{Error: n + " cannot be negative"}
		}
	}
	src, item, link, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: err.Error()}
	}
	p := Plan{Summary: k.summary(r)}
	if src == nil || item == nil {
		missing := r.Str(k.ownerKey)
		if src != nil {
			missing = r.Str("item")
		}
		if r.Action != ActionCreate {
			return Plan{Error: "no page called " + quote(missing)}
		}
		// The page may be created earlier in this same import.
		p.Warnings = append(p.Warnings, "No page called "+quote(missing)+" yet; it must be created in this import")
		return p
	}
	switch r.Action {
	case ActionCreate:
		if link != nil {
			p.Warnings = append(p.Warnings, "Already there; this changes it")
		}
	case ActionUpdate:
		if link == nil {
			return Plan{Error: r.Str("item") + " is not linked to " + src.Name}
		}
	case ActionDelete:
		if link == nil {
			return Plan{Error: r.Str("item") + " is not linked to " + src.Name}
		}
		p.Summary = "removes " + item.Name + " from " + src.Name
	}
	return p
}

// meta merges the record's keys into existing metadata, keeping keys the
// AI did not write (equipped, attuned, in_stock…).
func (k *linkKind) meta(existing json.RawMessage, r Record) (json.RawMessage, error) {
	m := map[string]any{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &m)
	}
	for _, n := range k.numbers {
		if f, ok, _ := r.Float(n); ok {
			m[n] = f
		}
	}
	for _, s := range k.strs {
		if v := r.Str(s); v != "" {
			m[s] = v
		}
	}
	if k.name == "shop-stock" {
		if _, ok := m["currency"]; !ok {
			m["currency"] = "gp"
		}
	}
	return json.Marshal(m)
}

func (k *linkKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	src, item, link, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return err
	}
	if src == nil || item == nil {
		return badRequestf("%s or %s is not a page", quote(r.Str(k.ownerKey)), quote(r.Str("item")))
	}
	if r.Action == ActionDelete {
		return k.Rels.Delete(ctx, link.ID)
	}
	var old json.RawMessage
	if link != nil {
		old = link.Metadata
	}
	md, err := k.meta(old, r)
	if err != nil {
		return err
	}
	if link != nil {
		return k.Rels.UpdateMetadata(ctx, link.ID, md)
	}
	_, err = k.Rels.Create(ctx, campaignID, src.ID, item.ID, k.relType, k.reverseType, a.UserID, md)
	return err
}

// Export is empty: the page export already lists each page's relations,
// stock and inventory included.
func (k *linkKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
