package campaigns

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
	"github.com/keyxmakerx/chronicle/internal/sitelook"
)

// transferTokenBytes is the number of random bytes in a transfer token.
const transferTokenBytes = 32

// transferExpiryHours is how long a transfer token remains valid.
const transferExpiryHours = 72

// CampaignService handles business logic for campaign operations.
// It owns slug generation, membership rules, and ownership transfers.
type CampaignService interface {
	// Campaign CRUD
	Create(ctx context.Context, userID string, input CreateCampaignInput) (*Campaign, error)
	GetByID(ctx context.Context, id string) (*Campaign, error)
	GetBySlug(ctx context.Context, slug string) (*Campaign, error)
	List(ctx context.Context, userID string, opts ListOptions) ([]Campaign, int, error)
	ListAll(ctx context.Context, opts ListOptions) ([]Campaign, int, error)
	SearchAll(ctx context.Context, opts AdminSearchOptions) ([]Campaign, error)
	CountBySystem(ctx context.Context, query string) ([]SystemCount, error)
	ListPublic(ctx context.Context, limit int) ([]Campaign, error)
	Update(ctx context.Context, campaignID string, input UpdateCampaignInput) (*Campaign, error)
	// MoveToTrash is what "delete campaign" means: the campaign disappears for
	// everyone and waits in the site Trash (trash_repository.go). PurgeTrashed
	// is the only real delete.
	MoveToTrash(ctx context.Context, campaignID, byUserID string) error
	RestoreFromTrash(ctx context.Context, campaignID string) error
	ListTrashed(ctx context.Context) ([]TrashedCampaign, error)
	ListPurgeDue(ctx context.Context, cutoff time.Time, all bool) ([]string, error)
	PurgeTrashed(ctx context.Context, campaignID string, olderThan time.Time) (bool, error)
	CountAll(ctx context.Context) (int, error)

	// Membership
	GetMember(ctx context.Context, campaignID, userID string) (*CampaignMember, error)
	AddMember(ctx context.Context, campaignID, email string, role Role) error
	RemoveMember(ctx context.Context, campaignID, userID string) error
	UpdateMemberRole(ctx context.Context, campaignID, userID string, role Role) error
	// SetMemberAccess applies a People role-menu choice: player, scribe or
	// codm (a Scribe with the DM-only grant).
	SetMemberAccess(ctx context.Context, campaignID, userID, access string) error
	UpdateMemberCharacter(ctx context.Context, campaignID, userID string, characterEntityID *string) error
	ListMembers(ctx context.Context, campaignID string) ([]CampaignMember, error)

	// Ownership transfer
	InitiateTransfer(ctx context.Context, campaignID, ownerID, targetEmail string) (*OwnershipTransfer, error)
	AcceptTransfer(ctx context.Context, token string, acceptingUserID string) error
	CancelTransfer(ctx context.Context, campaignID string) error
	GetPendingTransfer(ctx context.Context, campaignID string) (*OwnershipTransfer, error)

	// Backdrop and branding
	// UpdateBranding sets the campaign's custom brand name and logo path.
	UpdateBranding(ctx context.Context, campaignID, brandName, brandLogo string) error
	UpdateDmGrants(ctx context.Context, campaignID string, userIDs []string) error
	// IsUserDmGranted reports whether the user appears in the campaign's
	// DmGrantIDs setting. Used by the WS hub to gate RequiresDM messages
	// to non-Owner members the Owner has trusted with dm_only visibility.
	IsUserDmGranted(ctx context.Context, campaignID, userID string) (bool, error)

	// SetFoundryModulePin pins the campaign to a specific Foundry
	// module version (or clears the pin when version is "").
	SetFoundryModulePin(ctx context.Context, campaignID, version string) error
	// GetFoundryModulePin returns the campaign's current Foundry
	// module pin, or "" if none.
	GetFoundryModulePin(ctx context.Context, campaignID string) (string, error)
	// SetFoundryModulePinMode writes the campaign's pin_mode setting
	// (one of foundry_vtt.PinMode* constants). Validation is the
	// caller's responsibility, via foundry_vtt.IsValidPinMode.
	SetFoundryModulePinMode(ctx context.Context, campaignID, mode string) error
	// GetFoundryModulePinMode returns the campaign's pin_mode, or
	// empty string if not yet set.
	GetFoundryModulePinMode(ctx context.Context, campaignID string) (string, error)
	// CampaignExistsByID is a thin existence check for adapters that
	// don't need the full Campaign struct.
	CampaignExistsByID(ctx context.Context, campaignID string) (bool, error)
	// UpdateWelcomeMessage sets the campaign's MOTD banner message.
	UpdateWelcomeMessage(ctx context.Context, campaignID, message string) error
	// SaveAppearance writes one Save from the Customize page, every
	// setting at once.
	SaveAppearance(ctx context.Context, campaignID string, in AppearanceInput) error

	// GetEventTierDefinitions returns the campaign's event tier vocabulary.
	// Returns the platform default trio (major/standard/detail) when the
	// campaign has no custom tier defs set — matches the empty-means-
	// default semantic used for AccentColor / FontFamily / etc.
	GetEventTierDefinitions(ctx context.Context, campaignID string) ([]TierDefinition, error)
	// SetEventTierDefinitions replaces the campaign's tier vocabulary.
	// Validates exactly-one-default, non-empty array, unique slugs,
	// hex color format, prominence range.
	SetEventTierDefinitions(ctx context.Context, campaignID string, defs []TierDefinition) error
	// UpdateDefaultVisibility sets the default visibility for new entities.
	UpdateDefaultVisibility(ctx context.Context, campaignID, visibility string) error

	// Sidebar configuration
	UpdateSidebarConfig(ctx context.Context, campaignID string, req UpdateSidebarConfigRequest) error
	GetSidebarConfig(ctx context.Context, campaignID string) (*SidebarConfig, error)
	// EnsureSidebarItems is the one-time, idempotent boot reconciler that
	// converts campaigns still on the legacy sidebar model onto the unified
	// items model. Returns the number of campaigns converted.
	EnsureSidebarItems(ctx context.Context) (int, error)
	// NavPins returns a member's own pinned sidebar rows, in the order pinned.
	NavPins(ctx context.Context, campaignID, userID string) ([]string, error)
	// UpdateNavPins replaces the calling member's own pinned sidebar rows and
	// returns what was stored. Owners are refused (they pin for everyone in
	// the sidebar editor), and so is any row the member's sidebar does not show.
	UpdateNavPins(ctx context.Context, cc *CampaignContext, userID string, pins []string) ([]string, error)

	// Dashboard layout
	UpdateDashboardLayout(ctx context.Context, campaignID string, layout *DashboardLayout) error
	UpdateDashboardLayoutRaw(ctx context.Context, campaignID string, layoutJSON *string) error
	GetDashboardLayout(ctx context.Context, campaignID string) (*DashboardLayout, error)
	ResetDashboardLayout(ctx context.Context, campaignID string) error

	// Owner dashboard layout
	UpdateOwnerDashboardLayout(ctx context.Context, campaignID string, layout *DashboardLayout) error
	GetOwnerDashboardLayout(ctx context.Context, campaignID string) (*DashboardLayout, error)
	ResetOwnerDashboardLayout(ctx context.Context, campaignID string) error

	// Admin operations
	ForceTransferOwnership(ctx context.Context, campaignID, newOwnerID string) error
	AdminAddMember(ctx context.Context, campaignID, userID string, role Role) error

	// Archive
	ArchiveCampaign(ctx context.Context, campaignID string) error
	UnarchiveCampaign(ctx context.Context, campaignID string) error

	// Shareable invite link
	GenerateJoinCode(ctx context.Context, campaignID string) (string, error)
	RevokeJoinCode(ctx context.Context, campaignID string) error
	JoinByCode(ctx context.Context, code, userID string) error

	// Game system
	UpdateSystemID(ctx context.Context, campaignID, systemID string) error

	// Lifecycle hooks — set after construction to avoid circular initialization.
	SetContentTemplateSeeder(seeder ContentTemplateSeeder)
	SetWorldbuildingPromptSeeder(seeder WorldbuildingPromptSeeder)
	SetLayoutPresetSeeder(seeder LayoutPresetSeeder)

	// SetCharacterListSeeder sets what records a new campaign's character and
	// NPC page types. Late-bound and nil-safe.
	SetCharacterListSeeder(seeder CharacterListSeeder)

	// UpdateCharacterLists stores the page types the campaign lists as
	// characters and as NPCs. Callers validate the IDs; this only persists.
	UpdateCharacterLists(ctx context.Context, campaignID string, characterTypeIDs, npcTypeIDs []int) error
	SetMediaCleaner(cleaner MediaCleaner)
	SetHookDispatcher(dispatcher CampaignHookDispatcher)

	// SetConnectionRevoker injects the hub's revocation surface. Late-bound
	// and nil-safe: a nil revoker only skips the live-drop, since a
	// reconnect re-resolves role/grant from current settings anyway.
	SetConnectionRevoker(cr ConnectionRevoker)

	// SetNavSectionsSource injects what draws a member's sidebar, which
	// UpdateNavPins checks pins against. Until it is set, pins are refused.
	SetNavSectionsSource(src NavSectionsSource)

	// SetSiteLookSource injects the site-wide look, so a new campaign can
	// start with it. Late-bound and nil-safe: without it new campaigns start
	// as Classic.
	SetSiteLookSource(src SiteLookSource)
}

