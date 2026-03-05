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
	"fmt"
	"strconv"
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
