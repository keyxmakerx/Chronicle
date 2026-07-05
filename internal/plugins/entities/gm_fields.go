package entities

// gm_fields.go — the single shared GM-only field filter (audit M-1,
// dispatch C-FIELDS-GM-FILTER). Entity `fields_data` is served to every
// campaign member; fields a system manifest marks gm_only (e.g. Draw
// Steel's director `gm_notes`) must be stripped before the JSON reaches a
// non-GM (player / public) caller. Both egress points — syncapi
// GetEntity/ListEntities and the entities-plugin GetFieldsAPI — call this
// one helper so the two paths can't drift. Server is the authority; the
// widgets' client-side "hide the GM box" is not a fix.

// FilterGMOnlyFields returns fieldsData with GM-only field VALUES removed
// when the caller may not see GM content (canSeeGM == false). GM/owner
// sessions and Foundry Bearer callers (canSeeGM == true) get the map back
// unchanged.
//
// It never mutates the input map: when something must be stripped it
// returns a fresh copy, so the caller's scanned model / DB-backed map is
// left intact (matching the egress-sanitize convention). When the caller
// is a GM, there are no gm_only defs, or no gm_only key is actually present
// in the data, the original map is returned as-is (zero allocation).
//
// "GM-only" is declared per field via FieldDefinition.GMOnly, populated
// from a system manifest's gm_only annotation through preset application
// and EnsureFieldMetadataFromManifests — core stays system-agnostic and
// strips whatever the installed manifest marked.
func FilterGMOnlyFields(fieldsData map[string]any, defs []FieldDefinition, canSeeGM bool) map[string]any {
	if canSeeGM || len(fieldsData) == 0 || len(defs) == 0 {
		return fieldsData
	}

	// Which declared field keys are GM-only?
	var gmKeys map[string]struct{}
	for i := range defs {
		if defs[i].GMOnly {
			if gmKeys == nil {
				gmKeys = make(map[string]struct{}, 2)
			}
			gmKeys[defs[i].Key] = struct{}{}
		}
	}
	if len(gmKeys) == 0 {
		return fieldsData
	}

	// Fast path: nothing to strip if none of the gm_only keys are present.
	present := false
	for k := range gmKeys {
		if _, ok := fieldsData[k]; ok {
			present = true
			break
		}
	}
	if !present {
		return fieldsData
	}

	out := make(map[string]any, len(fieldsData))
	for k, v := range fieldsData {
		if _, isGM := gmKeys[k]; isGM {
			continue
		}
		out[k] = v
	}
	return out
}
