// Package campaigns manages campaigns (worldbuilding containers) and their
// role-based membership system. A campaign is the top-level organizational
// unit that holds all entities, maps, timelines, etc.
//
// This is a CORE plugin -- always enabled, cannot be disabled.
package campaigns

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

// --- Role System ---

// Role represents a user's permission level within a campaign.
// Higher numeric values indicate more permissions. Use >= comparisons:
//
//	if role >= RoleScribe { /* allow content creation */ }
type Role int

const (
	// RoleNone indicates the user has no membership in the campaign.
	// Used when a site admin accesses a campaign they haven't joined.
	RoleNone Role = 0

	// RolePlayer grants read access to permitted content. Default role on join.
	RolePlayer Role = 1

	// RoleScribe grants create/edit access to notes and entities.
	// The TTRPG note-taker / co-author.
	RoleScribe Role = 2

	// RoleOwner grants full control over the campaign. One per campaign.
	// Can transfer ownership, manage members, and change settings.
	RoleOwner Role = 3
)

// RoleFromString converts a database role string to a Role constant.
func RoleFromString(s string) Role {
	switch s {
	case "owner":
		return RoleOwner
	case "scribe":
		return RoleScribe
	case "player":
		return RolePlayer
	default:
		return RoleNone
	}
}

// String returns the database-safe string representation of a Role.
func (r Role) String() string {
	switch r {
	case RoleOwner:
		return "owner"
	case RoleScribe:
		return "scribe"
	case RolePlayer:
		return "player"
	default:
		return ""
	}
}

// DisplayName returns a human-readable label for the role.
func (r Role) DisplayName() string {
	switch r {
	case RoleOwner:
		return "Owner"
	case RoleScribe:
		return "Scribe"
	case RolePlayer:
		return "Player"
	default:
		return "None"
	}
}

// IsValid returns true if this is a valid campaign membership role.
func (r Role) IsValid() bool {
	return r >= RolePlayer && r <= RoleOwner
}

// --- Domain Models ---

// Validation constants for campaign fields.
const (
	MaxCampaignNameLen    = 200
	MaxDescriptionLen     = 5000
	MaxWelcomeMessageLen  = 500
)

