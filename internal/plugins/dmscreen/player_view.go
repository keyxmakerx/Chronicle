package dmscreen

import "fmt"

// player_view.go is the players' screen: the same docked rail and cards as the
// DM Screen, for the members who cannot open that one (players and scribes,
// and the DM team while viewing as a player). It is built by the same service
// from the same sources, but every source is asked as the player and the
// party's sheet values are narrowed here, so the screen never shows a value
// the player could not read on the hero's own page.

// CanOpenPlayerScreen reports whether a viewer gets the players' screen: a
// signed-in member who sees the campaign as a player or scribe. The DM team
// reads at Owner visibility and gets the DM Screen instead; while viewing as
// a player their role is lowered before it reaches here.
func (v Viewer) CanOpenPlayerScreen() bool {
	return v.UserID != "" && v.Role >= 1 && v.Role < 3
}

// PlayerView is everything the players' screen draws. It is not on any wire:
// only the site panel reads it.
type PlayerView struct {
	CampaignID string

	// HasParty is false when the party source is missing; My hero and The
	// party are then left out.
	HasParty bool
	// Hero is the viewer's own claimed hero, nil when they have none.
	Hero *HeroView
	// Party is every other claimed hero the viewer may open, with only the
	// sheet values a player may see.
	Party []HeroView
	// PartyFilled is true when the game system declares party meters.
	PartyFilled bool
	SystemName  string

	// PowerRoll is the system's power roll dice ("2d10"), empty when the
	// system declares none; My hero then offers no Power roll.
	PowerRoll string

	// World is today in the world as players see it, nil when there is no
	// calendar the viewer can see.
	World *WorldView

	// Conditions is empty when the system declares none; Rules is then left out.
	Conditions []ConditionView

	// Notes is the viewer's own private note, nil when notes are unavailable.
	Notes *NotesView
}

// PlayerSections lists the players' screen's sections in rail order. Tonight,
// Rolls and Dice are drawn in the browser by the Table, which every member
// has, so they are always offered; any other section whose source is missing
// is left out.
func (v *PlayerView) PlayerSections() []SectionView {
	var out []SectionView
	if v.HasParty {
		hero := SectionView{ID: "hero", Title: "My hero", Icon: "user-shield", Summary: "No hero yet"}
		if h := v.Hero; h != nil {
			hero.Summary = h.Name
			if bar, _, _ := h.Folded(); bar != nil {
				hero.Summary += fmt.Sprintf(" · %s %s of %s", bar.Label, bar.Current, bar.Max)
			}
			if len(h.Conditions) > 0 {
				hero.Summary += " · " + h.Conditions[0]
			}
		}
		out = append(out, hero)
		party := SectionView{ID: "party", Title: "The party", Icon: "users", Summary: "No other heroes yet"}
		if n := len(v.Party); n > 0 {
			party.Summary = plural(n, "other hero", "other heroes")
		}
		out = append(out, party)
	}
	if v.World != nil {
		world := SectionView{ID: "world", Title: "Today", Icon: "sun", Summary: v.World.DateLabel}
		if v.World.TimeLabel != "" {
			world.Summary += " · " + v.World.TimeLabel
		}
		out = append(out, world)
	}
	out = append(out,
		SectionView{ID: "tonight", Title: "Tonight", Icon: "scroll", Summary: "What you were shown tonight"},
		SectionView{ID: "rolls", Title: "Rolls", Icon: "dice", Summary: "The roll log and the roller"},
		SectionView{ID: "dice", Title: "Dice", Icon: "dice-d6", Summary: "How your dice look"},
	)
	if n := len(v.Conditions); n > 0 {
		out = append(out, SectionView{ID: "rules", Title: "Rules", Icon: "book", Summary: plural(n, "condition", "conditions")})
	}
	if v.Notes != nil {
		out = append(out, SectionView{ID: "notes", Title: "My notes", Icon: "pen", Summary: v.Notes.Label})
	}
	return out
}

// HeroLine is the line under the hero's name: "Tactician, level 3", either
// part alone, or empty.
func (h HeroView) HeroLine() string {
	switch {
	case h.Subtitle != "" && h.Level != "":
		return h.Subtitle + ", level " + h.Level
	case h.Level != "":
		return "Level " + h.Level
	}
	return h.Subtitle
}

// Initials are the hero's initials for the round badge beside the name.
func (h HeroView) Initials() string {
	var out []rune
	start := true
	for _, r := range h.Name {
		if r == ' ' {
			start = true
			continue
		}
		if start {
			out = append(out, r)
			start = false
		}
	}
	if len(out) > 2 {
		out = []rune{out[0], out[len(out)-1]}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}
