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
// request-scoped state (csrf token, fetched members, the operator's
// role, etc).
//
// Per-campaign feature enable/disable lives on the top-level Extensions
// hub at `/campaigns/:id/extensions`, not on a Settings tab; SortOrder
// 20 is intentionally left vacant for any future plugin tab that wants
// to land between General (10) and People (30).
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
			Content:   settingsGeneralTab(cc, csrfToken, systemOptionsJSON(systemOptions)),
		},
		// Slot 20 (Features) retired; per-campaign feature toggles
		// moved to the top-level Extensions hub.
		{
			ID:        "people",
			Label:     "People",
			Icon:      "fa-solid fa-users",
			MinRole:   RolePlayer,
			SortOrder: 30,
			Content:   settingsPeopleLink(cc),
		},
		{
			ID:        "integrations",
			Label:     "Integrations",
			Icon:      "fa-solid fa-plug",
			MinRole:   RolePlayer,
			SortOrder: 40,
			Content:   settingsIntegrationsTab(cc, csrfToken, h.baseURL),
		},
		// SortOrder slot 50 is intentionally left empty — the AI
		// Workspace plugin registers its tab at slot 55 via
		// campaigns.RegisterSettingsTab; that tab's renderer + content
		// live in internal/plugins/ai_workspace/.
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
	for _, factory := range h.extraSettingsTabs {
		all = append(all, factory(cc))
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].SortOrder < all[j].SortOrder
	})

	role := cc.MemberRole
	out := all[:0]
	for _, t := range all {
		if role < t.MinRole {
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
