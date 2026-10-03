package syncapi

import (
	"context"
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

// Bounds on what a client may report in one call. The module batches, so
// these are generous for it and tight for anything else.
const (
	historyMaxEvents   = 50
	historyMaxChildren = 200
	historyPageSize    = 50
)

// ChangeEditorLookup names who last changed a page in Chronicle at or before
// a time, so a change the module applied in Foundry says who made it.
type ChangeEditorLookup interface {
	LastEditor(ctx context.Context, campaignID, entityID string, before time.Time) (userID string, ok bool)
}

// SyncHistoryHandler serves the sync history to the module (REST) and to
// the owner and DM-access members (the Manage page).
type SyncHistoryHandler struct {
	repo        SyncHistoryRepository
	campaignSvc campaigns.CampaignService
	syncSvc     SyncAPIService
	editors     ChangeEditorLookup
	namer       EntityNamer
	now         func() time.Time
}

// NewSyncHistoryHandler creates the history handler. editors may be nil.
func NewSyncHistoryHandler(repo SyncHistoryRepository, campaignSvc campaigns.CampaignService, syncSvc SyncAPIService, editors ChangeEditorLookup) *SyncHistoryHandler {
	return &SyncHistoryHandler{repo: repo, campaignSvc: campaignSvc, syncSvc: syncSvc, editors: editors, now: time.Now}
}

// isDMEquivalent is the same gate as the change feed: the campaign owner or
// a member the owner has given DM access. Fails closed.
func (h *SyncHistoryHandler) isDMEquivalent(c echo.Context) bool {
	key := GetAPIKey(c)
	if key == nil {
		return false
	}
	ctx := c.Request().Context()
	if member, err := h.campaignSvc.GetMember(ctx, key.CampaignID, key.UserID); err == nil && member.Role >= campaigns.RoleOwner {
		return true
	}
	granted, err := h.campaignSvc.IsUserDmGranted(ctx, key.CampaignID, key.UserID)
	return err == nil && granted
}

// historyFilterFrom reads the shared query parameters of both readers.
func historyFilterFrom(c echo.Context) (SyncHistoryFilter, error) {
	f := SyncHistoryFilter{Limit: historyPageSize}
	for name, dst := range map[string]*int64{"before": &f.Before, "after": &f.After} {
		if v := c.QueryParam(name); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				return f, apperror.NewBadRequest(name + " must be a non-negative integer")
			}
			*dst = n
		}
	}
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			f.Limit = n
		}
	}
	switch d := c.QueryParam("direction"); d {
	case "", "all":
	case DirToChronicle, DirToFoundry, DirLink:
		f.Direction = d
	default:
		return f, apperror.NewBadRequest("direction must be to_chronicle, to_foundry or link")
	}
	f.FailedOnly = c.QueryParam("failed") == "1" || c.QueryParam("failed") == "true"
	f.Query = truncate(strings.TrimSpace(c.QueryParam("q")), 100)
	return f, nil
}

type syncHistoryResponse struct {
	Data       []SyncEvent `json:"data"`
	NextBefore int64       `json:"nextBefore,omitempty"`
}

// ListHistory returns history rows newest first.
// GET /api/v1/campaigns/:id/sync/history?before=&after=&limit=&direction=&failed=1&q=
func (h *SyncHistoryHandler) ListHistory(c echo.Context) error {
	if !h.isDMEquivalent(c) {
		return apperror.NewForbidden("the sync history requires owner or DM access")
	}
	f, err := historyFilterFrom(c)
	if err != nil {
		return err
	}
	events, err := h.repo.List(c.Request().Context(), c.Param("id"), f)
	if err != nil {
		return err
	}
	resp := syncHistoryResponse{Data: events}
	if len(events) == f.Limit {
		resp.NextBefore = events[len(events)-1].ID
	}
	return c.JSON(http.StatusOK, resp)
}

// reportedEvent is one event as the module sends it.
type reportedEvent struct {
	At         time.Time       `json:"at"`
	Direction  string          `json:"direction"`
	Kind       string          `json:"kind"`
	ResourceID string          `json:"resourceId"`
	Name       string          `json:"name"`
	Action     string          `json:"action"`
	Call       string          `json:"call"`
	Status     string          `json:"status"`
	OK         *bool           `json:"ok"`
	DurationMs int             `json:"durationMs"`
	Message    string          `json:"message"`
	Children   []reportedEvent `json:"children"`
}

type reportHistoryRequest struct {
	Events []reportedEvent `json:"events"`
}

// ReportHistory stores what the module saw happen on its side: changes it
// applied in Foundry, its connects and catch-up runs, and failures that
// never reached Chronicle.
// POST /api/v1/campaigns/:id/sync/history
func (h *SyncHistoryHandler) ReportHistory(c echo.Context) error {
	if !h.isDMEquivalent(c) {
		return apperror.NewForbidden("the sync history requires owner or DM access")
	}
	key := GetAPIKey(c)
	var req reportHistoryRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if len(req.Events) > historyMaxEvents {
		return apperror.NewBadRequest("too many events in one report (at most 50)")
	}
	ctx := c.Request().Context()
	campaignID := c.Param("id")
	now := h.now()
	stored := 0
	for _, re := range req.Events {
		if len(re.Children) > historyMaxChildren {
			return apperror.NewBadRequest("too many steps in one event (at most 200)")
		}
		ev, err := h.toEvent(ctx, campaignID, key, re, now)
		if err != nil {
			return err
		}
		for _, rc := range re.Children {
			ch, err := h.toEvent(ctx, campaignID, key, rc, now)
			if err != nil {
				return err
			}
			ev.Children = append(ev.Children, ch)
		}
		if _, err := h.repo.Insert(ctx, campaignID, &ev); err != nil {
			return err
		}
		stored++
	}
	return c.JSON(http.StatusOK, map[string]int{"stored": stored})
}

