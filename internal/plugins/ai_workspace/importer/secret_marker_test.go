package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer/htmlconv"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

func convertMarkers(t *testing.T, md string) string {
	t.Helper()
	out, err := MarkdownToHTML(md, func(o *renderOpts) { o.markers = true })
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMarkdownToHTML_SecretMarkers(t *testing.T) {
	tests := []struct {
		name, md string
		want     []string // secret passages found in the result
		wantHTML string   // substring, optional
	}{
		{"inline", "Public. {gm{Secret one.}gm} Public.", []string{"Secret one."}, `<span data-secret="true">Secret one.</span>`},
		{"two paragraphs", "{gm{A}gm}\n\n{gm{B}gm}", []string{"A", "B"}, ""},
		{"emphasis inside", "{gm{the **mayor** did it}gm}", []string{"the mayor did it"}, ""},
		{"emphasis straddles close", "**bold {gm{hidden** tail}gm} after", []string{"hidden", "tail"}, ""},
		{"emphasis straddles open", "{gm{hidden **tail}gm} bold**", []string{"hidden tail"}, ""},
		{"code untouched", "`{gm{x}gm}`", nil, "{gm{x}gm}"},
		{"list item", "- {gm{secret item}gm}\n- open item", []string{"secret item"}, ""},
		{"nested keeps the outer one", "{gm{a {gm{b}gm} c}gm} d", []string{"a b c"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convertMarkers(t, tc.md)
			passages := secretPassages(got)
			if strings.Join(passages, "|") != strings.Join(tc.want, "|") {
				t.Errorf("passages = %q, want %q\nhtml: %s", passages, tc.want, got)
			}
			if tc.wantHTML != "" && !strings.Contains(got, tc.wantHTML) {
				t.Errorf("html lacks %q:\n%s", tc.wantHTML, got)
			}
		})
	}
}

// A straddling emphasis must not push the tail of a passage out of the span.
func TestMarkdownToHTML_SecretStraddleTailStaysSecret(t *testing.T) {
	for _, md := range []string{"**bold {gm{hidden** tail}gm} after", "{gm{hidden **tail}gm} bold**"} {
		public := sanitize.StripSecretsHTML(convertMarkers(t, md))
		if strings.Contains(public, "hidden") || strings.Contains(public, "tail") {
			t.Errorf("%q: public text %q", md, public)
		}
	}
}

func TestMarkdownToHTML_SecretWriterGate(t *testing.T) {
	tests := []struct {
		name string
		role int
		ok   bool
	}{
		{"player", 1, false}, {"scribe", 2, false}, {"owner or co-DM", 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := NewPageLinks(context.Background(), &stubLookup{}, "c", Viewer{Role: tt.role, UserID: "u"})
			out, err := MarkdownToHTML("a {gm{b}gm}", WithPageLinks(links), WithGMOnlyPassages())
			if tt.ok {
				if err != nil || !strings.Contains(out, "data-secret") {
					t.Errorf("owner: err=%v out=%s", err, out)
				}
				return
			}
			if err != ErrSecretNotAllowed {
				t.Errorf("err = %v, want ErrSecretNotAllowed", err)
			}
		})
	}
	// Markers do nothing, and cost nothing, for a body without them.
	links := NewPageLinks(context.Background(), &stubLookup{}, "c", Viewer{Role: 1})
	if _, err := MarkdownToHTML("plain", WithPageLinks(links)); err != nil {
		t.Errorf("plain body refused: %v", err)
	}
}

func TestHTMLConv_SecretMark(t *testing.T) {
	got, err := htmlconv.Convert(`<p>a <span data-secret="true">b <strong>c</strong></span> d</p><p><span data-secret="true"><span data-secret="true">n</span></span></p>`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`{"type":"text","marks":[{"type":"secret"}],"text":"b "}`,
		`"marks":[{"type":"bold"},{"type":"secret"}],"text":"c"`,
		`{"type":"text","marks":[{"type":"secret"}],"text":"n"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("JSON lacks %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"text":"a ","marks"`) {
		t.Errorf("public text got a mark: %s", got)
	}
	// StripSecretsJSON sees exactly the secret text.
	if s := sanitize.StripSecretsJSON(got); strings.Contains(s, `"b "`) || strings.Contains(s, `"n"`) {
		t.Errorf("StripSecretsJSON left secret text: %s", s)
	}
}

