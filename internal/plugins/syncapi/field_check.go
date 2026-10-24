package syncapi

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// The entity service saves any fields map it is given, so the API checks a
// write against the page's category before it is stored: a key nobody
// declared would sit on the sheet unseen, and GM-only hiding reads the
// category's field list, so an undeclared key is never treated as
// Director-only. The check is narrow on purpose so sync clients keep working:
//
//   - a value sent back unchanged passes, so a client that resends the whole
//     stored set (Foundry does) is never refused for what was already there;
//   - a key a game system's sheet declares passes, since Foundry writes the
//     system's fields to whichever category it was pointed at;
//   - only number fields have their values checked, the one type every
//     client sends the same way;
//   - null clears a key and always passes.

// checkFieldPatch lists what is wrong with patch, as one plain sentence per
// key; empty when it may be saved.
func checkFieldPatch(defs []entities.FieldDefinition, systemKeys map[string]bool, stored, patch map[string]any) []string {
	byKey := make(map[string]entities.FieldDefinition, len(defs))
	for _, d := range defs {
		byKey[d.Key] = d
	}
	keys := make([]string, 0, len(patch))
	for k := range patch {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var problems []string
	for _, k := range keys {
		v := patch[k]
		if v == nil {
			continue
		}
		if old, ok := stored[k]; ok && reflect.DeepEqual(old, v) {
			continue
		}
		def, declared := byKey[k]
		if !declared {
			if !systemKeys[k] {
				problems = append(problems, fmt.Sprintf("%q is not one of this category's fields", k))
			}
			continue
		}
		if def.Type == "number" && !isNumberValue(v) {
			problems = append(problems, fmt.Sprintf("%q needs a number", k))
		}
	}
	return problems
}

// isNumberValue is a JSON number, a string holding one, or "" (an emptied
// number box).
func isNumberValue(v any) bool {
	switch t := v.(type) {
	case float64:
		return !math.IsNaN(t) && !math.IsInf(t, 0)
	case int, int64:
		return true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return true
		}
		f, err := strconv.ParseFloat(s, 64)
		return err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	}
	return false
}

// fieldPatchError is the 400 for a refused patch, naming every bad key.
func fieldPatchError(problems []string) error {
	return apperror.NewBadRequest("fields_data was not saved: " + strings.Join(problems, "; ") +
		". GET /entity-types/{id} lists the category's fields.")
}

// systemFieldKeys is every field key a game system's sheet presets declare:
// the built-in systems and the campaign's own uploaded one.
func (h *APIHandler) systemFieldKeys(campaignID string) map[string]bool {
	keys := map[string]bool{}
	add := func(m *systems.SystemManifest) {
		if m == nil {
			return
		}
		for _, p := range m.EntityPresets {
			for _, f := range p.Fields {
				keys[f.Key] = true
			}
		}
	}
	for _, m := range systems.Registry() {
		add(m)
	}
	if h.campaignSystemLister != nil {
		add(h.campaignSystemLister.GetManifest(campaignID))
	}
	return keys
}

// checkFields refuses a fields write to a page of the given category that
// would store an undeclared key or a non-number in a number field. stored is
// the page's current fields (nil on create). A category that can't be read
// is left to the service, which refuses it on its own.
func (h *APIHandler) checkFields(ctx context.Context, campaignID string, typeID int, stored, patch map[string]any) error {
	if len(patch) == 0 {
		return nil
	}
	et, err := h.entitySvc.GetEntityTypeByID(ctx, typeID)
	if err != nil || et == nil || et.CampaignID != campaignID {
		return nil
	}
	if problems := checkFieldPatch(et.Fields, h.systemFieldKeys(campaignID), stored, patch); len(problems) > 0 {
		return fieldPatchError(problems)
	}
	return nil
}
