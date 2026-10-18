package entities

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// FieldTypeCalendarDate is a date on the campaign's calendar ("Died on").
// Its value is "year-month-day" (month by number), optionally followed by
// "|" and, per countdown in order, the index of the length this page uses:
// "1043-3-22|1". Keeping the picks in the value keeps them under the field's
// own Director-only flag.
const FieldTypeCalendarDate = "calendar_date"

// FieldCountdown counts days from a calendar date field's date, such as how
// long a soul lingers after death. A page picks one of its lengths.
type FieldCountdown struct {
	Name    string            `json:"name"`
	Lengths []CountdownLength `json:"lengths"`
	// GMOnly keeps the countdown from players even when they see the date.
	GMOnly bool `json:"gm_only,omitempty"`
}

// CountdownLength is one length a countdown can run. The first is the usual
// one and needs no name; the others are named for when they apply.
type CountdownLength struct {
	Name string `json:"name,omitempty"`
	Days int    `json:"days"`
}

// Limits keep a field's settings and the day walks bounded.
const (
	maxCountdowns         = 6
	maxCountdownLengths   = 6
	maxCountdownDays      = 3650
	maxCountdownNameRunes = 60
)

// FieldDate is a calendar date field's parsed value.
type FieldDate struct {
	Year, Month, Day int
	// Picks is the length index per countdown; missing means the first.
	Picks []int
}

var fieldDateRe = regexp.MustCompile(`^(-?\d{1,9})-(\d{1,4})-(\d{1,4})(?:\|(\d{1,2}(?:,\d{1,2})*))?$`)

// ParseFieldDate reads a calendar date field's value; false for anything
// else, including an empty value.
func ParseFieldDate(v any) (FieldDate, bool) {
	s, ok := v.(string)
	if !ok {
		return FieldDate{}, false
	}
	m := fieldDateRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return FieldDate{}, false
	}
	var d FieldDate
	d.Year, _ = strconv.Atoi(m[1])
	d.Month, _ = strconv.Atoi(m[2])
	d.Day, _ = strconv.Atoi(m[3])
	if m[4] != "" {
		for _, p := range strings.Split(m[4], ",") {
			n, _ := strconv.Atoi(p)
			d.Picks = append(d.Picks, n)
		}
	}
	return d, true
}

// String is the stored form, dropping picks that are all the usual length.
func (d FieldDate) String() string {
	s := fmt.Sprintf("%d-%d-%d", d.Year, d.Month, d.Day)
	keep := 0
	for i, p := range d.Picks {
		if p != 0 {
			keep = i + 1
		}
	}
	if keep > 0 {
		parts := make([]string, keep)
		for i := 0; i < keep; i++ {
			parts[i] = strconv.Itoa(d.Picks[i])
		}
		s += "|" + strings.Join(parts, ",")
	}
	return s
}

// pick is the length index the page uses for countdown i.
func (d FieldDate) pick(i int) int {
	if i < len(d.Picks) {
		return d.Picks[i]
	}
	return 0
}

// FieldCalendar is the campaign calendar, as pages read dates on it. It
// belongs to the calendar plugin and is wired in internal/app; unset, a
// calendar date field shows its numbers and runs no countdowns.
type FieldCalendar interface {
	// Dates is the campaign's calendar; nil when it has none.
	Dates(ctx context.Context, campaignID string) (FieldDateBook, error)
}

// FieldDateBook reads dates on one calendar.
type FieldDateBook interface {
	Valid(year, month, day int) bool
	Label(year, month, day int) string
	// DaysSince is how many days ago the date was, negative for one still
	// to come.
	DaysSince(year, month, day int) int
	// LabelAfter is the label of the day n days after the date.
	LabelAfter(year, month, day, n int) string
	Months() []FieldDateMonth
	Today() (year, month, day int)
}