// Campaign represents a top-level worldbuilding container.
type Campaign struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	Description     *string   `json:"description,omitempty"`
	IsPublic        bool      `json:"is_public"`
	Settings        string    `json:"settings"`
	BackdropPath    *string   `json:"backdrop_path,omitempty"`
	SidebarConfig   string    `json:"sidebar_config"`
	DashboardLayout      *string   `json:"dashboard_layout,omitempty"`       // JSON layout; nil = use hardcoded default.
	OwnerDashboardLayout *string   `json:"owner_dashboard_layout,omitempty"` // Owner-only dashboard layout; nil = use default.
	CreatedBy            string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`               // Soft-archive timestamp; nil = active.
	JoinCode        *string    `json:"join_code,omitempty"`                  // Shareable invite code; nil = no active link.
}

// IsArchived returns true if the campaign has been soft-archived.
func (c *Campaign) IsArchived() bool {
	return c.ArchivedAt != nil
}

// SidebarConfig holds campaign-level sidebar customization settings.
// Stored as JSON in campaigns.sidebar_config. Controls the ordered list of
// sidebar items plus the sets of individually-hidden entities and folder nodes.
//
// This is the single, unified sidebar model. The pre-2026-07 legacy model
// (entity_type_order / hidden_type_ids / custom_sections / custom_links) was
// removed by C-NAV-V3; the EnsureSidebarItems boot reconciler converts any
// straggler campaign onto Items (see sidebar_reconcile.go). An empty Items
// array is valid — it renders the default sidebar, synthesized by the render
// injector.
type SidebarConfig struct {
	// Items is the unified, ordered list of all sidebar items. Each item
	// has a type (dashboard, addon, category, section, link, all_pages)
	// and is rendered in array order. Owners can reorder, show/hide any item.
	Items []SidebarItem `json:"items,omitempty"`

	// HiddenEntityIDs is a set of individual entity IDs that should be
	// hidden from the sidebar for non-owner roles.
	HiddenEntityIDs []string `json:"hidden_entity_ids,omitempty"`

	// HiddenNodeIDs is a set of sidebar folder node IDs that should be
	// hidden from the sidebar for non-owner roles.
	HiddenNodeIDs []string `json:"hidden_node_ids,omitempty"`
}

// SidebarItem represents a single item in the unified sidebar navigation.
// All sidebar content (dashboard, addons, categories, sections, links) is
// modeled as items so owners can freely reorder everything.
//
// Nesting is intentionally NOT a field here. Whether a category renders
// nested under a parent is derived from entity_types.parent_type_id (the
// structural source of truth). A persisted "nested" flag would be a second
// source of truth that can drift; old persisted values for the removed
// "nested" JSON key are silently ignored on unmarshal.
type SidebarItem struct {
	Type    string `json:"type"`              // "dashboard", "addon", "category", "section", "link", "all_pages"
	Visible bool   `json:"visible"`           // Whether to show this item.
	Slug    string `json:"slug,omitempty"`    // Addon slug (for type=addon).
	TypeID  int    `json:"type_id,omitempty"` // Entity type ID (for type=category).
	ID      string `json:"id,omitempty"`      // Unique ID (for sections/links).
	Label   string `json:"label,omitempty"`   // Display label (for sections/links).
	URL     string `json:"url,omitempty"`     // Link URL (for type=link).
	Icon    string `json:"icon,omitempty"`    // FontAwesome icon (for type=link).
}

// ParseSidebarConfig parses the campaign's sidebar_config JSON into a
// SidebarConfig struct. Returns an empty config on parse failure.
func (c *Campaign) ParseSidebarConfig() SidebarConfig {
	var cfg SidebarConfig
	if c.SidebarConfig != "" {
		if err := json.Unmarshal([]byte(c.SidebarConfig), &cfg); err != nil {
			slog.Warn("failed to parse sidebar config, using defaults",
				slog.String("campaign_id", c.ID),
				slog.String("error", err.Error()),
			)
		}
	}
	return cfg
}

// MemberLister provides campaign membership data. Defined here so plugins
// (timeline, sessions, etc.) can depend on campaigns.MemberLister instead
// of duplicating the interface.
type MemberLister interface {
	ListMembers(ctx context.Context, campaignID string) ([]CampaignMember, error)
}

// CampaignMember represents a user's membership in a campaign.
type CampaignMember struct {
	CampaignID        string    `json:"campaign_id"`
	UserID            string    `json:"user_id"`
	Role              Role      `json:"role"`
	CharacterEntityID *string   `json:"character_entity_id,omitempty"`
	JoinedAt          time.Time `json:"joined_at"`

	// Joined from users table for display purposes.
	DisplayName   string  `json:"display_name,omitempty"`
	Email         string  `json:"email,omitempty"`
	AvatarPath    *string `json:"avatar_path,omitempty"`
	// Joined from entities table for character display.
	CharacterName *string `json:"character_name,omitempty"`
}

// CampaignContext holds the resolved campaign and the requesting user's
// effective permissions. Injected into the Echo context by
// RequireCampaignAccess middleware.
//
// Two permission concepts:
//   - MemberRole: actual campaign_members role (for content visibility)
//   - IsSiteAdmin: site-level admin flag (for admin actions via /admin routes)
//
// An admin who joins as Player sees Player-visible content only.
// An admin who hasn't joined has MemberRole=RoleNone (no content access).
//
// Anonymous / public-visitor identity (C-PERM-ANON-IDENTITY): a viewer who is
// not a member of the campaign (logged-out public visitor, or an authenticated
// non-member browsing a public campaign) gets MemberRole=RoleNone — strictly
// BELOW RolePlayer. RolePlayer therefore means "an authenticated party member"
// and nothing weaker. IsMember/IsAnonymous record WHY MemberRole is what it is,
// so the visibility-read path and the glance badge can tell "the public" apart
// from "a Player" without re-deriving it from the role int.
type CampaignContext struct {
	Campaign    *Campaign
	MemberRole  Role // Actual membership role, or RoleNone if not a member.
	IsSiteAdmin bool // True if user has users.is_admin flag.
	IsDmGranted bool // True if user has been granted dm_only visibility by Owner.
	// IsMember is true only when MemberRole came from a real campaign_members
	// row. False for anonymous visitors, authenticated non-members, and
	// admins viewing without joining. Lets callers distinguish "RoleNone
	// because not a member" from a genuine low role.
	IsMember bool
	// IsAnonymous is true when there is no authenticated session at all
	// (logged-out public-campaign visitor). Implies !IsMember. Used by the
	// glance badge to label content "visible to the public".
	IsAnonymous bool
}

// EffectiveRole returns the permission level to use for route-level authorization.
// Site admins who are not members still get RoleNone here -- they should use
// /admin routes instead for admin operations.
func (cc *CampaignContext) EffectiveRole() Role {
	return cc.MemberRole
}

// VisibilityRole returns the effective role for content visibility filtering.
// DM-granted users are treated as Owners for visibility purposes so they
// can see dm_only content, while their actual MemberRole stays unchanged
// for authorization (create/edit) checks.
//
// For anonymous / non-member viewers this returns int(RoleNone) == 0, which is
// below RolePlayer (1). In the visibility filter a role-tier grant matches when
// subject_id <= viewerRole, so a viewer at 0 matches no Player-role grant — the
// public sees only content marked public/everyone (default + not-private, or an
// explicit 'public' grant), never Player-only or Player-role tag grants.
func (cc *CampaignContext) VisibilityRole() int {
	if cc.IsDmGranted {
		return int(RoleOwner)
	}
	return int(cc.MemberRole)
}

// CanControlWorldState reports whether the user may drive live world-state
// (advance time, set weather/mood) — the authority the Phase-4 GM panel and
// the world-state PUT (#401) gate on. Co-DM capability (C-CAL-COGM-CAPABILITY,
// D6): the campaign Owner OR a DM-grantee. A DmGrant is now a co-DM grant, not
// just secret-viewing — see CanAuthorDmOnly + the relabeled grant UI.
func (cc *CampaignContext) CanControlWorldState() bool {
	return cc.MemberRole >= RoleOwner || cc.IsDmGranted
}

// CanAuthorDmOnly reports whether the user may CREATE/mark dm_only content
// (e.g. a secret coming eclipse). Same co-DM capability as world-state
// control. Distinct from CanSeeDmOnly (read) — authoring is the write side a
// DmGrant now also confers. Route the calendar create/edit + the visibility
// UI through this so the grant is honored (not just VisibilityRole).
func (cc *CampaignContext) CanAuthorDmOnly() bool {
	return cc.MemberRole >= RoleOwner || cc.IsDmGranted
}

// OwnershipTransfer represents a pending campaign ownership transfer.
type OwnershipTransfer struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"campaign_id"`
	FromUserID string    `json:"from_user_id"`
	ToUserID   string    `json:"to_user_id"`
	Token      string    `json:"-"` // Never expose in JSON.
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// --- Dashboard Layout Types ---

