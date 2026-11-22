// desk_board.go builds the Overview board, Campaign settings' first tab: a
// tile per setting that shows where it stands and, for the two that flip and
// the features, is the switch itself. Every tile is a pure function of the
// desk's inputs and the providers' facts, built from the same value builders
// the tool pages' figures use, so the board and a page cannot disagree. A
// figure that could not be read is the word "Open", never a guess.

package campaigns

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DeskBoard is the Overview: one field per tile, drawn in a fixed order. A
// tile that belongs to an add-on (Foundry) is nil while its add-on is off.
type DeskBoard struct {
	// Needs is what waits on the owner; the tile is drawn only when it holds
	// something.
	Needs []NeedsYouItem
	// Access and Pages flip between their two states where they stand.
	Access, Pages DeskFlip
	People        DeskPeopleTile
	System        DeskGoTile
	Edits         DeskGoTile
	Features      DeskFeaturesTile
	Look          DeskLookTile
	Foundry       *DeskGoTile
	AI            DeskGoTile
	Storage       DeskGoTile
	Trash         DeskGoTile
}

// DeskFlip is a tile that flips a two-way setting and saves at once. It wraps
// the same blank the Campaign tab's choice draws, so both save through the
// same route with the same words. The first option is the resting state and
// the second the pressed one (Private and Public, Hidden and Shown).
type DeskFlip struct {
	Label string
	Blank *DeskBlank
}

// On reports whether the flip stands at its second option.
func (f DeskFlip) On() bool {
	return f.Blank != nil && len(f.Blank.Options) == 2 && f.Blank.Value == f.Blank.Options[1].Value
}

// Option is the flip's option at i (0 or 1); the zero option when it has none.
func (f DeskFlip) Option(i int) DeskOption {
	if f.Blank == nil || i < 0 || i >= len(f.Blank.Options) {
		return DeskOption{}
	}
	return f.Blank.Options[i]
}

// Now is the option the flip stands at.
func (f DeskFlip) Now() DeskOption {
	if f.On() {
		return f.Option(1)
	}
	return f.Option(0)
}

// DeskGoTile is a tile that shows a figure and leads to its tab or page. The
// figure is a DeskValue so its state dot and its "Open" fallback come from the
// same builders as the pages' figures.
type DeskGoTile struct {
	Label string
	Value DeskValue
	// Hint is the muted line under the figure; empty draws none.
	Hint string
	// Mono is a short mark drawn before the figure (the game system's
	// initials); empty draws none.
	Mono string
	Href string
}

// DeskPeopleTile is the campaign's people as faces, with a way to invite.
type DeskPeopleTile struct {
	// Count is "5 members · 1 invited"; empty when the members could not be
	// read.
	Count string
	Faces []DeskFace
	// More is how many people the tile has no room to show.
	More       int
	Href       string
	InviteHref string
}

// DeskFace is one person on the People tile.
type DeskFace struct {
	Name, Role string
	// Avatar is the person's picture, a media path; empty draws their initial.
	Avatar string
	// Hue picks one of a few tints for a face with no picture, from the
	// person's id, so a face keeps its tint from visit to visit.
	Hue int
}

// Short is the first word of a face's name, which fits under the face; the
// whole name is its hover words and what a screen reader hears.
func (f DeskFace) Short() string {
	if w := strings.Fields(f.Name); len(w) > 0 {
		return w[0]
	}
	return f.Name
}

