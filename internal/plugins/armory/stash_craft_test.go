package armory

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/changesource"
)

// newCraftFx is the pay fixture with materials in packs: Mira (c2, u2, 10 gp)
// holds 4 Iron and 2 Potions, Ash (c5, u1, a coin purse) and Vex (c6, a
// Wealth sheet) hold Iron, and Grub (c3, no money field) holds Iron.
func newCraftFx() (*giveFx, *recordingFields) {
	g, rf := newPayFx()
	g.dir.ents["i3"] = &EntityRef{ID: "i3", Name: "Iron", IsItem: true}
	g.fields.data["c6"] = map[string]any{"wealth": 2.0}
	g.rels.rels["c2"] = []*HasItemRelation{
		{ID: 201, ItemEntityID: "i3", ItemName: "Iron", Metadata: []byte(`{"quantity":4}`)},
		{ID: 202, ItemEntityID: "i1", ItemName: "Potion", Metadata: []byte(`{"quantity":2,"notes":"from the temple"}`)},
	}
	for i, cid := range []string{"c5", "c6", "c3"} {
		g.rels.rels[cid] = []*HasItemRelation{{ID: 210 + i, ItemEntityID: "i3", ItemName: "Iron", Metadata: []byte(`{"quantity":2}`)}}
	}
	g.rels.next = 300
	return g, rf
}

var mirasPlayer = Actor{"u2", rPlayer}

