package foundry_vtt

import (
	"fmt"
	"time"
)

// relativeTime renders a coarse "Xm ago / Xh ago / Xd ago" string
// for the admin "Campaigns Using v0.1.5" panel's last-active column.
//
// Local helper rather than a shared util because the codebase
// pattern is per-plugin formatting helpers (campaigns has its own,
// syncapi has its own, foundry_modules had its own — now ported
// here as part of C-FMC-5c).
//
// Coarse buckets on purpose — admins comparing "last active 30
// minutes ago vs 35 minutes ago" don't need precision; they want
// to see at a glance which campaigns are dormant.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
