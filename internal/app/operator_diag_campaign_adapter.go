package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// operator_diag_campaign_adapter.go injects the PER-CAMPAIGN read window into
// the operator diagnostics: the checks that answer "why does MY campaign look
// like this?" (`campaign.surfaces`, `campaign.config`).
//
// It lives here, not in internal/systems, because that package must not
// import the campaigns / addons / entities plugins; the app layer implements
// the interface and wires it at startup.
//
// READ-ONLY BY CONSTRUCTION: every call below is a Get/List/Is, reachable only
// from the admin-gated diagnostics route.
//
// EVERY READ DEGRADES INDIVIDUALLY: a failure lands in a Note the renderer
// prints, never in a zero value, so "no blocks" and "nobody could read the
// layout" never render the same.
type campaignDiagAdapter struct {
	campaigns campaigns.CampaignService
	addons    addons.AddonService
	entities  entities.EntityService

	// routes returns the LIVE Echo route table, supplied as a closure so this
	// file never imports Echo (CLAUDE.md: no Echo types outside handler files).
	// nil when the router could not be handed over, which the renderer reports
	// rather than printing an empty table.
	routes func() []systems.RouteFact

	// sidebarCalendarPath is the path the sidebar's Calendar item links to,
	// read from the SAME map the sidebar renders from rather than re-typed.
	sidebarCalendarPath string
}

// calendarAddonSlug is the addon the campaign-page calendar routes used to
// gate on. Kept as an alias of the plugin's own identifier, never a re-typed
// literal — tools/check-plugin-isolation.sh (T-B2) enforces this. Still read
// by SurfaceFacts below to report the addon's state; it is NOT load-bearing
// for the three campaign.surfaces PAGE routes (see the note SurfaceFacts
// writes — no calendar page UI exists yet, calendar-v5 seams, #778), but it
// IS load-bearing for the calendar plugin's own JSON API and its own
// diagnostic (see calendar.stats, internal/systems/operator_diag_calendar.go).
const calendarAddonSlug = calendar.PluginSlug

// ── campaign.surfaces ───────────────────────────────────────────────────────

func (a campaignDiagAdapter) SurfaceFacts(ctx context.Context, campaignID string) (systems.CampaignSurfaceFacts, error) {
	out := systems.CampaignSurfaceFacts{CampaignID: campaignID, SidebarCalendarPath: a.sidebarCalendarPath}
	camp, err := a.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		if apperror.SafeCode(err) == 404 {
			return out, nil
		}
		return out, err
	}
	out.Found = true
	out.CampaignName = camp.Name

	if a.routes == nil {
		out.RoutesNote = "**The live route table was not handed to this adapter**, so nothing below is checked against the running binary — the surface column is a declaration only."
	} else {
		out.Routes = a.routes()
		if len(out.Routes) == 0 {
			out.RoutesNote = "**The live route table came back EMPTY**, which cannot be true of a running server. Treat the surface column as unverified."
		}
	}

	if a.addons != nil {
		if enabled, aerr := a.addons.IsEnabledForCampaign(ctx, campaignID, calendarAddonSlug); aerr != nil {
			out.AddonNote = aerr.Error()
		} else {
			out.CalendarAddonEnabled = &enabled
		}
	} else {
		out.AddonNote = "the addons service is not wired into this adapter"
	}

	// The sidebar. An EMPTY Items array is valid and means "render the default
	// sidebar" — it is not a missing configuration, and saying so here stops the
	// renderer reading an absence as a finding.
	cfg := camp.ParseSidebarConfig()
	if len(cfg.Items) == 0 {
		out.SidebarNote = "`campaigns.sidebar_config` has no items, which is valid: the default sidebar is synthesized at render time."
	}
	for _, it := range cfg.Items {
		out.SidebarItems = append(out.SidebarItems, systems.SidebarItemFact{
			Type: it.Type, Slug: it.Slug, Label: it.Label, URL: it.URL, Visible: it.Visible,
		})
	}
	return out, nil
}

// ── campaign.config ─────────────────────────────────────────────────────────

// ConfigFacts reads the enabled addons and every placed block type.
func (a campaignDiagAdapter) ConfigFacts(ctx context.Context, campaignID string) (systems.CampaignConfigFacts, error) {
	out := systems.CampaignConfigFacts{CampaignID: campaignID}
	camp, err := a.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		if apperror.SafeCode(err) == 404 {
			return out, nil
		}
		return out, err
	}
	out.Found = true
	out.CampaignName = camp.Name
	out.DefaultDashboardBlocks = dashboardBlockTypes(campaigns.DefaultDashboardLayout())

	a.fillAddonRows(ctx, campaignID, &out)
	out.Layouts = append(out.Layouts,
		campaignLayoutFact("`campaigns.dashboard_layout` (the member-facing campaign page)", camp.DashboardLayout, camp.ParseDashboardLayout()),
		campaignLayoutFact("`campaigns.owner_dashboard_layout` (the owner's campaign page)", camp.OwnerDashboardLayout, camp.ParseOwnerDashboardLayout()),
	)
	a.fillEntityTypeLayouts(ctx, campaignID, &out)
	return out, nil
}

