package sessions

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// In-app notification HTTP surface (C-SCHED-P2). These routes are user-scoped
// (NOT campaign-scoped) — the topbar bell is global — so they ride a plain
// authenticated group, not the calendar-addon campaign group. Every read/write
// is scoped to the authenticated user in the service/repo (IDOR guard).

// notifDTO is the camelCase list item the bell widget renders.
type notifDTO struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Link      string `json:"link"`
	Read      bool   `json:"read"`
	Type      string `json:"type"`
	CreatedAt string `json:"createdAt"`
}

// ListNotificationsAPI returns the current user's recent notifications as JSON
// (the bell dropdown renders them client-side).
// GET /notifications
func (h *Handler) ListNotificationsAPI(c echo.Context) error {
	userID := auth.GetUserID(c)
	ns, err := h.svc.ListMyNotifications(c.Request().Context(), userID, 30)
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	out := make([]notifDTO, 0, len(ns))
	for _, n := range ns {
		d := notifDTO{
			ID:        n.ID,
			Type:      n.Type,
			Read:      n.ReadAt != nil,
			CreatedAt: n.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			Message:   notificationMessage(n),
		}
		if n.Link != nil {
			d.Link = *n.Link
		}
		out = append(out, d)
	}
	return c.JSON(http.StatusOK, out)
}

// NotificationBadgeAPI returns the unread-count badge as an HTML fragment for
// the topbar bell's HTMX poll. Empty output means zero unread (nothing shown).
// GET /notifications/badge
func (h *Handler) NotificationBadgeAPI(c echo.Context) error {
	userID := auth.GetUserID(c)
	n, err := h.svc.CountMyUnreadNotifications(c.Request().Context(), userID)
	if err != nil || n <= 0 {
		return c.HTML(http.StatusOK, "")
	}
	label := fmt.Sprintf("%d", n)
	if n > 99 {
		label = "99+"
	}
	return c.HTML(http.StatusOK, fmt.Sprintf(
		`<span class="absolute -top-0.5 -right-0.5 min-w-[16px] h-4 px-1 rounded-full bg-red-500 text-white text-[10px] font-bold flex items-center justify-center" data-count="%d">%s</span>`,
		n, label))
}

// MarkNotificationReadAPI marks one of the current user's notifications read.
// POST /notifications/:nid/read
func (h *Handler) MarkNotificationReadAPI(c echo.Context) error {
	userID := auth.GetUserID(c)
	if err := h.svc.MarkNotificationRead(c.Request().Context(), userID, c.Param("nid")); err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// MarkAllNotificationsReadAPI marks all of the current user's notifications read.
// POST /notifications/read-all
func (h *Handler) MarkAllNotificationsReadAPI(c echo.Context) error {
	userID := auth.GetUserID(c)
	if err := h.svc.MarkAllNotificationsRead(c.Request().Context(), userID); err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// notificationMessage extracts the human-readable message from a notification's
// JSON payload, falling back to a type-based label if the payload is missing.
func notificationMessage(n Notification) string {
	if n.Payload != nil {
		var p notificationPayload
		if err := json.Unmarshal([]byte(*n.Payload), &p); err == nil && p.Message != "" {
			return p.Message
		}
	}
	switch n.Type {
	case NotifProposalCreated:
		return "New scheduling proposal"
	case NotifProposalResponse:
		return "A player responded to your proposal"
	default:
		return "Notification"
	}
}
