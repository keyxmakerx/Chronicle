// Package app — export_adapters.go provides adapter implementations that bridge
// plugin services to the campaign export/import interfaces. Each adapter converts
// plugin-specific types to the export model types defined in campaigns/export.go.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
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
	// placeSvc carries a page's extra places in the tree; nil leaves them out.
	placeSvc entities.PlaceService
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
	}
	for _, et := range etypes {
		var parentSlug *string
		if et.ParentTypeID != nil {
			if s, ok := typeIDToSlug[*et.ParentTypeID]; ok {
				parentSlug = &s
			}
		}

		fieldsJSON, err := json.Marshal(et.Fields)
		if err != nil {
			return nil, fmt.Errorf("marshal entity type fields: %w", err)
		}
		layoutJSON, err := json.Marshal(et.Layout)
		if err != nil {
			return nil, fmt.Errorf("marshal entity type layout: %w", err)
		}

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
			PresetCategory:  et.PresetCategory,
			ParentTypeSlug:  parentSlug,
			Claimable:       et.Claimable,
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
				CoverImagePath: e.CoverImagePath,
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

	// Extra places a page is listed in the tree, by the parent's slug like the
	// real parent. Read without a viewer filter: export is the owner's copy.
	if a.placeSvc != nil {
		places, err := a.placeSvc.ExportPlaces(ctx, campaignID)
		if err != nil {
			return nil, err
		}
		slugsByEntity := make(map[string][]string)
		for _, p := range places {
			if slug, ok := entityIDToSlug[p.ParentEntityID]; ok {
				slugsByEntity[p.EntityID] = append(slugsByEntity[p.EntityID], slug)
			}
		}
		for i, e := range data.Entities {
			data.Entities[i].AlsoListedUnder = slugsByEntity[e.OriginalID]
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
		rels, err := a.relationSvc.ListByEntity(ctx, campaignID, entityID)
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
				DmOnly:              r.DmOnly,
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

// isNotFound reports whether err is an apperror.AppError carrying a 404 —
// the "no calendar" case ExportCalendar's caller (ExportImportService.Export)
// already treats as "skip this section" rather than a hard failure.
func isNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

// ExportCalendar walks one campaign calendar into campaigns.ExportCalendarData
// for a campaign backup, as a declared SYSTEM caller (ADR-049) that bypasses
// the per-user visibility layer entirely — a backup must capture dm_only
// events, hidden moons and GM-only calendars, or a restore silently loses
// content the campaign actually had.
//
// The envelope's "calendar" key stays singular for backward compatibility:
// it holds the campaign's DEFAULT calendar (the first by sort order if none
// is marked default), and any further calendars ride on its
// AdditionalCalendars. campaigns.IDMap.CalendarID still names the default.
func (a *calendarExportAdapter) ExportCalendar(ctx context.Context, campaignID string, entitySlugLookup func(string) string) (*campaigns.ExportCalendarData, error) {
	const ownerRole = 3
	systemViewer := permissions.SystemViewer(ownerRole)

	cal, err := a.svc.GetDefaultCalendarForViewer(ctx, campaignID, systemViewer)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		// No calendar is marked default — could be a campaign with zero
		// calendars (the common case, and the caller treats this NotFound as
		// "skip"), or one whose calendars predate SetDefaultCalendar ever
		// being called. Fall back to the first by sort order rather than
		// silently exporting nothing when a calendar clearly exists.
		cals, listErr := a.svc.ListCalendars(ctx, campaignID, systemViewer)
		if listErr != nil {
			return nil, listErr
		}
		if len(cals) == 0 {
			return nil, err
		}
		cal, err = a.svc.GetCalendarForViewer(ctx, cals[0].ID, campaignID, systemViewer)
		if err != nil {
			return nil, err
		}
	}

	data, err := a.exportOne(ctx, cal, campaignID, systemViewer, entitySlugLookup, true)
	if err != nil {
		return nil, err
	}

	// Every other calendar rides on the primary's AdditionalCalendars so the
	// envelope stays one singular "calendar" key that older importers still
	// read; they simply ignore the extras.
	others, err := a.svc.ListCalendars(ctx, campaignID, systemViewer)
	if err != nil {
		return nil, err
	}
	for _, o := range others {
		if o.ID == cal.ID {
			continue
		}
		full, err := a.svc.GetCalendarForViewer(ctx, o.ID, campaignID, systemViewer)
		if err != nil {
			return nil, err
		}
		extra, err := a.exportOne(ctx, full, campaignID, systemViewer, entitySlugLookup, false)
		if err != nil {
			return nil, err
		}
		data.AdditionalCalendars = append(data.AdditionalCalendars, *extra)
	}
	return data, nil
}

// exportOne walks a single calendar. Event kinds are campaign-scoped, so only
// the primary calendar (withKinds) carries them; repeating them on every
// calendar would make the import create each kind once per calendar.
func (a *calendarExportAdapter) exportOne(ctx context.Context, cal *calendar.Calendar, campaignID string, systemViewer permissions.Viewer, entitySlugLookup func(string) string, withKinds bool) (*campaigns.ExportCalendarData, error) {
	events, err := a.svc.ListAllEventsForCalendar(ctx, cal.ID, campaignID, systemViewer)
	if err != nil {
		return nil, err
	}

	// EventKind id → slug, for Event.KindID → ExportCalendarEvent.Category.
	kindSlugByID := make(map[int]string, len(cal.EventKinds))
	for _, k := range cal.EventKinds {
		kindSlugByID[k.ID] = k.Slug
	}

	data := &campaigns.ExportCalendarData{
		Ref:              cal.ID,
		Name:             cal.Name,
		Description:      cal.Description,
		Mode:             cal.Mode,
		Visibility:       cal.Visibility,
		VisibilityRules:  cal.VisibilityRules,
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
		// Additive settings on ExportCalendarData; see its doc comment.
		Hemisphere:         cal.Hemisphere,
		ForecastsEnabled:   cal.ForecastsEnabled,
		MonthStartsNewWeek: cal.MonthStartsNewWeek,
		TracksRealTime:     cal.TracksRealTime,
		RealTimeZone:       cal.RealTimeZone,
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
		// Always write a non-nil pointer: a fresh export must never itself
		// produce the "unknown, pre-V5 backup" shape ImportCalendar treats
		// as hidden — see ExportCalendarMoon's doc comment.
		hidden := m.HiddenFromPlayers
		data.Moons = append(data.Moons, campaigns.ExportCalendarMoon{
			Ref: m.ID, Name: m.Name, CycleDays: m.CycleDays, PhaseOffset: m.PhaseOffset,
			Color: m.Color, HiddenFromPlayers: &hidden,
			BaseDesign: m.BaseDesign, Tint: m.Tint, PhaseSource: m.PhaseSource,
			Size: m.Size, OrbitSpeed: m.OrbitSpeed,
		})
	}
	for _, s := range cal.Seasons {
		data.Seasons = append(data.Seasons, campaigns.ExportCalendarSeason{
			Ref: s.ID, Name: s.Name, StartMonth: s.StartMonth, StartDay: s.StartDay,
			EndMonth: s.EndMonth, EndDay: s.EndDay, Description: s.Description,
			Color: s.Color, WeatherEffect: s.WeatherEffect,
		})
	}
	data.EraLook = &campaigns.ExportCalendarEraLook{
		ColorsOn: cal.EraLook.ColorsOn, Feel: cal.EraLook.Feel,
		Intensity: cal.EraLook.Intensity, Speed: cal.EraLook.Speed,
	}
	for _, e := range cal.Eras {
		var loreSlug *string
		if e.LoreEntityID != nil {
			if s := entitySlugLookup(*e.LoreEntityID); s != "" {
				loreSlug = &s
			}
		}
		data.Eras = append(data.Eras, campaigns.ExportCalendarEra{
			Name: e.Name, StartYear: e.StartYear, StartMonth: e.StartMonth, StartDay: e.StartDay,
			EndYear: e.EndYear, EndMonth: e.EndMonth, EndDay: e.EndDay,
			Description: e.Description, Color: e.Color, SortOrder: e.SortOrder,
			Color2: e.Color2, Style: e.Style, Feel: e.Feel, LoreEntitySlug: loreSlug,
			DMNote: e.DMNote, HiddenUntilBegins: e.HiddenUntilBegins,
		})
	}
	for _, k := range cal.EventKinds {
		if !withKinds {
			break
		}
		data.EventCategories = append(data.EventCategories, campaigns.ExportEventCategory{
			Slug: k.Slug, Name: k.Name, Icon: k.Icon, Color: k.Color, SortOrder: k.SortOrder,
			DefaultAnnounced: k.DefaultAnnounced,
		})
	}

	// Cycles/Festivals/Weather: cal already carries these eager-loaded
	// sub-resources (GetCalendarForViewer / GetDefaultCalendarForViewer),
	// same as Months/Weekdays/Moons/Seasons/Eras above.
	for _, c := range cal.Cycles {
		ec := campaigns.ExportCalendarCycle{
			Name: c.Name, CycleLength: c.CycleLength, Type: c.Type, SortOrder: c.SortOrder,
		}
		for _, entry := range c.Entries {
			ec.Entries = append(ec.Entries, campaigns.ExportCalendarCycleEntry{
				Name: entry.Name, Icon: entry.Icon, YearOffset: entry.YearOffset, SortOrder: entry.SortOrder,
			})
		}
		data.Cycles = append(data.Cycles, ec)
	}
	for _, f := range cal.Festivals {
		data.Festivals = append(data.Festivals, campaigns.ExportCalendarFestival{
			Name: f.Name, Month: f.Month, Day: f.Day, AfterMonth: f.AfterMonth,
			Description: f.Description, Color: f.Color, Icon: f.Icon, SortOrder: f.SortOrder,
		})
	}
	if w := cal.Weather; w != nil {
		data.Weather = &campaigns.ExportCalendarWeather{
			PresetID: w.PresetID, PresetLabel: w.PresetLabel, Icon: w.Icon, Color: w.Color,
			TemperatureCelsius: w.TemperatureCelsius, ZoneID: w.ZoneID, ZoneName: w.ZoneName,
			Description: w.Description,
		}
		if w.Wind != nil {
			data.Weather.WindSpeedKPH = w.Wind.SpeedKPH
			data.Weather.WindSpeedTier = w.Wind.SpeedTier
			data.Weather.WindDirection = w.Wind.Direction
			data.Weather.WindDirectionDegrees = w.Wind.DirectionDegrees
		}
		if w.Precipitation != nil {
			data.Weather.PrecipitationType = w.Precipitation.Type
			data.Weather.PrecipitationIntensity = w.Precipitation.Intensity
		}
	}

	overrides, err := a.svc.ListOccurrenceOverrides(ctx, cal.ID, campaignID, systemViewer)
	if err != nil {
		return nil, err
	}

	for _, evt := range events {
		// Only an event that repeats by its rule carries one out.
		var rule json.RawMessage
		if evt.RecurrenceRule != nil && derefStr(evt.RecurrenceType) == calendar.RecurrenceByRule {
			if rule, err = json.Marshal(evt.RecurrenceRule); err != nil {
				return nil, fmt.Errorf("export recurrence rule: %w", err)
			}
		}
		var eventOverrides []campaigns.ExportCalendarEventOverride
		for _, o := range overrides[evt.ID] {
			eventOverrides = append(eventOverrides, campaigns.ExportCalendarEventOverride{
				Year: o.Year, Month: o.Month, Day: o.Day, Action: o.Action,
				NewYear: o.NewYear, NewMonth: o.NewMonth, NewDay: o.NewDay,
			})
		}
		var entitySlug *string
		if evt.EntityID != nil {
			if s := entitySlugLookup(*evt.EntityID); s != "" {
				entitySlug = &s
			}
		}
		var category *string
		if evt.KindID != nil {
			if slug, ok := kindSlugByID[*evt.KindID]; ok {
				category = &slug
			}
		}
		data.Events = append(data.Events, campaigns.ExportCalendarEvent{
			Name: evt.Name, Description: evt.Description, DescriptionHTML: evt.DescriptionHTML,
			EntitySlug: entitySlug, Year: evt.Year, Month: evt.Month, Day: evt.Day,
			StartHour: evt.StartHour, StartMinute: evt.StartMinute,
			EndYear: evt.EndYear, EndMonth: evt.EndMonth, EndDay: evt.EndDay,
			EndHour: evt.EndHour, EndMinute: evt.EndMinute,
			IsRecurring: evt.IsRecurring, RecurrenceType: evt.RecurrenceType,
			RecurrenceInterval: evt.RecurrenceInterval,
			RecurrenceEndYear:  evt.RecurrenceEndYear, RecurrenceEndMonth: evt.RecurrenceEndMonth, RecurrenceEndDay: evt.RecurrenceEndDay,
			RecurrenceMaxOccurrences: evt.RecurrenceMaxOccurrences,
			Visibility:               evt.Visibility, VisibilityRules: evt.VisibilityRules,
			Category:  category,
			Announced: evt.Announced, Tier: evt.Tier, Color: evt.Color, Icon: evt.Icon,
			AllDay: evt.AllDay, Payload: evt.Payload,
			Ref: evt.ID, RecurrenceRule: rule, Overrides: eventOverrides,
		})
	}

	return data, nil
}

// timelineExportAdapter implements campaigns.TimelineExporter.
type timelineExportAdapter struct {
	svc timeline.TimelineService
}

// ExportTimelines gathers timeline data for a campaign export.
func (a *timelineExportAdapter) ExportTimelines(ctx context.Context, campaignID string, entitySlugLookup func(string) string) ([]campaigns.ExportTimeline, error) {
	// A campaign export is a declared SYSTEM caller (ADR-049): it walks the
	// owner's own rows with no per-request identity, stated explicitly rather
	// than inferred from an empty userID.
	const ownerRole = 3
	systemViewer := permissions.SystemViewer(ownerRole)
	timelines, err := a.svc.ListTimelines(ctx, campaignID, systemViewer)
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
			CalendarRef:     tl.CalendarID,
			NoCalendar:      !tl.HasCalendar(),
		}

		// Standalone events travel here; calendar events travel in the
		// calendar section, and only the timeline's links to them here.
		// Build event ID → export index map for connection references.
		eventIDToIndex := make(map[string]int)
		events, err := a.svc.ListTimelineEvents(ctx, tl.ID, campaignID, systemViewer)
		if err == nil {
			for _, evt := range events {
				// A timeline event is either standalone or a calendar event
				// shown on it; the latter travels as a link.
				if evt.Source != "standalone" {
					et.CalendarEventLinks = append(et.CalendarEventLinks, campaigns.ExportTimelineEventLink{
						EventRef: evt.EventID, Label: evt.Label, ColorOverride: evt.ColorOverride,
						VisibilityOverride: evt.VisibilityOverride, VisibilityRules: evt.VisibilityRules,
					})
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
				out := campaigns.ExportTimelineEvent{
					Name: evt.EventName, EntitySlug: entitySlug,
					Year: evt.EventYear, Month: evt.EventMonth, Day: evt.EventDay,
					EndYear: evt.EventEndYear, EndMonth: evt.EventEndMonth, EndDay: evt.EventEndDay,
					Category: evt.EventCategory, Visibility: evt.EventVisibility,
					DisplayOrder: evt.DisplayOrder, Label: evt.Label, Color: evt.ColorOverride,
				}
				// The timeline's event list carries no text, times of day or
				// repeat; the event itself does.
				if full, err := a.svc.GetStandaloneEvent(ctx, evt.EventID); err == nil {
					out.Description, out.DescriptionHTML = full.Description, full.DescriptionHTML
					out.StartHour, out.StartMinute = full.StartHour, full.StartMinute
					out.EndHour, out.EndMinute = full.EndHour, full.EndMinute
					out.IsRecurring, out.RecurrenceType = full.IsRecurring, full.RecurrenceType
				} else {
					slog.Warn("export: timeline event details skipped", slog.String("event", evt.EventName), slog.Any("error", err))
				}
				et.Events = append(et.Events, out)
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
		groups, err := a.svc.ListEntityGroups(ctx, tl.ID, campaignID, systemViewer)
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
			Recap:              sess.Recap,
			RecapHTML:          sess.RecapHTML,
			ScheduledDate:      sess.ScheduledDate,
			ScheduledTime:      sess.ScheduledTime,
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
			Ref:         m.ID,
			Name:        m.Name,
			Description: m.Description,
			ImageID:     m.ImageID,
			ImageWidth:  m.ImageWidth,
			ImageHeight: m.ImageHeight,
			SortOrder:   m.SortOrder,
		}

		// Markers (owner role sees all).
		markers, err := a.mapSvc.ListMarkers(ctx, campaignID, m.ID, ownerRole, "")
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
					LinkedMapRef: mk.LinkedMapID,
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
			drawings, err := a.drawingSvc.ListDrawings(ctx, m.ID, ownerRole, "")
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

// --- Note Export Adapter ---

// noteExportAdapter implements campaigns.NoteExporter.
//
// Shared notes are campaign content — session recaps, party inventories,
// shared checklists — but export shipped without an exporter for them, so
// every campaign backup silently omitted the lot. Personal (unshared) notes
// stay out on purpose: they belong to a user, not to the campaign, and an
// export is handed to whoever imports it.
type noteExportAdapter struct {
	svc notes.NoteService
}

// ExportNotes gathers the campaign's shared notes. Folder membership is
// carried as an index into the returned slice, so the importer can rebuild
// the tree without depending on note IDs surviving the trip.
func (a *noteExportAdapter) ExportNotes(ctx context.Context, campaignID string, entitySlugLookup func(string) string) ([]campaigns.ExportNote, error) {
	shared, err := a.svc.ListSharedByCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	// First pass fixes each note's position so parent references can be
	// resolved to an index regardless of the order rows came back in.
	indexByID := make(map[string]int, len(shared))
	for i, n := range shared {
		indexByID[n.ID] = i
	}

	result := make([]campaigns.ExportNote, 0, len(shared))
	for _, n := range shared {
		en := campaigns.ExportNote{
			Title:     n.Title,
			Entry:     n.Entry,
			EntryHTML: n.EntryHTML,
			Color:     n.Color,
			Pinned:    n.Pinned,
			IsFolder:  n.IsFolder,
		}
		if n.EntityID != nil {
			if slug := entitySlugLookup(*n.EntityID); slug != "" {
				s := slug
				en.EntitySlug = &s
			}
		}
		if len(n.Content) > 0 {
			// Marshal failures here would mean unrepresentable blocks;
			// drop the block content rather than fail the whole export.
			if raw, err := json.Marshal(n.Content); err == nil {
				en.Content = raw
			} else {
				slog.Warn("export: note content not serializable; exporting without blocks",
					slog.String("note", n.ID), slog.Any("error", err))
			}
		}
		// A parent outside the shared set (a shared note filed under a
		// private folder) cannot be represented; the note is exported at
		// top level rather than dropped.
		if n.ParentID != nil {
			if idx, ok := indexByID[*n.ParentID]; ok {
				i := idx
				en.ParentIndex = &i
			}
		}
		result = append(result, en)
	}

	return result, nil
}

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

// mediaBundleAdapter implements campaigns.MediaBundler. Walks the same
// per-campaign listing as mediaExportAdapter, but pairs each file's
// metadata with an Open closure that the export handler can call when
// it's actually about to write the bytes into the zip — keeps the
// metadata pass cheap and the per-file IO lazy.
type mediaBundleAdapter struct {
	svc media.MediaService
}

func (a *mediaBundleAdapter) BundleMedia(ctx context.Context, campaignID string) ([]campaigns.BundledMediaFile, error) {
	var result []campaigns.BundledMediaFile
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
			f := f // capture
			path := a.svc.FilePath(&f)
			result = append(result, campaigns.BundledMediaFile{
				Filename:  f.Filename,
				SizeBytes: f.FileSize,
				MimeType:  f.MimeType,
				Open: func() (io.ReadCloser, error) {
					return os.Open(path)
				},
			})
		}
		page++
	}
	return result, nil
}

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
				Filename:     campaigns.MediaZipName(f.Filename),
			})
		}
		page++
	}

	return result, nil
}

