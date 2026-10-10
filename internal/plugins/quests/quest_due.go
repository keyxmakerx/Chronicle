package quests

import (
	"context"
	"log/slog"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// maxDueYear bounds the year a due date may name. The calendar's day count is
// closed-form, but a year this far out is never a real quest and keeps the
// stored number and the arithmetic comfortably inside int range.
const maxDueYear = 1_000_000

// playersCanOpen reports whether a plain player (no per-user grant) may open
// the page. It decides the calendar event's visibility, so an error is
// returned rather than guessed at: the caller must fail closed.
func (g gate) playersCanOpen(ctx context.Context, campaignID, entityID string) (bool, error) {
	ok, err := g.entities.FilterViewable(ctx, campaignID, []string{entityID}, permissions.RolePlayer, "")
	if err != nil {
		return false, err
	}
	return ok[entityID], nil
}

// calendarOf is dir.Calendar that tolerates a plugin wired without a calendar.
func calendarOf(ctx context.Context, dir CalendarDirectory, campaignID string) (QuestCalendar, error) {
	if dir == nil {
		return nil, nil
	}
	return dir.Calendar(ctx, campaignID)
}

func dayView(cal QuestCalendar, d DueDay) DayView {
	return DayView{Year: d.Year, Month: d.Month, Day: d.Day, Label: cal.Label(d)}
}

func dueView(cal QuestCalendar, d DueDay) *DueView {
	return &DueView{DayView: dayView(cal, d), DaysLeft: cal.DaysFromToday(d)}
}

func calendarView(cal QuestCalendar) *CalendarView {
	every, offset := cal.Leap()
	months := cal.Months()
	if months == nil {
		months = []CalendarMonth{}
	}
	return &CalendarView{
		Name: cal.Name(), Today: dayView(cal, cal.Today()), Months: months,
		LeapEvery: every, LeapOffset: offset,
	}
}

// viewCalendar reads the calendar for a read path. A calendar that cannot be
// read must not take the quest sheet down with it, so a failure reads as "no
// calendar" and is logged.
func (s *questService) viewCalendar(ctx context.Context, campaignID string) QuestCalendar {
	cal, err := calendarOf(ctx, s.cal, campaignID)
	if err != nil {
		slog.Warn("quests: reading the campaign calendar failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	return cal
}

// checkDue refuses a day the calendar does not have. AbsoluteDay never
// validates, so this runs before any arithmetic or event write.
func checkDue(cal QuestCalendar, d DueDay) error {
	if d.Year < -maxDueYear || d.Year > maxDueYear {
		return errInvalid("the due year is out of range")
	}
	if !cal.Valid(d) {
		return errInvalid("the due date is not a day on the campaign calendar")
	}
	return nil
}

// eventTitle is the calendar event's name: "Due: <notice title>".
func eventTitle(title, pageName string) string {
	if title == "" {
		title = pageName
	}
	t := "Due: " + title
	if utf8.RuneCountInString(t) > apperror.MaxNameLength {
		t = string([]rune(t)[:apperror.MaxNameLength])
	}
	return t
}

// duePlan is the calendar work a save implies, carried from before the
// sheet write to after it.
type duePlan struct {
	// created is an event this save added; it is removed again if the sheet
	// write then fails, so no stray event outlives a save that did not happen.
	created string
	// remove is the event of a due date this save cleared. It goes only after
	// the sheet is saved: deleting first would lose the event if the write
	// failed while the sheet still carried the due date.
	remove string
}

// planDue applies the patch's dueDate to q and brings the calendar event in
// line. Everything that can refuse the save (an invalid date, no calendar, a
// calendar write failing) happens here, before the sheet is written, so a
// refused save changes neither the sheet nor the calendar.
func (s *questService) planDue(ctx context.Context, campaignID string, ent EntityInfo, v Viewer, q *Quest, p QuestPatch, prevTitle string, prevHidden bool) (duePlan, error) {
	var plan duePlan
	var cal QuestCalendar
	switch {
	case p.DueDate.Present():
		d, ok := p.DueDate.Get()
		if !ok { // explicit null clears
			plan.remove = q.DueEventID
			q.DueDate, q.DueEventID = nil, ""
			return plan, nil
		}
		c, err := calendarOf(ctx, s.cal, campaignID)
		if err != nil {
			return plan, apperrorInternal(err)
		}
		if c == nil {
			return plan, errInvalid("this campaign has no calendar to set a due date on")
		}
		if err := checkDue(c, d); err != nil {
			return plan, err
		}
		q.DueDate, cal = &d, c
	case q.DueDate != nil && (q.Notice.Title != prevTitle || q.Layout.Notice.Hidden != prevHidden):
		// The event carries the notice title, so a rename follows it, and
		// hiding or showing the notice hides or shows the event with it.
		c, err := calendarOf(ctx, s.cal, campaignID)
		if err != nil {
			return plan, apperrorInternal(err)
		}
		cal = c // nil when the calendar is gone: the sheet still saves
	}
	if cal == nil {
		return plan, nil
	}
	open, err := s.playersCanOpen(ctx, campaignID, ent.ID)
	if err != nil {
		return plan, apperrorInternal(err)
	}
	id, err := s.cal.SaveDueEvent(ctx, campaignID, cal, q.DueEventID, DueEvent{
		EntityID: ent.ID, Title: eventTitle(q.Notice.Title, ent.Name), Day: *q.DueDate,
		DMOnly: dueEventDMOnly(open, *q), CreatedBy: v.UserID,
	})
	if err != nil {
		return plan, apperrorInternal(err)
	}
	if id != q.DueEventID {
		plan.created = id
	}
	q.DueEventID = id
	return plan, nil
}

// undoDue takes back an event a failed save created. Best effort: the sheet
// write already failed, and the stray event is only a calendar entry.
func (s *questService) undoDue(ctx context.Context, campaignID string, plan duePlan) {
	if plan.created == "" {
		return
	}
	ctx = context.WithoutCancel(ctx)
	cal, err := calendarOf(ctx, s.cal, campaignID)
	if err == nil && cal != nil {
		err = s.cal.DeleteDueEvent(ctx, campaignID, cal, plan.created)
	}
	if err != nil {
		slog.Warn("quests: removing the due event of a failed save failed", slog.String("campaign_id", campaignID), slog.String("event_id", plan.created), slog.Any("error", err))
	}
}

// finishDue removes the event of a cleared due date after the sheet is
// saved. The sheet is already correct by then, so a failure is logged, not
// returned: the DM's change did happen.
func (s *questService) finishDue(ctx context.Context, campaignID string, plan duePlan) {
	if plan.remove == "" {
		return
	}
	ctx = context.WithoutCancel(ctx)
	if err := s.deleteDueEvent(ctx, campaignID, plan.remove); err != nil {
		slog.Warn("quests: removing a cleared due date's event failed", slog.String("campaign_id", campaignID), slog.String("event_id", plan.remove), slog.Any("error", err))
	}
}

func (s *questService) deleteDueEvent(ctx context.Context, campaignID, eventID string) error {
	cal, err := calendarOf(ctx, s.cal, campaignID)
	if err != nil || cal == nil {
		return err
	}
	return s.cal.DeleteDueEvent(ctx, campaignID, cal, eventID)
}

// dueEventDMOnly reports whether the due event is for the DM only: players
// cannot open the page, or the DM hid the notice the due date belongs to (the
// player view of the sheet drops the due date with the notice).
func dueEventDMOnly(playersCanOpen bool, q Quest) bool {
	return !playersCanOpen || q.Layout.Notice.Hidden
}

// dueEventOf reads the page's sheet and the id of its due event; the id is ""
// when the page has no sheet, no due date or no event. One repository read.
func (s *questService) dueEventOf(ctx context.Context, campaignID, entityID string) (string, Quest, error) {
	var q Quest
	data, _, found, err := s.repo.Get(ctx, campaignID, entityID)
	if err != nil {
		return "", q, apperrorInternal(err)
	}
	if !found {
		return "", q, nil
	}
	if err := jsonUnmarshal(data, &q); err != nil {
		return "", q, apperrorInternal(err)
	}
	if q.DueDate == nil {
		return "", q, nil
	}
	return q.DueEventID, q, nil
}

func (s *questService) SyncDueEvent(ctx context.Context, campaignID, entityID string) error {
	id, q, err := s.dueEventOf(ctx, campaignID, entityID)
	if err != nil || id == "" {
		return err
	}
	cal, err := calendarOf(ctx, s.cal, campaignID)
	if err != nil || cal == nil {
		return err
	}
	open, err := s.playersCanOpen(ctx, campaignID, entityID)
	if err != nil {
		return apperrorInternal(err)
	}
	if err := s.cal.SetDueEventVisibility(ctx, campaignID, cal, id, dueEventDMOnly(open, q)); err != nil {
		return apperrorInternal(err)
	}
	return nil
}

func (s *questService) RemoveDueEvent(ctx context.Context, campaignID, entityID string) error {
	id, _, err := s.dueEventOf(ctx, campaignID, entityID)
	if err != nil || id == "" {
		return err
	}
	if err := s.deleteDueEvent(ctx, campaignID, id); err != nil {
		return apperrorInternal(err)
	}
	return nil
}
