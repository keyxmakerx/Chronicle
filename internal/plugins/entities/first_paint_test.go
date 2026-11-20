package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The page sends its text with itself and waits to show whole.
func TestShowSendsTheTextWithThePage(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner, IsMember: true}
	body := showEntryFor(t, cc, "u1", "")
	for _, want := range []string{`data-whole`, `data-first-paint`, `<template data-first-paint-data>`, `&#34;rev&#34;`} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %s", want)
		}
	}
	if n := strings.Count(body, `<template data-first-paint-data>`); n != 1 {
		t.Errorf("only the text is on this layout, so one seed is expected, got %d", n)
	}
}

func seeded(parts map[int]string) *firstPaint {
	f := &firstPaint{}
	for i, v := range parts {
		v := v
		f.compute[i] = func() json.RawMessage {
			if v == "" {
				return nil
			}
			return json.RawMessage(v)
		}
	}
	return f
}

func TestFirstPaintOf(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		part int
		want string
	}{
		{"no payload on the context", context.Background(), fpEntry, ""},
		{"a nil payload (era version) sends nothing", withFirstPaint(context.Background(), nil), fpTags, ""},
		{"a part nobody supplies", withFirstPaint(context.Background(), seeded(map[int]string{fpEntry: `{"a":1}`})), fpFields, ""},
		{"a part that failed sends nothing", withFirstPaint(context.Background(), seeded(map[int]string{fpRelations: ""})), fpRelations, ""},
		{"a supplied part", withFirstPaint(context.Background(), seeded(map[int]string{fpTags: `{"all":[],"mine":[]}`})), fpTags, `{"all":[],"mine":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstPaintOf(tt.ctx, tt.part); got != tt.want {
				t.Errorf("firstPaintOf = %q, want %q", got, tt.want)
			}
		})
	}
}

// A part is worked out once however many blocks ask, and only when asked.
func TestFirstPaintWorkedOutOnceAndLazily(t *testing.T) {
	calls := map[int]int{}
	f := &firstPaint{}
	for _, i := range []int{fpEntry, fpFields} {
		i := i
		f.compute[i] = func() json.RawMessage { calls[i]++; return json.RawMessage(`{}`) }
	}
	ctx := withFirstPaint(context.Background(), f)
	firstPaintOf(ctx, fpEntry)
	firstPaintOf(ctx, fpEntry)
	if calls[fpEntry] != 1 {
		t.Errorf("entry worked out %d times, want 1", calls[fpEntry])
	}
	if calls[fpFields] != 0 {
		t.Errorf("fields worked out %d times though no block asked, want 0", calls[fpFields])
	}
}

func TestSendsFirstPaint(t *testing.T) {
	if sendsFirstPaint(context.Background()) {
		t.Error("a page with no payload must show at once")
	}
	if sendsFirstPaint(withFirstPaint(context.Background(), nil)) {
		t.Error("a nil payload must show at once")
	}
	if !sendsFirstPaint(withFirstPaint(context.Background(), seeded(nil))) {
		t.Error("a page with a payload waits to show whole")
	}
}

// The payload is text a reader's browser parses inside the page, and it holds
// words anyone with edit rights wrote, so it must not be able to close its own
// <template> and run as markup.
func TestFirstPaintSeedIsEscaped(t *testing.T) {
	evil := `{"entry":"</template><script>alert(1)</script>"}`
	ctx := withFirstPaint(context.Background(), seeded(map[int]string{fpEntry: evil}))
	var buf bytes.Buffer
	if err := firstPaintSeed(fpEntry).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.HasPrefix(html, "<template data-first-paint-data>") || !strings.HasSuffix(html, "</template>") {
		t.Fatalf("seed = %s, want a single template", html)
	}
	if strings.Contains(html, "<script") || strings.Count(html, "</template>") != 1 {
		t.Errorf("seed is not escaped: %s", html)
	}
	var none bytes.Buffer
	if err := firstPaintSeed(fpTags).Render(ctx, &none); err != nil || none.Len() != 0 {
		t.Errorf("a part nobody supplies must draw nothing, got %q (%v)", none.String(), err)
	}
}
