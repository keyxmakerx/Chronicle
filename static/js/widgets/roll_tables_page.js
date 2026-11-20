/**
 * roll_tables_page.js -- the Rolling tables page (Campaign settings).
 *
 * One list of every table the campaign can roll on (its own, the starters and
 * the library) with Find, and a panel beside it that starts a new table three
 * ways: Blank, Examples (the builder: a few example lines, when they can come
 * up, who sees what a calendar day gets, and test rolls) and Copy one. The
 * campaign's own rows can be renamed, moved and removed from their three dots.
 *
 * It adds no engine and no storage of its own: tables are read and replaced
 * through Chronicle.RollTables (roll_tables.js), and test rolls are the same
 * roll the day roller makes.
 *
 * Mount via:
 *   <div data-widget="roll-tables-page" data-campaign-id="..."></div>
 *
 * Chronicle.RollTablesPage (also used by the tests):
 *   .WHEN                       the "they can come up" choices
 *   .tableFromDraft(draft, id)  a draft as the table document stores it
 *   .problems(draft, tab)       plain-language reasons a draft can't be saved
 *   .whenOf(entries)            which choice a table's lines carry ('' when none matches)
 *   .move(list, id, by)         the list with one table moved up (-1) or down (+1)
 *   .rename(list, id, name)     the list with one table renamed
 *   .remove(list, id)           the list without one table
 */
