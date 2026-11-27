package systems

import (
	"fmt"
	"log/slog"
	"sort"
)

// Entry effect change types, as Foundry's Active Effect changes name them.
const (
	EntryEffectOverride = "override"
	EntryEffectAdd      = "add"
	EntryEffectUpgrade  = "upgrade"
)

// EntryEffectSize reads a size such as "1M" or "2": the number goes to
// path.value and a size-1 letter to path.letter.
const EntryEffectSize = "size"

// EntryEffectDef is how one entry property changes the hero who has the item:
// an Active Effect change on a path under the actor's system data. The
// Foundry module puts the changes on one effect carried by the item sync
// makes, so taking the item off takes the change with it.
type EntryEffectDef struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Format string `json:"format"`
}

// sanitizeEntryEffects drops foundry_entry_effects mappings that can't be
// used, recording a warning for each, the same way and on the same fields as
// sanitizeEntryFields.
func (m *SystemManifest) sanitizeEntryEffects() {
	for i := range m.EntityPresets {
		preset := &m.EntityPresets[i]
		for j := range preset.Fields {
			f := &preset.Fields[j]
			if len(f.FoundryEntryEffects) == 0 {
				continue
			}
			warn := func(reason string) {
				msg := fmt.Sprintf("Preset %q field %q: foundry_entry_effects %s", preset.Slug, f.Key, reason)
				m.EntryFieldWarnings = append(m.EntryFieldWarnings, msg)
				slog.Warn("system manifest: entry effect mapping dropped",
					slog.String("system", m.ID),
					slog.String("preset", preset.Slug),
					slog.String("field", f.Key),
					slog.String("reason", reason))
			}
			if reason := entryMappingRefused(f, len(f.FoundryEntryEffects)); reason != "" {
				warn(reason)
				f.FoundryEntryEffects = nil
				continue
			}
			keys := make([]string, 0, len(f.FoundryEntryEffects))
			for k := range f.FoundryEntryEffects {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if reason := validateEntryEffect(k, f.FoundryEntryEffects[k]); reason != "" {
					warn(fmt.Sprintf("%q ignored: %s", k, reason))
					delete(f.FoundryEntryEffects, k)
				}
			}
			if len(f.FoundryEntryEffects) == 0 {
				f.FoundryEntryEffects = nil
			}
		}
	}
}

// validateEntryEffect returns why one effect mapping is unusable, or "" when
// it is fine. A size only makes sense as an override: it replaces the hero's
// size rather than adding to it.
func validateEntryEffect(key string, d EntryEffectDef) string {
	switch {
	case !ValidChoiceFieldKey(key):
		return "the property is not a plain identifier"
	case !usableEntryPath(d.Path):
		return "path must be a dotted path under system (system.a.b), each part starting with a letter"
	case d.Type != EntryEffectOverride && d.Type != EntryEffectAdd && d.Type != EntryEffectUpgrade:
		return "type must be override, add or upgrade"
	case d.Format != EntryFieldNumber && d.Format != EntryEffectSize:
		return "format must be number or size"
	case d.Format == EntryEffectSize && d.Type != EntryEffectOverride:
		return "a size can only be an override"
	}
	return ""
}
