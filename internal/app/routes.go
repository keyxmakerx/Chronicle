package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/systems"
	"github.com/keyxmakerx/chronicle/internal/extensions"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
	"github.com/keyxmakerx/chronicle/internal/plugins/smtp"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
	"github.com/keyxmakerx/chronicle/internal/templates/pages"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/npcs"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/posts"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

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
			ID:          et.ID,
			Name:        et.Name,
			NamePlural:  et.NamePlural,
			Icon:        et.Icon,
			Color:       et.Color,
			Description: et.Description,
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

// addonListerAdapter wraps addons.AddonService to implement the
// campaigns.AddonLister interface for the plugin hub page.
type addonListerAdapter struct {
	svc addons.AddonService
}

// ListForPluginHub returns all addons formatted for the plugin hub page.
func (a *addonListerAdapter) ListForPluginHub(ctx context.Context, campaignID string) ([]campaigns.PluginHubAddon, error) {
	addonList, err := a.svc.ListForCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	result := make([]campaigns.PluginHubAddon, len(addonList))
	for i, ca := range addonList {
		result[i] = campaigns.PluginHubAddon{
			AddonID:   ca.AddonID,
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
type entityTagFetcherAdapter struct {
	svc tags.TagService
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
			infos[i] = entities.EntityTagInfo{Name: t.Name, Color: t.Color}
		}
		result[eid] = infos
	}
	return result, nil
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

// AuthenticateSessionForWS validates the chronicle_session cookie and returns the user ID.
func (a *wsSessionAuthAdapter) AuthenticateSessionForWS(r *http.Request) (string, error) {
	cookie, err := r.Cookie("chronicle_session")
	if err != nil || cookie.Value == "" {
		return "", fmt.Errorf("no session cookie")
	}
	session, err := a.svc.ValidateSession(r.Context(), cookie.Value)
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

// calendarEventPublisherAdapter bridges the websocket.EventBus to the
// calendar.CalendarEventPublisher interface.
type calendarEventPublisherAdapter struct {
	bus ws.EventBus
}

// PublishCalendarEvent translates calendar domain events into WebSocket messages.
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
	case "weather.changed":
		msgType = ws.MsgCalendarWeatherChanged
	case "structure.updated":
		msgType = ws.MsgCalendarStructureUpdated
	case "season.changed":
		msgType = ws.MsgCalendarSeasonChanged
	case "era.changed":
		msgType = ws.MsgCalendarEraChanged
	case "moon.phase_changed":
		msgType = ws.MsgCalendarMoonPhaseChanged
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

// mapEventPublisherAdapter bridges the websocket.EventBus to the maps.MapEventPublisher
// interface, translating domain events into WebSocket messages.
type mapEventPublisherAdapter struct {
	bus ws.EventBus
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
	a.bus.Publish(ws.NewMessage(msgType, campaignID, drawing.ID, drawing))
}

// PublishTokenEvent translates map token domain events into WebSocket messages.
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
	a.bus.Publish(ws.NewMessage(msgType, campaignID, token.ID, token))
}

// PublishTokenPositionEvent broadcasts a token position update via WebSocket.
func (a *mapEventPublisherAdapter) PublishTokenPositionEvent(campaignID, tokenID string, x, y float64) {
	if campaignID == "" {
		return
	}
	a.bus.Publish(ws.NewMessage(ws.MsgTokenMoved, campaignID, tokenID, map[string]float64{
		"x": x,
		"y": y,
	}))
}

// PublishLayerEvent broadcasts a map layer update via WebSocket.
func (a *mapEventPublisherAdapter) PublishLayerEvent(eventType string, campaignID string, layer *maps.Layer) {
	if campaignID == "" {
		return
	}
	a.bus.Publish(ws.NewMessage(ws.MsgLayerUpdated, campaignID, layer.ID, layer))
}

// PublishFogEvent broadcasts a fog-of-war update via WebSocket.
func (a *mapEventPublisherAdapter) PublishFogEvent(eventType string, campaignID, mapID string, region *maps.FogRegion) {
	if campaignID == "" {
		return
	}
	payload := map[string]any{
		"event":  eventType,
		"map_id": mapID,
	}
	if region != nil {
		payload["region"] = region
	}
	a.bus.Publish(ws.NewMessage(ws.MsgFogUpdated, campaignID, mapID, payload))
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

// sessionListerAdapter wraps sessions.SessionService to implement the
// calendar.SessionLister interface for displaying sessions on the calendar grid.
type sessionListerAdapter struct {
	svc sessions.SessionService
}

// ListSessionsForDateRange returns sessions as lightweight CalendarSession structs.
func (a *sessionListerAdapter) ListSessionsForDateRange(ctx context.Context, campaignID, startDate, endDate string) ([]calendar.CalendarSession, error) {
	sess, err := a.svc.ListSessionsForDateRange(ctx, campaignID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	return sessionsToCalendarSessions(sess, ""), nil
}

// ListAllSessions returns all planned sessions for the calendar sessions modal.
func (a *sessionListerAdapter) ListAllSessions(ctx context.Context, campaignID, userID string) ([]calendar.CalendarSession, error) {
	sess, err := a.svc.ListPlannedSessions(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return sessionsToCalendarSessions(sess, userID), nil
}

// sessionsToCalendarSessions converts session models to calendar display structs.
func sessionsToCalendarSessions(sess []sessions.Session, userID string) []calendar.CalendarSession {
	result := make([]calendar.CalendarSession, 0, len(sess))
	for _, s := range sess {
		cs := calendar.CalendarSession{
			ID:              s.ID,
			Name:            s.Name,
			Status:          s.Status,
			IsRecurring:     s.IsRecurring,
			RecurrenceLabel: s.RecurrenceLabel(),
		}
		if s.ScheduledDate != nil {
			cs.ScheduledDate = *s.ScheduledDate
		}
		for _, att := range s.Attendees {
			cs.TotalCount++
			if att.Status == "accepted" {
				cs.AcceptedCount++
			}
			// Track current user's RSVP status.
			if userID != "" && att.UserID == userID {
				cs.UserRSVP = att.Status
			}
		}
		result = append(result, cs)
	}
	return result
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

// TogglePrivate flips an entity's is_private flag.
func (a *npcVisibilityTogglerAdapter) TogglePrivate(ctx context.Context, entityID string) (bool, error) {
	return a.svc.TogglePrivate(ctx, entityID)
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

	// Wire SMTP into auth service for password reset emails.
	auth.ConfigureMailSender(authService, smtpService, a.Config.BaseURL)

	// Entities plugin: entity types + entity CRUD (must be created before
	// campaigns so we can pass EntityService as the EntityTypeSeeder).
	entityTypeRepo := entities.NewEntityTypeRepository(a.DB)
	entityRepo := entities.NewEntityRepository(a.DB)
	entityPermRepo := entities.NewEntityPermissionRepository(a.DB)
	entityService := entities.NewEntityService(entityRepo, entityTypeRepo, entityPermRepo)

	// Campaigns plugin: CRUD, membership, ownership transfer.
	// EntityService is passed as EntityTypeSeeder to seed defaults on campaign creation.
	userFinder := campaigns.NewUserFinderAdapter(authRepo)
	campaignRepo := campaigns.NewCampaignRepository(a.DB)
	campaignService := campaigns.NewCampaignService(campaignRepo, userFinder, smtpService, entityService, a.Config.BaseURL)
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

	// Initialize HMAC URL signer for secure media access.
	// Auto-generate a signing secret on first boot if not configured.
	signingSecret := a.Config.Upload.SigningSecret
	if signingSecret == "" {
		generated, err := media.GenerateSigningSecret()
		if err != nil {
			slog.Error("failed to generate media signing secret", slog.Any("error", err))
		} else {
			signingSecret = generated
			slog.Warn("MEDIA_SIGNING_SECRET not set, using auto-generated secret (will change on restart)")
		}
	}
	var urlSigner *media.URLSigner
	if signingSecret != "" {
		urlSigner = media.NewURLSigner(signingSecret)
		mediaHandler.SetURLSigner(urlSigner)
	}

	// Wire campaign membership checker for private media access control.
	mediaHandler.SetMemberChecker(&mediaMemberCheckerAdapter{svc: campaignService})

	media.RegisterRoutes(e, mediaHandler, authService, a.Config.Upload.MaxSize, a.Config.Upload.ServeRateLimit)
	// Campaign media routes registered after addon service init (needs media-gallery addon gating).

	// Admin plugin: site-wide management (users, campaigns, SMTP settings, storage).
	adminHandler := admin.NewHandler(authRepo, campaignService, smtpService)
	adminHandler.SetMediaDeps(mediaRepo, mediaService, a.Config.Upload.MaxSize)
	adminHandler.SetBaseURL(a.Config.BaseURL)
	adminGroup := admin.RegisterRoutes(e, adminHandler, authService, smtpHandler)

	// Settings plugin: editable storage limits (global, per-user, per-campaign).
	// Registers on the admin group since all settings routes require site admin.
	settingsRepo := settings.NewSettingsRepository(a.DB)
	settingsService := settings.NewSettingsService(settingsRepo)
	settingsHandler := settings.NewHandler(settingsService)
	settings.RegisterRoutes(adminGroup, settingsHandler)

	// Wire settings service into admin handler for the combined storage page.
	adminHandler.SetSettingsDeps(settingsService)

	// Wire dynamic storage limits into the media service so uploads
	// respect per-user and per-campaign quotas from site settings.
	mediaService.SetStorageLimiter(&storageLimiterAdapter{svc: settingsService})

	// Addons plugin: extension framework with per-campaign enable/disable toggles.
	addonRepo := addons.NewAddonRepository(a.DB)
	addonService := addons.NewAddonService(addonRepo)
	// Auto-register discovered game systems as addons so new systems
	// (from internal/systems/, package manager, or GitHub) appear in the
	// addon UI without hardcoded definitions.
	for _, info := range systems.AddonInfos() {
		addons.RegisterSystemAddon(info.Slug, info.Name, info.Description, info.Version, info.Icon, info.Author)
	}
	// Seed all built-in addons (plugins, widgets, integrations + auto-registered
	// systems) into the database on startup.
	if err := addonService.SeedInstalledAddons(context.Background()); err != nil {
		slog.Error("failed to seed built-in addons", slog.String("error", err.Error()))
	}
	addonService.SetPresetApplier(newPresetApplier(entityService))
	addonHandler := addons.NewHandler(addonService)
	addons.RegisterAdminRoutes(adminGroup, addonHandler)
	addons.RegisterCampaignRoutes(e, addonHandler, campaignService, authService)

	// Campaign media browser routes (gated behind media-gallery addon).
	media.RegisterCampaignRoutes(e, mediaHandler, campaignService, authService, addonService)

	// Wire addon count into admin dashboard for the Extensions stat card.
	adminHandler.SetAddonCounter(addonService)

	// Wire addon checker into entity handler for conditional attributes rendering.
	entityHandler.SetAddonChecker(addonService)

	// Content extensions: user-installable content packs (calendar presets,
	// entity type templates, entity packs, tag collections, marker icons, themes).
	extRepo := extensions.NewExtensionRepository(a.DB)
	extService := extensions.NewExtensionService(extRepo, a.Config.ExtensionsPath)
	extService.SetMigrationRunner(extensions.NewMigrationRunner(a.DB))
	extHandler := extensions.NewHandler(extService, a.Config.ExtensionsPath)
	extensions.RegisterAdminRoutes(adminGroup, extHandler)
	extensions.RegisterCampaignRoutes(e, extHandler, campaignService, authService)
	extensions.RegisterAssetRoutes(e, extHandler)

	// Package manager: external repo management for systems and Foundry module.
	pkgRepo := packages.NewPackageRepository(a.DB)
	pkgGitHub := packages.NewGitHubClient()
	pkgService := packages.NewPackageService(pkgRepo, pkgGitHub, a.Config.Upload.MediaPath)
	packages.ConfigureSettings(pkgService, settingsRepo)
	pkgHandler := packages.NewHandler(pkgService)
	pkgOwnerHandler := packages.NewOwnerHandler(pkgService)
	if a.PluginHealth.IsHealthy("packages") {
		packages.RegisterRoutes(adminGroup, pkgHandler)

		// Owner-facing submission routes (authenticated, not admin-only).
		ownerGroup := e.Group("", auth.RequireAuth(authService))
		packages.RegisterOwnerRoutes(ownerGroup, pkgOwnerHandler)

		// Seed official Chronicle repos if no packages exist yet.
		pkgService.SeedOfficialPackages(context.Background())

		// Load systems from package manager install paths so externally
		// managed system packs override the bundled fallbacks.
		a.loadSystemsFromPackages(pkgService)

		// Store package service for Foundry module path resolution.
		a.pkgService = pkgService

		// Wire foundry dir resolver into admin handler so the Foundry page
		// reads/writes the package-manager-installed module, not the bundled fallback.
		adminHandler.SetFoundryDir(a.foundryModuleDir)

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

	// Wire security event logging into the auth handler so logins, logouts,
	// failed attempts, and password resets are recorded automatically.
	authHandler.SetSecurityLogger(securityService)

	// Wire security event logging into the media handler so uploads, deletes,
	// and quota failures are recorded in the admin security dashboard.
	mediaHandler.SetSecurityLogger(securityService)

	// Sync API plugin: external tool integration with API key auth,
	// request logging, security monitoring, and admin dashboard.
	syncRepo := syncapi.NewSyncAPIRepository(a.DB)
	syncService := syncapi.NewSyncAPIService(syncRepo)
	syncHandler := syncapi.NewHandler(syncService)
	// Inject sync mapping service early so the owner dashboard can show sync status.
	syncMappingRepoEarly := syncapi.NewSyncMappingRepository(a.DB)
	syncMappingSvcEarly := syncapi.NewSyncMappingService(syncMappingRepoEarly)
	syncHandler.SetSyncMappingService(syncMappingSvcEarly)
	syncHandler.SetCORSOriginLister(settingsService)
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
	if a.PluginHealth.IsHealthy("calendar") {
		calendar.RegisterRoutes(e, calendarHandler, campaignService, authService, addonService)
	} else {
		slog.Warn("calendar plugin degraded — routes not registered")
	}

	// Maps plugin: interactive maps with Leaflet.js, pin markers, entity linking.
	// Services created unconditionally (sync API references drawingService).
	mapsRepo := maps.NewMapRepository(a.DB)
	mapsService := maps.NewMapService(mapsRepo)
	mapsHandler := maps.NewHandler(mapsService)
	drawingRepo := maps.NewDrawingRepository(a.DB)
	drawingService := maps.NewDrawingService(drawingRepo)
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
		// Wire sessions into calendar for grid display (real-life mode).
		calendarHandler.SetSessionLister(&sessionListerAdapter{svc: sessionsService})
	} else {
		slog.Warn("sessions plugin degraded — routes not registered")
	}

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
	relHandler := relations.NewHandler(relService)
	relHandler.SetEntityTypeLister(&entityTypeListerForGraphAdapter{svc: entityService})
	relations.RegisterRoutes(e, relHandler, campaignService, authService)

	// Posts widget: entity sub-notes with rich text, visibility, and reorder.
	postRepo := posts.NewPostRepository(a.DB)
	postService := posts.NewPostService(postRepo)
	postHandler := posts.NewHandler(postService)
	posts.RegisterRoutes(e, postHandler, campaignService, authService)

	// REST API v1: versioned endpoints for external clients (Foundry VTT, etc.).
	// Authenticates via API keys, not browser sessions.
	syncAPIHandler := syncapi.NewAPIHandler(syncService, entityService, campaignService, relService)
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

	if a.PluginHealth.IsHealthy("syncapi") {
		syncapi.RegisterAPIRoutes(e, syncAPIHandler, calendarAPIHandler, mediaAPIHandler, mapAPIHandler, noteAPIHandler, syncMappingHandler, syncService, addonService)
	}

	// Tags widget: campaign-scoped entity tagging (CRUD + entity associations).
	tagRepo := tags.NewTagRepository(a.DB)
	tagService := tags.NewTagService(tagRepo)
	tagHandler := tags.NewHandler(tagService)
	tags.RegisterRoutes(e, tagHandler, campaignService, authService)

	// NPC plugin: gallery/hub view for revealed character entities.
	npcRepo := npcs.NewNPCRepository(a.DB)
	npcSvc := npcs.NewNPCService(npcRepo, &npcEntityTypeFinderAdapter{svc: entityService})
	npcHandler := npcs.NewHandler(npcSvc)
	npcHandler.SetVisibilityToggler(&npcVisibilityTogglerAdapter{svc: entityService})
	npcs.RegisterRoutes(e, npcHandler, campaignService, authService, addonService)

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
	audit.RegisterRoutes(e, auditHandler, campaignService, authService)

	// Wire audit logging into mutation handlers so CRUD actions are recorded.
	entityHandler.SetAuditService(auditService)
	entityHandler.SetTagFetcher(&entityTagFetcherAdapter{svc: tagService})
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

	// Calendar plugin blocks (requires "calendar" addon).
	blockRegistry.Register(entities.BlockMeta{
		Type: "calendar", Label: "Calendar", Icon: "fa-calendar-days",
		Description: "Entity calendar events", Addon: "calendar",
	}, func(ctx entities.BlockRenderContext) templ.Component {
		return calendar.BlockCalendarEvents(ctx.CC, ctx.Entity.ID)
	})
	blockRegistry.Register(entities.BlockMeta{
		Type: "upcoming_events", Label: "Upcoming Events", Icon: "fa-calendar-check",
		Description: "Upcoming calendar events list", Addon: "calendar",
	}, func(ctx entities.BlockRenderContext) templ.Component {
		return calendar.BlockUpcomingEvents(ctx.CC, entities.BlockConfigLimit(ctx.Block.Config, "limit", 5))
	})

	// Timeline plugin blocks (requires "timeline" addon).
	blockRegistry.Register(entities.BlockMeta{
		Type: "timeline", Label: "Timeline", Icon: "fa-timeline",
		Description: "Timeline preview with events", Addon: "timeline",
	}, func(ctx entities.BlockRenderContext) templ.Component {
		return timeline.BlockTimeline(ctx.CC)
	})

	// Maps plugin blocks (requires "maps" addon).
	blockRegistry.Register(entities.BlockMeta{
		Type: "map_preview", Label: "Map", Icon: "fa-map",
		Description: "Embedded map viewer", Addon: "maps",
	}, func(ctx entities.BlockRenderContext) templ.Component {
		return maps.BlockMapPreview(ctx.CC, entities.BlockConfigString(ctx.Block.Config, "map_id"))
	})

	// NPC gallery block — embeds a compact NPC grid on entity pages/dashboards.
	blockRegistry.Register(entities.BlockMeta{
		Type: "npc_gallery", Label: "NPC Gallery", Icon: "fa-users",
		Description: "Grid of revealed NPCs", Addon: "npcs",
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

	campaignHandler.SetAuditLogger(&campaignAuditAdapter{svc: auditService})
	campaignHandler.SetAddonLister(&addonListerAdapter{svc: addonService})
	campaignHandler.SetMediaUploader(&backdropUploaderAdapter{svc: mediaService})
	tagHandler.SetAuditService(auditService)

	// --- Campaign Export/Import ---
	exportSvc := campaigns.NewExportImportService(campaignService)
	exportSvc.SetEntityExporter(&entityExportAdapter{entitySvc: entityService, tagSvc: tagService, relationSvc: relService})
	exportSvc.SetCalendarExporter(&calendarExportAdapter{svc: calendarService})
	exportSvc.SetTimelineExporter(&timelineExportAdapter{svc: timelineSvc})
	exportSvc.SetSessionExporter(&sessionExportAdapter{svc: sessionsService})
	exportSvc.SetMapExporter(&mapExportAdapter{mapSvc: mapsService, drawingSvc: drawingService})
	exportSvc.SetAddonExporter(&addonExportAdapter{svc: addonService})
	exportSvc.SetMediaExporter(&mediaExportAdapter{svc: mediaService})
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

	// --- Layout Data Injector ---
	// Registers the callback that copies auth/campaign data from Echo's
	// context into Go's context.Context so Templ templates can read it.
	// This runs inside middleware.Render() before every template render.
	middleware.LayoutInjector = func(c echo.Context, ctx context.Context) context.Context {
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
			if campaignSettings.BrandName != "" {
				ctx = layouts.SetBrandName(ctx, campaignSettings.BrandName)
			}
			if campaignSettings.BrandLogo != "" {
				ctx = layouts.SetBrandLogo(ctx, campaignSettings.BrandLogo)
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
						ID:         et.ID,
						Slug:       et.Slug,
						Name:       et.Name,
						NamePlural: et.NamePlural,
						Icon:       et.Icon,
						Color:      et.Color,
						SortOrder:  et.SortOrder,
					}
				}

				// Apply sidebar config ordering/hiding if configured.
				sidebarCfg := cc.Campaign.ParseSidebarConfig()

				if sidebarCfg.HasUnifiedItems() {
					// New unified sidebar model — build SidebarItemViews.
					// Entity types indexed by ID for quick lookup.
					typeMap := make(map[int]layouts.SidebarEntityType)
					for _, et := range sidebarTypes {
						typeMap[et.ID] = et
					}

					var sidebarItems []layouts.SidebarItemView
					for _, item := range sidebarCfg.Items {
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
								sidebarItems = append(sidebarItems, layouts.SidebarItemView{
									Type: "category", TypeID: et.ID,
									Label: et.NamePlural, Icon: et.Icon, Color: et.Color,
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
				} else {
					// Legacy sidebar model — use entity type order + custom sections/links.
					sidebarTypes = layouts.SortSidebarTypes(sidebarTypes, sidebarCfg.EntityTypeOrder, sidebarCfg.HiddenTypeIDs)
					ctx = layouts.SetEntityTypes(ctx, sidebarTypes)

					if len(sidebarCfg.CustomSections) > 0 {
						secs := make([]layouts.SidebarSection, len(sidebarCfg.CustomSections))
						for i, s := range sidebarCfg.CustomSections {
							secs[i] = layouts.SidebarSection{ID: s.ID, Label: s.Label, After: s.After}
						}
						ctx = layouts.SetCustomSections(ctx, secs)
					}
					if len(sidebarCfg.CustomLinks) > 0 {
						lnks := make([]layouts.SidebarLink, len(sidebarCfg.CustomLinks))
						for i, l := range sidebarCfg.CustomLinks {
							lnks[i] = layouts.SidebarLink{ID: l.ID, Label: l.Label, URL: l.URL, Icon: l.Icon, Section: l.Section}
						}
						ctx = layouts.SetCustomLinks(ctx, lnks)
					}
				}
			}

			// Entity counts per type for sidebar badges (use effectiveRole so
			// "view as player" mode hides private entity counts).
			// Pass user ID for permission-aware entity counts.
			layoutUserID := ""
			if session := auth.GetSession(c); session != nil {
				layoutUserID = session.UserID
			}
			if counts, err := entityService.CountByType(reqCtx, cc.Campaign.ID, effectiveRole, layoutUserID); err == nil {
				ctx = layouts.SetEntityCounts(ctx, counts)
			}

			// Enabled addons for conditional widget rendering.
			if campaignAddons, err := addonService.ListForCampaign(reqCtx, cc.Campaign.ID); err == nil {
				enabledSlugs := make(map[string]bool)
				for _, ca := range campaignAddons {
					if ca.Enabled {
						enabledSlugs[ca.AddonSlug] = true
					}
				}
				ctx = layouts.SetEnabledAddons(ctx, enabledSlugs)
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

		// CSRF token for forms.
		ctx = layouts.SetCSRFToken(ctx, middleware.GetCSRFToken(c))

		// Active path for nav highlighting.
		ctx = layouts.SetActivePath(ctx, c.Request().URL.Path)

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
	calendarService.SetEventPublisher(&calendarEventPublisherAdapter{bus: wsEventBus})
	noteSvc.SetEventPublisher(&noteEventPublisherAdapter{bus: wsEventBus})

	drawingService.SetEventPublisher(&mapEventPublisherAdapter{bus: wsEventBus})
	drawingService.SetMapLookup(func(ctx context.Context, mapID string) (string, error) {
		m, err := mapsService.GetMap(ctx, mapID)
		if err != nil {
			return "", err
		}
		return m.CampaignID, nil
	})

	// --- Module Routes ---
	// Game system reference pages and tooltip APIs.
	// ref := e.Group("/ref")
	// dnd5eModule.RegisterRoutes(ref)

	// --- API Routes ---
	// REST API v1 is registered above via syncapi.RegisterAPIRoutes().
	// Endpoints: /api/v1/campaigns/:id/{entity-types,entities,sync}
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
