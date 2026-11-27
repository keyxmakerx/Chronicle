// desk_test.go pins the Overview board and the desk's provider seam: every
// tile is built without a database or a socket, a failing, panicking or stuck
// provider only turns its own tile's figure into "Open", and a lower role is
// refused. The rows that stay (the Campaign tab's fields, Preview and the
// Customize list) are pinned here too.

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

// Every figure tile's label, figure, muted line, destination and state dot
// for each state it can be in, including the plural forms and the "figure
// could not be read" fallback, which is the word Open and never a guess.
func TestBoardTiles_FigureAndDestination(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	two := now.Add(-2 * time.Hour)
	old := now.Add(-24 * 24 * time.Hour)
	tests := []struct {
		name  string
		tile  DeskGoTile
		label string
		value string
		hint  string
		href  string
		state string
	}{
		{"system none", systemTile(c, DeskInputs{}), "Game system", "None", "Pick one on Game", "/campaigns/c1/extensions", ""},
		{"system named", systemTile(c, DeskInputs{SystemID: "drawsteel", SystemName: "Draw Steel"}), "Game system", "Draw Steel", "Its rules and tooltips are on", "/campaigns/c1/extensions", ""},
		{"system of their own", systemTile(c, DeskInputs{SystemID: "custom:https://x"}), "Game system", "Custom", "A system of your own", "/campaigns/c1/extensions", ""},
		{"edits unread", editsTile(c, nil), "Player edits", "Open", "", "/campaigns/c1/players/changes", ""},
		{"edits none", editsTile(c, &DeskChangesFacts{}), "Player edits", "0", "None this week", "/campaigns/c1/players/changes", ""},
		{"edits some", editsTile(c, &DeskChangesFacts{Changes: 3, Players: 2}), "Player edits", "3", "this week · 2 players", "/campaigns/c1/players/changes", ""},
		{"edits by one player", editsTile(c, &DeskChangesFacts{Changes: 1, Players: 1}), "Player edits", "1", "this week · 1 player", "/campaigns/c1/players/changes", ""},
		{"storage without media", storageTile(c, false, nil), "Copies", "Export", "A copy of the whole campaign, to keep or move", "/campaigns/c1/data", ""},
		{"storage unread", storageTile(c, true, nil), "Storage and copies", "Open", "Export or import a copy", "/campaigns/c1/data", ""},
		{"storage empty", storageTile(c, true, &DeskMediaFacts{}), "Storage and copies", "Empty", "No pictures or files yet · export or import a copy", "/campaigns/c1/data", ""},
		{"storage some", storageTile(c, true, &DeskMediaFacts{Files: 212, Bytes: 310 << 20}), "Storage and copies", "310 MB", "212 files · export or import a copy", "/campaigns/c1/data", ""},
		{"trash unread", trashTile(c, nil), "Trash and activity", "Open", "", "/campaigns/c1/data", ""},
		{"trash empty", trashTile(c, &DeskTrashFacts{Days: 30}), "Trash and activity", "Empty", "Nothing waiting in the Trash", "/campaigns/c1/data", ""},
		{"trash one", trashTile(c, &DeskTrashFacts{Items: 1, Days: 30}), "Trash and activity", "1 item", "in the Trash · kept 30 days", "/campaigns/c1/data", ""},
		{"trash some, one day", trashTile(c, &DeskTrashFacts{Items: 2, Days: 1}), "Trash and activity", "2 items", "in the Trash · kept 1 day", "/campaigns/c1/data", ""},
		{"foundry unread", foundryTile(c, nil, now), "Foundry", "Open", "", "/campaigns/c1/foundry", ""},
		{"foundry never set up", foundryTile(c, &DeskFoundryFacts{}, now), "Foundry", "Not set up", "", "/campaigns/c1/foundry", ""},
		{"foundry key, no answer", foundryTile(c, &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true}}, now), "Foundry", "No answer yet", "", "/campaigns/c1/foundry", ""},
		{"foundry live", foundryTile(c, &DeskFoundryFacts{Connection: FoundryConnection{Connected: true, HasKey: true, HubLastSeen: &now, ModuleVersion: "3.2.0"}}, now), "Foundry", "Connected", "Module 3.2.0", "/campaigns/c1/foundry", "ok"},
		{"foundry seen", foundryTile(c, &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &two}}, now), "Foundry", "Last seen", "2 hours ago", "/campaigns/c1/foundry", "ok"},
		{"foundry silent for weeks", foundryTile(c, &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &old}}, now), "Foundry", "No answer", "Quiet for 24 days", "/campaigns/c1/foundry", "warn"},
		{"ai unread", aiTile(c, nil), "Your AI", "Open", "", "/campaigns/c1/ai", ""},
		{"ai none", aiTile(c, &DeskAIFacts{State: DeskAINone}), "Your AI", "Not connected", "", "/campaigns/c1/ai", ""},
		{"ai connected", aiTile(c, &DeskAIFacts{State: DeskAIConnected, Client: "Claude", CanAsk: true}), "Your AI", "Claude", "can ask", "/campaigns/c1/ai", "ok"},
		{"ai look only", aiTile(c, &DeskAIFacts{State: DeskAIConnected}), "Your AI", "Connected", "can look", "/campaigns/c1/ai", "ok"},
		{"ai waiting", aiTile(c, &DeskAIFacts{State: DeskAIConnected, Client: "Claude", CanAsk: true, Waiting: 3}), "Your AI", "Claude", "3 waiting", "/campaigns/c1/ai", "ok"},
		{"ai paused", aiTile(c, &DeskAIFacts{State: DeskAIPaused, Client: "Claude"}), "Your AI", "Claude", "Paused", "/campaigns/c1/ai", ""},
		{"ai ended", aiTile(c, &DeskAIFacts{State: DeskAIEnded}), "Your AI", "Ended", "", "/campaigns/c1/ai", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := tt.tile
			if g.Label != tt.label || g.Value.Text != tt.value || g.Hint != tt.hint || g.Href != tt.href || g.Value.State != tt.state {
				t.Errorf("tile = %q / %q / %q / %q / %q, want %q / %q / %q / %q / %q",
					g.Label, g.Value.Text, g.Hint, g.Href, g.Value.State, tt.label, tt.value, tt.hint, tt.href, tt.state)
			}
			// A tile says only names and figures: never a placeholder number,
			// a dash or a code.
			for _, bad := range []string{"—", "–", "N/A", "GM Only", "dm_only", "The table", "  "} {
				if strings.Contains(g.Label+g.Value.Text+g.Hint, bad) {
					t.Errorf("tile says %q", bad)
				}
			}
		})
	}
	if got := monoOf("Draw Steel"); got != "DS" {
		t.Errorf("initials = %q", got)
	}
	if got := systemTile(c, DeskInputs{SystemName: "D&D 5e", SystemID: "dnd5e"}).Mono; got != "D5" {
		t.Errorf("the system's mark is its first two initials, got %q", got)
	}
}

