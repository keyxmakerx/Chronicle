// Package app — export_adapters_tables.go carries rolling tables, quest sheets
// and crafting recipes through the campaign export and import.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
)

// --- Rolling tables ---

// rollTableExportAdapter implements campaigns.RollTableExporter.
type rollTableExportAdapter struct{ svc rolltables.Service }

// ExportRollTables returns the tables document, or nil when the campaign has
// none so an empty campaign adds nothing to the file.
func (a *rollTableExportAdapter) ExportRollTables(ctx context.Context, campaignID string) (json.RawMessage, error) {
	doc, err := a.svc.Get(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if len(doc.Tables) == 0 {
		return nil, nil
	}
	return json.Marshal(doc)
}

// rollTableImportAdapter implements campaigns.RollTableImporter.
type rollTableImportAdapter struct{ svc rolltables.Service }

// ImportRollTables stores the document through the plugin's own validation, so
// a table the plugin would refuse is refused here too (the whole set, since it
// is stored as one document).
func (a *rollTableImportAdapter) ImportRollTables(ctx context.Context, campaignID, userID string, data json.RawMessage, report *campaigns.ImportReport) error {
	if _, err := a.svc.Put(ctx, campaignID, data, userID); err != nil {
		report.Fail(campaigns.SectionRollTables, campaigns.KindRollTables, "", apperror.SafeMessage(err))
	}
	return nil
}

// --- Quest sheets ---

// questExportAdapter implements campaigns.QuestExporter.
type questExportAdapter struct{ archive quests.Archive }

// ExportQuests returns every stored sheet with the name of its page.
func (a *questExportAdapter) ExportQuests(ctx context.Context, campaignID string) ([]campaigns.ExportQuest, error) {
	rows, err := a.archive.Export(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]campaigns.ExportQuest, 0, len(rows))
	for _, r := range rows {
		out = append(out, campaigns.ExportQuest{EntityID: r.EntityID, Name: r.Name, Sheet: json.RawMessage(r.Data)})
	}
	return out, nil
}

// questImportAdapter implements campaigns.QuestImporter.
type questImportAdapter struct{ archive quests.Archive }

// ImportQuests stores each sheet on its page's new copy. A sheet whose page
// did not come back, or that the plugin refuses, is reported and skipped.
func (a *questImportAdapter) ImportQuests(ctx context.Context, campaignID, userID string, data []campaigns.ExportQuest, idMap *campaigns.IDMap, report *campaigns.ImportReport) error {
	remap := quests.Remap{
		Entity: func(id string) (string, bool) { n, ok := idMap.EntityIDs[id]; return n, ok },
		Map:    func(id string) (string, bool) { n, ok := idMap.MapIDs[id]; return n, ok },
		Event:  func(id string) (string, bool) { n, ok := idMap.CalendarEventIDs[id]; return n, ok },
	}
	for _, q := range data {
		label := q.Name
		if label == "" {
			label = q.EntityID
		}
		pageID, ok := idMap.EntityIDs[q.EntityID]
		if !ok {
			report.Fail(campaigns.SectionQuests, campaigns.KindQuest, label, "its page did not come back")
			continue
		}
		left, err := a.archive.Import(ctx, campaignID, pageID, userID, q.Sheet, remap)
		if err != nil {
			slog.Warn("import: quest sheet failed", slog.String("page", label), slog.Any("error", err))
			report.Fail(campaigns.SectionQuests, campaigns.KindQuest, label, apperror.SafeMessage(err))
			continue
		}
		if len(left) > 0 {
			report.Fail(campaigns.SectionQuests, campaigns.KindQuest, label,
				"restored without "+strings.Join(left, ", ")+" (not in the file or not restored)")
		}
	}
	return nil
}

// --- Crafting recipes ---

// recipeExportAdapter implements campaigns.RecipeExporter.
type recipeExportAdapter struct{ svc crafting.Service }

