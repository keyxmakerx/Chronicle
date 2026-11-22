package app

import (
	"context"
	"log/slog"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// aiAskLister is the connector's request queue as the lookup reads it.
type aiAskLister interface {
	Waiting(ctx context.Context, campaignID, userID string) ([]connector.Request, error)
	Decided(ctx context.Context, campaignID, userID string, olderDays int) ([]connector.Request, error)
}

// aiUndoReader is the Undo state of applied requests, so the AI can tell a
// change the owner put back from one that still stands. Optional: a lister
// without it leaves every applied request reading as applied.
type aiUndoReader interface {
	UndoSummaries(ctx context.Context, campaignID, userID string, ids []int64) (map[int64]connector.UndoSummary, error)
}

// aiEntryLister is the system entries service's list.
type aiEntryLister interface {
	List(ctx context.Context, campaignID, fieldKey string, actor systems.EntryActor) ([]systems.SystemEntry, error)
}

// aiChoicePickLister is the picks waiting on a Director for an ask-first
// entry, as the hero-choice rules service lists them.
type aiChoicePickLister interface {
	WaitingRequests(ctx context.Context, campaignID string, actor systems.EntryActor) ([]systems.WaitingChoiceRequest, error)
}

// aiMemberLister names the writers of players' entries.
type aiMemberLister interface {
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
}

// aiWaitingAdapter answers the AI's `waiting` lookup from the queues a
// Director reviews: the AI's own requests, players' unaccepted entries and
// the armory's requests (as the DM Screen lists them). Each source is
// optional; one that fails is named as missing rather than failing the rest.
type aiWaitingAdapter struct {
	asks     aiAskLister
	entries  aiEntryLister
	members  aiMemberLister
	requests dmscreen.RequestSource
	picks    aiChoicePickLister
}

func (w aiWaitingAdapter) Waiting(ctx context.Context, campaignID string, a records.Actor, olderDays int) (records.WaitingList, error) {
	var out records.WaitingList
	missing := func(what string, err error) {
		slog.Warn("ai waiting lookup", slog.String("campaign_id", campaignID), slog.String("queue", what), slog.Any("error", err))
		out.Missing = append(out.Missing, what)
	}
	if w.asks != nil {
		if l, err := w.asks.Waiting(ctx, campaignID, a.UserID); err != nil {
			missing("the AI's requests", err)
		} else {
			for _, q := range l {
				out.Asks = append(out.Asks, records.WaitingAsk{Summary: q.Summary, Counts: q.Counts, At: q.CreatedAt})
			}
			if d, err := w.asks.Decided(ctx, campaignID, a.UserID, olderDays); err != nil {
				missing("the AI's answered requests", err)
			} else {
				undone := w.undone(ctx, campaignID, a.UserID, d)
				for _, q := range d {
					at := q.CreatedAt
					if q.DecidedAt != nil {
						at = *q.DecidedAt
					}
					out.Decided = append(out.Decided, records.WaitingAsk{
						Summary: q.Summary, Counts: q.Counts, At: at,
						TurnedDown: q.Status == connector.RequestTurnedDown, Outcome: q.Outcome,
						Undone: undone[q.ID],
					})
				}
			}
		}
	}
	director := a.CanAuthorDmOnly()
	if w.entries != nil {
		if err := w.addEntries(ctx, campaignID, a.UserID, director, &out); err != nil {
			missing("players' entries", err)
		}
	}
	if w.picks != nil && director {
		list, err := w.picks.WaitingRequests(ctx, campaignID, systems.EntryActor{UserID: a.UserID, IsDirector: true})
		if err != nil {
			missing("players' picks waiting for a yes", err)
		}
		for _, q := range list {
			out.Picks = append(out.Picks, records.WaitingPick{
				Entry: q.Name, Field: q.FieldLabel, Player: q.PlayerName, Hero: q.EntityName, At: q.CreatedAt,
			})
		}
	}
	if w.requests != nil {
		reqs, ok, err := w.requests.WaitingRequests(ctx, campaignID, dmscreen.Viewer{UserID: a.UserID, Role: a.Role})
		switch {
		case err != nil:
			missing("players' stash and buy requests", err)
		case ok:
			sort.SliceStable(reqs, func(i, j int) bool { return reqs[i].CreatedAt.Before(reqs[j].CreatedAt) })
			for _, r := range reqs {
				out.Requests = append(out.Requests, records.WaitingRequest{Text: r.Text, At: r.CreatedAt})
			}
		}
	}
	return out, nil
}

// addEntries lists the players' entries no Director has accepted, handed-in
// ones first, naming each writer.
func (w aiWaitingAdapter) addEntries(ctx context.Context, campaignID, userID string, director bool, out *records.WaitingList) error {
	list, err := w.entries.List(ctx, campaignID, "", systems.EntryActor{UserID: userID, IsDirector: director})
	if err != nil {
		return err
	}
	names := map[string]string{}
	if w.members != nil {
		if ms, err := w.members.ListMembers(ctx, campaignID); err == nil {
			for _, m := range ms {
				names[m.UserID] = m.DisplayName
			}
		}
	}
	for _, e := range list {
		if e.Status != systems.EntryStatusSent && e.Status != systems.EntryStatusDraft {
			continue
		}
		out.Entries = append(out.Entries, records.WaitingEntry{
			Name: e.Name, Field: e.FieldKey, Writer: names[e.CreatedBy], For: e.ForName,
			HandedIn: e.Status == systems.EntryStatusSent, Note: e.ReviewNote,
		})
	}
	sort.SliceStable(out.Entries, func(i, j int) bool { return out.Entries[i].HandedIn && !out.Entries[j].HandedIn })
	return nil
}

// undone reports which applied requests the owner has put back with Undo. A
// failed read is logged and leaves them reading as applied.
func (w aiWaitingAdapter) undone(ctx context.Context, campaignID, userID string, d []connector.Request) map[int64]bool {
	u, ok := w.asks.(aiUndoReader)
	if !ok || len(d) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(d))
	for _, q := range d {
		if q.Status == connector.RequestAnswered {
			ids = append(ids, q.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sums, err := u.UndoSummaries(ctx, campaignID, userID, ids)
	if err != nil {
		slog.Warn("ai waiting lookup: undo state", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	out := make(map[int64]bool, len(sums))
	for id, s := range sums {
		out[id] = s.UndoneAt != nil
	}
	return out
}
