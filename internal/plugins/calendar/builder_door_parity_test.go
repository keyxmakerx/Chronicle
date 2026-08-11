// builder_door_parity_test.go — C-SWEEP-R4 stage 24,
// backlog/wizard-importer-door-lossier-than-importer.
//
// The wizard is a FRONT DOOR onto the importer: "one code path, two front
// doors" (dispatch §2.2 / [WZ-12]). A door that discards fields before the
// shared path starts breaks that claim in the one place a door can quietly
// differ, and it did — six of them.
//
// The existing round-trip pin
// (TestBuilderPresets_RoundTripThroughBuildExport) could see only two of the
// six, because every embedded preset carries the Gregorian time units and a
// zero leap offset, so "hardcode 24/60/60" and "read the payload" produce the
// same numbers. The fixture below deliberately declares none of them.
package calendar

import "testing"

// doorParityPayload is a Calendaria file that declares a value for EVERY field
// the wizard's door used to drop, and a DIFFERENT value from Chronicle's
// default in each case, so a hardcoded default cannot pass by coincidence:
//
//	hoursPerDay 20      (not 24)
//	minutesPerHour 50   (not 60)
//	secondsPerMinute 40 (not 60)
//	leapStart 3         (not 0 — "every 7 years" and "every 7 years offset 3"
//	                     are different calendars)
//	moon colour #22aa55 (not builderImportMoonSwatch, and not empty)
//	era abbreviation TA (DIFFERENT from the epoch, which is what the preset
//	                     payloads could not show: in all three of those the era
//	                     code equals the epoch name, so a dropped code still
//	                     came out the far end looking right)
const doorParityPayload = `{
  "name": "Odd Units",
  "years": { "yearZero": 900, "leapYear": { "leapInterval": 7, "leapStart": 3 } },
  "leapYearConfig": { "rule": "custom" },
  "days": {
    "hoursPerDay": 20, "minutesPerHour": 50, "secondsPerMinute": 40,
    "values": { "d1": { "name": "Firstday", "ordinal": 1 },
                "d2": { "name": "Secondday", "ordinal": 2 } }
  },
  "months": { "m1": { "name": "Solis", "days": 30, "ordinal": 1 },
              "m2": { "name": "Lunis", "days": 30, "ordinal": 2 } },
  "moons":  { "n1": { "name": "Verdant", "cycleLength": 28, "color": "#22aa55" } },
  "eras":   { "e1": { "name": "Third Age", "abbreviation": "TA", "startYear": 1 } }
}`

