package entities

import (
	"fmt"
	"strings"
	"testing"
)

// flatBook is a calendar of 30-day months with today at 1043-3-16, so a day
// number is year*360 + month*30 + day.
type flatBook struct{}

func (flatBook) n(y, m, d int) int                  { return y*360 + m*30 + d }
func (flatBook) Valid(y, m, d int) bool             { return m >= 1 && m <= 12 && d >= 1 && d <= 30 }
func (flatBook) Label(y, m, d int) string           { return fmt.Sprintf("M%d %d, %d", m, d, y) }
func (b flatBook) DaysSince(y, m, d int) int        { return b.n(1043, 3, 16) - b.n(y, m, d) }
func (flatBook) Months() []FieldDateMonth           { return nil }
func (flatBook) Today() (int, int, int)             { return 1043, 3, 16 }
func (b flatBook) LabelAfter(y, m, d, n int) string { return fmt.Sprintf("+%d from %d", n, d) }

func diedOn() FieldDefinition {
	return FieldDefinition{Key: "died_on", Label: "Died on", Type: FieldTypeCalendarDate, DateAgo: true,
		Countdowns: []FieldCountdown{
			{Name: "Soul lingers", Lengths: []CountdownLength{{Days: 3}, {Name: "Strong-willed", Days: 10}}},
			{Name: "Wake", Lengths: []CountdownLength{{Days: 1}}, GMOnly: true},
		}}
}

func TestParseFieldDate(t *testing.T) {
	tests := []struct {
		in   any
		ok   bool
		back string
	}{
		{"1043-3-22", true, "1043-3-22"},
		{" 1043-3-22|1 ", true, "1043-3-22|1"},
		{"1043-3-22|0,0", true, "1043-3-22"},
		{"-12-1-1", true, "-12-1-1"},
		{"1043/3/22", false, ""},
		{"", false, ""},
		{12, false, ""},
	}
	for _, tt := range tests {
		d, ok := ParseFieldDate(tt.in)
		if ok != tt.ok || (ok && d.String() != tt.back) {
			t.Errorf("ParseFieldDate(%v) = %+v %v, want %q %v", tt.in, d, ok, tt.back, tt.ok)
		}
	}
}

func TestCheckDateSettings(t *testing.T) {
	bad := []struct {
		name string
		cds  []FieldCountdown
		want string
	}{
		{"no name", []FieldCountdown{{Lengths: []CountdownLength{{Days: 3}}}}, "name each countdown"},
		{"no length", []FieldCountdown{{Name: "x"}}, "needs 1 to"},
		{"too long", []FieldCountdown{{Name: "x", Lengths: []CountdownLength{{Days: 4000}}}}, "runs 1 to"},
		{"unnamed other length", []FieldCountdown{{Name: "x", Lengths: []CountdownLength{{Days: 3}, {Days: 4}}}}, "name the other lengths"},
	}
	for _, tt := range bad {
		f := FieldDefinition{Label: "Died on", Type: FieldTypeCalendarDate, Countdowns: tt.cds}
		if err := CheckDateSettings(&f); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v, want %q", tt.name, err, tt.want)
		}
	}
	f := diedOn()
	if err := CheckDateSettings(&f); err != nil {
		t.Fatal(err)
	}
}

func TestCheckDateValue(t *testing.T) {
	d := diedOn()
	for v, want := range map[string]string{
		"":                "",
		"1043-3-14":       "",
		"1043-3-14|1":     "",
		"1043-13-14":      "not a day",
		"14 Ashfall":      "must be a date",
		"1043-3-14|2":     "doesn't have",
		"1043-3-14|0,0,0": "countdowns it doesn't have",
	} {
		err := checkDateValue(d, v, flatBook{})
		if (want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), want)) {
			t.Errorf("%q: %v, want %q", v, err, want)
		}
	}
	// Without a calendar only the form is checked.
	if err := checkDateValue(d, "1043-13-14", nil); err != nil {
		t.Errorf("no calendar: %v", err)
	}
}

func TestShapeDate(t *testing.T) {
	d := diedOn()
	tests := []struct {
		name      string
		value     string
		director  bool
		ago       string
		state     string
		left      int
		countdown int // how many countdowns show
	}{
		{"running", "1043-3-14", false, "2 days ago", "running", 1, 1},
		{"last day", "1043-3-13", false, "3 days ago", "last", 0, 1},
		{"ended", "1043-3-1", false, "15 days ago", "ended", 0, 1},
		{"still to come", "1043-3-20", false, "in 4 days", "before", 3, 1},
		{"other length", "1043-3-14|1", false, "2 days ago", "running", 8, 1},
		{"director sees all", "1043-3-14", true, "2 days ago", "running", 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sh, ok := shapeDate(d, tt.value, flatBook{}, tt.director)
			if !ok || sh.Date == nil {
				t.Fatalf("shown %+v %v", sh, ok)
			}
			if sh.Date.Ago != tt.ago || len(sh.Date.Countdowns) != tt.countdown {
				t.Fatalf("ago %q countdowns %d", sh.Date.Ago, len(sh.Date.Countdowns))
			}
			c := sh.Date.Countdowns[0]
			if c.State != tt.state || c.Left != tt.left {
				t.Errorf("countdown %+v, want %s %d left", c, tt.state, tt.left)
			}
		})
	}
	if sh, ok := shapeDate(d, "1043-3-14", nil, false); !ok || sh.Text != "1043-3-14" || sh.Date != nil {
		t.Errorf("no calendar: %+v", sh)
	}
}
