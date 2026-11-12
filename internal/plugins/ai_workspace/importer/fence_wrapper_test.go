package importer

import (
	"strings"
	"testing"
	"time"
)

func TestParse_CodeFenceWrapperIsNotPageText(t *testing.T) {
	blk := "---\nname: Old Brannoch\ntype: character\n---\nA retired ferryman."
	tests := []struct {
		name, input, wantBody string
		pages                 int
	}{
		{"fenced with chat around", "Sure!\n\n```markdown\n" + blk + "\n```\n\nLet me know!", "A retired ferryman.", 1},
		{"plain fence", "```\n" + blk + "\n```", "A retired ferryman.", 1},
		{"unfenced untouched", blk, "A retired ferryman.", 1},
		{"unclosed wrapper left alone", "```\n" + blk, "A retired ferryman.", 1},
		{"body code block kept", blk + "\n\n```\nroll 2d6\n```\n", "A retired ferryman.\n\n```\nroll 2d6\n```", 1},
		{"four-backtick wrapper keeps inner fence", "````\n" + blk + "\n\n```\nx\n```\n````\nBye", "A retired ferryman.\n\n```\nx\n```", 1},
		{"one fence per block", "```\n" + blk + "\n```\nand now\n```\n---\nname: Two\ntype: location\n---\nSecond.\n```\nDone", "A retired ferryman.", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pages := Parse(tc.input)
			if len(pages) != tc.pages {
				t.Fatalf("pages = %d, want %d", len(pages), tc.pages)
			}
			if got := strings.TrimSpace(pages[0].Body); got != tc.wantBody {
				t.Errorf("body = %q, want %q", got, tc.wantBody)
			}
			if tc.pages == 2 && strings.TrimSpace(pages[1].Body) != "Second." {
				t.Errorf("second body = %q", pages[1].Body)
			}
		})
	}
}

// Many fences that never close must not cost time quadratic in the input: a
// player's AI can send a request of a quarter megabyte.
func TestUnwrapFences_UnclosedWrappersStayLinear(t *testing.T) {
	input := strings.Repeat("```a\n---\n", 30000)
	start := time.Now()
	if got := unwrapFences(input); got != input {
		t.Fatal("unclosed wrappers must leave the input as it was")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v for %d bytes", d, len(input))
	}
}
