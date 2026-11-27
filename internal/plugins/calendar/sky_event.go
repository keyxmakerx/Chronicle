package calendar

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Sky event types: what an Event.Payload's "type" names for something the
// Sky Pane draws on the event's days. The five moon nights (MoonNight*) are
// sky events too; these are the rest.
const (
	SkyComet        = "comet"
	SkyMeteorShower = "meteor-shower"
	SkyMeteorStorm  = "meteor-storm"
	SkyShootingStar = "shooting-star"
	SkyStarFall     = "star-fall"
	SkyAurora       = "aurora"
	SkyArcaneAurora = "arcane-aurora"
	SkyEclipseSolar = "eclipse-solar"
	SkyBlueMoon     = "blue-moon"
	SkyHuntersMoon  = "hunters-moon"
	SkyDarkNight    = "dark-night"
	SkyCustom       = "custom"
)

// skyEventTypes is every sky type in the order people are shown them.
var skyEventTypes = []string{
	MoonNightBlood, MoonNightMagic, MoonNightConjunction, MoonNightEclipse, MoonNightHarvest,
	SkyComet, SkyMeteorShower, SkyMeteorStorm, SkyShootingStar, SkyStarFall,
	SkyAurora, SkyArcaneAurora, SkyEclipseSolar, SkyBlueMoon, SkyHuntersMoon,
	SkyDarkNight, SkyCustom,
}

// SkyEventTypes returns every sky event type, in display order.
func SkyEventTypes() []string { return append([]string(nil), skyEventTypes...) }

// IsSkyEventType reports whether t is one of the sky event types, the five
// moon nights included.
func IsSkyEventType(t string) bool {
	for _, s := range skyEventTypes {
		if s == t {
			return true
		}
	}
	return false
}

// Allowed words for the enum fields. Lists, not maps, so messages name them
// in a stable order.
var (
	skyCompassPoints  = []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	skyAltitudes      = []string{"low", "mid", "high"}
	skyEclipseKinds   = []string{"total", "annular", "partial"}
	skyCustomShapes   = []string{"streak", "glow", "ring", "star", "doorway", "comet", "curtain", "veil"}
	skySizes          = []string{"small", "medium", "large"}
	skyHexColor       = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	maxSkyEventMoons  = 8
	maxSkyEventColors = 2
)

// maxEventPayloadBytes caps an event's payload text. A sky event is a few
// hundred bytes; the column is a JSON (LONGTEXT) slot with no size of its
// own, so without this one write could store megabytes every event read
// then carries.
const maxEventPayloadBytes = 16 << 10

// SkyDirection is where in the sky a sight sits. Empty fields mean the
// defaults the sky draws with (south, mid altitude).
type SkyDirection struct {
	Compass  string `json:"compass,omitempty"`
	Altitude string `json:"altitude,omitempty"`
}

// SkyEventPayload is the sky event shape an Event.Payload carries: a
// superset of MoonNightPayload, so a stored moon night reads as either.
// Every field but Type is optional and each type reads only some of them;
// a field a type does not read is kept but ignored, so switching an event's
// type in the editor never refuses the save. Numbers are pointers so an
// absent one is told apart from a zero, which is out of range for some
// fields and meaningful for others (intensity 0).
type SkyEventPayload struct {
	Type            string        `json:"type"`
	Moons           []int         `json:"moons,omitempty"`
	Direction       *SkyDirection `json:"direction,omitempty"`
	Color           string        `json:"color,omitempty"`
	Colors          []string      `json:"colors,omitempty"`
	Intensity       *float64      `json:"intensity,omitempty"`
	RatePerHour     *int          `json:"rate_per_hour,omitempty"`
	Moonlit         *bool         `json:"moonlit,omitempty"`
	TailDegrees     *float64      `json:"tail_degrees,omitempty"`
	Subtype         string        `json:"subtype,omitempty"`
	Magnitude       *float64      `json:"magnitude,omitempty"`
	DurationMinutes *int          `json:"duration_minutes,omitempty"`
	DurationHours   *float64      `json:"duration_hours,omitempty"`
	Shape           string        `json:"shape,omitempty"`
	Size            string        `json:"size,omitempty"`
}

