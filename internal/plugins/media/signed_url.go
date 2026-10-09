// Package media implements HMAC-SHA256 signed media URLs, giving time-limited
// access to media files instead of permanent, irrevocable links.
package media

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// URLSigner generates and verifies HMAC-SHA256 signed media URLs.
// The signing secret must be kept confidential -- anyone who knows
// it can forge valid signed URLs for any media file.
type URLSigner struct {
	secret []byte
}

// NewURLSigner creates a signer with the given secret key.
// The secret should be at least 32 bytes for adequate security.
func NewURLSigner(secret string) *URLSigner {
	return &URLSigner{secret: []byte(secret)}
}

// SignedURLTTL bounds how long a signed media URL remains valid after
// minting (ADR-058 decision 6). Kept short since viewer binding (see below)
// already limits a copied link to the viewer it was minted for; a tab left
// open past this window just needs a reload to re-mint.
const SignedURLTTL = 15 * time.Minute

// --- Viewer binding (ADR-058 decision 6) ---
//
// Sign/SignThumb fold a viewer identity into the signed payload, so Verify
// only matches for the same presented viewer — a copied link is inert for
// anyone else. See Verify's doc comment for the one deliberate exception
// (the Foundry cross-origin flow).
const (
	// ViewerAnonymous marks a link minted for (or presented by) a request
	// with no authenticated Chronicle session: a cookieless or
	// cross-origin request always presents as this.
	ViewerAnonymous = "anon"

	// ViewerAPIKey marks a link minted by a Bearer-token-authenticated
	// syncapi caller (e.g. Foundry VTT) rather than a browser session. A
	// single fixed sentinel, not one per key or campaign, since the
	// request that eventually presents the link (a cross-origin <img>
	// fetch) is cookieless and can never present anything more specific.
	ViewerAPIKey = "apikey"
)

// ViewerSession returns the viewer identity for a signed URL minted for an
// authenticated Chronicle browser session. Embedding the user id means the
// signature Verify computes for any OTHER user id -- or for an anonymous
// presentation -- will not match it.
func ViewerSession(userID string) string {
	return "session:" + userID
}

// Sign generates a signed URL path for a media file with the given TTL,
// binding it to viewer (ADR-058 decision 6) -- one of ViewerAnonymous,
// ViewerAPIKey, or ViewerSession(userID). The returned path includes
// ?expires= and &sig= query parameters; the viewer is NOT itself present in
// the URL -- it is folded into the signature, and Verify's caller supplies
// the PRESENTED viewer for comparison rather than reading it back out.
func (s *URLSigner) Sign(fileID, viewer string, ttl time.Duration) string {
	expires := time.Now().Add(ttl).Unix()
	sig := s.computeSignature(fileID, viewer, expires)
	return fmt.Sprintf("/media/%s?expires=%d&sig=%s", fileID, expires, sig)
}

// SignThumb generates a signed URL path for a media thumbnail, viewer-bound
// exactly like Sign.
func (s *URLSigner) SignThumb(fileID, size, viewer string, ttl time.Duration) string {
	expires := time.Now().Add(ttl).Unix()
	// Include size in the signed payload to prevent size parameter tampering.
	sig := s.computeThumbSignature(fileID, size, viewer, expires)
	return fmt.Sprintf("/media/%s/thumb/%s?expires=%d&sig=%s", fileID, size, expires, sig)
}

// Verify checks that a signature is valid, not expired, and was minted for
// the presented viewer (see media.currentViewerIdentity: a session's user
// id, or ViewerAnonymous when the request carries none). Uses hmac.Equal
// for constant-time comparison.
//
// The one deliberate exception: when the presented viewer is
// ViewerAnonymous, a signature minted for the fixed ViewerAPIKey sentinel
// also verifies — this is the Foundry cross-origin <img> flow, which is
// cookieless by construction (see TestCheckMediaAccess_ValidSignedURL_NoCookie_PrivateCampaign).
// It does not let a link minted for a specific session
// (ViewerSession(userID)) be replayed cookielessly: that signature was
// computed over "session:<id>", never over ViewerAPIKey.
func (s *URLSigner) Verify(fileID, viewer, expiresStr, signature string) bool {
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expires {
		return false
	}
	if hmac.Equal([]byte(signature), []byte(s.computeSignature(fileID, viewer, expires))) {
		return true
	}
	if viewer == ViewerAnonymous {
		return hmac.Equal([]byte(signature), []byte(s.computeSignature(fileID, ViewerAPIKey, expires)))
	}
	return false
}

