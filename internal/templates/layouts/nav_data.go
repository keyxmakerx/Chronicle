package layouts

import (
	"context"
	"encoding/json"
	"regexp"
)

// nav_data.go carries the campaign sidebar's rows into the templates. The
// rows arrive already cut down for the viewer (the campaigns plugin's
// ViewNav, called by the LayoutInjector), so a template only draws what it is
// given and never decides visibility itself.

const keyNavSections ctxKey = "layout_nav_sections"

// NavSectionView is one section of the viewer's sidebar. Kind is "pinned",
// "apps", "categories" or "custom" (a section the owner added).
type NavSectionView struct {
	ID    string
	Kind  string
	Label string
	Rows  []NavRowView
}

// NavRowView is one row of the viewer's sidebar.
type NavRowView struct {
	Key      string // "app:<slug>", "cat:<id>" or "link:<id>"
	Kind     string // "app", "category" or "link"
	Label    string
	Icon     string // a Font Awesome name, or "" for none
	Color    string // a category's colour
	URL      string // absolute path, or a link's target
	Caption  string // a few muted words beside the label
	External bool   // a link leaving Chronicle
	TypeID   int    // a category's entity type
	Count    int
	Subs     []NavSubRowView
	Hidden   bool // hidden from players; only an owner is ever given one
	Personal bool // pinned by this viewer
}

// NavSubRowView is a sub-category row under its parent category.
type NavSubRowView struct {
	Key    string
	TypeID int
	Label  string
	Color  string
	URL    string
	Count  int
}

// SetNavSections stores the viewer's sidebar sections in context.
func SetNavSections(ctx context.Context, sections []NavSectionView) context.Context {
	return context.WithValue(ctx, keyNavSections, sections)
}

// GetNavSections returns the viewer's sidebar sections, or nil outside a
// campaign.
func GetNavSections(ctx context.Context) []NavSectionView {
	sections, _ := ctx.Value(keyNavSections).([]NavSectionView)
	return sections
}

// navIconPattern is the only icon shape the sidebar and the command palette
// accept: entity-type icons are stored as free text, and an icon is spliced
// into a class list (and, in the palette, into markup).
var navIconPattern = regexp.MustCompile(`^fa-[a-z0-9-]{1,40}$`)

// SafeNavIcon returns icon when it is a Font Awesome name, else fallback.
func SafeNavIcon(icon, fallback string) string {
	if navIconPattern.MatchString(icon) {
		return icon
	}
	return fallback
}

// navCommand is one "Go to" entry of the command palette.
type navCommand struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Icon  string `json:"icon,omitempty"`
}

// NavCommandsJSON lists the viewer's sidebar destinations for the command
// palette, as JSON [{label, href, icon}]. It is built from the same rows the
// sidebar draws, so the palette can never offer what the sidebar withholds:
// a row hidden from players, an app the viewer cannot open, or the owner's
// Manage pages for anyone else.
func NavCommandsJSON(ctx context.Context) string {
	if !InCampaign(ctx) {
		return "[]"
	}
	var out []navCommand
	add := func(label, href, icon string) {
		if label == "" || href == "" {
			return
		}
		out = append(out, navCommand{Label: label, Href: href, Icon: SafeNavIcon(icon, "")})
	}
	base := "/campaigns/" + GetCampaignID(ctx)
	add("Dashboard", base, "fa-house")
	for _, sec := range GetNavSections(ctx) {
		for _, row := range sec.Rows {
			add(row.Label, row.URL, row.Icon)
			for _, sub := range row.Subs {
				add(sub.Label, sub.URL, row.Icon)
			}
		}
	}
	add("All Pages", base+"/entities", "fa-layer-group")
	for _, row := range NavManageRows(ctx) {
		add(row.Label, row.URL, row.Icon)
	}
	b, err := json.Marshal(out)
	if err != nil || out == nil {
		return "[]"
	}
	return string(b)
}

// NavManageRows are the campaign-management pages, in the order the sidebar
// lists them. Only the owner manages a campaign, so anyone else (a Scribe, a
// player, an owner viewing as a player whose role reads Player, a visitor)
// gets none. The same rows feed the shared Manage header's tab strip.
func NavManageRows(ctx context.Context) []NavRowView {
	if !InCampaign(ctx) || !IsAuthenticated(ctx) || GetCampaignRole(ctx) < 3 {
		return nil
	}
	base := "/campaigns/" + GetCampaignID(ctx)
	rows := []NavRowView{
		{Key: "manage:dashboard", Label: "Overview", Icon: "fa-gauge", URL: base + "/dashboard"},
		{Key: "manage:members", Label: "People", Icon: "fa-users", URL: base + "/members"},
	}
	if IsAddonEnabled(ctx, "media-gallery") {
		rows = append(rows, NavRowView{Key: "manage:media", Label: "Media", Icon: "fa-photo-film", URL: base + "/media"})
	}
	rows = append(rows,
		NavRowView{Key: "manage:customize", Label: "Customize", Icon: "fa-paintbrush", URL: base + "/customize"},
		NavRowView{Key: "manage:extensions", Label: "Apps & game system", Icon: "fa-puzzle-piece", URL: base + "/extensions"},
	)
	// Sync history only means something once the campaign syncs with Foundry.
	if IsAddonEnabled(ctx, "sync-api") {
		rows = append(rows, NavRowView{Key: "manage:sync-history", Label: "Sync history", Icon: "fa-clock-rotate-left", URL: base + "/sync-history"})
	}
	return append(rows,
		NavRowView{Key: "manage:trash", Label: "Trash", Icon: "fa-trash-can", URL: base + "/trash"},
		NavRowView{Key: "manage:settings", Label: "Settings", Icon: "fa-gear", URL: base + "/settings"},
	)
}
