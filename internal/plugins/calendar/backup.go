// Package calendar - backup.go reads and restores the calendar data a
// campaign backup carries beyond the calendar's own row and sub-resources:
// each day's weather, a hand-set moon's mode and marks, and event rolls with
// their stored nights. Only a viewer who sees everything (the backup's
// system viewer, or the Director) may call it, since all three hold
// Director-only state.
package calendar

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// BackupService is reached by a type assertion on CalendarService, the way
// MoonMarkService and EventRollService are, so callers that don't back up
// calendars never see it.
type BackupService interface {
	// DayWeatherForBackup returns every stored day reading, lock included.
	DayWeatherForBackup(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) ([]DayWeather, error)
	// RestoreDayWeather stores readings as they were, source and lock
	// included. A reading for a day the calendar does not have, or one that
	// fails the usual checks, is skipped and counted.
	RestoreDayWeather(ctx context.Context, calendarID, campaignID string, days []DayWeather, v permissions.Viewer) (skipped int, err error)
	// RestoreMoonHand puts a moon back to being set by hand, with its words
	// and marks. Marks on days the calendar does not have are skipped and
	// counted.
	RestoreMoonHand(ctx context.Context, calendarID, campaignID string, moonID int, hand MoonHand, v permissions.Viewer) (skipped int, err error)
	// EventRollsForBackup returns, by event id, the roll and stored nights of
	// every event in the calendar that has one; none where rolls are not
	// wired.
	EventRollsForBackup(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (map[string]EventRollBackup, error)
	// RestoreEventRoll sets an event's roll and its stored nights. A table
	// roll is matched to this campaign's table by name; with no such table
	// the roll is left off and dropped is true.
	RestoreEventRoll(ctx context.Context, eventID, calendarID, campaignID string, roll EventRollBackup, v permissions.Viewer) (dropped bool, err error)
}

// MoonHand is what a hand-set moon carries beyond its row: the Director's
// words for its states, and its marks.
type MoonHand struct {
	StateNames map[string]string
	Marks      []MoonMark
}

// EventRollBackup is an event's roll and every night stored for it.
// TableName names a table roll's table, since its id means nothing in
// another campaign.
type EventRollBackup struct {
	Config    RollConfig
	TableName string
	Nights    []RollNight
}

// RollTableFinder finds a campaign's rolling table by name. The rolling
// table adapter provides it beside RollTableSource; without it a restored
// table roll is always dropped.
type RollTableFinder interface {
	RollTableIDByName(ctx context.Context, campaignID, name string) (string, error)
}

// allDays bound a whole-history read of nights; day readings and nights are
// held well inside them by validateDayDate's year range.
var (
	firstDay = DayDate{Year: -maxDayWeatherYear - 1}
	lastDay  = DayDate{Year: maxDayWeatherYear + 1}
)

func requireBackupViewer(v permissions.Viewer) error {
	if !v.SkipsPerUserRules() {
		return apperror.NewForbidden("only the campaign owner or a granted co-DM may back up a calendar")
	}
	return nil
}

// DayWeatherForBackup: see BackupService.
func (s *calendarService) DayWeatherForBackup(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) ([]DayWeather, error) {
	if err := requireBackupViewer(v); err != nil {
		return nil, err
	}
	if _, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v); err != nil {
		return nil, err
	}
	days, err := s.weatherRepo.ListAllDays(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("list day weather: %w", err)
	}
	return days, nil
}

// RestoreDayWeather: see BackupService. Readings go in by source, then the
// locked ones are locked, each in batches the size a paint may be.
func (s *calendarService) RestoreDayWeather(ctx context.Context, calendarID, campaignID string, days []DayWeather, v permissions.Viewer) (int, error) {
	if err := requireBackupViewer(v); err != nil {
		return 0, err
	}
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return 0, err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return 0, err
	}
	skipped := 0
	seen := make(map[DayDate]bool, len(days))
	var inputs []DayWeatherInput
	var locked []DayDate
	for _, d := range days {
		key := DayDate{Year: d.Year, Month: d.Month, Day: d.Day}
		in := DayWeatherInput{Year: d.Year, Month: d.Month, Day: d.Day, Source: d.Source, WeatherInput: d.input()}
		if seen[key] || validateDayDate(cal, d.Year, d.Month, d.Day) != nil || validateWeatherInput(in.WeatherInput) != nil ||
			(d.Source != WeatherSourceManual && d.Source != WeatherSourceGenerated) {
			skipped++
			continue
		}
		seen[key] = true
		inputs = append(inputs, in)
		if d.Locked != nil && *d.Locked {
			locked = append(locked, key)
		}
	}
	for start := 0; start < len(inputs); start += maxDayWeatherBatch {
		if err := s.weatherRepo.SetDays(ctx, calendarID, inputs[start:min(start+maxDayWeatherBatch, len(inputs))]); err != nil {
			return 0, fmt.Errorf("restore day weather: %w", err)
		}
	}
	for start := 0; start < len(locked); start += maxDayWeatherBatch {
		if _, err := s.weatherRepo.LockDays(ctx, calendarID, locked[start:min(start+maxDayWeatherBatch, len(locked))], true); err != nil {
			return 0, fmt.Errorf("lock day weather: %w", err)
		}
	}
	return skipped, s.publishAfter(nil, PubWeatherChanged, campaignID, calendarID)
}