// SiteLookSource reads the site-wide look an admin chose. The settings
// service implements it; campaigns reaches it only through this interface so
// the two plugins stay independent.
type SiteLookSource interface {
	GetSiteLook(ctx context.Context) (sitelook.Settings, error)
}

// NavSectionsSource draws a member's sidebar as ViewNav does, before their
// own pins are applied. The composition root provides it, because only it
// knows every plugin's apps and addons.
type NavSectionsSource interface {
	NavSectionsFor(ctx context.Context, cc *CampaignContext) ([]NavSection, error)
}

// ConnectionRevoker is the narrow hub view this plugin needs: drop a
// user's sockets when their access is lowered, or a whole campaign's
// once it's deleted. Declared locally, not imported.
type ConnectionRevoker interface {
	RevokeUser(campaignID, userID string)
	RevokeCampaign(campaignID string)
}

// GroupService handles business logic for campaign group operations.
type GroupService interface {
	CreateGroup(ctx context.Context, campaignID, name string, description *string) (*CampaignGroup, error)
	ListGroups(ctx context.Context, campaignID string) ([]CampaignGroup, error)
	GetGroup(ctx context.Context, groupID int) (*CampaignGroup, error)
	UpdateGroup(ctx context.Context, groupID int, name string, description *string) error
	DeleteGroup(ctx context.Context, groupID int) error
	AddGroupMember(ctx context.Context, groupID int, userID string) error
	RemoveGroupMember(ctx context.Context, groupID int, userID string) error
	ListGroupMembers(ctx context.Context, groupID int) ([]GroupMemberInfo, error)
}

// MediaCleaner handles bulk media file cleanup during campaign deletion.
// Implemented by the media service.
type MediaCleaner interface {
	DeleteCampaignFiles(ctx context.Context, campaignID string) (int, error)
}

// CampaignHookDispatcher fires lifecycle events for WASM plugin notification.
// Implemented by the extensions HookDispatcher.
type CampaignHookDispatcher interface {
	DispatchCampaignDeleted(ctx context.Context, campaignID string)
}

// campaignService implements CampaignService.
type campaignService struct {
	repo           CampaignRepository
	users          UserFinder
	mail           MailService            // May be nil if SMTP is not configured.
	seeder           EntityTypeSeeder       // Seeds default entity types on campaign creation. May be nil.
	templateSeeder   ContentTemplateSeeder  // Seeds default content templates on campaign creation. May be nil.
	promptSeeder     WorldbuildingPromptSeeder // Seeds default worldbuilding prompts on campaign creation. May be nil.
	layoutSeeder     LayoutPresetSeeder     // Seeds default layout presets on campaign creation. May be nil.
	characterSeeder  CharacterListSeeder    // Records the character/NPC page types on campaign creation. May be nil.
	mediaCleaner     MediaCleaner           // Cleans up media files on campaign delete. May be nil.
	hookDispatcher   CampaignHookDispatcher // Dispatches WASM lifecycle events. May be nil.
	connRevoker      ConnectionRevoker      // Drops live sockets when access is lowered. May be nil until wiring reaches it.
	navSections      NavSectionsSource      // Draws a member's sidebar for pin checks. Pins are refused while nil.
	siteLook         SiteLookSource         // Site-wide look new campaigns start with. May be nil.
	baseURL          string
	// memberRemoved runs after a member is removed, so other plugins can end
	// credentials they issued for that campaign without campaigns importing them.
	memberRemoved []func(ctx context.Context, campaignID, userID string)
}

// OnMemberRemoved registers fn to run after a user is removed from a campaign.
func OnMemberRemoved(svc CampaignService, fn func(ctx context.Context, campaignID, userID string)) {
	if s, ok := svc.(*campaignService); ok && fn != nil {
		s.memberRemoved = append(s.memberRemoved, fn)
	}
}

// NewCampaignService creates a new campaign service with the given dependencies.
// The mail and seeder parameters may be nil if the corresponding plugins are not yet wired.
func NewCampaignService(repo CampaignRepository, users UserFinder, mail MailService, seeder EntityTypeSeeder, baseURL string) CampaignService {
	return &campaignService{
		repo:    repo,
		users:   users,
		mail:    mail,
		seeder:  seeder,
		baseURL: baseURL,
	}
}

// SetContentTemplateSeeder sets the seeder for default content templates.
// Called after all plugins are wired to avoid initialization order issues.
func (s *campaignService) SetContentTemplateSeeder(seeder ContentTemplateSeeder) {
	s.templateSeeder = seeder
}

// SetWorldbuildingPromptSeeder sets the seeder for default worldbuilding prompts.
func (s *campaignService) SetWorldbuildingPromptSeeder(seeder WorldbuildingPromptSeeder) {
	s.promptSeeder = seeder
}

// SetLayoutPresetSeeder sets the seeder for default layout presets.
// Called after all plugins are wired to avoid initialization order issues.
func (s *campaignService) SetLayoutPresetSeeder(seeder LayoutPresetSeeder) {
	s.layoutSeeder = seeder
}

// SetCharacterListSeeder sets the seeder for the character and NPC page types.
func (s *campaignService) SetCharacterListSeeder(seeder CharacterListSeeder) {
	s.characterSeeder = seeder
}

// UpdateCharacterLists replaces both lists in the settings JSON, leaving every
// other setting as it was. A nil slice is stored as an empty list so the
// campaign counts as having chosen.
func (s *campaignService) UpdateCharacterLists(ctx context.Context, campaignID string, characterTypeIDs, npcTypeIDs []int) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign == nil {
		return apperror.NewNotFound("campaign not found")
	}
	if characterTypeIDs == nil {
		characterTypeIDs = []int{}
	}
	if npcTypeIDs == nil {
		npcTypeIDs = []int{}
	}
	settings := campaign.ParseSettings()
	settings.CharacterTypeIDs = &characterTypeIDs
	settings.NPCTypeIDs = &npcTypeIDs
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}
	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// SetMediaCleaner sets the media cleaner for campaign deletion cleanup.
// Called after all plugins are wired to avoid initialization order issues.
func (s *campaignService) SetMediaCleaner(cleaner MediaCleaner) {
	s.mediaCleaner = cleaner
}

// SetHookDispatcher sets the WASM hook dispatcher for campaign lifecycle events.
// Called after all plugins are wired to avoid initialization order issues.
func (s *campaignService) SetHookDispatcher(dispatcher CampaignHookDispatcher) {
	s.hookDispatcher = dispatcher
}

// SetSiteLookSource injects the site-wide look new campaigns start with.
func (s *campaignService) SetSiteLookSource(src SiteLookSource) {
	s.siteLook = src
}

// newCampaignSettings is the settings JSON a new campaign is created with.
// When the admin has set a site look, the campaign is seeded with that look's
// full style, the same values picking it on Customize and saving would store,
// so it renders in the site look at once and Customize opens on it. Classic is the Appearance
// default and is stored as empty, like every other default. A failed read
// only means the campaign starts as Classic: creating a campaign must not
// depend on the site look.
func (s *campaignService) newCampaignSettings(ctx context.Context) string {
	if s.siteLook == nil {
		return "{}"
	}
	look, err := s.siteLook.GetSiteLook(ctx)
	if err != nil {
		slog.Warn("reading the site look for a new campaign", slog.Any("error", err))
		return "{}"
	}
	if look.Look == "" || look.Look == AppearanceLooks[0] {
		return "{}"
	}
	var settings CampaignSettings
	if !seedLook(&settings, look.Look) {
		return "{}"
	}
	b, err := json.Marshal(settings)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// SetNavSectionsSource injects what draws a member's sidebar for pin checks.
func (s *campaignService) SetNavSectionsSource(src NavSectionsSource) {
	s.navSections = src
}

// NavPins returns a member's own pinned sidebar rows.
func (s *campaignService) NavPins(ctx context.Context, campaignID, userID string) ([]string, error) {
	if campaignID == "" || userID == "" {
		return nil, nil
	}
	return s.repo.GetMemberNavPins(ctx, campaignID, userID)
}

// UpdateNavPins stores the calling member's own pinned sidebar rows. Every
// pin is checked against the sidebar that member sees right now, so a pin can
// never name a row hidden from players or an app they cannot open, and the
// row written is the caller's own (campaign and user together).
func (s *campaignService) UpdateNavPins(ctx context.Context, cc *CampaignContext, userID string, pins []string) ([]string, error) {
	if cc == nil || cc.Campaign == nil || !cc.IsMember || cc.MemberRole < RolePlayer || userID == "" {
		return nil, apperror.NewForbidden("only members of this campaign can pin rows")
	}
	if cc.MemberRole >= RoleOwner {
		return nil, apperror.NewForbidden("the owner pins rows for everyone in the sidebar editor")
	}
	if s.navSections == nil {
		return nil, apperror.NewInternal(fmt.Errorf("nav sections source is not wired"))
	}
	secs, err := s.navSections.NavSectionsFor(ctx, cc)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("drawing the member's sidebar: %w", err))
	}
	clean, err := CleanNavPins(pins, PinnableNavKeys(secs))
	if err != nil {
		return nil, err
	}
	if err := s.repo.SetMemberNavPins(ctx, cc.Campaign.ID, userID, clean); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return clean, nil
}

