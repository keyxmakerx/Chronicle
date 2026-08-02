// builder_presets.go — THE PRESET GALLERY, WHICH IS THE IMPORTER WITH FIVE
// EMBEDDED PAYLOADS (C-CALV4-WIZARD-P13 §2.2, [WZ-7] SIGNED).
//
// ── THE RULING THAT COLLAPSES TWO FEATURES INTO ONE ────────────────────────
//
// Presets ship as //go:embed-ed JSON and are applied through the EXACT SAME
// DetectAndParse → ApplyImport path the importer uses.
//
//	NO preset table. NO preset migration. NO new parser. NO second apply path.
//
// The preset gallery and the importer become one code path with two front
// doors, which is precisely what L6 ("wraps, does not replace") and the design
// notes' "structure import → the builder, as step one" are asking for — and it
// makes every preset ROUND-TRIP-PROVABLE against export.go's BuildExport,
// because a preset IS an export.
//
// The proof is structural rather than asserted: there is no code path in this
// file that reads a preset's months without going through the shipped parser.
// If a preset payload were malformed, it would fail exactly where a user's
// malformed upload fails, with exactly the same message.
//
// ── THE ROSTER, AND WHY IT IS FIVE ─────────────────────────────────────────
//
// [WZ-7a]: ship the mockup's five, because they are what the signed stills show
// and the gate is fidelity. [WZ-7b]: the set must exercise MORE THAN ONE of the
// four formats, because that is itself the proof the gallery and the importer
// are one path — so Harptos, Dwarven and Blank ship as Chronicle-native and
// Elven ships in CALENDARIA's shape, parsed by parseCalendaria with no
// preset-specific code anywhere. [WZ-7c]: Cordinator's three real Calendaria
// reference calendars (athasian-tyr, calendar-of-therin, forbidden-lands) are a
// post-launch gallery expansion and NOT W-H scope — they parse today with no new
// code, so the expansion is a JSON drop.
//
// GREGORIAN HAS NO PAYLOAD, AND THAT IS [WZ-5] SIGNED. Mode is fantasy |
// reallife and reallife is not cosmetic: CreateCalendar overrides year/hours/
// leap from the wall clock and names the epoch "AD", seedDefaults seeds twelve
// Gregorian months and seven weekdays, and TracksRealTime + an IANA
// RealTimeZone ride on it. A "Real world" preset built from an embedded JSON
// would create a FANTASY calendar wearing Gregorian's clothes — the wrong mode,
// no timezone, no wall-clock authority — and would therefore not be the Bench's
// real-world Block, which the master plan requires the Bench to host. So the
// real-life card is a MODE FLAG on the same shell that short-circuits stations
// 1–7, exactly as the importer door already does, and Create routes it through
// CreateCalendar's own reallife path rather than through an import.
//
// ── THE IDENTITY TRIPLE ([WZ-6] SIGNED) ────────────────────────────────────
//
// Hue, pattern and letter are metadata on the roster below and are NOT
// persisted: Chronicle has no columns for them and no migration ships in this
// wave. They are what the WIZARD'S OWN CHROME prints — a preview of the
// identity the author chose — and Review carries the same `needs backend` chip
// calendar-settings.html already carries, because the created calendar's real
// hue is assigned by blockCalHue from a UUID that does not exist until Create.
// The Block inside the preview column derives its own and is left alone: it is
// the real renderer and the wizard does not override it.
//
// There is no --cal-blank token and inventing one is forbidden. Blank falls
// back to a NEUTRAL RULE token and never to --own-none: crossing the OWNER axis
// into calendar identity is the same category error, and Blank's whole point is
// that identity is UNCHOSEN, which a neutral rule expresses and a borrowed
// owner grey does not ([WZ-7d]).
package calendar

import (
	"embed"
	"fmt"
	"strings"
)

// builderPresetFS holds the payloads. They are ordinary calendar exports and
// ordinary Calendaria files — nothing about them is preset-shaped.
//
//go:embed presets/*.json
var builderPresetFS embed.FS