// fillAddonRows lists `campaign_addons` joined to `addons`.
func (a campaignDiagAdapter) fillAddonRows(ctx context.Context, campaignID string, out *systems.CampaignConfigFacts) {
	if a.addons == nil {
		out.AddonsNote = "the addons service is not wired into this adapter"
		return
	}
	rows, err := a.addons.ListForCampaign(ctx, campaignID)
	if err != nil {
		out.AddonsNote = "the addon list failed: " + err.Error()
		return
	}
	for _, r := range rows {
		out.Addons = append(out.Addons, systems.AddonFact{
			Slug:      r.AddonSlug,
			Name:      r.AddonName,
			Status:    string(r.AddonStatus),
			Enabled:   r.Enabled,
			Installed: r.Installed,
		})
	}
	sort.Slice(out.Addons, func(i, j int) bool { return out.Addons[i].Slug < out.Addons[j].Slug })
}

// fillEntityTypeLayouts records the blocks on each entity type's PAGE TEMPLATE
// and on its category dashboard, since both are places an operator can
// hand-place a block and neither is visible from the campaign columns alone.
func (a campaignDiagAdapter) fillEntityTypeLayouts(ctx context.Context, campaignID string, out *systems.CampaignConfigFacts) {
	if a.entities == nil {
		out.LayoutsNote = "the entities service is not wired into this adapter, so entity templates were NOT read"
		return
	}
	types, err := a.entities.GetEntityTypes(ctx, campaignID)
	if err != nil {
		out.LayoutsNote = "the entity-type list failed (" + err.Error() + "), so entity templates were NOT read"
		return
	}
	for i := range types {
		et := &types[i]
		if blocks := templateBlockTypes(et.Layout.Rows); len(blocks) > 0 {
			out.Layouts = append(out.Layouts, systems.LayoutFact{
				Surface: fmt.Sprintf("entity type **%s** — page template (`entity_types.layout_json`)", et.Name),
				Stored:  true,
				Blocks:  blocks,
			})
		}
		if et.DashboardLayout != nil && *et.DashboardLayout != "" {
			out.Layouts = append(out.Layouts, campaignLayoutFact(
				fmt.Sprintf("entity type **%s** — category dashboard (`entity_types.dashboard_layout`)", et.Name),
				et.DashboardLayout, et.ParseCategoryDashboardLayout()))
		}
	}
}

// ── flatteners ──────────────────────────────────────────────────────────────

// campaignLayoutFact turns a nullable JSON column plus its parsed value into one
// reportable row. The RAW column is checked separately from the parse so a
// column that is present but unparseable reads as a parse failure rather than as
// "not customised" — those two send a reader in opposite directions.
func campaignLayoutFact(surface string, raw *string, parsed *campaigns.DashboardLayout) systems.LayoutFact {
	f := systems.LayoutFact{Surface: surface}
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return f
	}
	f.Stored = true
	if parsed == nil {
		f.ParseErr = "the column holds JSON this build could not read as a layout"
		return f
	}
	f.Blocks = dashboardBlockTypes(parsed)
	return f
}

// dashboardBlockTypes flattens a dashboard layout to its block types, in
// placement order, keeping duplicates (two skybox blocks is a different finding
// from one).
func dashboardBlockTypes(l *campaigns.DashboardLayout) []string {
	if l == nil {
		return nil
	}
	var out []string
	for _, row := range l.Rows {
		for _, col := range row.Columns {
			for _, blk := range col.Blocks {
				out = append(out, blk.Type)
			}
		}
	}
	return out
}

// templateBlockTypes flattens an entity page template to its block types,
// descending into container blocks (`two_column`, `tabs`, `section`, …) whose
// children live in the untyped Config map rather than a typed field.
func templateBlockTypes(rows []entities.TemplateRow) []string {
	var out []string
	for _, row := range rows {
		for _, col := range row.Columns {
			for _, blk := range col.Blocks {
				out = append(out, blk.Type)
				out = append(out, nestedBlockTypes(blk.Config)...)
			}
		}
	}
	return out
}

// nestedBlockTypes walks a container block's untyped Config for child blocks.
// The editor stores them under a handful of keys and as JSON-decoded
// []any/map[string]any, so this walks the value generically and collects every
// `"type"` string it finds beneath a `blocks`-ish key.
func nestedBlockTypes(cfg map[string]any) []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			if s, ok := t["type"].(string); ok && s != "" {
				out = append(out, s)
			}
			for _, e := range t {
				walk(e)
			}
		}
	}
	for _, v := range cfg {
		walk(v)
	}
	return out
}
