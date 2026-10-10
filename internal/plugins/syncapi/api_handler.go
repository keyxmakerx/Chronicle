package syncapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/systems"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// AddonLister provides campaign addon listing for the discovery endpoint.
// Implemented by the addons plugin's service.
type AddonLister interface {
	ListForCampaign(ctx context.Context, campaignID string) ([]AddonInfo, error)
}

// AddonInfo is the API-safe representation of a campaign addon.
// Defined here to avoid importing the addons package directly.
type AddonInfo struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Category  string `json:"category"`
	Enabled   bool   `json:"enabled"`
	Installed bool   `json:"installed"`
}

// APIHandler serves the versioned REST API for external tool integration.
// External clients (Foundry VTT, custom scripts) use these endpoints to
// read and write campaign data programmatically via API key authentication.
type APIHandler struct {
	syncSvc              SyncAPIService
	entitySvc            entities.EntityService
	campaignSvc          campaigns.CampaignService
	relationSvc          relations.RelationService
	addonChecker         AddonChecker
	addonLister          AddonLister
	systemEnabler        SystemEnabler
	campaignSystemLister CampaignSystemLister
	tagGrantLister       TagGrantLister
	shopRoomReader       ShopRoomReader
	shopRoomAddon        string
	shopBuyer            ShopBuyAPIService
	dmScreen             DMScreenProvider
	systemState          SystemStateReader
	history              *SyncHistoryHandler
	quests               *QuestAPIHandler
	signer               *media.URLSigner
}

// TagGrantLister resolves an entity's tag-derived visibility grants so the
// permissions API can expose them additively to the Foundry module.
// Satisfied by the same adapter the entities plugin uses for its glance.
type TagGrantLister interface {
	GetEntityTagGrants(ctx context.Context, campaignID, entityID string) ([]entities.EntityTagGrantInfo, error)
}

// NewAPIHandler creates a new API handler with the required service dependencies.
func NewAPIHandler(syncSvc SyncAPIService, entitySvc entities.EntityService, campaignSvc campaigns.CampaignService, relationSvc relations.RelationService) *APIHandler {
	return &APIHandler{
		syncSvc:     syncSvc,
		entitySvc:   entitySvc,
		campaignSvc: campaignSvc,
		relationSvc: relationSvc,
	}
}

// SetCampaignSystemLister sets the custom campaign system lister for including
// per-campaign custom systems in API responses.
func (h *APIHandler) SetCampaignSystemLister(csl CampaignSystemLister) {
	h.campaignSystemLister = csl
}

// keyOwnerDegradedHeader is set on responses when a stored Bearer key's creator
// has lost Owner access to the key's campaign. The Foundry module can surface it
// as a "this sync key's owner lost access — rotate it" banner. The key keeps
// syncing; the header makes the otherwise-silent condition observable.
const keyOwnerDegradedHeader = "X-Chronicle-Key-Owner-Degraded"

// degradeSignalInterval throttles the heavyweight degrade signals (log line +
// persisted security event) per key so a high-frequency sync can't flood them,
// while still re-surfacing the condition periodically.
const degradeSignalInterval = time.Hour

// degradeSignalThrottle tracks the last time the loud degrade signal fired per
// API key id (int -> time.Time). The response header is always set; only the
// log/security-event emit is throttled.
var degradeSignalThrottle sync.Map

// resolveRole returns the caller's effective role for entity privacy filtering.
//
// The resolution differs by auth type:
//
//   - Session-authed callers (synthetic key, ID == synthKeySessionID) keep their
//     LIVE campaign role.
//   - A real stored Bearer key resolves to Owner-level sync visibility: keys
//     are strictly Owner-minted (routes.go: POST /api-keys is
//     RequireRole(Owner)) and the WebSocket path grants the same
//     (service.AuthenticateKeyForWS), so REST and WS agree.
//     RequireKeyOwnerStillOwner (middleware.go) already refused the request
//     before it reached here once the key's creator is no longer an Owner,
//     so a Bearer key that reaches this point is confirmed live-Owner; the
//     loud signal below is a defense-in-depth breadcrumb for any caller
//     that chain doesn't cover, not the primary gate.
func (h *APIHandler) resolveRole(c echo.Context) int {
	key := GetAPIKey(c)
	if key == nil {
		return 0
	}
	// Session-authed caller: keep today's live-membership behavior.
	if key.ID == synthKeySessionID {
		member, err := h.campaignSvc.GetMember(c.Request().Context(), key.CampaignID, key.UserID)
		if err != nil {
			return 0
		}
		return int(member.Role)
	}
	// Stored Bearer key: reliable Owner-level sync visibility, but surface a
	// lost-access condition loudly rather than silently degrading.
	h.flagIfKeyOwnerLostAccess(c, key)
	return int(campaigns.RoleOwner)
}

// flagIfKeyOwnerLostAccess emits a loud, module-surfaceable signal when a stored
// Bearer key's creator (key.UserID) is no longer an Owner (or no longer a member)
// of the key's campaign. RequireKeyOwnerStillOwner already refuses the request
// on every route that mounts it, so this rarely fires in production; it stays
// as a defense-in-depth breadcrumb for any resolveRole caller outside that
// middleware chain, so the condition still surfaces instead of looking silently
// normal.
func (h *APIHandler) flagIfKeyOwnerLostAccess(c echo.Context, key *APIKey) {
	member, err := h.campaignSvc.GetMember(c.Request().Context(), key.CampaignID, key.UserID)
	// err != nil => creator removed from campaign; role < Owner => demoted.
	if err == nil && member.Role >= campaigns.RoleOwner {
		return
	}

	// Always expose the condition to the client (cheap + idempotent) so the
	// module can show its banner regardless of log/security-event throttling.
	c.Response().Header().Set(keyOwnerDegradedHeader, "1")

	if !shouldEmitDegradeSignal(key.ID) {
		return
	}
	reason := "owner_demoted"
	if err != nil {
		reason = "owner_removed"
	}
	slog.Warn("sync api key owner lost campaign access; key still syncing — rotate it",
		slog.Int("key_id", key.ID),
		slog.String("campaign_id", key.CampaignID),
		slog.String("key_user_id", key.UserID),
		slog.String("reason", reason),
	)
	keyID := key.ID
	campaignID := key.CampaignID
	_ = h.syncSvc.LogSecurityEvent(c.Request().Context(), &SecurityEvent{
		EventType:  EventKeyOwnerDegraded,
		APIKeyID:   &keyID,
		CampaignID: &campaignID,
		IPAddress:  c.RealIP(),
		UserAgent:  strPtr(c.Request().UserAgent()),
		Details:    map[string]any{"reason": reason, "key_user_id": key.UserID},
	})
}

// shouldEmitDegradeSignal reports whether the loud degrade signal should fire for
// this key now, throttled to once per degradeSignalInterval. A small race (two
// concurrent first requests) is harmless for a warning.
func shouldEmitDegradeSignal(keyID int) bool {
	now := time.Now()
	if last, ok := degradeSignalThrottle.Load(keyID); ok {
		if t, ok := last.(time.Time); ok && now.Sub(t) < degradeSignalInterval {
			return false
		}
	}
	degradeSignalThrottle.Store(keyID, now)
	return true
}

