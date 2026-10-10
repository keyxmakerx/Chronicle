package entities

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// stubSvcForPeek answers only the reads Peek performs. canView stands in for
// the service's CheckEntityAccess verdict (private page, grants, ...); the
// handler's job under test is to act on it and to filter what it draws.
type stubSvcForPeek struct {
	EntityService
	entity  *Entity
	etype   *EntityType
	canView bool
}

func (s *stubSvcForPeek) GetByID(_ context.Context, _ string) (*Entity, error) {
	e := *s.entity
	return &e, nil
}

func (s *stubSvcForPeek) GetEntityTypeByID(_ context.Context, _ int) (*EntityType, error) {
	return s.etype, nil
}

func (s *stubSvcForPeek) CheckEntityAccess(_ context.Context, _ string, _ int, _ string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: s.canView}, nil
}

// TestPeek_Access pins who gets what from the side-panel fragment: the same
// gate as the full page, the same field and secret stripping, and one
// indistinguishable answer for every page the viewer may not see.
func TestPeek_Access(t *testing.T) {
	const secretText = "the-duke-is-a-vampire"
	const gmField = "gm-only-weakness"
	body := `<p>Open text.</p><span data-secret="true">` + secretText + `</span>`
	et := &EntityType{ID: 7, Name: "Character", Icon: "fa-solid fa-user", Fields: []FieldDefinition{
		{Key: "race", Label: "Race"},
		{Key: "weakness", Label: "Weakness", GMOnly: true},
	}}
	newEntity := func(campaign string) *Entity {
		return &Entity{
			ID: "e1", CampaignID: campaign, EntityTypeID: 7, Name: "Duke Varrin", IsPrivate: true,
			EntryHTML:  &body,
			FieldsData: map[string]any{"race": "Elf", "weakness": gmField},
		}
	}

	cases := []struct {
		name         string
		role         campaigns.Role
		canView      bool
		entityCamp   string
		wantNotFound bool
		wantSecret   bool
	}{
		{"owner sees GM-only text and fields", campaigns.RoleOwner, true, "c1", false, true},
		{"scribe sees GM-only text and fields", campaigns.RoleScribe, true, "c1", false, true},
		{"player allowed sees page without GM-only content", campaigns.RolePlayer, true, "c1", false, false},
		{"player denied on private page gets not found", campaigns.RolePlayer, false, "c1", true, false},
		{"page from another campaign gets not found", campaigns.RoleOwner, true, "other", true, false},
	}

	var denied *apperror.AppError
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{service: &stubSvcForPeek{entity: newEntity(tc.entityCamp), etype: et, canView: tc.canView}}
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1/peek", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", "e1")
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign:   &campaigns.Campaign{ID: "c1"},
				MemberRole: tc.role,
			})
			auth.SetSession(c, &auth.Session{UserID: "u1"})

			err := h.Peek(c)
			if tc.wantNotFound {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
					t.Fatalf("want 404 AppError, got %v", err)
				}
				// Every denial must read the same, so existence and title
				// cannot be told apart from a page that is not there.
				if denied == nil {
					denied = ae
				} else if ae.Message != denied.Message || ae.Type != denied.Type {
					t.Errorf("denials differ: %q vs %q", ae.Message, denied.Message)
				}
				if strings.Contains(rec.Body.String(), "Duke Varrin") {
					t.Errorf("denied response leaked the title: %s", rec.Body)
				}
				return
			}
			if err != nil {
				t.Fatalf("Peek: %v", err)
			}
			out := rec.Body.String()
			if !strings.Contains(out, "Duke Varrin") || !strings.Contains(out, "Open text.") || !strings.Contains(out, "Elf") {
				t.Errorf("allowed viewer is missing the page: %s", out)
			}
			if got := strings.Contains(out, secretText); got != tc.wantSecret {
				t.Errorf("secret text present=%v, want %v", got, tc.wantSecret)
			}
			if got := strings.Contains(out, gmField); got != tc.wantSecret {
				t.Errorf("GM-only field present=%v, want %v", got, tc.wantSecret)
			}
			if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
				t.Errorf("Cache-Control = %q, want no-store", cc)
			}
		})
	}
}

// TestPeek_DocumentNavigationRedirects keeps a pasted peek address from
// showing a bare fragment: the browser is sent to the real page.
func TestPeek_DocumentNavigationRedirects(t *testing.T) {
	h := &Handler{service: &stubSvcForPeek{}}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1/peek", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("c1", "e1")
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}})
	if err := h.Peek(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/campaigns/c1/entities/e1" {
		t.Errorf("got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}
