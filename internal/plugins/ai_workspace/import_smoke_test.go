// import_smoke_test.go drives the full handler → parser → classifier
// → review-screen templ render pipeline with a multi-page fixture
// covering every row status: new, name conflict, new category, and
// parse error. Asserts the response HTML contains the expected
// per-row status chips and the Submit button.

package ai_workspace

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubLookup is an in-memory CampaignLookup. Conflicts + types are
// pre-populated to mirror a real campaign mid-import.
type stubLookup struct {
	existing  map[string]*entities.Entity     // slug → entity
	typeBySlug map[string]*entities.EntityType // slug → type
	types     []entities.EntityType
}

func (s *stubLookup) GetBySlug(_ context.Context, _, slug string) (*entities.Entity, error) {
	return s.existing[slug], nil
}

func (s *stubLookup) GetEntityTypeBySlug(_ context.Context, _, slug string) (*entities.EntityType, error) {
	return s.typeBySlug[slug], nil
}

func (s *stubLookup) GetEntityTypes(_ context.Context, _ string) ([]entities.EntityType, error) {
	return s.types, nil
}

// fakeContext stuffs a CampaignContext into echo so the handler's
// GetCampaignContext returns non-nil. Mirrors what the
// RequireCampaignAccess middleware does in production.
func fakeContext(c echo.Context, cc *campaigns.CampaignContext) {
	c.Set("campaign_context", cc)
}

// TestImport_ParseAndReview_MixedStates sends a 5-page fixture
// through the parse endpoint and asserts the rendered review screen
// contains the expected per-row chips.
func TestImport_ParseAndReview_MixedStates(t *testing.T) {
	lookup := &stubLookup{
		existing: map[string]*entities.Entity{
			// Page 3 will conflict with this entity.
			"lyra-vance": {ID: "ent-100", Name: "Lyra Vance"},
		},
		types: []entities.EntityType{
			{ID: 1, Name: "Character", Slug: "character", Enabled: true},
			{ID: 2, Name: "Location", Slug: "location", Enabled: true},
		},
	}
	lookup.typeBySlug = map[string]*entities.EntityType{
		"character": &lookup.types[0],
		"location":  &lookup.types[1],
	}

	h := NewHandler(nil)
	h.SetImportLookup(lookup)

	// Five pages, every state at least once. Each page is one FM
	// block + body, with the closer of page N immediately followed
	// by the opener of page N+1 (matches real AI output format).
	//
	// 1. Full FM, new (character "Maro Halvi") — StatusNew
	// 2. Full FM, new (location "Drowned Reach") — StatusNew
	// 3. Full FM, name conflict (character "Lyra Vance" — already in lookup)
	// 4. Full FM, new category (proposed type "warrior" doesn't exist)
	// 5. Bad visibility — StatusParseError
	fixture := `---
name: Maro Halvi
type: character
visibility: private
tags: [scholar, ally]
---
# Maro Halvi

Cartographer of the Drowned Reach.
---
name: The Drowned Reach
type: location
visibility: public
---
# The Drowned Reach

A region 400 miles wide.
---
name: Lyra Vance
type: character
visibility: private
---
# Lyra Vance

PC, storm sorcerer.
---
name: Ash-Wraith
type: warrior
visibility: dm_only
---
# Ash-Wraith

A revenant.
---
name: Bone Citadel
type: location
visibility: PUBLIC
---
# Bone Citadel

Body.`

	body, contentType := multipartBody(t, "markdown_paste", fixture)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost,
		"/campaigns/camp-1/ai-workspace/import/parse", body)
	req.Header.Set(echo.HeaderContentType, contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	fakeContext(c, &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Ashfall"},
		MemberRole: campaigns.RoleOwner,
	})

	if err := h.ParseImport(c); err != nil {
		t.Fatalf("ParseImport: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body:\n%s", rec.Code, rec.Body.String())
	}

	out := rec.Body.String()

	// The form posts to the commit endpoint and carries the
	// markdown_source hidden field so the commit handler can
	// re-parse without round-tripping the per-page bodies.
	mustContain(t, out, `hx-post="/campaigns/camp-1/ai-workspace/import/commit"`)
	mustContain(t, out, `name="markdown_source"`)
	mustContain(t, out, `type="submit"`)

	// Page count + per-status counters (singular grammar: "1
	// conflict", not "1 conflicts").
	mustContain(t, out, "5 pages detected")
	mustContain(t, out, "1 conflict")
	mustContain(t, out, "1 new category")
	mustContain(t, out, "1 parse error")

	// Per-row chips — each status appears at least once. Match the
	// icon classes (stable across copy edits to the chip text).
	mustContain(t, out, "fa-check mr-1")              // StatusNew chip icon
	mustContain(t, out, "fa-triangle-exclamation")    // StatusConflict chip icon
	mustContain(t, out, "fa-folder-plus")             // StatusNewCategory chip icon
	mustContain(t, out, "fa-circle-xmark")            // StatusParseError chip icon

	// Specific row name inputs — match the `value=` attribute.
	mustContain(t, out, `value="Maro Halvi"`)
	mustContain(t, out, `value="The Drowned Reach"`)
	mustContain(t, out, `value="Lyra Vance"`)
	mustContain(t, out, `value="Ash-Wraith"`)
	mustContain(t, out, `value="Bone Citadel"`)

	// New-category UI surface: Create-new / Map-to-existing radio
	// pair; the proposed slug surfaces in a <code> chip next to the
	// Create radio.
	mustContain(t, out, "Create new:")
	mustContain(t, out, "warrior")
	mustContain(t, out, `value="new"`)      // Create-new radio value
	mustContain(t, out, `value="existing"`) // Map-to-existing radio value

	// Conflict surface — operator sees the existing entity name
	// so they know what they'd be overwriting.
	mustContain(t, out, "Lyra Vance")

	// Parse-error surface (the PUBLIC value should be quoted in the
	// per-row error message).
	mustContain(t, out, "PUBLIC")
}

