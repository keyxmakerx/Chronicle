// timezones.go — curated IANA time-zone list for the real-time calendar anchor
// dropdown (C-REAL-CALENDAR-P2).
//
// This mirrors auth.commonTimezones (internal/plugins/auth/handler.go) rather than
// reusing it: that function is unexported, and the r13 Wave-2 surface lock (RC-13)
// scopes this change to the calendar plugin + syncapi, so the auth package is off
// limits this wave. Keeping an in-plugin copy avoids a cross-package edit; the
// coordinator may DRY the two into a shared helper post-wave. The list is validated
// against Go's tz database at call time, so an entry missing from the host's tzdata
// is silently dropped instead of rendering an unselectable option.
package calendar

import "time"

// commonTimeZones returns a curated list of loadable IANA time zones for the
// real-time anchor dropdown — the major regions without overwhelming the user
// with every Olson entry. The server independently re-validates the chosen zone
// via time.LoadLocation on enable (RC-2), so this list is a UX convenience, not
// the security boundary.
func commonTimeZones() []string {
	regions := []string{
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
	zones := make([]string, 0, len(regions))
	for _, tz := range regions {
		if _, err := time.LoadLocation(tz); err == nil {
			zones = append(zones, tz)
		}
	}
	return zones
}