// SetConnectionRevoker injects the WebSocket hub's revocation surface.
func (s *campaignService) SetConnectionRevoker(cr ConnectionRevoker) {
	s.connRevoker = cr
}

// --- Campaign CRUD ---

// Create creates a new campaign and automatically adds the creator as Owner.
func (s *campaignService) Create(ctx context.Context, userID string, input CreateCampaignInput) (*Campaign, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("campaign name is required")
	}
	if len(name) > 200 {
		return nil, apperror.NewBadRequest("campaign name must be at most 200 characters")
	}

	desc := strings.TrimSpace(input.Description)
	if len(desc) > 5000 {
		return nil, apperror.NewBadRequest("description must be at most 5000 characters")
	}

	// Generate a unique slug from the name.
	slug, err := s.generateSlug(ctx, name)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("generating slug: %w", err))
	}

	now := time.Now().UTC()
	var descPtr *string
	if desc != "" {
		descPtr = &desc
	}

	campaign := &Campaign{
		ID:            generateUUID(),
		Name:          name,
		Slug:          slug,
		Description:   descPtr,
		Settings:      s.newCampaignSettings(ctx),
		SidebarConfig: "{}",
		CreatedBy:     userID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.repo.Create(ctx, campaign); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating campaign: %w", err))
	}

	// Auto-add the creator as Owner.
	member := &CampaignMember{
		CampaignID: campaign.ID,
		UserID:     userID,
		Role:       RoleOwner,
		JoinedAt:   now,
	}
	if err := s.repo.AddMember(ctx, member); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("adding owner member: %w", err))
	}

	// Seed entity types for the new campaign (genre-specific or defaults).
	if s.seeder != nil && !input.SkipEntityTypeSeed {
		var seedErr error
		if input.Genre != "" {
			seedErr = s.seeder.SeedGenre(ctx, campaign.ID, input.Genre)
		} else {
			seedErr = s.seeder.SeedDefaults(ctx, campaign.ID)
		}
		if seedErr != nil {
			slog.Warn("failed to seed entity types",
				slog.String("campaign_id", campaign.ID),
				slog.String("genre", input.Genre),
				slog.Any("error", seedErr),
			)
		}
	}

	// Seed default content templates for the new campaign.
	if s.templateSeeder != nil {
		if err := s.templateSeeder.SeedDefaults(ctx, campaign.ID); err != nil {
			slog.Warn("failed to seed default content templates",
				slog.String("campaign_id", campaign.ID),
				slog.Any("error", err),
			)
		}
	}

	// Seed default worldbuilding prompts for the new campaign.
	if s.promptSeeder != nil {
		if err := s.promptSeeder.SeedDefaults(ctx, campaign.ID); err != nil {
			slog.Warn("failed to seed default worldbuilding prompts",
				slog.String("campaign_id", campaign.ID),
				slog.Any("error", err),
			)
		}
	}

	// Seed default layout presets for the new campaign.
	if s.layoutSeeder != nil {
		if err := s.layoutSeeder.SeedDefaults(ctx, campaign.ID); err != nil {
			slog.Warn("failed to seed default layout presets",
				slog.String("campaign_id", campaign.ID),
				slog.Any("error", err),
			)
		}
	}

	// Record which page types the new campaign lists as characters and NPCs,
	// now that its entity types exist.
	if s.characterSeeder != nil {
		if err := s.characterSeeder.Seed(ctx, campaign.ID); err != nil {
			slog.Warn("failed to seed character lists",
				slog.String("campaign_id", campaign.ID),
				slog.Any("error", err),
			)
		}
	}

	slog.Info("campaign created",
		slog.String("campaign_id", campaign.ID),
		slog.String("slug", campaign.Slug),
		slog.String("user_id", userID),
	)

	return campaign, nil
}

// GetByID retrieves a campaign by ID.
func (s *campaignService) GetByID(ctx context.Context, id string) (*Campaign, error) {
	return s.repo.FindByID(ctx, id)
}

// GetBySlug retrieves a campaign by its URL slug.
func (s *campaignService) GetBySlug(ctx context.Context, slug string) (*Campaign, error) {
	return s.repo.FindBySlug(ctx, slug)
}

// List returns campaigns the user is a member of.
func (s *campaignService) List(ctx context.Context, userID string, opts ListOptions) ([]Campaign, int, error) {
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 24
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	return s.repo.ListByUser(ctx, userID, opts)
}

// ListAll returns all campaigns. Admin only.
func (s *campaignService) ListAll(ctx context.Context, opts ListOptions) ([]Campaign, int, error) {
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 24
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	return s.repo.ListAll(ctx, opts)
}

// ListPublic returns public campaigns for the landing page. Clamps the limit
// to a sane range to prevent abuse via URL parameter manipulation.
func (s *campaignService) ListPublic(ctx context.Context, limit int) ([]Campaign, error) {
	if limit < 1 || limit > 50 {
		limit = 12
	}
	return s.repo.ListPublic(ctx, limit)
}

// Update modifies a campaign's name and description.
func (s *campaignService) Update(ctx context.Context, campaignID string, input UpdateCampaignInput) (*Campaign, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("campaign name is required")
	}
	if len(name) > 200 {
		return nil, apperror.NewBadRequest("campaign name must be at most 200 characters")
	}

	desc := strings.TrimSpace(input.Description)
	if len(desc) > 5000 {
		return nil, apperror.NewBadRequest("description must be at most 5000 characters")
	}

	// Regenerate slug if name changed.
	if name != campaign.Name {
		slug, err := s.generateSlug(ctx, name)
		if err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("generating slug: %w", err))
		}
		campaign.Slug = slug
	}

	campaign.Name = name
	if desc != "" {
		campaign.Description = &desc
	} else {
		campaign.Description = nil
	}
	campaign.IsPublic = input.IsPublic

	if err := s.repo.Update(ctx, campaign); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("updating campaign: %w", err))
	}

	return campaign, nil
}

// MoveToTrash takes a campaign out of use without deleting anything: from this
// moment every read that opens it answers "not found", so members, the sync
// API and Foundry keys lose it at once, and live sockets are dropped. A site
// admin can bring it back with RestoreFromTrash until the retention is up.
// byUserID may be empty; the person's name is copied now so the Trash can still
// say who deleted it later.
func (s *campaignService) MoveToTrash(ctx context.Context, campaignID, byUserID string) error {
	var byName string
	if byUserID != "" && s.users != nil {
		if u, err := s.users.FindUserByID(ctx, byUserID); err == nil && u != nil {
			byName = u.DisplayName
		}
	}
	if err := s.repo.MoveToTrash(ctx, campaignID, byUserID, byName, time.Now().UTC()); err != nil {
		return err
	}

	// No member, grant or key can use the campaign now, so close what is open.
	if s.connRevoker != nil {
		s.connRevoker.RevokeCampaign(campaignID)
	}
	slog.Info("campaign moved to trash", slog.String("campaign_id", campaignID), slog.String("by", byUserID))
	return nil
}

// RestoreFromTrash is Undo: the campaign, its members, pages and files come
// back exactly as they were. It fails once the final delete has begun.
func (s *campaignService) RestoreFromTrash(ctx context.Context, campaignID string) error {
	if err := s.repo.RestoreFromTrash(ctx, campaignID); err != nil {
		return err
	}
	slog.Info("campaign restored from trash", slog.String("campaign_id", campaignID))
	return nil
}

// ListTrashed returns the campaigns waiting in the Trash, newest first.
func (s *campaignService) ListTrashed(ctx context.Context) ([]TrashedCampaign, error) {
	return s.repo.ListTrashed(ctx)
}

