/*
 * sky_pane.js — the sky pane widget (issue #763). Chronicle.register('sky-pane', ...).
 *
 * Fetches the campaign's default calendar (already resolved server-side —
 * see the "skybox" block in internal/app/routes.go, which passes the
 * calendar id straight into the mount so this widget never has to fetch
 * /calendars/list just to find it) plus its events, then draws the current
 * moment's sky with the painted sky (sky_gl.js), or the basic sky (sky_2d.js)
 * where that can't run, using SkyWorld's astronomy/palette
 * (sky_world.js), SkyLooks' weather-to-look mapping (sky_looks.js), SkyMoon's
 * body specs (sky_moon.js) and SkyEvents' payload-driven overlays
 * (sky_events.js).
 *
 * Motion (issue #760): the pane is a real button-controlled disclosure. Its
 * box-model height changes AT ONCE (no animated reflow — the rule against
 * animating layout properties is followed by never animating one); the
 * reveal itself animates only `clip-path` on the (already full-size) sky
 * element, like a shade rolling down. `prefers-reduced-motion` — and
 * Chronicle's own site-wide total-disable guard (static/css/input.css),
 * which is STRICTER than the design contract's suggested crossfade and is
 * followed here instead, since it's the established house convention next
 * to which a per-widget crossfade would be inconsistent and would in any
 * case be clamped to ~0ms by that guard's `transition-duration: .01ms
 * !important` — collapses the reveal to instant and the render loop to a
 * single still frame (see PACE below): no continuous rAF loop runs for a
 * reduced-motion viewer.
 */
