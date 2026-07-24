package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/extensions"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/prompt"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/backup"
	"github.com/keyxmakerx/chronicle/internal/plugins/bestiary"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/designlab"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/npcs"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
	"github.com/keyxmakerx/chronicle/internal/plugins/restore"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
	"github.com/keyxmakerx/chronicle/internal/plugins/smtp"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
	"github.com/keyxmakerx/chronicle/internal/systems"
	"github.com/keyxmakerx/chronicle/internal/templates/demo"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
	"github.com/keyxmakerx/chronicle/internal/templates/pages"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
	"github.com/keyxmakerx/chronicle/internal/widgets/entity_notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/posts"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// bestiaryUserFetcherAdapter wraps auth.AuthService to implement the
// bestiary.UserFetcher interface for creator profile display names.
type bestiaryUserFetcherAdapter struct {
	authSvc auth.AuthService
}

// GetUserPublicInfo returns minimal public user info for bestiary creator profiles.
func (a *bestiaryUserFetcherAdapter) GetUserPublicInfo(ctx context.Context, userID string) (*bestiary.UserInfo, error) {
	user, err := a.authSvc.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	info := &bestiary.UserInfo{
		ID:          user.ID,
		DisplayName: user.DisplayName,
	}
	if user.AvatarPath != nil {
		info.AvatarURL = *user.AvatarPath
	}
	return info, nil
}

// bestiaryEntityCreatorAdapter wraps entities.EntityService to implement the
// bestiary.EntityCreator interface for importing creatures into campaigns.
type bestiaryEntityCreatorAdapter struct {
	svc entities.EntityService
}

// CreateFromStatblock creates a new entity in a campaign from a bestiary statblock.
// Uses entity type ID 0 (default) since the real type depends on system configuration.
func (a *bestiaryEntityCreatorAdapter) CreateFromStatblock(ctx context.Context, campaignID, userID, name string, statblock json.RawMessage) (string, error) {
	input := entities.CreateEntityInput{
		Name:       name,
		FieldsData: map[string]any{"statblock_json": string(statblock)},
	}
	ent, err := a.svc.Create(ctx, campaignID, userID, input)
	if err != nil {
		return "", err
	}
	return ent.ID, nil
}

// bestiaryCampaignRoleAdapter wraps campaigns.CampaignService to implement the
// bestiary.CampaignRoleChecker interface for verifying import permissions.
type bestiaryCampaignRoleAdapter struct {
	svc campaigns.CampaignService
}

// HasMinRole checks if a user has at least the specified role in a campaign.
func (a *bestiaryCampaignRoleAdapter) HasMinRole(ctx context.Context, campaignID, userID string, minRole int) (bool, error) {
	member, err := a.svc.GetMember(ctx, campaignID, userID)
	if err != nil {
		return false, nil // Not a member → no role.
	}
	return int(member.Role) >= minRole, nil
}

// bestiaryCampaignSystemAdapter wraps campaigns.CampaignService to implement
// the bestiary.CampaignSystemFetcher interface. Used by Publish to tag a
// publication with the source campaign's selected game system rather than
// guessing a default.
type bestiaryCampaignSystemAdapter struct {
	svc campaigns.CampaignService
}

// GetCampaignSystemID returns the system_id from a campaign's settings JSON,
// or empty string if the campaign has no system selected or cannot be found.
func (a *bestiaryCampaignSystemAdapter) GetCampaignSystemID(ctx context.Context, campaignID string) (string, error) {
	c, err := a.svc.GetByID(ctx, campaignID)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", nil
	}
	return c.ParseSettings().SystemID, nil
}

// entityTypeListerAdapter wraps entities.EntityService to implement the
// campaigns.EntityTypeLister interface without creating a circular import.
type entityTypeListerAdapter struct {
	svc entities.EntityService
}

// GetEntityTypesForSettings returns entity types formatted for the settings page.
func (a *entityTypeListerAdapter) GetEntityTypesForSettings(ctx context.Context, campaignID string) ([]campaigns.SettingsEntityType, error) {
	etypes, err := a.svc.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	result := make([]campaigns.SettingsEntityType, len(etypes))
	for i, et := range etypes {
		result[i] = campaigns.SettingsEntityType{
			ID:           et.ID,
			Name:         et.Name,
			NamePlural:   et.NamePlural,
			Icon:         et.Icon,
			Color:        et.Color,
			Description:  et.Description,
			ParentTypeID: et.ParentTypeID,
		}
	}
	return result, nil
}

// recentEntityListerAdapter wraps entities.EntityService to implement the
// campaigns.RecentEntityLister interface without creating a circular import.
type recentEntityListerAdapter struct {
	svc entities.EntityService
}

// ListRecentForDashboard returns recently updated entities formatted for the dashboard.
func (a *recentEntityListerAdapter) ListRecentForDashboard(ctx context.Context, campaignID string, role int, userID string, limit int) ([]campaigns.RecentEntity, error) {
	ents, err := a.svc.ListRecent(ctx, campaignID, role, userID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]campaigns.RecentEntity, len(ents))
	for i, e := range ents {
		result[i] = campaigns.RecentEntity{
			ID:        e.ID,
			Name:      e.Name,
			TypeName:  e.TypeName,
			TypeIcon:  e.TypeIcon,
			TypeColor: e.TypeColor,
			ImagePath: e.ImagePath,
			IsPrivate: e.IsPrivate,
			UpdatedAt: e.UpdatedAt,
		}
	}
	return result, nil
}

// entityTypeLayoutFetcherAdapter wraps entities.EntityService to implement the
// campaigns.EntityTypeLayoutFetcher interface. Fetches a single entity type
// with pre-serialized layout and fields JSON for the page layout editor.
type entityTypeLayoutFetcherAdapter struct {
	svc entities.EntityService
}

// GetEntityTypeForLayoutEditor returns entity type data formatted for the
// template-editor widget mount. Layout and fields are pre-serialized to JSON.
func (a *entityTypeLayoutFetcherAdapter) GetEntityTypeForLayoutEditor(ctx context.Context, entityTypeID int) (*campaigns.LayoutEditorEntityType, error) {
	et, err := a.svc.GetEntityTypeByID(ctx, entityTypeID)
	if err != nil {
		return nil, err
	}
	layoutJSON, err := json.Marshal(et.Layout)
	if err != nil {
		return nil, fmt.Errorf("marshal entity type layout: %w", err)
	}
	fieldsJSON, err := json.Marshal(et.Fields)
	if err != nil {
		return nil, fmt.Errorf("marshal entity type fields: %w", err)
	}
	return &campaigns.LayoutEditorEntityType{
		ID:         et.ID,
		CampaignID: et.CampaignID,
		Name:       et.Name,
		NamePlural: et.NamePlural,
		Icon:       et.Icon,
		Color:      et.Color,
		LayoutJSON: string(layoutJSON),
		FieldsJSON: string(fieldsJSON),
	}, nil
}

// campaignAuditAdapter wraps audit.AuditService to implement the
// campaigns.AuditLogger interface without creating a circular import
// (audit already imports campaigns for middleware).
type campaignAuditAdapter struct {
	svc audit.AuditService
}

// LogEvent records a campaign-scoped audit event.
func (a *campaignAuditAdapter) LogEvent(ctx context.Context, campaignID, userID, action string, details map[string]any) error {
	return a.svc.Log(ctx, &audit.AuditEntry{
		CampaignID: campaignID,
		UserID:     userID,
		Action:     action,
		Details:    details,
	})
}

// systemManifestFinderAdapter bridges systems.Find to addons.SystemManifestFinder.
// Used for self-healing addon registration when a system is in the registry but
// not yet in the addons database.
type systemManifestFinderAdapter struct{}

func (a *systemManifestFinderAdapter) FindManifest(id string) *addons.SystemManifestInfo {
	m := systems.Find(id)
	if m == nil {
		return nil
	}
	return &addons.SystemManifestInfo{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Version:     m.Version,
		Icon:        m.Icon,
		Author:      m.Author,
	}
}

// systemListerAdapter wraps systems.Registry to implement the
// campaigns.SystemLister interface for the game system dropdown.
type systemListerAdapter struct{}

// ListSystems returns all available game systems with loading status.
// Systems that are "available" but failed to instantiate are flagged
// with HasError so the settings page can show a warning.
func (a *systemListerAdapter) ListSystems() []campaigns.SystemOption {
	manifests := systems.Registry()
	opts := make([]campaigns.SystemOption, 0, len(manifests))
	for _, m := range manifests {
		// A system is in error state if its manifest says "available" but
		// it wasn't successfully instantiated into a live System instance.
		hasError := m.Status == systems.StatusAvailable && systems.FindSystem(m.ID) == nil
		opts = append(opts, campaigns.SystemOption{
			ID:       m.ID,
			Name:     m.Name,
			HasError: hasError,
		})
	}
	return opts
}

// addonListerAdapter wraps addons.AddonService to implement the
// campaigns.AddonLister interface for the plugin hub page.
type addonListerAdapter struct {
	svc addons.AddonService
}

// ListForPluginHub returns all addons formatted for the plugin hub page
// and the C-EXT-HUB top-level Extensions hub. HasDashboard /
// HasEntitySetup are sourced from the campaigns plugin's slug-keyed
// capability tables so the hub catalog carries a single source of
// truth (see internal/plugins/campaigns/extensions_hub.go).
func (a *addonListerAdapter) ListForPluginHub(ctx context.Context, campaignID string) ([]campaigns.PluginHubAddon, error) {
	addonList, err := a.svc.ListForCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	result := make([]campaigns.PluginHubAddon, len(addonList))
	for i, ca := range addonList {
		// HasSetup: the registry is the single source of truth for which
		// addons expose a settings/onboarding page. NeedsSetup: a per-campaign
		// read (provider exists, enabled, not done/dismissed, actionable check)
		// — best-effort for the badge, so a read error just leaves it false.
		_, hasSetup := a.svc.SetupProviderFor(ca.AddonSlug)
		needsSetup := false
		if hasSetup {
			needsSetup, _ = a.svc.NeedsSetup(ctx, campaignID, ca.AddonSlug)
		}
		result[i] = campaigns.PluginHubAddon{
			AddonID:        ca.AddonID,
			Slug:           ca.AddonSlug,
			Name:           ca.AddonName,
			Icon:           ca.AddonIcon,
			Category:       string(ca.AddonCategory),
			Enabled:        ca.Enabled,
			Installed:      ca.Installed,
			HasDashboard:   campaigns.HasExtensionDashboard(ca.AddonSlug),
			HasEntitySetup: campaigns.HasExtensionEntitySetup(ca.AddonSlug),
			HasSetup:       hasSetup,
			NeedsSetup:     needsSetup,
		}
	}
	return result, nil
}

// addonListerAPIAdapter wraps the addon service to implement the
// syncapi.AddonLister interface for the REST API addon discovery endpoint.
type addonListerAPIAdapter struct {
	svc addons.AddonService
}

func (a *addonListerAPIAdapter) ListForCampaign(ctx context.Context, campaignID string) ([]syncapi.AddonInfo, error) {
	addonList, err := a.svc.ListForCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	result := make([]syncapi.AddonInfo, len(addonList))
	for i, ca := range addonList {
		result[i] = syncapi.AddonInfo{
			Slug:      ca.AddonSlug,
			Name:      ca.AddonName,
			Icon:      ca.AddonIcon,
			Category:  string(ca.AddonCategory),
			Enabled:   ca.Enabled,
			Installed: ca.Installed,
		}
	}
	return result, nil
}

// backdropUploaderAdapter wraps the media service to implement the
// campaigns.MediaUploader interface for backdrop image uploads.
type backdropUploaderAdapter struct {
	svc media.MediaService
}

// UploadBackdrop uploads an image via the media service with backdrop usage type.
func (a *backdropUploaderAdapter) UploadBackdrop(ctx context.Context, campaignID, userID string, fileBytes []byte, originalName, mimeType string) (string, error) {
	mf, err := a.svc.Upload(ctx, media.UploadInput{
		CampaignID:   campaignID,
		UploadedBy:   userID,
		OriginalName: originalName,
		MimeType:     mimeType,
		FileSize:     int64(len(fileBytes)),
		UsageType:    media.UsageBackdrop,
		FileBytes:    fileBytes,
	})
	if err != nil {
		return "", err
	}
	return mf.Filename, nil
}

// entityTagFetcherAdapter wraps tags.TagService to implement the
// entities.EntityTagFetcher interface for batch tag loading in list views.
// grantSvc backs the tag-grant glance methods (C-PERM-W1-TAG-GRANTS).
type entityTagFetcherAdapter struct {
	svc      tags.TagService
	grantSvc tags.TagGrantService
}

// GetEntityTagsBatch returns minimal tag info for multiple entities.
// includeDmOnly controls whether dm_only tags are included (true for Scribes+).
func (a *entityTagFetcherAdapter) GetEntityTagsBatch(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]entities.EntityTagInfo, error) {
	tagsMap, err := a.svc.GetEntityTagsBatch(ctx, entityIDs, includeDmOnly)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]entities.EntityTagInfo, len(tagsMap))
	for eid, tagList := range tagsMap {
		infos := make([]entities.EntityTagInfo, len(tagList))
		for i, t := range tagList {
			infos[i] = entities.EntityTagInfo{ID: t.ID, Name: t.Name, Color: t.Color}
		}
		result[eid] = infos
	}
	return result, nil
}

// GetEntityTags returns tags for a single entity.
func (a *entityTagFetcherAdapter) GetEntityTags(ctx context.Context, entityID string, includeDmOnly bool) ([]entities.EntityTagInfo, error) {
	tagList, err := a.svc.GetEntityTags(ctx, entityID, includeDmOnly)
	if err != nil {
		return nil, err
	}
	infos := make([]entities.EntityTagInfo, len(tagList))
	for i, t := range tagList {
		infos[i] = entities.EntityTagInfo{ID: t.ID, Name: t.Name, Color: t.Color}
	}
	return infos, nil
}

// SetEntityTags sets the tags for a single entity.
func (a *entityTagFetcherAdapter) SetEntityTags(ctx context.Context, entityID string, campaignID string, tagIDs []int) error {
	return a.svc.SetEntityTags(ctx, entityID, campaignID, tagIDs)
}

// GetEntityTagGrants resolves the tag-derived visibility grants on one entity
// for its effective-visibility glance (C-PERM-W1-TAG-GRANTS).
func (a *entityTagFetcherAdapter) GetEntityTagGrants(ctx context.Context, campaignID, entityID string) ([]entities.EntityTagGrantInfo, error) {
	if a.grantSvc == nil {
		return nil, nil
	}
	grants, err := a.grantSvc.GrantsForEntity(ctx, campaignID, entityID)
	if err != nil {
		return nil, err
	}
	infos := make([]entities.EntityTagGrantInfo, len(grants))
	for i, g := range grants {
		infos[i] = entities.EntityTagGrantInfo{
			TagName:      g.TagName,
			TagSlug:      g.TagSlug,
			TagColor:     g.TagColor,
			SubjectType:  g.SubjectType,
			SubjectID:    g.SubjectID,
			SubjectLabel: g.SubjectLabel,
		}
	}
	return infos, nil
}

// entityCampaignCheckerAdapter wraps entities.EntityService to implement the
// sessions.EntityCampaignChecker interface, verifying entity-campaign membership
// to prevent cross-campaign IDOR attacks on entity linking.
type entityCampaignCheckerAdapter struct {
	svc entities.EntityService
}

// EntityBelongsToCampaign checks if the given entity exists in the given campaign.
func (a *entityCampaignCheckerAdapter) EntityBelongsToCampaign(ctx context.Context, entityID, campaignID string) (bool, error) {
	entity, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		return false, err
	}
	return entity.CampaignID == campaignID, nil
}

// calendarListerAdapter wraps calendar.CalendarService to implement the
// timeline.CalendarLister interface. Returns available calendars for the
// timeline create form's calendar selector dropdown.
type calendarListerAdapter struct {
	svc calendar.CalendarService
}

