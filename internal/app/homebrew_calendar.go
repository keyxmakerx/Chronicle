package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// Limits on what a calendar rule may hold. They match what the calendar
// itself keeps: a moon or event name, a roll band's name, a short effect.
const (
	partyEffectNameChars = 80
	partyEffectTextChars = 300
	partyEffectTagChars  = 40
	calendarRefChars     = 80
	partyEffectMaxDays   = 3650
)

// Who a party effect falls on, who sees it, and how long it lasts.
const (
	effectWhoParty     = "party"
	effectWhoTag       = "tag"
	effectSeeParty     = "party"
	effectSeeDirector  = "director"
	effectUntilChange  = "until_change"
	effectLastsForDays = "duration"
)

// moonChangedArgs is a moon.changed rule's test: which moon (by name, so a
// recipe still works in another campaign) and, optionally, the state it
// changes to. No state means any change.
type moonChangedArgs struct {
	Moon  string `json:"moon"`
	State string `json:"state,omitempty"`
}

// calendarRollArgs is a calendar.roll rule's test: which recurring event (by
// name) and, optionally, the result's name. No result means any roll.
type calendarRollArgs struct {
	Event  string `json:"event"`
	Result string `json:"result,omitempty"`
}

// partyEffectArgs is what a party.effect action puts on the party: a named
// effect, on everyone or on pages with a tag, seen by the party or only the
// Director, lasting until the thing that caused it changes or for a number
// of days.
type partyEffectArgs struct {
	Name     string `json:"name"`
	Text     string `json:"text,omitempty"`
	Who      string `json:"who"`
	Tag      string `json:"tag,omitempty"`
	See      string `json:"see"`
	Lasts    string `json:"lasts"`
	Duration int    `json:"duration,omitempty"`
}

// registerCalendarHomebrew plugs the calendar into the Homebrew Workshop: a
// rule can start when a moon changes or a recurring event is rolled, and can
// put a party-wide effect on. None of these is tied to a part, so a recipe
// such as "Umbra goes dark: the party feels watched" needs no calendar part.
func registerCalendarHomebrew(reg *homebrew.Registry) error {
	regs := []func() error{
		func() error {
			return reg.RegisterEvent(homebrew.EventDef{Name: "moon.changed", Label: "A moon changes", Check: checkMoonChanged})
		},
		func() error {
			return reg.RegisterEvent(homebrew.EventDef{Name: "calendar.roll", Label: "A calendar event is rolled", Check: checkCalendarRoll})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: "party.effect", Label: "Put an effect on the party", Check: checkPartyEffect})
		},
	}
	for _, r := range regs {
		if err := r(); err != nil {
			return err
		}
	}
	return nil
}

func checkMoonChanged(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var a moonChangedArgs
	if err := strictJSON(raw, &a); err != nil {
		return nil, err
	}
	var err error
	if a.Moon, err = calendarText("the moon", a.Moon, calendarRefChars, true); err != nil {
		return nil, err
	}
	a.State = strings.ToLower(strings.TrimSpace(a.State))
	if a.State != "" && !calendar.ValidMoonState(a.State) {
		return nil, apperror.NewValidation("the moon's state must be one of " + strings.Join(calendar.MoonStates, ", "))
	}
	return json.Marshal(a)
}

func checkCalendarRoll(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var a calendarRollArgs
	if err := strictJSON(raw, &a); err != nil {
		return nil, err
	}
	var err error
	if a.Event, err = calendarText("the event", a.Event, calendarRefChars, true); err != nil {
		return nil, err
	}
	if a.Result, err = calendarText("the result", a.Result, calendarRefChars, false); err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

func checkPartyEffect(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var a partyEffectArgs
	if err := strictJSON(raw, &a); err != nil {
		return nil, err
	}
	var err error
	if a.Name, err = calendarText("the effect's name", a.Name, partyEffectNameChars, true); err != nil {
		return nil, err
	}
	if a.Text, err = calendarText("what the effect does", a.Text, partyEffectTextChars, false); err != nil {
		return nil, err
	}
	switch a.Who {
	case effectWhoParty:
		a.Tag = ""
	case effectWhoTag:
		if a.Tag, err = calendarText("the tag", a.Tag, partyEffectTagChars, true); err != nil {
			return nil, err
		}
	default:
		return nil, apperror.NewValidation("who it falls on must be party or tag")
	}
	if a.See != effectSeeParty && a.See != effectSeeDirector {
		return nil, apperror.NewValidation("who sees it must be party or director")
	}
	switch a.Lasts {
	case effectUntilChange:
		a.Duration = 0
	case effectLastsForDays:
		if a.Duration < 1 || a.Duration > partyEffectMaxDays {
			return nil, apperror.NewValidation(fmt.Sprintf("the effect lasts from 1 to %d days", partyEffectMaxDays))
		}
	default:
		return nil, apperror.NewValidation("how long it lasts must be until_change or duration")
	}
	return json.Marshal(a)
}

// calendarText trims a name or line and checks it is single-line text within
// max characters, so a recipe only ever holds text safe to show as-is.
func calendarText(field, s string, max int, required bool) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		if required {
			return "", apperror.NewValidation(field + " is required")
		}
		return "", nil
	}
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return "", apperror.NewValidation(fmt.Sprintf("%s is too long (at most %d characters)", field, max))
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", apperror.NewValidation(field + " contains a character that is not allowed")
		}
	}
	return s, nil
}
