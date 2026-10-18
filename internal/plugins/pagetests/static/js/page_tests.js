/**
 * page_tests.js: the Tests block.
 *
 * A test is a long effort the party makes (a montage, a chase) and the
 * result it ended in. The DM sees every test on the page: its results, the
 * tally of successes and failures, and the result recorded. Pressing a
 * result records it with the calendar's date; when a total fills, the
 * result it offers says so, and the DM confirms it by pressing it. Anyone
 * else sees only the tests marked shown, and only their name and recorded
 * result (the server sends nothing more). Every piece of user text goes in
 * with textContent, never as markup.
 *
 * The block marks the shared block parts (cb-head, cb-count, cb-section,
 * cb-dm, data-dm-only), so the page look and the style kit draw it;
 * page_tests.css sets out only the layout.
 *
 * A game system can offer its own starting results for a new test: the
 * block fires `chronicle:test-results` on document with
 * {campaignId, reply(setup)}, where setup is {kind, needed, limit,
 * results: [{label, on}]}. Without a reply a new test starts with three
 * plain results the DM renames.
 *
 * The pure helpers at the top are exposed as window.PageTestsHelpers for the
 * node tests.
 */
(function () {
  'use strict';

  // The server's limits, mirrored so the editor can stop short of a refusal.
  var LIM = { name: 120, kind: 40, label: 60, results: 8, total: 30, perPage: 20 };
  var ONS = [
    { v: '', label: 'Only by hand' },
    { v: 'successes', label: 'When the successes fill' },
    { v: 'failures', label: 'When the failures fill' }
  ];

  // ---------- Pure helpers ----------
  function rid() { return 'r_' + Math.random().toString(36).slice(2, 10); }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }

  function intIn(v, lo, hi, dflt) {
    v = parseInt(v, 10);
    return isNaN(v) ? dflt : Math.max(lo, Math.min(hi, v));
  }

  function str(v, max) { return typeof v === 'string' ? v.slice(0, max) : ''; }

  // normalise fills what a short or a player's answer lacks, so drawing never
  // meets an undefined list.
  function normalise(t) {
    t = t && typeof t === 'object' ? t : {};
    var rec = t.recorded && typeof t.recorded === 'object' ? {
      result: str(t.recorded.result, 64), label: str(t.recorded.label, LIM.label),
      dateLabel: str(t.recorded.dateLabel, 120)
    } : null;
    return {
      id: str(t.id, 64), name: str(t.name, LIM.name), kind: str(t.kind, LIM.kind), shown: !!t.shown,
      needed: intIn(t.needed, 0, LIM.total, 0), limit: intIn(t.limit, 0, LIM.total, 0),
      successes: intIn(t.successes, 0, LIM.total, 0), failures: intIn(t.failures, 0, LIM.total, 0),
      results: (Array.isArray(t.results) ? t.results : []).map(function (r) {
        return { id: str(r && r.id, 64) || rid(), label: str(r && r.label, LIM.label), on: r && (r.on === 'successes' || r.on === 'failures') ? r.on : '' };
      }),
      recorded: rec, offer: str(t.offer, 64), version: t.version || 0
    };
  }

  // starter is a new test's setup: the game system's when it answered,
  // else three plain results. Results the system marks for a total keep
  // that only when the total is set, as the server requires.
  function starter(sys) {
    var s = sys && typeof sys === 'object' ? sys : null;
    var needed = s ? intIn(s.needed, 0, LIM.total, 0) : 0;
    var limit = s ? intIn(s.limit, 0, LIM.total, 0) : 0;
    var list = s && Array.isArray(s.results) && s.results.length ? s.results : [
      { label: 'Success' }, { label: 'Partial success' }, { label: 'Failure' }
    ];
    var used = {};
    var results = list.slice(0, LIM.results).map(function (r) {
      var on = r && (r.on === 'successes' || r.on === 'failures') ? r.on : '';
      if ((on === 'successes' && !needed) || (on === 'failures' && !limit) || used[on]) on = '';
      if (on) used[on] = true;
      return { id: rid(), label: str(r && r.label, LIM.label).trim() || 'Result', on: on };
    }).filter(function (r) { return r.label; });
    return { name: 'New test', kind: s ? str(s.kind, LIM.kind) : '', shown: false, needed: needed, limit: limit, results: results };
  }

  // totals is the line under a test's name: what ends it.
  function totals(t) {
    var s = function (n) { return n + (n === 1 ? ' success' : ' successes'); };
    var f = function (n) { return n + (n === 1 ? ' failure' : ' failures'); };
    if (t.needed && t.limit) return s(t.needed) + ' before ' + f(t.limit);
    if (t.needed) return s(t.needed) + ' to finish';
    if (t.limit) return 'Ends at ' + f(t.limit);
    return '';
  }

  // offerNote says why a result is on offer.
  function offerNote(t, r) {
    if (t.offer !== r.id) return '';
    return r.on === 'failures' ? 'The failures are full. Press to record it.' : 'The successes are full. Press to record it.';
  }

  // toSave is the PUT body: the setup only. The tally and record have their
  // own calls, so saving the setup never overwrites a count made in play.
  // On a total the test no longer has is cleared, as the server requires.
  function toSave(t) {
    var used = {};
    return {
      name: t.name.trim() || 'Untitled test',
      kind: t.kind.trim(),
      shown: t.shown,
      needed: t.needed,
      limit: t.limit,
      results: t.results.filter(function (r) { return r.label.trim(); }).map(function (r) {
        var on = r.on;
        if ((on === 'successes' && !t.needed) || (on === 'failures' && !t.limit) || used[on]) on = '';
        if (on) used[on] = true;
        return { id: r.id, label: r.label.trim(), on: on };
      }),
      version: t.version
    };
  }

  // setOn gives a total to one result, taking it from any other, since a
  // filled total offers exactly one result.
  function setOn(t, id, on) {
    var n = clone(t);
    n.results.forEach(function (r) {
      if (r.id === id) r.on = on;
      else if (on && r.on === on) r.on = '';
    });
    return n;
  }

  window.PageTestsHelpers = {
    LIM: LIM, normalise: normalise, starter: starter, totals: totals, offerNote: offerNote,
    toSave: toSave, setOn: setOn
  };

  if (typeof Chronicle === 'undefined' || typeof document === 'undefined') return;

  // ---------- DOM ----------
  // h builds an element. text goes in as textContent; on is a map of event
  // handlers attached to this element directly.
  function h(tag, cls, text, attrs, on) {
    var el = document.createElement(tag);
    if (cls) el.className = cls;
    if (text != null && text !== '') el.textContent = text;
    if (attrs) Object.keys(attrs).forEach(function (k) {
      if (attrs[k] === false || attrs[k] == null) return;
      el.setAttribute(k, attrs[k] === true ? '' : attrs[k]);
    });
    if (on) Object.keys(on).forEach(function (k) { el.addEventListener(k, on[k]); });
    return el;
  }
  function append(parent, kids) {
    kids.forEach(function (k) { if (k) parent.appendChild(k); });
    return parent;
  }
  // quiet is the lightest control: a word with no border.
  function quiet(label, onclick, attrs) {
    return h('button', 'pt-quiet', label, Object.assign({ type: 'button' }, attrs || {}), { click: onclick });
  }
  // icon is a monochrome control framed only on hover.
  function icon(fa, label, onclick, attrs) {
    var b = h('button', 'pt-icon', '', Object.assign({ type: 'button', 'aria-label': label, title: label }, attrs || {}), { click: onclick });
    b.appendChild(h('i', 'fa-solid ' + fa, '', { 'aria-hidden': 'true' }));
    return b;
  }

  function api(S, method, url, body) {
    return Chronicle.apiFetch(url, { method: method, body: body, csrfToken: S.cfg.csrfToken }).then(function (res) {
      return res.text().then(function (txt) {
        var data = null;
        try { data = txt ? JSON.parse(txt) : null; } catch (err) { data = null; }
        if (!res.ok) {
          var e = new Error((data && (data.message || data.error)) || 'Something went wrong. Try again.');
          e.status = res.status;
          throw e;
        }
        return data;
      });
    });
  }
  function base(S) { return '/campaigns/' + encodeURIComponent(S.cfg.campaignId); }
  function testURL(S, id, tail) { return base(S) + '/tests/' + encodeURIComponent(id) + (tail || ''); }

  // ---------- The block ----------
  Chronicle.register('page_tests', {
    init: function (el, cfg) {
      var S = { el: el, cfg: cfg, list: [], canEdit: false, editing: null, draft: null, busy: {}, msg: {}, loaded: false };
      el.classList.add('pt');
      el._pt = S;
      load(S);
    },
    destroy: function (el) { el._pt = null; }
  });

  // load fetches the page's tests. A first load that fails leaves the block
  // out; a reload that fails keeps what is on screen.
  function load(S) {
    return api(S, 'GET', base(S) + '/pages/' + encodeURIComponent(S.cfg.entityId) + '/tests').then(function (v) {
      S.list = ((v && v.tests) || []).map(normalise);
      S.canEdit = !!(v && v.canEdit);
      S.loaded = true;
      draw(S);
    }).catch(function () {
      if (!S.loaded) S.el.textContent = '';
    });
  }

  function replace(S, t) {
    t = normalise(t);
    S.list = S.list.map(function (x) { return x.id === t.id ? t : x; });
    return t;
  }

  // act runs one write for a test at a time, carrying the version the last
  // answer returned, so quick presses at the table never trip the server's
  // conflict check. A conflict reloads and says so on the test.
  function act(S, id, path, body, focusKey) {
    if (S.busy[id]) return;
    var cur = S.list.filter(function (x) { return x.id === id; })[0];
    if (!cur) return;
    S.busy[id] = true;
    body.version = cur.version;
    api(S, 'POST', testURL(S, id, path), body).then(function (t) {
      S.busy[id] = false; S.msg[id] = '';
      replace(S, t);
      draw(S, focusKey);
    }).catch(function (err) {
      S.busy[id] = false;
      S.msg[id] = err.status === 409 ? 'Someone else changed this test. Showing their version.' : err.message;
      if (err.status === 409) { load(S).then(function () { draw(S, focusKey); }); return; }
      draw(S, focusKey);
    });
  }

  function draw(S, focusKey) {
    var el = S.el;
    var keep = focusKey || (document.activeElement && el.contains(document.activeElement) && document.activeElement.getAttribute('data-k'));
    el.textContent = '';
    if (!S.list.length && !S.canEdit) return;
    var head = h('div', 'pt-h');
    var title = h('h3', 'cb-head pt-title');
    append(title, [h('i', 'fa-solid fa-flag-checkered', '', { 'aria-hidden': 'true' }), document.createTextNode(' Tests '),
      S.list.length ? h('span', 'cb-count', String(S.list.length)) : null]);
    append(head, [title, S.canEdit ? quiet('Add a test', function () { create(S); }, {
      'data-k': 'add', disabled: S.list.length >= LIM.perPage || S.creating
    }) : null]);
    el.appendChild(head);
    if (!S.list.length) {
      el.appendChild(h('p', 'pt-empty', S.msg._new || 'Record how a montage or other long test ends. An encounter can pick its version from the result.'));
    }
    S.list.forEach(function (t) { el.appendChild(S.canEdit ? dmTest(S, t) : playerTest(t)); });
    if (keep) {
      var f = el.querySelector('[data-k="' + keep + '"]');
      if (f) f.focus();
    }
  }

  // playerTest is a shown test as anyone below the DM sees it.
  function playerTest(t) {
    var sec = h('section', 'pt-test cb-section');
    sec.appendChild(h('h4', 'pt-name', t.name));
    if (t.kind) sec.appendChild(h('p', 'pt-meta', t.kind));
    var line = h('p', 'pt-said');
    if (t.recorded) {
      append(line, [h('b', '', t.recorded.label), t.recorded.dateLabel ? h('span', 'pt-meta', ' · ' + t.recorded.dateLabel) : null]);
    } else {
      line.appendChild(h('span', 'pt-meta', 'Not settled yet'));
    }
    sec.appendChild(line);
    return sec;
  }

  // dmTest is a test for the DM: everything, with the result rows pressable.
  // A test players don't see is marked Director-only as a whole; a shown one
  // marks only what players aren't told, so viewing as a player shows exactly
  // what they get.
  function dmTest(S, t) {
    var sec = h('section', 'pt-test cb-section', '', { 'data-test': t.id, 'data-dm-only': !t.shown });
    var top = h('div', 'pt-top');
    append(top, [
      h('h4', 'pt-name', t.name),
      t.shown ? h('span', 'pt-meta', 'Players see the result') : h('span', 'cb-dm pt-dm', 'Only you see this'),
      icon('fa-pen', 'Change ' + t.name, function () { openEditor(S, t.id); }, { 'data-k': 'edit-' + t.id, 'aria-expanded': S.editing === t.id ? 'true' : 'false' })
    ]);
    sec.appendChild(top);
    var meta = [t.kind, totals(t)].filter(Boolean).join(' · ');
    if (meta) sec.appendChild(h('p', 'pt-meta', meta));
    if (t.needed || t.limit) sec.appendChild(tallyLine(S, t));
    var ul = h('ul', 'pt-results', '', { 'aria-label': 'Results of ' + t.name });
    t.results.forEach(function (r) {
      var on = !!(t.recorded && t.recorded.result === r.id);
      var li = h('li', 'pt-li', '', { 'data-dm-only': t.shown && !on });
      var b = h('button', 'pt-res' + (on ? ' on' : '') + (t.offer === r.id ? ' offer' : ''), '', {
        type: 'button', 'aria-pressed': on ? 'true' : 'false', 'data-k': 'res-' + t.id + '-' + r.id, disabled: !!S.busy[t.id]
      }, {
        click: function () { if (!on) act(S, t.id, '/record', { result: r.id }, 'res-' + t.id + '-' + r.id); }
      });
      var note = on ? 'Recorded' + (t.recorded.dateLabel ? ' ' + t.recorded.dateLabel : '') : offerNote(t, r);
      append(b, [h('span', 'pt-res-l', r.label), note ? h('span', 'pt-res-n', note) : null]);
      li.appendChild(b);
      if (on) li.appendChild(quiet('Clear', function () { act(S, t.id, '/record', { result: '' }, 'res-' + t.id + '-' + r.id); }, { 'data-k': 'clr-' + t.id, 'aria-label': 'Clear the recorded result' }));
      ul.appendChild(li);
    });
    sec.appendChild(ul);
    if (S.msg[t.id]) sec.appendChild(h('p', 'pt-err', S.msg[t.id], { role: 'status' }));
    if (S.editing === t.id) sec.appendChild(editor(S));
    return sec;
  }

  // tallyLine is the running count, stepped one at a time. Players are never
  // sent it, so it is Director-only even on a shown test.
  function tallyLine(S, t) {
    var p = h('div', 'pt-tally', '', { 'data-dm-only': true });
    function side(label, n, total, key, sideName) {
      var g = h('span', 'pt-side');
      append(g, [
        h('span', 'pt-meta', label),
        icon('fa-minus', 'One fewer ' + label.toLowerCase(), function () { act(S, t.id, '/tally', { side: sideName, step: -1 }, key + '-m'); },
          { 'data-k': key + '-m', disabled: n <= 0 || !!S.busy[t.id] }),
        h('output', 'pt-n', total ? n + ' of ' + total : String(n), { 'aria-label': label + ' ' + n + (total ? ' of ' + total : '') }),
        icon('fa-plus', 'One more ' + label.toLowerCase().replace(/s$/, ''), function () { act(S, t.id, '/tally', { side: sideName, step: 1 }, key + '-p'); },
          { 'data-k': key + '-p', disabled: (total && n >= total) || n >= LIM.total || !!S.busy[t.id] })
      ]);
      return g;
    }
    append(p, [
      t.needed ? side('Successes', t.successes, t.needed, 'ts-' + t.id, 'success') : null,
      t.limit ? side('Failures', t.failures, t.limit, 'tf-' + t.id, 'failure') : null,
      (t.successes || t.failures) ? quiet('Start again', function () { act(S, t.id, '/record', { result: '', reset: true }, 'rst-' + t.id); }, { 'data-k': 'rst-' + t.id }) : null
    ]);
    return p;
  }

  // ---------- Making and changing a test ----------
  // askSystem gives a game system one chance to answer with its starting
  // setup; a system that fails or stays quiet leaves the plain one.
  function askSystem(S) {
    var answer = null;
    try {
      document.dispatchEvent(new CustomEvent('chronicle:test-results', {
        detail: { campaignId: S.cfg.campaignId, reply: function (setup) { if (!answer) answer = setup; } }
      }));
    } catch (err) { answer = null; }
    return starter(answer);
  }

  function create(S) {
    if (S.creating) return;
    S.creating = true;
    S.msg._new = '';
    draw(S);
    api(S, 'POST', base(S) + '/pages/' + encodeURIComponent(S.cfg.entityId) + '/tests', askSystem(S)).then(function (t) {
      S.creating = false;
      t = normalise(t);
      S.list.push(t);
      openEditor(S, t.id, true);
    }).catch(function (err) {
      S.creating = false;
      S.msg._new = err.message;
      draw(S);
    });
  }

  function openEditor(S, id, fresh) {
    if (S.editing === id) { closeEditor(S, id); return; }
    var t = S.list.filter(function (x) { return x.id === id; })[0];
    if (!t) return;
    S.editing = id;
    S.draft = clone(t);
    S.armed = false;
    draw(S, 'ed-name');
    var name = S.el.querySelector('[data-k="ed-name"]');
    if (name && fresh) name.select();
  }

  function closeEditor(S, id) {
    S.editing = null; S.draft = null; S.armed = false;
    draw(S, 'edit-' + id);
  }

  function saveEditor(S) {
    var d = S.draft, id = S.editing;
    if (!d || S.busy[id]) return;
    var cur = S.list.filter(function (x) { return x.id === id; })[0];
    if (!cur) return;
    var body = toSave(d);
    body.version = cur.version;
    S.busy[id] = true;
    api(S, 'PUT', testURL(S, id), body).then(function (t) {
      S.busy[id] = false; S.msg[id] = '';
      replace(S, t);
      closeEditor(S, id);
    }).catch(function (err) {
      S.busy[id] = false;
      S.msg[id] = err.status === 409 ? 'Someone else changed this test. Open it again to see their version.' : err.message;
      if (err.status === 409) { S.editing = null; S.draft = null; load(S); return; }
      draw(S, 'ed-save');
    });
  }

  function removeTest(S, id) {
    if (S.busy[id]) return;
    S.busy[id] = true;
    api(S, 'DELETE', testURL(S, id)).then(function () {
      S.busy[id] = false;
      S.list = S.list.filter(function (x) { return x.id !== id; });
      S.editing = null; S.draft = null;
      draw(S, 'add');
    }).catch(function (err) {
      S.busy[id] = false; S.msg[id] = err.message; draw(S, 'ed-del');
    });
  }

  function field(label, control, hint) {
    var f = h('label', 'pt-field');
    append(f, [h('span', 'pt-flab', label), control, hint ? h('span', 'pt-meta', hint) : null]);
    return f;
  }

  function numberBox(S, key, value, label, onSet) {
    var n = h('input', 'pt-input pt-num', '', {
      type: 'number', min: '0', max: String(LIM.total), step: '1', inputmode: 'numeric', 'data-k': key, 'aria-label': label
    }, {
      change: function () { onSet(intIn(n.value, 0, LIM.total, 0)); draw(S, key); }
    });
    n.value = String(value);
    return n;
  }

  // editor hangs under the test it changes. Nothing is sent until Save, so a
  // half-typed result never reaches the table.
  function editor(S) {
    var d = S.draft, id = S.editing;
    var box = h('div', 'pt-editor', '', { role: 'group', 'aria-label': 'Change ' + (d.name || 'this test') });
    var name = h('input', 'pt-input', '', { type: 'text', maxlength: String(LIM.name), 'data-k': 'ed-name' }, {
      input: function () { S.draft.name = name.value; }
    });
    name.value = d.name;
    var kind = h('input', 'pt-input', '', { type: 'text', maxlength: String(LIM.kind), placeholder: 'Montage', 'data-k': 'ed-kind' }, {
      input: function () { S.draft.kind = kind.value; }
    });
    kind.value = d.kind;
    var shown = h('input', '', '', { type: 'checkbox', 'data-k': 'ed-shown' }, {
      change: function () { S.draft.shown = shown.checked; }
    });
    shown.checked = d.shown;
    var shownBox = h('label', 'pt-check');
    append(shownBox, [shown, h('span', '', 'Players see its name and result')]);

    var ends = h('div', 'pt-ends');
    append(ends, [
      field('Successes that finish it', numberBox(S, 'ed-needed', d.needed, 'Successes that finish it', function (v) { S.draft.needed = v; }), '0 for none'),
      field('Failures that end it', numberBox(S, 'ed-limit', d.limit, 'Failures that end it', function (v) { S.draft.limit = v; }), '0 for none')
    ]);

    var rows = h('div', 'pt-erows', '', { role: 'list', 'aria-label': 'Results' });
    d.results.forEach(function (r, i) {
      var row = h('div', 'pt-erow', '', { role: 'listitem' });
      var label = h('input', 'pt-input', '', { type: 'text', maxlength: String(LIM.label), 'aria-label': 'Result ' + (i + 1), 'data-k': 'ed-l-' + r.id }, {
        input: function () { S.draft.results[i].label = label.value; }
      });
      label.value = r.label;
      var on = h('select', 'pt-input pt-sel', '', { 'aria-label': 'When ' + (r.label || 'this result') + ' is offered', 'data-k': 'ed-o-' + r.id }, {
        change: function () { S.draft = setOn(S.draft, r.id, on.value); draw(S, 'ed-o-' + r.id); }
      });
      ONS.forEach(function (o) {
        var off = (o.v === 'successes' && !d.needed) || (o.v === 'failures' && !d.limit);
        on.appendChild(h('option', '', o.label, { value: o.v, selected: o.v === r.on, disabled: off && o.v !== r.on }));
      });
      append(row, [label, on, d.results.length > 1 ? icon('fa-xmark', 'Remove ' + (r.label || 'this result'), function () {
        S.draft.results.splice(i, 1); draw(S, 'ed-add');
      }, { 'data-k': 'ed-x-' + r.id }) : null]);
      rows.appendChild(row);
    });

    var acts = h('div', 'pt-eacts');
    var del = quiet(S.armed ? 'Delete for good?' : 'Delete', function () {
      if (S.armed) { removeTest(S, id); return; }
      S.armed = true; draw(S, 'ed-del');
    }, { 'data-k': 'ed-del' });
    del.classList.add('pt-danger');
    append(acts, [
      h('button', 'pt-btn', 'Save', { type: 'button', 'data-k': 'ed-save', disabled: !!S.busy[id] }, { click: function () { saveEditor(S); } }),
      quiet('Cancel', function () { closeEditor(S, id); }, { 'data-k': 'ed-cancel' }),
      del
    ]);

    append(box, [
      field('Name', name), field('Kind', kind, 'A word for your notes, like Montage or Chase'), ends,
      h('p', 'pt-flab', 'Results'), rows,
      d.results.length < LIM.results ? quiet('Add a result', function () {
        S.draft.results.push({ id: rid(), label: '', on: '' }); draw(S, 'ed-l-' + S.draft.results[S.draft.results.length - 1].id);
      }, { 'data-k': 'ed-add' }) : null,
      shownBox, acts
    ]);
    box.addEventListener('keydown', function (ev) {
      if (ev.key === 'Escape') { ev.stopPropagation(); closeEditor(S, id); }
    });
    return box;
  }
})();
