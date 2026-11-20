// desk_test.go pins the desk's rows and its provider seam: every row is built
// without a database or a socket, a failing, panicking or stuck provider only
// turns its own row's figure into "Open", and a lower role is refused.

package campaigns

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// vals is a row's values as one comparable string: "Text|Sub" per value,
// joined by " / ".
func vals(l DeskLine) string {
	var out []string
	for _, v := range l.Values {
		switch {
		case v.Blank != nil:
			out = append(out, "blank:"+v.Blank.Shown)
		default:
			out = append(out, v.Text+"|"+v.Sub)
		}
	}
	return strings.Join(out, " / ")
}

func hrefOf(l DeskLine) string {
	if len(l.Values) == 0 {
		return ""
	}
	return l.Values[0].Href
}

// Every row's name, value and destination for each state it can be in,
// including the plural forms and the "figure could not be read" fallback.
func TestDeskRows_NameAndValue(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	two := now.Add(-2 * time.Hour)
	old := now.Add(-24 * 24 * time.Hour)
	team := func(n int) []CampaignMember {
		out := []CampaignMember{{Role: RoleOwner}}
		for i := 1; i < n; i++ {
			out = append(out, CampaignMember{Role: RolePlayer})
		}
		return out
	}
	tests := []struct {
		name  string
		line  DeskLine
		row   string
		value string
		href  string
		state string
	}{
		{"members unread", peopleLine(c, nil), "Members", "Open|", "/campaigns/c1/members", ""},
		{"members alone", peopleLine(c, team(1)), "Members", "1|", "/campaigns/c1/members", ""},
		{"members five", peopleLine(c, team(5)), "Members", "5|", "/campaigns/c1/members", ""},
		{"invited unread", invitedLine(c, DeskInputs{}), "Invited", "Open|", "/campaigns/c1/members#invite-box", ""},
		{"invited none", invitedLine(c, DeskInputs{InvitedRead: true}), "Invited", "0|", "/campaigns/c1/members#invite-box", ""},
		{"invited two", invitedLine(c, DeskInputs{InvitedRead: true, Invited: 2}), "Invited", "2|", "/campaigns/c1/members#invite-box", ""},
		{"edits no provider", changesLine(c, nil), "Player edits", "Open|", "/campaigns/c1/players/changes", ""},
		{"edits none", changesLine(c, &DeskChangesFacts{}), "Player edits", "0|this week", "/campaigns/c1/players/changes", ""},
		{"edits some", changesLine(c, &DeskChangesFacts{Changes: 3, Players: 2}), "Player edits", "3|this week", "/campaigns/c1/players/changes", ""},
		{"features unread", featuresLine(c, DeskInputs{}), "Features", "Open|", "/campaigns/c1/extensions", ""},
		{"features", featuresLine(c, DeskInputs{FeaturesOn: 12, FeaturesTotal: 15}), "Features", "12 of 15|on", "/campaigns/c1/extensions", ""},
		{"media no provider", mediaLine(c, nil), "Media", "Open|", "/campaigns/c1/media", ""},
		{"media none", mediaLine(c, &DeskMediaFacts{}), "Media", "0 files|", "/campaigns/c1/media", ""},
		{"media one", mediaLine(c, &DeskMediaFacts{Files: 1, Bytes: 2048}), "Media", "1 file|2 KB", "/campaigns/c1/media", ""},
		{"media some", mediaLine(c, &DeskMediaFacts{Files: 212, Bytes: 310 << 20}), "Media", "212 files|310 MB", "/campaigns/c1/media", ""},
		{"foundry no provider", foundryLine(c, nil, now), "Foundry", "Open|", "/campaigns/c1/foundry", ""},
		{"foundry never set up", foundryLine(c, &DeskFoundryFacts{}, now), "Foundry", "Not set up|", "/campaigns/c1/foundry", ""},
		{"foundry key, no answer", foundryLine(c, &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true}}, now), "Foundry", "No answer|yet", "/campaigns/c1/foundry", ""},
		{"foundry live", foundryLine(c, &DeskFoundryFacts{Connection: FoundryConnection{Connected: true, HasKey: true, HubLastSeen: &now}}, now), "Foundry", "Connected|", "/campaigns/c1/foundry", "ok"},
		{"foundry seen", foundryLine(c, &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &two}}, now), "Foundry", "Last seen|2 hours ago", "/campaigns/c1/foundry", "ok"},
		{"foundry silent for weeks", foundryLine(c, &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &old}}, now), "Foundry", "No answer|24 days", "/campaigns/c1/foundry", "warn"},
		{"ai no provider", aiLine(c, nil), "Your AI", "Open|", "/campaigns/c1/ai", ""},
		{"ai none", aiLine(c, &DeskAIFacts{State: DeskAINone}), "Your AI", "Not connected|", "/campaigns/c1/ai", ""},
		{"ai connected", aiLine(c, &DeskAIFacts{State: DeskAIConnected, Client: "Claude", CanAsk: true}), "Your AI", "Claude|can ask", "/campaigns/c1/ai", "ok"},
		{"ai look only", aiLine(c, &DeskAIFacts{State: DeskAIConnected}), "Your AI", "Connected|can look", "/campaigns/c1/ai", "ok"},
		{"ai waiting", aiLine(c, &DeskAIFacts{State: DeskAIConnected, Client: "Claude", CanAsk: true, Waiting: 3}), "Your AI", "Claude|3 waiting", "/campaigns/c1/ai", "ok"},
		{"ai paused, unnamed", aiLine(c, &DeskAIFacts{State: DeskAIPaused}), "Your AI", "Paused|", "/campaigns/c1/ai", ""},
		{"ai paused, named", aiLine(c, &DeskAIFacts{State: DeskAIPaused, Client: "Claude"}), "Your AI", "Claude|Paused", "/campaigns/c1/ai", ""},
		{"ai ended", aiLine(c, &DeskAIFacts{State: DeskAIEnded}), "Your AI", "Ended|", "/campaigns/c1/ai", ""},
		{"keys unread", keysLine(c, nil), "Keys", "Open|", "/campaigns/c1/settings?tab=api-keys", ""},
		{"keys none", keysLine(c, &DeskKeysFacts{}), "Keys", "0|", "/campaigns/c1/settings?tab=api-keys", ""},
		{"keys some", keysLine(c, &DeskKeysFacts{Count: 3}), "Keys", "3|", "/campaigns/c1/settings?tab=api-keys", ""},
		{"activity", activityLine(c), "Activity", "Log|", "/campaigns/c1/settings?tab=activity", ""},
		{"trash no provider", trashLine(c, nil), "Trash", "Open|", "/campaigns/c1/trash", ""},
		{"trash empty", trashLine(c, &DeskTrashFacts{Days: 30}), "Trash", "Empty|", "/campaigns/c1/trash", ""},
		{"trash one", trashLine(c, &DeskTrashFacts{Items: 1, Days: 30}), "Trash", "1 item|kept 30 days", "/campaigns/c1/trash", ""},
		{"trash some", trashLine(c, &DeskTrashFacts{Items: 3, Days: 30}), "Trash", "3 items|kept 30 days", "/campaigns/c1/trash", ""},
		{"trash one day", trashLine(c, &DeskTrashFacts{Items: 2, Days: 1}), "Trash", "2 items|kept 1 day", "/campaigns/c1/trash", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := tt.line
			if l.Name != tt.row || vals(l) != tt.value || hrefOf(l) != tt.href || l.Values[0].State != tt.state {
				t.Errorf("row = %q / %q / %q / %q, want %q / %q / %q / %q", l.Name, vals(l), hrefOf(l), l.Values[0].State, tt.row, tt.value, tt.href, tt.state)
			}
			if len(l.Values) != 1 || l.Values[0].Word {
				t.Errorf("a figure is one value that is not a quiet word: %+v", l.Values)
			}
			// The model can say only names and values: a row never carries a
			// phrase, a placeholder number or a dash.
			for _, v := range l.Values {
				for _, bad := range []string{"—", "–", "N/A", "GM Only", "dm_only", "The table", "  "} {
					if strings.Contains(v.Text+v.Sub, bad) {
						t.Errorf("value says %q", bad)
					}
				}
			}
		})
	}
}

