package audit

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// SittingGap ends a sitting: 30 quiet minutes from one author, the same rule
// as the AI's history, so both pages count a stretch of work the same way.
const SittingGap = 30 * time.Minute

// feedEntries and feedFoundry bound what the activity reads: the newest
// changes, enough for weeks of play, never the whole log.
const (
	feedEntries = 1000
	feedFoundry = 500
)

// Kinds of row: what the chips filter and the strip's ticks show.
const (
	KindPage     = "page"
	KindSetting  = "setting"
	KindPlayers  = "players"
	KindFoundry  = "foundry"
	KindConflict = "conflict"
)

// FoundryAuthor keys Foundry Sync's sittings; no member id can equal it.
const FoundryAuthor = "foundry"

// FoundryEvent is one sync history row as the activity reads it: a change
// Foundry made in Chronicle, a connect or disconnect, or a settled conflict.
type FoundryEvent struct {
	At         time.Time
	Kind       string
	ResourceID string
	Name       string
	Action     string
	Direction  string
	Status     string
	Message    string
}

// IsConflict reports a row the module sent when it settled a conflict.
func (e FoundryEvent) IsConflict() bool { return e.Status == "conflict" }

// FoundryKept reports which side a settled conflict kept: true for Foundry.
func (e FoundryEvent) FoundryKept() bool { return strings.HasPrefix(e.Action, "Foundry") }

// FoundryActivitySource reads a campaign's sync history. It is wired from
// the sync plugin in app; nil leaves Foundry out of the activity.
type FoundryActivitySource interface {
	FoundryActivity(ctx context.Context, campaignID string, limit int) ([]FoundryEvent, error)
}

// ActivityRow is one change in the feed: a logged action or a Foundry event.
type ActivityRow struct {
	At      time.Time
	Kind    string
	Entry   *AuditEntry
	Foundry *FoundryEvent
}

// Sitting is one author's stretch of work, its rows oldest first.
type Sitting struct {
	Author     string
	AuthorName string
	Start, End time.Time
	Rows       []ActivityRow
}

// Count is how many rows of a kind a sitting holds.
func (s Sitting) Count(kind string) int {
	n := 0
	for _, r := range s.Rows {
		if r.Kind == kind {
			n++
		}
	}
	return n
}

// Feed is the activity page's list: sittings newest first, and how many
// rows of each kind they hold, for the chips.
type Feed struct {
	Sittings []Sitting
	Counts   map[string]int
	Total    int
}

// entryKind sorts a logged action into a chip. Pages are anything written
// in the campaign (pages, the calendar, timelines, tags); players are who is
// in it and their AI; the rest is a setting.
func entryKind(action string) string {
	area := actionArea(action)
	switch {
	case area == actionArea(ActionEntityCreated), area == actionArea(ActionCalendarCreated),
		area == actionArea(ActionTimelineCreated), area == actionArea(ActionTagCreated),
		area == actionArea(ActionItemShared):
		return KindPage
	case area == actionArea(ActionMemberJoined), area == "group", area == "guest",
		strings.HasPrefix(action, "campaign.ai_player"):
		return KindPlayers
	}
	return KindSetting
}

// foundryKind is a Foundry row's chip: a conflict is its own kind.
func foundryKind(e FoundryEvent) string {
	if e.IsConflict() {
		return KindConflict
	}
	return KindFoundry
}

// buildFeed merges logged actions and Foundry events and cuts each author's
// rows into sittings. Rows from different authors never share a sitting, so
// two people working at once each get their own.
func buildFeed(entries []AuditEntry, foundry []FoundryEvent) Feed {
	rows := make([]ActivityRow, 0, len(entries)+len(foundry))
	for i := range entries {
		e := &entries[i]
		rows = append(rows, ActivityRow{At: e.CreatedAt, Kind: entryKind(e.Action), Entry: e})
	}
	for i := range foundry {
		f := &foundry[i]
		rows = append(rows, ActivityRow{At: f.At, Kind: foundryKind(*f), Foundry: f})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].At.After(rows[j].At) })

	feed := Feed{Counts: map[string]int{}, Total: len(rows)}
	open := map[string]int{} // author -> index of their newest open sitting
	for _, r := range rows {
		author, name := FoundryAuthor, "Foundry Sync"
		if r.Entry != nil {
			author, name = r.Entry.UserID, r.Entry.UserName
		}
		feed.Counts[r.Kind]++
		if i, ok := open[author]; ok && feed.Sittings[i].Start.Sub(r.At) <= SittingGap {
			s := &feed.Sittings[i]
			s.Start = r.At
			s.Rows = append(s.Rows, r)
			continue
		}
		open[author] = len(feed.Sittings)
		feed.Sittings = append(feed.Sittings, Sitting{Author: author, AuthorName: name, Start: r.At, End: r.At, Rows: []ActivityRow{r}})
	}
	// Each sitting reads in the order things happened, oldest first.
	for i := range feed.Sittings {
		r := feed.Sittings[i].Rows
		for a, b := 0, len(r)-1; a < b; a, b = a+1, b-1 {
			r[a], r[b] = r[b], r[a]
		}
	}
	return feed
}

// SetFoundryActivity wires the sync history into the activity feed.
func (s *auditService) SetFoundryActivity(src FoundryActivitySource) { s.foundry = src }

// RecentActivity reads the newest changes and Foundry events as sittings.
// A Foundry read that fails leaves Foundry out rather than failing the page.
func (s *auditService) RecentActivity(ctx context.Context, campaignID string) (Feed, error) {
	entries, _, err := s.repo.ListByCampaign(ctx, campaignID, feedEntries, 0)
	if err != nil {
		return Feed{}, err
	}
	var foundry []FoundryEvent
	if s.foundry != nil {
		fe, ferr := s.foundry.FoundryActivity(ctx, campaignID, feedFoundry)
		if ferr != nil {
			slog.Warn("activity: reading Foundry sync history", slog.String("campaign_id", campaignID), slog.Any("error", ferr))
		}
		foundry = fe
	}
	return buildFeed(entries, foundry), nil
}
