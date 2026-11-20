package campaigns

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// deskFindRoot names the page every entry here lives under, as the path shown
// in a result ("Campaign settings › People").
const deskFindRoot = "Campaign settings"

// deskFindWords are the other names someone might type for a tool, matched
// without being shown. Keyed by the tool's pin key so the list and the
// sidebar's pins can never name different tools.
var deskFindWords = map[string]string{
	"manage:members":        "players invite invitation role scribe co-dm gm who people members guests",
	"manage:player-changes": "undo history edits players changed review player edits",
	"manage:media":          "pictures images files gallery uploads look and game",
	"manage:customize":      "look theme colour color font layout home categories style look and game",
	"manage:extensions":     "features add-ons plugins system game rules turn on off look and game",
	"manage:foundry":        "sync module connection vtt",
	"manage:ai":             "claude assistant keys connect requests",
	"manage:trash":          "deleted restore bring back recover safety net",
}

// deskFindGroups is the page's group each tool's row sits in, so a result
// says where to look on the page.
var deskFindGroups = map[string]string{
	"manage:members":        "People",
	"manage:player-changes": "People",
	"manage:media":          "Data",
	"manage:customize":      "Customize",
	"manage:extensions":     "Game",
	"manage:foundry":        "Connections",
	"manage:ai":             "Connections",
	"manage:trash":          "Data",
}

// deskFindPath is a result's path for a group ("" for a page with no group).
func deskFindPath(group string) string {
	if group == "" {
		return deskFindRoot
	}
	return deskFindRoot + " › " + group
}

// deskFindInput is what the index is built from. Everything in it was read for
// this owner and this campaign, so the builder only arranges it and cannot
// list a tool, an area or a tab the campaign does not have.
type deskFindInput struct {
	Base           string
	Media, Foundry bool
	// Archived swaps "Archive" for "Unarchive", the row the page shows.
	Archived     bool
	Customize    []customizeFindEntry
	SettingsTabs []SettingsTab
}

// deskFindRowTabs are the Details and data tabs the page has a row for, which
// the index would otherwise offer twice.
var deskFindRowTabs = map[string]bool{settingsTabAPIKeys: true, settingsTabData: true, settingsTabActivity: true, settingsTabImport: true}

