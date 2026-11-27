package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeHomebrewAsks struct {
	listed    bool
	run, user string
	body      string
}

func (f *fakeHomebrewAsks) Waiting(_ context.Context, _, userID string) (any, int, error) {
	f.listed, f.user = true, userID
	return []map[string]string{{"id": "run-1"}}, 1, nil
}

func (f *fakeHomebrewAsks) Answer(_ context.Context, _, userID, runID string, body json.RawMessage) (any, error) {
	f.user, f.run, f.body = userID, runID, string(body)
	return map[string]string{"id": runID, "status": "done"}, nil
}

// Answering an ask is the Director's act, so only the owner's or a co-DM's
// key reaches the service, and a lookup that fails refuses.
func TestHomebrewAskAPIDMUser(t *testing.T) {
	tests := []struct {
		name   string
		svc    *cutsceneCampaigns
		answer string
		want   int
	}{
		{name: "a player's key lists", svc: &cutsceneCampaigns{role: campaigns.RolePlayer}, want: http.StatusForbidden},
		{name: "a player's key answers", svc: &cutsceneCampaigns{role: campaigns.RolePlayer}, answer: `{"answer":"yes"}`, want: http.StatusForbidden},
		{name: "the owner's key lists", svc: &cutsceneCampaigns{role: campaigns.RoleOwner}, want: http.StatusOK},
		{name: "a co-DM's key answers", svc: &cutsceneCampaigns{role: campaigns.RolePlayer, granted: true}, answer: `{"answer":"no"}`, want: http.StatusOK},
		{name: "an answer that is not JSON", svc: &cutsceneCampaigns{role: campaigns.RoleOwner}, answer: `yes`, want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeHomebrewAsks{}
			h := NewHomebrewAskAPIHandler(api, tt.svc)
			var err error
			c, rec := dmScreenContext(http.MethodPost, tt.answer, 1, "runID", "run-1")
			if tt.answer == "" {
				err = h.Waiting(c)
			} else {
				err = h.Answer(c)
			}
			got := rec.Code
			if err != nil {
				got = dmStatusOf(t, err)
			}
			if got != tt.want {
				t.Fatalf("status %d, want %d", got, tt.want)
			}
			reached := api.listed || api.run != ""
			if reached != (tt.want == http.StatusOK) {
				t.Fatalf("reached the service = %v", reached)
			}
			if tt.want == http.StatusOK && (api.user != "user-1" || (tt.answer != "" && (api.run != "run-1" || api.body != tt.answer))) {
				t.Errorf("service got user %q run %q body %q", api.user, api.run, api.body)
			}
		})
	}
}
