// Pins that Show, TimelineDataAPI and EmbedTimeline apply the timeline's own
// visibility (dm_only base visibility and per-user visibility_rules) via
// GetTimelineForViewer's timelineVisibleToViewer predicate — the same one
// ListTimelines' filterTimelinesByUser uses, so the two paths cannot drift
// (ADR-058). A viewer who may not see the timeline gets NotFound, never
// Forbidden: on a public campaign, Forbidden would confirm the id exists
// (ADR-055 rule 3).
//
// Every anonymous/Player assertion is paired with an Owner and a co-DM
// control, plus an Owner-view-as-player control, so a fix that hides the
// timeline from everyone or breaks co-DM promotion still fails.
package timeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// secretTimeline is the dm_only fixture every case in this file reads.
func secretTimeline() *Timeline {
	return &Timeline{
		ID:         "tl-secret",
		CampaignID: "camp-1",
		Name:       "The Real Plan Behind The Curtain",
		Visibility: "dm_only",
		Color:      "#112233",
		Icon:       "fa-timeline",
	}
}

// newDMOnlyRepo returns a mock repo serving only secretTimeline(), by id,
// scoped to camp-1; every other call defaults to zero-value nil/empty.
func newDMOnlyRepo() *mockTimelineRepo {
	return &mockTimelineRepo{
		getByIDFn: func(_ context.Context, id string) (*Timeline, error) {
			if id == "tl-secret" {
				return secretTimeline(), nil
			}
			return nil, nil
		},
	}
}

// tlTestReq builds an echo.Context carrying a CampaignContext, an optional
// authenticated user id, and the view-as-player toggle: the cookie
// effectiveRole reads, and the layout flag the templates read when rendering.
func tlTestReq(path string, cc *campaigns.CampaignContext, userID string, viewAsPlayer bool, htmx bool) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	ctx := layouts.SetViewingAsPlayer(req.Context(), viewAsPlayer)
	req = req.WithContext(ctx)
	if viewAsPlayer {
		req.AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "1"})
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "tid")
	c.SetParamValues("camp-1", "tl-secret")
	c.Set("campaign_context", cc)
	if userID != "" {
		c.Set("auth_user_id", userID)
	}
	return c
}

// --- Viewer fixtures ---

func anonCampaignCtx() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Public Campaign", IsPublic: true},
		MemberRole: campaigns.RoleNone,
	}
}

func playerCampaignCtx() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Public Campaign", IsPublic: true},
		MemberRole: campaigns.RolePlayer,
	}
}

func ownerCampaignCtx() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Public Campaign", IsPublic: true},
		MemberRole: campaigns.RoleOwner,
	}
}

func coDMCampaignCtx() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1", Name: "Public Campaign", IsPublic: true},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
	}
}

// --- Show ---

func TestShow_Anonymous_DMOnlyTimeline_NotFound(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/tl-secret", anonCampaignCtx(), "", false, false)

	err := h.Show(c)
	assertAppError(t, err, http.StatusNotFound)
}

func TestShow_PlainPlayer_DMOnlyTimeline_NotFound(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/tl-secret", playerCampaignCtx(), "u-player", false, false)

	err := h.Show(c)
	assertAppError(t, err, http.StatusNotFound)
}

func TestShow_Owner_DMOnlyTimeline_Succeeds(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/tl-secret", ownerCampaignCtx(), "u-owner", false, true)

	if err := h.Show(c); err != nil {
		t.Fatalf("Owner must be able to open a dm_only timeline: %v", err)
	}
	rec := c.Response().Writer.(*httptest.ResponseRecorder)
	if !strings.Contains(rec.Body.String(), secretTimeline().Name) {
		t.Error("Owner's rendered page did not contain the timeline name")
	}
}

func TestShow_CoDM_DMOnlyTimeline_Succeeds(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/tl-secret", coDMCampaignCtx(), "u-codm", false, true)

	if err := h.Show(c); err != nil {
		t.Fatalf("a co-DM must see a dm_only timeline like an Owner does: %v", err)
	}
	rec := c.Response().Writer.(*httptest.ResponseRecorder)
	if !strings.Contains(rec.Body.String(), secretTimeline().Name) {
		t.Error("co-DM's rendered page did not contain the timeline name")
	}
}