// DefaultDashboardLayout returns the synthesized dashboard layout used
// when a campaign has no custom layout saved. Single source of truth for
// both the live page render and the customization editor — operators
// see the same structure they're about to edit. Each row is full-width
// (12-grid) so existing block components render at their natural width.
func DefaultDashboardLayout() *DashboardLayout {
	full := func(id, blockType string) DashboardRow {
		return DashboardRow{
			ID: "row-" + id,
			Columns: []DashboardColumn{
				{
					ID:    "col-" + id,
					Width: 12,
					Blocks: []DashboardBlock{
						{ID: "blk-" + id, Type: blockType},
					},
				},
			},
		}
	}
	return &DashboardLayout{
		Rows: []DashboardRow{
			full("welcome", BlockWelcomeBanner),
			full("quick", BlockQuickActions),
			full("categories", BlockCategoryGrid),
			full("recent", BlockRecentPages),
		},
	}
}

// DashboardLayout defines a configurable dashboard using a row/column/block
// grid system inspired by Kanka. Stored as JSON in the dashboard_layout column
// of campaigns (and entity_types for category dashboards). NULL means "use the
// hardcoded default dashboard".
type DashboardLayout struct {
	Rows []DashboardRow `json:"rows"`
}

// DashboardRow is a horizontal row in the dashboard grid.
type DashboardRow struct {
	ID      string            `json:"id"`
	Columns []DashboardColumn `json:"columns"`
}

// DashboardColumn is a column within a row. Width is 1-12 (12-column grid).
type DashboardColumn struct {
	ID     string           `json:"id"`
	Width  int              `json:"width"` // 1-12 grid units.
	Blocks []DashboardBlock `json:"blocks"`
}

