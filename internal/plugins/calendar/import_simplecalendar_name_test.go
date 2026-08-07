package calendar

import "testing"

// TestParseSimpleCalendar_CarriesTheFilesName pins that the Simple Calendar
// parser reads `calendar.name`. It used to seed CalendarName with the
// "Imported Calendar" placeholder and never touch it again, so every Simple
// Calendar import arrived on the wizard's Review station unnamed — and no
// downstream consumer could rescue it, because handler.go's
// `if calName == "" { calName = "Imported Calendar" }` guard only fires on an
// EMPTY name and the parser handed it a non-empty placeholder. The sibling
// Calendaria and Fantasy-Calendar parsers both carried the name through; this
// one alone dropped it.
func TestParseSimpleCalendar_CarriesTheFilesName(t *testing.T) {
	const months = `"months":[{"name":"Hammer","numberOfDays":30}],"weekdays":[{"name":"Sul"}]`

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "v1 single-calendar shape",
			raw:  `{"calendar":{"name":"Calendar of Harptos",` + months + `}}`,
			want: "Calendar of Harptos",
		},
		{
			name: "v2 exportVersion shape",
			raw:  `{"exportVersion":2,"calendars":[{"name":"Calendar of Harptos",` + months + `}]}`,
			want: "Calendar of Harptos",
		},
		{
			// Simple Calendar ships localization-key names; every other name
			// field in this parser is read through stripLocalizationKey, so
			// this one is too.
			name: "localization-key name is stripped like every sibling field",
			raw:  `{"calendar":{"name":"FSC.Calendar.Gregorian",` + months + `}}`,
			want: "Gregorian",
		},
		{
			// The placeholder is the FALLBACK, not the answer — a nameless
			// file must still get one.
			name: "nameless file keeps the placeholder",
			raw:  `{"calendar":{` + months + `}}`,
			want: "Imported Calendar",
		},
		{
			name: "whitespace-only name keeps the placeholder",
			raw:  `{"calendar":{"name":"   ",` + months + `}}`,
			want: "Imported Calendar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Go through the production entry point, not the inner helper, so
			// the detector is on the hook too.
			if got := detectFormat([]byte(tt.raw)); got != FormatSimpleCal {
				t.Fatalf("fixture must land on the Simple Calendar parser; detectFormat = %q", got)
			}
			res, err := DetectAndParse([]byte(tt.raw))
			if err != nil {
				t.Fatalf("DetectAndParse: %v", err)
			}
			if res.CalendarName != tt.want {
				t.Errorf("CalendarName = %q, want %q", res.CalendarName, tt.want)
			}
		})
	}
}
