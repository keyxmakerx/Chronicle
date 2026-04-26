package backup

import (
	"strings"
	"testing"
)

// TestCapBuf_TruncatesAtMax pins the safety property: a script that
// floods stdout cannot OOM the chronicle process; capBuf silently drops
// bytes past the cap and Write still claims success so the child
// doesn't block.
func TestCapBuf_TruncatesAtMax(t *testing.T) {
	b := newCapBuf(10)
	huge := strings.Repeat("x", 1_000_000)
	n, err := b.Write([]byte(huge))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(huge) {
		t.Errorf("Write returned %d, want %d (must claim full write)", n, len(huge))
	}
	if got := b.String(); got != "xxxxxxxxxx" {
		t.Errorf("buffer = %q, want exactly cap=10 chars", got)
	}
}

// TestCapBuf_PartialFill confirms small writes accumulate normally
// before the cap is hit.
func TestCapBuf_PartialFill(t *testing.T) {
	b := newCapBuf(100)
	_, _ = b.Write([]byte("hello "))
	_, _ = b.Write([]byte("world"))
	if got := b.String(); got != "hello world" {
		t.Errorf("buffer = %q, want %q", got, "hello world")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1500, "1.5 KB"},
		{2_500_000, "2.5 MB"},
		{3_000_000_000, "3.0 GB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