// People shows a face for the first few people, owner first, with a count of
// the rest, the head count as the members page words it, and the way to
// invite. A face never shows a whole email address.
func TestBoardTiles_People(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	pic := " /media/a.png "
	team := []CampaignMember{
		{UserID: "u0", Role: RoleOwner, DisplayName: "Dana"},
		{UserID: "u1", Role: RolePlayer, Email: "kit@example.com", AvatarPath: &pic},
		{UserID: "u2", Role: RoleScribe},
	}
	for i := 3; i < 9; i++ {
		team = append(team, CampaignMember{UserID: "u" + itoa(i), Role: RolePlayer, DisplayName: "P" + itoa(i)})
	}
	tests := []struct {
		name      string
		in        DeskInputs
		wantCount string
		wantFaces int
		wantMore  int
	}{
		{"members unread", DeskInputs{}, "", 0, 0},
		{"alone", DeskInputs{Members: team[:1]}, "1 member", 1, 0},
		{"three, one invited", DeskInputs{Members: team[:3], InvitedRead: true, Invited: 1}, "3 members · 1 invited", 3, 0},
		{"more than fit", DeskInputs{Members: team}, "9 members", deskBoardFaces, 9 - deskBoardFaces},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := peopleTile(c, tt.in)
			if p.Count != tt.wantCount || len(p.Faces) != tt.wantFaces || p.More != tt.wantMore {
				t.Errorf("people = %q, %d faces, %d more; want %q, %d, %d", p.Count, len(p.Faces), p.More, tt.wantCount, tt.wantFaces, tt.wantMore)
			}
			if p.Href != "/campaigns/c1/members" || p.InviteHref != "/campaigns/c1/members#invite-box" {
				t.Errorf("links = %q / %q", p.Href, p.InviteHref)
			}
		})
	}
	p := peopleTile(c, DeskInputs{Members: team[:3]})
	if (DeskFace{Name: "Owner Ines"}).Short() != "Owner" || (DeskFace{}).Short() != "" {
		t.Error("a face's short name is the first word of its name")
	}
	if f := p.Faces[0]; f.Name != "Dana" || f.Initial() != "D" || f.Role != RoleOwner.DisplayName() {
		t.Errorf("the owner comes first by name: %+v", f)
	}
	if f := p.Faces[1]; f.Name != "kit" || f.Avatar != "/media/a.png" {
		t.Errorf("a face without a name shows the part before the @ and its picture: %+v", f)
	}
	if f := p.Faces[2]; f.Name != "Someone" || f.Initial() != "S" {
		t.Errorf("a face with neither name nor email is Someone: %+v", f)
	}
	// The tint comes from the id alone, so it is one of the five and the
	// same id always lands on the same one.
	seen := map[int]bool{}
	for _, id := range []string{"u1", "u2", "u3", "owner-7", ""} {
		h := faceHue(id)
		if h < 0 || h > 4 {
			t.Errorf("faceHue(%q) = %d, want 0 to 4", id, h)
		}
		seen[h] = true
	}
	if faceHue("u1") != 0 || len(seen) < 2 {
		t.Errorf("faceHue(u1) = %d, tints seen %v", faceHue("u1"), seen)
	}
}

