// Package calendar - place_weather_repository.go persists the weather of
// each place that has a climate of its own (calendar_place_weather_days) and
// the days the party arrived somewhere (calendar_party_places). Like
// weather_repository.go it only reads and writes stored state; who may see
// which place is the service's decision.
package calendar

import (
	"context"
	"database/sql"
	"strings"
)

// PartyMark says the party is at PlaceID from this day until the next mark.
// An empty PlaceID means nowhere with a climate of its own.
type PartyMark struct {
	Year    int    `json:"year"`
	Month   int    `json:"month"`
	Day     int    `json:"day"`
	PlaceID string `json:"place_id,omitempty"`
}

// PlaceWeatherRepository stores per-place day readings and the party's marks.
type PlaceWeatherRepository interface {
	// ListPlaceDays returns the readings of the given places for a year, or
	// one month of it when month > 0, keyed by place.
	ListPlaceDays(ctx context.Context, calendarID string, placeIDs []string, year, month int) (map[string][]DayWeather, error)
	// SetPlaceDays upserts one place's readings in one transaction, with the
	// same manual and lock rules as WeatherRepository.SetDays.
	SetPlaceDays(ctx context.Context, calendarID, placeID string, days []DayWeatherInput) error
	// ClearPlaceDays deletes one place's readings on the given days.
	ClearPlaceDays(ctx context.Context, calendarID, placeID string, dates []DayDate) error
	// LockPlaceDays sets the lock on one place's stored readings and returns
	// how many changed; a day with no reading is skipped.
	LockPlaceDays(ctx context.Context, calendarID, placeID string, dates []DayDate, locked bool) (int, error)

	// ListPartyMarks returns every mark of the calendar in date order.
	ListPartyMarks(ctx context.Context, calendarID string) ([]PartyMark, error)
	// SetPartyMark creates or replaces the mark on one day.
	SetPartyMark(ctx context.Context, calendarID string, m PartyMark) error
	// ClearPartyMark deletes the mark on one day, if any.
	ClearPartyMark(ctx context.Context, calendarID string, d DayDate) error
}

type placeWeatherRepo struct{ db *sql.DB }

// NewPlaceWeatherRepository creates the MariaDB-backed place weather store.
func NewPlaceWeatherRepository(db *sql.DB) PlaceWeatherRepository {
	return &placeWeatherRepo{db: db}
}

var upsertManualPlaceDaySQL, upsertGeneratedPlaceDaySQL = buildDayUpserts("calendar_place_weather_days",
	"calendar_id", "place_id", "year", "month", "day")

func (r *placeWeatherRepo) ListPlaceDays(ctx context.Context, calendarID string, placeIDs []string, year, month int) (map[string][]DayWeather, error) {
	out := map[string][]DayWeather{}
	if len(placeIDs) == 0 {
		return out, nil
	}
	q := `SELECT place_id, ` + weatherDayCols + ` FROM calendar_place_weather_days
	      WHERE calendar_id = ? AND year = ? AND place_id IN (` + strings.TrimSuffix(strings.Repeat("?, ", len(placeIDs)), ", ") + `)`
	args := []any{calendarID, year}
	for _, id := range placeIDs {
		args = append(args, id)
	}
	if month > 0 {
		q += ` AND month = ?`
		args = append(args, month)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY place_id, year, month, day`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var place string
		w, err := scanDayWeather(prefixScanner{rows, &place})
		if err != nil {
			return nil, err
		}
		out[place] = append(out[place], w)
	}
	return out, rows.Err()
}

// prefixScanner reads one leading column into first and hands the rest to
// the shared day scanner, so the two tables keep one column order.
type prefixScanner struct {
	rows  *sql.Rows
	first *string
}

func (p prefixScanner) Scan(dest ...any) error {
	return p.rows.Scan(append([]any{p.first}, dest...)...)
}

func (r *placeWeatherRepo) SetPlaceDays(ctx context.Context, calendarID, placeID string, days []DayWeatherInput) error {
	if len(days) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	manual, err := tx.PrepareContext(ctx, upsertManualPlaceDaySQL)
	if err != nil {
		return err
	}
	defer func() { _ = manual.Close() }()
	generated, err := tx.PrepareContext(ctx, upsertGeneratedPlaceDaySQL)
	if err != nil {
		return err
	}
	defer func() { _ = generated.Close() }()
	for _, d := range days {
		in := d.WeatherInput
		stmt, source := manual, WeatherSourceManual
		if d.Source == WeatherSourceGenerated {
			stmt, source = generated, WeatherSourceGenerated
		}
		if _, err := stmt.ExecContext(ctx, calendarID, placeID, d.Year, d.Month, d.Day,
			in.PresetID, in.PresetLabel, in.Icon, in.Color,
			in.TemperatureCelsius, in.WindSpeedKPH, in.WindSpeedTier,
			in.WindDirection, in.WindDirectionDeg,
			in.PrecipitationType, in.PrecipitationIntensity,
			in.ZoneID, in.ZoneName, in.Description, source); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *placeWeatherRepo) ClearPlaceDays(ctx context.Context, calendarID, placeID string, dates []DayDate) error {
	if len(dates) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	stmt, err := tx.PrepareContext(ctx,
		`DELETE FROM calendar_place_weather_days WHERE calendar_id = ? AND place_id = ? AND year = ? AND month = ? AND day = ?`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, d := range dates {
		if _, err := stmt.ExecContext(ctx, calendarID, placeID, d.Year, d.Month, d.Day); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *placeWeatherRepo) LockPlaceDays(ctx context.Context, calendarID, placeID string, dates []DayDate, locked bool) (int, error) {
	if len(dates) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	stmt, err := tx.PrepareContext(ctx,
		`UPDATE calendar_place_weather_days SET locked = ? WHERE calendar_id = ? AND place_id = ? AND year = ? AND month = ? AND day = ?`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()
	changed := 0
	for _, d := range dates {
		res, err := stmt.ExecContext(ctx, locked, calendarID, placeID, d.Year, d.Month, d.Day)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		changed += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}

func (r *placeWeatherRepo) ListPartyMarks(ctx context.Context, calendarID string) ([]PartyMark, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT year, month, day, place_id FROM calendar_party_places WHERE calendar_id = ? ORDER BY year, month, day`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PartyMark
	for rows.Next() {
		var m PartyMark
		var place sql.NullString
		if err := rows.Scan(&m.Year, &m.Month, &m.Day, &place); err != nil {
			return nil, err
		}
		m.PlaceID = place.String
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *placeWeatherRepo) SetPartyMark(ctx context.Context, calendarID string, m PartyMark) error {
	var place any
	if m.PlaceID != "" {
		place = m.PlaceID
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_party_places (calendar_id, year, month, day, place_id) VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE place_id = VALUES(place_id)`,
		calendarID, m.Year, m.Month, m.Day, place)
	return err
}

func (r *placeWeatherRepo) ClearPartyMark(ctx context.Context, calendarID string, d DayDate) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM calendar_party_places WHERE calendar_id = ? AND year = ? AND month = ? AND day = ?`,
		calendarID, d.Year, d.Month, d.Day)
	return err
}
