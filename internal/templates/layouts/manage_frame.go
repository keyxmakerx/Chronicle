package layouts

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/a-h/templ"
)

// ManageFigureFunc builds a Manage page's own figures ("5 members · 1
// invited") for the frame to draw beside the page's title. The campaigns
// plugin registers it at wiring time: layouts cannot import it, and the
// figures must come from the same provider data the desk reads, so the page
// opened by address and the page reached from the desk show the same ones. It
// returns nil when there is nothing true to show (a viewer who is not the
// owner, a provider that failed), which draws nothing at all.
type ManageFigureFunc func(ctx context.Context, line string) templ.Component

var manageFigure atomic.Pointer[ManageFigureFunc]

// SetManageFigure registers the resolver the frame draws figures with.
func SetManageFigure(f ManageFigureFunc) {
	if f == nil {
		manageFigure.Store(nil)
		return
	}
	manageFigure.Store(&f)
}

// manageFigureFor returns the figures for a page's desk line, or nil.
func manageFigureFor(ctx context.Context, line string) templ.Component {
	if line == "" {
		return nil
	}
	f := manageFigure.Load()
	if f == nil {
		return nil
	}
	return (*f)(ctx, line)
}

// ManageTab is one of Campaign settings' index tabs: its key, its word on the
// tab, and its page under /campaigns/<id>.
type ManageTab struct {
	Key, Label, Path string
}

// manageTabs is the one list of Campaign settings' tabs, in order. Overview is
// the board; each other tab is the page its tiles lead to.
var manageTabs = []ManageTab{
	{Key: "overview", Label: "Overview", Path: "/dashboard"},
	{Key: "campaign", Label: "Campaign", Path: "/settings"},
	{Key: "people", Label: "People", Path: "/members"},
	{Key: "game", Label: "Game", Path: "/extensions"},
	{Key: "connections", Label: "Connections", Path: "/connections"},
	{Key: "data", Label: "Data", Path: "/data"},
}

// ManageTabs returns the tabs, in order.
func ManageTabs() []ManageTab { return append([]ManageTab(nil), manageTabs...) }

// manageTabOfLine is the tab a page sits under, by its desk line, so a tool
// page reached from a tile keeps its tab lit.
var manageTabOfLine = map[string]string{
	"settings": "campaign",
	"people":   "people",
	"changes":  "people",
	"game":     "game",
	"tables":   "game",
	"foundry":  "connections",
	"ai":       "connections",
	"media":    "data",
	"trash":    "data",
}

// manageTabFor is the tab a frame lights: the page's own, then its line's,
// then Overview for the board itself. A page under no tab lights none.
func manageTabFor(d ManageFrameData) string {
	switch {
	case d.Tab != "":
		return d.Tab
	case d.Home:
		return "overview"
	}
	return manageTabOfLine[d.Line]
}

// manageLanding reports whether the page is its tab's own page, where the tab
// already names it and the title is kept for the screen reader only.
func manageLanding(d ManageFrameData) bool { return d.Home || d.Landing }

// ManageMast is the campaign as the settings banner draws it. Everything in it
// was read for the owner of the campaign the page belongs to.
type ManageMast struct {
	Name string
	// Cover is the campaign's backdrop picture and Emblem its brand logo, each
	// a media path; empty draws a cover from the campaign's colour and the
	// name's first letter.
	Cover, Emblem    string
	Public, Archived bool
	// Facts are the muted words after the access pill ("Draw Steel", "5
	// members"); a fact that could not be read is left out.
	Facts []string
}

// Initial is the emblem's letter when the campaign has no logo.
func (m ManageMast) Initial() string {
	for _, r := range strings.TrimSpace(m.Name) {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// ManageMastFunc reads the banner's campaign for the frame. The campaigns
// plugin registers it, as it does the figures; it returns nil when there is
// nothing to draw, and the frame then draws its plain title.
type ManageMastFunc func(ctx context.Context) *ManageMast

var manageMast atomic.Pointer[ManageMastFunc]

// SetManageMast registers the resolver the frame draws the banner with.
func SetManageMast(f ManageMastFunc) {
	if f == nil {
		manageMast.Store(nil)
		return
	}
	manageMast.Store(&f)
}

// manageMastFor returns the banner for the owner of the campaign in context,
// or nil. The tabs lead to owner-only pages, so anyone else, an owner
// previewing as a player included, gets the plain frame.
func manageMastFor(ctx context.Context) *ManageMast {
	if !ManageFindEligible(ctx) || !InCampaign(ctx) {
		return nil
	}
	f := manageMast.Load()
	if f == nil {
		return nil
	}
	return (*f)(ctx)
}

// manageTabHref is a tab's address in the campaign in context.
func manageTabHref(ctx context.Context, t ManageTab) string {
	return "/campaigns/" + GetCampaignID(ctx) + t.Path
}
