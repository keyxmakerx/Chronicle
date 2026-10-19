package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

type stubSystemStateReader struct {
	called bool
	err    error
}

func (s *stubSystemStateReader) ReadSystemState(_ context.Context, _, _, _, _ string) (json.RawMessage, json.RawMessage, *time.Time, error) {
	s.called = true
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return json.RawMessage(`{"secret":"s"}`), json.RawMessage(`{"round":2}`), &now, s.err
}

func systemStateCall(t *testing.T, reader SystemStateReader, role campaigns.Role, bearer bool, campaignParam string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	ent, et := gmTestFixtures()
	h := &APIHandler{
		entitySvc:   &stubEntityServiceForGM{entity: ent, etype: et},
		campaignSvc: &stubCampaignSvcForGM{role: role},
	}
	if reader != nil {
		h.SetSystemStateReader(reader)
	}
	c, rec := gmContext(http.MethodGet, "/api/v1/campaigns/camp-1/entities/e1/system-state/drawsteel/negotiation", "e1", bearer)
	c.SetParamNames("id", "entityID", "system", "key")
	c.SetParamValues(campaignParam, "e1", "drawsteel", "negotiation")
	return rec, h.GetSystemState(c)
}

// The gm half goes only to callers the sync API treats as GM: a stored Bearer
// key (Owner-level) or an owner session; players and scribes get public only.
func TestGetSystemState_GMHalfOnlyForGM(t *testing.T) {
	tests := []struct {
		name   string
		role   campaigns.Role
		bearer bool
		wantGM bool
	}{
		{"bearer key", campaigns.RoleOwner, true, true},
		{"owner session", campaigns.RoleOwner, false, true},
		{"scribe session", campaigns.RoleScribe, false, false},
		{"player session", campaigns.RolePlayer, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := systemStateCall(t, &stubSystemStateReader{}, tt.role, tt.bearer, "camp-1")
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
				t.Fatal(err)
			}
			if _, has := m["gm"]; has != tt.wantGM {
				t.Fatalf("gm present = %v, want %v (%s)", has, tt.wantGM, rec.Body.String())
			}
			if !tt.wantGM && strings.Contains(rec.Body.String(), "secret") {
				t.Errorf("gm content leaked: %s", rec.Body.String())
			}
			if string(m["public"]) != `{"round":2}` {
				t.Errorf("public = %s", m["public"])
			}
		})
	}
}

func TestGetSystemState_Guards(t *testing.T) {
	t.Run("entity from another campaign", func(t *testing.T) {
		reader := &stubSystemStateReader{}
		// The fixture entity lives in camp-1; the key's campaign is camp-2.
		_, err := systemStateCall(t, reader, campaigns.RoleOwner, true, "camp-2")
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
			t.Fatalf("err = %v, want 404", err)
		}
		if reader.called {
			t.Error("reader must not run for a foreign entity")
		}
	})
	t.Run("not wired", func(t *testing.T) {
		_, err := systemStateCall(t, nil, campaigns.RoleOwner, true, "camp-1")
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
			t.Fatalf("err = %v, want 404", err)
		}
	})
}

// system_state.updated is a GM-side nudge, not a change-feed resource: the
// feed would record it and replay it to every client's catch-up.
func TestSystemStateUpdated_NotInChangeFeed(t *testing.T) {
	if _, _, ok := classifyChange(ws.MsgSystemStateUpdated); ok {
		t.Fatal("system_state.updated must not be classified into the change feed")
	}
}

func TestSystemStateRoute_RequiresRead(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	want := `cg.GET("/entities/:entityID/system-state/:system/:key", api.GetSystemState, RequirePermission(PermRead))`
	if !strings.Contains(string(src), want) {
		t.Errorf("routes.go missing %s", want)
	}
}

type stubSystemStateWriter struct {
	called       bool
	gm, public   *json.RawMessage
	campaign, id string
	userID       string
}

func (s *stubSystemStateWriter) WriteSystemState(_ context.Context, campaignID, entityID, _, _ string, gm, public *json.RawMessage, userID string) (json.RawMessage, json.RawMessage, *time.Time, error) {
	s.called = true
	s.gm, s.public, s.campaign, s.id, s.userID = gm, public, campaignID, entityID, userID
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return json.RawMessage(`{"secret":"new"}`), json.RawMessage(`{"round":3}`), &now, nil
}

