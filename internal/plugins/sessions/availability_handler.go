package sessions

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// UserDirectory is the narrow slice of the auth service the availability
// overlay needs: resolve a user's stored IANA timezone so the heatmap renders
// in the DM's own zone. Kept minimal to preserve plugin isolation.
type UserDirectory interface {
	GetUser(ctx context.Context, userID string) (*auth.User, error)
}

// SetUserDirectory wires the auth service for viewer-zone resolution. Called
// from RegisterRoutes with the auth service already passed in, so no extra
// app-wiring is required.
func (h *Handler) SetUserDirectory(ud UserDirectory) { h.userDir = ud }

// CampaignReader is the ONE campaign read the overlay's role column needs: the
// campaign row, whose parsed settings carry the co-DM grant list.
//
// It is deliberately GetByID and not IsUserDmGranted(campaign, user):
// campaigns' IsUserDmGranted re-reads the campaign row per call, so asking it
// once per member would turn a roster render into an N+1. One read, one grant
// set, every member labelled from it (WG-4).
type CampaignReader interface {
	GetByID(ctx context.Context, id string) (*campaigns.Campaign, error)
}

// SetCampaignReader wires the campaign read used for co-DM labelling. Nil-safe:
// with no reader the roster still labels roles correctly, it just cannot mark
// the co-DM grant — which is a degradation, not a leak, because the grant only
// ever ADDS a marker.
func (h *Handler) SetCampaignReader(cr CampaignReader) { h.campaignReader = cr }

// ShowAvailability renders the availability page shell (paint grid + DM
// overlay). The interactive grid/heatmap is rendered client-side from the JSON
// APIs below, matching the signed mockup encoding.
// GET /campaigns/:id/availability
func (h *Handler) ShowAvailability(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	csrf := middleware.GetCSRFToken(c)
	canSeeDetail := cc.MemberRole >= campaigns.RoleOwner || cc.IsDmGranted
	storedTZ := h.resolveViewerTZ(c, userID)
	return middleware.Render(c, http.StatusOK,
		AvailabilityPage(cc, csrf, canSeeDetail, userID, storedTZ))
}

// GetMyAvailabilityAPI returns the current user's own recurring pattern to seed
// the paint grid.
// GET /campaigns/:id/availability/mine
func (h *Handler) GetMyAvailabilityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	resp, err := h.svc.GetMyAvailability(c.Request().Context(), cc.Campaign.ID, userID)
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	if resp.TZ == "" {
		// No pattern saved yet — seed the editor with the member's stored zone.
		resp.TZ = h.resolveViewerTZ(c, userID)
	}
	return c.JSON(http.StatusOK, resp)
}

// SaveMyAvailabilityAPI replaces the current user's recurring pattern.
// PUT /campaigns/:id/availability/mine
func (h *Handler) SaveMyAvailabilityAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	var req SaveAvailabilityRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if err := h.svc.SaveMyAvailability(c.Request().Context(), cc.Campaign.ID, userID, req); err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// GetOverlayAPI returns the DM heatmap payload for a week. All members receive
// the anonymous density; per-member identity is included only for the owner /
// DM-granted (enforced here by role, not by route — design §5 / Q1).
// GET /campaigns/:id/availability/overlay?week=YYYY-MM-DD&tz=IANA
func (h *Handler) GetOverlayAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)

	week := c.QueryParam("week")
	if week == "" {
		week = time.Now().UTC().Format("2006-01-02") // service snaps to the Monday
	}
	viewerTZ := h.resolveViewerTZ(c, userID)
	includeDetail := cc.MemberRole >= campaigns.RoleOwner || cc.IsDmGranted

	overlay, err := h.svc.BuildOverlay(c.Request().Context(), cc.Campaign.ID,
		h.overlayMembers(c.Request().Context(), cc.Campaign.ID), week, viewerTZ, includeDetail)
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	return c.JSON(http.StatusOK, overlay)
}

// ListMyExceptionsAPI returns the current user's per-date overrides.
// GET /campaigns/:id/availability/exceptions
func (h *Handler) ListMyExceptionsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	excs, err := h.svc.ListMyExceptions(c.Request().Context(), cc.Campaign.ID, userID)
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	// Project to a small camelCase DTO (avoid leaking internal fields/zone rows).
	type excDTO struct {
		ID          string `json:"id"`
		OnDate      string `json:"onDate"`
		StartMinute int    `json:"startMinute"`
		EndMinute   int    `json:"endMinute"`
		State       string `json:"state"`
	}
	out := make([]excDTO, 0, len(excs))
	for _, e := range excs {
		out = append(out, excDTO{e.ID, e.OnDate, e.StartMinute, e.EndMinute, e.State})
	}
	return c.JSON(http.StatusOK, out)
}

