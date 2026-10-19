package calendar

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// MoonMarkService is the Director's control of a manual moon. Reads need no
// method of their own: a calendar's moons carry their marks, cut to the
// shown ones for a player (finishCalendarForViewer).
type MoonMarkService interface {
	// SetMoonMode switches a moon between following its cycle and being set
	// by hand, with the Director's words for its states (nil: defaults).
	SetMoonMode(ctx context.Context, calendarID, campaignID string, moonID int, mode string, names map[string]string, v permissions.Viewer) (*Moon, error)
	// SetMoonMark sets a manual moon's state from a day on.
	SetMoonMark(ctx context.Context, calendarID, campaignID string, moonID int, mark MoonMark, v permissions.Viewer) (*Moon, error)
	// DeleteMoonMark removes the mark on a day, so the one before holds on.
	DeleteMoonMark(ctx context.Context, calendarID, campaignID string, moonID int, d DayDate, v permissions.Viewer) (*Moon, error)
}

// SetMoonMarkRepository wires the moon mark store. Without it every moon
// follows its cycle and the writes above refuse.
func (s *calendarService) SetMoonMarkRepository(r MoonMarkRepository) { s.markRepo = r }

// loadMoons loads cal's moons with their marks indexed. cal must already
// carry its Months.
func (s *calendarService) loadMoons(ctx context.Context, cal *Calendar) error {
	moons, err := s.calRepo.GetMoons(ctx, cal.ID)
	if err != nil {
		return err
	}
	cal.Moons = moons
	return s.attachMoonMarks(ctx, cal)
}

// attachMoonMarks reads the marks of cal's manual moons onto them and
// indexes them.
func (s *calendarService) attachMoonMarks(ctx context.Context, cal *Calendar) error {
	anyManual := false
	for i := range cal.Moons {
		if cal.Moons[i].Manual() {
			anyManual = true
		}
	}
	if anyManual && s.markRepo != nil {
		marks, err := s.markRepo.MarksFor(ctx, cal.ID)
		if err != nil {
			return err
		}
		for i := range cal.Moons {
			cal.Moons[i].Marks = marks[cal.Moons[i].ID]
		}
	}
	cal.indexMoonMarks()
	return nil
}

// moonTarget loads the calendar with its moons for a Director's write and
// finds the moon.
func (s *calendarService) moonTarget(ctx context.Context, calendarID, campaignID string, moonID int, v permissions.Viewer) (*Calendar, *Moon, error) {
	if s.markRepo == nil {
		return nil, nil, apperror.NewInternalMessage("hand-set moons are not available", nil)
	}
	if !v.SkipsPerUserRules() {
		return nil, nil, apperror.NewForbidden("only the campaign owner or a granted co-DM may set a moon")
	}
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, nil, err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return nil, nil, err
	}
	if err := s.loadMoons(ctx, cal); err != nil {
		return nil, nil, fmt.Errorf("load moons: %w", err)
	}
	for i := range cal.Moons {
		if cal.Moons[i].ID == moonID {
			return cal, &cal.Moons[i], nil
		}
	}
	return nil, nil, apperror.NewNotFound("moon not found in calendar")
}

// reloadMoon is the moon as the Director now sees it, after a write.
func (s *calendarService) reloadMoon(ctx context.Context, calendarID, campaignID string, moonID int, v permissions.Viewer) (*Moon, error) {
	_, m, err := s.moonTarget(ctx, calendarID, campaignID, moonID, v)
	return m, err
}

// SetMoonMode: see MoonMarkService. A real-world calendar's Moon is the
// real one, so it cannot be set by hand.
func (s *calendarService) SetMoonMode(ctx context.Context, calendarID, campaignID string, moonID int, mode string, names map[string]string, v permissions.Viewer) (*Moon, error) {
	cal, _, err := s.moonTarget(ctx, calendarID, campaignID, moonID, v)
	if err != nil {
		return nil, err
	}
	if mode != MoonCycle && mode != MoonManual {
		return nil, apperror.NewValidation(`a moon moves by "cycle" or "manual"`)
	}
	if mode == MoonManual && cal.IsRealLife() {
		return nil, apperror.NewValidation("a real-world calendar's moon follows the real Moon and cannot be set by hand")
	}
	names, err = normalizeMoonStateNames(names)
	if err != nil {
		return nil, err
	}
	found, err := s.markRepo.SetMode(ctx, calendarID, moonID, mode, names)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, apperror.NewNotFound("moon not found in calendar")
	}
	return s.reloadMoon(ctx, calendarID, campaignID, moonID, v)
}

// SetMoonMark: see MoonMarkService. The moon must be set by hand, and the
// day must be one the calendar has.
func (s *calendarService) SetMoonMark(ctx context.Context, calendarID, campaignID string, moonID int, mark MoonMark, v permissions.Viewer) (*Moon, error) {
	cal, m, err := s.moonTarget(ctx, calendarID, campaignID, moonID, v)
	if err != nil {
		return nil, err
	}
	if !m.Manual() {
		return nil, apperror.NewValidation(m.Name + " follows its cycle; set it by hand first")
	}
	if !ValidMoonState(mark.State) {
		return nil, apperror.NewValidation("a moon's state is full, half, crescent, dark or vanished")
	}
	if err := checkMoonMarkDate(cal, mark.date()); err != nil {
		return nil, err
	}
	if m.markAt(mark.date()) == nil {
		n, err := s.markRepo.CountMarks(ctx, moonID)
		if err != nil {
			return nil, err
		}
		if n >= maxMoonMarks {
			return nil, apperror.NewValidation(fmt.Sprintf("a moon keeps at most %d marks; remove old ones first", maxMoonMarks))
		}
	}
	if err := s.markRepo.PutMark(ctx, moonID, mark); err != nil {
		return nil, err
	}
	return s.reloadMoon(ctx, calendarID, campaignID, moonID, v)
}

// DeleteMoonMark: see MoonMarkService.
func (s *calendarService) DeleteMoonMark(ctx context.Context, calendarID, campaignID string, moonID int, d DayDate, v permissions.Viewer) (*Moon, error) {
	if _, _, err := s.moonTarget(ctx, calendarID, campaignID, moonID, v); err != nil {
		return nil, err
	}
	found, err := s.markRepo.DeleteMark(ctx, moonID, d)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, apperror.NewNotFound("no mark on that day")
	}
	return s.reloadMoon(ctx, calendarID, campaignID, moonID, v)
}

// markAt is the mark on exactly d, nil if none.
func (m *Moon) markAt(d DayDate) *MoonMark {
	for i := range m.Marks {
		if m.Marks[i].date() == d {
			return &m.Marks[i]
		}
	}
	return nil
}

// checkMoonMarkDate refuses a day the calendar does not have.
func checkMoonMarkDate(cal *Calendar, d DayDate) error {
	if d.Month < 1 || d.Month > len(cal.Months) || d.Day < 1 || d.Day > cal.MonthDays(d.Month-1, d.Year) {
		return apperror.NewValidation("that day is not on this calendar")
	}
	return nil
}
