// Package app — export_adapters.go provides adapter implementations that bridge
// plugin services to the campaign export/import interfaces. Each adapter converts
// plugin-specific types to the export model types defined in campaigns/export.go.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/widgets/posts"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// --- Entity Export Adapter ---

// entityExportAdapter implements campaigns.EntityExporter.
type entityExportAdapter struct {
	entitySvc   entities.EntityService
	tagSvc      tags.TagService
	relationSvc relations.RelationService
}

// ExportEntities gathers all entity-related data for a campaign export.
func (a *entityExportAdapter) ExportEntities(ctx context.Context, campaignID string) (*campaigns.ExportEntityData, error) {
	data := &campaigns.ExportEntityData{}

	// Export entity types.
	etypes, err := a.entitySvc.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	typeIDToSlug := make(map[int]string, len(etypes))
	for _, et := range etypes {
		typeIDToSlug[et.ID] = et.Slug

		fieldsJSON, _ := json.Marshal(et.Fields)
		layoutJSON, _ := json.Marshal(et.Layout)

		data.Types = append(data.Types, campaigns.ExportEntityType{
			OriginalID:      et.ID,
			Slug:            et.Slug,
			Name:            et.Name,
			NamePlural:      et.NamePlural,
			Icon:            et.Icon,
			Color:           et.Color,
			Description:     et.Description,
			PinnedEntityIDs: et.PinnedEntityIDs,
			DashboardLayout: et.DashboardLayout,
			Fields:          fieldsJSON,
			Layout:          layoutJSON,
			SortOrder:       et.SortOrder,
			IsDefault:       et.IsDefault,
			Enabled:         et.Enabled,
		})
	}

	// Export all entities (paginated fetch, owner role = 3 sees everything).
	entitySlugToID := make(map[string]string)
	entityIDToSlug := make(map[string]string)
	entityIDToParentID := make(map[string]*string)

	const ownerRole = 3
	page := 1
	for {
		ents, _, err := a.entitySvc.List(ctx, campaignID, 0, ownerRole, "", entities.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, err
		}
		if len(ents) == 0 {
			break
		}

		for _, e := range ents {
			entitySlugToID[e.Slug] = e.ID
			entityIDToSlug[e.ID] = e.Slug
			entityIDToParentID[e.ID] = e.ParentID

			var fieldsData json.RawMessage
			if e.FieldsData != nil {
				fieldsData, _ = json.Marshal(e.FieldsData)
			}
			var fieldOverrides json.RawMessage
			if e.FieldOverrides != nil {
				fieldOverrides, _ = json.Marshal(e.FieldOverrides)
			}
			var popupConfig json.RawMessage
			if e.PopupConfig != nil {
				popupConfig, _ = json.Marshal(e.PopupConfig)
			}

			exportEntity := campaigns.ExportEntity{
				OriginalID:     e.ID,
				EntityTypeSlug: typeIDToSlug[e.EntityTypeID],
				Name:           e.Name,
				Slug:           e.Slug,
				Entry:          e.Entry,
				EntryHTML:      e.EntryHTML,
				ImagePath:      e.ImagePath,
				TypeLabel:      e.TypeLabel,
				IsPrivate:      e.IsPrivate,
				IsTemplate:     e.IsTemplate,
				Visibility:     string(e.Visibility),
				FieldsData:     fieldsData,
				FieldOverrides: fieldOverrides,
				PopupConfig:    popupConfig,
			}

			// Export per-entity permissions when visibility is custom.
			if e.Visibility == entities.VisibilityCustom {
				perms, err := a.entitySvc.GetEntityPermissions(ctx, e.ID)
				if err == nil {
					for _, p := range perms {
						exportEntity.Permissions = append(exportEntity.Permissions, campaigns.ExportEntityPermission{
							SubjectType: string(p.SubjectType),
							SubjectID:   p.SubjectID,
							Permission:  string(p.Permission),
						})
					}
				}
			}

			data.Entities = append(data.Entities, exportEntity)
		}

		page++
	}

	// Resolve parent slugs (second pass after all entities are loaded).
	for i, e := range data.Entities {
		if parentID := entityIDToParentID[e.OriginalID]; parentID != nil {
			if slug, ok := entityIDToSlug[*parentID]; ok {
				data.Entities[i].ParentSlug = &slug
			}
		}
	}

	// Export tags.
	allTags, err := a.tagSvc.ListByCampaign(ctx, campaignID, true)
	if err != nil {
		return nil, err
	}

	tagIDToSlug := make(map[int]string, len(allTags))
	for _, t := range allTags {
		tagIDToSlug[t.ID] = t.Slug
		data.Tags = append(data.Tags, campaigns.ExportTag{
			OriginalID: t.ID,
			Name:       t.Name,
			Slug:       t.Slug,
			Color:      t.Color,
			DmOnly:     t.DmOnly,
		})
	}

	// Export entity-tag associations.
	entityIDs := make([]string, 0, len(entitySlugToID))
	for _, id := range entitySlugToID {
		entityIDs = append(entityIDs, id)
	}
	if len(entityIDs) > 0 {
		tagMap, err := a.tagSvc.GetEntityTagsBatch(ctx, entityIDs, true)
		if err != nil {
			return nil, err
		}
		for entityID, entityTags := range tagMap {
			entitySlug := entityIDToSlug[entityID]
			for _, t := range entityTags {
				tagSlug := tagIDToSlug[t.ID]
				if entitySlug != "" && tagSlug != "" {
					data.EntityTags = append(data.EntityTags, campaigns.ExportEntityTag{
						EntitySlug: entitySlug,
						TagSlug:    tagSlug,
					})
				}
			}
		}
	}

	// Export relations.
	// Relations are per-entity, so we collect from all entities and deduplicate.
	seenRelations := make(map[string]bool)
	for entityID := range entityIDToSlug {
		rels, err := a.relationSvc.ListByEntity(ctx, entityID)
		if err != nil {
			continue
		}
		for _, r := range rels {
			// Deduplicate: each relation appears twice (source→target and target→source).
			key := fmt.Sprintf("%d", r.ID)
			if seenRelations[key] {
				continue
			}
			seenRelations[key] = true

			sourceSlug := entityIDToSlug[r.SourceEntityID]
			targetSlug := entityIDToSlug[r.TargetEntityID]
			if sourceSlug == "" || targetSlug == "" {
				continue
			}

			data.Relations = append(data.Relations, campaigns.ExportRelation{
				SourceEntitySlug:    sourceSlug,
				TargetEntitySlug:    targetSlug,
				RelationType:        r.RelationType,
				ReverseRelationType: r.ReverseRelationType,
				Metadata:            r.Metadata,
			})
		}
	}

	return data, nil
}

// --- Calendar Export Adapter ---

