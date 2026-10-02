package app

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// adminActivityAdapter lets other plugins write to the admin change log
// through their own one-method ActivityRecorder interfaces. It lives here so
// those plugins neither import admin nor auth: the adapter reads the signed-in
// admin from the request and forwards to the admin service.
type adminActivityAdapter struct {
	svc admin.ActivityRecorder
}

// RecordAdminChange stamps the current admin and records the change; the
// recorder itself swallows storage errors.
func (a adminActivityAdapter) RecordAdminChange(c echo.Context, action, targetType, targetID, label string) {
	a.svc.RecordActivity(c.Request().Context(), auth.GetUserID(c), action, targetType, targetID, label)
}

// adminAPIAlertCounter answers the Home page's "unresolved API alerts" count
// from the sync API stats, over a 30-day window to match how long an
// unreviewed alert is still worth surfacing.
type adminAPIAlertCounter struct {
	sync syncapi.SyncAPIService
}

// CountUnresolvedAPIAlerts returns the number of unresolved API security events.
func (a adminAPIAlertCounter) CountUnresolvedAPIAlerts(ctx context.Context) (int, error) {
	stats, err := a.sync.GetStats(ctx, time.Now().AddDate(0, 0, -30))
	if err != nil || stats == nil {
		return 0, err
	}
	return int(stats.UnresolvedEvents), nil
}