func TestGMOnlyPartsAfter(t *testing.T) {
	page := func(html string) *entities.Entity { return &entities.Entity{EntryHTML: strp(html)} }
	two := `<p>Hi</p><p><span data-secret="true">one</span></p><p><span data-secret="true">two</span></p>`
	tests := []struct {
		name     string
		existing *entities.Entity
		md       string
		want     GMOnlyParts
	}{
		{"all carried back", page(two), "Hi\n\n{gm{one}gm}\n\n{gm{two}gm}", GMOnlyParts{}},
		{"stripped export drops both", page(two), "Hi", GMOnlyParts{Passages: 2}},
		{"one dropped", page(two), "Hi\n\n{gm{one}gm}", GMOnlyParts{Passages: 1}},
		{"unmarked is not kept", page(two), "Hi\n\none\n\n{gm{two}gm}", GMOnlyParts{Passages: 1}},
		{"edited words count as lost", page(two), "{gm{one}gm} {gm{twelve}gm}", GMOnlyParts{Passages: 1}},
		{"duplicate needs two copies", page(`<p><span data-secret="true">x</span> <span data-secret="true">x</span></p>`), "{gm{x}gm}", GMOnlyParts{Passages: 1}},
		{"new secrets are fine", page("<p>Hi</p>"), "{gm{fresh}gm}", GMOnlyParts{}},
		{"picture still warns", page(`<p>Hi</p><figure class="ce-img ce-img--gm"><img src="/m/a.png"></figure>`), "Hi", GMOnlyParts{Pictures: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GMOnlyPartsForMarkdown(tt.existing, tt.md); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Round trip through the real converter chain: HTML with secrets, exported
// markers written by the importer's own constants, imported again.
func TestSecretRoundTrip_Import(t *testing.T) {
	md := "Rules the city. " + SecretOpen + "Poisoned the duke." + SecretClose + "\n\n" +
		SecretOpen + "Second." + SecretClose
	html := convertMarkers(t, md)
	ent := &entities.Entity{EntryHTML: strp(html)}
	if got := GMOnlyPartsForMarkdown(ent, md); got.Any() {
		t.Errorf("re-import of its own export warns: %+v", got)
	}
	json, err := htmlconv.Convert(html)
	if err != nil || !strings.Contains(json, `"secret"`) {
		t.Errorf("no secret mark in JSON (%v): %s", err, json)
	}
}

func TestCommit_SecretMarkers(t *testing.T) {
	existing := `<p>Hi</p><p><span data-secret="true">one</span></p>`
	tests := []struct {
		name       string
		role       int
		body       string
		wantStatus RowStatus
		wantSecret bool
	}{
		{"owner, passage carried back, no tick needed", 3, "# Maro\nHi\n\n{gm{one}gm}", StatusUpdated, true},
		{"owner, passage dropped, skipped", 3, "# Maro\nHi", StatusSkipped, false},
		{"scribe with markers is refused", 2, "# Maro\nHi\n\n{gm{one}gm}", StatusFailed, false},
		{"scribe, passage dropped, skipped", 2, "# Maro\nHi", StatusSkipped, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCreator{
				types:    []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
				existing: map[string]*entities.Entity{"maro": {ID: "ent-1", Name: "Maro", Slug: "maro", EntryHTML: strp(existing)}},
			}
			pg := pageWithAction("Maro", "character", tt.body, ActionUpdate)
			d := decisionWithAction(true, "Maro", "character", "", "", ActionUpdate, false)
			res, err := NewCommitter(f).Commit(context.Background(), "camp-1", CommitInput{
				OwnerID: "u-1", Pages: []ParsedPage{pg}, Decisions: []RowDecision{d},
				Viewer: Viewer{Role: tt.role, UserID: "u-1"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := res.Rows[0].Status; got != tt.wantStatus {
				t.Fatalf("status = %q (%s), want %q", got, res.Rows[0].Reason, tt.wantStatus)
			}
			if tt.wantSecret {
				w := f.updateEntries[0]
				if !strings.Contains(w.EntryHTML, `data-secret="true"`) || !strings.Contains(w.EntryJSON, `"secret"`) {
					t.Errorf("stored text lost the passage:\n%s\n%s", w.EntryHTML, w.EntryJSON)
				}
			} else if len(f.updateEntries) > 0 {
				t.Errorf("wrote despite %q", tt.wantStatus)
			}
		})
	}
}

func pageLinksFor(role int) *PageLinks {
	return NewPageLinks(context.Background(), &stubLookup{}, "c", Viewer{Role: role, UserID: "u"})
}

// A heading id is made from the heading words and no strip pass reads
// attributes, so no secret word may reach one.
func TestMarkdownToHTML_NoSecretInAttributes(t *testing.T) {
	for _, md := range []string{
		"# {gm{Heading secret}gm}",
		"## Public {gm{Hidden words}gm} title",
		"- {gm{item}gm}\n\n### {gm{Another}gm}",
	} {
		out, err := MarkdownToHTML(md, WithPageLinks(pageLinksFor(3)), WithGMOnlyPassages())
		if err != nil {
			t.Fatal(err)
		}
		public := sanitize.StripSecretsHTML(out)
		for _, w := range []string{"secret", "Hidden", "hidden", "item", "Another", "another", "gm"} {
			if strings.Contains(public, w) && w != "gm" {
				t.Errorf("%q: public html holds %q: %s", md, w, public)
			}
		}
		if strings.Contains(public, "id=") {
			t.Errorf("%q: heading kept an id: %s", md, public)
		}
	}
}

// Markers in an attribute cannot be hidden, so the row is refused.
func TestMarkdownToHTML_MarkerInAttributeRefused(t *testing.T) {
	for _, md := range []string{
		`![{gm{alt}gm}](/media/0b6a3f0e-8c1d-4f6a-9a51-2f3c4d5e6f70)`,
		`[text](https://x.test "{gm{t}gm}")`,
	} {
		_, err := MarkdownToHTML(md, WithPageLinks(pageLinksFor(3)), WithGMOnlyPassages())
		if err != ErrSecretInAttribute {
			t.Errorf("%q: err = %v", md, err)
		}
	}
}

func TestMarkdownToHTML_UnbalancedMarkersRefused(t *testing.T) {
	tests := []struct{ name, md string }{
		{"passage across paragraphs", "{gm{para one\n\npara two}gm}"},
		{"passage across list items", "- {gm{one\n- two}gm}"},
		{"unclosed at end", "text {gm{oops"},
		{"unclosed before next paragraph", "{gm{oops\n\nnext paragraph"},
		{"close with nothing open", "plain }gm} text"},
		{"close in list", "- a}gm}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := MarkdownToHTML(tt.md, WithPageLinks(pageLinksFor(3)), WithGMOnlyPassages())
			if err != ErrSecretUnbalanced || out != "" {
				t.Errorf("err = %v, out = %q", err, out)
			}
			if r := markdownFailReason(err); !strings.Contains(r, "each secret paragraph") {
				t.Errorf("reason does not tell the AI what to do: %s", r)
			}
		})
	}
	// Markers in code are text, not markers.
	if _, err := MarkdownToHTML("`}gm}` and `{gm{`\n\n```\n{gm{\n```", WithPageLinks(pageLinksFor(3)), WithGMOnlyPassages()); err != nil {
		t.Errorf("code refused: %v", err)
	}
}

// Without the page-body option a marker is refused, even with a resolver.
func TestMarkdownToHTML_MarkersNeedPageOption(t *testing.T) {
	for _, md := range []string{"a {gm{b}gm}", "stray }gm}"} {
		_, err := MarkdownToHTML(md, WithPageLinks(pageLinksFor(3)))
		if err != ErrSecretNotInText {
			t.Errorf("%q with links: err = %v", md, err)
		}
		if _, err := MarkdownToHTML(md); err != ErrSecretNotInText {
			t.Errorf("%q bare: err = %v", md, err)
		}
	}
	if got := markdownFailReason(ErrSecretNotInText); got != "DM-only passages only work in page text" {
		t.Errorf("reason = %q", got)
	}
}

// A picture, link or break inside a passage must be hidden in the JSON too,
// which is what players receive.
func TestSecretPassage_NonTextNodesHiddenInJSONAndHTML(t *testing.T) {
	const id = "0b6a3f0e-8c1d-4f6a-9a51-2f3c4d5e6f70"
	tests := []struct{ name, md string }{
		{"picture", "Map: {gm{![lair](/media/" + id + ")}gm} end"},
		{"picture with words", "Map: {gm{lair ![lair](/media/" + id + ") deep}gm} end"},
		{"emphasised picture", "Map: {gm{**![lair](/media/" + id + ")**}gm} end"},
		{"link", "See {gm{[the cult](https://x.test/cult)}gm} now"},
		{"hard break", "See {gm{cult\nleader}gm} now"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := MarkdownToHTML(tt.md, WithPageLinks(pageLinksFor(3)), WithGMOnlyPassages())
			if err != nil {
				t.Fatal(err)
			}
			j, err := htmlconv.Convert(h)
			if err != nil {
				t.Fatal(err)
			}
			pubJSON := sanitize.StripSecretsJSON(j)
			pubHTML := sanitize.StripSecretsHTML(h)
			for _, leak := range []string{id, "lair", "cult", "leader", "deep", "hardBreak", "x.test"} {
				if strings.Contains(pubJSON, leak) {
					t.Errorf("JSON leaks %q: %s", leak, pubJSON)
				}
				if strings.Contains(pubHTML, leak) {
					t.Errorf("HTML leaks %q: %s", leak, pubHTML)
				}
			}
		})
	}
}