// TestBuilderDoorIsNotLossierThanTheImporter is the regression proper.
//
// It runs ONE payload through both doors and compares them field by field:
//
//	plain importer : bytes → DetectAndParse                    → ImportResult
//	wizard door    : bytes → DetectAndParse → builderDraftFromImport
//	                                       → builderImportResult → ImportResult
//
// Both ends are the SAME type and both are what ApplyImport receives, so the
// comparison needs no restated mapping to be wrong about. Anything the wizard
// leg loses is a field the operator authored, saw the wizard display, and then
// did not get.
func TestBuilderDoorIsNotLossierThanTheImporter(t *testing.T) {
	direct, err := DetectAndParse([]byte(doorParityPayload))
	if err != nil {
		t.Fatalf("plain importer: %v", err)
	}
	viaWizard := builderImportResult(builderDraftFromImport(direct))

	// ── the four settings ─────────────────────────────────────────────────
	//
	// Measured before the fix, on this exact payload:
	//   direct  hours=20 min=50 sec=40 leapEvery=7 leapOffset=3
	//   wizard  hours=24 min=60 sec=60 leapEvery=7 leapOffset=0
	for _, f := range []struct {
		name      string
		got, want int
	}{
		{"hours_per_day", viaWizard.Settings.HoursPerDay, direct.Settings.HoursPerDay},
		{"minutes_per_hour", viaWizard.Settings.MinutesPerHour, direct.Settings.MinutesPerHour},
		{"seconds_per_minute", viaWizard.Settings.SecondsPerMinute, direct.Settings.SecondsPerMinute},
		{"leap_year_offset", viaWizard.Settings.LeapYearOffset, direct.Settings.LeapYearOffset},
		// Not part of the defect, asserted so a regression cannot hide behind
		// the four above.
		{"leap_year_every", viaWizard.Settings.LeapYearEvery, direct.Settings.LeapYearEvery},
		{"current_year", viaWizard.Settings.CurrentYear, direct.Settings.CurrentYear},
	} {
		if f.got != f.want {
			t.Errorf("%s: the plain importer keeps %d, the wizard's door produced %d",
				f.name, f.want, f.got)
		}
	}

	// ── the moon's colour ─────────────────────────────────────────────────
	if len(viaWizard.Moons) != len(direct.Moons) {
		t.Fatalf("moons: importer %d, wizard %d", len(direct.Moons), len(viaWizard.Moons))
	}
	for i, w := range direct.Moons {
		if w.Color == "" || w.Color == builderImportMoonSwatch {
			t.Fatalf("moon %q must author a colour that is neither empty nor the fallback "+
				"swatch, or this assertion proves nothing", w.Name)
		}
		if got := viaWizard.Moons[i]; got != w {
			t.Errorf("moon %d: the plain importer keeps %+v, the wizard's door produced %+v",
				i, w, got)
		}
	}

	// ── the era's code ────────────────────────────────────────────────────
	//
	// The code rides Description — parseCalendaria writes Calendaria's
	// `abbreviation` there and it is the only place Chronicle's Era has for it.
	// This payload's code ("TA") deliberately differs from its epoch, which is
	// what the preset payloads could not demonstrate.
	if len(viaWizard.Eras) != len(direct.Eras) {
		t.Fatalf("eras: importer %d, wizard %d", len(direct.Eras), len(viaWizard.Eras))
	}
	for i, w := range direct.Eras {
		wantCode := optStr(w.Description, "")
		if wantCode == "" {
			t.Fatal("the fixture era must carry an abbreviation, or this assertion proves nothing")
		}
		if gotCode := optStr(viaWizard.Eras[i].Description, ""); gotCode != wantCode {
			t.Errorf("era %d code: the plain importer keeps %q, the wizard's door produced %q",
				i, wantCode, gotCode)
		}
		if viaWizard.Eras[i].Name != w.Name || viaWizard.Eras[i].StartYear != w.StartYear {
			t.Errorf("era %d: importer %+v, wizard %+v", i, w, viaWizard.Eras[i])
		}
	}

	// The epoch is SEEDED from the first era's code when the payload carries no
	// epoch of its own — a documented inference, not a loss. Asserted here so
	// the era-code assertion above cannot be satisfied by the inference alone.
	if got := optStr(viaWizard.Settings.EpochName, ""); got != "TA" {
		t.Errorf("epoch: expected the documented era-code seed %q, got %q", "TA", got)
	}

	// ── the OTHER door out of the same draft ──────────────────────────────
	//
	// C-SWEEP-R4 stage 27. Stage 24 applied the same four-field fix TWICE —
	// once in builderImportResult (the commit path, asserted above) and once in
	// draftCalendar (the preview path) — and pinned only the first. Reverting
	// draftCalendar's four fields to their old hardcoded 24/60/60/0 left the
	// whole calendar suite green.
	//
	// draftCalendar is not a lesser path: builderPreviewBlock draws the wizard's
	// live month grid from it and builderMoonAlmanac derives the moon shelf from
	// it, both keyed on the leap configuration. Left unpinned, a regression
	// there shows the operator a preview whose leap years and moon phases
	// disagree with the calendar they are about to create — the same front-
	// door/back-door divergence stage 24 exists to close, just pointed the other
	// way.
	preview := draftCalendar(builderDraftFromImport(direct))
	assertPreviewMatchesImporter(t, preview, direct)
}

