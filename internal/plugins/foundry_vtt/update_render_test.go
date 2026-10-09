package foundry_vtt

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func renderHTML(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func readyView() *OwnerUpdateView {
	return &OwnerUpdateView{
		CampaignID: "c1", CampaignName: "Shattered Coast", Package: "Chronicle Sync",
		Running: "2.8.0", Ready: "2.9.0", Versions: []string{"2.9.0", "2.8.0"},
		NotesURL: "https://github.com/x/y/releases/tag/2.9.0",
	}
}

func TestAppsUpdateRow(t *testing.T) {
	held := readyView()
	held.AdminHold = true
	held.Versions = nil
	current := readyView()
	current.Ready, current.NotesURL = "", ""
	noNotes := readyView()
	noNotes.NotesURL = ""

	cases := []struct {
		name    string
		v       *OwnerUpdateView
		want    []string
		notWant []string
	}{
		{
			name: "update waiting",
			v:    readyView(),
			want: []string{"Running <span class=\"tabular-nums\">2.8.0</span>", "2.9.0 is ready.", "Update to 2.9.0",
				"What&rsquo;s new", "https://github.com/x/y/releases/tag/2.9.0", "Use another version&hellip;",
				`<option value="2.8.0" selected>`, `<option value="2.9.0">`, "When a new version comes out",
				`hx-post="/campaigns/c1/foundry-vtt/update/mode"`, "Choosing this moves your world to 2.9.0 now."},
		},
		{
			name:    "no release notes link is omitted",
			v:       noNotes,
			want:    []string{"Update to 2.9.0", "Use another version"},
			notWant: []string{"What&rsquo;s new"},
		},
		{
			name:    "up to date",
			v:       current,
			want:    []string{"Up to date.", "Use another version"},
			notWant: []string{"Update to", "is ready."},
		},
		{
			name:    "held by the admin replaces both buttons with one line",
			v:       held,
			want:    []string{"The site admin is keeping this campaign on 2.8.0, so updates can't be changed here for now."},
			notWant: []string{"Update to", "Use another version", "<dialog", "When a new version comes out"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderHTML(t, AppsUpdateRow(tc.v, "tok"))
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q in\n%s", w, html)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(html, w) {
					t.Errorf("must not contain %q in\n%s", w, html)
				}
			}
		})
	}
	if got := renderHTML(t, AppsUpdateRow(nil, "tok")); !strings.Contains(got, "isn't installed on this Chronicle site yet") {
		t.Errorf("no module must say so, got %q", got)
	}
}

// Both screens are swapped in by HTMX, so every control is an inline IIFE or an
// hx- attribute: no script tag may ride along.
func TestUpdateScreensAreSwapSafe(t *testing.T) {
	for name, c := range map[string]templ.Component{
		"row":    AppsUpdateRow(readyView(), "tok"),
		"banner": CampaignUpdateBanner(readyView(), "tok"),
	} {
		html := renderHTML(t, c)
		if strings.Contains(html, "<script") || strings.Contains(html, "__templ_") {
			t.Errorf("%s: a swapped fragment must not carry a script: %s", name, html)
		}
		if !strings.Contains(html, "onclick=\"(function(){var d=document.getElementById('fvtt-update-dlg-") {
			t.Errorf("%s: the Update button must open its dialog with an inline IIFE: %s", name, html)
		}
		if !strings.Contains(html, `hx-headers="{&#34;X-CSRF-Token&#34;:&#34;tok&#34;}"`) {
			t.Errorf("%s: forms must carry the CSRF token: %s", name, html)
		}
	}
}

func TestUpdateConfirmDialogNamesTheShownVersion(t *testing.T) {
	html := renderHTML(t, updateConfirmDialog(readyView(), updateViewBanner, "tok"))
	for _, w := range []string{
		"Update Chronicle Sync for Shattered Coast?",
		`name="version" value="2.9.0"`,
		`hx-post="/campaigns/c1/foundry-vtt/update"`,
		`hx-target="#fvtt-update-line"`,
		"Use another version on the Foundry page takes you back",
	} {
		if !strings.Contains(html, w) {
			t.Errorf("missing %q in\n%s", w, html)
		}
	}
}

func TestUpdateSwitchDialogOffersOnlyListedVersions(t *testing.T) {
	v := readyView()
	v.Versions = []string{"2.9.0", "2.8.0", "2.7.4"}
	html := renderHTML(t, updateSwitchDialog(v, "tok"))
	if n := strings.Count(html, "<option"); n != 3 {
		t.Errorf("want one option per installed version, got %d in\n%s", n, html)
	}
	if !strings.Contains(html, `hx-post="/campaigns/c1/foundry-vtt/update/switch"`) || !strings.Contains(html, `hx-target="#fvtt-update-row"`) {
		t.Errorf("switch must post to its own action and swap the row: %s", html)
	}
}

func TestCampaignBindingAsksByDefault(t *testing.T) {
	b, _, _ := newBinding("", "")
	a, ok := b.(interface{ AsksByDefault() bool })
	if !ok || !a.AsksByDefault() {
		t.Error("the Foundry module's binding must ask by default")
	}
}

// The install card carries no pin selector and no second copy of the
// version: both are on the Foundry page's Module version card, where an
// admin hold and the confirmation apply.
func TestOwnerTabHasNoPinSelector(t *testing.T) {
	html := renderHTML(t, OwnerTabFragment(OwnerTabData{CampaignID: "c1", PackageRegistered: true, InstallURL: "https://x", CurrentVersion: "2.8.0"}))
	if strings.Contains(html, "fvtt-pin-selector") || strings.Contains(html, "Save Pin") {
		t.Error("the pin selector must be gone")
	}
	if strings.Contains(html, "Currently Serving") {
		t.Errorf("the install card must not repeat the version: %s", html)
	}
}

// The update choice marks the campaign's own mode, so the owner can see
// whether updates are automatic.
func TestUpdateModeChooserMarksCurrentMode(t *testing.T) {
	for _, mode := range []string{"approve_first", "automatic", "pinned"} {
		v := readyView()
		v.Mode = mode
		html := renderHTML(t, updateModeChooser(v, "tok"))
		if n := strings.Count(html, " checked"); n != 1 {
			t.Errorf("%s: want exactly one checked choice, got %d", mode, n)
		}
		if !strings.Contains(html, `value="`+mode+`" checked`) {
			t.Errorf("%s: the current mode must be checked:\n%s", mode, html)
		}
		if !strings.Contains(html, "X-CSRF-Token") {
			t.Errorf("%s: the choice must carry the CSRF token", mode)
		}
	}
}
