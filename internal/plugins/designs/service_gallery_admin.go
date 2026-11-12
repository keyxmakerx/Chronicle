package designs

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// AdminQueue lists entries in one state for the admin page: pending by default,
// visible so a visible one can be removed. Any other state is refused.
func (g *gallery) AdminQueue(ctx context.Context, status string, page int) (*AdminPage, error) {
	if status == "" {
		status = StatusPending
	}
	if status != StatusPending && status != StatusVisible {
		return nil, apperror.NewBadRequest("status must be pending or visible")
	}
	if page < 1 {
		page = 1
	}
	list, total, err := g.repo.ByStatus(ctx, status, GalleryPageSize, (page-1)*GalleryPageSize)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	out := &AdminPage{Entries: make([]AdminEntry, 0, len(list)), Total: total, Page: page, PerPage: GalleryPageSize, Status: status}
	for i := range list {
		a, err := g.adminEntry(ctx, &list[i])
		if err != nil {
			return nil, err
		}
		out.Entries = append(out.Entries, *a)
	}
	return out, nil
}

func (g *gallery) adminEntry(ctx context.Context, e *GalleryEntry) (*AdminEntry, error) {
	reports, err := g.repo.Reports(ctx, e.ID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	words := make([]string, 0, len(e.Reasons))
	for _, r := range e.Reasons {
		words = append(words, ReasonText(r))
	}
	return &AdminEntry{
		ID: e.ID, Kind: e.Kind, SystemID: e.SystemID, Name: e.Name, Note: e.Note, MakerName: e.MakerName,
		Summary: summarize(e.Data), Status: e.Status, ReasonCodes: append([]string{}, e.Reasons...), Reasons: words,
		Reports: reports, PublishedAt: e.PublishedAt,
	}, nil
}

// AdminDecide moves an entry: approve (waiting or refused to visible), reject
// (waiting to refused) or remove (visible to removed). Anything else is a
// conflict, so a double click or a stale page cannot undo a newer decision.
func (g *gallery) AdminDecide(ctx context.Context, actorID, entryID, decision string) (*AdminEntry, error) {
	e, err := g.repo.Get(ctx, entryID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if e == nil {
		return nil, apperror.NewNotFound(msgEntryGone)
	}
	var to string
	switch {
	case decision == DecisionApprove && (e.Status == StatusPending || e.Status == StatusRejected):
		to = StatusVisible
	case decision == DecisionReject && e.Status == StatusPending:
		to = StatusRejected
	case decision == DecisionRemove && e.Status == StatusVisible:
		to = StatusRemoved
	case decision != DecisionApprove && decision != DecisionReject && decision != DecisionRemove:
		return nil, apperror.NewBadRequest("the decision must be approve, reject or remove")
	default:
		return nil, apperror.NewConflict("this entry is " + e.Status + " now, so that decision no longer applies")
	}
	e.Status, e.ReviewedBy = to, actorID
	if to == StatusVisible {
		// An approved entry starts clean: it needs three fresh reports to come back.
		e.Reasons = []string{}
		if err := g.repo.ClearReports(ctx, e.ID); err != nil {
			return nil, apperror.NewInternal(err)
		}
	}
	if err := g.repo.Save(ctx, e); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return g.adminEntry(ctx, e)
}
