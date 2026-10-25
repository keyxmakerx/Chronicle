package syncapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeMonsterRepo struct {
	replaced bool
	stored   FoundryMonsterList
}

func (f *fakeMonsterRepo) Replace(_ context.Context, _ string, list FoundryMonsterList) error {
	f.replaced, f.stored = true, list
	return nil
}
func (f *fakeMonsterRepo) Get(context.Context, string) (*FoundryMonsterList, error) {
	return &f.stored, nil
}
func (f *fakeMonsterRepo) PruneOlderThan(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func callReportMonsters(h *SyncHistoryHandler, body string) (*httptest.ResponseRecorder, error) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("c1")
	c.Set(apiKeyContextKey, &APIKey{ID: 3, UserID: "u1", CampaignID: "c1", IsActive: true})
	return rec, h.ReportMonsters(c)
}

func TestReportMonsters_OnlyOwnerOrDMAccess(t *testing.T) {
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
			repo := &fakeMonsterRepo{}
			h := NewSyncHistoryHandler(&fakeHistoryRepo{}, &stubCampaignSvcForPlayers{role: tc.role, granted: tc.granted}, nil, nil)
			h.SetFoundryMonsters(repo)
			rec, err := callReportMonsters(h, `{"monsters":[{"name":"Stalker","pack":"Monsters","level":"3"}]}`)
			got := rec.Code
			if err != nil {
				got = apperror.SafeCode(err)
			}
			if got != tc.want || repo.replaced != (tc.want == http.StatusOK) {
				t.Fatalf("status %d (want %d), replaced %v, err %v", got, tc.want, repo.replaced, err)
			}
		})
	}
}

func TestReportMonsters_NotWired(t *testing.T) {
	h := NewSyncHistoryHandler(&fakeHistoryRepo{}, &stubCampaignSvcForPlayers{role: campaigns.RoleOwner}, nil, nil)
	if _, err := callReportMonsters(h, `{}`); apperror.SafeCode(err) != http.StatusNotFound {
		t.Fatalf("err %v", err)
	}
}

func TestCleanMonsters(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	got, err := cleanMonsters(reportMonstersRequest{Truncated: true, Monsters: []FoundryMonster{
		{Name: "  Stal\x1bker\n ", Pack: "Monsters", Level: "3", Role: "Harrier"},
		{Name: "stalker", Pack: "monsters"},
		{Name: "Stalker"},
		{Name: "   "},
		{Name: strings.Repeat("x", 300), EV: strings.Repeat("9", 100), Linked: true},
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || !got.ReportedAt.Equal(now) || len(got.Monsters) != 3 {
		t.Fatalf("got %+v", got)
	}
	if m := got.Monsters[0]; m.Name != "Stalker" || m.Pack != "Monsters" || m.Level != "3" {
		t.Errorf("control characters stripped and trimmed: %+v", m)
	}
	if got.Monsters[1].Pack != "" {
		t.Errorf("a world actor and a compendium entry of one name are both kept: %+v", got.Monsters)
	}
	if m := got.Monsters[2]; len([]rune(m.Name)) != foundryMonsterNameMax || len(m.EV) != foundryMonsterShortMax || !m.Linked {
		t.Errorf("caps: %+v", m)
	}
	big := make([]FoundryMonster, foundryMonstersMax+1)
	for i := range big {
		big[i].Name = fmt.Sprint(i)
	}
	if _, err := cleanMonsters(reportMonstersRequest{Monsters: big}, now); apperror.SafeCode(err) != http.StatusBadRequest {
		t.Fatalf("over the cap: %v", err)
	}
}
