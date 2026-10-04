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
