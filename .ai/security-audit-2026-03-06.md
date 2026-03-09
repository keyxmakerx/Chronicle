# Security Audit Report — Chronicle

**Date:** 2026-03-06
**Auditor:** Claude (automated code review)
**Scope:** Full codebase security audit — authentication, authorization, injection,
XSS, CSRF, file handling, HTTP headers, configuration, and infrastructure.
**Codebase Version:** Post-Sprint Q-2 (Widget Extension Distribution)

---

## Executive Summary

Chronicle demonstrates **strong security fundamentals** for a self-hosted application.
The codebase includes argon2id password hashing, CSRF protection with constant-time
comparison, parameterized SQL queries, HTML sanitization via bluemonday, signed media
URLs, ClamAV integration, comprehensive security headers, and rate limiting.

However, several findings warrant attention, ranging from medium-severity session
management gaps to low-severity hardening opportunities.

**Findings by Severity:**
- Critical: 0
- High: 2
- Medium: 5
- Low: 8
- Informational: 5

---

## HIGH Severity

### H-1: Full Session Tokens Exposed in Admin HTML

**File:** `internal/plugins/admin/security.templ:227`
**Issue:** The admin security dashboard embeds full session tokens in `hx-delete` URLs:
```
hx-delete={ fmt.Sprintf("/admin/security/sessions/%s", s.Token) }
```
This means every active session token (64 hex chars, 256-bit entropy) is rendered
into the HTML body of the admin page. If an XSS vulnerability exists anywhere in
the admin panel, an attacker could extract all active session tokens and hijack
every user's session.

**Recommendation:** Use the `TokenHint` (first 8 chars) as a URL identifier and
look up the full token server-side by scanning the user's session set, or introduce
a separate session ID (not the auth token) for management purposes. The `SessionInfo`
struct already has a `TokenHint` field — use a hash-based lookup instead.

### H-2: Stored XSS via Session RecapHTML

**File:** `internal/plugins/sessions/handler.go:311-319`
**Issue:** The `UpdateRecapAPI` handler accepts `recap_html` from a JSON request body
and passes it directly through the service (`service.go:239-240`) to the repository
(`repository.go:213-226`) without calling `sanitize.HTML()`. The stored HTML is then
rendered via `@templ.Raw(*session.RecapHTML)` at `sessions.templ:296`.

This is a **stored XSS vulnerability**. Any user with Scribe+ role can inject arbitrary
JavaScript that executes for all campaign members viewing the session page.

**Recommendation:** Sanitize `RecapHTML` in the service layer before storage:
```go
func (s *sessionService) UpdateSessionRecap(ctx context.Context, id string, recap, recapHTML *string) error {
    if recapHTML != nil && *recapHTML != "" {
        sanitized := sanitize.HTML(*recapHTML)
        recapHTML = &sanitized
    }
    return s.repo.UpdateRecap(ctx, id, recap, recapHTML)
}
```

Note: Dashboard text blocks ARE properly sanitized (`campaigns/service.go:602-605`),
but sessions RecapHTML was missed.

---

## MEDIUM Severity

### M-1: ChangePassword Does Not Invalidate Existing Sessions

**File:** `internal/plugins/auth/service.go:565-591`
**Issue:** When a user changes their password via `ChangePassword()`, existing
sessions are NOT invalidated. Compare with `ResetPassword()` (line 479) which
correctly calls `s.destroyUserSessions()`. If a user's password is compromised
and they change it, the attacker's existing sessions remain active until they
expire (default TTL: 720 hours = 30 days).

**Recommendation:** Call `s.destroyUserSessions(ctx, userID)` after updating the
password in `ChangePassword()`, then create a fresh session for the current user.

### M-2: CSP Allows unsafe-inline and unsafe-eval

**File:** `internal/middleware/security.go:28-37`
**Issue:** The Content-Security-Policy includes `'unsafe-inline'` and `'unsafe-eval'`
in the `script-src` directive. This significantly weakens XSS protection — if an
attacker can inject HTML, inline scripts will execute. The comment notes this is
required by Alpine.js.

Additionally, `https://unpkg.com` is allowed as a script source, which is a CDN
that serves arbitrary npm packages. An attacker who finds an injection point could
load any package from unpkg.

**Recommendation:**
- Migrate to nonce-based CSP for inline scripts (generate per-request nonce,
  pass to templates, add to CSP header).
- Replace `https://unpkg.com` with specific versioned CDN URLs or vendor the
  scripts locally (some are already vendored in `static/vendor/`).

### M-3: Session Cookie MaxAge Mismatch with Redis TTL

