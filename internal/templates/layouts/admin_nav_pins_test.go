package layouts

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
)

// admin_nav_pins_test.go pins how an admin's pins show in the sidebar: a
// Pinned group under Home only when something is pinned, unknown pins skipped,
// and a toggle on every section row whose pressed state follows the pins.

func TestAdminNavPinnableHrefs_ExcludesHome(t *testing.T) {
	hrefs := AdminNavPinnableHrefs()
	if len(hrefs) == 0 {
		t.Fatal("no pinnable pages")
	}
	seen := map[string]bool{}
	for _, h := range hrefs {
		if h == adminNav.Home.Href {
			t.Errorf("Home must not be pinnable")
		}
		if seen[h] {
			t.Errorf("%s listed twice", h)
		}
		seen[h] = true
	}
}

func TestAdminNavPinnedItems(t *testing.T) {
	tests := []struct {
		name string
		pins []string
		want []string
	}{
		{"none", nil, nil},
		{"in pin order", []string{"/admin/storage", "/admin/users"}, []string{"/admin/storage", "/admin/users"}},
		{"unknown skipped", []string{"/admin/renamed", "/admin/users"}, []string{"/admin/users"}},
		{"home skipped", []string{"/admin"}, nil},
		{"repeat skipped", []string{"/admin/users", "/admin/users"}, []string{"/admin/users"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, it := range adminNavPinnedItems(tc.pins) {
				got = append(got, it.Href)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func renderAdminNav(t *testing.T, path string, pins []string) string {
	t.Helper()
	ctx := SetActivePath(context.Background(), path)
	ctx = SetAdminNavPins(ctx, pins)
	var buf bytes.Buffer
	if err := AdminSidebarNav().Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestAdminSidebarNav_PinnedGroup(t *testing.T) {
	t.Run("nothing pinned shows no Pinned group", func(t *testing.T) {
		out := renderAdminNav(t, "/admin", nil)
		if strings.Contains(out, "Pinned admin pages") {
			t.Error("an empty Pinned group must not render")
		}
		if !strings.Contains(out, `aria-label="Pin People"`) || !strings.Contains(out, `aria-pressed="false"`) {
			t.Error("each section row needs an unpressed Pin toggle")
		}
	})

	t.Run("pinned page is listed under Home and still in its section", func(t *testing.T) {
		out := renderAdminNav(t, "/admin", []string{"/admin/storage"})
		group := strings.Index(out, "Pinned admin pages")
		home := strings.Index(out, `href="/admin"`)
		sec := strings.Index(out, "admin-nav-sec-community")
		if group < 0 || home < 0 || !(home < group && group < sec) {
			t.Fatalf("Pinned must sit between Home and the sections (home=%d group=%d sec=%d)", home, group, sec)
		}
		if n := strings.Count(out, `href="/admin/storage"`); n != 2 {
			t.Errorf("storage should be linked in Pinned and in its section, got %d", n)
		}
		if !strings.Contains(out, `aria-label="Unpin Storage"`) || !strings.Contains(out, `aria-pressed="true"`) {
			t.Error("the pinned row's toggle must read Unpin and be pressed")
		}
	})

	t.Run("current page keeps aria-current in the Pinned group", func(t *testing.T) {
		out := renderAdminNav(t, "/admin/storage", []string{"/admin/storage"})
		if n := strings.Count(out, `aria-current="page"`); n != 2 {
			t.Errorf("want the pinned row and the section row both marked current, got %d", n)
		}
	})

	// The block is swapped into the page after a pin change, so its toggle
	// must work without a sibling <script>, and no server value may be
	// spliced into script: the toggle names a constant Alpine method and
	// carries its link in a data attribute.
	t.Run("toggle is a constant Alpine call, not a script helper", func(t *testing.T) {
		out := renderAdminNav(t, "/admin", nil)
		if strings.Contains(out, "<script") {
			t.Error("the nav block must not emit a sibling <script>")
		}
		if strings.Contains(out, "onclick=") {
			t.Error("the toggle must not use an inline onclick built from server values")
		}
		if !strings.Contains(out, `@click="pinToggle($el)"`) || !strings.Contains(out, `data-admin-pin="/admin/users"`) {
			t.Error("the toggle needs @click=pinToggle($el) and its link in data-admin-pin")
		}
		if !strings.Contains(out, "/admin/nav/pins") {
			t.Error("the nav state must call the pins route")
		}
	})
}
