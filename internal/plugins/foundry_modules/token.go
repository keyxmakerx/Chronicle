package foundry_modules

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// tokenDomain is the HMAC payload prefix that domain-separates Foundry
// module tokens from any other HMAC the same secret might sign (e.g.
// the media URL signer). Defense against signature-replay across
// signers should one of the other signers leak.
const tokenDomain = "foundry-module"

// TokenSigner mints and verifies the per-campaign manifest URL tokens
// Foundry sends back on every update check. The token bakes in the
// campaign's current token_version, so rotation = bump the counter,
// and every previously-issued token stops verifying.
//
// Token wire shape: base64url("{campaignID}.{tokenVersion}.{sig}")
// where sig = HMAC-SHA256(secret, "{tokenDomain}:{campaignID}:{tokenVersion}").
//
// No expiry. Foundry stores the install URL at install time and
// re-uses it forever; revocation goes through token-version rotation.
type TokenSigner struct {
	secret []byte
}

// NewTokenSigner constructs a signer with the given HMAC key. The
// secret is shared with the media URLSigner — the tokenDomain prefix
// keeps the two signers from being able to validate each other's
// signatures.
func NewTokenSigner(secret string) *TokenSigner {
	return &TokenSigner{secret: []byte(secret)}
}

// Sign returns the URL-safe token string for the (campaignID,
// tokenVersion) pair. Caller hands this verbatim to Foundry as the
// ?token= query param on the manifest URL.
func (t *TokenSigner) Sign(campaignID string, tokenVersion int) string {
	sig := t.compute(campaignID, tokenVersion)
	raw := fmt.Sprintf("%s.%d.%s", campaignID, tokenVersion, sig)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ErrInvalidToken is returned by Verify when the token can't be
// decoded, has the wrong shape, or fails the HMAC check. Kept as a
// single error type because clients (Foundry) shouldn't see the
// difference — a malformed token and a forged token are both 404s.
var ErrInvalidToken = errors.New("invalid module token")

// Verify decodes the token, recomputes the HMAC, and returns the
// embedded (campaignID, tokenVersion) on success. The caller must
// still confirm the embedded tokenVersion matches the repository's
// current value for that campaign — Verify only proves the token
// was signed by this server, not that it hasn't been rotated.
func (t *TokenSigner) Verify(token string) (campaignID string, tokenVersion int, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", 0, ErrInvalidToken
	}
	// Split from the right so a campaign ID containing dots doesn't
	// throw off the parser. The last two segments are always
	// (tokenVersion, sig); everything before is the campaign ID.
	parts := strings.Split(string(raw), ".")
	if len(parts) < 3 {
		return "", 0, ErrInvalidToken
	}
	sigPart := parts[len(parts)-1]
	verPart := parts[len(parts)-2]
	campaignID = strings.Join(parts[:len(parts)-2], ".")
	tv, err := strconv.Atoi(verPart)
	if err != nil {
		return "", 0, ErrInvalidToken
	}
	expected := t.compute(campaignID, tv)
	if !hmac.Equal([]byte(sigPart), []byte(expected)) {
		return "", 0, ErrInvalidToken
	}
	return campaignID, tv, nil
}

// compute is the inner HMAC. Hex-encoded so the resulting token is
// printable; base64 encoding wraps the whole "{cid}.{ver}.{sig}"
// blob so the on-wire form is URL-safe.
func (t *TokenSigner) compute(campaignID string, tokenVersion int) string {
	mac := hmac.New(sha256.New, t.secret)
	_, _ = fmt.Fprintf(mac, "%s:%s:%d", tokenDomain, campaignID, tokenVersion)
	return fmt.Sprintf("%x", mac.Sum(nil))
}
