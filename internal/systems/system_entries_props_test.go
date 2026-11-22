package systems

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

func words(n, size int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = strings.Repeat("w", size-1) + string(rune('a'+i%26))
	}
	return out
}

// The shapes a typed-in detail may take, each at its limits.
func TestValidateEntryProperties_Shapes(t *testing.T) {
	sixLists := map[string]any{}
	for _, k := range []string{"a", "b", "c", "d", "e", "f"} {
		sixLists[k] = []any{"x"}
	}
	sevenLists := map[string]any{"g": []any{"x"}}
	for k, v := range sixLists {
		sevenLists[k] = v
	}
	big := map[string]any{}
	for _, k := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q"} {
		big[k] = strings.Repeat("t", maxEntryPropText)
	}
	tests := []struct {
		name string
		p    map[string]any
		ok   bool
	}{
		{"list of words", map[string]any{"quick_build": []any{"Grounded", "Spark"}}, true},
		{"most words", map[string]any{"w": words(maxEntryListItems, 5)}, true},
		{"too many words", map[string]any{"w": words(maxEntryListItems+1, 5)}, false},
		{"longest word", map[string]any{"w": words(1, maxEntryPropString)}, true},
		{"word too long", map[string]any{"w": words(1, maxEntryPropString+1)}, false},
		{"most lists of words", sixLists, true},
		{"too many lists of words", sevenLists, false},
		{"words and items mixed", map[string]any{"w": []any{"a", map[string]any{"name": "b"}}}, false},
		{"a number in a list", map[string]any{"w": []any{"a", 1.0}}, false},
		{"empty list", map[string]any{"w": []any{}}, false},
		{"longest text", map[string]any{"benefit": strings.Repeat("t", maxEntryPropText)}, true},
		{"text too long", map[string]any{"benefit": strings.Repeat("t", maxEntryPropText+1)}, false},
		{"bytes cap still holds", big, false},
		{"object typed in", map[string]any{"ability": map[string]any{"a": 1.0}}, false},
		{"extra item key typed in", map[string]any{"traits": []any{map[string]any{"name": "X", "roll": "1"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateEntryProperties(tt.p); (err == nil) != tt.ok {
				t.Fatalf("ok = %v, err %v", tt.ok, err)
			}
		})
	}
}

// A value the validator does not understand passes only unchanged from what
// is stored; an item's extra key is compared with the stored item of the
// same name in the same list.
func TestValidateEntryProperties_Carried(t *testing.T) {
	ability := map[string]any{"type": "triggered", "distance": 1.0}
	stored := map[string]any{
		"signature_ability": map[string]any{"name": "Rune", "keywords": []any{"Magic"}},
		"traits":            []any{map[string]any{"name": "Stone Skin", "cost": 1.0, "ability": ability}},
		"other":             []any{map[string]any{"name": "Grounded"}},
	}
	item := func(name string, ab any, cost float64, text string) []any {
		it := map[string]any{"name": name, "cost": cost}
		if ab != nil {
			it["ability"] = ab
		}
		if text != "" {
			it["description"] = text
		}
		return []any{it}
	}
	tests := []struct {
		name   string
		p      map[string]any
		stored map[string]any
		ok     bool
	}{
		{"object unchanged", map[string]any{"signature_ability": map[string]any{"keywords": []any{"Magic"}, "name": "Rune"}}, stored, true},
		{"object changed", map[string]any{"signature_ability": map[string]any{"name": "Rune", "keywords": []any{"Fire"}}}, stored, false},
		{"object with nothing stored", map[string]any{"signature_ability": stored["signature_ability"]}, nil, false},
		{"object under another key", map[string]any{"sig": stored["signature_ability"]}, stored, false},
		{"item reworded, ability rides along", map[string]any{"traits": item("Stone Skin", ability, 2, "Tougher.")}, stored, true},
		{"item matched by name in any casing", map[string]any{"traits": item("stone skin", map[string]any{"distance": 1, "type": "triggered"}, 1, "")}, stored, true},
		{"ability changed", map[string]any{"traits": item("Stone Skin", map[string]any{"type": "action"}, 1, "")}, stored, false},
		{"renamed item loses its ability", map[string]any{"traits": item("Iron Skin", ability, 1, "")}, stored, false},
		{"renamed item without it", map[string]any{"traits": item("Iron Skin", nil, 1, "")}, stored, true},
		{"ability moved to another list", map[string]any{"other": item("Stone Skin", ability, 1, "")}, stored, false},
		{"bad cost still refused", map[string]any{"traits": item("Stone Skin", ability, 99, "")}, stored, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateEntryPropertiesOnto(tt.p, tt.stored); (err == nil) != tt.ok {
				t.Fatalf("ok = %v, err %v", tt.ok, err)
			}
		})
	}
}

