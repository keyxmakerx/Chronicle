// Package dmscreen serves the DM Screen: a control panel for the people
// running a campaign (owners and scribes), opened from the campaign top bar.
// It owns no tables. Every section is read from another plugin through the
// source interfaces in sources.go, and a section whose source is missing or
// fails is left out rather than failing the whole screen.
package dmscreen

import "time"

// Viewer is the person opening the screen.
type Viewer struct {
	UserID string
	// Role is the campaign visibility role (3 owner or co-DM, 2 scribe).
	Role int
}

// IsOwner reports whether the viewer may change owner-only game state such
// as the downtime switch.
func (v Viewer) IsOwner() bool { return v.Role >= 3 }

// View is everything the panel template draws.
type View struct {
	CampaignID string

	// Downtime is nil when the campaign has no armory (no downtime switch).
	Downtime *DowntimeView

	// World is nil when the campaign has no calendar the viewer can see.
	World *WorldView

	// Night is nil when no game night is coming up.
	Night *NightView

	Foundry FoundryView

	// SystemName is the enabled game system, empty when none.
	SystemName string

	// PartyFilled is true when the game system declares party meters; when
	// false the party panel lists names only and says why.
	PartyFilled bool
	Party       []HeroView

	// Hidden lists characters players can't see yet, newest first.
	Hidden []HiddenView

	// Conditions is empty when the system declares none; the tab is hidden.
	Conditions []ConditionView
}

// DowntimeView is the downtime switch's state plus requests waiting on the GM.
type DowntimeView struct {
	Open      bool
	CanToggle bool
	Pending   int
}

// WorldView is the in-world date and today's weather.
type WorldView struct {
	CalendarID string
	DateLabel  string
	TimeLabel  string
	// Weather is a one-line description, empty when none is set for today.
	Weather string
}

// NightView is the next game night and its answers so far.
type NightView struct {
	Name     string
	When     string
	Going    int
	Maybe    int
	Cant     int
	NoAnswer int
}

// FoundryView says whether the campaign's Foundry world is connected.
type FoundryView struct {
	Connected bool
	NeverSeen bool
	LastSeen  *time.Time
}

// HeroView is one player character with the meters the system declares.
type HeroView struct {
	ID         string
	Name       string
	PlayerName string
	Meters     []MeterView
}

// Folded splits a hero's meters for the one-line folded row: the first
// meter with a max becomes the bar, and every meter without a max (a class
// resource, an armour class) becomes a chip. Rest is what folds open.
func (h HeroView) Folded() (bar *MeterView, chips, rest []MeterView) {
	for i := range h.Meters {
		m := h.Meters[i]
		switch {
		case !m.HasMax:
			chips = append(chips, m)
		case bar == nil:
			bar = &h.Meters[i]
		default:
			rest = append(rest, m)
		}
	}
	return bar, chips, rest
}

// MeterView is one number on a hero. HasMax draws it as a bar.
type MeterView struct {
	Label   string
	Current string
	Max     string
	HasMax  bool
	// Percent is 0-100 for the bar width.
	Percent int
	Low     bool
}

// HiddenView is a character the GM can reveal from the screen.
type HiddenView struct {
	ID       string
	Name     string
	TypeName string
	Revealed bool
}

// ConditionView is one condition and its rule text, markup flattened.
type ConditionView struct {
	Name string
	Text string
}
