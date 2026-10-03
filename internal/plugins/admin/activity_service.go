package admin

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"unicode/utf8"
)

// activityPerPage is the page size of the full activity list.
const activityPerPage = 25

// ActivityRecorder is the write side of the admin change log. Handlers (and,
// through their own one-method copies of this interface, other plugins) depend
// on it rather than on the service, and it has no error return: a failed log
// write must never fail the admin action that already succeeded.
type ActivityRecorder interface {
	RecordActivity(ctx context.Context, actorUserID, action, targetType, targetID, targetLabel string)
}

// ActivityService records and lists admin changes.
type ActivityService interface {
	ActivityRecorder

	// Record validates and stores an entry, returning the storage error so
	// callers that care (tests, tools) can see it.
	Record(ctx context.Context, entry ActivityEntry) error

	// List returns one page (1-based) of entries matching the filter, newest
	// first, and the matching total.
	List(ctx context.Context, f ActivityFilter, page, perPage int) ([]ActivityEntry, int, error)

	// Actors lists the people who appear in the log, for the "who" menu.
	Actors(ctx context.Context) ([]ActivityActor, error)
}

type activityService struct {
	repo ActivityRepository
}

// NewActivityService creates the activity service.
func NewActivityService(repo ActivityRepository) ActivityService {
	return &activityService{repo: repo}
}

// Record rejects entries with no action: an unlabeled row can't be rendered
// as a sentence and would only be noise in the log.
func (s *activityService) Record(ctx context.Context, entry ActivityEntry) error {
	entry.Action = strings.TrimSpace(entry.Action)
	if entry.Action == "" {
		return errors.New("admin activity needs an action")
	}
	// Clamp to the column widths: a long campaign or package name must not
	// make the insert fail under strict SQL mode and lose the row.
	entry.Action = clampRunes(entry.Action, 64)
	entry.TargetType = clampRunes(entry.TargetType, 32)
	entry.TargetID = clampRunes(entry.TargetID, 64)
	entry.TargetLabel = clampRunes(entry.TargetLabel, 255)
	return s.repo.Insert(ctx, &entry)
}

// clampRunes cuts s to at most n characters without splitting one.
func clampRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// RecordActivity is the never-fails entry point: the admin change has already
// happened, so a logging error is reported and swallowed.
func (s *activityService) RecordActivity(ctx context.Context, actorUserID, action, targetType, targetID, targetLabel string) {
	err := s.Record(ctx, ActivityEntry{
		ActorUserID: actorUserID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		TargetLabel: targetLabel,
	})
	if err != nil {
		slog.Warn("failed to record admin activity",
			slog.String("action", action), slog.Any("error", err))
	}
}

// List clamps paging so a bad query string can't produce a negative offset or
// an unbounded read.
func (s *activityService) List(ctx context.Context, f ActivityFilter, page, perPage int) ([]ActivityEntry, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = activityPerPage
	}
	if perPage > 100 {
		perPage = 100
	}
	return s.repo.List(ctx, f, perPage, (page-1)*perPage)
}

// Actors passes through; kept on the service so handlers never see the repo.
func (s *activityService) Actors(ctx context.Context) ([]ActivityActor, error) {
	return s.repo.Actors(ctx)
}