// Every figure that opens a page ends in a chevron; a quiet word does not.
// The missing-figure fallback is "Open" with the chevron.
func TestDeskValues_ChevronOnFiguresOnly(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	render := func(l DeskLine) string {
		var sb strings.Builder
		if err := deskRow(l).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}
	if h := render(changesLine(c, nil)); !strings.Contains(h, `>Open<`) || !strings.Contains(h, "set-chev") || !strings.Contains(h, `aria-label="Player edits: Open"`) {
		t.Errorf("the fallback is Open with a chevron: %s", h)
	}
	if h := render(changesLine(c, &DeskChangesFacts{Changes: 3})); !strings.Contains(h, `aria-label="Player edits: 3 this week"`) || !strings.Contains(h, `class="set-sub">this week`) || !strings.Contains(h, "set-chev") {
		t.Errorf("a figure draws its muted second part and a chevron: %s", h)
	}
	if h := render(copyLine(c)); strings.Contains(h, "set-chev") || strings.Count(h, "set-word") != 2 || !strings.Contains(h, `aria-label="Copy: Export…"`) || !strings.Contains(h, `aria-label="Copy: Import…"`) {
		t.Errorf("quiet words carry no chevron: %s", h)
	}
	if h := render(foundryLine(c, &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: ptrTime(time.Now().Add(-30 * 24 * time.Hour))}}, time.Now())); !strings.Contains(h, "desk-dot warn") || !strings.Contains(h, "warning, No answer") {
		t.Errorf("a state dot shows state and is said in words: %s", h)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// The Customize block says what is chosen now in a DM's words: the look by
// its starter's name, the colour by name with its swatch and never a hex
// code, the page style by its kit; then each area in Customize's order, a
// plugin's area with the words that plugin gives, and the person's brush.
func TestDeskRows_Customize(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	parent := 1
	cats := []CustomizeCategory{{ID: 1, Name: "Character"}, {ID: 2, Name: "Location"}, {ID: 3, ParentID: &parent}, {ID: 8, DMOnly: true}}
	tests := []struct {
		name     string
		line     DeskLine
		wantName string
		wantVals string
		wantHref string
	}{
		{"look", lookLine(c, LookSummary{Look: "Parchment", Colour: "Sienna", Pages: "Sketchbook pages"}), "Look", "Parchment|Sienna · Sketchbook pages", "/campaigns/c1/customize/look"},
		{"look in a colour of the owner's own", lookLine(c, LookSummary{Look: "Custom", Pages: "Standard pages"}), "Look", "Custom|Standard pages", "/campaigns/c1/customize/look"},
		{"categories unread", categoriesLine(c, DeskInputs{}), "Categories", "Open|", "/campaigns/c1/customize/categories"},
		{"categories, kinds not counted again", categoriesLine(c, DeskInputs{Categories: cats, CategoriesRead: true}), "Categories", "3|1 hidden from players", "/campaigns/c1/customize/categories"},
		{"categories none hidden", categoriesLine(c, DeskInputs{Categories: cats[:2], CategoriesRead: true}), "Categories", "2|", "/campaigns/c1/customize/categories"},
		{"homes", homesLine(c, DeskInputs{GMTerm: "GM"}), "Home pages", "Everyone|no player home yet", "/campaigns/c1/customize/home"},
		{"a plugin's area with words", tabLine(c, DeskTab{ID: "dice", Label: "Dice", Summary: CustomizeSummary{Text: "Jewelled, onyx", Sub: "players pick their own"}}), "Dice", "Jewelled, onyx|players pick their own", "/campaigns/c1/customize/x/dice"},
		{"a plugin's area without words", tabLine(c, DeskTab{ID: "maps", Label: "Maps"}), "Maps", "|", "/campaigns/c1/customize/x/maps"},
		{"brush shown", brushLine(false), "Brush in the header", "blank:Shown", ""},
		{"brush hidden", brushLine(true), "Brush in the header", "blank:Hidden", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.line.Name != tt.wantName || vals(tt.line) != tt.wantVals || hrefOf(tt.line) != tt.wantHref {
				t.Errorf("row = %+v, want %s %q %s", tt.line, tt.wantName, tt.wantVals, tt.wantHref)
			}
			for _, v := range tt.line.Values {
				if strings.Contains(v.Text+v.Sub, "#") || strings.Contains(v.Text+v.Sub, " kit") {
					t.Errorf("row speaks in codes: %+v", v)
				}
			}
		})
	}
	b := brushLine(false).Values[0].Blank
	if b.URL != "/account/view-prefs" || b.Field != "brush" || b.Off != "" {
		t.Errorf("the brush saves to the person's own display choices, never off: %+v", b)
	}
	if v := lookLine(c, LookSummary{Look: "Classic", Colour: "Indigo"}).Values[0]; !v.SubSwatch || !v.Joined {
		t.Errorf("the look row draws the colour's swatch beside its name: %+v", v)
	}
	order := customizeLines(c, DeskInputs{Tabs: []DeskTab{{ID: "maps", Label: "Maps"}, {ID: "dice", Label: "Dice"}}})
	var ids []string
	for _, l := range order {
		ids = append(ids, l.ID)
	}
	if got := strings.Join(ids, ","); got != "look,categories,homes,x-maps,x-dice,brush" {
		t.Errorf("Customize block order = %s", got)
	}
}

