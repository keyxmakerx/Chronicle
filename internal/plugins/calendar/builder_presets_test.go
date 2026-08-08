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
			if _, err := svc.SetMonths(ctx, "c", res.Months); err != nil {
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

// TestBuilderPreset_TheLeapDayIsNamedAndTheNameIsChipped pins the fix for a
// blank inside the wizard's flagship "a sentence, not a form" station.
//
// The signed still reads "Every 4 years, add 1 day named Shieldmeet after
// Midsummer" on one line. The shipped build printed "named [   ] after
// [Midsummer]" — an empty box — because presets/harptos.json cannot carry a
// leap-day name: Chronicle models a leap day as Month.LeapYearDays, extra days
// on a named month, and NO import format has a field for the day's own name.
//
// So the name is roster data, and because it is roster data that Create cannot
// write, Review carries the SIGNED needs-backend chip for it. Both halves are
// asserted here: the sentence reads, and the honesty rides with it.
func TestBuilderPreset_TheLeapDayIsNamedAndTheNameIsChipped(t *testing.T) {
	d, err := builderPresetDraft("harptos")
	if err != nil {
		t.Fatal(err)
	}
	if d.LeapName != "Shieldmeet" {
		t.Errorf("the signed still names the leap day Shieldmeet; got %q", d.LeapName)
	}
	if d.LeapAfter != "Midsummer" {
		t.Errorf("the leap day rides Midsummer's LeapYearDays; got %q", d.LeapAfter)
	}

	// The month the name is derived FROM still carries the day, because that
	// is the only part of this Create actually writes.
	found := false
	for _, m := range d.Months {
		if m.Name == "Midsummer" && m.LeapDays == 1 {
			found = true
		}
	}
	if !found {
		t.Error("Midsummer must carry leap_year_days 1 — the name is display, the DAY is data")
	}

	var need string
	for _, c := range builderChecksFor(d) {
		if c.Kind == "need" && strings.Contains(c.Text, "Shieldmeet") {
			need = c.Text
		}
	}
	if need == "" {
		t.Fatal("a name Create drops must carry the needs-backend chip on Review")
	}
	for _, want := range []string{"no column", "drops the name"} {
		if !strings.Contains(need, want) {
			t.Errorf("the Review line must say what Create writes and what it drops; "+
				"missing %q in %q", want, need)
		}
	}

	// A preset with no leap rule declares no name and raises no chip — the
	// chip is a genuine gap, never decoration.
	blank, err := builderPresetDraft("blank")
	if err != nil {
		t.Fatal(err)
	}
	if blank.LeapName != "" {
		t.Errorf("Blank declares no leap day and therefore no name; got %q", blank.LeapName)
	}
	for _, c := range builderChecksFor(blank) {
		if c.Kind == "need" && strings.Contains(c.Text, "leap day's name") {
			t.Error("no leap rule, no leap-name chip")
		}
	}
}

// TestBuilderPresets_RoundTripThroughBuildExport closes the acceptance item
// that had been neither met nor booked: *"each preset round-trips against
// `export.go:BuildExport`"* (dispatch §2.2 / Acceptance).
//
// The previous rounds proved the FIRST half only — every preset loads through
// the shipped four-format sniffer (TestBuilderPresets_LoadThroughTheShippedParser).
// Nothing proved that the calendar the wizard then promises can be exported and
// read back as the same calendar, which is what makes a preset a real payload
// rather than a fixture that happens to parse.
//
// THE FIVE HOPS, ALL SHIPPED CODE. No hop restates a mapping this test could
// then agree with itself about:
//
//	0. the embedded preset bytes → DetectAndParse            (THE AUTHORED PAYLOAD,
//	                                                          the anchor everything
//	                                                          below is measured from)
//	1. the embedded preset bytes → DetectAndParse            (the importer's door)
//	2. → builderDraftFromImport → builderImportResult        (what Create APPLIES)
//	   and → draftCalendar                                   (the calendar shape
//	                                                          the wizard promises)
//	3. → BuildExport → encoding/json                         (the export door)
//	4. → DetectAndParse again                                (the importer's door,
//	                                                          a second time)
//
// Hop 4 must also DETECT the export as Chronicle's own format without being
// told, because an export nobody can re-import is not a round trip.
//
// ── WHY HOP 0 EXISTS, AND WHY THE TEST WAS WEAKER WITHOUT IT ────────────────
//
// The first edition of this test compared hops 2 and 3 ONLY. Both sides are
// derived from the SAME *builderDraft, so every payload change propagates
// identically to both and CANNOT make them disagree: adversarial verification
// mutated eleven distinct fields of presets/harptos.json — leap_year_every,
// hours_per_day, a month's days, current_year, epoch_name, a moon's name,
// cycle_days, phase_offset and colour, an era's colour, start_year and
// description — and the test stayed green on every one. A round trip measured
// only against itself is a tautology with assertions in it.
//
// Hop 0 is the fix: the assertions below run against the AUTHORED BYTES, so a
// field the pipeline drops, hardcodes or infers is visible. Three were, and two
// of them the self-referential comparison hid completely.
//
// ── AMENDMENT R4-S24-A: THE THREE ASYMMETRIES ARE CLOSED ────────────────────
//
// This test used to DESCRIBE three losses and assert that they still happened,
// exactly, so that a future change would have to say which one it moved. That
// was the right shape while they were booked. C-SWEEP-R4 stage 24 fixed all
// three, so every one of those exemptions is inverted into the stronger claim:
// the authored value SURVIVES the round trip. The old form tolerated a lossy
// front door as long as the loss was stable; this one forbids the loss.
//
// (1) A MOON'S AUTHORED COLOUR SURVIVES. presets/harptos.json and
// presets/elven.json AUTHOR moon colours (#cfd6dd, #d8cbb8, #c7cdd4, #b9bcc4 /
// #d5d0e8, #cfd8e0). builderMoon now carries Color, builderDraftFromImport
// copies it, it rides hidden through the form like a season's, and both
// builderImportResult and draftCalendar write it — so Create and the preview
// agree, and both agree with the payload. builderImportMoonSwatch survives for
// the one case it was ever right for: a moon the WIZARD created, which has no
// payload behind it. The "the payload must still author a colour" guard is KEPT
// — without an authored colour these assertions prove nothing.
//
// (2) AN AUTHORED ERA CODE SURVIVES. builderDraftFromImport reads
// Era.Description into builderEra.Code (parseCalendaria's `abbreviation`) and
// the Eras station displays it; builderImportResult now writes it back to
// Description, which is the same place and the only place Chronicle has. The
// old test could only assert the drop's SYMPTOM, because in all three embedded
// payloads the code happens to equal the epoch name (RoW / Deep-year / Cycle)
// and the epoch does round-trip. That coincidence is now irrelevant: the code
// is asserted directly against the payload, so a file whose code and epoch
// differ is covered by construction rather than by luck.
//
// (3) HOURS/MINUTES/SECONDS AND THE LEAP OFFSET ARE READ. builderImportResult
// used to hardcode 24/60/60 and never write LeapYearOffset. All four are now
// carried on the draft and emitted by builderCarryFields, so they survive the
// form round trip too. Every embedded preset happens to carry the Gregorian
// values, which is why the equality assertions below looked green before —
// TestBuilderDoorIsNotLossierThanTheImporter uses a payload that does NOT, and
// is the test that can actually see this one.
//
// All three remain asserted EXACTLY, never as "they agree somehow".
//
// ── MATCHING IS BY NAME, NOT BY INDEX, AND THAT IS A FINDING TOO ────────────
//
// Moons, seasons and eras are compared against hop 0 BY NAME. parseCalendaria
// reads them out of JSON OBJECTS (Go map iteration is randomised) and only
// partly re-sorts: moons are never sorted at all, and seasons sort on DayStart,
// which presets/elven.json ties three ways at 0. Two independent parses of the
// same bytes can therefore yield different orders, so an index comparison
// between hop 0 and hop 2 would be flaky rather than wrong-detecting. Hops 2→3
// keep their index comparison, because those two share one draft and one order.
// The parser non-determinism is booked, not fixed here — import.go is outside
// this slice.
func TestBuilderPresets_RoundTripThroughBuildExport(t *testing.T) {
	for _, p := range builderPresets {
		t.Run(p.Key, func(t *testing.T) {
			d, err := builderPresetDraft(p.Key)
			if err != nil {
				t.Fatalf("load preset: %v", err)
			}

			// WHAT CREATE APPLIES — the same *ImportResult the handler hands to
			// ApplyImport, from the same function.
			applied := builderImportResult(d)

			// THE CALENDAR THE WIZARD PROMISES, exported and read back.
			blob, err := json.Marshal(BuildExport(draftCalendar(d), nil, false))
			if err != nil {
				t.Fatalf("marshal export: %v", err)
			}
			back, err := DetectAndParse(blob)
			if err != nil {
				t.Fatalf("the export could not be re-imported by the shipped sniffer: %v", err)
			}
			if back.Format != FormatChronicle {
				t.Errorf("the export re-detects as %q — Chronicle's own export must be "+
					"recognised by Chronicle's own importer", back.Format)
			}

			// ── settings ──────────────────────────────────────────────────
			if back.CalendarName != applied.CalendarName {
				t.Errorf("name: export %q, create %q", back.CalendarName, applied.CalendarName)
			}
			gotEpoch, wantEpoch := "", ""
			if back.Settings.EpochName != nil {
				gotEpoch = *back.Settings.EpochName
			}
			if applied.Settings.EpochName != nil {
				wantEpoch = *applied.Settings.EpochName
			}
			if gotEpoch != wantEpoch {
				t.Errorf("epoch: export %q, create %q", gotEpoch, wantEpoch)
			}
			for _, f := range []struct {
				name      string
				got, want int
			}{
				{"current_year", back.Settings.CurrentYear, applied.Settings.CurrentYear},
				{"hours_per_day", back.Settings.HoursPerDay, applied.Settings.HoursPerDay},
				{"minutes_per_hour", back.Settings.MinutesPerHour, applied.Settings.MinutesPerHour},
				{"seconds_per_minute", back.Settings.SecondsPerMinute, applied.Settings.SecondsPerMinute},
				{"leap_year_every", back.Settings.LeapYearEvery, applied.Settings.LeapYearEvery},
				{"leap_year_offset", back.Settings.LeapYearOffset, applied.Settings.LeapYearOffset},
			} {
				if f.got != f.want {
					t.Errorf("%s: export %d, create %d", f.name, f.got, f.want)
				}
			}

			// ── months, weekdays, seasons, eras: structural equality ──────
			if len(back.Months) != len(applied.Months) {
				t.Fatalf("months: export %d, create %d", len(back.Months), len(applied.Months))
			}
			for i := range applied.Months {
				if back.Months[i] != applied.Months[i] {
					t.Errorf("month %d: export %+v, create %+v", i, back.Months[i], applied.Months[i])
				}
			}
			if len(back.Weekdays) != len(applied.Weekdays) {
				t.Fatalf("weekdays: export %d, create %d", len(back.Weekdays), len(applied.Weekdays))
			}
			for i := range applied.Weekdays {
				if back.Weekdays[i] != applied.Weekdays[i] {
					t.Errorf("weekday %d: export %+v, create %+v", i, back.Weekdays[i], applied.Weekdays[i])
				}
			}
			if len(back.Seasons) != len(applied.Seasons) {
				t.Fatalf("seasons: export %d, create %d", len(back.Seasons), len(applied.Seasons))
			}
			for i, w := range applied.Seasons {
				g := back.Seasons[i]
				if g.Name != w.Name || g.Color != w.Color || g.StartMonth != w.StartMonth ||
					g.StartDay != w.StartDay || g.EndMonth != w.EndMonth || g.EndDay != w.EndDay {
					t.Errorf("season %d: export %+v, create %+v", i, g, w)
				}
			}
			if len(back.Eras) != len(applied.Eras) {
				t.Fatalf("eras: export %d, create %d", len(back.Eras), len(applied.Eras))
			}
			for i, w := range applied.Eras {
				g := back.Eras[i]
				if g.Name != w.Name || g.StartYear != w.StartYear || g.Color != w.Color ||
					g.SortOrder != w.SortOrder {
					t.Errorf("era %d: export %+v, create %+v", i, g, w)
				}
				// THE ERA CODE ROUND-TRIPS (amendment R4-S24-A). It rides
				// Description — parseCalendaria's `abbreviation`, and the only
				// place Chronicle's Era has for it — so the export and Create
				// must carry the same string, or a nil on both sides for an era
				// that never had one. This equality only says the two derived
				// sides agree; the hop-0 block measures it against the AUTHORED
				// payload, which is where a drop would be visible.
				if optStr(g.Description, "\x00nil") != optStr(w.Description, "\x00nil") {
					t.Errorf("era %d description: export %v, create %v — an era's code must "+
						"survive both paths identically", i, optStr(g.Description, "<nil>"),
						optStr(w.Description, "<nil>"))
				}
			}

			// ── moons: EVERYTHING, INCLUDING THE COLOUR (amendment R4-S24-A)
			if len(back.Moons) != len(applied.Moons) {
				t.Fatalf("moons: export %d, create %d", len(back.Moons), len(applied.Moons))
			}
			for i, w := range applied.Moons {
				g := back.Moons[i]
				if g != w {
					t.Errorf("moon %d: export %+v, create %+v — name, cycle, phase AND colour "+
						"must round-trip exactly; the colour used to be the exemption and is "+
						"not one any more", i, g, w)
				}
			}

			// ── HOP 0: THE AUTHORED PAYLOAD ───────────────────────────────
			//
			// Everything above compares two derivations of one draft. This
			// compares the round trip to the BYTES, which is the only anchor a
			// payload mutation can move.
			if p.File == "" {
				// The real-life card has no payload BY RULING (see
				// builderPreset.File): its structure comes from the wall clock
				// and seedDefaults, so there is nothing authored to anchor to.
				return
			}
			data, err := builderPresetFS.ReadFile(p.File)
			if err != nil {
				t.Fatalf("read preset bytes: %v", err)
			}
			orig, err := DetectAndParse(data)
			if err != nil {
				t.Fatalf("parse preset bytes: %v", err)
			}

			// ── settings, against the payload ─────────────────────────────
			if back.CalendarName != orig.CalendarName {
				t.Errorf("name: payload %q, round trip %q", orig.CalendarName, back.CalendarName)
			}
			for _, f := range []struct {
				name      string
				got, want int
			}{
				{"current_year", back.Settings.CurrentYear, orig.Settings.CurrentYear},
				{"leap_year_every", back.Settings.LeapYearEvery, orig.Settings.LeapYearEvery},
				// ASYMMETRY 3, asserted as equality on purpose. The wizard has
				// no station for any of these four and builderImportResult
				// hardcodes 24/60/60 and drops the offset. Every embedded
				// payload happens to carry those values, so equality holds —
				// and a payload that says otherwise reds here rather than being
				// silently overwritten at Create.
				{"hours_per_day", back.Settings.HoursPerDay, orig.Settings.HoursPerDay},
				{"minutes_per_hour", back.Settings.MinutesPerHour, orig.Settings.MinutesPerHour},
				{"seconds_per_minute", back.Settings.SecondsPerMinute, orig.Settings.SecondsPerMinute},
				{"leap_year_offset", back.Settings.LeapYearOffset, orig.Settings.LeapYearOffset},
			} {
				if f.got != f.want {
					t.Errorf("%s: payload %d, round trip %d — the wizard reads no such "+
						"station, so a payload that declares one loses it at Create",
						f.name, f.want, f.got)
				}
			}

			// THE EPOCH, AND THE ONE DOCUMENTED INFERENCE. A payload with no
			// epoch but an era code gets the epoch seeded from that code
			// (builderDraftFromImport, stated there) — Calendaria is exactly
			// that shape, and elven.json exercises it. Anything else must match
			// the payload exactly.
			origEpoch, backEpoch := "", ""
			if orig.Settings.EpochName != nil {
				origEpoch = strings.TrimSpace(*orig.Settings.EpochName)
			}
			if back.Settings.EpochName != nil {
				backEpoch = strings.TrimSpace(*back.Settings.EpochName)
			}
			wantEpochFromPayload := origEpoch
			if origEpoch == "" && len(orig.Eras) > 0 {
				wantEpochFromPayload = eraCodeOf(orig.Eras[0])
			}
			if backEpoch != wantEpochFromPayload {
				t.Errorf("epoch: payload %q (era-code inference → %q), round trip %q",
					origEpoch, wantEpochFromPayload, backEpoch)
			}

			// ── months and weekdays, against the payload, BY INDEX ─────────
			// Both are re-sorted deterministically by every parser (months and
			// weekdays sort on ordinal), so index is the honest comparison.
			if len(back.Months) != len(orig.Months) {
				t.Fatalf("months: payload %d, round trip %d", len(orig.Months), len(back.Months))
			}
			for i := range orig.Months {
				if back.Months[i] != orig.Months[i] {
					t.Errorf("month %d: payload %+v, round trip %+v", i, orig.Months[i], back.Months[i])
				}
			}
			if len(back.Weekdays) != len(orig.Weekdays) {
				t.Fatalf("weekdays: payload %d, round trip %d", len(orig.Weekdays), len(back.Weekdays))
			}
			for i := range orig.Weekdays {
				if back.Weekdays[i] != orig.Weekdays[i] {
					t.Errorf("weekday %d: payload %+v, round trip %+v", i, orig.Weekdays[i], back.Weekdays[i])
				}
			}

			// ── seasons, BY NAME ──────────────────────────────────────────
			backSeasons := map[string]Season{}
			for _, s := range back.Seasons {
				backSeasons[s.Name] = s
			}
			if len(back.Seasons) != len(orig.Seasons) {
				t.Errorf("seasons: payload %d, round trip %d", len(orig.Seasons), len(back.Seasons))
			}
			for _, w := range orig.Seasons {
				g, ok := backSeasons[w.Name]
				if !ok {
					t.Errorf("season %q is in the payload and not in the round trip", w.Name)
					continue
				}
				if g.Color != w.Color || g.StartMonth != w.StartMonth || g.StartDay != w.StartDay ||
					g.EndMonth != w.EndMonth || g.EndDay != w.EndDay {
					t.Errorf("season %q: payload %+v, round trip %+v", w.Name, w, g)
				}
			}

			// ── moons, BY NAME — and ASYMMETRY 1 ──────────────────────────
			backMoons, appliedMoons := map[string]MoonInput{}, map[string]MoonInput{}
			for _, m := range back.Moons {
				backMoons[m.Name] = m
			}
			for _, m := range applied.Moons {
				appliedMoons[m.Name] = m
			}
			if len(back.Moons) != len(orig.Moons) {
				t.Errorf("moons: payload %d, round trip %d", len(orig.Moons), len(back.Moons))
			}
			for _, w := range orig.Moons {
				g, ok := backMoons[w.Name]
				if !ok {
					t.Errorf("moon %q is in the payload and not in the round trip", w.Name)
					continue
				}
				if g.CycleDays != w.CycleDays || g.PhaseOffset != w.PhaseOffset {
					t.Errorf("moon %q: payload cycle %v offset %v, round trip cycle %v offset %v",
						w.Name, w.CycleDays, w.PhaseOffset, g.CycleDays, g.PhaseOffset)
				}
				// AMENDMENT R4-S24-A. The payload AUTHORS a colour, and it must
				// now reach BOTH ends: the calendar Create applies and the
				// calendar the wizard exports. The "the payload still authors
				// one" leg is kept exactly as it was — it is what makes the two
				// assertions below mean anything, and it must red if a preset
				// ever stops authoring colours.
				if w.Color == "" {
					t.Errorf("moon %q: the payload no longer authors a colour, so the "+
						"preservation asserted below is no longer demonstrated by this "+
						"preset — do not delete these assertions, give the preset a colour",
						w.Name)
				}
				if w.Color == builderImportMoonSwatch {
					t.Errorf("moon %q: the payload now authors the fallback swatch %q "+
						"itself, so preservation and replacement look identical here — "+
						"pick a different authored colour", w.Name, w.Color)
				}
				if a := appliedMoons[w.Name]; a.Color != w.Color {
					t.Errorf("moon %q colour at Create: the payload authors %q and the "+
						"wizard's door must carry it; got %q. The plain importer keeps it, "+
						"so a difference here means the front door is lossier than the "+
						"importer it wraps", w.Name, w.Color, a.Color)
				}
				if g.Color != w.Color {
					t.Errorf("moon %q colour on export: the payload authors %q; the wizard's "+
						"preview/export calendar wrote %q", w.Name, w.Color, g.Color)
				}
			}

			// ── eras, BY NAME — and ASYMMETRY 2 ───────────────────────────
			backEras := map[string]EraInput{}
			for _, e := range back.Eras {
				backEras[e.Name] = e
			}
			if len(back.Eras) != len(orig.Eras) {
				t.Errorf("eras: payload %d, round trip %d", len(orig.Eras), len(back.Eras))
			}
			for _, w := range orig.Eras {
				g, ok := backEras[w.Name]
				if !ok {
					t.Errorf("era %q is in the payload and not in the round trip", w.Name)
					continue
				}
				if g.StartYear != w.StartYear || g.Color != w.Color {
					t.Errorf("era %q: payload start %d colour %q, round trip start %d colour %q",
						w.Name, w.StartYear, w.Color, g.StartYear, g.Color)
				}
				// AMENDMENT R4-S24-A. The code SURVIVES, and it is now asserted
				// against the payload directly rather than through the
				// coincidence that used to hide the drop (in all three embedded
				// payloads the era code happens to equal the epoch name, and
				// the epoch does round-trip, so a code-shaped string came out
				// the far end whether or not the code itself was carried).
				code := eraCodeOf(w)
				if code == "" {
					t.Errorf("era %q: the payload no longer carries a code, so the "+
						"preservation asserted below is no longer demonstrated by this "+
						"preset", w.Name)
				}
				if got := eraCodeOf(g); got != code {
					t.Errorf("era %q: the payload's code is %q; the round trip carries %q. "+
						"An era's code rides Description in every direction — the plain "+
						"importer keeps it, so the wizard must too", w.Name, code, got)
				}
			}
		})
	}
}

// optStr renders an optional string for comparison/printing, substituting a
// sentinel for nil so "absent on both sides" compares equal. (The package
// already has a derefOr for ints — worldstate_service.go — hence the name.)
func optStr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

// eraCodeOf reads an era's code out of the ONE place every parser puts it.
// Chronicle's Era has no code column; parseCalendaria writes an era's
// `abbreviation` into Description (import.go), and builderDraftFromImport reads
// the same place. See asymmetry 2 in TestBuilderPresets_RoundTripThroughBuildExport.
func eraCodeOf(e EraInput) string {
	if e.Description == nil {
		return ""
	}
	return strings.TrimSpace(*e.Description)
}
