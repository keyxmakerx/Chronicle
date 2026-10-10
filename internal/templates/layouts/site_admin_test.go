package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestInSiteAdmin(t *testing.T) {
	cases := []struct {
		name  string
		admin bool
		path  string
		want  bool
	}{
		{"admin home", true, "/admin", true},
		{"admin page", true, "/admin/users", true},
		{"admin elsewhere", true, "/campaigns", false},
		{"sibling path is not admin", true, "/administrators", false},
		{"non-admin on an admin path", false, "/admin/users", false},
		{"non-admin elsewhere", false, "/campaigns", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetActivePath(SetIsAdmin(context.Background(), tc.admin), tc.path)
			if got := InSiteAdmin(ctx); got != tc.want {
				t.Errorf("InSiteAdmin = %v, want %v", got, tc.want)
			}
		})
	}
}

// The admin menu replaces the everyday menu only for a site admin on an
// admin page; the footer's way in shows for a site admin anywhere and for
// nobody else.
func TestSidebar_SiteAdminPlace(t *testing.T) {
	cases := []struct {
		name      string
		admin     bool
		path      string
		menu      bool // the Site admin menu is drawn
		entry     bool // the footer's way in is drawn
		entryHere bool // and marked as the current place
	}{
		{"admin on an admin page", true, "/admin/users", true, true, true},
		{"admin outside admin", true, "/campaigns", false, true, false},
		{"non-admin on an admin path", false, "/admin/users", false, false, false},
		{"non-admin elsewhere", false, "/campaigns", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetActivePath(SetIsAdmin(context.Background(), tc.admin), tc.path)
			var buf bytes.Buffer
			if err := Sidebar().Render(ctx, &buf); err != nil {
				t.Fatalf("render: %v", err)
			}
			out := buf.String()
			if got := strings.Contains(out, `id="admin-nav"`); got != tc.menu {
				t.Errorf("admin menu drawn = %v, want %v", got, tc.menu)
			}
			if got := strings.Contains(out, "nav-admin-mode"); got != tc.menu {
				t.Errorf("admin shade = %v, want %v", got, tc.menu)
			}
			if got := strings.Contains(out, "My Campaigns"); got == tc.menu {
				t.Errorf("everyday menu drawn = %v, want %v", got, !tc.menu)
			}
			if got := strings.Contains(out, "data-site-admin-entry"); got != tc.entry {
				t.Errorf("way in drawn = %v, want %v", got, tc.entry)
			}
			if tc.entry {
				here := strings.Contains(out, `site-admin-entry" hx-boost="false" data-site-admin-entry title="Site admin" aria-label="Site admin" aria-current="page"`)
				if here != tc.entryHere {
					t.Errorf("way in marked current = %v, want %v", here, tc.entryHere)
				}
			}
		})
	}
}

// "Back to Chronicle" renders pointing at a fixed same-site page; only the
// script retargets it, after checking the stored path.
func TestAdminSidebarNav_BackLink(t *testing.T) {
	out := renderAdminNav(t, "/admin", nil)
	if !strings.Contains(out, `href="/campaigns" data-admin-back hx-boost="false"`) {
		t.Error("the way back must render as a full load to /campaigns")
	}
	if !strings.Contains(out, SiteAdminIcon) {
		t.Error("the band must carry the Site admin icon")
	}
}