func TestDeskRows_System(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	list := []DeskSystem{{ID: "drawsteel", Name: "Draw Steel"}, {ID: "dnd5e", Name: "D&D 5e"}}
	now := time.Now()
	archived := &Campaign{ID: "c1", Settings: "{}", ArchivedAt: &now}
	tests := []struct {
		name      string
		c         *Campaign
		in        DeskInputs
		wantBlank string // the blank's shown word, or "" for a figure
		wantFig   string
		wantOpts  []string
		wantOff   bool
	}{
		{"none chosen", c, DeskInputs{Systems: list}, "None", "", []string{"None", "Draw Steel", "D&D 5e"}, false},
		{"one chosen", c, DeskInputs{Systems: list, SystemID: "drawsteel", SystemName: "Draw Steel"}, "Draw Steel", "", []string{"None", "Draw Steel", "D&D 5e"}, false},
		{"archived is off", archived, DeskInputs{Systems: list, SystemID: "dnd5e", SystemName: "D&D 5e"}, "D&D 5e", "", nil, true},
		{"custom system is a figure", c, DeskInputs{Systems: list, SystemID: "custom:https://x"}, "", "Custom", nil, false},
		{"unlisted system with a name", c, DeskInputs{Systems: list, SystemID: "old", SystemName: "Old Game"}, "", "Old Game", nil, false},
		{"no lister, none chosen", c, DeskInputs{}, "", "None", nil, false},
		{"no lister, one set", c, DeskInputs{SystemID: "custom:x"}, "", "Custom", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := systemLine(tt.c, tt.in)
			v := l.Values[0]
			if l.Name != "System" {
				t.Errorf("name = %q", l.Name)
			}
			if tt.wantBlank == "" {
				if v.Blank != nil || v.Text != tt.wantFig || v.Href != "/campaigns/c1/extensions" {
					t.Fatalf("want the figure %q opening the features page, got %+v", tt.wantFig, v)
				}
				return
			}
			b := v.Blank
			if b == nil || b.Shown != tt.wantBlank || b.URL != "/campaigns/c1/system" || b.Method != "PUT" || b.Field != "system_id" || (b.Off != "") != tt.wantOff {
				t.Fatalf("blank = %+v", b)
			}
			if tt.wantOpts != nil {
				var got []string
				for _, o := range b.Options {
					got = append(got, o.Label)
				}
				if strings.Join(got, ",") != strings.Join(tt.wantOpts, ",") || b.Options[0].Value != "" {
					t.Errorf("options = %v, want %v with None first and empty", got, tt.wantOpts)
				}
			}
		})
	}
}

func TestDeskRows_Danger(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	now := time.Now()
	archived := &Campaign{ID: "c1", Settings: "{}", ArchivedAt: &now}
	tests := []struct {
		name string
		c    *Campaign
		want []string
	}{
		{"active", c, []string{"Archive…", "Transfer…", "Delete…"}},
		{"archived", archived, []string{"Unarchive…", "Transfer…", "Delete…"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := endLine(tt.c)
			var got []string
			for _, v := range l.Values {
				got = append(got, v.Text)
				if !v.Word || v.Href == "" || v.Blank != nil || v.View != nil {
					t.Errorf("%q must be a quiet word that only leads on: nothing here acts in one press", v.Text)
				}
			}
			if l.ID != "end" || l.Name != "Campaign" || strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("row = %q %v, want %v", l.Name, got, tt.want)
			}
			hrefs := []string{"/campaigns/c1/settings#danger", "/campaigns/c1/members#transfer", "/campaigns/c1/settings#danger"}
			for i, v := range l.Values {
				if v.Href != hrefs[i] {
					t.Errorf("%q leads to %q, want %q", v.Text, v.Href, hrefs[i])
				}
				if v.Danger != (v.Text == "Delete…") {
					t.Errorf("%q danger = %v: only Delete is in the state red", v.Text, v.Danger)
				}
			}
		})
	}
}

func TestDeskRows_PreviewPostsTheViewAsForm(t *testing.T) {
	tests := []struct {
		name      string
		c         *Campaign
		wantGuest string // the guest word's off reason ("" when it works)
	}{
		{"public campaign", &Campaign{ID: "c", IsPublic: true, Settings: "{}"}, ""},
		{"private campaign", &Campaign{ID: "c", Settings: "{}"}, "Guests can't open a private campaign. Make it Public first."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := previewLine(tt.c)
			if l.ID != "preview" || l.Name != "Preview" || len(l.Values) != 2 {
				t.Fatalf("row = %+v", l)
			}
			player, guest := l.Values[0], l.Values[1]
			if player.Text != "As a player" || player.View == nil || player.View.As != "player" || player.View.Next != "home" || player.Off != "" || !player.Word {
				t.Errorf("player word = %+v", player)
			}
			if guest.Text != "As a guest" || guest.View == nil || guest.View.As != "guest" || guest.View.Next != "home" || guest.Off != tt.wantGuest || !guest.Word || !guest.NeedsPublic {
				t.Errorf("guest word = %+v", guest)
			}
			var sb strings.Builder
			if err := deskRow(l).Render(layouts.SetCSRFToken(context.Background(), "tok"), &sb); err != nil {
				t.Fatal(err)
			}
			html := sb.String()
			for _, want := range []string{`method="POST"`, `/view-as"`, `name="as" value="player"`, `name="as" value="guest"`, `name="next" value="home"`, `name="csrf_token"`, `type="submit"`, `id="line-preview"`,
				`aria-label="Preview: As a player"`, `aria-label="Preview: As a guest"`, `data-needs-public`, `onclick=`} {
				if !strings.Contains(html, want) {
					t.Errorf("form is missing %q: %s", want, html)
				}
			}
			if tt.wantGuest == "" {
				// A working word has no reason attached to it, and the reason
				// stays hidden until the campaign turns private.
				if strings.Contains(html, `aria-disabled="true"`) || strings.Contains(html, "aria-describedby") || !strings.Contains(html, `class="set-why" id="why-preview" hidden`) {
					t.Errorf("public: nothing is off: %s", html)
				}
				return
			}
			// An off word stays a focusable submit button (a screen reader
			// still finds it) that says why, to the pointer and to the reader.
			for _, want := range []string{`aria-disabled="true"`, `title="Guests can&#39;t open a private campaign. Make it Public first."`, `aria-describedby="why-preview"`, `id="why-preview"`} {
				if !strings.Contains(html, want) {
					t.Errorf("off guest word is missing %q: %s", want, html)
				}
			}
			if strings.Contains(html, `id="why-preview" hidden`) {
				t.Errorf("the reason must show while the word is off: %s", html)
			}
		})
	}
}

