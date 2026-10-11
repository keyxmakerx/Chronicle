/**
 * rulebook-editor.js -- the Rulebook page editor.
 *
 * Lets a campaign's Directors edit their edition of a game system's book: a
 * contents tree, a form for the open page and the page itself drawn live by
 * the book's own renderer (window.ChronicleRulebook from rulebook.js). Every
 * edit is saved through the book editor API after a short pause, one page at
 * a time. The data model and routes are described in the book editor
 * contract (docs/system-rulebook-book.md).
 *
 * Every string from the server is untrusted: it is escaped before it is put
 * in markup, and the form is built once per structural change, never per
 * keystroke, so focus stays in the field being typed into.
 *
 * Mount: <div data-widget="rulebook-editor" data-campaign-id data-system-id
 *             data-source-url data-done-url>
 */
(function () {
  'use strict';

  var SAVE_MS = 600;
  var instances = new WeakMap();
  var uid = 0;

  var TYPES = {
    text: { name: 'Text', hint: 'Paragraphs. Blank line starts a new one.' },
    callout: { name: 'Highlighted box', hint: 'A rule worth remembering.' },
    roll: { name: 'Dice roll', hint: 'Readers roll; the result band lights up.' },
    cards: { name: 'Fold-out cards', hint: 'A row of cards that open for detail.' },
    flaps: { name: 'Flaps', hint: 'Headings that open to explain.' },
    example: { name: 'Worked example', hint: 'Stepped through one line at a time.' },
    creature: { name: 'Creature', hint: 'What heroes see; numbers for Directors.' },
    note: { name: 'Director\'s note', hint: 'Only Directors ever see this.' },
    links: { name: 'Chapter links', hint: 'Buttons that turn to other chapters.' }
  };
  var PALETTE = ['text', 'callout', 'roll', 'cards', 'flaps', 'example', 'creature', 'note'];
  var WIDGET_TYPE = { name: 'Widget', hint: 'Part of the book that comes from the game system.' };

  // --- Small helpers ---------------------------------------------------------

  function esc(s) { return Chronicle.escapeHtml(s == null ? '' : String(s)); }
  function str(v) { return typeof v === 'string' ? v : (typeof v === 'number' ? String(v) : ''); }
  function arr(v) { return Array.isArray(v) ? v : []; }
  function isObj(v) { return !!v && typeof v === 'object' && !Array.isArray(v); }
  function clone(v) { return JSON.parse(JSON.stringify(v)); }
  function has(o, k) { return Object.prototype.hasOwnProperty.call(o, k); }

  function safeUrl(u) {
    try {
      var x = new URL(String(u), window.location.href);
      return x.origin === window.location.origin ? x.href : '';
    } catch (e) { return ''; }
  }

  // --- Page model ------------------------------------------------------------
  // The form edits a "model" copy of the authored page (strings everywhere,
  // band limits as text). authored() turns it back into the API shape and
  // wire() into what the book renderer reads.

  function blockModel(raw) {
    var b = isObj(raw) ? clone(raw) : {};
    b.type = str(b.type) || 'text';
    b.director = b.director === true || b.type === 'note';
    b.title = str(b.title);
    b.text = str(b.text);
    switch (b.type) {
      case 'roll':
        b.dice = str(b.dice);
        b.modifier = b.modifier === true;
        b.bands = arr(b.bands).filter(isObj).map(function (x) {
          return { max: typeof x.max === 'number' ? String(x.max) : '', label: str(x.label), text: str(x.text) };
        });
        break;
      case 'cards':
      case 'flaps':
        b.items = arr(b.items).filter(isObj).map(function (it) {
          return { title: str(it.title), summary: str(it.summary), text: str(it.text) };
        });
        break;
      case 'example':
        b.steps = arr(b.steps).map(str);
        break;
      case 'creature':
        b.name = str(b.name); b.tagline = str(b.tagline); b.look = str(b.look); b.note = str(b.note);
        b.notice = arr(b.notice).map(str);
        b.stats = arr(b.stats).filter(isObj).map(function (s) { return { label: str(s.label), value: str(s.value) }; });
        break;
      case 'widget':
        b.widget = str(b.widget);
        break;
      case 'links':
        b.items = arr(b.items).filter(isObj).map(function (it) {
          return { title: str(it.title), summary: str(it.summary), chapter: str(it.chapter) };
        });
        break;
    }
    return b;
  }

  function pageModel(raw) {
    var p = isObj(raw) ? raw : {};
    return { title: str(p.title), director: p.director === true, wide: p.wide === true, columns: p.columns === true, blocks: arr(p.blocks).map(blockModel) };
  }

  function put(o, k, v) { if (v !== '' && v != null) o[k] = v; }

  function authoredBlock(b) {
    var o = {}, t = b.type;
    if (t !== 'text') o.type = t;
    if (b.director && t !== 'note') o.director = true;
    switch (t) {
      case 'text': case 'note': o.text = b.text; break;
      case 'callout': put(o, 'title', b.title); o.text = b.text; break;
      case 'roll':
        put(o, 'dice', b.dice); put(o, 'label', b.label);
        if (b.modifier) o.modifier = true;
        o.bands = b.bands.map(function (x, i) {
          var bd = {};
          if (i < b.bands.length - 1) {
            var m = String(x.max).trim();
            if (/^-?\d+$/.test(m)) bd.max = parseInt(m, 10);
            else if (m !== '') bd.max = m;
          }
          put(bd, 'label', x.label); put(bd, 'text', x.text);
          return bd;
        });
        break;
      case 'cards': case 'flaps':
        o.items = b.items.filter(function (it) { return it.title || it.summary || it.text; }).map(function (it) {
          var r = {};
          put(r, 'title', it.title);
          if (t === 'cards') put(r, 'summary', it.summary);
          put(r, 'text', it.text);
          return r;
        });
        break;
      case 'example':
        put(o, 'title', b.title);
        o.steps = b.steps.filter(function (s) { return s.trim() !== ''; });
        break;
      case 'creature':
        put(o, 'name', b.name); put(o, 'tagline', b.tagline); put(o, 'look', b.look);
        var nt = b.notice.filter(function (s) { return s.trim() !== ''; });
        if (nt.length) o.notice = nt;
        var st = b.stats.filter(function (s) { return s.label || s.value; });
        if (st.length) o.stats = st.map(function (s) { var r = {}; put(r, 'label', s.label); put(r, 'value', s.value); return r; });
        put(o, 'note', b.note);
        break;
      case 'widget': o.widget = b.widget; break;
      case 'links':
        put(o, 'title', b.title);
        o.items = b.items.map(function (it) {
          var r = {};
          put(r, 'title', it.title); put(r, 'summary', it.summary); put(r, 'chapter', it.chapter);
          return r;
        });
        break;
    }
    return o;
  }

  function authoredPage(m) {
    var o = { title: m.title };
    if (m.director) o.director = true;
    if (m.wide) o.wide = true;
    if (m.columns) o.columns = true;
    o.blocks = m.blocks.map(authoredBlock);
    return o;
  }

  function wireBlock(b) {
    var o = { type: b.type, director: b.director || b.type === 'note' };
    switch (b.type) {
      case 'text': case 'note': o.text = b.text; break;
      case 'callout': o.title = b.title; o.text = b.text; break;
      case 'roll':
        var m = /^\s*(\d+)\s*d\s*(\d+)\s*$/i.exec(b.dice);
        o.dice = m ? { count: +m[1], sides: +m[2] } : { count: 1, sides: 6 };
        o.label = str(b.label);
        o.modifier = b.modifier;
        o.bands = b.bands.map(function (x, i) {
          var mx = String(x.max).trim();
          return { max: i < b.bands.length - 1 && /^-?\d+$/.test(mx) ? parseInt(mx, 10) : null, label: x.label, text: x.text };
        });
        break;
      case 'cards': case 'flaps': o.items = b.items; break;
      case 'example': o.title = b.title; o.steps = b.steps.filter(function (s) { return s.trim() !== ''; }); break;
      case 'creature':
        o.name = b.name; o.tagline = b.tagline; o.look = b.look;
        o.notice = b.notice.filter(function (s) { return s.trim() !== ''; });
        o.stats = b.stats.filter(function (s) { return s.label || s.value; });
        o.note = b.note;
        o.hiddenStats = o.stats.length > 0;
        break;
      case 'widget': o.widget = b.widget; break;
      case 'links': o.title = b.title; o.items = b.items; break;
    }
    return o;
  }

  function wirePage(m) {
    return { title: m.title, director: m.director, wide: m.wide, columns: m.columns, blocks: m.blocks.map(wireBlock) };
  }

  function newBlock(t) {
    var b = { type: t, director: t === 'note', title: '', text: '' };
    if (t === 'roll') {
      b.dice = '2d10'; b.modifier = false;
      b.bands = [{ max: '11', label: 'Tier 1', text: '' }, { max: '16', label: 'Tier 2', text: '' }, { max: '', label: 'Tier 3', text: '' }];
    }
    if (t === 'cards' || t === 'flaps') b.items = [{ title: '', summary: '', text: '' }];
    if (t === 'example') b.steps = [''];
    if (t === 'creature') { b.name = ''; b.tagline = ''; b.look = ''; b.note = ''; b.notice = []; b.stats = []; }
    return b;
  }

  function newRow(list) {
    if (list === 'items') return { title: '', summary: '', text: '' };
    if (list === 'stats') return { label: '', value: '' };
    if (list === 'bands') return { max: '', label: '', text: '' };
    return '';
  }

  // Sets a value at a path like "blocks.2.items.0.title" inside a page model.
  // Paths come from the form's own data attributes, but a key that could
  // reach an object's prototype is refused anyway, and only existing own
  // properties are walked.
  var UNSAFE_KEY = { '__proto__': true, 'constructor': true, 'prototype': true };
  function setPath(model, path, value) {
    var keys = String(path).split('.'), o = model, i;
    for (i = 0; i < keys.length; i++) { if (UNSAFE_KEY[keys[i]]) return false; }
    for (i = 0; i < keys.length - 1; i++) {
      if (o == null || typeof o !== 'object' || !Object.prototype.hasOwnProperty.call(o, keys[i])) return false;
      o = o[keys[i]];
    }
    if (o == null || typeof o !== 'object') return false;
    o[keys[keys.length - 1]] = value;
    return true;
  }

  function errorText(err) {
    return err && err.message ? err.message : 'The server could not be reached.';
  }

  // --- The editor ------------------------------------------------------------

  function Editor(el, config) {
    this.el = el;
    this.n = ++uid;
    this.cfg = config || {};
    this.sourceUrl = str(this.cfg.sourceUrl);
    this.base = this.sourceUrl.replace(/\/source\/?(?:\?.*)?$/, '');
    this.doneUrl = str(this.cfg.doneUrl);
    this.src = null;
    this.parts = [];
    this.byId = {};
    this.cur = null;
    this.as = 'dir';
    this.compare = false;
    this.palOpen = false;
    this.confirm = '';
    this.ops = 0;
    this.opError = '';
    this.dead = false;
    this.handlers = [];
    this.nid = 0;
  }

  Editor.prototype.id = function (s) { return 'rbe' + this.n + '-' + s; };
  Editor.prototype.sys = function () { return (this.src && this.src.systemName) || 'the game system'; };
  Editor.prototype.on = function (t, type, fn, opts) { t.addEventListener(type, fn, opts); this.handlers.push([t, type, fn, opts]); };

  Editor.prototype.start = function () {
    var self = this;
    this.el.classList.add('rbe');
    this.on(this.el, 'input', function (e) { self.onInput(e); });
    this.on(this.el, 'change', function (e) { self.onChange(e); });
    this.on(this.el, 'click', function (e) { self.onClick(e); });
    this.on(this.el, 'mousedown', function (e) {
      // Keep the cursor in the text field when a formatting button is pressed.
      if (e.target.closest && e.target.closest('[data-act="fmt"]')) e.preventDefault();
    });
    this.on(this.el, 'keydown', function (e) { self.onKey(e); });
    this.on(window, 'beforeunload', function (e) {
      if (self.unsaved()) { e.preventDefault(); e.returnValue = ''; }
    });
    this.load();
  };

  Editor.prototype.stop = function () {
    this.dead = true;
    this.handlers.forEach(function (h) { h[0].removeEventListener(h[1], h[2], h[3]); });
    this.handlers = [];
    var self = this;
    Object.keys(this.byId).forEach(function (k) { clearTimeout(self.byId[k].timer); });
    this.parts.forEach(function (p) { p.chapters.forEach(function (c) { clearTimeout(c.timer); }); });
    this.el.innerHTML = '';
    this.el.classList.remove('rbe');
    this.el.removeAttribute('aria-busy');
  };

  // --- Talking to the server -------------------------------------------------

  Editor.prototype.call = function (method, url, body) {
    var opts = { method: method };
    if (body !== undefined) opts.body = body;
    return Chronicle.apiFetch(url, opts).then(function (res) {
      return res.json().then(function (j) { return j; }, function () { return {}; }).then(function (j) {
        if (!res.ok) {
          var msg = isObj(j) ? str(j.message) || str(j.error) : '';
          var e = new Error(msg || (res.status === 403 ? 'You do not have access to edit this book.' : 'The server answered with error ' + res.status + '.'));
          e.status = res.status;
          throw e;
        }
        return j;
      });
    }, function () { throw new Error('The server could not be reached.'); });
  };

  Editor.prototype.pageUrl = function (ch, key, tail) {
    return this.base + '/chapters/' + encodeURIComponent(ch.id) + '/pages' + (key ? '/' + encodeURIComponent(key) : '') + (tail || '');
  };

  Editor.prototype.load = function () {
    var self = this;
    if (!this.sourceUrl) { this.showState('error', 'This page has no editor address.'); return; }
    this.showState('loading');
    this.call('GET', this.sourceUrl).then(function (json) {
      if (self.dead) return;
      if (!isObj(json) || !Array.isArray(json.parts)) throw new Error('The book data could not be read.');
      self.open(json);
    }).catch(function (err) {
      if (!self.dead) self.showState('error', errorText(err));
    });
  };

  Editor.prototype.showState = function (kind, message) {
    this.el.innerHTML = kind === 'loading'
      ? '<div class="rbe-state" role="status"><p>Opening the editor…</p></div>'
      : '<div class="rbe-state" role="alert"><p class="rbe-state-title">The editor could not be opened</p><p>' + esc(message) +
        '</p><button type="button" class="rbe-btn rbe-pri" data-act="reload">Retry</button></div>';
    this.el.setAttribute('aria-busy', kind === 'loading' ? 'true' : 'false');
  };

  // --- Building the tree from the server's data ------------------------------

  Editor.prototype.addNode = function (ch, entry) {
    var n = {
      uid: 'n' + (++this.nid), ch: ch, key: str(entry.key), state: str(entry.state) || 'package', problem: str(entry.problem),
      page: pageModel(entry.page), theirs: isObj(entry.theirs) ? pageModel(entry.theirs) : null,
      timer: 0, dirty: false, inflight: null, again: false, error: '', save: Editor.prototype.savePage
    };
    this.byId[n.uid] = n;
    return n;
  };

  Editor.prototype.addChapter = function (part, c) {
    var ch = {
      uid: 'c' + (++this.nid), id: str(c.id), title: str(c.title), intro: str(c.intro), director: c.director === true,
      house: c.house === true, generated: c.generated === true, problem: str(c.problem), part: part, pages: [],
      timer: 0, dirty: false, inflight: null, again: false, error: '', save: Editor.prototype.saveChapter
    };
    var self = this;
    arr(c.pages).filter(isObj).forEach(function (e) { ch.pages.push(self.addNode(ch, e)); });
    part.chapters.push(ch);
    return ch;
  };

  Editor.prototype.open = function (json) {
    var self = this;
    this.src = json;
    this.parts = [];
    this.byId = {};
    arr(json.parts).filter(isObj).forEach(function (p) {
      var part = { title: str(p.title), director: p.director === true, house: p.house === true, chapters: [] };
      arr(p.chapters).filter(isObj).forEach(function (c) { self.addChapter(part, c); });
      self.parts.push(part);
    });
    this.terms = {};
    if (isObj(json.terms)) {
      Object.keys(json.terms).forEach(function (k) {
        var t = json.terms[k];
        if (isObj(t)) self.terms[k.toLowerCase()] = { name: str(t.name) || k, text: str(t.text) };
      });
    }
    this.termKeys = Object.keys(this.terms).sort();
    this.cur = this.firstNode();
    this.el.innerHTML = this.frameHTML();
    this.el.setAttribute('aria-busy', 'false');
    this.el.setAttribute('data-tab', 'form');
    if (window.ChronicleRulebook) window.ChronicleRulebook.applyTheme(this.el, isObj(json.theme) ? json.theme : {});
    var q = this.el.querySelector.bind(this.el);
    this.tocEl = q('.rbe-toc');
    this.bannerEl = q('.rbe-banner-slot');
    this.fieldsEl = q('.rbe-fields');
    this.pvEl = q('.rbe-pv');
    this.savedEl = q('.rbe-saved');
    this.drawAll();
  };

  Editor.prototype.firstNode = function () {
    for (var i = 0; i < this.parts.length; i++) {
      for (var j = 0; j < this.parts[i].chapters.length; j++) {
        if (this.parts[i].chapters[j].pages.length) return this.parts[i].chapters[j].pages[0];
      }
    }
    return null;
  };

  Editor.prototype.frameHTML = function () {
    var d = this.src, sys = str(d.systemName);
    var tab = function (k, label, on) { return '<button type="button" data-act="tab" data-tab="' + k + '" aria-pressed="' + on + '">' + label + '</button>'; };
    return '<header class="rbe-top">' +
      (str(d.mark) ? '<span class="rbe-mk" aria-hidden="true">' + esc(str(d.mark).slice(0, 3)) + '</span>' : '') +
      '<div><h1>' + esc(str(d.title) || 'Rulebook') + '</h1><small>Editing this campaign\'s edition of ' + esc(sys || 'the book') + '</small></div>' +
      '<span class="rbe-sp"></span><span class="rbe-saved" role="status" aria-live="polite"></span>' +
      '<button type="button" class="rbe-btn" data-act="export">Download as package files</button>' +
      '<button type="button" class="rbe-btn rbe-pri" data-act="done">Done</button></header>' +
      '<div class="rbe-seg rbe-tabs" role="group" aria-label="Show">' + tab('toc', 'Contents', false) + tab('form', 'Edit', true) + tab('prev', 'Page', false) + '</div>' +
      '<div class="rbe-panes">' +
      '<nav class="rbe-pane rbe-toc" aria-label="Book contents"></nav>' +
      '<main class="rbe-pane rbe-form"><div class="rbe-banner-slot"></div><div class="rbe-fields"></div></main>' +
      '<section class="rbe-pane rbe-prev" aria-label="Live page"><div class="rbe-ph"><span>The page, as it will look</span><span class="rbe-sp"></span>' +
      '<span class="rbe-seg" role="group" aria-label="Preview as">' +
      '<button type="button" data-act="as" data-as="dir" aria-pressed="true">Director</button>' +
      '<button type="button" data-act="as" data-as="pl" aria-pressed="false">Player</button></span></div>' +
      '<div class="rb rbe-rb"><article class="rb-page rbe-pv"></article></div></section></div>';
  };

  Editor.prototype.drawAll = function () {
    this.drawToc();
    this.drawBanner();
    this.drawFields();
    this.drawPage();
    this.syncSaved();
  };

  // --- Saved indicator -------------------------------------------------------

  Editor.prototype.each = function (fn) {
    var self = this;
    Object.keys(this.byId).forEach(function (k) { fn(self.byId[k]); });
    this.parts.forEach(function (p) { p.chapters.forEach(fn); });
  };

  Editor.prototype.unsaved = function () {
    var u = false;
    this.each(function (o) { if (o.dirty || o.inflight || o.error) u = true; });
    return u || this.ops > 0;
  };

  Editor.prototype.syncSaved = function () {
    if (!this.savedEl) return;
    var busy = this.ops > 0, error = this.opError;
    this.each(function (o) {
      if (o.dirty && !o.error || o.inflight) busy = true;
      if (o.error && !error) error = o.error;
    });
    var kind = error ? 'error' : (busy ? 'saving' : 'saved');
    this.savedEl.setAttribute('data-kind', kind);
    this.savedEl.textContent = kind === 'error' ? 'Couldn\'t save: ' + error : (kind === 'saving' ? 'Saving…' : 'All changes saved');
    if (kind === 'error') {
      var b = document.createElement('button');
      b.type = 'button'; b.className = 'rbe-btn rbe-sm'; b.setAttribute('data-act', 'retry'); b.textContent = 'Try again';
      this.savedEl.appendChild(b);
    }
  };

  // Debounced autosave. `o` is a page node or a house chapter, each carrying
  // a save() that returns a promise. A failed save keeps o.dirty so the next
  // edit (or Try again) sends it again.
  Editor.prototype.touch = function (o) {
    var self = this;
    o.dirty = true;
    o.error = '';
    this.opError = '';
    clearTimeout(o.timer);
    o.timer = setTimeout(function () { self.flush(o); }, SAVE_MS);
    this.syncSaved();
  };

  Editor.prototype.flush = function (o) {
    var self = this;
    clearTimeout(o.timer);
    o.timer = 0;
    if (!o.dirty) return Promise.resolve(!o.error);
    if (o.inflight) { o.again = true; return o.inflight; }
    o.dirty = false;
    o.inflight = o.save.call(self, o).then(function () {
      o.error = '';
    }, function (err) {
      o.dirty = true;
      o.error = errorText(err);
    }).then(function () {
      o.inflight = null;
      var again = o.again;
      o.again = false;
      if (self.dead) return false;
      self.syncSaved();
      self.refreshNode(o);
      return again && !o.error ? self.flush(o) : !o.error;
    });
    return o.inflight;
  };

  Editor.prototype.flushAll = function () {
    var self = this, all = [];
    this.each(function (o) { if (o.dirty || o.inflight) all.push(self.flush(o)); });
    return Promise.all(all).then(function (r) { return r.every(Boolean); });
  };

  // Stops the pending save of o and waits for any in-flight one.
  Editor.prototype.settle = function (o) {
    clearTimeout(o.timer);
    o.timer = 0;
    o.dirty = false;
    o.error = '';
    return o.inflight || Promise.resolve();
  };

  Editor.prototype.savePage = function (n) {
    var self = this;
    return this.call('PUT', this.pageUrl(n.ch, n.key), { page: authoredPage(n.page) }).then(function (j) {
      self.applyEntry(n, j && j.entry, false);
    });
  };

  Editor.prototype.saveChapter = function (ch) {
    return this.call('PUT', this.base + '/chapters/' + encodeURIComponent(ch.id), { title: ch.title });
  };

  // Takes the server's verdict (state, key, the system's version) without
  // replacing the page text, which the person may have changed since. With
  // full=true the page text is replaced too (after a go-back or use-theirs).
  Editor.prototype.applyEntry = function (n, e, full) {
    if (!isObj(e)) return;
    if (str(e.key)) n.key = str(e.key);
    if (str(e.state)) n.state = str(e.state);
    n.problem = str(e.problem);
    n.theirs = isObj(e.theirs) ? pageModel(e.theirs) : null;
    if (full) n.page = pageModel(e.page);
  };

  // An op is a one-off request (go back, keep mine, add, remove).
  Editor.prototype.op = function (fn) {
    var self = this;
    this.ops++;
    this.opError = '';
    this.syncSaved();
    return Promise.resolve().then(fn).then(function (r) {
      self.ops--;
      self.syncSaved();
      return r;
    }, function (err) {
      self.ops--;
      self.opError = errorText(err);
      self.syncSaved();
      return null;
    });
  };

  // --- Contents tree ---------------------------------------------------------

  Editor.prototype.dotClass = function (state) {
    return state === 'edited' ? 'ed' : (state === 'mine' ? 'mine' : (state === 'changed' ? 'warn' : ''));
  };

  Editor.prototype.stateText = function (state) {
    var s = this.sys();
    return state === 'edited' ? 'you changed it' : (state === 'mine' ? 'your own page' : (state === 'changed' ? s + ' changed it too' : s + '\'s page, untouched'));
  };

  Editor.prototype.pageButton = function (n) {
    return '<button type="button" class="rbe-pg" data-act="pick" data-id="' + n.uid + '" id="' + this.id('pg-' + n.uid) + '"' +
      (n === this.cur ? ' aria-current="true"' : '') + '><span class="rbe-dot ' + this.dotClass(n.state) + '" aria-hidden="true"></span>' +
      '<span class="rbe-pgt">' + esc(n.page.title || 'Untitled page') + '</span>' +
      (n.page.director ? ' <span class="rbe-pill rbe-dir">Directors</span>' : '') +
      '<span class="rbe-sr"> (' + esc(this.stateText(n.state)) + ')</span></button>';
  };

  Editor.prototype.drawToc = function () {
    var self = this, h = '', a = document.activeElement, fid = a && a.id && this.tocEl.contains(a) ? a.id : '';
    var s = this.sys();
    // Each part is a section, each chapter a group with its pages on a guide
    // line; "Add page" is a quiet action at the end of the group, never a row
    // that reads like another page.
    this.parts.forEach(function (part) {
      h += '<section class="rbe-part"><h3>' + esc(part.title) + (part.director ? ' <span class="rbe-pill rbe-dir">Directors</span>' : '') +
        (part.house ? ' <span class="rbe-pill rbe-mine">Yours</span>' : '') + '</h3>';
      part.chapters.forEach(function (ch) {
        var count = ch.problem || ch.generated ? '' : '<span class="rbe-cnt" aria-hidden="true">' + ch.pages.length + '</span>';
        h += '<div class="rbe-ch"><div class="rbe-chn"><span class="rbe-cht">' + esc(ch.title) + '</span>' +
          (ch.director && !part.director ? ' <span class="rbe-pill rbe-dir">Directors</span>' : '') + count + '</div><div class="rbe-pgs">';
        if (ch.problem) {
          h += '<p class="rbe-note-line">This chapter could not be loaded: ' + esc(ch.problem) + '</p>';
        } else if (ch.generated) {
          h += '<p class="rbe-note-line">Made from ' + esc(s) + '\'s rules data, so it changes when ' + esc(s) + ' updates. It isn\'t edited here.</p>';
        } else {
          ch.pages.forEach(function (n) { h += self.pageButton(n); });
          h += '<button type="button" class="rbe-add" id="' + self.id('addpg-' + ch.uid) + '" data-act="addpage" data-ch="' + ch.uid + '">' +
            '<span class="rbe-plus" aria-hidden="true">+</span>Add page<span class="rbe-sr"> to ' + esc(ch.title) + '</span></button>';
        }
        h += '</div></div>';
      });
      h += '</section>';
    });
    h += '<button type="button" class="rbe-add rbe-add-ch" id="' + this.id('addch') + '" data-act="addchapter"><span class="rbe-plus" aria-hidden="true">+</span>Add a house-rules chapter</button>';
    h += '<div class="rbe-legend"><span><span class="rbe-dot" aria-hidden="true"></span>' + esc(s) + '\'s page, untouched</span>' +
      '<span><span class="rbe-dot ed" aria-hidden="true"></span>You changed it</span>' +
      '<span><span class="rbe-dot mine" aria-hidden="true"></span>Your own page</span>' +
      '<span><span class="rbe-dot warn" aria-hidden="true"></span>' + esc(s) + ' changed it too</span></div>';
    this.tocEl.innerHTML = h;
    if (fid) this.focusId(fid);
  };

  // Updates one tree row in place (title, dot) without rebuilding the tree.
  Editor.prototype.refreshNode = function (o) {
    if (!this.tocEl || !o.page) return;
    var btn = document.getElementById(this.id('pg-' + o.uid));
    if (!btn) return;
    var dot = btn.querySelector('.rbe-dot'), t = btn.querySelector('.rbe-pgt');
    dot.className = 'rbe-dot ' + this.dotClass(o.state);
    t.textContent = o.page.title || 'Untitled page';
    var sr = btn.querySelector('.rbe-sr');
    if (sr) sr.textContent = ' (' + this.stateText(o.state) + ')';
    var dir = btn.querySelector('.rbe-dir');
    if (o.page.director && !dir) btn.insertBefore(this.pill(), sr);
    else if (!o.page.director && dir) dir.remove();
  };

  Editor.prototype.pill = function () {
    var s = document.createElement('span');
    s.className = 'rbe-pill rbe-dir';
    s.textContent = 'Directors';
    return s;
  };

  Editor.prototype.focusId = function (id) {
    var e = document.getElementById(id);
    if (e && this.el.contains(e) && !e.disabled) e.focus();
  };

  // --- Banner ----------------------------------------------------------------

  Editor.prototype.bannerHTML = function () {
    var n = this.cur, s = esc(this.sys());
    if (!n) return '';
    var btn = function (act, label, extra) { return '<button type="button" class="rbe-btn rbe-sm" data-act="' + act + '"' + (extra || '') + '>' + label + '</button>'; };
    if (n.problem && !(n.state === 'mine' && this.confirm === 'rmpage')) {
      return '<div class="rbe-banner warn" role="alert"><p><b>This page cannot be edited yet.</b> ' + esc(n.problem) + '</p>' +
        (n.state === 'mine' ? btn('rmpage', 'Remove this page') : btn('reset', 'Go back to ' + s + '\'s page')) + '</div>';
    }
    if (n.state === 'mine') {
      if (this.confirm === 'rmpage') {
        return '<div class="rbe-banner mine" role="group" aria-label="Remove this page"><p>Remove this page for good? It cannot be brought back.</p>' +
          btn('rmpage-yes', 'Yes, remove it') + btn('rmpage-no', 'Keep it') + '</div>';
      }
      return '<div class="rbe-banner mine"><p>This page is yours. ' + s + ' updates never touch it.</p>' + btn('rmpage', 'Remove this page') + '</div>';
    }
    if (n.state === 'package') {
      return '<div class="rbe-banner"><p>This is ' + s + '\'s page. Change anything and this campaign keeps its own copy; the original stays one click away.</p></div>';
    }
    if (n.state === 'changed') {
      if (!n.theirs) {
        return '<div class="rbe-banner warn"><p><b>' + s + ' no longer has a page here.</b> Your version is still the one players see.</p>' + btn('keep', 'Keep mine') + '</div>';
      }
      return '<div class="rbe-banner warn"><p><b>' + s + ' changed this page after you edited it.</b> Your version is still the one players see.</p>' +
        btn('compare', this.compare ? 'Hide comparison' : 'Compare', ' aria-expanded="' + this.compare + '"') + btn('keep', 'Keep mine') + btn('theirs', 'Use ' + s + '\'s') + '</div>' +
        (this.compare ? '<div class="rbe-cmp" aria-label="Your page next to ' + s + '\'s"><div><span class="rbe-lab">Yours</span><div class="rb rbe-rb"><article class="rb-page rbe-pv rbe-cmp-a"></article></div></div>' +
          '<div><span class="rbe-lab">' + s + '\'s page now</span><div class="rb rbe-rb"><article class="rb-page rbe-pv rbe-cmp-b"></article></div></div></div>' : '');
    }
    return '<div class="rbe-banner ed"><p>You changed this page. Players see your version.</p>' + btn('reset', 'Go back to ' + s + '\'s page') + '</div>';
  };

  Editor.prototype.drawBanner = function () {
    var a = document.activeElement, act = a && this.bannerEl.contains(a) ? a.getAttribute('data-act') : '';
    this.bannerEl.innerHTML = this.bannerHTML();
    if (act) {
      var b = this.bannerEl.querySelector('[data-act="' + act + '"]') || this.bannerEl.querySelector('button');
      if (b) b.focus();
    }
    this.drawCompare();
  };

  Editor.prototype.drawCompare = function () {
    var a = this.bannerEl.querySelector('.rbe-cmp-a'), b = this.bannerEl.querySelector('.rbe-cmp-b'), n = this.cur;
    if (!a || !b || !n || !n.theirs) return;
    a.innerHTML = this.render(n, n.page, true);
    b.innerHTML = this.render(n, n.theirs, true);
    this.tidyPreview(a); this.tidyPreview(b);
  };

  // --- The form --------------------------------------------------------------

  Editor.prototype.field = function (path, label, val, extra) {
    var id = this.id(path.replace(/\./g, '-'));
    return '<div class="rbe-field"><label for="' + id + '">' + esc(label) + '</label>' +
      '<input class="rbe-in" id="' + id + '" data-path="' + path + '" value="' + esc(val) + '"' + (extra || '') + '></div>';
  };

  Editor.prototype.area = function (path, label, val, bi) {
    var id = this.id(path.replace(/\./g, '-'));
    var tool = bi === undefined ? '' :
      '<div class="rbe-tool" role="group" aria-label="Text formatting">' +
      '<button type="button" data-act="fmt" data-f="b" data-i="' + bi + '">Bold</button>' +
      '<button type="button" data-act="fmt" data-f="i" data-i="' + bi + '">Italic</button>' +
      '<button type="button" data-act="fmt" data-f="t" data-i="' + bi + '" aria-expanded="false">Link a rule word</button></div>';
    return '<div class="rbe-field"><label for="' + id + '">' + esc(label) + '</label>' + tool +
      '<textarea class="rbe-in" id="' + id + '" data-path="' + path + '">' + esc(val) + '</textarea></div>';
  };

  // A repeating group of inputs (items, steps, notice lines, stats, bands).
  Editor.prototype.rowTools = function (bi, list, j, word, count) {
    return '<button type="button" class="rbe-ib" data-act="rmrow" data-i="' + bi + '" data-list="' + list + '" data-j="' + j +
      '" aria-label="Remove ' + word + ' ' + (j + 1) + '"' + (count < 2 && list === 'bands' ? ' disabled' : '') + '>✕</button>';
  };

  Editor.prototype.addRow = function (bi, list, label) {
    return '<button type="button" class="rbe-btn rbe-sm" id="' + this.id('add-' + bi + '-' + list) + '" data-act="addrow" data-i="' + bi + '" data-list="' + list + '">' + esc(label) + '</button>';
  };

  Editor.prototype.itemsForm = function (b, bi, cards) {
    var self = this, h = '<div class="rbe-items">';
    b.items.forEach(function (it, j) {
      var p = 'blocks.' + bi + '.items.' + j + '.';
      h += '<div class="rbe-item"><div class="rbe-row rbe-row-tight"><span class="rbe-lab">Item ' + (j + 1) + '</span><span class="rbe-sp"></span>' + self.rowTools(bi, 'items', j, 'item', 2) + '</div>' +
        self.plain(p + 'title', 'Heading for item ' + (j + 1), it.title, 'Heading') +
        (cards ? self.plain(p + 'summary', 'Short line on the card for item ' + (j + 1), it.summary, 'A short line on the card') : '') +
        self.plain(p + 'text', 'Text for item ' + (j + 1), it.text, 'What it says') + '</div>';
    });
    return h + this.addRow(bi, 'items', '+ Add an item') + '</div>';
  };

  Editor.prototype.plain = function (path, aria, val, placeholder) {
    return '<input class="rbe-in" id="' + this.id(path.replace(/\./g, '-')) + '" data-path="' + path + '" value="' + esc(val) +
      '" aria-label="' + esc(aria) + '" placeholder="' + esc(placeholder || '') + '">';
  };

  Editor.prototype.listForm = function (b, bi, list, heading, word, placeholder) {
    var self = this, h = '<div class="rbe-field"><span class="rbe-lab">' + esc(heading) + '</span><div class="rbe-items">';
    b[list].forEach(function (v, j) {
      h += '<div class="rbe-item rbe-line">' + self.plain('blocks.' + bi + '.' + list + '.' + j, word + ' ' + (j + 1), v, placeholder) + self.rowTools(bi, list, j, word.toLowerCase(), 2) + '</div>';
    });
    return h + this.addRow(bi, list, '+ Add ' + (list === 'notice' ? 'a line' : 'an item')) + '</div></div>';
  };

  Editor.prototype.blockForm = function (b, i, count) {
    var self = this, t = TYPES[b.type] || WIDGET_TYPE, p = 'blocks.' + i + '.', f = this.field.bind(this);
    var h = '<section class="rbe-blk' + (b.director ? ' isdir' : '') + '" aria-label="' + esc(t.name) + '" data-i="' + i + '"><div class="rbe-bh"><span class="rbe-bname"><span class="rbe-ty">' + esc(t.name) + '</span><span class="rbe-hint">' + esc(t.hint) + '</span></span><span class="rbe-bctl">';
    if (b.type !== 'note' && b.type !== 'widget') {
      h += '<label class="rbe-tog"><input type="checkbox" data-bdir="' + i + '"' + (b.director ? ' checked' : '') + '> Directors only</label>';
    }
    h += '<button type="button" class="rbe-ib" id="' + this.id('mv-' + i + '-up') + '" data-act="mv" data-i="' + i + '" data-d="-1" aria-label="Move up"' + (i === 0 ? ' disabled' : '') + '>↑</button>' +
      '<button type="button" class="rbe-ib" id="' + this.id('mv-' + i + '-down') + '" data-act="mv" data-i="' + i + '" data-d="1" aria-label="Move down"' + (i === count - 1 ? ' disabled' : '') + '>↓</button>' +
      '<button type="button" class="rbe-ib" data-act="rm" data-i="' + i + '" aria-label="Remove this block">✕</button></span></div><div class="rbe-bb">';
    switch (b.type) {
      case 'text': h += this.area(p + 'text', 'Text', b.text, i); break;
      case 'note': h += this.area(p + 'text', 'Note', b.text, i); break;
      case 'callout': h += f(p + 'title', 'Heading', b.title) + this.area(p + 'text', 'Text', b.text, i); break;
      case 'roll':
        h += f(p + 'dice', 'Dice', b.dice, ' style="max-width:120px"') +
          '<label class="rbe-tog rbe-tog-field"><input type="checkbox" data-bmod="' + i + '"' + (b.modifier ? ' checked' : '') + '> Readers can add a bonus</label>' +
          '<span class="rbe-lab">Result bands, lowest first</span><div class="rbe-items">';
        b.bands.forEach(function (bd, j) {
          var last = j === b.bands.length - 1, bp = p + 'bands.' + j + '.';
          h += '<div class="rbe-item rbe-band"><input class="rbe-in" id="' + self.id(bp.replace(/\./g, '-') + 'max') + '" data-path="' + bp + 'max" value="' + esc(last ? '' : bd.max) +
            '" aria-label="Highest total for band ' + (j + 1) + '" placeholder="' + (last ? 'and above' : 'up to') + '"' + (last ? ' disabled' : '') + '>' +
            '<input class="rbe-in" id="' + self.id(bp.replace(/\./g, '-') + 'label') + '" data-path="' + bp + 'label" value="' + esc(bd.label) + '" aria-label="Name of band ' + (j + 1) + '">' +
            self.rowTools(i, 'bands', j, 'band', b.bands.length) +
            '<span></span><input class="rbe-in" id="' + self.id(bp.replace(/\./g, '-') + 'text') + '" data-path="' + bp + 'text" value="' + esc(bd.text) + '" aria-label="What band ' + (j + 1) + ' means"></div>';
        });
        h += this.addRow(i, 'bands', '+ Add a band') + '</div>';
        break;
      case 'cards': h += this.itemsForm(b, i, true); break;
      case 'flaps': h += this.itemsForm(b, i, false); break;
      case 'example':
        h += f(p + 'title', 'Heading', b.title);
        h += '<div class="rbe-items">';
        b.steps.forEach(function (st, j) {
          h += '<div class="rbe-item"><div class="rbe-row rbe-row-tight"><span class="rbe-lab">Item ' + (j + 1) + '</span><span class="rbe-sp"></span>' + self.rowTools(i, 'steps', j, 'item', 2) + '</div>' +
            self.plain(p + 'steps.' + j, 'Text for item ' + (j + 1), st, 'What happens in this step') + '</div>';
        });
        h += this.addRow(i, 'steps', '+ Add an item') + '</div>';
        break;
      case 'creature':
        h += f(p + 'name', 'Name', b.name) + f(p + 'tagline', 'Tagline', b.tagline) + this.area(p + 'look', 'What heroes see', b.look) +
          this.listForm(b, i, 'notice', 'What heroes notice', 'Line', 'Something a hero would spot') +
          '<div class="rbe-field"><span class="rbe-lab">Numbers (Directors only)</span><div class="rbe-items">';
        b.stats.forEach(function (s, j) {
          var sp = p + 'stats.' + j + '.';
          h += '<div class="rbe-item rbe-line">' + self.plain(sp + 'label', 'Name of number ' + (j + 1), s.label, 'Stamina') + self.plain(sp + 'value', 'Value of number ' + (j + 1), s.value, '15') + self.rowTools(i, 'stats', j, 'number', 2) + '</div>';
        });
        h += this.addRow(i, 'stats', '+ Add a number') + '</div></div>' + this.area(p + 'note', 'Note (Directors only)', b.note);
        break;
      case 'widget':
        h += '<p class="rbe-widget">This part of the page is the <b>' + esc(b.widget || 'unnamed') + '</b> widget from ' + esc(this.sys()) + '. It can be moved or removed here, but not edited.</p>';
        break;
      case 'links':
        h += '<p class="rbe-widget">Turns to: ' + b.items.map(function (it) { return '<b>' + esc(it.title) + '</b>'; }).join(', ') +
          '. These buttons can be moved or removed here, but not edited.</p>';
        break;
    }
    return h + '</div></section>';
  };

  Editor.prototype.fieldsHTML = function () {
    var n = this.cur, h = '';
    if (!n) {
      return '<p class="rbe-empty">This book has no pages yet. Use “+ Add a house-rules chapter” in the contents to start one.</p>';
    }
    var ch = n.ch, m = n.page;
    if (n.problem) return '<p class="rbe-empty">Fix or reset this page to edit it.</p>';
    if (ch.house) {
      h += '<div class="rbe-chapter"><div class="rbe-field"><label for="' + this.id('chtitle') + '">Chapter title</label>' +
        '<input class="rbe-in" id="' + this.id('chtitle') + '" data-chtitle="' + ch.uid + '" value="' + esc(ch.title) + '"></div>';
      if (this.confirm === 'rmch') {
        h += '<div class="rbe-banner warn" role="group" aria-label="Remove this chapter"><p>Remove this chapter and its ' + ch.pages.length + (ch.pages.length === 1 ? ' page' : ' pages') +
          ' for good?</p><button type="button" class="rbe-btn rbe-sm" id="' + this.id('rmch-yes') + '" data-act="rmch-yes">Yes, remove it</button><button type="button" class="rbe-btn rbe-sm" data-act="rmch-no">Keep it</button></div>';
      } else {
        h += '<button type="button" class="rbe-btn rbe-sm" id="' + this.id('rmch') + '" data-act="rmch">Remove this chapter</button>';
      }
      h += '</div>';
    }
    h += '<section class="rbe-sec" aria-labelledby="' + this.id('sec-page') + '"><h2 class="rbe-sech" id="' + this.id('sec-page') + '">Page</h2>' +
      this.field('title', 'Title', m.title) +
      '<div class="rbe-row rbe-row-tight"><label class="rbe-tog"><input type="checkbox" data-pdir> Only Directors see this page</label>' +
      '<label class="rbe-tog"><input type="checkbox" data-pwide> Spread across both pages</label></div></section>';
    var self = this, nb = m.blocks.length;
    h += '<section class="rbe-sec rbe-sec-blocks" aria-labelledby="' + this.id('sec-blk') + '"><h2 class="rbe-sech" id="' + this.id('sec-blk') + '">Blocks' +
      '<span class="rbe-cnt">' + nb + '</span><span class="rbe-sech-hint">Top to bottom, as they appear on the page</span></h2>';
    m.blocks.forEach(function (b, i) { h += self.blockForm(b, i, nb); });
    h += '<button type="button" class="rbe-addblk" id="' + this.id('addblk') + '" data-act="addblk" aria-expanded="' + this.palOpen + '"><span class="rbe-plus" aria-hidden="true">+</span>Add a block</button>';
    if (this.palOpen) {
      h += '<div class="rbe-palette" role="group" aria-label="Block types">' + PALETTE.map(function (k) {
        return '<button type="button" data-act="new" data-type="' + k + '"><b>' + esc(TYPES[k].name) + '</b><span>' + esc(TYPES[k].hint) + '</span></button>';
      }).join('') + '</div>';
    }
    return h + '</section>';
  };

  // Rebuilds the form. Only for structural changes (never while typing); the
  // control that had focus gets it back, or `focus` names another.
  Editor.prototype.drawFields = function (focus) {
    var a = document.activeElement, fid = focus || (a && a.id && this.fieldsEl.contains(a) ? a.id : '');
    this.fieldsEl.innerHTML = this.fieldsHTML();
    var n = this.cur;
    if (n) {
      var pd = this.fieldsEl.querySelector('[data-pdir]'), pw = this.fieldsEl.querySelector('[data-pwide]');
      if (pd) pd.checked = n.page.director;
      if (pw) pw.checked = n.page.wide;
    }
    if (fid) this.focusId(fid);
  };

  // --- The live page ---------------------------------------------------------

  Editor.prototype.render = function (n, model, asDirector) {
    var dir = asDirector || this.as === 'dir';
    if (!window.ChronicleRulebook) return '<p class="rbe-empty">The book renderer is not loaded, so the page cannot be shown.</p>';
    if (!dir && (model.director || n.ch.director || n.ch.part.director)) {
      return '<p class="rb-kicker">Player view</p><h2>Players never see this page</h2><div class="rb-prose"><p>It is for Directors only, so it is left out of the book players get.</p></div>';
    }
    var html = window.ChronicleRulebook.renderPage(wirePage(model), { terms: this.src.terms, director: dir, chapterTitle: n.ch.title });
    var seen = dir || model.blocks.some(function (b) { return !b.director && b.type !== 'note'; });
    if (!seen && model.blocks.length) html += '<div class="rb-prose"><p class="rbe-empty">Every block on this page is for Directors, so players see an empty page.</p></div>';
    else if (!model.blocks.length) html += '<div class="rb-prose"><p class="rbe-empty">Add a block to start this page.</p></div>';
    return html;
  };

  // The preview is read-only: hover text on rule words, no stray tab stops.
  Editor.prototype.tidyPreview = function (root) {
    var self = this;
    [].forEach.call(root.querySelectorAll('.rb-term'), function (t) {
      var k = t.getAttribute('data-term'), def = k && has(self.terms, k) ? self.terms[k] : null;
      if (def && def.text) t.setAttribute('title', def.text);
      t.removeAttribute('tabindex');
      t.removeAttribute('role');
    });
  };

  Editor.prototype.drawPage = function () {
    var n = this.cur;
    if (!this.pvEl) return;
    this.pvEl.innerHTML = n ? this.render(n, n.page, false) : '';
    this.tidyPreview(this.pvEl);
    this.drawCompare();
  };

  Editor.prototype.setTab = function (t) {
    this.el.setAttribute('data-tab', t);
    [].forEach.call(this.el.querySelectorAll('[data-act="tab"]'), function (b) { b.setAttribute('aria-pressed', b.getAttribute('data-tab') === t ? 'true' : 'false'); });
  };

  Editor.prototype.narrow = function () {
    try { return window.matchMedia('(max-width: 700px)').matches; } catch (e) { return false; }
  };

  // --- Editing ---------------------------------------------------------------

  // Called for every change to the open page: marks it edited (a package page
  // becomes the campaign's own copy), schedules the save and refreshes the
  // parts that are not fields.
  Editor.prototype.edited = function () {
    var n = this.cur;
    if (!n) return;
    var was = n.state;
    if (n.state === 'package') n.state = 'edited';
    this.touch(n);
    this.refreshNode(n);
    if (was !== n.state) this.drawBanner();
    this.drawPage();
  };

  Editor.prototype.onInput = function (e) {
    var t = e.target, n = this.cur, d;
    if (!t || !t.getAttribute) return;
    if ((d = t.getAttribute('data-chtitle'))) {
      var ch = this.chapterByUid(d);
      if (ch) { ch.title = t.value; this.touch(ch); this.drawToc(); this.drawPage(); }
      return;
    }
    if (t.hasAttribute('data-termfilter')) { this.fillTerms(t); return; }
    if ((d = t.getAttribute('data-path')) && n) {
      if (setPath(n.page, d, t.value)) this.edited();
    }
  };

  Editor.prototype.onChange = function (e) {
    var t = e.target, n = this.cur, d;
    if (!t || !t.hasAttribute || !n) return;
    if (t.hasAttribute('data-pdir')) { n.page.director = t.checked; this.edited(); this.drawToc(); return; }
    if (t.hasAttribute('data-pwide')) { n.page.wide = t.checked; this.edited(); return; }
    if ((d = t.getAttribute('data-bdir')) !== null) {
      var b = n.page.blocks[+d];
      if (b) {
        b.director = t.checked;
        var sec = t.closest('.rbe-blk');
        if (sec) sec.classList.toggle('isdir', t.checked);
        this.edited();
      }
      return;
    }
    if ((d = t.getAttribute('data-bmod')) !== null) {
      var rb = n.page.blocks[+d];
      if (rb) { rb.modifier = t.checked; this.edited(); }
    }
  };

  Editor.prototype.onKey = function (e) {
    if (e.key !== 'Escape') return;
    var menu = e.target.closest && e.target.closest('.rbe-termmenu');
    if (menu) {
      var bi = menu.getAttribute('data-i');
      this.closeTermMenu(menu);
      var ta = document.getElementById(this.id('blocks-' + bi + '-text'));
      if (ta) ta.focus();
    }
  };

  Editor.prototype.chapterByUid = function (u) {
    for (var i = 0; i < this.parts.length; i++) {
      for (var j = 0; j < this.parts[i].chapters.length; j++) if (this.parts[i].chapters[j].uid === u) return this.parts[i].chapters[j];
    }
    return null;
  };

  Editor.prototype.select = function (n, focusTitle) {
    this.cur = n;
    this.palOpen = false;
    this.compare = false;
    this.confirm = '';
    this.drawToc();
    this.drawBanner();
    this.drawFields();
    this.drawPage();
    if (this.narrow()) this.setTab('form');
    if (focusTitle) {
      var t = document.getElementById(this.id('title'));
      if (t) { t.focus(); t.select(); }
    }
  };

  // --- Text tools ------------------------------------------------------------

  Editor.prototype.textField = function (bi) { return document.getElementById(this.id('blocks-' + bi + '-text')); };

  Editor.prototype.setText = function (ta, value) {
    ta.value = value;
    if (this.cur && setPath(this.cur.page, ta.getAttribute('data-path'), value)) this.edited();
  };

  Editor.prototype.wrapSel = function (bi, a, z) {
    var ta = this.textField(bi);
    if (!ta) return;
    var s = ta.selectionStart, e = ta.selectionEnd, v = ta.value, mid = v.slice(s, e) || 'words';
    this.setText(ta, v.slice(0, s) + a + mid + z + v.slice(e));
    ta.focus();
    ta.setSelectionRange(s + a.length, s + a.length + mid.length);
  };

  Editor.prototype.closeTermMenu = function (menu) {
    var btn = this.el.querySelector('[data-act="fmt"][data-f="t"][data-i="' + menu.getAttribute('data-i') + '"]');
    if (btn) btn.setAttribute('aria-expanded', 'false');
    menu.remove();
  };

  Editor.prototype.toggleTermMenu = function (btn) {
    var bi = btn.getAttribute('data-i'), field = btn.closest('.rbe-field'), old = field.querySelector('.rbe-termmenu');
    if (old) { this.closeTermMenu(old); return; }
    var menu = document.createElement('div');
    menu.className = 'rbe-termmenu';
    menu.setAttribute('role', 'group');
    menu.setAttribute('aria-label', 'Rule words');
    menu.setAttribute('data-i', bi);
    if (!this.termKeys.length) {
      menu.innerHTML = '<span class="rbe-note-line">This game system has no rule words to link yet.</span>';
    } else {
      menu.innerHTML = (this.termKeys.length > 12 ? '<input class="rbe-in" type="search" data-termfilter aria-label="Find a rule word" placeholder="Find a rule word">' : '') + '<div class="rbe-terms"></div>';
      this.fillTerms(menu);
    }
    btn.parentNode.insertAdjacentElement('afterend', menu);
    btn.setAttribute('aria-expanded', 'true');
    var f = menu.querySelector('input') || menu.querySelector('button');
    if (f) f.focus();
  };

  Editor.prototype.fillTerms = function (from) {
    var menu = from.closest('.rbe-termmenu'), box = menu.querySelector('.rbe-terms'), inp = menu.querySelector('input');
    if (!box) return;
    var q = inp ? inp.value.trim().toLowerCase() : '', self = this, n = 0, h = '';
    this.termKeys.forEach(function (k) {
      if (n >= 24 || (q && k.indexOf(q) < 0 && self.terms[k].name.toLowerCase().indexOf(q) < 0)) return;
      n++;
      h += '<button type="button" data-act="term" data-t="' + esc(k) + '">' + esc(self.terms[k].name) + '</button>';
    });
    box.innerHTML = h || '<span class="rbe-note-line">No rule word matches.</span>';
  };

  Editor.prototype.insertTerm = function (btn) {
    var menu = btn.closest('.rbe-termmenu'), bi = menu.getAttribute('data-i'), ta = this.textField(bi);
    var key = btn.getAttribute('data-t');
    this.closeTermMenu(menu);
    if (!ta) return;
    var s = ta.selectionStart, e = ta.selectionEnd, v = ta.value, sel = v.slice(s, e);
    var ins = '[[' + key + (sel && sel.toLowerCase() !== key ? '|' + sel : '') + ']]';
    this.setText(ta, v.slice(0, s) + ins + v.slice(e));
    ta.focus();
    ta.setSelectionRange(s + ins.length, s + ins.length);
  };

  // --- Clicks ----------------------------------------------------------------

  Editor.prototype.onClick = function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    var b = t.closest('[data-act]');
    if (!b || !this.el.contains(b)) return;
    var act = b.getAttribute('data-act'), n = this.cur, self = this, i = +b.getAttribute('data-i');
    var blk = n ? n.page.blocks[i] : null;
    switch (act) {
      case 'reload': this.load(); return;
      case 'tab': this.setTab(b.getAttribute('data-tab')); return;
      case 'as':
        this.as = b.getAttribute('data-as');
        [].forEach.call(this.el.querySelectorAll('[data-act="as"]'), function (x) { x.setAttribute('aria-pressed', x === b ? 'true' : 'false'); });
        this.drawPage();
        return;
      case 'pick': if (this.byId[b.getAttribute('data-id')]) this.select(this.byId[b.getAttribute('data-id')]); return;
      case 'export': this.leave(this.src.exportUrl); return;
      case 'done': this.leave(this.doneUrl); return;
      case 'retry':
        this.opError = '';
        this.each(function (o) { if (o.error) { o.error = ''; o.dirty = true; self.flush(o); } });
        this.syncSaved();
        return;
      case 'addpage': this.addPage(this.chapterByUid(b.getAttribute('data-ch'))); return;
      case 'addchapter': this.addHouseChapter(); return;
      case 'compare': this.compare = !this.compare; this.drawBanner(); return;
      case 'keep': this.keepMine(); return;
      case 'theirs': case 'reset': this.dropCopy(); return;
      case 'rmpage': this.confirm = 'rmpage'; this.drawBanner(); return;
      case 'rmpage-no': this.confirm = ''; this.drawBanner(); return;
      case 'rmpage-yes': this.removePage(); return;
      case 'rmch': this.confirm = 'rmch'; this.drawFields(this.id('rmch-yes')); return;
      case 'rmch-no': this.confirm = ''; this.drawFields(this.id('rmch')); return;
      case 'rmch-yes': this.removeChapter(); return;
    }
    if (!n) return;
    switch (act) {
      case 'addblk': this.palOpen = !this.palOpen; this.drawFields(this.id('addblk')); return;
      case 'new':
        n.page.blocks.push(newBlock(b.getAttribute('data-type')));
        this.palOpen = false;
        this.edited();
        this.drawFields();
        var secs = this.fieldsEl.querySelectorAll('.rbe-blk'), last = secs[secs.length - 1];
        if (last) { var f = last.querySelector('input:not([type="checkbox"]),textarea'); if (f) f.focus(); }
        return;
      case 'rm':
        if (!blk) return;
        n.page.blocks.splice(i, 1);
        this.edited();
        this.drawFields(n.page.blocks.length ? this.id('mv-' + Math.min(i, n.page.blocks.length - 1) + '-down') : this.id('addblk'));
        if (document.activeElement === document.body || !this.fieldsEl.contains(document.activeElement)) this.focusId(this.id('addblk'));
        return;
      case 'mv':
        var d = +b.getAttribute('data-d'), j = i + d;
        if (!blk || j < 0 || j >= n.page.blocks.length) return;
        n.page.blocks[i] = n.page.blocks[j];
        n.page.blocks[j] = blk;
        this.edited();
        this.drawFields(this.id('mv-' + j + (d < 0 ? '-up' : '-down')));
        if (document.activeElement !== document.getElementById(this.id('mv-' + j + (d < 0 ? '-up' : '-down')))) {
          this.focusId(this.id('mv-' + j + (d < 0 ? '-down' : '-up')));
        }
        return;
      case 'addrow':
        if (!blk) return;
        var list = b.getAttribute('data-list');
        blk[list].push(newRow(list));
        this.edited();
        this.drawFields();
        var last2 = blk[list].length - 1, first = { items: '-title', stats: '-label', bands: '-max' }[list] || '';
        this.focusId(this.id('blocks-' + i + '-' + list + '-' + last2 + first));
        return;
      case 'rmrow':
        if (!blk) return;
        var l2 = b.getAttribute('data-list');
        blk[l2].splice(+b.getAttribute('data-j'), 1);
        this.edited();
        this.drawFields(this.id('add-' + i + '-' + l2));
        return;
      case 'fmt':
        var f2 = b.getAttribute('data-f');
        if (f2 === 'b') this.wrapSel(i, '**', '**');
        else if (f2 === 'i') this.wrapSel(i, '*', '*');
        else this.toggleTermMenu(b);
        return;
      case 'term': this.insertTerm(b); return;
    }
  };

  // --- State changes that need the server ------------------------------------

  Editor.prototype.keepMine = function () {
    var self = this, n = this.cur;
    if (!n) return;
    this.op(function () {
      return self.flush(n).then(function () {
        return self.call('POST', self.pageUrl(n.ch, n.key, '/keep'), {});
      }).then(function (j) {
        self.applyEntry(n, j && j.entry, false);
        if (self.cur === n) { self.compare = false; self.drawBanner(); }
        self.refreshNode(n);
      });
    });
  };

  // "Go back to <System>'s page" and "Use <System>'s": drop the campaign's copy.
  Editor.prototype.dropCopy = function () {
    var self = this, n = this.cur;
    if (!n) return;
    this.op(function () {
      return self.settle(n).then(function () {
        return self.call('DELETE', self.pageUrl(n.ch, n.key));
      }).then(function (j) {
        if (!isObj(j) || !isObj(j.entry)) { self.dropNode(n); return; }
        self.applyEntry(n, j.entry, true);
        self.compare = false;
        if (self.cur === n) { self.drawBanner(); self.drawFields(); self.drawPage(); }
        self.refreshNode(n);
        self.syncSaved();
      });
    });
  };

  Editor.prototype.dropNode = function (n) {
    var ch = n.ch, idx = ch.pages.indexOf(n);
    if (idx >= 0) ch.pages.splice(idx, 1);
    delete this.byId[n.uid];
    var next = ch.pages[Math.min(idx, ch.pages.length - 1)] || this.firstNode();
    this.select(next);
  };

  Editor.prototype.removePage = function () {
    var self = this, n = this.cur;
    if (!n) return;
    this.op(function () {
      return self.settle(n).then(function () {
        return self.call('DELETE', self.pageUrl(n.ch, n.key));
      }).then(function () { self.dropNode(n); });
    });
  };

  Editor.prototype.addPage = function (ch) {
    var self = this;
    if (!ch) return;
    this.op(function () {
      return self.call('POST', self.pageUrl(ch), { page: { title: 'New page', blocks: [{ text: '' }] } }).then(function (j) {
        if (!j || !isObj(j.entry)) throw new Error('The server did not return the new page.');
        var n = self.addNode(ch, j.entry);
        ch.pages.push(n);
        self.select(n, true);
      });
    });
  };

  Editor.prototype.addHouseChapter = function () {
    var self = this;
    this.op(function () {
      return self.call('POST', self.base + '/chapters', { title: 'New chapter' }).then(function (j) {
        if (!j || !isObj(j.chapter)) throw new Error('The server did not return the new chapter.');
        var part = null;
        self.parts.forEach(function (p) { if (p.house) part = p; });
        if (!part) { part = { title: 'House rules', director: false, house: true, chapters: [] }; self.parts.push(part); }
        var ch = self.addChapter(part, j.chapter);
        ch.house = true;
        if (ch.pages.length) self.select(ch.pages[0]);
        else { self.drawToc(); }
        var t = document.getElementById(self.id('chtitle'));
        if (t) { t.focus(); t.select(); }
      });
    });
  };

  Editor.prototype.removeChapter = function () {
    var self = this, n = this.cur;
    if (!n || !n.ch.house) return;
    var ch = n.ch;
    this.op(function () {
      return self.settle(ch).then(function () {
        return Promise.all(ch.pages.map(function (p) { return self.settle(p); }));
      }).then(function () {
        return self.call('DELETE', self.base + '/chapters/' + encodeURIComponent(ch.id));
      }).then(function () {
        ch.pages.forEach(function (p) { delete self.byId[p.uid]; });
        var list = ch.part.chapters, idx = list.indexOf(ch);
        if (idx >= 0) list.splice(idx, 1);
        self.confirm = '';
        self.select(self.firstNode());
      });
    });
  };

  // Export and Done: pending edits go to the server first, so the download and
  // the book both show them. If a save fails the person stays and sees why.
  Editor.prototype.leave = function (url) {
    var self = this, to = safeUrl(url);
    if (!to) return;
    this.flushAll().then(function (ok) {
      if (self.dead || !ok) return;
      window.location.assign(to);
    });
  };

  Chronicle.register('rulebook-editor', {
    init: function (el, config) {
      var ed = new Editor(el, config || {});
      instances.set(el, ed);
      ed.start();
    },
    destroy: function (el) {
      var ed = instances.get(el);
      if (ed) { ed.stop(); instances.delete(el); }
    }
  });
})();
