// cal-almanac.js — C-CAL-SHOWCASE-DESIGN-1-ALMANAC + REFINEMENT-V2..V5 +
// WORLD-STATE WAVE 0.
//
// Per-interaction INIT_BLOCKS, each wrapped in its own try/catch so one
// failing handler can't kill the rest. Mock data is read from the inline
// JSON the templ emits; nothing here talks to a backend.
//
// WAVE 0 modules (synced world-state spine):
//   world-state            — the single `worldState` object (CATALOG Part 8)
//     + setWorldState(patch) pub/sub front door (changedKeys-gated notify)
//   world-state-subscribers — sky-band + hourglass both subscribe; one
//     setWorldState() call re-renders BOTH in back→front layer order
//   unified-effects        — ONE EFFECTS registry (per-surface renderers
//     skyBand/hgTop/hgBottom/hgSand/timeline); WEATHER_EFFECTS +
//     CELESTIAL_EFFECTS are now thin projections over it (additive)
//   applyTime / renderSkyForDay are thin shims into setWorldState, so every
//     existing entry point (drag, tick, day-nav, override, demo) is preserved
//
// REFINEMENT-V2 modules:
//   weather-registry / celestial-registry — WEATHER_EFFECTS +
//     CELESTIAL_EFFECTS registries (MUST renderFns + TBD stubs)
//   sky-band-ambient — re-renders the sky's weather + celestial layers
//     for the displayed day; always-animating via CSS
//   sun-drag-scrub / time-label-input — interactive time (no slider bar)
//   era-vignette — corner vignette click → era detail
//   snowglobe-timepiece — the small companion widget's drag + tick
//   popup-slidein / popup-expand — two-tier quick-view → editor
//   action-menu — the 8-action menu + its sub-popovers
//   widget-drag / widget-resize / drag-create / month-nav — carried.

