// builder_presets_test.go — the gallery and the importer are ONE PATH, proved
// rather than asserted.
//
// Every assertion here loads a preset the way an upload is loaded — bytes into
// DetectAndParse — so a malformed payload fails exactly where a user's
// malformed file fails. There is no preset-specific reader to test, and that
// absence IS the ruling (§2.2: no preset table, no preset migration, no new
// parser, no second apply path).
package calendar

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestBuilderPresets_TheRosterIsFive pins [WZ-7a]: the mockup's five ship,
// because they are what the signed stills show and the gate is fidelity.
func TestBuilderPresets_TheRosterIsFive(t *testing.T) {
	if len(builderPresets) != 5 {
		t.Fatalf("the gallery ships five cards, got %d", len(builderPresets))
	}
	want := []string{"harptos", "gregorian", "elven", "dwarven", "blank"}
	for i, k := range want {
		if builderPresets[i].Key != k {
			t.Errorf("card %d is %q, want %q — the order is the signed one",
				i, builderPresets[i].Key, k)
		}
	}
	if _, ok := builderPresetFor("nonesuch"); ok {
		t.Error("an unknown preset key must be REJECTED, never defaulted — a wizard " +
			"that silently substitutes a different calendar is worse than an error")
	}
}

// TestBuilderPresets_LoadThroughTheShippedParser is the wave's proof that the
// gallery IS the importer. Every payload is detected and parsed by the same
// four-format sniffer an upload meets.
func TestBuilderPresets_LoadThroughTheShippedParser(t *testing.T) {
	for _, p := range builderPresets {
		t.Run(p.Key, func(t *testing.T) {
			if p.File == "" {
				// The real-life card has no payload BY RULING ([WZ-5]): a
				// real-life calendar is a MODE, not a structure.
				if p.Mode != ModeRealLife {
					t.Fatalf("only the real-life card may ship without a payload")
				}
				return
			}
			data, err := builderPresetFS.ReadFile(p.File)
			if err != nil {
				t.Fatalf("the payload must be embedded: %v", err)
			}
			res, err := DetectAndParse(data)
			if err != nil {
				t.Fatalf("the SHIPPED parser must read this preset: %v", err)
			}
			if res.Format == FormatUnknown {
				t.Fatal("format detection failed — a preset the importer cannot read " +
					"is a preset with a second code path")
			}
			if len(res.Months) == 0 || len(res.Weekdays) == 0 {
				t.Fatalf("%s parsed to %d months / %d weekdays",
					p.Key, len(res.Months), len(res.Weekdays))
			}
		})
	}
}

// TestBuilderPresets_ExerciseMoreThanOneFormat pins [WZ-7b]. One format would
// leave "the gallery and the importer are one path" resting on a single parser;
// two makes it an observation.
func TestBuilderPresets_ExerciseMoreThanOneFormat(t *testing.T) {
	seen := map[ImportFormat]string{}
	for _, p := range builderPresets {
		if p.File == "" {
			continue
		}
		data, err := builderPresetFS.ReadFile(p.File)
		if err != nil {
			t.Fatalf("read %s: %v", p.File, err)
		}
		res, err := DetectAndParse(data)
		if err != nil {
			t.Fatalf("parse %s: %v", p.File, err)
		}
		seen[res.Format] = p.Key
	}
	if len(seen) < 2 {
		t.Fatalf("the roster exercises %d format(s): %v — [WZ-7b] wants at least two",
			len(seen), seen)
	}
	if _, ok := seen[FormatChronicle]; !ok {
		t.Error("Harptos ships Chronicle-native by ruling")
	}
	if _, ok := seen[FormatCalendaria]; !ok {
		t.Error("one preset ships in Calendaria's shape, parsed by parseCalendaria " +
			"with no preset-specific code anywhere")
	}
	t.Logf("formats exercised by the roster: %v", seen)
}