// resolveUserID returns the API key owner's user ID for permission checks.
func (h *APIHandler) resolveUserID(c echo.Context) string {
	key := GetAPIKey(c)
	if key == nil {
		return ""
	}
	return key.UserID
}

// visibilityRoleFor is the API-key path's equivalent of
// campaigns.CampaignContext.VisibilityRole(): it promotes role to RoleOwner
// when the caller has been DM-granted dm_only visibility in this campaign
// (ADR-057), so a Co-DM's CheckEntityAccess call resolves like the web Show
// handler's. Callers already Owner (every stored Bearer key) skip the extra
// lookup. Feed the RESULT into CheckEntityAccess only — leave the caller's
// own `role` alone for GM-field and secret-stripping, which stay on the
// real member role.
func (h *APIHandler) visibilityRoleFor(ctx context.Context, campaignID, userID string, role int) int {
	if role >= int(campaigns.RoleOwner) {
		return role
	}
	granted, err := h.campaignSvc.IsUserDmGranted(ctx, campaignID, userID)
	if err != nil || !granted {
		return role
	}
	return int(campaigns.RoleOwner)
}

// --- Campaign Info ---

// apiCampaignResponse is the API-safe representation of a campaign.
// Omits internal fields like Settings and SidebarConfig.
type apiCampaignResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	IsPublic    bool      `json:"is_public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GetCampaign returns campaign details for the API key's campaign.
// GET /api/v1/campaigns/:id
func (h *APIHandler) GetCampaign(c echo.Context) error {
	campaignID := c.Param("id")
	campaign, err := h.campaignSvc.GetByID(c.Request().Context(), campaignID)
	if err != nil {
		return apperror.NewNotFound("campaign not found")
	}
	return c.JSON(http.StatusOK, apiCampaignResponse{
		ID:          campaign.ID,
		Name:        campaign.Name,
		Slug:        campaign.Slug,
		Description: campaign.Description,
		IsPublic:    campaign.IsPublic,
		CreatedAt:   campaign.CreatedAt,
		UpdatedAt:   campaign.UpdatedAt,
	})
}

// ListMembers returns all members of the campaign.
// GET /api/v1/campaigns/:id/members
func (h *APIHandler) ListMembers(c echo.Context) error {
	campaignID := c.Param("id")
	members, err := h.campaignSvc.ListMembers(c.Request().Context(), campaignID)
	if err != nil {
		slog.Error("api: list members failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list members"))
	}
	out := make([]apiMemberResponse, 0, len(members))
	for i := range members {
		out = append(out, h.toAPIMember(campaignID, &members[i]))
	}
	return c.JSON(http.StatusOK, out)
}

// SetURLSigner enables signed avatar links in the members response. Without
// it members carry no avatar_url.
func (h *APIHandler) SetURLSigner(signer *media.URLSigner) {
	h.signer = signer
}

// avatarThumbSize is the thumbnail a member's picture is offered at: plenty
// for a Foundry user avatar and the size the web top bar already uses.
const avatarThumbSize = "300"

// apiMemberResponse is the members wire shape. It is its own struct, not the
// campaigns model, so a field added to CampaignMember (such as the account
// email) never reaches an API key's holder without a decision here.
type apiMemberResponse struct {
	CampaignID        string         `json:"campaign_id"`
	UserID            string         `json:"user_id"`
	Role              campaigns.Role `json:"role"`
	CharacterEntityID *string        `json:"character_entity_id,omitempty"`
	JoinedAt          time.Time      `json:"joined_at"`
	DisplayName       string         `json:"display_name,omitempty"`
	CharacterName     *string        `json:"character_name,omitempty"`
	// AvatarURL is a short-lived signed thumbnail link, absent when the member
	// has no picture or no signer is configured.
	AvatarURL string `json:"avatar_url,omitempty"`
}

// toAPIMember maps a member to the wire shape, signing the picture link for
// the requesting key's campaign: an avatar has no campaign of its own, so the
// media route admits the link only while its owner is a member of this one.
func (h *APIHandler) toAPIMember(campaignID string, m *campaigns.CampaignMember) apiMemberResponse {
	resp := apiMemberResponse{
		CampaignID:        m.CampaignID,
		UserID:            m.UserID,
		Role:              m.Role,
		CharacterEntityID: m.CharacterEntityID,
		JoinedAt:          m.JoinedAt,
		DisplayName:       m.DisplayName,
		CharacterName:     m.CharacterName,
	}
	// Only a bare media id is signed; a legacy path value would not name a
	// file, and a bad value must not become a link.
	if h.signer != nil && m.AvatarPath != nil {
		if id, err := uuid.Parse(*m.AvatarPath); err == nil {
			resp.AvatarURL = h.signer.SignAvatarThumb(id.String(), avatarThumbSize, campaignID, media.SignedURLTTL)
		}
	}
	return resp
}

// --- Entity Types ---

// ListEntityTypes returns all entity types for the campaign.
// GET /api/v1/campaigns/:id/entity-types
func (h *APIHandler) ListEntityTypes(c echo.Context) error {
	campaignID := c.Param("id")
	types, err := h.entitySvc.GetEntityTypes(c.Request().Context(), campaignID)
	if err != nil {
		slog.Error("api: failed to list entity types", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list entity types"))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"data":  types,
		"total": len(types),
	})
}

// GetEntityType returns a single entity type by ID.
// GET /api/v1/campaigns/:id/entity-types/:typeID
func (h *APIHandler) GetEntityType(c echo.Context) error {
	typeID, err := strconv.Atoi(c.Param("typeID"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	et, err := h.entitySvc.GetEntityTypeByID(c.Request().Context(), typeID)
	if err != nil {
		return apperror.NewNotFound("entity type not found")
	}

	// Verify it belongs to the API key's campaign.
	if et.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity type not found")
	}

	return c.JSON(http.StatusOK, et)
}

// --- Entity Read ---

// ListEntities returns entities with pagination and optional filters.
// GET /api/v1/campaigns/:id/entities?type_id=N&page=1&per_page=20&q=search
func (h *APIHandler) ListEntities(c echo.Context) error {
	campaignID := c.Param("id")
	role := h.resolveRole(c)

	typeID, _ := strconv.Atoi(c.QueryParam("type_id"))
	page, _ := strconv.Atoi(c.QueryParam("page"))
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	query := c.QueryParam("q")

	opts := entities.ListOptions{Page: page, PerPage: perPage}
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 20
	}

	var (
		items []entities.Entity
		total int
		err   error
	)

	userID := h.resolveUserID(c)
	if query != "" {
		items, total, err = h.entitySvc.Search(c.Request().Context(), campaignID, query, typeID, role, userID, opts)
	} else {
		items, total, err = h.entitySvc.List(c.Request().Context(), campaignID, typeID, role, userID, opts)
	}
	if err != nil {
		slog.Error("api: failed to list entities", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list entities"))
	}

	if items == nil {
		items = []entities.Entity{}
	}

	// Defense-in-depth egress sanitize — see egress_sanitize.go.
	sanitizeEntitiesHTMLForEgress(items)

	// Redact inline GM secrets for callers below the secret-visibility
	// bar (Player), mirroring the web path and GetEntity's same check at
	// list scope. Owner/Scribe responses are unchanged.
	stripEntitiesSecretsForEgress(items, role)

	// Strip GM-only and owner-only field VALUES at list scope. Load the
	// campaign's entity types ONCE (not per-entity) to resolve the markers.
	// Fail closed on a load error rather than leak.
	if role < int(campaigns.RoleScribe) && len(items) > 0 {
		types, terr := h.entitySvc.GetEntityTypes(c.Request().Context(), campaignID)
		if terr != nil {
			slog.Error("api: field strip could not load entity types", slog.Any("error", terr))
			return apperror.NewInternal(fmt.Errorf("failed to list entities"))
		}
		fieldsByType := make(map[int][]entities.FieldDefinition, len(types))
		for i := range types {
			fieldsByType[types[i].ID] = types[i].Fields
		}
		stripEntitiesFieldsForEgress(items, role, userID, func(typeID int) []entities.FieldDefinition {
			return fieldsByType[typeID]
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":     items,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}

// GetEntity returns a single entity by ID.
// GET /api/v1/campaigns/:id/entities/:entityID
func (h *APIHandler) GetEntity(c echo.Context) error {
	entityID := c.Param("entityID")
	role := h.resolveRole(c)
	ctx := c.Request().Context()

	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}

	// Verify the entity belongs to the API key's campaign.
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	// Enforce visibility: check both legacy is_private and custom permissions.
	// CheckEntityAccess gets a promoted role when the caller is DM-granted
	// (ADR-057), mirroring campaigns.CampaignContext.VisibilityRole() on the
	// web path so a Co-DM can read a dm_only entity here too. This handler is
	// API-key authenticated (no CampaignContext), so the promotion is resolved
	// directly via campaignSvc.IsUserDmGranted. `role` stays unpromoted for
	// everything below this call (GM-field / secret stripping).
	userID := h.resolveUserID(c)
	access, accessErr := h.entitySvc.CheckEntityAccess(ctx, entity.ID, h.visibilityRoleFor(ctx, entity.CampaignID, userID, role), userID)
	if accessErr != nil || !access.CanView {
		return apperror.NewNotFound("entity not found")
	}

	// Defense-in-depth egress sanitize — see egress_sanitize.go.
	sanitizeEntityHTMLForEgress(entity)

	// Redact inline GM secrets for callers below the secret-visibility bar
	// (Player), mirroring the web GetEntry path. The whole-entity CanView
	// gate above lets a player read this entity, but the secret PROSE inside
	// it must not ship. Owner/Scribe (>= RoleScribe) responses are unchanged.
	stripEntitySecretsForEgress(entity, role)

	// Strip GM-only and owner-only field VALUES from fields_data for callers
	// who can't see them. The CanView gate above lets a player read this
	// entity, but gm_only fields (e.g. Draw Steel's gm_notes) and owner_only
	// fields (e.g. backstory, for a non-owner viewer) must not ship.
	// Bearer/Owner/Scribe keep full data. Fail closed: if the type can't
	// load we cannot know which fields are restricted, so error rather than
	// leak (mirrors the entities-plugin GetFieldsAPI).
	if role < int(campaigns.RoleScribe) {
		et, terr := h.entitySvc.GetEntityTypeByID(ctx, entity.EntityTypeID)
		if terr != nil || et == nil {
			slog.Error("api: field strip could not load entity type",
				slog.Int("entity_type_id", entity.EntityTypeID), slog.Any("error", terr))
			return apperror.NewInternal(fmt.Errorf("failed to load entity"))
		}
		entity.FieldsData = entities.FilterRestrictedFields(entity.FieldsData, et.Fields, false, entity.IsOwnedBy(userID))
	}

	return c.JSON(http.StatusOK, entity)
}

// --- Entity Write ---

// entityPrivacyResolver decides the is_private flag for entities created
// through this API, merging what the client asked for over the campaign's
// DefaultVisibility via the shared campaigns.CampaignSettings.
// ResolveNewEntityPrivacy. Two rules specific to this surface:
//
//   - The settings read is LAZY and AT MOST ONCE per request: a sync batch
//     can carry up to 2000 changes, and re-reading per entity would be 2000
//     queries for a value that can't change mid-request.
//   - An unreadable campaign FAILS CLOSED to private. The setting is the
//     only thing standing between a DM-only campaign and a publicly visible
//     entity, and publishing a hidden entity can't be taken back.
type entityPrivacyResolver struct {
	h          *APIHandler
	campaignID string

	loaded     bool                       // settings read attempted
	settings   campaigns.CampaignSettings // valid only when !unreadable
	unreadable bool                       // read failed; fail closed
}

// newEntityPrivacyResolver returns a resolver scoped to one request. It does
// not touch the database; the read happens on the first absent is_private.
func (h *APIHandler) newEntityPrivacyResolver(campaignID string) *entityPrivacyResolver {
	return &entityPrivacyResolver{h: h, campaignID: campaignID}
}

// resolve merges one create request's is_private over the campaign default.
func (r *entityPrivacyResolver) resolve(ctx context.Context, requested patch.Field[bool]) bool {
	// An explicit value — true or false — is the client's decision and wins
	// outright. It also means we never need the campaign settings, so a
	// caller that always states its intent never pays for the read.
	if v, ok := requested.Get(); ok {
		return v
	}

	if !r.loaded {
		r.loaded = true
		if r.h == nil || r.h.campaignSvc == nil {
			// A wiring bug, not a client error. Fail closed and say so.
			slog.Error("api: no campaign service wired; new entity defaults to private",
				slog.String("campaign_id", r.campaignID))
			r.unreadable = true
		} else if campaign, err := r.h.campaignSvc.GetByID(ctx, r.campaignID); err != nil || campaign == nil {
			slog.Error("api: could not read campaign default visibility; new entity defaults to private",
				slog.String("campaign_id", r.campaignID), slog.Any("error", err))
			r.unreadable = true
		} else {
			r.settings = campaign.ParseSettings()
		}
	}
	if r.unreadable {
		return true
	}
	return r.settings.ResolveNewEntityPrivacy(requested)
}

// apiCreateEntityRequest is the JSON body for creating an entity via the API.
//
// IsPrivate is three-state: there is nothing stored to preserve on create, so
// absent means "let the campaign's DefaultVisibility decide" rather than
// "public". A plain bool would collapse absent to false and create entities
// public regardless of a DM-Only campaign default.
type apiCreateEntityRequest struct {
	Name         string            `json:"name"`
	EntityTypeID int               `json:"entity_type_id"`
	TypeLabel    string            `json:"type_label"`
	IsPrivate    patch.Field[bool] `json:"is_private"`
	FieldsData   map[string]any    `json:"fields_data"`
	// OwnerUserID claims the entity for a player at create time.
	// Optional. The server validates the user is a member of the
	// target campaign and rejects with 400 otherwise. Foundry sync
	// uses this to auto-claim character entities at creation; manual
	// API consumers can omit and let the player claim later via the UI.
	OwnerUserID *string `json:"owner_user_id,omitempty"`
}

// CreateEntity creates a new entity in the campaign.
// POST /api/v1/campaigns/:id/entities
func (h *APIHandler) CreateEntity(c echo.Context) error {
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	var req apiCreateEntityRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// Reject a missing/zero entity_type_id instead of silently defaulting to
	// the first available type: a client with a stale/wrong type mapping
	// would otherwise create entities under an arbitrary category with no
	// error. The batch-sync create path always passes a real type.
	if req.EntityTypeID == 0 {
		return apperror.NewBadRequest("entity_type_id is required")
	}

	// Validate owner_user_id (if provided) is a member of the target
	// campaign. Cross-campaign assignment would orphan the claim — a
	// user not in the campaign cannot see the entity, so the claim is
	// useless and likely a bug in the caller.
	if req.OwnerUserID != nil && *req.OwnerUserID != "" {
		member, err := h.campaignSvc.GetMember(c.Request().Context(), c.Param("id"), *req.OwnerUserID)
		if err != nil || member == nil {
			return apperror.NewBadRequest("owner_user_id is not a member of this campaign")
		}
	}

	// An absent is_private falls back to the campaign's DefaultVisibility;
	// an explicit value from the client wins. See entityPrivacyResolver.
	isPrivate := h.newEntityPrivacyResolver(c.Param("id")).
		resolve(c.Request().Context(), req.IsPrivate)

	entity, err := h.entitySvc.Create(c.Request().Context(), c.Param("id"), key.UserID, entities.CreateEntityInput{
		Name:         req.Name,
		EntityTypeID: req.EntityTypeID,
		TypeLabel:    req.TypeLabel,
		IsPrivate:    isPrivate,
		FieldsData:   req.FieldsData,
		OwnerUserID:  req.OwnerUserID,
	})
	if err != nil {
		return err
	}

	noteSyncResource(c, entity.ID, entity.Name)
	return c.JSON(http.StatusCreated, entity)
}

// apiUpdateEntityRequest is the JSON body for updating an entity via the API.
// This is a PARTIAL update: an ABSENT key preserves the stored value, an
// EXPLICIT null clears it, a present value replaces it. patch.Field is what
// makes absent and null different — a plain pointer collapses them. A
// value-typed is_private would default absent to false (public) and leak
// private entities on a narrow update such as a rename; keep it a
// patch.Field.
type apiUpdateEntityRequest struct {
	Name              patch.Field[string] `json:"name"`
	TypeLabel         patch.Field[string] `json:"type_label"`
	ParentID          patch.Field[string] `json:"parent_id"`
	IsPrivate         patch.Field[bool]   `json:"is_private"`
	Entry             patch.Field[string] `json:"entry"`
	PlayerNotes       *string             `json:"player_notes"`
	FieldsData        map[string]any      `json:"fields_data"`
	ExpectedUpdatedAt *time.Time          `json:"expected_updated_at"`
}

// UpdateEntity updates an existing entity.
// PUT /api/v1/campaigns/:id/entities/:entityID
func (h *APIHandler) UpdateEntity(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	// Verify entity belongs to this campaign.
	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	var req apiUpdateEntityRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// is_private: absent (and an explicit null, which a NOT NULL column has
	// no room for) yields a nil pointer, which the service reads as
	// "preserve"; a present true/false is written.
	updated, err := h.entitySvc.Update(entities.WithSyncActor(ctx, h.resolveUserID(c)), entityID, entities.UpdateEntityInput{
		Name:              req.Name,
		TypeLabel:         req.TypeLabel,
		ParentID:          req.ParentID,
		IsPrivate:         req.IsPrivate.Ptr(nil),
		Entry:             req.Entry,
		PlayerNotes:       req.PlayerNotes,
		FieldsData:        req.FieldsData,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	})
	if err != nil {
		return err
	}

	noteSyncResource(c, updated.ID, updated.Name)
	return c.JSON(http.StatusOK, updated)
}

// apiUpdateFieldsRequest is the JSON body for updating entity custom fields.
type apiUpdateFieldsRequest struct {
	FieldsData map[string]any `json:"fields_data"`
}

// UpdateEntityFields merges the sent custom fields into an entity's fields.
// PUT /api/v1/campaigns/:id/entities/:entityID/fields
func (h *APIHandler) UpdateEntityFields(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	// Verify entity belongs to this campaign.
	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	var req apiUpdateFieldsRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// Partial update, like every other sync API write: fields the client
	// doesn't send are kept, null clears one.
	if err := h.entitySvc.MergeFields(ctx, entityID, req.FieldsData); err != nil {
		return err
	}

	// Capture what was sent (read-only diagnostics ring buffer; no DB) so the
	// operator's sync.inbound diagnostic can show "what Foundry sent" for this
	// entity and compare it against the stored fields.
	recordInbound(entityID, "fields", req.FieldsData, time.Now())

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ToggleEntityReveal sets or toggles an entity's is_private flag via the REST API.
// POST /api/v1/campaigns/:id/entities/:entityID/reveal
// Used by Foundry VTT to sync NPC visibility changes bidirectionally.
// Body: {"is_private": true|false} — if omitted, toggles current state.
func (h *APIHandler) ToggleEntityReveal(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	// Verify entity belongs to this campaign.
	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	var req struct {
		IsPrivate *bool `json:"is_private"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// If explicit value matches current state, no-op.
	if req.IsPrivate != nil && *req.IsPrivate == entity.IsPrivate {
		return c.JSON(http.StatusOK, map[string]any{
			"entity_id":  entityID,
			"is_private": entity.IsPrivate,
		})
	}

	// Toggle (or set to desired value — same effect since we checked above).
	newPrivate, err := h.entitySvc.TogglePrivate(ctx, entityID)
	if err != nil {
		return err
	}

	slog.Info("entity visibility changed via API",
		slog.String("entity_id", entityID),
		slog.Bool("is_private", newPrivate),
	)

	return c.JSON(http.StatusOK, map[string]any{
		"entity_id":  entityID,
		"is_private": newPrivate,
	})
}