// mediaImportAdapter implements campaigns.MediaImporter.
type mediaImportAdapter struct {
	svc media.MediaService
}

// ImportMedia uploads each manifest file the zip carries into the new
// campaign through the ordinary upload path, so it gets the same type,
// size and quota checks as any upload. Files the upload lacks (all of them,
// for a JSON upload) are reported once as a count.
func (a *mediaImportAdapter) ImportMedia(ctx context.Context, campaignID, userID string, files []campaigns.ExportMediaFile, bundle *campaigns.ImportMediaBundle, idMap *campaigns.IDMap, report *campaigns.ImportReport) error {
	missing := 0
	for _, f := range files {
		data, ok, err := bundle.Read(f)
		if !ok {
			missing++
			continue
		}
		if err != nil {
			slog.Warn("import: read media file failed", slog.String("original_id", f.OriginalID), slog.Any("error", err))
			report.Fail("media", "media file", f.OriginalName, "the file in the zip could not be read")
			continue
		}
		usage := f.UsageType
		if usage == "" {
			usage = "attachment"
		}
		newFile, err := a.svc.Upload(ctx, media.UploadInput{
			CampaignID:   campaignID,
			UploadedBy:   userID,
			OriginalName: f.OriginalName,
			MimeType:     f.MimeType,
			FileSize:     int64(len(data)),
			UsageType:    usage,
			FileBytes:    data,
		})
		if err != nil {
			slog.Warn("import: restore media file failed", slog.String("original_id", f.OriginalID), slog.Any("error", err))
			report.Fail("media", "media file", f.OriginalName, apperror.SafeMessage(err))
			continue
		}
		idMap.MediaIDs[f.OriginalID] = newFile.ID
	}
	reason := "not in the zip"
	if bundle == nil {
		reason = "a JSON export carries no picture files; import the ZIP export to restore them"
	}
	report.FailN("media", "media file", "", reason, missing)
	return nil
}

