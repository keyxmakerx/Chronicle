package vault_import

import (
	"reflect"
	"testing"
)

// brief is the part of a Ref the table below checks.
type brief struct {
	Kind                         RefKind
	Target, Fragment, Text, Hint string
	Remote                       bool
}

func briefs(refs []Ref) []brief {
	var out []brief
	for _, r := range refs {
		out = append(out, brief{r.Kind, r.Target, r.Fragment, r.Text, r.Hint, r.Remote})
	}
	return out
}

func TestScanRefs(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []brief
	}{
		{"plain wiki link", "see [[Bob]] now", []brief{{Kind: RefWiki, Target: "Bob"}}},
		{"alias", "[[Bob|the baker]]", []brief{{Kind: RefWiki, Target: "Bob", Text: "the baker"}}},
		{"heading", "[[Bob#History]]", []brief{{Kind: RefWiki, Target: "Bob", Fragment: "History"}}},
		{"block id", "[[Bob#^abc123]]", []brief{{Kind: RefWiki, Target: "Bob", Fragment: "^abc123"}}},
		{"heading and alias", "[[Bob#History|his past]]", []brief{{Kind: RefWiki, Target: "Bob", Fragment: "History", Text: "his past"}}},
		{"path form", "[[People/Bob]]", []brief{{Kind: RefWiki, Target: "People/Bob"}}},
		{"same-note heading", "[[#Goals]]", []brief{{Kind: RefWiki, Fragment: "Goals"}}},
		{"table-escaped alias bar", `[[Bob\|baker]]`, []brief{{Kind: RefWiki, Target: "Bob", Text: "baker"}}},
		{"picture embed", "![[map.png]]", []brief{{Kind: RefEmbed, Target: "map.png"}}},
		{"picture embed width", "![[map.png|300]]", []brief{{Kind: RefEmbed, Target: "map.png", Hint: "300"}}},
		{"picture embed width and height", "![[map.png|300x200]]", []brief{{Kind: RefEmbed, Target: "map.png", Hint: "300x200"}}},
		{"note embed", "![[Bob]]", []brief{{Kind: RefEmbed, Target: "Bob"}}},
		{"markdown link, encoded", "[Bob](People/Bob%20Smith.md)", []brief{{Kind: RefLink, Target: "People/Bob Smith.md", Text: "Bob"}}},
		{"markdown link with fragment", "[x](Bob.md#Top)", []brief{{Kind: RefLink, Target: "Bob.md", Fragment: "Top", Text: "x"}}},
		{"markdown picture", "![a map](img/map.png)", []brief{{Kind: RefImage, Target: "img/map.png", Text: "a map"}}},
		{"web link is remote", "[x](https://example.com/a)", []brief{{Kind: RefLink, Target: "https://example.com/a", Text: "x", Remote: true}}},
		{"mail link is remote", "[x](mailto:a@b.c)", []brief{{Kind: RefLink, Target: "mailto:a@b.c", Text: "x", Remote: true}}},
		{"two on a line", "[[A]] and [[B]]", []brief{{Kind: RefWiki, Target: "A"}, {Kind: RefWiki, Target: "B"}}},
		{"inline code is not read", "use `[[Bob]]` here", nil},
		{"double-backtick code is not read", "use ``a ` [[Bob]]`` here", nil},
		{"fenced code is not read", "```\n[[Bob]]\n![[a.png]]\n```\nafter [[Real]]", []brief{{Kind: RefWiki, Target: "Real"}}},
		{"tilde fence is not read", "~~~\n[[Bob]]\n~~~", nil},
		{"unclosed fence swallows the rest", "```\n[[Bob]]", nil},
		{"unclosed link is plain text", "[[Bob and more", nil},
		{"empty link is dropped", "[[]]", nil},
		{"unicode name", "[[Ünïcödé Café]]", []brief{{Kind: RefWiki, Target: "Ünïcödé Café"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := briefs(ScanRefs(tc.src))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ScanRefs(%q)\n got  %+v\n want %+v", tc.src, got, tc.want)
			}
		})
	}
}

func TestRewrite(t *testing.T) {
	src := "a [[Bob]] b ![[map.png]] c `[[Code]]` d"
	got := Rewrite(src, ScanRefs(src), func(r Ref) (string, bool) {
		if r.Kind == RefEmbed {
			return "[picture]", true
		}
		return "<" + r.Target + ">", true
	})
	want := "a <Bob> b [picture] c `[[Code]]` d"
	if got != want {
		t.Errorf("Rewrite = %q, want %q", got, want)
	}

	// Declining leaves the original text byte for byte.
	same := Rewrite(src, ScanRefs(src), func(Ref) (string, bool) { return "", false })
	if same != src {
		t.Errorf("declined Rewrite changed the text: %q", same)
	}
}

func TestPrepare(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"comment removed", "a %%secret%% b", "a  b"},
		{"multi-line comment removed", "a %%x\ny%% b", "a \n b"},
		{"block id removed", "A line ^abc-1", "A line"},
		{"solo block id removed", "text\n^abc\nmore", "text\n\nmore"},
		{"callout becomes bold title", "> [!warning] Careful\n> body", "> **Careful**\n> body"},
		{"callout without title", "> [!note]\n> body", "> **Note**\n> body"},
		{"fold marker", "> [!tip]- Hidden", "> **Hidden**"},
		{"code is untouched", "```\n%%keep%% ^id\n```", "```\n%%keep%% ^id\n```"},
		{"crlf normalised", "a\r\nb", "a\nb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Prepare(tc.in); got != tc.want {
				t.Errorf("Prepare(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
