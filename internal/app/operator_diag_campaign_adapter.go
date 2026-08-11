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
// the operator diagnostics: the four checks that answer "why does MY campaign
// look like this?" (`calendar.render`, `calendar.config`, `campaign.surfaces`,
// `campaign.config`).
//
// It lives here for the same reason entityDiagAdapter does: internal/systems
// must not import the calendar / campaigns / addons / entities plugins, so the
// app layer implements the interface and wires it at startup.
//
// READ-ONLY BY CONSTRUCTION. Every call below is a Get/List/Is. Nothing here
// writes, and nothing here is reachable except from the admin-gated diagnostics
// route.
//
// EVERY READ DEGRADES INDIVIDUALLY. A campaign whose addon table cannot be read
// still reports its calendars; a calendar whose stored moon count cannot be read
// still reports what the Block draws. The failure lands in a Note the renderer
// prints, never in a zero — "no moons" and "nobody could read the moons table"
// must not render the same, and a struct full of zero values is exactly how they
// come to.
type campaignDiagAdapter struct {
	campaigns campaigns.CampaignService
	addons    addons.AddonService
	calendars calendar.CalendarService
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

// calendarAddonSlug is the addon every calendar route gates on
// (addons.RequireAddon(addonSvc, "calendar")). Disabling it makes the entire
// feature answer as if it did not exist.
const calendarAddonSlug = "calendar"

// ── calendar.render + calendar.config ───────────────────────────────────────

// CalendarFacts reads the calendar state behind one campaign's Bench, for one
// viewer (userID may be empty, which traces the owner path).
//
// IT REPRODUCES THE BENCH'S OWN LOADERS RATHER THAN A CONVENIENT SUBSTITUTE:
// the owner/player list split (buildBench keeps ListCalendars and
// ListVisibleCalendars separate to preserve the W5a visibility rule), then the
// spine's batched EagerLoadCalendars — which is what benchHydrate calls, and
// therefore the only loader that applies the real-Moon fallback. A diagnostic
// that hydrated through the 60-method service instead would report zero moons
// on exactly the calendar the operator is asking about.
func (a campaignDiagAdapter) CalendarFacts(ctx context.Context, campaignID, userID string) (systems.CampaignCalendarFacts, error) {
	out := systems.CampaignCalendarFacts{CampaignID: campaignID}
	camp, err := a.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		if apperror.SafeCode(err) == 404 {
			return out, nil // Found stays false — the renderer says "no such campaign"
		}
		return out, err
	}
	out.Found = true
	out.CampaignName = camp.Name

	a.fillCalendarAddon(ctx, campaignID, &out)
	spine := calendar.BlockSpine()
	out.SpineInstalled = spine != nil

	a.fillViewer(ctx, campaignID, userID, &out)
	a.fillCalendarLists(ctx, campaignID, spine, &out)
	a.fillViewerPrefs(ctx, campaignID, userID, &out)

	// The campaign DEFAULT calendar — what GetCalendar resolves, which is what
	// the Foundry sync API and every default-calendar surface are served. It is
	// deliberately a separate read from the list: the default is a property of
	// the row, and a list that happens to be ordered by it is not the same fact.
	if def, derr := a.calendars.GetCalendar(ctx, campaignID); derr != nil {
		out.DefaultNote = derr.Error()
	} else if def != nil {
		out.DefaultID = def.ID
	}
	return out, nil
}

// fillCalendarAddon records the gate that can make every line below moot.
func (a campaignDiagAdapter) fillCalendarAddon(ctx context.Context, campaignID string, out *systems.CampaignCalendarFacts) {
	if a.addons == nil {
		out.AddonNote = "the addons service is not wired into this adapter"
		return
	}
	enabled, err := a.addons.IsEnabledForCampaign(ctx, campaignID, calendarAddonSlug)
	if err != nil {
		out.AddonNote = err.Error()
		return
	}
	out.AddonEnabled = &enabled
}

