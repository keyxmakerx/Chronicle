package calendar

import (
	"context"
	"fmt"
	"log/slog"
)

// oldHarptosSeasons is the season set the Harptos preset shipped before its
// numbering was corrected for the 17-month layout (festival days are months
// of their own). Calendars the wizard created from that preset stored these
// rows verbatim; the reconciler below recognises them by exact equality.
// Colors are the stored form: the preset's oklch() strings are not hex, so
// import normalisation turned every one into the fallback gray.
//
// Frozen on purpose: it must keep describing the OLD preset, never track
// presets/harptos.json, or the "is this still the untouched old value" test
// would start matching the corrected rows.
var oldHarptosSeasons = []Season{
	{Name: "Long Night", StartMonth: 12, StartDay: 1, EndMonth: 2, EndDay: 30, Color: "#808080"},
	{Name: "The Thaw", StartMonth: 3, StartDay: 1, EndMonth: 5, EndDay: 30, Color: "#808080"},
	{Name: "High Sun", StartMonth: 6, StartDay: 1, EndMonth: 8, EndDay: 30, Color: "#808080"},
	{Name: "The Fading", StartMonth: 9, StartDay: 1, EndMonth: 11, EndDay: 30, Color: "#808080"},
}

// HarptosSeasonStore is the persistence surface ReconcileHarptosSeasons
// needs. Satisfied by the calendar repository returned from
// NewCalendarRepository.
type HarptosSeasonStore interface {
	ListCalendarIDsWithSeasonNamed(ctx context.Context, name string) ([]string, error)
	GetMonths(ctx context.Context, calendarID string) ([]Month, error)
	GetSeasons(ctx context.Context, calendarID string) ([]Season, error)
	// RewriteSeasonsIfUnchanged applies replacement only if the calendar's
	// stored seasons still equal expected at write time, reporting whether it
	// did. The check and the write share one transaction so an owner's edit
	// landing between the read and the write is never overwritten.
	RewriteSeasonsIfUnchanged(ctx context.Context, calendarID string, expected, replacement []Season) (bool, error)
}

// ReconcileHarptosSeasons rewrites the seasons of calendars created from the
// Harptos preset before its season numbering was fixed, and only those: a
// calendar qualifies when it has the preset's month layout and its seasons
// still exactly equal the old preset's (no edited bound, color, description
// or weather effect, none added or removed). Anything an owner touched is
// left alone, and a repaired calendar no longer matches, so a second run
// changes nothing. An idempotent boot reconciler rather than a migration:
// whether a row is "still the old preset" is a data judgement SQL schema
// files must not make. Returns the number of calendars repaired; a failure
// on one calendar is logged and skipped so the rest still get repaired, and
// the first such error is returned for the caller to log.
func ReconcileHarptosSeasons(ctx context.Context, store HarptosSeasonStore) (int, error) {
	preset, err := LoadPreset("harptos")
	if err != nil {
		return 0, fmt.Errorf("calendar.ReconcileHarptosSeasons: load preset: %w", err)
	}
	fixed, err := fixedHarptosSeasons(preset)
	if err != nil {
		return 0, err
	}

	ids, err := store.ListCalendarIDsWithSeasonNamed(ctx, oldHarptosSeasons[0].Name)
	if err != nil {
		return 0, fmt.Errorf("calendar.ReconcileHarptosSeasons: list candidates: %w", err)
	}

	repaired := 0
	var firstErr error
	for _, id := range ids {
		ok, err := reconcileOneHarptosCalendar(ctx, store, id, len(preset.Months), fixed)
		if err != nil {
			slog.Error("calendar: harptos season reconcile failed for one calendar; continuing",
				slog.String("calendar_id", id), slog.Any("error", err))
			if firstErr == nil {
				firstErr = fmt.Errorf("calendar.ReconcileHarptosSeasons: calendar %s: %w", id, err)
			}
			continue
		}
		if ok {
			repaired++
		}
	}
	return repaired, firstErr
}

