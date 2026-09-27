// surface_accent_html_test.go pins that the Surface Accents card's inline
// handlers survive real HTML attribute encoding. templ writes
// ComponentScript.Call into onclick="…"/onchange="…" with no escaping of
// its own (generator.go's writeExpressionAttributeValueScript writes .Call
// raw), so a JS body containing a literal '"' — the CSS attribute selector
// '[data-widget="appearance-editor"]' did — closes the attribute early and
// truncates everything after it: every downstream handler on the card lost
// the rest of its body and threw a SyntaxError in a real browser. Parses
// the rendered card with golang.org/x/net/html, which decodes attribute
// entities the way a browser does, so a still-broken inlineHandler shows up
// here as a truncated or malformed value, not just as a raw '"' in the
// source.
package campaigns

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestSurfaceAccentHandlers_SurviveHTMLAttributeEncoding(t *testing.T) {
	cc := &CampaignContext{
		Campaign:   &Campaign{ID: "camp-1", Settings: `{"accent_color":"#6366f1","accent_surface_1":"#3b82f6"}`},
		MemberRole: RoleOwner,
	}
	var sb strings.Builder
	if err := appearanceTab(cc, "tok").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render appearanceTab: %v", err)
	}

	doc, err := html.Parse(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatalf("parse rendered HTML: %v", err)
	}

	nodePath, nodeErr := exec.LookPath("node")

	found := 0
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key != "onclick" && a.Key != "onchange" {
					continue
				}
				// Only the Surface Accents card's controls are under test
				// here; other cards (semanticAccentPicker) carry no inline
				// handlers at all, so this scoping is by attribute
				// presence, not by ancestor lookup.
				found++
				val := a.Val
				if !strings.HasPrefix(val, "(function(") {
					t.Errorf("%s[%s] must start with `(function(` (a complete IIFE); got prefix %q",
						n.Data, a.Key, firstN(val, 40))
				}
				if !strings.HasSuffix(val, "})()") {
					t.Errorf("%s[%s] must end with `})()` (the handler was not truncated by an unescaped attribute-closing quote); got suffix %q",
						n.Data, a.Key, lastN(val, 40))
				}
				if nodeErr == nil {
					assertJSCompiles(t, nodePath, val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Two rows (Surface A, Surface B) x (8 presets + 1 reset + 1 custom
	// onchange) = 20 — matches len(accentColorPresets)==8.
	want := 2 * (len(accentColorPresets) + 2)
	if found != want {
		t.Fatalf("expected %d onclick/onchange handlers on the Surface Accents card, found %d", want, found)
	}
	if nodeErr != nil {
		t.Logf("node not on PATH (%v); skipped the JS-compiles check (prefix/suffix checks above still ran)", nodeErr)
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// assertJSCompiles shells out to `node --check` on the decoded handler body.
// Only called when node was found on PATH — go test ./... runs before CI's
// Node setup step, and node isn't installed at all in every dev environment,
// so this is a bonus check layered on the prefix/suffix assertions above,
// never the only thing standing between a truncated handler and a green
// test. Writes to a real temp file rather than piping to stdin: node's
// --check reads its argument with a plain file read, which a pipe/fd special
// file doesn't always satisfy.
func assertJSCompiles(t *testing.T, nodePath, body string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "handler-*.js")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(body); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	cmd := exec.Command(nodePath, "--check", f.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("handler body does not compile as JS: %v\n%s\nbody:\n%s", err, out, body)
	}
}
