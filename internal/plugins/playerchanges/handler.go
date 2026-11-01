package playerchanges

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Ranges the time switch offers.
const (
	RangeMonth = "month"
	RangeLong  = "long"
)

// Handler serves the owner's "What players changed" page.
type Handler struct {
	svc        Service
	keepMonths func(c echo.Context) int
	zone       func(c echo.Context) *time.Location
}

// NewHandler creates the handler. keepMonths says how far back the long
// range reaches; zone is the owner's time zone (UTC when nil).
func NewHandler(svc Service, keepMonths func(c echo.Context) int, zone func(c echo.Context) *time.Location) *Handler {
	return &Handler{svc: svc, keepMonths: keepMonths, zone: zone}
}

// View is what the page shows.
type View struct {
	CampaignID  string
	Suggestions []Suggestion
	// KeepMonths is how long changes are kept; 0 means only the last month.
	KeepMonths int
}

// ResultsView is one search's answer as the page shows it.
type ResultsView struct {
	CampaignID string
	Range      string
	KeepMonths int
	Groups     []PageGroup
	Count      int
	Truncated  bool
	Zone       *time.Location
	Words      []string
	Now        time.Time
}

// PageGroup is the changes to one page, newest first.
type PageGroup struct {
	EntityID string
	Name     string
	Changes  []Change
}

// Page renders the page.
func (h *Handler) Page(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	sugg, err := h.svc.Suggestions(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, PlayerChangesPage(View{CampaignID: cc.Campaign.ID, Suggestions: sugg, KeepMonths: h.months(c)}))
}

// Results answers the search box.
func (h *Handler) Results(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	q, rng := h.query(c)
	res, err := h.svc.Search(c.Request().Context(), cc.Campaign.ID, q, rng == RangeLong)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return middleware.Render(c, http.StatusOK, PlayerChangesResults(h.results(c, cc.Campaign.ID, rng, q, res)))
}

// Undo puts one change back.
func (h *Handler) Undo(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := strconv.ParseInt(c.Param("cid"), 10, 64)
	if err != nil || id <= 0 {
		return errNotFound
	}
	ch, err := h.svc.Undo(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c), id)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, changeLine(cc.Campaign.ID, *ch, h.loc(c), nil, time.Now()))
}

func (h *Handler) query(c echo.Context) (Query, string) {
	params := c.QueryParams()
	q := Query{
		Pages:   capped(params["page"], 20),
		Fields:  capped(params["field"], 20),
		AI:      c.QueryParam("ai") == "1",
		Removed: c.QueryParam("removed") == "1",
	}
	for _, w := range strings.Fields(c.QueryParam("q")) {
		if len(q.Words) < 8 {
			q.Words = append(q.Words, clip(w, 60))
		}
	}
	rng := RangeMonth
	q.Since = time.Now().UTC().AddDate(0, 0, -LiveDays)
	if c.QueryParam("range") == RangeLong && h.months(c) > 0 {
		rng = RangeLong
		q.Since = time.Now().UTC().AddDate(0, -h.months(c), 0)
	}
	return q, rng
}

func (h *Handler) results(c echo.Context, campaignID, rng string, q Query, res *Result) ResultsView {
	v := ResultsView{CampaignID: campaignID, Range: rng, KeepMonths: h.months(c), Count: len(res.Changes),
		Truncated: res.Truncated, Zone: h.loc(c), Words: q.Words, Now: time.Now()}
	at := map[string]int{}
	for _, ch := range res.Changes {
		i, ok := at[ch.EntityID]
		if !ok {
			i = len(v.Groups)
			at[ch.EntityID] = i
			v.Groups = append(v.Groups, PageGroup{EntityID: ch.EntityID, Name: ch.PageName})
		}
		v.Groups[i].Changes = append(v.Groups[i].Changes, ch)
	}
	return v
}

func (h *Handler) months(c echo.Context) int {
	if h.keepMonths == nil {
		return DefaultKeepMonths
	}
	return h.keepMonths(c)
}

func (h *Handler) loc(c echo.Context) *time.Location {
	if h.zone != nil {
		if z := h.zone(c); z != nil {
			return z
		}
	}
	return time.UTC
}

func capped(vals []string, n int) []string {
	var out []string
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" && len(out) < n {
			out = append(out, clip(v, 100))
		}
	}
	return out
}

// RegisterRoutes mounts the page on the campaign group, owner only.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id", auth.RequireAuth(authSvc), campaigns.RequireCampaignAccess(campaignSvc))
	owner := campaigns.RequireRole(campaigns.RoleOwner)
	cg.GET("/players/changes", h.Page, owner)
	cg.GET("/players/changes/results", h.Results, owner)
	cg.POST("/players/changes/:cid/undo", h.Undo, owner)
}
