package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func rollLinks(role int) *PageLinks {
	p := NewPageLinks(context.Background(), &stubLookup{}, "c", Viewer{Role: role, UserID: "u"})
	p.SetRollTables(map[string]string{"tavern names": "tavern-names", "tavern-names": "tavern-names", "rumors": "rumors"})
	return p
}

func TestMarkdownToHTML_Rollers(t *testing.T) {
	starter := &entities.Entity{EntryHTML: strp(`<p>Hi</p><div class="ce-roll ce-roll--t-rumours ce-roll--n-3"></div>`)}
	tests := []struct {
		name     string
		md       string
		role     int
		passages bool
		existing *entities.Entity
		want     string // a substring of the output
		not      string
	}{
		{"by name", "{roll{Tavern names|5}}", 3, true, nil, `<div class="ce-roll ce-roll--t-tavern-names ce-roll--n-5"></div>`, "{roll{"},
		{"by id, any case", "{roll{TAVERN-NAMES}}", 3, true, nil, `ce-roll--t-tavern-names ce-roll--n-3"`, "{roll{"},
		{"count rounds to the editor's", "{roll{Rumors|2}}", 3, true, nil, `ce-roll--n-3"`, ""},
		{"count above five", "{roll{Rumors|9}}", 3, true, nil, `ce-roll--n-5"`, ""},
		{"count of one", "{roll{Rumors|1}}", 3, true, nil, `ce-roll--n-1"`, ""},
		{"unknown stays text", "{roll{Gods}}", 3, true, nil, "<p>{roll{Gods}}</p>", "ce-roll"},
		{"inside other text stays text", "Roll {roll{Rumors}} now", 3, true, nil, "{roll{Rumors}}", "ce-roll"},
		{"hard-wrapped onto a line stays text", "Roll here:\n{roll{Rumors}}", 3, true, nil, "{roll{Rumors}}", "ce-roll"},
		{"in code stays text", "```\n{roll{Rumors}}\n```", 3, true, nil, "{roll{Rumors}}", "ce-roll"},
		{"a scribe can't place one", "{roll{Rumors}}", 2, true, nil, "<p>{roll{Rumors}}</p>", "ce-roll"},
		{"not in a record's text", "{roll{Rumors}}", 3, false, nil, "<p>{roll{Rumors}}</p>", "ce-roll"},
		{"a built-in table is unknown on a new page", "{roll{rumours}}", 3, true, nil, "{roll{rumours}}", "ce-roll"},
		{"the page's own built-in roller is kept", "{roll{rumours|3}}", 3, true, starter, `<div class="ce-roll ce-roll--t-rumours ce-roll--n-3"></div>`, "{roll{"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []RenderOption{WithPageLinks(rollLinks(tt.role)), WithKeptRollers(tt.existing)}
			if tt.passages {
				opts = append(opts, WithGMOnlyPassages())
			}
			got, err := MarkdownToHTML(tt.md, opts...)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, tt.want) || (tt.not != "" && strings.Contains(got, tt.not)) {
				t.Errorf("got %q, want %q and not %q", got, tt.want, tt.not)
			}
		})
	}
}

func TestRollWarnings(t *testing.T) {
	starter := &entities.Entity{EntryHTML: strp(`<div class="ce-roll ce-roll--t-rumours ce-roll--n-3"></div>`)}
	tests := []struct {
		name     string
		md       string
		role     int
		existing *entities.Entity
		want     []string
	}{
		{"known", "{roll{Tavern names}}\n\n{roll{rumors|1}}", 3, nil, nil},
		{"none", "Just text", 3, nil, nil},
		{"unknown, once", "{roll{Gods}}\n\n{roll{gods|3}}", 3, nil, []string{`No rolling table called "Gods"`}},
		{"inside text", "Roll {roll{Rumors}}", 3, nil, []string{"needs a line of its own"}},
		{"in code is not a roller", "`{roll{Gods}}`", 3, nil, nil},
		{"the page's own built-in", "{roll{rumours}}", 3, starter, nil},
		{"a scribe", "{roll{Rumors}}", 2, nil, []string{"Only the campaign's owner or a co-DM"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RollWarnings(rollLinks(tt.role), tt.md, tt.existing)
			if len(got) != len(tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			for i := range got {
				if !strings.Contains(got[i], tt.want[i]) {
					t.Errorf("warning %q, want %q", got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGMOnlyParts_Rollers(t *testing.T) {
	page := &entities.Entity{EntryHTML: strp(`<p>Hi</p><div class="ce-roll ce-roll--t-rumors ce-roll--n-3"></div><div class="ce-roll ce-roll--t-rumours ce-roll--n-5"></div>`)}
	tables := map[string]string{"rumors": "rumors"}
	tests := []struct {
		name string
		md   string
		want int
	}{
		{"both kept", "Hi\n\n{roll{Rumors}}\n\n{roll{rumours|5}}", 0},
		{"moved is kept", "{roll{rumours|5}}\n\nHi\n\n{roll{Rumors}}", 0},
		{"one dropped", "Hi\n\n{roll{Rumors}}", 1},
		{"a safe-mode copy drops both", "Hi", 2},
		{"an unknown line keeps nothing", "Hi\n\n{roll{Gods}}\n\n{roll{rumours}}", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GMOnlyPartsForMarkdown(page, tt.md, tables)
			if got.Rollers != tt.want {
				t.Errorf("rollers %d, want %d (%+v)", got.Rollers, tt.want, got)
			}
		})
	}
	if p := (GMOnlyParts{Rollers: 2}).Phrase(); p != "2 GM-only table rollers" {
		t.Errorf("phrase %q", p)
	}
}