// Initial is a face's letter when the person has no picture.
func (f DeskFace) Initial() string {
	for _, r := range strings.TrimSpace(f.Name) {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// DeskFeaturesTile is every installed feature as a chip that switches it on or
// off where it stands, through the add-ons store's own toggle route.
type DeskFeaturesTile struct {
	// Count is "12 of 15 on"; empty when the add-ons could not be read.
	Count string
	Chips []DeskChip
	Href  string
	// Off is why the chips cannot be pressed now (an archived campaign).
	Off string
}

// DeskChip is one feature on the Features tile.
type DeskChip struct {
	AddonID int
	Name    string
	Hint    string
	On      bool
}

// DeskLookTile is the saved look in words, opening the Customizer.
type DeskLookTile struct {
	Look LookSummary
	Href string
}

// deskBoardFaces is how many faces the People tile has room for.
const deskBoardFaces = 6

// boardOf builds the whole board from the desk's inputs and the facts the
// providers returned. Foundry and Media exist only while their add-on is on.
func boardOf(c *Campaign, in DeskInputs, f DeskFacts, needs []NeedsYouItem, features []PluginHubAddon, now time.Time) DeskBoard {
	b := DeskBoard{
		Needs:    needs,
		Access:   DeskFlip{Label: "Campaign", Blank: publicLine(c).Values[0].Blank},
		Pages:    DeskFlip{Label: "New pages", Blank: pagesLine(c).Values[0].Blank},
		People:   peopleTile(c, in),
		System:   systemTile(c, in),
		Edits:    editsTile(c, f.Changes),
		Features: featuresTile(c, in, features),
		Look:     DeskLookTile{Look: in.Look, Href: deskHref(c.ID, "/customize")},
		AI:       aiTile(c, f.AI),
		Storage:  storageTile(c, in.Addons["media-gallery"], f.Media),
		Trash:    trashTile(c, f.Trash),
	}
	if in.Addons["sync-api"] {
		t := foundryTile(c, f.Foundry, now)
		b.Foundry = &t
	}
	return b
}

// peopleTile is the head count, a face for each of the first few people (the
// owner first, as the members list orders them) and the way to invite more.
func peopleTile(c *Campaign, in DeskInputs) DeskPeopleTile {
	t := DeskPeopleTile{
		Count:      peopleFigure(in.Members, in).Text(),
		Href:       deskHref(c.ID, "/members"),
		InviteHref: deskHref(c.ID, "/members#invite-box"),
	}
	for _, m := range in.Members {
		if len(t.Faces) == deskBoardFaces {
			t.More++
			continue
		}
		face := DeskFace{Name: memberShownName(m), Role: m.Role.DisplayName(), Hue: faceHue(m.UserID)}
		if m.AvatarPath != nil {
			face.Avatar = strings.TrimSpace(*m.AvatarPath)
		}
		t.Faces = append(t.Faces, face)
	}
	return t
}

// memberShownName is the name a face carries: the display name, else the part
// of the email before the @, never the whole address.
func memberShownName(m CampaignMember) string {
	if n := strings.TrimSpace(m.DisplayName); n != "" {
		return n
	}
	if at := strings.IndexByte(m.Email, '@'); at > 0 {
		return m.Email[:at]
	}
	return "Someone"
}

// faceHue is a stable small number from an id, for a face's tint.
func faceHue(id string) int {
	h := 0
	for _, r := range id {
		h = (h*31 + int(r)) % 997
	}
	return h % 5
}

// systemTile is the game system and its initials, leading to Game. A system
// the list does not name is "Custom"; none chosen is "None".
func systemTile(c *Campaign, in DeskInputs) DeskGoTile {
	t := DeskGoTile{Label: "Game system", Href: deskHref(c.ID, "/extensions")}
	switch {
	case in.SystemName != "":
		t.Value = DeskValue{Text: in.SystemName}
		t.Mono = monoOf(in.SystemName)
		t.Hint = "Its rules and tooltips are on"
	case in.SystemID != "":
		t.Value = DeskValue{Text: "Custom"}
		t.Hint = "A system of your own"
	default:
		t.Value = DeskValue{Text: "None"}
		t.Hint = "Pick one on Game"
	}
	return t
}

// monoOf is up to two initials of a name ("Draw Steel" is "DS").
func monoOf(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		for _, r := range w {
			out = append(out, r)
			break
		}
		if len(out) == 2 {
			break
		}
	}
	return strings.ToUpper(string(out))
}

// editsTile is how much players changed this week, leading to the page that
// lists every change.
func editsTile(c *Campaign, f *DeskChangesFacts) DeskGoTile {
	href := deskHref(c.ID, "/players/changes")
	t := DeskGoTile{Label: "Player edits", Href: href, Value: openValue(href)}
	if f == nil {
		return t
	}
	t.Value = DeskValue{Text: strconv.Itoa(f.Changes)}
	switch {
	case f.Changes == 0:
		t.Hint = "None this week"
	case f.Players > 0:
		t.Hint = "this week · " + plural(f.Players, "player", "players")
	default:
		t.Hint = "this week"
	}
	return t
}

// featuresTile is a chip for every installed feature, in the store's order.
// One that is not installed yet cannot be switched on, so it has no chip.
func featuresTile(c *Campaign, in DeskInputs, addons []PluginHubAddon) DeskFeaturesTile {
	t := DeskFeaturesTile{Href: deskHref(c.ID, "/extensions")}
	if c.IsArchived() {
		t.Off = deskArchivedOff
	}
	if in.FeaturesTotal > 0 {
		t.Count = fmt.Sprintf("%d of %d on", in.FeaturesOn, in.FeaturesTotal)
	}
	for _, a := range featureAddons(addons) {
		if !a.Installed {
			continue
		}
		t.Chips = append(t.Chips, DeskChip{AddonID: a.AddonID, Name: a.Name, Hint: a.Description, On: a.Enabled})
	}
	return t
}

// foundryTile is the Foundry link's state, leading to the Foundry page.
func foundryTile(c *Campaign, f *DeskFoundryFacts, now time.Time) DeskGoTile {
	v := foundryValue(c, f, now)
	t := DeskGoTile{Label: "Foundry", Href: v.Href, Value: DeskValue{Text: v.Text, State: v.State}}
	switch {
	case f == nil:
		t.Value = v
	case v.Text == "No answer" && v.Sub != "" && v.Sub != "yet":
		t.Hint = "Quiet for " + v.Sub
	case v.Text == "Last seen":
		t.Hint = v.Sub
	case v.Sub != "":
		t.Value.Text += " " + v.Sub
	}
	if t.Hint == "" && f != nil && f.Connection.ModuleVersion != "" {
		t.Hint = "Module " + f.Connection.ModuleVersion
	}
	return t
}

// aiTile is the owner's AI connection, leading to Your AI.
func aiTile(c *Campaign, f *DeskAIFacts) DeskGoTile {
	v := aiValue(c, f)
	t := DeskGoTile{Label: "Your AI", Href: v.Href, Value: DeskValue{Text: v.Text, State: v.State}, Hint: v.Sub}
	if f == nil {
		t.Value, t.Hint = v, ""
	}
	return t
}

// storageTile is what the picture store holds and the way to a copy of the
// whole campaign, leading to Data. Without Media it is only the copies.
func storageTile(c *Campaign, media bool, f *DeskMediaFacts) DeskGoTile {
	href := deskHref(c.ID, "/data")
	t := DeskGoTile{Label: "Storage and copies", Href: href}
	switch {
	case !media:
		t.Label = "Copies"
		t.Value = DeskValue{Text: "Export"}
		t.Hint = "A copy of the whole campaign, to keep or move"
	case f == nil:
		t.Value = openValue(href)
		t.Hint = "Export or import a copy"
	case f.Files == 0:
		t.Value = DeskValue{Text: "Empty"}
		t.Hint = "No pictures or files yet · export or import a copy"
	default:
		t.Value = DeskValue{Text: deskBytes(f.Bytes)}
		t.Hint = plural(f.Files, "file", "files") + " · export or import a copy"
	}
	return t
}

// trashTile is what waits in the Trash and for how long, leading to Data,
// where the Trash and the activity log sit.
func trashTile(c *Campaign, f *DeskTrashFacts) DeskGoTile {
	href := deskHref(c.ID, "/data")
	t := DeskGoTile{Label: "Trash and activity", Href: href}
	v := trashValue(c, f)
	switch {
	case f == nil:
		t.Value = DeskValue{Text: "Open", Href: href}
	case f.Items == 0:
		t.Value = DeskValue{Text: "Empty"}
		t.Hint = "Nothing waiting in the Trash"
	default:
		t.Value = DeskValue{Text: v.Text}
		t.Hint = "in the Trash"
		if v.Sub != "" {
			t.Hint += " · " + v.Sub
		}
	}
	return t
}
