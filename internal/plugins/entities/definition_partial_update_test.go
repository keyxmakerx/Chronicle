// definition_partial_update_test.go pins the partial-update contract for the
// owner-edited definitions in this package (entity types, layout presets,
// content templates, worldbuilding prompts). Each table decodes a real JSON
// body, because the absent/null distinction only exists at the bind layer.
package entities

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUpdateEntityType_PartialUpdate(t *testing.T) {
	stored := EntityType{
		ID: 1, CampaignID: "camp-1", Name: "Location", NamePlural: "Places",
		Icon: "fa-map", Color: "#112233", Slug: "location",
	}
	tests := []struct {
		name       string
		body       string
		wantName   string
		wantPlural string
		wantIcon   string
		wantColor  string
	}{
		{"empty body changes nothing", `{}`, "Location", "Places", "fa-map", "#112233"},
		{"rename keeps plural, icon and color", `{"name":"Site"}`, "Site", "Places", "fa-map", "#112233"},
		{"color only", `{"color":"#ffffff"}`, "Location", "Places", "fa-map", "#ffffff"},
		{"icon only", `{"icon":"fa-dragon"}`, "Location", "Places", "fa-dragon", "#112233"},
		{"explicit null on NOT NULL columns preserves", `{"name":null,"name_plural":null,"icon":null,"color":null}`, "Location", "Places", "fa-map", "#112233"},
		{"empty plural is a value and auto-pluralizes", `{"name_plural":""}`, "Location", "Locations", "fa-map", "#112233"},
		{"empty icon is a value and resets to the default", `{"icon":""}`, "Location", "Places", "fa-circle", "#112233"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured *EntityType
			typeRepo := &mockEntityTypeRepo{
				findByIDFn: func(_ context.Context, _ int) (*EntityType, error) {
					cp := stored
					return &cp, nil
				},
				updateFn: func(_ context.Context, et *EntityType) error { captured = et; return nil },
			}
			// The handler binds the wire struct and converts, so do the same.
			var req UpdateEntityTypeRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			in := UpdateEntityTypeInput(req)
			svc := newTestService(&mockEntityRepo{}, typeRepo)
			if _, err := svc.UpdateEntityType(context.Background(), 1, in); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if captured.Name != tt.wantName || captured.NamePlural != tt.wantPlural ||
				captured.Icon != tt.wantIcon || captured.Color != tt.wantColor {
				t.Errorf("wrote %q/%q/%q/%q, want %q/%q/%q/%q",
					captured.Name, captured.NamePlural, captured.Icon, captured.Color,
					tt.wantName, tt.wantPlural, tt.wantIcon, tt.wantColor)
			}
		})
	}
}

func TestUpdateLayoutPreset_PartialUpdate(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantName string
		wantDesc string
		wantIcon string
	}{
		{"empty body changes nothing", `{}`, "Standard", "old description", "fa-map"},
		{"rename keeps layout, description and icon", `{"name":"Renamed"}`, "Renamed", "old description", "fa-map"},
		{"description only", `{"description":"new"}`, "Standard", "new", "fa-map"},
		{"explicit null preserves", `{"name":null,"description":null,"icon":null,"layout_json":null}`, "Standard", "old description", "fa-map"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured *LayoutPreset
			repo := &mockLayoutPresetRepo{
				findByIDFn: func(_ context.Context, id int) (*LayoutPreset, error) {
					return &LayoutPreset{ID: id, Name: "Standard", Description: "old description", LayoutJSON: validLayoutJSON, Icon: "fa-map"}, nil
				},
				updateFn: func(_ context.Context, p *LayoutPreset) error { captured = p; return nil },
			}
			var in UpdateLayoutPresetInput
			if err := json.Unmarshal([]byte(tt.body), &in); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			if _, err := NewLayoutPresetService(repo).Update(context.Background(), 1, in); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if captured.Name != tt.wantName || captured.Description != tt.wantDesc || captured.Icon != tt.wantIcon {
				t.Errorf("wrote %q/%q/%q, want %q/%q/%q", captured.Name, captured.Description, captured.Icon, tt.wantName, tt.wantDesc, tt.wantIcon)
			}
			if captured.LayoutJSON != validLayoutJSON {
				t.Error("layout JSON must survive a body that does not name it")
			}
		})
	}
}

