package calendarai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// skyDoc is the event kind's guide to `sky:`.
const skyDoc = "To put something in the calendar's sky on the event's days, add `sky:` with a `type`: blood, magic, conjunction, eclipse (lunar) or harvest (moon nights), comet, meteor-shower, meteor-storm, shooting-star, star-fall, aurora, arcane-aurora, eclipse-solar, blue-moon, hunters-moon, dark-night, or custom. The event's date (and end date, for a comet or an aurora over several nights) says when; its `hour` is the peak. Optional fields, as the type needs them: `moons` (moon names as the calendar spells them; a calendar with one moon may leave it out), `direction` (`compass` N, NE, E, SE, S, SW, W or NW and `altitude` low, mid or high; south and mid when left out), `color` (#rrggbb), `colors` (up to 2, for an aurora), `intensity` (0 to 1), `rate_per_hour` (1 to 2000 meteors), `moonlit` (true/false), `tail_degrees` (1 to 90, a comet's tail), `subtype` (total, annular or partial, a solar eclipse), `magnitude` (0.1 to 0.98, a partial eclipse), `duration_minutes` (1 to 600, totality), and for custom `shape` (streak, glow, ring, star, doorway, comet, curtain or veil), `size` (small, medium or large) and `duration_hours` (0.25 to 24). Any other field is refused. `sky: blood` is short for a type alone. Naming `sky:` on update replaces the event's whole sky event; `sky: none` removes it.\n\n" +
	"```\n---\nkind: event\nname: The Ember Comet\nyear: 1492\nmonth: Mirtul\nday: 14\nend_year: 1492\nend_month: Mirtul\nend_day: 26\nsky: {type: comet, direction: {compass: W, altitude: low}, color: \"#ffd9a0\", tail_degrees: 30}\n---\n```"

// skySpec is an event row's `sky:`: absent (keep), `none` (clear, update
// only) or a sky event, as the payload text the calendar stores. The text
// has already passed the calendar's own checks, so the review shows the
// same refusal the save would.
type skySpec struct {
	set, clear bool
	payload    string
	label      string // for the review summary
}

// skyTypeWords are the names an AI may write for a sky type besides the
// type itself, after lower-casing and turning spaces into hyphens.
var skyTypeWords = map[string]string{
	"blood-moon": calendar.MoonNightBlood, "magic-moon": calendar.MoonNightMagic,
	"harvest-moon": calendar.MoonNightHarvest, "lunar-eclipse": calendar.MoonNightEclipse,
	"moon-conjunction": calendar.MoonNightConjunction, "solar-eclipse": calendar.SkyEclipseSolar,
	"hunter-moon": calendar.SkyHuntersMoon, "meteor-showers": calendar.SkyMeteorShower,
	"meteors": calendar.SkyMeteorShower, "falling-star": calendar.SkyStarFall,
	"fireball": calendar.SkyShootingStar, "northern-lights": calendar.SkyAurora,
}

// skyMoonTypes are the types drawn on one moon, which a calendar with a
// single moon may leave unnamed. With no moon named, the sky moves the
// calendar's first moon across the sun, so eclipse-solar alone may name none
// on a calendar with several.
var skyMoonTypes = map[string]bool{
	calendar.MoonNightBlood: true, calendar.MoonNightMagic: true, calendar.MoonNightEclipse: true,
	calendar.MoonNightHarvest: true, calendar.SkyHuntersMoon: true, calendar.SkyBlueMoon: true,
	calendar.SkyEclipseSolar: true,
}

// skyTypeLabels say each type in plain words for the review.
var skyTypeLabels = map[string]string{
	calendar.MoonNightBlood: "a blood moon", calendar.MoonNightMagic: "a magic moon",
	calendar.MoonNightConjunction: "a conjunction of moons", calendar.MoonNightEclipse: "a lunar eclipse",
	calendar.MoonNightHarvest: "a harvest moon", calendar.SkyComet: "a comet",
	calendar.SkyMeteorShower: "a meteor shower", calendar.SkyMeteorStorm: "a meteor storm",
	calendar.SkyShootingStar: "a shooting star", calendar.SkyStarFall: "a falling star",
	calendar.SkyAurora: "an aurora", calendar.SkyArcaneAurora: "an arcane aurora",
	calendar.SkyEclipseSolar: "a solar eclipse", calendar.SkyBlueMoon: "a blue moon",
	calendar.SkyHuntersMoon: "a hunter's moon", calendar.SkyDarkNight: "a dark night",
	calendar.SkyCustom: "something of your own",
}

