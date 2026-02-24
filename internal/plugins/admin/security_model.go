package admin

import "time"

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
	}
	if icon, ok := icons[eventType]; ok {
		return icon
	}
	return "fa-solid fa-circle-info text-fg-muted"
}
