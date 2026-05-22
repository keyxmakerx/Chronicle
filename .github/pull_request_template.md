**Cites:** <e.g., 2026-05-21-core-tenets §T-B2, §T-O2; reports/chronicle/2026-05-21-c-hygiene-audit.md §0.5 D3>
**Security implication:** <one line; can be "none — pure refactor / CSS / docs">
**Consumer-verified:** <file:line citation if this PR specifies a wire surface; "n/a" otherwise>
**Mockup:** <path/to/mockups/file.html if UI-touching; "n/a" otherwise>

## What this changes

<2-3 sentence summary. Active voice. Focus on the WHY not the WHAT.>

## Why

<Cite the tenet, audit finding, or decision that motivated this work. Link to the binding doc.>

## Test plan

- [ ] Local verification step 1
- [ ] Local verification step 2
- [ ] CI passes (lint, tests, build)
- [ ] If UI-touching: verified in browser; mockup behavior matches; reduced-motion respected

## Tenet self-check

- [ ] T-B1 security: PR considers auth/input/validation impact (or declared n/a above)
- [ ] T-B2 plugin isolation: no new `"foundry-vtt"` / `"foundry-module"` / `"foundry_vtt"` string literals outside `internal/plugins/foundry_vtt/*`; no new cross-plugin imports
- [ ] T-B3 production UI: any UI change has transition + loading + error states; accessibility considered
- [ ] T-B4 dual-audience docs: any doc changed serves both humans and AI sessions

## Stop-and-flag

If during review you find this PR violates a tenet that the author missed, flag it explicitly in a review comment citing the tenet by number.
