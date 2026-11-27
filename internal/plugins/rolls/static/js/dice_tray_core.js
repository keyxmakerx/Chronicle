/**
 * dice_tray_core.js: the dice tray, mountable on any element (Chronicle.diceTray).
 *
 *   var tray = Chronicle.diceTray.mount(el, opts);
 *   tray.roll(view);          // a roll as the roll feed delivers it: it lands on the shared mat
 *   tray.own(view);           // a roll thrown in the member's own tower or bowl
 *   tray.setLevel('simple' | 'flat' | 'detailed');
 *   tray.setFinish('paper' | 'kit');   // Detailed only
 *   tray.setShape('tower' | 'bowl');   // the member's own tray
 *   tray.setOthers(true | false);      // other members' dice on the mat
 *   tray.destroy();
 *
 * opts: level, finish, shape, others (the starting choices), own (false leaves the member's own
 * tray out), tokens() and kit() and dark() (the host page's colours, kit and theme, for a host that
 * is not Chronicle), look(view) (the roller's { set, colour }), color(view) (a roller's tag colour),
 * motion() ('full' | 'calm' | 'off'), engine and libs (where the dice engine and its libraries are,
 * for a host that has no layout to read them from).
 *
 * Privacy comes first: the mat draws only a roll whose mode is "public". A private, blind or
 * Director-only roll is skipped here whatever the feed delivered, so a screen that holds one (the
 * Director's, or the roller's own) never puts it where anyone else could see it. The server already
 * sends other members nothing but public rolls; this is the second lock, and the only one a host
 * outside Chronicle can rely on.
 *
 * Values are decided by the server and arrive in the view; the tray only shows them. At Detailed
 * the mat's dice are 3D (dice_tray_stage.js); at Flat paper and Simple they are flat drawings, and a
 * browser with no WebGL shows Detailed's dice flat as well. Under Calm the dice are set down with a
 * fade, and under Off (or a device asking for less) nothing is drawn and the result line says it.
 */