// ListPurgeDue returns the trashed campaigns that are past cutoff, plus any
// whose final delete was started and not finished. all ignores the cutoff.
func (s *campaignService) ListPurgeDue(ctx context.Context, cutoff time.Time, all bool) ([]string, error) {
	return s.repo.ListPurgeDue(ctx, cutoff, all)
}

// PurgeTrashed is the final delete of a campaign that is in the Trash. It
// first claims the campaign, which is the point of no return (Undo refuses
// from then on), then performs the multi-step cleanup:
//  1. Delete media files from disk (before SQL CASCADE nullifies campaign_id)
//  2. Dispatch campaign.deleted hook to WASM plugins for cache cleanup
//  3. SQL DELETE with FK CASCADE handles remaining database rows
//
// It reports false, having done nothing, when the campaign is not in the
// Trash: never trashed, undone, or already purged. A run that stops partway
// is picked up by the next one, since the claim stays and every step repeats
// harmlessly.
func (s *campaignService) PurgeTrashed(ctx context.Context, campaignID string, olderThan time.Time) (bool, error) {
	claimed, err := s.repo.ClaimForPurge(ctx, campaignID, time.Now().UTC(), olderThan)
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, nil
	}

	// Step 1: Clean up media files from disk before the SQL DELETE.
	// The media_files FK uses ON DELETE SET NULL, so we must delete files
	// while we still know which campaign they belong to.
	if s.mediaCleaner != nil {
		deleted, err := s.mediaCleaner.DeleteCampaignFiles(ctx, campaignID)
		if err != nil {
			// Log but don't fail — orphaned files are preferable to a stuck campaign.
			slog.Warn("media cleanup failed during campaign deletion",
				slog.String("campaign_id", campaignID),
				slog.Any("error", err),
			)
		} else if deleted > 0 {
			slog.Info("cleaned up campaign media files",
				slog.String("campaign_id", campaignID),
				slog.Int("deleted", deleted),
			)
		}
	}

	// Step 2: Notify WASM plugins so they can clean up in-memory state.
	if s.hookDispatcher != nil {
		s.hookDispatcher.DispatchCampaignDeleted(ctx, campaignID)
	}

	// Step 3: SQL DELETE, FK CASCADE handles all remaining DB rows. A second
	// purger that got here first leaves nothing to delete, which is the
	// outcome both wanted.
	if err := s.repo.PurgeTrashed(ctx, campaignID); err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
			return true, nil
		}
		return false, err
	}

	slog.Info("campaign purged", slog.String("campaign_id", campaignID))
	return true, nil
}

// CountAll returns total number of campaigns. Used for admin dashboard.
func (s *campaignService) CountAll(ctx context.Context) (int, error) {
	return s.repo.CountAll(ctx)
}

// --- Membership ---

// GetMember retrieves a user's membership in a campaign.
func (s *campaignService) GetMember(ctx context.Context, campaignID, userID string) (*CampaignMember, error) {
	return s.repo.FindMember(ctx, campaignID, userID)
}

// AddMember adds a user to a campaign by their email address.
func (s *campaignService) AddMember(ctx context.Context, campaignID, email string, role Role) error {
	if !isValidEmail(email) {
		return apperror.NewBadRequest("invalid email address")
	}
	if !role.IsValid() {
		return apperror.NewBadRequest("invalid role")
	}
	// Only Owner and admin can add members, but Owner role can't be assigned
	// through regular member addition -- only through ownership transfer.
	if role == RoleOwner {
		return apperror.NewBadRequest("cannot add a member as owner; use ownership transfer instead")
	}

	// Look up the user by email.
	user, err := s.users.FindUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return apperror.NewBadRequest("no user found with that email")
	}

	// Check if already a member.
	_, err = s.repo.FindMember(ctx, campaignID, user.ID)
	if err == nil {
		return apperror.NewConflict("user is already a member of this campaign")
	}

	member := &CampaignMember{
		CampaignID: campaignID,
		UserID:     user.ID,
		Role:       role,
		JoinedAt:   time.Now().UTC(),
	}

	if err := s.repo.AddMember(ctx, member); err != nil {
		return apperror.NewInternal(fmt.Errorf("adding member: %w", err))
	}

	slog.Info("member added to campaign",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", user.ID),
		slog.String("role", role.String()),
	)
	return nil
}

// RemoveMember removes a user from a campaign. The owner cannot be removed.
func (s *campaignService) RemoveMember(ctx context.Context, campaignID, userID string) error {
	member, err := s.repo.FindMember(ctx, campaignID, userID)
	if err != nil {
		return err
	}

	// Owners must transfer ownership before they can be removed.
	if member.Role == RoleOwner {
		return apperror.NewBadRequest("cannot remove the campaign owner; transfer ownership first")
	}

	if err := s.repo.RemoveMember(ctx, campaignID, userID); err != nil {
		return apperror.NewInternal(fmt.Errorf("removing member: %w", err))
	}

	// A removed member must not keep their co-DM grant. Left in place, the id
	// survives in settings.dm_grant_ids indefinitely, and on a PUBLIC campaign
	// that is a live leak: AllowPublicCampaignAccess admits an authenticated
	// non-member as RoleNone, the grant still resolves, and VisibilityRole()
	// hands them RoleOwner over every dm_only object.
	if err := s.revokeDmGrant(ctx, campaignID, userID); err != nil {
		return err
	}

	// Unconditional, not just for a held grant: cached Role governs the
	// hub's gates too, so even a plain member keeps receiving otherwise.
	if s.connRevoker != nil {
		s.connRevoker.RevokeUser(campaignID, userID)
	}
	for _, fn := range s.memberRemoved {
		fn(ctx, campaignID, userID)
	}

	slog.Info("member removed from campaign",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
	)
	return nil
}

// revokeDmGrant drops one user from a campaign's dm_grant_ids. It is a no-op
// when the user holds no grant, so ordinary member removal does not churn the
// settings row. Only called from RemoveMember, which drops the removed
// member's live sockets itself (unconditionally, not just for a held grant)
// once this returns — so this method owns the STORED settings cleanup only.
//
// This closes the STORED half of the leak. The resolve-time half lives in
// middleware.go, which refuses to honour a grant for a non-member — both are
// needed. This alone leaves campaigns whose data is already corrupt exposed
// until someone happens to re-save them; the middleware gate alone leaves wrong
// data behind for the next code path that reads the list directly.
func (s *campaignService) revokeDmGrant(ctx context.Context, campaignID, userID string) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign == nil {
		return apperror.NewNotFound("campaign not found")
	}

	settings := campaign.ParseSettings()
	kept := make([]string, 0, len(settings.DmGrantIDs))
	for _, id := range settings.DmGrantIDs {
		if id != userID {
			kept = append(kept, id)
		}
	}
	if len(kept) == len(settings.DmGrantIDs) {
		return nil // held no grant; nothing to write
	}
	settings.DmGrantIDs = kept

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}
	if err := s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON)); err != nil {
		return apperror.NewInternal(fmt.Errorf("revoking dm grant: %w", err))
	}

	slog.Info("dm grant revoked with membership",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
	)
	return nil
}

// UpdateMemberRole changes a member's role. The owner's role cannot be changed
// through this method -- use ownership transfer instead.
func (s *campaignService) UpdateMemberRole(ctx context.Context, campaignID, userID string, role Role) error {
	if !role.IsValid() {
		return apperror.NewBadRequest("invalid role")
	}
	if role == RoleOwner {
		return apperror.NewBadRequest("cannot promote to owner; use ownership transfer instead")
	}

	member, err := s.repo.FindMember(ctx, campaignID, userID)
	if err != nil {
		return err
	}

	// Can't change the owner's role.
	if member.Role == RoleOwner {
		return apperror.NewBadRequest("cannot change the owner's role; transfer ownership first")
	}

	if err := s.repo.UpdateMemberRole(ctx, campaignID, userID, role); err != nil {
		return apperror.NewInternal(fmt.Errorf("updating role: %w", err))
	}

	// A downgrade must drop the live socket (cached Role still governs
	// the hub's gates); an upgrade needs nothing.
	if role < member.Role && s.connRevoker != nil {
		s.connRevoker.RevokeUser(campaignID, userID)
	}

	slog.Info("member role updated",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
		slog.String("new_role", role.String()),
	)
	return nil
}

// UpdateMemberCharacter sets a member's character entity assignment.
func (s *campaignService) UpdateMemberCharacter(ctx context.Context, campaignID, userID string, characterEntityID *string) error {
	return s.repo.UpdateMemberCharacter(ctx, campaignID, userID, characterEntityID)
}

// ListMembers returns all members of a campaign.
func (s *campaignService) ListMembers(ctx context.Context, campaignID string) ([]CampaignMember, error) {
	return s.repo.ListMembers(ctx, campaignID)
}

// --- Ownership Transfer ---

