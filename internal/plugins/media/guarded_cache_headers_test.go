package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// accessErr drops the "guarded" answer of an access check, for the tests that
// only ask whether the request was allowed.
func accessErr(_ bool, err error) error { return err }

const (
	cachePrivate = "private, no-store, max-age=0"
	cachePublic  = "public, max-age=31536000, immutable"
)

// servedFileService hands the serve handlers one stored file and real paths
// to send, so a test can read the headers of a whole response.
type servedFileService struct {
	fakeAccessMediaService
	file *MediaFile
	dir  string
}

func (s *servedFileService) GetByID(context.Context, string) (*MediaFile, error) {
	return s.file, nil
}

func (s *servedFileService) FilePath(*MediaFile) string {
	return filepath.Join(s.dir, "original.png")
}

func (s *servedFileService) ThumbnailPath(*MediaFile, string) string {
	return filepath.Join(s.dir, "thumb.png")
}

// A file a guard decides per viewer is never cacheable by a shared cache,
// in a public campaign and for a Director too: a cache that kept the
// Director's answer would hand it to the next request without the guard
// running. Every other picture in a public campaign keeps the long public
// lifetime. Read from the whole response, original and thumbnail, so the
// guards' answer is seen to reach the header.
func TestGuardedFilesAreNeverSharedCacheable(t *testing.T) {
	const camp = "camp-1"
	dir := t.TempDir()
	for _, name := range []string{"original.png", "thumb.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("png"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name  string
		cards *fakeCardPictureGuard
		maps  *fakeMapImageGuard
		user  string
		// onPage puts the file on a page everyone can see, which is how a
		// public campaign's visitor is let in at all.
		onPage bool
		want   string
	}{
		{name: "a card's picture, to the owner", cards: &fakeCardPictureGuard{isPic: true}, maps: &fakeMapImageGuard{}, user: "owner", want: cachePrivate},
		{name: "a card's picture, to a co-Director", cards: &fakeCardPictureGuard{isPic: true}, maps: &fakeMapImageGuard{}, user: "codm", want: cachePrivate},
		{name: "a card's picture, to a player it is face up to", cards: &fakeCardPictureGuard{isPic: true, seen: map[string]bool{"player": true}}, maps: &fakeMapImageGuard{}, user: "player", want: cachePrivate},
		{name: "a shadowed map picture, to the owner", cards: &fakeCardPictureGuard{}, maps: &fakeMapImageGuard{hidden: true}, user: "owner", want: cachePrivate},
		{name: "a map picture with no shadow, to a visitor", cards: &fakeCardPictureGuard{}, maps: &fakeMapImageGuard{isPic: true}, user: "", onPage: true, want: cachePublic},
		{name: "an ordinary picture, to the owner", cards: &fakeCardPictureGuard{}, maps: &fakeMapImageGuard{}, user: "owner", want: cachePublic},
		{name: "an ordinary picture, to a visitor", cards: &fakeCardPictureGuard{}, maps: &fakeMapImageGuard{}, user: "", onPage: true, want: cachePublic},
		{name: "a card's picture a public page also shows, to a visitor", cards: &fakeCardPictureGuard{isPic: true}, maps: &fakeMapImageGuard{}, user: "", onPage: true, want: cachePrivate},
	}
	for _, tc := range cases {
		for _, thumb := range []bool{false, true} {
			label := tc.name + ", original"
			if thumb {
				label = tc.name + ", thumbnail"
			}
			t.Run(label, func(t *testing.T) {
				h, _, _ := cardGuardFixture(tc.cards)
				h.mapImages = tc.maps
				svc := &servedFileService{file: publicMediaFileIn(camp), dir: dir}
				if tc.onPage {
					svc.findReferencesFn = func(context.Context, string, string) ([]MediaRef, error) {
						return []MediaRef{{EntityID: "page-1"}}, nil
					}
					h.entityVisibility = &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"page-1": true}}
				}
				h.service = svc

				e := echo.New()
				req := httptest.NewRequest(http.MethodGet, "/media/file-1", nil)
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				values := echo.PathValues{{Name: "id", Value: "file-1"}}
				if thumb {
					values = append(values, echo.PathValue{Name: "size", Value: "300"})
				}
				c.SetPathValues(values)
				if s := sessionOf(tc.user); s != nil {
					auth.SetSession(c, s)
				}

				var err error
				if thumb {
					err = h.serveThumbnail(c)
				} else {
					err = h.serve(c)
				}
				if err != nil {
					t.Fatalf("the file was refused: %v", err)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("status %d, want 200", rec.Code)
				}
				if got := rec.Header().Get("Cache-Control"); got != tc.want {
					t.Errorf("Cache-Control = %q, want %q", got, tc.want)
				}
			})
		}
	}
}

