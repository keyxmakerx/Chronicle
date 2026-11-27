package systems

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
)

// EntryItemDef says what Foundry item each named object in one list property
// of a campaign's own entry becomes (an ancestry's signature traits as
// features). The Foundry module makes one item per object, beside the item
// sync makes for the entry, and takes them away with it.
type EntryItemDef struct {
	Type string `json:"type"`
}

// entryItemType is a Foundry item type name: a plain identifier. The module
// also requires it to be a type the game system defines.
var entryItemType = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,31}$`)

// sanitizeEntryItems drops foundry_entry_items mappings that can't be used,
// recording a warning for each, the same way and on the same fields as
// sanitizeEntryFields.
func (m *SystemManifest) sanitizeEntryItems() {
	for i := range m.EntityPresets {
		preset := &m.EntityPresets[i]
		for j := range preset.Fields {
			f := &preset.Fields[j]
			if len(f.FoundryEntryItems) == 0 {
				continue
			}
			warn := func(reason string) {
				msg := fmt.Sprintf("Preset %q field %q: foundry_entry_items %s", preset.Slug, f.Key, reason)
				m.EntryFieldWarnings = append(m.EntryFieldWarnings, msg)
				slog.Warn("system manifest: entry item mapping dropped",
					slog.String("system", m.ID),
					slog.String("preset", preset.Slug),
					slog.String("field", f.Key),
					slog.String("reason", reason))
			}
			if reason := entryMappingRefused(f, len(f.FoundryEntryItems)); reason != "" {
				warn(reason)
				f.FoundryEntryItems = nil
				continue
			}
			keys := make([]string, 0, len(f.FoundryEntryItems))
			for k := range f.FoundryEntryItems {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				switch {
				case !ValidChoiceFieldKey(k):
					warn(fmt.Sprintf("%q ignored: the property is not a plain identifier", k))
					delete(f.FoundryEntryItems, k)
				case !entryItemType.MatchString(f.FoundryEntryItems[k].Type):
					warn(fmt.Sprintf("%q ignored: type must be a Foundry item type name (letters and digits, starting with a letter)", k))
					delete(f.FoundryEntryItems, k)
				}
			}
			if len(f.FoundryEntryItems) == 0 {
				f.FoundryEntryItems = nil
			}
		}
	}
}
