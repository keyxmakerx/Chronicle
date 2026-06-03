// cal-almanac.js — C-CAL-SHOWCASE-DESIGN-1-ALMANAC + REFINEMENT-V2.
//
// Per-interaction INIT_BLOCKS, each wrapped in its own try/catch so one
// failing handler can't kill the rest. Mock data is read from the inline
// JSON the templ emits; nothing here talks to a backend.
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
  var VIEW = { year: 0, month: 0, day: 0, timeFrac: 0.5 };

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function key(y, m, d) { return y + '-' + m + '-' + d; }
  function pad2(n) { return String(n).padStart(2, '0'); }

  // ============================================================
  // Block: data
  // ============================================================
  registerInitBlock('data', function () {
    var node = document.getElementById('cal-almanac-data');
    if (!node) throw new Error('cal-almanac-data JSON node missing');
    DATA = JSON.parse(node.textContent || '{}');
    VIEW.year = DATA.current_year;
    VIEW.month = DATA.current_month;
    VIEW.day = DATA.current_day;
    VIEW.timeFrac = (DATA.sky_time != null) ? DATA.sky_time : 0.5;
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
    WEATHER_EFFECTS.clear = { id: 'clear', tier: 'must', renderFn: function () {} };
    WEATHER_EFFECTS.cloudy = { id: 'cloudy', tier: 'must', renderFn: function (box) {
      spawn('cal-almanac-cloud cal-almanac-cloud--1', box);
      spawn('cal-almanac-cloud cal-almanac-cloud--2', box);
      spawn('cal-almanac-cloud cal-almanac-cloud--3', box);
    } };
    WEATHER_EFFECTS.rain = { id: 'rain', tier: 'must', renderFn: function (box) { rain(box, false); } };
    WEATHER_EFFECTS.thunderstorm = { id: 'thunderstorm', tier: 'must', renderFn: function (box) {
      spawn('cal-almanac-cloudbank cal-almanac-cloudbank--dark', box);
      rain(box, true);
      var l = spawn('cal-almanac-lightning', box); l.setAttribute('data-cal-lightning', '');
    } };
    WEATHER_EFFECTS.snow = { id: 'snow', tier: 'must', renderFn: function (box) {
      for (var i = 0; i < 26; i++) {
        var left = (i * 41 + 3) % 100;
        var delay = ((i * 61) % 100) / 100;
        var dur = 3 + ((i * 23) % 30) / 10;
        var drift = ((i % 5) - 2) * 8;
        spawn('cal-almanac-snow', box, 'left:' + left + '%;animation-delay:' + delay + 's;animation-duration:' + dur + 's;--drift:' + drift + 'px;');
      }
    } };
    WEATHER_EFFECTS.fog = { id: 'fog', tier: 'must', renderFn: function (box) { spawn('cal-almanac-fog', box); } };
    // TBD stubs — registry-wired; visual deferred.
    ['ashfall', 'acid-rain', 'arcane-winds', 'ley-surge', 'sakura-bloom'].forEach(function (id) {
      WEATHER_EFFECTS[id] = { id: id, tier: 'tbd', renderFn: function () {} };
    });
    window.__calWeatherEffects = WEATHER_EFFECTS;
  });

  registerInitBlock('celestial-registry', function () {
    CELESTIAL_EFFECTS['meteor-shower'] = { id: 'meteor-shower', tier: 'must', renderFn: function (box) {
      var wrap = document.createElement('div'); wrap.className = 'cal-almanac-meteors'; wrap.setAttribute('data-cal-meteors', '');
      for (var i = 0; i < 6; i++) {
        var top = (i * 23 + 4) % 40, left = 55 + (i * 17) % 40, delay = i * 1.7;
        spawn('cal-almanac-meteor', wrap, 'top:' + top + '%;left:' + left + '%;animation-delay:' + delay + 's;');
      }
      box.appendChild(wrap);
    } };
    CELESTIAL_EFFECTS['eclipse-solar'] = { id: 'eclipse-solar', tier: 'must', renderFn: function (box) {
      var e = document.createElement('div'); e.className = 'cal-almanac-eclipse cal-almanac-eclipse--solar'; box.appendChild(e);
    } };
    CELESTIAL_EFFECTS['eclipse-lunar'] = { id: 'eclipse-lunar', tier: 'must', renderFn: function (box) {
      var e = document.createElement('div'); e.className = 'cal-almanac-eclipse cal-almanac-eclipse--lunar'; box.appendChild(e);
    } };
    ['volcanic', 'ice-age', 'plague', 'arcane-surge', 'moon-special', 'aurora', 'comet'].forEach(function (id) {
      CELESTIAL_EFFECTS[id] = { id: id, tier: 'tbd', renderFn: function (box, ctx) {
        var s = document.createElement('div'); s.className = 'cal-almanac-celestial-stub'; s.textContent = (ctx && ctx.name) || id; box.appendChild(s);
      } };
    });
    window.__calCelestialEffects = CELESTIAL_EFFECTS;
  });

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
  function renderSkyForDay(m, day) {
    var sky = document.querySelector('[data-cal-sky]');
    if (!sky) return;
    var wtypeID = dayWeatherTypeID(m, day);
    var effID = wtypeID ? weatherEffectID(wtypeID) : 'clear';
    sky.setAttribute('data-cal-sky-weather', effID);
    sky.className = sky.className.replace(/cal-almanac-sky--wfx-\S+/g, '').trim() + ' cal-almanac-sky--wfx-' + effID;
    // Weather layer.
    var wlayer = sky.querySelector('[data-cal-sky-weather-layer]');
    if (wlayer) {
      wlayer.innerHTML = '';
      var w = WEATHER_EFFECTS[effID];
      if (w) w.renderFn(wlayer, weatherTypeById(wtypeID) || {});
    }
    // Celestial layer.
    var clayer = sky.querySelector('[data-cal-sky-celestial-layer]');
    var events = celestialFor(m, day);
    if (clayer) {
      clayer.innerHTML = '';
      events.forEach(function (c) {
        var fx = CELESTIAL_EFFECTS[c.type];
        if (fx) fx.renderFn(clayer, c);
      });
    }
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
    // Label suffix (weather name + season).
    var rest = sky.querySelector('[data-cal-sky-sub-rest]');
    if (rest) {
      var wt = weatherTypeById(wtypeID);
      rest.textContent = ' · ' + skyLabel(VIEW.timeFrac) + ' · ' + seasonName() + ' · ' + (wt ? wt.name : 'Clear');
    }
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
  registerInitBlock('sky-band-ambient', function () {
    // Re-render once on init so the JS-built layers match the registries
    // (the server pre-render is for no-JS; this keeps the source single).
    renderSkyForDay(VIEW.month, VIEW.day);
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
  function applyTime(t, opts) {
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
  // Block: era-vignette — click → era detail (in a sub-popover reusing
  // the quick-view shell as a lightweight info panel).
  // ============================================================
  registerInitBlock('era-vignette', function () {
    var vig = document.querySelector('[data-cal-era-vignette]');
    if (!vig) return;
    vig.addEventListener('click', function () {
      var era = (DATA.eras || []).find(function (e) { return e.id === vig.getAttribute('data-cal-era-id'); });
      if (!era) return;
      openSkyPanel('Era · ' + era.name, [
        '<div class="cal-almanac-skypanel__row"><b>' + esc(era.name) + '</b></div>',
        '<div class="cal-almanac-skypanel__row">' + esc(eraSpan(era)) + '</div>',
        era.description ? '<div class="cal-almanac-skypanel__row">' + esc(era.description) + '</div>' : ''
      ].join(''));
    });
  });
  function eraSpan(e) { return e.end_year ? (e.start_year + ' – ' + e.end_year) : (e.start_year + ' – ongoing'); }

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
    var cel = celestialFor(m, day);
    if (cel && cel.length) return { id: cel[0].type, kind: 'celestial' };
    var wtypeID = dayWeatherTypeID(m, day);
    var effID = wtypeID ? weatherEffectID(wtypeID) : 'clear';
    return { id: effID, kind: 'weather' };
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
    var stream = hg.querySelector('[data-cal-hourglass-stream]');
    if (stream) {
      var reg = theme.kind === 'celestial' ? CELESTIAL_EFFECTS[theme.id] : WEATHER_EFFECTS[theme.id];
      var fn = (reg && reg.sandRender) || defaultSandRender;
      fn(stream);
    }
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
    // Subscribe to time changes by wrapping applyTime.
    var prev = applyTime;
    window.__applyTimeOrig = prev;
    applyTime = function (t) {
      prev(t);
      applyHourglassLevels(t);
      applyHourglassFlip(t);
    };
  });

  registerInitBlock('hourglass-themed-sand', function () {
    hookSandRenderers();
    applySandTheme(VIEW.month, VIEW.day);
    // Re-apply on day-change. We piggyback on renderSkyForDay by wrapping.
    var prev = renderSkyForDay;
    renderSkyForDay = function (m, day) {
      prev(m, day);
      applySandTheme(m, day);
    };
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
  // Trigger
  // ============================================================
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
})();
