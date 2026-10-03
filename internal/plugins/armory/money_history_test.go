package armory

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/changesource"
)

func TestMoneyHistory_RecordFieldChange(t *testing.T) {
	src := func(kind, label string) context.Context {
		return changesource.With(context.Background(), changesource.Source{Kind: kind, Label: label, UserID: "u9"})
	}
	tests := []struct {
		name       string
		ctx        context.Context
		entity     string
		old, new   map[string]any
		disabled   bool
		wantReason string // empty means no row
		wantAmount Cents
	}{
		{"web edit", src(changesource.KindWeb, ""), "c1",
			map[string]any{"gp": 2.0}, map[string]any{"gp": 3.0},
			false, "gp changed 2 → 3 · on the sheet", 100},
		{"foundry edit", src(changesource.KindFoundry, ""), "c1",
			map[string]any{"gp": 2.0}, map[string]any{"gp": 3.0},
			false, "gp changed 2 → 3 · in Foundry", 100},
		{"extension edit", src(changesource.KindExtension, ""), "c1",
			map[string]any{"gp": 2.0}, map[string]any{"gp": 1.5},
			false, "gp changed 2 → 1.50 · by an extension", -50},
		{"shop with a label", src(changesource.KindShop, "Bought at Smithy"), "c1",
			map[string]any{"gp": 20.0}, map[string]any{"gp": 5.0},
			false, "gp changed 20 → 5 · Bought at Smithy", -1500},
		{"shop without a label", src(changesource.KindShop, ""), "c1",
			map[string]any{"gp": 20.0}, map[string]any{"gp": 5.0},
			false, "gp changed 20 → 5 · at a shop", -1500},
		{"absent field counts as zero", src(changesource.KindWeb, ""), "c1",
			map[string]any{}, map[string]any{"gp": 4.0},
			false, "gp changed 0 → 4 · on the sheet", 400},
		{"text numerals compare as numbers", src(changesource.KindWeb, ""), "c2",
			map[string]any{"gp": "10"}, map[string]any{"gp": 12.0},
			false, "gp changed 10 → 12 · on the sheet", 200},
		{"a stash move logs itself", src(changesource.KindStash, ""), "c1",
			map[string]any{"gp": 2.0}, map[string]any{"gp": 3.0}, false, "", 0},
		{"unchanged amount", src(changesource.KindWeb, ""), "c1",
			map[string]any{"gp": 2.0}, map[string]any{"gp": 2.0}, false, "", 0},
		{"same value in another notation", src(changesource.KindWeb, ""), "c2",
			map[string]any{"gp": "2"}, map[string]any{"gp": 2.0}, false, "", 0},
		{"other field only", src(changesource.KindWeb, ""), "c1",
			map[string]any{"gp": 2.0, "hp": 1}, map[string]any{"gp": 2.0, "hp": 5}, false, "", 0},
		{"not a character", src(changesource.KindWeb, ""), "n1",
			map[string]any{"gp": 2.0}, map[string]any{"gp": 3.0}, false, "", 0},
		{"character without a money field", src(changesource.KindWeb, ""), "c3",
			map[string]any{"gp": 2.0}, map[string]any{"gp": 3.0}, false, "", 0},
		{"no source set", context.Background(), "c1",
			map[string]any{"gp": 2.0}, map[string]any{"gp": 3.0}, false, "", 0},
		{"plugin disabled for the campaign", src(changesource.KindWeb, ""), "c1",
			map[string]any{"gp": 2.0}, map[string]any{"gp": 3.0}, true, "", 0},
		{"value that is not a number", src(changesource.KindWeb, ""), "c1",
			map[string]any{"gp": 2.0}, map[string]any{"gp": "lots"}, false, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFx()
			ev := &fakeEvents{}
			h := NewMoneyHistory(f.repo, f.dir, func(context.Context, string) bool { return !tt.disabled }, ev)

			if err := h.RecordFieldChange(tt.ctx, "camp", tt.entity, tt.old, tt.new); err != nil {
				t.Fatal(err)
			}
			if tt.wantReason == "" {
				if len(f.repo.moves) != 0 || len(ev.got) != 0 {
					t.Fatalf("expected nothing logged, got %+v / %v", f.repo.moves, ev.types())
				}
				return
			}
			if len(f.repo.moves) != 1 {
				t.Fatalf("rows = %d", len(f.repo.moves))
			}
			m := f.repo.moves[0]
			end := charEP(tt.entity)
			if m.Kind != MoveKindMoney || m.From != end || m.To != end || m.Status != MoveApplied ||
				m.Reason != tt.wantReason || m.Amount != tt.wantAmount || m.RequestedBy != "u9" || m.CampaignID != "camp" {
				t.Fatalf("row = %+v", m)
			}
			if !m.IsMoneyEdit() {
				t.Fatal("row is not recognised as a money edit")
			}
			if got := ev.types(); len(got) != 1 || got[0] != EventStashMoneyChanged ||
				ev.got[0].payload["characterId"] != tt.entity || ev.got[0].payload["moveId"] != m.ID {
				t.Fatalf("events = %+v", ev.got)
			}
		})
	}
}