// input is the reading as the flat fields a write takes.
func (d DayWeather) input() WeatherInput {
	in := WeatherInput{
		PresetID: d.PresetID, PresetLabel: d.PresetLabel, Icon: d.Icon, Color: d.Color,
		TemperatureCelsius: d.TemperatureCelsius, ZoneID: d.ZoneID, ZoneName: d.ZoneName,
		Description: d.Description,
	}
	if d.Wind != nil {
		in.WindSpeedKPH, in.WindSpeedTier = d.Wind.SpeedKPH, d.Wind.SpeedTier
		in.WindDirection, in.WindDirectionDeg = d.Wind.Direction, d.Wind.DirectionDegrees
	}
	if d.Precipitation != nil {
		in.PrecipitationType, in.PrecipitationIntensity = d.Precipitation.Type, d.Precipitation.Intensity
	}
	return in
}

// RestoreMoonHand: see BackupService. It goes through the same checks as the
// Director's own writes (SetMoonMode, SetMoonMark), loading the calendar once.
func (s *calendarService) RestoreMoonHand(ctx context.Context, calendarID, campaignID string, moonID int, hand MoonHand, v permissions.Viewer) (int, error) {
	if err := requireBackupViewer(v); err != nil {
		return 0, err
	}
	if _, err := s.SetMoonMode(ctx, calendarID, campaignID, moonID, MoonManual, hand.StateNames, v); err != nil {
		return 0, err
	}
	cal, _, err := s.moonTarget(ctx, calendarID, campaignID, moonID, v)
	if err != nil {
		return 0, err
	}
	skipped := 0
	seen := map[DayDate]bool{}
	for _, mark := range hand.Marks {
		d := mark.date()
		if seen[d] || len(seen) >= maxMoonMarks || !ValidMoonState(mark.State) || checkMoonMarkDate(cal, d) != nil {
			skipped++
			continue
		}
		seen[d] = true
		if err := s.markRepo.PutMark(ctx, moonID, MoonMark{Year: d.Year, Month: d.Month, Day: d.Day, State: mark.State, Shown: mark.Shown}); err != nil {
			return 0, fmt.Errorf("restore moon mark: %w", err)
		}
	}
	return skipped, nil
}

// EventRollsForBackup: see BackupService.
func (s *calendarService) EventRollsForBackup(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (map[string]EventRollBackup, error) {
	if err := requireBackupViewer(v); err != nil {
		return nil, err
	}
	if s.rollRepo == nil {
		return nil, nil
	}
	if _, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v); err != nil {
		return nil, err
	}
	events, err := s.eventRepo.ListAllEvents(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	ids := make([]string, len(events))
	for i := range events {
		ids[i] = events[i].ID
	}
	configs, err := s.rollRepo.ConfigsFor(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list rolls: %w", err)
	}
	out := make(map[string]EventRollBackup, len(configs))
	for eventID, cfg := range configs {
		if cfg == nil {
			continue
		}
		nights, err := s.rollRepo.ListNights(ctx, eventID, firstDay, lastDay)
		if err != nil {
			return nil, fmt.Errorf("list roll nights: %w", err)
		}
		b := EventRollBackup{Config: *cfg, Nights: nights}
		if cfg.Source == RollSourceTable && s.rollTables != nil {
			if t, err := s.rollTables.RollTable(ctx, campaignID, cfg.TableID); err == nil && t != nil {
				b.TableName = t.Name
			}
		}
		out[eventID] = b
	}
	return out, nil
}

// RestoreEventRoll: see BackupService. Nights are written as stored, then
// their locks and "tell players" choices are put back, since a write resets
// both.
func (s *calendarService) RestoreEventRoll(ctx context.Context, eventID, calendarID, campaignID string, roll EventRollBackup, v permissions.Viewer) (bool, error) {
	if err := requireBackupViewer(v); err != nil {
		return false, err
	}
	if err := s.rollsReady(); err != nil {
		return false, err
	}
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return false, err
	}
	if _, err := s.eventInCalendarForViewer(ctx, eventID, calendarID, v); err != nil {
		return false, err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return false, err
	}
	cfg := roll.Config
	if cfg.Source == RollSourceTable {
		finder, ok := s.rollTables.(RollTableFinder)
		if !ok || roll.TableName == "" {
			return true, nil
		}
		id, err := finder.RollTableIDByName(ctx, campaignID, roll.TableName)
		if err != nil {
			return false, fmt.Errorf("find rolling table: %w", err)
		}
		if id == "" {
			return true, nil
		}
		cfg.TableID = id
	}
	if err := cfg.Normalize(); err != nil {
		return false, err
	}
	if err := s.rollRepo.SetConfig(ctx, eventID, cfg); err != nil {
		return false, fmt.Errorf("restore roll: %w", err)
	}

	var nights []RollNight
	var locked []DayDate
	var told []RollNight
	seen := map[DayDate]bool{}
	for _, n := range roll.Nights {
		d := n.date()
		if seen[d] || validateDayDate(cal, d.Year, d.Month, d.Day) != nil ||
			(n.Source != RollNightAhead && n.Source != RollNightOnDay && n.Source != RollNightManual) {
			continue
		}
		seen[d] = true
		nights = append(nights, n)
		if n.Locked {
			locked = append(locked, d)
		}
		if n.Told != nil {
			told = append(told, n)
		}
	}
	if err := s.rollRepo.PutNights(ctx, eventID, nights, false); err != nil {
		return false, fmt.Errorf("restore roll nights: %w", err)
	}
	if _, err := s.rollRepo.LockNights(ctx, eventID, locked, true); err != nil {
		return false, fmt.Errorf("lock roll nights: %w", err)
	}
	for _, n := range told {
		if _, err := s.rollRepo.SetTold(ctx, eventID, n.date(), n.Told); err != nil {
			return false, fmt.Errorf("restore roll night: %w", err)
		}
	}
	return false, nil
}
