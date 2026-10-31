package maps

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// Two campaigns, one file each. A map of campaign A may show only A's file:
// the map page signs a media link for whatever id a background or token
// names, so a foreign id would hand campaign B's file to A's viewers.
const (
	ownFile     = "11111111-1111-4111-8111-111111111111"
	foreignFile = "22222222-2222-4222-8222-222222222222"
)

func twoCampaignMedia() *fakeMedia {
	return &fakeMedia{files: map[string]string{ownFile: "camp-1", foreignFile: "camp-2"}}
}

func TestCreateMap_BackgroundMustBeThisCampaigns(t *testing.T) {
	strp := func(s string) *string { return &s }
	tests := []struct {
		name    string
		imageID *string
		wantErr bool
	}{
		{"no background", nil, false},
		{"own file", strp(ownFile), false},
		{"another campaign's file", strp(foreignFile), true},
		{"unknown file", strp("33333333-3333-4333-8333-333333333333"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := false
			svc := NewMapService(&mockMapRepo{createMapFn: func(context.Context, *Map) error { created = true; return nil }})
			svc.SetMediaVerifier(twoCampaignMedia())
			_, err := svc.CreateMap(context.Background(), CreateMapInput{CampaignID: "camp-1", Name: "Keep", ImageID: tt.imageID})
			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateMap err = %v, wantErr %v", err, tt.wantErr)
			}
			if created == tt.wantErr {
				t.Fatalf("map written = %v, want %v", created, !tt.wantErr)
			}
		})
	}
}

func TestCreateMap_UnwiredVerifierRefusesABackground(t *testing.T) {
	svc := NewMapService(&mockMapRepo{createMapFn: func(context.Context, *Map) error { return nil }})
	id := ownFile
	if _, err := svc.CreateMap(context.Background(), CreateMapInput{CampaignID: "camp-1", Name: "Keep", ImageID: &id}); err == nil {
		t.Fatal("a background was accepted with no media check wired")
	}
}

func TestUpdateMap_BackgroundMustBeThisCampaigns(t *testing.T) {
	tests := []struct {
		name    string
		imageID patch.Field[string]
		wantErr bool
	}{
		{"absent", patch.Field[string]{}, false},
		{"cleared", patch.Null[string](), false},
		{"own file", patch.Of(ownFile), false},
		{"re-sending the stored id", patch.Of("img-123"), false},
		{"another campaign's file", patch.Of(foreignFile), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			written := false
			repo := &mockMapRepo{
				getMapFn:    func(context.Context, string) (*Map, error) { return storedMapRow(), nil },
				updateMapFn: func(context.Context, *Map) error { written = true; return nil },
			}
			svc := NewMapService(repo)
			svc.SetMediaVerifier(twoCampaignMedia())
			err := svc.UpdateMap(context.Background(), "map-1", UpdateMapInput{Name: "Keep", ImageID: tt.imageID})
			if (err != nil) != tt.wantErr {
				t.Fatalf("UpdateMap err = %v, wantErr %v", err, tt.wantErr)
			}
			if written == tt.wantErr {
				t.Fatalf("map written = %v, want %v", written, !tt.wantErr)
			}
		})
	}
}

// Every spelling the browser would resolve to "/media/<id>" is checked.
func TestToken_PictureMustBeThisCampaigns(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"bundled icon", "icons/wolf.png", false},
		{"own file", "/media/" + ownFile, false},
		{"own thumbnail", "/media/" + ownFile + "/thumb/300", false},
		{"data picture", "data:image/png;base64,AAAA", false},
		{"another campaign's file", "/media/" + foreignFile, true},
		{"bare foreign id", foreignFile, true},
		{"foreign id in upper case", "/media/" + "22222222-2222-4222-8222-22222222222A", true},
		{"foreign id percent-encoded", "/media/%32%32222222-2222-4222-8222-222222222222", true},
		{"relative path to a foreign id", "../../media/" + foreignFile, true},
		{"foreign id after an own one", "/media/" + ownFile + "?x=" + foreignFile, true},
	}
	for _, tt := range tests {
		t.Run("create "+tt.name, func(t *testing.T) {
			created := false
			repo := &mockDrawingRepo{createTokenFn: func(context.Context, *Token) error { created = true; return nil }}
			svc := newImageService(repo, twoCampaignMedia())
			p := tt.path
			_, err := svc.CreateToken(context.Background(), CreateTokenInput{MapID: "map-1", Name: "Wolf", X: 1, Y: 1, ImagePath: &p})
			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateToken err = %v, wantErr %v", err, tt.wantErr)
			}
			if created == tt.wantErr {
				t.Fatalf("token written = %v, want %v", created, !tt.wantErr)
			}
		})
		t.Run("update "+tt.name, func(t *testing.T) {
			updated := false
			repo := &mockDrawingRepo{
				getTokenFn:    func(context.Context, string) (*Token, error) { return &Token{ID: "tok-1", MapID: "map-1"}, nil },
				updateTokenFn: func(context.Context, *Token) error { updated = true; return nil },
			}
			svc := newImageService(repo, twoCampaignMedia())
			err := svc.UpdateToken(context.Background(), "tok-1", "map-1", true, UpdateTokenInput{ImagePath: patch.Of(tt.path)})
			if (err != nil) != tt.wantErr {
				t.Fatalf("UpdateToken err = %v, wantErr %v", err, tt.wantErr)
			}
			if updated == tt.wantErr {
				t.Fatalf("token written = %v, want %v", updated, !tt.wantErr)
			}
		})
	}
}

// A stored picture the caller didn't change is not re-checked, so an edit to
// a token's position keeps working after its file is deleted.
func TestUpdateToken_UnchangedPictureIsNotRechecked(t *testing.T) {
	stored := "/media/" + foreignFile
	media := twoCampaignMedia()
	repo := &mockDrawingRepo{
		getTokenFn:    func(context.Context, string) (*Token, error) { return &Token{ID: "tok-1", MapID: "map-1", ImagePath: &stored}, nil },
		updateTokenFn: func(context.Context, *Token) error { return nil },
	}
	svc := newImageService(repo, media)
	if err := svc.UpdateToken(context.Background(), "tok-1", "map-1", true, UpdateTokenInput{ImagePath: patch.Of(stored), X: patch.Of(5.0)}); err != nil {
		t.Fatalf("UpdateToken: %v", err)
	}
	if media.calls != 0 {
		t.Fatalf("media lookups = %d, want 0", media.calls)
	}
}
