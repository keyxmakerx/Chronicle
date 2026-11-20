package entities

// The campaign home's "Brochure" block: a small box for one page the owner
// wrote as a new-player leaflet. The box shows the page's title and a line;
// opening it lays the whole page out as panels in a dialog over the app. The
// home lives in the campaigns plugin, which may not read pages, so its
// placeholder loads this fragment.

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// brochureWordsLen caps the owner's words on the box: it is a lead line, and
// the words ride in the placeholder's address.
const brochureWordsLen = 300

// brochureView is what the block draws.
type brochureView struct {
	CampaignID string
	// Empty means no page to show: the owner gets a slot that says how to
	// pick one, and Gone says the picked page no longer exists.
	Empty bool
	Gone  bool
	Name  string
	// Line is the owner's words, or the page's first line when they left them
	// empty.
	Line string
	// Panels counts the cover plus one panel per heading; it is what the
	// client builds, so the box can promise it.
	Panels int
	// HTML is the page as this viewer may read it, for the dialog.
	HTML   string
	DMOnly bool
}

// h2Open marks where a new panel starts, as brochure.js reads the page.
var h2Open = regexp.MustCompile(`(?i)<h2[\s>]`)

// brochurePanels is the number of panels the page lays out as: the cover,
// then one for each second-level heading.
func brochurePanels(entryHTML string) int {
	return 1 + len(h2Open.FindAllStringIndex(entryHTML, -1))
}

// brochurePanelsLabel is the box's count, in words.
func brochurePanelsLabel(n int) string {
	if n == 1 {
		return "1 panel inside"
	}
	return strconv.Itoa(n) + " panels inside"
}

// brochureWords trims the owner's words to a single tidy run of text.
func brochureWords(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > brochureWordsLen {
		s = strings.TrimRight(string(r[:brochureWordsLen]), " ,;:") + "…"
	}
	return s
}

// Brochure draws the block for the page in ?page=, with the owner's words in
// ?text=. A page the viewer can't open, or no page at all, is an empty answer
// that removes the placeholder, except for the owner and co-DMs, who get the
// slot that says how to pick one.
// GET /campaigns/:id/brochure
func (h *Handler) Brochure(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	hp := h.loadHomePage(c, cc, c.QueryParam("page"))
	view := brochureView{CampaignID: cc.Campaign.ID, Empty: true, Gone: hp.Gone}
	if hp.Entity == nil {
		if !hp.Director {
			return c.HTML(http.StatusOK, "")
		}
		return middleware.Render(c, http.StatusOK, brochureBlock(view))
	}

	// The brochure is what a new player reads, so the owner's own secrets stay
	// out of it too: a preview that showed them would hide what players get.
	body := sanitize.HTML(sanitize.StripSecretsHTML(hp.HTML))

	view.Empty = false
	view.Name = hp.Entity.Name
	view.HTML = body
	view.Panels = brochurePanels(body)
	view.DMOnly = hp.Director && hp.Entity.IsPrivate
	view.Line = brochureWords(c.QueryParam("text"))
	if view.Line == "" {
		view.Line = startHereFirstLine(body)
	}
	return middleware.Render(c, http.StatusOK, brochureBlock(view))
}