func TestDropDerivedTwins(t *testing.T) {
	p := map[string]any{"traits": []any{}, "traits_display": "x", "provenance_display": "kept", "_display": "kept too", "speed": 5.0}
	dropDerivedTwins(p)
	if _, ok := p["traits_display"]; ok {
		t.Fatal("a twin with its base present was kept")
	}
	for _, k := range []string{"provenance_display", "_display", "traits", "speed"} {
		if _, ok := p[k]; !ok {
			t.Fatalf("%s dropped", k)
		}
	}
}

// A rewrite carries the package's values in verbatim (twins dropped, the
// snapshot whole); later saves may keep them only unchanged; nobody can type
// one in, and a player's own entry has none to carry.
func TestSystemEntryReading_CarriedThroughService(t *testing.T) {
	svc, repo, data := newReadingSvc(t)
	ctx := context.Background()
	e, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "dwarf", Name: "Hill Dwarf"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := e.Properties["signature_traits_display"]; ok {
		t.Fatal("the generated twin was copied")
	}
	if e.Properties["provenance_display"] != "Printed in the core rules." || e.Properties["breath"] == nil {
		t.Fatalf("copy lost a value: %+v", e.Properties)
	}
	var snap StandardSnapshot
	_ = json.Unmarshal([]byte(e.StandsForSeen), &snap)
	if snap.Properties["signature_traits_display"] == nil {
		t.Fatal("the snapshot keeps the standard entry whole")
	}
	std, _ := data.Get("ancestries", "dwarf")
	withTrait := func(trait map[string]any, breath any) map[string]any {
		p := copyProps(e.Properties)
		p["purchased_traits"] = []any{trait}
		if breath != nil {
			p["breath"] = breath
		}
		return p
	}
	ability := std.Properties["purchased_traits"].([]any)[0].(map[string]any)["ability"]
	tests := []struct {
		name    string
		actor   EntryActor
		id      int64
		patch   UpdateSystemEntryInput
		wantErr int
	}{
		{"rename keeps everything", dirActor, e.ID, UpdateSystemEntryInput{Name: patch.Of("Hillfolk")}, 0},
		{"reword a trait and change its cost", dirActor, e.ID, UpdateSystemEntryInput{Properties: patch.Of(withTrait(
			map[string]any{"name": "Stone Skin", "description": "Tougher.", "cost": 2.0, "ability": ability}, nil))}, 0},
		{"change the trait's ability", dirActor, e.ID, UpdateSystemEntryInput{Properties: patch.Of(withTrait(
			map[string]any{"name": "Stone Skin", "ability": map[string]any{"type": "free"}}, nil))}, 422},
		{"change the carried object", dirActor, e.ID, UpdateSystemEntryInput{Properties: patch.Of(withTrait(
			map[string]any{"name": "Stone Skin", "ability": ability}, map[string]any{"damage": "9d6"}))}, 422},
		{"type an object onto another entry", dirActor, entryByName(t, repo, "Dwarf").ID,
			UpdateSystemEntryInput{Properties: patch.Of(map[string]any{"breath": std.Properties["breath"]})}, 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Update(ctx, "c1", tt.actor, tt.id, tt.patch)
			wantCode(t, err, tt.wantErr)
		})
	}
	_, err = svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Typed", Properties: map[string]any{"breath": std.Properties["breath"]}})
	wantCode(t, err, 422)

	// A player's own draft: nothing stored to carry, whatever they send.
	if _, err := svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "ancestry", UserID: "pl"}); err != nil {
		t.Fatal(err)
	}
	d, err := svc.Create(ctx, "c1", playerActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Mine",
		Properties: map[string]any{"purchased_traits": []any{map[string]any{"name": "Stone Skin", "cost": 1.0}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []map[string]any{
		{"breath": std.Properties["breath"]},
		{"purchased_traits": []any{map[string]any{"name": "Stone Skin", "cost": 1.0, "ability": ability}}},
		std.Properties,
	} {
		_, err := svc.Update(ctx, "c1", playerActor, d.ID, UpdateSystemEntryInput{Properties: patch.Of(p)})
		wantCode(t, err, 422)
	}
}

// Rewritable says no once the entry is left out or already has a stand-in,
// and for a class whatever its shape.
func TestSystemEntryReading_RewritableState(t *testing.T) {
	svc, repo, _ := newReadingSvc(t)
	ctx := context.Background()
	if ok, _ := svc.Rewritable(ctx, "c1", "ancestry", "polder"); !ok {
		t.Fatal("polder copies cleanly")
	}
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.Rewritable(ctx, "c1", "ancestry", "polder"); ok {
		t.Fatal("a left-out entry is not offered a rewrite")
	}
	if _, err := svc.Tie(ctx, "c1", dirActor, entryByName(t, repo, "Dwarf").ID, "dwarf"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.Rewritable(ctx, "c1", "ancestry", "dwarf"); ok {
		t.Fatal("an entry with a stand-in is not offered a rewrite")
	}
	_, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "class", Slug: "censor"})
	if err == nil || !strings.Contains(err.Error(), "class") {
		t.Fatalf("a class is refused in a plain sentence: %v", err)
	}
}

// A changed carried part reads as the trait reworded, in both descriptions.
func TestChanges_CarriedParts(t *testing.T) {
	it := func(ab string) map[string]any {
		return map[string]any{"purchased_traits": []any{map[string]any{"name": "Stone Skin", "cost": 1.0, "ability": map[string]any{"type": ab}}}}
	}
	got := ChangedTraits(&SystemEntry{Properties: it("triggered")}, &SystemEntry{Properties: it("free")})
	if len(got) != 1 || !got[0].TextChanged || got[0].CostChanged {
		t.Fatalf("ChangedTraits = %+v", got)
	}
	if got := ChangedTraits(&SystemEntry{Properties: it("triggered")}, &SystemEntry{Properties: it("triggered")}); len(got) != 0 {
		t.Fatalf("unchanged = %+v", got)
	}
	line := DescribeStandardChange(StandardSnapshot{Properties: it("triggered")}, StandardSnapshot{Properties: it("free")})
	if line != "Stone Skin reworded." {
		t.Fatalf("line = %q", line)
	}
}

// A card border must be a library key and a card colour six hex digits; the
// refusal names the valid keys.
func TestValidateEntryProperties_CardLook(t *testing.T) {
	tests := []struct {
		name    string
		p       map[string]any
		wantErr string
	}{
		{"good pair", map[string]any{"card_border": "fire", "card_colour": "#d9622b"}, ""},
		{"upper-case colour", map[string]any{"card_border": "ice", "card_colour": "#8FD8FF"}, ""},
		{"border alone", map[string]any{"card_border": "vine"}, ""},
		{"unknown border", map[string]any{"card_border": "lava"}, "carved-stone"},
		{"border of the wrong type", map[string]any{"card_border": 3.0}, "must be one of"},
		{"empty border", map[string]any{"card_border": ""}, "must be one of"},
		{"short colour", map[string]any{"card_colour": "#fff"}, "six hex digits"},
		{"colour without hash", map[string]any{"card_colour": "d9622b"}, "six hex digits"},
		{"colour of the wrong type", map[string]any{"card_colour": 255.0}, "six hex digits"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEntryProperties(tt.p)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
			if tt.name == "unknown border" && !strings.Contains(err.Error(), "vine") {
				t.Errorf("the refusal should list every key: %v", err)
			}
		})
	}
	if g := CardBorderGuide(); !strings.Contains(g, "thorn-glass (Thorn glass)") || !strings.Contains(g, "card_colour") {
		t.Errorf("guide lacks a border or the colour property: %s", g)
	}
}