// validEventPayload rejects a payload that is too long or not JSON before it
// ever reaches the database; calendar_events.payload has no CHECK constraint,
// so a malformed value would otherwise sit silently until a reader fails on
// it. A sky event (see ParseSkyEventPayload) is also checked field by field;
// any other shape belongs to the feature that wrote it and is only held to
// being JSON.
func validEventPayload(payload *string) error {
	if payload == nil || *payload == "" {
		return nil
	}
	if len(*payload) > maxEventPayloadBytes {
		return apperror.NewValidation(fmt.Sprintf("payload is too long (at most %d bytes)", maxEventPayloadBytes))
	}
	if !json.Valid([]byte(*payload)) {
		return apperror.NewValidation("payload is not valid JSON")
	}
	_, err := ParseSkyEventPayload(*payload)
	return err
}

// skyFieldWants is what each field must be, for a decode error worded for
// a person rather than as Go's "cannot unmarshal" text.
var skyFieldWants = map[string]string{
	"type":               "a word, like comet",
	"moons":              "a list of moons",
	"direction":          `a compass point and an altitude, like {"compass":"W","altitude":"low"}`,
	"direction.compass":  "a compass point, like W",
	"direction.altitude": "low, mid or high",
	"color":              "a colour written like #ffd9a0",
	"colors":             "a list of up to 2 colours written like #ffd9a0",
	"intensity":          "a number from 0 to 1",
	"rate_per_hour":      "a whole number from 1 to 2000",
	"moonlit":            "true or false",
	"tail_degrees":       "a number from 1 to 90",
	"subtype":            "total, annular or partial",
	"magnitude":          "a number from 0.1 to 0.98",
	"duration_minutes":   "a whole number from 1 to 600",
	"duration_hours":     "a number from 0.25 to 24",
	"shape":              "one of " + strings.Join(skyCustomShapes, ", "),
	"size":               "small, medium or large",
}

// skyError is a refusal of a sky event, worded the same wherever it is
// shown: the event form, the API and the AI import's review.
func skyError(format string, args ...any) error {
	return apperror.NewValidation("sky event: " + fmt.Sprintf(format, args...))
}

