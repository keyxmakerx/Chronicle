// Package calendar — era_versions.go: a page's version for one era of the
// campaign's main calendar. A version holds only what it changes (name,
// picture, text, field values); everything else is the usual page. The main
// calendar's current date picks the era every page is shown in.
//
// The entities plugin owns how a page is drawn and edited; this file owns
// which era applies and what each version stores, reached through
// EraVersionService.
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// EntityEraVersion is one page's version for one era. A nil field means "as
// the usual page"; FieldsData holds only the values that differ from it.
type EntityEraVersion struct {
	EntityID   string
	EraID      int
	Name       *string
	ImagePath  *string
	Entry      *string
	EntryHTML  *string
	EntryRev   int
	FieldsData map[string]any
	UpdatedBy  *string
	UpdatedAt  time.Time
}

// PageEras is what a page needs to know about eras for one viewer: the
// campaign's main calendar, the eras that viewer may see (oldest first), and
// the era its current date is in.
type PageEras struct {
	Calendar *Calendar
	Eras     []Era
	Current  *Era
}

// EraEntryConflict is the stored text a save collided with.
type EraEntryConflict struct {
	Rev       int
	Entry     *string
	EntryHTML *string
	At        time.Time
	ByUserID  string
}

// EraVersionService reads and writes page era versions. Every write names
// the campaign, and the era must belong to one of its calendars.
type EraVersionService interface {
	// PageEras returns the main calendar's eras for v, or nil when the
	// campaign has no main calendar v may see, or it has no eras.
	PageEras(ctx context.Context, campaignID string, v permissions.Viewer) (*PageEras, error)
	// Get returns the page's version for an era, or nil when there is none.
	Get(ctx context.Context, entityID string, eraID int) (*EntityEraVersion, error)
	// EraIDs lists the eras the page has a version for.
	EraIDs(ctx context.Context, entityID string) ([]int, error)
	// ForEntities returns the versions for one era of the given pages, keyed
	// by page id; pages without one are absent.
	ForEntities(ctx context.Context, entityIDs []string, eraID int) (map[string]*EntityEraVersion, error)
	// ForEra lists every page's version for one era, for a campaign backup.
	ForEra(ctx context.Context, eraID int) ([]*EntityEraVersion, error)
	// Create starts an empty version (the usual page in every respect). It
	// is a no-op when one exists.
	Create(ctx context.Context, campaignID, entityID string, eraID int, userID string) error
	// SetIdentity changes the version's name and picture: absent keeps,
	// null goes back to the usual page's.
	SetIdentity(ctx context.Context, campaignID, entityID string, eraID int, name, imagePath patch.Field[string], userID string) error
	// SaveEntry stores the version's text. baseRev is the revision the
	// editor loaded; when it is behind, nothing is saved and the stored text
	// comes back as a conflict. A nil baseRev saves regardless.
	SaveEntry(ctx context.Context, campaignID, entityID string, eraID int, entry, entryHTML string, baseRev *int, userID string) (int, *EraEntryConflict, error)
	// SaveFields replaces the version's changed field values; an empty map
	// means every field is as the usual page.
	SaveFields(ctx context.Context, campaignID, entityID string, eraID int, changed map[string]any, userID string) error
	// Delete removes the version; the page shows its usual self in that era.
	Delete(ctx context.Context, campaignID, entityID string, eraID int) error
}

// EraVersionRepository is the storage EraVersionService writes through.
type EraVersionRepository interface {
	EraInCampaign(ctx context.Context, eraID int, campaignID string) (bool, error)
	EntityInCampaign(ctx context.Context, entityID, campaignID string) (bool, error)
	Get(ctx context.Context, entityID string, eraID int) (*EntityEraVersion, error)
	EraIDs(ctx context.Context, entityID string) ([]int, error)
	ForEntities(ctx context.Context, entityIDs []string, eraID int) (map[string]*EntityEraVersion, error)
	ForEra(ctx context.Context, eraID int) ([]*EntityEraVersion, error)
	Create(ctx context.Context, entityID string, eraID int, userID string) error
	SetIdentity(ctx context.Context, entityID string, eraID int, name, imagePath patch.Field[string], userID string) (bool, error)
	SaveEntry(ctx context.Context, entityID string, eraID int, entry, entryHTML string, baseRev *int, userID string) (rev int, saved bool, err error)
	SaveFields(ctx context.Context, entityID string, eraID int, fieldsJSON *string, userID string) (bool, error)
	Delete(ctx context.Context, entityID string, eraID int) error
}

// mainCalendarReader is the one calendar read PageEras needs.
type mainCalendarReader interface {
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*Calendar, error)
}

