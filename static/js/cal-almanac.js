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
      try { b.runner(); r.push({ name: b.name, status: 'OK' }); }
      catch (err) { r.push({ name: b.name, status: 'FAILED', error: (err && err.message) || String(err) });
        try { console.error('[cal-almanac]', b.name, err); } catch (e) {} }
    }
    return r;
  }
  function init() {
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
    var prefersReduced = (function () {
      try { return window.matchMedia('(prefers-reduced-motion: reduce)').matches; } catch (e) { return false; }
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
      var dpr = Math.min(window.devicePixelRatio || 1, 2); // clamp 2×
      s.w = Math.max(1, Math.round(r.width));
      s.h = Math.max(1, Math.round(r.height));
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

    function step(ts) {
      if (!running) return;
      var dt = lastTs ? Math.min(0.05, (ts - lastTs) / 1000) : 0.016;
      lastTs = ts;
      // Frame-time probe → auto low-power.
      if (probeFrames < 30) {
        probeFrames++;
        if (dt * 1000 > 24) probeOverBudget++;
        if (probeFrames === 30 && probeOverBudget > 10 && profile !== 'low') setProfile('low');
      }
      for (var si = 0; si < surfaces.length; si++) {
        var s = surfaces[si];
        if (!s.visible) continue;
        var ctx = s.ctx;
        ctx.clearRect(0, 0, s.w, s.h);
        if (s.clip) { ctx.save(); s.clip(ctx, s.w, s.h); }
        // WAVE 1 frame hook — interior render (heightmap/day-night) UNDER particles.
        if (s.frame) { s.frameT += dt; try { s.frame(ctx, s.w, s.h, dt, s.frameT); } catch (e) { try { console.error('[cal-almanac] frame', e); } catch (e2) {} } }
        // Emit.
        for (var ei = 0; ei < s.emitters.length; ei++) {
          var em = s.emitters[ei], spec = em.spec;
          var alive = 0;
          for (var k = 0; k < s.particles.length; k++) if (s.particles[k].emId === em.id) alive++;
          var rate = spec.spawnRate || 0;
          s.spawnAcc[em.id] = (s.spawnAcc[em.id] || 0) + rate * dt;
          while (s.spawnAcc[em.id] >= 1 && alive < (spec.maxAlive || 20) && liveCount() < globalCap) {
            s.spawnAcc[em.id] -= 1;
            var before = s.particles.length;
            spawn(s, spec);
            if (s.particles.length > before) { s.particles[s.particles.length - 1].emId = em.id; alive++; }
          }
        }
        // Integrate + draw.
        for (var pi = s.particles.length - 1; pi >= 0; pi--) {
          var p = s.particles[pi];
          p.x += p.vx * dt; p.y += p.vy * dt; p.life += dt;
          var off = p.y > s.h + 12 || p.x > s.w + 80 || p.x < -80 || p.life > p.max;
          if (off) { s.particles.splice(pi, 1); toPool(s, p); continue; }
          drawParticle(ctx, p);
        }
        if (s.clip) ctx.restore();
      }
      if (liveCount() > 0 || hasEmitters() || hasFrames()) rafId = requestAnimationFrame(step);
      else { running = false; rafId = null; }
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
      if (reducedNow()) { surfaces.forEach(drawStaticFrame); return; }
      if (!running && (hasEmitters() || hasFrames()) && anyVisible()) { running = true; lastTs = 0; rafId = requestAnimationFrame(step); }
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

    return {
      createSurface: createSurface,
      setProfile: setProfile,
      profile: function () { return profile; },
      cap: function () { return globalCap; },
      live: liveCount,
      reduced: reducedNow,
      sync: sync
    };
  })();
  window.CalParticleEngine = CalParticleEngine;

  // ============================================================
  // Block: data
  // ============================================================
  registerInitBlock('data', function () {
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
    var wType = (function () { var w = dayWeatherTypeID(VIEW.month, VIEW.day); return w ? weatherEffectID(w) : 'clear'; })();
    worldState = {
      timeOfDay: VIEW.timeFrac,                                  // 0..1
      season: seasonName(),                                      // derived label
      date: { year: VIEW.year, month: VIEW.month, day: VIEW.day },
      sun: { tint: null },                                       // CATALOG Part 2 (Wave 2 fills tint)
      moons: (DATA.moons || []).map(function (mn) {              // 0..N dynamic
        return { id: mn.id, phase: null, namedPhase: null, tint: null,
          size: mn.size || 1, orbitSpeed: 1, orbitOffset: mn.phase_offset || 0, cyclePct: null };
      }),
      weather: { type: wType, intensity: 1 },                   // {type,intensity}
      events: celestialFor(VIEW.month, VIEW.day),               // can stack
      moodTint: { color: null, intensity: 0 },                  // player overlay (Wave 2)
      timeControl: { direction: 1, speed: 1 }                   // DM verb layer (Wave 3)
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
    // 1) sky core.
    subscribeWorldState(function (st, changed) {
      if (changed.indexOf('timeOfDay') !== -1) renderTimePipeline(st.timeOfDay);
      if (wsAffectsDay(changed)) renderDayPipeline(st.date.month, st.date.day);
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
    window.__calWeatherEffects = WEATHER_EFFECTS;
  });

  registerInitBlock('celestial-registry', function () {
    // meteor-shower: SLOW per operator — long trailed streaks, low spawn.
    CELESTIAL_EFFECTS['meteor-shower'] = { id: 'meteor-shower', tier: 'must', renderFn: function (box) {
      var wrap = document.createElement('div'); wrap.className = 'cal-almanac-meteors'; wrap.setAttribute('data-cal-meteors', '');
      for (var i = 0; i < 6; i++) {
        var top = (i * 23 + 4) % 40, left = 55 + (i * 17) % 40, delay = i * 1.7;
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
      particleSpec: { shape: 'dot', color: 'oklch(0.92 0.16 75 / 0.8)', sizeRange: [1.5, 3.5], velocity: { x: [-12, 12], y: [-12, 12] }, spawnRate: 1.2, maxAlive: 8, blend: 'lighter' }
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
    return { shape: 'dot', color: color, sizeRange: [1.5, 3.5], velocity: { x: [-14, 14], y: [-14, 14] }, spawnRate: spawnRate, maxAlive: maxAlive, blend: 'lighter' };
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
  var SKY_SURFACE = null; // engine handle for the sky-band canvas
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
    // Weather layer.
    var wlayer = sky.querySelector('[data-cal-sky-weather-layer]');
    if (wlayer) {
      wlayer.innerHTML = '';
      var w = WEATHER_EFFECTS[effID];
      // Only fall back to CSS-DOM particles when the canvas isn't driving
      // them. Thunderstorm's lightning flash is a DOM element either way.
      if (w && (!engineLive || !w.particleSpec)) {
        w.renderFn(wlayer, weatherTypeById(wtypeID) || {});
      } else if (engineLive && effID === 'thunderstorm') {
        var l = spawn('cal-almanac-lightning', wlayer); l.setAttribute('data-cal-lightning', '');
      }
    }
    // Celestial layer.
    var clayer = sky.querySelector('[data-cal-sky-celestial-layer]');
    if (clayer) {
      clayer.innerHTML = '';
      events.forEach(function (c) {
        var fx = CELESTIAL_EFFECTS[c.type];
        // Particle celestials (meteor) go to the canvas; discs/glyphs stay DOM.
        if (fx && (!engineLive || !fx.particleSpec)) fx.renderFn(clayer, c);
      });
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
      var wt = weatherTypeById(wtypeID) || (DATA.weather_effects || []).find(function (e) { return e.id === effID; });
      rest.textContent = ' · ' + skyLabel(VIEW.timeFrac) + ' · ' + seasonName() + ' · ' + (wt ? wt.name : 'Clear');
    }
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
  function feedSkyEngine(effID, events) {
    if (!SKY_SURFACE) return;
    var specs = [];
    var w = WEATHER_EFFECTS[effID];
    if (w && w.particleSpec) specs.push(w.particleSpec);
    (events || []).forEach(function (c) {
      var fx = CELESTIAL_EFFECTS[c.type];
      if (fx && fx.particleSpec) specs.push(fx.particleSpec);
    });
    if (ERA_HOVER_SPEC) specs.push(ERA_HOVER_SPEC);
    // V5: the sun-bloom particles are an `alwaysActive` emitter that runs
    // whenever the sun is visible (every state but TBD-off-cases). The
    // spec is parameterized by current sun state — see sunBloomSpec().
    var sb = sunBloomSpec(currentSunState(events));
    if (sb) specs.push(sb);
    SKY_SURFACE.setEmitters(specs);
  }
  function refeedSky() {
    // Source from worldState so a synthetic weather/event survives a later
    // time-change refeed (normal nav is identical — worldState mirrors DATA).
    var effID = (worldState && worldState.weather && worldState.weather.type) ||
                (function () { var w = dayWeatherTypeID(VIEW.month, VIEW.day); return w ? weatherEffectID(w) : 'clear'; })();
    var events = (worldState && worldState.events) ? worldState.events : celestialFor(VIEW.month, VIEW.day);
    feedSkyEngine(effID, events);
  }

  registerInitBlock('particle-engine', function () {
    var canvas = document.querySelector('[data-cal-sky-canvas]');
    if (!canvas || !window.CalParticleEngine) return;
    SKY_SURFACE = CalParticleEngine.createSurface(canvas, {});
    window.__calSkyEngine = SKY_SURFACE;
    // Keep the canvas backing store sized to the sky-band as it resizes.
    try {
      if ('ResizeObserver' in window) {
        var ro = new ResizeObserver(function () { SKY_SURFACE.resize(); refeedSky(); });
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
  function clockStr(t) {
    var hpd = (DATA && DATA.calendar && DATA.calendar.hours_per_day) || 24;
    var total = Math.floor(t * hpd * 60); return pad2(Math.floor(total / 60)) + ':' + pad2(total % 60);
  }
  // renderTimePipeline — the base time render (sky gradient, sun arc, moons,
  // clocks, snowglobe). Wave 0: invoked by the sky-core subscriber on a
  // timeOfDay change; the public applyTime() shim below routes callers
  // through setWorldState.
  function renderTimePipeline(t) {
    t = Math.max(0, Math.min(0.9999, t));
    VIEW.timeFrac = t;
    var sky = document.querySelector('[data-cal-sky]');
    if (sky) {
      sky.style.background = gradAt(t);
      var sun = sky.querySelector('[data-cal-sky-sun]'); if (sun) place(sun, arcPos(t));
      sky.querySelectorAll('[data-cal-sky-moon]').forEach(function (mn) {
        var id = parseInt(mn.getAttribute('data-moon-id'), 10), off = 0;
        (DATA.moons || []).forEach(function (mm) { if (mm.id === id) off = mm.phase_offset; });
        var mt = t - 0.5 + off; while (mt < 0) mt += 1; while (mt > 1) mt -= 1; place(mn, arcPos(mt));
      });
      var rest = sky.querySelector('[data-cal-sky-sub-rest]');
      if (rest) { var parts = rest.textContent.split(' · '); parts[1] = skyLabel(t); rest.textContent = parts.join(' · '); }
    }
    // Sync clocks (sky time label + snowglobe).
    document.querySelectorAll('[data-cal-sky-time-label], [data-cal-time-clock]').forEach(function (c) { c.textContent = clockStr(t); });
    // Sync the snowglobe sun/moons.
    var gsun = document.querySelector('[data-cal-globe-sun]'); if (gsun) place(gsun, arcPos(t));
    document.querySelectorAll('.cal-almanac-globe__moon').forEach(function (mn, idx) {
      var mm = (DATA.moons || [])[idx]; var off = mm ? mm.phase_offset : 0;
      var mt = t - 0.5 + off; while (mt < 0) mt += 1; while (mt > 1) mt -= 1; place(mn, arcPos(mt));
    });
  }
  // Public time-change entry point → unified world-state (Wave 0 shim).
  // Preserves every caller (drag-scrub, time-input, hourglass tick,
  // demo-controls slider); the subscribers re-render both surfaces.
  function applyTime(t) {
    setWorldState({ timeOfDay: Math.max(0, Math.min(0.9999, t)) });
  }
  registerInitBlock('sun-drag-scrub', function () {
    var sun = document.querySelector('[data-cal-sky-sun]');
    var sky = document.querySelector('[data-cal-sky]');
    if (!sun || !sky) return;
    var dragging = false;
    sun.addEventListener('pointerdown', function (ev) {
      dragging = true;
      ev.stopPropagation(); // don't start the widget-shell drag
      try { sun.setPointerCapture(ev.pointerId); } catch (e) {}
      ev.preventDefault();
    });
    sun.addEventListener('pointermove', function (ev) {
      if (!dragging) return;
      var r = sky.getBoundingClientRect();
      var frac = (ev.clientX - r.left) / r.width;
      // Map x 8%..92% → time 0.25..0.75 (the visible arc), clamp outside to night.
      var t = 0.25 + ((frac - 0.08) / 0.84) * 0.5;
      applyTime(t);
    });
    function end(ev) { if (!dragging) return; dragging = false; try { sun.releasePointerCapture(ev.pointerId); } catch (e) {} }
    sun.addEventListener('pointerup', end);
    sun.addEventListener('pointercancel', end);
  });

  // ============================================================
  // Block: time-label-input — click time → type a specific time
  // ============================================================
  registerInitBlock('time-label-input', function () {
    var label = document.querySelector('[data-cal-sky-time-label]');
    if (!label) return;
    label.addEventListener('click', function () {
      var input = document.createElement('input');
      input.className = 'cal-almanac-sky__time-input';
      input.value = label.textContent;
      label.replaceWith(input);
      input.focus(); input.select();
      function commit(save) {
        if (save) {
          var hpd = (DATA && DATA.calendar && DATA.calendar.hours_per_day) || 24;
          var t = parseTime(input.value, hpd);
          if (t != null) applyTime(t);
        }
        input.replaceWith(label);
        label.textContent = clockStr(VIEW.timeFrac);
      }
      input.addEventListener('keydown', function (ev) {
        if (ev.key === 'Enter') commit(true);
        if (ev.key === 'Escape') commit(false);
      });
      input.addEventListener('blur', function () { commit(true); });
    });
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
        var ro = new ResizeObserver(function () { applyEraParams(vig, eraEffectFor(currentEraObj()), sky); });
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
  // Compose helper — used when celestial + weather both active.
  function composeSand(box, prefer) {
    if (prefer === 'celestial' && WEATHER_EFFECTS && CELESTIAL_EFFECTS) {
      var cs = (window.__currentCelestialIDs || []);
      for (var i = 0; i < cs.length; i++) {
        var fx = CELESTIAL_EFFECTS[cs[i]];
        if (fx && fx.sandRender) { fx.sandRender(box); return true; }
      }
    }
    return false;
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
    var hg = document.querySelector('[data-cal-time]');
    var stream = 'oklch(0.86 0.16 80)';
    if (hg) {
      var v = getComputedStyle(hg).getPropertyValue('--sand-stream');
      if (v && v.trim()) stream = v.trim();
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
    var sandColor = 'oklch(0.86 0.16 80)';
    function setSandColor(c) { if (c && String(c).trim()) sandColor = String(c).trim(); }
    function reset() { for (var i = 0; i < BCOLS; i++) bh[i] = 0; stream.length = 0; }
    function stepSim(dt) {
      var f = Math.min(3, dt / 0.016);            // normalize to ~60fps sim rate
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
      var sky = hgSkyForTimeOfDay(tod), y0 = V_CEIL * sy, y1 = V_FLOOR * sy;
      var g = ctx.createLinearGradient(0, y0, 0, y1); g.addColorStop(0, sky[0]); g.addColorStop(1, sky[1]);
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
      // pile (heightmap) — drawn AFTER the sun so the sun sinks behind it
      ctx.beginPath(); ctx.moveTo(V_BX0 * sx, V_FLOOR * sy);
      for (var c = 0; c < BCOLS; c++) ctx.lineTo((V_BX0 + c * bcwV + bcwV / 2) * sx, (V_FLOOR - bh[c]) * sy);
      ctx.lineTo((V_BX0 + V_BW) * sx, V_FLOOR * sy); ctx.closePath();
      ctx.fillStyle = sandColor; ctx.fill();
      ctx.globalAlpha = 0.95;
      for (var k = 0; k < stream.length; k++) { var s2 = stream[k]; ctx.beginPath(); ctx.arc(s2.x * sx, s2.y * sy, s2.r * sx, 0, 6.283); ctx.fill(); }
      ctx.globalAlpha = 1;
    }
    // Engine frame hook: step the sim when time advances (dt>0); always draw
    // (dt=0 = the reduced-motion static frame).
    function frame(ctx, w, h, dt) { if (dt > 0) { _t += dt; stepSim(dt); } draw(ctx, w, h); }
    return { frame: frame, setSandColor: setSandColor, reset: reset };
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
        var ro = new ResizeObserver(function () { GLASS_SURFACE.resize(); feedHourglassStream(); });
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
    document.addEventListener('keydown', function (ev) { if (ev.key === 'Escape') { closeEditor(); closeQuickview(); closeSkyPanel(); } });
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
  function showQuickview() { var qv = document.querySelector('[data-cal-qv]'); if (qv) { qv.setAttribute('data-cal-qv-open', 'true'); qv.setAttribute('data-cal-qv-zoomed', 'false'); qv.setAttribute('aria-hidden', 'false'); } closeSkyPanel(); }
  function closeQuickview() { var qv = document.querySelector('[data-cal-qv]'); if (qv) { qv.setAttribute('data-cal-qv-open', 'false'); qv.setAttribute('aria-hidden', 'true'); } }
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
  }
  function closeEditor() {
    var ed = document.querySelector('[data-cal-editor]'); var qv = document.querySelector('[data-cal-qv]');
    if (ed) { ed.setAttribute('data-cal-editor-open', 'false'); ed.setAttribute('aria-hidden', 'true'); }
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
      var name = window.prompt('Add ' + b.getAttribute('data-cal-vis-add') + ' rule for which user?'); if (!name) return;
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
  }
  function closeSkyPanel() { var p = document.querySelector('[data-cal-skypanel]'); if (p) { p.setAttribute('data-cal-skypanel-open', 'false'); p.setAttribute('aria-hidden', 'true'); } }
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
    setTimeout(function () { if (title) title.focus(); }, 30);
  }
  function closeCreatePopup() {
    var pop = document.querySelector('[data-cal-create]');
    if (!pop) return;
    pop.setAttribute('data-cal-qv-open', 'false');
    pop.setAttribute('data-cal-qv-zoomed', 'false');
    pop.setAttribute('aria-hidden', 'true');
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
    document.addEventListener('keydown', function (e) { if (e.key === 'Escape') closeCreatePopup(); });
    var commit = pop.querySelector('[data-cal-create-commit]');
    if (commit) commit.addEventListener('click', function () {
      var form = readCreateForm(); if (!form) return;
      mockCreateEvent(form);
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
  // A demo weather/celestial override that the sky render path consults.
  var DEMO = { weather: null, celestial: null };
  function demoApplySky() {
    var sky = document.querySelector('[data-cal-sky]');
    if (!sky || !SKY_SURFACE) return;
    var effID = DEMO.weather || (function () { var w = dayWeatherTypeID(VIEW.month, VIEW.day); return w ? weatherEffectID(w) : 'clear'; })();
    sky.setAttribute('data-cal-sky-weather', effID);
    sky.className = sky.className.replace(/cal-almanac-sky--wfx-\S+/g, '').trim() + ' cal-almanac-sky--wfx-' + effID;
    var specs = [];
    var w = WEATHER_EFFECTS[effID]; if (w && w.particleSpec) specs.push(w.particleSpec);
    var events = DEMO.celestial && DEMO.celestial !== 'none' ? [{ type: DEMO.celestial, name: DEMO.celestial }] : celestialFor(VIEW.month, VIEW.day);
    (events || []).forEach(function (c) { var fx = CELESTIAL_EFFECTS[c.type]; if (fx && fx.particleSpec) specs.push(fx.particleSpec); });
    if (ERA_HOVER_SPEC) specs.push(ERA_HOVER_SPEC);
    SKY_SURFACE.setEmitters(specs);
    // Drive the eclipse disc / meteor DOM + the hourglass sand theme too.
    var clayer = sky.querySelector('[data-cal-sky-celestial-layer]');
    if (clayer) { clayer.innerHTML = ''; (events || []).forEach(function (c) { var fx = CELESTIAL_EFFECTS[c.type]; if (fx && !fx.particleSpec) fx.renderFn(clayer, c); }); }
    var hg = document.querySelector('[data-cal-time]');
    if (hg) {
      var themeId = (DEMO.celestial && DEMO.celestial !== 'none') ? DEMO.celestial : effID;
      hg.setAttribute('data-cal-hourglass-theme', themeId);
      feedHourglassStream();
    }
  }
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
    bind('[data-cal-democtl-weather]', function (v) { DEMO.weather = v; demoApplySky(); say('weather=' + v); });
    bind('[data-cal-democtl-celestial]', function (v) { DEMO.celestial = v; demoApplySky(); say('celestial=' + v); });
    bind('[data-cal-democtl-time]', function (v) { applyTime(Math.max(0, Math.min(1, v / 1000))); say('time ' + (v / 10).toFixed(0) + '%'); });
    bind('[data-cal-democtl-frame]', function (v) { var hg = document.querySelector('[data-cal-time]'); if (hg) hg.setAttribute('data-cal-shelf-frame', v); say('frame=' + v); });
    bind('[data-cal-democtl-profile]', function (v) { if (window.CalParticleEngine) CalParticleEngine.setProfile(v); say('particles=' + v + ' (cap ' + (window.CalParticleEngine ? CalParticleEngine.cap() : '?') + ')'); });
    // V5 sun-state dropdown: lets the operator cycle painted sun states
    // independently from time/celestial. Forces the state attribute; the
    // CSS does the crossfade.
    bind('[data-cal-democtl-sun]', function (v) { applySunState(v); refeedSky(); say('sun=' + v); });
    say('ready · cap ' + (window.CalParticleEngine ? CalParticleEngine.cap() : '?'));
  });

  // ============================================================
  // Trigger
  // ============================================================
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
})();
