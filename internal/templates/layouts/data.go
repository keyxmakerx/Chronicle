// data.go provides typed context helpers for passing layout data from
// handlers/middleware to Templ templates. This avoids importing plugin
// types in the layouts package — only simple types are stored.
//
// Data flow: Handler/Middleware → Echo Context → LayoutInjector → Go Context → Templ
package layouts

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

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
	keyEnabledAddons     ctxKey = "layout_enabled_addons"
	keyEnabledSystem     ctxKey = "layout_enabled_system"
	keyViewingAsPlayer   ctxKey = "layout_viewing_as_player"
	keyIsOwner           ctxKey = "layout_is_owner"
	keyMediaURLFunc      ctxKey = "layout_media_url_func"
	keyMediaThumbFunc    ctxKey = "layout_media_thumb_func"
	keyExtWidgetScripts  ctxKey = "layout_ext_widget_scripts"
	keyPluginBodyScripts ctxKey = "layout_plugin_body_scripts"
	keySidebarItems      ctxKey = "layout_sidebar_items"
	keyAccentColor       ctxKey = "layout_accent_color"
	keyBrandName         ctxKey = "layout_brand_name"
	keyBrandLogo         ctxKey = "layout_brand_logo"
	keyTopbarStyle           ctxKey = "layout_topbar_style"
	keyTopbarContent         ctxKey = "layout_topbar_content"
	keyDegradedPluginCount   ctxKey = "layout_degraded_plugin_count"
	keyFontFamily            ctxKey = "layout_font_family"
	keyUserCampaigns         ctxKey = "layout_user_campaigns"
)

// NavCampaign holds the minimum info needed to render a campaign link
// in the topbar navigation. Defined here to avoid importing the campaigns package.
type NavCampaign struct {
	ID   string
	Name string
}