// AddExceptionAPI adds a per-date override for the current user.
// POST /campaigns/:id/availability/exceptions
func (h *Handler) AddExceptionAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	var req AddExceptionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if err := h.svc.AddMyException(c.Request().Context(), cc.Campaign.ID, userID, req); err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ReplaceDayExceptionsAPI atomically replaces the current user's overrides for
// one date with a composed set — the backend for the "compose the day" editor
// (C-SCHED-P2 0c). An empty blocks array clears the day.
// PUT /campaigns/:id/availability/exceptions
func (h *Handler) ReplaceDayExceptionsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	var req ReplaceDayExceptionsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if err := h.svc.ReplaceMyDayExceptions(c.Request().Context(), cc.Campaign.ID, userID, req); err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteExceptionAPI removes one of the current user's own exceptions.
// DELETE /campaigns/:id/availability/exceptions/:eid
func (h *Handler) DeleteExceptionAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	if err := h.svc.DeleteMyException(c.Request().Context(), cc.Campaign.ID, userID, c.Param("eid")); err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ---

// resolveViewerTZ picks the zone the overlay renders in: an explicit ?tz=
// override if valid, else the member's stored account zone, else UTC.
//
// The UTC fallback is correct HERE — the grid has to be drawn in some zone and
// it labels which — and wrong for a per-member clock, which is why storedTZ
// below reports absence rather than guessing (ADR-048 §18).
func (h *Handler) resolveViewerTZ(c echo.Context, userID string) string {
	if tz := c.QueryParam("tz"); timeutil.IsValidLocation(tz) {
		return tz
	}
	if tz := h.storedTZ(c.Request().Context(), userID); tz != "" {
		return tz
	}
	return "UTC"
}

// storedTZ returns a member's stored IANA zone, or "" when they have not set a
// valid one. UNLIKE resolveViewerTZ IT DOES NOT FALL BACK TO UTC: an empty
// string is the honest answer and the callers that print a per-member clock
// depend on being able to tell "no zone" from "UTC" (§5).
func (h *Handler) storedTZ(ctx context.Context, userID string) string {
	if h.userDir == nil || userID == "" {
		return ""
	}
	u, err := h.userDir.GetUser(ctx, userID)
	if err != nil || u == nil || u.Timezone == nil || !timeutil.IsValidLocation(*u.Timezone) {
		return ""
	}
	return *u.Timezone
}

// dmGrantSet reads the campaign's co-DM grant list ONCE and returns it as a set.
// A missing reader or a failed read yields an empty set, which under-marks
// rather than over-marks: a co-DM loses their badge, nobody gains one.
func (h *Handler) dmGrantSet(ctx context.Context, campaignID string) map[string]bool {
	if h.campaignReader == nil || campaignID == "" {
		return nil
	}
	camp, err := h.campaignReader.GetByID(ctx, campaignID)
	if err != nil || camp == nil {
		return nil
	}
	ids := camp.ParseSettings().DmGrantIDs
	if len(ids) == 0 {
		return nil
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// overlayMembers builds the deterministic roster the overlay renders: DM first,
// then members by name, then user ID — so lane colors stay stable per member
// across renders.
//
// The ORDER is also the identity key on the Bench's RSVP panel: hue and pattern
// are taken from this index, so a member answering an RSVP may not move a single
// element (C-CALV4-RSVP-P8 §3). Never sort by availability, tally or answer.
//
// Roles and zones are resolved HERE, once, from the truth sources — the roster's
// own campaigns.Role, the campaign's DmGrantIDs, and users.timezone — so the
// pure projection stays campaigns-free and there is exactly one role vocabulary
// in the product (WG-4).
func (h *Handler) overlayMembers(ctx context.Context, campaignID string) []overlayMemberInput {
	if h.memberLister == nil {
		return nil
	}
	members, err := h.memberLister.ListMembers(ctx, campaignID)
	if err != nil {
		return nil
	}
	grants := h.dmGrantSet(ctx, campaignID)
	out := make([]overlayMemberInput, 0, len(members))
	for _, m := range members {
		out = append(out, overlayMemberInput{
			UserID:    m.UserID,
			Name:      m.DisplayName,
			Avatar:    m.AvatarPath,
			IsOwner:   m.Role >= campaigns.RoleOwner,
			RoleLabel: m.Role.DisplayName(),
			IsCoDM:    grants[m.UserID],
			TZ:        h.storedTZ(ctx, m.UserID),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsOwner != out[j].IsOwner {
			return out[i].IsOwner // owners first
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}
