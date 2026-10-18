// entity_gate_test.go pins that the notes routes bind :eid to the URL campaign
// and to the caller's view access before the service is reached, and that Get
// refuses a note that does not hang off the gated entity.
package entity_notes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

type mapGate struct {
	campaignOf map[string]string
	canView    map[string]bool
}

func (g mapGate) ResolveViewableEntity(_ context.Context, id string, _ int, _ string) (string, bool, error) {
	camp, ok := g.campaignOf[id]
	if !ok {
		return "", false, apperror.NewNotFound("entity not found")
	}
	return camp, g.canView[id], nil
}

type recordingNotesSvc struct {
	Service
	note  *Note
	calls int
}

func (s *recordingNotesSvc) List(context.Context, string, ViewerContext) ([]Note, error) {
	s.calls++
	return []Note{}, nil
}

func (s *recordingNotesSvc) Create(context.Context, string, ViewerContext, CreateNoteRequest) (*Note, error) {
	s.calls++
	return &Note{ID: "n-new"}, nil
}

func (s *recordingNotesSvc) Get(context.Context, string, ViewerContext) (*Note, error) {
	s.calls++
	return s.note, nil
}

func TestNotesRoutes_EntityGate(t *testing.T) {
	gate := mapGate{
		campaignOf: map[string]string{"ok": "camp-1", "priv": "camp-1", "foreign": "camp-2", "other": "camp-1"},
		canView:    map[string]bool{"ok": true, "priv": false, "foreign": true, "other": true},
	}
	tests := []struct {
		name       string
		method     string
		path       string
		note       *Note
		wantStatus int
		wantCalls  int
	}{
		{"list ok", http.MethodGet, "/entities/ok/notes", nil, http.StatusOK, 1},
		{"list foreign campaign entity", http.MethodGet, "/entities/foreign/notes", nil, http.StatusNotFound, 0},
		{"list hidden entity", http.MethodGet, "/entities/priv/notes", nil, http.StatusNotFound, 0},
		{"create foreign campaign entity", http.MethodPost, "/entities/foreign/notes", nil, http.StatusNotFound, 0},
		{"create hidden entity", http.MethodPost, "/entities/priv/notes", nil, http.StatusNotFound, 0},
		{"get ok", http.MethodGet, "/entities/ok/notes/n1", &Note{ID: "n1", EntityID: "ok", CampaignID: "camp-1"}, http.StatusOK, 1},
		{"get hidden entity", http.MethodGet, "/entities/priv/notes/n1", &Note{ID: "n1", EntityID: "priv", CampaignID: "camp-1"}, http.StatusNotFound, 0},
		{"get note from other entity", http.MethodGet, "/entities/ok/notes/n1", &Note{ID: "n1", EntityID: "other", CampaignID: "camp-1"}, http.StatusNotFound, 1},
		{"get note from other campaign", http.MethodGet, "/entities/ok/notes/n1", &Note{ID: "n1", EntityID: "ok", CampaignID: "camp-2"}, http.StatusNotFound, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &recordingNotesSvc{note: tt.note}
			e := echo.New()
			e.HTTPErrorHandler = func(err error, c echo.Context) {
				var appErr *apperror.AppError
				if errors.As(err, &appErr) {
					_ = c.NoContent(appErr.Code)
					return
				}
				_ = c.NoContent(http.StatusInternalServerError)
			}
			h := NewHandler(svc)
			h.SetEntityGate(gate)
			RegisterRoutes(e, h, gateFakeCampaignSvc{}, gateFakeAuthSvc{}, gateFakeAddonSvc{enabled: true})
			req := httptest.NewRequest(tt.method, "/campaigns/camp-1"+tt.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "tok"})
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			createdOK := tt.method == http.MethodPost && tt.wantStatus == http.StatusOK && rec.Code == http.StatusCreated
			if rec.Code != tt.wantStatus && !createdOK {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if svc.calls != tt.wantCalls {
				t.Errorf("service calls = %d, want %d", svc.calls, tt.wantCalls)
			}
		})
	}
}
