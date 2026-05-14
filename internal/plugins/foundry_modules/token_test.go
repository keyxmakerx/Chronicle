package foundry_modules

import (
	"strings"
	"testing"
)

// TestTokenSigner_RoundTrip pins the basic sign-then-verify path: a
// signer should be able to decode and recompute every token it
// produces. The campaign ID and token version must come back unchanged.
func TestTokenSigner_RoundTrip(t *testing.T) {
	s := NewTokenSigner("super-secret-key-32-bytes-long----")
	cases := []struct {
		campaignID string
		version    int
	}{
		{"camp-abc", 1},
		{"camp-with-dots.in.id", 7},
		{"camp-uuid-style-aaaaaaaaaaaa", 42},
	}
	for _, tc := range cases {
		t.Run(tc.campaignID, func(t *testing.T) {
			tok := s.Sign(tc.campaignID, tc.version)
			cid, ver, err := s.Verify(tok)
			if err != nil {
				t.Fatalf("Verify returned %v", err)
			}
			if cid != tc.campaignID {
				t.Errorf("campaign id: got %q, want %q", cid, tc.campaignID)
			}
			if ver != tc.version {
				t.Errorf("token version: got %d, want %d", ver, tc.version)
			}
		})
	}
}

// TestTokenSigner_VersionRotationInvalidates is the rotation-revocation
// regression guard: after rotation, the OLD token's HMAC still verifies
// (the signer is the same), but the embedded version no longer matches
// the rotated value — the service layer's secondary check is what
// actually rejects the token. This test focuses on the signer half;
// the service-level test covers the DB-version mismatch.
func TestTokenSigner_VersionRotationInvalidates(t *testing.T) {
	s := NewTokenSigner("super-secret-key-32-bytes-long----")
	old := s.Sign("camp-1", 1)
	newer := s.Sign("camp-1", 2)
	if old == newer {
		t.Fatal("rotation should produce a distinct token")
	}
	_, ver, err := s.Verify(old)
	if err != nil {
		t.Fatalf("old token should still HMAC-verify: %v", err)
	}
	if ver != 1 {
		t.Errorf("old token version should still decode as 1, got %d", ver)
	}
}

// TestTokenSigner_TamperedTokenRejected covers the bad-actor case:
// modifying any segment of the encoded token should yield
// ErrInvalidToken, not a partial decode.
func TestTokenSigner_TamperedTokenRejected(t *testing.T) {
	s := NewTokenSigner("super-secret-key-32-bytes-long----")
	tok := s.Sign("camp-1", 1)

	// Flip a character mid-token. The base64 decode may still succeed
	// but the inner HMAC won't.
	tampered := tok
	if len(tok) > 5 {
		tampered = tok[:len(tok)-1] + "A"
		if tampered == tok {
			tampered = tok[:len(tok)-1] + "B"
		}
	}
	if _, _, err := s.Verify(tampered); err == nil {
		t.Error("tampered token should fail verify")
	}

	// Non-base64 garbage.
	if _, _, err := s.Verify("not!!base64???"); err == nil {
		t.Error("garbage token should fail verify")
	}

	// Wrong-shape (wrong number of dotted segments under the b64).
	bogus := strings.Repeat("a", 16)
	if _, _, err := s.Verify(bogus); err == nil {
		t.Error("malformed-shape token should fail verify")
	}
}

// TestTokenSigner_DifferentSecretsDontVerify guards against signing-
// secret rotation: a token signed with secret A must not verify under
// a signer built with secret B.
func TestTokenSigner_DifferentSecretsDontVerify(t *testing.T) {
	a := NewTokenSigner("key-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	b := NewTokenSigner("key-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	tok := a.Sign("camp-1", 1)
	if _, _, err := b.Verify(tok); err == nil {
		t.Error("token signed by A should not verify under B")
	}
}