// assertPreviewMatchesImporter compares the wizard's PREVIEW projection against
// the plain importer, on the fields stage 24 fixed.
//
// It also asserts the fixture is capable of failing: each of the four settings
// must differ from the value draftCalendar used to hardcode, or the comparison
// would pass on a fully reverted draftCalendar and prove nothing. That check is
// the test's own load-bearing part — it is what makes the reviewer's mutation
// (revert all four to 24/60/60/0) red instead of green.
func assertPreviewMatchesImporter(t *testing.T, preview *Calendar, direct *ImportResult) {
	t.Helper()

	for _, f := range []struct {
		name              string
		got, want, oldHar int
	}{
		{"hours_per_day", preview.HoursPerDay, direct.Settings.HoursPerDay, 24},
		{"minutes_per_hour", preview.MinutesPerHour, direct.Settings.MinutesPerHour, 60},
		{"seconds_per_minute", preview.SecondsPerMinute, direct.Settings.SecondsPerMinute, 60},
		{"leap_year_offset", preview.LeapYearOffset, direct.Settings.LeapYearOffset, 0},
	} {
		if f.want == f.oldHar {
			t.Fatalf("%s: the fixture authors %d, which is the value draftCalendar used to "+
				"hardcode — this assertion would pass on the reverted code and proves nothing",
				f.name, f.want)
		}
		if f.got != f.want {
			t.Errorf("%s: the plain importer keeps %d, the wizard's PREVIEW showed %d",
				f.name, f.want, f.got)
		}
	}

	// Not part of the four, asserted so a regression cannot hide behind them.
	if preview.LeapYearEvery != direct.Settings.LeapYearEvery {
		t.Errorf("leap_year_every: importer %d, preview %d",
			direct.Settings.LeapYearEvery, preview.LeapYearEvery)
	}
	if preview.CurrentYear != direct.Settings.CurrentYear {
		t.Errorf("current_year: importer %d, preview %d",
			direct.Settings.CurrentYear, preview.CurrentYear)
	}

	// The moon colour, on the preview side. draftCalendar stamps the fallback
	// swatch only over an EMPTY colour; the fixture authors one, so the swatch
	// must not appear.
	if len(preview.Moons) != len(direct.Moons) {
		t.Fatalf("moons: importer %d, preview %d", len(direct.Moons), len(preview.Moons))
	}
	for i, w := range direct.Moons {
		if got := preview.Moons[i].Color; got != w.Color {
			t.Errorf("moon %d colour: the plain importer keeps %q, the wizard's PREVIEW showed %q",
				i, w.Color, got)
		}
		if preview.Moons[i].CycleDays != w.CycleDays {
			t.Errorf("moon %d cycle: importer %v, preview %v",
				i, w.CycleDays, preview.Moons[i].CycleDays)
		}
	}

	// The era code, on the preview side.
	if len(preview.Eras) != len(direct.Eras) {
		t.Fatalf("eras: importer %d, preview %d", len(direct.Eras), len(preview.Eras))
	}
	for i, w := range direct.Eras {
		wantCode := optStr(w.Description, "")
		if wantCode == "" {
			t.Fatal("the fixture era must carry an abbreviation, or this assertion proves nothing")
		}
		if got := optStr(preview.Eras[i].Description, ""); got != wantCode {
			t.Errorf("era %d code: the plain importer keeps %q, the wizard's PREVIEW showed %q",
				i, wantCode, got)
		}
	}
}

// TestBuilderPreviewAndCreateAgree closes the loop the other way.
//
// The assertion above ties the preview to the IMPORTER. This one ties it to
// what Create will actually write, from the SAME draft — which is the promise
// the wizard makes to the operator ("the result looks the same", [WZ-15]) and
// the only comparison that stays meaningful for a draft the operator typed by
// hand, where there is no importer leg to compare against at all.
//
// It runs over every embedded preset as well as the odd-units payload, so a
// door that diverges only on Gregorian-shaped input is still caught.
func TestBuilderPreviewAndCreateAgree(t *testing.T) {
	drafts := map[string]*builderDraft{}

	direct, err := DetectAndParse([]byte(doorParityPayload))
	if err != nil {
		t.Fatalf("plain importer: %v", err)
	}
	drafts["odd-units"] = builderDraftFromImport(direct)

	for _, p := range builderPresets {
		d, perr := builderPresetDraft(p.Key)
		if perr != nil {
			t.Fatalf("preset %q: %v", p.Key, perr)
		}
		drafts[p.Key] = d
	}

	for name, d := range drafts {
		t.Run(name, func(t *testing.T) {
			// The preview WITHOUT the synthesized real Moon — the one body the
			// preview shows and the create payload deliberately does not store.
			// builderPreviewMinusRealMoon asserts it is exactly that before
			// removing it, so the exemption cannot widen into a free pass.
			preview := builderPreviewMinusRealMoon(t, d)
			created := builderImportResult(d)

			if preview.HoursPerDay != created.Settings.HoursPerDay ||
				preview.MinutesPerHour != created.Settings.MinutesPerHour ||
				preview.SecondsPerMinute != created.Settings.SecondsPerMinute ||
				preview.LeapYearEvery != created.Settings.LeapYearEvery ||
				preview.LeapYearOffset != created.Settings.LeapYearOffset ||
				preview.CurrentYear != created.Settings.CurrentYear {
				t.Errorf("the preview shows a different calendar from the one Create writes:\n"+
					"  preview hours=%d min=%d sec=%d every=%d offset=%d year=%d\n"+
					"  create  hours=%d min=%d sec=%d every=%d offset=%d year=%d",
					preview.HoursPerDay, preview.MinutesPerHour, preview.SecondsPerMinute,
					preview.LeapYearEvery, preview.LeapYearOffset, preview.CurrentYear,
					created.Settings.HoursPerDay, created.Settings.MinutesPerHour,
					created.Settings.SecondsPerMinute, created.Settings.LeapYearEvery,
					created.Settings.LeapYearOffset, created.Settings.CurrentYear)
			}
			if optStr(preview.EpochName, "") != optStr(created.Settings.EpochName, "") {
				t.Errorf("epoch: preview %q, create %q",
					optStr(preview.EpochName, ""), optStr(created.Settings.EpochName, ""))
			}

			if len(preview.Moons) != len(created.Moons) {
				t.Fatalf("moons: preview %d, create %d", len(preview.Moons), len(created.Moons))
			}
			for i := range created.Moons {
				if preview.Moons[i].Color != created.Moons[i].Color ||
					preview.Moons[i].CycleDays != created.Moons[i].CycleDays ||
					preview.Moons[i].PhaseOffset != created.Moons[i].PhaseOffset {
					t.Errorf("moon %d: preview %+v, create %+v",
						i, preview.Moons[i], created.Moons[i])
				}
			}

			if len(preview.Eras) != len(created.Eras) {
				t.Fatalf("eras: preview %d, create %d", len(preview.Eras), len(created.Eras))
			}
			for i := range created.Eras {
				if optStr(preview.Eras[i].Description, "") != optStr(created.Eras[i].Description, "") {
					t.Errorf("era %d code: preview %q, create %q", i,
						optStr(preview.Eras[i].Description, ""),
						optStr(created.Eras[i].Description, ""))
				}
			}
		})
	}
}

