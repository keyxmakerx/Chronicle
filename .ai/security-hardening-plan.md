# Security Hardening Plan

**Created:** 2026-03-09
**Branch:** `claude/dynamic-database-setup-1QSPA`
**Context:** Follow-up to security audit (`.ai/security-audit-2026-03-06.md`) and codebase audit.
Many original audit findings have already been fixed. This plan addresses remaining items
plus new findings from HTMX security research and web security best practices review.

---

## Status of Original Audit Findings

Before planning, here's what's already been resolved:

| ID | Finding | Status |
|----|---------|--------|
| H-1 | Session tokens in admin HTML | **FIXED** — uses `TokenHash` now |
| H-2 | RecapHTML stored XSS | **FIXED** — `sanitize.HTML()` in service |
| M-1 | ChangePassword doesn't invalidate sessions | **FIXED** — calls `destroyUserSessions()` |
| M-3 | Session cookie MaxAge hardcoded | **FIXED** — uses config `sessionTTL` |
| M-4 | No registration rate limiting | **FIXED** — rate limiting applied |
| M-5 | Password reset not rate-limited per email | **FIXED** — Redis per-email limiting |
| L-1 | Avatar trusts client Content-Type | **FIXED** — uses `http.DetectContentType()` |
| L-7 | Dockerfile runs as root | **FIXED** — non-root `chronicle` user |

**Remaining open items from audit:** M-2, L-2, L-3, L-4, L-5, L-6, L-8

---

## Phase 1: Quick Security Wins (Sprint 1)

Low-effort, high-impact changes that can be done in one session.

### 1.1 — HTMX Config Hardening ✅ DONE (2026-03-25)
**File:** `static/js/boot.js`

All four HTMX security configs are now set in `boot.js`:
- `selfRequestsOnly = true` (done in sprint 59)
- `allowScriptTags = false` (done in sprint 59)
- `historyCacheSize = 0` (done in sprint 59)
- `allowEval = false` (done in sprint 63 — this session)

### 1.2 — Cross-Origin Security Headers
**File:** `internal/middleware/security.go`
**Effort:** 5 minutes

Add COOP header to `SecurityHeaders()`:
```go
h.Set("Cross-Origin-Opener-Policy", "same-origin")
```

Defends against Spectre-class side-channel attacks (XS-Leaks, Spectre).

**Note on CORP:** We intentionally do NOT set `Cross-Origin-Resource-Policy` globally.
Foundry VTT (configured via `BASE_URL` env var) makes cross-origin API requests.
CORP would block those even with proper CORS headers. COOP alone is safe — it only
isolates the browsing context, not resource loading. The CORS middleware already
respects `BASE_URL` as the allowed origin for reverse proxy domain configuration.

### 1.3 — Tighten CSP: Replace unpkg.com Wildcard with Specific URLs
**File:** `internal/middleware/security.go`
**Effort:** 15 minutes

Currently CSP allows all of `https://unpkg.com` as script/style/img source. Only
Leaflet (1.9.4) and MarkerCluster (1.5.3) are loaded from there. Replace the wildcard
domain with specific versioned paths:

```
script-src 'self' 'unsafe-inline' 'unsafe-eval' https://unpkg.com/leaflet@1.9.4/ https://unpkg.com/leaflet.markercluster@1.5.3/;
style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com https://unpkg.com/leaflet@1.9.4/ https://unpkg.com/leaflet.markercluster@1.5.3/;
img-src 'self' data: blob: https://unpkg.com/leaflet@1.9.4/;
```

This prevents an attacker from loading arbitrary npm packages via an injection point.

### 1.4 — Inline CSRF Cookie Parsing → Chronicle.getCsrf()
**Files:** `calendar.templ`, `maps.templ`, `timeline.templ` (7 locations)
**Effort:** 20 minutes

Seven `.templ` files duplicate the CSRF cookie regex:
```js
var m = document.cookie.match('(?:^|; )chronicle_csrf=([^;]*)');
```

Replace with:
```js
var headers = {'Content-Type': 'application/json', 'X-CSRF-Token': Chronicle.getCsrf()};
```

This reduces duplication and ensures all CSRF handling flows through `boot.js`, making
future changes (like `__Host-` prefix rename) a single-point update.

---

## Phase 2: Defense Hardening (Sprint 2)

Moderate-effort improvements to close remaining audit gaps.

### 2.1 — `__Host-` CSRF Cookie Prefix (L-4)
**Files:** `internal/middleware/csrf.go`, `static/js/boot.js`, all `.templ` files
  referencing `chronicle_csrf` (should be zero after 1.4)
**Effort:** 30 minutes

Rename CSRF cookie from `chronicle_csrf` to `__Host-chronicle_csrf`. The `__Host-`
prefix enforces: Secure=true, no Domain attribute, Path=/. This prevents subdomain
cookie injection attacks.

**Requirements:**
- Cookie must have `Secure: true` (already conditional on TLS/proxy)
- Cookie must have `Path: "/"` (already set)
- Cookie must NOT set `Domain` (already not set)
- Update `boot.js` getCsrf() and htmx:configRequest to use new name
- After Phase 1.4, no `.templ` files should reference the old name