// Features is a chip for every installed feature, never a game system or one
// that is not installed, each pressed while it is on; an archived campaign
// keeps the chips but says why they cannot be pressed.
func TestBoardTiles_Features(t *testing.T) {
	addons := []PluginHubAddon{
		{AddonID: 1, Name: "Maps", Description: "Pin places", Installed: true, Enabled: true},
		{AddonID: 2, Name: "Calendar", Installed: true},
		{AddonID: 3, Name: "Coming", Installed: false},
		{AddonID: 4, Name: "Draw Steel", Category: addonCategorySystem, Installed: true, Enabled: true},
	}
	c := &Campaign{ID: "c1", Settings: "{}"}
	f := featuresTile(c, DeskInputs{FeaturesOn: 1, FeaturesTotal: 2}, addons)
	var got []string
	for _, ch := range f.Chips {
		got = append(got, ch.Name+":"+boolStr(ch.On))
	}
	if strings.Join(got, ",") != "Maps:true,Calendar:false" || f.Count != "1 of 2 on" || f.Href != "/campaigns/c1/extensions" || f.Off != "" {
		t.Errorf("features = %v %q %q %q", got, f.Count, f.Href, f.Off)
	}
	if f.Chips[0].AddonID != 1 || boardChipTitle(f.Chips[0], "") != "Pin places" {
		t.Errorf("a chip carries its add-on and says what it does: %+v", f.Chips[0])
	}
	if unread := featuresTile(c, DeskInputs{}, nil); unread.Count != "" || len(unread.Chips) != 0 {
		t.Errorf("an unread list draws no count and no chips: %+v", unread)
	}
	now := time.Now()
	off := featuresTile(&Campaign{ID: "c1", Settings: "{}", ArchivedAt: &now}, DeskInputs{}, addons)
	if off.Off != deskArchivedOff || len(off.Chips) != 2 || boardChipTitle(off.Chips[0], off.Off) != deskArchivedOff {
		t.Errorf("archived chips say why they are off: %+v", off)
	}
}

// The two flips wrap the same blanks as the Campaign tab, so they save through
// the same routes; the first option is the resting state.
func TestBoardFlips(t *testing.T) {
	tests := []struct {
		name     string
		c        *Campaign
		wantPub  string
		wantPage string
		pubOn    bool
		pageOn   bool
	}{
		{"private, hidden", &Campaign{ID: "c", Settings: `{"default_visibility":"dm_only"}`}, "Private", "Hidden", false, false},
		{"public, shown", &Campaign{ID: "c", IsPublic: true, Settings: "{}"}, "Public", "Shown", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := boardOf(tt.c, DeskInputs{Campaign: tt.c, Addons: map[string]bool{}}, DeskFacts{}, nil, nil, time.Now())
			if b.Access.Now().Label != tt.wantPub || b.Access.On() != tt.pubOn || b.Pages.Now().Label != tt.wantPage || b.Pages.On() != tt.pageOn {
				t.Errorf("flips = %q %v / %q %v", b.Access.Now().Label, b.Access.On(), b.Pages.Now().Label, b.Pages.On())
			}
			if b.Access.Blank.URL != "/campaigns/c/details" || !b.Access.Blank.Bool || b.Pages.Blank.URL != "/campaigns/c/default-visibility" {
				t.Errorf("flips save through the blanks' routes: %+v / %+v", b.Access.Blank, b.Pages.Blank)
			}
		})
	}
	if (DeskFlip{}).On() || (DeskFlip{}).Now().Label != "" || (DeskFlip{}).Option(5).Label != "" {
		t.Error("a flip with no blank is resting and has no words")
	}
}

