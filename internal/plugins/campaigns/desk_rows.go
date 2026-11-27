// desk_rows.go builds the settings Campaign settings changes where they stand
// (name, description, who can see it, new pages, the preview, the Customize
// rows), the values the board's tiles show, the figures a Manage tool page
// draws beside its title, and what needs the owner. Each builder is a pure
// function of its inputs, so the words for every state, the plural forms and
// the "figure could not be read" fallback are table-tested together. A figure
// that cannot be read is the quiet word "Open", never a placeholder number.

package campaigns

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// foundryStoppedAfter is how long Foundry may be silent before the desk
// lists it under Needs you. A Foundry world is often closed between sessions,
// so a day (the dot's amber) is too soon to ask for the owner's attention.
const foundryStoppedAfter = 7 * 24 * time.Hour

// plural returns "1 page" or "3 pages".
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// deskBytes words a size the way a person reads storage: "310 MB".
func deskBytes(n int64) string {
	const unit = 1024
	switch {
	case n < unit:
		return fmt.Sprintf("%d B", n)
	case n < unit*unit:
		return fmt.Sprintf("%d KB", (n+unit/2)/unit)
	case n < unit*unit*unit:
		return fmt.Sprintf("%d MB", (n+unit*unit/2)/(unit*unit))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(unit*unit*unit))
	}
}

func deskHref(campaignID, path string) string { return "/campaigns/" + campaignID + path }

// figureValue is a figure that opens its page: the figure, an optional muted
// second part, and a chevron the template adds for any link that is not a
// quiet word.
func figureValue(text, sub, href string) DeskValue {
	return DeskValue{Text: text, Sub: sub, Href: href}
}

// openValue is what a figure becomes when its facts could not be read: the
// quiet word "Open" leading to the same page. It never invents a number.
func openValue(href string) DeskValue { return DeskValue{Text: "Open", Href: href} }

// ---- Name and description ---------------------------------------------------

// nameLine is the campaign's name, renamed where it stands.
func nameLine(c *Campaign) DeskLine {
	b := &DeskBlank{
		Kind: DeskBlankName, ID: "name", Label: "Name", Value: c.Name, Shown: c.Name,
		URL: deskHref(c.ID, "/details"), Method: "PUT", Field: "name", Max: MaxCampaignNameLen,
	}
	offWhenArchived(c, b)
	return DeskLine{ID: "name", Name: "Name", Values: []DeskValue{{Blank: b}}}
}

// deskDescriptionShown caps how much of a description the row carries into
// the page; the stylesheet cuts it to the row's width with an ellipsis.
const deskDescriptionShown = 240

// descriptionLine is the campaign's description, edited where it stands in a
// field of a few lines. It saves through the same route as the name.
func descriptionLine(c *Campaign) DeskLine {
	raw := ""
	if c.Description != nil {
		raw = strings.TrimSpace(*c.Description)
	}
	b := &DeskBlank{
		Kind: DeskBlankText, ID: "description", Label: "Description", Value: raw,
		Shown: deskCut(strings.Join(strings.Fields(raw), " "), deskDescriptionShown),
		URL:   deskHref(c.ID, "/details"), Method: "PUT", Field: "description", Max: MaxDescriptionLen,
	}
	if raw == "" {
		b.Shown, b.Empty = "None", true
	}
	offWhenArchived(c, b)
	return DeskLine{ID: "description", Name: "Description", Values: []DeskValue{{Blank: b}}}
}

// deskCut shortens s to about n characters on a word boundary, with an
// ellipsis; a word longer than n is cut where it stands.
func deskCut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := r[:n]
	if r[n] != ' ' {
		if i := strings.LastIndex(string(cut), " "); i > 0 {
			cut = []rune(string(cut)[:i])
		}
	}
	return strings.TrimRight(string(cut), " ,.;:!?-") + "…"
}

// ---- Access --------------------------------------------------------------