// exportDirector is the viewer export and import act as: the owner doing the
// export or import is a Director of the campaign, and the crafting service
// checks only that flag.
func exportDirector(userID string) crafting.Viewer {
	return crafting.Viewer{UserID: userID, MemberRole: permissions.RoleOwner, VisibilityRole: permissions.RoleOwner, Director: true}
}

// ExportRecipes returns every recipe, drafts included, in the campaign's order.
func (a *recipeExportAdapter) ExportRecipes(ctx context.Context, campaignID string) ([]campaigns.ExportRecipe, error) {
	list, err := a.svc.ListRecipes(ctx, campaignID, exportDirector(""))
	if err != nil {
		return nil, err
	}
	out := make([]campaigns.ExportRecipe, 0, len(list))
	for _, r := range list {
		raw, err := json.Marshal(r)
		if err != nil {
			return nil, fmt.Errorf("export recipe %q: %w", r.Name, err)
		}
		out = append(out, campaigns.ExportRecipe{Name: r.Name, Draft: r.Draft, Recipe: raw})
	}
	return out, nil
}

// recipeImportAdapter implements campaigns.RecipeImporter.
type recipeImportAdapter struct{ svc crafting.Service }

// ImportRecipes creates each recipe through the service, so the plugin checks
// the item and character pages exist in the new campaign, then saves its rules,
// looks and extras on top. A recipe that fails is reported by name.
func (a *recipeImportAdapter) ImportRecipes(ctx context.Context, campaignID, userID string, data []campaigns.ExportRecipe, idMap *campaigns.IDMap, report *campaigns.ImportReport) error {
	v := exportDirector(userID)
	for _, er := range data {
		var r crafting.Recipe
		if err := json.Unmarshal(er.Recipe, &r); err != nil {
			report.Fail(campaigns.SectionRecipes, campaigns.KindRecipe, er.Name, "the recipe is not readable")
			continue
		}
		// A page id that did not come back is left as it is, so the service
		// refuses the recipe and the report says a page is missing.
		for i, m := range r.Materials {
			if n, ok := idMap.EntityIDs[m.ItemID]; ok {
				r.Materials[i].ItemID = n
			}
		}
		if n, ok := idMap.EntityIDs[r.Result.ItemID]; ok {
			r.Result.ItemID = n
		}
		known := make([]string, 0, len(r.Known))
		lostKnown := 0
		for _, id := range r.Known {
			if n, ok := idMap.EntityIDs[id]; ok {
				known = append(known, n)
			} else {
				lostKnown++
			}
		}
		created, err := a.svc.CreateRecipe(ctx, campaignID, v, crafting.RecipeInput{
			Name: r.Name, Template: r.Template, Bench: r.Bench, Materials: r.Materials, Steps: r.Steps,
			Result: r.Result, Goal: r.Goal, Stat: r.Stat, Prerequisite: r.Prerequisite, Source: r.Source,
			Cost: r.Cost, Who: r.Who, Known: known, Colors: r.Colors, Draft: r.Draft,
		})
		if err != nil {
			slog.Warn("import: crafting recipe failed", slog.String("recipe", er.Name), slog.Any("error", err))
			report.Fail(campaigns.SectionRecipes, campaigns.KindRecipe, er.Name, apperror.SafeMessage(err))
			continue
		}
		if lostKnown > 0 {
			report.Fail(campaigns.SectionRecipes, campaigns.KindRecipe, er.Name,
				fmt.Sprintf("restored, but %d character(s) who knew it did not come back", lostKnown))
		}
		rules, _ := json.Marshal(r.Rules)
		looks, _ := json.Marshal(r.Looks)
		extras, _ := json.Marshal(r.Extras)
		if _, err := a.svc.SaveRules(ctx, campaignID, created.ID, v, crafting.RulesInput{
			Rules: rules, Looks: looks, Extras: extras, Version: created.Version,
		}); err != nil {
			report.Fail(campaigns.SectionRecipes, campaigns.KindRecipe, er.Name,
				"restored without its rules and looks: "+apperror.SafeMessage(err))
		}
	}
	return nil
}