// Which tiles the board holds: Foundry only while Foundry sync is on, and the
// storage tile names only copies while Media is off. No provider answering
// leaves every figure as Open.
func TestDeskService_Build_Board(t *testing.T) {
	c := &Campaign{ID: "c1", Name: "Therin", Settings: "{}"}
	tests := []struct {
		name        string
		addons      map[string]bool
		wantFoundry bool
		wantStorage string
	}{
		{"add-ons off", map[string]bool{}, false, "Copies"},
		{"media and Foundry on", map[string]bool{"media-gallery": true, "sync-api": true}, true, "Storage and copies"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDeskService().Build(context.Background(), DeskRequest{Now: time.Now()}, DeskInputs{Campaign: c, Addons: tt.addons})
			b := d.Board
			if d.CampaignName != "Therin" || (b.Foundry != nil) != tt.wantFoundry || b.Storage.Label != tt.wantStorage {
				t.Errorf("board: name %q, foundry %v, storage %q", d.CampaignName, b.Foundry != nil, b.Storage.Label)
			}
			for _, g := range []DeskGoTile{b.Edits, b.AI, b.Trash} {
				if g.Value.Text != "Open" {
					t.Errorf("%s with no provider = %q, want the word Open", g.Label, g.Value.Text)
				}
			}
			if b.Look.Href != "/campaigns/c1/customize" || len(b.Needs) != 0 {
				t.Errorf("look leads to the Customizer and nothing needs the owner: %+v", b)
			}
		})
	}
}