// InitiateTransfer starts an ownership transfer. Generates a token and
// optionally sends an email if SMTP is configured.
func (s *campaignService) InitiateTransfer(ctx context.Context, campaignID, ownerID, targetEmail string) (*OwnershipTransfer, error) {
	email := strings.ToLower(strings.TrimSpace(targetEmail))
	if !isValidEmail(email) {
		return nil, apperror.NewBadRequest("invalid email address")
	}

	// Verify the target user exists.
	targetUser, err := s.users.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, apperror.NewBadRequest("no user found with that email")
	}

	// Can't transfer to yourself.
	if targetUser.ID == ownerID {
		return nil, apperror.NewBadRequest("cannot transfer ownership to yourself")
	}

	// Check for existing pending transfer.
	existing, err := s.repo.FindTransferByCampaign(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("checking existing transfer: %w", err))
	}
	if existing != nil {
		return nil, apperror.NewConflict("a transfer is already pending for this campaign; cancel it first")
	}

	// Generate a random token.
	token, err := generateToken()
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("generating transfer token: %w", err))
	}

	now := time.Now().UTC()
	transfer := &OwnershipTransfer{
		ID:         generateUUID(),
		CampaignID: campaignID,
		FromUserID: ownerID,
		ToUserID:   targetUser.ID,
		Token:      token,
		ExpiresAt:  now.Add(transferExpiryHours * time.Hour),
		CreatedAt:  now,
	}

	if err := s.repo.CreateTransfer(ctx, transfer); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating transfer: %w", err))
	}

	// Send email if SMTP is configured.
	if s.mail != nil && s.mail.IsConfigured(ctx) {
		campaign, _ := s.repo.FindByID(ctx, campaignID)
		campaignName := "your campaign"
		if campaign != nil {
			campaignName = campaign.Name
		}

		link := fmt.Sprintf("%s/campaigns/%s/accept-transfer?token=%s", s.baseURL, campaignID, token)
		body := fmt.Sprintf(
			"You have been offered ownership of the campaign \"%s\" on Chronicle.\n\n"+
				"Click the link below to accept (you must be logged in):\n%s\n\n"+
				"This link expires in %d hours. If you did not expect this, you can ignore it.",
			campaignName, link, transferExpiryHours,
		)

		if err := s.mail.SendMail(ctx, []string{email}, "Campaign Ownership Transfer", body); err != nil {
			// Log but don't fail -- the transfer is still created and can be
			// accepted via the campaign settings page.
			slog.Warn("failed to send transfer email",
				slog.String("campaign_id", campaignID),
				slog.String("to", email),
				slog.Any("error", err),
			)
		}
	}

	slog.Info("ownership transfer initiated",
		slog.String("campaign_id", campaignID),
		slog.String("from", ownerID),
		slog.String("to", targetUser.ID),
	)

	return transfer, nil
}

// AcceptTransfer completes a pending ownership transfer. The accepting user
// must match the transfer's to_user_id and the token must not be expired.
func (s *campaignService) AcceptTransfer(ctx context.Context, token string, acceptingUserID string) error {
	transfer, err := s.repo.FindTransferByToken(ctx, token)
	if err != nil {
		return apperror.NewBadRequest("invalid or expired transfer link")
	}

	// Verify the token hasn't expired.
	if time.Now().UTC().After(transfer.ExpiresAt) {
		// Clean up the expired transfer.
		_ = s.repo.DeleteTransfer(ctx, transfer.ID)
		return apperror.NewBadRequest("this transfer link has expired")
	}

	// Verify the accepting user is the intended recipient.
	if transfer.ToUserID != acceptingUserID {
		return apperror.NewForbidden("this transfer is not for your account")
	}

	// A campaign in the site Trash is not found: its ownership cannot change.
	if _, err := s.repo.FindByID(ctx, transfer.CampaignID); err != nil {
		return err
	}

	// Perform the atomic transfer.
	if err := s.repo.TransferOwnership(ctx, transfer.CampaignID, transfer.FromUserID, transfer.ToUserID); err != nil {
		return apperror.NewInternal(fmt.Errorf("transferring ownership: %w", err))
	}

	// Drop the old owner's live socket: cached Role=Owner isn't touched
	// by the demotion above, so it would keep owner-level visibility.
	if s.connRevoker != nil {
		s.connRevoker.RevokeUser(transfer.CampaignID, transfer.FromUserID)
	}

	slog.Info("ownership transfer completed",
		slog.String("campaign_id", transfer.CampaignID),
		slog.String("from", transfer.FromUserID),
		slog.String("to", transfer.ToUserID),
	)

	return nil
}

// CancelTransfer removes a pending ownership transfer.
func (s *campaignService) CancelTransfer(ctx context.Context, campaignID string) error {
	transfer, err := s.repo.FindTransferByCampaign(ctx, campaignID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("finding transfer: %w", err))
	}
	if transfer == nil {
		return apperror.NewNotFound("no pending transfer for this campaign")
	}

	if err := s.repo.DeleteTransfer(ctx, transfer.ID); err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting transfer: %w", err))
	}

	slog.Info("ownership transfer cancelled", slog.String("campaign_id", campaignID))
	return nil
}

// GetPendingTransfer returns the pending transfer for a campaign, or nil.
func (s *campaignService) GetPendingTransfer(ctx context.Context, campaignID string) (*OwnershipTransfer, error) {
	return s.repo.FindTransferByCampaign(ctx, campaignID)
}

// --- Sidebar Configuration ---

// maxSidebarConfigEntries caps the number of entries in sidebar config arrays
// to prevent abuse via oversized JSON payloads.
const maxSidebarConfigEntries = 100

// UpdateBranding sets the campaign's custom brand name and logo path.
// Brand name max 40 chars. Empty strings clear the respective fields.
func (s *campaignService) UpdateBranding(ctx context.Context, campaignID, brandName, brandLogo string) error {
	if len(brandName) > 40 {
		return apperror.NewBadRequest("brand name must be 40 characters or fewer")
	}
	if len(brandLogo) > 255 {
		return apperror.NewBadRequest("brand logo path too long")
	}

	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.BrandName = brandName
	settings.BrandLogo = brandLogo

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// normalizeNavIcons clears any navigation icon that fails the shared icon
// check, so it renders with the default instead, and logs what it dropped.
func normalizeNavIcons[T any](items []T, icon func(*T) *string) {
	for i := range items {
		p := icon(&items[i])
		cleaned, replaced := sanitize.IconOrDefault(*p, "")
		if replaced {
			slog.Warn("campaign navigation: dropped invalid icon", slog.String("icon", *p))
		}
		*p = cleaned
	}
}

// validateNavLinkURL rejects an owner-supplied navigation link URL that isn't an
// http(s) absolute or a same-origin relative path — the stored-XSS /
// open-redirect ingress guard for sidebar/topbar links (audit-R2 Finding 1).
// The allowlist lives in sanitize.SafeLinkURL so ingress + egress agree.
func validateNavLinkURL(label, url string) error {
	if _, ok := sanitize.SafeLinkURL(url); !ok {
		name := strings.TrimSpace(label)
		if name == "" {
			name = "(unnamed)"
		}
		return apperror.NewBadRequest(fmt.Sprintf(
			"link %q has an invalid URL — use a full http(s):// address or a path starting with /", name))
	}
	return nil
}

// isValidHexColor checks that s is a 7-character hex color (#RRGGBB).
func isValidHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// IsUserDmGranted reports whether the campaign Owner has granted the
// given user dm_only visibility via CampaignSettings.DmGrantIDs.
//
// Mirrors the package-private hasDmGrant helper used by middleware so
// the websocket hub can resolve the same flag without importing the
// campaigns package directly. The campaign lookup is the same call the
// HTTP middleware makes on every request, so a 1:1 cache hit on the
// repo's campaign cache (if any) is expected.
func (s *campaignService) IsUserDmGranted(ctx context.Context, campaignID, userID string) (bool, error) {
	if campaignID == "" || userID == "" {
		return false, nil
	}
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return false, err
	}
	if campaign == nil {
		return false, nil
	}
	return hasDmGrant(campaign, userID), nil
}

// SetFoundryModulePin updates CampaignSettings.FoundryModulePin and
// re-persists the settings JSON. An empty version clears the pin
// (campaign auto-tracks the installed version). The foundry_vtt
// service owns version validation (exists on disk) and calls this
// only after that check passes.
func (s *campaignService) SetFoundryModulePin(ctx context.Context, campaignID, version string) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign == nil {
		return apperror.NewNotFound("campaign not found")
	}
	settings := campaign.ParseSettings()
	settings.FoundryModulePin = version
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}
	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// GetFoundryModulePin returns the campaign's current pin string, or
// "" if unpinned. Used by foundry_vtt's manifest resolver
// (resolveCampaignManifest) to pick which on-disk version's
// module.json to serve for a given campaign.
func (s *campaignService) GetFoundryModulePin(ctx context.Context, campaignID string) (string, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return "", err
	}
	if campaign == nil {
		return "", apperror.NewNotFound("campaign not found")
	}
	return campaign.ParseSettings().FoundryModulePin, nil
}