**Risk:** Only works over HTTPS. Self-hosted users on plain HTTP (no reverse proxy)
would lose CSRF protection. Add fallback: use `__Host-` prefix only when Secure=true.

### 2.2 — Sensitive Query Parameter Redaction in Logs (L-5) ✅ DONE (2026-03-25)
**File:** `internal/middleware/logging.go`

Redaction implemented in sprint 59. Sprint 63 added `api_key` to the sensitive params list.
Current list: `token`, `key`, `password`, `secret`, `api_key`.

### 2.4 — Default DB Password Fails in Production (L-6)
**File:** `internal/config/config.go`
**Effort:** 10 minutes

Change the default DB password check from a warning to a hard failure in production
mode, matching the existing behavior for the secret key.

### 2.5 — Verify bluemonday Strips hx-* Attributes
**File:** `internal/sanitize/sanitize.go` (or equivalent)
**Effort:** 20 minutes

HTMX attributes (`hx-get`, `hx-post`, `hx-on:*`, `data-hx-*`) in user-generated HTML
could be used for request forgery or script execution. Verify that the bluemonday UGC
policy strips these attributes. If not, add explicit stripping.

Also verify that `<meta name="htmx-config">` is stripped (bluemonday UGC policy should
strip `<meta>` by default, but worth confirming).

---

## Phase 3: Advanced Hardening (Sprint 3)

Higher-effort improvements for defense-in-depth. Lower priority but valuable.

### 3.1 — innerHTML Audit in JS Widgets (L-8) ✅ DONE (2026-03-25)
**Files:** `static/js/widgets/*.js`, `static/js/*.js`, `extensions/dice-roller/`

Full audit completed in sprint 63. ~330 innerHTML assignments across 35+ files reviewed.
Fixes applied:
- **Critical:** `aliases.js` — `Chronicle.escapeHTML` typo → `Chronicle.escapeHtml`
- **Escape consolidation:** Removed 6 local `escapeHtml` duplicates in `template_picker.js`,
  `command_palette.js`, `inventory.js`, `layout_editor.js`, `template_editor.js`,
  `timeline_viz.js`. All now use `Chronicle.escapeHtml()` from `boot.js`.
- **Missing escaping:** Added `Chronicle.escapeHtml()` for entity type icons in
  `entity_manager.js` and `inventory.js`. Added `Chronicle.escapeAttr()` for
  `type_color` in `editor_mention.js` and `inventory.js`.
- **Attribute context:** Changed `escapeHtml` → `escapeAttr` for `href` attribute in
  `timeline_widget.js`.
- **Verified safe:** ~95% of instances are static HTML (icons, spinners), element clearing,
  server-sanitized content, or already properly escaped.

### 3.2 — Per-User Login Throttling ✅ DONE (2026-03-25)
**File:** `internal/plugins/auth/service.go`

Per-email Redis-backed throttling implemented in sprint 59. Sprint 63 enhanced from
hard-reject to progressive delays:
- After 10 failures in 15 min: delay doubles each attempt (2s, 4s, 8s... capped at 5min)
- Uses `time.After` with `ctx.Done()` select for graceful cancellation
- Clears on successful login (existing behavior)
- Redis key: `login_failures:{sha256(email)}` with 15-min TTL

### 3.3 — Vendor Leaflet Locally (Eliminate unpkg.com Entirely) ✅ DONE (prior sprint)
**Files:** `static/vendor/leaflet.js`, `static/vendor/leaflet.markercluster.js`

Leaflet and MarkerCluster already vendored locally. CSP has no unpkg.com reference.
Maps template loads from `/static/vendor/`. No action needed.

### 3.4 — SVG Sanitizer Library (I-2)
**File:** `internal/extensions/security.go`
**Effort:** 1-2 hours

Replace string-based SVG XSS detection with a proper SVG sanitizer that parses as XML.
Current regex-based approach can be bypassed with encoding tricks, CDATA sections, or
XML entity expansion.

---

## Out of Scope (Documented, Not Planned)

These items are acknowledged but intentionally deferred:

| Item | Reason |
|------|--------|
| **Nonce-based CSP (M-2)** | Requires Alpine.js migration or replacement. High effort, architectural change. Tracked separately. |
| **Redis-based rate limiting (L-3)** | Only needed for multi-instance deployments. Chronicle is single-instance self-hosted. |
| **DB connection TLS (I-3)** | Handled by reverse proxy / Docker network. Document in deployment guide. |
| **2FA implementation (I-1)** | Separate feature, not a hardening task. Already on roadmap. |
| **Post-quantum TLS** | Future concern. Monitor NIST PQC standards. |

---

## Summary

| Phase | Sprint | Items | Estimated Effort |
|-------|--------|-------|-----------------|
| **1: Quick Wins** | 1 | HTMX config, COOP/CORP headers, CSP tightening, CSRF dedup | ~50 minutes |
| **2: Defense Hardening** | 2 | __Host- prefix, log redaction, DB password fail, hx-* sanitization | ~1.5 hours |
| **3: Advanced** | 3 | innerHTML audit, per-user throttling, vendor Leaflet, SVG sanitizer | ~5 hours |

All changes are backwards-compatible and can be deployed incrementally.