// DeleteEntity deletes an entity from the campaign.
// DELETE /api/v1/campaigns/:id/entities/:entityID
//
// Owner-gated to match the web route (entities/routes.go: DELETE
// /entities/:eid requires RoleOwner — "Owner can delete" per that file's
// header comment). RequirePermission(PermWrite) alone also admits a
// Scribe, whether via a Foundry key or the Scribe's own session cookie
// (this route group accepts both), which the web UI refuses.
func (h *APIHandler) DeleteEntity(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()
	role := h.resolveRole(c)

	// Verify entity belongs to this campaign.
	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	// Only campaign owners can delete entities.
	if role < int(campaigns.RoleOwner) {
		return apperror.NewForbidden("only campaign owners can delete entities")
	}

	if err := h.entitySvc.Delete(entities.WithSyncActor(ctx, h.resolveUserID(c)), entityID); err != nil {
		slog.Error("api: failed to delete entity", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to delete entity"))
	}
	noteSyncResource(c, entity.ID, entity.Name)

	return c.NoContent(http.StatusNoContent)
}

// --- Sync Endpoint ---

// syncMaxPullPages caps the number of internal pages ONE pull request walks,
// so a single request cannot hold a connection open across an unbounded table
// scan. This is a page size, not a ceiling: when the walk stops early the
// response carries a next_cursor that resumes it.
const syncMaxPullPages = 10

// syncPageSize is the per-page size used for internal pagination during sync.
const syncPageSize = 100

// syncRequest is the JSON body for the bulk sync endpoint.
type syncRequest struct {
	Since   *time.Time   `json:"since"`   // Pull entities modified after this time.
	Cursor  string       `json:"cursor"`  // Opaque resume token from a previous response's next_cursor. Empty starts at the beginning.
	Changes []syncChange `json:"changes"` // Batch of create/update/delete operations.
}

// syncChange describes a single mutation in a sync batch.
// The content fields are patch.Field for the same reason
// apiUpdateEntityRequest's are: on an "update" action this struct is a
// PARTIAL body, and absent must preserve rather than write. On a "create"
// action there is nothing to preserve, so the create branch reads each
// field with its zero default.
type syncChange struct {
	Action       string              `json:"action"`         // "create", "update", "delete".
	EntityID     string              `json:"entity_id"`      // Required for update/delete.
	EntityTypeID int                 `json:"entity_type_id"` // Required for create.
	Name         patch.Field[string] `json:"name"`
	TypeLabel    patch.Field[string] `json:"type_label"`
	ParentID     patch.Field[string] `json:"parent_id"`
	IsPrivate    patch.Field[bool]   `json:"is_private"`
	Entry        patch.Field[string] `json:"entry"`
	FieldsData   map[string]any      `json:"fields_data"`
}

// syncResult describes the outcome of a single sync operation.
type syncResult struct {
	Action   string `json:"action"`
	EntityID string `json:"entity_id"`
	Status   string `json:"status"` // "ok" or "error".
	Error    string `json:"error,omitempty"`
}

// syncResponse is the full response from the sync endpoint.
type syncResponse struct {
	ServerTime time.Time         `json:"server_time"`
	Entities   []entities.Entity `json:"entities"`
	HasMore    bool              `json:"has_more"`

	// NextCursor resumes the pull where this response stopped. Non-empty
	// exactly when HasMore is true. A client walks a pull by re-POSTing the
	// SAME since with this cursor until has_more is false, then keeps the
	// server_time from the FIRST response of that walk as the next since —
	// not the last, which would skip anything modified mid-walk.
	NextCursor string `json:"next_cursor,omitempty"`

	Results []syncResult `json:"results"`
}

// Sync performs a bidirectional sync operation.
// POST /api/v1/campaigns/:id/sync
//
// Pull: if "since" is provided, returns entities modified after that timestamp.
// Push: if "changes" is provided, applies the batch of create/update/delete operations.
//
// One request walks at most syncMaxPullPages*syncPageSize entities. When the
// campaign is larger than that the response sets has_more and next_cursor;
// the client re-POSTs the SAME since with that cursor until has_more is
// false. server_time from the FIRST response of a completed walk is the next
// since — taking it from the last response would skip anything modified while
// the walk was in flight.
func (h *APIHandler) Sync(c echo.Context) error {
	key := GetAPIKey(c)
	if key == nil {
		return apperror.NewUnauthorized("api key required")
	}

	campaignID := c.Param("id")
	ctx := c.Request().Context()
	role := h.resolveRole(c)

	var req syncRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// Reject oversized sync batches to prevent memory/CPU exhaustion.
	const maxSyncChanges = 2000
	if len(req.Changes) > maxSyncChanges {
		return apperror.NewBadRequest(
			fmt.Sprintf("too many changes; maximum is %d per request", maxSyncChanges))
	}

	serverTime := time.Now().UTC()

	// Pull: get entities modified since the given timestamp.
	var pulledEntities []entities.Entity
	hasMore := false
	nextCursor := ""

	if req.Since != nil {
		since := *req.Since
		syncUserID := h.resolveUserID(c)

		startPage, err := decodeSyncCursor(req.Cursor)
		if err != nil {
			return err
		}
		endPage := startPage + syncMaxPullPages - 1

		for page := startPage; page <= endPage; page++ {
			items, total, err := h.entitySvc.List(ctx, campaignID, 0, role, syncUserID, entities.ListOptions{
				Page:    page,
				PerPage: syncPageSize,
			})
			if err != nil {
				slog.Error("api: sync pull failed", slog.Any("error", err))
				return apperror.NewInternal(fmt.Errorf("failed to pull entities"))
			}

			for _, e := range items {
				if e.UpdatedAt.After(since) || e.CreatedAt.After(since) {
					pulledEntities = append(pulledEntities, e)
				}
			}

			// A short page means we ran off the end of the list even if
			// `total` says otherwise (rows deleted mid-walk), so stop.
			if len(items) < syncPageSize {
				break
			}
			// Everything the campaign has is now behind us.
			if page*syncPageSize >= total {
				break
			}
			// Out of budget for this request, but not out of entities:
			// hand the caller the token that resumes the walk. Without
			// this the remaining entities were unreachable forever.
			if page == endPage {
				hasMore = true
				nextCursor = encodeSyncCursor(page + 1)
			}
		}
	}

	if pulledEntities == nil {
		pulledEntities = []entities.Entity{}
	}

	// Push: apply batch changes.
	//
	// One resolver for the whole batch: the campaign's DefaultVisibility
	// cannot change mid-request, and it reads lazily, so a batch that
	// creates nothing (or that states is_private on every create) never
	// touches the campaigns table.
	privacy := h.newEntityPrivacyResolver(campaignID)

	var results []syncResult
	for _, change := range req.Changes {
		result := syncResult{Action: change.Action, EntityID: change.EntityID}

		switch change.Action {
		case "create":
			// Create has no stored value to preserve, so content fields
			// read with their zero default: absent is the same as empty.
			//
			// is_private is the exception: absent means "the client has no
			// opinion", and must fall through to the campaign's default
			// visibility rather than defaulting to false (public).
			entity, err := h.entitySvc.Create(ctx, campaignID, key.UserID, entities.CreateEntityInput{
				Name:         change.Name.Val(""),
				EntityTypeID: change.EntityTypeID,
				TypeLabel:    change.TypeLabel.Val(""),
				IsPrivate:    privacy.resolve(ctx, change.IsPrivate),
				FieldsData:   change.FieldsData,
			})
			if err != nil {
				result.Status = "error"
				result.Error = apperror.SafeMessage(err)
			} else {
				result.Status = "ok"
				result.EntityID = entity.ID
			}

		case "update":
			// Verify entity belongs to this campaign before updating.
			existing, err := h.entitySvc.GetByID(ctx, change.EntityID)
			if err != nil || existing.CampaignID != campaignID {
				result.Status = "error"
				result.Error = "entity not found"
			} else {
				// Batch sync is a PARTIAL update, same contract as the
				// single-entity PUT: absent preserves, a present value
				// writes. is_private absent yields a nil pointer, which the
				// service preserves.
				_, err := h.entitySvc.Update(entities.WithSyncActor(ctx, h.resolveUserID(c)), change.EntityID, entities.UpdateEntityInput{
					Name:       change.Name,
					TypeLabel:  change.TypeLabel,
					ParentID:   change.ParentID,
					IsPrivate:  change.IsPrivate.Ptr(nil),
					Entry:      change.Entry,
					FieldsData: change.FieldsData,
				})
				if err != nil {
					result.Status = "error"
					result.Error = apperror.SafeMessage(err)
				} else {
					result.Status = "ok"
				}
			}

		case "delete":
			// Verify entity belongs to this campaign before deleting.
			existing, err := h.entitySvc.GetByID(ctx, change.EntityID)
			if err != nil || existing.CampaignID != campaignID {
				result.Status = "error"
				result.Error = "entity not found"
			} else {
				if err := h.entitySvc.Delete(entities.WithSyncActor(ctx, h.resolveUserID(c)), change.EntityID); err != nil {
					result.Status = "error"
					result.Error = apperror.SafeMessage(err)
				} else {
					result.Status = "ok"
				}
			}

		default:
			result.Status = "error"
			result.Error = "unknown action; expected create, update, or delete"
		}

		results = append(results, result)
	}

	if results == nil {
		results = []syncResult{}
	}

	return c.JSON(http.StatusOK, syncResponse{
		ServerTime: serverTime,
		Entities:   pulledEntities,
		HasMore:    hasMore,
		NextCursor: nextCursor,
		Results:    results,
	})
}

// --- Entity Relations ---

// ListEntityRelations returns all relations for an entity, enriched with target
// entity display data and metadata (price, quantity for shop inventory).
// GET /api/v1/campaigns/:id/entities/:entityID/relations
func (h *APIHandler) ListEntityRelations(c echo.Context) error {
	entityID := c.Param("entityID")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID required")
	}

	rels, err := h.relationSvc.ListByEntity(c.Request().Context(), c.Param("id"), entityID)
	if err != nil {
		slog.Error("listing entity relations", slog.String("entity_id", entityID), slog.String("error", err.Error()))
		return apperror.NewInternal(fmt.Errorf("failed to list relations"))
	}

	if rels == nil {
		rels = []relations.Relation{}
	}

	return c.JSON(http.StatusOK, rels)
}