// TestBuilderPresets_DraftsAreCreatable walks each preset all the way to the
// payload Create would hand ApplyImport, and runs the SHIPPED validators over
// it. A preset that previews and then fails at Create is the divergence §7.3
// calls a bug you own.
func TestBuilderPresets_DraftsAreCreatable(t *testing.T) {
	svc, ctx := builderStubService(), context.Background()

	for _, p := range builderPresets {
		t.Run(p.Key, func(t *testing.T) {
			d, err := builderPresetDraft(p.Key)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if d.Mode != p.Mode {
				t.Errorf("mode = %q, want %q", d.Mode, p.Mode)
			}
			if err := validateBuilderDraft(d); err != nil {
				t.Fatalf("a shipped preset must validate: %v", err)
			}
			res := builderImportResult(d)
			if err := svc.SetMonths(ctx, "c", res.Months); err != nil {
				t.Errorf("months: %v", err)
			}
			if err := svc.SetWeekdays(ctx, "c", res.Weekdays); err != nil {
				t.Errorf("weekdays: %v", err)
			}
			if len(res.Moons) > 0 {
				if err := svc.SetMoons(ctx, "c", res.Moons); err != nil {
					t.Errorf("moons: %v", err)
				}
			}
			if len(res.Eras) > 0 {
				if err := svc.SetEras(ctx, "c", res.Eras); err != nil {
					t.Errorf("eras: %v", err)
				}
			}

			// And it must draw. The preview is the shipped Block, so "it
			// validates" and "it renders" are two different claims.
			got := builderPreviewBlock(d, 0)
			if got.Month.WeekLen != len(d.Weekdays) {
				t.Errorf("the grid reads the declared week: got %d, want %d",
					got.Month.WeekLen, len(d.Weekdays))
			}
			if got.Fault != "" {
				t.Errorf("a shipped preset resolves its date; got fault %q", got.Fault)
			}
		})
	}
}

// TestBuilderPresets_BlankIsEraLessOnPurpose. Blank's card says so at Start,
// the wizard holds Create, and empty is its IDENTITY rather than an oversight.
// Every other preset creates as advertised.
func TestBuilderPresets_BlankIsEraLessOnPurpose(t *testing.T) {
	for _, p := range builderPresets {
		d, err := builderPresetDraft(p.Key)
		if err != nil {
			t.Fatalf("%s: %v", p.Key, err)
		}
		blocked := builderCreateBlocked(d)
		if p.Key == "blank" {
			if !p.EraGap {
				t.Error("Blank's card must declare its era gap at Start")
			}
			if blocked == "" {
				t.Error("Blank must hold Create until an era is added")
			}
			continue
		}
		if p.EraGap {
			t.Errorf("%s declares an era gap it does not have", p.Key)
		}
		if blocked != "" {
			t.Errorf("%s must create as advertised; held by %q", p.Key, blocked)
		}
	}
}

// TestBuilderPresets_HarptosIsTheSignedShape.
func TestBuilderPresets_HarptosIsTheSignedShape(t *testing.T) {
	d, err := builderPresetDraft("harptos")
	if err != nil {
		t.Fatal(err)
	}
	if got := builderMonthDays(d); got != 360 {
		t.Errorf("twelve 30-day months = 360; got %d", got)
	}
	if got := builderIntercalaryDays(d); got != 5 {
		t.Errorf("five festivals between them; got %d", got)
	}
	if len(d.Weekdays) != 10 {
		t.Errorf("ten-day weeks; got %d", len(d.Weekdays))
	}
	if got := builderWeekSplit(len(d.Weekdays)); got != 5 {
		t.Errorf("a ten-wide grid splits 5+5; got a mark at %d", got)
	}
	if len(d.Moons) != 4 {
		t.Errorf("four moons — three drawn, one almanac-only; got %d", len(d.Moons))
	}
	if d.LeapEvery != 4 {
		t.Errorf("a leap day every fourth year; got %d", d.LeapEvery)
	}

	// [WZ-4] SIGNED: eraBands is STRUCK. block_geometry.go states a mid-month
	// era boundary "is not expressible on main" and Era{StartYear, EndYear} is
	// YEARS, not dates — so the payload declares a year-granular era and the
	// Eras station describes what Create actually writes. The mockup's second
	// era existed to demonstrate the mid-month band; a year-granular AE
	// beginning in 1523 would contradict the signed "1523 RoW" year line, so
	// the era that survives is the one the year line names.
	if len(d.Eras) != 1 || d.Eras[0].Code != "RoW" {
		t.Errorf("Harptos ships one year-granular era, RoW; got %d: %+v", len(d.Eras), d.Eras)
	}
	if d.EpochName != "RoW" {
		t.Errorf("the year line reads its epoch; got %q", d.EpochName)
	}
}

