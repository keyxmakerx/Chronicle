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
	Color    string // a category's saved colour; drawn only when the campaign colours icons by category
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
	Color  string // see NavRowView.Color
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
	// Words are other names a person might type for the place; the palette
	// and quick search match them as well as the label. Omitted when empty.
	Words string `json:"words,omitempty"`
}

// NavCommandsJSON lists the viewer's sidebar destinations for the command
// palette, as JSON [{label, href, icon, words}]. It is built from the same rows the
// sidebar draws, so the palette can never offer what the sidebar withholds:
// a row hidden from players, an app the viewer cannot open, or the owner's
// campaign settings pages for anyone else.
func NavCommandsJSON(ctx context.Context) string {
	if !InCampaign(ctx) {
		return "[]"
	}
	var out []navCommand
	add := func(label, href, icon string) {
		addWords(&out, label, href, icon, "")
	}
	// A tool that people look for under another name carries those names.
	addTool := func(key, label, href, icon string) {
		addWords(&out, label, href, icon, manageToolWords[key])
	}
	base := "/campaigns/" + GetCampaignID(ctx)
	add("Home", base, "fa-house")
	for _, sec := range GetNavSections(ctx) {
		for _, row := range sec.Rows {
			add(row.Label, row.URL, row.Icon)
			for _, sub := range row.Subs {
				add(sub.Label, sub.URL, row.Icon)
			}
		}
	}
	add("All pages", base+"/entities", "fa-layer-group")
	// The sidebar has one Campaign settings row, but the palette still reaches
	// each tool by name. The desk and the settings page are covered by the
	// owner's phrases below, so their plain rows are not listed twice.
	for _, row := range NavManagePages(ctx) {
		if row.Key == "manage:dashboard" || row.Key == "manage:settings" {
			continue
		}
		addTool(row.Key, row.Label, row.URL, row.Icon)
	}
	if len(NavManagePages(ctx)) > 0 {
		for _, c := range ownerCommands(base) {
			addWords(&out, c.Label, c.Href, c.Icon, c.Words)
		}
	}
	b, err := json.Marshal(out)
	if err != nil || out == nil {
		return "[]"
	}
	return string(b)
}

// addWords appends one palette command, dropping an entry with no label or
// target and any icon that is not a Font Awesome name.
func addWords(out *[]navCommand, label, href, icon, words string) {
	if label == "" || href == "" {
		return
	}
	*out = append(*out, navCommand{Label: label, Href: href, Icon: SafeNavIcon(icon, ""), Words: words})
}

// manageToolWords are other names for the owner's tools. Deleted pages wait in
// the Trash, and the page-by-page record of what players changed is the nearest
// thing to a history the owner can open from here; a single page's own History
// and Delete are on that page.
var manageToolWords = map[string]string{
	"manage:trash":          "delete deleted restore undo bin",
	"manage:player-changes": "history changes undo revert",
}

// ownerCommands are the owner's own phrases: what people type when they want
// a campaign setting ("export", "delete", "guest"), each with the words that
// also find it. base is "/campaigns/<id>". The tab ids and the #danger and
// #line-preview anchors are the ones the settings and desk pages answer to.
func ownerCommands(base string) []navCommand {
	return []navCommand{
		{Label: "Campaign settings", Href: base + "/dashboard", Icon: "fa-sliders", Words: "settings manage owner options"},
		{Label: "Preview as a player or guest", Href: base + "/dashboard#line-preview", Icon: "fa-eye", Words: "player guest visitor public dashboard home view as see preview"},
		{Label: "Export a copy", Href: base + "/settings?tab=data", Icon: "fa-file-export", Words: "export backup download copy"},
		{Label: "Import", Href: base + "/settings?tab=import", Icon: "fa-file-import", Words: "import upload restore"},
		{Label: "Activity log", Href: base + "/settings?tab=activity", Icon: "fa-clock-rotate-left", Words: "activity log history audit"},
		{Label: "Keys", Href: base + "/settings?tab=api-keys", Icon: "fa-key", Words: "api keys tokens tools"},
		{Label: "Invite someone", Href: base + "/members", Icon: "fa-user-plus", Words: "invite add player join"},
		// The finished page pieces are on Game & features, each under its own anchor.
		{Label: "Quest board", Href: base + "/extensions#quest-board", Icon: "fa-scroll", Words: "quest board quests cork put on a page"},
		{Label: "Notice boards", Href: base + "/extensions#notice-boards", Icon: "fa-thumbtack", Words: "notice board boards tavern town put on a page"},
		{Label: "Trackers", Href: base + "/extensions#trackers", Icon: "fa-gauge", Words: "tracker clock clocks counter steps put on a page"},
		{Label: "Tests", Href: base + "/extensions#tests", Icon: "fa-flag-checkered", Words: "test montage chase put on a page"},
		{Label: "Archive or delete this campaign", Href: base + "/settings#danger", Icon: "fa-triangle-exclamation", Words: "delete archive remove end"},
	}
}

