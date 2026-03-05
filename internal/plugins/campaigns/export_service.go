// Package campaigns — export_service.go implements the campaign export/import
// business logic. Uses adapter interfaces for all cross-plugin data access
// to avoid circular imports.
package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Adapter interfaces for cross-plugin data access ---

// ExportEntityData holds all entity-related data for export.
type ExportEntityData struct {
	Types     []ExportEntityType
	Entities  []ExportEntity
	Tags      []ExportTag
	EntityTags []ExportEntityTag
	Relations []ExportRelation
}

// EntityExporter gathers all entity-related data for a campaign export.
// Implemented as an adapter in app/routes.go to avoid circular imports.
type EntityExporter interface {
	ExportEntities(ctx context.Context, campaignID string) (*ExportEntityData, error)
}

// CalendarExporter gathers calendar data for a campaign export.
type CalendarExporter interface {
	ExportCalendar(ctx context.Context, campaignID string, entitySlugLookup func(string) string) (*ExportCalendarData, error)
}

// TimelineExporter gathers timeline data for a campaign export.
type TimelineExporter interface {
	ExportTimelines(ctx context.Context, campaignID string, entitySlugLookup func(string) string) ([]ExportTimeline, error)
}

// SessionExporter gathers session data for a campaign export.
type SessionExporter interface {
	ExportSessions(ctx context.Context, campaignID string, entitySlugLookup func(string) string) ([]ExportSession, error)
}

// MapExporter gathers map data for a campaign export.
type MapExporter interface {
	ExportMaps(ctx context.Context, campaignID string, entitySlugLookup func(string) string) ([]ExportMap, error)
}

// NoteExporter gathers shared notes for a campaign export.
type NoteExporter interface {
	ExportNotes(ctx context.Context, campaignID string, entitySlugLookup func(string) string) ([]ExportNote, error)
}

// AddonExporter gathers addon configuration for a campaign export.
type AddonExporter interface {
	ExportAddons(ctx context.Context, campaignID string) ([]ExportAddon, error)
}

// MediaExporter gathers media file metadata for a campaign export.
type MediaExporter interface {
	ExportMedia(ctx context.Context, campaignID string) ([]ExportMediaFile, error)
}

// --- Import adapter interfaces ---

// EntityImporter creates entities from import data. Returns the ID map
// for cross-referencing by other importers.
type EntityImporter interface {
	ImportEntities(ctx context.Context, campaignID, userID string, data *ExportEntityData) (*IDMap, error)
}

// CalendarImporter creates calendar from import data.
type CalendarImporter interface {
	ImportCalendar(ctx context.Context, campaignID string, data *ExportCalendarData, idMap *IDMap) error
}

// TimelineImporter creates timelines from import data.
type TimelineImporter interface {
	ImportTimelines(ctx context.Context, campaignID, userID string, data []ExportTimeline, idMap *IDMap) error
}

// SessionImporter creates sessions from import data.
type SessionImporter interface {
	ImportSessions(ctx context.Context, campaignID, userID string, data []ExportSession, idMap *IDMap) error
}

// MapImporter creates maps from import data.
type MapImporter interface {
	ImportMaps(ctx context.Context, campaignID, userID string, data []ExportMap, idMap *IDMap) error
}

// NoteImporter creates notes from import data.
type NoteImporter interface {
	ImportNotes(ctx context.Context, campaignID, userID string, data []ExportNote, idMap *IDMap) error
}

// AddonImporter enables addons from import data.
type AddonImporter interface {
	ImportAddons(ctx context.Context, campaignID, userID string, data []ExportAddon) error
}

// --- Export/Import Service ---

// ExportImportService handles campaign export and import operations.
type ExportImportService struct {
	campaigns CampaignService

	// Export adapters.
	entityExp   EntityExporter
	calendarExp CalendarExporter
	timelineExp TimelineExporter
	sessionExp  SessionExporter
	mapExp      MapExporter
	noteExp     NoteExporter
	addonExp    AddonExporter
	mediaExp    MediaExporter

	// Import adapters.
	entityImp   EntityImporter
	calendarImp CalendarImporter
	timelineImp TimelineImporter
	sessionImp  SessionImporter
	mapImp      MapImporter
	noteImp     NoteImporter
	addonImp    AddonImporter
}

