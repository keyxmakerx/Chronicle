package layouts

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func renderOpening(t *testing.T, ctx context.Context, d AppOpeningData) string {
	t.Helper()
	var buf bytes.Buffer
	body := templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := w.Write([]byte("<span>3 maps</span>"))
		return err
	})
	if err := AppOpening(d).Render(templ.WithChildren(ctx, body), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// The opening is one component in two sizes: empty draws the sentence for
// whoever is looking, and never offers an action that viewer can't take.
func TestAppOpening_EmptyIsAudienceAware(t *testing.T) {
	d := AppOpeningData{
		Name: "Maps", Empty: true,
		Intros: AppIntros{Player: "player sentence", CoDM: "codm sentence", Owner: "owner sentence"},
	}
	act := &AppAction{Label: "Add a map", Attrs: templ.Attributes{"data-x": "1"}}
	base := context.Background()
	tests := []struct {
		name    string
		ctx     context.Context
		action  *AppAction
		want    []string
		notWant []string
	}{
		{"player", base, nil, []string{"player sentence", "Nothing to do yet", `data-app-opening="empty"`}, []string{"owner sentence", "Add a map", "<button", "<a "}},
		{"co-DM", SetIsDmGranted(base, true), nil, []string{"codm sentence"}, []string{"Add a map", "player sentence"}},
		{"owner", SetIsOwner(base, true), act, []string{"owner sentence", "Add a map", `data-x="1"`}, []string{"Nothing to do yet", "player sentence"}},
		{"owner viewing as player", SetViewingAsPlayer(SetIsOwner(base, true), true), nil, []string{"player sentence"}, []string{"owner sentence"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dd := d
			dd.Action = tt.action
			out := renderOpening(t, tt.ctx, dd)
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in:\n%s", w, out)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(out, w) {
					t.Errorf("should not contain %q in:\n%s", w, out)
				}
			}
		})
	}
}

// With content the opening is one title line: the tools sit beside it and the
// action ends it, with no box or sentence.
func TestAppOpening_FullIsOneLine(t *testing.T) {
	d := AppOpeningData{
		Name: "Armory", Intros: AppIntros{Player: "must not show"},
		Action: &AppAction{Label: "Add an item", Href: "/x/new"},
	}
	out := renderOpening(t, SetIsOwner(context.Background(), true), d)
	for _, w := range []string{`data-app-opening="full"`, "Armory", "<span>3 maps</span>", `href="/x/new"`, "Add an item", "cb-title", "cb-head"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
	if strings.Contains(out, "must not show") || strings.Contains(out, "Nothing to do yet") {
		t.Errorf("a page with content still shows the empty sentence:\n%s", out)
	}
}

// The game master's name comes from GMTerm in the sentence an app writes.
func TestAppIntros_For(t *testing.T) {
	i := AppIntros{Player: "p", CoDM: "c", Owner: "o"}
	for a, want := range map[AppAudience]string{AudiencePlayer: "p", AudienceCoDM: "c", AudienceOwner: "o"} {
		if got := i.For(a); got != want {
			t.Errorf("For(%v) = %q, want %q", a, got, want)
		}
	}
}
