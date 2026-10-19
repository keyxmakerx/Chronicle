/**
 * trackers.js: the Trackers page block.
 *
 * It fetches the page's trackers from data-endpoint, so what it shows is what
 * the server decided this viewer may see: a Director-only tracker never
 * arrives for a player. A tracker's value is worked out on the server from its
 * log; every change here writes one log line and draws the tracker the server
 * sends back. User text goes in with textContent, never as markup. The pure
 * helpers are exposed as window.TrackerHelpers for the node tests.
 *
 * Styles are in ../css/trackers.css, scoped under .tk, on the theme tokens so
 * the page look and style kit restyle the block.
 */
(function () {
  'use strict';

  // The server's limits, mirrored so a form can stop short of a refusal.
  var LIM = { name: 80, text: 300, note: 200, steps: 20, bands: 12, segMin: 2, segMax: 24 };

  // ---------- Pure helpers ----------

  // parseSteps reads one step per line, "Name: what it means" or "Name".
  function parseSteps(text) {
    return String(text || '').split('\n').map(function (l) { return l.trim(); }).filter(Boolean).map(function (l) {
      var i = l.indexOf(':');
      if (i < 0) return { name: l, text: '' };
      return { name: l.slice(0, i).trim(), text: l.slice(i + 1).trim() };
    });
  }

  // parseBands reads one band per line, "2: what it means from 2 up". A line
  // that doesn't start with a whole number is reported, not guessed at.
  function parseBands(text) {
    var bands = [], bad = [];
    String(text || '').split('\n').forEach(function (raw, n) {
      var l = raw.trim();
      if (!l) return;
      var m = /^(-?\d+)\s*[:.)-]?\s*(.+)$/.exec(l);
      if (!m) { bad.push(n + 1); return; }
      bands.push({ at: parseInt(m[1], 10), text: m[2].trim() });
    });
    return { bands: bands, bad: bad };
  }

  // bandIndex is the band in force at value v (the highest starting at or
  // below it), or -1.
  function bandIndex(bands, v) {
    var best = -1;
    (bands || []).forEach(function (b, i) {
      if (b.at <= v && (best < 0 || b.at >= bands[best].at)) best = i;
    });
    return best;
  }

  // change describes one log line in words: "+1", "−2", "set to 3", or a
  // step's name for a steps tracker.
  function change(t, e) {
    if (e.op === 'set') {
      if (t.shape === 'steps' && t.steps[e.amount]) return 'set to ' + t.steps[e.amount].name;
      return 'set to ' + e.amount;
    }
    if (t.shape === 'steps') return e.amount > 0 ? 'stepped forward' : 'stepped back';
    return (e.amount > 0 ? '+' : '−') + Math.abs(e.amount);
  }

  // valueLabel is the tracker's value in words.
  function valueLabel(t) {
    if (t.shape === 'steps') return t.steps[t.value] ? t.steps[t.value].name : '';
    if (t.shape === 'clock') return t.value + ' of ' + t.max;
    return String(t.value);
  }

  // canStep says whether a step of dir (+1/-1) would change anything.
  function canStep(t, dir) {
    var lo = t.shape === 'count' ? (t.min == null ? null : t.min) : 0;
    var hi = t.shape === 'steps' ? t.steps.length - 1 : (t.max == null ? null : t.max);
    if (dir < 0) return lo === null || t.value > lo;
    return hi === null || t.value < hi;
  }

  // segments returns the clock's wedges as SVG path data on a unit circle,
  // from twelve o'clock clockwise.
  function segments(n) {
    var out = [];
    for (var k = 0; k < n; k++) {
      var a0 = k / n * 2 * Math.PI - Math.PI / 2, a1 = (k + 1) / n * 2 * Math.PI - Math.PI / 2;
      out.push('M0 0 L' + Math.cos(a0).toFixed(4) + ' ' + Math.sin(a0).toFixed(4) +
        ' A1 1 0 0 1 ' + Math.cos(a1).toFixed(4) + ' ' + Math.sin(a1).toFixed(4) + ' Z');
    }
    return out;
  }

  // kindBody builds the kind a "make a new one" form describes, or returns
  // {error} naming what to fix.
  function kindBody(f) {
    var name = String(f.name || '').trim();
    if (!name) return { error: 'Give the tracker a name.' };
    var body = { name: name, shape: f.shape, visibility: f.visibility || 'directors', steps: [], bands: [] };
    if (f.shape === 'steps') {
      body.steps = parseSteps(f.steps);
      if (body.steps.length < 2) return { error: 'Write at least two steps, one per line.' };
      if (body.steps.length > LIM.steps) return { error: 'At most ' + LIM.steps + ' steps.' };
      return body;
    }
    if (f.shape === 'clock') {
      var seg = parseInt(f.segments, 10);
      if (!(seg >= LIM.segMin && seg <= LIM.segMax)) return { error: 'A clock needs ' + LIM.segMin + ' to ' + LIM.segMax + ' segments.' };
      body.max = seg;
    } else {
      var lo = String(f.min == null ? '' : f.min).trim(), hi = String(f.max == null ? '' : f.max).trim();
      if (lo !== '') body.min = Number(lo);
      if (hi !== '') body.max = Number(hi);
      if ((lo !== '' && !Number.isInteger(body.min)) || (hi !== '' && !Number.isInteger(body.max))) {
        return { error: 'Lowest and highest must be whole numbers.' };
      }
      if (body.min != null && body.max != null && body.min >= body.max) return { error: 'Highest must be above lowest.' };
    }
    var pb = parseBands(f.bands);
    if (pb.bad.length) return { error: 'Start each band line with a number, like "2: A bane to resist".' };
    if (pb.bands.length > LIM.bands) return { error: 'At most ' + LIM.bands + ' bands.' };
    body.bands = pb.bands;
    return body;
  }

  var H = {
    LIM: LIM, parseSteps: parseSteps, parseBands: parseBands, bandIndex: bandIndex,
    change: change, valueLabel: valueLabel, canStep: canStep, segments: segments, kindBody: kindBody
  };
  if (typeof window !== 'undefined') window.TrackerHelpers = H;
  if (typeof Chronicle === 'undefined' || !Chronicle.register) return;

  // ---------- DOM ----------
  var SVGNS = 'http://www.w3.org/2000/svg';

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
  // icon is the icon-weight control: monochrome, framed only on hover, named
  // for screen readers.
  function icon(fa, label, onclick, disabled) {
    var b = h('button', 'tk-ic', '', { type: 'button', 'aria-label': label, title: label, disabled: !!disabled }, { click: onclick });
    b.appendChild(h('i', 'fa-solid ' + fa, '', { 'aria-hidden': 'true' }));
    return b;
  }
  // quiet is the quiet-word control: no border, it reveals more.
  function quiet(label, onclick, attrs) {
    return h('button', 'tk-q', label, Object.assign({ type: 'button' }, attrs || {}), { click: onclick });
  }

  function api(url, method, body, csrf) {
    return Chronicle.apiFetch(url, { method: method || 'GET', body: body, csrfToken: csrf }).then(function (res) {
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

  Chronicle.register('trackers', {
    init: function (el, cfg) {
      var S = { el: el, cfg: cfg, view: null, open: {}, adding: false, menu: null, busy: false, status: '', statusErr: false };
      el._tk = S;
      load(S);
    },
    destroy: function (el) { el._tk = null; }
  });

  function load(S) {
    // An owned view (a recipe's block) reads only the kinds owned under its
    // prefix; its first change on a kind creates that kind's row here.
    var url = S.cfg.owner ? S.cfg.endpoint + '?owner=' + encodeURIComponent(S.cfg.owner) : S.cfg.endpoint;
    api(url).then(function (v) {
      S.view = v;
      draw(S);
    }).catch(function () {
      // A block that cannot load is left out; the rest of the page is fine.
      S.el.textContent = '';
    });
  }

  function setStatus(S, msg, isErr) {
    S.status = msg || ''; S.statusErr = !!isErr;
    var line = S.el.querySelector('.tk-status');
    if (line) { line.textContent = S.status; line.classList.toggle('err', S.statusErr); }
  }

  // replace swaps one tracker in the view for the one the server returned.
  function replace(S, key, tv) {
    S.view.trackers = S.view.trackers.map(function (t) { return keyOf(t) === key ? tv : t; });
  }
  function keyOf(t) { return t.id || ('kind:' + t.kindId); }

  function draw(S) {
    var el = S.el, v = S.view;
    el.textContent = '';
    if (!v.trackers.length && !v.canEdit) return;
    var head = h('div', 'tk-head');
    append(head, [
      h('h3', 'tk-title cb-head', 'Trackers'),
      v.trackers.length ? h('span', 'tk-count cb-count', String(v.trackers.length)) : null,
      h('span', 'tk-sp'),
      v.canEdit && !S.cfg.owner ? quiet(S.adding ? 'Close' : 'Add a tracker', function () { S.adding = !S.adding; draw(S); },
        { 'aria-expanded': S.adding ? 'true' : 'false' }) : null
    ]);
    el.appendChild(head);
    if (S.adding) el.appendChild(addPanel(S));
    if (!v.trackers.length) {
      el.appendChild(h('p', 'tk-empty', S.cfg.owner
        ? 'This recipe has no trackers or clocks to show here. Change it in the Homebrew Workshop.'
        : 'No trackers on this page yet. Add one to count something, step through named stages, or run a clock.'));
    }
    var list = h('div', 'tk-list');
    v.trackers.forEach(function (t) { list.appendChild(row(S, t)); });
    el.appendChild(list);
    el.appendChild(h('p', 'tk-status' + (S.statusErr ? ' err' : ''), S.status, { role: 'status', 'aria-live': 'polite' }));
  }

  // ---------- One tracker ----------
  function row(S, t) {
    var key = keyOf(t), edit = S.view.canEdit, hidden = t.visibility !== 'everyone';
    var r = h('section', 'tk-row cb-group', '', { 'aria-label': t.name });
    if (hidden) r.setAttribute('data-dm-only', '');

    var top = h('div', 'tk-top');
    append(top, [
      h('span', 'tk-name', t.name),
      edit ? h('span', 'tk-who' + (hidden ? ' cb-dm' : ''), hidden ? 'Director only' : 'Players see this') : null,
      h('span', 'tk-sp'),
      edit ? icon('fa-minus', t.shape === 'steps' ? 'Step back' : 'Take one off', function () { step(S, t, -1); }, S.busy || !canStep(t, -1)) : null,
      h('span', 'tk-val', valueLabel(t), { 'aria-live': 'polite' }),
      edit ? icon('fa-plus', t.shape === 'steps' ? 'Step forward' : (t.shape === 'clock' ? 'Tick the clock' : 'Add one'),
        function () { step(S, t, 1); }, S.busy || !canStep(t, 1)) : null,
      edit && t.id ? icon('fa-ellipsis', 'More for ' + t.name, function () { S.menu = S.menu === key ? null : key; draw(S); }) : null
    ]);
    r.appendChild(top);
    if (S.menu === key) r.appendChild(menu(S, t));

    if (t.shape === 'steps') r.appendChild(stepsView(S, t));
    else if (t.shape === 'clock') r.appendChild(clockView(S, t));
    else if (t.bands && t.bands.length) r.appendChild(bandsView(t));
    if (t.now) r.appendChild(h('p', 'tk-now', t.now));

    if (t.logCount || edit) {
      var open = !!S.open[key];
      r.appendChild(quiet((open ? 'Hide the log' : 'Log') + (t.logCount ? ' · ' + t.logCount : ''),
        function () { S.open[key] = !open; draw(S); }, { 'aria-expanded': open ? 'true' : 'false' }));
      if (open) r.appendChild(logView(S, t));
    }
    return r;
  }

  // stepsView shows every step; a Director moves to one by choosing it.
  function stepsView(S, t) {
    var ol = h('ol', 'tk-steps');
    t.steps.forEach(function (s, i) {
      var cls = 'tk-step' + (i === t.value ? ' on' : (i < t.value ? ' past' : ''));
      var label = s.name + (i === t.value ? ' (now)' : '');
      var li = h('li', '');
      if (S.view.canEdit && i !== t.value) {
        li.appendChild(h('button', cls, s.name, { type: 'button', 'aria-label': 'Set to ' + s.name, disabled: S.busy },
          { click: function () { log(S, t, { op: 'set', amount: i }); } }));
      } else {
        li.appendChild(h('span', cls, s.name, { 'aria-label': label, 'aria-current': i === t.value ? 'step' : null }));
      }
      ol.appendChild(li);
    });
    return ol;
  }

  function clockView(S, t) {
    var wrap = h('div', 'tk-clock');
    var svg = document.createElementNS(SVGNS, 'svg');
    svg.setAttribute('viewBox', '-1.06 -1.06 2.12 2.12');
    svg.setAttribute('class', 'tk-face');
    svg.setAttribute('role', 'img');
    svg.setAttribute('aria-label', t.value + ' of ' + t.max + ' segments filled');
    segments(t.max).forEach(function (d, k) {
      var p = document.createElementNS(SVGNS, 'path');
      p.setAttribute('d', d);
      p.setAttribute('class', k < t.value ? 'tk-seg on' : 'tk-seg');
      svg.appendChild(p);
    });
    wrap.appendChild(svg);
    if (S.view.canEdit && t.value > 0) {
      wrap.appendChild(quiet('Start again', function () { log(S, t, { op: 'set', amount: 0, note: 'started again' }); }));
    }
    return wrap;
  }

  // bandsView is one mark per band, lit up to the band in force.
  function bandsView(t) {
    var cur = bandIndex(t.bands, t.value);
    var sorted = t.bands.slice().sort(function (a, b) { return a.at - b.at; });
    var bar = h('ol', 'tk-bands', '', { 'aria-label': 'Bands' });
    sorted.forEach(function (b) {
      var on = cur >= 0 && b.at <= t.bands[cur].at;
      var here = cur >= 0 && b.at === t.bands[cur].at;
      bar.appendChild(h('li', 'tk-band' + (on ? ' on' : '') + (here ? ' here' : ''), '', { title: 'From ' + b.at + ': ' + b.text }));
    });
    return bar;
  }

  // ---------- The log ----------
  function logView(S, t) {
    var box = h('div', 'tk-log');
    if (S.view.canEdit) box.appendChild(logForm(S, t));
    var ul = h('ul', 'tk-lines');
    t.log.forEach(function (e) {
      var li = h('li', 'tk-line' + (e.struck ? ' struck' : ''));
      var words = h(e.struck ? 's' : 'span', 'tk-what');
      words.textContent = [e.when, change(t, e), e.note].filter(Boolean).join(' · ');
      append(li, [
        words,
        h('span', 'tk-after', e.struck ? 'struck' : '→ ' + (t.shape === 'steps' && t.steps[e.after] ? t.steps[e.after].name : e.after)),
        S.view.canEdit ? icon(e.struck ? 'fa-rotate-left' : 'fa-strikethrough', e.struck ? 'Bring this line back' : 'Strike this line',
          function () { strike(S, t, e); }, S.busy) : null
      ]);
      ul.appendChild(li);
    });
    if (!t.log.length) ul.appendChild(h('li', 'tk-line tk-none', 'Nothing logged yet.'));
    box.appendChild(ul);
    if (t.logCount > t.log.length) box.appendChild(h('p', 'tk-more', 'Showing the latest ' + t.log.length + ' of ' + t.logCount + ' lines.'));
    return box;
  }

  // logForm sets the tracker to a value with a note, for anything the step
  // controls don't cover ("the dose was taken").
  function logForm(S, t) {
    var f = h('form', 'tk-form tk-logform');
    var id = 'tk-' + Math.random().toString(36).slice(2, 8);
    var input;
    if (t.shape === 'steps') {
      input = h('select', 'tk-field', '', { id: id + 'v' });
      t.steps.forEach(function (s, i) { input.appendChild(h('option', '', s.name, { value: String(i), selected: i === t.value })); });
    } else {
      input = h('input', 'tk-field tk-num', '', { id: id + 'v', type: 'number', value: String(t.value), step: '1',
        min: t.shape === 'clock' ? '0' : (t.min == null ? null : String(t.min)), max: t.max == null ? null : String(t.max) });
    }
    var note = h('input', 'tk-field', '', { id: id + 'n', type: 'text', maxlength: String(LIM.note), placeholder: 'Why, for the log' });
    append(f, [
      h('label', 'tk-l', 'Set to', { for: id + 'v' }), input,
      h('label', 'tk-l', 'Note', { for: id + 'n' }), note,
      h('button', 'tk-btn', 'Log it', { type: 'submit', disabled: S.busy })
    ]);
    f.addEventListener('submit', function (ev) {
      ev.preventDefault();
      var n = parseInt(input.value, 10);
      if (isNaN(n)) { setStatus(S, 'Type a whole number.', true); return; }
      log(S, t, { op: 'set', amount: n, note: note.value });
    });
    return f;
  }

  // ---------- Row menu ----------
  function menu(S, t) {
    var m = h('div', 'tk-menu', '', { role: 'group', 'aria-label': 'More for ' + t.name });
    var hidden = t.visibility !== 'everyone';
    append(m, [
      quiet('Rename', function () {
        var name = window.prompt('New name for ' + t.name, t.name);
        if (name == null || !name.trim() || name.trim() === t.name) return;
        patch(S, t, { name: name.trim() });
      }),
      quiet(hidden ? 'Show to players' : 'Hide from players', function () { patch(S, t, { visibility: hidden ? 'everyone' : 'directors' }); }),
      quiet('Remove', function () {
        if (!window.confirm('Remove ' + t.name + ' and its log from this page? This cannot be undone.')) return;
        remove(S, t);
      }, { 'class': 'tk-q tk-danger' })
    ]);
    return m;
  }

  // ---------- Saving ----------
  function run(S, p, after) {
    S.busy = true; draw(S);
    return p.then(function (res) {
      S.busy = false;
      if (after) after(res);
      setStatus(S, 'Saved');
      draw(S);
    }).catch(function (err) {
      S.busy = false;
      setStatus(S, err.message, true);
      draw(S);
    });
  }

  function log(S, t, change) {
    var body = { op: change.op, amount: change.amount, note: change.note || '' };
    if (t.id) body.trackerId = t.id; else body.kindId = t.kindId;
    if (S.cfg.owner) body.owner = S.cfg.owner;
    var key = keyOf(t);
    return run(S, api(S.cfg.endpoint + '/log', 'POST', body, S.cfg.csrfToken), function (tv) {
      replace(S, key, tv);
      if (S.open[key]) { S.open[keyOf(tv)] = true; }
    });
  }
  function step(S, t, dir) { return log(S, t, { op: 'add', amount: dir }); }

  function strike(S, t, e) {
    var url = '/campaigns/' + encodeURIComponent(S.cfg.campaignId) + '/trackers/entries/' + e.id + '/strike';
    return run(S, api(url, 'POST', { struck: !e.struck }, S.cfg.csrfToken), function (tv) { replace(S, keyOf(t), tv); });
  }

  function trackerURL(S, t) {
    return '/campaigns/' + encodeURIComponent(S.cfg.campaignId) + '/trackers/' + encodeURIComponent(t.id);
  }
  function patch(S, t, body) {
    S.menu = null;
    return run(S, api(trackerURL(S, t), 'PATCH', body, S.cfg.csrfToken), function (tv) { replace(S, keyOf(t), tv); });
  }
  function remove(S, t) {
    S.menu = null;
    return run(S, api(trackerURL(S, t), 'DELETE', null, S.cfg.csrfToken), function () {
      S.view.trackers = S.view.trackers.filter(function (x) { return x.id !== t.id; });
    });
  }

  // ---------- Add a tracker ----------
  // addPanel places a saved kind, or sets up a new kind and places it in one
  // go, so a Director never has to leave the page to start counting.
  function addPanel(S) {
    var p = h('div', 'tk-add');
    var id = 'tk-' + Math.random().toString(36).slice(2, 8);
    var kinds = S.view.kinds || [];
    var mode = S.addMode || (kinds.length ? 'kind' : 'new');
    if (kinds.length) {
      var tabs = h('div', 'tk-tabs', '', { role: 'tablist', 'aria-label': 'Add a tracker' });
      [['kind', 'Use a saved kind'], ['new', 'Make a new one']].forEach(function (m) {
        tabs.appendChild(h('button', 'tk-tab' + (mode === m[0] ? ' on' : ''), m[1],
          { type: 'button', role: 'tab', 'aria-selected': mode === m[0] ? 'true' : 'false' },
          { click: function () { S.addMode = m[0]; draw(S); } }));
      });
      p.appendChild(tabs);
    }
    p.appendChild(mode === 'kind' ? useKindForm(S, id, kinds) : newKindForm(S, id));
    return p;
  }

  function seeChoice(id, value) {
    var sel = h('select', 'tk-field', '', { id: id });
    [['directors', 'Directors only'], ['everyone', 'Everyone in the campaign']].forEach(function (o) {
      sel.appendChild(h('option', '', o[1], { value: o[0], selected: o[0] === value }));
    });
    return sel;
  }

  function useKindForm(S, id, kinds) {
    var f = h('form', 'tk-form');
    var kind = h('select', 'tk-field', '', { id: id + 'k' });
    kinds.forEach(function (k) { kind.appendChild(h('option', '', k.name, { value: k.id })); });
    var name = h('input', 'tk-field', '', { id: id + 'n', type: 'text', maxlength: String(LIM.name), placeholder: 'Same as the kind' });
    var see = seeChoice(id + 's', kinds[0] ? kinds[0].visibility : 'directors');
    kind.addEventListener('change', function () {
      var k = kinds.filter(function (x) { return x.id === kind.value; })[0];
      if (k) see.value = k.visibility;
    });
    append(f, [
      h('label', 'tk-l', 'Kind', { for: id + 'k' }), kind,
      h('label', 'tk-l', 'Name on this page', { for: id + 'n' }), name,
      h('label', 'tk-l', 'Who sees it', { for: id + 's' }), see,
      h('button', 'tk-btn', 'Add it', { type: 'submit', disabled: S.busy })
    ]);
    f.addEventListener('submit', function (ev) {
      ev.preventDefault();
      place(S, { kindId: kind.value, name: name.value.trim(), visibility: see.value });
    });
    return f;
  }

  function newKindForm(S, id) {
    var f = h('form', 'tk-form tk-newkind');
    var shape = S.addShape || 'count';
    var name = h('input', 'tk-field', '', { id: id + 'n', type: 'text', maxlength: String(LIM.name), required: true });
    var shapes = h('fieldset', 'tk-shapes');
    shapes.appendChild(h('legend', 'tk-l', 'What it tracks'));
    [['count', 'A number'], ['steps', 'Named steps'], ['clock', 'A clock']].forEach(function (o) {
      var lab = h('label', 'tk-radio');
      append(lab, [
        h('input', '', '', { type: 'radio', name: id + 'shape', value: o[0], checked: o[0] === shape },
          { change: function () { S.addShape = o[0]; S.addDraft = read(); draw(S); } }),
        h('span', '', o[1])
      ]);
      shapes.appendChild(lab);
    });
    var d = S.addDraft || {};
    if (d.name) name.value = d.name;
    var kids = [h('label', 'tk-l', 'Name', { for: id + 'n' }), name, shapes];
    var min, max, steps, seg, bands;
    if (shape === 'count') {
      min = h('input', 'tk-field tk-num', '', { id: id + 'lo', type: 'number', step: '1', value: d.min != null ? d.min : '0' });
      max = h('input', 'tk-field tk-num', '', { id: id + 'hi', type: 'number', step: '1', value: d.max || '', placeholder: 'No limit' });
      kids.push(h('label', 'tk-l', 'Lowest', { for: id + 'lo' }), min, h('label', 'tk-l', 'Highest', { for: id + 'hi' }), max);
    } else if (shape === 'steps') {
      steps = h('textarea', 'tk-field', '', { id: id + 'st', rows: '4', placeholder: 'One per line, like\nCalm: nothing yet\nStirring: the ink moves' });
      if (d.steps) steps.value = d.steps;
      kids.push(h('label', 'tk-l', 'Steps', { for: id + 'st' }), steps);
    } else {
      seg = h('input', 'tk-field tk-num', '', { id: id + 'sg', type: 'number', step: '1', min: String(LIM.segMin), max: String(LIM.segMax), value: d.segments || '6' });
      kids.push(h('label', 'tk-l', 'Segments', { for: id + 'sg' }), seg);
    }
    if (shape !== 'steps') {
      bands = h('textarea', 'tk-field', '', { id: id + 'bd', rows: '3', placeholder: 'Optional, one per line, like\n2: A bane to resist\n4: The ink takes hold' });
      if (d.bands) bands.value = d.bands;
      kids.push(h('label', 'tk-l', 'Bands', { for: id + 'bd' }), bands);
    }
    var see = seeChoice(id + 's', d.visibility || 'directors');
    kids.push(h('label', 'tk-l', 'Who sees it', { for: id + 's' }), see,
      h('button', 'tk-btn', 'Make and add it', { type: 'submit', disabled: S.busy }));
    append(f, kids);

    function read() {
      return {
        name: name.value, shape: shape, visibility: see.value,
        min: min ? min.value : '', max: max ? max.value : '',
        steps: steps ? steps.value : '', segments: seg ? seg.value : '', bands: bands ? bands.value : ''
      };
    }
    f.addEventListener('submit', function (ev) {
      ev.preventDefault();
      var body = kindBody(read());
      if (body.error) { setStatus(S, body.error, true); return; }
      S.busy = true; draw(S);
      api(S.cfg.kindsEndpoint, 'POST', body, S.cfg.csrfToken).then(function (k) {
        S.busy = false;
        S.addDraft = null;
        S.view.kinds = (S.view.kinds || []).concat([{ id: k.id, name: k.name, shape: k.shape, visibility: k.visibility }]);
        return place(S, { kindId: k.id, name: '', visibility: body.visibility });
      }).catch(function (err) {
        S.busy = false;
        S.addDraft = read();
        setStatus(S, err.message, true);
        draw(S);
      });
    });
    return f;
  }

  function place(S, body) {
    return run(S, api(S.cfg.endpoint, 'POST', body, S.cfg.csrfToken), function (tv) {
      // A placed kind replaces the page's automatic copy of it, if any.
      S.view.trackers = S.view.trackers.filter(function (x) { return x.id || x.kindId !== tv.kindId; });
      S.view.trackers.push(tv);
      S.adding = false;
      S.addShape = null;
      S.addMode = null;
    });
  }
})();
