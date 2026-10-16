/**
 * page_blocks.js: the Read-aloud, Table and Steps page blocks.
 *
 * One widget draws all three; data-kind says which. It fetches the block's
 * content from data-endpoint, so what it shows is whatever the server decided
 * this viewer may see: a DM-only table column never arrives here. Every piece
 * of user text goes in with textContent, never as markup. The pure helpers at
 * the top are exposed as window.PageBlocksHelpers for the node tests.
 *
 * Styles live in static/css/page_blocks.css, scoped under .pb.
 */
(function () {
  'use strict';

  // The server's limits, mirrored so the editor can stop short of a refusal.
  var LIM = {
    readAloud: 4000, columns: 8, rows: 60, label: 80, cell: 500,
    steps: 30, stepTitle: 120, stepText: 1000
  };
  var KINDS = { read_aloud: 'Read-aloud', page_table: 'Table', page_steps: 'Steps' };

  // ---------- Pure helpers ----------
  function uid(prefix) {
    return prefix + '_' + Math.random().toString(36).slice(2, 10);
  }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }

  function emptyData(kind) {
    if (kind === 'read_aloud') return { text: '' };
    if (kind === 'page_table') return { columns: [], rows: [] };
    if (kind === 'page_steps') return { steps: [] };
    return {};
  }

  // normalise fills what a short or older document lacks so rendering never
  // meets an undefined list.
  function normalise(kind, d) {
    d = d && typeof d === 'object' ? d : {};
    if (kind === 'read_aloud') return { text: typeof d.text === 'string' ? d.text : '' };
    if (kind === 'page_table') {
      var cols = (d.columns || []).map(function (c) {
        return { id: c.id, label: c.label || '', dmOnly: !!c.dmOnly };
      });
      var rows = (d.rows || []).map(function (r) { return { id: r.id, cells: r.cells || {} }; });
      return { columns: cols, rows: rows };
    }
    if (kind === 'page_steps') {
      return {
        steps: (d.steps || []).map(function (s) {
          return { id: s.id, title: s.title || '', text: s.text || '', done: !!s.done };
        })
      };
    }
    return d;
  }

  // isEmpty is what decides whether a non-editor sees anything at all.
  function isEmpty(kind, d) {
    if (kind === 'read_aloud') return !String(d.text || '').trim();
    if (kind === 'page_table') return !d.columns.length;
    if (kind === 'page_steps') return !d.steps.length;
    return true;
  }

  // move returns a copy of list with item i shifted by dir (-1 up, +1 down);
  // an edge stays put.
  function move(list, i, dir) {
    var j = i + dir, out = list.slice();
    if (i < 0 || i >= out.length || j < 0 || j >= out.length) return out;
    var t = out[i]; out[i] = out[j]; out[j] = t;
    return out;
  }

  function addColumn(t) {
    var n = clone(t);
    if (n.columns.length >= LIM.columns) return n;
    n.columns.push({ id: uid('c'), label: '', dmOnly: false });
    return n;
  }
  function removeColumn(t, id) {
    var n = clone(t);
    n.columns = n.columns.filter(function (c) { return c.id !== id; });
    n.rows.forEach(function (r) { delete r.cells[id]; });
    return n;
  }
  function addRow(t) {
    var n = clone(t);
    if (n.rows.length >= LIM.rows) return n;
    n.rows.push({ id: uid('r'), cells: {} });
    return n;
  }
  function addStep(d) {
    var n = clone(d);
    if (n.steps.length >= LIM.steps) return n;
    n.steps.push({ id: uid('s'), title: '', text: '', done: false });
    return n;
  }
  function setDone(d, id, done) {
    var n = clone(d);
    n.steps.forEach(function (s) { if (s.id === id) s.done = !!done; });
    return n;
  }
  function clearTicks(d) {
    var n = clone(d);
    n.steps.forEach(function (s) { s.done = false; });
    return n;
  }
  function anyDone(d) {
    return d.steps.some(function (s) { return s.done; });
  }

  var H = {
    LIM: LIM, uid: uid, emptyData: emptyData, normalise: normalise, isEmpty: isEmpty,
    move: move, addColumn: addColumn, removeColumn: removeColumn, addRow: addRow,
    addStep: addStep, setDone: setDone, clearTicks: clearTicks, anyDone: anyDone
  };
  if (typeof window !== 'undefined') window.PageBlocksHelpers = H;
  if (typeof Chronicle === 'undefined' || !Chronicle.register) return;

  // ---------- DOM ----------
  // h builds an element. text goes in as textContent; on is a map of event
  // handlers attached to this element directly, so nothing relies on a
  // listener outside the fragment that an HTMX swap could drop.
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
  function btn(label, cls, onclick, attrs) {
    return h('button', 'pb-btn ' + (cls || ''), label, Object.assign({ type: 'button' }, attrs || {}), { click: onclick });
  }
  // iconBtn is a small square button carrying a glyph, named for screen
  // readers by aria-label.
  function iconBtn(glyph, label, onclick, disabled) {
    return h('button', 'pb-icon', glyph, { type: 'button', 'aria-label': label, title: label, disabled: !!disabled }, { click: onclick });
  }

  function api(S, method, body) {
    var url = S.cfg.endpoint + (method === 'GET' ? '?kind=' + encodeURIComponent(S.cfg.kind) : '');
    return Chronicle.apiFetch(url, { method: method, body: body, csrfToken: S.cfg.csrfToken }).then(function (res) {
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

  var uidCount = 0;

  Chronicle.register('page_blocks', {
    init: function (el, cfg) {
      var S = {
        el: el, cfg: cfg, kind: cfg.kind, id: 'pb' + (++uidCount),
        view: null, data: null, draft: null, editing: false, status: '', statusErr: false,
        saving: Promise.resolve(), focus: null
      };
      el.classList.add('pb');
      el._pb = S;
      load(S);
    },
    destroy: function (el) {
      var S = el._pb; if (!S) return;
      if (S.editing && Chronicle.markClean) Chronicle.markClean(S.id);
      el._pb = null;
    }
  });

  function load(S) {
    api(S, 'GET').then(function (v) {
      S.view = v;
      S.kind = v.kind || S.cfg.kind;
      S.data = normalise(S.kind, v.data);
      draw(S);
    }).catch(function () {
      // A block that cannot load is left out; the rest of the page is fine.
      S.el.textContent = '';
    });
  }

  function canEdit(S) { return !!(S.view && S.view.canEdit); }

  function setStatus(S, msg, isErr) {
    S.status = msg; S.statusErr = !!isErr;
    var line = S.el.querySelector('.pb-status');
    if (line) { line.textContent = msg; line.classList.toggle('err', !!isErr); }
  }

  // ---------- Drawing ----------
  function draw(S) {
    var el = S.el;
    el.textContent = '';
    if (S.editing) { el.appendChild(editor(S)); focusAfter(S); return; }
    if (isEmpty(S.kind, S.data)) {
      if (!canEdit(S)) return; // nothing to read, nothing to edit: draw nothing
      el.appendChild(emptyPrompt(S));
      return;
    }
    var wrap = h('div', 'pb-wrap');
    wrap.appendChild(readView(S));
    if (canEdit(S)) wrap.appendChild(readBar(S));
    el.appendChild(wrap);
  }

  function focusAfter(S) {
    if (!S.focus) return;
    var t = S.el.querySelector(S.focus);
    S.focus = null;
    if (t) t.focus();
  }

  var PROMPTS = {
    read_aloud: 'Add the text you read to the table',
    page_table: 'Add a table of anything you want to look up at the table',
    page_steps: 'Add the beats of this scene, in order'
  };

  function emptyPrompt(S) {
    var box = h('div', 'pb-empty');
    append(box, [
      h('span', 'pb-empty-text', PROMPTS[S.kind] || 'Add content'),
      btn('Edit ' + (KINDS[S.kind] || 'block').toLowerCase(), 'pb-primary', function () { startEdit(S); })
    ]);
    return box;
  }

  function readBar(S) {
    var bar = h('div', 'pb-bar');
    if (S.kind === 'page_steps' && anyDone(S.data)) {
      bar.appendChild(btn('Clear ticks', '', function () { tickAll(S); }));
    }
    bar.appendChild(btn('Edit', '', function () { startEdit(S); }, { 'aria-label': 'Edit ' + (KINDS[S.kind] || 'block').toLowerCase() }));
    bar.appendChild(h('span', 'pb-status', S.status, { role: 'status', 'aria-live': 'polite' }));
    return bar;
  }

  function readView(S) {
    if (S.kind === 'read_aloud') return h('blockquote', 'pb-read', S.data.text);
    if (S.kind === 'page_table') return tableView(S.data);
    return stepsView(S);
  }

  function tableView(d) {
    var scroll = h('div', 'pb-scroll');
    var table = h('table', 'pb-table');
    var head = h('tr');
    d.columns.forEach(function (c) {
      var th = h('th', '', c.label, { scope: 'col' });
      if (c.dmOnly) th.appendChild(h('span', 'pb-dm cb-dm', 'DM'));
      head.appendChild(th);
    });
    table.appendChild(h('thead', '', '', null)).appendChild(head);
    var body = h('tbody');
    d.rows.forEach(function (r) {
      var tr = h('tr');
      d.columns.forEach(function (c) { tr.appendChild(h('td', '', r.cells[c.id] || '')); });
      body.appendChild(tr);
    });
    table.appendChild(body);
    scroll.appendChild(table);
    return scroll;
  }

  function stepsView(S) {
    var ol = h('ol', 'pb-steps');
    var editable = canEdit(S);
    S.data.steps.forEach(function (s, i) {
      var li = h('li', 'pb-step' + (s.done ? ' done' : ''));
      var n = String(i + 1);
      var num = editable
        ? h('button', 'pb-num', n, { type: 'button', 'aria-pressed': s.done ? 'true' : 'false',
            'aria-label': 'Step ' + n + (s.done ? ', done. Mark not done' : '. Mark done') }, { click: function () { tick(S, s.id, !s.done); } })
        : h('span', 'pb-num', n, s.done ? { 'aria-label': 'Step ' + n + ', done' } : null);
      var body = h('div', 'pb-step-body');
      append(body, [
        s.title ? h('div', 'pb-step-title', s.title) : null,
        s.text ? h('div', 'pb-step-text', s.text) : null
      ]);
      append(li, [num, body]);
      ol.appendChild(li);
    });
    return ol;
  }

  // ---------- Saving ----------
  // Saves go one at a time so each carries the version the last one returned;
  // ticking several steps quickly cannot trip the server's conflict check.
  function save(S, data) {
    var run = function () {
      return api(S, 'PUT', { kind: S.kind, data: data, version: S.view.version }).then(function (v) {
        S.view = v;
        S.data = normalise(S.kind, v.data);
        return v;
      });
    };
    S.saving = S.saving.then(run, run);
    return S.saving;
  }

  function reloadAfterConflict(S, err) {
    if (err.status === 409) {
      setStatus(S, 'Someone else changed this. Showing their version.', true);
      return api(S, 'GET').then(function (v) {
        S.view = v; S.data = normalise(S.kind, v.data); S.editing = false; draw(S);
        setStatus(S, 'Someone else changed this. Showing their version.', true);
      }).catch(function () {});
    }
    setStatus(S, err.message, true);
    return null;
  }

  function tick(S, id, done) {
    var prev = S.data;
    S.data = setDone(S.data, id, done); // show the tick at once
    draw(S);
    save(S, S.data).then(function () {
      setStatus(S, ''); draw(S);
    }).catch(function (err) {
      S.data = prev; draw(S);
      reloadAfterConflict(S, err);
    });
  }

  function tickAll(S) {
    var prev = S.data;
    S.data = clearTicks(S.data); draw(S);
    save(S, S.data).then(function () { setStatus(S, ''); draw(S); }).catch(function (err) {
      S.data = prev; draw(S);
      reloadAfterConflict(S, err);
    });
  }

  // ---------- Editing ----------
  function startEdit(S) {
    S.draft = clone(S.data);
    S.editing = true;
    setStatusQuiet(S, '');
    if (Chronicle.markDirty) Chronicle.markDirty(S.id);
    S.focus = '.pb-ed [data-first]';
    draw(S);
  }
  function setStatusQuiet(S, msg) { S.status = msg; S.statusErr = false; }

  function stopEdit(S) {
    S.editing = false; S.draft = null;
    if (Chronicle.markClean) Chronicle.markClean(S.id);
  }

  function commit(S, saveBtn) {
    saveBtn.disabled = true; saveBtn.textContent = 'Saving…';
    save(S, S.draft).then(function () {
      stopEdit(S); setStatusQuiet(S, 'Saved'); draw(S);
      setTimeout(function () {
        if (S.el._pb && S.status === 'Saved') { setStatus(S, ''); }
      }, 2000);
    }).catch(function (err) {
      saveBtn.disabled = false; saveBtn.textContent = 'Save';
      reloadAfterConflict(S, err);
    });
  }

  function editor(S) {
    var ed = h('div', 'pb-ed');
    var body = S.kind === 'read_aloud' ? textEditor(S) : S.kind === 'page_table' ? tableEditor(S) : stepsEditor(S);
    var foot = h('div', 'pb-bar');
    var saveBtn = btn('Save', 'pb-primary', function () { commit(S, saveBtn); });
    append(foot, [
      saveBtn,
      btn('Cancel', '', function () { stopEdit(S); draw(S); }),
      h('span', 'pb-status' + (S.statusErr ? ' err' : ''), S.status, { role: 'status', 'aria-live': 'polite' })
    ]);
    append(ed, [body, foot]);
    return ed;
  }

  function redraw(S, focusSel) { S.focus = focusSel || null; draw(S); }

  function textEditor(S) {
    var wrap = h('div', 'pb-field');
    var ta = h('textarea', 'pb-input pb-area', '', { rows: '6', maxlength: String(LIM.readAloud),
      'aria-label': 'Read-aloud text', placeholder: 'Text you read aloud to the table', 'data-first': true }, {
      input: function () { S.draft.text = ta.value; count.textContent = ta.value.length + ' / ' + LIM.readAloud; }
    });
    ta.value = S.draft.text;
    var count = h('div', 'pb-count', S.draft.text.length + ' / ' + LIM.readAloud);
    return append(wrap, [ta, count]);
  }

  function tableEditor(S) {
    var t = S.draft;
    var wrap = h('div', 'pb-field');
    var scroll = h('div', 'pb-scroll');
    var table = h('table', 'pb-table pb-edit-table');
    var head = h('tr');
    t.columns.forEach(function (c, i) {
      var th = h('th', '', '', { scope: 'col' });
      var label = h('input', 'pb-input', '', { type: 'text', maxlength: String(LIM.label), 'aria-label': 'Column ' + (i + 1) + ' name',
        placeholder: 'Column name', 'data-first': i === 0 }, { input: function () { c.label = label.value; } });
      label.value = c.label;
      var dm = h('label', 'pb-check');
      var cb = h('input', '', '', { type: 'checkbox' }, { change: function () { c.dmOnly = cb.checked; } });
      cb.checked = c.dmOnly;
      append(dm, [cb, h('span', '', 'DM only')]);
      var tools = h('div', 'pb-tools');
      append(tools, [
        iconBtn('←', 'Move column ' + (i + 1) + ' left', function () { t.columns = move(t.columns, i, -1); redraw(S, '.pb-ed [data-col="' + c.id + '"]'); }, i === 0),
        iconBtn('→', 'Move column ' + (i + 1) + ' right', function () { t.columns = move(t.columns, i, 1); redraw(S, '.pb-ed [data-col="' + c.id + '"]'); }, i === t.columns.length - 1),
        iconBtn('×', 'Remove column ' + (i + 1), function () { S.draft = removeColumn(t, c.id); redraw(S); }, t.columns.length <= 1)
      ]);
      th.setAttribute('data-col', c.id);
      append(th, [label, dm, tools]);
      head.appendChild(th);
    });
    head.appendChild(h('th', 'pb-rowhead'));
    table.appendChild(h('thead')).appendChild(head);
    var body = h('tbody');
    t.rows.forEach(function (r, ri) {
      var tr = h('tr');
      t.columns.forEach(function (c, ci) {
        var td = h('td');
        var cell = h('textarea', 'pb-input pb-cell', '', { rows: '1', maxlength: String(LIM.cell),
          'aria-label': (c.label || 'Column ' + (ci + 1)) + ', row ' + (ri + 1) }, {
          input: function () { r.cells[c.id] = cell.value; }
        });
        cell.value = r.cells[c.id] || '';
        td.appendChild(cell);
        tr.appendChild(td);
      });
      var tools = h('td', 'pb-rowtools');
      tr.setAttribute('data-row', r.id);
      append(tools, [
        iconBtn('↑', 'Move row ' + (ri + 1) + ' up', function () { t.rows = move(t.rows, ri, -1); redraw(S, '.pb-ed [data-row="' + r.id + '"] textarea'); }, ri === 0),
        iconBtn('↓', 'Move row ' + (ri + 1) + ' down', function () { t.rows = move(t.rows, ri, 1); redraw(S, '.pb-ed [data-row="' + r.id + '"] textarea'); }, ri === t.rows.length - 1),
        iconBtn('×', 'Remove row ' + (ri + 1), function () { t.rows = t.rows.filter(function (x) { return x.id !== r.id; }); redraw(S); })
      ]);
      tr.appendChild(tools);
      body.appendChild(tr);
    });
    table.appendChild(body);
    scroll.appendChild(table);
    var add = h('div', 'pb-add');
    append(add, [
      btn('Add column', '', function () { S.draft = addColumn(t); redraw(S, '.pb-ed th:last-of-type input'); }, { disabled: t.columns.length >= LIM.columns }),
      btn('Add row', '', function () {
        if (!t.columns.length) S.draft = addColumn(t);
        S.draft = addRow(S.draft);
        redraw(S, '.pb-ed tbody tr:last-child textarea');
      }, { disabled: t.rows.length >= LIM.rows }),
      h('span', 'pb-count', t.columns.length + ' of ' + LIM.columns + ' columns, ' + t.rows.length + ' of ' + LIM.rows + ' rows')
    ]);
    if (!t.columns.length) {
      // A new table starts with one column so the first thing on screen is a
      // place to type.
      S.draft = addColumn(t);
      return tableEditor(S);
    }
    return append(wrap, [scroll, add]);
  }

  function stepsEditor(S) {
    var d = S.draft;
    var wrap = h('div', 'pb-field');
    var list = h('ol', 'pb-steps pb-edit-steps');
    d.steps.forEach(function (s, i) {
      var li = h('li', 'pb-step');
      var title = h('input', 'pb-input', '', { type: 'text', maxlength: String(LIM.stepTitle), 'aria-label': 'Step ' + (i + 1) + ' title',
        placeholder: 'Step title', 'data-first': i === 0 }, { input: function () { s.title = title.value; } });
      title.value = s.title;
      var text = h('textarea', 'pb-input pb-area', '', { rows: '2', maxlength: String(LIM.stepText), 'aria-label': 'Step ' + (i + 1) + ' text',
        placeholder: 'What happens' }, { input: function () { s.text = text.value; } });
      text.value = s.text;
      var tools = h('div', 'pb-tools');
      append(tools, [
        iconBtn('↑', 'Move step ' + (i + 1) + ' up', function () { d.steps = move(d.steps, i, -1); redraw(S, '.pb-ed [data-step="' + s.id + '"] input'); }, i === 0),
        iconBtn('↓', 'Move step ' + (i + 1) + ' down', function () { d.steps = move(d.steps, i, 1); redraw(S, '.pb-ed [data-step="' + s.id + '"] input'); }, i === d.steps.length - 1),
        iconBtn('×', 'Remove step ' + (i + 1), function () { d.steps = d.steps.filter(function (x) { return x.id !== s.id; }); redraw(S); })
      ]);
      li.setAttribute('data-step', s.id);
      var num = h('span', 'pb-num', String(i + 1));
      var fields = h('div', 'pb-step-body');
      append(fields, [title, text]);
      append(li, [num, fields, tools]);
      list.appendChild(li);
    });
    var add = h('div', 'pb-add');
    append(add, [
      btn('Add step', '', function () { S.draft = addStep(d); redraw(S, '.pb-ed li:last-child input'); }, { disabled: d.steps.length >= LIM.steps }),
      h('span', 'pb-count', d.steps.length + ' of ' + LIM.steps + ' steps')
    ]);
    return append(wrap, [list, add]);
  }
})();
