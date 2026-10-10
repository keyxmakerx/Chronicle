package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/extensions"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/notifyprefs"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/prompt"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/backup"
	"github.com/keyxmakerx/chronicle/internal/plugins/bestiary"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/designlab"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/npcs"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	"github.com/keyxmakerx/chronicle/internal/plugins/restore"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
	"github.com/keyxmakerx/chronicle/internal/plugins/smtp"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	"github.com/keyxmakerx/chronicle/internal/plugins/systemstate"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
	"github.com/keyxmakerx/chronicle/internal/systems"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
	"github.com/keyxmakerx/chronicle/internal/templates/pages"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
	"github.com/keyxmakerx/chronicle/internal/widgets/entity_notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/posts"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	skywidget "github.com/keyxmakerx/chronicle/internal/widgets/sky/templates"
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
	// avatar_path holds a media id, not a URL. The bestiary pages are signed-in
	// only, and the media route serves an avatar to any signed-in session
	// without a signature, so the plain thumbnail path is enough here.
	if user.AvatarPath != nil {
		if id, err := uuid.Parse(*user.AvatarPath); err == nil {
			info.AvatarURL = "/media/" + id.String() + "/thumb/300"
		}
	}
	return info, nil
}

// bestiaryEntityCreatorAdapter wraps entities.EntityService to implement the
// bestiary.EntityCreator interface for importing creatures into campaigns.
//
// It carries campaignSvc only to read the campaign's DefaultVisibility
// setting. The bestiary plugin has no opinion about visibility and its
// EntityCreator interface has no is_private parameter, so an import is
// always the ABSENT case: the campaign default decides.
type bestiaryEntityCreatorAdapter struct {
	svc         entities.EntityService
	campaignSvc campaigns.CampaignService
}

// CreateFromStatblock creates a new entity in a campaign from a bestiary statblock.
// Uses entity type ID 0 (default) since the real type depends on system configuration.
//
// Visibility comes from the campaign's DefaultVisibility setting: the
// bestiary's EntityCreator interface has no is_private parameter and the
// import UI has no per-import visibility control, so the campaign default is
// the only input there is.
func (a *bestiaryEntityCreatorAdapter) CreateFromStatblock(ctx context.Context, campaignID, userID, name string, statblock json.RawMessage) (string, error) {
	input := entities.CreateEntityInput{
		Name:       name,
		FieldsData: map[string]any{"statblock_json": string(statblock)},
		IsPrivate:  a.defaultPrivate(ctx, campaignID),
	}
	ent, err := a.svc.Create(ctx, campaignID, userID, input)
	if err != nil {
		return "", err
	}
	return ent.ID, nil
}

