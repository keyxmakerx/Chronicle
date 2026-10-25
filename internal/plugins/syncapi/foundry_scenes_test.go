package syncapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeSceneRepo struct {
	replaced bool
	campaign string
	stored   FoundrySceneReport
}

func (f *fakeSceneRepo) Replace(_ context.Context, campaignID string, r FoundrySceneReport) error {
	f.replaced, f.campaign, f.stored = true, campaignID, r
	return nil
}

func (f *fakeSceneRepo) Get(context.Context, string) (*FoundrySceneReport, error) {
	if !f.replaced {
		return nil, nil
	}
	return &f.stored, nil
}

func callReportScenes(h *SyncHistoryHandler, body string) (*httptest.ResponseRecorder, error) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("c1")
	c.Set(apiKeyContextKey, &APIKey{ID: 3, UserID: "u1", CampaignID: "c1", IsActive: true})
	return rec, h.ReportScenes(c)
}

func newScenesHandler(svc campaigns.CampaignService, repo FoundrySceneRepository) *SyncHistoryHandler {
	h := newPlayersHandler(svc, nil)
	h.SetFoundryScenes(repo)
	return h
}

func TestCleanScenes(t *testing.T) {
	long := strings.Repeat("é", foundrySceneNameMax+20)
	cases := []struct {
		name      string
		in        reportScenesRequest
		wantErr   bool
		wantNames []string
		check     func(t *testing.T, r FoundrySceneReport)
	}{
		{name: "empty id is refused", in: reportScenesRequest{Scenes: []reportedScene{{ID: " ", Name: "Cave"}}}, wantErr: true},
		{name: "id with spaces is refused", in: reportScenesRequest{Scenes: []reportedScene{{ID: "a b", Name: "Cave"}}}, wantErr: true},
		{name: "overlong id is refused", in: reportScenesRequest{Scenes: []reportedScene{{ID: strings.Repeat("x", 65)}}}, wantErr: true},
		{name: "duplicate id keeps the first", in: reportScenesRequest{Scenes: []reportedScene{{ID: "s1", Name: "Cave"}, {ID: "s1", Name: "Other"}}}, wantNames: []string{"Cave"}},
		{name: "blank name still counts", in: reportScenesRequest{Scenes: []reportedScene{{ID: "s1", Name: "  "}}}, wantNames: []string{"Unnamed scene"}},
		{name: "control characters are removed", in: reportScenesRequest{Scenes: []reportedScene{{ID: "s1", Name: "Ca\nve\x1b", Folder: "Act\t1"}}}, wantNames: []string{"Cave"},
			check: func(t *testing.T, r FoundrySceneReport) {
				if r.Scenes[0].Folder != "Act1" {
					t.Errorf("folder %q", r.Scenes[0].Folder)
				}
			}},
		{name: "long name is cut by characters", in: reportScenesRequest{Scenes: []reportedScene{{ID: "s1", Name: long}}},
			check: func(t *testing.T, r FoundrySceneReport) {
				if n := len([]rune(r.Scenes[0].Name)); n != foundrySceneNameMax {
					t.Errorf("name has %d characters", n)
				}
			}},
		{name: "flags and truncated carry through", in: reportScenesRequest{Truncated: true, Scenes: []reportedScene{{ID: "s1", Name: "Keep", Active: true, Navigation: true}}},
			check: func(t *testing.T, r FoundrySceneReport) {
				if !r.Truncated || !r.Scenes[0].Active || !r.Scenes[0].InNavigation || !r.ReportedAt.Equal(playersNow) {
					t.Errorf("got %+v", r)
				}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanScenes(tc.in, playersNow)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantNames != nil {
				var names []string
				for _, s := range got.Scenes {
					names = append(names, s.Name)
				}
				if fmt.Sprint(names) != fmt.Sprint(tc.wantNames) {
					t.Fatalf("names %v, want %v", names, tc.wantNames)
				}
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestCleanScenes_Limit(t *testing.T) {
	in := reportScenesRequest{}
	for i := 0; i < foundryScenesMax; i++ {
		in.Scenes = append(in.Scenes, reportedScene{ID: fmt.Sprintf("s%d", i), Name: "x"})
	}
	if _, err := cleanScenes(in, playersNow); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	in.Scenes = append(in.Scenes, reportedScene{ID: "one-more"})
	if _, err := cleanScenes(in, playersNow); err == nil {
		t.Fatal("over the limit was accepted")
	}
}

func TestReportScenes_OnlyOwnerOrDMAccess(t *testing.T) {
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
			repo := &fakeSceneRepo{}
			h := newScenesHandler(&stubCampaignSvcForPlayers{role: tc.role, granted: tc.granted}, repo)
			rec, err := callReportScenes(h, `{"scenes":[{"id":"s1","name":"Cave"}]}`)
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

func TestReportScenes_NotWiredIsNotFound(t *testing.T) {
	h := NewSyncHistoryHandler(&fakeHistoryRepo{}, &stubCampaignSvcForPlayers{role: campaigns.RoleOwner}, nil, nil)
	_, err := callReportScenes(h, `{"scenes":[]}`)
	if apperror.SafeCode(err) != http.StatusNotFound {
		t.Fatalf("err %v", err)
	}
}

func TestReportScenes_Stores(t *testing.T) {
	svc := &stubCampaignSvcForPlayers{role: campaigns.RoleOwner}
	cases := []struct {
		name       string
		body       string
		wantStored int
		wantErr    int
	}{
		{"scenes are stored", `{"scenes":[{"id":"s1","name":"Cave","folder":"Act 1"},{"id":"s2","name":"Keep"}]}`, 2, 0},
		{"empty report clears the list", `{"scenes":[]}`, 0, 0},
		{"bad id is a 400 and stores nothing", `{"scenes":[{"id":""}]}`, 0, http.StatusBadRequest},
		{"malformed body is a 400", `{"scenes":`, 0, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeSceneRepo{}
			_, err := callReportScenes(newScenesHandler(svc, repo), tc.body)
			if tc.wantErr != 0 {
				if apperror.SafeCode(err) != tc.wantErr || repo.replaced {
					t.Fatalf("err %v replaced %v", err, repo.replaced)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !repo.replaced || repo.campaign != "c1" || len(repo.stored.Scenes) != tc.wantStored {
				t.Fatalf("replaced %v campaign %q stored %d", repo.replaced, repo.campaign, len(repo.stored.Scenes))
			}
		})
	}
}
