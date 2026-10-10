/*
 * sky_world.js — day arithmetic, sun/moon spherical-astronomy positions, and
 * the OKLab palette keyed by sun height. Ported from the sky-pane design
 * contract (issue #763)'s sky-world.js/inline astronomy (SKYM/PROJ/PAL), but
 * rebuilt against Chronicle's REAL calendar data instead of the mockup's
 * Harptos sample + generator engine:
 *
 *  - Day arithmetic mirrors calendar.Calendar.AbsoluteDay (Go,
 *    internal/plugins/calendar/model.go) so a moon's phase here always
 *    agrees with the server's Moon.MoonPhase for the same absolute day.
 *  - A moon's inclination (how far its path stands from the sun's) has no
 *    real field on calendar_moons, so it is derived deterministically from a
 *    hash of the moon's id/name — stable across reloads, never invented per
 *    render. Documented deviation: the mockup's SURF sample moons hand-pick
 *    inclination; real moons don't carry one yet.
 *  - Viewing latitude has no per-calendar field either; a fixed magnitude is
 *    used, its SIGN flipped by Calendar.Hemisphere so a southern-hemisphere
 *    campaign's seasons run the opposite way, which is the one hemisphere
 *    effect the model actually exposes.
 *
 * Exposes window.SkyWorld.
 */
