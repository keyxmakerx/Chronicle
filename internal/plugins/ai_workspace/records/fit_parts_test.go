package records

import (
	"fmt"
	"strings"
	"testing"
)

// Parts that fit are written whole; parts that don't are all named, the
// ones that fit written, and the rest named to ask for, inside the cap.
func TestFitParts(t *testing.T) {
	small := []namedPart{{"A", "aaa\n"}, {"B", "bbb\n"}}
	var b strings.Builder
	fitParts(&b, small, "end\n", "recipe", "recipes")
	if b.String() != "aaa\nbbb\nend\n" {
		t.Fatalf("small: %q", b.String())
	}

	var big []namedPart
	for i := 0; i < 40; i++ {
		big = append(big, namedPart{fmt.Sprintf("Recipe %02d", i), strings.Repeat("x", 2000) + "\n"})
	}
	b.Reset()
	b.WriteString("## Recipes\n\n")
	fitParts(&b, big, "", "recipe", "recipes")
	out := b.String()
	if len(out) > maxAnswerBytes {
		t.Fatalf("%d bytes, over the cap", len(out))
	}
	for _, p := range big {
		if !strings.Contains(out, "- "+p.name+"\n") {
			t.Fatalf("%s not listed", p.name)
		}
	}
	if !strings.Contains(out, `left out above: "Recipe `) || !strings.Contains(out, `"Recipe 39". Ask again with `+"`name`") {
		t.Fatalf("closing line:\n%s", out[len(out)-300:])
	}
	if strings.Count(out, strings.Repeat("x", 2000)) == 0 {
		t.Fatal("no part was written whole")
	}
}
