package editordoc

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

func TestToHTML(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string
		ok   bool
	}{
		{"paragraph", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"The head of the Order."}]}]}`, `<p>The head of the Order.</p>`, true},
		{"heading and marks", `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Lore"}]},{"type":"paragraph","content":[{"type":"text","marks":[{"type":"bold"},{"type":"italic"}],"text":"bold"}]}]}`, `<h2>Lore</h2><p><strong><em>bold</em></strong></p>`, true},
		{"lists", `{"type":"doc","content":[{"type":"orderedList","attrs":{"start":3},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}]}]}`, `<ol start="3"><li><p>a</p></li></ol><ul><li><p>b</p></li></ul>`, true},
		{"text is escaped", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"<script>x</script>"}]}]}`, `<p>&lt;script&gt;x&lt;/script&gt;</p>`, true},
		{"secret mark", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"open "},{"type":"text","marks":[{"type":"secret"}],"text":"hidden"}]}]}`, `<p>open <span data-secret="true" class="chronicle-secret">hidden</span></p>`, true},
		{"mention link", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":"/campaigns/c/entities/e","data-mention-id":"e"}}],"text":"Vex"}]}]}`, `<p><a href="/campaigns/c/entities/e" data-mention-id="e" class="entity-link">Vex</a></p>`, true},
		{"unknown node dropped with its content", `{"type":"doc","content":[{"type":"gmImage","content":[{"type":"text","text":"gm"}]},{"type":"paragraph","content":[{"type":"text","text":"kept"}]}]}`, `<p>kept</p>`, true},
		{"code block and breaks", `{"type":"doc","content":[{"type":"codeBlock","content":[{"type":"text","text":"a<b"}]},{"type":"paragraph","content":[{"type":"text","text":"x"},{"type":"hardBreak"},{"type":"text","text":"y"}]},{"type":"horizontalRule"}]}`, `<pre><code>a&lt;b</code></pre><p>x<br>y</p><hr>`, true},
		{"not a doc", `{"type":"paragraph"}`, ``, false},
		{"not json", `<p>hi</p>`, ``, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ToHTML(tt.doc)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("ToHTML() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// A secret rendered here must be one the players' view strips.
func TestToHTMLSecretIsStrippedForPlayers(t *testing.T) {
	got, _ := ToHTML(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"open "},{"type":"text","marks":[{"type":"bold"},{"type":"secret"}],"text":"hidden"}]}]}`)
	if players := sanitize.StripSecretsHTML(sanitize.HTML(got)); strings.Contains(players, "hidden") {
		t.Fatalf("secret reached the players' view: %q", players)
	}
}