func systemStatePutCall(t *testing.T, w SystemStateWriter, role campaigns.Role, bearer bool, campaignParam, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	ent, et := gmTestFixtures()
	h := &APIHandler{
		entitySvc:   &stubEntityServiceForGM{entity: ent, etype: et},
		campaignSvc: &stubCampaignSvcForGM{role: role},
	}
	if w != nil {
		h.SetSystemStateWriter(w)
	}
	c, rec := gmContext(http.MethodPut, "/api/v1/campaigns/camp-1/entities/e1/system-state/drawsteel/negotiation", "e1", bearer)
	c.SetRequest(httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body)))
	c.SetParamNames("id", "entityID", "system", "key")
	c.SetParamValues(campaignParam, "e1", "drawsteel", "negotiation")
	return rec, h.PutSystemState(c)
}

// Only an Owner-level caller writes, and the body's halves reach the writer
// exactly as sent so the service sees absent, empty and null distinctly.
func TestPutSystemState(t *testing.T) {
	tests := []struct {
		name       string
		role       campaigns.Role
		bearer     bool
		campaign   string
		body       string
		wantStatus int
		wantCalled bool
		wantGM     string // "" = nil pointer
		wantPublic string
	}{
		{"bearer key writes both halves", campaigns.RoleOwner, true, "camp-1", `{"gm":{"a":1},"public":{"b":2}}`, http.StatusOK, true, `{"a":1}`, `{"b":2}`},
		{"owner session writes one half", campaigns.RoleOwner, false, "camp-1", `{"public":{"b":2}}`, http.StatusOK, true, "", `{"b":2}`},
		{"empty object reaches the writer to clear", campaigns.RoleOwner, true, "camp-1", `{"gm":{}}`, http.StatusOK, true, `{}`, ""},
		{"null half reaches the writer to be rejected", campaigns.RoleOwner, true, "camp-1", `{"gm":null}`, http.StatusOK, true, `null`, ""},
		{"scribe session", campaigns.RoleScribe, false, "camp-1", `{"public":{}}`, http.StatusForbidden, false, "", ""},
		{"player session", campaigns.RolePlayer, false, "camp-1", `{"public":{}}`, http.StatusForbidden, false, "", ""},
		{"other campaign", campaigns.RoleOwner, true, "camp-2", `{"public":{}}`, http.StatusNotFound, false, "", ""},
		{"unknown member", campaigns.RoleOwner, true, "camp-1", `{"gmm":{}}`, http.StatusUnprocessableEntity, false, "", ""},
		{"not an object", campaigns.RoleOwner, true, "camp-1", `[1]`, http.StatusBadRequest, false, "", ""},
		{"null body", campaigns.RoleOwner, true, "camp-1", `null`, http.StatusBadRequest, false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &stubSystemStateWriter{}
			rec, err := systemStatePutCall(t, w, tt.role, tt.bearer, tt.campaign, tt.body)
			if tt.wantStatus != http.StatusOK {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tt.wantStatus {
					t.Fatalf("err = %v, want %d", err, tt.wantStatus)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				var m map[string]json.RawMessage
				if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
					t.Fatal(err)
				}
				if string(m["gm"]) != `{"secret":"new"}` || string(m["public"]) != `{"round":3}` || string(m["isGm"]) != "true" {
					t.Errorf("response = %s", rec.Body.String())
				}
			}
			if w.called != tt.wantCalled {
				t.Fatalf("writer called = %v, want %v", w.called, tt.wantCalled)
			}
			for _, h := range []struct {
				name string
				got  *json.RawMessage
				want string
			}{{"gm", w.gm, tt.wantGM}, {"public", w.public, tt.wantPublic}} {
				switch {
				case h.want == "" && h.got != nil:
					t.Errorf("%s = %s, want absent", h.name, *h.got)
				case h.want != "" && (h.got == nil || string(*h.got) != h.want):
					t.Errorf("%s = %v, want %s", h.name, h.got, h.want)
				}
			}
		})
	}
}

func TestPutSystemState_NotWired(t *testing.T) {
	_, err := systemStatePutCall(t, nil, campaigns.RoleOwner, true, "camp-1", `{"public":{}}`)
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Fatalf("err = %v, want 404", err)
	}
}

func TestSystemStatePutRoute_RequiresWrite(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	want := `cg.PUT("/entities/:entityID/system-state/:system/:key", api.PutSystemState, RequirePermission(PermWrite))`
	if !strings.Contains(string(src), want) {
		t.Errorf("routes.go missing %s", want)
	}
}
