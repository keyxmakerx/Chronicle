package timeutil

import (
	"fmt"
	"time"
)

// Ago renders how long before now t was, in whole units of the largest size
// that fits ("just now", "5 minutes ago", "2 days ago", "3 months ago"). A
// time at or after now reads "just now", so a small clock skew between the
// database and the app never shows a negative age. Months are 30 days and
// years 365, which is coarse on purpose: the exact moment belongs in the
// element's title and datetime, not in the label.
func Ago(t, now time.Time) string {
	d := now.Sub(t)
	unit := func(n int, name string) string {
		if n == 1 {
			return "1 " + name + " ago"
		}
		return fmt.Sprintf("%d %ss ago", n, name)
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return unit(int(d/time.Minute), "minute")
	case d < 24*time.Hour:
		return unit(int(d/time.Hour), "hour")
	case d < 30*24*time.Hour:
		return unit(int(d/(24*time.Hour)), "day")
	case d < 365*24*time.Hour:
		return unit(int(d/(30*24*time.Hour)), "month")
	default:
		return unit(int(d/(365*24*time.Hour)), "year")
	}
}

// EditedAgo is Ago for a card or row: "Edited 2 days ago", or "Edited just
// now" for a fresh change.
func EditedAgo(t, now time.Time) string {
	return "Edited " + Ago(t, now)
}