// --- Entity Permissions ---

// permissionsAPIResponse is the JSON response for entity permission queries.
type permissionsAPIResponse struct {
	Visibility  entities.VisibilityMode     `json:"visibility"`
	IsPrivate   bool                        `json:"is_private"`
	Permissions []entities.EntityPermission `json:"permissions"`
	// TagGrants is a separate array, not folded into Permissions, so
	// consumers reading only `permissions` keep working unchanged.
	// subject_type (here or in Permissions) may be "public" (reveal to
	// everyone); unrecognized subject types are ignored by old consumers.
	TagGrants []entities.EntityTagGrantInfo `json:"tag_grants"`
}

// GetEntityPermissions returns the visibility mode and permission grants for an entity.
// GET /api/v1/campaigns/:id/entities/:entityID/permissions
func (h *APIHandler) GetEntityPermissions(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}

	// Verify entity belongs to the API key's campaign.
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	grants, err := h.entitySvc.GetEntityPermissions(ctx, entityID)
	if err != nil {
		slog.Error("fetching entity permissions",
			slog.String("entity_id", entityID),
			slog.String("error", err.Error()))
		return apperror.NewInternal(fmt.Errorf("failed to fetch permissions"))
	}

	if grants == nil {
		grants = []entities.EntityPermission{}
	}

	// Tag-derived grants, exposed additively for the Foundry module's
	// ownership sync. Best-effort: a lookup failure must not break the
	// permissions read the module relies on.
	var tagGrants []entities.EntityTagGrantInfo
	if h.tagGrantLister != nil {
		tagGrants, err = h.tagGrantLister.GetEntityTagGrants(ctx, c.Param("id"), entityID)
		if err != nil {
			slog.Error("fetching entity tag grants",
				slog.String("entity_id", entityID), slog.String("error", err.Error()))
			tagGrants = nil
		}
	}
	if tagGrants == nil {
		tagGrants = []entities.EntityTagGrantInfo{}
	}

	return c.JSON(http.StatusOK, permissionsAPIResponse{
		Visibility:  entity.Visibility,
		IsPrivate:   entity.IsPrivate,
		Permissions: grants,
		TagGrants:   tagGrants,
	})
}

