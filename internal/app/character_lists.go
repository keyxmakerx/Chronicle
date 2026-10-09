package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// character_lists.go wires the owner's choice of character and NPC page types:
// where it is stored (the campaign settings JSON) and the one-time pass that
// records, for campaigns that predate the choice, what they showed before.

// characterListsSeededKey is the site setting that records the one-time pass.
const characterListsSeededKey = "reconcile.character_lists_seeded"

// characterListCampaigns is the slice of the campaign service the store needs.
type characterListCampaigns interface {
	GetByID(ctx context.Context, id string) (*campaigns.Campaign, error)
	UpdateCharacterLists(ctx context.Context, campaignID string, characterTypeIDs, npcTypeIDs []int) error
}

// characterListStore keeps the lists in the campaign settings, so no table or
// migration is involved and they travel with the campaign's other settings.
type characterListStore struct {
	camps characterListCampaigns
}

// Get returns the stored lists; ok is false when the campaign never recorded
// either list, which is how the one-time pass tells it still has work to do.
func (s *characterListStore) Get(ctx context.Context, campaignID string) (entities.CharacterLists, bool, error) {
	c, err := s.camps.GetByID(ctx, campaignID)
	if err != nil {
		return entities.CharacterLists{}, false, err
	}
	if c == nil {
		return entities.CharacterLists{}, false, apperror.NewNotFound("campaign not found")
	}
	st := c.ParseSettings()
	var out entities.CharacterLists
	if st.CharacterTypeIDs != nil {
		out.CharacterTypeIDs = *st.CharacterTypeIDs
	}
	if st.NPCTypeIDs != nil {
		out.NPCTypeIDs = *st.NPCTypeIDs
	}
	return out, st.CharacterTypeIDs != nil || st.NPCTypeIDs != nil, nil
}

// Save stores both lists.
func (s *characterListStore) Save(ctx context.Context, campaignID string, l entities.CharacterLists) error {
	return s.camps.UpdateCharacterLists(ctx, campaignID, l.CharacterTypeIDs, l.NPCTypeIDs)
}

// characterListSeedSettings is the slice of the settings repository the pass
// needs.
type characterListSeedSettings interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// characterListSeeder fills one campaign's lists if it has none.
type characterListSeeder interface {
	Seed(ctx context.Context, campaignID string) error
}

// seedCharacterListsOnce gives every campaign without lists its starting
// lists, once per site. The site setting is written only when every campaign
// succeeded, so a failure is retried on the next boot; campaigns already seeded
// (or edited by their owner) are skipped by Seed, so a repeat changes nothing.
func seedCharacterListsOnce(ctx context.Context, st characterListSeedSettings, camps pageExtrasCampaigns, seeder characterListSeeder) (int, error) {
	if v, err := st.Get(ctx, characterListsSeededKey); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", characterListsSeededKey, err)
		}
	}

	seeded, failed := 0, 0
	for page := 1; ; page++ {
		list, total, err := camps.ListAll(ctx, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return seeded, fmt.Errorf("listing campaigns: %w", err)
		}
		for _, c := range list {
			if err := seeder.Seed(ctx, c.ID); err != nil {
				failed++
				slog.Warn("character lists: seeding failed",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			seeded++
		}
		if len(list) == 0 || page*100 >= total {
			break
		}
	}
	if failed > 0 {
		return seeded, fmt.Errorf("%d campaigns could not be seeded", failed)
	}
	if err := st.Set(ctx, characterListsSeededKey, "1"); err != nil {
		return seeded, fmt.Errorf("recording %s: %w", characterListsSeededKey, err)
	}
	return seeded, nil
}