// ParseSkyEventPayload reads raw as a sky event. It returns (nil, nil) when
// raw is not one: empty, not a JSON object, or a "type" that is not a sky
// type, which is another feature's shape and stays its own business. A sky
// type is read strictly: an unknown field, a wrong kind of value or a value
// out of range is an error, so a typo is refused rather than ignored.
func ParseSkyEventPayload(raw string) (*SkyEventPayload, error) {
	var head struct {
		Type any `json:"type"`
	}
	if raw == "" || json.Unmarshal([]byte(raw), &head) != nil {
		return nil, nil
	}
	t, ok := head.Type.(string)
	if !ok || !IsSkyEventType(t) {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	var p SkyEventPayload
	if err := dec.Decode(&p); err != nil {
		return nil, skyDecodeError(err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// skyDecodeError words a strict decode failure for a person.
func skyDecodeError(err error) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		// The path names list positions ("moons.0"); the wording is per field.
		var parts []string
		for _, part := range strings.Split(typeErr.Field, ".") {
			if strings.Trim(part, "0123456789") != "" {
				parts = append(parts, part)
			}
		}
		field := strings.Join(parts, ".")
		if want, ok := skyFieldWants[field]; ok {
			return skyError("%s must be %s", field, want)
		}
		return skyError("%s has the wrong kind of value", field)
	}
	// encoding/json has no typed error for an unknown field; its text is
	// stable: `json: unknown field "name"`.
	if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return skyError("%s is not a sky event field", name)
	}
	return skyError("it could not be read")
}

// Validate checks every field's value against what the sky can draw. It
// does not check that the moons belong to the event's calendar; that needs
// the calendar, so the service does it (see checkSkyEventMoons).
func (p *SkyEventPayload) Validate() error {
	if !IsSkyEventType(p.Type) {
		return skyError("%q is not a kind of sky event; use one of %s", p.Type, strings.Join(skyEventTypes, ", "))
	}
	if len(p.Moons) > maxSkyEventMoons {
		return skyError("moons lists at most %d moons", maxSkyEventMoons)
	}
	if d := p.Direction; d != nil {
		if d.Compass != "" && !oneOf(d.Compass, skyCompassPoints) {
			return skyError("direction compass must be one of %s", strings.Join(skyCompassPoints, ", "))
		}
		if d.Altitude != "" && !oneOf(d.Altitude, skyAltitudes) {
			return skyError("direction altitude must be low, mid or high")
		}
	}
	if p.Color != "" && !skyHexColor.MatchString(p.Color) {
		return skyError("color must be a colour written like #ffd9a0")
	}
	if len(p.Colors) > maxSkyEventColors {
		return skyError("colors lists at most %d colours", maxSkyEventColors)
	}
	for _, c := range p.Colors {
		if !skyHexColor.MatchString(c) {
			return skyError("colors: %q must be a colour written like #ffd9a0", c)
		}
	}
	type numRange struct {
		name     string
		v        *float64
		min, max float64
	}
	for _, n := range []numRange{
		{"intensity", p.Intensity, 0, 1},
		{"tail_degrees", p.TailDegrees, 1, 90},
		{"magnitude", p.Magnitude, 0.1, 0.98},
		{"duration_hours", p.DurationHours, 0.25, 24},
	} {
		if n.v != nil && (*n.v < n.min || *n.v > n.max) {
			return skyError("%s must be %s", n.name, skyFieldWants[n.name])
		}
	}
	type intRange struct {
		name     string
		v        *int
		min, max int
	}
	for _, n := range []intRange{
		{"rate_per_hour", p.RatePerHour, 1, 2000},
		{"duration_minutes", p.DurationMinutes, 1, 600},
	} {
		if n.v != nil && (*n.v < n.min || *n.v > n.max) {
			return skyError("%s must be %s", n.name, skyFieldWants[n.name])
		}
	}
	if p.Subtype != "" && !oneOf(p.Subtype, skyEclipseKinds) {
		return skyError("subtype must be total, annular or partial")
	}
	if p.Shape != "" && !oneOf(p.Shape, skyCustomShapes) {
		return skyError("shape must be one of %s", strings.Join(skyCustomShapes, ", "))
	}
	if p.Size != "" && !oneOf(p.Size, skySizes) {
		return skyError("size must be small, medium or large")
	}
	return nil
}

func oneOf(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// SkyEvent returns the event's sky event, or nil when it has none or its
// payload does not read as one. A stored payload was checked when it was
// written, so nil here means "not a sky event", not a fault to report.
func (e *Event) SkyEvent() *SkyEventPayload {
	if e.Payload == nil {
		return nil
	}
	p, err := ParseSkyEventPayload(*e.Payload)
	if err != nil {
		return nil
	}
	return p
}

// skyEventMoonsUnknown returns the first of p's moons that is not one of
// moons, and whether there is one.
func skyEventMoonsUnknown(p *SkyEventPayload, moons []Moon) (int, bool) {
	for _, id := range p.Moons {
		found := false
		for _, m := range moons {
			if m.ID == id {
				found = true
				break
			}
		}
		if !found {
			return id, true
		}
	}
	return 0, false
}

// ImportedEventPayload readies an imported event's payload for the calendar
// the import made. A sky event's moon ids are the exporting calendar's, so
// each is rewritten through moons (old id to new); one with no mapping is
// left off. A sky event that does not pass today's checks is left off
// whole. Either way the event itself is still imported, and note says what
// was left off ("" when nothing was): an import warns, it does not refuse
// an event over its sky. Any other payload comes back unchanged.
func ImportedEventPayload(payload *string, moons map[int]int) (*string, string) {
	if payload == nil {
		return nil, ""
	}
	p, err := ParseSkyEventPayload(*payload)
	if err != nil {
		return nil, "its sky event was left off (" + apperror.SafeMessage(err) + ")"
	}
	if p == nil || len(p.Moons) == 0 {
		return payload, ""
	}
	kept := make([]int, 0, len(p.Moons))
	for _, id := range p.Moons {
		if to, ok := moons[id]; ok && to != 0 {
			kept = append(kept, to)
		}
	}
	note := ""
	if dropped := len(p.Moons) - len(kept); dropped > 0 {
		note = fmt.Sprintf("%d of its sky event's moons could not be matched to this calendar's and were left off", dropped)
	}
	p.Moons = kept
	out, err := json.Marshal(p)
	if err != nil {
		return payload, note // plain struct: cannot fail
	}
	s := string(out)
	return &s, note
}

// PayloadNamesMoons reports whether payload is a sky event naming moons,
// so an import knows it must map moon ids even when no repeat rule does.
func PayloadNamesMoons(payload *string) bool {
	if payload == nil {
		return false
	}
	p, err := ParseSkyEventPayload(*payload)
	return err == nil && p != nil && len(p.Moons) > 0
}