// VerifyThumb checks a thumbnail signature including the size parameter,
// with the same viewer binding (and the same ViewerAPIKey/anonymous
// carve-out) as Verify.
func (s *URLSigner) VerifyThumb(fileID, size, viewer, expiresStr, signature string) bool {
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expires {
		return false
	}
	if hmac.Equal([]byte(signature), []byte(s.computeThumbSignature(fileID, size, viewer, expires))) {
		return true
	}
	if viewer == ViewerAnonymous {
		return hmac.Equal([]byte(signature), []byte(s.computeThumbSignature(fileID, size, ViewerAPIKey, expires)))
	}
	return false
}

// VerifyAPIKeyLink reports whether the signature was minted for the fixed
// ViewerAPIKey sentinel, and only that (unlike Verify it never accepts a link
// minted for a session or for anonymous viewers). It lets the shadowed-map
// picture rule tell the sync API's own links from everyone else's.
func (s *URLSigner) VerifyAPIKeyLink(fileID, expiresStr, signature string) bool {
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(s.computeSignature(fileID, ViewerAPIKey, expires)))
}

// VerifyThumbAPIKeyLink is VerifyAPIKeyLink for a thumbnail link.
func (s *URLSigner) VerifyThumbAPIKeyLink(fileID, size, expiresStr, signature string) bool {
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(s.computeThumbSignature(fileID, size, ViewerAPIKey, expires)))
}

// ViewerAPIKeyCampaign is the viewer identity of a campaign-less avatar link
// minted for a sync API key of campaignID. Unlike the single ViewerAPIKey
// sentinel it names the campaign, because an avatar belongs to no campaign
// and the only thing that may justify serving it to a cookieless caller is
// that the avatar's owner is a member of the key's campaign.
func ViewerAPIKeyCampaign(campaignID string) string {
	return "apikey-campaign:" + campaignID
}

// AvatarCampaignParam carries the campaign a SignAvatarThumb link was minted
// for. It sits outside the signature's input only as a lookup key: Verify
// recomputes the signature over it, so a tampered value fails.
const AvatarCampaignParam = "campaign"

// SignAvatarThumb signs a thumbnail URL of a campaign-less avatar for a sync
// API key of campaignID. Thumbnails only: the sync module needs a small
// picture, and the original is never offered this way.
func (s *URLSigner) SignAvatarThumb(fileID, size, campaignID string, ttl time.Duration) string {
	expires := time.Now().Add(ttl).Unix()
	sig := s.computeThumbSignature(fileID, size, ViewerAPIKeyCampaign(campaignID), expires)
	return fmt.Sprintf("/media/%s/thumb/%s?expires=%d&sig=%s&%s=%s",
		fileID, size, expires, sig, AvatarCampaignParam, url.QueryEscape(campaignID))
}

// VerifyAvatarThumbLink reports whether the signature was minted by
// SignAvatarThumb for exactly this file, size and campaign. It accepts no
// other viewer binding, so a session or plain API-key link never passes.
func (s *URLSigner) VerifyAvatarThumbLink(fileID, size, campaignID, expiresStr, signature string) bool {
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil || time.Now().Unix() > expires || campaignID == "" {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(s.computeThumbSignature(fileID, size, ViewerAPIKeyCampaign(campaignID), expires)))
}

