package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
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
	layoutJSON, _ := json.Marshal(et.Layout)
	fieldsJSON, _ := json.Marshal(et.Fields)
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
			Slug:     ca.AddonSlug,
			Name:     ca.AddonName,
			Icon:     ca.AddonIcon,
			Category: string(ca.AddonCategory),
			Enabled:  ca.Enabled,
		}
	}
	return result, nil
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

// ListCalendars returns all calendars for a campaign as lightweight refs.
// Currently returns at most one (one-per-campaign constraint), but is
// forward-compatible with future multi-calendar support.
func (a *calendarListerAdapter) ListCalendars(ctx context.Context, campaignID string) ([]timeline.CalendarRef, error) {
	cal, err := a.svc.GetCalendar(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if cal == nil {
		return nil, nil
	}
	return []timeline.CalendarRef{
		{ID: cal.ID, Name: cal.Name},
	}, nil
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
		if role < 3 && ev.Visibility == "dm_only" {
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

// mapEventPublisherAdapter bridges the websocket.EventBus to the maps.MapEventPublisher
// interface, translating domain events into WebSocket messages.
type mapEventPublisherAdapter struct {
	bus ws.EventBus
}

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

func (a *mapEventPublisherAdapter) PublishTokenPositionEvent(campaignID, tokenID string, x, y float64) {
	if campaignID == "" {
		return
	}
	a.bus.Publish(ws.NewMessage(ws.MsgTokenMoved, campaignID, tokenID, map[string]float64{
		"x": x,
		"y": y,
	}))
}

func (a *mapEventPublisherAdapter) PublishLayerEvent(eventType string, campaignID string, layer *maps.Layer) {
	if campaignID == "" {
		return
	}
	a.bus.Publish(ws.NewMessage(ws.MsgLayerUpdated, campaignID, layer.ID, layer))
}

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
	authHandler := auth.NewHandler(authService)
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
	campaignHandler.SetEntityLister(&entityTypeListerAdapter{svc: entityService})
	campaignHandler.SetLayoutFetcher(&entityTypeLayoutFetcherAdapter{svc: entityService})
	campaignHandler.SetRecentEntityLister(&recentEntityListerAdapter{svc: entityService})
	groupRepo := campaigns.NewGroupRepository(a.DB)
	groupService := campaigns.NewGroupService(groupRepo)
	campaignHandler.SetGroupService(groupService)
	campaigns.RegisterRoutes(e, campaignHandler, campaignService, authService)

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
	entityHandler := entities.NewHandler(entityService)
	entities.RegisterRoutes(e, entityHandler, campaignService, authService)

	// Media plugin: file upload, storage, thumbnailing, serving.
	// Graceful degradation: if the media directory can't be created, log a warning
	// but don't crash -- the rest of the app keeps running.
	mediaRepo := media.NewMediaRepository(a.DB)
	mediaService := media.NewMediaService(mediaRepo, a.Config.Upload.MediaPath, a.Config.Upload.MaxSize)
	// Wire ClamAV virus scanner if configured.
	if scanner := media.NewClamAVScanner(a.Config.Upload.ClamAVAddress); scanner != nil {
		mediaService.SetVirusScanner(scanner)
		slog.Info("ClamAV virus scanning enabled", slog.String("address", a.Config.Upload.ClamAVAddress))
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
	media.RegisterCampaignRoutes(e, mediaHandler, campaignService, authService)

	// Admin plugin: site-wide management (users, campaigns, SMTP settings, storage).
	adminHandler := admin.NewHandler(authRepo, campaignService, smtpService)
	adminHandler.SetMediaDeps(mediaRepo, mediaService, a.Config.Upload.MaxSize)
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
	addonHandler := addons.NewHandler(addonService)
	addons.RegisterAdminRoutes(adminGroup, addonHandler)
	addons.RegisterCampaignRoutes(e, addonHandler, campaignService, authService)

	// Wire addon count into admin dashboard for the Extensions stat card.
	adminHandler.SetAddonCounter(addonService)

	// Wire addon checker into entity handler for conditional attributes rendering.
	entityHandler.SetAddonChecker(addonService)

	// Security admin: event logging, session management, user account actions.
	securityRepo := admin.NewSecurityEventRepository(a.DB)
	securityService := admin.NewSecurityService(securityRepo, authRepo, authService)
	adminHandler.SetSecurityService(securityService)

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
	syncapi.RegisterAdminRoutes(adminGroup, syncHandler)
	syncapi.RegisterCampaignRoutes(e, syncHandler, campaignService, authService)

	// Calendar plugin: custom fantasy calendar with months, moons, events.
	// Created early so the sync API can reference calendarService.
	calendarRepo := calendar.NewCalendarRepository(a.DB)
	calendarService := calendar.NewCalendarService(calendarRepo)
	calendarHandler := calendar.NewHandler(calendarService)
	calendarHandler.SetAddonService(addonService)
	calendar.RegisterRoutes(e, calendarHandler, campaignService, authService, addonService)

	// Maps plugin: interactive maps with Leaflet.js, pin markers, entity linking.
	mapsRepo := maps.NewMapRepository(a.DB)
	mapsService := maps.NewMapService(mapsRepo)
	mapsHandler := maps.NewHandler(mapsService)
	maps.RegisterRoutes(e, mapsHandler, campaignService, authService, addonService)

	// Map expansion: drawings, tokens, layers, fog of war for real-time map sync.
	drawingRepo := maps.NewDrawingRepository(a.DB)
	drawingService := maps.NewDrawingService(drawingRepo)
	drawingHandler := maps.NewDrawingHandler(mapsService, drawingService)
	maps.RegisterDrawingRoutes(e, drawingHandler, campaignService, authService)

	// Sessions plugin: game session scheduling, linked entities, RSVP tracking.
	// Entity campaign checker prevents cross-campaign entity linking (IDOR).
	// Sessions require the calendar addon (integrated into calendar UI).
	sessionsRepo := sessions.NewSessionRepository(a.DB)
	sessionsService := sessions.NewSessionService(sessionsRepo, &entityCampaignCheckerAdapter{svc: entityService})
	sessionsHandler := sessions.NewHandler(sessionsService)
	sessionsHandler.SetMemberLister(campaignService)
	sessionsHandler.SetMailSender(smtpService, a.Config.BaseURL)
	sessions.RegisterRoutes(e, sessionsHandler, campaignService, authService, addonService)

	// Wire sessions into calendar for grid display (real-life mode).
	calendarHandler.SetSessionLister(&sessionListerAdapter{svc: sessionsService})

	// Timeline plugin: interactive visual timelines with zoom levels and entity grouping.
	timelineRepo := timeline.NewTimelineRepository(a.DB)
	timelineSvc := timeline.NewTimelineService(timelineRepo, &calendarListerAdapter{svc: calendarService}, &calendarEventListerAdapter{svc: calendarService}, &calendarEraListerAdapter{svc: calendarService})
	timelineHandler := timeline.NewHandler(timelineSvc)
	timelineHandler.SetMemberLister(campaignService)
	timeline.RegisterRoutes(e, timelineHandler, campaignService, authService, addonService)

	// Relations widget: bi-directional entity linking. Created before REST API
	// so it can be injected into the API handler for shop inventory support.
	relRepo := relations.NewRelationRepository(a.DB)
	relService := relations.NewRelationService(relRepo)
	relHandler := relations.NewHandler(relService)
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

	// Sync mapping service and handler for Foundry VTT bidirectional sync.
	syncMappingRepo := syncapi.NewSyncMappingRepository(a.DB)
	syncMappingSvc := syncapi.NewSyncMappingService(syncMappingRepo)
	syncMappingHandler := syncapi.NewSyncHandler(syncMappingSvc)
	_ = syncMappingSvc // Service will also be used by map/entity handlers.
	mapAPIHandler := syncapi.NewMapAPIHandler(syncService, mapsService, drawingService, campaignService)

	syncapi.RegisterAPIRoutes(e, syncAPIHandler, calendarAPIHandler, mediaAPIHandler, mapAPIHandler, syncMappingHandler, syncService, addonService)

	// Tags widget: campaign-scoped entity tagging (CRUD + entity associations).
	tagRepo := tags.NewTagRepository(a.DB)
	tagService := tags.NewTagService(tagRepo)
	tagHandler := tags.NewHandler(tagService)
	tags.RegisterRoutes(e, tagHandler, campaignService, authService)

	// Notes widget: personal floating note-taking panel (Google Keep-style).
	noteRepo := notes.NewNoteRepository(a.DB)
	noteService := notes.NewNoteService(noteRepo)
	noteHandler := notes.NewHandler(noteService)
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
	entityHandler.SetMemberLister(campaignService)
	entityHandler.SetGroupLister(groupService)
	entityHandler.SetCache(a.Redis)
	campaignHandler.SetAuditLogger(&campaignAuditAdapter{svc: auditService})
	campaignHandler.SetAddonLister(&addonListerAdapter{svc: addonService})
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
		}

		// Campaign info from campaign middleware.
		if cc := campaigns.GetCampaignContext(c); cc != nil {
			ctx = layouts.SetCampaignID(ctx, cc.Campaign.ID)
			ctx = layouts.SetCampaignName(ctx, cc.Campaign.Name)

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
				sidebarTypes = layouts.SortSidebarTypes(sidebarTypes, sidebarCfg.EntityTypeOrder, sidebarCfg.HiddenTypeIDs)

				ctx = layouts.SetEntityTypes(ctx, sidebarTypes)

				// Pass custom nav sections and links for sidebar rendering.
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
	e.GET("/ws", ws.HandleUpgrade(wsHub, wsAuth, []string{a.Config.BaseURL}))

	// Wire EventBus into services for real-time event publishing.
	wsEventBus := ws.NewEventBus(wsHub)

	entityService.SetEventPublisher(&entityEventPublisherAdapter{bus: wsEventBus})
	calendarService.SetEventPublisher(&calendarEventPublisherAdapter{bus: wsEventBus})

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
