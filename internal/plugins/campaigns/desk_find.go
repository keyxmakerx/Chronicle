package campaigns

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// deskFindWords are the other names someone might type for a Manage tool,
// matched without being shown. Keyed by the tool's pin key so the list and
// the sidebar's pins can never name different tools.
var deskFindWords = map[string]string{
	"manage:members":        "players invite invitation role scribe co-dm gm who people members",
	"manage:player-changes": "undo history edits players changed review",
	"manage:media":          "pictures images files gallery uploads",
	"manage:customize":      "look theme colour color font layout home categories",
	"manage:extensions":     "features add-ons plugins system game rules turn on off",
	"manage:foundry":        "sync module connection vtt",
	"manage:ai":             "claude assistant keys connect requests",
	"manage:trash":          "deleted restore bring back recover",
	"manage:settings":       "export import activity log archive delete transfer api keys data",
}

// deskFindInput is what the index is built from. Everything in it was read for
// this owner and this campaign, so the builder only arranges it and cannot
// list a tool, an area or a tab the campaign does not have.
type deskFindInput struct {
	Base           string
	Media, Foundry bool
	Customize      []customizeFindEntry
	SettingsTabs   []SettingsTab
}

// buildDeskFind lists what the desk's Find can open: every Manage tool the
// campaign has, the settings the desk itself holds, the Customizer's own
// settings and each Settings tab. A tool whose add-on is off is not a tool
// here, so neither it nor anything inside it is offered.
func buildDeskFind(in deskFindInput) []customizeFindEntry {
	var out []customizeFindEntry
	for _, t := range layouts.ManageToolRows(in.Base, in.Media, in.Foundry) {
		if t.Key == "manage:dashboard" {
			continue
		}
		out = append(out, customizeFindEntry{Label: t.Label, Path: "Manage", Href: t.URL, Icon: t.Icon, Words: deskFindWords[t.Key]})
	}
	desk := in.Base + "/dashboard"
	out = append(out,
		customizeFindEntry{Label: "Campaign name", Path: "Manage › The campaign", Href: desk + "#line-name", Icon: "fa-pen", Words: "rename called title"},
		customizeFindEntry{Label: "Who sees new pages", Path: "Manage › The campaign", Href: desk + "#line-pages", Icon: "fa-eye", Words: "visibility hidden shown default new pages"},
		customizeFindEntry{Label: "Public or private", Path: "Manage › The campaign", Href: desk + "#line-pages", Icon: "fa-lock", Words: "visibility discover members read"},
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
		if t.Label == "" || t.MinRole > RoleOwner {
			continue
		}
		icon := strings.TrimSpace(strings.TrimPrefix(t.Icon, "fa-solid "))
		out = append(out, customizeFindEntry{Label: t.Label, Path: "Settings", Href: in.Base + "/settings?tab=" + t.ID, Icon: icon})
	}
	return out
}

// DeskFind answers the desk's Find list (GET /campaigns/:id/dashboard/find) as
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
		Customize:    h.customizeFindEntries(cc, areas, h.CustomizeCategories(ctx, cc.Campaign.ID)),
		SettingsTabs: h.visibleSettingsTabs(cc, nil, nil, "", nil, false),
	})
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"entries": entries})
}