// TestBuilderPresets_TheRealLifeCardIsAModeNotAStructure pins [WZ-5]. It is a
// scope hole and not a detail: the shipped chooser's FIRST card is "Sync to
// Real Life" and the shipped Bench's first pill is `Real-life`, so a front door
// that demoted it would not be a front door — and the Gregorian preset creating
// a fantasy calendar would be a silent product defect.
func TestBuilderPresets_TheRealLifeCardIsAModeNotAStructure(t *testing.T) {
	p, ok := builderPresetFor("gregorian")
	if !ok {
		t.Fatal("the real-life card is missing from the roster")
	}
	if p.File != "" {
		t.Error("the real-life card ships NO payload — a real-life calendar is a mode, " +
			"and an embedded JSON would create a fantasy calendar wearing Gregorian's clothes")
	}
	d, err := builderPresetDraft("gregorian")
	if err != nil {
		t.Fatal(err)
	}
	if d.Mode != ModeRealLife {
		t.Fatalf("mode = %q, want %q", d.Mode, ModeRealLife)
	}
	if len(d.Months) != 12 || len(d.Weekdays) != 7 {
		t.Errorf("the preview draws what Create will seed: %d months, %d weekdays",
			len(d.Months), len(d.Weekdays))
	}
	if d.Months[1].LeapDays != 1 {
		t.Error("February is the only month whose length moves")
	}
	if d.LeapNote == "" {
		t.Error("the ×100/×400 clause has no home in the single-modulus model and must " +
			"be SHOWN so nothing silently disappears")
	}

	// A LITERAL 7 IS LEGAL AS PRESET DATA AND ILLEGAL AS LAYOUT ([WZ-11a]).
	// Gregorian genuinely has seven weekday names; the grid derives from the
	// declared list and knows no seven.
	if got := builderPreviewBlock(d, 0).Month.WeekLen; got != 7 {
		t.Errorf("the grid reads the declared week: got %d", got)
	}
	if builderWeekSplit(7) != 0 {
		t.Error("a seven-day week gets no split — nothing in the grid knows the number seven")
	}
}

// TestBuilderPresets_ColourNamesAreAReadingNotASource. The mono name beside a
// swatch is the SECOND CHANNEL and it is not optional — REVIEW failed an earlier
// revision partly for colour with no second channel. But a colour nobody named
// must read as itself rather than as a fabricated word.
func TestBuilderPresets_ColourNamesAreAReadingNotASource(t *testing.T) {
	if got := builderColourName("oklch(0.65 0.05 240)"); got != "frost" {
		t.Errorf("a known colour reads by name; got %q", got)
	}
	if got := builderColourName("#90ee90"); got != "#90ee90" {
		t.Errorf("an unnamed colour reads as ITSELF, never as an invented word; got %q", got)
	}
	if got := builderColourName("  "); got != "" {
		t.Errorf("no colour declared is not a colour; got %q", got)
	}
}

