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

	// TerminateSession destroys a specific session by token.
	TerminateSession(ctx context.Context, token string) error

	// ForceLogoutUser destroys all sessions for a user.
	ForceLogoutUser(ctx context.Context, userID string) (int, error)

	// DisableUser disables a user account and destroys all their sessions.
	DisableUser(ctx context.Context, userID string) error

	// EnableUser re-enables a previously disabled user account.
	EnableUser(ctx context.Context, userID string) error
}

// securityService implements SecurityService.
type securityService struct {
	repo        SecurityEventRepository
	authRepo    auth.UserRepository
	authService auth.AuthService
}

// NewSecurityService creates a new security service.
func NewSecurityService(repo SecurityEventRepository, authRepo auth.UserRepository, authService auth.AuthService) SecurityService {
	return &securityService{
		repo:        repo,
		authRepo:    authRepo,
		authService: authService,
	}
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

// TerminateSession destroys a specific session.
func (s *securityService) TerminateSession(ctx context.Context, token string) error {
	if token == "" {
		return apperror.NewBadRequest("session token is required")
	}
	return s.authService.DestroySession(ctx, token)
}

// ForceLogoutUser destroys all sessions for a user.
func (s *securityService) ForceLogoutUser(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, apperror.NewBadRequest("user ID is required")
	}
	return s.authService.DestroyAllUserSessions(ctx, userID)
}

// DisableUser disables a user account and invalidates all their sessions.
// Prevents future logins until re-enabled by an admin.
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
