package entities

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Other plugins' things share the campaign Trash with pages. Each kind
// registers a TrashKind, and entities lists, restores and purges through it
// without reading that plugin's tables. The adapters live in internal/app.

// TrashedThing is one item other than a page waiting in the Trash.
type TrashedThing struct {
	Kind      string // the kind's word in the restore route, like "game-night"
	Label     string // what it is, shown after its name: "Game night"
	Icon      string // Font Awesome class
	ID        string
	Name      string
	DeletedAt time.Time
}

// TrashKind is one kind of thing that goes to the campaign Trash.
type TrashKind interface {
	// TrashKind is the kind's word in the restore route.
	TrashKind() string
	// ListTrashed lists the campaign's trashed items of this kind.
	ListTrashed(ctx context.Context, campaignID string) ([]TrashedThing, error)
	// RestoreTrashed brings one item back and returns the in-app link to
	// it. An id from another campaign, or one not in the Trash, is not found.
	RestoreTrashed(ctx context.Context, campaignID, id string) (string, error)
	// PurgeTrashedBefore removes for good every item of this kind trashed
	// before cutoff, in any campaign.
	PurgeTrashedBefore(ctx context.Context, cutoff time.Time) (int, error)
}

// AddTrashKind registers a kind; the Trash then lists, restores and purges it.
func (p *pageSafetyService) AddTrashKind(k TrashKind) { p.kinds = append(p.kinds, k) }

// ListOtherTrash lists every registered kind's trashed items, newest first.
// A kind that fails to list is logged and left out, so one broken kind
// never empties the Trash page.
func (p *pageSafetyService) ListOtherTrash(ctx context.Context, campaignID string) []TrashedThing {
	var out []TrashedThing
	for _, k := range p.kinds {
		list, err := k.ListTrashed(ctx, campaignID)
		if err != nil {
			slog.Warn("trash: listing a kind failed", slog.String("kind", k.TrashKind()), slog.Any("error", err))
			continue
		}
		out = append(out, list...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DeletedAt.After(out[j].DeletedAt) })
	return out
}

// RestoreOther restores one item of a registered kind. It reads the item
// from the kind's Trash listing first, so the activity log can name it.
func (p *pageSafetyService) RestoreOther(ctx context.Context, campaignID, kind, id string) (TrashedThing, string, error) {
	for _, k := range p.kinds {
		if k.TrashKind() != kind {
			continue
		}
		list, err := k.ListTrashed(ctx, campaignID)
		if err != nil {
			return TrashedThing{}, "", err
		}
		for _, it := range list {
			if it.ID == id {
				link, err := k.RestoreTrashed(ctx, campaignID, id)
				return it, link, err
			}
		}
		break
	}
	return TrashedThing{}, "", apperror.NewNotFound("nothing like that is in the Trash")
}

// purgeKinds runs each kind's purge with the pages' cutoff.
func (p *pageSafetyService) purgeKinds(ctx context.Context, cutoff time.Time) {
	for _, k := range p.kinds {
		n, err := k.PurgeTrashedBefore(ctx, cutoff)
		if err != nil {
			slog.Error("trash purge failed", slog.String("kind", k.TrashKind()), slog.Any("error", err))
			continue
		}
		if n > 0 {
			slog.Info("trash purged", slog.String("kind", k.TrashKind()), slog.Int("items", n), slog.Time("cutoff", cutoff))
		}
	}
}
