package sessions

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// monthMove is one old -> new month position.
type monthMove struct{ from, to int }

// moveList turns a remap into a sorted list of real moves. Identity entries
// and positions below 1 are dropped: month positions are 1-based, and a
// non-move needs no write. Sorting keeps the generated SQL deterministic.
func moveList(remap map[int]int) []monthMove {
	moves := make([]monthMove, 0, len(remap))
	for from, to := range remap {
		if from < 1 || to < 1 || from == to {
			continue
		}
		moves = append(moves, monthMove{from, to})
	}
	sort.Slice(moves, func(i, j int) bool { return moves[i].from < moves[j].from })
	return moves
}

// RemapMonthPositions moves the in-world month of this campaign's sessions
// dated on calendarID, so a game night follows its month when a structure
// edit reorders the calendar. remap maps old 1-based position to new, and
// holds only months that moved. Sessions in a month the edit removed are not
// in the map and keep their stored position, as the calendar does for events.
// Days are left as they are: a day past the new month's length stays stored
// rather than being silently rewritten. Returns how many sessions changed.
//
// calendarID is required: month positions mean nothing across calendars, and
// an empty id must never become "every calendar in the campaign".
func (s *sessionService) RemapMonthPositions(ctx context.Context, campaignID, calendarID string, remap map[int]int) (int, error) {
	if campaignID == "" || calendarID == "" {
		return 0, apperror.NewBadRequest("campaign and calendar are required")
	}
	if len(moveList(remap)) == 0 {
		return 0, nil
	}
	n, err := s.repo.RemapMonthPositions(ctx, campaignID, calendarID, remap)
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("remapping session months for calendar %s: %w", calendarID, err))
	}
	return int(n), nil
}

// RemapMonthPositions rewrites calendar_month for the calendar's sessions in
// one UPDATE. A CASE over the old value is what makes a swap (3->4, 4->3)
// correct: every row's new value is computed from its old one, so no row is
// moved twice, which sequential per-month updates would do. Soft-deleted and
// non-planned sessions move too: their dates are history that must keep
// pointing at the same month, and a restore must not land in the wrong one.
func (r *sessionRepository) RemapMonthPositions(ctx context.Context, campaignID, calendarID string, remap map[int]int) (int64, error) {
	moves := moveList(remap)
	if len(moves) == 0 {
		return 0, nil
	}
	query, args := buildRemapQuery(campaignID, calendarID, moves)
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("remapping session month positions: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// buildRemapQuery is split out so the statement's shape is testable without
// a database.
func buildRemapQuery(campaignID, calendarID string, moves []monthMove) (string, []any) {
	var caseSQL, inSQL strings.Builder
	args := make([]any, 0, len(moves)*3+2)
	for i, m := range moves {
		caseSQL.WriteString(" WHEN ? THEN ?")
		args = append(args, m.from, m.to)
		if i > 0 {
			inSQL.WriteString(",")
		}
		inSQL.WriteString("?")
	}
	query := "UPDATE sessions SET calendar_month = CASE calendar_month" + caseSQL.String() +
		" ELSE calendar_month END WHERE campaign_id = ? AND calendar_id = ? AND calendar_month IN (" + inSQL.String() + ")"
	args = append(args, campaignID, calendarID)
	for _, m := range moves {
		args = append(args, m.from)
	}
	return query, args
}
