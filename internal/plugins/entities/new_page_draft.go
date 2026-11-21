package entities

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// NewPageDraft is what the new page form keeps besides its name, parent and
// category: the choices a refused save must not forget, and the place Cancel
// returns to.
type NewPageDraft struct {
	TypeLabel  string
	TemplateID int
	// IsPrivate is the state the page will be saved in, resolved against the
	// campaign's default and the maker's role, never the form's own guess.
	IsPrivate bool
	// Sheet draws the form for the small sheet over the current screen.
	Sheet bool
	// Return is a same-site path the form was opened from, or "".
	Return string
}

// newPageCancelURL is where Cancel goes: the page the new one was to sit
// under, else the category it was started from, else the place the form was
// opened from, else the Pages list.
func newPageCancelURL(campaignID string, parent *Entity, preselect int, ret string) string {
	base := "/campaigns/" + campaignID + "/entities"
	switch {
	case parent != nil:
		return base + "/" + parent.ID
	case preselect > 0:
		return base + "?type=" + strconv.Itoa(preselect)
	case ret != "":
		return ret
	}
	return base
}

// openedFrom is the same-site path the new page form was opened from: the
// return field or query when it carries one, else, for the page's own GET, the
// Referer when it names this site. The form's own address never counts, so a
// reload or a refused save does not make Cancel loop back to the form.
func openedFrom(c *echo.Context) string {
	ret := auth.SameSitePath(c.FormValue("return"))
	if ret == "" && c.Request().Method == http.MethodGet {
		if u, err := url.Parse(c.Request().Referer()); err == nil && u.Host == c.Request().Host {
			ret = auth.SameSitePath(u.RequestURI())
		}
	}
	if u, err := url.Parse(ret); err != nil || strings.HasSuffix(u.Path, "/entities/new") {
		return ""
	}
	return ret
}
