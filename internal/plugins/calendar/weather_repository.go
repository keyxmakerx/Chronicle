// Package calendar - weather_repository.go persists a calendar's weather:
// the single current reading (calendar_weather) and one reading per day
// (calendar_weather_days). Player visibility of this data is a caller
// decision, not a repository one: this file only reads and writes the
// stored state.
package calendar

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// WeatherRepository defines persistence for a calendar's current weather.
type WeatherRepository interface {
	Get(ctx context.Context, calendarID string) (*Weather, error)
	Set(ctx context.Context, calendarID string, input WeatherInput) error

	// ListDays returns the stored day readings for year, or for one month of
	// it when month > 0, in date order.
	ListDays(ctx context.Context, calendarID string, year, month int) ([]DayWeather, error)
	// SetDays upserts day readings in one transaction. A generated reading
	// never replaces a stored manual one or a locked one; a manual reading
	// replaces anything and clears the lock.
	SetDays(ctx context.Context, calendarID string, days []DayWeatherInput) error
	// ClearDays deletes the readings on the given days, whatever their source.
	ClearDays(ctx context.Context, calendarID string, dates []DayDate) error

	// LockDays sets the lock on the stored readings on the given days and
	// returns how many rows changed. A day with no reading is skipped, never
	// created.
	LockDays(ctx context.Context, calendarID string, dates []DayDate, locked bool) (int, error)

	// GetSettings returns the calendar's climate settings, or (nil, nil)
	// when none were ever stored.
	GetSettings(ctx context.Context, calendarID string) (*WeatherSettings, error)
	// SetSettings creates or replaces the calendar's climate settings.
	SetSettings(ctx context.Context, calendarID string, s WeatherSettings) error
}

// weatherRepo is the MariaDB implementation of WeatherRepository.
type weatherRepo struct {
	db *sql.DB
}

// NewWeatherRepository creates a new MariaDB-backed weather repository.
func NewWeatherRepository(db *sql.DB) WeatherRepository {
	return &weatherRepo{db: db}
}

// weatherCols is the column list for weather queries. scanWeather reads it
// positionally, see calendarCols' doc comment (repository.go) for why
// appends are safe and insertions are not.
const weatherCols = `id, calendar_id, preset_id, preset_label, icon, color,
        temperature_celsius, wind_speed_kph, wind_speed_tier,
        wind_direction, wind_direction_degrees,
        precipitation_type, precipitation_intensity,
        zone_id, zone_name, description, updated_at`

// scanWeather reads a row into a Weather struct (see foldWindPrecip).
func scanWeather(scanner interface{ Scan(...any) error }) (*Weather, error) {
	w := &Weather{}
	var windSpeedKPH sql.NullFloat64
	var windSpeedTier, windDir sql.NullString
	var windDirDeg sql.NullInt32
	var precipType sql.NullString
	var precipIntensity sql.NullFloat64

	err := scanner.Scan(&w.ID, &w.CalendarID, &w.PresetID, &w.PresetLabel, &w.Icon, &w.Color,
		&w.TemperatureCelsius, &windSpeedKPH, &windSpeedTier,
		&windDir, &windDirDeg,
		&precipType, &precipIntensity,
		&w.ZoneID, &w.ZoneName, &w.Description, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}

	w.Wind, w.Precipitation = foldWindPrecip(windSpeedKPH, windSpeedTier, windDir, windDirDeg, precipType, precipIntensity)
	return w, nil
}

// foldWindPrecip turns the nullable wind and precipitation columns into
// their sub-structs: nil when nothing was ever set for that group, rather
// than a struct of all-nil fields.
func foldWindPrecip(windSpeedKPH sql.NullFloat64, windSpeedTier, windDir sql.NullString, windDirDeg sql.NullInt32,
	precipType sql.NullString, precipIntensity sql.NullFloat64) (*Wind, *Precipitation) {
	var wind *Wind
	if windSpeedKPH.Valid || windDir.Valid || windSpeedTier.Valid || windDirDeg.Valid {
		wind = &Wind{}
		if windSpeedKPH.Valid {
			v := windSpeedKPH.Float64
			wind.SpeedKPH = &v
		}
		if windSpeedTier.Valid {
			v := windSpeedTier.String
			wind.SpeedTier = &v
		}
		if windDir.Valid {
			v := windDir.String
			wind.Direction = &v
		}
		if windDirDeg.Valid {
			v := int(windDirDeg.Int32)
			wind.DirectionDegrees = &v
		}
	}
	var precip *Precipitation
	if precipType.Valid {
		v := precipType.String
		precip = &Precipitation{Type: &v}
		if precipIntensity.Valid {
			f := precipIntensity.Float64
			precip.Intensity = &f
		}
	}
	return wind, precip
}

