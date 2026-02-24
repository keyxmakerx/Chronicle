// data.go provides typed context helpers for passing layout data from
// handlers/middleware to Templ templates. This avoids importing plugin
// types in the layouts package — only simple types are stored.
//
// Data flow: Handler/Middleware → Echo Context → LayoutInjector → Go Context → Templ
package layouts

import "context"

// ctxKey is a private type for context keys to prevent collisions.
type ctxKey string

const (
	keyIsAuthenticated ctxKey = "layout_is_authenticated"
	keyUserID          ctxKey = "layout_user_id"
	keyUserName        ctxKey = "layout_user_name"
	keyUserEmail       ctxKey = "layout_user_email"
	keyIsAdmin         ctxKey = "layout_is_admin"
	keyCampaignID    ctxKey = "layout_campaign_id"
	keyCampaignName  ctxKey = "layout_campaign_name"
	keyCampaignRole  ctxKey = "layout_campaign_role"
	keyCSRFToken     ctxKey = "layout_csrf_token"
	keyFlashSuccess  ctxKey = "layout_flash_success"
	keyFlashError    ctxKey = "layout_flash_error"
	keyActivePath    ctxKey = "layout_active_path"
	keyEntityTypes   ctxKey = "layout_entity_types"
	keyEntityCounts  ctxKey = "layout_entity_counts"
	keyEnabledAddons   ctxKey = "layout_enabled_addons"
	keyCustomSections  ctxKey = "layout_custom_sections"
	keyCustomLinks     ctxKey = "layout_custom_links"
)

// SidebarEntityType holds the minimum entity type info needed for sidebar
// rendering. Defined here to avoid importing the entities package.
type SidebarEntityType struct {
	ID         int
	Slug       string
	Name       string
	NamePlural string
	Icon       string
	Color      string
	SortOrder  int
}

// SortSidebarTypes reorders entity types according to a sidebar config
// ordering and filters out hidden types. Types not in the order list appear
// at the end in their original sort_order.
func SortSidebarTypes(types []SidebarEntityType, order []int, hidden []int) []SidebarEntityType {
	// Build hidden set.
	hiddenSet := make(map[int]bool, len(hidden))
	for _, id := range hidden {
		hiddenSet[id] = true
	}

	// If no custom order, just filter hidden.
	if len(order) == 0 {
		result := make([]SidebarEntityType, 0, len(types))
		for _, t := range types {
			if !hiddenSet[t.ID] {
				result = append(result, t)
			}
		}
		return result
	}

	// Build a map for quick lookup.
	typeMap := make(map[int]SidebarEntityType, len(types))
	for _, t := range types {
		typeMap[t.ID] = t
	}

	// Ordered types first.
	seen := make(map[int]bool, len(order))
	result := make([]SidebarEntityType, 0, len(types))
	for _, id := range order {
		if hiddenSet[id] {
			continue
		}
		if t, ok := typeMap[id]; ok {
			result = append(result, t)
			seen[id] = true
		}
	}

	// Remaining types not in the order list (preserving original sort_order).
	for _, t := range types {
		if !seen[t.ID] && !hiddenSet[t.ID] {
			result = append(result, t)
		}
	}

	return result
}

// --- Setters (called by the layout injector in app/routes.go) ---

// SetIsAuthenticated marks whether the current request has a valid session.
func SetIsAuthenticated(ctx context.Context, authed bool) context.Context {
	return context.WithValue(ctx, keyIsAuthenticated, authed)
}

// SetUserID stores the authenticated user's ID in context.
func SetUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyUserID, id)
}

// SetUserName stores the authenticated user's display name in context.
func SetUserName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, keyUserName, name)
}

// SetUserEmail stores the authenticated user's email in context.
func SetUserEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, keyUserEmail, email)
}

// SetIsAdmin stores whether the user is a site admin.
func SetIsAdmin(ctx context.Context, isAdmin bool) context.Context {
	return context.WithValue(ctx, keyIsAdmin, isAdmin)
}

// SetCampaignID stores the current campaign's ID in context.
func SetCampaignID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyCampaignID, id)
}

// SetCampaignName stores the current campaign's display name in context.
func SetCampaignName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, keyCampaignName, name)
}

// SetCampaignRole stores the user's campaign membership role (as int).
func SetCampaignRole(ctx context.Context, role int) context.Context {
	return context.WithValue(ctx, keyCampaignRole, role)
}

// SetCSRFToken stores the CSRF token for forms.
func SetCSRFToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, keyCSRFToken, token)
}

// SetFlashSuccess stores a success flash message for the current render.
func SetFlashSuccess(ctx context.Context, msg string) context.Context {
	return context.WithValue(ctx, keyFlashSuccess, msg)
}

// SetFlashError stores an error flash message for the current render.
func SetFlashError(ctx context.Context, msg string) context.Context {
	return context.WithValue(ctx, keyFlashError, msg)
}

// SetActivePath stores the current request path for nav highlighting.
func SetActivePath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, keyActivePath, path)
}

// --- Getters (called by Templ templates) ---

// IsAuthenticated returns true if the current request has a valid session.
func IsAuthenticated(ctx context.Context) bool {
	authed, _ := ctx.Value(keyIsAuthenticated).(bool)
	return authed
}

