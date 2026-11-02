package dmscreen

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
		out = append(out, SectionView{ID: "fight", Title: "Fight", Icon: "khanda", Summary: "Run a prepared encounter"})
	}
	party := SectionView{ID: "party", Title: "The party", Icon: "users", Summary: "No player characters yet"}
	if n := len(v.Party); n > 0 {
		party.Summary = plural(n, "hero", "heroes")
	}
	out = append(out, party)
	if n := len(v.Conditions); n > 0 {
		out = append(out, SectionView{ID: "rules", Title: "Rules", Icon: "book", Summary: plural(n, "condition", "conditions")})
	}
	reveal := SectionView{ID: "reveal", Title: "Reveal", Icon: "eye", Summary: "Nothing hidden right now"}
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
		if n := len(v.Adventures.Items); n > 0 {
			adv.Summary = plural(n, "card", "cards")
		}
		out = append(out, adv)
	}
	if v.Homebrew != nil {
		out = append(out, SectionView{ID: "homebrew", Title: "Homebrew", Icon: "beer-mug-empty", Summary: "Your table's own rules and items"})
	}
	return out
}
