package syncapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

const (
	// flowReadLimit bounds the rows read for one flow before it picks the
	// ones it draws.
	flowReadLimit = 500
	// flowMaxRepeatLookups bounds the "same problem lately" lookups one
	// flow makes.
	flowMaxRepeatLookups = 5
	// flowAdminLookback is how far back the admin flow looks when nothing
	// failed in the last day.
	flowAdminLookback = historyRetention
)

// flowOptions says how a flow is drawn for its reader.
type flowOptions struct {
	admin       bool
	oneGroup    bool
	openHistory string
}

// loadFlow reads the rows around centre and draws them.
func (h *SyncHistoryHandler) loadFlow(ctx context.Context, campaignID string, centre *SyncEvent, opt flowOptions) (*CallFlow, error) {
	at := centre.OccurredAt
	rows, err := h.repo.Window(ctx, campaignID, at.Add(-flowWindow), at.Add(flowWindow), flowReadLimit)
	if err != nil {
		return nil, err
	}
	group := ""
	if opt.oneGroup {
		group = flowGroupKey(*centre)
	}
	rows = flowPick(rows, group, at)

	in := flowInput{
		Events:      rows,
		Admin:       opt.admin,
		Selected:    centre.ID,
		OneGroup:    opt.oneGroup,
		Keys:        h.flowKeys(ctx, campaignID),
		Repeats:     map[int64][]SyncEvent{},
		OpenHistory: opt.openHistory,
	}
	if !opt.admin {
		in.OnlyThese = func(ev SyncEvent) string {
			if ev.ReportedBy != reportedByChronicle {
				return ""
			}
			v := url.Values{}
			v.Set("failed", "1")
			v.Set("q", ev.Call)
			return "/campaigns/" + campaignID + "/sync-history?" + v.Encode()
		}
	}
	since := h.now().Add(-flowRepeatWindow)
	looked := 0
	for _, ev := range rows {
		if ev.OK || ev.Call == "" || looked >= flowMaxRepeatLookups {
			continue
		}
		looked++
		reps, err := h.repo.Failures(ctx, campaignID, ev.Call, ev.Status, since, 50)
		if err != nil {
			// The repeats are a hint; the flow still draws without them.
			slog.Warn("sync flow: repeated failures not read", slog.String("campaign_id", campaignID), slog.Any("error", err))
			continue
		}
		in.Repeats[ev.ID] = reps
	}
	f := buildCallFlow(in)
	f.ID = "sf-" + strconv.FormatInt(centre.ID, 10)
	return f, nil
}

// flowKeys describes each of the campaign's keys: its name, whose it is and
// their role now.
func (h *SyncHistoryHandler) flowKeys(ctx context.Context, campaignID string) map[int]FlowKey {
	out := map[int]FlowKey{}
	if h.syncSvc == nil {
		return out
	}
	keys, err := h.syncSvc.ListKeysByCampaign(ctx, campaignID)
	if err != nil {
		return out
	}
	people := map[string]FlowKey{}
	for _, k := range keys {
		p, ok := people[k.UserID]
		if !ok {
			p = FlowKey{Role: "former member"}
			if m, err := h.campaignSvc.GetMember(ctx, campaignID, k.UserID); err == nil && m != nil {
				p.Owner = m.DisplayName
				p.Role = m.Role.String()
				if m.Role < campaigns.RoleOwner {
					if granted, err := h.campaignSvc.IsUserDmGranted(ctx, campaignID, k.UserID); err == nil && granted {
						p.Role += " with DM access"
					}
				}
			}
			people[k.UserID] = p
		}
		out[k.ID] = FlowKey{Name: k.Name, Owner: p.Owner, Role: p.Role}
	}
	return out
}

// Flow serves the call flow around one history row, for the Sync history
// page: only the steps about the same thing.
// GET /campaigns/:id/sync-history/flow/:eventID
func (h *SyncHistoryHandler) Flow(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := strconv.ParseInt(c.Param("eventID"), 10, 64)
	if err != nil || id < 1 {
		return apperror.NewBadRequest("invalid history entry")
	}
	ctx := c.Request().Context()
	centre, err := h.repo.Get(ctx, cc.Campaign.ID, id)
	if err != nil {
		return err
	}
	f, err := h.loadFlow(ctx, cc.Campaign.ID, centre, flowOptions{oneGroup: true})
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, SyncFlow(f))
}

// AdminFlow serves a campaign's call flow to site admins: around its latest
// failure in the last day, else its latest row, with names hidden.
// GET /admin/api/sync-flow/:campaignID
func (h *SyncHistoryHandler) AdminFlow(c echo.Context) error {
	campaignID := c.Param("campaignID")
	ctx := c.Request().Context()
	now := h.now()
	centre, err := h.repo.Latest(ctx, campaignID, now.Add(-flowRepeatWindow), true)
	if err != nil {
		return err
	}
	if centre == nil {
		if centre, err = h.repo.Latest(ctx, campaignID, now.Add(-flowAdminLookback), false); err != nil {
			return err
		}
	}
	if centre == nil {
		return middleware.Render(c, http.StatusOK, SyncFlowEmpty())
	}
	opt := flowOptions{admin: true}
	// The link to the campaign's own history shows only to an admin who
	// can open it there.
	if userID := auth.GetUserID(c); userID != "" {
		if m, err := h.campaignSvc.GetMember(ctx, campaignID, userID); err == nil && m != nil {
			ok := m.Role >= campaigns.RoleOwner
			if !ok {
				granted, gerr := h.campaignSvc.IsUserDmGranted(ctx, campaignID, userID)
				ok = gerr == nil && granted
			}
			if ok {
				opt.openHistory = "/campaigns/" + campaignID + "/sync-history?failed=1"
			}
		}
	}
	f, err := h.loadFlow(ctx, campaignID, centre, opt)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, SyncFlow(f))
}

// RegisterAdminSyncFlowRoute mounts the admin flow on the admin group, which
// already requires a site admin.
func RegisterAdminSyncFlowRoute(adminGroup *echo.Group, h *SyncHistoryHandler) {
	adminGroup.GET("/api/sync-flow/:campaignID", h.AdminFlow)
}
