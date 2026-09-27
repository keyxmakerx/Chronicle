/*
 * sky_pane.js — the sky pane widget (issue #763). Chronicle.register('sky-pane', ...).
 *
 * Fetches the campaign's default calendar (already resolved server-side —
 * see the "skybox" block in internal/app/routes.go, which passes the
 * calendar id straight into the mount so this widget never has to fetch
 * /calendars/list just to find it) plus its events, then draws the current
 * moment's sky with Sky2D (sky_2d.js) using SkyWorld's astronomy/palette
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

  // ── One shared render loop for every sky-pane instance on the page, so N
  // panes cost one rAF, not N. An instance only receives ticks while it opts
  // in (continuousInstances) — see PACE below. ──
  var LOOP = (function () {
    var insts = [], running = false;
    function tick(tMs) {
      if (!document.hidden) {
        insts.slice().forEach(function (inst) {
          if (inst.onScreen !== false) inst.render(tMs / 1000);
        });
      }
      if (insts.length) requestAnimationFrame(tick);
      else running = false;
    }
    return {
      add: function (inst) { if (insts.indexOf(inst) < 0) insts.push(inst); if (!running) { running = true; requestAnimationFrame(tick); } },
      remove: function (inst) { var i = insts.indexOf(inst); if (i >= 0) insts.splice(i, 1); }
    };
  })();

  function reducedMotion() {
    try { return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches); }
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
    if (p < .125) return 'New Moon';
    if (p < .25) return 'Waxing Crescent';
    if (p < .375) return 'First Quarter';
    if (p < .5) return 'Waxing Gibbous';
    if (p < .625) return 'Full Moon';
    if (p < .75) return 'Waning Gibbous';
    if (p < .875) return 'Last Quarter';
    return 'Waning Crescent';
  }
  function hhmm(hour, minute) {
    var h = Math.floor(hour), m = Math.round(minute || 0);
    var ap = h >= 12 ? 'PM' : 'AM', h12 = h % 12; if (h12 === 0) h12 = 12;
    return h12 + ':' + (m < 10 ? '0' : '') + m + ' ' + ap;
  }
  function hashSeed(s) { s = String(s || ''); var v = 0; for (var i = 0; i < s.length; i++) v = (v * 131 + s.charCodeAt(i)) >>> 0; return v || 1; }

  // buildState(model, tSeconds): the per-frame render state Sky2D.draw
  // consumes. Mirrors the design contract's Surface.prototype.state, minus
  // every field only the WebGL uniforms needed.
  function buildState(model, tSeconds) {
    var cal = model.calendar;
    var year = cal.current_year, month = cal.current_month, day = cal.current_day;
    var h24 = SW.hour24(cal, cal.current_hour || 0, cal.current_minute || 0);
    var skym = model.skym;
    var sun = skym.sun(year, month, day, h24);
    var absDay = SW.absoluteDay(cal, year, month, day);
    var tCont = absDay + h24 / 24;

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

    var moons = astro.map(function (a) {
      var mo = a.mo, up = a.up, r = R * (mo.size || 1);
      var blood = up && !!overlay.blood[mo.id];
      var harvest = up && !!overlay.harvest[mo.id];
      if (harvest) r *= 1.18;
      var shadow = null;
      if (up && overlay.eclipse[mo.id] != null) shadow = [0, 0, r * .55, overlay.eclipse[mo.id]];
      var p = up ? PROJ_at(L, a.m.alt, a.m.az) : { x: 0, y: 0 };
      var rot = up ? SW.moonTurn(L, a.m, sun) : 0;
      return { mo: mo, m: a.m, up: up, x: p.x, y: p.y, r: r, rot: rot, spec: window.SkyMoon.specFor(mo), blood: blood, shadow: shadow };
    });

    var weatherSpec = LOOKS.spec(cal.weather);
    var look = LOOKS.resolve(weatherSpec);
    var wx = look || LOOKS.blank();
    var P = SW.paletteFor(altDeg, eve, look, wx, dark, false);
    if (overlay.magic) SW.tintPalette(P, '#8a5fe0', .3, ['zen', 'mid', 'hor', 'anti', 'clit', 'cshade', 'h0', 'h1', 'h2', 'h3']);

    return {
      P: P, L: L, W: model.width, H: model.height, wx: wx, t: tSeconds,
      sun: sun, sp: sp, alt: sun.alt, altEff: altDeg, dark: dark, R: R,
      moons: moons, cam: cam, sidereal: SW.mod((sun.H + sun.lon) / SW.D2R, 360),
      landSeed: model.landSeed, overlay: overlay,
      weatherLabel: weatherSpec.label || 'Clear skies'
    };
  }
  function PROJ_at(L, alt, az) { return SW.PROJ.at(L, alt, az); }

  function paceOf(state) {
    return Object.keys(state.overlay.blood || {}).length > 0;
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
    this.load();
    // A slow drift check even while closed, so the chip swatch and caption
    // never go stale across a long-open page (the world clock advances).
    this._refreshTimer = window.setInterval(function () { self.refreshPalette(); }, 60000);
  };
  Instance.prototype.destroy = function () {
    this.chip.removeEventListener('click', this._boundClick);
    this.el.removeEventListener('keydown', this._boundKeydown);
    window.removeEventListener('resize', this._boundResize);
    if (this.io) this.io.disconnect();
    if (this._refreshTimer) window.clearInterval(this._refreshTimer);
    LOOP.remove(this);
  };
  Instance.prototype.load = function () {
    var self = this, cid = this.el.getAttribute('data-campaign-id'), calid = this.el.getAttribute('data-calendar-id');
    var fetchJSON = function (url) {
      return (window.Chronicle && Chronicle.apiFetch ? Chronicle.apiFetch(url) : fetch(url, { credentials: 'same-origin', headers: { Accept: 'application/json' } }))
        .then(function (r) { if (!r.ok) throw new Error('http ' + r.status); return r.json(); });
    };
    fetchJSON('/campaigns/' + cid + '/calendars/' + calid).then(function (cal) {
      var y = cal.current_year, m = cal.current_month;
      return fetchJSON('/campaigns/' + cid + '/calendars/' + calid + '/events?year=' + y + '&month=' + m).then(function (events) {
        var todayEvents = (events || []).filter(function (e) { return e.year === cal.current_year && e.month === cal.current_month && e.day === cal.current_day; });
        self.model = { calendar: cal, todayEvents: todayEvents, skym: SW.makeSkym(cal), landSeed: hashSeed(calid), width: 0, height: 0 };
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
    var st = buildState(this.model, 0);
    var hexOf = function (lin) { return LOOKS.linHex(lin); };
    this.chipSw.style.background = 'linear-gradient(' + hexOf(st.P.zen) + ',' + hexOf(st.P.hor) + ')';
    var cal = this.model.calendar;
    this.chipText.textContent = hhmm(cal.current_hour || 0, cal.current_minute || 0);
    var brightest = st.moons.filter(function (m) { return m.up; }).sort(function (a, b) { return b.r - a.r; })[0];
    var bits = [st.weatherLabel];
    if (brightest) bits.push(brightest.mo.name + ' – ' + phaseName(brightest.m.p));
    bits.push(hhmm(cal.current_hour || 0, cal.current_minute || 0));
    this.caption.textContent = bits.join(' · ');
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
      self.render(self.reduced ? 0 : performance.now() / 1000);
    }, 100);
  };
  Instance.prototype.resizeCanvas = function () {
    var w = this.sky.clientWidth, h = this.sky.clientHeight;
    if (!w || !h) return;
    var W = Math.max(1, Math.round(w * this.dpr)), H = Math.max(1, Math.round(h * this.dpr));
    if (this.canvas.width !== W || this.canvas.height !== H) { this.canvas.width = W; this.canvas.height = H; }
    if (this.model) { this.model.width = w; this.model.height = h; }
  };
  Instance.prototype.render = function (tSeconds) {
    if (!this.model || !this.open) return;
    if (!this.model.width) this.resizeCanvas();
    if (!this.model.width) return;
    var st = buildState(this.model, tSeconds);
    window.Sky2D.draw(this.ctx, st, this.dpr);
  };
  Instance.prototype.renderNow = function () {
    this.resizeCanvas();
    this.render(this.reduced ? 0 : performance.now() / 1000);
    if (!this.reduced && this.model) { if (paceOf(buildState(this.model, 0))) LOOP.add(this); }
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
    LOOP.remove(this);
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
  // the calendar page's day grid (internal/plugins/calendar/view.templ)
  // exists now but does not mount this widget or use these rules — it
  // paints its own separate, simplified sky context instead (TODO(#741));
  // see internal/widgets/sky/.ai.md's "Integrating into the calendar page"
  // for what wiring them in would take.
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
    /* ── Day card: ready to wire in, but unused — the calendar page's day
       grid paints its own sky context instead of mounting this widget
       (TODO(#741)). ── */
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

  Chronicle.register('sky-pane', {
    init: function (el) { injectStyles(); var inst = new Instance(el); el._skyPane = inst; inst.init(); },
    destroy: function (el) { if (el._skyPane) { el._skyPane.destroy(); el._skyPane = null; } }
  });
})();
