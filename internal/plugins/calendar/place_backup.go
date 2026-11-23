// Package calendar - place_backup.go reads and restores the weather stored
// for each place and the party's moves, for a campaign backup. Like
// backup.go, only a viewer who sees everything may call it: a hidden place's
// weather and the party's whole route are Director-only.
package calendar

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// PlaceBackupService is reached by a type assertion on CalendarService, so
// a service without place weather (or a test fake) simply backs up none.
type PlaceBackupService interface {
	// PlaceWeatherForBackup returns every stored place reading, by place
	// id, and every party move. Nil when place weather is not wired.
	PlaceWeatherForBackup(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*PlaceBackup, error)
	// RestorePlaceWeather stores one place's readings as they were, source
	// and lock included. A reading that fails the usual checks is skipped
	// and counted.
	RestorePlaceWeather(ctx context.Context, calendarID, campaignID, placeID string, days []DayWeather, v permissions.Viewer) (skipped int, err error)
	// RestorePartyMoves puts the party's moves back. A move on a day the
	// calendar does not have, or to a page the campaign does not have, is
	// skipped and counted.
	RestorePartyMoves(ctx context.Context, calendarID, campaignID string, marks []PartyMark, v permissions.Viewer) (skipped int, err error)
}

// PlaceBackup is a calendar's place weather as a backup carries it.
type PlaceBackup struct {
	Days  map[string][]DayWeather
	Marks []PartyMark
}

// PlaceWeatherForBackup: see PlaceBackupService.
func (s *calendarService) PlaceWeatherForBackup(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*PlaceBackup, error) {
	if err := requireBackupViewer(v); err != nil {
		return nil, err
	}
	if !s.placesOn() {
		return nil, nil
	}
	if _, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v); err != nil {
		return nil, err
	}
	days, err := s.placeRepo.ListAllPlaceDays(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("list place weather: %w", err)
	}
	marks, err := s.placeRepo.ListPartyMarks(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("list party moves: %w", err)
	}
	return &PlaceBackup{Days: days, Marks: marks}, nil
}

// RestorePlaceWeather: see PlaceBackupService. The place only has to be a
// page of the campaign: a backup keeps readings of a place whose climate was
// later taken off, as the original still holds them, and they show again if
// it gets one back.
func (s *calendarService) RestorePlaceWeather(ctx context.Context, calendarID, campaignID, placeID string, days []DayWeather, v permissions.Viewer) (int, error) {
	cal, err := s.placeRestoreCalendar(ctx, calendarID, campaignID, v)
	if err != nil {
		return 0, err
	}
	if _, ok := s.places.PlaceName(ctx, campaignID, placeID); !ok {
		return len(days), nil
	}
	inputs, locked, skipped := restorableDays(cal, days)
	for start := 0; start < len(inputs); start += maxDayWeatherBatch {
		if err := s.placeRepo.SetPlaceDays(ctx, calendarID, placeID, inputs[start:min(start+maxDayWeatherBatch, len(inputs))]); err != nil {
			return 0, fmt.Errorf("restore place weather: %w", err)
		}
	}
	for start := 0; start < len(locked); start += maxDayWeatherBatch {
		if _, err := s.placeRepo.LockPlaceDays(ctx, calendarID, placeID, locked[start:min(start+maxDayWeatherBatch, len(locked))], true); err != nil {
			return 0, fmt.Errorf("lock place weather: %w", err)
		}
	}
	return skipped, s.publishAfter(nil, PubWeatherChanged, campaignID, calendarID)
}

// RestorePartyMoves: see PlaceBackupService.
func (s *calendarService) RestorePartyMoves(ctx context.Context, calendarID, campaignID string, marks []PartyMark, v permissions.Viewer) (int, error) {
	cal, err := s.placeRestoreCalendar(ctx, calendarID, campaignID, v)
	if err != nil {
		return 0, err
	}
	skipped := 0
	seen := make(map[DayDate]bool, len(marks))
	for _, m := range marks {
		d := DayDate{Year: m.Year, Month: m.Month, Day: m.Day}
		if seen[d] || validateDayDate(cal, d.Year, d.Month, d.Day) != nil {
			skipped++
			continue
		}
		if m.PlaceID != "" {
			if _, ok := s.places.PlaceName(ctx, campaignID, m.PlaceID); !ok {
				skipped++
				continue
			}
		}
		seen[d] = true
		if err := s.placeRepo.SetPartyMark(ctx, calendarID, m); err != nil {
			return 0, fmt.Errorf("restore party move: %w", err)
		}
	}
	return skipped, s.publishAfter(nil, PubWeatherChanged, campaignID, calendarID)
}

// placeRestoreCalendar is a restore's calendar with its geometry, after the
// viewer and wiring checks.
func (s *calendarService) placeRestoreCalendar(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*Calendar, error) {
	if err := requireBackupViewer(v); err != nil {
		return nil, err
	}
	if !s.placesOn() {
		return nil, errNoPlaceWeather()
	}
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return nil, err
	}
	return cal, nil
}
