package maps

import (
	"context"
	"testing"
)

type fakePictureStore struct {
	maps, tokens   []PictureRef
	clearedMaps    []string
	clearedTokens  []string
	changedSinceID string // a row edited between the listing and the clear
}

func (f *fakePictureStore) ListMapBackgrounds(context.Context) ([]PictureRef, error) {
	return f.maps, nil
}
func (f *fakePictureStore) ListTokenPictures(context.Context) ([]PictureRef, error) {
	return f.tokens, nil
}
func (f *fakePictureStore) ClearMapBackground(_ context.Context, id, _ string) (bool, error) {
	if id == f.changedSinceID {
		return false, nil
	}
	f.clearedMaps = append(f.clearedMaps, id)
	return true, nil
}
func (f *fakePictureStore) ClearTokenPicture(_ context.Context, id, _ string) (bool, error) {
	f.clearedTokens = append(f.clearedTokens, id)
	return true, nil
}

type fakeMediaOwners map[string]string // media id -> campaign id ("" = none)

func (f fakeMediaOwners) MediaCampaign(_ context.Context, id string) (string, bool, error) {
	c, ok := f[id]
	return c, ok, nil
}

func TestClearForeignPictures(t *testing.T) {
	const avatar = "44444444-4444-4444-8444-444444444444"
	owners := fakeMediaOwners{ownFile: "camp-1", foreignFile: "camp-2", avatar: ""}
	store := &fakePictureStore{
		maps: []PictureRef{
			{ID: "own-bg", CampaignID: "camp-1", Value: ownFile},
			{ID: "foreign-bg", CampaignID: "camp-1", Value: foreignFile},
			{ID: "edited-bg", CampaignID: "camp-1", Value: foreignFile},
			{ID: "missing-bg", CampaignID: "camp-1", Value: "55555555-5555-4555-8555-555555555555"},
		},
		tokens: []PictureRef{
			{ID: "icon", CampaignID: "camp-1", Value: "icons/wolf.png"},
			{ID: "own-tok", CampaignID: "camp-1", Value: "/media/" + ownFile},
			{ID: "foreign-tok", CampaignID: "camp-1", Value: "/media/" + foreignFile + "/thumb/300"},
			{ID: "avatar-tok", CampaignID: "camp-1", Value: "/media/" + avatar},
			{ID: "home-tok", CampaignID: "camp-2", Value: "/media/" + foreignFile},
		},
		changedSinceID: "edited-bg",
	}
	n, err := ClearForeignPictures(context.Background(), store, owners)
	if err != nil {
		t.Fatalf("ClearForeignPictures: %v", err)
	}
	if n != 2 {
		t.Errorf("cleared = %d, want 2", n)
	}
	if len(store.clearedMaps) != 1 || store.clearedMaps[0] != "foreign-bg" {
		t.Errorf("cleared maps = %v, want [foreign-bg]", store.clearedMaps)
	}
	if len(store.clearedTokens) != 1 || store.clearedTokens[0] != "foreign-tok" {
		t.Errorf("cleared tokens = %v, want [foreign-tok]", store.clearedTokens)
	}
}
