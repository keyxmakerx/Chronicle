package systems

import (
	"encoding/json"
	"strings"
	"testing"
)

func panelManifest(panels ...EntityPanelDef) SystemManifest {
	return SystemManifest{
		ID:         "test-system",
		Name:       "Test",
		APIVersion: "1",
		Version:    "1.0.0",
		Widgets: []WidgetDef{
			{Slug: "negotiation", Name: "Negotiation", ScriptFile: "widgets/negotiation.js"},
		},
		EntityPanels: panels,
	}
}

func TestValidateManifest_EntityPanels(t *testing.T) {
	many := make([]EntityPanelDef, maxEntityPanels+1)
	for i := range many {
		many[i] = EntityPanelDef{Widget: "negotiation", AppliesTo: "npc"}
	}
	tests := []struct {
		name    string
		panels  []EntityPanelDef
		wantErr string // substring; empty means valid
	}{
		{"none", nil, ""},
		{"valid", []EntityPanelDef{{Widget: "negotiation", AppliesTo: "npc"}}, ""},
		{"unknown widget", []EntityPanelDef{{Widget: "nope", AppliesTo: "npc"}}, "must match a widgets[].slug"},
		{"empty widget", []EntityPanelDef{{AppliesTo: "npc"}}, "widget is required"},
		{"bad widget chars", []EntityPanelDef{{Widget: "Negotiation!", AppliesTo: "npc"}}, "lowercase"},
		{"typo applies_to", []EntityPanelDef{{Widget: "negotiation", AppliesTo: "npcs"}}, "not supported"},
		{"missing applies_to", []EntityPanelDef{{Widget: "negotiation"}}, "not supported"},
		{"other audience", []EntityPanelDef{{Widget: "negotiation", AppliesTo: "location"}}, "not supported"},
		{"too many", many, "too many entity panels"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := panelManifest(tt.panels...)
			err := ValidateManifest(&m)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// The manifest key is entity_panels with widget/applies_to members.
func TestEntityPanels_JSONKeys(t *testing.T) {
	var m SystemManifest
	raw := `{"entity_panels":[{"widget":"negotiation","applies_to":"npc"}]}`
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.EntityPanels) != 1 || m.EntityPanels[0].Widget != "negotiation" || m.EntityPanels[0].AppliesTo != EntityPanelAppliesNPC {
		t.Fatalf("decoded %+v", m.EntityPanels)
	}
}

// A widget the host mounts as a panel or renderer is not offered in the layout
// palette, so it cannot be placed a second time without its context.
func TestPlaceableWidgetMetas_ExcludesHostMounted(t *testing.T) {
	m := &SystemManifest{
		Widgets: []WidgetDef{
			{Slug: "free", Name: "Free"},
			{Slug: "panel", Name: "Panel"},
			{Slug: "page", Name: "Page"},
		},
		Renderers:    []RendererDef{{Slug: "x", Widget: "page"}},
		EntityPanels: []EntityPanelDef{{Widget: "panel", AppliesTo: EntityPanelAppliesNPC}},
	}
	got := placeableWidgetMetas(m)
	if len(got) != 1 || got[0].WidgetSlug != "free" {
		t.Fatalf("placeable = %+v, want only the free widget", got)
	}
}
