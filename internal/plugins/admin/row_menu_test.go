package admin

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func renderToString(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// confirmsIn returns every hx-confirm value in the markup, in order.
func confirmsIn(html string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`hx-confirm="([^"]*)"`).FindAllStringSubmatch(html, -1) {
		out = append(out, m[1])
	}
	return out
}

// splitLayouts separates the desktop table markup from the phone card markup.
func splitLayouts(t *testing.T, html, cardsID string) (table, cards string) {
	t.Helper()
	i := strings.Index(html, `data-testid="`+cardsID+`"`)
	if i < 0 {
		t.Fatalf("card list %s missing", cardsID)
	}
	return html[:i], html[i:]
}

func TestAdminUsersList_TableAndCardsShareActions(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		user         auth.User
		wantConfirms []string
	}{
		{"member", auth.User{ID: "u1", DisplayName: `Jane "J" D.`, Email: "j@x.test", CreatedAt: now},
			[]string{"Grant admin privileges to this user?", "Disable this user account? They will be immediately logged out.", "Force logout all sessions for this user?"}},
		{"admin", auth.User{ID: "u2", DisplayName: "Key", Email: "k@x.test", IsAdmin: true, CreatedAt: now},
			[]string{"Remove admin privileges from this user?", "Force logout all sessions for this user?"}},
		{"disabled", auth.User{ID: "u3", DisplayName: "Zed", Email: "z@x.test", IsDisabled: true, CreatedAt: now},
			[]string{"Grant admin privileges to this user?", "Re-enable this user account?", "Force logout all sessions for this user?"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := UserListData{
				Users:     []auth.User{tt.user},
				View:      userListView(listQuery{}, auth.UserFilterAll, auth.UserFilterCounts{All: 1}, 1),
				CSRFToken: "tok123",
			}
			html := renderToString(t, AdminUsersList(data))
			table, cards := splitLayouts(t, html, "users-cards")

			if got := confirmsIn(table); strings.Join(got, "|") != strings.Join(tt.wantConfirms, "|") {
				t.Errorf("table confirms = %v, want %v", got, tt.wantConfirms)
			}
			if got := confirmsIn(cards); strings.Join(got, "|") != strings.Join(tt.wantConfirms, "|") {
				t.Errorf("card menu confirms = %v, want %v", got, tt.wantConfirms)
			}
			for _, want := range []string{
				`aria-label="Actions for ` + templEscapedName(tt.user.DisplayName) + `"`,
				`aria-haspopup="menu"`, `aria-expanded="false"`, `role="menu"`, `role="menuitem"`,
				`/admin/users/` + tt.user.ID + `/admin`,
			} {
				if !strings.Contains(cards, want) {
					t.Errorf("card missing %s", want)
				}
			}
			if strings.Count(cards, `hx-headers="{&#34;X-CSRF-Token&#34;:&#34;tok123&#34;}"`) != len(tt.wantConfirms) {
				t.Errorf("every card action must carry the CSRF header; got %s", cards)
			}
			if xd := xData(cards); strings.Contains(xd, tt.user.ID) || strings.Contains(xd, tt.user.DisplayName) || strings.Contains(xd, "tok123") {
				t.Errorf("row data leaked into x-data: %s", xd)
			}
			// The destructive account action is red in the menu.
			if tt.name == "member" && !strings.Contains(cards, "text-orange-600") {
				t.Errorf("disable item should be toned")
			}
		})
	}
}

func xData(html string) string {
	var b strings.Builder
	for _, m := range regexp.MustCompile(`(?s)x-data="([^"]*)"`).FindAllStringSubmatch(html, -1) {
		b.WriteString(m[1])
	}
	return b.String()
}

func templEscapedName(s string) string { return strings.ReplaceAll(s, `"`, "&#34;") }

func TestAdminCampaignsList_TableAndCardsShareActions(t *testing.T) {
	c := campaigns.Campaign{ID: "c1", Name: "Shattered Coast", Slug: "coast", CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	data := CampaignListData{
		Campaigns: []campaigns.Campaign{c},
		View:      campaignListView(listQuery{}, "", nil, 1),
		CSRFToken: "tok123",
	}
	html := renderToString(t, AdminCampaignsList(data))
	table, cards := splitLayouts(t, html, "campaigns-cards")

	want := []string{
		"Join as Owner? Ownership moves to you and the current owner loses it.",
		"Leave this campaign? You will lose the access you gave yourself.",
		"Move this campaign to the trash? It disappears for everyone right away. You can bring it back from Trash until it empties.",
	}
	if got := confirmsIn(table); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("table confirms = %v, want %v", got, want)
	}
	if got := confirmsIn(cards); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("card confirms = %v, want %v", got, want)
	}
	for _, w := range []string{`aria-label="Actions for Shattered Coast"`, "Join as player", "Join as scribe", "Join as owner (transfers)…", "Leave campaign…", "Move to trash…", "text-red-600"} {
		if !strings.Contains(cards, w) {
			t.Errorf("card missing %q", w)
		}
	}
	for _, w := range []string{"Join as...", "Leave", "Move to trash", "aria-expanded"} {
		if !strings.Contains(table, w) {
			t.Errorf("table missing %q", w)
		}
	}
	// The popup must not be clipped by a wrapper: it is fixed-positioned and
	// itself carries no overflow rule.
	menuTag := regexp.MustCompile(`<div[^>]*role="menu"[^>]*>`).FindString(html)
	if !strings.Contains(menuTag, "fixed z-50") || strings.Contains(menuTag, "overflow") {
		t.Errorf("menu must be fixed-positioned and carry no overflow rule: %s", menuTag)
	}
	if !strings.Contains(html, `{&#34;csrf_token&#34;:&#34;tok123&#34;,&#34;role&#34;:&#34;owner&#34;}`) {
		t.Errorf("join role must be sent with the CSRF field; got %s", cards)
	}
}

// TestAdminLists_EmptyStateOnPhone keeps the empty message in the card list so
// a phone does not show a blank box.
func TestAdminLists_EmptyStateOnPhone(t *testing.T) {
	data := UserListData{View: userListView(listQuery{Q: "nobody"}, auth.UserFilterAll, auth.UserFilterCounts{}, 1), CSRFToken: "t"}
	_, cards := splitLayouts(t, renderToString(t, AdminUsersList(data)), "users-cards")
	if !strings.Contains(cards, "Clear search") {
		t.Errorf("phone empty state missing: %s", cards)
	}
}
