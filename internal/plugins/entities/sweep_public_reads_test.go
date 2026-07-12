// sweep_public_reads_test.go — C-SWEEP-FIXES-R1 / cordinator#39 findings 3 + 5.
//   - The entity aliases read is reachable anonymously on a PUBLIC campaign and
//     bounces to /login on a PRIVATE one (finding 3); the handler's IDOR +
//     entity-privacy gate is also exercised.
//   - The "Player Notes" (entity_notes) block is not mounted for a viewer with
//     no authenticated identity (finding 5).
package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type guardAuthSvc struct{ auth.AuthService } // ValidateSession never called for anonymous.

type guardCampaignSvc struct {
	campaigns.CampaignService
	public bool
}

func (m guardCampaignSvc) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, IsPublic: m.public}, nil
}

type guardEntitySvc struct {
	EntityService
	private bool
}

func (m guardEntitySvc) GetByID(_ context.Context, id string) (*Entity, error) {
	return &Entity{ID: id, CampaignID: "camp-1", IsPrivate: m.private}, nil
}
func (guardEntitySvc) GetAliases(_ context.Context, _ string) ([]EntityAlias, error) {
	return []EntityAlias{}, nil
}

// CheckEntityAccess mirrors default-visibility semantics for this fixture so the
// aliases handler's canonical gate resolves the same as the removed bare check:
// a private entity needs Scribe+, a public entity is viewable by all.
func (m guardEntitySvc) CheckEntityAccess(_ context.Context, _ string, role int, _ string) (*EffectivePermission, error) {
	if m.private && role < int(campaigns.RoleScribe) {
		return &EffectivePermission{CanView: false}, nil
	}
	return &EffectivePermission{CanView: true}, nil
}

func isLoginRedirect(rec *httptest.ResponseRecorder) bool {
	switch rec.Code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return rec.Header().Get("Location") == "/login"
	}
	return false
}

func TestAliasesAnonymousAccess_PublicVsPrivate(t *testing.T) {
	newRouter := func(public, privateEntity bool) *echo.Echo {
		e := echo.New()
		e.Use(emw.Recover())
		RegisterRoutes(e, NewHandler(guardEntitySvc{private: privateEntity}), guardCampaignSvc{public: public}, guardAuthSvc{})
		return e
	}
	const path = "/campaigns/camp-1/entities/e1/aliases"

	tests := []struct {
		name          string
		public        bool
		privateEntity bool
		wantLogin     bool
	}{
		{"public campaign → reachable", true, false, false},
		{"private campaign → /login", false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newRouter(tt.public, tt.privateEntity).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if got := isLoginRedirect(rec); got != tt.wantLogin {
				t.Errorf("anonymous GET aliases (public=%v): login-redirect=%v (code=%d), want %v",
					tt.public, got, rec.Code, tt.wantLogin)
			}
		})
	}

	// Privacy gate: a private entity's aliases are NOT returned to an anonymous
	// viewer on a public campaign (RolePlayer < Scribe). The handler returns
	// apperror.NewNotFound (→ 404 under the app's error handler; the default
	// Echo handler used here surfaces it as a non-200 error), so the contract is
	// simply: never a 200 alias payload.
	t.Run("private entity not leaked to player", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newRouter(true, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("private entity aliases must not be returned to a player viewer (got 200: %q)", rec.Body.String())
		}
	})
}

func TestEntityNotesBlock_GatedToAuthenticated(t *testing.T) {
	r := NewBlockRegistry()
	if !r.HasRenderer("entity_notes") {
		RegisterCoreBlocks(r)
	}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer}
	ent := &Entity{ID: "e1", CampaignID: "camp-1"}

	render := func(userID string) string {
		comp := r.Render(context.Background(), BlockRenderContext{
			Block: TemplateBlock{Type: "entity_notes"}, CC: cc, Entity: ent, UserID: userID,
		})
		if comp == nil {
			return ""
		}
		var sb strings.Builder
		if err := comp.Render(context.Background(), &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		return sb.String()
	}

	if html := render(""); strings.Contains(html, `data-widget="entity-notes"`) {
		t.Errorf("a viewer with no identity must NOT receive the per-user entity-notes widget")
	}
	if html := render("user-1"); !strings.Contains(html, `data-widget="entity-notes"`) {
		t.Errorf("an authenticated member must receive the entity-notes widget")
	}
}
