package dmscreen

import (
	"fmt"
	"strings"
)

// SectionView is one section of the docked screen: an icon on the rail and a
// card in the column. Summary is the one line a folded card and the icon's
// peek show; Badge is a count drawn on the icon (zero draws none).
type SectionView struct {
	ID      string
	Title   string
	Icon    string // Font Awesome solid icon name, without the fa- prefix
	Summary string
	Badge   int
}

// Waiting counts the requests waiting on the viewer: the listed ones plus
// those past the list, or the armory's pending count when no list is shown.
func (v *View) Waiting() int {
	if v.Requests != nil {
		return len(v.Requests.Items) + v.Requests.More
	}
	if v.Downtime != nil {
		return v.Downtime.Pending
	}
	return 0
}

// Sections lists the rail's sections in rail order. A section whose source is
// missing is left out, so the rail never offers an empty card.
func (v *View) Sections() []SectionView {
	world := SectionView{ID: "world", Title: "The world", Icon: "globe", Summary: "Game state", Badge: v.Waiting()}
	if v.World != nil {
		world.Summary = v.World.DateLabel
		if v.World.TimeLabel != "" {
			world.Summary += " · " + v.World.TimeLabel
		}
	}
	if n := v.Waiting(); n > 0 {
		world.Summary = plural(n, "request", "requests") + " waiting · " + world.Summary
	}
	out := []SectionView{world}
	if v.Fight != nil {
		fight := SectionView{ID: "fight", Title: "Fight", Icon: "khanda", Summary: "Run a prepared encounter"}
		if v.FightCounted {
			fight.Summary = "Nothing ready to run"
			if v.FightReady > 0 {
				fight.Summary = fmt.Sprintf("%d ready to run", v.FightReady)
			}
		}
		out = append(out, fight)
	}
	party := SectionView{ID: "party", Title: "The party", Icon: "users", Summary: "No player characters yet"}
	if n := len(v.Party); n > 0 {
		party.Summary = plural(n, "hero", "heroes")
		if v.Presence != nil {
			party.Summary += fmt.Sprintf(", %d here now", v.Presence.Here)
		}
	}
	out = append(out, party)
	if v.Decks != nil {
		out = append(out, SectionView{ID: "decks", Title: "Card decks", Icon: "layer-group", Summary: v.DecksSummary})
	}
	if n := len(v.Conditions); n > 0 {
		out = append(out, SectionView{ID: "rules", Title: "Rules", Icon: "book", Summary: plural(n, "condition", "conditions")})
	}
	reveal := SectionView{ID: "reveal", Title: "Reveal", Icon: "eye", Summary: "Nothing hidden"}
	if n := len(v.Hidden); n > 0 {
		reveal.Summary = plural(n, "character", "characters") + " hidden"
		if v.HiddenMore {
			reveal.Summary = "More than " + reveal.Summary
		}
	}
	out = append(out, reveal)
	if v.Notes != nil {
		out = append(out, SectionView{ID: "notes", Title: "Notes", Icon: "pen", Summary: v.Notes.Label})
	}
	if v.Adventures != nil {
		adv := SectionView{ID: "adventures", Title: "Adventures", Icon: "map", Summary: "Nothing pinned yet"}
		var pages, notes int
		for _, it := range v.Adventures.Items {
			if it.Kind == BoardKindNote {
				notes++
			} else {
				pages++
			}
		}
		var bits []string
		if pages > 0 {
			bits = append(bits, fmt.Sprintf("%d pinned", pages))
		}
		if notes > 0 {
			bits = append(bits, plural(notes, "note", "notes"))
		}
		if len(bits) > 0 {
			adv.Summary = strings.Join(bits, ", ")
		}
		out = append(out, adv)
	}
	if v.Homebrew != nil {
		out = append(out, SectionView{ID: "homebrew", Title: "Homebrew", Icon: "beer-mug-empty", Summary: "House rules"})
	}
	return out
}
