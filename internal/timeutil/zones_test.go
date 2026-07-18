package timeutil

import (
	"encoding/json"
	"testing"
)

// The three pre-consolidation curated lists, encoded VERBATIM (C-TZ-
// CONSOLIDATION Step-0 inventory) so this test pins the union property
// against what shipped before — not against CommonZones' own source, which
// would make the test tautological and blind to an accidental future drop.
//
//   - oldAuthList:     internal/plugins/auth/handler.go commonTimezones()
//     (pre-consolidation, account settings dropdown)
//   - oldCalendarList: internal/plugins/calendar/timezones.go commonTimeZones()
//     (pre-consolidation, real-time calendar anchor dropdown) — Step-0 found
//     this byte-identical to oldAuthList (its own header comment says so:
//     an intentional-for-now mirror flagged for a future DRY pass, not a
//     deliberate divergence), so it adds nothing to the union beyond
//     oldAuthList, but is kept here for fidelity to the inventory.
//   - oldAvailabilityList: static/js/availability.js COMMON_TZ (pre-
//     consolidation, availability scheduler zone picker). The one list with
//     a genuine addition: the literal "UTC" entry, absent from the other two.
var oldAuthList = []string{
	"Africa/Cairo", "Africa/Johannesburg", "Africa/Lagos", "Africa/Nairobi",
	"America/Anchorage", "America/Argentina/Buenos_Aires", "America/Bogota",
	"America/Chicago", "America/Denver", "America/Halifax", "America/Los_Angeles",
	"America/Mexico_City", "America/New_York", "America/Phoenix",
	"America/Santiago", "America/Sao_Paulo", "America/St_Johns", "America/Toronto",
	"America/Vancouver",
	"Asia/Baghdad", "Asia/Bangkok", "Asia/Colombo", "Asia/Dubai", "Asia/Hong_Kong",
	"Asia/Istanbul", "Asia/Jakarta", "Asia/Karachi", "Asia/Kolkata", "Asia/Manila",
	"Asia/Seoul", "Asia/Shanghai", "Asia/Singapore", "Asia/Taipei", "Asia/Tehran",
	"Asia/Tokyo",
	"Atlantic/Reykjavik",
	"Australia/Adelaide", "Australia/Brisbane", "Australia/Melbourne",
	"Australia/Perth", "Australia/Sydney",
	"Europe/Amsterdam", "Europe/Athens", "Europe/Berlin", "Europe/Brussels",
	"Europe/Dublin", "Europe/Helsinki", "Europe/Lisbon", "Europe/London",
	"Europe/Madrid", "Europe/Moscow", "Europe/Oslo", "Europe/Paris",
	"Europe/Prague", "Europe/Rome", "Europe/Stockholm", "Europe/Vienna",
	"Europe/Warsaw", "Europe/Zurich",
	"Pacific/Auckland", "Pacific/Fiji", "Pacific/Guam", "Pacific/Honolulu",
}

var oldCalendarList = oldAuthList // Step-0: byte-identical source lists.

var oldAvailabilityList = []string{
	"UTC", "America/New_York", "America/Chicago", "America/Denver",
	"America/Los_Angeles", "America/Anchorage", "America/Phoenix",
	"America/Toronto", "America/Sao_Paulo", "Europe/London", "Europe/Dublin",
	"Europe/Paris", "Europe/Berlin", "Europe/Madrid", "Europe/Rome",
	"Europe/Athens", "Europe/Moscow", "Africa/Johannesburg", "Asia/Dubai",
	"Asia/Kolkata", "Asia/Bangkok", "Asia/Singapore", "Asia/Shanghai",
	"Asia/Tokyo", "Australia/Sydney", "Pacific/Auckland",
}

// commonZoneValueSet returns CommonZones' values as a set, for membership tests.
func commonZoneValueSet(t *testing.T) map[string]bool {
	t.Helper()
	set := make(map[string]bool)
	for _, z := range CommonZones() {
		set[z.Value] = true
	}
	return set
}

