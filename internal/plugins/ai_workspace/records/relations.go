package records

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

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

// CarriedAPI keeps a character's count of an Armory item through the
// Armory itself, so a change the owner approves is a give or a take in the
// item history, with the player's bell and remembered copies handled the way
// every Armory move handles them. Implemented in internal/app; nil leaves
// every count a plain edit of the relation, as the inventory widget makes.
type CarriedAPI interface {
	// Counts reports whether the Armory keeps this count for a: the page is
	// a character, the item an Armory item, and a may give (Owner or co-DM).
	Counts(ctx context.Context, campaignID string, a Actor, characterID, itemID string) bool
	// SetCarried makes the character hold qty of the item; 0 takes the line
	// away.
	SetCarried(ctx context.Context, campaignID string, a Actor, characterID, itemID string, qty int) error
}

// linkKind is a page-to-page relation with numeric metadata: a shop's stock
// ("sells") or what a character carries ("Has Item"). The relation types
// and metadata keys are the ones the armory and shop widgets read.
type linkKind struct {
	name, label, doc     string
	ownerKey             string // front-matter key naming the source page
	joiner, qtyWord      string // review wording: "Rope at the shop", "5 in stock"
	relType, reverseType string
	numbers              []string // metadata keys copied from front matter
	strs                 []string
	// flags are true-or-false metadata keys; false takes the key away, as
	// the widgets store an unset flag.
	flags []string
	Entities             EntityLookup
	Rels                 RelationsAPI
	// Armory, on carried items only, writes counts through the Armory.
	Armory CarriedAPI
}

// ShopStockKind is what a shop sells and for how much.
func ShopStockKind(e EntityLookup, r RelationsAPI) Kind {
	return &linkKind{
		name: "shop-stock", label: "Shop stock", ownerKey: "shop", joiner: "at", qtyWord: "in stock",
		relType: "sells", reverseType: "sold by",
		numbers: []string{"price", "quantity"}, strs: []string{"currency"}, flags: []string{"today"},
		Entities: e, Rels: r,
		doc: "An item a shop sells. Keys: `shop` (the shop's page name), `item` (the item's page name), `price` (number), `currency` (cp, sp, gp…; default gp), `quantity` (leave out for unlimited, 0 = sold out), `today` (true: it turns up on today's shelf on only some days, as `what: stock` shows; false: sold every day). Both pages must exist or be created in the same paste. `action: delete` stops selling it.\n\n```\n---\nkind: shop-stock\nshop: The Rusty Anchor\nitem: Rope (50 ft)\nprice: 1\nquantity: 5\n---\n```",
	}
}