// Get returns the current weather state for a calendar, or nil if none set.
func (r *weatherRepo) Get(ctx context.Context, calendarID string) (*Weather, error) {
	w, err := scanWeather(r.db.QueryRowContext(ctx,
		`SELECT `+weatherCols+` FROM calendar_weather WHERE calendar_id = ?`, calendarID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return w, err
}

// Set upserts the current weather state for a calendar.
func (r *weatherRepo) Set(ctx context.Context, calendarID string, input WeatherInput) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_weather (calendar_id, preset_id, preset_label, icon, color,
		        temperature_celsius, wind_speed_kph, wind_speed_tier,
		        wind_direction, wind_direction_degrees,
		        precipitation_type, precipitation_intensity,
		        zone_id, zone_name, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		        preset_id = VALUES(preset_id), preset_label = VALUES(preset_label),
		        icon = VALUES(icon), color = VALUES(color),
		        temperature_celsius = VALUES(temperature_celsius),
		        wind_speed_kph = VALUES(wind_speed_kph), wind_speed_tier = VALUES(wind_speed_tier),
		        wind_direction = VALUES(wind_direction), wind_direction_degrees = VALUES(wind_direction_degrees),
		        precipitation_type = VALUES(precipitation_type), precipitation_intensity = VALUES(precipitation_intensity),
		        zone_id = VALUES(zone_id), zone_name = VALUES(zone_name),
		        description = VALUES(description)`,
		calendarID, input.PresetID, input.PresetLabel, input.Icon, input.Color,
		input.TemperatureCelsius, input.WindSpeedKPH, input.WindSpeedTier,
		input.WindDirection, input.WindDirectionDeg,
		input.PrecipitationType, input.PrecipitationIntensity,
		input.ZoneID, input.ZoneName, input.Description,
	)
	return err
}

// weatherDayCols is the column list for day-weather reads, in the order
// scanDayWeather reads it.
const weatherDayCols = `year, month, day, preset_id, preset_label, icon, color,
        temperature_celsius, wind_speed_kph, wind_speed_tier,
        wind_direction, wind_direction_degrees,
        precipitation_type, precipitation_intensity,
        zone_id, zone_name, description, source, locked, updated_at`

func scanDayWeather(scanner interface{ Scan(...any) error }) (DayWeather, error) {
	var w DayWeather
	var windSpeedKPH sql.NullFloat64
	var windSpeedTier, windDir sql.NullString
	var windDirDeg sql.NullInt32
	var precipType sql.NullString
	var precipIntensity sql.NullFloat64
	var locked bool
	err := scanner.Scan(&w.Year, &w.Month, &w.Day, &w.PresetID, &w.PresetLabel, &w.Icon, &w.Color,
		&w.TemperatureCelsius, &windSpeedKPH, &windSpeedTier,
		&windDir, &windDirDeg,
		&precipType, &precipIntensity,
		&w.ZoneID, &w.ZoneName, &w.Description, &w.Source, &locked, &w.UpdatedAt)
	if err != nil {
		return w, err
	}
	w.Locked = &locked
	w.Wind, w.Precipitation = foldWindPrecip(windSpeedKPH, windSpeedTier, windDir, windDirDeg, precipType, precipIntensity)
	return w, nil
}

// ListDays returns a year's (or one month's) day readings in date order.
func (r *weatherRepo) ListDays(ctx context.Context, calendarID string, year, month int) ([]DayWeather, error) {
	q := `SELECT ` + weatherDayCols + ` FROM calendar_weather_days WHERE calendar_id = ? AND year = ?`
	args := []any{calendarID, year}
	if month > 0 {
		q += ` AND month = ?`
		args = append(args, month)
	}
	q += ` ORDER BY year, month, day`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DayWeather
	for rows.Next() {
		w, err := scanDayWeather(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// dayWeatherDataCols are the reading's own columns, everything SetDays
// writes besides the key and source.
var dayWeatherDataCols = []string{"preset_id", "preset_label", "icon", "color",
	"temperature_celsius", "wind_speed_kph", "wind_speed_tier",
	"wind_direction", "wind_direction_degrees",
	"precipitation_type", "precipitation_intensity",
	"zone_id", "zone_name", "description"}

// upsertManualDaySQL replaces whatever a day holds and clears its lock,
// since painting is the owner saying what the day is. upsertGeneratedDaySQL
// leaves a manual or locked row untouched: every assignment keeps the stored
// value while source is 'manual' or locked is set, and source itself is
// assigned last because MariaDB applies ON DUPLICATE KEY assignments left to
// right, so the earlier IF()s must still see the stored source. locked is
// never assigned by a generated write, so it stays as stored.
var upsertManualDaySQL, upsertGeneratedDaySQL = buildDayUpserts()

func buildDayUpserts() (manual, generated string) {
	cols := append([]string{"calendar_id", "year", "month", "day"}, dayWeatherDataCols...)
	cols = append(cols, "source")
	insert := `INSERT INTO calendar_weather_days (` + strings.Join(cols, ", ") +
		`) VALUES (` + strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ") + `) ON DUPLICATE KEY UPDATE `
	var m, g []string
	for _, c := range append(append([]string{}, dayWeatherDataCols...), "source") {
		m = append(m, fmt.Sprintf("%s = VALUES(%s)", c, c))
		g = append(g, fmt.Sprintf("%s = IF(source = '%s' OR locked, %s, VALUES(%s))", c, WeatherSourceManual, c, c))
	}
	m = append(m, "locked = FALSE")
	return insert + strings.Join(m, ", "), insert + strings.Join(g, ", ")
}

// SetDays upserts day readings in one transaction, so a painted or
// generated range lands whole or not at all.
func (r *weatherRepo) SetDays(ctx context.Context, calendarID string, days []DayWeatherInput) error {
	if len(days) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	manual, err := tx.PrepareContext(ctx, upsertManualDaySQL)
	if err != nil {
		return err
	}
	defer func() { _ = manual.Close() }()
	generated, err := tx.PrepareContext(ctx, upsertGeneratedDaySQL)
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
		if _, err := stmt.ExecContext(ctx, calendarID, d.Year, d.Month, d.Day,
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

// ClearDays deletes the readings on the given days in one transaction.
func (r *weatherRepo) ClearDays(ctx context.Context, calendarID string, dates []DayDate) error {
	if len(dates) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	stmt, err := tx.PrepareContext(ctx,
		`DELETE FROM calendar_weather_days WHERE calendar_id = ? AND year = ? AND month = ? AND day = ?`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, d := range dates {
		if _, err := stmt.ExecContext(ctx, calendarID, d.Year, d.Month, d.Day); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LockDays flips the lock on existing readings in one transaction. The count
// is rows whose lock actually changed (MariaDB reports changed rows, not
// matched ones), so locking an already-locked day counts nothing.
func (r *weatherRepo) LockDays(ctx context.Context, calendarID string, dates []DayDate, locked bool) (int, error) {
	if len(dates) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	stmt, err := tx.PrepareContext(ctx,
		`UPDATE calendar_weather_days SET locked = ? WHERE calendar_id = ? AND year = ? AND month = ? AND day = ?`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()
	changed := 0
	for _, d := range dates {
		res, err := stmt.ExecContext(ctx, locked, calendarID, d.Year, d.Month, d.Day)
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

// GetSettings reads the calendar's climate settings; no row is (nil, nil)
// so the service decides the defaults.
func (r *weatherRepo) GetSettings(ctx context.Context, calendarID string) (*WeatherSettings, error) {
	s := &WeatherSettings{}
	var kinds sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT climate, continuity, kinds, forecast_days FROM calendar_weather_settings WHERE calendar_id = ?`, calendarID,
	).Scan(&s.Climate, &s.Continuity, &kinds, &s.ForecastDays)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// A column that does not parse reads as no kinds: the owner's list is
	// re-saved whole from the editor, and the page must still open.
	s.Kinds = []WeatherKind{}
	if kinds.Valid && kinds.String != "" {
		var stored []WeatherKind
		if json.Unmarshal([]byte(kinds.String), &stored) == nil && stored != nil {
			s.Kinds = stored
		}
	}
	return s, nil
}

// SetSettings upserts the calendar's climate settings.
func (r *weatherRepo) SetSettings(ctx context.Context, calendarID string, s WeatherSettings) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_weather_settings (calendar_id, climate, continuity, kinds, forecast_days) VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE climate = VALUES(climate), continuity = VALUES(continuity), kinds = VALUES(kinds),
		        forecast_days = VALUES(forecast_days)`,
		calendarID, s.Climate, s.Continuity, encodeWeatherKinds(s.Kinds), s.ForecastDays)
	return err
}