// The words of the choices of who may read a campaign and of where new pages
// start, with the hint each menu row carries under its label. The word is the
// whole state shown on the row, so a pick shows exactly what the menu said.
const (
	deskPrivateShown = "Private"
	deskPublicShown  = "Public"
	deskHiddenShown  = "Hidden"
	deskOpenShown    = "Shown"
)

// deskArchivedOff is why a blank is off on an archived campaign.
const deskArchivedOff = "Unarchive the campaign to change this."

// deskGuestOff is why the guest preview is off while the campaign is private.
const deskGuestOff = "Guests can't open a private campaign. Make it Public first."

// offWhenArchived marks a blank off while the campaign is read-only.
func offWhenArchived(c *Campaign, b *DeskBlank) {
	if c.IsArchived() {
		b.Off = deskArchivedOff
	}
}

// publicLine is whether people outside the campaign may read it. It saves
// through the details route.
func publicLine(c *Campaign) DeskLine {
	pub := DeskBlank{
		Kind: DeskBlankChoice, ID: "public", Label: "Campaign", Bool: true,
		URL: deskHref(c.ID, "/details"), Method: "PUT", Field: "isPublic",
		Options: []DeskOption{
			{Value: "false", Label: deskPrivateShown, Hint: "Members only"},
			{Value: "true", Label: deskPublicShown, Hint: "Anyone with the link"},
		},
		Value: "false", Shown: deskPrivateShown,
	}
	if c.IsPublic {
		pub.Value, pub.Shown = "true", deskPublicShown
	}
	offWhenArchived(c, &pub)
	return DeskLine{ID: "public", Name: "Campaign", Values: []DeskValue{{Blank: &pub}}}
}

// pagesLine is where new pages start. It saves the moment it is changed,
// through the existing default-visibility route.
func pagesLine(c *Campaign) DeskLine {
	vis := DeskBlank{
		Kind: DeskBlankChoice, ID: "visibility", Label: "New pages",
		URL: deskHref(c.ID, "/default-visibility"), Method: "PUT", Field: "visibility",
		Options: []DeskOption{
			{Value: DefaultVisibilityDMOnly, Label: deskHiddenShown, Hint: "Yours until you show them"},
			{Value: "", Label: deskOpenShown, Hint: "Players see them at once"},
		},
		Value: "", Shown: deskOpenShown,
	}
	if c.ParseSettings().DefaultsToPrivate() {
		// A stored "private" reads as hidden too: the storage has one flag.
		vis.Value, vis.Shown = DefaultVisibilityDMOnly, deskHiddenShown
	}
	offWhenArchived(c, &vis)
	return DeskLine{ID: "pages", Name: "New pages", Values: []DeskValue{{Blank: &vis}}}
}

// previewLine opens the campaign's home as a player and as a guest sees it,
// each by posting the view-as form. While the campaign is private a guest
// cannot open it, so that word is off with the reason; the page enables it
// the moment the campaign is made public.
func previewLine(c *Campaign) DeskLine {
	player := DeskValue{Text: "As a player", Word: true, View: &DeskView{As: "player", Next: "home"}}
	guest := DeskValue{Text: "As a guest", Word: true, View: &DeskView{As: "guest", Next: "home"}, NeedsPublic: true}
	// The reason is always carried so the page can show it the moment the
	// campaign turns private again; only an off value points at it.
	l := DeskLine{ID: "preview", Name: "Preview", Why: deskGuestOff}
	if !c.IsPublic {
		guest.Off = deskGuestOff
	}
	l.Values = []DeskValue{player, guest}
	return l
}

// ---- People --------------------------------------------------------------

// headCount is how many people are in the campaign, the owner included; zero
// when the members could not be read.
func headCount(members []CampaignMember) int {
	if len(members) == 0 {
		return 0
	}
	n := 1
	for _, m := range members {
		if m.Role != RoleOwner {
			n++
		}
	}
	return n
}

// peopleFigure is what the People page draws beside its title: "5 members ·
// 1 invited", with no invited part when nobody waits.
func peopleFigure(members []CampaignMember, in DeskInputs) DeskFigure {
	n := headCount(members)
	if n == 0 {
		return DeskFigure{}
	}
	f := DeskFigure{Parts: []string{plural(n, "member", "members")}}
	if in.InvitedRead && in.Invited > 0 {
		f.Parts = append(f.Parts, fmt.Sprintf("%d invited", in.Invited))
	}
	return f
}

