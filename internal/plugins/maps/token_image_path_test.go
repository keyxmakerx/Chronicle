// token_image_path_test.go pins that map_tokens.image_path can't be used to
// make every viewer's browser fetch an outside URL: the map widget passes
// the stored value straight into an <img>/Leaflet iconUrl
// (static/js/map_widget.js), so an absolute or protocol-relative value is a
// cross-origin request forced on every viewer, GM included. Relative
// references ("wolf.png", "/media/<id>") are the only forms Chronicle's own
// token create/update paths and the sync API ever write, and must keep
// working; "data:" URIs are self-contained and are allowed too.
package maps

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

func TestCreateToken_RejectsExternalImagePath(t *testing.T) {
	cases := []struct {
		name      string
		imagePath string
		wantErr   bool
	}{
		{"bare filename", "wolf.png", false},
		{"root-relative media reference", "/media/abc-123", false},
		{"data URI", "data:image/png;base64,AAAA", false},
		{"no image path", "", false},
		{"absolute http URL", "http://evil.example/tracker.png", true},
		{"absolute https URL", "https://evil.example/tracker.png", true},
		{"protocol-relative URL", "//evil.example/tracker.png", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"leading space before scheme", " http://evil.example/x.png", true},
		{"leading tab before scheme", "\thttp://evil.example/x.png", true},
		{"leading newline and space before scheme", "\n http://evil.example/x.png", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created := false
			repo := &mockDrawingRepo{
				createTokenFn: func(_ context.Context, _ *Token) error {
					created = true
					return nil
				},
			}
			svc := NewDrawingService(repo)

			input := CreateTokenInput{MapID: "map-1", Name: "Ambush Wolf", X: 10, Y: 10}
			if tc.imagePath != "" {
				p := tc.imagePath
				input.ImagePath = &p
			}
			_, err := svc.CreateToken(context.Background(), input)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected a validation error, got nil")
				}
				var appErr *apperror.AppError
				if !errors.As(err, &appErr) || appErr.Code != 422 {
					t.Errorf("expected a 422 validation AppError, got %v", err)
				}
				if created {
					t.Error("repo.CreateToken must not be reached when image_path is rejected")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for legitimate image_path %q: %v", tc.imagePath, err)
			}
			if !created {
				t.Error("expected repo.CreateToken to be reached for a legitimate image_path")
			}
		})
	}
}

func TestUpdateToken_RejectsExternalImagePath(t *testing.T) {
	cases := []struct {
		name      string
		imagePath string
		wantErr   bool
	}{
		{"bare filename", "dire-wolf.png", false},
		{"root-relative media reference", "/media/xyz-999", false},
		{"protocol-relative URL", "//evil.example/x.png", true},
		{"absolute URL", "http://evil.example/x.png", true},
		{"leading space before scheme", " http://evil.example/x.png", true},
		{"leading tab before scheme", "\thttp://evil.example/x.png", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updated := false
			repo := &mockDrawingRepo{
				getTokenFn: func(_ context.Context, id string) (*Token, error) {
					return &Token{ID: id, MapID: "map-1", Name: "Ambush Wolf", ImagePath: strPtrDS("wolf.png")}, nil
				},
				updateTokenFn: func(_ context.Context, _ *Token) error {
					updated = true
					return nil
				},
			}
			svc := NewDrawingService(repo)

			err := svc.UpdateToken(context.Background(), "tok-1", "map-1", true,
				UpdateTokenInput{ImagePath: patch.Of(tc.imagePath)})

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected a validation error, got nil")
				}
				if updated {
					t.Error("repo.UpdateToken must not be reached when image_path is rejected")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for legitimate image_path %q: %v", tc.imagePath, err)
			}
			if !updated {
				t.Error("expected repo.UpdateToken to be reached for a legitimate image_path")
			}
		})
	}
}

// TestUpdateToken_UnrelatedFieldSucceedsWithPreExistingExternalImagePath pins
// the partial-update contract for the image_path check: a token that already
// carries an external image_path in storage (a legacy row predating this
// validation, or one restored via an older sync-API client) must not have
// every future update rejected just because the stored value is bad. Only a
// caller that actually sends image_path should be validated against it.
func TestUpdateToken_UnrelatedFieldSucceedsWithPreExistingExternalImagePath(t *testing.T) {
	updated := false
	var savedToken *Token
	repo := &mockDrawingRepo{
		getTokenFn: func(_ context.Context, id string) (*Token, error) {
			return &Token{
				ID:        id,
				MapID:     "map-1",
				Name:      "Ambush Wolf",
				ImagePath: strPtrDS("http://evil.example/already-stored.png"),
			}, nil
		},
		updateTokenFn: func(_ context.Context, tok *Token) error {
			updated = true
			savedToken = tok
			return nil
		},
	}
	svc := NewDrawingService(repo)

	// image_path is absent from this patch; only IsLocked is sent.
	err := svc.UpdateToken(context.Background(), "tok-1", "map-1", true,
		UpdateTokenInput{IsLocked: patch.Of(true)})

	if err != nil {
		t.Fatalf("expected an update to an unrelated field to succeed despite a pre-existing external image_path, got: %v", err)
	}
	if !updated {
		t.Error("expected repo.UpdateToken to be reached")
	}
	if savedToken == nil || savedToken.ImagePath == nil || *savedToken.ImagePath != "http://evil.example/already-stored.png" {
		t.Error("expected the untouched image_path to be preserved unchanged")
	}
	if savedToken == nil || !savedToken.IsLocked {
		t.Error("expected the actually-sent IsLocked field to be applied")
	}
}