// NewExportImportService creates a new export/import service.
func NewExportImportService(campaigns CampaignService) *ExportImportService {
	return &ExportImportService{campaigns: campaigns}
}

// --- Setter methods for wiring adapters after construction ---

// SetEntityExporter wires the entity export adapter.
func (s *ExportImportService) SetEntityExporter(e EntityExporter) { s.entityExp = e }

// SetCalendarExporter wires the calendar export adapter.
func (s *ExportImportService) SetCalendarExporter(e CalendarExporter) { s.calendarExp = e }

// SetTimelineExporter wires the timeline export adapter.
func (s *ExportImportService) SetTimelineExporter(e TimelineExporter) { s.timelineExp = e }

// SetSessionExporter wires the session export adapter.
func (s *ExportImportService) SetSessionExporter(e SessionExporter) { s.sessionExp = e }

// SetMapExporter wires the map export adapter.
func (s *ExportImportService) SetMapExporter(e MapExporter) { s.mapExp = e }

// SetNoteExporter wires the note export adapter.
func (s *ExportImportService) SetNoteExporter(e NoteExporter) { s.noteExp = e }

// SetAddonExporter wires the addon export adapter.
func (s *ExportImportService) SetAddonExporter(e AddonExporter) { s.addonExp = e }

// SetMediaExporter wires the media export adapter.
func (s *ExportImportService) SetMediaExporter(e MediaExporter) { s.mediaExp = e }

// SetEntityImporter wires the entity import adapter.
func (s *ExportImportService) SetEntityImporter(i EntityImporter) { s.entityImp = i }

// SetCalendarImporter wires the calendar import adapter.
func (s *ExportImportService) SetCalendarImporter(i CalendarImporter) { s.calendarImp = i }

// SetTimelineImporter wires the timeline import adapter.
func (s *ExportImportService) SetTimelineImporter(i TimelineImporter) { s.timelineImp = i }

// SetSessionImporter wires the session import adapter.
func (s *ExportImportService) SetSessionImporter(i SessionImporter) { s.sessionImp = i }

// SetMapImporter wires the map import adapter.
func (s *ExportImportService) SetMapImporter(i MapImporter) { s.mapImp = i }

// SetNoteImporter wires the note import adapter.
func (s *ExportImportService) SetNoteImporter(i NoteImporter) { s.noteImp = i }

// SetAddonImporter wires the addon import adapter.
func (s *ExportImportService) SetAddonImporter(i AddonImporter) { s.addonImp = i }