func TestDeskRows_ChangeInPlace(t *testing.T) {
	tests := []struct {
		name      string
		c         *Campaign
		wantVis   string
		wantPub   string
		wantShown string
	}{
		{"shown and private", &Campaign{ID: "c", Name: "Therin", Settings: "{}"}, "Shown", "Private", "Therin"},
		{"hidden and public", &Campaign{ID: "c", Name: "Therin", IsPublic: true, Settings: `{"default_visibility":"dm_only"}`}, "Hidden", "Public", "Therin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vis, pub, name := pagesLine(tt.c), publicLine(tt.c), nameLine(tt.c)
			if vis.Name != "New pages" || pub.Name != "Campaign" || name.Name != "Name" {
				t.Errorf("names = %q / %q / %q", vis.Name, pub.Name, name.Name)
			}
			vb, pb, nb := vis.Values[0].Blank, pub.Values[0].Blank, name.Values[0].Blank
			if vb == nil || pb == nil || nb == nil {
				t.Fatal("all three rows change in place")
			}
			if vb.Shown != tt.wantVis || pb.Shown != tt.wantPub || nb.Shown != tt.wantShown {
				t.Errorf("got %q / %q / %q", vb.Shown, pb.Shown, nb.Shown)
			}
			if vb.URL != "/campaigns/c/default-visibility" || pb.URL != "/campaigns/c/details" || nb.URL != "/campaigns/c/details" || !pb.Bool {
				t.Errorf("blanks must save through their own routes, got %q, %q and %q", vb.URL, pb.URL, nb.URL)
			}
			if vb.Kind != DeskBlankChoice || pb.Kind != DeskBlankChoice || nb.Kind != DeskBlankName || nb.Max != 200 {
				t.Errorf("kinds = %q / %q / %q (max %d)", vb.Kind, pb.Kind, nb.Kind, nb.Max)
			}
			// A pick shows exactly the word the menu offered, and each menu
			// row says what it means under its label.
			for _, b := range []*DeskBlank{vb, pb} {
				found := false
				for _, o := range b.Options {
					found = found || o.Label == b.Shown
					if o.Hint == "" {
						t.Errorf("%q option %q has no hint", b.ID, o.Label)
					}
				}
				if !found {
					t.Errorf("%q shows %q, which is not one of its options %+v", b.ID, b.Shown, b.Options)
				}
			}
			wantHints := map[string]string{"Private": "Members only", "Public": "Anyone with the link", "Hidden": "Yours until you show them", "Shown": "Players see them at once"}
			for _, b := range []*DeskBlank{vb, pb} {
				for _, o := range b.Options {
					if o.Hint != wantHints[o.Label] {
						t.Errorf("hint for %q = %q", o.Label, o.Hint)
					}
				}
			}
			if vb.Off != "" || pb.Off != "" || nb.Off != "" {
				t.Errorf("an active campaign's blanks are on")
			}
		})
	}
}

func TestDeskRows_Description(t *testing.T) {
	long := "The Shattered Heroes fight to keep the last free city out of the hands of the Ash King"
	multi := "  Line one\n\nline two  "
	empty := "   "
	tests := []struct {
		name      string
		desc      *string
		wantShown string
		wantValue string
		wantEmpty bool
	}{
		{"none", nil, "None", "", true},
		{"only spaces", &empty, "None", "", true},
		{"several lines show on one", &multi, "Line one line two", "Line one\n\nline two", false},
		{"long text is kept whole for the field", &long, long, long, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := descriptionLine(&Campaign{ID: "c1", Description: tt.desc})
			b := l.Values[0].Blank
			if l.Name != "Description" || b == nil || b.Kind != DeskBlankText || b.Shown != tt.wantShown || b.Value != tt.wantValue || b.Empty != tt.wantEmpty {
				t.Fatalf("row = %+v blank = %+v", l, b)
			}
			if b.URL != "/campaigns/c1/details" || b.Method != "PUT" || b.Field != "description" || b.Max != 5000 {
				t.Errorf("must save through the details route with the description limit: %+v", b)
			}
		})
	}
	if got := deskCut("Supercalifragilisticexpialidocious and more words", 20); got != "Supercalifragilistic…" {
		t.Errorf("an over-long word is cut where it stands: %q", got)
	}
	// The page carries a bounded copy for the row; the field gets the whole.
	huge := strings.Repeat("word ", 2000)
	if b := descriptionLine(&Campaign{ID: "c1", Description: &huge}).Values[0].Blank; len([]rune(b.Shown)) > deskDescriptionShown+1 {
		t.Errorf("row text is %d characters", len([]rune(b.Shown)))
	}
}

func TestDeskBlanks_OffWhenArchived(t *testing.T) {
	now := time.Now()
	c := &Campaign{ID: "c", Settings: "{}", ArchivedAt: &now}
	in := DeskInputs{Systems: []DeskSystem{{ID: "a", Name: "A"}}}
	for _, l := range []DeskLine{nameLine(c), descriptionLine(c), pagesLine(c), publicLine(c), systemLine(c, in)} {
		b := l.Values[0].Blank
		if b == nil || b.Off != "Unarchive the campaign to change this." {
			t.Errorf("row %q must be off on an archived campaign with its reason", l.ID)
		}
		var sb strings.Builder
		if err := deskRow(l).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`aria-disabled="true"`, `tabindex="-1"`, `title="Unarchive the campaign to change this."`, `aria-describedby="why-` + l.ID + `"`} {
			if !strings.Contains(sb.String(), want) {
				t.Errorf("%s: archived blank lacks %q", l.ID, want)
			}
		}
	}
}

