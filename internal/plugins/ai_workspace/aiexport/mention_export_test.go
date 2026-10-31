package aiexport

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

func TestHtmlToMarkdown_MentionsBecomePageLinks(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"editor mention", `<p>See <a data-mention-id="abc" href="/campaigns/c/entities/abc" data-entity-preview="/campaigns/c/entities/abc/preview">@Lyra Dawn</a> now.</p>`, "See @[Lyra Dawn] now."},
		{"plain link untouched", `<p><a href="https://example.com/x">site</a></p>`, "[site](https://example.com/x)"},
		{"brackets stripped", `<p><a data-mention-id="a" href="/x">@A [b]|c</a></p>`, "@[A bc]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			got, err := htmlToMarkdown(&in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A Safe export is a player's view: a mention of a page players can't open is
// written as "a hidden page", in page text and recaps alike, and GM-only
// parts of a recap go too. The director's modes keep both.
func TestRender_SafeHidesMentionsOfHiddenPages(t *testing.T) {
	const hidden, seen = "dddddddd-0000-4000-8000-000000000001", "dddddddd-0000-4000-8000-000000000002"
	text := `<p>Ask <a data-mention-id="` + seen + `">@Bram</a> about <a data-mention-id="` + hidden + `">@The vampire duke</a>. <span data-secret="true">He bites.</span></p>`
	ents := []entities.Entity{{ID: "e1", Name: "Inn", EntityTypeID: 1, EntryHTML: sp(text)}}
	types := []entities.EntityType{{ID: 1, Name: "Place"}}
	nights := []sessions.Session{{ID: "s1", Name: "One", Status: sessions.StatusCompleted, RecapHTML: sp(text)}}

	tests := []struct {
		name     string
		opts     Options
		wantDuke bool
		wantBram bool
	}{
		{"safe", Options{Privacy: PrivacyModeSafe, VisiblePages: map[string]bool{seen: true}}, false, true},
		{"safe with no visibility answer hides every mention", Options{Privacy: PrivacyModeSafe}, false, false},
		{"permitted", Options{Privacy: PrivacyModePermitted, VisiblePages: map[string]bool{seen: true}}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages, err := RenderEntities(context.Background(), ents, types, nil, nil, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			recaps, err := RenderSessions(context.Background(), nights, nil, nil, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			for _, got := range []string{pages, recaps} {
				if strings.Contains(got, "vampire duke") != tt.wantDuke {
					t.Errorf("hidden page named = %v, want %v:\n%s", !tt.wantDuke, tt.wantDuke, got)
				}
				if strings.Contains(got, "@[Bram]") != tt.wantBram {
					t.Errorf("visible page named = %v, want %v:\n%s", !tt.wantBram, tt.wantBram, got)
				}
				if !tt.wantDuke && (strings.Contains(got, "bites") || !strings.Contains(got, "a hidden page")) {
					t.Errorf("safe export kept GM-only text or lost the stand-in:\n%s", got)
				}
			}
		})
	}
}

type pageFilterStub struct {
	asked []string
	err   error
}

func (p *pageFilterStub) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, _ int, _ string) (map[string]bool, error) {
	p.asked = append(p.asked, ids...)
	if p.err != nil {
		return nil, p.err
	}
	return map[string]bool{ids[0]: true}, nil
}

// The Safe export asks once for every page its texts mention; the
// director's modes and a failed check ask nothing of the answer.
func TestService_VisiblePages(t *testing.T) {
	a := `<a data-mention-id="p1">A</a><a data-mention-id="p2">B</a>`
	b := `<a data-mention-id="p2">B</a>`
	stub := &pageFilterStub{}
	s := &Service{Pages: stub}
	got := s.visiblePages(context.Background(), "c1", 1, "u1", Options{Privacy: PrivacyModeSafe}, &a, nil, &b)
	if strings.Join(stub.asked, ",") != "p1,p2" || !got["p1"] || got["p2"] {
		t.Errorf("asked %v, got %v", stub.asked, got)
	}
	if got := s.visiblePages(context.Background(), "c1", 1, "u1", Options{Privacy: PrivacyModePermitted}, &a); got != nil {
		t.Errorf("permitted mode asked for visibility: %v", got)
	}
	s.Pages = &pageFilterStub{err: context.Canceled}
	if got := s.visiblePages(context.Background(), "c1", 1, "u1", Options{Privacy: PrivacyModeSafe}, &a); got != nil {
		t.Errorf("a failed check must hide every mention, got %v", got)
	}
}
