/*
 * sky_looks.js — how weather looks in the sky (window.SkyLooks).
 *
 * DISCLOSED TRIM vs. the design contract's sky-looks.js: the mockup resolves
 * a weather into one of 41 hand-tuned "effect" building blocks (clouds,
 * aurora, sandstorms, arcane wind, particle systems...), each driving the
 * mockup's WebGL painter (air layers, particles, fireballs). Chronicle's
 * sole renderer is the contract's OWN 2D fallback painter (sky_2d.js,
 * SKY2D.draw) — see the PR description for why WebGL was skipped — and that
 * painter never reads any of those layers; it only ever consumes cloud,
 * dark, fog, rain, snow, hail, storm, wind, a sky tint + light factor, and a
 * star dimmer. Porting the full 41-effect table would spend real effort
 * feeding fields the fallback painter throws away.
 *
 * So this ports the REAL mechanism (the OKLab colour-banding math: tame/lit/
 * okOf/okLin, and the blank/resolve/blend recipe shape) faithfully, over a
 * SMALL effect table that covers what real Chronicle weather actually needs
 * to show in the fallback painter: clear, light/heavy/overcast cloud, fog,
 * rain, a heavier downpour, snow, heavier snow, hail, and a storm (cloud +
 * rain + darkening — the mockup's lightning bolts are a WebGL-only draw the
 * 2D painter never had either, so dropping them costs nothing here).
 *
 * Follow-up filed for the operator: a fuller weather-look vocabulary
 * (aurora, sand, smoke, arcane looks, etc.) is only worth adding once/if a
 * WebGL painter is, since the 2D painter has no surface for them today.
 *
 * A real Weather row (PresetID/Icon/Precipitation) is mapped onto this table
 * by id first, then by a small heuristic over Icon/Precipitation/intensity
 * for any preset id sky_looks.js doesn't recognize by name — so a custom
 * campaign preset still renders something reasonable instead of "clear".
 */