func TestNeedsYou(t *testing.T) {
	c := &Campaign{ID: "c1"}
	now := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	old := now.Add(-10 * 24 * time.Hour)
	recent := now.Add(-2 * 24 * time.Hour)
	tests := []struct {
		name string
		f    DeskFacts
		want int
	}{
		{"nothing measured", DeskFacts{}, 0},
		{"foundry quiet a few days", DeskFacts{Foundry: &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &recent}}}, 0},
		{"foundry silent over a week", DeskFacts{Foundry: &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &old}}}, 1},
		{"foundry connected", DeskFacts{Foundry: &DeskFoundryFacts{Connection: FoundryConnection{Connected: true, HasKey: true, KeyLastUsed: &old}}}, 0},
		{"ai waiting", DeskFacts{AI: &DeskAIFacts{State: DeskAIConnected, Waiting: 2}}, 1},
		{"both", DeskFacts{AI: &DeskAIFacts{Waiting: 1}, Foundry: &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &old}}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(needsYou(c, tt.f, now)); got != tt.want {
				t.Errorf("rows = %d, want %d", got, tt.want)
			}
		})
	}
}

// stubProvider is a desk provider whose Facts is a function.
type stubProvider struct {
	slot DeskSlot
	fn   func(ctx context.Context) (DeskFacts, error)
}

func (s stubProvider) Slot() DeskSlot { return s.slot }
func (s stubProvider) Facts(ctx context.Context, _ DeskRequest) (DeskFacts, error) {
	return s.fn(ctx)
}

func lineByID(d *Desk, id string) *DeskLine {
	for _, col := range [][]DeskGroup{d.Left, d.Right} {
		for gi := range col {
			for li := range col[gi].Lines {
				if col[gi].Lines[li].ID == id {
					return &col[gi].Lines[li]
				}
			}
		}
	}
	return nil
}

func TestDeskService_Providers(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	in := DeskInputs{Campaign: c, Addons: map[string]bool{"media-gallery": true, "sync-api": true}}
	good := stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Trash: &DeskTrashFacts{Items: 3, Days: 30}}, nil
	}}
	tests := []struct {
		name      string
		bad       DeskProvider
		wantMedia string
	}{
		{"no provider", nil, "Open|"},
		{"provider errors", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) { return DeskFacts{}, errors.New("db down") }}, "Open|"},
		{"provider panics", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) { panic("boom") }}, "Open|"},
		{"provider fills another slot's pointer", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) {
			return DeskFacts{Trash: &DeskTrashFacts{Items: 99}}, nil
		}}, "Open|"},
		{"provider works", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) {
			return DeskFacts{Media: &DeskMediaFacts{Files: 1, Bytes: 10}}, nil
		}}, "1 file|10 B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewDeskService()
			s.Register(good)
			s.Register(nil) // a nil provider is ignored
			if tt.bad != nil {
				s.Register(tt.bad)
			}
			d := s.Build(context.Background(), DeskRequest{CampaignID: "c1"}, in)
			if got := vals(*lineByID(d, "media")); got != tt.wantMedia {
				t.Errorf("media = %q, want %q", got, tt.wantMedia)
			}
			if got := vals(*lineByID(d, "trash")); got != "3 items|kept 30 days" {
				t.Errorf("a failing neighbour changed the trash row: %q", got)
			}
		})
	}
}

func TestDeskService_SameSlotReplaces(t *testing.T) {
	s := NewDeskService()
	mk := func(n int) DeskProvider {
		return stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
			return DeskFacts{Trash: &DeskTrashFacts{Items: n, Days: 30}}, nil
		}}
	}
	s.Register(mk(1))
	s.Register(mk(2))
	d := s.Build(context.Background(), DeskRequest{}, DeskInputs{Campaign: &Campaign{ID: "c", Settings: "{}"}})
	if got := vals(*lineByID(d, "trash")); got != "2 items|kept 30 days" {
		t.Errorf("second provider must replace the first, got %q", got)
	}
}

// The Keys count comes from its own provider slot and degrades to Open.
func TestDeskService_KeysProvider(t *testing.T) {
	in := DeskInputs{Campaign: &Campaign{ID: "c", Settings: "{}"}}
	s := NewDeskService()
	s.Register(stubProvider{DeskKeys, func(context.Context) (DeskFacts, error) { return DeskFacts{Keys: &DeskKeysFacts{Count: 2}}, nil }})
	if got := vals(*lineByID(s.Build(context.Background(), DeskRequest{}, in), "keys")); got != "2|" {
		t.Errorf("keys = %q", got)
	}
	s.Register(stubProvider{DeskKeys, func(context.Context) (DeskFacts, error) { return DeskFacts{}, errors.New("db down") }})
	if got := vals(*lineByID(s.Build(context.Background(), DeskRequest{}, in), "keys")); got != "Open|" {
		t.Errorf("a failed keys provider must leave Open, got %q", got)
	}
}

// A provider that ignores its context must not hold the desk shut.
func TestCallProvider_StuckProviderIsAbandoned(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	p := stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) { <-release; return DeskFacts{}, nil }}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := callProvider(ctx, p, DeskRequest{}); err == nil {
		t.Fatal("a stuck provider must come back as an error")
	}
	if time.Since(start) > time.Second {
		t.Errorf("desk waited %v on a stuck provider", time.Since(start))
	}
}

func TestDeskService_DisabledFoundryDropsLine(t *testing.T) {
	s := NewDeskService()
	s.Register(stubProvider{DeskFoundry, func(context.Context) (DeskFacts, error) {
		old := time.Now().Add(-30 * 24 * time.Hour)
		return DeskFacts{Foundry: &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &old}}}, nil
	}})
	d := s.Build(context.Background(), DeskRequest{Now: time.Now()}, DeskInputs{Campaign: &Campaign{ID: "c", Settings: "{}"}, Addons: map[string]bool{}})
	if lineByID(d, "foundry") != nil || len(d.Needs) != 0 {
		t.Error("with Foundry sync off the desk must neither draw nor flag Foundry")
	}
}

// deskStubService answers only what the desk handler reads.
type deskStubService struct{ CampaignService }

func (deskStubService) ListMembers(context.Context, string) ([]CampaignMember, error) {
	return nil, nil
}

func deskRequestAs(h *Handler, role Role, dmGranted bool) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/dashboard", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	c.Set(contextKeyCampaign, &CampaignContext{
		Campaign: &Campaign{ID: "camp-1", Name: "Therin", Settings: "{}"}, MemberRole: role, IsDmGranted: dmGranted, IsMember: true,
	})
	auth.SetSession(c, &auth.Session{UserID: "u1"})
	return rec, h.OwnerDashboard(c)
}

