package calendarai

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EraLookWriter saves the calendar-wide era look, the way the calendar's
// settings do. calendar.CalendarService is one.
type EraLookWriter interface {
	SaveEraLook(ctx context.Context, calendarID, campaignID string, look calendar.EraLook, eras []calendar.EraLookEra) error
}

// calOptions are the calendar block's simple stored settings and the
// campaign's event importance levels. A nil or absent value keeps what is
// stored; only what the block wrote changes.
type calOptions struct {
	tiers      []campaigns.TierDefinition // nil when the block has no `tiers`
	hemisphere patch.Field[string]
	forecasts  *bool
	newWeek    *bool
	eraLook    *eraLookSpec
}

// eraLookSpec is an `era_look` map: only the keys written change.
type eraLookSpec struct {
	colorsOn  *bool
	feel      string
	intensity *float64
	speed     *float64
}

// readBool reads a yes/no key strictly: Record.Bool would read a typo as
// "no", which for a switch the owner may not want turned off.
func readBool(r Record, key string) (v, ok bool, err error) {
	raw, has := r.Fields[key]
	if !has || raw == nil {
		return false, false, nil
	}
	switch t := raw.(type) {
	case bool:
		return t, true, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "yes", "on":
			return true, true, nil
		case "false", "no", "off":
			return false, true, nil
		}
	}
	return false, true, badRequestf("%s must be true or false", key)
}

func readCalOptions(r Record) (calOptions, error) {
	var o calOptions
	if r.Has("tiers") {
		defs, err := readTiers(r)
		if err != nil {
			return o, err
		}
		o.tiers = defs
	}
	if r.Has("hemisphere") {
		switch h := strings.ToLower(r.Str("hemisphere")); h {
		case calendar.HemisphereNorth, calendar.HemisphereSouth:
			o.hemisphere = patch.Of(h)
		case "none":
			o.hemisphere = patch.Null[string]()
		default:
			return o, badRequestf("hemisphere must be %s, %s or none (to clear it)", calendar.HemisphereNorth, calendar.HemisphereSouth)
		}
	}
	for _, p := range []struct {
		key string
		dst **bool
	}{{"forecasts", &o.forecasts}, {"month_starts_new_week", &o.newWeek}} {
		v, ok, err := readBool(r, p.key)
		if err != nil {
			return o, err
		}
		if ok {
			*p.dst = &v
		}
	}
	if r.Has("era_look") {
		l, err := readEraLook(r)
		if err != nil {
			return o, err
		}
		o.eraLook = l
	}
	return o, nil
}

// readTiers reads `tiers`: the whole set of importance levels, which the
// campaign's screen also replaces whole.
func readTiers(r Record) ([]campaigns.TierDefinition, error) {
	items, err := listItems(r, "tiers", false)
	if err != nil {
		return nil, err
	}
	defs := make([]campaigns.TierDefinition, 0, len(items))
	for _, it := range items {
		d := campaigns.TierDefinition{Slug: it.Str("slug"), Name: it.Name, Color: it.Str("color")}
		p, ok, err := it.Int("prominence")
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, badRequestf("tier %s needs prominence, 0 to 100", quote(it.Name))
		}
		d.Prominence = p
		d.IsDefault, _, err = readBool(it, "default")
		if err != nil {
			return nil, badRequestf("tier %s: %s", quote(it.Name), planError(err))
		}
		defs = append(defs, d)
	}
	if err := campaigns.ValidateTierDefinitions(defs); err != nil {
		return nil, badRequestf("tiers: %s", planError(err))
	}
	return defs, nil
}

func sameTiers(a, b []campaigns.TierDefinition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Slug != b[i].Slug || a[i].Name != b[i].Name || !strings.EqualFold(a[i].Color, b[i].Color) ||
			a[i].Prominence != b[i].Prominence || a[i].IsDefault != b[i].IsDefault {
			return false
		}
	}
	return true
}

