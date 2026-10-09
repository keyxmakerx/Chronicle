/**
 * rulebook.js -- the Rulebook book renderer.
 *
 * Draws a game system's book as a full-screen, page-turning rulebook: a slim
 * top bar, two flat pages (one on narrow screens), page-corner turn buttons, a
 * Contents drawer and a small library of interactive blocks (dice, fold-out
 * cards, flaps, a stepped example, creature cards, Director notes, hover
 * terms). The styling is rulebook.css; the data is the book JSON the server
 * assembles for the viewer.
 *
 * The server decides what each reader may see. This file hides Director
 * content again only so a Director can preview the player view, and treats
 * every string in the JSON as untrusted: text is escaped first and the markup
 * rules (paragraphs, lists, bold, italic, [[terms]]) are applied to the escaped
 * result. Theme values only reach CSS through setProperty on an allow-list.
 *
 * Mount: <div data-widget="rulebook" data-campaign-id data-book-url>
 */
(function () {
  'use strict';

  var PLACE_KEY = 'chronicle.rulebook.place.';
  var NARROW_QUERY = '(max-width: 900px)';
  var TURN_MS = { flip: 780, slide: 520, fade: 380 };
  var SWIPE_PX = 60;

  var THEME_COLOURS = { 'paper': 1, 'paper-alt': 1, 'ink': 1, 'ink-soft': 1, 'muted': 1, 'edge': 1, 'accent': 1, 'cover': 1 };
  var THEME_FONTS = { 'heading-font': 1, 'body-font': 1 };
  var COLOUR_RE = /^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;
  var FONT_RE = /^[A-Za-z0-9 ,'"._-]{1,200}$/;
  var SLUG_RE = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/;
  // House chapters a campaign writes have ids with this prefix; a package's
  // chapter ids cannot contain _, so the two never clash.
  var HOUSE_PREFIX = 'house_';

  var instances = new WeakMap();
  var uid = 0;

  // --- Small helpers ---------------------------------------------------------

  function esc(s) { return Chronicle.escapeHtml(s == null ? '' : String(s)); }
  function str(v) { return typeof v === 'string' ? v : (typeof v === 'number' ? String(v) : ''); }
  function arr(v) { return Array.isArray(v) ? v : []; }
  function isObj(v) { return !!v && typeof v === 'object' && !Array.isArray(v); }
  function has(o, k) { return Object.prototype.hasOwnProperty.call(o, k); }
  function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }
  function media(q) { try { return window.matchMedia(q).matches; } catch (e) { return false; } }
  function reduced() { return media('(prefers-reduced-motion: reduce)'); }
  function intOr(v, lo, hi, dflt) {
    var n = typeof v === 'number' ? v : NaN;
    return isFinite(n) && Math.floor(n) === n ? clamp(n, lo, hi) : dflt;
  }
  function rnd(n) {
    try {
      var a = new Uint32Array(1);
      crypto.getRandomValues(a);
      return 1 + (a[0] % n);
    } catch (e) {
      return 1 + Math.floor(Math.random() * n);
    }
  }

  function loadPlace(campaignId) {
    try {
      var v = JSON.parse(localStorage.getItem(PLACE_KEY + campaignId) || 'null');
      return isObj(v) ? v : null;
    } catch (e) { return null; }
  }
  function savePlace(campaignId, place) {
    try { localStorage.setItem(PLACE_KEY + campaignId, JSON.stringify(place)); } catch (e) { /* storage blocked */ }
  }

  // --- Text markup -----------------------------------------------------------
  // Order matters: [[terms]] are pulled out first (they need the unescaped
  // text for the lookup), everything else is escaped, then bold/italic run on
  // the escaped text, then the term spans go back in. Nothing from the JSON
  // reaches the output without passing esc().

  function inline(raw, terms) {
    var slots = [];
    var s = str(raw).replace(/\u0000/g, '');
    s = s.replace(/\[\[([^\[\]|\n]+?)(?:\|([^\[\]\n]+?))?\]\]/g, function (_, term, shown) {
      var key = term.trim().toLowerCase();
      var label = (shown || term).trim();
      if (!has(terms, key) || !terms[key].text) return label;
      slots.push('<span class="rb-term" role="button" tabindex="0" data-term="' + esc(key) + '">' + esc(label) + '</span>');
      return '\u0000' + (slots.length - 1) + '\u0000';
    });
    s = esc(s);
    s = s.replace(/\*\*(?=\S)([^*]+?)\*\*/g, '<strong>$1</strong>');
    s = s.replace(/(^|[^*])\*(?=\S)([^*]+?)\*(?!\*)/g, '$1<em>$2</em>');
    return s.replace(/\u0000(\d+)\u0000/g, function (_, i) { return slots[+i] || ''; });
  }

  // Paragraphs split on blank lines; consecutive "- " lines become a list.
  function rich(raw, terms) {
    var text = str(raw).replace(/\r\n?/g, '\n').trim();
    if (!text) return '';
    return text.split(/\n[ \t]*\n/).map(function (para) {
      var out = '', buf = [], list = [];
      function flushP() { if (buf.length) { out += '<p>' + inline(buf.join(' '), terms) + '</p>'; buf = []; } }
      function flushL() {
        if (list.length) {
          out += '<ul>' + list.map(function (li) { return '<li>' + inline(li, terms) + '</li>'; }).join('') + '</ul>';
          list = [];
        }
      }
      para.split('\n').forEach(function (line) {
        var m = /^\s*-\s+(.*)$/.exec(line);
        if (m) { flushP(); list.push(m[1]); }
        // An indented line straight after a list item carries that item on.
        else if (list.length && /^\s+\S/.test(line)) list[list.length - 1] += ' ' + line.trim();
        else { flushL(); if (line.trim()) buf.push(line.trim()); }
      });
      flushP(); flushL();
      return out;
    }).join('');
  }

  // --- Reading the wire format ------------------------------------------------
  // Shape is checked once here so rendering can trust its input's types (not
  // its content).

  function normalize(json) {
    if (!isObj(json) || !Array.isArray(json.parts)) return null;
    var terms = {};
    if (isObj(json.terms)) {
      Object.keys(json.terms).forEach(function (k) {
        var t = json.terms[k];
        if (isObj(t) && str(t.text)) terms[k.toLowerCase()] = { name: str(t.name) || k, text: str(t.text) };
      });
    }
    var turn = str(json.turn);
    return {
      title: str(json.title) || 'Rulebook',
      mark: str(json.mark).slice(0, 3),
      systemName: str(json.systemName),
      turn: TURN_MS[turn] ? turn : 'flip',
      theme: isObj(json.theme) ? json.theme : {},
      look: json.look === 'paper' ? 'paper' : '',
      tabs: arr(json.tabs).filter(function (id) { return typeof id === 'string' && SLUG_RE.test(id); }),
      start: SLUG_RE.test(str(json.start)) ? str(json.start) : '',
      first: SLUG_RE.test(str(json.first)) ? str(json.first) : '',
      isDirector: json.isDirector === true,
      canEdit: json.canEdit === true,
      terms: terms,
      parts: json.parts.filter(isObj).map(function (part) {
        return {
          title: str(part.title),
          director: part.director === true,
          chapters: arr(part.chapters).filter(isObj).map(function (ch) {
            return {
              id: str(ch.id),
              title: str(ch.title),
              intro: str(ch.intro),
              director: ch.director === true,
              generated: ch.generated === true,
              pages: arr(ch.pages).filter(isObj).map(function (pg) {
                return {
                  title: str(pg.title),
                  director: pg.director === true,
                  wide: pg.wide === true,
                  columns: pg.columns === true,
                  blocks: arr(pg.blocks).filter(isObj)
                };
              })
            };
          })
        };
      })
    };
  }

  function errorMessage(err) {
    var s = err && err.status;
    if (s === 401 || s === 403) return 'You do not have access to this book.';
    if (s === 404) return 'This game system does not have a book.';
    if (s) return 'The book could not be loaded (error ' + s + ').';
    if (err && err.unreadable) return 'The book data could not be read.';
    return 'The server could not be reached.';
  }

  // --- The widget ------------------------------------------------------------

  function Rulebook(el, config) {
    this.el = el;
    this.campaignId = str(config.campaignId);
    this.url = str(config.bookUrl);
    this.data = null;
    this.view = 'director';
    this.items = [];
    this.spreads = [];
    this.at = 0;
    this.narrow = media(NARROW_QUERY);
    this.busy = false;
    this.dead = false;
    this.loadSeq = 0;
    this.timers = [];
    this.card = null;
    this.pinned = false;
    this.leafTimer = 0;
    this.touch = null;
    this.handlers = [];
    this.finishNow = null;
    this.refocus = false;
    this.ixCache = {};
    this.pendingEntry = '';
    this.searchSeq = 0;
    this.searchTimer = 0;
    this.inst = ++uid;
  }

  Rulebook.prototype.domId = function (name) { return 'rb' + this.inst + '-' + name; };

  Rulebook.prototype.on = function (target, type, fn, opts) {
    target.addEventListener(type, fn, opts);
    this.handlers.push([target, type, fn, opts]);
  };

  Rulebook.prototype.start = function () {
    var self = this;
    this.el.classList.add('rb');
    this.on(document, 'click', function (e) { self.onClick(e); });
    this.on(document, 'keydown', function (e) { self.onKey(e); });
    this.on(this.el, 'mouseover', function (e) { self.onOver(e); });
    this.on(this.el, 'focusin', function (e) { self.onFocus(e, true); });
    this.on(this.el, 'focusout', function (e) { self.onFocus(e, false); });
    this.on(this.el, 'input', function (e) { self.onInput(e); });
    this.on(this.el, 'scroll', function () { if (self.card) self.hideCard(true); }, true);
    this.on(this.el, 'touchstart', function (e) { self.onTouch(e, true); }, { passive: true });
    this.on(this.el, 'touchend', function (e) { self.onTouch(e, false); }, { passive: true });
    try {
      var mq = window.matchMedia(NARROW_QUERY);
      var change = function () { self.onResize(mq.matches); };
      if (mq.addEventListener) { mq.addEventListener('change', change); this.handlers.push([mq, 'change', change]); }
      else if (mq.addListener) { mq.addListener(change); this.handlers.push([mq, null, change]); }
    } catch (e) { /* no matchMedia: stay in two-page mode */ }
    this.load();
  };

  Rulebook.prototype.stop = function () {
    this.dead = true;
    this.handlers.forEach(function (h) {
      if (h[1] === null) h[0].removeListener(h[2]);
      else h[0].removeEventListener(h[1], h[2], h[3]);
    });
    this.handlers = [];
    this.timers.forEach(function (t) { clearInterval(t); clearTimeout(t); });
    this.timers = [];
    clearTimeout(this.leafTimer);
    clearTimeout(this.searchTimer);
    this.destroyNested(this.el);
    this.el.innerHTML = '';
    this.el.classList.remove('rb');
    this.el.removeAttribute('aria-busy');
  };

  // --- Loading ---------------------------------------------------------------

  Rulebook.prototype.showState = function (kind, message) {
    var html;
    if (kind === 'loading') {
      html = '<div class="rb-state" role="status"><span class="rb-spinner" aria-hidden="true"></span><p>Opening the book…</p></div>';
    } else {
      html = '<div class="rb-state rb-state-error" role="alert"><p class="rb-state-title">The book could not be opened</p>' +
        '<p>' + esc(message) + '</p><button type="button" class="rb-btn rb-btn-primary" data-act="retry">Retry</button></div>';
    }
    this.el.innerHTML = html;
    this.el.setAttribute('aria-busy', kind === 'loading' ? 'true' : 'false');
  };

  Rulebook.prototype.load = function () {
    var self = this, seq = ++this.loadSeq;
    if (!this.url) { this.showState('error', 'This page has no book address.'); return; }
    this.showState('loading');
    Chronicle.apiFetch(this.url).then(function (res) {
      if (!res.ok) { var err = new Error('http'); err.status = res.status; throw err; }
      return res.json().catch(function () { var e = new Error('json'); e.unreadable = true; throw e; });
    }).then(function (json) {
      if (self.dead || seq !== self.loadSeq) return;
      var data = normalize(json);
      if (!data) { var e = new Error('shape'); e.unreadable = true; throw e; }
      self.open(data);
    }).catch(function (err) {
      if (self.dead || seq !== self.loadSeq) return;
      self.showState('error', errorMessage(err));
    });
  };

  Rulebook.prototype.applyTheme = function (theme) {
    var style = this.el.style;
    Object.keys(theme).forEach(function (k) {
      var v = theme[k];
      if (typeof v !== 'string') return;
      if ((has(THEME_COLOURS, k) && COLOUR_RE.test(v)) || (has(THEME_FONTS, k) && FONT_RE.test(v))) {
        style.setProperty('--book-' + k, v);
      }
    });
    this.el.setAttribute('data-turn', this.data.turn);
    if (this.data.look) this.el.setAttribute('data-look', this.data.look);
    else this.el.removeAttribute('data-look');
  };

  Rulebook.prototype.open = function (data) {
    this.data = data;
    this.applyTheme(data.theme);
    this.view = data.isDirector ? 'director' : 'player';
    var place = loadPlace(this.campaignId);
    if (data.isDirector && place && (place.view === 'player' || place.view === 'director')) this.view = place.view;

    this.el.innerHTML = this.frameHTML();
    this.el.setAttribute('aria-busy', 'false');
    var q = this.el.querySelector.bind(this.el);
    this.pl = q('.rb-page.l');
    this.pr = q('.rb-page.r');
    this.bookEl = q('.rb-book');
    this.drawerEl = q('.rb-drawer');
    this.scrimEl = q('.rb-scrim');
    this.cardEl = q('.rb-hc');
    this.whereEl = q('.rb-where');
    this.srEl = q('.rb-sr');
    this.tocBtn = q('[data-act="toc"]');
    this.curlPrev = q('.rb-curl.l');
    this.curlNext = q('.rb-curl.r');
    this.searchEl = q('[data-search]');
    this.resultsEl = q('.rb-results');

    this.tabsEl = q('.rb-tabs');
    this.rebuild(this.openingKey(place));
  };

  // Where the book opens: a chapter named in the address (#making-a-hero),
  // else the book's first-visit chapter for a reader with no saved place,
  // else its start chapter, else the reader's saved place.
  Rulebook.prototype.openingKey = function (place) {
    var d = this.data, hash = '';
    try { hash = decodeURIComponent(String(window.location.hash || '').slice(1)); } catch (e) { hash = ''; }
    var key = SLUG_RE.test(hash) ? this.chapterKey(hash) : '';
    if (key) return key;
    var saved = place && typeof place.key === 'string' ? place.key : '';
    if (!saved && d.first) key = this.chapterKey(d.first);
    if (!key && d.start) key = this.chapterKey(d.start);
    return key || saved;
  };

  // chapterKey is the item key prefix ("part.chapter") of a chapter id, which
  // rebuild resolves to the chapter's opener or its first shown page.
  Rulebook.prototype.chapterKey = function (id) {
    var parts = this.data.parts;
    for (var pi = 0; pi < parts.length; pi++) {
      for (var ci = 0; ci < parts[pi].chapters.length; ci++) {
        if (parts[pi].chapters[ci].id === id) return pi + '.' + ci;
      }
    }
    return '';
  };

  // firstItemOf is the first shown item of a chapter, or -1 when the current
  // view does not show it.
  Rulebook.prototype.firstItemOf = function (id) {
    for (var i = 0; i < this.items.length; i++) if (this.items[i].ch.id === id) return i;
    return -1;
  };

  Rulebook.prototype.goChapter = function (id) {
    var n = this.firstItemOf(id);
    if (n >= 0) this.goItem(n);
  };

  // The tab strip: the book's chosen chapters, then a House rules tab when
  // the campaign has written its own chapters.
  Rulebook.prototype.tabsHTML = function () {
    var d = this.data;
    if (!d.tabs.length) return '';
    var tabs = [], seen = {};
    d.parts.forEach(function (part) {
      part.chapters.forEach(function (ch) { seen[ch.id] = ch; });
    });
    d.tabs.forEach(function (id) { if (seen[id]) tabs.push({ id: id, label: seen[id].title }); });
    var house = '';
    d.parts.forEach(function (part) {
      part.chapters.forEach(function (ch) { if (!house && ch.id.indexOf(HOUSE_PREFIX) === 0) house = ch.id; });
    });
    if (house) tabs.push({ id: house, label: 'House rules', house: true });
    if (!tabs.length) return '';
    return '<nav class="rb-tabs" aria-label="Sections">' + tabs.map(function (t) {
      return '<button type="button" class="rb-tab' + (t.house ? ' house' : '') + '" data-tab="' + esc(t.id) + '"' +
        (t.house ? ' data-house="1"' : '') + '>' + esc(t.label) + '</button>';
    }).join('') + '</nav>';
  };

  // syncTabs hides tabs the current view does not show and marks the tab of
  // the chapter on the page.
  Rulebook.prototype.syncTabs = function () {
    if (!this.tabsEl) return;
    var self = this, sp = this.spreads[this.at], ch = sp ? sp.items[0].ch : null;
    var house = ch && ch.id.indexOf(HOUSE_PREFIX) === 0;
    [].forEach.call(this.tabsEl.querySelectorAll('[data-tab]'), function (b) {
      var id = b.getAttribute('data-tab');
      b.hidden = self.firstItemOf(id) < 0;
      var on = !!ch && (ch.id === id || (house && b.hasAttribute('data-house')));
      if (on) b.setAttribute('aria-current', 'true'); else b.removeAttribute('aria-current');
    });
  };

  Rulebook.prototype.frameHTML = function () {
    var d = this.data, seg = '';
    if (d.isDirector) {
      seg = '<div class="rb-seg" role="group" aria-label="Book view">' +
        '<button type="button" data-act="view" data-view="player" aria-pressed="false">Player view</button>' +
        '<button type="button" data-act="view" data-view="director" aria-pressed="false">Director view</button></div>';
    }
    var edit = d.canEdit && this.url
      ? '<button type="button" class="rb-btn" data-act="edit" data-href="' + esc(this.url.replace(/[?#].*$/, '').replace(/\/+$/, '') + '/edit') + '">Edit</button>'
      : '';
    return '<div class="rb-bar">' +
      '<div class="rb-brand">' + (d.mark ? '<span class="rb-mark" aria-hidden="true">' + esc(d.mark) + '</span>' : '') +
      '<div class="rb-name"><span class="rb-title">' + esc(d.title) + '</span>' +
      (d.systemName ? '<small>' + esc(d.systemName) + '</small>' : '') + '</div></div>' +
      '<button type="button" class="rb-btn" data-act="toc" aria-expanded="false" aria-label="Open the contents"><span aria-hidden="true">☰</span> Contents</button>' +
      '<span class="rb-where"></span><span class="rb-sp"></span>' +
      '<div class="rb-search" role="search"><span class="rb-search-ic" aria-hidden="true">⌕</span>' +
      '<input type="search" class="rb-search-in" data-search placeholder="Search the book" aria-label="Search the book" autocomplete="off" spellcheck="false"' +
      ' aria-controls="' + this.domId('results') + '" aria-expanded="false"><kbd aria-hidden="true">/</kbd>' +
      '<div class="rb-results" id="' + this.domId('results') + '" role="region" aria-label="Search results" hidden></div></div>' +
      edit + seg + '</div>' + this.tabsHTML() +
      '<div class="rb-book">' +
      '<article class="rb-page l"></article><div class="rb-spine" aria-hidden="true"></div><article class="rb-page r"></article>' +
      '<button type="button" class="rb-curl l" data-act="prev" aria-label="Previous page"></button>' +
      '<button type="button" class="rb-curl r" data-act="next" aria-label="Next page"></button>' +
      '<div class="rb-scrim"></div><aside class="rb-drawer" aria-label="Contents"></aside></div>' +
      '<div class="rb-hc" role="tooltip" hidden></div><div class="rb-sr" aria-live="polite"></div>';
  };

  // --- Page list -------------------------------------------------------------

  Rulebook.prototype.showDirector = function () {
    return this.data.isDirector && this.view === 'director';
  };

  // Every chapter becomes an opener page followed by its pages, so a spread
  // always holds two consecutive items; a wide page takes a spread alone. A
  // chapter that is one wide page (a system's own front page) needs no
  // opener. In a Director's player-view preview, a page whose blocks are all
  // Director-only is skipped, as the server skips it for real players.
  Rulebook.prototype.buildItems = function () {
    var dir = this.showDirector(), items = [];
    var pageShows = function (p) {
      if (p.director && !dir) return false;
      return dir || p.blocks.some(function (b) { return !b.director; });
    };
    this.data.parts.forEach(function (part, pi) {
      if (part.director && !dir) return;
      part.chapters.forEach(function (ch, ci) {
        if (ch.director && !dir) return;
        var shown = [];
        ch.pages.forEach(function (p, gi) { if (pageShows(p)) shown.push({ p: p, gi: gi }); });
        if (!shown.length) return;
        var opener = { kind: 'open', part: part, ch: ch, key: pi + '.' + ci, wide: false, pages: [] };
        var solo = shown.length === 1 && shown[0].p.wide;
        if (!solo) items.push(opener);
        shown.forEach(function (s) {
          var it = { kind: 'page', part: part, ch: ch, p: s.p, key: pi + '.' + ci + '.' + s.gi, wide: s.p.wide && !media(NARROW_QUERY), solo: solo };
          items.push(it);
          opener.pages.push(it);
        });
      });
    });
    items.forEach(function (it, i) { it.n = i; });
    this.items = items;
  };

  Rulebook.prototype.buildSpreads = function () {
    var sp = [], cur = [];
    if (this.narrow) {
      this.items.forEach(function (it) { sp.push({ items: [it], wide: false }); });
    } else {
      // A handbook starts every chapter on a fresh spread, so a tab or link
      // always lands on a spread that opens with that chapter.
      var fresh = this.data.look === 'paper';
      this.items.forEach(function (it) {
        if (fresh && it.kind === 'open' && cur.length) { sp.push({ items: cur, wide: false }); cur = []; }
        if (it.wide) {
          if (cur.length) { sp.push({ items: cur, wide: false }); cur = []; }
          sp.push({ items: [it], wide: true });
        } else {
          cur.push(it);
          if (cur.length === 2) { sp.push({ items: cur, wide: false }); cur = []; }
        }
      });
      if (cur.length) sp.push({ items: cur, wide: false });
    }
    this.spreads = sp;
  };

  Rulebook.prototype.spreadOf = function (itemIndex) {
    for (var i = 0; i < this.spreads.length; i++) {
      for (var j = 0; j < this.spreads[i].items.length; j++) {
        if (this.spreads[i].items[j].n === itemIndex) return i;
      }
    }
    return 0;
  };

  Rulebook.prototype.currentKey = function () {
    var sp = this.spreads[this.at];
    return sp ? sp.items[0].key : '';
  };

  // Rebuild after the view or the page-per-spread count changes, staying on
  // the same page, or on the same chapter when that page is no longer shown.
  Rulebook.prototype.rebuild = function (key) {
    if (this.finishNow) this.finishNow();
    this.closeDrawer(false);
    this.hideCard();
    this.narrow = media(NARROW_QUERY);
    this.buildItems();
    this.buildSpreads();
    var idx = -1, i;
    for (i = 0; i < this.items.length && key; i++) if (this.items[i].key === key) { idx = i; break; }
    if (idx < 0 && key) {
      var prefix = key.split('.').slice(0, 2).join('.');
      for (i = 0; i < this.items.length; i++) {
        if (this.items[i].key === prefix || this.items[i].key.indexOf(prefix + '.') === 0) { idx = i; break; }
      }
    }
    this.at = idx < 0 ? 0 : this.spreadOf(idx);
    var self = this;
    [].forEach.call(this.el.querySelectorAll('[data-act="view"]'), function (b) {
      b.setAttribute('aria-pressed', b.getAttribute('data-view') === self.view ? 'true' : 'false');
    });
    this.paint(true);
  };

  Rulebook.prototype.onResize = function (narrow) {
    if (!this.data || narrow === this.narrow || this.busy) return;
    this.rebuild(this.currentKey());
  };

  // --- Painting --------------------------------------------------------------

  Rulebook.prototype.destroyNested = function (root) {
    if (!window.Chronicle || !Chronicle.destroyWidget) return;
    [].forEach.call(root.querySelectorAll('[data-widget]'), function (w) {
      if (w !== root) Chronicle.destroyWidget(w);
    });
  };

  // Widget elements are created only here, on the live page, so a copy of a
  // page used for the turning leaf never carries one.
  Rulebook.prototype.mountNested = function (page) {
    var self = this, cid = this.campaignId;
    [].forEach.call(page.querySelectorAll('[data-ix]'), function (box) { self.fillIndex(box); });
    [].forEach.call(page.querySelectorAll('.rb-wslot'), function (slot) {
      var slug = slot.getAttribute('data-slug');
      if (!SLUG_RE.test(slug || '') || !window.Chronicle || !Chronicle.mountWidgets) return;
      var w = document.createElement('div');
      w.setAttribute('data-widget', slug);
      w.setAttribute('data-campaign-id', cid);
      slot.appendChild(w);
      try { Chronicle.mountWidgets(slot); } catch (e) { /* the page keeps working without the widget */ }
    });
  };

  Rulebook.prototype.setPage = function (el, item, mount) {
    this.destroyNested(el);
    el.innerHTML = item ? this.pageHTML(item) : '';
    el.classList.toggle('blank', !item);
    if (item && mount) this.mountNested(el);
  };

  Rulebook.prototype.paint = function (mount) {
    var sp = this.spreads[this.at], two = !this.narrow;
    this.bookEl.classList.toggle('is-wide', !!(sp && sp.wide));
    this.bookEl.classList.toggle('is-narrow', !two);
    this.pl.classList.toggle('wide', !!(sp && sp.wide));
    this.pr.classList.toggle('gone', !!(sp && sp.wide));
    if (!sp) {
      this.destroyNested(this.pl);
      this.destroyNested(this.pr);
      this.pl.innerHTML = '';
      this.pr.innerHTML = '<div class="rb-scroll"><p class="rb-empty">This book has no pages yet.</p></div>';
    } else if (!two) {
      this.setPage(this.pl, null);
      this.setPage(this.pr, sp.items[0], mount);
    } else {
      this.setPage(this.pl, sp.items[0], mount);
      this.setPage(this.pr, sp.wide ? null : sp.items[1], mount);
    }
    this.syncChrome();
  };

  Rulebook.prototype.syncChrome = function () {
    var sp = this.spreads[this.at], last = this.spreads.length - 1;
    this.curlPrev.hidden = this.at <= 0;
    this.curlNext.hidden = this.at >= last;
    var it = sp && sp.items[sp.items.length - 1];
    var first = sp && sp.items[0];
    var label = '';
    if (it) {
      label = '<b>' + esc(first.ch.title) + '</b>' + (first.kind === 'page' && first.p.title ? ' · ' + esc(first.p.title) : '');
    }
    this.whereEl.innerHTML = label;
    this.syncTabs();
    if (first) {
      this.srEl.textContent = 'Page ' + (first.n + 1) + (it !== first ? ' and ' + (it.n + 1) : '') + ' of ' + this.items.length + ': ' +
        first.ch.title + (first.kind === 'page' ? ', ' + first.p.title : '');
      var place = { key: first.key };
      if (this.data.isDirector) place.view = this.view;
      savePlace(this.campaignId, place);
    }
  };

  Rulebook.prototype.dirTag = function () { return '<span class="rb-dir">Director</span>'; };

  Rulebook.prototype.pageHTML = function (item) {
    var h, label;
    if (item.kind === 'open') {
      var ch = item.ch;
      label = ch.title;
      h = '<p class="rb-kicker">' + esc(item.part.title) + (ch.director ? ' ' + this.dirTag() : '') + '</p>' +
        '<h1>' + esc(ch.title) + '</h1>' +
        (ch.intro ? '<p class="rb-lede">' + esc(ch.intro) + '</p>' : '');
      if (item.pages.length) {
        h += '<ul class="rb-toc">' + item.pages.map(function (p) {
          return '<li><button type="button" data-go="' + p.n + '"><span>' + esc(p.p.title) +
            (p.p.director ? ' ' + '<span class="rb-dir">Director</span>' : '') + '</span><span class="rb-pn">' + (p.n + 1) + '</span></button></li>';
        }).join('') + '</ul>';
      }
    } else {
      label = item.p.title || item.ch.title;
      h = this.pageBodyHTML(item.ch.title, item.p);
    }
    return '<div class="rb-scroll" tabindex="0" role="region" aria-label="Page ' + (item.n + 1) + ': ' + esc(label) + '">' + h + '</div>' +
      '<div class="rb-foot"><span class="rb-folio">' + (item.n + 1) + '</span></div>';
  };

  // The kicker, title and blocks of one page, shared with the page editor's
  // live preview so the preview is the real renderer.
  Rulebook.prototype.pageBodyHTML = function (chapterTitle, p) {
    var self = this;
    return '<p class="rb-kicker">' + esc(chapterTitle) + (p.director ? ' ' + this.dirTag() : '') + '</p>' +
      (p.title ? '<h2>' + esc(p.title) + '</h2>' : '') +
      '<div class="rb-blocks' + (p.columns === true ? ' rb-cols' : '') + '">' + arr(p.blocks).filter(isObj).map(function (b) { return self.blockHTML(b); }).join('') + '</div>';
  };

  // --- Blocks ----------------------------------------------------------------

  Rulebook.prototype.blockHTML = function (b) {
    var dir = this.showDirector(), type = str(b.type), t = this.data.terms;
    if ((b.director === true || type === 'note') && !dir) return '';
    var html = '';
    switch (type) {
      case 'text':
        html = '<div class="rb-prose">' + rich(b.text, t) + '</div>';
        break;
      case 'callout':
        html = '<aside class="rb-callout">' + (str(b.title) ? '<b class="rb-ctitle">' + esc(b.title) + '</b>' : '') +
          '<div class="rb-prose">' + rich(b.text, t) + '</div></aside>';
        break;
      case 'note':
        return '<aside class="rb-gmnote">' + this.dirTag() + '<div class="rb-prose">' + rich(b.text, t) + '</div></aside>';
      case 'roll': html = this.rollHTML(b); break;
      case 'cards': html = this.cardsHTML(b); break;
      case 'flaps': html = this.flapsHTML(b); break;
      case 'links': html = this.linksHTML(b); break;
      case 'example': html = this.exampleHTML(b); break;
      case 'creature': html = this.creatureHTML(b); break;
      case 'index': html = this.indexHTML(b); break;
      case 'widget':
        html = SLUG_RE.test(str(b.widget))
          ? '<div class="rb-wslot" data-slug="' + esc(b.widget) + '"></div>'
          : '<div class="rb-problem" role="note"><span class="rb-problem-ic" aria-hidden="true">!</span><div>This part of the page could not be shown.</div></div>';
        break;
      case 'problem':
        return '<div class="rb-problem" role="note"><span class="rb-problem-ic" aria-hidden="true">!</span><div>' + esc(b.text) + '</div></div>';
      default:
        return '';
    }
    return b.director === true ? '<div class="rb-dirwrap">' + this.dirTag() + html + '</div>' : html;
  };

  function rangeText(bands, i) {
    var max = bands[i].max, prev = i > 0 ? bands[i - 1].max : null;
    if (max === null) return i === 0 ? 'Any result' : (prev + 1) + ' or more';
    return i === 0 ? max + ' or less' : (prev + 1) === max ? String(max) : (prev + 1) + ' to ' + max;
  }

  Rulebook.prototype.rollHTML = function (b) {
    var t = this.data.terms, dice = isObj(b.dice) ? b.dice : {};
    var count = intOr(dice.count, 1, 8, 1), sides = intOr(dice.sides, 2, 1000, 6);
    var bands = arr(b.bands).filter(isObj).map(function (x) {
      return { max: intOr(x.max, -1000000, 1000000, null), label: str(x.label), text: str(x.text) };
    });
    var h = '<div class="rb-roll" data-roll data-count="' + count + '" data-sides="' + sides + '">' +
      '<div class="rb-roll-hd"><b>' + esc(str(b.label) || 'Roll') + '</b><span>' + count + 'd' + sides + '</span></div><div class="rb-row">';
    for (var i = 0; i < count; i++) h += '<span class="rb-die" data-die aria-hidden="true">?</span>';
    if (b.modifier === true) {
      h += '<span class="rb-plus" aria-hidden="true">+</span><label class="rb-bonus"><span>Bonus</span><select class="rb-sel">';
      for (var m = -3; m <= 8; m++) h += '<option value="' + m + '"' + (m === 0 ? ' selected' : '') + '>' + (m > 0 ? '+' : '') + m + '</option>';
      h += '</select></label>';
    }
    h += '<button type="button" class="rb-go" data-act="roll">Roll</button><output class="rb-total" data-total aria-live="polite"></output></div>';
    if (bands.length) {
      h += '<div class="rb-tiers">' + bands.map(function (x, k) {
        return '<div class="rb-tier"' + (x.max === null ? '' : ' data-max="' + x.max + '"') + '><b>' + esc(x.label || 'Result') + '</b><small>' +
          esc(rangeText(bands, k)) + '</small>' + (x.text ? '<span>' + inline(x.text, t) + '</span>' : '') + '</div>';
      }).join('') + '</div>';
    }
    return h + '</div>';
  };

  Rulebook.prototype.cardsHTML = function (b) {
    var t = this.data.terms, items = arr(b.items).filter(isObj);
    if (!items.length) return '';
    return '<div class="rb-cards">' + items.map(function (it) {
      return '<button type="button" class="rb-fc" aria-expanded="false"><b>' + esc(it.title) + '</b>' +
        (str(it.summary) ? '<span>' + inline(it.summary, t) + '</span>' : '') + '</button>';
    }).join('') + items.map(function (it, i) {
      return '<div class="rb-fold" data-i="' + i + '" hidden>' + (str(it.title) ? '<b class="rb-ftitle">' + esc(it.title) + '</b>' : '') + rich(it.text, t) + '</div>';
    }).join('') + '</div>';
  };

  // A links block is a row of "turn to" buttons, each opening a chapter.
  Rulebook.prototype.linksHTML = function (b) {
    // Once the page list is built, a link to a chapter this view does not
    // show (a Director previewing Player view) is left out, not left dead.
    var self = this, t = this.data.terms, items = arr(b.items).filter(function (it) {
      return isObj(it) && SLUG_RE.test(str(it.chapter)) && (!self.items.length || self.firstItemOf(it.chapter) >= 0);
    });
    if (!items.length) return '';
    return '<nav class="rb-links" aria-label="' + esc(str(b.title) || 'Turn to') + '">' +
      (str(b.title) ? '<b class="rb-links-hd">' + esc(b.title) + '</b>' : '') +
      '<div class="rb-links-row">' + items.map(function (it) {
        return '<button type="button" class="rb-link" data-chapter="' + esc(it.chapter) + '"><b>' + esc(it.title) + '</b>' +
          (str(it.summary) ? '<span>' + inline(it.summary, t) + '</span>' : '') + '<i aria-hidden="true">Turn to it \u2192</i></button>';
      }).join('') + '</div></nav>';
  };

  Rulebook.prototype.flapsHTML = function (b) {
    var t = this.data.terms, items = arr(b.items).filter(isObj);
    if (!items.length) return '';
    return '<ul class="rb-flaps">' + items.map(function (it) {
      return '<li class="rb-flap"><button type="button" aria-expanded="false">' + esc(it.title) + '</button>' +
        '<div class="rb-body" hidden>' + rich(it.text, t) + '</div></li>';
    }).join('') + '</ul>';
  };

  Rulebook.prototype.exampleHTML = function (b) {
    var t = this.data.terms, steps = arr(b.steps).map(str).filter(Boolean);
    if (!steps.length) return '';
    return '<div class="rb-play" data-play data-at="-1"><div class="rb-play-hd"><span class="rb-play-t">' + esc(str(b.title) || 'Example') +
      '</span><span class="rb-sp"></span><span class="rb-count" data-count>0 / ' + steps.length + '</span>' +
      '<button type="button" class="rb-ctrl" data-act="prev-step" aria-label="Previous step" disabled>◀</button>' +
      '<button type="button" class="rb-ctrl" data-act="next-step" aria-label="Next step">▶</button></div>' +
      '<ol>' + steps.map(function (s) { return '<li>' + inline(s.replace(/\s*\n\s*/g, ' '), t) + '</li>'; }).join('') + '</ol></div>';
  };

  // Players get a "?" where the numbers would be; a Director previewing the
  // player view sees each stat's label with a "?" so the gap is obvious.
  Rulebook.prototype.creatureHTML = function (b) {
    var t = this.data.terms, dir = this.showDirector();
    var stats = arr(b.stats).filter(isObj);
    var notice = arr(b.notice).map(str).filter(Boolean);
    var h = '<div class="rb-stat"><div class="rb-stat-hd"><b>' + esc(b.name) + '</b>' + (str(b.tagline) ? '<span>' + esc(b.tagline) + '</span>' : '') + '</div>';
    if (str(b.look)) h += '<div class="rb-stat-look rb-prose">' + rich(b.look, t) + '</div>';
    if (notice.length) {
      h += '<div class="rb-stat-seen"><b>What your hero notices</b><ul>' + notice.map(function (n) { return '<li>' + inline(n, t) + '</li>'; }).join('') + '</ul></div>';
    }
    if (dir && stats.length) {
      h += '<div class="rb-grid">' + stats.map(function (s) { return '<div>' + esc(s.label) + '<b>' + esc(s.value) + '</b></div>'; }).join('') + '</div>';
    } else if (this.data.isDirector && stats.length) {
      h += '<div class="rb-grid">' + stats.map(function (s) { return '<div class="rb-hid">' + esc(s.label) + '<b>?</b></div>'; }).join('') + '</div>';
    } else if (b.hiddenStats === true || stats.length) {
      h += '<div class="rb-grid"><div class="rb-hid">Stats<b>?</b></div></div>';
    }
    if (dir && str(b.note)) h += '<div class="rb-stat-note">' + this.dirTag() + '<div class="rb-prose">' + rich(b.note, t) + '</div></div>';
    return h + '</div>';
  };


  // --- Rules index -----------------------------------------------------------
  // A generated page lists one slice of a data category. The entries load
  // when the page is shown (once per slice) and fold down like flaps.

  Rulebook.prototype.indexHTML = function (b) {
    var count = intOr(b.count, 0, 100000, 0);
    var noun = count === 1 ? 'entry' : 'entries';
    return '<div class="rb-ix" data-ix data-cat="' + esc(str(b.category)) + '" data-key="' + esc(str(b.key)) +
      '" data-value="' + esc(str(b.value)) + '" data-from="' + intOr(b.from, 0, 100000, 0) + '" data-to="' + intOr(b.to, 0, 100000, 0) + '">' +
      '<div class="rb-ix-hd"><input type="search" class="rb-ix-filter" data-ix-filter placeholder="Filter ' + count + ' ' + noun + '"' +
      ' aria-label="Filter this page" autocomplete="off" spellcheck="false"><span class="rb-ix-n" aria-live="polite"></span></div>' +
      '<ul class="rb-flaps rb-ix-list" data-ix-list><li class="rb-ix-wait">Opening the entries…</li></ul></div>';
  };

  // A Director previewing the player view asks the server for exactly what a
  // player would get (no gm_only fields, no Director-only chapters).
  Rulebook.prototype.viewParam = function (sep) {
    return this.data && this.data.isDirector && this.view === 'player' ? sep + 'view=player' : '';
  };

  Rulebook.prototype.indexURL = function (box) {
    var base = this.url.replace(/[?#].*$/, '').replace(/\/+$/, '');
    var q = 'key=' + encodeURIComponent(box.getAttribute('data-key') || '') +
      '&value=' + encodeURIComponent(box.getAttribute('data-value') || '') +
      '&from=' + (+box.getAttribute('data-from') || 0) + '&to=' + (+box.getAttribute('data-to') || 0);
    return base + '/index/' + encodeURIComponent(box.getAttribute('data-cat') || '') + '?' + q + this.viewParam('&');
  };

  Rulebook.prototype.fillIndex = function (box) {
    var self = this, url = this.indexURL(box), hit = this.ixCache[url];
    if (hit && hit.items) { this.renderIndex(box, hit.items); return; }
    if (!hit) {
      hit = this.ixCache[url] = { wait: [] };
      Chronicle.apiFetch(url).then(function (res) {
        if (!res.ok) throw new Error('http');
        return res.json();
      }).then(function (json) {
        hit.items = arr(json && json.items).filter(isObj);
      }).catch(function () {
        delete self.ixCache[url];
        hit.failed = true;
      }).then(function () {
        if (self.dead) return;
        hit.wait.forEach(function (b) { if (b.isConnected) self.renderIndex(b, hit.items || null); });
        hit.wait = [];
      });
    }
    hit.wait.push(box);
  };

  Rulebook.prototype.renderIndex = function (box, items) {
    var list = box.querySelector('[data-ix-list]'), t = this.data.terms;
    if (!items) {
      list.innerHTML = '<li class="rb-ix-wait">The entries could not be loaded. Turn away and back to try again.</li>';
      return;
    }
    list.innerHTML = items.map(function (it) {
      var fields = arr(it.fields).filter(isObj);
      var body = (fields.length ? '<dl class="rb-ix-dl">' + fields.map(function (f) {
        return '<div><dt>' + esc(f.label) + '</dt><dd>' + inline(f.value, t) + '</dd></div>';
      }).join('') + '</dl>' : '') + (str(it.text) ? '<div class="rb-prose">' + rich(it.text, t) + '</div>' : '') +
        (!fields.length && !str(it.text) && str(it.summary) ? '<div class="rb-prose"><p>' + esc(it.summary) + '</p></div>' : '');
      return '<li class="rb-flap rb-ix-row" data-id="' + esc(str(it.id)) + '" data-find="' + esc((str(it.name) + ' ' + str(it.summary)).toLowerCase()) + '">' +
        '<button type="button" aria-expanded="false"><span class="rb-ix-t"><b>' + esc(it.name) + '</b>' +
        (str(it.summary) ? '<span>' + esc(it.summary) + '</span>' : '') + '</span></button>' +
        '<div class="rb-body" hidden>' + body + '</div></li>';
    }).join('');
    box.setAttribute('data-ready', '');
    var f = box.querySelector('[data-ix-filter]');
    if (f && f.value) this.filterIndex(f);
    this.openPending(box);
  };

  Rulebook.prototype.filterIndex = function (input) {
    var box = input.closest('[data-ix]'), q = input.value.trim().toLowerCase(), shown = 0, rows = box.querySelectorAll('.rb-ix-row');
    [].forEach.call(rows, function (li) {
      var on = !q || li.getAttribute('data-find').indexOf(q) >= 0;
      li.hidden = !on;
      if (on) shown++;
    });
    box.querySelector('.rb-ix-n').textContent = q ? shown + ' of ' + rows.length : '';
  };

  // Opens the entry a search result asked for once its page is on screen.
  Rulebook.prototype.openPending = function (box) {
    var p = this.pendingEntry;
    if (!p || p.url !== this.indexURL(box)) return;
    this.pendingEntry = '';
    var row = null;
    [].forEach.call(box.querySelectorAll('.rb-ix-row'), function (li) { if (li.getAttribute('data-id') === p.id) row = li; });
    if (!row) return;
    var btn = row.querySelector('button');
    if (!row.classList.contains('open')) this.toggleFlap(btn);
    var scroller = row.closest('.rb-scroll');
    if (scroller) scroller.scrollTop += row.getBoundingClientRect().top - scroller.getBoundingClientRect().top - 24;
    btn.focus({ preventScroll: true });
  };

  // --- Search ----------------------------------------------------------------
  // Pages are searched here, in the book already loaded; rules-index entries
  // are searched by the server, which knows every entry's page.

  Rulebook.prototype.onInput = function (e) {
    var t = e.target;
    if (!t || !t.matches) return;
    if (t.matches('[data-ix-filter]')) { this.filterIndex(t); return; }
    if (t.matches('[data-search]')) {
      var self = this;
      clearTimeout(this.searchTimer);
      this.searchTimer = setTimeout(function () { self.runSearch(t.value); }, 180);
    }
  };

  function blockWords(b) {
    var out = [str(b.title), str(b.text), str(b.label), str(b.name), str(b.tagline), str(b.look)];
    arr(b.items).filter(isObj).forEach(function (it) { out.push(str(it.title), str(it.summary), str(it.text)); });
    arr(b.steps).forEach(function (x) { out.push(str(x)); });
    arr(b.notice).forEach(function (x) { out.push(str(x)); });
    return out.join('\n');
  }

  function plain(s) { return s.replace(/\[\[([^\]|]+)(?:\|([^\]]+))?\]\]/g, function (_, a, b) { return b || a; }).replace(/\*+/g, ''); }

  Rulebook.prototype.findPages = function (q) {
    var dir = this.showDirector(), hits = [];
    this.items.forEach(function (it) {
      if (it.kind !== 'page' || it.ch.generated || hits.length >= 8) return;
      var title = str(it.p.title), words = plain(arr(it.p.blocks).filter(function (b) {
        return isObj(b) && (dir || (b.director !== true && b.type !== 'note'));
      }).map(blockWords).join('\n'));
      var inTitle = title.toLowerCase().indexOf(q) >= 0, at = words.toLowerCase().indexOf(q);
      if (!inTitle && at < 0) return;
      var snip = '';
      if (at >= 0) {
        var from = Math.max(0, at - 40);
        snip = (from > 0 ? '…' : '') + words.slice(from, at + q.length + 60).replace(/\s+/g, ' ').trim() + '…';
      }
      hits.push({ n: it.n, title: title || it.ch.title, where: it.ch.title, snip: snip, rank: inTitle ? 0 : 1 });
    });
    return hits.sort(function (a, b) { return a.rank - b.rank; });
  };

  Rulebook.prototype.runSearch = function (raw) {
    var self = this, q = str(raw).trim().toLowerCase(), seq = ++this.searchSeq;
    if (q.length < 2) { this.closeResults(); return; }
    var pages = this.findPages(q);
    this.showResults(q, pages, null);
    if (!this.items.some(function (it) { return it.ch.generated; })) { this.showResults(q, pages, []); return; }
    var base = this.url.replace(/[?#].*$/, '').replace(/\/+$/, '');
    Chronicle.apiFetch(base + '/find?q=' + encodeURIComponent(q) + this.viewParam('&')).then(function (res) {
      if (!res.ok) throw new Error('http');
      return res.json();
    }).then(function (json) {
      if (self.dead || seq !== self.searchSeq) return;
      self.showResults(q, pages, arr(json && json.results).filter(isObj));
    }).catch(function () {
      if (self.dead || seq !== self.searchSeq) return;
      self.showResults(q, pages, []);
    });
  };

  // Where a rules-index result lives: the item showing that chapter's page.
  Rulebook.prototype.itemFor = function (chapterId, page) {
    for (var i = 0; i < this.items.length; i++) {
      var it = this.items[i];
      if (it.kind === 'page' && it.ch.id === chapterId && it.ch.pages[page] === it.p) return it;
    }
    return null;
  };

  Rulebook.prototype.showResults = function (q, pages, entries) {
    var self = this, h = '';
    if (pages.length) {
      h += '<h3>In the book</h3><ul>' + pages.map(function (r) {
        return '<li><button type="button" data-hit="' + r.n + '"><b>' + esc(r.title) + '</b><small>' + esc(r.where) + '</small>' +
          (r.snip ? '<span>' + esc(r.snip) + '</span>' : '') + '</button></li>';
      }).join('') + '</ul>';
    }
    var found = (entries || []).map(function (r) { return { r: r, it: self.itemFor(str(r.chapter), intOr(r.page, 0, 100000, -1)) }; })
      .filter(function (x) { return x.it; });
    if (found.length) {
      h += '<h3>Rules index</h3><ul>' + found.map(function (x) {
        return '<li><button type="button" data-hit="' + x.it.n + '" data-entry="' + esc(str(x.r.id)) + '"><b>' + esc(x.r.name) + '</b>' +
          '<small>' + esc(x.r.where) + '</small>' + (str(x.r.summary) ? '<span>' + esc(x.r.summary) + '</span>' : '') + '</button></li>';
      }).join('') + '</ul>';
    }
    if (!h) h = entries === null ? '<p class="rb-results-note">Searching…</p>' : '<p class="rb-results-note">Nothing in this book matches “' + esc(q) + '”.</p>';
    this.resultsEl.innerHTML = h;
    this.resultsEl.hidden = false;
    this.searchEl.setAttribute('aria-expanded', 'true');
  };

  Rulebook.prototype.closeResults = function () {
    if (!this.resultsEl || this.resultsEl.hidden) return;
    this.resultsEl.hidden = true;
    this.resultsEl.innerHTML = '';
    this.searchEl.setAttribute('aria-expanded', 'false');
  };

  // Keys inside the search box and its results: Enter takes the first
  // result, the arrows move through them, Escape closes them.
  Rulebook.prototype.searchKey = function (e, t) {
    if (!this.resultsEl || !t.closest('.rb-search')) return false;
    var hits = [].slice.call(this.resultsEl.querySelectorAll('[data-hit]')), i = hits.indexOf(t);
    if (e.key === 'Escape') {
      e.preventDefault();
      if (!this.resultsEl.hidden) { this.closeResults(); this.searchEl.focus(); }
      else { this.searchEl.value = ''; this.searchEl.blur(); }
      return true;
    }
    if (this.resultsEl.hidden || !hits.length) return false;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      var next = e.key === 'ArrowDown' ? i + 1 : i - 1;
      if (next < 0) this.searchEl.focus(); else hits[Math.min(next, hits.length - 1)].focus();
      return true;
    }
    if (e.key === 'Enter' && t === this.searchEl) { e.preventDefault(); this.pickResult(hits[0]); return true; }
    return false;
  };

  Rulebook.prototype.pickResult = function (btn) {
    var n = +btn.getAttribute('data-hit'), id = btn.getAttribute('data-entry') || '';
    this.closeResults();
    this.pendingEntry = '';
    var it = this.items[n], blk = it && it.kind === 'page' && it.p.blocks[0];
    if (id && isObj(blk) && blk.type === 'index') {
      // The entry opens only on the page that lists it, once its rows load.
      var probe = document.createElement('div');
      probe.innerHTML = this.indexHTML(blk);
      this.pendingEntry = { id: id, url: this.indexURL(probe.firstChild) };
    }
    var sp = this.spreadOf(n);
    if (sp === this.at) {
      var self = this;
      [].forEach.call(this.el.querySelectorAll('.rb-page [data-ix][data-ready]'), function (box) { self.openPending(box); });
      if (!id) this.goItem(n);
    } else {
      this.goItem(n);
    }
  };

  // --- Turning pages ---------------------------------------------------------

  // A copy of a page for the leaf: widgets stay out of it and the scroll
  // position is carried over so the page does not jump as it lifts.
  Rulebook.prototype.snapshot = function (el) {
    var c = el.cloneNode(true), s = el.querySelector('.rb-scroll');
    [].forEach.call(c.querySelectorAll('[data-widget]'), function (w) { w.removeAttribute('data-widget'); });
    c._st = s ? s.scrollTop : 0;
    return c;
  };

  Rulebook.prototype.makePage = function (item, side) {
    var a = document.createElement('article');
    a.className = 'rb-page ' + side + (item ? '' : ' blank');
    if (item) a.innerHTML = this.pageHTML(item);
    return a;
  };

  Rulebook.prototype.settled = function () {
    if (!this.refocus) return;
    this.refocus = false;
    var s = this.bookEl.querySelector('.rb-page:not(.blank):not(.gone) .rb-scroll');
    if (s) s.focus({ preventScroll: true });
  };

  Rulebook.prototype.turnTo = function (ns) {
    var last = this.spreads.length - 1, self = this;
    ns = clamp(ns, 0, last);
    if (this.busy || last < 0 || ns === this.at) return;
    var oldSp = this.spreads[this.at], newSp = this.spreads[ns], fwd = ns > this.at;
    this.hideCard();
    if (reduced()) { this.at = ns; this.paint(true); this.settled(); return; }

    var style = this.data.turn, two = !this.narrow;
    if (style === 'flip' && (oldSp.wide || newSp.wide)) style = 'slide';
    var leaf = document.createElement('div'), face, b;
    leaf.className = 'rb-leaf ' + style + (style === 'fade' ? '' : (fwd ? ' fwd' : ' back'));
    leaf.setAttribute('aria-hidden', 'true');
    leaf.setAttribute('inert', '');

    if (style === 'flip') {
      face = document.createElement('div');
      face.className = 'rb-face';
      face.appendChild(this.snapshot(fwd || !two ? this.pr : this.pl));
      face.insertAdjacentHTML('beforeend', '<div class="rb-shade"></div>');
      b = document.createElement('div');
      b.className = 'rb-face b';
      b.appendChild(this.makePage(two ? (fwd ? newSp.items[0] : newSp.items[1]) : null, two && fwd ? 'l' : 'r'));
      leaf.appendChild(face);
      leaf.appendChild(b);
      this.at = ns;
      // The page the leaf uncovers already shows where the reader is going.
      if (fwd || !two) this.setPage(this.pr, two ? newSp.items[1] : newSp.items[0], false);
      else this.setPage(this.pl, newSp.items[0], false);
      this.syncChrome();
    } else {
      face = document.createElement('div');
      face.className = 'rb-face';
      face.appendChild(this.snapshot(this.pl));
      face.appendChild(this.snapshot(this.pr));
      leaf.appendChild(face);
      this.at = ns;
      this.paint(false);
    }

    this.busy = true;
    this.bookEl.classList.add('is-turning');
    this.bookEl.appendChild(leaf);
    [].forEach.call(leaf.querySelectorAll('.rb-page'), function (p) {
      var s = p.querySelector('.rb-scroll');
      if (s && p._st) s.scrollTop = p._st;
    });

    var done = false;
    function finish() {
      if (done) return;
      done = true;
      clearTimeout(self.leafTimer);
      self.finishNow = null;
      leaf.remove();
      self.bookEl.classList.remove('is-turning');
      self.busy = false;
      self.paint(true);
      self.settled();
    }
    this.finishNow = finish;
    leaf.addEventListener('animationend', function (e) { if (e.target === leaf) finish(); });
    this.leafTimer = setTimeout(finish, TURN_MS[this.data.turn] + 300);
    requestAnimationFrame(function () { leaf.classList.add('go'); });
  };

  Rulebook.prototype.goItem = function (n) {
    this.closeDrawer(false);
    this.refocus = true;
    this.turnTo(this.spreadOf(n));
    if (!this.busy) this.settled();
  };

  // --- Contents drawer -------------------------------------------------------

  Rulebook.prototype.drawerHTML = function () {
    var self = this, h = '', lastPart = null, sp = this.spreads[this.at];
    var cur = {};
    if (sp) sp.items.forEach(function (it) { cur[it.n] = true; });
    var curCh = sp ? sp.items[0].ch : null;
    this.items.forEach(function (it) {
      // A solo chapter (one wide page, no opener) is listed as a chapter.
      if (it.kind === 'open' || it.solo) {
        if (it.part !== lastPart) {
          lastPart = it.part;
          if (it.part.title) h += '<h3>' + esc(it.part.title) + (it.part.director ? ' ' + self.dirTag() : '') + '</h3>';
        }
        h += '<button type="button" class="rb-dch" data-go="' + it.n + '"' + (it.ch === curCh ? ' aria-current="true"' : '') + '><span>' +
          esc(it.ch.title) + (it.ch.director && !it.part.director ? ' ' + self.dirTag() : '') + '</span><span class="rb-pn">' + (it.n + 1) + '</span></button>';
      } else {
        h += '<button type="button" class="rb-dpg" data-go="' + it.n + '"' + (cur[it.n] ? ' aria-current="true"' : '') + '><span>' +
          esc(it.p.title || 'Untitled page') + (it.p.director && !it.ch.director ? ' ' + self.dirTag() : '') + '</span><span class="rb-pn">' + (it.n + 1) + '</span></button>';
      }
    });
    return h || '<p class="rb-empty">This book has no pages yet.</p>';
  };

  Rulebook.prototype.drawerOpen = function () { return this.drawerEl && this.drawerEl.classList.contains('open'); };

  Rulebook.prototype.openDrawer = function () {
    this.hideCard();
    this.drawerEl.innerHTML = this.drawerHTML();
    this.drawerEl.inert = false;
    this.drawerEl.classList.add('open');
    this.scrimEl.classList.add('open');
    this.tocBtn.setAttribute('aria-expanded', 'true');
    this.tocBtn.setAttribute('aria-label', 'Close the contents');
    var target = this.drawerEl.querySelector('[aria-current="true"]') || this.drawerEl.querySelector('button');
    if (target) requestAnimationFrame(function () { target.focus({ preventScroll: false }); });
  };

  Rulebook.prototype.closeDrawer = function (restoreFocus) {
    if (!this.drawerOpen()) return;
    var hadFocus = this.drawerEl.contains(document.activeElement);
    this.drawerEl.classList.remove('open');
    this.scrimEl.classList.remove('open');
    this.drawerEl.inert = true;
    this.tocBtn.setAttribute('aria-expanded', 'false');
    this.tocBtn.setAttribute('aria-label', 'Open the contents');
    if (restoreFocus && hadFocus) this.tocBtn.focus();
  };

  // --- Hover terms -----------------------------------------------------------

  Rulebook.prototype.showCard = function (t) {
    var key = t.getAttribute('data-term'), def = has(this.data.terms, key) ? this.data.terms[key] : null;
    if (!def) return;
    var hc = this.cardEl;
    if (this.card && this.card !== t) this.card.removeAttribute('aria-describedby');
    hc.innerHTML = '<b>' + esc(def.name) + '</b>' + esc(def.text);
    hc.id = hc.id || 'rb-hc-' + (++uid);
    t.setAttribute('aria-describedby', hc.id);
    hc.hidden = false;
    hc.style.left = '0px';
    hc.style.top = '0px';
    var r = t.getBoundingClientRect(), w = hc.offsetWidth, h = hc.offsetHeight;
    var x = clamp(r.left, 8, Math.max(8, window.innerWidth - w - 8)), y = r.bottom + 6;
    if (y + h > window.innerHeight - 8 && r.top - h - 6 > 8) y = r.top - h - 6;
    hc.style.left = Math.round(x) + 'px';
    hc.style.top = Math.round(y) + 'px';
    this.card = t;
  };

  Rulebook.prototype.hideCard = function () {
    if (this.card) this.card.removeAttribute('aria-describedby');
    if (this.cardEl) this.cardEl.hidden = true;
    this.card = null;
    this.pinned = false;
  };

  Rulebook.prototype.togglePin = function (t) {
    if (this.card === t && this.pinned) { this.hideCard(); return; }
    this.showCard(t);
    this.pinned = this.card === t;
  };

  Rulebook.prototype.onOver = function (e) {
    if (!this.data || this.pinned) return;
    var t = e.target.closest && e.target.closest('.rb-term');
    if (t) { if (t !== this.card) this.showCard(t); }
    else if (this.card) this.hideCard();
  };

  Rulebook.prototype.onFocus = function (e, inn) {
    if (!this.data || this.pinned) return;
    var t = e.target.classList && e.target.classList.contains('rb-term') ? e.target : null;
    if (inn && t) this.showCard(t);
    else if (!inn && t) this.hideCard();
  };

  // --- Block behaviour -------------------------------------------------------

  Rulebook.prototype.doRoll = function (box) {
    var self = this;
    var count = +box.getAttribute('data-count'), sides = +box.getAttribute('data-sides');
    var dice = box.querySelectorAll('[data-die]'), sel = box.querySelector('select');
    var go = box.querySelector('[data-act="roll"]'), out = box.querySelector('[data-total]');
    var bonus = sel ? +sel.value : 0, vals = [], sum = 0, i;
    for (i = 0; i < count; i++) { vals.push(rnd(sides)); sum += vals[i]; }
    var total = sum + bonus;

    function settle() {
      for (i = 0; i < dice.length; i++) dice[i].textContent = vals[i];
      out.textContent = '= ' + total;
      var hit = null;
      [].forEach.call(box.querySelectorAll('.rb-tier'), function (tier) {
        var max = tier.getAttribute('data-max');
        var on = !hit && (max === null || total <= +max);
        if (on) hit = tier;
        tier.classList.toggle('hit', on);
        if (on) tier.setAttribute('aria-current', 'true'); else tier.removeAttribute('aria-current');
      });
      go.disabled = false;
    }

    if (reduced()) { settle(); return; }
    go.disabled = true;
    [].forEach.call(dice, function (d) { d.classList.remove('spin'); void d.offsetWidth; d.classList.add('spin'); });
    var ticks = 0;
    var iv = setInterval(function () {
      if (++ticks > 6) {
        clearInterval(iv);
        self.timers = self.timers.filter(function (x) { return x !== iv; });
        settle();
        return;
      }
      for (var k = 0; k < dice.length; k++) dice[k].textContent = rnd(sides);
    }, 60);
    this.timers.push(iv);
  };

  // The panel lands under the row the card is in, spanning the whole grid.
  Rulebook.prototype.foldCard = function (btn) {
    var grid = btn.parentNode, open = btn.getAttribute('aria-expanded') === 'true';
    var cards = [].slice.call(grid.querySelectorAll('.rb-fc'));
    [].forEach.call(grid.querySelectorAll('.rb-fold'), function (f) { f.hidden = true; });
    cards.forEach(function (c) { c.setAttribute('aria-expanded', 'false'); });
    if (open) return;
    var idx = cards.indexOf(btn), panel = grid.querySelector('.rb-fold[data-i="' + idx + '"]');
    if (!panel) return;
    var cols = Math.max(1, getComputedStyle(grid).gridTemplateColumns.split(' ').length);
    var after = cards[Math.min(cards.length - 1, Math.floor(idx / cols) * cols + cols - 1)];
    after.insertAdjacentElement('afterend', panel);
    panel.hidden = false;
    btn.setAttribute('aria-expanded', 'true');
  };

  Rulebook.prototype.toggleFlap = function (btn) {
    var li = btn.parentNode, open = !li.classList.contains('open');
    li.classList.toggle('open', open);
    btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    var body = li.querySelector('.rb-body');
    if (body) body.hidden = !open;
  };

  Rulebook.prototype.stepExample = function (btn, d) {
    var box = btn.closest('[data-play]'), lis = box.querySelectorAll('li');
    var at = clamp(+box.getAttribute('data-at') + d, -1, lis.length - 1);
    box.setAttribute('data-at', String(at));
    [].forEach.call(lis, function (li, i) {
      li.classList.toggle('on', i < at);
      li.classList.toggle('now', i === at);
    });
    box.querySelector('[data-count]').textContent = (at + 1) + ' / ' + lis.length;
    var prev = box.querySelector('[data-act="prev-step"]'), next = box.querySelector('[data-act="next-step"]');
    var wasFocused = document.activeElement;
    prev.disabled = at <= -1;
    next.disabled = at >= lis.length - 1;
    if (wasFocused && wasFocused.disabled) (wasFocused === prev ? next : prev).focus();
  };

  // --- Input -----------------------------------------------------------------

  Rulebook.prototype.setView = function (v) {
    if (!this.data.isDirector || v === this.view || (v !== 'player' && v !== 'director')) return;
    if (this.finishNow) this.finishNow();
    var key = this.currentKey();
    this.view = v;
    this.closeResults();
    this.rebuild(key);
  };

  Rulebook.prototype.onClick = function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    if (!this.el.contains(t)) { if (this.card) this.hideCard(); this.closeResults(); return; }
    var x;
    if ((x = t.closest('.rb-term'))) { this.togglePin(x); return; }
    if (this.card) this.hideCard();
    if (!this.data) {
      if (t.closest('[data-act="retry"]')) this.load();
      return;
    }
    if ((x = t.closest('.rb-results [data-hit]'))) { this.pickResult(x); return; }
    if (!t.closest('.rb-search')) this.closeResults();
    if ((x = t.closest('[data-go]'))) { this.goItem(+x.getAttribute('data-go')); return; }
    if ((x = t.closest('[data-tab]'))) { this.goChapter(x.getAttribute('data-tab')); return; }
    if ((x = t.closest('[data-chapter]'))) { this.goChapter(x.getAttribute('data-chapter')); return; }
    if ((x = t.closest('[data-act]'))) {
      switch (x.getAttribute('data-act')) {
        case 'prev': this.turnTo(this.at - 1); break;
        case 'next': this.turnTo(this.at + 1); break;
        case 'edit': window.location.assign(x.getAttribute('data-href')); break;
        case 'toc': if (this.drawerOpen()) this.closeDrawer(true); else this.openDrawer(); break;
        case 'view': this.setView(x.getAttribute('data-view')); break;
        case 'roll': this.doRoll(x.closest('[data-roll]')); break;
        case 'prev-step': this.stepExample(x, -1); break;
        case 'next-step': this.stepExample(x, 1); break;
      }
      return;
    }
    if ((x = t.closest('.rb-fc'))) { this.foldCard(x); return; }
    if ((x = t.closest('.rb-flap > button'))) { this.toggleFlap(x); return; }
    if (t.closest('.rb-scrim')) this.closeDrawer(true);
  };

  Rulebook.prototype.onKey = function (e) {
    if (!this.data || !this.el.isConnected) return;
    var t = e.target;
    if (t && t.closest) {
      if (t.classList.contains('rb-term') && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); this.togglePin(t); return; }
      if (this.searchKey(e, t)) return;
      if (t.closest('input,select,textarea,[contenteditable="true"]')) return;
      if (t.closest('.rb-wslot') && e.key !== 'Escape') return;
    }
    if (e.key === 'Escape') { this.closeDrawer(true); this.hideCard(); this.closeResults(); return; }
    if (e.key === '/' && !e.altKey && !e.ctrlKey && !e.metaKey && this.searchEl) { e.preventDefault(); this.searchEl.focus(); this.searchEl.select(); return; }
    if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    if (e.key === 'ArrowRight') { this.turnTo(this.at + 1); e.preventDefault(); }
    else if (e.key === 'ArrowLeft') { this.turnTo(this.at - 1); e.preventDefault(); }
  };

  Rulebook.prototype.onTouch = function (e, start) {
    var p = e.changedTouches && e.changedTouches[0];
    if (!p || !this.data) return;
    if (start) {
      var t = e.target;
      var ok = t && t.closest && t.closest('.rb-book') && !t.closest('.rb-drawer, .rb-wslot, .rb-roll, select');
      this.touch = ok ? { x: p.clientX, y: p.clientY } : null;
      return;
    }
    var s = this.touch;
    this.touch = null;
    if (!s || this.drawerOpen()) return;
    var dx = p.clientX - s.x, dy = p.clientY - s.y;
    if (Math.abs(dx) >= SWIPE_PX && Math.abs(dy) < Math.abs(dx) * 0.6) this.turnTo(this.at + (dx < 0 ? 1 : -1));
  };

  // Reuse by the page editor: draws one wire-format page (the same shape the
  // book JSON carries) with the book's own block renderers. ctx: { terms,
  // director } where director=false previews the player view. The result is
  // static markup; widget blocks are not mounted.
  window.ChronicleRulebook = {
    renderPage: function (page, ctx) {
      ctx = ctx || {};
      var r = new Rulebook(document.createElement('div'), {});
      var terms = {};
      if (isObj(ctx.terms)) {
        Object.keys(ctx.terms).forEach(function (k) {
          var t = ctx.terms[k];
          if (isObj(t) && str(t.text)) terms[k.toLowerCase()] = { name: str(t.name) || k, text: str(t.text) };
        });
      }
      r.data = { terms: terms, isDirector: true };
      r.view = ctx.director === false ? 'player' : 'director';
      return r.pageBodyHTML(str(ctx.chapterTitle), isObj(page) ? page : {});
    },
    // Sets the --book-* theme variables on el, through the same allow-list.
    applyTheme: function (el, theme) {
      var r = new Rulebook(el, {});
      r.data = { turn: 'flip' };
      r.applyTheme(isObj(theme) ? theme : {});
    }
  };

  Chronicle.register('rulebook', {
    init: function (el, config) {
      var rb = new Rulebook(el, config || {});
      instances.set(el, rb);
      rb.start();
    },
    destroy: function (el) {
      var rb = instances.get(el);
      if (rb) { rb.stop(); instances.delete(el); }
    }
  });
})();