// SetFoundryModulePinMode updates CampaignSettings.FoundryModulePinMode
// and re-persists the settings JSON. Companion to SetFoundryModulePin.
// Validation lives at the foundry_vtt service layer (see
// foundry_vtt.IsValidPinMode); this method writes whatever the caller
// passes through.
func (s *campaignService) SetFoundryModulePinMode(ctx context.Context, campaignID, mode string) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign == nil {
		return apperror.NewNotFound("campaign not found")
	}
	settings := campaign.ParseSettings()
	settings.FoundryModulePinMode = mode
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}
	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// GetFoundryModulePinMode returns the campaign's pin_mode setting, or
// empty string if not yet set. Companion to GetFoundryModulePin.
func (s *campaignService) GetFoundryModulePinMode(ctx context.Context, campaignID string) (string, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return "", err
	}
	if campaign == nil {
		return "", apperror.NewNotFound("campaign not found")
	}
	return campaign.ParseSettings().FoundryModulePinMode, nil
}

// CampaignExistsByID returns true iff the campaign exists. Used by
// the foundry_vtt service to reject install-URL minting + token
// rotation requests for unknown campaigns before any DB writes
// happen.
func (s *campaignService) CampaignExistsByID(ctx context.Context, campaignID string) (bool, error) {
	c, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return false, err
	}
	return c != nil, nil
}

// UpdateDmGrants sets which users are granted dm_only content visibility.
// Only campaign Owners may call this. The granted users can see dm_only
// content but cannot create or toggle dm_only flags.
func (s *campaignService) UpdateDmGrants(ctx context.Context, campaignID string, userIDs []string) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	// Sibling settings mutators (SetFoundryModulePin) all carry this guard; its
	// absence here made a missing campaign a nil dereference instead of a 404.
	if campaign == nil {
		return apperror.NewNotFound("campaign not found")
	}

	settings := campaign.ParseSettings()
	alreadyGranted := make(map[string]bool, len(settings.DmGrantIDs))
	for _, id := range settings.DmGrantIDs {
		alreadyGranted[id] = true
	}

	// Validate and dedup before storing. Membership gates whether a grant is
	// HONOURED (middleware.go), but an unvalidated write is still the way to
	// get an id into the list that should never have been there — a grant
	// held by a non-member. An id already on the list that is no longer a
	// member (a deleted account) is dropped rather than refused, or one stale
	// id would block every later change, removals included.
	seen := make(map[string]bool, len(userIDs))
	clean := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := s.repo.FindMember(ctx, campaignID, id); err != nil {
			if alreadyGranted[id] {
				continue
			}
			return apperror.NewBadRequest("cannot grant dm_only visibility to a non-member")
		}
		clean = append(clean, id)
	}

	// Diffed against the old list before it's overwritten, so anyone
	// losing the grant can have their live socket dropped below.
	stillGranted := make(map[string]bool, len(clean))
	for _, id := range clean {
		stillGranted[id] = true
	}
	var removed []string
	for _, id := range settings.DmGrantIDs {
		if !stillGranted[id] {
			removed = append(removed, id)
		}
	}

	settings.DmGrantIDs = clean

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	if err := s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON)); err != nil {
		return err
	}

	if s.connRevoker != nil {
		for _, userID := range removed {
			s.connRevoker.RevokeUser(campaignID, userID)
		}
	}
	return nil
}

// platformDefaultTiers is the platform-wide event tier vocabulary
// returned by GetEventTierDefinitions when a campaign has no override.
// Slugs are stable for upgrades; renaming or removing requires a
// migration of every campaign storing those slugs on the
// calendar_events.tier column.
var platformDefaultTiers = []TierDefinition{
	{Slug: "major", Name: "Major", Color: "#ef4444", Prominence: 100, IsDefault: false},
	{Slug: "standard", Name: "Standard", Color: "#6366f1", Prominence: 50, IsDefault: true},
	{Slug: "detail", Name: "Detail", Color: "#94a3b8", Prominence: 10, IsDefault: false},
}

// hexColorRE matches #RRGGBB color codes (uppercase OR lowercase hex).
var hexColorRE = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// tierSlugRE matches valid tier slugs (lowercase ASCII letters, digits,
// dash, underscore). Same shape as Chronicle's general slug pattern.
var tierSlugRE = regexp.MustCompile(`^[a-z0-9_-]+$`)

// validateTierDefinitions runs the per-set + per-entry validation rules.
// Caller passes the proposed full set; returns nil if valid, an
// apperror.AppError describing the first violation otherwise.
func validateTierDefinitions(defs []TierDefinition) error {
	if len(defs) == 0 {
		return apperror.NewBadRequest("at least one tier definition is required")
	}
	defaults := 0
	slugSeen := make(map[string]bool, len(defs))
	for i, d := range defs {
		if !tierSlugRE.MatchString(d.Slug) {
			return apperror.NewBadRequest(fmt.Sprintf(
				"tier %d: slug %q must be lowercase alphanumeric with dashes or underscores",
				i+1, d.Slug))
		}
		if slugSeen[d.Slug] {
			return apperror.NewBadRequest(fmt.Sprintf("tier slug %q appears more than once", d.Slug))
		}
		slugSeen[d.Slug] = true
		if d.Name == "" {
			return apperror.NewBadRequest(fmt.Sprintf("tier %q: name is required", d.Slug))
		}
		if !hexColorRE.MatchString(d.Color) {
			return apperror.NewBadRequest(fmt.Sprintf(
				"tier %q: color %q must be #RRGGBB hex format", d.Slug, d.Color))
		}
		if d.Prominence < 0 || d.Prominence > 100 {
			return apperror.NewBadRequest(fmt.Sprintf(
				"tier %q: prominence %d must be 0-100", d.Slug, d.Prominence))
		}
		if d.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		return apperror.NewBadRequest(fmt.Sprintf(
			"exactly one tier must be marked is_default; got %d", defaults))
	}
	return nil
}

// GetEventTierDefinitions returns the campaign's tier vocabulary, falling
// back to the platform default trio when the campaign has none set.
// Mirrors the empty-means-default pattern used for AccentColor +
// FontFamily + etc. — no migration backfill needed for existing
// campaigns. Returns a defensive copy of the default slice so the caller
// can't mutate platformDefaultTiers via the returned reference.
func (s *campaignService) GetEventTierDefinitions(ctx context.Context, campaignID string) ([]TierDefinition, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if campaign == nil {
		return nil, apperror.NewNotFound("campaign not found")
	}
	settings := campaign.ParseSettings()
	if len(settings.EventTierDefinitions) == 0 {
		out := make([]TierDefinition, len(platformDefaultTiers))
		copy(out, platformDefaultTiers)
		return out, nil
	}
	return settings.EventTierDefinitions, nil
}

// SetEventTierDefinitions replaces the campaign's tier vocabulary with
// the provided set. Validates exactly-one-default + non-empty array +
// unique slugs + hex color + prominence range. To reset to the platform
// default trio, the caller can set an empty array via a separate "reset"
// affordance OR pass the platform default values explicitly — the
// service treats the stored array as the source of truth when non-empty.
func (s *campaignService) SetEventTierDefinitions(ctx context.Context, campaignID string, defs []TierDefinition) error {
	if err := validateTierDefinitions(defs); err != nil {
		return err
	}

	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign == nil {
		return apperror.NewNotFound("campaign not found")
	}

	settings := campaign.ParseSettings()
	settings.EventTierDefinitions = defs

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}
	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// UpdateWelcomeMessage sets the campaign's MOTD banner message.
