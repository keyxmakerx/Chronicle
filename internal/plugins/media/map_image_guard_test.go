package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// fakeMapImageGuard answers the maps plugin's question from a fixed table.
type fakeMapImageGuard struct {
	hidden bool
	isPic  bool
	err    error
	asked  int
}

func (f *fakeMapImageGuard) IsShadowedMapImage(context.Context, string, string) (bool, error) {
	f.asked++
	return f.hidden, f.err
}

func (f *fakeMapImageGuard) IsMapPicture(context.Context, string, string) (bool, error) {
	return f.isPic || f.hidden, f.err
}

// The original (and every thumbnail) of a map picture that has a shadow is
// refused to anyone who may not see under the shadows, even behind a valid
// signature, and fails closed when the check itself breaks.
func TestCheckMediaAccess_ShadowedMapPicture(t *testing.T) {
	const camp = "camp-1"
	signer := NewURLSigner("test-secret")
	apiExp, apiSig := signMediaURL(t, signer, "file-1", ViewerAPIKey, time.Minute)
	anonExp, anonSig := signMediaURL(t, signer, "file-1", ViewerAnonymous, time.Minute)
	sessExp, sessSig := signMediaURL(t, signer, "file-1", ViewerSession("u"), time.Minute)
	apiThumbExp, apiThumbSig := signThumbURL(t, signer, "file-1", "300", ViewerAPIKey, time.Minute)

	roles := map[string]map[string]int{camp: {"player": 1, "scribe": 2, "owner": 3, "codm": 2}}
	checker := &stubMemberChecker{
		members:   map[string]map[string]bool{camp: {"player": true, "scribe": true, "owner": true, "codm": true}},
		roles:     roles,
		dmGranted: map[string]map[string]bool{camp: {"codm": true}},
	}

	cases := []struct {
		name    string
		hidden  bool
		err     error
		user    string // "" = no session
		query   map[string]string
		thumb   bool
		allowed bool
	}{
		{"not a shadowed picture: player allowed", false, nil, "player", nil, false, true},
		{"player refused", true, nil, "player", nil, false, false},
		{"scribe refused like a player", true, nil, "scribe", nil, false, false},
		{"owner allowed", true, nil, "owner", nil, false, true},
		{"co-DM grant allowed", true, nil, "codm", nil, false, true},
		{"player refused even with a valid session-bound signature", true, nil, "player", map[string]string{"expires": sessExp, "sig": sessSig}, false, false},
		{"anonymous refused", true, nil, "", nil, false, false},
		{"anonymous-bound signed link refused", true, nil, "", map[string]string{"expires": anonExp, "sig": anonSig}, false, false},
		{"api-key link allowed (owner's sync module)", true, nil, "", map[string]string{"expires": apiExp, "sig": apiSig}, false, true},
		{"api-key thumbnail link allowed", true, nil, "", map[string]string{"expires": apiThumbExp, "sig": apiThumbSig}, true, true},
		{"api-key link presented by a player's session refused", true, nil, "player", map[string]string{"expires": apiExp, "sig": apiSig}, false, false},
		{"api-key original link does not open a thumbnail", true, nil, "", map[string]string{"expires": apiExp, "sig": apiSig}, true, false},
		{"check error refuses an owner", true, errors.New("db down"), "owner", nil, false, false},
		{"check error refuses an api-key link", true, errors.New("db down"), "", map[string]string{"expires": apiExp, "sig": apiSig}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{signer: signer, memberChecker: checker, mapImages: &fakeMapImageGuard{hidden: tc.hidden, err: tc.err}}
			var sess *auth.Session
			if tc.user != "" {
				sess = &auth.Session{UserID: tc.user}
			}
			c := newAccessTestContext(tc.query, sess)
			size := ""
			if tc.thumb {
				size = "300"
			}
			err := h.checkMapImageGuard(c, privateMediaFile(), tc.thumb, size)
			if tc.allowed {
				mustAllow(t, err, tc.name)
			} else {
				mustDeny(t, err, tc.name)
			}
		})
	}
}

// A validly signed URL skips the base membership checks, so the picture rule has
// to sit after them in checkMediaAccess, not beside the membership branch.
func TestCheckMediaAccess_SignedURLDoesNotBypassMapPictureRule(t *testing.T) {
	signer := NewURLSigner("test-secret")
	exp, sig := signMediaURL(t, signer, "file-1", ViewerSession("player"), time.Minute)
	checker := &stubMemberChecker{members: map[string]map[string]bool{"camp-1": {"player": true}}}
	h := &Handler{signer: signer, memberChecker: checker, service: &fakeAccessMediaService{},
		entityVisibility: &fakeEntityVisibilityFilter{}, mapImages: &fakeMapImageGuard{hidden: true}}
	c := newAccessTestContext(map[string]string{"expires": exp, "sig": sig}, &auth.Session{UserID: "player"})
	mustDeny(t, h.checkMediaAccess(c, privateMediaFile(), false, ""), "signed player link to a shadowed map picture")

	h.mapImages = &fakeMapImageGuard{hidden: false}
	mustAllow(t, h.checkMediaAccess(c, privateMediaFile(), false, ""), "same link once no shadow covers the map")
}

// Without a guard the handler behaves as before, and files with no campaign
// are never sent to it.
func TestCheckMapImageGuard_Unwired(t *testing.T) {
	h := &Handler{}
	mustAllow(t, h.checkMapImageGuard(newAccessTestContext(nil, nil), privateMediaFile(), false, ""), "unwired guard")
	if h.MapImageGuardWired() {
		t.Error("MapImageGuardWired must be false when unset")
	}
	g := &fakeMapImageGuard{hidden: true}
	h.SetMapImageGuard(g)
	if !h.MapImageGuardWired() {
		t.Error("MapImageGuardWired must be true once set")
	}
	mustAllow(t, h.checkMapImageGuard(newAccessTestContext(nil, nil), &MediaFile{ID: "avatar", UsageType: UsageAvatar}, false, ""), "campaignless file")
	if g.asked != 0 {
		t.Error("a file with no campaign must not be sent to the guard")
	}
}
