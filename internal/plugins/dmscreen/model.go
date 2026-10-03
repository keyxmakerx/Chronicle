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

// View is everything the panel template draws. The Foundry module gets the
// same data as JSON from the sync API, so the field tags are a wire contract.
type View struct {
	CampaignID string `json:"campaign_id"`

	// Downtime is nil when the campaign has no armory (no downtime switch).
	Downtime *DowntimeView `json:"downtime,omitempty"`

	// World is nil when the campaign has no calendar the viewer can see.
	World *WorldView `json:"world,omitempty"`

	// Night is nil when no game night is coming up.
	Night *NightView `json:"night,omitempty"`

	Foundry FoundryView `json:"foundry"`

	// SystemName is the enabled game system, empty when none.
	SystemName string `json:"system_name"`

	// PartyFilled is true when the game system declares party meters; when
	// false the party panel lists names only and says why.
	PartyFilled bool       `json:"party_filled"`
	Party       []HeroView `json:"party,omitempty"`

	// Hidden lists characters players can't see yet, newest first.
	Hidden []HiddenView `json:"hidden,omitempty"`

	// Conditions is empty when the system declares none; the tab is hidden.
	Conditions []ConditionView `json:"conditions,omitempty"`
}

// DowntimeView is the downtime switch's state plus requests waiting on the GM.
type DowntimeView struct {
	Open      bool `json:"open"`
	CanToggle bool `json:"can_toggle"`
	Pending   int  `json:"pending"`
}

// WorldView is the in-world date and today's weather.
type WorldView struct {
	CalendarID string `json:"calendar_id"`
	DateLabel  string `json:"date_label"`
	TimeLabel  string `json:"time_label"`
	// Weather is a one-line description, empty when none is set for today.
	Weather string `json:"weather"`
}

// NightView is the next game night and its answers so far.
type NightView struct {
	Name     string `json:"name"`
	When     string `json:"when"`
	Going    int    `json:"going"`
	Maybe    int    `json:"maybe"`
	Cant     int    `json:"cant"`
	NoAnswer int    `json:"no_answer"`
}

// FoundryView says whether the campaign's Foundry world is connected.
type FoundryView struct {
	Connected bool       `json:"connected"`
	NeverSeen bool       `json:"never_seen"`
	LastSeen  *time.Time `json:"last_seen,omitempty"`
}

// HeroView is one player character with the meters the system declares.
type HeroView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PlayerName string `json:"player_name"`
	// Subtitle and Conditions come from the sheet fields the system's
	// manifest names; either may be empty.
	Subtitle   string      `json:"subtitle"`
	Conditions []string    `json:"conditions,omitempty"`
	Meters     []MeterView `json:"meters,omitempty"`
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
	Label   string `json:"label"`
	Current string `json:"current"`
	Max     string `json:"max"`
	HasMax  bool   `json:"has_max"`
	// Percent is 0-100 for the bar width.
	Percent int  `json:"percent"`
	Low     bool `json:"low"`
}

// HiddenView is a character the GM can reveal from the screen.
type HiddenView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	TypeName string `json:"type_name"`
	Revealed bool   `json:"revealed"`
}

// ConditionView is one condition and its rule text, markup flattened.
type ConditionView struct {
	Name string `json:"name"`
	Text string `json:"text"`
}
