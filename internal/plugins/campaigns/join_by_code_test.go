// join_by_code_test.go pins where a share link leaves a player: inside the
// campaign, told they joined only when they just did, and never on an error
// page for following the link a second time.

package campaigns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestJoinByCode_Service(t *testing.T) {
	archivedAt := time.Now()
	tests := []struct {
		name       string
		archived   bool
		member     bool
		wantJoined bool
		wantAdded  bool
		wantErr    int
	}{
		{name: "new player joins", wantJoined: true, wantAdded: true},
		{name: "existing member is let in again", member: true},
		{name: "existing member of an archived campaign is let in", archived: true, member: true},
		{name: "archived campaign takes no one new", archived: true, wantErr: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			camp := &Campaign{ID: "camp-1", Name: "The Shattered Coast"}
			if tt.archived {
				camp.ArchivedAt = &archivedAt
			}
			added := false
			repo := &mockCampaignRepo{
				findByJoinCodeFn: func(_ context.Context, code string) (*Campaign, error) {
					if code != "abc123" {
						t.Fatalf("looked up code %q", code)
					}
					return camp, nil
				},
				findMemberFn: func(_ context.Context, _, _ string) (*CampaignMember, error) {
					if tt.member {
						return &CampaignMember{Role: RolePlayer}, nil
					}
					return nil, apperror.NewNotFound("member not found")
				},
				addMemberFn: func(_ context.Context, m *CampaignMember) error {
					added = true
					if m.Role != RolePlayer || m.CampaignID != "camp-1" || m.UserID != "user-9" {
						t.Errorf("added %+v, want a player of camp-1", m)
					}
					return nil
				},
			}
			svc := newTestCampaignService(repo, &mockUserFinder{})
			got, joined, err := svc.JoinByCode(context.Background(), "abc123", "user-9")
			if tt.wantErr != 0 {
				assertAppError(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil || got.ID != "camp-1" {
				t.Errorf("campaign = %+v, want camp-1", got)
			}
			if joined != tt.wantJoined {
				t.Errorf("joined = %v, want %v", joined, tt.wantJoined)
			}
			if added != tt.wantAdded {
				t.Errorf("member added = %v, want %v", added, tt.wantAdded)
			}
		})
	}
}

// joinStubService answers JoinByCode and nothing else.
type joinStubService struct {
	CampaignService
	joined bool
}

func (s joinStubService) JoinByCode(context.Context, string, string) (*Campaign, bool, error) {
	return &Campaign{ID: "camp-1"}, s.joined, nil
}

func TestJoinByCode_HandlerLandsInCampaign(t *testing.T) {
	tests := []struct {
		name   string
		user   string
		joined bool
		want   string
	}{
		{name: "just joined", user: "user-9", joined: true, want: "/campaigns/camp-1?joined=1"},
		{name: "already a member", user: "user-9", want: "/campaigns/camp-1"},
		{name: "signed out keeps the link", want: "/login?redirect=%2Fjoin%2Fabc123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/join/abc123", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("code")
			c.SetParamValues("abc123")
			if tt.user != "" {
				c.Set("auth_user_id", tt.user)
			}
			h := &Handler{service: joinStubService{joined: tt.joined}}
			if err := h.JoinByCode(c); err != nil {
				t.Fatalf("JoinByCode: %v", err)
			}
			if got := rec.Header().Get("Location"); got != tt.want {
				t.Errorf("Location = %q, want %q", got, tt.want)
			}
		})
	}
}
