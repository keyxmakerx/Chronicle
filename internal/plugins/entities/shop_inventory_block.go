package entities

import (
	"encoding/json"
	"strings"
)

// shopTypeSlug is the entity-type slug whose pages carry a shop inventory.
const shopTypeSlug = "shop"

// needsImplicitShopInventory reports whether a shop page must render the
// shop-inventory widget even though its layout does not place it. Shop types
// are seeded with DefaultLayout, which has no shop_inventory block, so
// without this the inventory never appears on a shop page. A layout that
// places the block anywhere (including inside a container block's config)
// owns its position, and the widget is not rendered twice.
func needsImplicitShopInventory(entity *Entity, entityType *EntityType) bool {
	if entity == nil || entity.TypeSlug != shopTypeSlug {
		return false
	}
	if entityType == nil {
		return true
	}
	return !layoutPlacesBlock(entityType.Layout, "shop_inventory")
}

// layoutPlacesBlock reports whether any block of the given type appears in
// the layout. Container blocks (tabs, columns, sections) keep their children
// in free-form config, so the check scans the serialized layout rather than
// walking a fixed shape.
func layoutPlacesBlock(layout EntityTypeLayout, blockType string) bool {
	raw, err := json.Marshal(layout)
	if err != nil {
		return false
	}
	needle, _ := json.Marshal(blockType)
	return strings.Contains(string(raw), `"type":`+string(needle))
}