// NavManageKey is the one sidebar row that leads to the desk (Campaign
// settings). A tool's own key ("manage:trash") is what a pin and the palette
// use; this row stands for all of them, so it is current on every owner tool
// page. The key stays "manage" so pins, old links and the current-row match
// keep working under the new name.
const NavManageKey = "manage"

// NavManageRows is the sidebar's Campaign settings entry: one row to the desk, for the
// owner only. Anyone else (a Scribe, a player, an owner viewing as a player
// whose role reads Player, a visitor) gets none.
func NavManageRows(ctx context.Context) []NavRowView {
	if len(NavManagePages(ctx)) == 0 {
		return nil
	}
	return []NavRowView{{
		Key: NavManageKey, Kind: "app", Label: "Campaign settings", Icon: "fa-sliders",
		URL: "/campaigns/" + GetCampaignID(ctx) + "/dashboard",
	}}
}

// NavManagePages are the campaign-management tools, in the order the desk
// lists them. They no longer get a sidebar row each; they feed the command
// palette, the active-row match and the owner's pins. Same owner-only rule as
// NavManageRows.
func NavManagePages(ctx context.Context) []NavRowView {
	if !InCampaign(ctx) || !IsAuthenticated(ctx) || GetCampaignRole(ctx) < 3 {
		return nil
	}
	return ManageToolRows("/campaigns/"+GetCampaignID(ctx),
		IsAddonEnabled(ctx, "media-gallery"), IsAddonEnabled(ctx, "sync-api"))
}

// ManageToolRows is the one list of owner tools (the Campaign settings pages), for the desk's own pages
// and for the campaigns plugin, which checks an owner's pins against it.
// Media and Foundry exist only while their add-on is on.
func ManageToolRows(base string, media, foundry bool) []NavRowView {
	rows := []NavRowView{
		{Key: "manage:dashboard", Label: "Campaign settings", Icon: "fa-gauge", URL: base + "/dashboard"},
		{Key: "manage:members", Label: "People", Icon: "fa-users", URL: base + "/members"},
		// Every change a player made to a page, searchable and undoable.
		{Key: "manage:player-changes", Label: "What players changed", Icon: "fa-clock-rotate-left", URL: base + "/players/changes"},
	}
	if media {
		rows = append(rows, NavRowView{Key: "manage:media", Label: "Media", Icon: "fa-photo-film", URL: base + "/media"})
	}
	rows = append(rows,
		NavRowView{Key: "manage:customize", Label: "Customize", Icon: "fa-paintbrush", URL: base + "/customize"},
		NavRowView{Key: "manage:extensions", Label: "Game & features", Icon: "fa-puzzle-piece", URL: base + "/extensions"},
	)
	// The Foundry page (connection, players, module version, install link,
	// and from there the sync history) only means something once the
	// campaign syncs with Foundry.
	if foundry {
		rows = append(rows, NavRowView{Key: "manage:foundry", Label: "Foundry", Icon: "fa-dice-d20", URL: base + "/foundry"})
	}
	// Your AI: connecting an AI app, what it did, and copy and paste.
	return append(rows,
		NavRowView{Key: "manage:ai", Label: "Your AI", Icon: "fa-wand-magic-sparkles", URL: base + "/ai"},
		NavRowView{Key: "manage:trash", Label: "Trash", Icon: "fa-trash-can", URL: base + "/trash"},
		NavRowView{Key: "manage:settings", Label: "Details and data", Icon: "fa-gear", URL: base + "/settings"},
	)
}
