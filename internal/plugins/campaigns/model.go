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
	DashboardLayout *string   `json:"dashboard_layout,omitempty"` // JSON layout; nil = use hardcoded default.
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SidebarConfig holds campaign-level sidebar customization settings.
// Stored as JSON in campaigns.sidebar_config. Controls entity type ordering,
// visibility, and custom navigation elements in the sidebar.
type SidebarConfig struct {
	// EntityTypeOrder is an ordered list of entity type IDs controlling
	// sidebar display order. Types not listed appear at the end.
	EntityTypeOrder []int `json:"entity_type_order,omitempty"`

	// HiddenTypeIDs is a set of entity type IDs that should not appear
	// in the sidebar. Hidden types are still accessible via the All Entities page.
	HiddenTypeIDs []int `json:"hidden_type_ids,omitempty"`

	// CustomSections are labeled dividers that appear between entity type
	// groups in the sidebar. Each section appears after the entity type
	// referenced by its After field.
	CustomSections []NavSection `json:"custom_sections,omitempty"`

	// CustomLinks are additional navigation items in the sidebar. They can
	// be internal (relative URL) or external (absolute URL) links.
	CustomLinks []NavLink `json:"custom_links,omitempty"`
}

// NavSection represents a labeled divider in the sidebar navigation.
// Renders as a section header between entity type groups.
type NavSection struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	After string `json:"after"` // Entity type ID this appears after ("" = top).
}

// NavLink represents a custom link in the sidebar navigation.
type NavLink struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	URL      string `json:"url"`
	Icon     string `json:"icon"`    // FontAwesome icon class (e.g. "fa-globe").
	Section  string `json:"section"` // NavSection ID this belongs to ("" = top level).
	Position int    `json:"position"`
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

// CampaignMember represents a user's membership in a campaign.
type CampaignMember struct {
	CampaignID string    `json:"campaign_id"`
	UserID     string    `json:"user_id"`
	Role       Role      `json:"role"`
	JoinedAt   time.Time `json:"joined_at"`

	// Joined from users table for display purposes.
	DisplayName string  `json:"display_name,omitempty"`
	Email       string  `json:"email,omitempty"`
	AvatarPath  *string `json:"avatar_path,omitempty"`
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
type CampaignContext struct {
	Campaign    *Campaign
	MemberRole  Role // Actual membership role, or RoleNone if not a member.
	IsSiteAdmin bool // True if user has users.is_admin flag.
}

// EffectiveRole returns the permission level to use for route-level authorization.
// Site admins who are not members still get RoleNone here -- they should use
// /admin routes instead for admin operations.
func (cc *CampaignContext) EffectiveRole() Role {
	return cc.MemberRole
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

// ParseDashboardLayout parses the campaign's dashboard_layout JSON into a
// DashboardLayout struct. Returns nil if the column is NULL (use default).
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
	return &layout
}

// Supported dashboard block types. Each maps to a Templ component that knows
// how to render the block with its config.
const (
	BlockWelcomeBanner = "welcome_banner" // Campaign name + description hero.
	BlockCategoryGrid  = "category_grid"  // Quick-nav grid of entity types.
	BlockRecentPages   = "recent_pages"   // Recently updated entities.
	BlockEntityList    = "entity_list"    // Filtered entity list by category.
	BlockTextBlock     = "text_block"     // Custom rich text / markdown.
	BlockPinnedPages   = "pinned_pages"   // Pinned entities grid.
)

// ValidBlockTypes is the set of supported dashboard block types. Used for
// validation when saving layouts.
var ValidBlockTypes = map[string]bool{
	BlockWelcomeBanner: true,
	BlockCategoryGrid:  true,
	BlockRecentPages:   true,
	BlockEntityList:    true,
	BlockTextBlock:     true,
	BlockPinnedPages:   true,
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
}

// --- Request DTOs (bound from HTTP requests) ---

// CreateCampaignRequest holds the data submitted by the campaign creation form.
type CreateCampaignRequest struct {
	Name        string `json:"name" form:"name"`
	Description string `json:"description" form:"description"`
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
type UpdateSidebarConfigRequest struct {
	EntityTypeOrder []int        `json:"entity_type_order"`
	HiddenTypeIDs   []int        `json:"hidden_type_ids"`
	CustomSections  []NavSection `json:"custom_sections,omitempty"`
	CustomLinks     []NavLink    `json:"custom_links,omitempty"`
}

// --- Service Input DTOs ---

// CreateCampaignInput is the validated input for creating a campaign.
type CreateCampaignInput struct {
	Name        string
	Description string
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
