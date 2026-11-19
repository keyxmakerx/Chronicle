package timeutil

import "time"

// DateForm names how a real-world date is written. The values are the
// data-date attribute static/js/dates.js reads; keep the two in step.
type DateForm string

const (
	DateShort    DateForm = "short"     // 18 Nov 2026
	DateDay      DateForm = "day"       // 18 Nov
	DateLong     DateForm = "long"      // 18 November 2026
	DateWeekday  DateForm = "weekday"   // Wednesday 18 November
	DateWithTime DateForm = "with-time" // 18 Nov 2026, 15:04
)

// FixedDate writes t in the one fixed form used where there is no reader's
// device to ask: the fallback text inside a page (dates.js rewrites it in the
// browser), emails and exported files. It is day first with the month as a
// word, so it cannot be misread as month first. Page templates never call
// Format for a date a person reads; they use components.Date.
func FixedDate(t time.Time, form DateForm) string {
	switch form {
	case DateDay:
		return t.Format("2 Jan")
	case DateLong:
		return t.Format("2 January 2006")
	case DateWeekday:
		return t.Format("Monday 2 January")
	case DateWithTime:
		return t.Format("2 Jan 2006, 15:04")
	default:
		return t.Format("2 Jan 2006")
	}
}
