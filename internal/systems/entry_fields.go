package systems

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
)

// Entry field formats: how the Foundry module turns an entry property into the
// item's value.
const (
	// EntryFieldNumber writes a number (a numeric text such as "+2" is read as
	// one); anything else is left out.
	EntryFieldNumber = "number"
	// EntryFieldText writes the property as plain text.
	EntryFieldText = "text"
	// EntryFieldTiers splits "+2/+2/+4" (or one number for all three) into
	// path.tier1, path.tier2 and path.tier3.
	EntryFieldTiers = "tiers"

	maxEntryFields = 32
)

// EntryFieldDef is where one entry property goes on the Foundry item: a path
// under the item's system data, and how its value is read.
type EntryFieldDef struct {
	Path   string `json:"path"`
	Format string `json:"format"`
}

// entryFieldPath allows only the item's own system data: never its name,
// flags, ownership or embedded documents, which sync manages itself. Each
// segment starts with a letter, and reservedPathSegments are refused too, so
// a path can't name a JavaScript object's shared internals.
var entryFieldPath = regexp.MustCompile(`^system(\.[A-Za-z][A-Za-z0-9_]*){1,8}$`)

var reservedPathSegments = map[string]bool{"constructor": true, "prototype": true}

// usableEntryPath reports whether a mapping's path may be written on an item.
func usableEntryPath(path string) bool {
	if !entryFieldPath.MatchString(path) {
		return false
	}
	for _, seg := range strings.Split(path, ".") {
		if reservedPathSegments[seg] {
			return false
		}
	}
	return true
}

// sanitizeEntryFields drops foundry_entry_fields mappings that can't be used,
// recording a warning for each, so a bad mapping never refuses the system: the
// field keeps working and the item is made with its description only. A
// mapping belongs on a single-item collection field that players may see,
// since only those make an item from a campaign entry.
func (m *SystemManifest) sanitizeEntryFields() {
	for i := range m.EntityPresets {
		preset := &m.EntityPresets[i]
		for j := range preset.Fields {
			f := &preset.Fields[j]
			if len(f.FoundryEntryFields) == 0 {
				continue
			}
			warn := func(reason string) {
				msg := fmt.Sprintf("Preset %q field %q: foundry_entry_fields %s", preset.Slug, f.Key, reason)
				m.EntryFieldWarnings = append(m.EntryFieldWarnings, msg)
				slog.Warn("system manifest: entry field mapping dropped",
					slog.String("system", m.ID),
					slog.String("preset", preset.Slug),
					slog.String("field", f.Key),
					slog.String("reason", reason))
			}
			switch {
			case f.FoundryCollection != "items" || !f.FoundryItemSingle:
				warn("ignored: the field is not a single Foundry item")
				f.FoundryEntryFields = nil
				continue
			case f.GMOnly:
				warn("ignored: the field is GM-only")
				f.FoundryEntryFields = nil
				continue
			case len(f.FoundryEntryFields) > maxEntryFields:
				warn(fmt.Sprintf("ignored: more than %d properties", maxEntryFields))
				f.FoundryEntryFields = nil
				continue
			}
			keys := make([]string, 0, len(f.FoundryEntryFields))
			for k := range f.FoundryEntryFields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if reason := validateEntryField(k, f.FoundryEntryFields[k]); reason != "" {
					warn(fmt.Sprintf("%q ignored: %s", k, reason))
					delete(f.FoundryEntryFields, k)
				}
			}
			if len(f.FoundryEntryFields) == 0 {
				f.FoundryEntryFields = nil
			}
		}
	}
}

// validateEntryField returns why one mapping is unusable, or "" when it is
// fine.
func validateEntryField(key string, d EntryFieldDef) string {
	switch {
	case !ValidChoiceFieldKey(key):
		return "the property is not a plain identifier"
	case !usableEntryPath(d.Path):
		return "path must be a dotted path under system (system.a.b), each part starting with a letter"
	case d.Format != EntryFieldNumber && d.Format != EntryFieldText && d.Format != EntryFieldTiers:
		return "format must be number, text or tiers"
	}
	return ""
}
