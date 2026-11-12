package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/designs"
)

// The room-theme kind is the armory's own counter-theme check, so a starter
// theme and the example the AI is shown both pass, and an unknown part is
// refused by it.
func TestRoomThemeKindUsesTheArmorysRules(t *testing.T) {
	k, ok := newDesignKinds().Kind(designs.KindRoomTheme)
	if !ok {
		t.Fatal("room-theme is not registered")
	}
	if _, err := k.Validate(armory.ShopRoomCounterStarter("forge")); err != nil {
		t.Errorf("a starter theme was refused: %v", err)
	}
	doc := records.DesignKind{}.Doc()
	i := strings.Index(doc, "data: {")
	if i < 0 {
		t.Fatal("the design block's example has no data")
	}
	example := doc[i+len("data: "):]
	example = example[:strings.Index(example, "\n")]
	if _, err := k.Validate(json.RawMessage(example)); err != nil {
		t.Errorf("the example in the block's doc is refused by the armory: %v\n%s", err, example)
	}
	bad := strings.Replace(example, `"anvil"`, `"dragon"`, 1)
	if _, err := k.Validate(json.RawMessage(bad)); err == nil {
		t.Error("an unknown prop was accepted")
	}
	if lacks, err := k.Lacks(context.Background(), "c", nil); err != nil || len(lacks) != 0 {
		t.Errorf("room themes need nothing of a campaign: %v %v", lacks, err)
	}
}

// The kind registry starts with room-theme and takes more, which is how
// crafting adds its kinds without this package changing.
func TestDesignKindsAcceptLaterKinds(t *testing.T) {
	r := newDesignKinds()
	r.Register("crafting-circle", roomThemeKind{})
	if got := strings.Join(r.Names(), ","); got != "crafting-circle,room-theme" {
		t.Errorf("kinds = %s", got)
	}
}