// fixedHarptosSeasons pairs each old season with its corrected bounds from
// the current preset, by name, so the corrected numbers live in exactly one
// place (the preset JSON).
func fixedHarptosSeasons(preset *ImportResult) ([]Season, error) {
	byName := make(map[string]Season, len(preset.Seasons))
	for _, s := range preset.Seasons {
		byName[s.Name] = s
	}
	out := make([]Season, 0, len(oldHarptosSeasons))
	for _, old := range oldHarptosSeasons {
		cur, ok := byName[old.Name]
		if !ok {
			return nil, fmt.Errorf("calendar.ReconcileHarptosSeasons: preset has no season %q", old.Name)
		}
		fixed := old
		fixed.StartMonth, fixed.StartDay = cur.StartMonth, cur.StartDay
		fixed.EndMonth, fixed.EndDay = cur.EndMonth, cur.EndDay
		out = append(out, fixed)
	}
	return out, nil
}

func reconcileOneHarptosCalendar(ctx context.Context, store HarptosSeasonStore, id string, presetMonths int, fixed []Season) (bool, error) {
	months, err := store.GetMonths(ctx, id)
	if err != nil {
		return false, err
	}
	// The old numbers only meant something against the 17-month layout; a
	// calendar with a different structure is someone else's calendar that
	// happens to reuse the season names.
	if len(months) != presetMonths {
		return false, nil
	}
	seasons, err := store.GetSeasons(ctx, id)
	if err != nil {
		return false, err
	}
	if !seasonsMatchPreset(seasons, oldHarptosSeasons) {
		return false, nil
	}
	return store.RewriteSeasonsIfUnchanged(ctx, id, seasons, fixedWithIDs(seasons, fixed))
}

// fixedWithIDs returns fixed re-keyed to the stored rows' ids (matched by
// name) so the write updates rows in place.
func fixedWithIDs(stored, fixed []Season) []Season {
	idByName := make(map[string]int, len(stored))
	for _, s := range stored {
		idByName[s.Name] = s.ID
	}
	out := make([]Season, len(fixed))
	for i, f := range fixed {
		f.ID = idByName[f.Name]
		out[i] = f
	}
	return out
}

// seasonsMatchPreset reports whether stored is exactly want: same count, and
// every want season present once with identical bounds and color and no
// description or weather effect. Order and row ids are ignored (the table
// has no order column).
func seasonsMatchPreset(stored, want []Season) bool {
	if len(stored) != len(want) {
		return false
	}
	byName := make(map[string]Season, len(stored))
	for _, s := range stored {
		if _, dup := byName[s.Name]; dup {
			return false
		}
		byName[s.Name] = s
	}
	empty := func(p *string) bool { return p == nil || *p == "" }
	for _, w := range want {
		s, ok := byName[w.Name]
		if !ok ||
			s.StartMonth != w.StartMonth || s.StartDay != w.StartDay ||
			s.EndMonth != w.EndMonth || s.EndDay != w.EndDay ||
			s.Color != w.Color || !empty(s.Description) || !empty(s.WeatherEffect) {
			return false
		}
	}
	return true
}

// ListCalendarIDsWithSeasonNamed returns the ids of calendars that have a
// season with the given name: the cheap candidate filter for a reconciler.
func (r *calendarRepo) ListCalendarIDsWithSeasonNamed(ctx context.Context, name string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT calendar_id FROM calendar_seasons WHERE name = ?`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RewriteSeasonsIfUnchanged updates the bounds of a calendar's seasons in
// place, but only if the rows locked inside the transaction still equal
// expected (see HarptosSeasonStore).
func (r *calendarRepo) RewriteSeasonsIfUnchanged(ctx context.Context, calendarID string, expected, replacement []Season) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		`SELECT `+seasonCols+` FROM calendar_seasons WHERE calendar_id = ? FOR UPDATE`, calendarID)
	if err != nil {
		return false, err
	}
	var current []Season
	for rows.Next() {
		s, err := scanSeason(rows)
		if err != nil {
			rows.Close()
			return false, err
		}
		current = append(current, *s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close()

	if !seasonsMatchPreset(current, expected) {
		return false, nil
	}
	for _, s := range replacement {
		if _, err := tx.ExecContext(ctx,
			`UPDATE calendar_seasons SET start_month = ?, start_day = ?, end_month = ?, end_day = ?
			 WHERE id = ? AND calendar_id = ?`,
			s.StartMonth, s.StartDay, s.EndMonth, s.EndDay, s.ID, calendarID,
		); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}