// --- Import Adapters ---

// entityImportAdapter implements campaigns.EntityImporter.
type entityImportAdapter struct {
	entitySvc   entities.EntityService
	tagSvc      tags.TagService
	relationSvc relations.RelationService
	// placeSvc restores a page's extra places in the tree; nil skips them.
	placeSvc entities.PlaceService
}

// ImportEntities creates entity types, entities, tags, and relations from
// import data. Returns an IDMap for cross-referencing by other importers.
func (a *entityImportAdapter) ImportEntities(ctx context.Context, campaignID, userID string, data *campaigns.ExportEntityData, idMap *campaigns.IDMap, report *campaigns.ImportReport) error {
	// 1. Create entity types. Add-ons are restored before this runs, and an
	// add-on can already have made some of the file's types (the Player
	// Character addon premakes its category, a game system its presets);
	// those are reused rather than duplicated. Parents go first so a
	// sub-category can name its parent's new id.
	existing, err := a.entitySvc.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("list existing entity types: %w", err)
	}
	claimed := make(map[int]bool, len(existing))
	typeSlugToNewID := make(map[string]int)
	for _, et := range importTypeOrder(data.Types) {
		var fields []entities.FieldDefinition
		fieldsOK := len(et.Fields) == 0
		if len(et.Fields) > 0 {
			if err := json.Unmarshal(et.Fields, &fields); err != nil {
				slog.Warn("import: invalid fields JSON", slog.String("type", et.Slug), slog.Any("error", err))
				report.Fail("entities", "entity type field set", et.Name, "field definitions were not readable")
			} else {
				fieldsOK = true
			}
		}
		var parentID *int
		if et.ParentTypeSlug != nil {
			if id, ok := typeSlugToNewID[*et.ParentTypeSlug]; ok {
				parentID = &id
			} else {
				report.Fail("entities", "entity type parent link", et.Name, "parent \""+*et.ParentTypeSlug+"\" is not in the file")
			}
		}

		var newType *entities.EntityType
		if match := matchExistingType(existing, claimed, et); match != nil {
			claimed[match.ID] = true
			in := entities.UpdateEntityTypeInput{
				Name: patch.Of(et.Name), NamePlural: patch.Of(et.NamePlural), Icon: patch.Of(et.Icon), Color: patch.Of(et.Color),
				ParentTypeID: parentID, ClearParent: parentID == nil, Claimable: et.Claimable,
			}
			if fieldsOK {
				in.Fields = fields
				if in.Fields == nil {
					in.Fields = []entities.FieldDefinition{}
				}
			}
			newType, err = a.entitySvc.UpdateEntityType(ctx, match.ID, in)
		} else {
			newType, err = a.entitySvc.CreateEntityType(ctx, campaignID, entities.CreateEntityTypeInput{
				Name:           et.Name,
				NamePlural:     et.NamePlural,
				Icon:           et.Icon,
				Color:          et.Color,
				PresetCategory: derefStr(et.PresetCategory),
				ParentTypeID:   parentID,
				Claimable:      et.Claimable,
				Fields:         fields,
			})
		}
		if err != nil {
			slog.Warn("import: create entity type failed", slog.String("slug", et.Slug), slog.Any("error", err))
			report.Fail("entities", "entity type", et.Name, apperror.SafeMessage(err))
			continue
		}

		idMap.EntityTypeIDs[et.OriginalID] = newType.ID
		typeSlugToNewID[et.Slug] = newType.ID

		// Apply layout if present.
		if len(et.Layout) > 0 {
			var layout entities.EntityTypeLayout
			if err := json.Unmarshal(et.Layout, &layout); err == nil {
				if err := a.entitySvc.UpdateEntityTypeLayout(ctx, newType.ID, layout); err != nil {
					slog.Warn("import: apply layout failed", slog.String("type", et.Slug), slog.Any("error", err))
					report.Fail("entities", "entity type layout", et.Name, apperror.SafeMessage(err))
				}
			}
		}
	}

	// 2. Create entities (first pass: without parent references).
	entitySlugToNewID := make(map[string]string)
	missingPictures := 0
	for _, e := range data.Entities {
		typeID, ok := typeSlugToNewID[e.EntityTypeSlug]
		if !ok {
			slog.Warn("import: unknown entity type", slog.String("slug", e.EntityTypeSlug))
			report.Fail("entities", "entity", e.Name, "entity type \""+e.EntityTypeSlug+"\" is not in the file")
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
			report.Fail("entities", "entity", e.Name, apperror.SafeMessage(err))
			continue
		}

		idMap.EntityIDs[e.OriginalID] = newEntity.ID
		idMap.EntitySlugToID[e.Slug] = newEntity.ID
		entitySlugToNewID[e.Slug] = newEntity.ID

		// Create already wrote the name, label, privacy and fields; the
		// page text and pictures each have their own writer.
		if err := a.restoreEntityBody(ctx, newEntity.ID, e); err != nil {
			slog.Warn("import: restore entity text failed", slog.String("entity", e.Name), slog.Any("error", err))
			report.Fail("entities", "entity body", e.Name, apperror.SafeMessage(err))
		}
		missingPictures += a.restoreEntityPicture(ctx, newEntity.ID, e.Name, e.ImagePath, idMap, a.entitySvc.UpdateImage, report)
		missingPictures += a.restoreEntityPicture(ctx, newEntity.ID, e.Name, e.CoverImagePath, idMap, a.entitySvc.UpdateCoverImage, report)

		// Apply field overrides.
		if len(e.FieldOverrides) > 0 {
			var overrides entities.FieldOverrides
			if err := json.Unmarshal(e.FieldOverrides, &overrides); err == nil {
				if err := a.entitySvc.UpdateFieldOverrides(ctx, newEntity.ID, &overrides); err != nil {
					slog.Warn("import: apply field overrides failed", slog.String("entity", e.Name), slog.Any("error", err))
					report.Fail("entities", "entity field override", e.Name, apperror.SafeMessage(err))
				}
			}
		}

		// Apply popup config.
		if len(e.PopupConfig) > 0 {
			var config entities.PopupConfig
			if err := json.Unmarshal(e.PopupConfig, &config); err == nil {
				if err := a.entitySvc.UpdatePopupConfig(ctx, newEntity.ID, &config); err != nil {
					slog.Warn("import: apply popup config failed", slog.String("entity", e.Name), slog.Any("error", err))
					report.Fail("entities", "entity popup config", e.Name, apperror.SafeMessage(err))
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
				report.Fail("entities", "entity permission set", e.Name, apperror.SafeMessage(err))
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
			report.Fail("entities", "entity parent link", e.Name, "parent \""+*e.ParentSlug+"\" is not in the file")
			continue
		}

		// A partial update: only the parent changes, so the text and
		// pictures restored in the first pass are kept as they are.
		_, err := a.entitySvc.Update(ctx, entityNewID, entities.UpdateEntityInput{
			ParentID: patch.Of(parentNewID),
		})
		if err != nil {
			slog.Warn("import: set parent failed", slog.String("entity", e.Name), slog.Any("error", err))
			report.Fail("entities", "entity parent link", e.Name, apperror.SafeMessage(err))
		}
	}

	// 2c. Extra places in the tree. They go in after every parent is set so the
	// cycle rule sees the finished tree; one that is refused is reported, not
	// fatal, and the page itself is already imported.
	if a.placeSvc != nil {
		for _, e := range data.Entities {
			entityNewID, ok := entitySlugToNewID[e.Slug]
			if !ok {
				continue
			}
			for _, parentSlug := range e.AlsoListedUnder {
				parentNewID, ok := entitySlugToNewID[parentSlug]
				if !ok {
					report.Fail("entities", "extra place in the page tree", e.Name, "parent \""+parentSlug+"\" is not in the file")
					continue
				}
				if err := a.placeSvc.AddPlace(ctx, campaignID, entityNewID, parentNewID, ""); err != nil {
					slog.Warn("import: add extra place failed", slog.String("entity", e.Name), slog.Any("error", err))
					report.Fail("entities", "extra place in the page tree", e.Name, apperror.SafeMessage(err))
				}
			}
		}
	}

	report.FailN("entities", "page picture", "",
		"its picture file was not in the upload; import the ZIP export to keep pictures", missingPictures)

	// 2c. Category descriptions and pinned pages. Pins name pages by id, so
	// they wait until every page has its new one; a pin whose page did not
	// come back is dropped.
	for _, et := range data.Types {
		typeID, ok := idMap.EntityTypeIDs[et.OriginalID]
		if !ok || (et.Description == nil && len(et.PinnedEntityIDs) == 0) {
			continue
		}
		pinned := make([]string, 0, len(et.PinnedEntityIDs))
		for _, old := range et.PinnedEntityIDs {
			if id, ok := idMap.EntityIDs[old]; ok {
				pinned = append(pinned, id)
			}
		}
		if err := a.entitySvc.UpdateEntityTypeDashboard(ctx, typeID, et.Description, pinned); err != nil {
			slog.Warn("import: apply category dashboard failed", slog.String("type", et.Slug), slog.Any("error", err))
			report.Fail("entities", "category description", et.Name, apperror.SafeMessage(err))
		}
	}

	// 3. Create tags.
	tagSlugToNewID := make(map[string]int)
	for _, t := range data.Tags {
		newTag, err := a.tagSvc.Create(ctx, campaignID, t.Name, t.Color, t.DmOnly)
		if err != nil {
			slog.Warn("import: create tag failed", slog.String("name", t.Name), slog.Any("error", err))
			report.Fail("entities", "tag", t.Name, apperror.SafeMessage(err))
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
			r.RelationType, r.ReverseRelationType, userID, r.Metadata, r.DmOnly)
		if err != nil {
			slog.Warn("import: create relation failed",
				slog.String("source", r.SourceEntitySlug),
				slog.String("target", r.TargetEntitySlug),
				slog.Any("error", err))
			report.Fail("entities", "relation", r.SourceEntitySlug+" \u2192 "+r.TargetEntitySlug, apperror.SafeMessage(err))
		}
	}

	return nil
}

// restoreEntityBody writes a page's text as the export had it. Editor
// pages carry the editor document and its HTML, saved together the way the
// editor saves them; a page written as plain HTML (the sync API's pages)
// has no editor document, and its HTML is the body.
func (a *entityImportAdapter) restoreEntityBody(ctx context.Context, entityID string, e campaigns.ExportEntity) error {
	doc, html := ptrString(e.Entry), ptrString(e.EntryHTML)
	switch {
	case strings.TrimSpace(doc) != "" && strings.TrimSpace(html) != "":
		return a.entitySvc.UpdateEntry(ctx, entityID, doc, html)
	case strings.TrimSpace(doc) != "":
		_, err := a.entitySvc.Update(ctx, entityID, entities.UpdateEntityInput{Entry: patch.Of(doc)})
		return err
	case strings.TrimSpace(html) != "":
		_, err := a.entitySvc.Update(ctx, entityID, entities.UpdateEntityInput{Entry: patch.Of(html)})
		return err
	}
	return nil
}

// restoreEntityPicture sets a page's portrait or cover through its own
// writer, which accepts only a file in the page's campaign. A picture whose
// file was not restored is not set and is counted (returns 1) so the report
// can say so once rather than once per page.
func (a *entityImportAdapter) restoreEntityPicture(ctx context.Context, entityID, name string, path *string, idMap *campaigns.IDMap,
	set func(ctx context.Context, entityID, mediaID string) error, report *campaigns.ImportReport) int {
	if path == nil || *path == "" {
		return 0
	}
	id, ok := restoredMediaID(idMap, *path)
	if !ok {
		return 1
	}
	if err := set(ctx, entityID, id); err != nil {
		slog.Warn("import: set entity picture failed", slog.String("entity", name), slog.Any("error", err))
		report.Fail("entities", "page picture", name, apperror.SafeMessage(err))
	}
	return 0
}

// restoredMediaID returns the media id a picture reference names when that
// file was restored into the new campaign by this import. Anything else
// (a file the upload did not carry, or an id from some other campaign) is
// not ok, so an import can never point a picture at a file it does not own.
func restoredMediaID(idMap *campaigns.IDMap, path string) (string, bool) {
	id := mediaIDFromPath(path)
	for _, newID := range idMap.MediaIDs {
		if newID == id {
			return id, true
		}
	}
	return "", false
}

// mediaIDFromPath reduces a stored picture reference to its media id: older
// rows hold the file's path ("2026/03/<id>.jpg"), newer ones the bare id.
func mediaIDFromPath(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
		if j := strings.LastIndex(p, "."); j > 0 {
			p = p[:j]
		}
	}
	return p
}

