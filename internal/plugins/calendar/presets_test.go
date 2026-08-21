package calendar

import "testing"

// TestEveryShippedPresetParsesThroughTheImporter is the structural proof the
// preset ruling depends on: a preset IS an export, and the only way to read one
// is through the shipped parser.
//
// CALV5 SALVAGE: written for the recovered payloads. The pre-deletion gallery
// asserted this implicitly by having no other code path; with the gallery UI
// gone, the invariant needs saying out loud — otherwise a future preset could
// be added in a shape only a bespoke loader understands, which is how the
// second parser gets built.
func TestEveryShippedPresetParsesThroughTheImporter(t *testing.T) {
	names, err := PresetNames()
	if err != nil {
		t.Fatalf("PresetNames: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("no presets embedded — the payloads were lost, not salvaged")
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			res, err := LoadPreset(name)
			if err != nil {
				t.Fatalf("preset %q does not survive DetectAndParse: %v\n"+
					"A preset that needs its own loader is the start of a second "+
					"parser, which is exactly what the one-code-path ruling forbids.", name, err)
			}
			if res == nil || res.CalendarName == "" {
				t.Fatalf("preset %q parsed to an unusable calendar", name)
			}
			// "blank" is legitimately month-less; every other preset must carry
			// a real structure or it is not a calendar anyone can start from.
			if name != "blank" && len(res.Months) == 0 {
				t.Errorf("preset %q parsed with no months", name)
			}
		})
	}
}

// TestPresetNamesAreDeterministic guards the ordering bug the pre-deletion tree
// carried a dedicated regression test for: map iteration produced a different
// month order between runs, so the same preset built a different calendar
// depending on when you clicked it.
func TestPresetNamesAreDeterministic(t *testing.T) {
	first, err := PresetNames()
	if err != nil {
		t.Fatalf("PresetNames: %v", err)
	}
	for i := 0; i < 20; i++ {
		again, err := PresetNames()
		if err != nil {
			t.Fatalf("PresetNames: %v", err)
		}
		if len(again) != len(first) {
			t.Fatalf("preset count changed between calls: %d then %d", len(first), len(again))
		}
		for j := range first {
			if again[j] != first[j] {
				t.Fatalf("preset order is not stable: %v then %v", first, again)
			}
		}
	}
}
