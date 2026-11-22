package records

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// maxRelationWords is the Relations box's limit on a relation's words; the
// service enforces it too, but a review row should refuse it up front.
const maxRelationWords = 100

// PageViewAPI says which pages the Actor may see. The Relations box only
// lets a person link pages they can view, so this kind asks the same
// question through the entities service rather than guessing from a name.
type PageViewAPI interface {
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

// ownedRelationTypes are written by kinds that keep more than the link
// (stock, prices, Armory counts and history). Writing them here would skip
// that bookkeeping, so they are refused with a pointer to the right kind.
var ownedRelationTypes = map[string]string{
	"sells": "shop-stock", "sold by": "shop-stock",
	"has item": "carried-item", "in inventory of": "carried-item",
}

// ownedLinkError refuses a picked link that another kind keeps, read from the
// link's own type and reverse rather than the record's words: a delete that
// names no type picks the only link between two pages, whatever it is, and
// must not take away a shop's stock or a character's item without the
// bookkeeping that goes with it.
func ownedLinkError(link *relations.Relation) error {
	if link == nil {
		return nil
	}
	for _, w := range []string{link.RelationType, link.ReverseRelationType} {
		if kind, owned := ownedRelationTypes[strings.ToLower(strings.TrimSpace(w))]; owned {
			return badRequestf("%s links are written with kind `%s`", quote(link.RelationType), kind)
		}
	}
	return nil
}

// PageLinkKind is a free-text link between two pages: the Relations box on a
// page ("ally of", "remnant of"). Each link is stored as a pair, one row per
// end, so a link has a type and a reverse type.
type PageLinkKind struct {
	Entities EntityLookup
	Rels     RelationsAPI
	// Views narrows both ends to pages the Actor can see. Nil refuses every
	// link, so a server that cannot tell never links hidden pages.
	Views PageViewAPI
}

var _ aiplug.Undoable = PageLinkKind{}

func (PageLinkKind) Name() string  { return "relation" }
func (PageLinkKind) Label() string { return "Page link" }

func (PageLinkKind) Doc() string {
	return "A link between two pages, as the Relations box on a page shows it. Keys: `page` (the page the link is written on), `to` (the page it points at), `type` (the link's words as `page` reads them, up to 100 characters, free text such as \"ally of\"), `reverse` (the words as `to` reads them; leave out and both read the same), `directors_only` (true: only the DM team sees the link). The link shows on both pages. Both pages must be pages you can see; one that does not exist yet can be made earlier in the same paste. Adding a link that is already there changes nothing. `action: delete` takes the link away from both pages (give `type` when the pages are linked more than one way; the reverse goes with it). To change a link's words, delete it and add the new one. Shop stock (`sells`) and what a character carries (`Has Item`) have their own kinds, `shop-stock` and `carried-item`. Only the campaign's Owner or co-DM writes links. Example: to mark a creature as a remnant of a fallen power, with the page it came from reading it back as \"has remnant\":\n\n```\n---\nkind: relation\npage: Ashen Wraith\nto: The Lich-King Vael\ntype: remnant of\nreverse: has remnant\n---\n```"
}

// Title names the row, since a link has no name of its own.
func (PageLinkKind) Title(r Record) string {
	if r.Str("page") == "" || r.Str("to") == "" {
		return ""
	}
	t := r.Str("type")
	if t == "" {
		t = "linked to"
	}
	return r.Str("page") + " " + t + " " + r.Str("to")
}

// pages finds both ends as the Actor may see them; a hidden page reads as
// missing so a refusal never confirms it exists.
func (k PageLinkKind) pages(ctx context.Context, campaignID string, a Actor, r Record) (src, dst *entities.Entity, err error) {
	find := func(name string) *entities.Entity {
		if strings.TrimSpace(name) == "" || k.Entities == nil {
			return nil
		}
		e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
		if err != nil || e == nil || e.CampaignID != campaignID {
			return nil
		}
		return e
	}
	src, dst = find(r.Str("page")), find(r.Str("to"))
	var ids []string
	for _, e := range []*entities.Entity{src, dst} {
		if e != nil {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 || k.Views == nil {
		return nil, nil, nil
	}
	ok, err := k.Views.FilterViewableEntityIDs(ctx, campaignID, ids, a.Role, a.UserID)
	if err != nil {
		return nil, nil, badRequestf("could not check which pages you can see")
	}
	if src != nil && !ok[src.ID] {
		src = nil
	}
	if dst != nil && !ok[dst.ID] {
		dst = nil
	}
	return src, dst, nil
}

// linksBetween lists the links written on src that point at dst. A link only
// the DM team may see is left out for anyone else, as the Relations box
// hides it.
func (k PageLinkKind) linksBetween(ctx context.Context, campaignID string, a Actor, src, dst *entities.Entity) ([]relations.Relation, error) {
	rels, err := k.Rels.ListByEntity(ctx, campaignID, src.ID)
	if err != nil {
		return nil, badRequestf("could not read %s's links", src.Name)
	}
	var out []relations.Relation
	for _, rel := range rels {
		if rel.SourceEntityID != src.ID || rel.TargetEntityID != dst.ID {
			continue
		}
		if rel.DmOnly && !a.CanAuthorDmOnly() {
			continue
		}
		out = append(out, rel)
	}
	return out, nil
}

// pickLink chooses the link a record means: the one whose type matches, or
// the only one when the record names no type. More than one with no type is
// ambiguous and says which types exist.
func pickLink(links []relations.Relation, typ string) (*relations.Relation, error) {
	if typ != "" {
		for i := range links {
			if aiplug.SameName(links[i].RelationType, typ) {
				return &links[i], nil
			}
		}
		return nil, nil
	}
	switch len(links) {
	case 0:
		return nil, nil
	case 1:
		return &links[0], nil
	}
	var kinds []string
	for _, l := range links {
		kinds = append(kinds, quote(l.RelationType))
	}
	return nil, badRequestf("these pages are linked more than one way (%s); give `type` to say which", strings.Join(kinds, ", "))
}

// check is the part of a plan that needs no lookups.
func (PageLinkKind) check(a Actor, r Record) string {
	// The Relations box lets any member who can see both pages add a link;
	// an AI's change is held to the Owner or co-DM, the keys that can write
	// at all, so a narrower key never gains a write the box would give it.
	if !a.CanAuthorDmOnly() {
		return "only the campaign's Owner or co-DM can write page links"
	}
	if r.Str("page") == "" || r.Str("to") == "" {
		return "needs `page` and `to`"
	}
	if aiplug.SameName(r.Str("page"), r.Str("to")) {
		return "a page cannot be linked to itself"
	}
	switch r.Action {
	case ActionUpdate:
		return "a link's words cannot be changed in place: delete it, then add the new one"
	case ActionCreate:
		if strings.TrimSpace(r.Str("type")) == "" {
			return "needs `type`, the link's words such as \"ally of\""
		}
	}
	for _, key := range []string{"type", "reverse"} {
		w := strings.TrimSpace(r.Str(key))
		if len(w) > maxRelationWords {
			return fmt.Sprintf("`%s` must be %d characters or fewer", key, maxRelationWords)
		}
		if kind, owned := ownedRelationTypes[strings.ToLower(w)]; owned {
			return fmt.Sprintf("%s links are written with kind `%s`", quote(w), kind)
		}
	}
	return ""
}

func (k PageLinkKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if msg := k.check(a, r); msg != "" {
		return Plan{Error: msg}
	}
	if k.Rels == nil {
		return Plan{Error: "page links can't be written on this server"}
	}
	src, dst, err := k.pages(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	p := Plan{Summary: k.summary(r)}
	if src == nil || dst == nil {
		missing := r.Str("page")
		if src != nil {
			missing = r.Str("to")
		}
		if r.Action != ActionCreate {
			return Plan{Error: "no page called " + quote(missing)}
		}
		// The page may be created earlier in this same import.
		if !r.Made[entities.Slugify(missing)] {
			p.Warnings = append(p.Warnings, "No page called "+quote(missing)+" yet; it must be created in this import")
		}
		return p
	}
	links, err := k.linksBetween(ctx, campaignID, a, src, dst)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	link, err := pickLink(links, strings.TrimSpace(r.Str("type")))
	if err != nil {
		return Plan{Error: planError(err)}
	}
	if r.Action == ActionDelete {
		if err := ownedLinkError(link); err != nil {
			return Plan{Error: planError(err)}
		}
	}
	switch r.Action {
	case ActionCreate:
		if link != nil {
			p.Warnings = append(p.Warnings, "Already linked this way; nothing changes")
		}
	case ActionDelete:
		if link == nil {
			return Plan{Error: src.Name + " is not linked to " + dst.Name + " that way"}
		}
		p.Summary = "removes " + quote(link.RelationType) + " between " + src.Name + " and " + dst.Name
	}
	return p
}

func (PageLinkKind) summary(r Record) string {
	s := strings.TrimSpace(r.Str("type"))
	if rev := strings.TrimSpace(r.Str("reverse")); rev != "" && !aiplug.SameName(rev, s) {
		s += " · reads back as " + rev
	}
	if on, _ := r.Bool("directors_only"); on {
		s += " · DM team only"
	}
	if s == "" {
		return "a link"
	}
	return s
}

func (k PageLinkKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	src, dst, err := k.pages(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if src == nil || dst == nil {
		return badRequestf("%s or %s is not a page", quote(r.Str("page")), quote(r.Str("to")))
	}
	links, err := k.linksBetween(ctx, campaignID, a, src, dst)
	if err != nil {
		return err
	}
	link, err := pickLink(links, strings.TrimSpace(r.Str("type")))
	if err != nil {
		return err
	}
	if r.Action == ActionDelete {
		if link == nil {
			return apperror.NewBadRequest("it changed while you were reviewing; check it again")
		}
		// Checked again on the link read now, which may not be the one
		// the review showed.
		if err := ownedLinkError(link); err != nil {
			return err
		}
		return k.Rels.Delete(ctx, link.ID)
	}
	if link != nil {
		return nil
	}
	dmOnly, _ := r.Bool("directors_only")
	_, err = k.Rels.Create(ctx, campaignID, src.ID, dst.ID, strings.TrimSpace(r.Str("type")), strings.TrimSpace(r.Str("reverse")), a.UserID, nil, dmOnly)
	return err
}

// Export is empty: the page export already lists each page's links.
func (PageLinkKind) Export(context.Context, string, Actor) (string, error) { return "", nil }

// linkState is what a snapshot keeps: which link, and enough to make it again.
type linkState struct {
	Src, Dst, Type, Reverse string
	DmOnly                  bool
	Metadata                json.RawMessage
}

// The snapshot's target says what an undo does: take away a link the record
// made ("made") or make one it removed ("gone").
const (
	linkMade = "made"
	linkGone = "gone"
)

// Before keeps nothing when the record changes nothing (a link already
// there). A link the record adds is undone by removing it; one it removes is
// undone by adding it back, with the metadata it carried.
func (k PageLinkKind) Before(ctx context.Context, campaignID string, a Actor, r Record) (*aiplug.Snapshot, error) {
	if k.Rels == nil || k.check(a, r) != "" {
		return nil, nil
	}
	src, dst, err := k.pages(ctx, campaignID, a, r)
	if err != nil || src == nil || dst == nil {
		return nil, nil
	}
	links, err := k.linksBetween(ctx, campaignID, a, src, dst)
	if err != nil {
		return nil, nil
	}
	typ := strings.TrimSpace(r.Str("type"))
	link, err := pickLink(links, typ)
	if err != nil || (r.Action == ActionDelete && ownedLinkError(link) != nil) {
		return nil, nil
	}
	name := "the link between " + quote(src.Name) + " and " + quote(dst.Name)
	switch {
	case r.Action == ActionCreate && link == nil:
		rev := strings.TrimSpace(r.Str("reverse"))
		if rev == "" {
			rev = typ
		}
		dm, _ := r.Bool("directors_only")
		return linkSnapshot(linkMade, name, linkState{Src: src.ID, Dst: dst.ID, Type: typ, Reverse: rev, DmOnly: dm})
	case r.Action == ActionDelete && link != nil:
		return linkSnapshot(linkGone, name, linkState{Src: src.ID, Dst: dst.ID, Type: link.RelationType, Reverse: link.ReverseRelationType, DmOnly: link.DmOnly, Metadata: link.Metadata})
	}
	return nil, nil
}

func linkSnapshot(mode, name string, st linkState) (*aiplug.Snapshot, error) {
	data, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}
	return &aiplug.Snapshot{Target: mode + ":" + st.Src + ":" + st.Dst, Name: name, Data: string(data)}, nil
}

// current finds the link a snapshot names, nil when it is gone. Undo is the
// owner's, so no DM-only filter applies.
func (k PageLinkKind) current(ctx context.Context, campaignID string, s aiplug.Snapshot) (*linkState, *relations.Relation, error) {
	var st linkState
	if err := json.Unmarshal([]byte(s.Data), &st); err != nil {
		return nil, nil, err
	}
	rels, err := k.Rels.ListByEntity(ctx, campaignID, st.Src)
	if err != nil {
		return nil, nil, err
	}
	for i := range rels {
		if rels[i].SourceEntityID == st.Src && rels[i].TargetEntityID == st.Dst && aiplug.SameName(rels[i].RelationType, st.Type) {
			return &st, &rels[i], nil
		}
	}
	return &st, nil, nil
}

// Fingerprint is whether the link is there now and how it reads, so an undo
// that finds it edited or re-added since refuses rather than overwrites.
func (k PageLinkKind) Fingerprint(ctx context.Context, campaignID string, _ Actor, s aiplug.Snapshot) (string, error) {
	_, link, err := k.current(ctx, campaignID, s)
	if err != nil {
		return "", err
	}
	if link == nil {
		return aiplug.Hash("none"), nil
	}
	return aiplug.Hash([]any{link.RelationType, link.ReverseRelationType, link.DmOnly}), nil
}

// Restore removes a link the record made, or makes a removed one again.
func (k PageLinkKind) Restore(ctx context.Context, campaignID string, a Actor, s aiplug.Snapshot) error {
	st, link, err := k.current(ctx, campaignID, s)
	if err != nil {
		return err
	}
	if strings.HasPrefix(s.Target, linkMade+":") {
		if link == nil {
			return nil
		}
		return k.Rels.Delete(ctx, link.ID)
	}
	if link != nil {
		return nil
	}
	_, err = k.Rels.Create(ctx, campaignID, st.Src, st.Dst, st.Type, st.Reverse, a.UserID, st.Metadata, st.DmOnly)
	return err
}