// builderPreset is one card in the Start gallery.
type builderPreset struct {
	Key  string
	Name string
	Desc string

	// File is the embedded payload, or "" for the real-life card, which has
	// none by ruling.
	File string
	// Mode is what Create writes. Only the real-life card is reallife.
	Mode string

	// The identity triple — DERIVED-AND-CHIPPED display metadata, never a
	// column. Hue is one of the four CLOSED calendar tokens or "" for Blank,
	// whose swatch is hollow because its identity is unchosen.
	Hue     string
	Pattern string
	Letter  string
	Hollow  bool

	// EraGap marks a preset that ships with no era on purpose. Blank is
	// era-less BY IDENTITY — its card says so at Start, and Review holds Create
	// until one is added. Empty is Blank's identity, not an oversight.
	EraGap bool

	// LeapNote is a real-world exception clause Chronicle's single-modulus leap
	// model has no home for. It is SHOWN so nothing silently disappears, it is
	// not editable, and Review says what Create will actually write.
	LeapNote string

	// LeapName is the leap day's own name, and it is roster data because NO
	// IMPORT FORMAT CARRIES ONE. Chronicle stores a leap day as Month.
	// LeapYearDays — extra days on a named month — so Harptos's Shieldmeet
	// arrives from presets/harptos.json as `Midsummer: leap_year_days 1` and
	// nothing else. The wizard's flagship sentence reads "add 1 day named
	// Shieldmeet after Midsummer" in the signed still, and it printed an empty
	// box instead. The name is display-only, it is chipped as such on Review,
	// and Create writes the DAY and drops the NAME.
	LeapName string
}

// builderPresets is the signed roster, in the signed order.
var builderPresets = []builderPreset{
	{
		Key: "harptos", Name: "Harptos of Imix", File: "presets/harptos.json", Mode: ModeFantasy,
		Desc: "Twelve 30-day months, five festivals between them, ten-day weeks.",
		Hue:  "harptos", Pattern: "p1", Letter: "H",
		LeapName: "Shieldmeet",
	},
	{
		// NO FILE, BY RULING. See the header: a real-life calendar is a mode,
		// not a structure, and seedDefaults already seeds the Gregorian shape.
		Key: "gregorian", Name: "Real world / Gregorian", File: "", Mode: ModeRealLife,
		Desc: "The twelve months you already know, seven-day weeks, a leap day every fourth year — synced to real-world dates and your timezone.",
		Hue:  "real", Pattern: "p2", Letter: "R",
		LeapNote: "skip years ×100 unless ×400",
	},
	{
		Key: "elven", Name: "Elven Reckoning", File: "presets/elven.json", Mode: ModeFantasy,
		Desc: "Eight 45-day months in nine-day weeks; years are counted in Cycles.",
		Hue:  "elven", Pattern: "p3", Letter: "E",
	},
	{
		Key: "dwarven", Name: "Dwarven Deep-count", File: "presets/dwarven.json", Mode: ModeFantasy,
		Desc: "Ten 36-day months in six-day work-weeks. No sky is declared underground.",
		Hue:  "dwarven", Pattern: "p4", Letter: "D",
	},
	{
		Key: "blank", Name: "Blank calendar", File: "presets/blank.json", Mode: ModeFantasy,
		Desc: "One month, a ten-day week, nothing else — every station starts empty.",
		Hue:  "", Pattern: "p8", Letter: "B", Hollow: true, EraGap: true,
	},
}

// builderPresetFor resolves a preset key. Unknown keys are REJECTED, not
// defaulted: a wizard that silently substitutes a different calendar for the
// one a click asked for is worse than an error page.
func builderPresetFor(key string) (builderPreset, bool) {
	for _, p := range builderPresets {
		if p.Key == key {
			return p, true
		}
	}
	return builderPreset{}, false
}

