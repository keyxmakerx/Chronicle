// calendar_v2_actions_test.go — the event drawer Action set (C-CAL-EDITOR-
// EXPANSION PR1). The Actions section renders inside the (Scribe-gated) drawer,
// ships HIDDEN by default (event_grid.js reveals it only in EDIT mode — the
// view-mode gate), and surfaces create-entity (only when the campaign has entity
// types), the existing ties picker as "Linked entities", duplicate, permalink,
// and the (relocated) delete. readRepoFile lives in calendar_v2_daypeek_test.go.
package calendar

import (
	"context"
	"strings"
	"testing"
)

func renderEventDrawerActions(t *testing.T, types []EntityTypeRef) string {
	t.Helper()
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{ID: "cal-1", Name: "Harptos", Months: []Month{{Name: "Hammer", Days: 30}}},
		CampaignID:     "camp-1",
		IsScribe:       true,
		EntityTypes:    types,
	}
	var sb strings.Builder
	if err := eventV2Drawer(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render drawer: %v", err)
	}
	return sb.String()
}

// TestEventDrawerActions_ScribeWithTypes: all five actions render, the section is
// hidden by default (edit-mode reveal), and the create-entity picker lists the
// campaign's entity types.
func TestEventDrawerActions_ScribeWithTypes(t *testing.T) {
	html := renderEventDrawerActions(t, []EntityTypeRef{{ID: 5, Name: "NPC", Slug: "npc"}})
	for _, want := range []string{
		`data-drawer-actions class="hidden`, // view-mode gate: hidden until edit mode
		"data-action-create-entity",         // 1. create entity from event
		`value="5"`, "NPC",                  // the entity-type option in the picker
		"data-event-ties-section",                                    // ties picker (now in the Details section)
		"Linked entities",                                            // the relabeled ties group
		"data-action-duplicate", "data-dup-day", "data-duplicate-go", // duplicate to date
		"data-action-permalink", // permalink
		"data-drawer-delete",    // delete (now in the footer, C-CAL-LARGE-EDITOR)
	} {
		if !strings.Contains(html, want) {
			t.Errorf("scribe drawer Actions section missing %q", want)
		}
	}
}

// TestEventDrawerActions_NoEntityTypesHidesCreate: with no entity types the
// create-entity action is omitted; the other actions remain.
func TestEventDrawerActions_NoEntityTypesHidesCreate(t *testing.T) {
	html := renderEventDrawerActions(t, nil)
	if strings.Contains(html, "data-action-create-entity") {
		t.Errorf("create-entity action must be omitted when the campaign has no entity types")
	}
	for _, want := range []string{"data-action-duplicate", "data-action-permalink", "data-drawer-delete", "data-event-ties-section"} {
		if !strings.Contains(html, want) {
			t.Errorf("Actions section still missing %q", want)
		}
	}
}

// TestEventDrawer_FooterOrder: the redesigned drawer (C-CAL-LARGE-EDITOR, signed
// mockup) puts Delete… back in the FOOTER — order is Delete… (left) · Cancel ·
// Save changes (right). Supersedes the pre-redesign "delete lives in Actions"
// contract; the actions section keeps only the GM power-tools.
func TestEventDrawer_FooterOrder(t *testing.T) {
	html := renderEventDrawerActions(t, nil)
	saveIdx := strings.LastIndex(html, "data-drawer-save")
	deleteIdx := strings.LastIndex(html, "data-drawer-delete")
	if saveIdx < 0 || deleteIdx < 0 {
		t.Fatal("footer must carry both delete and save")
	}
	if deleteIdx > saveIdx {
		t.Errorf("footer order must be Delete… before Save changes (mockup)")
	}
}

// TestEventDrawer_ScribeGatedAtCallSite: the whole drawer (hence the Actions
// section) is server-gated to Scribe+ at the page call site — players never get
// the markup.
func TestEventDrawer_ScribeGatedAtCallSite(t *testing.T) {
	src := readRepoFile(t, "internal/plugins/calendar/calendar_v2.templ")
	if !strings.Contains(src, "&& data.IsScribe {") || !strings.Contains(src, "@eventV2Drawer(data)") {
		t.Errorf("eventV2Drawer must be call-site gated on data.IsScribe")
	}
}