// A figure that opens a page ends in a chevron; a quiet word does not. The
// missing-figure fallback is "Open" with the chevron.
func TestDeskValues_ChevronOnFiguresOnly(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	render := func(l DeskLine) string {
		var sb strings.Builder
		if err := deskRow(l).Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}
	if h := render(categoriesLine(c, DeskInputs{})); !strings.Contains(h, `>Open<`) || !strings.Contains(h, "set-chev") || !strings.Contains(h, `aria-label="Categories: Open"`) {
		t.Errorf("the fallback is Open with a chevron: %s", h)
	}
	if h := render(lookLine(c, LookSummary{Look: "Parchment", Colour: "Sienna"})); !strings.Contains(h, "set-chev") || !strings.Contains(h, `class="set-sub`) {
		t.Errorf("a figure draws its muted second part and a chevron: %s", h)
	}
	if h := render(previewLine(&Campaign{ID: "c1", IsPublic: true, Settings: "{}"})); strings.Contains(h, "set-chev") {
		t.Errorf("quiet words carry no chevron: %s", h)
	}
}

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
		{"a plugin's area without words says what it holds", tabLine(c, DeskTab{ID: "hero-creator", Label: "Hero creator", Blurb: "The border each card wears in the hero creator."}), "Hero creator", "|The border each card wears in the hero creator.", "/campaigns/c1/customize/x/hero-creator"},
		{"brush shown", brushLine("c1", false), "Header brush", "blank:Shown", ""},
		{"brush hidden", brushLine("c1", true), "Header brush", "blank:Hidden", ""},
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
	b := brushLine("c1", false).Values[0].Blank
	if b.URL != "/account/brush/c1" || b.Field != "brush" || b.Off != "" {
		t.Errorf("the brush saves to the person's own choice for this campaign, never off: %+v", b)
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
	for _, l := range []DeskLine{nameLine(c), descriptionLine(c), pagesLine(c), publicLine(c)} {
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
		{"drafts waiting", DeskFacts{Entries: &DeskEntriesFacts{Entries: 3, Drafts: 2}}, 1},
		{"asks waiting", DeskFacts{Entries: &DeskEntriesFacts{Asks: 1}}, 1},
		{"entries but no drafts", DeskFacts{Entries: &DeskEntriesFacts{Entries: 3}}, 0},
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
		{"no provider", nil, "Open"},
		{"provider errors", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) { return DeskFacts{}, errors.New("db down") }}, "Open"},
		{"provider panics", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) { panic("boom") }}, "Open"},
		{"provider fills another slot's pointer", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) {
			return DeskFacts{Trash: &DeskTrashFacts{Items: 99}}, nil
		}}, "Open"},
		{"provider works", stubProvider{DeskMedia, func(context.Context) (DeskFacts, error) {
			return DeskFacts{Media: &DeskMediaFacts{Files: 1, Bytes: 10}}, nil
		}}, "10 B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewDeskService()
			s.Register(good)
			s.Register(nil) // a nil provider is ignored
			if tt.bad != nil {
				s.Register(tt.bad)
			}
			b := s.Build(context.Background(), DeskRequest{CampaignID: "c1"}, in).Board
			if got := b.Storage.Value.Text; got != tt.wantMedia {
				t.Errorf("storage = %q, want %q", got, tt.wantMedia)
			}
			if got := b.Trash.Value.Text + "|" + b.Trash.Hint; got != "3 items|in the Trash · kept 30 days" {
				t.Errorf("a failing neighbour changed the trash tile: %q", got)
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
	if got := d.Board.Trash.Value.Text; got != "2 items" {
		t.Errorf("second provider must replace the first, got %q", got)
	}
}

// Facts asks only the named slots, so a tab reads only what it draws, and a
// call naming none asks nobody.
func TestDeskService_FactsAsksOnlyTheNamedSlots(t *testing.T) {
	s := NewDeskService()
	asked := map[DeskSlot]bool{}
	for _, slot := range []DeskSlot{DeskKeys, DeskTrash} {
		slot := slot
		s.Register(stubProvider{slot, func(context.Context) (DeskFacts, error) {
			asked[slot] = true
			return DeskFacts{Keys: &DeskKeysFacts{Count: 2}, Trash: &DeskTrashFacts{Items: 1}}, nil
		}})
	}
	f := s.Facts(context.Background(), DeskRequest{}, DeskKeys)
	if f.Keys == nil || f.Keys.Count != 2 || f.Trash != nil || asked[DeskTrash] {
		t.Errorf("only the keys slot was asked for: %+v, asked %v", f, asked)
	}
	if f := s.Facts(context.Background(), DeskRequest{}); f.Keys != nil || f.Trash != nil {
		t.Errorf("no slots asks nobody: %+v", f)
	}
	s.Register(stubProvider{DeskKeys, func(context.Context) (DeskFacts, error) { return DeskFacts{}, errors.New("db down") }})
	if f := s.Facts(context.Background(), DeskRequest{}, DeskKeys); f.Keys != nil {
		t.Errorf("a failed keys provider leaves the count unread: %+v", f.Keys)
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

func TestDeskService_DisabledFoundryDropsTile(t *testing.T) {
	s := NewDeskService()
	s.Register(stubProvider{DeskFoundry, func(context.Context) (DeskFacts, error) {
		old := time.Now().Add(-30 * 24 * time.Hour)
		return DeskFacts{Foundry: &DeskFoundryFacts{Connection: FoundryConnection{HasKey: true, KeyLastUsed: &old}}}, nil
	}})
	d := s.Build(context.Background(), DeskRequest{Now: time.Now()}, DeskInputs{Campaign: &Campaign{ID: "c", Settings: "{}"}, Addons: map[string]bool{}})
	if d.Board.Foundry != nil || len(d.Board.Needs) != 0 {
		t.Error("with Foundry sync off the board must neither draw nor flag Foundry")
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
				for _, want := range []string{
					"Campaign settings", "7 items", "in the Trash · kept 30 days", `href="/campaigns/camp-1/extensions"`, `href="/campaigns/camp-1/data"`,
					`id="line-access"`, `id="line-pages"`, `id="line-people"`, `id="line-features"`, `id="line-look"`, `id="line-trash"`, `data-sb-status`,
				} {
					if !strings.Contains(body, want) {
						t.Errorf("board is missing %q", want)
					}
				}
				for _, gone := range []string{"Danger zone", `id="line-end"`, "settings?tab=", "manage-pin", "The table"} {
					if strings.Contains(body, gone) {
						t.Errorf("the board no longer carries %q", gone)
					}
				}
				if n := len(regexp.MustCompile(`\sdata-manage-find[\s>=]`).FindAllString(body, -1)); n > 1 {
					t.Errorf("the page draws at most one Find, got %d", n)
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

// The board page draws every tile in reading order, the flips as buttons that
// say their state, the chips as pressed or not, and the board itself carries
// no script tag: the page arrives by a boosted swap, so each control brings
// its own handler.
func TestDeskPage_RendersTheBoard(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Therin", Settings: "{}"}, MemberRole: RoleOwner}
	in := DeskInputs{Campaign: cc.Campaign, Addons: map[string]bool{"sync-api": true}, FeaturesOn: 1, FeaturesTotal: 2}
	features := []PluginHubAddon{{AddonID: 5, Name: "Maps", Installed: true, Enabled: true}, {AddonID: 6, Name: "Calendar", Installed: true}}
	needs := []NeedsYouItem{{Lead: "Foundry", Rest: "Stopped answering 9 days ago", URL: "/campaigns/camp-1/foundry", State: "warn"}}
	d := &Desk{CampaignName: "Therin", Board: boardOf(cc.Campaign, in, DeskFacts{Changes: &DeskChangesFacts{Changes: 3}}, needs, features, time.Now())}
	ctx := layouts.SetCampaignRole(layouts.SetCampaignID(layouts.SetIsAuthenticated(context.Background(), true), "camp-1"), 3)
	var sb strings.Builder
	if err := DeskPage(cc, d).Render(ctx, &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	for _, want := range []string{
		`data-desk-needs`, `Stopped answering 9 days ago`, `href="/campaigns/camp-1/foundry"`, `desk-dot warn`,
		`<button type="button" class="sb-t sb-flip sb-flip-access"`, `aria-pressed="false"`, `aria-label="Campaign: Private"`,
		`data-url="/campaigns/camp-1/details"`, `data-url="/campaigns/camp-1/default-visibility"`,
		`data-sb-chip="5" aria-pressed="true"`, `data-sb-chip="6" aria-pressed="false"`, `data-sb-count>1 of 2 on`,
		`aria-label="Player edits: 3, this week"`, `href="/campaigns/camp-1/members#invite-box"`, `aria-label="Invite someone"`,
		`role="status" data-sb-status`, `onclick=`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("board is missing %q", want)
		}
	}
	var board strings.Builder
	if err := deskBoard(cc, d.Board).Render(ctx, &board); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(board.String(), "<script") {
		t.Error("the board must not carry a script tag")
	}
	order := []string{`id="line-access"`, `id="line-pages"`, `id="line-people"`, `id="line-system"`, `id="line-edits"`, `id="line-features"`, `id="line-look"`, `id="line-foundry"`, `id="line-ai"`, `id="line-storage"`, `id="line-trash"`}
	last := -1
	for _, w := range order {
		at := strings.Index(html, w)
		if at < 0 || at < last {
			t.Errorf("%s is missing or out of order", w)
		}
		last = at
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

// updateService records what Update was asked.
type updateService struct {
	CampaignService
	got *UpdateCampaignInput
}

func (s *updateService) Update(_ context.Context, _ string, in UpdateCampaignInput) (*Campaign, error) {
	s.got = &in
	return &Campaign{ID: "camp-1"}, nil
}

// A form that posts only the description must hand the service a description
// and nothing else, so the name and the public flag stay as they are; a body
// that names them still reaches the service.
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

// A waiting player draft shows in Needs you and in the sidebar's Manage count,
// both from one provider slot.
func TestDeskService_EntriesWaitingNeedsYou(t *testing.T) {
	in := DeskInputs{Campaign: &Campaign{ID: "c", Settings: "{}"}}
	s := NewDeskService()
	s.Register(stubProvider{DeskEntries, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Entries: &DeskEntriesFacts{Entries: 7, Drafts: 2}}, nil
	}})
	d := s.Build(context.Background(), DeskRequest{}, in)
	if len(d.Board.Needs) != 1 || d.Board.Needs[0].Rest != "2 drafts to read" || d.Board.Needs[0].URL != "/campaigns/c/characters/entries" {
		t.Errorf("needs = %+v", d.Board.Needs)
	}
	if n := s.NeedsCount(context.Background(), DeskRequest{}, in); n != 1 {
		t.Errorf("needs count = %d, want 1", n)
	}
}

func TestNeedsYou_EntriesWording(t *testing.T) {
	c := &Campaign{ID: "c1"}
	for _, tt := range []struct {
		f    DeskEntriesFacts
		want string
	}{
		{DeskEntriesFacts{Drafts: 1}, "1 draft to read"},
		{DeskEntriesFacts{Asks: 1}, "1 ask to answer"},
		{DeskEntriesFacts{Drafts: 2, Asks: 3}, "2 drafts to read · 3 asks to answer"},
	} {
		got := needsYou(c, DeskFacts{Entries: &tt.f}, time.Time{})
		if len(got) != 1 || got[0].Rest != tt.want {
			t.Errorf("%+v: got %+v, want %q", tt.f, got, tt.want)
		}
	}
}