(function () {
  'use strict';

  var SW = window.SkyWorld, LOOKS = window.SkyLooks, EV = window.SkyEvents;

  // The sky's time is the page's rest clock (motion_rest.js): it slows to a
  // standstill when the viewer steps away and resumes where it stopped.
  function clock() { return window.MotionRest ? window.MotionRest.now() : performance.now() / 1000; }
  function resting() { return !!(window.MotionRest && window.MotionRest.still()); }

  // ── One shared render loop for every sky-pane instance on the page, so N
  // panes cost one rAF, not N. An instance only receives ticks while it opts
  // in (continuousInstances) — see PACE below. ──
  var LOOP = (function () {
    var insts = [], running = false;
    function tick() {
      if (!document.hidden) {
        var t = clock();
        insts.slice().forEach(function (inst) {
          // One failing sky must not stop the loop for the others.
          try { if (inst.onScreen !== false) inst.render(t, true); }
          catch (e) { LOOP.remove(inst); if (window.console) console.error('[sky-pane] render failed', e); }
        });
      }
      // At rest the last frame stays up and no frames are asked for until the viewer is back.
      if (insts.length && !resting()) requestAnimationFrame(tick);
      else running = false;
    }
    function start() { if (!running && insts.length) { running = true; requestAnimationFrame(tick); } }
    if (window.MotionRest) window.MotionRest.onWake(start);
    return {
      add: function (inst) { if (insts.indexOf(inst) < 0) insts.push(inst); start(); },
      remove: function (inst) { var i = insts.indexOf(inst); if (i >= 0) insts.splice(i, 1); }
    };
  })();

  // A sky where only clouds drift and stars twinkle is painted 12 times a second.
  var IDLE_S = 1 / 12;

  function reducedMotion() {
    // Either reduce switch counts too: the campaign's (data-cz-reduce) and the
    // person's own Calmer choice (data-view-motion), so the sky holds still.
    try {
      var root = document.documentElement;
      return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) ||
        root.hasAttribute('data-cz-reduce') || root.getAttribute('data-view-motion') === 'calm';
    }
    catch (e) { return false; }
  }

  function injectStyles() {
    if (document.getElementById('sky-pane-styles')) return;
    var style = document.createElement('style');
    style.id = 'sky-pane-styles';
    style.textContent = SKY_PANE_CSS;
    document.head.appendChild(style);
  }

  // Phase name thresholds mirror Moon.MoonPhaseName exactly (Go,
  // internal/plugins/calendar/model.go) so the pane's caption and the
  // server's own phase name never disagree.
  function phaseName(p) {
    if (p < .0625 || p >= .9375) return 'New Moon';
    if (p < .1875) return 'Waxing Crescent';
    if (p < .3125) return 'First Quarter';
    if (p < .4375) return 'Waxing Gibbous';
    if (p < .5625) return 'Full Moon';
    if (p < .6875) return 'Waning Gibbous';
    if (p < .8125) return 'Last Quarter';
    return 'Waning Crescent';
  }
  function hhmm(hour, minute) {
    var h = Math.floor(hour), m = Math.round(minute || 0);
    var ap = h >= 12 ? 'PM' : 'AM', h12 = h % 12; if (h12 === 0) h12 = 12;
    return h12 + ':' + (m < 10 ? '0' : '') + m + ' ' + ap;
  }
  function getJSON(url) {
    return (window.Chronicle && Chronicle.apiFetch ? Chronicle.apiFetch(url) : fetch(url, { credentials: 'same-origin', headers: { Accept: 'application/json' } }))
      .then(function (r) { if (!r.ok) throw new Error('http ' + r.status); return r.json(); });
  }
  // The shown day's own weather reading, or null. A day's reading wins over
  // the calendar's current weather; a server without day weather, or any
  // failure, just leaves the current weather in charge.
  function fetchDayWeather(cid, calid, cal) {
    var y = cal.current_year, m = cal.current_month, d = cal.current_day;
    return getJSON('/campaigns/' + encodeURIComponent(cid) + '/calendars/' + encodeURIComponent(calid) + '/weather/days?year=' + y + '&month=' + m)
      .then(function (days) {
        days = Array.isArray(days) ? days : (days && days.data) || [];
        for (var i = 0; i < days.length; i++) if (days[i] && days[i].year === y && days[i].month === m && days[i].day === d) return days[i];
        return null;
      }, function () { return null; });
  }
  function hashSeed(s) { s = String(s || ''); var v = 0; for (var i = 0; i < s.length; i++) v = (v * 131 + s.charCodeAt(i)) >>> 0; return v || 1; }

  // buildState(model, tSeconds, dt): the per-frame render state both
  // painters consume. Mirrors the design contract's Surface.prototype.state.
  // dt (seconds since the last frame) lets a weather change roll in instead
  // of snapping: each dial with an ease glides at its own pace.
  var GLIDE = LOOKS.DIALS.filter(function (d) { return d.ease; });
  // Things with a place in the sky arrive and leave by their kind instead of
  // fading where they stand: 'edge' moves in from beyond the right edge and
  // leaves the same way, 'open' spreads from where it is. Meteors and
  // lightning are the third kind, 'once': they play and need nothing here.
  var BODIES = { funnel: { enter: 'edge', s: 8 }, blood: { enter: 'open', s: 6 } };
  // A body's progress toward being fully here (1) or gone (0).
  function arrive(b, want, live, dt, s) {
    b.v = !live ? want : want > b.v ? Math.min(want, b.v + dt / s) : Math.max(want, b.v - dt / s);
    return b.v > 0 && b.v < 1;
  }
  function ease(v) { return v * v * (3 - 2 * v); }
  function buildState(model, tSeconds, dt) {
    var cal = model.calendar;
    var year = cal.current_year, month = cal.current_month, day = cal.current_day;
    var h24 = SW.hour24(cal, cal.current_hour || 0, cal.current_minute || 0);
    var skym = model.skym;
    var sun = skym.sun(year, month, day, h24);
    var tCont = SW.dayIndex(cal, year, month, day) + h24 / 24;

    // Astronomy first (independent of the view's facing), so the day's
    // events can point the camera before the projection is built.
    //
    // No client-side hidden_from_players filter here: the server already
    // strips hidden moons for a Player viewer (see filterMoonsForViewer in
    // the calendar plugin), so re-filtering here is redundant for a Player
    // and actively wrong for a GM — it would hide the GM's own hidden moon
    // from the GM. The server decides what a viewer receives; this widget
    // trusts the response as-is (CLAUDE.md).
    var astro = (cal.moons || []).map(function (mo) {
      var m = skym.moon(mo, tCont, year, month, day, h24);
      return { mo: mo, m: m, up: m.alt > -6 * SW.D2R };
    });
    var dayEvents = EV.parseDayEvents(model.todayEvents);
    var cam = EV.camFor(dayEvents, astro);
    var overlay = EV.overlaysFor(dayEvents, astro);

    var L = { W: model.width, H: model.height, hor: model.height * .82, top: model.height * .07, az0: cam };
    var sp = SW.PROJ.at(L, sun.alt, sun.az);
    var altDeg = sun.alt / SW.D2R, eve = h24 >= 12 ? 1 : 0;
    var dark = SW.smooth01(4, -14, altDeg);
    var R = SW.clamp(model.height * .11, 7, 26);
    // A folding pane draws at every height it passes through, but shades its
    // moons once, at the height it rests at (restH), and scales them.
    var RS = model.restH ? SW.clamp(model.restH * .11, 7, 26) : R;

    var moons = astro.map(function (a, i) {
      var mo = a.mo, up = a.up, r = R * (mo.size || 1), sr = RS * (mo.size || 1);
      var blood = up && !!overlay.blood[mo.id];
      var harvest = up && !!overlay.harvest[mo.id];
      if (harvest) { r *= 1.18; sr *= 1.18; }
      var shadow = null;
      if (up && overlay.eclipse[mo.id] != null) shadow = [0, 0, r * .55, overlay.eclipse[mo.id]];
      var p = up ? PROJ_at(L, a.m.alt, a.m.az) : { x: 0, y: 0 };
      var rot = up ? SW.moonTurn(L, a.m, sun) : 0;
      // i: the moon's slot in the painter's atlas; past its four slots a
      // moon is drawn in the painted sky's drawn layer instead.
      return { i: i < 4 ? i : -1, extra: i >= 4, mo: mo, m: a.m, up: up, x: p.x, y: p.y, r: r, sr: sr, rot: rot, spec: window.SkyMoon.specFor(mo), blood: blood, shadow: shadow };
    });

    var weatherSpec = LOOKS.spec(model.dayWeather || cal.weather);
    var look = LOOKS.resolve(weatherSpec);
    // A blood moon brings its own weather over whatever the day's already is:
    // a dark sky with no rain, because the sky itself turns blood red and bleeds
    // (the painted sky and the drawn layer's bleeding).
    if (moons.some(function (M) { return M.blood; })) {
      look = LOOKS.blend(look || LOOKS.blank(), LOOKS.resolve(LOOKS.spec({ preset_id: 'blood-rain', preset_label: 'Blood rain' })), .7);
      look.dark = Math.max(look.dark, .6); look.stars = Math.min(look.stars, .25);
      look.rain = 0; look.cloud = .45; look.sky = ['#7a0612', .95]; look.light *= .7; look.rainTint = null;
    }
    // A change of weather rolls in: the look's layers and its unpaced dials fade across over FADE_S, and each paced
    // dial heads for the new weather itself at its own pace. A first frame, a jump in time or reduced motion shows the
    // new weather at once; a repaint with no time passed holds where it is. g is kept across frames, and across days
    // by the dock.
    var goal = look || LOOKS.blank(), g = model.glide || (model.glide = {}), live = !model.reduced && dt >= 0 && dt < 1, gliding = false;
    var key = JSON.stringify(model.dayWeather || cal.weather || null) + (moons.some(function (M) { return M.blood; }) ? ':blood' : '');
    if (g.key !== key) { g.from = live && g.shown ? g.shown : null; g.k = 0; g.key = key; }
    if (g.from && live) {
      g.k = Math.min(1, g.k + dt / LOOKS.FADE_S);
      look = LOOKS.blend(g.from, goal, g.k * g.k * (3 - 2 * g.k)); gliding = true;
      if (g.k >= 1) { g.from = null; look = goal; }
    } else g.from = null;
    g.shown = look || goal;
    var wx = g.wx;
    if (!wx || !live) { wx = g.wx = {}; GLIDE.forEach(function (d) { wx[d.n] = goal[d.n]; }); }
    else GLIDE.forEach(function (d) { wx[d.n] += (goal[d.n] - wx[d.n]) * (1 - Math.exp(-dt / d.ease)); if (Math.abs(goal[d.n] - wx[d.n]) > .005) gliding = true; });
    var Bd = g.bodies || (g.bodies = { funnel: { v: 0, spec: null }, blood: { v: 0 } }), fun = Bd.funnel;
    if (goal.funnel) fun.spec = goal.funnel;
    if (arrive(fun, goal.funnel ? 1 : 0, live, dt, BODIES.funnel.s)) gliding = true;
    // The funnel keeps its full strength while it moves; only the fade of step 2 is replaced.
    if (fun.v > 0 || (look && look.funnel)) look = Object.assign({}, look || goal, { funnel: fun.v > 0 && fun.spec ? fun.spec : null });
    var P = SW.paletteFor(altDeg, eve, look, wx, dark, false);
    if (overlay.magic) SW.tintPalette(P, '#8a5fe0', .3, ['zen', 'mid', 'hor', 'anti', 'clit', 'cshade', 'h0', 'h1', 'h2', 'h3']);

    var st = {
      P: P, L: L, W: model.width, H: model.height, wx: wx, t: tSeconds, look: look, rm: !!model.reduced, almanac: false,
      sun: sun, sp: sp, alt: sun.alt, altEff: altDeg, dark: dark, R: R,
      moons: moons, cam: cam, sidereal: SW.mod((sun.H + sun.lon) / SW.D2R, 360),
      landSeed: model.landSeed, overlay: overlay, fxItems: [],
      weatherLabel: weatherSpec.label || 'Clear skies',
      // How fast the sky is moving: below 1 while it comes to rest, when lightning fades rather than freezing mid-flash.
      motion: window.MotionRest ? window.MotionRest.speed() : 1
    };
    // What the painted sky adds: the scene's light, then the day's events
    // and the look's moving things (meteors, aurora, the bleeding moon).
    st.sl = window.SkyFX.sceneLight(st, P);
    EV.apply(st, dayEvents);
    // How far the funnel still is beyond the right edge, as a share of the width.
    st.funnelOff = (1 - ease(fun.v)) * .62;
    // A blood moon's stain and drips spread from the moon; when the event ends they go with the look's fade.
    if (!st.bleed) Bd.blood.v = 0;
    else {
      if (arrive(Bd.blood, 1, live, dt, BODIES.blood.s)) gliding = true;
      st.bleed = Object.assign({}, st.bleed, { k: st.bleed.k * ease(Bd.blood.v) });
    }
    st.pace = motionOf(st);
    // A change rolling in is painted at the full rate, so it never steps.
    st.gliding = gliding;
    if (gliding && st.pace) st.pace = 2;
    return st;
  }
  // motionOf(st): how much the painted sky moves — 2 when something visibly
  // moves (particles, rain and snow, meteors, a storm, aurora, a bleeding
  // moon), 1 when only clouds drift and stars twinkle, 0 under reduced
  // motion.
  function motionOf(st) {
    if (st.rm) return 0;
    var lk = st.look, moving = !!(st.met.length || st.aur.length || st.bleed || st.fxItems.length || st.airs.length ||
      st.moons.some(function (M) { return M.conj && M.up; }) ||
      (lk && (lk.parts.length || lk.rain + lk.snow + lk.hail > .04 || lk.storm > .1 || lk.air.length || lk.fire.length || lk.sigils.length || lk.funnel)));
    return moving ? 2 : 1;
  }
  function PROJ_at(L, alt, az) { return SW.PROJ.at(L, alt, az); }

  // paceOf(state): how often the sky must be painted. The painted sky
  // moves as motionOf says; the basic sky moves only for a blood moon or
  // while a change of weather rolls in.
  function paceOf(state) {
    if (window.SkyGL && window.SkyGL.available()) return state.pace;
    return !state.rm && (state.gliding || Object.keys(state.overlay.blood || {}).length > 0) ? 2 : 0;
  }
  // paintSky: the painted sky when it can be had, the basic sky otherwise.
  // hold keeps that sky's own drawn-layer canvases between frames.
  function paintSky(ctx, hold, st, dpr, oy, moving) {
    if (window.SkyGL && window.SkyGL.paint(ctx, hold, st, dpr, oy, moving)) return 'gl';
    window.Sky2D.draw(ctx, st, dpr, oy);
    return '2d';
  }

  // The moon that lights the sky most: the fullest, weighted by its size.
  function brightestMoon(st) {
    return st.moons.filter(function (m) { return m.up && m.m.alt > 0; })
      .sort(function (a, b) { return b.m.lit * (b.mo.size || 1) - a.m.lit * (a.mo.size || 1); })[0] || null;
  }

  // The sky in words: weather, the brightest moon and its phase, the time.
  function describe(model, st) {
    var cal = model.calendar, bits = [st.weatherLabel], b = brightestMoon(st);
    if (b) bits.push(b.mo.name + ' – ' + phaseName(b.m.p));
    bits.push(hhmm(cal.current_hour || 0, cal.current_minute || 0));
    return bits.join(' · ');
  }

  function Instance(el) {
    this.el = el;
    this.chip = el.querySelector('.skychip');
    this.chipSw = el.querySelector('.skychip .sw');
    this.chipText = el.querySelector('.skychip-text');
    this.wrap = el.querySelector('.skywrap');
    this.sky = el.querySelector('.sky');
    this.canvas = el.querySelector('.sky canvas');
    this.caption = el.querySelector('.skycap');
    this.ctx = this.canvas.getContext('2d');
    this.model = null;
    this.open = false;
    this.onScreen = true;
    this.dpr = Math.max(1, Math.min(2, window.devicePixelRatio || 1));
    this.reduced = reducedMotion();
    this._boundKeydown = this.onKeydown.bind(this);
    this._boundClick = this.toggle.bind(this);
    this._boundResize = this.onResize.bind(this);
    this._refreshTimer = null;
  }
  Instance.prototype.init = function () {
    var self = this;
    this.chip.addEventListener('click', this._boundClick);
    this.el.addEventListener('keydown', this._boundKeydown);
    window.addEventListener('resize', this._boundResize);
    if ('IntersectionObserver' in window) {
      this.io = new IntersectionObserver(function (es) { es.forEach(function (e) { self.onScreen = e.isIntersecting; }); }, { threshold: 0 });
      this.io.observe(this.sky);
    }
    // The painted sky becoming ready, or giving way to the basic one, repaints.
    this._glChange = function () { if (self.open) self.renderNow(); };
    if (window.SkyGL) window.SkyGL.onChange(this._glChange);
    this.load();
    // A slow drift check even while closed, so the chip swatch and caption
    // never go stale across a long-open page (the world clock advances).
    this._refreshTimer = window.setInterval(function () { self.refreshPalette(); }, 60000);
  };
  Instance.prototype.destroy = function () {
    this.chip.removeEventListener('click', this._boundClick);
    this.el.removeEventListener('keydown', this._boundKeydown);
    if (window.SkyGL) window.SkyGL.offChange(this._glChange);
    window.removeEventListener('resize', this._boundResize);
    if (this.io) this.io.disconnect();
    if (this._refreshTimer) window.clearInterval(this._refreshTimer);
    LOOP.remove(this);
  };
  Instance.prototype.load = function () {
    var self = this, cid = this.el.getAttribute('data-campaign-id'), calid = this.el.getAttribute('data-calendar-id');
    var fetchJSON = getJSON;
    fetchJSON('/campaigns/' + cid + '/calendars/' + calid).then(function (cal) {
      var y = cal.current_year, m = cal.current_month;
      return Promise.all([
        fetchJSON('/campaigns/' + cid + '/calendars/' + calid + '/events?year=' + y + '&month=' + m),
        fetchDayWeather(cid, calid, cal)
      ]).then(function (got) {
        var events = got[0];
        var todayEvents = (events || []).filter(function (e) { return e.year === cal.current_year && e.month === cal.current_month && e.day === cal.current_day; });
        self.model = { calendar: cal, todayEvents: todayEvents, dayWeather: got[1], skym: SW.makeSkym(cal), landSeed: hashSeed(calid), width: 0, height: 0 };
        self.onDataReady();
      });
    }).catch(function (err) {
      self.caption.textContent = 'The sky could not be reached.';
      if (window.console) console.error('[sky-pane] load failed', err);
    });
  };
  Instance.prototype.onDataReady = function () {
    this.refreshPalette();
    if (this.open) this.renderNow();
  };
  // refreshPalette: cheap — no canvas paint — just recomputes the chip's
  // two-colour swatch and its caption text, so a closed pane still reflects
  // "the sky now" (the mockup's own promise for the chip) without running
  // the render loop while nothing is visible.
  Instance.prototype.refreshPalette = function () {
    if (!this.model) return;
    // A copy without the glide, so a refresh never cuts short a change that is rolling in.
    var m = this.model, st = buildState({ calendar: m.calendar, todayEvents: m.todayEvents, dayWeather: m.dayWeather, skym: m.skym, landSeed: m.landSeed, width: m.width, height: m.height, reduced: m.reduced }, 0);
    var hexOf = function (lin) { return LOOKS.linHex(lin); };
    this.chipSw.style.background = 'linear-gradient(' + hexOf(st.P.zen) + ',' + hexOf(st.P.hor) + ')';
    var cal = this.model.calendar;
    this.chipText.textContent = hhmm(cal.current_hour || 0, cal.current_minute || 0);
    this.caption.textContent = describe(this.model, st);
  };
  Instance.prototype.onResize = function () {
    clearTimeout(this._resizeT);
    var self = this;
    this._resizeT = setTimeout(function () {
      if (!self.open) return;
      self.resizeCanvas();
      // Setting canvas.width/height (inside resizeCanvas, on an actual size
      // change) clears the canvas per the HTML canvas spec. Nothing redraws
      // it afterward unless the shared render loop happens to be running —
      // and it only runs while a blood-moon-style overlay effect is active
      // (see paceOf) — so a resize/rotation would otherwise leave the sky
      // blank until the next such effect. Force one paint here instead.
      self.render(self.reduced ? 0 : clock());
    }, 100);
  };
  Instance.prototype.resizeCanvas = function () {
    var w = this.sky.clientWidth, h = this.sky.clientHeight;
    if (!w || !h) return;
    var W = Math.max(1, Math.round(w * this.dpr)), H = Math.max(1, Math.round(h * this.dpr));
    if (this.canvas.width !== W || this.canvas.height !== H) { this.canvas.width = W; this.canvas.height = H; }
    if (this.model) { this.model.width = w; this.model.height = h; }
  };
  // render(t, looping): one frame. In the loop, a sky that only drifts is
  // painted at a low idle rate rather than every frame.
  Instance.prototype.render = function (tSeconds, looping) {
    if (!this.model || !this.open) return;
    if (!this.model.width) this.resizeCanvas();
    if (!this.model.width) return;
    if (looping && this.pace === 1 && tSeconds - (this.lastPaint || 0) < IDLE_S) return;
    var dt = this.lastPaint ? tSeconds - this.lastPaint : 0;
    this.lastPaint = tSeconds;
    this.model.reduced = this.reduced;
    var st = buildState(this.model, tSeconds, dt);
    this.ctx.setTransform(1, 0, 0, 1, 0, 0);
    this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
    paintSky(this.ctx, this, st, this.dpr, 0, !!looping);
    var pace = this.reduced ? 0 : paceOf(st);
    if (pace !== this.pace) { this.pace = pace; if (pace) LOOP.add(this); else LOOP.remove(this); }
  };
  Instance.prototype.renderNow = function () {
    this.resizeCanvas();
    this.lastPaint = 0;
    this.render(this.reduced ? 0 : clock());
  };
  Instance.prototype.toggle = function () {
    if (this.open) this.close(); else this.openPane();
  };
  Instance.prototype.openPane = function () {
    if (this.open) return;
    this.open = true;
    this.chip.setAttribute('aria-expanded', 'true');
    // The room changes at once — no transition on height/display, ever.
    this.wrap.classList.add('sky-open');
    if (this.reduced) {
      this.sky.classList.add('sky-reveal');
      this.renderNow();
      return;
    }
    // Force layout so the browser sees the closed clip-path before we
    // animate to the open one (otherwise the two writes coalesce and there
    // is nothing to transition).
    void this.sky.offsetHeight;
    var self = this;
    requestAnimationFrame(function () { self.sky.classList.add('sky-reveal'); });
    this.renderNow();
  };
  Instance.prototype.close = function () {
    if (!this.open) return;
    this.open = false;
    this.chip.setAttribute('aria-expanded', 'false');
    this.sky.classList.remove('sky-reveal');
    LOOP.remove(this); this.pace = 0;
    var self = this, doneAfter = this.reduced ? 0 : 320;
    var finish = function () { if (!self.open) self.wrap.classList.remove('sky-open'); };
    window.setTimeout(finish, doneAfter);
    if (this.el.contains(document.activeElement) && document.activeElement !== this.chip) {
      try { this.chip.focus({ preventScroll: true }); } catch (e) {}
    }
  };
  Instance.prototype.onKeydown = function (e) {
    if (e.key === 'Escape' && this.open) {
      e.preventDefault();
      this.close();
      try { this.chip.focus({ preventScroll: true }); } catch (err) {}
    }
  };

  // ── Scoped styles. Ported layout from sky-widget.css (the design
  // contract), values re-mapped onto Chronicle's own design tokens
  // (static/css/input.css's --color-*/--ease-*/--dur-* family) instead of
  // the mockup's own --ink/--paper/--edge names, so the pane matches the
  // rest of the app in both themes without a second palette to maintain.
  // .dsky/.dscrub/.dwx/.dcard (the day-card weather band + its grow
  // interaction) are ported here too, at the CSS layer only, ahead of need:
  // the calendar docks its sky through SkyPane.Dock above, with its own
  // markup and styles (calendar-view.css), and its day card has no sky band
  // yet (TODO(#833)).
  //
  // Declared BEFORE Chronicle.register below, not after: every script here
  // loads with `defer`, and a deferred script executes once the document is
  // already "interactive" — so Chronicle.register's own mount scan (it
  // mounts any matching element immediately when the DOM isn't still
  // "loading", see boot.js) runs SYNCHRONOUSLY as part of evaluating this
  // very statement, calling injectStyles() before a `var` declared further
  // down this same file would exist. That was a real, shipped bug: the
  // hoisted-but-unassigned SKY_PANE_CSS read as undefined,
  // `style.textContent = undefined` silently became an empty string
  // (confirmed in Chromium — assigning undefined does NOT stringify to the
  // word "undefined"), and injectStyles()'s own guard (return early once
  // #sky-pane-styles exists) meant the pane never got a second chance: the
  // ENTIRE scoped stylesheet below was permanently empty on every real page
  // load, in every browser — the widget worked (fetch, render, toggle
  // state) but rendered fully unstyled. Caught by
  // test/e2e/sky_pane.spec.mjs's real-browser assertion that the closed
  // pane's computed height is 0. ──
  var SKY_PANE_CSS = [
    // container-name "cal" reuses the design contract's own container name
    // (.cal in sky-widget.css) so the phone rule below matches whether this
    // pane sits standalone (a dashboard/entity block, establishing its own
    // container) or, if a future caller nests it inside a real .cal
    // calendar card, that becomes the nearest match instead — either way
    // `cqw` below resolves against a real size container, never falling
    // back to 0.
    '.skypane{display:block;border-radius:12px;overflow:hidden;background:var(--color-card-bg);box-shadow:var(--elev-resting);container-type:inline-size;container-name:cal;}',
    '.skypane-bar{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:8px 12px;}',
    '.skypane-title{font:600 12px/1 var(--font-serif,inherit);color:var(--color-text-primary);}',
    '.skychip{display:inline-flex;align-items:center;gap:7px;height:30px;padding:0 11px 0 7px;border-radius:999px;font:600 12px/1 inherit;color:var(--color-text-body);background:none;border:0;cursor:pointer;box-shadow:inset 0 0 0 1px var(--color-border);}',
    '.skychip:hover{background:var(--color-bg-tertiary);color:var(--color-text-primary);}',
    '.skychip:focus-visible{outline:2px solid var(--color-text-primary);outline-offset:2px;}',
    '.skychip[aria-expanded="true"]{background:var(--color-bg-secondary);color:var(--color-text-primary);box-shadow:inset 0 0 0 1px var(--color-border);}',
    '.skychip .sw{width:16px;height:16px;border-radius:50%;flex:none;box-shadow:inset 0 0 0 1px rgba(0,0,0,.14);background:linear-gradient(#35507f,#9fb2d4);}',
    '.skychip-text{font-variant-numeric:tabular-nums;white-space:nowrap;}',
    '.skywrap{position:relative;height:0;overflow:hidden;--skyH:clamp(100px,32cqw,180px);}',
    '.skywrap.sky-open{height:var(--skyH);}',
    '.sky{position:relative;width:100%;height:var(--skyH);visibility:hidden;clip-path:inset(100% 0 0 0);transition:clip-path var(--dur-large,280ms) var(--ease-out,ease);}',
    '.skywrap.sky-open .sky{visibility:visible;}',
    '.skywrap.sky-open .sky.sky-reveal{clip-path:inset(0 0 0 0);}',
    '.sky canvas{position:absolute;inset:0;width:100%;height:100%;display:block;}',
    '.skycap{position:absolute;left:8px;bottom:6px;margin:0;font-size:11px;line-height:1.3;color:#f2f4fa;text-shadow:0 1px 2px rgba(0,0,0,.55);pointer-events:none;}',
    '@container cal (max-width:600px){.skywrap{--skyH:clamp(100px,14cqw,166px);}}',
    '@media (max-width:600px){.skywrap{--skyH:clamp(100px,14cqw,166px);}}',
    /* ── Day card: ready to wire in, but unused until the calendar's day
       card gets its own sky band (TODO(#833)). ── */
    '.dcard{position:relative;width:100%;max-width:340px;}',
    '.dsky{position:relative;height:80px;cursor:pointer;border:0;padding:0;display:block;width:100%;background:none;}',
    '.dsky canvas{position:absolute;left:0;top:0;width:100%;height:100%;display:block;}',
    '.dsky:focus-visible{outline:2px solid var(--color-text-primary);outline-offset:-2px;}',
    '.dscrub{padding:10px 14px 4px;}',
    '.dwx{display:flex;align-items:center;gap:8px;width:100%;margin-top:8px;padding:8px 10px;border-radius:10px;background:var(--color-bg-tertiary);font:500 12.5px/1.3 inherit;color:var(--color-text-primary);border:0;cursor:pointer;}',
    '.dwx:hover{box-shadow:inset 0 0 0 1px var(--color-border);}',
    '.dwx:focus-visible{outline:2px solid var(--color-text-primary);outline-offset:2px;}',
    '@container cal (max-width:600px){.dcard{max-width:100%;}}'
  ].join('\n');

  // ── The sky docked over a calendar's month (#830). The calendar builds the
  // markup (the pane flush at the top of its card, the Sky chip in its
  // header) and hands it here; this draws the sky and folds it.
  //
  // The fold, as signed: at the first frame the pane takes (or gives up) its
  // room; the card rides a transform so its top edge stays put and a clip
  // hides what is not there yet; the sky is drawn whole into the strip that
  // shows, its lower edge riding the header, like a shade rolling into the
  // chip. Reduced motion only fades. Frames run only while something moves:
  // folded, at rest, off screen or in a hidden tab, nothing is drawn. ──
  var FOLD_KEY = 'chronicle.calendar.sky';
  function readFolded() { try { return window.localStorage.getItem(FOLD_KEY) === 'folded'; } catch (e) { return false; } }
  function writeFolded(folded) {
    try { if (folded) window.localStorage.setItem(FOLD_KEY, 'folded'); else window.localStorage.removeItem(FOLD_KEY); }
    catch (e) { /* a private window: not remembered */ }
  }

  // Opening springs with a little give; closing is critically damped and
  // stiffer, so it settles sooner without passing its mark.
  function spring(sp, dt, opening) {
    var k = opening ? 240 : 900, z = opening ? .8 : 1, c = 2 * Math.sqrt(k) * z, n = Math.max(1, Math.ceil(dt / (1 / 240))), h = dt / n;
    for (var i = 0; i < n; i++) { sp.v += (k * (sp.target - sp.e) - c * sp.v) * h; sp.e += sp.v * h; }
    if (Math.abs(sp.target - sp.e) < .0012 && Math.abs(sp.v) < .015) { sp.e = sp.target; sp.v = 0; sp.on = false; }
  }
  function px(v) { return (Math.round(v * 100) / 100) + 'px'; }
  // The card's clip while it travels: its top edge rounded like the card,
  // open to the sides and below so its shadow still shows. g is the card's
  // size and corner, measured once per fold.
  function cardGeo(el) { return { W: el.offsetWidth, H: el.offsetHeight, r: parseFloat(getComputedStyle(el).borderTopLeftRadius) || 0 }; }
  function clipTop(g, top) {
    var W = g.W, H = g.H, r = g.r, s = 40, t = Math.round(top * 100) / 100;
    return 'path("M0 ' + (t + r) + (r ? ' A' + r + ' ' + r + ' 0 0 1 ' + r + ' ' + t : '') + ' H' + (W - r) + (r ? ' A' + r + ' ' + r + ' 0 0 1 ' + W + ' ' + (t + r) : '') +
      ' H' + (W + s) + ' V' + (H + s) + ' H' + (-s) + ' V' + (t + r) + ' Z")';
  }
  // The pane's height: 14% of the card's width, at least 100px (a phone's
  // compact strip) and at most 166px. Set from here because the card cannot
  // be a size container: its phone sheets are position:fixed.
  function paneHeight(w) { return Math.round(SW.clamp(w * .14, 100, 166)); }

  // o: {card, wrap (.skywrap holding .sky > canvas), chip (holding .sw),
  //     seed, name, campaignId + calendarId (to read the day's own weather;
  //     without them the calendar's current weather shows), followers() → elements below the card that ride with it,
  //     before() → called as a fold starts, travel() → false to fold without
  //     moving the card, say(text)}. Throws when there is nothing to draw with.
  function Dock(o) {
    var self = this;
    this.o = o;
    this.card = o.card; this.wrap = o.wrap; this.chip = o.chip;
    this.sky = o.wrap.querySelector('.sky');
    this.canvas = this.sky.querySelector('canvas');
    this.sw = o.chip.querySelector('.sw');
    this.ctx = this.canvas.getContext('2d');
    if (!this.ctx || !window.Sky2D || !SW || !LOOKS || !EV || !window.SkyFX) throw new Error('sky: nothing to draw with');
    this.rm = reducedMotion();
    this.dpr = Math.max(1, Math.min(2, window.devicePixelRatio || 1));
    this.model = null; this.pace = 0; this.tmo = 0; this.W = 0; this.H = 0; this.raf = 0; this.lastT = 0; this.onScreen = true;
    var open = !readFolded();
    this.S = { target: open ? 1 : 0, e: open ? 1 : 0, v: 0, on: false, layout: open, fade: 1, lastE: open ? 1 : 0 };
    this.wrap.classList.toggle('open', open);
    this.chip.setAttribute('aria-expanded', String(open));
    this._tick = function (tMs) { self.tick(tMs); };
    this._click = function () { self.fold(!self.S.layout); };
    this._vis = function () { self.kick(); };
    // Back from rest: start drawing again.
    if (window.MotionRest) window.MotionRest.onWake(this._vis);
    // The painted sky becoming ready, or giving way to the basic one, repaints.
    this._glChange = function () { if (self.S.layout && !self.S.on) self.paint(clock(), 0, self.H); self.kick(); };
    if (window.SkyGL) window.SkyGL.onChange(this._glChange);
    this.chip.addEventListener('click', this._click);
    document.addEventListener('visibilitychange', this._vis);
    if ('ResizeObserver' in window) {
      this.ro = new ResizeObserver(function () { if (self.wrap.clientWidth !== self.W) self.resized(); });
      this.ro.observe(this.wrap);
    } else {
      this._resize = function () { self.resized(); };
      window.addEventListener('resize', this._resize);
    }
    if ('IntersectionObserver' in window) {
      this.io = new IntersectionObserver(function (es) { es.forEach(function (e) { self.onScreen = e.isIntersecting; }); self.kick(); }, { threshold: 0 });
      this.io.observe(this.sky);
    }
    this.measure();
  }
  Dock.prototype.measure = function () {
    var W = this.wrap.clientWidth;
    if (!W) return false;
    var H = paneHeight(W), bw = Math.round(W * this.dpr), bh = Math.round(H * this.dpr);
    if (H !== this.H) this.wrap.style.setProperty('--skyH', H + 'px');
    this.W = W; this.H = H;
    if (this.canvas.width !== bw || this.canvas.height !== bh) { this.canvas.width = bw; this.canvas.height = bh; }
    return true;
  };
  // The day the sky shows: the calendar as this viewer may see it, and that
  // day's events. Players only ever get today as it is from here.
  Dock.prototype.setDay = function (cal, events) {
    // A new day keeps the weather it is rolling from, so a change rolls in.
    var self = this, prev = this.model, glide = prev ? prev.glide : null;
    // The same day keeps its reading while a fresh one loads.
    var same = prev && prev.calendar.current_year === cal.current_year && prev.calendar.current_month === cal.current_month && prev.calendar.current_day === cal.current_day;
    this.model = { calendar: cal, todayEvents: events || [], dayWeather: same ? prev.dayWeather : null, skym: SW.makeSkym(cal), landSeed: hashSeed(this.o.seed), width: 0, height: 0, reduced: this.rm, glide: glide };
    this.redraw();
    var tok = this._wxTok = (this._wxTok || 0) + 1;
    if (this.o.campaignId && this.o.calendarId) fetchDayWeather(this.o.campaignId, this.o.calendarId, cal).then(function (w) {
      if (tok !== self._wxTok || !self.model || JSON.stringify(w) === JSON.stringify(self.model.dayWeather)) return;
      self.model.dayWeather = w;
      self.redraw();
    });
  };
  // Re-reads the model into the chip, the pace and (when it rests open) the sky.
  Dock.prototype.redraw = function () {
    if (this.destroyed) return;
    var m = this.model;
    var st = buildState({ calendar: m.calendar, todayEvents: m.todayEvents, dayWeather: m.dayWeather, skym: m.skym, landSeed: m.landSeed, width: this.W || 640, height: this.H || 120, reduced: this.rm }, 0);
    this.pace = paceOf(st);
    this.label(st);
    if (this.measure() && this.S.layout && !this.S.on) this.paint(clock(), 0, this.H);
    this.kick();
  };
  // The chip's swatch is the sky now, in two colours, with its brightest body
  // as a dot; the sky's own name says the same in words.
  Dock.prototype.label = function (st) {
    var src = st.alt > 0 ? st.sp : brightestMoon(st), body = st.alt > 0 ? 'rgba(255,236,190,.95)' : 'rgba(230,236,248,.9)';
    var dot = src ? 'radial-gradient(circle at ' + SW.clamp(src.x / st.W * 100, 20, 80).toFixed(0) + '% ' + SW.clamp(src.y / st.H * 100, 22, 70).toFixed(0) + '%,' + body + ' 0 2px,transparent 2.6px),' : '';
    this.sw.style.setProperty('--sw', dot + 'linear-gradient(' + LOOKS.linHex(st.P.zen) + ',' + LOOKS.linHex(st.P.hor) + ')');
    this.sky.setAttribute('aria-label', 'The sky over ' + (this.o.name || 'the world') + ' now. ' + describe(this.model, st));
  };
  // Draws the whole sky into the strip [y, y+h) of the pane, snapped to
  // device pixels, and clears the rest.
  Dock.prototype.paint = function (t, y, h) {
    if (!this.model || !this.W) return;
    var d = this.dpr, y0 = Math.round(y * d) / d, h0 = Math.round((y + h) * d) / d - y0;
    this.ctx.setTransform(1, 0, 0, 1, 0, 0);
    this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
    if (h0 < 2) return;
    this.model.width = this.W; this.model.height = h0; this.model.restH = this.H;
    var dt = this.lastPaint ? t - this.lastPaint : 0;
    this.lastPaint = t;
    var st = buildState(this.model, this.rm ? 0 : t, dt);
    paintSky(this.ctx, this, st, d, y0, !!this.looping);
    this.pace = paceOf(st);
  };
  Dock.prototype.fold = function (open) {
    var S = this.S, target = open ? 1 : 0;
    if (target === S.target && (S.on || S.e === target)) return;
    if (this.o.before) this.o.before();
    S.target = target;
    // The first frame: the room changes at once; transforms hold everything where it was.
    this.wrap.classList.toggle('open', open); S.layout = open;
    this.chip.setAttribute('aria-expanded', String(open));
    writeFolded(!open);
    var travel = !this.rm && !document.hidden && (!this.o.travel || this.o.travel());
    if (travel) S.on = true;
    else { S.e = target; S.v = 0; S.on = false; S.fade = open && this.rm ? 0 : 1; }
    this.measure();
    this.geo = cardGeo(this.card);
    this.apply(clock());
    this.kick();
    if (this.o.say) this.o.say(open ? 'Sky open' : 'Sky folded into its chip');
  };
  Dock.prototype.apply = function (t) {
    var S = this.S, H = this.H, e = S.e, card = this.card, fol = this.o.followers ? this.o.followers() : [];
    if (!S.on && (e === 0 || e === 1)) {
      card.style.transform = ''; card.style.clipPath = ''; card.classList.remove('moving'); this.sky.style.clipPath = '';
      fol.forEach(function (el) { el.style.transform = ''; });
      this.sky.style.opacity = this.rm && S.layout && S.fade < 1 ? String(S.fade) : '';
      if (S.layout) this.paint(t, 0, H);
      // The shade rolled into the chip, and the chip catches it.
      if (S.lastE !== e && e === 0 && !this.rm) { this.chip.classList.remove('caught'); void this.chip.offsetWidth; this.chip.classList.add('caught'); }
      S.lastE = e;
      return;
    }
    S.lastE = e;
    var ty = S.layout ? -H * (1 - e) : H * e, top = S.layout ? H * (1 - e) : -H * e;
    card.classList.add('moving');
    card.style.transform = 'translateY(' + px(ty) + ')';
    card.style.clipPath = clipTop(this.geo || (this.geo = cardGeo(card)), top);
    this.sky.style.clipPath = 'inset(' + px(H * (1 - e)) + ' 0 0 0)';
    fol.forEach(function (el) { el.style.transform = 'translateY(' + px(ty) + ')'; });
    this.paint(t, H * (1 - e), H * e);
  };
  Dock.prototype.tick = function (tMs) {
    this.raf = 0;
    var S = this.S, dt = this.lastT ? Math.min(.05, (tMs - this.lastT) / 1000) : 1 / 60, t = clock();
    this.lastT = tMs;
    if (S.on || S.fade < 1) {
      if (S.fade < 1) S.fade = Math.min(1, S.fade + dt / .18);
      // The frame a spring settles in is applied too, so the rest state lands.
      if (S.on) spring(S, dt, S.target === 1);
      this.apply(t);
    } else if (this.pace && S.layout) { this.looping = true; this.paint(t, 0, this.H); this.looping = false; }
    this.kick();
  };
  Dock.prototype.kick = function () {
    var S = this.S, self = this, live = !document.hidden && !this.rm && S.layout && this.onScreen && !resting();
    var want = !document.hidden && (S.on || S.fade < 1 || (live && this.pace === 2));
    if (want) { if (this.tmo) { clearTimeout(this.tmo); this.tmo = 0; } if (!this.raf) this.raf = requestAnimationFrame(this._tick); return; }
    if (this.raf) { cancelAnimationFrame(this.raf); this.raf = 0; }
    this.lastT = 0;
    // A sky where only clouds drift is painted at the idle rate.
    if (live && this.pace === 1) { if (!this.tmo) this.tmo = setTimeout(function () { self.tmo = 0; if (!self.raf) self.raf = requestAnimationFrame(self._tick); }, IDLE_S * 1000); }
    else if (this.tmo) { clearTimeout(this.tmo); this.tmo = 0; }
    // A fold that could not run its frames (a hidden tab) lands at once.
    if (S.on) { S.e = S.target; S.v = 0; S.on = false; this.apply(clock()); }
  };
  Dock.prototype.resized = function () {
    var S = this.S;
    if (S.on) { S.e = S.target; S.v = 0; S.on = false; }
    if (!this.measure()) return;
    this.apply(clock());
  };
  Dock.prototype.destroy = function () {
    this.destroyed = true;
    if (this.raf) cancelAnimationFrame(this.raf);
    this.raf = 0;
    if (this.tmo) clearTimeout(this.tmo);
    this.tmo = 0;
    if (window.SkyGL) window.SkyGL.offChange(this._glChange);
    this.chip.removeEventListener('click', this._click);
    document.removeEventListener('visibilitychange', this._vis);
    if (window.MotionRest) window.MotionRest.offWake(this._vis);
    if (this.ro) this.ro.disconnect();
    if (this._resize) window.removeEventListener('resize', this._resize);
    if (this.io) this.io.disconnect();
    this.card.style.transform = ''; this.card.style.clipPath = ''; this.card.classList.remove('moving');
    (this.o.followers ? this.o.followers() : []).forEach(function (el) { el.style.transform = ''; });
  };
  window.SkyPane = { Dock: Dock, paneHeight: paneHeight, spring: spring, FOLD_KEY: FOLD_KEY };

  Chronicle.register('sky-pane', {
    init: function (el) { injectStyles(); var inst = new Instance(el); el._skyPane = inst; inst.init(); },
    destroy: function (el) { if (el._skyPane) { el._skyPane.destroy(); el._skyPane = null; } }
  });
})();
