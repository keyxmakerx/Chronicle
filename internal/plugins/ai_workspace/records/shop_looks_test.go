package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeShopLooks keeps one saved room and storefront per shop; refuse makes
// every check fail with its words, as the armory's normalizers would.
type fakeShopLooks struct {
	rooms, fronts map[string]json.RawMessage
	refuse        string
}

func (f *fakeShopLooks) Room(_ context.Context, _, id string, _ Actor) (json.RawMessage, error) {
	return f.rooms[id], nil
}
func (f *fakeShopLooks) SaveRoom(_ context.Context, _, id string, _ Actor, layout json.RawMessage, check bool) error {
	if f.refuse != "" {
		return badRequestf("%s", f.refuse)
	}
	if !check {
		f.rooms[id] = layout
	}
	return nil
}
func (f *fakeShopLooks) Front(_ context.Context, _, id string, _ Actor) (json.RawMessage, bool, error) {
	if fr, ok := f.fronts[id]; ok {
		return fr, true, nil
	}
	return json.RawMessage(`{"style":"timber","colour":"#d7b98f","awning":"stripe","sign":"hang","icon":"bread","wares":["bread","scroll"],"lamp":true,"closed":"shutters"}`), false, nil
}
func (f *fakeShopLooks) SaveFront(_ context.Context, _, id string, _ Actor, front json.RawMessage, check bool) error {
	if f.refuse != "" {
		return badRequestf("%s", f.refuse)
	}
	if !check {
		f.fronts[id] = front
	}
	return nil
}

const arrangedRoom = `{"version":1,"roomType":"forge","setting":"room","size":"m","furniture":"normal","decorations":"some","palette":"ember","look":"lit","mood":"cozy","effects":{"shadows":80},"seeds":{"room":3,"goods":1,"deco":1},"pieces":[{"id":1,"kind":"anvil","pinned":true}],"decor":[],"items":{},"portrait":null,"lines":["Mind the sparks."],"manner":"gruff","moments":{"bye":["Off with you."]}}`

func shopPages() fakeEntities {
	pages := fakeEntities{}
	for id, name := range map[string]string{"anchor": "The Rusty Anchor", "forge": "Ember Forge"} {
		e := page(id, name, "Shop", false)
		e.TypeSlug = "shop"
		pages[e.Slug] = &e
	}
	inn := page("inn", "The Inn", "Place", false)
	pages[inn.Slug] = &inn
	return pages
}

func shopKinds() (ShopRoomKind, StorefrontKind, *fakeShopLooks) {
	f := &fakeShopLooks{rooms: map[string]json.RawMessage{"forge": json.RawMessage(arrangedRoom)}, fronts: map[string]json.RawMessage{}}
	pages, owners := shopPages(), fakeOwners{"owner": true}
	return NewShopRoomKind(f, pages, owners), NewStorefrontKind(f, pages, owners), f
}