func changesFigure(f *DeskChangesFacts) DeskFigure {
	if f == nil {
		return DeskFigure{}
	}
	return DeskFigure{Parts: []string{fmt.Sprintf("%d this week", f.Changes)}}
}

// ---- Game ----------------------------------------------------------------

// gameFigure is what the Game and features page draws beside its title:
// "Draw Steel · 12 of 15 on". A part it cannot name is left out.
func gameFigure(in DeskInputs) DeskFigure {
	var f DeskFigure
	switch {
	case in.SystemName != "":
		f.Parts = append(f.Parts, in.SystemName)
	case in.SystemID != "":
		f.Parts = append(f.Parts, "Custom")
	}
	if in.FeaturesTotal > 0 {
		f.Parts = append(f.Parts, fmt.Sprintf("%d of %d on", in.FeaturesOn, in.FeaturesTotal))
	}
	return f
}

// ---- Customize -------------------------------------------------------------

// The words of the brush and Store icon choices, which are the viewer's own.
const (
	deskBrushShown  = "Shown"
	deskBrushHidden = "Hidden"
)

// lookLine reads the saved look back in a DM's words: the starter look's
// name, the colour's name beside its swatch and the page style ("Parchment ·
// Sienna · Sketchbook pages"). A colour the owner mixed has no name, so only
// its swatch stands there.
func lookLine(c *Campaign, s LookSummary) DeskLine {
	v := figureValue(s.Look, joinParts(s.Colour, s.Pages), deskHref(c.ID, "/customize/look"))
	v.Joined, v.SubSwatch = true, true
	return DeskLine{ID: "look", Name: "Look", Values: []DeskValue{v}}
}