// importTypeOrder returns the file's types with every top-level type ahead
// of the sub-categories, keeping the file's order within each group, so a
// sub-category's parent already exists when it is created.
func importTypeOrder(types []campaigns.ExportEntityType) []campaigns.ExportEntityType {
	out := make([]campaigns.ExportEntityType, 0, len(types))
	for _, et := range types {
		if et.ParentTypeSlug == nil {
			out = append(out, et)
		}
	}
	for _, et := range types {
		if et.ParentTypeSlug != nil {
			out = append(out, et)
		}
	}
	return out
}

// importIsPlayerCharacterType is the entities plugin's rule for the Player
// Character category (preset player_character, or the premade slug), applied
// to a type from an export file.
func importIsPlayerCharacterType(preset, slug string) bool {
	return preset == entities.PresetCategoryPlayerCharacter || slug == entities.SlugPlayerCharacter
}

// matchExistingType finds a type already in the importing campaign that the
// file's type should become: the Player Character category the addon
// premade (a campaign has only one), else an unclaimed type with the same
// slug. A type made by a game-system addon carries the slug the exporting
// campaign's copy had, since both were named by the same preset.
func matchExistingType(existing []entities.EntityType, claimed map[int]bool, et campaigns.ExportEntityType) *entities.EntityType {
	wantPC := importIsPlayerCharacterType(derefStr(et.PresetCategory), et.Slug)
	for i := range existing {
		t := &existing[i]
		if claimed[t.ID] {
			continue
		}
		if wantPC && importIsPlayerCharacterType(derefStr(t.PresetCategory), t.Slug) {
			return t
		}
	}
	for i := range existing {
		t := &existing[i]
		if !claimed[t.ID] && t.Slug == et.Slug {
			return t
		}
	}
	return nil
}

// calendarImportAdapter implements campaigns.CalendarImporter, the inverse of
// calendarExportAdapter.
type calendarImportAdapter struct {
	svc calendar.CalendarService
}

// ImportCalendar rebuilds one calendar (and its months/weekdays/moons/
// seasons/eras/event kinds/events) from data into a freshly-created campaign,
// re-linking entity ties through idMap. Best-effort: a bad sub-resource is
// reported via report.Fail and skipped rather than aborting the whole import
// (see ExportImportService.Import's doc comment) — a partially-restored
// calendar beats none.
//
// data.EndMonth/EndDay-less eras and CreateEventKind's own icon-shape
// validation are the two places a PRE-V5 backup's calendar section is most
// likely to partially fail: V4's ExportCalendarEra carried year-only
// boundaries (no month/day — see ExportCalendarEra's doc comment) and V4's
// default event kinds used emoji icons, which V5's CreateEventKind rejects
// (Font Awesome class names only, enforced at creation, not merely at
// display). Both degrade gracefully here — an era gets month/day defaults
// (see eraStartMonthDay below), and an event whose category's kind failed to
// import still gets created, just with no KindID — rather than losing the
// era or the event outright.
func (a *calendarImportAdapter) ImportCalendar(ctx context.Context, campaignID string, data *campaigns.ExportCalendarData, idMap *campaigns.IDMap, report *campaigns.ImportReport) error {
	kindIDBySlug, err := a.importOne(ctx, campaignID, data, idMap, report, true, nil)
	if err != nil {
		return err
	}
	// Extras are best-effort: a failing one is reported and the rest still
	// import, so one bad calendar never costs the others.
	for i := range data.AdditionalCalendars {
		extra := &data.AdditionalCalendars[i]
		if _, err := a.importOne(ctx, campaignID, extra, idMap, report, false, kindIDBySlug); err != nil {
			slog.Warn("import: additional calendar failed", slog.String("name", extra.Name), slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, campaigns.KindCalendar, extra.Name, apperror.SafeMessage(err))
		}
	}
	return nil
}