type eraVersionService struct {
	repo EraVersionRepository
	cals mainCalendarReader
}

// NewEraVersionService builds the service over its repository and the
// calendar service, which already applies every era visibility rule.
func NewEraVersionService(repo EraVersionRepository, cals mainCalendarReader) EraVersionService {
	return &eraVersionService{repo: repo, cals: cals}
}

// MaxEraVersionName bounds a version's name like a page name.
const MaxEraVersionName = 200

func (s *eraVersionService) PageEras(ctx context.Context, campaignID string, v permissions.Viewer) (*PageEras, error) {
	cal, err := s.cals.GetDefaultCalendarForViewer(ctx, campaignID, v)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == 404 {
			return nil, nil
		}
		return nil, err
	}
	if cal == nil || len(cal.Eras) == 0 {
		return nil, nil
	}
	eras := SortedEras(cal.Eras)
	pe := &PageEras{Calendar: cal, Eras: eras}
	if i := EraIndexAt(eras, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay); i >= 0 {
		pe.Current = &pe.Eras[i]
	}
	return pe, nil
}

// SortedEras returns the eras oldest first, ties by their own order.
func SortedEras(in []Era) []Era {
	out := append([]Era(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.StartYear != b.StartYear || a.StartMonth != b.StartMonth || a.StartDay != b.StartDay {
			return dateLess(a.StartYear, a.StartMonth, a.StartDay, b.StartYear, b.StartMonth, b.StartDay)
		}
		return a.SortOrder < b.SortOrder
	})
	return out
}

// EraIndexAt returns the index in sorted (oldest first) of the era a day
// belongs to, or -1: the latest-starting era that has begun and has not
// explicitly ended, the rule the calendar page draws by (EraMath.at), so an
// era with no end gives way to the next one.
func EraIndexAt(sorted []Era, y, m, d int) int {
	at := -1
	for i := range sorted {
		e := &sorted[i]
		if dateLess(y, m, d, e.StartYear, e.StartMonth, e.StartDay) {
			break
		}
		if e.ContainsDate(y, m, d) {
			at = i
		}
	}
	return at
}

func (s *eraVersionService) Get(ctx context.Context, entityID string, eraID int) (*EntityEraVersion, error) {
	return s.repo.Get(ctx, entityID, eraID)
}

func (s *eraVersionService) EraIDs(ctx context.Context, entityID string) ([]int, error) {
	return s.repo.EraIDs(ctx, entityID)
}

func (s *eraVersionService) ForEntities(ctx context.Context, entityIDs []string, eraID int) (map[string]*EntityEraVersion, error) {
	if len(entityIDs) == 0 || eraID <= 0 {
		return map[string]*EntityEraVersion{}, nil
	}
	return s.repo.ForEntities(ctx, entityIDs, eraID)
}

func (s *eraVersionService) ForEra(ctx context.Context, eraID int) ([]*EntityEraVersion, error) {
	return s.repo.ForEra(ctx, eraID)
}

// checkScope refuses an era or page from another campaign with the same
// NotFound a missing one gets, so ids from elsewhere tell nothing.
func (s *eraVersionService) checkScope(ctx context.Context, campaignID, entityID string, eraID int) error {
	okEra, err := s.repo.EraInCampaign(ctx, eraID, campaignID)
	if err != nil {
		return err
	}
	okEnt, err := s.repo.EntityInCampaign(ctx, entityID, campaignID)
	if err != nil {
		return err
	}
	if !okEra || !okEnt {
		return apperror.NewNotFound("era not found")
	}
	return nil
}

var errNoVersion = apperror.NewNotFound("this page has no version for that era")

func (s *eraVersionService) Create(ctx context.Context, campaignID, entityID string, eraID int, userID string) error {
	if err := s.checkScope(ctx, campaignID, entityID, eraID); err != nil {
		return err
	}
	return s.repo.Create(ctx, entityID, eraID, userID)
}

func (s *eraVersionService) SetIdentity(ctx context.Context, campaignID, entityID string, eraID int, name, imagePath patch.Field[string], userID string) error {
	if err := s.checkScope(ctx, campaignID, entityID, eraID); err != nil {
		return err
	}
	if n, ok := name.Get(); ok {
		n = strings.TrimSpace(n)
		if n == "" {
			// An empty name means "the usual name", not a blank heading.
			name = patch.Null[string]()
		} else {
			if len([]rune(n)) > MaxEraVersionName {
				return apperror.NewValidation(fmt.Sprintf("a name can be at most %d characters", MaxEraVersionName))
			}
			name = patch.Of(n)
		}
	}
	if p, ok := imagePath.Get(); ok && strings.TrimSpace(p) == "" {
		imagePath = patch.Null[string]()
	}
	found, err := s.repo.SetIdentity(ctx, entityID, eraID, name, imagePath, userID)
	if err != nil {
		return err
	}
	if !found {
		return errNoVersion
	}
	return nil
}

