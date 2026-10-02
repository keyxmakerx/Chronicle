package sessions

import (
	"reflect"
	"testing"
)

func TestOccurrenceDates(t *testing.T) {
	tests := []struct {
		name     string
		s        Session
		from, to string
		want     []string
	}{
		{"one-off inside", Session{ScheduledDate: strp("2026-10-10")}, "2026-10-01", "2026-10-31", []string{"2026-10-10"}},
		{"one-off outside", Session{ScheduledDate: strp("2026-11-10")}, "2026-10-01", "2026-10-31", nil},
		{"no date", Session{}, "2026-10-01", "2026-10-31", nil},
		{"weekly from before the window", Session{ScheduledDate: strp("2026-09-24"), IsRecurring: true, RecurrenceType: strp(RecurrenceWeekly)},
			"2026-10-01", "2026-10-31", []string{"2026-10-01", "2026-10-08", "2026-10-15", "2026-10-22", "2026-10-29"}},
		{"biweekly stops at its end date", Session{ScheduledDate: strp("2026-10-03"), IsRecurring: true, RecurrenceType: strp(RecurrenceBiWeekly), RecurrenceEndDate: strp("2026-10-20")},
			"2026-10-01", "2026-10-31", []string{"2026-10-03", "2026-10-17"}},
		{"custom every 3 weeks", Session{ScheduledDate: strp("2026-10-02"), IsRecurring: true, RecurrenceType: strp(RecurrenceCustom), RecurrenceInterval: 3},
			"2026-10-01", "2026-10-31", []string{"2026-10-02", "2026-10-23"}},
		{"monthly", Session{ScheduledDate: strp("2026-08-15"), IsRecurring: true, RecurrenceType: strp(RecurrenceMonthly)},
			"2026-10-01", "2026-10-31", []string{"2026-10-15"}},
		{"unknown recurrence keeps the first night only", Session{ScheduledDate: strp("2026-10-05"), IsRecurring: true, RecurrenceType: strp("fortnightly-ish")},
			"2026-10-01", "2026-10-31", []string{"2026-10-05"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := occurrenceDates(tt.s, tt.from, tt.to); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildNightRoster(t *testing.T) {
	members := []NightMember{{"u1", "Ana"}, {"u2", "Bryn"}, {"u3", "Cal"}, {"u4", "Dee"}, {"u5", "Eli"}}
	rows := []nightRow{
		{UserID: "u1", Status: RSVPAccepted, Note: strp("Might be late"), Excluded: true},
		{UserID: "u2", Status: RSVPTentative, Recheck: true},
		{UserID: "u3", Status: RSVPDeclined},
		{UserID: "u4", Status: RSVPCarriedYes},
		{UserID: "gone", Status: RSVPAccepted, Note: strp("left the campaign")},
	}
	roster, tally := buildNightRoster(members, rows)

	if len(roster) != len(members) {
		t.Fatalf("roster has %d rows, want one per current member (%d)", len(roster), len(members))
	}
	for _, a := range roster {
		if a.UserID == "gone" {
			t.Fatal("a departed member's answer must not reach the roster")
		}
	}
	want := []NightAnswer{
		{UserID: "u1", Name: "Ana", Answer: NightYes, Note: "Might be late", Excluded: true},
		{UserID: "u2", Name: "Bryn", Answer: NightMaybe, Recheck: true},
		{UserID: "u3", Name: "Cal", Answer: NightNo},
		{UserID: "u4", Name: "Dee", Answer: NightNoAnswer, Carried: true},
		{UserID: "u5", Name: "Eli", Answer: NightNoAnswer},
	}
	if !reflect.DeepEqual(roster, want) {
		t.Fatalf("roster\n got %+v\nwant %+v", roster, want)
	}
	// Ana is excluded, so her yes counts nowhere; silence counts as no
	// answer, never as a no.
	if (tally != NightTally{Going: 0, Maybe: 1, Cant: 1, NoAnswer: 2}) {
		t.Fatalf("tally = %+v", tally)
	}
}

func TestValidateNightsRange(t *testing.T) {
	tests := []struct {
		from, to string
		ok       bool
	}{
		{"2026-10-01", "2026-10-31", true},
		{"2026-10-01", "2026-12-02", true},
		{"2026-10-01", "2026-12-03", false},
		{"2026-10-31", "2026-10-01", false},
		{"", "2026-10-01", false},
		{"2026-10-01", "October", false},
	}
	for _, tt := range tests {
		if err := validateNightsRange(tt.from, tt.to); (err == nil) != tt.ok {
			t.Errorf("validateNightsRange(%q, %q) err = %v, want ok=%v", tt.from, tt.to, err, tt.ok)
		}
	}
}

func TestGameNightForViewer(t *testing.T) {
	base := func() GameNight {
		return GameNight{OrganizerID: "dm", Roster: []NightAnswer{{UserID: "dm", Answer: NightYes}, {UserID: "p1", Answer: NightMaybe}}}
	}
	tests := []struct {
		name       string
		viewer     string
		owner      bool
		wantMine   string
		wantExcl   bool
		wantNoMine bool
	}{
		{"organizer", "dm", false, NightYes, true, false},
		{"player", "p1", false, NightMaybe, false, false},
		{"owner who is a player here", "p1", true, NightMaybe, true, false},
		{"not on the roster", "stranger", true, "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := base()
			n.ForViewer(tt.viewer, tt.owner)
			if tt.wantNoMine {
				if n.Mine != nil {
					t.Fatalf("Mine = %+v, want nil", n.Mine)
				}
			} else if n.Mine == nil || n.Mine.Answer != tt.wantMine {
				t.Fatalf("Mine = %+v, want answer %q", n.Mine, tt.wantMine)
			}
			if n.CanExclude != tt.wantExcl {
				t.Fatalf("CanExclude = %v, want %v", n.CanExclude, tt.wantExcl)
			}
		})
	}
}