// fillViewer resolves the two roles the Bench actually uses. They are DIFFERENT
// numbers and the page reads both: RequireRole compares the MEMBERSHIP role,
// while cc.VisibilityRole() promotes a DM-grantee to Owner. Reporting one of
// them would make half the gate traces wrong.
func (a campaignDiagAdapter) fillViewer(ctx context.Context, campaignID, userID string, out *systems.CampaignCalendarFacts) {
	if strings.TrimSpace(userID) == "" {
		out.ListVia = "ListCalendars — the OWNER branch (no viewer supplied)"
		return
	}
	out.Viewer.Supplied = true
	out.Viewer.UserID = userID

	m, err := a.campaigns.GetMember(ctx, campaignID, userID)
	switch {
	case err != nil && apperror.SafeCode(err) == 404:
		out.Viewer.Note = "no `campaign_members` row for this user in this campaign"
	case err != nil:
		out.Viewer.Note = "membership read failed: " + err.Error()
	case m != nil:
		out.Viewer.Found = true
		out.Viewer.MemberRole = int(m.Role)
	}

	if granted, gerr := a.campaigns.IsUserDmGranted(ctx, campaignID, userID); gerr != nil {
		out.Notes = append(out.Notes, "DM-grant read failed ("+gerr.Error()+"), so the visibility role below assumes NO grant")
	} else {
		out.Viewer.DmGranted = granted
	}

	// cc.VisibilityRole(): RoleOwner for a DM-grantee, else the membership role.
	out.Viewer.Role = out.Viewer.MemberRole
	if out.Viewer.DmGranted {
		out.Viewer.Role = int(campaigns.RoleOwner)
	}

	if out.Viewer.MemberRole >= int(campaigns.RoleOwner) {
		out.ListVia = "ListCalendars — the OWNER branch (every calendar, unfiltered)"
	} else {
		out.ListVia = fmt.Sprintf("ListVisibleCalendars(role=%d) — the non-owner branch (per-calendar visibility applied)", out.Viewer.Role)
	}
}

// fillCalendarLists loads both the full campaign list and the viewer's filtered
// one, hydrated through the spine exactly as benchHydrate does.
func (a campaignDiagAdapter) fillCalendarLists(ctx context.Context, campaignID string, spine *calendar.BlockService, out *systems.CampaignCalendarFacts) {
	all, err := a.calendars.ListCalendars(ctx, campaignID)
	if err != nil {
		out.ListErr = err.Error()
		return
	}
	out.All = a.hydrateForDiag(ctx, spine, all, out)

	if !out.Viewer.Supplied {
		return
	}
	// Visible is ALWAYS populated once a viewer was named, including for an
	// owner (whose list is the full one, by the owner branch of buildBench).
	// The renderer reads a nil Visible as "this viewer's list was not
	// resolved" and prints a caveat, so leaving it nil for the owner — where
	// the full list IS the right answer — would raise a warning about a
	// correct result. An empty non-nil slice keeps "the viewer sees nothing"
	// distinguishable from "we did not look".
	if out.Viewer.MemberRole >= int(campaigns.RoleOwner) {
		out.Visible = nonNil(out.All)
		return
	}
	vis, verr := a.calendars.ListVisibleCalendars(ctx, campaignID, out.Viewer.Role, out.Viewer.UserID)
	if verr != nil {
		out.Notes = append(out.Notes, "the viewer's visible-calendar list failed ("+verr.Error()+")")
		return // Visible stays nil — the renderer says so rather than guessing
	}
	out.Visible = nonNil(a.hydrateForDiag(ctx, spine, vis, out))
}

// nonNil guarantees a non-nil slice, so "resolved to nothing" and "not
// resolved" stay distinguishable across the seam.
func nonNil(xs []systems.DiagCalendar) []systems.DiagCalendar {
	if xs == nil {
		return []systems.DiagCalendar{}
	}
	return xs
}

