// Package media -- signed_url.go implements HMAC-SHA256 signed media URLs.
// Signed URLs prevent permanent, irrevocable access to media files by
// requiring a time-limited cryptographic token. This mirrors the approach
// used by AWS S3, Google Cloud Storage, and Cloudflare R2 presigned URLs.
package media

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
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

// Sign generates a signed URL path for a media file with the given TTL.
// The returned path includes ?expires= and &sig= query parameters.
func (s *URLSigner) Sign(fileID string, ttl time.Duration) string {
	expires := time.Now().Add(ttl).Unix()
	sig := s.computeSignature(fileID, expires)
	return fmt.Sprintf("/media/%s?expires=%d&sig=%s", fileID, expires, sig)
}

// SignThumb generates a signed URL path for a media thumbnail.
func (s *URLSigner) SignThumb(fileID, size string, ttl time.Duration) string {
	expires := time.Now().Add(ttl).Unix()
	// Include size in the signed payload to prevent size parameter tampering.
	sig := s.computeThumbSignature(fileID, size, expires)
	return fmt.Sprintf("/media/%s/thumb/%s?expires=%d&sig=%s", fileID, size, expires, sig)
}

// Verify checks that a signature is valid and not expired.
// Uses hmac.Equal for constant-time comparison to prevent timing attacks.
func (s *URLSigner) Verify(fileID string, expiresStr, signature string) bool {
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expires {
		return false
	}
	expected := s.computeSignature(fileID, expires)
	return hmac.Equal([]byte(signature), []byte(expected))
}

// VerifyThumb checks a thumbnail signature including the size parameter.
func (s *URLSigner) VerifyThumb(fileID, size string, expiresStr, signature string) bool {
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expires {
		return false
	}
	expected := s.computeThumbSignature(fileID, size, expires)
	return hmac.Equal([]byte(signature), []byte(expected))
}

// computeSignature creates an HMAC-SHA256 hex digest over "{fileID}:{expires}".
func (s *URLSigner) computeSignature(fileID string, expires int64) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = fmt.Fprintf(mac, "%s:%d", fileID, expires)
	return hex.EncodeToString(mac.Sum(nil))
}

// computeThumbSignature includes the size to prevent size parameter tampering.
func (s *URLSigner) computeThumbSignature(fileID, size string, expires int64) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = fmt.Fprintf(mac, "%s:%s:%d", fileID, size, expires)
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

	// SecretGeneratedInMemory — generated this boot, persistence
	// failed (data dir not writable, or path empty). DANGER: every
	// restart will silently invalidate every Foundry manifest
	// token. This is the Issue #17 mode.
	SecretGeneratedInMemory SigningSecretSource = "generated_in_memory"
)

// LoadOrInitSigningSecret resolves the HMAC signing secret used by
// both the media URLSigner and the foundry_vtt TokenSigner.
// Priority:
//
//  1. envSecret if non-empty (operator set MEDIA_SIGNING_SECRET).
//  2. The persisted file at path if it exists.
//  3. A freshly generated secret, persisted to path. If persist
//     fails, the secret is still returned but flagged in-memory.
//
// Persistence is load-bearing: the foundry_vtt TokenSigner uses this
// secret as its HMAC key, and Foundry stores manifest URLs (which
// embed tokens signed with this secret) indefinitely. Without
// persistence, restart → new secret → every outstanding manifest
// token 403s — the symptom diagnosed in cordinator Issue #17.
//
// Pass path="" to disable persistence (test-only; production should
// always pass a path).
//
// Returns (secret, source, error). A non-nil error never means the
// secret is unusable — it's a soft signal that something prevented
// persistence (most often "data dir not writable"). The caller
// should log the source + error to the operator either way.
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