// TestBuilderDoorSurvivesTheFormRoundTrip is the other half, and it is the half
// that would have caught a fix that stopped at builderImportResult.
//
// The wizard has NO server-side draft: every preview rebuilds the whole
// declaration from the posted body (builderReadForm), so a field the draft
// gains but builderCarryFields does not emit is a field that silently resets on
// the author's next keystroke. It would have looked fixed in the unit test
// above and still been lost in the product.
//
// This runs the draft through the SHIPPED pairing — builderCarryFields emits
// hidden inputs, builderReadForm parses them back — at every station, because a
// carry is conditional on which station is open.
func TestBuilderDoorSurvivesTheFormRoundTrip(t *testing.T) {
	direct, err := DetectAndParse([]byte(doorParityPayload))
	if err != nil {
		t.Fatalf("plain importer: %v", err)
	}
	original := builderDraftFromImport(direct)

	for step := range builderStations {
		for _, importer := range []bool{false, true} {
			rebuilt, _, _, _, _, rerr := builderReadForm(
				builderFormCtx(t, builderFormFor(original, step, importer)))
			if rerr != nil {
				t.Fatalf("step %d importer=%v: builderReadForm: %v", step, importer, rerr)
			}

			got := builderImportResult(rebuilt)
			want := builderImportResult(original)

			// Compared field by field rather than as a struct: ImportedSettings
			// holds *string EpochName, and == on the struct compares the
			// POINTER, so two identical settings always differ.
			gs, ws := got.Settings, want.Settings
			if gs.HoursPerDay != ws.HoursPerDay || gs.MinutesPerHour != ws.MinutesPerHour ||
				gs.SecondsPerMinute != ws.SecondsPerMinute || gs.LeapYearEvery != ws.LeapYearEvery ||
				gs.LeapYearOffset != ws.LeapYearOffset || gs.CurrentYear != ws.CurrentYear ||
				optStr(gs.EpochName, "") != optStr(ws.EpochName, "") {
				t.Errorf("step %d importer=%v: settings did not survive the form round trip: "+
					"got %+v, want %+v", step, importer, gs, ws)
			}
			if len(got.Moons) != len(want.Moons) {
				t.Fatalf("step %d importer=%v: moons %d, want %d",
					step, importer, len(got.Moons), len(want.Moons))
			}
			for i := range want.Moons {
				if got.Moons[i] != want.Moons[i] {
					t.Errorf("step %d importer=%v: moon %d did not survive the form round trip: "+
						"got %+v, want %+v", step, importer, i, got.Moons[i], want.Moons[i])
				}
			}
			if len(got.Eras) != len(want.Eras) {
				t.Fatalf("step %d importer=%v: eras %d, want %d",
					step, importer, len(got.Eras), len(want.Eras))
			}
			for i := range want.Eras {
				gc, wc := optStr(got.Eras[i].Description, ""), optStr(want.Eras[i].Description, "")
				if gc != wc {
					t.Errorf("step %d importer=%v: era %d code did not survive the form round "+
						"trip: got %q, want %q", step, importer, i, gc, wc)
				}
			}
		}
	}
}
