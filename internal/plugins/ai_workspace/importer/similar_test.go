package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"New Player Guide", "new player guide"},
		{"New Player Guide (Imported)", "new player guide"},
		{"New Player Guide (copy)", "new player guide"},
		{"New Player Guide (Copy 2)", "new player guide"},
		{"New Player Guide [draft]", "new player guide"},
		{"New Player Guide - Copy", "new player guide"},
		{"New Player Guide copy", "new player guide"},
		{"New  Player-Guide!", "new player guide"},
		{"Guide (Imported) (copy)", "guide"},
		{"Session 3", "session 3"},
		{"(Imported)", "imported"}, // a name that is only a suffix is kept, not emptied
		{"Copy", "copy"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := NormalizeTitle(tt.in); got != tt.want {
				t.Fatalf("NormalizeTitle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTitleStrength(t *testing.T) {
	tests := []struct {
		name, a, b string
		want       int
	}{
		{"same after suffix", "New Player Guide (Imported)", "New Player Guide", 2},
		{"case and punctuation", "new player guide!", "New Player Guide", 2},
		{"one holds the other", "New Player Guide", "New Player Guide Extended", 1},
		{"numbered sessions differ", "Session 3", "Session 4", 0},
		{"single word never contains", "Guide", "Guide to Therin", 0},
		{"single word same", "Therin", "therin", 2},
		{"unrelated", "Harbour rumours", "New Player Guide", 0},
		{"empty", "", "Guide", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := titleStrength(tt.a, tt.b); got != tt.want {
				t.Fatalf("titleStrength(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func prose(n int, extra ...string) string {
	var w []string
	for i := 0; i < n; i++ {
		w = append(w, "word"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	return strings.Join(append(w, extra...), " ")
}

func TestBodyOverlap(t *testing.T) {
	tests := []struct {
		name        string
		a, b        string
		wantOK      bool
		wantAtLeast float64
		wantDiffer  int
	}{
		{"identical", prose(30), prose(30), true, 1, 0},
		{"nine words differ", prose(40, "x1", "x2", "x3", "x4"), prose(40, "y1", "y2", "y3", "y4", "y5"), true, 0.8, 9},
		{"short body is not compared", "just a few words", "just a few words", false, 0, 0},
		{"different subjects", prose(30), strings.ReplaceAll(prose(30), "word", "other"), true, 0, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, d, ok := bodyOverlap(tt.a, tt.b)
			if ok != tt.wantOK || (ok && (j < tt.wantAtLeast || d != tt.wantDiffer)) {
				t.Fatalf("bodyOverlap = %v, %d, %v", j, d, ok)
			}
		})
	}
}

// fakeFinder returns pages by the role that asks, as the real search does,
// and remembers who asked.
type fakeFinder struct {
	pages    []entities.Entity
	hiddenTo map[string]int // page ID → highest role that may not see it (it needs more)
	gotRole  int
	gotQuery string
}

func (f *fakeFinder) Search(_ context.Context, _, q string, _ int, role int, _ string, _ entities.ListOptions) ([]entities.Entity, int, error) {
	f.gotRole, f.gotQuery = role, q
	var out []entities.Entity
	for _, e := range f.pages {
		if need, hidden := f.hiddenTo[e.ID]; hidden && role <= need {
			continue
		}
		out = append(out, e)
	}
	return out, len(out), nil
}

func simPage(id, name, html string) entities.Entity {
	e := entities.Entity{ID: id, Name: name, Slug: entities.Slugify(name)}
	if html != "" {
		e.EntryHTML = &html
	}
	return e
}

func TestFindSimilar(t *testing.T) {
	guide := "<p>" + prose(30) + "</p>"
	secret := `<p>` + prose(30) + `</p><p><span data-secret="true">the dragon sleeps under the chapel</span></p>`
	tests := []struct {
		name     string
		req      ParsedPage
		pages    []entities.Entity
		hidden   map[string]int
		role     int
		wantID   string
		wantDiff int
	}{
		{"imported twin by name", ParsedPage{Name: "New Player Guide (Imported)", Body: prose(30, "extra")}, []entities.Entity{simPage("1", "New Player Guide", guide)}, nil, 3, "1", 1},
		{"unrelated page", ParsedPage{Name: "Harbour rumours", Body: prose(30)}, []entities.Entity{simPage("1", "New Player Guide", "<p>nothing alike here at all</p>")}, nil, 3, "", 0},
		{"same words under another name", ParsedPage{Name: "Welcome Handbook", Body: prose(30)}, []entities.Entity{simPage("1", "New Player Guide", guide)}, nil, 3, "1", 0},
		{"numbered sessions stay apart", ParsedPage{Name: "Session 4", Body: "We play."}, []entities.Entity{simPage("1", "Session 3", "<p>We play.</p>")}, nil, 3, "", 0},
		{"exact name is the conflict, not a look-alike", ParsedPage{Name: "New Player Guide", Body: prose(30)}, []entities.Entity{simPage("1", "New Player Guide", guide)}, nil, 3, "", 0},
		{"best of several", ParsedPage{Name: "Player Guide Extended", Body: prose(30)}, []entities.Entity{simPage("1", "Player Guide", "<p>short</p>"), simPage("2", "Player Guide Extended (copy)", guide)}, nil, 3, "2", 0},
		{"a page the viewer cannot see is never named", ParsedPage{Name: "New Player Guide (Imported)", Body: prose(30)}, []entities.Entity{simPage("1", "New Player Guide", guide)}, map[string]int{"1": 2}, 2, "", 0},
		{"gm-only passages do not count as words", ParsedPage{Name: "New Player Guide (Imported)", Body: prose(30)}, []entities.Entity{simPage("1", "New Player Guide", secret)}, nil, 3, "1", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeFinder{pages: tt.pages, hiddenTo: tt.hidden}
			got := FindSimilar(context.Background(), f, "c", Viewer{Role: tt.role, UserID: "u"}, tt.req, nil)
			if f.gotRole != tt.role {
				t.Fatalf("searched as role %d, want the viewer's %d", f.gotRole, tt.role)
			}
			if tt.wantID == "" {
				if got != nil {
					t.Fatalf("matched %+v, want none", got)
				}
				return
			}
			if got == nil || got.ID != tt.wantID {
				t.Fatalf("got %+v, want page %s", got, tt.wantID)
			}
			if got.HasBody && got.Differ != tt.wantDiff {
				t.Fatalf("differ = %d, want %d", got.Differ, tt.wantDiff)
			}
			for _, l := range got.Have {
				if strings.Contains(l, "dragon") {
					t.Fatalf("GM-only text leaked into the comparison: %q", l)
				}
			}
			if !got.CanUpdate {
				t.Fatal("a page whose slug follows its name can be changed by name")
			}
		})
	}
}

func TestFindSimilarCannotUpdateARenamedPage(t *testing.T) {
	e := simPage("1", "New Player Guide", "<p>"+prose(30)+"</p>")
	e.Slug = "old-slug"
	got := FindSimilar(context.Background(), &fakeFinder{pages: []entities.Entity{e}}, "c", Viewer{Role: 3}, ParsedPage{Name: "New Player Guide (Imported)", Body: prose(30)}, nil)
	if got == nil || got.CanUpdate {
		t.Fatalf("got %+v: a change finds its page by name, so a page whose slug no longer follows its name cannot be offered", got)
	}
}

func TestFindSimilarSkips(t *testing.T) {
	f := &fakeFinder{pages: []entities.Entity{simPage("1", "New Player Guide", "<p>"+prose(30)+"</p>")}}
	tests := []struct {
		name string
		p    ParsedPage
		f    PageFinder
	}{
		{"no finder", ParsedPage{Name: "New Player Guide (Imported)"}, nil},
		{"parse error", ParsedPage{Name: "New Player Guide (Imported)", Status: StatusParseError}, f},
		{"no usable word", ParsedPage{Name: "?"}, f},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FindSimilar(context.Background(), tt.f, "c", Viewer{Role: 3}, tt.p, nil); got != nil {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
