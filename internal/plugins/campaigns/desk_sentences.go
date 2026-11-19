// desk_sentences.go words every line of the desk from live numbers. Each
// builder is a pure function of its inputs so the row words, the sentences, the
// plural forms and the "no numbers" fallbacks are table-tested together. A
// line whose numbers are missing shows its name alone: the desk never prints a
// placeholder or a made-up figure. A line that a tool page also opens with
// carries its sentence (Parts) beside the short row words, both worded from
// the same facts.

package campaigns

import (
	"fmt"
	"strings"
	"time"
)

// foundryStoppedAfter is how long Foundry may be silent before the desk
// lists it under Needs you. A Foundry world is often closed between sessions,
// so a day (the dot's amber) is too soon to ask for the owner's attention.
const foundryStoppedAfter = 7 * 24 * time.Hour

func txt(s string) DeskPart  { return DeskPart{Text: s} }
func bold(s string) DeskPart { return DeskPart{Text: s, Bold: true} }

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

// peopleLine counts who is in the campaign by role in its sentence ("4 people:
// you, a co-DM and 2 players.") and says only the head count on its row. members
// is nil when they could not be read, which leaves the row's name alone and the
// sentence a plain link.
func peopleLine(c *Campaign, members []CampaignMember, invited int) DeskLine {
	l := DeskLine{ID: "people", Name: "People", Href: deskHref(c.ID, "/members")}
	if len(members) == 0 {
		l.Parts = []DeskPart{txt("Everyone in the campaign, and what each may do, is on the People page.")}
		return l
	}
	var coDM, scribes, players int
	for _, m := range members {
		switch {
		case m.Role == RoleOwner:
		case MemberAccess(c, m) == AccessCoDM:
			coDM++
		case m.Role == RoleScribe:
			scribes++
		default:
			players++
		}
	}
	total := coDM + scribes + players + 1
	// Waiting invitations are said apart from the count: nobody invited is a
	// person yet.
	with := ""
	if invited > 0 {
		with = fmt.Sprintf(", with %d invited", invited)
	}
	if total == 1 {
		l.Short = "Just you"
		if invited > 0 {
			l.Short += fmt.Sprintf(", %d invited", invited)
		}
		l.Parts = []DeskPart{txt("Just you, so far" + with + ".")}
		return l
	}
	l.Short = fmt.Sprintf("%d people", total)
	if invited > 0 {
		l.Short += fmt.Sprintf(", %d invited", invited)
	}
	l.Parts = []DeskPart{txt(fmt.Sprintf("%d people: you", total))}
	var tail [][]DeskPart
	if coDM > 0 {
		if coDM == 1 {
			tail = append(tail, []DeskPart{txt("a co-"), {GM: true}})
		} else {
			tail = append(tail, []DeskPart{txt(fmt.Sprintf("%d co-", coDM)), {GM: true}, txt("s")})
		}
	}
	if scribes > 0 {
		tail = append(tail, []DeskPart{txt(plural(scribes, "scribe", "scribes"))})
	}
	if players > 0 {
		if players == 1 {
			tail = append(tail, []DeskPart{txt("a player")})
		} else {
			tail = append(tail, []DeskPart{txt(fmt.Sprintf("%d players", players))})
		}
	}
	for i, t := range tail {
		sep := ", "
		if i == len(tail)-1 {
			sep = " and "
		}
		l.Parts = append(l.Parts, txt(sep))
		l.Parts = append(l.Parts, t...)
	}
	l.Parts = append(l.Parts, txt(with+"."))
	return l
}

// changesLine says how much players changed this week.
func changesLine(c *Campaign, f *DeskChangesFacts) DeskLine {
	l := DeskLine{ID: "changes", Name: "What players changed", Href: deskHref(c.ID, "/players/changes")}
	switch {
	case f == nil:
		l.Parts = []DeskPart{txt("What players changed on your pages can be looked at, and undone, on its own page.")}
	case f.Changes == 0:
		l.Short = "Nothing this week"
		l.Parts = []DeskPart{txt("No player has changed anything this week.")}
	default:
		who := "A player"
		by := "1 player"
		if f.Players > 1 {
			who = fmt.Sprintf("%d players", f.Players)
			by = who
		}
		l.Short = plural(f.Changes, "change", "changes") + " by " + by + " this week"
		l.Parts = []DeskPart{bold(who), txt(" changed " + plural(f.Changes, "thing", "things") + " this week.")}
	}
	return l
}