func TestMoneyHistory_UsesFieldLabelAndClipsReason(t *testing.T) {
	f := newFx()
	f.dir.ents["c1"].MoneyLabel = "Wealth"
	h := NewMoneyHistory(f.repo, f.dir, nil, nil)
	ctx := changesource.With(context.Background(), changesource.Source{Kind: changesource.KindShop, Label: strings.Repeat("é", 400)})
	if err := h.RecordFieldChange(ctx, "camp", "c1", map[string]any{"gp": 2.0}, map[string]any{"gp": 3.0}); err != nil {
		t.Fatal(err)
	}
	r := []rune(f.repo.moves[0].Reason)
	if len(r) != maxReasonLen || !strings.HasPrefix(string(r), "Wealth changed 2 → 3 · ") {
		t.Fatalf("reason = %q (%d runes)", f.repo.moves[0].Reason, len(r))
	}
}

// The edit shows up in the history of the character it happened to, for the
// people who can already act as that character, and for nobody else.
func TestMoneyHistory_RowFollowsHistoryVisibility(t *testing.T) {
	f := newFx()
	f.dir.ents["c1"].MoneyLabel = "Wealth"
	h := NewMoneyHistory(f.repo, f.dir, nil, nil)
	ctx := changesource.With(context.Background(), changesource.Source{Kind: changesource.KindFoundry, UserID: "gm"})
	if err := h.RecordFieldChange(ctx, "camp", "c1", map[string]any{"gp": 2.0}, map[string]any{"gp": 3.0}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		actor    Actor
		wantCode int
	}{
		{"owner of the character", Actor{"u1", rPlayer}, 0},
		{"scribe", Actor{"sc", rScribe}, 0},
		{"owner", Actor{"gm", rOwner}, 0},
		{"another player", Actor{"u2", rPlayer}, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, err := f.svc.CharacterHistory(context.Background(), "camp", tt.actor, "c1")
			if code(err) != tt.wantCode {
				t.Fatalf("err = %v", err)
			}
			if tt.wantCode != 0 {
				return
			}
			if len(lines) != 1 || MoveSummary(lines[0]) != "Wealth changed 2 → 3 · in Foundry" {
				t.Fatalf("lines = %+v", lines)
			}
		})
	}
}

func TestShopPurchaseSource(t *testing.T) {
	got, ok := changesource.From(ShopPurchaseSource(context.Background(), "u3", "Smithy"))
	if !ok || got != (changesource.Source{Kind: changesource.KindShop, Label: "Smithy", UserID: "u3"}) {
		t.Fatalf("source = %+v, %v", got, ok)
	}
}

func TestMoveSummary_MoneyEditVersusMove(t *testing.T) {
	edit := MoveLine{Move: Move{Kind: MoveKindMoney, From: charEP("c1"), To: charEP("c1"), Status: MoveApplied, Reason: "Wealth changed 2 → 3 · in Foundry"}}
	move := MoveLine{Move: Move{Kind: MoveKindMoney, Amount: 500, From: charEP("c1"), To: charEP("c2"), Status: MoveApplied}, FromName: "A", ToName: "B", RequesterName: "Pat"}
	if got := MoveSummary(edit); got != edit.Reason {
		t.Errorf("edit summary = %q", got)
	}
	if got := MoveSummary(move); got != "Pat moved 5 money from A to B." {
		t.Errorf("move summary = %q", got)
	}
}
