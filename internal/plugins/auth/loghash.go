// loghash provides email hashing for debug-log correlation.
// Per cordinator/reports/chronicle/2026-05-22-c-security-audit.md §2 M-1,
// password-reset Debug log branches previously included raw email addresses;
// a log-shipping operator could distinguish "known but rate-limited" from
// "unknown email" branches and enumerate registered emails. This file's
// helper replaces the raw email with a SHA-256 hex digest truncated to 16
// characters — enough uniqueness for correlation across related log lines
// without exposing the address itself.
//
// The hash is for DEBUG correlation only. It is NOT a credential, NOT
// stored, and NOT used for any security decision. Truncation is acceptable
// because two emails colliding at the prefix only confuses one operator's
// debugging session, not any security boundary.
package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// hashEmail returns a 16-character SHA-256 hex prefix of the lowercased
// email address. Used to make password-reset Debug logs preserve their
// differential (rate-limited vs unknown email) for operator debugging
// while preventing raw email addresses from reaching log shipment paths.
func hashEmail(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h[:])[:16]
}
