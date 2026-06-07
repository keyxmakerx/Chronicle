# worldstate widget + provider

**Files:** `worldstate.js` (the widget), `worldstate_provider.js` (the singleton).
**Dispatch:** C-CAL-WORLDSTATE-WIDGETS (Phase 6 widgetization of the World-State
Production Arc).

## What it is

Graduates the showcase worldState renderers (sky band + hourglass-on-shelf, the
"mini shelf view") into a production widget that mounts on any page via
`data-widget="worldstate"` (boot.js auto-mount). It is the third entity-able
"view" of the time system, alongside the calendar (`entity_calendar`) and the
timeline (Tuner showcase).

## Architecture

```
[server] entity_worldstate block (internal/plugins/calendar/entity_worldstate_block.*)
   ├─ embeds the #401 worldState seed on #cal-v2-worldstate  (engine prod mode + zero-fetch)
   ├─ renders the band + hourglass-shelf scaffold (reuses calendar_v2_worldstate templ)
   └─ data-widget="worldstate" data-campaign-id=… data-variant="hourglass"
        │
[client] boot.js → Chronicle.register('worldstate')  (worldstate.js)
   ├─ ChronicleWorldState.get(campaignId)            (worldstate_provider.js — singleton)
   │     • ONE fetch of GET /campaigns/:id/calendar/world-state per page (memoized promise)
   │     • OR adopts the server seed (#cal-v2-worldstate) → ZERO fetch
   │     • subscribe(fn) / onError(fn) / current() / push(seed) / onFrame(fn, shared rAF)
   │     • reduced-motion: no rAF; destroy() on last unsubscribe
   └─ on each provider update → window.__calSetWorldState(seed)   (drives the SHARED engine,
                                                                   static/js/cal-almanac.js)
```

**Engine reuse, not rebuild:** the widget does NOT reimplement the sky/sun/moon/
weather/hourglass rendering — it reuses `static/js/cal-almanac.js` (the validated
particle engine + effect registries) exactly as `calendar_v2` does, driving it
via `window.__calSetWorldState`. The provider is the only new data layer.

## Why a provider singleton

Multiple worldstate widgets can mount on one page; each fetching independently
would hammer the API and desync. They all read the one provider, which fetches
at most once. The single-fetch behavior is pinned by
`test/js/worldstate_provider.test.mjs`.

## Footguns

- **Fixed engine id:** the engine binds `#cal-v2-worldstate` (one per page), so
  the block is `Singleton` and you pick `entity_calendar` OR `entity_worldstate`
  per page, not both.
- **First paint vs live updates:** the first frame comes from the server seed
  blob (engine prod mode at init); `__calSetWorldState` is a no-op until the
  engine has booted, which is fine — the blob already painted.
- **CSS:** the rendering canvas reuses the already-exempt `cal-almanac.css`
  (rendering-canvas exemption marker, `decisions/2026-06-05-…`); chrome stays on
  V2 tokens. We deliberately did NOT duplicate the ~1700-line exempt canvas CSS
  into a new file.

## Tests

- `test/js/worldstate_provider.test.mjs` — single-fetch, seed→zero-fetch,
  subscribe/onError/current/push, shared rAF + reduced-motion, self-destroy.
- `test/js/worldstate_widget.test.mjs` — register/init/destroy, engine drive,
  fetch-once, error state.
- `internal/plugins/calendar/entity_worldstate_block_test.go` — block render,
  empty CTA, campaign-level (no-entity) render, unavailable state.

Visual fidelity is the operator's local gate (no headless browser in CI).