// buildDeskFind lists what Find can open: every tool the campaign has, the
// settings the page itself holds, the Customizer's own settings and each
// Details and data tab. A tool whose add-on is off is not a tool here, so
// neither it nor anything inside it is offered.
func buildDeskFind(in deskFindInput) []customizeFindEntry {
	page := in.Base + "/dashboard"
	// This page and Details and data lead the list, and list "settings", so
	// typing it finds this place before the Customizer's "<Category> settings".
	out := []customizeFindEntry{
		{Label: "Campaign settings", Path: deskFindRoot, Href: page, Icon: "fa-sliders", Words: "settings rows overview"},
		{Label: "Details and data", Path: deskFindRoot, Href: in.Base + "/settings", Icon: "fa-gear", Words: "settings details description keys export import activity"},
	}
	for _, t := range layouts.ManageToolRows(in.Base, in.Media, in.Foundry) {
		if t.Key == "manage:dashboard" || t.Key == "manage:settings" {
			continue
		}
		out = append(out, customizeFindEntry{Label: t.Label, Path: deskFindPath(deskFindGroups[t.Key]), Href: t.URL, Icon: t.Icon, Words: deskFindWords[t.Key]})
	}
	general := deskFindPath("General")
	access := deskFindPath("Access")
	game := deskFindPath("Game")
	conns := deskFindPath("Connections")
	data := deskFindPath("Data")
	end := deskFindPath("Danger zone")
	// The old group and row names stay as hidden words, so someone who types
	// what the page used to say still finds the row.
	archive := customizeFindEntry{Label: "Archive", Path: end, Href: page + "#line-end", Icon: "fa-box-archive", Words: "archive this campaign finish close read-only end archive or delete"}
	if in.Archived {
		archive.Label, archive.Words = "Unarchive", "unarchive this campaign bring back reopen restore archived archive or delete"
	}
	out = append(out,
		customizeFindEntry{Label: "Private or public", Path: access, Href: page + "#line-public", Icon: "fa-lock", Words: "campaign public private visibility discover members link who can read it what players and guests see"},
		customizeFindEntry{Label: "New pages", Path: access, Href: page + "#line-pages", Icon: "fa-eye", Words: "new pages start visibility hidden shown default secret what players and guests see"},
		customizeFindEntry{Label: "Preview as a player", Path: access, Href: page + "#line-preview", Icon: "fa-user", Words: "player view see as preview home players' dashboard what players and guests see"},
		customizeFindEntry{Label: "Preview as a guest", Path: access, Href: page + "#line-preview", Icon: "fa-user-secret", Words: "guest visitor public view see as preview home guests' dashboard what players and guests see"},
		customizeFindEntry{Label: "Name", Path: general, Href: page + "#line-name", Icon: "fa-pen", Words: "rename called title campaign name look and game"},
		customizeFindEntry{Label: "Description", Path: general, Href: page + "#line-description", Icon: "fa-align-left", Words: "description about summary blurb look and game"},
		customizeFindEntry{Label: "Game system", Path: game, Href: page + "#line-system", Icon: "fa-dice-d20", Words: "system game rules ruleset pick"},
		customizeFindEntry{Label: "Keys", Path: conns, Href: in.Base + "/settings?tab=" + settingsTabAPIKeys, Icon: "fa-key", Words: "api tokens other tools"},
		customizeFindEntry{Label: "Export", Path: data, Href: in.Base + "/settings?tab=" + settingsTabData, Icon: "fa-file-export", Words: "export a copy download backup copy data safety net"},
		customizeFindEntry{Label: "Import", Path: data, Href: in.Base + "/settings?tab=" + settingsTabImport, Icon: "fa-file-import", Words: "upload restore vault safety net"},
		customizeFindEntry{Label: "Activity log", Path: data, Href: in.Base + "/settings?tab=" + settingsTabActivity, Icon: "fa-clock-rotate-left", Words: "history audit who did safety net"},
		archive,
		customizeFindEntry{Label: "Transfer ownership", Path: end, Href: page + "#line-end", Icon: "fa-right-left", Words: "hand over give new owner archive or delete"},
		customizeFindEntry{Label: "Delete", Path: end, Href: page + "#line-end", Icon: "fa-trash", Words: "delete this campaign remove destroy erase archive or delete"},
	)
	for _, e := range in.Customize {
		// Customize's own front page is the tool already listed above.
		if e.Href == in.Base+"/customize" {
			continue
		}
		path := "Customize"
		if e.Path != "" && e.Path != "Customize" {
			path += " › " + e.Path
		}
		e.Path = path
		out = append(out, e)
	}
	for _, t := range in.SettingsTabs {
		if t.Label == "" || t.MinRole > RoleOwner || deskFindRowTabs[t.ID] {
			continue
		}
		icon := strings.TrimSpace(strings.TrimPrefix(t.Icon, "fa-solid "))
		out = append(out, customizeFindEntry{Label: t.Label, Path: "Details and data", Href: in.Base + "/settings?tab=" + t.ID, Icon: icon})
	}
	return out
}

// DeskFind answers Find's list (GET /campaigns/:id/dashboard/find) as
// {"entries": [...]}. It is built on the server for the owner alone, and only
// when Find is opened, so no Manage page pays for it. The browser narrows it
// by what is typed; it never sees a tool the campaign switched off.
func (h *Handler) DeskFind(c *echo.Context) error {
	cc, err := deskOwner(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	in := h.deskInputsFor(ctx, cc.Campaign, deskAddons)
	areas := h.customizeAreas(cc)
	entries := buildDeskFind(deskFindInput{
		Base:         "/campaigns/" + cc.Campaign.ID,
		Media:        in.Addons["media-gallery"],
		Foundry:      in.Addons["sync-api"],
		Archived:     cc.Campaign.IsArchived(),
		Customize:    h.customizeFindEntries(cc, areas, h.CustomizeCategories(ctx, cc.Campaign.ID)),
		SettingsTabs: h.visibleSettingsTabs(cc, nil, nil, "", nil, false),
	})
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"entries": entries})
}