// FieldDateMonth is a month as the date picker offers it.
type FieldDateMonth struct {
	Name string `json:"name"`
	Days int    `json:"days"`
}

// SetFieldCalendar wires the campaign calendar into the fields API.
func (h *Handler) SetFieldCalendar(fc FieldCalendar) { h.fieldCalendar = fc }

// SetFieldCalendar lets field saves check a date is on the calendar.
func (s *entityService) SetFieldCalendar(fc FieldCalendar) { s.fieldCalendar = fc }

// DateShown is how a calendar date field reads beyond its label.
type DateShown struct {
	// Ago says how long ago the date was, when the field asks for it.
	Ago        string           `json:"ago,omitempty"`
	Countdowns []CountdownShown `json:"countdowns,omitempty"`
}

// CountdownShown is one countdown as of the calendar's today.
type CountdownShown struct {
	Name string `json:"name"`
	// Days is the length this page uses; Length its name, if it has one.
	Days   int    `json:"days"`
	Length string `json:"length,omitempty"`
	// State is "before" (the date is still to come), "running", "last" (its
	// last day is today) or "ended".
	State string `json:"state"`
	Left  int    `json:"left"`
	// Done is how many of its days have passed, for the ticks.
	Done int `json:"done"`
	// Last is its last day's label.
	Last         string `json:"last"`
	DirectorOnly bool   `json:"directorOnly,omitempty"`
}

// CheckDateSettings checks a calendar date field's countdowns before the
// category is saved.
func CheckDateSettings(f *FieldDefinition) error {
	if len(f.Countdowns) > maxCountdowns {
		return apperror.NewBadRequest(fmt.Sprintf("the %s field has more than %d countdowns", f.Label, maxCountdowns))
	}
	for i := range f.Countdowns {
		cd := &f.Countdowns[i]
		cd.Name = strings.TrimSpace(cd.Name)
		if cd.Name == "" {
			return apperror.NewBadRequest(fmt.Sprintf("name each countdown on the %s field", f.Label))
		}
		if utf8.RuneCountInString(cd.Name) > maxCountdownNameRunes {
			return apperror.NewBadRequest(fmt.Sprintf("a countdown's name is at most %d characters", maxCountdownNameRunes))
		}
		if len(cd.Lengths) == 0 || len(cd.Lengths) > maxCountdownLengths {
			return apperror.NewBadRequest(fmt.Sprintf("the %s countdown needs 1 to %d lengths", cd.Name, maxCountdownLengths))
		}
		for j := range cd.Lengths {
			l := &cd.Lengths[j]
			l.Name = strings.TrimSpace(l.Name)
			if utf8.RuneCountInString(l.Name) > maxCountdownNameRunes {
				return apperror.NewBadRequest(fmt.Sprintf("a length's name is at most %d characters", maxCountdownNameRunes))
			}
			if l.Days < 1 || l.Days > maxCountdownDays {
				return apperror.NewBadRequest(fmt.Sprintf("the %s countdown runs 1 to %d days", cd.Name, maxCountdownDays))
			}
			if j > 0 && l.Name == "" {
				return apperror.NewBadRequest(fmt.Sprintf("name the other lengths of the %s countdown, so a page can pick one", cd.Name))
			}
		}
	}
	return nil
}

// checkDateValue refuses a calendar date value that isn't a date this
// campaign's calendar has, or picks a length its countdown lacks. Without a
// calendar only the form is checked. Empty clears the field.
func checkDateValue(d FieldDefinition, v any, book FieldDateBook) error {
	if strings.TrimSpace(fieldText(v)) == "" {
		return nil
	}
	fd, ok := ParseFieldDate(v)
	if !ok {
		return apperror.NewBadRequest(fmt.Sprintf("%s must be a date like 1043-3-22 (year, month number, day)", d.Label))
	}
	if book != nil && !book.Valid(fd.Year, fd.Month, fd.Day) {
		return apperror.NewBadRequest(fmt.Sprintf("%s is not a day on the campaign's calendar", d.Label))
	}
	if len(fd.Picks) > len(d.Countdowns) {
		return apperror.NewBadRequest(fmt.Sprintf("%s picks lengths for countdowns it doesn't have", d.Label))
	}
	for i, p := range fd.Picks {
		if p < 0 || p >= len(d.Countdowns[i].Lengths) {
			return apperror.NewBadRequest(fmt.Sprintf("%s picks a length its %s countdown doesn't have", d.Label, d.Countdowns[i].Name))
		}
	}
	return nil
}

