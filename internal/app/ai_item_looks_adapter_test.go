package app

import (
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
)

// TestItemLookChoicesMatchService pins the looks, colours, sounds and spots
// the AI is told about to the ones the look service accepts, so the
// item-look block's docs never offer a choice the save refuses.
func TestItemLookChoicesMatchService(t *testing.T) {
	looks, colors, sounds, spots := records.ItemLookChoices()
	if len(looks) != 9 || len(colors) != 8 || len(sounds) != 9 || len(spots) != 5 {
		t.Fatalf("the lists changed size (%d looks, %d colours, %d sounds, %d spots): update records/item_looks.go with the look service", len(looks), len(colors), len(sounds), len(spots))
	}
	for _, l := range looks {
		for _, c := range colors {
			v := records.ItemLookValue{Look: l, Tint: c, Spots: map[string]records.ItemLookSpot{}}
			for _, s := range spots {
				v.Spots[s] = records.ItemLookSpot{Look: l, Tint: c}
			}
			for _, snd := range sounds {
				v.Sound = snd
				raw, err := lookJSON(v)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := armory.NormalizeItemLook(raw); err != nil {
					t.Fatalf("the service refuses %s: %v", raw, err)
				}
			}
		}
	}
}

// lookValue and lookJSON carry every field both ways.
func TestItemLookAdapterRoundTrip(t *testing.T) {
	in := records.ItemLookValue{Look: "ember", Tint: "#ffd66b", Sound: "none", Words: "6 fire", MadeFromID: "abc",
		Spots: map[string]records.ItemLookSpot{"impact": {Look: "storm", Tint: "#8fd8ff"}}}
	raw, err := lookJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := armory.NormalizeItemLook(raw)
	if err != nil {
		t.Fatal(err)
	}
	var l armory.ItemLook
	if err := json.Unmarshal(doc, &l); err != nil {
		t.Fatal(err)
	}
	out := lookValue(l)
	if out.Look != in.Look || out.Tint != in.Tint || out.Sound != in.Sound || out.Words != in.Words || out.MadeFromID != in.MadeFromID || out.Spots["impact"] != in.Spots["impact"] {
		t.Fatalf("round trip: got %+v, want %+v", out, in)
	}
}