// ListCalendars returns all calendars for a campaign as lightweight refs
// for the timeline create form's calendar selector dropdown.
func (a *calendarListerAdapter) ListCalendars(ctx context.Context, campaignID string) ([]timeline.CalendarRef, error) {
	cals, err := a.svc.ListCalendars(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	refs := make([]timeline.CalendarRef, len(cals))
	for i, cal := range cals {
		refs[i] = timeline.CalendarRef{ID: cal.ID, Name: cal.Name}
	}
	return refs, nil
}

// timelineForCalendarAdapter wraps timeline.TimelineService to implement the
// calendar.TimelineLister interface — the reverse-direction cross-plugin read
// the Calendars dashboard's associations panel needs (C-APPS-CAL-DASH-W1). The
// calendar plugin never imports the timeline repo; it reaches the timeline
// service through this adapter at the app boundary (plugin-isolation rule).
type timelineForCalendarAdapter struct {
	svc timeline.TimelineService
}

// ListTimelinesForCalendar returns the timelines bound to a calendar as the
// calendar plugin's lightweight TimelineRef projection.
func (a *timelineForCalendarAdapter) ListTimelinesForCalendar(ctx context.Context, calendarID string, role int, userID string) ([]calendar.TimelineRef, error) {
	tls, err := a.svc.ListTimelinesForCalendar(ctx, calendarID, role, userID)
	if err != nil {
		return nil, err
	}
	refs := make([]calendar.TimelineRef, 0, len(tls))
	for _, t := range tls {
		refs = append(refs, calendar.TimelineRef{
			ID: t.ID, Name: t.Name, Color: t.Color, Icon: t.Icon,
			Visibility: t.Visibility, EventCount: t.EventCount,
		})
	}
	return refs, nil
}

// calendarEntityCreatorAdapter wraps entities.EntityService to implement the
// calendar.EntityCreator interface — the cross-plugin write the event drawer's
// "create entity from event" action needs (C-CAL-EDITOR-EXPANSION PR1). The
// calendar plugin never imports the entities repo; it reaches the entities
// service through this adapter at the app boundary (plugin-isolation rule 8).
type calendarEntityCreatorAdapter struct {
	svc entities.EntityService
}

// ListEntityTypes projects the campaign's entity types into the calendar
// plugin's lightweight EntityTypeRef for the create-entity picker.
func (a *calendarEntityCreatorAdapter) ListEntityTypes(ctx context.Context, campaignID string) ([]calendar.EntityTypeRef, error) {
	ts, err := a.svc.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	refs := make([]calendar.EntityTypeRef, 0, len(ts))
	for _, t := range ts {
		refs = append(refs, calendar.EntityTypeRef{ID: t.ID, Name: t.Name, Slug: t.Slug})
	}
	return refs, nil
}

// CreateEntity creates a campaign entity of the given type via the entities
// service's own creation policy (which validates the type belongs to the
// campaign) and returns its id.
func (a *calendarEntityCreatorAdapter) CreateEntity(ctx context.Context, campaignID, userID string, typeID int, name string) (string, error) {
	ent, err := a.svc.Create(ctx, campaignID, userID, entities.CreateEntityInput{Name: name, EntityTypeID: typeID})
	if err != nil {
		return "", err
	}
	return ent.ID, nil
}

// calendarRSVPNotifierAdapter maps calendar.RSVPNotifier onto the sessions
// notifications service's generic NotifyUsers (C-CAL-RSVP-P1, rule 8). The
// notifications store is documented generic (T-B2); the calendar RSVP feature is
// its second writer, reaching it only through this narrow adapter.
type calendarRSVPNotifierAdapter struct {
	svc sessions.SessionService
}

// NotifyUsers forwards a bell fan-out to the sessions notifications service.
func (a *calendarRSVPNotifierAdapter) NotifyUsers(ctx context.Context, userIDs []string, campaignID, ntype, message, link string) error {
	return a.svc.NotifyUsers(ctx, userIDs, campaignID, ntype, message, link)
}

// calendarAvailabilityWriterAdapter maps calendar.AvailabilityExceptionWriter
// onto the sessions availability service (C-CAL-RSVP-P1, rule 8). SELF-write
// only: userID flows straight through from the redeemed RSVP token / the
// authenticated caller — never a value a request body controls.
type calendarAvailabilityWriterAdapter struct {
	svc sessions.SessionService
}

// ListMyExceptionDates projects the user's existing exceptions down to the set
// of dates they occur on, so "out this week" can skip hand-authored days.
func (a *calendarAvailabilityWriterAdapter) ListMyExceptionDates(ctx context.Context, campaignID, userID string) (map[string]bool, error) {
	excs, err := a.svc.ListMyExceptions(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	dates := make(map[string]bool, len(excs))
	for _, e := range excs {
		dates[e.OnDate] = true
	}
	return dates, nil
}

// AddFullDayUnavailable writes a full-day (0–1440) 'unavailable' exception for
// the given date in the caller's own recurring pattern (TZ is UTC — a full-day
// block is zone-agnostic; the sessions service still validates it as a real
// IANA zone).
func (a *calendarAvailabilityWriterAdapter) AddFullDayUnavailable(ctx context.Context, campaignID, userID, onDate string) error {
	return a.svc.AddMyException(ctx, campaignID, userID, sessions.AddExceptionRequest{
		OnDate:      onDate,
		StartMinute: 0,
		EndMinute:   1440,
		State:       "unavailable",
		TZ:          "UTC",
	})
}

// calendarEventListerAdapter wraps calendar.CalendarService to implement the
// timeline.CalendarEventLister interface. Lists all calendar events for the
// event picker when linking events to a timeline.
type calendarEventListerAdapter struct {
	svc calendar.CalendarService
}

// ListEventsForCalendar returns all events for a calendar as lightweight refs.
func (a *calendarEventListerAdapter) ListEventsForCalendar(ctx context.Context, calendarID string, role int) ([]timeline.CalendarEventRef, error) {
	cal, err := a.svc.GetCalendarByID(ctx, calendarID)
	if err != nil {
		return nil, err
	}
	if cal == nil {
		return nil, nil
	}

	// Use ListAllEvents for owner-level access (gets all events regardless of visibility).
	// For non-owners, use ListEventsForYear across a broad range.
	// ListAllEvents returns all events with owner visibility.
	events, err := a.svc.ListAllEvents(ctx, calendarID)
	if err != nil {
		return nil, err
	}

	refs := make([]timeline.CalendarEventRef, 0, len(events))
	for _, ev := range events {
		// Apply role-based visibility filter (dm_only = Owner only).
		if !permissions.CanSeeDmOnly(role) && ev.Visibility == "dm_only" {
			continue
		}
		refs = append(refs, timeline.CalendarEventRef{
			ID:         ev.ID,
			Name:       ev.Name,
			Year:       ev.Year,
			Month:      ev.Month,
			Day:        ev.Day,
			Category:   ev.Category,
			Visibility: ev.Visibility,
			EntityID:   ev.EntityID,
			EntityName: ev.EntityName,
			EntityIcon: ev.EntityIcon,
		})
	}
	return refs, nil
}

// calendarEraListerAdapter wraps calendar.CalendarService to implement the
// timeline.CalendarEraLister interface. Returns calendar eras for the D3
// visualization background bands.
type calendarEraListerAdapter struct {
	svc calendar.CalendarService
}

// ListEras returns all eras for a calendar as lightweight refs for the timeline viz.
// Uses GetCalendarByID which loads all sub-resources including eras.
func (a *calendarEraListerAdapter) ListEras(ctx context.Context, calendarID string) ([]timeline.CalendarEra, error) {
	cal, err := a.svc.GetCalendarByID(ctx, calendarID)
	if err != nil {
		return nil, err
	}
	if cal == nil {
		return nil, nil
	}

	refs := make([]timeline.CalendarEra, 0, len(cal.Eras))
	for _, e := range cal.Eras {
		refs = append(refs, timeline.CalendarEra{
			Name:      e.Name,
			StartYear: e.StartYear,
			EndYear:   e.EndYear,
			Color:     e.Color,
		})
	}
	return refs, nil
}

// wsSessionAuthAdapter wraps auth.AuthService to implement the
// websocket.SessionAuthenticator interface. Extracts the session cookie
// from the raw HTTP request and validates it.
type wsSessionAuthAdapter struct {
	svc auth.AuthService
}

// AuthenticateSessionForWS validates the session cookie and returns the user ID.
// Uses the scheme-appropriate cookie name (__Host- over HTTPS) so the WebSocket
// handshake reads the same cookie the browser was issued.
func (a *wsSessionAuthAdapter) AuthenticateSessionForWS(r *http.Request) (string, error) {
	token := auth.ReadSessionToken(r)
	if token == "" {
		return "", fmt.Errorf("no session cookie")
	}
	session, err := a.svc.ValidateSession(r.Context(), token)
	if err != nil {
		return "", fmt.Errorf("invalid session: %w", err)
	}
	return session.UserID, nil
}

// wsCampaignRoleAdapter wraps campaigns.CampaignService to implement the
// websocket.CampaignRoleLookup interface.
type wsCampaignRoleAdapter struct {
	svc campaigns.CampaignService
}

// GetUserCampaignRole returns the user's role in the campaign.
func (a *wsCampaignRoleAdapter) GetUserCampaignRole(ctx context.Context, campaignID, userID string) (int, error) {
	member, err := a.svc.GetMember(ctx, campaignID, userID)
	if err != nil {
		return 0, err
	}
	if member == nil {
		return 0, nil
	}
	return int(member.Role), nil
}

// IsUserDmGranted reports whether the campaign Owner has granted this user
// dm_only visibility via CampaignSettings.DmGrantIDs.
func (a *wsCampaignRoleAdapter) IsUserDmGranted(ctx context.Context, campaignID, userID string) (bool, error) {
	return a.svc.IsUserDmGranted(ctx, campaignID, userID)
}

// calendarEventPublisherAdapter bridges the websocket.EventBus to the
// calendar.CalendarEventPublisher interface.
type calendarEventPublisherAdapter struct {
	bus ws.EventBus
}

// PublishCalendarEvent translates calendar domain events into WebSocket messages.
//
// Internal eventType strings now match the public ws.MessageType values
// 1:1 (C-CAL-WS-DOTTED, 2026-05-19). Earlier emitters used short forms
// like "season.changed" and this adapter mapped them to the dotted public
// names; the indirection is gone now so service.go publishes the same
// string the bus broadcasts. Event-CRUD and date.advanced events keep
// their short internal form because they weren't in the C-CAL-WS-DOTTED
// scope — leaving them alone avoids touching consumers (test fixtures
// and any future Foundry sync code) that already see the dotted public
// names through the bus layer.
func (a *calendarEventPublisherAdapter) PublishCalendarEvent(eventType, campaignID, resourceID string, payload any) {
	if campaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch eventType {
	case "event.created":
		msgType = ws.MsgCalendarEventCreated
	case "event.updated":
		msgType = ws.MsgCalendarEventUpdated
	case "event.deleted":
		msgType = ws.MsgCalendarEventDeleted
	case "date.advanced":
		msgType = ws.MsgCalendarDateAdvanced
	case "calendar.weather.changed":
		msgType = ws.MsgCalendarWeatherChanged
	case "calendar.structure.updated":
		msgType = ws.MsgCalendarStructureUpdated
	case "calendar.season.changed":
		msgType = ws.MsgCalendarSeasonChanged
	case "calendar.era.changed":
		msgType = ws.MsgCalendarEraChanged
	case "calendar.moon.phase_changed":
		msgType = ws.MsgCalendarMoonPhaseChanged
	case "calendar.cycle.changed":
		msgType = ws.MsgCalendarCycleChanged
	case "calendar.festival.changed":
		msgType = ws.MsgCalendarFestivalChanged
	default:
		return
	}
	a.bus.Publish(ws.NewMessage(msgType, campaignID, resourceID, payload))
}

// entityEventPublisherAdapter bridges the websocket.EventBus to the
// entities.EntityEventPublisher interface.
type entityEventPublisherAdapter struct {
	bus ws.EventBus
}

// PublishEntityEvent translates entity domain events into WebSocket messages.
func (a *entityEventPublisherAdapter) PublishEntityEvent(eventType, campaignID, entityID string, entity *entities.Entity) {
	if campaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch eventType {
	case "created":
		msgType = ws.MsgEntityCreated
	case "updated":
		msgType = ws.MsgEntityUpdated
	case "deleted":
		msgType = ws.MsgEntityDeleted
	default:
		return
	}
	a.bus.Publish(ws.NewMessage(msgType, campaignID, entityID, entity))
}

// PublishEntityTypeEvent translates entity type domain events into WebSocket messages.
func (a *entityEventPublisherAdapter) PublishEntityTypeEvent(eventType, campaignID string, entityType *entities.EntityType) {
	if campaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch eventType {
	case "created":
		msgType = ws.MsgEntityTypeCreated
	case "updated":
		msgType = ws.MsgEntityTypeUpdated
	case "deleted":
		msgType = ws.MsgEntityTypeDeleted
	default:
		return
	}
	a.bus.Publish(ws.NewMessage(msgType, campaignID, fmt.Sprintf("%d", entityType.ID), entityType))
}

// sidebarConfigStore is the narrow slice of the campaign service the sidebar
// auto-adder needs: read the current config and write back the items. Narrowing
// it (from the full CampaignService) keeps the auto-add behavior unit-testable.
type sidebarConfigStore interface {
	GetSidebarConfig(ctx context.Context, campaignID string) (*campaigns.SidebarConfig, error)
	UpdateSidebarConfig(ctx context.Context, campaignID string, req campaigns.UpdateSidebarConfigRequest) error
}

// sidebarAutoAdderAdapter implements entities.SidebarAutoAdder by appending
// new entity types to the campaign's unified sidebar config.
type sidebarAutoAdderAdapter struct {
	campaignService sidebarConfigStore
}

func (a *sidebarAutoAdderAdapter) AddEntityTypeToSidebar(ctx context.Context, campaignID string, typeID int) error {
	cfg, err := a.campaignService.GetSidebarConfig(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("get sidebar config: %w", err)
	}

	// Only persist an auto-add when the campaign has an explicit, customized
	// items order. A campaign with empty Items renders the DEFAULT sidebar,
	// which the render injector (injectDefaultSidebarItems) already completes
	// with every top-level type — including this new one, in its natural
	// sort_order position. Persisting a lone category item here would instead
	// make the new type the only explicit entry and snap it to the FRONT of the
	// list, so we leave empty configs to the injector. Converted campaigns (the
	// reconciler put them on a non-empty Items array) fall through and correctly
	// auto-gain the new type appended in their customized order.
	if len(cfg.Items) == 0 {
		return nil
	}

	// Check if the type is already present to avoid duplicates.
	for _, item := range cfg.Items {
		if item.Type == "category" && item.TypeID == typeID {
			return nil
		}
	}

	// Insert before "all_pages" if it exists, otherwise append to end.
	newItem := campaigns.SidebarItem{
		Type:    "category",
		TypeID:  typeID,
		Visible: true,
	}

	inserted := false
	for i, item := range cfg.Items {
		if item.Type == "all_pages" {
			cfg.Items = append(cfg.Items[:i+1], cfg.Items[i:]...)
			cfg.Items[i] = newItem
			inserted = true
			break
		}
	}
	if !inserted {
		cfg.Items = append(cfg.Items, newItem)
	}

	// Merge-write: this path only changes Items, so send only Items —
	// under load-merge-write semantics (#473) the other config fields
	// are preserved server-side.
	return a.campaignService.UpdateSidebarConfig(ctx, campaignID,
		campaigns.UpdateSidebarConfigRequest{Items: &cfg.Items})
}

// defaultSidebarAddons are the addon shortcuts the default sidebar shows in its
// top nav (each rendered only when its addon is enabled). They mirror the
// pre-C-NAV-V3 legacy hardcoded links (Journal + Calendar) so a never-customized
// campaign renders the same top nav after the legacy fallback path was removed.
var defaultSidebarAddons = []campaigns.SidebarItem{
	{Type: "addon", Slug: "notes", Label: "Journal", Icon: "fa-book-open", Visible: true},
	{Type: "addon", Slug: "calendar", Label: "Calendar", Icon: "fa-calendar-days", Visible: true},
}

// injectDefaultSidebarItems completes a sidebar items array with the standard
// scaffold — Dashboard, the default addon shortcuts, every top-level entity
// type, and All Pages — adding only what is missing and preserving the caller's
// order for everything already present. It is the server mirror of
// injectMissing()/generateDefaults() in sidebar_editor.js and the reason an
// empty Items array (a never-customized or reconciler-skipped campaign) still
// renders the full default sidebar now that the legacy fallback path is gone.
// Non-persistent: it shapes one render only.
//
// Sub-category entity_types (parent_type_id != nil) are template variants of
// their parent and are never added as sidebar items.
func injectDefaultSidebarItems(items []campaigns.SidebarItem, types []layouts.SidebarEntityType) []campaigns.SidebarItem {
	hasDashboard, hasAllPages := false, false
	presentAddons := make(map[string]bool)
	presentCats := make(map[int]bool)
	for _, it := range items {
		switch it.Type {
		case "dashboard":
			hasDashboard = true
		case "all_pages":
			hasAllPages = true
		case "addon":
			presentAddons[it.Slug] = true
		case "category":
			presentCats[it.TypeID] = true
		}
	}

	if !hasDashboard {
		items = append([]campaigns.SidebarItem{{Type: "dashboard", Visible: true}}, items...)
	}
	for _, addon := range defaultSidebarAddons {
		if !presentAddons[addon.Slug] {
			items = append(items, addon)
		}
	}
	for _, et := range types {
		if et.ParentTypeID != nil {
			continue
		}
		if !presentCats[et.ID] {
			items = append(items, campaigns.SidebarItem{Type: "category", TypeID: et.ID, Visible: true})
		}
	}
	if !hasAllPages {
		items = append(items, campaigns.SidebarItem{Type: "all_pages", Visible: true})
	}
	return items
}

// noteEventPublisherAdapter bridges the websocket.EventBus to the
// notes.NoteEventPublisher interface.
type noteEventPublisherAdapter struct {
	bus ws.EventBus
}

// PublishNoteEvent translates note domain events into WebSocket messages.
func (a *noteEventPublisherAdapter) PublishNoteEvent(eventType, campaignID, noteID string, note *notes.Note) {
	if campaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch eventType {
	case "created":
		msgType = ws.MsgNoteCreated
	case "updated":
		msgType = ws.MsgNoteUpdated
	case "deleted":
		msgType = ws.MsgNoteDeleted
	default:
		return
	}
	a.bus.Publish(ws.NewMessage(msgType, campaignID, noteID, note))
}

// entityNotesNotifierHolder is the late-bound bridge from
// entity_notes.Service.Notify (function-typed) to the WebSocket bus.
// We construct the service before wsHub exists, so the holder lets us
// inject the bus once boot reaches the WebSocket setup. Until then,
// Notify is a safe no-op — mutations during startup don't broadcast,
// which matters not at all because no client is connected.
//
// The `bus` field is plain (no mutex) because there's no concurrent
// writer: it's set exactly once in RegisterRoutes between two
// synchronous statements, before the HTTP server starts accepting
// connections.
type entityNotesNotifierHolder struct {
	bus ws.EventBus
}

// Notify is the entity_notes.Notifier callback. Translates the
// service's string event into a websocket message type and broadcasts
// to the campaign that owns the note. Never to specific users — the
// audience filter is server-side on the next list refresh; clients
// just need a "something changed, refetch" nudge.
func (h *entityNotesNotifierHolder) Notify(event string, note *entity_notes.Note, _ entity_notes.Audience) {
	if h.bus == nil || note == nil || note.CampaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch event {
	case "entity_notes.created":
		msgType = ws.MsgEntityNoteCreated
	case "entity_notes.updated":
		msgType = ws.MsgEntityNoteUpdated
	case "entity_notes.deleted":
		msgType = ws.MsgEntityNoteDeleted
	default:
		return
	}
	// Payload is the note ID + entity ID only — clients refetch the list
	// for fresh ACL filtering. We deliberately do NOT broadcast the note
	// body so a private note's contents never leave the server, even
	// if a misconfigured client subscribes to the wrong campaign.
	h.bus.Publish(ws.NewMessage(msgType, note.CampaignID, note.ID, map[string]string{
		"entityId": note.EntityID,
		"noteId":   note.ID,
	}))
}

// mapEventPublisherAdapter bridges the websocket.EventBus to the maps.MapEventPublisher
// interface, translating domain events into WebSocket messages.
type mapEventPublisherAdapter struct {
	bus ws.EventBus
}

// publishWithAudience wraps ws.NewMessage with the RequiresDM audience
// flag derived from the source row. Pulled out so every map sub-resource
// emit funnels the same way — one place to audit, not five.
func (a *mapEventPublisherAdapter) publishWithAudience(msgType ws.MessageType, campaignID, resourceID string, payload any, dmOnly bool) {
	msg := ws.NewMessage(msgType, campaignID, resourceID, payload)
	msg.RequiresDM = dmOnly
	a.bus.Publish(msg)
}

// PublishDrawingEvent translates map drawing domain events into WebSocket messages.
func (a *mapEventPublisherAdapter) PublishDrawingEvent(eventType string, campaignID string, drawing *maps.Drawing) {
	if campaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch eventType {
	case "created":
		msgType = ws.MsgDrawingCreated
	case "updated":
		msgType = ws.MsgDrawingUpdated
	case "deleted":
		msgType = ws.MsgDrawingDeleted
	default:
		return
	}
	a.publishWithAudience(msgType, campaignID, drawing.ID, drawing, drawing.Visibility == "dm_only")
}

// PublishTokenEvent translates map token domain events into WebSocket messages.
// Tokens flagged is_hidden are GM-only — same gate as the SQL filter in
// drawing_repository.ListTokens.
func (a *mapEventPublisherAdapter) PublishTokenEvent(eventType string, campaignID string, token *maps.Token) {
	if campaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch eventType {
	case "created":
		msgType = ws.MsgTokenCreated
	case "updated":
		msgType = ws.MsgTokenUpdated
	case "deleted":
		msgType = ws.MsgTokenDeleted
	default:
		return
	}
	a.publishWithAudience(msgType, campaignID, token.ID, token, token.IsHidden)
}

// PublishTokenPositionEvent broadcasts a token position update via WebSocket.
// The fast-drag path doesn't carry the source token, so we conservatively
// treat position updates as everyone-visible — a hidden token's position
// is leaked to non-GMs only as percentage coordinates with no other
// metadata, which is the same shape clients already filter on receipt.
// To plug that gap fully, the service would need to thread is_hidden
// through PublishTokenPositionEvent; leave a TODO to revisit when the
// drag path can afford the extra fetch.
func (a *mapEventPublisherAdapter) PublishTokenPositionEvent(campaignID, tokenID string, x, y float64) {
	if campaignID == "" {
		return
	}
	a.bus.Publish(ws.NewMessage(ws.MsgTokenMoved, campaignID, tokenID, map[string]float64{
		"x": x,
		"y": y,
	}))
}

// PublishLayerEvent broadcasts a map layer event via WebSocket. Layers
// don't carry a visibility flag — they're z-order containers — so layer
// events are everyone-visible. Hiding individual drawings/tokens on a
// layer happens via their own visibility/is_hidden gates.
//
// Before C-MAP-EVT this adapter flattened all lifecycle events to
// MsgLayerUpdated, which forced Foundry to refetch the full layer list
// just to find out what changed. Now the eventType drives the message
// type so clients can discriminate create / update / delete and apply
// a targeted local mutation.
func (a *mapEventPublisherAdapter) PublishLayerEvent(eventType string, campaignID string, layer *maps.Layer) {
	if campaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch eventType {
	case "created":
		msgType = ws.MsgLayerCreated
	case "updated":
		msgType = ws.MsgLayerUpdated
	case "deleted":
		msgType = ws.MsgLayerDeleted
	default:
		return
	}
	a.bus.Publish(ws.NewMessage(msgType, campaignID, layer.ID, layer))
}

// PublishFogEvent broadcasts a fog-of-war event via WebSocket. All fog
// events are GM-only — non-GM clients should never learn the shape of
// the fog mask, since that'd reveal what they haven't explored yet.
//
// Before C-MAP-EVT this adapter flattened all lifecycle events to
// MsgFogUpdated and squeezed the actual event name into the payload.
// Promoting eventType to the message type lets clients dispatch
// without parsing the payload first; the redundant "event" key in the
// payload stays for backwards compatibility with the existing handler.
func (a *mapEventPublisherAdapter) PublishFogEvent(eventType string, campaignID, mapID string, region *maps.FogRegion) {
	if campaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch eventType {
	case "created":
		msgType = ws.MsgFogCreated
	case "updated", "reset":
		msgType = ws.MsgFogUpdated
	case "deleted":
		msgType = ws.MsgFogDeleted
	default:
		return
	}
	payload := map[string]any{
		"event":  eventType,
		"map_id": mapID,
	}
	if region != nil {
		payload["region"] = region
	}
	a.publishWithAudience(msgType, campaignID, mapID, payload, true)
}

// PublishMarkerEvent translates map marker domain events into WebSocket messages.
func (a *mapEventPublisherAdapter) PublishMarkerEvent(eventType string, campaignID string, marker *maps.Marker) {
	if campaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch eventType {
	case "created":
		msgType = ws.MsgMarkerCreated
	case "updated":
		msgType = ws.MsgMarkerUpdated
	case "deleted":
		msgType = ws.MsgMarkerDeleted
	default:
		return
	}
	a.publishWithAudience(msgType, campaignID, marker.ID, marker, marker.IsDMOnly())
}

// (foundryVTTBannerAdapter + GetFoundryModuleBanner removed in NW-2.2
// Chunk D2-cleanup. The campaigns show page now lazy-loads the banner
// via foundry_vtt's /foundry-vtt/show-banner-fragment route, which
// owns the adapter shape internally. Per
// cordinator/reports/chronicle/2026-05-26-c-d2-cleanup-verification.md.)

// foundryCampaignSettingsAdapter wraps campaigns.CampaignService to
// implement foundry_vtt.CampaignSettingsAdapter without creating
// a circular import. Reads/writes the FoundryModulePin field on
// CampaignSettings + checks campaign existence.
type foundryCampaignSettingsAdapter struct {
	svc campaigns.CampaignService
}

// SetFoundryModulePin delegates to the campaigns service's typed setter.
func (a *foundryCampaignSettingsAdapter) SetFoundryModulePin(ctx context.Context, campaignID, version string) error {
	return a.svc.SetFoundryModulePin(ctx, campaignID, version)
}

// GetFoundryModulePin delegates to the campaigns service's typed getter.
func (a *foundryCampaignSettingsAdapter) GetFoundryModulePin(ctx context.Context, campaignID string) (string, error) {
	return a.svc.GetFoundryModulePin(ctx, campaignID)
}

// SetFoundryModulePinMode delegates to the campaigns service. Added
// in C-FMC-ADMIN-UX-AUDIT Chunk 1; consumed by Chunk 3's owner-side
// UI (the new always-promote checkbox).
func (a *foundryCampaignSettingsAdapter) SetFoundryModulePinMode(ctx context.Context, campaignID, mode string) error {
	return a.svc.SetFoundryModulePinMode(ctx, campaignID, mode)
}

// GetFoundryModulePinMode delegates to the campaigns service. Added
// in C-FMC-ADMIN-UX-AUDIT Chunk 1; consumed by OwnerTabData
// population so the owner-side templ (Chunk 3) renders the right
// initial state.
func (a *foundryCampaignSettingsAdapter) GetFoundryModulePinMode(ctx context.Context, campaignID string) (string, error) {
	return a.svc.GetFoundryModulePinMode(ctx, campaignID)
}

// CampaignExists is the existence check foundry_vtt's install-URL
// builder and token-rotation flow use to reject unknown campaigns.
func (a *foundryCampaignSettingsAdapter) CampaignExists(ctx context.Context, campaignID string) (bool, error) {
	return a.svc.CampaignExistsByID(ctx, campaignID)
}

// foundryCampaignOwnerLookupAdapter resolves a campaign's owner email
// for foundry_vtt's NotifyOlderCampaigns SMTP fan-out. Pulls the
// creator's user row, then their email from the auth service.
// Best-effort — soft-fails to ("", "", err) which the notify path
// treats as "skip email but still log the audit event."
type foundryCampaignOwnerLookupAdapter struct {
	campaignSvc campaigns.CampaignService
	authSvc     auth.AuthService
}

// GetCampaignOwnerEmail returns (email, displayName, error). The
// foundry_vtt service handles a non-nil error by skipping the
// email send and falling back to the in-app banner.
func (a *foundryCampaignOwnerLookupAdapter) GetCampaignOwnerEmail(ctx context.Context, campaignID string) (string, string, error) {
	c, err := a.campaignSvc.GetByID(ctx, campaignID)
	if err != nil {
		return "", "", err
	}
	if c == nil {
		return "", "", nil
	}
	// Campaign's original creator is treated as the primary owner for
	// notify emails. Campaigns can have multiple RoleOwner members but
	// the notify path is "tell THE owner" — multi-owner emailing is
	// a future iteration.
	user, err := a.authSvc.GetUser(ctx, c.CreatedBy)
	if err != nil || user == nil {
		return "", "", err
	}
	display := user.DisplayName
	if display == "" {
		display = user.Email
	}
	return user.Email, display, nil
}

// mediaMemberCheckerAdapter wraps campaigns.CampaignService to implement the
// media.MemberChecker interface without creating a circular import.
// Uses background context since membership checks happen on unauthenticated
// serve requests where the request context may not carry campaign data.
type mediaMemberCheckerAdapter struct {
	svc campaigns.CampaignService
}

// IsCampaignMember checks if the user is a member of the campaign.
func (a *mediaMemberCheckerAdapter) IsCampaignMember(campaignID, userID string) bool {
	member, err := a.svc.GetMember(context.Background(), campaignID, userID)
	return err == nil && member != nil
}

// storageLimiterAdapter wraps settings.SettingsService to implement the
// media.StorageLimiter interface without creating a circular import.
type storageLimiterAdapter struct {
	svc settings.SettingsService
}

// GetEffectiveLimits resolves storage limits for a user+campaign context.
func (a *storageLimiterAdapter) GetEffectiveLimits(ctx context.Context, userID, campaignID string) (int64, int64, int, error) {
	limits, err := a.svc.GetEffectiveLimits(ctx, userID, campaignID)
	if err != nil {
		return 0, 0, 0, err
	}
	return limits.MaxUploadSize, limits.MaxTotalStorage, limits.MaxFiles, nil
}

// registrationInviteCheckerAdapter wraps the campaigns invite service to satisfy
// auth.RegistrationInviteChecker, so the auth plugin can validate invite-only
// registrations without importing the campaigns plugin (T-B2).
type registrationInviteCheckerAdapter struct {
	invites campaigns.InviteService
}

// IsRegistrationInviteValid reports whether the token names a live invite that
// can still be used to create an account: it exists, has not been accepted, has
// not expired, and — when email is non-empty — was issued to that address.
// Campaign invites are email-scoped, so binding the token to the registering
// email makes an invite effectively single-use (a second registration for the
// same address hits email-uniqueness; no other address can consume the invite).
func (a *registrationInviteCheckerAdapter) IsRegistrationInviteValid(ctx context.Context, token, email string) bool {
	if token == "" {
		return false
	}
	inv, err := a.invites.GetInviteByToken(ctx, token)
	if err != nil || inv == nil {
		return false
	}
	if inv.AcceptedAt != nil || inv.IsExpired() {
		return false
	}
	if email != "" && !strings.EqualFold(strings.TrimSpace(inv.Email), strings.TrimSpace(email)) {
		return false
	}
	return true
}

// widgetBlockListerAdapter bridges extensions.Handler and systems.SystemHandler
// to entities.WidgetBlockLister. Converts widget metadata from both extension
// widgets and system-provided widgets into entity block metadata.
type widgetBlockListerAdapter struct {
	extHandler *extensions.Handler
	sysHandler *systems.SystemHandler
}

// GetWidgetBlockMetas returns widget blocks from both extensions and game systems.
func (a *widgetBlockListerAdapter) GetWidgetBlockMetas(ctx context.Context, campaignID string) []entities.BlockMeta {
	var metas []entities.BlockMeta

	// Extension widgets.
	if a.extHandler != nil {
		infos := a.extHandler.GetWidgetBlockInfos(ctx, campaignID)
		for _, info := range infos {
			icon := info.Icon
			if icon == "" {
				icon = "fa-puzzle-piece"
			}
			metas = append(metas, entities.BlockMeta{
				Type:        "ext_widget",
				Label:       info.Name,
				Icon:        icon,
				Description: info.Description,
				WidgetSlug:  info.Slug,
			})
		}
	}

	// System-provided widgets.
	if a.sysHandler != nil {
		metas = append(metas, a.sysHandler.GetSystemWidgetBlockMetas(ctx, campaignID)...)
	}

	return metas
}

// mentionLinkAdapter wraps entities.EntityService to implement the
// relations.MentionLinkProvider interface, supplying @mention link data
// for the graph visualization without creating a circular import.
type mentionLinkAdapter struct {
	svc entities.EntityService
}

// GetMentionLinksForGraph returns @mention references across a campaign for
// the relations graph. Converts between entity and relations package types.
func (a *mentionLinkAdapter) GetMentionLinksForGraph(ctx context.Context, campaignID string, includeDmOnly bool, userID string) ([]relations.MentionLinkData, error) {
	// Determine role for visibility filtering: DM sees everything, others
	// see only entities they have access to.
	role := permissions.RolePlayer
	if includeDmOnly {
		role = permissions.RoleOwner
	}

	links, err := a.svc.GetMentionLinks(ctx, campaignID, role, userID)
	if err != nil {
		return nil, err
	}
	result := make([]relations.MentionLinkData, len(links))
	for i, l := range links {
		result[i] = relations.MentionLinkData{
			SourceEntityID: l.SourceEntityID,
			TargetEntityID: l.TargetEntityID,
		}
	}
	return result, nil
}

// entityTypeListerForGraphAdapter wraps entities.EntityService to implement the
// relations.EntityTypeListerForGraph interface for the graph filter dropdown.
type entityTypeListerForGraphAdapter struct {
	svc entities.EntityService
}

// ListEntityTypesForGraph returns entity types as lightweight summaries.
func (a *entityTypeListerForGraphAdapter) ListEntityTypesForGraph(ctx context.Context, campaignID string) ([]relations.EntityTypeSummary, error) {
	etypes, err := a.svc.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	result := make([]relations.EntityTypeSummary, 0, len(etypes))
	for _, et := range etypes {
		if !et.Enabled {
			continue
		}
		result = append(result, relations.EntityTypeSummary{
			Slug:  et.Slug,
			Name:  et.Name,
			Color: et.Color,
			Icon:  et.Icon,
		})
	}
	return result, nil
}

// entityAccessAdapter wraps entities.EntityService to implement the entity-access
// seams the post / relation / tag widgets consume (posts.EntityGate,
// tags.EntityGate, relations.EntityGate / relations.EntityViewFilter). It lets
// those widgets honor entity visibility + campaign binding without importing the
// entities repo directly (plugin-isolation, rule 8). Introduced by
// cordinator/dispatches/chronicle/C-PUBLIC-VIEW-FIX-R2.md to close the anon
// private-entity leaks + cross-campaign IDOR. Cites: 2026-05-21-core-tenets §T-B1,
// §T-B2.
type entityAccessAdapter struct {
	svc entities.EntityService
}

// ResolveViewableEntity returns the entity's owning campaign ID and whether the
// viewer (role, userID) may view it. Mirrors the entity Show page gate
// (entities/handler.go Show): GetByID for the campaign binding + CheckEntityAccess
// for visibility. A missing entity surfaces as the GetByID error (NotFound), which
// the caller renders as 404.
func (a *entityAccessAdapter) ResolveViewableEntity(ctx context.Context, entityID string, role int, userID string) (string, bool, error) {
	ent, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		return "", false, err
	}
	access, err := a.svc.CheckEntityAccess(ctx, entityID, role, userID)
	if err != nil {
		return "", false, err
	}
	return ent.CampaignID, access.CanView, nil
}

// FilterViewableEntityIDs returns the subset of entityIDs the viewer may view,
// batched (no N+1). Used to hide private-entity relation targets + graph nodes.
func (a *entityAccessAdapter) FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error) {
	return a.svc.FilterViewableEntityIDs(ctx, campaignID, entityIDs, role, userID)
}

