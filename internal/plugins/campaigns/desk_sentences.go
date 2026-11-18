// desk_sentences.go words every line of the desk from live numbers. Each
// builder is a pure function of its inputs so the sentences, the plural
// forms and the "no numbers" fallbacks are table-tested together. A line
// whose numbers are missing is a plain link sentence: the desk never prints a
// placeholder or a made-up figure.

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

// peopleLine counts who is in the campaign by role: "4 people: you, a
// co-DM and 2 players." members is nil when they could not be read, which
// leaves the plain link sentence.
func peopleLine(c *Campaign, members []CampaignMember, invited int) DeskLine {
	l := DeskLine{ID: "people", Acts: []DeskAct{{Label: "Open People", Href: deskHref(c.ID, "/members"), Carry: true}}}
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
		l.Parts = []DeskPart{txt("Just you, so far" + with + ".")}
		return l
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
	l := DeskLine{ID: "changes", Acts: []DeskAct{{Label: "Look", Href: deskHref(c.ID, "/players/changes"), Carry: true}}}
	switch {
	case f == nil:
		l.Parts = []DeskPart{txt("What players changed on your pages can be looked at, and undone, on its own page.")}
	case f.Changes == 0:
		l.Parts = []DeskPart{txt("No player has changed anything this week.")}
	default:
		who := "A player"
		if f.Players > 1 {
			who = fmt.Sprintf("%d players", f.Players)
		}
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
	return DeskLine{ID: "name", Parts: []DeskPart{txt("Called "), {Blank: b}, txt(".")}}
}

// gameLine says which game the campaign plays and how many features are on.
// It names nothing it was not told: with neither number it is a plain link.
func gameLine(c *Campaign, in DeskInputs) DeskLine {
	l := DeskLine{ID: "game", Acts: []DeskAct{{Label: "Game & features", Href: deskHref(c.ID, "/extensions"), Carry: true}}}
	feats := ""
	if in.FeaturesTotal > 0 {
		feats = fmt.Sprintf("%d of %d features", in.FeaturesOn, in.FeaturesTotal)
	}
	switch {
	case in.SystemName != "" && feats != "":
		l.Parts = []DeskPart{txt("Plays " + in.SystemName + ", with " + feats + " on.")}
	case in.SystemName != "":
		l.Parts = []DeskPart{txt("Plays " + in.SystemName + ".")}
	case feats != "":
		l.Parts = []DeskPart{txt("Plays no game system, with " + feats + " on.")}
	default:
		l.Parts = []DeskPart{txt("Its game system and features are chosen on their own page.")}
	}
	return l
}

// Words for the two choices of who may read a campaign.
const (
	deskPrivateAfter = " only its members can read it."
	deskPublicAfter  = " anyone can find it and read its open pages."
)

// deskArchivedOff is why a blank is off on an archived campaign.
const deskArchivedOff = "Unarchive the campaign to change this."

// offWhenArchived marks a blank off while the campaign is read-only.
func offWhenArchived(c *Campaign, b *DeskBlank) {
	if c.IsArchived() {
		b.Off = deskArchivedOff
	}
}

// pagesLine holds the two blanks about who sees what: where new pages start,
// and whether outsiders may read the campaign at all. Both save the moment
// they are changed, through their existing routes (default visibility) or
// the details route.
func pagesLine(c *Campaign) DeskLine {
	hidden := c.ParseSettings().DefaultsToPrivate()
	vis := DeskBlank{
		Kind: DeskBlankChoice, ID: "visibility", Label: "New pages start",
		URL: deskHref(c.ID, "/default-visibility"), Method: "PUT", Field: "visibility",
		Options: []DeskOption{{Value: DefaultVisibilityDMOnly, Label: "hidden from players"}, {Value: "", Label: "open to players"}},
		Value:   "", Shown: "open to players",
	}
	if hidden {
		// A stored "private" reads as hidden too: the storage has one flag.
		vis.Value, vis.Shown = DefaultVisibilityDMOnly, "hidden from players"
	}
	pub := DeskBlank{
		Kind: DeskBlankChoice, ID: "public", Label: "The campaign is", Bool: true,
		URL: deskHref(c.ID, "/details"), Method: "PUT", Field: "isPublic",
		Options: []DeskOption{{Value: "false", Label: "private", After: deskPrivateAfter}, {Value: "true", Label: "public", After: deskPublicAfter}},
		Value:   "false", Shown: "private",
	}
	after := deskPrivateAfter
	if c.IsPublic {
		pub.Value, pub.Shown = "true", "public"
		after = deskPublicAfter
	}
	offWhenArchived(c, &vis)
	offWhenArchived(c, &pub)
	return DeskLine{ID: "pages", Parts: []DeskPart{
		txt("New pages start "), {Blank: &vis}, txt(". The campaign is "), {Blank: &pub}, txt(":"), {Text: after, After: true},
	}}
}

// lookLine reads the owner's Customize choices back as one sentence.
func lookLine(c *Campaign) DeskLine {
	l := DeskLine{ID: "look", Acts: []DeskAct{{Label: "Open the Customizer", Href: deskHref(c.ID, "/customize")}}}
	a := c.ParseSettings().Appearance
	switch {
	case a == nil || a.PageLook != "annals":
		l.Parts = []DeskPart{txt("Looks like Standard, in your colour ")}
	default:
		kit := a.StyleKit
		if kit == "" {
			kit = "material"
		}
		l.Parts = []DeskPart{txt("Looks like Material, the " + strings.ToUpper(kit[:1]) + kit[1:] + " kit, in your colour ")}
	}
	l.Parts = append(l.Parts, DeskPart{Swatch: true}, txt("."))
	return l
}

// mediaLine says what the picture store holds.
func mediaLine(c *Campaign, f *DeskMediaFacts) DeskLine {
	l := DeskLine{ID: "media", Acts: []DeskAct{{Label: "Open Media", Href: deskHref(c.ID, "/media"), Carry: true}}}
	if f == nil {
		l.Parts = []DeskPart{txt("Its pictures and files are kept in Media.")}
		return l
	}
	if f.Files == 0 {
		l.Parts = []DeskPart{txt("Media holds no pictures or files yet.")}
		return l
	}
	l.Parts = []DeskPart{txt("Holds " + plural(f.Files, "picture or file", "pictures and files") + ", " + deskBytes(f.Bytes) + ".")}
	return l
}

// settingsLine points at everything the desk does not draw.
func settingsLine(c *Campaign) DeskLine {
	return DeskLine{ID: "settings", Parts: []DeskPart{txt("Export, import, the activity log and the rest are in Settings.")},
		Acts: []DeskAct{{Label: "Open Settings", Href: deskHref(c.ID, "/settings"), Carry: true}}}
}

// foundrySeen is when Foundry last answered, from the socket or the keys.
func foundrySeen(conn FoundryConnection) *time.Time {
	return latestTime(conn.HubLastSeen, conn.KeyLastUsed)
}

// foundryLine says whether Foundry is answering, and which module it runs.
func foundryLine(c *Campaign, f *DeskFoundryFacts, now time.Time) DeskLine {
	l := DeskLine{ID: "foundry", Acts: []DeskAct{{Label: "Open Foundry", Href: deskHref(c.ID, "/foundry"), Carry: true}}}
	if f == nil {
		l.Parts = []DeskPart{txt("Foundry's link to this campaign, its module and its players are on the Foundry page.")}
		return l
	}
	conn := f.Connection
	st := ComputeFoundryStatus(now, conn)
	var s string
	switch st.Level {
	case FoundryConnectedNow:
		s, l.State = "Foundry is connected and answering now.", "ok"
	case FoundrySeenRecently:
		s, l.State = "Foundry last answered "+relativeAgo(now.Sub(*foundrySeen(conn)))+".", "ok"
	case FoundrySeenLongAgo:
		s, l.State = "Foundry last answered "+relativeAgo(now.Sub(*foundrySeen(conn)))+".", "warn"
	default:
		if conn.HasKey {
			s = "Foundry has a key but has not answered yet."
		} else {
			s = "Foundry is not set up yet."
		}
	}
	if conn.ModuleVersion != "" {
		s += " Its module is " + conn.ModuleVersion
		if conn.ServedVersion != "" && conn.ServedVersion != conn.ModuleVersion {
			s += "; Chronicle serves " + conn.ServedVersion
		}
		s += "."
	}
	l.Parts = []DeskPart{txt(s)}
	return l
}

// aiLine says whether the owner's AI is connected and what it may do.
func aiLine(c *Campaign, f *DeskAIFacts) DeskLine {
	l := DeskLine{ID: "ai", Acts: []DeskAct{{Label: "Open Your AI", Href: deskHref(c.ID, "/ai"), Carry: true}}}
	if f == nil {
		l.Parts = []DeskPart{txt("Connecting an AI app, and what it did, are on the Your AI page.")}
		return l
	}
	name := "Your AI"
	if f.Client != "" {
		name = "Your AI (" + f.Client + ")"
	}
	may := "may look and ask for changes"
	if !f.CanAsk {
		may = "may only look"
	}
	switch f.State {
	case DeskAIConnected:
		l.State = "ok"
		l.Parts = []DeskPart{txt(name + " is connected and " + may + ".")}
	case DeskAIPaused:
		l.Parts = []DeskPart{txt(name + " is paused, so it can do nothing until you resume it.")}
	case DeskAIEnded:
		l.Parts = []DeskPart{txt(name + "'s key has run out, so it can no longer get in.")}
	default:
		l.Parts = []DeskPart{txt("No AI is connected yet.")}
	}
	return l
}

// keysLine leads to where keys for other tools are made today; it carries no
// count because the keys page loads its own.
func keysLine(c *Campaign) DeskLine {
	return DeskLine{ID: "keys", Parts: []DeskPart{txt("Other tools get in with keys, which you make and take back in Settings.")},
		Acts: []DeskAct{{Label: "Keys", Href: deskHref(c.ID, "/settings?tab="+settingsTabAPIKeys)}}}
}

// trashLine says what waits in the campaign Trash.
func trashLine(c *Campaign, f *DeskTrashFacts) DeskLine {
	l := DeskLine{ID: "trash", Acts: []DeskAct{{Label: "Trash", Href: deskHref(c.ID, "/trash"), Carry: true}}}
	switch {
	case f == nil:
		l.Parts = []DeskPart{txt("Deleted pages wait in the Trash, where you can bring them back.")}
	case f.Items == 0:
		l.Parts = []DeskPart{txt("Trash is empty.")}
	default:
		l.Parts = []DeskPart{txt("Trash holds " + plural(f.Items, "item", "items") + " for " + plural(f.Days, "day", "days") + ".")}
	}
	return l
}

// endLine is the foot of the page: links to where archive and delete live.
// It only links; nothing here changes the campaign.
func endLine(c *Campaign) DeskLine {
	settings := deskHref(c.ID, "/settings?tab=general")
	if c.IsArchived() {
		return DeskLine{ID: "end", Parts: []DeskPart{txt("This campaign is archived and read-only. It can be brought back, or deleted.")},
			Acts: []DeskAct{{Label: "Unarchive", Href: settings}, {Label: "Delete", Href: settings, Danger: true}}}
	}
	return DeskLine{ID: "end", Parts: []DeskPart{txt("Finished with this campaign? It can be archived or deleted.")},
		Acts: []DeskAct{{Label: "Archive", Href: settings}, {Label: "Delete", Href: settings, Danger: true}}}
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
				Lead: "Foundry stopped answering", Rest: " " + relativeAgo(now.Sub(*seen)) + ".",
				Action: "See why", URL: deskHref(c.ID, "/foundry"), State: "warn",
			})
		}
	}
	if f.AI != nil && f.AI.Waiting > 0 {
		out = append(out, NeedsYouItem{
			Lead: "Your AI has " + plural(f.AI.Waiting, "request", "requests"), Rest: " waiting for your answer.",
			Action: "Look", URL: deskHref(c.ID, "/ai"),
		})
	}
	return out
}
