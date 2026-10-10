package vault_import

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// perfSize is the size of each hostile note. VI_PERF_BYTES lets the same test
// be pointed at smaller inputs to see how the time grows.
func perfSize() int {
	if v, err := strconv.Atoi(os.Getenv("VI_PERF_BYTES")); err == nil && v > 0 {
		return v
	}
	return 1 << 20
}

// hostileNotes are single notes shaped to make a naive scanner quadratic.
func hostileNotes(n int) map[string]string {
	rep := func(unit string) string { return strings.Repeat(unit, n/len(unit)) }
	var runs strings.Builder
	for l := 1; runs.Len() < n; l++ {
		runs.WriteString(strings.Repeat("`", l))
		runs.WriteByte(' ')
	}
	return map[string]string{
		"single open brackets":      rep("x["),
		"double open brackets":      rep("[["),
		"unclosed wiki links":       rep("[[a "),
		"link opens with no close":  rep("[a](<"),
		"nested labels":             strings.Repeat("[a", n/4) + strings.Repeat("](", n/4),
		"backtick runs of new size": runs.String(),
		"embeds":                    rep("![[a.png|3]]"),
		"markdown images":           rep("![a](b.png)"),
	}
}

// The scanner must be linear: a megabyte of any of these has to cost about
// what a megabyte of ordinary prose does.
func TestScanRefs_HostileInputIsFast(t *testing.T) {
	n := perfSize()
	for name, body := range hostileNotes(n) {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			ScanRefs(body)
			Prepare(body)
			d := time.Since(start)
			t.Logf("%d bytes: %v", len(body), d)
			if d > 3*time.Second {
				t.Errorf("took %v for %d bytes, want well under 3s", d, len(body))
			}
		})
	}
}

// Links written with folders used to walk every path in the vault each time.
func TestAnalyze_PathLinksStayFast(t *testing.T) {
	const notes = 2000
	files := map[string]string{}
	for i := 0; i < notes; i++ {
		var b strings.Builder
		for k := 0; k < 50; k++ {
			j := (i*7 + k*131) % notes
			fmt.Fprintf(&b, "[[d%d/s%d/note%d]] ", j%40, j%7, j)
		}
		files[fmt.Sprintf("d%d/s%d/note%d.md", i%40, i%7, i)] = b.String()
	}
	start := time.Now()
	v := analyze(t, files)
	d := time.Since(start)
	t.Logf("%d notes: %v", notes, d)
	if v.Stats.LinksToPages == 0 {
		t.Fatal("no links resolved; the fixture is wrong")
	}
	if d > 5*time.Second {
		t.Errorf("analysing %d notes took %v, want well under 5s", notes, d)
	}
}
