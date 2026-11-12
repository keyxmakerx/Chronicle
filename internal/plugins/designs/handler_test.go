package designs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// stubSvc records what the handler passed on; an unexpected call panics on the
// nil embedded Service, which is the failure we want.
type stubSvc struct {
	Service
	calls  int
	update UpdateDesignInput
	save   SaveInput
	err    error
}

func (s *stubSvc) List(context.Context, string, string, string) ([]Design, error) {
	s.calls++
	return []Design{}, s.err
}
func (s *stubSvc) Save(_ context.Context, _, _ string, in SaveInput) (*Design, error) {
	s.calls++
	s.save = in
	return &Design{ID: "d1"}, s.err
}
func (s *stubSvc) Update(_ context.Context, _, _, _ string, in UpdateDesignInput) (*Design, error) {
	s.calls++
	s.update = in
	return &Design{ID: "d1"}, s.err
}
func (s *stubSvc) ShareState(context.Context, string, string, string) (*ShareState, error) {
	s.calls++
	return &ShareState{}, s.err
}

func dctx(method, body string, role campaigns.Role) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "did", Value: "d1"}})
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role})
	return c, rec
}

// The routes put RequireRole(Owner) in front of every campaign handler; this
// pins what that gate does for each role, so a handler is never reached by a
// player or a co-DM-level member.
func TestOwnerGateInFrontOfHandlers(t *testing.T) {
	for _, role := range []struct {
		name string
		r    campaigns.Role
		ok   bool
	}{{"player", campaigns.RolePlayer, false}, {"scribe", campaigns.RoleScribe, false}, {"non-member", campaigns.RoleNone, false}, {"owner", campaigns.RoleOwner, true}} {
		for name, call := range map[string]func(h *Handler) echo.HandlerFunc{
			"list": func(h *Handler) echo.HandlerFunc { return h.List }, "save": func(h *Handler) echo.HandlerFunc { return h.Save },
			"update": func(h *Handler) echo.HandlerFunc { return h.Update }, "share": func(h *Handler) echo.HandlerFunc { return h.ShareState },
		} {
			t.Run(role.name+"/"+name, func(t *testing.T) {
				svc := &stubSvc{}
				c, _ := dctx(http.MethodPost, `{"name":"x"}`, role.r)
				err := campaigns.RequireRole(campaigns.RoleOwner)(call(NewHandler(svc)))(c)
				var ae *apperror.AppError
				if role.ok && err != nil {
					t.Fatalf("owner refused: %v", err)
				}
				if !role.ok && (!errors.As(err, &ae) || ae.Code != http.StatusForbidden) {
					t.Fatalf("want forbidden, got %v", err)
				}
				if !role.ok && svc.calls != 0 {
					t.Errorf("the service was reached %d times", svc.calls)
				}
			})
		}
	}
}

// A rename names only the name, so the note must arrive absent, not empty.
func TestUpdateBindsPresence(t *testing.T) {
	tests := []struct {
		name, body            string
		nameSet, noteSet, nul bool
	}{
		{"name only", `{"name":"X"}`, true, false, false},
		{"note only", `{"note":"warm"}`, false, true, false},
		{"explicit null note", `{"note":null}`, false, true, true},
		{"nothing", `{}`, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubSvc{}
			c, _ := dctx(http.MethodPatch, tt.body, campaigns.RoleOwner)
			if err := NewHandler(svc).Update(c); err != nil {
				t.Fatal(err)
			}
			if svc.update.Name.Present() != tt.nameSet || svc.update.Note.Present() != tt.noteSet || svc.update.Note.IsNull() != tt.nul {
				t.Errorf("name present %v, note present %v null %v", svc.update.Name.Present(), svc.update.Note.Present(), svc.update.Note.IsNull())
			}
		})
	}
}

func TestBadBodiesAre400(t *testing.T) {
	for name, body := range map[string]string{"not JSON": `{`, "wrong shape": `[1]`, "too large": `{"name":"` + strings.Repeat("a", maxBodyBytes) + `"}`} {
		t.Run(name, func(t *testing.T) {
			svc := &stubSvc{}
			c, _ := dctx(http.MethodPost, body, campaigns.RoleOwner)
			var ae *apperror.AppError
			if err := NewHandler(svc).Save(c); !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
				t.Fatalf("err = %v", err)
			}
			if svc.calls != 0 {
				t.Error("a bad body reached the service")
			}
		})
	}
}

func TestSaveBindsTheDesign(t *testing.T) {
	svc := &stubSvc{}
	c, rec := dctx(http.MethodPost, `{"kind":"room-theme","name":"Ward","note":"n","data":{"a":1}}`, campaigns.RoleOwner)
	if err := NewHandler(svc).Save(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated || svc.save.Kind != KindRoomTheme || svc.save.Name != "Ward" || svc.save.Note != "n" || string(svc.save.Data) != `{"a":1}` {
		t.Errorf("code %d, saved %+v", rec.Code, svc.save)
	}
}

func TestWantsPage(t *testing.T) {
	for accept, want := range map[string]bool{
		"text/html,application/xhtml+xml,*/*;q=0.8": true, "application/json": false, "text/html, application/json": false, "": false, "*/*": false,
	} {
		c, _ := dctx(http.MethodGet, "", campaigns.RoleOwner)
		c.Request().Header.Set("Accept", accept)
		if got := wantsPage(c); got != want {
			t.Errorf("Accept %q: wantsPage = %v, want %v", accept, got, want)
		}
	}
}