func TestUpdateContentTemplate_PartialUpdate(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantName string
		wantDesc string
		wantHTML string
		wantIcon string
	}{
		{"empty body changes nothing", `{}`, "Recap", "old description", "<p>old</p>", "fa-scroll"},
		{"rename keeps description and preview HTML", `{"name":"Renamed"}`, "Renamed", "old description", "<p>old</p>", "fa-scroll"},
		{"preview HTML only", `{"content_html":"<p>new</p>"}`, "Recap", "old description", "<p>new</p>", "fa-scroll"},
		{"explicit null preserves", `{"name":null,"description":null,"content_html":null,"icon":null}`, "Recap", "old description", "<p>old</p>", "fa-scroll"},
		{"empty description is a value", `{"description":""}`, "Recap", "", "<p>old</p>", "fa-scroll"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured *ContentTemplate
			repo := &mockContentTemplateRepo{
				findByIDFn: func(_ context.Context, id int) (*ContentTemplate, error) {
					return &ContentTemplate{ID: id, Name: "Recap", Description: "old description", ContentJSON: `{"type":"doc"}`, ContentHTML: "<p>old</p>", Icon: "fa-scroll"}, nil
				},
				updateFn: func(_ context.Context, ct *ContentTemplate) error { captured = ct; return nil },
			}
			var in UpdateContentTemplateInput
			if err := json.Unmarshal([]byte(tt.body), &in); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			if _, err := NewContentTemplateService(repo, &mockEntityTypeRepo{}).Update(context.Background(), 1, in); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if captured.Name != tt.wantName || captured.Description != tt.wantDesc ||
				captured.ContentHTML != tt.wantHTML || captured.Icon != tt.wantIcon {
				t.Errorf("wrote %q/%q/%q/%q, want %q/%q/%q/%q", captured.Name, captured.Description, captured.ContentHTML, captured.Icon,
					tt.wantName, tt.wantDesc, tt.wantHTML, tt.wantIcon)
			}
			if captured.ContentJSON != `{"type":"doc"}` {
				t.Error("content JSON must survive a body that does not name it")
			}
		})
	}
}

func TestUpdatePrompt_PartialUpdate(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantName string
		wantText string
		wantIcon string
	}{
		{"empty body changes nothing", `{}`, "Original", "original text", "fa-heart"},
		{"rename keeps text and icon", `{"name":"Renamed"}`, "Renamed", "original text", "fa-heart"},
		{"text only", `{"prompt_text":"new text"}`, "Original", "new text", "fa-heart"},
		{"explicit null preserves", `{"name":null,"prompt_text":null,"icon":null}`, "Original", "original text", "fa-heart"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewWorldbuildingPromptService(newMockWBPromptRepo(), &mockEntityTypeListerForPrompts{})
			p, err := svc.Create(context.Background(), "camp-1", CreatePromptInput{
				Name: "Original", PromptText: "original text", Icon: "fa-heart",
			})
			if err != nil {
				t.Fatalf("seed: %v", err)
			}
			var in UpdatePromptInput
			if err := json.Unmarshal([]byte(tt.body), &in); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			if err := svc.Update(context.Background(), p.ID, in); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, _ := svc.GetByID(context.Background(), p.ID)
			if got.Name != tt.wantName || got.PromptText != tt.wantText || got.Icon != tt.wantIcon {
				t.Errorf("stored %q/%q/%q, want %q/%q/%q", got.Name, got.PromptText, got.Icon, tt.wantName, tt.wantText, tt.wantIcon)
			}
		})
	}
}
