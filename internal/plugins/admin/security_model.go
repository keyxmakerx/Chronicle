package admin

import (
	"fmt"
	"time"
)

// Security event type constants follow the "resource.verb" pattern for
// consistent filtering and display grouping in the admin security dashboard.
const (
	EventLoginSuccess           = "login.success"
	EventLoginFailed            = "login.failed"
	EventLogout                 = "logout"
	EventPasswordResetInitiated = "password.reset_initiated"
	EventPasswordResetCompleted = "password.reset_completed"
	EventAdminPrivilegeChanged  = "admin.privilege_changed"
	EventUserDisabled           = "admin.user_disabled"
	EventUserEnabled            = "admin.user_enabled"
	EventSessionTerminated      = "admin.session_terminated"
	EventForceLogout            = "admin.force_logout"
	EventDiagnosticsBatchRun    = "admin.diagnostics_batch_run"
	EventMediaUploaded          = "media.uploaded"
	EventMediaDeleted           = "media.deleted"
	EventMediaQuotaExceeded     = "media.quota_exceeded"
)

// SecurityEvent represents a single site-wide security event. Unlike campaign
// audit entries, these track authentication and admin security actions across
// the entire Chronicle instance.
type SecurityEvent struct {
	ID        int64          `json:"id"`
	EventType string         `json:"eventType"`
	UserID    string         `json:"userId,omitempty"`
	ActorID   string         `json:"actorId,omitempty"` // Admin who performed the action.
	IPAddress string         `json:"ipAddress"`
	UserAgent string         `json:"userAgent,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`

	// Joined fields for display (not stored in security_events table).
	UserName  string `json:"userName,omitempty"`
	ActorName string `json:"actorName,omitempty"`
}

// SecurityStats holds aggregate statistics for the admin security dashboard.
type SecurityStats struct {
	TotalEvents        int `json:"totalEvents"`
	FailedLogins24h    int `json:"failedLogins24h"`
	SuccessfulLogins24h int `json:"successfulLogins24h"`
	ActiveSessions     int `json:"activeSessions"`
	DisabledUsers      int `json:"disabledUsers"`
	UniqueIPs24h       int `json:"uniqueIps24h"`
}

// EventTypeLabel returns a human-readable label for a security event type.
func EventTypeLabel(eventType string) string {
	labels := map[string]string{
		EventLoginSuccess:           "Login Success",
		EventLoginFailed:            "Login Failed",
		EventLogout:                 "Logout",
		EventPasswordResetInitiated: "Password Reset Requested",
		EventPasswordResetCompleted: "Password Reset Completed",
		EventAdminPrivilegeChanged:  "Admin Privilege Changed",
		EventUserDisabled:           "User Disabled",
		EventUserEnabled:            "User Enabled",
		EventSessionTerminated:      "Session Terminated",
		EventForceLogout:            "Force Logout",
		EventDiagnosticsBatchRun:    "Diagnostics Batch Run",
		EventMediaUploaded:          "Media Uploaded",
		EventMediaDeleted:           "Media Deleted",
		EventMediaQuotaExceeded:     "Media Quota Exceeded",
	}
	if label, ok := labels[eventType]; ok {
		return label
	}
	return eventType
}

// EventTypeIcon returns a Font Awesome icon class for a security event type.
func EventTypeIcon(eventType string) string {
	icons := map[string]string{
		EventLoginSuccess:           "fa-solid fa-right-to-bracket text-emerald-500",
		EventLoginFailed:            "fa-solid fa-triangle-exclamation text-red-500",
		EventLogout:                 "fa-solid fa-right-from-bracket text-fg-muted",
		EventPasswordResetInitiated: "fa-solid fa-envelope text-amber-500",
		EventPasswordResetCompleted: "fa-solid fa-key text-blue-500",
		EventAdminPrivilegeChanged:  "fa-solid fa-shield text-purple-500",
		EventUserDisabled:           "fa-solid fa-user-slash text-red-500",
		EventUserEnabled:            "fa-solid fa-user-check text-emerald-500",
		EventSessionTerminated:      "fa-solid fa-plug-circle-xmark text-orange-500",
		EventForceLogout:            "fa-solid fa-power-off text-red-500",
		EventDiagnosticsBatchRun:    "fa-solid fa-stethoscope text-slate-500",
		EventMediaUploaded:          "fa-solid fa-cloud-arrow-up text-blue-500",
		EventMediaDeleted:           "fa-solid fa-trash text-red-400",
		EventMediaQuotaExceeded:     "fa-solid fa-hard-drive text-amber-500",
	}
	if icon, ok := icons[eventType]; ok {
		return icon
	}
	return "fa-solid fa-circle-info text-fg-muted"
}

// Tabs of the Sign-ins & sessions page. Each is a real link (?tab=…) so the
// page works without JavaScript and under hx-boost.
const (
	SecurityTabOverview = "overview"
	SecurityTabSessions = "sessions"
	SecurityTabLog      = "log"
	SecurityTabSignup   = "signup"
)

// normalizeSecurityTab whitelists the ?tab= value; anything unknown falls back
// to the overview so a hand-edited URL never renders a blank page.
func normalizeSecurityTab(raw string) string {
	switch raw {
	case SecurityTabSessions, SecurityTabLog, SecurityTabSignup:
		return raw
	default:
		return SecurityTabOverview
	}
}

// securityTabHref is the link for a tab; the overview is the bare page.
func securityTabHref(tab string) string {
	if tab == SecurityTabOverview {
		return "/admin/security"
	}
	return "/admin/security?tab=" + tab
}

// WatchItem is one line of the overview's "Worth a look" list.
type WatchItem struct {
	Title  string
	Detail string
}

// How many wrong passwords for one address inside the window make it worth
// surfacing; a single typo is not.
const (
	repeatedFailureThreshold = 2
	repeatedFailureWindow    = 24 * time.Hour
)

// worthALook turns recent failed-login events into plain-language lines. It
// only reads events already recorded, so it never claims more than the log
// holds.
func worthALook(events []SecurityEvent, now time.Time) []WatchItem {
	type group struct {
		count  int
		ips    map[string]struct{}
		latest time.Time
	}
	byEmail := map[string]*group{}
	var order []string
	for _, e := range events {
		if e.EventType != EventLoginFailed || now.Sub(e.CreatedAt) > repeatedFailureWindow {
			continue
		}
		email, _ := e.Details["email"].(string)
		if email == "" {
			continue
		}
		g, ok := byEmail[email]
		if !ok {
			g = &group{ips: map[string]struct{}{}, latest: e.CreatedAt}
			byEmail[email] = g
			order = append(order, email)
		}
		g.count++
		if e.IPAddress != "" {
			g.ips[e.IPAddress] = struct{}{}
		}
		if e.CreatedAt.After(g.latest) {
			g.latest = e.CreatedAt
		}
	}
	var items []WatchItem
	for _, email := range order {
		g := byEmail[email]
		if g.count < repeatedFailureThreshold {
			continue
		}
		where := "from one address"
		if len(g.ips) > 1 {
			where = fmt.Sprintf("from %d addresses", len(g.ips))
		}
		items = append(items, WatchItem{
			Title:  fmt.Sprintf("%d wrong passwords for %s", g.count, email),
			Detail: where + ", latest " + timeAgo(g.latest),
		})
	}
	return items
}
