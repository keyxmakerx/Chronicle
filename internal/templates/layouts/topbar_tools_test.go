package layouts

import (
	"bytes"
	"context"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// viewer builds the context a signed-in person in a campaign has.
func viewer(role int, owner, dm, asPlayer, guest bool) context.Context {
	ctx := SetIsAuthenticated(context.Background(), true)
	ctx = SetCampaignID(ctx, "camp-1")
	ctx = SetCampaignRole(ctx, role)
	ctx = SetIsOwner(ctx, owner)
	ctx = SetIsDmGranted(ctx, dm)
	ctx = SetViewingAsPlayer(ctx, asPlayer)
	ctx = SetIsGuest(ctx, guest)
	return SetGMTerm(ctx, "Director")
}

var shelfEntry = regexp.MustCompile(`data-tb-for="([a-z]+)"[^>]*>.*?<span class="tb-tool-name">([^<]*)</span>`)

func renderBar(t *testing.T, ctx context.Context) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Topbar().Render(ctx, &buf); err != nil {
		t.Fatalf("render Topbar: %v", err)
	}
	return buf.String()
}

// The shelf lists a tool under exactly the conditions the bar draws its
// button, and a viewer with fewer than two tools keeps the bar as it was.
func TestTopbarToolsShelf_EntriesPerViewer(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want []string // "id:name", in shelf order
		tab  bool
	}{
		{"owner", viewer(3, true, false, false, false), []string{"ai:Your AI", "brush:Customize", "table:The table", "store:Store", "dms:Director Screen", "pv:View as player", "bell:Notifications", "find:Find"}, true},
		{"owner viewing as a player", viewer(1, true, false, true, false), []string{"table:The table", "pv:Back to your view", "bell:Notifications", "find:Find"}, true},
		{"co-DM", viewer(1, false, true, false, false), []string{"ai:Your AI", "table:The table", "dms:Director Screen", "pv:View as player", "bell:Notifications", "find:Find"}, true},
		{"player", viewer(1, false, false, false, false), []string{"ai:Your AI", "table:The table", "bell:Notifications", "find:Find"}, true},
		{"guest", viewer(1, false, false, false, true), []string{"table:The table", "bell:Notifications", "find:Find"}, true},
		{"signed in, no role here", viewer(0, false, false, false, false), nil, false},
		{"signed in outside a campaign", SetIsAuthenticated(context.Background(), true), nil, false},
		{"signed out", context.Background(), nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			html := renderBar(t, tc.ctx)
			hasTab := strings.Contains(html, `id="tb-tools-tab"`)
			if hasTab != tc.tab {
				t.Fatalf("Tools tab drawn = %v, want %v", hasTab, tc.tab)
			}
			if got := strings.Contains(html, "data-tb-tools"); got != tc.tab {
				t.Fatalf("header marks the folded bar = %v, want %v", got, tc.tab)
			}
			var got []string
			for _, m := range shelfEntry.FindAllStringSubmatch(html, -1) {
				got = append(got, m[1]+":"+m[2])
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("shelf entries = %v, want %v", got, tc.want)
			}
		})
	}
}

// Each entry stands for a real button that stays in the bar, marked so the
// phone stylesheet can step it off the row without removing its host.
func TestTopbarToolsShelf_RealButtonsStayMarked(t *testing.T) {
	html := renderBar(t, viewer(3, true, false, false, false))
	for _, id := range []string{"ai", "table", "dms", "pv", "bell"} {
		if !strings.Contains(html, `data-tb-tool="`+id+`"`) {
			t.Errorf("no bar tool marked %q", id)
		}
	}
	for _, real := range []string{"data-ai-chip", "data-tt-tab", "data-dms-toggle", "data-notif-toggle", "data-pv-toggle"} {
		if !strings.Contains(html, real) {
			t.Errorf("the real button %q is gone from the bar", real)
		}
	}
}

// The shelf is a bar menu, so the bar lifts over the page while it shows
// (input.css), and it starts closed.
func TestTopbarToolsShelf_IsABarMenuAndStartsClosed(t *testing.T) {
	html := renderBar(t, viewer(3, true, false, false, false))
	shelf := regexp.MustCompile(`<div[^>]*id="tb-shelf"[^>]*>`).FindString(html)
	if shelf == "" {
		t.Fatal("no shelf drawn")
	}
	if !strings.Contains(shelf, `tb-pop`) {
		t.Errorf("the shelf is not a .tb-pop: %s", shelf)
	}
	if !regexp.MustCompile(`\shidden[\s>]`).MatchString(shelf) {
		t.Errorf("the shelf starts open: %s", shelf)
	}
	tab := regexp.MustCompile(`<button[^>]*id="tb-tools-tab"[^>]*>`).FindString(html)
	for _, want := range []string{`aria-expanded="false"`, `aria-controls="tb-shelf"`, `type="button"`} {
		if !strings.Contains(tab, want) {
			t.Errorf("tab misses %s: %s", want, tab)
		}
	}
}

// A member's AI waits hidden until the plugin allows it, so it never earns a
// viewer the shelf by itself.
func TestTopbarToolsShelf_MemberAIDoesNotEarnTheShelf(t *testing.T) {
	ctx := viewer(1, false, false, false, false)
	ctx = SetIsAuthenticated(ctx, false)
	if topbarShowsToolsShelf(ctx) {
		t.Fatal("a viewer with only the member AI got the shelf")
	}
}

// The banner carries the eye's switch in words, on every screen width.
func TestViewingBanner_BackToYourView(t *testing.T) {
	var buf bytes.Buffer
	ctx := viewer(1, true, false, true, false)
	ctx = SetCSRFToken(ctx, "tok")
	if err := App("Page").Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	banner := regexp.MustCompile(`(?s)<div[^>]*data-pv-banner>.*?</div>`).FindString(html)
	for _, want := range []string{"Back to your view", "toggle-view-mode", "data-pv-toggle", "X-CSRF-Token"} {
		if !strings.Contains(banner, want) {
			t.Errorf("banner misses %q: %s", want, banner)
		}
	}
}

// The draw overlay's script is named on the Table's host and on the DM Screen's
// host, and only while the Card decks add-on is on, so a page that never uses
// the overlay never loads it.
func TestTopbar_DrawOverlayScriptIsNamedOnlyWithTheAddOn(t *testing.T) {
	srcOn := regexp.MustCompile(`data-dkd-src="/static/plugins/decks/js/deck_draw\.js[^"]*"`)
	for _, tt := range []struct {
		name  string
		addon bool
		hosts int
	}{{"add-on on", true, 2}, {"add-on off", false, 0}} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := viewer(3, true, false, false, false)
			ctx = SetEnabledAddons(ctx, map[string]bool{"decks": tt.addon})
			html := renderBar(t, ctx)
			if n := len(srcOn.FindAllString(html, -1)); n != tt.hosts {
				t.Errorf("%d hosts name the overlay script, want %d", n, tt.hosts)
			}
			if !strings.Contains(html, `data-tb-tool="table"`) || !strings.Contains(html, `data-tb-tool="dms"`) {
				t.Error("the owner's bar lacks the Table or the DM Screen")
			}
		})
	}
}