// skyCompassWords maps written directions to the compass points stored.
var skyCompassWords = map[string]string{
	"north": "N", "north-east": "NE", "northeast": "NE", "east": "E", "south-east": "SE",
	"southeast": "SE", "south": "S", "south-west": "SW", "southwest": "SW", "west": "W",
	"north-west": "NW", "northwest": "NW",
}

var skyCompassNames = map[string]string{
	"N": "north", "NE": "north-east", "E": "east", "SE": "south-east",
	"S": "south", "SW": "south-west", "W": "west", "NW": "north-west",
}

func skyWord(v any) string {
	s := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
	s = strings.NewReplacer("'", "", "’", "", "_", "-", " ", "-").Replace(s)
	return s
}

// readSky reads `sky:`. A map names a sky event's type and fields; a bare
// word is short for a map with only a type. Names the calendar owns (its
// moons) are matched as it spells them, and any other field is passed to
// the calendar's checks as written, so an unknown one is refused with the
// calendar's own words.
func readSky(cal *calendar.Calendar, r Record) (skySpec, error) {
	if !r.Has("sky") {
		return skySpec{}, nil
	}
	var fields map[string]any
	switch v := r.Fields["sky"].(type) {
	case string:
		if strings.EqualFold(strings.TrimSpace(v), pageNone) {
			if r.Action != ActionUpdate {
				return skySpec{}, badRequestf("sky: none only applies to action: update; leave sky out instead")
			}
			return skySpec{set: true, clear: true, label: "no sky event"}, nil
		}
		fields = map[string]any{"type": v}
	case map[string]any:
		fields = lowerMap(v)
	default:
		return skySpec{}, badRequestf("sky is a sky event, like `sky: {type: comet}`, or none")
	}

	typ := skyWord(fields["type"])
	if fields["type"] == nil || typ == "" {
		return skySpec{}, badRequestf("sky needs a type, one of %s", strings.Join(calendar.SkyEventTypes(), ", "))
	}
	if t, ok := skyTypeWords[typ]; ok {
		typ = t
	}
	if !calendar.IsSkyEventType(typ) {
		return skySpec{}, badRequestf("sky event: %q is not a kind of sky event; use one of %s", fmt.Sprint(fields["type"]), strings.Join(calendar.SkyEventTypes(), ", "))
	}
	fields["type"] = typ

	moonIDs, moonNames, err := readSkyMoons(cal, typ, fields)
	if err != nil {
		return skySpec{}, err
	}
	if len(moonIDs) > 0 {
		fields["moons"] = moonIDs
	} else {
		delete(fields, "moons")
	}
	if err := normalizeSkyDirection(fields); err != nil {
		return skySpec{}, err
	}
	for _, key := range []string{"subtype", "shape", "size"} {
		if s, ok := fields[key].(string); ok {
			fields[key] = strings.ToLower(strings.TrimSpace(s))
		}
	}

	raw, err := json.Marshal(fields)
	if err != nil {
		return skySpec{}, badRequestf("sky could not be read; write it as a map, like `sky: {type: comet}`")
	}
	p, err := calendar.ParseSkyEventPayload(string(raw))
	if err != nil {
		return skySpec{}, err
	}
	// Stored in the calendar's own field order, so the same sky reads the
	// same however the AI ordered it.
	out, _ := json.Marshal(p) // plain struct: cannot fail
	return skySpec{set: true, payload: string(out), label: skyLabel(p, moonNames)}, nil
}

