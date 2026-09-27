// Package calendar - weather_repository.go persists the single current-
// weather row per calendar (calendar_weather). Player visibility of this
// data (today only, unless the calendar's ForecastsEnabled is on) is a
// caller decision, not a repository one: this file only reads and writes
// the stored state.
package calendar

import (
	"context"
	"database/sql"
)

// WeatherRepository defines persistence for a calendar's current weather.
type WeatherRepository interface {
	Get(ctx context.Context, calendarID string) (*Weather, error)
	Set(ctx context.Context, calendarID string, input WeatherInput) error
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

// scanWeather reads a row into a Weather struct, folding the nullable wind
// and precipitation columns into their sub-structs (nil when nothing was
// ever set for that group, rather than a struct of all-nil fields).
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

	if windSpeedKPH.Valid || windDir.Valid || windSpeedTier.Valid || windDirDeg.Valid {
		wind := &Wind{}
		if windSpeedKPH.Valid {
			v := windSpeedKPH.Float64
			wind.SpeedKPH = &v
		}
		if windSpeedTier.Valid {
			wind.SpeedTier = &windSpeedTier.String
		}
		if windDir.Valid {
			wind.Direction = &windDir.String
		}
		if windDirDeg.Valid {
			v := int(windDirDeg.Int32)
			wind.DirectionDegrees = &v
		}
		w.Wind = wind
	}

	if precipType.Valid {
		p := &Precipitation{Type: &precipType.String}
		if precipIntensity.Valid {
			p.Intensity = &precipIntensity.Float64
		}
		w.Precipitation = p
	}

	return w, nil
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
