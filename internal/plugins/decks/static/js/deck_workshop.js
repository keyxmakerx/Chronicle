/**
 * Deck workshop
 *
 * The Director's pages for Card decks (internal/plugins/decks/workshop*.templ).
 * The server draws every page and fragment; this file only sends writes to the
 * deck's JSON routes and then shows what the server now says. One widget:
 *
 *   deck-workshop  mounts on both pages: on the making-a-deck page (data-dkw-new-page)
 *                  a name, who it is for and a style; on a deck's page the table
 *                  row and four tabs
 *
 * Every move asks the server first. A refused move says why in plain words,
 * changes nothing on screen and puts focus back where it was; an accepted one
 * plays the card widget's fixed move (deck_cards.js) and then swaps in the
 * server's table row. Cards are drawn only from the server's markup.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});

  // ---- Lists (pinned to settings.go by test/js/deck_workshop.test.mjs) ----

  var PALETTE = ['violet', 'indigo', 'blue', 'cyan', 'teal', 'moss', 'plum', 'rose', 'umber', 'slate'];
  var LISTS = {
    shape: ['tall', 'standard', 'small'],
    border: ['double', 'line', 'kit', 'metal', 'brass', 'steel', 'bound', 'stone', 'none'],
    turn: ['flip', 'slow', 'toss', 'slam', 'sweep', 'burn', 'ink', 'seal', 'bolt', 'forge'],
    window: ['arch', 'square', 'pointed', 'round', 'full'],
    name: ['below', 'over'],
    lettering: ['plain', 'carved', 'classic', 'condensed', 'hand'],
    corner: ['roman', 'number', 'suit', 'pips', 'none'],
    marker: ['gem', 'banner', 'tab', 'ribbon', 'line'],
    finish: ['matte', 'glow', 'sheen', 'foil'],
    'back.pattern': ['lattice', 'rays', 'waves', 'stars', 'check', 'plain', 'iron'],
    'back.sign': ['star', 'sun', 'moon', 'eye', 'crown', 'anchor', 'flame', 'sword', 'none'],
    'back.colour': [''].concat(PALETTE)
  };
  var DRAW_MIN = 1;
  var DRAW_MAX = 5; // MaxDrawCount in model.go
  var COPIES_MAX = 20; // MaxCopiesPerCard
  var MAX_KINDS = 12;

  // ---- Pure helpers (unit-tested in test/js/deck_workshop.test.mjs) ----

  /** clampCount: the draw stepper's value, at least 1 and at most what may still be shown. */
  function clampCount(n, max) {
    var hi = Math.max(DRAW_MIN, Math.min(DRAW_MAX, max == null ? DRAW_MAX : max));
    n = parseInt(n, 10);
    if (isNaN(n) || n < DRAW_MIN) return DRAW_MIN;
    return n > hi ? hi : n;
  }

  /** stepCount: the stepper after one press; stays put at either end. */
  function stepCount(n, dir, max) { return clampCount((parseInt(n, 10) || DRAW_MIN) + dir, max); }

  /**
   * lookPatch: the body that changes one look choice, or null for a path or
   * value the lists do not have (the page never sends what the server would
   * refuse by name).
   */
  function lookPatch(path, value) {
    var list = LISTS[path];
    if (!list || list.indexOf(value) < 0) return null;
    var dot = path.indexOf('.');
    if (dot < 0) { var a = {}; a[path] = value; return a; }
    var b = {}, inner = {};
    inner[path.slice(dot + 1)] = value;
    b[path.slice(0, dot)] = inner;
    return b;
  }

  /** kindKey: a key for a new kind from its name, unlike every key in use. */
  function kindKey(name, used) {
    var base = String(name || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
    if (!/^[a-z]/.test(base)) base = 'kind' + (base ? '-' + base : '');
    base = base.slice(0, 28);
    var key = base, n = 2;
    while (used.indexOf(key) >= 0) key = base + '-' + n++;
    return key;
  }

  /** nextKindName: "Kind 1", "Kind 2"... the first no kind already has. */
  function nextKindName(names) {
    for (var i = 1; ; i++) if (names.indexOf('Kind ' + i) < 0) return 'Kind ' + i;
  }

  /** isStale: whether a refused save says the deck changed under the page. */
  function isStale(message) { return /this deck changed/i.test(String(message || '')); }

  // ---- Small DOM helpers ----

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }

  function api(method, url, body) {
    var opts = { method: method };
    if (body !== undefined) opts.body = body;
    return C.apiAction(url, opts);
  }

  /** words: a refusal as a sentence, never a status code. */
  function words(err) {
    var m = err && err.message ? String(err.message) : '';
    if (!m || /^request failed|Failed to fetch|NetworkError|Load failed/i.test(m)) return 'That did not go through. Check your connection and try again.';
    return /[.!?]$/.test(m) ? m : m + '.';
  }

  // The widget's way to take the deck's version from a fragment (set while it is mounted).
  var versionSink = null;

  /** parseVersion: the deck version a server fragment carries, or 0. */
  function parseVersion(html) {
    var m = /data-dkw-version="(\d+)"/.exec(String(html || ''));
    return m ? parseInt(m[1], 10) : 0;
  }

  /** fragment: GET a server fragment as text (the same markup a swap would use). */
  function fragment(url) {
    return fetch(url, { credentials: 'same-origin', headers: { 'HX-Request': 'true', 'Accept': 'text/html' } }).then(function (r) {
      if (!r.ok) throw new Error(r.status === 404 ? 'That page is not there any more.' : 'That did not load. Try again.');
      return r.text().then(function (text) {
        var v = parseVersion(text);
        if (v && versionSink) versionSink(v);
        return text;
      });
    });
  }

  function adopt(node) {
    if (window.htmx && window.htmx.process) window.htmx.process(node);
    if (C.mountWidgets) C.mountWidgets(node);
  }

  /** loadInto: replace a container's content with a fragment, then mount what is in it. */
  function loadInto(target, url) {
    target.setAttribute('aria-busy', 'true');
    target.classList.add('is-loading');
    function done() {
      target.removeAttribute('aria-busy');
      target.classList.remove('is-loading');
    }
    return fragment(url).then(function (html) {
      target.innerHTML = html;
      adopt(target);
      done();
    }, function (e) { done(); throw e; });
  }

  /** refreshParts: swap named parts of the page from a fresh fragment, leaving the rest (and focus) alone. */
  function refreshParts(url, selectors) {
    return fragment(url).then(function (html) {
      var doc = new DOMParser().parseFromString('<div>' + html + '</div>', 'text/html');
      selectors.forEach(function (sel) {
        var fresh = $(sel, doc), old = $(sel);
        if (fresh && old) { old.replaceWith(fresh); adopt(fresh); }
      });
    });
  }

  /** rove: one tab stop per radio group; arrows move focus among its choices. */
  function rove(root) {
    $$('[role=radiogroup]', root).forEach(function (g) {
      var opts = $$('[role=radio]', g);
      var on = opts.filter(function (o) { return o.getAttribute('aria-checked') === 'true'; })[0] || opts[0];
      opts.forEach(function (o) { o.tabIndex = o === on ? 0 : -1; });
      if (g.__dkwRove) return;
      g.__dkwRove = true;
      g.addEventListener('keydown', function (ev) {
        var k = ev.key;
        if (k !== 'ArrowRight' && k !== 'ArrowLeft' && k !== 'ArrowDown' && k !== 'ArrowUp') return;
        var all = $$('[role=radio]', g).filter(function (o) { return !o.hidden; });
        var i = all.indexOf(document.activeElement);
        if (i < 0) return;
        ev.preventDefault();
        var j = (i + (k === 'ArrowRight' || k === 'ArrowDown' ? 1 : -1) + all.length) % all.length;
        all.forEach(function (o) { o.tabIndex = -1; });
        all[j].tabIndex = 0;
        all[j].focus();
      });
    });
  }

  /** pickedValue: the value of the checked choice in a radio group. */
  function pickedValue(group) {
    var on = group && $('[role=radio][aria-checked=true]', group);
    return on ? on.getAttribute('data-value') : '';
  }

  function check(group, chosen) {
    $$('[role=radio]', group).forEach(function (o) {
      var on = o === chosen;
      o.setAttribute('aria-checked', on ? 'true' : 'false');
      o.tabIndex = on ? 0 : -1;
    });
  }


  // ---- Comes up when: a card's tests (pinned to when.go by test/js/deck_workshop.test.mjs) ----

  var MAX_TESTS = 4; // MaxTests in when.go
  var KIND_ORDER = ['number', 'tracker', 'tag', 'layer', 'say'];
  // The keys each kind of test may carry besides "test" and "open"; the server refuses any other.
  var TEST_KEYS = {
    number: ['who', 'field', 'op', 'value', 'of', 'share'],
    tracker: ['who', 'kind', 'op', 'value'],
    tag: ['who', 'tag', 'has'],
    layer: ['who', 'name', 'on'],
    say: []
  };
  var ORDINALS = ['first', 'second', 'third', 'fourth'];

  function copy(o) {
    var out = {};
    Object.keys(o || {}).forEach(function (k) { out[k] = o[k]; });
    return out;
  }

  /** whenBody: the list the card update takes: each test with only its own kind's keys, never a draft. */
  function whenBody(tests) {
    return (tests || []).filter(function (t) { return !t._draft; }).map(function (t) {
      var o = { test: t.test, open: !!t.open };
      (TEST_KEYS[t.test] || []).forEach(function (k) {
        if (t[k] !== undefined && t[k] !== null && t[k] !== '') o[k] = t[k];
      });
      return o;
    });
  }

  function possessive(n, on) {
    if (!on) return n;
    return /s$/.test(n) ? n + '’' : n + '’s';
  }

  /** shareLabel: a share as the editor says it ("half"), from the server's phrase ("half of"). */
  function shareLabel(w) {
    var p = String(w.phrase || '').replace(/ of$/, '');
    return p || 'all';
  }

  function firstWith(list, key) {
    for (var i = 0; i < (list || []).length; i++) if (list[i][key]) return list[i];
    return null;
  }

  /**
   * kindAvailable: whether the add menu can offer a kind now. A kind that
   * needs something the campaign has none of is left off rather than offered
   * and refused.
   */
  function kindAvailable(kind, o) {
    if (kind === 'say' || kind === 'layer') return kind === 'say' || !!o.available;
    if (!o.available) return false;
    if (kind === 'number') return (o.fields || []).length > 0;
    if (kind === 'tracker') return (o.trackers || []).length > 0;
    if (kind === 'tag') return (o.tags || []).length > 0;
    return false;
  }

  /**
   * newTest: a test of a kind with sensible first values. On a hero deck it is
   * about that hero; a number test starts at "at or below half of" the first
   * field that has a max field. A layer with no name on any hero yet starts as
   * a draft, which is not saved until it has a name.
   */
  function newTest(kind, c, o) {
    var t = { test: kind, open: false };
    if (kind === 'say') return t;
    t.who = c.heroDeck && c.holder ? 'holder' : 'any';
    if (kind === 'number') {
      var f = firstWith(o.fields, 'maxKey') || (o.fields || [])[0];
      if (!f) return null;
      t.field = f.key;
      t.op = 'lte';
      if (f.maxKey) { t.of = f.maxKey; t.share = 'half'; } else { t.value = 0; }
    } else if (kind === 'tracker') {
      var k = (o.trackers || [])[0];
      if (!k) return null;
      t.kind = k.id; t.op = 'gte'; t.value = 1;
    } else if (kind === 'tag') {
      var g = (o.tags || [])[0];
      if (!g) return null;
      t.tag = g.id; t.has = true;
    } else if (kind === 'layer') {
      t.name = (o.layers || [])[0] || '';
      t.on = true;
      if (!t.name) t._draft = true;
    }
    return t;
  }

  function fieldOf(o, key) {
    var f = (o.fields || []).filter(function (x) { return x.key === key; })[0];
    return f || null;
  }

  /** otherField: what a share is "of" when none is set: the field's max field, else another field. */
  function otherField(o, field) {
    var f = fieldOf(o, field);
    if (f && f.maxKey) return f.maxKey;
    var other = (o.fields || []).filter(function (x) { return x.key !== field; })[0];
    return other ? other.key : field;
  }

  /**
   * changeTest: the test after one blank changes, as a new object. Choosing a
   * share turns "a number" into a share of another field and back.
   */
  function changeTest(t, key, val, o) {
    var n = copy(t);
    switch (key) {
      case 'who': case 'op': case 'field': case 'of': case 'kind': case 'name':
        n[key] = val;
        if (key === 'field' && n.of !== undefined && n.share) {
          var f = fieldOf(o, val);
          if (f && f.maxKey) n.of = f.maxKey;
        }
        if (key === 'name') delete n._draft;
        break;
      case 'tag': n.tag = parseInt(val, 10); break;
      case 'has': n.has = val === 'true'; break;
      case 'on': n.on = val === 'true'; break;
      case 'value': n.value = val; break;
      case 'compare':
        if (val === 'number') {
          delete n.of; delete n.share;
          n.value = t.value != null ? t.value : 0;
        } else {
          delete n.value;
          n.share = val;
          if (!n.of) n.of = otherField(o, n.field);
        }
        break;
      default: break;
    }
    return n;
  }

  /** parseNumber: what was typed in a number blank, or null when it is not a number. */
  function parseNumber(s) {
    var x = String(s == null ? '' : s).trim().replace(',', '.');
    if (x === '' || !/^[-+]?(\d+\.?\d*|\.\d+)$/.test(x)) return null;
    var v = Number(x);
    return isFinite(v) ? v : null;
  }

  /** whenHint: the words under the card that say which conditions it does not show. */
  function whenHint(tests) {
    var printed = [], own = [];
    (tests || []).forEach(function (t, i) { if (i < ORDINALS.length) (t.open ? printed : own).push(ORDINALS[i]); });
    if (!printed.length || !own.length) return '';
    var and = function (w) { return w.length > 1 ? w.slice(0, -1).join(', ') + ' and ' + w[w.length - 1] : w[0]; };
    return 'The ' + and(own) + (own.length > 1 ? ' conditions are' : ' condition is') + ' yours alone, so the card shows only the ' + and(printed) + '.';
  }

  /** nowLine: the one line of state under the rows. */
  function nowLine(tests, st) {
    if (!(tests || []).length) return '';
    if (st && st.fitsNow) return 'Fits now.';
    var why = (st && st.whyNot) || 'not yet';
    return 'Waiting: ' + (/[.!?]$/.test(why) ? why : why + '.');
  }

  function hasSay(tests) { return (tests || []).some(function (t) { return t.test === 'say'; }); }

  // ---- Small DOM helpers for the rows ----

  function node(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  }

  /** blank: a select drawn as an underlined blank, the same control the Rules tab uses. */
  function blank(label, choices, current, onPick) {
    var sel = node('select', 'dw-blank is-filled');
    sel.setAttribute('aria-label', label);
    var group = null;
    var seen = false;
    choices.forEach(function (c) {
      var op = node('option', '', c.l);
      op.value = String(c.v);
      if (String(c.v) === String(current)) { op.selected = true; seen = true; }
      if (c.group) {
        if (!group) { group = node('optgroup'); group.label = c.group; sel.appendChild(group); }
        group.appendChild(op);
      } else {
        sel.appendChild(op);
      }
    });
    if (!seen) {
      var gone = node('option', '', 'something that is gone');
      gone.value = String(current);
      gone.selected = true;
      sel.insertBefore(gone, sel.firstChild);
    }
    sel.addEventListener('change', function () { onPick(sel.value); });
    return sel;
  }

  function numberBlank(label, current, onPick) {
    var inp = node('input', 'dkw-numblank');
    inp.type = 'text';
    inp.setAttribute('inputmode', 'decimal');
    inp.setAttribute('autocomplete', 'off');
    inp.setAttribute('aria-label', label);
    inp.value = current == null ? '' : String(current);
    inp.addEventListener('change', function () { onPick(inp.value); });
    return inp;
  }

  function whoChoices(kind, env) {
    var poss = kind === 'number' || kind === 'tracker';
    var out = [];
    var c = env.ctx;
    if (c.heroDeck && c.holder) out.push({ v: 'holder', l: possessive(c.holderName || 'the hero', poss) });
    out.push({ v: 'any', l: poss ? 'any hero’s' : 'any hero' });
    (env.opts.heroes || []).forEach(function (h) {
      if (h.id !== c.holder) out.push({ v: h.id, l: possessive(h.name, poss), group: 'A page' });
    });
    return out;
  }

  /** rowSentence fills a row's sentence node with words and blanks for a test. */
  function rowSentence(p, t, env) {
    var o = env.opts, n = 0;
    function say(w) { p.appendChild(node('span', 'dkw-w', w)); }
    // slot adds one blank; make gets the blank's number, which a redraw uses to put focus back.
    function slot(make) {
      var k = n;
      var el = make(k);
      el.setAttribute('data-bk', String(k));
      p.appendChild(el);
      n++;
      return el;
    }
    function pickOf(key, k) { return function (v) { env.change(changeTest(t, key, v, o), 'b' + k); }; }
    function numberOf(key, k) {
      return function (v) {
        var x = parseNumber(v);
        if (x == null) { env.refuse('Type a number.', 'b' + k); return; }
        env.change(changeTest(t, key, x, o), 'b' + k);
      };
    }
    function choose(label, list, cur, key) {
      slot(function (k) { return blank(label, list, cur, pickOf(key, k)); });
    }
    var opChoices = (o.ops || []).map(function (w) { return { v: w.key, l: w.phrase }; });
    if (t.test === 'say') { say('the Director says so'); return; }
    choose('Whose page', whoChoices(t.test, env), t.who, 'who');
    if (t.test === 'number') {
      choose('Which number', (o.fields || []).map(function (f) { return { v: f.key, l: f.label }; }), t.field, 'field');
      say('is');
      choose('How it compares', opChoices, t.op, 'op');
      var cmp = [{ v: 'number', l: 'a number' }].concat((o.shares || []).map(function (w) { return { v: w.key, l: shareLabel(w) }; }));
      choose('A number or a share', cmp, t.value != null ? 'number' : (t.share || 'all'), 'compare');
      if (t.value != null) {
        slot(function (k) { return numberBlank('The number', t.value, numberOf('value', k)); });
      } else {
        say('of');
        var f = fieldOf(o, t.field);
        var rest = (o.fields || []).slice();
        // The field's own max field comes first.
        if (f && f.maxKey) rest.sort(function (a, b) { return (b.key === f.maxKey ? 1 : 0) - (a.key === f.maxKey ? 1 : 0); });
        choose('Of which number', rest.map(function (x) { return { v: x.key, l: x.label }; }), t.of, 'of');
      }
    } else if (t.test === 'tracker') {
      choose('Which tracker', (o.trackers || []).map(function (k) { return { v: k.id, l: k.name }; }), t.kind, 'kind');
      say('is');
      choose('How it compares', opChoices, t.op, 'op');
      slot(function (k) { return numberBlank('The number', t.value, numberOf('value', k)); });
    } else if (t.test === 'tag') {
      choose('Has or does not have', [{ v: 'true', l: 'has' }, { v: 'false', l: 'does not have' }], String(t.has !== false), 'has');
      say('the tag');
      choose('Which tag', (o.tags || []).map(function (g) { return { v: g.id, l: g.name }; }), t.tag, 'tag');
    } else if (t.test === 'layer') {
      choose('Is or is not', [{ v: 'true', l: 'is' }, { v: 'false', l: 'is not' }], String(t.on !== false), 'on');
      slot(function (k) {
        var nm = node('input', 'dkw-textblank');
        nm.type = 'text';
        nm.maxLength = 80;
        nm.setAttribute('autocomplete', 'off');
        nm.setAttribute('aria-label', 'The transformation’s name');
        nm.placeholder = 'a transformation';
        nm.value = t.name || '';
        var dl = node('datalist');
        dl.id = 'dkw-layers-' + env.index;
        (o.layers || []).forEach(function (l) { var op = node('option'); op.value = l; dl.appendChild(op); });
        nm.setAttribute('list', dl.id);
        nm.addEventListener('change', function () {
          var v = nm.value.trim();
          if (!v) { env.refuse('Type the name of a transformation.', 'b' + k); return; }
          env.change(changeTest(t, 'name', v, o), 'b' + k);
        });
        p.appendChild(dl);
        return nm;
      });
    }
  }

  /**
   * buildRow: one test as a row: whose it is, the sentence with its blanks and
   * a remove button. env carries the options, the deck's hero and the
   * callbacks that save.
   */
  function buildRow(t, i, env) {
    var li = node('li', 'dkw-when-row' + (t.open ? '' : ' is-own') + (t.broken ? ' is-broken' : ''));
    li.setAttribute('data-when-row', String(i));
    var mark = node('button', 'dkw-mark' + (t.open ? '' : ' is-own'), t.open ? 'On the card' : 'Only you');
    mark.type = 'button';
    mark.setAttribute('data-act', 'when-mark');
    mark.setAttribute('aria-label', (t.open ? 'On the card' : 'Only you') + '. Press to make it ' + (t.open ? 'only yours' : 'show on the card') + '.');
    mark.addEventListener('click', function () { var n = copy(t); n.open = !t.open; env.change(n, 'mark'); });
    li.appendChild(mark);
    var p = node('p', 'dkw-when-s');
    p.appendChild(node('span', 'dkw-lead', i === 0 ? 'When' : 'and'));
    env.index = i;
    if (t.broken) {
      p.appendChild(node('span', 'dkw-gone', 'This no longer points at anything.'));
      if (t.sentence) p.appendChild(node('span', 'dkw-quiet dkw-was', t.sentence));
    } else if (env.plain) {
      p.appendChild(node('span', 'dkw-w', t.sentence || ''));
    } else {
      rowSentence(p, t, env);
    }
    li.appendChild(p);
    var rm = node('button', 'dkw-rm');
    rm.type = 'button';
    rm.setAttribute('data-act', 'when-remove');
    rm.setAttribute('aria-label', 'Remove this condition' + (t.sentence ? ': ' + t.sentence : ''));
    rm.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"></path></svg>';
    rm.addEventListener('click', function () { env.remove(); });
    li.appendChild(rm);
    var err = node('p', 'dkw-err dkw-when-rowerr');
    err.setAttribute('role', 'alert');
    err.setAttribute('data-dkw-err', '');
    err.hidden = true;
    li.appendChild(err);
    return li;
  }

  // ---- deck-new ----

  function initNew(el) {
    {
      var cid = el.getAttribute('data-campaign-id');
      var form = $('[data-dkw-new]', el);
      var err = $('[data-dkw-err]', el);
      var kindGroup = $('.dkw-chips', form);
      var styleGroup = $('.dkw-styles', form);
      var hero = $('[data-dkw-hero]', form);
      var kind = 'hero';
      rove(el);

      function say(msg, focusEl) {
        err.textContent = msg;
        err.hidden = !msg;
        if (focusEl) focusEl.focus();
      }
      kindGroup.addEventListener('click', function (ev) {
        var b = ev.target.closest('[data-kind]');
        if (!b) return;
        kind = b.getAttribute('data-kind');
        check(kindGroup, b);
        hero.hidden = kind !== 'hero';
      });
      styleGroup.addEventListener('click', function (ev) {
        var b = ev.target.closest('[data-style]');
        if (b) check(styleGroup, b);
      });

      form.addEventListener('submit', function (ev) {
        ev.preventDefault();
        var nameEl = $('[name=name]', form);
        var name = nameEl.value.trim();
        if (!name) return say('Give the deck a name.', nameEl);
        var style = $('[data-style][aria-checked=true]', styleGroup);
        var look = {};
        try { look = JSON.parse(style.getAttribute('data-look') || '{}'); } catch (e) { look = {}; }
        var body = { name: name, kind: kind, look: look };
        var sel = $('[name=holderEntityId]', form);
        if (kind === 'hero' && sel && sel.value) body.holderEntityId = sel.value;
        var btn = $('[type=submit]', form);
        say('');
        btn.disabled = true;
        if (C.working) C.working(btn, true);
        api('POST', '/campaigns/' + cid + '/decks', body).then(function (deck) {
          location.assign('/campaigns/' + cid + '/decks/workshop/' + encodeURIComponent(deck.id) + '?tab=cards');
        }, function (e) {
          btn.disabled = false;
          if (C.working) C.working(btn, false);
          say(words(e), nameEl);
        });
      });
    }
  }

  // ---- deck-workshop ----

  var MOVE_AT = {
    draw: { dx: 70, dy: 0 }, deal: { dx: 40, dy: 60 }, back: { dx: -70, dy: 0 }, used: { dx: 40, dy: 0 }
  };

  /** play: one fixed move on one card, then the card is hidden so the server's row can replace it without a flash. */
  function play(el, name, at) {
    var DC = window.DeckCards;
    if (!el || !DC || !DC.move) return Promise.resolve();
    return DC.move(el, name, { dx: at.dx, dy: at.dy, done: function (e) { e.style.visibility = 'hidden'; } });
  }

  C.register('deck-workshop', {
    init: function (el) {
      if (el.hasAttribute('data-dkw-new-page')) return initNew(el);
      var cid = el.getAttribute('data-campaign-id');
      var did = el.getAttribute('data-deck-id');
      var base = '/campaigns/' + cid + '/decks/';
      var deckURL = base + did;
      var panel = $('#dkw-panel', el);
      var count = 1;
      var kindsOpen = false;
      var savedTimer = 0;
      this.el = el;

      function version() { return parseInt(el.getAttribute('data-version'), 10) || 0; }
      versionSink = function (v) { if (v > version()) el.setAttribute('data-version', String(v)); };
      function setVersion(deck) {
        if (deck && typeof deck.version === 'number') el.setAttribute('data-version', String(deck.version));
        var n = deck && deck.counts;
        var total = $('[data-dkw-total]', el);
        if (n && total) {
          var sum = (n.pile || 0) + (n.shown || 0) + (n.hand || 0) + (n.play || 0) + (n.used || 0) + (n.gone || 0);
          total.textContent = sum === 1 ? '1 card' : sum + ' cards';
        }
      }
      function mark(msg) {
        var s = $('[data-dkw-saved]', el);
        if (!s) return;
        s.textContent = msg;
        s.classList.toggle('is-on', !!msg);
        clearTimeout(savedTimer);
        if (msg) savedTimer = setTimeout(function () { s.textContent = ''; s.classList.remove('is-on'); }, 2200);
      }
      function saved() { mark('Saved'); }

      /** say: a refusal in plain words beside the thing that failed; it stays until dismissed. */
      function say(scope, msg) {
        var box = (scope && $('[data-dkw-err]', scope)) || $('[data-dkw-err]', panel) || $('[data-dkw-err]', el);
        if (!box) return;
        box.textContent = msg || '';
        box.hidden = !msg;
        if (msg && !box.querySelector('button')) {
          var b = document.createElement('button');
          b.type = 'button';
          b.className = 'dkw-word';
          b.textContent = 'Dismiss';
          b.addEventListener('click', function () { box.hidden = true; });
          box.appendChild(document.createTextNode(' '));
          box.appendChild(b);
        }
      }
      function clearErr() { $$('[data-dkw-err]', el).forEach(function (b) { b.hidden = true; }); }

      /** put: a deck write with the version the page loaded. */
      function put(patch) {
        var body = { version: version() };
        Object.keys(patch).forEach(function (k) { body[k] = patch[k]; });
        return api('PUT', deckURL, body).then(function (deck) { setVersion(deck); return deck; });
      }

      function refreshTable() { return refreshParts(base + 'workshop/' + did + '/table', ['#dkw-table']).then(wireTable); }
      function wireTable() {
        var t = $('[data-dkw-table]', el);
        if (!t) return;
        var max = parseInt(t.getAttribute('data-maxdraw'), 10);
        count = clampCount(count, isNaN(max) ? DRAW_MAX : max);
        var out = $('[data-dkw-count]', t);
        if (out) out.textContent = String(count);
      }
      wireTable();

      /** focusAfter: put focus back on a control with the same job once the row is redrawn. */
      function focusAfter(sel) {
        var n = $(sel, el);
        if (n && !n.disabled) n.focus({ preventScroll: true });
      }

      /** stale: the deck changed under the page; reload what is shown and say so. */
      function stale() {
        return Promise.all([loadTab(currentTab(), true), refreshTable()]).then(function () {
          say(panel, 'This deck changed somewhere else, so it was reloaded. Your last change was not saved.');
        });
      }
      function failed(e, scope, focusSel) {
        if (isStale(e && e.message)) { stale(); return; }
        say(scope, words(e));
        if (focusSel) focusAfter(focusSel);
      }

      // ---- tabs ----
      function currentTab() { return el.getAttribute('data-tab') || 'cards'; }
      function tabButton(tab) { return $('.dkw-tab[data-tab="' + tab + '"]', el); }

      function loadTab(tab, keepCard, extra) {
        var b = tabButton(tab);
        if (!b) return Promise.resolve();
        var url = b.getAttribute('data-url') + (extra || '');
        $$('.dkw-tab', el).forEach(function (t) {
          var on = t === b;
          t.setAttribute('aria-selected', on ? 'true' : 'false');
          t.tabIndex = on ? 0 : -1;
        });
        el.setAttribute('data-tab', tab);
        panel.setAttribute('aria-labelledby', b.id);
        return loadInto(panel, url).then(function () { afterPanel(); }, function (e) {
          panel.textContent = '';
          var p = document.createElement('p');
          p.className = 'dkw-err';
          p.setAttribute('role', 'alert');
          p.textContent = words(e);
          panel.appendChild(p);
        });
      }
      function afterPanel() {
        rove(panel);
        var k = $('[data-dkw-kinds]', panel);
        if (k && kindsOpen) { k.hidden = false; var t = $('[data-act=toggle-kinds]', panel); if (t) t.setAttribute('aria-expanded', 'true'); }
        $$('[data-dkw-rules] select, [data-dkw-look] select', panel).forEach(function (s) { s.setAttribute('data-prev', s.value); });
        initWhen();
      }
      afterPanel();

      function showTab(tab, extra) {
        var url = new URL(location.href);
        url.searchParams.set('tab', tab);
        if (!extra) url.searchParams.delete('card');
        try { history.replaceState(null, '', url.pathname + url.search); } catch (e) { /* the address bar is a convenience */ }
        return loadTab(tab, false, extra);
      }

      el.addEventListener('keydown', function (ev) {
        var t = ev.target.closest && ev.target.closest('.dkw-tab');
        if (!t) return;
        var tabs = $$('.dkw-tab', el), i = tabs.indexOf(t), j = -1;
        if (ev.key === 'ArrowRight') j = (i + 1) % tabs.length;
        else if (ev.key === 'ArrowLeft') j = (i - 1 + tabs.length) % tabs.length;
        else if (ev.key === 'Home') j = 0;
        else if (ev.key === 'End') j = tabs.length - 1;
        if (j < 0) return;
        ev.preventDefault();
        tabs[j].focus();
        showTab(tabs[j].getAttribute('data-tab')).then(function () { tabs[j].focus(); });
      });

      // ---- the table row ----
      function table() { return $('[data-dkw-table]', el); }

      function row(node) { return node.closest('[data-copy]'); }

      function verb(node, name, body, moveName, at, cardEl, focusSel) {
        var li = row(node);
        var copy = li.getAttribute('data-copy');
        node.disabled = true;
        return api('POST', deckURL + '/copies/' + encodeURIComponent(copy) + '/' + name, body || {}).then(function () {
          clearErr();
          return play(cardEl || $('[data-dk-card]', li), moveName, at);
        }).then(function () { return refreshTable(); }).then(function () { focusAfter(focusSel || '[data-act=draw]'); }, function (e) {
          node.disabled = false;
          failed(e, table(), null);
          node.focus();
        });
      }

      function draw(btn) {
        var t = table();
        var max = parseInt(t.getAttribute('data-maxdraw'), 10);
        btn.disabled = true;
        return api('POST', deckURL + '/draw', { count: clampCount(count, max) }).then(function () {
          clearErr();
          return play($('[data-dkw-pile] [data-dk-card]', t), 'draw', MOVE_AT.draw);
        }).then(function () { return refreshTable(); }).then(function () { focusAfter('[data-act=draw]'); }, function (e) {
          btn.disabled = false;
          failed(e, t, null);
          btn.focus();
        });
      }

      function nudge(node, id) {
        var which = node.getAttribute('data-which');
        var patch = {};
        patch[which === 'next' ? 'nextCardId' : 'lastCardId'] = id === '' ? null : id;
        return put(patch).then(function () { clearErr(); saved(); return refreshTable(); }).then(function () {
          focusAfter('#dkw-nudge-' + which);
        }, function (e) { failed(e, table(), '#dkw-nudge-' + which); });
      }

      // ---- the Cards tab ----
      function cardsRoot() { return $('[data-dkw-cards]', panel); }
      function editor() { return $('[data-dkw-editor]', panel); }
      function cardURL(id) { return deckURL + '/cards/' + encodeURIComponent(id); }

      function openCard(id, focusSel) {
        return showTab('cards', '?card=' + encodeURIComponent(id)).then(function () {
          if (focusSel) focusAfter(focusSel);
          else { var a = $('.dkw-cardlink[data-card="' + id + '"]', panel); if (a) a.focus({ preventScroll: true }); }
        });
      }

      function saveCard(patch, scope, revert) {
        var ed = editor();
        if (!ed) return Promise.resolve();
        var id = ed.getAttribute('data-card-id');
        return api('PUT', cardURL(id), patch).then(function (deck) {
          setVersion(deck);
          clearErr();
          saved();
          return Promise.all([
            refreshParts(base + 'workshop/' + did + '/cards?card=' + encodeURIComponent(id), ['#dkw-grid', '#dkw-preview']),
            refreshTable()
          ]);
        }, function (e) {
          if (revert) revert();
          failed(e, scope || ed, null);
        });
      }

      function addCard(btn) {
        var root = cardsRoot();
        var name = root.getAttribute('data-newname') || 'New card';
        btn.disabled = true;
        return api('POST', deckURL + '/cards', { name: name, copies: 1 }).then(function (deck) {
          setVersion(deck);
          var found = (deck.cards || []).filter(function (c) { return c.name === name; })[0];
          if (!found) return showTab('cards');
          return openCard(found.id, '#dkw-f-name').then(function () { var n = $('#dkw-f-name', panel); if (n) n.select(); });
        }).then(function () { return refreshTable(); }, function (e) {
          btn.disabled = false;
          failed(e, root, null);
          btn.focus();
        });
      }

      function askDelete(box, o) {
        if (C.askSlip) return C.askSlip(box, o);
        if (window.confirm(o.ask + ' ' + o.detail)) o.onDiscard();
      }

      // ---- the Look tab ----
      function lookRoot() { return $('[data-dkw-look]', panel); }

      function rowsExpanded() {
        return $$('[data-dkw-look] [data-act=show-all][aria-expanded=true]', panel).map(function (b) { return b.closest('[data-row]').getAttribute('data-row'); });
      }

      /** reloadLook: redraw the whole Look tab from the server, keeping folds, the kinds panel and focus. */
      function reloadLook(focus, extra) {
        var open = rowsExpanded();
        return loadTab('look', false, extra || '').then(function () {
          open.forEach(function (r) { var b = $('[data-row="' + r + '"] [data-act=show-all]', panel); if (b) toggleAll(b, true); });
          if (focus) { var n = $(focus, panel); if (n) n.focus({ preventScroll: true }); }
        });
      }

      function toggleAll(btn, on) {
        var rowEl = btn.closest('[data-row]');
        var open = on == null ? btn.getAttribute('aria-expanded') !== 'true' : on;
        $$('.dkw-opt', rowEl).forEach(function (o, i) { if (i >= 8) o.hidden = !open; });
        btn.setAttribute('aria-expanded', open ? 'true' : 'false');
        btn.textContent = open ? 'Show fewer' : 'Show all ' + btn.getAttribute('data-count');
      }

      function playTurn() {
        var DC = window.DeckCards;
        var stage = $('[data-dkw-look] .dkw-stage', panel);
        if (!stage || !DC) return;
        var slot = $('[data-dk-slot]', stage), tpl = $('template.dk-down-tpl', stage);
        if (!slot || !tpl) return;
        slot.innerHTML = tpl.innerHTML; // the server's own markup for the card face down
        var card = $('[data-dk-card]', slot);
        if (card) DC.turnOver(card);
      }

      function choose(opt) {
        var path = opt.getAttribute('data-path'), value = opt.getAttribute('data-value');
        if (opt.getAttribute('aria-disabled') === 'true') return;
        var group = opt.closest('[role=radiogroup]');
        var was = $('[aria-checked=true]', group);
        var patch = lookPatch(path, value);
        if (!patch) return;
        if (was === opt) { if (path === 'turn') playTurn(); return; }
        check(group, opt); // answers at once; put back below if the server says no
        var cur = opt.closest('[data-row]').querySelector('[data-dkw-cur]');
        var prevWord = cur ? cur.textContent : '';
        if (cur) cur.textContent = opt.getAttribute('data-word');
        return put({ look: patch }).then(function () {
          clearErr();
          saved();
          return reloadLook('[data-path="' + path + '"][data-value="' + value + '"]');
        }).then(function () { if (path === 'turn') playTurn(); }, function (e) {
          if (isStale(e && e.message)) return stale();
          if (was) check(group, was);
          if (cur) cur.textContent = prevWord;
          say(lookRoot(), words(e));
          opt.focus();
        });
      }

      function applyStyle(btn) {
        var look;
        try { look = JSON.parse(btn.getAttribute('data-look')); } catch (e) { return; }
        var focusSel = '[data-act=apply-style][data-style="' + btn.getAttribute('data-style') + '"]';
        function apply() {
          return put({ look: look }).then(function () {
            clearErr();
            saved();
            return reloadLook(focusSel);
          }, function (e) { failed(e, lookRoot(), focusSel); });
        }
        var note = $('[data-dkw-stylenote]', panel);
        if (note && note.getAttribute('data-changed') === 'true') {
          var nm = btn.querySelector('b') ? btn.querySelector('b').textContent : 'this style';
          askDelete($('.dkw-startrow', panel), {
            ask: 'Start from ' + nm + '?',
            detail: 'This replaces the shape, border, turning over, picture window, lettering, corner mark, kind marker, finish and the back\u2019s pattern, colour and sign with the style\u2019s. Your kinds and the words on the back stay as they are.',
            discard: 'Replace them',
            onDiscard: apply
          });
          return Promise.resolve();
        }
        return apply();
      }

      function kindsFromDom() {
        return $$('[data-dkw-kindlist] .dkw-kindrow', panel).map(function (r) {
          return {
            key: r.getAttribute('data-kind-key'),
            name: $('[data-kind-name]', r).value.trim(),
            colour: pickedValue($('[data-kind-colour]', r)),
            sign: pickedValue($('[data-kind-sign]', r))
          };
        });
      }

      function saveKinds(kinds, focus) {
        return put({ look: { kinds: kinds } }).then(function () {
          clearErr();
          saved();
          kindsOpen = true;
          return reloadLook(focus).then(function () { return refreshTable(); });
        }, function (e) {
          if (isStale(e && e.message)) return stale();
          var kp = $('[data-dkw-kinds]', panel);
          say(kp, words(e));
        });
      }

      // ---- the Rules tab ----
      function saveRule(sel) {
        var key = sel.getAttribute('data-rule');
        var v = sel.value;
        var settings = {};
        settings[key] = key === 'discard' ? v === 'true' : v;
        return put({ settings: settings }).then(function () { clearErr(); sel.setAttribute('data-prev', v); saved(); }, function (e) {
          sel.value = sel.getAttribute('data-prev');
          failed(e, $('[data-dkw-rules]', panel), null);
          sel.focus();
        });
      }

      // ---- the Players tab ----
      function savePlayers(node) {
        var players = [];
        $$('[data-dkw-players] .dkw-prow', panel).forEach(function (r) {
          var on = $('[data-chosen]', r).checked;
          var left = $('[data-left]', r);
          left.disabled = !on;
          if (!on) return;
          var n = left.value.trim() === '' ? null : parseInt(left.value, 10);
          players.push({ userId: r.getAttribute('data-user'), drawsLeft: n == null || isNaN(n) ? null : n });
        });
        return api('PUT', deckURL + '/players', { version: version(), players: players }).then(function (deck) {
          setVersion(deck); clearErr(); saved();
        }, function (e) { failed(e, $('[data-dkw-players]', panel), null); loadTab('players'); });
      }

      // ---- Comes up when (the Cards tab's card editor) ----
      function whenSec() { return $('[data-dkw-when]', panel); }
      function readJSON(text, fallback) {
        try { var v = JSON.parse(text); return v == null ? fallback : v; } catch (e) { return fallback; }
      }

      /** loadOptions: what the blanks may offer, read fresh each time a card's editor opens. */
      function loadOptions() {
        return fetch(base + 'when-options', { credentials: 'same-origin', headers: { 'Accept': 'application/json' } }).then(function (r) {
          if (!r.ok) throw new Error('The choices for a condition could not be read. Close the card and open it again.');
          return r.json();
        });
      }

      function initWhen() {
        var sec = whenSec();
        if (!sec) return;
        var w = {
          tests: readJSON(sec.getAttribute('data-when'), []),
          state: readJSON(sec.getAttribute('data-state'), {}),
          opts: null, draft: null,
          ctx: { heroDeck: sec.getAttribute('data-hero-deck') === 'true', holder: sec.getAttribute('data-holder') || '', holderName: sec.getAttribute('data-holder-name') || '' }
        };
        sec.__w = w;
        drawWhen(sec);
        loadOptions().then(function (o) {
          if (sec.__w !== w) return;
          w.opts = o;
          drawWhen(sec);
        }, function (e) {
          if (sec.__w !== w) return;
          sectionSay(sec, words(e));
        });
      }

      function sectionSay(sec, msg) {
        var box = $('[data-dkw-when-err]', sec);
        if (!box) return;
        box.textContent = msg || '';
        box.hidden = !msg;
      }

      /** rowSay: a refusal in the server's words, in the row it belongs to. */
      function rowSay(sec, idx, msg) {
        var row = idx == null ? null : $('[data-when-row="' + idx + '"]', sec);
        var box = row && $('[data-dkw-err]', row);
        if (!box) { sectionSay(sec, msg); return; }
        box.textContent = msg;
        box.hidden = false;
      }

      function lock(sec) { $$('select, input, button', sec).forEach(function (n) { n.disabled = true; }); }

      function drawWhen(sec, focus) {
        var w = sec.__w;
        if (!w) return;
        var list = $('[data-dkw-whens]', sec);
        var all = w.tests.concat(w.draft ? [w.draft] : []);
        list.textContent = '';
        sectionSay(sec, '');
        all.forEach(function (t, i) {
          var env = {
            opts: w.opts || {}, ctx: w.ctx, plain: !w.opts, index: i,
            change: function (n, key) { commit(sec, i, n, key); },
            refuse: function (msg, key) { drawWhen(sec, { row: i, key: key }); rowSay(sec, i, msg); },
            remove: function () { removeAt(sec, i); }
          };
          list.appendChild(buildRow(t, i, env));
        });
        drawAdd(sec);
        drawNow(sec);
        if (!focus) return;
        var target = null;
        if (focus.key === 'add') target = $('.dkw-addsel', sec) || $('.dkw-mark', sec);
        else {
          var row = $('[data-when-row="' + (focus.key === 'new' ? all.length - 1 : focus.row) + '"]', sec);
          if (row) target = focus.key === 'mark' ? $('.dkw-mark', row) : $('[data-bk="' + (focus.key === 'new' ? '0' : focus.key.slice(1)) + '"]', row) || $('.dkw-mark', row);
        }
        if (target) target.focus({ preventScroll: true });
      }

      function drawAdd(sec) {
        var w = sec.__w, box = $('[data-dkw-whenadd]', sec);
        box.textContent = '';
        if (!w.opts) return;
        var count = w.tests.length + (w.draft ? 1 : 0);
        box.appendChild(node('span', 'dkw-quiet dkw-whenhint', 'Every condition must hold. Up to four.'));
        if (count >= MAX_TESTS) return;
        var sel = node('select', 'dkw-addsel');
        sel.setAttribute('aria-label', count ? 'And another condition' : 'Add a condition');
        var head = node('option', '', count ? 'And another condition' : 'Add a condition');
        head.value = '';
        head.selected = true;
        head.disabled = true;
        sel.appendChild(head);
        var words2 = {};
        (w.opts.testKinds || []).forEach(function (k) { words2[k.key] = k.phrase; });
        KIND_ORDER.forEach(function (k) {
          var ok = kindAvailable(k, w.opts);
          var op = node('option', '', (words2[k] || k) + (ok ? '' : ' (none yet)'));
          op.value = k;
          op.disabled = !ok;
          sel.appendChild(op);
        });
        sel.addEventListener('change', function () {
          var kind = sel.value;
          sel.value = '';
          var t = newTest(kind, w.ctx, w.opts);
          if (!t) return;
          if (t._draft) { w.draft = t; drawWhen(sec, { key: 'new' }); return; }
          saveWhen(sec, w.tests.concat([t]), w.tests.length, { key: 'new' });
        });
        box.insertBefore(sel, box.firstChild);
      }

      function drawNow(sec) {
        var w = sec.__w, box = $('[data-dkw-now]', sec);
        box.textContent = '';
        var line = nowLine(w.tests, w.state);
        if (!line) return;
        box.appendChild(node('span', 'dkw-nowline', line));
        if (!hasSay(w.tests)) return;
        var said = !!(w.state && w.state.saidAt);
        if (said) box.appendChild(node('span', 'dkw-nowline', ' You said so.'));
        var b = node('button', 'dkw-word', said ? 'Take it back' : 'Say so');
        b.type = 'button';
        b.setAttribute('data-act', 'when-say');
        b.addEventListener('click', function () { say2(sec, !said); });
        box.appendChild(b);
      }

      function adoptCard(sec, deck, id) {
        var w = sec.__w;
        var c = ((deck && deck.cards) || []).filter(function (x) { return x.id === id; })[0];
        if (!c) return;
        w.tests = c.when || [];
        w.state = { fitsNow: !!c.fitsNow, whyNot: c.whyNot || '', saidAt: c.saidAt || null };
      }

      /** commit: one row changed; a draft row becomes a test once it has its name. */
      function commit(sec, i, n, key) {
        var w = sec.__w;
        if (i >= w.tests.length) {
          if (n._draft) { w.draft = n; drawWhen(sec, { row: i, key: key }); return; }
          w.draft = null;
          saveWhen(sec, w.tests.concat([n]), w.tests.length, { row: i, key: key });
          return;
        }
        var next = w.tests.slice();
        next[i] = n;
        saveWhen(sec, next, i, { row: i, key: key });
      }

      function removeAt(sec, i) {
        var w = sec.__w;
        if (i >= w.tests.length) { w.draft = null; drawWhen(sec, { key: 'add' }); return; }
        saveWhen(sec, w.tests.filter(function (t, j) { return j !== i; }), null, { key: 'add' });
      }

      /** saveWhen: the whole list goes in one card update; the server's check is the only check. */
      function saveWhen(sec, next, idx, focus) {
        var ed = editor();
        if (!ed) return Promise.resolve();
        var id = ed.getAttribute('data-card-id');
        lock(sec);
        return api('PUT', cardURL(id), { when: whenBody(next) }).then(function (deck) {
          setVersion(deck);
          clearErr();
          saved();
          adoptCard(sec, deck, id);
          drawWhen(sec, focus);
          return refreshParts(base + 'workshop/' + did + '/cards?card=' + encodeURIComponent(id), ['#dkw-grid', '#dkw-preview']).then(function () { return refreshTable(); });
        }, function (e) {
          if (isStale(e && e.message)) return stale();
          // Back to what is stored, with the refusal in the row that caused it.
          drawWhen(sec, focus);
          rowSay(sec, idx, words(e));
        });
      }

      function say2(sec, said) {
        var ed = editor();
        if (!ed) return;
        var id = ed.getAttribute('data-card-id');
        lock(sec);
        api('POST', cardURL(id) + '/say', { said: said }).then(function (deck) {
          adoptCard(sec, deck, id);
          drawWhen(sec, null);
          var b = $('[data-act=when-say]', sec);
          if (b) b.focus({ preventScroll: true });
          return refreshParts(base + 'workshop/' + did + '/cards?card=' + encodeURIComponent(id), ['#dkw-grid']);
        }, function (e) {
          drawWhen(sec, null);
          sectionSay(sec, words(e));
          var b = $('[data-act=when-say]', sec);
          if (b) b.focus({ preventScroll: true });
        });
      }

      // ---- events ----
      // Enter in a field saves nothing by itself (each field saves as it changes), and never sends the form.
      el.addEventListener('submit', function (ev) { ev.preventDefault(); });
      el.addEventListener('click', function (ev) {
        // The condition rows act through their own handlers.
        if (ev.target.closest('[data-dkw-when]')) return;
        var tab = ev.target.closest('.dkw-tab');
        if (tab) { showTab(tab.getAttribute('data-tab')); return; }
        var link = ev.target.closest('.dkw-cardlink');
        if (link) {
          if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.button !== 0) return;
          ev.preventDefault();
          openCard(link.getAttribute('data-card'));
          return;
        }
        var opt = ev.target.closest('.dkw-opt');
        if (opt) { choose(opt); return; }
        var ko = ev.target.closest('.dkw-kopt');
        if (ko) {
          var kg = ko.closest('[role=radiogroup]'), kr = ko.closest('.dkw-kindrow');
          var was = $('[role=radio][aria-checked=true]', kg);
          if (was === ko) return;
          check(kg, ko);
          var which = kg.hasAttribute('data-kind-colour') ? 'data-kind-colour' : 'data-kind-sign';
          saveKinds(kindsFromDom(), '[data-kind-key="' + kr.getAttribute('data-kind-key') + '"] [' + which + '] [aria-checked=true]');
          return;
        }
        var tg = ev.target.closest('button.toggle[data-field]');
        if (tg && editor()) {
          var was = tg.getAttribute('aria-checked') === 'true';
          var flip = function (on) { tg.setAttribute('aria-checked', on ? 'true' : 'false'); };
          var body = {};
          body[tg.getAttribute('data-field')] = !was;
          flip(!was);
          saveCard(body, null, function () { flip(was); }).then(function () { tg.focus({ preventScroll: true }); });
          return;
        }
        var r = ev.target.closest('[role=radio][data-kind-key]');
        if (r && editor()) {
          var grp = r.closest('[role=radiogroup]');
          var prior = $('[aria-checked=true]', grp);
          check(grp, r);
          saveCard({ kindKey: r.getAttribute('data-kind-key') }, null, function () { if (prior) check(grp, prior); });
          return;
        }
        var btn = ev.target.closest('[data-act]');
        if (!btn || btn.tagName === 'SELECT') return;
        var act = btn.getAttribute('data-act');
        switch (act) {
          case 'count-down': case 'count-up': {
            var max = parseInt(table().getAttribute('data-maxdraw'), 10);
            count = stepCount(count, act === 'count-up' ? 1 : -1, isNaN(max) ? DRAW_MAX : max);
            $('[data-dkw-count]', table()).textContent = String(count);
            break;
          }
          case 'draw': draw(btn); break;
          case 'deal-table': verb(btn, 'deal', { to: 'table' }, 'deal', MOVE_AT.deal); break;
          case 'deal-party': verb(btn, 'deal', { to: 'hand' }, 'deal', MOVE_AT.deal); break;
          case 'put-back': verb(btn, 'put-back', {}, 'draw', MOVE_AT.back, null, '[data-act=draw]'); break;
          case 'another': verb(btn, 'another', {}, 'draw', MOVE_AT.back, null, '[data-act=draw]'); break;
          case 'take-off': verb(btn, 'take-off', {}, 'used', MOVE_AT.used); break;
          case 'approve': verb(btn, 'approve', {}, 'play', MOVE_AT.deal, null, '[data-act=draw]'); break;
          case 'turn-down': verb(btn, 'turn-down', {}, 'ready', MOVE_AT.deal, null, '[data-act=draw]'); break;
          case 'shuffle': {
            btn.disabled = true;
            api('POST', deckURL + '/shuffle', {}).then(function () {
              clearErr();
              var cards = $$('[data-dkw-used] [data-dk-card]', table());
              var DC = window.DeckCards;
              return DC && DC.shuffle ? DC.shuffle(cards) : null;
            }).then(function () { return refreshTable(); }).then(function () { focusAfter('[data-act=draw]'); }, function (e) {
              btn.disabled = false; failed(e, table(), null); btn.focus();
            });
            break;
          }
          case 'nudge-clear': nudge(btn, ''); break;
          case 'add-card': addCard(btn); break;
          case 'close-card': showTab('cards').then(function () { focusAfter('[data-act=add-card]'); }); break;
          case 'copies-down': case 'copies-up': {
            var out = $('[data-dkw-copies]', panel);
            var cur = parseInt(out.textContent, 10) || 1;
            var next = Math.max(1, Math.min(COPIES_MAX, cur + (act === 'copies-up' ? 1 : -1)));
            if (next === cur) break;
            out.textContent = String(next);
            saveCard({ copies: next }, null, function () { out.textContent = String(cur); }).then(function () {
              var o = $('[data-dkw-copies]', panel);
              var d = $('[data-act=copies-down]', panel), u = $('[data-act=copies-up]', panel);
              if (o && d && u) { var n = parseInt(o.textContent, 10); d.disabled = n <= 1; u.disabled = n >= COPIES_MAX; }
            });
            break;
          }
          case 'remove-picture': saveCard({ picture: '' }).then(function () { var id = editor() && editor().getAttribute('data-card-id'); if (id) return openCard(id, '[data-dkw-pick]'); }); break;
          case 'delete-card': {
            var ed = editor();
            var name = btn.getAttribute('data-name');
            askDelete(ed, {
              ask: 'Delete “' + name + '”?',
              detail: 'Every copy of it leaves the deck, wherever it is, including any a hero is holding. This cannot be undone.',
              discard: 'Delete the card',
              onDiscard: function () {
                api('DELETE', cardURL(ed.getAttribute('data-card-id'))).then(function (deck) {
                  setVersion(deck); saved();
                  return Promise.all([showTab('cards'), refreshTable()]);
                }).then(function () { focusAfter('[data-act=add-card]'); }, function (e) { failed(e, ed, null); });
              }
            });
            break;
          }
          case 'delete-deck': {
            var n = btn.getAttribute('data-copies');
            askDelete($('.dkw-foot', el), {
              ask: 'Delete this deck?',
              detail: 'Its cards (' + n + ' in all) go, and so does anything players are holding from it. This cannot be undone.',
              discard: 'Delete the deck',
              onDiscard: function () {
                api('DELETE', deckURL).then(function () {
                  location.assign(base + 'workshop');
                }, function (e) { failed(e, $('.dkw-foot', el), '[data-act=delete-deck]'); });
              }
            });
            break;
          }
          case 'play-turn': playTurn(); break;
          case 'show-all': toggleAll(btn); break;
          case 'show-kind': reloadLook('[data-act=show-kind][data-kind-key="' + btn.getAttribute('data-kind-key') + '"]', '?kind=' + encodeURIComponent(btn.getAttribute('data-kind-key'))); break;
          case 'apply-style': applyStyle(btn); break;
          case 'toggle-kinds': {
            var kp = $('[data-dkw-kinds]', panel);
            kindsOpen = kp.hidden;
            kp.hidden = !kindsOpen;
            btn.setAttribute('aria-expanded', kindsOpen ? 'true' : 'false');
            if (kindsOpen) { var f = $('input', kp); if (f) f.focus(); }
            break;
          }
          case 'add-kind': {
            var kinds = kindsFromDom();
            if (kinds.length >= MAX_KINDS) break;
            var nm = nextKindName(kinds.map(function (k) { return k.name; }));
            kinds.push({ key: kindKey(nm, kinds.map(function (k) { return k.key; })), name: nm, colour: PALETTE[kinds.length % PALETTE.length], sign: '' });
            saveKinds(kinds, '[data-act=add-kind]');
            break;
          }
          case 'remove-kind': {
            var key = btn.closest('.dkw-kindrow').getAttribute('data-kind-key');
            saveKinds(kindsFromDom().filter(function (k) { return k.key !== key; }), '[data-act=add-kind]');
            break;
          }
          default: break;
        }
      });

      el.addEventListener('change', function (ev) {
        var t = ev.target;
        if (t.closest && t.closest('[data-dkw-when]')) return;
        var act = t.getAttribute && t.getAttribute('data-act');
        if (act === 'deal-hero') {
          if (!t.value) return;
          var hero = t.value;
          t.value = '';
          verb(t, 'deal', { to: 'hand', heroId: hero }, 'deal', MOVE_AT.deal);
          return;
        }
        if (act === 'keep') {
          if (!t.value) return;
          var target = t.value;
          t.value = '';
          verb(t, 'keep', { targetDeckId: target }, 'deal', MOVE_AT.deal);
          return;
        }
        if (act === 'nudge') { nudge(t, t.value); return; }
        if (t.hasAttribute('data-rule')) { saveRule(t); return; }
        if (t.hasAttribute('data-dkw-name')) {
          var nm = t.value.trim();
          if (!nm) { t.value = t.defaultValue; say(el, 'A deck needs a name.'); return; }
          put({ name: nm }).then(function () { t.defaultValue = nm; clearErr(); saved(); document.title = nm; }, function (e) { t.value = t.defaultValue; failed(e, el, null); t.focus(); });
          return;
        }
        if (t.hasAttribute('data-chosen') || t.hasAttribute('data-left')) { savePlayers(t); return; }
        if (t.hasAttribute('data-back-words')) {
          put({ look: { back: { words: t.value.trim() } } }).then(function () { clearErr(); saved(); return refreshParts(base + 'workshop/' + did + '/look', ['.dkw-stagecol']); }, function (e) { failed(e, lookRoot(), null); });
          return;
        }
        if (t.closest && t.closest('[data-dkw-kindlist]')) { saveKinds(kindsFromDom(), null); return; }
        var field = t.getAttribute && t.getAttribute('data-field');
        if (field && editor()) {
          var patch = {}, prev;
          if (t.type === 'checkbox') { patch[field] = t.checked; prev = !t.checked; }
          else { patch[field] = t.value; prev = t.defaultValue; }
          saveCard(patch, null, function () { if (t.type === 'checkbox') t.checked = prev; else t.value = prev; }).then(function () { if (t.type !== 'checkbox') t.defaultValue = t.value; });
        }
      });

      // The campaign's picture picker fires this on its button; the card keeps the id.
      this._onPick = function (ev) {
        var btn = ev.target;
        if (!btn || !btn.hasAttribute || !btn.hasAttribute('data-dkw-pick') || !editor()) return;
        var id = ev.detail && ev.detail.id;
        if (!id) return;
        saveCard({ picture: id }, null).then(function () { var cur = editor() && editor().getAttribute('data-card-id'); if (cur) return openCard(cur, '[data-dkw-pick]'); });
      };
      document.addEventListener('media-picker:select', this._onPick);
    },

    destroy: function () {
      versionSink = null;
      if (this._onPick) document.removeEventListener('media-picker:select', this._onPick);
    }
  });

  window.DeckWorkshop = {
    LISTS: LISTS, PALETTE: PALETTE, DRAW_MIN: DRAW_MIN, DRAW_MAX: DRAW_MAX, COPIES_MAX: COPIES_MAX, MAX_KINDS: MAX_KINDS,
    clampCount: clampCount, stepCount: stepCount, lookPatch: lookPatch, kindKey: kindKey, nextKindName: nextKindName, isStale: isStale, words: words, parseVersion: parseVersion,
    MAX_TESTS: MAX_TESTS, KIND_ORDER: KIND_ORDER, TEST_KEYS: TEST_KEYS, whenBody: whenBody, newTest: newTest, changeTest: changeTest, parseNumber: parseNumber,
    whenHint: whenHint, nowLine: nowLine, kindAvailable: kindAvailable, shareLabel: shareLabel, buildRow: buildRow
  };
})();