// Empty string clears the message.
func (s *campaignService) UpdateWelcomeMessage(ctx context.Context, campaignID, message string) error {
	if len(message) > 500 {
		return apperror.NewBadRequest("welcome message must be 500 characters or fewer")
	}

	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.WelcomeMessage = message

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// validDefaultVisibilities defines the allowed default visibility values.
var validDefaultVisibilities = map[string]bool{
	"":        true, // everyone (default)
	"dm_only": true,
	"private": true,
}

// UpdateDefaultVisibility sets the campaign's default visibility for new entities.
// Empty string means "everyone" (the platform default).
func (s *campaignService) UpdateDefaultVisibility(ctx context.Context, campaignID, visibility string) error {
	if !validDefaultVisibilities[visibility] {
		return apperror.NewBadRequest("invalid visibility, expected empty, dm_only, or private")
	}

	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.DefaultVisibility = visibility

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// UpdateSidebarConfig applies a partial update to the stored sidebar config via
// a load-merge-write pattern. Nil pointer fields in req are absent from the
// JSON body and are left unchanged; non-nil fields (including explicit empty
// slices) replace the stored value. This prevents Writers A and B from
// clobbering each other's model when they only touch a subset of fields.
func (s *campaignService) UpdateSidebarConfig(ctx context.Context, campaignID string, req UpdateSidebarConfigRequest) error {
	if req.Items != nil && len(*req.Items) > maxSidebarConfigEntries {
		return apperror.NewBadRequest("sidebar items list is too long")
	}
	// Validates only the REQUEST's items (merge semantics: nil = field absent,
	// nothing new to check). Link URLs are rendered to every visitor, so an
	// unsafe one is refused here and dropped again at render (ViewNav).
	if req.Items != nil {
		// The editor re-sends every stored item on each save, so a bad icon
		// is dropped rather than refused: an old value must not block the
		// owner from ever saving the sidebar again.
		normalizeNavIcons(*req.Items, func(it *SidebarItem) *string { return &it.Icon })
		cleaned, err := validateSidebarItems(*req.Items)
		if err != nil {
			return err
		}
		req.Items = &cleaned
	}

	// Read current stored config so absent fields are preserved.
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	config := campaign.ParseSidebarConfig()

	// If this campaign is still on the legacy model — the boot reconciler hasn't
	// converged it yet, or EnsureSidebarItems failed for this row — converting
	// here first closes a data-loss window: the canonical SidebarConfig struct no
	// longer carries the legacy fields, so the Marshal below would drop them and
	// permanently destroy the operator's saved customization on the very first
	// write. Converting lazily on write self-heals the straggler onto items,
	// exactly as the reconciler would, before the merge applies.
	if len(config.Items) == 0 {
		if converted, ok := convertLegacySidebarConfig(campaign.SidebarConfig); ok {
			config = converted
		}
	}

	// Merge only the fields present in the request.
	if req.Items != nil {
		config.Items = *req.Items
	}
	if req.HiddenEntityIDs != nil {
		config.HiddenEntityIDs = *req.HiddenEntityIDs
	}
	if req.HiddenNodeIDs != nil {
		config.HiddenNodeIDs = *req.HiddenNodeIDs
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling sidebar config: %w", err))
	}

	if err := s.repo.UpdateSidebarConfig(ctx, campaignID, string(configJSON)); err != nil {
		return err
	}

	slog.Info("sidebar config updated", slog.String("campaign_id", campaignID))
	return nil
}

// GetSidebarConfig returns the parsed sidebar configuration for a campaign.
func (s *campaignService) GetSidebarConfig(ctx context.Context, campaignID string) (*SidebarConfig, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	cfg := campaign.ParseSidebarConfig()
	return &cfg, nil
}

// --- Dashboard Layout ---

// maxDashboardRows caps the number of rows in a dashboard layout to prevent
// abuse via oversized JSON payloads.
const maxDashboardRows = 50

// maxDashboardBlocksPerRow caps the total number of blocks per row.
const maxDashboardBlocksPerRow = 20

// validateDashboardLayout validates block types, column widths, and sanitizes
// text_block content. Shared by both campaign and owner dashboard layouts.
func validateDashboardLayout(layout *DashboardLayout) error {
	if len(layout.Rows) > maxDashboardRows {
		return apperror.NewBadRequest("dashboard layout has too many rows")
	}

	for _, row := range layout.Rows {
		totalWidth := 0
		blockCount := 0
		for _, col := range row.Columns {
			if col.Width < 1 || col.Width > 12 {
				return apperror.NewBadRequest("column width must be between 1 and 12")
			}
			totalWidth += col.Width
			blockCount += len(col.Blocks)
			for i, block := range col.Blocks {
				if !ValidBlockTypes[block.Type] {
					return apperror.NewBadRequest(fmt.Sprintf("unsupported block type: %s", block.Type))
				}
				if block.Type == "text_block" {
					if content, ok := block.Config["content"].(string); ok {
						col.Blocks[i].Config["content"] = sanitize.HTML(content)
					}
				}
			}
		}
		if totalWidth > 12 {
			return apperror.NewBadRequest("row column widths exceed 12")
		}
		if blockCount > maxDashboardBlocksPerRow {
			return apperror.NewBadRequest("too many blocks in a single row")
		}
	}
	return nil
}

// marshalLayout marshals a DashboardLayout to a JSON string pointer.
func marshalLayout(layout *DashboardLayout) (*string, error) {
	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("marshaling dashboard layout: %w", err))
	}
	s := string(layoutJSON)
	return &s, nil
}

// UpdateDashboardLayout validates and saves a dashboard layout for a campaign.
func (s *campaignService) UpdateDashboardLayout(ctx context.Context, campaignID string, layout *DashboardLayout) error {
	if layout == nil {
		return s.repo.UpdateDashboardLayout(ctx, campaignID, nil)
	}
	if err := validateDashboardLayout(layout); err != nil {
		return err
	}
	layoutStr, err := marshalLayout(layout)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateDashboardLayout(ctx, campaignID, layoutStr); err != nil {
		return err
	}
	slog.Info("dashboard layout updated", slog.String("campaign_id", campaignID))
	return nil
}

// UpdateDashboardLayoutRaw saves a pre-marshaled dashboard layout JSON string.
// Used by the handler when merging role-specific layouts.
func (s *campaignService) UpdateDashboardLayoutRaw(ctx context.Context, campaignID string, layoutJSON *string) error {
	if err := s.repo.UpdateDashboardLayout(ctx, campaignID, layoutJSON); err != nil {
		return err
	}
	slog.Info("dashboard layout updated (raw)", slog.String("campaign_id", campaignID))
	return nil
}

// GetDashboardLayout returns the parsed dashboard layout for a campaign.
// Returns nil if no custom layout is set (use default).
func (s *campaignService) GetDashboardLayout(ctx context.Context, campaignID string) (*DashboardLayout, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return campaign.ParseDashboardLayout(), nil
}

// ResetDashboardLayout removes the custom dashboard layout, reverting to default.
func (s *campaignService) ResetDashboardLayout(ctx context.Context, campaignID string) error {
	if err := s.repo.UpdateDashboardLayout(ctx, campaignID, nil); err != nil {
		return err
	}
	slog.Info("dashboard layout reset to default", slog.String("campaign_id", campaignID))
	return nil
}

// UpdateOwnerDashboardLayout validates and saves the owner dashboard layout.
func (s *campaignService) UpdateOwnerDashboardLayout(ctx context.Context, campaignID string, layout *DashboardLayout) error {
	if layout == nil {
		return s.repo.UpdateOwnerDashboardLayout(ctx, campaignID, nil)
	}
	if err := validateDashboardLayout(layout); err != nil {
		return err
	}
	layoutStr, err := marshalLayout(layout)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateOwnerDashboardLayout(ctx, campaignID, layoutStr); err != nil {
		return err
	}
	slog.Info("owner dashboard layout updated", slog.String("campaign_id", campaignID))
	return nil
}

// GetOwnerDashboardLayout returns the parsed owner dashboard layout.
// Returns nil if no custom layout is set (use default).
func (s *campaignService) GetOwnerDashboardLayout(ctx context.Context, campaignID string) (*DashboardLayout, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return campaign.ParseOwnerDashboardLayout(), nil
}

// ResetOwnerDashboardLayout removes the custom owner dashboard layout.
func (s *campaignService) ResetOwnerDashboardLayout(ctx context.Context, campaignID string) error {
	if err := s.repo.UpdateOwnerDashboardLayout(ctx, campaignID, nil); err != nil {
		return err
	}
	slog.Info("owner dashboard layout reset to default", slog.String("campaign_id", campaignID))
	return nil
}

// --- Admin Operations ---

// ForceTransferOwnership is used by admins to take ownership of a campaign.
// No email confirmation needed — this is an administrative action.
func (s *campaignService) ForceTransferOwnership(ctx context.Context, campaignID, newOwnerID string) error {
	// Looked up before the demotion below, which targets by role, not id —
	// this is the only place the previous owner's id is available.
	previousOwner, ownerErr := s.repo.FindOwnerMember(ctx, campaignID)

	if err := s.repo.ForceTransferOwnership(ctx, campaignID, newOwnerID); err != nil {
		return apperror.NewInternal(fmt.Errorf("force transferring ownership: %w", err))
	}

	// Drop the old owner's live sockets — same reasoning as AcceptTransfer.
	// Skipped if newOwnerID was already the owner: nobody's access changed.
	if s.connRevoker != nil && ownerErr == nil && previousOwner != nil && previousOwner.UserID != newOwnerID {
		s.connRevoker.RevokeUser(campaignID, previousOwner.UserID)
	}

	slog.Info("admin force-transferred campaign ownership",
		slog.String("campaign_id", campaignID),
		slog.String("new_owner", newOwnerID),
	)
	return nil
}

