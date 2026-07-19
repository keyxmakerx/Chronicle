// settings_tabs.go — Declarative tab registry for the Campaign Settings
// page. Per reports/chronicle/2026-05-26-c-ai-workspace-scoping.md §1.1
// and §4 Phase 1. Built-in tabs are constructed inside Settings handlers
// with their per-tab dependencies captured in closures; plugins can
// contribute additional tabs via RegisterSettingsTab without forking
// settings.templ.
//
// The registry preserves existing UX byte-for-byte: same six tabs, same
// icons, same labels, same role gates. The refactor's value is for
// future plugins (AI Workspace V1 Phase 2) — no functional change to
// the operator-visible Settings page.

package campaigns

import (
	"sort"

	"github.com/a-h/templ"
)

// SettingsTab is the declarative description of one tab on the
// campaign Settings page.
//
//   - ID is the stable slug used in the URL query (`?tab=<id>`) and
//     the Alpine.js `x-show="tab === '<id>'"` predicate. Must be
//     URL-safe + stable across releases (operator bookmarks rely on it).
//   - Label is the human-readable button text.
//   - Icon is a FontAwesome class (e.g. "fa-solid fa-gear").
//   - MinRole gates both the button and the content. A viewer whose
//     role is below MinRole sees neither. Today's gates: General /
//     People / Integrations / Activity = RolePlayer (any member);
//     Features / AI Export = RoleOwner. New plugin-contributed tabs
//     set MinRole to whatever discipline they need; the campaigns
//     plugin is the single enforcement point.
//   - SortOrder controls render order across the tab bar AND the
//     content blocks. Lower renders first. Built-ins use multiples of
//     10 (10..60) so plugins can insert between them; the AI Workspace
//     plugin (Phase 2) will register itself at SortOrder 55 to land
//     between AI Export (50) and Activity (60).
//   - Content is the rendered tab body. The handler captures all
//     per-tab dependencies (csrfToken, members, addons, services, ...)
//     in this closure at Settings-handler time, so CampaignSettingsPage
//     itself stays signature-thin.
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
// The factory shape (rather than a static SettingsTab) is the
// difference between Phase 1's API sketch and the working shape — the
// AI Workspace plugin (Phase 2) is the first caller and needs the per-
// request binding. Built-in tabs don't go through this path; they're
// constructed inline in the Settings handler.
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
// The data parameters mirror what the Settings handler already loads
// today; the only structural change vs the pre-refactor templ is that
// the closures live here instead of being inlined inside settings.templ.
//
// C-EXT-HUB Phase 1 (2026-05-29) removed the "features" tab (slot 20).
// Per-campaign feature enable/disable lives on the new top-level
// Extensions hub at `/campaigns/:id/extensions`. The addons store and
// `settingsFeaturesTab` templ component are otherwise unchanged —
// removing the tab here is the entire operator-visible delta for the
// settings page. SortOrder 20 is intentionally left vacant for any
// future plugin tab that wants to land between General (10) and
// People (30).
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
		// Slot 20 (Features) retired by C-EXT-HUB Phase 1; per-
		// campaign feature toggles moved to the top-level Extensions
		// hub.
		{
			ID:        "people",
			Label:     "People",
			Icon:      "fa-solid fa-users",
			MinRole:   RolePlayer,
			SortOrder: 30,
			Content:   settingsPeopleTab(cc, members, transfer, csrfToken, smtpConfigured),
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
		// Workspace plugin (NW-2.2+ ai_workspace) registers its tab at
		// slot 55 via campaigns.RegisterSettingsTab. The campaigns-side
		// AI Export tab was retired in C-AI-WORKSPACE-V1-B; the
		// renderer + tab content now live in
		// internal/plugins/ai_workspace/.
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

// sanitizeSettingsTab resolves a requested settings-tab ID (from the
// `?tab=` query param) to a known-safe value. It returns the requested
// ID only when it matches a tab actually visible to the current viewer;
// any empty, unknown, or hostile value falls back to "general".
//
// This is a security boundary, not merely UX. CampaignSettingsPage
// interpolates the returned value into an Alpine.js `x-data` expression
// (settings.templ), and because the browser HTML-decodes an attribute
// before Alpine evaluates it as JavaScript, an unvalidated request value
// is a reflected-XSS vector (audit SEC-1; cordinator core-tenets §T-B1).
// Constraining the result to a developer-defined tab ID from `tabs`
// closes that vector at the source; the templ sink additionally escapes
// it (defense in depth).
//
// Matching against the already-role-filtered `tabs` slice (rather than a
// static allowlist) also means a viewer cannot pre-select a tab their
// role hides, and the fallback fixes the blank-tab-body symptom an
// unknown tab produced (no `x-show` predicate matched). "general" is a
// safe fallback: MinRole RolePlayer makes it visible to every role, and
// Settings is owner-gated, so it is always present in `tabs`.
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