(function () {
  'use strict';

  var ODDS = [1, 2, 3];
  var SHOW_FIRST = 3;
  // The choices the builder offers. A page has no moon or season of its own,
  // so these only decide things on a calendar day.
  var WHEN = [
    { id: 'any', label: 'Any time', when: null },
    { id: 'full', label: 'Near a full moon', when: { moon: { name: 'any', phase: 'full', within: 1 } } },
    { id: 'new', label: 'Near a new moon', when: { moon: { name: 'any', phase: 'new', within: 1 } } },
    { id: 'spring', label: 'In spring', when: { season: ['spring'] } },
    { id: 'summer', label: 'In summer', when: { season: ['summer'] } },
    { id: 'autumn', label: 'In autumn', when: { season: ['autumn'] } },
    { id: 'winter', label: 'In winter', when: { season: ['winter'] } }
  ];

  function RT() { return window.Chronicle.RollTables; }
  function esc(s) { return RT()._internal.esc(s); }

  function whenFor(id) {
    for (var i = 0; i < WHEN.length; i++) if (WHEN[i].id === id) return WHEN[i].when;
    return null;
  }
  function whenOf(entries) {
    var first = entries && entries[0] && entries[0].when;
    var key = first ? JSON.stringify(first) : '';
    for (var i = 0; i < (entries || []).length; i++) {
      if ((entries[i].when ? JSON.stringify(entries[i].when) : '') !== key) return '';
    }
    if (!key) return 'any';
    for (var j = 0; j < WHEN.length; j++) if (WHEN[j].when && JSON.stringify(WHEN[j].when) === key) return WHEN[j].id;
    return '';
  }
  function whenLabel(entries) {
    var id = whenOf(entries);
    if (id) { for (var i = 0; i < WHEN.length; i++) if (WHEN[i].id === id) return WHEN[i].label; }
    return 'Some days';
  }

  function filled(draft) {
    return draft.lines.filter(function (l) { return l.name.trim(); });
  }
  /* Why a draft can't be saved yet, in the words the page shows. */
  function problems(draft, tab) {
    var out = [];
    if (!draft.name.trim()) out.push('Give the table a name.');
    var n = filled(draft).length;
    if (tab === 'examples' && n < 3) out.push('Write three or more examples.');
    else if (n < 1) out.push('Write at least one line.');
    return out;
  }
  /* A draft as the document stores it. Conditions and who sees a day's roll
     are the Examples tab's; Blank saves plain lines. */
  function tableFromDraft(draft, id, tab) {
    var cond = tab === 'examples' ? whenFor(draft.when) : null;
    var t = {
      id: id, name: draft.name.trim(),
      entries: filled(draft).map(function (l) {
        var e = { name: l.name.trim(), brief: (l.brief || '').trim(), weight: l.weight };
        if (cond) e.when = JSON.parse(JSON.stringify(cond));
        return e;
      })
    };
    if (tab === 'examples') t.visibility = draft.see === 'dm_only' ? 'dm_only' : 'everyone';
    if (draft.credit) t.credit = draft.credit;
    return t;
  }

  function indexOf(list, id) { for (var i = 0; i < list.length; i++) if (list[i].id === id) return i; return -1; }
  function move(list, id, by) {
    var i = indexOf(list, id), j = i + by;
    if (i < 0 || j < 0 || j >= list.length) return list.slice();
    var out = list.slice(), t = out[i];
    out[i] = out[j]; out[j] = t;
    return out;
  }
  function rename(list, id, name) {
    return list.map(function (t) {
      if (t.id !== id) return t;
      var c = {};
      Object.keys(t).forEach(function (k) { c[k] = t[k]; });
      c.name = name;
      return c;
    });
  }
  function remove(list, id) { return list.filter(function (t) { return t.id !== id; }); }

  function lines(n) { return n === 1 ? '1 line' : n + ' lines'; }
  function blankLine(w) { return { name: '', brief: '', weight: w || 2 }; }
  function newDraft() {
    return { name: '', lines: [blankLine(), blankLine(), blankLine()], when: 'any', see: 'everyone', credit: null };
  }

  function mount(host) {
    var campaignId = host.getAttribute('data-campaign-id');
    var S = {
      G: null, list: [], q: '', open: {}, loaded: false, failed: false,
      panel: null,        // null | { mode: 'new', tab, draft, copyId, saving } | { mode: 'view', id }
      menu: null,         // id of the row whose menu is open
      renaming: null, saving: false
    };
    host.classList.add('rtp');
    host.innerHTML = '<p class="rtp-hint">Getting the tables ready…</p>';

    function notify(m, k) { RT()._internal.notify(m, k); }
    function gm() { return window.Chronicle && window.Chronicle.gmTerm ? window.Chronicle.gmTerm() : 'GM'; }

    /* ---- the list ---- */
    function groups() {
      var q = S.q.trim().toLowerCase();
      return RT()._internal.catalog(S.G, S.list).filter(function (g) {
        return g[0] !== 'Names';
      }).map(function (g) {
        var items = g[1].filter(function (t) { return !q || t.name.toLowerCase().indexOf(q) >= 0; });
        return { name: g[0], all: g[1].length, items: items, own: g[0] === 'Your tables' };
      }).filter(function (g) { return g.items.length || (g.own && !q); });
    }
    function rowHTML(t, own, i, n) {
      var sel = S.panel && S.panel.mode === 'view' && S.panel.id === t.id;
      var h = '<div class="rtp-row' + (sel ? ' is-sel' : '') + '" data-row="' + esc(t.id) + '">';
      if (S.renaming === t.id) {
        h += '<input class="rtp-rename" data-rename aria-label="Table name" maxlength="120" value="' + esc(t.name) + '">';
      } else {
        h += '<button type="button" class="rtp-nm" data-view="' + esc(t.id) + '" aria-pressed="' + !!sel + '">' + esc(t.name) + '</button>';
      }
      h += '<span class="rtp-hint">' + lines(t.n) + '</span>';
      if (own) {
        h += '<button type="button" class="rtp-ico" data-menu="' + esc(t.id) + '" aria-haspopup="menu" aria-expanded="' + (S.menu === t.id) + '" aria-label="More for ' + esc(t.name) + '">' +
          '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="5" cy="12" r="1.8"/><circle cx="12" cy="12" r="1.8"/><circle cx="19" cy="12" r="1.8"/></svg></button>';
        if (S.menu === t.id) {
          h += '<div class="rtp-menu" role="menu" aria-label="' + esc(t.name) + '">' +
            '<button type="button" role="menuitem" data-act="rename">Rename</button>' +
            '<button type="button" role="menuitem" data-act="up"' + (i === 0 ? ' disabled' : '') + '>Move up</button>' +
            '<button type="button" role="menuitem" data-act="down"' + (i === n - 1 ? ' disabled' : '') + '>Move down</button>' +
            '<button type="button" role="menuitem" class="is-danger" data-act="remove">Remove</button></div>';
        }
      }
      return h + '</div>';
    }
    function listHTML() {
      var h = '';
      var gs = groups();
      if (!gs.length) return '<p class="rtp-empty">No table has that name. Try fewer letters.</p>';
      gs.forEach(function (g) {
        h += '<div class="rtp-grp" data-grp="' + esc(g.name) + '"><div class="rtp-lab">' + esc(g.name) + ' <span class="rtp-pill">' + g.all + '</span></div>';
        if (g.own && !g.items.length) {
          h += '<p class="rtp-empty">None yet. Start one with New table.</p>';
        }
        var folded = !g.own && !S.q.trim() && !S.open[g.name] && g.items.length > SHOW_FIRST;
        var shown = folded ? g.items.slice(0, SHOW_FIRST) : g.items;
        shown.forEach(function (t, i) { h += rowHTML(t, g.own, i, g.items.length); });
        if (folded) h += '<button type="button" class="rtp-more" data-all="' + esc(g.name) + '">Show all ' + g.items.length + '</button>';
        h += '</div>';
      });
      return h;
    }

    /* ---- the panel ---- */
    function linesHTML(d, tab) {
      var ex = tab === 'examples';
      return d.lines.map(function (l, i) {
        return '<div class="rtp-ex" data-i="' + i + '">' +
          '<input class="rtp-line rtp-n" data-n maxlength="200" aria-label="' + (ex ? 'Example ' : 'Line ') + (i + 1) + '" placeholder="What happens, in a few words" value="' + esc(l.name) + '">' +
          '<button type="button" class="rtp-odds" data-odds aria-label="How often: ' + RT()._internal.weightLabel(l.weight) + '. Change">' + RT()._internal.weightLabel(l.weight) + '</button>' +
          '<button type="button" class="rtp-ico" data-del aria-label="Remove ' + (ex ? 'example ' : 'line ') + (i + 1) + '"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg></button>' +
          '<textarea class="rtp-line rtp-b" data-b rows="1" maxlength="1000" aria-label="More detail for ' + (ex ? 'example ' : 'line ') + (i + 1) + '" placeholder="One line a ' + esc(gm()) + ' can use as it stands">' + esc(l.brief) + '</textarea></div>';
      }).join('');
    }
    function formHTML(p) {
      var d = p.draft, ex = p.tab === 'examples';
      var h = '<label class="rtp-fl"><span>Name</span><input class="rtp-line" data-name maxlength="120" value="' + esc(d.name) + '" placeholder="Dockside omens"></label>' +
        '<div><div class="rtp-wrap"><span class="rtp-lab">' + (ex ? 'A few examples' : 'Lines') + '</span><span class="rtp-hint">' +
        (ex ? 'Three or more. ' : '') + 'Words in <span class="rtp-brace">{braces}</span> roll on another table.</span></div>' +
        '<div data-lines>' + linesHTML(d, p.tab) + '</div>' +
        '<p class="rtp-add"><button type="button" class="rtp-q" data-add>' + (ex ? 'Add an example' : 'Add a line') + '</button></p></div>';
      if (ex) {
        h += '<div class="rtp-two"><label class="rtp-fl"><span>They can come up</span><select class="rtp-select" data-when>' +
          WHEN.map(function (w) { return '<option value="' + w.id + '"' + (w.id === d.when ? ' selected' : '') + '>' + w.label + '</option>'; }).join('') +
          '</select></label>' +
          '<div class="rtp-fl" role="group" aria-labelledby="rtp-see"><span id="rtp-see">A calendar day’s roll is seen by</span><span class="rtp-seg">' +
          '<button type="button" data-see="everyone" aria-pressed="' + (d.see !== 'dm_only') + '">Everyone</button>' +
          '<button type="button" data-see="dm_only" aria-pressed="' + (d.see === 'dm_only') + '">Only you</button></span></div></div>' +
          '<div><div class="rtp-wrap"><span class="rtp-lab">Test rolls</span><span class="rtp-sp"></span><button type="button" class="rtp-q" data-again>Roll again</button></div>' +
          '<div class="rtp-tr" data-tests aria-live="polite"></div></div>';
      }
      return h + '<div class="rtp-foot"><button type="button" class="btn-primary rtp-save" data-save' + (p.saving ? ' disabled data-working' : '') + '>' + (p.saving ? 'Saving…' : 'Save table') + '</button>' +
        '<button type="button" class="rtp-q" data-discard>Discard</button>' +
        '<span class="rtp-hint rtp-err" data-err role="alert"></span></div>';
    }
    function copyHTML(p) {
      var q = (p.copyQ || '').toLowerCase(), n = 0, h = '';
      RT()._internal.catalog(S.G, S.list).forEach(function (g) {
        if (g[0] === 'Your tables' || g[0] === 'Names') return;
        var items = g[1].filter(function (t) { return !q || t.name.toLowerCase().indexOf(q) >= 0; });
        if (!items.length) return;
        h += '<div class="rtp-lab rtp-lab-s">' + esc(g[0]) + '</div>';
        items.forEach(function (t) {
          n++;
          h += '<button type="button" class="rtp-pick" data-copy="' + esc(t.id) + '" aria-pressed="' + (p.copyId === t.id) + '">' + esc(t.name) + '<span class="rtp-hint">' + lines(t.n) + '</span></button>';
        });
      });
      var chosen = p.copyId ? RT()._internal.tableName(S.G, S.list, p.copyId) : '';
      return '<label class="rtp-fl"><span>Find the table to copy</span><input class="rtp-line" type="search" data-copyq placeholder="Find a table" value="' + esc(p.copyQ || '') + '"></label>' +
        '<div class="rtp-picks" data-picks>' + (n ? h : '<p class="rtp-empty">No table has that name.</p>') + '</div>' +
        (p.copyId ? '<label class="rtp-fl"><span>Name of your copy</span><input class="rtp-line" data-name maxlength="120" value="' + esc(p.draft.name || chosen + ' (copy)') + '"></label>' +
          creditNote(p.copyId) : '<p class="rtp-hint">Pick a table to copy. The copy is yours to change; the original stays as it is.</p>') +
        '<div class="rtp-foot"><button type="button" class="btn-primary rtp-save" data-save' + (!p.copyId || p.saving ? ' disabled' : '') + '>' + (p.saving ? 'Saving…' : 'Make a copy') + '</button>' +
        '<button type="button" class="rtp-q" data-discard>Discard</button><span class="rtp-hint rtp-err" data-err role="alert"></span></div>';
    }
    function creditNote(id) {
      var c = RT()._internal.creditOf(S.G, S.list, id);
      var line = c ? RT()._internal.creditLine(c) : '';
      return line ? '<p class="rtp-hint">The copy keeps its credit: ' + line + '</p>' : '';
    }
    function viewHTML(p) {
      var I = RT()._internal, own = I.own(S.list, p.id), es = I.entriesFor(S.G, S.list, p.id);
      var h = '<div class="rtp-ph"><b>' + esc(I.tableName(S.G, S.list, p.id)) + '</b><button type="button" class="rtp-ico" data-close aria-label="Close"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg></button></div><div class="rtp-body">';
      if (own) {
        h += '<p class="rtp-hint">Comes up: ' + esc(whenLabel(es).toLowerCase()) + ' · A calendar day’s roll is seen by ' + (own.visibility === 'everyone' ? 'everyone' : 'only you') + '</p>';
      } else {
        h += '<p class="rtp-hint">A built-in table stays as it is, so updates can improve it. <button type="button" class="rtp-q" data-copy-this="' + esc(p.id) + '">Copy it to your tables</button></p>';
        var cr = I.creditOf(S.G, S.list, p.id);
        if (cr) h += I.creditHTML(cr);
      }
      h += '<div class="rtp-tr">' + es.map(function (e) {
        return '<div><b>' + I.braces(e.name) + '</b>' + (e.brief ? '<span>' + I.braces(e.brief) + '</span>' : '') + '</div>';
      }).join('') + '</div></div>';
      return h;
    }
    function panelHTML() {
      var p = S.panel;
      if (!p) return '';
      if (p.mode === 'view') return viewHTML(p);
      return '<div class="rtp-ph"><b>New table</b><button type="button" class="rtp-ico" data-close aria-label="Close"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg></button></div>' +
        '<div class="rtp-tabs" role="tablist" aria-label="Ways to start a table">' + [['blank', 'Blank'], ['examples', 'Examples'], ['copy', 'Copy one']].map(function (t) {
          return '<button type="button" role="tab" data-tab="' + t[0] + '" aria-selected="' + (p.tab === t[0]) + '">' + t[1] + '</button>';
        }).join('') + '</div><div class="rtp-body" role="tabpanel" data-body>' + (p.tab === 'copy' ? copyHTML(p) : formHTML(p)) + '</div>';
    }

    /* ---- drawing ---- */
    function drawList() { var el = host.querySelector('[data-list]'); if (el) el.innerHTML = listHTML(); }
    function drawPanel(fade) {
      var el = host.querySelector('[data-panel]');
      if (!el) return;
      el.hidden = !S.panel;
      host.classList.toggle('has-panel', !!S.panel);
      el.innerHTML = panelHTML();
      if (fade) { var b = el.querySelector('.rtp-body'); if (b) { b.classList.remove('is-fade'); void b.offsetWidth; b.classList.add('is-fade'); } }
      grow(el);
      if (S.panel && S.panel.mode === 'new' && S.panel.tab === 'examples') drawTests();
    }
    /* A detail line grows to its words; none is ever cut off. */
    function grow(root) {
      [].forEach.call(root.querySelectorAll('textarea'), function (t) { t.style.height = 'auto'; t.style.height = (t.scrollHeight + 1) + 'px'; });
    }
    function frame() {
      host.innerHTML =
        '<div class="rtp-tools"><label class="rtp-find"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="6"/><path d="M20 20l-4.5-4.5"/></svg>' +
        '<input type="search" data-find aria-label="Find a table" placeholder="Find a table" value="' + esc(S.q) + '"></label>' +
        '<button type="button" class="btn-secondary" data-new>New table</button></div>' +
        '<div class="rtp-split"><div class="rtp-list" data-list></div><section class="rtp-panel" data-panel aria-label="Table panel" hidden></section></div>';
      drawList(); drawPanel(false);
    }

    /* ---- test rolls: the real roll, without the day conditions ---- */
    function drawTests() {
      var box = host.querySelector('[data-tests]');
      if (!box || !S.panel || S.panel.mode !== 'new') return;
      var d = S.panel.draft, es = filled(d);
      if (!es.length) { box.innerHTML = '<p class="rtp-hint">Write an example and a test roll appears here.</p>'; return; }
      var t = tableFromDraft(d, 'draft-table', 'blank');
      var seen = {}, out = [], k = 0;
      while (out.length < Math.min(3, es.length) && k++ < 30) {
        var r = RT().roll(S.G, S.list.concat([t]), 'draft-table');
        if (r.problem) { out = [r]; break; }
        if (!seen[r.name]) { seen[r.name] = 1; out.push(r); }
      }
      box.innerHTML = out.map(function (r) {
        return '<div><b>' + esc(r.name) + '</b>' + (r.brief && r.brief !== r.name ? '<span>' + esc(r.brief) + '</span>' : '') + '</div>';
      }).join('');
    }

    /* ---- panel open and close ---- */
    function setDirty(on) {
      if (window.Chronicle && Chronicle.markDirty) { if (on) Chronicle.markDirty('roll-tables-page'); else Chronicle.markClean('roll-tables-page'); }
    }
    function isDirty() {
      var p = S.panel;
      if (!p || p.mode !== 'new') return false;
      return !!(p.draft.name.trim() || filled(p.draft).length || p.copyId);
    }
    function openPanel(p, fade) {
      S.panel = p; S.menu = null;
      drawList(); drawPanel(fade);
      var panel = host.querySelector('[data-panel]');
      if (panel && !RT()._internal.reduced()) { panel.classList.remove('is-in'); void panel.offsetWidth; panel.classList.add('is-in'); }
    }
    function closePanel() {
      var panel = host.querySelector('[data-panel]');
      var done = function () { S.panel = null; setDirty(false); drawList(); drawPanel(false); if (panel) panel.classList.remove('is-out'); };
      if (!panel || RT()._internal.reduced()) return done();
      panel.classList.add('is-out');
      setTimeout(done, 120);
    }
    function askDiscard(then) {
      if (!isDirty()) return then();
      var panel = host.querySelector('[data-panel]');
      if (!window.Chronicle || !Chronicle.askSlip || !panel) return then();
      var n = S.panel.draft.name.trim();
      Chronicle.askSlip(panel, {
        ask: 'Discard this table?',
        detail: (n ? '“' + n + '”' : 'The new table') + ' hasn’t been saved.',
        discard: 'Discard', onDiscard: then
      });
    }
    function startNew(tab) {
      if (S.panel && S.panel.mode === 'new') {
        if (S.panel.tab !== tab) switchTab(tab);
        var f = host.querySelector('[data-name]'); if (f) f.focus();
        return;
      }
      askDiscard(function () {
        openPanel({ mode: 'new', tab: tab || 'blank', draft: newDraft(), copyId: '', copyQ: '' }, false);
        var f = host.querySelector('[data-name]'); if (f) f.focus();
      });
    }
    function switchTab(tab) {
      var p = S.panel; readForm();
      p.tab = tab;
      drawPanel(true);
    }
    function readForm() {
      var p = S.panel;
      if (!p || p.mode !== 'new') return;
      var n = host.querySelector('[data-name]');
      if (n) p.draft.name = n.value;
      if (p.tab === 'copy') return;
      p.draft.lines = [].map.call(host.querySelectorAll('.rtp-ex'), function (r) {
        return { name: r.querySelector('[data-n]').value, brief: r.querySelector('[data-b]').value, weight: p.draft.lines[+r.getAttribute('data-i')].weight };
      });
    }

    /* ---- saving ---- */
    function persist(next, okMsg, after) {
      S.saving = true;
      return RT().save(campaignId, next).then(function (saved) {
        S.saving = false; S.list = saved;
        if (okMsg) notify(okMsg);
        if (after) after(saved);
      }).catch(function (e) {
        S.saving = false;
        notify(/403|forbidden|DM access/i.test(e.message) ? 'Only the campaign owner or members with ' + gm() + ' access can change tables.' : e.message, 'error');
        throw e;
      });
    }
    function saveNew() {
      var p = S.panel, I = RT()._internal;
      readForm();
      var err = host.querySelector('[data-err]');
      var t;
      if (p.tab === 'copy') {
        if (!p.copyId) return;
        var name = (p.draft.name || '').trim() || I.tableName(S.G, S.list, p.copyId) + ' (copy)';
        t = { id: '', name: name, entries: I.entriesFor(S.G, S.list, p.copyId).map(function (e) { return { name: e.name, brief: e.brief, weight: e.weight }; }) };
        var cr = I.creditOf(S.G, S.list, p.copyId);
        if (cr) t.credit = cr;
      } else {
        var bad = problems(p.draft, p.tab);
        if (bad.length) { if (err) err.textContent = bad[0]; var nf = host.querySelector(p.draft.name.trim() ? '[data-n]' : '[data-name]'); if (nf) nf.focus(); return; }
        t = tableFromDraft(p.draft, '', p.tab);
      }
      t.id = I.freeId(S.G, S.list, t.name);
      // The engine checks the set the way the day roller will use it, so a
      // brace that names nothing is caught here, in plain words.
      try {
        var v = S.G.tables.validate(I.setWith(S.G, S.list.concat([t])));
        if (!v.ok) { if (err) err.textContent = v.errors[0].message; return; }
      } catch (e) { /* the server's check is the one that decides */ }
      p.saving = true; drawPanel(false);
      persist(S.list.concat([t]), 'Saved. “' + t.name + '” is in Your tables for every page and calendar.', function () {
        S.panel = null; setDirty(false);
        drawList(); drawPanel(false);
        var row = host.querySelector('[data-row="' + t.id + '"] .rtp-nm'); if (row) row.focus();
      }).catch(function () { p.saving = false; drawPanel(false); });
    }

    /* ---- the row menu ---- */
    function openMenu(id) {
      S.menu = S.menu === id ? null : id;
      drawList();
      if (S.menu) { var first = host.querySelector('.rtp-menu button:not([disabled])'); if (first) first.focus(); }
      else { var b = host.querySelector('[data-menu="' + id + '"]'); if (b) b.focus(); }
    }
    function startRename(id) {
      S.renaming = id; S.menu = null; drawList();
      var f = host.querySelector('[data-rename]'); if (f) { f.focus(); f.select(); }
    }
    function endRename(commit) {
      var id = S.renaming, f = host.querySelector('[data-rename]');
      if (!id) return;
      S.renaming = null;
      var name = f ? f.value.trim() : '';
      var cur = RT()._internal.own(S.list, id);
      if (!commit || !name || (cur && cur.name === name)) { drawList(); return; }
      persist(rename(S.list, id, name), 'Renamed to “' + name + '”.', function () { drawList(); if (S.panel && S.panel.mode === 'view') drawPanel(false); })
        .catch(function () { drawList(); });
    }
    function askRemove(id) {
      var t = RT()._internal.own(S.list, id), row = host.querySelector('[data-row="' + id + '"]');
      S.menu = null; drawList();
      row = host.querySelector('[data-row="' + id + '"]');
      if (!t || !row || !window.Chronicle || !Chronicle.askSlip) return;
      Chronicle.askSlip(row, {
        ask: 'Remove “' + t.name + '”?',
        detail: 'Its ' + lines(t.entries.length) + ' go with it. Pages and days that roll on it will say the table was deleted.',
        discard: 'Remove',
        onDiscard: function () {
          persist(remove(S.list, id), '“' + t.name + '” is removed.', function () {
            if (S.panel && S.panel.mode === 'view' && S.panel.id === id) S.panel = null;
            drawList(); drawPanel(false);
          });
        }
      });
    }

    /* ---- events ---- */
    var testTimer = null;
    host.addEventListener('click', function (e) {
      var t = e.target.closest('button');
      if (!t || !host.contains(t)) return;
      var v;
      if (t.hasAttribute('data-new')) { startNew('blank'); return; }
      if (t.hasAttribute('data-close') || t.hasAttribute('data-discard')) {
        readForm();
        askDiscard(function () { closePanel(); });
        return;
      }
      if (t.hasAttribute('data-tab')) { if (S.panel.tab !== t.getAttribute('data-tab')) switchTab(t.getAttribute('data-tab')); return; }
      if (t.hasAttribute('data-view')) {
        var id = t.getAttribute('data-view');
        var go = function () { openPanel({ mode: 'view', id: id }, false); };
        if (S.panel && S.panel.mode === 'view' && S.panel.id === id) { closePanel(); return; }
        readForm(); askDiscard(go); return;
      }
      if (t.hasAttribute('data-all')) { S.open[t.getAttribute('data-all')] = true; drawList(); return; }
      if (t.hasAttribute('data-menu')) { openMenu(t.getAttribute('data-menu')); return; }
      if (t.hasAttribute('data-act')) {
        var rowId = t.closest('[data-row]').getAttribute('data-row'), act = t.getAttribute('data-act');
        if (act === 'rename') startRename(rowId);
        else if (act === 'remove') askRemove(rowId);
        else { S.menu = null; persist(move(S.list, rowId, act === 'up' ? -1 : 1), '', function () { drawList(); var b = host.querySelector('[data-menu="' + rowId + '"]'); if (b) b.focus(); }); }
        return;
      }
      if (t.hasAttribute('data-copy-this')) {
        var cid = t.getAttribute('data-copy-this');
        openPanel({ mode: 'new', tab: 'copy', draft: { name: '', lines: [], when: 'any', see: 'everyone', credit: null }, copyId: cid, copyQ: '' }, false);
        return;
      }
      if (t.hasAttribute('data-copy')) {
        readForm();
        S.panel.copyId = t.getAttribute('data-copy'); S.panel.draft.name = '';
        drawPanel(false);
        var ni = host.querySelector('[data-name]'); if (ni) ni.focus();
        return;
      }
      if (t.hasAttribute('data-add')) {
        readForm(); S.panel.draft.lines.push(blankLine()); drawPanel(false);
        var ins = host.querySelectorAll('.rtp-ex [data-n]'); ins[ins.length - 1].focus();
        return;
      }
      if (t.hasAttribute('data-del')) {
        readForm(); S.panel.draft.lines.splice(+t.closest('.rtp-ex').getAttribute('data-i'), 1);
        if (!S.panel.draft.lines.length) S.panel.draft.lines.push(blankLine());
        drawPanel(false); return;
      }
      if (t.hasAttribute('data-odds')) {
        readForm();
        var ln = S.panel.draft.lines[+t.closest('.rtp-ex').getAttribute('data-i')];
        ln.weight = ODDS[(ODDS.indexOf(ln.weight) + 1) % ODDS.length];
        t.textContent = RT()._internal.weightLabel(ln.weight);
        t.setAttribute('aria-label', 'How often: ' + t.textContent + '. Change');
        drawTests();
        return;
      }
      if (t.hasAttribute('data-see')) {
        readForm(); S.panel.draft.see = t.getAttribute('data-see');
        [].forEach.call(host.querySelectorAll('[data-see]'), function (b) { b.setAttribute('aria-pressed', String(b === t)); });
        return;
      }
      if (t.hasAttribute('data-again')) { drawTests(); return; }
      if (t.hasAttribute('data-save')) { saveNew(); return; }
    });
    host.addEventListener('input', function (e) {
      var el = e.target, p = S.panel;
      if (el.matches('[data-find]')) { S.q = el.value; drawList(); return; }
      if (el.matches('[data-copyq]')) {
        p.copyQ = el.value;
        var picks = host.querySelector('[data-picks]'), keep = p.copyId;
        var tmp = document.createElement('div'); tmp.innerHTML = copyHTML(p);
        picks.innerHTML = tmp.querySelector('[data-picks]').innerHTML; p.copyId = keep;
        return;
      }
      if (p && p.mode === 'new') {
        setDirty(true);
        if (el.matches('textarea')) grow(el.parentNode);
        if (el.closest('.rtp-ex') || el.matches('[data-name]')) {
          clearTimeout(testTimer);
          testTimer = setTimeout(function () { readForm(); drawTests(); }, 150);
        }
      }
    });
    host.addEventListener('change', function (e) {
      if (e.target.matches('[data-when]')) { S.panel.draft.when = e.target.value; setDirty(true); }
    });
    host.addEventListener('focusout', function (e) {
      if (e.target.matches('[data-rename]')) endRename(true);
    });
    host.addEventListener('keydown', function (e) {
      if (e.target.matches('[data-rename]')) {
        if (e.key === 'Enter') { e.preventDefault(); e.target.blur(); }
        else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); S.renaming = null; e.target.removeEventListener('blur', null); drawList(); }
        return;
      }
      if (S.menu && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
        var items = [].slice.call(host.querySelectorAll('.rtp-menu button:not([disabled])')), at = items.indexOf(document.activeElement);
        if (items.length) { e.preventDefault(); items[(at + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length].focus(); }
        return;
      }
      if (e.key !== 'Escape') return;
      if (S.menu) { e.stopPropagation(); openMenu(S.menu); return; }
      if (S.panel) { e.stopPropagation(); readForm(); askDiscard(closePanel); }
    });
    window.addEventListener('resize', function () { grow(host); });
    document.addEventListener('mousedown', function (e) {
      if (S.menu && !e.target.closest('.rtp-menu') && !e.target.closest('[data-menu]')) { S.menu = null; drawList(); }
    });

    Promise.all([RT().engine(), RT().mine(campaignId), RT().library()]).then(function (res) {
      S.G = res[0]; S.list = res[1]; S.loaded = true;
      frame();
    }).catch(function () {
      S.failed = true;
      host.innerHTML = '<p class="rtp-hint">The tables couldn’t load. Reload the page to try again.</p>';
    });
  }

  var API = { WHEN: WHEN, tableFromDraft: tableFromDraft, problems: problems, whenOf: whenOf, move: move, rename: rename, remove: remove };
  window.Chronicle = window.Chronicle || {};
  window.Chronicle.RollTablesPage = API;
  if (window.Chronicle.register) {
    window.Chronicle.register('roll-tables-page', { init: function (el) { mount(el); }, destroy: function (el) { el.innerHTML = ''; } });
  }
})();
