package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// stubCampaignSvcForPlayers answers the three calls ReportPlayers makes.
type stubCampaignSvcForPlayers struct {
	campaigns.CampaignService
	role    campaigns.Role
	granted bool
	members []campaigns.CampaignMember
}

func (s *stubCampaignSvcForPlayers) GetMember(_ context.Context, _, _ string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{Role: s.role}, nil
}

func (s *stubCampaignSvcForPlayers) IsUserDmGranted(_ context.Context, _, _ string) (bool, error) {
	return s.granted, nil
}

func (s *stubCampaignSvcForPlayers) ListMembers(_ context.Context, _ string) ([]campaigns.CampaignMember, error) {
	return s.members, nil
}

type fakePlayerRepo struct {
	replaced bool
	campaign string
	stored   []FoundryPlayer
}

func (f *fakePlayerRepo) Replace(_ context.Context, campaignID string, players []FoundryPlayer) error {
	f.replaced, f.campaign, f.stored = true, campaignID, players
	return nil
}

func (f *fakePlayerRepo) List(context.Context, string) ([]FoundryPlayer, error) { return f.stored, nil }

func (f *fakePlayerRepo) PruneOlderThan(context.Context, time.Time) (int64, error) { return 0, nil }

// callReportPlayers runs the handler directly with a key set, returning the
// recorder and the handler's error.
func callReportPlayers(h *SyncHistoryHandler, body string) (*httptest.ResponseRecorder, error) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("c1")
	c.Set(apiKeyContextKey, &APIKey{ID: 3, UserID: "u1", CampaignID: "c1", IsActive: true})
	return rec, h.ReportPlayers(c)
}

func newPlayersHandler(svc campaigns.CampaignService, repo FoundryPlayerRepository) *SyncHistoryHandler {
	h := NewSyncHistoryHandler(&fakeHistoryRepo{}, svc, nil, nil)
	h.SetFoundryPlayers(repo)
	h.now = func() time.Time { return playersNow }
	return h
}

func TestReportPlayers_OnlyOwnerOrDMAccess(t *testing.T) {
	cases := []struct {
		name    string
		role    campaigns.Role
		granted bool
		want    int
	}{
		{"owner", campaigns.RoleOwner, false, http.StatusOK},
		{"player with DM access", campaigns.RolePlayer, true, http.StatusOK},
		{"scribe without DM access", campaigns.RoleScribe, false, http.StatusForbidden},
		{"player", campaigns.RolePlayer, false, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakePlayerRepo{}
			h := newPlayersHandler(&stubCampaignSvcForPlayers{role: tc.role, granted: tc.granted}, repo)
			rec, err := callReportPlayers(h, `{"players":[{"foundryUserId":"f1","name":"Ann"}]}`)
			got := rec.Code
			if err != nil {
				got = apperror.SafeCode(err)
			}
			if got != tc.want {
				t.Fatalf("status %d, want %d (err %v)", got, tc.want, err)
			}
			if repo.replaced != (tc.want == http.StatusOK) {
				t.Fatalf("replaced = %v for status %d", repo.replaced, tc.want)
			}
		})
	}
}

func TestReportPlayers_NoKeyIsForbidden(t *testing.T) {
	repo := &fakePlayerRepo{}
	h := newPlayersHandler(&stubCampaignSvcForPlayers{role: campaigns.RoleOwner}, repo)
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)), httptest.NewRecorder())
	err := h.ReportPlayers(c)
	if apperror.SafeCode(err) != http.StatusForbidden || repo.replaced {
		t.Fatalf("err %v replaced %v", err, repo.replaced)
	}
}

func TestReportPlayers_NotWiredIsNotFound(t *testing.T) {
	h := NewSyncHistoryHandler(&fakeHistoryRepo{}, &stubCampaignSvcForPlayers{role: campaigns.RoleOwner}, nil, nil)
	_, err := callReportPlayers(h, `{"players":[]}`)
	if apperror.SafeCode(err) != http.StatusNotFound {
		t.Fatalf("err %v", err)
	}
}

func TestReportPlayers_Stores(t *testing.T) {
	svc := &stubCampaignSvcForPlayers{role: campaigns.RoleOwner, members: []campaigns.CampaignMember{{UserID: "m1"}}}
	cases := []struct {
		name       string
		body       string
		wantStored int
		wantMember string
		wantErr    int
	}{
		{"member in the campaign is linked", `{"players":[{"foundryUserId":"f1","name":"Ann","memberId":"m1","online":true}]}`, 1, "m1", 0},
		{"member outside the campaign is stored unlinked", `{"players":[{"foundryUserId":"f1","name":"Eve","memberId":"stranger"}]}`, 1, "", 0},
		{"empty report clears the list", `{"players":[]}`, 0, "", 0},
		{"bad id is a 400 and stores nothing", `{"players":[{"foundryUserId":""}]}`, 0, "", http.StatusBadRequest},
		{"malformed body is a 400", `{"players":`, 0, "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakePlayerRepo{}
			rec, err := callReportPlayers(newPlayersHandler(svc, repo), tc.body)
			if tc.wantErr != 0 {
				if apperror.SafeCode(err) != tc.wantErr || repo.replaced {
					t.Fatalf("err %v replaced %v", err, repo.replaced)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var out map[string]int
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			if out["stored"] != tc.wantStored || !repo.replaced || repo.campaign != "c1" || len(repo.stored) != tc.wantStored {
				t.Fatalf("out %v repo %+v", out, repo)
			}
			if tc.wantStored == 1 && repo.stored[0].MemberUserID != tc.wantMember {
				t.Fatalf("member %q, want %q", repo.stored[0].MemberUserID, tc.wantMember)
			}
		})
	}
}
