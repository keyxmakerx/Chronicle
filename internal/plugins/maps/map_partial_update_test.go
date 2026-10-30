// map_partial_update_test.go pins the map-settings half of the
// absent-means-preserve contract (.ai/conventions.md): ImageID, ImageWidth,
// ImageHeight and Description must use patch.Field[T] rather than a plain
// *string, since a plain pointer bound from JSON can't distinguish "key
// omitted" from "key sent null" — a rename-only PUT must not unlink the
// map's image or wipe its description.
package maps

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// storedMapRow is the fully-configured row every case starts from.
func storedMapRow() *Map {
	desc := "A dungeon beneath the old keep"
	imgID := "img-123"
	bg := "#101010"
	return &Map{
		ID:              "map-1",
		CampaignID:      "camp-1",
		Name:            "Old Map Name",
		Description:     &desc,
		ImageID:         &imgID,
		ImageWidth:      1920,
		ImageHeight:     1080,
		BackgroundColor: &bg,
	}
}

func runMapUpdate(t *testing.T, input UpdateMapInput) *Map {
	t.Helper()
	var written *Map
	repo := &mockMapRepo{
		getMapFn:    func(_ context.Context, _ string) (*Map, error) { return storedMapRow(), nil },
		updateMapFn: func(_ context.Context, m *Map) error { written = m; return nil },
	}
	svc := newTestMapService(repo)
	svc.SetMediaVerifier(&fakeMedia{files: map[string]string{"img-999": "camp-1"}})
	if err := svc.UpdateMap(context.Background(), "map-1", input); err != nil {
		t.Fatalf("UpdateMap: %v", err)
	}
	if written == nil {
		t.Fatal("nothing was written")
	}
	return written
}

// THE headline regression: a rename-only PUT must rename the map and
// change nothing else — including the image link and the description.
func TestMapRename_KeepsImageAndDescription(t *testing.T) {
	got := runMapUpdate(t, UpdateMapInput{Name: "New Map Name"})
	want := storedMapRow()

	if got.Name != "New Map Name" {
		t.Errorf("Name = %q, want %q", got.Name, "New Map Name")
	}
	assertMarkerPtr(t, "ImageID", got.ImageID, want.ImageID)
	assertMarkerPtr(t, "Description", got.Description, want.Description)
	if got.ImageWidth != want.ImageWidth {
		t.Errorf("ImageWidth = %d, want %d (preserved)", got.ImageWidth, want.ImageWidth)
	}
	if got.ImageHeight != want.ImageHeight {
		t.Errorf("ImageHeight = %d, want %d (preserved)", got.ImageHeight, want.ImageHeight)
	}
	assertMarkerPtr(t, "BackgroundColor", got.BackgroundColor, want.BackgroundColor)
}

// The three directions on ImageID/Description/ImageWidth/ImageHeight:
// absent preserves, present replaces, explicit null clears.
func TestMap_ImageAndDescription_ThreeDirections(t *testing.T) {
	strp := func(s string) *string { return &s }
	cases := []struct {
		name        string
		input       UpdateMapInput
		wantImageID *string
		wantDesc    *string
		wantW       int
		wantH       int
	}{
		{
			"absent preserves all four",
			UpdateMapInput{Name: "Old Map Name"},
			strp("img-123"), strp("A dungeon beneath the old keep"), 1920, 1080,
		},
		{
			"present replaces all four",
			UpdateMapInput{
				Name:        "Old Map Name",
				ImageID:     patch.Of("img-999"),
				Description: patch.Of("A new wing"),
				ImageWidth:  patch.Of(800),
				ImageHeight: patch.Of(600),
			},
			strp("img-999"), strp("A new wing"), 800, 600,
		},
		{
			"explicit null clears the pointer fields",
			UpdateMapInput{
				Name:        "Old Map Name",
				ImageID:     patch.Null[string](),
				Description: patch.Null[string](),
			},
			nil, nil, 1920, 1080,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runMapUpdate(t, tc.input)
			assertMarkerPtr(t, "ImageID", got.ImageID, tc.wantImageID)
			assertMarkerPtr(t, "Description", got.Description, tc.wantDesc)
			if got.ImageWidth != tc.wantW {
				t.Errorf("ImageWidth = %d, want %d", got.ImageWidth, tc.wantW)
			}
			if got.ImageHeight != tc.wantH {
				t.Errorf("ImageHeight = %d, want %d", got.ImageHeight, tc.wantH)
			}
		})
	}
}
