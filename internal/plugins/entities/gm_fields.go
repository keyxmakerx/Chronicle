package entities

// gm_fields.go — the shared field-restriction filters (audit M-1, dispatch
// C-FIELDS-GM-FILTER / C-FIELDS-OWNER-FILTER). Entity `fields_data` is served
// to every campaign member; fields a system manifest marks gm_only (e.g. Draw
// Steel's director `gm_notes`) or owner_only (e.g. Draw Steel's `backstory`)
// must be stripped before the JSON reaches a caller who isn't allowed to see
// them. Every egress point — syncapi GetEntity/ListEntities, the
// entities-plugin GetFieldsAPI/PreviewAPI, and CharacterSurfaceSchemaJSON —
// calls FilterRestrictedFields so the paths can't drift. Server is the
// authority; the widgets' client-side "hide the box" is not a fix.

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

// FilterOwnerOnlyFields returns fieldsData with owner-only field VALUES
// removed when the caller can neither see GM content nor owns the entity
// (canSeeGM == false && isOwner == false). Mirrors FilterGMOnlyFields's
// contract exactly: never mutates the input map, and returns it unchanged
// (zero allocation) when there is nothing to strip.
//
// "Owner-only" is declared per field via FieldDefinition.OwnerOnly, populated
// from a system manifest's owner_only annotation the same way gm_only is.
// Unlike GMOnly, an owner-only field's value IS shown to the entity's own
// claimed owner — this is for player-private content (e.g. a character's
// backstory), not a GM-exclusive secret.
func FilterOwnerOnlyFields(fieldsData map[string]any, defs []FieldDefinition, canSeeGM, isOwner bool) map[string]any {
	if canSeeGM || isOwner || len(fieldsData) == 0 || len(defs) == 0 {
		return fieldsData
	}

	var ownerKeys map[string]struct{}
	for i := range defs {
		if defs[i].OwnerOnly {
			if ownerKeys == nil {
				ownerKeys = make(map[string]struct{}, 2)
			}
			ownerKeys[defs[i].Key] = struct{}{}
		}
	}
	if len(ownerKeys) == 0 {
		return fieldsData
	}

	present := false
	for k := range ownerKeys {
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
		if _, isOwnerField := ownerKeys[k]; isOwnerField {
			continue
		}
		out[k] = v
	}
	return out
}

// FilterRestrictedFields composes FilterGMOnlyFields and FilterOwnerOnlyFields
// — the single call every egress path should use, so the two restriction
// tiers can't be applied out of order or have one forgotten at a call site.
func FilterRestrictedFields(fieldsData map[string]any, defs []FieldDefinition, canSeeGM, isOwner bool) map[string]any {
	return FilterOwnerOnlyFields(FilterGMOnlyFields(fieldsData, defs, canSeeGM), defs, canSeeGM, isOwner)
}
