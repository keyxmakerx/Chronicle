package layouts

import (
	"bytes"
	"strings"
	"testing"
)

// Only the owner manages a campaign: a Scribe, a Player, an owner viewing as
// a player (role reads Player) and a visitor all get no Manage rows.
func TestNavManageRows_OwnerOnly(t *testing.T) {
	tests := []struct {
		name string
		c    navStateCase
		want []string
	}{
		{"owner", navStateCase{role: 3, path: "/campaigns/c1"}, []string{"Overview", "People", "Customize", "Game & features", "Trash", "Settings"}},
		{"scribe", navStateCase{role: 2, path: "/campaigns/c1"}, nil},
		{"player", navStateCase{role: 1, path: "/campaigns/c1"}, nil},
		{"owner viewing as player", navStateCase{role: 1, player: true, path: "/campaigns/c1"}, nil},
		{"visitor", navStateCase{role: 0, path: "/campaigns/c1"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, r := range NavManageRows(tt.c.ctx()) {
				got = append(got, r.Label)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("rows = %v, want %v", got, tt.want)
			}
		})
	}
}

// The Foundry page joins the owner's Manage rows only when the campaign syncs.
func TestNavManageRows_FoundryNeedsSyncAPI(t *testing.T) {
	ctx := SetEnabledAddons(navStateCase{role: 3, path: "/campaigns/c1"}.ctx(), map[string]bool{"sync-api": true})
	var got []string
	for _, r := range NavManageRows(ctx) {
		got = append(got, r.Label)
	}
	want := "Overview,People,Customize,Game & features,Foundry,Trash,Settings"
	if strings.Join(got, ",") != want {
		t.Fatalf("rows = %v, want %s", got, want)
	}
}

// The sidebar is the only Manage menu: the header is a title, never a
// second row of the same links.
func TestManageHeader_TitleOnly(t *testing.T) {
	var buf bytes.Buffer
	ctx := navStateCase{role: 3, path: "/campaigns/c1/members"}.ctx()
	if err := ManageHeader("People").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Manage", "Saltmarsh", "<h1", "People"} {
		if !strings.Contains(out, want) {
			t.Errorf("header is missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "<nav") || strings.Contains(out, "href=") {
		t.Errorf("header repeats the Manage links: %s", out)
	}
}