// defaultPrivate reports whether this campaign's DefaultVisibility setting
// means "new content starts hidden".
//
// It FAILS CLOSED. An import whose campaign settings cannot be read still
// succeeds — a transient read failure is no reason to break creature import —
// but it succeeds hidden, because "we could not find out what the DM asked
// for" is not a licence to publish. Over-hiding costs the DM one toggle;
// revealing a boss statblock to the party cannot be undone.
func (a *bestiaryEntityCreatorAdapter) defaultPrivate(ctx context.Context, campaignID string) bool {
	if a.campaignSvc == nil {
		slog.Error("bestiary import: no campaign service wired; imported entity defaults to private",
			slog.String("campaign_id", campaignID))
		return true
	}
	campaign, err := a.campaignSvc.GetByID(ctx, campaignID)
	if err != nil || campaign == nil {
		slog.Error("bestiary import: could not read campaign default visibility; imported entity defaults to private",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return true
	}
	return campaign.ParseSettings().DefaultsToPrivate()
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

// ListForPluginHub returns all addons formatted for the plugin hub page and
// the top-level Extensions hub. HasDashboard / HasEntitySetup are sourced
// from the campaigns plugin's slug-keyed capability tables (see
// internal/plugins/campaigns/extensions_hub.go) so the hub catalog carries a
// single source of truth.
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
			Description:    ca.AddonDescription,
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

// foundryConnectorAdapter implements campaigns.FoundryConnector over the sync
// service (keys), the websocket hub (live presence) and the configured base
// URL, so the campaigns plugin never reaches into either plugin's internals.
type foundryConnectorAdapter struct {
	keys syncapi.SyncAPIService
	hub  interface {
		FoundryPresence(campaignID string) (*time.Time, bool)
	}
	baseURL string
	// served names the module version the campaign's world is served, so the
	// Foundry page can say when Foundry still runs an older one. May be nil.
	served interface {
		View(ctx context.Context, campaignID string) (*foundry_vtt.OwnerUpdateView, error)
	}
}

// foundryConnectKeyName and foundryConnectVTTTag mark keys minted from the
// Foundry page; the tag matches the value the Integrations form's
// Foundry option stores, so these keys group with hand-made Foundry keys.
const (
	foundryConnectKeyName = "Foundry connect line"
	foundryConnectVTTTag  = "foundry"
)

// FoundryConnection gathers the hub presence and the campaign's active keys.
// "Active" means enabled and unexpired; a revoked or lapsed key says nothing
// about whether Foundry is connected today.
func (a *foundryConnectorAdapter) FoundryConnection(ctx context.Context, campaignID string) (campaigns.FoundryConnection, error) {
	keys, err := a.keys.ListKeysByCampaign(ctx, campaignID)
	if err != nil {
		return campaigns.FoundryConnection{}, err
	}
	conn := campaigns.FoundryConnection{}
	if a.hub != nil {
		conn.HubLastSeen, conn.Connected = a.hub.FoundryPresence(campaignID)
	}

	// Keys arrive newest-created first, so the first active key is the one the
	// preview describes. Module version follows the most recently *used* key.
	var newestUse *time.Time
	for i := range keys {
		k := &keys[i]
		if !k.IsActive || k.IsExpired() {
			continue
		}
		// A key labelled "custom" belongs to some other tool (a bot, a
		// script); counting it would show that tool's activity as Foundry's.
		if k.VTTTag != nil && *k.VTTTag == "custom" {
			continue
		}
		if !conn.HasKey {
			conn.HasKey = true
			conn.KeyPrefix = k.KeyPrefix
		}
		if k.LastUsedAt != nil && (newestUse == nil || k.LastUsedAt.After(*newestUse)) {
			newestUse = k.LastUsedAt
			conn.KeyLastUsed = k.LastUsedAt
			conn.ModuleVersion = ""
			if k.ModuleVersion != nil {
				conn.ModuleVersion = *k.ModuleVersion
			}
		}
	}
	if conn.HasKey {
		// The prefix is stored in clear for display; the ellipsis is appended
		// after escaping so it stays a literal character.
		if line, err := campaigns.BuildFoundryConnectLine(a.baseURL, campaignID, conn.KeyPrefix); err == nil {
			conn.LinePreview = line + "\u2026"
		}
	}
	if a.served != nil {
		// A failed lookup only hides the version check; the rest of the page
		// still answers.
		if v, err := a.served.View(ctx, campaignID); err == nil && v != nil {
			conn.ServedVersion = v.Running
		}
	}
	return conn, nil
}

// NewFoundryConnectLine mints a read/write/sync key and returns the full
// connect line. Existing keys are left untouched. The base URL is checked
// before minting: the raw key is shown once, so a key minted and then lost to a
// line-building failure could never be recovered.
func (a *foundryConnectorAdapter) NewFoundryConnectLine(ctx context.Context, campaignID, userID string) (string, error) {
	if _, err := campaigns.BuildFoundryConnectLine(a.baseURL, campaignID, "probe"); err != nil {
		return "", apperror.NewInternal(fmt.Errorf("base url cannot form a connect line: %w", err))
	}
	result, err := a.keys.CreateKey(ctx, userID, syncapi.CreateAPIKeyInput{
		Name:        foundryConnectKeyName,
		VTTTag:      foundryConnectVTTTag,
		CampaignID:  campaignID,
		Permissions: []syncapi.APIKeyPermission{syncapi.PermRead, syncapi.PermWrite, syncapi.PermSync},
	})
	if err != nil {
		return "", err
	}
	return campaigns.BuildFoundryConnectLine(a.baseURL, campaignID, result.RawKey)
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

// OwnsFile reports whether filename (the stored "YYYY/MM/<id>.<ext>" that
// UploadBackdrop returns) is a media file belonging to campaignID.
func (a *backdropUploaderAdapter) OwnsFile(ctx context.Context, campaignID, filename string) (bool, error) {
	base := path.Base(filename)
	id := strings.TrimSuffix(base, path.Ext(base))
	mf, err := a.svc.GetByID(ctx, id)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	// A note picture or page file is readable only through what holds it; a
	// campaign backdrop must not adopt one by its stored name.
	if mf.IsBound() {
		return false, nil
	}
	return mf.Filename == filename && mf.CampaignID != nil && *mf.CampaignID == campaignID, nil
}

// DeletePicture deletes an appearance picture the Customize page uploaded
// and never saved. UsageBackdrop is what UploadBackdrop stamps on every file
// this adapter stores, so a campaign's other media (attachments, entity
// images, avatars) can never match, even by guessing a name.
func (a *backdropUploaderAdapter) DeletePicture(ctx context.Context, campaignID, filename string) (bool, error) {
	base := path.Base(filename)
	id := strings.TrimSuffix(base, path.Ext(base))
	mf, err := a.svc.GetByID(ctx, id)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	if mf.Filename != filename || mf.UsageType != media.UsageBackdrop ||
		mf.CampaignID == nil || *mf.CampaignID != campaignID {
		return false, nil
	}
	if err := a.svc.Delete(ctx, mf.ID); err != nil {
		return false, err
	}
	return true, nil
}

// siteMediaAdapter stores the site's logo and sign-in picture through the
// media service. They are campaignless UsageBackdrop files, which the media
// handler serves publicly (the sign-in page has no signed-in viewer). The only
// other writer of UsageBackdrop always sets a campaign, so a campaignless
// backdrop is a site picture and nothing else.
type siteMediaAdapter struct {
	svc media.MediaService
}

// StoreSitePicture saves a validated image with no campaign.
func (a *siteMediaAdapter) StoreSitePicture(ctx context.Context, userID string, data []byte, originalName, mimeType string) (string, error) {
	mf, err := a.svc.Upload(ctx, media.UploadInput{
		UploadedBy:   userID,
		OriginalName: originalName,
		MimeType:     mimeType,
		FileSize:     int64(len(data)),
		UsageType:    media.UsageBackdrop,
		FileBytes:    data,
	})
	if err != nil {
		return "", err
	}
	return mf.Filename, nil
}

// site returns the media row for filename when it is a site picture.
func (a *siteMediaAdapter) site(ctx context.Context, filename string) (*media.MediaFile, error) {
	base := path.Base(filename)
	id := strings.TrimSuffix(base, path.Ext(base))
	mf, err := a.svc.GetByID(ctx, id)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	if mf.Filename != filename || mf.UsageType != media.UsageBackdrop || mf.CampaignID != nil {
		return nil, nil
	}
	return mf, nil
}

// OwnsSitePicture reports whether filename is a site picture.
func (a *siteMediaAdapter) OwnsSitePicture(ctx context.Context, filename string) (bool, error) {
	mf, err := a.site(ctx, filename)
	return mf != nil, err
}

// DeleteSitePicture removes filename if it is a site picture.
func (a *siteMediaAdapter) DeleteSitePicture(ctx context.Context, filename string) error {
	mf, err := a.site(ctx, filename)
	if err != nil || mf == nil {
		return err
	}
	return a.svc.Delete(ctx, mf.ID)
}

// entityTagFetcherAdapter wraps tags.TagService to implement the
// entities.EntityTagFetcher interface for batch tag loading in list views.
// grantSvc backs the tag-grant glance methods.
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
// for its effective-visibility glance.
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

// entityVisibilityFilterAdapter wraps entities.EntityService to implement the
// sessions.EntityVisibilityFilter interface, so the sessions plugin can hide a
// linked entity's name from a viewer who could not see that entity directly
// (ADR-055 rule 3) without importing the entities plugin's repository.
type entityVisibilityFilterAdapter struct {
	svc entities.EntityService
}

// FilterViewableEntityIDs delegates to the entities plugin's own visibility
// policy — the same one entity pages and the relations widget already apply.
func (a *entityVisibilityFilterAdapter) FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error) {
	return a.svc.FilterViewableEntityIDs(ctx, campaignID, entityIDs, role, userID)
}

// CALV5-PLACEHOLDER: V5 must still re-implement six cross-plugin bridge
// adapters against its own service, toward the calendar:
// timelineForCalendarAdapter, calendarSyncLinkAdapter,
// calendarEntityCreatorAdapter, calendarAvailabilityAdapter (member
// zones, exception dates, offered windows), calendarBenchScheduleAdapter,
// calendarOwnWeekAdapter. Each is a narrow interface owned by the consuming
// plugin, not a shared type — keep that shape so the rebuild stays
// surgical. TODO(#778)
//
// The three below feed timeline away from the calendar (selector, event
// picker, era bands); calendarEventLinkListerAdapter after them is a
// separate seam, timeline's calendar-name display and its own
// timeline_event_links -> calendar_events resolution.

// calendarListerAdapter powers the create-form's calendar selector
// (GET /campaigns/:id/timelines/calendars) — Owner-gated at the route, and
// this interface carries no role, so it reads explicitly at Owner level.
type calendarListerAdapter struct {
	svc calendar.CalendarService
}

func (a *calendarListerAdapter) ListCalendars(ctx context.Context, campaignID string) ([]timeline.CalendarRef, error) {
	cals, err := a.svc.ListCalendars(ctx, campaignID, permissions.RequestViewer(permissions.RoleOwner, ""))
	if err != nil {
		return nil, err
	}
	refs := make([]timeline.CalendarRef, len(cals))
	for i, cal := range cals {
		refs[i] = timeline.CalendarRef{ID: cal.ID, Name: cal.Name}
	}
	return refs, nil
}

// calendarEventListerAdapter powers the timeline event-picker's list of
// linkable calendar events. CalendarService.ListEventsForCalendar already
// scopes to campaignID and gates dm_only by role; this just reshapes.
type calendarEventListerAdapter struct {
	svc calendar.CalendarService
}

func (a *calendarEventListerAdapter) ListEventsForCalendar(ctx context.Context, campaignID, calendarID string, role int) ([]timeline.CalendarEventRef, error) {
	events, err := a.svc.ListEventsForCalendar(ctx, campaignID, calendarID, role)
	if err != nil {
		return nil, err
	}
	refs := make([]timeline.CalendarEventRef, 0, len(events))
	for _, ev := range events {
		var category *string
		if ev.KindSlug != "" {
			category = &ev.KindSlug
		}
		refs = append(refs, timeline.CalendarEventRef{
			ID: ev.ID, Name: ev.Name, Description: ev.Description,
			Year: ev.Year, Month: ev.Month, Day: ev.Day,
			EndYear: ev.EndYear, EndMonth: ev.EndMonth, EndDay: ev.EndDay,
			Category: category, Visibility: ev.Visibility,
			EntityID: ev.EntityID, EntityName: ev.EntityName, EntityIcon: ev.EntityIcon,
		})
	}
	return refs, nil
}

// calendarEraListerAdapter powers the D3 timeline visualization's era
// background bands. CalendarService.ListErasForCalendar gates on role alone
// — eras are calendar structure, Owner/co-DM only regardless of the bound
// calendar's own visibility.
type calendarEraListerAdapter struct {
	svc calendar.CalendarService
}

func (a *calendarEraListerAdapter) ListEras(ctx context.Context, campaignID, calendarID string, role int) ([]timeline.CalendarEra, error) {
	eras, err := a.svc.ListErasForCalendar(ctx, campaignID, calendarID, role)
	if err != nil {
		return nil, err
	}
	refs := make([]timeline.CalendarEra, 0, len(eras))
	for _, e := range eras {
		refs = append(refs, timeline.CalendarEra{Name: e.Name, StartYear: e.StartYear, EndYear: e.EndYear, Color: e.Color})
	}
	return refs, nil
}

// calendarEventLinkListerAdapter feeds timeline's calendar-name display and
// its timeline_event_links -> calendar_events resolution — a separate seam
// from the three adapters above (see the CALV5-PLACEHOLDER comment).
type calendarEventLinkListerAdapter struct {
	svc calendar.CalendarService
}

// CalendarName implements timeline.CalendarEventLinkLister. A real,
// role-gated read — NOT a system bypass — because unlike EventsByIDs'
// per-event visibility (which the deleted JOIN already enforced), the
// calendar's OWN visibility was never checked at this seam before: a
// dm_only calendar's real name would otherwise leak to a Player/anonymous
// timeline viewer through this display-only field. Built as a role-only
// RequestViewer, matching EventsByIDs' own convention just below (the
// interface carries no per-request user id, so a calendar's per-user
// visibility_rules allow/deny list is not evaluated here — role alone is
// enough to keep a dm_only calendar's name away from anyone who isn't at
// least an Owner/co-DM). Best-effort — any miss (not found, or hidden from
// this role) degrades to an empty name rather than surfacing as a page
// error, matching the deleted `LEFT JOIN` + `COALESCE(c.name, "")`'s own
// failure-to-empty behavior.
func (a *calendarEventLinkListerAdapter) CalendarName(ctx context.Context, campaignID, calendarID string, role int) string {
	if calendarID == "" {
		return ""
	}
	v := permissions.RequestViewer(role, "")
	name, err := a.svc.GetCalendarNameForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return ""
	}
	return name
}

// EventsByIDs implements timeline.CalendarEventLinkLister. Built as a plain
// role-only RequestViewer (no per-request user id — the interface carries
// none, matching CalendarEventLister's existing picker convention above):
// this is STRICTER than the deleted JOIN's own `ce.visibility = 'everyone'`
// check, never looser, since it also runs a calendar event's own
// visibility_rules and redacts a hidden linked entity, both of which the old
// SQL never touched.
// One batch read serves every id, so cost no longer grows with link count.
func (a *calendarEventLinkListerAdapter) EventsByIDs(ctx context.Context, calendarID, campaignID string, eventIDs []string, role int) ([]timeline.CalendarEventRef, error) {
	if calendarID == "" || len(eventIDs) == 0 {
		return nil, nil
	}
	v := permissions.RequestViewer(role, "")
	events, err := a.svc.ListEventsByIDsForViewer(ctx, calendarID, campaignID, eventIDs, v)
	if err != nil {
		if isNotFound(err) {
			// Calendar missing or hidden from this role: no event resolves.
			return nil, nil
		}
		return nil, err
	}
	refs := make([]timeline.CalendarEventRef, 0, len(events))
	for i := range events {
		evt := &events[i]
		var category *string
		if evt.KindSlug != "" {
			category = &evt.KindSlug
		}
		refs = append(refs, timeline.CalendarEventRef{
			ID: evt.ID, Name: evt.Name, Description: evt.Description,
			Year: evt.Year, Month: evt.Month, Day: evt.Day,
			EndYear: evt.EndYear, EndMonth: evt.EndMonth, EndDay: evt.EndDay,
			Category: category, Visibility: evt.Visibility,
			EntityID: evt.EntityID, EntityName: evt.EntityName, EntityIcon: evt.EntityIcon,
		})
	}
	return refs, nil
}

// calendarScopeAdapter implements timeline.CalendarScope with the calendar
// plugin's own campaign-scoped read. It asks at Owner level, the level every
// caller already holds (the create route is Owner-only and campaign import
// runs as the owner), so any calendar in the campaign is found and the
// answer depends only on which campaign the calendar lives in.
type calendarScopeAdapter struct {
	svc calendar.CalendarService
}

// CalendarInCampaign implements timeline.CalendarScope.
func (a *calendarScopeAdapter) CalendarInCampaign(ctx context.Context, campaignID, calendarID string) (bool, error) {
	_, err := a.svc.GetCalendarForViewer(ctx, calendarID, campaignID, permissions.RequestViewer(permissions.RoleOwner, ""))
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

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

// wsNotesGrantAdapter checks a notes grant for a WebSocket upgrade the same
// way notes.RequireAppGrant checks it for a request: a live grant, in a
// campaign whose outside apps are on.
type wsNotesGrantAdapter struct {
	grants notes.AppGrantService
	gate   notes.AppGate
}

// AuthenticateNotesGrantForWS returns the grant's campaign and player.
func (a *wsNotesGrantAdapter) AuthenticateNotesGrantForWS(ctx context.Context, token string) (string, string, error) {
	g, err := a.grants.Authenticate(ctx, token)
	if err != nil {
		return "", "", err
	}
	if a.gate != nil {
		open, err := a.gate(ctx, g.CampaignID)
		if err != nil {
			return "", "", err
		}
		if !open {
			return "", "", apperror.NewForbidden("this campaign has outside apps turned off")
		}
	}
	return g.CampaignID, g.UserID, nil
}

// CALV5-PLACEHOLDER: V5 must rebuild calendarEventPublisherAdapter — the
// bridge from the calendar's PublishCalendarEvent to the websocket bus.
// Nothing publishes calendar events now.
//
// A switch ending in `default: return` fails silently: an emitter whose event
// type has no case here publishes into nothing, unreported. Rebuild it with a
// test that walks every emitter's event type and asserts a case exists.
// TODO(#778)
//
// The mapping it carried, so V5 has the checklist rather than rediscovering it:
//   "event.created"                            -> ws.MsgCalendarEventCreated
//   "event.updated"                            -> ws.MsgCalendarEventUpdated
//   "event.deleted"                            -> ws.MsgCalendarEventDeleted
//   "date.advanced"                            -> ws.MsgCalendarDateAdvanced
//   "calendar.weather.changed"                 -> ws.MsgCalendarWeatherChanged
//   "calendar.structure.updated"               -> ws.MsgCalendarStructureUpdated
//   "calendar.season.changed"                  -> ws.MsgCalendarSeasonChanged
//   "calendar.era.changed"                     -> ws.MsgCalendarEraChanged
//   "calendar.moon.phase_changed"              -> ws.MsgCalendarMoonPhaseChanged
//   "calendar.cycle.changed"                   -> ws.MsgCalendarCycleChanged
//   "calendar.festival.changed"                -> ws.MsgCalendarFestivalChanged
//   calendar.EventWorldStateChanged            -> ws.MsgCalendarWorldstateChanged
//   calendar.EventWorldStateChangedDM          -> ws.MsgCalendarWorldstateChanged
//   "calendar.weather.zones.changed"           -> ws.MsgCalendarWeatherZonesChanged
//   (calendar.worldstate.changed also had a DM-gated twin that set
//   RequiresDM = true — the dm_only worldstate payload must never reach a
//   player's socket.)

// relationEventPublisherAdapter bridges the websocket.EventBus to the
// relations.RelationEventPublisher interface.
type relationEventPublisherAdapter struct {
	bus ws.EventBus
	// shares, when set, re-checks the row's source character's item shares
	// after a relation is deleted or its metadata changes, so a share never
	// outlives the holding. Runs detached: the armory's own moves call this
	// while holding the campaign lock the check takes.
	shares interface {
		ReleaseLetGoShares(ctx context.Context, campaignID, characterID string)
	}
}

// PublishRelationEvent translates a relation row write into a WebSocket
// message keyed on the row's source entity.
func (a *relationEventPublisherAdapter) PublishRelationEvent(eventType string, rel *relations.Relation) {
	if rel == nil || rel.CampaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch eventType {
	case relations.RelationEventCreated:
		msgType = ws.MsgRelationCreated
	case relations.RelationEventDeleted:
		msgType = ws.MsgRelationDeleted
	case relations.RelationEventMetadataUpdated:
		msgType = ws.MsgRelationMetadataUpdated
	default:
		return
	}
	// A relation can name a private entity or be dm_only, and the row
	// carries none of the per-viewer filtering the HTTP reads apply, so it
	// only goes to DM-equivalent sockets, like entity events.
	msg := ws.NewMessage(msgType, rel.CampaignID, rel.SourceEntityID, rel)
	msg.RequiresDM = true
	a.bus.Publish(msg)

	if a.shares != nil && eventType != relations.RelationEventCreated {
		campaignID, characterID := rel.CampaignID, rel.SourceEntityID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			a.shares.ReleaseLetGoShares(ctx, campaignID, characterID)
		}()
	}
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
	// The payload is the whole stored entity: private pages, GM-only fields
	// and secret text included, none of the per-viewer filtering the HTTP
	// reads apply. So it only goes to DM-equivalent sockets; anyone else
	// reads entities over HTTP, where that filtering happens.
	msg := ws.NewMessage(msgType, campaignID, entityID, entity)
	msg.RequiresDM = true
	a.bus.Publish(msg)
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
	// which campaigns.NormalizeNav already completes with every top-level
	// type — including this new one, in its natural sort_order position.
	// Persisting a lone category item here would instead make the new type the
	// only explicit entry and snap it to the FRONT of the list, so empty
	// configs are left to NormalizeNav. Converted campaigns (the reconciler put
	// them on a non-empty Items array) fall through and correctly auto-gain the
	// new type appended in their customized order.
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

// navAppDef is one app page the campaign sidebar can list. It lives here, in
// the composition root, because it names other plugins' addons and routes.
type navAppDef struct {
	slug    string // the app item's slug in sidebar_config (Journal keeps "notes", its addon)
	label   string
	icon    string
	path    string   // campaign-relative page
	caption string   // a few muted words beside the label
	addons  []string // the app is on when any of these addons is enabled
	needs   []string // and only while every one of these is enabled too
	access  campaigns.NavAccess
	pinned  bool // starts in Pinned for a campaign that never arranged its sidebar
	system  bool // the enabled game system's reference; label, icon and path come from it
}

// navAppCatalog lists every app the sidebar can show, in the order a campaign
// that never arranged its sidebar lists them. Each access level mirrors the
// app route's own gate, so the sidebar never offers a page that would turn
// the viewer away; Game nights has its own switch (addon slug "sessions")
// and also needs the calendar addon, because its routes need both. Its slug stays "sessions" so sidebars already arranged keep it. Characters is the campaign's cast, party and NPCs together, which is
// why the NPC gallery addon also turns it on.
var navAppCatalog = []navAppDef{
	{slug: "notes", label: "Journal", icon: "fa-book-open", path: "/journal", addons: []string{"notes"}, access: campaigns.NavAccessMember, pinned: true},
	{slug: "calendar", label: "Calendar", icon: "fa-calendar-days", path: "/apps/calendar", addons: []string{calendar.PluginSlug}, access: campaigns.NavAccessMemberOrAdmin, pinned: true},
	{slug: "sessions", label: "Game nights", icon: "fa-dice-d20", path: "/game-nights", addons: []string{"sessions"}, needs: []string{calendar.PluginSlug}, access: campaigns.NavAccessAnyone},
	{slug: "maps", label: "Maps", icon: "fa-map", path: "/maps", addons: []string{"maps"}, access: campaigns.NavAccessAnyone},
	{slug: "characters", label: "Characters", icon: "fa-masks-theater", path: "/characters", caption: "Party & NPCs", addons: []string{entities.AddonPlayerCharacterClaiming, "npcs"}, access: campaigns.NavAccessAnyone},
	{slug: "armory", label: "Armory", icon: "fa-shield-halved", path: "/armory", addons: []string{"armory"}, access: campaigns.NavAccessAnyone},
	{slug: "timeline", label: "Timeline", icon: "fa-timeline", path: "/timelines", addons: []string{"timeline"}, access: campaigns.NavAccessAnyone},
	{slug: "rulebook", label: "Rulebook", icon: "fa-book", system: true, access: campaigns.NavAccessMemberOrAdmin},
}

// navAppPath returns the campaign-relative page an app links to ("" for an
// unknown slug or the system reference, whose path depends on the system).
// The operator diagnostic reports the sidebar's Calendar destination from it.
func navAppPath(slug string) string {
	for _, d := range navAppCatalog {
		if d.slug == slug {
			return d.path
		}
	}
	return ""
}

// noteEventPublisherAdapter bridges the websocket.EventBus to the
// notes.NoteEventPublisher interface.
type noteEventPublisherAdapter struct {
	bus ws.EventBus
}

// PublishNoteEvent translates a note change into a WebSocket message that
// carries IDs only and reaches only the note's audience. Clients refetch the
// note over HTTP, where CanView decides what they get (#715). The payload
// deliberately has no "id" key: a client that reads payload.id as a whole
// note must skip the message, not overwrite its copy with an empty one.
func (a *noteEventPublisherAdapter) PublishNoteEvent(ev notes.NoteEvent) {
	if ev.CampaignID == "" {
		return
	}
	var msgType ws.MessageType
	switch ev.Type {
	case "created":
		msgType = ws.MsgNoteCreated
	case "updated":
		msgType = ws.MsgNoteUpdated
	case "deleted":
		msgType = ws.MsgNoteDeleted
	default:
		return
	}
	payload := map[string]string{"noteId": ev.NoteID}
	if ev.EntityID != nil {
		payload["entityId"] = *ev.EntityID
	}
	msg := ws.NewMessage(msgType, ev.CampaignID, ev.NoteID, payload)
	msg.AllowedUsers, msg.StrictAudience = noteAudience(ev.Audience)
	a.bus.Publish(msg)
}

// noteAudience maps a note's audience onto the hub's allowlist. Shared with
// the party: no list. Otherwise the named users; a GM share lets the hub's
// usual DM bypass admit the Owner and co-DMs, and without one the list binds
// them too, because a private note is private from the GM.
func noteAudience(a notes.Audience) (allowed []string, strict bool) {
	if a.Everyone {
		return nil, false
	}
	if len(a.Users) == 0 {
		// An audience naming nobody reaches nobody; never widen a malformed
		// one to "everyone", which is what an empty allowlist means.
		return []string{noteNoRecipient}, true
	}
	return a.Users, !a.GMs
}

// noteNoRecipient is an allowlist entry no user id can equal (ids are UUIDs).
const noteNoRecipient = "-"

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

// wsRevokerHolder bridges each plugin's own ConnectionRevoker interface
// to the hub. hub is set once in RegisterRoutes before the server accepts
// connections, so the field needs no mutex.
type wsRevokerHolder struct {
	hub *ws.Hub
}

func (h *wsRevokerHolder) RevokeAPIKeyClients(campaignID string) {
	if h.hub != nil {
		h.hub.RevokeAPIKeyClients(campaignID)
	}
}

func (h *wsRevokerHolder) RevokeUser(campaignID, userID string) {
	if h.hub != nil {
		h.hub.RevokeUser(campaignID, userID)
	}
}

func (h *wsRevokerHolder) RevokeNotesAppClients(campaignID, userID string) {
	if h.hub != nil {
		h.hub.RevokeNotesAppClients(campaignID, userID)
	}
}

func (h *wsRevokerHolder) RevokeNotesAppClientsEverywhere(userID string) {
	if h.hub != nil {
		h.hub.RevokeNotesAppClientsEverywhere(userID)
	}
}

func (h *wsRevokerHolder) RevokeUserEverywhere(userID string) {
	if h.hub != nil {
		h.hub.RevokeUserEverywhere(userID)
	}
}

func (h *wsRevokerHolder) RevokeCampaign(campaignID string) {
	if h.hub != nil {
		h.hub.RevokeCampaign(campaignID)
	}
}

// mapEventPublisherAdapter bridges the websocket.EventBus to the maps.MapEventPublisher
// interface, translating domain events into WebSocket messages.
type mapEventPublisherAdapter struct {
	bus ws.EventBus
	// shadows resolves a map's shadow areas so a pin or drawing under one is
	// published to DM-equivalent clients only. Nil fails closed: with no way to
	// tell, every pin and drawing event is restricted.
	shadows maps.ShadowLookup
	// frames reads a map's frame, to place a turned picture against shadows.
	// Nil leaves the frame unknown, which withholds every turned picture.
	frames interface {
		MapFrame(ctx context.Context, mapID string) maps.MapFrame
	}
	// fog resolves a map's unexplored hexes so a pin or drawing wholly inside
	// them is published to DM-equivalent clients only. Nil fails closed, like
	// shadows.
	fog maps.HexFogLookup
}

// wireHexFog gives pins, drawings, tokens, the event publisher, the map picture
// and the media guard their fog source (the hex service, and the drawing
// service for which picture files it withholds), and the hex service the lookups it needs
// to build the fog and to announce changes. A named helper so a test can prove
// the production wiring sets all of it; without it players would see every pin
// and the whole picture under unexplored hexes.
func wireHexFog(mapsService maps.MapService, drawingService maps.DrawingService, events *mapEventPublisherAdapter, hexService maps.HexService) {
	mapsService.SetHexFogLookup(hexService)
	mapsService.SetPictureFileLookup(drawingService)
	drawingService.SetHexFogLookup(hexService)
	events.fog = hexService
	hexService.SetEventPublisher(events)
	hexService.SetMapLoader(mapsService.GetMap)
	// The terrain art lives in the map's display settings; the map service
	// exposes the narrow write by assertion, as the picture drop below does.
	if aw, ok := mapsService.(interface {
		SetHexArt(ctx context.Context, mapID, art string) error
	}); ok {
		hexService.SetArtWriter(aw.SetHexArt)
	}
	// Fog on a whole-map layer changes who may fetch the original picture, so a
	// toggle drops the cached answers; the map service exposes the drop by
	// assertion, as other optional wiring here does.
	if inv, ok := mapsService.(interface{ InvalidateMapPictures(campaignID string) }); ok {
		hexService.SetPictureInvalidator(inv.InvalidateMapPictures)
	}
}

// wireMapShadows gives the map service its shadow source. A named helper so a
// test can prove the production wiring sets it; without it every player would
// see every pin.
func wireMapShadows(mapsService maps.MapService, drawingService maps.DrawingService) {
	mapsService.SetShadowLookup(drawingService)
}

// wireMapPictures gives the map service the media plugin's originals to render
// player copies from, and the media server the check that refuses the original
// of a shadowed map to anyone who may not see under the shadows. A named helper
// so a test can prove the production wiring sets both; without the guard the
// original stays one right-click away.
//
// Copies are cached in a sibling of the media root, not inside it: the media
// orphan sweep deletes every file under the root that has no database row.
func wireMapPictures(mapsService maps.MapService, mediaService media.MediaService, mediaHandler *media.Handler, mediaPath string) {
	mapsService.SetPlayerImageSource(&mapImageSourceAdapter{svc: mediaService},
		filepath.Join(filepath.Dir(filepath.Clean(mediaPath)), "map-player-images"))
	mediaHandler.SetMapImageGuard(mapsService)
}

// mapImageSourceAdapter reads a campaign picture's original bytes for the maps
// plugin, which has no access to media storage itself.
type mapImageSourceAdapter struct {
	svc media.MediaService
}

// ReadImage returns the file's bytes only for an image that belongs to the
// campaign, so a map cannot be pointed at another campaign's media.
func (a *mapImageSourceAdapter) ReadImage(ctx context.Context, campaignID, mediaID string) ([]byte, error) {
	file, err := a.svc.GetByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if file.CampaignID == nil || *file.CampaignID != campaignID || !file.IsImage() || file.IsBound() {
		return nil, apperror.NewNotFound("media file not found")
	}
	data, err := os.ReadFile(a.svc.FilePath(file))
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return data, nil
}

// newMapEventPublisher builds the WebSocket publisher with its shadow lookup.
func newMapEventPublisher(bus ws.EventBus, drawingService maps.DrawingService) *mapEventPublisherAdapter {
	return &mapEventPublisherAdapter{bus: bus, shadows: drawingService, frames: drawingService}
}

// underShadow reports whether an event about a pin or drawing on mapID must be
// restricted to DM-equivalent clients because the item lies under a shadow
// area. The publisher has no request context, so it uses a short bounded one;
// on a lookup failure it answers true, because withholding a live update is
// recoverable and leaking a hidden pin is not.
//
// The hub gates by DM-equivalence, which is exactly who the HTTP paths exempt
// (owner or co-DM); scribes are hidden like players on both.
func (a *mapEventPublisherAdapter) underShadow(mapID string, check func([]maps.ShadowArea) bool) bool {
	if a.shadows == nil {
		slog.Error("maps: shadow lookup not wired; restricting map events to DMs", slog.String("map_id", mapID))
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	areas, err := a.shadows.ShadowAreas(ctx, mapID)
	if err != nil {
		slog.Error("maps: shadow lookup failed while publishing; restricting event to DMs",
			slog.String("map_id", mapID), slog.Any("error", err))
		return true
	}
	return check(areas)
}

// underFog is underShadow for hex fog: it reports whether an event about a pin
// or drawing on mapID must be restricted to DM-equivalent clients because the
// item lies in unexplored hexes. Same fail-closed rule: an unwired or failing
// lookup restricts.
func (a *mapEventPublisherAdapter) underFog(mapID string, check func(*maps.FogMask) bool) bool {
	if a.fog == nil {
		slog.Error("maps: hex fog lookup not wired; restricting map events to DMs", slog.String("map_id", mapID))
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mask, err := a.fog.FogMask(ctx, mapID)
	if err != nil {
		slog.Error("maps: hex fog lookup failed while publishing; restricting event to DMs",
			slog.String("map_id", mapID), slog.Any("error", err))
		return true
	}
	return mask != nil && check(mask)
}

// PublishHexChanged announces a hex write. The message carries the map id, the
// layer version and, only when the service judged it safe, the party's path:
// never a cell, a name or a note. Clients refetch the role-filtered read, so
// the audience rules live in one place. It goes to every client of the
// campaign: the version tells a viewer who cannot see the layer nothing.
func (a *mapEventPublisherAdapter) PublishHexChanged(campaignID, mapID string, version uint64, partyPath []maps.HexKey) {
	if campaignID == "" || a.bus == nil {
		return
	}
	payload := map[string]any{"map_id": mapID, "version": version}
	if len(partyPath) > 0 {
		payload["party_path"] = partyPath
	}
	a.bus.Publish(ws.NewMessage(ws.MsgHexChanged, campaignID, mapID, payload))
}

// Kinds carried by PublishItemsChanged; the viewer maps each to a refetch.
const (
	mapItemsMarkers  = "markers"
	mapItemsDrawings = "drawings"
	mapItemsShadows  = "shadows"
)

// publishItemsChanged announces that something of one kind changed on a map.
// It carries ids only: the viewer refetches through the role-filtered reads,
// so DM-only, shadow and hex-fog rules stay decided in one place and nothing
// secret rides this message. By default it goes to every client of the
// campaign; creating or deleting a DM-only or rule-restricted pin or drawing
// passes that item's audience (dmOnly, rules) so players are not told that a
// hidden thing appeared or vanished. Updates and shadows stay open: a shadow
// changes what players see, and an update may move an item into view. No
// token notice exists: tokens have no layer on the web viewer, and a notice
// per hidden drag would leak movement. It is
// sent just before the per-item event (which stays audience-gated) because the
// per-item event is not enough for a page to stay right: it cannot say what a
// shadow or the fog now covers or uncovers.
func (a *mapEventPublisherAdapter) publishItemsChanged(campaignID, mapID, kind string, dmOnly bool, rules *maps.VisibilityRules) {
	if campaignID == "" || mapID == "" || a.bus == nil {
		return
	}
	msg := ws.NewMessage(ws.MsgMapItemsChanged, campaignID, mapID, map[string]any{
		"map_id": mapID,
		"kind":   kind,
	})
	msg.RequiresDM = dmOnly
	if rules != nil {
		msg.AllowedUsers = rules.AllowedUsers
		msg.DeniedUsers = rules.DeniedUsers
	}
	a.bus.Publish(msg)
}

// publishWithAudience wraps ws.NewMessage with the audience derived from
// the source row: the binary RequiresDM (dm_only) flag, plus — for
// markers and drawings, which carry per-user visibility_rules — the
// explicit allowed/denied user sets. Pulled out so every map sub-resource
// emit funnels the same way — one place to audit, not five.
//
// rules is nil for source kinds that don't carry per-user overrides
// (tokens, fog); those pass dmOnly with rules=nil.
// The hub (internal/websocket/hub.go) is what actually enforces this per
// recipient — this method only computes the audience, per ADR-055 rule 3:
// broadcasting to everyone and hoping the client hides it is forbidden.
func (a *mapEventPublisherAdapter) publishWithAudience(msgType ws.MessageType, campaignID, resourceID string, payload any, dmOnly bool, rules *maps.VisibilityRules) {
	msg := ws.NewMessage(msgType, campaignID, resourceID, payload)
	msg.RequiresDM = dmOnly
	if rules != nil {
		msg.AllowedUsers = rules.AllowedUsers
		msg.DeniedUsers = rules.DeniedUsers
	}
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
	// A picture whose file the fog or a shadow withholds is DM-only on the
	// wire too: its event carries the file id, which is the secret.
	var frame maps.MapFrame
	if a.frames != nil && maps.PictureIsTurned(drawing) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		frame = a.frames.MapFrame(ctx, drawing.MapID)
		cancel()
	}
	dmOnly := drawing.Visibility == "dm_only" ||
		(drawing.DrawingType != maps.DrawingTypeShadow &&
			(a.underShadow(drawing.MapID, func(areas []maps.ShadowArea) bool {
				return maps.DrawingUnderShadow(areas, drawing) || maps.ShadowWithholdsImageOf(areas, drawing, frame)
			}) ||
				a.underFog(drawing.MapID, func(f *maps.FogMask) bool { return f.HidesDrawing(drawing) || f.WithholdsImageOf(drawing) })))
	// Shadows have their own kind so a viewer can tell that pins under or
	// uncovered by them need a refetch too; pictures count as drawings.
	kind := mapItemsDrawings
	if drawing.DrawingType == maps.DrawingTypeShadow {
		kind = mapItemsShadows
	}
	rules := maps.ParseVisibilityRules(drawing.VisibilityRules)
	if kind == mapItemsShadows || eventType == "updated" {
		a.publishItemsChanged(campaignID, drawing.MapID, kind, false, nil)
	} else {
		a.publishItemsChanged(campaignID, drawing.MapID, kind, dmOnly, rules)
	}
	a.publishWithAudience(msgType, campaignID, drawing.ID, drawing, dmOnly, rules)
}

// PublishTokenEvent translates map token domain events into WebSocket messages.
// Tokens flagged is_hidden are GM-only — same gate as the SQL filter in
// drawing_repository.ListTokens — and so are tokens in unexplored hexes, the
// gate DrawingService.ListTokens adds.
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
	dmOnly := token.IsHidden || a.underFog(token.MapID, func(f *maps.FogMask) bool { return f.HidesToken(token) })
	a.publishWithAudience(msgType, campaignID, token.ID, token, dmOnly, nil)
}

// PublishTokenPositionEvent broadcasts a token position update via WebSocket.
// Gated on isHidden and the fog exactly like PublishTokenEvent, so neither a
// GM-only token's live drag position nor a token walking in unexplored land
// reaches a non-GM client.
func (a *mapEventPublisherAdapter) PublishTokenPositionEvent(campaignID, mapID, tokenID string, x, y float64, isHidden bool) {
	if campaignID == "" {
		return
	}
	dmOnly := isHidden || a.underFog(mapID, func(f *maps.FogMask) bool { return f.HidesPoint(x, y) })
	a.publishWithAudience(ws.MsgTokenMoved, campaignID, tokenID, map[string]float64{
		"x": x,
		"y": y,
	}, dmOnly, nil)
}

// PublishLayerEvent broadcasts a map layer event via WebSocket. Layers
// don't carry a visibility flag — they're z-order containers — so layer
// events are everyone-visible. Hiding individual drawings/tokens on a
// layer happens via their own visibility/is_hidden gates. eventType drives
// the message type so clients can discriminate create/update/delete and
// apply a targeted local mutation instead of refetching the full layer list.
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
// eventType drives the message type so clients can dispatch without
// parsing the payload; the redundant "event" key in the payload stays
// for backwards compatibility with the existing handler.
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
	a.publishWithAudience(msgType, campaignID, mapID, payload, true, nil)
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
	dmOnly := marker.IsDMOnly() ||
		a.underShadow(marker.MapID, func(areas []maps.ShadowArea) bool { return maps.MarkerUnderShadow(areas, marker) }) ||
		a.underFog(marker.MapID, func(f *maps.FogMask) bool { return f.HidesMarker(marker) })
	rules := maps.ParseVisibilityRules(marker.VisibilityRules)
	if eventType == "updated" {
		a.publishItemsChanged(campaignID, marker.MapID, mapItemsMarkers, false, nil)
	} else {
		a.publishItemsChanged(campaignID, marker.MapID, mapItemsMarkers, dmOnly, rules)
	}
	a.publishWithAudience(msgType, campaignID, marker.ID, marker, dmOnly, rules)
}

// (The campaigns show page lazy-loads the Foundry banner via
// foundry_vtt's /foundry-vtt/show-banner-fragment route, which owns
// the adapter shape internally.)

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

// SetFoundryModulePinMode delegates to the campaigns service, consumed by
// the owner-side UI's always-promote checkbox.
func (a *foundryCampaignSettingsAdapter) SetFoundryModulePinMode(ctx context.Context, campaignID, mode string) error {
	return a.svc.SetFoundryModulePinMode(ctx, campaignID, mode)
}

// GetFoundryModulePinMode delegates to the campaigns service, consumed by
// OwnerTabData population so the owner-side templ renders the right
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
	// An owner who switched module-update emails off gets no address, which
	// the notify path already reads as "skip the email, keep the banner".
	if len(a.authSvc.AllowedRecipients(ctx, []string{user.ID}, notifyprefs.ModuleUpdates, notifyprefs.Email)) == 0 {
		return "", "", nil
	}
	display := user.DisplayName
	if display == "" {
		display = user.Email
	}
	return user.Email, display, nil
}

