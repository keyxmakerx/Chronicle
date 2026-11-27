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
 * Mount: <div data-widget="rulebook" data-campaign-id data-book-url>, or
 * data-book-json for a book shown inside another page (a draft under review),
 * which keeps no place and only answers keys pressed inside it.
 *
 * A leaf is a third mode: data-leaf="<chapter id>" draws only that chapter's
 * pages, one at a time, torn out of the book and set in another page (the
 * campaign home's house rules). It has no bar, tabs, cover, search or saved
 * place, listens to nothing outside itself, and waits for the fold-out it sits
 * in to open before it asks for the book. data-open-href is the link that
 * opens the full book at the chapter; data-leaf-preview adds a hover preview
 * to the fold-out's row.
 */
(function () {
  'use strict';
  function gmTerm() { return (window.Chronicle && Chronicle.gmTerm) ? Chronicle.gmTerm() : 'GM'; }


  var PLACE_KEY = 'chronicle.rulebook.place.';
  var NARROW_QUERY = '(max-width: 900px)';
  // The screen folds away in this long; a card list this long gets a find box.
  var SCREEN_CLOSE_MS = 380;
  var FILTER_AT = 8;
  var TURN_MS = { flip: 780, slide: 520, fade: 380 };
  var SWIPE_PX = 60;
  // The cover swings open this long; the Sections ribbon rolls up or down in
  // BM_MS, and a page is only turned to once the ribbon has rolled back up.
  var FRONT_MS = 1300;
  var BM_MS = 560;
  var LOOKS = { 'paper': 1, 'bold': 1 };
  // The bold look gives each chapter its own colour, in the order of the
  // signed demo, so chapters are told apart at a glance. Each holds 4.5:1
  // with the white numbers and buttons set on it.
  var CHAPTER_HUES = ['#1d7f80', '#c22f39', '#5a3a98', '#2f7a45', '#b45b1c', '#8e1f2b', '#4b5f86', '#7a4d8f',
    '#1d7f80', '#c22f39', '#a5620c', '#5a3a98', '#2f7a45', '#b45b1c'];
  // Cover and font addresses are same-origin paths the server wrote; anything
  // else (another host, a scheme, a path climbing out) is not used.
  var ASSET_RE = /^\/(?!\/)[A-Za-z0-9._~\/%-]{1,300}$/;
  var MAX_FONTS = 8;

  var THEME_COLOURS = { 'paper': 1, 'paper-alt': 1, 'ink': 1, 'ink-soft': 1, 'muted': 1, 'edge': 1, 'accent': 1, 'cover': 1 };
  var THEME_FONTS = { 'heading-font': 1, 'body-font': 1, 'ui-font': 1 };
  var COLOUR_RE = /^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;
  var FONT_RE = /^[A-Za-z0-9 ,'"._-]{1,200}$/;
  var SLUG_RE = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/;
  // House chapters a campaign writes have ids with this prefix; a package's
  // chapter ids cannot contain _, so the two never clash.
  var HOUSE_PREFIX = 'house_';

  var instances = new WeakMap();
  var uid = 0;
  // Leaves on one page ask for the same book once. A failed request is not
  // kept, so the next leaf to open tries again.
  var bookRequests = {};
  // A preview excerpt stays short enough to read at a glance.
  var PREVIEW_CHARS = 220;

  // --- Small helpers ---------------------------------------------------------

  function esc(s) { return Chronicle.escapeHtml(s == null ? '' : String(s)); }
  function str(v) { return typeof v === 'string' ? v : (typeof v === 'number' ? String(v) : ''); }
  function arr(v) { return Array.isArray(v) ? v : []; }
  function isObj(v) { return !!v && typeof v === 'object' && !Array.isArray(v); }
  function has(o, k) { return Object.prototype.hasOwnProperty.call(o, k); }

  // getBook fetches and parses a book's JSON; the error carries the HTTP
  // status or an unreadable flag for errorMessage.
  function getBook(url) {
    return Chronicle.apiFetch(url).then(function (res) {
      if (!res.ok) { var err = new Error('http'); err.status = res.status; throw err; }
      return res.json().catch(function () { var e = new Error('json'); e.unreadable = true; throw e; });
    });
  }

  function sharedBook(url) {
    if (!has(bookRequests, url)) {
      bookRequests[url] = getBook(url).catch(function (err) { delete bookRequests[url]; throw err; });
    }
    return bookRequests[url];
  }

  function roman(n) {
    var out = '', vals = [[1000, 'M'], [900, 'CM'], [500, 'D'], [400, 'CD'], [100, 'C'], [90, 'XC'], [50, 'L'], [40, 'XL'], [10, 'X'], [9, 'IX'], [5, 'V'], [4, 'IV'], [1, 'I']];
    vals.forEach(function (v) { while (n >= v[0]) { out += v[1]; n -= v[0]; } });
    return out;
  }
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
    // [[@category/id|shown]] names one rules-index entry, which opens in
    // place; the server checks that the reader's book lists it.
    s = s.replace(/\[\[@([a-z0-9_-]{1,64})\/([A-Za-z0-9_.-]{1,128})\|([^\[\]\n]+?)\]\]/g, function (_, cat, id, shown) {
      slots.push('<span class="rb-term rb-ref" role="button" tabindex="0" data-ref-cat="' + esc(cat) + '" data-ref="' + esc(id) + '">' + esc(shown.trim()) + '</span>');
      return '\u0000' + (slots.length - 1) + '\u0000';
    });
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
      look: typeof json.look === 'string' && has(LOOKS, json.look) ? json.look : '',
      cover: assetURL(json.cover),
      fonts: readFonts(json.fonts),
      tabs: arr(json.tabs).filter(function (id) { return typeof id === 'string' && SLUG_RE.test(id); }),
      start: SLUG_RE.test(str(json.start)) ? str(json.start) : '',
      first: SLUG_RE.test(str(json.first)) ? str(json.first) : '',
      screen: SLUG_RE.test(str(json.screen)) ? str(json.screen) : '',
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

  // The gold and red of a book's text and thin lines come in a bright and a
  // deep shade; the one that reads better on the book's paper is used, so a
  // light paper never gets the bright shade meant for a dark one.
  function lumOf(hex) {
    var h = hex.slice(1);
    if (h.length < 6) h = h.charAt(0) + h.charAt(0) + h.charAt(1) + h.charAt(1) + h.charAt(2) + h.charAt(2);
    var c = [0, 2, 4].map(function (i) {
      var v = parseInt(h.substr(i, 2), 16) / 255;
      return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
    });
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
  }
  function contrastOf(a, b) {
    var x = lumOf(a), y = lumOf(b);
    return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
  }
  function readableOn(paper, bright, deep) {
    return contrastOf(deep, paper) > contrastOf(bright, paper) ? deep : bright;
  }

  // hueStyle hands a chapter's colour to the bold look's accent parts.
  function hueStyle(hue) { return hue ? ' style="--rb-hot:' + hue + '"' : ''; }

  function isContentsPage(p) {
    return p.blocks.length === 1 && str(p.blocks[0].type) === 'contents';
  }

  function assetURL(v) {
    var u = str(v);
    return ASSET_RE.test(u) && u.indexOf('..') < 0 ? u : '';
  }

  function readFonts(list) {
    var out = [];
    arr(list).filter(isObj).slice(0, MAX_FONTS).forEach(function (f) {
      var family = str(f.family).replace(/["',\\]/g, '').trim().slice(0, 100), url = assetURL(f.url);
      if (!family || !url) return;
      out.push({
        family: family, url: url,
        weight: intOr(f.weight, 100, 900, 400),
        style: f.style === 'italic' ? 'italic' : 'normal'
      });
    });
    return out;
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
    // "Before you play in <campaign>", for every reader of the campaign's own book.
    this.beforeUrl = str(config.beforeUrl);
    this.campaignName = str(config.campaignName);
    // data-book-json carries the book itself (a draft shown for review
    // before it is saved); such a book has no address to search or edit.
    this.inline = str(config.bookJson);
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
    this.leafTimer = 0;
    this.touch = null;
    this.handlers = [];
    this.finishNow = null;
    this.refocus = false;
    this.ixCache = {};
    this.pendingEntry = '';
    this.searchSeq = 0;
    this.searchTimer = 0;
    this.screenTimer = 0;
    this.screenPages = [];
    this.screenAtStart = false;
    this.deepLink = false;
    this.frontShut = false;
    this.frontTimer = 0;
    this.faces = [];
    // Leaf mode: the one chapter drawn, where it opens the full book, and
    // whether its fold-out row previews it on hover.
    this.leafId = str(config.leaf);
    this.openHref = str(config.openHref);
    this.leafPreview = config.leafPreview === true || config.leafPreview === 'true';
    this.leafItems = [];
    this.leafAt = 0;
    this.leafStarted = false;
    // The spread a jump left, offered as "Back to p. N" until the reader
    // turns a page by hand or uses it.
    this.backTo = null;
    this.pendingBack = null;
    // The address follows the book only once the reader has turned a page.
    this.addrOn = false;
    this.pendingPush = null;
    this.pushTimer = 0;
    this.releaseDrawer = null;
    this.releaseScreen = null;
    this.inst = ++uid;
  }

  Rulebook.prototype.domId = function (name) { return 'rb' + this.inst + '-' + name; };

  Rulebook.prototype.on = function (target, type, fn, opts) {
    target.addEventListener(type, fn, opts);
    this.handlers.push([target, type, fn, opts]);
  };

  Rulebook.prototype.start = function () {
    var self = this;
    if (this.leafId) { this.startLeaf(); return; }
    this.el.classList.add('rb');
    if (this.inline) this.el.classList.add('rb-inline');
    this.on(document, 'click', function (e) { self.onClick(e); });
    this.on(document, 'keydown', function (e) { self.onKey(e); });
    // Capture phase, so a turn back inside the book is handled before htmx
    // sees the entry as a page to restore.
    this.basePath = window.location.pathname + window.location.search;
    this.on(window, 'popstate', function (e) { self.onPop(e); }, true);
    this.on(document, 'rulebook:goto', function (e) { self.onGoto(e); });
    // Rule words open Chronicle's shared hover card, in the campaign's look.
    if (window.Chronicle && Chronicle.hovercard) {
      this.unbindCard = Chronicle.hovercard.bind(this.el, '.rb-term', function (t) { return self.termCard(t); }, { pinOnClick: true });
    }
    // The peek card lives outside the book; its own controls answer here.
    this.on(document, 'click', function (e) { self.onCardClick(e); }, true);
    this.on(document, 'keydown', function (e) {
      if ((e.key === 'Enter' || e.key === ' ') && e.target && e.target.closest && e.target.closest('.chc .rb-peek .rb-term')) self.onCardClick(e);
    }, true);
    this.on(this.el, 'input', function (e) { self.onInput(e); });
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
    clearTimeout(this.pushTimer);
    this.letGoBack('releaseDrawer');
    this.letGoBack('releaseScreen');
    this.handlers.forEach(function (h) {
      if (h[1] === null) h[0].removeListener(h[2]);
      else h[0].removeEventListener(h[1], h[2], h[3]);
    });
    this.handlers = [];
    if (this.unbindCard) { this.unbindCard(); this.unbindCard = null; }
    if (this.unbindPreview) { this.unbindPreview(); this.unbindPreview = null; }
    if (this.ro) { this.ro.disconnect(); this.ro = null; }
    this.timers.forEach(function (t) { clearInterval(t); clearTimeout(t); });
    this.timers = [];
    clearTimeout(this.leafTimer);
    clearTimeout(this.searchTimer);
    clearTimeout(this.screenTimer);
    clearTimeout(this.frontTimer);
    this.unloadFonts();
    this.destroyNested(this.el);
    this.el.innerHTML = '';
    this.el.classList.remove('rb', 'rb-inline', 'rb-closed', 'rb-torn');
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
    if (this.inline) {
      var parsed = null;
      try { parsed = normalize(JSON.parse(this.inline)); } catch (e) { parsed = null; }
      if (parsed) this.open(parsed);
      else this.showState('error', errorMessage({ unreadable: true }));
      return;
    }
    if (!this.url) { this.showState('error', 'This page has no book address.'); return; }
    this.showState('loading');
    (this.leafId ? sharedBook(this.url) : getBook(this.url)).then(function (json) {
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
    if (typeof theme.paper === 'string' && COLOUR_RE.test(theme.paper)) {
      style.setProperty('--rb-gold-ink', readableOn(theme.paper, '#fbbf24', '#8a5a00'));
      style.setProperty('--rb-bad-ink', readableOn(theme.paper, '#f87171', '#a3271b'));
    }
    this.el.setAttribute('data-turn', this.data.turn);
    if (this.data.look) this.el.setAttribute('data-look', this.data.look);
    else this.el.removeAttribute('data-look');
  };

  Rulebook.prototype.open = function (data) {
    if (this.leafId) { this.openLeaf(data); return; }
    this.data = data;
    this.applyTheme(data.theme);
    this.view = data.isDirector ? 'director' : 'player';
    var place = this.inline ? null : loadPlace(this.campaignId);
    if (data.isDirector && place && (place.view === 'player' || place.view === 'director')) this.view = place.view;

    this.unloadFonts();
    this.loadFonts();
    this.frontShut = false;
    this.el.classList.remove('rb-closed', 'rb-unveil');
    clearTimeout(this.frontTimer);
    this.el.innerHTML = this.frameHTML();
    this.el.setAttribute('aria-busy', 'false');
    var q = this.el.querySelector.bind(this.el);
    this.pl = q('.rb-page.l');
    this.pr = q('.rb-page.r');
    this.bookEl = q('.rb-book');
    this.drawerEl = q('.rb-drawer');
    this.scrimEl = q('.rb-scrim');
    this.whereEl = q('.rb-where');
    this.srEl = q('.rb-sr');
    this.tocBtn = q('[data-act="toc"]');
    this.curlPrev = q('.rb-curl.l');
    this.curlNext = q('.rb-curl.r');
    this.turnEl = q('.rb-turnbar');
    this.searchEl = q('[data-search]');
    this.resultsEl = q('.rb-results');

    this.tabsEl = q('.rb-tabs');
    this.ribbonEl = q('.rb-ribbon');
    this.screenEl = q('.rb-screen');
    this.sscrimEl = q('.rb-sscrim');
    this.frontEl = q('.rb-front');
    this.bmEl = q('.rb-bm');
    this.bmTab = q('.rb-bm-tab');
    this.bmRoll = q('.rb-bm-roll');
    this.bmList = q('.rb-bm-list');
    var key = this.openingKey(place);
    this.placeReturn();
    this.rebuild(key);
    // The book opens shut on its cover, as a book on the table does, but
    // never over a link that names a chapter: that reader asked for a page.
    if (this.frontEl && !this.deepLink) {
      this.frontShut = true;
      this.el.classList.add('rb-closed');
      this.frontEl.hidden = false;
      this.syncTurnbar();
    } else if (this.screenAtStart) {
      this.openScreen();
    }
  };

  // --- The book's own fonts and cover -------------------------------------
  // Faces are registered with the FontFace API, which is @font-face without
  // building CSS from package text. The theme's heading-font, body-font and ui-font
  // name the family to use; until a face loads the next font in that list
  // shows.

  Rulebook.prototype.loadFonts = function () {
    var self = this;
    if (this.inline || !window.FontFace || !document.fonts) return;
    this.data.fonts.forEach(function (f) {
      try {
        // The family is passed bare: quotes here would become part of its name.
        var face = new FontFace(f.family, 'url("' + f.url + '") format("woff2")', { weight: String(f.weight), style: f.style, display: 'swap' });
        self.faces.push(face);
        face.load().then(function () { if (!self.dead) document.fonts.add(face); }, function () { /* the theme's fallback font stays */ });
      } catch (e) { /* the theme's fallback font stays */ }
    });
  };

  Rulebook.prototype.unloadFonts = function () {
    this.faces.forEach(function (face) { try { document.fonts.delete(face); } catch (e) { /* not added */ } });
    this.faces = [];
  };

  // The cover is a picture of the closed book laid over the pages. It
  // belongs to the reader, not the page: the bar stays, the pages under it
  // are hidden from sight and from the keyboard until it opens.
  Rulebook.prototype.frontHTML = function () {
    var d = this.data;
    if (!d.cover || this.inline) return '';
    return '<div class="rb-front" hidden><div class="rb-front-desk"></div><div class="rb-front-book"><div class="rb-front-board">' +
      '<button type="button" class="rb-front-face" data-act="front" aria-label="Open the ' + esc(d.title) + '">' +
      '<img src="' + esc(d.cover) + '" alt="' + esc(d.title) + ': the cover" draggable="false">' +
      '<span class="rb-front-hint" aria-hidden="true">Open</span></button>' +
      '<div class="rb-front-back" aria-hidden="true"></div></div></div></div>';
  };

  Rulebook.prototype.openFront = function () {
    var self = this;
    if (!this.frontEl || !this.frontShut) return;
    // Focus follows only a keyboard opening; after a click it would draw a
    // focus ring on a page the reader did not ask to focus.
    var keyboard = !!document.activeElement && document.activeElement.matches && document.activeElement.matches(':focus-visible');
    this.frontShut = false;
    this.el.classList.remove('rb-closed');
    this.syncTurnbar();
    this.fillFrontBack();
    this.frontEl.classList.add('open');
    clearTimeout(this.frontTimer);
    this.frontTimer = setTimeout(function () {
      if (self.dead) return;
      self.frontEl.hidden = true;
      self.el.classList.remove('rb-unveil');
      if (self.screenAtStart) { self.screenAtStart = false; self.openScreen(); return; }
      var page = self.pr && self.pr.querySelector('.rb-scroll');
      if (keyboard && page) page.focus({ preventScroll: true });
    }, reduced() ? 0 : FRONT_MS);
  };

  // The inside of the cover carries the left-hand page while it swings, so
  // it lands on that page instead of a blank board; the real page stays
  // hidden under it until the cover is put away. A page spread across both
  // sides lends its left half.
  Rulebook.prototype.fillFrontBack = function () {
    var back = this.frontEl.querySelector('.rb-front-back');
    var src = this.pl && !this.pl.classList.contains('gone') ? this.pl : this.pr;
    back.textContent = '';
    if (!src) return;
    var copy = src.cloneNode(true);
    copy.removeAttribute('id');
    [].forEach.call(copy.querySelectorAll('[id]'), function (n) { n.removeAttribute('id'); });
    copy.setAttribute('inert', '');
    copy.classList.toggle('rb-front-half', src.classList.contains('wide'));
    back.appendChild(copy);
    this.el.classList.add('rb-unveil');
  };

  Rulebook.prototype.shutFront = function () {
    var self = this;
    if (!this.frontEl || this.frontShut) return;
    this.closeDrawer(false);
    this.closeScreen(false);
    this.bmSet(false, false);
    this.hideCard();
    this.closeResults();
    this.frontShut = true;
    this.syncTurnbar();
    this.fillFrontBack();
    this.frontEl.hidden = false;
    this.frontEl.classList.add('open');
    void this.frontEl.offsetWidth; // let the open state paint so the swing back is animated
    this.frontEl.classList.remove('open');
    clearTimeout(this.frontTimer);
    this.frontTimer = setTimeout(function () {
      if (self.dead) return;
      self.el.classList.add('rb-closed');
      self.el.classList.remove('rb-unveil');
      var face = self.frontEl.querySelector('.rb-front-face');
      if (face) face.focus({ preventScroll: true });
    }, reduced() ? 0 : FRONT_MS);
  };

  // Where the book opens: a chapter named in the address (#making-a-hero),
  // else the book's first-visit chapter for a reader with no saved place,
  // else its start chapter, else the reader's saved place.
  Rulebook.prototype.openingKey = function (place) {
    var d = this.data, hash = '';
    try { hash = decodeURIComponent(String(window.location.hash || '').slice(1)); } catch (e) { hash = ''; }
    var key = SLUG_RE.test(hash) ? this.chapterKey(hash) : '';
    this.deepLink = !!key;
    // A reload or a return to the book keeps the exact page the address was
    // last written for, when it is still in the chapter the address names.
    var st = window.history.state;
    if (key && st && typeof st.rbKey === 'string' && st.rbKey.indexOf(key + '.') === 0) return st.rbKey;
    if (key) return key;
    var saved = place && typeof place.key === 'string' ? place.key : '';
    if (!saved && d.first) key = this.chapterKey(d.first);
    // A book that starts on its screen opens the screen over the page the
    // reader was on, since the screen is not a page of its own.
    if (d.start && d.start === d.screen) this.screenAtStart = !hash;
    else if (!key && d.start) key = this.chapterKey(d.start);
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
    if (id && id === this.data.screen) { this.openScreen(); return true; }
    var n = this.firstItemOf(id);
    if (n >= 0) this.goItem(n);
    return n >= 0;
  };

  // holdBack makes the phone's Back gesture close a drawer or screen instead of
  // leaving the page, when Chronicle's helper is there to do it. The result is
  // the release to call when the sheet closes by its own control.
  Rulebook.prototype.holdBack = function (name, close) {
    if (!window.Chronicle || !Chronicle.backCloses) return null;
    try { return Chronicle.backCloses(name, close); } catch (e) { return null; }
  };

  Rulebook.prototype.letGoBack = function (field) {
    var release = this[field];
    this[field] = null;
    if (typeof release === 'function') release();
  };

  // itemIndexOf is the first shown item for an item key, or for the chapter
  // part of it when that exact page is not shown in this view; -1 if neither.
  Rulebook.prototype.itemIndexOf = function (key) {
    var i;
    if (!key) return -1;
    for (i = 0; i < this.items.length; i++) if (this.items[i].key === key) return i;
    var prefix = key.split('.').slice(0, 2).join('.');
    for (i = 0; i < this.items.length; i++) {
      if (this.items[i].key === prefix || this.items[i].key.indexOf(prefix + '.') === 0) return i;
    }
    return -1;
  };

  // --- Address ---------------------------------------------------------------
  // The address names the chapter being read (#making-a-hero), and each entry
  // remembers the exact page, so a reload keeps the page and the browser's
  // Back undoes a jump inside the book before it leaves it. Not for a book
  // shown inline or a leaf. A state carrying .sheet belongs to an open drawer
  // or screen; it is never written over, and a jump's state never carries it.

  Rulebook.prototype.addressed = function () {
    return !this.inline && !this.leafId && !this.dead && window.location.pathname + window.location.search === this.basePath;
  };

  Rulebook.prototype.sheetLive = function () {
    var st = window.history.state;
    return !!(st && st.sheet);
  };

  Rulebook.prototype.chapterIdOf = function (item) {
    var id = item && item.ch ? item.ch.id : '';
    return SLUG_RE.test(id) ? id : '';
  };

  // writeAddress puts the page being read into the current entry, or, for a
  // jump, into a new one.
  Rulebook.prototype.writeAddress = function (item, push) {
    if (!item || !this.addressed()) return;
    var id = this.chapterIdOf(item);
    if (!id) return;
    try {
      if (push) {
        window.history.pushState({ rb: 1, rbKey: item.key }, '', '#' + id);
      } else if (!this.sheetLive()) {
        var st = {}, cur = window.history.state;
        if (cur && typeof cur === 'object') Object.keys(cur).forEach(function (k) { st[k] = cur[k]; });
        st.rbKey = item.key;
        window.history.replaceState(st, '', '#' + id);
      }
    } catch (e) { /* the book works without an address that follows it */ }
  };

  // A jump records the spread it left in the entry it leaves, then adds an
  // entry for where it lands. While a drawer's own entry is still being
  // dropped, the push waits for that step to finish so the two never cross.
  Rulebook.prototype.jumpAddress = function (leftItem, landItem, sheetWas) {
    if (!this.addressed()) return;
    if (!sheetWas) {
      this.writeAddress(leftItem, false);
      this.writeAddress(landItem, true);
      return;
    }
    var self = this;
    this.pendingPush = { left: leftItem, land: landItem };
    clearTimeout(this.pushTimer);
    this.pushTimer = setTimeout(function () { self.commitPush(); }, 600);
  };

  Rulebook.prototype.commitPush = function () {
    var p = this.pendingPush;
    clearTimeout(this.pushTimer);
    this.pendingPush = null;
    if (!p || this.dead) return;
    if (this.sheetLive()) return;
    this.writeAddress(p.left, false);
    this.writeAddress(p.land, true);
  };

  Rulebook.prototype.onPop = function (e) {
    if (!this.data || !this.addressed()) return;
    var st = e.state;
    if (st && st.sheet) return;
    if (this.pendingPush) { e.stopImmediatePropagation(); this.commitPush(); return; }
    var key = st && typeof st.rbKey === 'string' ? st.rbKey : '';
    if (!key) {
      var hash = '';
      try { hash = decodeURIComponent(String(window.location.hash || '').slice(1)); } catch (err) { hash = ''; }
      key = SLUG_RE.test(hash) ? this.chapterKey(hash) : '';
    }
    var idx = this.itemIndexOf(key), sp = idx < 0 ? -1 : this.spreadOf(idx);
    if (sp < 0 || sp === this.at) return;
    e.stopImmediatePropagation();
    this.closeResults();
    this.refocus = true;
    this.turnTo(sp);
    if (!this.busy) this.settled();
  };

  // rulebook:goto is how something drawn inside a page asks the book to turn
  // to a chapter, as a jump the reader can return from. A turn the book makes
  // cancels the event, so the asker can fall back to its own link otherwise.
  Rulebook.prototype.onGoto = function (e) {
    var d = e && e.detail, id = d && typeof d.chapter === 'string' ? d.chapter : '';
    if (!this.data || this.inline || !SLUG_RE.test(id) || !this.el.contains(e.target)) return;
    if (this.goChapter(id) && e.preventDefault) e.preventDefault();
  };

  // "Back to p. N": the quiet word in the page foot after a jump.
  Rulebook.prototype.placeBack = function () {
    [].forEach.call(this.el.querySelectorAll('.rb-back'), function (b) { b.remove(); });
    if (!this.backTo || this.inline) return;
    var host = this.pr.classList.contains('gone') || this.pr.classList.contains('blank') ? this.pl : this.pr;
    var foot = host.querySelector('.rb-foot');
    if (!foot) return;
    var b = document.createElement('button');
    b.type = 'button';
    b.className = 'rb-back';
    b.setAttribute('data-act', 'jump-back');
    b.textContent = 'Back to p. ' + this.backTo.page;
    foot.insertBefore(b, foot.firstChild);
  };

  Rulebook.prototype.useBack = function () {
    var b = this.backTo;
    if (!b) return;
    this.backTo = null;
    this.refocus = true;
    // When the jump's own entry is the current one, going back through
    // history turns the page and keeps Back and the word in step.
    var st = window.history.state;
    if (st && st.rb && this.addressed()) { window.history.back(); return; }
    this.turnTo(b.at);
    if (!this.busy) this.settled();
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
    d.tabs.forEach(function (id) { if (seen[id] && id !== d.screen) tabs.push({ id: id, label: seen[id].title }); });
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
    var d = this.data, seg = '', turnbar = d.look === 'bold' && !this.inline;
    if (d.isDirector) {
      seg = '<div class="rb-seg" role="group" aria-label="Book view">' +
        '<button type="button" data-act="view" data-view="player" aria-pressed="false">Player view</button>' +
        '<button type="button" data-act="view" data-view="director" aria-pressed="false">' + gmTerm() + ' view</button></div>';
    }
    var edit = d.canEdit && this.url
      ? '<button type="button" class="rb-btn" data-act="edit" data-href="' + esc(this.url.replace(/[?#].*$/, '').replace(/\/+$/, '') + '/edit') + '">Edit</button>'
      : '';
    // What the campaign wrote, rewrote and left out of its rules is read
    // from the rulebook, by players and Directors alike.
    var own = this.beforeUrl && this.campaignName && !this.inline
      ? '<button type="button" class="rb-btn" data-act="edit" data-before-link="true" data-href="' + esc(this.beforeUrl) + '">' +
        esc('Before you play in ' + this.campaignName) + '</button>'
      : '';
    // The Homebrew Workshop builds the campaign's house-rule recipes, so it
    // opens from here too; it is the Directors' alone.
    var workshop = d.isDirector && this.campaignId && !this.inline
      ? '<button type="button" class="rb-btn" data-act="edit" data-workshop-link="true" data-href="/campaigns/' +
        encodeURIComponent(this.campaignId) + '/homebrew">Homebrew workshop</button>'
      : '';
    return '<div class="rb-bar">' +
      '<div class="rb-brand">' + (d.mark ? '<span class="rb-mark" aria-hidden="true">' + esc(d.mark) + '</span>' : '') +
      '<div class="rb-name"><span class="rb-title">' + esc(d.title) + '</span>' +
      (d.systemName ? '<small>' + esc(d.systemName) + '</small>' : '') + '</div></div>' +
      // A bold book has the ribbon and turnbar; contents, tabs and "where" would repeat them.
      (turnbar ? '' : '<button type="button" class="rb-btn" data-act="toc" aria-expanded="false" aria-label="Open the contents"><span aria-hidden="true">☰</span> Contents</button>') +
      '<span class="rb-where"' + (turnbar ? ' hidden' : '') + '></span><span class="rb-sp"></span>' +
      (this.inline ? '' : '<div class="rb-search" role="search"><span class="rb-search-ic" aria-hidden="true">⌕</span>' +
      '<input type="search" class="rb-search-in" data-search placeholder="Search the book" aria-label="Search the book" autocomplete="off" spellcheck="false"' +
      ' aria-controls="' + this.domId('results') + '" aria-expanded="false"><kbd aria-hidden="true">/</kbd>' +
      '<div class="rb-results" id="' + this.domId('results') + '" role="region" aria-label="Search results" hidden></div></div>') +
      ((own || workshop || edit || seg) ? '<button type="button" class="rb-btn rb-more" data-act="more" aria-expanded="false" aria-controls="' + this.domId('tools') +
        '" aria-label="More"><span aria-hidden="true">\u22EF</span></button><div class="rb-tools" id="' + this.domId('tools') + '">' + own + workshop + edit + seg + '</div>' : '') +
      (d.cover && !this.inline && !turnbar ? '<button type="button" class="rb-btn rb-shut" data-act="shut">Close the book</button>' : '') +
      this.ribbonHTML() + '</div>' + (turnbar ? '' : this.tabsHTML()) + this.bmHTML() +
      '<div class="rb-book">' +
      '<article class="rb-page l"></article><div class="rb-spine" aria-hidden="true"></div><article class="rb-page r"></article>' +
      '<button type="button" class="rb-curl l" data-act="prev" aria-label="Previous page"></button>' +
      '<button type="button" class="rb-curl r" data-act="next" aria-label="Next page"></button>' +
      '<div class="rb-scrim"></div><aside class="rb-drawer" aria-label="Contents"></aside>' +
      (this.screenChapter() ? '<div class="rb-sscrim" data-act="screen"></div><section class="rb-screen" id="' + this.domId('screen') + '" aria-label="' +
        esc(this.screenChapter().title) + '" hidden></section>' : '') + this.frontHTML() + '</div>' +
      (turnbar ? this.turnbarHTML() : '') +
      '<div class="rb-sr" aria-live="polite"></div>';
  };

  // returnLink is the way out for a reader a link elsewhere in Chronicle sent
  // to a place in the book: the page they came from. The book's own page
  // controls only turn pages, so without it the reader has no way back on the
  // page. It is empty for a reader who opened the book themselves.
  Rulebook.prototype.returnLink = function () {
    if (this.inline || !this.deepLink) return null;
    // Opening the book from a page is usually an in-page swap, which leaves
    // document.referrer on the last page loaded in full; boot.js knows the
    // page actually left.
    var ref, from = window.Chronicle && Chronicle.cameFrom ? Chronicle.cameFrom() : document.referrer;
    try { ref = new URL(from); } catch (e) { return null; }
    if (ref.origin !== window.location.origin || ref.pathname === window.location.pathname) return null;
    return {
      href: ref.pathname + ref.search + ref.hash,
      label: /^\/campaigns\/[^/]+\/?$/.test(ref.pathname) ? 'Back to the campaign home' : 'Back to where you were'
    };
  };

  // placeReturn adds the way back once the opening place is known: first in
  // the turn bar of a bold book, else before Contents in the bar.
  Rulebook.prototype.placeReturn = function () {
    var r = this.returnLink(), before = this.turnEl ? this.turnEl.firstChild : this.tocBtn;
    if (!r || !before) return;
    var a = document.createElement('a');
    a.className = (this.turnEl ? 'rb-pill' : 'rb-btn') + ' rb-return';
    a.setAttribute('data-act', 'return');
    a.href = r.href;
    a.textContent = '\u21A9 ' + r.label;
    before.parentNode.insertBefore(a, before);
  };

  // The bold look turns its pages from a bar under the book, as a printed
  // handbook's reader would expect: previous, where the reader is, close the
  // book, next. While the book is shut, Next opens it.
  Rulebook.prototype.turnbarHTML = function () {
    return '<div class="rb-turnbar">' +
      '<button type="button" class="rb-pill" data-act="prev">\u2039 Previous</button>' +
      '<span class="rb-turn-where" aria-hidden="true"></span><span class="rb-turn-ctl">' +
      (this.data.cover ? '<button type="button" class="rb-pill" data-act="shut">Close the book</button>' : '') +
      '<button type="button" class="rb-pill" data-act="next">Next \u203A</button></span></div>';
  };

  Rulebook.prototype.syncTurnbar = function () {
    if (!this.turnEl) return;
    var q = this.turnEl.querySelector.bind(this.turnEl), shut = this.frontShut;
    var sp = this.spreads[this.at], last = this.spreads.length - 1, where = '';
    if (shut) where = 'The cover';
    else if (sp) {
      where = sp.items.map(function (it) { return it.kind === 'page' && it.p.title ? it.p.title : it.ch.title; })
        .filter(function (t, i, all) { return all.indexOf(t) === i; }).join(' \u00B7 ');
    }
    q('.rb-turn-where').textContent = where;
    q('[data-act="prev"]').disabled = shut || this.at <= 0;
    q('[data-act="next"]').disabled = !shut && this.at >= last;
    var close = q('[data-act="shut"]');
    if (close) close.disabled = shut;
  };

  // On a phone the bar keeps one row: the book's own pages, Edit and the view
  // switch fold under a More button.
  Rulebook.prototype.moreOpen = function () {
    var b = this.el && this.el.querySelector('.rb-more');
    return !!b && b.getAttribute('aria-expanded') === 'true';
  };

  Rulebook.prototype.setMore = function (open) {
    var b = this.el && this.el.querySelector('.rb-more');
    if (!b) return;
    b.setAttribute('aria-expanded', open ? 'true' : 'false');
    this.el.querySelector('.rb-tools').classList.toggle('open', open);
  };

  // --- The screen ------------------------------------------------------------
  // A book can name one chapter as its screen: the quick-reference pages a
  // player reaches for mid-game. It is not a page of the book. A ribbon under
  // the bar drops it over whatever page is open, and Escape, the ribbon or a
  // click on the book folds it away.

  Rulebook.prototype.screenChapter = function () {
    var id = this.data.screen, found = null;
    if (!id) return null;
    this.data.parts.forEach(function (part) {
      part.chapters.forEach(function (ch) { if (!found && ch.id === id) found = ch; });
    });
    return found;
  };

  Rulebook.prototype.ribbonHTML = function () {
    var ch = this.screenChapter();
    if (!ch) return '';
    return '<button type="button" class="rb-ribbon" data-act="screen" aria-expanded="false" aria-controls="' + this.domId('screen') + '">' +
      '<span class="rb-ribbon-t">' + esc(ch.title) + '</span> <span class="rb-chev" aria-hidden="true">\u25BE</span><kbd aria-hidden="true">P</kbd></button>';
  };

  Rulebook.prototype.screenOpen = function () { return !!this.screenEl && this.el.classList.contains('rb-screen-open'); };

  // Each page of the screen chapter is one leaf; a long card list on a leaf
  // gets a box to find one card in it.
  Rulebook.prototype.screenHTML = function () {
    var self = this;
    return this.screenPages.map(function (sp, i) {
      return '<div class="rb-sleaf" style="--i:' + i + '"><div class="rb-scroll" tabindex="-1">' + self.pageBodyHTML('', sp.p) + '</div></div>';
    }).join('');
  };

  Rulebook.prototype.openScreen = function () {
    if (!this.screenEl || !this.screenPages.length || this.screenOpen()) return;
    this.bmSet(false, false);
    this.closeDrawer(false);
    this.hideCard();
    this.closeResults();
    clearTimeout(this.screenTimer);
    this.screenEl.classList.remove('closing');
    this.destroyNested(this.screenEl);
    this.screenEl.innerHTML = this.screenHTML();
    this.screenEl.style.setProperty('--n', String(Math.min(3, this.screenPages.length)));
    [].forEach.call(this.screenEl.querySelectorAll('.rb-cards'), function (grid) {
      if (grid.querySelectorAll('.rb-fc').length < FILTER_AT) return;
      grid.insertAdjacentHTML('beforebegin', '<input type="search" class="rb-cfind" data-cfind placeholder="Find one…" aria-label="Find in this list" autocomplete="off" spellcheck="false">');
    });
    this.mountNested(this.screenEl);
    this.screenEl.hidden = false;
    this.el.classList.add('rb-screen-open');
    if (!this.releaseScreen) {
      var me = this;
      this.releaseScreen = this.holdBack('rb-screen', function () { me.closeScreen(true); });
    }
    this.ribbonEl.setAttribute('aria-expanded', 'true');
    var first = this.screenEl.querySelector('button, input, select, [tabindex="0"]');
    if (first) requestAnimationFrame(function () { first.focus({ preventScroll: true }); });
  };

  Rulebook.prototype.closeScreen = function (restoreFocus) {
    if (!this.screenOpen()) return;
    if (!this.rebuilding) this.letGoBack('releaseScreen');
    var self = this, el = this.screenEl, hadFocus = el.contains(document.activeElement);
    this.hideCard();
    this.el.classList.remove('rb-screen-open');
    this.ribbonEl.setAttribute('aria-expanded', 'false');
    el.classList.add('closing');
    this.screenTimer = setTimeout(function () {
      el.classList.remove('closing');
      el.hidden = true;
      self.destroyNested(el);
      el.innerHTML = '';
    }, reduced() ? 0 : SCREEN_CLOSE_MS);
    if (restoreFocus && hadFocus) this.ribbonEl.focus({ preventScroll: true });
  };

  // Hides the cards in a screen list that do not match what was typed.
  Rulebook.prototype.findCard = function (input) {
    var grid = input.nextElementSibling, q = input.value.trim().toLowerCase();
    if (!grid || !grid.classList.contains('rb-cards')) return;
    [].forEach.call(grid.querySelectorAll('.rb-fold'), function (f) { f.hidden = true; });
    [].forEach.call(grid.querySelectorAll('.rb-fc'), function (c) {
      c.setAttribute('aria-expanded', 'false');
      c.hidden = !!q && c.textContent.toLowerCase().indexOf(q) < 0;
    });
  };

  // --- Page list -------------------------------------------------------------

  Rulebook.prototype.showDirector = function () {
    return this.data.isDirector && this.view === 'director';
  };

  // Every chapter becomes an opener page followed by its pages, so a spread
  // always holds two consecutive items; a wide page takes a spread alone. A
  // chapter that is one wide page (a system's own front page) needs no
  // opener. In a Director's player-view preview, a page whose blocks are all
  // Director-only is skipped, as the server skips it for real players. In the
  // bold look such a chapter that is only a contents list takes the left page
  // and the next chapter opens beside it, as in the signed demo.
  Rulebook.prototype.buildItems = function () {
    var dir = this.showDirector(), items = [], bold = this.data.look === 'bold', hue = 0;
    var pageShows = function (p) {
      if (p.director && !dir) return false;
      return dir || p.blocks.some(function (b) { return !b.director; });
    };
    var screen = this.data.screen, screenPages = [];
    this.data.parts.forEach(function (part, pi) {
      if (part.director && !dir) return;
      part.chapters.forEach(function (ch, ci) {
        if (ch.director && !dir) return;
        var shown = [];
        ch.pages.forEach(function (p, gi) { if (pageShows(p)) shown.push({ p: p, gi: gi }); });
        if (!shown.length) return;
        if (ch.id === screen) {
          shown.forEach(function (s) { screenPages.push({ ch: ch, p: s.p }); });
          return;
        }
        var opener = { kind: 'open', part: part, ch: ch, key: pi + '.' + ci, wide: false, pages: [] };
        var solo = shown.length === 1 && shown[0].p.wide;
        var leaf = bold && solo && isContentsPage(shown[0].p);
        opener.hue = !bold || solo ? '' : CHAPTER_HUES[hue++ % CHAPTER_HUES.length];
        if (!solo) items.push(opener);
        shown.forEach(function (s) {
          var it = { kind: 'page', part: part, ch: ch, p: s.p, key: pi + '.' + ci + '.' + s.gi, wide: s.p.wide && !leaf && !media(NARROW_QUERY), solo: solo, leaf: leaf, hue: opener.hue };
          items.push(it);
          opener.pages.push(it);
        });
      });
    });
    items.forEach(function (it, i) { it.n = i; });
    this.items = items;
    this.screenPages = screenPages;
  };

  Rulebook.prototype.buildSpreads = function () {
    var sp = [], cur = [];
    if (this.narrow) {
      this.items.forEach(function (it) { sp.push({ items: [it], wide: false }); });
    } else {
      // Every chapter starts on a fresh spread, in either look, so a tab or
      // link always lands on a spread that opens with that chapter. The one
      // exception is a bold book's contents page, which the next chapter
      // shares.
      this.items.forEach(function (it) {
        if (it.kind === 'open' && cur.length && !(cur.length === 1 && cur[0].leaf)) { sp.push({ items: cur, wide: false }); cur = []; }
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
    var idx = this.itemIndexOf(key);
    this.backTo = null;
    this.at = idx < 0 ? 0 : this.spreadOf(idx);
    if (this.bmEl) { this.bmSet(false, false); this.bmEl.hidden = !this.items.length; }
    if (this.ribbonEl) {
      this.ribbonEl.hidden = !this.screenPages.length;
      this.el.classList.toggle('rb-has-screen', !!this.screenPages.length);
      if (this.screenOpen()) {
        // The screen is closed and drawn again around the same history step.
        this.rebuilding = true;
        this.closeScreen(false);
        if (this.screenPages.length) this.openScreen();
        this.rebuilding = false;
        if (!this.screenOpen()) this.letGoBack('releaseScreen');
      }
    }
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
    this.syncTurnbar();
    this.syncTabs();
    if (first) {
      this.srEl.textContent = 'Page ' + (first.n + 1) + (it !== first ? ' and ' + (it.n + 1) : '') + ' of ' + this.items.length + ': ' +
        first.ch.title + (first.kind === 'page' ? ', ' + first.p.title : '');
      var place = { key: first.key };
      if (this.data.isDirector) place.view = this.view;
      if (!this.inline) savePlace(this.campaignId, place);
      if (this.addrOn) this.writeAddress(first, false);
    }
    this.placeBack();
  };

  Rulebook.prototype.dirTag = function () { return '<span class="rb-dir">' + gmTerm() + '</span>'; };

  // foot replaces the page's folio line; the leaf draws its own.
  Rulebook.prototype.pageHTML = function (item, foot) {
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
            (p.p.director ? ' ' + '<span class="rb-dir">' + gmTerm() + '</span>' : '') + '</span><span class="rb-pn">' + (p.n + 1) + '</span></button></li>';
        }).join('') + '</ul>';
      }
    } else {
      label = item.p.title || item.ch.title;
      this.paintingChapter = item.ch.id;
      h = this.pageBodyHTML(item.ch.title, item.p);
      this.paintingChapter = '';
    }
    // The running head repeats what the page already says, so readers skip
    // it. A wide page keeps it all on the left, clear of the Sections ribbon.
    var chapter = item.ch.title === this.data.title ? '' : item.ch.title;
    var head = item.wide ? '<span>' + esc(this.data.title) + (chapter ? ' \u00B7 ' + esc(chapter) : '') + '</span>'
      : '<span>' + esc(this.data.title) + '</span><span>' + esc(chapter) + '</span>';
    return '<div class="rb-fhd" aria-hidden="true">' + head + '</div>' +
      '<div class="rb-scroll" tabindex="0" role="region" aria-label="Page ' + (item.n + 1) + ': ' + esc(label) + '"' + hueStyle(item.hue) + '>' + h + '</div>' +
      '<div class="rb-foot">' + (foot || '<span class="rb-folio">' + (item.n + 1) + '</span>') + '</div>';
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
      case 'contents': html = this.contentsHTML(b); break;
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
      var id = str(it.chapter);
      return isObj(it) && SLUG_RE.test(id) && (!self.items.length || self.firstItemOf(id) >= 0 || (id === self.data.screen && self.screenPages.length > 0));
    });
    if (!items.length) return '';
    return '<nav class="rb-links" aria-label="' + esc(str(b.title) || 'Turn to') + '">' +
      (str(b.title) ? '<b class="rb-links-hd">' + esc(b.title) + '</b>' : '') +
      '<div class="rb-links-row">' + items.map(function (it) {
        return '<button type="button" class="rb-link" data-chapter="' + esc(it.chapter) + '"><b>' + esc(it.title) + '</b>' +
          (str(it.summary) ? '<span>' + inline(it.summary, t) + '</span>' : '') + '<i aria-hidden="true">Turn to it \u2192</i></button>';
      }).join('') + '</div></nav>';
  };

  // contentsHTML numbers every chapter this view shows, except the one the
  // list is on, with the page each starts on. The screen chapter is listed
  // too and drops the screen, as its links do. A page drawn on its own (the
  // editor's preview) has no laid-out book, so it lists the chapters it was
  // given without page numbers.
  Rulebook.prototype.contentsHTML = function (b) {
    var self = this, rows = [], seen = {}, here = this.paintingChapter, laidOut = this.items.length > 0;
    arr(this.data.parts).filter(isObj).forEach(function (part) {
      arr(part.chapters).filter(isObj).forEach(function (ch) {
        if (seen[ch.id] || ch.id === here) return;
        var n = self.firstItemOf(ch.id), screen = ch.id === self.data.screen && self.screenPages.length > 0;
        if (laidOut ? n < 0 && !screen : self.view !== 'director' && (ch.director || part.director)) return;
        seen[ch.id] = true;
        rows.push('<li><button type="button" data-chapter="' + esc(ch.id) + '"' + hueStyle(n >= 0 ? self.items[n].hue : '') + '><span class="rb-pn">' + (rows.length + 1) + '</span><span>' + esc(ch.title) +
          (ch.director ? ' ' + self.dirTag() : '') + '</span><span class="rb-cpg">' + (screen ? 'Screen' : laidOut ? 'p. ' + (n + 1) : '') + '</span></button></li>');
      });
    });
    if (!rows.length) return '';
    return '<nav class="rb-contents" aria-label="' + esc(str(b.title) || 'Contents') + '">' +
      (str(b.title) ? '<b class="rb-links-hd">' + esc(b.title) + '</b>' : '') +
      '<ol class="rb-toc">' + rows.join('') + '</ol></nav>';
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
  // when the page is shown (once per slice) and fold down like flaps. A page
  // whose chapter names filters offers them as index tabs over the list: each
  // tab drops a drawer of the values on this page, with how many entries each
  // would leave, and the chosen values narrow the list together with the
  // word box, which reads every word of an entry.

  var MAX_LIST_FILTERS = 6;

  // The layout the server sent with the block, checked: property keys only,
  // labels as text.
  function indexList(b) {
    var l = isObj(b.list) ? b.list : {};
    var key = function (k) { return /^[a-z0-9_]{1,64}$/.test(str(k)) ? str(k) : ''; };
    return {
      filters: arr(l.filters).filter(isObj).map(function (f) { return { key: key(f.key), label: str(f.label) || key(f.key) }; })
        .filter(function (f) { return f.key; }).slice(0, MAX_LIST_FILTERS),
      sections: key(l.sections),
      sectionLabel: str(l.sectionLabel),
      tiers: arr(l.tiers).map(key).filter(Boolean).slice(0, 4)
    };
  }

  Rulebook.prototype.indexHTML = function (b) {
    var count = intOr(b.count, 0, 100000, 0);
    var noun = count === 1 ? 'entry' : 'entries';
    var list = indexList(b);
    return '<div class="rb-ix" data-ix data-cat="' + esc(str(b.category)) + '" data-key="' + esc(str(b.key)) +
      '" data-value="' + esc(str(b.value)) + '" data-from="' + intOr(b.from, 0, 100000, 0) + '" data-to="' + intOr(b.to, 0, 100000, 0) + '"' +
      ' data-list="' + esc(JSON.stringify(list)) + '">' +
      '<div class="rb-ix-hd"><input type="search" class="rb-ix-filter" data-ix-filter placeholder="Filter ' + count + ' ' + noun + '"' +
      ' aria-label="Filter this page" autocomplete="off" spellcheck="false"><span class="rb-ix-n" aria-live="polite"></span></div>' +
      (list.filters.length ? '<div class="rb-ixf" data-ixf></div>' : '') +
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

  function boxList(box) {
    try { return indexList({ list: JSON.parse(box.getAttribute('data-list') || '{}') }); } catch (e) { return indexList({}); }
  }
  // An entry's values for one property, as the server sent them.
  function facetVals(it, key) {
    var f = arr(it.facets).filter(function (x) { return isObj(x) && x.key === key; })[0];
    return f ? arr(f.values).map(str).filter(Boolean) : [];
  }
  // A number reads with its property's name ("Level 2"), an ordinal before
  // it ("1st echelon"); a word reads alone.
  function valueLabel(label, v) {
    if (!label) return v;
    if (/^-?\d+(\.\d+)?$/.test(v)) return label + ' ' + v;
    return /^\d+(st|nd|rd|th)$/.test(v) ? v + ' ' + label.toLowerCase() : v;
  }
  function entryName(it) { return str(it.name); }
  function entryText(it) {
    return [it.name, it.summary, it.text].concat(arr(it.fields).map(function (f) { return isObj(f) ? f.value : ''; }))
      .map(str).join(' ').toLowerCase();
  }

  // The line under an entry's name: its one-value filters, in order, so a
  // reader can scan level and action without opening anything.
  function metaLine(it, list) {
    return list.filters.map(function (f) {
      var v = facetVals(it, f.key);
      return v.length === 1 ? esc(valueLabel(f.label, v[0])) : '';
    }).filter(Boolean).join('<i aria-hidden="true">·</i>');
  }

  // An entry's body: the result tiers on the book's tier tiles, then every
  // other field the line above does not already show, then its text.
  function entryBodyHTML(it, list, terms) {
    var shown = {};
    list.filters.forEach(function (f) { if (facetVals(it, f.key).length === 1) shown[f.key] = 1; });
    list.tiers.forEach(function (k) { shown[k] = 1; });
    var fields = arr(it.fields).filter(isObj), tiers = [], rest = [];
    fields.forEach(function (f) {
      var k = str(f.key), base = k.replace(/_display$/, '');
      if (list.tiers.indexOf(k) >= 0 || list.tiers.indexOf(base) >= 0) tiers.push(f);
      else if (!shown[k] && !shown[base]) rest.push(f);
    });
    var h = '';
    if (tiers.length) {
      h += '<div class="rb-tiers rb-ix-tiers">' + tiers.map(function (f) {
        // "Tier 1 (11 or lower)" is drawn as the tile's name and its range.
        var m = /^(.*?)\s*\(([^)]*)\)\s*$/.exec(str(f.label));
        return '<div class="rb-tier"><b>' + esc(m ? m[1] : str(f.label)) + '</b><small>' + esc(m ? m[2] : '') + '</small><span>' + inline(f.value, terms) + '</span></div>';
      }).join('') + '</div>';
    }
    if (rest.length) {
      h += '<dl class="rb-ix-dl">' + rest.map(function (f) {
        return '<div><dt>' + esc(f.label) + '</dt><dd>' + inline(f.value, terms) + '</dd></div>';
      }).join('') + '</dl>';
    }
    if (str(it.text)) h += '<div class="rb-prose">' + rich(it.text, terms) + '</div>';
    else if (!h && str(it.summary)) h += '<div class="rb-prose"><p>' + esc(it.summary) + '</p></div>';
    return h;
  }

  Rulebook.prototype.renderIndex = function (box, items) {
    var list = box.querySelector('[data-ix-list]'), t = this.data.terms, lay = boxList(box);
    if (!items) {
      list.innerHTML = '<li class="rb-ix-wait">The entries could not be loaded. Turn away and back to try again.</li>';
      return;
    }
    box._items = items;
    box._sel = box._sel || {};
    var rows = items.slice(), sec = lay.sections;
    if (sec) {
      // Sections in their natural order (Level 1 before Level 10); entries
      // without the property go last. Name order holds within a section.
      var keyOf = function (it) { return facetVals(it, sec)[0] || ''; };
      var natural = function (a, b) {
        if (a === b) return 0;
        if (!a || !b) return a ? -1 : 1;
        var na = +a, nb = +b;
        return !isNaN(na) && !isNaN(nb) ? na - nb : a.localeCompare(b);
      };
      rows = rows.map(function (it, i) { return { it: it, i: i }; })
        .sort(function (a, b) { return natural(keyOf(a.it), keyOf(b.it)) || a.i - b.i; })
        .map(function (x) { return x.it; });
    }
    var last = null, h = '';
    rows.forEach(function (it) {
      if (sec) {
        var g = facetVals(it, sec)[0] || '';
        if (g !== last) {
          h += '<li class="rb-ix-sec" aria-hidden="true">' + esc(g ? valueLabel(lay.sectionLabel, g) : 'Other') + '</li>';
          last = g;
        }
      }
      var meta = metaLine(it, lay);
      h += '<li class="rb-flap rb-ix-row" data-id="' + esc(str(it.id)) + '" data-find="' + esc(entryText(it)) + '">' +
        '<button type="button" aria-expanded="false"><span class="rb-ix-t"><b>' + esc(entryName(it)) + '</b>' +
        (meta ? '<span class="rb-ix-meta">' + meta + '</span>' : '') +
        (str(it.summary) ? '<span class="rb-ix-sum">' + esc(it.summary) + '</span>' : '') + '</span></button>' +
        '<div class="rb-body" hidden>' + entryBodyHTML(it, lay, t) + '</div></li>';
    });
    list.innerHTML = h;
    // Each row keeps its entry, so filtering never parses the page again.
    var lis = list.querySelectorAll('.rb-ix-row');
    rows.forEach(function (it, i) { if (lis[i]) lis[i]._entry = it; });
    box.setAttribute('data-ready', '');
    this.buildFilters(box);
    this.applyFilters(box);
    this.openPending(box);
  };

  // The values each filter takes on this page, in natural order. A filter
  // with one value or none narrows nothing and is left out.
  function facetTable(box) {
    var lay = boxList(box), items = box._items || [];
    return lay.filters.map(function (f) {
      var seen = {};
      items.forEach(function (it) { facetVals(it, f.key).forEach(function (v) { seen[v] = 1; }); });
      var vals = Object.keys(seen).sort(function (a, b) {
        var na = +a, nb = +b;
        return !isNaN(na) && !isNaN(nb) ? na - nb : a.localeCompare(b);
      });
      return { key: f.key, label: f.label, values: vals };
    }).filter(function (f) { return f.values.length > 1; });
  }

  function passes(it, sel, skip, q) {
    for (var k in sel) {
      if (k === skip || !has(sel, k) || !sel[k].length) continue;
      var vals = facetVals(it, k);
      if (!sel[k].some(function (v) { return vals.indexOf(v) >= 0; })) return false;
    }
    return !q || entryText(it).indexOf(q) >= 0;
  }
  function boxQuery(box) {
    var f = box.querySelector('[data-ix-filter]');
    return f ? f.value.trim().toLowerCase() : '';
  }

  Rulebook.prototype.buildFilters = function (box) {
    var host = box.querySelector('[data-ixf]');
    if (!host) return;
    var facets = box._facets = facetTable(box);
    host.hidden = !facets.length;
    host.innerHTML = facets.length ? '<div class="rb-ixf-tabs" role="group" aria-label="Narrow this page by">' + facets.map(function (f) {
      return '<button type="button" class="rb-ixf-tab" data-ixf-tab="' + esc(f.key) + '" aria-expanded="false">' +
        esc(f.label) + '<span class="rb-ixf-n"></span></button>';
    }).join('') + '</div><div class="rb-ixf-drawer" role="group" hidden></div><p class="rb-ixf-on" aria-live="polite"></p>' : '';
  };

  // A tab's drawer: each value with how many entries it would leave given
  // every other choice, so a reader never picks into an empty list.
  Rulebook.prototype.filterDrawerHTML = function (box, key) {
    var f = (box._facets || []).filter(function (x) { return x.key === key; })[0];
    if (!f) return '';
    var sel = box._sel[key] || [], q = boxQuery(box);
    var pool = (box._items || []).filter(function (it) { return passes(it, box._sel, key, q); });
    return f.values.map(function (v) {
      var n = pool.filter(function (it) { return facetVals(it, key).indexOf(v) >= 0; }).length;
      var on = sel.indexOf(v) >= 0;
      return '<button type="button" class="rb-ixf-val' + (n ? '' : ' none') + '"' + (n || on ? '' : ' disabled') + ' data-ixf-val="' + esc(key) + '" data-v="' + esc(v) +
        '" aria-pressed="' + on + '" aria-label="' + esc(valueLabel(f.label, v) + ', ' + n + (n === 1 ? ' entry' : ' entries')) +
        '">' + esc(v) + '<span aria-hidden="true">' + n + '</span></button>';
    }).join('');
  };

  Rulebook.prototype.applyFilters = function (box) {
    var sel = box._sel || {}, q = boxQuery(box), shown = 0, rows = box.querySelectorAll('.rb-ix-row');
    [].forEach.call(rows, function (li) {
      var on = li._entry ? passes(li._entry, sel, '', q) : (!q || li.getAttribute('data-find').indexOf(q) >= 0);
      li.hidden = !on;
      if (on) shown++;
    });
    [].forEach.call(box.querySelectorAll('.rb-ix-sec'), function (g) {
      var n = g.nextElementSibling, any = false;
      while (n && !n.classList.contains('rb-ix-sec')) { if (!n.hidden) any = true; n = n.nextElementSibling; }
      g.hidden = !any;
    });
    var none = box.querySelector('.rb-ix-none');
    if (!shown && rows.length) {
      if (!none) {
        none = document.createElement('li');
        none.className = 'rb-ix-wait rb-ix-none';
        none.textContent = 'Nothing on this page matches. Let go of a choice or a word to see more.';
        box.querySelector('[data-ix-list]').appendChild(none);
      }
      none.hidden = false;
    } else if (none) {
      none.hidden = true;
    }
    var picked = [];
    (box._facets || []).forEach(function (f) { (sel[f.key] || []).forEach(function (v) { picked.push([f, v]); }); });
    var on = box.querySelector('.rb-ixf-on'), n = box.querySelector('.rb-ix-n');
    if (on) {
      on.innerHTML = picked.length ? '<span class="rb-ixf-cnt">' + shown + ' of ' + rows.length + '</span>' + picked.map(function (p) {
        return '<button type="button" class="rb-ixf-tok" data-ixf-drop="' + esc(p[0].key) + '" data-v="' + esc(p[1]) + '" aria-label="Stop narrowing by ' +
          esc(p[0].label + ' ' + p[1]) + '">' + esc(valueLabel(p[0].label, p[1])) + '<i aria-hidden="true">×</i></button>';
      }).join('') + '<button type="button" class="rb-ixf-clear" data-ixf-clear>Clear</button>' : '';
    }
    if (n) n.textContent = q && !picked.length ? shown + ' of ' + rows.length : '';
    [].forEach.call(box.querySelectorAll('[data-ixf-tab]'), function (b) {
      var c = (sel[b.getAttribute('data-ixf-tab')] || []).length;
      b.querySelector('.rb-ixf-n').textContent = c ? ' ' + c : '';
      b.classList.toggle('on', !!c);
    });
    var open = box.querySelector('[data-ixf-tab][aria-expanded="true"]');
    if (open) box.querySelector('.rb-ixf-drawer').innerHTML = this.filterDrawerHTML(box, open.getAttribute('data-ixf-tab'));
  };
  Rulebook.prototype.filterIndex = function (input) { this.applyFilters(input.closest('[data-ix]')); };

  // Clicks on the index tabs, their values and the chosen-values line.
  // Closes every open choices drawer but the one the click landed in.
  Rulebook.prototype.closeFilterDrawers = function (t) {
    [].forEach.call(this.el.querySelectorAll('.rb-ixf-drawer:not([hidden])'), function (dr) {
      var f = dr.closest('.rb-ixf');
      if (t && f && f.contains(t)) return;
      dr.hidden = true;
      [].forEach.call(f ? f.querySelectorAll('[data-ixf-tab]') : [], function (b) { b.setAttribute('aria-expanded', 'false'); });
    });
  };

  Rulebook.prototype.onIndexClick = function (t) {
    var box = t.closest('[data-ix]'), x;
    this.closeFilterDrawers(t);
    if (!box) return false;
    var dr = box.querySelector('.rb-ixf-drawer');
    if ((x = t.closest('[data-ixf-tab]'))) {
      var key = x.getAttribute('data-ixf-tab'), was = x.getAttribute('aria-expanded') === 'true';
      [].forEach.call(box.querySelectorAll('[data-ixf-tab]'), function (b) { b.setAttribute('aria-expanded', 'false'); });
      if (was) { dr.hidden = true; return true; }
      x.setAttribute('aria-expanded', 'true');
      dr.setAttribute('aria-label', x.firstChild.textContent);
      dr.innerHTML = this.filterDrawerHTML(box, key);
      // The drawer hangs from its tab's side line.
      dr.style.setProperty('--ixf-x', x.offsetLeft + 'px');
      dr.hidden = false;
      return true;
    }
    if ((x = t.closest('[data-ixf-val]'))) {
      var k = x.getAttribute('data-ixf-val'), v = x.getAttribute('data-v'), s = box._sel[k] = box._sel[k] || [];
      var i = s.indexOf(v);
      if (i >= 0) s.splice(i, 1); else s.push(v);
      this.applyFilters(box);
      var again = box.querySelector('[data-ixf-val="' + k + '"][data-v="' + (window.CSS && CSS.escape ? CSS.escape(v) : v) + '"]');
      if (again) again.focus();
      return true;
    }
    if ((x = t.closest('[data-ixf-drop]'))) {
      var dk = x.getAttribute('data-ixf-drop'), ds = box._sel[dk] || [];
      ds.splice(ds.indexOf(x.getAttribute('data-v')), 1);
      this.applyFilters(box);
      var f = box.querySelector('[data-ix-filter]'); if (f) f.focus({ preventScroll: true });
      return true;
    }
    if (t.closest('[data-ixf-clear]')) {
      box._sel = {};
      this.applyFilters(box);
      return true;
    }
    if (dr && !dr.hidden && !t.closest('.rb-ixf')) {
      dr.hidden = true;
      [].forEach.call(box.querySelectorAll('[data-ixf-tab]'), function (b) { b.setAttribute('aria-expanded', 'false'); });
    }
    return false;
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

  // --- Entries in place ------------------------------------------------------
  // A name written [[@category/id|shown]] opens its whole entry in the shared
  // hover card, without leaving the page. A rule word inside that card
  // follows in the same card, with a way back, so only one card is ever
  // open. Hold clips the entry to the book's edge as a slip that stays while
  // pages turn (a few at most, for this visit only); Turn to its page opens
  // it where its index lists it.

  var MAX_SLIPS = 3;

  // The card sits outside the book, so it carries the book's colours and
  // fonts with it. Only values that are plainly colours, lengths or font
  // names are copied.
  var BOOK_VARS = ['--rb-paper', '--rb-ink', '--rb-ink2', '--rb-mut', '--rb-edge', '--rb-acc', '--rb-box', '--rb-box2', '--rb-gold',
    '--rb-ui', '--rb-body', '--rb-head', '--rb-display', '--rb-steel', '--rb-on-steel', '--rb-hot', '--rb-t1', '--rb-t2', '--rb-t3'];
  Rulebook.prototype.bookVars = function () {
    var cs = window.getComputedStyle(this.el), out = [];
    BOOK_VARS.forEach(function (n) {
      var v = cs.getPropertyValue(n).trim();
      if (v && /^[#\w\s,.%()"'-]+$/.test(v)) out.push(n + ':' + v.replace(/"/g, "'"));
    });
    return out.join(';');
  };

  // An entry with its page's layout and where its index lists it. One fetch
  // per entry and view; a failure is forgotten so a later hover asks again.
  Rulebook.prototype.fetchEntry = function (cat, id) {
    var base = this.url.replace(/[?#].*$/, '').replace(/\/+$/, '');
    var url = base + '/entry/' + encodeURIComponent(cat) + '/' + encodeURIComponent(id) + this.viewParam('?');
    var cache = this.entryCache || (this.entryCache = {});
    if (!cache[url]) {
      cache[url] = Chronicle.apiFetch(url).then(function (res) {
        if (!res.ok) throw new Error('http');
        return res.json();
      }).then(function (j) {
        if (!isObj(j) || !isObj(j.entry)) throw new Error('shape');
        return {
          cat: cat, it: j.entry, list: indexList(j), chapter: SLUG_RE.test(str(j.chapter)) ? str(j.chapter) : '',
          page: intOr(j.page, 0, 100000, 0), kind: str(j.pageTitle)
        };
      });
      cache[url].catch(function () { delete cache[url]; });
    }
    return cache[url];
  };

  function shortName(it) { return entryName(it).replace(/\s*\([^)]*\)\s*$/, '') || entryName(it); }

  // What the hover card shows for an entry name: a Promise, so the card says
  // it is loading until the entry arrives, and closes if it never does.
  Rulebook.prototype.refCard = function (t) {
    var self = this;
    this.peekStack = [];
    return this.fetchEntry(t.getAttribute('data-ref-cat'), t.getAttribute('data-ref')).then(function (got) {
      self.peekNow = self.peekContent(got);
      return self.peekNow;
    });
  };

  Rulebook.prototype.peekContent = function (got) {
    var it = got.it, key = got.cat + '/' + str(it.id), meta = metaLine(it, got.list), back = this.peekStack && this.peekStack[this.peekStack.length - 1];
    var foot = '';
    if (!this.leafId) {
      foot = '<div class="rb-peek-foot">' +
        (got.chapter ? '<button type="button" class="rb-peek-go" data-peek-turn="' + esc(key) + '">Turn to its page</button>' : '<span></span>') +
        '<button type="button" class="rb-peek-hold" data-peek-hold="' + esc(key) + '" aria-label="Hold ' + esc(shortName(it)) +
        ' at the edge of the book">Hold</button></div>';
    }
    var c = {
      kind: got.kind, title: entryName(it),
      extraHTML: this.peekWrap((back ? this.crumbHTML(back) : '') + (meta ? '<p class="rb-ix-meta">' + meta + '</p>' : '') +
        entryBodyHTML(it, got.list, this.data.terms), foot)
    };
    c.got = got;
    return c;
  };

  // The card opens below its word, or above when there is more room there;
  // its body scrolls to fit that side, and the foot with Hold stays below it.
  var PEEK_HEAD = 140;
  Rulebook.prototype.peekWrap = function (inner, foot) {
    var cur = Chronicle.hovercard.current(), room = 520;
    if (cur && cur.getBoundingClientRect) {
      var r = cur.getBoundingClientRect();
      room = Math.max(160, Math.min(520, Math.max(r.top, window.innerHeight - r.bottom) - 16 - PEEK_HEAD));
    }
    return '<div class="rb-peek" data-look="' + esc(this.el.getAttribute('data-look') || '') + '" style="' + esc(this.bookVars()) +
      ';--rb-peek-max:' + Math.round(room) + 'px"><div class="rb-peek-body">' + inner + '</div>' + (foot || '') + '</div>';
  };
  Rulebook.prototype.crumbHTML = function (back) {
    return '<button type="button" class="rb-peek-back" data-peek-back>‹ Back to ' + esc(back.title) + '</button>';
  };

  // Clicks inside this book's peek card: rule words and entry names follow
  // in the card, Back returns, Hold and Turn to its page act on the book.
  Rulebook.prototype.onCardClick = function (e) {
    var hc = window.Chronicle && Chronicle.hovercard, t = e.target, x;
    if (!hc || !t || !t.closest || !t.closest('.chc .rb-peek')) return;
    var cur = hc.current();
    if (!cur || !this.el.contains(cur)) return;
    var self = this, stack = this.peekStack || (this.peekStack = []);
    var showing = this.peekNow;
    if ((x = t.closest('[data-peek-back]'))) {
      e.preventDefault(); e.stopPropagation();
      var prev = stack.pop();
      if (prev) { this.peekNow = prev; hc.update(prev); }
      return;
    }
    if ((x = t.closest('.rb-term'))) {
      e.preventDefault(); e.stopPropagation();
      if (!showing) return;
      if (x.hasAttribute('data-ref')) {
        this.fetchEntry(x.getAttribute('data-ref-cat'), x.getAttribute('data-ref')).then(function (got) {
          if (hc.current() !== cur) return;
          stack.push(showing);
          self.peekNow = self.peekContent(got);
          hc.update(self.peekNow);
        }, function () { /* the card keeps what it shows */ });
        return;
      }
      var def = this.data && has(this.data.terms, x.getAttribute('data-term')) ? this.data.terms[x.getAttribute('data-term')] : null;
      if (!def) return;
      stack.push(showing);
      this.peekNow = { kind: 'Rule', title: def.name, extraHTML: this.peekWrap(this.crumbHTML(showing) + '<p class="rb-peek-text">' + esc(def.text) + '</p>') };
      hc.update(this.peekNow);
      return;
    }
    if ((x = t.closest('[data-peek-hold]'))) {
      e.preventDefault(); e.stopPropagation();
      this.holdEntry(x.getAttribute('data-peek-hold'));
      return;
    }
    if ((x = t.closest('[data-peek-turn]')) && showing && showing.got) {
      e.preventDefault(); e.stopPropagation();
      hc.close();
      this.goEntry(showing.got);
    }
  };

  // Turns to the page whose index lists the entry and opens it there.
  Rulebook.prototype.goEntry = function (got) {
    for (var i = 0; i < this.items.length; i++) {
      var it = this.items[i];
      if (it.kind === 'page' && it.ch.id === got.chapter && it.p === it.ch.pages[got.page]) {
        this.openAt(i, str(got.it.id));
        return true;
      }
    }
    return this.goChapter(got.chapter);
  };

  Rulebook.prototype.holdEntry = function (key) {
    this.slips = (this.slips || []).filter(function (k) { return k !== key; });
    this.slips.push(key);
    if (this.slips.length > MAX_SLIPS) this.slips.shift();
    this.hideCard();
    this.paintSlips(key);
  };

  // The slips hang from the book's right edge, outside the pages, so turning
  // a page never takes them away. One is open at a time.
  Rulebook.prototype.paintSlips = function (openKey) {
    var self = this, rail = null;
    [].forEach.call(this.el.children, function (c) { if (c.classList.contains('rb-slips')) rail = c; });
    if (!this.slips || !this.slips.length) { if (rail) rail.remove(); return; }
    if (!rail) {
      rail = document.createElement('aside');
      rail.className = 'rb-slips';
      rail.setAttribute('aria-label', 'Held entries');
      this.el.appendChild(rail);
    }
    rail.innerHTML = '';
    this.slips.forEach(function (key) {
      var cut = key.indexOf('/'), open = key === openKey, slip = document.createElement('div');
      slip.className = 'rb-slip' + (open ? ' open' : '');
      slip.setAttribute('data-slip', key);
      slip.innerHTML = '<button type="button" class="rb-slip-tab" data-slip-tab aria-expanded="' + open + '">…</button>' +
        '<div class="rb-slip-body" role="region"' + (open ? '' : ' hidden') + '></div>';
      rail.appendChild(slip);
      self.fetchEntry(key.slice(0, cut), key.slice(cut + 1)).then(function (got) {
        if (self.dead || !slip.isConnected) return;
        var name = shortName(got.it), meta = metaLine(got.it, got.list), body = slip.querySelector('.rb-slip-body');
        slip.querySelector('.rb-slip-tab').textContent = name;
        body.setAttribute('aria-label', name);
        body.innerHTML = '<header><b>' + esc(entryName(got.it)) + '</b><button type="button" class="rb-slip-x" data-slip-drop aria-label="Let go of ' +
          esc(name) + '">×</button></header>' + (meta ? '<p class="rb-ix-meta">' + meta + '</p>' : '') + entryBodyHTML(got.it, got.list, self.data.terms);
      }, function () {
        // Not in this view (a Director's player preview): the slip goes.
        if (self.dead) return;
        self.slips = self.slips.filter(function (k) { return k !== key; });
        slip.remove();
        if (!self.slips.length && rail.isConnected) rail.remove();
      });
    });
  };

  // Clicks on a slip's tab and its close button.
  Rulebook.prototype.onSlipClick = function (t) {
    var slip = t.closest('.rb-slip'), x;
    if (!slip) return false;
    if ((x = t.closest('[data-slip-tab]'))) {
      var on = !slip.classList.contains('open');
      [].forEach.call(slip.parentNode.children, function (s) {
        s.classList.remove('open');
        s.querySelector('.rb-slip-body').hidden = true;
        s.querySelector('[data-slip-tab]').setAttribute('aria-expanded', 'false');
      });
      slip.classList.toggle('open', on);
      slip.querySelector('.rb-slip-body').hidden = !on;
      x.setAttribute('aria-expanded', String(on));
      return true;
    }
    if (t.closest('[data-slip-drop]')) {
      var key = slip.getAttribute('data-slip'), tab = null;
      this.slips = (this.slips || []).filter(function (k) { return k !== key; });
      this.paintSlips('');
      [].forEach.call(this.el.querySelectorAll('.rb-slip-tab'), function (b) { if (!tab) tab = b; });
      if (tab) tab.focus(); else if (this.searchEl) this.searchEl.focus({ preventScroll: true });
      return true;
    }
    return true;
  };

  // --- Search ----------------------------------------------------------------
  // Pages are searched here, in the book already loaded; rules-index entries
  // are searched by the server, which knows every entry's page.

  Rulebook.prototype.onInput = function (e) {
    var t = e.target;
    if (!t || !t.matches) return;
    if (t.matches('[data-ix-filter]')) { this.filterIndex(t); return; }
    if (t.matches('[data-cfind]')) { this.findCard(t); return; }
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
    var screen = this.screenPages.map(function (sp) { return { kind: 'page', ch: sp.ch, p: sp.p, n: 'screen' }; });
    screen.concat(this.items).forEach(function (it) {
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
    if (btn.getAttribute('data-hit') === 'screen') { this.closeResults(); this.openScreen(); return; }
    this.closeResults();
    this.openAt(+btn.getAttribute('data-hit'), btn.getAttribute('data-entry') || '');
  };

  // Turns to item n and, when id names an entry its index lists, opens it.
  Rulebook.prototype.openAt = function (n, id) {
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

  // One side of a whole spread: the spread laid out at full book width and
  // clipped to the left or right half, so a wide page can be cut at the spine.
  Rulebook.prototype.half = function (pages, side) {
    var h = document.createElement('div'), row = document.createElement('div');
    h.className = 'rb-half ' + side;
    row.className = 'rb-spread';
    pages.forEach(function (p) { row.appendChild(p); });
    h.appendChild(row);
    return h;
  };

  // Fresh pages for a spread, laid out as the book would show it.
  Rulebook.prototype.spreadPages = function (sp) {
    var l = this.makePage(sp.items[0], 'l');
    if (sp.wide) { l.classList.add('wide'); return [l]; }
    return [l, this.makePage(sp.items[1] || null, 'r')];
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
    this.backTo = this.pendingBack;
    this.pendingBack = null;
    this.addrOn = true;
    var oldSp = this.spreads[this.at], newSp = this.spreads[ns], fwd = ns > this.at;
    this.hideCard();
    if (reduced()) { this.at = ns; this.paint(true); this.settled(); return; }

    var style = this.data.turn, two = !this.narrow, cover = null;
    // A wide page fills the whole spread. A turn to or from one still turns a
    // leaf about the spine: each face shows its half of the spread.
    var whole = style === 'flip' && two && (oldSp.wide || newSp.wide);
    var leaf = document.createElement('div'), face, b;
    leaf.className = 'rb-leaf ' + style + (style === 'fade' ? '' : (fwd ? ' fwd' : ' back'));
    leaf.setAttribute('aria-hidden', 'true');
    leaf.setAttribute('inert', '');

    if (whole) {
      var oldPages = [this.snapshot(this.pl), this.snapshot(this.pr)];
      face = this.half(oldPages, fwd ? 'r' : 'l');
      face.classList.add('rb-face');
      face.insertAdjacentHTML('beforeend', '<div class="rb-shade"></div>');
      b = this.half(this.spreadPages(newSp), fwd ? 'l' : 'r');
      b.classList.add('rb-face', 'b');
      leaf.appendChild(face);
      leaf.appendChild(b);
      // Until the leaf lands, the half it has not reached yet still shows
      // the spread being left.
      cover = this.half([this.snapshot(this.pl), this.snapshot(this.pr)], fwd ? 'l' : 'r');
      cover.classList.add('rb-cover', fwd ? 'l' : 'r');
      cover.setAttribute('aria-hidden', 'true');
      this.at = ns;
      this.paint(false);
    } else if (style === 'flip') {
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
    if (cover) this.bookEl.appendChild(cover);
    this.bookEl.appendChild(leaf);
    [leaf, cover].forEach(function (layer) {
      if (!layer) return;
      [].forEach.call(layer.querySelectorAll('.rb-page'), function (p) {
        var s = p.querySelector('.rb-scroll');
        if (s && p._st) s.scrollTop = p._st;
      });
    });

    var done = false;
    function finish() {
      if (done) return;
      done = true;
      clearTimeout(self.leafTimer);
      self.finishNow = null;
      leaf.remove();
      if (cover) cover.remove();
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

  // goItem is a jump: a turn to a page chosen from a list, link or search.
  // One that moves more than a spread leaves a way back to the page it left.
  Rulebook.prototype.goItem = function (n) {
    var sheetWas = this.sheetLive(), ns = this.spreadOf(n);
    var here = this.spreads[this.at], there = this.spreads[ns];
    var far = !!here && !!there && !this.busy && Math.abs(ns - this.at) > 1;
    this.closeDrawer(false);
    this.closeScreen(false);
    this.refocus = true;
    if (far) this.pendingBack = { at: this.at, page: here.items[0].n + 1 };
    this.turnTo(ns);
    this.pendingBack = null;
    if (far && this.at === ns) this.jumpAddress(here.items[0], there.items[0], sheetWas);
    if (!this.busy) this.settled();
  };

  // --- The Sections bookmark -------------------------------------------------
  // A ribbon over the top of the right-hand page. Pulled down it lists every
  // chapter (and the pages of a chapter with several); picking one rolls it
  // back up first and only then turns the page, so the ribbon is never in
  // the way of the turn.

  Rulebook.prototype.bmHTML = function () {
    if (this.inline) return '';
    var id = this.domId('bm');
    return '<div class="rb-bm-anchor"><div class="rb-bm" data-open="false" hidden><div class="rb-bm-roll">' +
      '<button type="button" class="rb-bm-tab" data-act="bm" aria-expanded="false" aria-controls="' + id + '"><span>Sections</span>' +
      '<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M5 8l7 8 7-8z"/></svg></button>' +
      '<ul class="rb-bm-list" id="' + id + '"></ul></div></div></div>';
  };

  // A page with a package widget on it (a pop-up book to play) is the one kind
  // of page the Sections list lights up, so a reader can see where to play.
  function playable(p) {
    return arr(p && p.blocks).some(function (b) { return str(b.type) === 'widget' && SLUG_RE.test(str(b.widget)); });
  }

  Rulebook.prototype.bmListHTML = function () {
    var self = this, h = '', open = false, lastPart = null, num = 0, parts = {};
    var sp = this.spreads[this.at], cur = {}, curCh = sp ? sp.items[0].ch : null;
    if (sp) sp.items.forEach(function (it) { cur[it.n] = true; });
    this.items.forEach(function (it) { parts[it.part.title] = true; });
    var grouped = Object.keys(parts).length > 1, bold = this.data.look === 'bold';
    this.items.forEach(function (it) {
      if (it.kind === 'open' || it.solo) {
        if (open) h += '</li>';
        if (grouped && it.part !== lastPart) {
          lastPart = it.part;
          h += '<li class="rb-bm-part">' + esc(it.part.title) + '</li>';
        }
        num++;
        h += '<li><button type="button" data-bm="' + it.n + '"' + (it.ch === curCh ? ' aria-current="true"' : '') + (it.solo && playable(it.p) ? ' class="rb-bm-live"' : '') + '><i>' + (bold ? roman(num) : num) + '</i><span>' +
          esc(it.ch.title) + '</span>' + (it.ch.director && !it.part.director ? '<em>' + gmTerm() + '</em>' : '') + '</button>';
        open = true;
        if (it.kind === 'open' && it.pages.length > 1) {
          h += '<ul class="rb-bm-sub">' + it.pages.map(function (p) {
            return '<li><button type="button" data-bm="' + p.n + '"' + (cur[p.n] ? ' aria-current="true"' : '') + (playable(p.p) ? ' class="rb-bm-live"' : '') + '><span>' +
              esc(p.p.title || 'Untitled page') + '</span>' + (p.p.director && !it.ch.director ? '<em>' + gmTerm() + '</em>' : '') + '</button></li>';
          }).join('') + '</ul>';
        }
      }
    });
    if (open) h += '</li>';
    var browse = this.browseURL();
    if (browse) h += '<li class="rb-bm-all"><a href="' + esc(browse) + '">All reference lists</a></li>';
    return h;
  };

  // The category browser of the system the book belongs to, which has an
  // address of its own beside the book's.
  Rulebook.prototype.browseURL = function () {
    var m = /^(\/campaigns\/[A-Za-z0-9_-]+\/systems\/[A-Za-z0-9_-]+)\/book$/.exec(this.url || '');
    return !this.inline && m ? m[1] + '/browse' : '';
  };

  Rulebook.prototype.bmOpen = function () { return !!this.bmEl && this.bmEl.getAttribute('data-open') === 'true'; };

  Rulebook.prototype.bmSet = function (open, restoreFocus) {
    var self = this;
    if (!this.bmEl || open === this.bmOpen()) return;
    var roll = this.bmRoll, tab = this.bmTab, list = this.bmList;
    this.bmEl.setAttribute('data-open', open ? 'true' : 'false');
    tab.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (open) {
      this.closeDrawer(false);
      this.closeScreen(false);
      this.closeResults();
      this.hideCard();
      list.innerHTML = this.bmListHTML();
      // The ribbon is as long as its list, but never longer than 80% of the
      // window or the room left below it in the book; the list scrolls.
      var want = tab.offsetHeight + list.scrollHeight;
      var room = (this.bookEl || this.el).getBoundingClientRect().bottom - roll.getBoundingClientRect().top - 12;
      var cap = Math.max(tab.offsetHeight + 40, Math.min(window.innerHeight * 0.8, room));
      roll.style.height = Math.min(want, Math.round(cap)) + 'px';
      roll.style.overflowY = want > cap ? 'auto' : 'hidden';
      var first = list.querySelector('[aria-current="true"]') || list.querySelector('button');
      var id = setTimeout(function () {
        if (!self.dead && self.bmOpen() && first) first.focus({ preventScroll: true });
      }, reduced() ? 0 : BM_MS);
      this.timers.push(id);
    } else {
      var hadFocus = this.bmEl.contains(document.activeElement);
      roll.style.height = '';
      roll.style.overflowY = '';
      roll.scrollTop = 0;
      if (restoreFocus && hadFocus) tab.focus({ preventScroll: true });
    }
  };

  // The leaf turns when the ribbon's roll-up ends. A timer of BM_MS alone can
  // fire a frame early, turning over a ribbon still a pixel short; the longer
  // timer covers a roll-up that never runs.
  Rulebook.prototype.bmPick = function (n) {
    var self = this, roll = this.bmRoll, done = false;
    function go() {
      if (done) return;
      done = true;
      if (roll) roll.removeEventListener('transitionend', onEnd);
      if (!self.dead) self.goItem(n);
    }
    function onEnd(e) { if (e.target === roll && e.propertyName === 'height') go(); }
    this.bmSet(false, true);
    if (reduced() || !roll) { this.timers.push(setTimeout(go, 0)); return; }
    roll.addEventListener('transitionend', onEnd);
    this.timers.push(setTimeout(go, BM_MS * 2));
  };

  // --- Contents drawer -------------------------------------------------------

  Rulebook.prototype.drawerHTML = function () {
    var self = this, h = '', lastPart = null, sp = this.spreads[this.at];
    var cur = {};
    if (sp) sp.items.forEach(function (it) { cur[it.n] = true; });
    var curCh = sp ? sp.items[0].ch : null;
    if (this.screenPages.length) {
      h += '<button type="button" class="rb-dch rb-dscreen" data-act="screen"><span>' + esc(this.screenChapter().title) + '</span><span class="rb-pn">P</span></button>';
    }
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
    this.bmSet(false, false);
    this.hideCard();
    this.drawerEl.innerHTML = this.drawerHTML();
    this.drawerEl.inert = false;
    this.drawerEl.classList.add('open');
    this.scrimEl.classList.add('open');
    var self = this;
    this.letGoBack('releaseDrawer');
    this.releaseDrawer = this.holdBack('rb-contents', function () { self.closeDrawer(true); });
    this.tocBtn.setAttribute('aria-expanded', 'true');
    this.tocBtn.setAttribute('aria-label', 'Close the contents');
    var target = this.drawerEl.querySelector('[aria-current="true"]') || this.drawerEl.querySelector('button');
    if (target) requestAnimationFrame(function () { target.focus({ preventScroll: false }); });
  };

  Rulebook.prototype.closeDrawer = function (restoreFocus) {
    if (!this.drawerOpen()) return;
    this.letGoBack('releaseDrawer');
    var hadFocus = this.drawerEl.contains(document.activeElement);
    this.drawerEl.classList.remove('open');
    this.scrimEl.classList.remove('open');
    this.drawerEl.inert = true;
    this.tocBtn.setAttribute('aria-expanded', 'false');
    this.tocBtn.setAttribute('aria-label', 'Open the contents');
    if (restoreFocus && hadFocus) this.tocBtn.focus();
  };

  // --- Hover terms -----------------------------------------------------------

  // What a rule word's hover card says. Inside the book there is no
  // "open in the rulebook" link: the reader is already in it.
  Rulebook.prototype.termCard = function (t) {
    if (t.hasAttribute('data-ref')) return this.refCard(t);
    var key = t.getAttribute('data-term');
    var def = this.data && has(this.data.terms, key) ? this.data.terms[key] : null;
    return def ? { kind: 'Rule', title: def.name, text: def.text } : null;
  };

  // Closes the hover card when it belongs to this book.
  Rulebook.prototype.hideCard = function () {
    var hc = window.Chronicle && Chronicle.hovercard;
    var cur = hc && hc.current();
    if (cur && this.el.contains(cur)) hc.close();
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
    var key = this.currentKey(), tops = this.scrollTops();
    this.view = v;
    this.closeResults();
    this.rebuild(key);
    // Still on the same spread: keep the reader where they were on it.
    if (this.currentKey() === key) this.scrollTops(tops);
    // Held entries are asked for again as this view would see them.
    if (this.slips && this.slips.length) this.paintSlips('');
  };

  // Reads, or with an argument restores, how far each shown page is scrolled.
  Rulebook.prototype.scrollTops = function (tops) {
    return [this.pl, this.pr].map(function (page, i) {
      var s = page.querySelector('.rb-scroll');
      if (s && tops) s.scrollTop = tops[i];
      return s ? s.scrollTop : 0;
    });
  };

  Rulebook.prototype.onClick = function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    if (this.bmOpen() && !t.closest('.rb-bm')) this.bmSet(false, false);
    if (!t.closest('.rb-more')) this.setMore(false);
    if (!this.el.contains(t)) { this.closeResults(); this.closeFilterDrawers(null); return; }
    var x;
    if (!this.data) {
      if (t.closest('[data-act="retry"]')) this.load();
      return;
    }
    if ((x = t.closest('.rb-results [data-hit]'))) { this.pickResult(x); return; }
    if (this.onIndexClick(t)) return;
    if (this.onSlipClick(t)) return;
    if (!t.closest('.rb-search')) this.closeResults();
    if ((x = t.closest('[data-bm]'))) { this.bmPick(+x.getAttribute('data-bm')); return; }
    if ((x = t.closest('[data-go]'))) { this.goItem(+x.getAttribute('data-go')); return; }
    if ((x = t.closest('[data-tab]'))) { this.goChapter(x.getAttribute('data-tab')); return; }
    if ((x = t.closest('[data-chapter]'))) { this.goChapter(x.getAttribute('data-chapter')); return; }
    if ((x = t.closest('[data-act]'))) {
      switch (x.getAttribute('data-act')) {
        case 'prev': this.turnTo(this.at - 1); break;
        case 'next': if (this.frontShut) this.openFront(); else this.turnTo(this.at + 1); break;
        case 'edit': window.location.assign(x.getAttribute('data-href')); break;
        case 'toc': if (this.drawerOpen()) this.closeDrawer(true); else this.openDrawer(); break;
        case 'screen': if (this.screenOpen()) this.closeScreen(true); else this.openScreen(); break;
        case 'bm': this.bmSet(!this.bmOpen(), false); break;
        case 'front': this.openFront(); break;
        case 'jump-back': this.useBack(); break;
        case 'shut': this.shutFront(); break;
        // Going back in history returns the reader to their place on that page.
        case 'return': if (window.history.length > 1) { e.preventDefault(); window.history.back(); } break;
        case 'more': this.setMore(x.getAttribute('aria-expanded') !== 'true'); break;
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
    // A book shown inside another page only answers keys pressed in it.
    if (this.inline && !(t && this.el.contains(t))) return;
    if (t && t.closest) {
      if (this.searchKey(e, t)) return;
      if (t.closest('input,select,textarea,[contenteditable="true"]')) return;
      if (t.closest('.rb-wslot') && e.key !== 'Escape') return;
    }
    if (this.frontShut) return;
    if (e.key === 'Escape' && this.moreOpen()) { this.setMore(false); this.el.querySelector('.rb-more').focus(); return; }
    if (e.key === 'Escape' && this.bmOpen()) { this.bmSet(false, true); return; }
    if (e.key === 'Escape') {
      if (this.screenOpen()) { this.closeScreen(true); return; }
      this.closeDrawer(true); this.hideCard(); this.closeResults(); return;
    }
    if ((e.key === 'p' || e.key === 'P') && !e.altKey && !e.ctrlKey && !e.metaKey && this.ribbonEl && !this.ribbonEl.hidden) {
      e.preventDefault();
      if (this.screenOpen()) this.closeScreen(true); else this.openScreen();
      return;
    }
    if (this.screenOpen()) return;
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
      var ok = !this.frontShut && t && t.closest && t.closest('.rb-book') && !t.closest('.rb-drawer, .rb-screen, .rb-wslot, .rb-roll, .rb-ixf, select');
      this.touch = ok ? { x: p.clientX, y: p.clientY } : null;
      return;
    }
    var s = this.touch;
    this.touch = null;
    if (!s || this.drawerOpen()) return;
    var dx = p.clientX - s.x, dy = p.clientY - s.y;
    if (Math.abs(dx) >= SWIPE_PX && Math.abs(dy) < Math.abs(dx) * 0.6) this.turnTo(this.at + (dx < 0 ? 1 : -1));
  };

  // --- Leaf mode -------------------------------------------------------------
  // One chapter's pages, one at a time, drawn by the book's own page renderer
  // and set in another page. Nothing here reads or writes the reader's saved
  // place, and nothing listens outside the leaf, so any number can share a page
  // with the app's own shortcuts.

  Rulebook.prototype.startLeaf = function () {
    var self = this, det = this.el.closest('details');
    this.el.classList.add('rb', 'rb-torn');
    this.on(this.el, 'click', function (e) { self.onLeafClick(e); });
    this.on(this.el, 'input', function (e) { self.onInput(e); });
    if (window.Chronicle && Chronicle.hovercard) {
      this.unbindCard = Chronicle.hovercard.bind(this.el, '.rb-term', function (t) { return self.termCard(t); }, { pinOnClick: true });
    }
    this.on(document, 'click', function (e) { self.onCardClick(e); }, true);
    // Closed, the leaf costs nothing: the book is asked for when the fold-out
    // it sits in first opens.
    if (!det || det.open) { this.leafStarted = true; this.load(); }
    else {
      this.on(det, 'toggle', function () {
        if (det.open && !self.leafStarted) { self.leafStarted = true; self.load(); }
      });
    }
    if (det && this.leafPreview) this.bindPreview(det);
  };

  // The row's hover preview is the shared hover card; it describes the
  // chapter's first page in plain text and never draws a popup of its own.
  Rulebook.prototype.bindPreview = function (det) {
    var self = this, sum = det.querySelector(':scope > summary');
    if (!sum || !window.Chronicle || !Chronicle.hovercard) return;
    this.unbindPreview = Chronicle.hovercard.bind(sum, 'summary', function (t) { return self.previewCard(det, t); });
    this.on(det, 'toggle', function () {
      var hc = Chronicle.hovercard;
      if (det.open && hc.current() === sum) hc.close();
    });
  };

  function keyboardFocus(el) {
    try { return el.matches(':focus-visible'); } catch (e) { return true; }
  }

  Rulebook.prototype.previewCard = function (det, row) {
    // An open rule already shows the page; a touch screen has no resting
    // pointer, and a row focused by a press is about to open.
    if (det.open || !media('(hover: hover)')) return null;
    if (row === document.activeElement && !keyboardFocus(row)) return null;
    var id = this.leafId;
    return sharedBook(this.url).then(function (json) {
      var data = normalize(json);
      return data ? previewOf(data, id) : null;
    }).catch(function () { return null; });
  };

  // previewOf is the hover card's content for a chapter: its title, the start
  // of its first page in plain words, and how long it is. Director-only pages
  // and blocks are left out, whoever asks, so a preview never carries them
  // onto a shared screen.
  function previewOf(data, id) {
    var dir = data.isDirector, found = null;
    data.parts.forEach(function (part) {
      if (part.director && !dir) return;
      part.chapters.forEach(function (ch) { if (ch.id === id && !(ch.director && !dir)) found = ch; });
    });
    if (!found) return null;
    var pages = found.pages.filter(function (p) { return !p.director && p.blocks.some(function (b) { return !b.director && str(b.type) !== 'note'; }); });
    var first = pages[0], text = '';
    if (first) {
      first.blocks.some(function (b) {
        var type = str(b.type);
        if (b.director === true || (type !== 'text' && type !== 'callout')) return false;
        text = plain(str(b.text)).replace(/\s+/g, ' ').trim();
        return !!text;
      });
    }
    if (!text) text = found.intro;
    if (text.length > PREVIEW_CHARS) text = text.slice(0, PREVIEW_CHARS).replace(/\s+\S*$/, '') + '…';
    return {
      kind: 'Rulebook', title: found.title, text: text,
      foot: pages.length ? pages.length + (pages.length === 1 ? ' page' : ' pages') + (first.title ? ' · ' + first.title : '') : ''
    };
  }

  Rulebook.prototype.openLeaf = function (data) {
    this.data = data;
    this.applyTheme(data.theme);
    this.view = data.isDirector ? 'director' : 'player';
    this.unloadFonts();
    this.loadFonts();
    this.buildItems();
    var id = this.leafId;
    // The chapter's own pages, without its opener: the row above the leaf
    // already shows the introduction.
    this.leafItems = this.items.filter(function (it) { return it.kind === 'page' && it.ch.id === id; });
    this.leafItems.forEach(function (it) { it.wide = false; });
    this.leafAt = 0;
    this.el.innerHTML = '<div class="rb-tear"></div><div class="rb-sr" aria-live="polite"></div>';
    this.el.setAttribute('aria-busy', 'false');
    this.tearEl = this.el.querySelector('.rb-tear');
    this.srEl = this.el.querySelector('.rb-sr');
    this.paintLeaf(false, '');
  };

  // Where "Open the book at this page" goes: the full book, at this chapter.
  Rulebook.prototype.leafHref = function () {
    if (/^\/(?!\/)[^\s"'<>\\]*$/.test(this.openHref)) return this.openHref;
    return this.url.replace(/[?#].*$/, '').replace(/\/book\/?$/, '') + '#' + encodeURIComponent(this.leafId);
  };

  Rulebook.prototype.leafFootHTML = function (it, prev, next) {
    var n = this.leafItems.length, words = '';
    if (prev) words += '<button type="button" class="rb-tw" data-act="leaf-prev">Previous</button>';
    if (next) words += '<button type="button" class="rb-tw" data-act="leaf-next">Next' + (next.p.title ? ': ' + esc(next.p.title) : '') + '</button>';
    return '<nav class="rb-tnav" aria-label="Pages of ' + esc(it.ch.title) + '">' +
      '<div class="rb-trow">' + (words ? '<span class="rb-twords">' + words + '</span>' : '<span></span>') +
      '<span class="rb-folio">' + (it.n + 1) + '</span></div>' +
      '<div class="rb-trow"><a class="rb-tw" href="' + esc(this.leafHref()) + '">Open the book at this page</a>' +
      (n > 1 ? '<span class="rb-tcount">' + (this.leafAt + 1) + ' of ' + n + '</span>' : '') + '</div></nav>';
  };

  Rulebook.prototype.paintLeaf = function (turned, focusAct) {
    var items = this.leafItems, it = items[this.leafAt];
    this.hideCard();
    this.destroyNested(this.tearEl);
    if (this.ro) { this.ro.disconnect(); this.ro = null; }
    this.tearH = 0;
    if (!it) {
      this.tearEl.innerHTML = '<p class="rb-empty">This page is not in the book you can read.</p>';
      return;
    }
    this.tearEl.innerHTML = '<article class="rb-page' + (turned ? ' rb-in' : '') + '">' +
      this.pageHTML(it, this.leafFootHTML(it, items[this.leafAt - 1], items[this.leafAt + 1])) + '</article>';
    // The page grows with its words and never scrolls inside the leaf, so it
    // is not a tab stop.
    var sc = this.tearEl.querySelector('.rb-scroll');
    if (sc) sc.removeAttribute('tabindex');
    this.mountNested(this.tearEl);
    this.el.classList.add('rb-ready');
    this.shapeEdge();
    if (turned) {
      this.srEl.textContent = 'Page ' + (it.n + 1) + ' of ' + this.items.length + ': ' + it.ch.title + (it.p.title ? ', ' + it.p.title : '');
    }
    if (focusAct) {
      // The control just used may be gone (the last page has no Next).
      var other = focusAct === 'leaf-next' ? 'leaf-prev' : 'leaf-next';
      var f = this.tearEl.querySelector('[data-act="' + focusAct + '"]') || this.tearEl.querySelector('[data-act="' + other + '"]') ||
        this.tearEl.querySelector('.rb-tnav a');
      if (f) f.focus({ preventScroll: true });
    }
  };

  // A tear drawn to the page's own height: points every few pixels down the
  // binding edge, jittered from the chapter and page so the same page always
  // tears the same way. The stylesheet carries a fixed tear for when
  // ResizeObserver is missing.
  function tornEdge(seed, h) {
    var x = 0, i;
    for (i = 0; i < seed.length; i++) x = (x * 31 + seed.charCodeAt(i)) >>> 0;
    function rand() { x = (x * 1664525 + 1013904223) >>> 0; return x / 4294967296; }
    var n = Math.max(6, Math.round(h / 12)), pts = [];
    for (i = 0; i <= n; i++) {
      var r = rand(), dx = r > 0.82 ? 5 + Math.round(rand() * 4) : Math.round(r * 3);
      pts.push(dx + 'px ' + Math.round(i * h / n) + 'px');
    }
    return 'polygon(' + pts.join(', ') + ', 100% ' + h + 'px, 100% 0)';
  }

  Rulebook.prototype.shapeEdge = function () {
    var self = this, page = this.tearEl.querySelector('.rb-page');
    if (!page || !window.ResizeObserver) return;
    var seed = this.leafId + ':' + this.leafAt;
    var apply = function () {
      var h = page.offsetHeight;
      // Hidden in a closed fold-out the page has no height yet.
      if (h <= 0 || h === self.tearH) return;
      self.tearH = h;
      page.style.clipPath = tornEdge(seed, h);
    };
    apply();
    this.ro = new ResizeObserver(apply);
    this.ro.observe(page);
  };

  Rulebook.prototype.turnLeaf = function (d) {
    var at = clamp(this.leafAt + d, 0, this.leafItems.length - 1);
    if (at === this.leafAt) return;
    this.leafAt = at;
    this.paintLeaf(true, d > 0 ? 'leaf-next' : 'leaf-prev');
  };

  // A link inside a page to another chapter opens the full book there: the
  // leaf shows one chapter only.
  Rulebook.prototype.leaveFor = function (id) {
    if (!SLUG_RE.test(id || '')) return;
    window.location.assign(this.leafHref().replace(/#.*$/, '') + '#' + encodeURIComponent(id));
  };

  Rulebook.prototype.onLeafClick = function (e) {
    var t = e.target, x;
    if (!t || !t.closest) return;
    if (!this.data) {
      if (t.closest('[data-act="retry"]')) this.load();
      return;
    }
    if ((x = t.closest('[data-chapter]'))) { this.leaveFor(x.getAttribute('data-chapter')); return; }
    if ((x = t.closest('[data-go]'))) {
      var item = this.items[+x.getAttribute('data-go')];
      if (item) this.leaveFor(item.ch.id);
      return;
    }
    if ((x = t.closest('[data-act]'))) {
      switch (x.getAttribute('data-act')) {
        case 'leaf-prev': this.turnLeaf(-1); break;
        case 'leaf-next': this.turnLeaf(1); break;
        case 'roll': this.doRoll(x.closest('[data-roll]')); break;
        case 'prev-step': this.stepExample(x, -1); break;
        case 'next-step': this.stepExample(x, 1); break;
      }
      return;
    }
    if ((x = t.closest('.rb-fc'))) { this.foldCard(x); return; }
    if (this.onIndexClick(t)) return;
    if ((x = t.closest('.rb-flap > button'))) { this.toggleFlap(x); }
  };

  // Reuse by the page editor: draws one wire-format page (the same shape the
  // book JSON carries) with the book's own block renderers. ctx: { terms,
  // parts, chapterId, director } where director=false previews the player
  // view and parts feed a contents block. The result is static markup; widget
  // blocks are not mounted.
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
      r.data = { terms: terms, isDirector: true, parts: arr(ctx.parts) };
      r.view = ctx.director === false ? 'player' : 'director';
      r.paintingChapter = str(ctx.chapterId);
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
