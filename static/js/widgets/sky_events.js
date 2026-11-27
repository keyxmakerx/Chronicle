/*
 * sky_events.js — the day's sky events, and the camera hook that turns the
 * sky toward them (window.SkyEvents). Ported from the signed Sky Pane
 * mockup's sky-events.js.
 *
 * An event's payload (internal/plugins/calendar/model.go) names one sky
 * event. The five moon nights (blood, magic, conjunction, eclipse, harvest)
 * are `{type, moons:[ids]}` and hold at their peak while their moon is up.
 * The rest of the signed set places itself by its own fields (a compass
 * point and height, colour, strength, rate, tail, eclipse kind) and by the
 * event's own day, end day and start hour, which is its peak: comets,
 * meteor showers and storms, falling stars, aurora nights, solar eclipses,
 * blue and hunter's moons, dark nights and an owner's own shapes. A lunar
 * "eclipse" is the world's shadow crossing the named moons; a solar one is
 * its own type.
 */
(function () {
  'use strict';

  var clamp = window.SkyWorld.clamp, mod = window.SkyWorld.mod, D2R = window.SkyWorld.D2R;

  // camFor(dayEvents, moonStates): the azimuth (degrees from south, growing
  // west — SkyWorld's own convention) the sky should turn toward, given the
  // day's moon-night events. Mirrors the contract's V.face: turn toward the
  // referenced moons' spread, clamped to +/-100 degrees so the view never
  // spins past what a 270-degree panorama can show; span too wide (events on
  // opposite horizons) and it gives up and faces south (0), same as the
  // contract. moonStates: the state builder's per-moon array, each
  // {mo, m:{az radians, alt radians}, up, ...} — see sky_pane.js.
  // An event placed by a compass point (a comet, a shower's radiant, an
  // aurora, a falling star, an owner's shape) turns it toward that point; a
  // solar eclipse toward the sun (sunAz, degrees, when given).
  var PLACED = ['comet', 'meteor-shower', 'meteor-storm', 'shooting-star', 'star-fall', 'aurora', 'arcane-aurora', 'custom'];
  function camFor(dayEvents, moonStates, sunAz) {
    if (!dayEvents || !dayEvents.length) return 0;
    var az = [];
    dayEvents.forEach(function (e) {
      if (PLACED.indexOf(e.type) >= 0 && e.sky) { az.push(dirOf(e.sky.direction).az); return; }
      if (e.type === 'eclipse-solar') { if (sunAz != null) az.push(sunAz); return; }
      (e.moons || []).forEach(function (moonID) {
        var m = findMoonState(moonStates, moonID);
        if (m) az.push(m.m.az / D2R);
      });
    });
    if (!az.length) return 0;
    var LIM = 100, lo = Math.min.apply(null, az.concat([0])), hi = Math.max.apply(null, az.concat([0]));
    if (hi - lo > 2 * LIM) return 0;
    return lo < -LIM ? lo + LIM : hi > LIM ? hi - LIM : 0;
  }
  function findMoonState(moonStates, moonID) {
    for (var i = 0; i < moonStates.length; i++) if (moonStates[i].mo && moonStates[i].mo.id === moonID) return moonStates[i];
    return null;
  }

  // parseDayEvents(events): events is the real /events list (already
  // viewer-filtered server-side — never re-derive visibility here). Returns
  // the subset that carry a recognized payload, each as
  // {type, moons:[ids], sky:<its fields, checked>, raw: <the Event>}.
  var MOON_TYPES = ['blood', 'magic', 'conjunction', 'eclipse', 'harvest'];
  var SKY_TYPES = ['comet', 'meteor-shower', 'meteor-storm', 'shooting-star', 'star-fall', 'aurora', 'arcane-aurora',
    'eclipse-solar', 'blue-moon', 'hunters-moon', 'dark-night', 'custom'];
  var TYPES = MOON_TYPES.concat(SKY_TYPES);
  var ALT = { low: 9, mid: 32, high: 62 }, DEG = { N: 0, NE: 45, E: 90, SE: 135, S: 180, SW: 225, W: 270, NW: 315 };
  var SHAPES = ['streak', 'glow', 'ring', 'star', 'doorway', 'comet', 'curtain', 'veil'];
  var HEX = /^#[0-9a-fA-F]{6}$/;
  // The server checks a payload when it is saved; this keeps an old or
  // hand-made one from handing the painter a value it can't draw.
  function num(v, lo, hi) { return typeof v === 'number' && isFinite(v) ? clamp(v, lo, hi) : null; }
  function skyOf(p) {
    var d = p.direction && typeof p.direction === 'object' ? p.direction : null;
    return {
      direction: { compass: d && DEG[d.compass] != null ? d.compass : 'S', altitude: d && ALT[d.altitude] ? d.altitude : 'mid' },
      color: HEX.test(p.color || '') ? p.color : null,
      colors: Array.isArray(p.colors) ? p.colors.filter(function (c) { return HEX.test(c || ''); }).slice(0, 2) : [],
      intensity: num(p.intensity, 0, 1), rate: num(p.rate_per_hour, 1, 2000), moonlit: p.moonlit === true,
      tail: num(p.tail_degrees, 1, 90), subtype: ['total', 'annular', 'partial'].indexOf(p.subtype) >= 0 ? p.subtype : 'total',
      magnitude: num(p.magnitude, .1, .98), minutes: num(p.duration_minutes, 1, 600), hours: num(p.duration_hours, .25, 24),
      shape: SHAPES.indexOf(p.shape) >= 0 ? p.shape : 'glow', size: ['small', 'medium', 'large'].indexOf(p.size) >= 0 ? p.size : 'medium'
    };
  }
  function parseDayEvents(events) {
    var out = [];
    (events || []).forEach(function (e) {
      if (!e.payload) return;
      var p;
      try { p = JSON.parse(e.payload); } catch (err) { return; }
      if (!p || TYPES.indexOf(p.type) < 0) return;
      out.push({ type: p.type, moons: Array.isArray(p.moons) ? p.moons : [], sky: skyOf(p), raw: e });
    });
    return out;
  }

  // A compass point and a height as a direction in the sky: altitude and
  // azimuth in degrees, azimuth from south, growing westward.
  function dirOf(d) { return { alt: ALT[d.altitude] || 32, az: mod(DEG[d.compass] != null ? DEG[d.compass] : 180, 360) - 180 }; }

  // overlaysFor(dayEvents, moonStates): visual hints for sky_2d's drawn
  // layer. moonStates: the array the state builder placed this render
  // (each {mo, x, y, r, up, ...}), so overlays can be skipped/adjusted for a
  // moon that isn't currently above the horizon.
  function overlaysFor(dayEvents, moonStates) {
    var out = { blood: {}, eclipse: {}, harvest: {}, conjGroups: [], magic: false };
    (dayEvents || []).forEach(function (e) {
      var ups = (e.moons || []).map(function (id) { return findMoonState(moonStates, id); }).filter(function (m) { return m && m.up; });
      if (e.type === 'blood') ups.forEach(function (m) { out.blood[m.mo.id] = true; });
      else if (e.type === 'eclipse') ups.forEach(function (m) { out.eclipse[m.mo.id] = .78; });
      else if (e.type === 'harvest' || e.type === 'hunters-moon') ups.forEach(function (m) { out.harvest[m.mo.id] = true; });
      else if (e.type === 'conjunction' && ups.length > 1) out.conjGroups.push(ups.map(function (m) { return m.mo.id; }));
      else if (e.type === 'magic') out.magic = true;
    });
    return out;
  }


  // ── The painted sky's moving events (sky_gl.js reads them; the basic sky
  // keeps its own soft overlays). apply(st, dayEvents) fills, on the state
  // the pane built: st.met (meteors: head, tail, colour, strength), st.aur
  // (aurora curtains), st.comets, st.airs, st.darkNight, st.bleed (the
  // bleeding blood moon), st.shadows (the world's shadow on a moon, by moon
  // slot) and st.fxItems (drawn shapes). The real payload says which moons
  // and nothing about timing, so a moon-night event holds at its peak for
  // as long as its moon is up. ──
  function hashN(n) { var x = Math.sin(n * 127.1 + 311.7) * 43758.5453; return x - Math.floor(x); }
  function h(i, k) { var x = Math.sin(i * 127.1 + k * 311.7 + 7.7) * 43758.5453; return x - Math.floor(x); }
  function smooth01(a, b, x) { return window.SkyWorld.smooth01(a, b, x); }
  function hexLin(hex) { return window.SkyWorld.PAL.hexLin(hex); }
  function lit(hex, role, sl) { return window.SkyLooks.lit(hex, role, sl); }
  function night(st) { return smooth01(.15, .75, st.dark); }
  function mix3(a, b, t) { return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t]; }
  // How much of a disc of radius a is hidden by a disc of radius b whose
  // centre is d away.
  function discCover(a, b, d) {
    if (d >= a + b) return 0;
    if (d <= Math.abs(b - a)) return b >= a ? 1 : (b * b) / (a * a);
    var a2 = a * a, b2 = b * b, x = (d * d + a2 - b2) / (2 * d), y = Math.sqrt(Math.max(0, a2 - x * x));
    var area = a2 * Math.acos(clamp(x / a, -1, 1)) + b2 * Math.acos(clamp((d - x) / b, -1, 1)) - d * y;
    return clamp(area / (Math.PI * a2), 0, 1);
  }

  // The meteor plan, made from a seed, so any moment can be drawn again exactly.
  var MP = [];
  function metPlan(k) {
    while (MP.length <= k) {
      var j = MP.length, prev = MP[j - 1], t0 = prev ? prev.t0 + .1 + Math.pow(hashN(j * 1.7 + .2), 1.6) * .22 : .3;
      var fire = hashN(j * 4.9 + 3) < .045, c = hashN(j * 6.1 + 2), side = hashN(j * 1.3 + 8) < .5 ? 0 : Math.PI, tilt = (hashN(j * 2.9 + 5) - .3) * 1.05;
      MP.push({ t0: t0, ang: side === 0 ? tilt : Math.PI - tilt, d0: .12 + hashN(j * 3.3 + 1) * .9, len: (.28 + hashN(j * 7.7) * .45) * (fire ? 1.5 : 1),
        dur: (.5 + hashN(j * 8.3 + 4) * .6) * (fire ? 1.9 : 1), mag: fire ? 2.4 : .55 + Math.pow(hashN(j * 5.3 + 6), 2.5) * 1.3,
        col: c < .68 ? [1, .92, .78] : c < .86 ? [.55, 1, .7] : [1, .62, .35] });
      MP[j].train = Math.min(1, MP[j].mag) * (fire ? 1.6 : 1);
    }
    return MP[k];
  }
  function pushMeteor(st, mp, age, rp, col, gain) {
    var H = st.H, L = st.L, TRAIN = 2.2, u = age / mp.dur, dx = Math.cos(mp.ang), dy = Math.sin(mp.ang), d0 = mp.d0 * H, ln = mp.len * H;
    if (u > 1) {
      var v = (age - mp.dur) / (TRAIN * mp.train), a0 = d0 + ln * .3, a1 = d0 + ln * .92;
      // The train has faded out; past here its fade would be a fractional power of a negative number.
      if (v >= 1) return;
      if (rp.y + dy * a1 > L.hor + 4 && rp.y + dy * a0 > L.hor + 4) return;
      st.met.push([rp.x + dx * a1, rp.y + dy * a1, rp.x + dx * a0, rp.y + dy * a0, col[0], col[1], col[2], -Math.pow(1 - v, 1.6) * .5 * mp.train * gain]);
      return;
    }
    var e2 = 1 - Math.pow(1 - u, 1.6), hx = rp.x + dx * (d0 + ln * e2), hy = rp.y + dy * (d0 + ln * e2);
    var tr = ln * Math.min(.55, .15 + u * .8), tx = hx - dx * tr, ty = hy - dy * tr;
    if (hy > L.hor + 4 && ty > L.hor + 4) return;
    var env = smooth01(0, .12, u) * (1 - smooth01(.7, 1, u));
    st.met.push([hx, hy, tx, ty, col[0], col[1], col[2], env * mp.mag * gain]);
  }
  // Fireballs from a look (meteor weather): now and then, never two at once.
  function fireballs(st, f, gain) {
    var tt = st.rm ? 2.2 : st.t, per = clamp(1 / Math.max(.02, f.rate), 3, 30), cyc = Math.floor(tt / per), u = (tt - cyc * per) / 1.8;
    if (u > 1.4) return;
    var x0 = st.W * (.12 + .76 * h(cyc, 1.7)), y0 = st.H * (.08 + .25 * h(cyc, 2.9)), dirX = h(cyc, 4.1) < .5 ? -1 : 1, len = st.W * .26, ang = .38 + .2 * h(cyc, 5.3);
    var e2 = 1 - Math.pow(1 - Math.min(u, 1), 1.4), hx = x0 + dirX * len * Math.cos(ang) * e2, hy = y0 + len * Math.sin(ang) * e2, col = hexLin(f.c);
    var g = gain * (1 - st.wx.cloud * .85) * (u <= 1 ? smooth01(0, .08, u) : Math.max(0, 1 - (u - 1) / .4));
    st.met.push([hx, hy, hx - dirX * len * .32 * Math.cos(ang), hy - len * .32 * Math.sin(ang), col[0], col[1], col[2], 2.2 * g]);
    var pts = [];
    for (var i = 1; i < 7; i++) { var q = e2 - i * .05; if (q > 0) pts.push([x0 + dirX * len * Math.cos(ang) * q + (h(i, cyc) - .5) * 4, y0 + len * Math.sin(ang) * q + i * 1.4, 1 + h(i, 3) * 1.4, .55 * g * (1 - i / 7)]); }
    st.fxItems.push({ shape: 'embers', pts: pts, col: lit(f.c, 'glow', st.sl), k: g });
  }
  // Meteor weather: a steady shower between the fireballs.
  function lookShower(st, f) {
    var dk = st.dark * (1 - st.wx.cloud * .9); if (dk <= .05) return;
    var rp = { x: st.W * .6, y: st.H * .06 }, tt = st.rm ? 9.5 : st.t, keep = clamp(f.rate * 2.4, .2, .6), col = hexLin(f.c);
    for (var j = 0; ; j++) {
      var mp = metPlan(j); if (mp.t0 > tt) break;
      var age = tt - mp.t0; if (age > mp.dur + 2.4 * mp.train || st.met.length >= 8) continue;
      if (h(j, 5.7) > keep) continue;
      pushMeteor(st, mp, age, rp, mix3(mp.col, col, .35), dk);
    }
  }
  // Now and then on a clear night a faint shooting star; rare, short, quiet.
  function ambient(st) {
    if (st.dark < .7 || st.wx.cloud > .35 || st.met.length || st.rm) return;
    var tt = st.t, per = 23, cyc = Math.floor(tt / per), off = h(cyc, 9.1) * (per - 2), age = tt - cyc * per - off;
    if (age < 0 || age > 1.4 || h(cyc, 7.3) < .25) return;
    var rp = { x: st.W * (.15 + .7 * h(cyc, 1.1)), y: st.H * (.02 + .1 * h(cyc, 2.2)) };
    pushMeteor(st, { ang: (h(cyc, 3.3) < .5 ? .5 : Math.PI - .5), d0: .1, len: .32, dur: .7, mag: .55, train: .4, col: [1, .95, .85] }, age, rp, [1, .95, .85], .8 * (st.dark - .6));
  }
  // The world's shadow on a moon, at the event's peak. A blood moon sits in
  // totality and turns to congealed blood, and the sky bleeds; an eclipse
  // is a deep partial one.
  function lunar(st, M, blood) {
    var Ru = M.r * 2.6, mag = .78, miss = blood ? Ru * .15 : Ru + M.r - 2 * mag * M.r;
    // A moon past the painter's four slots has no shadow uniform to carry.
    if (M.i >= 0) st.shadows[M.i] = [0, miss, Ru, .93];
    M.shadowCover = discCover(M.r, Ru, miss);
    if (blood) {
      var total = clamp((Ru - M.r - miss) / (M.r * .35) + .5, 0, 1);
      if (total > 0) { M.blood = true; M.bloodK = total; if (!st.bleed && M.i >= 0) st.bleed = { M: M, k: total, color: '#b8472f' }; }
    }
  }
  // ── When an event is in the sky. Times are the calendar's continuous day
  // count (st.tNow: day index + hour / 24). The event's own day and end day
  // bound it; its start hour is its peak. A repeating event's day is today. ──
  var PEAK = { 'meteor-shower': 23, 'meteor-storm': 23, 'shooting-star': 21, 'star-fall': 21, comet: 19.6, 'eclipse-solar': 12, custom: 21 };
  function when(st, e) {
    var cal = st.cal; if (!cal || st.tNow == null) return null;
    var raw = e.raw || {}, n0 = Math.floor(st.tNow), n1 = n0;
    if (!raw.is_recurring && raw.year != null && raw.month != null && raw.day != null) {
      n0 = window.SkyWorld.dayIndex(cal, raw.year, raw.month, raw.day);
      n1 = raw.end_year != null && raw.end_month != null && raw.end_day != null ? Math.max(n0, window.SkyWorld.dayIndex(cal, raw.end_year, raw.end_month, raw.end_day)) : n0;
    }
    var hasHour = raw.start_hour != null && !raw.all_day, hp = hasHour ? window.SkyWorld.hour24(cal, raw.start_hour, raw.start_minute || 0) : (PEAK[e.type] != null ? PEAK[e.type] : 21);
    return { n0: n0, n1: n1, hp: hp, tp: n0 + hp / 24, hasHour: hasHour };
  }
  // Hours on each side of an eclipse's peak: a solar one takes the moon a
  // little over an hour to cross; a partial one as long as it says.
  function span(e) { return e.sky.subtype === 'partial' ? (e.sky.minutes || 60) / 120 : 1.2; }
  // The stretch of time an event can be seen in, [from, to], in days.
  function windowOf(e, w) {
    var t = e.type, H = 1 / 24;
    // From the afternoon before its first peak to the morning after its last.
    if (t === 'meteor-shower') return [w.tp - .4, w.n1 + (w.tp - w.n0) + .4];
    if (t === 'meteor-storm') return [w.tp - 5 * H, w.tp + 5 * H];
    if (t === 'comet') return [w.n0 - .2, w.n1 + 1];
    if (t === 'eclipse-solar') return [w.tp - (span(e) + .1) * H, w.tp + (span(e) + .1) * H];
    if ((t === 'shooting-star' || t === 'star-fall') && w.hasHour) return [w.tp - 10 / 1440, w.tp + 25 / 1440];
    if (t === 'custom' && w.hasHour) return [w.tp - .25 * H, w.tp + (e.sky.hours || 2) * H];
    // Everything else holds for its days and the night after the last.
    return [w.n0, w.n1 + 1.3];
  }
  // How strongly an event shows at this moment, before darkness and weather:
  // 0 outside its window. With no calendar to place it (a test, a preview)
  // it holds at its strength.
  function strength(st, e) {
    var s = e.sky, k = s.intensity != null ? s.intensity : 1, w = when(st, e);
    if (!w) return clamp(k, .4, 1);
    var win = windowOf(e, w), T = st.tNow, t = e.type, hr = (T - Math.floor(T)) * 24;
    if (T < win[0] || T > win[1]) return 0;
    if (t === 'meteor-shower') { var dd = T - w.tp; return clamp(k * 1.2, .35, 1) * Math.exp(-dd * dd / (2 * 1.1 * 1.1)); }
    if (t === 'meteor-storm') { var dh = (T - w.tp) * 24; return Math.exp(-dh * dh / (2 * 1.8 * 1.8)); }
    if (t === 'comet') {
      var pk = (w.n0 + w.n1) / 2 + .8, wd = Math.max(2, (w.n1 - w.n0 + 1) / 3), x = (T - pk) / wd;
      return clamp(k, .3, 1) * (.3 + .7 * Math.exp(-.5 * x * x)) * smooth01(win[0], win[0] + 2, T) * (1 - smooth01(win[1] - 2, win[1], T));
    }
    if (t === 'aurora' || t === 'arcane-aurora') { var hn = mod(hr - 12, 24) - 11.5; return clamp(k * 1.1, .3, 1) * (.45 + .55 * Math.exp(-hn * hn / (2 * 3.2 * 3.2))); }
    if (t === 'custom' && w.hasHour) { var fade = Math.min(.6, (s.hours || 2) * .3) / 24; return clamp(k * 1.15, .5, 1) * smooth01(win[0], w.tp, T) * (1 - smooth01(win[1] - fade, win[1], T)); }
    return clamp(k, .4, 1);
  }
  // The sidereal time at day T, from now's: the stars turn once a day and a
  // little more, as the sun moves along its year.
  function lstAt(st, T) {
    var yl = st.cal ? window.SkyWorld.yearLength(st.cal) || 365 : 365;
    return (st.sidereal || 0) * D2R + (T - (st.tNow || 0)) * 2 * Math.PI * (1 + 1 / yl);
  }
  // A point among the stars that stood at `d` at day T0, where it is now
  // (drifting `drift` degrees a day, for a comet); without the sky model, it
  // stays at d.
  function amongStars(st, d, T0, drift) {
    var q = dirOf(d);
    if (!st.skym || st.tNow == null) return { alt: q.alt * D2R, az: q.az * D2R, v: vecOf(q.alt * D2R, q.az * D2R) };
    var f = st.skym.fixAt(q.alt * D2R, q.az * D2R, lstAt(st, T0)), days = drift ? st.tNow - T0 : 0;
    return st.skym.starAt(f.ra + days * .9 * D2R * drift, f.dec + days * .25 * D2R * drift, (st.sidereal || 0) * D2R);
  }
  function vecOf(alt, az) { var c = Math.cos(alt); return [Math.sin(az) * c, Math.sin(alt), Math.cos(az) * c]; }
  function place(st, d, jitter) { var q = dirOf(d), j = jitter || 0; return window.SkyWorld.PROJ.at(st.L, (q.alt + j * 3) * D2R, (q.az + j * 6) * D2R); }
  // The screen angle a comet's tail takes: straight away from the sun, along the sky.
  function awayFromSun(L, c, sun) {
    var m = c.v, s = sun && sun.v ? sun.v : [0, -1, 0], k = m[0] * s[0] + m[1] * s[1] + m[2] * s[2], d = [m[0] * k - s[0], m[1] * k - s[1], m[2] * k - s[2]], dl = Math.hypot(d[0], d[1], d[2]) || 1, ep = .02;
    var q = [m[0] + d[0] / dl * ep, m[1] + d[1] / dl * ep, m[2] + d[2] / dl * ep], ql = Math.hypot(q[0], q[1], q[2]);
    var a = window.SkyWorld.PROJ.at(L, c.alt, c.az), b = window.SkyWorld.PROJ.at(L, Math.asin(q[1] / ql), Math.atan2(q[0], q[2]));
    return Math.atan2(b.y - a.y, b.x - a.x);
  }
  function keyHash(s) { var v = 0; s = String(s); for (var i = 0; i < s.length; i++) v = (v * 31 + s.charCodeAt(i)) % 100003; return v; }
  function deeper(hex) { var L2 = window.SkyLooks, o = L2.okOf(hex); return L2.linHex(L2.okLin(Math.max(.35, o.L - .25), Math.max(o.C, .08), o.h + .9)); }
  function sunDeg(st) { return st.altSun != null ? st.altSun : st.altEff != null ? st.altEff : (st.alt || 0) / D2R; }

  // Before the moons are placed: a solar eclipse slides its moon across the
  // sun. The moon's size against the sun sets how long totality lasts; an
  // annular one's moon is small enough to leave a ring; a partial one passes
  // off-centre by its magnitude. Sets st.ecl (how much of the sun is
  // covered), st.eclSky (how dark that makes the sky) and st.eclMoon.
  function pre(st, dayEvents) {
    st.ecl = 0; st.eclSky = 0; st.eclMoon = null;
    (dayEvents || []).forEach(function (e) {
      if (e.type !== 'eclipse-solar' || !st.sp) return;
      var w = when(st, e); if (!w) return;
      // The moon that crosses must be in this sky: one hidden from players
      // would leave their noon dark with the sun uncovered.
      var ms = (st.cal && st.cal.moons) || [];
      if (!ms.length || ((e.moons || []).length && !ms.some(function (m) { return e.moons.indexOf(m.id) >= 0; }))) return;
      var s = e.sky, sp2 = span(e), u = (st.tNow - w.tp) * 24 / sp2;
      if (Math.abs(u) >= 1) return;
      var sr = st.R * .95, dur = (s.minutes || 4) / 60, q = clamp(dur * 1.02 / (2 * sp2), 0, .2), mr = sr, miss = 0;
      if (s.subtype === 'total') mr = sr * (1 + q) / (1 - q);
      else if (s.subtype === 'annular') mr = sr * .85;
      else miss = sr + mr - 2 * (s.magnitude != null ? s.magnitude : .6) * sr;
      var along = u * (sr + mr) * 1.02, th = -.22;
      var x = st.sp.x - along * Math.cos(th) - miss * Math.sin(th), y = st.sp.y - along * Math.sin(th) + miss * Math.cos(th);
      var crossing = (e.moons || []).filter(function (id) { return ms.some(function (m) { return m.id === id; }); })[0];
      st.eclMoon = { x: x, y: y, r: mr, id: crossing != null ? crossing : null, type: s.subtype };
      st.ecl = discCover(sr, mr, Math.hypot(x - st.sp.x, y - st.sp.y));
      // The drawn ring is wider than a real one so it reads; the light dims as much as a real one's does.
      st.eclSky = s.subtype === 'annular' ? Math.min(1, st.ecl * 1.45) : st.ecl;
    });
  }

  // Meteors run straight away from their radiant, which stands at the
  // event's compass point at its peak and turns with the stars. The rate sets
  // how often one comes, the strength how bright, the colour theirs; a
  // moonlit night hides the faint ones.
  function shower(st, e, k) {
    var s = e.sky, w = when(st, e), rad = amongStars(st, s.direction, w ? w.tp : 0, 0), rp = window.SkyWorld.PROJ.at(st.L, rad.alt, rad.az);
    var dk = st.dark * (1 - st.wx.cloud * .9), act = k * smooth01(-8, 12, rad.alt / D2R) * dk;
    if (act <= .01) return;
    var rate = s.rate || (e.type === 'meteor-storm' ? 600 : 40), gap = clamp(34 / Math.pow(rate, .72), .14, 5), keep = clamp(.2 / gap, 0, 1) * act, tt = st.rm ? 9.5 : st.t;
    var col = hexLin(s.color || '#fff1c9'), moonK = s.moonlit ? .45 : 0;
    for (var j = 0; ; j++) {
      var mp = metPlan(j); if (mp.t0 > tt) break;
      var age = tt - mp.t0; if (age > mp.dur + 2.4 * mp.train || st.met.length >= 8) continue;
      if (h(j, 3.3) > keep) continue;
      if (mp.mag < .8 && h(j, 4.4) < moonK) continue;
      pushMeteor(st, mp, age, rp, mix3(mp.col, col, .55), (s.intensity != null ? .6 + .5 * s.intensity : 1) * dk);
    }
  }
  // One fireball: from its compass point a bright head with a long train
  // crosses a third of the sky in about a second and a half, and comes round
  // again every few seconds while it is on, so it can be found. A falling
  // star runs down to the horizon and the place it came down glows.
  function fireball(st, e, k) {
    var s = e.sky, land = e.type === 'star-fall', per = 7.5, tt = st.rm ? 1.05 : st.t, cyc = Math.floor(tt / per), u = (tt - cyc * per) / 1.6;
    var p = place(st, s.direction), dirX = p.x > st.W / 2 ? -1 : 1, W = st.W, H = st.H;
    var ang = land ? .62 : .32, len = W * .34, sx = p.x - dirX * len * .5 * Math.cos(ang), sy = p.y - len * .5 * Math.sin(ang) - H * .08;
    var ex = sx + dirX * len * Math.cos(ang), ey = land ? st.L.hor + 2 : sy + len * Math.sin(ang);
    if (land) { var fall = (ey - sy) / Math.sin(ang); ex = sx + dirX * fall * Math.cos(ang); }
    var gain = clamp(k, .5, 1) * (.35 + .65 * night(st)) * (1 - st.wx.cloud * .85), col = hexLin(s.color || '#fff1c9');
    if (u >= 0 && u <= 1) {
      var e2 = 1 - Math.pow(1 - u, 1.4), hx = sx + (ex - sx) * e2, hy = sy + (ey - sy) * e2, tr = .28 + .25 * u;
      st.met.push([hx, hy, hx - (ex - sx) * tr, hy - (ey - sy) * tr, col[0], col[1], col[2], 2.6 * gain * smooth01(0, .06, u) * (land ? 1 : 1 - smooth01(.82, 1, u))]);
      var pts = []; for (var i = 1; i < 6; i++) { var q = e2 - i * .045; if (q > 0) pts.push([sx + (ex - sx) * q + (h(i, cyc) - .5) * 5, sy + (ey - sy) * q + (h(i, cyc + 1) - .5) * 5 + i * 1.2, 1.2 + h(i, 2) * 1.2, .5 * gain * (1 - i / 6)]); }
      st.fxItems.push({ shape: 'embers', pts: pts, col: lit(s.color || '#ffcf8a', 'glow', st.sl), k: gain });
    } else if (u > 1 && !land) {
      var v = (u - 1) * 1.6 / 4.5;
      if (v < 1) st.met.push([ex, ey, sx + (ex - sx) * .35, sy + (ey - sy) * .35, col[0], col[1], col[2], -Math.pow(1 - v, 1.6) * .6 * gain]);
    }
    if (land && u > .95) {
      var g = Math.exp(-(u - 1) * 1.6 / 2.2) * gain;
      st.fxItems.push({ shape: 'landing', x: ex, y: st.L.hor - 2, col: lit(s.color || '#ffcf8a', 'glow', st.sl), hi: lit('#fff4dc', 'glow', st.sl), k: clamp(g, 0, 1) });
    }
  }
  // A comet: its head where the compass point puts it on its first evening,
  // drifting a little among the stars night by night; its tail as long as
  // tail_degrees, away from the sun. By day a pale streak at a quarter of
  // its night strength.
  function comet(st, e, k) {
    var s = e.sky, w = when(st, e), T0 = w ? w.n0 + (w.hasHour ? w.hp + .6 : 19.6) / 24 : 0, c = amongStars(st, s.direction, T0, 1);
    if (c.alt < -3 * D2R) return;
    var hp = window.SkyWorld.PROJ.at(st.L, c.alt, c.az), day = .25 + .75 * smooth01(-2, -12, sunDeg(st)), len = st.H * (.25 + .9 * clamp((s.tail || 25) / 55, .15, 1.2));
    st.comets.push({ x: hp.x, y: hp.y, ang: awayFromSun(st.L, c, st.sun), len: len, k: k * day * (1 - st.wx.cloud * .9) * smooth01(-3, 2, c.alt / D2R), w: 1, day: day, c: s.color || '#d6e8ff' });
  }
  // An owner's shape, placed by its compass point and height, sized,
  // coloured in its band, and fainter by day.
  function custom(st, e, k, seed) {
    var s = e.sky, p = place(st, s.direction, (h(seed, 1) - .5) * .4), dayK = .3 + .7 * night(st), cloudK = 1 - st.wx.cloud * .8;
    var col = lit(s.color || '#d8e8ff', 'glow', st.sl), hi = col.map(function (v) { return Math.min(1, v * 1.35 + .06); }), kk = k * dayK * cloudK;
    // Without an hour a shape holds all day, open, and a wandering star
    // stands an hour along its path rather than a day.
    var w = when(st, e), timed = !!(w && w.hasHour), elapsed = timed ? (st.tNow - w.tp) * 24 : 1, SZ = window.SkyFX.SIZE || {}, sz = SZ[s.size] || 1, q = dirOf(s.direction);
    if (s.shape === 'comet') { var c = { alt: q.alt * D2R, az: q.az * D2R, v: vecOf(q.alt * D2R, q.az * D2R) }; st.comets.push({ x: p.x, y: p.y, ang: awayFromSun(st.L, c, st.sun), len: st.H * .5 * sz, k: kk, w: sz, day: 1, c: s.color || '#d8e8ff' }); return; }
    if (s.shape === 'curtain') { st.aur.push({ k: kk * 1.2, c0: s.color || '#9ff2d2', c1: deeper(s.color || '#9ff2d2'), az: q.az, span: sz > 1.2 ? 75 : sz < .8 ? 28 : 48, base: { low: .05, mid: .2, high: .38 }[s.direction.altitude] || .2, seed: seed % 89, own: true }); return; }
    if (s.shape === 'veil') { var cx = p.x / st.W, v = (st.L.hor - p.y) / Math.max(1, st.L.hor - st.L.top); st.airs.push({ t: 'veil', c: s.color || '#d8c8f0', k: kk * .9, sx: .01, sy: .02, v: [v - .45 * sz, v + .35 * sz], d: 0, cx: cx, hw: .14 * sz }); return; }
    st.fxItems.push({ shape: s.shape, x: p.x, y: p.y, sz: sz, size: s.size, col: col, hi: hi, k: kk, seed: seed % 101, elapsed: Math.max(0, elapsed), open: timed ? smooth01(0, .3, Math.max(0, elapsed)) : 1 });
  }
  // The signed sky events that aren't moon nights, at this moment.
  function skyEvents(st, dayEvents) {
    (dayEvents || []).forEach(function (e) {
      if (MOON_TYPES.indexOf(e.type) >= 0) return;
      var s = e.sky, k = strength(st, e), ty = e.type, seed = keyHash((e.raw && e.raw.id) || ty);
      if (k <= 0) return;
      if (ty === 'meteor-shower' || ty === 'meteor-storm') shower(st, e, k);
      else if (ty === 'shooting-star' || ty === 'star-fall') fireball(st, e, k);
      else if (ty === 'comet') comet(st, e, k);
      else if (ty === 'aurora' || ty === 'arcane-aurora') {
        var q = dirOf(s.direction), def = ty === 'arcane-aurora' ? ['#d08cff', '#6ad8ff'] : ['#bff2cf'], cols = s.colors.length ? s.colors : s.color ? [s.color] : def, high = s.direction.altitude === 'high';
        st.aur.push({ k: k, c0: cols[0], c1: cols[1] || deeper(cols[0]), az: q.az, span: high ? 150 : 85, base: high ? .26 : .08, seed: seed % 97, arcane: ty === 'arcane-aurora' });
      }
      else if (ty === 'blue-moon') (e.moons || []).forEach(function (id) {
        // A ring of light 22 degrees round the moon, a little bluer inside.
        var M = findMoonState(st.moons, id);
        if (!M || !M.up || M.m.alt <= 0) return;
        var up = window.SkyWorld.PROJ.at(st.L, M.m.alt + 22 * D2R, M.m.az), R22 = Math.max(M.r * 3, Math.abs(up.y - M.y));
        st.fxItems.push({ shape: 'halo', x: M.x, y: M.y, r: R22, col: lit('#d6e6ff', 'glow', st.sl), hi: lit('#eef4ff', 'glow', st.sl), k: (s.intensity != null ? s.intensity : .5) * 1.6 * night(st) * (1 - st.wx.cloud * .6), still: true });
      });
      else if (ty === 'dark-night') st.darkNight = Math.max(st.darkNight, k * night(st));
      else if (ty === 'custom') custom(st, e, k, seed);
    });
  }

  function apply(st, dayEvents) {
    st.met = []; st.aur = []; st.comets = []; st.airs = []; st.darkNight = 0; st.bleed = null; st.shadows = {}; st.fxItems = st.fxItems || [];
    var look = st.look;
    (look ? look.aurora : []).forEach(function (a) { st.aur.push({ k: a.k, c0: a.c0, c1: a.c1, az: 0, span: a.span || 170, base: a.alt === 'high' ? .3 : a.alt === 'low' ? .08 : .16, seed: 1 }); });
    (look ? look.fire : []).forEach(function (f) { fireballs(st, f, night(st) * .85 + .15); lookShower(st, f); });
    // The shapes a weather draws into the sky: a rainbow, a halo, a rift.
    if (window.SkyFX.lookItems) window.SkyFX.lookItems(st);
    (dayEvents || []).forEach(function (e) {
      var ms = (e.moons || []).map(function (id) { return findMoonState(st.moons, id); }).filter(function (M) { return M && M.up; });
      if (e.type === 'blood') ms.forEach(function (M) { lunar(st, M, true); });
      else if (e.type === 'eclipse') ms.forEach(function (M) { lunar(st, M, false); });
      else if (e.type === 'conjunction' && ms.length > 1) ms.forEach(function (M) { M.conj = .8; });
      else if (e.type === 'harvest' || e.type === 'hunters-moon') ms.forEach(function (M) {
        // Warm and large near the hills, where it is watched rising; a hunter's moon a deeper orange.
        var low = smooth01(24, 2, M.m.alt / D2R), c = (e.sky && e.sky.color) || (e.type === 'hunters-moon' ? '#ffb36a' : '#ffd58a'), k = e.sky && e.sky.intensity != null ? e.sky.intensity : .7;
        M.warm = [c, k * (.45 + .55 * low)];
        st.fxItems.push({ shape: 'warm', x: M.x, y: M.y, r: M.r, col: lit(c, 'glow', st.sl), k: k * (.3 + .7 * low) * night(st) });
      });
    });
    skyEvents(st, dayEvents);
    ambient(st);
  }

  window.SkyEvents = { camFor: camFor, parseDayEvents: parseDayEvents, overlaysFor: overlaysFor, pre: pre, apply: apply, strength: strength, metPlan: metPlan, discCover: discCover, TYPES: TYPES, SKY_TYPES: SKY_TYPES };
})();
