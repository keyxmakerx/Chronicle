package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// fakePlayerCheck says which files any player could open.
type fakePlayerCheck struct {
	open map[string]bool
	err  error
}

func (f *fakePlayerCheck) PlainPlayerCanOpen(_ context.Context, file *media.MediaFile) (bool, error) {
	return f.open[file.ID], f.err
}

// GetMedia's ?audience=players answers only for a file players could open,
// echoes the audience it applied, and refuses when the check is missing or fails.
func TestGetMedia_PlayersAudience(t *testing.T) {
	get := func(h *MediaAPIHandler, key *APIKey, id, audience string) (int, map[string]any) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/media/"+id+"?audience="+audience, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(apiKeyContextKey, key)
		c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "mediaID", Value: id}})
		if err := h.GetMedia(c); err != nil {
			var ae *apperror.AppError
			if !errors.As(err, &ae) {
				t.Fatalf("unexpected error %v", err)
			}
			return ae.Code, nil
		}
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}
	owner := func(check PlayerMediaCheck) (*MediaAPIHandler, *APIKey) {
		h, _, key := cardFixture(campaigns.RoleScribe, &fakeCardGuard{})
		key.ID = 42 // a stored key is the owner's
		if check != nil {
			h.SetPlayerMediaCheck(check)
		}
		return h, key
	}

	tests := []struct {
		name     string
		check    PlayerMediaCheck
		id       string
		audience string
		want     int
		echo     string
	}{
		{name: "a file players can open", check: &fakePlayerCheck{open: map[string]bool{"plain": true}}, id: "plain", audience: "players", want: http.StatusOK, echo: "players"},
		{name: "a file players can't open", check: &fakePlayerCheck{}, id: "plain", audience: "players", want: http.StatusNotFound},
		{name: "the check fails", check: &fakePlayerCheck{open: map[string]bool{"plain": true}, err: errors.New("db")}, id: "plain", audience: "players", want: http.StatusNotFound},
		{name: "the check is unwired", id: "plain", audience: "players", want: http.StatusNotFound},
		{name: "no audience keeps the owner's link", check: &fakePlayerCheck{}, id: "plain", audience: "", want: http.StatusOK},
		{name: "an unknown audience", check: &fakePlayerCheck{}, id: "plain", audience: "everyone", want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, key := owner(tt.check)
			code, body := get(h, key, tt.id, tt.audience)
			if code != tt.want {
				t.Fatalf("answered %d, want %d", code, tt.want)
			}
			if code == http.StatusOK {
				got, _ := body["audience"].(string)
				if got != tt.echo {
					t.Errorf("audience echo = %q, want %q", got, tt.echo)
				}
			}
		})
	}
}

func TestMediaAPIPlayerMediaCheckWiredFlag(t *testing.T) {
	h := NewMediaAPIHandler(&stubSyncSvcForRole{}, &listSvc{})
	if h.PlayerMediaCheckWired() {
		t.Error("PlayerMediaCheckWired must be false when unset")
	}
	h.SetPlayerMediaCheck(&fakePlayerCheck{})
	if !h.PlayerMediaCheckWired() {
		t.Error("PlayerMediaCheckWired must be true once set")
	}
}
