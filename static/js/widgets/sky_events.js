/*
 * sky_events.js — the day's sky events, and the camera hook that turns the
 * sky toward them (window.SkyEvents). Ported from the design contract's
 * sky-events.js.
 *
 * The real Event.Payload (MoonNightPayload, internal/plugins/calendar/
 * model.go) is `{type, moons:[ids]}`: five types (blood, magic, conjunction,
 * eclipse, harvest) naming the moons they concern and nothing else, so an
 * event holds at its peak while its moon is up. The mockup's comets, meteor
 * showers, aurora and custom shapes need richer payloads the server doesn't
 * send yet; the look's own aurora and fireballs (a weather) and the odd
 * shooting star on a clear night are drawn. The payload doesn't say solar or
 * lunar, so every eclipse is the world's shadow crossing the named moons.
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
  function camFor(dayEvents, moonStates) {
    if (!dayEvents || !dayEvents.length) return 0;
    var az = [];
    dayEvents.forEach(function (e) {
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
  // the subset that carry a recognized MoonNightPayload, each as
  // {type, moons:[ids], raw: <the Event>}.
  var TYPES = ['blood', 'magic', 'conjunction', 'eclipse', 'harvest'];
  function parseDayEvents(events) {
    var out = [];
    (events || []).forEach(function (e) {
      if (!e.payload) return;
      var p;
      try { p = JSON.parse(e.payload); } catch (err) { return; }
      if (!p || TYPES.indexOf(p.type) < 0) return;
      out.push({ type: p.type, moons: p.moons || [], raw: e });
    });
    return out;
  }

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
      else if (e.type === 'harvest') ups.forEach(function (m) { out.harvest[m.mo.id] = true; });
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
  function apply(st, dayEvents) {
    st.met = []; st.aur = []; st.comets = []; st.airs = []; st.darkNight = 0; st.bleed = null; st.shadows = {}; st.fxItems = st.fxItems || [];
    var look = st.look;
    (look ? look.aurora : []).forEach(function (a) { st.aur.push({ k: a.k, c0: a.c0, c1: a.c1, az: 0, span: a.span || 170, base: a.alt === 'high' ? .3 : a.alt === 'low' ? .08 : .16, seed: 1 }); });
    (look ? look.fire : []).forEach(function (f) { fireballs(st, f, night(st) * .85 + .15); lookShower(st, f); });
    (dayEvents || []).forEach(function (e) {
      var ms = (e.moons || []).map(function (id) { return findMoonState(st.moons, id); }).filter(function (M) { return M && M.up; });
      if (e.type === 'blood') ms.forEach(function (M) { lunar(st, M, true); });
      else if (e.type === 'eclipse') ms.forEach(function (M) { lunar(st, M, false); });
      else if (e.type === 'conjunction' && ms.length > 1) ms.forEach(function (M) { M.conj = .8; });
      else if (e.type === 'harvest') ms.forEach(function (M) {
        // Warm and large near the hills, where it is watched rising.
        var low = smooth01(24, 2, M.m.alt / D2R);
        M.warm = ['#ffd58a', .7 * (.45 + .55 * low)];
        st.fxItems.push({ shape: 'warm', x: M.x, y: M.y, r: M.r, col: lit('#ffd58a', 'glow', st.sl), k: .7 * (.3 + .7 * low) * night(st) });
      });
    });
    ambient(st);
  }

  window.SkyEvents = { camFor: camFor, parseDayEvents: parseDayEvents, overlaysFor: overlaysFor, apply: apply, metPlan: metPlan, discCover: discCover, TYPES: TYPES };
})();
