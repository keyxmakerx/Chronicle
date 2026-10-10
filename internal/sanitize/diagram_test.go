package sanitize

import (
	"strings"
	"testing"
)

// A diagram (static/js/widgets/editor_diagram.js) is saved as a <pre> holding
// its Mermaid text. The sanitizer must keep that text as text and nothing
// else: the drawing is made in the reader's browser, so markup smuggled into
// the source must come out inert.
func TestHTML_KeepsDiagramSourceAsText(t *testing.T) {
	src := "flowchart LR\n  A[Start] --&gt; B{Choice}\n  B --|Yes| C"
	in := `<pre class="ce-diagram"><code>` + src + `</code></pre>`
	got := HTML(in)
	if !strings.Contains(got, `class="ce-diagram"`) {
		t.Errorf("diagram class dropped: %q", got)
	}
	if !strings.Contains(got, "flowchart LR\n  A[Start] --&gt; B{Choice}") {
		t.Errorf("diagram source not kept as text: %q", got)
	}
}

func TestHTML_DiagramInjectionIsInert(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		banned  []string
		mustHas string
	}{
		{
			name:    "script inside the source",
			in:      `<pre class="ce-diagram"><code>flowchart LR<script>alert(1)</script></code></pre>`,
			banned:  []string{"<script", "alert(1)"},
			mustHas: "flowchart LR",
		},
		{
			name:    "escaped markup stays text",
			in:      `<pre class="ce-diagram"><code>A[&lt;img src=x onerror=alert(1)&gt;]</code></pre>`,
			banned:  []string{"<img", "<script"},
			mustHas: "&lt;img src=x onerror=alert(1)&gt;",
		},
		{
			name:    "event handler and data attributes on the pre",
			in:      `<pre class="ce-diagram" onclick="alert(1)" data-svg="<svg onload=alert(1)>" data-source="x" style="position:fixed" hx-get="/x"><code>flowchart LR</code></pre>`,
			banned:  []string{"onclick", "data-svg", "data-source", "position", "hx-get", "<svg"},
			mustHas: `class="ce-diagram"`,
		},
		{
			name:    "a rendered drawing pasted in is stripped to nothing",
			in:      `<pre class="ce-diagram"><code>flowchart LR</code></pre><svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>alert(1)</script><foreignObject><div>x</div></foreignObject><a xlink:href="javascript:alert(1)"><text>A</text></a></svg>`,
			banned:  []string{"<svg", "<script", "foreignObject", "javascript:", "onload"},
			mustHas: "flowchart LR",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HTML(tt.in)
			for _, b := range tt.banned {
				if strings.Contains(got, b) {
					t.Errorf("sanitized output still holds %q: %q", b, got)
				}
			}
			if !strings.Contains(got, tt.mustHas) {
				t.Errorf("sanitized output lost %q: %q", tt.mustHas, got)
			}
		})
	}
}

// A GM-only diagram is hidden from players like a GM-only picture.
func TestStripSecretsHTML_DropsGMDiagram(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "GM-only diagram between paragraphs",
			in:   `<p>a</p><pre class="ce-diagram ce-diagram--gm"><code>flowchart LR` + "\n" + `A--&gt;B</code></pre><p>b</p>`,
			want: `<p>a</p><p>b</p>`,
		},
		{
			name: "a public diagram stays",
			in:   `<p>a</p><pre class="ce-diagram"><code>flowchart LR</code></pre>`,
			want: `<p>a</p><pre class="ce-diagram"><code>flowchart LR</code></pre>`,
		},
		{
			name: "only the GM-only one of two goes",
			in:   `<pre class="ce-diagram ce-diagram--gm"><code>secret</code></pre><pre class="ce-diagram"><code>open</code></pre>`,
			want: `<pre class="ce-diagram"><code>open</code></pre>`,
		},
		{
			name: "look-alike class kept",
			in:   `<pre class="ce-diagram--gmx"><code>x</code></pre>`,
			want: `<pre class="ce-diagram--gmx"><code>x</code></pre>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripSecretsHTML(tt.in); got != tt.want {
				t.Errorf("StripSecretsHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripSecretsHTML_GMDiagramSurvivesSanitizeThenStrips(t *testing.T) {
	stored := HTML(`<p>x</p><pre class="ce-diagram ce-diagram--gm"><code>flowchart LR` + "\n" + `Secret--&gt;Base</code></pre>`)
	if !strings.Contains(stored, "ce-diagram--gm") || !strings.Contains(stored, "Secret") {
		t.Fatalf("sanitizer dropped the GM-only diagram: %q", stored)
	}
	got := StripSecretsHTML(stored)
	if strings.Contains(got, "ce-diagram") || strings.Contains(got, "Secret") {
		t.Errorf("GM-only diagram reached players: %q", got)
	}
}

func TestStripSecretsJSON_DropsGMDiagram(t *testing.T) {
	in := `{"type":"doc","content":[` +
		`{"type":"paragraph","content":[{"type":"text","text":"kept"}]},` +
		`{"type":"diagram","attrs":{"gmOnly":true},"content":[{"type":"text","text":"flowchart LR\nSecret-->Base"}]},` +
		`{"type":"diagram","attrs":{"gmOnly":false},"content":[{"type":"text","text":"flowchart LR\nOpen-->Map"}]}]}`
	got := StripSecretsJSON(in)
	if strings.Contains(got, "Secret") {
		t.Errorf("GM-only diagram reached players: %s", got)
	}
	if !strings.Contains(got, "kept") || !strings.Contains(got, "Open") {
		t.Errorf("public content lost: %s", got)
	}
}

// A diagram nested in another block that is itself dropped goes with it, and a
// diagram carrying the secret mark (never produced by the editor, but a
// hand-written document could) is removed like any other secret node.
func TestStripSecretsJSON_DiagramInsideDroppedContent(t *testing.T) {
	in := `{"type":"doc","content":[` +
		`{"type":"blockquote","marks":[{"type":"secret"}],"content":[{"type":"diagram","attrs":{"gmOnly":false},"content":[{"type":"text","text":"flowchart LR\nHidden-->One"}]}]},` +
		`{"type":"diagram","attrs":{"gmOnly":false},"marks":[{"type":"secret"}],"content":[{"type":"text","text":"flowchart LR\nHidden-->Two"}]},` +
		`{"type":"paragraph","content":[{"type":"text","text":"kept"}]}]}`
	got := StripSecretsJSON(in)
	if strings.Contains(got, "Hidden") {
		t.Errorf("diagram inside secret content reached players: %s", got)
	}
	if !strings.Contains(got, "kept") {
		t.Errorf("ordinary text lost: %s", got)
	}
}
