// dm_grant_visibility_test.go pins that campaigns.CampaignContext.
// VisibilityRole(), which promotes a DM-granted member (IsDmGranted=true) to
// Owner for visibility purposes (campaigns/model.go), is what every
// CheckEntityAccess/GetChildren call site here uses — never the raw
// cc.MemberRole — so a Co-DM (a Player with a DM grant) reaches dm_only
// content instead of getting 404'd (ADR-057).
//
// These tests drive Handler.Show and Handler.BacklinksFragment directly with
// a campaign context set the way other handler tests do
// (c.Set("campaign_context", ...)), so no router/session middleware is
// needed. The stub CheckEntityAccess mirrors the real service's
// default-visibility "dm_only" rule (Scribe+, role>=2) so the test exercises
// the actual promotion threshold.
package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// dmGrantEntitySvc is a minimal EntityService stub for a single dm_only
// (IsPrivate=true, default visibility) entity, plus one backlink entry that
// mentions it. CheckEntityAccess reproduces the real service's legacy
// default-mode rule so the Scribe+ (role>=2) threshold is genuine, not just
// an assertion on which constant got passed.
type dmGrantEntitySvc struct {
	EntityService
	entity   *Entity
	etype    *EntityType
	children []Entity
}

func (s *dmGrantEntitySvc) GetByID(_ context.Context, _ string) (*Entity, error) {
	e := *s.entity
	return &e, nil
}

func (s *dmGrantEntitySvc) GetEntityTypeByID(_ context.Context, _ int) (*EntityType, error) {
	et := *s.etype
	return &et, nil
}

func (s *dmGrantEntitySvc) GetAncestors(_ context.Context, _ string, _ int, _ string) ([]Entity, error) {
	return nil, nil
}

// GetChildren mirrors the real repository's visibilityFilter default-mode
// rule (entities/repository.go: role>=RoleScribe sees dm_only), so it
// exercises the actual promotion threshold Show's GetChildren call must
// clear for a Co-DM.
func (s *dmGrantEntitySvc) GetChildren(_ context.Context, _ string, role int, _ string) ([]Entity, error) {
	if role >= int(campaigns.RoleScribe) {
		return s.children, nil
	}
	visible := make([]Entity, 0, len(s.children))
	for _, ch := range s.children {
		if !ch.IsPrivate {
			visible = append(visible, ch)
		}
	}
	return visible, nil
}

// CheckEntityAccess mirrors entityService.CheckEntityAccess's legacy
// default-mode branch: a private (dm_only) entity needs role>=RoleScribe,
// the threshold VisibilityRole()'s Owner promotion must clear for a Co-DM.
func (s *dmGrantEntitySvc) CheckEntityAccess(_ context.Context, _ string, role int, _ string) (*EffectivePermission, error) {
	if s.entity.IsPrivate && role < int(campaigns.RoleScribe) {
		return &EffectivePermission{CanView: false}, nil
	}
	return &EffectivePermission{CanView: true, CanEdit: role >= int(campaigns.RoleScribe)}, nil
}

// GetBacklinksWithSnippets always returns one entry. Production scopes this
// query by VisibilityRole() (handler.go's BacklinksFragment computes `role`
// once, at the top); the list side is not what's under test here.
func (s *dmGrantEntitySvc) GetBacklinksWithSnippets(_ context.Context, _ string, _ string, _ int, _ string, _ bool) ([]BacklinkEntry, error) {
	return []BacklinkEntry{{
		Entity:  BacklinkEntity{ID: "mentioner-1", Name: "Secret War Council Minutes"},
		Snippet: "...the Baron's plan hinges on the hidden garrison...",
	}}, nil
}

// dmOnlyFixture returns a dm_only (IsPrivate, default-visibility) entity and
// a bare entity type. The entity type's Layout is left zero-valued (no rows)
// so Show falls through to its default two-column layout instead of a
// custom template — the same path a freshly-seeded entity type takes.
func dmOnlyFixture() (*Entity, *EntityType) {
	ent := &Entity{
		ID: "e1", CampaignID: "c1", EntityTypeID: 7, Name: "The Baron's Real Plan",
		IsPrivate: true, Visibility: VisibilityDefault,
	}
	et := &EntityType{ID: 7, CampaignID: "c1", Slug: "npc", Name: "NPC", NamePlural: "NPCs"}
	return ent, et
}

