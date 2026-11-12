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
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// stubGallery counts moderation calls; any other call panics on the nil
// embedded interface, which is the failure we want.
type stubGallery struct {
	Gallery
	calls int
}

func (s *stubGallery) AdminQueue(context.Context, string, int) (*AdminPage, error) {
	s.calls++
	return &AdminPage{Entries: []AdminEntry{}}, nil
}

func (s *stubGallery) AdminDecide(_ context.Context, _, id, _ string) (*AdminEntry, error) {
	s.calls++
	return &AdminEntry{ID: id, Name: "n"}, nil
}

type recorder struct{ actions []string }

func (r *recorder) RecordAdminChange(_ *echo.Context, action, _, _, _ string) {
	r.actions = append(r.actions, action)
}

// adminApp mounts the real moderation routes on a group guarded the way the
// admin group is: a session, then RequireSiteAdmin.
func adminApp(svc Gallery, rec ActivityRecorder, session *auth.Session) *echo.Echo {
	e := echo.New()
	g := e.Group("/admin",
		func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				if session != nil {
					auth.SetSession(c, session)
				}
				return next(c)
			}
		},
		auth.RequireSiteAdmin(),
	)
	RegisterAdminRoutes(g, NewGalleryHandler(svc, rec))
	return e
}

// Every moderation route refuses a signed-in non-admin and an anonymous
// request, and never reaches the service.
func TestModerationRoutesAreAdminOnly(t *testing.T) {
	routes := []struct{ method, path string }{
		{http.MethodGet, "/admin/design-gallery/entries"},
		{http.MethodPost, "/admin/design-gallery/e1/approve"},
		{http.MethodPost, "/admin/design-gallery/e1/reject"},
		{http.MethodPost, "/admin/design-gallery/e1/remove"},
	}
	sessions := map[string]*auth.Session{
		"owner who is not an admin": {UserID: "alice", IsAdmin: false},
		"anonymous":                 nil,
	}
	for who, sess := range sessions {
		for _, r := range routes {
			t.Run(who+" "+r.method+" "+r.path, func(t *testing.T) {
				stub := &stubGallery{}
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(r.method, r.path, strings.NewReader(""))
				req.Header.Set("Accept", "application/json")
				adminApp(stub, nil, sess).ServeHTTP(rec, req)
				if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized && rec.Code != http.StatusSeeOther {
					t.Fatalf("status = %d", rec.Code)
				}
				if stub.calls != 0 {
					t.Fatal("the service was reached")
				}
			})
		}
	}
}

func TestAdminDecisionIsLogged(t *testing.T) {
	stub, rec := &stubGallery{}, &recorder{}
	app := adminApp(stub, rec, &auth.Session{UserID: "root", IsAdmin: true})
	want := map[string]string{"approve": "designgallery.approved", "reject": "designgallery.rejected", "remove": "designgallery.removed"}
	for decision, action := range want {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/design-gallery/e1/"+decision, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d", decision, w.Code)
		}
		if got := rec.actions[len(rec.actions)-1]; got != action {
			t.Errorf("%s logged %q, want %q", decision, got, action)
		}
	}
	if stub.calls != 3 || len(rec.actions) != 3 {
		t.Fatalf("calls %d, logged %d", stub.calls, len(rec.actions))
	}
}

// The routes put RequireRole(Owner) in front of every campaign-side gallery
// handler; this pins what that gate does, so a handler is never reached by a
// player or a co-DM-level member.
func TestGalleryHandlersBehindOwnerGate(t *testing.T) {
	gh := NewGalleryHandler(&stubGallery{}, nil)
	handlers := map[string]echo.HandlerFunc{
		"browse": gh.Browse, "mine": gh.Mine, "copy": gh.Copy, "report": gh.Report,
		"unpublish": gh.Unpublish, "publish": gh.Publish, "republish": gh.Republish,
	}
	for _, role := range []campaigns.Role{campaigns.RolePlayer, campaigns.RoleScribe, campaigns.RoleNone} {
		for name, h := range handlers {
			c, _ := dctx(http.MethodPost, `{}`, role)
			var ae *apperror.AppError
			err := campaigns.RequireRole(campaigns.RoleOwner)(h)(c)
			if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
				t.Errorf("%s as role %v: %v", name, role, err)
			}
		}
	}
}

// "Update in gallery" is partial: a body naming only the note leaves the name
// and shown-as name absent.
type presenceGallery struct {
	Gallery
	in RepublishInput
}

func (p *presenceGallery) Republish(_ context.Context, _, _, _ string, in RepublishInput) (*GalleryMine, error) {
	p.in = in
	return &GalleryMine{}, nil
}

func TestRepublishBindsPresence(t *testing.T) {
	tests := []struct {
		name, body                 string
		nameSet, noteSet, makerSet bool
		noteNull                   bool
	}{
		{"note only", `{"note":"x"}`, false, true, false, false},
		{"null note", `{"note":null}`, false, true, false, true},
		{"name and maker", `{"name":"N","makerName":"M"}`, true, false, true, false},
		{"nothing", `{}`, false, false, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &presenceGallery{}
			c, _ := dctx(http.MethodPut, tc.body, campaigns.RoleOwner)
			if err := NewGalleryHandler(p, nil).Republish(c); err != nil {
				t.Fatal(err)
			}
			if p.in.Name.Present() != tc.nameSet || p.in.Note.Present() != tc.noteSet || p.in.MakerName.Present() != tc.makerSet || p.in.Note.IsNull() != tc.noteNull {
				t.Errorf("presence = %v/%v/%v null %v", p.in.Name.Present(), p.in.Note.Present(), p.in.MakerName.Present(), p.in.Note.IsNull())
			}
		})
	}
}
