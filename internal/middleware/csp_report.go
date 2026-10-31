package middleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/labstack/echo/v4"
)

// CSPReportPath is where browsers send report-only violations. It takes no
// session and changes nothing but a log line, so CSRF does not apply to it.
const CSPReportPath = "/csp-report"

// reportOnlyCSP is the script rule Chronicle means to enforce: scripts from
// this site only, with no inline code, no inline event handlers and no
// eval. 'report-sample' asks the browser for the start of each blocked
// inline script, which is what finds it in the templates. It uses report-uri
// alone: Chromium ignores report-uri once report-to is present, and sends
// report-to reports only from HTTPS pages, so a plain-HTTP install would
// report nothing.
const reportOnlyCSP = "script-src 'self' 'report-sample'; report-uri " + CSPReportPath

// cspReportMaxBody caps what a report request may make the server read.
const cspReportMaxBody = 64 << 10

// cspReportMaxKeys bounds the distinct violations remembered, so a client
// inventing endless variations cannot grow memory or the log without limit.
const cspReportMaxKeys = 2000

// cspViolation is one report, in the fields both report formats share.
type cspViolation struct {
	Directive string
	Document  string
	Blocked   string
	Source    string
	Line      int
	Sample    string
}

// CSPReportHandler accepts both report formats (the older
// application/csp-report body and the Reporting API's list) and logs each
// distinct violation once, with how its page and script are found again.
// Repeats are counted, not logged. It always answers 204, so a browser has
// nothing to retry and a caller learns nothing.
func CSPReportHandler() echo.HandlerFunc {
	t := &cspReportTable{seen: make(map[string]int)}
	return func(c echo.Context) error {
		body, err := io.ReadAll(io.LimitReader(c.Request().Body, cspReportMaxBody))
		if err == nil {
			for _, v := range parseCSPReports(body) {
				t.record(v)
			}
		}
		return c.NoContent(http.StatusNoContent)
	}
}

// cspReportTable remembers which violations were already logged.
type cspReportTable struct {
	mu   sync.Mutex
	seen map[string]int
	full bool
}

func (t *cspReportTable) record(v cspViolation) {
	key := strings.Join([]string{v.Directive, v.Document, v.Blocked, v.Source, strconv.Itoa(v.Line)}, "|")
	t.mu.Lock()
	if n, ok := t.seen[key]; ok {
		t.seen[key] = n + 1
		t.mu.Unlock()
		return
	}
	if len(t.seen) >= cspReportMaxKeys {
		first := !t.full
		t.full = true
		t.mu.Unlock()
		if first {
			slog.Warn("csp report-only: too many distinct violations, ignoring new ones until restart",
				slog.Int("limit", cspReportMaxKeys))
		}
		return
	}
	t.seen[key] = 1
	t.mu.Unlock()
	slog.Warn("csp report-only: script would be blocked",
		slog.String("directive", v.Directive),
		slog.String("page", v.Document),
		slog.String("blocked", v.Blocked),
		slog.String("source", v.Source),
		slog.Int("line", v.Line),
		slog.String("sample", v.Sample),
	)
}

// parseCSPReports reads either report format. Anything it can't read is
// dropped: reports are advisory and come from any client.
func parseCSPReports(body []byte) []cspViolation {
	var legacy struct {
		Report *struct {
			Document  string `json:"document-uri"`
			Blocked   string `json:"blocked-uri"`
			Violated  string `json:"violated-directive"`
			Effective string `json:"effective-directive"`
			Source    string `json:"source-file"`
			Line      int    `json:"line-number"`
			Sample    string `json:"script-sample"`
		} `json:"csp-report"`
	}
	if json.Unmarshal(body, &legacy) == nil && legacy.Report != nil {
		r := legacy.Report
		dir := r.Effective
		if dir == "" {
			dir = r.Violated
		}
		return []cspViolation{clean(cspViolation{dir, r.Document, r.Blocked, r.Source, r.Line, r.Sample})}
	}
	var list []struct {
		Type string `json:"type"`
		Body struct {
			Document  string `json:"documentURL"`
			Blocked   string `json:"blockedURL"`
			Effective string `json:"effectiveDirective"`
			Source    string `json:"sourceFile"`
			Line      int    `json:"lineNumber"`
			Sample    string `json:"sample"`
		} `json:"body"`
	}
	if json.Unmarshal(body, &list) != nil {
		return nil
	}
	var out []cspViolation
	for _, r := range list {
		if r.Type != "csp-violation" {
			continue
		}
		b := r.Body
		out = append(out, clean(cspViolation{b.Effective, b.Document, b.Blocked, b.Source, b.Line, b.Sample}))
	}
	return out
}

// idSegment matches a path segment that names one record (a number or a
// UUID-like id), so the same template on many records is one violation.
var idSegment = regexp.MustCompile(`^(\d+|[0-9a-fA-F-]{16,})$`)

// clean keeps only what finds the script again: the path without its query
// or fragment (which can carry tokens), ids folded to :id, and short
// printable text.
func clean(v cspViolation) cspViolation {
	v.Directive = short(v.Directive, 64)
	v.Document = pathOnly(v.Document)
	v.Source = pathOnly(v.Source)
	if v.Blocked != "inline" && v.Blocked != "eval" && v.Blocked != "wasm-eval" {
		v.Blocked = pathOnly(v.Blocked)
	}
	v.Sample = short(v.Sample, 80)
	if v.Line < 0 {
		v.Line = 0
	}
	return v
}

// pathOnly reduces a URL to its host-less path with ids folded, or to the
// scheme alone for data: and similar URLs.
func pathOnly(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || raw == "" {
		return short(raw, 32)
	}
	if u.Opaque != "" || (u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") {
		return short(u.Scheme+":", 32)
	}
	segs := strings.Split(u.Path, "/")
	for i, s := range segs {
		if idSegment.MatchString(s) {
			segs[i] = ":id"
		}
	}
	p := strings.Join(segs, "/")
	if u.Host != "" {
		p = u.Host + p
	}
	return short(p, 200)
}

// short trims s to n runes and replaces control characters.
func short(s string, n int) string {
	var b strings.Builder
	i := 0
	for _, r := range s {
		if i == n {
			b.WriteString("…")
			break
		}
		if unicode.IsControl(r) {
			r = ' '
		}
		b.WriteRune(r)
		i++
	}
	return b.String()
}