// TestBuilderPresets_EraCodeRidesDescriptionAndNeverReachesTheYearLine.
//
// Chronicle's Era has NO code column. Every parser that reads an abbreviation
// puts it in Description — parseCalendaria does exactly that — so the wizard
// reads the same place rather than deriving an initialism, and an era whose
// format carried none simply has no code.
//
// The year suffix itself comes from Calendar.EpochName and never from an Era
// (blockDateLabel), which is why a Calendaria import seeds the epoch from the
// first era's code: without it the year line drops to a bare number and the
// reading the file plainly intended is lost.
func TestBuilderPresets_EraCodeRidesDescriptionAndNeverReachesTheYearLine(t *testing.T) {
	for key, want := range map[string]string{
		"harptos": "RoW", "dwarven": "Deep-year", "elven": "Cycle", "gregorian": "AD",
	} {
		d, err := builderPresetDraft(key)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if len(d.Eras) == 0 {
			t.Fatalf("%s declares no era", key)
		}
		if got := d.Eras[0].Code; got != want {
			t.Errorf("%s: era code = %q, want %q", key, got, want)
		}
		if d.EpochName != want {
			t.Errorf("%s: the year line reads %q, want %q", key, d.EpochName, want)
		}
		// And the reading is the one the shipped renderer produces — for an
		// IN-WORLD calendar. A real-life one takes blockDateLabel's Gregorian
		// form ("Thu 1 Jan 2026") and carries no reckoning suffix at all, which
		// is correct: nobody writes "1 Jan 2026 AD" on a session calendar. The
		// epoch is still stored and still what the settings page shows.
		label := builderPreviewBlock(d, 0).DateLabel
		if d.Mode == ModeRealLife {
			if strings.Contains(label, want) {
				t.Errorf("%s: a real-world date line must not carry a reckoning; got %q",
					key, label)
			}
			continue
		}
		if !strings.Contains(label, want) {
			t.Errorf("%s: the Block's date line %q does not carry the reckoning %q",
				key, label, want)
		}
	}

	// Blank declares no era, so it has no reckoning and claims none.
	blank, err := builderPresetDraft("blank")
	if err != nil {
		t.Fatal(err)
	}
	if blank.EpochName != "" {
		t.Errorf("Blank's identity is UNCHOSEN; got epoch %q", blank.EpochName)
	}

	// The code is display only and never reaches what Create writes.
	d, _ := builderPresetDraft("harptos")
	blob, err := json.Marshal(builderImportResult(d))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), `"code"`) {
		t.Error("the era code leaked into the create payload — it is a display reading")
	}
}

// TestBuilderPresets_TheCalendariaNestingWeShip records a FINDING about the
// shipped importer, and the reason this preset is authored the way it is.
//
// parseCalendaria's unmarshalValuedMap (import.go:583) tries a DIRECT map
// before the `{values: …}` wrapper — but json.Unmarshal of `{"values":{…}}`
// into map[string]T SUCCEEDS, producing one zero-valued entry under the key
// "values", so the direct branch always wins and the wrapper branch is
// UNREACHABLE. A Calendaria file using the wrapper therefore parses to a single
// nameless zero-day month. That is the shape Cordinator's three reference
// calendars use and the shape real Calendaria exports use.
//
// import.go's parsers are explicitly NOT W-H's files, so this is REPORTED AND
// NOT FIXED, and the Elven preset is authored in the direct nesting — which is
// a nesting the parser's own comment names and handles correctly. This test
// pins both halves so the finding cannot be lost, and so that whoever fixes the
// parser sees this go green from the other direction.
func TestBuilderPresets_TheCalendariaNestingWeShip(t *testing.T) {
	direct := []byte(`{"name":"D","years":{"yearZero":10},` +
		`"months":{"a0":{"name":"Aevel","days":45,"ordinal":1}},` +
		`"days":{"values":{"d0":{"name":"Sel","ordinal":1}},"hoursPerDay":24}}`)
	res, err := DetectAndParse(direct)
	if err != nil {
		t.Fatalf("the direct nesting must parse: %v", err)
	}
	if len(res.Months) != 1 || res.Months[0].Name != "Aevel" || res.Months[0].Days != 45 {
		t.Fatalf("direct nesting parsed to %+v", res.Months)
	}

	wrapped := []byte(`{"name":"W","years":{"yearZero":10},` +
		`"months":{"values":{"a0":{"name":"Aevel","days":45,"ordinal":1}}},` +
		`"days":{"values":{"d0":{"name":"Sel","ordinal":1}},"hoursPerDay":24}}`)
	res, err = DetectAndParse(wrapped)
	if err != nil {
		t.Fatalf("the wrapped nesting parses without error (that is the problem): %v", err)
	}
	if len(res.Months) == 1 && res.Months[0].Name == "Aevel" {
		t.Log("the {values:…} wrapper now parses correctly — the importer defect W-H " +
			"reported has been fixed, and the Elven preset may be re-authored with the " +
			"wrapper if a reason to prefer it appears")
		return
	}
	t.Logf("FINDING, carried not fixed: the {values:…} wrapper parses to %d month(s) "+
		"%+v — unmarshalValuedMap's direct branch shadows the wrapper branch "+
		"(import.go:583). Reported upward; import.go's parsers are not W-H's files.",
		len(res.Months), res.Months)
}
