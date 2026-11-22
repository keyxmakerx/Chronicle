// settings_conn_data.go builds and serves Campaign settings' Connections and
// Data tabs. Connections is every outside thing linked to the campaign (the
// Foundry module, the owner's AI, API keys); Data is the campaign as a whole
// thing (what it stores, copies of it, its Trash and activity log, and the
// Danger zone). Their figures come from the same providers and builders as the
// Overview's tiles.

package campaigns

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// ConnectionsTab is the Connections tab's page.
type ConnectionsTab struct {
	// Foundry is the Foundry link's state; nil while Foundry sync is off,
	// when FoundryOffHref leads to where it is switched on.
	Foundry        *DeskGoTile
	FoundryOffHref string
	AI             DeskGoTile
	// Keys is how many API keys are in use; nil when it could not be read.
	Keys *DeskKeysFacts
	// BaseURL is the server's address, for tools that call the API.
	BaseURL string
}

// connectionsTabOf builds the tab from the facts its providers returned.
func connectionsTabOf(c *Campaign, addons map[string]bool, f DeskFacts, baseURL string, now time.Time) ConnectionsTab {
	t := ConnectionsTab{AI: aiTile(c, f.AI), Keys: f.Keys, BaseURL: baseURL}
	if addons["sync-api"] {
		ft := foundryTile(c, f.Foundry, now)
		t.Foundry = &ft
	} else {
		t.FoundryOffHref = deskHref(c.ID, "/extensions")
	}
	return t
}

// DataTab is the Data tab's page.
type DataTab struct {
	// Storage is what the picture store holds; nil while Media is off.
	Storage *DeskGoTile
	Trash   DeskGoTile
	// Sections are the other plugins' parts of this tab (Import), each with
	// its own heading and anchor, in their registered order.
	Sections []SettingsTab
	// Transfer leads to where ownership moves, on People.
	Transfer string
}

// dataTabOf builds the tab. The Trash and storage figures lead to their own
// pages from here, not back to this tab.
func dataTabOf(c *Campaign, addons map[string]bool, f DeskFacts, sections []SettingsTab) DataTab {
	t := DataTab{Sections: sections, Transfer: deskHref(c.ID, "/members#transfer")}
	t.Trash = trashTile(c, f.Trash)
	t.Trash.Label, t.Trash.Href = "Trash", deskHref(c.ID, "/trash")
	if t.Trash.Value.Href != "" {
		t.Trash.Value.Href = t.Trash.Href
	}
	if addons["media-gallery"] {
		s := storageTile(c, true, f.Media)
		s.Label, s.Href = "Pictures and files", deskHref(c.ID, "/media")
		if s.Value.Href != "" {
			s.Value.Href = s.Href
		}
		s.Hint = mediaHint(f.Media)
		t.Storage = &s
	}
	return t
}

// mediaHint is the line under the storage figure on Data.
func mediaHint(f *DeskMediaFacts) string {
	switch {
	case f == nil:
		return ""
	case f.Files == 0:
		return "Nothing uploaded yet"
	}
	return plural(f.Files, "file", "files")
}

// dataSectionIDs are the registered parts that the Data tab draws itself, so
// they are not drawn twice: the export is the tab's own Copies part, and the
// old Details, Keys and Activity tabs live elsewhere now.
var dataSectionIDs = map[string]bool{"general": true, settingsTabAPIKeys: true, settingsTabData: true, settingsTabActivity: true}

// dataSections are the other plugins' parts the Data tab carries.
func dataSections(tabs []SettingsTab) []SettingsTab {
	var out []SettingsTab
	for _, t := range tabs {
		if !dataSectionIDs[t.ID] {
			out = append(out, t)
		}
	}
	return out
}

// ConnectionsPage serves the Connections tab (GET /campaigns/:id/connections).
// Owner only, like every Campaign settings page.
func (h *Handler) ConnectionsPage(c *echo.Context) error {
	cc, err := deskOwner(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	in := h.deskInputsFor(ctx, cc.Campaign, deskAddons)
	req := DeskRequest{CampaignID: cc.Campaign.ID, OwnerID: auth.GetUserID(c), Now: time.Now()}
	slots := []DeskSlot{DeskAI, DeskKeys}
	if in.Addons["sync-api"] {
		slots = append(slots, DeskFoundry)
	}
	facts := h.desk.Facts(ctx, req, slots...)
	return middleware.Render(c, http.StatusOK, ConnectionsTabPage(cc, connectionsTabOf(cc.Campaign, in.Addons, facts, h.baseURL, req.Now)))
}

// DataPage serves the Data tab (GET /campaigns/:id/data). Owner only.
func (h *Handler) DataPage(c *echo.Context) error {
	cc, err := deskOwner(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	in := h.deskInputsFor(ctx, cc.Campaign, deskAddons)
	req := DeskRequest{CampaignID: cc.Campaign.ID, OwnerID: auth.GetUserID(c), Now: time.Now()}
	slots := []DeskSlot{DeskTrash}
	if in.Addons["media-gallery"] {
		slots = append(slots, DeskMedia)
	}
	facts := h.desk.Facts(ctx, req, slots...)
	csrf := middleware.GetCSRFToken(c)
	sections := dataSections(h.visibleSettingsTabs(cc))
	return middleware.Render(c, http.StatusOK, DataTabPage(cc, dataTabOf(cc.Campaign, in.Addons, facts, sections), csrf))
}