func readEraLook(r Record) (*eraLookSpec, error) {
	m, ok := r.Fields["era_look"].(map[string]any)
	if !ok {
		return nil, badRequestf("era_look must be keys such as `colors_on`, `feel`, `intensity` and `speed`")
	}
	f := lowerMap(m)
	for k := range f {
		switch k {
		case "colors_on", "feel", "intensity", "speed":
		default:
			return nil, badRequestf("era_look: %q is not a key (use colors_on, feel, intensity, speed)", k)
		}
	}
	sub := Record{Fields: f}
	l := &eraLookSpec{}
	if v, ok, err := readBool(sub, "colors_on"); err != nil {
		return nil, badRequestf("era_look: %s", planError(err))
	} else if ok {
		l.colorsOn = &v
	}
	if sub.Has("feel") {
		l.feel = strings.ToLower(sub.Str("feel"))
		if _, _, preset := calendar.EraFeelNumbers(l.feel); !preset && l.feel != calendar.EraFeelCustom {
			return nil, badRequestf("era_look: feel must be still, subtle, lively or custom")
		}
	}
	for _, p := range []struct {
		key string
		dst **float64
	}{{"intensity", &l.intensity}, {"speed", &l.speed}} {
		v, ok, err := sub.Float(p.key)
		if err != nil {
			return nil, badRequestf("era_look: %s", planError(err))
		}
		if ok {
			*p.dst = &v
		}
	}
	return l, nil
}

// look is the era look after the block, over the calendar's own. A named
// feel stores its numbers, so the screen shows the same feel back.
func (l *eraLookSpec) look(cur calendar.EraLook) (calendar.EraLook, error) {
	if cur.Feel == "" {
		cur = calendar.DefaultEraLook()
	}
	next := cur
	if l.colorsOn != nil {
		next.ColorsOn = *l.colorsOn
	}
	if l.feel != "" {
		next.Feel = l.feel
		if i, sp, preset := calendar.EraFeelNumbers(l.feel); preset {
			next.Intensity, next.Speed = i, sp
		}
	}
	if l.intensity != nil {
		next.Intensity = *l.intensity
	}
	if l.speed != nil {
		next.Speed = *l.speed
	}
	if l.intensity != nil || l.speed != nil {
		// Numbers a preset stands for read as that preset, anything else as
		// the owner's own fine-tuning, as the settings name them.
		next.Feel = calendar.EraFeelCustom
		for _, f := range []string{calendar.EraFeelStill, calendar.EraFeelSubtle, calendar.EraFeelLively} {
			if i, sp, _ := calendar.EraFeelNumbers(f); i == next.Intensity && sp == next.Speed {
				next.Feel = f
			}
		}
	}
	if err := calendar.ValidateEraLook(next); err != nil {
		return next, badRequestf("era_look: %s", planError(err))
	}
	return next, nil
}

func yesNo(v bool, yes, no string) string {
	if v {
		return yes
	}
	return no
}

// changes is the review wording for each option the block really changes,
// and the refusal for one the calendar can't take. tiers are the campaign's
// current levels; nil when they could not be read.
func (o calOptions) changes(cal *calendar.Calendar, tiers []campaigns.TierDefinition) ([]string, error) {
	var did []string
	if o.tiers != nil && !sameTiers(o.tiers, tiers) {
		did = append(did, "replaces the event importance levels ("+plural(len(o.tiers), "level", "levels")+")")
	}
	if h, ok := o.hemisphere.Get(); ok {
		if cal.Hemisphere == nil || *cal.Hemisphere != h {
			did = append(did, "sets the hemisphere to "+h)
		}
	} else if o.hemisphere.IsNull() && cal.Hemisphere != nil {
		did = append(did, "clears the hemisphere")
	}
	if o.forecasts != nil && *o.forecasts != cal.ForecastsEnabled {
		did = append(did, yesNo(*o.forecasts, "turns weather forecasts on for players", "turns weather forecasts off"))
	}
	if o.newWeek != nil {
		if cal.UsesRealTime() {
			return nil, badRequestf("this calendar follows the real-world date, so its weeks can't be made to start with each month")
		}
		if *o.newWeek != cal.MonthStartsNewWeek {
			did = append(did, yesNo(*o.newWeek, "makes every month start a new week", "stops months starting a new week"))
		}
	}
	if o.eraLook != nil {
		next, err := o.eraLook.look(cal.EraLook)
		if err != nil {
			return nil, err
		}
		cur := cal.EraLook
		if cur.Feel == "" {
			cur = calendar.DefaultEraLook()
		}
		if next != cur {
			did = append(did, "changes how eras show behind the month")
		}
	}
	return did, nil
}

