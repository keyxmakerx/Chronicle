package entities

import (
	"encoding/json"
	"fmt"
)

// character_surface.go builds the dynamic-surface mount schema for the
// `character_surface` layout block — the "big widget" character sheet. The
// server seeds the entity's real data; the client frame (Chronicle.surface,
// dynamic_surface.js) mounts it and the generic box renderers in
// static/js/widgets/character_surface.js paint the bodies from the seed.
//
// The description box does NOT inline EntryHTML (that would bypass the
// role-aware secret handling players rely on). Instead it carries the same
// editor-widget wiring the standard `entry` block uses, so secrets/edit-gating
// stay identical.

// characterSurfaceField is one attribute value on the sheet.
type characterSurfaceField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// characterSurfaceSection groups attributes by their field section.
type characterSurfaceSection struct {
	Title  string                  `json:"title"`
	Fields []characterSurfaceField `json:"fields"`
}

// characterSurfaceEditor carries the wiring the client needs to mount the
// shared `editor` widget for the description box (role-aware, autosaving).
type characterSurfaceEditor struct {
	Endpoint   string `json:"endpoint"`
	CampaignID string `json:"campaignId"`
	CSRF       string `json:"csrf"`
	CanEdit    bool   `json:"canEdit"`
}

// characterSurfaceSeed is the entity payload the client box renderers read.
type characterSurfaceSeed struct {
	Name      string                    `json:"name"`
	Image     string                    `json:"image,omitempty"`
	TypeName  string                    `json:"typeName,omitempty"`
	TypeLabel string                    `json:"typeLabel,omitempty"`
	Sections  []characterSurfaceSection `json:"sections"`
	Editor    *characterSurfaceEditor   `json:"editor,omitempty"`
}

// CharacterSurfaceSchemaJSON builds the dynamic-surface mount schema (with the
// entity's real data seeded) that the character_surface block embeds. Returns
// "{}" on marshal failure so the block degrades to an empty surface rather than
// breaking the page. json.Marshal HTML-escapes the output, so it is safe to
// embed inside a <script type="application/json"> tag.
//
// canSeeGM gates the GM-only field filter: when false (a player viewing the
// sheet), fields the system marks gm_only are omitted from the seed so their
// values never reach the browser (audit M-1). Server is the authority.
func CharacterSurfaceSchemaJSON(entity *Entity, entityType *EntityType, campaignID, csrfToken string, canEdit, canSeeGM bool) string {
	if entity == nil {
		return "{}"
	}

	seed := characterSurfaceSeed{
		Name:     entity.Name,
		TypeName: entity.TypeName,
		Sections: []characterSurfaceSection{},
		Editor: &characterSurfaceEditor{
			Endpoint:   fmt.Sprintf("/campaigns/%s/entities/%s/entry", campaignID, entity.ID),
			CampaignID: campaignID,
			CSRF:       csrfToken,
			CanEdit:    canEdit,
		},
	}
	if entity.ImagePath != nil && *entity.ImagePath != "" {
		seed.Image = "/media/" + *entity.ImagePath
	}
	if entity.TypeLabel != nil {
		seed.TypeLabel = *entity.TypeLabel
	}

	// Group the entity's non-empty custom fields by section, preserving the
	// first-seen section order so the sheet reads like the type designed it.
	if entityType != nil {
		fields := MergeFields(entityType.Fields, entity.FieldOverrides)
		// Strip GM-only field values for non-GM viewers (audit M-1). Gate on
		// the full type schema so a "hidden" override can't leave a value un-
		// stripped; non-mutating, so other blocks rendering this entity see
		// the original data.
		fieldsData := FilterGMOnlyFields(entity.FieldsData, entityType.Fields, canSeeGM)
		order := []string{}
		bySection := map[string]*characterSurfaceSection{}
		for _, f := range fields {
			raw, ok := fieldsData[f.Key]
			if !ok || raw == nil {
				continue
			}
			val := fmt.Sprintf("%v", raw)
			if val == "" {
				continue
			}
			s, exists := bySection[f.Section]
			if !exists {
				order = append(order, f.Section)
				s = &characterSurfaceSection{Title: f.Section, Fields: []characterSurfaceField{}}
				bySection[f.Section] = s
			}
			s.Fields = append(s.Fields, characterSurfaceField{Label: f.Label, Value: val})
		}
		for _, sec := range order {
			seed.Sections = append(seed.Sections, *bySection[sec])
		}
	}

	schema := map[string]any{
		"provider": map[string]any{
			"key":  "entity:" + entity.ID,
			"seed": seed,
		},
		"rows": []any{
			map[string]any{
				"columns": []any{
					map[string]any{
						"width": 12,
						"boxes": []any{
							map[string]any{"id": "char-header:" + entity.ID, "title": entity.Name, "block": "char-header", "expand": "expanded"},
							map[string]any{"id": "char-details:" + entity.ID, "title": "Details", "block": "char-attributes", "expand": "expanded"},
							map[string]any{"id": "char-entry:" + entity.ID, "title": "Description", "block": "char-entry", "expand": "expanded"},
						},
					},
				},
			},
		},
	}

	b, err := json.Marshal(schema)
	if err != nil {
		return "{}"
	}
	return string(b)
}