// DashboardBlock is a single content block within a column.
// The Type field determines which Templ component renders it.
// Config holds type-specific options (e.g. limit, entity_type_id).
type DashboardBlock struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

// RoleDashboardLayouts holds per-role campaign page layouts. When the
// dashboard_layout column uses the new role-keyed format, each role can have
// its own layout. Players and Scribes fall back to Default when their
// role-specific layout is nil.
type RoleDashboardLayouts struct {
	Default *DashboardLayout `json:"default,omitempty"`
	Player  *DashboardLayout `json:"player,omitempty"`
	Scribe  *DashboardLayout `json:"scribe,omitempty"`
}

// ParseDashboardLayout parses the campaign's dashboard_layout JSON into a
// DashboardLayout struct. Returns nil if the column is NULL (use default).
// For backward compatibility, this returns the "default" layout from the
// role-keyed format, or the bare layout from the legacy format.
func (c *Campaign) ParseDashboardLayout() *DashboardLayout {
	if c.DashboardLayout == nil || *c.DashboardLayout == "" {
		return nil
	}
	var layout DashboardLayout
	if err := json.Unmarshal([]byte(*c.DashboardLayout), &layout); err != nil {
		slog.Warn("failed to parse dashboard layout, using default",
			slog.String("campaign_id", c.ID),
			slog.String("error", err.Error()),
		)
		return nil
	}
	// If we got a valid layout with rows, it's legacy format.
	if len(layout.Rows) > 0 {
		return &layout
	}
	// Try role-keyed format.
	roles := c.parseRoleDashboardLayouts()
	if roles != nil && roles.Default != nil {
		return roles.Default
	}
	return nil
}

// ParseRoleDashboardLayout returns the dashboard layout for the given role.
// Falls back: role-specific → default → nil (use hardcoded default).
// Handles both legacy format (bare layout) and new role-keyed format.
func (c *Campaign) ParseRoleDashboardLayout(role Role) *DashboardLayout {
	if c.DashboardLayout == nil || *c.DashboardLayout == "" {
		return nil
	}

	// Try legacy format first (bare {"rows": [...]}).
	var bare DashboardLayout
	if err := json.Unmarshal([]byte(*c.DashboardLayout), &bare); err == nil && len(bare.Rows) > 0 {
		return &bare // Legacy: all roles see the same layout.
	}

	// Try role-keyed format.
	roles := c.parseRoleDashboardLayouts()
	if roles == nil {
		return nil
	}

	// Look up role-specific layout, fall back to default.
	switch role {
	case RolePlayer:
		if roles.Player != nil {
			return roles.Player
		}
	case RoleScribe:
		if roles.Scribe != nil {
			return roles.Scribe
		}
	}
	return roles.Default
}

// parseRoleDashboardLayouts attempts to parse the dashboard_layout column as
// the role-keyed wrapper format.
func (c *Campaign) parseRoleDashboardLayouts() *RoleDashboardLayouts {
	if c.DashboardLayout == nil || *c.DashboardLayout == "" {
		return nil
	}
	var roles RoleDashboardLayouts
	if err := json.Unmarshal([]byte(*c.DashboardLayout), &roles); err != nil {
		return nil
	}
	if roles.Default == nil && roles.Player == nil && roles.Scribe == nil {
		return nil
	}
	return &roles
}

// GetRoleDashboardJSON extracts a single role's layout from the dashboard_layout
// column. Used by the API to return role-specific layouts to the editor.
func (c *Campaign) GetRoleDashboardJSON(roleName string) *DashboardLayout {
	if c.DashboardLayout == nil || *c.DashboardLayout == "" {
		return nil
	}

	// Check legacy format (bare layout → treat as "default").
	var bare DashboardLayout
	if err := json.Unmarshal([]byte(*c.DashboardLayout), &bare); err == nil && len(bare.Rows) > 0 {
		if roleName == "default" || roleName == "" {
			return &bare
		}
		return nil // Legacy format has no role-specific layouts.
	}

	// Role-keyed format.
	roles := c.parseRoleDashboardLayouts()
	if roles == nil {
		return nil
	}
	switch roleName {
	case "player":
		return roles.Player
	case "scribe":
		return roles.Scribe
	default:
		return roles.Default
	}
}

