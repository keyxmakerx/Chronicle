package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// linkStore keeps links the way the service does: a create writes both ends
// and a delete takes both away.
type linkStore struct {
	rels   []relations.Relation
	nextID int
}

func (s *linkStore) Create(_ context.Context, c, src, dst, typ, rev, _ string, md json.RawMessage, dmOnly ...bool) (*relations.Relation, error) {
	if rev == "" {
		rev = typ
	}
	dm := len(dmOnly) > 0 && dmOnly[0]
	s.nextID += 2
	f := relations.Relation{ID: s.nextID, CampaignID: c, SourceEntityID: src, TargetEntityID: dst, RelationType: typ, ReverseRelationType: rev, DmOnly: dm, Metadata: md}
	b := relations.Relation{ID: s.nextID + 1, CampaignID: c, SourceEntityID: dst, TargetEntityID: src, RelationType: rev, ReverseRelationType: typ, DmOnly: dm}
	s.rels = append(s.rels, f, b)
	return &f, nil
}
func (s *linkStore) ListByEntity(_ context.Context, _, id string) ([]relations.Relation, error) {
	var out []relations.Relation
	for _, r := range s.rels {
		if r.SourceEntityID == id {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *linkStore) UpdateMetadata(context.Context, int, json.RawMessage) error { return nil }
func (s *linkStore) Delete(_ context.Context, id int) error {
	var fwd *relations.Relation
	for i := range s.rels {
		if s.rels[i].ID == id {
			fwd = &s.rels[i]
		}
	}
	if fwd == nil {
		return nil
	}
	src, dst, rev := fwd.SourceEntityID, fwd.TargetEntityID, fwd.ReverseRelationType
	var keep []relations.Relation
	for _, r := range s.rels {
		if r.ID == id || (r.SourceEntityID == dst && r.TargetEntityID == src && r.RelationType == rev) {
			continue
		}
		keep = append(keep, r)
	}
	s.rels = keep
	return nil
}

// seeing is the set of page ids the Actor may view.
type seeing map[string]bool

func (v seeing) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, _ int, _ string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = v[id]
	}
	return out, nil
}

func linkRec(action string, f map[string]any) Record {
	return Record{Kind: "relation", Action: action, Fields: f}
}

