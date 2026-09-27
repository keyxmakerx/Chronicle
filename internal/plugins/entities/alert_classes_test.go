// alert_classes_test.go pins that the create/edit entity form's error
// banner uses Chronicle's shared alert-error class instead of hand-written
// Tailwind. The hand-written version had no dark-mode colors at all, so on
// a dark screen it rendered as a stark light-pink box; the shared class
// fixes that as a side effect of matching how every other error banner
// (e.g. login's) is already styled.

package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestEntityCreateFormComponent_ErrorBannerUsesSharedAlertClass pins the
// create form's error banner to the shared alert-error class.
func TestEntityCreateFormComponent_ErrorBannerUsesSharedAlertClass(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}}
	entityTypes := []EntityType{{ID: 1, Name: "Character", Enabled: true}}
	component := EntityCreateFormComponent(cc, entityTypes, 1, nil, nil, "csrf", "Name is required.")

	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, `class="alert-error"`) {
		t.Errorf("create form error banner must use the shared alert-error class; got: %s", html)
	}
	if strings.Contains(html, "bg-red-50") {
		t.Errorf("create form error banner must not hand-write its colors; bg-red-50 has no dark-mode pair")
	}
}

// TestEntityEditFormComponent_ErrorBannerUsesSharedAlertClass pins the same
// contract for the edit form.
func TestEntityEditFormComponent_ErrorBannerUsesSharedAlertClass(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}, MemberRole: campaigns.RoleOwner}
	entity := &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Gandalf"}
	entityType := &EntityType{ID: 1, Name: "Character"}
	component := EntityEditFormComponent(cc, entity, entityType, nil, "csrf", "Name is required.")

	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, `class="alert-error"`) {
		t.Errorf("edit form error banner must use the shared alert-error class; got: %s", html)
	}
	if strings.Contains(html, "bg-red-50") {
		t.Errorf("edit form error banner must not hand-write its colors; bg-red-50 has no dark-mode pair")
	}
}