// SetEntityPermissions updates the visibility mode and permission grants for an entity.
// PUT /api/v1/campaigns/:id/entities/:entityID/permissions
func (h *APIHandler) SetEntityPermissions(c echo.Context) error {
	entityID := c.Param("entityID")
	ctx := c.Request().Context()
	role := h.resolveRole(c)

	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}

	// Verify entity belongs to the API key's campaign.
	if entity.CampaignID != c.Param("id") {
		return apperror.NewNotFound("entity not found")
	}

	// Only campaign owners can modify permissions.
	if role < int(campaigns.RoleOwner) {
		return apperror.NewForbidden("only campaign owners can modify entity permissions")
	}

	var input entities.SetPermissionsInput
	if err := c.Bind(&input); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.entitySvc.SetEntityPermissions(ctx, entityID, input); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// SetAddonChecker injects the addon checker for system-aware endpoints.
// Called after construction because the addon service is wired separately.
func (h *APIHandler) SetAddonChecker(ac AddonChecker) {
	h.addonChecker = ac
}

// SetAddonLister injects the addon lister for the discovery endpoint.
func (h *APIHandler) SetAddonLister(al AddonLister) {
	h.addonLister = al
}

// SetTagGrantLister injects the tag-grant lister so the permissions endpoint
// can expose tag-derived grants to the Foundry module.
func (h *APIHandler) SetTagGrantLister(tgl TagGrantLister) {
	h.tagGrantLister = tgl
}