func TestOwnerDashboard_OnlyTheOwnerOpensTheDesk(t *testing.T) {
	tests := []struct {
		name      string
		role      Role
		dmGranted bool
		wantOpen  bool
	}{
		{"player", RolePlayer, false, false},
		{"scribe", RoleScribe, false, false},
		{"co-dm", RolePlayer, true, false},
		{"owner", RoleOwner, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(deskStubService{})
			// Providers are the owner's numbers: a lower role must never
			// reach them, so a provider that records its call proves it.
			called := false
			h.RegisterDeskProvider(stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
				called = true
				return DeskFacts{Trash: &DeskTrashFacts{Items: 7, Days: 30}}, nil
			}})
			rec, err := deskRequestAs(h, tt.role, tt.dmGranted)
			if tt.wantOpen {
				if err != nil || rec.Code != http.StatusOK {
					t.Fatalf("owner: err=%v code=%d", err, rec.Code)
				}
				body := rec.Body.String()
				// "Needs you" is drawn only when something needs the owner.
				for _, want := range []string{
					"Campaign settings", "General", "Access", "People", "Game", "Customize", "Connections", "Data", "Danger zone",
					"7 items", "kept 30 days", `href="/campaigns/camp-1/extensions"`, `id="line-pages"`, `id="line-preview"`, `id="line-end"`,
				} {
					if !strings.Contains(body, want) {
						t.Errorf("desk is missing %q", want)
					}
				}
				for _, gone := range []string{"What players and guests see", "Look and game", "Safety net", "Archive or delete", `id="line-settings"`, "manage-pin", "manage-sentence"} {
					if strings.Contains(body, gone) {
						t.Errorf("the sheet no longer carries %q", gone)
					}
				}
				if strings.Contains(body, "The table") {
					t.Error("no group may be called The table: that is the dice sheet's name")
				}
				if n := len(regexp.MustCompile(`\sdata-manage-find[\s>=]`).FindAllString(body, -1)); n > 1 {
					t.Errorf("the page draws at most one Find, got %d", n)
				}
				if strings.Contains(body, "settings?tab=features") {
					t.Error("desk must not link to the retired Features settings tab")
				}
				return
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
				t.Fatalf("want forbidden, got %v", err)
			}
			if called || strings.Contains(rec.Body.String(), "7 items") {
				t.Error("a lower role reached the desk's numbers")
			}
		})
	}
}

func TestUpdateDetails_PartialSemantics(t *testing.T) {
	stored := func() *Campaign {
		return &Campaign{ID: "c1", Name: "Therin", Slug: "therin", IsPublic: true, Description: strPtr("kept"), Settings: "{}"}
	}
	type write struct {
		name, slug *string
		pub        *bool
		desc       patch.Field[string]
	}
	long := strings.Repeat("x", MaxDescriptionLen+1)
	exact := strings.Repeat("x", MaxDescriptionLen)
	tests := []struct {
		name    string
		input   UpdateCampaignDetailsInput
		want    *write // nil: no write at all
		wantErr bool
	}{
		{"absent everything writes nothing", UpdateCampaignDetailsInput{}, nil, false},
		{"name only leaves public and description alone", UpdateCampaignDetailsInput{Name: patch.Of("Shattered")}, &write{name: strPtr("Shattered"), slug: strPtr("shattered")}, false},
		{"public only leaves the name and description alone", UpdateCampaignDetailsInput{IsPublic: patch.Of(false)}, &write{pub: boolPtr(false)}, false},
		{"same value writes nothing", UpdateCampaignDetailsInput{IsPublic: patch.Of(true), Name: patch.Of("Therin")}, nil, false},
		{"empty name refused", UpdateCampaignDetailsInput{Name: patch.Of("   ")}, nil, true},
		{"explicit null name preserves", UpdateCampaignDetailsInput{Name: patch.Null[string]()}, nil, false},
		{"description replaces and touches nothing else", UpdateCampaignDetailsInput{Description: patch.Of("New words")}, &write{desc: patch.Of("New words")}, false},
		{"description is trimmed", UpdateCampaignDetailsInput{Description: patch.Of("  New words \n")}, &write{desc: patch.Of("New words")}, false},
		{"multi-line description is kept as typed", UpdateCampaignDetailsInput{Description: patch.Of("One\n\nTwo")}, &write{desc: patch.Of("One\n\nTwo")}, false},
		{"empty description clears", UpdateCampaignDetailsInput{Description: patch.Of("   ")}, &write{desc: patch.Null[string]()}, false},
		{"explicit null description clears", UpdateCampaignDetailsInput{Description: patch.Null[string]()}, &write{desc: patch.Null[string]()}, false},
		{"same description writes nothing", UpdateCampaignDetailsInput{Description: patch.Of(" kept ")}, nil, false},
		{"description at the limit is saved", UpdateCampaignDetailsInput{Description: patch.Of(exact)}, &write{desc: patch.Of(exact)}, false},
		{"description over the limit refused", UpdateCampaignDetailsInput{Description: patch.Of(long)}, nil, true},
		{"description with name and public together", UpdateCampaignDetailsInput{Name: patch.Of("Shattered"), Description: patch.Of("d"), IsPublic: patch.Of(false)},
			&write{name: strPtr("Shattered"), slug: strPtr("shattered"), pub: boolPtr(false), desc: patch.Of("d")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *write
			repo := &mockCampaignRepo{
				findByIDFn: func(context.Context, string) (*Campaign, error) { return stored(), nil },
				updateDetailsFn: func(_ context.Context, _ string, n, s *string, p *bool, d patch.Field[string]) error {
					got = &write{n, s, p, d}
					return nil
				},
			}
			svc := newTestCampaignService(repo, &mockUserFinder{})
			camp, err := svc.UpdateDetails(context.Background(), "c1", tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("wrote %+v, want no write", got)
			case tt.want != nil && got == nil:
				t.Fatal("want a write, got none")
			case tt.want != nil:
				if !eqStr(got.name, tt.want.name) || !eqStr(got.slug, tt.want.slug) || (got.pub == nil) != (tt.want.pub == nil) || (got.pub != nil && *got.pub != *tt.want.pub) {
					t.Errorf("write = %+v, want %+v", got, tt.want)
				}
				// The description reaches the repository in the state the
				// caller meant: absent keeps the column, null clears it, a
				// value replaces it.
				gv, gok := got.desc.Get()
				wv, wok := tt.want.desc.Get()
				if got.desc.Present() != tt.want.desc.Present() || got.desc.IsNull() != tt.want.desc.IsNull() || gok != wok || gv != wv {
					t.Errorf("description write = present %v null %v %q, want present %v null %v %q", got.desc.Present(), got.desc.IsNull(), gv, tt.want.desc.Present(), tt.want.desc.IsNull(), wv)
				}
				// What the service answers is what was stored.
				if tt.want.desc.Present() {
					switch {
					case tt.want.desc.IsNull() && camp.Description != nil:
						t.Errorf("a cleared description answered %q", *camp.Description)
					case !tt.want.desc.IsNull() && (camp.Description == nil || *camp.Description != wv):
						t.Errorf("the answer carries %v, want %q", camp.Description, wv)
					}
				}
			}
		})
	}
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func eqStr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// deskDetailsService records what UpdateDetails was asked.
type deskDetailsService struct {
	CampaignService
	got   *UpdateCampaignDetailsInput
	after Campaign
}

func (s *deskDetailsService) UpdateDetails(_ context.Context, _ string, in UpdateCampaignDetailsInput) (*Campaign, error) {
	s.got = &in
	c := s.after
	return &c, nil
}

func TestUpdateDetailsAPI(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		body     string
		wantCode int
		wantCall bool
	}{
		{"player refused", RolePlayer, `{"isPublic":true}`, http.StatusForbidden, false},
		{"scribe refused", RoleScribe, `{"name":"x"}`, http.StatusForbidden, false},
		{"owner empty body", RoleOwner, `{}`, http.StatusBadRequest, false},
		{"owner public only", RoleOwner, `{"isPublic":false}`, http.StatusOK, true},
		{"owner name only", RoleOwner, `{"name":"New"}`, http.StatusOK, true},
		{"owner description only", RoleOwner, `{"description":"Words"}`, http.StatusOK, true},
		{"owner clears the description", RoleOwner, `{"description":null}`, http.StatusOK, true},
		{"scribe cannot change the description", RoleScribe, `{"description":"x"}`, http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &deskDetailsService{after: Campaign{ID: "camp-1", Name: "New", Description: strPtr("Words")}}
			h := NewHandler(svc)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1/details", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "camp-1", Settings: "{}"}, MemberRole: tt.role, IsMember: true})
			auth.SetSession(c, &auth.Session{UserID: "u1"})

			err := h.UpdateDetailsAPI(c)
			code := rec.Code
			var ae *apperror.AppError
			if errors.As(err, &ae) {
				code = ae.Code
			} else if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
			}
			if (svc.got != nil) != tt.wantCall {
				t.Errorf("service called = %v, want %v", svc.got != nil, tt.wantCall)
			}
			if tt.name == "owner public only" && (svc.got.Name.Present() || svc.got.Description.Present() || !svc.got.IsPublic.Present()) {
				t.Error("only the key sent may reach the service")
			}
			if tt.name == "owner description only" && (svc.got.Name.Present() || svc.got.IsPublic.Present() || !svc.got.Description.Present() || svc.got.Description.IsNull()) {
				t.Error("only the description may reach the service, as a value")
			}
			if tt.name == "owner clears the description" && !svc.got.Description.IsNull() {
				t.Error("an explicit null must reach the service as a clear")
			}
			if tt.name == "owner description only" && !strings.Contains(rec.Body.String(), `"description":"Words"`) {
				t.Errorf("the answer carries the stored description: %s", rec.Body.String())
			}
		})
	}
}