// TestCommonZones_IsUnionOfOldLists is the semantics pin: no zone any of the
// three pre-consolidation surfaces could offer may be missing from the
// consolidated list. Losing a zone a calendar or availability row already
// uses would be a data-facing regression, not just a cosmetic one.
func TestCommonZones_IsUnionOfOldLists(t *testing.T) {
	got := commonZoneValueSet(t)

	for _, old := range [][]string{oldAuthList, oldCalendarList, oldAvailabilityList} {
		for _, zone := range old {
			if !got[zone] {
				t.Errorf("CommonZones() is missing %q, present in a pre-consolidation list — a user could previously pick this zone", zone)
			}
		}
	}
}

// TestCommonZones_ExactUnion pins the OTHER direction too: CommonZones must
// be the union and nothing more (no new zones snuck in beyond what the
// dispatch authorizes) — every entry traces back to one of the three old
// lists. New zones beyond the union are explicitly out of scope
// (cordinator/dispatches/chronicle/C-TZ-CONSOLIDATION.md "Out of scope").
func TestCommonZones_ExactUnion(t *testing.T) {
	union := make(map[string]bool)
	for _, old := range [][]string{oldAuthList, oldCalendarList, oldAvailabilityList} {
		for _, zone := range old {
			union[zone] = true
		}
	}

	got := commonZoneValueSet(t)
	if len(got) != len(union) {
		t.Errorf("CommonZones() has %d entries, want exactly %d (the union of the old lists)", len(got), len(union))
	}
	for zone := range got {
		if !union[zone] {
			t.Errorf("CommonZones() contains %q, not present in any pre-consolidation list", zone)
		}
	}
}

// TestCommonZones_AllLoadable pins that every emitted zone actually resolves
// against the host's tz database — the curated list is a UX convenience, but
// an unselectable option (one that fails to load) would be worse than not
// listing it, matching the validation the three old lists each already did.
func TestCommonZones_AllLoadable(t *testing.T) {
	for _, z := range CommonZones() {
		if !IsValidLocation(z.Value) {
			t.Errorf("CommonZones() contains %q, which does not resolve via time.LoadLocation", z.Value)
		}
		if z.Value != z.Label {
			t.Errorf("Zone %+v: Value and Label differ — no consumer expects a friendly name yet", z)
		}
	}
}

// TestCommonZones_NoDuplicates guards against a future edit to
// commonZoneNames introducing an accidental repeat (would render two
// identical <option>s).
func TestCommonZones_NoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, z := range CommonZones() {
		if seen[z.Value] {
			t.Errorf("CommonZones() contains duplicate %q", z.Value)
		}
		seen[z.Value] = true
	}
}

// TestCommonZones_ExistingFixturesStillResolve pins the specific zones
// exercised heavily by other packages' test fixtures (sessions/calendar DST
// tests) — a reader's guarantee that consolidation didn't disturb them, even
// though by construction a union can only add entries, never drop one.
func TestCommonZones_ExistingFixturesStillResolve(t *testing.T) {
	for _, zone := range []string{"America/New_York", "America/Chicago", "Europe/London"} {
		if !IsValidLocation(zone) {
			t.Fatalf("fixture zone %q no longer resolves", zone)
		}
	}
}

// TestCommonZonesJSON_MarshalsValuesOnly pins the wire shape the JS consumer
// depends on: a flat JSON array of zone identifiers (no label objects) in the
// same order as CommonZones, so availability.js's `zones.indexOf(z)` /
// `.forEach` usage keeps working unchanged.
func TestCommonZonesJSON_MarshalsValuesOnly(t *testing.T) {
	var values []string
	if err := json.Unmarshal([]byte(CommonZonesJSON()), &values); err != nil {
		t.Fatalf("CommonZonesJSON() did not unmarshal as []string: %v", err)
	}
	zones := CommonZones()
	if len(values) != len(zones) {
		t.Fatalf("CommonZonesJSON() has %d entries, want %d (len(CommonZones()))", len(values), len(zones))
	}
	for i, z := range zones {
		if values[i] != z.Value {
			t.Errorf("CommonZonesJSON()[%d] = %q, want %q", i, values[i], z.Value)
		}
	}
}