// importOne rebuilds a single calendar. isPrimary controls the things that
// are per-campaign rather than per-calendar: only the primary becomes the
// default and idMap.CalendarID (timelines bind to it), and only it creates
// the campaign-scoped event kinds, which sharedKinds hands to later calendars
// so their events still resolve a category. It returns the slug-to-kind map.
func (a *calendarImportAdapter) importOne(ctx context.Context, campaignID string, data *campaigns.ExportCalendarData, idMap *campaigns.IDMap, report *campaigns.ImportReport, isPrimary bool, sharedKinds map[string]int) (map[string]int, error) {
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

	cal, err := a.svc.CreateCalendar(ctx, campaignID, calendar.CreateCalendarInput{
		Name:             data.Name,
		Description:      data.Description,
		Mode:             data.Mode,
		EpochName:        data.EpochName,
		CurrentYear:      data.CurrentYear,
		HoursPerDay:      data.HoursPerDay,
		MinutesPerHour:   data.MinutesPerHour,
		SecondsPerMinute: data.SecondsPerMinute,
		LeapYearEvery:    data.LeapYearEvery,
		LeapYearOffset:   data.LeapYearOffset,
		Visibility:       importCalendarVisibility(data.Visibility),
		VisibilityRules:  data.VisibilityRules,
	})
	if err != nil {
		return nil, fmt.Errorf("create calendar: %w", err)
	}
	if isPrimary {
		idMap.CalendarID = cal.ID
	}
	if data.Ref != "" {
		idMap.CalendarIDs[data.Ref] = cal.ID
	}

	// The primary is the campaign's first calendar, so it becomes the default — every default-calendar
	// reader (GetDefaultCalendarForViewer, the skybox/dashboard/category
	// blocks) needs one marked, and CreateCalendar itself never sets it.
	if isPrimary {
		if err := a.svc.SetDefaultCalendar(ctx, campaignID, cal.ID); err != nil {
			slog.Warn("import: set default calendar failed", slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, campaigns.KindCalendar, data.Name, apperror.SafeMessage(err))
		}
	}

	if len(months) > 0 {
		if err := a.svc.SetMonths(ctx, cal.ID, campaignID, months); err != nil {
			slog.Warn("import: set months failed", slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, "months", data.Name, apperror.SafeMessage(err))
		}
	}
	if len(weekdays) > 0 {
		if err := a.svc.SetWeekdays(ctx, cal.ID, campaignID, weekdays); err != nil {
			slog.Warn("import: set weekdays failed", slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, "weekdays", data.Name, apperror.SafeMessage(err))
		}
	}
	if len(data.Moons) > 0 {
		moons := make([]calendar.MoonInput, len(data.Moons))
		for i, m := range data.Moons {
			moons[i] = calendar.MoonInput{
				Name: m.Name, CycleDays: m.CycleDays, PhaseOffset: m.PhaseOffset,
				Color: m.Color, HiddenFromPlayers: importMoonHidden(m.HiddenFromPlayers),
				// Render params: zero-valued when absent from the import data.
				// upsertMoons' own insert-time fallback treats that the same
				// way it would a hand-created moon's zero value — never an
				// empty/invalid look.
				BaseDesign: m.BaseDesign, Tint: m.Tint, PhaseSource: m.PhaseSource,
				Size: m.Size, OrbitSpeed: m.OrbitSpeed,
			}
		}
		if err := a.svc.SetMoons(ctx, cal.ID, campaignID, moons); err != nil {
			slog.Warn("import: set moons failed", slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, "moons", data.Name, apperror.SafeMessage(err))
		}
	}
	if len(data.Seasons) > 0 {
		seasons := make([]calendar.Season, len(data.Seasons))
		for i, s := range data.Seasons {
			seasons[i] = calendar.Season{
				Name: s.Name, StartMonth: s.StartMonth, StartDay: s.StartDay,
				EndMonth: s.EndMonth, EndDay: s.EndDay, Description: s.Description,
				Color: s.Color, WeatherEffect: s.WeatherEffect,
			}
		}
		if err := a.svc.SetSeasons(ctx, cal.ID, campaignID, seasons); err != nil {
			slog.Warn("import: set seasons failed", slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, "seasons", data.Name, apperror.SafeMessage(err))
		}
	}

	// Cycles/Festivals/Weather: same bulk-replace shape as
	// months/weekdays/moons/seasons above.
	if len(data.Cycles) > 0 {
		cycles := make([]calendar.CycleInput, len(data.Cycles))
		for i, c := range data.Cycles {
			ci := calendar.CycleInput{Name: c.Name, CycleLength: c.CycleLength, Type: c.Type, SortOrder: c.SortOrder}
			for _, entry := range c.Entries {
				ci.Entries = append(ci.Entries, calendar.CycleEntryInput{
					Name: entry.Name, Icon: entry.Icon, YearOffset: entry.YearOffset, SortOrder: entry.SortOrder,
				})
			}
			cycles[i] = ci
		}
		if err := a.svc.SetCycles(ctx, cal.ID, campaignID, cycles); err != nil {
			slog.Warn("import: set cycles failed", slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, "cycles", data.Name, apperror.SafeMessage(err))
		}
	}
	if len(data.Festivals) > 0 {
		festivals := make([]calendar.FestivalInput, len(data.Festivals))
		for i, f := range data.Festivals {
			festivals[i] = calendar.FestivalInput{
				Name: f.Name, Month: f.Month, Day: f.Day, AfterMonth: f.AfterMonth,
				Description: f.Description, Color: f.Color, Icon: f.Icon, SortOrder: f.SortOrder,
			}
		}
		if err := a.svc.SetFestivals(ctx, cal.ID, campaignID, festivals); err != nil {
			slog.Warn("import: set festivals failed", slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, "festivals", data.Name, apperror.SafeMessage(err))
		}
	}
	if w := data.Weather; w != nil {
		input := calendar.WeatherInput{
			PresetID: w.PresetID, PresetLabel: w.PresetLabel, Icon: w.Icon, Color: w.Color,
			TemperatureCelsius: w.TemperatureCelsius,
			WindSpeedKPH:       w.WindSpeedKPH, WindSpeedTier: w.WindSpeedTier,
			WindDirection: w.WindDirection, WindDirectionDeg: w.WindDirectionDegrees,
			PrecipitationType: w.PrecipitationType, PrecipitationIntensity: w.PrecipitationIntensity,
			ZoneID: w.ZoneID, ZoneName: w.ZoneName, Description: w.Description,
		}
		if err := a.svc.SetWeather(ctx, cal.ID, campaignID, input); err != nil {
			slog.Warn("import: set weather failed", slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, "weather", data.Name, apperror.SafeMessage(err))
		}
	}

	// Eras, one CreateEra call each — the only way to write eras this
	// service exposes (no bulk SetEras; see CalendarService's doc comment).
	for _, e := range data.Eras {
		startMonth, startDay := eraStartMonthDay(e.StartMonth, e.StartDay)
		var endMonth, endDay *int
		if e.EndYear != nil {
			endMonth, endDay = e.EndMonth, e.EndDay
			if endMonth == nil {
				m := 12
				endMonth = &m
			}
			if endDay == nil {
				d := 31
				endDay = &d
			}
		}
		var loreID *string
		if e.LoreEntitySlug != nil {
			if id, ok := idMap.EntitySlugToID[*e.LoreEntitySlug]; ok {
				loreID = &id
			}
		}
		if _, err := a.svc.CreateEra(ctx, cal.ID, campaignID, calendar.EraInput{
			Name: e.Name, StartYear: e.StartYear, StartMonth: startMonth, StartDay: startDay,
			EndYear: e.EndYear, EndMonth: endMonth, EndDay: endDay,
			Description: e.Description, Color: e.Color, SortOrder: e.SortOrder,
			Color2: e.Color2, Style: e.Style, Feel: e.Feel, LoreEntityID: loreID,
			DMNote: e.DMNote, HiddenUntilBegins: e.HiddenUntilBegins,
		}); err != nil {
			slog.Warn("import: create era failed", slog.String("name", e.Name), slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, "era", e.Name, apperror.SafeMessage(err))
		}
	}
	if l := data.EraLook; l != nil {
		if err := a.svc.SaveEraLook(ctx, cal.ID, campaignID, calendar.EraLook{
			ColorsOn: l.ColorsOn, Feel: l.Feel, Intensity: l.Intensity, Speed: l.Speed,
		}, nil); err != nil {
			slog.Warn("import: set era look failed", slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, campaigns.KindCalendar, data.Name, apperror.SafeMessage(err))
		}
	}

	// Event kinds (categories), by slug — an event referencing a slug whose
	// kind failed to import (see the icon-validation note above) is still
	// created below, just with no KindID.
	kindIDBySlug := make(map[string]int, len(data.EventCategories)+len(sharedKinds))
	for slug, id := range sharedKinds {
		kindIDBySlug[slug] = id
	}
	for _, c := range data.EventCategories {
		kind, err := a.svc.CreateEventKind(ctx, campaignID, calendar.EventKindInput{
			Slug: c.Slug, Name: c.Name, Icon: c.Icon, Color: c.Color, SortOrder: c.SortOrder,
			DefaultAnnounced: c.DefaultAnnounced,
		})
		if err != nil {
			slog.Warn("import: create event kind failed", slog.String("slug", c.Slug), slog.Any("error", err))
			report.Fail(campaigns.SectionCalendar, "event category", c.Name, apperror.SafeMessage(err))
			continue
		}
		kindIDBySlug[c.Slug] = kind.ID
	}

	// Set current date/time (CreateCalendar only takes CurrentYear) plus the
	// settings CreateCalendarInput has no field for: Hemisphere/
	// ForecastsEnabled/MonthStartsNewWeek. A full restore of an already-
	// exported row, so every field is sent PRESENT — the same reasoning the
	// entity/session importers already use elsewhere in this file (an
	// absent value in the source really does mean "unset"). Real-time
	// tracking is switched on only when the backup carries a zone this
	// server knows; otherwise the fresh calendar's off-by-default state
	// stands, so a bad zone can't also lose the date and settings above.
	var setRealTime *bool
	if data.TracksRealTime && data.RealTimeZone != nil && *data.RealTimeZone != "" {
		if _, err := time.LoadLocation(*data.RealTimeZone); err == nil {
			on := true
			setRealTime = &on
		} else {
			report.Fail(campaigns.SectionCalendar, "real-time setting", data.Name, "its time zone is not one this server knows")
		}
	}
	if err := a.svc.UpdateCalendar(ctx, cal.ID, campaignID, calendar.UpdateCalendarInput{
		Name:               data.Name,
		CurrentMonth:       patch.Of(data.CurrentMonth),
		CurrentDay:         patch.Of(data.CurrentDay),
		CurrentHour:        patch.Of(data.CurrentHour),
		CurrentMinute:      patch.Of(data.CurrentMinute),
		Hemisphere:         patch.FromPtr(data.Hemisphere),
		ForecastsEnabled:   patch.Of(data.ForecastsEnabled),
		MonthStartsNewWeek: patch.Of(data.MonthStartsNewWeek),
		SetRealTime:        setRealTime,
		RealTimeZone:       data.RealTimeZone,
	}); err != nil {
		slog.Warn("import: set current date failed", slog.Any("error", err))
		report.Fail(campaigns.SectionCalendar, campaigns.KindCalendar, data.Name, apperror.SafeMessage(err))
	}

	// Events. CanAuthorDmOnly is forced true: this is a Director-initiated
	// campaign restore (the export/import endpoints are Owner-only end to
	// end), rebuilding events the campaign already had, including dm_only
	// ones — the same trust level ExportCalendar itself was read under.
	//
	// Repeat rules name moons, seasons and other events by their exporting
	// ids, so those are mapped to the new ones. Events another event repeats
	// relative to are created first (an anchor never itself repeats relative
	// to another, so two passes always suffice), and skips and moves go on
	// last, once every event they depend on exists.
	refs := calendarRuleRefs{events: map[string]string{}}
	if calendarDataHasRules(data) {
		refs = a.ruleRefsFor(ctx, cal.ID, campaignID, data)
	}
	created := make([]string, len(data.Events))
	for pass := 0; pass < 2; pass++ {
		for i, evt := range data.Events {
			if ruleHasAnchor(evt) != (pass == 1) {
				continue
			}
			created[i] = a.importEvent(ctx, cal.ID, campaignID, evt, idMap, kindIDBySlug, refs, report)
			if created[i] != "" && evt.Ref != "" {
				refs.events[evt.Ref] = created[i]
				idMap.CalendarEventIDs[evt.Ref] = created[i]
			}
		}
	}
	for i, evt := range data.Events {
		if created[i] != "" && len(evt.Overrides) > 0 {
			a.replayOverrides(ctx, cal.ID, campaignID, created[i], evt, report)
		}
	}

	return kindIDBySlug, nil
}