**File:** `internal/plugins/auth/handler.go:554` vs `internal/config/config.go:169`
**Issue:** The session cookie `MaxAge` is hardcoded to `30 * 24 * 60 * 60` (30 days)
while the Redis session TTL is configurable via `SESSION_TTL` (default: 720h = 30 days).
If an admin sets `SESSION_TTL` shorter (e.g., 24h), the cookie persists for 30 days
while the session is already expired in Redis. This isn't a security vulnerability
per se (the session validation will fail), but it creates UX confusion and the
cookie should honor the configured TTL.

**Recommendation:** Pass the `sessionTTL` config value to the handler and use it
for `MaxAge`.

### M-4: No Registration Rate Limiting

**File:** `internal/plugins/auth/routes.go`
**Issue:** While login has rate limiting applied, the registration endpoint
(`POST /register`) does not appear to have rate limiting. An attacker could
automate account creation to fill the database, consume resources (argon2id
hashing is intentionally expensive), or enumerate valid emails via the
"email already exists" error.

**Recommendation:** Apply `RateLimit(5, 1*time.Minute)` or similar to the
registration route.

### M-5: Password Reset Not Rate-Limited Per Email

**File:** `internal/plugins/auth/handler.go:222-239`
**Issue:** The forgot-password endpoint always returns success (correct for
preventing email enumeration), but there's no per-email rate limiting. An
attacker could flood a user's inbox with reset emails, and each request
generates a token + does expensive hashing + sends an email.

**Recommendation:** Add a rate limit check per email address (e.g., 3 requests
per 15 minutes) using Redis. Return success regardless to avoid enumeration.

---

## LOW Severity

### L-1: Avatar Upload Trusts Client-Provided Content-Type

**File:** `internal/plugins/auth/handler.go:386-389`
**Issue:** Avatar upload validates MIME type using `file.Header.Get("Content-Type")`,
which is set by the client and can be spoofed. The check is
`strings.HasPrefix(contentType, "image/")`. An attacker could upload a malicious
file with `Content-Type: image/png` that is actually HTML or SVG containing
JavaScript.

**Recommendation:** Use `http.DetectContentType()` on the file bytes to verify
the actual file type, not the client-declared Content-Type. The main media upload
path already validates MIME types against an allowlist — apply the same to avatars.

### L-2: Avatar Upload Not Scanned by ClamAV — N/A

**Status:** ClamAV was removed from Chronicle (too heavyweight for self-hosted).
Avatar uploads use `http.DetectContentType()` magic byte validation and image
re-encoding (CDR) which strips embedded payloads. No action needed.

### L-3: In-Memory Rate Limiter Not Suitable for Multi-Instance

**File:** `internal/middleware/ratelimit.go`
**Issue:** The rate limiter uses an in-memory map with a sync.Mutex. If Chronicle
is deployed behind a load balancer with multiple instances, each instance has its
own rate limit counter, effectively multiplying the allowed rate by the number of
instances.

**Recommendation:** For single-instance self-hosted deployments this is fine. If
multi-instance deployment is ever supported, migrate to Redis-based rate limiting
using `INCR` + `EXPIRE`.

### L-4: CSRF Cookie Not Tied to Session

**File:** `internal/middleware/csrf.go`
**Issue:** The CSRF token is generated independently of the user session (double-submit
cookie pattern). While the constant-time comparison is correctly implemented, the
token has no binding to the authenticated session. If an attacker can set a cookie
on the user's browser (e.g., via a subdomain), they could set both the cookie and
the header value. This is a known limitation of double-submit cookies.

**Recommendation:** Consider signing the CSRF token with the session token or
using the `__Host-` cookie prefix (requires HTTPS, no domain, path=/) to prevent
subdomain cookie injection.

### L-5: Query String Logged in Request Logger

**File:** `internal/middleware/logging.go:38-39`
**Issue:** The request logger includes the raw query string. If sensitive data is
passed via query parameters (e.g., password reset tokens in
`/reset-password?token=...`), these will be written to logs.

**Recommendation:** Redact known sensitive query parameters (token, key, password)
from logs, or avoid passing sensitive data in query strings where possible.

### L-6: Default Database Password in Development

**File:** `internal/config/config.go:155`
**Issue:** Default DB password is `"chronicle"` and default secret key is
`"dev-secret-key-do-not-use-in-production!!"`. While production validation
exists (line 187-196), it only prints a warning for the default DB password
rather than failing. The secret key check properly fails in production.

**Recommendation:** Fail startup in production if DB_PASSWORD equals the default.

### L-7: Dockerfile Does Not Use Non-Root User

**File:** `Dockerfile`
**Issue:** The runtime container runs as root. If the application is compromised,
the attacker has root access inside the container.

**Recommendation:** Add a non-root user:
```dockerfile
RUN adduser -D -H chronicle
USER chronicle
```

### L-8: Some JS Widgets Use innerHTML with User Data