(function () {
  'use strict';

  var D2R = Math.PI / 180, TAU = Math.PI * 2;

  function clamp(v, a, b) { return v < a ? a : v > b ? b : v; }
  function mod(a, n) { return ((a % n) + n) % n; }
  function smooth01(a, b, x) { var t = clamp((x - a) / (b - a), 0, 1); return t * t * (3 - 2 * t); }
  function dot3(a, b) { return a[0] * b[0] + a[1] * b[1] + a[2] * b[2]; }

  // ── Day arithmetic (mirrors Calendar.AbsoluteDay server-side). ──
  function yearLength(cal) {
    var total = 0;
    (cal.months || []).forEach(function (m) { total += m.days || 0; });
    return total || 360;
  }
  // A real-world calendar that tracks real time follows the Gregorian leap
  // rule, which leap_year_every cannot express (every=4 would make 2100 a
  // leap year). Mirrors Calendar.UsesRealTime: mode alone is not enough.
  function usesRealTime(cal) { return cal.mode === 'reallife' && !!cal.tracks_real_time; }
  function isGregorianLeap(year) { return (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0; }
  function isLeapYear(cal, year) {
    if (usesRealTime(cal)) return isGregorianLeap(year);
    if (!cal.leap_year_every) return false;
    return mod(year - (cal.leap_year_offset || 0), cal.leap_year_every) === 0;
  }
  var GREGORIAN_DAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  function monthDays(cal, idx, year) {
    var m = (cal.months || [])[idx];
    if (!m) return 0;
    // Same source as Calendar.MonthDays for real-time calendars: the
    // Gregorian table, not the stored month geometry.
    if (usesRealTime(cal) && idx < 12) return GREGORIAN_DAYS[idx] + (idx === 1 && isGregorianLeap(year) ? 1 : 0);
    var d = m.days || 0;
    if (isLeapYear(cal, year)) d += m.leap_year_days || 0;
    return d;
  }
  function leapExtraDays(cal) {
    var total = 0;
    (cal.months || []).forEach(function (m) { total += m.leap_year_days || 0; });
    return total;
  }
  function leapYearsBefore(cal, year) {
    var e = cal.leap_year_every;
    if (!e || e <= 0 || year <= 0) return 0;
    var r = mod(cal.leap_year_offset || 0, e);
    if (year <= r) return 0;
    return Math.floor((year - 1 - r) / e) + 1;
  }
  // absoluteDay(cal, year, month, day): month is 1-based, matching the wire
  // format's current_month/Event.Month. Same closed form as the Go method.
  function absoluteDay(cal, year, month, day) {
    var total = 0;
    if (year > 0) {
      total = year * yearLength(cal);
      var extra = leapExtraDays(cal);
      if (extra) total += extra * leapYearsBefore(cal, year);
    }
    for (var i = 0; i < month - 1 && i < (cal.months || []).length; i++) total += monthDays(cal, i, year);
    return total + day;
  }
  // dayIndex(cal, year, month, day): the day count the calendar page reads
  // its moons on (calendar_view.js's CalDate.dayIndex), so both show one sky.
  // A real-world calendar that tracks real time counts Julian days, which
  // its real Moon is anchored to — mode alone is not enough, since a manual
  // (non-real-time) reallife calendar keeps its stored, non-Julian day count
  // server-side (Calendar.UsesRealTime); a year at or before 0 counts
  // constant-length years; any other year counts absoluteDay.
  function dayIndex(cal, year, month, day) {
    if (cal.mode === 'reallife' && cal.tracks_real_time) {
      var a = Math.floor((14 - month) / 12), yy = year + 4800 - a, mm = month + 12 * a - 3;
      return day + Math.floor((153 * mm + 2) / 5) + 365 * yy + Math.floor(yy / 4) - Math.floor(yy / 100) + Math.floor(yy / 400) - 32045;
    }
    if (year <= 0) {
      var t = year * yearLength(cal), ms = cal.months || [];
      for (var i = 0; i < month - 1 && i < ms.length; i++) t += ms[i].days || 0;
      return t + day;
    }
    return absoluteDay(cal, year, month, day);
  }
  // hour24(cal, hour, minute): the calendar's own hour/minute, rescaled onto
  // a 24-hour clock face for the astronomy formulas below (which are written
  // against a 24-hour day). A calendar with HoursPerDay != 24 just runs its
  // "hours" at a different real-time rate; the sky still turns once per day.
  function hour24(cal, hour, minute) {
    var hpd = cal.hours_per_day || 24, mpH = cal.minutes_per_hour || 60;
    var frac = (hour + (minute || 0) / mpH) / hpd;
    return frac * 24;
  }

  // ── Per-moon inclination: stable, derived, never a real field. ──
  function hash01(s) {
    s = String(s || '');
    var v = 2166136261;
    for (var i = 0; i < s.length; i++) { v ^= s.charCodeAt(i); v = (v * 16777619) >>> 0; }
    return (v >>> 0) / 4294967295;
  }
  function inclinationFor(moon) {
    var key = (moon.id != null ? moon.id : '') + ':' + (moon.name || '');
    return (hash01(key) - .5) * 24; // +/-12 degrees
  }

  // ── SKYM: sun/moon spherical astronomy, ported near-verbatim. ──
  function makeSkym(cal) {
    var LAT = ((cal.hemisphere === 'south') ? -45 : 45) * D2R;
    var EPS = 23.44 * D2R;
    var YL = yearLength(cal);
    // Winter solstice is pinned to year-day 0 (month 1, day 1): no calendar
    // field says which day is the solstice, so this is an arbitrary but
    // stable reference — it drives the SHAPE of the seasonal swing (the sky
    // still gets colder/paler in winter, warmer in summer), not any claim
    // about which named month is "really" midwinter.
    var SOLSTICE = 0;

    function dayOfYear(cal2, year, month, day) {
      var d = day;
      for (var i = 0; i < month - 1 && i < (cal2.months || []).length; i++) d += monthDays(cal2, i, year);
      return d - 1; // 0-based
    }
    function sunLon(doy, h24) { return 1.5 * Math.PI + TAU * (doy + (h24 - 12) / 24 - SOLSTICE) / YL; }
    function decOf(lon) { return Math.asin(Math.sin(EPS) * Math.sin(lon)); }
    function altAz(H, dec) {
      var sa = Math.sin(LAT) * Math.sin(dec) + Math.cos(LAT) * Math.cos(dec) * Math.cos(H);
      return { alt: Math.asin(clamp(sa, -1, 1)), az: Math.atan2(Math.sin(H), Math.cos(H) * Math.sin(LAT) - Math.tan(dec) * Math.cos(LAT)) };
    }
    function vec(alt, az) { var c = Math.cos(alt); return [Math.sin(az) * c, Math.sin(alt), Math.cos(az) * c]; }
    function sun(year, month, day, h24) {
      var doy = dayOfYear(cal, year, month, day), lon = sunLon(doy, h24), dec = decOf(lon), H = (h24 - 12) * 15 * D2R, a = altAz(H, dec);
      return { id: 'sun', alt: a.alt, az: a.az, dec: dec, H: H, lon: lon, v: vec(a.alt, a.az) };
    }
    // moon(moon, absDay, year, month, day, h24): absDay is the moon's own
    // continuous day+fraction (dayIndex + h24/24), used for its phase;
    // year/month/day/h24 place the sun that phase is measured against.
    function moon(mo, tContinuous, year, month, day, h24) {
      var p = phaseAt(mo, tContinuous), s = sun(year, month, day, h24);
      var lon = s.lon + TAU * p, incl = mo._incl != null ? mo._incl : (mo._incl = inclinationFor(mo));
      var dec = decOf(lon) + incl * D2R, H = s.H - TAU * p, a = altAz(H, dec);
      return { id: mo.id, mo: mo, p: p, lit: (1 - Math.cos(TAU * p)) / 2, alt: a.alt, az: a.az, dec: dec, H: H, v: vec(a.alt, a.az) };
    }
    return { LAT: LAT, sun: sun, moon: moon, altAz: altAz, vec: vec, dayOfYear: dayOfYear };
  }
  // phaseAt(moon, t): the SAME formula Moon.MoonPhase uses server-side
  // (frac((day+offset)/cycle)), but with t a continuous day+fraction number
  // (not an integer day) so phase — and moon position — move through the
  // night rather than jumping once at noon.
  function phaseAt(mo, t) {
    var cycle = mo.cycle_days || mo.CycleDays;
    if (!cycle || cycle <= 0) return 0;
    var offset = mo.phase_offset != null ? mo.phase_offset : mo.PhaseOffset || 0;
    var raw = (t + offset) / cycle;
    return raw - Math.floor(raw);
  }

  // ── Projection: a 270-degree panorama, centred on `az0` (the "cam"). ──
  var PROJ = {
    AZ: 270, GAM: .8,
    panorama: function (L, alt, az) {
      var a = az - (L.az0 || 0) * D2R;
      a = Math.atan2(Math.sin(a), Math.cos(a));
      var x = (a / (PROJ.AZ * D2R) + .5) * L.W, s = alt < 0 ? -1 : 1, v = Math.pow(Math.abs(alt) / (90 * D2R), PROJ.GAM);
      return { x: x, y: L.hor - s * v * (L.hor - L.top) };
    },
    at: function (L, alt, az) { return PROJ.panorama(L, alt, az); }
  };
  function moonTurn(L, m, s) {
    var d = [s.v[0] - m.v[0] * dot3(s.v, m.v), s.v[1] - m.v[1] * dot3(s.v, m.v), s.v[2] - m.v[2] * dot3(s.v, m.v)], dl = Math.hypot(d[0], d[1], d[2]);
    if (dl < 1e-6) return 0;
    var e = .02, q = [m.v[0] + d[0] / dl * e, m.v[1] + d[1] / dl * e, m.v[2] + d[2] / dl * e], ql = Math.hypot(q[0], q[1], q[2]);
    var alt2 = Math.asin(q[1] / ql), az2 = Math.atan2(q[0], q[2]), a = PROJ.at(L, m.alt, m.az), b = PROJ.at(L, alt2, az2);
    var ang = Math.atan2(b.y - a.y, b.x - a.x), rot = ang - (m.p < .5 ? 0 : Math.PI);
    rot = Math.atan2(Math.sin(rot), Math.cos(rot));
    var w = smooth01(.04, .16, Math.abs(m.p - .5));
    return clamp(rot, -1.4, 1.4) * w;
  }

  // ── PAL: the OKLab palette machinery + Painted's keyframe table, verbatim. ──
  var PAL = (function () {
    function hexLin(h) { var n = parseInt(h.slice(1), 16); return [(n >> 16) & 255, (n >> 8) & 255, n & 255].map(function (v) { v /= 255; return v <= .04045 ? v / 12.92 : Math.pow((v + .055) / 1.055, 2.4); }); }
    function linLab(c) {
      var l = Math.cbrt(.4122214708 * c[0] + .5363325363 * c[1] + .0514459929 * c[2]), m = Math.cbrt(.2119034982 * c[0] + .6806995451 * c[1] + .1073969566 * c[2]), s = Math.cbrt(.0883024619 * c[0] + .2817188376 * c[1] + .6299787005 * c[2]);
      return [.2104542553 * l + .793617785 * m - .0040720468 * s, 1.9779984951 * l - 2.428592205 * m + .4505937099 * s, .0259040371 * l + .7827717662 * m - .808675766 * s];
    }
    function labLin(L) {
      var l = L[0] + .3963377774 * L[1] + .2158037573 * L[2], m = L[0] - .1055613458 * L[1] - .0638541728 * L[2], s = L[0] - .0894841775 * L[1] - 1.291485548 * L[2];
      l = l * l * l; m = m * m * m; s = s * s * s;
      return [4.0767416621 * l - 3.3077115913 * m + .2309699292 * s, -1.2684380046 * l + 2.6097574011 * m - .3413193965 * s, -.0041960863 * l - .7034186147 * m + 1.707614701 * s].map(function (v) { return Math.max(0, v); });
    }
    function lab(h) { return linLab(hexLin(h)); }
    function mixLin(a, b, t) { return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t]; }
    function mixLab(a, b, t) { return mixLin(a, b, t); }
    function scale(a, k) { return [a[0] * k, a[1] * k, a[2] * k]; }
    function grey(a, k) { var y = .2126 * a[0] + .7152 * a[1] + .0722 * a[2]; return mixLin(a, [y, y, y], k); }
    function table(keys) {
      return keys.map(function (k) {
        var o = { alt: k.alt, dawn: {}, dusk: {} };
        ['dawn', 'dusk'].forEach(function (side) { var src = {}; for (var n in k.c) src[n] = k.c[n]; var over = k[side] || {}; for (var n2 in over) src[n2] = over[n2]; for (var n3 in src) o[side][n3] = lab(src[n3]); });
        return o;
      });
    }
    function sample(T, alt, eve) {
      var i = 0; while (i < T.length - 2 && alt > T[i + 1].alt) i++;
      var a = T[i], b = T[i + 1], t = clamp((alt - a.alt) / (b.alt - a.alt), 0, 1);
      t = t * t * (3 - 2 * t);
      var out = {};
      for (var n in a.dusk) {
        var A = mixLab(a.dawn[n], a.dusk[n], eve), B = mixLab(b.dawn[n], b.dusk[n], eve);
        out[n] = labLin(mixLab(A, B, t));
      }
      return out;
    }
    var PAINTED = table([
      { alt: -90, c: { zen: '#060a1a', mid: '#0c1430', hor: '#172347', glow: '#000000', anti: '#172347', clit: '#2a3454', cshade: '#0a0f22', fog: '#1b2544', rim: '#000000', h0: '#18213d', h1: '#11182f', h2: '#0b1124', h3: '#070b17' } },
      { alt: -16, c: { zen: '#070b1d', mid: '#0e1733', hor: '#1b284d', glow: '#000000', anti: '#1b284d', clit: '#2c3657', cshade: '#0b1024', fog: '#1c2645', rim: '#000000', h0: '#19233f', h1: '#121a31', h2: '#0c1224', h3: '#070b16' } },
      { alt: -10, dawn: { zen: '#0b1330', mid: '#17244d', hor: '#34396c', glow: '#352f5e', anti: '#28315e', clit: '#3a3b66', h0: '#262c52' }, dusk: { zen: '#0b1330', mid: '#18234b', hor: '#383668', glow: '#44305a', anti: '#28315e', clit: '#40385f', h0: '#2a2b4e' }, c: { cshade: '#0e1330', fog: '#262e55', rim: '#1c1a33', h1: '#171d3a', h2: '#10152c', h3: '#090d1c' } },
      { alt: -6, dawn: { zen: '#10214f', mid: '#26418a', hor: '#6a6aa6', glow: '#a3789e', anti: '#46538f', clit: '#806c9c', cshade: '#1f2650', fog: '#5b6394', rim: '#6f5f8c', h0: '#57578a', h1: '#383c6a', h2: '#23284a', h3: '#12162c' }, dusk: { zen: '#0f1f4a', mid: '#263d80', hor: '#7a6798', glow: '#c0707a', anti: '#46528c', clit: '#9a6680', cshade: '#221f48', fog: '#6a5f8a', rim: '#8e5f78', h0: '#5f5285', h1: '#3d3866', h2: '#252447', h3: '#13142a' } },
      { alt: -2.5, dawn: { zen: '#1b3470', mid: '#4665a8', hor: '#d8a2aa', glow: '#f3a684', anti: '#8f86bb', clit: '#f2b8a8', cshade: '#4b4c7e', fog: '#c8a2b0', rim: '#f5b49a', h0: '#a38fb2', h1: '#6f6893', h2: '#434466', h3: '#23253b' }, dusk: { zen: '#1a3068', mid: '#44609f', hor: '#e59078', glow: '#ff8a48', anti: '#9582b4', clit: '#ffa46e', cshade: '#51466f', fog: '#d6977f', rim: '#ffa061', h0: '#a8809c', h1: '#735f83', h2: '#453f5e', h3: '#241f33' } },
      { alt: 1, dawn: { zen: '#2a4b8e', mid: '#6284c2', hor: '#f3c4a8', glow: '#ffc28e', anti: '#b3a8cf', clit: '#ffe0c8', cshade: '#6f7299', fog: '#e5c7bd', rim: '#ffd0a4', h0: '#b0a8c4', h1: '#7c7fa2', h2: '#4f5776', h3: '#2c3346' }, dusk: { zen: '#29478a', mid: '#5e7fbd', hor: '#f6ac78', glow: '#ffa04e', anti: '#bb9cc4', clit: '#ffc48e', cshade: '#735f82', fog: '#e7b497', rim: '#ffb26a', h0: '#b39aae', h1: '#7e6f8f', h2: '#514d69', h3: '#2d283d' } },
      { alt: 6, c: { zen: '#3a66ad', mid: '#76a1d8', hor: '#f1d6ae', glow: '#ffd792', anti: '#c3c8e2', clit: '#fff3de', cshade: '#8f97b6', fog: '#e9e2d8', rim: '#c9b48a', h0: '#aab2c6', h1: '#7f8ca0', h2: '#5a6a68', h3: '#36453f' } },
      { alt: 14, c: { zen: '#3f71bf', mid: '#82ade3', hor: '#dbe8f1', glow: '#fff1cf', anti: '#d5e2ef', clit: '#ffffff', cshade: '#a7b6cd', fog: '#e3e9ef', rim: '#6f6a5f', h0: '#a3b6ce', h1: '#7d93a6', h2: '#56706c', h3: '#344a43' } },
      { alt: 90, c: { zen: '#3a6fc1', mid: '#7eaee6', hor: '#d5e7f4', glow: '#fffaf0', anti: '#d5e7f4', clit: '#ffffff', cshade: '#b2c2da', fog: '#e6edf3', rim: '#5f5c55', h0: '#a8bbd1', h1: '#8198ab', h2: '#5a7470', h3: '#374d45' } }
    ]);
    // Weather veils a palette: cloud greys/dims it, storm cools it toward
    // slate, fog pales it toward its own colour.
    function veil(P, w, names) {
      var k = w.cloud, dk = w.dark, fg = w.fog || 0, fogTo = P.fog || P.hor;
      names.forEach(function (n) {
        if (!P[n]) return;
        var c = grey(P[n], k * .75);
        c = scale(c, 1 - k * .28 - dk * .58);
        c = mixLin(c, [c[0] * .9, c[1] * .97, c[2] * 1.03], dk);
        if (fg > 0 && fogTo) c = mixLin(c, fogTo, fg * .62);
        P[n] = c;
      });
      if (P.glow) P.glow = scale(P.glow, 1 - k * .85);
      if (P.anti && P.hor) P.anti = mixLin(P.anti, P.hor, k);
      return P;
    }
    return { PAINTED: PAINTED, sample: sample, veil: veil, hexLin: hexLin, mixLin: mixLin, scale: scale, grey: grey, lab: lab, labLin: labLin, linLab: linLab };
  })();

  // A design colour carried into the palette at strength k: each named
  // colour keeps its own lightness (the hour's light holds) and takes the
  // tint's hue — used for a weather look's sky tint.
  function tintPalette(P, hex, k, names) {
    var t = PAL.lab(hex), tc = Math.hypot(t[1], t[2]) || 1e-6, kk = clamp(k, 0, .9);
    names.forEach(function (nm) {
      if (!P[nm]) return;
      var c = PAL.linLab(P[nm]), cc = Math.min(.12, Math.max(Math.hypot(c[1], c[2]), tc * .8));
      var to = [c[0] * (.82 + .18 * clamp(t[0] / Math.max(c[0], .05), 0, 1.5)), t[1] / tc * cc, t[2] / tc * cc];
      P[nm] = PAL.labLin([c[0] + (to[0] - c[0]) * kk, c[1] + (to[1] - c[1]) * kk, c[2] + (to[2] - c[2]) * kk]);
    });
  }
  var PALETTE_NAMES = ['zen', 'mid', 'hor', 'anti', 'clit', 'cshade', 'fog', 'h0', 'h1', 'h2', 'h3'];
  // paletteFor(altEff, eve, look, wx, dark, almanac): the painted palette for
  // one moment. `look` is a resolved SkyLooks recipe (or null for "no
  // weather known yet"); `wx` is {cloud,dark,fog,...} to veil it with.
  function paletteFor(altEff, eve, look, wx, dark, almanac) {
    var P = PAL.sample(PAL.PAINTED, altEff, eve), names = PALETTE_NAMES;
    if (look && look.sky) tintPalette(P, look.sky[0], look.sky[1] * (.6 + .4 * (1 - dark)), names);
    if (look && look.fogTint) tintPalette(P, look.fogTint[0], look.fogTint[1], ['fog']);
    if (look && look.grey) names.forEach(function (nm) { P[nm] = PAL.grey(P[nm], look.grey); });
    if (look && look.light !== 1 && look) names.concat(['glow']).forEach(function (nm) { if (P[nm]) P[nm] = PAL.scale(P[nm], look.light); });
    if (almanac) names.forEach(function (nm) { P[nm] = PAL.grey(P[nm], .4); });
    PAL.veil(P, wx, ['zen', 'mid', 'hor', 'anti', 'clit', 'cshade', 'h0', 'h1', 'h2', 'h3']);
    P.fog = PAL.mixLin(P.fog, PAL.grey(P.fog, .5), wx.cloud * .5);
    return P;
  }

  window.SkyWorld = {
    D2R: D2R, TAU: TAU, clamp: clamp, mod: mod, smooth01: smooth01, dot3: dot3,
    absoluteDay: absoluteDay, dayIndex: dayIndex, yearLength: yearLength, isLeapYear: isLeapYear, monthDays: monthDays, hour24: hour24,
    phaseAt: phaseAt, inclinationFor: inclinationFor,
    makeSkym: makeSkym, PROJ: PROJ, moonTurn: moonTurn,
    PAL: PAL, paletteFor: paletteFor, tintPalette: tintPalette
  };
})();