// packageOwnerReminder lets the admin Packages page remind an owner that a
// version is waiting, through the Foundry module's own notify path (an audit
// event and a best-effort email). It lives here so the packages plugin never
// imports the Foundry one.
type packageOwnerReminder struct {
	fvtt foundry_vtt.Service
}

// RemindOwner reminds the owner of a Foundry module campaign; other package
// types have no way to reach an owner yet.
func (r packageOwnerReminder) RemindOwner(ctx context.Context, pkg *packages.Package, campaignID, version string, actor packages.ActorInfo) error {
	if pkg.Type != packages.PackageTypeFoundryModule {
		return apperror.NewBadRequest("owners of this package cannot be reminded")
	}
	return r.fvtt.NotifyCampaignOfUpdate(ctx, campaignID, version, actor.UserID, actor.IP, actor.UserAgent)
}

// avatarUploaderAdapter wraps media.MediaService to implement the
// auth.AvatarUploader interface without creating a circular import: media
// already imports auth (for auth.GetUserID and friends), so auth cannot
// import media back. Mirrors media.Handler.Upload's own signed-vs-unsigned
// URL logic so a freshly uploaded avatar is immediately displayable the
// same way a freshly uploaded entity image is.
type avatarUploaderAdapter struct {
	svc    media.MediaService
	signer *media.URLSigner // nil when no signing secret is configured
}

// UploadAvatar stores fileBytes as userID's avatar with usage_type "avatar"
// and no campaign.
func (a *avatarUploaderAdapter) UploadAvatar(ctx context.Context, userID string, fileBytes []byte, originalName, mimeType string) (string, string, error) {
	file, err := a.svc.Upload(ctx, media.UploadInput{
		UploadedBy:   userID,
		OriginalName: originalName,
		MimeType:     mimeType,
		FileSize:     int64(len(fileBytes)),
		UsageType:    media.UsageAvatar,
		FileBytes:    fileBytes,
	})
	if err != nil {
		return "", "", err
	}
	if a.signer != nil {
		return file.ID, a.signer.Sign(file.ID, media.ViewerSession(userID), media.SignedURLTTL), nil
	}
	return file.ID, "/media/" + file.ID, nil
}

// DeleteAvatarMedia removes mediaID's row and file, but only if it still
// belongs to userID and is still usage_type "avatar" -- never someone
// else's file, and never one repurposed for something else since it was
// last a user's avatar. A media id that no longer exists or fails either
// check is treated as nothing-to-do rather than an error, since the caller
// (auth.authService, after replacing or clearing an avatar) always calls
// this as best-effort cleanup of what the column used to point at.
func (a *avatarUploaderAdapter) DeleteAvatarMedia(ctx context.Context, userID, mediaID string) error {
	file, err := a.svc.GetByID(ctx, mediaID)
	if err != nil {
		return nil
	}
	if file.UploadedBy != userID || file.UsageType != media.UsageAvatar {
		return nil
	}
	return a.svc.Delete(ctx, mediaID)
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

// MemberRole returns the caller's membership role in the campaign, or
// campaigns.RoleNone if they are not a member (including "campaign doesn't
// exist" and any other lookup error — fail closed, never guess a role).
func (a *mediaMemberCheckerAdapter) MemberRole(campaignID, userID string) int {
	member, err := a.svc.GetMember(context.Background(), campaignID, userID)
	if err != nil || member == nil {
		return int(campaigns.RoleNone)
	}
	return int(member.Role)
}

// IsUserDmGranted reports whether the campaign Owner has granted the user
// co-DM/dm_only visibility (ADR-058: the media plugin needs the SAME
// promotion campaigns.CampaignContext.VisibilityRole() applies elsewhere,
// but has no *CampaignContext to call it on — /media/:id carries no
// :campaignId to hang campaigns.RequireCampaignAccess off of). Delegates to
// the campaign service's own IsUserDmGranted rather than re-deriving the
// DmGrantIDs check here, so this stays a thin signal, not a second copy of
// that predicate. Any lookup error resolves to false (not granted) — the
// safe direction, since a wrongly-withheld promotion only lowers a
// viewer's role, never raises it.
func (a *mediaMemberCheckerAdapter) IsUserDmGranted(campaignID, userID string) bool {
	granted, err := a.svc.IsUserDmGranted(context.Background(), campaignID, userID)
	if err != nil {
		return false
	}
	return granted
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
// entities repo directly (plugin isolation), closing anonymous private-entity
// leaks and cross-campaign IDOR.
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

// npcEntityTypeFinderAdapter implements npcs.EntityTypeFinder from the page
// types the owner listed as NPCs, so the NPC section lists exactly those.
type npcEntityTypeFinderAdapter struct {
	lists entities.CharacterListReader
}

// FindCharacterTypeIDs returns the campaign's NPC page types.
func (a *npcEntityTypeFinderAdapter) FindCharacterTypeIDs(ctx context.Context, campaignID string) ([]int, error) {
	return a.lists.NPCTypeIDs(ctx, campaignID)
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

// npcTagListerAdapter wraps tags.TagService to implement npcs.TagLister so NPC
// cards carry tags and the Characters page can offer a tag filter. dm_only tags
// are included only when the caller asks (the npcs handler decides by role).
type npcTagListerAdapter struct {
	svc tags.TagService
}

// ListTagsForEntities batch-fetches tags for the given entities.
func (a *npcTagListerAdapter) ListTagsForEntities(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]npcs.TagInfo, error) {
	tagsMap, err := a.svc.GetEntityTagsBatch(ctx, entityIDs, includeDmOnly)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]npcs.TagInfo, len(tagsMap))
	for eid, tagList := range tagsMap {
		infos := make([]npcs.TagInfo, len(tagList))
		for i, t := range tagList {
			infos[i] = npcs.TagInfo{ID: t.ID, Name: t.Name, Slug: t.Slug, Color: t.Color}
		}
		result[eid] = infos
	}
	return result, nil
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

// armoryTagListerAdapter wraps tags.TagService to implement armory.TagLister,
// so gallery cards carry their tags. Whether GM-only tags are included is the
// caller's decision, passed through.
type armoryTagListerAdapter struct {
	svc tags.TagService
}

// ListTagsForEntities batch-fetches tags for the given entities.
func (a *armoryTagListerAdapter) ListTagsForEntities(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]armory.TagInfo, error) {
	tagsMap, err := a.svc.GetEntityTagsBatch(ctx, entityIDs, includeDmOnly)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]armory.TagInfo, len(tagsMap))
	for eid, tagList := range tagsMap {
		infos := make([]armory.TagInfo, len(tagList))
		for i, t := range tagList {
			infos[i] = armory.TagInfo{ID: t.ID, Name: t.Name, Slug: t.Slug, Color: t.Color}
		}
		result[eid] = infos
	}
	return result, nil
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