// readSkyMoons turns `moons:` (names, or one name) into the calendar's
// moon ids, and fills in a one-moon calendar's moon for a type drawn on one.
func readSkyMoons(cal *calendar.Calendar, typ string, fields map[string]any) ([]int, []string, error) {
	var items []any
	switch v := fields["moons"].(type) {
	case nil:
	case []any:
		items = v
	default:
		items = []any{v}
	}
	moonList := names(cal.Moons, func(m calendar.Moon) string { return m.Name })
	var ids []int
	var picked []string
	seen := map[string]bool{}
	for _, it := range items {
		name := strings.TrimSpace(fmt.Sprint(it))
		found := false
		for _, mo := range cal.Moons {
			if sameName(mo.Name, name) {
				found = true
				if !seen[mo.Name] {
					seen[mo.Name] = true
					ids = append(ids, mo.ID)
					picked = append(picked, mo.Name)
				}
				break
			}
		}
		if !found {
			return nil, nil, badRequestf("sky event: %q is not a moon of %s (moons: %s)", name, cal.Name, moonList)
		}
	}
	if len(items) > 0 {
		return ids, picked, nil
	}
	switch {
	case skyMoonTypes[typ] && len(cal.Moons) == 1:
		return []int{cal.Moons[0].ID}, []string{cal.Moons[0].Name}, nil
	case typ == calendar.SkyEclipseSolar:
		return nil, nil, nil
	case skyMoonTypes[typ] && len(cal.Moons) == 0:
		return nil, nil, badRequestf("sky event: %s needs a moon, and %s has none", skyTypeLabels[typ], cal.Name)
	case skyMoonTypes[typ]:
		return nil, nil, badRequestf("sky event: say which moon %s is with moons (moons: %s)", skyTypeLabels[typ], moonList)
	case typ == calendar.MoonNightConjunction:
		return nil, nil, badRequestf("sky event: name the moons that meet with moons (moons: %s)", moonList)
	}
	return nil, nil, nil
}

// normalizeSkyDirection reads `direction:` written as a map, or as one
// compass word (`direction: west`), into the stored compass and altitude.
func normalizeSkyDirection(fields map[string]any) error {
	d, ok := fields["direction"]
	if !ok || d == nil {
		return nil
	}
	var dir map[string]any
	switch v := d.(type) {
	case string:
		dir = map[string]any{"compass": v}
	case map[string]any:
		dir = lowerMap(v)
	default:
		return nil // the calendar's check words the refusal
	}
	if c, ok := dir["compass"].(string); ok {
		w := skyWord(c)
		if pt, ok := skyCompassWords[w]; ok {
			dir["compass"] = pt
		} else {
			dir["compass"] = strings.ToUpper(w)
		}
	}
	if a, ok := dir["altitude"].(string); ok {
		dir["altitude"] = skyWord(a)
	}
	fields["direction"] = dir
	return nil
}

// skyLabel says the sky event in plain words: "in the sky: a comet, low in
// the west" or "in the sky: a blood moon (Selune)".
func skyLabel(p *calendar.SkyEventPayload, moons []string) string {
	what := skyTypeLabels[p.Type]
	switch {
	case p.Type == calendar.SkyEclipseSolar && p.Subtype != "":
		what = "a " + p.Subtype + " solar eclipse"
	case p.Type == calendar.SkyCustom && p.Shape != "":
		what = "a " + p.Shape + " of your own"
	}
	s := "in the sky: " + what
	if len(moons) > 0 {
		s += " (" + strings.Join(moons, ", ") + ")"
	}
	if d := p.Direction; d != nil && (d.Compass != "" || d.Altitude != "") {
		where := ""
		switch d.Altitude {
		case "low":
			where = "low"
		case "high":
			where = "high"
		}
		if d.Compass != "" {
			where = strings.TrimSpace(where + " in the " + skyCompassNames[d.Compass])
		}
		if where != "" {
			s += ", " + where
		}
	}
	return s
}

func withSky(summary string, sky skySpec) string {
	if sky.label != "" {
		return summary + ", " + sky.label
	}
	return summary
}

func (s skySpec) toCreate(in *calendar.CreateEventInput) {
	if s.set && !s.clear {
		p := s.payload
		in.Payload = &p
	}
}

func (s skySpec) toUpdate(in *calendar.UpdateEventInput) {
	switch {
	case s.clear:
		in.Payload = patch.Null[string]()
	case s.set:
		in.Payload = patch.Of(s.payload)
	}
}
