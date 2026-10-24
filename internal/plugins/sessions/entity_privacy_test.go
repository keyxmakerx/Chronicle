package sessions

// Session pages must not leak the names of privately-visible linked entities
// to a Player. ADR-055 rule 3: hidden content is ABSENT for a non-Owner
// viewer — not greyed, not counted, not named.
//
// ListSessionEntities' repository join carries no privacy predicate, so
// ShowSession filters the linked-entity list against the same visibility
// policy the entities plugin applies (via EntityVisibilityFilter) before the
// list reaches the template. Owners and Scribes are unaffected.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// stubEntityVisibility is a minimal EntityVisibilityFilter: every entity id
// listed in `hidden` is reported as not viewable, everything else is.
type stubEntityVisibility struct {
	hidden map[string]bool
}

func (s *stubEntityVisibility) FilterViewableEntityIDs(_ context.Context, _ string, entityIDs []string, _ int, _ string) (map[string]bool, error) {
	out := make(map[string]bool, len(entityIDs))
	for _, id := range entityIDs {
		out[id] = !s.hidden[id]
	}
	return out, nil
}

// showSessionHTML drives the real handler (repo -> service -> handler ->
// template) for a session with two linked entities, one of which the
// visibility filter reports as hidden, and returns the rendered body.
func showSessionHTML(t *testing.T, role campaigns.Role) string {
	t.Helper()
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, CampaignID: "camp-1", Name: "Session One", Status: StatusPlanned}, nil
		},
		listSessionEntitiesFn: func(_ context.Context, _ string) ([]SessionEntity, error) {
			return []SessionEntity{
				{EntityID: "ent-secret", EntityName: "Secret Villain", EntitySlug: "secret-villain", Role: EntityRoleEncountered},
				{EntityID: "ent-public", EntityName: "Town Square", EntitySlug: "town-square", Role: EntityRoleMentioned},
			}, nil
		},
	}
	visFilter := &stubEntityVisibility{hidden: map[string]bool{"ent-secret": true}}
	svc := NewSessionService(repo, nil, visFilter)
	h := NewHandler(svc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/sessions/s1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "sid")
	c.SetParamValues("camp-1", "s1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: role,
	})

	if err := h.ShowSession(c); err != nil {
		t.Fatalf("ShowSession returned error: %v", err)
	}
	return rec.Body.String()
}

// TestShowSession_PlayerDoesNotSeeHiddenEntityName is the RED test for the
// leak: a Player must not receive the name or slug of a session-linked entity
// the entities plugin's own visibility policy would hide from them.
func TestShowSession_PlayerDoesNotSeeHiddenEntityName(t *testing.T) {
	html := showSessionHTML(t, campaigns.RolePlayer)
	if strings.Contains(html, "Secret Villain") {
		t.Errorf("Player-role session page leaked a hidden entity's name\n%s", html)
	}
	if strings.Contains(html, "secret-villain") {
		t.Errorf("Player-role session page leaked a hidden entity's slug (in its link href)\n%s", html)
	}
	if !strings.Contains(html, "Town Square") {
		t.Errorf("Player-role session page hid a visible entity it should still show\n%s", html)
	}
}

// TestShowSession_OwnerSeesEverything pins ADR-055's "keep the Owner view
// unchanged": the fix must not narrow what the owner already saw.
func TestShowSession_OwnerSeesEverything(t *testing.T) {
	html := showSessionHTML(t, campaigns.RoleOwner)
	if !strings.Contains(html, "Secret Villain") {
		t.Error("the owner lost visibility into a linked entity it could already see")
	}
	if !strings.Contains(html, "Town Square") {
		t.Error("the owner lost visibility into a visible linked entity")
	}
}

// A Scribe sees what players see, so a hidden linked page stays hidden.
func TestShowSession_ScribeSeesWhatPlayersSee(t *testing.T) {
	html := showSessionHTML(t, campaigns.RoleScribe)
	if strings.Contains(html, "Secret Villain") {
		t.Errorf("Scribe session page showed a hidden entity's name\n%s", html)
	}
	if !strings.Contains(html, "Town Square") {
		t.Errorf("Scribe session page hid a visible entity it should still show\n%s", html)
	}
}

// TestFilterEntitiesForViewer_NilFilterFailsClosed pins the fail-closed branch
// of sessionService.FilterEntitiesForViewer: a Player/anonymous viewer with no
// EntityVisibilityFilter wired must get back no linked entities at all, never
// the unfiltered list.
// Without this test, a later "simplification" of the nil check could turn it
// into a leak of hidden page names (keyxmakerx/Chronicle#716).
func TestFilterEntitiesForViewer_NilFilterFailsClosed(t *testing.T) {
	svc := NewSessionService(&mockSessionRepo{}, nil, nil)
	ents := []SessionEntity{
		{EntityID: "ent-secret", EntityName: "Secret Villain", EntitySlug: "secret-villain"},
		{EntityID: "ent-public", EntityName: "Town Square", EntitySlug: "town-square"},
	}

	got, err := svc.FilterEntitiesForViewer(context.Background(), "camp-1", ents, int(campaigns.RolePlayer), "user-1")
	if err != nil {
		t.Fatalf("FilterEntitiesForViewer returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FilterEntitiesForViewer with a nil filter returned %d entities, want 0 (fail closed): %+v", len(got), got)
	}
}

// TestFilterEntitiesForViewer_NilFilterStillUnchangedForOwner confirms the
// nil-filter fail-closed branch only applies below RoleOwner: the owner or a
// Co-DM (promoted to Owner) still sees everything when no filter is wired,
// since role >= RoleOwner returns before the nil check, and a Scribe fails
// closed like a player.
func TestFilterEntitiesForViewer_NilFilterStillUnchangedForOwner(t *testing.T) {
	svc := NewSessionService(&mockSessionRepo{}, nil, nil)
	ents := []SessionEntity{
		{EntityID: "ent-secret", EntityName: "Secret Villain"},
	}

	got, err := svc.FilterEntitiesForViewer(context.Background(), "camp-1", ents, int(campaigns.RoleOwner), "user-1")
	if err != nil {
		t.Fatalf("FilterEntitiesForViewer returned error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("FilterEntitiesForViewer for the owner with a nil filter returned %d entities, want 1 (unchanged)", len(got))
	}
	got, err = svc.FilterEntitiesForViewer(context.Background(), "camp-1", ents, int(campaigns.RoleScribe), "user-1")
	if err != nil {
		t.Fatalf("FilterEntitiesForViewer returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FilterEntitiesForViewer for a Scribe with a nil filter returned %d entities, want 0 (fail closed)", len(got))
	}
}