// SetSystemEnabler injects the system enabler for API-level self-healing.
// When a campaign has a selected system that isn't enabled as an addon,
// the ListSystems endpoint auto-enables it so the Foundry module sees
// the system with enabled=true.
func (h *APIHandler) SetSystemEnabler(se SystemEnabler) {
	h.systemEnabler = se
}

// --- Systems ---

// systemInfoResponse is the API-safe representation of a game system.
type systemInfoResponse struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Status             string `json:"status"`
	HasCharacterFields bool   `json:"has_character_fields"`
	HasItemFields      bool   `json:"has_item_fields"`
	FoundrySystemID    string `json:"foundry_system_id"`
	Enabled            bool   `json:"enabled"`
}

// SystemEnabler enables a game system addon for a campaign. Used for
// API-level self-healing: if the campaign's selected system isn't enabled
// as an addon (e.g., set before self-healing was deployed), the API
// auto-enables it on read so the Foundry module sees enabled=true.
type SystemEnabler interface {
	EnableSystemForCampaign(ctx context.Context, campaignID, systemSlug, userID string) error
}

// CampaignSystemLister provides access to per-campaign custom systems.
type CampaignSystemLister interface {
	GetManifest(campaignID string) *systems.SystemManifest
}

// ListSystems returns game systems available for the campaign.
// Includes built-in systems from the global registry with an enabled flag
// based on per-campaign addon state. Used by the Foundry module to detect
// whether the current game system matches a Chronicle system.
//
// Self-healing: if the campaign has a selected system (in settings) but
// the addon isn't enabled — e.g., the system was set before self-healing
// was deployed — the endpoint auto-enables the addon on read so the
// Foundry module sees enabled=true without manual intervention.
// GET /api/v1/campaigns/:id/systems
func (h *APIHandler) ListSystems(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	// Resolve the campaign's selected system for self-healing.
	selectedSystemID := h.getSelectedSystemID(ctx, campaignID)

	registry := systems.Registry()
	result := make([]systemInfoResponse, 0, len(registry))

	for _, manifest := range registry {
		enabled := h.checkOrHealSystemAddon(ctx, campaignID, manifest.ID, selectedSystemID)

		result = append(result, systemInfoResponse{
			ID:                 manifest.ID,
			Name:               manifest.Name,
			Status:             string(manifest.Status),
			HasCharacterFields: manifest.CharacterPreset() != nil,
			HasItemFields:      manifest.ItemPreset() != nil,
			FoundrySystemID:    manifest.FoundrySystemID,
			Enabled:            enabled,
		})
	}

	// Include the campaign's custom system if one is uploaded.
	if h.campaignSystemLister != nil {
		if custom := h.campaignSystemLister.GetManifest(campaignID); custom != nil {
			enabled := h.checkOrHealSystemAddon(ctx, campaignID, custom.ID, selectedSystemID)
			result = append(result, systemInfoResponse{
				ID:                 custom.ID,
				Name:               custom.Name,
				Status:             string(custom.Status),
				HasCharacterFields: custom.CharacterPreset() != nil,
				FoundrySystemID:    custom.FoundrySystemID,
				Enabled:            enabled,
			})
		}
	}

	slog.Debug("ListSystems response",
		slog.String("campaign_id", campaignID),
		slog.Int("count", len(result)),
	)

	return c.JSON(http.StatusOK, map[string]any{
		"data":  result,
		"total": len(result),
	})
}

