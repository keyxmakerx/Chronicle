package syncapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// linkPageCall runs a session-side link handler as a signed-in member.
func linkPageCall(h *SyncHistoryHandler, method string, cc *campaigns.CampaignContext, userID string) (*httptest.ResponseRecorder, error) {
	req := httptest.NewRequest(method, "/", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}})
	c.Set("campaign_context", cc)
	if userID != "" {
		c.Set("auth_user_id", userID)
	}
	if method == http.MethodPost {
		return rec, h.IssueLinkCode(c)
	}
	return rec, h.LinkFragment(c)
}

func playerCtx() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true}
}

func newLinkHandler(players []FoundryPlayer, members []campaigns.CampaignMember, role campaigns.Role) (*SyncHistoryHandler, *LinkCodes) {
	h := newPlayersHandler(&stubCampaignSvcForPlayers{role: role, members: members}, &fakePlayerRepo{stored: players})
	codes := NewLinkCodes()
	h.SetLinkCodes(codes)
	return h, codes
}

var shownCode = regexp.MustCompile(`data-link-code>([A-Z0-9]{3}-[A-Z0-9]{3})<`)

func TestLinkFragment_WhoSeesWhat(t *testing.T) {
	reported := []FoundryPlayer{{FoundryUserID: "f1", Name: "Mira's Foundry", MemberUserID: "u2"}}
	owner := playerCtx()
	owner.MemberRole = campaigns.RoleOwner
	preview := playerCtx()
	preview.Preview = "player"
	cases := []struct {
		name    string
		players []FoundryPlayer
		cc      *campaigns.CampaignContext
		user    string
		want    string // substring; "" means an empty answer
	}{
		{"unlinked player is offered a code", reported, playerCtx(), "u1", `data-foundry-link="ask"`},
		{"linked player is told so", reported, playerCtx(), "u2", "Linked to Foundry as"},
		{"no world has reported: nothing", nil, playerCtx(), "u1", ""},
		{"the owner is Foundry's GM: nothing", reported, owner, "u1", ""},
		{"a preview never links: nothing", reported, preview, "u1", ""},
		{"signed out: nothing", reported, playerCtx(), "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newLinkHandler(tc.players, nil, campaigns.RolePlayer)
			rec, err := linkPageCall(h, http.MethodGet, tc.cc, tc.user)
			if err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			if tc.want == "" {
				if strings.TrimSpace(body) != "" {
					t.Fatalf("want nothing, got %q", body)
				}
				return
			}
			if !strings.Contains(body, tc.want) {
				t.Fatalf("want %q in %q", tc.want, body)
			}
			if strings.Contains(body, "data-link-code") {
				t.Fatal("a code is only made on a press")
			}
		})
	}
}

func TestIssueLinkCode_GivesTheSessionUsersOwnCode(t *testing.T) {
	members := []campaigns.CampaignMember{{UserID: "u1", Role: campaigns.RolePlayer, DisplayName: "Mira"}}
	h, codes := newLinkHandler([]FoundryPlayer{{FoundryUserID: "f9", Name: "Someone"}}, members, campaigns.RolePlayer)
	rec, err := linkPageCall(h, http.MethodPost, playerCtx(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	m := shownCode.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("no code shown in %q", rec.Body.String())
	}
	user, err := codes.Redeem("c1", m[1])
	if err != nil || user != "u1" {
		t.Fatalf("the shown code names %q (%v), want u1", user, err)
	}
}

func TestIssueLinkCode_Refusals(t *testing.T) {
	owner := playerCtx()
	owner.MemberRole = campaigns.RoleOwner
	stranger := playerCtx()
	stranger.IsMember = false
	for name, cc := range map[string]*campaigns.CampaignContext{"owner": owner, "not a member": stranger} {
		t.Run(name, func(t *testing.T) {
			h, _ := newLinkHandler(nil, nil, campaigns.RolePlayer)
			_, err := linkPageCall(h, http.MethodPost, cc, "u1")
			if apperror.SafeCode(err) != http.StatusForbidden {
				t.Fatalf("got %v, want 403", err)
			}
		})
	}
}

func callRedeem(h *SyncHistoryHandler, body string) (*httptest.ResponseRecorder, error) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}})
	c.Set(apiKeyContextKey, &APIKey{ID: 3, UserID: "gm", CampaignID: "c1", IsActive: true})
	return rec, h.RedeemLinkCode(c)
}

func TestRedeemLinkCode(t *testing.T) {
	members := []campaigns.CampaignMember{
		{UserID: "u1", Role: campaigns.RolePlayer, DisplayName: "Mira"},
		{UserID: "gm", Role: campaigns.RoleOwner, DisplayName: "GM"},
	}
	cases := []struct {
		name    string
		role    campaigns.Role // the key holder's
		issueTo string
		want    int
	}{
		{"GM key redeems a player's code", campaigns.RoleOwner, "u1", http.StatusOK},
		{"a player's key may not redeem", campaigns.RolePlayer, "u1", http.StatusForbidden},
		{"a member who left links no one", campaigns.RoleOwner, "gone", http.StatusNotFound},
		{"the owner's own code links no one", campaigns.RoleOwner, "gm", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, codes := newLinkHandler(nil, members, tc.role)
			code, _, _ := codes.Issue("c1", tc.issueTo)
			rec, err := callRedeem(h, `{"code":"`+FormatLinkCode(code)+`"}`)
			got := rec.Code
			if err != nil {
				got = apperror.SafeCode(err)
			}
			if got != tc.want {
				t.Fatalf("status %d, want %d (%v)", got, tc.want, err)
			}
			if tc.want != http.StatusOK {
				return
			}
			var resp redeemLinkCodeResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.UserID != "u1" || resp.DisplayName != "Mira" {
				t.Fatalf("got %+v", resp)
			}
			if _, err := callRedeem(h, `{"code":"`+code+`"}`); apperror.SafeCode(err) != http.StatusNotFound {
				t.Fatalf("a second redeem: %v, want 404", err)
			}
		})
	}
}

func TestRedeemLinkCode_NotWiredIsNotFound(t *testing.T) {
	h := newPlayersHandler(&stubCampaignSvcForPlayers{role: campaigns.RoleOwner}, &fakePlayerRepo{})
	if _, err := callRedeem(h, `{"code":"ABC-DEF"}`); apperror.SafeCode(err) != http.StatusNotFound {
		t.Fatalf("got %v", err)
	}
}
