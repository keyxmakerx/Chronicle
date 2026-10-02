package admin

import (
	"fmt"
	"time"
)

// HomeTile is one Admin Home tile. Every tile has the same shape so the page
// scans: a big figure or status word, the menu label, one plain line, and a
// dot when something needs a look.
type HomeTile struct {
	Big       string
	Label     string
	Line      string
	Href      string
	Attention bool
}

// HomeGroup is one labelled row of tiles, in menu order.
type HomeGroup struct {
	Label string
	Tiles []HomeTile
}

// homeInput is everything the tiles are derived from. A count of -1 means the
// lookup failed or the source isn't wired, and the tile then shows a dash
// rather than a false zero.
type homeInput struct {
	Users, Campaigns, Features, APIAlerts, ChangesThisWeek int
	DisabledUsers                                          int
	PendingSubmissions                                     int
	RegisteredSystems, FailedSystems                       int
	ActiveSessions, FailedLogins24h                        int
	MediaFiles                                             int
	StorageBytes                                           int64
	DegradedParts                                          int
	SMTPConfigured, SMTPKnown                              bool
	// Backup state: BackupsKnown is false when the lister isn't wired;
	// BackupsEnabled is whether a backup folder is configured; LastBackup is
	// the newest backup file's time, zero when there is none.
	BackupsKnown, BackupsEnabled bool
	LastBackup                   time.Time
	Now                          time.Time
}

// buildHomeGroups lays the tiles out like the admin menu. Only figures the
// handler can already read are shown; a line that would need data that does
// not exist is left out rather than guessed.
func buildHomeGroups(in homeInput) []HomeGroup {
	people := HomeTile{Big: countOrDash(in.Users), Label: "People", Href: "/admin/users", Line: "accounts on this site"}
	if in.DisabledUsers > 0 {
		people.Line = fmt.Sprintf("%d disabled", in.DisabledUsers)
	}

	packages := HomeTile{Big: countOrDash(in.RegisteredSystems), Label: "Packages", Href: "/admin/packages", Line: "game systems loaded"}
	switch {
	case in.PendingSubmissions > 0:
		packages.Line = fmt.Sprintf("%d to review", in.PendingSubmissions)
		packages.Attention = true
	case in.FailedSystems > 0:
		packages.Line = fmt.Sprintf("%d failed to load", in.FailedSystems)
	}
	if in.FailedSystems > 0 {
		packages.Attention = true
	}

	signins := HomeTile{Big: countOrDash(in.ActiveSessions), Label: "Sign-ins & sessions", Href: "/admin/security", Line: "signed in now"}
	if in.FailedLogins24h > 0 {
		signins.Line = fmt.Sprintf("%d failed sign-ins in 24 hours", in.FailedLogins24h)
		signins.Attention = true
	}

	api := HomeTile{Big: countOrDash(in.APIAlerts), Label: "API & access", Href: "/admin/api", Line: "alerts not looked at"}
	api.Attention = in.APIAlerts > 0

	email := HomeTile{Big: "Not set up", Label: "Email", Href: "/admin/smtp", Line: "password resets and invites can't be sent", Attention: true}
	switch {
	case !in.SMTPKnown:
		email = HomeTile{Big: "—", Label: "Email", Href: "/admin/smtp", Line: "status unavailable"}
	case in.SMTPConfigured:
		email = HomeTile{Big: "Set up", Label: "Email", Href: "/admin/smtp", Line: "mail server saved"}
	}

	health := HomeTile{Big: "Healthy", Label: "Health & diagnostics", Href: "/admin/systems",
		Line: fmt.Sprintf("database and %d game systems", in.RegisteredSystems)}
	if in.DegradedParts > 0 {
		health.Big = fmt.Sprintf("%d need a look", in.DegradedParts)
		health.Attention = true
	}

	return []HomeGroup{
		{Label: "Community", Tiles: []HomeTile{
			people,
			{Big: countOrDash(in.Campaigns), Label: "Campaigns", Href: "/admin/campaigns", Line: "campaigns on this site"},
		}},
		{Label: "Packages & apps", Tiles: []HomeTile{
			packages,
			{Big: countOrDash(in.Features), Label: "Features", Href: "/admin/addons", Line: "features available"},
			{Big: "Manage", Label: "Extensions", Href: "/admin/extensions", Line: "install and update extensions"},
		}},
		{Label: "Security", Tiles: []HomeTile{
			signins,
			api,
			{Big: countOrDash(in.ChangesThisWeek), Label: "Admin activity", Href: "/admin/activity", Line: "changes this week"},
		}},
		{Label: "Site", Tiles: []HomeTile{
			{Big: formatBytes(in.StorageBytes), Label: "Storage & cleanup", Href: "/admin/storage", Line: fmt.Sprintf("in %d files", in.MediaFiles)},
			backupTile(in),
			email,
		}},
		{Label: "Tools", Tiles: []HomeTile{
			health,
			{Big: "—", Label: "Design Lab", Href: "/admin/design-lab", Line: "try out page styles"},
		}},
	}
}

// backupTile reports the age of the newest backup file. The dot appears only
// when backups are configured yet no file exists: any age threshold would be a
// guess about how often this site backs up.
func backupTile(in homeInput) HomeTile {
	t := HomeTile{Label: "Backups & restore", Href: "/admin/backup"}
	switch {
	case !in.BackupsKnown:
		t.Big, t.Line = "—", "status unavailable"
	case !in.BackupsEnabled:
		t.Big, t.Line = "Not set up", "no backup folder is configured"
	case in.LastBackup.IsZero():
		t.Big, t.Line, t.Attention = "None yet", "no backup files found", true
	default:
		t.Big, t.Line = backupAge(in.Now.Sub(in.LastBackup)), "since the last backup"
	}
	return t
}

// backupAge words a duration as a round figure ("3 days", "5 hours").
func backupAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return "Under 1 hour"
	case d < 48*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}
