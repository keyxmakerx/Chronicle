package sessions

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// Hours saved as UTC, re-told in Chicago with the same real times: the case
// of a player who really did paint UTC times.
func TestConvertBlocks_KeepsTheRealTimes(t *testing.T) {
	ref := timeutil.CivilDate{Year: 2026, Month: time.December, Day: 1} // a Tuesday, CST
	in := []AvailabilityBlock{
		{DayOfWeek: int(time.Monday), StartMinute: 18 * 60, EndMinute: 24 * 60, State: AvailAvailable, TZ: "UTC"},
		{DayOfWeek: int(time.Sunday), StartMinute: 2 * 60, EndMinute: 9 * 60, State: AvailPreferred, TZ: "UTC"},
	}
	got := convertBlocks(in, "America/Chicago", ref)
	want := []AvailabilityBlock{
		{DayOfWeek: int(time.Monday), StartMinute: 12 * 60, EndMinute: 18 * 60, State: AvailAvailable, TZ: "America/Chicago"},
		{DayOfWeek: int(time.Saturday), StartMinute: 20 * 60, EndMinute: 24 * 60, State: AvailPreferred, TZ: "America/Chicago"},
		{DayOfWeek: int(time.Sunday), StartMinute: 0, EndMinute: 3 * 60, State: AvailPreferred, TZ: "America/Chicago"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("convertBlocks:\n got %+v\nwant %+v", got, want)
	}
}

// A block on one week track that moves back across Sunday lands on the
// Saturday before, which belongs to the other track's week.
func TestConvertBlocks_KeepsTheWeekTrackOfTheRealDate(t *testing.T) {
	ref := timeutil.CivilDate{Year: 2026, Month: time.December, Day: 1}
	in := []AvailabilityBlock{{DayOfWeek: int(time.Sunday), StartMinute: 60, EndMinute: 120, State: AvailAvailable, TZ: "UTC", WeekCadence: CadenceWeekA}}
	got := convertBlocks(in, "America/Chicago", ref)
	if len(got) != 1 || got[0].DayOfWeek != int(time.Saturday) {
		t.Fatalf("got %+v, want one Saturday block", got)
	}
	// The real Sunday it stood for, and the Saturday before it.
	sun := ref
	for int(sun.Weekday()) != int(time.Sunday) || !cadenceApplies(CadenceWeekA, sun) {
		sun = sun.AddDays(1)
	}
	if !cadenceApplies(got[0].WeekCadence, sun.AddDays(-1)) {
		t.Errorf("the Saturday block's track %d does not apply on %s", got[0].WeekCadence, sun.AddDays(-1))
	}
}

func TestConfirmMyAvailabilityZone(t *testing.T) {
	saved := []AvailabilityBlock{{DayOfWeek: int(time.Monday), StartMinute: 18 * 60, EndMinute: 24 * 60, State: AvailAvailable, TZ: "UTC"}}
	cases := []struct {
		mode, tz  string
		wantErr   bool
		rewritten bool
		start     int
		from      string
		confirmed bool
	}{
		{mode: "relabel", tz: "America/Chicago", rewritten: true, start: 18 * 60, from: "UTC"},
		{mode: "convert", tz: "America/Chicago", rewritten: true, start: 12 * 60, from: ""},
		{mode: "keep", confirmed: true},
		{mode: "relabel", tz: "Mars/Olympus", wantErr: true},
		{mode: "shrug", tz: "UTC", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.mode+" "+tc.tz, func(t *testing.T) {
			var gotTZ, gotFrom string
			var got []AvailabilityBlock
			rewritten, confirmed := false, false
			repo := &mockSessionRepo{
				listUserAvailabilityFn: func(context.Context, string, string) ([]AvailabilityBlock, error) { return saved, nil },
				rewriteAvailabilityZoneFn: func(_ context.Context, _, _ string, tz string, b []AvailabilityBlock, from string) error {
					rewritten, gotTZ, got, gotFrom = true, tz, b, from
					return nil
				},
				confirmAvailabilityZoneFn: func(context.Context, string, string) error { confirmed = true; return nil },
			}
			err := NewSessionService(repo, nil, nil).ConfirmMyAvailabilityZone(context.Background(), "c1", "u1", ConfirmZoneRequest{Mode: tc.mode, TZ: tc.tz})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if rewritten != tc.rewritten || confirmed != tc.confirmed {
				t.Fatalf("rewritten %v confirmed %v, want %v %v", rewritten, confirmed, tc.rewritten, tc.confirmed)
			}
			if tc.rewritten && (gotTZ != tc.tz || gotFrom != tc.from || len(got) != 1 || got[0].StartMinute != tc.start) {
				t.Errorf("rewrote %q from %q with %+v", gotTZ, gotFrom, got)
			}
		})
	}
}