// hydrateForDiag turns the plugin's calendars into the diagnostic's flat rows.
//
// The moon counts are read TWICE ON PURPOSE. MoonsRendered comes from the spine
// (post-fallback: what the Block actually draws) and MoonRowsStored from the
// calendar service's own eager load (pre-fallback: what Settings → Moons can
// edit). Since 2026-08-11 those are different numbers on a real-world calendar,
// and the difference is the answer to "where is my moon" — a single count would
// have to pick one and would mislead whichever way it picked.
func (a campaignDiagAdapter) hydrateForDiag(ctx context.Context, spine *calendar.BlockService, cals []calendar.Calendar, out *systems.CampaignCalendarFacts) []systems.DiagCalendar {
	if len(cals) == 0 {
		return nil
	}
	hydrated := map[string]*calendar.Calendar{}
	if spine != nil {
		ids := make([]string, 0, len(cals))
		for i := range cals {
			ids = append(ids, cals[i].ID)
		}
		full, err := spine.EagerLoadCalendars(ctx, ids)
		if err != nil {
			out.Notes = append(out.Notes, "the Block spine's batched hydration failed ("+err.Error()+"), so the per-calendar counts below are from the shallow list and the RENDERED moon count is unknown")
		} else {
			hydrated = full
		}
	}

	rows := make([]systems.DiagCalendar, 0, len(cals))
	for i := range cals {
		src := &cals[i]
		if h := hydrated[src.ID]; h != nil {
			src = h
		}
		row := systems.DiagCalendar{
			ID:             src.ID,
			Name:           src.Name,
			Mode:           src.Mode,
			IsDefault:      src.IsDefault,
			TracksRealTime: src.TracksRealTime,
			Visibility:     src.Visibility,
			Months:         len(src.Months),
			Weekdays:       len(src.Weekdays),
			Seasons:        len(src.Seasons),
			Eras:           len(src.Eras),
			Cycles:         len(src.Cycles),
			Festivals:      len(src.Festivals),
			MoonsRendered:  len(src.Moons),
		}
		if src.RealTimeZone != nil {
			row.RealTimeZone = *src.RealTimeZone
		}
		for _, m := range src.Moons {
			row.MoonNames = append(row.MoonNames, m.Name)
			// The synthesized real Moon keeps ID 0 precisely so it cannot be
			// mistaken for a row (moon_fallback.go states that invariant); a
			// stored row's id is AUTO_INCREMENT and never 0.
			if m.ID == 0 {
				row.SynthesizedMoon = true
			}
		}
		a.fillStoredMoonCount(ctx, &row)
		rows = append(rows, row)
	}
	return rows
}

// fillStoredMoonCount reads the `calendar_moons` row count through the calendar
// service, which applies no fallback. A failure is recorded, never defaulted to
// zero: "no rows" is the finding this diagnostic exists to report, so a read
// failure that looked like it would be the worst possible lie here.
func (a campaignDiagAdapter) fillStoredMoonCount(ctx context.Context, row *systems.DiagCalendar) {
	stored, err := a.calendars.GetCalendarByID(ctx, row.ID)
	switch {
	case err != nil:
		row.StoredCountNote = "the stored-row read failed: " + err.Error()
	case stored == nil:
		row.StoredCountNote = "the calendar row disappeared between the list and the read"
	default:
		row.MoonRowsStored = len(stored.Moons)
	}
}

// fillViewerPrefs reads the two per-(user, campaign) preference rows that decide
// what the page discloses. BOTH distinguish nil from empty, and both mean
// something different by it, so the nil case is reported as its own state.
func (a campaignDiagAdapter) fillViewerPrefs(ctx context.Context, campaignID, userID string, out *systems.CampaignCalendarFacts) {
	if strings.TrimSpace(userID) == "" {
		return
	}
	if stored, err := a.calendars.GetBenchSections(ctx, userID, campaignID); err != nil {
		out.SectionsNote = "the disclosure-preference read failed (" + err.Error() + "). The page ITSELF degrades to the ruled default when this happens, so the states below are what a viewer would see — but they are not evidence of a stored choice."
		out.SectionsNeverChosen = true
	} else {
		out.SectionsStored = stored
		out.SectionsNeverChosen = stored == nil
	}
	if layers, err := a.calendars.GetBlockLayers(ctx, userID, campaignID); err != nil {
		out.LayersNote = "the layer-preference read failed (" + err.Error() + ")"
		out.LayersNeverChosen = true
	} else {
		out.LayersStored = layers
		out.LayersNeverChosen = layers == nil
	}
	if active, err := a.calendars.GetActiveCalendar(ctx, userID, campaignID); err != nil {
		out.ActiveNote = err.Error()
	} else if active != nil {
		out.ActiveID = active.ID
	}
}

// ── campaign.surfaces ───────────────────────────────────────────────────────

// SurfaceFacts reads the live route table plus the campaign's nav config.
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
// and on its category dashboard. Both are places an operator can hand-place a
// block, and the 2026-08-11 question ("is a skybox block placed anywhere?")
// cannot be answered from the campaign columns alone.
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

// templateBlockTypes flattens an entity page template to its block types.
//
// It descends into container blocks (`two_column`, `tabs`, `section`, …), whose
// children live in the Config map rather than in a typed field. A flattener that
// stopped at the top level would report a campaign as having no skybox block
// while one sat inside a tab — an answer that is confidently wrong, which is the
// single failure mode this whole diagnostic family exists to remove.
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