// npcEntityTypeFinderAdapter wraps entities.EntityService to implement the
// npcs.EntityTypeFinder interface. Resolves the "characters" entity type ID
// for the NPC gallery without creating a circular import.
type npcEntityTypeFinderAdapter struct {
	svc entities.EntityService
}

// FindCharacterTypeID looks up the "characters" entity type for a campaign.
func (a *npcEntityTypeFinderAdapter) FindCharacterTypeID(ctx context.Context, campaignID string) (int, error) {
	et, err := a.svc.GetEntityTypeBySlug(ctx, campaignID, "characters")
	if err != nil {
		return 0, err
	}
	return et.ID, nil
}

// npcVisibilityTogglerAdapter wraps entities.EntityService to implement the
// npcs.VisibilityToggler interface for the reveal toggle.
type npcVisibilityTogglerAdapter struct {
	svc entities.EntityService
}

// TogglePrivate flips an entity's is_private flag, scoped to campaignID so the
// npcs reveal toggle can't reach across campaigns (SEC-IDOR-1).
func (a *npcVisibilityTogglerAdapter) TogglePrivate(ctx context.Context, entityID, campaignID string) (bool, error) {
	return a.svc.TogglePrivateInCampaign(ctx, entityID, campaignID)
}

// auditEntityViewGuardAdapter wraps entities.EntityService to implement the
// audit.EntityViewGuard interface. Resolves an entity's campaign and the
// caller's view permission so the entity-history endpoint can enforce campaign
// ownership + per-entity visibility (SEC-IDOR-2).
type auditEntityViewGuardAdapter struct {
	svc entities.EntityService
}

// ResolveEntityView returns the entity's campaign and whether the given
// role/user may view it, mirroring the gate entities.GetEntry applies.
func (a *auditEntityViewGuardAdapter) ResolveEntityView(ctx context.Context, entityID string, role int, userID string) (string, bool, error) {
	e, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		return "", false, err
	}
	access, err := a.svc.CheckEntityAccess(ctx, entityID, role, userID)
	if err != nil {
		return e.CampaignID, false, err
	}
	return e.CampaignID, access.CanView, nil
}

// armoryItemTypeFinderAdapter wraps entities.EntityService to implement the
// armory.ItemTypeFinder interface. Resolves item-category entity types using
// the preset_category column.
type armoryItemTypeFinderAdapter struct {
	svc entities.EntityService
}

// FindItemTypeIDs returns the IDs of entity types with preset_category "item".
func (a *armoryItemTypeFinderAdapter) FindItemTypeIDs(ctx context.Context, campaignID string) ([]int, error) {
	types, err := a.svc.GetEntityTypesByPresetCategory(ctx, campaignID, "item")
	if err != nil {
		return nil, err
	}
	ids := make([]int, len(types))
	for i, t := range types {
		ids[i] = t.ID
	}
	return ids, nil
}

// FindItemTypes returns item-category entity types for the Armory filter dropdown.
func (a *armoryItemTypeFinderAdapter) FindItemTypes(ctx context.Context, campaignID string) ([]armory.ItemTypeInfo, error) {
	types, err := a.svc.GetEntityTypesByPresetCategory(ctx, campaignID, "item")
	if err != nil {
		return nil, err
	}
	infos := make([]armory.ItemTypeInfo, len(types))
	for i, t := range types {
		infos[i] = armory.ItemTypeInfo{
			ID:    t.ID,
			Name:  t.Name,
			Icon:  t.Icon,
			Color: t.Color,
		}
	}
	return infos, nil
}

// armoryRelationMetadataAdapter wraps the relations service to implement
// armory.RelationMetadataUpdater. Used by the transaction service to decrement
// shop stock when a purchase is made.
type armoryRelationMetadataAdapter struct {
	svc relations.RelationService
}

// UpdateMetadata updates the metadata JSON for a relation.
func (a *armoryRelationMetadataAdapter) UpdateMetadata(ctx context.Context, id int, metadata json.RawMessage) error {
	return a.svc.UpdateMetadata(ctx, id, metadata)
}

// entityMapVerifierAdapter wraps maps.MapService to implement
// entities.MapCampaignVerifier. Used by entityService.AssignMap to
// confirm a map exists AND lives in the entity's own campaign before
// writing entities.map_id. The FK alone catches non-existence; this
// adapter closes the cross-campaign IDOR (Scribe in campaign A cannot
// point an entity at a map from campaign B).
type entityMapVerifierAdapter struct {
	svc maps.MapService
}