// calendarExportAdapter implements campaigns.CalendarExporter.
type calendarExportAdapter struct {
	svc calendar.CalendarService
}

// ExportCalendar gathers calendar data for a campaign export.
func (a *calendarExportAdapter) ExportCalendar(ctx context.Context, campaignID string, entitySlugLookup func(string) string) (*campaigns.ExportCalendarData, error) {
	cal, err := a.svc.GetCalendar(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	data := &campaigns.ExportCalendarData{
		Name:             cal.Name,
		Description:      cal.Description,
		Mode:             cal.Mode,
		EpochName:        cal.EpochName,
		CurrentYear:      cal.CurrentYear,
		CurrentMonth:     cal.CurrentMonth,
		CurrentDay:       cal.CurrentDay,
		CurrentHour:      cal.CurrentHour,
		CurrentMinute:    cal.CurrentMinute,
		HoursPerDay:      cal.HoursPerDay,
		MinutesPerHour:   cal.MinutesPerHour,
		SecondsPerMinute: cal.SecondsPerMinute,
		LeapYearEvery:    cal.LeapYearEvery,
		LeapYearOffset:   cal.LeapYearOffset,
	}

	for _, m := range cal.Months {
		data.Months = append(data.Months, campaigns.ExportCalendarMonth{
			Name: m.Name, Days: m.Days, SortOrder: m.SortOrder,
			IsIntercalary: m.IsIntercalary, LeapYearDays: m.LeapYearDays,
		})
	}
	for _, w := range cal.Weekdays {
		data.Weekdays = append(data.Weekdays, campaigns.ExportCalendarWeekday{
			Name: w.Name, SortOrder: w.SortOrder,
		})
	}
	for _, m := range cal.Moons {
		data.Moons = append(data.Moons, campaigns.ExportCalendarMoon{
			Name: m.Name, CycleDays: m.CycleDays, PhaseOffset: m.PhaseOffset, Color: m.Color,
		})
	}
	for _, s := range cal.Seasons {
		data.Seasons = append(data.Seasons, campaigns.ExportCalendarSeason{
			Name: s.Name, StartMonth: s.StartMonth, StartDay: s.StartDay,
			EndMonth: s.EndMonth, EndDay: s.EndDay, Description: s.Description,
			Color: s.Color, WeatherEffect: s.WeatherEffect,
		})
	}
	for _, e := range cal.Eras {
		data.Eras = append(data.Eras, campaigns.ExportCalendarEra{
			Name: e.Name, StartYear: e.StartYear, EndYear: e.EndYear,
			Description: e.Description, Color: e.Color, SortOrder: e.SortOrder,
		})
	}

	// Event categories.
	cats, err := a.svc.GetEventCategories(ctx, cal.ID)
	if err == nil {
		for _, c := range cats {
			data.EventCategories = append(data.EventCategories, campaigns.ExportEventCategory{
				Slug: c.Slug, Name: c.Name, Icon: c.Icon, Color: c.Color, SortOrder: c.SortOrder,
			})
		}
	}

	// Events (all, including DM-only).
	events, err := a.svc.ListAllEvents(ctx, cal.ID)
	if err == nil {
		for _, evt := range events {
			var entitySlug *string
			if evt.EntityID != nil {
				s := entitySlugLookup(*evt.EntityID)
				if s != "" {
					entitySlug = &s
				}
			}
			data.Events = append(data.Events, campaigns.ExportCalendarEvent{
				Name: evt.Name, Description: evt.Description, DescriptionHTML: evt.DescriptionHTML,
				EntitySlug: entitySlug, Year: evt.Year, Month: evt.Month, Day: evt.Day,
				StartHour: evt.StartHour, StartMinute: evt.StartMinute,
				EndYear: evt.EndYear, EndMonth: evt.EndMonth, EndDay: evt.EndDay,
				EndHour: evt.EndHour, EndMinute: evt.EndMinute,
				IsRecurring: evt.IsRecurring, RecurrenceType: evt.RecurrenceType,
				Visibility: evt.Visibility, Category: evt.Category,
			})
		}
	}

	return data, nil
}

// --- Timeline Export Adapter ---

// timelineExportAdapter implements campaigns.TimelineExporter.
type timelineExportAdapter struct {
	svc timeline.TimelineService
}

// ExportTimelines gathers timeline data for a campaign export.
func (a *timelineExportAdapter) ExportTimelines(ctx context.Context, campaignID string, entitySlugLookup func(string) string) ([]campaigns.ExportTimeline, error) {
	const ownerRole = 3
	timelines, err := a.svc.ListTimelines(ctx, campaignID, ownerRole, "")
	if err != nil {
		return nil, err
	}

	var result []campaigns.ExportTimeline
	for _, tl := range timelines {
		et := campaigns.ExportTimeline{
			Name:            tl.Name,
			Description:     tl.Description,
			DescriptionHTML: tl.DescriptionHTML,
			Color:           tl.Color,
			Icon:            tl.Icon,
			Visibility:      tl.Visibility,
			SortOrder:       tl.SortOrder,
			ZoomDefault:     tl.ZoomDefault,
		}

		// Export standalone events only (calendar events are in the calendar section).
		// Build event ID → export index map for connection references.
		eventIDToIndex := make(map[string]int)
		events, err := a.svc.ListTimelineEvents(ctx, tl.ID, ownerRole, "")
		if err == nil {
			for _, evt := range events {
				if evt.Source != "standalone" {
					continue
				}
				var entitySlug *string
				if evt.EventEntityID != nil {
					s := entitySlugLookup(*evt.EventEntityID)
					if s != "" {
						entitySlug = &s
					}
				}
				idx := len(et.Events)
				eventIDToIndex[evt.EventID] = idx
				et.Events = append(et.Events, campaigns.ExportTimelineEvent{
					Name: evt.EventName, EntitySlug: entitySlug,
					Year: evt.EventYear, Month: evt.EventMonth, Day: evt.EventDay,
					EndYear: evt.EventEndYear, EndMonth: evt.EventEndMonth, EndDay: evt.EventEndDay,
					Category: evt.EventCategory, Visibility: evt.EventVisibility,
					DisplayOrder: evt.DisplayOrder, Label: evt.Label, Color: evt.ColorOverride,
				})
			}
		}

		// Export connections between exported events.
		connections, err := a.svc.ListConnections(ctx, tl.ID)
		if err == nil {
			for _, c := range connections {
				srcIdx, srcOK := eventIDToIndex[c.SourceID]
				tgtIdx, tgtOK := eventIDToIndex[c.TargetID]
				if !srcOK || !tgtOK {
					continue // Skip connections to non-exported events.
				}
				et.Connections = append(et.Connections, campaigns.ExportEventConnection{
					SourceIndex: srcIdx,
					TargetIndex: tgtIdx,
					Label:       c.Label,
					Color:       c.Color,
					Style:       c.Style,
				})
			}
		}

		// Export entity groups (swim lanes).
		groups, err := a.svc.ListEntityGroups(ctx, tl.ID)
		if err == nil {
			for _, g := range groups {
				eg := campaigns.ExportEntityGroup{
					Name:      g.Name,
					Color:     g.Color,
					SortOrder: g.SortOrder,
				}
				for _, m := range g.Members {
					slug := entitySlugLookup(m.EntityID)
					if slug != "" {
						eg.Members = append(eg.Members, slug)
					}
				}
				et.EntityGroups = append(et.EntityGroups, eg)
			}
		}

		result = append(result, et)
	}

	return result, nil
}

// --- Session Export Adapter ---

// sessionExportAdapter implements campaigns.SessionExporter.
type sessionExportAdapter struct {
	svc sessions.SessionService
}

// ExportSessions gathers session data for a campaign export.
func (a *sessionExportAdapter) ExportSessions(ctx context.Context, campaignID string, entitySlugLookup func(string) string) ([]campaigns.ExportSession, error) {
	allSessions, err := a.svc.ListSessions(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	var result []campaigns.ExportSession
	for _, sess := range allSessions {
		es := campaigns.ExportSession{
			Name:               sess.Name,
			Summary:            sess.Summary,
			Notes:              sess.Notes,
			NotesHTML:          sess.NotesHTML,
			ScheduledDate:      sess.ScheduledDate,
			CalendarYear:       sess.CalendarYear,
			CalendarMonth:      sess.CalendarMonth,
			CalendarDay:        sess.CalendarDay,
			Status:             sess.Status,
			IsRecurring:        sess.IsRecurring,
			RecurrenceType:     sess.RecurrenceType,
			RecurrenceInterval: sess.RecurrenceInterval,
			SortOrder:          sess.SortOrder,
		}

		// Session entities.
		for _, se := range sess.Entities {
			slug := entitySlugLookup(se.EntityID)
			if slug != "" {
				es.Entities = append(es.Entities, campaigns.ExportSessionEntity{
					EntitySlug: slug,
					Role:       se.Role,
				})
			}
		}

		// Session attendees (RSVP statuses).
		attendees, err := a.svc.ListAttendees(ctx, sess.ID)
		if err == nil {
			for _, att := range attendees {
				es.Attendees = append(es.Attendees, campaigns.ExportAttendee{
					UserID: att.UserID,
					Status: att.Status,
				})
			}
		}

		result = append(result, es)
	}

	return result, nil
}

// --- Map Export Adapter ---

// mapExportAdapter implements campaigns.MapExporter.
type mapExportAdapter struct {
	mapSvc     maps.MapService
	drawingSvc maps.DrawingService
}

// ExportMaps gathers map data for a campaign export.
func (a *mapExportAdapter) ExportMaps(ctx context.Context, campaignID string, entitySlugLookup func(string) string) ([]campaigns.ExportMap, error) {
	allMaps, err := a.mapSvc.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	const ownerRole = 3
	var result []campaigns.ExportMap
	for _, m := range allMaps {
		em := campaigns.ExportMap{
			Name:        m.Name,
			Description: m.Description,
			ImageID:     m.ImageID,
			ImageWidth:  m.ImageWidth,
			ImageHeight: m.ImageHeight,
			SortOrder:   m.SortOrder,
		}

		// Markers (owner role sees all).
		markers, err := a.mapSvc.ListMarkers(ctx, m.ID, ownerRole)
		if err == nil {
			for _, mk := range markers {
				var entitySlug *string
				if mk.EntityID != nil {
					s := entitySlugLookup(*mk.EntityID)
					if s != "" {
						entitySlug = &s
					}
				}
				em.Markers = append(em.Markers, campaigns.ExportMarker{
					Name: mk.Name, Description: mk.Description,
					X: mk.X, Y: mk.Y, Icon: mk.Icon, Color: mk.Color,
					EntitySlug: entitySlug, Visibility: mk.Visibility,
				})
			}
		}

		// Layers.
		layers, err := a.drawingSvc.ListLayers(ctx, m.ID)
		if err == nil {
			layerIDToName := make(map[string]string, len(layers))
			for _, l := range layers {
				layerIDToName[l.ID] = l.Name
				em.Layers = append(em.Layers, campaigns.ExportLayer{
					Name: l.Name, LayerType: l.LayerType,
					Visible: l.IsVisible, Locked: l.IsLocked,
					Opacity: l.Opacity, SortOrder: l.SortOrder,
				})
			}

			// Drawings (owner sees all).
			drawings, err := a.drawingSvc.ListDrawings(ctx, m.ID, ownerRole)
			if err == nil {
				for _, d := range drawings {
					var layerName *string
					if d.LayerID != nil {
						if name, ok := layerIDToName[*d.LayerID]; ok {
							layerName = &name
						}
					}
					em.Drawings = append(em.Drawings, campaigns.ExportDrawing{
						DrawingType: d.DrawingType, LayerName: layerName,
						Points: d.Points, StrokeColor: d.StrokeColor,
						StrokeWidth: d.StrokeWidth, FillColor: d.FillColor,
						FillAlpha: d.FillAlpha, TextContent: d.TextContent,
						FontSize: d.FontSize, Rotation: d.Rotation,
						Visibility: d.Visibility,
					})
				}
			}

			// Tokens (owner sees all).
			tokens, err := a.drawingSvc.ListTokens(ctx, m.ID, ownerRole)
			if err == nil {
				for _, t := range tokens {
					var entitySlug *string
					if t.EntityID != nil {
						s := entitySlugLookup(*t.EntityID)
						if s != "" {
							entitySlug = &s
						}
					}
					var layerName *string
					if t.LayerID != nil {
						if name, ok := layerIDToName[*t.LayerID]; ok {
							layerName = &name
						}
					}
					em.Tokens = append(em.Tokens, campaigns.ExportToken{
						Name: t.Name, EntitySlug: entitySlug,
						ImagePath: t.ImagePath, LayerName: layerName,
						X: t.X, Y: t.Y, Width: t.Width, Height: t.Height,
						Rotation: t.Rotation, Scale: t.Scale,
						IsHidden: t.IsHidden, IsLocked: t.IsLocked,
						Bar1Value: t.Bar1Value, Bar1Max: t.Bar1Max,
						Bar2Value: t.Bar2Value, Bar2Max: t.Bar2Max,
						AuraRadius: t.AuraRadius, AuraColor: t.AuraColor,
						LightRadius: t.LightRadius, LightDimRadius: t.LightDimRadius,
						LightColor: t.LightColor, VisionEnabled: t.VisionEnabled,
						VisionRange: t.VisionRange, Elevation: t.Elevation,
						StatusEffects: t.StatusEffects,
					})
				}
			}
		}

		// Fog regions.
		fog, err := a.drawingSvc.ListFog(ctx, m.ID)
		if err == nil {
			for _, f := range fog {
				em.FogRegions = append(em.FogRegions, campaigns.ExportFogRegion{
					Points: f.Points, IsExplored: f.IsExplored,
				})
			}
		}

		result = append(result, em)
	}

	return result, nil
}

// Note export is intentionally not implemented in v1 because notes are
// per-user and the NoteService doesn't support listing all shared notes
// across users. A future version could add a ListSharedByCampaign
// repository method to enable shared note export.

// --- Addon Export Adapter ---

// addonExportAdapter implements campaigns.AddonExporter.
type addonExportAdapter struct {
	svc addons.AddonService
}

// ExportAddons gathers addon configuration for a campaign export.
func (a *addonExportAdapter) ExportAddons(ctx context.Context, campaignID string) ([]campaigns.ExportAddon, error) {
	campaignAddons, err := a.svc.ListForCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	var result []campaigns.ExportAddon
	for _, ca := range campaignAddons {
		if !ca.Enabled {
			continue
		}
		var config json.RawMessage
		if ca.ConfigJSON != nil {
			config, _ = json.Marshal(ca.ConfigJSON)
		}
		result = append(result, campaigns.ExportAddon{
			Slug:    ca.AddonSlug,
			Enabled: ca.Enabled,
			Config:  config,
		})
	}

	return result, nil
}

// --- Media Export Adapter ---

// mediaExportAdapter implements campaigns.MediaExporter.
type mediaExportAdapter struct {
	svc media.MediaService
}

// ExportMedia gathers media file metadata for a campaign export.
func (a *mediaExportAdapter) ExportMedia(ctx context.Context, campaignID string) ([]campaigns.ExportMediaFile, error) {
	// Fetch all media pages.
	var result []campaigns.ExportMediaFile
	page := 1
	for {
		files, _, err := a.svc.ListCampaignMedia(ctx, campaignID, page, 100)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			break
		}
		for _, f := range files {
			result = append(result, campaigns.ExportMediaFile{
				OriginalID:   f.ID,
				OriginalName: f.OriginalName,
				MimeType:     f.MimeType,
				FileSize:     f.FileSize,
				UsageType:    f.UsageType,
			})
		}
		page++
	}

	return result, nil
}

// --- Import Adapters ---

// entityImportAdapter implements campaigns.EntityImporter.
type entityImportAdapter struct {
	entitySvc   entities.EntityService
	tagSvc      tags.TagService
	relationSvc relations.RelationService
}

// ImportEntities creates entity types, entities, tags, and relations from
// import data. Returns an IDMap for cross-referencing by other importers.
func (a *entityImportAdapter) ImportEntities(ctx context.Context, campaignID, userID string, data *campaigns.ExportEntityData) (*campaigns.IDMap, error) {
	idMap := campaigns.NewIDMap(campaignID)

	// 1. Create entity types.
	typeSlugToNewID := make(map[string]int)
	for _, et := range data.Types {
		// Create type with basic fields.
		newType, err := a.entitySvc.CreateEntityType(ctx, campaignID, entities.CreateEntityTypeInput{
			Name:       et.Name,
			NamePlural: et.NamePlural,
			Icon:       et.Icon,
			Color:      et.Color,
		})
		if err != nil {
			slog.Warn("import: create entity type failed", slog.String("slug", et.Slug), slog.Any("error", err))
			continue
		}

		idMap.EntityTypeIDs[et.OriginalID] = newType.ID
		typeSlugToNewID[et.Slug] = newType.ID

		// Apply fields via UpdateEntityType.
		var fields []entities.FieldDefinition
		if len(et.Fields) > 0 {
			if err := json.Unmarshal(et.Fields, &fields); err != nil {
				slog.Warn("import: invalid fields JSON", slog.String("type", et.Slug), slog.Any("error", err))
			}
		}
		if len(fields) > 0 {
			_, err := a.entitySvc.UpdateEntityType(ctx, newType.ID, entities.UpdateEntityTypeInput{
				Name: et.Name, NamePlural: et.NamePlural, Icon: et.Icon, Color: et.Color,
				Fields: fields,
			})
			if err != nil {
				slog.Warn("import: update entity type fields failed", slog.String("type", et.Slug), slog.Any("error", err))
			}
		}

		// Apply layout if present.
		if len(et.Layout) > 0 {
			var layout entities.EntityTypeLayout
			if err := json.Unmarshal(et.Layout, &layout); err == nil {
				if err := a.entitySvc.UpdateEntityTypeLayout(ctx, newType.ID, layout); err != nil {
					slog.Warn("import: apply layout failed", slog.String("type", et.Slug), slog.Any("error", err))
				}
			}
		}

		// Apply description and dashboard layout.
		if et.Description != nil || et.DashboardLayout != nil {
			_ = a.entitySvc.UpdateEntityTypeDashboard(ctx, newType.ID, et.Description, et.PinnedEntityIDs)
		}
	}

	// 2. Create entities (first pass: without parent references).
	entitySlugToNewID := make(map[string]string)
	for _, e := range data.Entities {
		typeID, ok := typeSlugToNewID[e.EntityTypeSlug]
		if !ok {
			slog.Warn("import: unknown entity type", slog.String("slug", e.EntityTypeSlug))
			continue
		}

		var fieldsData map[string]any
		if len(e.FieldsData) > 0 {
			_ = json.Unmarshal(e.FieldsData, &fieldsData)
		}

		newEntity, err := a.entitySvc.Create(ctx, campaignID, userID, entities.CreateEntityInput{
			Name:         e.Name,
			EntityTypeID: typeID,
			TypeLabel:    ptrString(e.TypeLabel),
			IsPrivate:    e.IsPrivate,
			FieldsData:   fieldsData,
		})
		if err != nil {
			slog.Warn("import: create entity failed", slog.String("name", e.Name), slog.Any("error", err))
			continue
		}

		idMap.EntityIDs[e.OriginalID] = newEntity.ID
		idMap.EntitySlugToID[e.Slug] = newEntity.ID
		entitySlugToNewID[e.Slug] = newEntity.ID

		// Apply entry content and image via Update.
		if e.Entry != nil || e.ImagePath != nil {
			_, updateErr := a.entitySvc.Update(ctx, newEntity.ID, entities.UpdateEntityInput{
				Name:       e.Name,
				TypeLabel:  ptrString(e.TypeLabel),
				IsPrivate:  e.IsPrivate,
				Entry:      ptrString(e.Entry),
				ImagePath:  ptrString(e.ImagePath),
				FieldsData: fieldsData,
			})
			if updateErr != nil {
				slog.Warn("import: update entity entry/image failed", slog.String("entity", e.Name), slog.Any("error", updateErr))
			}
		}

		// Apply field overrides.
		if len(e.FieldOverrides) > 0 {
			var overrides entities.FieldOverrides
			if err := json.Unmarshal(e.FieldOverrides, &overrides); err == nil {
				if err := a.entitySvc.UpdateFieldOverrides(ctx, newEntity.ID, &overrides); err != nil {
					slog.Warn("import: apply field overrides failed", slog.String("entity", e.Name), slog.Any("error", err))
				}
			}
		}

		// Apply popup config.
		if len(e.PopupConfig) > 0 {
			var config entities.PopupConfig
			if err := json.Unmarshal(e.PopupConfig, &config); err == nil {
				if err := a.entitySvc.UpdatePopupConfig(ctx, newEntity.ID, &config); err != nil {
					slog.Warn("import: apply popup config failed", slog.String("entity", e.Name), slog.Any("error", err))
				}
			}
		}

		// Apply entity permissions and visibility mode.
		if e.Visibility == string(entities.VisibilityCustom) && len(e.Permissions) > 0 {
			grants := make([]entities.PermissionGrant, 0, len(e.Permissions))
			for _, p := range e.Permissions {
				grants = append(grants, entities.PermissionGrant{
					SubjectType: entities.SubjectType(p.SubjectType),
					SubjectID:   p.SubjectID,
					Permission:  entities.Permission(p.Permission),
				})
			}
			err := a.entitySvc.SetEntityPermissions(ctx, newEntity.ID, entities.SetPermissionsInput{
				Visibility:  entities.VisibilityCustom,
				Permissions: grants,
			})
			if err != nil {
				slog.Warn("import: set entity permissions failed", slog.String("entity", e.Name), slog.Any("error", err))
			}
		}
	}

	// 2b. Resolve parent references (second pass: all entities now exist).
	for _, e := range data.Entities {
		if e.ParentSlug == nil {
			continue
		}
		entityNewID, ok := entitySlugToNewID[e.Slug]
		if !ok {
			continue
		}
		parentNewID, ok := entitySlugToNewID[*e.ParentSlug]
		if !ok {
			slog.Warn("import: parent entity not found", slog.String("entity", e.Name), slog.String("parent_slug", *e.ParentSlug))
			continue
		}

		var fieldsData map[string]any
		if len(e.FieldsData) > 0 {
			_ = json.Unmarshal(e.FieldsData, &fieldsData)
		}

		_, err := a.entitySvc.Update(ctx, entityNewID, entities.UpdateEntityInput{
			Name:       e.Name,
			TypeLabel:  ptrString(e.TypeLabel),
			ParentID:   parentNewID,
			IsPrivate:  e.IsPrivate,
			Entry:      ptrString(e.Entry),
			ImagePath:  ptrString(e.ImagePath),
			FieldsData: fieldsData,
		})
		if err != nil {
			slog.Warn("import: set parent failed", slog.String("entity", e.Name), slog.Any("error", err))
		}
	}

	// 3. Create tags.
	tagSlugToNewID := make(map[string]int)
	for _, t := range data.Tags {
		newTag, err := a.tagSvc.Create(ctx, campaignID, t.Name, t.Color, t.DmOnly)
		if err != nil {
			slog.Warn("import: create tag failed", slog.String("name", t.Name), slog.Any("error", err))
			continue
		}
		idMap.TagIDs[t.OriginalID] = newTag.ID
		idMap.TagSlugToID[t.Slug] = newTag.ID
		tagSlugToNewID[t.Slug] = newTag.ID
	}

	// 4. Apply entity-tag associations.
	for _, et := range data.EntityTags {
		entityID, ok := entitySlugToNewID[et.EntitySlug]
		if !ok {
			continue
		}
		tagID, ok := tagSlugToNewID[et.TagSlug]
		if !ok {
			continue
		}

		// Get current tags for entity and add the new one.
		currentTags, err := a.tagSvc.GetEntityTags(ctx, entityID, true)
		if err != nil {
			continue
		}
		tagIDs := make([]int, 0, len(currentTags)+1)
		for _, ct := range currentTags {
			tagIDs = append(tagIDs, ct.ID)
		}
		tagIDs = append(tagIDs, tagID)
		_ = a.tagSvc.SetEntityTags(ctx, entityID, campaignID, tagIDs)
	}

	// 5. Create relations.
	for _, r := range data.Relations {
		sourceID, ok := entitySlugToNewID[r.SourceEntitySlug]
		if !ok {
			continue
		}
		targetID, ok := entitySlugToNewID[r.TargetEntitySlug]
		if !ok {
			continue
		}
		_, err := a.relationSvc.Create(ctx, campaignID, sourceID, targetID,
			r.RelationType, r.ReverseRelationType, userID, r.Metadata)
		if err != nil {
			slog.Warn("import: create relation failed",
				slog.String("source", r.SourceEntitySlug),
				slog.String("target", r.TargetEntitySlug),
				slog.Any("error", err))
		}
	}

	return idMap, nil
}

// calendarImportAdapter implements campaigns.CalendarImporter.
type calendarImportAdapter struct {
	svc calendar.CalendarService
}

// ImportCalendar creates a calendar from import data.
func (a *calendarImportAdapter) ImportCalendar(ctx context.Context, campaignID string, data *campaigns.ExportCalendarData, idMap *campaigns.IDMap) error {
	// Convert to calendar import format.
	months := make([]calendar.MonthInput, len(data.Months))
	for i, m := range data.Months {
		months[i] = calendar.MonthInput{
			Name: m.Name, Days: m.Days, SortOrder: m.SortOrder,
			IsIntercalary: m.IsIntercalary, LeapYearDays: m.LeapYearDays,
		}
	}

	weekdays := make([]calendar.WeekdayInput, len(data.Weekdays))
	for i, w := range data.Weekdays {
		weekdays[i] = calendar.WeekdayInput{Name: w.Name, SortOrder: w.SortOrder}
	}

	// Create the calendar.
	cal, err := a.svc.CreateCalendar(ctx, campaignID, calendar.CreateCalendarInput{
		Name:             data.Name,
		Mode:             data.Mode,
		HoursPerDay:      data.HoursPerDay,
		MinutesPerHour:   data.MinutesPerHour,
		SecondsPerMinute: data.SecondsPerMinute,
	})
	if err != nil {
		return err
	}

	idMap.CalendarID = cal.ID

	// Set sub-resources.
	if len(months) > 0 {
		_ = a.svc.SetMonths(ctx, cal.ID, months)
	}
	if len(weekdays) > 0 {
		_ = a.svc.SetWeekdays(ctx, cal.ID, weekdays)
	}

	// Moons.
	if len(data.Moons) > 0 {
		moons := make([]calendar.MoonInput, len(data.Moons))
		for i, m := range data.Moons {
			moons[i] = calendar.MoonInput{
				Name: m.Name, CycleDays: m.CycleDays, PhaseOffset: m.PhaseOffset, Color: m.Color,
			}
		}
		_ = a.svc.SetMoons(ctx, cal.ID, moons)
	}

	// Eras.
	if len(data.Eras) > 0 {
		eras := make([]calendar.EraInput, len(data.Eras))
		for i, e := range data.Eras {
			eras[i] = calendar.EraInput{
				Name: e.Name, StartYear: e.StartYear, EndYear: e.EndYear,
				Description: e.Description, Color: e.Color, SortOrder: e.SortOrder,
			}
		}
		_ = a.svc.SetEras(ctx, cal.ID, eras)
	}

	// Event categories.
	if len(data.EventCategories) > 0 {
		cats := make([]calendar.EventCategoryInput, len(data.EventCategories))
		for i, c := range data.EventCategories {
			cats[i] = calendar.EventCategoryInput{
				Slug: c.Slug, Name: c.Name, Icon: c.Icon, Color: c.Color, SortOrder: c.SortOrder,
			}
		}
		_ = a.svc.SetEventCategories(ctx, cal.ID, cats)
	}

	// Set current date (via UpdateCalendar).
	_ = a.svc.UpdateCalendar(ctx, cal.ID, calendar.UpdateCalendarInput{
		Name:             data.Name,
		Description:      data.Description,
		EpochName:        data.EpochName,
		CurrentYear:      data.CurrentYear,
		CurrentMonth:     data.CurrentMonth,
		CurrentDay:       data.CurrentDay,
		HoursPerDay:      data.HoursPerDay,
		MinutesPerHour:   data.MinutesPerHour,
		SecondsPerMinute: data.SecondsPerMinute,
		LeapYearEvery:    data.LeapYearEvery,
		LeapYearOffset:   data.LeapYearOffset,
	})

	// Create events.
	for _, evt := range data.Events {
		var entityID *string
		if evt.EntitySlug != nil {
			if id, ok := idMap.EntitySlugToID[*evt.EntitySlug]; ok {
				entityID = &id
			}
		}
		_, err := a.svc.CreateEvent(ctx, cal.ID, calendar.CreateEventInput{
			Name:           evt.Name,
			Description:    evt.Description,
			DescriptionHTML: evt.DescriptionHTML,
			EntityID:       entityID,
			Year:           evt.Year,
			Month:          evt.Month,
			Day:            evt.Day,
			StartHour:      evt.StartHour,
			StartMinute:    evt.StartMinute,
			EndYear:        evt.EndYear,
			EndMonth:       evt.EndMonth,
			EndDay:         evt.EndDay,
			EndHour:        evt.EndHour,
			EndMinute:      evt.EndMinute,
			IsRecurring:    evt.IsRecurring,
			RecurrenceType: evt.RecurrenceType,
			Visibility:     evt.Visibility,
			Category:       evt.Category,
		})
		if err != nil {
			slog.Warn("import: create calendar event failed", slog.String("name", evt.Name), slog.Any("error", err))
		}
	}

	return nil
}

// sessionImportAdapter implements campaigns.SessionImporter.
type sessionImportAdapter struct {
	svc sessions.SessionService
}

// ImportSessions creates sessions from import data.
func (a *sessionImportAdapter) ImportSessions(ctx context.Context, campaignID, userID string, data []campaigns.ExportSession, idMap *campaigns.IDMap) error {
	for _, sess := range data {
		newSession, err := a.svc.CreateSession(ctx, campaignID, sessions.CreateSessionInput{
			Name:                sess.Name,
			ScheduledDate:       sess.ScheduledDate,
			CalendarYear:        sess.CalendarYear,
			CalendarMonth:       sess.CalendarMonth,
			CalendarDay:         sess.CalendarDay,
			IsRecurring:         sess.IsRecurring,
			RecurrenceType:      sess.RecurrenceType,
			RecurrenceInterval:  sess.RecurrenceInterval,
			CreatedBy:           userID,
		})
		if err != nil {
			slog.Warn("import: create session failed", slog.String("name", sess.Name), slog.Any("error", err))
			continue
		}

		// Apply summary and status via UpdateSession.
		if sess.Summary != nil || sess.Status != "" {
			status := sess.Status
			if status == "" {
				status = "planned"
			}
			_, _ = a.svc.UpdateSession(ctx, newSession.ID, sessions.UpdateSessionInput{
				Name:               sess.Name,
				Summary:            sess.Summary,
				ScheduledDate:      sess.ScheduledDate,
				CalendarYear:       sess.CalendarYear,
				CalendarMonth:      sess.CalendarMonth,
				CalendarDay:        sess.CalendarDay,
				Status:             status,
				IsRecurring:        sess.IsRecurring,
				RecurrenceType:     sess.RecurrenceType,
				RecurrenceInterval: sess.RecurrenceInterval,
			})
		}

		// Link entities (LinkEntity requires campaignID for IDOR checks).
		for _, se := range sess.Entities {
			if entityID, ok := idMap.EntitySlugToID[se.EntitySlug]; ok {
				_ = a.svc.LinkEntity(ctx, newSession.ID, entityID, se.Role, campaignID)
			}
		}

		// Import attendees. Invite all, then update RSVP status for non-invited.
		if len(sess.Attendees) > 0 {
			userIDs := make([]string, 0, len(sess.Attendees))
			for _, att := range sess.Attendees {
				userIDs = append(userIDs, att.UserID)
			}
			if err := a.svc.InviteAll(ctx, newSession.ID, userIDs); err != nil {
				slog.Warn("import: invite attendees failed", slog.String("session", sess.Name), slog.Any("error", err))
			} else {
				// Update statuses for attendees who responded.
				for _, att := range sess.Attendees {
					if att.Status != "invited" {
						_ = a.svc.UpdateRSVP(ctx, newSession.ID, att.UserID, att.Status)
					}
				}
			}
		}
	}
	return nil
}

// timelineImportAdapter implements campaigns.TimelineImporter.
type timelineImportAdapter struct {
	svc timeline.TimelineService
}

// ImportTimelines creates timelines from import data.
func (a *timelineImportAdapter) ImportTimelines(ctx context.Context, campaignID, userID string, data []campaigns.ExportTimeline, idMap *campaigns.IDMap) error {
	for _, tl := range data {
		// Link to calendar if one was created.
		var calendarID *string
		if idMap.CalendarID != "" {
			calendarID = &idMap.CalendarID
		}

		newTimeline, err := a.svc.CreateTimeline(ctx, campaignID, timeline.CreateTimelineInput{
			CampaignID:  campaignID,
			CalendarID:  calendarID,
			Name:        tl.Name,
			Description: tl.Description,
			Color:       tl.Color,
			Icon:        tl.Icon,
			Visibility:  tl.Visibility,
			ZoomDefault: tl.ZoomDefault,
			CreatedBy:   userID,
		})
		if err != nil {
			slog.Warn("import: create timeline failed", slog.String("name", tl.Name), slog.Any("error", err))
			continue
		}

		// Create standalone events and track index → new event ID for connections.
		eventIndexToID := make(map[int]string, len(tl.Events))
		for i, evt := range tl.Events {
			var entityID *string
			if evt.EntitySlug != nil {
				if id, ok := idMap.EntitySlugToID[*evt.EntitySlug]; ok {
					entityID = &id
				}
			}
			newEvt, err := a.svc.CreateStandaloneEvent(ctx, newTimeline.ID, timeline.CreateTimelineEventInput{
				Name:        evt.Name,
				Description: evt.Description,
				EntityID:    entityID,
				Year:        evt.Year,
				Month:       evt.Month,
				Day:         evt.Day,
				StartHour:   evt.StartHour,
				StartMinute: evt.StartMinute,
				EndYear:     evt.EndYear,
				EndMonth:    evt.EndMonth,
				EndDay:      evt.EndDay,
				EndHour:     evt.EndHour,
				EndMinute:   evt.EndMinute,
				IsRecurring: evt.IsRecurring,
				RecurrenceType: evt.RecurrenceType,
				Category:    evt.Category,
				Visibility:  evt.Visibility,
				Label:       evt.Label,
				Color:       evt.Color,
				CreatedBy:   userID,
			})
			if err != nil {
				slog.Warn("import: create timeline event failed", slog.String("name", evt.Name), slog.Any("error", err))
				continue
			}
			eventIndexToID[i] = newEvt.ID
		}

		// Create connections between imported events.
		for _, conn := range tl.Connections {
			srcID, srcOK := eventIndexToID[conn.SourceIndex]
			tgtID, tgtOK := eventIndexToID[conn.TargetIndex]
			if !srcOK || !tgtOK {
				continue
			}
			_, err := a.svc.CreateConnection(ctx, newTimeline.ID, timeline.CreateConnectionInput{
				SourceID:   srcID,
				TargetID:   tgtID,
				SourceType: "standalone",
				TargetType: "standalone",
				Label:      conn.Label,
				Color:      conn.Color,
				Style:      conn.Style,
			})
			if err != nil {
				slog.Warn("import: create timeline connection failed", slog.Any("error", err))
			}
		}

		// Create entity groups (swim lanes).
		for _, eg := range tl.EntityGroups {
			newGroup, err := a.svc.CreateEntityGroup(ctx, newTimeline.ID, timeline.CreateEntityGroupInput{
				Name:  eg.Name,
				Color: eg.Color,
			})
			if err != nil {
				slog.Warn("import: create entity group failed", slog.String("name", eg.Name), slog.Any("error", err))
				continue
			}
			for _, memberSlug := range eg.Members {
				if entityID, ok := idMap.EntitySlugToID[memberSlug]; ok {
					_ = a.svc.AddGroupMember(ctx, newTimeline.ID, newGroup.ID, entityID)
				}
			}
		}
	}
	return nil
}

// mapImportAdapter implements campaigns.MapImporter.
type mapImportAdapter struct {
	mapSvc     maps.MapService
	drawingSvc maps.DrawingService
}

// ImportMaps creates maps from import data.
func (a *mapImportAdapter) ImportMaps(ctx context.Context, campaignID, userID string, data []campaigns.ExportMap, idMap *campaigns.IDMap) error {
	for _, m := range data {
		newMap, err := a.mapSvc.CreateMap(ctx, maps.CreateMapInput{
			CampaignID:  campaignID,
			Name:        m.Name,
			Description: m.Description,
			ImageID:     m.ImageID,
			ImageWidth:  m.ImageWidth,
			ImageHeight: m.ImageHeight,
		})
		if err != nil {
			slog.Warn("import: create map failed", slog.String("name", m.Name), slog.Any("error", err))
			continue
		}

		// Create layers first (needed for drawing/token references).
		layerNameToID := make(map[string]string)
		for _, l := range m.Layers {
			newLayer, err := a.drawingSvc.CreateLayer(ctx, maps.CreateLayerInput{
				MapID: newMap.ID, Name: l.Name, LayerType: l.LayerType,
				SortOrder: l.SortOrder, IsVisible: l.Visible,
				Opacity: l.Opacity, IsLocked: l.Locked,
			})
			if err != nil {
				slog.Warn("import: create layer failed", slog.String("name", l.Name), slog.Any("error", err))
				continue
			}
			layerNameToID[l.Name] = newLayer.ID
		}

		// Markers.
		for _, mk := range m.Markers {
			var entityID *string
			if mk.EntitySlug != nil {
				if id, ok := idMap.EntitySlugToID[*mk.EntitySlug]; ok {
					entityID = &id
				}
			}
			_, err := a.mapSvc.CreateMarker(ctx, maps.CreateMarkerInput{
				MapID: newMap.ID, Name: mk.Name, Description: mk.Description,
				X: mk.X, Y: mk.Y, Icon: mk.Icon, Color: mk.Color,
				EntityID: entityID, Visibility: mk.Visibility, CreatedBy: userID,
			})
			if err != nil {
				slog.Warn("import: create marker failed", slog.String("name", mk.Name), slog.Any("error", err))
			}
		}

		// Drawings.
		for _, d := range m.Drawings {
			var layerID *string
			if d.LayerName != nil {
				if id, ok := layerNameToID[*d.LayerName]; ok {
					layerID = &id
				}
			}
			_, err := a.drawingSvc.CreateDrawing(ctx, maps.CreateDrawingInput{
				MapID: newMap.ID, LayerID: layerID, DrawingType: d.DrawingType,
				Points: d.Points, StrokeColor: d.StrokeColor,
				StrokeWidth: d.StrokeWidth, FillColor: d.FillColor,
				FillAlpha: d.FillAlpha, TextContent: d.TextContent,
				FontSize: d.FontSize, Rotation: d.Rotation,
				Visibility: d.Visibility, CreatedBy: userID,
			})
			if err != nil {
				slog.Warn("import: create drawing failed", slog.Any("error", err))
			}
		}

		// Tokens.
		for _, t := range m.Tokens {
			var entityID *string
			if t.EntitySlug != nil {
				if id, ok := idMap.EntitySlugToID[*t.EntitySlug]; ok {
					entityID = &id
				}
			}
			var layerID *string
			if t.LayerName != nil {
				if id, ok := layerNameToID[*t.LayerName]; ok {
					layerID = &id
				}
			}
			_, err := a.drawingSvc.CreateToken(ctx, maps.CreateTokenInput{
				MapID: newMap.ID, LayerID: layerID, EntityID: entityID,
				Name: t.Name, ImagePath: t.ImagePath,
				X: t.X, Y: t.Y, Width: t.Width, Height: t.Height,
				Rotation: t.Rotation, Scale: t.Scale,
				IsHidden: t.IsHidden, IsLocked: t.IsLocked,
				Bar1Value: t.Bar1Value, Bar1Max: t.Bar1Max,
				Bar2Value: t.Bar2Value, Bar2Max: t.Bar2Max,
				AuraRadius: t.AuraRadius, AuraColor: t.AuraColor,
				LightRadius: t.LightRadius, LightDimRadius: t.LightDimRadius,
				LightColor: t.LightColor, VisionEnabled: t.VisionEnabled,
				VisionRange: t.VisionRange, Elevation: t.Elevation,
				StatusEffects: t.StatusEffects,
				CreatedBy: userID,
			})
			if err != nil {
				slog.Warn("import: create token failed", slog.String("name", t.Name), slog.Any("error", err))
			}
		}

		// Fog regions.
		for _, f := range m.FogRegions {
			_, err := a.drawingSvc.CreateFog(ctx, maps.CreateFogInput{
				MapID: newMap.ID, Points: f.Points, IsExplored: f.IsExplored,
			})
			if err != nil {
				slog.Warn("import: create fog region failed", slog.Any("error", err))
			}
		}
	}
	return nil
}

// Note import is intentionally not implemented in v1 (see note export comment).

// addonImportAdapter implements campaigns.AddonImporter.
type addonImportAdapter struct {
	svc addons.AddonService
}

// ImportAddons enables addons from import data.
func (a *addonImportAdapter) ImportAddons(ctx context.Context, campaignID, userID string, data []campaigns.ExportAddon) error {
	for _, ad := range data {
		// Look up addon by slug.
		addon, err := a.svc.GetBySlug(ctx, ad.Slug)
		if err != nil {
			slog.Warn("import: addon not found", slog.String("slug", ad.Slug))
			continue
		}

		if err := a.svc.EnableForCampaign(ctx, campaignID, addon.ID, userID); err != nil {
			slog.Warn("import: enable addon failed", slog.String("slug", ad.Slug), slog.Any("error", err))
			continue
		}

		// Apply config if present.
		if len(ad.Config) > 0 {
			var config map[string]any
			if err := json.Unmarshal(ad.Config, &config); err == nil {
				_ = a.svc.UpdateCampaignConfig(ctx, campaignID, addon.ID, config)
			}
		}
	}
	return nil
}

// --- Group Export/Import Adapters ---

// groupExportAdapter implements campaigns.GroupExporter.
type groupExportAdapter struct {
	svc campaigns.GroupService
}

// ExportGroups gathers campaign groups with their member user IDs.
func (a *groupExportAdapter) ExportGroups(ctx context.Context, campaignID string) ([]campaigns.ExportGroup, error) {
	groups, err := a.svc.ListGroups(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	var result []campaigns.ExportGroup
	for _, g := range groups {
		eg := campaigns.ExportGroup{
			Name:        g.Name,
			Description: g.Description,
		}

		members, err := a.svc.ListGroupMembers(ctx, g.ID)
		if err == nil {
			for _, m := range members {
				eg.MemberIDs = append(eg.MemberIDs, m.UserID)
			}
		}

		result = append(result, eg)
	}

	return result, nil
}

// groupImportAdapter implements campaigns.GroupImporter.
type groupImportAdapter struct {
	svc campaigns.GroupService
}

// ImportGroups creates campaign groups from import data.
// Member user IDs that don't exist on the target instance are skipped.
func (a *groupImportAdapter) ImportGroups(ctx context.Context, campaignID string, data []campaigns.ExportGroup) error {
	for _, g := range data {
		newGroup, err := a.svc.CreateGroup(ctx, campaignID, g.Name, g.Description)
		if err != nil {
			slog.Warn("import: create group failed", slog.String("name", g.Name), slog.Any("error", err))
			continue
		}

		for _, userID := range g.MemberIDs {
			if err := a.svc.AddGroupMember(ctx, newGroup.ID, userID); err != nil {
				slog.Warn("import: add group member skipped (user may not exist)",
					slog.String("group", g.Name), slog.String("user_id", userID))
			}
		}
	}
	return nil
}

// --- Post Export/Import Adapters ---

// postExportAdapter implements campaigns.PostExporter.
type postExportAdapter struct {
	postSvc   posts.PostService
	entitySvc entities.EntityService
}

// ExportPosts gathers all entity posts for a campaign. Iterates all entities
// and collects their posts.
func (a *postExportAdapter) ExportPosts(ctx context.Context, campaignID string, entitySlugLookup func(string) string) ([]campaigns.ExportPost, error) {
	// Paginate through all entities to collect their IDs.
	const ownerRole = 3
	var result []campaigns.ExportPost
	page := 1

	for {
		ents, _, err := a.entitySvc.List(ctx, campaignID, 0, ownerRole, "", entities.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, err
		}
		if len(ents) == 0 {
			break
		}

		for _, e := range ents {
			entityPosts, err := a.postSvc.ListByEntity(ctx, e.ID, true)
			if err != nil {
				continue
			}

			slug := entitySlugLookup(e.ID)
			if slug == "" {
				continue
			}

			for _, p := range entityPosts {
				result = append(result, campaigns.ExportPost{
					EntitySlug: slug,
					Name:       p.Name,
					Entry:      p.Entry,
					EntryHTML:  p.EntryHTML,
					IsPrivate:  p.IsPrivate,
					SortOrder:  p.SortOrder,
				})
			}
		}

		page++
	}

	return result, nil
}

// postImportAdapter implements campaigns.PostImporter.
type postImportAdapter struct {
	svc posts.PostService
}

// ImportPosts creates entity posts from import data, resolving entity slugs
// to new IDs via the IDMap.
func (a *postImportAdapter) ImportPosts(ctx context.Context, campaignID, userID string, data []campaigns.ExportPost, idMap *campaigns.IDMap) error {
	for _, p := range data {
		entityID, ok := idMap.EntitySlugToID[p.EntitySlug]
		if !ok {
			slog.Warn("import: post entity not found", slog.String("entity_slug", p.EntitySlug), slog.String("post", p.Name))
			continue
		}

		_, err := a.svc.Create(ctx, campaignID, entityID, userID, p.Name, posts.CreatePostRequest{
			Entry:     p.Entry,
			EntryHTML: p.EntryHTML,
			IsPrivate: p.IsPrivate,
		})
		if err != nil {
			slog.Warn("import: create post failed", slog.String("post", p.Name), slog.Any("error", err))
		}
	}
	return nil
}

// ptrString dereferences a *string or returns empty string.
func ptrString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