// builderPresetDraft loads a preset THROUGH THE SHIPPED PARSER.
//
// This function is the whole argument of §2.2 in eight lines: read the bytes,
// hand them to DetectAndParse exactly as an upload is handed to it, and turn
// the resulting *ImportResult into draft state. Nothing here knows which format
// it just read, and nothing here can read a preset the importer could not.
func builderPresetDraft(key string) (*builderDraft, error) {
	p, ok := builderPresetFor(key)
	if !ok {
		return nil, fmt.Errorf("unknown preset %q", key)
	}
	if p.File == "" {
		return builderRealLifeDraft(p), nil
	}
	data, err := builderPresetFS.ReadFile(p.File)
	if err != nil {
		return nil, fmt.Errorf("read preset %q: %w", key, err)
	}
	res, err := DetectAndParse(data)
	if err != nil {
		return nil, fmt.Errorf("parse preset %q: %w", key, err)
	}
	d := builderDraftFromImport(res)
	d.Preset, d.Mode = p.Key, p.Mode
	d.Hue, d.Pattern, d.Letter, d.HollowSwatch = p.Hue, p.Pattern, p.Letter, p.Hollow
	d.LeapNote = p.LeapNote
	// The leap day's NAME is roster data: no import format carries one, so a
	// preset that has a named leap day says so here or the sentence prints a
	// blank. It is display-only either way — see builderPreset.LeapName.
	if d.LeapEvery > 0 && d.LeapAdd > 0 {
		d.LeapName = p.LeapName
	}
	if d.Name == "" {
		d.Name = p.Name
	}
	return d, nil
}

// builderRealLifeDraft is the real-life card's draft.
//
// IT DECLARES NO STRUCTURE ON PURPOSE. Stations 1–7 are short-circuited because
// the wall clock owns the structure: CreateCalendar seeds twelve Gregorian
// months and seven weekdays and names the epoch AD, and a wizard that let the
// author bend those first would be offering an edit the create path discards.
// What the author DOES choose is the name and the timezone, and Review states
// the wall-clock facts in plain language.
//
// The month list below is therefore a PREVIEW of what Create will seed, not an
// authored declaration — it exists so the preview column has a month to draw
// and so Review's counts are true. The era is Chronicle's own "AD" epoch, which
// is what lets the real-life card clear the wizard's era gate honestly rather
// than by exemption.
func builderRealLifeDraft(p builderPreset) *builderDraft {
	d := &builderDraft{
		Preset: p.Key, Mode: ModeRealLife, Name: "Real world",
		EpochName: "AD", Year: builderRealLifeYear,
		LeapEvery: 4, LeapAdd: 1, LeapAfter: "February", LeapNote: p.LeapNote,
		Hue: p.Hue, Pattern: p.Pattern, Letter: p.Letter,
	}
	for i, m := range builderGregorianMonths {
		leap := 0
		if i == 1 {
			leap = 1 // February, and the ONLY month whose length moves
		}
		d.Months = append(d.Months, builderMonth{Name: m.name, Days: m.days, LeapDays: leap})
	}
	d.Weekdays = append(d.Weekdays,
		"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday")
	d.Moons = []builderMoon{{Name: "Luna", Period: 29.53, NewAt: 6}}
	d.Seasons = []builderSeason{
		{Name: "Winter", ColorName: "frost", Color: "oklch(0.65 0.05 240)", StartMonth: 12, StartDay: 1, EndMonth: 2, EndDay: 28},
		{Name: "Spring", ColorName: "sedge", Color: "oklch(0.65 0.10 140)", StartMonth: 3, StartDay: 1, EndMonth: 5, EndDay: 31},
		{Name: "Summer", ColorName: "brass", Color: "oklch(0.72 0.12 85)", StartMonth: 6, StartDay: 1, EndMonth: 8, EndDay: 31},
		{Name: "Autumn", ColorName: "russet", Color: "oklch(0.58 0.12 40)", StartMonth: 9, StartDay: 1, EndMonth: 11, EndDay: 30},
	}
	d.Eras = []builderEra{{
		Name: "Common Era", Code: "AD", ColorName: "slate",
		Color: "oklch(0.55 0.03 260)", StartYear: 1,
	}}
	return d
}

// builderRealLifeYear is the preview's year. CreateCalendar overrides it from
// the wall clock, so this number is only ever a drawing, and Review says so.
const builderRealLifeYear = 2026

var builderGregorianMonths = []struct {
	name string
	days int
}{
	{"January", 31}, {"February", 28}, {"March", 31}, {"April", 30},
	{"May", 31}, {"June", 30}, {"July", 31}, {"August", 31},
	{"September", 30}, {"October", 31}, {"November", 30}, {"December", 31},
}