func TestCraftSpend_TakesMaterialsAndCoins(t *testing.T) {
	g, rf := newCraftFx()
	out, err := g.svc.CraftSpend(context.Background(), "camp", mirasPlayer, CraftSpendInput{
		CharacterID: "c2", Cost: 250, Currency: "gp", Label: "Healing Draught",
		Items: []CraftItem{{ItemID: "i3", Quantity: 2}, {ItemID: "i1", Quantity: 2}, {ItemID: "i3", Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Paid != "2.50 gp" {
		t.Fatalf("paid %q", out.Paid)
	}
	if got := g.fields.data["c2"]["gp"]; got != 7.5 {
		t.Fatalf("gp = %v, want 7.5", got)
	}
	if g.carried("c2", "i3") != 1 {
		t.Fatalf("iron left %d, want 1", g.carried("c2", "i3"))
	}
	// A line spent to zero is removed once the spend is certain.
	for _, r := range g.rels.rels["c2"] {
		if r.ItemEntityID == "i1" {
			t.Fatalf("the spent Potion line is still there: %s", r.Metadata)
		}
	}
	if len(rf.sources) != 1 || rf.sources[0] != (changesource.Source{Kind: changesource.KindCraft, Label: "to make Healing Draught", UserID: "u2"}) {
		t.Fatalf("coin write source %+v", rf.sources)
	}
	want := []string{"Used 3 × Iron to make Healing Draught", "Used 2 × Potion to make Healing Draught"}
	if len(g.repo.moves) != len(want) {
		t.Fatalf("moves %d, want %d", len(g.repo.moves), len(want))
	}
	for i, m := range g.repo.moves {
		if !m.IsGive() || m.From != charEP("c2") || m.Status != MoveApplied || m.RequestedBy != "u2" || m.Reason != want[i] {
			t.Fatalf("move %d: %+v", i, m)
		}
	}
	if got := g.events.types(); len(got) != 2 || got[0] != EventStashMoved || got[1] != EventStashMoved {
		t.Fatalf("events %v", got)
	}
}

func TestCraftSpend_PaymentKinds(t *testing.T) {
	tests := []struct {
		name      string
		char      string
		cost      Cents
		currency  string
		wantPaid  string
		wantCode  int
		wantMsg   string
		wantMoney func(map[string]any) bool
	}{
		{"a purse pays in coins", "c5", 150, "gp", "1.50 gp", 0, "",
			func(d map[string]any) bool { return purseValue(d) == 151 }},
		{"a purse pays silver prices", "c5", 500, "sp", "5 sp", 0, "",
			func(d map[string]any) bool { return purseValue(d) == 251 }},
		{"a purse short of coin", "c5", 400, "gp", "", http.StatusBadRequest, notEnoughCoinMessage,
			func(d map[string]any) bool { return purseValue(d) == 301 }},
		{"Wealth is needed, not spent", "c6", 200, "gp", "", 0, "",
			func(d map[string]any) bool { return d["wealth"] == 2.0 }},
		{"Wealth too low", "c6", 300, "gp", "", http.StatusBadRequest, "Needs Wealth 3",
			func(d map[string]any) bool { return d["wealth"] == 2.0 }},
		{"no money field and a cost", "c3", 100, "gp", "", http.StatusBadRequest, "can't be paid",
			func(d map[string]any) bool { return len(d) == 0 }},
		{"no money field and no cost", "c3", 0, "", "", 0, "",
			func(d map[string]any) bool { return len(d) == 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newCraftFx()
			out, err := g.svc.CraftSpend(context.Background(), "camp", gm, CraftSpendInput{
				CharacterID: tt.char, Cost: tt.cost, Currency: tt.currency, Label: "Sword",
				Items: []CraftItem{{ItemID: "i3", Quantity: 1}},
			})
			if got := code(err); got != tt.wantCode {
				t.Fatalf("code %d (%v), want %d", got, err, tt.wantCode)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error %q, want it to say %q", err, tt.wantMsg)
			}
			wantIron := 1
			if tt.wantCode != 0 {
				wantIron = 2
			} else if out.Paid != tt.wantPaid {
				t.Fatalf("paid %q, want %q", out.Paid, tt.wantPaid)
			}
			if got := g.carried(tt.char, "i3"); got != wantIron {
				t.Fatalf("iron %d, want %d", got, wantIron)
			}
			if !tt.wantMoney(g.fields.data[tt.char]) {
				t.Fatalf("money %v", g.fields.data[tt.char])
			}
		})
	}
}

// purseValue is a fake sheet's purse in copper.
func purseValue(d map[string]any) int64 {
	var cp int64
	for coin, v := range d {
		f, _ := v.(float64)
		cp += int64(f) * coinRateCp[coin]
	}
	return cp
}

func TestCraftSpend_Refusals(t *testing.T) {
	many := make([]CraftItem, maxCraftItems+1)
	for i := range many {
		many[i] = CraftItem{ItemID: "x" + string(rune('a'+i)), Quantity: 1}
	}
	tests := []struct {
		name string
		camp string
		who  Actor
		in   CraftSpendInput
		want int
	}{
		{"another player's hero", "camp", Actor{"u1", rPlayer}, CraftSpendInput{CharacterID: "c2", Items: []CraftItem{{ItemID: "i3", Quantity: 1}}}, http.StatusForbidden},
		{"holds too little", "camp", mirasPlayer, CraftSpendInput{CharacterID: "c2", Items: []CraftItem{{ItemID: "i3", Quantity: 5}}}, http.StatusBadRequest},
		{"merged lines hold too little", "camp", mirasPlayer, CraftSpendInput{CharacterID: "c2", Items: []CraftItem{{ItemID: "i3", Quantity: 3}, {ItemID: "i3", Quantity: 2}}}, http.StatusBadRequest},
		{"not carried at all", "camp", mirasPlayer, CraftSpendInput{CharacterID: "c2", Items: []CraftItem{{ItemID: "i2", Quantity: 1}}}, http.StatusBadRequest},
		{"not an item", "camp", mirasPlayer, CraftSpendInput{CharacterID: "c2", Items: []CraftItem{{ItemID: "n1", Quantity: 1}}}, http.StatusNotFound},
		{"not a character", "camp", gm, CraftSpendInput{CharacterID: "i1", Items: []CraftItem{{ItemID: "i3", Quantity: 1}}}, http.StatusNotFound},
		{"other campaign", "other", gm, CraftSpendInput{CharacterID: "c2", Items: []CraftItem{{ItemID: "i3", Quantity: 1}}}, http.StatusNotFound},
		{"zero quantity", "camp", mirasPlayer, CraftSpendInput{CharacterID: "c2", Items: []CraftItem{{ItemID: "i3", Quantity: 0}}}, http.StatusBadRequest},
		{"too many kinds", "camp", mirasPlayer, CraftSpendInput{CharacterID: "c2", Items: many}, http.StatusBadRequest},
		{"negative cost", "camp", mirasPlayer, CraftSpendInput{CharacterID: "c2", Cost: -1, Items: []CraftItem{{ItemID: "i3", Quantity: 1}}}, http.StatusBadRequest},
		{"not enough coin", "camp", mirasPlayer, CraftSpendInput{CharacterID: "c2", Cost: 1001, Currency: "gp", Items: []CraftItem{{ItemID: "i3", Quantity: 1}}}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newCraftFx()
			_, err := g.svc.CraftSpend(context.Background(), tt.camp, tt.who, tt.in)
			if got := code(err); got != tt.want {
				t.Fatalf("code %d (%v), want %d", got, err, tt.want)
			}
			if g.carried("c2", "i3") != 4 || g.carried("c2", "i1") != 2 || g.fields.data["c2"]["gp"] != 10.0 || len(g.repo.moves) != 0 {
				t.Fatalf("a refused spend changed something: iron %d, potion %d, gp %v, moves %d",
					g.carried("c2", "i3"), g.carried("c2", "i1"), g.fields.data["c2"]["gp"], len(g.repo.moves))
			}
		})
	}
}

// failingFields refuses every sheet write, standing in for a coin write that
// fails after the materials were taken.
type failingFields struct{ *fakeFields }

func (failingFields) UpdateEntityFields(context.Context, string, map[string]any) error {
	return errors.New("db down")
}

func TestCraftSpend_FailedPaymentGivesMaterialsBack(t *testing.T) {
	g, _ := newCraftFx()
	svc := NewStashService(StashDeps{
		Repo: g.repo, Directory: g.dir, Visibility: g.vis, Actor: fakeActor{g.fx.dir},
		Fields: failingFields{g.fields}, Relations: g.rels, Events: g.events,
	})
	_, err := svc.CraftSpend(context.Background(), "camp", mirasPlayer, CraftSpendInput{
		CharacterID: "c2", Cost: 100, Currency: "gp", Label: "Sword",
		Items: []CraftItem{{ItemID: "i3", Quantity: 1}, {ItemID: "i1", Quantity: 2}},
	})
	if err == nil {
		t.Fatal("want the failed coin write to fail the spend")
	}
	if g.carried("c2", "i3") != 4 || g.carried("c2", "i1") != 2 {
		t.Fatalf("not given back: iron %d, potion %d", g.carried("c2", "i3"), g.carried("c2", "i1"))
	}
	// The Potion line was taken to zero and kept, so it comes back with its notes.
	for _, r := range g.rels.rels["c2"] {
		if r.ItemEntityID == "i1" && !strings.Contains(string(r.Metadata), "from the temple") {
			t.Fatalf("the Potion line lost its notes: %s", r.Metadata)
		}
	}
	if len(g.repo.moves) != 0 {
		t.Fatalf("a failed spend wrote %d history lines", len(g.repo.moves))
	}
}

func TestCraftHistory_ReadsTheReason(t *testing.T) {
	g, _ := newCraftFx()
	ctx := context.Background()
	if _, err := g.svc.CraftSpend(ctx, "camp", mirasPlayer, CraftSpendInput{CharacterID: "c2", Label: "Sword", Items: []CraftItem{{ItemID: "i3", Quantity: 2}}}); err != nil {
		t.Fatal(err)
	}
	if err := g.svc.CraftGive(ctx, "camp", mirasPlayer, "c2", "i1", 1, "Healing Draught", Memory{}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.svc.Give(ctx, "camp", gm, GiveInput{CharacterID: "c2", ItemID: "i1", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	lines, err := g.svc.CharacterHistory(ctx, "camp", mirasPlayer, "c2")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range lines {
		got = append(got, l.Summary)
	}
	want := map[string]bool{"Used 2 × Iron to make Sword": true, "Made 1 × Potion (Healing Draught)": true, "Your GM gave you 1 × Potion": true}
	if len(got) != len(want) {
		t.Fatalf("history %q", got)
	}
	for _, s := range got {
		if !want[s] {
			t.Fatalf("history %q has an unexpected line %q", got, s)
		}
	}
	// The API's summary comes from MoveSummary, which reads the same reason.
	if s := MoveSummary(MoveLine{Move: *g.repo.moves[0]}); s != "Used 2 × Iron to make Sword" {
		t.Fatalf("MoveSummary %q", s)
	}
}

func TestCraftGive(t *testing.T) {
	tests := []struct {
		name       string
		who        Actor
		char, item string
		qty        int
		label      string
		wantCode   int
		wantReason string
	}{
		{"the hero's player", mirasPlayer, "c2", "i2", 2, "Secret Brew", 0, "Made 2 × Secret (Secret Brew)"},
		{"the label is the item's name", gm, "c2", "i2", 1, "Secret", 0, "Made 1 × Secret"},
		{"another player's hero", Actor{"u1", rPlayer}, "c2", "i2", 1, "", http.StatusForbidden, ""},
		{"zero", mirasPlayer, "c2", "i2", 0, "", http.StatusBadRequest, ""},
		{"too many", mirasPlayer, "c2", "i2", maxMoveQuantity + 1, "", http.StatusBadRequest, ""},
		{"not an item", mirasPlayer, "c2", "n1", 1, "", http.StatusNotFound, ""},
		{"not a character", gm, "i1", "i2", 1, "", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newCraftFx()
			err := g.svc.CraftGive(context.Background(), "camp", tt.who, tt.char, tt.item, tt.qty, tt.label, Memory{})
			if got := code(err); got != tt.wantCode {
				t.Fatalf("code %d (%v), want %d", got, err, tt.wantCode)
			}
			if tt.wantCode != 0 {
				if g.carried("c2", "i2") != 0 || len(g.repo.moves) != 0 {
					t.Fatal("a refused give changed something")
				}
				return
			}
			if g.carried(tt.char, tt.item) != tt.qty {
				t.Fatalf("holds %d, want %d", g.carried(tt.char, tt.item), tt.qty)
			}
			// The player sees what they made, even an item kept private.
			if users := g.hand.allowed[tt.item]; len(users) != 1 || users[0] != "u2" {
				t.Fatalf("shared with %v", users)
			}
			if len(g.repo.moves) != 1 || g.repo.moves[0].Reason != tt.wantReason || !g.repo.moves[0].IsGive() {
				t.Fatalf("moves %+v", g.repo.moves)
			}
			if got := g.events.types(); len(got) != 1 || got[0] != EventStashMoved {
				t.Fatalf("events %v", got)
			}
		})
	}
}

func TestSpoilGive(t *testing.T) {
	tests := []struct {
		name       string
		who        Actor
		harvested  bool
		wantCode   int
		wantReason string
	}{
		{"taken as it is", mirasPlayer, false, 0, "Took 2 × Secret from the spoils"},
		{"harvested", gm, true, 0, "Harvested 2 × Secret"},
		{"another player's hero", Actor{"u1", rPlayer}, false, http.StatusForbidden, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newCraftFx()
			err := g.svc.SpoilGive(context.Background(), "camp", tt.who, "c2", "i2", 2, tt.harvested, Memory{})
			if got := code(err); got != tt.wantCode {
				t.Fatalf("code %d (%v), want %d", got, err, tt.wantCode)
			}
			if tt.wantCode != 0 {
				if g.carried("c2", "i2") != 0 || len(g.repo.moves) != 0 {
					t.Fatal("a refused give changed something")
				}
				return
			}
			// The player sees what they took, even an item kept private, and
			// the history names the item but not where it dropped.
			if g.carried("c2", "i2") != 2 || len(g.hand.allowed["i2"]) != 1 {
				t.Fatalf("holds %d, shared with %v", g.carried("c2", "i2"), g.hand.allowed["i2"])
			}
			if len(g.repo.moves) != 1 || g.repo.moves[0].Reason != tt.wantReason {
				t.Fatalf("moves %+v", g.repo.moves)
			}
		})
	}
}

func TestCraftPack(t *testing.T) {
	tests := []struct {
		name     string
		who      Actor
		char     string
		wantKind string
		wantHave Cents
		wantCode int
	}{
		{"coins", mirasPlayer, "c2", BuyKindCoins, 1000, 0},
		{"purse in copper", Actor{"u1", rPlayer}, "c5", BuyKindPurse, 301, 0},
		{"Wealth", gm, "c6", BuyKindWealth, 200, 0},
		{"no money field", gm, "c3", "", 0, 0},
		{"another player's hero", Actor{"u1", rPlayer}, "c2", "", 0, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newCraftFx()
			p, err := g.svc.CraftPack(context.Background(), "camp", tt.who, tt.char)
			if got := code(err); got != tt.wantCode {
				t.Fatalf("code %d (%v), want %d", got, err, tt.wantCode)
			}
			if err != nil {
				return
			}
			if p.PayKind != tt.wantKind || p.Have != tt.wantHave || len(p.Items) == 0 {
				t.Fatalf("pack %+v", p)
			}
		})
	}
}

func TestCraftCharge(t *testing.T) {
	tests := []struct {
		name     string
		pack     CraftPack
		total    Cents
		currency string
		want     Cents
	}{
		{"purse converts gold to copper", CraftPack{PayKind: BuyKindPurse}, 150, "gp", 150},
		{"purse converts silver", CraftPack{PayKind: BuyKindPurse}, 500, "sp", 50},
		{"gp sheet converts silver", CraftPack{PayKind: BuyKindCoins, MoneyKey: "gp"}, 500, "sp", 50},
		{"other money at face value", CraftPack{PayKind: BuyKindCoins, MoneyKey: "coins"}, 500, "sp", 500},
		{"Wealth at face value", CraftPack{PayKind: BuyKindWealth}, 300, "gp", 300},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CraftCharge(&tt.pack, tt.total, tt.currency); got != tt.want {
				t.Fatalf("CraftCharge = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCraft_RememberedCopies(t *testing.T) {
	g, rf := newCraftFx()
	cs := &fakeCopies{}
	g.svc = NewStashService(StashDeps{
		Repo: g.repo, Directory: g.dir, Visibility: g.vis, Actor: fakeActor{g.fx.dir},
		Fields: rf, Relations: g.rels, Events: g.events, Handouts: g.hand, Notifier: g.notify, Copies: cs,
	})
	ctx := context.Background()
	// Mira's 4 Iron: 2 moonlit, 1 poor, 1 plain.
	moon, _ := cs.Add(ctx, "camp", HolderCharacter, "c2", Copy{ItemID: "i3", Quantity: 2, Memory: Memory{Marks: []string{"moonlit"}}})
	cs.Add(ctx, "camp", HolderCharacter, "c2", Copy{ItemID: "i3", Quantity: 1, Memory: Memory{Quality: QualityPoor}})

	pack, err := g.svc.CraftPack(ctx, "camp", mirasPlayer, "c2")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range pack.Items {
		if it.ItemID == "i3" && (len(it.Copies) != 2 || it.Copies[0].Quantity != 2) {
			t.Fatalf("the pack's Iron copies %+v", it.Copies)
		}
	}

	out, err := g.svc.CraftSpend(ctx, "camp", mirasPlayer, CraftSpendInput{CharacterID: "c2", Label: "Sword",
		Items: []CraftItem{{ItemID: "i3", Quantity: 2, Pick: []CopyPick{{ID: moon, N: 1}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Used) != 1 || !out.Used[0].Memory.HasMark("moonlit") || out.Used[0].Quantity != 1 {
		t.Fatalf("used %+v, want one moonlit (the other one plain)", out.Used)
	}
	if got := cs.held(HolderCharacter, "c2"); got["moonlit"] != 1 || got["poor"] != 1 {
		t.Fatalf("left %v", got)
	}

	made := Memory{Quality: QualityFine, How: "made", By: "Mira", On: "3 Frostmoon"}
	if err := g.svc.CraftGive(ctx, "camp", mirasPlayer, "c2", "i1", 1, "Healing Draught", made); err != nil {
		t.Fatal(err)
	}
	if err := g.svc.CraftGive(ctx, "camp", mirasPlayer, "c2", "i1", 1, "Healing Draught", Memory{}); err != nil {
		t.Fatal(err)
	}
	if got := cs.held(HolderCharacter, "c2"); got["fine"] != 1 || len(cs.rows) != 3 {
		t.Fatalf("after two makes, one fine and one plain: %v (%d rows)", got, len(cs.rows))
	}
}
