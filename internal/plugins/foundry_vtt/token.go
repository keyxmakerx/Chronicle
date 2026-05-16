package foundry_vtt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// tokenDomain is the HMAC payload prefix that domain-separates this
// plugin's manifest tokens from any other HMAC the same secret might
// sign (currently the media URLSigner; future signers should each
// pick a distinct domain string).
//
// Historical note: during the C-FMC-5b parallel period, the deleted
// foundry_modules plugin signed with a "foundry-module" prefix.
// Tokens stored by Foundry clients from that era were intentionally
// designed to NOT verify against this plugin's endpoint, and vice
// versa — operators minted fresh foundry-vtt-domain tokens via the
// new owner tab. foundry_modules was deleted in C-FMC-5c so the
// old domain is no longer in use; this comment preserved as
// attribution for the chosen prefix.
const tokenDomain = "foundry-vtt"

// TokenSigner mints and verifies the per-campaign manifest URL tokens
// Foundry sends back on every update check. The token bakes in the
// campaign's current token_version, so rotation = bump the counter,
// and every previously-issued token stops verifying.
//
// Token wire shape: base64url("{campaignID}.{tokenVersion}.{sig}")
// where sig = HMAC-SHA256(secret, "foundry-vtt:{campaignID}:{tokenVersion}").
//
// No expiry. Foundry stores the install URL at install time and
// re-uses it forever; revocation goes through token-version rotation.
//
// Ported from foundry_modules/token.go with only the domain prefix
// changed. Wire format and verification logic are identical so
// porting a test fixture between plugins is straightforward.
type TokenSigner struct {
	secret []byte
}

// NewTokenSigner constructs a signer with the given HMAC key. The
// secret is shared with the media URLSigner — the tokenDomain
// prefix keeps the two signers signature-incompatible so a media
// signature can't be replayed as a foundry-vtt token (or vice versa).
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

// errInvalidToken is returned by Verify when the token can't be
// decoded, has the wrong shape, or fails the HMAC check. Kept as a
// single error type because clients (Foundry) shouldn't see the
// difference — malformed and forged both surface as errInvalidToken
// (Auth/invalid_token).
var errInvalidToken = errors.New("invalid module token")

// Verify decodes the token, recomputes the HMAC, and returns the
// embedded (campaignID, tokenVersion) on success. The caller must
// still confirm the embedded tokenVersion matches the repository's
// current value — Verify only proves the token was signed by this
// server, not that it hasn't been rotated.
func (t *TokenSigner) Verify(token string) (campaignID string, tokenVersion int, err error) {
	raw, decErr := base64.RawURLEncoding.DecodeString(token)
	if decErr != nil {
		return "", 0, errInvalidToken
	}
	// Split from the right so a campaign ID containing dots doesn't
	// throw off the parser. The last two segments are always
	// (tokenVersion, sig); everything before is the campaign ID.
	parts := strings.Split(string(raw), ".")
	if len(parts) < 3 {
		return "", 0, errInvalidToken
	}
	sigPart := parts[len(parts)-1]
	verPart := parts[len(parts)-2]
	campaignID = strings.Join(parts[:len(parts)-2], ".")
	tv, atoiErr := strconv.Atoi(verPart)
	if atoiErr != nil {
		return "", 0, errInvalidToken
	}
	expected := t.compute(campaignID, tv)
	if !hmac.Equal([]byte(sigPart), []byte(expected)) {
		return "", 0, errInvalidToken
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
