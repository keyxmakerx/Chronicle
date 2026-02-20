// Package syncapi provides a secure REST API for external tool integration.
// External clients (Foundry VTT, custom scripts, etc.) authenticate with API keys
// and can read/write campaign data through versioned endpoints.
package syncapi

import "time"

// APIKeyPermission represents an allowed operation for an API key.
type APIKeyPermission string

const (
	PermRead  APIKeyPermission = "read"
	PermWrite APIKeyPermission = "write"
	PermSync  APIKeyPermission = "sync"
)

// APIKey represents a registered API key for external client access.
type APIKey struct {
	ID          int                `json:"id"`
	KeyHash     string             `json:"-"`                       // Never exposed in JSON.
	KeyPrefix   string             `json:"key_prefix"`              // First 8 chars for display.
	Name        string             `json:"name"`
	UserID      string             `json:"user_id"`
	CampaignID  string             `json:"campaign_id"`
	Permissions []APIKeyPermission `json:"permissions"`
	IPAllowlist []string           `json:"ip_allowlist,omitempty"`
	RateLimit   int                `json:"rate_limit"`              // Requests per minute.
	IsActive    bool               `json:"is_active"`
	LastUsedAt  *time.Time         `json:"last_used_at,omitempty"`
	LastUsedIP  *string            `json:"last_used_ip,omitempty"`
	ExpiresAt   *time.Time         `json:"expires_at,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// IsExpired returns true if the key has passed its expiry date.
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

// HasPermission checks if the key has a specific permission.
func (k *APIKey) HasPermission(perm APIKeyPermission) bool {
	for _, p := range k.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// CreateAPIKeyInput is the validated input for creating a new API key.
type CreateAPIKeyInput struct {
	Name        string
	CampaignID  string
	Permissions []APIKeyPermission
	IPAllowlist []string
	RateLimit   int
	ExpiresAt   *time.Time
}

// CreateAPIKeyResult is returned after key creation, containing the
// plaintext key that is only shown once.
type CreateAPIKeyResult struct {
	Key      *APIKey `json:"key"`
	RawKey   string  `json:"raw_key"` // Plaintext key — shown once, never stored.
}

// APIRequestLog records a single API request for auditing.
type APIRequestLog struct {
	ID           int64     `json:"id"`
	APIKeyID     int       `json:"api_key_id"`
	CampaignID   string    `json:"campaign_id"`
	UserID       string    `json:"user_id"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	StatusCode   int       `json:"status_code"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    *string   `json:"user_agent,omitempty"`
	RequestSize  int       `json:"request_size"`
	ResponseSize int       `json:"response_size"`
	DurationMs   int       `json:"duration_ms"`
	ErrorMessage *string   `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SecurityEventType classifies API security events.
type SecurityEventType string

const (
	EventRateLimit   SecurityEventType = "rate_limit"
	EventAuthFailure SecurityEventType = "auth_failure"
	EventIPBlocked   SecurityEventType = "ip_blocked"
	EventKeyExpired  SecurityEventType = "key_expired"
	EventSuspicious  SecurityEventType = "suspicious"
)

// SecurityEvent records a security-related API event.
type SecurityEvent struct {
	ID         int64             `json:"id"`
	EventType  SecurityEventType `json:"event_type"`
	APIKeyID   *int              `json:"api_key_id,omitempty"`
	CampaignID *string           `json:"campaign_id,omitempty"`
	IPAddress  string            `json:"ip_address"`
	UserAgent  *string           `json:"user_agent,omitempty"`
	Details    map[string]any    `json:"details,omitempty"`
	Resolved   bool              `json:"resolved"`
	ResolvedBy *string           `json:"resolved_by,omitempty"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// IPBlock represents a blocked IP in the admin blocklist.
type IPBlock struct {
	ID        int        `json:"id"`
	IPAddress string     `json:"ip_address"`
	Reason    *string    `json:"reason,omitempty"`
	BlockedBy string     `json:"blocked_by"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// APIStats holds aggregated statistics for dashboards.
type APIStats struct {
	TotalRequests     int64 `json:"total_requests"`
	TotalErrors       int64 `json:"total_errors"`
	UniqueIPs         int   `json:"unique_ips"`
	ActiveKeys        int   `json:"active_keys"`
	SecurityEvents    int64 `json:"security_events"`
	UnresolvedEvents  int64 `json:"unresolved_events"`
	BlockedIPs        int   `json:"blocked_ips"`
	AvgResponseTimeMs int   `json:"avg_response_time_ms"`
}

// RequestLogFilter filters API request log queries.
type RequestLogFilter struct {
	APIKeyID   *int
	CampaignID *string
	IPAddress  *string
	StatusCode *int
	Since      *time.Time
	Limit      int
	Offset     int
}

// SecurityEventFilter filters security event queries.
type SecurityEventFilter struct {
	EventType  *SecurityEventType
	IPAddress  *string
	Resolved   *bool
	Since      *time.Time
	Limit      int
	Offset     int
}

// TimeSeriesPoint represents a single data point in a time series chart.
type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int64     `json:"count"`
}

// TopEntry represents a ranked item (IP, key, path, etc.) for dashboard charts.
type TopEntry struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}
