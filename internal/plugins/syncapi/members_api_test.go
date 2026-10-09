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
			c.SetParamNames("id")
			c.SetParamValues("c1")
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
