package entities

import (
	"strings"
	"testing"
)

// TestShopOnlyWhereAdded pins the opt-in rule for the shop: a page shows it
// only where the owner placed the Shop block in the layout. show.templ must
// not mount it on its own for shop-type pages.
func TestShopOnlyWhereAdded(t *testing.T) {
	src := readRepoFile(t, "internal/plugins/entities/show.templ")
	if n := strings.Count(src, "@blockShopInventory("); n != 0 {
		t.Errorf("show.templ mounts the shop by itself %d time(s); it must come only from a placed Shop block", n)
	}
	reg := readRepoFile(t, "internal/plugins/entities/block_registry_core.go")
	if !strings.Contains(reg, `Type: "shop_inventory", Label: "Shop"`) {
		t.Errorf("the Shop block must stay registered so the owner can add it")
	}
}