(function () {
  'use strict';

  var INIT_BLOCKS = [];
  function registerInitBlock(name, runner) { INIT_BLOCKS.push({ name: name, runner: runner }); }
  function runAll() {
    var r = [];
    for (var i = 0; i < INIT_BLOCKS.length; i++) {
      var b = INIT_BLOCKS[i];
      if (PROD && PROD_SKIP[b.name]) { r.push({ name: b.name, status: 'SKIPPED_PROD' }); continue; }
      try { b.runner(); r.push({ name: b.name, status: 'OK' }); }
      catch (err) { r.push({ name: b.name, status: 'FAILED', error: (err && err.message) || String(err) });
        try { console.error('[cal-almanac]', b.name, err); } catch (e) {} }
    }
    return r;
  }
  // teardownProd tears down the live production surfaces/subscribers/ticks/
  // listeners before a re-init (C-WIDGET-BINDING-QA2). The worldstate band is
  // injected by hx-boost navigation + the P4b binding swap; under
  // htmx.config.allowScriptTags=false the band's <script> never re-runs, so the
  // already-loaded engine must re-bind itself to the freshly-swapped band. We
  // destroy the prior surfaces (and disconnect their IO) + clear the subscriber
  // and tick lists + drop the cursor listener so nothing accumulates across
  // navigations and no rAF paints to a detached canvas.
  // W1 (E3): ResizeObservers created by the prod init blocks (particle canvas,
  // era overlay, hourglass) were local vars, never disconnected — so each
  // boosted-nav re-init left the old observers firing resize()/refeed against
  // reassigned globals. Track them here so teardownProd can disconnect them.
  var PROD_OBSERVERS = [];
  function trackObserver(ro) { if (ro) PROD_OBSERVERS.push(ro); return ro; }
  function teardownProd() {
    try { if (SKY_SURFACE && SKY_SURFACE.destroy) SKY_SURFACE.destroy(); } catch (e) {}
    try { if (SKY_FRONT && SKY_FRONT.destroy) SKY_FRONT.destroy(); } catch (e) {}
    try { if (GLASS_SURFACE && GLASS_SURFACE.destroy) GLASS_SURFACE.destroy(); } catch (e) {}
    SKY_SURFACE = null; SKY_FRONT = null; GLASS_SURFACE = null;
    for (var oi = 0; oi < PROD_OBSERVERS.length; oi++) { try { PROD_OBSERVERS[oi].disconnect(); } catch (e) {} }
    PROD_OBSERVERS.length = 0;
    WS_SUBS.length = 0;
    try { if (window.CalParticleEngine && CalParticleEngine.resetTicks) CalParticleEngine.resetTicks(); } catch (e) {}
    if (cursorSyncHandler) {
      try { document.removeEventListener('cal:cursor-change', cursorSyncHandler); } catch (e) {}
      cursorSyncHandler = null;
    }
    worldState = null;
  }

  function init() {
    // PRODUCTION (live calendar_v2 + entity embeds): the band carries the seed
    // on a [data-cal-worldstate] element (E7: read by ATTRIBUTE, not a fixed id
    // — entity embeds namespace their id per band so multi-embed pages stay
    // valid; the singleton engine binds the FIRST band). Re-init PER BAND NODE
    // so a boosted-nav / P4b swap that injects a fresh band re-paints (the prior
    // band is torn down first).
    var band = (typeof document !== 'undefined' && document.querySelector)
      ? document.querySelector('[data-cal-worldstate]') : null;
    if (band) {
      if (band.__calInited) return;   // this exact band node is already live
      teardownProd();                 // clean up a previous band's engine state
      window.__calAlmanacResults = runAll();
      band.__calInited = true;
      window.__calAlmanacInited = true;
      return;
    }
    // DEMO / no-band page: init exactly once (unchanged behavior).
    if (window.__calAlmanacInited) return;
    window.__calAlmanacResults = runAll();
    window.__calAlmanacInited = true;
  }

  var DATA = null;
  // The displayed day (operator can click around the grid). Defaults to
  // the calendar's current day. Time-of-day is a 0..1 fraction.
  // VIEW is the legacy v3/v4/v5 mirror; Wave 0 makes `worldState` the single
  // source of truth and keeps VIEW in lockstep so existing code that still
  // reads VIEW (currentSunState, the render pipelines) sees the same values.
  var VIEW = { year: 0, month: 0, day: 0, timeFrac: 0.5 };

  // ============================================================
  // PRODUCTION MODE (C-CAL-WORLDSTATE-PRODUCTION-PORT, Phase 2a).
  // The same engine drives BOTH the /demo showcase AND the live
  // calendar_v2 surface. In production the server hands us the
  // worldState seed directly (BuildWorldStateSeed → CATALOG Part-8
  // shape) on `#cal-v2-worldstate`, instead of the mock DATA blob the
  // demo builds worldState from. PROD is detected in the 'data' block.
  //
  // PROD_SKIP lists the demo-only / interaction init blocks that must
  // NOT run on production (the GM controls are Phase 4; the day-pane /
  // editor interaction is Phase 2b/2c). Skipping them here — rather than
  // guarding each block — keeps the cut in one auditable place and means
  // demo-controls genuinely do not ship to prod. The /demo path is
  // unaffected (PROD stays false there).
  var PROD = false;
  var PROD_SEED = null;
  var PROD_SKIP = {
    'time-control': 1, 'date-setter': 1, 'time-control-hotkey': 1, // time/date controls (2c / Phase 4)
    'popup-slidein': 1, 'popup-expand': 1, 'popup-create-flow': 1, // two-tier pane + editor (2b)
    'drag-create': 1, 'action-menu': 1, 'visibility-editor': 1, 'sky-panel': 1, // interaction (2b)
    'dialog-a11y': 1, // no dialogs on the read-only 2a surface
    'widget-drag': 1, 'widget-resize': 1, 'month-nav': 1, 'demo-controls': 1 // showcase-only chrome
  };

  // ============================================================
  // WAVE 0 — the shared world-state model + pub/sub (CATALOG Part 8).
  // ============================================================
  // ONE object drives BOTH surfaces (sky-band + hourglass). Change it once
  // via setWorldState(patch) → every subscriber re-renders. This replaces
  // the v3/v4/v5 implicit sync (which monkey-patch-wrapped applyTime +
  // renderSkyForDay); those are now thin shims into setWorldState so every
  // existing caller (drag-scrub, time-input, tick, day-nav,
  // weather-override, demo-controls) is preserved verbatim.
  var worldState = null;     // seeded from DATA/VIEW in the 'world-state' block
  var WS_SUBS = [];          // ordered surface subscribers
  // C-WIDGET-BINDING-QA2: the cursor-sync document listener, held so a prod
  // re-init (htmx:afterSettle) can remove it before re-binding — otherwise
  // listeners accumulate across boosted navigations.
  var cursorSyncHandler = null;

  function subscribeWorldState(fn) { if (typeof fn === 'function') WS_SUBS.push(fn); }

  // Structural equality for the small one-level world-state values (numbers,
  // strings, {…} param bags, and the moons[]/events[] arrays). Used to skip
  // no-op patches so a subscriber never re-renders on an unchanged value.
  function wsEqual(a, b) {
    if (a === b) return true;
    if (!a || !b || typeof a !== 'object' || typeof b !== 'object') return false;
    var aArr = Array.isArray(a), bArr = Array.isArray(b);
    if (aArr || bArr) {
      if (!aArr || !bArr || a.length !== b.length) return false;
      for (var i = 0; i < a.length; i++) if (!wsEqual(a[i], b[i])) return false;
      return true;
    }
    var ka = Object.keys(a), kb = Object.keys(b);
    if (ka.length !== kb.length) return false;
    for (var j = 0; j < ka.length; j++) {
      if (!Object.prototype.hasOwnProperty.call(b, ka[j])) return false;
      if (!wsEqual(a[ka[j]], b[ka[j]])) return false;
    }
    return true;
  }
  // Field-merge a patch value onto the current value so a partial patch
  // ({weather:{type:'rain'}}) keeps the untouched fields (intensity). Arrays
  // and scalars replace wholesale; only plain objects merge.
  function wsMerge(cur, patchVal) {
    if (patchVal && typeof patchVal === 'object' && !Array.isArray(patchVal) &&
        cur && typeof cur === 'object' && !Array.isArray(cur)) {
      var merged = {}, k;
      for (k in cur) if (Object.prototype.hasOwnProperty.call(cur, k)) merged[k] = cur[k];
      for (k in patchVal) if (Object.prototype.hasOwnProperty.call(patchVal, k)) merged[k] = patchVal[k];
      return merged;
    }
    return patchVal;
  }
  // THE front door. Shallow-merges patch into worldState and notifies
  // subscribers with the list of keys that ACTUALLY changed. A no-op patch
  // (same value) notifies nobody — keeps per-effect renders cold unless the
  // state truly moved. Returns the changedKeys array (used by tests).
  function setWorldState(patch) {
    if (!worldState || !patch || typeof patch !== 'object') return [];
    var changed = [];
    for (var k in patch) {
      if (!Object.prototype.hasOwnProperty.call(patch, k)) continue;
      var next = wsMerge(worldState[k], patch[k]);
      if (!wsEqual(worldState[k], next)) { worldState[k] = next; changed.push(k); }
    }
    if (!changed.length) return changed;
    // Mirror into VIEW before notifying so currentSunState() and the render
    // pipelines read one consistent truth.
    if (worldState.date) { VIEW.year = worldState.date.year; VIEW.month = worldState.date.month; VIEW.day = worldState.date.day; }
    if (typeof worldState.timeOfDay === 'number') VIEW.timeFrac = worldState.timeOfDay;
    for (var i = 0; i < WS_SUBS.length; i++) {
      try { WS_SUBS[i](worldState, changed); }
      catch (e) { try { console.error('[cal-almanac] ws-subscriber', e); } catch (e2) {} }
    }
    return changed;
  }
  window.__calSetWorldState = setWorldState;
  window.__calSubscribeWorldState = subscribeWorldState;

  // The back→front render-resolution order (CATALOG Part 0). Both surfaces
  // compose layers in this order; the subscriber registration order in
  // 'world-state-subscribers' honours it. resolveLayers(state) returns the
  // layers currently ACTIVE for a state — used by tests now and by later
  // waves to drive layer painting; some layers are structural no-ops until
  // their wave lands (season=W1, moodTint=W2, timeControl=W3).
  var WS_LAYER_ORDER = ['timeOfDay', 'season', 'celestial', 'weather', 'events', 'moodTint', 'timeControl'];
  function resolveLayers(state) {
    state = state || worldState || {};
    return WS_LAYER_ORDER.filter(function (layer) {
      switch (layer) {
        case 'timeOfDay': return typeof state.timeOfDay === 'number';
        case 'season': return !!state.season;
        case 'celestial': return (state.sun != null) || (state.moons && state.moons.length > 0);
        case 'weather': return !!(state.weather && state.weather.type && state.weather.type !== 'clear');
        case 'events': return !!(state.events && state.events.length);
        case 'moodTint': return !!(state.moodTint && state.moodTint.color && state.moodTint.intensity > 0);
        case 'timeControl': return !!(state.timeControl && (state.timeControl.direction !== 1 || state.timeControl.speed !== 1));
        default: return false;
      }
    });
  }
  window.__calLayerOrder = WS_LAYER_ORDER;
  window.__calResolveLayers = resolveLayers;

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function key(y, m, d) { return y + '-' + m + '-' + d; }
  function pad2(n) { return String(n).padStart(2, '0'); }

  // ============================================================
  // REFINEMENT-V4 — Shared canvas particle engine (§B)
  // ============================================================
  // ONE rAF loop drives EVERY particle surface (the sky-band canvas + the
  // in-glass hourglass canvas). Effects are DATA (`particleSpec`), not
  // code paths — the registries supply specs; the engine owns the loop,
  // the pool, and every perf guard. This is design-neutral infrastructure
  // (no `.cal-almanac-*` coupling) so the Timeline Tuner can lift it
  // verbatim; coordinator decides whether to extract to a shared file.
  //
  // Perf discipline (binding, §B3): single shared rAF; pooled particles
  // (never GC-churned); global live cap; IntersectionObserver pause when
  // offscreen; visibilitychange pause; DPR clamped 2×; reduced-motion
  // renders ONE static frame and never starts the loop; low-power
  // auto-detect drops to a 20-particle profile.
  var CalParticleEngine = (function () {
    var PROFILES = { high: 80, normal: 40, low: 20 };
    // Slice 2: live reduced-motion — re-read on OS-setting change mid-session
    // (was read once at module-eval). On RM-on we freeze to a static frame; on
    // RM-off we resume the loop.
    var prefersReduced = false;
    (function () {
      try {
        var mq = window.matchMedia('(prefers-reduced-motion: reduce)');
        prefersReduced = mq.matches;
        var onRMChange = function () {
          prefersReduced = mq.matches;
          if (prefersReduced) { if (rafId) { cancelAnimationFrame(rafId); rafId = null; } running = false; surfaces.forEach(drawStaticFrame); }
          else sync();
        };
        if (mq.addEventListener) mq.addEventListener('change', onRMChange);
        else if (mq.addListener) mq.addListener(onRMChange);
      } catch (e) {}
    })();
    // Forced-state proof classes can pin reduced-motion in headless shots.
    function reducedNow() {
      if (prefersReduced) return true;
      try { return !!document.querySelector('.cal-almanac--proof-reduced-motion'); } catch (e) { return false; }
    }
    var autoLow = (function () {
      try { return (navigator.hardwareConcurrency || 8) <= 4; } catch (e) { return false; }
    })();

    var surfaces = [];          // { canvas, ctx, w, h, dpr, emitters, particles, pool, visible, clip }
    var running = false, rafId = null, lastTs = 0;
    // WAVE 3: shared-rAF tween hooks + global atmosphere-pause. addTick(fn)
    // registers a per-frame callback (dt seconds) — lets the time-control
    // tweens (~600ms advance / ~400ms reverse-sand) run on the ONE loop, no new
    // rAF. enginePaused freezes everything ("suspended in amber").
    var ENGINE_TICKS = [];
    var enginePaused = false;
    var profile = autoLow ? 'low' : 'normal';
    var globalCap = PROFILES[profile];
    var probeFrames = 0, probeOverBudget = 0;

    function liveCount() { var n = 0; for (var i = 0; i < surfaces.length; i++) n += surfaces[i].particles.length; return n; }

    function makeParticle() {
      return { x: 0, y: 0, vx: 0, vy: 0, life: 0, max: 1, size: 1, alpha: 1, shape: 'dot', color: '#fff', blur: 0, trail: false, blend: 'source-over', active: false };
    }
    function fromPool(s) { return s.pool.length ? s.pool.pop() : makeParticle(); }
    function toPool(s, p) { p.active = false; if (s.pool.length < 120) s.pool.push(p); }

    function rng(a, b) { return a + Math.random() * (b - a); }

    function createSurface(canvas, opts) {
      opts = opts || {};
      var ctx = canvas.getContext('2d');
      var s = { canvas: canvas, ctx: ctx, w: 0, h: 0, dpr: 1, emitters: [], particles: [], pool: [], visible: true, clip: opts.clip || null, spawnAcc: {}, frame: opts.frame || null, frameT: 0 };
      resize(s);
      surfaces.push(s);
      // Pause when the surface scrolls offscreen.
      try {
        if ('IntersectionObserver' in window) {
          var io = new IntersectionObserver(function (ents) {
            s.visible = ents[0] ? ents[0].isIntersecting : true;
            sync();
          }, { threshold: 0.01 });
          io.observe(canvas);
          s.io = io;
        }
      } catch (e) {}
      return {
        setEmitters: function (specs) { setEmitters(s, specs); },
        clear: function () { s.emitters = []; },
        resize: function () { resize(s); },
        staticFrame: function () { drawStaticFrame(s); },
        // WAVE 1: a per-surface frame hook drawn UNDER the particles on the
        // one shared rAF (used by the hourglass interior: heightmap sand +
        // day/night sky). Minimal addition — no scene-graph. A surface with a
        // frame keeps the loop alive even with zero particles; reduced-motion
        // draws it once via drawStaticFrame (dt=0 → the hook draws, doesn't step).
        setFrame: function (fn) { s.frame = fn || null; sync(); },
        destroy: function () { destroy(s); }
      };
    }

    function resize(s) {
      var r = s.canvas.getBoundingClientRect();
      var w = Math.round(r.width);
      var h = Math.round(r.height);
      // Defense-in-depth (C-WIDGET-BINDING-QA1 Bug 4): bail on a not-yet-laid-out
      // or absurd rect so a misconfigured/unbounded container can't feed a
      // degenerate size into the backing store — which throws "Canvas exceeds
      // max size" / "could not create basic draw target" and, under the
      // ResizeObserver, floods the console in a loop. This stands on its own
      // regardless of the layout fix.
      if (!isFinite(w) || !isFinite(h) || w <= 0 || h <= 0) return;
      var dpr = Math.min(window.devicePixelRatio || 1, 2); // clamp 2×
      // Cap the CSS dims so the backing store (dims × dpr) stays well under the
      // browser canvas limit (~32767px); 8192 device px is ample for a band.
      var MAX_DEVICE = 8192;
      var maxCSS = Math.floor(MAX_DEVICE / dpr);
      s.w = Math.min(w, maxCSS);
      s.h = Math.min(h, maxCSS);
      s.dpr = dpr;
      s.canvas.width = s.w * dpr;
      s.canvas.height = s.h * dpr;
      s.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    function setEmitters(s, specs) {
      s.emitters = [];
      s.spawnAcc = {};
      (specs || []).forEach(function (spec, i) {
        if (!spec) return;
        s.emitters.push({ id: i, spec: spec });
        s.spawnAcc[i] = 0;
      });
      if (reducedNow()) { drawStaticFrame(s); return; }
      sync();
    }

    // Spawn one particle from a spec into surface s.
    function spawn(s, spec) {
      if (liveCount() >= globalCap) return;
      var p = fromPool(s);
      var sz = spec.sizeRange || [1, 2];
      p.size = rng(sz[0], sz[1]);
      p.shape = spec.shape || 'dot';
      p.color = spec.color || '#fff';
      p.blur = spec.blur || 0;
      p.trail = !!spec.trail;
      p.blend = spec.blend || 'source-over';
      p.alpha = spec.alpha != null ? spec.alpha : 1;
      var vel = spec.velocity || { x: [0, 0], y: [40, 60] };
      p.vx = rng((vel.x || [0, 0])[0], (vel.x || [0, 0])[1]);
      p.vy = rng((vel.y || [0, 0])[0], (vel.y || [0, 0])[1]);
      // Spawn position: top edge for fallers, full-width for drifters.
      if (spec.spawn === 'stream') {
        // hourglass waist: spawn at the neck centre with slight scatter.
        p.x = s.w * 0.5 + rng(-s.w * 0.06, s.w * 0.06);
        p.y = s.h * (spec.streamTop != null ? spec.streamTop : 0.42);
      } else if (spec.spawn === 'sun') {
        // W1 (E1): anchor the sun-bloom sparkles to the PAINTED SUN position
        // (arcPos for the current time-of-day), with a tight scatter. The old
        // edge-spawn (vy<0 → left edge / else → top full-width) scattered the
        // bloom as "mini suns" across the whole band, on top of every weather.
        var ap = arcPos(VIEW.timeFrac);
        p.x = s.w * (ap.left / 100) + rng(-s.w * 0.035, s.w * 0.035);
        p.y = s.h * (ap.top / 100) + rng(-s.h * 0.06, s.h * 0.06);
      } else if (p.vy < 0 || spec.spawn === 'left') {
        p.x = rng(-s.w * 0.1, 0);
        p.y = rng(0, s.h);
      } else {
        p.x = rng(0, s.w);
        p.y = rng(-s.h * 0.1, p.shape === 'streak-long' ? 0 : -2);
      }
      p.life = 0;
      p.max = spec.maxLifeRange ? rng(spec.maxLifeRange[0], spec.maxLifeRange[1]) : 6;
      p.active = true;
      s.particles.push(p);
    }

    // Rate-limited error logging so one bad spec doesn't spam the console.
    var _engineLogged = {};
    function engineLogOnce(tag, e) {
      if (_engineLogged[tag]) return; _engineLogged[tag] = true;
      try { console.error('[cal-almanac] ' + tag, (e && e.message) || e); } catch (e2) {}
    }

    // Render ONE surface. Every inner loop is guarded so a throwing
    // particleSpec / frame is skipped, never escaping to kill the shared rAF.
    function renderSurface(s, dt) {
      var ctx = s.ctx;
      ctx.clearRect(0, 0, s.w, s.h);
      if (s.clip) { ctx.save(); s.clip(ctx, s.w, s.h); }
      // WAVE 1 frame hook — interior render (heightmap/day-night) UNDER particles.
      if (s.frame) { s.frameT += dt; try { s.frame(ctx, s.w, s.h, dt, s.frameT); } catch (e) { engineLogOnce('frame', e); } }
      // Emit — per-emitter guarded; a persistently-throwing emitter is disabled.
      for (var ei = 0; ei < s.emitters.length; ei++) {
        var em = s.emitters[ei];
        if (em.disabled) continue;
        try {
          var spec = em.spec, alive = 0;
          for (var k = 0; k < s.particles.length; k++) if (s.particles[k].emId === em.id) alive++;
          var rate = spec.spawnRate || 0;
          s.spawnAcc[em.id] = (s.spawnAcc[em.id] || 0) + rate * dt;
          while (s.spawnAcc[em.id] >= 1 && alive < (spec.maxAlive || 20) && liveCount() < globalCap) {
            s.spawnAcc[em.id] -= 1;
            var before = s.particles.length;
            spawn(s, spec);
            if (s.particles.length > before) { s.particles[s.particles.length - 1].emId = em.id; alive++; }
          }
        } catch (e) {
          em.errCount = (em.errCount || 0) + 1;
          engineLogOnce('emitter', e);
          if (em.errCount >= 3) { em.disabled = true; try { console.error('[cal-almanac] emitter disabled after repeated errors', em.spec); } catch (e2) {} }
        }
      }
      // Integrate + draw — per-particle guarded; a bad particle is recycled.
      for (var pi = s.particles.length - 1; pi >= 0; pi--) {
        var p = s.particles[pi];
        try {
          p.x += p.vx * dt; p.y += p.vy * dt; p.life += dt;
          var offscr = p.y > s.h + 12 || p.x > s.w + 80 || p.x < -80 || p.life > p.max;
          if (offscr) { s.particles.splice(pi, 1); toPool(s, p); continue; }
          drawParticle(ctx, p);
        } catch (e) { s.particles.splice(pi, 1); toPool(s, p); engineLogOnce('particle', e); }
      }
      if (s.clip) ctx.restore();
    }

    function step(ts) {
      if (!running) return;
      var dt = lastTs ? Math.min(0.05, (ts - lastTs) / 1000) : 0.016;
      lastTs = ts;
      // The whole body is guarded + the reschedule is in `finally`, so NOTHING
      // (a bad tick, spec, frame) can silently kill the shared rAF for both
      // surfaces — the loop always reschedules.
      try {
        // WAVE 3: drive shared-rAF tweens first (time-control transitions).
        for (var ti = ENGINE_TICKS.length - 1; ti >= 0; ti--) { try { ENGINE_TICKS[ti](dt); } catch (e) { engineLogOnce('tick', e); } }
        // Frame-time probe → auto low-power.
        if (probeFrames < 30) {
          probeFrames++;
          if (dt * 1000 > 24) probeOverBudget++;
          if (probeFrames === 30 && probeOverBudget > 10 && profile !== 'low') setProfile('low');
        }
        for (var si = 0; si < surfaces.length; si++) {
          var s = surfaces[si];
          if (!s.visible) continue;
          try { renderSurface(s, dt); } catch (e) { engineLogOnce('surface', e); try { if (s.clip) s.ctx.restore(); } catch (e2) {} }
        }
      } catch (e) { engineLogOnce('step', e); }
      finally {
        if (liveCount() > 0 || hasEmitters() || hasFrames() || ENGINE_TICKS.length) rafId = requestAnimationFrame(step);
        else { running = false; rafId = null; }
      }
    }

    function hasEmitters() { for (var i = 0; i < surfaces.length; i++) if (surfaces[i].emitters.length) return true; return false; }
    function hasFrames() { for (var i = 0; i < surfaces.length; i++) if (surfaces[i].frame) return true; return false; }

    function drawParticle(ctx, p) {
      ctx.globalAlpha = p.alpha;
      ctx.globalCompositeOperation = p.blend;
      ctx.fillStyle = p.color;
      ctx.strokeStyle = p.color;
      if (p.blur) { ctx.shadowBlur = p.blur; ctx.shadowColor = p.color; } else { ctx.shadowBlur = 0; }
      switch (p.shape) {
        case 'streak':
          ctx.lineWidth = p.size; ctx.beginPath();
          ctx.moveTo(p.x, p.y); ctx.lineTo(p.x - p.vx * 0.03, p.y - p.vy * 0.03); ctx.stroke();
          break;
        case 'streak-long':
          ctx.lineWidth = p.size; ctx.beginPath();
          ctx.moveTo(p.x, p.y); ctx.lineTo(p.x - p.vx * 0.12, p.y - p.vy * 0.12); ctx.stroke();
          break;
        case 'blob':
          ctx.beginPath(); ctx.arc(p.x, p.y, p.size, 0, 6.283); ctx.fill();
          break;
        case 'flake':
        case 'grain':
        case 'dot':
        default:
          ctx.beginPath(); ctx.arc(p.x, p.y, p.size, 0, 6.283); ctx.fill();
          break;
      }
      ctx.globalAlpha = 1; ctx.globalCompositeOperation = 'source-over'; ctx.shadowBlur = 0;
    }

    // One frozen representative frame for reduced-motion / static gate.
    function drawStaticFrame(s) {
      var ctx = s.ctx;
      ctx.clearRect(0, 0, s.w, s.h);
      if (s.clip) { ctx.save(); s.clip(ctx, s.w, s.h); }
      // WAVE 1: one static interior frame (dt=0 → the hook draws without stepping).
      if (s.frame) { try { s.frame(ctx, s.w, s.h, 0, s.frameT); } catch (e) {} }
      s.emitters.forEach(function (em) {
        var spec = em.spec, n = Math.min(spec.maxAlive || 6, 8);
        for (var i = 0; i < n; i++) {
          var p = makeParticle();
          p.size = rng((spec.sizeRange || [1, 2])[0], (spec.sizeRange || [1, 2])[1]);
          p.shape = spec.shape || 'dot'; p.color = spec.color || '#fff';
          p.blur = spec.blur || 0; p.alpha = (spec.alpha != null ? spec.alpha : 1) * 0.9;
          p.blend = spec.blend || 'source-over';
          p.x = rng(0, s.w); p.y = rng(0, s.h);
          var vel = spec.velocity || { x: [0, 0], y: [40, 60] };
          p.vx = (vel.x || [0, 0])[1]; p.vy = (vel.y || [40, 60])[1];
          drawParticle(ctx, p);
        }
      });
      if (s.clip) ctx.restore();
    }

    function sync() {
      if (enginePaused) return;                 // WAVE 3: frozen "in amber"
      if (reducedNow()) { surfaces.forEach(drawStaticFrame); return; }
      if (!running && (hasEmitters() || hasFrames() || ENGINE_TICKS.length) && anyVisible()) { running = true; lastTs = 0; rafId = requestAnimationFrame(step); }
    }
    function anyVisible() { for (var i = 0; i < surfaces.length; i++) if (surfaces[i].visible) return true; return false; }

    function destroy(s) {
      var idx = surfaces.indexOf(s); if (idx >= 0) surfaces.splice(idx, 1);
      try { if (s.io) s.io.disconnect(); } catch (e) {}
    }

    function setProfile(name) {
      if (!PROFILES[name]) return;
      profile = name; globalCap = PROFILES[name];
      // Trim live particles down to the new cap.
      while (liveCount() > globalCap) {
        for (var i = 0; i < surfaces.length && liveCount() > globalCap; i++) {
          if (surfaces[i].particles.length) toPool(surfaces[i], surfaces[i].particles.pop());
        }
      }
    }

    // Tab-hidden pause / resume.
    try {
      document.addEventListener('visibilitychange', function () {
        if (document.hidden) { if (rafId) { cancelAnimationFrame(rafId); rafId = null; } running = false; }
        else sync();
      });
    } catch (e) {}

    // WAVE 3: register a per-frame tween on the shared rAF; returns a remover.
    function addTick(fn) {
      if (typeof fn !== 'function') return function () {};
      ENGINE_TICKS.push(fn); sync();
      return function () { var i = ENGINE_TICKS.indexOf(fn); if (i >= 0) ENGINE_TICKS.splice(i, 1); };
    }
    // WAVE 3: global atmosphere-pause — freeze (stop the loop, hold the last
    // frame) or resume. Surfaces keep their last-drawn pixels = "in amber".
    function setPaused(b) {
      enginePaused = !!b;
      if (enginePaused) { if (rafId) { cancelAnimationFrame(rafId); rafId = null; } running = false; }
      else sync();
    }

    // Tab-hidden pause / resume (respects an explicit atmosphere-pause).
    try {
      document.addEventListener('visibilitychange', function () {
        if (document.hidden) { if (rafId) { cancelAnimationFrame(rafId); rafId = null; } running = false; }
        else if (!enginePaused) sync();
      });
    } catch (e) {}

    return {
      createSurface: createSurface,
      setProfile: setProfile,
      profile: function () { return profile; },
      cap: function () { return globalCap; },
      live: liveCount,
      reduced: reducedNow,
      sync: sync,
      addTick: addTick,
      setPaused: setPaused,
      paused: function () { return enginePaused; },
      // C-WIDGET-BINDING-QA2: clear all registered per-frame ticks. teardownProd
      // calls this on a re-init so hourglass ticks (and the rAF they keep alive)
      // from a swapped-out band don't accumulate across boosted navigations.
      resetTicks: function () { ENGINE_TICKS.length = 0; }
    };
  })();
  window.CalParticleEngine = CalParticleEngine;

  // ============================================================
  // Block: data
  // ============================================================
  registerInitBlock('data', function () {
    // PRODUCTION (2a): the server embeds the worldState seed on a
    // [data-cal-worldstate] element (E7: read by attribute, not a fixed id).
    // There is no mock DATA blob to navigate; the seed is the current-day
    // worldState. Stash it + seed VIEW, and give DATA a minimal stub so any
    // date-derived helper is safe (the demo navigation/recompute paths are
    // PROD_SKIP-ped, so DATA is unused).
    var prodNode = document.querySelector('[data-cal-worldstate]');
    if (prodNode) {
      PROD = true;
      PROD_SEED = JSON.parse(prodNode.getAttribute('data-cal-worldstate') || '{}');
      var pd = PROD_SEED.date || {};
      VIEW.year = pd.year || 0; VIEW.month = pd.month || 1; VIEW.day = pd.day || 1;
      VIEW.timeFrac = (PROD_SEED.timeOfDay != null) ? PROD_SEED.timeOfDay : 0.5;
      DATA = { current_year: VIEW.year, current_month: VIEW.month, current_day: VIEW.day,
        moons: [], moon_phases: [], celestial_events: {}, weather_types: [], weather_days: {} };
      return;
    }
    var node = document.getElementById('cal-almanac-data');
    if (!node) throw new Error('cal-almanac-data JSON node missing');
    // V5 BUGFIX: switched from `<script type="application/json">…body…` (where
    // templ doesn't interpolate `{ expr }`) to a `data-` attribute on a div
    // (which does interpolate). Fall back to textContent so any legacy
    // markup that ships the old script-tag form keeps working.
    var raw = node.getAttribute('data-cal-almanac-data') || node.textContent || '{}';
    DATA = JSON.parse(raw);
    VIEW.year = DATA.current_year;
    VIEW.month = DATA.current_month;
    VIEW.day = DATA.current_day;
    VIEW.timeFrac = (DATA.sky_time != null) ? DATA.sky_time : 0.5;
  });

  // ============================================================
  // Block: world-state (Wave 0) — seed `worldState` from DATA/VIEW so the
  // opening frame is byte-identical to v5. Runs right after 'data'.
  // ============================================================
  registerInitBlock('world-state', function () {
    // PRODUCTION (2a): the seed already IS the CATALOG Part-8 worldState
    // (BuildWorldStateSeed emits that exact shape — #401's parity test
    // pins it). Use it verbatim; the server omits the ephemeral
    // live-control fields (timepieceFill / timeControl direction-speed),
    // so default those client-side.
    if (PROD) {
      worldState = PROD_SEED;
      if (worldState.timepieceFill == null) worldState.timepieceFill = 0;
      if (worldState.atmospherePaused == null) worldState.atmospherePaused = false;
      if (!worldState.timeControl) worldState.timeControl = { direction: 1, speed: 1 };
      if (!worldState.moons) worldState.moons = [];
      if (!worldState.events) worldState.events = [];
      window.__calWorldState = worldState;
      return;
    }
    var wType = (function () { var w = dayWeatherTypeID(VIEW.month, VIEW.day); return w ? weatherEffectID(w) : 'clear'; })();
    worldState = {
      timeOfDay: VIEW.timeFrac,                                  // 0..1
      season: seasonName(),                                      // derived label
      date: { year: VIEW.year, month: VIEW.month, day: VIEW.day },
      sun: { tint: null },                                       // CATALOG Part 2 (Wave 2 fills tint)
      moons: (DATA.moons || []).map(function (mn) {              // 0..N dynamic (CATALOG §12.1)
        return {
          id: mn.id, name: mn.name,
          baseDesign: mn.base_design || 'moon-realistic-selene', // a MOON_DESIGNS id or emoji family
          tint: mn.tint || mn.color || null,                     // procedural fill-tint overlay
          phaseSource: mn.phase_source || 'css-clip',            // 'noto' | 'twemoji' | 'css-clip'
          size: mn.size || 1, orbitSpeed: mn.orbit_speed || 1, orbitOffset: mn.phase_offset || 0,
          phase: null, namedPhase: null, cyclePct: null,
          namedPhases: (DATA.moon_phases || []).filter(function (p) { return p.moon_id === mn.id; })
        };
      }),
      weather: { type: wType, intensity: 1 },                   // {type,intensity}
      events: celestialFor(VIEW.month, VIEW.day),               // can stack
      moodTint: { color: null, intensity: 0 },                  // player overlay (Wave 2)
      timeControl: { direction: 1, speed: 1 },                  // DM verb intent
      timepieceFill: 0,                                         // 0..~0.33 elapsed-period fill (Wave 3)
      atmospherePaused: false                                   // freeze "in amber" (Wave 3)
    };
    window.__calWorldState = worldState;
  });

  // ============================================================
  // Block: world-state-subscribers (Wave 0) — both surfaces subscribe to
  // worldState. Registered here (before the render/init blocks) so they're
  // live for every runtime setWorldState() call; the per-block initial
  // renders still run explicitly, so init behaviour is unchanged.
  //
  // Notify order = back→front render-resolution per CATALOG Part 0:
  //   1) sky-core   — time-of-day base sky + day weather/celestial layers
  //   2) sun        — celestial bodies (resolve painted-sun state + bloom)
  //   3) hourglass  — hg levels/flip (time) + sand theme/stream (day/weather)
  // (season / events / mood-tint / time-control layers are wired here as
  // change-keys now; their visuals fill in Waves 1–3.)
  // ============================================================
  function wsAffectsDay(changed) {
    return changed.indexOf('date') !== -1 || changed.indexOf('weather') !== -1 ||
           changed.indexOf('events') !== -1 || changed.indexOf('season') !== -1;
  }
  registerInitBlock('world-state-subscribers', function () {
    // 1) sky core (+ Wave 2 moon designs on a moons[] mutation).
    subscribeWorldState(function (st, changed) {
      if (changed.indexOf('timeOfDay') !== -1) renderTimePipeline(st.timeOfDay);
      if (wsAffectsDay(changed)) renderDayPipeline(st.date.month, st.date.day);
      if (changed.indexOf('moons') !== -1) applyMoonDesigns();
      if (changed.indexOf('moodTint') !== -1) applyMoodTint();
    });
    // 2) sun (celestial-bodies layer): resolve + apply painted-sun state;
    // recolour the sun-bloom emitter on a time move (matches the v5 order:
    // applySunState → refeedSky).
    subscribeWorldState(function (st, changed) {
      if (changed.indexOf('timeOfDay') !== -1 || wsAffectsDay(changed)) {
        applySunState(currentSunState());
        if (changed.indexOf('timeOfDay') !== -1) refeedSky();
      }
    });
    // 3) hourglass.
    subscribeWorldState(function (st, changed) {
      if (changed.indexOf('timeOfDay') !== -1) { applyHourglassLevels(st.timeOfDay); applyHourglassFlip(st.timeOfDay); }
      if (wsAffectsDay(changed)) { applySandTheme(st.date.month, st.date.day); feedHourglassStream(); }
    });
  });

  // ============================================================
  // Registries (dispatch §F). renderFn(container, ctx) builds the
  // particle/visual DOM for one effect on the displayed day. MUST tier
  // gets full renders; TBD entries draw a small label stub only.
  // ============================================================
  var WEATHER_EFFECTS = {};
  var CELESTIAL_EFFECTS = {};

  function spawn(cls, parent, style) {
    var s = document.createElement('span');
    s.className = cls;
    if (style) s.setAttribute('style', style);
    parent.appendChild(s);
    return s;
  }

  registerInitBlock('weather-registry', function () {
    function rain(box, heavy) {
      for (var i = 0; i < (heavy ? 30 : 28); i++) {
        var left = (i * 37 + 5) % 100;
        var delay = ((i * 53) % 100) / 100;
        var dur = 0.5 + ((i * 29) % 40) / 100;
        spawn('cal-almanac-rain' + (heavy ? ' cal-almanac-rain--heavy' : ''), box,
          'left:' + left + '%;animation-delay:' + delay + 's;animation-duration:' + dur + 's;');
      }
    }
    // V4: each MUST entry now also carries a `particleSpec` the shared
    // canvas engine reads (data, not code). The CSS-DOM renderFn stays as
    // the no-JS / server-render fallback. `null` particleSpec = no canvas
    // particles (CSS ambient only).
    WEATHER_EFFECTS.clear = { id: 'clear', tier: 'must', renderFn: function () {}, particleSpec: null };
    WEATHER_EFFECTS.cloudy = { id: 'cloudy', tier: 'must', renderFn: function (box) {
      spawn('cal-almanac-cloud cal-almanac-cloud--1', box);
      spawn('cal-almanac-cloud cal-almanac-cloud--2', box);
      spawn('cal-almanac-cloud cal-almanac-cloud--3', box);
    }, particleSpec: { shape: 'blob', color: 'oklch(0.72 0.02 250 / 0.10)', sizeRange: [40, 90], velocity: { x: [10, 24], y: [-2, 2] }, spawnRate: 0.3, maxAlive: 5, blend: 'screen', blur: 6, spawn: 'left' } };
    WEATHER_EFFECTS.rain = { id: 'rain', tier: 'must', renderFn: function (box) { rain(box, false); },
      particleSpec: { shape: 'streak', color: 'oklch(0.74 0.07 235 / 0.7)', sizeRange: [1, 2.4], velocity: { x: [-12, 8], y: [200, 280] }, spawnRate: 22, maxAlive: 30 } };
    WEATHER_EFFECTS.thunderstorm = { id: 'thunderstorm', tier: 'must', renderFn: function (box) {
      spawn('cal-almanac-cloudbank cal-almanac-cloudbank--dark', box);
      rain(box, true);
      var l = spawn('cal-almanac-lightning', box); l.setAttribute('data-cal-lightning', '');
    }, particleSpec: { shape: 'streak', color: 'oklch(0.78 0.08 240 / 0.8)', sizeRange: [1.4, 3], velocity: { x: [-22, 6], y: [260, 340] }, spawnRate: 34, maxAlive: 38 } };
    WEATHER_EFFECTS.snow = { id: 'snow', tier: 'must', renderFn: function (box) {
      for (var i = 0; i < 26; i++) {
        var left = (i * 41 + 3) % 100;
        var delay = ((i * 61) % 100) / 100;
        var dur = 3 + ((i * 23) % 30) / 10;
        var drift = ((i % 5) - 2) * 8;
        spawn('cal-almanac-snow', box, 'left:' + left + '%;animation-delay:' + delay + 's;animation-duration:' + dur + 's;--drift:' + drift + 'px;');
      }
    }, particleSpec: { shape: 'flake', color: 'oklch(0.98 0.01 240 / 0.9)', sizeRange: [1.5, 3], velocity: { x: [-14, 14], y: [30, 60] }, spawnRate: 14, maxAlive: 28 } };
    WEATHER_EFFECTS.fog = { id: 'fog', tier: 'must', renderFn: function (box) { spawn('cal-almanac-fog', box); },
      particleSpec: { shape: 'blob', color: 'oklch(0.82 0.01 245 / 0.12)', sizeRange: [60, 140], velocity: { x: [8, 20], y: [-2, 2] }, spawnRate: 0.3, maxAlive: 8, blend: 'screen', blur: 8, spawn: 'left' } };
    // TBD stubs — registry-wired; minimal tinted ambient spec (faint
    // motes in the type's colour); adding fidelity = editing the spec.
    var tbdTint = { ashfall: 'oklch(0.5 0.02 60 / 0.4)', 'acid-rain': 'oklch(0.78 0.18 125 / 0.5)', 'arcane-winds': 'oklch(0.66 0.22 310 / 0.5)', 'ley-surge': 'oklch(0.72 0.20 290 / 0.5)', 'sakura-bloom': 'oklch(0.84 0.10 350 / 0.6)' };
    ['ashfall', 'acid-rain', 'arcane-winds', 'ley-surge', 'sakura-bloom'].forEach(function (id) {
      WEATHER_EFFECTS[id] = { id: id, tier: 'tbd', renderFn: function () {},
        particleSpec: { shape: 'dot', color: tbdTint[id], sizeRange: [1.5, 3], velocity: { x: [-10, 10], y: [20, 50] }, spawnRate: 4, maxAlive: 10 } };
    });
    // GM-overhaul catalog additions: the canvas engine owns their visuals
    // (SKY_FX); these registry rows exist so the no-JS fallback renders a
    // family-appropriate DOM approximation and the sand/label lookups resolve.
    var FAMILY_FALLBACK = {
      'partly-cloudy': 'cloudy', 'overcast': 'cloudy', 'mist': 'fog',
      'drizzle': 'rain', 'heavy-rain': 'thunderstorm', 'snow-flurries': 'snow',
      'hail': 'rain', 'blizzard': 'snow', 'sandstorm': 'fog', 'tornado': 'thunderstorm',
      'ember-rain': 'ashfall', 'falling-leaves': 'sakura-bloom',
      'pollen-drift': 'sakura-bloom', 'fireflies': 'clear', 'miasma': 'fog',
    };
    Object.keys(FAMILY_FALLBACK).forEach(function (id) {
      var base = WEATHER_EFFECTS[FAMILY_FALLBACK[id]];
      WEATHER_EFFECTS[id] = { id: id, tier: 'tbd',
        renderFn: base ? base.renderFn : function () {},
        particleSpec: base ? base.particleSpec : null };
    });
    window.__calWeatherEffects = WEATHER_EFFECTS;
  });

  registerInitBlock('celestial-registry', function () {
    // meteor-shower: SLOW per operator — long trailed streaks, low spawn.
    CELESTIAL_EFFECTS['meteor-shower'] = { id: 'meteor-shower', tier: 'must', renderFn: function (box) {
      var wrap = document.createElement('div'); wrap.className = 'cal-almanac-meteors'; wrap.setAttribute('data-cal-meteors', '');
      // E6: spread the DOM-fallback meteors across the WHOLE sky. The previous
      // seed clustered them at left:55-95% / top:0-40% (the operator's "meteors
      // in the lower-right" when this CSS-DOM path renders instead of the
      // full-width canvas renderer). This fallback only paints when the canvas
      // engine is unavailable; either way it now streaks edge-to-edge.
      for (var i = 0; i < 6; i++) {
        var top = (i * 17 + 3) % 55, left = (i * 31 + 6) % 90, delay = i * 1.7;
        spawn('cal-almanac-meteor', wrap, 'top:' + top + '%;left:' + left + '%;animation-delay:' + delay + 's;');
      }
      box.appendChild(wrap);
    }, particleSpec: { shape: 'streak-long', color: 'oklch(0.95 0.06 80 / 0.9)', sizeRange: [2, 4], velocity: { x: [-150, -95], y: [60, 110] }, spawnRate: 0.6, maxAlive: 6, trail: true, blend: 'lighter' } };
    // Eclipses are SVG/CSS discs — no canvas particles.
    CELESTIAL_EFFECTS['eclipse-solar'] = { id: 'eclipse-solar', tier: 'must', renderFn: function (box) {
      var e = document.createElement('div'); e.className = 'cal-almanac-eclipse cal-almanac-eclipse--solar'; box.appendChild(e);
    }, particleSpec: null };
    CELESTIAL_EFFECTS['eclipse-lunar'] = { id: 'eclipse-lunar', tier: 'must', renderFn: function (box) {
      var e = document.createElement('div'); e.className = 'cal-almanac-eclipse cal-almanac-eclipse--lunar'; box.appendChild(e);
    }, particleSpec: null };
    var celTint = { volcanic: 'oklch(0.58 0.20 35 / 0.6)', 'ice-age': 'oklch(0.88 0.06 220 / 0.5)', plague: 'oklch(0.62 0.14 145 / 0.5)', 'arcane-surge': 'oklch(0.70 0.22 300 / 0.6)', 'moon-special': 'oklch(0.88 0.06 95 / 0.6)', aurora: 'oklch(0.78 0.16 160 / 0.5)', comet: 'oklch(0.86 0.10 210 / 0.7)' };
    ['volcanic', 'ice-age', 'plague', 'arcane-surge', 'moon-special', 'aurora', 'comet'].forEach(function (id) {
      CELESTIAL_EFFECTS[id] = { id: id, tier: 'tbd', renderFn: function (box, ctx) {
        var s = document.createElement('div'); s.className = 'cal-almanac-celestial-stub'; s.textContent = (ctx && ctx.name) || id; box.appendChild(s);
      }, particleSpec: { shape: 'dot', color: celTint[id], sizeRange: [1.5, 3.5], velocity: { x: [-8, 8], y: [10, 40] }, spawnRate: 2, maxAlive: 6 } };
    });
    // GM-overhaul catalog additions — full canvas visuals live in SKY_FX;
    // these rows give the no-JS fallback a label stub + the lookups an entry.
    ['meteor-storm', 'shooting-star', 'star-fall', 'arcane-aurora',
     'blood-moon', 'supermoon', 'harvest-moon', 'blue-moon'].forEach(function (id) {
      CELESTIAL_EFFECTS[id] = { id: id, tier: 'tbd', renderFn: function (box, ctx) {
        var s = document.createElement('div'); s.className = 'cal-almanac-celestial-stub'; s.textContent = (ctx && ctx.name) || id; box.appendChild(s);
      }, particleSpec: null };
    });
    // V5: sun-bloom — additive sparkles around the painted sun position.
    // Unlike date-triggered celestial entries, this one runs WHENEVER
    // the sun is visible (see alwaysActive). Spec is a function of the
    // current sun state so the bloom recolours / densifies appropriately.
    // No renderFn — this is a pure canvas-engine emitter; it doesn't go
    // into the DOM celestial layer.
    CELESTIAL_EFFECTS['sun-bloom'] = {
      id: 'sun-bloom',
      tier: 'must',
      alwaysActive: true,
      renderFn: function () {},
      // particleSpec is fixed (default state); engine emitter uses this.
      // State-parameterized variant goes through sunBloomSpec() below so
      // the sun state can recolor/densify the bloom live.
      particleSpec: { shape: 'dot', color: 'oklch(0.92 0.16 75 / 0.8)', sizeRange: [1.5, 3.5], velocity: { x: [-12, 12], y: [-12, 12] }, spawnRate: 1.2, maxAlive: 8, blend: 'lighter', spawn: 'sun' }
    };
    window.__calCelestialEffects = CELESTIAL_EFFECTS;
  });
  // V5: state-parameterized sun-bloom emitter spec. Cap-safe (max 14).
  function sunBloomSpec(state) {
    if (!state) state = 'default';
    var color = state === 'eclipse' ? 'oklch(0.97 0.06 85 / 0.95)'
              : state === 'special' ? 'oklch(0.72 0.22 25 / 0.9)'
              : state === 'dawn' || state === 'dusk' ? 'oklch(0.85 0.20 40 / 0.85)'
              : 'oklch(0.92 0.16 75 / 0.8)';
    var spawnRate = state === 'eclipse' ? 3 : state === 'special' ? 2.5 : 1.2;
    var maxAlive = state === 'eclipse' ? 14 : 8;
    return { shape: 'dot', color: color, sizeRange: [1.5, 3.5], velocity: { x: [-14, 14], y: [-14, 14] }, spawnRate: spawnRate, maxAlive: maxAlive, blend: 'lighter', spawn: 'sun' };
  }

  // ============================================================
  // Block: unified-effects (Wave 0) — ONE registry keyed by effect id,
  // each entry exposing per-surface renderers (skyBand / hgTop / hgBottom /
  // hgSand / timeline) per CATALOG Part 0.
  //
  // ADDITIVE by design (coordinator D1): the entries here are the SAME
  // objects as WEATHER_EFFECTS / CELESTIAL_EFFECTS, so every existing
  // call-site keeps working — the two legacy maps are now thin domain
  // projections (weather subset / celestial subset) over EFFECTS. Adding any
  // of the ~140 CATALOG effects later = one EFFECTS entry + its renderFns,
  // no refactor. Preserve that property.
  //   skyBand  ← the existing DOM/particle builder (renderFn)
  //   hgSand   ← delegates live to the sandRender hook (wired later in
  //              hookSandRenderers, after this block runs)
  //   hgTop / hgBottom / timeline ← optional hooks filled by later waves
  // ============================================================
  var EFFECTS = {};
  function projectIntoEffects(src, category) {
    Object.keys(src).forEach(function (id) {
      var e = src[id];
      if (!e) return;
      if (e.category == null) e.category = category;
      if (e.skyBand == null) e.skyBand = e.renderFn || null;
      if (!('hgTop' in e)) e.hgTop = null;
      if (!('hgBottom' in e)) e.hgBottom = null;
      if (!('timeline' in e)) e.timeline = null;
      // Live delegate so hgSand reflects the sandRender hooked later.
      if (e.hgSand == null) e.hgSand = function (box, ctx) { return e.sandRender ? e.sandRender(box, ctx) : undefined; };
      EFFECTS[id] = e;
    });
  }
  registerInitBlock('unified-effects', function () {
    projectIntoEffects(WEATHER_EFFECTS, 'weather');
    projectIntoEffects(CELESTIAL_EFFECTS, 'celestial');
    window.__calEffects = EFFECTS;
  });

  // ============================================================
  // SKY FX — layered, band-relative renderer library
  // (C-CAL-WORLDSTATE-GM-OVERHAUL — supersedes the Wave-2 ten-pack).
  // ============================================================
  // Every visual is a FACTORY returning a frame(ctx,W,H,dt,t) closure on the
  // shared engine. Three structural fixes over the Wave-2 renderers:
  //   1. BAND-RELATIVE: every speed/size/count derives from W/H, so the same
  //      effect reads correctly on the 200px production band, the taller
  //      /demo canvas, and any embed (the old absolute px/frame constants are
  //      why meteors crawled and hugged the right edge on a wide band).
  //   2. TIME-AWARE: closures read worldState.timeOfDay, so daytime cloud
  //      wisps fade out at night (the "giant glowing orbs over the night sky"
  //      bug) and the starfield breathes in with darkness.
  //   3. LAYERED + STACKABLE: each effect declares back/front layer factories.
  //      BACK paints behind the DOM sun/moons (stars, aurora, meteors, sky
  //      washes); FRONT paints over them (clouds occlude the sun, rain falls
  //      in front) onto the second canvas. Events STACK with weather (CATALOG
  //      Part 0/4: rain + meteor shower co-render) instead of replacing it.
  function clamp01(v) { return v < 0 ? 0 : (v > 1 ? 1 : v); }
  // Darkness 0 (day) → 1 (deep night), with dawn/dusk ramps; the solar
  // eclipse boosts it so stars emerge mid-day (CATALOG §11.9).
  var SKY_ENV = { darkBoost: 0 };
  function skyDarkness() {
    var t = (worldState && typeof worldState.timeOfDay === 'number') ? worldState.timeOfDay : VIEW.timeFrac;
    var d;
    if (t < 0.20) d = 1;
    else if (t < 0.32) d = 1 - (t - 0.20) / 0.12;
    else if (t < 0.68) d = 0;
    else if (t < 0.82) d = (t - 0.68) / 0.14;
    else d = 1;
    return clamp01(d + SKY_ENV.darkBoost);
  }
  function k60(dt) { return Math.min(3, dt * 60); } // legacy ≈60fps step scale
  // §5 perf: count caps scale with the engine profile (low-power drops cost).
  function fxCap(base) {
    var sc = 1; try { var p = CalParticleEngine.profile(); sc = p === 'low' ? 0.45 : (p === 'high' ? 1.15 : 1); } catch (e) {}
    return Math.max(3, Math.round(base * sc));
  }

  // ---- primitives (each returns a frame-closure factory) ----
  // Starfield underlay: density by area, alpha by darkness, gentle twinkle.
  // Always composed first on the BACK canvas — every weather/event gets the
  // same night sky instead of each renderer hand-rolling its own stars.
  function mkStarfield(opts) {
    opts = opts || {};
    return function () {
      var stars = null;
      return function (ctx, W, H, dt, t) {
        var dark = skyDarkness();
        if (dark <= 0.02) { stars = stars && stars.W === W ? stars : stars; return; }
        if (!stars || stars.W !== W || stars.H !== H) {
          var n = fxCap(Math.min(150, Math.max(45, (W * H) / 2400)));
          stars = { W: W, H: H, pts: [] };
          for (var i = 0; i < n; i++) {
            var bright = Math.random() < 0.12;
            stars.pts.push({ x: Math.random() * W, y: Math.random() * H * 0.92,
              r: bright ? 1.1 + Math.random() * 0.9 : 0.4 + Math.random() * 0.8,
              b: bright ? 1 : 0.45 + Math.random() * 0.4, ph: Math.random() * 6.28, sp: 0.6 + Math.random() * 1.2 });
          }
        }
        var tint = opts.tint || '255,255,255';
        for (var j = 0; j < stars.pts.length; j++) {
          var st = stars.pts[j];
          var tw = 0.55 + 0.45 * Math.sin(t * st.sp + st.ph);
          ctx.fillStyle = 'rgba(' + tint + ',' + (dark * st.b * tw).toFixed(3) + ')';
          ctx.beginPath(); ctx.arc(st.x, st.y, st.r, 0, 6.283); ctx.fill();
        }
      };
    };
  }
  // Full-band colour wash (storm darkening, blood-red omen, eclipse dusk...).
  // alpha may be a number or fn(darkness) for day/night-dependent strength.
  function mkWash(color, alpha) {
    return function () {
      return function (ctx, W, H) {
        var a = (typeof alpha === 'function') ? alpha(skyDarkness()) : alpha;
        if (a <= 0.003) return;
        ctx.fillStyle = color; ctx.globalAlpha = a;
        ctx.fillRect(0, 0, W, H);
        ctx.globalAlpha = 1;
      };
    };
  }
  // Drifting cloud deck: parallax puffs near the top of the band. coverage
  // 0..1 scales count; darkness 0..1 shades them stormy. Drawn on the FRONT
  // canvas so heavy decks genuinely occlude the sun (Q1: the sun element
  // keeps a floor opacity, so it stays faintly visible through the deck).
  function mkCloudDeck(cfg) {
    cfg = cfg || {};
    return function () {
      var puffs = null;
      return function (ctx, W, H, dt) {
        if (!puffs || puffs.W !== W) {
          var n = fxCap(Math.round((W / 1600) * (6 + 12 * (cfg.coverage || 0.5))));
          puffs = { W: W, list: [] };
          for (var i = 0; i < n; i++) {
            // Each cloud = 3–4 overlapping lobes (a readable cumulus shape,
            // not an amorphous radial blob).
            var r = W * (0.024 + Math.random() * 0.026), lobes = [], nl = 3 + (Math.random() < 0.5 ? 1 : 0);
            for (var li = 0; li < nl; li++) {
              lobes.push({ dx: (li - (nl - 1) / 2) * r * 0.9 + (Math.random() - 0.5) * r * 0.3,
                dy: (Math.random() - 0.4) * r * 0.35 - (li > 0 && li < nl - 1 ? r * 0.3 : 0),
                rr: 0.62 + Math.random() * 0.5 });
            }
            puffs.list.push({ x: Math.random() * W * 1.4 - W * 0.2, y: H * (0.05 + Math.random() * (cfg.band || 0.26)),
              r: r, lobes: lobes, vx: W * (0.006 + Math.random() * 0.012), squash: 0.6 + Math.random() * 0.15 });
          }
        }
        var dark = skyDarkness();
        var shade = cfg.darkness || 0;
        // Clouds read darker at night (lit only by the moon) and with storm shade.
        var topL = Math.round(250 - 165 * shade - 130 * dark * (1 - shade));
        var a = (cfg.alpha != null ? cfg.alpha : 0.55) * (1 - dark * 0.2);
        for (var j = 0; j < puffs.list.length; j++) {
          var p = puffs.list[j];
          p.x += p.vx * dt; if (p.x > W + p.r * 2.6) p.x = -p.r * 2.6;
          ctx.save(); ctx.translate(p.x, p.y); ctx.scale(1, p.squash);
          for (var k2 = 0; k2 < p.lobes.length; k2++) {
            var lb = p.lobes[k2], lr = p.r * lb.rr;
            var g = ctx.createRadialGradient(lb.dx, lb.dy - lr * 0.2, 0, lb.dx, lb.dy, lr);
            g.addColorStop(0, 'rgba(' + Math.min(255, topL + 8) + ',' + Math.min(255, topL + 8) + ',' + Math.min(255, topL + 14) + ',' + a + ')');
            g.addColorStop(0.66, 'rgba(' + topL + ',' + topL + ',' + Math.min(255, topL + 6) + ',' + (a * 0.66).toFixed(3) + ')');
            g.addColorStop(1, 'rgba(' + (topL - 22) + ',' + (topL - 20) + ',' + (topL - 10) + ',0)');
            ctx.fillStyle = g;
            ctx.beginPath(); ctx.arc(lb.dx, lb.dy, lr, 0, 6.283); ctx.fill();
          }
          ctx.restore();
        }
      };
    };
  }
  // Daytime wisps (the CLEAR sky's "alive but quiet" drift). Gated by
  // daylight — at night these fade out entirely (they were the operator's
  // "giant orb stars": white day-wisps painting over the dark sky).
  function mkWisps() {
    return function () {
      var ws = null;
      return function (ctx, W, H, dt) {
        var day = 1 - skyDarkness();
        if (day <= 0.05) return;
        if (!ws || ws.W !== W) {
          ws = { W: W, list: [] };
          for (var i = 0; i < 6; i++) ws.list.push({ x: Math.random() * W, y: H * (0.08 + Math.random() * 0.36), r: W * (0.03 + Math.random() * 0.045), vx: W * (0.006 + Math.random() * 0.006), o: 0.04 + Math.random() * 0.05 });
        }
        for (var j = 0; j < ws.list.length; j++) {
          var w = ws.list[j]; w.x += w.vx * dt; if (w.x > W + w.r) w.x = -w.r;
          var g = ctx.createRadialGradient(w.x, w.y, 0, w.x, w.y, w.r);
          g.addColorStop(0, 'rgba(255,255,255,' + (w.o * day).toFixed(3) + ')');
          g.addColorStop(1, 'rgba(255,255,255,0)');
          ctx.fillStyle = g; ctx.beginPath(); ctx.arc(w.x, w.y, w.r, 0, 6.283); ctx.fill();
        }
      };
    };
  }
  // Layered drifting haze banks (fog / mist / miasma / sandstorm body).
  function mkHaze(cfg) {
    cfg = cfg || {};
    return function () {
      var bl = null;
      return function (ctx, W, H, dt) {
        if (!bl || bl.W !== W) {
          var n = fxCap(cfg.layers || 6);
          bl = { W: W, list: [] };
          var sz = cfg.size || 1;
          for (var i = 0; i < n; i++) bl.list.push({ x: Math.random() * W * 1.5 - W * 0.25, y: H * (0.12 + Math.random() * 0.75), r: W * (0.10 + Math.random() * 0.13) * sz, vx: W * ((cfg.speed || 0.012) * (0.6 + Math.random() * 0.8)), o: (cfg.alpha || 0.2) * (0.7 + Math.random() * 0.6) });
        }
        var rgb = cfg.rgb || '200,205,215';
        for (var j = 0; j < bl.list.length; j++) {
          var b = bl.list[j]; b.x += b.vx * dt; if (b.x > W + b.r) b.x = -b.r * 2;
          var g = ctx.createRadialGradient(b.x, b.y, 0, b.x, b.y, b.r);
          g.addColorStop(0, 'rgba(' + rgb + ',' + b.o.toFixed(3) + ')');
          g.addColorStop(1, 'rgba(' + rgb + ',0)');
          ctx.fillStyle = g; ctx.beginPath(); ctx.arc(b.x, b.y, b.r, 0, 6.283); ctx.fill();
        }
      };
    };
  }
  // Precipitation: angled streaks (rain/drizzle/acid) or pellets (hail).
  // Speeds are band-relative (fraction of H per second).
  function mkPrecip(cfg) {
    cfg = cfg || {};
    return function () {
      var drops = [];
      return function (ctx, W, H, dt) {
        var cap = fxCap(W * (cfg.density || 0.22));
        var vyBase = H * (cfg.speed || 3.2), vx = W * (cfg.slant != null ? cfg.slant : -0.04);
        if (!drops.length) for (var q = 0; q < cap; q++) drops.push({ x: Math.random() * W * 1.2 - W * 0.1, y: Math.random() * H, vy: vyBase * (0.85 + Math.random() * 0.4), len: H * (cfg.len || 0.07) * (0.7 + Math.random() * 0.6) });
        var spawn = cap * dt * 1.4;
        for (var n = 0; n < spawn; n++) if (drops.length < cap) drops.push({ x: Math.random() * W * 1.2 - W * 0.1, y: -H * 0.06, vy: vyBase * (0.85 + Math.random() * 0.4), len: H * (cfg.len || 0.07) * (0.7 + Math.random() * 0.6) });
        ctx.strokeStyle = cfg.color || 'rgba(180,200,230,.55)';
        ctx.lineWidth = cfg.width || 1;
        if (cfg.pellet) ctx.fillStyle = cfg.color || 'rgba(245,250,255,.8)';
        for (var i = drops.length - 1; i >= 0; i--) {
          var d = drops[i]; d.x += vx * dt; d.y += d.vy * dt;
          if (cfg.pellet) { ctx.beginPath(); ctx.arc(d.x, d.y, cfg.width || 1.4, 0, 6.283); ctx.fill(); }
          else { ctx.beginPath(); ctx.moveTo(d.x, d.y); ctx.lineTo(d.x + vx * 0.05, d.y + d.len); ctx.stroke(); }
          if (d.y > H + 10) drops.splice(i, 1);
        }
      };
    };
  }
  // Drifting fallers/floaters: snow, ash, petals, leaves, embers, motes.
  function mkDrift(cfg) {
    cfg = cfg || {};
    return function () {
      var fl = [];
      return function (ctx, W, H, dt, t) {
        var cap = fxCap(W * (cfg.density || 0.18));
        var vyBase = H * (cfg.speed || 0.28) * (cfg.rise ? -1 : 1);
        if (!fl.length) for (var q = 0; q < cap; q++) fl.push(newP(Math.random() * H));
        var spawn = cap * dt * 0.5;
        for (var n = 0; n < spawn; n++) if (fl.length < cap) fl.push(newP(cfg.rise ? H + 6 : -6));
        function newP(y) {
          return { x: Math.random() * W, y: y, vy: vyBase * (0.7 + Math.random() * 0.7),
            r: (cfg.size || 2) * (0.6 + Math.random() * 0.9), ph: Math.random() * 6.28,
            swA: W * (cfg.sway || 0.008), rot: Math.random() * 6.28, vr: (Math.random() - 0.5) * 2,
            c: cfg.colors ? cfg.colors[Math.floor(Math.random() * cfg.colors.length)] : (cfg.color || 'rgba(255,255,255,.85)') };
        }
        for (var i = fl.length - 1; i >= 0; i--) {
          var f = fl[i];
          f.y += f.vy * dt; f.x += Math.sin(t * 1.6 + f.ph) * f.swA * dt; f.rot += f.vr * dt;
          if (cfg.glow) { ctx.shadowBlur = 6; ctx.shadowColor = f.c; }
          ctx.fillStyle = f.c;
          if (cfg.shape === 'petal' || cfg.shape === 'leaf') {
            ctx.save(); ctx.translate(f.x, f.y); ctx.rotate(f.rot);
            ctx.beginPath(); ctx.ellipse(0, 0, f.r * 1.5, f.r * 0.75, 0, 0, 6.283); ctx.fill(); ctx.restore();
          } else {
            ctx.beginPath(); ctx.arc(f.x, f.y, f.r, 0, 6.283); ctx.fill();
          }
          if (cfg.glow) ctx.shadowBlur = 0;
          if ((!cfg.rise && f.y > H + 8) || (cfg.rise && f.y < -8)) fl.splice(i, 1);
        }
      };
    };
  }
  // Fireflies: night-gated wandering glow motes near the lower band.
  function mkFireflies() {
    return function () {
      var fs = null;
      return function (ctx, W, H, dt, t) {
        var dark = skyDarkness();
        if (dark < 0.25) return;
        if (!fs) { fs = []; var n = fxCap(Math.round(W / 90)); for (var i = 0; i < n; i++) fs.push({ x: Math.random() * W, y: H * (0.45 + Math.random() * 0.5), ph: Math.random() * 6.28, sp: 0.5 + Math.random() }); }
        for (var j = 0; j < fs.length; j++) {
          var f = fs[j];
          f.x += Math.sin(t * 0.7 * f.sp + f.ph) * W * 0.012 * dt;
          f.y += Math.cos(t * 0.9 * f.sp + f.ph * 2) * H * 0.05 * dt;
          var pulse = Math.max(0, Math.sin(t * 1.8 * f.sp + f.ph));
          if (pulse < 0.1) continue;
          ctx.shadowBlur = 8; ctx.shadowColor = 'rgba(190,255,130,1)';
          ctx.fillStyle = 'rgba(205,255,140,' + (dark * pulse * 0.85).toFixed(3) + ')';
          ctx.beginPath(); ctx.arc(f.x, f.y, 1.4, 0, 6.283); ctx.fill();
          ctx.shadowBlur = 0;
        }
      };
    };
  }
  // Meteors: spawn across the WHOLE top span (not only the right edge) and
  // traverse at a band-relative diagonal, so a wide production band reads as
  // sky-wide streaks (the operator's "meteors only lower-right" fix).
  function mkMeteors(cfg) {
    cfg = cfg || {};
    return function () {
      // Prime the accumulator so the first streak fires within ~half a second
      // of the effect activating, instead of a full spawn period later.
      var ms = [], spawnAcc = 0.85;
      return function (ctx, W, H, dt) {
        spawnAcc += dt * (cfg.rate || 0.5);
        var max = fxCap(cfg.max || 6);
        if (spawnAcc >= 1 && ms.length < max) {
          spawnAcc = 0;
          // Spawn anywhere along the top + the right edge upper half, with a
          // shallow band-relative diagonal (crosses most of the width in
          // ~2.5–4s while descending roughly half the band height).
          var fromTop = Math.random() < 0.75;
          ms.push({
            x: fromTop ? Math.random() * W * 1.15 : W + W * 0.02,
            y: fromTop ? -H * 0.05 : Math.random() * H * 0.4,
            vx: -W * (cfg.speed || 0.30) * (0.8 + Math.random() * 0.5),
            vy: H * (cfg.speed || 0.30) * 0.8 * (0.55 + Math.random() * 0.5),
            life: 0,
          });
        }
        var rgb = cfg.rgb || '255,215,150';
        ctx.lineCap = 'round';
        // impact blooms (fiery meteors detonate just past the horizon line)
        if (!ms.impacts) ms.impacts = [];
        for (var bi = ms.impacts.length - 1; bi >= 0; bi--) {
          var b = ms.impacts[bi]; b.t += dt;
          var bp = b.t / 0.55;
          if (bp >= 1) { ms.impacts.splice(bi, 1); continue; }
          var br = H * (0.06 + bp * 0.3), ba = (1 - bp) * 0.5;
          var bg = ctx.createRadialGradient(b.x, b.y, 0, b.x, b.y, br);
          bg.addColorStop(0, 'rgba(255,230,180,' + (ba * 0.9).toFixed(3) + ')');
          bg.addColorStop(0.4, 'rgba(255,160,70,' + (ba * 0.55).toFixed(3) + ')');
          bg.addColorStop(1, 'rgba(255,120,40,0)');
          ctx.fillStyle = bg; ctx.beginPath(); ctx.arc(b.x, b.y, br, 0, 6.283); ctx.fill();
        }
        for (var i = ms.length - 1; i >= 0; i--) {
          var m = ms[i];
          m.x += m.vx * dt; m.y += m.vy * dt; m.life += dt;
          // Tapered luminous tail: a gradient stroke back along the velocity
          // vector — smooth at any speed (the per-frame dot trail broke into
          // a dotted line at band-relative velocities).
          var tailS = (cfg.tail || 14) / 32; // seconds of travel the tail spans
          var tx = m.x - m.vx * tailS, ty = m.y - m.vy * tailS;
          var grad = ctx.createLinearGradient(m.x, m.y, tx, ty);
          grad.addColorStop(0, 'rgba(' + rgb + ',0.85)');
          grad.addColorStop(0.35, 'rgba(' + rgb + ',0.30)');
          grad.addColorStop(1, 'rgba(' + rgb + ',0)');
          ctx.strokeStyle = grad; ctx.lineWidth = (cfg.head || 1.8) * 1.1;
          ctx.beginPath(); ctx.moveTo(m.x, m.y); ctx.lineTo(tx, ty); ctx.stroke();
          if (cfg.fiery) {
            // burning head: flickering two-tone core + ember sputter shed
            // behind along the path
            var flick = 0.85 + Math.random() * 0.3;
            ctx.shadowBlur = 16; ctx.shadowColor = 'rgba(255,140,50,0.95)';
            ctx.fillStyle = 'rgba(255,170,80,.9)';
            ctx.beginPath(); ctx.arc(m.x, m.y, (cfg.head || 3) * flick, 0, 6.283); ctx.fill();
            ctx.shadowBlur = 8; ctx.shadowColor = 'rgba(255,235,200,1)';
            ctx.fillStyle = 'rgba(255,245,225,.95)';
            ctx.beginPath(); ctx.arc(m.x, m.y, (cfg.head || 3) * 0.55 * flick, 0, 6.283); ctx.fill();
            ctx.shadowBlur = 0;
            m.sput = (m.sput || 0) + dt;
            if (m.sput > 0.05) {
              m.sput = 0;
              m.embers = m.embers || [];
              if (m.embers.length < 14) m.embers.push({ x: m.x, y: m.y, t: 0, vx: (Math.random() - 0.5) * 30, vy: (Math.random() - 0.3) * 30 });
            }
            if (m.embers) {
              for (var ei2 = m.embers.length - 1; ei2 >= 0; ei2--) {
                var em2 = m.embers[ei2]; em2.t += dt; em2.x += em2.vx * dt; em2.y += em2.vy * dt;
                var ea = 1 - em2.t / 0.6;
                if (ea <= 0) { m.embers.splice(ei2, 1); continue; }
                ctx.fillStyle = 'rgba(255,' + Math.round(120 + 90 * ea) + ',60,' + (ea * 0.8).toFixed(3) + ')';
                ctx.beginPath(); ctx.arc(em2.x, em2.y, 1 + ea, 0, 6.283); ctx.fill();
              }
            }
          } else {
            ctx.shadowBlur = 6; ctx.shadowColor = 'rgba(' + rgb + ',0.8)';
            ctx.fillStyle = 'rgba(255,250,240,.9)';
            ctx.beginPath(); ctx.arc(m.x, m.y, cfg.head || 1.2, 0, 6.283); ctx.fill();
            ctx.shadowBlur = 0;
          }
          // fiery meteors impact at the horizon (a bloom flares just past the
          // band's lower edge — the strike lands "off screen")
          if (cfg.fiery && m.y >= H * 0.96) {
            ms.impacts.push({ x: m.x + m.vx * 0.05, y: H * 1.0, t: 0 });
            ms.splice(i, 1); continue;
          }
          if (m.x < -W * 0.05 || m.y > H * 1.05) ms.splice(i, 1);
        }
      };
    };
  }
  // Aurora: hanging CURTAINS built from vertical strips on a half-res
  // offscreen canvas (per-strip alpha shimmer + a destination-in vertical
  // fade for soft top/bottom edges), composited 'screen' onto the sky.
  // Replaces the opaque hard-edged ribbons. Faint by day, full at night.
  function mkAurora(cfg) {
    cfg = cfg || {};
    var cols = cfg.cols || [
      { rgb: '80,255,150', ph: 0.0, fr: 1.4, amp: 0.10, base: 0.10, th: 0.34 },
      { rgb: '150,90,255', ph: 1.9, fr: 1.1, amp: 0.13, base: 0.20, th: 0.42 },
      { rgb: '90,205,255', ph: 3.6, fr: 1.9, amp: 0.08, base: 0.06, th: 0.26 },
    ];
    return function () {
      var off = null, octx = null;
      return function (ctx, W, H, dt, t) {
        var dark = skyDarkness();
        var strength = (0.5 + 0.5 * dark) * (cfg.boost || 1);
        if (!off || off.width !== Math.ceil(W / 2)) {
          off = (typeof document !== 'undefined' && document.createElement) ? document.createElement('canvas') : null;
          if (!off) return;
          off.width = Math.ceil(W / 2); off.height = Math.ceil(H / 2);
          octx = off.getContext('2d');
        }
        if (!octx) return;
        var w2 = off.width, h2 = off.height;
        octx.clearRect(0, 0, w2, h2);
        for (var ci = 0; ci < cols.length; ci++) {
          var c = cols[ci];
          var breathe = 0.7 + 0.3 * Math.sin(t * 0.45 + c.ph * 2);
          var aBase = 0.5 * strength * breathe;
          octx.fillStyle = 'rgb(' + c.rgb + ')';
          for (var x = 0; x < w2; x += 3) {
            var u = x / w2;
            var yT = h2 * (c.base + 0.05) + Math.sin(u * 6.283 * c.fr + t * 0.55 + c.ph) * h2 * c.amp
              + Math.sin(u * 6.283 * c.fr * 2.6 + t * 0.95 + c.ph) * h2 * c.amp * 0.35;
            var th = h2 * c.th * (0.75 + 0.35 * Math.sin(u * 6.283 * c.fr * 1.7 + t * 0.7 + c.ph * 3));
            // per-strip shimmer (the aurora's vertical "search-light" rays)
            var shim = 0.62 + 0.38 * Math.sin(u * 24 + t * 1.6 + c.ph * 5);
            octx.globalAlpha = Math.max(0, aBase * shim);
            octx.fillRect(x - 1, yT, 5, th);
          }
        }
        // Soft vertical falloff: bright in the curtain's upper body, melting
        // to nothing toward the lower sky.
        octx.globalAlpha = 1;
        octx.globalCompositeOperation = 'destination-in';
        var fade = octx.createLinearGradient(0, 0, 0, h2);
        fade.addColorStop(0, 'rgba(0,0,0,0.55)');
        fade.addColorStop(0.22, 'rgba(0,0,0,1)');
        fade.addColorStop(0.85, 'rgba(0,0,0,0)');
        octx.fillStyle = fade;
        octx.fillRect(0, 0, w2, h2);
        octx.globalCompositeOperation = 'source-over';
        ctx.save();
        ctx.globalCompositeOperation = 'screen';
        ctx.imageSmoothingEnabled = true;
        ctx.drawImage(off, 0, 0, W, H);
        ctx.restore();
      };
    };
  }
  // Comet: a single slow traverser with a long luminous tail (multi-second loop).
  function mkComet() {
    return function () {
      var phase = Math.random();
      return function (ctx, W, H, dt, t) {
        phase += dt / 75; // full crossing ≈ 75s
        var p = phase % 1.25; // pause off-screen between passes
        if (p > 1) return;
        var x = W * (1.06 - p * 1.12), y = H * (0.16 + 0.10 * Math.sin(p * 3.14));
        var tailLen = W * 0.16;
        var g = ctx.createLinearGradient(x, y, x + tailLen, y - tailLen * 0.22);
        g.addColorStop(0, 'rgba(210,240,255,0.85)');
        g.addColorStop(0.3, 'rgba(160,210,255,0.32)');
        g.addColorStop(1, 'rgba(140,190,255,0)');
        ctx.strokeStyle = g; ctx.lineWidth = 2.4; ctx.lineCap = 'round';
        ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x + tailLen, y - tailLen * 0.22); ctx.stroke();
        ctx.shadowBlur = 12; ctx.shadowColor = 'rgba(220,245,255,1)';
        ctx.fillStyle = 'rgba(235,250,255,.95)';
        ctx.beginPath(); ctx.arc(x, y, 2.6, 0, 6.283); ctx.fill();
        ctx.shadowBlur = 0;
      };
    };
  }
  // Lightning: dt-based random flashes + a jagged bolt (front layer).
  function mkLightning() {
    return function () {
      var nextIn = 2.5 + Math.random() * 4, flashT = 0, boltSeed = 0;
      return function (ctx, W, H, dt) {
        nextIn -= dt;
        if (nextIn <= 0) { flashT = 0.16; boltSeed = Math.random(); nextIn = 3 + Math.random() * 5; }
        if (flashT <= 0) return;
        flashT -= dt;
        ctx.fillStyle = 'rgba(215,228,248,' + (0.16 * (flashT / 0.16)).toFixed(3) + ')';
        ctx.fillRect(0, 0, W, H);
        if (flashT > 0.08) {
          var x = W * (0.18 + boltSeed * 0.64), y = 0, seg = H * 0.09;
          ctx.strokeStyle = 'rgba(255,250,225,.9)'; ctx.lineWidth = 2;
          ctx.shadowBlur = 10; ctx.shadowColor = 'rgba(230,240,255,1)';
          ctx.beginPath(); ctx.moveTo(x, y);
          var s2 = boltSeed * 997;
          while (y < H * 0.62) { s2 = (s2 * 16807) % 2147483647; x += ((s2 / 2147483647) - 0.5) * W * 0.04; y += seg; ctx.lineTo(x, y); }
          ctx.stroke(); ctx.shadowBlur = 0;
        }
      };
    };
  }
  // Tornado: band-relative funnel + spiralling debris (kept from Wave 2,
  // re-proportioned).
  function mkTornado() {
    return function () {
      var debris = null;
      return function (ctx, W, H, dt, t) {
        var fx = W * 0.58, topW = W * 0.006, botW = W * 0.028;
        if (!debris) { debris = []; var n = fxCap(26); for (var i = 0; i < n; i++) debris.push({ a: Math.random() * 6.28, h: Math.random() * H, r: W * (0.004 + Math.random() * 0.045), sp: 2.4 + Math.random() * 2, size: 1 + Math.random() * 2 }); }
        ctx.fillStyle = 'rgba(40,35,45,.5)';
        ctx.beginPath(); ctx.moveTo(fx - topW, 0); ctx.lineTo(fx + topW, 0); ctx.lineTo(fx + botW, H); ctx.lineTo(fx - botW, H); ctx.closePath(); ctx.fill();
        ctx.fillStyle = 'rgba(20,15,25,.4)';
        ctx.beginPath(); ctx.moveTo(fx - topW * 0.5, 0); ctx.lineTo(fx + topW * 0.5, 0); ctx.lineTo(fx + botW * 0.6, H); ctx.lineTo(fx - botW * 0.6, H); ctx.closePath(); ctx.fill();
        ctx.fillStyle = 'rgba(95,90,100,.7)';
        for (var d = 0; d < debris.length; d++) {
          var db = debris[d]; db.a += db.sp * dt;
          var yr = db.h / H, radius = db.r * (0.4 + yr * 0.6);
          ctx.beginPath(); ctx.arc(fx + Math.cos(db.a) * radius, db.h + Math.sin(db.a * 2) * 3, db.size, 0, 6.283); ctx.fill();
        }
        ctx.strokeStyle = 'rgba(140,135,145,.35)'; ctx.lineWidth = 1;
        for (var i3 = 0; i3 < 4; i3++) {
          var y = (t * H * 0.18 + i3 * H * 0.26) % H, ww = (topW + (botW - topW) * (y / H)) * 1.8;
          ctx.beginPath(); ctx.moveTo(fx - ww + Math.cos(t * 3 + i3) * 3, y); ctx.lineTo(fx + ww + Math.cos(t * 3 + i3) * 3, y); ctx.stroke();
        }
      };
    };
  }
  // Ground-up pulsing glow (ley surge).
  function mkGroundGlow(rgb) {
    return function () {
      return function (ctx, W, H, dt, t) {
        var pulse = 0.55 + 0.45 * Math.sin(t * 0.9);
        var g = ctx.createLinearGradient(0, H, 0, H * 0.35);
        g.addColorStop(0, 'rgba(' + rgb + ',' + (0.22 * pulse).toFixed(3) + ')');
        g.addColorStop(1, 'rgba(' + rgb + ',0)');
        ctx.fillStyle = g; ctx.fillRect(0, H * 0.35, W, H * 0.65);
      };
    };
  }

  // ---- the effect catalog: id → { back: [factories], front: [factories] } ----
  // BACK = behind sun/moons (sky itself); FRONT = in front (nearer weather).
  // Adding an effect = one entry of primitive configs — no engine changes.
  var SKY_FX = {};
  (function () {
    // Standard weather
    SKY_FX['clear'] = { back: [mkWisps()], front: [] };
    SKY_FX['partly-cloudy'] = { back: [], front: [mkCloudDeck({ coverage: 0.3, darkness: 0.04, alpha: 0.42 })] };
    SKY_FX['cloudy'] = { back: [], front: [mkCloudDeck({ coverage: 0.62, darkness: 0.10, alpha: 0.52 })] };
    SKY_FX['overcast'] = { back: [], front: [mkWash('rgb(120,128,142)', 0.18), mkCloudDeck({ coverage: 1, darkness: 0.24, alpha: 0.55, band: 0.5 })] };
    SKY_FX['mist'] = { back: [], front: [mkWash('rgb(190,198,210)', 0.10), mkHaze({ rgb: '208,214,224', alpha: 0.18, layers: 6, speed: 0.008, size: 1.3 })] };
    SKY_FX['fog'] = { back: [], front: [mkWash('rgb(172,180,192)', 0.30), mkHaze({ rgb: '204,210,220', alpha: 0.34, layers: 9, speed: 0.012, size: 1.5 })] };
    SKY_FX['drizzle'] = { back: [], front: [mkCloudDeck({ coverage: 0.4, darkness: 0.07, alpha: 0.4 }), mkPrecip({ density: 0.10, speed: 2.2, len: 0.045, color: 'rgba(185,205,232,.4)' })] };
    SKY_FX['rain'] = { back: [], front: [mkCloudDeck({ coverage: 0.55, darkness: 0.10, alpha: 0.48 }), mkPrecip({ density: 0.22, speed: 3.2, len: 0.07 })] };
    SKY_FX['heavy-rain'] = { back: [], front: [mkWash('rgb(48,58,74)', 0.18), mkCloudDeck({ coverage: 0.8, darkness: 0.34, alpha: 0.6 }), mkPrecip({ density: 0.4, speed: 3.9, len: 0.09, width: 1.2 })] };
    SKY_FX['snow'] = { back: [], front: [mkDrift({ density: 0.16, speed: 0.28, size: 1.8, sway: 0.010 })] };
    SKY_FX['snow-flurries'] = { back: [], front: [mkDrift({ density: 0.06, speed: 0.20, size: 1.6, sway: 0.014 })] };
    SKY_FX['hail'] = { back: [], front: [mkCloudDeck({ coverage: 0.6, darkness: 0.14, alpha: 0.5 }), mkPrecip({ density: 0.16, speed: 4.4, pellet: true, width: 1.5, color: 'rgba(240,246,255,.85)' })] };
    // Severe
    SKY_FX['thunderstorm'] = { back: [], front: [mkWash('rgb(16,19,32)', 0.52), mkCloudDeck({ coverage: 1, darkness: 0.66, alpha: 0.8, band: 0.42 }), mkPrecip({ density: 0.4, speed: 4.1, len: 0.085, width: 1.2, color: 'rgba(186,205,232,.65)' }), mkLightning()] };
    SKY_FX['blizzard'] = { back: [], front: [mkWash('rgb(214,224,238)', 0.10), mkDrift({ density: 0.45, speed: 0.9, size: 1.7, sway: 0.030 }), mkHaze({ rgb: '228,236,246', alpha: 0.12, layers: 4, speed: 0.05 })] };
    SKY_FX['sandstorm'] = { back: [], front: [mkWash('rgb(168,128,72)', 0.15), mkHaze({ rgb: '186,148,92', alpha: 0.26, layers: 7, speed: 0.07 }), mkDrift({ density: 0.3, speed: 0.5, size: 1, sway: 0.05, color: 'rgba(205,170,115,.55)' })] };
    SKY_FX['tornado'] = { back: [], front: [mkCloudDeck({ coverage: 0.85, darkness: 0.22, alpha: 0.55, band: 0.4 }), mkTornado()] };
    // Environmental
    SKY_FX['ashfall'] = { back: [mkWash('rgb(122,44,26)', 0.09)], front: [mkDrift({ density: 0.12, speed: 0.16, size: 1.3, sway: 0.006, color: 'rgba(150,140,130,.7)' })] };
    SKY_FX['ember-rain'] = { back: [mkWash('rgb(140,52,20)', 0.10)], front: [mkDrift({ density: 0.10, speed: 0.30, size: 1.4, sway: 0.012, color: 'rgba(255,150,60,.9)', glow: true })] };
    SKY_FX['sakura-bloom'] = { back: [], front: [mkDrift({ density: 0.10, speed: 0.22, size: 2.2, sway: 0.020, shape: 'petal', colors: ['rgba(255,190,210,.85)', 'rgba(250,165,195,.8)', 'rgba(255,210,225,.85)'] })] };
    SKY_FX['falling-leaves'] = { back: [], front: [mkDrift({ density: 0.07, speed: 0.24, size: 2.4, sway: 0.022, shape: 'leaf', colors: ['rgba(205,130,50,.85)', 'rgba(180,95,40,.85)', 'rgba(225,170,60,.85)'] })] };
    SKY_FX['pollen-drift'] = { back: [], front: [mkDrift({ density: 0.08, speed: 0.08, size: 1, sway: 0.024, rise: true, color: 'rgba(240,220,130,.6)', glow: true })] };
    SKY_FX['fireflies'] = { back: [mkFireflies()], front: [] };
    // Fantasy
    SKY_FX['arcane-winds'] = { back: [mkAurora({ cols: [{ rgb: '190,120,255', ph: 0, fr: 2.2, amp: 0.09, base: 0.10 }, { rgb: '120,160,255', ph: 2.4, fr: 1.7, amp: 0.12, base: 0.22 }] })], front: [mkDrift({ density: 0.06, speed: 0.10, size: 1.2, sway: 0.030, color: 'rgba(205,150,255,.7)', glow: true })] };
    SKY_FX['ley-surge'] = { back: [mkGroundGlow('150,90,255')], front: [mkDrift({ density: 0.06, speed: 0.16, size: 1.2, rise: true, sway: 0.012, color: 'rgba(185,140,255,.8)', glow: true })] };
    SKY_FX['acid-rain'] = { back: [mkWash('rgb(80,120,40)', 0.08)], front: [mkCloudDeck({ coverage: 0.5, darkness: 0.14, alpha: 0.45 }), mkPrecip({ density: 0.2, speed: 3.0, len: 0.07, color: 'rgba(160,230,90,.55)' })] };
    SKY_FX['miasma'] = { back: [mkWash('rgb(70,110,60)', 0.08)], front: [mkHaze({ rgb: '120,170,90', alpha: 0.18, layers: 6, speed: 0.012 }), mkDrift({ density: 0.05, speed: 0.08, size: 1.6, rise: true, sway: 0.012, color: 'rgba(150,210,110,.5)' })] };
    // Celestial events
    SKY_FX['meteor-shower'] = { back: [mkMeteors({ rate: 1.7, max: 12, speed: 0.34, tail: 11, head: 1.0, rgb: '230,238,255' }), mkMeteors({ rate: 0.35, max: 2, fiery: true, speed: 0.20, tail: 24, head: 3.0, rgb: '255,170,90' })], front: [] };
    SKY_FX['meteor-storm'] = { back: [mkWash('rgb(255,200,120)', 0.05), mkMeteors({ rate: 3.2, max: 18, speed: 0.42, tail: 12, head: 1.1, rgb: '235,240,255' }), mkMeteors({ rate: 0.6, max: 4, fiery: true, speed: 0.26, tail: 26, head: 3.4, rgb: '255,160,80' })], front: [] };
    SKY_FX['shooting-star'] = { back: [mkMeteors({ rate: 0.10, max: 1, tail: 16, head: 1.2, rgb: '235,240,255' })], front: [] };
    SKY_FX['star-fall'] = { back: [mkWash('rgb(255,225,160)', 0.05), mkMeteors({ rate: 2.6, max: 18, speed: 0.16, tail: 20, rgb: '255,230,170', head: 2.2 })], front: [] };
    SKY_FX['comet'] = { back: [mkComet()], front: [] };
    SKY_FX['aurora'] = { back: [mkAurora()], front: [] };
    SKY_FX['arcane-aurora'] = { back: [mkAurora({ boost: 1.5, cols: [{ rgb: '225,120,255', ph: 0, fr: 1.8, amp: 0.15, base: 0.04, th: 0.4 }, { rgb: '140,120,255', ph: 1.8, fr: 1.3, amp: 0.18, base: 0.14, th: 0.44 }, { rgb: '255,140,215', ph: 3.4, fr: 2.3, amp: 0.10, base: 0.02, th: 0.3 }] })], front: [] };
    SKY_FX['eclipse-solar'] = { back: [mkWash('rgb(10,10,26)', function () { return 0.38; })], front: [] };
    SKY_FX['eclipse-lunar'] = { back: [mkWash('rgb(120,30,30)', 0.10)], front: [] };
    SKY_FX['blood-moon'] = { back: [mkWash('rgb(140,26,32)', 0.15)], front: [] };
    SKY_FX['supermoon'] = { back: [mkWash('rgb(190,210,255)', 0.05)], front: [] };
    SKY_FX['harvest-moon'] = { back: [mkWash('rgb(220,150,60)', 0.06)], front: [] };
    SKY_FX['blue-moon'] = { back: [mkWash('rgb(110,150,255)', 0.06)], front: [] };
    SKY_FX['volcanic'] = { back: [mkWash('rgb(150,52,22)', 0.13)], front: [mkDrift({ density: 0.08, speed: 0.14, size: 1.3, sway: 0.008, color: 'rgba(140,128,120,.6)' }), mkDrift({ density: 0.04, speed: 0.22, size: 1.3, rise: true, sway: 0.012, color: 'rgba(255,140,60,.85)', glow: true })] };
    SKY_FX['plague'] = { back: [mkWash('rgb(80,120,55)', 0.10)], front: [mkHaze({ rgb: '125,175,95', alpha: 0.16, layers: 5, speed: 0.010 })] };
    SKY_FX['ice-age'] = { back: [mkWash('rgb(165,200,230)', 0.14)], front: [mkDrift({ density: 0.08, speed: 0.12, size: 1.5, sway: 0.006 }), mkHaze({ rgb: '210,228,244', alpha: 0.10, layers: 4, speed: 0.006 })] };
  })();
  window.__calSkyFx = SKY_FX;

  // Back-compat projection: the Wave-2 names still resolve to a composed
  // single-canvas factory (used by tests + the legacy weatherV2Id path).
  var WEATHER_RENDERERS = {};
  function composeLayerFactories(factories) {
    return function () {
      var frames = factories.map(function (f) { return f(); });
      return function (ctx, W, H, dt, t) {
        for (var i = 0; i < frames.length; i++) {
          try { frames[i](ctx, W, H, dt, t); } catch (e) {}
        }
      };
    };
  }
  (function () {
    var alias = { 'weather-clear': 'clear', 'weather-cloudy': 'cloudy', 'weather-rain': 'rain', 'weather-thunderstorm': 'thunderstorm', 'weather-snow': 'snow', 'weather-fog': 'fog', 'weather-tornado': 'tornado', 'weather-ashfall': 'ashfall', 'celestial-meteor-shower': 'meteor-shower', 'celestial-aurora': 'aurora' };
    Object.keys(alias).forEach(function (k) {
      var fx = SKY_FX[alias[k]];
      WEATHER_RENDERERS[k] = composeLayerFactories([].concat(fx.back, fx.front));
    });
  })();
  window.__calWeatherRenderers = WEATHER_RENDERERS;

  // SKY_FX_META — display name / category / hourglass-sand colour / glyph for
  // EVERY catalog effect (weather + celestial). The GM panel mirrors this
  // vocabulary server-side (gmWeatherTypes/gmCelestialTypes); the hourglass
  // sand recolors from it; the timeline glyph hook reads it.
  var SKY_FX_META = {
    // Standard weather
    'clear': { name: 'Clear', category: 'standard-weather', sand: 'oklch(0.86 0.16 80)', glyph: '○' },
    'partly-cloudy': { name: 'Partly Cloudy', category: 'standard-weather', sand: 'oklch(0.80 0.08 85)', glyph: '⛅' },
    'cloudy': { name: 'Cloudy', category: 'standard-weather', sand: 'oklch(0.74 0.03 250)', glyph: '☁' },
    'overcast': { name: 'Overcast', category: 'standard-weather', sand: 'oklch(0.66 0.02 250)', glyph: '☁' },
    'mist': { name: 'Mist', category: 'standard-weather', sand: 'oklch(0.82 0.01 245)', glyph: '≈' },
    'fog': { name: 'Fog', category: 'standard-weather', sand: 'oklch(0.80 0.01 245)', glyph: '≋' },
    'drizzle': { name: 'Drizzle', category: 'standard-weather', sand: 'oklch(0.78 0.07 235)', glyph: '☂' },
    'rain': { name: 'Rain', category: 'standard-weather', sand: 'oklch(0.74 0.10 235)', glyph: '☂' },
    'heavy-rain': { name: 'Heavy Rain', category: 'standard-weather', sand: 'oklch(0.66 0.12 240)', glyph: '☔' },
    'snow': { name: 'Snow', category: 'standard-weather', sand: 'oklch(0.96 0.01 240)', glyph: '❄' },
    'snow-flurries': { name: 'Snow Flurries', category: 'standard-weather', sand: 'oklch(0.93 0.01 240)', glyph: '❄' },
    'hail': { name: 'Hail', category: 'standard-weather', sand: 'oklch(0.90 0.02 240)', glyph: '◌' },
    // Severe
    'thunderstorm': { name: 'Thunderstorm', category: 'severe-weather', sand: 'oklch(0.66 0.12 250)', glyph: '⚡' },
    'blizzard': { name: 'Blizzard', category: 'severe-weather', sand: 'oklch(0.97 0.02 240)', glyph: '❅' },
    'sandstorm': { name: 'Sandstorm', category: 'severe-weather', sand: 'oklch(0.68 0.08 70)', glyph: '〰' },
    'tornado': { name: 'Tornado', category: 'severe-weather', sand: 'oklch(0.45 0.02 290)', glyph: '🌪' },
    // Environmental
    'ashfall': { name: 'Ashfall', category: 'environmental-weather', sand: 'oklch(0.50 0.04 40)', glyph: '◍' },
    'ember-rain': { name: 'Ember Rain', category: 'environmental-weather', sand: 'oklch(0.62 0.16 45)', glyph: '✺' },
    'sakura-bloom': { name: 'Sakura Bloom', category: 'environmental-weather', sand: 'oklch(0.84 0.10 350)', glyph: '❀' },
    'falling-leaves': { name: 'Falling Leaves', category: 'environmental-weather', sand: 'oklch(0.70 0.12 60)', glyph: '🍂' },
    'pollen-drift': { name: 'Pollen Drift', category: 'environmental-weather', sand: 'oklch(0.88 0.12 95)', glyph: '✿' },
    'fireflies': { name: 'Fireflies', category: 'environmental-weather', sand: 'oklch(0.88 0.16 120)', glyph: '✧' },
    // Fantasy
    'arcane-winds': { name: 'Arcane Winds', category: 'fantasy-weather', sand: 'oklch(0.66 0.22 310)', glyph: '✨' },
    'ley-surge': { name: 'Ley Surge', category: 'fantasy-weather', sand: 'oklch(0.72 0.20 290)', glyph: '⚡' },
    'acid-rain': { name: 'Acid Rain', category: 'fantasy-weather', sand: 'oklch(0.78 0.18 125)', glyph: '☣' },
    'miasma': { name: 'Miasma', category: 'fantasy-weather', sand: 'oklch(0.68 0.12 140)', glyph: '☁' },
    // Celestial events
    'meteor-shower': { name: 'Meteor Shower', category: 'celestial-event', sand: 'oklch(0.90 0.10 85)', glyph: '★' },
    'meteor-storm': { name: 'Meteor Storm', category: 'celestial-event', sand: 'oklch(0.88 0.14 80)', glyph: '☄' },
    'shooting-star': { name: 'Shooting Star', category: 'celestial-event', sand: 'oklch(0.92 0.08 85)', glyph: '☄' },
    'star-fall': { name: 'Star Fall', category: 'celestial-event', sand: 'oklch(0.92 0.12 90)', glyph: '✵' },
    'comet': { name: 'Comet', category: 'celestial-event', sand: 'oklch(0.82 0.14 200)', glyph: '☄' },
    'aurora': { name: 'Aurora', category: 'celestial-event', sand: 'oklch(0.82 0.16 160)', glyph: '✦' },
    'arcane-aurora': { name: 'Arcane Aurora', category: 'celestial-event', sand: 'oklch(0.72 0.20 310)', glyph: '✦' },
    'eclipse-solar': { name: 'Solar Eclipse', category: 'celestial-event', sand: 'oklch(0.28 0.02 270)', glyph: '◑' },
    'eclipse-lunar': { name: 'Lunar Eclipse', category: 'celestial-event', sand: 'oklch(0.45 0.12 25)', glyph: '◑' },
    'blood-moon': { name: 'Blood Moon', category: 'celestial-event', sand: 'oklch(0.50 0.19 27)', glyph: '●' },
    'supermoon': { name: 'Supermoon', category: 'celestial-event', sand: 'oklch(0.90 0.03 250)', glyph: '○' },
    'harvest-moon': { name: 'Harvest Moon', category: 'celestial-event', sand: 'oklch(0.76 0.14 60)', glyph: '○' },
    'blue-moon': { name: 'Blue Moon', category: 'celestial-event', sand: 'oklch(0.76 0.10 240)', glyph: '○' },
    'volcanic': { name: 'Volcanic Unrest', category: 'celestial-event', sand: 'oklch(0.55 0.18 35)', glyph: '▲' },
    'plague': { name: 'Plague Miasma', category: 'celestial-event', sand: 'oklch(0.62 0.14 145)', glyph: '☠' },
    'ice-age': { name: 'Deep Freeze', category: 'celestial-event', sand: 'oklch(0.88 0.06 220)', glyph: '❆' },
  };
  window.__calSkyFxMeta = SKY_FX_META;
  registerInitBlock('weather-fx', function () {
    // Per-surface EFFECTS entries for every catalog id (CATALOG Part 0 shape):
    // skyBand = composed layer factory, hgSand = sand recolor, timeline glyph.
    Object.keys(SKY_FX_META).forEach(function (id) {
      if (!SKY_FX[id]) return;
      var meta = SKY_FX_META[id];
      var composed = composeLayerFactories([].concat(SKY_FX[id].back, SKY_FX[id].front));
      var prior = EFFECTS[id] || {};
      EFFECTS[id] = {
        id: id, name: meta.name, category: meta.category, tier: prior.tier || 'must',
        renderFn: prior.renderFn || function () {},
        skyBand: composed, hgTop: prior.hgTop || null, hgBottom: prior.hgBottom || null,
        hgSand: { color: meta.sand }, timeline: meta.glyph, particleSpec: prior.particleSpec || null,
        sandRender: prior.sandRender
      };
    });
    // Legacy Wave-2 prefixed ids stay resolvable (tests + old call sites).
    Object.keys(WEATHER_RENDERERS).forEach(function (pid) {
      var plain = pid.replace(/^weather-|^celestial-/, '');
      var meta = SKY_FX_META[plain];
      if (!meta) return;
      EFFECTS[pid] = {
        id: pid, name: meta.name, category: meta.category, tier: 'must',
        skyBand: WEATHER_RENDERERS[pid], hgTop: null, hgBottom: null,
        hgSand: { color: meta.sand }, timeline: meta.glyph, particleSpec: null
      };
    });
    window.__calEffects = EFFECTS;
  });
  // Map a legacy weather effId / event type → a renderable catalog id.
  // (Kept for the hourglass sand path + back-compat; the sky band now feeds
  // through effectLayersFor, which stacks events over weather.)
  function weatherV2Id(effID, events) {
    var evs = events || [];
    for (var i = 0; i < evs.length; i++) {
      var t = String(evs[i].type || '').replace(/^celestial-/, '');
      if (SKY_FX[t] && SKY_FX_META[t] && SKY_FX_META[t].category === 'celestial-event') return t;
    }
    var plain = String(effID || '').replace(/^weather-/, '');
    if (SKY_FX[plain]) return plain;
    return null;
  }

  // ============================================================
  // Block: era-effects-registry (§A5) — each era type gets a signature
  // hover animation (particleSpec + palette + size/position) the engine
  // renders. Adding an era type = a data object, not a refactor.
  // ============================================================
  var ERA_EFFECTS = {};
  registerInitBlock('era-effects-registry', function () {
    // Each entry: { color {hue,chroma,lightness,opacity}, particleSpec,
    // size, position }. All editable via the demo-controls panel.
    ERA_EFFECTS.golden = { id: 'golden', name: 'Golden Age', color: { hue: 85, chroma: 0.14, lightness: 0.78, opacity: 0.34 },
      particleSpec: { shape: 'dot', color: 'oklch(0.88 0.13 85 / 0.8)', sizeRange: [1, 2.4], velocity: { x: [-6, 6], y: [-26, -12] }, spawnRate: 6, maxAlive: 12, blend: 'lighter' } };
    ERA_EFFECTS.dark = { id: 'dark', name: 'Age of Decline', color: { hue: 285, chroma: 0.03, lightness: 0.32, opacity: 0.42 },
      particleSpec: { shape: 'blob', color: 'oklch(0.3 0.02 285 / 0.18)', sizeRange: [30, 70], velocity: { x: [6, 16], y: [-2, 2] }, spawnRate: 0.4, maxAlive: 5, blend: 'source-over', blur: 6, spawn: 'left' } };
    ERA_EFFECTS.war = { id: 'war', name: 'Age of Conflict', color: { hue: 32, chroma: 0.16, lightness: 0.55, opacity: 0.4 },
      particleSpec: { shape: 'dot', color: 'oklch(0.72 0.19 38 / 0.85)', sizeRange: [1, 2.6], velocity: { x: [-8, 10], y: [-30, -14] }, spawnRate: 5, maxAlive: 10, blend: 'lighter' } };
    ERA_EFFECTS.mythic = { id: 'mythic', name: 'Mythic Era', color: { hue: 305, chroma: 0.18, lightness: 0.66, opacity: 0.4 },
      particleSpec: { shape: 'dot', color: 'oklch(0.82 0.2 305 / 0.85)', sizeRange: [1, 3], velocity: { x: [-12, 12], y: [-22, -8] }, spawnRate: 5, maxAlive: 10, blend: 'lighter' } };
    ERA_EFFECTS.ancient = { id: 'ancient', name: 'Forgotten Age', color: { hue: 70, chroma: 0.04, lightness: 0.6, opacity: 0.36 },
      particleSpec: { shape: 'dot', color: 'oklch(0.7 0.04 70 / 0.6)', sizeRange: [1, 2], velocity: { x: [-8, 8], y: [-10, 8] }, spawnRate: 4, maxAlive: 9 } };
    ERA_EFFECTS.neutral = { id: 'neutral', name: 'Era', color: { hue: 250, chroma: 0.04, lightness: 0.6, opacity: 0.32 },
      particleSpec: { shape: 'dot', color: 'oklch(0.78 0.04 250 / 0.6)', sizeRange: [1, 2], velocity: { x: [-6, 6], y: [-14, -4] }, spawnRate: 3, maxAlive: 7 } };
    window.__calEraEffects = ERA_EFFECTS;
  });
  // Map an era's mock id / name to an ERA_EFFECTS key. Mock eras don't
  // carry a type field yet; classify by name keyword with a neutral
  // fallback (data-driven; a real Era.type would replace this).
  function eraEffectFor(era) {
    if (!era) return ERA_EFFECTS.neutral;
    var n = (era.effect_type || era.name || '').toLowerCase();
    if (/golden|prosper|bloom|dawn/.test(n)) return ERA_EFFECTS.golden;
    if (/dark|decline|fall|shadow|silence/.test(n)) return ERA_EFFECTS.dark;
    if (/war|conflict|sundering|strife|blood/.test(n)) return ERA_EFFECTS.war;
    if (/myth|arcane|wonder|magic|weave/.test(n)) return ERA_EFFECTS.mythic;
    if (/ancient|forgotten|elder|first/.test(n)) return ERA_EFFECTS.ancient;
    return ERA_EFFECTS.neutral;
  }

  // ============================================================
  // Day-state lookups
  // ============================================================
  function dayWeatherTypeID(m, day) {
    if (!DATA || !DATA.day_weather) return null;
    return DATA.day_weather[key(DATA.current_year, m, day)] || null;
  }
  function weatherEffectID(wtypeID) {
    return ({ 'w-clear': 'clear', 'w-cloudy': 'cloudy', 'w-rain': 'rain', 'w-storm': 'thunderstorm',
      'w-blizzard': 'snow', 'w-fog': 'fog', 'w-sakura': 'sakura-bloom', 'w-ashfall': 'ashfall',
      'w-arcane': 'arcane-winds', 'w-leysurge': 'ley-surge', 'w-acidrain': 'acid-rain' })[wtypeID] || 'clear';
  }
  function weatherTypeById(id) { return (DATA.weather_types || []).find(function (w) { return w.id === id; }); }
  function celestialFor(m, day) { return (DATA.celestial_events || {})[key(DATA.current_year, m, day)] || []; }
  function celestialMeta(typeID) { return (DATA.celestial_effects || []).find(function (e) { return e.id === typeID; }); }

  // ============================================================
  // Block: sky-band-ambient — re-render the sky's weather + celestial
  // for the displayed day. (Initial day server-rendered; this drives
  // day-change swaps + keeps the snowglobe in sync.)
  // ============================================================
  // V4: when the canvas engine is live, particle effects (rain/snow/fog/
  // meteor/etc.) render on the canvas; the DOM layers keep only the
  // non-particle pieces (eclipse disc, lightning flash, cloud banks, TBD
  // glyphs). The server-rendered DOM particles are the no-JS fallback.
  var SKY_SURFACE = null; // engine handle for the BACK sky canvas (behind sun/moons)
  var SKY_FRONT = null;   // engine handle for the FRONT canvas (weather over the sun);
                          // optional — older band markup without it composites
                          // every layer onto the back canvas.
  // renderDayPipeline — the base day render (weather + celestial layers +
  // happening chips + label + canvas emitter feed). Wave 0: invoked by the
  // sky-core subscriber on a day/weather/events change; the public
  // renderSkyForDay() shim below routes callers through setWorldState.
  function renderDayPipeline(m, day) {
    var sky = document.querySelector('[data-cal-sky]');
    if (!sky) return;
    // worldState is the source of truth (Wave 0). It's kept in lockstep with
    // the authored DATA on every day-nav, so normal navigation renders
    // identically to v5; a synthetic setWorldState({weather|events}) now
    // actually paints. wtypeID (raw type, for the label) still comes from the
    // authored day; the effect id can be overridden by worldState.
    var wtypeID = dayWeatherTypeID(m, day);
    var effID = (worldState && worldState.weather && worldState.weather.type) ||
                (wtypeID ? weatherEffectID(wtypeID) : 'clear');
    sky.setAttribute('data-cal-sky-weather', effID);
    sky.className = sky.className.replace(/cal-almanac-sky--wfx-\S+/g, '').trim() + ' cal-almanac-sky--wfx-' + effID;
    var engineLive = !!SKY_SURFACE;
    var events = (worldState && worldState.events) ? worldState.events : celestialFor(m, day);
    // Weather layer. When the canvas engine is live it owns EVERY weather
    // visual (incl. the storm deck + lightning, which used to be DOM); the
    // CSS-DOM renderFns are purely the no-JS / engine-dead fallback now.
    var wlayer = sky.querySelector('[data-cal-sky-weather-layer]');
    if (wlayer) {
      wlayer.innerHTML = '';
      var w = WEATHER_EFFECTS[effID];
      if (w && !engineLive) w.renderFn(wlayer, weatherTypeById(wtypeID) || {});
    }
    // Celestial layer — same rule: canvas owns it when live (the solar
    // eclipse renders on the SUN itself via data-cal-sun-state + a sky wash,
    // not as a free-floating DOM disc).
    var clayer = sky.querySelector('[data-cal-sky-celestial-layer]');
    if (clayer) {
      clayer.innerHTML = '';
      if (!engineLive) {
        events.forEach(function (c) {
          var fx = CELESTIAL_EFFECTS[c.type];
          if (fx) fx.renderFn(clayer, c);
        });
      }
    }
    // Feed the canvas engine with this day's active particle specs.
    if (engineLive) feedSkyEngine(effID, events);
    // Happening chips bottom-right.
    var hap = sky.querySelector('[data-cal-sky-happening]');
    if (hap) {
      hap.innerHTML = '';
      events.forEach(function (c) {
        var meta = celestialMeta(c.type);
        var chip = document.createElement('span');
        chip.className = 'cal-almanac-sky__happening-chip';
        chip.title = c.name;
        chip.textContent = glyphFor(meta ? meta.icon : 'meteor');
        hap.appendChild(chip);
      });
    }
    // Label suffix (weather name + season). Prefer the authored raw type;
    // for a synthetic worldState weather, fall back to the effect-id meta so
    // the label still names it (e.g. 'Rain').
    var rest = sky.querySelector('[data-cal-sky-sub-rest]');
    if (rest) {
      if (PROD) {
        // W1 (E2): in production DATA is an empty stub, so the demo path below
        // would force a BLANK season + literal "Clear", clobbering the correct
        // server-rendered season/weather on first paint. Source both from the
        // seed worldState instead so the text matches the visuals.
        var prodSeason = (worldState && worldState.season) || '';
        var prodWx = (worldState && worldState.weather && worldState.weather.type) || 'clear';
        rest.textContent = ' · ' + skyLabel(VIEW.timeFrac) + ' · ' + prodSeason + ' · ' + titleCaseWeather(prodWx);
      } else {
        var wt = weatherTypeById(wtypeID) || (DATA.weather_effects || []).find(function (e) { return e.id === effID; });
        rest.textContent = ' · ' + skyLabel(VIEW.timeFrac) + ' · ' + seasonName() + ' · ' + (wt ? wt.name : 'Clear');
      }
    }
    // WAVE 2: repaint moon designs/phases for the displayed day + the mood wash.
    if (typeof applyMoonDesigns === 'function') applyMoonDesigns();
    if (typeof applyMoodTint === 'function') applyMoodTint();
  }
  // Public day-change entry point → unified world-state (Wave 0 shim).
  // Preserves every caller (sky-band-ambient init, weather-override,
  // month-nav "today"); the sky-core/sun/hourglass subscribers do the work.
  function renderSkyForDay(m, day) {
    var wtypeID = dayWeatherTypeID(m, day);
    var effID = wtypeID ? weatherEffectID(wtypeID) : 'clear';
    setWorldState({
      date: { year: (DATA && DATA.current_year) || (worldState && worldState.date.year) || 0, month: m, day: day },
      weather: { type: effID },
      events: celestialFor(m, day)
    });
  }
  // Tiny unicode glyph fallback for the happening chips (the SVG icon
  // set is server-side; JS-built chips use a glyph).
  function glyphFor(icon) {
    return ({ meteor: '★', eclipse: '◑', sun: '☀', moon: '☾', snowflake: '❄', ember: '◆', swirl: '✦' })[icon] || '✦';
  }
  // W1 (E2): production weather label from a seed effect-id. Dashed ids read
  // as words ("heavy-rain" → "Heavy Rain"), mirroring the Go wsWeatherLabel.
  function titleCaseWeather(id) {
    if (!id) return 'Clear';
    return id.split('-').map(function (w) { return w ? w.charAt(0).toUpperCase() + w.slice(1) : w; }).join(' ');
  }
  function skyLabel(t) {
    if (t < 0.20) return 'Pre-dawn'; if (t < 0.32) return 'Dawn'; if (t < 0.45) return 'Morning';
    if (t < 0.55) return 'Midday'; if (t < 0.70) return 'Afternoon'; if (t < 0.82) return 'Dusk'; return 'Night';
  }
  function seasonName() {
    if (!DATA || !DATA.seasons) return '';
    var best = DATA.seasons[0] ? DATA.seasons[0].name : '', bs = -1;
    (DATA.seasons || []).forEach(function (s) { if (s.start <= VIEW.month && s.start > bs) { bs = s.start; best = s.name; } });
    return best;
  }
  // Active sky emitters = weather particleSpec + celestial particleSpecs
  // + an optional era-hover spec (layered, transient). Recomputed whenever
  // weather/day/era-hover changes.
  var ERA_HOVER_SPEC = null;
  // Resolve the ACTIVE layer stack for a weather id + events list, honoring
  // the CATALOG Part-0 order: starfield (time-of-day base) → celestial events
  // (the sky's own drama) → weather (nearer atmosphere) — events STACK with
  // weather now instead of replacing it (rain + meteor shower co-render).
  // Returns { back: [factory], front: [factory] }. Exposed for tests.
  function effectLayersFor(effID, events) {
    var back = [mkStarfield()()], front = [];
    // De-dup: the same event type twice contributes once.
    var seen = {};
    (events || []).forEach(function (c) {
      var fx = SKY_FX[c.type];
      if (!fx || seen[c.type]) return;
      seen[c.type] = true;
      fx.back.forEach(function (f) { back.push(f()); });
      fx.front.forEach(function (f) { front.push(f()); });
    });
    var w = SKY_FX[effID];
    if (w) {
      w.back.forEach(function (f) { back.push(f()); });
      w.front.forEach(function (f) { front.push(f()); });
    }
    // The solar eclipse boosts darkness so the starfield emerges mid-day.
    SKY_ENV.darkBoost = seen['eclipse-solar'] ? 0.85 : 0;
    return { back: back, front: front };
  }
  window.__calEffectLayersFor = function (effID, events) {
    var L = effectLayersFor(effID, events);
    return { back: L.back.length, front: L.front.length };
  };
  function composeFrames(frames) {
    if (!frames.length) return null;
    return function (ctx, W, H, dt, t) {
      for (var i = 0; i < frames.length; i++) {
        try { frames[i](ctx, W, H, dt, t); } catch (e) {}
      }
    };
  }
  function feedSkyEngine(effID, events) {
    if (!SKY_SURFACE) return;
    var L = effectLayersFor(effID, events);
    if (SKY_FRONT) {
      SKY_SURFACE.setFrame(composeFrames(L.back));
      SKY_FRONT.setFrame(composeFrames(L.front));
    } else {
      // Single-canvas markup (legacy embeds): composite everything back-to-front.
      SKY_SURFACE.setFrame(composeFrames(L.back.concat(L.front)));
    }
    var specs = [];
    if (ERA_HOVER_SPEC) specs.push(ERA_HOVER_SPEC);
    // GM-overhaul r2 (operator): the sun-bloom particle emitter is RETIRED —
    // its drifting gold dots read as "weird sprites coming off the sun" next
    // to the CSS ray wheel + corona, which are the sun's liveliness now.
    // (sunBloomSpec stays defined for the demo-controls panel/tests.)
    SKY_SURFACE.setEmitters(specs);
    if (SKY_FRONT) SKY_FRONT.setEmitters([]);
  }
  function refeedSky() {
    // Source from worldState so a synthetic weather/event survives a later
    // time-change refeed (normal nav is identical — worldState mirrors DATA).
    var effID = (worldState && worldState.weather && worldState.weather.type) ||
                (function () { var w = dayWeatherTypeID(VIEW.month, VIEW.day); return w ? weatherEffectID(w) : 'clear'; })();
    var events = (worldState && worldState.events) ? worldState.events : celestialFor(VIEW.month, VIEW.day);
    feedSkyEngine(effID, events);
  }

  // Derive the band-height-relative render vars (sun size) on the sky root.
  // ONE sizing rule for every surface — the 200px prod band, the taller /demo
  // canvas, and any embed all get a proportionate sun, killing the per-surface
  // size-override drift (the "sun too big in prod" class).
  function setBandSizeVars() {
    var sky = document.querySelector('[data-cal-sky]');
    if (!sky || !sky.getBoundingClientRect) return;
    var h = sky.getBoundingClientRect().height || 0;
    if (h <= 0) return;
    var sun = Math.round(Math.max(64, Math.min(150, h * 0.46)));
    sky.style.setProperty('--cal-sun-size', sun + 'px');
  }
  registerInitBlock('particle-engine', function () {
    var canvas = document.querySelector('[data-cal-sky-canvas]');
    if (!canvas || !window.CalParticleEngine) return;
    SKY_SURFACE = CalParticleEngine.createSurface(canvas, {});
    window.__calSkyEngine = SKY_SURFACE;
    // FRONT canvas (weather in front of the sun) — optional markup; the feed
    // composites everything onto the back canvas when it's absent.
    var front = document.querySelector('[data-cal-sky-canvas-front]');
    if (front) {
      SKY_FRONT = CalParticleEngine.createSurface(front, {});
      window.__calSkyFrontEngine = SKY_FRONT;
    }
    setBandSizeVars();
    // Keep the canvas backing stores sized to the sky-band as it resizes.
    try {
      if ('ResizeObserver' in window) {
        var ro = trackObserver(new ResizeObserver(function () {
          SKY_SURFACE.resize();
          if (SKY_FRONT) SKY_FRONT.resize();
          setBandSizeVars();
          refeedSky();
        }));
        ro.observe(canvas);
      }
    } catch (e) {}
  });

  registerInitBlock('sky-band-ambient', function () {
    // Re-render once on init so the JS-built layers match the registries
    // (the server pre-render is for no-JS; this keeps the source single)
    // and the canvas engine gets its first emitter set. Call the base
    // pipeline directly (not the setWorldState shim) so init stays identical
    // to v5 — subscribers handle subsequent runtime changes.
    renderDayPipeline(VIEW.month, VIEW.day);
    // C-CAL-V2-SKY-RENDER-COMPLETION: also do the INITIAL TIME PAINT. The day
    // pipeline alone leaves the SUN UNPLACED and the smooth gradient unrun —
    // both live only in renderTimePipeline, which otherwise fires only on a
    // timeOfDay CHANGE (the sky-core subscriber). The demo got its first
    // time-paint from the time-slider; production has no equivalent, so on load
    // there was no sun, a coarse dusk-at-4pm SSR gradient, and night celestial
    // in daytime. Mirror the time-change subscribers' order (time pipeline →
    // sun state → refeed sky) for the first frame. This also runs on the QA2
    // re-init path: runAll() re-executes this block on htmx:afterSettle, so the
    // sun re-places after boosted nav / a binding swap, not just first load.
    renderTimePipeline(VIEW.timeFrac);
    applySunState(currentSunState());
    refeedSky();
  });

  // ============================================================
  // Block: sun-drag-scrub + gradient/clock recompute
  // ============================================================
  function gradAt(t) {
    var KEY = [
      ['oklch(0.18 0.05 270)', 'oklch(0.10 0.04 270)'], ['oklch(0.55 0.13 30)', 'oklch(0.35 0.10 305)'],
      ['oklch(0.78 0.13 220)', 'oklch(0.62 0.10 230)'], ['oklch(0.62 0.16 60)', 'oklch(0.38 0.12 350)'],
      ['oklch(0.18 0.05 270)', 'oklch(0.10 0.04 270)']];
    if (t < 0) t = 0; if (t > 1) t = 1;
    var pos = t * 4, i = Math.min(3, Math.floor(pos)), f = pos - i, a = KEY[i], b = KEY[i + 1];
    var p = ((1 - f) * 100).toFixed(1);
    return 'linear-gradient(180deg, color-mix(in oklch, ' + a[0] + ' ' + p + '%, ' + b[0] + ') 0%, color-mix(in oklch, ' + a[1] + ' ' + p + '%, ' + b[1] + ') 100%)';
  }
  function arcPos(t) {
    var wake = (t - 0.25) / 0.5;
    if (wake < -0.1 || wake > 1.1) return { left: 50, top: 120, opacity: 0 };
    return { left: 8 + wake * 84, top: 95 - 4 * wake * (1 - wake) * 85, opacity: (wake < 0 || wake > 1) ? 0.6 : 1 };
  }
  function place(el, p) { el.style.left = p.left.toFixed(1) + '%'; el.style.top = p.top.toFixed(1) + '%'; el.style.opacity = p.opacity.toFixed(2); }
  // Q1 (operator): how much each weather condition DIMS the sun. Clear + the
  // exotic tints leave it full; overcast/precipitation dim it progressively.
  // The caller floors the result (0.28) so the sun is NEVER fully hidden while
  // above the horizon — even a thunderstorm leaves it at least faintly visible.
  function sunWeatherDim(effID) {
    switch (effID) {
      case 'thunderstorm': return 0.22;
      case 'blizzard':     return 0.42;
      case 'sandstorm':    return 0.45;
      case 'tornado':      return 0.45;
      case 'fog':          return 0.25;
      case 'heavy-rain':   return 0.50;
      case 'rain':         return 0.55;
      case 'acid-rain':    return 0.55;
      case 'hail':         return 0.55;
      case 'overcast':     return 0.58;
      case 'snow':         return 0.62;
      case 'drizzle':      return 0.65;
      case 'mist':         return 0.68;
      case 'miasma':       return 0.68;
      case 'ashfall':      return 0.70;
      case 'cloudy':       return 0.72;
      case 'snow-flurries': return 0.78;
      case 'partly-cloudy': return 0.88;
      default:             return 1; // clear + exotic tints
    }
  }
  // Ray-wheel strength per weather: full spokes only on a (mostly) clear sky;
  // anything that veils the sun kills the rays before it kills the disc.
  function sunRayStrength(effID) {
    var dim = sunWeatherDim(effID);
    if (dim >= 0.99) return 0.9;
    if (dim >= 0.8) return 0.45;
    if (dim >= 0.7) return 0.2;
    return 0;
  }
  // Edge fade for arc bodies: full inside the arc, fading through the first/
  // last 14% so suns and moons melt into the horizon instead of sitting
  // half-clipped on the band edge.
  function arcEdgeFade(wake) {
    if (wake <= 0 || wake >= 1) return 0.6; // arcPos's own off-edge dim
    var edge = Math.min(wake, 1 - wake);
    return edge >= 0.14 ? 1 : 0.35 + 0.65 * (edge / 0.14);
  }

  // ============================================================
  // WAVE 2 — Moon library (CATALOG §12.1).
  // ============================================================
  // Owner picks a per-moon DESIGN + tint; phases render per the moon's
  // phaseSource. This is a GLYPH-SOURCE SWAP only — the sky-arc placement
  // (arcPos, below) and the named-phase popover are unchanged. 12 procedural
  // designs (vendored SVG, css-clip phase) + 2 emoji families (vendored Noto /
  // Twemoji, phase-index glyph swap). All static except moon-holographic's
  // CSS hue-shift, which freezes under reduced-motion (see cal-almanac.css).
  var MOON_DESIGNS = {
    'moon-watercolor': { name: 'Watercolor', category: 'stylized', phaseSource: 'css-clip' },
    'moon-holographic': { name: 'Holographic', category: 'stylized', phaseSource: 'css-clip', animated: true },
    'moon-etched': { name: 'Etched', category: 'stylized', phaseSource: 'css-clip' },
    'moon-constellation': { name: 'Constellation', category: 'stylized', phaseSource: 'css-clip' },
    'moon-realistic-selene': { name: 'Selene Classic', category: 'realistic-small', phaseSource: 'css-clip' },
    'moon-realistic-silver': { name: 'Pale Silver', category: 'realistic-small', phaseSource: 'css-clip' },
    'moon-realistic-warm': { name: 'Warm Cream', category: 'realistic-small', phaseSource: 'css-clip' },
    'moon-realistic-full': { name: 'Realistic Full', category: 'realistic-major', phaseSource: 'css-clip' },
    'moon-realistic-eclipse': { name: 'Blood Moon (Eclipse)', category: 'realistic-major', phaseSource: 'css-clip' },
    'moon-realistic-ancient': { name: 'Ancient Cratered', category: 'realistic-major', phaseSource: 'css-clip' },
    'moon-realistic-icy': { name: 'Icy (Europa)', category: 'realistic-major', phaseSource: 'css-clip' },
    'moon-realistic-volcanic': { name: 'Volcanic (Io)', category: 'realistic-major', phaseSource: 'css-clip' },
    'noto': { name: 'Noto Emoji', category: 'emoji', phaseSource: 'noto' },
    'twemoji': { name: 'Twemoji', category: 'emoji', phaseSource: 'twemoji' }
  };
  // 8-phase emoji Unicode (new → waxing → full → waning).
  var EMOJI_PHASE_CODES = ['1f311', '1f312', '1f313', '1f314', '1f315', '1f316', '1f317', '1f318'];
  var MOON_PHASE_CLASS = ['new', 'cres-wax', 'quarter-wax', 'gibb-wax', 'full', 'gibb-wane', 'quarter-wane', 'cres-wane'];
  function moonDesignSrc(id) { return '/static/vendor/cal-moons/' + id + '.svg'; }
  function emojiSrc(family, code) { return '/static/vendor/' + (family === 'twemoji' ? 'twemoji' : 'noto-emoji') + '/moons/' + code + '.svg'; }

  // Cycle position 0..1 of a moon's lunar month (separate from the intra-day
  // arc). Derived from the displayed day + the moon's orbit params so it's
  // deterministic for the mock. ~30-day synodic period.
  function moonCyclePct(moon) {
    // PRODUCTION: the server already computed the true cycle position from
    // the real calendar (BuildWorldStateSeed → moon.cyclePct), which honors
    // the calendar's actual month lengths + cycle_days. The demo's 30-day
    // synodic approximation below is only correct for the mock, so prefer
    // the seed value in prod.
    if (PROD && typeof moon.cyclePct === 'number') return moon.cyclePct;
    var dayIndex = ((VIEW.month - 1) * 30 + (VIEW.day - 1));
    var period = 30 / (moon.orbitSpeed || 1);
    var pct = ((dayIndex / period) + (moon.orbitOffset || 0)) % 1;
    if (pct < 0) pct += 1;
    return pct;
  }
  function moonPhaseIndex(pct) { return ((Math.round(pct * 8) % 8) + 8) % 8; }
  // Named-phase walk (CATALOG §2 vocab): the popover reads "The Silver Crown",
  // not a raw number. Walk the moon's namedPhases spans first; procedural
  // fallback only when none covers the day.
  function moonNamedPhase(moon, pct) {
    var p = pct * 100, spans = moon.namedPhases || [];
    for (var i = 0; i < spans.length; i++) {
      var s = spans[i], a = s.start_pct, b = s.end_pct;
      if (a <= b ? (p >= a && p < b) : (p >= a || p < b)) return s.name;
    }
    return ['New', 'Waxing Crescent', 'First Quarter', 'Waxing Gibbous', 'Full', 'Waning Gibbous', 'Last Quarter', 'Waning Crescent'][moonPhaseIndex(pct)];
  }
  function moonById(id) { return (worldState && worldState.moons || []).find(function (m) { return m.id === id; }) || null; }

  // Active celestial events can re-dress every moon (CATALOG §11.8): the
  // blood moon tints it crimson, the supermoon swells it, harvest warms it.
  // Treatment wins over the moon's authored tint while the event is active.
  var MOON_EVENT_FX = {
    'blood-moon': { tint: 'oklch(0.52 0.21 27)', scale: 1.15 },
    'eclipse-lunar': { tint: 'oklch(0.45 0.17 27)', scale: 1 },
    'supermoon': { tint: null, scale: 1.6 },
    'harvest-moon': { tint: 'oklch(0.74 0.15 55)', scale: 1.3 },
    'blue-moon': { tint: 'oklch(0.78 0.10 240)', scale: 1 },
  };
  function moonEventTreatment() {
    var evs = (worldState && worldState.events) || [];
    for (var i = 0; i < evs.length; i++) {
      if (MOON_EVENT_FX[evs[i].type]) return { id: evs[i].type, fx: MOON_EVENT_FX[evs[i].type] };
    }
    return null;
  }
  // Paint one moon element from its worldState design/phase/tint.
  function applyMoonDesign(el, moon) {
    if (!el || !moon) return;
    // §6: resolve to a KNOWN design id so the vendored src can never 404 on an
    // unknown baseDesign (falls back to the baseline realistic moon).
    var designId = MOON_DESIGNS[moon.baseDesign] ? moon.baseDesign : 'moon-realistic-selene';
    var design = MOON_DESIGNS[designId];
    var src = moon.phaseSource || design.phaseSource, pct = moonCyclePct(moon);
    moon.cyclePct = pct; moon.phase = moonPhaseIndex(pct); moon.namedPhase = moonNamedPhase(moon, pct);
    var treat = moonEventTreatment();
    el.style.setProperty('--moon-size', ((moon.size || 1) * (treat ? treat.fx.scale : 1)).toFixed(2));
    if (treat) el.setAttribute('data-cal-moon-fx', treat.id);
    else el.removeAttribute('data-cal-moon-fx');
    if (treat && treat.fx.tint) el.style.setProperty('--moon-color', treat.fx.tint);
    else if (moon.tint) el.style.setProperty('--moon-color', moon.tint);
    else el.style.removeProperty('--moon-color');
    if (src === 'noto' || src === 'twemoji') {
      // Emoji glyph already encodes the phase → no css-clip terminator.
      el.setAttribute('data-cal-moon-mode', 'emoji');
      el.style.setProperty('--moon-img', 'url(' + emojiSrc(src, EMOJI_PHASE_CODES[moon.phase]) + ')');
    } else {
      el.setAttribute('data-cal-moon-mode', 'procedural');
      el.setAttribute('data-cal-moon-design', designId);
      el.style.setProperty('--moon-img', 'url(' + moonDesignSrc(designId) + ')');
      // css-clip phase via the existing ::after terminator class.
      el.className = el.className.replace(/cal-almanac-sky__moon--\S+/g, '').trim() + ' cal-almanac-sky__moon--' + MOON_PHASE_CLASS[moon.phase];
      el.setAttribute('data-cal-moon-animated', design.animated ? 'true' : 'false');
    }
    var nm = moon.name ? moon.name + ' — ' + moon.namedPhase : moon.namedPhase;
    el.setAttribute('title', nm);
  }
  // Place a moon on its arc from worldState.timeOfDay + orbitOffset, with the
  // same horizon treatment as the time pipeline: fully hidden below the
  // horizon, edge-faded while low, never cropped above the band.
  function moonArcPlace(el, moon) {
    var t = (worldState && typeof worldState.timeOfDay === 'number') ? worldState.timeOfDay : VIEW.timeFrac;
    var mt = t - 0.5 + (moon.orbitOffset || 0); while (mt < 0) mt += 1; while (mt > 1) mt -= 1;
    var mp = arcPos(mt);
    var mwake = (mt - 0.25) / 0.5;
    if (mwake <= 0.02 || mwake >= 0.98) mp.opacity = 0;
    else mp.opacity *= arcEdgeFade(mwake);
    var sky = document.querySelector('[data-cal-sky]');
    place(el, sky ? clampArcToBand(mp, sky, 40) : mp);
  }
  // Reconcile the moon DOM with worldState.moons (the source of truth):
  // create elements for new moons, repaint design/phase/tint, place them, and
  // remove orphans. Lets the demo add / remove / randomize moons live.
  function applyMoonDesigns() {
    var sky = document.querySelector('[data-cal-sky]');
    var arc = sky && sky.querySelector('[data-cal-sky-arc]');
    if (!sky || !arc) return;
    var moons = (worldState && worldState.moons) || [], seen = {};
    moons.forEach(function (moon) {
      seen[moon.id] = true;
      var el = sky.querySelector('[data-cal-sky-moon][data-moon-id="' + moon.id + '"]');
      if (!el) {
        el = document.createElement('span');
        el.className = 'cal-almanac-sky__moon';
        el.setAttribute('data-cal-sky-moon', '');
        el.setAttribute('data-moon-id', String(moon.id));
        arc.appendChild(el);
      }
      applyMoonDesign(el, moon);
      moonArcPlace(el, moon);
    });
    sky.querySelectorAll('[data-cal-sky-moon]').forEach(function (el) {
      var id = parseInt(el.getAttribute('data-moon-id'), 10);
      if (!seen[id] && el.parentNode) el.parentNode.removeChild(el);
    });
  }
  window.__calMoonDesigns = MOON_DESIGNS;
  window.__calApplyMoonDesigns = applyMoonDesigns;
  window.__calMoonSim = { phaseIndex: moonPhaseIndex, namedPhase: moonNamedPhase, cyclePct: moonCyclePct, emojiCodes: EMOJI_PHASE_CODES, phaseClasses: MOON_PHASE_CLASS };

  // ============================================================
  // WAVE 2 — Player mood-tint wash (CATALOG Part 5; resolution step 6).
  // ============================================================
  // A global color wash tinting BOTH surfaces at once — the simplest proof of
  // the shared-registry sync. Sky-band: a mix-blend 'overlay' div above
  // weather+events (below the text label). Hourglass: a canvas 'overlay'
  // composite over both chambers + sand (in HG_INTERIOR.draw). STATIC — no
  // rAF; recomputed only on setWorldState, so reduced-motion-safe by
  // construction (it still renders, just doesn't animate). Independent of
  // weather/events; intensity 0 / no color = a fully transparent no-op.
  var MOOD_PRESETS = {
    'ominous-red': { color: 'oklch(0.55 0.22 25)', intensity: 0.45 },
    'eerie-green': { color: 'oklch(0.70 0.20 150)', intensity: 0.40 },
    'melancholy-blue': { color: 'oklch(0.55 0.16 250)', intensity: 0.42 },
    'festive-gold': { color: 'oklch(0.85 0.16 85)', intensity: 0.40 },
    'cursed-violet': { color: 'oklch(0.55 0.22 305)', intensity: 0.45 },
    'holy-white': { color: 'oklch(0.97 0.02 95)', intensity: 0.40 },
    'void-black': { color: 'oklch(0.15 0.02 280)', intensity: 0.50 },
    'frostbite-cyan': { color: 'oklch(0.85 0.12 200)', intensity: 0.40 }
  };
  // Legibility cap (stop-and-flag: mood TINTS, never erases weather/events).
  var MOOD_ALPHA_CAP = 0.8;
  function moodWashAlpha(intensity) { return Math.max(0, Math.min(MOOD_ALPHA_CAP, intensity || 0)); }
  function applyMoodTint() {
    var mood = (worldState && worldState.moodTint) || { color: null, intensity: 0 };
    var a = moodWashAlpha(mood.intensity), on = !!(mood.color && a > 0);
    var sky = document.querySelector('[data-cal-sky]');
    if (sky) {
      var wash = sky.querySelector('[data-cal-mood-wash]');
      if (!wash) {
        wash = document.createElement('div');
        wash.className = 'cal-almanac-sky__mood-wash';
        wash.setAttribute('data-cal-mood-wash', '');
        wash.setAttribute('aria-hidden', 'true');
        var label = sky.querySelector('[data-cal-sky-overlay]');
        if (label) sky.insertBefore(wash, label); else sky.appendChild(wash);
      }
      wash.style.background = on ? mood.color : 'transparent';
      wash.style.opacity = on ? a.toFixed(3) : '0';
    }
    // Hourglass interior composites the same wash on its next frame.
    if (typeof HG_INTERIOR !== 'undefined' && HG_INTERIOR.setMood) HG_INTERIOR.setMood(on ? mood.color : null, a);
  }
  window.__calApplyMoodTint = applyMoodTint;
  window.__calMoodPresets = MOOD_PRESETS;

  // ============================================================
  // WAVE 3 — Time-control verb layer (CATALOG Part 6, D&D narrative-chunk model).
  // ============================================================
  // NOT VCR playback. Atmospheric animation runs ALWAYS; timepieceFill (0..CAP)
  // is elapsed in-game time this period, capped so the piece never visually
  // runs out (ambient sand pours regardless). Verbs jump time in narrative
  // chunks (+1hr / +1day / long-rest / custom), set-time, step-back (single
  // undo + ~400ms reverse-sand), and atmosphere-pause. The ~600ms time
  // transition + reverse-sand tween on the SHARED rAF (engine.addTick) — no new
  // loop; prefers-reduced-motion → instant snaps with the correct end-state.
  var TC_FILL_CAP = 0.33;          // configurable per campaign
  var TC_HISTORY = [];             // step-back undo stack of {timeOfDay,date,fill}
  var TC_ACTIVE = null;            // current tween remover (cancel on a new verb)
  function tcHpd() { return (DATA && DATA.calendar && DATA.calendar.hours_per_day) || 24; }
  function tcPeriodHours() { return tcHpd() / 2; }
  function tcReduced() { try { return !!(window.CalParticleEngine && CalParticleEngine.reduced()); } catch (e) { return false; } }
  // Tween progress 0..1 over durMs on the shared rAF. Reduced-motion → instant.
  function tcTween(durMs, onUpdate, onDone) {
    if (TC_ACTIVE) { TC_ACTIVE(); TC_ACTIVE = null; }
    if (tcReduced() || !(window.CalParticleEngine && CalParticleEngine.addTick)) { onUpdate(1); if (onDone) onDone(); return; }
    var elapsed = 0;
    var remove = CalParticleEngine.addTick(function (dt) {
      elapsed += dt * 1000;
      var p = elapsed / durMs; if (p > 1) p = 1; if (p < 0) p = 0;
      onUpdate(p);
      if (p >= 1) { remove(); if (TC_ACTIVE === remove) TC_ACTIVE = null; if (onDone) onDone(); }
    });
    TC_ACTIVE = remove;
  }
  function tcSnapshot() {
    if (!worldState) return;
    TC_HISTORY.push({ timeOfDay: worldState.timeOfDay, date: Object.assign({}, worldState.date), fill: worldState.timepieceFill });
    if (TC_HISTORY.length > 24) TC_HISTORY.shift();
  }
  // Set fill (clamped to the cap) on both the state + the hourglass base.
  function tcSetFill(f) {
    var capped = Math.max(0, Math.min(TC_FILL_CAP, f));
    setWorldState({ timepieceFill: capped });
    if (HG_INTERIOR.setFill) HG_INTERIOR.setFill(capped / TC_FILL_CAP);
  }
  // Period boundary (fill hit the cap / "end day"): reuse the dawn/dusk flip,
  // swap chambers, reset fill to 0 for the fresh period.
  function tcPeriodBoundary() {
    if (typeof forceHourglassFlip === 'function') forceHourglassFlip();
    tcSetFill(0);
  }
  function tcDayWeather(m, day) { var w = dayWeatherTypeID(m, day); return w ? weatherEffectID(w) : 'clear'; }
  // Move the calendar cursor by n days (30-day months); repaint to that day's
  // weather/moons/events via the day pipeline.
  function tcAdvanceDateBy(n) {
    if (!worldState) return;
    var d = Object.assign({}, worldState.date), months = ((DATA.months || []).length) || 12;
    d.day += n;
    while (d.day > 30) { d.day -= 30; d.month += 1; if (d.month > months) { d.month = 1; d.year += 1; } }
    while (d.day < 1) { d.day += 30; d.month -= 1; if (d.month < 1) { d.month = months; d.year -= 1; } }
    setWorldState({ date: d, weather: { type: tcDayWeather(d.month, d.day) }, events: celestialFor(d.month, d.day) });
  }
  // +N hours: smooth ~600ms time transition (date rolls at midnight), then bump
  // fill (cap → period boundary).
  function tcAdvanceHours(hours) {
    if (!worldState) return;
    tcSnapshot();
    var hpd = tcHpd(), from = worldState.timeOfDay, deltaFrac = hours / hpd;
    var rawEnd = from + deltaFrac, dayInc = Math.floor(rawEnd), end = rawEnd - dayInc;
    tcTween(600, function (p) {
      var cur = from + deltaFrac * p, frac = cur - Math.floor(cur);
      setWorldState({ timeOfDay: Math.max(0, Math.min(0.9999, frac)) });
    }, function () {
      if (dayInc > 0) tcAdvanceDateBy(dayInc);
      setWorldState({ timeOfDay: Math.max(0, Math.min(0.9999, end)) });
      var nf = worldState.timepieceFill + (hours / tcPeriodHours()) * TC_FILL_CAP;
      if (nf >= TC_FILL_CAP) tcPeriodBoundary(); else tcSetFill(nf);
    });
  }
  // Period fraction (0..1) of a time-of-day within its current half-day.
  function tcPeriodFrac(t) {
    var rise = (DATA && DATA.sunrise) || 0.25, set = (DATA && DATA.sunset) || 0.75;
    if (t >= rise && t < set) return (t - rise) / (set - rise);
    var nl = (rise - set + 1) % 1 || 0.5, e = (t - set + 1) % 1; return Math.max(0, Math.min(1, e / nl));
  }
  // Set-time: snap to a time-of-day (brief crossfade) + snap fill to its period
  // fraction. The renderTimePipeline crossfades the sky/gradient.
  function tcSetTime(t) {
    tcSnapshot();
    setWorldState({ timeOfDay: Math.max(0, Math.min(0.9999, t)) });
    tcSetFill(tcPeriodFrac(t) * TC_FILL_CAP);
  }
  // Step-back: single undo of the last verb + the ~400ms reverse-sand flourish.
  function tcStepBack() {
    if (!TC_HISTORY.length) return false;
    var prev = TC_HISTORY.pop();
    if (HG_INTERIOR.reverseSand) HG_INTERIOR.reverseSand();
    var from = worldState.timeOfDay;
    tcTween(400, function (p) {
      var cur = from + (prev.timeOfDay - from) * p;
      setWorldState({ timeOfDay: Math.max(0, Math.min(0.9999, cur)) });
    }, function () {
      setWorldState({ date: Object.assign({}, prev.date), timeOfDay: Math.max(0, Math.min(0.9999, prev.timeOfDay)),
        weather: { type: tcDayWeather(prev.date.month, prev.date.day) }, events: celestialFor(prev.date.month, prev.date.day) });
      tcSetFill(prev.fill);
    });
    return true;
  }
  // Atmosphere-pause: freeze everything ("suspended in amber"). Stops the shared
  // rAF (engine holds the last frame) + pauses CSS animations via a shell class.
  function tcSetPaused(paused) {
    setWorldState({ atmospherePaused: !!paused });
    try { if (window.CalParticleEngine && CalParticleEngine.setPaused) CalParticleEngine.setPaused(!!paused); } catch (e) {}
    // Freeze the CSS layers too. The attribute lands on the demo shell AND
    // the production render roots (sky band + shelf) — the paused rule was
    // shell-scoped before, so the GM pause froze /demo but not production.
    document.querySelectorAll('.cal-almanac-shell, [data-cal-sky], [data-cal-time]').forEach(function (el) {
      if (paused) el.setAttribute('data-cal-atmosphere-paused', 'true');
      else el.removeAttribute('data-cal-atmosphere-paused');
    });
  }
  function tcTogglePause() { tcSetPaused(!(worldState && worldState.atmospherePaused)); }
  // Public verb API (the future GM Live Control Panel reuses these).
  var TIME_CONTROL = {
    advanceHours: tcAdvanceHours, advanceDays: tcAdvanceDateBy, longRest: function () { tcAdvanceHours(8); },
    setTime: tcSetTime, stepBack: tcStepBack, togglePause: tcTogglePause, setPaused: tcSetPaused,
    setFill: tcSetFill, fillCap: TC_FILL_CAP, history: TC_HISTORY
  };
  window.__calTimeControl = TIME_CONTROL;

  function clockStr(t) {
    var hpd = (DATA && DATA.calendar && DATA.calendar.hours_per_day) || 24;
    var total = Math.floor(t * hpd * 60); return pad2(Math.floor(total / 60)) + ':' + pad2(total % 60);
  }
  // renderTimePipeline — the base time render (sky gradient, sun arc, moons,
  // clocks, snowglobe). Wave 0: invoked by the sky-core subscriber on a
  // timeOfDay change; the public applyTime() shim below routes callers
  // through setWorldState.
  // Clamp an arc position so a body of `sizePx` never crops ABOVE the band
  // (the raw arc put the midday sun half-cut off the 200px prod band's top).
  // The bottom is deliberately unclamped — a low sun/moon sinking below the
  // band edge reads as setting behind the horizon.
  function clampArcToBand(p, sky, sizePx) {
    var h = (sky.getBoundingClientRect && sky.getBoundingClientRect().height) || 0;
    if (h <= 0 || p.opacity <= 0) return p;
    var minTop = ((sizePx / 2) / h) * 100 + 3;
    return { left: p.left, top: p.top < minTop ? minTop : p.top, opacity: p.opacity };
  }
  function sunSizePx(sky) {
    var v = '';
    try { v = (sky.style && sky.style.getPropertyValue) ? sky.style.getPropertyValue('--cal-sun-size') : ''; } catch (e) {}
    var n = parseFloat(v);
    return isFinite(n) && n > 0 ? n : 96;
  }
  function renderTimePipeline(t) {
    t = Math.max(0, Math.min(0.9999, t));
    VIEW.timeFrac = t;
    var sky = document.querySelector('[data-cal-sky]');
    if (sky) {
      sky.style.background = gradAt(t);
      var sun = sky.querySelector('[data-cal-sky-sun]');
      if (sun) {
        var ap = arcPos(t);
        var wake = (t - 0.25) / 0.5;
        place(sun, clampArcToBand(ap, sky, sunSizePx(sky)));
        // Q1: weather dims but never fully hides the sun. arcPos already fades
        // it at the horizon (opacity 0 = below); only apply the weather factor
        // (with a daytime floor) while it is actually up. Rays die first.
        var sunEff = (worldState && worldState.weather && worldState.weather.type) || 'clear';
        if (ap.opacity > 0) {
          // Q1 floor is weather-aware: ordinary cover keeps the sun clearly
          // visible (0.28); a heavy storm/fog may swallow it to a faint ember
          // (0.10) — dimmed, never fully gone.
          var dim = sunWeatherDim(sunEff);
          var floor = dim <= 0.3 ? 0.10 : 0.28;
          sun.style.opacity = Math.max(ap.opacity * dim * arcEdgeFade(wake), floor).toFixed(2);
        }
        sun.style.setProperty('--cal-sun-rays', sunRayStrength(sunEff).toFixed(2));
      }
      sky.querySelectorAll('[data-cal-sky-moon]').forEach(function (mn) {
        var id = parseInt(mn.getAttribute('data-moon-id'), 10), off = 0;
        var wm = moonById(id);                                  // WAVE 2: orbit from worldState (incl. demo-added moons)
        if (wm) off = wm.orbitOffset || 0;
        else (DATA.moons || []).forEach(function (mm) { if (mm.id === id) off = mm.phase_offset; });
        var mt = t - 0.5 + off; while (mt < 0) mt += 1; while (mt > 1) mt -= 1;
        var mp = arcPos(mt);
        var mwake = (mt - 0.25) / 0.5;
        // Moons hide entirely below the horizon (no half-sunk disc parking on
        // the band edge) and melt in/out through the edge fade while low.
        if (mwake <= 0.02 || mwake >= 0.98) mp.opacity = 0;
        else mp.opacity *= arcEdgeFade(mwake);
        place(mn, clampArcToBand(mp, sky, 40));
      });
      var rest = sky.querySelector('[data-cal-sky-sub-rest]');
      if (rest) { var parts = rest.textContent.split(' · '); parts[1] = skyLabel(t); rest.textContent = parts.join(' · '); }
    }
    // Sync clocks (sky time label + hourglass clock).
    document.querySelectorAll('[data-cal-sky-time-label], [data-cal-time-clock]').forEach(function (c) { c.textContent = clockStr(t); });
    if (window.__calSyncTimeAria) window.__calSyncTimeAria();   // R1: keep the time-slider a11y in sync
  }
  // Public time-change entry point → unified world-state (Wave 0 shim).
  // Preserves every caller (drag-scrub, time-input, hourglass tick,
  // demo-controls slider); the subscribers re-render both surfaces.
  function applyTime(t) {
    setWorldState({ timeOfDay: Math.max(0, Math.min(0.9999, t)) });
  }
  // R1: the SUN is passive now (decorative, aria-hidden) — it just tracks
  // worldState.timeOfDay (placement + recolor + bloom). No drag wiring.

  function hpdOf() { return (DATA && DATA.calendar && DATA.calendar.hours_per_day) || 24; }
  function monthLen(m) { var mo = (DATA && DATA.months || [])[m - 1]; return (mo && mo.days) || 30; }

  // ============================================================
  // R1 — time control = the TIME readout (drag / arrow-keys / type). The slider
  // a11y the hardening deferred from the sun lives here.
  // ============================================================
  registerInitBlock('time-control', function () {
    var label = document.querySelector('[data-cal-sky-time-label]');
    if (!label) return;
    label.setAttribute('role', 'slider');
    label.setAttribute('aria-label', 'Time of day — drag horizontally or use arrow keys; click to type');
    label.setAttribute('aria-valuemin', '0');
    label.setAttribute('tabindex', '0');
    function syncAria() {
      var perDay = hpdOf() * 60, total = Math.floor(VIEW.timeFrac * perDay);
      label.setAttribute('aria-valuemax', String(perDay - 1));
      label.setAttribute('aria-valuenow', String(total));
      label.setAttribute('aria-valuetext', clockStr(VIEW.timeFrac));
    }
    window.__calSyncTimeAria = syncAria;
    syncAria();
    function openTimeInput() {
      var input = document.createElement('input');
      input.className = 'cal-almanac-sky__time-input';
      input.value = label.textContent;
      label.replaceWith(input);
      input.focus(); input.select();
      function commit(save) {
        if (save) { var t = parseTime(input.value, hpdOf()); if (t != null) applyTime(t); }
        input.replaceWith(label); label.textContent = clockStr(VIEW.timeFrac); syncAria();
        try { label.focus(); } catch (e) {}
      }
      input.addEventListener('keydown', function (ev) { if (ev.key === 'Enter') commit(true); if (ev.key === 'Escape') commit(false); });
      input.addEventListener('blur', function () { commit(true); });
    }
    // Drag-to-scrub (a full day over ~600px); a click without movement opens
    // the type-to-set input.
    var down = false, moved = false, sx = 0, startT = 0;
    label.addEventListener('pointerdown', function (ev) { down = true; moved = false; sx = ev.clientX; startT = VIEW.timeFrac; try { label.setPointerCapture(ev.pointerId); } catch (e) {} ev.stopPropagation(); });
    label.addEventListener('pointermove', function (ev) {
      if (!down) return; var dx = ev.clientX - sx; if (Math.abs(dx) > 3) moved = true;
      if (moved) { var t = ((startT + dx / 600) % 1 + 1) % 1; applyTime(t); syncAria(); ev.preventDefault(); }
    });
    function up(ev) { if (!down) return; down = false; try { label.releasePointerCapture(ev.pointerId); } catch (e) {} if (!moved) openTimeInput(); }
    label.addEventListener('pointerup', up); label.addEventListener('pointercancel', up);
    // Keyboard slider a11y: ←/→ step a minute, PageUp/Down an hour, Home/End day bounds.
    label.addEventListener('keydown', function (ev) {
      var step = 1 / (hpdOf() * 60), big = 1 / hpdOf(), t = VIEW.timeFrac, handled = true;
      switch (ev.key) {
        case 'ArrowLeft': case 'ArrowDown': t -= step; break;
        case 'ArrowRight': case 'ArrowUp': t += step; break;
        case 'PageDown': t -= big; break;
        case 'PageUp': t += big; break;
        case 'Home': t = 0; break;
        case 'End': t = 0.9999; break;
        case 'Enter': case ' ': ev.preventDefault(); openTimeInput(); return;
        default: handled = false;
      }
      if (handled) { ev.preventDefault(); t = ((t % 1) + 1) % 1; applyTime(Math.min(0.9999, Math.max(0, t))); syncAria(); }
    });
  });

  // ============================================================
  // R2 — date setter: click the date readout → day / named-month / year + Go.
  // Commits setWorldState({date}); both surfaces + the grid repaint.
  // ============================================================
  registerInitBlock('date-setter', function () {
    var trigger = document.querySelector('[data-cal-sky-date]');
    var pop = document.querySelector('[data-cal-datesetter]');
    if (!trigger || !pop) return;
    var dayI = pop.querySelector('[data-cal-datesetter-day]');
    var monI = pop.querySelector('[data-cal-datesetter-month]');
    var yrI = pop.querySelector('[data-cal-datesetter-year]');
    function syncDayMax() { if (dayI && monI) dayI.max = String(monthLen(parseInt(monI.value, 10) || 1)); }
    function open() {
      var d = (worldState && worldState.date) || { year: VIEW.year, month: VIEW.month, day: VIEW.day };
      if (yrI) yrI.value = d.year; if (monI) monI.value = String(d.month);
      syncDayMax(); if (dayI) dayI.value = d.day;
      pop.setAttribute('data-cal-datesetter-open', 'true'); pop.setAttribute('aria-hidden', 'false');
      openDialog(pop, closePop);
    }
    function closePop() { pop.setAttribute('data-cal-datesetter-open', 'false'); pop.setAttribute('aria-hidden', 'true'); closeDialog(pop); }
    function commit() {
      var months = ((DATA.months || []).length) || 12;
      var y = parseInt(yrI.value, 10), mo = parseInt(monI.value, 10), day = parseInt(dayI.value, 10);
      if (isNaN(y) || isNaN(mo) || isNaN(day)) return;
      mo = Math.max(1, Math.min(months, mo)); day = Math.max(1, Math.min(monthLen(mo), day));
      setWorldState({ date: { year: y, month: mo, day: day }, weather: { type: tcDayWeather(mo, day) }, events: celestialFor(mo, day) });
      closePop();
    }
    trigger.addEventListener('click', open);
    if (monI) monI.addEventListener('change', syncDayMax);
    var go = pop.querySelector('[data-cal-datesetter-go]'); if (go) go.addEventListener('click', commit);
    var cancel = pop.querySelector('[data-cal-datesetter-cancel]'); if (cancel) cancel.addEventListener('click', closePop);
  });
  function parseTime(str, hpd) {
    str = String(str).trim();
    var ampm = /am|pm/i.test(str), pm = /pm/i.test(str);
    var m = str.match(/(\d{1,2})\s*:\s*(\d{1,2})/);
    if (!m) return null;
    var h = parseInt(m[1], 10), min = parseInt(m[2], 10);
    if (ampm) { if (h === 12) h = 0; if (pm) h += 12; }
    if (h < 0 || h >= hpd || min < 0 || min >= 60) return null;
    return (h * 60 + min) / (hpd * 60);
  }
  window.__calParseTime = parseTime;

  // ============================================================
  // Block: era-overlay (§A) — responsive sizing + OKLCH colour from the
  // ERA_EFFECTS registry + badge-click → era detail + badge-hover →
  // signature particle animation via the shared engine.
  // ============================================================
  function currentEraObj() {
    var vig = document.querySelector('[data-cal-era-vignette]');
    if (!vig || !DATA) return null;
    return (DATA.eras || []).find(function (e) { return e.id === vig.getAttribute('data-cal-era-id'); }) || null;
  }
  // Apply an ERA_EFFECTS palette + size/position to the era element via
  // CSS custom properties. Size is responsive: a fraction of the sky-band
  // width, clamped to <=120px (base) and the CSS 25% hard cap.
  function applyEraParams(el, eff, sky) {
    if (!el || !eff) return;
    var c = eff.color || {};
    if (c.hue != null) el.style.setProperty('--cal-era-hue', c.hue);
    if (c.chroma != null) el.style.setProperty('--cal-era-chroma', c.chroma);
    if (c.lightness != null) el.style.setProperty('--cal-era-lightness', c.lightness);
    if (c.opacity != null) el.style.setProperty('--cal-era-opacity', c.opacity);
    // Responsive size: ~22% of the sky-band width, capped 64..120px.
    if (sky) {
      var w = sky.getBoundingClientRect().width || 1080;
      var size = Math.max(64, Math.min(120, Math.round(w * 0.115)));
      el.style.setProperty('--cal-era-size', size + 'px');
    }
    el.setAttribute('data-cal-era-effect', eff.id);
  }
  registerInitBlock('era-overlay', function () {
    var vig = document.querySelector('[data-cal-era-vignette]');
    var badge = document.querySelector('[data-cal-era-badge]');
    var sky = document.querySelector('[data-cal-sky]');
    if (!vig || !badge) return;
    var era = currentEraObj();
    var eff = eraEffectFor(era);
    applyEraParams(vig, eff, sky);
    // Responsive re-size as the widget changes dimensions.
    try {
      if ('ResizeObserver' in window && sky) {
        var ro = trackObserver(new ResizeObserver(function () { applyEraParams(vig, eraEffectFor(currentEraObj()), sky); }));
        ro.observe(sky);
      }
    } catch (e) {}
    // Badge click → era detail panel.
    badge.addEventListener('click', function (ev) {
      ev.stopPropagation();
      var e = currentEraObj();
      if (!e) return;
      openSkyPanel('Era · ' + e.name, [
        '<div class="cal-almanac-skypanel__row"><b>' + esc(e.name) + '</b></div>',
        '<div class="cal-almanac-skypanel__row">' + esc(eraSpan(e)) + '</div>',
        e.description ? '<div class="cal-almanac-skypanel__row">' + esc(e.description) + '</div>' : ''
      ].join(''));
    });
    // Badge hover → layer the era's signature particles into the sky.
    badge.addEventListener('mouseenter', function () {
      var ef = eraEffectFor(currentEraObj());
      ERA_HOVER_SPEC = (ef && ef.particleSpec) || null;
      refeedSky();
    });
    badge.addEventListener('mouseleave', function () { ERA_HOVER_SPEC = null; refeedSky(); });
  });
  function eraSpan(e) { return e.end_year ? (e.start_year + ' – ' + e.end_year) : (e.start_year + ' – ongoing'); }
  // Re-expose for the demo-controls panel: lets the operator drive era
  // params live (§E3).
  function setEraParam(name, value) {
    var vig = document.querySelector('[data-cal-era-vignette]');
    if (vig) vig.style.setProperty('--cal-era-' + name, value);
  }
  window.__calSetEraParam = setEraParam;

  // ============================================================
  // REFINEMENT-V5 — Painted sun state machine
  // ============================================================
  // resolveSunState — pure function: given (timeFrac, activeCelestial id,
  // isSpecialMoonDay) returns the right state ID. Eclipse > special >
  // dawn/dusk window > default. The thresholds are 0.20-0.32 (dawn) and
  // 0.68-0.80 (dusk) — wider than the visual horizon to give the painted
  // dawn/dusk asset breathing room around sunrise/sunset.
  function resolveSunState(timeFrac, activeCelestial, isSpecialMoonDay) {
    if (activeCelestial === 'eclipse-solar') return 'eclipse';
    if (isSpecialMoonDay) return 'special';
    if (timeFrac > 0.20 && timeFrac < 0.32) return 'dawn';
    if (timeFrac > 0.68 && timeFrac < 0.80) return 'dusk';
    return 'default';
  }
  // Helper: is the current day flagged as a special-moon-day? Read from
  // the mock SpecialMoonDays array (added in v5 if not present).
  function isSpecialMoonDayFor(m, day) {
    if (!DATA || !DATA.special_moon_days) return false;
    var k = key(DATA.current_year, m, day);
    return DATA.special_moon_days.indexOf(k) !== -1;
  }
  // Helper: the active celestial id for the current day, or null.
  function activeCelestialId(events) {
    if (!events || !events.length) return null;
    return events[0].type;
  }
  function currentSunState(events) {
    var evs = events || (worldState && worldState.events) || celestialFor(VIEW.month, VIEW.day);
    return resolveSunState(VIEW.timeFrac, activeCelestialId(evs), isSpecialMoonDayFor(VIEW.month, VIEW.day));
  }
  // Apply the resolved state to the sun element. Crossfading + CSS pulse
  // is driven by the matching layer's CSS rule.
  function applySunState(state) {
    var sun = document.querySelector('[data-cal-sky-sun]');
    if (!sun) return;
    state = state || 'default';
    if (sun.getAttribute('data-cal-sun-state') !== state) {
      sun.setAttribute('data-cal-sun-state', state);
    }
  }
  registerInitBlock('sun-state', function () {
    applySunState(currentSunState());
    // Wave 0: the painted-sun state is re-resolved by the sun subscriber
    // (registered in 'world-state-subscribers') on every timeOfDay/day
    // change — the v5 applyTime/renderSkyForDay monkey-patching is gone.
  });
  window.__calResolveSunState = resolveSunState;

  // ============================================================
  // REFINEMENT-V3 — Hourglass time-piece
  // ============================================================
  // The hourglass replaces the snowglobe. Composition:
  //   - hourglass-render   : drag + tick + chamber-level update on tick
  //   - hourglass-flip     : sunrise/sunset cadence wiring
  //   - hourglass-themed-sand : applies the per-day weather/celestial
  //                             sand theme via WEATHER/CELESTIAL_EFFECTS
  //                             registries' sandRender hook.
  // ============================================================

  // sandRender(box, ctx) — re-paints the stream box's grains. Pure
  // particle emission; chamber colours come from the data-theme
  // attribute (CSS-driven, see cal-almanac.css). Each registry entry
  // may override the default to inject themed grains.
  function defaultSandRender(box) {
    if (!box) return;
    box.innerHTML = '';
    var n = 15; // base count — kept low for perf (see stop-and-flag #2)
    for (var i = 0; i < n; i++) {
      var g = document.createElement('span');
      g.className = 'cal-almanac-hourglass__grain';
      g.style.setProperty('--dur', (0.7 + (i % 5) * 0.12).toFixed(2) + 's');
      g.style.setProperty('--delay', ((i * 73) % 100 / 100).toFixed(2) + 's');
      box.appendChild(g);
    }
  }
  function streakSandRender(box) {
    defaultSandRender(box);
    if (!box) return;
    for (var i = 0; i < 4; i++) {
      var s = document.createElement('span');
      s.className = 'cal-almanac-hourglass__grain cal-almanac-hourglass__grain--streak';
      s.style.setProperty('--dur', '0.55s');
      s.style.setProperty('--delay', (i * 0.22).toFixed(2) + 's');
      box.appendChild(s);
    }
  }
  function bigDropSandRender(box) {
    if (!box) return;
    box.innerHTML = '';
    for (var i = 0; i < 12; i++) {
      var g = document.createElement('span');
      g.className = 'cal-almanac-hourglass__grain cal-almanac-hourglass__grain--big';
      g.style.setProperty('--dur', (0.9 + (i % 4) * 0.18).toFixed(2) + 's');
      g.style.setProperty('--delay', ((i * 83) % 100 / 100).toFixed(2) + 's');
      box.appendChild(g);
    }
  }

  function hookSandRenderers() {
    // MUST-tier weather
    if (WEATHER_EFFECTS.clear)        WEATHER_EFFECTS.clear.sandRender = defaultSandRender;
    if (WEATHER_EFFECTS.cloudy)       WEATHER_EFFECTS.cloudy.sandRender = defaultSandRender;
    if (WEATHER_EFFECTS.rain)         WEATHER_EFFECTS.rain.sandRender = bigDropSandRender;
    if (WEATHER_EFFECTS.thunderstorm) WEATHER_EFFECTS.thunderstorm.sandRender = function (box) {
      bigDropSandRender(box);
      // Brief flash element — sky-band lightning syncs the timing via CSS.
      var flash = document.createElement('span');
      flash.className = 'cal-almanac-hourglass__grain cal-almanac-hourglass__grain--streak';
      flash.style.setProperty('--dur', '1.2s');
      flash.style.setProperty('--delay', '0.4s');
      box && box.appendChild(flash);
    };
    if (WEATHER_EFFECTS.snow) WEATHER_EFFECTS.snow.sandRender = defaultSandRender;
    if (WEATHER_EFFECTS.fog)  WEATHER_EFFECTS.fog.sandRender = function (box) {
      defaultSandRender(box);
      // Fog grains are softened by CSS (already gray-tinted via --sand-color).
    };
    // TBD weather stubs — single default-shaped renderer; theme-coloured by CSS.
    ['ashfall', 'acid-rain', 'arcane-winds', 'ley-surge', 'sakura-bloom'].forEach(function (id) {
      if (WEATHER_EFFECTS[id]) WEATHER_EFFECTS[id].sandRender = defaultSandRender;
    });

    // MUST-tier celestial
    if (CELESTIAL_EFFECTS['meteor-shower']) CELESTIAL_EFFECTS['meteor-shower'].sandRender = streakSandRender;
    if (CELESTIAL_EFFECTS['eclipse-solar']) CELESTIAL_EFFECTS['eclipse-solar'].sandRender = function (box) {
      defaultSandRender(box);
      // Eclipse glow is rendered via CSS box-shadow on the waist when
      // [data-cal-hourglass-theme="eclipse-solar"] is set.
    };
    if (CELESTIAL_EFFECTS['eclipse-lunar']) CELESTIAL_EFFECTS['eclipse-lunar'].sandRender = function (box) {
      defaultSandRender(box);
    };
    // TBD celestial stubs.
    ['volcanic', 'ice-age', 'plague', 'arcane-surge', 'moon-special', 'aurora', 'comet'].forEach(function (id) {
      if (CELESTIAL_EFFECTS[id]) CELESTIAL_EFFECTS[id].sandRender = defaultSandRender;
    });
  }

  // Picks the active sand theme for the current day. Celestial wins
  // over weather per dispatch stop-and-flag #3 (multi-effect resolution).
  function activeSandThemeForDay(m, day) {
    // Source from worldState (kept in lockstep with DATA on day-nav) so a
    // synthetic setWorldState({weather|events}) re-themes the sand too;
    // celestial still wins over weather per stop-and-flag #3.
    var cel = (worldState && worldState.events) ? worldState.events : celestialFor(m, day);
    if (cel && cel.length) return { id: cel[0].type, kind: 'celestial' };
    var effID = (worldState && worldState.weather && worldState.weather.type) ||
                (function () { var w = dayWeatherTypeID(m, day); return w ? weatherEffectID(w) : 'clear'; })();
    return { id: effID, kind: 'weather' };
  }

  // The in-glass interior is a canvas surface (clipped to the glass silhouette)
  // driven by the shared engine's frame hook (WAVE 1). The handle is created
  // in the hourglass-internals block. The sand grains + pile are owned by
  // HG_INTERIOR (heightmap sim), not engine emitters — so feedHourglassStream
  // now just syncs the themed sand COLOUR into the sim.
  var GLASS_SURFACE = null;
  function feedHourglassStream() {
    var stream = 'oklch(0.86 0.16 80)';
    // WAVE 2: prefer the active weather/celestial effect's hgSand colour so
    // the hourglass sand recolors in sync with the sky-band. Fall back to the
    // legacy per-theme --sand-stream CSS var.
    var effID = (worldState && worldState.weather && worldState.weather.type) || 'clear';
    var v2 = (typeof weatherV2Id === 'function') ? weatherV2Id(effID, worldState && worldState.events) : null;
    if (v2 && EFFECTS[v2] && EFFECTS[v2].hgSand && EFFECTS[v2].hgSand.color) {
      stream = EFFECTS[v2].hgSand.color;
    } else {
      var hg = document.querySelector('[data-cal-time]');
      if (hg) { var v = getComputedStyle(hg).getPropertyValue('--sand-stream'); if (v && v.trim()) stream = v.trim(); }
    }
    HG_INTERIOR.setSandColor(stream);
  }

  function applySandTheme(m, day) {
    var hg = document.querySelector('[data-cal-time]');
    if (!hg) return;
    var theme = activeSandThemeForDay(m, day);
    hg.setAttribute('data-cal-hourglass-theme', theme.id);
    var label = hg.querySelector('[data-cal-hourglass-theme-label]');
    if (label) {
      var name = theme.id;
      if (theme.kind === 'weather') {
        var w = (DATA.weather_effects || []).find(function (e) { return e.id === theme.id; });
        if (w) name = w.name;
      } else {
        var c = (DATA.celestial_effects || []).find(function (e) { return e.id === theme.id; });
        if (c) name = c.name;
      }
      label.textContent = name;
    }
    // Re-feed the in-glass canvas stream with the new theme colour.
    feedHourglassStream();
  }

  function isNightFrac(t) {
    var rise = (DATA && DATA.sunrise) || 0.25;
    var set  = (DATA && DATA.sunset)  || 0.75;
    if (rise < set) return t < rise || t >= set;
    return t >= set && t < rise;
  }

  function applyHourglassLevels(t) {
    var hg = document.querySelector('[data-cal-time]');
    if (!hg) return;
    var rise = (DATA && DATA.sunrise) || 0.25;
    var set  = (DATA && DATA.sunset)  || 0.75;
    var night = isNightFrac(t);
    // Fraction of the current half-day remaining (1 → fresh, 0 → empty).
    var halfRemaining;
    if (!night) {
      var dayLen = (set - rise + 1) % 1; if (dayLen === 0) dayLen = 0.5;
      halfRemaining = Math.max(0, Math.min(1, (set - t + (t < rise ? -1 : 0)) / dayLen));
      if (t < rise) halfRemaining = 0; // before sunrise → day-sand not started
    } else {
      var nightLen = (rise - set + 1) % 1; if (nightLen === 0) nightLen = 0.5;
      var elapsed = (t - set + 1) % 1;
      halfRemaining = Math.max(0, Math.min(1, 1 - (elapsed / nightLen)));
    }
    var topFill = hg.querySelector('[data-cal-hourglass-fill="top"]');
    var botFill = hg.querySelector('[data-cal-hourglass-fill="bot"]');
    if (topFill) topFill.style.setProperty('--fill', halfRemaining.toFixed(3));
    if (botFill) botFill.style.setProperty('--fill', (1 - halfRemaining).toFixed(3));
  }

  // Flip orientation when the day-state crosses sunrise/sunset. Tracks
  // last-applied state so we only animate on crossings.
  var __hgLastNight = null;
  function applyHourglassFlip(t, opts) {
    var hg = document.querySelector('[data-cal-time]');
    if (!hg) return;
    var night = isNightFrac(t);
    var first = (__hgLastNight === null);
    if (!first && night === __hgLastNight) return;
    __hgLastNight = night;
    if (first || (opts && opts.instant)) {
      hg.setAttribute('data-cal-hourglass-flipped', night ? 'true' : 'false');
      return;
    }
    // Animate the crossing.
    hg.setAttribute('data-cal-hourglass-flipping', 'true');
    hg.setAttribute('data-cal-hourglass-flipped', night ? 'true' : 'false');
    setTimeout(function () { hg.removeAttribute('data-cal-hourglass-flipping'); }, 1500);
  }
  // WAVE 3: force the period-boundary flip (fill hit the cap / "end day") even
  // without a time-of-day crossing — toggles the chambers + keeps __hgLastNight
  // in sync so the Wave-1 crossing logic doesn't immediately undo it.
  function forceHourglassFlip() {
    var hg = document.querySelector('[data-cal-time]');
    if (!hg) return;
    var nowFlipped = hg.getAttribute('data-cal-hourglass-flipped') === 'true';
    __hgLastNight = !nowFlipped;
    hg.setAttribute('data-cal-hourglass-flipping', 'true');
    hg.setAttribute('data-cal-hourglass-flipped', nowFlipped ? 'false' : 'true');
    setTimeout(function () { hg.removeAttribute('data-cal-hourglass-flipping'); }, 1500);
  }

  // ============================================================
  // WAVE 1 — Hourglass interior sim (heightmap sand + day/night).
  // ============================================================
  // Ported from prototypes/hourglass-meteor-daynight-mockup.html into the
  // production 60×110 triangular geometry. Runs on the engine's per-surface
  // frame hook (one shared rAF; reduced-motion → one static frame). The
  // BOTTOM chamber renders the current half-day sky from worldState.timeOfDay
  // (sun arcs + sets behind the sand horizon, stars emerge); the live pile is
  // a slope-limited column-height heightmap fed by the neck stream. The v4
  // dawn/dusk FLIP (applyHourglassFlip) is preserved + orthogonal — the glass
  // shell still rotates 180° at the boundary; the canvas counter-rotates (CSS)
  // so the sand always obeys gravity and the sky stays upright.

  // --- pure heightmap + sky math (unit-tested in test/js/hourglass.test.mjs) ---
  // Slope-limited avalanche: any column-pair steeper than the angle-of-repose
  // slope sheds half the excess to its lower neighbour. A few passes/frame.
  function hgAvalanche(bh, repose, iters) {
    var n = bh.length;
    for (var it = 0; it < iters; it++) {
      for (var i = 0; i < n - 1; i++) {
        var d = bh[i] - bh[i + 1];
        if (d > repose) { var mv = (d - repose) * 0.5; bh[i] -= mv; bh[i + 1] += mv; }
        else if (-d > repose) { var mv2 = (-d - repose) * 0.5; bh[i + 1] -= mv2; bh[i] += mv2; }
      }
    }
    return bh;
  }
  function hgHex(h) { h = h.replace('#', ''); return [parseInt(h.substr(0, 2), 16), parseInt(h.substr(2, 2), 16), parseInt(h.substr(4, 2), 16)]; }
  function hgMix(a, b, t) { var A = hgHex(a), B = hgHex(b); return 'rgb(' + Math.round(A[0] + (B[0] - A[0]) * t) + ',' + Math.round(A[1] + (B[1] - A[1]) * t) + ',' + Math.round(A[2] + (B[2] - A[2]) * t) + ')'; }
  // Day→night sky keyframes keyed by timeOfDay (0..1) → [topColor, botColor].
  var HG_SKY = [
    [0.00, '#04060f', '#0a0c1c'], [0.20, '#0a0f28', '#1a1a3a'], [0.28, '#5a86c0', '#e6a06a'],
    [0.50, '#4a86c8', '#bfe0ff'], [0.70, '#5a86c0', '#e6895a'], [0.80, '#2a2350', '#7a4d6a'], [1.00, '#04060f', '#0a0c1c']
  ];
  function hgSkyForTimeOfDay(tod) {
    if (tod < 0) tod = 0; if (tod > 1) tod = 1;
    for (var i = 0; i < HG_SKY.length - 1; i++) {
      if (tod >= HG_SKY[i][0] && tod <= HG_SKY[i + 1][0]) {
        var span = (HG_SKY[i + 1][0] - HG_SKY[i][0]) || 1, t = (tod - HG_SKY[i][0]) / span;
        return [hgMix(HG_SKY[i][1], HG_SKY[i + 1][1], t), hgMix(HG_SKY[i][2], HG_SKY[i + 1][2], t)];
      }
    }
    var L = HG_SKY[HG_SKY.length - 1]; return [L[1], L[2]];
  }
  // Sun position within the bottom chamber for a timeOfDay: arcs left→right,
  // rises then sinks toward the horizon, fades out near dawn/dusk. y is the
  // normalized arc height (0 horizon → 1 zenith); not visible outside the day.
  function hgSunPos(tod) {
    var DAWN = 0.22, DUSK = 0.80;
    if (tod <= DAWN || tod >= DUSK) return { visible: false, x: 0, y: 0, alpha: 0 };
    var pp = (tod - DAWN) / (DUSK - DAWN);
    var alpha = pp < 0.12 ? pp / 0.12 : (pp > 0.88 ? (1 - pp) / 0.12 : 1);
    return { visible: true, x: pp, y: Math.sin(Math.PI * pp), alpha: Math.max(0, Math.min(1, alpha)) };
  }
  // Star fade-in (0 day → 1 deep night), used for twinkle alpha.
  function hgStarFade(tod) {
    if (tod < 0.22 || tod > 0.80) return 1;
    if (tod < 0.30) return Math.max(0, (0.30 - tod) / 0.08);
    if (tod > 0.72) return Math.max(0, (tod - 0.72) / 0.08);
    return 0;
  }
  window.__calHgSim = { avalanche: hgAvalanche, sky: hgSkyForTimeOfDay, sun: hgSunPos, starFade: hgStarFade };

  // Stateful bottom-chamber interior. Geometry in viewBox 60×110 coords:
  // bottom chamber triangle (6,102)-(54,102)-(33,57)-(27,57); neck ~ y55.
  var HG_INTERIOR = (function () {
    var BCOLS = 48, V_BX0 = 6, V_BW = 48, V_FLOOR = 102, V_CEIL = 57, V_NECK = 55;
    var bcwV = V_BW / BCOLS, reposeV = bcwV * 0.9, CHAMBER = V_FLOOR - V_CEIL;
    var bh = new Float32Array(BCOLS), stream = [], _t = 0;
    var _skyGrad = null, _skyGradKey = '';   // §5 perf: cached sky gradient
    var sandColor = 'oklch(0.86 0.16 80)';
    // WAVE 3: fillFloor = the verb-controlled elapsed-period fill (a guaranteed
    // minimum sand level the ambient pour builds on, decoupled from the ambient
    // heightmap `bh`). _reverseT = the brief step-back "grains lift" flourish.
    var fillFloor = 0, _reverseT = 0;
    function setSandColor(c) { if (c && String(c).trim()) sandColor = String(c).trim(); }
    function setFill(frac01) { fillFloor = Math.max(0, Math.min(1, frac01 || 0)) * (CHAMBER * 0.6); }
    function reverseSand() { _reverseT = 0.4; }     // 400ms reverse-sand visual
    function reset() { for (var i = 0; i < BCOLS; i++) bh[i] = 0; stream.length = 0; }
    function stepSim(dt) {
      var f = Math.min(3, dt / 0.016);            // normalize to ~60fps sim rate
      if (_reverseT > 0) {
        // Step-back flourish: grains lift back up the neck for ~400ms.
        _reverseT -= dt;
        for (var r = stream.length - 1; r >= 0; r--) { var g = stream[r]; g.vy = -Math.abs(g.vy) - 0.4; g.y += g.vy * f; if (g.y < V_NECK - 6) stream.splice(r, 1); }
        return;
      }
      if (stream.length < 18) stream.push({ x: 30 + (Math.random() - 0.5) * 2.2, y: V_NECK, vy: 0.7 + Math.random() * 0.4, vx: (Math.random() - 0.5) * 0.12, r: 0.5 + Math.random() * 0.8 });
      for (var n = stream.length - 1; n >= 0; n--) {
        var s = stream[n]; s.vy += 0.02 * f; s.y += s.vy * f; s.x += s.vx * f;
        if (s.y >= V_CEIL) {
          var col = Math.floor((s.x - V_BX0) / bcwV); if (col < 0) col = 0; if (col > BCOLS - 1) col = BCOLS - 1;
          if (s.y >= V_FLOOR - bh[col]) { bh[col] += bcwV * 0.5; stream.splice(n, 1); continue; }
        }
        if (s.y > V_FLOOR + 2) stream.splice(n, 1);
      }
      hgAvalanche(bh, reposeV, 3);
      var mx = 0; for (var j = 0; j < BCOLS; j++) if (bh[j] > mx) mx = bh[j];
      if (mx > CHAMBER) reset();                  // chamber full → cadence reset (flip owns the real boundary)
    }
    function draw(ctx, w, h) {
      var sx = w / 60, sy = h / 110;
      var tod = (worldState && typeof worldState.timeOfDay === 'number') ? worldState.timeOfDay : 0.5;
      var y0 = V_CEIL * sy, y1 = V_FLOOR * sy;
      // §5 perf: cache the sky gradient by size + quantized time-of-day so we
      // don't allocate a new gradient on every frame (only when it changes).
      var gkey = w + 'x' + h + ':' + Math.round(tod * 200);
      if (gkey !== _skyGradKey) {
        var sky = hgSkyForTimeOfDay(tod);
        _skyGrad = ctx.createLinearGradient(0, y0, 0, y1); _skyGrad.addColorStop(0, sky[0]); _skyGrad.addColorStop(1, sky[1]);
        _skyGradKey = gkey;
      }
      var g = _skyGrad;
      ctx.fillStyle = g; ctx.fillRect(V_BX0 * sx, y0, V_BW * sx, y1 - y0);
      var dark = hgStarFade(tod);
      if (dark > 0.02) {
        ctx.fillStyle = '#fff';
        for (var i = 0; i < 14; i++) {
          var stx = (V_BX0 + (i * 37 % V_BW)) * sx, sty = (V_CEIL + 2 + (i * 53 % (CHAMBER - 6))) * sy, tw = 0.5 + 0.5 * Math.sin(_t * 2 + i);
          ctx.globalAlpha = dark * tw * 0.9; ctx.beginPath(); ctx.arc(stx, sty, 0.7 * sx, 0, 6.283); ctx.fill();
        }
        ctx.globalAlpha = 1;
      }
      var sun = hgSunPos(tod);
      if (sun.visible) {
        var cx = (V_BX0 + sun.x * V_BW) * sx, arcTop = (V_CEIL + 6) * sy, arcBot = (V_FLOOR - 4) * sy, cy = arcBot - (arcBot - arcTop) * sun.y;
        ctx.globalAlpha = sun.alpha;
        var rg = ctx.createRadialGradient(cx, cy, 1, cx, cy, 9 * sx); rg.addColorStop(0, 'rgba(255,240,200,.9)'); rg.addColorStop(1, 'rgba(255,200,120,0)');
        ctx.fillStyle = rg; ctx.beginPath(); ctx.arc(cx, cy, 9 * sx, 0, 6.283); ctx.fill();
        ctx.fillStyle = '#fff3cf'; ctx.beginPath(); ctx.arc(cx, cy, 3.2 * sx, 0, 6.283); ctx.fill();
        ctx.globalAlpha = 1;
      }
      // pile (heightmap) — drawn AFTER the sun so the sun sinks behind it. The
      // visible column = max(ambient pile, the verb-controlled fillFloor).
      ctx.beginPath(); ctx.moveTo(V_BX0 * sx, V_FLOOR * sy);
      for (var c = 0; c < BCOLS; c++) ctx.lineTo((V_BX0 + c * bcwV + bcwV / 2) * sx, (V_FLOOR - Math.max(bh[c], fillFloor)) * sy);
      ctx.lineTo((V_BX0 + V_BW) * sx, V_FLOOR * sy); ctx.closePath();
      ctx.fillStyle = sandColor; ctx.fill();
      ctx.globalAlpha = 0.95;
      for (var k = 0; k < stream.length; k++) { var s2 = stream[k]; ctx.beginPath(); ctx.arc(s2.x * sx, s2.y * sy, s2.r * sx, 0, 6.283); ctx.fill(); }
      ctx.globalAlpha = 1;
      // WAVE 2 — mood-tint composite (resolution step 6): washes BOTH chambers
      // + the sand via an 'overlay' blend (screen-on-light / multiply-on-dark),
      // clipped to the glass interior. Composes ONTO the already-drawn sand —
      // never clobbers the weather/event sand color.
      if (_moodColor && _moodA > 0) {
        ctx.save();
        ctx.globalCompositeOperation = 'overlay';
        ctx.globalAlpha = _moodA;
        ctx.fillStyle = _moodColor;
        ctx.fillRect(0, 0, w, h);
        ctx.restore();
      }
    }
    var _moodColor = null, _moodA = 0;
    function setMood(color, alpha) { _moodColor = color || null; _moodA = alpha || 0; }
    // Engine frame hook: step the sim when time advances (dt>0); always draw
    // (dt=0 = the reduced-motion static frame).
    function frame(ctx, w, h, dt) { if (dt > 0) { _t += dt; stepSim(dt); } draw(ctx, w, h); }
    return { frame: frame, setSandColor: setSandColor, setMood: setMood, setFill: setFill, reverseSand: reverseSand, reset: reset };
  })();

  registerInitBlock('hourglass-render', function () {
    var widget = document.querySelector('[data-cal-time]');
    var handle = document.querySelector('[data-cal-time-drag]');
    var tick = document.querySelector('[data-cal-time-tick]');
    if (!widget || !handle) return;
    var dragging = false, sx = 0, sy = 0, sl = 0, st = 0;
    handle.addEventListener('pointerdown', function (ev) {
      dragging = true; widget.setAttribute('data-cal-time-dragging', 'true');
      try { handle.setPointerCapture(ev.pointerId); } catch (e) {}
      sx = ev.clientX; sy = ev.clientY; var r = widget.getBoundingClientRect(); sl = r.left; st = r.top;
      widget.style.left = sl + window.scrollX + 'px'; widget.style.top = st + window.scrollY + 'px'; ev.preventDefault();
    });
    handle.addEventListener('pointermove', function (ev) {
      if (!dragging) return;
      widget.style.left = (sl + window.scrollX + (ev.clientX - sx)) + 'px';
      widget.style.top = (st + window.scrollY + (ev.clientY - sy)) + 'px';
    });
    function end(ev) { if (!dragging) return; dragging = false; widget.removeAttribute('data-cal-time-dragging'); try { handle.releasePointerCapture(ev.pointerId); } catch (e) {} }
    handle.addEventListener('pointerup', end); handle.addEventListener('pointercancel', end);
    if (tick) tick.addEventListener('click', function () {
      var hpd = (DATA && DATA.calendar && DATA.calendar.hours_per_day) || 24;
      applyTime((VIEW.timeFrac + 1 / hpd) % 1.0);
    });
    // Initial state.
    applyHourglassLevels(VIEW.timeFrac);
  });

  registerInitBlock('hourglass-flip', function () {
    applyHourglassFlip(VIEW.timeFrac, { instant: true });
    // Wave 0: hg levels + flip are driven by the hourglass subscriber
    // (registered in 'world-state-subscribers') on every timeOfDay change —
    // the v5 applyTime monkey-patch wrap is gone.
  });

  // hourglass-internals (§D2): create the in-glass canvas surface,
  // clipped to the glass interior, and feed it the falling-stream grains
  // via the SHARED engine (one rAF, two surfaces). This MUST run before
  // hourglass-themed-sand so applySandTheme can feed it.
  registerInitBlock('hourglass-internals', function () {
    var canvas = document.querySelector('[data-cal-hourglass-canvas]');
    if (!canvas || !window.CalParticleEngine) return;
    // Clip to the hourglass silhouette (proportional to the viewBox path
    // M6 8 H54 L33 53 V57 L54 102 H6 L27 57 V53 Z on a 60×110 viewBox).
    function clip(ctx, w, h) {
      var sx = w / 60, sy = h / 110;
      ctx.beginPath();
      ctx.moveTo(6 * sx, 8 * sy); ctx.lineTo(54 * sx, 8 * sy); ctx.lineTo(33 * sx, 53 * sy);
      ctx.lineTo(33 * sx, 57 * sy); ctx.lineTo(54 * sx, 102 * sy); ctx.lineTo(6 * sx, 102 * sy);
      ctx.lineTo(27 * sx, 57 * sy); ctx.lineTo(27 * sx, 53 * sy); ctx.closePath();
      ctx.clip();
    }
    GLASS_SURFACE = CalParticleEngine.createSurface(canvas, { clip: clip });
    // WAVE 1: the interior (heightmap sand + day/night sky) renders via the
    // engine's per-surface frame hook — one shared rAF, reduced-motion-gated.
    GLASS_SURFACE.setFrame(function (ctx, w, h, dt) { HG_INTERIOR.frame(ctx, w, h, dt); });
    feedHourglassStream();
    window.__calGlassEngine = GLASS_SURFACE;
    try {
      if ('ResizeObserver' in window) {
        var ro = trackObserver(new ResizeObserver(function () { GLASS_SURFACE.resize(); feedHourglassStream(); }));
        ro.observe(canvas);
      }
    } catch (e) {}
  });

  registerInitBlock('hourglass-themed-sand', function () {
    hookSandRenderers();
    applySandTheme(VIEW.month, VIEW.day);
    feedHourglassStream();
    // Wave 0: the sand theme is re-applied by the hourglass subscriber
    // (registered in 'world-state-subscribers') on every day/weather change —
    // the v5 renderSkyForDay monkey-patch wrap is gone.
  });

  // ============================================================
  // Event/day lookups for the popups
  // ============================================================
  function eventTouchesDay(e, m, day) {
    if (!e.end_month) return e.month === m && e.day === day;
    if (e.month === m && day >= e.day) { if (e.end_month === m && day <= e.end_day) return true; if (e.end_month > m) return true; }
    if (e.end_month === m && day <= e.end_day && e.month < m) return true;
    if (e.month < m && e.end_month > m) return true;
    return false;
  }
  function recurringTouchesDay(rec, m, day) {
    if (!rec.interval_days) return false;
    var start = (rec.start_month - 1) * 30 + rec.start_day, cur = (m - 1) * 30 + day;
    return cur >= start && (cur - start) % rec.interval_days === 0;
  }
  function findEventById(id) {
    if (!DATA) return null;
    if (id && id.indexOf('@') !== -1) {
      var parts = id.split('@'), rec = (DATA.recurring || []).find(function (r) { return r.id === parts[0]; });
      if (rec) {
        var dp = parts[1].split('-'), name = (rec.overrides && rec.overrides[parts[1]]) || rec.name;
        return { id: id, name: name, description: rec.description, year: +dp[0], month: +dp[1], day: +dp[2],
          hour: rec.hour, tier: rec.tier, category: rec.category, visibility: 'public', recurring_ref: rec.id };
      }
    }
    return (DATA.events || []).find(function (e) { return e.id === id; }) || null;
  }
  function eventsForDay(m, day) {
    var out = [];
    (DATA.events || []).forEach(function (e) { if (eventTouchesDay(e, m, day)) out.push(e); });
    (DATA.recurring || []).forEach(function (rec) { if (recurringTouchesDay(rec, m, day)) out.push(findEventById(rec.id + '@' + key(DATA.current_year, m, day))); });
    return out;
  }
  function categoryById(id) { return (DATA.categories || []).find(function (c) { return c.id === id; }); }
  function monthName(i) { var mo = (DATA.months || [])[i - 1]; return mo ? mo.name : String(i); }

  // ============================================================
  // Block: popup-slidein — click event/day → quick-view (tier 1)
  // ============================================================
  var CTX = null; // current popup context: { kind:'event'|'day', event, month, day }

  registerInitBlock('popup-slidein', function () {
    var qv = document.querySelector('[data-cal-qv]');
    if (!qv) return;
    document.addEventListener('click', function (ev) {
      if (ev.target.closest('[data-cal-qv]') || ev.target.closest('[data-cal-editor]') ||
          ev.target.closest('[data-cal-time]') || ev.target.closest('[data-cal-skypanel]')) return;
      // Click blank sky → what's-happening panel.
      var skyHit = ev.target.closest('[data-cal-sky]');
      if (skyHit && !ev.target.closest('[data-cal-sky-sun]') && !ev.target.closest('[data-cal-sky-time-label]') &&
          !ev.target.closest('[data-cal-sky-moon]') && !ev.target.closest('[data-cal-era-vignette]')) {
        openHappeningPanel(); return;
      }
      var chip = ev.target.closest('[data-cal-event-id]');
      if (chip) { openEventQuickview(findEventById(chip.getAttribute('data-cal-event-id'))); return; }
      var cell = ev.target.closest('[data-cal-cell]');
      if (cell) {
        var m = +cell.getAttribute('data-cell-month'), day = +cell.getAttribute('data-cell-day');
        openDayQuickview(m, day); return;
      }
      closeQuickview(); closeEditor();
    });
    // Quick-view controls.
    var close = qv.querySelector('[data-cal-qv-close]'); if (close) close.addEventListener('click', function () { closeQuickview(); closeEditor(); });
    var save = qv.querySelector('[data-cal-qv-save]'); if (save) save.addEventListener('click', function () { commitQuickview(); flash(save, 'Saved ✓'); });
    [qv.querySelector('[data-cal-qv-expand]'), qv.querySelector('[data-cal-qv-expand2]')].forEach(function (b) {
      if (b) b.addEventListener('click', expandToEditor);
    });
    // Escape handled centrally by the 'dialog-a11y' block (closes the topmost open dialog).
  });

  function openEventQuickview(e) {
    if (!e) return;
    CTX = { kind: 'event', event: e, month: e.month, day: e.day };
    var cat = categoryById(e.category) || {};
    setQV(e.name, monthName(e.month) + ' ' + e.day + ' · ' + (cat.name || e.category) + (e.hour >= 0 ? ' · ' + pad2(e.hour) + ':00' : ''), e.description || '');
    var box = document.querySelector('[data-cal-qv-events]'); if (box) box.innerHTML = e.recurring_ref ? '<p style="margin:0;font-size:11px;color:oklch(0.74 0.022 261)">↻ Recurring (' + esc(e.recurring_ref) + ') — edits affect this instance.</p>' : '';
    fillState(e.month, e.day);
    fillLinks(e);
    showQuickview();
  }
  function openDayQuickview(m, day) {
    CTX = { kind: 'day', event: null, month: m, day: day };
    var wd = (DATA.weekdays || [])[(day - 1) % (DATA.weekdays || []).length];
    setQV(monthName(m) + ' ' + day, (wd ? wd.name : '') + ' · ' + DATA.current_year + ' ' + DATA.calendar.epoch_name, dayNote(m, day));
    var box = document.querySelector('[data-cal-qv-events]');
    if (box) {
      box.innerHTML = '';
      var evs = eventsForDay(m, day);
      evs.forEach(function (e) {
        if (!e) return; var cat = categoryById(e.category) || {};
        var row = document.createElement('div'); row.className = 'cal-almanac-qv__pop-event';
        row.style.setProperty('--chip-cat', cat.color || 'oklch(0.62 0.18 240)');
        row.innerHTML = '<span>' + esc(e.name) + '</span>' + (e.hour >= 0 ? '<span style="margin-left:auto;font-family:JetBrains Mono,monospace;font-size:11px">' + pad2(e.hour) + ':00</span>' : '');
        row.addEventListener('click', function () { openEventQuickview(e); });
        box.appendChild(row);
      });
      if (!evs.length) { var em = document.createElement('div'); em.style.cssText = 'font-size:11px;color:oklch(0.62 0.022 261);font-style:italic'; em.textContent = 'No events.'; box.appendChild(em); }
    }
    fillState(m, day);
    fillLinks(null);
    showQuickview();
  }
  function setQV(title, meta, desc) {
    var t = document.querySelector('[data-cal-qv-title]'); if (t) t.value = title;
    var mt = document.querySelector('[data-cal-qv-meta]'); if (mt) mt.textContent = meta;
    var d = document.querySelector('[data-cal-qv-desc]'); if (d) d.value = desc || '';
  }
  function fillState(m, day) {
    var box = document.querySelector('[data-cal-qv-state]'); if (!box) return;
    box.innerHTML = '';
    var wtypeID = dayWeatherTypeID(m, day);
    if (wtypeID) { var wt = weatherTypeById(wtypeID); if (wt) box.appendChild(stateRow('Weather', wt.name + ' · ' + wt.temp_c + '°C', wt.color)); }
    var cel = celestialFor(m, day);
    cel.forEach(function (c) { box.appendChild(stateRow('Celestial', c.name, 'oklch(0.88 0.10 90)')); });
  }
  function stateRow(label, val, color) {
    var d = document.createElement('div');
    d.style.cssText = 'font-size:12px;padding:5px 8px;border-radius:5px;background:oklch(0.16 0.02 264);border-left:3px solid ' + (color || 'oklch(0.5 0.02 260)');
    d.innerHTML = '<span style="color:oklch(0.74 0.022 261)">' + esc(label) + ':</span> ' + esc(val);
    return d;
  }
  function fillLinks(e) {
    var box = document.querySelector('[data-cal-qv-links]'); if (!box) return;
    box.innerHTML = '';
    function lk(txt, tab) { var b = document.createElement('button'); b.type = 'button'; b.className = 'cal-almanac-qv__link'; b.textContent = txt;
      b.addEventListener('click', function () { expandToEditor(); setEditorTab(tab); }); box.appendChild(b); }
    var rules = 0;
    if (e) { rules = (e.allow_users || []).length + (e.deny_users || []).length; }
    lk('Visibility' + (rules ? ' (' + rules + ')' : ''), 'vis');
    lk('Notes', 'notes');
    if (e && e.recurring_ref) lk('Recurring series', 'detail');
    if (e && (DATA.event_entities || {})[e.id]) lk('Entities (' + (DATA.event_entities[e.id] || []).length + ')', 'detail');
  }
  // ============================================================
  // Slice 2 a11y — dialog focus management (shared by quick-view / editor /
  // create / sky-panel). focus-INTO on open, focus-RESTORE to the trigger on
  // close, Tab focus-TRAP while open, and ONE Escape handler that closes the
  // topmost open dialog (replaces the two drifted Escape listeners).
  // ============================================================
  var DIALOG_STACK = []; // { el, trigger, close }
  function dlgFocusables(el) {
    return Array.prototype.slice.call(el.querySelectorAll(
      'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])'
    ));
  }
  function openDialog(el, closeFn) {
    if (!el) return;
    // avoid double-push if re-opened.
    for (var i = 0; i < DIALOG_STACK.length; i++) if (DIALOG_STACK[i].el === el) return;
    var trigger = document.activeElement;
    DIALOG_STACK.push({ el: el, trigger: trigger, close: closeFn });
    var f = dlgFocusables(el);
    var target = f[0] || el;
    try { setTimeout(function () { try { target.focus(); } catch (e) {} }, 0); } catch (e) {}
  }
  function closeDialog(el) {
    for (var i = DIALOG_STACK.length - 1; i >= 0; i--) {
      if (DIALOG_STACK[i].el === el) {
        var d = DIALOG_STACK.splice(i, 1)[0];
        if (d.trigger && d.trigger.focus) { try { d.trigger.focus(); } catch (e) {} }
        break;
      }
    }
  }
  registerInitBlock('dialog-a11y', function () {
    document.addEventListener('keydown', function (ev) {
      if (!DIALOG_STACK.length) return;
      var top = DIALOG_STACK[DIALOG_STACK.length - 1];
      if (ev.key === 'Escape') { ev.preventDefault(); if (top.close) top.close(); return; }
      if (ev.key === 'Tab') {
        var f = dlgFocusables(top.el); if (!f.length) return;
        var first = f[0], last = f[f.length - 1], a = document.activeElement;
        // keep focus inside the topmost dialog (trap).
        if (ev.shiftKey && (a === first || !top.el.contains(a))) { ev.preventDefault(); last.focus(); }
        else if (!ev.shiftKey && (a === last || !top.el.contains(a))) { ev.preventDefault(); first.focus(); }
      }
    });
  });

  function showQuickview() {
    var qv = document.querySelector('[data-cal-qv]');
    if (qv) { qv.setAttribute('data-cal-qv-open', 'true'); qv.setAttribute('data-cal-qv-zoomed', 'false'); qv.setAttribute('aria-hidden', 'false'); openDialog(qv, closeQuickview); }
    closeSkyPanel();
  }
  function closeQuickview() { var qv = document.querySelector('[data-cal-qv]'); if (qv) { qv.setAttribute('data-cal-qv-open', 'false'); qv.setAttribute('aria-hidden', 'true'); closeDialog(qv); } }
  function commitQuickview() {
    if (!CTX) return;
    var title = (document.querySelector('[data-cal-qv-title]') || {}).value;
    var desc = (document.querySelector('[data-cal-qv-desc]') || {}).value;
    if (CTX.kind === 'event' && CTX.event) {
      var e = (DATA.events || []).find(function (x) { return x.id === CTX.event.id; });
      if (e) { if (title != null) e.name = title; if (desc != null) e.description = desc; }
    } else if (CTX.kind === 'day') {
      setDayNote(CTX.month, CTX.day, desc);
    }
  }
  function flash(btn, txt) { var t = btn.textContent; btn.textContent = txt; setTimeout(function () { btn.textContent = t; }, 1200); }
  function dayNote(m, day) { return (DATA.day_notes || {})[key(DATA.current_year, m, day)] || ''; }
  function setDayNote(m, day, txt) { if (!DATA.day_notes) DATA.day_notes = {}; DATA.day_notes[key(DATA.current_year, m, day)] = txt; }

  // ============================================================
  // Block: popup-expand — quick-view → editor (tier 2)
  // ============================================================
  function expandToEditor() {
    var qv = document.querySelector('[data-cal-qv]'); var ed = document.querySelector('[data-cal-editor]');
    if (!ed) return;
    if (qv) qv.setAttribute('data-cal-qv-zoomed', 'true');
    hydrateEditor();
    ed.setAttribute('data-cal-editor-open', 'true'); ed.setAttribute('aria-hidden', 'false');
    openDialog(ed, closeEditor);
  }
  function closeEditor() {
    var ed = document.querySelector('[data-cal-editor]'); var qv = document.querySelector('[data-cal-qv]');
    if (ed) { ed.setAttribute('data-cal-editor-open', 'false'); ed.setAttribute('aria-hidden', 'true'); closeDialog(ed); }
    if (qv) qv.setAttribute('data-cal-qv-zoomed', 'false');
    closeSubpop();
  }
  function setEditorTab(name) {
    var ed = document.querySelector('[data-cal-editor]'); if (!ed) return;
    ed.querySelectorAll('[data-cal-editor-tab]').forEach(function (t) {
      var on = t.getAttribute('data-cal-editor-tab') === name;
      t.classList.toggle('cal-almanac-editor__tab--active', on); t.setAttribute('aria-selected', on ? 'true' : 'false');
    });
    ed.querySelectorAll('[data-cal-editor-panel]').forEach(function (p) {
      var on = p.getAttribute('data-cal-editor-panel') === name;
      p.classList.toggle('cal-almanac-editor__panel--active', on); if (on) p.removeAttribute('hidden'); else p.setAttribute('hidden', '');
    });
  }
  function hydrateEditor() {
    if (!CTX) return;
    var e = CTX.event;
    var title = e ? e.name : (monthName(CTX.month) + ' ' + CTX.day);
    setText('[data-cal-editor-title]', title);
    setText('[data-cal-editor-meta]', e ? (monthName(e.month) + ' ' + e.day + ' · ' + e.tier + ' · ' + e.category) : ('Day editor · ' + DATA.current_year));
    setVal('[data-cal-editor-f-title]', title);
    setVal('[data-cal-editor-f-desc]', e ? (e.description || '') : '');
    if (e) { setVal('[data-cal-editor-f-tier]', e.tier); setVal('[data-cal-editor-f-cat]', e.category); }
    setVal('[data-cal-editor-notes]', CTX.kind === 'day' ? dayNote(CTX.month, CTX.day) : '');
    hydrateVisibility(e || { visibility: 'public', allow_users: [], deny_users: [] });
    // Linked entities.
    var linked = document.querySelector('[data-cal-editor-linked]');
    if (linked) {
      linked.innerHTML = '';
      var refs = e ? ((DATA.event_entities || {})[e.id] || []) : [];
      if (refs.length) {
        var h = document.createElement('div'); h.className = 'cal-almanac-editor__lbl'; h.textContent = 'Linked entities'; linked.appendChild(h);
        refs.forEach(function (r) {
          var row = document.createElement('div'); row.style.cssText = 'font-size:12px;padding:5px 8px;background:oklch(0.16 0.02 264);border-radius:5px';
          row.textContent = r.type + ' · ' + r.name; linked.appendChild(row);
        });
      }
    }
    // Recurring action label.
    var rl = document.querySelector('[data-cal-action-recurring-label]');
    if (rl) rl.textContent = (e && e.recurring_ref) ? 'Edit Recurrence…' : 'Set as Recurring…';
    // Override-weather only meaningful for day context; keep visible either way.
  }
  function setText(sel, v) { var el = document.querySelector(sel); if (el) el.textContent = v; }
  function setVal(sel, v) { var el = document.querySelector(sel); if (el) el.value = v; }

  registerInitBlock('popup-expand', function () {
    var ed = document.querySelector('[data-cal-editor]'); if (!ed) return;
    ed.querySelectorAll('[data-cal-editor-tab]').forEach(function (t) { t.addEventListener('click', function () { setEditorTab(t.getAttribute('data-cal-editor-tab')); }); });
    var close = ed.querySelector('[data-cal-editor-close]'); if (close) close.addEventListener('click', function () { closeEditor(); closeQuickview(); });
    var coll = ed.querySelector('[data-cal-editor-collapse]'); if (coll) coll.addEventListener('click', closeEditor);
    // Editor field commits back into the mock + reflect in quick-view.
    var ft = ed.querySelector('[data-cal-editor-f-title]'); if (ft) ft.addEventListener('input', function () {
      setText('[data-cal-editor-title]', ft.value); var qt = document.querySelector('[data-cal-qv-title]'); if (qt) qt.value = ft.value;
      if (CTX && CTX.event) { var e = (DATA.events || []).find(function (x) { return x.id === CTX.event.id; }); if (e) e.name = ft.value; }
    });
    var fn = ed.querySelector('[data-cal-editor-notes]'); if (fn) fn.addEventListener('input', function () { if (CTX && CTX.kind === 'day') setDayNote(CTX.month, CTX.day, fn.value); });
  });

  // ============================================================
  // Visibility editor (Q-V2-7 chip-row), reused inside the editor.
  // ============================================================
  function hydrateVisibility(e) {
    var radios = document.querySelectorAll('[data-cal-vis-mode]'); var rules = document.querySelector('[data-cal-vis-rules]'); var chips = document.querySelector('[data-cal-vis-chips]');
    if (!radios.length || !rules || !chips) return;
    var mode = (e.visibility === 'specific') ? 'specific' : 'public';
    radios.forEach(function (r) { r.checked = (r.value === mode); });
    if (mode === 'specific') rules.removeAttribute('hidden'); else rules.setAttribute('hidden', '');
    chips.innerHTML = '';
    (e.allow_users || []).forEach(function (u) { chips.appendChild(visChip('allow', u)); });
    (e.deny_users || []).forEach(function (u) { chips.appendChild(visChip('deny', u)); });
    refreshVisSummary();
  }
  function visChip(kind, name) {
    var li = document.createElement('li'); li.className = 'cal-almanac-vis-chip cal-almanac-vis-chip--' + kind;
    li.setAttribute('data-vis-kind', kind); li.setAttribute('data-vis-name', name);
    var ic = document.createElement('span'); ic.className = 'cal-almanac-vis-chip__icon'; ic.textContent = kind === 'allow' ? '✓' : '✗';
    var n = document.createElement('span'); n.className = 'cal-almanac-vis-chip__name'; n.textContent = '@' + name;
    var del = document.createElement('button'); del.type = 'button'; del.className = 'cal-almanac-vis-chip__del'; del.textContent = '×';
    del.addEventListener('click', function () { li.remove(); refreshVisSummary(); });
    li.appendChild(ic); li.appendChild(n); li.appendChild(del); return li;
  }
  function refreshVisSummary() {
    var s = document.querySelector('[data-cal-vis-summary]'); if (!s) return;
    var spec = document.querySelector('[data-cal-vis-mode][value="specific"]');
    if (!spec || !spec.checked) { s.textContent = 'Effective audience: everyone'; return; }
    var allow = [], deny = [];
    document.querySelectorAll('[data-cal-vis-chips] [data-vis-kind]').forEach(function (c) {
      (c.getAttribute('data-vis-kind') === 'allow' ? allow : deny).push(c.getAttribute('data-vis-name'));
    });
    var msg = !allow.length && !deny.length ? 'No rules: nobody can see this.' : (!allow.length ? 'Everyone except: ' + deny.join(', ') : (!deny.length ? allow.length + ' people: ' + allow.join(', ') : allow.join(', ') + ' (deny: ' + deny.join(', ') + ')'));
    s.textContent = 'Effective audience — ' + msg;
  }
  registerInitBlock('visibility-editor', function () {
    var radios = document.querySelectorAll('[data-cal-vis-mode]'); var rules = document.querySelector('[data-cal-vis-rules]');
    if (!radios.length || !rules) return;
    radios.forEach(function (r) { r.addEventListener('change', function () {
      if (r.checked && r.value === 'specific') rules.removeAttribute('hidden'); if (r.checked && r.value === 'public') rules.setAttribute('hidden', ''); refreshVisSummary();
    }); });
    document.querySelectorAll('[data-cal-vis-add]').forEach(function (b) { b.addEventListener('click', function () {
      // §2 FLAG: window.prompt is a DEMO-ONLY shortcut. The Phase-2/4 production
      // editor (C-CAL-WORLDSTATE-INTERACTION-REDESIGN / the real visibility UI)
      // replaces this with an inline user-picker — do NOT carry prompt() into
      // the port. Gated here behind a clearly-labeled demo affordance.
      var name = window.prompt('[Demo] Add ' + b.getAttribute('data-cal-vis-add') + ' rule for which user? (the production editor uses an inline picker)'); if (!name) return;
      var chips = document.querySelector('[data-cal-vis-chips]'); if (chips) chips.appendChild(visChip(b.getAttribute('data-cal-vis-add'), name.replace(/^@/, ''))); refreshVisSummary();
    }); });
  });

  // ============================================================
  // Block: action-menu — the 8 actions + their sub-popovers
  // ============================================================
  function openSubpop(title, html) {
    var sp = document.querySelector('[data-cal-subpop]'); if (!sp) return;
    setText('[data-cal-subpop-title]', title);
    var body = sp.querySelector('[data-cal-subpop-body]'); if (body) body.innerHTML = html;
    sp.removeAttribute('hidden');
    return body;
  }
  function closeSubpop() { var sp = document.querySelector('[data-cal-subpop]'); if (sp) sp.setAttribute('hidden', ''); }
  function toast(msg) {
    var t = document.createElement('div');
    t.style.cssText = 'position:fixed;left:50%;bottom:24px;transform:translateX(-50%);z-index:200;padding:10px 16px;background:oklch(0.26 0.03 264);border:1px solid oklch(0.45 0.04 258);border-radius:8px;color:oklch(0.985 0 0);font:13px Inter,system-ui,sans-serif;box-shadow:0 8px 20px -6px oklch(0 0 0 / 0.5)';
    t.textContent = msg; document.body.appendChild(t);
    setTimeout(function () { t.remove(); }, 2200);
  }
  registerInitBlock('action-menu', function () {
    var menu = document.querySelector('[data-cal-actions]'); if (!menu) return;
    menu.querySelectorAll('[data-cal-action]').forEach(function (b) {
      b.addEventListener('click', function () { runAction(b.getAttribute('data-cal-action')); });
    });
    var spc = document.querySelector('[data-cal-subpop-close]'); if (spc) spc.addEventListener('click', closeSubpop);
  });
  function runAction(a) {
    var e = CTX && CTX.event;
    switch (a) {
      case 'create-entity': {
        var html = (DATA.entity_types || []).map(function (t) {
          return '<button type="button" class="cal-almanac-subpop__opt" data-entity-type="' + t.id + '">' + esc(t.name) + '</button>';
        }).join('');
        var body = openSubpop('Create Entity From', html);
        if (body) body.querySelectorAll('[data-entity-type]').forEach(function (o) {
          o.addEventListener('click', function () { closeSubpop(); toast('Mock entity created (' + o.getAttribute('data-entity-type') + ') — real entity flow in the production port.'); });
        });
        break;
      }
      case 'timeline': toast('Timeline view: coming in the timeline arc.'); break;
      case 'permalink': {
        var url = '/campaigns/demo/calendar/' + DATA.current_year + '/' + (CTX ? CTX.month : DATA.current_month) + '/' + (CTX ? CTX.day : DATA.current_day);
        copy(url); toast('Permalink copied — ' + url); break;
      }
      case 'duplicate': {
        var b1 = openSubpop('Duplicate to date', '<label class="cal-almanac-editor__lbl">Day of ' + esc(monthName(CTX ? CTX.month : DATA.current_month)) + '</label><input class="cal-almanac-editor__field" data-dup-day type="number" min="1" max="30" value="' + (CTX ? CTX.day : 1) + '"/><button type="button" class="cal-almanac-subpop__opt" data-dup-go>Duplicate here</button>');
        if (b1) { var go = b1.querySelector('[data-dup-go]'); if (go) go.addEventListener('click', function () {
          var dd = +(b1.querySelector('[data-dup-day]').value || 0); closeSubpop();
          if (e) { var copy2 = JSON.parse(JSON.stringify(e)); copy2.id = e.id + '-dup-' + dd; copy2.day = dd; copy2.recurring_ref = ''; (DATA.events || []).push(copy2); }
          toast('Duplicated to ' + monthName(CTX.month) + ' ' + dd + ' (in-memory).');
        }); } break;
      }
      case 'recurring': {
        var html2 = ['daily', 'weekly', 'monthly', 'custom'].map(function (k) {
          return '<button type="button" class="cal-almanac-subpop__opt" data-rec="' + k + '">' + k.charAt(0).toUpperCase() + k.slice(1) + '</button>';
        }).join('');
        var b2 = openSubpop((e && e.recurring_ref) ? 'Edit Recurrence' : 'Set as Recurring', html2);
        if (b2) b2.querySelectorAll('[data-rec]').forEach(function (o) { o.addEventListener('click', function () { closeSubpop(); toast('Recurrence set: ' + o.getAttribute('data-rec') + ' (in-memory).'); }); });
        break;
      }
      case 'override-weather': {
        var html3 = (DATA.weather_types || []).map(function (w) {
          return '<button type="button" class="cal-almanac-subpop__opt" data-w="' + w.id + '"><span style="width:10px;height:10px;border-radius:50%;background:' + w.color + '"></span>' + esc(w.name) + '</button>';
        }).join('');
        var b3 = openSubpop('Override Weather — ' + monthName(CTX.month) + ' ' + CTX.day, html3);
        if (b3) b3.querySelectorAll('[data-w]').forEach(function (o) { o.addEventListener('click', function () {
          if (!DATA.day_weather) DATA.day_weather = {}; DATA.day_weather[key(DATA.current_year, CTX.month, CTX.day)] = o.getAttribute('data-w');
          closeSubpop(); renderSkyForDay(CTX.month, CTX.day); fillState(CTX.month, CTX.day); toast('Weather overridden — sky updated.');
        }); }); break;
      }
      case 'history': {
        var hist = e ? ((DATA.event_history || {})[e.id] || []) : [];
        var html4 = hist.length ? hist.map(function (h) { return '<div class="cal-almanac-subpop__hist"><b>' + esc(h.by) + '</b> · ' + esc(h.action) + '<br><span style="opacity:.6">' + esc(h.at) + '</span></div>'; }).join('') : '<div class="cal-almanac-subpop__hist">No history recorded.</div>';
        openSubpop('History', html4); break;
      }
      case 'delete': {
        var b5 = openSubpop('Delete', '<p style="margin:0 0 8px;font-size:13px">Delete &ldquo;' + esc(e ? e.name : (monthName(CTX.month) + ' ' + CTX.day)) + '&rdquo;? This cannot be undone.</p><button type="button" class="cal-almanac-subpop__opt" style="border-color:oklch(0.55 0.18 25);color:oklch(0.8 0.16 25)" data-del-confirm>Confirm delete</button><button type="button" class="cal-almanac-subpop__opt" data-del-cancel>Cancel</button>');
        if (b5) {
          var c = b5.querySelector('[data-del-confirm]'); var cn = b5.querySelector('[data-del-cancel]');
          if (c) c.addEventListener('click', function () {
            if (e) { var idx = (DATA.events || []).findIndex(function (x) { return x.id === e.id; }); if (idx >= 0) DATA.events.splice(idx, 1); }
            closeSubpop(); closeEditor(); closeQuickview(); toast('Deleted (in-memory).');
          });
          if (cn) cn.addEventListener('click', closeSubpop);
        }
        break;
      }
    }
  }
  function copy(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) { navigator.clipboard.writeText(text).catch(function () {}); }
    else { try { var ta = document.createElement('textarea'); ta.value = text; document.body.appendChild(ta); ta.select(); document.execCommand('copy'); document.body.removeChild(ta); } catch (e) {} }
  }

  // ============================================================
  // Sky "what's happening" panel
  // ============================================================
  function openHappeningPanel() {
    var m = VIEW.month, day = VIEW.day;
    var rows = [];
    var wtypeID = dayWeatherTypeID(m, day);
    if (wtypeID) { var wt = weatherTypeById(wtypeID); if (wt) rows.push('<div class="cal-almanac-skypanel__row"><span>🌦</span> <b>' + esc(wt.name) + '</b> · ' + wt.temp_c + '°C</div>'); }
    celestialFor(m, day).forEach(function (c) { rows.push('<div class="cal-almanac-skypanel__row"><span>✦</span> ' + esc(c.name) + '</div>'); });
    (DATA.moons || []).forEach(function (mo) { rows.push('<div class="cal-almanac-skypanel__row"><span>☾</span> ' + esc(mo.name) + '</div>'); });
    if (!rows.length) rows.push('<div class="cal-almanac-skypanel__row">A quiet day.</div>');
    openSkyPanel('What\'s happening · ' + monthName(m) + ' ' + day, rows.join(''));
  }
  function openSkyPanel(title, html) {
    var p = document.querySelector('[data-cal-skypanel]'); if (!p) return;
    setText('[data-cal-skypanel-title]', title);
    var body = p.querySelector('[data-cal-skypanel-body]'); if (body) body.innerHTML = html;
    p.setAttribute('data-cal-skypanel-open', 'true'); p.setAttribute('aria-hidden', 'false');
    openDialog(p, closeSkyPanel);
  }
  function closeSkyPanel() { var p = document.querySelector('[data-cal-skypanel]'); if (p) { p.setAttribute('data-cal-skypanel-open', 'false'); p.setAttribute('aria-hidden', 'true'); closeDialog(p); } }
  registerInitBlock('sky-panel', function () {
    var p = document.querySelector('[data-cal-skypanel]'); if (!p) return;
    var c = p.querySelector('[data-cal-skypanel-close]'); if (c) c.addEventListener('click', closeSkyPanel);
  });

  // ============================================================
  // Block: widget-drag (header handle moves the calendar widget)
  // ============================================================
  registerInitBlock('widget-drag', function () {
    var widget = document.querySelector('[data-cal-widget]'); var handle = document.querySelector('[data-cal-drag-handle]');
    if (!widget || !handle) return;
    var dragging = false, sx = 0, sy = 0, sl = 0, st = 0;
    handle.addEventListener('pointerdown', function (ev) {
      if (ev.target.closest('.cal-almanac-iconbtn')) return;
      dragging = true; widget.setAttribute('data-cal-dragging', 'true');
      try { handle.setPointerCapture(ev.pointerId); } catch (e) {}
      sx = ev.clientX; sy = ev.clientY; var r = widget.getBoundingClientRect(); sl = r.left; st = r.top;
      widget.style.left = sl + window.scrollX + 'px'; widget.style.top = st + window.scrollY + 'px'; ev.preventDefault();
    });
    handle.addEventListener('pointermove', function (ev) { if (!dragging) return; widget.style.left = (sl + window.scrollX + (ev.clientX - sx)) + 'px'; widget.style.top = (st + window.scrollY + (ev.clientY - sy)) + 'px'; });
    function end(ev) { if (!dragging) return; dragging = false; widget.removeAttribute('data-cal-dragging'); try { handle.releasePointerCapture(ev.pointerId); } catch (e) {} }
    handle.addEventListener('pointerup', end); handle.addEventListener('pointercancel', end);
  });

  // ============================================================
  // Block: widget-resize
  // ============================================================
  registerInitBlock('widget-resize', function () {
    var widget = document.querySelector('[data-cal-widget]'); var grip = document.querySelector('[data-cal-resize]');
    if (!widget || !grip) return;
    var resizing = false, sx = 0, sy = 0, sw = 0, sh = 0;
    grip.addEventListener('pointerdown', function (ev) { resizing = true; try { grip.setPointerCapture(ev.pointerId); } catch (e) {} sx = ev.clientX; sy = ev.clientY; var r = widget.getBoundingClientRect(); sw = r.width; sh = r.height; ev.stopPropagation(); ev.preventDefault(); });
    grip.addEventListener('pointermove', function (ev) { if (!resizing) return; widget.style.width = Math.max(520, sw + (ev.clientX - sx)) + 'px'; widget.style.height = Math.max(460, sh + (ev.clientY - sy)) + 'px'; });
    function end(ev) { if (!resizing) return; resizing = false; try { grip.releasePointerCapture(ev.pointerId); } catch (e) {} }
    grip.addEventListener('pointerup', end); grip.addEventListener('pointercancel', end);
  });

  // ============================================================
  // Block: drag-create
  // ============================================================
  registerInitBlock('drag-create', function () {
    var grid = document.querySelector('[data-cal-grid]'); var ghost = document.querySelector('[data-cal-drag-ghost]');
    if (!grid || !ghost) return;
    var dragging = false, startCell = null;
    function cellAt(x, y) { var el = document.elementFromPoint(x, y); return el && el.closest('[data-cal-cell]'); }
    function pos(a, b) { if (!a || !b) return; var ra = a.getBoundingClientRect(), rb = b.getBoundingClientRect(), g = grid.getBoundingClientRect();
      var l = Math.min(ra.left, rb.left) - g.left, t = Math.min(ra.top, rb.top) - g.top, r = Math.max(ra.right, rb.right) - g.left, btm = Math.max(ra.bottom, rb.bottom) - g.top;
      ghost.style.left = l + 'px'; ghost.style.top = t + 'px'; ghost.style.width = (r - l) + 'px'; ghost.style.height = (btm - t) + 'px'; }
    grid.addEventListener('pointerdown', function (ev) {
      if (ev.target.closest('[data-cal-event-id]')) return; var cell = ev.target.closest('[data-cal-cell]'); if (!cell) return;
      dragging = true; startCell = cell; ghost.setAttribute('data-cal-ghost-active', 'true'); pos(startCell, startCell);
      try { grid.setPointerCapture(ev.pointerId); } catch (e) {} ev.preventDefault();
    });
    grid.addEventListener('pointermove', function (ev) { if (!dragging) return; var c = cellAt(ev.clientX, ev.clientY); if (c) pos(startCell, c); });
    function end(ev) {
      if (!dragging) return; dragging = false; ghost.removeAttribute('data-cal-ghost-active'); try { grid.releasePointerCapture(ev.pointerId); } catch (e) {}
      var endCell = cellAt(ev.clientX, ev.clientY) || startCell;
      var s = +startCell.getAttribute('data-cell-day'), en = +endCell.getAttribute('data-cell-day');
      // Open a stub event in the quick-view.
      CTX = { kind: 'event', event: { id: 'new', name: 'New event', description: 'Drag-created ' + (s === en ? 'on day ' + s : 'days ' + s + '–' + en) + '. Save would create it; showcase is in-memory.', month: VIEW.month, day: s, hour: -1, tier: 'standard', category: 'session', visibility: 'public' }, month: VIEW.month, day: s };
      setQV('New event', 'tier standard · ' + (s === en ? 'day ' + s : 'days ' + s + '–' + en), CTX.event.description);
      var box = document.querySelector('[data-cal-qv-events]'); if (box) box.innerHTML = '';
      fillState(VIEW.month, s); fillLinks(null); showQuickview();
    }
    grid.addEventListener('pointerup', end); grid.addEventListener('pointercancel', end);
  });

  // ============================================================
  // REFINEMENT-V3 — Click-empty-day → mini "Create event" popup
  // ============================================================
  // Flow:
  //   1. Listener observes day-cell clicks where there's no existing
  //      event chip under the cursor.
  //   2. Opens the .cal-almanac-qv--create popup pre-filled with the
  //      clicked date.
  //   3. "Create" button commits a new event to DATA.events (in-memory
  //      mock; the page repaint shows the chip in the cell).
  //   4. "More options ⤢" button escalates to the full editor with the
  //      mini popup's data pre-filled so operator doesn't re-type.
  // ============================================================

  var CREATE_CTX = null; // { month, day, sourceCell }

  function openCreatePopup(m, day, sourceCell) {
    var pop = document.querySelector('[data-cal-create]');
    if (!pop) return;
    CREATE_CTX = { month: m, day: day, sourceCell: sourceCell };
    // Close any other popups to avoid layering noise.
    closeQuickview(); closeEditor(); closeSkyPanel();
    var meta = pop.querySelector('[data-cal-create-meta]');
    if (meta) meta.textContent = monthName(m) + ' ' + day + ', ' + DATA.current_year + ' ' + DATA.calendar.epoch_name;
    var title = pop.querySelector('[data-cal-create-title]'); if (title) title.value = '';
    var notes = pop.querySelector('[data-cal-create-notes]'); if (notes) notes.value = '';
    var tier  = pop.querySelector('[data-cal-create-tier]');  if (tier)  tier.value  = 'standard';
    var cat   = pop.querySelector('[data-cal-create-cat]');   if (cat && DATA.categories && DATA.categories[0]) cat.value = DATA.categories[0].id;
    pop.setAttribute('data-cal-qv-open', 'true');
    pop.setAttribute('aria-hidden', 'false');
    openDialog(pop, closeCreatePopup);
  }
  function closeCreatePopup() {
    var pop = document.querySelector('[data-cal-create]');
    if (!pop) return;
    pop.setAttribute('data-cal-qv-open', 'false');
    pop.setAttribute('data-cal-qv-zoomed', 'false');
    pop.setAttribute('aria-hidden', 'true');
    closeDialog(pop);
  }

  function readCreateForm() {
    if (!CREATE_CTX) return null;
    var pop = document.querySelector('[data-cal-create]'); if (!pop) return null;
    return {
      title: (pop.querySelector('[data-cal-create-title]') || {}).value || '',
      tier:  (pop.querySelector('[data-cal-create-tier]')  || {}).value || 'standard',
      cat:   (pop.querySelector('[data-cal-create-cat]')   || {}).value || '',
      notes: (pop.querySelector('[data-cal-create-notes]') || {}).value || '',
      month: CREATE_CTX.month,
      day:   CREATE_CTX.day
    };
  }

  // Mock-data CreateEvent — appends an event to the in-memory dataset
  // and repaints the affected cell. Mirrors the eventual production
  // service-layer signature so the showcase pattern ports cleanly.
  function mockCreateEvent(form) {
    if (!DATA || !form) return null;
    DATA.events = DATA.events || [];
    var id = 'm-' + Math.random().toString(36).slice(2, 8);
    var ev = {
      id: id,
      name: form.title || 'Untitled event',
      description: form.notes || '',
      month: form.month,
      day: form.day,
      hour: -1,
      tier: form.tier,
      category: form.cat,
      visibility: 'public'
    };
    DATA.events.push(ev);
    repaintCellChips(form.month, form.day);
    return ev;
  }

  function repaintCellChips(m, day) {
    var cell = document.querySelector('[data-cal-cell][data-cell-month="' + m + '"][data-cell-day="' + day + '"]');
    if (!cell) return;
    var chipBox = cell.querySelector('[data-cal-cell-chips]') || cell;
    // Remove only mock-added chips (those with id prefix m-) so we don't
    // accidentally drop the server-rendered chips.
    var existing = chipBox.querySelectorAll('[data-cal-event-id^="m-"]');
    existing.forEach(function (n) { n.parentNode && n.parentNode.removeChild(n); });
    eventsForDay(m, day).forEach(function (e) {
      if (!e || !e.id || e.id.indexOf('m-') !== 0) return; // only render mock-added; server chips already present
      var span = document.createElement('span');
      var cat = categoryById(e.category) || {};
      span.className = 'cal-almanac-chip cal-almanac-chip--' + e.tier;
      span.setAttribute('data-cal-event-id', e.id);
      span.style.setProperty('--chip-cat', cat.color || 'oklch(0.62 0.18 240)');
      span.textContent = e.name;
      chipBox.appendChild(span);
    });
  }

  registerInitBlock('popup-create-flow', function () {
    var pop = document.querySelector('[data-cal-create]');
    if (!pop) return;
    // Listen for empty-cell-bg clicks. Existing event chips have
    // [data-cal-event-id]; clicks on those take the existing
    // popup-slidein path. We hit-test by walking up from the target.
    document.addEventListener('click', function (ev) {
      // Skip clicks inside any popup, editor, time-piece, or sky.
      if (ev.target.closest('[data-cal-qv]') || ev.target.closest('[data-cal-create]') ||
          ev.target.closest('[data-cal-editor]') || ev.target.closest('[data-cal-time]') ||
          ev.target.closest('[data-cal-skypanel]') || ev.target.closest('[data-cal-event-id]') ||
          ev.target.closest('[data-cal-sky]')) return;
      var cell = ev.target.closest('[data-cal-cell]');
      if (!cell) return;
      // If the click landed on a chip child (caught above) we'd have
      // bailed; reaching here means it's an empty-area-of-cell click.
      var m = +cell.getAttribute('data-cell-month'), day = +cell.getAttribute('data-cell-day');
      // Don't preempt the existing popup-slidein day click — only fire
      // create when the cell has zero events for the day.
      var evs = eventsForDay(m, day) || [];
      if (evs.length === 0) { openCreatePopup(m, day, cell); ev.stopPropagation(); }
    }, true);
    var close = pop.querySelector('[data-cal-create-close]');
    if (close) close.addEventListener('click', closeCreatePopup);
    // Escape handled centrally by the 'dialog-a11y' block.
    var commit = pop.querySelector('[data-cal-create-commit]');
    if (commit) commit.addEventListener('click', function () {
      var form = readCreateForm(); if (!form) return;
      mockCreateEvent(form);
      // Cursor-sync (Tuner §J1): tell sibling widgets a new event exists.
      try { if (window.__calCursorSync) window.__calCursorSync.emitEventCreate(form.id || null, { year: VIEW.year, month: form.month, day: form.day }); } catch (e) {}
      flash(commit, 'Created ✓');
      setTimeout(closeCreatePopup, 700);
    });
    var expand = pop.querySelector('[data-cal-create-expand]');
    if (expand) expand.addEventListener('click', function () {
      var form = readCreateForm(); if (!form) return;
      // Build a CTX as if the operator had clicked an existing event,
      // so the editor pre-fills cleanly via hydrateEditor.
      var draft = {
        id: 'new-pending',
        name: form.title || 'New event',
        description: form.notes || '',
        month: form.month, day: form.day, hour: -1,
        tier: form.tier, category: form.cat, visibility: 'public',
        allow_users: [], deny_users: []
      };
      CTX = { kind: 'event', event: draft, month: form.month, day: form.day };
      pop.setAttribute('data-cal-qv-zoomed', 'true');
      expandToEditor();
    });
  });

  // ============================================================
  // Block: month-nav (mock — updates title; full re-render is a port
  // concern). Also re-points VIEW + re-renders the sky for "today".
  // ============================================================
  registerInitBlock('month-nav', function () {
    var titleEl = document.querySelector('.cal-almanac-widget__title');
    var prev = document.querySelector('[data-cal-prev]'), next = document.querySelector('[data-cal-next]'), today = document.querySelector('[data-cal-today]');
    if (!titleEl || !DATA) return;
    var mi = DATA.current_month - 1;
    function paint() { var m = DATA.months[mi]; titleEl.textContent = DATA.calendar.name + ' · ' + m.name + ' ' + DATA.current_year + ' ' + DATA.calendar.epoch_name; }
    if (prev) prev.addEventListener('click', function () { mi = (mi - 1 + DATA.months.length) % DATA.months.length; paint(); });
    if (next) next.addEventListener('click', function () { mi = (mi + 1) % DATA.months.length; paint(); });
    if (today) today.addEventListener('click', function () { mi = DATA.current_month - 1; paint(); VIEW.day = DATA.current_day; renderSkyForDay(VIEW.month, VIEW.day); });
  });

  // ============================================================
  // Block: demo-controls (§E3) — the beta-test harness. Showcase-only;
  // drives era / weather / celestial / time / frame / particle-profile
  // live so the operator can exercise every fix in one place.
  // ============================================================
  // Demo weather/celestial overrides drive the REAL worldState path (so they
  // get the Wave-2 frame renderers + hgSand sync), not a parallel copy.
  function demoSetWeather(v) { setWorldState({ weather: { type: v } }); }
  function demoSetCelestial(v) { setWorldState({ events: (v && v !== 'none') ? [{ type: v, name: v }] : [] }); }
  registerInitBlock('demo-controls', function () {
    var panel = document.querySelector('[data-cal-democtl]');
    if (!panel) return;
    var toggle = panel.querySelector('[data-cal-democtl-toggle]');
    var readout = panel.querySelector('[data-cal-democtl-readout]');
    function say(msg) { if (readout) readout.textContent = msg; }
    if (toggle) toggle.addEventListener('click', function () {
      var open = panel.getAttribute('data-cal-democtl-open') === 'true';
      panel.setAttribute('data-cal-democtl-open', open ? 'false' : 'true');
    });
    function bind(sel, fn) { var el = panel.querySelector(sel); if (el) el.addEventListener('input', function () { fn(el.value); }); }
    var vig = document.querySelector('[data-cal-era-vignette]');
    var sky = document.querySelector('[data-cal-sky]');
    bind('[data-cal-democtl-era]', function (v) {
      var eff = ERA_EFFECTS[v] || ERA_EFFECTS.neutral;
      applyEraParams(vig, eff, sky);
      ERA_HOVER_SPEC = (eff && eff.particleSpec) || null; refeedSky();
      say('era=' + v);
    });
    bind('[data-cal-democtl-era-size]', function (v) { setEraParam('size', v + 'px'); say('era size ' + v + 'px'); });
    bind('[data-cal-democtl-era-hue]', function (v) { setEraParam('hue', v); say('era hue ' + v); });
    bind('[data-cal-democtl-weather]', function (v) { demoSetWeather(v); say('weather=' + v); });
    bind('[data-cal-democtl-celestial]', function (v) { demoSetCelestial(v); say('celestial=' + v); });
    bind('[data-cal-democtl-time]', function (v) { applyTime(Math.max(0, Math.min(1, v / 1000))); say('time ' + (v / 10).toFixed(0) + '%'); });
    bind('[data-cal-democtl-frame]', function (v) { var hg = document.querySelector('[data-cal-time]'); if (hg) hg.setAttribute('data-cal-shelf-frame', v); say('frame=' + v); });
    bind('[data-cal-democtl-profile]', function (v) { if (window.CalParticleEngine) CalParticleEngine.setProfile(v); say('particles=' + v + ' (cap ' + (window.CalParticleEngine ? CalParticleEngine.cap() : '?') + ')'); });
    // V5 sun-state dropdown: lets the operator cycle painted sun states
    // independently from time/celestial. Forces the state attribute; the
    // CSS does the crossfade.
    bind('[data-cal-democtl-sun]', function (v) { applySunState(v); refeedSky(); say('sun=' + v); });

    // WAVE 2 moon library controls. Mutate clones of worldState.moons + push
    // via setWorldState so the 'moons' subscriber repaints (wsEqual needs new
    // objects to detect the change).
    function curMoons() { return (worldState && worldState.moons || []).map(function (m) { return Object.assign({}, m); }); }
    function designPhaseSource(d) { return (MOON_DESIGNS[d] || {}).phaseSource || 'css-clip'; }
    bind('[data-cal-democtl-moon-design]', function (v) {
      var ms = curMoons(); if (!ms.length) return;
      ms[0].baseDesign = v; ms[0].phaseSource = designPhaseSource(v);
      setWorldState({ moons: ms }); say('moon[0] design=' + v);
    });
    bind('[data-cal-democtl-moon-tint]', function (v) {
      var ms = curMoons(); if (!ms.length) return; ms[0].tint = v; setWorldState({ moons: ms }); say('moon[0] tint=' + v);
    });
    function onClick(sel, fn) { var el = panel.querySelector(sel); if (el) el.addEventListener('click', fn); }
    onClick('[data-cal-democtl-moon-randomize]', function () {
      var ms = curMoons(); if (!ms.length) return;
      var keys = Object.keys(MOON_DESIGNS);
      for (var i = keys.length - 1; i > 0; i--) { var j = Math.floor(Math.random() * (i + 1)); var t = keys[i]; keys[i] = keys[j]; keys[j] = t; }
      ms.forEach(function (m, i) { var d = keys[i % keys.length]; m.baseDesign = d; m.phaseSource = designPhaseSource(d); });
      setWorldState({ moons: ms }); say('randomized ' + ms.length + ' moons');
    });
    onClick('[data-cal-democtl-moon-add]', function () {
      var ms = curMoons(); var maxId = 0; ms.forEach(function (m) { if (m.id > maxId) maxId = m.id; });
      var keys = Object.keys(MOON_DESIGNS), d = keys[Math.floor(Math.random() * keys.length)];
      ms.push({ id: maxId + 1, name: 'Moon ' + (maxId + 1), baseDesign: d, phaseSource: designPhaseSource(d),
        tint: null, size: 0.7 + Math.random() * 0.5, orbitSpeed: 0.6 + Math.random() * 0.8, orbitOffset: Math.random(),
        phase: null, namedPhase: null, cyclePct: null, namedPhases: [] });
      setWorldState({ moons: ms }); say('moons=' + ms.length);
    });

    // WAVE 2 mood-tint controls. Preset buttons set {color,intensity}; the
    // custom picker / intensity slider override; clear sets intensity → 0.
    function curMoodIntensity() { var m = worldState && worldState.moodTint; return (m && m.intensity) || 0.4; }
    panel.querySelectorAll('[data-cal-democtl-mood-preset]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var id = btn.getAttribute('data-mood'), p = MOOD_PRESETS[id];
        if (p) { setWorldState({ moodTint: { color: p.color, intensity: p.intensity } }); say('mood=' + id); }
      });
    });
    bind('[data-cal-democtl-mood-color]', function (v) {
      setWorldState({ moodTint: { color: v, intensity: curMoodIntensity() } }); say('mood color=' + v);
    });
    bind('[data-cal-democtl-mood-intensity]', function (v) {
      setWorldState({ moodTint: { intensity: Math.max(0, Math.min(1, v / 100)) } }); say('mood intensity=' + v + '%');
    });
    onClick('[data-cal-democtl-mood-clear]', function () {
      setWorldState({ moodTint: { intensity: 0 } }); say('mood cleared');
    });

    // WAVE 3 time-control verbs (showcase buttons; the mechanics live in
    // TIME_CONTROL / window.__calTimeControl for the future GM panel to reuse).
    panel.querySelectorAll('[data-cal-democtl-tc]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        switch (btn.getAttribute('data-cal-democtl-tc')) {
          case 'hour': tcAdvanceHours(1); say('+1 hour'); break;
          case 'day': tcSnapshot(); tcAdvanceDateBy(1); say('+1 day'); break;
          case 'rest': tcAdvanceHours(8); say('+ long rest (8h)'); break;
          case 'set-dawn': tcSetTime(0.26); say('set dawn'); break;
          case 'set-dusk': tcSetTime(0.74); say('set dusk'); break;
          case 'stepback': say(tcStepBack() ? 'step back' : 'nothing to undo'); break;
          case 'pause': tcTogglePause(); say(worldState.atmospherePaused ? 'atmosphere paused' : 'resumed'); break;
        }
      });
    });
    say('ready · cap ' + (window.CalParticleEngine ? CalParticleEngine.cap() : '?'));
  });

  // WAVE 3: atmosphere-pause hotkey (Space) — ignored while typing in a field.
  registerInitBlock('time-control-hotkey', function () {
    document.addEventListener('keydown', function (ev) {
      if (ev.key !== ' ' && ev.code !== 'Space') return;
      var el = document.activeElement, tag = el && el.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || (el && el.isContentEditable)) return;
      if (!document.querySelector('[data-cal-sky]')) return;   // almanac page only
      ev.preventDefault(); tcTogglePause();
    });
  });

  // ============================================================
  // Block: cursor-sync — cross-widget cursor protocol (Tuner §J1)
  // ============================================================
  // Almanac participates in the cal:cursor-change / cal:event-create
  // DOM-event protocol so a sibling timeline (Tuner) on the same page
  // stays in sync. Standalone (no siblings): emits are harmless no-ops
  // with nothing listening. Loop-prevention is by sourceWidgetId.
  registerInitBlock('cursor-sync', function () {
    var selfId = 'almanac-' + Math.random().toString(36).slice(2, 9);
    var applyingExternal = false;
    // Prime from current world-state so subscribing below doesn't emit a
    // spurious cursor-change at load.
    var lastTod = (window.__calWorldState && typeof window.__calWorldState.timeOfDay === 'number')
      ? window.__calWorldState.timeOfDay : null;
    var sync = { selfId: selfId, type: 'calendar', lastExternal: null, lastEmitted: null };

    function emit(name, detail) {
      detail = detail || {}; detail.sourceWidgetId = selfId; detail.sourceWidgetType = 'calendar';
      sync.lastEmitted = detail;
      try { document.dispatchEvent(new CustomEvent(name, { detail: detail })); } catch (e) {}
    }
    // The create-popup Create button calls this so siblings refresh.
    sync.emitEventCreate = function (eventId, date) { emit('cal:event-create', { eventId: eventId, date: date }); };

    // Emit cal:cursor-change when our time-of-day moves (sun-drag / type /
    // slider all route through setWorldState → the subscriber below),
    // unless the move came from an external sibling.
    if (window.__calSubscribeWorldState) {
      try {
        window.__calSubscribeWorldState(function (ws) {
          if (!ws || typeof ws.timeOfDay !== 'number' || ws.timeOfDay === lastTod) return;
          lastTod = ws.timeOfDay;
          if (applyingExternal) return;
          emit('cal:cursor-change', { date: { year: VIEW.year, month: VIEW.month, day: VIEW.day }, skyTime: ws.timeOfDay });
        });
      } catch (e) {}
    }

    // Listen for a sibling's cursor change → mirror its time-of-day onto
    // the sky band (loop-prevented; external apply suppresses re-emit). The
    // handler is held in cursorSyncHandler so teardownProd() can remove it on
    // a re-init (C-WIDGET-BINDING-QA2) — no duplicate listeners across nav.
    try {
      cursorSyncHandler = function (ev) {
        var d = ev && ev.detail; if (!d || d.sourceWidgetId === selfId) return;
        sync.lastExternal = d;
        if (typeof d.skyTime === 'number' && window.__calSetWorldState) {
          applyingExternal = true;
          try { window.__calSetWorldState({ timeOfDay: d.skyTime }); } catch (e) {}
          applyingExternal = false;
        }
      };
      document.addEventListener('cal:cursor-change', cursorSyncHandler);
    } catch (e) {}

    window.__calCursorSync = sync;
  });

  // ============================================================
  // Trigger
  // ============================================================
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
  // C-WIDGET-BINDING-QA2: re-init after HTMX swaps so the worldstate band
  // animates when it arrives via hx-boost navigation OR a P4b binding swap
  // (the band's <script> can't re-run under allowScriptTags=false). init() is a
  // cheap no-op when there's no new band, so binding to both events is safe.
  try {
    document.addEventListener('htmx:afterSettle', init);
    document.addEventListener('htmx:load', init);
  } catch (e) {}
})();
