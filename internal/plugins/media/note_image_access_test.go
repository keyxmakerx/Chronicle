// note_image_access_test.go pins the rule for a picture that lives in notes:
// readers of a note holding it may open it, nobody else, whatever the pages
// say, and never by falling back to campaign membership.
package media

import (
	"context"
	"errors"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// fakeNoteMedia answers CanReadNoteMedia from a fixed set of readers and
// records what it was asked.
type fakeNoteMedia struct {
	readers  map[string]bool
	err      error
	calls int
	role  int
}

func (f *fakeNoteMedia) CanReadNoteMedia(_ context.Context, _, _ string, role int, userID string) (bool, error) {
	f.calls++
	f.role = role
	if f.err != nil {
		return false, f.err
	}
	return f.readers[userID], nil
}

const (
	noteAuthorID = "user-author"
	noteReaderID = "user-reader"
	noteOtherID  = "user-other"
)

func noteImageFile() *MediaFile {
	f := adr058File()
	f.UsageType = UsageNoteImage
	f.UploadedBy = noteAuthorID
	return f
}

func noteImageHandler(nm NoteMediaAccess, vis *fakeEntityVisibilityFilter, refs []MediaRef) (*Handler, *fakeAccessMediaService) {
	members := map[string]map[string]bool{testCampaignID: {noteAuthorID: true, noteReaderID: true, noteOtherID: true}}
	h, svc := newADR058Handler(members, nil, func(context.Context, string, string) ([]MediaRef, error) { return refs, nil }, vis)
	h.noteMedia = nm
	return h, svc
}

func TestNoteImageAccess(t *testing.T) {
	tests := []struct {
		name         string
		viewer       string // "" is anonymous
		nm           NoteMediaAccess
		allowed      bool
		wantAsk      bool // whether the notes rule had to be consulted
		nonMembr     bool
		uploaderGone bool
	}{
		{name: "uploader before any note holds it", viewer: noteAuthorID, nm: &fakeNoteMedia{}, allowed: true},
		{name: "reader of a note holding it", viewer: noteReaderID, nm: &fakeNoteMedia{readers: map[string]bool{noteReaderID: true}}, allowed: true, wantAsk: true},
		{name: "member who cannot read any such note", viewer: noteOtherID, nm: &fakeNoteMedia{readers: map[string]bool{noteReaderID: true}}, wantAsk: true},
		{name: "anonymous", viewer: "", nm: &fakeNoteMedia{readers: map[string]bool{"": true}}},
		{name: "reader who left the campaign", viewer: noteReaderID, nm: &fakeNoteMedia{readers: map[string]bool{noteReaderID: true}}, nonMembr: true},
		{name: "notes rule errors: fail closed", viewer: noteReaderID, nm: &fakeNoteMedia{err: errors.New("db down")}, wantAsk: true},
		{name: "uploader has left the campaign: others lose it", viewer: noteReaderID, nm: &fakeNoteMedia{readers: map[string]bool{noteReaderID: true}}, uploaderGone: true},
		{name: "notes rule unwired: only the uploader", viewer: noteReaderID, nm: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := noteImageHandler(tc.nm, &fakeEntityVisibilityFilter{}, nil)
			if tc.nonMembr {
				h.memberChecker = &stubMemberChecker{members: map[string]map[string]bool{testCampaignID: {}}}
			}
			if tc.uploaderGone {
				h.memberChecker = &stubMemberChecker{members: map[string]map[string]bool{testCampaignID: {noteReaderID: true}}}
			}
			var c echo.Context
			if tc.viewer == "" {
				c = newAccessTestContext(nil, nil)
			} else {
				c = newAccessTestContext(nil, &auth.Session{UserID: tc.viewer})
			}
			err := h.checkMediaAccess(c, noteImageFile(), false, "")
			if tc.allowed {
				mustAllow(t, err, tc.name)
			} else {
				mustDeny(t, err, tc.name)
			}
			if f, ok := tc.nm.(*fakeNoteMedia); ok && f != nil {
				if asked := f.calls > 0; asked != tc.wantAsk {
					t.Errorf("notes rule consulted = %v, want %v", asked, tc.wantAsk)
				}
			}
		})
	}
}

// A picture used only in someone's private note is not opened up by the
// membership fallback, and a page that happens to mention the file does not
// widen it either.
func TestNoteImageAccess_IgnoresPageRulesAndMembership(t *testing.T) {
	visible := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"ent-public": true}}
	refs := []MediaRef{{EntityID: "ent-public", RefType: "content"}}
	h, svc := noteImageHandler(&fakeNoteMedia{}, visible, refs)

	err := h.checkMediaAccess(newADR058TestContext(noteOtherID), noteImageFile(), false, "")
	mustDeny(t, err, "a visible page mentioning a note picture")
	if svc.findReferencesCalls != 0 || visible.callCount != 0 {
		t.Errorf("the page rule ran for a note picture (refs=%d, filter=%d)", svc.findReferencesCalls, visible.callCount)
	}
}

// Site admins get no way around the notes rule: notes are private from the
// people who run the site as well as from the GM.
func TestNoteImageAccess_AdminIsNotExempt(t *testing.T) {
	h, _ := noteImageHandler(&fakeNoteMedia{}, &fakeEntityVisibilityFilter{}, nil)
	c := newAccessTestContext(nil, &auth.Session{UserID: "user-admin", IsAdmin: true})
	mustDeny(t, h.checkMediaAccess(c, noteImageFile(), false, ""), "site admin reading a private note picture")
}

// Existing entity pictures keep the page rule: the note rule is reached only
// by files stored as note pictures.
func TestNoteImageAccess_OtherFilesKeepThePageRule(t *testing.T) {
	nm := &fakeNoteMedia{readers: map[string]bool{noteOtherID: true}}
	h, _ := noteImageHandler(nm, &fakeEntityVisibilityFilter{}, nil)
	f := adr058File() // an attachment with no page reference: plain membership
	mustAllow(t, h.checkMediaAccess(newADR058TestContext(noteOtherID), f, false, ""), "member reading an unreferenced attachment")
	if nm.calls != 0 {
		t.Errorf("the notes rule ran for a file that is not a note picture")
	}
}

func TestNoteImageAccess_CoDMReachesTheNotesRuleAsGM(t *testing.T) {
	nm := &fakeNoteMedia{readers: map[string]bool{noteReaderID: true}}
	h, _ := noteImageHandler(nm, &fakeEntityVisibilityFilter{}, nil)
	h.memberChecker = &stubMemberChecker{
		members:   map[string]map[string]bool{testCampaignID: {noteReaderID: true, noteAuthorID: true}},
		dmGranted: map[string]map[string]bool{testCampaignID: {noteReaderID: true}},
	}
	mustAllow(t, h.checkMediaAccess(newADR058TestContext(noteReaderID), noteImageFile(), false, ""), "co-DM")
	if nm.role < 3 {
		t.Errorf("the notes rule saw role %d, want the promoted owner role so GM-shared notes count", nm.role)
	}
}

func TestNoteImage_WithoutACampaignIsGone(t *testing.T) {
	h, _ := noteImageHandler(&fakeNoteMedia{readers: map[string]bool{noteAuthorID: true}}, &fakeEntityVisibilityFilter{}, nil)
	f := noteImageFile()
	f.CampaignID = nil
	f.CampaignIsPublic = nil
	mustDeny(t, h.checkMediaAccess(newADR058TestContext(noteAuthorID), f, false, ""), "note picture whose campaign was deleted")
}