// toEvent validates and clamps one reported event. Text is stored as given
// and escaped when shown; a time far from now is replaced by now so a wrong
// clock cannot reorder the history.
func (h *SyncHistoryHandler) toEvent(ctx context.Context, campaignID string, key *APIKey, re reportedEvent, now time.Time) (SyncEvent, error) {
	switch re.Direction {
	case DirToChronicle, DirToFoundry, DirLink:
	default:
		return SyncEvent{}, apperror.NewBadRequest("direction must be to_chronicle, to_foundry or link")
	}
	at := re.At
	if at.IsZero() || at.After(now.Add(5*time.Minute)) || at.Before(now.Add(-7*24*time.Hour)) {
		at = now
	}
	ok := true
	if re.OK != nil {
		ok = *re.OK
	}
	dur := re.DurationMs
	if dur < 0 || dur > 3_600_000 {
		dur = 0
	}
	ev := SyncEvent{
		OccurredAt:   at,
		Direction:    re.Direction,
		ReportedBy:   reportedByClient,
		Kind:         truncate(strings.TrimSpace(re.Kind), 20),
		ResourceID:   truncate(strings.TrimSpace(re.ResourceID), 64),
		ResourceName: truncate(strings.TrimSpace(re.Name), 200),
		Action:       truncate(strings.TrimSpace(re.Action), 80),
		Call:         truncate(strings.TrimSpace(re.Call), 200),
		Status:       truncate(strings.TrimSpace(re.Status), 16),
		OK:           ok,
		DurationMs:   dur,
		Message:      truncate(strings.TrimSpace(re.Message), 500),
		UserID:       &key.UserID,
		APIKeyID:     &key.ID,
	}
	// The module sends ids; name the page when it didn't, from this campaign only.
	if ev.ResourceName == "" && ev.ResourceID != "" && (ev.Kind == "page" || ev.Kind == "character") && h.namer != nil {
		if name, found := h.namer.EntityName(ctx, campaignID, ev.ResourceID); found {
			ev.ResourceName = truncate(name, 200)
		}
	}
	// A change applied in Foundry was made by someone in Chronicle; name them
	// when the page's own change log says who.
	if ev.Direction == DirToFoundry && ev.ResourceID != "" && h.editors != nil {
		if uid, found := h.editors.LastEditor(ctx, campaignID, ev.ResourceID, at); found {
			ev.UserID = &uid
		}
	}
	return ev, nil
}

// SyncHistoryPageData is what the Sync history page shows.
type SyncHistoryPageData struct {
	CampaignID string
	Events     []SyncEvent
	Filter     SyncHistoryFilter
	NextBefore int64
	LastSeen   *time.Time
	Now        time.Time
}

// canSeeHistory is the page's gate: the owner, or a member given DM access.
func canSeeHistory(cc *campaigns.CampaignContext) bool {
	return cc != nil && cc.VisibilityRole() >= int(campaigns.RoleOwner)
}

func (h *SyncHistoryHandler) pageData(c echo.Context, cc *campaigns.CampaignContext) (SyncHistoryPageData, error) {
	f, err := historyFilterFrom(c)
	if err != nil {
		return SyncHistoryPageData{}, err
	}
	ctx := c.Request().Context()
	events, err := h.repo.List(ctx, cc.Campaign.ID, f)
	if err != nil {
		return SyncHistoryPageData{}, err
	}
	d := SyncHistoryPageData{CampaignID: cc.Campaign.ID, Events: events, Filter: f, Now: h.now()}
	if len(events) == f.Limit && f.After == 0 {
		d.NextBefore = events[len(events)-1].ID
	}
	if keys, err := h.syncSvc.ListKeysByCampaign(ctx, cc.Campaign.ID); err == nil {
		for i := range keys {
			if t := keys[i].LastUsedAt; t != nil && keys[i].IsActive && (d.LastSeen == nil || t.After(*d.LastSeen)) {
				d.LastSeen = t
			}
		}
	}
	return d, nil
}

// Page renders the Sync history page.
// GET /campaigns/:id/sync-history
func (h *SyncHistoryHandler) Page(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	d, err := h.pageData(c, cc)
	if err != nil {
		return err
	}
	if middleware.IsHTMX(c) && c.QueryParam("rows") == "1" {
		return middleware.Render(c, http.StatusOK, SyncHistoryRows(d))
	}
	return middleware.Render(c, http.StatusOK, SyncHistoryPage(cc, d))
}

// RegisterSyncHistoryPageRoutes mounts the Manage page.
func RegisterSyncHistoryPageRoutes(e *echo.Echo, h *SyncHistoryHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id", auth.RequireAuth(authSvc), campaigns.RequireCampaignAccess(campaignSvc))
	cg.GET("/sync-history", h.Page, campaigns.RequireCapability(canSeeHistory, "only the campaign owner and members with DM access can see the sync history"))
}

// WithSyncHistory mounts the sync history on the REST API: the recorder on
// every campaign route and the module's read and report endpoints.
func WithSyncHistory(h *SyncHistoryHandler) func(*APIHandler) {
	return func(api *APIHandler) {
		api.history = h
		h.namer = api
	}
}

// EntityName names a page for a history row. Only a page in this campaign
// is named, so a row can never carry another campaign's title.
func (h *APIHandler) EntityName(ctx context.Context, campaignID, entityID string) (string, bool) {
	e, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil || e == nil || e.CampaignID != campaignID {
		return "", false
	}
	return e.Name, true
}