// Export generates a complete campaign export as a CampaignExport struct.
// Requires the caller to have owner access to the campaign.
func (s *ExportImportService) Export(ctx context.Context, campaignID string) (*CampaignExport, error) {
	// Fetch campaign metadata.
	campaign, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("export campaign: %w", err)
	}

	export := &CampaignExport{
		Format:     ExportFormat,
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Campaign: ExportCampaignMeta{
			Name:        campaign.Name,
			Description: campaign.Description,
			IsPublic:    campaign.IsPublic,
		},
	}

	// Serialize campaign JSON settings.
	if campaign.Settings != "" {
		export.Campaign.Settings = json.RawMessage(campaign.Settings)
	}
	if campaign.SidebarConfig != "" {
		export.Campaign.SidebarConfig = json.RawMessage(campaign.SidebarConfig)
	}
	if campaign.DashboardLayout != nil {
		export.Campaign.DashboardLayout = json.RawMessage(*campaign.DashboardLayout)
	}

	// Build entity ID → slug lookup for cross-references.
	var entityIDToSlug map[string]string

	// Export entities (types, entities, tags, relations).
	if s.entityExp != nil {
		entityData, err := s.entityExp.ExportEntities(ctx, campaignID)
		if err != nil {
			return nil, fmt.Errorf("export entities: %w", err)
		}
		export.EntityTypes = entityData.Types
		export.Entities = entityData.Entities
		export.Tags = entityData.Tags
		export.EntityTags = entityData.EntityTags
		export.Relations = entityData.Relations

		// Build slug lookup from exported entities.
		entityIDToSlug = make(map[string]string, len(entityData.Entities))
		for _, e := range entityData.Entities {
			entityIDToSlug[e.OriginalID] = e.Slug
		}
	}

	slugLookup := func(entityID string) string {
		if entityIDToSlug == nil {
			return ""
		}
		return entityIDToSlug[entityID]
	}

	// Export calendar (if addon enabled and adapter wired).
	if s.calendarExp != nil {
		calData, err := s.calendarExp.ExportCalendar(ctx, campaignID, slugLookup)
		if err != nil {
			slog.Warn("export calendar skipped", slog.Any("error", err))
		} else {
			export.Calendar = calData
		}
	}

	// Export timelines.
	if s.timelineExp != nil {
		timelines, err := s.timelineExp.ExportTimelines(ctx, campaignID, slugLookup)
		if err != nil {
			slog.Warn("export timelines skipped", slog.Any("error", err))
		} else {
			export.Timelines = timelines
		}
	}

	// Export sessions.
	if s.sessionExp != nil {
		sessions, err := s.sessionExp.ExportSessions(ctx, campaignID, slugLookup)
		if err != nil {
			slog.Warn("export sessions skipped", slog.Any("error", err))
		} else {
			export.Sessions = sessions
		}
	}

	// Export maps.
	if s.mapExp != nil {
		maps, err := s.mapExp.ExportMaps(ctx, campaignID, slugLookup)
		if err != nil {
			slog.Warn("export maps skipped", slog.Any("error", err))
		} else {
			export.Maps = maps
		}
	}

	// Export notes.
	if s.noteExp != nil {
		notes, err := s.noteExp.ExportNotes(ctx, campaignID, slugLookup)
		if err != nil {
			slog.Warn("export notes skipped", slog.Any("error", err))
		} else {
			export.Notes = notes
		}
	}

	// Export addon configuration.
	if s.addonExp != nil {
		addons, err := s.addonExp.ExportAddons(ctx, campaignID)
		if err != nil {
			slog.Warn("export addons skipped", slog.Any("error", err))
		} else {
			export.Addons = addons
		}
	}

	// Export media manifest.
	if s.mediaExp != nil {
		media, err := s.mediaExp.ExportMedia(ctx, campaignID)
		if err != nil {
			slog.Warn("export media skipped", slog.Any("error", err))
		} else {
			export.Media = media
		}
	}

	return export, nil
}