func TestDeskPage_RendersRowsNeedsYouAndBlanks(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Therin", Settings: "{}"}, MemberRole: RoleOwner}
	d := &Desk{
		CampaignName: "Therin",
		Needs: []NeedsYouItem{
			{Lead: "Foundry", Rest: "Stopped answering 9 days ago", URL: "/campaigns/camp-1/foundry", State: "warn"},
		},
		Left: []DeskGroup{
			{Name: "General", Lines: []DeskLine{nameLine(cc.Campaign), descriptionLine(cc.Campaign)}},
			{Name: "People", Lines: []DeskLine{peopleLine(cc.Campaign, nil), changesLine(cc.Campaign, &DeskChangesFacts{Changes: 3})}},
		},
		Right: []DeskGroup{
			{Name: "Data", Lines: []DeskLine{copyLine(cc.Campaign)}},
			{Name: "Danger zone", Danger: true, Lines: []DeskLine{endLine(cc.Campaign)}},
		},
	}
	// The viewer is the signed-in owner, so Find is drawn on the title's row.
	ctx := layouts.SetCampaignRole(layouts.SetCampaignID(layouts.SetIsAuthenticated(context.Background(), true), "camp-1"), 3)
	var sb strings.Builder
	if err := DeskPage(cc, d).Render(ctx, &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	for _, want := range []string{
		`data-desk-needs`, `Foundry`, `Stopped answering 9 days ago`, `href="/campaigns/camp-1/foundry"`, `desk-dot warn`,
		// Each row is a name and its value; a figure is a link ending in a chevron.
		`class="set-name" aria-hidden="true">Members`, `href="/campaigns/camp-1/members"`, `aria-label="Members: Open"`,
		`aria-label="Player edits: 3 this week"`,
		// Quiet words, the last of them in the state red.
		`aria-label="Copy: Export…"`, `class="set-v set-word set-danger"`, `aria-label="Campaign: Delete…"`,
		// A blank answers both the pointer and the keyboard from its own
		// attributes, and the Saved tick has its place on the row.
		`role="button"`, `onclick=`, `onkeydown=`, `aria-expanded="false"`, `class="desk-acts"`,
		`id="line-name"`, `id="line-description"`, `id="line-end"`, `data-desk-blank="text"`, `data-desk-blank="name"`, `data-maxlen="5000"`,
		`class="desk-b set-v is-empty set-wide"`, `aria-label="Description: None"`,
		// Find on the title's row, the one Find on the page.
		`manage-find-field`, `Find a setting`, `class="set-grp set-grp-danger"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("desk is missing %q", want)
		}
	}
	if n := len(regexp.MustCompile(`\sdata-manage-find[\s>=]`).FindAllString(html, -1)); n != 1 {
		t.Errorf("the page draws one Find, got %d", n)
	}
	// Find sits inside the title's header, not under it.
	head := strings.Index(html, `class="manage-head"`)
	if f, end := strings.Index(html, "manage-find-field"), head+strings.Index(html[head:], "</header>"); head < 0 || f < head || f > end {
		t.Error("Find belongs on the title's row")
	}
	if regexp.MustCompile(`<button[^>]*desk-b`).MatchString(html) {
		t.Error("a blank must not be a real button: buttons cannot wrap across lines")
	}
	// Left column, then right, in reading order, so Tab follows it.
	order := []string{`>General<`, `>People<`, `>Data<`, `>Danger zone<`}
	last := -1
	for _, w := range order {
		at := strings.Index(html, w)
		if at < 0 || at < last {
			t.Errorf("%s is missing or out of order", w)
		}
		last = at
	}
	for _, gone := range []string{"desk-chev", "data-fold", "data-carry", "Open People", "manage-pin", `class="desk-s"`} {
		if strings.Contains(html, gone) {
			t.Errorf("the rows no longer carry %q", gone)
		}
	}
}

// A choice carries the down-chevron; a name or a description, which open a
// field, do not. The word is in its own span so changing it keeps the chevron.
func TestDeskBlank_ChevronOnChoicesOnly(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	render := func(l DeskLine) string {
		var sb strings.Builder
		if err := deskRow(l).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}
	if h := render(publicLine(c)); !strings.Contains(h, "set-chev-down") || !strings.Contains(h, `<span class="desk-t">Private</span>`) || !strings.Contains(h, `aria-label="Campaign: Private"`) || !strings.Contains(h, `aria-haspopup="menu"`) {
		t.Errorf("choice carries the chevron-down and a name that says the row and the value: %s", h)
	}
	for _, l := range []DeskLine{nameLine(c), descriptionLine(c)} {
		if h := render(l); strings.Contains(h, "set-chev") || strings.Contains(h, `aria-haspopup="menu"`) {
			t.Errorf("%s opens a field, not a menu: %s", l.ID, h)
		}
	}
	// A long description is cut for the screen reader's name for the blank.
	long := strings.Repeat("word ", 100)
	if h := render(descriptionLine(&Campaign{ID: "c1", Description: &long})); strings.Contains(h, `aria-label="Description: `+strings.Repeat("word ", 30)) {
		t.Errorf("a long description must not become a button's whole name: %s", h)
	}
}

// The menu's option hints reach the script as JSON under the key it reads.
func TestDeskBlank_OptionsCarryHints(t *testing.T) {
	got := deskOptionsJSON(publicLine(&Campaign{ID: "c1", Settings: "{}"}).Values[0].Blank.Options)
	for _, want := range []string{`"v":"false"`, `"l":"Private"`, `"h":"Members only"`, `"h":"Anyone with the link"`} {
		if !strings.Contains(got, want) {
			t.Errorf("options JSON lacks %q: %s", want, got)
		}
	}
	if got := deskOptionsJSON([]DeskOption{{Value: "", Label: "None"}}); strings.Contains(got, `"h"`) {
		t.Errorf("an option with no hint carries none: %s", got)
	}
}

// The group names and their order, and which rows each holds, with the add-on
// rows present only while their add-on is on. Row ids matter: links point at
// "#line-<id>".
func TestDeskService_Build_GroupsAndRows(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	shape := func(cols ...[]DeskGroup) string {
		var parts []string
		for _, col := range cols {
			var groups []string
			for _, g := range col {
				var ids []string
				for _, l := range g.Lines {
					ids = append(ids, l.ID)
				}
				groups = append(groups, g.Name+"["+strings.Join(ids, ",")+"]")
			}
			parts = append(parts, strings.Join(groups, " "))
		}
		return strings.Join(parts, " || ")
	}
	tests := []struct {
		name   string
		addons map[string]bool
		want   string
	}{
		{"add-ons off", map[string]bool{},
			"General[name,description] Access[public,pages,preview] People[people,invited,changes] Game[system,features]" +
				" || Customize[look,categories,homes,brush] Connections[ai,keys] Data[copy,activity,trash] Danger zone[end]"},
		{"media and Foundry on", map[string]bool{"media-gallery": true, "sync-api": true},
			"General[name,description] Access[public,pages,preview] People[people,invited,changes] Game[system,features]" +
				" || Customize[look,categories,homes,brush] Connections[foundry,ai,keys] Data[copy,activity,media,trash] Danger zone[end]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No provider answers: every figure row still draws, as Open.
			d := NewDeskService().Build(context.Background(), DeskRequest{Now: time.Now()}, DeskInputs{Campaign: c, Addons: tt.addons})
			if got := shape(d.Left, d.Right); got != tt.want {
				t.Errorf("sheet =\n%s\nwant\n%s", got, tt.want)
			}
			for _, id := range []string{"changes", "keys", "trash"} {
				if got := vals(*lineByID(d, id)); got != "Open|" {
					t.Errorf("%s with no provider = %q, want the word Open", id, got)
				}
			}
			if !d.Right[len(d.Right)-1].Danger {
				t.Error("the Danger zone group is the state red")
			}
			for _, col := range [][]DeskGroup{d.Left, d.Right} {
				for _, g := range col {
					if g.Name == "The table" {
						t.Error("a group is called The table, the dice sheet's name")
					}
					if g.Danger != (g.Name == "Danger zone") {
						t.Errorf("group %q danger = %v", g.Name, g.Danger)
					}
				}
			}
		})
	}
}