// shapeDate is how one calendar date value reads to the viewer, as of the
// calendar's today. Countdowns marked Director-only are left out unless
// director. Pure, so the states are pinned by table tests.
func shapeDate(d FieldDefinition, v any, book FieldDateBook, director bool) (FieldShown, bool) {
	fd, ok := ParseFieldDate(v)
	if !ok {
		return FieldShown{}, false
	}
	if book == nil || !book.Valid(fd.Year, fd.Month, fd.Day) {
		return FieldShown{Text: fmt.Sprintf("%d-%d-%d", fd.Year, fd.Month, fd.Day)}, true
	}
	since := book.DaysSince(fd.Year, fd.Month, fd.Day)
	ds := &DateShown{}
	if d.DateAgo {
		ds.Ago = daysAgo(since)
	}
	for i, cd := range d.Countdowns {
		if cd.GMOnly && !director {
			continue
		}
		p := fd.pick(i)
		if p >= len(cd.Lengths) {
			p = 0
		}
		l := cd.Lengths[p]
		cs := CountdownShown{Name: cd.Name, Days: l.Days, Length: l.Name, DirectorOnly: cd.GMOnly,
			Last: book.LabelAfter(fd.Year, fd.Month, fd.Day, l.Days)}
		cs.Left = l.Days - since
		switch {
		case since < 0:
			cs.State, cs.Left = "before", l.Days
		case cs.Left > 0:
			cs.State, cs.Done = "running", since
		case cs.Left == 0:
			cs.State, cs.Done = "last", l.Days
		default:
			cs.State, cs.Left, cs.Done = "ended", 0, l.Days
		}
		ds.Countdowns = append(ds.Countdowns, cs)
	}
	sh := FieldShown{Text: book.Label(fd.Year, fd.Month, fd.Day)}
	if ds.Ago != "" || len(ds.Countdowns) > 0 {
		sh.Date = ds
	}
	return sh, true
}

// daysAgo says how far a date is from today in words.
func daysAgo(since int) string {
	switch {
	case since == 0:
		return "today"
	case since == 1:
		return "1 day ago"
	case since > 1:
		return fmt.Sprintf("%d days ago", since)
	case since == -1:
		return "in 1 day"
	}
	return fmt.Sprintf("in %d days", -since)
}

// fieldDateBook is the campaign's calendar for date reads, nil without one
// or when it can't be read (the dates then show as numbers).
func fieldDateBook(ctx context.Context, fc FieldCalendar, campaignID string) FieldDateBook {
	if fc == nil {
		return nil
	}
	book, err := fc.Dates(ctx, campaignID)
	if err != nil {
		return nil
	}
	return book
}

// dateFieldCalendar is what the date picker needs from the calendar: its
// months and today. Nil when no field is a calendar date or there is no
// calendar.
func (h *Handler) dateFieldCalendar(ctx context.Context, campaignID string, defs []FieldDefinition) map[string]any {
	for _, d := range defs {
		if d.Type != FieldTypeCalendarDate {
			continue
		}
		book := fieldDateBook(ctx, h.fieldCalendar, campaignID)
		if book == nil {
			return nil
		}
		y, m, day := book.Today()
		return map[string]any{"months": book.Months(), "today": map[string]int{"year": y, "month": m, "day": day}}
	}
	return nil
}
