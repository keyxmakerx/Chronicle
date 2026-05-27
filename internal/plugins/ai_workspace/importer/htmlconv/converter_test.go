// converter_test.go pins the HTML→ProseMirror schema mapping. Each
// test exercises one node/mark type the editor's TipTap extensions
// (StarterKit + Link + Table) consume — see scoping §1.4.
//
// The output is parsed back as JSON so the assertions are
// structural rather than string-match (resilient to attribute
// reordering inside maps).
package htmlconv

import (
	"encoding/json"
	"strings"
	"testing"
)

// asMap unmarshals the converter's JSON output to a map for
// structural assertions.
func asMap(t *testing.T, jsonStr string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%s", err, jsonStr)
	}
	return m
}

// firstChildOfDoc returns content[0] of the top-level doc node.
func firstChildOfDoc(t *testing.T, jsonStr string) map[string]any {
	t.Helper()
	m := asMap(t, jsonStr)
	if m["type"] != "doc" {
		t.Fatalf("top-level type = %v, want doc", m["type"])
	}
	content, ok := m["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("doc.content empty:\n%s", jsonStr)
	}
	return content[0].(map[string]any)
}

func TestConvert_Empty(t *testing.T) {
	got, err := Convert("")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// Empty input still produces a valid doc with an empty paragraph
	// so TipTap doesn't reject it.
	m := asMap(t, got)
	if m["type"] != "doc" {
		t.Errorf("type = %v, want doc", m["type"])
	}
}

func TestConvert_Paragraph(t *testing.T) {
	got, err := Convert("<p>Hello world</p>")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	p := firstChildOfDoc(t, got)
	if p["type"] != "paragraph" {
		t.Errorf("first child type = %v, want paragraph", p["type"])
	}
	content := p["content"].([]any)
	text := content[0].(map[string]any)
	if text["type"] != "text" {
		t.Errorf("paragraph child type = %v, want text", text["type"])
	}
	if text["text"] != "Hello world" {
		t.Errorf("text = %v", text["text"])
	}
}

func TestConvert_HeadingLevels(t *testing.T) {
	for level := 1; level <= 6; level++ {
		input := "<h" + string(rune('0'+level)) + ">Title " + string(rune('0'+level)) + "</h" + string(rune('0'+level)) + ">"
		got, err := Convert(input)
		if err != nil {
			t.Fatalf("Convert level %d: %v", level, err)
		}
		h := firstChildOfDoc(t, got)
		if h["type"] != "heading" {
			t.Errorf("level %d: type = %v, want heading", level, h["type"])
			continue
		}
		attrs := h["attrs"].(map[string]any)
		// JSON unmarshals numbers as float64.
		if int(attrs["level"].(float64)) != level {
			t.Errorf("level %d: attrs.level = %v", level, attrs["level"])
		}
	}
}

func TestConvert_Marks_BoldItalicCodeStrike(t *testing.T) {
	cases := []struct {
		html, want string
	}{
		{"<p><strong>x</strong></p>", "bold"},
		{"<p><b>x</b></p>", "bold"},
		{"<p><em>x</em></p>", "italic"},
		{"<p><i>x</i></p>", "italic"},
		{"<p><u>x</u></p>", "underline"},
		{"<p><s>x</s></p>", "strike"},
		{"<p><del>x</del></p>", "strike"},
		{"<p><code>x</code></p>", "code"},
	}
	for _, c := range cases {
		got, err := Convert(c.html)
		if err != nil {
			t.Fatalf("Convert %q: %v", c.html, err)
		}
		p := firstChildOfDoc(t, got)
		content := p["content"].([]any)
		text := content[0].(map[string]any)
		marks, ok := text["marks"].([]any)
		if !ok || len(marks) == 0 {
			t.Errorf("%q → no marks; raw:\n%s", c.html, got)
			continue
		}
		m := marks[0].(map[string]any)
		if m["type"] != c.want {
			t.Errorf("%q → mark type %v, want %s", c.html, m["type"], c.want)
		}
	}
}

func TestConvert_Link_PreservesHrefAndMentionID(t *testing.T) {
	got, err := Convert(`<p><a href="/x" data-mention-id="ent-42">Lyra</a></p>`)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	p := firstChildOfDoc(t, got)
	text := p["content"].([]any)[0].(map[string]any)
	marks := text["marks"].([]any)
	if len(marks) != 1 {
		t.Fatalf("expected 1 mark, got %d", len(marks))
	}
	link := marks[0].(map[string]any)
	if link["type"] != "link" {
		t.Errorf("mark type = %v, want link", link["type"])
	}
	attrs := link["attrs"].(map[string]any)
	if attrs["href"] != "/x" {
		t.Errorf("attrs.href = %v, want /x", attrs["href"])
	}
	if attrs["data-mention-id"] != "ent-42" {
		t.Errorf("attrs.data-mention-id = %v, want ent-42", attrs["data-mention-id"])
	}
}

