// marker_icons.go — Chronicle's canonical map-marker icon vocabulary
// (C-MAPS-EDITOR-PIN-AND-ICON-PARITY, Part A).
//
// Chronicle is authoritative for the marker icon set (Option 1 / §A4 coupling
// inversion: Chronicle is the world database). A marker stores its icon as the
// canonical Font Awesome class string (e.g. "fa-castle"); that same ID travels
// over the Foundry sync wire. The Foundry module keeps a translation table
// that maps each canonical ID to its own render mechanism, so the SAME ID
// means the SAME concept on both sides — which is what closes the
// icon-mismatch the operator hit.
//
// This file is the single source of truth: the editor's icon picker renders
// from it, and GET /campaigns/:id/maps/marker-icons exposes it so the Foundry
// module (a separate repo) can fetch the canonical list and align its
// translation table against a concrete contract rather than a hardcoded copy.
package maps

// MarkerIcon is one entry in the canonical vocabulary. ID is the stable Font
// Awesome class persisted on markers + sent over sync; Label is the human
// concept; Category groups the picker.
type MarkerIcon struct {
	ID       string `json:"id"`       // canonical Font Awesome class, e.g. "fa-castle"
	Label    string `json:"label"`    // human concept, e.g. "Castle"
	Category string `json:"category"` // picker group, e.g. "Fortifications"
}

// DefaultMarkerIcon is the fallback for a marker with no icon or an icon
// outside the canonical set (matches the renderers' "fa-map-pin" default).
const DefaultMarkerIcon = "fa-map-pin"

// markerIconCatalog is the ordered canonical vocabulary. ORDER MATTERS — the
// picker renders groups and options in this order. Adding/removing/renaming an
// entry changes the cross-system contract: keep the Foundry module's
// translation table (scripts/map-sync.mjs) in sync, or the same ID will render
// differently across sides again.
var markerIconCatalog = []MarkerIcon{
	// General
	{"fa-map-pin", "Pin", "General"},
	{"fa-location-dot", "Location", "General"},
	{"fa-star", "Star", "General"},
	{"fa-flag", "Flag", "General"},
	{"fa-landmark", "Landmark", "General"},
	// Settlements
	{"fa-city", "City", "Settlements"},
	{"fa-house", "House", "Settlements"},
	{"fa-shop", "Shop", "Settlements"},
	{"fa-building", "Town", "Settlements"},
	{"fa-tent", "Village", "Settlements"},
	{"fa-campground", "Camp", "Settlements"},
	// Fortifications
	{"fa-castle", "Castle", "Fortifications"},
	{"fa-chess-rook", "Tower", "Fortifications"},
	{"fa-shield-halved", "Fortress", "Fortifications"},
	{"fa-tower-observation", "Watchtower", "Fortifications"},
	// Dungeons & Ruins
	{"fa-dungeon", "Dungeon", "Dungeons & Ruins"},
	{"fa-skull", "Danger", "Dungeons & Ruins"},
	{"fa-skull-crossbones", "Ruins", "Dungeons & Ruins"},
	{"fa-door-open", "Entrance", "Dungeons & Ruins"},
	{"fa-stairs", "Underground", "Dungeons & Ruins"},
	// Nature
	{"fa-mountain", "Mountain", "Nature"},
	{"fa-tree", "Forest", "Nature"},
	{"fa-water", "Water", "Nature"},
	{"fa-mountain-sun", "Hills", "Nature"},
	{"fa-mound", "Cave", "Nature"},
	// Maritime
	{"fa-anchor", "Port", "Maritime"},
	{"fa-ship", "Ship", "Maritime"},
	{"fa-sailboat", "Sailboat", "Maritime"},
	{"fa-bridge", "Bridge", "Maritime"},
	// Sacred & Magic
	{"fa-cross", "Temple", "Sacred & Magic"},
	{"fa-place-of-worship", "Shrine", "Sacred & Magic"},
	{"fa-hat-wizard", "Wizard", "Sacred & Magic"},
	{"fa-book-open", "Library", "Sacred & Magic"},
	{"fa-gem", "Treasure", "Sacred & Magic"},
	{"fa-wand-sparkles", "Magic", "Sacred & Magic"},
	// Resources
	{"fa-helmet-safety", "Mine", "Resources"},
	{"fa-wheat-awn", "Farm", "Resources"},
	{"fa-wine-glass", "Tavern", "Resources"},
	{"fa-hammer", "Forge", "Resources"},
}

// markerIconSet indexes the catalog for O(1) validation. Built once at init.
var markerIconSet = func() map[string]bool {
	m := make(map[string]bool, len(markerIconCatalog))
	for _, ic := range markerIconCatalog {
		m[ic.ID] = true
	}
	return m
}()

// MarkerIconCatalog returns a copy of the canonical icon vocabulary in display
// order. Copy (not the package slice) so callers can't mutate the source.
func MarkerIconCatalog() []MarkerIcon {
	return append([]MarkerIcon(nil), markerIconCatalog...)
}

// MarkerIconGroup is a category and its icons, for grouped rendering.
type MarkerIconGroup struct {
	Category string       `json:"category"`
	Icons    []MarkerIcon `json:"icons"`
}

// MarkerIconGroups returns the catalog grouped by category, preserving the
// catalog's first-seen category order (so the picker is stable).
func MarkerIconGroups() []MarkerIconGroup {
	var groups []MarkerIconGroup
	idx := map[string]int{}
	for _, ic := range markerIconCatalog {
		i, ok := idx[ic.Category]
		if !ok {
			idx[ic.Category] = len(groups)
			groups = append(groups, MarkerIconGroup{Category: ic.Category})
			i = len(groups) - 1
		}
		groups[i].Icons = append(groups[i].Icons, ic)
	}
	return groups
}

// IsValidMarkerIcon reports whether id is in the canonical vocabulary.
func IsValidMarkerIcon(id string) bool { return markerIconSet[id] }

// NormalizeMarkerIcon returns id when it's canonical, else DefaultMarkerIcon.
// Render-time safety net so an unknown/empty icon (e.g. from an out-of-date
// sync peer) degrades to the pin rather than a broken glyph.
func NormalizeMarkerIcon(id string) string {
	if IsValidMarkerIcon(id) {
		return id
	}
	return DefaultMarkerIcon
}