// updateService records what Update was asked.
type updateService struct {
	CampaignService
	got *UpdateCampaignInput
}

func (s *updateService) Update(_ context.Context, _ string, in UpdateCampaignInput) (*Campaign, error) {
	s.got = &in
	return &Campaign{ID: "camp-1"}, nil
}

// The Details tab posts only the description. The handler must hand the service
// a description and nothing else, so the name and the public flag stay as they
// are; a body that names them still reaches the service.
func TestUpdate_FormBodyKeepsAbsentKeysAbsent(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantName   bool
		wantPublic bool
	}{
		{"description only", "description=New+words", false, false},
		{"old full form", "name=Therin&description=x&is_public=true", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &updateService{}
			h := NewHandler(svc)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "camp-1", Settings: "{}"}, MemberRole: RoleOwner, IsMember: true})
			auth.SetSession(c, &auth.Session{UserID: "u1"})
			if err := h.Update(c); err != nil {
				t.Fatalf("update: %v", err)
			}
			if svc.got == nil || svc.got.Description == nil {
				t.Fatalf("the description must reach the service: %+v", svc.got)
			}
			if (svc.got.Name != nil) != tt.wantName || (svc.got.IsPublic != nil) != tt.wantPublic {
				t.Errorf("name present = %v, public present = %v", svc.got.Name != nil, svc.got.IsPublic != nil)
			}
		})
	}
}