// The guards' own answer: guarded for a file they decide, whoever was let
// through, and not for a file they have nothing to say about.
func TestCheckMediaAccessSaysWhenAFileIsGuarded(t *testing.T) {
	cases := []struct {
		name  string
		cards *fakeCardPictureGuard
		maps  *fakeMapImageGuard
		user  string
		want  bool
	}{
		{"a card's picture, Director", &fakeCardPictureGuard{isPic: true}, &fakeMapImageGuard{}, "owner", true},
		{"a card's picture, shown to the player", &fakeCardPictureGuard{isPic: true, seen: map[string]bool{"player": true}}, &fakeMapImageGuard{}, "player", true},
		{"a shadowed map picture, Director", &fakeCardPictureGuard{}, &fakeMapImageGuard{hidden: true}, "owner", true},
		{"both at once", &fakeCardPictureGuard{isPic: true}, &fakeMapImageGuard{hidden: true}, "owner", true},
		{"a map picture with no shadow", &fakeCardPictureGuard{}, &fakeMapImageGuard{isPic: true}, "player", false},
		{"an ordinary picture", &fakeCardPictureGuard{}, &fakeMapImageGuard{}, "player", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := cardGuardFixture(tc.cards)
			h.mapImages = tc.maps
			c := newAccessTestContext(nil, sessionOf(tc.user))
			guarded, err := h.checkMediaAccess(c, publicMediaFileIn("camp-1"), false, "")
			mustAllow(t, err, tc.name)
			if guarded != tc.want {
				t.Errorf("guarded = %v, want %v", guarded, tc.want)
			}
		})
	}

	t.Run("a refusal never reports guarded", func(t *testing.T) {
		h, _, _ := cardGuardFixture(&fakeCardPictureGuard{isPic: true})
		c := newAccessTestContext(nil, sessionOf("player"))
		guarded, err := h.checkMediaAccess(c, publicMediaFileIn("camp-1"), false, "")
		mustDeny(t, err, "face-down card")
		if guarded {
			t.Error("a refused request reported guarded")
		}
	})
}

// A refusal is never cacheable, whatever was decided on the way to it: a
// face-down card's picture refused to a player, and a public picture whose
// bytes are gone, which fails only after the file's own long public header
// was written. A file that is served keeps the header it was given.
func TestRefusalsAreNeverCacheable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "original.png"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	onPage := func(context.Context, string, string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "page-1"}}, nil
	}

	cases := []struct {
		name    string
		cards   *fakeCardPictureGuard
		dir     string
		user    string
		refused bool
		want    string
	}{
		{name: "a face-down card's picture, to a player", cards: &fakeCardPictureGuard{isPic: true}, dir: dir, user: "player", refused: true, want: "no-store"},
		{name: "a public picture whose bytes are gone", cards: &fakeCardPictureGuard{}, dir: filepath.Join(dir, "missing"), user: "owner", refused: true, want: "no-store"},
		{name: "a public picture that is served", cards: &fakeCardPictureGuard{}, dir: dir, user: "owner", want: cachePublic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := cardGuardFixture(tc.cards)
			h.refusalFloor = 0
			svc := &servedFileService{file: publicMediaFileIn("camp-1"), dir: tc.dir}
			if !tc.cards.isPic {
				svc.findReferencesFn = onPage
				h.entityVisibility = &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"page-1": true}}
			}
			h.service = svc

			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/media/file-1", nil), rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "file-1"}})
			auth.SetSession(c, sessionOf(tc.user))

			err := h.Serve(c)
			if (err != nil) != tc.refused {
				t.Fatalf("refused = %v, want %v (err %v)", err != nil, tc.refused, err)
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.want {
				t.Errorf("Cache-Control = %q, want %q", got, tc.want)
			}
		})
	}
}
