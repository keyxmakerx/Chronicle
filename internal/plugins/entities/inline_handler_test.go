package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"golang.org/x/net/html"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// attrsOf parses `<b onclick="CALL">` and returns the element's attributes, so
// a handler that ends its attribute early shows up as extra attributes or a
// different value.
func attrsOf(t *testing.T, call templ.ComponentScript) []html.Attribute {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(`<b onclick="` + call.Call + `"></b>`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found []html.Attribute
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "b" {
			found = n.Attr
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func TestInlineHandlersStayInsideTheirAttribute(t *testing.T) {
	tests := []struct {
		name string
		call templ.ComponentScript
		want string // a fragment the browser must hand to the JS intact
	}{
		{"claim submit", claimSubmitCall, `'<span class="fx-spin" aria-hidden="true"></span>`},
		{"claim retry", claimRetryCall, `f.requestSubmit()`},
		{"cast pick", castPickCall, `'<i class="fa-solid fa-eye"></i><span></span>'`},
		{"cast search", castSearchCall, `rows[k].style.display=m?'':'none'`},
		{"cast fold", castFoldCall("add"), `})(this,'add')`},
		{"cast finder", castFinderCall("give"), `})(this,'give')`},
		{"cast ask claim", castAskClaimCall, `(function(`},
		{"cast cancel claim", castCancelClaimCall, `(function(`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := attrsOf(t, tt.call)
			if len(attrs) != 1 || attrs[0].Key != "onclick" {
				t.Fatalf("handler leaked out of its attribute: %+v", attrs)
			}
			if !strings.Contains(attrs[0].Val, tt.want) {
				t.Errorf("parsed handler lost %q:\n%s", tt.want, attrs[0].Val)
			}
		})
	}
}

// The claim banner once printed its own submit script as page text, because
// the script's markup quotes closed the onsubmit attribute.
func TestClaimBannerPrintsNoScript(t *testing.T) {
	yes := true
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}}
	var buf bytes.Buffer
	err := claimBanner(cc, &Entity{ID: "e1"}, &EntityType{Claimable: &yes}, true, "", "tok").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	doc, err := html.Parse(&buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if got := text.String(); strings.Contains(got, "function") || strings.Contains(got, "apiFetch") {
		t.Errorf("banner text contains script:\n%s", got)
	}
}
