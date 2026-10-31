// extension_pack_adapter_integration_test.go creates entity-pack pages through
// the production adapter against MariaDB: the page lands GM-only in the named
// category with its Markdown body rendered and its fields kept.
//
// Skipped under -short. Run with `make test-int-local`.
package app

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/extensions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestExtensionPackPages_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	h := newRoundTripHarness(t)
	ctx := context.Background()
	owner := h.newUser("pack-owner")
	camp, err := h.campaigns.Create(ctx, owner, campaigns.CreateCampaignInput{Name: "Pack"})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	types, err := h.entities.GetEntityTypes(ctx, camp.ID)
	if err != nil || len(types) == 0 {
		t.Fatalf("entity types: %v (%d)", err, len(types))
	}

	pages := extensionPackPages{svc: h.entities}
	typeID, err := pages.EntityTypeIDBySlug(ctx, camp.ID, types[0].Slug)
	if err != nil || typeID != types[0].ID {
		t.Fatalf("EntityTypeIDBySlug = %d, %v; want %d", typeID, err, types[0].ID)
	}
	if _, err := pages.EntityTypeIDBySlug(ctx, camp.ID, "no-such-category"); err == nil {
		t.Error("EntityTypeIDBySlug found a category that does not exist")
	}

	public := false
	tests := []struct {
		name        string
		entry       extensions.PackEntry
		wantPrivate bool
		wantHTML    string
	}{
		{"default is GM only, markdown rendered",
			extensions.PackEntry{Name: "Goblin", Entry: "Small and **mean**.<script>x()</script>", Fields: map[string]any{"cr": "1/4"}},
			true, "<strong>mean</strong>"},
		{"opted into player view, no body",
			extensions.PackEntry{Name: "Owlbear", Private: &public},
			false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := pages.CreatePackEntity(ctx, camp.ID, owner, typeID, tt.entry)
			if err != nil {
				t.Fatalf("CreatePackEntity: %v", err)
			}
			got, err := h.entities.GetByID(ctx, id)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got.IsPrivate != tt.wantPrivate || got.EntityTypeID != typeID || got.Name != tt.entry.Name {
				t.Errorf("page = private %v type %d name %q; want %v %d %q",
					got.IsPrivate, got.EntityTypeID, got.Name, tt.wantPrivate, typeID, tt.entry.Name)
			}
			html := ""
			if got.EntryHTML != nil {
				html = *got.EntryHTML
			}
			if tt.wantHTML == "" && html != "" {
				t.Errorf("entry_html = %q, want none", html)
			}
			if tt.wantHTML != "" {
				if !strings.Contains(html, tt.wantHTML) || strings.Contains(html, "<script") {
					t.Errorf("entry_html = %q, want it to contain %q and no script", html, tt.wantHTML)
				}
				if got.Entry == nil || !strings.Contains(*got.Entry, `"type":"doc"`) {
					t.Errorf("entry = %v, want an editor document", got.Entry)
				}
			}
			for k, v := range tt.entry.Fields {
				if got.FieldsData[k] != v {
					t.Errorf("field %s = %v, want %v", k, got.FieldsData[k], v)
				}
			}
		})
	}
}