// getSelectedSystemID returns the campaign's configured system ID from
// settings, or empty string if none is set or the campaign can't be loaded.
func (h *APIHandler) getSelectedSystemID(ctx context.Context, campaignID string) string {
	campaign, err := h.campaignSvc.GetByID(ctx, campaignID)
	if err != nil || campaign == nil {
		return ""
	}
	return campaign.ParseSettings().SystemID
}

// checkOrHealSystemAddon checks if a system addon is enabled for a campaign.
// If the addon is NOT enabled but the system IS the campaign's selected system,
// it auto-enables the addon (self-healing for pre-deployment system selections).
func (h *APIHandler) checkOrHealSystemAddon(ctx context.Context, campaignID, systemID, selectedSystemID string) bool {
	if h.addonChecker == nil {
		return false
	}

	ok, err := h.addonChecker.IsEnabledForCampaign(ctx, campaignID, systemID)
	if err == nil && ok {
		return true
	}

	// Self-heal: system is selected in campaign settings but addon not enabled.
	if h.systemEnabler != nil && selectedSystemID != "" && systemID == selectedSystemID {
		if err := h.systemEnabler.EnableSystemForCampaign(ctx, campaignID, systemID, ""); err == nil {
			slog.Info("auto-healed system addon via API",
				slog.String("campaign_id", campaignID),
				slog.String("system_id", systemID),
			)
			return true
		}
		slog.Warn("API self-heal failed for system addon",
			slog.String("campaign_id", campaignID),
			slog.String("system_id", systemID),
		)
	}

	return false
}

// GetCharacterFields returns the character preset field definitions for a
// specific system, including Foundry path annotations. Used by the Foundry
// module's generic adapter to auto-generate field mappings at runtime.
// GET /api/v1/campaigns/:id/systems/:systemId/character-fields
func (h *APIHandler) GetCharacterFields(c echo.Context) error {
	campaignID := c.Param("id")
	systemID := c.Param("systemId")

	// Look up the system manifest: first in global registry, then custom.
	manifest := systems.Find(systemID)
	if manifest == nil && h.campaignSystemLister != nil {
		if custom := h.campaignSystemLister.GetManifest(campaignID); custom != nil && custom.ID == systemID {
			manifest = custom
		}
	}

	if manifest == nil {
		return apperror.NewNotFound("system not found: " + systemID)
	}

	resp := manifest.CharacterFieldsForAPI()
	if resp == nil {
		return apperror.NewNotFound("character fields not found for system: " + systemID)
	}

	return c.JSON(http.StatusOK, resp)
}