// replayOverrides re-applies one event's skips and moves. The export lists
// them by date, which is not an order they can always be written in: moving
// D1 onto D2 is refused while D2 still holds its own occurrence, so a
// series where D2 was first moved away (or skipped) must have that written
// first. Skips go first, then moves, and a move refused as a conflict is
// retried after the others until a round makes no progress; only what is
// still refused then is reported.
func (a *calendarImportAdapter) replayOverrides(ctx context.Context, calendarID, campaignID, eventID string, evt campaigns.ExportCalendarEvent, report *campaigns.ImportReport) {
	systemViewer := permissions.SystemViewer(3)
	apply := func(o campaigns.ExportCalendarEventOverride) error {
		input := calendar.OccurrenceOverrideInput{Action: o.Action}
		if o.NewYear != nil && o.NewMonth != nil && o.NewDay != nil {
			input.Year, input.Month, input.Day = *o.NewYear, *o.NewMonth, *o.NewDay
		}
		occ := calendar.DayDate{Year: o.Year, Month: o.Month, Day: o.Day}
		_, err := a.svc.SetOccurrenceOverride(ctx, eventID, calendarID, campaignID, occ, input, systemViewer)
		return err
	}
	fail := func(err error) {
		slog.Warn("import: occurrence override failed", slog.String("event", evt.Name), slog.Any("error", err))
		report.Fail(campaigns.SectionCalendar, "calendar event occurrence", evt.Name, apperror.SafeMessage(err))
	}

	var pending []campaigns.ExportCalendarEventOverride
	for _, o := range evt.Overrides {
		if o.Action == calendar.OverrideMove {
			pending = append(pending, o)
			continue
		}
		if err := apply(o); err != nil {
			fail(err)
		}
	}
	lastErr := map[int]error{}
	for len(pending) > 0 {
		var retry []campaigns.ExportCalendarEventOverride
		for _, o := range pending {
			err := apply(o)
			switch {
			case err == nil:
			case apperror.SafeCode(err) == http.StatusConflict:
				lastErr[len(retry)] = err
				retry = append(retry, o)
			default:
				fail(err)
			}
		}
		if len(retry) == len(pending) {
			for i := range retry {
				fail(lastErr[i])
			}
			return
		}
		pending = retry
		lastErr = map[int]error{}
	}
}

// calendarRuleRefs maps an export's ids (moons, seasons, events) to the ones
// the import created, for repeat rules that name them.
type calendarRuleRefs struct {
	moons   map[int]int
	seasons map[int]int
	events  map[string]string
}

// calendarDataHasRules reports whether any event carries a repeat rule.
func calendarDataHasRules(data *campaigns.ExportCalendarData) bool {
	for _, e := range data.Events {
		if len(e.RecurrenceRule) > 0 {
			return true
		}
	}
	return false
}

// ruleHasAnchor reports whether an exported event repeats relative to
// another event. A rule on an event of any other type is never used, so it
// never orders the import; an unreadable rule counts as anchor-free and
// importEvent reports it.
func ruleHasAnchor(evt campaigns.ExportCalendarEvent) bool {
	if len(evt.RecurrenceRule) == 0 || derefStr(evt.RecurrenceType) != calendar.RecurrenceByRule {
		return false
	}
	var r calendar.RecurrenceRule
	if err := json.Unmarshal(evt.RecurrenceRule, &r); err != nil {
		return false
	}
	for _, c := range r.Match {
		if c.Kind == calendar.RuleAfterEvent || c.Kind == calendar.RuleRelativeToEvent {
			return true
		}
	}
	return false
}

// derefStr is *s, or "" for nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ruleRefsFor pairs the export's moons and seasons with the ones just
// created, in order: SetMoons/SetSeasons insert in list order, so the new
// rows sorted by id line up with the export's. A count mismatch (a failed
// set) leaves that map empty and a rule naming one then fails its own
// create, reported per event.
func (a *calendarImportAdapter) ruleRefsFor(ctx context.Context, calendarID, campaignID string, data *campaigns.ExportCalendarData) calendarRuleRefs {
	refs := calendarRuleRefs{moons: map[int]int{}, seasons: map[int]int{}, events: map[string]string{}}
	cal, err := a.svc.GetCalendarForViewer(ctx, calendarID, campaignID, permissions.SystemViewer(3))
	if err != nil {
		slog.Warn("import: reading new calendar for rule references failed", slog.Any("error", err))
		return refs
	}
	moons := append([]calendar.Moon(nil), cal.Moons...)
	sort.Slice(moons, func(i, j int) bool { return moons[i].ID < moons[j].ID })
	if len(moons) == len(data.Moons) {
		for i, m := range data.Moons {
			if m.Ref != 0 {
				refs.moons[m.Ref] = moons[i].ID
			}
		}
	}
	seasons := append([]calendar.Season(nil), cal.Seasons...)
	sort.Slice(seasons, func(i, j int) bool { return seasons[i].ID < seasons[j].ID })
	if len(seasons) == len(data.Seasons) {
		for i, s := range data.Seasons {
			if s.Ref != 0 {
				refs.seasons[s.Ref] = seasons[i].ID
			}
		}
	}
	return refs
}

// remapRule rewrites a stored rule's moon, season and event ids to the
// import's. An id with no mapping is left as is, so the create refuses it
// and the event is reported rather than repeating on the wrong thing.
func remapRule(raw json.RawMessage, refs calendarRuleRefs) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var r calendar.RecurrenceRule
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	for i := range r.Match {
		c := &r.Match[i]
		if id, ok := refs.moons[c.MoonID]; ok && c.MoonID != 0 {
			c.MoonID = id
		}
		if id, ok := refs.seasons[c.SeasonID]; ok && c.SeasonID != 0 {
			c.SeasonID = id
		}
		if id, ok := refs.events[c.EventID]; ok && c.EventID != "" {
			c.EventID = id
		}
	}
	return json.Marshal(r)
}

// importEvent creates one exported event and returns its new id, or "" when
// it failed (reported).
func (a *calendarImportAdapter) importEvent(ctx context.Context, calendarID, campaignID string, evt campaigns.ExportCalendarEvent, idMap *campaigns.IDMap, kindIDBySlug map[string]int, refs calendarRuleRefs, report *campaigns.ImportReport) string {
	var entityID *string
	if evt.EntitySlug != nil {
		if id, ok := idMap.EntitySlugToID[*evt.EntitySlug]; ok {
			entityID = &id
		}
	}
	var kindID *int
	if evt.Category != nil {
		if id, ok := kindIDBySlug[*evt.Category]; ok {
			kindID = &id
		}
	}
	// A rule left on an event of another type (an older export) is not
	// sent: the create would drop it anyway, and an unreadable one must not
	// cost the event.
	var rule json.RawMessage
	var err error
	if derefStr(evt.RecurrenceType) == calendar.RecurrenceByRule {
		rule, err = remapRule(evt.RecurrenceRule, refs)
	}
	if err != nil {
		report.Fail(campaigns.SectionCalendar, "calendar event", evt.Name, "unreadable repeat rule")
		return ""
	}
	created, err := a.svc.CreateEvent(ctx, calendarID, campaignID, calendar.CreateEventInput{
		Name:                     evt.Name,
		Description:              evt.Description,
		DescriptionHTML:          evt.DescriptionHTML,
		EntityID:                 entityID,
		Year:                     evt.Year,
		Month:                    evt.Month,
		Day:                      evt.Day,
		StartHour:                evt.StartHour,
		StartMinute:              evt.StartMinute,
		EndYear:                  evt.EndYear,
		EndMonth:                 evt.EndMonth,
		EndDay:                   evt.EndDay,
		EndHour:                  evt.EndHour,
		EndMinute:                evt.EndMinute,
		IsRecurring:              evt.IsRecurring,
		RecurrenceType:           evt.RecurrenceType,
		RecurrenceInterval:       evt.RecurrenceInterval,
		RecurrenceEndYear:        evt.RecurrenceEndYear,
		RecurrenceEndMonth:       evt.RecurrenceEndMonth,
		RecurrenceEndDay:         evt.RecurrenceEndDay,
		RecurrenceMaxOccurrences: evt.RecurrenceMaxOccurrences,
		RecurrenceRule:           rule,
		Visibility:               evt.Visibility,
		VisibilityRules:          evt.VisibilityRules,
		KindID:                   kindID,
		Announced:                evt.Announced,
		Tier:                     evt.Tier,
		Color:                    evt.Color,
		Icon:                     evt.Icon,
		AllDay:                   evt.AllDay,
		Payload:                  evt.Payload,
		CanAuthorDmOnly:          true,
		Author:                   permissions.SystemViewer(3),
	})
	if err != nil {
		slog.Warn("import: create calendar event failed", slog.String("name", evt.Name), slog.Any("error", err))
		report.Fail(campaigns.SectionCalendar, "calendar event", evt.Name, apperror.SafeMessage(err))
		return ""
	}
	return created.ID
}

// eraStartMonthDay resolves an imported era's start month/day: a modern
// export always carries real values (validated 1-12 / 1-31 at creation, see
// validateEraShape), so 0 only appears from a PRE-V5 backup whose
// ExportCalendarEra never had these fields (see its doc comment) — read as
// "day-level precision unknown" and defaulted to the 1st of the start year,
// the same "whole year" reading Era.ContainsDate gives an end year with no
// end month/day.
func eraStartMonthDay(month, day int) (int, int) {
	if month == 0 {
		month = 1
	}
	if day == 0 {
		day = 1
	}
	return month, day
}

// importCalendarVisibility resolves an imported calendar's visibility,
// never trusting the JSON blindly. A recognized value ("everyone" or
// "dm_only") passes through unchanged. Anything else — including "" from a
// PRE-V5 backup, which never had this field (see ExportCalendarData's doc
// comment) — fails toward privacy rather than toward exposure: a calendar
// that was actually dm_only before the backup must never come back as
// "everyone" just because the backup predates this field or was corrupted,
// so an unknown value defaults to dm_only, not everyone.
func importCalendarVisibility(v string) string {
	switch v {
	case "everyone", "dm_only":
		return v
	default:
		return "dm_only"
	}
}

// importMoonHidden resolves an imported moon's HiddenFromPlayers, mirroring
// importCalendarVisibility's reasoning one field down: a nil pointer means
// the backup predates ExportCalendarMoon.HiddenFromPlayers (V5, #778) and
// never recorded whether the Director had hidden this moon, so it must fail
// toward privacy rather than toward exposure — an unknown value imports as
// hidden, never visible, so restoring an old backup can never un-hide a moon
// on its own.
func importMoonHidden(hidden *bool) bool {
	if hidden == nil {
		return true
	}
	return *hidden
}

