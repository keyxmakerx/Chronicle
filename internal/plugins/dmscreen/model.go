// Package dmscreen serves the DM Screen: a control panel for the people
// running a campaign (owners and scribes), opened from the campaign top bar.
// It owns no tables. Every section is read from another plugin through the
// source interfaces in sources.go, and a section whose source is missing or
// fails is left out rather than failing the whole screen.
package dmscreen

import (
	"fmt"
	"strings"
	"time"
)

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

	// Notes is nil when notes are unavailable; the Notes tab is then hidden.
	Notes *NotesView `json:"notes,omitempty"`

	// Requests is nil when the campaign has no armory or nothing waits.
	Requests *RequestsView `json:"requests,omitempty"`

	// Presence is nil when the campaign has no players or the source is not
	// wired; the strip then says nothing about who is here.
	Presence *PresenceView `json:"presence,omitempty"`

	// SystemName is the enabled game system, empty when none.
	SystemName string `json:"system_name"`

	// PartyFilled is true when the game system declares party meters; when
	// false the party panel lists names only and says why.
	PartyFilled bool       `json:"party_filled"`
	Party       []HeroView `json:"party,omitempty"`

	// Hidden lists characters players can't see yet, newest first.
	Hidden []HiddenView `json:"hidden,omitempty"`

	// HiddenMore is true when more are hidden than Hidden lists.
	HiddenMore bool `json:"hidden_more,omitempty"`

	// Conditions is empty when the system declares none; the tab is hidden.
	Conditions []ConditionView `json:"conditions,omitempty"`
}

// DowntimeView is the downtime switch's state plus requests waiting on the GM.
type DowntimeView struct {
	Open      bool `json:"open"`
	CanToggle bool `json:"can_toggle"`
	Pending   int  `json:"pending"`
}

// Confirm is the question the switch asks before changing anything, and
// its yes button. Starting downtime puts every waiting request through at
// once, so a stray click must not do it.
func (d DowntimeView) Confirm() (text, yes string) {
	if d.Open {
		return "End downtime? Moves will need your OK again and shops close.", "End downtime"
	}
	switch d.Pending {
	case 0:
		return "Start downtime? Moves will happen at once and shops open.", "Start downtime"
	case 1:
		return "Start downtime? 1 waiting request goes through now and shops open.", "Start downtime"
	}
	return fmt.Sprintf("Start downtime? %d waiting requests go through now and shops open.", d.Pending), "Start downtime"
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
	// PlayerUserID and PlayerHere are additive: the dot after "Played by"
	// needs to know whether that player is here right now.
	PlayerUserID string `json:"player_user_id,omitempty"`
	PlayerHere   bool   `json:"player_here,omitempty"`
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

// PresenceView counts the campaign's players who are here right now. A player
// is a member with the Player role; the owner, co-DMs and scribes run the game
// and are not counted.
type PresenceView struct {
	Here  int `json:"here"`
	Total int `json:"total"`
	// HereNames and AwayNames are display names, sorted, for the strip's tooltip.
	HereNames []string `json:"here_names,omitempty"`
	AwayNames []string `json:"away_names,omitempty"`
}

// Label is the strip text: "3 of 4 players here".
func (p PresenceView) Label() string {
	noun := "players"
	if p.Total == 1 {
		noun = "player"
	}
	return fmt.Sprintf("%d of %d %s here", p.Here, p.Total, noun)
}

// Title is the strip's tooltip: "Here: a, b. Not here: c."
func (p PresenceView) Title() string {
	list := func(names []string) string {
		if len(names) == 0 {
			return "nobody"
		}
		return strings.Join(names, ", ")
	}
	return "Here: " + list(p.HereNames) + ". Not here: " + list(p.AwayNames) + "."
}

// Request kinds, which pick the armory route a button posts to.
const (
	RequestMove     = "move"
	RequestPurchase = "purchase"
)

// RequestView is one stash move or ask-to-buy request waiting on the owner.
type RequestView struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
	// Text is one plain line: "Bren: move 2 × Healing potion from A to B".
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// ApprovePath and DeclinePath are the armory routes the buttons post to.
func (r RequestView) ApprovePath(campaignID string) string { return r.path(campaignID, "approve") }
func (r RequestView) DeclinePath(campaignID string) string { return r.path(campaignID, "decline") }

func (r RequestView) path(campaignID, verb string) string {
	dir := "moves"
	if r.Kind == RequestPurchase {
		dir = "purchase-requests"
	}
	return fmt.Sprintf("/campaigns/%s/armory/%s/%d/%s", campaignID, dir, r.ID, verb)
}

// RequestsView is the "Waiting on you" block: the oldest few requests, and
// how many more sit on the Stashes page.
type RequestsView struct {
	Items []RequestView `json:"items"`
	More  int           `json:"more,omitempty"`
	// CanAnswer is true for the owner and DM-granted co-DMs, the only people
	// the armory lets approve or refuse; others would see rows without buttons.
	CanAnswer bool `json:"can_answer"`
}

// NotesView is the screen's note: one per campaign, shared with the GM side,
// kept with the next game night.
type NotesView struct {
	// Label reads "Kept with <game night>" or "DM Screen notes".
	Label string `json:"label"`
	// Text is the note as plain lines; empty before anything is saved.
	Text string `json:"text"`
	// NoteID is empty until the first save creates the note.
	NoteID string `json:"note_id,omitempty"`
	// Link opens the note in the Journal; empty until it exists.
	Link string `json:"link,omitempty"`
}
