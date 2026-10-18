/**
 * homebrew_workshop.js: the Homebrew Workshop. A Director builds a recipe here: parts
 * (trackers, clocks and the kinds other plugins register) wired by When/Then
 * rules. Left is the library and the palette, the middle is a canvas of
 * cards the Director can pan, zoom and arrange, the right is the inspector
 * for the selected card.
 *
 * The catalog decides what can be added: nothing here knows a game system,
 * and the server checks every save against the same catalog. Only the
 * tracker and clock shapes have a form of their own, because those two are
 * the everyday widgets the Workshop ships with.
 */
(function () {
  'use strict';

  // ---------- Pure helpers (tested in test/js/homebrew_workshop.test.mjs) ----------

  var SEE = [['party', 'Party'], ['name', 'Name only'], ['director', 'Director only']];
  var TIMING = [['now', 'At once'], ['encounter_end', 'When the encounter ends'], ['held', 'Held for the Director']];
  var COL = { when: 40, then: 330, part: 640 };
  var ROW = 110;

  // partKey turns a label into a key no other part of the recipe uses.
  function partKey(label, taken) {
    var base = String(label || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 32) || 'part';
    if (!/^[a-z0-9]/.test(base)) base = 'p' + base;
    var key = base, n = 2;
    while (taken.indexOf(key) >= 0) key = base + '-' + n++;
    return key;
  }

  // cardsOf lists every card the doc draws, in drawing order, with where it
  // sits: its saved place, or a column by type when it has none.
  function cardsOf(doc) {
    var lay = doc.layout || {}, cards = [], y = 40;
    doc.rules.forEach(function (r, i) {
      var id = 'r' + i, rows = Math.max(1, r.then.length);
      cards.push({ id: id, type: 'when', rule: i, pos: lay[id] || { x: COL.when, y: y + (rows - 1) * ROW / 2 } });
      r.then.forEach(function (d, j) {
        var tid = 'r' + i + '.' + j;
        cards.push({ id: tid, type: 'then', rule: i, step: j, pos: lay[tid] || { x: COL.then, y: y + j * ROW } });
      });
      y += rows * ROW + 30;
    });
    doc.parts.forEach(function (p, i) {
      var id = 'p:' + p.key;
      cards.push({ id: id, type: 'part', key: p.key, pos: lay[id] || { x: COL.part, y: 40 + i * 150 } });
    });
    return cards;
  }

  // edgesOf wires the cards: the part an event comes from feeds its When,
  // a When feeds each of its Thens, and a Then feeds the part it acts on.
  function edgesOf(doc) {
    var out = [];
    doc.rules.forEach(function (r, i) {
      if (r.when.part) out.push(['p:' + r.when.part, 'r' + i]);
      r.then.forEach(function (d, j) {
        out.push(['r' + i, 'r' + i + '.' + j]);
        if (d.part) out.push(['r' + i + '.' + j, 'p:' + d.part]);
      });
    });
    return out;
  }

  // shapeText says in words what a tracker or clock part holds.
  function shapeText(data) {
    var d = data || {};
    if (d.shape === 'clock') return 'Clock of ' + (d.max || '?') + ' segments';
    if (d.shape === 'steps') return 'Named steps: ' + (d.steps || []).map(function (s) { return s.name; }).join(', ');
    var lo = d.min == null ? 'no lowest' : d.min, hi = d.max == null ? 'no highest' : d.max;
    return 'A number, ' + (d.min == null && d.max == null ? 'no limits' : lo + ' to ' + hi);
  }

  // argsText reads a When's or Then's args back through its catalog fields.
  function argsText(fields, args) {
    var a = args || {};
    return (fields || []).filter(function (f) { return a[f.name] != null && a[f.name] !== ''; })
      .map(function (f) { return f.label + ' ' + a[f.name]; }).join(', ');
  }

  // compatible lists the recipe's parts an event or action can use.
  function compatible(doc, def) {
    var kinds = (def && def.partKinds) || [];
    if (!kinds.length) return [];
    return doc.parts.filter(function (p) { return kinds.indexOf(p.kind) >= 0; });
  }

  // removePart drops a part and every When or Then that pointed at it, and
  // says how many rules and steps went with it, so the Director is told.
  function removePart(doc, key) {
    var gone = { rules: 0, steps: 0 };
    doc.parts = doc.parts.filter(function (p) { return p.key !== key; });
    doc.rules = doc.rules.filter(function (r) {
      if (r.when.part === key) { gone.rules++; return false; }
      var n = r.then.length;
      r.then = r.then.filter(function (d) { return d.part !== key; });
      gone.steps += n - r.then.length;
      return true;
    });
    return gone;
  }

  // defaultData is what a new part of a known kind starts with.
  function defaultData(kind) {
    if (kind === 'clock') return { shape: 'clock', max: 6 };
    if (kind === 'tracker') return { shape: 'count', min: 0, max: 5 };
    return {};
  }

  // freeSpot moves a new card's place down until it covers no other card, so
  // a card added beside a busy one does not land on top of it.
  function freeSpot(doc, pos) {
    var others = cardsOf(doc).map(function (c) { return c.pos; }), p = { x: pos.x, y: pos.y };
    var hits = function () { return others.some(function (o) { return Math.abs(o.x - p.x) < 240 && Math.abs(o.y - p.y) < 100; }); };
    for (var n = 0; n < 200 && hits(); n++) p.y += 30;
    return p;
  }

  // startArgs fills a new When's or Then's whole-number fields with 1, so a
  // fresh card already saves; the Director changes it in the inspector.
  function startArgs(defn) {
    var a = {};
    ((defn && defn.fields) || []).forEach(function (f) { if (f.type === 'whole') a[f.name] = 1; });
    return a;
  }

  // ---------- Live preview (pure; tested beside the helpers above) ----------
  //
  // The preview acts a recipe out in the page, with nothing saved: a part
  // whose kind previews as a shape holds a number, and events and actions
  // whose owners declared a preview op move it. Bounds, clamping and bands
  // follow the trackers ledger, so the preview shows what the block would.

  function findDef(cat, list, name) {
    return ((cat && cat[list]) || []).filter(function (d) { return d.name === name; })[0] || null;
  }

  function hasNumber(cat, part) {
    var k = part && findDef(cat, 'parts', part.kind);
    return !!(k && k.preview === 'shape');
  }

  // bounds is the lowest and highest value a shape allows; hi is null for a
  // count with no top, and floor is false for a count with no bottom.
  function bounds(d) {
    d = d || {};
    if (d.shape === 'steps') return { lo: 0, hi: Math.max(0, (d.steps || []).length - 1), floor: true };
    if (d.shape === 'clock') return { lo: 0, hi: d.max == null ? 0 : d.max, floor: true };
    return { lo: d.min == null ? 0 : d.min, hi: d.max == null ? null : d.max, floor: d.min != null };
  }

  function clampV(d, v) {
    var b = bounds(d);
    if (b.floor && v < b.lo) return b.lo;
    if (b.hi != null && v > b.hi) return b.hi;
    return v;
  }

  // nowText is what value v means: a step's name and text, or the band in
  // force (the highest starting at or below v).
  function nowText(d, v) {
    d = d || {};
    if (d.shape === 'steps') {
      var s = (d.steps || [])[v];
      return s ? (s.text ? s.name + ': ' + s.text : s.name) : '';
    }
    var best = null;
    (d.bands || []).forEach(function (b) { if (b.at <= v && (!best || b.at >= best.at)) best = b; });
    return best ? best.text : '';
  }

  // simStart is a fresh preview: every number at its lowest, nothing logged.
  function simStart(doc, cat) {
    var st = { v: {}, log: [], waiting: [] };
    simSync(doc, cat, st);
    return st;
  }

  // simSync keeps the preview in step with an edited recipe: a new part
  // starts at its lowest, a changed shape re-clamps, a removed part and any
  // waiting rule that no longer exists are dropped.
  function simSync(doc, cat, st) {
    var keep = {};
    doc.parts.forEach(function (p) {
      if (!hasNumber(cat, p)) return;
      keep[p.key] = clampV(p.data, st.v[p.key] == null ? bounds(p.data).lo : st.v[p.key]);
    });
    st.v = keep;
    // A waiting rule is held by the rule itself, so removing an earlier rule
    // (which renumbers the rest) keeps it pointing at the right one.
    st.waiting = st.waiting.filter(function (w) { w.rule = doc.rules.indexOf(w.ref); return w.rule >= 0; });
    return st;
  }

  var LIMIT = 50;

  // run starts a rule. A rule timed for later waits; one that runs now does
  // each Then in order. Returns the cards it lit, in order.
  function run(doc, cat, st, i, now, ctx) {
    var rule = doc.rules[i];
    if (!rule) return;
    var ev = findDef(cat, 'events', rule.when.event), p = rule.when.part ? doc.parts.filter(function (x) { return x.key === rule.when.part; })[0] : null;
    var what = (ev ? ev.label : rule.when.event) + (p ? ': ' + p.label : '');
    ctx.lit.push('r' + i);
    if (rule.timing !== 'now' && !now) {
      st.waiting.push({ rule: i, ref: rule, what: what, timing: rule.timing });
      st.log.push(what + ', waits' + (rule.timing === 'held' ? ' for you to run it' : ' for the encounter to end'));
      return;
    }
    rule.then.forEach(function (d, j) {
      if (ctx.steps++ >= LIMIT) { ctx.stopped = true; return; }
      ctx.lit.push('r' + i + '.' + j);
      var act = findDef(cat, 'actions', d.action), tp = doc.parts.filter(function (x) { return x.key === d.part; })[0];
      var label = act ? act.label : d.action;
      if (act && act.preview && tp && hasNumber(cat, tp)) {
        var n = act.preview.arg ? Number((d.args || {})[act.preview.arg]) : 1;
        if (!Number.isFinite(n)) n = 0;
        var was = st.v[tp.key], to = clampV(tp.data, act.preview.op === 'set' ? n : was + n);
        ctx.lit.push('p:' + tp.key);
        st.log.push(label + ': ' + tp.label + ' ' + was + ' to ' + to);
        change(doc, cat, st, tp.key, was, to, ctx);
      } else {
        st.log.push((act && act.clientRun ? 'You would be asked to run: ' : 'At the table this would: ') + label + (tp ? ', ' + tp.label : ''));
      }
    });
  }

  // change sets a part's number and starts every rule whose When it meets: a
  // reach fires when the number lands on its value or moves past it.
  function change(doc, cat, st, key, was, to, ctx) {
    st.v[key] = to;
    if (was === to) return;
    doc.rules.forEach(function (r, i) {
      if (ctx.stopped || r.when.part !== key) return;
      var ev = findDef(cat, 'events', r.when.event);
      if (!(ev && ev.preview && ev.preview.op === 'reach')) return;
      var at = Number((r.when.args || {})[ev.preview.arg]);
      if ((was < at && to >= at) || (was > at && to <= at)) run(doc, cat, st, i, false, ctx);
    });
  }

  function done(st, ctx) {
    if (ctx.stopped) st.log.push('Stopped after ' + LIMIT + ' steps: these rules keep setting each other off.');
    if (st.log.length > 40) st.log.splice(0, st.log.length - 40);
    return ctx.lit;
  }

  // simFire acts out rule i's When happening; now runs it even when it is
  // timed for later, as Run does in the drawer.
  function simFire(doc, cat, st, i, now) {
    var ctx = { lit: [], steps: 0, stopped: false };
    run(doc, cat, st, i, now, ctx);
    return done(st, ctx);
  }

  // simRunWaiting runs the waiting rule at index w now.
  function simRunWaiting(doc, cat, st, w) {
    var item = st.waiting.splice(w, 1)[0];
    return item ? simFire(doc, cat, st, item.rule, true) : [];
  }

  // simNudge moves a part's number by hand, one step, as the block's
  // buttons do; rules that watch it react.
  function simNudge(doc, cat, st, key, by) {
    var p = doc.parts.filter(function (x) { return x.key === key; })[0], ctx = { lit: ['p:' + key], steps: 0, stopped: false };
    if (!p || st.v[key] == null) return [];
    var was = st.v[key], to = clampV(p.data, was + by);
    if (to !== was) st.log.push('By hand: ' + p.label + ' ' + was + ' to ' + to);
    change(doc, cat, st, key, was, to, ctx);
    return done(st, ctx);
  }

  // simSetTo puts a part's number at v, as Try it does for a rule that
  // watches it; null when the shape can't hold v or it is already there.
  function simSetTo(doc, cat, st, key, v) {
    var p = doc.parts.filter(function (x) { return x.key === key; })[0];
    if (!p || st.v[key] == null || !Number.isFinite(v) || clampV(p.data, v) !== v || st.v[key] === v) return null;
    var was = st.v[key], ctx = { lit: ['p:' + key], steps: 0, stopped: false };
    st.log.push('Try it: ' + p.label + ' ' + was + ' to ' + v);
    change(doc, cat, st, key, was, v, ctx);
    return done(st, ctx);
  }

  // firable lists the rules the preview can't start from a number (an event
  // from outside the recipe, or one with no preview), each a button.
  function firable(doc, cat) {
    var out = [];
    doc.rules.forEach(function (r, i) {
      var ev = findDef(cat, 'events', r.when.event);
      if (!(ev && ev.preview)) out.push(i);
    });
    return out;
  }

  var H = {
    partKey: partKey, startArgs: startArgs, freeSpot: freeSpot, cardsOf: cardsOf, edgesOf: edgesOf, shapeText: shapeText, argsText: argsText,
    compatible: compatible, removePart: removePart, defaultData: defaultData,
    bounds: bounds, clampV: clampV, nowText: nowText, simStart: simStart, simSync: simSync, simFire: simFire,
    simRunWaiting: simRunWaiting, simNudge: simNudge, simSetTo: simSetTo, firable: firable
  };
  if (typeof window !== 'undefined') window.HomebrewWorkshop = H;
  if (typeof Chronicle === 'undefined' || !Chronicle.register) return;

  // ---------- DOM ----------

  function h(tag, cls, text, attrs, on) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null && text !== '') e.textContent = text;
    if (attrs) Object.keys(attrs).forEach(function (k) {
      if (attrs[k] === false || attrs[k] == null) return;
      e.setAttribute(k, attrs[k] === true ? '' : attrs[k]);
    });
    if (on) Object.keys(on).forEach(function (k) { e.addEventListener(k, on[k]); });
    return e;
  }
  function append(parent, kids) {
    kids.forEach(function (k) { if (k) parent.appendChild(k); });
    return parent;
  }
  function fa(name) { return h('i', 'fa-solid ' + name, '', { 'aria-hidden': 'true' }); }
  function iconBtn(name, label, onclick) {
    return append(h('button', 'hbw-ic', '', { type: 'button', 'aria-label': label, title: label }, { click: onclick }), [fa(name)]);
  }
  function btn(label, cls, onclick, icon) {
    var b = h('button', 'hbw-btn' + (cls ? ' ' + cls : ''), '', { type: 'button' }, { click: onclick });
    if (icon) b.appendChild(fa(icon));
    b.appendChild(document.createTextNode(label));
    return b;
  }
  function quiet(label, onclick, cls) {
    return h('button', 'hbw-q' + (cls ? ' ' + cls : ''), label, { type: 'button' }, { click: onclick });
  }
  function labelled(text, control) {
    var id = 'hbw-' + Math.random().toString(36).slice(2, 9);
    control.id = id;
    return append(h('div', 'hbw-f'), [h('label', 'hbw-lab', text, { for: id }), control]);
  }
  // seg is a row of pressed/unpressed choices; one is always chosen.
  function seg(label, opts, value, onpick, disabled) {
    var g = h('div', 'hbw-seg', '', { role: 'group', 'aria-label': label });
    opts.forEach(function (o) {
      g.appendChild(h('button', '', o[1], { type: 'button', 'aria-pressed': String(o[0] === value), disabled: !!disabled },
        { click: function () { onpick(o[0]); } }));
    });
    return append(h('div', 'hbw-f'), [h('span', 'hbw-lab', label), g]);
  }
  function select(opts, value, onchange) {
    var s = h('select', 'hbw-in', '', null, { change: function () { onchange(s.value); } });
    opts.forEach(function (o) { s.appendChild(h('option', '', o[1], { value: o[0], selected: o[0] === value })); });
    return s;
  }
  function input(type, value, onchange, attrs) {
    var i = h('input', 'hbw-in', '', Object.assign({ type: type, value: value == null ? '' : String(value) }, attrs || {}),
      { change: function () { onchange(i.value); } });
    return i;
  }

  // ---------- Loading and saving ----------

  function api(S, path, method, body) {
    return Chronicle.apiFetch(S.cfg.endpoint + path, { method: method || 'GET', body: body, csrfToken: S.cfg.csrf }).then(function (res) {
      return res.text().then(function (txt) {
        var data = null;
        try { data = txt ? JSON.parse(txt) : null; } catch (e) { data = null; }
        if (!res.ok) {
          var err = new Error((data && (data.message || data.error)) || 'Something went wrong. Try again.');
          err.status = res.status;
          throw err;
        }
        return data;
      });
    });
  }

  Chronicle.register('homebrew-workshop', {
    init: function (el, cfg) {
      var S = { el: el, cfg: cfg, catalog: null, list: [], cur: null, sel: null, dirty: false, busy: false,
        view: { x: 0, y: 0, s: 1 } };
      el._hbw = S;
      Promise.all([api(S, '/catalog'), api(S, '/recipes')]).then(function (r) {
        S.catalog = r[0];
        S.list = (r[1] && r[1].data) || [];
        frame(S);
        if (S.list.length) open(S, S.list[0].id); else fresh(S);
      }).catch(function (e) {
        el.textContent = '';
        el.appendChild(h('p', 'hbw-err', e.message || 'The workshop could not open.'));
      });
      S.leave = function (ev) { if (S.dirty) { ev.preventDefault(); ev.returnValue = ''; } };
      window.addEventListener('beforeunload', S.leave);
    },
    destroy: function (el) {
      if (el._hbw) window.removeEventListener('beforeunload', el._hbw.leave);
      el._hbw = null;
    }
  });

  function def(S, list, name) {
    return (S.catalog[list] || []).filter(function (d) { return d.name === name; })[0] || null;
  }

  function setStatus(S, msg, isErr) {
    S.statusEl.textContent = msg || '';
    S.statusEl.classList.toggle('err', !!isErr);
  }

  function touch(S) {
    S.dirty = true;
    S.saveBtn.disabled = false;
    setStatus(S, 'Not saved yet');
  }

  // leaveOk asks before throwing away unsaved changes.
  function leaveOk(S) {
    return !S.dirty || window.confirm('This recipe has changes that are not saved. Leave it anyway?');
  }

  function fresh(S) {
    S.cur = { id: null, version: 0, name: 'Untitled recipe', doc: { parts: [], rules: [], layout: {} } };
    S.sel = null; S.dirty = false;
    S.sim = simStart(S.cur.doc, S.catalog);
    redraw(S, true);
  }

  function open(S, id) {
    api(S, '/recipes/' + encodeURIComponent(id)).then(function (r) {
      r.doc.parts = r.doc.parts || []; r.doc.rules = r.doc.rules || []; r.doc.layout = r.doc.layout || {};
      r.doc.parts.forEach(function (p) { p.data = p.data || {}; });
      r.doc.rules.forEach(function (rule) { rule.when.args = rule.when.args || {}; rule.then.forEach(function (d) { d.args = d.args || {}; }); });
      S.cur = r; S.sel = null; S.dirty = false;
      S.sim = simStart(r.doc, S.catalog);
      redraw(S, true);
      setStatus(S, '');
    }).catch(function (e) { setStatus(S, e.message, true); });
  }

  function save(S) {
    if (S.busy) return;
    var c = S.cur, body = { name: c.name, doc: c.doc, version: c.version };
    S.busy = true; S.saveBtn.disabled = true; setStatus(S, 'Saving…');
    var req = c.id ? api(S, '/recipes/' + encodeURIComponent(c.id), 'PUT', body) : api(S, '/recipes', 'POST', body);
    req.then(function (r) {
      S.busy = false; S.dirty = false;
      c.id = r.id; c.version = r.version; c.doc.layout = r.doc.layout || {};
      setStatus(S, 'Saved');
      return api(S, '/recipes').then(function (l) { S.list = (l && l.data) || []; drawLibrary(S); });
    }).catch(function (e) {
      S.busy = false; S.saveBtn.disabled = false;
      setStatus(S, e.status === 409 ? 'Someone else saved this recipe since you opened it. Open it again to see their version.' : e.message, true);
    });
  }

  function remove(S) {
    var c = S.cur;
    if (!c.id) { fresh(S); return; }
    if (!window.confirm('Delete "' + c.name + '"? This cannot be undone.')) return;
    api(S, '/recipes/' + encodeURIComponent(c.id), 'DELETE').then(function () {
      S.list = S.list.filter(function (r) { return r.id !== c.id; });
      S.dirty = false;
      if (S.list.length) open(S, S.list[0].id); else fresh(S);
      setStatus(S, 'Deleted');
    }).catch(function (e) { setStatus(S, e.message, true); });
  }

  // frame builds the three panes once; redraw fills them.
  function frame(S) {
    var el = S.el;
    el.textContent = '';
    S.statusEl = h('span', 'hbw-status', '', { role: 'status' });
    S.saveBtn = btn('Save recipe', 'hbw-primary', function () { save(S); }, 'fa-check');
    S.saveBtn.disabled = true;
    S.libEl = h('div', 'hbw-scroll');
    S.titleEl = h('h2', 'hbw-h');
    S.wrapEl = h('div', 'hbw-wrap', '', { tabindex: '0', 'aria-label': 'Recipe canvas. Drag to move around; plus and minus zoom.' });
    S.cvEl = h('div', 'hbw-cv');
    S.edgesEl = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    S.edgesEl.setAttribute('class', 'hbw-edges');
    S.edgesEl.setAttribute('aria-hidden', 'true');
    S.cvEl.appendChild(S.edgesEl);
    S.wrapEl.appendChild(S.cvEl);
    append(S.wrapEl, [
      h('p', 'hbw-hint', 'Drag a card to arrange it. Click a card to edit it.'),
      append(h('div', 'hbw-zoom'), [
        iconBtn('fa-minus', 'Zoom out', function () { zoom(S, 1 / 1.15); }),
        iconBtn('fa-expand', 'Fit the recipe in view', function () { fit(S); }),
        iconBtn('fa-plus', 'Zoom in', function () { zoom(S, 1.15); })
      ])
    ]);
    S.inspTitle = h('h2', 'hbw-h', 'Recipe');
    S.inspEl = h('div', 'hbw-insp');
    S.prevEl = h('div', 'hbw-prev', '', { 'aria-live': 'polite' });
    S.whereEl = h('div', 'hbw-where');
    S.tryBtn = btn('Try it', '', function () { tryIt(S); }, 'fa-play');
    append(el, [
      append(h('div', 'hbw-top'), [h('span', 'hbw-sp'), S.statusEl, S.tryBtn, S.saveBtn]),
      append(h('div', 'hbw-work'), [
        append(h('section', 'hbw-pane hbw-lib', '', { 'aria-label': 'Your homebrew' }), [
          append(h('div', 'hbw-pane-h'), [h('h2', 'hbw-h', 'Your homebrew'),
            iconBtn('fa-plus', 'New recipe', function () { if (leaveOk(S)) fresh(S); })]),
          S.libEl
        ]),
        append(h('section', 'hbw-pane hbw-cvpane', '', { 'aria-label': 'Recipe' }), [
          append(h('div', 'hbw-pane-h'), [S.titleEl]), S.wrapEl
        ]),
        append(h('section', 'hbw-pane', '', { 'aria-label': 'Inspector' }), [
          append(h('div', 'hbw-pane-h'), [S.inspTitle]),
          append(h('div', 'hbw-scroll'), [
            S.inspEl,
            append(h('section', 'hbw-sec', '', { 'aria-label': 'Live preview' }), [h('h3', 'hbw-lab', 'Live preview'), S.prevEl]),
            append(h('section', 'hbw-sec', '', { 'aria-label': 'Where it shows up' }), [h('h3', 'hbw-lab', 'Where it shows up'), S.whereEl])
          ])
        ])
      ])
    ]);
    panZoom(S);
  }

  function redraw(S, refit) {
    simSync(S.cur.doc, S.catalog, S.sim);
    S.titleEl.textContent = S.cur.name || 'Untitled recipe';
    S.saveBtn.disabled = !S.dirty;
    drawLibrary(S);
    drawCanvas(S);
    drawInspector(S);
    drawPreview(S);
    drawWhere(S);
    if (refit) window.requestAnimationFrame(function () { fit(S); });
  }

  // ---------- Library and palette ----------

  function drawLibrary(S) {
    var lib = S.libEl;
    lib.textContent = '';
    var list = h('ul', 'hbw-list');
    if (!S.cur.id) {
      list.appendChild(append(h('li'), [append(h('button', 'hbw-item', '', { type: 'button', 'aria-current': 'true' }), [
        fa('fa-flask-vial'), append(h('span'), [document.createTextNode(S.cur.name), h('small', '', 'Not saved yet')])])]));
    }
    S.list.forEach(function (r) {
      var cur = S.cur.id === r.id;
      list.appendChild(append(h('li'), [append(h('button', 'hbw-item', '', { type: 'button', 'aria-current': cur ? 'true' : 'false' }, {
        click: function () { if (!cur && leaveOk(S)) open(S, r.id); }
      }), [fa('fa-flask-vial'), append(h('span'), [document.createTextNode(cur ? S.cur.name : r.name),
        h('small', '', plural(r.parts, 'part') + ', ' + plural(r.rules, 'rule'))])])]));
    });
    if (!S.list.length && S.cur.id) list.appendChild(h('li', 'hbw-muted', 'No recipes yet.'));
    lib.appendChild(list);

    var parts = S.catalog.parts || [], events = S.catalog.events || [], actions = S.catalog.actions || [];
    group(lib, 'Parts', 'Adds a part to the recipe.', parts.map(function (p) {
      return chip(p.label, p.name === 'clock' ? 'fa-clock' : 'fa-cube', function () { addPart(S, p); });
    }));
    group(lib, 'When', 'Starts a rule. Pick a part first to tie it to that part.', events.map(function (e) {
      return chip(e.label, 'fa-bolt', function () { addRule(S, e); });
    }));
    group(lib, 'Then', 'Adds a step to the selected rule.', actions.map(function (a) {
      return chip(a.label, 'fa-arrow-right', function () { addStep(S, a); });
    }));
  }

  function plural(n, word) { return n + ' ' + word + (n === 1 ? '' : 's'); }

  function group(lib, title, help, chips) {
    if (!chips.length) return;
    append(lib, [append(h('div', 'hbw-g'), [h('h3', 'hbw-lab', title), h('p', 'hbw-muted', help)]),
      append(h('div', 'hbw-pal'), chips)]);
  }

  function chip(label, icon, onclick) {
    return append(h('button', 'hbw-chip', '', { type: 'button' }, { click: onclick }), [fa(icon), document.createTextNode(label)]);
  }

  // selectedRule is the rule whose When or Then card is selected, or -1.
  function selectedRule(S) {
    var m = /^r(\d+)/.exec(S.sel || '');
    return m ? +m[1] : -1;
  }

  // near places a new card beside the selected one, or in the middle of the
  // view, so it appears where the Director is looking.
  function near(S, dx) {
    var cards = cardsOf(S.cur.doc), sel = cards.filter(function (c) { return c.id === S.sel; })[0];
    if (sel) return { x: Math.round(sel.pos.x + dx), y: Math.round(sel.pos.y + 20) };
    var w = S.wrapEl;
    return { x: Math.round((w.clientWidth / 2 - S.view.x) / S.view.s - 100), y: Math.round((w.clientHeight / 2 - S.view.y) / S.view.s - 40) };
  }

  function addPart(S, kind) {
    var doc = S.cur.doc, label = 'New ' + kind.label.toLowerCase();
    var key = partKey(label, doc.parts.map(function (p) { return p.key; }));
    var spot = freeSpot(doc, near(S, 280));
    doc.parts.push({ key: key, kind: kind.name, label: label, see: kind.forceSee || 'party', data: defaultData(kind.name) });
    doc.layout['p:' + key] = spot;
    S.sel = 'p:' + key;
    touch(S); redraw(S); reveal(S, S.sel);
  }

  function addRule(S, ev) {
    var doc = S.cur.doc, fits = compatible(doc, ev), part = '';
    if ((ev.partKinds || []).length) {
      if (!fits.length) { setStatus(S, 'Add a part this can come from first: ' + kindNames(S, ev.partKinds) + '.', true); return; }
      var selKey = (S.sel || '').indexOf('p:') === 0 ? S.sel.slice(2) : '';
      part = fits.some(function (p) { return p.key === selKey; }) ? selKey : fits[0].key;
    }
    var spot = freeSpot(doc, near(S, -300));
    doc.rules.push({ when: { event: ev.name, part: part, args: startArgs(ev) }, timing: 'now', then: [] });
    var i = doc.rules.length - 1;
    doc.layout['r' + i] = spot;
    S.sel = 'r' + i;
    touch(S); redraw(S); reveal(S, S.sel);
    setStatus(S, 'Now add what it does from Then.');
  }

  function addStep(S, act) {
    var doc = S.cur.doc, i = selectedRule(S);
    if (i < 0) { setStatus(S, 'Select a When card first: a Then belongs to a rule.', true); return; }
    var rule = doc.rules[i], fits = compatible(doc, act), part = '';
    if ((act.partKinds || []).length) {
      if (!fits.length) { setStatus(S, 'Add a part this can act on first: ' + kindNames(S, act.partKinds) + '.', true); return; }
      part = fits.some(function (p) { return p.key === rule.when.part; }) ? rule.when.part : fits[0].key;
    }
    if (rule.then.length >= 12) { setStatus(S, 'A rule does at most 12 things.', true); return; }
    rule.then.push({ action: act.name, part: part, args: startArgs(act) });
    // A new Then goes under the rule's last one, or beside its When.
    var j = rule.then.length - 1, at = {};
    cardsOf(doc).forEach(function (c) { at[c.id] = c.pos; });
    var prev = at['r' + i + '.' + (j - 1)], when = at['r' + i];
    delete doc.layout['r' + i + '.' + j];
    doc.layout['r' + i + '.' + j] = freeSpot(doc, prev ? { x: prev.x, y: prev.y + ROW } : { x: when.x + 290, y: when.y });
    S.sel = 'r' + i + '.' + j;
    touch(S); redraw(S); reveal(S, S.sel);
  }

  function kindNames(S, kinds) {
    return kinds.map(function (k) { var d = def(S, 'parts', k); return d ? d.label : k; }).join(' or ');
  }

  // ---------- Canvas ----------

  var SEE_ICON = { party: 'fa-eye', name: 'fa-eye-low-vision', director: 'fa-eye-slash' };

  function partOf(doc, key) { return doc.parts.filter(function (p) { return p.key === key; })[0] || null; }

  // cardFace is what a card says: its type, its name and a line of detail.
  function cardFace(S, c) {
    var doc = S.cur.doc;
    if (c.type === 'part') {
      var p = partOf(doc, c.key), k = def(S, 'parts', p.kind);
      return { kind: 'Part · ' + (k ? k.label : p.kind), icon: p.kind === 'clock' ? 'fa-clock' : 'fa-cube', title: p.label,
        body: p.kind === 'tracker' || p.kind === 'clock' ? shapeText(p.data) : '', see: p.see };
    }
    var rule = doc.rules[c.rule];
    if (c.type === 'when') {
      var ev = def(S, 'events', rule.when.event), wp = partOf(doc, rule.when.part);
      var lines = [wp ? wp.label : '', argsText(ev && ev.fields, rule.when.args)];
      if (rule.timing !== 'now') lines.push(TIMING.filter(function (t) { return t[0] === rule.timing; })[0][1]);
      if (!rule.then.length) lines.push('Does nothing yet');
      return { kind: 'When', icon: 'fa-bolt', title: ev ? ev.label : rule.when.event, body: lines.filter(Boolean).join('\n'), warn: !rule.then.length };
    }
    var d = rule.then[c.step], act = def(S, 'actions', d.action), tp = partOf(doc, d.part);
    return { kind: 'Then', icon: 'fa-arrow-right', title: act ? act.label : d.action,
      body: [tp ? tp.label : '', argsText(act && act.fields, d.args)].filter(Boolean).join('\n') };
  }

  function drawCanvas(S) {
    var cv = S.cvEl;
    [].slice.call(cv.querySelectorAll('.hbw-card')).forEach(function (e) { e.remove(); });
    var cards = cardsOf(S.cur.doc);
    if (!cards.length) {
      cv.appendChild(h('p', 'hbw-card hbw-empty', 'Start with a part from the left, such as a tracker or a clock.', { style: 'left:40px;top:40px' }));
    }
    cards.forEach(function (c) {
      var f = cardFace(S, c);
      var card = h('div', 'hbw-card hbw-' + c.type + (c.id === S.sel ? ' sel' : '') + (f.warn ? ' warn' : ''), '', {
        role: 'button', tabindex: '0', 'data-card': c.id, 'aria-pressed': String(c.id === S.sel),
        style: 'left:' + c.pos.x + 'px;top:' + c.pos.y + 'px'
      });
      var head = append(h('div', 'hbw-card-k'), [fa(f.icon), document.createTextNode(f.kind)]);
      if (f.see) {
        var see = append(h('span', 'hbw-see' + (f.see === 'director' ? ' dm' : ''), '', { title: SEE.filter(function (s) { return s[0] === f.see; })[0][1] }), [fa(SEE_ICON[f.see])]);
        head.appendChild(see);
      }
      append(card, [h('span', 'hbw-port in'), h('span', 'hbw-port out'), head, h('div', 'hbw-card-t', f.title), f.body ? h('div', 'hbw-card-b', f.body) : null]);
      card.addEventListener('keydown', function (ev) { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); pick(S, c.id); } });
      dragCard(S, card, c);
      cv.appendChild(card);
    });
    applyView(S);
    drawEdges(S);
  }

  function pick(S, id) {
    S.sel = id;
    [].forEach.call(S.cvEl.querySelectorAll('.hbw-card[data-card]'), function (e) {
      var on = e.getAttribute('data-card') === id;
      e.classList.toggle('sel', on);
      e.setAttribute('aria-pressed', String(on));
    });
    drawInspector(S);
  }

  function drawEdges(S) {
    var svg = S.edgesEl, ns = 'http://www.w3.org/2000/svg';
    while (svg.firstChild) svg.removeChild(svg.firstChild);
    edgesOf(S.cur.doc).forEach(function (e) {
      var a = S.cvEl.querySelector('[data-card="' + e[0] + '"]'), b = S.cvEl.querySelector('[data-card="' + e[1] + '"]');
      if (!a || !b) return;
      var y1 = a.offsetTop + a.offsetHeight / 2, y2 = b.offsetTop + b.offsetHeight / 2;
      // A wire leaves a card's right side for a card further right; when the
      // target sits to the left (a part feeding its When), it leaves the left
      // side instead, so it never loops back across both cards.
      var back = b.offsetLeft + b.offsetWidth < a.offsetLeft, dir = back ? -1 : 1;
      var x1 = back ? a.offsetLeft : a.offsetLeft + a.offsetWidth, x2 = back ? b.offsetLeft + b.offsetWidth : b.offsetLeft;
      var dx = Math.max(40, Math.abs(x2 - x1) / 2) * dir;
      var p = document.createElementNS(ns, 'path');
      p.setAttribute('d', 'M' + x1 + ' ' + y1 + ' C' + (x1 + dx) + ' ' + y1 + ' ' + (x2 - dx) + ' ' + y2 + ' ' + x2 + ' ' + y2);
      p.setAttribute('class', 'hbw-edge' + (S.sel && (e[0] === S.sel || e[1] === S.sel) ? ' hot' : ''));
      svg.appendChild(p);
    });
  }

  function applyView(S) {
    S.cvEl.style.transform = 'translate(' + S.view.x + 'px,' + S.view.y + 'px) scale(' + S.view.s + ')';
  }

  // dragCard moves a card with the pointer; a press without a move selects it.
  function dragCard(S, el, c) {
    el.addEventListener('pointerdown', function (ev) {
      if (ev.button !== 0) return;
      ev.stopPropagation();
      var sx = ev.clientX, sy = ev.clientY, ox = c.pos.x, oy = c.pos.y, moved = false;
      el.setPointerCapture(ev.pointerId);
      function mv(e) {
        var dx = (e.clientX - sx) / S.view.s, dy = (e.clientY - sy) / S.view.s;
        if (!moved && Math.abs(dx) + Math.abs(dy) < 4) return;
        moved = true;
        el.classList.add('drag');
        c.pos = { x: Math.round(ox + dx), y: Math.round(oy + dy) };
        el.style.left = c.pos.x + 'px'; el.style.top = c.pos.y + 'px';
        drawEdges(S);
      }
      function up() {
        el.removeEventListener('pointermove', mv);
        el.removeEventListener('pointerup', up);
        el.removeEventListener('pointercancel', up);
        el.classList.remove('drag');
        if (moved) { S.cur.doc.layout[c.id] = c.pos; touch(S); } else pick(S, c.id);
      }
      el.addEventListener('pointermove', mv);
      el.addEventListener('pointerup', up);
      el.addEventListener('pointercancel', up);
    });
  }

  function panZoom(S) {
    var w = S.wrapEl;
    w.addEventListener('pointerdown', function (ev) {
      if (ev.button !== 0 || ev.target.closest('.hbw-zoom,.hbw-card[data-card]')) return;
      var sx = ev.clientX, sy = ev.clientY, ox = S.view.x, oy = S.view.y, moved = false;
      w.setPointerCapture(ev.pointerId);
      w.classList.add('panning');
      function mv(e) {
        if (Math.abs(e.clientX - sx) + Math.abs(e.clientY - sy) > 3) moved = true;
        S.view.x = ox + e.clientX - sx; S.view.y = oy + e.clientY - sy;
        applyView(S);
      }
      function up() {
        w.removeEventListener('pointermove', mv);
        w.removeEventListener('pointerup', up);
        w.removeEventListener('pointercancel', up);
        w.classList.remove('panning');
        if (!moved && S.sel) { S.sel = null; drawCanvas(S); drawInspector(S); }
      }
      w.addEventListener('pointermove', mv);
      w.addEventListener('pointerup', up);
      w.addEventListener('pointercancel', up);
    });
    w.addEventListener('wheel', function (e) {
      if (!e.ctrlKey && !e.metaKey && Math.abs(e.deltaX) + Math.abs(e.deltaY) < 1) return;
      e.preventDefault();
      if (e.ctrlKey || e.metaKey) {
        var r = w.getBoundingClientRect();
        zoom(S, e.deltaY < 0 ? 1.1 : 1 / 1.1, e.clientX - r.left, e.clientY - r.top);
      } else {
        S.view.x -= e.deltaX; S.view.y -= e.deltaY; applyView(S);
      }
    }, { passive: false });
    w.addEventListener('keydown', function (e) {
      if (e.target !== w) return;
      var step = 40, moves = { ArrowLeft: [step, 0], ArrowRight: [-step, 0], ArrowUp: [0, step], ArrowDown: [0, -step] };
      if (e.key === '+' || e.key === '=') zoom(S, 1.15);
      else if (e.key === '-') zoom(S, 1 / 1.15);
      else if (moves[e.key]) { S.view.x += moves[e.key][0]; S.view.y += moves[e.key][1]; applyView(S); }
      else return;
      e.preventDefault();
    });
  }

  function zoom(S, f, cx, cy) {
    var w = S.wrapEl;
    cx = cx == null ? w.clientWidth / 2 : cx; cy = cy == null ? w.clientHeight / 2 : cy;
    var s = Math.max(0.35, Math.min(1.8, S.view.s * f));
    S.view.x = cx - (cx - S.view.x) * s / S.view.s;
    S.view.y = cy - (cy - S.view.y) * s / S.view.s;
    S.view.s = s;
    applyView(S);
  }

  // fit scales and centres the view on every card.
  function fit(S) {
    var w = S.wrapEl, cards = S.cvEl.querySelectorAll('.hbw-card');
    if (!cards.length || !w.clientWidth) return;
    var x0 = 1e9, y0 = 1e9, x1 = -1e9, y1 = -1e9;
    [].forEach.call(cards, function (e) {
      x0 = Math.min(x0, e.offsetLeft); y0 = Math.min(y0, e.offsetTop);
      x1 = Math.max(x1, e.offsetLeft + e.offsetWidth); y1 = Math.max(y1, e.offsetTop + e.offsetHeight);
    });
    var s = Math.min(1, (w.clientWidth - 60) / (x1 - x0), (w.clientHeight - 80) / (y1 - y0));
    S.view.s = Math.max(0.35, s);
    S.view.x = (w.clientWidth - (x1 - x0) * S.view.s) / 2 - x0 * S.view.s;
    S.view.y = (w.clientHeight - 40 - (y1 - y0) * S.view.s) / 2 - y0 * S.view.s;
    applyView(S);
  }

  // reveal pans just enough to bring a card into view, keeping the zoom.
  function reveal(S, id) {
    var e = S.cvEl.querySelector('[data-card="' + id + '"]'), w = S.wrapEl;
    if (!e || !w.clientWidth) return;
    var pad = 24, s = S.view.s;
    var l = S.view.x + e.offsetLeft * s, t = S.view.y + e.offsetTop * s, r = l + e.offsetWidth * s, b = t + e.offsetHeight * s;
    if (l < pad) S.view.x += pad - l; else if (r > w.clientWidth - pad) S.view.x -= r - (w.clientWidth - pad);
    if (t < pad) S.view.y += pad - t; else if (b > w.clientHeight - 56) S.view.y -= b - (w.clientHeight - 56);
    applyView(S);
  }

  // keepPlaces remembers each card's place on its object before a removal
  // renumbers the rules, then writes the layout back under the new numbers.
  function keepPlaces(doc, change) {
    var cards = cardsOf(doc), at = {};
    cards.forEach(function (c) { at[c.id] = c.pos; });
    doc.rules.forEach(function (r, i) {
      r._pos = at['r' + i];
      r.then.forEach(function (d, j) { d._pos = at['r' + i + '.' + j]; });
    });
    doc.parts.forEach(function (p) { p._pos = at['p:' + p.key]; });
    var out = change();
    doc.layout = {};
    doc.rules.forEach(function (r, i) {
      if (r._pos) doc.layout['r' + i] = r._pos;
      delete r._pos;
      r.then.forEach(function (d, j) { if (d._pos) doc.layout['r' + i + '.' + j] = d._pos; delete d._pos; });
    });
    doc.parts.forEach(function (p) { if (p._pos) doc.layout['p:' + p.key] = p._pos; delete p._pos; });
    return out;
  }

  // ---------- Inspector ----------

  function drawInspector(S) {
    var box = S.inspEl, doc = S.cur.doc, sel = S.sel;
    box.textContent = '';
    if (!sel) { S.inspTitle.textContent = 'Recipe'; recipeForm(S, box); return; }
    if (sel.indexOf('p:') === 0) {
      var p = partOf(doc, sel.slice(2));
      if (!p) { S.sel = null; drawInspector(S); return; }
      var k = def(S, 'parts', p.kind);
      S.inspTitle.textContent = k ? k.label : p.kind;
      partForm(S, box, p, k);
      return;
    }
    var m = /^r(\d+)(?:\.(\d+))?$/.exec(sel), rule = m && doc.rules[+m[1]];
    if (!rule || (m[2] != null && !rule.then[+m[2]])) { S.sel = null; drawInspector(S); return; }
    if (m[2] == null) { S.inspTitle.textContent = 'When'; whenForm(S, box, +m[1], rule); }
    else { S.inspTitle.textContent = 'Then'; thenForm(S, box, +m[1], +m[2], rule); }
  }

  // changed redraws what a form edit affects, leaving the form itself alone
  // so the field being typed in keeps its focus.
  function changed(S) {
    touch(S);
    S.titleEl.textContent = S.cur.name || 'Untitled recipe';
    drawCanvas(S);
    simSync(S.cur.doc, S.catalog, S.sim);
    drawPreview(S);
  }

  function recipeForm(S, box) {
    var c = S.cur;
    append(box, [
      labelled('Recipe name', input('text', c.name, function (v) { c.name = v.trim() || 'Untitled recipe'; changed(S); drawLibrary(S); }, { maxlength: '80' })),
      h('p', 'hbw-muted', 'A recipe is parts, such as a tracker or a clock, and rules that change them. Add a part from the left, then a When to start a rule and a Then for what it does. Click any card to edit it.'),
      h('p', 'hbw-muted', 'Placing a recipe on pages and running its rules at the table come in later updates.'),
      append(h('div', 'hbw-row'), [quiet(c.id ? 'Delete this recipe' : 'Discard this recipe', function () { remove(S); }, 'hbw-danger')])
    ]);
  }

  function partForm(S, box, p, k) {
    var forced = k && k.forceSee;
    append(box, [
      labelled('Name', input('text', p.label, function (v) { p.label = v.trim() || p.label; changed(S); }, { maxlength: '80' })),
      seg('Who sees it', SEE, p.see, function (v) { p.see = v; changed(S); drawInspector(S); }, !!forced),
      h('p', 'hbw-muted', forced ? 'This kind is always ' + SEE.filter(function (s) { return s[0] === forced; })[0][1].toLowerCase() + '.'
        : 'Set once here, for every copy of this part.')
    ]);
    if (p.kind === 'tracker' || p.kind === 'clock') shapeForm(S, box, p);
    else box.appendChild(h('p', 'hbw-muted', 'This kind of part is set up where it lives in the campaign.'));
    var uses = usesOf(S.cur.doc, p.key);
    append(box, [
      uses ? h('p', 'hbw-muted', 'Used by ' + uses + '.') : null,
      append(h('div', 'hbw-row'), [quiet('Remove this part', function () {
        if (uses && !window.confirm('Removing "' + p.label + '" also removes ' + uses + ' that use it. Remove it?')) return;
        keepPlaces(S.cur.doc, function () { return removePart(S.cur.doc, p.key); });
        S.sel = null; touch(S); redraw(S);
      }, 'hbw-danger')])
    ]);
  }

  // usesOf says in words how many Whens and Thens point at a part.
  function usesOf(doc, key) {
    var w = 0, t = 0;
    doc.rules.forEach(function (r) {
      if (r.when.part === key) w++;
      r.then.forEach(function (d) { if (d.part === key) t++; });
    });
    return [w ? plural(w, 'rule') : '', t ? plural(t, 'step') : ''].filter(Boolean).join(' and ');
  }

  function num(v) {
    var s = String(v).trim();
    if (s === '') return null;
    var n = Number(s);
    return Number.isInteger(n) ? n : NaN;
  }

  // Steps are written one a line, "Name: what it means"; bands "3 what
  // happens from 3 up".
  function stepsText(steps) { return (steps || []).map(function (s) { return s.text ? s.name + ': ' + s.text : s.name; }).join('\n'); }
  function parseSteps(txt) {
    return String(txt).split('\n').map(function (l) { return l.trim(); }).filter(Boolean).map(function (l) {
      var i = l.indexOf(':');
      return i < 0 ? { name: l, text: '' } : { name: l.slice(0, i).trim(), text: l.slice(i + 1).trim() };
    });
  }
  function bandsText(bands) { return (bands || []).map(function (b) { return b.at + ' ' + b.text; }).join('\n'); }
  function parseBands(txt) {
    var bad = false, out = String(txt).split('\n').map(function (l) { return l.trim(); }).filter(Boolean).map(function (l) {
      var m = /^(-?\d+)\+?\s+(.+)$/.exec(l);
      if (!m) { bad = true; return null; }
      return { at: +m[1], text: m[2] };
    });
    return bad ? null : out;
  }

  function shapeForm(S, box, p) {
    var d = p.data, set = function (k, v) { if (v == null || v === '') delete d[k]; else d[k] = v; changed(S); };
    if (p.kind === 'tracker') {
      box.appendChild(seg('Shape', [['count', 'A number'], ['steps', 'Named steps'], ['clock', 'A clock']], d.shape || 'count', function (v) {
        p.data = v === 'clock' ? { shape: 'clock', max: 6 } : v === 'steps' ? { shape: 'steps', steps: [{ name: 'First', text: '' }, { name: 'Second', text: '' }] } : { shape: 'count', min: 0, max: 5 };
        changed(S); drawInspector(S);
      }));
    }
    var whole = function (key) {
      return function (v) {
        var n = num(v);
        if (Number.isNaN(n)) { setStatus(S, 'Use a whole number.', true); return; }
        set(key, n);
      };
    };
    if (d.shape === 'clock') {
      box.appendChild(labelled('Segments', input('number', d.max, whole('max'), { min: '2', max: '24', step: '1' })));
    } else if (d.shape === 'steps') {
      var st = h('textarea', 'hbw-in', '', { rows: '5' }, { change: function () { set('steps', parseSteps(st.value)); } });
      st.value = stepsText(d.steps);
      append(box, [labelled('Steps, one a line', st), h('p', 'hbw-muted', 'Write "Name: what it means", or just a name.')]);
    } else {
      append(box, [append(h('div', 'hbw-row'), [
        labelled('Lowest', input('number', d.min, whole('min'), { step: '1', placeholder: 'None' })),
        labelled('Highest', input('number', d.max, whole('max'), { step: '1', placeholder: 'None' }))
      ])]);
    }
    if (d.shape !== 'steps') {
      var bt = h('textarea', 'hbw-in', '', { rows: '4' }, { change: function () {
        var b = parseBands(bt.value);
        if (b === null) { setStatus(S, 'Start each band with a number, like "3 Bane to resist".', true); return; }
        set('bands', b.length ? b : null);
      } });
      bt.value = bandsText(d.bands);
      append(box, [labelled('Bands, one a line', bt), h('p', 'hbw-muted', 'Write "3 what happens from 3 up". Leave it empty for none.')]);
    }
  }

  function partChoice(S, box, defn, cur, onpick) {
    var fits = compatible(S.cur.doc, defn);
    if (!(defn && (defn.partKinds || []).length)) return;
    box.appendChild(labelled('Part', select(fits.map(function (p) { return [p.key, p.label]; }), cur, onpick)));
  }

  function argFields(S, box, defn, args) {
    ((defn && defn.fields) || []).forEach(function (f) {
      var ctl = f.type === 'whole'
        ? input('number', args[f.name], function (v) {
          var n = num(v);
          if (Number.isNaN(n)) { setStatus(S, 'Use a whole number for ' + f.label.toLowerCase() + '.', true); return; }
          if (n == null) delete args[f.name]; else args[f.name] = n;
          changed(S);
        }, { step: '1' })
        : input('text', args[f.name], function (v) { if (v.trim()) args[f.name] = v.trim(); else delete args[f.name]; changed(S); }, { maxlength: '200' });
      box.appendChild(labelled(f.label, ctl));
    });
  }

  function whenForm(S, box, i, rule) {
    var evs = S.catalog.events || [], ev = def(S, 'events', rule.when.event);
    box.appendChild(labelled('Starts when', select(evs.map(function (e) { return [e.name, e.label]; }), rule.when.event, function (v) {
      var nd = def(S, 'events', v), fits = compatible(S.cur.doc, nd);
      rule.when = { event: v, part: (nd.partKinds || []).length ? (fits.some(function (p) { return p.key === rule.when.part; }) ? rule.when.part : (fits[0] ? fits[0].key : '')) : '', args: startArgs(nd) };
      changed(S); drawInspector(S);
    })));
    partChoice(S, box, ev, rule.when.part, function (v) { rule.when.part = v; changed(S); });
    argFields(S, box, ev, rule.when.args);
    append(box, [
      seg('Runs', TIMING, rule.timing, function (v) { rule.timing = v; changed(S); drawInspector(S); }),
      h('p', 'hbw-muted', rule.timing === 'held' ? 'It waits in the Homebrew drawer until you run it.'
        : rule.timing === 'encounter_end' ? 'It waits until the encounter it happened in is over.' : 'It happens as soon as the When does.'),
      rule.then.length ? null : h('p', 'hbw-warn', 'This rule does nothing yet. Add a Then from the left.'),
      append(h('div', 'hbw-row'), [quiet('Remove this rule', function () {
        keepPlaces(S.cur.doc, function () { S.cur.doc.rules.splice(i, 1); });
        S.sel = null; touch(S); redraw(S);
      }, 'hbw-danger')])
    ]);
  }

  function thenForm(S, box, i, j, rule) {
    var d = rule.then[j], acts = S.catalog.actions || [], act = def(S, 'actions', d.action);
    box.appendChild(labelled('Does', select(acts.map(function (a) { return [a.name, a.label]; }), d.action, function (v) {
      var nd = def(S, 'actions', v), fits = compatible(S.cur.doc, nd);
      rule.then[j] = { action: v, part: (nd.partKinds || []).length ? (fits.some(function (p) { return p.key === d.part; }) ? d.part : (fits[0] ? fits[0].key : '')) : '', args: startArgs(nd) };
      changed(S); drawInspector(S);
    })));
    partChoice(S, box, act, d.part, function (v) { d.part = v; changed(S); });
    argFields(S, box, act, d.args);
    if (act && act.clientRun) box.appendChild(h('p', 'hbw-muted', 'This runs in your browser when you press Run in the Homebrew drawer.'));
    append(box, [
      h('p', 'hbw-muted', 'Part of the rule that starts on: ' + ((def(S, 'events', rule.when.event) || {}).label || rule.when.event) + '.'),
      append(h('div', 'hbw-row'), [quiet('Remove this step', function () {
        keepPlaces(S.cur.doc, function () { rule.then.splice(j, 1); });
        S.sel = 'r' + i; touch(S); redraw(S);
      }, 'hbw-danger')])
    ]);
  }

  // ---------- Live preview ----------

  // valueText is a part's number in words: a step's name, "3 / 6", or a
  // bare number for a count with no top.
  function valueText(d, v) {
    if (d.shape === 'steps') { var s = (d.steps || [])[v]; return (s ? s.name : v) + ' (' + (v + 1) + ' of ' + (d.steps || []).length + ')'; }
    var b = bounds(d);
    return b.hi == null ? String(v) : v + ' / ' + b.hi;
  }

  // bar draws a value as segments when there are few enough to read.
  function bar(d, v) {
    var b = bounds(d);
    if (b.hi == null || b.hi - b.lo < 1 || b.hi - b.lo > 24) return null;
    var row = h('div', 'hbw-bar', '', { 'aria-hidden': 'true' });
    for (var i = b.lo + (d.shape === 'steps' ? 0 : 1); i <= b.hi; i++) row.appendChild(h('i', i <= v ? 'on' : ''));
    return row;
  }

  // src ties a preview line to its card: pointing at it lights the card,
  // clicking it selects the card and brings it into view.
  function src(S, el, id) {
    el.setAttribute('data-src', id);
    el.addEventListener('mouseenter', function () { peek(S, id, true); });
    el.addEventListener('mouseleave', function () { peek(S, id, false); });
    el.addEventListener('click', function (ev) {
      if (ev.target.closest('button')) return;
      pick(S, id); reveal(S, id);
    });
    return el;
  }

  function peek(S, id, on) {
    var c = S.cvEl.querySelector('[data-card="' + id + '"]');
    if (c) c.classList.toggle('peek', on);
  }

  // flash lights the cards a run went through, one after another, so the
  // Director sees which wires carried it.
  function flash(S, ids) {
    var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    ids.forEach(function (id, i) {
      window.setTimeout(function () {
        var c = S.cvEl.querySelector('[data-card="' + id + '"]');
        if (!c) return;
        c.classList.add('flash');
        window.setTimeout(function () { c.classList.remove('flash'); }, reduce ? 1200 : 700);
      }, reduce ? 0 : i * 160);
    });
  }

  function acted(S, lit) {
    drawPreview(S);
    flash(S, lit);
  }

  function drawPreview(S) {
    var box = S.prevEl, doc = S.cur.doc, cat = S.catalog, st = S.sim;
    box.textContent = '';
    if (!doc.parts.length && !doc.rules.length) {
      box.appendChild(h('p', 'hbw-muted', 'Add a part and its preview shows here.'));
      return;
    }
    var card = h('div', 'hbw-pv');
    doc.parts.forEach(function (p) {
      var row = src(S, h('div', 'hbw-pv-part'), 'p:' + p.key), k = def(S, 'parts', p.kind);
      var head = append(h('div', 'hbw-row'), [h('strong', 'hbw-pv-name', p.label)]);
      if (p.see !== 'party') {
        head.appendChild(append(h('span', 'hbw-pill' + (p.see === 'director' ? ' dm' : '')), [fa(SEE_ICON[p.see]),
          document.createTextNode(' ' + SEE.filter(function (x) { return x[0] === p.see; })[0][1])]));
      }
      row.appendChild(head);
      if (st.v[p.key] == null) {
        row.appendChild(h('p', 'hbw-muted', (k ? k.label : p.kind) + '. Set up where it lives in the campaign.'));
      } else {
        var v = st.v[p.key], b = bounds(p.data), now = nowText(p.data, v);
        append(head, [h('span', 'hbw-pill', valueText(p.data, v)),
          iconBtn('fa-minus', 'Step ' + p.label + ' down', function () { acted(S, simNudge(doc, cat, st, p.key, -1)); }),
          iconBtn('fa-plus', 'Step ' + p.label + ' up', function () { acted(S, simNudge(doc, cat, st, p.key, 1)); })]);
        var minus = head.children[head.children.length - 2], plus = head.children[head.children.length - 1];
        minus.disabled = b.floor && v <= b.lo; plus.disabled = b.hi != null && v >= b.hi;
        append(row, [bar(p.data, v), now ? h('div', 'hbw-band', now) : null]);
      }
      card.appendChild(row);
    });
    box.appendChild(card);

    var fire = firable(doc, cat);
    if (fire.length) {
      var acts = h('div', 'hbw-pv-acts');
      fire.forEach(function (i) {
        var r = doc.rules[i], ev = def(S, 'events', r.when.event), p = partOf(doc, r.when.part);
        acts.appendChild(btn((ev ? ev.label : r.when.event) + (p ? ': ' + p.label : ''), '', function () { acted(S, simFire(doc, cat, st, i)); }, 'fa-bolt'));
      });
      append(box, [acts, h('p', 'hbw-muted', 'Press one to act out that When. Rules that watch a number run when you step it.')]);
    } else if (doc.rules.length) {
      box.appendChild(h('p', 'hbw-muted', 'Step a part up or down and the rules that watch it run.'));
    }

    if (st.waiting.length) {
      var wl = h('ul', 'hbw-wait');
      st.waiting.forEach(function (w, n) {
        wl.appendChild(append(src(S, h('li', ''), 'r' + w.rule), [
          append(h('span', ''), [document.createTextNode(w.what), h('small', '', w.timing === 'held' ? 'Held until you run it' : 'At the end of the encounter')]),
          btn('Run', '', function () { acted(S, simRunWaiting(doc, cat, st, n)); }, 'fa-play')
        ]));
      });
      append(box, [h('h4', 'hbw-lab', 'Waiting'), wl]);
    }

    if (st.log.length) {
      var log = h('ol', 'hbw-log');
      st.log.slice(-6).forEach(function (l) { log.appendChild(h('li', '', l)); });
      append(box, [log]);
    }
    append(box, [append(h('div', 'hbw-row'), [
      h('span', 'hbw-muted hbw-sp', 'Only you see this, and nothing is saved.'),
      quiet('Start over', function () { S.sim = simStart(doc, cat); drawPreview(S); })
    ])]);
  }

  // tryIt acts out the first rule: its When happening, or for a rule that
  // watches a number, that number landing on its value.
  function tryIt(S) {
    var doc = S.cur.doc, cat = S.catalog, st = S.sim;
    if (!doc.rules.length) { setStatus(S, 'Add a When from the left to have something to try.'); return; }
    var r = doc.rules[0], ev = def(S, 'events', r.when.event), lit;
    if (ev && ev.preview && st.v[r.when.part] != null) {
      var p = partOf(doc, r.when.part), at = Number(r.when.args[ev.preview.arg]);
      lit = simSetTo(doc, cat, st, p.key, at);
      if (!lit) {
        setStatus(S, st.v[p.key] === at ? p.label + ' is already at ' + at + '. Start over to try it again.'
          : p.label + ' can\'t reach ' + at + '. Check the rule\'s number.', true);
        return;
      }
    } else {
      lit = simFire(doc, cat, st, 0);
    }
    setStatus(S, '');
    acted(S, lit);
    S.prevEl.scrollIntoView({ block: 'nearest' });
  }

  // drawWhere lists the places the recipe shows up. Nothing places a recipe
  // yet, so it says so rather than listing places it might go.
  // TODO(#1619): list the pages and categories the Customizer placed it on.
  function drawWhere(S) {
    S.whereEl.textContent = '';
    S.whereEl.appendChild(h('p', 'hbw-muted', S.cur.id ? 'Not placed on any page yet.' : 'Save the recipe first. It is not on any page yet.'));
  }
})();
