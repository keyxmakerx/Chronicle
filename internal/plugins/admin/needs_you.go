package admin

import (
	"context"
	"fmt"
	"sort"
)

// APIAlertCounter reports unresolved API security alerts for the Home page.
// A one-method interface so admin needs nothing from the sync API plugin.
type APIAlertCounter interface {
	CountUnresolvedAPIAlerts(ctx context.Context) (int, error)
}

// NeedsItem is one row of the Home "Needs you" list: a plain sentence, how many
// things it covers, and the single page that fixes it.
type NeedsItem struct {
	Text   string
	Count  int
	Href   string
	Button string
	// rank orders the list; lower is more urgent.
	rank int
}

// needsInput is everything the list is derived from, gathered by the handler.
// Keeping the derivation a pure function lets the ordering be table-tested.
type needsInput struct {
	UnhealthyPlugins   int
	PendingMigrations  int
	APIAlerts          int
	PendingSubmissions int
	SMTPConfigured     bool
	// SMTPKnown is false when the SMTP service isn't wired, so a missing
	// service is not reported as "email is not set up".
	SMTPKnown bool
}

// buildNeedsYou returns the actionable items, most urgent first: things that
// are broken, then things that block an update, then waiting reviews, then
// optional setup.
func buildNeedsYou(in needsInput) []NeedsItem {
	var items []NeedsItem
	if in.UnhealthyPlugins > 0 {
		items = append(items, NeedsItem{
			Text:   plural(in.UnhealthyPlugins, "part of the site failed to start", "parts of the site failed to start"),
			Count:  in.UnhealthyPlugins,
			Href:   "/admin/systems",
			Button: "See what failed",
			rank:   1,
		})
	}
	if in.PendingMigrations > 0 {
		items = append(items, NeedsItem{
			Text:   plural(in.PendingMigrations, "database update is waiting to be applied", "database updates are waiting to be applied"),
			Count:  in.PendingMigrations,
			Href:   "/admin/database",
			Button: "Open database",
			rank:   2,
		})
	}
	if in.APIAlerts > 0 {
		items = append(items, NeedsItem{
			Text:   plural(in.APIAlerts, "API security alert has not been looked at", "API security alerts have not been looked at"),
			Count:  in.APIAlerts,
			Href:   "/admin/api",
			Button: "Review alerts",
			rank:   3,
		})
	}
	if in.PendingSubmissions > 0 {
		items = append(items, NeedsItem{
			Text:   plural(in.PendingSubmissions, "package is waiting for your review", "packages are waiting for your review"),
			Count:  in.PendingSubmissions,
			Href:   "/admin/packages/pending",
			Button: "Review packages",
			rank:   4,
		})
	}
	if in.SMTPKnown && !in.SMTPConfigured {
		items = append(items, NeedsItem{
			Text:   "Email is not set up, so password resets and invites can't be sent",
			Count:  1,
			Href:   "/admin/smtp",
			Button: "Set up email",
			rank:   5,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].rank < items[j].rank })
	return items
}

// plural picks the singular or plural sentence for a count and keeps the
// number out of the sentence, since the row shows it as a badge.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// needsCountLabel is the badge text for a row.
func needsCountLabel(n int) string {
	return fmt.Sprintf("%d", n)
}
