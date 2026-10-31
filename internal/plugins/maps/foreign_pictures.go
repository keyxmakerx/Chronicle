package maps

import (
	"context"
	"fmt"
	"log/slog"
)

// PictureRef is one stored picture reference: a map's background or a
// token's picture, with the campaign of the map that holds it.
type PictureRef struct {
	ID         string
	CampaignID string
	Value      string
}

// PictureRefStore reads and clears stored picture references for
// ClearForeignPictures. A clear names the value it expects, so a picture
// changed since the listing is left alone.
type PictureRefStore interface {
	ListMapBackgrounds(ctx context.Context) ([]PictureRef, error)
	ListTokenPictures(ctx context.Context) ([]PictureRef, error)
	ClearMapBackground(ctx context.Context, mapID, expected string) (bool, error)
	ClearTokenPicture(ctx context.Context, tokenID, expected string) (bool, error)
}

// MediaCampaignLookup answers which campaign a media file belongs to; found
// is false for an unknown id, and campaignID is "" for a file of no campaign.
type MediaCampaignLookup interface {
	MediaCampaign(ctx context.Context, mediaID string) (campaignID string, found bool, err error)
}

// ClearForeignPictures clears every map background and token picture that
// names another campaign's media file. The write paths refuse such a file;
// this removes any stored before they did. Only a file that exists and
// belongs to a different campaign is cleared: a missing file shows nothing,
// and a file of no campaign is an avatar or backdrop, never campaign
// content. Idempotent, so it runs on every boot.
func ClearForeignPictures(ctx context.Context, store PictureRefStore, media MediaCampaignLookup) (int, error) {
	owners := map[string]string{}
	foreign := func(campaignID, mediaID string) (bool, error) {
		owner, seen := owners[mediaID]
		if !seen {
			c, found, err := media.MediaCampaign(ctx, mediaID)
			if err != nil {
				return false, err
			}
			if !found {
				c = ""
			}
			owner = c
			owners[mediaID] = c
		}
		return owner != "" && owner != campaignID, nil
	}

	cleared := 0
	backgrounds, err := store.ListMapBackgrounds(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing map backgrounds: %w", err)
	}
	for _, r := range backgrounds {
		bad, err := foreign(r.CampaignID, r.Value)
		if err != nil {
			return cleared, err
		}
		if !bad {
			continue
		}
		ok, err := store.ClearMapBackground(ctx, r.ID, r.Value)
		if err != nil {
			return cleared, err
		}
		if ok {
			slog.Warn("maps: cleared a background that named another campaign's file",
				slog.String("map_id", r.ID), slog.String("campaign_id", r.CampaignID), slog.String("media_id", r.Value))
			cleared++
		}
	}

	tokens, err := store.ListTokenPictures(ctx)
	if err != nil {
		return cleared, fmt.Errorf("listing token pictures: %w", err)
	}
	for _, r := range tokens {
		bad := false
		for _, id := range tokenImageMediaIDs(r.Value) {
			f, err := foreign(r.CampaignID, id)
			if err != nil {
				return cleared, err
			}
			if f {
				bad = true
				break
			}
		}
		if !bad {
			continue
		}
		ok, err := store.ClearTokenPicture(ctx, r.ID, r.Value)
		if err != nil {
			return cleared, err
		}
		if ok {
			slog.Warn("maps: cleared a token picture that named another campaign's file",
				slog.String("token_id", r.ID), slog.String("campaign_id", r.CampaignID))
			cleared++
		}
	}
	return cleared, nil
}