// MapExistsInCampaign returns true only when the map exists AND its
// CampaignID matches. Map-not-found is NOT an error here — it's just
// false, since the caller's question is "is this a valid choice?"
func (a *entityMapVerifierAdapter) MapExistsInCampaign(ctx context.Context, mapID, campaignID string) (bool, error) {
	m, err := a.svc.GetMap(ctx, mapID)
	if err != nil {
		// Treat "not found" as a clean false; bubble other errors.
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	if m == nil {
		return false, nil
	}
	return m.CampaignID == campaignID, nil
}

// armoryBuyerAccessAdapter wraps entities.EntityService to implement
// armory.BuyerAccessChecker. Used by the transaction service to verify
// the calling user can act on the buyer entity (own / shared / Owner /
// Scribe-with-grant) before a Purchase. Mitigates buyer_entity_id
// spoofing from clients with a single per-campaign API key.
//
// Acting-as is mapped to CanEdit (not just CanView) because purchasing
// modifies the buyer's state — currency deduction, transaction log entry.
// CanView would let any campaign member buy as anyone, which is the
// vulnerability we're closing.
type armoryBuyerAccessAdapter struct {
	svc entities.EntityService
}

// CanUserActAsBuyer returns true if the user has edit-level access to the
// buyer entity. Owners short-circuit true at the entity-service layer.
func (a *armoryBuyerAccessAdapter) CanUserActAsBuyer(ctx context.Context, entityID, userID string, role int) (bool, error) {
	perm, err := a.svc.CheckEntityAccess(ctx, entityID, role, userID)
	if err != nil {
		return false, err
	}
	if perm == nil {
		return false, nil
	}
	return perm.CanEdit, nil
}

// armoryRelationFinderAdapter wraps the relations service to implement
// armory.RelationFinder. Used by the transaction service to validate stock
// before a purchase.
type armoryRelationFinderAdapter struct {
	svc relations.RelationService
}

// GetByID retrieves a relation by ID, mapping to the armory.RelationInfo type.
func (a *armoryRelationFinderAdapter) GetByID(ctx context.Context, id int) (*armory.RelationInfo, error) {
	rel, err := a.svc.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &armory.RelationInfo{
		ID:         rel.ID,
		Metadata:   rel.Metadata,
		CampaignID: rel.CampaignID,
	}, nil
}

// loadSystemsFromPackages scans installed system packages and loads them into
// the system registry. Package-managed systems override bundled ones.
func (a *App) loadSystemsFromPackages(pkgService packages.PackageService) {
	pkgs, err := pkgService.ListPackages(context.Background())
	if err != nil {
		slog.Warn("failed to list packages for system loading", slog.Any("error", err))
		return
	}

	for _, pkg := range pkgs {
		if pkg.Type != packages.PackageTypeSystem {
			continue
		}
		if pkg.InstallPath == "" || pkg.Status != packages.StatusApproved {
			continue
		}
		if err := systems.LoadAdditionalDir(pkg.InstallPath); err != nil {
			slog.Warn("failed to load package system",
				slog.String("package", pkg.Slug),
				slog.String("path", pkg.InstallPath),
				slog.Any("error", err),
			)
		}
	}
}

// registerManifestRenderers walks every loaded system manifest and
// auto-registers an EntityShowRenderer for every entry in its `renderers`
// field. This is the CH4.5 path that lets JSON-only system packages ship
// page-level renderers without shipping Go: the manifest declares
// {slug, widget} pairs, and we register a renderer that emits the widget
// mount point and lets boot.js take over.
//
// Must be called after loadSystemsFromPackages (so packaged manifests are
// in the registry) and before SetGlobalEntityShowRendererRegistry (so
// the global is published with renderers already in place — no observable
// half-built state for incoming requests).
func registerManifestRenderers(showRegistry *entities.EntityShowRendererRegistry) {
	for _, manifest := range systems.Registry() {
		if manifest == nil {
			continue
		}
		for _, r := range manifest.Renderers {
			renderer := entities.MakeWidgetMountRenderer(r.Widget)
			// A renderer binds by slug (a system's own type) or by preset_category
			// (the system-agnostic seam — fills a Chronicle-owned category). The
			// manifest validator guarantees exactly one is set.
			if r.Slug != "" {
				showRegistry.Register(r.Slug, renderer)
			} else if r.PresetCategory != "" {
				showRegistry.RegisterByPresetCategory(r.PresetCategory, renderer)
			}
		}
	}
}

// RegisterRoutes sets up all application routes. It registers public routes
// directly and delegates to each plugin's route registration function.
//
// This is the single place where all routes are aggregated. When a new
// plugin is added, its routes are registered here.
func (a *App) RegisterRoutes() {
	e := a.Echo

	// --- Public Routes (no auth required) ---

	// Health check endpoint for Docker/Cosmos health monitoring.
	// Pings both MariaDB and Redis to report actual infrastructure health.
	// Registered on both /healthz (Kubernetes convention) and /health (common alias).
	healthHandler := func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
		defer cancel()

		// Log full errors server-side but return only generic component names
		// to avoid leaking internal hostnames, ports, and driver details.
		if err := a.DB.PingContext(ctx); err != nil {
			slog.Error("health check failed: mariadb", slog.Any("error", err))
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"status": "unhealthy",
				"error":  "mariadb unavailable",
			})
		}
		if err := a.Redis.Ping(ctx).Err(); err != nil {
			slog.Error("health check failed: redis", slog.Any("error", err))
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"status": "unhealthy",
				"error":  "redis unavailable",
			})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}
	e.GET("/healthz", healthHandler)
	e.GET("/health", healthHandler)

	// --- Plugin Routes ---

	// Auth plugin: login, register, logout (public routes).
	authRepo := auth.NewUserRepository(a.DB)
	authService := auth.NewAuthService(authRepo, a.Redis, a.Config.Auth.SessionTTL)
	authHandler := auth.NewHandler(authService, a.Config.Auth.SessionTTL)
	auth.RegisterRoutes(e, authHandler)

	// SMTP plugin: outbound email for transfers, password resets.
	smtpRepo := smtp.NewSMTPRepository(a.DB)
	smtpService := smtp.NewSMTPService(smtpRepo, a.Config.Auth.SecretKey)
	smtpHandler := smtp.NewHandler(smtpService)

	// NW-2.2 Chunk A pilot: register smtp in the App's metadata registry.
	// Per cordinator/decisions/2026-05-23-plugin-registration.md. Slug is
	// the canonical plugin identifier; HealthCheck wraps the existing
	// PluginHealth registry lookup so the registry surface exposes a
	// uniform health signal.
	a.registerPlugin(PluginRegistration{
		Slug: smtp.PluginSlug,
		HealthCheck: func() error {
			if a.PluginHealth != nil && !a.PluginHealth.IsHealthy(smtp.PluginHealthKey) {
				return errors.New("smtp schema unhealthy")
			}
			return nil
		},
	})

	// Wire SMTP into auth service for password reset emails.
	auth.ConfigureMailSender(authService, smtpService, a.Config.BaseURL)

	// Entities plugin: entity types + entity CRUD (must be created before
	// campaigns so we can pass EntityService as the EntityTypeSeeder).
	entityTypeRepo := entities.NewEntityTypeRepository(a.DB)
	entityRepo := entities.NewEntityRepository(a.DB)
	entityPermRepo := entities.NewEntityPermissionRepository(a.DB)
	entityService := entities.NewEntityService(entityRepo, entityTypeRepo, entityPermRepo)

	// One-shot heal of legacy auto-pluralize defaults that produced
	// "Mapss"-style values (name="Maps", plural="Mapss"). Idempotent;
	// failures are logged but never block boot. Runs in a goroutine
	// so a hung query can't stall startup.
	go func() {
		n, err := entityService.HealAutoPluralizedTypes(context.Background())
		if err != nil {
			slog.Warn("entity_types: doubled-plural heal failed", slog.Any("error", err))
			return
		}
		if n > 0 {
			slog.Info("entity_types: doubled-plural heal corrected rows", slog.Int("rows", n))
		}
	}()

	// One-shot boot reconcilers for entity_types, run SERIALLY in a single
	// goroutine. The permissions and player-notes backfills both read a full
	// pre-backfill snapshot and then rewrite the whole layout_json per row, so
	// running them as two uncoordinated goroutines let the second clobber the
	// first's block on a type missing BOTH — a type would end up with only one
	// of {permissions, entity_notes} until the next boot (#514 backfill
	// lost-update race, coordinator verification). Chaining them serializes
	// the writes: permissions completes before player-notes reads. The
	// gm_only field-flag sync runs last; it touches a different column
	// (fields, not layout_json) so it can't clobber the layout backfills, but
	// keeping all entity_types reconcilers in one ordered goroutine is the
	// simplest guarantee. Each step is idempotent; a failure is logged and the
	// chain continues so one bad step can't strand the others.
	go func() {
		ctx := context.Background()
		if n, err := entityService.EnsurePermissionsBlockInDefaults(ctx); err != nil {
			slog.Warn("entity_types: permissions block backfill failed", slog.Any("error", err))
		} else if n > 0 {
			slog.Info("entity_types: permissions block backfill added to layouts", slog.Int("rows", n))
		}

		// Player Notes was only wired into new default layouts, so custom
		// sub-categories created earlier never showed the block even with the
		// addon enabled (cordinator#7).
		if n, err := entityService.EnsureEntityNotesBlockInDefaults(ctx); err != nil {
			slog.Warn("entity_types: player-notes block backfill failed", slog.Any("error", err))
		} else if n > 0 {
			slog.Info("entity_types: player-notes block backfill added to layouts", slog.Int("rows", n))
		}

		// Converge gm_only field flags from installed system manifests onto
		// existing types so the GM-field egress filter (audit M-1) covers
		// characters created before the manifest carried gm_only.
		reconcileFieldGMFlags(ctx, entityService)

		// Same convergence for owner_only field flags (C-FIELDS-OWNER-FILTER)
		// so backstory-style fields become owner-private on types created
		// before the manifest carried owner_only.
		reconcileFieldOwnerOnlyFlags(ctx, entityService)
	}()

	// Campaigns plugin: CRUD, membership, ownership transfer.
	// EntityService is passed as EntityTypeSeeder to seed defaults on campaign creation.
	userFinder := campaigns.NewUserFinderAdapter(authRepo)
	campaignRepo := campaigns.NewCampaignRepository(a.DB)
	campaignService := campaigns.NewCampaignService(campaignRepo, userFinder, smtpService, entityService, a.Config.BaseURL)

	// One-time, idempotent boot reconciler: convert any campaign still on the
	// legacy sidebar model onto the unified items model (C-NAV-V3). Runs
	// synchronously before serving so a straggler never renders the default
	// sidebar in place of its saved order. Safe on every boot (no-op once
	// converted); best-effort so a failure can't block startup.
	if n, err := campaignService.EnsureSidebarItems(context.Background()); err != nil {
		slog.Error("sidebar items reconcile failed", slog.String("error", err.Error()))
	} else if n > 0 {
		slog.Info("sidebar items reconcile: converted legacy campaigns", slog.Int("campaigns", n))
	}

	campaignHandler := campaigns.NewHandler(campaignService)
	campaignHandler.SetBaseURL(a.Config.BaseURL)
	campaignHandler.SetEntityLister(&entityTypeListerAdapter{svc: entityService})
	campaignHandler.SetLayoutFetcher(&entityTypeLayoutFetcherAdapter{svc: entityService})
	campaignHandler.SetRecentEntityLister(&recentEntityListerAdapter{svc: entityService})
	groupRepo := campaigns.NewGroupRepository(a.DB)
	groupService := campaigns.NewGroupService(groupRepo)
	campaignHandler.SetGroupService(groupService)
	campaigns.RegisterRoutes(e, campaignHandler, campaignService, authService)

	// Campaign invites.
	inviteRepo := campaigns.NewInviteRepository(a.DB)
	inviteService := campaigns.NewInviteService(inviteRepo, campaignRepo, smtpService, a.Config.BaseURL)
	inviteHandler := campaigns.NewInviteHandler(inviteService, campaignService, a.Config.BaseURL)
	campaigns.RegisterInviteRoutes(e, inviteHandler, campaignService, authService)

	// Discover page (/) -- browse public campaigns. Uses OptionalAuth so
	// authenticated users get the App layout with sidebar, while guests
	// see a standalone page with signup CTA.
	e.GET("/", func(c echo.Context) error {
		publicCampaigns, err := campaignService.ListPublic(c.Request().Context(), 24)
		if err != nil {
			slog.Warn("failed to load public campaigns for discover page", slog.Any("error", err))
			publicCampaigns = nil
		}
		if auth.GetSession(c) != nil {
			return middleware.Render(c, http.StatusOK, pages.DiscoverAuthPage(publicCampaigns))
		}
		return middleware.Render(c, http.StatusOK, pages.DiscoverPublicPage(publicCampaigns))
	}, auth.OptionalAuth(authService))

	// About/Welcome page -- Chronicle marketing and feature highlights.
	e.GET("/about", func(c echo.Context) error {
		return middleware.Render(c, http.StatusOK, pages.AboutPage())
	}, auth.OptionalAuth(authService))

	// Entity routes (campaign-scoped, registered after campaign service exists).
	sidebarNodeRepo := entities.NewSidebarNodeRepository(a.DB)
	favoriteRepo := entities.NewFavoriteRepository(a.DB)
	entityHandler := entities.NewHandler(entityService)
	entityHandler.SetSidebarNodeRepo(sidebarNodeRepo)
	entityHandler.SetFavoriteRepo(favoriteRepo)
	entityHandler.SetSavedFilterRepo(entities.NewSavedFilterRepository(a.DB))
	entities.RegisterRoutes(e, entityHandler, campaignService, authService)

	// Expose the entities plugin's embedded static assets at
	// /static/plugins/entities/ (currently js/characters.js, the Characters
	// page's mini→full launch enhancement). Per
	// cordinator/decisions/2026-05-25-plugin-static-assets.md.
	a.registerPlugin(PluginRegistration{
		Slug:     entities.PluginSlug,
		StaticFS: echo.MustSubFS(entities.StaticAssetsFS, "static"),
	})

	// Content template routes (entity content blueprints).
	contentTemplateRepo := entities.NewContentTemplateRepository(a.DB)
	contentTemplateService := entities.NewContentTemplateService(contentTemplateRepo, entityTypeRepo)
	contentTemplateHandler := entities.NewContentTemplateHandler(contentTemplateService)
	entities.RegisterContentTemplateRoutes(e, contentTemplateHandler, campaignService, authService)
	campaignService.SetContentTemplateSeeder(contentTemplateService)
	entityHandler.SetContentTemplateService(contentTemplateService)

	// Worldbuilding prompt routes (guided writing prompts for content creators).
	wbPromptRepo := entities.NewWorldbuildingPromptRepository(a.DB)
	wbPromptService := entities.NewWorldbuildingPromptService(wbPromptRepo, entityTypeRepo)
	wbPromptHandler := entities.NewWorldbuildingPromptHandler(wbPromptService)
	entities.RegisterWorldbuildingPromptRoutes(e, wbPromptHandler, campaignService, authService)
	campaignService.SetWorldbuildingPromptSeeder(wbPromptService)

	// Layout preset routes (reusable page layout configurations).
	layoutPresetRepo := entities.NewLayoutPresetRepository(a.DB)
	layoutPresetService := entities.NewLayoutPresetService(layoutPresetRepo)
	layoutPresetHandler := entities.NewLayoutPresetHandler(layoutPresetService)
	entities.RegisterLayoutPresetRoutes(e, layoutPresetHandler, campaignService, authService)
	campaignService.SetLayoutPresetSeeder(layoutPresetService)

	// Media plugin: file upload, storage, thumbnailing, serving.
	// Graceful degradation: if the media directory can't be created, log a warning
	// but don't crash -- the rest of the app keeps running.
	mediaRepo := media.NewMediaRepository(a.DB)
	mediaService := media.NewMediaService(mediaRepo, a.Config.Upload.MediaPath, a.Config.Upload.MaxSize)
	if err := mediaService.ValidateMediaPath(); err != nil {
		slog.Warn("media storage validation failed; uploads may not work",
			slog.Any("error", err),
		)
	}
	mediaHandler := media.NewHandler(mediaService)

	// Settings service is built here (instead of with the other admin
	// services below) because the media body-limit middleware needs to
	// resolve the live max-upload-size from settings on every request.
	// Without this ordering, the body-limit would freeze at the env-var
	// default forever and admin changes wouldn't take effect — which is
	// exactly the bug operators hit when raising the cap from 10 MB.
	settingsRepo := settings.NewSettingsRepository(a.DB)
	settingsService := settings.NewSettingsService(settingsRepo)
	mediaService.SetStorageLimiter(&storageLimiterAdapter{svc: settingsService})

	// Beta registration gate (B-R4): the auth service reads the site registration
	// mode from settings and validates invite-only signups against live campaign
	// invites. Both deps are optional at the auth layer (nil ⇒ open), wired here
	// now that settings + invites exist.
	auth.ConfigureRegistrationGate(authService, settingsService, &registrationInviteCheckerAdapter{invites: inviteService})

	// Migration 26 added media_files.content_hash for per-campaign upload
	// dedup. Existing rows from before the migration have NULL hashes —
	// run the backfill in a detached goroutine so a campaign with
	// thousands of legacy media files doesn't stall startup. New uploads
	// always populate the hash inline; this only catches the gap.
	go func() {
		bgCtx := context.Background()
		n, err := mediaService.BackfillContentHashes(bgCtx, 100)
		if err != nil {
			slog.Warn("media: content_hash backfill aborted", slog.Any("error", err), slog.Int("hashed_so_far", n))
			return
		}
		if n > 0 {
			slog.Info("media: content_hash backfill complete", slog.Int("hashed", n))
		}
	}()

	// Resolver consulted by the media body-limit middleware on every
	// /media/upload to honor the live admin-configured limit. Falls back
	// to the env-var value if the settings lookup fails so a transient
	// DB hiccup can't block all uploads.
	resolveMaxUpload := func(c echo.Context) int64 {
		userID := auth.GetUserID(c)
		if eff, err := settingsService.GetEffectiveLimits(c.Request().Context(), userID, ""); err == nil && eff != nil {
			if eff.MaxUploadSize > 0 {
				return eff.MaxUploadSize
			}
		}
		return a.Config.Upload.MaxSize
	}

	// Initialize HMAC URL signer for secure media access.
	//
	// The same secret feeds foundry_vtt.NewTokenSigner below, where
	// it signs the per-campaign manifest tokens Foundry stores
	// indefinitely. Auto-generating in-memory and discarding on
	// restart used to silently invalidate every outstanding Foundry
	// token (cordinator Issue #17 / C-UPDATER-MANIFEST-403).
	// LoadOrInitSigningSecret persists the auto-generated secret so
	// it survives restarts; env-managed deploys see no behavior change.
	signingSecret, secretSource, secretErr := media.LoadOrInitSigningSecret(
		a.Config.Upload.SigningSecret,
		a.Config.Upload.SigningSecretFile,
	)
	switch secretSource {
	case media.SecretFromEnv:
		// Operator-managed via MEDIA_SIGNING_SECRET; nothing to log.
	case media.SecretFromFile:
		slog.Info("media signing secret loaded from persisted file",
			slog.String("path", a.Config.Upload.SigningSecretFile))
	case media.SecretGeneratedAndPersisted:
		slog.Warn("MEDIA_SIGNING_SECRET not set; generated a new secret and "+
			"persisted it. Subsequent restarts will reuse this secret. For "+
			"production, set MEDIA_SIGNING_SECRET in env so the secret is "+
			"managed alongside other credentials.",
			slog.String("path", a.Config.Upload.SigningSecretFile))
	case media.SecretGeneratedInMemory:
		slog.Error("MEDIA_SIGNING_SECRET not set AND failed to persist a "+
			"generated secret. Foundry manifest tokens WILL be invalidated "+
			"on next restart. Set MEDIA_SIGNING_SECRET in env, or ensure "+
			"the configured path is writable.",
			slog.Any("error", secretErr),
			slog.String("path", a.Config.Upload.SigningSecretFile))
	}
	if secretErr != nil && secretSource != media.SecretGeneratedInMemory {
		// Non-fatal soft error (e.g., read error on a stale file
		// that we recovered from by regenerating). Surface it so
		// the operator can investigate.
		slog.Warn("media signing secret load reported a recoverable error",
			slog.Any("error", secretErr),
			slog.String("source", string(secretSource)))
	}
	var urlSigner *media.URLSigner
	if signingSecret != "" {
		urlSigner = media.NewURLSigner(signingSecret)
		mediaHandler.SetURLSigner(urlSigner)
	}

	// Wire campaign membership checker for private media access control.
	mediaHandler.SetMemberChecker(&mediaMemberCheckerAdapter{svc: campaignService})

	media.RegisterRoutes(e, mediaHandler, authService, resolveMaxUpload, a.Config.Upload.ServeRateLimit)
	// Campaign media routes registered after addon service init (needs media-gallery addon gating).

	// Admin plugin: site-wide management (users, campaigns, SMTP settings, storage).
	adminHandler := admin.NewHandler(authRepo, campaignService, smtpService)
	// Pass a function so the storage admin page reads the LIVE limit
	// (matching what the body-limit middleware enforces) rather than the
	// frozen-at-startup env value.
	adminHandler.SetMediaDeps(mediaRepo, mediaService, func() int64 {
		if g, err := settingsService.GetStorageLimits(context.Background()); err == nil && g != nil && g.MaxUploadSize > 0 {
			return g.MaxUploadSize
		}
		return a.Config.Upload.MaxSize
	})
	adminHandler.SetBaseURL(a.Config.BaseURL)
	adminGroup := admin.RegisterRoutes(e, adminHandler, authService, smtpHandler)

	// Admin Backup plugin: lists backup artifacts and exposes a "Run
	// backup" button that shells out to scripts/backup.sh under the
	// configured timeout.
	backupSvc := backup.NewService(backup.Config{
		ScriptPath: a.Config.BackupScriptPath,
		BackupDir:  a.Config.BackupDir,
	})
	backupHandler := backup.NewHandler(backupSvc)
	backup.RegisterRoutes(adminGroup, backupHandler)

	// Admin Restore plugin: lists backup manifests in BACKUP_DIR and
	// shells out to scripts/restore.sh under a typed-RESTORE
	// confirmation. Reverses ADR-035's "sysadmin-only" stance — see
	// ADR-036 for the policy reasoning.
	//
	// Always registered: BackupDir now defaults to /app/data/backups in
	// config.Load, so the panic in restore.NewService(BackupDir=="") is
	// unreachable from production wiring. Removing the conditional fixes
	// the "Restore link is in the sidebar but 404s" bug that operators
	// who hadn't set BACKUP_DIR explicitly hit on first deploy.
	restoreSvc := restore.NewService(restore.Config{
		ScriptPath: a.Config.RestoreScriptPath,
		BackupDir:  a.Config.BackupDir,
	})
	restoreHandler := restore.NewHandler(restoreSvc)
	restore.RegisterRoutes(adminGroup, restoreHandler)

	// Settings plugin route registration. The service + repo were
	// constructed earlier (above the media routes) so the body-limit
	// middleware can read live settings — see the resolveMaxUpload
	// closure. Per-user/campaign quotas wired via SetStorageLimiter
	// up there too. Here we just register the admin HTTP routes.
	settingsHandler := settings.NewHandler(settingsService)
	settings.RegisterRoutes(adminGroup, settingsHandler)

	// Design Lab: admin-only page hosting the dynamic-surface demo (a live
	// character sheet built by the Chronicle.surface frame).
	designLabHandler := designlab.NewHandler()
	designlab.RegisterRoutes(adminGroup, designLabHandler)

	// Wire settings service into admin handler for the combined storage page.
	adminHandler.SetSettingsDeps(settingsService)

	// Addons plugin: extension framework with per-campaign enable/disable toggles.
	addonRepo := addons.NewAddonRepository(a.DB)
	addonService := addons.NewAddonService(addonRepo)
	// Auto-register discovered game systems as addons so new systems
	// (from internal/systems/, package manager, or GitHub) appear in the
	// addon UI without hardcoded definitions.
	sysAddonInfos := systems.AddonInfos()
	slog.Info("registering system addons at startup", slog.Int("count", len(sysAddonInfos)))
	for _, info := range sysAddonInfos {
		slog.Info("registering system addon", slog.String("slug", info.Slug), slog.String("name", info.Name))
		addons.RegisterSystemAddon(info.Slug, info.Name, info.Description, info.Version, info.Icon, info.Author)
	}
	// Seed all built-in addons (plugins, widgets, integrations + auto-registered
	// systems) into the database on startup.
	if err := addonService.SeedInstalledAddons(context.Background()); err != nil {
		slog.Error("failed to seed built-in addons", slog.String("error", err.Error()))
	}
	addonService.SetPresetApplier(newPresetApplier(entityService))
	// Register per-extension settings/onboarding providers (the SetupProvider
	// framework). The player-character provider drives the claiming addon's
	// settings page: detect existing PCs / sub-categories / the duplicate-category
	// artifact, choose the system vs a custom name, and merge the duplicate on
	// demand (replacing the old silent boot migration).
	addonService.RegisterSetupProvider(newPCSetupProvider(entityService))
	// One-time, idempotent startup backfill: premake the claimable "Player
	// Character" type for campaigns that enabled the claiming addon before its
	// enable-effect shipped. Safe to run every boot (no-op where present);
	// best-effort so a failure can't block startup.
	if n, err := backfillPlayerCharacterTypes(context.Background(), addonService, entityService); err != nil {
		slog.Error("player-character-type backfill failed", slog.String("error", err.Error()))
	} else if n > 0 {
		slog.Info("player-character-type backfill complete", slog.Int("campaigns", n))
	}
	addonService.SetSystemFinder(&systemManifestFinderAdapter{})
	addonHandler := addons.NewHandler(addonService)
	addons.RegisterAdminRoutes(adminGroup, addonHandler)
	addons.RegisterCampaignRoutes(e, addonHandler, campaignService, authService)

	// Campaign media browser routes (gated behind media-gallery addon).
	media.RegisterCampaignRoutes(e, mediaHandler, campaignService, authService, addonService)

	// Wire addon count into admin dashboard for the Extensions stat card.
	adminHandler.SetAddonCounter(addonService)
	adminHandler.SetAddonUsageCounter(addonService)

	// Wire addon checker into entity handler for conditional attributes rendering.
	entityHandler.SetAddonChecker(addonService)
	// Wire the same checker into the entity service so CreateEntityType can
	// gate player-character sub-type creation on the Player Character Claiming
	// addon (PC-CLAIM-2).
	entityService.SetAddonChecker(addonService)

	// Content extensions: user-installable content packs (calendar presets,
	// entity type templates, entity packs, tag collections, marker icons, themes).
	extRepo := extensions.NewExtensionRepository(a.DB)
	extService := extensions.NewExtensionService(extRepo, a.Config.ExtensionsPath)
	extService.SetMigrationRunner(extensions.NewMigrationRunner(a.DB))
	extHandler := extensions.NewHandler(extService, a.Config.ExtensionsPath)
	extensions.RegisterAdminRoutes(adminGroup, extHandler)
	extensions.RegisterCampaignRoutes(e, extHandler, campaignService, authService)
	extensions.RegisterAssetRoutes(e, extHandler)

	// C-EXT-HUB Phase 1: let the top-level Extensions hub (campaigns
	// plugin) embed the per-campaign Content Packs list as a card.
	// Inverts the import direction so campaigns stays
	// extensions-agnostic at compile time.
	campaignHandler.SetContentPacksCardRenderer(extHandler)

	// Package manager: external repo management for systems and Foundry module.
	pkgRepo := packages.NewPackageRepository(a.DB)
	pkgGitHub := packages.NewGitHubClient()
	pkgService := packages.NewPackageService(pkgRepo, pkgGitHub, a.Config.Upload.MediaPath, a.Config.BaseURL)
	// Rescan system registry and re-register addons when a system package
	// is installed or updated, so it appears in the campaign Settings >
	// Game System dropdown immediately without requiring a server restart.
	packages.SetOnSystemInstall(pkgService, func(installPath string) {
		systems.ScanPackageDir(filepath.Join(a.Config.Upload.MediaPath, "packages", "systems"))
		// Force-load the exact dir that was just installed. The rescan
		// above applies "highest version wins", which silently ignores a
		// deliberate rollback to an OLDER version; an explicit install is
		// operator intent and must be what the loader serves. Failure is
		// logged only — the post-install verifier persists it as the
		// package's last_error for the admin UI.
		if installPath != "" {
			if err := systems.ForceLoadDir(installPath); err != nil {
				slog.Error("force-load of installed system dir failed",
					slog.String("dir", installPath), slog.Any("error", err))
			}
		}
		// Re-register discovered systems as addons (idempotent — updates
		// existing entries, adds new ones) and upsert to DB so campaign
		// addon associations are preserved.
		for _, info := range systems.AddonInfos() {
			addons.RegisterSystemAddon(info.Slug, info.Name, info.Description, info.Version, info.Icon, info.Author)
		}
		if err := addonService.SeedInstalledAddons(context.Background()); err != nil {
			slog.Error("failed to re-seed addons after system install", slog.Any("error", err))
		}
		// Rebuild + republish the entity-show-renderer registry so a newly
		// installed/updated system's page renderers (e.g. a character sheet)
		// take effect WITHOUT a server restart. Build fully, then publish — no
		// observable half-built state for in-flight requests. (Bug 5a: previously
		// registerManifestRenderers ran only at boot, so a package update's
		// renderer binding required a restart.)
		freshRegistry := entities.NewEntityShowRendererRegistry()
		registerManifestRenderers(freshRegistry)
		entities.SetGlobalEntityShowRendererRegistry(freshRegistry)

		// Converge gm_only field flags now that the freshly installed/updated
		// manifest is in the registry, so an updated system that newly marks a
		// field gm_only (audit M-1) takes effect on existing types without a
		// restart — mirrors the boot-time reconcile.
		reconcileFieldGMFlags(context.Background(), entityService)
		// Same convergence for owner_only field flags (C-FIELDS-OWNER-FILTER).
		reconcileFieldOwnerOnlyFlags(context.Background(), entityService)
	})
	packages.ConfigureSettings(pkgService, settingsRepo)
	// Fail-loud installs: run the FULL loader-grade manifest validation at
	// install time for system packages, so a manifest the boot scan would
	// reject (content caps, slugs, renderer bindings…) fails the install
	// with the real error instead of installing "green" and shadow-failing
	// at load while the old version keeps serving.
	packages.SetManifestValidator(pkgService, func(manifestPath string) error {
		_, err := systems.LoadManifest(manifestPath)
		return err
	})
	// Verified installs: after the rescan, confirm the loader actually
	// serves the just-installed dir+version. On miss, pull the real
	// rejection reason from the load-event log so the persisted
	// last_error tells the admin WHY (e.g. a validation failure on a
	// pre-validator install, or a stale registry).
	// Stale-version cleanup safety: the prune wizard must never delete a
	// dir the loader is live-serving, even an OLD one (stale registry).
	packages.SetLoadedDirsProvider(pkgService, func() map[string]bool {
		out := map[string]bool{}
		for _, sh := range systems.LoadedHealth() {
			if sh.Dir != "" {
				out[sh.Dir] = true
			}
		}
		return out
	})
	packages.SetPostInstallVerifier(pkgService, func(installPath, version string) error {
		for _, sh := range systems.LoadedHealth() {
			if sh.Dir == installPath && sh.Version == version {
				return nil
			}
		}
		events := systems.DiagnosticEvents()
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Dir == installPath && events[i].Kind == systems.EventFailed {
				return fmt.Errorf("loader rejected it: %s", events[i].Error)
			}
		}
		return fmt.Errorf("loader did not register %s (an older version may still be serving)", installPath)
	})

	// Wire installed-package state into the operator diagnostics (dependency
	// inversion: systems can't import packages) so packages.installed-vs-loaded /
	// on-disk-versions can compare the DB's installed version to the live loader.
	systems.SetInstalledPackagesProvider(func() []systems.InstalledPackage {
		pkgs, err := pkgService.ListPackages(context.Background())
		if err != nil {
			return nil
		}
		out := make([]systems.InstalledPackage, 0, len(pkgs))
		for _, p := range pkgs {
			if p.Type == packages.PackageTypeSystem {
				out = append(out, systems.InstalledPackage{Slug: p.Slug, Version: p.InstalledVersion, InstallPath: p.InstallPath})
			}
		}
		return out
	})

	// Wire a read-only entity-data window into the operator diagnostics so
	// entity.fields / entity.field-coverage can inspect stored hero data (the
	// "renders blank — is the data even there?" check). Dependency inversion:
	// systems can't import entities, so the adapter lives in the app layer.
	systems.SetEntityDiagProvider(entityDiagAdapter{entities: entityService})

	// Wire the campaigns list (admin-only ListAll) so campaigns.list can resolve a
	// campaign id by name — the entry point for the entity.* diagnostics.
	systems.SetCampaignListProvider(func(ctx context.Context) ([]systems.CampaignInfo, error) {
		cs, _, err := campaignService.ListAll(ctx, campaigns.ListOptions{Page: 1, PerPage: 500})
		if err != nil {
			return nil, err
		}
		out := make([]systems.CampaignInfo, 0, len(cs))
		for _, c := range cs {
			out = append(out, systems.CampaignInfo{ID: c.ID, Name: c.Name, Slug: c.Slug})
		}
		return out, nil
	})

	// Wire the inbound-sync ring buffer (what external clients SENT) into the
	// operator diagnostics so sync.inbound / sync.recent can show the Foundry→
	// Chronicle payloads — the missing probe point between "sent" and "stored".
	systems.SetSyncInboundProvider(func(entityID string, limit int) []systems.InboundSyncRecord {
		recs := syncapi.RecentInbound(entityID, limit)
		out := make([]systems.InboundSyncRecord, 0, len(recs))
		for _, r := range recs {
			out = append(out, systems.InboundSyncRecord{EntityID: r.EntityID, At: r.At, Source: r.Source, Fields: r.Fields})
		}
		return out
	})

	pkgHandler := packages.NewHandler(pkgService)
	pkgOwnerHandler := packages.NewOwnerHandler(pkgService)
	// Public package file serving — always available so Foundry VTT can
	// fetch module.json even when the admin UI is degraded.
	pkgServeHandler := packages.NewServeHandler(pkgService, a.Config.BaseURL)
	packages.SetOnServeInvalidate(pkgService, pkgServeHandler.InvalidateCache)
	packages.RegisterPublicRoutes(e, pkgServeHandler, middleware.RateLimit(300, time.Minute))

	if a.PluginHealth.IsHealthy("packages") {
		packages.RegisterRoutes(adminGroup, pkgHandler)

		// Owner-facing submission routes (authenticated, not admin-only).
		ownerGroup := e.Group("", auth.RequireAuth(authService))
		packages.RegisterOwnerRoutes(ownerGroup, pkgOwnerHandler)

		// Load systems from package manager install paths so externally
		// managed system packs override the bundled fallbacks.
		a.loadSystemsFromPackages(pkgService)

		// Store package service reference.
		a.pkgService = pkgService

		// Start background auto-update worker.
		go pkgService.StartAutoUpdateWorker(context.Background())
	} else {
		slog.Warn("packages plugin degraded — routes not registered")
	}

	// Security admin: event logging, session management, user account actions.
	securityRepo := admin.NewSecurityEventRepository(a.DB)
	securityService := admin.NewSecurityService(securityRepo, authRepo, authService)
	adminHandler.SetSecurityService(securityService)

	// Data hygiene scanner: orphan detection and cleanup for media, API keys, stale files.
	hygieneScanner := admin.NewHygieneService(a.DB, mediaRepo, mediaService, a.Config.Upload.MediaPath, securityRepo)
	adminHandler.SetHygieneScanner(hygieneScanner)

	// Database explorer: schema visualization and migration management.
	dbExplorer := admin.NewDatabaseExplorer(a.DB, a.PluginHealth, a.PluginSchemas)
	adminHandler.SetDatabaseExplorer(dbExplorer)
	// Database page Health + Backups tabs: run the same checks boot runs, and
	// surface the existing backup/restore artifacts — adapters so admin imports
	// neither the boot config nor the backup/restore plugins.
	adminHandler.SetHealthChecker(&adminHealthChecker{db: a.DB, cfg: a.Config})
	adminHandler.SetBackupLister(&adminBackupLister{backups: backupSvc, restores: restoreSvc, backupDir: a.Config.BackupDir})

	// Wire security event logging into the auth handler so logins, logouts,
	// failed attempts, and password resets are recorded automatically.
	authHandler.SetSecurityLogger(securityService)

	// Wire security event logging into the media handler so uploads, deletes,
	// and quota failures are recorded in the admin security dashboard.
	mediaHandler.SetSecurityLogger(securityService)

	// foundry_vtt sub-plugin (C-FMC-5b + C-FMC-5c): the Foundry-VTT-
	// specific extension to the generic packages plugin. Owns every
	// Foundry-specific behavior: per-campaign signed manifest URLs,
	// per-campaign pinning, chronicle-package.json descriptor reading
	// + the post-install module.json version rewrite, admin "campaigns
	// using v0.1.5" expandable cards on /admin/packages.
	//
	// C-FMC-5c is the cleanup PR that deleted the parallel
	// foundry_modules plugin, renamed the token table to
	// foundry_vtt_campaign_tokens (via this plugin's migration 001),
	// added admin endpoints (force-pin, notify, mass variants) here,
	// and removed Foundry coupling from the packages plugin. After
	// this PR ships, the packages plugin has zero Foundry-specific
	// code — Chronicle is fully Foundry-agnostic at the packages layer.

	// NW-2.2 Chunk A pilot: register foundry_vtt in the App's metadata
	// registry. Per cordinator/decisions/2026-05-23-plugin-registration.md.
	// Slug is the canonical external identifier (matches the WS protocol
	// + the URL prefix); HealthCheck wraps the existing PluginHealth
	// lookup via the plugin's exported PluginHealthKey const (which uses
	// the Go-package-name underscore form for historical reasons —
	// distinct from PluginSlug).
	a.registerPlugin(PluginRegistration{
		Slug: foundry_vtt.PluginSlug,
		HealthCheck: func() error {
			if a.PluginHealth != nil && !a.PluginHealth.IsHealthy(foundry_vtt.PluginHealthKey) {
				return errors.New("foundry_vtt schema unhealthy")
			}
			return nil
		},
	})

	fvttRepo := foundry_vtt.NewRepository(a.DB)
	fvttTokenSigner := foundry_vtt.NewTokenSigner(signingSecret)
	fvttCampaignAdapter := &foundryCampaignSettingsAdapter{svc: campaignService}
	fvttOwnerLookup := &foundryCampaignOwnerLookupAdapter{
		campaignSvc: campaignService,
		authSvc:     authService,
	}
	fvttService := foundry_vtt.NewService(
		fvttRepo, fvttTokenSigner, fvttCampaignAdapter, pkgService,
		securityService, smtpService, fvttOwnerLookup,
		settingsRepo, // C-FMC-8: powers the auto-pin install banner
		a.Config.BaseURL,
	)
	fvttHandler := foundry_vtt.NewHandler(fvttService)
	// (Banner adapter wire removed in D2-cleanup; the campaign show
	// page lazy-loads /foundry-vtt/show-banner-fragment instead.)
	if a.PluginHealth.IsHealthy(foundry_vtt.PluginHealthKey) && a.PluginHealth.IsHealthy("packages") {
		// Register the PostInstallHook with the packages plugin. The
		// hook fires after every foundry-module typed install and
		// rewrites module.json's version field to match the installed
		// version (the fix for the operator's version-stale bug end-
		// to-end).
		packages.RegisterPostInstallHook(pkgService, foundry_vtt.NewPostInstallHook())
		// C-FMC-6: second hook for install-time auto-pinning. Runs
		// alongside the C-FMC-5b version-rewrite hook. Pins auto-
		// tracking campaigns to the previous version on every
		// foundry-module install so the admin sees the version
		// spread instead of silently bumping everyone.
		packages.RegisterPostInstallHook(pkgService, foundry_vtt.NewAutoPinHook(fvttService))

		// C-FMC-6: one-time auto-pin migration for pre-feature
		// campaigns. Pins all auto-tracking campaigns to the
		// foundry-module's currently-installed version so future
		// installs trigger the AutoPinHook flow (which preserves
		// state) instead of silently bumping. Idempotent via a
		// settings key — re-runs are no-ops after first completion.
		// Runs synchronously here so it completes before any
		// campaign loads its settings page. Errors abort startup
		// loudly (matches the C-FMC-5c PreMigrationCheck pattern).
		if err := foundry_vtt.AutoPinMigrate(context.Background(), fvttService, settingsRepo); err != nil {
			slog.Error("foundry_vtt autopin migration failed", slog.Any("error", err))
			// Not fatal — migration is best-effort. Log + continue.
			// A future boot can retry once the operator addresses
			// the underlying issue (DB connection, schema state).
		}

		// Admin routes: "campaigns using v0.1.5" fragment + force-pin
		// + notify + mass variants. Force-pin routes additionally
		// require admin password re-auth.
		foundry_vtt.RegisterAdminRoutes(adminGroup, fvttHandler, auth.RequireReauth(authService))

		// Owner-facing routes (per-campaign pin, token rotate, install
		// URL, settings tab fragment).
		fvttCampaignAuthed := e.Group("/campaigns/:id",
			auth.RequireAuth(authService),
			campaigns.RequireCampaignAccess(campaignService),
		)
		foundry_vtt.RegisterOwnerRoutes(fvttCampaignAuthed, fvttHandler,
			campaigns.RequireRole(campaigns.RoleOwner))

		// Public manifest + download. Same rate limit as the packages
		// public endpoints — manifest hits are frequent (every Foundry
		// update check), the limit needs headroom for moderately-
		// sized deployments.
		foundry_vtt.RegisterPublicRoutes(e, fvttHandler, middleware.RateLimit(300, time.Minute))
	} else {
		slog.Warn("foundry_vtt plugin degraded — routes not registered")
	}

	// Sync API plugin: external tool integration with API key auth,
	// request logging, security monitoring, and admin dashboard.
	syncRepo := syncapi.NewSyncAPIRepository(a.DB)
	syncService := syncapi.NewSyncAPIService(syncRepo)
	syncHandler := syncapi.NewHandler(syncService)
	// Inject sync mapping service early so the owner dashboard can show sync status.
	syncMappingRepoEarly := syncapi.NewSyncMappingRepository(a.DB)
	syncMappingSvcEarly := syncapi.NewSyncMappingService(syncMappingRepoEarly)
	syncHandler.SetSyncMappingService(syncMappingSvcEarly)

	// Wire the sync-mapping reader into the operator diagnostics so
	// entity.sync-mappings can answer "is this entity linked to a Foundry actor?"
	// — the root-cause check for a permanently-blank hero.
	systems.SetSyncMappingProvider(func(ctx context.Context, campaignID, entityID string) ([]systems.SyncMappingInfo, error) {
		ms, _, err := syncMappingSvcEarly.ListMappings(ctx, campaignID, 1000, 0)
		if err != nil {
			return nil, err
		}
		out := make([]systems.SyncMappingInfo, 0)
		for _, m := range ms {
			if m.ChronicleType == "entity" && m.ChronicleID == entityID {
				out = append(out, systems.SyncMappingInfo{
					ExternalSystem: m.ExternalSystem,
					ExternalID:     m.ExternalID,
					ChronicleType:  m.ChronicleType,
					ChronicleID:    m.ChronicleID,
					LastSync:       m.LastSyncedAt.UTC().Format("2006-01-02T15:04:05Z"),
				})
			}
		}
		return out, nil
	})
	syncHandler.SetCORSOriginLister(settingsService)
	syncHandler.SetBaseURL(a.Config.BaseURL)
	if a.PluginHealth.IsHealthy("syncapi") {
		syncapi.RegisterAdminRoutes(adminGroup, syncHandler)
		syncapi.RegisterCampaignRoutes(e, syncHandler, campaignService, authService)
	} else {
		slog.Warn("syncapi plugin degraded — routes not registered")
	}

	// Calendar plugin: custom fantasy calendar with months, moons, events.
	// Created early so the sync API can reference calendarService.
	// Service is always created (other plugins reference it), but routes
	// are only registered if the calendar schema is healthy.
	calendarRepo := calendar.NewCalendarRepository(a.DB)
	calendarService := calendar.NewCalendarService(calendarRepo)
	calendarHandler := calendar.NewHandler(calendarService)
	calendarHandler.SetAddonService(addonService)
	// "Create entity from event" drawer action (C-CAL-EDITOR-EXPANSION PR1) —
	// the cross-plugin write seam over the entities service (rule 8).
	calendarHandler.SetEntityCreator(&calendarEntityCreatorAdapter{svc: entityService})

	// C-CAL-RSVP-P1: first-class event RSVPs. Its OWN repo/service/handler,
	// disjoint from CalendarService/CalendarRepository. The mailer wires now
	// (smtpService already exists ~:1664); the notifier + self-write availability
	// adapter are wired AFTER sessionsService is constructed below, via the SetX
	// setter pattern (like SetTimelineLister :~2740). Nil-safe until then.
	rsvpRepo := calendar.NewRSVPRepository(a.DB)
	rsvpService := calendar.NewRSVPService(rsvpRepo, calendarService, campaignService, a.Config.BaseURL)
	rsvpService.SetMailSender(smtpService)
	rsvpHandler := calendar.NewRSVPHandler(rsvpService, calendarService)

	// NW-2.2 Chunk F: register calendar in the App's metadata registry +
	// expose its embedded static assets for serving at /static/plugins/calendar/.
	// echo.MustSubFS strips the leading "static" dir from the embed so
	// /static/plugins/calendar/js/calendar_widget.js maps cleanly. Per
	// cordinator/decisions/2026-05-25-plugin-static-assets.md.
	a.registerPlugin(PluginRegistration{
		Slug: calendar.PluginSlug,
		HealthCheck: func() error {
			if a.PluginHealth != nil && !a.PluginHealth.IsHealthy(calendar.PluginHealthKey) {
				return errors.New("calendar schema unhealthy")
			}
			return nil
		},
		StaticFS: echo.MustSubFS(calendar.StaticAssetsFS, "static"),
	})
	// Finding 4 (M-B2.1): plugin body scripts contributed by plugins at registration
	// time so base.templ no longer hardcodes plugin asset paths. The calendar widget
	// is the first contributor; future plugins append to this slice. Injected into
	// every page's Templ context by the LayoutInjector below.
	// Follow-up: a WidgetScript field on PluginRegistration would make this implicit
	// (post-launch C-PLUGIN-CONTRACTS-REFACTOR).
	pluginBodyScripts := []string{
		"/static/plugins/" + calendar.PluginSlug + "/js/calendar_widget.js",
	}

	if a.PluginHealth.IsHealthy("calendar") {
		calendar.RegisterRoutes(e, calendarHandler, rsvpHandler, campaignService, authService, addonService)

		// C-CALENDAR-ENDPOINTS: public Foundry-facing calendar API
		// gated by the same per-campaign signed token foundry_vtt
		// uses for the manifest endpoint. fvttService satisfies
		// the calendar.TokenVerifier interface so the calendar
		// plugin has no compile-time edge into foundry_vtt.
		// Skipped if foundry_vtt is degraded — the token verifier
		// wouldn't exist.
		if a.PluginHealth.IsHealthy(foundry_vtt.PluginHealthKey) {
			calendarAPIHandler := calendar.NewAPIHandler(calendarService, fvttService)
			calendar.RegisterPublicAPIRoutes(e, calendarAPIHandler, middleware.RateLimit(300, time.Minute))
		}
	} else {
		slog.Warn("calendar plugin degraded — routes not registered")
	}

	// Bestiary plugin: community creature sharing with ratings, favorites, import.
	bestiaryRepo := bestiary.NewBestiaryRepository(a.DB)
	bestiarySvc := bestiary.NewBestiaryService(bestiaryRepo)
	bestiarySvc.SetUserFetcher(&bestiaryUserFetcherAdapter{authSvc: authService})
	bestiarySvc.SetEntityCreator(&bestiaryEntityCreatorAdapter{svc: entityService})
	bestiarySvc.SetCampaignRoleChecker(&bestiaryCampaignRoleAdapter{svc: campaignService})
	bestiarySvc.SetCampaignSystemFetcher(&bestiaryCampaignSystemAdapter{svc: campaignService})
	bestiaryHandler := bestiary.NewHandler(bestiarySvc)
	if a.PluginHealth.IsHealthy("bestiary") {
		bestiary.RegisterRoutes(e, bestiaryHandler, authService)
		bestiary.RegisterAdminRoutes(e, bestiaryHandler, authService)
	} else {
		slog.Warn("bestiary plugin degraded — routes not registered")
	}

	// Maps plugin: interactive maps with Leaflet.js, pin markers, entity linking.
	// Services created unconditionally (sync API references drawingService).
	mapsRepo := maps.NewMapRepository(a.DB)
	mapsService := maps.NewMapService(mapsRepo)
	mapsHandler := maps.NewHandler(mapsService)
	drawingRepo := maps.NewDrawingRepository(a.DB)
	drawingService := maps.NewDrawingService(drawingRepo)
	// Wire the map-existence + same-campaign check used by AssignMap on
	// entities. This sits BELOW where entityService is constructed (1251)
	// and is set as a post-construction dependency.
	entityService.SetMapVerifier(&entityMapVerifierAdapter{svc: mapsService})
	if a.PluginHealth.IsHealthy("maps") {
		maps.RegisterRoutes(e, mapsHandler, campaignService, authService, addonService)
		drawingHandler := maps.NewDrawingHandler(mapsService, drawingService)
		maps.RegisterDrawingRoutes(e, drawingHandler, campaignService, authService, addonService)
	} else {
		slog.Warn("maps plugin degraded — routes not registered")
	}

	// Sessions plugin: game session scheduling, linked entities, RSVP tracking.
	// Entity campaign checker prevents cross-campaign entity linking (IDOR).
	sessionsRepo := sessions.NewSessionRepository(a.DB)
	sessionsService := sessions.NewSessionService(sessionsRepo, &entityCampaignCheckerAdapter{svc: entityService})
	sessionsHandler := sessions.NewHandler(sessionsService)
	sessionsHandler.SetMemberLister(campaignService)
	sessionsHandler.SetMailSender(smtpService, a.Config.BaseURL)
	if a.PluginHealth.IsHealthy("sessions") {
		sessions.RegisterRoutes(e, sessionsHandler, campaignService, authService, addonService)
	} else {
		slog.Warn("sessions plugin degraded — routes not registered")
	}

	// C-CAL-RSVP-P1 cross-plugin wiring (rule 8): back the calendar RSVP
	// notifier + self-write availability writer with the sessions plugin now that
	// sessionsService exists. Post-construction setters mutate the same
	// rsvpService instance the calendar routes already hold (registered above),
	// exactly like SetTimelineLister wires calendarHandler after the fact.
	rsvpService.SetNotifier(&calendarRSVPNotifierAdapter{svc: sessionsService})
	rsvpService.SetAvailabilityExceptionWriter(&calendarAvailabilityWriterAdapter{svc: sessionsService})

	// Timeline plugin: interactive visual timelines with zoom levels and entity grouping.
	timelineRepo := timeline.NewTimelineRepository(a.DB)
	timelineSvc := timeline.NewTimelineService(timelineRepo, &calendarListerAdapter{svc: calendarService}, &calendarEventListerAdapter{svc: calendarService}, &calendarEraListerAdapter{svc: calendarService})
	timelineHandler := timeline.NewHandler(timelineSvc)
	timelineHandler.SetMemberLister(campaignService)
	if a.PluginHealth.IsHealthy("timeline") {
		timeline.RegisterRoutes(e, timelineHandler, campaignService, authService, addonService)
	} else {
		slog.Warn("timeline plugin degraded — routes not registered")
	}

	// Relations widget: bi-directional entity linking. Created before REST API
	// so it can be injected into the API handler for shop inventory support.
	relRepo := relations.NewRelationRepository(a.DB)
	relService := relations.NewRelationService(relRepo)
	relService.SetMentionLinkProvider(&mentionLinkAdapter{svc: entityService})
	// Entity-privacy gate (C-PUBLIC-VIEW-FIX-R2): hide private-entity nodes from
	// the graph, and enforce entity privacy + campaign binding on the list.
	relEntityGate := &entityAccessAdapter{svc: entityService}
	relService.SetEntityViewFilter(relEntityGate)
	relHandler := relations.NewHandler(relService)
	relHandler.SetEntityTypeLister(&entityTypeListerForGraphAdapter{svc: entityService})
	relHandler.SetEntityGate(relEntityGate)
	relations.RegisterRoutes(e, relHandler, campaignService, authService)

	// Posts widget: entity sub-notes with rich text, visibility, and reorder.
	postRepo := posts.NewPostRepository(a.DB)
	postService := posts.NewPostService(postRepo)
	postHandler := posts.NewHandler(postService)
	// Entity-privacy gate (C-PUBLIC-VIEW-FIX-R2): the public posts list must
	// respect entity visibility + campaign binding, like the entity page.
	postHandler.SetEntityGate(&entityAccessAdapter{svc: entityService})
	posts.RegisterRoutes(e, postHandler, campaignService, authService)

	// Player Notes (entity_notes) widget: per-user, per-entity notes
	// with a 5-tier audience ACL (private / dm_only / dm_scribe /
	// everyone / custom). The notifier is wired below after wsEventBus
	// is constructed; until then, mutations don't broadcast (which is
	// fine — this code path executes at startup, before any client can
	// reach the API).
	//
	// The holder is heap-allocated (pointer) so the method value bound
	// into the service captures the same struct that we mutate later
	// when wsEventBus exists.
	entityNotesRepo := entity_notes.NewRepository(a.DB)
	entityNotesNotifier := &entityNotesNotifierHolder{}
	entityNotesService := entity_notes.NewService(entityNotesRepo, entityNotesNotifier.Notify)
	entityNotesHandler := entity_notes.NewHandler(entityNotesService)
	entity_notes.RegisterRoutes(e, entityNotesHandler, campaignService, authService)

	// Tags widget: campaign-scoped entity tagging (CRUD + entity associations).
	// Created before sync API so the tag service is available for the REST API handler.
	tagRepo := tags.NewTagRepository(a.DB)
	tagService := tags.NewTagService(tagRepo)
	tagHandler := tags.NewHandler(tagService)
	// Entity-privacy gate (C-PUBLIC-VIEW-FIX-R2): the public per-entity tag read
	// must respect entity visibility + campaign binding, like the entity page.
	tagHandler.SetEntityGate(&entityAccessAdapter{svc: entityService})
	// Tag visibility grants (C-PERM-W1-TAG-GRANTS): Owner-gated CRUD plus the
	// effective-visibility glance source. The grant service validates grant
	// subjects against the campaign (member/group lookups) and resolves their
	// human labels for the badge tooltip.
	tagGrantRepo := tags.NewTagPermissionRepository(a.DB)
	tagGrantService := tags.NewTagGrantService(tagGrantRepo, tagRepo, campaignService, groupService)
	tagHandler.SetGrantService(tagGrantService)
	tags.RegisterRoutes(e, tagHandler, campaignService, authService)
	// Shared tag adapter: feeds the entities glance (SetTagFetcher below) and the
	// syncapi permissions endpoint (SetTagGrantLister) the same tag-grant view.
	tagFetcherAdapter := &entityTagFetcherAdapter{svc: tagService, grantSvc: tagGrantService}

	// REST API v1: versioned endpoints for external clients (Foundry VTT, etc.).
	// Authenticates via API keys, not browser sessions.
	syncAPIHandler := syncapi.NewAPIHandler(syncService, entityService, campaignService, relService)
	syncAPIHandler.SetAddonLister(&addonListerAPIAdapter{svc: addonService})
	// Expose tag-derived grants on the permissions endpoint for Foundry ownership
	// sync (C-PERM-W1-TAG-GRANTS), reusing the entities glance adapter.
	syncAPIHandler.SetTagGrantLister(tagFetcherAdapter)
	syncAPIHandler.SetSystemEnabler(addonService)
	calendarAPIHandler := syncapi.NewCalendarAPIHandler(syncService, calendarService)
	mediaAPIHandler := syncapi.NewMediaAPIHandler(syncService, mediaService)
	if urlSigner != nil {
		mediaAPIHandler.SetURLSigner(urlSigner)
	}

	// Sync mapping handler for Foundry VTT bidirectional sync.
	// Reuses the sync mapping service created earlier for the owner dashboard.
	syncMappingHandler := syncapi.NewSyncHandler(syncMappingSvcEarly)
	mapAPIHandler := syncapi.NewMapAPIHandler(syncService, mapsService, drawingService, campaignService)

	// Note API handler for sync API — uses the same note repo/service as the web handler.
	// Created here (before RegisterAPIRoutes) so the service is available; the web
	// handler wiring below reuses the same noteSvc instance.
	noteRepo := notes.NewNoteRepository(a.DB)
	attRepo := notes.NewAttachmentRepository(a.DB)
	noteSvc := notes.NewNoteServiceWithAttachments(noteRepo, attRepo)
	noteAPIHandler := syncapi.NewNoteAPIHandler(syncService, noteSvc)

	// Tag API handler for sync API — exposes tag CRUD and bulk tag operations.
	tagAPIHandler := syncapi.NewTagAPIHandler(syncService, tagService, entityService, campaignService)

	if a.PluginHealth.IsHealthy("syncapi") {
		syncapi.RegisterAPIRoutes(e, syncAPIHandler, calendarAPIHandler, mediaAPIHandler, mapAPIHandler, noteAPIHandler, tagAPIHandler, syncMappingHandler, syncService, addonService, authService, campaignService)
	}

	// NPC plugin: gallery/hub view for revealed character entities.
	npcRepo := npcs.NewNPCRepository(a.DB)
	npcSvc := npcs.NewNPCService(npcRepo, &npcEntityTypeFinderAdapter{svc: entityService})
	npcHandler := npcs.NewHandler(npcSvc)
	npcHandler.SetVisibilityToggler(&npcVisibilityTogglerAdapter{svc: entityService})
	npcs.RegisterRoutes(e, npcHandler, campaignService, authService, addonService)
	// The npcs plugin contributes the NPCs/Monsters section of the unified
	// Characters page (the standalone NPC gallery folded in). npcHandler
	// structurally satisfies entities.NPCSectionProvider.
	entityHandler.SetNPCSectionProvider(npcHandler)

	// Armory plugin: gallery/hub view for item-category entities.
	armoryRepo := armory.NewArmoryRepository(a.DB)
	armorySvc := armory.NewArmoryService(armoryRepo, &armoryItemTypeFinderAdapter{svc: entityService})
	armoryHandler := armory.NewHandler(armorySvc)

	// Instance service: named inventory collections per campaign.
	instRepo := armory.NewInstanceRepository(a.DB)
	instSvc := armory.NewInstanceService(instRepo)
	instHandler := armory.NewInstanceHandler(instSvc)
	armoryHandler.SetInstanceService(instSvc)

	// Transaction service: purchase flow, stock management, transaction logging.
	txRepo := armory.NewTransactionRepository(a.DB)
	txSvc := armory.NewTransactionService(txRepo)
	txSvc.SetRelationMetadataUpdater(&armoryRelationMetadataAdapter{svc: relService})
	txSvc.SetRelationFinder(&armoryRelationFinderAdapter{svc: relService})
	txSvc.SetBuyerAccessChecker(&armoryBuyerAccessAdapter{svc: entityService})
	txHandler := armory.NewTransactionHandler(txSvc)
	armory.RegisterRoutes(e, armoryHandler, txHandler, instHandler, campaignService, authService, addonService)

	// Notes widget: personal floating note-taking panel (Google Keep-style).
	// noteSvc was created above (before REST API v1 registration).
	noteHandler := notes.NewHandler(noteSvc)
	noteHandler.SetAttachmentService(noteSvc)
	noteHandler.SetMediaUploader(&mediaUploadAdapter{svc: mediaService})
	noteHandler.SetMemberLister(campaignService)
	notes.RegisterRoutes(e, noteHandler, campaignService, authService)

	// Relations widget routes already registered above (before REST API v1).

	// Audit plugin: campaign activity logging and history.
	auditRepo := audit.NewAuditRepository(a.DB)
	auditService := audit.NewAuditService(auditRepo)
	auditHandler := audit.NewHandler(auditService)
	// Guard the entity-history endpoint with campaign ownership + per-entity
	// visibility, resolved via the entities service (SEC-IDOR-2).
	auditHandler.SetEntityViewGuard(&auditEntityViewGuardAdapter{svc: entityService})
	audit.RegisterRoutes(e, auditHandler, campaignService, authService)

	// Wire audit logging into mutation handlers so CRUD actions are recorded.
	entityHandler.SetAuditService(auditService)
	calendarHandler.SetAuditService(auditService)
	// Wave 1.6.5: wire campaign tier vocabulary into the V2 calendar
	// shell so EventCard + MultiDayRibbon render campaign-aware tier
	// labels + colors. CampaignService satisfies the narrow
	// TierDefinitionsLister interface via its existing
	// GetEventTierDefinitions method.
	calendarHandler.SetTierDefinitionsLister(campaignService)
	// C-EXT-HUB Phase 2: register the calendar inline dashboard with
	// the Extensions hub. Mirrors ai_workspace.SettingsTabFactory at
	// the campaignHandler.RegisterSettingsTab call below. Per-request
	// data load lives inside the factory closure (see
	// internal/plugins/calendar/extension_dashboard.go).
	campaignHandler.RegisterExtensionDashboard(calendarHandler.ExtensionDashboardFactory())
	// Calendars dashboard (E1 W1): inject the cross-plugin timeline read so the
	// associations panel can list timelines bound to a calendar.
	calendarHandler.SetTimelineLister(&timelineForCalendarAdapter{svc: timelineSvc})
	// And the enable-state checker the hub fragment route consults
	// to render the disabled-extension placeholder. addonService
	// already exposes IsEnabledForCampaign with the canonical narrow
	// interface shape entities + syncapi use.
	campaignHandler.SetExtensionEnableChecker(addonService)
	timelineHandler.SetAuditService(auditService)
	entityHandler.SetTagFetcher(tagFetcherAdapter)
	entityHandler.SetTimelineSearcher(timelineSvc)
	entityHandler.SetMapSearcher(mapsService)
	entityHandler.SetCalendarSearcher(calendarService)
	entityHandler.SetSessionSearcher(sessionsService)
	entityHandler.SetSystemSearcher(systems.NewSystemSearchAdapter(addonService))
	entityHandler.SetMemberLister(campaignService)
	entityHandler.SetGroupLister(groupService)
	entityHandler.SetCache(a.Redis)

	// --- Entity Block Registry ---
	// Create the block registry and let each plugin register its block types.
	// This drives validation, rendering, and the template editor palette.
	blockRegistry := entities.NewBlockRegistry()
	entities.RegisterCoreBlocks(blockRegistry)

	// Widget-binding framework (C-WIDGET-BINDING-P1-SPINE): the dynamic
	// host↔widget-type↔instance registry + service. Widget types register
	// declaratively; the service resolves a host's instance via the precedence
	// chain (own binding → entity-type template → default = today's behavior).
	// P1 registers calendar; maps/timeline/worldstate fold in later (P2/P3).
	widgetRegistry := widgetbindings.NewRegistry()
	widgetRegistry.Register(calendar.NewCalendarWidgetType(calendarService))
	// P2: worldstate (instance = a calendar id, a view over the clock) +
	// timeline (instance = a timeline record) register as widget types.
	widgetRegistry.Register(calendar.NewWorldStateWidgetType(calendarService))
	widgetRegistry.Register(timeline.NewTimelineWidgetType(timelineSvc))
	// P3a: maps registers (instance = a map id; no campaign default — the
	// legacy entity.map_id fallback lives in the map_editor closure).
	widgetRegistry.Register(maps.NewMapWidgetType(mapsService))
	widgetBindingSvc := widgetbindings.NewService(widgetbindings.NewRepository(a.DB), widgetRegistry)
	// P2: wire the delete hooks P1 left unconnected. The calendar/timeline
	// services call OnInstanceDeleted on delete so a removed instance's
	// bindings are swept promptly (render-time guard + Sweep are the backstop).
	// Reached via a type assertion so the CalendarService/TimelineService
	// interfaces stay unchanged.
	if c, ok := calendarService.(interface {
		SetBindingCleaner(calendar.BindingCleaner)
	}); ok {
		c.SetBindingCleaner(widgetBindingSvc)
	}
	if t, ok := timelineSvc.(interface {
		SetBindingCleaner(timeline.BindingCleaner)
	}); ok {
		t.SetBindingCleaner(widgetBindingSvc)
	}
	if m, ok := mapsService.(interface {
		SetBindingCleaner(maps.BindingCleaner)
	}); ok {
		m.SetBindingCleaner(widgetBindingSvc)
	}
	// P4a: the create-or-pick binding UI (picker + bind/create/unbind, Scribe+).
	widgetbindings.RegisterRoutes(e, widgetbindings.NewHandler(widgetBindingSvc, widgetRegistry), campaignService, authService)

	// renderBoundBlock is the single seam every widget-bound entity block goes
	// through on FIRST render (C-WIDGET-BINDING-P4b). It resolves the host's
	// instance via the framework and delegates to the widget type's RenderBlock —
	// the SAME path the binding handler uses for a post-mutation swap, so the
	// initial render and the swap are byte-identical (same BlockHost wrapper +
	// stable id). legacyID is the maps `entity.map_id` fallback: used as the
	// instance only when nothing resolves, preserving pre-binding behavior
	// (binding wins; unbound entity with a legacy map_id renders that map).
	renderBoundBlock := func(widgetType string, rc entities.BlockRenderContext, legacyID string) templ.Component {
		wt, ok := widgetRegistry.Get(widgetType)
		if !ok {
			return templ.NopComponent
		}
		hostID := ""
		entityTypeID := ""
		if rc.Entity != nil {
			hostID = rc.Entity.ID
			entityTypeID = strconv.Itoa(rc.Entity.EntityTypeID)
		}
		var res widgetbindings.Resolution
		if rc.CC != nil && rc.CC.Campaign != nil && hostID != "" {
			host := widgetbindings.HostRef{
				CampaignID:   rc.CC.Campaign.ID,
				Type:         widgetbindings.HostTypeEntity,
				ID:           hostID,
				EntityTypeID: entityTypeID,
			}
			if r, err := widgetBindingSvc.Resolve(context.Background(), host, widgetType); err == nil {
				res = r
			}
		}
		// Legacy fallback (maps entity.map_id) only when nothing resolved.
		if !res.Resolved() && legacyID != "" {
			res = widgetbindings.Resolution{InstanceID: legacyID, Source: widgetbindings.SourceDefault, WidgetType: widgetType}
		}
		role := 0
		if rc.CC != nil {
			role = rc.CC.VisibilityRole()
		}
		return wt.RenderBlock(context.Background(), widgetbindings.BlockRenderContext{
			CC:         rc.CC,
			HostID:     hostID,
			UserID:     rc.UserID,
			CSRFToken:  rc.CSRFToken,
			Role:       role,
			Resolution: res,
		})
	}

	// Calendar plugin blocks (requires "calendar" addon).
	// NOTE: the old per-entity `calendar` block (BlockCalendarEvents) was
	// retired in C-CAL-EMBED-CONVERGE-POLISH — it was never used and is
	// superseded by `entity_calendar` (the worldstate band + #402 linked
	// events). Entity pages now offer exactly one calendar block.
	blockRegistry.Register(entities.BlockMeta{
		Type: "upcoming_events", Label: "Upcoming Events", Icon: "fa-calendar-check",
		Description: "Upcoming calendar events list", Addon: "calendar",
		Contexts: []string{"template"},
		ConfigFields: []entities.ConfigFieldMeta{
			{Key: "limit", Label: "Events to show", Type: "number", Min: entities.IntPtr(1), Max: entities.IntPtr(20), Default: 5},
		},
	}, func(ctx entities.BlockRenderContext) templ.Component {
		return calendar.BlockUpcomingEvents(ctx.CC, entities.BlockConfigLimit(ctx.Block.Config, "limit", 5))
	})
	// entity_calendar — the entity-PAGE calendar embed (C-CAL-ENTITY-PAGE-EMBED,
	// Phase 6). Template context + REAL renderer (the closure captures
	// calendarService, like map_editor): a compact worldstate band (#401 seed)
	// + THIS entity's linked events (#402 EventsForEntity, dm_only filtered).
	// Singleton — the band binds the engine's fixed #cal-v2-worldstate id, so
	// one per page. Distinct from calendar_preview (dashboard upcoming-events
	// card) by design.
	blockRegistry.Register(entities.BlockMeta{
		Type: "entity_calendar", Label: "Calendar (this entity)", Icon: "fa-calendar-days",
		Description: "Ambient calendar + this entity's linked events",
		Addon:       "calendar", Contexts: []string{"template"}, Singleton: true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		// Resolve the calendar instance + render via the framework seam
		// (C-WIDGET-BINDING-P4b): own binding → entity-type template → default
		// (campaign default calendar = today's behavior). EntityCalendarBlock
		// renders the friendly not-found state itself when context is missing;
		// unbound entities render exactly as before (#411–#420 unchanged).
		return renderBoundBlock(calendar.WidgetTypeCalendar, rc, "")
	})

	// entity_worldstate — the entity-PAGE worldState timepiece embed
	// (C-CAL-WORLDSTATE-WIDGETS, Phase 6 widgetization). The "mini shelf
	// view": the hourglass-on-shelf over a compact sky band, painted by the
	// shared engine from the #401 seed and driven live by the worldState
	// provider singleton (widgets/worldstate_provider.js). Completes "all
	// three views entity-able" (calendar #411/#413, timeline Tuner #414,
	// worldstate here). Singleton — like entity_calendar it binds the
	// engine's fixed #cal-v2-worldstate id, so one worldState surface per
	// page (operator picks entity_calendar OR entity_worldstate per page).
	// Campaign-level (no per-entity data), so it works in BOTH the entity
	// page (template) and the campaign dashboard contexts (dispatch D).
	blockRegistry.Register(entities.BlockMeta{
		Type: "entity_worldstate", Label: "Worldstate timepiece", Icon: "fa-hourglass-half",
		Description: "Ambient sky + hourglass shelf for the current world date",
		Addon:       "calendar", Contexts: []string{"template", "dashboard"}, Singleton: true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		// Resolve the host's hourglass calendar via the "worldstate" widget type
		// + render via the framework seam (C-WIDGET-BINDING-P4b). Empty/unbound →
		// today's behavior (campaign default calendar). On the campaign-dashboard
		// context rc.Entity is nil → no host → default + no affordance (P3b).
		return renderBoundBlock(calendar.WidgetTypeWorldstate, rc, "")
	})

	// skybox — the ambient SKY-ONLY block (C-SKYBOX-WIDGET): no hourglass, no
	// per-entity binding (always the campaign's default calendar — a "which
	// calendar" choice isn't meaningful without an hourglass to bind it to).
	// Campaign-level, so — like entity_worldstate — it works in both the
	// entity page (template) and the campaign dashboard contexts.
	blockRegistry.Register(entities.BlockMeta{
		Type: "skybox", Label: "Sky", Icon: "fa-cloud-sun",
		Description: "Ambient sky only — moons, stars, weather + celestial events for the current world date",
		Addon:       "calendar", Contexts: []string{"template", "dashboard"}, Singleton: true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		entityID := ""
		if rc.Entity != nil {
			entityID = rc.Entity.ID
		}
		return calendar.EntitySkyboxBlock(calendarService, rc.CC, entityID, rc.UserID)
	})

	// Timeline plugin blocks (requires "timeline" addon).
	blockRegistry.Register(entities.BlockMeta{
		Type: "timeline", Label: "Timeline", Icon: "fa-timeline",
		Description: "Timeline preview with events", Addon: "timeline",
		Contexts: []string{"template"},
	}, func(rc entities.BlockRenderContext) templ.Component {
		// Resolve the host's bound timeline + render via the framework seam
		// (C-WIDGET-BINDING-P4b). Unbound (no default) → empty instance →
		// BlockTimeline keeps today's campaign preview list. Bound → that one.
		return renderBoundBlock(timeline.WidgetTypeTimeline, rc, "")
	})

	// Maps plugin blocks (requires "maps" addon).
	//
	// map_editor — per-entity Map Editor block. Reads from entities.map_id
	// (NOT from block config) so the choice lives on the entity itself,
	// not on a shared entity-type layout. Template context only — this
	// block is tied to a specific entity. Three render branches:
	//   - entity has map_id: full inline editor (markers + drawings +
	//     settings — same widget the dedicated map page uses) with a
	//     "Change map" button for Scribe+. Players get view-only.
	//   - no map_id + Scribe+: thumbnail picker grid.
	//   - no map_id + Player: friendly empty state.
	// The closure captures mapsService (built earlier in this file) so
	// the picker grid can list the campaign's maps and the embed branch
	// can build a full MapViewData server-side.
	//
	// Replaced map_preview / map_full as the recommended way to embed a
	// map on an entity page — those were limited to view-only and stored
	// the map_id at the layout level (same map across all entities of a
	// type), which the operator's per-entity use case made awkward.
	blockRegistry.Register(entities.BlockMeta{
		Type: "map_editor", Label: "Map Editor", Icon: "fa-map-location-dot",
		Description: "Full per-entity map (markers, drawings, settings)",
		Addon:       "maps", Contexts: []string{"template"},
		// No ConfigFields — the source of truth is entity.MapID. The
		// picker is rendered by the block itself, not the layout editor.
		// Singleton: only one map_editor per layout. The IIFE inside
		// MapEditorBody binds fixed DOM IDs (#map-container, #marker-modal,
		// etc.) that would collide with multiple instances. Enforced in
		// three layers — see BlockMeta.Singleton docstring.
		Singleton: true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		// P4b: resolve + render via the framework seam (the embed/choose/empty
		// branch logic now lives in maps.mapWidgetType.RenderBlock so the binding
		// handler can re-render after a bind/unbind). LEGACY FALLBACK preserved:
		// a widget_bindings row (widget_type="map") wins; an unbound entity with
		// a legacy entity.map_id renders that map (identical to today). The
		// bespoke AssignMap picker grid is superseded by the generic picker.
		legacyID := ""
		if rc.Entity != nil && rc.Entity.MapID != nil {
			legacyID = *rc.Entity.MapID
		}
		return renderBoundBlock(maps.WidgetTypeMap, rc, legacyID)
	})

	// NPC gallery block — embeds a compact NPC grid on entity pages/dashboards.
	blockRegistry.Register(entities.BlockMeta{
		Type: "npc_gallery", Label: "NPC Gallery", Icon: "fa-users",
		Description: "Grid of revealed NPCs", Addon: "npcs",
		Contexts: []string{"template"},
	}, func(bctx entities.BlockRenderContext) templ.Component {
		limit := entities.BlockConfigLimit(bctx.Block.Config, "limit", 8)
		cards, err := npcHandler.GalleryBlock(context.Background(), bctx.CC.Campaign.ID, int(bctx.CC.MemberRole), "", limit)
		if err != nil {
			return templ.NopComponent
		}
		return npcs.BlockNPCGallery(bctx.CC, cards, limit)
	})

	// Armory preview block — embeds a compact item grid on entity pages/dashboards.
	blockRegistry.Register(entities.BlockMeta{
		Type: "armory_preview", Label: "Armory Preview", Icon: "fa-shield-halved",
		Description: "Grid of campaign items", Addon: "armory",
		Contexts: []string{"template"},
	}, func(bctx entities.BlockRenderContext) templ.Component {
		limit := entities.BlockConfigLimit(bctx.Block.Config, "limit", 8)
		cards, err := armoryHandler.GalleryBlock(context.Background(), bctx.CC.Campaign.ID, int(bctx.CC.MemberRole), "", limit)
		if err != nil {
			return templ.NopComponent
		}
		return armory.BlockArmoryPreview(bctx.CC, cards, limit)
	})

	// Entity manager block — sortable, filterable entity list with visibility controls.
	blockRegistry.Register(entities.BlockMeta{
		Type: "entity_manager", Label: "Entity Manager", Icon: "fa-list-check",
		Description: "Sortable, filterable entity list with visibility controls",
		Contexts:    []string{"template"},
	}, func(bctx entities.BlockRenderContext) templ.Component {
		typeID := entities.BlockConfigInt(bctx.Block.Config, "entity_type_id", 0)
		return entities.BlockEntityManager(bctx.CC, typeID, bctx.CSRFToken)
	})

	// Set the registry on the entity service (validation) and as the global (rendering).
	// The addon checker lets Render() skip blocks whose addon is disabled.
	blockRegistry.SetAddonChecker(addonService)
	entityService.SetBlockRegistry(blockRegistry)
	entities.SetGlobalBlockRegistry(blockRegistry)
	entityHandler.SetBlockRegistry(blockRegistry)
	entityHandler.SetWidgetBlockLister(&widgetBlockListerAdapter{extHandler: extHandler})

	// Slug-keyed entity-show renderer registry (CH4). System packages
	// register character / monster / item renderers here at startup.
	// Empty registry is fine: lookupEntityShowRenderer returns nil for
	// every slug → show.templ falls through to the standard block
	// dispatch. See docs/system-package-rendering.md for the
	// system-package-author contract.
	//
	// V1 registers no built-in renderers — the host ships zero
	// character-specific code by design. System packages (Draw Steel,
	// future D&D 5.5e, etc.) hook in by adding a call here, mirroring
	// the calendar.RegisterCalendarBlock pattern. Example, commented
	// out until DS-CH1 lands the function:
	//
	//   drawsteel.RegisterEntityShowRenderers(showRegistry)
	//
	showRegistry := entities.NewEntityShowRendererRegistry()
	registerManifestRenderers(showRegistry)
	entities.SetGlobalEntityShowRendererRegistry(showRegistry)

	campaignHandler.SetAuditLogger(&campaignAuditAdapter{svc: auditService})
	campaignHandler.SetAddonLister(&addonListerAdapter{svc: addonService})
	campaignHandler.SetSystemAddonEnabler(addonService)
	campaignHandler.SetMediaUploader(&backdropUploaderAdapter{svc: mediaService})
	campaignHandler.SetSMTPChecker(smtpService)
	campaignHandler.SetSystemLister(&systemListerAdapter{})
	tagHandler.SetAuditService(auditService)

	// --- AI Workspace (C-AI-WORKSPACE-V1-B) ---
	// First plugin built under the post-NW-2.2 isolation rules.
	// Owns the AI Export feature (relocated from internal/aiexport),
	// the Prompt builder (Phase 3), and the AI Import surface
	// (Phases 4-5). The renderer Service depends on narrow per-plugin
	// listers; every plugin Service already implements them.
	//
	// Wiring:
	//   1. Construct the renderer with each plugin's Service.
	//   2. Build the plugin handler around the renderer + audit hook.
	//   3. Register the settings-tab factory with campaigns.
	//   4. Mount the owner-gated routes on a /campaigns/:id group
	//      that already enforces auth + campaign membership (mirror
	//      of foundry_vtt's RegisterOwnerRoutes pattern).
	//
	// D4=(c) lossless backup carve-out preserved — no edits to
	// internal/app/export_adapters.go or the restore pipeline.
	aiWorkspaceRenderer := aiexport.NewService(
		entityService,
		noteSvc,
		calendarService,
		sessionsService,
		timelineSvc,
		relService,
		tagService,
	)
	aiWorkspaceHandler := ai_workspace.NewHandler(aiWorkspaceRenderer)
	aiWorkspaceHandler.SetAuditLogger(&aiWorkspaceAuditAdapter{svc: auditService})

	// Phase 3 — Prompt builder. Reuses the relocated aiexport renderer
	// as the content Exporter so the prompt's "Existing world context"
	// section inherits SEC-6-AMENDED egress sanitization + the privacy
	// modes without duplicating logic.
	aiWorkspacePrompt := prompt.NewService(
		entityService,
		tagService,
		aiWorkspaceRenderer,
	)
	aiWorkspaceHandler.SetPromptBuilder(aiWorkspacePrompt)

	// Phase 4 — Import parse + review. The entities service
	// implements importer.CampaignLookup as-is (GetBySlug +
	// GetEntityTypeBySlug + GetEntityTypes are already on its
	// public interface). No adapter needed.
	aiWorkspaceHandler.SetImportLookup(entityService)

	// Phase 5 — Import commit. The entities service also
	// implements importer.EntityCreator (Create + Update +
	// UpdateEntry + CreateEntityType + the lookup methods).
	// SEC-6-AMENDED ingress mirror is enforced by the AST pin in
	// internal/plugins/ai_workspace/importer/committer_sanitize_test.go.
	aiWorkspaceHandler.SetImportCommitter(importer.NewCommitter(entityService))

	campaignHandler.RegisterSettingsTab(aiWorkspaceHandler.SettingsTabFactory())

	aiWorkspaceCampaignAuthed := e.Group("/campaigns/:id",
		auth.RequireAuth(authService),
		campaigns.RequireCampaignAccess(campaignService),
	)
	ai_workspace.RegisterOwnerRoutes(aiWorkspaceCampaignAuthed, aiWorkspaceHandler,
		campaigns.RequireRole(campaigns.RoleOwner))

	// --- Campaign Export/Import ---
	exportSvc := campaigns.NewExportImportService(campaignService)
	exportSvc.SetEntityExporter(&entityExportAdapter{entitySvc: entityService, tagSvc: tagService, relationSvc: relService})
	exportSvc.SetCalendarExporter(&calendarExportAdapter{svc: calendarService})
	exportSvc.SetTimelineExporter(&timelineExportAdapter{svc: timelineSvc})
	exportSvc.SetSessionExporter(&sessionExportAdapter{svc: sessionsService})
	exportSvc.SetMapExporter(&mapExportAdapter{mapSvc: mapsService, drawingSvc: drawingService})
	exportSvc.SetAddonExporter(&addonExportAdapter{svc: addonService})
	exportSvc.SetMediaExporter(&mediaExportAdapter{svc: mediaService})
	exportSvc.SetMediaBundler(&mediaBundleAdapter{svc: mediaService})
	exportSvc.SetEntityImporter(&entityImportAdapter{entitySvc: entityService, tagSvc: tagService, relationSvc: relService})
	exportSvc.SetCalendarImporter(&calendarImportAdapter{svc: calendarService})
	exportSvc.SetTimelineImporter(&timelineImportAdapter{svc: timelineSvc})
	exportSvc.SetSessionImporter(&sessionImportAdapter{svc: sessionsService})
	exportSvc.SetMapImporter(&mapImportAdapter{mapSvc: mapsService, drawingSvc: drawingService})
	exportSvc.SetAddonImporter(&addonImportAdapter{svc: addonService})
	exportSvc.SetGroupExporter(&groupExportAdapter{svc: groupService})
	exportSvc.SetGroupImporter(&groupImportAdapter{svc: groupService})
	exportSvc.SetPostExporter(&postExportAdapter{postSvc: postService, entitySvc: entityService})
	exportSvc.SetPostImporter(&postImportAdapter{svc: postService})
	exportHandler := campaigns.NewExportHandler(exportSvc)
	campaigns.RegisterExportRoutes(e, exportHandler, campaignService, authService)

	// --- Content Extension Applier ---
	// Wire the content applier now that entity and tag services are available.
	// The applier creates campaign content (entity types, tags, etc.) when an
	// extension is enabled, with provenance tracking for clean removal.
	extApplier := extensions.NewContentApplier(
		a.Config.ExtensionsPath,
		extRepo,
		extensions.NewEntityTypeAdapter(func(ctx context.Context, campaignID string, name, namePlural, icon, color, presetCategory string) (int, string, error) {
			et, err := entityService.CreateEntityType(ctx, campaignID, entities.CreateEntityTypeInput{
				Name:           name,
				NamePlural:     namePlural,
				Icon:           icon,
				Color:          color,
				PresetCategory: presetCategory,
			})
			if err != nil {
				return 0, "", err
			}
			return et.ID, et.Slug, nil
		}),
		extensions.NewTagAdapter(func(ctx context.Context, campaignID string, name, color string, dmOnly bool) (int, error) {
			t, err := tagService.Create(ctx, campaignID, name, color, dmOnly)
			if err != nil {
				return 0, err
			}
			return t.ID, nil
		}),
	)
	extService.SetApplier(extApplier)

	// --- WASM Runtime (Layer 3 Logic Extensions) ---
	// Wire the WASM plugin manager with read-only host functions that let
	// sandboxed WASM plugins query entities, calendar, and tags.
	wasmEntityReader := extensions.NewWASMEntityAdapter(
		// get_entity: returns entity JSON by ID.
		func(ctx context.Context, id string) (json.RawMessage, error) {
			ent, err := entityService.GetByID(ctx, id)
			if err != nil {
				return nil, err
			}
			return json.Marshal(ent)
		},
		// search_entities: returns matching entities as JSON array.
		func(ctx context.Context, campaignID, query string, limit int) (json.RawMessage, error) {
			results, _, err := entityService.Search(ctx, campaignID, query, 0, int(campaigns.RoleOwner), "", entities.ListOptions{Page: 1, PerPage: limit})
			if err != nil {
				return nil, err
			}
			return json.Marshal(results)
		},
		// list_entity_types: returns entity types as JSON array.
		func(ctx context.Context, campaignID string) (json.RawMessage, error) {
			types, err := entityService.GetEntityTypes(ctx, campaignID)
			if err != nil {
				return nil, err
			}
			return json.Marshal(types)
		},
	)

	wasmCalendarReader := extensions.NewWASMCalendarAdapter(
		// get_calendar: returns calendar config JSON.
		func(ctx context.Context, campaignID string) (json.RawMessage, error) {
			cal, err := calendarService.GetCalendar(ctx, campaignID)
			if err != nil {
				return nil, err
			}
			return json.Marshal(cal)
		},
		// list_events: returns upcoming calendar events as JSON.
		func(ctx context.Context, campaignID string, limit int) (json.RawMessage, error) {
			cal, err := calendarService.GetCalendar(ctx, campaignID)
			if err != nil {
				return nil, err
			}
			events, err := calendarService.ListUpcomingEvents(ctx, cal.ID, limit, int(campaigns.RoleOwner), "")
			if err != nil {
				return nil, err
			}
			return json.Marshal(events)
		},
	)

	wasmTagReader := extensions.NewWASMTagAdapter(
		// list_tags: returns all campaign tags as JSON.
		func(ctx context.Context, campaignID string) (json.RawMessage, error) {
			tags, err := tagService.ListByCampaign(ctx, campaignID, true)
			if err != nil {
				return nil, err
			}
			return json.Marshal(tags)
		},
	)

	wasmKVStore := extensions.NewKVStore(extRepo)
	wasmHostEnv := extensions.NewHostEnvironment(wasmEntityReader, wasmCalendarReader, wasmTagReader, wasmKVStore)

	// Wire write adapters for WASM host functions.
	wasmHostEnv.SetEntityWriter(extensions.NewWASMEntityWriteAdapter(
		// update_entity_fields: unmarshal JSON fields and delegate to entity service.
		func(ctx context.Context, entityID string, fieldsData json.RawMessage) error {
			var fields map[string]any
			if err := json.Unmarshal(fieldsData, &fields); err != nil {
				return fmt.Errorf("invalid fields JSON: %w", err)
			}
			return entityService.UpdateFields(ctx, entityID, fields)
		},
	))

	wasmHostEnv.SetCalendarWriter(extensions.NewWASMCalendarWriteAdapter(
		// create_event: unmarshal JSON input and delegate to calendar service.
		func(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error) {
			cal, err := calendarService.GetCalendar(ctx, campaignID)
			if err != nil {
				return nil, fmt.Errorf("getting calendar: %w", err)
			}
			var eventInput calendar.CreateEventInput
			if err := json.Unmarshal(input, &eventInput); err != nil {
				return nil, fmt.Errorf("invalid event input: %w", err)
			}
			event, err := calendarService.CreateEvent(ctx, cal.ID, eventInput)
			if err != nil {
				return nil, err
			}
			return json.Marshal(event)
		},
	))

	wasmHostEnv.SetTagWriter(extensions.NewWASMTagWriteAdapter(
		// set_entity_tags: unmarshal tag IDs and delegate to tag service.
		func(ctx context.Context, entityID, campaignID string, tagIDsJSON json.RawMessage) error {
			var tagIDs []int
			if err := json.Unmarshal(tagIDsJSON, &tagIDs); err != nil {
				return fmt.Errorf("invalid tag_ids JSON: %w", err)
			}
			return tagService.SetEntityTags(ctx, entityID, campaignID, tagIDs)
		},
		// get_entity_tags: return tags as JSON (include DM-only for WASM plugins).
		func(ctx context.Context, entityID string) (json.RawMessage, error) {
			entityTags, err := tagService.GetEntityTags(ctx, entityID, true)
			if err != nil {
				return nil, err
			}
			return json.Marshal(entityTags)
		},
	))

	wasmHostEnv.SetRelationWriter(extensions.NewWASMRelationWriteAdapter(
		// create_relation: unmarshal relation input and delegate to relation service.
		func(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error) {
			var req struct {
				SourceEntityID      string          `json:"source_entity_id"`
				TargetEntityID      string          `json:"target_entity_id"`
				RelationType        string          `json:"relation_type"`
				ReverseRelationType string          `json:"reverse_relation_type"`
				CreatedBy           string          `json:"created_by"`
				Metadata            json.RawMessage `json:"metadata"`
				DmOnly              bool            `json:"dm_only"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return nil, fmt.Errorf("invalid relation input: %w", err)
			}
			rel, err := relService.Create(ctx, campaignID, req.SourceEntityID, req.TargetEntityID,
				req.RelationType, req.ReverseRelationType, req.CreatedBy, req.Metadata, req.DmOnly)
			if err != nil {
				return nil, err
			}
			return json.Marshal(rel)
		},
	))

	wasmPluginMgr := extensions.NewPluginManager(a.Config.ExtensionsPath, wasmHostEnv)
	wasmHostEnv.SetPluginManager(wasmPluginMgr)
	wasmHookDispatcher := extensions.NewHookDispatcher(wasmPluginMgr)
	wasmHandler := extensions.NewWASMHandler(wasmPluginMgr, wasmHookDispatcher, extService)
	extensions.RegisterWASMAdminRoutes(adminGroup, wasmHandler)
	extensions.RegisterWASMCampaignRoutes(e, wasmHandler, campaignService, authService)

	// Wire WASM loader into the content applier so enabling an extension
	// with WASM plugins automatically loads them into the plugin manager.
	extApplier.SetWASMLoader(wasmPluginMgr)

	// Store references for graceful shutdown and auto-loading.
	a.WASMPluginManager = wasmPluginMgr
	a.WASMHookDispatcher = wasmHookDispatcher

	// Wire campaign deletion cleanup: media files and WASM hooks.
	campaignService.SetMediaCleaner(mediaService)
	campaignService.SetHookDispatcher(wasmHookDispatcher)

	// --- Game Systems ---
	// System reference pages and tooltip API, gated by per-campaign addon checks.
	// Custom system manager stores per-campaign uploads under media/systems/.
	campaignSystemMgr := systems.NewCampaignSystemManager(filepath.Join(a.Config.Upload.MediaPath, "systems"))
	systemHandler := systems.NewSystemHandler()
	systemHandler.SetCampaignSystems(campaignSystemMgr)
	systemHandler.SetAddonService(addonService)
	systems.RegisterRoutes(e, systemHandler, addonService, authService, campaignService)

	// Admin-only deployment-health diagnostic: read-only fingerprints of the
	// version + files each system loader is ACTUALLY serving, to catch the
	// "Packages says X but the old file renders" mismatch from the UI.
	adminGroup.GET("/extensions/health", systemHandler.ExtensionsHealthAPI)

	// Operator AI-diagnostics report (copy-paste to the AI assistant): the
	// served-reality systems table + a modular run-and-paste-back probe library.
	adminGroup.GET("/diagnostics", systemHandler.OperatorDiagnosticsAPI)

	// Wire system widgets into the template editor palette (deferred from
	// block registry setup because systemHandler is created after entityHandler).
	entityHandler.SetWidgetBlockLister(&widgetBlockListerAdapter{extHandler: extHandler, sysHandler: systemHandler})
	campaignSystemHandler := systems.NewCampaignSystemHandler(campaignSystemMgr)
	// Wire upload policy provider so campaign handler checks admin setting.
	campaignSystemHandler.SetUploadPolicy(func(ctx context.Context) string {
		if a.PluginHealth.IsHealthy("packages") {
			if settings, err := pkgService.GetSecuritySettings(ctx); err == nil {
				return settings.OwnerUploadPolicy
			}
		}
		return "auto_approve"
	})
	// Wire entity data providers for the system diagnostics page.
	campaignSystemHandler.SetEntityDeps(
		func(ctx context.Context, campaignID string) (map[int]int, error) {
			return entityService.CountByType(ctx, campaignID, 3, "") // role=3 (owner) to see all
		},
		func(ctx context.Context, campaignID string) ([]systems.DiagEntityType, error) {
			types, err := entityService.GetEntityTypes(ctx, campaignID)
			if err != nil {
				return nil, err
			}
			result := make([]systems.DiagEntityType, len(types))
			for i, t := range types {
				cat := ""
				if t.PresetCategory != nil {
					cat = *t.PresetCategory
				}
				result[i] = systems.DiagEntityType{
					ID: t.ID, Name: t.Name, Slug: t.Slug,
					PresetCategory: cat,
				}
			}
			return result, nil
		},
	)
	systems.RegisterCustomSystemRoutes(e, campaignSystemHandler, authService, campaignService)

	// Wire campaign system lister into sync API so custom systems appear
	// in /systems and /systems/:id/character-fields endpoints.
	syncAPIHandler.SetCampaignSystemLister(campaignSystemMgr)

	// Dashboard redirects to campaigns list for authenticated users.
	e.GET("/dashboard", func(c echo.Context) error {
		return c.Redirect(http.StatusSeeOther, "/campaigns")
	}, auth.RequireAuth(authService))

	// Candidate calendar designs for the V2 plugin port. Per the
	// operator's page-separation directive (2026-06-03): each design
	// lives on its OWN isolated route loading ONLY its own CSS+JS, so a
	// bug in one design can never affect another. `/demo/calendar` is a
	// tiny plain-link index (no design assets); each design is a sibling
	// route. Designs 2 (Linear) + 3 (Compact) get their own routes when
	// they ship. Mock data only (no backend); operator selects the
	// winning design, the real plugin port follows. Auth-gated; exposes
	// no campaign data. Dispatches:
	// dispatches/chronicle/C-CAL-SHOWCASE-DESIGN-1-ALMANAC.md +
	// dispatches/chronicle/C-CAL-SHOWCASE-DESIGN-2-LINEAR.md.
	e.GET("/demo/calendar", func(c echo.Context) error {
		return middleware.Render(c, http.StatusOK, demo.DemoCalendarIndex())
	}, auth.RequireAuth(authService))
	e.GET("/demo/calendar/almanac", func(c echo.Context) error {
		return middleware.Render(c, http.StatusOK, demo.DemoCalendarAlmanac())
	}, auth.RequireAuth(authService))
	// Timeline showcase (C-TIMELINE-V2-DESIGN-1-TUNER). Own isolated
	// route per the page-separation directive; loads only its own
	// CSS+JS. Lead of two candidate timeline designs (Ledger alternate).
	e.GET("/demo/timeline/tuner", func(c echo.Context) error {
		return middleware.Render(c, http.StatusOK, demo.DemoTimelineTuner())
	}, auth.RequireAuth(authService))
	// Ledger timeline showcase (Timeline V2 arc W0, cordinator#36): the flat
	// record-keeping alternate to the Tuner — the second of the two candidate
	// designs the index has promised since the timeline showcase shipped.
	// Same isolation rules: own route, own self-contained CSS, mock data only.
	e.GET("/demo/timeline/ledger", func(c echo.Context) error {
		return middleware.Render(c, http.StatusOK, demo.DemoTimelineLedger())
	}, auth.RequireAuth(authService))

	// --- Layout Data Injector ---
	// Registers the callback that copies auth/campaign data from Echo's
	// context into Go's context.Context so Templ templates can read it.
	// This runs inside middleware.Render() before every template render.
	middleware.LayoutInjector = func(c echo.Context, ctx context.Context) context.Context {
		// Inject plugin-contributed body scripts (constant for process lifetime).
		// Allows plugins to register widget scripts without hardcoding paths in
		// the core base.templ layout (Finding 4 / M-B2.1 quick-win).
		ctx = layouts.SetPluginBodyScripts(ctx, pluginBodyScripts)

		// User info from auth session.
		if session := auth.GetSession(c); session != nil {
			ctx = layouts.SetIsAuthenticated(ctx, true)
			ctx = layouts.SetUserID(ctx, session.UserID)
			ctx = layouts.SetUserName(ctx, session.Name)
			ctx = layouts.SetUserEmail(ctx, session.Email)
			ctx = layouts.SetIsAdmin(ctx, session.IsAdmin)

			// Inject degraded plugin count for admin sidebar badge.
			if session.IsAdmin {
				ctx = layouts.SetDegradedPluginCount(ctx, len(a.PluginHealth.DegradedPlugins()))
			}
		}

		// Campaign info from campaign middleware.
		if cc := campaigns.GetCampaignContext(c); cc != nil {
			ctx = layouts.SetCampaignID(ctx, cc.Campaign.ID)
			ctx = layouts.SetCampaignName(ctx, cc.Campaign.Name)

			// Campaign visual customization from settings.
			campaignSettings := cc.Campaign.ParseSettings()
			if campaignSettings.AccentColor != "" {
				ctx = layouts.SetAccentColor(ctx, campaignSettings.AccentColor)
			}
			if campaignSettings.AccentSurface1 != "" {
				ctx = layouts.SetAccentSurface(ctx, 1, campaignSettings.AccentSurface1)
			}
			if campaignSettings.AccentSurface2 != "" {
				ctx = layouts.SetAccentSurface(ctx, 2, campaignSettings.AccentSurface2)
			}
			if campaignSettings.AccentAction != "" {
				ctx = layouts.SetAccentAction(ctx, campaignSettings.AccentAction)
			}
			if campaignSettings.AccentApp != "" {
				ctx = layouts.SetAccentApp(ctx, campaignSettings.AccentApp)
			}
			if campaignSettings.BrandName != "" {
				ctx = layouts.SetBrandName(ctx, campaignSettings.BrandName)
			}
			if campaignSettings.BrandLogo != "" {
				ctx = layouts.SetBrandLogo(ctx, campaignSettings.BrandLogo)
			}
			if campaignSettings.FontFamily != "" {
				ctx = layouts.SetFontFamily(ctx, campaignSettings.FontFamily)
			}
			if campaignSettings.TopbarStyle != nil {
				ctx = layouts.SetTopbarStyle(ctx, &layouts.TopbarStyleData{
					Mode:         campaignSettings.TopbarStyle.Mode,
					Color:        campaignSettings.TopbarStyle.Color,
					GradientFrom: campaignSettings.TopbarStyle.GradientFrom,
					GradientTo:   campaignSettings.TopbarStyle.GradientTo,
					GradientDir:  campaignSettings.TopbarStyle.GradientDir,
					ImagePath:    campaignSettings.TopbarStyle.ImagePath,
				})
			}
			if campaignSettings.TopbarContent != nil && campaignSettings.TopbarContent.Mode != "" && campaignSettings.TopbarContent.Mode != "none" {
				tc := &layouts.TopbarContentData{
					Mode:  campaignSettings.TopbarContent.Mode,
					Quote: campaignSettings.TopbarContent.Quote,
				}
				for _, link := range campaignSettings.TopbarContent.Links {
					tc.Links = append(tc.Links, layouts.TopbarLinkData{
						Label: link.Label,
						URL:   link.URL,
						Icon:  link.Icon,
					})
				}
				ctx = layouts.SetTopbarContent(ctx, tc)
			}

			// "View as player" override: when an owner has the toggle active,
			// templates see RolePlayer instead of RoleOwner. Access control
			// (RequireRole middleware) still uses the actual cc.MemberRole.
			effectiveRole := int(cc.MemberRole)
			isOwner := cc.MemberRole >= campaigns.RoleOwner
			ctx = layouts.SetIsOwner(ctx, isOwner)
			if isOwner {
				if cookie, err := c.Cookie("chronicle_view_as_player"); err == nil && cookie.Value == "1" {
					effectiveRole = int(campaigns.RolePlayer)
					ctx = layouts.SetViewingAsPlayer(ctx, true)
				}
			}
			ctx = layouts.SetCampaignRole(ctx, effectiveRole)

			// Entity types for dynamic sidebar rendering.
			// Use the request context (not the enriched ctx) since service calls
			// only need cancellation/deadline, not layout data.
			reqCtx := c.Request().Context()
			if etypes, err := entityService.GetEntityTypes(reqCtx, cc.Campaign.ID); err == nil {
				sidebarTypes := make([]layouts.SidebarEntityType, len(etypes))
				for i, et := range etypes {
					sidebarTypes[i] = layouts.SidebarEntityType{
						ID:           et.ID,
						Slug:         et.Slug,
						Name:         et.Name,
						NamePlural:   et.NamePlural,
						Icon:         et.Icon,
						Color:        et.Color,
						SortOrder:    et.SortOrder,
						ParentTypeID: et.ParentTypeID,
					}
				}

				// Build the sidebar from the single unified items model. Sidebar
				// config carries only Items now (C-NAV-V3 retired the legacy
				// entity_type_order / hidden_type_ids / custom_sections /
				// custom_links model + its fallback render path). An empty Items
				// array is valid: injectDefaultSidebarItems synthesizes the full
				// default sidebar, so a never-customized (or reconciler-skipped)
				// campaign renders exactly as it did under the old legacy default.
				sidebarCfg := cc.Campaign.ParseSidebarConfig()

				// Entity types indexed by ID for quick lookup.
				typeMap := make(map[int]layouts.SidebarEntityType)
				for _, et := range sidebarTypes {
					typeMap[et.ID] = et
				}

				// Complete the items with the standard scaffold + any missing
				// entity types. Non-persistent: shapes this render only.
				items := injectDefaultSidebarItems(sidebarCfg.Items, sidebarTypes)

				var sidebarItems []layouts.SidebarItemView
				for _, item := range items {
					if !item.Visible {
						continue
					}
					switch item.Type {
					case "dashboard":
						sidebarItems = append(sidebarItems, layouts.SidebarItemView{
							Type: "dashboard", Label: "Dashboard",
							Icon: "fa-home",
						})
					case "addon":
						sidebarItems = append(sidebarItems, layouts.SidebarItemView{
							Type: "addon", Slug: item.Slug, Label: item.Label,
							Icon: item.Icon,
						})
					case "category":
						if et, ok := typeMap[item.TypeID]; ok {
							// Sub-category entity_types (ParentTypeID != nil) are
							// template variants of their parent, not navigable
							// collections. They must not appear in the sidebar —
							// they surface through the +New picker on the parent
							// instead. Skip them at build time; any persisted
							// sub-category SidebarItem rows are silently ignored.
							if et.ParentTypeID != nil {
								continue
							}
							sidebarItems = append(sidebarItems, layouts.SidebarItemView{
								Type: "category", TypeID: et.ID,
								Label: et.NamePlural, Icon: et.Icon, Color: et.Color,
								ParentTypeID: et.ParentTypeID,
							})
						}
					case "all_pages":
						sidebarItems = append(sidebarItems, layouts.SidebarItemView{
							Type: "all_pages", Label: "All Pages",
							Icon: "fa-layer-group",
						})
					case "section":
						sidebarItems = append(sidebarItems, layouts.SidebarItemView{
							Type: "section", ID: item.ID, Label: item.Label,
						})
					case "link":
						sidebarItems = append(sidebarItems, layouts.SidebarItemView{
							Type: "link", ID: item.ID, Label: item.Label,
							URL: item.URL, Icon: item.Icon,
						})
					}
				}
				ctx = layouts.SetSidebarItems(ctx, sidebarItems)
				// Still set entity types for the drill panel.
				ctx = layouts.SetEntityTypes(ctx, sidebarTypes)
			}

			// Entity counts per type for sidebar badges (use effectiveRole so
			// "view as player" mode hides private entity counts).
			// Pass user ID for permission-aware entity counts.
			layoutUserID := ""
			if session := auth.GetSession(c); session != nil {
				layoutUserID = session.UserID
			}
			if counts, err := entityService.CountByType(reqCtx, cc.Campaign.ID, effectiveRole, layoutUserID); err == nil {
				// CountByType already rolls child entity_type counts up into
				// their parent under the sub-category-as-template model.
				ctx = layouts.SetEntityCounts(ctx, counts)
			}

			// Enabled addons for conditional widget rendering.
			if campaignAddons, err := addonService.ListForCampaign(reqCtx, cc.Campaign.ID); err == nil {
				enabledSlugs := make(map[string]bool)
				var enabledSystem layouts.EnabledSystem
				for _, ca := range campaignAddons {
					if !ca.Enabled {
						continue
					}
					enabledSlugs[ca.AddonSlug] = true
					// The enabled game system backs the campaign's rulebook
					// (reference) nav link. Systems are mutually exclusive, so the
					// first enabled one wins.
					if ca.AddonCategory == addons.CategorySystem && enabledSystem.Slug == "" {
						enabledSystem = layouts.EnabledSystem{Slug: ca.AddonSlug, Name: ca.AddonName, Icon: ca.AddonIcon}
					}
				}
				ctx = layouts.SetEnabledAddons(ctx, enabledSlugs)
				if enabledSystem.Slug != "" {
					ctx = layouts.SetEnabledSystem(ctx, enabledSystem)
				}
			}

			// Extension widget scripts for campaign pages.
			if widgetURLs := extHandler.GetWidgetScriptURLs(reqCtx, cc.Campaign.ID); len(widgetURLs) > 0 {
				ctx = layouts.SetExtWidgetScripts(ctx, widgetURLs)
			}

			// System-provided widget scripts for the enabled game system.
			if sysScripts := systemHandler.GetSystemWidgetScriptURLs(reqCtx, cc.Campaign.ID); len(sysScripts) > 0 {
				existing := layouts.GetExtWidgetScripts(ctx)
				ctx = layouts.SetExtWidgetScripts(ctx, append(existing, sysScripts...))
			}
		}

		// Inject user campaigns for topbar navigation on non-campaign pages.
		// Skipped inside campaigns (sidebar handles navigation there).
		if session := auth.GetSession(c); session != nil && campaigns.GetCampaignContext(c) == nil {
			reqCtx := c.Request().Context()
			opts := campaigns.DefaultListOptions()
			opts.PerPage = 10
			if userCampaigns, _, err := campaignService.List(reqCtx, session.UserID, opts); err == nil && len(userCampaigns) > 0 {
				navCampaigns := make([]layouts.NavCampaign, len(userCampaigns))
				for i, uc := range userCampaigns {
					navCampaigns[i] = layouts.NavCampaign{
						ID:   uc.ID,
						Name: uc.Name,
					}
				}
				ctx = layouts.SetUserCampaigns(ctx, navCampaigns)
			}
		}

		// CSRF token for forms.
		ctx = layouts.SetCSRFToken(ctx, middleware.GetCSRFToken(c))

		// Active path for nav highlighting. Handlers can pre-set this on the
		// request context (e.g., entity show overrides to the category URL).
		if layouts.GetActivePath(ctx) == "" {
			ctx = layouts.SetActivePath(ctx, c.Request().URL.Path)
		}

		// Signed media URL generators for templates.
		if urlSigner != nil {
			ctx = layouts.SetMediaURLFunc(ctx, func(fileID string) string {
				return urlSigner.Sign(fileID, 1*time.Hour)
			})
			ctx = layouts.SetMediaThumbFunc(ctx, func(fileID, size string) string {
				return urlSigner.SignThumb(fileID, size, 1*time.Hour)
			})
		}

		return ctx
	}

	// --- WebSocket Hub ---
	// Real-time bidirectional sync for Foundry VTT and browser clients.
	wsHub := ws.NewHub()
	go wsHub.Run()

	// Wire the WS hub's presence lookup into foundry_vtt — the only
	// remaining consumer post-NW-2.3. fvttHandler now owns both:
	//   - GET /campaigns/:id/foundry-presence (live diagnostic JSON;
	//     relocated from campaigns.Handler in NW-2.3)
	//   - /foundry-vtt/presence-pill-fragment (lazy-loaded pill on the
	//     map detail page; landed in NW-2.2 Chunk D)
	// Both endpoints read through foundry_vtt.PresenceLookup, so a
	// single SetPresenceLookup call covers them. The campaigns-side
	// SetFoundryPresence wire was removed by NW-2.3; the maps-side
	// SetFoundryPresence wire was removed earlier in D2-cleanup.
	fvttHandler.SetPresenceLookup(wsHub)

	wsAuth := ws.NewMultiAuthenticator(
		syncService,
		&wsSessionAuthAdapter{svc: authService},
		&wsCampaignRoleAdapter{svc: campaignService},
	)
	// Dynamic CORS origins for WebSocket — reuse the same settings service
	// that backs the HTTP CORS middleware so the admin whitelist applies to
	// both REST API and WebSocket connections.
	wsDynamicOrigins := ws.DynamicOrigins(func() []string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		origins, err := settingsService.GetCORSOrigins(ctx)
		if err != nil {
			slog.Warn("failed to load dynamic CORS origins for ws", slog.Any("error", err))
			return nil
		}
		return origins
	})
	e.GET("/ws", ws.HandleUpgrade(wsHub, wsAuth, []string{a.Config.BaseURL}, wsDynamicOrigins))

	// Wire EventBus into services for real-time event publishing.
	wsEventBus := ws.NewEventBus(wsHub)

	entityService.SetEventPublisher(&entityEventPublisherAdapter{bus: wsEventBus})
	entityService.SetSidebarAutoAdder(&sidebarAutoAdderAdapter{campaignService: campaignService})
	calendarService.SetEventPublisher(&calendarEventPublisherAdapter{bus: wsEventBus})
	noteSvc.SetEventPublisher(&noteEventPublisherAdapter{bus: wsEventBus})

	// Late-bind the entity_notes notifier now that wsEventBus exists.
	// The service was constructed earlier with a holder.Notify reference;
	// setting holder.bus here makes future mutations broadcast over WS.
	entityNotesNotifier.bus = wsEventBus

	drawingService.SetEventPublisher(&mapEventPublisherAdapter{bus: wsEventBus})
	drawingService.SetMapLookup(func(ctx context.Context, mapID string) (string, error) {
		m, err := mapsService.GetMap(ctx, mapID)
		if err != nil {
			return "", err
		}
		return m.CampaignID, nil
	})
	mapsService.SetEventPublisher(&mapEventPublisherAdapter{bus: wsEventBus})

	// --- Module Routes ---
	// Game system reference pages and tooltip APIs.
	// ref := e.Group("/ref")
	// dnd5eModule.RegisterRoutes(ref)

	// --- API Routes ---
	// REST API v1 is registered above via syncapi.RegisterAPIRoutes().
	// Endpoints: /api/v1/campaigns/:id/{entity-types,entities,sync}

	// --- Plugin Static Assets ---
	// NW-2.2 Chunk F: mount each registered plugin's static assets at
	// /static/plugins/<slug>/. Must run AFTER all plugins have called
	// a.registerPlugin() above. Per
	// cordinator/decisions/2026-05-25-plugin-static-assets.md.
	a.mountPluginStatic()
}

// mediaUploadAdapter adapts MediaService to the notes.MediaUploader interface.
type mediaUploadAdapter struct {
	svc media.MediaService
}

// UploadRaw stores a file via the media service and returns the relative path.
func (a *mediaUploadAdapter) UploadRaw(ctx context.Context, campaignID, userID string, fileBytes []byte, originalName, mimeType string) (string, error) {
	file, err := a.svc.Upload(ctx, media.UploadInput{
		CampaignID:   campaignID,
		UploadedBy:   userID,
		OriginalName: originalName,
		MimeType:     mimeType,
		FileSize:     int64(len(fileBytes)),
		UsageType:    "attachment",
		FileBytes:    fileBytes,
	})
	if err != nil {
		return "", err
	}
	return file.Filename, nil
}

// aiWorkspaceAuditAdapter bridges audit.AuditService to the narrow
// ai_workspace.AuditLogger contract. The plugin doesn't import the
// audit package directly — same isolation pattern as
// campaignAuditAdapter above (this adapter wraps the same underlying
// service, just exposes a different narrow interface).
type aiWorkspaceAuditAdapter struct {
	svc audit.AuditService
}

func (a *aiWorkspaceAuditAdapter) LogCampaignEvent(ctx context.Context, campaignID, action string, details map[string]any) {
	// Fire-and-forget; audit failures must not break the response.
	_ = a.svc.Log(ctx, &audit.AuditEntry{
		CampaignID: campaignID,
		Action:     action,
		Details:    details,
	})
}