(function () {
  'use strict';

  var W = window;
  W.Chronicle = W.Chronicle || {};
  var Art = W.Chronicle.diceTrayArt, Stage = W.Chronicle.diceTrayStage;

  var SIDES = [4, 6, 8, 10, 12, 20];
  var LEVELS = ['simple', 'flat', 'detailed'];
  // White name tags sit on these, so each reaches 4.5:1 against white.
  var TAGS = ['#4f46e5', '#0f766e', '#92400e', '#a16207', '#9d174d', '#166534', '#9a3412', '#1d4ed8'];
  // The mat's half sizes in world units; the stage's mat layout is the same.
  var HW = 4.6, HD = 3, EDGE = 0.75;
  // The most dice the mat holds; the oldest give way.
  var KEEP = 10;
  // A roller's side of the mat as the angle their dice come in from: the member at the near edge, the
  // others in turn round the other three sides.
  var NEAR = Math.PI / 2, OTHERS = [Math.PI, -Math.PI / 2, 0];

  // Flat dice, drawn the way the Table's quick dice are: outline, lit face, shaded sides, facet lines.
  var GEO = {
    4: { o: [[28, 4], [53, 49], [3, 49]], f: [[28, 17], [43, 44], [13, 44]], k: [[0, 0], [1, 1], [2, 2]], sh: [[[3, 49], [53, 49], [43, 44], [13, 44]]], t: [28, 35] },
    6: { o: [[8, 5], [48, 5], [52, 9], [52, 47], [48, 51], [8, 51], [4, 47], [4, 9]], f: [[10, 10], [46, 10], [46, 44], [10, 44]], k: [[0, 0], [1, 1], [2, 4], [3, 5]], sh: [[[4, 47], [8, 51], [48, 51], [52, 47], [46, 44], [10, 44]]], t: [28, 27] },
    8: { o: [[28, 2], [54, 28], [28, 54], [2, 28]], f: [[28, 9], [47, 39], [9, 39]], k: [[0, 0], [1, 1], [2, 3], [1, 2], [2, 2]], sh: [[[9, 39], [47, 39], [28, 54]]], t: [28, 29] },
    10: { o: [[28, 2], [53, 23], [28, 54], [3, 23]], f: [[28, 11], [42, 26], [28, 43], [14, 26]], k: [[0, 0], [1, 1], [2, 2], [3, 3]], sh: [[[14, 26], [28, 43], [28, 54], [3, 23]], [[42, 26], [53, 23], [28, 54], [28, 43]]], t: [28, 27] },
    12: { o: [[28, 2], [53, 20], [44, 52], [12, 52], [3, 20]], f: [[28, 13], [42, 24], [37, 41], [19, 41], [14, 24]], k: [[0, 0], [1, 1], [2, 2], [3, 3], [4, 4]], sh: [[[19, 41], [37, 41], [44, 52], [12, 52]]], t: [28, 29] },
    20: { o: [[28, 2], [51, 15], [51, 41], [28, 54], [5, 41], [5, 15]], f: [[28, 12], [45, 41], [11, 41]], k: [[0, 0], [0, 1], [0, 5], [1, 1], [1, 2], [1, 3], [2, 3], [2, 4], [2, 5]], sh: [[[11, 41], [45, 41], [28, 54]], [[45, 41], [51, 41], [51, 15]]], t: [28, 31] }
  };
  function pts(a) { return a.map(function (p) { return p.join(','); }).join(' '); }
  // Only numbers go into the markup, so a roll's content can never reach it as HTML.
  function flatSvg(sides, value) {
    var g = GEO[sides] || GEO[20];
    var lines = g.k.map(function (k) { var a = g.f[k[0]], b = g.o[k[1]]; return '<line x1="' + a[0] + '" y1="' + a[1] + '" x2="' + b[0] + '" y2="' + b[1] + '" class="dtr-fl"/>'; }).join('');
    return '<svg viewBox="0 0 56 56" aria-hidden="true"><polygon class="dtr-fb" points="' + pts(g.o) + '"/>' +
      g.sh.map(function (s) { return '<polygon class="dtr-fs" points="' + pts(s) + '"/>'; }).join('') +
      '<polygon class="dtr-ff" points="' + pts(g.f) + '"/>' + lines + '<polygon class="dtr-fr" points="' + pts(g.o) + '"/>' +
      '<text x="' + g.t[0] + '" y="' + g.t[1] + '">' + (value == null ? '' : Number(value)) + '</text></svg>';
  }

  function pick(v, list, dflt) { return list.indexOf(v) >= 0 ? v : dflt; }
  function hash(s) { var h = 0; for (var i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0; return Math.abs(h); }
  function motionNow() {
    var m = document.documentElement.getAttribute('data-motion');
    if (m === 'off' || m === 'calm') return m;
    if (W.matchMedia && W.matchMedia('(prefers-reduced-motion: reduce)').matches) return 'off';
    return 'full';
  }

  // The dice a roll shows on the mat: whole dice only, in the faces they have. A roll whose mode is
  // not public, or a blind stub, shows none.
  function publicDice(v) {
    if (!v || v.hidden || v.mode !== 'public' || !Array.isArray(v.dice)) return null;
    var out = [];
    v.dice.forEach(function (d) {
      if (d && SIDES.indexOf(d.sides) >= 0 && d.value >= 1 && d.value <= d.sides && d.value % 1 === 0) out.push({ sides: d.sides, value: d.value });
    });
    return out.length ? out.slice(0, KEEP) : null;
  }

  // Which of this page's own throws are the tray's: a roll the member makes from the tray is thrown in
  // the tray, so the page-wide throw (table.js) stands down for it.
  var pendingOwn = 0, ownIds = {};
  function owns(r) { return !!r && (!!ownIds[r.id] || (pendingOwn > 0 && !!r.mine)); }

  // The page's colours as hex, which the art paints with. Computed values come back in any CSS form.
  var probe = null;
  function hex(c) {
    if (!probe) probe = document.createElement('canvas').getContext('2d');
    probe.fillStyle = '#000000'; probe.fillStyle = c;
    return probe.fillStyle;
  }
  function pageTokens() {
    var cs = getComputedStyle(document.documentElement);
    function g(n, f) { return hex(cs.getPropertyValue(n).trim() || f); }
    return { surface: g('--color-card-bg', '#ffffff'), alt: g('--color-bg-tertiary', '#f3f4f6'), border: g('--color-border', '#e5e7eb'), accent: g('--color-accent', '#6366f1'), text: g('--color-text-primary', '#111827') };
  }

  function mount(el, o) {
    o = o || {};
    var st = {
      level: pick(o.level, LEVELS, 'detailed'), finish: pick(o.finish, ['paper', 'kit'], 'paper'), shape: pick(o.shape, ['tower', 'bowl'], 'tower'),
      others: o.others !== false, round: [], seen: {}, sides: {}, nextSide: 0, dead: false, flat: [], glBad: false
    };
    var self = {};
    el.classList.add('dtr-core');
    el.innerHTML = '<div class="dtr-mat" data-dtr-mat><div class="dtr-flat" aria-hidden="true"></div></div>' +
      '<p class="dtr-result" role="status" aria-live="polite"></p>' +
      (o.own === false ? '' : '<div class="dtr-own" data-dtr-own></div>');
    var matEl = el.querySelector('[data-dtr-mat]'), flatEl = el.querySelector('.dtr-flat'), resEl = el.querySelector('.dtr-result'), ownEl = el.querySelector('[data-dtr-own]');
    var matArtEl = null, mat = null, matInit = null, own = null, ownInit = null, lookObj = null, painter = 0;

    function kitName() { return o.kit ? o.kit() : ((el.closest('[data-dms-kit]') || document.documentElement).getAttribute('data-dms-kit') || document.documentElement.getAttribute('data-cz-kit') || 'material'); }
    function isDark() { return o.dark ? o.dark() : document.documentElement.classList.contains('dark'); }
    function look() {
      var kind = st.level === 'simple' ? 'simple' : st.level === 'flat' ? 'flat' : st.finish;
      return Art.lookFrom(kind, o.tokens ? o.tokens() : pageTokens(), kitName(), isDark(), 'paper');
    }
    function motion() { return o.motion ? o.motion() : motionNow(); }
    function matGL() { return st.level === 'detailed' && !st.glBad; }
    function say(text) { resEl.textContent = text; }

    // ---------- Roller identity: a colour and a side of the mat ----------
    function keyOf(v) { return v.mine ? 'me' : (v.rollerId || 'n:' + (v.rollerName || '')); }
    function colorOf(v, key) {
      if (o.color) return o.color(v);
      return key === 'me' ? TAGS[0] : TAGS[1 + hash(key) % (TAGS.length - 1)];
    }
    function sideOf(key) {
      if (key === 'me') return NEAR;
      if (st.sides[key] == null) st.sides[key] = OTHERS[st.nextSide++ % OTHERS.length];
      return st.sides[key];
    }
    function lookOf(v) {
      if (o.look) return o.look(v);
      var C = W.Chronicle, mine = C.diceTable || null;
      return C.rollDice ? C.rollDice.forRoll(v, mine || {}, mine) : (mine || {});
    }
    function nameOf(v, key) { return key === 'me' ? 'You' : (v.rollerName || 'Someone'); }

    // ---------- The mat's picture ----------
    // Detailed draws the mat on a WebGL stage that also throws the 3D dice; the other levels paint
    // the same art on a plain canvas and lay flat dice over it, so they take no GPU context.
    function paintMatFlat() {
      var L = lookObj = look();
      var c = Art.matArt(L, HW, HD, 100);
      c.className = 'dtr-matart'; c.setAttribute('aria-hidden', 'true');
      if (matArtEl && matArtEl.parentNode) matArtEl.parentNode.replaceChild(c, matArtEl); else matEl.insertBefore(c, flatEl);
      matArtEl = c;
    }
    function ensureMat() {
      if (mat) return Promise.resolve(mat);
      if (matInit) return matInit;
      var t = new Stage.Tray({ style: 'mat' });
      matInit = t.init().then(function () {
        if (st.dead) { t.destroy(); return null; }
        t.setMode('shared'); t.setStyle('mat'); t.setLook(look());
        t.mount(matEl);
        mat = t;
        return t;
      }).catch(function (e) {
        // No WebGL, or the files did not load: Detailed shows its dice flat instead.
        st.glBad = true; matInit = null;
        if (W.console) console.warn('dice tray: 3D dice unavailable', e);
        return null;
      });
      return matInit;
    }
    function ensureOwn() {
      if (!ownEl) return Promise.resolve(null);
      if (own) return Promise.resolve(own);
      if (ownInit) return ownInit;
      var t = new Stage.Tray({ style: st.shape, cls: 'dtr-own-tray' });
      ownInit = t.init().then(function () {
        if (st.dead) { t.destroy(); return null; }
        t.setMode('solo'); t.setStyle(st.shape); t.setLook(look());
        t.mount(ownEl);
        own = t;
        return t;
      }).catch(function (e) {
        ownInit = null;
        ownEl.classList.add('dtr-own-off');
        ownEl.textContent = 'The 3D dice could not start in this browser, so your tray is not shown.';
        if (W.console) console.warn('dice tray: own tray unavailable', e);
        return null;
      });
      return ownInit;
    }
    function setMatLook() {
      matEl.setAttribute('data-level', st.level);
      matEl.setAttribute('data-mode', matGL() && mat ? 'gl' : 'flat');
      if (matGL()) {
        if (matArtEl) { matArtEl.remove(); matArtEl = null; }
        if (mat) { mat.setLook(look()); if (mat.el.parentNode !== matEl) matEl.appendChild(mat.el); mat.el.hidden = false; }
      } else {
        if (mat) mat.el.hidden = true;
        paintMatFlat();
      }
      if (own) own.setLook(look());
    }

    // ---------- Flat dice ----------
    var scale = function () { return matEl.clientWidth / (2 * (HW + EDGE) * 1.02); };
    function px(x, z) { var w = matEl.clientWidth, h = matEl.clientHeight; return { x: w / 2 + x / ((HW + EDGE) * 1.02) * (w / 2), y: h / 2 + z / ((HD + EDGE) * 1.02) * (h / 2) }; }
    // Where a roll's dice sit: along the roller's side of the mat, a second row further in when there
    // are many, then moved inward from any die already down.
    function slots(n, side) {
      var ax = Math.cos(side) * (HW - 1.3), az = Math.sin(side) * (HD - 1.1), ux = -Math.sin(side), uz = Math.cos(side), gap = 1.25, out = [];
      var taken = st.flat.map(function (d) { return d.at; });
      for (var i = 0; i < n; i++) {
        var row = Math.floor(i / 4), col = i % 4, cnt = Math.min(4, n - row * 4);
        var p = { x: ax + ux * (col - (cnt - 1) / 2) * gap - Math.cos(side) * row * gap, z: az + uz * (col - (cnt - 1) / 2) * gap - Math.sin(side) * row * gap };
        for (var tries = 0; tries < 8 && taken.some(function (q) { return Math.hypot(q.x - p.x, q.z - p.z) < gap * 0.92; }); tries++) { p.x -= Math.cos(side) * gap * 0.6; p.z -= Math.sin(side) * gap * 0.6; }
        p.x = Math.max(-HW + 0.7, Math.min(HW - 0.7, p.x)); p.z = Math.max(-HD + 0.7, Math.min(HD - 0.7, p.z));
        taken.push(p); out.push(p);
      }
      return out;
    }
    function flatLook() { return st.level === 'simple' ? 'simple' : 'paper'; }
    function dropFlat(r, fade) {
      st.flat = st.flat.filter(function (x) { return x !== r; });
      var gone = function () { r.els.forEach(function (n) { if (n.parentNode) n.parentNode.removeChild(n); }); };
      if (!fade || !r.els[0].animate) { gone(); return; }
      r.els.forEach(function (n) { n.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 220, fill: 'forwards' }); });
      setTimeout(gone, 240);
    }
    function trimFlat(incoming) {
      var count = incoming;
      for (var i = st.flat.length - 1; i >= 0; i--) { count += st.flat[i].n; if (count > KEEP) dropFlat(st.flat[i], true); }
    }
    // Lays one roll's flat dice. settled: put down at once (a redraw); else thrown in from the roller's
    // side, or faded in under Calm.
    function drawFlat(d, settled) {
      var m = settled ? 'off' : motion(), sc = scale(), size = Math.max(26, Math.min(56, sc * 1.1)), at = slots(d.dice.length, d.side), els = [];
      var rec = { key: d.key, id: d.id, n: d.dice.length, els: els, at: at[0] };
      st.flat.forEach(function (x) { if (x.key === d.key) dropFlat(x, true); });
      trimFlat(d.dice.length);
      var done;
      var p = new Promise(function (res) { done = res; });
      d.dice.forEach(function (die, i) {
        var n = document.createElement('div');
        n.className = 'dtr-fd'; n.setAttribute('data-look', flatLook()); n.setAttribute('data-sides', String(die.sides)); n.setAttribute('data-value', String(die.value));
        n.style.setProperty('--dtr-edge', d.color);
        var c = px(at[i].x, at[i].z);
        n.style.width = n.style.height = size + 'px'; n.style.left = (c.x - size / 2) + 'px'; n.style.top = (c.y - size / 2) + 'px';
        var cycle = m === 'full';
        n.innerHTML = flatSvg(die.sides, cycle ? '' : die.value);
        flatEl.appendChild(n); els.push(n);
        if (m === 'calm' && n.animate) n.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 220, fill: 'backwards' });
        if (cycle && n.animate) {
          var dur = 440 + Math.min(i, 4) * 10, sx = Math.cos(d.side) * matEl.clientWidth * 0.55, sy = Math.sin(d.side) * matEl.clientHeight * 0.55, spin = -(600 + i * 70), txt = n.querySelector('text');
          n.animate([
            { transform: 'translate(' + sx + 'px,' + sy + 'px) rotate(' + spin + 'deg) scale(0.82)', opacity: 0 },
            { transform: 'translate(' + sx * 0.5 + 'px,' + sy * 0.2 + 'px) rotate(' + spin * 0.55 + 'deg) scale(1.04)', opacity: 1, offset: 0.3 },
            { transform: 'translate(' + sx * 0.1 + 'px,' + sy * 0.05 + 'px) rotate(' + spin * 0.1 + 'deg)', offset: 0.75 },
            { transform: 'none' }], { duration: dur, easing: 'cubic-bezier(0.22,0.8,0.24,1)' });
          [0.2, 0.45, 0.7].forEach(function (f) { setTimeout(function () { if (txt.isConnected) txt.textContent = String(1 + Math.floor(Math.random() * die.sides)); }, dur * f); });
          setTimeout(function () { if (txt.isConnected) txt.textContent = String(die.value); if (i === d.dice.length - 1) done(); }, dur);
        } else if (i === d.dice.length - 1) setTimeout(done, m === 'calm' ? 220 : 0);
      });
      // The name goes over the first die, in the roller's colour.
      var tag = document.createElement('span');
      tag.className = 'tray3d-tag dtr-tag'; tag.textContent = d.name; tag.style.setProperty('--tag', d.color);
      var tc = px(at[0].x, at[0].z);
      tag.style.transform = 'translate(' + Math.round(tc.x) + 'px,' + Math.round(tc.y - size * 0.62) + 'px) translate(-50%,-100%)';
      flatEl.appendChild(tag); els.push(tag);
      st.flat.push(rec);
      return p;
    }

    // ---------- Putting a roll on the mat ----------
    function visible(d) { return d.key === 'me' || st.others; }
    function draw3D(d, settled) {
      return ensureMat().then(function (t) {
        if (!t || st.dead || !matGL()) return drawFlat(d, settled);
        matEl.setAttribute('data-mode', 'gl');
        var m = settled ? 'calm' : motion();
        return t.roll(d.dice.map(function (x) { return { sides: x.sides, value: x.value, kind: d.set, colour: d.colour }; }),
          { calm: m !== 'full', tag: d.name, color: d.color, who: d.key, from: d.side, replace: d.key });
      });
    }
    function drawRoll(d, settled) {
      if (!visible(d)) return Promise.resolve();
      return matGL() ? draw3D(d, settled) : drawFlat(d, settled);
    }
    function resultLine(v, d) {
      var vals = d.dice.map(function (x) { return x.value; });
      var mod = typeof v.modifier === 'number' ? v.modifier : 0;
      var total = typeof v.total === 'number' ? v.total : vals.reduce(function (s, x) { return s + x; }, 0) + mod;
      say(d.name + ' rolled ' + (v.formula || '') + ': ' + vals.join(' + ') + (mod ? (mod > 0 ? ' + ' : ' − ') + Math.abs(mod) : '') + ' = ' + total);
    }
    function clearMat(now) {
      st.flat.slice().forEach(function (r) { dropFlat(r, !now); });
      if (mat) { if (now) mat.clearAll(true); else mat.clearAll(); }
    }
    // Redraws every kept roll, set down, after a change of level or of Others' dice.
    function redraw() {
      clearMat(true);
      st.round.forEach(function (d) { drawRoll(d, true); });
    }

    self.roll = function (v) {
      var dice = publicDice(v);
      if (!dice || !v.id || st.seen[v.id]) return Promise.resolve(null);
      st.seen[v.id] = true;
      var key = keyOf(v), lk = lookOf(v) || {};
      var d = { id: v.id, key: key, name: nameOf(v, key), color: colorOf(v, key), side: sideOf(key), dice: dice, set: lk.set || '', colour: lk.colour || '' };
      // A roller's new roll takes the place of their last, and the mat keeps the latest ten dice.
      st.round = st.round.filter(function (x) { return x.key !== key; });
      st.round.push(d);
      var total = 0;
      for (var i = st.round.length - 1; i >= 0; i--) { total += st.round[i].dice.length; if (total > KEEP) st.round.splice(i, 1); }
      if (motion() === 'off') { if (visible(d) || key === 'me') resultLine(v, d); return Promise.resolve(d); }
      return drawRoll(d, false).then(function () { if (!st.dead && visible(d)) resultLine(v, d); return d; });
    };

    // A roll thrown in the member's own tray. The mat gets the same roll through roll(); this is only
    // the member's own throw, so it never changes what anyone else sees.
    self.own = function (v) {
      var dice = (v && Array.isArray(v.dice) ? v.dice : []).filter(function (x) { return x && SIDES.indexOf(x.sides) >= 0 && x.value >= 1 && x.value <= x.sides; }).slice(0, KEEP);
      if (!dice.length || !ownEl) return Promise.resolve(null);
      if (v.id) ownIds[v.id] = true;
      if (motion() === 'off') return Promise.resolve(dice);
      var lk = lookOf(Object.assign({ mine: true }, v)) || {}, m = motion();
      return ensureOwn().then(function (t) {
        if (!t || st.dead) return null;
        return t.roll(dice.map(function (x) { return { sides: x.sides, value: x.value, kind: lk.set || '', colour: lk.colour || '' }; }), { calm: m !== 'full', who: 'me' });
      });
    };

    self.setLevel = function (l) {
      l = pick(l, LEVELS, st.level);
      if (l === st.level) return;
      st.level = l;
      setMatLook();
      if (matGL()) { ensureMat().then(function () { if (st.dead) return; setMatLook(); redraw(); }); } else redraw();
    };
    self.setFinish = function (f) { st.finish = pick(f, ['paper', 'kit'], st.finish); setMatLook(); };
    self.setShape = function (s) {
      s = pick(s, ['tower', 'bowl'], st.shape);
      if (s === st.shape) return;
      st.shape = s;
      if (own) { own.clearAll(true); own.setStyle(s); own.setLook(look()); }
    };
    self.setOthers = function (on) { on = !!on; if (on === st.others) return; st.others = on; redraw(); };
    self.clear = function () { st.round = []; st.seen = {}; clearMat(false); say(''); };
    // What is on show, for tests and for a host that wants to know.
    self.state = function () {
      var three = 0;
      if (mat) mat.rolls.forEach(function (r) { if (r.state !== 'leave') three += r.dice.length; });
      var tags = Array.prototype.map.call(el.querySelectorAll('[data-dtr-mat] .tray3d-tag'), function (n) { return n.textContent; });
      return { level: st.level, finish: st.finish, shape: st.shape, others: st.others, mode: matEl.getAttribute('data-mode'), three: three, flat: el.querySelectorAll('[data-dtr-mat] .dtr-fd').length, tags: tags, result: resEl.textContent };
    };
    self.destroy = function () {
      st.dead = true;
      clearTimeout(painter);
      if (mo) mo.disconnect();
      if (mat) mat.destroy();
      if (own) own.destroy();
      el.classList.remove('dtr-core');
      el.textContent = '';
    };

    // A change of theme or style kit repaints the art.
    var mo = null;
    function repaint() {
      clearTimeout(painter);
      painter = setTimeout(function () { if (!st.dead) setMatLook(); }, 60);
    }
    try {
      mo = new MutationObserver(repaint);
      mo.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'data-cz-kit', 'data-theme'] });
      var dock = el.closest('[data-dms-kit]');
      if (dock) mo.observe(dock, { attributes: true, attributeFilter: ['data-dms-kit'] });
    } catch (e) { /* the art then repaints with the next change of level */ }

    setMatLook();
    if (matGL()) ensureMat().then(function () { if (!st.dead) setMatLook(); });
    if (ownEl) ensureOwn();
    return self;
  }

  W.Chronicle.diceTray = { mount: mount, owns: owns, publicDice: publicDice, pending: function (n) { pendingOwn = Math.max(0, pendingOwn + n); } };
})();