// TestShow_OwnerViewAsPlayer_DMOnlyTimeline_NotFound pins that an Owner
// previewing the player experience sees exactly what a Player sees: the
// dm_only timeline does not exist to them, same as
// TestShow_PlainPlayer_DMOnlyTimeline_NotFound.
func TestShow_OwnerViewAsPlayer_DMOnlyTimeline_NotFound(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/tl-secret", ownerCampaignCtx(), "u-owner", true, false)

	err := h.Show(c)
	assertAppError(t, err, http.StatusNotFound)
}

// --- TimelineDataAPI ---

func TestTimelineDataAPI_Anonymous_DMOnlyTimeline_NotFound(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/tl-secret/data", anonCampaignCtx(), "", false, false)

	err := h.TimelineDataAPI(c)
	assertAppError(t, err, http.StatusNotFound)
}

func TestTimelineDataAPI_PlainPlayer_DMOnlyTimeline_NotFound(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/tl-secret/data", playerCampaignCtx(), "u-player", false, false)

	err := h.TimelineDataAPI(c)
	assertAppError(t, err, http.StatusNotFound)
}

func TestTimelineDataAPI_Owner_DMOnlyTimeline_Succeeds(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/tl-secret/data", ownerCampaignCtx(), "u-owner", false, false)

	if err := h.TimelineDataAPI(c); err != nil {
		t.Fatalf("Owner must be able to fetch dm_only timeline data: %v", err)
	}
	rec := c.Response().Writer.(*httptest.ResponseRecorder)
	if !strings.Contains(rec.Body.String(), secretTimeline().Name) {
		t.Error("Owner's JSON response did not contain the timeline name")
	}
}

func TestTimelineDataAPI_CoDM_DMOnlyTimeline_Succeeds(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/tl-secret/data", coDMCampaignCtx(), "u-codm", false, false)

	if err := h.TimelineDataAPI(c); err != nil {
		t.Fatalf("a co-DM must be able to fetch dm_only timeline data: %v", err)
	}
	rec := c.Response().Writer.(*httptest.ResponseRecorder)
	if !strings.Contains(rec.Body.String(), secretTimeline().Name) {
		t.Error("co-DM's JSON response did not contain the timeline name")
	}
}

// --- EmbedTimeline ---
//
// EmbedTimeline always renders something rather than erroring on a
// bad/unauthorized id, so the assertion is on rendered body content: an
// unauthorized viewer must get the empty-state fragment, never
// TimelineEmbedFragment carrying the secret timeline's name.

func TestEmbedTimeline_Anonymous_DMOnlyTimeline_RendersEmpty(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/embed?timeline_id=tl-secret", anonCampaignCtx(), "", false, false)

	if err := h.EmbedTimeline(c); err != nil {
		t.Fatalf("EmbedTimeline must not error, even for a hidden id: %v", err)
	}
	rec := c.Response().Writer.(*httptest.ResponseRecorder)
	body := rec.Body.String()
	if strings.Contains(body, secretTimeline().Name) {
		t.Errorf("anonymous viewer's embed fragment leaked the dm_only timeline's name: %s", body)
	}
}

func TestEmbedTimeline_PlainPlayer_DMOnlyTimeline_RendersEmpty(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/embed?timeline_id=tl-secret", playerCampaignCtx(), "u-player", false, false)

	if err := h.EmbedTimeline(c); err != nil {
		t.Fatalf("EmbedTimeline must not error, even for a hidden id: %v", err)
	}
	rec := c.Response().Writer.(*httptest.ResponseRecorder)
	body := rec.Body.String()
	if strings.Contains(body, secretTimeline().Name) {
		t.Errorf("Player's embed fragment leaked the dm_only timeline's name: %s", body)
	}
}