// SetRoleDashboardJSON updates a single role's layout within the role-keyed
// wrapper format and returns the new full JSON string. Migrates legacy format
// to role-keyed format automatically.
func (c *Campaign) SetRoleDashboardJSON(roleName string, layout *DashboardLayout) (*string, error) {
	var roles RoleDashboardLayouts

	if c.DashboardLayout != nil && *c.DashboardLayout != "" {
		// Check if legacy format.
		var bare DashboardLayout
		if err := json.Unmarshal([]byte(*c.DashboardLayout), &bare); err == nil && len(bare.Rows) > 0 {
			// Migrate legacy to role-keyed: existing layout becomes "default".
			roles.Default = &bare
		} else {
			// Try parsing as role-keyed.
			_ = json.Unmarshal([]byte(*c.DashboardLayout), &roles)
		}
	}

	switch roleName {
	case "player":
		roles.Player = layout
	case "scribe":
		roles.Scribe = layout
	default:
		roles.Default = layout
	}

	// If all roles are nil, return nil (reset to default).
	if roles.Default == nil && roles.Player == nil && roles.Scribe == nil {
		return nil, nil
	}

	data, err := json.Marshal(&roles)
	if err != nil {
		return nil, err
	}
	s := string(data)
	return &s, nil
}

// RemoveRoleDashboardJSON removes a single role's layout from the wrapper.
// Returns the new full JSON string, or nil if all roles are now empty.
func (c *Campaign) RemoveRoleDashboardJSON(roleName string) (*string, error) {
	return c.SetRoleDashboardJSON(roleName, nil)
}

// ParseOwnerDashboardLayout parses the campaign's owner_dashboard_layout JSON
// into a DashboardLayout struct. Returns nil if the column is NULL (use default).
func (c *Campaign) ParseOwnerDashboardLayout() *DashboardLayout {
	if c.OwnerDashboardLayout == nil || *c.OwnerDashboardLayout == "" {
		return nil
	}
	var layout DashboardLayout
	if err := json.Unmarshal([]byte(*c.OwnerDashboardLayout), &layout); err != nil {
		slog.Warn("failed to parse owner dashboard layout, using default",
			slog.String("campaign_id", c.ID),
			slog.String("error", err.Error()),
		)
		return nil
	}
	return &layout
}

// CampaignSettings holds campaign-level configuration stored as JSON in
// the campaigns.settings column. Accent color, display preferences, etc.
type CampaignSettings struct {
	AccentColor       string       `json:"accent_color,omitempty"`        // Hex color, e.g. "#6366f1". Slot 1 "Chrome": header + nav + global interactive (C-ACCENT-TRIO rev 2).
	AccentSurface1    string       `json:"accent_surface_1,omitempty"`    // Surface-pair accent A (primary) for themed content surfaces. Empty = inherit AccentColor (cordinator design D14 rev).
	AccentSurface2    string       `json:"accent_surface_2,omitempty"`    // Surface-pair accent B (secondary). Empty = inherit AccentColor.
	DmGrantIDs        []string     `json:"dm_grant_ids,omitempty"`        // User IDs granted dm_only visibility.
	BrandName         string       `json:"brand_name,omitempty"`          // Custom sidebar brand name (replaces campaign name).
	BrandLogo         string       `json:"brand_logo,omitempty"`          // Media path for brand logo image.
	TopbarStyle       *TopbarStyle   `json:"topbar_style,omitempty"`        // Topbar visual customization.
	TopbarContent     *TopbarContent `json:"topbar_content,omitempty"`     // Customizable topbar center content.
	FontFamily        string         `json:"font_family,omitempty"`       // Campaign body font: "serif", "sans-serif", "monospace", "georgia", "merriweather".
	WelcomeMessage    string       `json:"welcome_message,omitempty"`     // MOTD banner shown on campaign dashboard (max 500 chars).
	DefaultVisibility string       `json:"default_visibility,omitempty"`  // Default visibility for new entities: "", "dm_only", "private".
	SystemID          string       `json:"system_id,omitempty"`           // Game system ID (e.g. "dnd5e", "drawsteel") or "custom:<url>".

	// FoundryModulePin is the version string the campaign is pinned
	// to for the Chronicle-served Foundry module catalog (e.g. "0.1.5").
	// Empty = follow latest available (manifest endpoint resolves to
	// LatestAvailable). Set via the owner-side Foundry Module settings
	// tab; admins can override via the force-pin admin action.
	FoundryModulePin string `json:"foundry_module_pin,omitempty"`

	// FoundryModulePinMode controls how AutoPinOnInstall treats the
	// campaign when an admin installs a new module version. Added in
	// C-FMC-ADMIN-UX-AUDIT Chunk 1 (decisions/audit ref: §0.5 D5).
	// Valid values are the foundry_vtt.PinMode* constants:
	//   "preserve" → today's C-FMC-6 default; on install, campaign's
	//                pin is set to the previous version (state preserved).
	//   "promote"  → audit-resolved new default; on install, campaign's
	//                pin is set to the new version (auto-bump).
	//   "pinned"   → explicit version pin; install doesn't touch.
	// Empty string = "not yet set" (pre-Chunk-6 backfill). Chunk 2's
	// hook treats empty as the C-FMC-6 default until Chunk 6's
	// migration writes the resolved default to every empty-mode row.
	FoundryModulePinMode string `json:"foundry_module_pin_mode,omitempty"`

	// EventTierDefinitions holds the per-campaign event tier vocabulary
	// (V2 Wave 0 PR 2 per decisions/2026-05-28-cal-timeline-v2-design.md
	// §C1). Empty/nil at read time returns the platform default trio
	// (major / standard / detail) via GetEventTierDefinitions — matches
	// the empty-means-default pattern used for AccentColor, FontFamily,
	// etc. No migration on the campaigns table; tier defs nest in the
	// existing settings JSON per Option B locked 2026-05-28 post-
	// C-THEME-CUSTOMIZATION-AUDIT.
	EventTierDefinitions []TierDefinition `json:"event_tier_definitions,omitempty"`
}