// nameLine is the campaign's name, renamed where it stands.
func nameLine(c *Campaign) DeskLine {
	b := &DeskBlank{
		Kind: DeskBlankName, ID: "name", Label: "Campaign name", Value: c.Name, Shown: c.Name,
		URL: deskHref(c.ID, "/details"), Method: "PUT", Field: "name",
	}
	offWhenArchived(c, b)
	return DeskLine{ID: "name", Name: "Name", Blank: b}
}

// gameLine says which game the campaign plays and how many features are on.
// It names nothing it was not told: with neither number the row is its name
// alone and the sentence a plain link.
func gameLine(c *Campaign, in DeskInputs) DeskLine {
	l := DeskLine{ID: "game", Name: "Game & features", Href: deskHref(c.ID, "/extensions")}
	feats := ""
	if in.FeaturesTotal > 0 {
		feats = fmt.Sprintf("%d of %d features", in.FeaturesOn, in.FeaturesTotal)
	}
	switch {
	case in.SystemName != "" && feats != "":
		l.Short = in.SystemName + ", " + feats + " on"
		l.Parts = []DeskPart{txt("Plays " + in.SystemName + ", with " + feats + " on.")}
	case in.SystemName != "":
		l.Short = in.SystemName
		l.Parts = []DeskPart{txt("Plays " + in.SystemName + ".")}
	case feats != "":
		l.Short = "No game system, " + feats + " on"
		l.Parts = []DeskPart{txt("Plays no game system, with " + feats + " on.")}
	default:
		l.Parts = []DeskPart{txt("Its game system and features are chosen on their own page.")}
	}
	return l
}

// The words of the two choices of who may read a campaign, and of where new
// pages start. They are the whole state shown on the row, so a pick shows
// exactly what the menu said.
const (
	deskPrivateShown = "Private: members only"
	deskPublicShown  = "Public: anyone with the link"
	deskHiddenShown  = "Hidden from players"
	deskOpenShown    = "Shown to players"
)

// deskArchivedOff is why a blank is off on an archived campaign.
const deskArchivedOff = "Unarchive the campaign to change this."

// offWhenArchived marks a blank off while the campaign is read-only.
func offWhenArchived(c *Campaign, b *DeskBlank) {
	if c.IsArchived() {
		b.Off = deskArchivedOff
	}
}

// pagesLine is where new pages start. It saves the moment it is changed,
// through the existing default-visibility route.
func pagesLine(c *Campaign) DeskLine {
	vis := DeskBlank{
		Kind: DeskBlankChoice, ID: "visibility", Label: "New pages start",
		URL: deskHref(c.ID, "/default-visibility"), Method: "PUT", Field: "visibility",
		Options: []DeskOption{{Value: DefaultVisibilityDMOnly, Label: deskHiddenShown}, {Value: "", Label: deskOpenShown}},
		Value:   "", Shown: deskOpenShown,
	}
	if c.ParseSettings().DefaultsToPrivate() {
		// A stored "private" reads as hidden too: the storage has one flag.
		vis.Value, vis.Shown = DefaultVisibilityDMOnly, deskHiddenShown
	}
	offWhenArchived(c, &vis)
	return DeskLine{ID: "pages", Name: "New pages start", Blank: &vis}
}

// publicLine is whether people outside the campaign may read it. It saves
// through the details route.
func publicLine(c *Campaign) DeskLine {
	pub := DeskBlank{
		Kind: DeskBlankChoice, ID: "public", Label: "Who can read it", Bool: true,
		URL: deskHref(c.ID, "/details"), Method: "PUT", Field: "isPublic",
		Options: []DeskOption{{Value: "false", Label: deskPrivateShown}, {Value: "true", Label: deskPublicShown}},
		Value:   "false", Shown: deskPrivateShown,
	}
	if c.IsPublic {
		pub.Value, pub.Shown = "true", deskPublicShown
	}
	offWhenArchived(c, &pub)
	return DeskLine{ID: "public", Name: "Who can read it", Blank: &pub}
}

// playersHomeLine opens the campaign's home as a player sees it.
func playersHomeLine() DeskLine {
	return DeskLine{ID: "players-home", Name: "Players' dashboard", Short: "See it", View: &DeskView{As: "player", Next: "home"}}
}