func TestConvert_BulletList(t *testing.T) {
	got, err := Convert("<ul><li>One</li><li>Two</li></ul>")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	ul := firstChildOfDoc(t, got)
	if ul["type"] != "bulletList" {
		t.Errorf("type = %v, want bulletList", ul["type"])
	}
	items := ul["content"].([]any)
	if len(items) != 2 {
		t.Errorf("got %d items, want 2", len(items))
	}
	for i, it := range items {
		m := it.(map[string]any)
		if m["type"] != "listItem" {
			t.Errorf("items[%d].type = %v, want listItem", i, m["type"])
		}
		// listItem children must be block-level (paragraph).
		c := m["content"].([]any)
		if len(c) == 0 || c[0].(map[string]any)["type"] != "paragraph" {
			t.Errorf("items[%d] children not block-wrapped: %v", i, c)
		}
	}
}

func TestConvert_OrderedList_PreservesStart(t *testing.T) {
	got, err := Convert(`<ol start="3"><li>Three</li></ol>`)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	ol := firstChildOfDoc(t, got)
	if ol["type"] != "orderedList" {
		t.Errorf("type = %v, want orderedList", ol["type"])
	}
	attrs := ol["attrs"].(map[string]any)
	if int(attrs["start"].(float64)) != 3 {
		t.Errorf("attrs.start = %v, want 3", attrs["start"])
	}
}

func TestConvert_Blockquote(t *testing.T) {
	got, err := Convert("<blockquote><p>quoted</p></blockquote>")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	bq := firstChildOfDoc(t, got)
	if bq["type"] != "blockquote" {
		t.Errorf("type = %v, want blockquote", bq["type"])
	}
}

func TestConvert_CodeBlock(t *testing.T) {
	got, err := Convert(`<pre><code class="language-go">func main() {}</code></pre>`)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	cb := firstChildOfDoc(t, got)
	if cb["type"] != "codeBlock" {
		t.Errorf("type = %v, want codeBlock", cb["type"])
	}
	content := cb["content"].([]any)
	text := content[0].(map[string]any)
	if !strings.Contains(text["text"].(string), "func main") {
		t.Errorf("codeBlock body lost: %v", text["text"])
	}
}

func TestConvert_HorizontalRule(t *testing.T) {
	got, err := Convert("<hr>")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	hr := firstChildOfDoc(t, got)
	if hr["type"] != "horizontalRule" {
		t.Errorf("type = %v, want horizontalRule", hr["type"])
	}
}

func TestConvert_Table_HeaderAndBody(t *testing.T) {
	html := `<table>
<thead><tr><th>Name</th><th>Role</th></tr></thead>
<tbody><tr><td>Lyra</td><td>PC</td></tr></tbody>
</table>`
	got, err := Convert(html)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	tbl := firstChildOfDoc(t, got)
	if tbl["type"] != "table" {
		t.Fatalf("type = %v, want table", tbl["type"])
	}
	rows := tbl["content"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (header + body)", len(rows))
	}
	// First row should have tableHeader cells.
	r0 := rows[0].(map[string]any)
	cells := r0["content"].([]any)
	if cells[0].(map[string]any)["type"] != "tableHeader" {
		t.Errorf("header cell type = %v, want tableHeader", cells[0].(map[string]any)["type"])
	}
	// Second row tableCell.
	r1 := rows[1].(map[string]any)
	bcells := r1["content"].([]any)
	if bcells[0].(map[string]any)["type"] != "tableCell" {
		t.Errorf("body cell type = %v, want tableCell", bcells[0].(map[string]any)["type"])
	}
}

func TestConvert_RealisticAIOutput(t *testing.T) {
	// Mimics goldmark output for a typical AI-generated entity page.
	html := `<h1>Lyra Vance</h1>
<p>Lyra is a <strong>storm sorcerer</strong> bonded to <em>Sigil</em>.</p>
<ul>
<li>Class: Storm Sorcerer</li>
<li>Race: Half-Elf</li>
</ul>
<p>See <a href="#sigil" data-mention-id="ent-99">Sigil</a> for details.</p>`
	got, err := Convert(html)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	m := asMap(t, got)
	content := m["content"].([]any)
	if len(content) < 4 {
		t.Fatalf("expected at least 4 top-level children (h1 + p + ul + p), got %d", len(content))
	}
	// First child: heading level 1.
	h1 := content[0].(map[string]any)
	if h1["type"] != "heading" || int(h1["attrs"].(map[string]any)["level"].(float64)) != 1 {
		t.Errorf("first child not h1: %v", h1)
	}
	// Third child: bulletList.
	ul := content[2].(map[string]any)
	if ul["type"] != "bulletList" {
		t.Errorf("third child = %v, want bulletList", ul["type"])
	}
}
