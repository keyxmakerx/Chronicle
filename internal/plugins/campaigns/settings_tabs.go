// settings_tabs.go — Declarative tab registry for the Campaign Settings
// page. Built-in tabs are constructed inside Settings handlers with
// their per-tab dependencies captured in closures; plugins can
// contribute additional tabs via RegisterSettingsTab without forking
// settings.templ.

package campaigns

import (
	"sort"

	"github.com/a-h/templ"
)

// SettingsTab is the declarative description of one tab on the campaign
// Settings page.
//
//   - ID: stable slug used in the URL (`?tab=<id>`) and the Alpine.js
//     x-show predicate; must stay URL-safe and stable (operator bookmarks
//     rely on it).
//   - Label/Icon: button text / FontAwesome class.
//   - MinRole gates both the button and the content; a viewer below
//     MinRole sees neither. Built-ins use RolePlayer; plugins set
//     whatever discipline they need.
//   - SortOrder controls render order (built-ins use multiples of 10) so
//     plugins can insert between them.
//   - Content is the rendered tab body; the handler captures per-tab
//     dependencies in this closure at Settings-handler time.
type SettingsTab struct {
	ID        string
	Label     string
	Icon      string
	MinRole   Role
	SortOrder int
	Content   templ.Component
}

// RegisterSettingsTab appends a tab-factory to the Handler's plugin-
// contributed registry. Called by other plugins at startup (after the
// campaigns handler is constructed and after their own services are
// wired). The factory is invoked per-request inside visibleSettingsTabs
// with the current CampaignContext so the plugin's tab Content closure
// can capture per-request state (campaign ID, csrf token via context,
// member role for finer-grained rendering, etc).
//
// The factory shape (rather than a static SettingsTab) lets a plugin's
// Content closure capture per-request state; built-in tabs don't go
// through this path, they're constructed inline in the Settings handler.
//
// Tabs added here merge with the built-ins at render time; sorting is
// stable per SortOrder + insertion order so a plugin contributing two
// tabs with the same SortOrder preserves call order.
func (h *Handler) RegisterSettingsTab(factory func(*CampaignContext) SettingsTab) {
	h.extraSettingsTabs = append(h.extraSettingsTabs, factory)
}

// builtInSettingsTabs returns the canonical-order pre-plugin tabs.
// Constructed per-request because each tab's content closure captures
// request-scoped state (csrf token, the operator's role, etc).
//
// People and Foundry are Manage pages of their own, not Settings tabs;
// old ?tab=people and ?tab=integrations links are redirected by the
// Settings handler. The Data tab is replaced by the AI plugin's "Data &
// AI" tab when that plugin is wired (see visibleSettingsTabs).
func (h *Handler) builtInSettingsTabs(
	cc *CampaignContext,
	transfer *OwnershipTransfer,
	members []CampaignMember,
	csrfToken string,
	systemOptions []SystemOption,
	smtpConfigured bool,
) []SettingsTab {
	return []SettingsTab{
		{
			ID:        "general",
			Label:     "General",
			Icon:      "fa-solid fa-gear",
			MinRole:   RolePlayer,
			SortOrder: 10,
			Content:   settingsGeneralTab(cc, csrfToken),
		},
		{
			ID:        settingsTabAPIKeys,
			Label:     "API keys",
			Icon:      "fa-solid fa-key",
			MinRole:   RolePlayer,
			SortOrder: 40,
			Content:   settingsAPIKeysTab(cc, h.baseURL),
		},
		{
			ID:        settingsTabData,
			Label:     "Data",
			Icon:      "fa-solid fa-database",
			MinRole:   RolePlayer,
			SortOrder: 50,
			Content:   SettingsDataSection(cc),
		},
		{
			ID:        "activity",
			Label:     "Activity",
			Icon:      "fa-solid fa-clock-rotate-left",
			MinRole:   RolePlayer,
			SortOrder: 60,
			Content:   settingsActivityTab(cc),
		},
	}
}

// Settings tab IDs that other code points at.
const (
	settingsTabAPIKeys = "api-keys"
	settingsTabData    = "data"
	// settingsTabAI is the AI plugin's "Data & AI" tab, which stands in for
	// the built-in Data tab when it is registered.
	settingsTabAI = "ai-workspace"
)

// visibleSettingsTabs returns the merged + role-filtered + sorted tab
// list for a viewer with the given role. Built-ins + RegisterSettingsTab
// additions are sorted together by SortOrder (stable); rows whose
// MinRole exceeds the viewer's role are dropped.
//
// Stable sort preserves insertion order when SortOrders tie — useful
// for plugins that register multiple tabs at the same priority.
func (h *Handler) visibleSettingsTabs(
	cc *CampaignContext,
	transfer *OwnershipTransfer,
	members []CampaignMember,
	csrfToken string,
	systemOptions []SystemOption,
	smtpConfigured bool,
) []SettingsTab {
	built := h.builtInSettingsTabs(cc, transfer, members, csrfToken, systemOptions, smtpConfigured)
	all := make([]SettingsTab, 0, len(built)+len(h.extraSettingsTabs))
	all = append(all, built...)
	hasAI := false
	for _, factory := range h.extraSettingsTabs {
		t := factory(cc)
		hasAI = hasAI || t.ID == settingsTabAI
		all = append(all, t)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].SortOrder < all[j].SortOrder
	})

	role := cc.MemberRole
	out := all[:0]
	for _, t := range all {
		if role < t.MinRole || (hasAI && t.ID == settingsTabData) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// sanitizeSettingsTab resolves the `?tab=` query param to a known-safe
// value: the requested ID only if it matches a tab visible to the current
// viewer, else falls back to "general".
//
// SECURITY: CampaignSettingsPage interpolates this into an Alpine.js
// x-data expression, and since the browser HTML-decodes the attribute
// before Alpine evaluates it as JS, an unvalidated value is reflected XSS.
// Constraining the result to a developer-defined ID from `tabs` closes
// this at the source (the templ sink also escapes, defense in depth).
//
// Matching against the role-filtered `tabs` (not a static allowlist) also
// stops a viewer selecting a tab their role hides. "general" is always
// present (RolePlayer-visible, Settings is owner-gated).
func sanitizeSettingsTab(requested string, tabs []SettingsTab) string {
	// The Data tab and the AI plugin's Data & AI tab are one place under
	// two IDs; whichever is shown answers both.
	switch requested {
	case settingsTabData:
		requested = settingsTabAI
		if hasSettingsTabID(tabs, settingsTabData) {
			requested = settingsTabData
		}
	case settingsTabAI:
		if !hasSettingsTabID(tabs, settingsTabAI) {
			requested = settingsTabData
		}
	}
	for _, t := range tabs {
		if t.ID == requested {
			// Return the registry's constant, never the raw request
			// string. They are equal on a match, but returning t.ID makes
			// the trusted provenance explicit at the call site.
			return t.ID
		}
	}
	return "general"
}

// hasSettingsTabID reports whether tabs includes one with the given ID.
func hasSettingsTabID(tabs []SettingsTab, id string) bool {
	for _, t := range tabs {
		if t.ID == id {
			return true
		}
	}
	return false
}

// settingsTabRedirect is where an old Settings tab now lives, or "" when the
// tab is still on the Settings page. People and Foundry became Manage pages.
func settingsTabRedirect(campaignID, tab string) string {
	switch tab {
	case "people":
		return "/campaigns/" + campaignID + "/members"
	case "integrations":
		return "/campaigns/" + campaignID + "/foundry"
	}
	return ""
}
