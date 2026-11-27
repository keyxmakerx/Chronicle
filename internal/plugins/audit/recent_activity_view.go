package audit

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// sittingsStep is how many sittings the page shows, and how many more each
// "Show older sittings" adds.
const sittingsStep = 25

// ActivityView is what the Recent Activity tab draws.
type ActivityView struct {
	CampaignID string
	Feed       Feed
	Stats      *CampaignStats
	Now        time.Time
	// Loc is the viewer's time zone for every time shown (UTC when unset).
	Loc   *time.Location
	Shown int
}

// shownSittings is the sittings the page lists now.
func shownSittings(v ActivityView) []Sitting {
	if len(v.Feed.Sittings) > v.Shown {
		return v.Feed.Sittings[:v.Shown]
	}
	return v.Feed.Sittings
}

// sentence starts words with a capital: "created" reads "Created".
func sentence(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

func plural(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %ss", n, one)
}

// sittingSummary says in one line what a sitting did.
func sittingSummary(s Sitting) string {
	if s.Author == FoundryAuthor {
		line := "Brought " + plural(s.Count(KindFoundry), "change") + " in from Foundry"
		if s.Count(KindFoundry) == 0 {
			line = "Synced with Foundry"
		}
		if c := s.Count(KindConflict); c > 0 {
			line += ", " + plural(c, "conflict") + " settled"
		}
		return line
	}
	var parts []string
	if n := s.Count(KindSetting); n > 0 {
		parts = append(parts, plural(n, "setting"))
	}
	if n := distinctPages(s); n > 0 {
		parts = append(parts, plural(n, "page"))
	}
	if n := s.Count(KindPlayers); n > 0 {
		if n == 1 {
			parts = append(parts, "1 change to players")
		} else {
			parts = append(parts, fmt.Sprintf("%d changes to players", n))
		}
	}
	return "Changed " + listWords(parts)
}

// distinctPages counts the pages a sitting touched: five saves of one page
// are one page.
func distinctPages(s Sitting) int {
	seen, n := map[string]bool{}, 0
	for _, r := range s.Rows {
		if r.Kind != KindPage || r.Entry == nil {
			continue
		}
		if id := r.Entry.EntityID; id != "" {
			if seen[id] {
				continue
			}
			seen[id] = true
		}
		n++
	}
	return n
}

// sittingDay names a sitting's day the way people say it.
func sittingDay(v ActivityView, t time.Time) string {
	now, t := v.Now.In(v.Loc), t.In(v.Loc)
	day := func(x time.Time) time.Time { return time.Date(x.Year(), x.Month(), x.Day(), 0, 0, 0, 0, v.Loc) }
	switch days := int(day(now).Sub(day(t)).Hours()/24 + 0.5); {
	case days <= 0:
		return "Today"
	case days == 1:
		return "Yesterday"
	case days < 7:
		return t.Format("Monday")
	case t.Year() == now.Year():
		return t.Format("Mon 2 Jan")
	default:
		return t.Format("2 Jan 2006")
	}
}

func clockText(v ActivityView, t time.Time) string {
	return strings.ToLower(t.In(v.Loc).Format("3:04 PM"))
}

// sittingWhen is a sitting's day and span: "Today, 7:02 to 7:48pm".
func sittingWhen(v ActivityView, s Sitting) string {
	from, to := clockText(v, s.Start), clockText(v, s.End)
	if from == to {
		return sittingDay(v, s.Start) + ", " + from
	}
	return sittingDay(v, s.Start) + ", " + from + " to " + to
}

// tickLeft places a row's tick along its sitting's strip, 0 to 100.
func tickLeft(s Sitting, r ActivityRow) float64 {
	span := s.End.Sub(s.Start)
	if span <= 0 {
		return 50
	}
	return float64(r.At.Sub(s.Start)) / float64(span) * 100
}

// tickStyle keeps a tick 3px in from the strip's rounded ends.
func tickStyle(left float64) string {
	return fmt.Sprintf("left: calc(3px + (100%% - 8px) * %.3f)", left/100)
}

func authorInitial(s Sitting) string {
	if s.Author == FoundryAuthor {
		return "F"
	}
	return avatarInitial(s.AuthorName)
}

func authorName(s Sitting) string {
	if strings.TrimSpace(s.AuthorName) == "" {
		return "Someone"
	}
	return s.AuthorName
}

// rowWords is what a row says before its link: "Created", "Changed the
// welcome message", "Fields updated on".
func rowWords(r ActivityRow) string {
	if r.Entry != nil {
		return sentence(entryLabel(*r.Entry))
	}
	f := r.Foundry
	switch {
	case f.IsConflict():
		return ""
	case f.Direction == "link":
		return sentence(f.Action)
	}
	return ""
}

// rowName is the thing a row is about, shown as its link.
func rowName(r ActivityRow) string {
	if r.Entry != nil {
		return r.Entry.EntityName
	}
	return r.Foundry.Name
}

// rowTail is what follows the name: a Foundry change's own words.
func rowTail(r ActivityRow) string {
	if r.Foundry == nil {
		return ""
	}
	switch {
	case r.Foundry.IsConflict():
		return ": changed on both sides"
	case r.Foundry.Direction == "link":
		return ""
	case r.Foundry.Name == "":
		return sentence(r.Foundry.Action)
	}
	return ": " + r.Foundry.Action
}

// pageID is the page a row links to, if any.
func pageID(r ActivityRow) string {
	if r.Entry != nil {
		if r.Entry.EntityID != "" && (r.Entry.EntityType == "entity" || strings.HasPrefix(r.Entry.Action, "entity.")) {
			return r.Entry.EntityID
		}
		return ""
	}
	if r.Foundry.Kind == "page" || r.Foundry.Kind == "character" {
		return r.Foundry.ResourceID
	}
	return ""
}

// pageHref and historyHref escape the id: a conflict's id comes from the module.
func pageHref(v ActivityView, id string) string {
	return fmt.Sprintf("/campaigns/%s/entities/%s", v.CampaignID, url.PathEscape(id))
}

func historyHref(v ActivityView, id string) string {
	return fmt.Sprintf("/campaigns/%s/entities/%s/history", v.CampaignID, url.PathEscape(id))
}

// rowSide is the quiet link at a row's right: where to look further.
func rowSide(v ActivityView, r ActivityRow) (label, href string) {
	switch {
	case r.Foundry != nil:
		if r.Foundry.Direction == "link" {
			return "", ""
		}
		return "from Foundry", ""
	case r.Kind == KindPage && pageID(r) != "" && r.Entry.Action != ActionEntityDeleted:
		return "History", historyHref(v, pageID(r))
	case r.Kind == KindSetting:
		return "Settings", fmt.Sprintf("/campaigns/%s/settings", v.CampaignID)
	case r.Kind == KindPlayers:
		return "People", fmt.Sprintf("/campaigns/%s/members", v.CampaignID)
	}
	return "", ""
}

// rowText is what the search box matches a row against.
func rowText(s Sitting, r ActivityRow) string {
	return strings.ToLower(strings.Join([]string{authorName(s), rowWords(r), rowName(r), rowTail(r)}, " "))
}

// conflictKept names the side a conflict kept, for its pill.
func conflictKept(f *FoundryEvent) string {
	if f.FoundryKept() {
		return "Foundry kept"
	}
	return "Chronicle kept"
}

// kindCount is how many changes a chip shows.
func kindCount(v ActivityView, kind string) int {
	if kind == "" {
		return v.Feed.Total
	}
	return v.Feed.Counts[kind]
}

// moreSittings reports whether older sittings are waiting.
func moreSittings(v ActivityView) bool { return len(v.Feed.Sittings) > v.Shown }