// TestImport_EmptyBodyReturns400 — the operator hit Parse with
// nothing in the textarea or files. Should be 400 with a hint.
func TestImport_EmptyBodyReturns400(t *testing.T) {
	h := NewHandler(nil)
	h.SetImportLookup(&stubLookup{})

	body, contentType := multipartBody(t, "markdown_paste", "")
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost,
		"/campaigns/camp-1/ai-workspace/import/parse", body)
	req.Header.Set(echo.HeaderContentType, contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	fakeContext(c, &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner,
	})

	err := h.ParseImport(c)
	if err == nil {
		t.Fatal("expected error for empty body, got nil")
	}
	if !strings.Contains(err.Error(), "Nothing to import") {
		t.Errorf("error message = %q; want 'Nothing to import' hint", err.Error())
	}
}

// TestImport_UnwiredLookupGracefulDegrade — handler returns the
// empty review screen rather than crashing when SetImportLookup
// wasn't called.
func TestImport_UnwiredLookupGracefulDegrade(t *testing.T) {
	h := NewHandler(nil)
	// No SetImportLookup call.

	body, contentType := multipartBody(t, "markdown_paste", "# X")
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost,
		"/campaigns/camp-1/ai-workspace/import/parse", body)
	req.Header.Set(echo.HeaderContentType, contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	fakeContext(c, &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner,
	})

	if err := h.ParseImport(c); err != nil {
		t.Fatalf("ParseImport returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// multipartBody builds a small multipart payload with a single
// text field. The integration tests share this helper.
func multipartBody(t *testing.T, field, value string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if value != "" {
		if err := mw.WriteField(field, value); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("response missing %q\nfirst 2000 chars:\n%s", needle, truncate(haystack, 2000))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...[truncated]"
}

func (s *stubLookup) CheckEntityAccess(context.Context, string, int, string) (*entities.EffectivePermission, error) {
	return &entities.EffectivePermission{CanView: true}, nil
}
