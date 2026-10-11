package entities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type stubTimelineSearcher struct{ called bool }

func (s *stubTimelineSearcher) SearchTimelines(_ context.Context, _, _ string, _ int, _ string) ([]map[string]string, error) {
	s.called = true
	return []map[string]string{{"id": "tl-1", "name": "Timeline hit"}}, nil
}

func searchJSON(t *testing.T, h *Handler, query string) (map[string]any, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/search?"+query, nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe})
	if err := h.SearchAPI(c); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out, rec.Code
}

// TestSearchAPI_PagesOnlyForThePicker: the place picker asks for pages=1 and
// must get entity pages only; without it the quick search still mixes in
// timelines, maps, events and the rest.
func TestSearchAPI_PagesOnlyForThePicker(t *testing.T) {
	repo := &mockEntityRepo{
		searchFn: func(context.Context, string, string, []int, int, string, ListOptions) ([]Entity, int, error) {
			return []Entity{{ID: "e1", Name: "Guild", CampaignID: "camp-1"}}, 1, nil
		},
	}
	tl := &stubTimelineSearcher{}
	h := NewHandler(NewEntityService(repo, &mockEntityTypeRepo{}, &mockPermissionRepo{}))
	h.SetTimelineSearcher(tl)

	out, _ := searchJSON(t, h, "q=guild&pages=1")
	if tl.called {
		t.Error("a pages-only search must not ask the other searchers")
	}
	if got := len(out["results"].([]any)); got != 1 {
		t.Errorf("pages-only results = %d, want 1", got)
	}

	out, _ = searchJSON(t, h, "q=guild")
	if !tl.called || len(out["results"].([]any)) != 2 {
		t.Errorf("the plain quick search should still include the timeline hit: %v", out["results"])
	}
}

// TestRemovePlaceAPI_ParentMustBeVisible: removing a place answers "not found"
// when the viewer can't see the parent, exactly like adding one.
func TestRemovePlaceAPI_ParentMustBeVisible(t *testing.T) {
	pages := map[string]*Entity{
		"e1":     {ID: "e1", CampaignID: "camp-1", Name: "Cook"},
		"hidden": {ID: "hidden", CampaignID: "camp-1", Name: "Vault", IsPrivate: true},
	}
	repo := &mockEntityRepo{findByIDFn: func(_ context.Context, id string) (*Entity, error) {
		if p, ok := pages[id]; ok {
			c := *p
			return &c, nil
		}
		return nil, apperror.NewNotFound("entity not found")
	}}
	// The hidden parent uses custom visibility and the permission repo grants
	// this viewer nothing on it; every other page is viewable.
	perms := &mockPermissionRepo{getEffectivePermFn: func(_ context.Context, id string, _ int, _ string) (*EffectivePermission, error) {
		if id == "hidden" {
			return &EffectivePermission{}, nil
		}
		return &EffectivePermission{CanView: true, CanEdit: true}, nil
	}}
	h := NewHandler(NewEntityService(repo, &mockEntityTypeRepo{}, perms))
	h.SetPlaceService(NewPlaceService(repo, &fakePlaces{pages: map[string]*Entity{}}))

	pages["hidden"].Visibility = VisibilityCustom
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id", "eid", "pid")
	c.SetParamValues("camp-1", "e1", "hidden")
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe})
	err := h.RemovePlaceAPI(c)
	var ae *apperror.AppError
	if err == nil || !asAppErr(err, &ae) || ae.Code != http.StatusNotFound || !strings.Contains(ae.Message, "not found") {
		t.Fatalf("RemovePlaceAPI error = %v, want 404 not found", err)
	}
}

func asAppErr(err error, out **apperror.AppError) bool {
	ae, ok := err.(*apperror.AppError)
	if ok {
		*out = ae
	}
	return ok
}