// Import creates a new campaign from a CampaignExport. Returns the newly
// created campaign. The import processes data in dependency order:
// 1. Campaign metadata → 2. Entity types + entities + tags + relations →
// 3. Calendar → 4. Timelines → 5. Sessions → 6. Maps → 7. Notes → 8. Addons
func (s *ExportImportService) Import(ctx context.Context, userID string, data *CampaignExport) (*Campaign, error) {
	// Create the new campaign.
	campaign, err := s.campaigns.Create(ctx, userID, CreateCampaignInput{
		Name:        data.Campaign.Name,
		Description: ptrToString(data.Campaign.Description),
	})
	if err != nil {
		return nil, fmt.Errorf("import create campaign: %w", err)
	}

	campaignID := campaign.ID

	// Apply campaign settings if present.
	if len(data.Campaign.SidebarConfig) > 0 {
		var sidebarCfg SidebarConfig
		if err := json.Unmarshal(data.Campaign.SidebarConfig, &sidebarCfg); err == nil {
			if err := s.campaigns.UpdateSidebarConfig(ctx, campaignID, sidebarCfg); err != nil {
				slog.Warn("import sidebar config failed", slog.Any("error", err))
			}
		}
	}
	if len(data.Campaign.DashboardLayout) > 0 {
		var dashLayout DashboardLayout
		if err := json.Unmarshal(data.Campaign.DashboardLayout, &dashLayout); err == nil {
			if err := s.campaigns.UpdateDashboardLayout(ctx, campaignID, &dashLayout); err != nil {
				slog.Warn("import dashboard layout failed", slog.Any("error", err))
			}
		}
	}

	// Import entities (creates entity types, entities, tags, relations).
	var idMap *IDMap
	if s.entityImp != nil && (len(data.EntityTypes) > 0 || len(data.Entities) > 0) {
		entityData := &ExportEntityData{
			Types:      data.EntityTypes,
			Entities:   data.Entities,
			Tags:       data.Tags,
			EntityTags: data.EntityTags,
			Relations:  data.Relations,
		}
		idMap, err = s.entityImp.ImportEntities(ctx, campaignID, userID, entityData)
		if err != nil {
			return nil, fmt.Errorf("import entities: %w", err)
		}
	}
	if idMap == nil {
		idMap = NewIDMap(campaignID)
	}

	// Import calendar.
	if s.calendarImp != nil && data.Calendar != nil {
		if err := s.calendarImp.ImportCalendar(ctx, campaignID, data.Calendar, idMap); err != nil {
			slog.Warn("import calendar failed", slog.Any("error", err))
		}
	}

	// Import timelines.
	if s.timelineImp != nil && len(data.Timelines) > 0 {
		if err := s.timelineImp.ImportTimelines(ctx, campaignID, userID, data.Timelines, idMap); err != nil {
			slog.Warn("import timelines failed", slog.Any("error", err))
		}
	}

	// Import sessions.
	if s.sessionImp != nil && len(data.Sessions) > 0 {
		if err := s.sessionImp.ImportSessions(ctx, campaignID, userID, data.Sessions, idMap); err != nil {
			slog.Warn("import sessions failed", slog.Any("error", err))
		}
	}

	// Import maps.
	if s.mapImp != nil && len(data.Maps) > 0 {
		if err := s.mapImp.ImportMaps(ctx, campaignID, userID, data.Maps, idMap); err != nil {
			slog.Warn("import maps failed", slog.Any("error", err))
		}
	}

	// Import notes.
	if s.noteImp != nil && len(data.Notes) > 0 {
		if err := s.noteImp.ImportNotes(ctx, campaignID, userID, data.Notes, idMap); err != nil {
			slog.Warn("import notes failed", slog.Any("error", err))
		}
	}

	// Import addons.
	if s.addonImp != nil && len(data.Addons) > 0 {
		if err := s.addonImp.ImportAddons(ctx, campaignID, userID, data.Addons); err != nil {
			slog.Warn("import addons failed", slog.Any("error", err))
		}
	}

	return campaign, nil
}

// Validate checks a CampaignExport for structural integrity before import.
// Returns nil if the export is valid, or an error describing the problem.
func (s *ExportImportService) Validate(data *CampaignExport) error {
	if data.Format != ExportFormat {
		return apperror.NewBadRequest(fmt.Sprintf("unsupported format %q", data.Format))
	}
	if data.Campaign.Name == "" {
		return apperror.NewBadRequest("campaign name is required")
	}

	// Check for duplicate entity type slugs.
	typeSlugs := make(map[string]bool)
	for _, et := range data.EntityTypes {
		if typeSlugs[et.Slug] {
			return apperror.NewBadRequest(fmt.Sprintf("duplicate entity type slug: %s", et.Slug))
		}
		typeSlugs[et.Slug] = true
	}

	// Check for duplicate entity slugs.
	entitySlugs := make(map[string]bool)
	for _, e := range data.Entities {
		if entitySlugs[e.Slug] {
			return apperror.NewBadRequest(fmt.Sprintf("duplicate entity slug: %s", e.Slug))
		}
		entitySlugs[e.Slug] = true
	}

	return nil
}

// ptrToString dereferences a *string or returns empty string.
func ptrToString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}