// sessionImportAdapter implements campaigns.SessionImporter.
type sessionImportAdapter struct {
	svc sessions.SessionService
}

// ImportSessions creates sessions from import data.
func (a *sessionImportAdapter) ImportSessions(ctx context.Context, campaignID, userID string, data []campaigns.ExportSession, idMap *campaigns.IDMap, report *campaigns.ImportReport) error {
	for _, sess := range data {
		newSession, err := a.svc.CreateSession(ctx, campaignID, sessions.CreateSessionInput{
			Name:               sess.Name,
			ScheduledDate:      sess.ScheduledDate,
			ScheduledTime:      sess.ScheduledTime,
			CalendarYear:       sess.CalendarYear,
			CalendarMonth:      sess.CalendarMonth,
			CalendarDay:        sess.CalendarDay,
			IsRecurring:        sess.IsRecurring,
			RecurrenceType:     sess.RecurrenceType,
			RecurrenceInterval: sess.RecurrenceInterval,
			CreatedBy:          userID,
		})
		if err != nil {
			slog.Warn("import: create session failed", slog.String("name", sess.Name), slog.Any("error", err))
			report.Fail("sessions", "session", sess.Name, apperror.SafeMessage(err))
			continue
		}

		// Apply summary and status via UpdateSession.
		if sess.Summary != nil || sess.Status != "" {
			status := sess.Status
			if status == "" {
				status = "planned"
			}
			// Full restore of an exported row: every field is sent
			// explicitly, since an explicit null clears under the
			// partial-update contract and patch.FromPtr renders exactly that.
			_, _ = a.svc.UpdateSession(ctx, newSession.ID, sessions.UpdateSessionInput{
				Name:               patch.Of(sess.Name),
				Summary:            patch.FromPtr(sess.Summary),
				ScheduledDate:      patch.FromPtr(sess.ScheduledDate),
				ScheduledTime:      patch.FromPtr(sess.ScheduledTime),
				CalendarYear:       patch.FromPtr(sess.CalendarYear),
				CalendarMonth:      patch.FromPtr(sess.CalendarMonth),
				CalendarDay:        patch.FromPtr(sess.CalendarDay),
				Status:             patch.Of(status),
				IsRecurring:        patch.Of(sess.IsRecurring),
				RecurrenceType:     patch.FromPtr(sess.RecurrenceType),
				RecurrenceInterval: patch.Of(sess.RecurrenceInterval),
			})
		}

		// Apply recap if present.
		if sess.Recap != nil || sess.RecapHTML != nil {
			_ = a.svc.UpdateSessionRecap(ctx, newSession.ID, sess.Recap, sess.RecapHTML)
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
				report.Fail("sessions", "session attendee list", sess.Name, apperror.SafeMessage(err))
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
func (a *timelineImportAdapter) ImportTimelines(ctx context.Context, campaignID, userID string, data []campaigns.ExportTimeline, idMap *campaigns.IDMap, report *campaigns.ImportReport) error {
	for _, tl := range data {
		calendarID := importTimelineCalendar(tl, idMap)

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
			report.Fail(campaigns.SectionTimelines, campaigns.KindTimeline, tl.Name, apperror.SafeMessage(err))
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
				Name:            evt.Name,
				Description:     evt.Description,
				DescriptionHTML: evt.DescriptionHTML,
				EntityID:        entityID,
				Year:            evt.Year,
				Month:           evt.Month,
				Day:             evt.Day,
				StartHour:       evt.StartHour,
				StartMinute:     evt.StartMinute,
				EndYear:         evt.EndYear,
				EndMonth:        evt.EndMonth,
				EndDay:          evt.EndDay,
				EndHour:         evt.EndHour,
				EndMinute:       evt.EndMinute,
				IsRecurring:     evt.IsRecurring,
				RecurrenceType:  evt.RecurrenceType,
				Category:        evt.Category,
				Visibility:      evt.Visibility,
				Label:           evt.Label,
				Color:           evt.Color,
				CreatedBy:       userID,
			})
			if err != nil {
				slog.Warn("import: create timeline event failed", slog.String("name", evt.Name), slog.Any("error", err))
				report.Fail("timelines", "timeline event", evt.Name, apperror.SafeMessage(err))
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
				report.Fail("timelines", "timeline connection", tl.Name, apperror.SafeMessage(err))
			}
		}

		// Re-link the calendar events the timeline showed, in their order.
		if calendarID != nil {
			for _, l := range tl.CalendarEventLinks {
				eventID, ok := idMap.CalendarEventIDs[l.EventRef]
				if !ok {
					report.Fail("timelines", "timeline event link", tl.Name, "its calendar event is not in the file")
					continue
				}
				if _, err := a.svc.LinkEvent(ctx, newTimeline.ID, eventID, timeline.LinkEventInput{
					Label: l.Label, ColorOverride: l.ColorOverride,
				}); err != nil {
					slog.Warn("import: link timeline event failed", slog.String("name", tl.Name), slog.Any("error", err))
					report.Fail("timelines", "timeline event link", tl.Name, apperror.SafeMessage(err))
					continue
				}
				if l.VisibilityOverride != nil || l.VisibilityRules != nil {
					if err := a.svc.UpdateEventLinkVisibility(ctx, newTimeline.ID, eventID, timeline.UpdateEventVisibilityInput{
						// A restore states the whole link, so a nil pointer means "none", not "absent".
						VisibilityOverride: patch.FromPtr(l.VisibilityOverride), VisibilityRules: patch.FromPtr(l.VisibilityRules),
					}); err != nil {
						slog.Warn("import: timeline link visibility failed", slog.String("name", tl.Name), slog.Any("error", err))
						report.Fail("timelines", "timeline event link", tl.Name, apperror.SafeMessage(err))
					}
				}
			}
		} else if len(tl.CalendarEventLinks) > 0 {
			report.FailN("timelines", "timeline event link", tl.Name, "its calendar was not restored", len(tl.CalendarEventLinks))
		}

		// Create entity groups (swim lanes).
		for _, eg := range tl.EntityGroups {
			newGroup, err := a.svc.CreateEntityGroup(ctx, newTimeline.ID, timeline.CreateEntityGroupInput{
				Name:  eg.Name,
				Color: eg.Color,
			})
			if err != nil {
				slog.Warn("import: create entity group failed", slog.String("name", eg.Name), slog.Any("error", err))
				report.Fail("timelines", "timeline entity group", eg.Name, apperror.SafeMessage(err))
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

// importTimelineCalendar picks the calendar a restored timeline draws on:
// none for a timeline that had none, the restored copy of the one it named
// (none if that calendar was not restored, rather than some other calendar),
// and the default calendar for a file from before timelines recorded theirs.
func importTimelineCalendar(tl campaigns.ExportTimeline, idMap *campaigns.IDMap) *string {
	if tl.NoCalendar {
		return nil
	}
	if tl.CalendarRef != nil {
		if id, ok := idMap.CalendarIDs[*tl.CalendarRef]; ok {
			return &id
		}
		return nil
	}
	if idMap.CalendarID != "" {
		id := idMap.CalendarID
		return &id
	}
	return nil
}

// mapImportAdapter implements campaigns.MapImporter.
type mapImportAdapter struct {
	mapSvc     maps.MapService
	drawingSvc maps.DrawingService
}

// importTokenImage keeps a token picture that names no media file (an
// inline data: image, a bundled icon) as it is, and one that names a media
// file only when this import restored that file; ok is false when a media
// picture was dropped.
func importTokenImage(idMap *campaigns.IDMap, path *string) (*string, bool) {
	if path == nil || *path == "" {
		return path, true
	}
	if uuid.Validate(mediaIDFromPath(*path)) != nil {
		return path, true
	}
	if _, ok := restoredMediaID(idMap, *path); ok {
		return path, true
	}
	return nil, false
}

// ImportMaps creates maps from import data. A map background or token
// picture is kept only when it names a file this import restored.
func (a *mapImportAdapter) ImportMaps(ctx context.Context, campaignID, userID string, data []campaigns.ExportMap, idMap *campaigns.IDMap, report *campaigns.ImportReport) error {
	missingPictures := 0
	defer func() {
		report.FailN("maps", "map picture", "",
			"its picture file was not in the upload; import the ZIP export to keep pictures", missingPictures)
	}()
	// A marker may open a map that comes later in the file, so links are set
	// once every map exists: refs maps each exported map to its copy.
	refs := make(map[string]string, len(data))
	type pendingLink struct{ markerID, mapRef, name string }
	var links []pendingLink
	for _, m := range data {
		var imageID *string
		if m.ImageID != nil && *m.ImageID != "" {
			if id, ok := restoredMediaID(idMap, *m.ImageID); ok {
				imageID = &id
			} else {
				missingPictures++
			}
		}
		newMap, err := a.mapSvc.CreateMap(ctx, maps.CreateMapInput{
			CampaignID:  campaignID,
			Name:        m.Name,
			Description: m.Description,
			ImageID:     imageID,
			ImageWidth:  m.ImageWidth,
			ImageHeight: m.ImageHeight,
		})
		if err != nil {
			slog.Warn("import: create map failed", slog.String("name", m.Name), slog.Any("error", err))
			report.Fail("maps", "map", m.Name, apperror.SafeMessage(err))
			continue
		}
		if m.Ref != "" {
			refs[m.Ref] = newMap.ID
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
				report.Fail("maps", "map layer", l.Name, apperror.SafeMessage(err))
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
			created, err := a.mapSvc.CreateMarker(ctx, maps.CreateMarkerInput{
				MapID: newMap.ID, Name: mk.Name, Description: mk.Description,
				X: mk.X, Y: mk.Y, Icon: mk.Icon, Color: mk.Color,
				EntityID: entityID, Visibility: mk.Visibility, CreatedBy: userID,
			})
			if err != nil {
				slog.Warn("import: create marker failed", slog.String("name", mk.Name), slog.Any("error", err))
				report.Fail("maps", "map marker", mk.Name, apperror.SafeMessage(err))
				continue
			}
			if mk.LinkedMapRef != nil && *mk.LinkedMapRef != "" {
				links = append(links, pendingLink{markerID: created.ID, mapRef: *mk.LinkedMapRef, name: mk.Name})
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
				// An import runs as the campaign's owner and writes into a map it
				// just created, so the draw gate (which defaults to scribes) is met.
				CallerRole: permissions.RoleOwner,
				CallerIsDM: true,
				Imported:   true,
			})
			if err != nil {
				slog.Warn("import: create drawing failed", slog.Any("error", err))
				report.Fail("maps", "map drawing", m.Name, apperror.SafeMessage(err))
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
			imagePath, ok := importTokenImage(idMap, t.ImagePath)
			if !ok {
				missingPictures++
			}
			_, err := a.drawingSvc.CreateToken(ctx, maps.CreateTokenInput{
				MapID: newMap.ID, LayerID: layerID, EntityID: entityID,
				Name: t.Name, ImagePath: imagePath,
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
				CreatedBy:     userID,
			})
			if err != nil {
				slog.Warn("import: create token failed", slog.String("name", t.Name), slog.Any("error", err))
				report.Fail("maps", "map token", t.Name, apperror.SafeMessage(err))
			}
		}

		// Fog regions.
		for _, f := range m.FogRegions {
			_, err := a.drawingSvc.CreateFog(ctx, maps.CreateFogInput{
				MapID: newMap.ID, Points: f.Points, IsExplored: f.IsExplored,
			})
			if err != nil {
				slog.Warn("import: create fog region failed", slog.Any("error", err))
				report.Fail("maps", "map fog region", m.Name, apperror.SafeMessage(err))
			}
		}
	}
	for _, l := range links {
		to, ok := refs[l.mapRef]
		if !ok {
			report.Fail("maps", "map marker link", l.name, "the map it opened was not imported")
			continue
		}
		if err := a.mapSvc.UpdateMarker(ctx, l.markerID, maps.UpdateMarkerInput{LinkedMapID: patch.Of(to)}, true); err != nil {
			slog.Warn("import: link marker failed", slog.String("name", l.name), slog.Any("error", err))
			report.Fail("maps", "map marker link", l.name, apperror.SafeMessage(err))
		}
	}
	return nil
}

// --- Note Import Adapter ---

// noteImportAdapter implements campaigns.NoteImporter.
type noteImportAdapter struct {
	svc notes.NoteService
}

// ImportNotes recreates the campaign's shared notes, owned by the importing
// user. Runs in two passes: create every note flat, then re-parent the ones
// that were filed in a folder. Two passes rather than one because a folder
// may appear after its children in the export, and a single pass would have
// to drop those children.
//
// Entry / EntryHTML / Pinned are applied through Update rather than Create
// because CreateNoteRequest carries neither, and because Update runs the
// imported HTML through the sanitizer on the way in — an imported export is
// untrusted input.
func (a *noteImportAdapter) ImportNotes(ctx context.Context, campaignID, userID string, data []campaigns.ExportNote, idMap *campaigns.IDMap, report *campaigns.ImportReport) error {
	newIDs := make([]string, len(data))
	// The importer owns every note it creates here, so its own role never
	// widens what it can see; a plain-member viewer is enough to re-file its
	// own notes into its own folders.
	importer := permissions.RequestViewer(permissions.RolePlayer, userID)

	for i, n := range data {
		var entityID *string
		if n.EntitySlug != nil {
			if id, ok := idMap.EntitySlugToID[*n.EntitySlug]; ok {
				eid := id
				entityID = &eid
			} else {
				slog.Warn("import: note entity not found; importing note campaign-wide",
					slog.String("entity_slug", *n.EntitySlug), slog.String("note", n.Title))
				report.Fail("notes", "note entity link", n.Title, "the linked entity is not in the file")
			}
		}

		var blocks []notes.Block
		if len(n.Content) > 0 {
			if err := json.Unmarshal(n.Content, &blocks); err != nil {
				slog.Warn("import: note content unreadable; importing note without blocks",
					slog.String("note", n.Title), slog.Any("error", err))
				report.Fail("notes", "note checklist", n.Title, "block content was not readable")
				blocks = nil
			}
		}

		created, err := a.svc.Create(ctx, campaignID, importer, notes.CreateNoteRequest{
			EntityID: entityID,
			IsFolder: n.IsFolder,
			Title:    n.Title,
			Content:  blocks,
			Color:    n.Color,
			IsShared: true, // Only shared notes are exported; keep them shared.
		})
		if err != nil {
			slog.Warn("import: create note failed", slog.String("note", n.Title), slog.Any("error", err))
			report.Fail("notes", "note", n.Title, apperror.SafeMessage(err))
			continue
		}
		newIDs[i] = created.ID

		if n.Entry != nil || n.EntryHTML != nil || n.Pinned {
			pinned := n.Pinned
			if _, err := a.svc.Update(ctx, created.ID, importer, notes.UpdateNoteRequest{
				Entry:     n.Entry,
				EntryHTML: n.EntryHTML,
				Pinned:    &pinned,
			}); err != nil {
				slog.Warn("import: note body not applied", slog.String("note", n.Title), slog.Any("error", err))
				report.Fail("notes", "note body", n.Title, apperror.SafeMessage(err))
			}
		}
	}

	// Second pass: re-parent notes whose folder now exists.
	for i, n := range data {
		if n.ParentIndex == nil || newIDs[i] == "" {
			continue
		}
		pi := *n.ParentIndex
		if pi < 0 || pi >= len(newIDs) || newIDs[pi] == "" {
			slog.Warn("import: note parent folder missing; note left at top level",
				slog.String("note", n.Title))
			report.Fail("notes", "note folder link", n.Title, "its folder is not in the file")
			continue
		}
		parentID := newIDs[pi]
		if _, err := a.svc.Update(ctx, newIDs[i], importer, notes.UpdateNoteRequest{
			ParentID: &parentID,
		}); err != nil {
			slog.Warn("import: note re-parent failed", slog.String("note", n.Title), slog.Any("error", err))
			report.Fail("notes", "note folder link", n.Title, apperror.SafeMessage(err))
		}
	}

	return nil
}

// addonImportAdapter implements campaigns.AddonImporter.
type addonImportAdapter struct {
	svc addons.AddonService
}

// ImportAddons enables addons from import data.
func (a *addonImportAdapter) ImportAddons(ctx context.Context, campaignID, userID string, data []campaigns.ExportAddon, report *campaigns.ImportReport) error {
	for _, ad := range data {
		// Look up addon by slug.
		addon, err := a.svc.GetBySlug(ctx, ad.Slug)
		if err != nil {
			slog.Warn("import: addon not found", slog.String("slug", ad.Slug))
			report.Fail("addons", "addon", ad.Slug, "not installed on this instance")
			continue
		}

		if err := a.svc.EnableForCampaign(ctx, campaignID, addon.ID, userID); err != nil {
			slog.Warn("import: enable addon failed", slog.String("slug", ad.Slug), slog.Any("error", err))
			report.Fail("addons", "addon", ad.Slug, apperror.SafeMessage(err))
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
func (a *groupImportAdapter) ImportGroups(ctx context.Context, campaignID string, data []campaigns.ExportGroup, report *campaigns.ImportReport) error {
	for _, g := range data {
		newGroup, err := a.svc.CreateGroup(ctx, campaignID, g.Name, g.Description)
		if err != nil {
			slog.Warn("import: create group failed", slog.String("name", g.Name), slog.Any("error", err))
			report.Fail("groups", "group", g.Name, apperror.SafeMessage(err))
			continue
		}

		for _, userID := range g.MemberIDs {
			if err := a.svc.AddGroupMember(ctx, newGroup.ID, userID); err != nil {
				slog.Warn("import: add group member skipped (user may not exist)",
					slog.String("group", g.Name), slog.String("user_id", userID))
				report.Fail("groups", "group member", userID, "no such user on this instance")
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
			entityPosts, err := a.postSvc.ListByEntity(ctx, e.CampaignID, e.ID, true)
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
func (a *postImportAdapter) ImportPosts(ctx context.Context, campaignID, userID string, data []campaigns.ExportPost, idMap *campaigns.IDMap, report *campaigns.ImportReport) error {
	for _, p := range data {
		entityID, ok := idMap.EntitySlugToID[p.EntitySlug]
		if !ok {
			slog.Warn("import: post entity not found", slog.String("entity_slug", p.EntitySlug), slog.String("post", p.Name))
			report.Fail("posts", "post", p.Name, "its entity is not in the file")
			continue
		}

		_, err := a.svc.Create(ctx, campaignID, entityID, userID, p.Name, posts.CreatePostRequest{
			Entry:     p.Entry,
			EntryHTML: p.EntryHTML,
			IsPrivate: p.IsPrivate,
		})
		if err != nil {
			slog.Warn("import: create post failed", slog.String("post", p.Name), slog.Any("error", err))
			report.Fail("posts", "post", p.Name, apperror.SafeMessage(err))
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

// aiExportCalendarListerAdapter implements aiexport.CalendarLister.
// A declared SYSTEM caller (ADR-049): the AI export is a Director-only tool
// (ai_workspace's own route gating enforces this) that needs the FULL
// picture — dm_only events, not-yet-announced ones, hidden moons — so the
// renderer's own Safe-mode filter (aiexport/renderer.go's
// RenderCalendarEvents) can apply its own honest rules rather than trusting
// a per-viewer calendar read to already agree with what "Safe" means.
type aiExportCalendarListerAdapter struct {
	svc calendar.CalendarService
}

// GetCalendar returns the campaign's default calendar, unfiltered. Same
// "no calendar yet" -> nil,nil degrade aiexport's service.go already expects
// (a disabled addon or a campaign with no calendar just skips the section).
func (a *aiExportCalendarListerAdapter) GetCalendar(ctx context.Context, campaignID string) (*calendar.Calendar, error) {
	const ownerRole = 3
	cal, err := a.svc.GetDefaultCalendarForViewer(ctx, campaignID, permissions.SystemViewer(ownerRole))
	if err != nil {
		if isNotFound(err) {
			return nil, nil //nolint:nilerr // no calendar yet — a normal state, not a failure
		}
		return nil, err
	}
	return cal, nil
}

// ListAllEventsForCalendar returns every event on calendarID, unfiltered —
// see the type doc comment for why this bypass is safe here specifically.
func (a *aiExportCalendarListerAdapter) ListAllEventsForCalendar(ctx context.Context, campaignID, calendarID string) ([]calendar.Event, error) {
	const ownerRole = 3
	return a.svc.ListAllEventsForCalendar(ctx, calendarID, campaignID, permissions.SystemViewer(ownerRole))
}