// computeSignature creates an HMAC-SHA256 hex digest over
// "{fileID}:{viewer}:{expires}". Including viewer is decision 6's whole
// mechanism: two requests presenting different viewers for the same file
// and expiry produce different digests, so a link cannot be re-authorized
// for someone it was not minted for just by knowing fileID and expires.
func (s *URLSigner) computeSignature(fileID, viewer string, expires int64) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = fmt.Fprintf(mac, "%s:%s:%d", fileID, viewer, expires)
	return hex.EncodeToString(mac.Sum(nil))
}

// computeThumbSignature includes the size (to prevent size parameter
// tampering) and the viewer (decision 6), same rationale as
// computeSignature.
func (s *URLSigner) computeThumbSignature(fileID, size, viewer string, expires int64) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = fmt.Fprintf(mac, "%s:%s:%s:%d", fileID, size, viewer, expires)
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateSigningSecret creates a cryptographically random 32-byte hex string
// suitable for use as a MEDIA_SIGNING_SECRET. Called during first boot if
// no secret is configured.
func GenerateSigningSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating signing secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SigningSecretSource describes where LoadOrInitSigningSecret got
// its secret, so the caller can log the right warning. The signer
// itself doesn't care about provenance, but the operator does — an
// in-memory secret means every restart silently invalidates every
// outstanding Foundry manifest token (since foundry_vtt's
// TokenSigner shares this secret with the media URLSigner).
type SigningSecretSource string

const (
	// SecretFromEnv — secret came from the env var (operator-managed).
	// Subsequent restarts will read the same env. No persistence
	// concern.
	SecretFromEnv SigningSecretSource = "env"

	// SecretFromFile — secret was previously generated and persisted;
	// this boot read the persisted value. Restart-stable.
	SecretFromFile SigningSecretSource = "file"

	// SecretGeneratedAndPersisted — generated this boot AND written
	// to disk. Restart-stable from now on. Operator should still
	// switch to env-managed for production hygiene.
	SecretGeneratedAndPersisted SigningSecretSource = "generated_and_persisted"

	// SecretGeneratedInMemory — generated this boot, persistence failed
	// (data dir not writable, or path empty). DANGER: every restart will
	// silently invalidate every Foundry manifest token.
	SecretGeneratedInMemory SigningSecretSource = "generated_in_memory"
)

// LoadOrInitSigningSecret resolves the HMAC signing secret used by both the
// media URLSigner and the foundry_vtt TokenSigner. Priority: envSecret
// (MEDIA_SIGNING_SECRET) > persisted file at path > freshly generated and
// persisted. Persistence is load-bearing — Foundry stores manifest URLs
// with embedded tokens signed by this secret indefinitely, so an
// unpersisted secret invalidates every outstanding token on restart.
//
// Pass path="" to disable persistence (test-only). Returns (secret, source,
// error); a non-nil error is a soft signal persistence failed, not that the
// secret is unusable — log source + error to the operator either way.
func LoadOrInitSigningSecret(envSecret, path string) (string, SigningSecretSource, error) {
	if envSecret != "" {
		return envSecret, SecretFromEnv, nil
	}

	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			secret := strings.TrimSpace(string(b))
			if secret != "" {
				return secret, SecretFromFile, nil
			}
			// Empty file — fall through to regenerate. The file
			// will be overwritten with the new secret.
		} else if !errors.Is(err, fs.ErrNotExist) {
			// Read error other than "doesn't exist" — return it so
			// the caller can log it, but proceed to generate.
			generated, genErr := GenerateSigningSecret()
			if genErr != nil {
				return "", SecretGeneratedInMemory, fmt.Errorf("read %s: %w; then generate: %v", path, err, genErr)
			}
			return generated, SecretGeneratedInMemory, fmt.Errorf("read %s: %w", path, err)
		}
	}

	generated, err := GenerateSigningSecret()
	if err != nil {
		return "", SecretGeneratedInMemory, err
	}

	if path == "" {
		return generated, SecretGeneratedInMemory, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return generated, SecretGeneratedInMemory, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(generated), 0o600); err != nil {
		return generated, SecretGeneratedInMemory, fmt.Errorf("write %s: %w", path, err)
	}
	return generated, SecretGeneratedAndPersisted, nil
}
