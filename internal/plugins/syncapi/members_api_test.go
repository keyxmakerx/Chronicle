package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

type stubMembersSvc struct {
	campaigns.CampaignService
	members []campaigns.CampaignMember
}

func (s *stubMembersSvc) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	return s.members, nil
}

func TestListMembers_AvatarURLAndNoEmail(t *testing.T) {
	const avatarID = "b7c17bb1-6563-462c-8b49-5b2e8bd57108"
	path := avatarID
	legacy := "2026/03/" + avatarID + ".jpg"

	cases := []struct {
		name    string
		signer  bool
		avatar  *string
		wantURL bool
	}{
		{"picture, signer configured", true, &path, true},
		{"no picture", true, nil, false},
		{"legacy path value is not signed", true, &legacy, false},
		{"no signer", false, &path, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubMembersSvc{members: []campaigns.CampaignMember{{
				CampaignID: "c1", UserID: "u1", Role: campaigns.RolePlayer, JoinedAt: time.Now(),
				DisplayName: "Alice", Email: "alice@example.com", AvatarPath: tc.avatar,
			}}}
			h := NewAPIHandler(nil, nil, svc, nil)
			if tc.signer {
				h.SetURLSigner(media.NewURLSigner("test-secret"))
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}})
			if err := h.ListMembers(c); err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			if strings.Contains(body, "alice@example.com") || strings.Contains(body, `"email"`) || strings.Contains(body, "avatar_path") {
				t.Errorf("response leaks email or the raw avatar id: %s", body)
			}
			var out []map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out) != 1 {
				t.Fatalf("bad body %s: %v", body, err)
			}
			url, _ := out[0]["avatar_url"].(string)
			if tc.wantURL {
				if !strings.HasPrefix(url, "/media/"+avatarID+"/thumb/300?expires=") || !strings.Contains(url, "&campaign=c1") {
					t.Errorf("avatar_url = %q", url)
				}
			} else if url != "" {
				t.Errorf("avatar_url = %q, want none", url)
			}
			if out[0]["display_name"] != "Alice" || out[0]["user_id"] != "u1" {
				t.Errorf("identity fields lost: %v", out[0])
			}
		})
	}
}

// redactSvc answers the member list plus the caller's role and co-DM grant,
// which ListMembers needs to decide what a session caller may see.
type redactSvc struct {
	campaigns.CampaignService
	role    campaigns.Role
	granted bool
}

func (redactSvc) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	shown, hidden, own := "e-shown", "e-hidden", "e-own"
	shownName, hiddenName, ownName := "Tyne", "Kestrel", "Orrin"
	return []campaigns.CampaignMember{
		{UserID: "ginko", Role: campaigns.RolePlayer, CharacterEntityID: &shown, CharacterName: &shownName, CharacterShared: true},
		{UserID: "kit", Role: campaigns.RolePlayer, CharacterEntityID: &hidden, CharacterName: &hiddenName},
		{UserID: "me", Role: campaigns.RolePlayer, CharacterEntityID: &own, CharacterName: &ownName},
	}, nil
}

func (s redactSvc) GetMember(_ context.Context, _, userID string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{UserID: userID, Role: s.role}, nil
}

func (s redactSvc) IsUserDmGranted(context.Context, string, string) (bool, error) {
	return s.granted, nil
}

// The sync API member list is open to session callers below Owner, so
// another player's hidden hero must not reach them; stored keys (Owner) and
// co-DMs see every hero, and a caller always sees their own.
func TestListMembers_HiddenHeroRedaction(t *testing.T) {
	cases := []struct {
		name       string
		keyID      int
		role       campaigns.Role
		granted    bool
		wantHidden bool
	}{
		{"player session", synthKeySessionID, campaigns.RolePlayer, false, false},
		{"scribe session", synthKeySessionID, campaigns.RoleScribe, false, false},
		{"co-DM session", synthKeySessionID, campaigns.RolePlayer, true, true},
		{"owner session", synthKeySessionID, campaigns.RoleOwner, false, true},
		{"stored key", 7, campaigns.RoleOwner, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewAPIHandler(nil, nil, redactSvc{role: tc.role, granted: tc.granted}, nil)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}})
			c.Set(apiKeyContextKey, &APIKey{ID: tc.keyID, CampaignID: "c1", UserID: "me", IsActive: true})
			if err := h.ListMembers(c); err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			if got := strings.Contains(body, "Kestrel") || strings.Contains(body, "e-hidden"); got != tc.wantHidden {
				t.Errorf("hidden hero present = %v, want %v; body %s", got, tc.wantHidden, body)
			}
			if !strings.Contains(body, "Tyne") || !strings.Contains(body, "Orrin") {
				t.Errorf("a shared hero or the caller's own was cleared: %s", body)
			}
		})
	}
}
