// Package campaigns manages campaigns (worldbuilding containers) and their
// role-based membership system. A campaign is the top-level organizational
// unit that holds all entities, maps, timelines, etc.
//
// This is a CORE plugin -- always enabled, cannot be disabled.
package campaigns

import (
	"context"
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
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	Settings    string    `json:"settings"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
