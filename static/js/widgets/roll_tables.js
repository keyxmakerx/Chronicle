/**
 * roll_tables.js -- rolling tables: rumours, omens, names and the rest.
 *
 * Two homes, one engine. A page holds a roller placed with /roll (the
 * editor node in editor_rolltable.js mounts mountBlock); a calendar day card
 * gets "Roll one" (calendar_view.js mounts mountDay). Rolls come from
 * ChronicleGen.tables (chronicle_gen.js, loaded on first use): its starter
 * tables plus the campaign's own, kept at /campaigns/:id/roll-tables.
 * Starter tables are never edited in place; a copy becomes the campaign's
 * own, so an update can improve the starters without overwriting changes.
 *
 * Chronicle.RollTables:
 *   .engine()                    promise of ChronicleGen
 *   .mine(campaignId)            promise of the campaign's own tables ([] when none or not allowed)
 *   .save(campaignId, tables)    replace the campaign's own tables
 *   .roll(G, mine, id, opts)     one result {name, brief, kind, moon}; opts {calendar, date}
 *   .mountBlock(host, opts)      the page roller (see mountBlock)
 *   .mountDay(host, opts)        the day roller (see mountDay)
 */
(function () {
  'use strict';

  // The page hands over the engine's versioned URL; only that one file, with
  // an optional ?v= token, is ever loaded from it.
  var ENGINE_PATH = '/static/js/widgets/chronicle_gen.js';
  var SELF = document.currentScript;
  var ENGINE_V = /^\/static\/js\/widgets\/chronicle_gen\.js\?v=([A-Za-z0-9._-]{1,80})$/.exec((SELF && SELF.getAttribute('data-engine-src')) || '');
  var ENGINE_SRC = ENGINE_PATH + (ENGINE_V ? '?v=' + encodeURIComponent(ENGINE_V[1]) : '');
  var ID_RE = /^[a-z][a-z0-9-]{0,63}$/;
  var NAMES = { 'names-people': 'People', 'names-places': 'Places' };
  // Pages have no calendar of their own; with this one, entries that need a
  // moon or a season simply don't come up.
  var PAGE_CAL = { months: [{ name: 'Month', days: 30 }], weekdays: [{ name: 'Day' }], moons: [], seasons: [], current_year: 1, current_month: 1, current_day: 1 };

  function esc(s) { return window.Chronicle && Chronicle.escapeHtml ? Chronicle.escapeHtml(String(s == null ? '' : s)) : String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
  function reduced() {
    return matchMedia('(prefers-reduced-motion: reduce)').matches || !!document.querySelector('.nav-rm, html.rm');
  }
  function notify(msg, kind) { if (window.Chronicle && Chronicle.notify) Chronicle.notify(msg, kind || 'success'); }

  var enginePromise = null;
  function engine() {
    if (window.ChronicleGen) return Promise.resolve(window.ChronicleGen);
    if (enginePromise) return enginePromise;
    enginePromise = new Promise(function (resolve, reject) {
      var s = document.createElement('script');
      s.src = ENGINE_SRC;
      s.onload = function () { window.ChronicleGen ? resolve(window.ChronicleGen) : reject(new Error('engine missing')); };
      s.onerror = function () { enginePromise = null; reject(new Error('engine failed to load')); };
      document.head.appendChild(s);
    });
    return enginePromise;
  }

  var mineCache = {};
  function base(campaignId) { return '/campaigns/' + encodeURIComponent(campaignId) + '/roll-tables'; }
  function mine(campaignId) {
    if (!campaignId) return Promise.resolve([]);
    if (mineCache[campaignId]) return mineCache[campaignId];
    mineCache[campaignId] = Chronicle.apiFetch(base(campaignId)).then(function (r) {
      if (!r.ok) return [];
      return r.json().then(function (j) { return (j && Array.isArray(j.tables)) ? j.tables : []; });
    }).catch(function () { delete mineCache[campaignId]; return []; });
    return mineCache[campaignId];
  }
  function save(campaignId, tables) {
    return Chronicle.apiFetch(base(campaignId), { method: 'PUT', body: { tables: tables } }).then(function (r) {
      if (!r.ok) {
        return r.json().catch(function () { return {}; }).then(function (j) { throw new Error((j && (j.message || j.error)) || 'The table couldn’t be saved.'); });
      }
      return r.json().then(function (j) {
        var t = (j && Array.isArray(j.tables)) ? j.tables : tables;
        mineCache[campaignId] = Promise.resolve(t);
        return t;
      });
    });
  }

  function own(list, id) { for (var i = 0; i < list.length; i++) if (list[i].id === id) return list[i]; return null; }
  function starter(G, id) { var t = G._internal.STARTER_TABLES.tables; for (var i = 0; i < t.length; i++) if (t[i].id === id) return t[i]; return null; }
  function tableName(G, list, id) {
    if (NAMES[id]) return NAMES[id];
    var t = own(list, id) || starter(G, id);
    return t ? t.name : 'A table that was deleted';
  }

  /* The campaign's tables join the starters in one set, so their {braces}
     can roll on a starter table ({trade}, {building}) as the starters do. */
  function setWith(G, list) {
    var tables = G._internal.STARTER_TABLES.tables.slice();
    list.forEach(function (t) {
      tables.push({ id: t.id, name: t.name, output: { kind: 'quest' }, entries: t.entries.map(function (e) {
        return { weight: e.weight == null ? 1 : e.weight, name: e.name, brief: e.brief || e.name };
      }) });
    });
    return { format: G.tables.format, version: 1, id: 'campaign', name: 'Campaign tables', tables: tables };
  }

  var seq = 0;
  function roll(G, list, id, opts) {
    opts = opts || {};
    if (NAMES[id]) {
      var themes = G.recipes.builtIn('names');
      var made = G.names.generate({ seed: G.randomSeed(), recipe: themes[0] }).names;
      var pool = id === 'names-people' ? made.people : made.places;
      var v = pool[Math.floor(Math.random() * pool.length)];
      return { name: typeof v === 'string' ? v : v.name, brief: null };
    }
    var setRef = own(list, id) ? setWith(G, list) : 'starter';
    var r;
    try {
      r = G.tables.roll(setRef, id, { calendar: opts.calendar || PAGE_CAL, date: opts.date, seed: G.randomSeed(), n: seq++ });
    } catch (e) {
      return { name: 'This table has a problem', brief: e.message, problem: true };
    }
    if (!r) return { name: 'Nothing on this table fits here', brief: 'Every line needs something this day or page doesn’t have, like a full moon.', problem: true };
    if (r.name) return { name: r.name, brief: r.brief, kind: r.kind, moon: r.moon };
    var t = r.text || '';
    return { name: t.charAt(0).toUpperCase() + t.slice(1), brief: null };
  }

  /* A few real entries flick past, slowing, before the result lands. */
  function reel(box, next, finalHTML, done) {
    if (reduced()) { box.innerHTML = finalHTML; if (done) done(); return; }
    var steps = [40, 55, 75, 100, 140], i = 0;
    (function tick() {
      if (i >= steps.length) {
        box.innerHTML = '<span class="rt-reel rt-reel--land">' + finalHTML + '</span>';
        if (done) done();
        return;
      }
      box.innerHTML = '<span class="rt-reel rt-reel--flick" style="animation-duration:' + steps[i] + 'ms"><b>' + esc(next().name) + '</b></span>';
      setTimeout(tick, steps[i++]);
    })();
  }

  /* The picker's groups: the campaign's own first, then the starters by use. */
  function catalog(G, list) {
    var happen = [], words = [], customs = [];
    G._internal.STARTER_TABLES.tables.forEach(function (t) {
      var o = { id: t.id, name: t.name, n: t.entries.length };
      if (/^customs-/.test(t.id)) customs.push(o); else if (t.output) happen.push(o); else words.push(o);
    });
    return [
      ['Your tables', list.map(function (t) { return { id: t.id, name: t.name, n: t.entries.length }; })],
      ['Happenings', happen],
      ['Names', [{ id: 'names-people', name: 'People', names: true }, { id: 'names-places', name: 'Places', names: true }]],
      ['Words', words],
      ['Customs and festivals', customs]
    ];
  }

  function entriesFor(G, list, id) {
    var o = own(list, id);
    if (o) return o.entries.map(function (e) { return { name: e.name, brief: e.brief || '', weight: e.weight == null ? 1 : e.weight }; });
    var t = starter(G, id);
    if (!t) return [];
    return t.entries.map(function (e) {
      if (typeof e === 'string') return { name: e, brief: '', weight: 1 };
      return { name: e.name || e.text || ('Roll on ' + e.roll), brief: e.brief || '', weight: e.weight == null ? 1 : e.weight };
    });
  }
  function freeId(G, list, want) {
    var id = String(want || 'table').toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/^[^a-z]+/, '').slice(0, 50) || 'table';
    var taken = function (x) { return !!own(list, x) || !!starter(G, x) || !!NAMES[x]; };
    if (!taken(id)) return id;
    for (var n = 2; ; n++) if (!taken(id + '-' + n)) return id + '-' + n;
  }
  function weightLabel(w) { return w >= 3 ? 'Very often' : w >= 2 ? 'Often' : 'Rare'; }
  function braces(s) { return esc(s).replace(/\{([A-Za-z][\w-]*)(\|[a-z]+)?\}/g, '<span class="rt-brace">{$1}</span>'); }

  /**
   * The page roller. opts:
   *   campaignId, table, count     what the node holds
   *   onAttrs(table, count)        store a new choice in the page (ignored when not editing)
   *   onPut(rows)                  put the results into the page above the roller
   * Returns {destroy, update(table, count)}.
   */
  function mountBlock(host, opts) {
    var S = { table: opts.table || 'rumours', count: opts.count || 3, rows: [], list: [], G: null, editing: false, dirty: false, picking: false, draft: null, saving: false };
    var B = document.createElement('div');
    B.className = 'rt-block';
    host.appendChild(B);
    B.innerHTML = '<div class="rt-loading"><i class="fa-solid fa-dice-d20 fa-spin"></i> Getting the tables ready…</div>';

    function anyPin() { return S.rows.some(function (r) { return r.pin; }); }
    function cell(r) { return '<b>' + esc(r.name) + '</b>' + (r.brief ? '<span>' + esc(r.brief) + '</span>' : ''); }
    function next() { return roll(S.G, S.list, S.table); }
    function fresh(taken) {
      var r, k = 0;
      do { r = next(); } while (k++ < 10 && taken.some(function (x) { return x && x.name === r.name; }));
      return r;
    }
    function isOwn() { return !!own(S.list, S.table); }
    function srcLabel() { return isOwn() ? 'Your table' : NAMES[S.table] ? 'Names' : 'Starter table'; }
    function rowHTML(r, i) {
      return '<div class="rt-row' + (r.pin ? ' is-pinned' : '') + '" data-i="' + i + '"><div class="rt-txt" data-box>' + cell(r) + '</div>' +
        '<button type="button" class="rt-btn" data-pin aria-pressed="' + !!r.pin + '" title="' + (r.pin ? 'Unpin, so it rolls again' : 'Pin: keep this one') + '" aria-label="' + (r.pin ? 'Unpin ' : 'Pin ') + esc(r.name) + '"><i class="fa-solid fa-thumbtack"></i></button>' +
        '<button type="button" class="rt-btn" data-one title="Roll just this one" aria-label="Roll again instead of ' + esc(r.name) + '"' + (r.pin ? ' disabled' : '') + '><i class="fa-solid fa-dice"></i></button></div>';
    }
    function draw() {
      var names = !!NAMES[S.table];
      B.innerHTML =
        '<div class="rt-head"><span class="rt-ic" aria-hidden="true"><i class="fa-solid fa-dice-d20"></i></span>' +
        '<button type="button" class="rt-tbl" data-pick aria-haspopup="listbox" aria-expanded="' + S.picking + '">' + esc(tableName(S.G, S.list, S.table)) + ' <i class="fa-solid fa-chevron-down" aria-hidden="true"></i></button>' +
        '<span class="rt-src">' + srcLabel() + '</span><span class="rt-sp"></span>' +
        '<span class="rt-seg" role="group" aria-label="How many to roll">' + [1, 3, 5].map(function (n) { return '<button type="button" data-n="' + n + '" aria-pressed="' + (S.count === n) + '">' + n + '</button>'; }).join('') + '</span>' +
        (names ? '' : '<button type="button" class="rt-ghost" data-edit aria-expanded="' + S.editing + '"><i class="fa-solid fa-pen" aria-hidden="true"></i>' + (S.editing ? 'Close table' : 'Edit table') + '</button>') + '</div>' +
        '<div class="rt-ed" data-ed></div>' +
        (S.editing ? '' : '<div class="rt-rows" aria-live="polite">' + S.rows.map(rowHTML).join('') + '</div>') +
        '<div class="rt-warn" data-warn role="alert"><i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i><span>Not saved yet. Save the table, or throw the changes away.</span><button type="button" data-discard>Discard</button></div>' +
        (S.editing ? '' : '<div class="rt-foot"><button type="button" class="rt-main" data-roll><i class="fa-solid fa-dice" aria-hidden="true"></i>' + (anyPin() ? 'Reroll the rest' : 'Roll') + '</button>' +
          '<button type="button" class="rt-sec" data-put><i class="fa-solid fa-arrow-turn-up" aria-hidden="true"></i>Put in page</button>' +
          '<span class="rt-sp"></span><span class="rt-note"><i class="fa-solid fa-eye-slash" aria-hidden="true"></i> Players don’t see the roller, only what you put in the page</span></div>');
      if (S.editing) drawEditor(true);
      if (S.picking) drawPicker('');
    }
    function rollAll(quiet) {
      var keep = S.rows.slice(), pinned = keep.filter(function (r) { return r.pin; });
      S.rows = [];
      for (var i = 0; i < S.count; i++) S.rows.push(keep[i] && keep[i].pin ? keep[i] : fresh(pinned.concat(S.rows)));
      draw();
      if (quiet) return;
      B.querySelectorAll('.rt-row').forEach(function (row) {
        var r = S.rows[+row.getAttribute('data-i')];
        if (r.pin) return;
        reel(row.querySelector('[data-box]'), next, cell(r), function () { row.classList.add('is-landed'); });
      });
    }
    function rollOne(i) {
      var r = fresh(S.rows);
      S.rows[i] = r;
      var row = B.querySelector('.rt-row[data-i="' + i + '"]');
      row.outerHTML = rowHTML(r, i);
      row = B.querySelector('.rt-row[data-i="' + i + '"]');
      reel(row.querySelector('[data-box]'), next, cell(r), function () { row.classList.add('is-landed'); });
    }

    function drawPicker(q) {
      var old = B.querySelector('.rt-pick');
      if (old) old.remove();
      q = (q || '').toLowerCase();
      var h = '<div class="rt-pick" role="listbox" aria-label="Choose a table"><input type="search" placeholder="Find a table" aria-label="Find a table" data-q value="' + esc(q) + '">';
      catalog(S.G, S.list).forEach(function (g) {
        var items = g[1].filter(function (t) { return !q || t.name.toLowerCase().indexOf(q) >= 0; });
        if (!items.length && !(g[0] === 'Your tables' && !q)) return;
        h += '<h6>' + g[0] + '</h6>';
        if (!items.length) h += '<p class="rt-note rt-pick-empty">None yet. Copy a starter table, or start a new one.</p>';
        h += items.map(function (t) {
          return '<button type="button" role="option" data-t="' + esc(t.id) + '" aria-selected="' + (t.id === S.table) + '">' + esc(t.name) + '<small>' + (t.names ? 'from the name generator' : t.n + ' lines') + '</small></button>';
        }).join('');
      });
      h += '<hr><button type="button" class="rt-pick-new" data-new><i class="fa-solid fa-plus" aria-hidden="true"></i>New table</button></div>';
      B.insertAdjacentHTML('beforeend', h);
      var inp = B.querySelector('[data-q]');
      inp.focus();
      inp.setSelectionRange(q.length, q.length);
    }

    /* Editing opens inside the roller. A draft holds the changes until Save. */
    function drawEditor(instant) {
      var ed = B.querySelector('[data-ed]'), mineT = !!S.draft;
      var es = mineT ? S.draft.entries : entriesFor(S.G, S.list, S.table);
      var h = '';
      if (!mineT) {
        h += '<div class="rt-banner"><i class="fa-solid fa-lock" aria-hidden="true"></i><span>Starter tables stay as they are, so updates can improve them. Make a copy to change this one.</span><button type="button" class="rt-main" data-copy><i class="fa-solid fa-copy" aria-hidden="true"></i>Make a copy</button></div>';
      } else {
        h += '<label class="rt-name">Table name <input data-tname maxlength="120" value="' + esc(S.draft.name) + '"></label>';
      }
      h += '<div class="rt-ents">' + es.map(function (e, i) {
        if (!mineT) {
          return '<div class="rt-ent is-locked"><span class="rt-w">' + weightLabel(e.weight) + '</span><div><div class="rt-en">' + braces(e.name) + '</div>' + (e.brief ? '<div class="rt-eb">' + braces(e.brief) + '</div>' : '') + '</div><span></span></div>';
        }
        return '<div class="rt-ent" data-e="' + i + '"><select aria-label="How often" data-w>' + [1, 2, 3].map(function (w) {
          return '<option value="' + w + '"' + (Math.min(3, Math.max(1, Math.round(e.weight))) === w ? ' selected' : '') + '>' + weightLabel(w) + '</option>';
        }).join('') + '</select><div><input data-n maxlength="200" aria-label="Name" placeholder="What happens, in a few words" value="' + esc(e.name) + '"><textarea data-b maxlength="1000" rows="1" aria-label="More detail" placeholder="One line a DM can use as it stands">' + esc(e.brief) + '</textarea></div>' +
          '<button type="button" class="rt-btn" data-del title="Remove this line" aria-label="Remove this line"><i class="fa-solid fa-trash-can"></i></button></div>';
      }).join('') + '</div>';
      h += '<div class="rt-foot">' + (mineT ? '<button type="button" class="rt-main" data-save' + (S.saving ? ' disabled' : '') + '><i class="fa-solid fa-check" aria-hidden="true"></i>' + (S.saving ? 'Saving…' : 'Save table') + '</button><button type="button" class="rt-sec" data-add><i class="fa-solid fa-plus" aria-hidden="true"></i>Add a line</button>' : '') +
        '<span class="rt-sp"></span><span class="rt-note">Words in <span class="rt-brace">{braces}</span> roll on another table, like <span class="rt-brace">{trade}</span>.</span></div>';
      ed.innerHTML = h;
      ed.querySelectorAll('textarea').forEach(function (t) { t.style.height = (t.scrollHeight + 2) + 'px'; });
      if (instant || reduced()) { ed.style.height = 'auto'; return; }
      var to = ed.scrollHeight;
      ed.style.height = '0px'; void ed.offsetHeight; ed.style.height = to + 'px';
      setTimeout(function () { ed.style.height = 'auto'; }, 340);
    }
    function readDraft() {
      if (!S.draft) return;
      var n = B.querySelector('[data-tname]');
      if (n) S.draft.name = n.value;
      S.draft.entries = [].map.call(B.querySelectorAll('.rt-ent[data-e]'), function (r) {
        return { weight: +r.querySelector('[data-w]').value, name: r.querySelector('[data-n]').value, brief: r.querySelector('[data-b]').value };
      });
    }
    function closeEditor() {
      var ed = B.querySelector('[data-ed]');
      var done = function () { S.editing = false; S.dirty = false; S.draft = null; if (!S.rows.length) rollAll(true); else draw(); };
      if (reduced() || !ed) return done();
      ed.style.height = ed.offsetHeight + 'px'; void ed.offsetHeight; ed.style.height = '0px';
      setTimeout(done, 320);
    }
    function nag() {
      B.classList.remove('is-nagging'); void B.offsetWidth; B.classList.add('is-nagging');
      var w = B.querySelector('[data-warn]');
      w.classList.remove('is-on'); void w.offsetWidth; w.classList.add('is-on');
      var s = B.querySelector('[data-save]');
      if (s) s.focus({ preventScroll: true });
    }
    function choose(id) {
      S.table = id; S.rows = []; S.picking = false;
      if (opts.onAttrs) opts.onAttrs(S.table, S.count);
      rollAll();
    }
    function saveDraft() {
      readDraft();
      var d = S.draft;
      d.name = d.name.trim();
      d.entries = d.entries.filter(function (e) { return e.name.trim(); }).map(function (e) { return { name: e.name.trim(), brief: e.brief.trim(), weight: e.weight }; });
      if (!d.name) { notify('Give the table a name first.', 'error'); return; }
      if (!d.entries.length) { notify('Add at least one line first.', 'error'); return; }
      var next = S.list.filter(function (t) { return t.id !== d.id; }).concat([{ id: d.id, name: d.name, entries: d.entries }]);
      S.saving = true; drawEditor(true);
      save(opts.campaignId, next).then(function (saved) {
        S.saving = false; S.list = saved; S.table = d.id; S.dirty = false;
        if (opts.onAttrs) opts.onAttrs(S.table, S.count);
        notify('Saved. “' + d.name + '” is in Your tables for every page and calendar.');
        S.rows = [];
        closeEditor();
      }).catch(function (e) {
        S.saving = false; drawEditor(true);
        notify(/403|forbidden|DM access/i.test(e.message) ? 'Only the campaign owner or members with DM access can change tables.' : e.message, 'error');
      });
    }

    B.addEventListener('click', function (e) {
      var t = e.target.closest('button');
      if (!t || !B.contains(t)) return;
      if (t.hasAttribute('data-pick')) { S.picking = !S.picking; draw(); return; }
      if (t.hasAttribute('data-t')) { if (S.editing && S.dirty) { S.picking = false; draw(); nag(); return; } S.editing = false; S.draft = null; choose(t.getAttribute('data-t')); return; }
      if (t.hasAttribute('data-new')) {
        S.picking = false; S.editing = true; S.dirty = true;
        S.draft = { id: freeId(S.G, S.list, 'my-table'), name: 'New table', entries: [{ name: '', brief: '', weight: 1 }, { name: '', brief: '', weight: 1 }, { name: '', brief: '', weight: 1 }] };
        draw();
        var f = B.querySelector('[data-tname]'); if (f) f.select();
        return;
      }
      if (t.hasAttribute('data-n')) { S.count = +t.getAttribute('data-n'); S.rows = S.rows.slice(0, S.count); if (opts.onAttrs) opts.onAttrs(S.table, S.count); rollAll(); return; }
      if (t.hasAttribute('data-edit')) {
        if (S.editing) { if (S.dirty) nag(); else closeEditor(); return; }
        S.editing = true;
        var o = own(S.list, S.table);
        S.draft = o ? { id: o.id, name: o.name, entries: entriesFor(S.G, S.list, o.id) } : null;
        draw(); drawEditor(false);
        return;
      }
      if (t.hasAttribute('data-copy')) {
        S.draft = { id: freeId(S.G, S.list, S.table + '-copy'), name: tableName(S.G, S.list, S.table) + ' (copy)', entries: entriesFor(S.G, S.list, S.table) };
        S.dirty = true; draw();
        return;
      }
      if (t.hasAttribute('data-add')) { readDraft(); S.draft.entries.push({ name: '', brief: '', weight: 1 }); S.dirty = true; draw(); var ins = B.querySelectorAll('.rt-ent [data-n]'); ins[ins.length - 1].focus(); return; }
      if (t.hasAttribute('data-del')) { readDraft(); S.draft.entries.splice(+t.closest('.rt-ent').getAttribute('data-e'), 1); S.dirty = true; draw(); return; }
      if (t.hasAttribute('data-save')) { saveDraft(); return; }
      if (t.hasAttribute('data-discard')) { S.dirty = false; closeEditor(); return; }
      if (t.hasAttribute('data-roll')) { rollAll(); return; }
      if (t.hasAttribute('data-pin')) { var i = +t.closest('.rt-row').getAttribute('data-i'); S.rows[i].pin = !S.rows[i].pin; draw(); return; }
      if (t.hasAttribute('data-one')) { rollOne(+t.closest('.rt-row').getAttribute('data-i')); return; }
      if (t.hasAttribute('data-put')) {
        var rows = S.rows.filter(function (r) { return !r.problem; });
        if (rows.length && opts.onPut) opts.onPut(rows.map(function (r) { return { name: r.name, brief: r.brief }; }));
      }
    });
    B.addEventListener('input', function (e) {
      if (e.target.matches('[data-q]')) { drawPicker(e.target.value); return; }
      if (e.target.closest('.rt-ed')) {
        S.dirty = true;
        if (e.target.matches('textarea')) { e.target.style.height = 'auto'; e.target.style.height = (e.target.scrollHeight + 2) + 'px'; }
      }
    });
    B.addEventListener('change', function (e) { if (e.target.closest('.rt-ed')) S.dirty = true; });
    B.addEventListener('keydown', function (e) {
      if (e.key !== 'Escape') return;
      if (S.picking) { e.stopPropagation(); S.picking = false; draw(); B.querySelector('[data-pick]').focus(); }
      else if (S.editing) { e.stopPropagation(); if (S.dirty) nag(); else closeEditor(); }
    });
    /* Click-off: the picker just closes; the editor closes only when nothing is unsaved. */
    function outside(e) {
      if (B.contains(e.target)) return;
      if (S.picking) { S.picking = false; draw(); return; }
      if (S.editing) { if (S.dirty) nag(); else closeEditor(); }
    }
    document.addEventListener('mousedown', outside);

    Promise.all([engine(), mine(opts.campaignId)]).then(function (res) {
      S.G = res[0]; S.list = res[1];
      if (!NAMES[S.table] && !own(S.list, S.table) && !starter(S.G, S.table)) S.table = 'rumours';
      rollAll(true);
    }).catch(function () {
      B.innerHTML = '<div class="rt-loading">The tables couldn’t load. Reload the page to try again.</div>';
    });

    return {
      destroy: function () { document.removeEventListener('mousedown', outside); B.remove(); },
      isDirty: function () { return S.editing && S.dirty; }
    };
  }

  var DAY_CHIPS = [['omens', 'Omens'], ['rumours', 'Rumours'], ['market', 'Market'], ['troubles', 'Troubles'], ['road', 'On the road'], ['sky-sights', 'Sky']];
  var KIND_WORD = { sky: 'Sky', quest: 'Quest', downtime: 'Downtime', social: 'Social' };

  /**
   * The day roller, opened under "Add an event". opts:
   *   campaignId, calendar (the calendar's export shape), date {year, month, day}
   *   why(result)        extra tags for a result, as HTML (the day's moon, season)
   *   onPut(result)      promise; resolves when the event is saved
   *   onClose()          the roller has folded away
   * Returns {close, isDirty, nag}.
   */
  function mountDay(host, opts) {
    var S = { table: 'omens', res: null, G: null, list: [], more: false, busy: false };
    var P = document.createElement('div');
    P.className = 'rtd';
    P.setAttribute('data-roll-day', '');
    P.style.height = '0px';
    host.appendChild(P);

    function chipList() {
      var chips = DAY_CHIPS.slice();
      if (S.more) catalog(S.G, S.list).forEach(function (g) {
        if (g[0] === 'Names') return;
        g[1].forEach(function (t) { if (!chips.some(function (c) { return c[0] === t.id; })) chips.push([t.id, t.name]); });
      });
      return chips;
    }
    function resHTML(r) {
      return r ? '<b>' + esc(r.name) + '</b>' + (r.brief ? '<span>' + esc(r.brief) + '</span>' : '') : '<span class="rt-note">Getting the tables ready…</span>';
    }
    function whyHTML(r) {
      var w = opts.why ? opts.why(r) : '';
      if (r && r.kind && KIND_WORD[r.kind]) w += '<em>' + KIND_WORD[r.kind] + '</em>';
      return w;
    }
    function draw() {
      P.innerHTML = '<div class="rtd-chips" role="group" aria-label="Table">' + chipList().map(function (c) {
        return '<button type="button" data-t="' + esc(c[0]) + '" aria-pressed="' + (c[0] === S.table) + '">' + esc(c[1]) + '</button>';
      }).join('') + (S.more ? '' : '<button type="button" data-more>More…</button>') + '</div>' +
        '<div class="rtd-res" data-box aria-live="polite">' + resHTML(S.res) + '</div><div class="rtd-why">' + (S.res ? whyHTML(S.res) : '') + '</div>' +
        '<div class="rtd-act"><button type="button" class="rt-btn" data-roll title="Roll again" aria-label="Roll again"><i class="fa-solid fa-dice"></i></button>' +
        '<button type="button" class="rt-main" data-put' + (S.res && !S.res.problem && !S.busy ? '' : ' disabled') + '><i class="fa-solid fa-calendar-plus" aria-hidden="true"></i>' + (S.busy ? 'Putting it on the day…' : 'Put on this day') + '</button>' +
        '<button type="button" class="rt-ghost" data-cancel>Cancel</button></div>' +
        '<p class="rt-note rtd-hid"><i class="fa-solid fa-eye-slash" aria-hidden="true"></i> Only you see it until you show it.</p>' +
        '<div class="rt-warn" data-warn role="alert"><i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i><span>This roll isn’t on the day yet.</span><button type="button" data-discard>Discard</button></div>';
    }
    function rollIt() {
      if (!S.G) return;
      var r = roll(S.G, S.list, S.table, { calendar: opts.calendar, date: opts.date });
      S.res = r;
      draw();
      var d = P.querySelector('[data-roll]');
      d.classList.add('is-spinning');
      var w = P.querySelector('.rtd-why');
      w.innerHTML = '';
      reel(P.querySelector('[data-box]'), function () { return roll(S.G, S.list, S.table, { calendar: opts.calendar, date: opts.date }); }, resHTML(r), function () { w.innerHTML = whyHTML(r); });
    }
    function open() {
      draw();
      var h = P.scrollHeight;
      if (reduced()) P.style.height = 'auto';
      else { void P.offsetHeight; P.style.height = h + 'px'; setTimeout(function () { if (P.isConnected) P.style.height = 'auto'; }, 360); }
      Promise.all([engine(), mine(opts.campaignId)]).then(function (res) { S.G = res[0]; S.list = res[1]; rollIt(); })
        .catch(function () { P.querySelector('[data-box]').innerHTML = '<span class="rt-note">The tables couldn’t load. Reload the page to try again.</span>'; });
    }
    var closed = false;
    function close() {
      if (closed) return;
      closed = true;
      var fin = function () { P.remove(); if (opts.onClose) opts.onClose(); };
      if (reduced()) return fin();
      P.style.height = P.offsetHeight + 'px'; void P.offsetHeight; P.style.height = '0px'; P.style.opacity = '0';
      setTimeout(fin, 340);
    }
    function isDirty() { return !closed && !!S.res && !S.res.problem; }
    function nag() {
      var card = P.closest('.wing') || P;
      card.classList.remove('is-nagging'); void card.offsetWidth; card.classList.add('is-nagging');
      var w = P.querySelector('[data-warn]');
      w.classList.remove('is-on'); void w.offsetWidth; w.classList.add('is-on');
    }
    P.addEventListener('click', function (e) {
      var t = e.target.closest('button');
      if (!t) return;
      e.stopPropagation();
      if (t.hasAttribute('data-t')) { S.table = t.getAttribute('data-t'); rollIt(); return; }
      if (t.hasAttribute('data-more')) { S.more = true; draw(); return; }
      if (t.hasAttribute('data-roll')) { rollIt(); return; }
      if (t.hasAttribute('data-cancel') || t.hasAttribute('data-discard')) { S.res = null; close(); return; }
      if (t.hasAttribute('data-put') && S.res && !S.busy) {
        S.busy = true; draw();
        Promise.resolve(opts.onPut(S.res)).then(function () { S.res = null; close(); })
          .catch(function () { S.busy = false; draw(); notify('The roll couldn’t be put on the day. Try again.', 'error'); });
      }
    });
    open();
    return { close: close, isDirty: isDirty, nag: nag };
  }

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.RollTables = {
    engine: engine, mine: mine, save: save, roll: roll, mountBlock: mountBlock, mountDay: mountDay,
    _internal: { catalog: catalog, entriesFor: entriesFor, freeId: freeId, esc: esc, reel: reel, reduced: reduced, notify: notify, own: own, starter: starter, tableName: tableName, NAMES: NAMES, ID_RE: ID_RE, setWith: setWith }
  };
})();