// GetUserID returns the authenticated user's ID, or "".
func GetUserID(ctx context.Context) string {
	id, _ := ctx.Value(keyUserID).(string)
	return id
}

// GetUserName returns the authenticated user's display name, or "".
func GetUserName(ctx context.Context) string {
	name, _ := ctx.Value(keyUserName).(string)
	return name
}

// GetUserEmail returns the authenticated user's email, or "".
func GetUserEmail(ctx context.Context) string {
	email, _ := ctx.Value(keyUserEmail).(string)
	return email
}

// GetIsAdmin returns true if the user is a site admin.
func GetIsAdmin(ctx context.Context) bool {
	isAdmin, _ := ctx.Value(keyIsAdmin).(bool)
	return isAdmin
}

// GetCampaignID returns the current campaign ID, or "" if not in campaign context.
func GetCampaignID(ctx context.Context) string {
	id, _ := ctx.Value(keyCampaignID).(string)
	return id
}

// GetCampaignName returns the current campaign name, or "".
func GetCampaignName(ctx context.Context) string {
	name, _ := ctx.Value(keyCampaignName).(string)
	return name
}

// GetCampaignRole returns the user's campaign role as int, or 0.
func GetCampaignRole(ctx context.Context) int {
	role, _ := ctx.Value(keyCampaignRole).(int)
	return role
}

// GetCSRFToken returns the CSRF token, or "".
func GetCSRFToken(ctx context.Context) string {
	token, _ := ctx.Value(keyCSRFToken).(string)
	return token
}

// GetFlashSuccess returns a success flash message, or "".
func GetFlashSuccess(ctx context.Context) string {
	msg, _ := ctx.Value(keyFlashSuccess).(string)
	return msg
}

// GetFlashError returns an error flash message, or "".
func GetFlashError(ctx context.Context) string {
	msg, _ := ctx.Value(keyFlashError).(string)
	return msg
}

// GetActivePath returns the current request path for nav highlighting.
func GetActivePath(ctx context.Context) string {
	path, _ := ctx.Value(keyActivePath).(string)
	return path
}

// InCampaign returns true if we're currently in a campaign context.
func InCampaign(ctx context.Context) bool {
	return GetCampaignID(ctx) != ""
}

// --- Entity Types (for sidebar) ---

// SetEntityTypes stores the campaign's entity types for sidebar rendering.
func SetEntityTypes(ctx context.Context, types []SidebarEntityType) context.Context {
	return context.WithValue(ctx, keyEntityTypes, types)
}

// GetEntityTypes returns the campaign's entity types for the sidebar.
func GetEntityTypes(ctx context.Context) []SidebarEntityType {
	types, _ := ctx.Value(keyEntityTypes).([]SidebarEntityType)
	return types
}

// SetEntityCounts stores per-type entity counts for sidebar badges.
func SetEntityCounts(ctx context.Context, counts map[int]int) context.Context {
	return context.WithValue(ctx, keyEntityCounts, counts)
}

// GetEntityCounts returns per-type entity counts for sidebar badges.
func GetEntityCounts(ctx context.Context) map[int]int {
	counts, _ := ctx.Value(keyEntityCounts).(map[int]int)
	return counts
}

// GetEntityCount returns the entity count for a specific type ID.
func GetEntityCount(ctx context.Context, typeID int) int {
	counts := GetEntityCounts(ctx)
	if counts == nil {
		return 0
	}
	return counts[typeID]
}

// --- Enabled Addons (for conditional widget rendering) ---

// SetEnabledAddons stores the set of addon slugs enabled for the current campaign.
func SetEnabledAddons(ctx context.Context, slugs map[string]bool) context.Context {
	return context.WithValue(ctx, keyEnabledAddons, slugs)
}

// IsAddonEnabled checks whether an addon is enabled for the current campaign.
func IsAddonEnabled(ctx context.Context, slug string) bool {
	addons, _ := ctx.Value(keyEnabledAddons).(map[string]bool)
	return addons[slug]
}

// --- Custom Sidebar Navigation (sections + links) ---

// SidebarSection represents a custom section header/divider in the sidebar.
// Defined here to avoid importing the campaigns package.
type SidebarSection struct {
	ID    string
	Label string
	After string // Entity type ID (as string) this appears after; "" = top.
}

// SidebarLink represents a custom link in the sidebar navigation.
type SidebarLink struct {
	ID      string
	Label   string
	URL     string
	Icon    string // FontAwesome icon class (e.g. "fa-globe").
	Section string // SidebarSection ID this belongs to; "" = top level.
}

// SetCustomSections stores custom sidebar sections in context.
func SetCustomSections(ctx context.Context, sections []SidebarSection) context.Context {
	return context.WithValue(ctx, keyCustomSections, sections)
}

// GetCustomSections returns custom sidebar sections from context.
func GetCustomSections(ctx context.Context) []SidebarSection {
	sections, _ := ctx.Value(keyCustomSections).([]SidebarSection)
	return sections
}

// SetCustomLinks stores custom sidebar links in context.
func SetCustomLinks(ctx context.Context, links []SidebarLink) context.Context {
	return context.WithValue(ctx, keyCustomLinks, links)
}

// GetCustomLinks returns custom sidebar links from context.
func GetCustomLinks(ctx context.Context) []SidebarLink {
	links, _ := ctx.Value(keyCustomLinks).([]SidebarLink)
	return links
}