// coDmContext is a Player MemberRole with an Owner-granted dm_only visibility
// grant — the Co-DM shape the defect hits. Real Owners already satisfy every
// CheckEntityAccess call without promotion, so they don't exercise this path.
func coDmContext() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "c1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
		IsMember:    true,
	}
}

// TestShow_CoDmCanOpenDmOnlyEntity pins that a Co-DM (Player + DM grant)
// opening a dm_only entity gets the page, not a 404: Show's
// CheckEntityAccess call must use cc.VisibilityRole(), which promotes this
// viewer to Owner for visibility purposes, not the raw MemberRole.
func TestShow_CoDmCanOpenDmOnlyEntity(t *testing.T) {
	ent, et := dmOnlyFixture()
	h := &Handler{service: &dmGrantEntitySvc{entity: ent, etype: et}}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("c1", "e1")
	c.Set("campaign_context", coDmContext())
	auth.SetSession(c, &auth.Session{UserID: "codm-1"})

	err := h.Show(c)
	if err != nil {
		t.Fatalf("Co-DM opening a dm_only entity: Show returned %v, want nil (200) — "+
			"VisibilityRole() promotes IsDmGranted to Owner for visibility, but the "+
			"CheckEntityAccess call site is still gating on raw MemberRole", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Co-DM opening a dm_only entity: got status %d, want 200", rec.Code)
	}
}

// TestBacklinksFragment_CoDmSeesListAndReachesTarget pins that
// BacklinksFragment's list (built with cc.VisibilityRole()) and its own
// entity's CheckEntityAccess call use the same promoted role for a Co-DM:
// 200, with the populated list in the body, not a 404 on the entity itself.
func TestBacklinksFragment_CoDmSeesListAndReachesTarget(t *testing.T) {
	ent, et := dmOnlyFixture()
	h := &Handler{service: &dmGrantEntitySvc{entity: ent, etype: et}}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1/backlinks", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("c1", "e1")
	c.Set("campaign_context", coDmContext())
	auth.SetSession(c, &auth.Session{UserID: "codm-1"})

	err := h.BacklinksFragment(c)
	if err != nil {
		t.Fatalf("Co-DM requesting backlinks of a dm_only entity: got %v, want nil (200)", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Co-DM requesting backlinks of a dm_only entity: got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Secret War Council Minutes") {
		t.Errorf("backlinks list must be populated for the Co-DM viewer; body=%s", rec.Body)
	}
}

// TestShow_CoDmSeesDmOnlyChildren pins that a Co-DM let onto a dm_only
// parent page also sees that page's dm_only children in the Sub-pages
// section: Show's GetChildren call must use cc.VisibilityRole(), the same
// promoted role as its CheckEntityAccess call, not the raw MemberRole.
func TestShow_CoDmSeesDmOnlyChildren(t *testing.T) {
	ent, et := dmOnlyFixture()
	child := Entity{
		ID: "child-1", CampaignID: "c1", EntityTypeID: 7, Name: "The Hidden Vault",
		IsPrivate: true, Visibility: VisibilityDefault, TypeName: "NPC",
	}
	h := &Handler{service: &dmGrantEntitySvc{entity: ent, etype: et, children: []Entity{child}}}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("c1", "e1")
	c.Set("campaign_context", coDmContext())
	auth.SetSession(c, &auth.Session{UserID: "codm-1"})

	err := h.Show(c)
	if err != nil {
		t.Fatalf("Co-DM opening a dm_only entity: Show returned %v, want nil (200)", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Co-DM opening a dm_only entity: got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "The Hidden Vault") {
		t.Errorf("Co-DM must see the dm_only child in the Sub-pages list; body=%s", rec.Body)
	}
}