// TierDefinition is a single entry in the per-campaign event tier
// vocabulary. Slugs are stored on Event.Tier (via the calendar plugin's
// V2 Wave 0 PR 2 migration) as foreign-key-like references; the
// TierDefinition supplies the rendering hints (color, prominence) the
// Wave 1 calendar UI consumes. Exactly one tier per definition set
// carries IsDefault=true.
type TierDefinition struct {
	Slug       string `json:"slug"`        // e.g. "major", "standard", "detail", "encounter"
	Name       string `json:"name"`        // display name
	Color      string `json:"color"`       // hex, validated as #RRGGBB
	Prominence int    `json:"prominence"`  // 0-100 visual weight; higher = larger/bolder render
	IsDefault  bool   `json:"is_default"`  // exactly one per definition set
}

// TopbarStyle configures the visual appearance of the campaign's top navigation bar.
type TopbarStyle struct {
	Mode         string `json:"mode"`                       // "solid", "gradient", or "image".
	Color        string `json:"color,omitempty"`             // Hex color for solid mode.
	GradientFrom string `json:"gradient_from,omitempty"`     // Start color for gradient mode.
	GradientTo   string `json:"gradient_to,omitempty"`       // End color for gradient mode.
	GradientDir  string `json:"gradient_dir,omitempty"`      // Direction: "to-r", "to-br", etc.
	ImagePath    string `json:"image_path,omitempty"`        // Media path for background image.
}

// TopbarContent configures what the owner wants displayed in the topbar center area.
type TopbarContent struct {
	Mode  string       `json:"mode"`            // "none", "links", "quote".
	Quote string       `json:"quote,omitempty"` // Text to display in quote mode (max 200 chars).
	Links []TopbarLink `json:"links,omitempty"` // Quick-link buttons in links mode (max 8).
}

// TopbarLink is a quick-link button displayed in the topbar center area.
type TopbarLink struct {
	Label string `json:"label"`              // Button text (max 30 chars).
	URL   string `json:"url"`                // Navigation URL.
	Icon  string `json:"icon,omitempty"`     // Optional FA icon class (e.g., "fa-user").
}

// ParseSettings parses the campaign's settings JSON into a CampaignSettings
// struct. Returns a zero-value struct if parsing fails or settings are empty.
func (c *Campaign) ParseSettings() CampaignSettings {
	var s CampaignSettings
	if c.Settings == "" || c.Settings == "{}" {
		return s
	}
	_ = json.Unmarshal([]byte(c.Settings), &s)
	return s
}

