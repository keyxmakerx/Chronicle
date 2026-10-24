// import_result_test.go pins that the result screen's per-row "Open"
// link uses the entity's UUID (EntityID), not its slug: the entity
// show route (`/entities/:eid`) is UUID-keyed, so a slug-keyed link
// 404s.

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

	// The slug may appear in other contexts (e.g. category-creation
	// summary); check specifically for the "/entities/<slug>" form
	// that the route would 404 on.
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

// TestImportResult_BackButton_LinksToAIWorkspaceTab pins the
// hard-navigation link back to the AI Workspace tab, so browser Back
// lands somewhere sensible even when HTMX history hasn't been pushed.
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

	wantBackHref := `href="/campaigns/camp-9/ai"`
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
