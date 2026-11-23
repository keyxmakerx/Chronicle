package app

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// exportFieldSwitch is a category's field switch answer for the file; nil
// when it has none, so the key stays out and older readers see nothing new.
func exportFieldSwitch(sw *entities.FieldSwitch) *campaigns.ExportFieldSwitch {
	if sw == nil {
		return nil
	}
	out := &campaigns.ExportFieldSwitch{
		State:             sw.State,
		SystemID:          sw.SystemID,
		PresetSlug:        sw.PresetSlug,
		OldPresetCategory: sw.OldPresetCategory,
	}
	if sw.OldFields != nil {
		out.OldFields, _ = json.Marshal(sw.OldFields)
	}
	return out
}

// restoreFieldSwitch makes a restored category's answer the file's. Turning
// a game system on during the same import can record an answer of its own
// (an empty category takes the system's fields), but the category's fields
// came from the file, so the file's answer, or no answer, is the true one.
func (a *entityImportAdapter) restoreFieldSwitch(ctx context.Context, campaignID, userID string, typeID int, et campaigns.ExportEntityType, report *campaigns.ImportReport) {
	if a.fieldSwitch == nil {
		if et.FieldSwitch != nil {
			report.Fail("entities", "game system field answer", et.Name, "this server has no game system field switch, so the answer was left out")
		}
		return
	}
	var sw *entities.FieldSwitch
	if fs := et.FieldSwitch; fs != nil {
		sw = &entities.FieldSwitch{
			State: fs.State, SystemID: fs.SystemID, PresetSlug: fs.PresetSlug,
			OldPresetCategory: fs.OldPresetCategory,
		}
		if len(fs.OldFields) > 0 && string(fs.OldFields) != "null" {
			if err := json.Unmarshal(fs.OldFields, &sw.OldFields); err != nil {
				report.Fail("entities", "game system field answer", et.Name, "the fields to switch back to could not be read")
				return
			}
		}
	}
	if err := a.fieldSwitch.Restore(ctx, campaignID, typeID, sw, userID); err != nil {
		slog.Warn("import: restore field switch failed", slog.String("type", et.Slug), slog.Any("error", err))
		report.Fail("entities", "game system field answer", et.Name, apperror.SafeMessage(err))
	}
}

// restoreSetAside puts back the values a field switch kept aside from a page.
func (a *entityImportAdapter) restoreSetAside(ctx context.Context, campaignID string, typeID int, entityID string, e campaigns.ExportEntity, report *campaigns.ImportReport) {
	if len(e.SetAside) == 0 || string(e.SetAside) == "null" {
		return
	}
	if a.fieldSwitch == nil {
		report.Fail("entities", "kept-aside field values", e.Name, "this server has no game system field switch, so they were left out")
		return
	}
	var values map[string]any
	if err := json.Unmarshal(e.SetAside, &values); err != nil {
		report.Fail("entities", "kept-aside field values", e.Name, "the file's kept-aside values could not be read")
		return
	}
	if err := a.fieldSwitch.RestoreSetAside(ctx, campaignID, typeID, entityID, values); err != nil {
		slog.Warn("import: restore kept-aside values failed", slog.String("entity", e.Name), slog.Any("error", err))
		report.Fail("entities", "kept-aside field values", e.Name, apperror.SafeMessage(err))
	}
}