// guestsHomeLine opens the campaign's home as a guest sees it. A private
// campaign says guests can't open it but the row still opens: the home it lands
// on shows that note and is where the owner arranges the guests' dashboard.
func guestsHomeLine(c *Campaign) DeskLine {
	l := DeskLine{ID: "guests-home", Name: "Guests' dashboard", Short: "See it", View: &DeskView{As: "guest", Next: "home"}}
	if !c.IsPublic {
		l.Short = "Private campaign: guests can't open it"
	}
	return l
}

// deskDescriptionWidth is about how much of the description a row shows.
const deskDescriptionWidth = 40

// descriptionLine shows the start of the campaign's description, cut on a
// word, and leads to the Details tab where it is edited.
func descriptionLine(c *Campaign) DeskLine {
	l := DeskLine{ID: "description", Name: "Description", Short: "None yet", Href: deskHref(c.ID, "/settings")}
	if c.Description != nil {
		if d := strings.Join(strings.Fields(*c.Description), " "); d != "" {
			l.Short = deskCut(d, deskDescriptionWidth)
		}
	}
	return l
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

// lookLine reads the owner's Customize choices back in a few words.
func lookLine(c *Campaign) DeskLine {
	l := DeskLine{ID: "look", Name: "Customize", Href: deskHref(c.ID, "/customize"), Swatch: true}
	a := c.ParseSettings().Appearance
	if a == nil || a.PageLook != "annals" {
		l.Short = "Standard"
		return l
	}
	kit := a.StyleKit
	if kit == "" {
		kit = "material"
	}
	l.Short = "Material, " + strings.ToUpper(kit[:1]) + kit[1:] + " kit"
	return l
}

// mediaLine says what the picture store holds.
func mediaLine(c *Campaign, f *DeskMediaFacts) DeskLine {
	l := DeskLine{ID: "media", Name: "Media", Href: deskHref(c.ID, "/media")}
	if f == nil {
		l.Parts = []DeskPart{txt("Its pictures and files are kept in Media.")}
		return l
	}
	if f.Files == 0 {
		l.Short = "No pictures or files yet"
		l.Parts = []DeskPart{txt("Media holds no pictures or files yet.")}
		return l
	}
	l.Short = plural(f.Files, "picture or file", "pictures and files") + ", " + deskBytes(f.Bytes)
	l.Parts = []DeskPart{txt("Holds " + plural(f.Files, "picture or file", "pictures and files") + ", " + deskBytes(f.Bytes) + ".")}
	return l
}

// settingsLine is the sentence on the Details and data page. The page is the
// home of everything the rows lead to, so it says what is on it; the page
// itself is not a row.
func settingsLine(c *Campaign) DeskLine {
	return DeskLine{ID: "settings", Name: "Details and data", Href: deskHref(c.ID, "/settings"),
		Parts: []DeskPart{txt("Details, keys, export, import and the activity log are on this page.")}}
}

// foundrySeen is when Foundry last answered, from the socket or the keys.
func foundrySeen(conn FoundryConnection) *time.Time {
	return latestTime(conn.HubLastSeen, conn.KeyLastUsed)
}

// foundryLine says whether Foundry is answering, and which module it runs.
func foundryLine(c *Campaign, f *DeskFoundryFacts, now time.Time) DeskLine {
	l := DeskLine{ID: "foundry", Name: "Foundry", Href: deskHref(c.ID, "/foundry")}
	if f == nil {
		l.Parts = []DeskPart{txt("Foundry's link to this campaign, its module and its players are on the Foundry page.")}
		return l
	}
	conn := f.Connection
	st := ComputeFoundryStatus(now, conn)
	var s, short string
	switch st.Level {
	case FoundryConnectedNow:
		s, short, l.State = "Foundry is connected and answering now.", "Connected now", "ok"
	case FoundrySeenRecently:
		ago := relativeAgo(now.Sub(*foundrySeen(conn)))
		s, short, l.State = "Foundry last answered "+ago+".", "Last answered "+ago, "ok"
	case FoundrySeenLongAgo:
		ago := relativeAgo(now.Sub(*foundrySeen(conn)))
		s, short, l.State = "Foundry last answered "+ago+".", "Last answered "+ago, "warn"
	default:
		if conn.HasKey {
			s, short = "Foundry has a key but has not answered yet.", "Key made, no answer yet"
		} else {
			s, short = "Foundry is not set up yet.", "Not set up yet"
		}
	}
	if conn.ModuleVersion != "" {
		s += " Its module is " + conn.ModuleVersion
		short += ", module " + conn.ModuleVersion
		if conn.ServedVersion != "" && conn.ServedVersion != conn.ModuleVersion {
			s += "; Chronicle serves " + conn.ServedVersion
			short += ", Chronicle serves " + conn.ServedVersion
		}
		s += "."
	}
	l.Short = short
	l.Parts = []DeskPart{txt(s)}
	return l
}

// aiLine says whether the owner's AI is connected and what it may do.
func aiLine(c *Campaign, f *DeskAIFacts) DeskLine {
	l := DeskLine{ID: "ai", Name: "Your AI", Href: deskHref(c.ID, "/ai")}
	if f == nil {
		l.Parts = []DeskPart{txt("Connecting an AI app, and what it did, are on the Your AI page.")}
		return l
	}
	name := "Your AI"
	who := "Connected"
	if f.Client != "" {
		name = "Your AI (" + f.Client + ")"
		who = f.Client
	}
	may, mayShort := "may look and ask for changes", "may ask for changes"
	if !f.CanAsk {
		may, mayShort = "may only look", "may only look"
	}
	switch f.State {
	case DeskAIConnected:
		l.State = "ok"
		l.Short = who + ", " + mayShort
		l.Parts = []DeskPart{txt(name + " is connected and " + may + ".")}
	case DeskAIPaused:
		l.Short = "Paused"
		l.Parts = []DeskPart{txt(name + " is paused, so it can do nothing until you resume it.")}
	case DeskAIEnded:
		l.Short = "Key ran out"
		l.Parts = []DeskPart{txt(name + "'s key has run out, so it can no longer get in.")}
	default:
		l.Short = "Not connected yet"
		l.Parts = []DeskPart{txt("No AI is connected yet.")}
	}
	return l
}

// keysLine leads to where keys for other tools are made; it carries no count
// because the keys page loads its own.
func keysLine(c *Campaign) DeskLine {
	return DeskLine{ID: "keys", Name: "Keys", Short: "For other tools", Href: deskHref(c.ID, "/settings?tab="+settingsTabAPIKeys)}
}

// exportLine, importLine and activityLine lead to the tabs of Details and data
// that hold them, so nothing waits behind a page the owner has to guess at.
func exportLine(c *Campaign) DeskLine {
	return DeskLine{ID: "export", Name: "Export a copy", Href: deskHref(c.ID, "/settings?tab="+settingsTabData)}
}

func importLine(c *Campaign) DeskLine {
	return DeskLine{ID: "import", Name: "Import", Href: deskHref(c.ID, "/settings?tab="+settingsTabImport)}
}

func activityLine(c *Campaign) DeskLine {
	return DeskLine{ID: "activity", Name: "Activity log", Href: deskHref(c.ID, "/settings?tab="+settingsTabActivity)}
}

// trashLine says what waits in the campaign Trash.
func trashLine(c *Campaign, f *DeskTrashFacts) DeskLine {
	l := DeskLine{ID: "trash", Name: "Trash", Href: deskHref(c.ID, "/trash")}
	switch {
	case f == nil:
		l.Parts = []DeskPart{txt("Deleted pages wait in the Trash, where you can bring them back.")}
	case f.Items == 0:
		l.Short = "Empty"
		l.Parts = []DeskPart{txt("Trash is empty.")}
	default:
		l.Short = plural(f.Items, "thing", "things") + ", kept " + plural(f.Days, "day", "days")
		l.Parts = []DeskPart{txt("Trash holds " + plural(f.Items, "item", "items") + " for " + plural(f.Days, "day", "days") + ".")}
	}
	return l
}

// endLine and deleteLine are the foot of the page: they lead to the danger
// zone on Details and data. Nothing here changes the campaign.
func endLine(c *Campaign) DeskLine {
	href := deskHref(c.ID, "/settings#danger")
	if c.IsArchived() {
		return DeskLine{ID: "end", Name: "Unarchive this campaign", Short: "Archived, read-only", Href: href}
	}
	return DeskLine{ID: "end", Name: "Archive this campaign", Href: href}
}

func deleteLine(c *Campaign) DeskLine {
	return DeskLine{ID: "delete", Name: "Delete this campaign", Href: deskHref(c.ID, "/settings#danger"), Danger: true}
}

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
	return out
}
