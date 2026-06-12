// url.go — link-URL allowlisting for owner-supplied navigation links.
package sanitize

import "strings"

// SafeLinkURL validates an owner-supplied navigation link URL against a strict
// allowlist, defending against stored XSS and open-redirect via javascript:,
// data:, vbscript:, protocol-relative //host, and backslash tricks. It returns
// the trimmed URL and true when allowed:
//   - an absolute http:// or https:// URL, or
//   - a same-origin relative path beginning with a single "/".
//
// Everything else (including an empty string) returns ("", false).
//
// The scheme test runs against a lowercased, control-char-stripped, backslash-
// normalized PROBE so " javascript:…", "java\tscript:…", "JAVASCRIPT:…", and
// "/\evil.com" (which browsers normalize to "//evil.com") can't slip through;
// the returned value is the trimmed ORIGINAL so the real URL is preserved.
func SafeLinkURL(raw string) (string, bool) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", false
	}
	// Build the probe: drop spaces + C0/C1 control chars, normalize backslashes
	// to slashes (browsers treat "\" as "/" in URL authority), lowercase.
	var b strings.Builder
	for _, r := range u {
		switch {
		case r <= 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			// drop control chars + spaces
		case r == '\\':
			b.WriteRune('/')
		default:
			b.WriteRune(r)
		}
	}
	probe := strings.ToLower(b.String())

	if strings.HasPrefix(probe, "/") {
		// Same-origin relative path. Reject protocol-relative "//host".
		if strings.HasPrefix(probe, "//") {
			return "", false
		}
		return u, true
	}
	if strings.HasPrefix(probe, "http://") || strings.HasPrefix(probe, "https://") {
		return u, true
	}
	return "", false
}
