package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

func init() {
	addWorkshopSource("ancestries", func(reg *homebrew.Registry, d workshopDeps) error {
		return registerAncestryParts(reg, d.SystemEntries)
	})
}

// ancestryEntries is the part of systems.SystemEntryService an ancestry part
// needs: the campaign's own ancestry entries, as a Director sees them.
type ancestryEntries interface {
	List(ctx context.Context, campaignID, fieldKey string, actor systems.EntryActor) ([]systems.SystemEntry, error)
}

// registerAncestryParts plugs the campaign's own ancestries into the Homebrew
// Workshop. A part holds only the entry's slug: the entry stays the one copy
// the hero creator, the points budget and the Foundry sync read, and its own
// visibility decides who sees the ancestry, so nothing can drift apart.
func registerAncestryParts(reg *homebrew.Registry, entries ancestryEntries) error {
	type ancestryData struct {
		Slug string `json:"slug"`
	}
	return reg.RegisterPart(homebrew.PartKind{Name: "ancestry", Label: "Ancestry",
		Check: func(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
			var d ancestryData
			if err := strictJSON(raw, &d); err != nil {
				return nil, err
			}
			d.Slug = strings.TrimSpace(d.Slug)
			if d.Slug == "" {
				return nil, apperror.NewValidation("pick one of the campaign's ancestries")
			}
			// Only Directors save recipes, so the lookup sees Directors-only
			// and one-character entries too.
			list, err := entries.List(ctx, campaignID, "ancestry", systems.EntryActor{IsDirector: true})
			if err != nil {
				return nil, err
			}
			for _, e := range list {
				if e.Slug == d.Slug {
					return json.Marshal(d)
				}
			}
			return nil, apperror.NewValidation("is not one of the campaign's own ancestries")
		}})
}
