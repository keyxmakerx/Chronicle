package admin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// securityPerPage is the number of security events shown per page.
const securityPerPage = 50

// SecurityService handles security event management for the admin dashboard.
// It provides logging, querying, session management, and user account actions.
type SecurityService interface {
	// LogEvent records a security event. Fire-and-forget friendly.
	LogEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) error

	// ListEvents returns paginated security events, optionally filtered by type.
	ListEvents(ctx context.Context, eventType string, page int) ([]SecurityEvent, int, error)

	// GetStats returns aggregate security statistics for the dashboard.
	GetStats(ctx context.Context) (*SecurityStats, error)

	// GetActiveSessions returns all active sessions from the session store.
	GetActiveSessions(ctx context.Context) ([]auth.SessionInfo, error)

	// TerminateSessionByHash destroys a specific session identified by the
	// SHA-256 hash of its token. Avoids exposing raw tokens in admin UI.
	TerminateSessionByHash(ctx context.Context, tokenHash string) error

	// ForceLogoutUser destroys all sessions for a user.
	ForceLogoutUser(ctx context.Context, userID string) (int, error)

	// DisableUser disables a user account and destroys all their sessions.
	DisableUser(ctx context.Context, userID string) error

	// EnableUser re-enables a previously disabled user account.
	EnableUser(ctx context.Context, userID string) error

	// SetConnectionRevoker injects the hub's revocation surface, used by
	// DisableUser. Late-bound and nil-safe: a disabled account is already
	// logged out and refused on its next login regardless.
	SetConnectionRevoker(cr ConnectionRevoker)
}

// ConnectionRevoker is the narrow hub view this plugin needs:
// force-disconnect a user's sockets everywhere when their account is
// disabled — unlike other plugins, there's no single campaign to scope to.
type ConnectionRevoker interface {
	RevokeUserEverywhere(userID string)
}

// securityService implements SecurityService.
type securityService struct {
	repo        SecurityEventRepository
	authRepo    auth.UserRepository
	authService auth.AuthService

	// connRevoker drops an account's sockets everywhere when disabled.
	// Injected after construction; nil means not wired yet, a no-op.
	connRevoker ConnectionRevoker
}

// NewSecurityService creates a new security service.
func NewSecurityService(repo SecurityEventRepository, authRepo auth.UserRepository, authService auth.AuthService) SecurityService {
	return &securityService{
		repo:        repo,
		authRepo:    authRepo,
		authService: authService,
	}
}

// SetConnectionRevoker injects the WebSocket hub's revocation surface.
func (s *securityService) SetConnectionRevoker(cr ConnectionRevoker) {
	s.connRevoker = cr
}

// LogEvent validates and persists a security event.
func (s *securityService) LogEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) error {
	if eventType == "" {
		return apperror.NewBadRequest("event type is required")
	}

	event := &SecurityEvent{
		EventType: eventType,
		UserID:    userID,
		ActorID:   actorID,
		IPAddress: ip,
		UserAgent: userAgent,
		Details:   details,
	}

	if err := s.repo.Log(ctx, event); err != nil {
		slog.Error("failed to log security event",
			slog.String("event_type", eventType),
			slog.String("ip", ip),
			slog.Any("error", err),
		)
		return apperror.NewInternal(fmt.Errorf("logging security event: %w", err))
	}

	return nil
}

// ListEvents returns paginated security events.
func (s *securityService) ListEvents(ctx context.Context, eventType string, page int) ([]SecurityEvent, int, error) {
	if page < 1 {
		page = 1
	}

	offset := (page - 1) * securityPerPage
	events, total, err := s.repo.List(ctx, eventType, securityPerPage, offset)
	if err != nil {
		return nil, 0, apperror.NewInternal(fmt.Errorf("listing security events: %w", err))
	}

	return events, total, nil
}

// GetStats returns aggregate security statistics.
func (s *securityService) GetStats(ctx context.Context) (*SecurityStats, error) {
	stats, err := s.repo.GetStats(ctx)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("getting security stats: %w", err))
	}

	// Add active session count from Redis.
	sessions, err := s.authService.ListAllSessions(ctx)
	if err == nil {
		stats.ActiveSessions = len(sessions)
	}

	return stats, nil
}

// GetActiveSessions returns all active sessions from Redis.
func (s *securityService) GetActiveSessions(ctx context.Context) ([]auth.SessionInfo, error) {
	sessions, err := s.authService.ListAllSessions(ctx)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing active sessions: %w", err))
	}
	return sessions, nil
}

// TerminateSessionByHash destroys a specific session identified by the
// SHA-256 hash of its token. Scans active sessions to find the matching token.
func (s *securityService) TerminateSessionByHash(ctx context.Context, tokenHash string) error {
	if tokenHash == "" {
		return apperror.NewBadRequest("session token hash is required")
	}
	return s.authService.DestroySessionByHash(ctx, tokenHash)
}

// ForceLogoutUser destroys all sessions for a user.
func (s *securityService) ForceLogoutUser(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, apperror.NewBadRequest("user ID is required")
	}
	return s.authService.DestroyAllUserSessions(ctx, userID)
}

// DisableUser disables a user account, invalidates all their sessions, and
// drops any of their live WebSocket sockets in every campaign. Prevents
// future logins until re-enabled by an admin.
func (s *securityService) DisableUser(ctx context.Context, userID string) error {
	if userID == "" {
		return apperror.NewBadRequest("user ID is required")
	}

	// Verify the user exists and isn't already disabled.
	user, err := s.authRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.IsDisabled {
		return apperror.NewBadRequest("user is already disabled")
	}

	// Prevent disabling admin accounts without removing admin first.
	if user.IsAdmin {
		return apperror.NewBadRequest("cannot disable an admin account — remove admin privileges first")
	}

	// Disable the account.
	if err := s.authRepo.UpdateIsDisabled(ctx, userID, true); err != nil {
		return apperror.NewInternal(fmt.Errorf("disabling user: %w", err))
	}

	// Destroy all active sessions so the user is immediately logged out.
	count, _ := s.authService.DestroyAllUserSessions(ctx, userID)
	if count > 0 {
		slog.Info("destroyed sessions for disabled user",
			slog.String("user_id", userID),
			slog.Int("session_count", count),
		)
	}

	// A destroyed session stops HTTP cold, but an open WebSocket is never
	// rechecked, so it would keep receiving without this. Runs last, after
	// the account is disabled and logged out.
	if s.connRevoker != nil {
		s.connRevoker.RevokeUserEverywhere(userID)
	}

	return nil
}

// EnableUser re-enables a previously disabled user account.
func (s *securityService) EnableUser(ctx context.Context, userID string) error {
	if userID == "" {
		return apperror.NewBadRequest("user ID is required")
	}

	user, err := s.authRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !user.IsDisabled {
		return apperror.NewBadRequest("user is not disabled")
	}

	if err := s.authRepo.UpdateIsDisabled(ctx, userID, false); err != nil {
		return apperror.NewInternal(fmt.Errorf("enabling user: %w", err))
	}

	return nil
}