func TestEmbedTimeline_Owner_DMOnlyTimeline_RendersFragment(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/embed?timeline_id=tl-secret", ownerCampaignCtx(), "u-owner", false, false)

	if err := h.EmbedTimeline(c); err != nil {
		t.Fatalf("EmbedTimeline must not error: %v", err)
	}
	rec := c.Response().Writer.(*httptest.ResponseRecorder)
	body := rec.Body.String()
	if !strings.Contains(body, secretTimeline().Name) {
		t.Errorf("Owner's embed fragment should contain the timeline name, got: %s", body)
	}
}

func TestEmbedTimeline_CoDM_DMOnlyTimeline_RendersFragment(t *testing.T) {
	h := NewHandler(newTestTimelineService(newDMOnlyRepo()))
	c := tlTestReq("/campaigns/camp-1/timelines/embed?timeline_id=tl-secret", coDMCampaignCtx(), "u-codm", false, false)

	if err := h.EmbedTimeline(c); err != nil {
		t.Fatalf("EmbedTimeline must not error: %v", err)
	}
	rec := c.Response().Writer.(*httptest.ResponseRecorder)
	body := rec.Body.String()
	if !strings.Contains(body, secretTimeline().Name) {
		t.Errorf("co-DM's embed fragment should contain the timeline name, got: %s", body)
	}
}

// --- Search ---
//
// SearchTimelines runs the same per-user visibility filter List runs, so a
// timeline with an allow-list-restricting visibility_rules entry is never
// named to a viewer the allow-list excludes, even when its base visibility
// is 'everyone'.

func restrictedByRulesTimeline() Timeline {
	rules := `{"allowed_users":["u-1"]}`
	return Timeline{
		ID:              "tl-restricted",
		CampaignID:      "camp-1",
		Name:            "For u-1's Eyes Only",
		Visibility:      "everyone",
		VisibilityRules: &rules,
		Icon:            "fa-timeline",
		Color:           "#445566",
	}
}

// TestSearchTimelines_AnonymousViewer_DoesNotNameRestrictedTimeline pins
// that SearchTimelines takes a userID and builds the same permissions.Viewer
// ListTimelines does: an anonymous viewer must never see this timeline's name.
func TestSearchTimelines_AnonymousViewer_DoesNotNameRestrictedTimeline(t *testing.T) {
	repo := &mockTimelineRepo{
		searchFn: func(_ context.Context, _ string, _ string, _ int) ([]Timeline, error) {
			return []Timeline{restrictedByRulesTimeline()}, nil
		},
	}
	svc := newTestTimelineService(repo)

	results, err := svc.SearchTimelines(context.Background(), "camp-1", "eyes", int(permissions.RoleNone), "")
	if err != nil {
		t.Fatalf("SearchTimelines: %v", err)
	}
	for _, r := range results {
		if r["name"] == restrictedByRulesTimeline().Name {
			t.Errorf("anonymous search results named a visibility_rules-restricted timeline: %+v", results)
		}
	}
}

// TestSearchTimelines_AllowedUser_StillSeesTheirRestrictedTimeline is the
// control proving the fix is a per-user filter, not a blanket hide: the
// user the allow-list actually names must still find their own timeline
// by name.
func TestSearchTimelines_AllowedUser_StillSeesTheirRestrictedTimeline(t *testing.T) {
	repo := &mockTimelineRepo{
		searchFn: func(_ context.Context, _ string, _ string, _ int) ([]Timeline, error) {
			return []Timeline{restrictedByRulesTimeline()}, nil
		},
	}
	svc := newTestTimelineService(repo)

	results, err := svc.SearchTimelines(context.Background(), "camp-1", "eyes", int(permissions.RolePlayer), "u-1")
	if err != nil {
		t.Fatalf("SearchTimelines: %v", err)
	}
	if len(results) != 1 || results[0]["name"] != restrictedByRulesTimeline().Name {
		t.Errorf("the allow-listed user must still see their own restricted timeline in search, got: %+v", results)
	}
}