// AdminAddMember adds a user to a campaign by their user ID. Used by admins
// to add themselves. When adding as Owner, triggers a force transfer.
func (s *campaignService) AdminAddMember(ctx context.Context, campaignID, userID string, role Role) error {
	if !role.IsValid() {
		return apperror.NewBadRequest("invalid role")
	}

	// Check if already a member.
	existing, err := s.repo.FindMember(ctx, campaignID, userID)
	if err == nil {
		// Already a member -- update their role if different.
		if existing.Role == role {
			return nil // No change needed.
		}

		// If promoting to Owner, use force transfer.
		if role == RoleOwner {
			return s.ForceTransferOwnership(ctx, campaignID, userID)
		}

		// Otherwise just update the role.
		if err := s.repo.UpdateMemberRole(ctx, campaignID, userID, role); err != nil {
			return err
		}
		// Same downgrade check as UpdateMemberRole, which this admin
		// path bypasses (it can even demote an admin's own Owner row).
		if role < existing.Role && s.connRevoker != nil {
			s.connRevoker.RevokeUser(campaignID, userID)
		}
		return nil
	}

	// Not a member -- add them. If joining as Owner, force-transfer.
	if role == RoleOwner {
		return s.ForceTransferOwnership(ctx, campaignID, userID)
	}

	member := &CampaignMember{
		CampaignID: campaignID,
		UserID:     userID,
		Role:       role,
		JoinedAt:   time.Now().UTC(),
	}

	if err := s.repo.AddMember(ctx, member); err != nil {
		return apperror.NewInternal(fmt.Errorf("admin adding member: %w", err))
	}

	slog.Info("admin added member to campaign",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
		slog.String("role", role.String()),
	)
	return nil
}

// --- Helpers ---

// maxSlugAttempts caps slug deduplication iterations to prevent DoS from
// adversarial name collisions (e.g., creating "test", "test-2" ... "test-N").
const maxSlugAttempts = 100

// generateSlug creates a unique slug for a campaign. If the base slug is
// taken, appends -2, -3, etc. After maxSlugAttempts, falls back to a random suffix.
func (s *campaignService) generateSlug(ctx context.Context, name string) (string, error) {
	base := Slugify(name)
	slug := base

	for i := 2; i < maxSlugAttempts+2; i++ {
		exists, err := s.repo.SlugExists(ctx, slug)
		if err != nil {
			return "", fmt.Errorf("checking slug: %w", err)
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}

	// Fallback: append random suffix to guarantee uniqueness.
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random slug suffix: %w", err)
	}
	return fmt.Sprintf("%s-%s", base, hex.EncodeToString(b)), nil
}

// generateUUID creates a new v4 UUID string using crypto/rand.
// Panics if the system entropy source fails, as this indicates a
// catastrophic system problem that would compromise all security.
func generateUUID() string {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// generateToken creates a cryptographically random hex-encoded token.
func generateToken() (string, error) {
	b := make([]byte, transferTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- Archive & Join Code ---

// ArchiveCampaign soft-archives a campaign, making it read-only.
func (s *campaignService) ArchiveCampaign(ctx context.Context, campaignID string) error {
	return s.repo.ArchiveCampaign(ctx, campaignID)
}

// UnarchiveCampaign restores write access to a soft-archived campaign.
func (s *campaignService) UnarchiveCampaign(ctx context.Context, campaignID string) error {
	return s.repo.UnarchiveCampaign(ctx, campaignID)
}

// GenerateJoinCode creates a shareable invite code for the campaign.
// Returns the generated code.
func (s *campaignService) GenerateJoinCode(ctx context.Context, campaignID string) (string, error) {
	b := make([]byte, 6) // 12 hex chars
	if _, err := rand.Read(b); err != nil {
		return "", apperror.NewInternal(fmt.Errorf("generating join code: %w", err))
	}
	code := hex.EncodeToString(b)

	if err := s.repo.SetJoinCode(ctx, campaignID, code); err != nil {
		return "", err
	}

	slog.Info("join code generated",
		slog.String("campaign_id", campaignID),
	)
	return code, nil
}

// RevokeJoinCode removes the shareable invite code for the campaign.
func (s *campaignService) RevokeJoinCode(ctx context.Context, campaignID string) error {
	return s.repo.ClearJoinCode(ctx, campaignID)
}

// JoinByCode joins a user to a campaign via its shareable invite code.
func (s *campaignService) JoinByCode(ctx context.Context, code, userID string) error {
	campaign, err := s.repo.FindByJoinCode(ctx, code)
	if err != nil {
		return err
	}

	if campaign.IsArchived() {
		return apperror.NewBadRequest("campaign is archived and not accepting new members")
	}

	// Check if already a member.
	_, err = s.repo.FindMember(ctx, campaign.ID, userID)
	if err == nil {
		return apperror.NewConflict("you are already a member of this campaign")
	}

	member := &CampaignMember{
		CampaignID: campaign.ID,
		UserID:     userID,
		Role:       RolePlayer,
		JoinedAt:   time.Now().UTC(),
	}

	if err := s.repo.AddMember(ctx, member); err != nil {
		return apperror.NewInternal(fmt.Errorf("joining campaign: %w", err))
	}

	slog.Info("user joined campaign via invite code",
		slog.String("campaign_id", campaign.ID),
		slog.String("user_id", userID),
	)
	return nil
}

// UpdateSystemID sets the game system for a campaign. Stores in the
// settings JSON column. Accepts a system registry ID or "custom:<url>".
func (s *campaignService) UpdateSystemID(ctx context.Context, campaignID, systemID string) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}

	settings := campaign.ParseSettings()
	settings.SystemID = systemID

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}

	return s.repo.UpdateSettings(ctx, campaignID, string(settingsJSON))
}

// isValidEmail performs a basic format check on an email address.
func isValidEmail(email string) bool {
	// Simple regex: something@something.something
	return len(email) <= 254 && len(email) >= 5 && strings.Contains(email, "@") && strings.Contains(email, ".")
}

// --- Campaign Group Service ---

// groupService implements GroupService.
type groupService struct {
	repo GroupRepository
}

// NewGroupService creates a new group service.
func NewGroupService(repo GroupRepository) GroupService {
	return &groupService{repo: repo}
}

// CreateGroup creates a new campaign group with validation.
func (s *groupService) CreateGroup(ctx context.Context, campaignID, name string, description *string) (*CampaignGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperror.NewBadRequest("group name is required")
	}
	if len(name) > 100 {
		return nil, apperror.NewBadRequest("group name must be 100 characters or less")
	}

	group, err := s.repo.CreateGroup(ctx, campaignID, name, description)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, apperror.NewBadRequest("a group with this name already exists")
		}
		return nil, apperror.NewInternal(err)
	}
	return group, nil
}

// ListGroups returns all groups for a campaign with their members.
func (s *groupService) ListGroups(ctx context.Context, campaignID string) ([]CampaignGroup, error) {
	groups, err := s.repo.ListGroups(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	// Populate members for each group.
	for i := range groups {
		members, err := s.repo.ListGroupMembers(ctx, groups[i].ID)
		if err != nil {
			slog.Error("failed to list group members", slog.Int("group_id", groups[i].ID), slog.Any("error", err))
			continue
		}
		groups[i].Members = members
	}
	return groups, nil
}

// GetGroup returns a single group by ID with its members.
func (s *groupService) GetGroup(ctx context.Context, groupID int) (*CampaignGroup, error) {
	group, err := s.repo.GetGroup(ctx, groupID)
	if err != nil {
		return nil, apperror.NewNotFound("group not found")
	}
	members, _ := s.repo.ListGroupMembers(ctx, groupID)
	group.Members = members
	return group, nil
}

// UpdateGroup updates a group's name and description.
func (s *groupService) UpdateGroup(ctx context.Context, groupID int, name string, description *string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return apperror.NewBadRequest("group name is required")
	}
	if err := s.repo.UpdateGroup(ctx, groupID, name, description); err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return apperror.NewBadRequest("a group with this name already exists")
		}
		return apperror.NewInternal(err)
	}
	return nil
}

// DeleteGroup deletes a group.
func (s *groupService) DeleteGroup(ctx context.Context, groupID int) error {
	if err := s.repo.DeleteGroup(ctx, groupID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// AddGroupMember adds a user to a group.
func (s *groupService) AddGroupMember(ctx context.Context, groupID int, userID string) error {
	if err := s.repo.AddGroupMember(ctx, groupID, userID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// RemoveGroupMember removes a user from a group.
func (s *groupService) RemoveGroupMember(ctx context.Context, groupID int, userID string) error {
	if err := s.repo.RemoveGroupMember(ctx, groupID, userID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// ListGroupMembers returns all members of a group.
func (s *groupService) ListGroupMembers(ctx context.Context, groupID int) ([]GroupMemberInfo, error) {
	members, err := s.repo.ListGroupMembers(ctx, groupID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return members, nil
}
