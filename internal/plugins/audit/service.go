package audit

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// perPage is the number of audit entries shown per page in the activity feed.
const perPage = 50

// maxEntityHistoryEntries caps the number of history entries returned for a
// single entity to prevent unbounded result sets.
const maxEntityHistoryEntries = 100

// AuditService handles business logic for the audit log. It validates inputs,
// enforces limits, and delegates persistence to the repository.
type AuditService interface {
	// Log records an audit entry. Designed to be fire-and-forget friendly:
	// errors are logged but callers may choose to ignore them since audit
	// failures should not block the primary operation.
	Log(ctx context.Context, entry *AuditEntry) error

	// GetCampaignActivity returns a paginated activity feed for a campaign.
	// Returns entries, total count, and any error.
	GetCampaignActivity(ctx context.Context, campaignID string, page int) ([]AuditEntry, int, error)

	// GetEntityHistory returns the recent change history for a single entity.
	GetEntityHistory(ctx context.Context, entityID string) ([]AuditEntry, error)

	// GetCampaignStats returns aggregate statistics for a campaign including
	// entity counts, word counts, and editor activity.
	GetCampaignStats(ctx context.Context, campaignID string) (*CampaignStats, error)
}

// auditService implements AuditService.
type auditService struct {
	repo AuditRepository
}

// NewAuditService creates a new audit service with the given repository.
func NewAuditService(repo AuditRepository) AuditService {
	return &auditService{repo: repo}
}

// Log validates and persists an audit entry. Missing required fields cause
// a validation error. Logging failures are recorded via slog so the caller
// can treat this as fire-and-forget when appropriate.
func (s *auditService) Log(ctx context.Context, entry *AuditEntry) error {
	if entry.CampaignID == "" {
		return apperror.NewBadRequest("campaign ID is required for audit entry")
	}
	if entry.UserID == "" {
		return apperror.NewBadRequest("user ID is required for audit entry")
	}
	if entry.Action == "" {
		return apperror.NewBadRequest("action is required for audit entry")
	}

	if err := s.repo.Log(ctx, entry); err != nil {
		slog.Error("failed to write audit log entry",
			slog.String("campaign_id", entry.CampaignID),
			slog.String("action", entry.Action),
			slog.Any("error", err),
		)
		return apperror.NewInternal(fmt.Errorf("writing audit entry: %w", err))
	}

	return nil
}

// GetCampaignActivity returns the paginated activity feed for a campaign.
// Pages are 1-indexed. Invalid page numbers are clamped to 1.
func (s *auditService) GetCampaignActivity(ctx context.Context, campaignID string, page int) ([]AuditEntry, int, error) {
	if page < 1 {
		page = 1
	}

	offset := (page - 1) * perPage
	entries, total, err := s.repo.ListByCampaign(ctx, campaignID, perPage, offset)
	if err != nil {
		return nil, 0, apperror.NewInternal(fmt.Errorf("listing campaign activity: %w", err))
	}

	return entries, total, nil
}

// GetEntityHistory returns the recent change history for a single entity.
// Limited to maxEntityHistoryEntries to prevent excessively large responses.
func (s *auditService) GetEntityHistory(ctx context.Context, entityID string) ([]AuditEntry, error) {
	if entityID == "" {
		return nil, apperror.NewBadRequest("entity ID is required")
	}

	entries, err := s.repo.ListByEntity(ctx, entityID, maxEntityHistoryEntries)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing entity history: %w", err))
	}

	return entries, nil
}

// GetCampaignStats returns aggregate statistics for a campaign.
func (s *auditService) GetCampaignStats(ctx context.Context, campaignID string) (*CampaignStats, error) {
	if campaignID == "" {
		return nil, apperror.NewBadRequest("campaign ID is required")
	}

	stats, err := s.repo.GetCampaignStats(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("getting campaign stats: %w", err))
	}

	return stats, nil
}
