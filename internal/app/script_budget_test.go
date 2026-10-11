package app

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The page-script budget: what every page downloads before it shows anything
// of its own, i.e. the layout's <script src> tags plus the plugin body
// scripts. Features belong in on-sight widgets (ADR-063), so a change that
// adds a script here must raise these numbers in the same PR and say why.
// When a feature moves out, lower them to the new totals.
const (
	everyPageScriptBudget     = 99
	everyPageGzipBudgetKB     = 920
	everyPageScriptReportRows = 15
)

var (
	layoutScriptSrc = regexp.MustCompile(`<script src=\{ AssetURL\("(/static/[^"]+\.js)"\) \}`)
	bodyScriptLine  = regexp.MustCompile(`"/static/plugins/" \+ (\w+)\.PluginSlug \+ "([^"]+)"|"(/static/[^"]+\.js)"`)
)

// everyPageScripts lists the site paths of the scripts every page loads,
// read from the layout and from routes.go's plugin body-script list.
func everyPageScripts(t *testing.T) []string {
	t.Helper()
	layout, err := os.ReadFile(filepath.Join("..", "templates", "layouts", "base.templ"))
	if err != nil {
		t.Fatal(err)
	}
	var urls []string
	for _, m := range layoutScriptSrc.FindAllStringSubmatch(string(layout), -1) {
		urls = append(urls, m[1])
	}

	routes, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(routes)
	start := strings.Index(src, "pluginBodyScripts := []string{")
	if start < 0 {
		t.Fatal("routes.go: pluginBodyScripts list not found; update this test if it moved")
	}
	block := src[start:]
	block = block[:strings.Index(block, "\n\t}")]
	for _, m := range bodyScriptLine.FindAllStringSubmatch(block, -1) {
		if m[3] != "" {
			urls = append(urls, m[3])
		} else {
			// The slug const lives in a package named after the plugin's
			// folder, which is where its static files are embedded from.
			urls = append(urls, "/static/plugins/"+m[1]+m[2])
		}
	}
	return urls
}

// sitePathFile maps a served script path to the file in the repo.
func sitePathFile(url string) string {
	if rest, ok := strings.CutPrefix(url, "/static/plugins/"); ok {
		pkg, file, _ := strings.Cut(rest, "/")
		return filepath.Join("..", "plugins", pkg, "static", file)
	}
	return filepath.Join("..", "..", strings.TrimPrefix(url, "/"))
}

// TestEveryPageScriptBudget fails when the scripts every page loads grow past
// the budget, and logs the largest so a reviewer can see what to move next.
func TestEveryPageScriptBudget(t *testing.T) {
	type row struct {
		url      string
		raw, gzd int
	}
	var rows []row
	var raw, gzd int
	for _, u := range everyPageScripts(t) {
		b, err := os.ReadFile(sitePathFile(u))
		if err != nil {
			t.Errorf("%s: %v", u, err)
			continue
		}
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, 6)
		zw.Write(b)
		zw.Close()
		rows = append(rows, row{u, len(b), buf.Len()})
		raw += len(b)
		gzd += buf.Len()
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].gzd > rows[j].gzd })
	var report strings.Builder
	fmt.Fprintf(&report, "every page loads %d scripts, %d KB raw, %d KB gzip (budget %d scripts, %d KB gzip). Largest:\n",
		len(rows), raw/1024, gzd/1024, everyPageScriptBudget, everyPageGzipBudgetKB)
	for i, r := range rows {
		if i == everyPageScriptReportRows {
			break
		}
		fmt.Fprintf(&report, "  %4d KB gzip  %s\n", r.gzd/1024, r.url)
	}
	t.Log(report.String())

	if len(rows) > everyPageScriptBudget {
		t.Errorf("every page loads %d scripts, over the budget of %d: load the new script on sight (ADR-063), or raise the budget in this test and say why in the PR",
			len(rows), everyPageScriptBudget)
	}
	if gzd/1024 > everyPageGzipBudgetKB {
		t.Errorf("every page loads %d KB gzip of script, over the budget of %d KB: load the new code on sight (ADR-063), or raise the budget in this test and say why in the PR",
			gzd/1024, everyPageGzipBudgetKB)
	}
}
