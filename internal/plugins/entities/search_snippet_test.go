package entities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// joined flattens a snippet to its text, with marked runs in [brackets].
func joined(parts []SnippetPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Mark {
			b.WriteString("[" + p.Text + "]")
		} else {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func TestBuildSnippet(t *testing.T) {
	long := strings.Repeat("The harbour was quiet that season. ", 8) + "The Tidewrights owe fees. " + strings.Repeat("Nothing else happened there. ", 8)
	tests := []struct {
		name  string
		text  string
		terms []string
		want  string // "" means no snippet
	}{
		{"marks the word inside its longer word", "Ships wait three tides for a berth", []string{"tide"}, "Ships wait three [tide]s for a berth"},
		{"ignores case and keeps the page's own", "The Tidewrights owe fees", []string{"tidewrights"}, "The [Tidewrights] owe fees"},
		{"marks every occurrence", "tide and Tide again", []string{"tide"}, "[tide] and [Tide] again"},
		{"marks each word of a several-word query", "Captain Ilsa keeps the tide tables", []string{"ilsa", "tide"}, "Captain [Ilsa] keeps the [tide] tables"},
		{"no word, no line", "Nothing about it here", []string{"tide"}, ""},
		{"empty text", "", []string{"tide"}, ""},
		{"no terms", "tide", nil, ""},
		{"entities read as the character", "Salt &amp; tide", []string{"tide"}, "Salt & [tide]"},
		{"non-Latin case folding keeps positions", "Привет ТИДЕ мир", []string{"тиде"}, "Привет [ТИДЕ] мир"},
		{"markup in a page's words stays text", "<b>tide</b> <script>x</script>", []string{"tide"}, "<b>[tide]</b> <script>x</script>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joined(BuildSnippet(tt.text, tt.terms)); got != tt.want {
				t.Errorf("snippet = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("a long text is quoted around the match with ellipses", func(t *testing.T) {
		got := joined(BuildSnippet(long, []string{"tidewrights"}))
		if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
			t.Errorf("both ends should be elided: %q", got)
		}
		if !strings.Contains(got, "[Tidewrights]") {
			t.Errorf("the match must be marked: %q", got)
		}
		if n := len([]rune(got)); n > snippetWidth+20 {
			t.Errorf("snippet is %d characters, want about %d", n, snippetWidth)
		}
	})
}

func TestSearchTerms(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"tide", "tide"},
		{"ti", "ti"},
		{"tide wright", "tide|wright"},
		{"+tide -wright*", "tide|wright"},
		{"Half-Elf", "Half|Elf"},
		{"Not @Notes", "Not|Notes"},
		{"kara@example.test", "kara|example.test"},
		{"----", "----"}, // no word in it, so it is matched as one run
		{"  ", ""},
		{"a b", "a b"}, // under four characters it is matched as one run
	}
	for _, tt := range tests {
		if got := strings.Join(SearchTerms(tt.query), "|"); got != tt.want {
			t.Errorf("SearchTerms(%q) = %q, want %q", tt.query, got, tt.want)
		}
	}
}

// No character the FULLTEXT boolean parser reads as an operator may reach it,
// and each one is a word break, as it is to the index.
func TestStripFTOperators(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"tide wright", "tide wright"},
		{"+tide -wright*", "tide wright"},
		{"Half-Elf", "Half Elf"},
		{"Not @Notes", "Not Notes"},
		{"kara@example.test", "kara example.test"},
		{`"the (old) ~inn" <now> @4`, "the old inn now 4"},
		{"  two   spaces ", "two spaces"},
		{"----", ""},
		{`@ "( )" *`, ""},
	}
	for _, tt := range tests {
		got := stripFTOperators(tt.query)
		if got != tt.want {
			t.Errorf("stripFTOperators(%q) = %q, want %q", tt.query, got, tt.want)
		}
		if strings.ContainsAny(got, `+-><()~*"@`) {
			t.Errorf("stripFTOperators(%q) = %q still holds an operator", tt.query, got)
		}
	}
}

// snippetRepo is a repository that holds the reader-safe text a page was
// indexed with, as the MariaDB one does, and answers by what a reader may see.
type snippetRepo struct {
	mockEntityRepo
	pages   map[string]*Entity
	texts   map[string]string
	visible func(role int, id string) bool
	asked   []ListOptions
}

func (r *snippetRepo) Search(_ context.Context, _, query string, _ []int, role int, _ string, opts ListOptions) ([]Entity, int, error) {
	r.asked = append(r.asked, opts)
	var out []Entity
	for id, e := range r.pages {
		if r.visible(role, id) && strings.Contains(strings.ToLower(r.texts[id]), strings.ToLower(query)) {
			out = append(out, *e)
		}
	}
	return out, len(out), nil
}

func (r *snippetRepo) SearchTexts(_ context.Context, _ string, ids []string, role int, _ string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if r.visible(role, id) {
			out[id] = r.texts[id]
		}
	}
	return out, nil
}