// UpdateMetadataIf delegates the conditional write used for shop stock.
func (a *armoryRelationMetadataAdapter) UpdateMetadataIf(ctx context.Context, id int, expected, metadata json.RawMessage) (bool, error) {
	return a.svc.UpdateMetadataIf(ctx, id, expected, metadata)
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

// entityMediaVerifierAdapter wraps media.MediaService to implement
// entities.MediaCampaignVerifier. Used by entityService.UpdateImage /
// UpdateCoverImage to confirm a media file exists AND lives in the
// entity's own campaign before writing it into an image field — the
// same cross-campaign IDOR that entityMapVerifierAdapter closes for
// map_id.
type entityMediaVerifierAdapter struct {
	svc media.MediaService
}

// MediaExistsInCampaign returns true only when the media file exists AND
// its CampaignID matches. Not-found is a clean false, not an error — the
// caller only wants to know "is this a valid choice?".
func (a *entityMediaVerifierAdapter) MediaExistsInCampaign(ctx context.Context, mediaID, campaignID string) (bool, error) {
	f, err := a.svc.GetByID(ctx, mediaID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	if f == nil || f.IsBound() {
		return false, nil
	}
	return f.CampaignID != nil && *f.CampaignID == campaignID, nil
}

// mapMediaVerifierAdapter wraps media.MediaService to implement
// maps.MediaVerifier. A picture placed on a map must be an image of the map's
// own campaign, or one campaign could pull another's artwork onto its map by
// guessing a media id.
type mapMediaVerifierAdapter struct {
	svc media.MediaService
}

// ImageInCampaign is true only for an existing image file of the campaign.
// Not-found is a clean false so the caller answers the same for "no such
// file" and "someone else's file".
func (a *mapMediaVerifierAdapter) ImageInCampaign(ctx context.Context, mediaID, campaignID string) (bool, error) {
	f, err := a.svc.GetByID(ctx, mediaID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	if f == nil || f.IsBound() || f.CampaignID == nil || *f.CampaignID != campaignID {
		return false, nil
	}
	return strings.HasPrefix(f.MimeType, "image/"), nil
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
func (a *armoryBuyerAccessAdapter) CanUserActAsBuyer(ctx context.Context, campaignID, entityID, userID string, role int) (bool, error) {
	ent, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	if ent.CampaignID != campaignID {
		return false, nil
	}
	perm, err := a.svc.CheckEntityAccess(ctx, entityID, role, userID)
	if err != nil {
		return false, err
	}
	if perm == nil {
		return false, nil
	}
	return perm.CanEdit, nil
}

// armoryShopCheckerAdapter wraps entities.EntityService to implement
// armory.ShopEntityChecker: an entity counts as a shop only when it is in the
// campaign and its entity type slug is "shop".
type armoryShopCheckerAdapter struct {
	svc entities.EntityService
}

// IsShopInCampaign reports false (not an error) for a missing entity.
func (a *armoryShopCheckerAdapter) IsShopInCampaign(ctx context.Context, campaignID, entityID string) (bool, error) {
	ent, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return ent.CampaignID == campaignID && ent.TypeSlug == "shop", nil
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
		ID:             rel.ID,
		Metadata:       rel.Metadata,
		CampaignID:     rel.CampaignID,
		SourceEntityID: rel.SourceEntityID,
		TargetEntityID: rel.TargetEntityID,
		RelationType:   rel.RelationType,
		DmOnly:         rel.DmOnly,
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
// field, letting JSON-only system packages ship page-level renderers
// without shipping Go: the manifest declares {slug, widget} pairs, and
// the renderer emits the widget mount point and lets boot.js take over.
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

	// wsRevoker is wired into syncapi, addons and campaigns below, well
	// before wsHub is constructed further down — see wsRevokerHolder.
	wsRevoker := &wsRevokerHolder{}

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

	// Register smtp in the App's metadata registry. HealthCheck wraps the
	// existing PluginHealth registry lookup for a uniform health signal.
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

	// Pages saved before search_text stopped indexing GM-only content still
	// carry it until edited; recompute those rows. Idempotent, so it is a
	// no-op on later boots; detached so a large campaign can't stall startup.
	go func() {
		n, err := entities.ReindexSecretSearchText(a.ShutdownCtx, entityRepo)
		if err != nil {
			slog.Warn("entities: search_text reindex stopped", slog.Any("error", err), slog.Int("rewritten", n))
			return
		}
		if n > 0 {
			slog.Info("entities: search_text reindexed", slog.Int("rewritten", n))
		}
	}()

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

	// Boot reconcilers for entity_types fields, run SERIALLY in a single
	// goroutine. They write only the fields column; the one layout_json
	// reconciler (placePageExtrasOnce) runs synchronously later in boot, so
	// no two reconcilers rewrite the same layout. A new layout_json
	// reconciler must run beside that one, not in a goroutine of its own.
	// Each step is idempotent; a failure is logged and the chain continues.
	go func() {
		ctx := context.Background()

		// Converge gm_only field flags from installed system manifests onto
		// existing types so the GM-field egress filter covers characters
		// created before the manifest carried gm_only.
		reconcileFieldGMFlags(ctx, entityService)

		// Same convergence for owner_only field flags, so backstory-style
		// fields become owner-private on types created before the manifest
		// carried owner_only.
		reconcileFieldOwnerOnlyFlags(ctx, entityService)

		// Same for play blocks, so an installed system's play declarations
		// reach types created before it declared them.
		reconcileFieldPlay(ctx, entityService)
	}()

	// Campaigns plugin: CRUD, membership, ownership transfer.
	// EntityService is passed as EntityTypeSeeder to seed defaults on campaign creation.
	userFinder := campaigns.NewUserFinderAdapter(authRepo)
	campaignRepo := campaigns.NewCampaignRepository(a.DB)
	campaignService := campaigns.NewCampaignService(campaignRepo, userFinder, smtpService, entityService, a.Config.BaseURL)
	// Drops a user's live sockets when their co-DM grant is pulled, so a
	// stale IsDmGranted resolved at connect time can't outlive the grant.
	// See wsRevokerHolder — wsHub itself is constructed further down.
	campaignService.SetConnectionRevoker(wsRevoker)

	// One-time, idempotent boot reconciler: convert any campaign still on the
	// legacy sidebar model onto the unified items model. Runs synchronously
	// before serving so a straggler never renders the default sidebar in
	// place of its saved order.
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
	// page's mini→full launch enhancement).
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

	// Which page types are characters and which are NPCs is the owner's
	// choice, kept in the campaign settings; a new campaign starts with the
	// types its defaults provide.
	characterListService := entities.NewCharacterListService(entityService, &characterListStore{camps: campaignService})
	campaignService.SetCharacterListSeeder(characterListService)
	entityHandler.SetCharacterLists(characterListService)

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

	// The auth service reads the site registration mode from settings and
	// validates invite-only signups against live campaign invites. Both
	// deps are optional at the auth layer (nil ⇒ open).
	auth.ConfigureRegistrationGate(authService, settingsService, &registrationInviteCheckerAdapter{invites: inviteService})

	// Migration 26 added media_files.content_hash for per-campaign upload
	// dedup. Existing rows from before the migration have NULL hashes —
	// run the backfill in a detached goroutine so a campaign with
	// thousands of legacy media files doesn't stall startup. New uploads
	// always populate the hash inline; this only catches the gap.
	// Uses a.ShutdownCtx (not context.Background()) so the backfill stops
	// on graceful shutdown instead of continuing to work against a closing
	// DB connection (#711); BackfillContentHashes already checks ctx.Done()
	// between batches.
	go func() {
		n, err := mediaService.BackfillContentHashes(a.ShutdownCtx, 100)
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

	// Initialize HMAC URL signer for secure media access. The same secret
	// feeds foundry_vtt.NewTokenSigner below, where it signs per-campaign
	// manifest tokens Foundry stores indefinitely — an in-memory-only secret
	// discarded on restart would silently invalidate every outstanding
	// token, so LoadOrInitSigningSecret persists the auto-generated one.
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

	// Avatar uploads go through the media pipeline: the auth plugin's upload
	// handler never touches disk itself, so it gets the same magic-byte
	// validation, EXIF stripping/re-encode and disk-space checks every other
	// upload gets (not the per-campaign storage quota — an avatar has no
	// campaign; auth/routes.go rate-limits the route instead). Wired here
	// (rather than at auth.NewHandler) because it needs mediaService, and
	// for a display-ready URL, the same signer media's own Upload handler
	// uses.
	avatarUploader := &avatarUploaderAdapter{svc: mediaService, signer: urlSigner}
	auth.ConfigureAvatarUploader(authService, avatarUploader)

	// One-time, idempotent startup migration for avatar_path rows still
	// pointing at a dead legacy /uploads/avatars/ web path: moves the file
	// into the media store if it's still there (rare — that directory lived
	// outside the Docker volume), or clears the column so the default avatar
	// shows instead of a permanent 404 (the common case). Best-effort: logs
	// and never blocks startup.
	if moved, cleared, err := auth.ReconcileLegacyAvatarPaths(context.Background(), authRepo, avatarUploader, filepath.Join("uploads", "avatars")); err != nil {
		slog.Error("auth: legacy avatar path reconciler failed; some legacy avatar_path rows may still point at the dead /uploads/ path",
			slog.String("error", err.Error()))
	} else if moved > 0 || cleared > 0 {
		slog.Info("auth: legacy avatar path reconciler complete", slog.Int("moved", moved), slog.Int("cleared", cleared))
	}

	// Wire campaign membership checker for private media access control.
	mediaHandler.SetMemberChecker(&mediaMemberCheckerAdapter{svc: campaignService})

	// ADR-058: a picture inherits the visibility of the pages that use it.
	// Reuses the same entityVisibilityFilterAdapter sessions, npcs and
	// armory wire above/below — never a second copy of the predicate.
	mediaHandler.SetEntityVisibilityFilter(&entityVisibilityFilterAdapter{svc: entityService})

	// Same two seams, wired onto the service too, so mediaService.Upload
	// can decide whether a content-hash dedup match is safe to merge — a
	// decision made deep inside Upload, before any HTTP-layer check, so it
	// applies to every caller (notes attachments, campaign backdrops, the
	// Foundry sync API), not just the /media/upload route.
	mediaService.SetMemberChecker(&mediaMemberCheckerAdapter{svc: campaignService})
	mediaService.SetEntityVisibilityFilter(&entityVisibilityFilterAdapter{svc: entityService})

	// Caches the entity-scoped access decision per (file, viewer) so a
	// lookup isn't repeated on every image request. Nil-safe if Redis init
	// failed.
	mediaHandler.SetCache(a.Redis)

	media.RegisterRoutes(e, mediaHandler, authService, resolveMaxUpload, a.Config.Upload.ServeRateLimit)
	// Campaign media routes registered after addon service init (needs media-gallery addon gating).

	// Admin plugin: site-wide management (users, campaigns, SMTP settings, storage).
	adminHandler := admin.NewHandler(authRepo, campaignService, smtpService)
	// Each admin's own sidebar pins; the sidebar's data names which pages can
	// be pinned, and the layout injector below reads the pins per request.
	adminNavPinService := admin.NewAdminNavPinService(admin.NewAdminNavPinRepository(a.DB), layouts.AdminNavPinnableHrefs())
	adminHandler.SetNavPinService(adminNavPinService)
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

	// Site-wide admin change log, shown on admin Home. Other plugins write to
	// it through the adapter so none of them import admin.
	adminActivityService := admin.NewActivityService(admin.NewActivityRepository(a.DB))
	adminHandler.SetActivityService(adminActivityService)
	adminActivity := adminActivityAdapter{svc: adminActivityService}
	smtpHandler.SetActivityRecorder(adminActivity)
	adminGroup := admin.RegisterRoutes(e, adminHandler, authService, smtpHandler)

	// Admin Backup plugin: lists backup artifacts and exposes a "Run
	// backup" button that shells out to scripts/backup.sh under the
	// configured timeout.
	backupSvc := backup.NewService(backup.Config{
		ScriptPath: a.Config.BackupScriptPath,
		BackupDir:  a.Config.BackupDir,
	})
	backupHandler := backup.NewHandler(backupSvc)
	backupHandler.SetActivityRecorder(adminActivity)
	backupHandler.SetDownloadAuth(signingSecret, auth.GetUserID)
	backupHandler.SetScheduleStore(settingsRepo)
	backup.RegisterRoutes(adminGroup, backupHandler, auth.RequireReauth(authService))

	// Daily backup, when the owner turns it on under Admin > Backup.
	go backup.NewScheduler(backupSvc, settingsRepo, backupFailureMailer{users: authRepo, mail: smtpService}).Run(a.ShutdownCtx)

	// Admin Restore plugin: lists backup manifests in BACKUP_DIR and
	// shells out to scripts/restore.sh under a typed-RESTORE
	// confirmation. Reverses ADR-035's "sysadmin-only" stance — see
	// ADR-036 for the policy reasoning. Always registered: BackupDir
	// defaults to /app/data/backups in config.Load, so
	// restore.NewService(BackupDir=="") is unreachable from production
	// wiring.
	restoreSvc := restore.NewService(restore.Config{
		ScriptPath: a.Config.RestoreScriptPath,
		BackupDir:  a.Config.BackupDir,
	})
	restoreHandler := restore.NewHandler(restoreSvc)
	restoreHandler.SetActivityRecorder(adminActivity)
	restore.RegisterRoutes(adminGroup, restoreHandler, auth.RequireReauth(authService))

	// Settings plugin route registration. The service + repo were
	// constructed earlier (above the media routes) so the body-limit
	// middleware can read live settings — see the resolveMaxUpload
	// closure. Per-user/campaign quotas wired via SetStorageLimiter
	// up there too. Here we just register the admin HTTP routes.
	settingsHandler := settings.NewHandler(settingsService)
	settingsHandler.SetActivityRecorder(adminActivity)
	settings.RegisterRoutes(adminGroup, settingsHandler)

	// Design Lab: admin-only page hosting the dynamic-surface demo (a live
	// character sheet built by the Chronicle.surface frame).
	designLabHandler := designlab.NewHandler()
	designlab.RegisterRoutes(adminGroup, designLabHandler)

	// Wire settings service into admin handler for the combined storage page.
	adminHandler.SetSettingsDeps(settingsService)

	// Site look: the admin page saves through the settings service and the
	// media adapter; new campaigns read the same look through the campaigns
	// service's narrow interface.
	adminHandler.SetSiteLookService(admin.NewSiteLookService(settingsService, &siteMediaAdapter{svc: mediaService}))
	campaignService.SetSiteLookSource(settingsService)

	// Addons plugin: extension framework with per-campaign enable/disable toggles.
	addonRepo := addons.NewAddonRepository(a.DB)
	addonService := addons.NewAddonService(addonRepo)
	// A member's own sidebar pins are checked against the sidebar they see,
	// which only this composition root can draw.
	campaignService.SetNavSectionsSource(&navSectionsSource{entities: entityService, addons: addonService})
	// Drops live Foundry sockets when the Sync API addon is switched off
	// for a campaign, so a socket that authenticated while it was on can't
	// keep receiving after it's off. See wsRevokerHolder.
	addonService.SetConnectionRevoker(wsRevoker)
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
	// Move a system's character sheet off an empty duplicate type onto the
	// default Characters type; idempotent and never deletes.
	reconcileCharacterPresetHome(context.Background(), entityService)
	addonService.SetSystemFinder(&systemManifestFinderAdapter{})
	addonHandler := addons.NewHandler(addonService)
	addonHandler.SetActivityRecorder(adminActivity)
	addons.RegisterAdminRoutes(adminGroup, addonHandler, auth.RequireReauth(authService))
	addons.RegisterCampaignRoutes(e, addonHandler, campaignService, authService)

	// Campaign media browser routes (gated behind media-gallery addon).
	media.RegisterCampaignRoutes(e, mediaHandler, campaignService, authService, addonService)

	// Files attached to a page. Who may see, add or remove one is the page's
	// own rule, asked of the entities service, never of text that mentions it.
	pageFileService := media.NewPageFileService(media.NewPageFileRepository(a.DB), mediaService, &pageAccessAdapter{svc: entityService})
	pageFileHandler := media.NewPageFileHandler(pageFileService, mediaService)
	media.RegisterPageFileRoutes(e, pageFileHandler, campaignService, authService, resolveMaxUpload, a.Config.Upload.ServeRateLimit)

	// Wire addon count into admin dashboard for the Extensions stat card.
	adminHandler.SetAddonCounter(addonService)
	adminHandler.SetAddonUsageCounter(addonService)

	// Wire addon checker into entity handler for conditional attributes rendering.
	entityHandler.SetAddonChecker(addonService)
	// Wire the same checker into the entity service so CreateEntityType can
	// gate player-character sub-type creation on the Player Character
	// Claiming addon.
	entityService.SetAddonChecker(addonService)

	// Content extensions: user-installable content packs (calendar presets,
	// entity type templates, entity packs, tag collections, marker icons, themes).
	extRepo := extensions.NewExtensionRepository(a.DB)
	extService := extensions.NewExtensionService(extRepo, a.Config.ExtensionsPath)
	extService.SetMigrationRunner(extensions.NewMigrationRunner(a.DB))
	extHandler := extensions.NewHandler(extService, a.Config.ExtensionsPath)
	extHandler.SetActivityRecorder(adminActivity)
	extensions.RegisterAdminRoutes(adminGroup, extHandler, auth.RequireReauth(authService))
	extensions.RegisterCampaignRoutes(e, extHandler, campaignService, authService)
	extensions.RegisterAssetRoutes(e, extHandler)

	// Let the top-level Extensions hub (campaigns plugin) embed the
	// per-campaign Content Packs list as a card, inverting the import
	// direction so campaigns stays extensions-agnostic at compile time.
	campaignHandler.SetContentPacksCardRenderer(extHandler)

	// Package manager: external repo management for systems and Foundry module.
	pkgRepo := packages.NewPackageRepository(a.DB)
	pkgGitHub := packages.NewGitHubClient()
	pkgService := packages.NewPackageService(pkgRepo, pkgGitHub, a.Config.Upload.MediaPath, a.Config.BaseURL)
	// Wire the pending-submission count into the admin dashboard. Without
	// this, SetPendingCounter is never called and the dashboard's "Pending"
	// stat reads 0 no matter how many submissions are actually queued.
	adminHandler.SetPendingCounter(pkgService)
	// Rescan system registry and re-register addons when a system package
	// is installed or updated, so it appears in the campaign Settings >
	// Game System dropdown immediately without requiring a server restart.
	packages.SetOnSystemInstall(pkgService, func(installPath string) {
		// Taken before the rescan swaps manifests, to tell which sheet fields
		// this install introduces.
		presetsBefore := snapshotPresetFieldKeys()
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
		// take effect without a server restart. Build fully, then publish —
		// no observable half-built state for in-flight requests.
		freshRegistry := entities.NewEntityShowRendererRegistry()
		registerManifestRenderers(freshRegistry)
		entities.SetGlobalEntityShowRendererRegistry(freshRegistry)

		// Converge gm_only and owner_only field flags now that the
		// freshly installed/updated manifest is in the registry, so an
		// updated system's flag changes take effect on existing types
		// without a restart — mirrors the boot-time reconcile.
		reconcileFieldGMFlags(context.Background(), entityService)
		reconcileFieldOwnerOnlyFlags(context.Background(), entityService)
		reconcileFieldPlay(context.Background(), entityService)

		// Add the sheet fields this install introduced to campaigns already
		// using the system; background since it walks every such campaign.
		go reconcileSystemSheetFields(context.Background(), addonService, newPresetApplier(entityService), presetsBefore)
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

	// Wire the plugin registry's embedded static filesystems so host.embedded /
	// host.embedded-contains can list assets that exist ONLY inside the binary.
	// The closure reads the registry when the diagnostic runs, so it does not
	// matter that plugins register (and mountPluginStatic runs) after this line.
	systems.SetEmbeddedAssetsProvider(a.embeddedAssetSets)

	// Wire the merged plugin registries so host.plugins can answer "is this
	// plugin even loaded, and did its schema actually run?". Same late-binding
	// closure as above: the metadata registry is still being filled by the
	// registerPlugin calls further down this function.
	systems.SetHostPluginsProvider(a.hostPluginRows)

	// Wire the in-memory error ring so host.errors / host.errors-summary can
	// answer "what broke overnight?" without shell access to the container.
	// The ring itself is written by app.errorHandler and the panic-recovery
	// middleware and exists from process start; this only grants the read.
	// Until this line runs the diagnostics print "provider not wired" rather
	// than an empty list — "no errors" and "nobody is recording" must never
	// render the same.
	systems.SetRecentErrorsProvider(recentErrorSnapshot)

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
	pkgHandler.SetActivityRecorder(adminActivity)
	pkgOwnerHandler := packages.NewOwnerHandler(pkgService)
	// Public package file serving — always available so Foundry VTT can
	// fetch module.json even when the admin UI is degraded.
	pkgServeHandler := packages.NewServeHandler(pkgService, a.Config.BaseURL)
	packages.SetOnServeInvalidate(pkgService, pkgServeHandler.InvalidateCache)
	packages.RegisterPublicRoutes(e, pkgServeHandler, middleware.RateLimit(300, time.Minute))

	if a.PluginHealth.IsHealthy("packages") {
		packages.RegisterRoutes(adminGroup, pkgHandler, auth.RequireReauth(authService))

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

	// Per-campaign update modes (automatic / stay on a version / ask first).
	// The service holds the game-system binding; the Foundry binding joins
	// below if that plugin is healthy, and its owners are then asked before a
	// campaign moves. Game systems have no screens for it yet. When the
	// packages tables are degraded none of this is wired, and clean-up stays
	// refused because its campaign-versions provider is unset (fail closed).
	var pkgUpdateSvc packages.CampaignUpdateService
	if a.PluginHealth.IsHealthy("packages") {
		pkgUpdateSvc = packages.NewCampaignUpdateService(
			packages.NewCampaignUpdateRepository(a.DB), pkgService, securityService)
		// Admin Packages page: per-campaign state, hold, move.
		pkgHandler.SetCampaignUpdates(pkgUpdateSvc)
		// Clean-up must never delete a version some campaign is on.
		packages.SetCampaignVersionsProvider(pkgService, pkgUpdateSvc.KeptVersions)
		// Freeze pinned / ask-first campaigns when a system installs.
		packages.RegisterPostInstallHook(pkgService,
			packages.NewUpdateModeHook(pkgUpdateSvc, packages.PackageTypeSystem))
	}

	// Drops a disabled account's live sockets in every campaign, so a
	// socket that authenticated before the disable can't keep receiving.
	// See wsRevokerHolder — wsHub itself is constructed further down.
	securityService.SetConnectionRevoker(wsRevoker)

	// Data hygiene scanner: orphan detection and cleanup for media, API keys, stale files.
	hygieneScanner := admin.NewHygieneService(a.DB, mediaRepo, mediaService, a.Config.Upload.MediaPath, securityRepo)
	adminHandler.SetHygieneScanner(hygieneScanner)

	// Site Trash: a deleted campaign and a file clean-up wait here with Undo
	// before anything is removed. The purger is the periodic job that empties
	// items older than the retention setting (an hourly ticker, started the
	// same way as the page Trash purger below).
	trashService := admin.NewTrashService(campaignService, mediaService, hygieneScanner,
		admin.NewTrashBatchRepository(a.DB), settingsService)
	adminHandler.SetTrashService(trashService)
	go trashService.StartPurger(a.ShutdownCtx)

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
	pageFileHandler.SetSecurityLogger(securityService)

	// foundry_vtt sub-plugin: the Foundry-VTT-specific extension to the
	// generic packages plugin. Owns every Foundry-specific behavior:
	// per-campaign signed manifest URLs, per-campaign pinning,
	// chronicle-package.json descriptor reading + the post-install
	// module.json version rewrite, admin "campaigns using v0.1.5" cards.
	// The packages plugin itself has zero Foundry-specific code — Chronicle
	// is fully Foundry-agnostic at the packages layer.

	// Register foundry_vtt in the App's metadata registry. Slug is the
	// canonical external identifier (matches the WS protocol + URL
	// prefix); HealthCheck wraps PluginHealth via the plugin's exported
	// PluginHealthKey const (the Go-package-name underscore form, distinct
	// from PluginSlug).
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
		settingsRepo, // powers the auto-pin install banner
		a.Config.BaseURL,
	)
	fvttHandler := foundry_vtt.NewHandler(fvttService)
	// Also read by the Foundry page's version check; nil without packages.
	var fvttOwnerUpdates foundry_vtt.OwnerUpdates
	fvttHandler.SetActivityRecorder(adminActivity)
	if pkgUpdateSvc != nil {
		// Owners are asked before a new module version reaches their campaign.
		fvttOwnerUpdates = foundry_vtt.NewOwnerUpdates(pkgUpdateSvc, pkgService)
		fvttHandler.SetOwnerUpdates(fvttOwnerUpdates)
		pkgHandler.SetOwnerReminder(packageOwnerReminder{fvtt: fvttService})
	}
	// The campaign show page lazy-loads /foundry-vtt/show-banner-fragment
	// rather than using a banner adapter wire.
	if a.PluginHealth.IsHealthy(foundry_vtt.PluginHealthKey) && a.PluginHealth.IsHealthy("packages") {
		// Fires after every foundry-module typed install and rewrites
		// module.json's version field to match the installed version.
		packages.RegisterPostInstallHook(pkgService, foundry_vtt.NewPostInstallHook())
		// Pins auto-tracking campaigns to the previous version on every
		// foundry-module install so the admin sees the version spread
		// instead of silently bumping everyone.
		packages.RegisterPostInstallHook(pkgService, foundry_vtt.NewAutoPinHook(fvttService))
		// Update modes for the Foundry module read the same pin and pin mode.
		// Registered AFTER the auto-pin hook: it reads the state that hook
		// leaves behind and then handles ask-first campaigns.
		if pkgUpdateSvc != nil {
			pkgUpdateSvc.RegisterBinding(foundry_vtt.NewCampaignBinding(
				fvttService, fvttCampaignAdapter, foundry_vtt.NewCampaignPinLister(a.DB)))
			packages.RegisterPostInstallHook(pkgService,
				packages.NewUpdateModeHook(pkgUpdateSvc, packages.PackageTypeFoundryModule))
		}

		// One-time auto-pin migration for pre-feature campaigns: pins all
		// auto-tracking campaigns to the currently-installed version so
		// future installs trigger the state-preserving AutoPinHook flow
		// instead of silently bumping. Idempotent via a settings key; runs
		// synchronously so it completes before any campaign loads its
		// settings page. Errors abort startup loudly.
		if err := foundry_vtt.AutoPinMigrate(context.Background(), fvttService, settingsRepo); err != nil {
			slog.Error("foundry_vtt autopin migration failed", slog.Any("error", err))
			// Not fatal — migration is best-effort; a future boot retries.
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
		foundry_vtt.RegisterDMTeamRoutes(fvttCampaignAuthed, fvttHandler)

		// Public manifest + download. Same rate limit as the packages
		// public endpoints — manifest hits are frequent (every Foundry
		// update check), the limit needs headroom for moderately-
		// sized deployments.
		foundry_vtt.RegisterPublicRoutes(e, fvttHandler, middleware.RateLimit(300, time.Minute))
	} else {
		slog.Warn("foundry_vtt plugin degraded — routes not registered")
	}

	// Idempotent boot pass for the update modes: held versions agree with the
	// installed version, empty rows go. Best effort; a failure is retried on
	// the next boot and changes nothing a campaign is served.
	if pkgUpdateSvc != nil {
		if res, err := pkgUpdateSvc.Reconcile(context.Background()); err != nil {
			slog.Error("update modes reconcile failed", slog.Any("error", err))
		} else {
			slog.Info("update modes reconciled",
				slog.Int("held_set", res.HeldSet), slog.Int("held_cleared", res.HeldCleared),
				slog.Int64("rows_removed", res.RowsRemoved))
		}
	}

	// Sync API plugin: external tool integration with API key auth,
	// request logging, security monitoring, and admin dashboard.
	syncRepo := syncapi.NewSyncAPIRepository(a.DB)
	syncService := syncapi.NewSyncAPIService(syncRepo)
	// The campaign "Sync API" addon toggle is what gates external Bearer
	// access (REST via syncapi.RequireSyncAPIAddon, WebSocket via
	// AuthenticateKeyForWS). Both read it through this gate; the WS path
	// fails CLOSED if this line is ever dropped, which is the intended
	// direction for a security control.
	syncService.SetAddonGate(addonService)
	// The key's creator must currently be a campaign Owner or the key stops
	// working (REST via syncapi.RequireKeyOwnerStillOwner, WebSocket via
	// AuthenticateKeyForWS). Both read membership through this checker; the
	// WS path fails CLOSED if this line is ever dropped, same direction as
	// the addon gate above.
	syncService.SetMemberChecker(campaignService)
	// Drops live Foundry sockets when a key is revoked, so a socket that
	// authenticated with it can't keep receiving until it happens to
	// reconnect on its own. See wsRevokerHolder.
	syncService.SetConnectionRevoker(wsRevoker)
	// One-time, idempotent startup backfill: enable sync-api for campaigns
	// that already own API keys but have no recorded toggle state, so
	// enforcing the toggle cannot cut off an integration that was working.
	// Only where no campaign_addons row exists — enabled=0 is an owner's
	// decision and is left alone. Best-effort: logs and never blocks startup.
	if n, err := syncapi.ReconcileAddonEnablement(context.Background(), syncService, addonService); err != nil {
		slog.Error("sync-api addon enablement backfill failed; campaigns that already use the "+
			"Sync API may be refused until an owner enables Sync API on the campaign's Game & features page (Manage → Game & features)",
			slog.String("error", err.Error()))
	} else if n > 0 {
		slog.Info("sync-api addon enablement backfill complete", slog.Int("campaigns", n))
	}
	syncHandler := syncapi.NewHandler(syncService)
	// Inject sync mapping service early so the owner dashboard can show sync status.
	syncMappingRepoEarly := syncapi.NewSyncMappingRepository(a.DB)
	syncMappingSvcEarly := syncapi.NewSyncMappingService(syncMappingRepoEarly)
	syncHandler.SetSyncMappingService(syncMappingSvcEarly)

	// Wire the sync-mapping reader into the operator diagnostics so
	// entity.sync-mappings can answer "is this entity linked to a Foundry actor?"
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
	syncHandler.SetActivityRecorder(adminActivity)
	adminHandler.SetAPIAlertCounter(adminAPIAlertCounter{sync: syncService})
	syncHandler.SetBaseURL(a.Config.BaseURL)
	if a.PluginHealth.IsHealthy("syncapi") {
		syncapi.RegisterAdminRoutes(adminGroup, syncHandler, auth.RequireReauth(authService))
		syncapi.RegisterCampaignRoutes(e, syncHandler, campaignService, authService)
	} else {
		slog.Warn("syncapi plugin degraded — routes not registered")
	}

	// Calendar plugin: service + handler over the repositories, routed below
	// (after the rebuild-notice group) once migrations report healthy.
	//
	// CALV5-PLACEHOLDER: V5 must still restore the entity-creator seam and
	// the StaticFS mount for /static/plugins/calendar/. The old per-event
	// RSVP is not coming back: game-night answers live in the sessions
	// plugin, which the calendar reads over HTTP. TODO(#778)
	calendarRepo := calendar.NewCalendarRepository(a.DB)
	// One-time, idempotent: repair Harptos calendars created before the
	// preset's season numbering was fixed. Only rows still exactly equal to
	// the old preset are rewritten; best-effort, never blocks startup.
	if store, ok := calendarRepo.(calendar.HarptosSeasonStore); ok {
		if n, err := calendar.ReconcileHarptosSeasons(context.Background(), store); err != nil {
			slog.Error("calendar: harptos season reconcile failed", slog.Any("error", err), slog.Int("repaired", n))
		} else if n > 0 {
			slog.Info("calendar: harptos season reconcile repaired calendars", slog.Int("calendars", n))
		}
	}
	calendarEventRepo := calendar.NewEventRepository(a.DB)
	calendarKindRepo := calendar.NewEventKindRepository(a.DB)
	calendarWeatherRepo := calendar.NewWeatherRepository(a.DB)
	calendarService := calendar.NewCalendarService(calendarRepo, calendarEventRepo, calendarKindRepo, calendarWeatherRepo)
	// An event's entity_id join reads straight from entities with no
	// visibility filter of its own (calendar.EntityVisibilityGate's doc
	// comment) — reuses the same entityVisibilityFilterAdapter sessions,
	// maps, npcs, armory and media wire above, never a second copy of the
	// predicate. Reached via a type assertion (the CalendarService interface
	// itself stays unchanged for callers that don't need it).
	if gated, ok := calendarService.(interface {
		SetEntityVisibilityGate(calendar.EntityVisibilityGate)
	}); ok {
		gated.SetEntityVisibilityGate(&entityVisibilityFilterAdapter{svc: entityService})
	}
	calendarHandler := calendar.NewHandler(calendarService)
	// Optional UX nicety: suggest the owner's own stored account timezone as
	// the real-world calendar wizard step's starting default (see
	// calendar.Handler.SetTimezoneLookup's own doc comment).
	calendarHandler.SetTimezoneLookup(authService)
	a.registerPlugin(PluginRegistration{
		Slug: calendar.PluginSlug,
		HealthCheck: func() error {
			if a.PluginHealth != nil && !a.PluginHealth.IsHealthy(calendar.PluginSlug) {
				return errors.New("calendar schema unhealthy")
			}
			return nil
		},
	})

	// Plugin body scripts contributed by plugins at registration time so
	// base.templ doesn't hardcode plugin asset paths; injected into every
	// page's Templ context by the LayoutInjector below.
	//
	// ORDER IS LOAD-BEARING: base.templ emits these in slice order with
	// `defer`, which executes in document order — a script reading global
	// state another sets up must come after it.
	//
	// These belong in this registry, not a `<script src>` tag inside a page
	// body: htmx boosted navigation swaps `#main-content` via makeFragment,
	// and with `htmx.config.allowScriptTags = false` that DELETES any
	// `<script>` in the swapped fragment, so a body-embedded tag would work
	// on direct load but silently no-op when reached via the sidebar. This
	// registry emits after `{children...}`, outside the swapped region, so
	// both paths deliver the same scripts; each re-inits on
	// htmx:afterSettle/htmx:load and no-ops when its mount is absent.
	//
	// The calendar's page scripts: calendar_era_blend.js (loaded first)
	// paints the era colours for calendar_view.js and for the structure
	// editor's Era look part (calendar_era_look.js, which mounts on that
	// page's data-widget="calendar_era_look"); calendar_view.js mounts on
	// data-widget="calendar_view" (the calendar's own page, however it was
	// reached) and opens calendar_game_night.js's game night editor,
	// calendar_editor.js self-gates on that
	// mount's data-can-edit="true" and opens calendar_event_drawer.js's full
	// event editor and calendar_weather_sheet.js's weather calendar (both
	// loaded first so they exist when the editor binds; the drawer's
	// repeat-by-rule logic is calendar_rule.js, loaded before it), and
	// calendar_open.js peeks and opens the Calendars page's cards. rulebook.js
	// mounts on the Rules page's data-widget="rulebook" when a system ships a
	// book, and rulebook_editor.js on the editor page's
	// data-widget="rulebook-editor". All are no-ops on every other page, same
	// as every entry here.
	pluginBodyScripts := []string{
		"/static/plugins/" + entities.PluginSlug + "/js/characters.js",
		"/static/plugins/" + entities.PluginSlug + "/js/hero_creator.js",
		"/static/plugins/" + entities.PluginSlug + "/js/own_entries.js",
		"/static/js/widgets/calendar_era_blend.js",
		"/static/js/widgets/calendar_era_look.js",
		"/static/js/widgets/calendar_view.js",
		"/static/js/widgets/calendar_game_night.js",
		"/static/js/widgets/calendar_rule.js",
		"/static/js/widgets/calendar_event_drawer.js",
		"/static/js/widgets/calendar_weather_sheet.js",
		"/static/js/widgets/calendar_editor.js",
		"/static/js/calendar_open.js",
		"/static/js/widgets/rulebook.js",
		"/static/js/widgets/rulebook_editor.js",
		// After boot.js, so Chronicle exists before a token can arrive. It
		// does nothing on pages without #notes-embed.
		"/static/js/notes_embed.js",
	}

	// The sidebar, campaign dashboard and Extensions hub link to
	// /campaigns/:id/apps/calendar: addonURLMap in app.templ keeps that path
	// because the plugin-isolation guard flags a literal "calendar" slug
	// outside this file and the calendar plugin. That path and the legacy
	// /calendar redirect to the calendars list page, which
	// calendar.RegisterRoutes serves below.
	calendarRebuildGroup := e.Group("/campaigns/:id",
		auth.RequireAuth(authService),
		campaigns.RequireCampaignAccess(campaignService),
	)
	calendarRedirectToCalendars := func(c echo.Context) error {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/campaigns/%s/calendars", c.Param("id")))
	}
	calendarRebuildGroup.GET("/apps/calendar", calendarRedirectToCalendars)
	calendarRebuildGroup.GET("/calendar", calendarRedirectToCalendars)

	// calendar.RegisterRoutes is both the JSON API (calendars, events, event
	// kinds, eras, the moon hidden flag), the calendars list/preview pages,
	// the new-calendar wizard and each calendar's own page. If the
	// calendar plugin is unhealthy, none of it registers (the else branch
	// below only logs) and every one of these paths, including
	// /campaigns/:id/calendars itself, 404s rather than showing a notice —
	// unlike /apps/calendar and /calendar above, which redirect there
	// regardless of plugin health (a 302 to a 404 is still clearer than a
	// notice claiming the feature is merely rebuilding).
	//
	// CALV5-PLACEHOLDER: V5 must still restore the public Foundry-facing
	// calendar API (token-verified through fvttService, rate limited 300/min) behind the schema health gate. That
	// public API overlapped syncapi's calendar surface — rebuild only
	// syncapi's, the one the module's contract documents. TODO(#778)
	if a.PluginHealth.IsHealthy(calendar.PluginSlug) {
		calendar.RegisterRoutes(e, calendarHandler, campaignService, authService, addonService)
	} else {
		slog.Warn("calendar plugin degraded — routes not registered")
	}

	// Bestiary plugin: community creature sharing with ratings, favorites, import.
	bestiaryRepo := bestiary.NewBestiaryRepository(a.DB)
	bestiarySvc := bestiary.NewBestiaryService(bestiaryRepo)
	bestiarySvc.SetUserFetcher(&bestiaryUserFetcherAdapter{authSvc: authService})
	bestiarySvc.SetEntityCreator(&bestiaryEntityCreatorAdapter{svc: entityService, campaignSvc: campaignService})
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
	// Reuses the same entityVisibilityFilterAdapter armory, media, npcs and
	// sessions wire, so a marker naming a dm_only/private entity is narrowed
	// by entities' one canonical visibility predicate. Type-asserted like
	// SetBindingCleaner below so the MapService interface stays unchanged.
	if g, ok := mapsService.(interface {
		SetEntityVisibilityGate(maps.EntityVisibilityGate)
	}); ok {
		g.SetEntityVisibilityGate(&entityVisibilityFilterAdapter{svc: entityService})
	}
	mapsHandler := maps.NewHandler(mapsService)
	drawingRepo := maps.NewDrawingRepository(a.DB)
	drawingService := maps.NewDrawingService(drawingRepo)
	hexService := maps.NewHexService(maps.NewHexRepository(a.DB))
	// Pins under a shadow area are withheld from players; the map service asks
	// the drawing service (which owns drawings) where the shadows are.
	wireMapShadows(mapsService, drawingService)
	wireMapPictures(mapsService, mediaService, mediaHandler, a.Config.Upload.MediaPath)
	// Wire the map-existence + same-campaign check used by AssignMap on
	// entities, as a post-construction dependency (mapsService doesn't
	// exist yet when entityService is constructed).
	entityService.SetMapVerifier(&entityMapVerifierAdapter{svc: mapsService})
	// Wire the media-existence + same-campaign check used by
	// UpdateImage/UpdateCoverImage — mediaService already exists by this
	// point (constructed earlier in this function).
	entityService.SetMediaVerifier(&entityMediaVerifierAdapter{svc: mediaService})
	if a.PluginHealth.IsHealthy("maps") {
		maps.RegisterRoutes(e, mapsHandler, campaignService, authService, addonService)
		// The campaign-wide map frame is a "Maps" tab on the Customize page;
		// the campaigns plugin only hosts the tab, maps owns what is in it.
		campaignHandler.RegisterCustomizeTab(mapsHandler.CustomizeTabFactory())
		drawingHandler := maps.NewDrawingHandler(mapsService, drawingService)
		maps.RegisterDrawingRoutes(e, drawingHandler, campaignService, authService, addonService)
		maps.RegisterHexRoutes(e, maps.NewHexHandler(hexService), campaignService, authService, addonService)
	} else {
		slog.Warn("maps plugin degraded — routes not registered")
	}

	// Sessions plugin: game session scheduling, linked entities, RSVP tracking.
	// Entity campaign checker prevents cross-campaign entity linking (IDOR).
	sessionsRepo := sessions.NewSessionRepository(a.DB)
	sessionsService := sessions.NewSessionService(sessionsRepo, &entityCampaignCheckerAdapter{svc: entityService}, &entityVisibilityFilterAdapter{svc: entityService})
	// The calendar's anchor-move preview names the sessions it would re-date
	// through this lookup; without it the owner's warning would always read
	// "no sessions affected".
	if wired, ok := calendarService.(interface {
		SetGameNightsAffectedByAnchorMove(calendar.GameNightsAffectedByAnchorMove)
	}); ok {
		wired.SetGameNightsAffectedByAnchorMove(&gameNightsAnchorMoveAdapter{svc: sessionsService})
	}
	// Keeps Game nights on for campaigns that already use them; a recorded
	// owner choice is left alone. Best-effort: logs and never blocks startup.
	if n, err := sessions.ReconcileAddonEnablement(context.Background(), sessionsService, addonService); err != nil {
		slog.Error("game nights addon enablement backfill failed; campaigns that already use game nights "+
			"may not see them until an owner turns on Game nights (Manage → Game & features)",
			slog.String("error", err.Error()))
	} else if n > 0 {
		slog.Info("game nights addon enablement backfill complete", slog.Int("campaigns", n))
	}
	sessionsHandler := sessions.NewHandler(sessionsService)
	sessionsHandler.SetMemberLister(campaignService)
	sessionsHandler.SetMailSender(smtpService, a.Config.BaseURL)
	// People's notification choices (Account settings) filter the bell and
	// the game-night emails.
	sessionsHandler.SetRecipientFilter(authService)
	sessions.ConfigureRecipientFilter(sessionsService, authService)
	// Game-night links open the real-world calendar; with the calendar plugin
	// down they keep going to the Sessions page.
	if a.PluginHealth.IsHealthy(calendar.PluginSlug) {
		sessionsHandler.SetCalendarFinder(&realWorldCalendarFinderAdapter{svc: calendarService})
	}
	// CALV5-PLACEHOLDER: V5 must restore three post-construction setters —
	// SetAvailabilityWriter (member zones + exception dates),
	// SetScheduleReader and SetOwnWeekReader — all nil-safe on the
	// calendar side, so a degraded neighbour never takes the calendar down.
	// TODO(#778)

	if a.PluginHealth.IsHealthy("sessions") {
		sessions.RegisterRoutes(e, sessionsHandler, campaignService, authService, addonService)
	} else {
		slog.Warn("sessions plugin degraded — routes not registered")
	}

	// Timeline plugin: interactive visual timelines with zoom levels and entity grouping.
	timelineRepo := timeline.NewTimelineRepository(a.DB)
	// Wires the calendar selector dropdown, event picker and era
	// visualization bands back to the calendar plugin (calendar-v5 seams,
	// #778) — distinct from SetCalendarEventLinkLister below, restored
	// separately.
	timelineSvc := timeline.NewTimelineService(timelineRepo,
		&calendarListerAdapter{svc: calendarService},
		&calendarEventListerAdapter{svc: calendarService},
		&calendarEraListerAdapter{svc: calendarService},
	)
	// Reuses the same entityVisibilityFilterAdapter maps, media, npcs and
	// sessions wire, so a timeline event or entity-group member naming a
	// dm_only/private entity is narrowed by entities' one canonical
	// visibility predicate. Type-asserted like maps' own wiring above so the
	// TimelineService interface stays unchanged.
	if g, ok := timelineSvc.(interface {
		SetEntityVisibilityGate(timeline.EntityVisibilityGate)
	}); ok {
		g.SetEntityVisibilityGate(&entityVisibilityFilterAdapter{svc: entityService})
	}
	// SetCalendarEventLinkLister restores timeline's calendar-name display
	// and calendar-linked event reads (calendar-v5 seams, #778) — a
	// SEPARATE seam from the three constructor args above. Reached via a
	// type assertion so TimelineService's interface stays unchanged, the
	// same pattern SetBindingCleaner below uses.
	if t, ok := timelineSvc.(interface {
		SetCalendarEventLinkLister(timeline.CalendarEventLinkLister)
	}); ok {
		t.SetCalendarEventLinkLister(&calendarEventLinkListerAdapter{svc: calendarService})
	}
	// Timeline creation (the form and campaign import alike) binds a calendar
	// only once this confirms it is in the same campaign.
	if t, ok := timelineSvc.(interface {
		SetCalendarScope(timeline.CalendarScope)
	}); ok {
		t.SetCalendarScope(&calendarScopeAdapter{svc: calendarService})
	}
	timelineHandler := timeline.NewHandler(timelineSvc)
	timelineHandler.SetMemberLister(campaignService)
	// The timeline page's chart and the dashboard/category preview cards load
	// their scripts on sight, so pages without a timeline never fetch them.
	a.registerPlugin(PluginRegistration{
		Slug:     timeline.PluginSlug,
		StaticFS: echo.MustSubFS(timeline.StaticAssetsFS, "static"),
		Widgets: []PluginWidget{
			{Name: "timeline-viz", Scripts: []string{"js/timeline_viz.js"}},
			{Name: "timeline-widget", Scripts: []string{"js/timeline_widget.js"}},
		},
	})
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
	// Entity-privacy gate: hide private-entity nodes from the graph, and
	// enforce entity privacy + campaign binding on the list.
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
	// Entity-privacy gate: the public posts list must respect entity
	// visibility + campaign binding, like the entity page.
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
	entity_notes.RegisterRoutes(e, entityNotesHandler, campaignService, authService, addonService)

	// Tags widget: campaign-scoped entity tagging (CRUD + entity associations).
	// Created before sync API so the tag service is available for the REST API handler.
	tagRepo := tags.NewTagRepository(a.DB)
	tagService := tags.NewTagService(tagRepo)
	tagHandler := tags.NewHandler(tagService)
	// Entity-privacy gate: the public per-entity tag read must respect
	// entity visibility + campaign binding, like the entity page.
	tagHandler.SetEntityGate(&entityAccessAdapter{svc: entityService})
	// Tag visibility grants: Owner-gated CRUD plus the effective-visibility
	// glance source. The grant service validates grant subjects against the
	// campaign (member/group lookups) and resolves labels for the badge.
	tagGrantRepo := tags.NewTagPermissionRepository(a.DB)
	tagGrantService := tags.NewTagGrantService(tagGrantRepo, tagRepo, campaignService, groupService)
	tagHandler.SetGrantService(tagGrantService)
	tags.RegisterRoutes(e, tagHandler, campaignService, authService)
	// Shared tag adapter: feeds the entities glance (SetTagFetcher below) and the
	// syncapi permissions endpoint (SetTagGrantLister) the same tag-grant view.
	tagFetcherAdapter := &entityTagFetcherAdapter{svc: tagService, grantSvc: tagGrantService}

	// Stashes and moves: the service reaches entities, relations and members
	// only through the adapters in armory_stash_adapters.go. It is built before
	// the sync API so the API's stash endpoints can use it; the event bus does
	// not exist yet, so events bind to it later through stashEvents.
	stashEvents := &armoryStashEventAdapter{}
	// Sharing a hidden item needs the armory's own table; if its migration
	// failed the plugin is degraded and sharing is simply not offered. The
	// activity log is bound once the audit service exists, further down.
	var shareStore armory.ShareStore
	if a.PluginHealth.IsHealthy(armory.AddonSlug) {
		shareStore = armory.NewShareRepository(a.DB)
	} else {
		slog.Warn("armory plugin schema degraded — item sharing is off")
	}
	shareAudit := &armoryShareAuditAdapter{}
	stashRepo := armory.NewStashRepository(a.DB)
	stashDirectory := &armoryStashDirectoryAdapter{svc: entityService, lists: characterListService}
	// The stash directory caches page-type facts for a few seconds; drop them
	// when the owner changes the lists so the change shows at once.
	characterListService.SetChangeHook(stashDirectory.Forget)
	stashSvc := armory.NewStashService(armory.StashDeps{
		Repo:       stashRepo,
		Directory:  stashDirectory,
		Visibility: &entityVisibilityFilterAdapter{svc: entityService},
		Actor:      &armoryCharacterActorAdapter{svc: entityService},
		Fields:     &armoryEntityFieldsAdapter{svc: entityService},
		Relations:  &armoryHasItemAdapter{svc: relService},
		UserNames:  &armoryMemberNamesAdapter{svc: campaignService},
		Events:     stashEvents,
		Handouts:   &armoryHandoutAdapter{maps: mapsService, svc: entityService, dir: stashDirectory},
		Notifier:   &armoryGiveNotifierAdapter{svc: sessionsService},
		Shares:     shareStore,
		Auditor:    shareAudit,
	})
	// A money change made on a sheet (web, Foundry, extension) leaves a line in
	// the character's history, like a move would.
	entityService.SetFieldChangeObserver(&armoryMoneyObserver{history: armory.NewMoneyHistory(
		stashRepo, stashDirectory,
		func(ctx context.Context, campaignID string) bool {
			on, err := addonService.IsEnabledForCampaign(ctx, campaignID, "armory")
			return err == nil && on
		},
		stashEvents,
	)})
	stashAPI := armory.NewStashAPI(stashSvc, &armoryMemberDirectoryAdapter{svc: campaignService})
	stashAPIHandler := syncapi.NewStashAPIHandler(
		&syncStashAPIAdapter{api: stashAPI},
		"armory",
	)
	// Quests for the Foundry module; the quest services attach below, once
	// the quests plugin is built.
	questAPI := &syncQuestAPIAdapter{stash: stashSvc, actors: stashAPI, signer: urlSigner}

	// REST API v1: versioned endpoints for external clients (Foundry VTT, etc.).
	// Authenticates via API keys, not browser sessions.
	syncAPIHandler := syncapi.NewAPIHandler(syncService, entityService, campaignService, relService)
	syncAPIHandler.SetAddonLister(&addonListerAPIAdapter{svc: addonService})
	if urlSigner != nil {
		syncAPIHandler.SetURLSigner(urlSigner)
	}
	// Expose tag-derived grants on the permissions endpoint for Foundry
	// ownership sync, reusing the entities glance adapter.
	syncAPIHandler.SetTagGrantLister(tagFetcherAdapter)
	syncAPIHandler.SetSystemEnabler(addonService)
	// The shop room service is shared with the armory web routes below, so the
	// Foundry module reads the same layout and visibility rules.
	shopRoomService := armory.NewShopRoomService(
		armory.NewShopRoomRepository(a.DB),
		&armoryShopCheckerAdapter{svc: entityService},
		&entityVisibilityFilterAdapter{svc: entityService},
	)
	syncAPIHandler.SetShopRoomReader(shopRoomService, "armory")
	calendarAPIHandler := syncapi.NewCalendarAPIHandler(syncService, calendarService, campaignService)
	mediaAPIHandler := syncapi.NewMediaAPIHandler(syncService, mediaService)
	if urlSigner != nil {
		mediaAPIHandler.SetURLSigner(urlSigner)
	}
	// Needed to resolve the caller's role for ListMedia's Scribe+ gate.
	mediaAPIHandler.SetCampaignService(campaignService)
	mediaAPIHandler.SetMapImageGuard(mapsService)

	// Sync mapping handler for Foundry VTT bidirectional sync.
	// Reuses the sync mapping service created earlier for the owner dashboard.
	syncMappingHandler := syncapi.NewSyncHandler(syncMappingSvcEarly)
	mapAPIHandler := syncapi.NewMapAPIHandler(syncService, mapsService, drawingService, campaignService)
	syncChangeRepo := syncapi.NewSyncChangeRepository(a.DB)
	syncChangesHandler := syncapi.NewSyncChangesHandler(syncChangeRepo, campaignService)

	// Note API handler for sync API — uses the same note repo/service as the web handler.
	// Created here (before RegisterAPIRoutes) so the service is available; the web
	// handler wiring below reuses the same noteSvc instance.
	noteRepo := notes.NewNoteRepository(a.DB)
	attRepo := notes.NewAttachmentRepository(a.DB)
	noteSvc := notes.NewNoteServiceWithAttachments(noteRepo, attRepo)
	noteAPIHandler := syncapi.NewNoteAPIHandler(syncService, noteSvc)
	// Decides whether an API caller counts as a GM for notes shared with the GM.
	noteAPIHandler.SetCampaignService(campaignService)

	// Tag API handler for sync API — exposes tag CRUD and bulk tag operations.
	tagAPIHandler := syncapi.NewTagAPIHandler(syncService, tagService, entityService, campaignService)

	// Sync history: one record of both directions, read by the Manage page
	// and the Foundry module's History tab.
	syncHistoryRepo := syncapi.NewSyncHistoryRepository(a.DB)
	syncHistoryHandler := syncapi.NewSyncHistoryHandler(syncHistoryRepo, campaignService, syncService,
		syncHistoryEditorAdapter{audit: audit.NewAuditService(audit.NewAuditRepository(a.DB))})
	// Who is in each campaign's Foundry world, as the GM's client reports it.
	foundryPlayerRepo := syncapi.NewFoundryPlayerRepository(a.DB)
	syncHistoryHandler.SetFoundryPlayers(foundryPlayerRepo)

	if a.PluginHealth.IsHealthy("syncapi") {
		syncapi.RegisterAPIRoutes(e, syncAPIHandler, calendarAPIHandler, mediaAPIHandler, mapAPIHandler, noteAPIHandler, tagAPIHandler, syncMappingHandler, syncChangesHandler, stashAPIHandler, syncService, addonService, authService, campaignService, syncapi.WithSyncHistory(syncHistoryHandler), syncapi.WithQuests(syncapi.NewQuestAPIHandler(questAPI, campaignService, "armory")))
		syncapi.RegisterSyncHistoryPageRoutes(e, syncHistoryHandler, campaignService, authService)
		syncapi.RegisterFoundryPageRoutes(e, syncHistoryHandler, campaignService, authService)
		syncapi.RegisterAdminSyncFlowRoute(adminGroup, syncHistoryHandler)
		go syncapi.StartHistoryPruner(a.ShutdownCtx, syncHistoryRepo, foundryPlayerRepo)
	}

	// NPC plugin: gallery/hub view for revealed character entities.
	// The visibility gate reuses entityVisibilityFilterAdapter (the same
	// adapter wired into sessions above), so Player/anonymous views are
	// narrowed by entities' canonical FilterViewableEntityIDs rather than a
	// second, hand-rolled predicate.
	npcRepo := npcs.NewNPCRepository(a.DB)
	npcSvc := npcs.NewNPCService(npcRepo, &npcEntityTypeFinderAdapter{lists: characterListService}, &entityVisibilityFilterAdapter{svc: entityService})
	npcSvc.SetTagLister(&npcTagListerAdapter{svc: tagService})
	npcHandler := npcs.NewHandler(npcSvc)
	npcHandler.SetVisibilityToggler(&npcVisibilityTogglerAdapter{svc: entityService})
	npcs.RegisterRoutes(e, npcHandler, campaignService, authService, addonService)
	// The npcs plugin contributes the NPCs/Monsters section of the unified
	// Characters page (the standalone NPC gallery folded in). npcHandler
	// structurally satisfies entities.NPCSectionProvider.
	entityHandler.SetNPCSectionProvider(npcHandler)

	// Armory plugin: gallery/hub view for item-category entities.
	// Same visibility-gate reasoning as the NPC plugin above.
	armoryRepo := armory.NewArmoryRepository(a.DB)
	armorySvc := armory.NewArmoryService(armoryRepo, &armoryItemTypeFinderAdapter{svc: entityService}, &entityVisibilityFilterAdapter{svc: entityService})
	armorySvc.SetTagLister(&armoryTagListerAdapter{svc: tagService})
	armoryHandler := armory.NewHandler(armorySvc)

	// Instance service: named inventory collections per campaign.
	instRepo := armory.NewInstanceRepository(a.DB)
	instSvc := armory.NewInstanceService(instRepo, &entityVisibilityFilterAdapter{svc: entityService}, &entityCampaignCheckerAdapter{svc: entityService})
	instHandler := armory.NewInstanceHandler(instSvc)
	armoryHandler.SetInstanceService(instSvc)

	// Transaction service: purchase flow, stock management, transaction logging.
	txRepo := armory.NewTransactionRepository(a.DB)
	txSvc := armory.NewTransactionService(txRepo)
	txSvc.SetRelationMetadataUpdater(&armoryRelationMetadataAdapter{svc: relService})
	txSvc.SetRelationFinder(&armoryRelationFinderAdapter{svc: relService})
	txSvc.SetBuyerAccessChecker(&armoryBuyerAccessAdapter{svc: entityService})
	txHandler := armory.NewTransactionHandler(txSvc)
	txHandler.SetEntityVisibility(&entityVisibilityFilterAdapter{svc: entityService})
	// Stashes and moves: the service is built earlier, before the sync API.
	stashHandler := armory.NewStashHandler(stashSvc)
	// Buying shares the stash service's campaign lock, so a purchase and a
	// stash move can't spend the same coins.
	shopBuySvc := armory.NewShopBuyService(stashSvc, txSvc, &armoryShopCheckerAdapter{svc: entityService}, armory.NewPurchaseRequestRepository(a.DB))
	shopBuyHandler := armory.NewShopBuyHandler(shopBuySvc)
	// The Foundry module buys through the sync API as the player at the table,
	// with the same service and acting-as rule as the stash calls. Its routes
	// are registered above and read this at request time.
	syncAPIHandler.SetShopBuyer(&syncShopBuyAPIAdapter{actors: stashAPI, buy: shopBuySvc})
	entityHandler.SetCharacterPagePanel(entities.PagePanel{
		Addon: "armory",
		URL: func(campaignID, entityID string) string {
			return "/campaigns/" + campaignID + "/armory/characters/" + entityID + "/panel"
		},
	})
	shopRoomHandler := armory.NewShopRoomHandler(shopRoomService)
	armory.RegisterRoutes(e, armoryHandler, txHandler, instHandler, stashHandler, shopRoomHandler, shopBuyHandler, campaignService, authService, addonService)

	// Notes widget: personal floating note-taking panel (Google Keep-style).
	// noteSvc was created above (before REST API v1 registration).
	noteHandler := notes.NewHandler(noteSvc)
	noteHandler.SetAttachmentService(noteSvc)
	noteHandler.SetMediaUploader(&mediaUploadAdapter{svc: mediaService})
	noteHandler.SetPictureUploader(&mediaUploadAdapter{svc: mediaService})
	// Who may open a picture in a note is decided by who can read the notes
	// holding it, asked of the notes service rather than its tables.
	mediaHandler.SetNoteMediaAccess(&noteMediaAccessAdapter{svc: noteSvc})
	noteHandler.SetMemberLister(campaignService)
	noteHandler.SetCharacterLister(&journalCharacterAdapter{svc: entityService})
	notePages := &notesPagesAdapter{svc: entityService}
	noteHandler.SetPageNamer(notePages)
	noteHandler.SetPageLinker(notePages)
	notes.RegisterRoutes(e, noteHandler, campaignService, authService)
	// A player can allow the Foundry notebook to use their notes. Grants go
	// only to an address that may already call Chronicle across sites.
	noteGrants := notes.NewAppGrantService(notes.NewAppGrantRepository(a.DB))
	// See wsRevokerHolder: wsHub itself is constructed further down.
	noteGrants.SetConnectionRevoker(wsRevoker)
	noteGrantHandler := notes.NewAppGrantHandler(noteGrants, &notesOriginAllower{baseURL: a.Config.BaseURL, settings: settingsService})
	// Pictures and voice memos in the frames load through member-checked
	// signed links, as Foundry's media does.
	noteGrantHandler.SetMediaLinker(mediaHandler)
	// A grant ends whenever the player's sessions do (password reset or
	// change, force sign-out).
	auth.OnSessionsRevoked(authService, func(ctx context.Context, userID string) {
		if err := noteGrants.RevokeAllForUser(ctx, userID); err != nil {
			slog.Error("revoking notes app grants failed", slog.String("user_id", userID), slog.Any("error", err))
		}
	})
	// Deleting your own account: the campaign side runs first and refuses
	// while the person still owns a campaign; once the account is emptied,
	// the rest of what other plugins hold about the person goes.
	auth.ConfigureAccountDeletion(authService, &accountDeletionAdapter{campaigns: campaignService})
	auth.OnAccountDeleted(authService, func(ctx context.Context, userID string) {
		forgetDeletedAccount(ctx, userID, entityRepo, entityNotesRepo, noteRepo, syncService)
	})
	// Removing a player from a campaign ends their grants there, so a later
	// re-invite starts with none.
	campaigns.OnMemberRemoved(campaignService, func(ctx context.Context, campaignID, userID string) {
		if err := noteGrants.RevokeAllInCampaign(ctx, campaignID, userID); err != nil {
			slog.Error("revoking notes app grants on removal failed", slog.String("campaign_id", campaignID), slog.String("user_id", userID), slog.Any("error", err))
		}
	})
	// The campaign's Sync API switch governs outside apps, the notebook too.
	notesAppGate := func(ctx context.Context, campaignID string) (bool, error) {
		return addonService.IsEnabledForCampaign(ctx, campaignID, syncapi.SyncAPIAddonSlug)
	}
	noteHandler.SetJotsGate(func(ctx context.Context, campaignID string) (bool, error) {
		return addonService.IsEnabledForCampaign(ctx, campaignID, addons.JotNotesAddonSlug)
	})
	notesApp := notes.RegisterAppGrantRoutes(e, noteHandler, noteGrantHandler, noteGrants, notesAppGate, campaignService, authService)
	// The editor's @ page picker, as the player sees pages.
	notesApp.GET("/entities/search", entityHandler.SearchAPI, campaigns.RequireViewAccess())
	notesApp.GET("/entities/:eid/preview", entityHandler.PreviewAPI, campaigns.RequireViewAccess())
	// The Foundry calendar window: the calendar page over the same grant,
	// each route at its site gate, only while the calendar plugin is healthy.
	if a.PluginHealth.IsHealthy(calendar.PluginSlug) {
		calendar.RegisterAppRoutes(notesApp, calendarHandler, addonService)
		noteGrantHandler.AllowEmbedMode(calendar.PluginSlug)
	}

	// Relations widget routes already registered above (before REST API v1).

	// Audit plugin: campaign activity logging and history.
	auditRepo := audit.NewAuditRepository(a.DB)
	auditService := audit.NewAuditService(auditRepo)
	shareAudit.svc = auditService
	auditHandler := audit.NewHandler(auditService)
	// Guard the entity-history endpoint with campaign ownership + per-entity
	// visibility, resolved via the entities service (SEC-IDOR-2).
	auditHandler.SetEntityViewGuard(&auditEntityViewGuardAdapter{svc: entityService})
	audit.RegisterRoutes(e, auditHandler, campaignService, authService)

	// Wire audit logging into mutation handlers so CRUD actions are recorded.
	entityHandler.SetAuditService(auditService)
	// CALV5-PLACEHOLDER: V5 must restore the calendar's remaining wiring —
	// SetAuditService, SetTierDefinitionsLister, RegisterExtensionDashboard,
	// SetTimelineLister, and the block spine (calendar.NewBlockService +
	// RealTimeSeam + SyncLinkProbe + calendar.InstallBlockSpine). Keep the
	// spine's repository a narrow read surface over *sql.DB rather than a
	// wide CalendarRepository interface — the old plugin's 60-method
	// interface was itself a reason it became hard to change. TODO(#778)

	// Wire the per-campaign read window into the operator diagnostics, so it
	// can answer "why does MY campaign look like this?" and not only "which
	// code is running". The route table is a closure, evaluated when the
	// diagnostic runs, because routes are still being registered below this
	// line and because it keeps Echo types out of the adapter file.
	systems.SetCampaignDiagProvider(campaignDiagAdapter{
		campaigns: campaignService,
		addons:    addonService,
		entities:  entityService,
		routes: func() []systems.RouteFact {
			live := e.Routes()
			out := make([]systems.RouteFact, 0, len(live))
			for _, r := range live {
				out = append(out, systems.RouteFact{Method: r.Method, Path: r.Path, Handler: r.Name})
			}
			return out
		},
		// Read from the same catalog the sidebar links from, never re-typed
		// here — a copied path would keep reporting the old destination
		// after the real one moved.
		sidebarCalendarPath: navAppPath(calendar.PluginSlug),
	})

	// The new calendar.* diagnostic (calendar-v5 seams, #778): counts only
	// (calendars/events/moons/eras/kinds), the plugin's own migration state,
	// and the Foundry sync state — never event text, member names, or
	// answers. A separate provider/interface from campaignDiagAdapter above,
	// mirroring how entity.* has its own EntityDiagProvider.
	systems.SetCalendarDiagProvider(calendarDiagAdapter{
		campaigns:    campaignService,
		addons:       addonService,
		calendar:     calendarService,
		pluginHealth: a.PluginHealth,
	})

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

	// --- Undo for world pages: History, Trash, save clashes ---
	// Wired here, after settings exists, because the Trash's retention is a
	// site setting. Until SetPageSafety runs, deletes are permanent.
	pageSafetyRepo := entities.NewPageSafetyRepository(a.DB)
	entityService.SetPageSafety(pageSafetyRepo)
	pageSafetyService := entities.NewPageSafetyService(pageSafetyRepo, entityRepo, entityService, settingsService)
	entities.RegisterPageSafetyRoutes(e, entities.NewPageSafetyHandler(pageSafetyService, entityService, campaignService), campaignService, authService)
	go pageSafetyService.StartPurger(a.ShutdownCtx)

	// --- Entity Block Registry ---
	// Create the block registry and let each plugin register its block types.
	// This drives validation, rendering, and the template editor palette.
	blockRegistry := entities.NewBlockRegistry()
	entities.RegisterCoreBlocks(blockRegistry)
	blockRegistry.Register(entities.BlockMeta{
		Type: entities.BlockCharacterItems, Label: "Items & Money", Icon: "fa-sack-dollar",
		Description: "What a character carries, their money and recent moves",
		Addon:       armory.AddonSlug, Contexts: []string{"template"},
	}, entities.RenderCharacterItemsBlock)

	// Widget-binding framework: the dynamic host↔widget-type↔instance
	// registry + service. Widget types register declaratively; the service
	// resolves a host's instance via the precedence chain (own binding →
	// entity-type template → default).
	widgetRegistry := widgetbindings.NewRegistry()
	widgetRegistry.Register(timeline.NewTimelineWidgetType(timelineSvc))
	// Registers the calendar and worldstate widget types. service.Sweep
	// skips widget types it does not recognize rather than deleting their
	// bindings, so a GM's entity→calendar bindings persist across any gap
	// in these types' registration and resolve again once InstanceExists
	// can answer for them.
	widgetRegistry.Register(calendar.NewCalendarWidgetType(calendarService))
	widgetRegistry.Register(calendar.NewWorldstateWidgetType(calendarService))
	// maps registers with no campaign default — the legacy entity.map_id
	// fallback lives in the map_editor closure instead.
	widgetRegistry.Register(maps.NewMapWidgetType(mapsService))
	widgetBindingSvc := widgetbindings.NewService(widgetbindings.NewRepository(a.DB), widgetRegistry)
	// The calendar/timeline services call OnInstanceDeleted on delete so a
	// removed instance's bindings are swept promptly (render-time guard and
	// Sweep are the backstop). Reached via a type assertion so the
	// CalendarService/TimelineService interfaces stay unchanged.
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
	// The create-or-pick binding UI (picker + bind/create/unbind, Scribe+).
	widgetbindings.RegisterRoutes(e, widgetbindings.NewHandler(widgetBindingSvc, widgetRegistry), campaignService, authService)

	// renderBoundBlock is the single seam every widget-bound entity block goes
	// through on first render. It resolves the host's instance via the
	// framework and delegates to the widget type's RenderBlock — the same
	// path the binding handler uses for a post-mutation swap, so the initial
	// render and the swap are byte-identical. legacyID is the maps
	// `entity.map_id` fallback, used only when nothing else resolves (a
	// binding always wins).
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
	blockRegistry.Register(entities.BlockMeta{
		Type: "upcoming_events", Label: "Upcoming Events", Icon: "fa-calendar-check",
		Description: "Upcoming calendar events list", Addon: "calendar",
		Contexts: []string{"template"},
		ConfigFields: []entities.ConfigFieldMeta{
			{Key: "limit", Label: "Events to show", Type: "number", Min: entities.IntPtr(1), Max: entities.IntPtr(20), Default: 5},
		},
	}, func(ctx entities.BlockRenderContext) templ.Component {
		// Reuses the calendar plugin's own hx-get fragment, the same one
		// the dashboard/category "Upcoming Events" cards lazy-load — see
		// upcoming_events_block.templ.
		limit := entities.BlockConfigLimit(ctx.Block.Config, "limit", 5)
		return upcomingEventsBlockShell(ctx.CC.Campaign.ID, limit)
	})
	// entity_calendar — the entity-page calendar embed, bindable to any of
	// the campaign's calendars (widgetbindings). Singleton per page (the
	// swap target is a fixed DOM id). Renders that calendar's upcoming
	// events only — calendar.calendarWidgetType's doc comment says why
	// that is the honest scope rather than a fuller "ambient band + this
	// entity's linked events" engine. Distinct from calendar_preview
	// (dashboard upcoming-events card) by design.
	blockRegistry.Register(entities.BlockMeta{
		Type: "entity_calendar", Label: "Calendar (this entity)", Icon: "fa-calendar-days",
		Description: "Upcoming events for a bound calendar",
		Addon:       "calendar", Contexts: []string{"template"}, Singleton: true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		return renderBoundBlock(calendar.WidgetTypeCalendar, rc, "")
	})

	// entity_worldstate — the entity-page/dashboard sky embed, bindable to
	// any of the campaign's calendars (widgetbindings). Singleton like
	// entity_calendar. calendar.worldstateWidgetType's doc comment says why
	// this renders sky only, not yet the hourglass shelf (real UI work of
	// its own).
	blockRegistry.Register(entities.BlockMeta{
		Type: "entity_worldstate", Label: "Worldstate timepiece", Icon: "fa-hourglass-half",
		Description: "Ambient sky for the current world date",
		Addon:       "calendar", Contexts: []string{"template", "dashboard"}, Singleton: true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		return renderBoundBlock(calendar.WidgetTypeWorldstate, rc, "")
	})

	// skybox — the ambient sky-only block: no hourglass, no per-entity
	// binding (always the campaign's default calendar). Campaign-level, like
	// entity_worldstate, so it works on both the entity page and the
	// campaign dashboard.
	blockRegistry.Register(entities.BlockMeta{
		Type: "skybox", Label: "Sky", Icon: "fa-cloud-sun",
		Description: "Ambient sky only — moons, stars, weather + celestial events for the current world date",
		Addon:       "calendar", Contexts: []string{"template", "dashboard"}, Singleton: true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		return renderSkyboxBlock(context.Background(), calendarService, rc)
	})

	// Quest board and Notice boards: cork-board blocks for quest pages and
	// place pages. Always available (no addon); the widgets fetch their own
	// data, so these only emit the mount point.
	blockRegistry.Register(entities.BlockMeta{
		Type: "quest_board", Label: "Quest board", Icon: "fa-scroll",
		Description: "Cork board with the quest's notice, a map scrap and a reward tag, plus a DM-only ledger",
		Contexts:    []string{"template"}, Singleton: true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		if rc.CC == nil || rc.Entity == nil {
			return templ.NopComponent
		}
		return quests.QuestBoardMount(rc.CC.Campaign.ID, rc.Entity.ID, rc.CSRFToken, rc.CC.CanControlWorldState())
	})
	blockRegistry.Register(entities.BlockMeta{
		Type: "notice_boards", Label: "Notice boards", Icon: "fa-thumbtack",
		Description: "Boards players can cycle through, with quest notices, notes, pinned pages and maps",
		Contexts:    []string{"template", "category"}, Singleton: true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		if rc.CC == nil || rc.Entity == nil {
			return templ.NopComponent
		}
		return quests.NoticeBoardsMount(rc.CC.Campaign.ID, rc.Entity.ID, rc.CSRFToken, rc.CC.CanControlWorldState(), int(rc.CC.MemberRole))
	})
	// The same block on a category dashboard: its boards belong to the
	// category, not to a page.
	entities.RegisterCategoryBlock("notice_boards", func(cc *campaigns.CampaignContext, et *entities.EntityType) templ.Component {
		return quests.CategoryBoardsMount(cc.Campaign.ID, et.ID, cc.CanControlWorldState(), int(cc.MemberRole))
	})

	// Timeline plugin blocks (requires "timeline" addon).
	blockRegistry.Register(entities.BlockMeta{
		Type: "timeline", Label: "Timeline", Icon: "fa-timeline",
		Description: "Timeline preview with events", Addon: "timeline",
		Contexts: []string{"template"},
	}, func(rc entities.BlockRenderContext) templ.Component {
		// Unbound (no default) renders BlockTimeline's campaign preview list;
		// bound renders that timeline instead.
		return renderBoundBlock(timeline.WidgetTypeTimeline, rc, "")
	})

	// Maps plugin blocks (requires "maps" addon).
	//
	// map_editor — per-entity Map block. The map is resolved from a widget
	// binding, else the legacy entities.map_id (NOT block config), so the
	// choice lives on the entity itself, not a shared entity-type layout.
	// Template context only. Three render branches: a resolved map → framed
	// preview that unfolds into the live viewer on click (Scribe+ can change
	// it); none + Scribe+ → create-or-pick prompt; none + Player → friendly
	// empty state.
	blockRegistry.Register(entities.BlockMeta{
		Type: "map_editor", Label: "Map Editor", Icon: "fa-map-location-dot",
		Description: "Framed map preview that opens the full map",
		Addon:       "maps", Contexts: []string{"template"},
		// No ConfigFields — the source of truth is entity.MapID. The
		// picker is rendered by the block itself, not the layout editor.
		// Singleton: only one map_editor per layout — every instance would
		// resolve to the same entity-bound map, and the live viewer it opens
		// binds fixed DOM IDs. See BlockMeta.Singleton docstring.
		Singleton: true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		// Resolve + render via maps.mapWidgetType.RenderBlock. A
		// widget_bindings row (widget_type="map") wins; an unbound entity
		// with a legacy entity.map_id falls back to rendering that map.
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
		// Use the promoted role (VisibilityRole), not the raw one, so a
		// co-DM sees the same content here as in the full gallery.
		cards, err := npcHandler.GalleryBlock(context.Background(), bctx.CC.Campaign.ID, bctx.CC.VisibilityRole(), "", limit)
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
		// Use the promoted role (VisibilityRole), not the raw one, so a
		// co-DM sees the same content here as in the full gallery.
		cards, err := armoryHandler.GalleryBlock(context.Background(), bctx.CC.Campaign.ID, bctx.CC.VisibilityRole(), "", limit)
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

	// Slug-keyed entity-show renderer registry. System packages register
	// character/monster/item renderers here at startup; an empty registry
	// is fine — lookupEntityShowRenderer returns nil for every slug and
	// show.templ falls through to the standard block dispatch. See
	// docs/system-package-rendering.md for the system-package contract.
	showRegistry := entities.NewEntityShowRendererRegistry()
	registerManifestRenderers(showRegistry)
	entities.SetGlobalEntityShowRendererRegistry(showRegistry)

	// One-time: place the page pieces that became blocks into existing
	// layouts, so no page loses anything (see page_extras.go). Best-effort;
	// a failure is retried on the next boot.
	// The lists come first: the page-extras pass reads them.
	// A failed seed skips the page-extras pass this boot: it would read empty
	// lists, place nothing for that campaign, and still mark itself done.
	var seedErr error
	if n, err := seedCharacterListsOnce(context.Background(), settingsRepo, campaignService, characterListService); err != nil {
		seedErr = err
		slog.Error("seeding character lists failed", slog.String("error", err.Error()))
	} else if n > 0 {
		slog.Info("checked character lists for existing campaigns", slog.Int("campaigns", n))
	}
	if seedErr != nil {
		slog.Warn("placing page extras deferred until character lists are seeded")
	} else if n, err := placePageExtrasOnce(context.Background(), settingsRepo, campaignService, addonService, entityService, characterListService); err != nil {
		slog.Error("placing page extras failed", slog.String("error", err.Error()))
	} else if n > 0 {
		slog.Info("placed page extras into layouts", slog.Int("layouts", n))
	}

	// One-time: Notes became Journal plus Jot notes, so campaigns that had
	// Notes on keep both (see jot_notes_split.go). Retried on the next boot.
	if n, err := splitJotNotesOnce(context.Background(), settingsRepo, addonService); err != nil {
		slog.Error("splitting jot notes failed", slog.String("error", err.Error()))
	} else if n > 0 {
		slog.Info("turned jot notes on where the journal was on", slog.Int("campaigns", n))
	}

	campaignHandler.SetAuditLogger(&campaignAuditAdapter{svc: auditService})
	campaignHandler.SetAddonLister(&addonListerAdapter{svc: addonService})
	campaignHandler.SetSystemAddonEnabler(addonService)
	campaignHandler.SetMediaUploader(&backdropUploaderAdapter{svc: mediaService})
	campaignHandler.SetSMTPChecker(smtpService)
	campaignHandler.SetSystemLister(&systemListerAdapter{})
	tagHandler.SetAuditService(auditService)

	// --- AI Workspace ---
	// Owns the AI Export feature, the Prompt builder, and the AI Import
	// surface. The renderer Service depends on narrow per-plugin listers,
	// which every plugin Service already implements. Owner-gated routes
	// mount on a /campaigns/:id group that already enforces auth + campaign
	// membership, mirroring foundry_vtt's RegisterOwnerRoutes pattern.
	//
	aiWorkspaceRenderer := aiexport.NewService(
		entityService,
		noteSvc,
		&aiExportCalendarListerAdapter{svc: calendarService},
		sessionsService,
		timelineSvc,
		relService,
		tagService,
	)
	aiWorkspaceHandler := ai_workspace.NewHandler(aiWorkspaceRenderer)
	aiWorkspaceHandler.SetAuditLogger(&aiWorkspaceAuditAdapter{svc: auditService})

	// Prompt builder reuses the aiexport renderer as the content Exporter
	// so the prompt's "Existing world context" section inherits
	// SEC-6-AMENDED egress sanitization + the privacy modes.
	aiWorkspacePrompt := prompt.NewService(
		entityService,
		tagService,
		aiWorkspaceRenderer,
	)
	aiWorkspaceHandler.SetPromptBuilder(aiWorkspacePrompt)

	// The entities service implements importer.CampaignLookup and
	// importer.EntityCreator as-is; no adapter needed. SEC-6-AMENDED
	// ingress sanitization is enforced by the AST pin in
	// internal/plugins/ai_workspace/importer/committer_sanitize_test.go.
	aiWorkspaceHandler.SetImportLookup(entityService)
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
	exportSvc.SetNoteExporter(&noteExportAdapter{svc: noteSvc})
	exportSvc.SetAddonExporter(&addonExportAdapter{svc: addonService})
	exportSvc.SetMediaExporter(&mediaExportAdapter{svc: mediaService})
	exportSvc.SetMediaBundler(&mediaBundleAdapter{svc: mediaService})
	exportSvc.SetEntityImporter(&entityImportAdapter{entitySvc: entityService, tagSvc: tagService, relationSvc: relService})
	exportSvc.SetCalendarImporter(&calendarImportAdapter{svc: calendarService})
	exportSvc.SetTimelineImporter(&timelineImportAdapter{svc: timelineSvc})
	exportSvc.SetSessionImporter(&sessionImportAdapter{svc: sessionsService})
	exportSvc.SetMapImporter(&mapImportAdapter{mapSvc: mapsService, drawingSvc: drawingService})
	exportSvc.SetNoteImporter(&noteImportAdapter{svc: noteSvc})
	exportSvc.SetAddonImporter(&addonImportAdapter{svc: addonService})
	exportSvc.SetGroupExporter(&groupExportAdapter{svc: groupService})
	exportSvc.SetGroupImporter(&groupImportAdapter{svc: groupService})
	exportSvc.SetPostExporter(&postExportAdapter{postSvc: postService, entitySvc: entityService})
	exportSvc.SetPostImporter(&postImportAdapter{svc: postService})
	exportSvc.SetMediaImporter(&mediaImportAdapter{svc: mediaService})
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

	// CALV5-PLACEHOLDER: V5 must rewire these closures to read through
	// calendarService (GetCalendar, then ListUpcomingEvents) again. Until
	// then the adapter stays wired but errors, so a WASM plugin calling
	// get_calendar gets a reportable error instead of a misleading null.
	// TODO(#778)
	errCalendarRebuilding := errors.New("calendar is being rebuilt (V5) and is unavailable to extensions")
	wasmCalendarReader := extensions.NewWASMCalendarAdapter(
		func(ctx context.Context, campaignID string) (json.RawMessage, error) {
			return nil, errCalendarRebuilding
		},
		func(ctx context.Context, campaignID string, limit int) (json.RawMessage, error) {
			return nil, errCalendarRebuilding
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
			ctx = changesource.With(ctx, changesource.Source{Kind: changesource.KindExtension})
			return entityService.UpdateFields(ctx, entityID, fields)
		},
	))

	// CALV5-PLACEHOLDER: V5 must rewire create_event to unmarshal a
	// calendar.CreateEventInput and delegate to calendarService.CreateEvent.
	// TODO(#778)
	wasmHostEnv.SetCalendarWriter(extensions.NewWASMCalendarWriteAdapter(
		func(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error) {
			return nil, errCalendarRebuilding
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
	wasmHandler.SetActivityRecorder(adminActivity)
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
	systemHandler.SetBookEdits(systems.NewBookEditService(systems.NewBookEditRepository(a.DB)))
	systems.RegisterRoutes(e, systemHandler, addonService, authService, campaignService)
	// The campaign's own pick-list entries, merged into the pick lists below.
	// The manifest is the campaign's custom upload, else its chosen built-in.
	systemEntrySvc := systems.NewSystemEntryService(
		systems.NewSystemEntryRepository(a.DB),
		func(ctx context.Context, campaignID string) *systems.SystemManifest {
			if m := campaignSystemMgr.GetManifest(campaignID); m != nil {
				return m
			}
			c, err := campaignService.GetByID(ctx, campaignID)
			if err != nil {
				return nil
			}
			return systems.Find(c.ParseSettings().SystemID)
		})
	systems.RegisterSystemEntryChoices(systemEntrySvc)
	systems.RegisterSystemEntryRoutes(e, systems.NewSystemEntryHandler(systemEntrySvc), authService, campaignService)
	// Pick lists (Ancestry, Kit, Race, Class…) for the character attributes editor.
	characterChoiceSvc := systems.NewCharacterChoiceService(addonService, campaignSystemMgr)
	systems.RegisterCharacterChoiceRoutes(e, systems.NewCharacterChoiceHandler(characterChoiceSvc), authService, campaignService)
	entityHandler.SetHeroPlanner(heroPlannerAdapter{choices: characterChoiceSvc})

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

	// CALV5-PLACEHOLDER: the calendar/timeline demo routes and the
	// internal/templates/demo package were removed. V5's design is signed as
	// static renders instead, not a maintained route (#741). TODO(#778):
	// not a re-wiring target, listed for completeness.

	// widgetScripts is the on-sight widget manifest. It is filled after every
	// plugin has registered and its static files are mounted (the content
	// hashes need them), which is before the server takes a request.
	var widgetScripts map[string][]string

	// --- Layout Data Injector ---
	// Registers the callback that copies auth/campaign data from Echo's
	// context into Go's context.Context so Templ templates can read it.
	// This runs inside middleware.Render() before every template render.
	middleware.MediaSigner = func(c echo.Context) (layouts.MediaURLFunc, layouts.MediaThumbFunc) {
		return mediaSignerFor(urlSigner, c)
	}
	middleware.LayoutInjector = func(c echo.Context, ctx context.Context) context.Context {
		// Inject plugin-contributed body scripts (constant for process lifetime),
		// so plugins can register widget scripts without hardcoding paths in the
		// core base.templ layout.
		ctx = layouts.SetPluginBodyScripts(ctx, pluginBodyScripts)
		ctx = layouts.SetWidgetScripts(ctx, widgetScripts)

		// Site look (name, logo, tab icon, and for pages outside a campaign the
		// borrowed look). A failed read leaves the shipped look rather than
		// failing the page. A campaign page only gets the name and tab icon:
		// ApplySiteLook does nothing inside a campaign.
		if sl, err := settingsService.GetSiteLook(c.Request().Context()); err != nil {
			slog.Warn("reading the site look", slog.Any("error", err))
		} else {
			ctx = layouts.SetSiteLook(ctx, sl)
		}

		// User info from auth session.
		if session := auth.GetSession(c); session != nil {
			ctx = layouts.SetIsAuthenticated(ctx, true)
			ctx = layouts.SetUserID(ctx, session.UserID)
			ctx = layouts.SetUserName(ctx, session.Name)
			ctx = layouts.SetUserEmail(ctx, session.Email)
			ctx = layouts.SetUserAvatarPath(ctx, session.AvatarPath)
			ctx = layouts.SetIsAdmin(ctx, session.IsAdmin)

			// The person's own look. HTMX swaps never replace <html>, so only
			// full-page renders need it. A failed read leaves the default look
			// rather than failing the page.
			if !middleware.IsHTMX(c) {
				if vp, err := authService.GetViewPrefs(ctx, session.UserID); err == nil {
					ctx = layouts.SetViewPrefs(ctx, &layouts.ViewPrefsData{Theme: vp.Theme, Motion: vp.Motion, TextSize: vp.TextSize, Contrast: vp.Contrast})
				} else {
					slog.Warn("reading view prefs", slog.String("user_id", session.UserID), slog.Any("error", err))
				}
			}

			// Inject degraded plugin count for admin sidebar badge.
			if session.IsAdmin {
				ctx = layouts.SetDegradedPluginCount(ctx, len(a.PluginHealth.DegradedPlugins()))
				// The admin's own pins, read for this admin only. A failed
				// read leaves the Pinned group out rather than failing the page.
				if pins, err := adminNavPinService.Pins(ctx, session.UserID); err == nil {
					ctx = layouts.SetAdminNavPins(ctx, pins)
				} else {
					slog.Warn("reading admin nav pins", slog.String("user_id", session.UserID), slog.Any("error", err))
				}
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
			if ap := campaignSettings.Appearance; ap != nil {
				ad := &layouts.AppearanceData{
					NavStyle: ap.NavStyle, NavStrength: ap.NavStrength, NavPageName: ap.NavPageName,
					PageTone: ap.PageTone, Contrast: ap.Contrast,
					BodyFont: ap.BodyFont, HeadingFont: ap.HeadingFont, TypeScale: ap.TypeScale,
					ButtonStyle: ap.ButtonStyle, Elevation: ap.Elevation, MotionSpeed: ap.MotionSpeed,
					ReduceMotion:  ap.ReduceMotion,
					HeaderHeight:  ap.HeaderHeight,
					SidebarColour: ap.SidebarColour, SidebarOwn: ap.SidebarOwn,
					SidebarCorner: ap.SidebarCorner, SidebarSubtitle: ap.SidebarSubtitle, SidebarBanner: ap.SidebarBanner,
					PeekGlow: ap.PeekGlow, PeekGlowColour: ap.PeekGlowColour,
					HoverCard: ap.HoverCard,

					SheetStyle: ap.SheetStyle,
				}
				ctx = layouts.SetAppearance(ctx, ad)
			}
			if campaignSettings.TopbarStyle != nil {
				ctx = layouts.SetTopbarStyle(ctx, &layouts.TopbarStyleData{
					Mode:         campaignSettings.TopbarStyle.Mode,
					Color:        campaignSettings.TopbarStyle.Color,
					GradientFrom: campaignSettings.TopbarStyle.GradientFrom,
					GradientTo:   campaignSettings.TopbarStyle.GradientTo,
					GradientDir:  campaignSettings.TopbarStyle.GradientDir,
					ImagePath:    campaignSettings.TopbarStyle.ImagePath,
					Scrim:        campaignSettings.TopbarStyle.Scrim,
				})
			}
			if campaignSettings.TopbarContent != nil && campaignSettings.TopbarContent.Mode != "" && campaignSettings.TopbarContent.Mode != "none" {
				tc := &layouts.TopbarContentData{
					Mode:    campaignSettings.TopbarContent.Mode,
					Quote:   campaignSettings.TopbarContent.Quote,
					Widgets: campaignSettings.TopbarContent.Widgets,
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
			ctx = layouts.SetIsDmGranted(ctx, cc.IsDmGranted)
			if isOwner {
				if cookie, err := c.Cookie("chronicle_view_as_player"); err == nil && cookie.Value == "1" {
					effectiveRole = int(campaigns.RolePlayer)
					ctx = layouts.SetViewingAsPlayer(ctx, true)
				}
			}
			ctx = layouts.SetCampaignRole(ctx, effectiveRole)
			ctx = layouts.SetCampaignArchived(ctx, cc.Campaign.IsArchived())

			// Entity types, per-type counts and the enabled addons: what the
			// sidebar is built from. Use the request context (not the
			// enriched ctx) since service calls only need cancellation/
			// deadline, not layout data.
			reqCtx := c.Request().Context()
			var sidebarTypes []layouts.SidebarEntityType
			etypes, typesErr := entityService.GetEntityTypes(reqCtx, cc.Campaign.ID)
			if typesErr == nil {
				sidebarTypes = sidebarTypesFrom(etypes)
				ctx = layouts.SetEntityTypes(ctx, sidebarTypes)
			}

			// Entity counts per type for sidebar badges (use effectiveRole so
			// "view as player" mode hides private entity counts).
			// Pass user ID for permission-aware entity counts.
			layoutUserID := ""
			if session := auth.GetSession(c); session != nil {
				layoutUserID = session.UserID
			}
			counts, countsErr := entityService.CountByType(reqCtx, cc.Campaign.ID, effectiveRole, layoutUserID)
			if countsErr == nil {
				// CountByType already rolls child entity_type counts up into
				// their parent under the sub-category-as-template model.
				ctx = layouts.SetEntityCounts(ctx, counts)
			}

			// Enabled addons for conditional widget rendering, and the game
			// system that backs the campaign's rulebook.
			enabledSlugs := make(map[string]bool)
			var enabledSystem layouts.EnabledSystem
			if campaignAddons, err := addonService.ListForCampaign(reqCtx, cc.Campaign.ID); err == nil {
				enabledSlugs, enabledSystem = navAddonsFrom(campaignAddons)
				ctx = layouts.SetEnabledAddons(ctx, enabledSlugs)
				if enabledSystem.Slug != "" {
					ctx = layouts.SetEnabledSystem(ctx, enabledSystem)
				}
			}

			// The sidebar, cut down for this viewer. Rows hidden from
			// players reach only the owner, and not while they view as a
			// player (effectiveRole is Player then); so does the whole
			// arrangement the owner's editor starts from.
			if typesErr == nil {
				in := navInputsFor(cc, sidebarTypes, counts, enabledSlugs, enabledSystem)
				navOwner := effectiveRole >= int(campaigns.RoleOwner)
				// A member who is not the owner has pins of their own; they
				// are read for this user only.
				var pins []string
				if cc.IsMember && cc.MemberRole < campaigns.RoleOwner && layoutUserID != "" {
					if p, err := campaignService.NavPins(reqCtx, cc.Campaign.ID, layoutUserID); err == nil {
						pins = p
					} else {
						slog.Warn("reading member nav pins", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
					}
				}
				ctx = layouts.SetNavSections(ctx, viewNavSections(cc, in, navOwner, pins))
				if navOwner {
					ctx = layouts.SetNavEdit(ctx, buildNavEdit(in))
				}
			}

			// Plugin health, for the same dashboard/category blocks that
			// check IsAddonEnabled above: an addon can be turned on in
			// settings while its plugin's own schema is degraded, in which
			// case its routes (like calendar's) were never registered (see
			// the calendar.PluginSlug health gate around RegisterRoutes
			// below) and an hx-get to one would 404 forever with no swap.
			// Only calendar is populated today; a block for another
			// health-gated plugin can add its slug here when it needs the
			// same guard.
			calendarHealthy := a.PluginHealth == nil || a.PluginHealth.IsHealthy(calendar.PluginSlug)
			healthyPlugins := map[string]bool{
				calendar.PluginSlug: calendarHealthy,
			}
			ctx = layouts.SetHealthyPlugins(ctx, healthyPlugins)

			// The campaign/category "Upcoming Events" cards need both facts
			// above (addon on AND plugin healthy) combined into one flag,
			// but campaigns/entities may not name the calendar plugin
			// themselves (plugin isolation, T-B2) to compute it — resolve it
			// here, where calendar.PluginSlug is already in scope.
			ctx = layouts.SetUpcomingEventsAvailable(ctx, enabledSlugs[calendar.PluginSlug] && calendarHealthy)

			// The header's data-backed widgets (date, weather, moon, game
			// night) read today's world from the calendar and sessions. This
			// sits here because it needs the enabled addons resolved above;
			// a fragment swap never draws the bar, so it skips the reads.
			if tc := layouts.GetTopbarContent(ctx); tc != nil && !middleware.IsHTMX(c) {
				liveCtx, cancel := context.WithTimeout(reqCtx, headerLiveTimeout)
				tc.Live = buildTopbarLive(liveCtx, calendarService, sessionsService, headerLiveRequest{
					CampaignID: cc.Campaign.ID,
					Viewer:     permissions.RequestViewer(cc.VisibilityRole(), layoutUserID),
					Widgets:    tc.TopbarWidgets(),
					Calendar:   enabledSlugs[calendar.PluginSlug] && calendarHealthy,
					// Game nights are the table's own business: members only,
					// never a public-campaign visitor.
					Nights: cc.IsMember && enabledSlugs[calendar.PluginSlug],
					Now:    time.Now(),
				})
				cancel()
			}

			// The campaign's sky is drawn from its default calendar, so the
			// Sky header background needs that calendar's id, and so does
			// the Customize page's example header. Without one the header
			// keeps its still night colours. A fragment swap never redraws
			// the bar, so only a full page reads it for the header.
			skyHeader := !middleware.IsHTMX(c)
			if ts := layouts.GetTopbarStyle(ctx); ts == nil || ts.Mode != "sky" {
				skyHeader = false
			}
			if (skyHeader || c.Path() == "/campaigns/:id/customize") && enabledSlugs[calendar.PluginSlug] && calendarHealthy {
				skyCtx, cancel := context.WithTimeout(reqCtx, headerLiveTimeout)
				ctx = layouts.SetSkyCalendarID(ctx, headerSkyCalendarID(skyCtx, calendarService, cc.Campaign.ID,
					permissions.RequestViewer(cc.VisibilityRole(), layoutUserID)))
				cancel()
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
		ctx = layouts.SetRequestPath(ctx, c.Request().URL.Path)

		// Where the viewer is in the campaign sidebar, and which sections
		// they folded (a per-campaign cookie sidebar_nav.js writes), so the
		// server paints the finished sidebar and nothing jumps on load.
		if campaigns.GetCampaignContext(c) != nil {
			if cookie, err := c.Cookie(layouts.NavFoldsCookie); err == nil {
				ctx = layouts.SetNavFolds(ctx, layouts.ParseNavFolds(cookie.Value))
			}
			ctx = layouts.ResolveNavState(ctx)
		}

		// Signed media URL generators for templates. Bound to whoever is
		// RENDERING this response (ADR-058), resolved once per render and
		// closed over by both funcs so every fileID in the response embeds
		// the same viewer. A session cookie present means the viewer is that
		// user; none means anonymous — media.URLSigner.Verify derives the
		// same identity from the same session lookup when links are fetched.
		if urlFn, thumbFn := mediaSignerFor(urlSigner, c); urlFn != nil {
			ctx = layouts.SetMediaURLFunc(ctx, urlFn)
			ctx = layouts.SetMediaThumbFunc(ctx, thumbFn)
		}

		// Last, so the campaign values above are already in place: a page
		// outside a campaign borrows the site look, a campaign page does not.
		ctx = layouts.ApplySiteLook(ctx)

		return ctx
	}

	// --- WebSocket Hub ---
	// Real-time bidirectional sync for Foundry VTT and browser clients.
	wsHub := ws.NewHub()
	go wsHub.Run()
	campaignHandler.SetFoundryConnector(&foundryConnectorAdapter{keys: syncService, hub: wsHub, baseURL: a.Config.BaseURL, served: fvttOwnerUpdates})

	// Late-bind now that wsHub exists — see wsRevokerHolder above. From
	// here on, every wired revoke path force-disconnects the sockets it
	// affects instead of being a no-op.
	wsRevoker.hub = wsHub

	// Wire the WS hub's presence lookup into foundry_vtt. fvttHandler owns
	// both GET /campaigns/:id/foundry-presence (live diagnostic JSON) and
	// /foundry-vtt/presence-pill-fragment (lazy-loaded pill on the map detail
	// page); both read through foundry_vtt.PresenceLookup, so a single
	// SetPresenceLookup call covers them.
	fvttHandler.SetPresenceLookup(wsHub)

	// DM Screen: the owner's and scribes' control panel. It owns no data and
	// reads every section through the adapters in dm_screen_adapters.go;
	// registered here because Foundry presence comes from wsHub.
	dmScreenSvc := dmscreen.NewService(dmscreen.Sources{
		Downtime: &dmDowntimeAdapter{stash: stashSvc, addons: addonService},
		World:    &dmWorldAdapter{svc: calendarService},
		Nights:   &dmNightAdapter{svc: sessionsService, members: campaignService},
		Foundry:  wsHub,
		Party:    &dmPartyAdapter{entities: entityService, campaigns: campaignService},
		Hidden:   &dmHiddenAdapter{entities: entityService, lists: characterListService},
		System:   systemHandler,
	})
	dmscreen.RegisterRoutes(e, dmscreen.NewHandler(dmScreenSvc), campaignService, authService)
	// The sync API routes are already registered; they answer 404 until this
	// is set, and it is set before the server starts serving.
	syncAPIHandler.SetDMScreen(&dmScreenSyncAPIAdapter{svc: dmScreenSvc})

	wsAuth := ws.NewMultiAuthenticator(
		syncService,
		&wsSessionAuthAdapter{svc: authService},
		&wsCampaignRoleAdapter{svc: campaignService},
	)
	// The Foundry notebook's frames hold a notes grant, not a sign-in.
	wsAuth.SetNotesGrantAuth(&wsNotesGrantAdapter{grants: noteGrants, gate: notesAppGate})
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
	// The recording wrapper appends allowlisted events to the change feed
	// before they reach the hub, so no publisher can forget to record.
	wsEventBus := ws.EventBus(syncapi.NewRecordingEventBus(ws.NewEventBus(wsHub), syncChangeRepo))
	go syncapi.StartChangePruner(a.ShutdownCtx, syncChangeRepo)

	// The quests due-date events follow a page's visibility, so the entity
	// events also reach them once the quests service exists (attached below).
	questEntityEvts := newQuestEntityEvents(&entityEventPublisherAdapter{bus: wsEventBus})
	entityService.SetEventPublisher(questEntityEvts)
	relService.SetEventPublisher(&relationEventPublisherAdapter{bus: wsEventBus, shares: stashSvc})
	stashEvents.bus = wsEventBus
	entityService.SetSidebarAutoAdder(&sidebarAutoAdderAdapter{campaignService: campaignService})
	noteSvc.SetEventPublisher(&noteEventPublisherAdapter{bus: wsEventBus})

	// "Show in Foundry" on NPC pages: npc.spotlight is not a change-feed
	// type, so the recording wrapper passes it straight to the hub.
	fvttHandler.SetNPCSpotlight(&npcSpotlightResolver{entities: entityService, lists: characterListService}, &npcSpotlightPublisher{bus: wsEventBus})

	// Game-system widget panels under NPC page titles (manifest entity_panels).
	entityHandler.SetSystemPanelResolver(newSystemPanelResolver(systemHandler, characterListService))

	// Per-page game-system state. system_state.updated is not a change-feed
	// type, so the recording wrapper passes it straight to the hub.
	if a.PluginHealth.IsHealthy(systemstate.PluginSlug) {
		systemStateSvc := systemstate.NewService(
			systemstate.NewRepository(a.DB),
			&systemStateEntityLookup{entities: entityService},
			&systemStateSystemChecker{systems: systemHandler},
			&systemStatePublisher{bus: wsEventBus},
		)
		systemstate.RegisterRoutes(e,
			systemstate.NewHandler(systemStateSvc, &entityAccessAdapter{svc: entityService}),
			campaignService, authService)
		syncAPIHandler.SetSystemStateReader(&systemStateSyncReader{svc: systemStateSvc})
	} else {
		slog.Warn("systemstate plugin degraded — routes not registered")
	}

	// Per-campaign rolling tables: the DM team edits them, scribes may roll.
	rollTablesSvc := rolltables.NewService(rolltables.NewRepository(a.DB))
	if a.PluginHealth.IsHealthy(rolltables.PluginSlug) {
		rolltables.RegisterRoutes(e, rolltables.NewHandler(rollTablesSvc), campaignService, authService)
	} else {
		slog.Warn("rolltables plugin degraded — routes not registered")
	}

	// Quest sheets and notice boards. Cross-plugin lookups go through the
	// adapters in quests_adapters.go. Both boards share quest_board_kit.js,
	// so it loads first in each list (ADR-063).
	a.registerPlugin(PluginRegistration{
		Slug:     quests.PluginSlug,
		StaticFS: echo.MustSubFS(quests.StaticAssetsFS, "static"),
		Widgets: []PluginWidget{
			{Name: "quest_board", Scripts: []string{"js/quest_board_kit.js", "js/quest_board.js"}},
			{Name: "notice_boards", Scripts: []string{"js/quest_board_kit.js", "js/notice_boards.js"}},
		},
	})
	if a.PluginHealth.IsHealthy(quests.PluginSlug) {
		questEntities := &questEntityAdapter{svc: entityService, cards: entities.NewPageCards(a.DB)}
		questMaps := &questMapAdapter{svc: mapsService, addons: addonService}
		questRepo := quests.NewQuestRepository(a.DB)
		questCal := &questCalendarAdapter{svc: calendarService, addons: addonService}
		questSvc, boardSvc := quests.WithAnnouncer(
			quests.NewQuestService(questRepo, questEntities, questMaps, questCal),
			quests.NewBoardService(quests.NewBoardRepository(a.DB), questRepo, questEntities, &questTypeAdapter{svc: entityService}, questMaps, &questMemberNamesAdapter{svc: campaignService}, questCal),
			questEntities, &questAnnouncerAdapter{bus: wsEventBus})
		questEntityEvts.attach(questSvc)
		questPicker := quests.NewPickerService(questEntities, questMaps, &questCharacterAdapter{
			dir:   stashDirectory,
			names: &questMemberNamesAdapter{svc: campaignService},
		})
		questAPI.attach(questSvc, boardSvc, questPicker)
		quests.RegisterRoutes(e, quests.NewHandler(questSvc, boardSvc, questPicker), campaignService, authService)
	} else {
		slog.Warn("quests plugin degraded — routes not registered")
	}

	// AI Import's record kinds: the features beyond pages it may write.
	// Wired here, after the rulebook and rolling tables exist; a degraded
	// plugin's kind is left out, so its blocks are refused at review.
	aiKinds := []records.Kind{
		records.CalendarKind{Svc: calendarService},
		records.EventKind{Svc: calendarService},
		records.WeatherKind{Svc: calendarService},
	}
	aiTables := records.TableKind{Svc: aiRollTablesAdapter{rollTablesSvc}}
	aiHouseRules := records.HouseRuleKind{Book: systemHandler, SystemOf: func(ctx context.Context, campaignID string) string {
		c, err := campaignService.GetByID(ctx, campaignID)
		if err != nil {
			return ""
		}
		return c.ParseSettings().SystemID
	}}
	if a.PluginHealth.IsHealthy(rolltables.PluginSlug) {
		aiKinds = append(aiKinds, aiTables)
	}
	aiKinds = append(aiKinds,
		records.ShopStockKind(entityService, relService),
		records.CarriedItemKind(entityService, relService),
		records.PinKind{Svc: aiMapsAdapter{mapsService}, Entities: entityService},
		records.NoteKind{Svc: noteSvc, Entities: entityService},
		aiHouseRules,
		records.SystemEntryKind{Svc: systemEntrySvc},
		records.GeneratorKind{Cal: calendarService, Tables: aiTables},
	)
	aiWorkspaceHandler.SetRecords(records.NewRegistry(aiKinds...))

	// The read-only lookups an AI may ask for. Game-system entries wait on
	// the character pick-list service (TODO(#1170)).
	aiLookups := &records.Lookups{
		Cal: calendarService, Weather: calendarService, Maps: aiMapsAdapter{mapsService}, Pages: entityService,
		Rels: relService, Notes: noteSvc, Rules: &aiHouseRules,
		Party: &aiPartyAdapter{screen: dmScreenSvc, nights: sessionsService, members: campaignService},
	}
	if a.PluginHealth.IsHealthy(rolltables.PluginSlug) {
		aiLookups.Tables = aiTables.Svc
	}
	aiWorkspaceHandler.SetLookups(aiLookups)

	// Late-bind the entity_notes notifier now that wsEventBus exists.
	// The service was constructed earlier with a holder.Notify reference;
	// setting holder.bus here makes future mutations broadcast over WS.
	entityNotesNotifier.bus = wsEventBus

	mapEvents := newMapEventPublisher(wsEventBus, drawingService)
	drawingService.SetEventPublisher(mapEvents)
	drawingService.SetMapLookup(func(ctx context.Context, mapID string) (string, error) {
		m, err := mapsService.GetMap(ctx, mapID)
		if err != nil {
			return "", err
		}
		return m.CampaignID, nil
	})
	// The per-map "who can draw" gate is read from the map's display settings.
	// Wired here (not in NewDrawingService) so the drawing service never needs
	// the map repository itself.
	drawingService.SetDrawPolicyLookup(func(ctx context.Context, mapID string) (string, error) {
		m, err := mapsService.GetMap(ctx, mapID)
		if err != nil {
			return "", err
		}
		return m.DrawWho(), nil
	})
	drawingService.SetMapFrameLookup(func(ctx context.Context, mapID string) (int, int, error) {
		m, err := mapsService.GetMap(ctx, mapID)
		if err != nil {
			return 0, 0, err
		}
		return m.ImageWidth, m.ImageHeight, nil
	})
	drawingService.SetMediaVerifier(&mapMediaVerifierAdapter{svc: mediaService})
	mapsService.SetEventPublisher(mapEvents)
	wireHexFog(mapsService, drawingService, mapEvents, hexService)
	hexService.SetPictures(maps.NewHexPictures(drawingService))
	hexService.SetMapLookup(func(ctx context.Context, mapID string) (string, error) {
		m, err := mapsService.GetMap(ctx, mapID)
		if err != nil {
			return "", err
		}
		return m.CampaignID, nil
	})
	// Painting terrain is drawing on the map, so it follows the same gate.
	hexService.SetDrawPolicyLookup(func(ctx context.Context, mapID string) (string, error) {
		m, err := mapsService.GetMap(ctx, mapID)
		if err != nil {
			return "", err
		}
		return m.DrawWho(), nil
	})

	// --- Module Routes ---
	// Game system reference pages and tooltip APIs.
	// ref := e.Group("/ref")
	// dnd5eModule.RegisterRoutes(ref)

	// --- API Routes ---
	// REST API v1 is registered above via syncapi.RegisterAPIRoutes().
	// Endpoints: /api/v1/campaigns/:id/{entity-types,entities,sync}

	// --- Plugin Static Assets ---
	// Mount each registered plugin's static assets at /static/plugins/<slug>/.
	// Must run AFTER all plugins have called a.registerPlugin() above.
	a.mountPluginStatic()

	// A wiring mistake leaves only the widgets it names unloaded, so it is
	// logged rather than stopping the site.
	m, err := buildWidgetManifest(a.registeredPlugins, layouts.AssetURL)
	if err != nil {
		slog.Error("on-sight widget manifest", slog.Any("error", err))
	}
	widgetScripts = m
}

// journalCharacterAdapter adapts EntityService to notes.CharacterLister: the
// characters a player has claimed, most recently played first.
type journalCharacterAdapter struct {
	svc entities.EntityService
}

// ClaimedCharacters lists userID's characters in the campaign.
func (a *journalCharacterAdapter) ClaimedCharacters(ctx context.Context, campaignID, userID string) ([]notes.ClaimedCharacter, error) {
	if userID == "" {
		return nil, nil
	}
	owned, err := a.svc.ListByOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]notes.ClaimedCharacter, 0, len(owned))
	for _, e := range owned {
		out = append(out, notes.ClaimedCharacter{ID: e.ID, Name: e.Name, TypeName: e.TypeName})
	}
	return out, nil
}

// notesPagesAdapter adapts EntityService to notes.PageNamer and
// notes.PageLinker: pages named and listed only as far as the viewer may
// see them.
type notesPagesAdapter struct {
	svc entities.EntityService
}

// PageNames names the ids the viewer can see; the rest are left out.
func (a *notesPagesAdapter) PageNames(ctx context.Context, campaignID string, v permissions.Viewer, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		e, err := a.svc.GetByID(ctx, id)
		if err != nil || e == nil || e.CampaignID != campaignID {
			continue
		}
		perm, err := a.svc.CheckEntityAccess(ctx, id, v.Role(), v.UserID())
		if err != nil || perm == nil || !perm.CanView {
			continue
		}
		out[id] = e.Name
	}
	return out, nil
}

// PagesLinkingNote lists the visible pages that link a note, by name only.
func (a *notesPagesAdapter) PagesLinkingNote(ctx context.Context, campaignID string, v permissions.Viewer, seesSecrets bool, noteID string) ([]notes.PageRef, error) {
	pages, err := a.svc.PagesLinkingNote(ctx, campaignID, noteID, v.Role(), v.UserID(), seesSecrets)
	if err != nil {
		return nil, err
	}
	out := make([]notes.PageRef, 0, len(pages))
	for _, p := range pages {
		out = append(out, notes.PageRef{ID: p.ID, Name: p.Name, TypeName: p.TypeName})
	}
	return out, nil
}

// skyboxCalendarService is the narrow seam renderSkyboxBlock needs from
// calendar.CalendarService — small enough that a test double
// (skybox_block_test.go) doesn't have to implement the plugin's whole
// surface. calendar.CalendarService satisfies this structurally, so
// RegisterRoutes passes it straight through with no adapter.
type skyboxCalendarService interface {
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*calendar.Calendar, error)
}

// renderSkyboxBlock resolves rc's viewer and campaign, then renders the
// "skybox" block (see its BlockRegistry.Register call above) for it.
// Extracted out of that block's registration closure purely so it can be
// unit tested without spinning up the whole app — see skybox_block_test.go.
//
// Failure modes:
//   - no campaign in context: an empty slot (nothing to resolve against).
//   - GetDefaultCalendarForViewer's NotFound (no default calendar yet, OR one
//     the viewer may not see — the two collapse identically, see that
//     method's own doc comment): sky.Empty's quiet placeholder, not a crash.
//   - any other error: an infra problem, not a policy outcome — logged and
//     failed safe to an empty slot, same as the npc_gallery/armory_preview
//     blocks' own error handling above: an ambient dashboard/entity-page
//     decoration never turns into a 500.
func renderSkyboxBlock(ctx context.Context, svc skyboxCalendarService, rc entities.BlockRenderContext) templ.Component {
	if rc.CC == nil || rc.CC.Campaign == nil {
		return templ.NopComponent
	}
	campaignID := rc.CC.Campaign.ID
	// Same viewer construction as calendar.viewerFrom (handler.go): promoted
	// VisibilityRole + the request's user id, empty for an anonymous
	// public-campaign visitor — this block has no echo.Context to build it
	// the handler's own way.
	viewer := permissions.RequestViewer(rc.CC.VisibilityRole(), rc.UserID)
	cal, err := svc.GetDefaultCalendarForViewer(ctx, campaignID, viewer)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return skywidget.Empty(campaignID)
		}
		slog.Error("skybox block: get default calendar for viewer",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return templ.NopComponent
	}
	return skywidget.Mount(campaignID, cal.ID)
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

// UploadPicture stores a picture written into a note and returns its media id.
func (a *mediaUploadAdapter) UploadPicture(ctx context.Context, campaignID, userID string, fileBytes []byte, originalName, mimeType string) (string, error) {
	file, err := a.svc.Upload(ctx, media.UploadInput{
		CampaignID:   campaignID,
		UploadedBy:   userID,
		OriginalName: originalName,
		MimeType:     mimeType,
		FileSize:     int64(len(fileBytes)),
		UsageType:    media.UsageNoteImage,
		FileBytes:    fileBytes,
	})
	if err != nil {
		return "", err
	}
	return file.ID, nil
}

// noteMediaAccessAdapter answers the media plugin's note-picture question
// over the notes service, so media never touches note tables.
type noteMediaAccessAdapter struct {
	svc notes.NoteService
}

func (a *noteMediaAccessAdapter) CanReadNoteMedia(ctx context.Context, campaignID, mediaID string, role int, userID string) (bool, error) {
	return a.svc.ViewerReadsMedia(ctx, campaignID, mediaID, permissions.RequestViewer(role, userID))
}

// pageAccessAdapter answers the media plugin's page-file question over the
// entities service, so media never reads entity tables or repeats the page
// visibility rule.
type pageAccessAdapter struct {
	svc entities.EntityService
}

// PageAccess says whether the viewer can see the page (which is false for a
// page that is missing, in another campaign, in the Trash, or hidden from
// them) and whether they can edit it. Editing is only asked of a page already
// found visible, because CheckEntityAccess alone does not scope to a campaign.
func (a *pageAccessAdapter) PageAccess(ctx context.Context, campaignID, entityID string, role int, userID string) (media.PageAccess, error) {
	viewable, err := a.svc.FilterViewableEntityIDs(ctx, campaignID, []string{entityID}, role, userID)
	if err != nil || !viewable[entityID] {
		return media.PageAccess{}, err
	}
	ep, err := a.svc.CheckEntityAccess(ctx, entityID, role, userID)
	if err != nil {
		return media.PageAccess{}, err
	}
	return media.PageAccess{CanView: true, CanEdit: ep.CanEdit}, nil
}

// aiMapsAdapter is the maps service in AI Import's pin types, so the
// ai_workspace plugin never imports maps.
type aiMapsAdapter struct{ svc maps.MapService }

func (m aiMapsAdapter) ListMaps(ctx context.Context, campaignID string) ([]records.MapRef, error) {
	ms, err := m.svc.ListMaps(ctx, campaignID)
	out := make([]records.MapRef, len(ms))
	for i, x := range ms {
		out[i] = records.MapRef{ID: x.ID, CampaignID: x.CampaignID, Name: x.Name}
	}
	return out, err
}

func (m aiMapsAdapter) ListPins(ctx context.Context, campaignID, mapID string, role int, userID string) ([]records.PinRef, error) {
	ps, err := m.svc.ListMarkers(ctx, campaignID, mapID, role, userID)
	out := make([]records.PinRef, len(ps))
	for i, x := range ps {
		out[i] = records.PinRef{ID: x.ID, Name: x.Name, X: x.X, Y: x.Y}
	}
	return out, err
}

func (m aiMapsAdapter) CreatePin(ctx context.Context, mapID, userID string, in records.PinInput) error {
	c := maps.CreateMarkerInput{MapID: mapID, Name: in.Name, Icon: in.Icon, Color: in.Color,
		Visibility: in.Visibility, Description: in.Description, EntityID: in.EntityID, CreatedBy: userID}
	if in.X != nil {
		c.X = *in.X
	}
	if in.Y != nil {
		c.Y = *in.Y
	}
	if in.Category != "" {
		c.PinCategory = &in.Category
	}
	_, err := m.svc.CreateMarker(ctx, c)
	return err
}

func (m aiMapsAdapter) UpdatePin(ctx context.Context, pinID string, in records.PinInput, canAuthorDmOnly bool) error {
	var u maps.UpdateMarkerInput
	set := func(f *patch.Field[string], v string) {
		if v != "" {
			*f = patch.Of(v)
		}
	}
	set(&u.Name, in.Name)
	set(&u.Icon, in.Icon)
	set(&u.Color, in.Color)
	set(&u.PinCategory, in.Category)
	set(&u.Visibility, in.Visibility)
	// nil means "not written": leave the stored value (never FromPtr,
	// which would clear it).
	if in.Description != nil {
		u.Description = patch.Of(*in.Description)
	}
	if in.EntityID != nil {
		u.EntityID = patch.Of(*in.EntityID)
	}
	if in.X != nil {
		u.X = patch.Of(*in.X)
	}
	if in.Y != nil {
		u.Y = patch.Of(*in.Y)
	}
	return m.svc.UpdateMarker(ctx, pinID, u, canAuthorDmOnly)
}

// DeletePin skips the concurrency token: the pin was read from its map in
// the same request.
func (m aiMapsAdapter) DeletePin(ctx context.Context, pinID string, canAuthorDmOnly bool, actorID string, role int) error {
	return m.svc.DeleteMarker(ctx, pinID, nil, canAuthorDmOnly, actorID, role)
}

// aiRollTablesAdapter is the rolltables service as AI Import's TableDoc,
// a JSON round trip, so the ai_workspace plugin never imports rolltables.
type aiRollTablesAdapter struct{ svc rolltables.Service }

func (r aiRollTablesAdapter) Get(ctx context.Context, campaignID string) (records.TableDoc, error) {
	var out records.TableDoc
	doc, err := r.svc.Get(ctx, campaignID)
	if err != nil {
		return out, err
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(b, &out)
}

func (r aiRollTablesAdapter) Put(ctx context.Context, campaignID string, doc records.TableDoc, userID string) error {
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	_, err = r.svc.Put(ctx, campaignID, b, userID)
	return err
}

// aiWorkspaceAuditAdapter bridges audit.AuditService to the narrow
// ai_workspace.AuditLogger contract, keeping the plugin from importing
// the audit package directly (plugin isolation).
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

// notesOriginAllower is the notes Allow window's origin check: the site's own
// address or an admin-allowed cross-site origin, exactly the list the CORS
// middleware in app.go uses.
type notesOriginAllower struct {
	baseURL  string
	settings settings.SettingsService
}

func (a *notesOriginAllower) AllowedOrigins(ctx context.Context) []string {
	out := []string{strings.TrimRight(a.baseURL, "/")}
	if a.settings != nil {
		if list, err := a.settings.GetCORSOrigins(ctx); err == nil {
			out = append(out, list...)
		}
	}
	return out
}

func (a *notesOriginAllower) OriginAllowed(ctx context.Context, origin string) bool {
	if origin == "" {
		return false
	}
	if strings.EqualFold(strings.TrimRight(a.baseURL, "/"), origin) {
		return true
	}
	if a.settings == nil {
		return false
	}
	list, err := a.settings.GetCORSOrigins(ctx)
	if err != nil {
		return false
	}
	for _, o := range list {
		if strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

// accountDeletionAdapter gives auth's account deletion the campaign-side
// steps without auth importing the campaigns plugin.
type accountDeletionAdapter struct {
	campaigns campaigns.CampaignService
}

func (a *accountDeletionAdapter) OwnedCampaigns(ctx context.Context, userID string) ([]auth.OwnedCampaignRef, error) {
	owned, err := a.campaigns.OwnedCampaigns(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]auth.OwnedCampaignRef, len(owned))
	for i, o := range owned {
		out[i] = auth.OwnedCampaignRef{ID: o.ID, Name: o.Name, MemberCount: o.MemberCount}
	}
	return out, nil
}

func (a *accountDeletionAdapter) LeaveAllCampaigns(ctx context.Context, userID string) error {
	return a.campaigns.LeaveAllForDeletedAccount(ctx, userID)
}

// forgetDeletedAccount clears what other plugins hold about a deleted
// account: its characters' link to it, its private notes and its sync API
// keys. Each repository is type-asserted so a test double without the
// method just skips that step; a failure is logged and the rest still run.
func forgetDeletedAccount(ctx context.Context, userID string, entityRepo, entityNotesRepo, noteRepo any, syncSvc syncapi.SyncAPIService) {
	type cleaner func(context.Context, string) (int64, error)
	steps := map[string]cleaner{}
	if r, ok := entityRepo.(interface {
		ClearOwnerForUser(context.Context, string) (int64, error)
	}); ok {
		steps["character links"] = r.ClearOwnerForUser
	}
	if r, ok := entityNotesRepo.(interface {
		DeletePrivateByAuthor(context.Context, string) (int64, error)
	}); ok {
		steps["private page notes"] = r.DeletePrivateByAuthor
	}
	if r, ok := noteRepo.(interface {
		DeletePrivateByUser(context.Context, string) (int64, error)
	}); ok {
		steps["private notes"] = r.DeletePrivateByUser
	}
	for name, fn := range steps {
		if _, err := fn(ctx, userID); err != nil {
			slog.Error("clearing deleted account data failed", slog.String("step", name), slog.String("user_id", userID), slog.Any("error", err))
		}
	}
	if syncSvc == nil {
		return
	}
	keys, err := syncSvc.ListKeysByUser(ctx, userID)
	if err != nil {
		slog.Error("listing deleted account's API keys failed", slog.String("user_id", userID), slog.Any("error", err))
		return
	}
	for _, k := range keys {
		if err := syncSvc.DeactivateKey(ctx, k.ID); err != nil {
			slog.Error("deactivating deleted account's API key failed", slog.Int("key_id", k.ID), slog.Any("error", err))
		}
	}
}
