package sessions

// What the campaign home's "Since you were last here" block reads from game
// nights: recaps posted, nights set and date polls opened after a time.
//
// It is its own reader over the database, not part of SessionService, so the
// block (assembled in internal/app, which the home may not import) needs no
// new method on a service interface that many fakes implement. Every query is
// bounded by campaign and time and capped, and each source answers on its own:
// the caller leaves out the row of one that fails.

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// sinceLimit caps a source's rows. The look-back is at most thirty days, so
// this is far above what a table produces; it only bounds a bulk import.
const sinceLimit = 50

// SinceViewer says who is reading. DMTeam is the owner or a Co-DM, and is
// false for an owner previewing as a player, so that reader gets the recap a
// player gets.
type SinceViewer struct {
	DMTeam bool
}

// SinceItem is one night or poll after the cut-off. Date is a night's own
// YYYY-MM-DD ("" for the others).
type SinceItem struct {
	ID   string
	Name string
	Date string
	At   time.Time
}

// SinceReader reads game-night news.
type SinceReader struct {
	db *sql.DB
}

// NewSinceReader creates the reader.
func NewSinceReader(db *sql.DB) *SinceReader { return &SinceReader{db: db} }

var recapTextTag = regexp.MustCompile(`<[^>]*>`)

// recapReadable reports whether a recap has anything a reader can read once
// the parts meant only for the DM team are taken out. Without this a recap
// made wholly of GM-only text would announce itself to players.
func recapReadable(recapHTML string, v SinceViewer) bool {
	if !v.DMTeam {
		recapHTML = sanitize.StripSecretsHTML(recapHTML)
	}
	if strings.TrimSpace(recapTextTag.ReplaceAllString(recapHTML, "")) != "" {
		return true
	}
	return strings.Contains(strings.ToLower(recapHTML), "<img")
}

// RecapsSince lists the nights whose recap was posted after since and that
// this reader can read, newest first. A night with no readable text is left
// out.
func (r *SinceReader) RecapsSince(ctx context.Context, campaignID string, since time.Time, v SinceViewer) ([]SinceItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, recap_html, recap_posted_at FROM sessions
		 WHERE campaign_id = ? AND deleted_at IS NULL AND status <> ?
		   AND recap_posted_at > ?
		 ORDER BY recap_posted_at DESC LIMIT ?`,
		campaignID, StatusCancelled, since, sinceLimit)
	if err != nil {
		return nil, fmt.Errorf("reading recaps posted since: %w", err)
	}
	defer rows.Close()
	var out []SinceItem
	for rows.Next() {
		var it SinceItem
		var html sql.NullString
		if err := rows.Scan(&it.ID, &it.Name, &html, &it.At); err != nil {
			return nil, fmt.Errorf("scanning recap posted since: %w", err)
		}
		if html.Valid && recapReadable(html.String, v) {
			out = append(out, it)
		}
	}
	return out, rows.Err()
}

// NightsSince lists the planned nights set after since that have not happened
// yet (today is the reader's own YYYY-MM-DD), newest first. A night in the
// past is not news.
func (r *SinceReader) NightsSince(ctx context.Context, campaignID string, since time.Time, today string) ([]SinceItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, scheduled_date, created_at FROM sessions
		 WHERE campaign_id = ? AND deleted_at IS NULL AND status = ?
		   AND created_at > ? AND scheduled_date IS NOT NULL AND scheduled_date >= ?
		 ORDER BY created_at DESC LIMIT ?`,
		campaignID, StatusPlanned, since, today, sinceLimit)
	if err != nil {
		return nil, fmt.Errorf("reading nights set since: %w", err)
	}
	defer rows.Close()
	var out []SinceItem
	for rows.Next() {
		var it SinceItem
		var date sql.NullString
		if err := rows.Scan(&it.ID, &it.Name, &date, &it.At); err != nil {
			return nil, fmt.Errorf("scanning night set since: %w", err)
		}
		// A DATE arrives as RFC 3339 when parseTime is on; the day is its
		// first ten characters either way.
		if len(date.String) >= 10 {
			it.Date = date.String[:10]
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// PollsSince lists the date polls opened after since that are still open,
// newest first. Any member may see a poll, so there is no per-reader filter.
func (r *SinceReader) PollsSince(ctx context.Context, campaignID string, since time.Time) ([]SinceItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, title, created_at FROM slot_proposals
		 WHERE campaign_id = ? AND status = ? AND created_at > ?
		 ORDER BY created_at DESC LIMIT ?`,
		campaignID, ProposalOpen, since, sinceLimit)
	if err != nil {
		return nil, fmt.Errorf("reading polls opened since: %w", err)
	}
	defer rows.Close()
	var out []SinceItem
	for rows.Next() {
		var it SinceItem
		if err := rows.Scan(&it.ID, &it.Name, &it.At); err != nil {
			return nil, fmt.Errorf("scanning poll opened since: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