// TestSearchAPI_MentionSnippets walks the reader kinds through the handler:
// the snippet is built from the player-safe text for each, a secret never
// appears, and only the mention group carries a line.
func TestSearchAPI_MentionSnippets(t *testing.T) {
	const secret = "Duskharrow cult"
	// What buildSearchText stores for a page whose entry holds a secret span.
	indexed := buildSearchText(`<p>The Tidewrights owe fees. <span data-secret="true">`+secret+` meets below</span></p>`, nil)
	if strings.Contains(indexed, "Duskharrow") {
		t.Fatalf("fixture: the indexed text must already be secret-free: %q", indexed)
	}
	repo := &snippetRepo{
		pages: map[string]*Entity{
			"pub":    {ID: "pub", Name: "Ledger", CampaignID: "c1"},
			"hidden": {ID: "hidden", Name: "Vault", CampaignID: "c1"},
		},
		texts: map[string]string{"pub": indexed, "hidden": "The Tidewrights hide coin here."},
		visible: func(role int, id string) bool {
			return id == "pub" || role >= int(campaigns.RoleOwner)
		},
	}
	h := NewHandler(NewEntityService(repo, &mockEntityTypeRepo{}, &mockPermissionRepo{}))

	tests := []struct {
		name       string
		role       campaigns.Role
		dmGranted  bool
		query      string
		scope      string
		wantPages  int
		wantLine   bool
		wantHidden bool
	}{
		{"owner", campaigns.RoleOwner, false, "tidewrights", "mention", 2, true, true},
		{"co-GM", campaigns.RolePlayer, true, "tidewrights", "mention", 2, true, true},
		{"scribe", campaigns.RoleScribe, false, "tidewrights", "mention", 1, true, false},
		{"player", campaigns.RolePlayer, false, "tidewrights", "mention", 1, true, false},
		{"guest", campaigns.RoleNone, false, "tidewrights", "mention", 1, true, false},
		{"named group draws no line", campaigns.RolePlayer, false, "tidewrights", "named", 1, false, false},
		{"player, secret-only word", campaigns.RolePlayer, false, "duskharrow", "mention", 0, false, false},
		{"owner, secret-only word", campaigns.RoleOwner, false, "duskharrow", "mention", 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/search?q="+tt.query+"&scope="+tt.scope, nil)
			req.Header.Set("Accept", "application/json")
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tt.role, IsDmGranted: tt.dmGranted})
			if err := h.SearchAPI(c); err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			if strings.Contains(body, "Duskharrow") || strings.Contains(body, secret) {
				t.Fatalf("SECURITY: the answer quotes secret text: %s", body)
			}
			var out struct {
				Results []map[string]any `json:"results"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if len(out.Results) != tt.wantPages {
				t.Fatalf("results = %d, want %d: %s", len(out.Results), tt.wantPages, body)
			}
			sawHidden, sawLine := false, false
			for _, r := range out.Results {
				if r["id"] == "hidden" {
					sawHidden = true
				}
				if r["id"] == "pub" {
					_, sawLine = r["snippet"]
				}
			}
			if sawHidden != tt.wantHidden {
				t.Errorf("hidden page listed = %v, want %v", sawHidden, tt.wantHidden)
			}
			if tt.wantPages > 0 && sawLine != tt.wantLine {
				t.Errorf("snippet present = %v, want %v", sawLine, tt.wantLine)
			}
		})
	}
}

// TestSearchAPI_ScopeShapesTheAnswer: the named group carries the maps and
// events, the mention group never does, and a category or tag filter leaves
// them out of the named group too since they have neither.
func TestSearchAPI_ScopeShapesTheAnswer(t *testing.T) {
	repo := &mockEntityRepo{
		searchFn: func(_ context.Context, _ string, _ string, _ []int, _ int, _ string, _ ListOptions) ([]Entity, int, error) {
			return []Entity{{ID: "e1", Name: "Guild", CampaignID: "camp-1"}}, 1, nil
		},
	}
	tl := &stubTimelineSearcher{}
	h := NewHandler(NewEntityService(repo, &mockEntityTypeRepo{}, &mockPermissionRepo{}))
	h.SetTimelineSearcher(tl)

	tests := []struct {
		query     string
		wantAsked bool
	}{
		{"q=guild&scope=named", true},
		{"q=guild&scope=mention", false},
		{"q=guild&scope=named&type=3", false},
		{"q=guild&scope=named&tags=coast", false},
		{"q=guild", true},        // quick search is unchanged
		{"q=guild&type=3", true}, // and so is its filter behaviour
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			tl.called = false
			searchJSON(t, h, tt.query)
			if tl.called != tt.wantAsked {
				t.Errorf("other searchers asked = %v, want %v", tl.called, tt.wantAsked)
			}
		})
	}
}