// Supported dashboard block types. Each maps to a Templ component that knows
// how to render the block with its config. Used by both campaign and category
// dashboard editors.
const (
	// Campaign dashboard blocks.
	BlockWelcomeBanner = "welcome_banner" // Campaign name + description hero.
	BlockQuickActions  = "quick_actions"  // All Pages / Members / Categories cards.
	BlockCategoryGrid  = "category_grid"  // Quick-nav grid of entity types.
	BlockRecentPages   = "recent_pages"   // Recently updated entities.
	BlockEntityList    = "entity_list"    // Filtered entity list by category.
	BlockTextBlock     = "text_block"     // Custom rich text / markdown.
	BlockPinnedPages     = "pinned_pages"     // Pinned entities grid.
	BlockCalendarPreview = "calendar_preview" // Upcoming calendar events.
	BlockTimelinePreview = "timeline_preview" // Timeline visualization preview.
	// BlockMapPreview ("map_preview") was retired — superseded by the
	// per-entity map_editor block (entity templates) and BlockMapFull
	// (dashboards). Constant removed; existing layouts with the type
	// silently drop the block at render via the case-handler removal.
	BlockRelationsGraph = "relations_graph" // Entity relations force-directed graph.
	BlockCalendarFull    = "calendar_full"    // Full interactive calendar grid view.
	BlockTimelineFull    = "timeline_full"    // Full timeline visualization with D3.
	BlockRelationsGraphFull = "relations_graph_full" // Large relations graph view.
	BlockMapFull         = "map_full"         // Full interactive map viewer with Phase 2 objects.
	BlockSessionTracker  = "session_tracker"  // Upcoming sessions with RSVP status.
	BlockActivityFeed    = "activity_feed"    // Recent campaign activity log.
	BlockSyncStatus      = "sync_status"      // Foundry VTT sync health/status.

	// Category dashboard blocks.
	BlockCategoryHeader = "category_header" // Category name, icon, count, description.
	BlockEntityGrid     = "entity_grid"     // All entities in category as card grid.
	BlockSearchBar      = "search_bar"      // Search input for filtering within category.
)

// ValidBlockTypes is the set of supported dashboard block types. Used for
// validation when saving layouts (both campaign and category dashboards).
var ValidBlockTypes = map[string]bool{
	BlockWelcomeBanner:  true,
	BlockQuickActions:   true,
	BlockCategoryGrid:   true,
	BlockRecentPages:    true,
	BlockEntityList:     true,
	BlockTextBlock:      true,
	BlockPinnedPages:     true,
	BlockCalendarPreview: true,
	BlockTimelinePreview: true,
	BlockRelationsGraph:  true,
	BlockCalendarFull:    true,
	BlockTimelineFull:    true,
	BlockRelationsGraphFull: true,
	BlockMapFull:         true,
	BlockSessionTracker:  true,
	BlockActivityFeed:    true,
	BlockSyncStatus:      true,
	BlockCategoryHeader:  true,
	BlockEntityGrid:     true,
	BlockSearchBar:      true,
}

// --- Cross-Plugin Interfaces ---

// UserFinder finds users for membership operations. Avoids importing the
// auth plugin's types directly. Implemented by UserFinderAdapter which
// wraps auth.UserRepository.
type UserFinder interface {
	FindUserByEmail(ctx context.Context, email string) (*MemberUser, error)
	FindUserByID(ctx context.Context, id string) (*MemberUser, error)
}

// MemberUser is the minimal user info needed for membership operations.
type MemberUser struct {
	ID          string
	Email       string
	DisplayName string
}

// MailService is the interface for sending email. Implemented by the SMTP
// plugin. Campaigns depends on this for ownership transfer emails. May be
// nil if SMTP is not configured.
type MailService interface {
	SendMail(ctx context.Context, to []string, subject, body string) error
	IsConfigured(ctx context.Context) bool
}

// EntityTypeSeeder seeds default entity types when a campaign is created.
// Implemented by the entities plugin's EntityService. Avoids importing the
// entities package directly.
type EntityTypeSeeder interface {
	SeedDefaults(ctx context.Context, campaignID string) error
	SeedGenre(ctx context.Context, campaignID string, genre string) error
}

