package campaigns

import (
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// kindIcon is the report noun for an icon the import could not keep. The
// object itself is still created, so the row names the icon, not the object.
const kindIcon = "icon"

// iconReplacedReason is the user-safe report reason for a discarded icon.
const iconReplacedReason = "not a valid icon name; the default icon was used"

// normalizeImportIcons clears every icon in an import file that fails the
// shared icon check, so each owning service applies its own default. One bad
// icon never fails the object or the run; each replacement is logged and
// noted in the report.
func normalizeImportIcons(data *CampaignExport, report *ImportReport) {
	if data == nil {
		return
	}
	fix := func(section, name string, icon *string) {
		cleaned, replaced := sanitize.IconOrDefault(*icon, "")
		if replaced {
			slog.Warn("import: replaced invalid icon",
				slog.String("section", section),
				slog.String("name", name),
				slog.String("icon", *icon),
			)
			report.Fail(section, kindIcon, name, iconReplacedReason)
		}
		*icon = cleaned
	}
	for i := range data.EntityTypes {
		fix("entities", data.EntityTypes[i].Name, &data.EntityTypes[i].Icon)
	}
	for i := range data.Timelines {
		fix(SectionTimelines, data.Timelines[i].Name, &data.Timelines[i].Icon)
	}
	for i := range data.Maps {
		for j := range data.Maps[i].Markers {
			fix("maps", data.Maps[i].Markers[j].Name, &data.Maps[i].Markers[j].Icon)
		}
	}
}

// normalizeSidebarIcons applies the same rule to link icons in an imported
// sidebar layout, so one bad icon doesn't drop the whole layout.
func normalizeSidebarIcons(items []SidebarItem, campaignName string, report *ImportReport) {
	for i := range items {
		cleaned, replaced := sanitize.IconOrDefault(items[i].Icon, "")
		if replaced {
			slog.Warn("import: replaced invalid sidebar icon",
				slog.String("label", items[i].Label),
				slog.String("icon", items[i].Icon),
			)
			name := items[i].Label
			if name == "" {
				name = campaignName
			}
			report.Fail("campaign", kindIcon, name, iconReplacedReason)
		}
		items[i].Icon = cleaned
	}
}