// joinParts joins the non-empty parts with a middle dot.
func joinParts(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// categoriesLine is how many categories there are, as the sidebar counts
// them, and how many only the DM team sees.
func categoriesLine(c *Campaign, in DeskInputs) DeskLine {
	href := deskHref(c.ID, "/customize/categories")
	l := DeskLine{ID: "categories", Name: "Categories", Values: []DeskValue{openValue(href)}}
	if !in.CategoriesRead {
		return l
	}
	n, hidden := categoriesSummaryOf(in.Categories)
	sub := ""
	if hidden > 0 {
		sub = fmt.Sprintf("%d hidden from players", hidden)
	}
	v := figureValue(strconv.Itoa(n), sub, href)
	v.Joined = true
	l.Values = []DeskValue{v}
	return l
}

// homesLine says who sees which home.
func homesLine(c *Campaign, in DeskInputs) DeskLine {
	text, sub := homesSummaryOf(c, in.GMTerm)
	v := figureValue(text, sub, deskHref(c.ID, "/customize/home"))
	v.Joined = true
	return DeskLine{ID: "homes", Name: "Home pages", Values: []DeskValue{v}}
}

// tabLine is an area another plugin adds, with the words that plugin gives
// for what is chosen now. A plugin that gives none shows what the area
// holds instead, so the row is never just a name and a chevron.
func tabLine(c *Campaign, t DeskTab) DeskLine {
	v := figureValue(t.Summary.Text, "", deskHref(c.ID, "/customize/x/"+t.ID))
	if t.Summary.Text != "" {
		v.Sub, v.Joined = t.Summary.Sub, true
	} else {
		v.Sub = t.Blurb
	}
	return DeskLine{ID: "x-" + t.ID, Name: t.Label, Values: []DeskValue{v}}
}

// brushLine is whether the header's Customize brush shows for this person in
// this campaign. It is their own choice, kept per campaign with their other
// display choices, so it is
// never off on an archived campaign.
func brushLine(campaignID string, hidden bool) DeskLine {
	b := DeskBlank{
		Kind: DeskBlankChoice, ID: "brush", Label: "Header brush",
		URL: "/account/brush/" + campaignID, Method: "PUT", Field: "brush",
		Options: []DeskOption{
			{Value: "shown", Label: deskBrushShown, Hint: "Customize from any page you're on"},
			{Value: "hidden", Label: deskBrushHidden, Hint: "Only you, only in this campaign"},
		},
		Value: "shown", Shown: deskBrushShown,
	}
	if hidden {
		b.Value, b.Shown = "hidden", deskBrushHidden
	}
	return DeskLine{ID: "brush", Name: "Header brush", Values: []DeskValue{{Blank: &b}}}
}

// storeLine is whether the top bar's Store icon shows for this person in
// this campaign: their own choice, kept per campaign like the brush's, and
// so never off on an archived campaign either.
func storeLine(campaignID string, hidden bool) DeskLine {
	b := DeskBlank{
		Kind: DeskBlankChoice, ID: "store", Label: "Store icon",
		URL: "/account/store/" + campaignID, Method: "PUT", Field: "store",
		Options: []DeskOption{
			{Value: "shown", Label: deskBrushShown, Hint: "On the top bar, from any page you're on"},
			{Value: "hidden", Label: deskBrushHidden, Hint: "Only you, only in this campaign"},
		},
		Value: "shown", Shown: deskBrushShown,
	}
	if hidden {
		b.Value, b.Shown = "hidden", deskBrushHidden
	}
	return DeskLine{ID: "store", Name: "Store icon", Values: []DeskValue{{Blank: &b}}}
}

// customizeLines are the Customize block: one row per Customize area in its
// own order, then the brush and the Store icon.
func customizeLines(c *Campaign, in DeskInputs) []DeskLine {
	out := []DeskLine{lookLine(c, in.Look), categoriesLine(c, in), homesLine(c, in)}
	for _, t := range in.Tabs {
		out = append(out, tabLine(c, t))
	}
	return append(out, brushLine(c.ID, in.BrushHidden), storeLine(c.ID, in.StoreHidden))
}

func mediaFigure(f *DeskMediaFacts) DeskFigure {
	if f == nil {
		return DeskFigure{}
	}
	out := DeskFigure{Parts: []string{plural(f.Files, "file", "files")}}
	if f.Files > 0 {
		out.Parts = append(out.Parts, deskBytes(f.Bytes))
	}
	return out
}

// ---- Connections ---------------------------------------------------------

// foundrySeen is when Foundry last answered, from the socket or the keys.
func foundrySeen(conn FoundryConnection) *time.Time {
	return latestTime(conn.HubLastSeen, conn.KeyLastUsed)
}

// foundryValue words the Foundry link as a short figure with a dot: connected,
// last seen a while ago, no answer for some days, or not set up.
func foundryValue(c *Campaign, f *DeskFoundryFacts, now time.Time) DeskValue {
	href := deskHref(c.ID, "/foundry")
	if f == nil {
		return openValue(href)
	}
	conn := f.Connection
	v := figureValue("", "", href)
	switch ComputeFoundryStatus(now, conn).Level {
	case FoundryConnectedNow:
		v.Text, v.State = "Connected", "ok"
	case FoundrySeenRecently:
		v.Text, v.Sub, v.State = "Last seen", relativeAgo(now.Sub(*foundrySeen(conn))), "ok"
	case FoundrySeenLongAgo:
		v.Text, v.Sub, v.State = "No answer", strings.TrimSuffix(relativeAgo(now.Sub(*foundrySeen(conn))), " ago"), "warn"
	default:
		if conn.HasKey {
			v.Text, v.Sub = "No answer", "yet"
		} else {
			v.Text = "Not set up"
		}
	}
	return v
}

// foundryFigure is the Foundry page's figure: the dot, then the same words.
func foundryFigure(c *Campaign, f *DeskFoundryFacts, now time.Time) DeskFigure {
	if f == nil {
		return DeskFigure{}
	}
	return figureOf(foundryValue(c, f, now))
}

// figureOf turns a row's figure into the parts a tool page draws.
func figureOf(v DeskValue) DeskFigure {
	out := DeskFigure{State: v.State}
	for _, p := range []string{v.Text, v.Sub} {
		if p != "" {
			out.Parts = append(out.Parts, p)
		}
	}
	return out
}

// aiValue words the owner's AI connection: the app's name (or "Connected")
// with what it may do, "Paused", "Ended" or how many requests wait.
func aiValue(c *Campaign, f *DeskAIFacts) DeskValue {
	href := deskHref(c.ID, "/ai")
	if f == nil {
		return openValue(href)
	}
	v := figureValue("", "", href)
	switch f.State {
	case DeskAIConnected:
		v.State = "ok"
		v.Text = f.Client
		if v.Text == "" {
			v.Text = "Connected"
		}
		v.Sub = "can look"
		if f.CanAsk {
			v.Sub = "can ask"
		}
	case DeskAIPaused, DeskAIEnded:
		word := "Paused"
		if f.State == DeskAIEnded {
			word = "Ended"
		}
		v.Text, v.Sub = f.Client, word
		if v.Text == "" {
			v.Text, v.Sub = word, ""
		}
	default:
		v.Text = "Not connected"
	}
	if f.Waiting > 0 && f.State != DeskAINone {
		v.Sub = fmt.Sprintf("%d waiting", f.Waiting)
	}
	return v
}

func aiFigure(c *Campaign, f *DeskAIFacts) DeskFigure {
	if f == nil {
		return DeskFigure{}
	}
	return figureOf(aiValue(c, f))
}

// ---- Data ----------------------------------------------------------------

// trashValue is what waits in the campaign Trash and for how long.
func trashValue(c *Campaign, f *DeskTrashFacts) DeskValue {
	href := deskHref(c.ID, "/trash")
	switch {
	case f == nil:
		return openValue(href)
	case f.Items == 0:
		return figureValue("Empty", "", href)
	}
	sub := ""
	if f.Days > 0 {
		sub = "kept " + plural(f.Days, "day", "days")
	}
	return figureValue(plural(f.Items, "item", "items"), sub, href)
}

func trashFigure(c *Campaign, f *DeskTrashFacts) DeskFigure {
	if f == nil || f.Items == 0 {
		return DeskFigure{}
	}
	return figureOf(trashValue(c, f))
}

// ---- Needs you -------------------------------------------------------------

// needsYou lists what only the owner can unblock, most urgent first. Only
// what a provider could actually measure appears: a missing or failed
// provider adds nothing, never a guess. Today that is Foundry silent for a
// week and AI requests waiting for an answer.
func needsYou(c *Campaign, f DeskFacts, now time.Time) []NeedsYouItem {
	var out []NeedsYouItem
	if f.Foundry != nil {
		conn := f.Foundry.Connection
		if seen := foundrySeen(conn); conn.HasKey && !conn.Connected && seen != nil && now.Sub(*seen) >= foundryStoppedAfter {
			out = append(out, NeedsYouItem{
				Lead: "Foundry", Rest: "Stopped answering " + relativeAgo(now.Sub(*seen)),
				URL: deskHref(c.ID, "/foundry"), State: "warn",
			})
		}
	}
	if f.AI != nil && f.AI.Waiting > 0 {
		out = append(out, NeedsYouItem{
			Lead: "Your AI", Rest: plural(f.AI.Waiting, "request", "requests") + " waiting for your answer",
			URL: deskHref(c.ID, "/ai"),
		})
	}
	if f.Entries != nil && f.Entries.Waiting() > 0 {
		var parts []string
		if n := f.Entries.Drafts; n > 0 {
			parts = append(parts, plural(n, "draft", "drafts")+" to read")
		}
		if n := f.Entries.Asks; n > 0 {
			parts = append(parts, plural(n, "ask", "asks")+" to answer")
		}
		out = append(out, NeedsYouItem{
			Lead: "Own entries", Rest: strings.Join(parts, " · "),
			URL: deskHref(c.ID, "/characters/entries"),
		})
	}
	return out
}