(function () {
  'use strict';

  var PAL = window.SkyWorld.PAL, clamp = window.SkyWorld.clamp;

  function okOf(hex) { var q = PAL.lab(hex); return { L: q[0], C: Math.hypot(q[1], q[2]), h: Math.atan2(q[2], q[1]) }; }
  function okLin(l, c, h) { return PAL.labLin([l, c * Math.cos(h), c * Math.sin(h)]); }
  var BANDS = {
    glow: { day: [.74, .93, .04, .16], night: [.64, .86, .05, .15] },
    body: { day: [.55, .85, .02, .14], night: [.42, .72, .02, .12] },
    air: { day: [.62, .92, .01, .11], night: [.40, .70, .01, .10] },
    shade: { day: [.14, .34, .02, .12], night: [.08, .26, .02, .10] }
  };
  function tame(hex, role, night) {
    var o = okOf(hex), b = BANDS[role] || BANDS.body, n = clamp(night || 0, 0, 1);
    var lo = b.day[0] + (b.night[0] - b.day[0]) * n, hi = b.day[1] + (b.night[1] - b.day[1]) * n;
    var cl = b.day[2] + (b.night[2] - b.day[2]) * n, ch = b.day[3] + (b.night[3] - b.day[3]) * n;
    var l = clamp(o.L, lo, hi), c = o.C < .025 ? Math.min(o.C, ch) : clamp(o.C, cl, ch);
    return okLin(l, c, o.h);
  }
  function linHex(rgb) { return '#' + rgb.map(function (v) { v = clamp(v, 0, 1); v = v <= .0031308 ? v * 12.92 : 1.055 * Math.pow(v, 1 / 2.4) - .055; return ('0' + Math.round(v * 255).toString(16)).slice(-2); }).join(''); }
  function srgb(rgb, a) { return 'rgba(' + rgb.map(function (v) { v = clamp(v, 0, 1); return Math.round(255 * (v <= .0031308 ? v * 12.92 : 1.055 * Math.pow(v, 1 / 2.4) - .055)); }).join(',') + ',' + (a == null ? 1 : +a.toFixed(3)) + ')'; }
  function lum(c) { return .2126 * c[0] + .7152 * c[1] + .0722 * c[2]; }
  // lit(hex, role, sl): a colour lit by the scene (sl = {col, amb, night, cap}
  // from SKYFX-equivalent scene light), banded by role. Used by the sky
  // events layer (blood tint, harvest glow) so those colours sit in the same
  // OKLab bands the moon/weather colours do, day or night.
  function lit(hex, role, sl) {
    var c = tame(hex, role === 'glow' ? 'glow' : role, sl.night), out;
    if (role === 'glow') out = [0, 1, 2].map(function (k) { return c[k] * (1 + (sl.col[k] - 1) * .25); });
    else if (role === 'shade') out = [0, 1, 2].map(function (k) { return c[k] * (.6 + .4 * sl.col[k] * sl.amb); });
    else out = [0, 1, 2].map(function (k) { return c[k] * sl.col[k] * Math.max(sl.amb, .1); });
    var y = lum(out);
    if (y > sl.cap) out = out.map(function (v) { return v * sl.cap / y; });
    return out;
  }

  // ── The trimmed effect table (see the file doc comment for what's kept
  // and why). base: 0..1 painted-sky physics; sky: [tintHex, strength, light]. ──
  var EFFECTS = {
    'clear': { base: { cloud: .08, wind: .2 } },
    'clouds-light': { base: { cloud: .38, fog: .02, wind: .3 }, sky: ['#a9b3c0', .08, 1] },
    'clouds-heavy': { base: { cloud: .72, dark: .12, fog: .05, wind: .35 }, sky: ['#828896', .18, .96] },
    'clouds-overcast': { base: { cloud: .95, dark: .22, fog: .08, wind: .3 }, sky: ['#6e7078', .3, .9] },
    'fog': { base: { cloud: .35, dark: .05, fog: 1, wind: .1 }, sky: ['#b9bec2', .2, 1] },
    'rain': { base: { cloud: .95, dark: .38, fog: .18, rain: 1, wind: .5 }, sky: ['#5a6478', .25, .92] },
    'rain-heavy': { base: { cloud: 1, dark: .55, fog: .26, rain: 1.5, wind: .75 }, sky: ['#434a5a', .35, .85] },
    'storm': { base: { cloud: 1, dark: .85, fog: .1, rain: 1.25, storm: 1, wind: 1 }, sky: ['#2c2f45', .35, .88] },
    'hail': { base: { cloud: .95, dark: .5, fog: .12, rain: .35, hail: 1, wind: .6 }, sky: ['#4f566a', .25, .9] },
    'snow': { base: { cloud: .86, dark: .06, fog: .2, snow: 1, wind: .25 }, sky: ['#c6ceda', .1, 1.02] },
    'snow-heavy': { base: { cloud: .98, dark: .15, fog: .45, snow: 1.6, wind: .85 }, sky: ['#cfd6e3', .18, 1] }
  };

  // A campaign's own preset id, mapped onto one of the ids above. Covers
  // Calendaria's own 42 built-in ids (the ones this table has a real analog
  // for), so an unmodified stock calendar renders correctly by id alone.
  var PRESET_MAP = {
    'clear': 'clear', 'partly-cloudy': 'clouds-light', 'cloudy': 'clouds-heavy', 'overcast': 'clouds-overcast',
    'drizzle': 'rain', 'rain': 'rain', 'sunshower': 'rain', 'monsoon': 'rain-heavy', 'hurricane': 'rain-heavy',
    'fog': 'fog', 'mist': 'fog', 'rolling-fog': 'fog', 'windy': 'clear',
    'snow': 'snow', 'blizzard': 'snow-heavy', 'sleet': 'rain', 'ice-storm': 'hail',
    'heat-wave': 'clear', 'thunderstorm': 'storm', 'hail': 'hail', 'tornado': 'storm',
    'ashfall': 'clouds-overcast', 'sandstorm': 'clouds-overcast', 'dust-devil': 'clouds-light',
    'acid-rain': 'rain', 'blood-rain': 'rain-heavy'
  };

  function heuristicEffect(w) {
    var precip = (w.precipitation && w.precipitation.type) || '';
    var intensity = (w.precipitation && w.precipitation.intensity) || 0;
    var icon = (w.icon || '').toLowerCase();
    if (/storm|thunder/.test(icon)) return 'storm';
    if (precip === 'hail') return 'hail';
    if (precip === 'snow') return intensity > .65 ? 'snow-heavy' : 'snow';
    if (precip === 'rain' || precip === 'drizzle' || /rain/.test(icon)) return intensity > .65 ? 'rain-heavy' : 'rain';
    if (/fog|mist/.test(icon)) return 'fog';
    if (/overcast/.test(icon)) return 'clouds-overcast';
    if (/cloud/.test(icon)) return intensity > .5 ? 'clouds-heavy' : 'clouds-light';
    return 'clear';
  }

  // spec(weather): weather is the real Calendar.Weather row (or null/undefined
  // for "no weather configured"). Cached by preset id.
  var SPEC_CACHE = {};
  function spec(weather) {
    if (!weather) return { id: 'clear', effect: 'clear' };
    var id = weather.preset_id || 'clear';
    if (SPEC_CACHE[id]) return SPEC_CACHE[id];
    var effect = PRESET_MAP[id] || (EFFECTS[id] ? id : heuristicEffect(weather));
    return (SPEC_CACHE[id] = { id: id, effect: effect, label: weather.preset_label || id, icon: weather.icon, color: weather.color });
  }

  function blank() {
    return { cloud: 0, dark: 0, fog: 0, rain: 0, snow: 0, hail: 0, storm: 0, wind: .2, sky: null, fogTint: null, grey: 0, light: 1, stars: 1 };
  }
  function resolve(sp) {
    if (!sp) return null;
    var E = EFFECTS[sp.effect] || EFFECTS.clear, b = E.base || {}, R = blank();
    R.cloud = b.cloud || 0; R.dark = b.dark || 0; R.fog = b.fog || 0;
    R.rain = b.rain || 0; R.snow = b.snow || 0; R.hail = b.hail || 0; R.storm = b.storm || 0;
    R.wind = b.wind != null ? b.wind : .2;
    if (E.sky) R.sky = [E.sky[0], E.sky[1], E.sky[2]], R.light = E.sky[2];
    return R;
  }
  // blend(A, B, t): part of the way from one recipe to another (a Director's
  // weather change rolling in). Kept even though the sky pane doesn't
  // currently animate a change mid-render, so a future scrubber/day-card can
  // use it without another port.
  function blend(A, B, t) {
    if (!A) return B; if (!B) return A;
    if (t <= 0) return A; if (t >= 1) return B;
    var R = blank(), u = 1 - t;
    ['cloud', 'dark', 'fog', 'rain', 'snow', 'hail', 'storm', 'wind', 'grey', 'light', 'stars'].forEach(function (n) { R[n] = A[n] * u + B[n] * t; });
    R.sky = A.sky && B.sky ? [linHex(PAL.mixLin(PAL.hexLin(A.sky[0]), PAL.hexLin(B.sky[0]), t)), A.sky[1] * u + B.sky[1] * t, A.sky[2] * u + B.sky[2] * t] : A.sky ? [A.sky[0], A.sky[1] * u, A.sky[2]] : B.sky ? [B.sky[0], B.sky[1] * t, B.sky[2]] : null;
    return R;
  }
  // soften(R, confidence): a forecast's uncertainty dampens how far the look
  // departs from a plain clear sky.
  function soften(R, confidence) {
    if (!R) return R;
    return blend(resolve({ effect: 'clear' }), R, clamp(confidence == null ? .6 : confidence, .15, 1));
  }

  window.SkyLooks = {
    okOf: okOf, okLin: okLin, tame: tame, lit: lit, linHex: linHex, srgb: srgb, lum: lum,
    EFFECTS: EFFECTS, spec: spec, resolve: resolve, blend: blend, soften: soften, blank: blank
  };
})();