// GetItemFields returns the item preset field definitions for a specific
// system, including Foundry path annotations. Used by the Foundry module
// for item sync field mappings.
// GET /api/v1/campaigns/:id/systems/:systemId/item-fields
func (h *APIHandler) GetItemFields(c echo.Context) error {
	campaignID := c.Param("id")
	systemID := c.Param("systemId")

	// Look up the system manifest: first in global registry, then custom.
	manifest := systems.Find(systemID)
	if manifest == nil && h.campaignSystemLister != nil {
		if custom := h.campaignSystemLister.GetManifest(campaignID); custom != nil && custom.ID == systemID {
			manifest = custom
		}
	}

	if manifest == nil {
		return apperror.NewNotFound("system not found: " + systemID)
	}

	resp := manifest.ItemFieldsForAPI()
	if resp == nil {
		// System has no item preset — return empty fields instead of 404.
		// This is expected for systems like Draw Steel that have character
		// and creature presets but no formal item preset.
		return c.JSON(http.StatusOK, map[string]any{
			"system_id": systemID,
			"fields":    []any{},
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// --- Addon Discovery ---

// ListAddons returns all addons for the campaign with their enabled state.
// Used by external clients to discover available features without probing.
// GET /api/v1/campaigns/:id/addons
func (h *APIHandler) ListAddons(c echo.Context) error {
	campaignID := c.Param("id")

	if h.addonLister == nil {
		return c.JSON(http.StatusOK, map[string]any{"data": []any{}, "total": 0})
	}

	addons, err := h.addonLister.ListForCampaign(c.Request().Context(), campaignID)
	if err != nil {
		slog.Error("api: list addons failed", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list addons"))
	}

	if addons == nil {
		addons = []AddonInfo{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":  addons,
		"total": len(addons),
	})
}

// --- Relation Types & CRUD ---

// ListRelationTypes returns the predefined relation type pairs for the frontend.
// GET /api/v1/campaigns/:id/relations/types
func (h *APIHandler) ListRelationTypes(c echo.Context) error {
	types := h.relationSvc.GetCommonTypes()
	return c.JSON(http.StatusOK, map[string]any{
		"data":  types,
		"total": len(types),
	})
}

// apiCreateRelationRequest is the JSON body for creating a relation.
type apiCreateRelationRequest struct {
	TargetEntityID      string          `json:"target_entity_id"`
	RelationType        string          `json:"relation_type"`
	ReverseRelationType string          `json:"reverse_relation_type"`
	Metadata            json.RawMessage `json:"metadata"`
	DmOnly              bool            `json:"dm_only"`
}

// CreateRelation creates a new relation between two entities.
// POST /api/v1/campaigns/:id/entities/:entityID/relations
func (h *APIHandler) CreateRelation(c echo.Context) error {
	campaignID := c.Param("id")
	entityID := c.Param("entityID")
	ctx := c.Request().Context()

	// Verify source entity belongs to this campaign.
	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != campaignID {
		return apperror.NewNotFound("entity not found")
	}

	var req apiCreateRelationRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if req.TargetEntityID == "" {
		return apperror.NewBadRequest("target_entity_id is required")
	}
	if req.RelationType == "" {
		return apperror.NewBadRequest("relation_type is required")
	}

	userID := h.resolveUserID(c)
	rel, err := h.relationSvc.Create(ctx, campaignID, entityID, req.TargetEntityID,
		req.RelationType, req.ReverseRelationType, userID, req.Metadata, req.DmOnly)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, rel)
}

// apiUpdateRelationRequest is the JSON body for updating a relation's metadata.
type apiUpdateRelationRequest struct {
	Metadata json.RawMessage `json:"metadata"`
}

// UpdateRelation updates a relation's metadata.
// PUT /api/v1/campaigns/:id/relations/:relationId
func (h *APIHandler) UpdateRelation(c echo.Context) error {
	campaignID := c.Param("id")
	relationID, err := strconv.Atoi(c.Param("relationId"))
	if err != nil {
		return apperror.NewBadRequest("invalid relation ID")
	}

	// Verify the relation belongs to this campaign. The relation is addressed
	// by its enumerable integer primary key and the repository looks it up by
	// id alone, so without this check a key scoped to one campaign could
	// overwrite any relation row in the database.
	existing, err := h.relationSvc.GetByID(c.Request().Context(), relationID)
	if err != nil {
		return apperror.NewNotFound("relation not found")
	}
	if existing.CampaignID != campaignID {
		return apperror.NewNotFound("relation not found")
	}

	var req apiUpdateRelationRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.relationSvc.UpdateMetadata(c.Request().Context(), relationID, req.Metadata); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteRelation removes a relation and its reverse.
// DELETE /api/v1/campaigns/:id/relations/:relationId
func (h *APIHandler) DeleteRelation(c echo.Context) error {
	campaignID := c.Param("id")
	relationID, err := strconv.Atoi(c.Param("relationId"))
	if err != nil {
		return apperror.NewBadRequest("invalid relation ID")
	}

	// Verify the relation belongs to this campaign before deleting -- same
	// cross-campaign exposure as UpdateRelation, and Delete also removes the
	// reverse direction, so an unscoped call destroys two rows.
	existing, err := h.relationSvc.GetByID(c.Request().Context(), relationID)
	if err != nil {
		return apperror.NewNotFound("relation not found")
	}
	if existing.CampaignID != campaignID {
		return apperror.NewNotFound("relation not found")
	}

	if err := h.relationSvc.Delete(c.Request().Context(), relationID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// --- Entity Type CRUD ---

// apiCreateEntityTypeRequest is the JSON body for creating an entity type.
type apiCreateEntityTypeRequest struct {
	Name       string `json:"name"`
	NamePlural string `json:"name_plural"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
}

// CreateEntityType creates a new entity type for the campaign.
// POST /api/v1/campaigns/:id/entity-types
func (h *APIHandler) CreateEntityType(c echo.Context) error {
	campaignID := c.Param("id")

	var req apiCreateEntityTypeRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	et, err := h.entitySvc.CreateEntityType(c.Request().Context(), campaignID, entities.CreateEntityTypeInput{
		Name:       req.Name,
		NamePlural: req.NamePlural,
		Icon:       req.Icon,
		Color:      req.Color,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, et)
}

// apiUpdateEntityTypeRequest is the JSON body for updating an entity type.
type apiUpdateEntityTypeRequest struct {
	Name       string `json:"name"`
	NamePlural string `json:"name_plural"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
}

// UpdateEntityType updates an existing entity type.
// PUT /api/v1/campaigns/:id/entity-types/:typeID
func (h *APIHandler) UpdateEntityType(c echo.Context) error {
	campaignID := c.Param("id")
	typeID, err := strconv.Atoi(c.Param("typeID"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	// Verify entity type belongs to this campaign.
	existing, err := h.entitySvc.GetEntityTypeByID(c.Request().Context(), typeID)
	if err != nil {
		return apperror.NewNotFound("entity type not found")
	}
	if existing.CampaignID != campaignID {
		return apperror.NewNotFound("entity type not found")
	}

	var req apiUpdateEntityTypeRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	et, err := h.entitySvc.UpdateEntityType(c.Request().Context(), typeID, entities.UpdateEntityTypeInput{
		Name:       req.Name,
		NamePlural: req.NamePlural,
		Icon:       req.Icon,
		Color:      req.Color,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, et)
}

// --- Bulk Operations ---

// apiBulkUpdateEntityTypeRequest is the JSON body for bulk entity type reassignment.
type apiBulkUpdateEntityTypeRequest struct {
	EntityIDs    []string `json:"entity_ids"`
	EntityTypeID int      `json:"entity_type_id"`
}

// BulkUpdateEntityType changes the entity type for multiple entities at once.
// POST /api/v1/campaigns/:id/entities/bulk-update
func (h *APIHandler) BulkUpdateEntityType(c echo.Context) error {
	campaignID := c.Param("id")

	var req apiBulkUpdateEntityTypeRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	const maxBulkEntities = 200
	if len(req.EntityIDs) == 0 {
		return apperror.NewBadRequest("entity_ids is required")
	}
	if len(req.EntityIDs) > maxBulkEntities {
		return apperror.NewBadRequest(fmt.Sprintf("too many entities; maximum is %d per request", maxBulkEntities))
	}
	if req.EntityTypeID == 0 {
		return apperror.NewBadRequest("entity_type_id is required")
	}

	updated, err := h.entitySvc.BulkUpdateType(c.Request().Context(), campaignID, req.EntityIDs, req.EntityTypeID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":  "ok",
		"updated": updated,
	})
}
