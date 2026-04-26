package backup

import "fmt"

// humanBytes formats a byte count for display in the artifact list.
// Plain decimal units (KB, MB, GB) are used because backup operators
// reason about disk in MB/GB terms; binary units (MiB, GiB) would only
// add cognitive load for the same on-screen space.
func humanBytes(n int64) string {
	const (
		kb = 1000
		mb = 1000 * kb
		gb = 1000 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