// ContentTemplateSeeder seeds default content templates when a campaign is
// created. Implemented by the entities plugin's ContentTemplateService.
type ContentTemplateSeeder interface {
	SeedDefaults(ctx context.Context, campaignID string) error
}

// WorldbuildingPromptSeeder seeds default worldbuilding prompts when a campaign
// is created. Implemented by the entities plugin's WorldbuildingPromptService.
type WorldbuildingPromptSeeder interface {
	SeedDefaults(ctx context.Context, campaignID string) error
}

// LayoutPresetSeeder seeds default layout presets when a campaign is created.
// Implemented by the entities plugin's LayoutPresetService.
type LayoutPresetSeeder interface {
	SeedDefaults(ctx context.Context, campaignID string) error
}

// --- Request DTOs (bound from HTTP requests) ---

// CreateCampaignRequest holds the data submitted by the campaign creation form.
type CreateCampaignRequest struct {
	Name        string `json:"name" form:"name"`
	Description string `json:"description" form:"description"`
	Genre       string `json:"genre" form:"genre"`
}

// UpdateCampaignRequest holds the data submitted by the campaign edit form.
type UpdateCampaignRequest struct {
	Name        string `json:"name" form:"name"`
	Description string `json:"description" form:"description"`
	IsPublic    bool   `json:"is_public" form:"is_public"`
}

// AddMemberRequest holds the data for adding a member to a campaign.
type AddMemberRequest struct {
	Email string `json:"email" form:"email"`
	Role  string `json:"role" form:"role"`
}

// UpdateRoleRequest holds the data for changing a member's role.
type UpdateRoleRequest struct {
	Role string `json:"role" form:"role"`
}

// TransferOwnershipRequest holds the data for initiating an ownership transfer.
type TransferOwnershipRequest struct {
	Email string `json:"email" form:"email"`
}

// UpdateSidebarConfigRequest holds the data for updating sidebar configuration.
// Pointer fields let the server distinguish "field absent from JSON" (nil →
// preserve stored value) from "field explicitly sent" (non-nil → replace
// stored value, even an explicitly-empty slice clears the field).
type UpdateSidebarConfigRequest struct {
	Items           *[]SidebarItem `json:"items"`
	HiddenEntityIDs *[]string      `json:"hidden_entity_ids"`
	HiddenNodeIDs   *[]string      `json:"hidden_node_ids"`
}

// --- Service Input DTOs ---

// CreateCampaignInput is the validated input for creating a campaign.
type CreateCampaignInput struct {
	Name        string
	Description string
	Genre       string // Optional genre preset for entity type seeding.
}

// UpdateCampaignInput is the validated input for updating a campaign.
type UpdateCampaignInput struct {
	Name        string
	Description string
	IsPublic    bool
}

// ListOptions holds pagination parameters for list queries.
type ListOptions struct {
	Page    int
	PerPage int
}

// DefaultListOptions returns sensible defaults for pagination.
func DefaultListOptions() ListOptions {
	return ListOptions{Page: 1, PerPage: 24}
}

// Offset returns the SQL OFFSET value for the current page.
func (o ListOptions) Offset() int {
	if o.Page < 1 {
		o.Page = 1
	}
	return (o.Page - 1) * o.PerPage
}

// --- Slug Generation ---

// --- Campaign Groups ---

// CampaignGroup is a named collection of campaign members used for
// per-entity permission grants (e.g., "Party A can see this entity").
type CampaignGroup struct {
	ID          int        `json:"id"`
	CampaignID  string     `json:"campaign_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Members     []GroupMemberInfo `json:"members,omitempty"`
}

// GroupMemberInfo is a campaign member's summary within a group.
type GroupMemberInfo struct {
	UserID      string  `json:"user_id"`
	DisplayName string  `json:"display_name"`
	Email       string  `json:"email"`
	Role        Role    `json:"role"`
	AvatarPath  *string `json:"avatar_path,omitempty"`
}

// slugPattern matches one or more non-alphanumeric characters for replacement.
var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify creates a URL-safe slug from a name. Lowercase, replace
// non-alphanumeric characters with hyphens, trim leading/trailing hyphens.
func Slugify(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = slugPattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "campaign"
	}
	return slug
}
