// alert_classes_test.go pins that the login success banner uses Chronicle's
// shared alert-success class (matching the alert-error class login's own
// error message already used) instead of hand-written Tailwind, so a color
// tweak or dark-mode fix only has to be made once.

package auth

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestLoginPage_SuccessBannerUsesSharedAlertClass pins that the success
// banner is styled with .alert-success (plus the bottom margin it needs
// alongside the shared class, since alert-success itself carries none) and
// no longer with hand-written green Tailwind utilities.
func TestLoginPage_SuccessBannerUsesSharedAlertClass(t *testing.T) {
	component := LoginPage("csrf", "", "", "Password updated", "")

	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, `class="alert-success mb-4"`) {
		t.Errorf("success banner must use the shared alert-success class (with its bottom margin); got: %s", html)
	}
	if strings.Contains(html, "bg-green-50") {
		t.Errorf("success banner must not hand-write its colors; bg-green-50 has no dark-mode pair")
	}
}
