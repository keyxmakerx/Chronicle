package admin

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// SetTrashService wires the site Trash.
func (h *Handler) SetTrashService(svc TrashService) {
	h.trash = svc
}

// trashRegion renders the swappable region for a result. HTMX gets the region
// alone; a plain request is sent back to the full page so the action's result
// is never a bare fragment. Every action answers 200 so HTMX swaps the notice
// in even when the action was refused.
func (h *Handler) trashRegion(c echo.Context, notice string, isErr bool) error {
	data := TrashPageData{CSRFToken: middleware.GetCSRFToken(c), Notice: notice, Error: isErr}
	ov, err := h.trash.Overview(c.Request().Context())
	if err != nil {
		slog.Warn("trash: reading the trash failed", slog.Any("error", err))
		data.LoadFailed = true
	}
	data.Overview = ov
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, TrashRegion(data))
	}
	return middleware.Render(c, http.StatusOK, AdminTrashPage(data))
}

// Trash renders the Trash page (GET /admin/trash).
func (h *Handler) Trash(c echo.Context) error {
	if h.trash == nil {
		return apperror.NewMissingContext()
	}
	return h.trashRegion(c, "", false)
}

// UndoTrashedCampaign brings a deleted campaign back
// (POST /admin/trash/campaigns/:id/undo).
func (h *Handler) UndoTrashedCampaign(c echo.Context) error {
	if h.trash == nil {
		return apperror.NewMissingContext()
	}
	id := c.Param("id")
	name, err := h.trash.UndoCampaign(c.Request().Context(), id)
	if err != nil {
		if isNotFound(err) {
			return h.trashRegion(c, "That campaign can't be brought back any more. It is already gone or being emptied.", true)
		}
		return err
	}
	h.record(c, "trash.campaign_restored", "campaign", id, name)
	slog.Info("admin restored campaign from trash", slog.String("campaign_id", id), slog.String("by", auth.GetUserID(c)))
	if name == "" {
		return h.trashRegion(c, "Brought the campaign back.", false)
	}
	return h.trashRegion(c, fmt.Sprintf("Brought back the campaign %s.", name), false)
}

// UndoTrashedBatch brings a file clean-up back
// (POST /admin/trash/batches/:id/undo).
func (h *Handler) UndoTrashedBatch(c echo.Context) error {
	if h.trash == nil {
		return apperror.NewMissingContext()
	}
	id := c.Param("id")
	b, err := h.trash.UndoBatch(c.Request().Context(), id)
	if err != nil {
		if isNotFound(err) {
			return h.trashRegion(c, "That clean-up can't be brought back any more. It is already gone or being emptied.", true)
		}
		return err
	}
	h.record(c, "trash.files_restored", "trash", id, batchSummary(*b))
	slog.Info("admin restored file clean-up from trash", slog.String("batch_id", id), slog.String("by", auth.GetUserID(c)))
	return h.trashRegion(c, fmt.Sprintf("Brought back %s.", batchSummary(*b)), false)
}

// SaveTrashRetention saves how long deleted things are kept
// (POST /admin/trash/retention).
func (h *Handler) SaveTrashRetention(c echo.Context) error {
	if h.trash == nil || h.settingsService == nil {
		return apperror.NewMissingContext()
	}
	days, err := strconv.Atoi(strings.TrimSpace(c.FormValue("trash_retention_days")))
	if err != nil {
		return h.trashRegion(c, "Choose how many days to keep deleted things.", true)
	}
	if err := h.settingsService.UpdateSiteTrashRetentionDays(c.Request().Context(), days); err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusBadRequest {
			return h.trashRegion(c, appErr.Message, true)
		}
		return err
	}
	h.record(c, "trash.retention_changed", "setting", "trash", strconv.Itoa(days)+" days")
	return h.trashRegion(c, fmt.Sprintf("Deleted things are now kept for %d days.", days), false)
}

// EmptyTrash removes everything in the Trash for good (DELETE /admin/trash).
// The route sits behind the password re-check, like every other admin action
// that cannot be undone.
func (h *Handler) EmptyTrash(c echo.Context) error {
	if h.trash == nil {
		return apperror.NewMissingContext()
	}
	res, err := h.trash.EmptyNow(c.Request().Context())
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("emptying the trash: %w", err))
	}
	h.record(c, "trash.emptied", "trash", "", fmt.Sprintf("%d campaigns and %d clean-ups", res.Campaigns, res.Batches))
	slog.Info("admin emptied the trash",
		slog.Int("campaigns", res.Campaigns), slog.Int("cleanups", res.Batches), slog.Int("failed", res.Failed),
		slog.String("by", auth.GetUserID(c)))
	if res.Failed > 0 {
		return h.trashRegion(c, fmt.Sprintf("Emptied what could be removed. %d item(s) could not be finished and will be retried.", res.Failed), true)
	}
	return h.trashRegion(c, "The trash is empty.", false)
}

// actor describes the signed-in admin for the "deleted by" copy.
func (h *Handler) actor(c echo.Context) TrashActor {
	id := auth.GetUserID(c)
	return TrashActor{UserID: id, Name: h.userLabel(c.Request().Context(), id)}
}

// isNotFound reports whether err is a 404 from the services.
func isNotFound(err error) bool {
	var appErr *apperror.AppError
	return errors.As(err, &appErr) && appErr.Code == http.StatusNotFound
}
