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
// admin page; the Site admin row, the way in, shows for a site admin
// everywhere else and for nobody else.
func TestSidebar_SiteAdminPlace(t *testing.T) {
	cases := []struct {
		name  string
		admin bool
		path  string
		menu  bool // the Site admin menu is drawn
		entry bool // the way in is drawn
	}{
		{"admin on an admin page", true, "/admin/users", true, false},
		{"admin outside admin", true, "/campaigns", false, true},
		{"non-admin on an admin path", false, "/admin/users", false, false},
		{"non-admin elsewhere", false, "/campaigns", false, false},
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
		})
	}
}

// The way in is a labelled menu row and a full load, so the menu switches
// and site_admin.js can play the rise.
func TestSiteAdminEntry_Row(t *testing.T) {
	var buf bytes.Buffer
	if err := SiteAdminEntry().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`href="/admin" class="nav-row site-admin-entry" hx-boost="false" data-site-admin-entry`,
		`<span class="nav-lb">Site admin</span>`,
		SiteAdminIcon,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("row missing %q", want)
		}
	}
}

// Site admin pages carry data-site-admin on <html> (the control room look);
// no other page does, admin or not.
func TestAppearanceAttrs_SiteAdmin(t *testing.T) {
	cases := []struct {
		name  string
		admin bool
		path  string
		want  bool
	}{
		{"admin page", true, "/admin/users", true},
		{"admin elsewhere", true, "/campaigns", false},
		{"non-admin on an admin path", false, "/admin", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetActivePath(SetIsAdmin(context.Background(), tc.admin), tc.path)
			_, got := AppearanceAttrs(ctx)["data-site-admin"]
			if got != tc.want {
				t.Errorf("data-site-admin = %v, want %v", got, tc.want)
			}
		})
	}
}

// The top bar's way out shows only in Site admin, as a full load.
func TestTopbar_LeaveSiteAdmin(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{{"/admin/users", true}, {"/campaigns", false}} {
		ctx := SetActivePath(SetIsAdmin(context.Background(), true), tc.path)
		var buf bytes.Buffer
		if err := Topbar().Render(ctx, &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		out := buf.String()
		if got := strings.Contains(out, "Leave Site admin"); got != tc.want {
			t.Errorf("%s: leave button = %v, want %v", tc.path, got, tc.want)
		}
		if tc.want && !strings.Contains(out, `href="/campaigns" data-admin-back hx-boost="false"`) {
			t.Errorf("%s: leave button must be a full load marked data-admin-back", tc.path)
		}
		if got := strings.Contains(out, "data-theme-toggle"); got == tc.want {
			t.Errorf("%s: theme toggle = %v, want %v", tc.path, got, !tc.want)
		}
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
