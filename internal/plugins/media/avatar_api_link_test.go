// avatar_api_link_test.go pins the one way a cookieless caller (the Foundry
// module) may fetch a campaign-less avatar: a thumbnail link signed for a
// campaign whose members include the avatar's owner. Nothing else campaign-less
// opens up.

package media

import (
	"testing"
	"time"
)

func TestCheckMediaAccess_AvatarThumbForAPIKeyCampaign(t *testing.T) {
	members := map[string]map[string]bool{"camp-1": {"alice": true}}
	avatar := func(owner string) *MediaFile {
		return &MediaFile{ID: "file-av", UsageType: UsageAvatar, UploadedBy: owner}
	}
	attachment := &MediaFile{ID: "file-av", UsageType: UsageAttachment, UploadedBy: "alice"}

	// mint returns the query a link signed for campaign would carry.
	mint := func(h *Handler, fileID, size, campaign string) map[string]string {
		expires, sig := signThumbURL(t, h.signer, fileID, size, ViewerAPIKeyCampaign(campaign), time.Hour)
		return map[string]string{"expires": expires, "sig": sig, AvatarCampaignParam: campaign}
	}

	cases := []struct {
		name  string
		file  *MediaFile
		thumb bool
		query func(h *Handler) map[string]string
		allow bool
	}{
		{"unsigned", avatar("alice"), true, func(*Handler) map[string]string { return nil }, false},
		{"valid link, owner is a member", avatar("alice"), true,
			func(h *Handler) map[string]string { return mint(h, "file-av", "300", "camp-1") }, true},
		{"valid link, owner not in that campaign", avatar("bob"), true,
			func(h *Handler) map[string]string { return mint(h, "file-av", "300", "camp-1") }, false},
		{"link signed for a campaign the owner is not in", avatar("alice"), true,
			func(h *Handler) map[string]string { return mint(h, "file-av", "300", "camp-2") }, false},
		{"campaign param swapped after signing", avatar("alice"), true,
			func(h *Handler) map[string]string {
				q := mint(h, "file-av", "300", "camp-2")
				q[AvatarCampaignParam] = "camp-1"
				return q
			}, false},
		{"original, not a thumbnail", avatar("alice"), false,
			func(h *Handler) map[string]string { return mint(h, "file-av", "300", "camp-1") }, false},
		{"plain API-key link is not enough", avatar("alice"), true,
			func(h *Handler) map[string]string {
				expires, sig := signThumbURL(t, h.signer, "file-av", "300", ViewerAPIKey, time.Hour)
				return map[string]string{"expires": expires, "sig": sig, AvatarCampaignParam: "camp-1"}
			}, false},
		{"expired link", avatar("alice"), true,
			func(h *Handler) map[string]string {
				expires, sig := signThumbURL(t, h.signer, "file-av", "300", ViewerAPIKeyCampaign("camp-1"), -time.Minute)
				return map[string]string{"expires": expires, "sig": sig, AvatarCampaignParam: "camp-1"}
			}, false},
		{"valid link for a non-avatar campaign-less file", attachment, true,
			func(h *Handler) map[string]string { return mint(h, "file-av", "300", "camp-1") }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler("test-secret", members)
			c := newAccessTestContext(tc.query(h), nil)
			size := ""
			if tc.thumb {
				size = "300"
			}
			err := h.checkMediaAccess(c, tc.file, tc.thumb, size)
			if tc.allow {
				mustAllow(t, err, tc.name)
			} else {
				mustDeny(t, err, tc.name)
			}
		})
	}
}

func TestSignAvatarThumb_RoundTrip(t *testing.T) {
	s := NewURLSigner("test-secret")
	if got := s.SignAvatarThumb("f", "300", "c1", time.Hour); got == "" {
		t.Fatal("empty link")
	}
	expires, sig := signThumbURL(t, s, "f", "300", ViewerAPIKeyCampaign("c1"), time.Hour)
	if !s.VerifyAvatarThumbLink("f", "300", "c1", expires, sig) {
		t.Error("a link minted for c1 must verify for c1")
	}
	if s.VerifyAvatarThumbLink("f", "300", "c2", expires, sig) {
		t.Error("a link minted for c1 must not verify for c2")
	}
	if s.VerifyAvatarThumbLink("f", "800", "c1", expires, sig) {
		t.Error("a link minted for 300 must not verify for 800")
	}
}
