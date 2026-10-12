package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// eraVersionBatch bounds how many page ids one era read carries.
const eraVersionBatch = 500

// aiExportEraAdapter gives AI Export each page's era versions from the
// calendar plugin, so the export never touches that plugin's repository.
type aiExportEraAdapter struct {
	versions calendar.EraVersionService
}

// EraVersionsFor returns, per page, its versions for the eras v may see
// (oldest era first), with the calendar's current era marked. The calendar
// service already drops eras hidden from v, so none can leak into a Safe
// export.
func (a aiExportEraAdapter) EraVersionsFor(ctx context.Context, campaignID string, entityIDs []string, v permissions.Viewer) (map[string][]aiexport.EraVersion, error) {
	pe, err := a.versions.PageEras(ctx, campaignID, v)
	if err != nil || pe == nil || len(entityIDs) == 0 {
		return nil, err
	}
	out := map[string][]aiexport.EraVersion{}
	for _, era := range pe.Eras {
		for start := 0; start < len(entityIDs); start += eraVersionBatch {
			end := min(start+eraVersionBatch, len(entityIDs))
			found, err := a.versions.ForEntities(ctx, entityIDs[start:end], era.ID)
			if err != nil {
				return nil, err
			}
			for id, ver := range found {
				out[id] = append(out[id], aiexport.EraVersion{
					EraName:   era.Name,
					Current:   pe.Current != nil && pe.Current.ID == era.ID,
					Name:      ver.Name,
					EntryHTML: ver.EntryHTML,
				})
			}
		}
	}
	return out, nil
}

// aiImportEraAdapter lets AI Import find the main calendar's eras and write
// or remove a page's version of one, through the calendar plugin's service.
type aiImportEraAdapter struct {
	versions calendar.EraVersionService
}

// Eras lists the main calendar's eras as the importing viewer sees them.
func (a aiImportEraAdapter) Eras(ctx context.Context, campaignID string, v importer.Viewer) ([]importer.EraRef, error) {
	pe, err := a.versions.PageEras(ctx, campaignID, permissions.RequestViewer(v.Role, v.UserID))
	if err != nil || pe == nil {
		return nil, err
	}
	out := make([]importer.EraRef, 0, len(pe.Eras))
	for _, e := range pe.Eras {
		out = append(out, importer.EraRef{ID: e.ID, Name: e.Name})
	}
	return out, nil
}

// WriteVersion creates the version if it is missing, then sets only what the
// block carries: an empty name or text leaves that part as it was.
func (a aiImportEraAdapter) WriteVersion(ctx context.Context, campaignID, entityID string, eraID int, userID string, in importer.EraVersionWrite) error {
	if err := a.versions.Create(ctx, campaignID, entityID, eraID, userID); err != nil {
		return err
	}
	if in.KnownAs != "" {
		if err := a.versions.SetIdentity(ctx, campaignID, entityID, eraID, patch.Of(in.KnownAs), patch.Absent[string](), userID); err != nil {
			return err
		}
	}
	if in.EntryJSON != "" {
		// A nil base revision saves regardless: the import is the owner's
		// deliberate overwrite, reviewed before commit. SaveEntry sanitizes
		// the HTML again on the way in.
		if _, _, err := a.versions.SaveEntry(ctx, campaignID, entityID, eraID, in.EntryJSON, in.EntryHTML, nil, userID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteVersion removes one era's version; the usual page is untouched.
func (a aiImportEraAdapter) DeleteVersion(ctx context.Context, campaignID, entityID string, eraID int) error {
	return a.versions.Delete(ctx, campaignID, entityID, eraID)
}
