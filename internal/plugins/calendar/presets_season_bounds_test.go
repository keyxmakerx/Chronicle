package calendar

import (
	"encoding/json"
	"testing"
)

// TestChroniclePresetSeasonsFitTheirOwnMonths pins that every
// chronicle-calendar-v1 preset's season boundaries name a real day of a real
// month in that preset's OWN month list, checked against the raw preset
// JSON rather than what LoadPreset returns: clampCalendarStructure silently
// forces an out-of-range day back into range on import, which would hide a
// mismatched preset instead of catching it at the source.
func TestChroniclePresetSeasonsFitTheirOwnMonths(t *testing.T) {
	names, err := PresetNames()
	if err != nil {
		t.Fatalf("PresetNames: %v", err)
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			data, err := presetFS.ReadFile("presets/" + name + ".json")
			if err != nil {
				t.Fatalf("read preset: %v", err)
			}
			var export ChronicleExport
			if err := json.Unmarshal(data, &export); err != nil {
				t.Fatalf("unmarshal preset: %v", err)
			}

			months := export.Calendar.Months
			if len(months) == 0 {
				// Not this format (e.g. a Simple Calendar-shaped preset), or
				// legitimately month-less (blank) — nothing to check here.
				return
			}

			checkBound := func(seasonName, which string, month, day int) {
				if month < 1 || month > len(months) {
					t.Errorf("season %q %s_month is %d, outside this preset's %d months", seasonName, which, month, len(months))
					return
				}
				m := months[month-1]
				if day < 1 || day > m.Days {
					t.Errorf("season %q %s_day is %d, but month %q only has %d days", seasonName, which, day, m.Name, m.Days)
				}
			}

			for _, s := range export.Calendar.Seasons {
				checkBound(s.Name, "start", s.StartMonth, s.StartDay)
				checkBound(s.Name, "end", s.EndMonth, s.EndDay)
			}
		})
	}
}

// monthIndexByName returns the 1-based season-month-number of the month
// named want in months, failing the test if no such month exists. Looking
// the month up by name, rather than trusting a hand-counted position, is
// the point: a hand count is what miscounts festival-day months in the
// first place.
func monthIndexByName(t *testing.T, months []ExportMonth, want string) int {
	t.Helper()
	for i, m := range months {
		if m.Name == want {
			return i + 1
		}
	}
	t.Fatalf("no month named %q in this preset's month list", want)
	return 0
}

// TestHarptosPresetSeasonsMatchCanon pins the four Calendar of Harptos
// seasons to the months Forgotten Realms canon puts them in: Long Night
// (Nightal-Hammer-Midwinter-Alturiak), the Thaw (Ches-Tarsakh-Greengrass-
// Mirtul), High Sun (Kythorn-Flamerule-Midsummer-Eleasis) and the Fading
// (Eleint-Highharvestide-Marpenoth-Uktar-Feast of the Moon). The preset's
// month list has 17 entries (12 regular months plus 5 one-day festivals),
// so a season boundary numbered for a 12-month year lands on the wrong
// month — expected month numbers are looked up by name in the preset's own
// month list rather than hand-counted, so the test can't repeat that
// mistake.
func TestHarptosPresetSeasonsMatchCanon(t *testing.T) {
	data, err := presetFS.ReadFile("presets/harptos.json")
	if err != nil {
		t.Fatalf("read preset: %v", err)
	}
	var export ChronicleExport
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("unmarshal preset: %v", err)
	}
	months := export.Calendar.Months

	seasonByName := func(t *testing.T, want string) ExportSeason {
		t.Helper()
		for _, s := range export.Calendar.Seasons {
			if s.Name == want {
				return s
			}
		}
		t.Fatalf("no season named %q in the harptos preset", want)
		return ExportSeason{}
	}

	tests := []struct {
		season    string
		startName string
		startDay  int
		endName   string
		endDay    int
	}{
		{"Long Night", "Nightal", 1, "Alturiak", 30},
		{"The Thaw", "Ches", 1, "Mirtul", 30},
		{"High Sun", "Kythorn", 1, "Eleasis", 30},
		{"The Fading", "Eleint", 1, "Feast of the Moon", 1},
	}

	for _, tt := range tests {
		t.Run(tt.season, func(t *testing.T) {
			s := seasonByName(t, tt.season)
			wantStartMonth := monthIndexByName(t, months, tt.startName)
			wantEndMonth := monthIndexByName(t, months, tt.endName)

			if s.StartMonth != wantStartMonth || s.StartDay != tt.startDay {
				t.Errorf("season %q starts %d/%d, want %d/%d (%s day %d)",
					tt.season, s.StartMonth, s.StartDay, wantStartMonth, tt.startDay, tt.startName, tt.startDay)
			}
			if s.EndMonth != wantEndMonth || s.EndDay != tt.endDay {
				t.Errorf("season %q ends %d/%d, want %d/%d (%s day %d)",
					tt.season, s.EndMonth, s.EndDay, wantEndMonth, tt.endDay, tt.endName, tt.endDay)
			}
		})
	}
}