**Files:** Various files in `static/js/widgets/`
**Issue:** Several JavaScript widgets build HTML strings with user data and
assign to `innerHTML`. While most use `Chronicle.escapeHtml()`, some widgets
still use raw `fetch()` instead of `Chronicle.apiFetch()` (10 files use raw
`fetch()`), and the escaping coverage is inconsistent. The `dice-roller.js`
extension example uses `innerHTML` with computed values.

**Recommendation:** Audit all `innerHTML` assignments in widget JS files to
ensure user-supplied data passes through `Chronicle.escapeHtml()`. Consider
using a `createSafeHTML()` helper or template literals with auto-escaping.

---

## INFORMATIONAL

### I-1: 2FA Infrastructure Incomplete

The status.md mentions 2FA DB columns exist but are incomplete. Until 2FA is
implemented, accounts rely solely on password authentication. This is acceptable
for self-hosted instances but should be prioritized for multi-user deployments.

### I-2: SVG Validation Uses String Matching

**File:** `internal/extensions/security.go:253-269`
SVG validation checks for `<script`, `javascript:`, and `on*=` using string
matching on lowercased content. While this catches common attack vectors,
sophisticated SVG XSS can bypass string-based detection (e.g., character
encoding, CDATA sections, XML entity expansion). Consider using an SVG
sanitizer library or parsing the SVG as XML.

### I-3: No Database Connection TLS

**File:** `internal/config/config.go:83-95`
The MariaDB DSN configuration does not include TLS parameters. In a self-hosted
setup where the database is on the same host or same Docker network, this is
acceptable. For production deployments with remote databases, TLS should be
configured.

### I-4: WASM Plugin Security Model

The WASM plugin system has good security controls (capability-based permissions,
memory limits, timeout enforcement, input size limits). The capability-based
host function filtering follows least-privilege principles. No issues found.

### I-5: Export/Import Data Integrity

Campaign export/import (`internal/plugins/campaigns/export.go`) handles data
serialization. Import should be treated as untrusted input. The current
implementation uses the standard service layer for imports, which provides
validation. No injection issues found.

---

## Positive Findings (What's Done Well)

1. **Password Hashing:** argon2id with OWASP-recommended parameters (64MB memory,
   3 iterations, 4 threads). Constant-time comparison. Excellent.

2. **Session Management:** Cryptographically random 256-bit tokens, stored in Redis
   with configurable TTL. HttpOnly + Secure + SameSite=Lax cookies. Session
   invalidation on password reset.

3. **SQL Injection Prevention:** All repository queries use parameterized `?`
   placeholders. Dynamic WHERE clauses are built with parameterized args. The
   `OrderByClause()` method uses a whitelist switch. LIKE queries escape `%` and `_`.
   No SQL injection vectors found.

4. **CSRF Protection:** Double-submit cookie with 256-bit tokens, constant-time
   comparison, properly skips API routes (which use Bearer auth).

5. **HTML Sanitization:** Bluemonday UGC policy with carefully scoped allowlists
   for Chronicle-specific attributes. Applied before storage. `templ.Raw()` usage
   appears to be only on pre-sanitized content.

6. **Media Security:** HMAC-signed URLs, MIME type allowlist, ClamAV virus scanning,
   defense-in-depth headers (CSP, nosniff, X-Frame-Options), filename sanitization,
   concurrent upload limits, disk space checks.

7. **Extension Security:** Zip extraction validates paths (no traversal), file
   extension allowlist, size limits, SVG script detection, CSS injection prevention.

8. **HTTP Security Headers:** Full set including CSP, HSTS, X-Frame-Options,
   Referrer-Policy, Permissions-Policy, X-Content-Type-Options.

9. **Error Handling:** Domain error types prevent raw DB error leakage. Recovery
   middleware catches panics and returns generic 500. Login errors use non-revealing
   messages ("invalid email or password").

10. **API Authentication:** Sync API uses Bearer tokens validated with bcrypt.
    CORS properly refuses wildcard + credentials combination.

---

## Remediation Priority

| ID  | Severity | Effort | Priority |
|-----|----------|--------|----------|
| H-1 | High     | Low    | **Fix immediately** |
| H-2 | High     | Low    | **Fix immediately** |
| M-1 | Medium   | Low    | Fix this sprint |
| M-4 | Medium   | Low    | Fix this sprint |
| M-5 | Medium   | Low    | Fix this sprint |
| M-2 | Medium   | Medium | Plan for next sprint |
| M-3 | Medium   | Low    | Fix this sprint |
| L-1 | Low      | Low    | Fix when touching auth |
| L-2 | Low      | N/A    | N/A — ClamAV removed |
| L-5 | Low      | Low    | Fix when convenient |
| L-6 | Low      | Low    | Fix when convenient |
| L-7 | Low      | Low    | Fix when convenient |
| L-3 | Low      | N/A    | Accept for self-hosted |
| L-4 | Low      | Medium | Accept for now |
| L-8 | Low      | Medium | Gradual improvement |
