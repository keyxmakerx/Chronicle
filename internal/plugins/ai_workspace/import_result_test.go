// import_result_test.go pins V1-F's Bug 1 fix: the result-screen
// per-row "Open" link MUST use the entity's UUID (`EntityID`), not
// its slug. The web-facing entity show route at
// internal/plugins/entities/routes.go:146 is `/entities/:eid`, which
// `c.Param("eid")` treats as a UUID via the entity handler's Show
// method. V1-E shipped with the slug and the links 404'd — operator
// surfaced it during the post-merge end-to-end smoke (PR #355).
//
// Owner-stability: if anyone refactors the result fragment and
// accidentally re-introduces the slug-keyed link, this test fails
// pinpointed.

package ai_workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
)

// TestImportResult_OpenLink_UsesEntityIDNotSlug renders the result
// fragment with a row that carries distinguishable EntityID + Slug
// values + asserts the rendered href contains the ID, not the slug.
func TestImportResult_OpenLink_UsesEntityIDNotSlug(t *testing.T) {
	data := ImportResultData{
		CampaignID: "camp-uuid-abc",
		Result: importer.CommitResult{
			Created: 1,
			Rows: []importer.RowOutcome{{
				Index:    0,
				Name:     "Tideturn",
				Status:   importer.StatusCreated,
				EntityID: "ent-uuid-12345678",
				Slug:     "tideturn",
			}},
		},
	}

	var sb strings.Builder
	if err := ImportResult(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("ImportResult.Render: %v", err)
	}
	out := sb.String()

	wantHref := "/campaigns/camp-uuid-abc/entities/ent-uuid-12345678"
	if !strings.Contains(out, wantHref) {
		t.Errorf("result fragment is missing the UUID-keyed link %q\noutput:\n%s",
			wantHref, truncateString(out, 2000))
	}

	// Negative assertion — the slug-keyed link must NOT appear (Bug
	// 1 regression guard). We allow the slug to appear in other
	// contexts (e.g. category-creation summary), so we check for
	// the specific "/entities/<slug>" form that the route would
	// 404 on.
	badHref := "/campaigns/camp-uuid-abc/entities/tideturn"
	if strings.Contains(out, badHref) {
		t.Errorf("result fragment leaks the slug-keyed link %q — this is the V1-E Bug 1 regression",
			badHref)
	}
}

// TestImportResult_OpenLink_OmittedWhenNoEntityID — failed-row
// outcomes have no EntityID; the "Open" link must not render
// (would point at /entities/ which 404s).
func TestImportResult_OpenLink_OmittedWhenNoEntityID(t *testing.T) {
	data := ImportResultData{
		CampaignID: "camp-1",
		Result: importer.CommitResult{
			Failed: 1,
			Rows: []importer.RowOutcome{{
				Status: importer.StatusFailed,
				Name:   "doomed",
				Reason: "could not save",
			}},
		},
	}

	var sb strings.Builder
	if err := ImportResult(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("ImportResult.Render: %v", err)
	}
	out := sb.String()

	if strings.Contains(out, "/campaigns/camp-1/entities/") {
		t.Errorf("failed row rendered an Open link; should omit when EntityID is empty\noutput:\n%s",
			truncateString(out, 1500))
	}
}

// TestImportResult_BackButton_LinksToAIWorkspaceTab pins Bug 2's
// footer-button half of fix (c): there must be a hard-navigation
// <a> back to the AI Workspace tab so browser Back lands somewhere
// sensible even when HTMX history hasn't been pushed.
func TestImportResult_BackButton_LinksToAIWorkspaceTab(t *testing.T) {
	data := ImportResultData{
		CampaignID: "camp-9",
		Result: importer.CommitResult{
			Created: 1,
			Rows: []importer.RowOutcome{{
				Name:     "X",
				Status:   importer.StatusCreated,
				EntityID: "ent-1",
			}},
		},
	}

	var sb strings.Builder
	if err := ImportResult(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("ImportResult.Render: %v", err)
	}
	out := sb.String()

	wantBackHref := `href="/campaigns/camp-9/settings?tab=ai-workspace"`
	if !strings.Contains(out, wantBackHref) {
		t.Errorf("result fragment is missing the 'Back to AI Workspace' link %q (V1-F Bug 2 fix)",
			wantBackHref)
	}
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...[truncated]"
}
