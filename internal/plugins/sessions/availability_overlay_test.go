package sessions

import (
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

// week of Monday 2026-07-13 (Tue=07-14, Wed=07-15, Thu=07-16, Sat=07-18).
func testWeekStart() timeutil.CivilDate {
	return timeutil.CivilDate{Year: 2026, Month: time.July, Day: 13}
}

func block(dow, startMin, endMin int, state, tz string) AvailabilityBlock {
	return AvailabilityBlock{DayOfWeek: dow, StartMinute: startMin, EndMinute: endMin, State: state, TZ: tz}
}

func TestBuildWeekOverlay_SameZoneSingleMember(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	members := []overlayMemberInput{{UserID: "u1", Name: "Alex", IsOwner: false}}
	avail := map[string][]AvailabilityBlock{
		"u1": {block(2 /*Tue*/, 18*60, 21*60, AvailAvailable, "America/New_York")},
	}
	ov := buildWeekOverlay(members, avail, nil, testWeekStart(), ny, "America/New_York", true)

	if ov.TotalMembers != 1 {
		t.Fatalf("TotalMembers = %d, want 1", ov.TotalMembers)
	}
	tue := ov.Days[1] // Monday is index 0
	if tue.Date != "2026-07-14" || tue.Weekday != int(time.Tuesday) {
		t.Fatalf("Days[1] = %s (wd %d), want 2026-07-14 Tuesday", tue.Date, tue.Weekday)
	}
	for h := 0; h < 24; h++ {
		want := 0
		if h == 18 || h == 19 || h == 20 {
			want = 1
		}
		if tue.Hours[h].Free != want {
			t.Errorf("Tue hour %d Free = %d, want %d", h, tue.Hours[h].Free, want)
		}
	}
	if len(ov.Members) != 1 || len(ov.Members[0].Lanes) != 1 {
		t.Fatalf("expected 1 member with 1 lane, got %d members", len(ov.Members))
	}
	lane := ov.Members[0].Lanes[0]
	if lane.DayIndex != 1 || lane.StartMinute != 18*60 || lane.EndMinute != 21*60 {
		t.Errorf("lane = %+v, want day 1 1080-1260", lane)
	}
}

// Cross-zone projection: a London member's Saturday evening lands earlier the
// same day in New York (London 20:00 BST == 15:00 EDT). Proves the overlay
// converts each member's zone-local block into the viewer's zone.
func TestBuildWeekOverlay_CrossZone(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	members := []overlayMemberInput{{UserID: "lon", Name: "Devi"}}
	avail := map[string][]AvailabilityBlock{
		"lon": {block(6 /*Sat*/, 20*60, 23*60, AvailAvailable, "Europe/London")},
	}
	ov := buildWeekOverlay(members, avail, nil, testWeekStart(), ny, "America/New_York", true)

	sat := ov.Days[5] // Saturday 2026-07-18
	if sat.Date != "2026-07-18" {
		t.Fatalf("Days[5] = %s, want 2026-07-18", sat.Date)
	}
	// London 20:00–23:00 BST => New York 15:00–18:00 EDT.
	for h := 0; h < 24; h++ {
		want := 0
		if h == 15 || h == 16 || h == 17 {
			want = 1
		}
		if sat.Hours[h].Free != want {
			t.Errorf("Sat NY hour %d Free = %d, want %d (cross-zone)", h, sat.Hours[h].Free, want)
		}
	}
}

// C-SCHED-P2 0a pin — the widened projection window (-2..+8) must capture a
// block from a >24h zone spread. Reproducer from the #530 gate: a
// Pacific/Kiritimati (UTC+14) member's recurring Tuesday 00:00–01:00 block,
// viewed from Pacific/Pago_Pago (UTC-11), lands on the visible week's SUNDAY
// 23:00 — sourced from real-date offset +8 (2026-07-21, a Tuesday), a column
// the old -1..+7 loop never visited. This test fails on the old window.
func TestBuildWeekOverlay_WideZoneSpread_Kiritimati_PagoPago(t *testing.T) {
	pago := mustLoc(t, "Pacific/Pago_Pago")
	members := []overlayMemberInput{{UserID: "kir", Name: "Teuru"}}
	avail := map[string][]AvailabilityBlock{
		"kir": {block(2 /*Tue*/, 0, 60, AvailAvailable, "Pacific/Kiritimati")},
	}
	ov := buildWeekOverlay(members, avail, nil, testWeekStart(), pago, "Pacific/Pago_Pago", true)

	sun := ov.Days[6] // Sunday 2026-07-19 (last column)
	if sun.Date != "2026-07-19" {
		t.Fatalf("Days[6] = %s, want 2026-07-19 (Sunday)", sun.Date)
	}
	// Tue 07-21 00:00 Kiritimati (UTC+14) == 07-20 10:00 UTC == 07-19 23:00
	// Pago_Pago (UTC-11). Without the widened window this cell is dropped.
	if sun.Hours[23].Free != 1 {
		t.Errorf("Sun Pago_Pago hour 23 Free = %d, want 1 (wide zone spread must be captured)", sun.Hours[23].Free)
	}
}

// An exception fully replaces the recurring pattern for its date: an
// "unavailable" override punches the member out of that day's overlay.
func TestBuildWeekOverlay_ExceptionOverridesRecurring(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	members := []overlayMemberInput{{UserID: "u1", Name: "Alex"}}
	avail := map[string][]AvailabilityBlock{
		"u1": {block(4 /*Thu*/, 18*60, 22*60, AvailAvailable, "America/New_York")},
	}
	exc := map[string][]AvailabilityException{
		"u1": {{OnDate: "2026-07-16", StartMinute: 0, EndMinute: 1440, State: AvailUnavailable, TZ: "America/New_York"}},
	}
	ov := buildWeekOverlay(members, avail, exc, testWeekStart(), ny, "America/New_York", true)

	thu := ov.Days[3] // Thursday 2026-07-16
	for h := 0; h < 24; h++ {
		if thu.Hours[h].Free != 0 {
			t.Errorf("Thu hour %d Free = %d, want 0 (exception removed availability)", h, thu.Hours[h].Free)
		}
	}
}

func TestBuildWeekOverlay_DetailGatingHidesRoster(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	members := []overlayMemberInput{{UserID: "u1", Name: "Alex"}}
	avail := map[string][]AvailabilityBlock{
		"u1": {block(2, 18*60, 19*60, AvailAvailable, "America/New_York")},
	}
	ov := buildWeekOverlay(members, avail, nil, testWeekStart(), ny, "America/New_York", false)

	if len(ov.Members) != 0 {
		t.Errorf("non-detail overlay leaked %d member roster entries, want 0", len(ov.Members))
	}
	tue := ov.Days[1]
	if tue.Hours[18].Free != 1 {
		t.Errorf("aggregate count should still be visible: Tue 18 Free = %d, want 1", tue.Hours[18].Free)
	}
	if tue.Hours[18].FreeIDs != nil {
		t.Errorf("non-detail overlay leaked per-cell user IDs: %v", tue.Hours[18].FreeIDs)
	}
}

func TestBuildWeekOverlay_DensityAndPreferCounts(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	members := []overlayMemberInput{{UserID: "a", Name: "A"}, {UserID: "b", Name: "B"}}
	avail := map[string][]AvailabilityBlock{
		"a": {block(3 /*Wed*/, 19*60, 20*60, AvailPreferred, "America/New_York")},
		"b": {block(3, 19*60, 20*60, AvailPreferred, "America/New_York")},
	}
	ov := buildWeekOverlay(members, avail, nil, testWeekStart(), ny, "America/New_York", true)

	wed := ov.Days[2] // Wednesday
	if wed.Hours[19].Free != 2 || wed.Hours[19].Prefer != 2 {
		t.Errorf("Wed 19: Free=%d Prefer=%d, want 2/2 (full house, all keen → client ★)", wed.Hours[19].Free, wed.Hours[19].Prefer)
	}
	if len(wed.Hours[19].FreeIDs) != 2 {
		t.Errorf("detail FreeIDs = %v, want 2 ids", wed.Hours[19].FreeIDs)
	}
}