// builderDraftFromImport turns ANY *ImportResult — a preset's or an upload's —
// into draft state. One function, both doors.
//
// A COLOUR IS NEVER INVENTED HERE. A season or era whose format carried no
// colour arrives with an empty one and the station renders a hollow swatch with
// an em dash beside it, because "no colour was declared" and "grey" are
// different facts and the mono colour NAME is the second channel.
func builderDraftFromImport(res *ImportResult) *builderDraft {
	d := &builderDraft{
		Mode: ModeFantasy,
		Name: res.CalendarName,
		Year: res.Settings.CurrentYear,
	}
	if res.Settings.EpochName != nil {
		d.EpochName = strings.TrimSpace(*res.Settings.EpochName)
	}
	if res.Settings.LeapYearEvery > 0 {
		d.LeapEvery = res.Settings.LeapYearEvery
		d.LeapAdd = 1
	}
	for _, m := range res.Months {
		d.Months = append(d.Months, builderMonth{
			Name: m.Name, Days: m.Days, Intercalary: m.IsIntercalary, LeapDays: m.LeapYearDays,
		})
		// The leap day's home is the month that carries it, which is the only
		// place Chronicle records one.
		if m.LeapYearDays > 0 && d.LeapAfter == "" {
			d.LeapAfter = m.Name
		}
	}
	for _, w := range res.Weekdays {
		d.Weekdays = append(d.Weekdays, w.Name)
	}
	for _, m := range res.Moons {
		d.Moons = append(d.Moons, builderMoon{Name: m.Name, Period: m.CycleDays, NewAt: m.PhaseOffset})
	}
	for _, s := range res.Seasons {
		d.Seasons = append(d.Seasons, builderSeason{
			Name: s.Name, Color: s.Color, ColorName: builderColourName(s.Color),
			StartMonth: s.StartMonth, StartDay: s.StartDay,
			EndMonth: s.EndMonth, EndDay: s.EndDay,
		})
	}
	for _, e := range res.Eras {
		// THE CODE IS NOT DERIVED, AND IT IS NOT A NEW COLUMN. Chronicle's Era
		// has no code field, but every parser that reads one puts it in
		// Description — parseCalendaria does exactly that with an era's
		// `abbreviation` (import.go:845-849) — so the wizard reads the same
		// place. An era whose format carried no abbreviation simply has no code,
		// and the station prints an em dash rather than inventing an initialism.
		code := ""
		if e.Description != nil {
			code = strings.TrimSpace(*e.Description)
		}
		d.Eras = append(d.Eras, builderEra{
			Name: e.Name, Code: code, Color: e.Color,
			ColorName: builderColourName(e.Color), StartYear: e.StartYear,
		})
	}

	// THE YEAR SUFFIX COMES FROM EpochName AND NEVER FROM AN ERA
	// (blockDateLabel). A format that carried an era abbreviation but no epoch
	// — Calendaria is exactly that format — would otherwise lose the reading it
	// plainly intended, and the year line would drop to a bare number. So the
	// first era's code seeds the epoch when the import carried none. It is an
	// inference and it is stated: the Eras station prints the year the calendar
	// will actually read, so the author sees the result rather than the rule.
	if d.EpochName == "" && len(d.Eras) > 0 {
		d.EpochName = d.Eras[0].Code
	}
	return d
}

// builderColourNames is the mono second channel: a small closed vocabulary of
// human names for the colours the presets and the four import formats actually
// carry. It is a READING of a colour, never a source of one.
//
// A colour with no name reads as its own value in mono rather than as a
// fabricated word — "#90ee90" is honest; calling it "sedge" when nobody said so
// is not.
var builderColourNames = map[string]string{
	"oklch(0.65 0.05 240)": "frost",
	"oklch(0.65 0.10 140)": "sedge",
	"oklch(0.72 0.12 85)":  "brass",
	"oklch(0.58 0.12 40)":  "russet",
	"oklch(0.55 0.08 290)": "dusk",
	"oklch(0.62 0.13 45)":  "ember",
	"oklch(0.60 0.03 250)": "slate",
	"oklch(0.55 0.03 260)": "slate",
	"oklch(0.55 0.09 210)": "ward",
	"oklch(0.60 0.09 150)": "moss",
	"oklch(0.58 0.06 70)":  "bronze",
}

// builderColourName reads a colour. An unknown one reads as itself.
func builderColourName(colour string) string {
	colour = strings.TrimSpace(colour)
	if colour == "" {
		return ""
	}
	if n, ok := builderColourNames[colour]; ok {
		return n
	}
	return colour
}
