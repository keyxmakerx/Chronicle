package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestBase_RevealsBeforeDeferredScripts pins that the page is shown once its
// HTML and CSS are in, not after every deferred script has downloaded and run:
// waiting for DOMContentLoaded kept a phone's screen white for seconds.
func TestBase_RevealsBeforeDeferredScripts(t *testing.T) {
	var buf bytes.Buffer
	if err := Base("t").Render(templ.WithChildren(context.Background(), templ.Raw(`<aside id="sidebar"></aside>`)), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	reveal := strings.Index(html, "document.body.style.opacity='1'")
	if reveal < 0 {
		t.Fatal("the page must reveal itself")
	}
	if strings.Contains(html, "DOMContentLoaded',function(){document.body.style.opacity") {
		t.Error("the reveal must not wait for DOMContentLoaded")
	}
	body := strings.Index(html, "<body")
	firstDeferred := strings.Index(html, " defer>")
	if reveal < body || firstDeferred < 0 || reveal > firstDeferred {
		t.Errorf("the reveal must sit in the body before the first deferred script (body %d, reveal %d, deferred %d)", body, reveal, firstDeferred)
	}
	if !strings.Contains(html[reveal-600:reveal], "alpine:init") {
		t.Error("the sidebar's saved state must be handed back to Alpine when it starts")
	}
}