// CarriedItemKind is an item in a character's inventory.
func CarriedItemKind(e EntityLookup, r RelationsAPI, armory CarriedAPI) Kind {
	return &linkKind{
		name: "carried-item", label: "Carried item", ownerKey: "character", joiner: "for", qtyWord: "carried",
		relType: "Has Item", reverseType: "In Inventory Of",
		numbers: []string{"quantity"}, strs: []string{"notes"},
		Entities: e, Rels: r, Armory: armory,
		doc: "An item in a character's inventory. Keys: `character` (the character's page name), `item` (the item's page name), `quantity` (how many they hold in all; default 1), `notes`. `action: delete` takes it away. For an Armory item the change is a give or a take in the character's item history, and lowering a count takes plain copies before remembered ones.\n\n```\n---\nkind: carried-item\ncharacter: Ser Aldric\nitem: Potion of Healing\nquantity: 2\n---\n```",
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

// Title names the row on the review screen, since these records have no
// name of their own: "Rope at The Rusty Anchor", "Potion for Ser Aldric".
func (k *linkKind) Title(r Record) string {
	if r.Str("item") == "" || r.Str(k.ownerKey) == "" {
		return ""
	}
	return r.Str("item") + " " + k.joiner + " " + r.Str(k.ownerKey)
}

func (k *linkKind) summary(r Record) string {
	var parts []string
	if k.name == "shop-stock" && r.Has("price") {
		cur := r.Str("currency")
		if cur == "" {
			cur = "gp"
		}
		parts = append(parts, r.Str("price")+" "+cur)
	}
	if r.Has("quantity") {
		parts = append(parts, r.Str("quantity")+" "+k.qtyWord)
	}
	if on, ok := r.Bool("today"); ok && k.name == "shop-stock" {
		if on {
			parts = append(parts, "on today's shelf some days")
		} else {
			parts = append(parts, "sold every day")
		}
	}
	if len(parts) == 0 {
		if k.name == "carried-item" {
			return "1 carried"
		}
		return "no price or quantity given"
	}
	return strings.Join(parts, " · ")
}

func (k *linkKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if r.Str(k.ownerKey) == "" || r.Str("item") == "" {
		return Plan{Error: fmt.Sprintf("needs `%s` and `item`", k.ownerKey)}
	}
	for _, n := range k.numbers {
		if f, ok, err := r.Float(n); err != nil {
			return Plan{Error: planError(err)}
		} else if ok && f < 0 {
			return Plan{Error: n + " cannot be negative"}
		}
	}
	src, item, link, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
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
	armory := k.armoryCounts(ctx, campaignID, a, src, item)
	if armory && r.Has("quantity") {
		if f, _, _ := r.Float("quantity"); f != math.Trunc(f) || f > maxCarried {
			return Plan{Error: "quantity is a whole number up to 1,000,000"}
		}
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
	if armory {
		p.Summary = k.armorySummary(r, link, p.Summary)
	}
	return p
}

// maxCarried is the Armory's largest count.
const maxCarried = 1000000

// armoryCounts reports whether this carried item's count goes through the
// Armory.
func (k *linkKind) armoryCounts(ctx context.Context, campaignID string, a Actor, src, item *entities.Entity) bool {
	return k.Armory != nil && src != nil && item != nil && k.Armory.Counts(ctx, campaignID, a, src.ID, item.ID)
}

// carriedTarget is the count a carried-item record asks for, and false when
// it leaves the count alone (a change of notes on a line already there).
func carriedTarget(r Record, link *relations.Relation) (int, bool) {
	if r.Action == ActionDelete {
		return 0, true
	}
	if f, ok, _ := r.Float("quantity"); ok {
		return int(f), true
	}
	if link == nil {
		return 1, true
	}
	return 0, false
}

// heldOn reads the count stored on a carried line; a line without one holds 1.
func heldOn(link *relations.Relation) int {
	if link == nil {
		return 0
	}
	var m struct {
		Quantity *float64 `json:"quantity"`
	}
	if json.Unmarshal(link.Metadata, &m) != nil || m.Quantity == nil {
		return 1
	}
	return int(*m.Quantity)
}

// armorySummary says what the Armory will write in the item history.
func (k *linkKind) armorySummary(r Record, link *relations.Relation, plain string) string {
	want, ok := carriedTarget(r, link)
	held := heldOn(link)
	switch {
	case !ok || want == held:
		return plain
	case want > held:
		return fmt.Sprintf("gives %d (holds %d after) · in the item history", want-held, want)
	case want == 0:
		return fmt.Sprintf("takes all %d away · in the item history", held)
	default:
		return fmt.Sprintf("takes %d away (holds %d after) · in the item history", held-want, want)
	}
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
	for _, f := range k.flags {
		if on, ok := r.Bool(f); ok && on {
			m[f] = true
		} else if ok {
			delete(m, f)
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
	if r.Action != ActionCreate && link == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	if k.armoryCounts(ctx, campaignID, a, src, item) {
		return k.applyThroughArmory(ctx, campaignID, a, r, src, item, link)
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

// applyThroughArmory sets the count with the Armory, then writes the notes
// onto the line it leaves, keeping every other field the Armory and the
// inventory widget keep there (equipped, attuned…).
func (k *linkKind) applyThroughArmory(ctx context.Context, campaignID string, a Actor, r Record, src, item *entities.Entity, link *relations.Relation) error {
	if want, ok := carriedTarget(r, link); ok {
		if err := k.Armory.SetCarried(ctx, campaignID, a, src.ID, item.ID, want); err != nil {
			return err
		}
	}
	notes := r.Str("notes")
	if notes == "" || r.Action == ActionDelete {
		return nil
	}
	_, _, link, err := k.resolve(ctx, campaignID, r)
	if err != nil || link == nil {
		return err
	}
	m := map[string]any{}
	if len(link.Metadata) > 0 {
		_ = json.Unmarshal(link.Metadata, &m)
	}
	m["notes"] = notes
	md, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return k.Rels.UpdateMetadata(ctx, link.ID, md)
}

// Export is empty: the page export already lists each page's relations,
// stock and inventory included.
func (k *linkKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
