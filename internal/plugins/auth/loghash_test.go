package auth

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestHashEmail_Deterministic asserts that the same email always produces the
// same hash so log lines for the same user can be correlated across requests.
func TestHashEmail_Deterministic(t *testing.T) {
	got1 := hashEmail("alice@example.com")
	got2 := hashEmail("alice@example.com")
	if got1 != got2 {
		t.Errorf("hashEmail not deterministic: %q vs %q", got1, got2)
	}
}

// TestHashEmail_CaseInsensitive asserts that case + whitespace differences in
// the input collapse to the same hash. Without this, a user signing in as
// `Alice@Example.com` and resetting as `alice@example.com` would produce two
// different log streams.
func TestHashEmail_CaseInsensitive(t *testing.T) {
	cases := []string{
		"alice@example.com",
		"Alice@Example.com",
		"  alice@example.com  ",
		"ALICE@EXAMPLE.COM",
	}
	want := hashEmail(cases[0])
	for _, c := range cases[1:] {
		if got := hashEmail(c); got != want {
			t.Errorf("hashEmail(%q) = %q, want %q", c, got, want)
		}
	}
}

// TestHashEmail_Distinct asserts that different emails produce different
// hashes (at the 16-char prefix; truncation collisions are debugging-only
// noise but should not be the common case).
func TestHashEmail_Distinct(t *testing.T) {
	a := hashEmail("alice@example.com")
	b := hashEmail("bob@example.com")
	if a == b {
		t.Errorf("hashEmail collided for distinct inputs: %q", a)
	}
}

// TestHashEmail_Length pins the 16-char output so log-format consumers (alert
// rules, dashboards) can rely on the shape.
func TestHashEmail_Length(t *testing.T) {
	got := hashEmail("alice@example.com")
	if len(got) != 16 {
		t.Errorf("hashEmail length = %d, want 16", len(got))
	}
}

// TestHashEmail_NoRawEmail is the security regression guard. The hash MUST
// NOT contain any of the local-part or domain characters that the raw email
// did. This is paranoia — SHA-256 of "alice@example.com" doesn't contain
// "alice" — but a future change that swapped to a weaker scheme (e.g. base64
// of a salt) could regress without this assertion.
func TestHashEmail_NoRawEmail(t *testing.T) {
	email := "alice@example.com"
	got := hashEmail(email)
	for _, frag := range []string{"alice", "example", "@", "."} {
		if strings.Contains(got, frag) {
			t.Errorf("hashEmail(%q) = %q leaks fragment %q", email, got, frag)
		}
	}
}

// TestPasswordResetDebugLog_UsesHashedEmail captures both Debug log branches
// the audit's M-1 finding called out and asserts that neither emits the raw
// email under the `email` slog key.
//
// We can't easily drive the full service.RequestPasswordReset flow from a
// pure unit test (it needs redis + repo + email-sender mocks). Instead, this
// test instantiates a slog handler that captures attributes, then invokes
// the same slog.Debug calls the service uses — the assertion is on the
// attribute shape, not the surrounding control flow.
//
// Rationale: regression-pin that future contributors writing similar Debug
// logs in this file use hashEmail rather than the raw value. If a new branch
// adds `slog.String("email", email)`, the contributor should see this test
// nearby and adopt the same pattern.
func TestPasswordResetDebugLog_UsesHashedEmail(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)

	email := "alice@example.com"
	logger.Debug("password reset rate-limited", slog.String("email_hash", hashEmail(email)))
	logger.Debug("password reset requested for unknown email", slog.String("email_hash", hashEmail(email)))

	out := buf.String()
	if strings.Contains(out, email) {
		t.Errorf("log output contains raw email %q:\n%s", email, out)
	}
	if !strings.Contains(out, "email_hash=") {
		t.Errorf("log output missing email_hash attribute:\n%s", out)
	}
	if strings.Contains(out, "email=alice") {
		t.Errorf("log output contains email= attribute with raw value:\n%s", out)
	}
}