// SidebarEntityType holds the minimum entity type info needed for sidebar
// rendering. Defined here to avoid importing the entities package.
type SidebarEntityType struct {
	ID           int
	Slug         string
	Name         string
	NamePlural   string
	Icon         string
	Color        string
	SortOrder    int
	ParentTypeID *int // Parent entity type ID for sub-type hierarchy.
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

// --- User Campaigns (for topbar navigation) ---

// SetUserCampaigns stores the user's campaign list for topbar navigation.
func SetUserCampaigns(ctx context.Context, campaigns []NavCampaign) context.Context {
	return context.WithValue(ctx, keyUserCampaigns, campaigns)
}

// GetUserCampaigns returns the user's campaigns for topbar navigation, or nil.
func GetUserCampaigns(ctx context.Context) []NavCampaign {
	campaigns, _ := ctx.Value(keyUserCampaigns).([]NavCampaign)
	return campaigns
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

// EntityTypesJSON serializes the sidebar entity types to a JSON string for
// JavaScript widget data attributes.
func EntityTypesJSON(ctx context.Context) string {
	types := GetEntityTypes(ctx)
	type jsonET struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		NamePlural   string `json:"name_plural"`
		Icon         string `json:"icon"`
		Color        string `json:"color"`
		ParentTypeID *int   `json:"parent_type_id,omitempty"`
	}
	out := make([]jsonET, len(types))
	for i, t := range types {
		out[i] = jsonET{ID: t.ID, Name: t.Name, NamePlural: t.NamePlural, Icon: t.Icon, Color: t.Color, ParentTypeID: t.ParentTypeID}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
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

// EnabledSystem identifies the game system enabled for the current campaign,
// used to render its rulebook (reference) nav link. Slug is the system's
// module ID (the `:mod` path segment, e.g. "drawsteel").
type EnabledSystem struct {
	Slug string
	Name string
	Icon string
}

// SetEnabledSystem stores the campaign's enabled game system (if any).
func SetEnabledSystem(ctx context.Context, sys EnabledSystem) context.Context {
	return context.WithValue(ctx, keyEnabledSystem, sys)
}

// GetEnabledSystem returns the campaign's enabled game system. ok is false when
// no system is enabled (or no Slug resolved), so the rulebook link is omitted.
func GetEnabledSystem(ctx context.Context) (EnabledSystem, bool) {
	sys, _ := ctx.Value(keyEnabledSystem).(EnabledSystem)
	return sys, sys.Slug != ""
}

// --- Custom Sidebar Navigation (link items) ---

// SidebarLink represents a custom link item in the unified sidebar navigation
// (rendered by customNavLink from a SidebarItemView of type "link").
type SidebarLink struct {
	ID    string
	Label string
	URL   string
	Icon  string // FontAwesome icon class (e.g. "fa-globe").
}

// --- Unified Sidebar Items ---

// SidebarItemView is the template-ready representation of a sidebar item.
// Populated by the LayoutInjector from the campaign's SidebarConfig.Items.
//
// Sub-category entity_types (ParentTypeID != nil) are filtered out at
// build time in routes.go and never become SidebarItemViews — they are
// template variants of their parent, not navigable collections. The
// ParentTypeID field here is always nil in practice; it is retained as
// defensive documentation of the invariant.
type SidebarItemView struct {
	Type         string // "dashboard", "addon", "category", "section", "link", "all_pages"
	Slug         string // Addon slug (for addon items).
	TypeID       int    // Entity type ID (for category items).
	ID           string // Unique ID (for sections/links).
	Label        string // Display label.
	URL          string // Navigation URL.
	Icon         string // FontAwesome icon class.
	Color        string // Category color.
	Count        int    // Entity count (for categories).
	ParentTypeID *int   // Always nil for items in the sidebar; see type comment.
}

// SetSidebarItems stores the unified sidebar items in context.
func SetSidebarItems(ctx context.Context, items []SidebarItemView) context.Context {
	return context.WithValue(ctx, keySidebarItems, items)
}

// GetSidebarItems returns unified sidebar items from context.
// Returns nil if the campaign uses the legacy sidebar format.
func GetSidebarItems(ctx context.Context) []SidebarItemView {
	items, _ := ctx.Value(keySidebarItems).([]SidebarItemView)
	return items
}

// drillSearchURL builds the search endpoint URL for a drill panel.
// Sub-category entity_types are not surfaced in the sidebar, so the drill
// panel searches only the parent's own type — child entities flow into the
// parent's listing via aggregation in the entities service.
func drillSearchURL(_ context.Context, campaignID string, typeID int) string {
	return fmt.Sprintf("/campaigns/%s/entities/search?type=%d&sidebar=1", campaignID, typeID)
}

// getEntityTypeSlug looks up the slug for an entity type by ID from context.
func getEntityTypeSlug(ctx context.Context, typeID int) string {
	for _, et := range GetEntityTypes(ctx) {
		if et.ID == typeID {
			return et.Slug
		}
	}
	return ""
}

// --- View As Player (owner preview toggle) ---

// SetViewingAsPlayer marks whether the owner is currently in "view as player" mode.
func SetViewingAsPlayer(ctx context.Context, viewing bool) context.Context {
	return context.WithValue(ctx, keyViewingAsPlayer, viewing)
}

// IsViewingAsPlayer returns true if the owner has toggled "view as player" mode.
func IsViewingAsPlayer(ctx context.Context) bool {
	viewing, _ := ctx.Value(keyViewingAsPlayer).(bool)
	return viewing
}

// SetIsOwner stores whether the user's actual campaign role is Owner.
// This is separate from GetCampaignRole because "view as player" overrides
// GetCampaignRole to RolePlayer, but the toggle button must still render.
func SetIsOwner(ctx context.Context, isOwner bool) context.Context {
	return context.WithValue(ctx, keyIsOwner, isOwner)
}

// IsOwner returns true if the user's actual campaign role is Owner,
// regardless of the "view as player" display override.
func IsOwner(ctx context.Context) bool {
	isOwner, _ := ctx.Value(keyIsOwner).(bool)
	return isOwner
}

// --- Signed Media URLs ---

// MediaURLFunc generates a signed media URL given a file ID.
// The function encapsulates the HMAC signing logic so templates don't
// need to import the media package.
type MediaURLFunc func(fileID string) string

// MediaThumbFunc generates a signed thumbnail URL given a file ID and size.
type MediaThumbFunc func(fileID, size string) string

// SetMediaURLFunc stores the signed URL generator in context. Called by
// the LayoutInjector in app/routes.go after the URLSigner is created.
func SetMediaURLFunc(ctx context.Context, fn MediaURLFunc) context.Context {
	return context.WithValue(ctx, keyMediaURLFunc, fn)
}

// SetMediaThumbFunc stores the signed thumbnail URL generator in context.
func SetMediaThumbFunc(ctx context.Context, fn MediaThumbFunc) context.Context {
	return context.WithValue(ctx, keyMediaThumbFunc, fn)
}

// MediaURL returns a signed URL for a media file. Falls back to an
// unsigned URL if no signing function is configured (dev mode, migration).
// Normalizes path-format fileIDs (e.g., "2026/03/uuid.jpg") to just the UUID.
func MediaURL(ctx context.Context, fileID string) string {
	fileID = normalizeMediaID(fileID)
	if fn, ok := ctx.Value(keyMediaURLFunc).(MediaURLFunc); ok && fn != nil {
		return fn(fileID)
	}
	return "/media/" + fileID
}

// MediaThumbURL returns a signed URL for a media thumbnail at the given
// size. Falls back to an unsigned URL if no signing function is configured.
func MediaThumbURL(ctx context.Context, fileID, size string) string {
	fileID = normalizeMediaID(fileID)
	if fn, ok := ctx.Value(keyMediaURLFunc).(MediaURLFunc); ok && fn != nil {
		if thumbFn, ok2 := ctx.Value(keyMediaThumbFunc).(MediaThumbFunc); ok2 && thumbFn != nil {
			return thumbFn(fileID, size)
		}
	}
	return "/media/" + fileID + "/thumb/" + size
}

// normalizeMediaID extracts the UUID from a media file identifier.
// Handles both UUID-only ("b7c17bb1-...") and path-format ("2026/03/b7c17bb1-....jpg")
// inputs, returning just the UUID portion without extension.
func normalizeMediaID(fileID string) string {
	if !strings.Contains(fileID, "/") {
		return fileID
	}
	// Extract basename: "2026/03/b7c17bb1-6563-462c-8b49-5b2e8bd57108.jpg" → "b7c17bb1-...108.jpg"
	if idx := strings.LastIndex(fileID, "/"); idx >= 0 {
		fileID = fileID[idx+1:]
	}
	// Strip extension: "b7c17bb1-...108.jpg" → "b7c17bb1-...108"
	if idx := strings.LastIndex(fileID, "."); idx > 0 {
		fileID = fileID[:idx]
	}
	return fileID
}

// --- Extension Widget Scripts ---

// SetExtWidgetScripts stores the list of extension widget script URLs
// that should be injected into campaign pages.
func SetExtWidgetScripts(ctx context.Context, urls []string) context.Context {
	return context.WithValue(ctx, keyExtWidgetScripts, urls)
}

// GetExtWidgetScripts returns extension widget script URLs for the current campaign.
func GetExtWidgetScripts(ctx context.Context) []string {
	urls, _ := ctx.Value(keyExtWidgetScripts).([]string)
	return urls
}

// --- Plugin Body Scripts ---

// SetPluginBodyScripts stores script URLs that plugins contribute to the
// page <body> before the extension-widget block. Used so plugins can inject
// their own widget scripts without hardcoding plugin paths in the core layout.
// Called from App startup (RegisterRoutes) for always-on plugin assets; the
// calendar plugin's calendar_widget.js is the canonical first use-case.
//
// Unlike SetExtWidgetScripts (per-campaign, set per-request), plugin body
// scripts are global and constant for the lifetime of the process. They are
// set once during startup and read by base.templ on every request.
func SetPluginBodyScripts(ctx context.Context, urls []string) context.Context {
	return context.WithValue(ctx, keyPluginBodyScripts, urls)
}

// GetPluginBodyScripts returns plugin-contributed body script URLs.
func GetPluginBodyScripts(ctx context.Context) []string {
	urls, _ := ctx.Value(keyPluginBodyScripts).([]string)
	return urls
}

// SetAccentColor stores the campaign's custom accent color in the context.
func SetAccentColor(ctx context.Context, color string) context.Context {
	return context.WithValue(ctx, keyAccentColor, color)
}

// keyAccentSurface1/2 hold the campaign's surface-pair accents (C-ACCENT-TRIO
// rev 2, cordinator design D14 rev): slot 1 stays the chrome accent above;
// these two are consumed by themed content surfaces as primary/secondary via
// --color-accent-surface-1/2 tokens. Unset = surfaces fall back to the chrome
// accent through var(..., var(--color-accent)) chains at the consumer.
const (
	keyAccentSurface1 ctxKey = "layout_accent_surface_1"
	keyAccentSurface2 ctxKey = "layout_accent_surface_2"
)

// SetAccentSurface stores a surface-pair accent (slot 1 or 2) in the context.
// Any other slot value is ignored — the trio is a fixed vocabulary, not a list.
func SetAccentSurface(ctx context.Context, slot int, color string) context.Context {
	switch slot {
	case 1:
		return context.WithValue(ctx, keyAccentSurface1, color)
	case 2:
		return context.WithValue(ctx, keyAccentSurface2, color)
	}
	return ctx
}

// GetAccentSurface returns the surface-pair accent for slot 1 or 2, or empty
// string when unset (consumers then inherit the chrome accent via CSS fallback).
func GetAccentSurface(ctx context.Context, slot int) string {
	switch slot {
	case 1:
		color, _ := ctx.Value(keyAccentSurface1).(string)
		return color
	case 2:
		color, _ := ctx.Value(keyAccentSurface2).(string)
		return color
	}
	return ""
}

// GetAccentColor returns the campaign's custom accent color, or empty string for default.
func GetAccentColor(ctx context.Context) string {
	color, _ := ctx.Value(keyAccentColor).(string)
	return color
}

// keyAccentAction/keyAccentApp hold the campaign's two NEW semantic accent
// slots (C-ACCENT-SLOTS, operator-corrected mapping): slot 2 "Action
// highlight" (primary buttons, hover/press, FABs — no prior trio analog) and
// slot 3 "App accent" (per-app identity — character pages, calendar app).
// Site accent (slot 1) is unchanged: it's the existing keyAccentColor above.
const (
	keyAccentAction ctxKey = "layout_accent_action"
	keyAccentApp    ctxKey = "layout_accent_app"
)

// SetAccentAction stores the campaign's action-highlight accent in the context.
func SetAccentAction(ctx context.Context, color string) context.Context {
	return context.WithValue(ctx, keyAccentAction, color)
}

// GetAccentAction returns the campaign's action-highlight accent, or empty
// string when unset (consumers then inherit the site accent via CSS fallback).
func GetAccentAction(ctx context.Context) string {
	color, _ := ctx.Value(keyAccentAction).(string)
	return color
}

// SetAccentApp stores the campaign's app-identity accent in the context.
func SetAccentApp(ctx context.Context, color string) context.Context {
	return context.WithValue(ctx, keyAccentApp, color)
}

// GetAccentApp returns the campaign's app-identity accent, or empty string
// when unset (consumers then inherit the legacy surface-1 accent, then the
// site accent, via CSS fallback chains).
func GetAccentApp(ctx context.Context) string {
	color, _ := ctx.Value(keyAccentApp).(string)
	return color
}

// AccentColorCSS returns a CSS block that overrides the accent color custom
// properties. It computes hover (darker) and light (lighter) variants from the
// base hex color. Returns empty string if no accent is set.
func AccentColorCSS(ctx context.Context) string {
	// Site slot (semantic slot 1 — today's "chrome"). Its emission is
	// byte-identical to the pre-trio implementation — campaigns that never
	// touch the other slots must render EXACTLY the CSS they rendered before
	// (pinned by TestAccentColorCSS_*).
	css := accentSlotCSS("--color-accent", GetAccentColor(ctx))
	// Legacy surface pair (C-ACCENT-TRIO rev 2). Unset slots emit nothing —
	// consumers inherit chrome via var(--color-accent-surface-N, var(--color-accent)).
	// Kept as-is (C-ACCENT-SLOTS Step-0: map onto the new slots, don't delete).
	css += accentSlotCSS("--color-accent-surface-1", GetAccentSurface(ctx, 1))
	css += accentSlotCSS("--color-accent-surface-2", GetAccentSurface(ctx, 2))
	// New semantic slots (C-ACCENT-SLOTS). Unset emits nothing; consumers'
	// var() fallback chains (and the Tailwind action/app tokens) resolve
	// through the legacy trio down to the site accent, so this is additive —
	// it never changes the bytes emitted above.
	css += accentSlotCSS("--color-accent-action", GetAccentAction(ctx))
	css += accentSlotCSS("--color-accent-app", GetAccentApp(ctx))
	return css
}

// accentSlotCSS renders one accent slot's :root block: the base color plus
// derived hover (12% darken) and light (60% white-blend) variants and their
// RGB triples. The derivation is shared by all three slots so they can never
// drift (C-ACCENT-TRIO stop-and-flag: one derivation, no forks). Empty base
// emits nothing; a non-#RRGGBB base passes through without variants (legacy
// behavior for hand-entered values the picker never produces).
func accentSlotCSS(varName, base string) string {
	if base == "" {
		return ""
	}
	r, g, b, ok := parseHex(base)
	if !ok {
		return fmt.Sprintf(":root{%s:%s;}", varName, base)
	}
	// Hover: darken by ~12%
	hr, hg, hb := clampByte(int(float64(r)*0.88)), clampByte(int(float64(g)*0.88)), clampByte(int(float64(b)*0.88))
	// Light: blend toward white by ~60%
	lr, lg, lb := clampByte(int(float64(r)+float64(255-r)*0.6)), clampByte(int(float64(g)+float64(255-g)*0.6)), clampByte(int(float64(b)+float64(255-b)*0.6))
	return fmt.Sprintf(
		":root{%[1]s:%[2]s;%[1]s-hover:#%02[3]x%02[4]x%02[5]x;%[1]s-light:#%02[6]x%02[7]x%02[8]x;"+
			"%[1]s-rgb:%[9]d %[10]d %[11]d;%[1]s-hover-rgb:%[3]d %[4]d %[5]d;%[1]s-light-rgb:%[6]d %[7]d %[8]d;}",
		varName, base, hr, hg, hb, lr, lg, lb, r, g, b,
	)
}

// parseHex parses a #RRGGBB hex color into RGB components.
func parseHex(hex string) (r, g, b uint8, ok bool) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	rv, err1 := strconv.ParseUint(hex[0:2], 16, 8)
	gv, err2 := strconv.ParseUint(hex[2:4], 16, 8)
	bv, err3 := strconv.ParseUint(hex[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return uint8(rv), uint8(gv), uint8(bv), true
}

// clampByte clamps an int to the 0-255 range.
func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// SetFontFamily stores the campaign's custom font family in the context.
func SetFontFamily(ctx context.Context, family string) context.Context {
	return context.WithValue(ctx, keyFontFamily, family)
}

// GetFontFamily returns the campaign's custom font family, or empty string for default.
func GetFontFamily(ctx context.Context) string {
	family, _ := ctx.Value(keyFontFamily).(string)
	return family
}

// fontFamilyCSS maps font family setting values to CSS font-family declarations.
var fontFamilyCSSMap = map[string]string{
	"serif":        "Georgia, 'Times New Roman', serif",
	"sans-serif":   "'Inter', system-ui, sans-serif",
	"monospace":    "'JetBrains Mono', 'Fira Code', monospace",
	"georgia":      "Georgia, Cambria, serif",
	"merriweather": "'Merriweather', Georgia, serif",
}

// FontFamilyCSS returns a CSS block that overrides the body font family.
// Returns empty string if no custom font is set.
func FontFamilyCSS(ctx context.Context) string {
	family := GetFontFamily(ctx)
	if family == "" {
		return ""
	}
	css, ok := fontFamilyCSSMap[family]
	if !ok {
		return ""
	}
	return fmt.Sprintf(":root{--font-campaign:%s;}", css)
}

// SetBrandName stores the campaign's custom brand name in the context.
func SetBrandName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, keyBrandName, name)
}

// GetBrandName returns the campaign's custom brand name, or empty string for default.
func GetBrandName(ctx context.Context) string {
	name, _ := ctx.Value(keyBrandName).(string)
	return name
}

// GetDisplayName returns the name to show for the current campaign in chrome
// (topbar + sidebar): the custom brand name when the owner has set one, else
// the campaign's real name. This is the single source of truth for the
// brand-name-override fallback — before C-CUSTOMIZE-RESCUE the sidebar honored
// the brand name but the two topbar sites read GetCampaignName directly, so a
// custom brand appeared in the sidebar and silently reverted to the campaign
// name in the header (the operator-reported "header customization is broken",
// audit §8.1). Callers already inside an in-campaign guard can use this
// directly; GetBrandName is empty whenever no brand is set (or off-campaign),
// so the fallback is always correct.
func GetDisplayName(ctx context.Context) string {
	if name := GetBrandName(ctx); name != "" {
		return name
	}
	return GetCampaignName(ctx)
}

// SetBrandLogo stores the campaign's brand logo media path in the context.
func SetBrandLogo(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, keyBrandLogo, path)
}

// GetBrandLogo returns the campaign's brand logo path, or empty string if none.
func GetBrandLogo(ctx context.Context) string {
	path, _ := ctx.Value(keyBrandLogo).(string)
	return path
}

// TopbarStyleData holds topbar visual customization for template rendering.
// Defined here to avoid importing the campaigns package.
type TopbarStyleData struct {
	Mode         string
	Color        string
	GradientFrom string
	GradientTo   string
	GradientDir  string
	ImagePath    string
}

// SetTopbarStyle stores the campaign's topbar style in the context.
func SetTopbarStyle(ctx context.Context, style *TopbarStyleData) context.Context {
	return context.WithValue(ctx, keyTopbarStyle, style)
}

// GetTopbarStyle returns the campaign's topbar style, or nil for default.
func GetTopbarStyle(ctx context.Context) *TopbarStyleData {
	style, _ := ctx.Value(keyTopbarStyle).(*TopbarStyleData)
	return style
}

// TopbarContentData holds the topbar center area content for templates.
type TopbarContentData struct {
	Mode  string
	Quote string
	Links []TopbarLinkData
}

// TopbarLinkData holds a single quick-link button for the topbar.
type TopbarLinkData struct {
	Label string
	URL   string
	Icon  string
}

// SetTopbarContent stores the campaign's topbar content configuration in the context.
func SetTopbarContent(ctx context.Context, content *TopbarContentData) context.Context {
	return context.WithValue(ctx, keyTopbarContent, content)
}

// GetTopbarContent returns the campaign's topbar content, or nil for empty.
func GetTopbarContent(ctx context.Context) *TopbarContentData {
	content, _ := ctx.Value(keyTopbarContent).(*TopbarContentData)
	return content
}

// SetDegradedPluginCount stores the number of unhealthy plugins in the context.
// Used by the admin sidebar to show a warning badge on the Database link.
func SetDegradedPluginCount(ctx context.Context, count int) context.Context {
	return context.WithValue(ctx, keyDegradedPluginCount, count)
}

// GetDegradedPluginCount returns the number of unhealthy plugins, or 0.
func GetDegradedPluginCount(ctx context.Context) int {
	count, _ := ctx.Value(keyDegradedPluginCount).(int)
	return count
}

// EscapeJSONString escapes a string for safe embedding inside a JSON
// double-quoted value. Only handles the characters that could break
// the JSON structure (backslash and double-quote).
func EscapeJSONString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
