// Package calendarai is the calendar's part of the AI engine (aiplug,
// ADR-071): the blocks an AI can ask to write onto the calendar (the
// calendar itself, events, a day's weather, a moon set by hand, a rolled
// night and Chronicle's generators) and the calendar lookups (the calendar,
// kinds of weather, events and weather by day). It reaches the calendar
// only through its service, and other features through the narrow
// interfaces declared here, which app wiring fills.
package calendarai

import (
	"context"
	"errors"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// Short names for the contract types this package works in.
type (
	Record = aiplug.Record
	Actor  = aiplug.Actor
	Plan   = aiplug.Plan
)

// Actions a record may carry.
const (
	ActionCreate = aiplug.ActionCreate
	ActionUpdate = aiplug.ActionUpdate
	ActionDelete = aiplug.ActionDelete
)

func badRequestf(format string, args ...any) error { return aiplug.BadRequestf(format, args...) }
func sameName(a, b string) bool                    { return aiplug.SameName(a, b) }
func quote(s string) string                        { return aiplug.Quote(s) }
func planError(err error) string                   { return aiplug.PlanError(err) }

// RollTables is the campaign's rolling tables as the calendar's blocks
// reach them: an event's table roll names one, and the names generator adds
// to one. The engine's rolling-table kind is one.
type RollTables interface {
	// FindTable is the id of the table with that name or id; "" when there
	// is none.
	FindTable(ctx context.Context, campaignID, name string) (string, error)
	// AddNames adds names to the table of that name, making it when there
	// is none.
	AddNames(ctx context.Context, campaignID string, a Actor, table string, names []string) error
}

// bodyHTML is markdown as the engine renders a record's text (its
// aiplug.Text: the paste's @[Page] links, GM-only markers refused); nil for
// an empty body.
func bodyHTML(r Record, body string) (*string, error) {
	if strings.TrimSpace(body) == "" {
		return nil, nil
	}
	if r.Text == nil {
		return nil, errors.New("calendarai: a record with a body was applied without its Text")
	}
	return r.Text.HTML(body)
}