func TestPageLinkKind_Plan(t *testing.T) {
	pages := fakeEntities{
		"ashen-wraith": {ID: "w", Name: "Ashen Wraith", CampaignID: camp},
		"vael":         {ID: "v", Name: "Vael", CampaignID: camp},
		"secret-cult":  {ID: "s", Name: "Secret Cult", CampaignID: camp},
		"elsewhere":    {ID: "x", Name: "Elsewhere", CampaignID: "other"},
	}
	existing := func() *linkStore {
		s := &linkStore{}
		_, _ = s.Create(context.Background(), camp, "w", "v", "remnant of", "has remnant", "o", nil)
		return s
	}
	tests := []struct {
		name        string
		actor       Actor
		action      string
		fields      map[string]any
		store       *linkStore
		wantErr     string
		wantWarning string
	}{
		{"adds a link", owner, ActionCreate, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": "remnant of", "reverse": "has remnant"}, &linkStore{}, "", ""},
		{"already there warns", owner, ActionCreate, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": "Remnant Of"}, existing(), "", "Already linked"},
		{"removes by type", owner, ActionDelete, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": "remnant of"}, existing(), "", ""},
		{"removes without type when only one", owner, ActionDelete, map[string]any{"page": "Ashen Wraith", "to": "Vael"}, existing(), "", ""},
		{"delete of a missing link", owner, ActionDelete, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": "ally of"}, existing(), "not linked", ""},
		{"unknown page on create is a warning", owner, ActionCreate, map[string]any{"page": "Ashen Wraith", "to": "Nobody", "type": "ally of"}, &linkStore{}, "", "No page called"},
		{"unknown page on delete is an error", owner, ActionDelete, map[string]any{"page": "Ashen Wraith", "to": "Nobody", "type": "ally of"}, &linkStore{}, "no page called", ""},
		{"page in another campaign reads as missing", owner, ActionDelete, map[string]any{"page": "Ashen Wraith", "to": "Elsewhere"}, &linkStore{}, "no page called", ""},
		{"hidden page reads as missing", owner, ActionDelete, map[string]any{"page": "Ashen Wraith", "to": "Secret Cult"}, &linkStore{}, "no page called", ""},
		{"a player key writes nothing", player, ActionCreate, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": "ally of"}, &linkStore{}, "Owner or co-DM", ""},
		{"needs a type to add", owner, ActionCreate, map[string]any{"page": "Ashen Wraith", "to": "Vael"}, &linkStore{}, "needs `type`", ""},
		{"not to itself", owner, ActionCreate, map[string]any{"page": "Vael", "to": "vael", "type": "ally of"}, &linkStore{}, "itself", ""},
		{"words over 100", owner, ActionCreate, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": strings.Repeat("x", 101)}, &linkStore{}, "100 characters", ""},
		{"reverse over 100", owner, ActionCreate, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": "a", "reverse": strings.Repeat("x", 101)}, &linkStore{}, "100 characters", ""},
		{"shop links go to shop-stock", owner, ActionCreate, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": "Sells"}, &linkStore{}, "shop-stock", ""},
		{"carried links go to carried-item", owner, ActionCreate, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": "x", "reverse": "In Inventory Of"}, &linkStore{}, "carried-item", ""},
		{"update is refused", owner, ActionUpdate, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": "x"}, existing(), "delete it", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := PageLinkKind{Entities: pages, Rels: tt.store, Views: seeing{"w": true, "v": true, "x": true}}
			p := k.Plan(context.Background(), camp, tt.actor, linkRec(tt.action, tt.fields))
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error = %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("unexpected error %q", p.Error)
			}
			joined := strings.Join(p.Warnings, "|")
			if (tt.wantWarning == "" && joined != "") || !strings.Contains(joined, tt.wantWarning) {
				t.Errorf("warnings = %q, want %q", joined, tt.wantWarning)
			}
		})
	}
}

func TestPageLinkKind_DirectorsOnlyLinkHiddenFromOthers(t *testing.T) {
	pages := fakeEntities{"a": {ID: "a", CampaignID: camp, Name: "A"}, "b": {ID: "b", CampaignID: camp, Name: "B"}}
	s := &linkStore{}
	_, _ = s.Create(context.Background(), camp, "a", "b", "serves", "", "o", nil, true)
	k := PageLinkKind{Entities: pages, Rels: s, Views: seeing{"a": true, "b": true}}
	src, dst := pages["a"], pages["b"]
	// A non-director never sees it, so it can neither match nor be removed.
	got, _ := k.linksBetween(context.Background(), camp, player, src, dst)
	if len(got) != 0 {
		t.Errorf("a player saw a DM-only link: %v", got)
	}
	got, _ = k.linksBetween(context.Background(), camp, owner, src, dst)
	if len(got) != 1 {
		t.Errorf("the owner should see it, got %v", got)
	}
}

func TestPageLinkKind_ApplyAndUndo(t *testing.T) {
	ctx := context.Background()
	pages := fakeEntities{
		"ashen-wraith": {ID: "w", Name: "Ashen Wraith", CampaignID: camp},
		"vael":         {ID: "v", Name: "Vael", CampaignID: camp},
	}
	s := &linkStore{}
	k := PageLinkKind{Entities: pages, Rels: s, Views: seeing{"w": true, "v": true}}
	add := linkRec(ActionCreate, map[string]any{"page": "Ashen Wraith", "to": "Vael", "type": "remnant of", "reverse": "has remnant", "directors_only": true})

	snap, err := k.Before(ctx, camp, owner, add)
	if err != nil || snap == nil {
		t.Fatalf("Before = %v, %v", snap, err)
	}
	if err := k.Apply(ctx, camp, owner, add); err != nil {
		t.Fatal(err)
	}
	if len(s.rels) != 2 || !s.rels[0].DmOnly || s.rels[1].RelationType != "has remnant" {
		t.Fatalf("both ends should be written, got %+v", s.rels)
	}
	// Adding it again changes nothing and keeps nothing to undo.
	if again, _ := k.Before(ctx, camp, owner, add); again != nil {
		t.Error("a link already there should keep nothing")
	}
	if err := k.Apply(ctx, camp, owner, add); err != nil || len(s.rels) != 2 {
		t.Fatalf("re-adding wrote again: %v %d", err, len(s.rels))
	}

	after, err := k.Fingerprint(ctx, camp, owner, *snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Restore(ctx, camp, owner, *snap); err != nil {
		t.Fatal(err)
	}
	if len(s.rels) != 0 {
		t.Fatalf("undo of an add should remove both ends, got %+v", s.rels)
	}
	gone, _ := k.Fingerprint(ctx, camp, owner, *snap)
	if gone == after {
		t.Error("the fingerprint should change when the link is gone")
	}

	// A removal is undone by making the link again, words and visibility kept.
	_ = k.Apply(ctx, camp, owner, add)
	del := linkRec(ActionDelete, map[string]any{"page": "Ashen Wraith", "to": "Vael"})
	dsnap, err := k.Before(ctx, camp, owner, del)
	if err != nil || dsnap == nil {
		t.Fatalf("Before(delete) = %v, %v", dsnap, err)
	}
	if err := k.Apply(ctx, camp, owner, del); err != nil || len(s.rels) != 0 {
		t.Fatalf("delete: %v %+v", err, s.rels)
	}
	if err := k.Restore(ctx, camp, owner, *dsnap); err != nil {
		t.Fatal(err)
	}
	if len(s.rels) != 2 || s.rels[0].RelationType != "remnant of" || s.rels[1].RelationType != "has remnant" || !s.rels[0].DmOnly {
		t.Errorf("link not put back as it was: %+v", s.rels)
	}
}

func TestPageLinkKind_NoViewerRefusesEverything(t *testing.T) {
	pages := fakeEntities{"a": {ID: "a", CampaignID: camp, Name: "A"}, "b": {ID: "b", CampaignID: camp, Name: "B"}}
	k := PageLinkKind{Entities: pages, Rels: &linkStore{}}
	p := k.Plan(context.Background(), camp, owner, linkRec(ActionDelete, map[string]any{"page": "A", "to": "B"}))
	if p.Error == "" {
		t.Error("without a view check no page may be linked")
	}
}

// swapAfterPlan answers the plan's read with one store and every later read
// with another, as a link changed between the review and the apply would.
type swapAfterPlan struct {
	*linkStore
	plan  *linkStore
	reads int
}

func (s *swapAfterPlan) ListByEntity(ctx context.Context, c, id string) ([]relations.Relation, error) {
	s.reads++
	if s.reads == 1 {
		return s.plan.ListByEntity(ctx, c, id)
	}
	return s.linkStore.ListByEntity(ctx, c, id)
}

// A delete that names no type picks the only link between two pages; when
// that link is shop stock or a carried item, from either end, it is refused,
// since removing it here would skip the stock and Armory bookkeeping.
func TestPageLinkKind_DeleteNeverTakesAnOwnedLink(t *testing.T) {
	ctx := context.Background()
	pages := fakeEntities{
		"forge":  {ID: "f", Name: "Forge", CampaignID: camp},
		"sword":  {ID: "s", Name: "Sword", CampaignID: camp},
		"hero":   {ID: "h", Name: "Hero", CampaignID: camp},
		"friend": {ID: "r", Name: "Friend", CampaignID: camp},
	}
	views := seeing{"f": true, "s": true, "h": true, "r": true}
	linked := func(src, dst, typ, rev string) *linkStore {
		s := &linkStore{}
		_, _ = s.Create(ctx, camp, src, dst, typ, rev, "o", json.RawMessage(`{"price":"5 gp"}`))
		return s
	}
	tests := []struct {
		name     string
		page, to string
		store    *linkStore
		wantErr  string
	}{
		{"a shop's stock", "Forge", "Sword", linked("f", "s", "sells", "sold by"), "shop-stock"},
		{"the stock read from the item's end", "Sword", "Forge", linked("f", "s", "sells", "sold by"), "shop-stock"},
		{"a character's item", "Hero", "Sword", linked("h", "s", "Has Item", "In Inventory Of"), "carried-item"},
		{"the item read from its end", "Sword", "Hero", linked("h", "s", "Has Item", "In Inventory Of"), "carried-item"},
		{"a free-text link still goes", "Hero", "Friend", linked("h", "r", "ally of", ""), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := PageLinkKind{Entities: pages, Rels: tt.store, Views: views}
			del := linkRec(ActionDelete, map[string]any{"page": tt.page, "to": tt.to})
			p := k.Plan(ctx, camp, owner, del)
			snap, _ := k.Before(ctx, camp, owner, del)
			err := k.Apply(ctx, camp, owner, del)
			if tt.wantErr == "" {
				if p.Error != "" || err != nil || len(tt.store.rels) != 0 || snap == nil {
					t.Fatalf("plan %q, apply %v, left %d rows", p.Error, err, len(tt.store.rels))
				}
				return
			}
			if !strings.Contains(p.Error, tt.wantErr) {
				t.Errorf("plan error = %q, want %q", p.Error, tt.wantErr)
			}
			if err == nil || len(tt.store.rels) != 2 {
				t.Errorf("apply removed an owned link: err %v, %d rows left", err, len(tt.store.rels))
			}
			if snap != nil {
				t.Errorf("a refused delete kept an undo snapshot")
			}
		})
	}

	t.Run("a link that became stock after the review", func(t *testing.T) {
		now := linked("f", "s", "sells", "sold by")
		rels := &swapAfterPlan{linkStore: now, plan: linked("f", "s", "ally of", "")}
		k := PageLinkKind{Entities: pages, Rels: rels, Views: views}
		err := k.Apply(ctx, camp, owner, linkRec(ActionDelete, map[string]any{"page": "Forge", "to": "Sword"}))
		if err == nil || !strings.Contains(err.Error(), "shop-stock") || len(now.rels) != 2 {
			t.Errorf("apply err %v, %d rows left", err, len(now.rels))
		}
	})
}
