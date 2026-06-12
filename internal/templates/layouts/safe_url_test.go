package layouts

import (
	"context"
	"strings"
	"testing"
)

// TestSafeExternalURL_Helper: the render-time guard (audit-R2 Finding 1) maps a
// dangerous owner URL to "#" while preserving valid ones.
func TestSafeExternalURL_Helper(t *testing.T) {
	for _, c := range []struct {
		in, want string
	}{
		{"javascript:alert(1)", "#"},
		{" javascript:alert(1)", "#"},
		{"data:text/html,x", "#"},
		{"//evil.com", "#"},
		{"https://example.com/x", "https://example.com/x"},
		{"/campaigns/abc", "/campaigns/abc"},
	} {
		if got := string(safeExternalURL(c.in)); got != c.want {
			t.Errorf("safeExternalURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCustomNavLink_PoisonedURLRendersHash: a link row stored BEFORE the ingress
// guard must still never render a javascript: href — it falls back to "#".
func TestCustomNavLink_PoisonedURLRendersHash(t *testing.T) {
	var sb strings.Builder
	if err := customNavLink(SidebarLink{Label: "Evil", URL: "javascript:alert(document.cookie)"}).
		Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if strings.Contains(html, "javascript:") {
		t.Errorf("poisoned URL must not render as javascript:; got %q", html)
	}
	if !strings.Contains(html, `href="#"`) {
		t.Errorf("poisoned URL should fall back to href=\"#\"; got %q", html)
	}
}
