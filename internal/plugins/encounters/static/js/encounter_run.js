/**
 * encounter_run.js: the DM Screen's Fight tab.
 *
 * It runs a prepped fight when nobody is in Foundry: the DM starts one of the
 * encounters marked Ready, types each side's initiative, and steps through
 * the turns, with a hit point box for each copy of a creature and conditions
 * named from the game system's list or typed. There is one fight per
 * campaign, kept on the server, so a co-DM opening the screen sees the same
 * one. It saves as you go; a save that lost a race with another DM reloads
 * the fight and says so. Every piece of user text goes in with textContent.
 *
 * Before the first turn the lines stay in the order they were listed, so the
 * DM can type initiative down the column; from the first turn on they are in
 * turn order. A creature's name folds its numbers out under its line
 * (creature_fold.js). When another app says what each creature's page
 * leaves (the server's leaves, from Crafting's drop lists), a chip says so
 * beside its name, and ending the fight shows that app's report of what
 * was left.
 *
 * The pure helpers are exposed as window.EncounterRunHelpers for the node
 * tests. Styles live in static/css/encounters.css, scoped under .erun.
 */
(function () {
  'use strict';

  // The server's limits, mirrored so the tab stops short of a refusal.
  var LIM = { conds: 12, cond: 40, hp: 99999, initLo: -99, initHi: 999, round: 9999 };
  var SAVE_DELAY = 500;

  // A save still in flight when the tab is torn down (a refresh of the
  // screen) is waited on by the next mount, so it never shows the fight from
  // before that save.
  var inflight = {};

  // ---------- Pure helpers ----------
  function clone(o) { return JSON.parse(JSON.stringify(o)); }

  function intOrNull(v) { return typeof v === 'number' && isFinite(v) ? Math.round(v) : null; }

  function normaliseLine(l) {
    l = l && typeof l === 'object' ? l : {};
    var hp = Array.isArray(l.hp) && l.hp.length ? l.hp.map(intOrNull) : [null];
    return {
      id: String(l.id || ''), kind: l.kind === 'hero' ? 'hero' : 'creature',
      name: typeof l.name === 'string' ? l.name : '', entityId: l.entityId || '',
      group: typeof l.group === 'string' ? l.group : '',
      init: intOrNull(l.init), max: intOrNull(l.max), hp: hp,
      conds: (l.conds || []).filter(function (c) { return typeof c === 'string' && c; }),
      waiting: !!l.waiting
    };
  }

  function normaliseRun(r) {
    if (!r || typeof r !== 'object') return null;
    return {
      encounterId: r.encounterId || '', name: typeof r.name === 'string' ? r.name : '',
      round: Math.max(1, intOrNull(r.round) || 1), turn: r.turn || '',
      lines: (r.lines || []).map(normaliseLine)
    };
  }

  // parseNum reads a typed number: '' clears it (null), anything unreadable
  // leaves it as it was (undefined), and the rest is kept inside lo..hi.
  function parseNum(text, lo, hi) {
    var s = String(text == null ? '' : text).trim();
    if (s === '') return null;
    if (!/^[-+]?\d+$/.test(s)) return undefined;
    return Math.max(lo, Math.min(hi, parseInt(s, 10)));
  }

  // order is the lines in turn order: initiative high to low, lines with
  // none last, ties as listed. Lines waiting to arrive are left out.
  function order(run) {
    var live = [];
    run.lines.forEach(function (l, i) { if (!l.waiting) live.push({ l: l, i: i }); });
    live.sort(function (a, b) {
      var x = a.l.init, y = b.l.init;
      if (x === null && y === null) return a.i - b.i;
      if (x === null) return 1;
      if (y === null) return -1;
      return (y - x) || (a.i - b.i);
    });
    return live.map(function (e) { return e.l; });
  }

  // shown is the order the tab draws: as listed until the first turn, so
  // typing initiative never moves the line being typed in.
  function shown(run) {
    if (run.turn) return order(run);
    return run.lines.filter(function (l) { return !l.waiting; });
  }

  // nextTurn moves to the next line in turn order, and past the last one
  // to the first line of a new round. With no turn yet it starts round 1.
  function nextTurn(run) {
    var n = clone(run), ord = order(n), at = -1;
    if (!ord.length) return n;
    for (var i = 0; i < ord.length; i++) if (ord[i].id === n.turn) at = i;
    if (at < 0) {
      n.turn = ord[0].id;
    } else if (at === ord.length - 1) {
      n.round = Math.min(LIM.round, n.round + 1);
      n.turn = ord[0].id;
    } else {
      n.turn = ord[at + 1].id;
    }
    return n;
  }

  function edit(run, id, fn) {
    var n = clone(run);
    n.lines.forEach(function (l) { if (l.id === id) fn(l); });
    return n;
  }

  function setInit(run, id, v) { return edit(run, id, function (l) { l.init = v; }); }
  function setMax(run, id, v) { return edit(run, id, function (l) { l.max = v; }); }
  function setHP(run, id, box, v) {
    return edit(run, id, function (l) { if (box >= 0 && box < l.hp.length) l.hp[box] = v; });
  }
  function bringIn(run, id) { return edit(run, id, function (l) { l.waiting = false; }); }

  // addCond adds a condition once, whatever its case, up to the limit.
  function addCond(run, id, name) {
    name = String(name || '').trim().slice(0, LIM.cond);
    if (!name) return run;
    return edit(run, id, function (l) {
      var have = l.conds.some(function (c) { return c.toLowerCase() === name.toLowerCase(); });
      if (!have && l.conds.length < LIM.conds) l.conds.push(name);
    });
  }

  function removeCond(run, id, name) {
    return edit(run, id, function (l) { l.conds = l.conds.filter(function (c) { return c !== name; }); });
  }

  // current is what a box shows: its own number, or the line's full hit
  // points while it hasn't been touched; null when neither is known.
  function current(line, box) {
    var v = line.hp[box];
    return v !== null && v !== undefined ? v : line.max;
  }

  function isDown(line, box) {
    var v = current(line, box);
    return v !== null && v <= 0;
  }

  // dropNote is the chip beside a creature: what its page leaves, or that a
  // line with no page leaves nothing. No chips at all when the server sent
  // none, which is when no page in the fight has a drop list.
  function dropNote(leaves, l) {
    if (l.kind !== 'creature' || !leaves || !Object.keys(leaves).length) return '';
    return l.entityId ? leaves[l.entityId] || '' : 'No page, so no drops';
  }

  window.EncounterRunHelpers = {
    normaliseRun: normaliseRun, parseNum: parseNum, order: order, shown: shown, nextTurn: nextTurn,
    setInit: setInit, setMax: setMax, setHP: setHP, bringIn: bringIn, addCond: addCond,
    removeCond: removeCond, current: current, isDown: isDown, dropNote: dropNote, LIM: LIM
  };

  if (!window.Chronicle || typeof window.Chronicle.register !== 'function') return;
  var C = window.Chronicle;

  function h(tag, cls, text) {
    var el = document.createElement(tag);
    if (cls) el.className = cls;
    if (text != null && text !== '') el.textContent = text;
    return el;
  }

  function btn(cls, text, act, title) {
    var b = h('button', 'erun-btn' + (cls ? ' ' + cls : ''), text);
    b.type = 'button';
    if (act) b.setAttribute('data-act', act);
    if (title) b.title = title;
    return b;
  }

  function numInput(cls, label, value) {
    var i = h('input', cls);
    i.type = 'text';
    i.setAttribute('inputmode', 'numeric');
    i.setAttribute('autocomplete', 'off');
    i.setAttribute('aria-label', label);
    i.value = value === null || value === undefined ? '' : String(value);
    return i;
  }

  // setValue updates an input unless the DM is typing in it.
  function setValue(input, v) {
    var s = v === null || v === undefined ? '' : String(v);
    if (document.activeElement !== input && input.value !== s) input.value = s;
  }

  C.register('encounter-run', {
    init: function (el) {
      var cid = el.getAttribute('data-campaign-id');
      if (!cid) return;
      var conds = [];
      try { conds = JSON.parse(el.getAttribute('data-conditions') || '[]') || []; } catch (e) { conds = []; }
      var url = '/campaigns/' + encodeURIComponent(cid) + '/encounter-run';
      var S = {
        el: el, cid: cid, url: url, conds: conds, run: null, version: 0, ready: [], blocks: [], leaves: {},
        nodes: {}, open: {}, folds: {}, timer: 0, dirty: false, saving: null, again: false,
        confirmEnd: false, loaded: false, gone: false, report: null
      };
      el._erun = S;
      el.classList.add('erun');
      el.addEventListener('click', function (e) { onClick(S, e); });
      el.addEventListener('change', function (e) { onChange(S, e); });
      el.addEventListener('keydown', function (e) { onKey(S, e); });
      draw(S);
      load(S, '');
    },
    destroy: function (el) {
      var S = el._erun;
      if (!S) return;
      S.gone = true;
      Object.keys(S.folds).forEach(function (id) { C.CreatureFold.dispose(S.folds[id]); });
      S.folds = {};
      save(S);
      el._erun = null;
    }
  });

  function status(S, text) {
    var s = S.el.querySelector('.erun-status');
    if (s) s.textContent = text || '';
  }

  // load reads the fight. Only a fresh mount waits for a save left in
  // flight by the tab before it; a save that lost a race reloads at once,
  // since it is the save in flight.
  function load(S, note, now) {
    var wait = (!now && inflight[S.cid]) || Promise.resolve();
    return wait.catch(function () {}).then(function () {
      return C.apiFetch(S.url, { method: 'GET' });
    }).then(function (res) {
      if (!res.ok) throw new Error('HTTP ' + res.status);
      return res.json();
    }).then(function (v) {
      if (S.gone) return;
      S.loaded = true;
      S.run = normaliseRun(v && v.run);
      S.version = (v && v.version) || 0;
      S.ready = (v && v.ready) || [];
      S.blocks = (v && v.statBlocks) || [];
      S.leaves = (v && v.leaves) || {};
      S.dirty = false;
      S.confirmEnd = false;
      draw(S);
      status(S, note);
    }).catch(function () {
      if (S.gone) return;
      S.loaded = true;
      S.failed = true;
      draw(S);
    });
  }

  function change(S, next) {
    S.run = next;
    S.dirty = true;
    clearTimeout(S.timer);
    S.timer = setTimeout(function () { save(S); }, SAVE_DELAY);
    draw(S);
  }

  // save sends the whole fight with the version it was read at. A change
  // made while a save is in flight goes in a second save after it.
  function save(S) {
    clearTimeout(S.timer);
    S.timer = 0;
    if (S.saving) { if (S.dirty) S.again = true; return S.saving; }
    if (!S.dirty || !S.run) return Promise.resolve();
    S.dirty = false;
    status(S, 'Saving\u2026');
    var p = C.apiFetch(S.url, { method: 'PUT', body: { version: S.version, run: S.run } }).then(function (res) {
      if (res.status === 409) {
        if (!S.gone) return load(S, 'Changed elsewhere, so this is the latest.', true);
        return null;
      }
      if (!res.ok) throw new Error('HTTP ' + res.status);
      return res.json().then(function (v) {
        S.version = (v && v.version) || S.version;
        status(S, 'Saved');
      });
    }).catch(function () {
      S.dirty = true;
      status(S, 'Couldn\u2019t save. It tries again with your next change.');
    }).then(function () {
      S.saving = null;
      if (inflight[S.cid] === p) delete inflight[S.cid];
      if (S.again) { S.again = false; return save(S); }
      return null;
    });
    S.saving = p;
    inflight[S.cid] = p;
    return p;
  }

  // ---------- Drawing ----------
  var seq = 0;

  function mode(S) {
    if (!S.loaded) return 'loading';
    if (S.failed) return 'failed';
    return S.run ? 'run' : 'none';
  }

  function draw(S) {
    var m = mode(S);
    if (S.mode !== m) {
      Object.keys(S.folds).forEach(function (id) { C.CreatureFold.dispose(S.folds[id]); });
      S.folds = {}; S.nodes = {}; S.open = {};
      S.el.textContent = '';
      S.mode = m;
      if (m === 'loading') S.el.appendChild(h('p', 'erun-note', 'Loading the fight\u2026'));
      if (m === 'failed') {
        S.el.appendChild(h('p', 'erun-note', 'Couldn\u2019t load the fight.'));
        S.el.appendChild(btn('', 'Try again', 'retry'));
      }
      if (m === 'none') S.el.appendChild(h('div', 'erun-none'));
      if (m === 'run') skeleton(S);
      if (m === 'none' || m === 'run') S.el.appendChild(foot());
    }
    if (m === 'none') drawReady(S);
    if (m === 'run') drawRun(S);
  }

  function foot() {
    var f = h('div', 'erun-foot');
    var st = h('span', 'erun-status');
    st.setAttribute('role', 'status');
    st.setAttribute('aria-live', 'polite');
    f.appendChild(st);
    return f;
  }

  // ended is the report of the fight just ended: a line per creature and,
  // when something was left, the way to it. It stays until Done or the
  // next fight.
  function ended(S) {
    var r = S.report, box = h('div', 'erun-ended');
    box.setAttribute('role', 'status');
    box.appendChild(h('b', '', (r.name || 'The fight') + ' is over.'));
    var ul = h('ul');
    r.lines.forEach(function (t) { ul.appendChild(h('li', '', t)); });
    box.appendChild(ul);
    var foot = h('div', 'erun-ended-foot');
    if (r.link && r.link.charAt(0) === '/' && r.link.charAt(1) !== '/') {
      var a = h('a', 'erun-btn erun-go', r.linkText || 'Open');
      a.href = r.link;
      foot.appendChild(a);
    }
    if (r.note) foot.appendChild(h('span', 'erun-note', r.note));
    foot.appendChild(btn('', 'Done', 'report-done'));
    box.appendChild(foot);
    return box;
  }

  function drawReady(S) {
    var box = S.el.querySelector('.erun-none');
    box.textContent = '';
    if (S.report) box.appendChild(ended(S));
    box.appendChild(h('p', 'erun-lbl', 'Ready to run'));
    if (!S.ready.length) {
      box.appendChild(h('p', 'erun-note', 'No encounter is marked Ready. Open an encounter on its page, turn on Ready, and it shows here.'));
      return;
    }
    var ul = h('ul', 'erun-ready');
    S.ready.forEach(function (e) {
      var li = h('li', 'erun-pick');
      var txt = h('span', 'erun-pick-txt');
      txt.appendChild(h('b', '', e.name || 'Untitled encounter'));
      var n = e.creatureCount || 0;
      var sub = (e.page && e.page.name ? 'On ' + e.page.name + ' \u00b7 ' : '') + n + (n === 1 ? ' creature' : ' creatures');
      txt.appendChild(h('span', 'erun-sub', sub));
      li.appendChild(txt);
      var b = btn('erun-go', 'Start', 'start');
      b.setAttribute('data-id', e.id);
      b.setAttribute('aria-label', 'Start ' + (e.name || 'this encounter'));
      li.appendChild(b);
      ul.appendChild(li);
    });
    box.appendChild(ul);
  }

  function skeleton(S) {
    var bar = h('div', 'erun-bar');
    bar.appendChild(h('b', 'erun-name'));
    bar.appendChild(h('span', 'erun-round'));
    bar.appendChild(btn('erun-go', 'Next turn', 'next'));
    S.el.appendChild(bar);
    S.el.appendChild(h('p', 'erun-note erun-hint', 'Type each side\u2019s initiative, then press First turn.'));
    var list = h('ol', 'erun-order');
    list.setAttribute('aria-label', 'Turn order');
    S.el.appendChild(list);
    S.el.appendChild(h('p', 'erun-note erun-empty', 'No one is in this fight.'));
    var wait = h('div', 'erun-wait');
    wait.appendChild(h('p', 'erun-lbl', 'Arriving later'));
    wait.appendChild(h('ul', 'erun-waiting'));
    S.el.appendChild(wait);
    var dl = h('datalist');
    dl.id = 'erun-conds-' + (++seq);
    S.conds.forEach(function (c) { var o = h('option'); o.value = String(c); dl.appendChild(o); });
    S.el.appendChild(dl);
    S.listId = dl.id;
    var end = h('div', 'erun-endbox');
    end.appendChild(btn('erun-end', 'End the fight', 'end'));
    var ask = h('div', 'erun-confirm');
    ask.setAttribute('role', 'group');
    ask.setAttribute('aria-label', 'End the fight');
    ask.appendChild(h('p', '', 'End this fight? Its turn order, hit points and conditions are cleared. The encounter itself stays as it is.'));
    ask.appendChild(btn('erun-end erun-armed', 'End it', 'end-yes'));
    ask.appendChild(btn('', 'Keep going', 'end-no'));
    end.appendChild(ask);
    S.el.appendChild(end);
  }

  function drawRun(S) {
    var run = S.run, el = S.el;
    el.querySelector('.erun-name').textContent = run.name || 'Untitled encounter';
    el.querySelector('.erun-round').textContent = run.turn ? 'Round ' + run.round : 'Not started';
    var go = el.querySelector('.erun-bar .erun-go');
    go.textContent = run.turn ? 'Next turn' : 'First turn';
    var lines = shown(run);
    go.disabled = !lines.length;
    el.querySelector('.erun-hint').hidden = !!run.turn || !lines.length;
    el.querySelector('.erun-empty').hidden = lines.length > 0 || run.lines.length > 0;

    var active = document.activeElement;
    var inside = active && el.contains(active);
    var list = el.querySelector('.erun-order');
    var keep = {};
    lines.forEach(function (l) { keep[l.id] = true; });
    Object.keys(S.nodes).forEach(function (id) {
      if (!keep[id]) { S.nodes[id].remove(); delete S.nodes[id]; }
    });
    lines.forEach(function (l, i) {
      var node = S.nodes[l.id] || (S.nodes[l.id] = makeLine(S, l));
      updateLine(S, node, l);
      if (list.children[i] !== node) list.insertBefore(node, list.children[i] || null);
    });
    if (inside && active.isConnected && document.activeElement !== active) active.focus({ preventScroll: true });

    var waiting = run.lines.filter(function (l) { return l.waiting; });
    var wait = el.querySelector('.erun-wait');
    wait.hidden = !waiting.length;
    var ul = wait.querySelector('.erun-waiting');
    ul.textContent = '';
    waiting.forEach(function (l) {
      var li = h('li', 'erun-pick');
      li.setAttribute('data-line', l.id);
      li.appendChild(h('span', 'erun-pick-txt', l.name + (l.hp.length > 1 ? ' \u00d7' + l.hp.length : '')));
      var b = btn('', 'Bring in', 'in');
      b.setAttribute('aria-label', 'Bring in ' + l.name);
      li.appendChild(b);
      ul.appendChild(li);
    });

    var box = el.querySelector('.erun-endbox');
    box.querySelector('[data-act="end"]').hidden = S.confirmEnd;
    box.querySelector('.erun-confirm').hidden = !S.confirmEnd;
  }

  function makeLine(S, l) {
    var li = h('li', 'erun-line');
    li.setAttribute('data-line', l.id);
    var row = h('div', 'erun-row');
    var init = numInput('erun-init', 'Initiative for ' + l.name, l.init);
    init.setAttribute('data-f', 'init');
    init.placeholder = '\u2013';
    row.appendChild(init);
    // The boxes sit beside the name while they fit and wrap under it when
    // they don't, so a name is never squeezed to a few letters a line.
    var main = h('div', 'erun-main');
    main.appendChild(h('div', 'erun-who'));
    main.appendChild(h('div', 'erun-hp'));
    row.appendChild(main);
    var more = h('button', 'erun-more');
    more.type = 'button';
    more.setAttribute('data-act', 'more');
    more.setAttribute('aria-label', 'Hit points and conditions for ' + l.name);
    more.title = 'Hit points and conditions';
    var ic = h('i', 'fa-solid fa-ellipsis');
    ic.setAttribute('aria-hidden', 'true');
    more.appendChild(ic);
    row.appendChild(more);
    li.appendChild(row);
    return li;
  }

  function updateLine(S, li, l) {
    var now = S.run.turn === l.id;
    li.classList.toggle('erun-now', now);
    if (now) li.setAttribute('aria-current', 'true'); else li.removeAttribute('aria-current');
    setValue(li.querySelector('.erun-init'), l.init);

    var who = li.querySelector('.erun-who');
    var fold = S.folds[l.id];
    // Rebuilt only when what it shows changes: a change event can redraw
    // while Tab is moving focus into it, and a replaced button drops focus.
    var drop = dropNote(S.leaves, l);
    var sig = JSON.stringify([l.name, l.kind, l.entityId, l.hp.length, l.conds, !!fold, drop]);
    if (who.getAttribute('data-sig') !== sig) {
      who.setAttribute('data-sig', sig);
      drawWho(S, who, l, fold, drop);
    }

    drawHP(li.querySelector('.erun-hp'), l);

    var open = !!S.open[l.id];
    li.querySelector('.erun-more').setAttribute('aria-expanded', open ? 'true' : 'false');
    var det = li.querySelector('.erun-detail');
    if (open && !det) det = li.appendChild(makeDetail(S, l));
    if (!open && det) { det.remove(); det = null; }
    if (det) setValue(det.querySelector('.erun-max'), l.max);
    // The fold sits last in the line, under its details.
    if (fold && li.lastChild !== fold) li.appendChild(fold);
  }

  function drawWho(S, who, l, fold, drop) {
    who.textContent = '';
    if (l.kind === 'creature' && l.entityId) {
      who.appendChild(C.CreatureFold.nameButton(l.name, !!fold, fold && fold.id, function () { toggleFold(S, l.id); }));
    } else {
      who.appendChild(h('b', 'erun-nm', l.name));
    }
    if (l.hp.length > 1) who.appendChild(h('span', 'erun-x', '\u00d7' + l.hp.length));
    if (drop) who.appendChild(h('span', 'erun-drop' + (drop.indexOf('No ') === 0 ? '' : ' erun-has'), drop));
    l.conds.forEach(function (c) {
      var chip = h('button', 'erun-cond', c);
      chip.type = 'button';
      chip.setAttribute('data-act', 'cond-rm');
      chip.setAttribute('data-cond', c);
      chip.setAttribute('aria-label', 'Remove ' + c + ' from ' + l.name);
      chip.title = 'Remove';
      who.appendChild(chip);
    });
  }

  // drawHP keeps the boxes while their number and shape hold, so a box the
  // DM is typing in is never replaced under them.
  function drawHP(box, l) {
    var shape = l.hp.length + ':' + (l.max !== null);
    if (box.getAttribute('data-shape') !== shape) {
      box.setAttribute('data-shape', shape);
      box.textContent = '';
      l.hp.forEach(function (_, i) {
        var label = l.hp.length > 1 ? l.name + ' ' + (i + 1) + ', hit points' : l.name + ', hit points';
        var inp = numInput('erun-box', label, null);
        inp.setAttribute('data-f', 'hp');
        inp.setAttribute('data-box', String(i));
        box.appendChild(inp);
      });
      if (l.hp.length === 1 && l.max !== null) box.appendChild(h('span', 'erun-of'));
    }
    box.querySelectorAll('.erun-box').forEach(function (inp, i) {
      setValue(inp, current(l, i));
      inp.placeholder = l.max === null ? 'HP' : String(l.max);
      inp.classList.toggle('erun-down', isDown(l, i));
    });
    var of = box.querySelector('.erun-of');
    if (of) of.textContent = '/' + l.max;
  }

  function makeDetail(S, l) {
    var d = h('div', 'erun-detail');
    var lab = h('label', 'erun-field');
    lab.appendChild(h('span', '', l.hp.length > 1 ? 'Full hit points, each' : 'Full hit points'));
    var max = numInput('erun-max', '', l.max);
    max.removeAttribute('aria-label');
    max.setAttribute('data-f', 'max');
    lab.appendChild(max);
    d.appendChild(lab);
    var add = h('div', 'erun-condadd');
    var inp = h('input', 'erun-condin');
    inp.type = 'text';
    inp.maxLength = LIM.cond;
    inp.setAttribute('autocomplete', 'off');
    inp.setAttribute('list', S.listId);
    inp.setAttribute('aria-label', 'Add a condition to ' + l.name);
    inp.placeholder = S.conds.length ? 'Add a condition' : 'Add a condition (type it)';
    add.appendChild(inp);
    add.appendChild(btn('', 'Add', 'cond-add'));
    d.appendChild(add);
    return d;
  }

  function toggleFold(S, id) {
    var l = lineOf(S, id);
    if (!l) return;
    if (S.folds[id]) {
      C.CreatureFold.dispose(S.folds[id]);
      delete S.folds[id];
    } else {
      S.folds[id] = C.CreatureFold.build({ campaignId: S.cid, entityId: l.entityId, name: l.name, statBlocks: S.blocks });
    }
    draw(S);
    var li = S.nodes[id], b = li && li.querySelector('.cf-name');
    if (b) b.focus({ preventScroll: true });
  }

  function lineOf(S, id) {
    var ls = (S.run && S.run.lines) || [];
    for (var i = 0; i < ls.length; i++) if (ls[i].id === id) return ls[i];
    return null;
  }

  // ---------- Events ----------
  function lineId(t) {
    var li = t.closest('[data-line]');
    return li ? li.getAttribute('data-line') : '';
  }

  function onChange(S, e) {
    var t = e.target, f = t.getAttribute && t.getAttribute('data-f');
    if (!f || !S.run) return;
    var id = lineId(t), l = lineOf(S, id);
    if (!l) return;
    var v, next;
    if (f === 'init') {
      v = parseNum(t.value, LIM.initLo, LIM.initHi);
      if (v !== undefined && v !== l.init) next = setInit(S.run, id, v);
      if (v === undefined) t.value = l.init === null ? '' : String(l.init);
    } else if (f === 'hp') {
      var i = parseInt(t.getAttribute('data-box'), 10);
      v = parseNum(t.value, -LIM.hp, LIM.hp);
      // Typing the full number back reads as untouched.
      if (v !== undefined && v !== null && v === l.max) v = null;
      if (v !== undefined && v !== l.hp[i]) next = setHP(S.run, id, i, v);
      if (v === undefined) t.value = current(l, i) === null ? '' : String(current(l, i));
    } else if (f === 'max') {
      v = parseNum(t.value, 0, LIM.hp);
      if (v !== undefined && v !== l.max) next = setMax(S.run, id, v);
      if (v === undefined) t.value = l.max === null ? '' : String(l.max);
    }
    if (next) change(S, next);
    else draw(S);
  }

  function onKey(S, e) {
    if (e.key !== 'Enter') return;
    var t = e.target;
    if (t.classList.contains('erun-condin')) {
      e.preventDefault();
      addFrom(S, t);
    } else if (t.getAttribute('data-f')) {
      e.preventDefault();
      onChange(S, { target: t });
    }
  }

  function addFrom(S, input) {
    var id = lineId(input);
    if (!input.value.trim()) return;
    change(S, addCond(S.run, id, input.value));
    input.value = '';
    input.focus({ preventScroll: true });
  }

  function onClick(S, e) {
    var b = e.target.closest('[data-act]');
    if (!b || !S.el.contains(b)) return;
    var act = b.getAttribute('data-act'), id = lineId(b);
    if (act === 'retry') { S.loaded = false; S.failed = false; draw(S); load(S, ''); return; }
    if (act === 'start') { start(S, b.getAttribute('data-id'), b); return; }
    if (act === 'report-done') { S.report = null; draw(S); return; }
    if (!S.run) return;
    if (act === 'next') {
      change(S, nextTurn(S.run));
      var now = S.nodes[S.run.turn];
      if (now && now.scrollIntoView) now.scrollIntoView({ block: 'nearest' });
    } else if (act === 'more') {
      S.open[id] = !S.open[id];
      draw(S);
      var max = S.open[id] && S.nodes[id] && S.nodes[id].querySelector('.erun-max');
      if (max) max.focus({ preventScroll: true });
    } else if (act === 'cond-add') {
      var inp = S.nodes[id] && S.nodes[id].querySelector('.erun-condin');
      if (inp) addFrom(S, inp);
    } else if (act === 'cond-rm') {
      change(S, removeCond(S.run, id, b.getAttribute('data-cond')));
      var more = S.nodes[id] && S.nodes[id].querySelector('.erun-more');
      if (more) more.focus({ preventScroll: true });
    } else if (act === 'in') {
      change(S, bringIn(S.run, id));
    } else if (act === 'end') {
      S.confirmEnd = true;
      draw(S);
      S.el.querySelector('[data-act="end-no"]').focus({ preventScroll: true });
    } else if (act === 'end-no') {
      S.confirmEnd = false;
      draw(S);
      S.el.querySelector('[data-act="end"]').focus({ preventScroll: true });
    } else if (act === 'end-yes') {
      end(S, b);
    }
  }

  function start(S, encounterId, b) {
    b.disabled = true;
    S.report = null;
    C.apiFetch(S.url, { method: 'POST', body: { encounterId: encounterId } }).then(function (res) {
      if (res.status === 409) return load(S, 'A fight was already on, so this is that one.', true);
      if (!res.ok) throw new Error('HTTP ' + res.status);
      return load(S, 'The fight is on.', true);
    }).catch(function () {
      b.disabled = false;
      status(S, 'Couldn\u2019t start the fight. Try again.');
    });
  }

  // end saves the last change first, since a box set to 0 a moment ago is
  // what makes that creature drop its spoils; a save that fails stops the
  // end, so the fight isn't over on numbers the server never got.
  function end(S, b) {
    b.disabled = true;
    save(S).then(function () {
      if (S.dirty) throw new Error('unsaved');
      return C.apiFetch(S.url, { method: 'DELETE' });
    }).then(function (res) {
      if (!res.ok && res.status !== 404) throw new Error('HTTP ' + res.status);
      return res.status === 404 ? null : res.json().catch(function () { return null; });
    }).then(function (rep) {
      S.report = rep && Array.isArray(rep.lines) && rep.lines.length ? rep : null;
      return load(S, 'The fight is over.', true);
    }).catch(function () {
      b.disabled = false;
      status(S, 'Couldn\u2019t end the fight. Try again.');
    });
  }
})();