func TestShopRoomKind(t *testing.T) {
	coDM := Actor{UserID: "codm", Role: 3}
	tests := []struct {
		name    string
		r       Record
		a       Actor
		summary string
		warn    bool
		err     string
	}{
		{"a room nobody arranged is set up", craftRec("shop-room", ActionCreate, "", map[string]any{"shop": "The Rusty Anchor", "type": "smithy", "mood": "haunted", "manner": "gruff",
			"says": map[string]any{"hello": []any{"Wipe your boots."}, "poor": "Coin first."}}), owner,
			"sets up The Rusty Anchor's room: type forge; mood haunted; manner gruff; the keeper says: hello 1 line, poor 1 line", true, ""},
		{"the keeper's lines only leave the furniture alone", craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Ember Forge", "says": map[string]any{"bye": "none", "hello": []any{"Hot iron!", "Mind your hands."}}, "chatter": "none"}), owner,
			"Ember Forge's room: the keeper says: bye back to the manner, hello 2 lines; 0 lines of chatter", false, ""},
		{"a new mood", craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Ember Forge", "mood": "dwarven", "look": "pop-up book"}), owner,
			"Ember Forge's room: look paper; mood dwarven", true, ""},
		{"what it already is changes nothing", craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Ember Forge", "type": "forge"}), owner,
			"changes nothing in Ember Forge's room", false, ""},
		{"an unknown type", craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Ember Forge", "type": "tavern"}), owner, "", false, `type: "tavern" is not one of apothecary, forge, general, jeweler, library, market`},
		{"an unknown moment", craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Ember Forge", "says": map[string]any{"haggle": "No."}}), owner, "", false, `"haggle" is not a moment`},
		{"not a shop page", craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "The Inn", "mood": "cozy"}), owner, "", false, "is not a shop page"},
		{"no such page", craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Nowhere", "mood": "cozy"}), owner, "", false, "no page called"},
		{"removing is refused", craftRec("shop-room", ActionDelete, "", map[string]any{"shop": "Ember Forge"}), owner, "", false, "can't be removed by a block"},
		{"a co-DM can't", craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Ember Forge", "mood": "cozy"}), coDM, "", false, "only the campaign's owner"},
		{"a player can't", craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Ember Forge", "mood": "cozy"}), player, "", false, "only the campaign's owner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, _, _ := shopKinds()
			p := k.Plan(context.Background(), camp, tt.a, tt.r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || p.Summary != tt.summary || (len(p.Warnings) > 0) != tt.warn {
				t.Fatalf("plan = %+v, want %q", p, tt.summary)
			}
		})
	}

	// Applying keeps everything the block doesn't name and marks what the
	// widget lays out again.
	k, _, f := shopKinds()
	ctx := context.Background()
	if err := k.Apply(ctx, camp, owner, craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Ember Forge", "mood": "fey", "says": map[string]any{"hello": "Welcome."}})); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(f.rooms["forge"], &got)
	if got["furnish"] != "mood" || got["effects"] != nil || got["palette"] != "ember" || len(got["pieces"].([]any)) != 1 || got["lines"].([]any)[0] != "Mind the sparks." {
		t.Fatalf("saved %v", got)
	}
	if m := got["moments"].(map[string]any); m["bye"] == nil || m["hello"].([]any)[0] != "Welcome." {
		t.Fatalf("moments %v", m)
	}
	if err := k.Apply(ctx, camp, owner, craftRec("shop-room", ActionCreate, "", map[string]any{"shop": "The Rusty Anchor", "palette": "moss"})); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(f.rooms["anchor"], &got)
	if got["furnish"] != "room" || got["roomType"] != "general" || got["palette"] != "moss" {
		t.Fatalf("a first room %v", got)
	}

	// The normalizer's refusal names the room.
	f.refuse = "too many lines for a moment"
	if p := k.Plan(ctx, camp, owner, craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Ember Forge", "says": map[string]any{"hello": "Hi."}})); p.Error != "the room: too many lines for a moment" {
		t.Fatalf("refusal %q", p.Error)
	}
}

func TestStorefrontKind(t *testing.T) {
	tests := []struct {
		name    string
		fields  map[string]any
		summary string
		err     string
	}{
		{"a front nobody dressed", map[string]any{"style": "stone", "colour": "#6B4F3A", "sign_picture": "hammer", "wares": []any{"hammer", "shield"}, "closed": "board", "closed_note": "Back at dusk", "lamp": false},
			"dresses The Rusty Anchor's storefront: style stone; sign picture hammer; closed board; colour #6b4f3a; closed note Back at dusk; wares hammer and shield; lamp false (the rest stays the street's default)", ""},
		{"color is read too, and none clears a trim", map[string]any{"color": "#000000", "trim": "none", "sign_picture": "none"},
			"dresses The Rusty Anchor's storefront: sign picture none; colour #000000; trim none (the rest stays the street's default)", ""},
		{"a colour by name is refused", map[string]any{"colour": "teal"}, "", "colour: write a colour as #rrggbb"},
		{"the walls need a colour", map[string]any{"colour": "none"}, "", "colour: write a colour as #rrggbb"},
		{"too many wares", map[string]any{"wares": []any{"gem", "gem", "gem", "gem"}}, "", "at most 3"},
		{"an unknown ware", map[string]any{"wares": []any{"cheese"}}, "", `wares: "cheese" is not one of`},
		{"an unknown style", map[string]any{"style": "glass"}, "", `style: "glass" is not one of painted, plaster, stone, timber`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, k, _ := shopKinds()
			tt.fields["shop"] = "The Rusty Anchor"
			p := k.Plan(context.Background(), camp, owner, craftRec("storefront", ActionUpdate, "", tt.fields))
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || p.Summary != tt.summary {
				t.Fatalf("plan = %+v\nwant %q", p, tt.summary)
			}
		})
	}

	_, k, f := shopKinds()
	if err := k.Apply(context.Background(), camp, owner, craftRec("storefront", ActionUpdate, "", map[string]any{"shop": "The Rusty Anchor", "sign_words": "Rope & Rum"})); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(f.fronts["anchor"], &got)
	if got["words"] != "Rope & Rum" || got["style"] != "timber" || got["lamp"] != true {
		t.Fatalf("saved %v", got)
	}
}

// The stock lookup shows the room and storefront as blocks that plan back
// to "changes nothing".
func TestStockLookupShowsTheShopsLooks(t *testing.T) {
	room, front, f := shopKinds()
	f.fronts["forge"] = json.RawMessage(`{"style":"stone","colour":"#333333","awning":"none","sign":"board","words":"Ember & Son","wares":["sword"],"lamp":false,"closed":"dark"}`)
	pages := []entities.Entity{}
	for _, e := range shopPages() {
		pages = append(pages, *e)
	}
	l := &Lookups{Pages: &fakePages{all: pages}, Rels: &fakeRels{}, Shops: f}
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("stock", map[string]any{"shop": "Ember Forge"}), lookup("stock", map[string]any{"shop": "The Rusty Anchor"})})
	if ans[0].Error != "" || !strings.Contains(ans[0].Text, "says:\n  bye: [\"Off with you.\"]\nchatter: [\"Mind the sparks.\"]") || !strings.Contains(ans[0].Text, "sign_words: \"Ember & Son\"") {
		t.Fatalf("stock with looks:\n%s", ans[0].Text)
	}
	if !strings.Contains(ans[1].Text, "Nobody has arranged it yet") || !strings.Contains(ans[1].Text, "Not dressed yet") {
		t.Fatalf("an undressed shop:\n%s", ans[1].Text)
	}
	for _, part := range strings.Split(ans[0].Text, "```\n")[1:] {
		if !strings.HasPrefix(part, "---\n") {
			continue
		}
		r := blockRecord(t, part[:strings.LastIndex(part, "---")+3], ActionUpdate)
		var p Plan
		if r.Kind == "shop-room" {
			p = room.Plan(context.Background(), camp, owner, r)
		} else {
			p = front.Plan(context.Background(), camp, owner, r)
		}
		if p.Error != "" || !strings.HasPrefix(p.Summary, "changes nothing") {
			t.Errorf("%s read back as %+v", r.Kind, p)
		}
	}
}
