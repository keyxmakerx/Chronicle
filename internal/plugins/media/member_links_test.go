package media

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// linkFilesService serves GetByID from a fixed set of files.
type linkFilesService struct {
	fakeAccessMediaService
	files map[string]*MediaFile
}

func (f *linkFilesService) GetByID(_ context.Context, id string) (*MediaFile, error) {
	if file, ok := f.files[id]; ok {
		return file, nil
	}
	return nil, apperror.NewNotFound("media file not found")
}

const (
	linkNoteAudio  = "11111111-1111-4111-8111-111111111111"
	linkHiddenPic  = "22222222-2222-4222-8222-222222222222"
	linkOtherCamp  = "33333333-3333-4333-8333-333333333333"
	linkAvatar     = "44444444-4444-4444-8444-444444444444"
	linkVisiblePic = "55555555-5555-4555-8555-555555555555"
)

func newLinkHandler() *Handler {
	camp, other := "camp-l", "camp-other"
	file := func(id string, campaign *string, usage string) *MediaFile {
		return &MediaFile{ID: id, CampaignID: campaign, CampaignIsPublic: boolPtr(false), UsageType: usage}
	}
	h := newTestHandler("link-secret", map[string]map[string]bool{
		"camp-l": {"player": true},
	})
	h.service = &linkFilesService{
		fakeAccessMediaService: fakeAccessMediaService{
			findReferencesFn: func(_ context.Context, _, mediaID string) ([]MediaRef, error) {
				switch mediaID {
				case linkHiddenPic:
					return []MediaRef{{EntityID: "secret-page"}}, nil
				case linkVisiblePic:
					return []MediaRef{{EntityID: "open-page"}}, nil
				}
				return nil, nil
			},
		},
		files: map[string]*MediaFile{
			linkNoteAudio:  file(linkNoteAudio, &camp, UsageAttachment),
			linkHiddenPic:  file(linkHiddenPic, &camp, UsageEntityImage),
			linkVisiblePic: file(linkVisiblePic, &camp, UsageEntityImage),
			linkOtherCamp:  file(linkOtherCamp, &other, UsageAttachment),
			linkAvatar:     file(linkAvatar, &camp, UsageAvatar),
		},
	}
	h.entityVisibility = &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"open-page": true}}
	return h
}

func TestSignedLinksForMember(t *testing.T) {
	h := newLinkHandler()
	paths := []string{
		"/media/" + linkNoteAudio,
		"/media/" + linkVisiblePic + "/thumb/300?expires=1&sig=old",
		"/media/" + linkHiddenPic,
		"/media/" + linkOtherCamp,
		"/media/" + linkAvatar,
		"/media/" + linkNoteAudio + "/thumb/999",
		"/media/not-a-uuid",
		"https://elsewhere.example/media/" + linkNoteAudio,
		"/media/66666666-6666-4666-8666-666666666666",
	}
	got := h.SignedLinksForMember(context.Background(), "camp-l", "player", paths)
	if len(got) != 2 || got[paths[0]] == "" || got[paths[1]] == "" {
		t.Fatalf("want links for the note audio and the visible picture only, got %v", got)
	}

	// A non-member, or no user, gets nothing at all.
	if n := len(h.SignedLinksForMember(context.Background(), "camp-l", "stranger", paths)); n != 0 {
		t.Fatalf("non-member got %d links", n)
	}
	if n := len(h.SignedLinksForMember(context.Background(), "camp-l", "", paths)); n != 0 {
		t.Fatalf("no user got %d links", n)
	}
	// Without a signer nothing can be made cookieless.
	h.signer = nil
	if n := len(h.SignedLinksForMember(context.Background(), "camp-l", "player", paths)); n != 0 {
		t.Fatalf("no signer still gave %d links", n)
	}
}

// The links work where they are used: a request with no cookie.
func TestSignedLinksForMember_OpenWithoutACookie(t *testing.T) {
	h := newLinkHandler()
	path := "/media/" + linkNoteAudio
	link := h.SignedLinksForMember(context.Background(), "camp-l", "player", []string{path})[path]
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		t.Fatal(err)
	}
	q := req.URL.Query()
	c := newAccessTestContext(map[string]string{"expires": q.Get("expires"), "sig": q.Get("sig")}, nil)
	file, _ := h.service.GetByID(context.Background(), linkNoteAudio)
	if err := h.checkMediaAccess(c, file, false, ""); err != nil {
		t.Fatalf("the signed link didn't open without a cookie: %v", err)
	}
}

// Many spellings of one file cost one look-up and all get the link.
func TestSignedLinksForMember_OneLookUpPerFile(t *testing.T) {
	h := newLinkHandler()
	svc := h.service.(*linkFilesService)
	paths := make([]string, 0, 300)
	for i := 0; i < 300; i++ {
		paths = append(paths, "/media/"+linkNoteAudio+"?n="+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	got := h.SignedLinksForMember(context.Background(), "camp-l", "player", paths)
	if len(got) != len(paths) {
		t.Fatalf("want every spelling linked, got %d of %d", len(got), len(paths))
	}
	if svc.findReferencesCalls != 1 {
		t.Fatalf("want one access check, got %d", svc.findReferencesCalls)
	}
}

// A failing visibility check leaves the file out rather than signing it.
func TestSignedLinksForMember_FailsClosed(t *testing.T) {
	h := newLinkHandler()
	h.entityVisibility = &fakeEntityVisibilityFilter{err: errors.New("db down")}
	path := "/media/" + linkVisiblePic
	if got := h.SignedLinksForMember(context.Background(), "camp-l", "player", []string{path}); len(got) != 0 {
		t.Fatalf("visibility error still signed: %v", got)
	}
}