func (s *eraVersionService) SaveEntry(ctx context.Context, campaignID, entityID string, eraID int, entry, entryHTML string, baseRev *int, userID string) (int, *EraEntryConflict, error) {
	if err := s.checkScope(ctx, campaignID, entityID, eraID); err != nil {
		return 0, nil, err
	}
	if strings.TrimSpace(entry) == "" {
		return 0, nil, apperror.NewBadRequest("entry content is required")
	}
	if !json.Valid([]byte(entry)) {
		return 0, nil, apperror.NewBadRequest("the page text is not valid")
	}
	// Stored HTML is shown to every reader, so it is cleaned on the way in
	// exactly like the usual page's.
	entryHTML = sanitize.HTML(entryHTML)
	rev, saved, err := s.repo.SaveEntry(ctx, entityID, eraID, entry, entryHTML, baseRev, userID)
	if err != nil {
		return 0, nil, err
	}
	if saved {
		return rev, nil, nil
	}
	cur, err := s.repo.Get(ctx, entityID, eraID)
	if err != nil {
		return 0, nil, err
	}
	if cur == nil {
		return 0, nil, errNoVersion
	}
	c := &EraEntryConflict{Rev: cur.EntryRev, Entry: cur.Entry, EntryHTML: cur.EntryHTML, At: cur.UpdatedAt}
	if cur.UpdatedBy != nil {
		c.ByUserID = *cur.UpdatedBy
	}
	return 0, c, nil
}

func (s *eraVersionService) SaveFields(ctx context.Context, campaignID, entityID string, eraID int, changed map[string]any, userID string) error {
	if err := s.checkScope(ctx, campaignID, entityID, eraID); err != nil {
		return err
	}
	var js *string
	if len(changed) > 0 {
		b, err := json.Marshal(changed)
		if err != nil {
			return apperror.NewBadRequest("field values are not valid")
		}
		v := string(b)
		js = &v
	}
	found, err := s.repo.SaveFields(ctx, entityID, eraID, js, userID)
	if err != nil {
		return err
	}
	if !found {
		return errNoVersion
	}
	return nil
}

func (s *eraVersionService) Delete(ctx context.Context, campaignID, entityID string, eraID int) error {
	if err := s.checkScope(ctx, campaignID, entityID, eraID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, entityID, eraID)
}

// EraExtent is how one era of PageEras is drawn to scale.
type EraExtent struct {
	// Days is how long the era runs. An era with no end runs to the next
	// era's start; the last one to the calendar's date, and at least a year.
	Days int
	// Here is how far through the era the calendar's date is (0 to 1), or -1
	// when the date is not in it.
	Here float64
	// Span says when it runs, in the calendar's years.
	Span string
}

// Extents returns the extent of each of pe.Eras, in order.
func (pe *PageEras) Extents() []EraExtent {
	cal := pe.Calendar
	cur := cal.CurrentDayIndex()
	out := make([]EraExtent, len(pe.Eras))
	for i := range pe.Eras {
		e := &pe.Eras[i]
		start := cal.absDayIndex(e.StartYear, e.StartMonth, e.StartDay)
		var end int
		switch {
		case e.EndYear != nil && e.EndMonth != nil && e.EndDay != nil:
			end = cal.absDayIndex(*e.EndYear, *e.EndMonth, *e.EndDay)
		case e.EndYear != nil:
			end = cal.absDayIndex(*e.EndYear+1, 1, 1) - 1
		case i+1 < len(pe.Eras):
			n := &pe.Eras[i+1]
			end = cal.absDayIndex(n.StartYear, n.StartMonth, n.StartDay) - 1
		default:
			end = max(cur, start+max(cal.YearLength(), 1))
		}
		x := EraExtent{Days: max(end-start+1, 1), Here: -1, Span: eraSpan(e)}
		if pe.Current != nil && pe.Current.ID == e.ID {
			x.Here = min(max(float64(cur-start)/float64(x.Days), 0), 1)
		}
		out[i] = x
	}
	return out
}

func eraSpan(e *Era) string {
	if e.EndYear == nil {
		return fmt.Sprintf("From year %d", e.StartYear)
	}
	if *e.EndYear == e.StartYear {
		return fmt.Sprintf("Year %d", e.StartYear)
	}
	return fmt.Sprintf("Year %d to %d", e.StartYear, *e.EndYear)
}