// needsCalendarUpdate is true when the block writes a calendar-row option
// (everything but the campaign's tiers and the era look, which have their own
// saves).
func (o calOptions) needsCalendarUpdate() bool {
	return o.hemisphere.Present() || o.forecasts != nil || o.newWeek != nil
}

// toUpdate carries the written options onto the calendar's partial update.
func (o calOptions) toUpdate(in *calendar.UpdateCalendarInput) {
	in.Hemisphere = o.hemisphere
	if o.forecasts != nil {
		in.ForecastsEnabled = patch.Of(*o.forecasts)
	}
	if o.newWeek != nil {
		in.MonthStartsNewWeek = patch.Of(*o.newWeek)
	}
}

// currentTiers reads the campaign's levels for a plan to compare against; nil
// when they are not wired or can't be read.
func (k CalendarKind) currentTiers(ctx context.Context, campaignID string) []campaigns.TierDefinition {
	if k.Tiers == nil {
		return nil
	}
	defs, err := k.Tiers.GetEventTierDefinitions(ctx, campaignID)
	if err != nil {
		return nil
	}
	return defs
}

// checkTiers refuses `tiers` where it can't be written: without the
// campaign service, or from anyone but the owner (the calendar kind is owner
// only already; this is the second lock the campaign's own route has too).
func (k CalendarKind) checkTiers(a Actor, o calOptions) error {
	if o.tiers == nil {
		return nil
	}
	if k.Tiers == nil {
		return badRequestf("tiers can't be set here; leave it out")
	}
	if a.Role < permissions.RoleOwner {
		return apperror.NewForbidden("only the campaign owner can change the event importance levels")
	}
	return nil
}

// applyOptions writes the era look and the importance levels; the calendar
// row's own options ride the block's UpdateCalendar call.
func (k CalendarKind) applyOptions(ctx context.Context, campaignID string, s *calSpec, cal *calendar.Calendar) error {
	if s.opts.eraLook != nil {
		look, err := s.opts.eraLook.look(cal.EraLook)
		if err != nil {
			return err
		}
		w, ok := k.Svc.(EraLookWriter)
		if !ok {
			return badRequestf("era_look can't be set here; leave it out")
		}
		if err := w.SaveEraLook(ctx, cal.ID, campaignID, look, nil); err != nil {
			return err
		}
	}
	if s.opts.tiers != nil {
		if err := k.Tiers.SetEventTierDefinitions(ctx, campaignID, s.opts.tiers); err != nil {
			return err
		}
	}
	return nil
}

// tiersBlock is the levels in the shape `tiers:` reads them, so a block sent
// back unchanged plans as no change. indent is each line's leading space.
func tiersBlock(defs []campaigns.TierDefinition) string {
	var b strings.Builder
	b.WriteString("tiers:\n")
	for _, d := range defs {
		fmt.Fprintf(&b, "  - {slug: %s, name: %s, color: %s, prominence: %d", quote(d.Slug), quote(d.Name), quote(d.Color), d.Prominence)
		if d.IsDefault {
			b.WriteString(", default: true")
		}
		b.WriteString("}\n")
	}
	return b.String()
}

// optionLines are the calendar's simple settings as the keys that set them.
func optionLines(cal *calendar.Calendar) string {
	var b strings.Builder
	if cal.Hemisphere != nil && *cal.Hemisphere != "" {
		b.WriteString("hemisphere: " + *cal.Hemisphere + "\n")
	}
	fmt.Fprintf(&b, "forecasts: %t\n", cal.ForecastsEnabled)
	if !cal.UsesRealTime() {
		fmt.Fprintf(&b, "month_starts_new_week: %t\n", cal.MonthStartsNewWeek)
	}
	l := cal.EraLook
	if l.Feel == "" {
		l = calendar.DefaultEraLook()
	}
	fmt.Fprintf(&b, "era_look: {colors_on: %t, feel: %s, intensity: %s, speed: %s}\n", l.ColorsOn, l.Feel, num(l.Intensity), num(l.Speed))
	return b.String()
}
