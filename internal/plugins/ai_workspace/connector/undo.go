package connector

import (
	"context"
	"log/slog"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// BeforesKept is how long a request keeps its "before" for Undo. After that
// the request stays in "What it did" but offers no Undo; the page's own
// history and the Trash are what remain.
const BeforesKept = 30 * 24 * time.Hour

// Before is one change of an applied request that Undo can put back: what the
// thing was (BeforeJSON, nil when it did not exist), and a fingerprint of what
// the request left (AfterHash), which Undo compares with the live state so it
// never overwrites later work. BeforeJSON can hold text the owner marked
// GM-only, so it is read only by Undo and never sent to a page.
type Before struct {
	ID         int64
	RequestID  int64
	CampaignID string
	// Kind is "page" or a record kind's name.
	Kind       string
	TargetID   string
	TargetName string
	Action     string
	BeforeJSON *string
	AfterHash  string
	CreatedAt  time.Time
	KeepUntil  time.Time
	UndoneAt   *time.Time
}

// UndoSummary is what "What it did" shows about a request's Undo: how many
// changes can still be put back, of which kinds, and when it was undone. It
// carries no before text.
type UndoSummary struct {
	Left     []Before
	UndoneAt *time.Time
}

// UndoStore is the SQL behind Undo (ai_request_befores and
// ai_connection_requests.undone_at). Every statement is scoped by campaign
// and, through the request's key, by person.
type UndoStore interface {
	// SaveBefores stores the befores of userID's request id, which must be in
	// the given status.
	SaveBefores(ctx context.Context, campaignID, userID string, id int64, status string, befores []Before) error
	// Summaries reads the Undo state of userID's requests, without before text.
	Summaries(ctx context.Context, campaignID, userID string, ids []int64, now time.Time) (map[int64]UndoSummary, error)
	// Claim sets undone_at on userID's answered request in one conditional
	// update and reports whether this call won it.
	Claim(ctx context.Context, campaignID, userID string, id int64, at time.Time) (bool, error)
	// Release clears undone_at again so a failed undo can be tried again.
	Release(ctx context.Context, campaignID, userID string, id int64) error
	// Live lists the request's befores not yet undone and not past keep_until,
	// with their before text.
	Live(ctx context.Context, campaignID, userID string, id int64, now time.Time) ([]Before, error)
	// MarkUndone stamps one before as put back.
	MarkUndone(ctx context.Context, beforeID int64, userID string, at time.Time) error
	// Purge deletes befores past keep_until and returns how many.
	Purge(ctx context.Context, now time.Time) (int64, error)
}

// SetUndo wires Undo. Optional: unwired, requests keep no before and offer
// no Undo.
func (s *Service) SetUndo(u UndoStore) { s.undoStore = u }

// CanUndo reports whether Undo is wired.
func (s *Service) CanUndo() bool { return s.undoStore != nil }

// KeepBefores stores what an applied request changed, so it can be undone for
// BeforesKept. A failure is returned for the caller to log: the changes are
// already written, and they simply offer no Undo.
func (s *Service) KeepBefores(ctx context.Context, campaignID, userID string, id int64, befores []Before) error {
	if s.undoStore == nil || len(befores) == 0 {
		return nil
	}
	now := s.now().UTC()
	for i := range befores {
		befores[i].CampaignID = campaignID
		befores[i].RequestID = id
		befores[i].CreatedAt = now
		befores[i].KeepUntil = now.Add(BeforesKept)
	}
	return s.undoStore.SaveBefores(context.WithoutCancel(ctx), campaignID, userID, id, RequestApplying, befores)
}

// UndoSummaries is the Undo state of userID's requests.
func (s *Service) UndoSummaries(ctx context.Context, campaignID, userID string, ids []int64) (map[int64]UndoSummary, error) {
	if s.undoStore == nil || len(ids) == 0 {
		return nil, nil
	}
	return s.undoStore.Summaries(ctx, campaignID, userID, ids, s.now().UTC())
}

// Applied is userID's applied requests, newest first, for "What it did".
func (s *Service) Applied(ctx context.Context, campaignID, userID string) ([]Request, error) {
	return s.repo.Requests(ctx, campaignID, userID, RequestAnswered, requestsKept)
}

// ClaimUndo takes an applied request for an Undo and returns what it can put
// back. The claim is one conditional update, so a double press, or the same
// page open in two tabs, runs the undo once: the loser gets a conflict.
func (s *Service) ClaimUndo(ctx context.Context, campaignID, userID string, id int64) (*Request, []Before, error) {
	if s.undoStore == nil {
		return nil, nil, apperror.NewBadRequest("Undo is not available on this Chronicle site")
	}
	q, err := s.repo.Request(ctx, campaignID, userID, id)
	if err != nil {
		return nil, nil, err
	}
	if q == nil {
		return nil, nil, apperror.NewNotFound("that request isn't here any more")
	}
	if q.Status != RequestAnswered {
		return nil, nil, apperror.NewBadRequest("only a request you applied can be undone")
	}
	now := s.now().UTC()
	won, err := s.undoStore.Claim(ctx, campaignID, userID, id, now)
	if err != nil {
		return nil, nil, err
	}
	if !won {
		return nil, nil, apperror.NewConflict("that request was already undone, or is being undone")
	}
	left, err := s.undoStore.Live(ctx, campaignID, userID, id, now)
	if err != nil {
		s.ReleaseUndo(ctx, campaignID, userID, id)
		return nil, nil, err
	}
	if len(left) == 0 {
		s.ReleaseUndo(ctx, campaignID, userID, id)
		return nil, nil, apperror.NewBadRequest("there is nothing left of this request to put back")
	}
	return q, left, nil
}

// ReleaseUndo gives an Undo claim back, so a refused or part-done undo can be
// pressed again. It runs detached from the caller's cancellation.
func (s *Service) ReleaseUndo(ctx context.Context, campaignID, userID string, id int64) {
	if err := s.undoStore.Release(context.WithoutCancel(ctx), campaignID, userID, id); err != nil {
		slog.Error("ai connector: releasing undo claim", slog.Int64("request_id", id), slog.Any("error", err))
	}
}

// BeforeUndone records that one change was put back.
func (s *Service) BeforeUndone(ctx context.Context, beforeID int64, userID string) error {
	return s.undoStore.MarkUndone(context.WithoutCancel(ctx), beforeID, userID, s.now().UTC())
}

// StartBeforePurger deletes befores past their keep time now and then every
// hour until ctx ends.
func (s *Service) StartBeforePurger(ctx context.Context) {
	if s.undoStore == nil {
		return
	}
	run := func() {
		n, err := s.undoStore.Purge(ctx, s.now().UTC())
		if err != nil {
			slog.Warn("ai connector: purging old befores", slog.Any("error", err))
		} else if n > 0 {
			slog.Info("ai connector: purged old befores", slog.Int64("rows", n))
		}
	}
	run()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
