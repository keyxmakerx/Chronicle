package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestCtaTrigger(t *testing.T) {
	tests := []struct {
		name   string
		seeded bool
		want   string
	}{
		{"unseeded asks on load and every minute", false, "load, every 60s"},
		{"seeded only polls", true, "every 60s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ctaTrigger(tt.seeded); got != tt.want {
				t.Errorf("ctaTrigger(%v) = %q, want %q", tt.seeded, got, tt.want)
			}
		})
	}
}

func TestCalloutSeed(t *testing.T) {
	if body, ok := CalloutSeed(context.Background()); body != "" || ok {
		t.Errorf("a context without a seed = (%q, %v), want (\"\", false)", body, ok)
	}
	ctx := SetCalloutSeed(context.Background(), func() (string, bool) { return "<p>x</p>", true })
	if body, ok := CalloutSeed(ctx); body != "<p>x</p>" || !ok {
		t.Errorf("seeded = (%q, %v)", body, ok)
	}
}

// The notice bar is drawn into the page, not left for a poll to fill: a filled
// strip has its height from the first paint, and an empty known answer leaves
// it empty without a first fetch.
func TestAppDrawsTheNoticeBar(t *testing.T) {
	tests := []struct {
		name         string
		seed         func() (string, bool)
		wantInside   string
		wantTrigger  string
		wantNoTrig   string
		wantEmptyBar bool
	}{
		{"a bar to show", func() (string, bool) { return `<div class="cta-bar" data-cta>RSVP</div>`, true }, `data-cta>RSVP`, `hx-trigger="every 60s"`, `hx-trigger="load`, false},
		{"nothing to say, known", func() (string, bool) { return "", true }, "", `hx-trigger="every 60s"`, `hx-trigger="load`, true},
		{"could not be built, asked for after load", func() (string, bool) { return "", false }, "", `hx-trigger="load, every 60s"`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := SetIsAuthenticated(context.Background(), true)
			ctx = SetCampaignID(ctx, "c1")
			ctx = SetCalloutSeed(ctx, tt.seed)
			var buf bytes.Buffer
			if err := App("t").Render(templ.WithChildren(ctx, templ.NopComponent), &buf); err != nil {
				t.Fatalf("render: %v", err)
			}
			html := buf.String()
			i := strings.Index(html, `class="cta-host"`)
			if i < 0 {
				t.Fatalf("no notice bar strip in the layout")
			}
			strip := html[i:]
			strip = strip[:strings.Index(strip, "</div>")+len("</div>")]
			if !strings.Contains(strip, tt.wantTrigger) {
				t.Errorf("strip = %s, want %s", strip, tt.wantTrigger)
			}
			if tt.wantNoTrig != "" && strings.Contains(strip, tt.wantNoTrig) {
				t.Errorf("strip = %s, must not contain %s", strip, tt.wantNoTrig)
			}
			if tt.wantInside != "" && !strings.Contains(strip, tt.wantInside) {
				t.Errorf("strip = %s, want it to hold %s", strip, tt.wantInside)
			}
			if tt.wantEmptyBar && !strings.HasSuffix(strip, "></div>") {
				t.Errorf("strip = %s, want an empty strip", strip)
			}
		})
	}
}
