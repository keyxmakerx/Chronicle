package ai_workspace

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
)

func TestImportReview_SimilarLine(t *testing.T) {
	match := &importer.SimilarPage{
		Name: "New Player Guide", Differ: 9, HasBody: true, CanUpdate: true,
		Have: []string{"Welcome to Therin.", "Bring a hero of level 1."},
		Sent: []string{"Welcome to Therin.", "Bring a hero of level 2."},
	}
	tests := []struct {
		name    string
		similar []*importer.SimilarPage
		edit    func(*ImportReviewData)
		want    []string
		absent  []string
	}{
		{
			name:    "a close match is named with both ways forward",
			similar: []*importer.SimilarPage{match},
			want: []string{
				"Much like a page you have:", "New Player Guide", ", 9 words differ",
				"Update that page instead", "See both",
				`data-match-name="New Player Guide"`, `data-name-field="page_0_name"`, `data-action-field="page_0_action"`,
				"The page you have", "What your AI sent", "Only here",
			},
			absent: []string{"replace_gm_confirmed"},
		},
		{
			name:    "one differing word reads in the singular",
			similar: []*importer.SimilarPage{{Name: "New Player Guide", Differ: 1, HasBody: true, CanUpdate: true}},
			want:    []string{", 1 word differs"},
			absent:  []string{"1 words", "differ<", "1 word differ,"},
		},
		{
			name:    "no match, no line",
			similar: []*importer.SimilarPage{nil, nil, nil, nil},
			absent:  []string{"Much like a page you have", "air-sim"},
		},
		{
			name:    "a page that cannot be changed by name offers only See both",
			similar: []*importer.SimilarPage{{Name: "New Player Guide", CanUpdate: false}},
			want:    []string{"Much like a page you have:", "See both"},
			absent:  []string{"Update that page instead"},
		},
		{
			name:    "no body to compare says only the name",
			similar: []*importer.SimilarPage{{Name: "New Player Guide", CanUpdate: true}},
			want:    []string{"New Player Guide"},
			absent:  []string{"words differ", "the same words"},
		},
		{
			name:    "a page with GM-only parts asks for the replace tick, hidden until it is a change",
			similar: []*importer.SimilarPage{{Name: "New Player Guide", CanUpdate: true, GMOnly: importer.GMOnlyParts{Passages: 1}}},
			want:    []string{`name="page_0_replace_gm_confirmed"`, "data-sim-gm hidden"},
		},
		{
			name:    "a removal is not an add",
			similar: []*importer.SimilarPage{match},
			edit:    func(d *ImportReviewData) { d.Pages[0].FrontMatter.Action = importer.ActionDelete },
			absent:  []string{"Much like a page you have"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := representativeReviewData()
			d.Similar = tt.similar
			if tt.edit != nil {
				tt.edit(&d)
			}
			out := renderReview(t, d)
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("review is missing %q", w)
				}
			}
			for _, w := range tt.absent {
				if strings.Contains(out, w) {
					t.Errorf("review should not contain %q", w)
				}
			}
		})
	}
}

func TestSimLines(t *testing.T) {
	tests := []struct {
		name         string
		lines, other []string
		want         []bool
	}{
		{"same lines are not marked", []string{"A b", "c"}, []string{"a B", "C"}, []bool{false, false}},
		{"a line the other lacks is marked", []string{"One", "Two"}, []string{"One"}, []bool{false, true}},
		{"nothing to compare against", []string{"One"}, nil, []bool{true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := simLines(tt.lines, tt.other)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d lines", len(got))
			}
			for i, w := range tt.want {
				if got[i].Only != w {
					t.Errorf("line %d only = %v, want %v", i, got[i].Only, w)
				}
			}
		})
	}
	long := make([]string, maxSimLines+5)
	for i := range long {
		long[i] = strings.Repeat("x", i+1)
	}
	if got := simLines(long, nil); len(got) != maxSimLines+1 || got[maxSimLines].Text != "…" {
		t.Errorf("a long page shows its start and says so, got %d lines", len(got))
	}
}

// The browser test test/js/ai_import_similar_browser.test.mjs drives the
// review's own markup, so it reads this fixture; the Go template stays the
// one source. Refresh it with CHRONICLE_UPDATE_FIXTURES=1 go test -run
// SimilarFixture ./internal/plugins/ai_workspace/.
func TestImportReview_SimilarFixture(t *testing.T) {
	d := representativeReviewData()
	d.Pages[0].FrontMatter.Action = importer.ActionCreate
	d.Pages[0].Name = "New Player Guide (Imported)"
	d.Similar = []*importer.SimilarPage{{
		Name: "New Player Guide", Differ: 9, HasBody: true, CanUpdate: true,
		GMOnly: importer.GMOnlyParts{Passages: 1},
		Have:   []string{"Welcome to Therin.", "Bring a hero of level 1.", importer.GMOnlyMark},
		Sent:   []string{"Welcome to Therin.", "Bring a hero of level 2."},
	}}
	// The stylesheet's cache-busting hash changes with every CSS edit.
	out := regexp.MustCompile(`\?v=[0-9a-f]+`).ReplaceAllString(renderReview(t, d), "")
	path := filepath.Join("..", "..", "..", "test", "js", "fixtures", "ai_import_similar_review.html")
	if os.Getenv("CHRONICLE_UPDATE_FIXTURES") == "1" {
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != out {
		t.Fatal("the review's markup changed; refresh test/js/fixtures/ai_import_similar_review.html (see the comment above this test)")
	}
}
