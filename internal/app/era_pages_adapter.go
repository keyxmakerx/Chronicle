package app

import (
	"context"
	"math"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// eraPagesAdapter gives the entities plugin pages by era from the calendar
// plugin, which owns the eras and what each page's version stores.
type eraPagesAdapter struct {
	versions calendar.EraVersionService
}

// minEraWeight keeps a short era wide enough to click on the ribbon.
const minEraWeight = 0.08

func (a eraPagesAdapter) PageEras(ctx context.Context, campaignID string, v permissions.Viewer) (*entities.PageEraSet, error) {
	pe, err := a.versions.PageEras(ctx, campaignID, v)
	if err != nil || pe == nil {
		return nil, err
	}
	ext := pe.Extents()
	// Square-root lengths, so a long first age does not squeeze the rest out
	// of sight; then a floor, so none is too thin to pick.
	total := 0.0
	roots := make([]float64, len(ext))
	for i, x := range ext {
		roots[i] = math.Sqrt(float64(x.Days))
		total += roots[i]
	}
	set := &entities.PageEraSet{CalendarName: pe.Calendar.Name}
	if pe.Current != nil {
		set.CurrentID = pe.Current.ID
	}
	for i := range pe.Eras {
		e := &pe.Eras[i]
		w := 1.0
		if total > 0 {
			w = max(roots[i]/total, minEraWeight)
		}
		pg := entities.PageEra{
			ID: e.ID, Name: e.Name, Color: e.Color, Span: ext[i].Span,
			Hidden: pe.Calendar.EraIsSecret(e), Weight: w, Here: ext[i].Here,
		}
		if e.Color2 != nil {
			pg.Color2 = *e.Color2
		}
		set.Eras = append(set.Eras, pg)
	}
	return set, nil
}

func toPageEraVersion(v *calendar.EntityEraVersion) *entities.PageEraVersion {
	if v == nil {
		return nil
	}
	out := &entities.PageEraVersion{
		Name: v.Name, ImagePath: v.ImagePath, Entry: v.Entry, EntryHTML: v.EntryHTML,
		EntryRev: v.EntryRev, FieldsData: v.FieldsData, UpdatedAt: v.UpdatedAt,
	}
	if v.UpdatedBy != nil {
		out.UpdatedBy = *v.UpdatedBy
	}
	return out
}

func (a eraPagesAdapter) Version(ctx context.Context, entityID string, eraID int) (*entities.PageEraVersion, error) {
	v, err := a.versions.Get(ctx, entityID, eraID)
	return toPageEraVersion(v), err
}

func (a eraPagesAdapter) VersionEraIDs(ctx context.Context, entityID string) ([]int, error) {
	return a.versions.EraIDs(ctx, entityID)
}

func (a eraPagesAdapter) VersionsFor(ctx context.Context, entityIDs []string, eraID int) (map[string]*entities.PageEraVersion, error) {
	m, err := a.versions.ForEntities(ctx, entityIDs, eraID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*entities.PageEraVersion, len(m))
	for id, v := range m {
		out[id] = toPageEraVersion(v)
	}
	return out, nil
}

func (a eraPagesAdapter) CreateVersion(ctx context.Context, campaignID, entityID string, eraID int, userID string) error {
	return a.versions.Create(ctx, campaignID, entityID, eraID, userID)
}

func (a eraPagesAdapter) SetVersionIdentity(ctx context.Context, campaignID, entityID string, eraID int, name, imagePath patch.Field[string], userID string) error {
	return a.versions.SetIdentity(ctx, campaignID, entityID, eraID, name, imagePath, userID)
}

func (a eraPagesAdapter) SaveVersionEntry(ctx context.Context, campaignID, entityID string, eraID int, entry, entryHTML string, baseRev *int, userID string) (int, *entities.EraEntryClash, error) {
	rev, c, err := a.versions.SaveEntry(ctx, campaignID, entityID, eraID, entry, entryHTML, baseRev, userID)
	if err != nil || c == nil {
		return rev, nil, err
	}
	return 0, &entities.EraEntryClash{Rev: c.Rev, Entry: c.Entry, EntryHTML: c.EntryHTML, ByUserID: c.ByUserID, At: c.At}, nil
}

func (a eraPagesAdapter) SaveVersionFields(ctx context.Context, campaignID, entityID string, eraID int, changed map[string]any, userID string) error {
	return a.versions.SaveFields(ctx, campaignID, entityID, eraID, changed, userID)
}

func (a eraPagesAdapter) DeleteVersion(ctx context.Context, campaignID, entityID string, eraID int) error {
	return a.versions.Delete(ctx, campaignID, entityID, eraID)
}

// pageMoonsAdapter gives the entities plugin the moons whose lore page a
// page is.
type pageMoonsAdapter struct {
	calendars calendar.CalendarService
}

func (a pageMoonsAdapter) MoonsForPage(ctx context.Context, campaignID, entityID string, v permissions.Viewer) ([]entities.PageMoon, error) {
	refs, err := a.calendars.MoonsForEntity(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	out := make([]entities.PageMoon, 0, len(refs))
	for _, r := range refs {
		out = append(out, entities.PageMoon{Name: r.MoonName, CalendarID: r.CalendarID, CalendarName: r.CalendarName, Phase: r.PhaseName})
	}
	return out, nil
}
