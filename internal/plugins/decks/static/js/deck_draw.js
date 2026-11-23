/**
 * Deck draw
 *
 * The card draw overlay and what opens it (internal/plugins/decks, option "Pull
 * from the fan"). One script, loaded only where it is used (the Table's
 * script loads it on the first press of a Draw button, when the DM Screen
 * shows its Card decks section, or when a dealt card is waiting):
 *
 *   DeckDraw.open({campaignId, deckId, from, viewer, heroId, startCopy})
 *       the overlay: the pile fans out as backs, the person pulls one, the card
 *       waits face down until they turn it.
 *   DeckDraw.arrive({campaignId, card})
 *       the pop-up for a card a Director dealt: one face-down card.
 *   [data-dkd-screen]  the DM Screen's Card decks section (self-mounting).
 *
 * The server draws every fragment; this file sends the person's moves to the
 * deck routes and shows what the server then says. A face is asked for only
 * when the person turns the card, through the turn the deck has chosen
 * (DeckCards.turnOver). Nothing about a card (id, name, words, picture) is on
 * a player's screen before that answer: a pulled back is only a copy id the
 * server handed over. Text from the server reaches the page as its own markup
 * or through textContent.
 */
(function () {
  'use strict';
  if (window.DeckDraw) return;

  var C = (window.Chronicle = window.Chronicle || {});
  var LIVE_EVENT = 'chronicle:decks-changed';
  // Said when an overlay has closed, so a dealt card that waited behind it can pop up.
  var CLOSED_EVENT = 'chronicle:deck-draw-closed';
  var MAX_SHOWN = 5;
  var OFFLINE = 'That did not go through. Check your connection and try again.';
  var DRAW_FAILED = 'The draw failed. Pull again to try again.';
  var GONE = 'That card is not here any more.';
  var CLOSE_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"></path></svg>';
  var HEADERS = { 'Accept': 'application/json, text/html' };

  // ---- Small helpers ----

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }
  function el(html) { var t = document.createElement('template'); t.innerHTML = html.trim(); return t.content.firstElementChild; }
  function wait(ms) { return new Promise(function (r) { setTimeout(r, ms); }); }
  function esc(s) { return window.Chronicle.escapeHtml(s); }
  function enc(s) { return encodeURIComponent(s); }

  /** cap: a server refusal as a sentence (it starts in lower case on the wire). */
  function cap(m) {
    m = m ? String(m) : '';
    if (!m) return '';
    m = m.charAt(0).toUpperCase() + m.slice(1);
    return /[.!?]$/.test(m) ? m : m + '.';
  }

  /** level: how much motion the page allows ('full' | 'calm' | 'off'); a device asking for less is Off. */
  function level() {
    var reduced = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    var attr = document.documentElement.getAttribute('data-motion');
    if (window.DeckCards && window.DeckCards.levelOf) return window.DeckCards.levelOf(attr, reduced);
    if (reduced) return 'off';
    return attr === 'calm' || attr === 'off' ? attr : 'full';
  }

  /** tok: a CSS custom property's value from the page, or the fallback. */
  function tok(name, fallback) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || fallback;
  }

  /** speed: the owner's Motion speed retimes --dur-standard; every move here scales with it. */
  function speed() {
    var v = parseFloat(tok('--dur-standard', '200'));
    return v > 0 ? v / 200 : 1;
  }
  function ms(n) { return Math.round(n * speed()); }

  function anim(node, frames, dur, easing, extra) {
    if (!node || !node.animate) return Promise.resolve();
    var a = node.animate(frames, Object.assign({ duration: ms(dur), easing: easing || 'linear', fill: 'both' }, extra || {}));
    return a.finished.then(function () { return a; }, function () { return a; });
  }
  function settle(a) { try { if (a && a.cancel) a.cancel(); } catch (e) { /* gone */ } }
  var OUT = function () { return tok('--ease-out', 'ease-out'); };
  var IN = function () { return tok('--ease-in', 'ease-in'); };

  /** fade: opacity only, what Calm does for everything that would travel. */
  function fade(node, from, to, dur, easing) {
    if (level() === 'off') return Promise.resolve();
    return anim(node, [{ opacity: from }, { opacity: to }], dur, easing || OUT()).then(function (a) { if (to === 1) settle(a); });
  }
  function center(r) { return { x: r.left + r.width / 2, y: r.top + r.height / 2 }; }

  /** flyIn: a card already at its place arrives from another rectangle (placed, no turn, under 500ms). */
  function flyIn(node, from, dur) {
    var lv = level();
    if (lv === 'off' || !node) return Promise.resolve();
    if (lv === 'calm') return fade(node, 0, 1, 200);
    var to = node.getBoundingClientRect(), a = center(from), b = center(to);
    var s = Math.max(0.15, Math.min(1.5, from.width / (to.width || 1)));
    return anim(node, [
      { transform: 'translate(' + (a.x - b.x) + 'px,' + (a.y - b.y) + 'px) scale(' + s + ')', opacity: 0 },
      { opacity: 1, offset: 0.25 },
      { transform: 'none', opacity: 1 }
    ], dur || 420, OUT()).then(settle);
  }

  /** flyOut: a card at its place leaves toward another rectangle. */
  function flyOut(node, toRect, dur) {
    var lv = level();
    if (lv === 'off') return Promise.resolve();
    if (lv === 'calm') return fade(node, 1, 0, 200, IN()).then(function () { node.style.opacity = '0'; });
    var from = node.getBoundingClientRect(), a = center(from), b = center(toRect);
    var s = Math.max(0.15, Math.min(1.5, toRect.width / (from.width || 1)));
    return anim(node, [
      { transform: 'none', opacity: 1 },
      { opacity: 1, offset: 0.7 },
      { transform: 'translate(' + (b.x - a.x) + 'px,' + (b.y - a.y) + 'px) scale(' + s + ')', opacity: 0 }
    ], dur || 380, tok('--ease-in-out', 'ease-in-out'));
  }

  /** peek: a fade and a 4px rise. Calm fades only. */
  function peek(node) {
    var lv = level();
    if (lv === 'off') return Promise.resolve();
    var frames = lv === 'calm' ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 0, transform: 'translateY(4px)' }, { opacity: 1, transform: 'none' }];
    return anim(node, frames, lv === 'calm' ? 200 : 120, OUT()).then(settle);
  }

  /** fold: to the content's measured height; 200ms open, 150ms closed. Calm fades, Off jumps. */
  function fold(box, open) {
    var lv = level();
    if (open) {
      box.hidden = false;
      if (lv === 'off') return Promise.resolve();
      if (lv === 'calm') return fade(box, 0, 1, 200);
      var h = box.scrollHeight;
      box.style.overflow = 'hidden';
      return anim(box, [{ height: '0px', opacity: 0 }, { height: h + 'px', opacity: 1 }], 200, OUT()).then(function (a) { settle(a); box.style.overflow = ''; });
    }
    if (lv === 'off') { box.hidden = true; return Promise.resolve(); }
    if (lv === 'calm') return fade(box, 1, 0, 150, IN()).then(function () { box.hidden = true; });
    var h2 = box.getBoundingClientRect().height;
    box.style.overflow = 'hidden';
    return anim(box, [{ height: h2 + 'px', opacity: 1 }, { height: '0px', opacity: 0 }], 150, IN()).then(function (a) { box.hidden = true; settle(a); box.style.overflow = ''; });
  }

  /** grow: a small picker grows out of what opened it (120ms). */
  function grow(menu) {
    var lv = level();
    if (lv === 'off') return Promise.resolve();
    var frames = lv === 'calm' ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 0, transform: 'scale(.96)' }, { opacity: 1, transform: 'none' }];
    return anim(menu, frames, 120, OUT()).then(settle);
  }

  /** deckMove: a fixed move from DeckCards ('play', 'deal') on a real card. */
  function deckMove(card, name, dx, dy) {
    if (!window.DeckCards || !window.DeckCards.move) return Promise.resolve();
    return window.DeckCards.move(card, name, { dx: dx, dy: dy });
  }

  // ---- The network ----

  /**
   * send: one request. A refusal carries the server's own sentence, a lost
   * connection is marked offline so each screen can say it in its own words.
   */
  function send(method, url, body) {
    var opts = { method: method, headers: HEADERS };
    if (body !== undefined) opts.body = body;
    var call = C.apiFetch ? C.apiFetch(url, opts) : fetch(url, { method: method, credentials: 'same-origin', headers: HEADERS, body: body === undefined ? undefined : JSON.stringify(body) });
    return Promise.resolve(call).then(function (res) {
      if (res.ok) return res;
      return res.text().then(function (text) {
        var data = {};
        try { data = JSON.parse(text); } catch (e) { data = {}; }
        var e = new Error((data && (data.message || data.error)) || '');
        e.status = res.status;
        e.words = res.status === 404 ? GONE : (cap(e.message) || 'That did not go through. Try again.');
        throw e;
      });
    }, function () {
      var e = new Error('offline');
      e.offline = true;
      e.words = OFFLINE;
      throw e;
    });
  }
  function textOf(res) { return res.text(); }
  function jsonOf(res) { return res.text().then(function (t) { try { return JSON.parse(t); } catch (e) { return {}; } }); }

  function baseOf(campaignId) { return '/campaigns/' + enc(campaignId) + '/decks'; }

  /** compact: a face-up card's markup at another size, without its leaf and without a widget to mount. */
  function resize(html, size) {
    var t = document.createElement('template');
    t.innerHTML = html;
    $$('.dk-leaf', t.content).forEach(function (n) { n.parentNode.removeChild(n); });
    var w = $('.dk-wrap', t.content);
    var out = w || t.content.firstElementChild;
    if (!out) return null;
    out.removeAttribute('data-widget');
    out.removeAttribute('data-dk-card');
    out.removeAttribute('tabindex');
    out.classList.remove('dk-open');
    var s = out.outerHTML.replace(/dk-sz-(open|row|hand)/g, 'dk-sz-' + size);
    return el(s);
  }
  function labelOf(node) {
    var c = node && node.querySelector('.dk-card');
    return (c && c.getAttribute('aria-label')) || '';
  }
  /** leafOf: the leaf's markup out of a face-up open card, '' when it has none. */
  function leafOf(html) {
    var t = document.createElement('template');
    t.innerHTML = html;
    var l = $('.dk-leaf', t.content);
    return l ? l.outerHTML : '';
  }

  /** heroesIn: the party's heroes the DM Screen section carries for Deal to. */
  function heroesIn() {
    var t = $('template[data-dkd-heroes]');
    if (!t) return [];
    var out = [];
    $$('[data-hero]', t.content).forEach(function (b) { out.push({ id: b.getAttribute('data-hero'), name: b.textContent.trim() }); });
    return out;
  }

  // ---- The overlay shell ----

  var current = null;

  function tabbables(box) {
    return $$('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])', box)
      .filter(function (n) { return n.tabIndex >= 0 && (n.offsetParent !== null || n === document.activeElement); });
  }
  function isPhone() { return !!(window.matchMedia && window.matchMedia('(max-width: 700px)').matches); }
  function topBarBottom() { var h = $('#app-topbar'); return h ? Math.round(h.getBoundingClientRect().bottom) : 56; }

  /**
   * shell: scrim, panel, head and close mark; modal with a focus trap, Escape,
   * scroll lock and focus back to the opener. A press on the scrim does not
   * close it. Placed on the body because the Table drawer clips fixed layers.
   * Returns the ctx the content mounts into.
   */
  function shell(o) {
    var phone = isPhone();
    var node = el('<div class="dkd-ov" data-dkd-ov' + (phone ? ' data-phone' : '') + (o.arrive ? ' data-arrive' : '') + '>' +
      '<div class="dkd-scrim" data-dkd-scrim></div>' +
      '<section class="dkd-panel" role="dialog" aria-modal="true" aria-labelledby="dkd-title" aria-describedby="dkd-sub" tabindex="-1">' +
      '<header class="dkd-head"><h2 class="dkd-title" id="dkd-title" tabindex="-1"></h2><p class="dkd-subhead" id="dkd-sub"></p>' +
      '<button type="button" class="dkd-x" data-dkd-close aria-label="Close">' + CLOSE_SVG + '</button></header>' +
      '<div class="dkd-body" data-dkd-body></div>' +
      '<p class="dkd-live" role="status" aria-live="polite" data-dkd-live></p></section></div>');
    var panel = $('.dkd-panel', node), scrim = $('.dkd-scrim', node), body = $('[data-dkd-body]', node);
    var sub = $('.dkd-subhead', node), live = $('[data-dkd-live]', node);
    $('.dkd-title', node).textContent = o.title || '';
    sub.textContent = o.sub || '';
    if (phone) node.style.setProperty('--dkd-top', topBarBottom() + 'px');
    var from = o.from;
    var opener = from && from.isConnected ? from : document.activeElement;
    var fromRect = from && from.isConnected ? from.getBoundingClientRect() : { left: innerWidth / 2 - 60, top: innerHeight / 2 - 20, width: 120, height: 40 };
    document.body.appendChild(node);
    document.documentElement.classList.add('dkd-locked');
    var inerted = [];
    $$('body > *').forEach(function (n) { if (n !== node && !n.hasAttribute('inert')) { n.setAttribute('inert', ''); inerted.push(n); } });

    var escapes = [], closing = null, instance = null, readyResolve;
    var ctx = {
      node: node, panel: panel, body: body, phone: phone,
      setSub: function (t) { sub.textContent = t || ''; },
      say: function (t) { live.textContent = ''; setTimeout(function () { live.textContent = t || ''; }, 30); },
      onEscape: function (fn) { escapes.push(fn); return function () { escapes = escapes.filter(function (f) { return f !== fn; }); }; },
      close: function () { return close(); }
    };
    ctx.ready = new Promise(function (r) { readyResolve = r; });

    function onKey(e) {
      if (e.key === 'Escape') {
        e.preventDefault(); e.stopPropagation();
        if (escapes.length && escapes[escapes.length - 1]() === true) return;
        close();
        return;
      }
      if (e.key === 'Tab') {
        var t = tabbables(panel);
        if (!t.length) { e.preventDefault(); panel.focus(); return; }
        var first = t[0], last = t[t.length - 1], a = document.activeElement;
        if (!panel.contains(a)) { e.preventDefault(); first.focus(); }
        else if (e.shiftKey && (a === first || a === panel)) { e.preventDefault(); last.focus(); }
        else if (!e.shiftKey && a === last) { e.preventDefault(); first.focus(); }
      }
    }
    function onFocusIn(e) { if (!panel.contains(e.target)) { var t = tabbables(panel); (t[0] || panel).focus(); } }
    document.addEventListener('keydown', onKey, true);
    document.addEventListener('focusin', onFocusIn, true);
    $('[data-dkd-close]', node).addEventListener('click', function () { close(); });

    // Open: grow out of the pressed thing (~200ms); Calm fades; Off at once.
    var lv = level(), opening;
    if (lv === 'off') opening = Promise.resolve();
    else if (lv === 'calm') opening = Promise.all([fade(panel, 0, 1, 200), fade(scrim, 0, 1, 200)]);
    else {
      var to = panel.getBoundingClientRect();
      var sx = fromRect.width / to.width, sy = fromRect.height / to.height;
      opening = Promise.all([
        anim(panel, [{ transform: 'translate(' + (fromRect.left - to.left) + 'px,' + (fromRect.top - to.top) + 'px) scale(' + sx + ',' + sy + ')', opacity: 0 }, { opacity: 1, offset: 0.55 }, { transform: 'none', opacity: 1 }], 200, OUT()),
        anim(scrim, [{ opacity: 0 }, { opacity: 1 }], 200, OUT())
      ]).then(function (as) { as.forEach(settle); });
    }
    instance = o.mount(ctx) || {};
    opening.then(function () { panel.focus({ preventScroll: true }); readyResolve(); });

    function close() {
      if (closing) return closing;
      var lv2 = level();
      var back = from && from.isConnected ? from.getBoundingClientRect() : fromRect;
      var out;
      if (lv2 === 'off') out = Promise.resolve();
      else if (lv2 === 'calm') out = Promise.all([fade(panel, 1, 0, 150, IN()), fade(scrim, 1, 0, 150, IN())]);
      else {
        var cur = panel.getBoundingClientRect();
        out = Promise.all([
          anim(panel, [{ transform: 'none', opacity: 1 }, { opacity: 1, offset: 0.45 }, { transform: 'translate(' + (back.left - cur.left) + 'px,' + (back.top - cur.top) + 'px) scale(' + (back.width / cur.width) + ',' + (back.height / cur.height) + ')', opacity: 0 }], 150, IN()),
          anim(scrim, [{ opacity: 1 }, { opacity: 0 }], 150, IN())
        ]);
      }
      closing = out.then(function () {
        document.removeEventListener('keydown', onKey, true);
        document.removeEventListener('focusin', onFocusIn, true);
        try { if (instance.destroy) instance.destroy(); } catch (e) { /* the content is gone either way */ }
        node.remove();
        document.documentElement.classList.remove('dkd-locked');
        inerted.forEach(function (n) { n.removeAttribute('inert'); });
        current = null;
        var back2 = (opener && opener.isConnected) ? opener : (from && from.isConnected ? from : null);
        if (back2 && back2.focus) back2.focus({ preventScroll: true });
        if (o.onClose) o.onClose();
        window.dispatchEvent(new CustomEvent(CLOSED_EVENT));
      });
      return closing;
    }
    ctx.instance = function () { return instance; };
    return ctx;
  }

  // ---- The draw overlay: the fan, the stage and the drawn row ----

  /**
   * mountDraw: the content of the overlay. `view` is the server's draw-view
   * answer; every number and refusal on screen comes from it, read again after
   * each move. Moves run one at a time.
   */
  function mountDraw(ctx, o, view) {
    var phone = ctx.phone, isDir = o.viewer === 'director';
    var base = baseOf(o.campaignId), did = o.deckId;
    var tall = view.shape === 'tall';
    var heroes = isDir ? heroesIn() : [];
    var items = [];            // cards held in this visit: { id, turned, up, leaf, label }
    var cur = null, cardEl = null;
    var busy = false, dead = false, again = false, idleP = Promise.resolve();
    var drewOnce = false, closers = [], timer = 0;

    // Fan geometry: backs overlap and swing about a pivot well below them.
    var NMAX = phone ? 5 : 7, STEP = phone ? 12.4 : 6.3, RAD = phone ? 300 : 520, BW = 92;
    var BH = Math.round(BW * (tall ? 12 / 7 : 7 / 5));
    var AMAX = (NMAX - 1) / 2 * STEP * Math.PI / 180;
    var FULLH = Math.round(14 + BH + RAD * (1 - Math.cos(AMAX)) + 10);
    var FANH = phone ? 120 : FULLH;

    // ---- the server ----
    var api = {
      view: function () { return send('GET', base + '/' + enc(did) + '/draw-view').then(jsonOf); },
      draw: function () {
        var body = isDir ? { count: 1 } : { heroId: view.heroId || o.heroId, count: 1 };
        return send('POST', base + '/' + enc(did) + '/draw', body).then(jsonOf).then(function (j) {
          var id = (j && j.copyIds && j.copyIds[0]) || (j && j.copyId);
          if (!id) { var e = new Error('no card'); e.words = DRAW_FAILED; throw e; }
          return id;
        });
      },
      face: function (id) {
        if (isDir) return send('GET', base + '/' + enc(did) + '/copies/' + enc(id) + '/face').then(textOf);
        return send('POST', base + '/hand/' + enc(did) + '/copies/' + enc(id) + '/turn?size=open').then(textOf);
      },
      verb: function (id, verb, body) { return send('POST', base + '/' + enc(did) + '/copies/' + enc(id) + '/' + verb, body || {}).then(jsonOf); }
    };

    // ---- structure ----
    var root = el(
      '<div class="dko" data-dko' + (phone ? ' data-phone' : '') + (tall ? '' : ' data-noleaf') + '>' +
        '<div class="dko-main"><div class="dko-stagefan" data-dko-stagefan></div><div class="dko-group"><div class="dko-centre">' +
          '<div class="dko-slot" data-dko-slot><span class="dko-ground" aria-hidden="true"></span></div>' +
          '<div class="dko-leafcol" data-dko-leaf aria-live="off"></div>' +
        '</div></div></div>' +
        '<div class="dko-foot"><p class="dko-line" role="status" data-dko-line></p><div class="dko-acts" data-dko-acts></div></div>' +
        '<div class="dko-band" data-dko-band><div class="dko-bandfan" data-dko-bandfan>' +
          '<div class="dko-fanpane" data-dko-fanpane><div class="dko-fan" role="group" data-dko-fan><div class="dko-fanbox" data-dko-fanbox></div></div>' +
            '<p class="dko-hint" role="status" data-dko-hint></p></div></div>' +
          '<div class="dko-tray" data-dko-tray><p class="dko-tray-none" data-dko-trayline>Cards you set aside wait here.</p>' +
            '<div class="dko-tray-body" data-dko-traybody hidden><p class="dko-tray-lab" data-dko-traylab></p><div class="dko-tray-row" role="list" aria-label="Cards set aside" data-dko-row></div></div></div>' +
        '</div>' +
      '</div>');
    var bottom = null, scrollEl = null, band = null;
    if (phone) {
      // One scroll region (card, leaf, drawn row); the fan, then the line and actions, pinned below it.
      var ce = $('.dko-centre', root), lc = $('.dko-leafcol', root), tr = $('.dko-tray', root), fp = $('.dko-fanpane', root), ft = $('.dko-foot', root);
      scrollEl = el('<div class="dko-scroll" data-dko-scroll></div>');
      bottom = el('<div class="dko-bottom"></div>');
      ce.removeChild(lc);
      scrollEl.appendChild(el('<div class="dko-stagefan" data-dko-stagefan></div>'));
      scrollEl.appendChild(ce); scrollEl.appendChild(lc); scrollEl.appendChild(tr);
      bottom.appendChild(fp); bottom.appendChild(ft);
      root.textContent = '';
      root.appendChild(scrollEl); root.appendChild(bottom);
    } else {
      band = $('[data-dko-band]', root);
    }
    ctx.body.appendChild(root);
    root.style.setProperty('--dko-bw', BW + 'px'); root.style.setProperty('--dko-bh', BH + 'px');
    root.style.setProperty('--dko-R', RAD + 'px'); root.style.setProperty('--dko-fanh', FANH + 'px'); root.style.setProperty('--dko-fullh', FULLH + 'px');
    var slot = $('[data-dko-slot]', root), leafBox = $('[data-dko-leaf]', root);
    var line = $('[data-dko-line]', root), acts = $('[data-dko-acts]', root), row = $('[data-dko-row]', root), lab = $('[data-dko-traylab]', root);
    var trayLine = $('[data-dko-trayline]', root), trayBody = $('[data-dko-traybody]', root);
    var stageHost = $('[data-dko-stagefan]', root), bandHost = $('[data-dko-bandfan]', root), fanpane = $('[data-dko-fanpane]', root);
    var ground = $('.dko-ground', root);
    var fan = $('[data-dko-fan]', root), fanbox = $('[data-dko-fanbox]', root), hint = $('[data-dko-hint]', root);

    // Before the first pull the fan is the stage, centred; a pull moves it down into the lower band.
    var stageMode = !o.startCopy;
    function applyMode() {
      if (stageMode) { stageHost.appendChild(fanpane); root.setAttribute('data-stage', ''); if (band) band.setAttribute('data-nofan', ''); }
      else {
        if (phone) bottom.insertBefore(fanpane, bottom.firstChild); else bandHost.appendChild(fanpane);
        root.removeAttribute('data-stage'); if (band) band.removeAttribute('data-nofan');
      }
    }
    applyMode();
    /** The fan's one move between the stage and the band (Settle, 280ms); Calm fades, Off jumps. */
    function moveFan(toStage) {
      if (stageMode === toStage) return Promise.resolve();
      var r0 = fanbox.getBoundingClientRect();
      stageMode = toStage; applyMode();
      var lv = level();
      if (lv === 'off') return Promise.resolve();
      if (lv === 'calm') return fade(fan, 0, 1, 200);
      var r1 = fanbox.getBoundingClientRect(), S = phone ? 1 : r0.height / Math.max(1, r1.height);
      fan.style.transformOrigin = '50% 0';
      return anim(fan, [{ transform: 'translate(' + (r0.left + r0.width / 2 - r1.left - r1.width / 2) + 'px,' + (r0.top - r1.top) + 'px) scale(' + S + ')' }, { transform: 'none' }], 280, OUT()).then(settle);
    }
    fan.setAttribute('aria-label', 'The shuffled ' + view.name + ' deck, face down');

    function setLine(t, tone) { line.textContent = t || ''; if (tone) line.setAttribute('data-tone', tone); else line.removeAttribute('data-tone'); }

    // ---- the fan ----
    var backs = [], activeIdx = 0, fanMsg = null;
    function targetN() { return view.pile > 0 ? Math.min(NMAX, view.pile) : 0; }
    function angleOf(i, n) { return (i - (n - 1) / 2) * STEP; }
    function fanOff() { return !view.canDraw || view.pile <= 0; }
    function backNode() { return el(view.backHtml); }
    function makeBack() {
      var b = el('<button type="button" class="dko-back" data-dko-back tabindex="-1"></button>');
      b.appendChild(backNode());
      var bo = { el: b, a: 0 };
      b.addEventListener('click', function () { pull(bo); });
      b.addEventListener('focus', function () { var i = backs.indexOf(bo); if (i >= 0) { activeIdx = i; rove(); } });
      return bo;
    }
    function put(bo, a, i) { bo.a = a; bo.el.style.setProperty('--a', a + 'deg'); bo.el.style.zIndex = String(i + 1); }
    function rove() { backs.forEach(function (bo, i) { bo.el.tabIndex = i === activeIdx ? 0 : -1; }); }
    function labelBacks() {
      var n = backs.length;
      backs.forEach(function (bo, i) { bo.el.setAttribute('aria-label', 'Face-down card ' + (i + 1) + ' of ' + n + ', ' + view.name); });
      if (activeIdx >= n) activeIdx = Math.max(0, n - 1);
      var off = fanOff();
      backs.forEach(function (bo) { bo.el.disabled = off; });
      if (off) fan.setAttribute('data-off', ''); else fan.removeAttribute('data-off');
      if (!stageMode && !off && (cur || items.length)) fan.setAttribute('data-dim', ''); else fan.removeAttribute('data-dim');
      rove();
    }
    /** Every back to its angle for n backs; those already out Settle, new ones deal out. */
    function layout(opts) {
      opts = opts || {};
      var n = backs.length, lv = level(), ps = [];
      backs.forEach(function (bo, i) {
        var a = angleOf(i, n), old = bo.a, fresh = bo.fresh;
        put(bo, a, i);
        if (fresh) {
          bo.fresh = false;
          var d = opts.stagger ? i * 60 : (opts.delay || 0);
          if (lv === 'full') ps.push(anim(bo.el, [{ transform: 'rotate(0deg)', opacity: 0 }, { opacity: 1, offset: 0.25 }, { transform: 'rotate(' + a + 'deg)', opacity: 1 }], 360, OUT(), { delay: d }).then(settle));
          else if (lv === 'calm') ps.push(anim(bo.el, [{ opacity: 0 }, { opacity: 1 }], 200, OUT(), { delay: opts.stagger ? i * 40 : (opts.delay || 0) }).then(settle));
        } else if (old !== a && lv === 'full') {
          ps.push(anim(bo.el, [{ transform: 'rotate(' + old + 'deg)' }, { transform: 'rotate(' + a + 'deg)' }], 200, OUT()).then(settle));
        }
      });
      labelBacks();
      return Promise.all(ps);
    }
    function addBack(at, hidden) {
      var bo = makeBack(); bo.fresh = true;
      if (hidden) bo.el.style.visibility = 'hidden';
      backs.splice(at == null ? backs.length : at, 0, bo);
      fanbox.appendChild(bo.el);
      return bo;
    }
    function removeBack(bo, fadeIt) {
      var i = backs.indexOf(bo); if (i < 0) return Promise.resolve();
      backs.splice(i, 1);
      if (fadeIt && level() !== 'off') return fade(bo.el, 1, 0, 150, IN()).then(function () { bo.el.remove(); });
      bo.el.remove(); return Promise.resolve();
    }
    /** Bring the number of backs to what the pile holds, up to the fan's room. */
    function syncFan(opts) {
      var n = targetN(), ps = [];
      while (backs.length > n) ps.push(removeBack(backs[backs.length - 1], true));
      while (backs.length < n) addBack(null, false);
      ps.push(layout(opts));
      renderHint();
      return Promise.all(ps);
    }
    function hintBase() {
      var shown = targetN(), s = 'Shuffled: every back is the same draw. Pull one.';
      if (view.pile > shown) s += ' ' + shown + ' of ' + view.pile + ' shown.';
      // The count is said once: "Draw another · 2 left" carries it whenever that word shows.
      if (!isDir && view.drawsLeft != null && !$('[data-dko-another]', acts)) s += ' ' + (view.drawsLeft === 1 ? '1 draw left' : view.drawsLeft + ' draws left') + '.';
      return s;
    }
    function refusalText() { return view.pile <= 0 ? 'Nothing left in this deck.' : cap(view.refusal); }
    function renderHint() {
      if (fanMsg) { hint.textContent = fanMsg.t; if (fanMsg.quiet) hint.removeAttribute('data-tone'); else hint.setAttribute('data-tone', 'refuse'); return; }
      var r = fanOff() ? refusalText() : '';
      if (r) { hint.textContent = r; hint.setAttribute('data-tone', 'refuse'); }
      else { hint.textContent = hintBase(); hint.removeAttribute('data-tone'); }
    }
    function say(t, quiet) { fanMsg = t ? { t: t, quiet: !!quiet } : null; renderHint(); }
    function fanKeys(e) {
      var i = -1;
      backs.forEach(function (bo, k) { if (bo.el === document.activeElement) i = k; });
      if (i < 0) return;
      var j = i;
      if (e.key === 'ArrowRight' || e.key === 'ArrowDown') j = Math.min(backs.length - 1, i + 1);
      else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') j = Math.max(0, i - 1);
      else if (e.key === 'Home') j = 0; else if (e.key === 'End') j = backs.length - 1; else return;
      e.preventDefault(); backs[j].el.focus({ preventScroll: true });
    }
    fanbox.addEventListener('keydown', fanKeys);
    function focusFan() {
      var bo = backs[Math.min(activeIdx, backs.length - 1)];
      if (bo && !bo.el.disabled) bo.el.focus({ preventScroll: true });
    }

    // ---- the drawn row: one quiet line until a card is set aside, then it opens with the Fold ----
    var trayOpen = false, trayInit = false;
    function trayItems() { return items.filter(function (i) { return i !== cur; }); }
    function trayLabel(it) { return it.turned ? (it.label || 'A card from ' + view.name) : 'A face-down card from ' + view.name; }
    function trayCardEl(it) {
      var b = el('<button type="button" class="dko-tcard" role="listitem"></button>');
      b.setAttribute('data-copy', it.id);
      b.setAttribute('aria-label', trayLabel(it) + '. Press to bring it back to the stage.');
      var c = it.turned && it.up ? resize(it.up, 'hand') : backNode();
      if (c) b.appendChild(c);
      var cp = el('<span class="dko-tcap" aria-hidden="true"></span>');
      cp.textContent = it.turned ? trayLabel(it).split(',')[0] : 'Face down';
      b.appendChild(cp);
      b.addEventListener('click', function () { bringBack(it, b); });
      return b;
    }
    function trayCardOf(id) {
      return $$('.dko-tcard', row).filter(function (b) { return b.getAttribute('data-copy') === id; })[0] || null;
    }
    function renderTrayCount() {
      var n = items.length;
      lab.textContent = (isDir ? 'Drawn' : 'Drawn now') + ' ';
      var s = document.createElement('span');
      s.textContent = isDir ? n + ' of ' + MAX_SHOWN : String(n);
      lab.appendChild(s);
    }
    function setTray(open) {
      if (open === trayOpen && trayInit) return Promise.resolve();
      var first = !trayInit; trayOpen = open; trayInit = true;
      if (band) { if (open) band.setAttribute('data-tray', ''); else band.removeAttribute('data-tray'); }
      if (open) root.setAttribute('data-hastray', ''); else root.removeAttribute('data-hastray');
      if (first) { trayBody.hidden = !open; trayLine.hidden = open; return Promise.resolve(); }
      if (open) { trayLine.hidden = true; return fold(trayBody, true); }
      return fold(trayBody, false).then(function () { trayLine.hidden = false; });
    }
    function renderTray(skipId) {
      var t = trayItems();
      row.textContent = '';
      t.forEach(function (it) { var b = trayCardEl(it); if (skipId === it.id) b.style.visibility = 'hidden'; row.appendChild(b); });
      renderTrayCount();
      return setTray(t.length > 0);
    }

    // ---- sizes on a phone: the largest open size that fits the scroll region ----
    function fit() {
      if (!phone || !scrollEl) return;
      var availH = scrollEl.clientHeight - 28 - (tall ? 120 : 0), availW = scrollEl.clientWidth - 32;
      var w = Math.max(132, Math.min(availW, availH * (tall ? 7 / 12 : 5 / 7), 230));
      root.style.setProperty('--dko-w', Math.floor(w) + 'px');
    }
    function more() {
      if (!scrollEl) return;
      var left = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight;
      if (left > 6) scrollEl.setAttribute('data-more', ''); else scrollEl.removeAttribute('data-more');
    }
    var ro = window.ResizeObserver ? new ResizeObserver(function () { fit(); more(); }) : null;
    if (ro && phone) { ro.observe(scrollEl); Array.prototype.forEach.call(scrollEl.children, function (c) { ro.observe(c); }); }
    if (scrollEl) scrollEl.addEventListener('scroll', more, { passive: true });

    // ---- the stage card ----
    function makeDown(it) {
      var shape = window.DeckCards ? window.DeckCards.shapeOf(view.shape) : view.shape;
      var tn = window.DeckCards ? window.DeckCards.turnOf(view.turn) : view.turn;
      var w = el('<div class="dk-wrap dk-sz-open dk-s-' + esc(shape) + ' dk-t-' + esc(tn) + '" data-dk-card data-turn="' + esc(tn) + '" data-shape="' + esc(shape) + '" data-state="down">' +
        '<span class="dk-turner">' + view.backHtml.replace(/dk-sz-hand/g, 'dk-sz-open') + '<span class="dk-faceslot"></span></span>' +
        '<button type="button" class="dk-turn" data-dko-cardturn></button>' +
        '<span class="dk-sr dk-live" role="status" aria-live="polite"></span></div>');
      var tb = $('.dk-turn', w);
      tb.setAttribute('aria-label', 'Turn over the card you pulled from ' + view.name);
      tb.addEventListener('click', function (e) { e.preventDefault(); turnIt(); });
      w.setAttribute('data-copy', it.id);
      return w;
    }
    // The stage shows the leaf in its own column, so the card keeps the plate only.
    function makeUp(it) {
      var w = it.up ? resize(it.up, 'open') : null;
      if (!w) return makeDown(it);
      w.setAttribute('data-copy', it.id);
      return w;
    }
    function showLeaf(it, animate) {
      leafBox.textContent = '';
      if (!tall || !it.leaf) return;
      var leaf = el(it.leaf);
      leafBox.appendChild(leaf);
      if (animate) peek(leaf);
    }
    function clearLeaf() { leafBox.textContent = ''; }
    function fadeLeaf() {
      var l = leafBox.firstElementChild; if (!l) return Promise.resolve();
      return fade(l, 1, 0, level() === 'full' ? 120 : 150, IN()).then(function () { l.remove(); });
    }
    function slotCard(w) { w.classList.add('dko-card'); slot.insertBefore(w, ground); slot.setAttribute('data-has', ''); }
    function emptySlot() { slot.removeAttribute('data-has'); }
    function topOfScroll() { if (scrollEl) scrollEl.scrollTop = 0; }

    // ---- the actions under the card ----
    function btn(text, cls, handler, attrs) {
      var b = el('<button type="button" class="' + cls + '"></button>');
      b.textContent = text;
      if (attrs) Object.keys(attrs).forEach(function (a) { b.setAttribute(a, attrs[a]); });
      b.addEventListener('click', handler);
      return b;
    }
    function anotherLabel() {
      if (isDir || view.drawsLeft == null) return 'Draw another';
      return view.drawsLeft > 0 ? 'Draw another · ' + view.drawsLeft + ' left' : 'Draw another';
    }
    function anotherBtn() {
      var b = btn(anotherLabel(), 'dkd-word', function () { drawAnother(); }, { 'data-dko-another': '' });
      if (fanOff()) b.disabled = true;
      return b;
    }
    function renderActions(phase, keepFocus) {
      var hadFocus = acts.contains(document.activeElement);
      acts.textContent = '';
      var list = [];
      if (phase === 'down') {
        list.push(btn('Turn it over', 'dkd-word', function () { turnIt(); }, { 'data-dko-turn': '' }));
      } else if (phase === 'up') {
        if (isDir) {
          list.push(btn('Show the table', 'btn-primary dkd-btn', function () { leave('show'); }, { 'data-dko-primary': '' }));
          list.push(btn('Deal to…', 'dkd-btn dkd-btn-quiet', function (e) { openDeal(e.currentTarget); }, { 'aria-haspopup': 'menu', 'aria-expanded': 'false', 'data-dko-deal': '' }));
          list.push(btn('Put it back', 'dkd-btn dkd-btn-quiet', function () { putBack(); }, { 'data-dko-putback': '' }));
        } else {
          list.push(btn('Play it', 'btn-primary dkd-btn', function () { leave('play'); }, { 'data-dko-primary': '' }));
          list.push(btn('Show the table', 'dkd-btn dkd-btn-quiet', function () { leave('show'); }, { 'data-dko-show': '' }));
          list.push(btn('Keep it', 'dkd-btn dkd-btn-quiet', function () { ctx.close(); }, { 'data-dko-keep': '' }));
        }
        list.push(anotherBtn());
      } else if (phase === 'none' && drewOnce && !stageMode) {
        list.push(anotherBtn());
      }
      list.forEach(function (b) { acts.appendChild(b); });
      if (phase === 'up') list.forEach(peek);
      renderHint();
      if (keepFocus && hadFocus) { var f = $('[data-dko-primary]', acts) || list[0] || (backs[0] && backs[0].el); if (f) f.focus({ preventScroll: true }); }
    }
    function stateLine() {
      if (!cur) { setLine(''); return; }
      if (!cur.turned) { setLine('Face down. Turn it over to read it.'); return; }
      setLine(isDir ? 'Drawn. Only Directors can see it.' : 'In your hand.');
    }

    // ---- sequences: one at a time ----
    function run(fn) {
      if (busy || dead) return idleP;
      busy = true;
      var p = Promise.resolve().then(fn).catch(function () { /* each move says its own failure */ }).then(function () {
        busy = false;
        if (again && !dead) { again = false; refreshView(); }
      });
      idleP = p;
      return p;
    }

    /** Pull a back: it lifts at once and rings; when the draw answers it rises to the stage face down and the fan settles. */
    function pull(bo) {
      if (busy || dead || bo.el.disabled) return idleP;
      return run(function () {
        var from = bo.el.getBoundingClientRect();
        bo.el.setAttribute('data-working', '');
        setLine('');
        say('Pulling…', true);
        return api.draw().then(function (id) {
          var it = { id: id, turned: false };
          var stageP = moveFan(false);
          var old = cur, oldEl = cardEl, oldFrom = oldEl ? oldEl.getBoundingClientRect() : null;
          var ps = [];
          view.pile = Math.max(0, view.pile - 1);
          if (!isDir && view.drawsLeft != null) view.drawsLeft = Math.max(0, view.drawsLeft - 1);
          // The card already on the stage goes to the drawn row as a real compact card.
          if (old) {
            var tb = trayCardEl(old);
            if (!trayOpen) row.textContent = '';
            tb.style.visibility = 'hidden'; row.appendChild(tb);
            items.push(it); cur = it;
            renderTrayCount();
            ps.push(setTray(true));
            row.scrollLeft = row.scrollWidth;
            tb.style.visibility = '';
            ps.push(flyIn(tb, oldFrom, 420));
            oldEl.remove(); emptySlot(); fadeLeaf();
          } else { items.push(it); cur = it; }
          drewOnce = true;
          cardEl = makeDown(it); topOfScroll();
          slotCard(cardEl); fade(ground, 0, 1, 200);
          ps.push(stageP);
          ps.push(flyIn(cardEl, from, 420));
          backs.splice(backs.indexOf(bo), 1); bo.el.remove();
          fanMsg = null;
          var refill = targetN() > backs.length ? (addBack(null, false), true) : false;
          ps.push(layout({ delay: refill ? 120 : 0 }));
          renderHint();
          ctx.setSub(subLine());
          renderActions('down'); stateLine();
          again = true;
          return Promise.all(ps).then(function () {
            var b = $('[data-dko-cardturn]', cardEl); if (b) b.focus({ preventScroll: true });
            ctx.say('A card is face down. Turn it over to read it.');
          });
        }, function (err) {
          bo.el.removeAttribute('data-working');
          fanMsg = { t: err.offline || !err.words ? DRAW_FAILED : err.words };
          again = !err.offline;
          return syncFan().then(function () { renderHint(); if (err.offline) bo.el.focus({ preventScroll: true }); });
        });
      });
    }

    /** Turn the card over: the deck's own turn, and the face is asked for only now. */
    function turnIt() {
      if (!cur || cur.turned || busy || dead) return idleP;
      var it = cur, w = cardEl;
      return run(function () {
        setLine('Turning…');
        var bn = $('[data-dko-turn]', acts); if (bn) bn.disabled = true;
        var failed = null;
        function loadFace() {
          return api.face(it.id).then(function (html) {
            var t = document.createElement('template');
            t.innerHTML = html;
            var wrap = $('.dk-wrap', t.content), src = wrap || t.content;
            var lf = $('.dk-leaf', src);
            it.leaf = lf ? lf.outerHTML : '';
            if (lf) lf.parentNode.removeChild(lf);
            if (wrap) return wrap.innerHTML;
            var d = document.createElement('div');
            d.appendChild(t.content.cloneNode(true));
            return d.innerHTML;
          }, function (e) { failed = e; throw e; });
        }
        return window.DeckCards.turnOver(w, loadFace).then(function () {
          if (failed) {
            setLine(failed.words || OFFLINE, 'refuse');
            var b2 = $('[data-dko-turn]', acts); if (b2) b2.disabled = false;
            return;
          }
          it.turned = true; it.up = w.outerHTML; it.label = labelOf(w);
          topOfScroll();
          showLeaf(it, true);
          renderActions('up', false);
          stateLine();
          var f = $('[data-dko-primary]', acts); if (f) f.focus({ preventScroll: true });
          again = true;
        });
      });
    }

    /** bringBack: a drawn card returns to the stage; the one that was there takes its place in the row. */
    function bringBack(it, tb) {
      if (busy || dead || it === cur) return;
      run(function () {
        var oldCur = cur, oldEl = cardEl;
        var trayFrom = tb.getBoundingClientRect();
        var curFrom = oldEl ? oldEl.getBoundingClientRect() : null;
        var newTb = null;
        if (oldCur) { newTb = trayCardEl(oldCur); tb.replaceWith(newTb); newTb.style.visibility = 'hidden'; }
        else { tb.remove(); }
        cur = it;
        var stageP = moveFan(false);
        var nw = it.turned ? makeUp(it) : makeDown(it);
        fadeLeaf();
        if (oldEl) { oldEl.remove(); }
        slotCard(nw); cardEl = nw; topOfScroll();
        var ps = [flyIn(nw, trayFrom, 420), stageP];
        if (newTb) { newTb.style.visibility = ''; ps.push(flyIn(newTb, curFrom, 420)); }
        renderTrayCount();
        if (!newTb) ps.push(setTray(trayItems().length > 0));
        return Promise.all(ps).then(function () {
          if (it.turned) { showLeaf(it, true); renderActions('up'); }
          else { clearLeaf(); renderActions('down'); var b = $('[data-dko-cardturn]', nw); if (b) b.focus({ preventScroll: true }); }
          stateLine();
          labelBacks();
          if (!newTb) renderTray();
        });
      });
    }

    /**
     * drawAnother: the card on the stage is set aside into the drawn row and the
     * fan lifts back to the middle, ready to pull. It draws nothing itself.
     */
    function drawAnother() {
      if (busy || dead || fanOff()) return idleP;
      return run(function () {
        var old = cur, oldEl = cardEl, ps = [];
        if (old && oldEl) {
          var from = oldEl.getBoundingClientRect();
          cur = null; cardEl = null;
          var trayP = renderTray(old.id);
          var tb = trayCardOf(old.id);
          if (tb) { tb.style.visibility = ''; ps.push(flyIn(tb, from, 420)); }
          ps.push(trayP);
          oldEl.remove(); emptySlot(); fadeLeaf();
        }
        ps.push(moveFan(true));
        setLine('');
        renderActions('none');
        labelBacks();
        say('Pull one.', true);
        return Promise.all(ps).then(function () {
          labelBacks();
          ctx.say('Pull one.');
          focusFan();
        });
      });
    }

    function subLine() { return view.sub; }

    function dropCurrent(thenLine, actionP, how) {
      var it = cur, w = cardEl;
      return actionP.then(function () {
        items = items.filter(function (x) { return x !== it; });
        cur = null; cardEl = null;
        fadeLeaf();
        var lv = level();
        var leaveP = lv === 'off' ? Promise.resolve()
          : lv === 'calm' ? fade(w, 1, 0, 200, IN())
          : how === 'deal' ? deckMove(w, 'deal', Math.max(160, root.getBoundingClientRect().right - w.getBoundingClientRect().left + 20), 24)
          : deckMove(w, 'play', 0, -(w.getBoundingClientRect().height + 80));
        return leaveP;
      }).then(function () {
        w.remove(); emptySlot(); fanMsg = null;
        renderTray(); labelBacks(); renderHint(); ctx.setSub(subLine());
        setLine(thenLine);
        renderActions('none', true);
        ctx.say(thenLine);
        again = true;
      }, function (err) {
        setLine(err.words || OFFLINE, 'refuse');
        throw err;
      });
    }
    function leave(verb) {
      if (busy || dead || !cur) return idleP;
      return run(function () { return dropCurrent(verb === 'show' ? 'On the table.' : 'Played.', api.verb(cur.id, verb), 'play'); });
    }
    function dealTo(hero) {
      if (busy || dead || !cur) return;
      run(function () { return dropCurrent('Dealt to ' + hero.name + '.', api.verb(cur.id, 'deal', { to: 'hand', heroId: hero.id }), 'deal'); });
    }
    /** Put it back: face down at once, then it slides into the fan, which opens a gap for it. */
    function putBack() {
      if (busy || dead || !cur) return idleP;
      return run(function () {
        var it = cur, w = cardEl;
        return api.verb(it.id, 'put-back').then(function () {
          items = items.filter(function (x) { return x !== it; });
          cur = null; cardEl = null; fadeLeaf();
          view.pile += 1;
          var down = makeDown(it);
          var tb0 = $('.dk-turn', down); if (tb0) tb0.remove();
          w.replaceWith(down); slot.insertBefore(down, ground);
          var gap = null;
          if (targetN() > backs.length) gap = addBack(Math.floor(backs.length / 2), true);
          var settled = layout();
          var lv = level();
          var to = gap ? gap.el.getBoundingClientRect() : fanbox.getBoundingClientRect();
          var flight;
          if (lv === 'full') flight = flyOut(down, gap ? to : { left: to.left + to.width / 2 - BW / 2, top: to.top, width: BW, height: BH, right: 0, bottom: 0 }, 380);
          else if (lv === 'calm') flight = fade(down, 1, 0, 200, IN());
          else flight = Promise.resolve();
          return Promise.all([settled, flight]).then(function () {
            down.remove(); emptySlot();
            if (gap) { gap.el.style.visibility = ''; if (lv !== 'off') fade(gap.el, 0, 1, 120); }
            fanMsg = null; labelBacks(); renderHint(); ctx.setSub(subLine()); renderTray();
            setLine('Back in the deck.');
            renderActions('none', true);
            ctx.say('Back in the deck.');
            again = true;
          });
        }, function (err) { setLine(err.words || OFFLINE, 'refuse'); });
      });
    }

    // ---- Deal to…: a small list of the party's heroes ----
    var dealMenu = null;
    function closeDeal() {
      if (!dealMenu) return;
      var m = dealMenu; dealMenu = null;
      var b = $('[data-dko-deal]', acts); if (b) b.setAttribute('aria-expanded', 'false');
      if (level() === 'off') m.remove(); else anim(m, [{ opacity: 1 }, { opacity: 0 }], 80, IN()).then(function () { m.remove(); });
    }
    function openDeal(btnEl) {
      if (dealMenu) { closeDeal(); return; }
      var m = el('<div class="dkd-menu" role="menu" aria-label="Deal to"><p class="dkd-menu-t">Deal to</p></div>');
      if (!heroes.length) { var none = el('<p class="dkd-menu-t"></p>'); none.textContent = 'No heroes in the party yet.'; m.appendChild(none); }
      heroes.forEach(function (h) {
        var b = el('<button type="button" role="menuitem" class="dkd-mi"></button>'); b.textContent = h.name;
        b.setAttribute('data-hero', h.id);
        b.addEventListener('click', function () { closeDeal(); dealTo(h); });
        m.appendChild(b);
      });
      acts.appendChild(m);
      var r = btnEl.getBoundingClientRect(), ar = acts.getBoundingClientRect(), pr = ctx.panel.getBoundingClientRect();
      var mh = m.offsetHeight, below = !phone && (pr.bottom - r.bottom - 12) >= mh;
      m.style.left = Math.max(0, r.left - ar.left) + 'px';
      if (below) { m.style.bottom = 'auto'; m.style.top = (r.bottom - ar.top + 6) + 'px'; }
      else { m.style.top = 'auto'; m.style.bottom = (ar.bottom - r.top + 6) + 'px'; }
      btnEl.setAttribute('aria-expanded', 'true');
      dealMenu = m; grow(m);
      var first = $('.dkd-mi', m); if (first) first.focus({ preventScroll: true });
      m.addEventListener('keydown', function (e) {
        var its = $$('.dkd-mi', m), i = its.indexOf(document.activeElement);
        if (!its.length) return;
        if (e.key === 'ArrowDown') { e.preventDefault(); its[(i + 1) % its.length].focus(); }
        else if (e.key === 'ArrowUp') { e.preventDefault(); its[(i - 1 + its.length) % its.length].focus(); }
        else if (e.key === 'Tab') { closeDeal(); }
      });
    }
    function onDocClick(e) { if (dealMenu && !dealMenu.contains(e.target) && !e.target.closest('[data-dko-deal]')) closeDeal(); }
    document.addEventListener('click', onDocClick, true);
    closers.push(ctx.onEscape(function () {
      if (!dealMenu) return false;
      closeDeal();
      var b = $('[data-dko-deal]', acts); if (b) b.focus();
      return true;
    }));

    // ---- reading the server again ----
    function itemOf(s) {
      var up = resize(s.html, 'open');
      return { id: s.copyId, turned: true, up: up ? up.outerHTML : '', leaf: leafOf(s.html), label: up ? labelOf(up) : '' };
    }
    /** applyView: the server's numbers and words, and (Director) cards shown elsewhere or taken away. */
    function applyView(v) {
      view = Object.assign({}, view, v);
      if (isDir && v.shown) {
        var ids = v.shown.map(function (s) { return s.copyId; });
        items.filter(function (it) { return ids.indexOf(it.id) < 0; }).forEach(function (it) {
          items = items.filter(function (x) { return x !== it; });
          if (it === cur) {
            var w = cardEl; cur = null; cardEl = null;
            fadeLeaf(); emptySlot();
            if (w) fade(w, 1, 0, 150, IN()).then(function () { w.remove(); });
            setLine('That card was taken away.');
            ctx.say('That card was taken away.');
          }
        });
        v.shown.forEach(function (s) {
          if (items.some(function (it) { return it.id === s.copyId; })) return;
          items.push(itemOf(s));
          drewOnce = true;
        });
      }
      ctx.setSub(subLine());
      renderTray();
      if (cur) renderActions(cur.turned ? 'up' : 'down');
      else if (!stageMode) renderActions('none');
      return syncFan();
    }
    function refreshView() {
      if (dead) return Promise.resolve();
      if (busy) { again = true; return idleP; }
      return api.view().then(function (v) {
        if (dead) return undefined;
        if (busy) { again = true; return undefined; }
        return applyView(v);
      }, function (e) {
        if (e && e.status === 404 && !dead) setLine(GONE, 'refuse');
      });
    }
    function onLive() { clearTimeout(timer); timer = setTimeout(refreshView, 150); }
    window.addEventListener(LIVE_EVENT, onLive);

    // ---- start ----
    if (isDir) (view.shown || []).forEach(function (s) { items.push(itemOf(s)); });
    if (items.length) drewOnce = true;
    ctx.setSub(subLine());
    if (o.startCopy) {
      var start = items.filter(function (it) { return it.id === o.startCopy; })[0];
      if (start) {
        cur = start;
        cardEl = makeUp(start); slotCard(cardEl); showLeaf(start, false);
        renderActions('up'); stateLine();
      } else { stageMode = true; applyMode(); renderActions('none'); }
    } else {
      renderActions('none');
    }
    renderTray();
    fit(); more();
    renderHint();
    // The fan deals out once the panel has grown (60ms apart, at most seven), then waits for a pull.
    idleP = ctx.ready.then(function () { return wait(level() === 'off' ? 0 : ms(200)); }).then(function () {
      if (dead) return undefined;
      busy = true;
      var n = targetN();
      for (var i = 0; i < n; i++) addBack(null, false);
      return layout({ stagger: true }).then(function () {
        busy = false;
        renderHint();
        if (!cur) { var f = backs[Math.floor(backs.length / 2)]; if (f) { activeIdx = backs.indexOf(f); rove(); if (!f.el.disabled) f.el.focus({ preventScroll: true }); } }
        else { var p = $('[data-dko-primary]', acts); if (p) p.focus({ preventScroll: true }); }
        if (again) { again = false; refreshView(); }
      });
    });

    return {
      whenIdle: function () { return idleP; },
      destroy: function () {
        dead = true; clearTimeout(timer);
        document.removeEventListener('click', onDocClick, true);
        window.removeEventListener(LIVE_EVENT, onLive);
        closers.forEach(function (f) { f(); });
        if (ro) ro.disconnect();
      }
    };
  }

  /**
   * open: read the deck's draw-view, then show the overlay. The returned
   * promise rejects with the server's words when the deck cannot be opened.
   */
  function open(o) {
    if (current) return Promise.resolve(current);
    o = o || {};
    var from = o.from;
    current = {};
    if (from && C.working) C.working(from, true);
    return send('GET', baseOf(o.campaignId) + '/' + enc(o.deckId) + '/draw-view').then(jsonOf).then(function (view) {
      if (from && C.working) C.working(from, false);
      current = shell({
        title: view.name, sub: view.sub, from: from, onClose: o.onClose,
        mount: function (c) { return mountDraw(c, o, view); }
      });
      return current;
    }, function (e) {
      if (from && C.working) C.working(from, false);
      current = null;
      throw e;
    });
  }

  // ---- The dealt-card pop-up ----

  /**
   * arrive: a card a Director dealt, face down and centred, on whatever campaign
   * page the player has open. It is marked seen as it opens, so a reload or a
   * second device does not show it again. The face is asked for only when the
   * player turns it. Resolves false when another overlay is already open.
   */
  function arrive(o) {
    var card = o && o.card;
    if (!card || current) return Promise.resolve(false);
    var base = baseOf(o.campaignId);
    send('POST', base + '/hand/fresh/' + enc(card.copyId) + '/seen').catch(function () { /* told again next time */ });
    current = {};
    var ctx = shell({
      arrive: true, title: card.deckName, sub: card.sub || '',
      mount: function (c) {
        var tall = card.shape === 'tall';
        var root = el('<div class="dko dko-arrive" data-dko-arrive>' +
          '<div class="dko-slot" data-dko-slot><span class="dko-ground" aria-hidden="true"></span></div>' +
          '<div class="dko-leafcol" data-dko-leaf aria-live="off"></div>' +
          '<p class="dko-line" role="status" data-dko-line>Face down. Turn it over to read it.</p>' +
          '<div class="dko-acts" data-dko-acts></div></div>');
        if (!tall) root.setAttribute('data-noleaf', '');
        c.body.appendChild(root);
        var slot = $('[data-dko-slot]', root), leafBox = $('[data-dko-leaf]', root), line = $('[data-dko-line]', root), acts = $('[data-dko-acts]', root);
        var shape = window.DeckCards ? window.DeckCards.shapeOf(card.shape) : card.shape;
        var tn = window.DeckCards ? window.DeckCards.turnOf(card.turn) : card.turn;
        var w = el('<div class="dk-wrap dko-card dk-sz-open dk-s-' + esc(shape) + ' dk-t-' + esc(tn) + '" data-dk-card data-turn="' + esc(tn) + '" data-shape="' + esc(shape) + '" data-state="down">' +
          '<span class="dk-turner">' + String(card.backHtml).replace(/dk-sz-hand/g, 'dk-sz-open') + '<span class="dk-faceslot"></span></span>' +
          '<button type="button" class="dk-turn" data-dko-cardturn></button>' +
          '<span class="dk-sr dk-live" role="status" aria-live="polite"></span></div>');
        w.setAttribute('data-copy', card.copyId);
        $('.dk-turn', w).setAttribute('aria-label', 'Turn over the card dealt to you from ' + card.deckName);
        slot.insertBefore(w, $('.dko-ground', slot));
        slot.setAttribute('data-has', '');
        var busy = false, turned = false;
        function later() { return btn('Later', 'dkd-word', function () { ctx.close(); }, { 'data-dko-later': '' }); }
        function btn(text, cls, fn, attrs) {
          var b = el('<button type="button" class="' + cls + '"></button>');
          b.textContent = text;
          Object.keys(attrs || {}).forEach(function (k) { b.setAttribute(k, attrs[k]); });
          b.addEventListener('click', fn);
          return b;
        }
        function turnIt() {
          if (busy || turned) return;
          busy = true;
          line.textContent = 'Turning…';
          var failed = null;
          var go = $('[data-dko-turn]', acts); if (go) go.disabled = true;
          function loadFace() {
            return send('POST', base + '/hand/' + enc(card.deckId) + '/copies/' + enc(card.copyId) + '/turn?size=open').then(textOf).then(function (html) {
              var t = document.createElement('template');
              t.innerHTML = html;
              var wrap = $('.dk-wrap', t.content), src = wrap || t.content;
              var lf = $('.dk-leaf', src);
              var leaf = lf ? lf.outerHTML : '';
              if (lf) lf.parentNode.removeChild(lf);
              if (tall && leaf) { var l = el(leaf); leafBox.appendChild(l); l.style.visibility = 'hidden'; w.__leaf = l; }
              if (wrap) return wrap.innerHTML;
              var d = document.createElement('div');
              d.appendChild(t.content.cloneNode(true));
              return d.innerHTML;
            }, function (e) { failed = e; throw e; });
          }
          window.DeckCards.turnOver(w, loadFace).then(function () {
            busy = false;
            if (failed) {
              line.textContent = failed.words || OFFLINE; line.setAttribute('data-tone', 'refuse');
              if (go) go.disabled = false;
              var tb = $('.dk-turn', w); if (tb) tb.disabled = false;
              return;
            }
            turned = true;
            line.textContent = 'In your hand.'; line.removeAttribute('data-tone');
            if (w.__leaf) { w.__leaf.style.visibility = ''; peek(w.__leaf); }
            acts.textContent = '';
            var close = btn('Close', 'btn-primary dkd-btn', function () { ctx.close(); }, { 'data-dko-later': '' });
            acts.appendChild(close);
            close.focus({ preventScroll: true });
            c.say('In your hand.');
          });
        }
        $('.dk-turn', w).addEventListener('click', function (e) { e.preventDefault(); turnIt(); });
        acts.appendChild(btn('Turn it over', 'btn-primary dkd-btn', turnIt, { 'data-dko-turn': '' }));
        acts.appendChild(later());
        c.ready.then(function () { var b = $('.dk-turn', w); if (b && !turned) b.focus({ preventScroll: true }); c.say('A card was dealt to you. Face down.'); });
        return { destroy: function () { /* nothing outside the panel */ } };
      },
      onClose: function () { current = null; if (o.onClose) o.onClose(); }
    });
    current = ctx;
    return Promise.resolve(ctx);
  }

  // ---- The DM Screen section: Card decks ----

  /**
   * mountScreen: wire one server-drawn section. The server owns the markup;
   * after each move the section is read again and put back with the same rows
   * open, so counts and drawn cards are always the server's.
   */
  function mountScreen(root) {
    if (root.__dkd) return;
    root.__dkd = true;
    var cid = root.getAttribute('data-campaign');
    var base = baseOf(cid);
    var errs = {};          // deck id -> the line said under that row
    var menu = null, syncing = false, queued = null, timer = 0;

    function deckId(node) { return node.getAttribute('data-dkd-deck'); }
    function nodeOf(t) { return t.closest ? t.closest('.dkd-deck') : null; }
    function deckBy(id) { return $$('.dkd-deck', root).filter(function (n) { return deckId(n) === id; })[0] || null; }
    function rowOf(node) { return $('[data-dkd-row]', node); }
    function isOpen(node) { var r = rowOf(node); return !!r && r.getAttribute('aria-expanded') === 'true'; }

    function setErr(id, text) {
      if (text) errs[id] = text; else delete errs[id];
      var n = deckBy(id), p = n && $('[data-dkd-err]', n);
      if (!p) return;
      p.textContent = text || '';
      p.hidden = !text;
    }
    function paintErrs() { Object.keys(errs).forEach(function (id) { setErr(id, errs[id]); }); }

    /** setOpen: a row's Fold; what it holds (the buttons, the drawn cards, its line) comes and goes together. */
    function setOpen(node, open) {
      var r = rowOf(node);
      if (!r) return Promise.resolve();
      r.setAttribute('aria-expanded', open ? 'true' : 'false');
      var ps = [];
      ['[data-dkd-acts]', '[data-dkd-shown]'].forEach(function (sel) {
        var box = $(sel, node);
        if (box && box.hidden === open) ps.push(fold(box, open));
      });
      return Promise.all(ps);
    }
    function toggleRow(node) {
      closeMenu();
      var open = !isOpen(node);
      var ps = [];
      if (open) $$('.dkd-deck', root).forEach(function (n) { if (n !== node && isOpen(n)) ps.push(setOpen(n, false)); });
      ps.push(setOpen(node, open));
      return Promise.all(ps);
    }
    function foldAll() {
      $$('.dkd-deck', root).forEach(function (n) {
        var open = isOpen(n);
        var a = $('[data-dkd-acts]', n), s = $('[data-dkd-shown]', n);
        if (a) a.hidden = !open;
        if (s) s.hidden = !open;
      });
      var rest = $('[data-dkd-rest]', root), more = $('[data-dkd-more]', root);
      if (rest && more) rest.hidden = more.getAttribute('aria-expanded') !== 'true';
      var pk = $('[data-dkd-picker]', root), ch = $('[data-dkd-choose]', root);
      if (pk) { pk.hidden = !(ch && ch.getAttribute('aria-expanded') === 'true'); }
      paintErrs();
    }

    // ---- reading the section again ----
    function snapshot() {
      var a = document.activeElement, key = null;
      if (a && root.contains(a)) {
        var n = nodeOf(a);
        if (n) {
          var kind = ['data-dkd-row', 'data-dkd-draw', 'data-dkd-quick'].filter(function (k) { return a.hasAttribute(k); })[0];
          key = { deck: deckId(n), kind: kind || '', copy: a.getAttribute('data-dkd-copy') || '' };
        } else {
          key = { top: ['data-dkd-more', 'data-dkd-choose'].filter(function (k) { return a.hasAttribute(k); })[0] || '' };
        }
      }
      return {
        open: $$('.dkd-deck', root).filter(isOpen).map(deckId),
        more: ($('[data-dkd-more]', root) || document.createElement('i')).getAttribute('aria-expanded') === 'true',
        picker: ($('[data-dkd-choose]', root) || document.createElement('i')).getAttribute('aria-expanded') === 'true',
        copies: $$('[data-dkd-copy]', root).map(function (b) { return b.getAttribute('data-dkd-copy'); }),
        focus: key
      };
    }
    function restore(st, animateDeck) {
      $$('.dkd-deck', root).forEach(function (n) {
        var open = st.open.indexOf(deckId(n)) >= 0, r = rowOf(n);
        if (r) r.setAttribute('aria-expanded', open ? 'true' : 'false');
      });
      var more = $('[data-dkd-more]', root); if (more) more.setAttribute('aria-expanded', st.more ? 'true' : 'false');
      var ch = $('[data-dkd-choose]', root); if (ch) ch.setAttribute('aria-expanded', st.picker ? 'true' : 'false');
      foldAll();
      var f = st.focus, t = null;
      if (f && f.top) t = $('[' + f.top + ']', root);
      else if (f) {
        var n = deckBy(f.deck);
        if (n && f.copy) t = $$('[data-dkd-copy]', n).filter(function (b) { return b.getAttribute('data-dkd-copy') === f.copy; })[0];
        if (n && !t && f.kind) t = $('[' + f.kind + ']', n);
      }
      if (t && t.focus) t.focus({ preventScroll: true });
      if (animateDeck) {
        var n2 = deckBy(animateDeck), thumb = n2 && $('.dkd-back', n2);
        if (n2 && thumb) $$('[data-dkd-copy]', n2).forEach(function (b) {
          if (st.copies.indexOf(b.getAttribute('data-dkd-copy')) < 0) flyIn(b, thumb.getBoundingClientRect(), 420);
        });
      }
    }
    /** resync: one read at a time; a request that comes while one is out runs after it. */
    function resync(animateDeck) {
      if (!root.isConnected) return Promise.resolve();
      if (syncing) { queued = animateDeck || queued || ''; return Promise.resolve(); }
      syncing = true;
      return send('GET', base + '/screen').then(textOf).then(function (html) {
        var t = document.createElement('template');
        t.innerHTML = html;
        var fresh = $('[data-dkd-screen]', t.content);
        if (!fresh) return;
        var st = snapshot();
        closeMenu(true);
        root.innerHTML = fresh.innerHTML;
        restore(st, animateDeck);
      }, function () { /* the section keeps what it shows */ }).then(function () {
        syncing = false;
        if (queued !== null) { var q = queued; queued = null; return resync(q); }
        return undefined;
      });
    }
    // The DM Screen redraws its sections, so a section that has left the page
    // lets go of its page-wide listeners the next time one fires.
    function gone() {
      if (root.isConnected) return false;
      window.removeEventListener(LIVE_EVENT, onLive);
      document.removeEventListener('click', onDocClick, true);
      clearTimeout(timer);
      return true;
    }
    function onLive() {
      if (gone()) return;
      clearTimeout(timer);
      timer = setTimeout(function () { if (!menu) resync(); }, 300);
    }
    window.addEventListener(LIVE_EVENT, onLive);

    // ---- Quick draw ----
    function quick(node, b) {
      var id = deckId(node);
      if (b.hasAttribute('data-working')) return;
      setErr(id, '');
      var label = b.textContent;
      b.setAttribute('data-working', ''); b.textContent = 'Drawing…';
      var draw = $('[data-dkd-draw]', node); if (draw) draw.disabled = true;
      send('POST', base + '/' + enc(id) + '/draw', { count: 1 }).then(function () {
        return resync(id);
      }, function (e) {
        setErr(id, e.offline ? DRAW_FAILED : e.words);
        return resync();
      }).then(function () {
        var nb = deckBy(id) && $('[data-dkd-quick]', deckBy(id));
        if (nb && nb !== b) return;
        b.removeAttribute('data-working'); b.textContent = label;
        if (draw) draw.disabled = false;
      });
    }

    // ---- A drawn card's menu ----
    function closeMenu(now) {
      if (!menu) return;
      var m = menu; menu = null;
      if (m.btn) m.btn.setAttribute('aria-expanded', 'false');
      if (now || level() === 'off') m.node.remove();
      else anim(m.node, [{ opacity: 1 }, { opacity: 0 }], 80, IN()).then(function () { m.node.remove(); });
    }
    function openMenu(node, btn) {
      closeMenu(true);
      var id = deckId(node), copy = btn.getAttribute('data-dkd-copy');
      var m = el('<div class="dkd-menu" role="menu"></div>');
      function fill(heroList) {
        m.textContent = '';
        if (heroList) {
          var t = el('<p class="dkd-menu-t"></p>'); t.textContent = 'Deal to'; m.appendChild(t);
          var hs = heroesIn();
          if (!hs.length) { var none = el('<p class="dkd-menu-t"></p>'); none.textContent = 'No heroes in the party yet.'; m.appendChild(none); }
          hs.forEach(function (h) {
            var b = el('<button type="button" role="menuitem" class="dkd-mi"></button>');
            b.textContent = h.name; b.setAttribute('data-hero', h.id);
            m.appendChild(b);
          });
          var back = el('<button type="button" role="menuitem" class="dkd-mi dkd-mi-quiet" data-act="menu"></button>'); back.textContent = 'Back'; m.appendChild(back);
        } else {
          [['show', 'Show the table'], ['deal', 'Deal to…'], ['put-back', 'Put it back'], ['open', 'Open']].forEach(function (it) {
            var b = el('<button type="button" role="menuitem" class="dkd-mi"></button>');
            b.textContent = it[1]; b.setAttribute('data-act', it[0]);
            m.appendChild(b);
          });
        }
        var first = $('.dkd-mi', m); if (first) first.focus({ preventScroll: true });
      }
      fill(false);
      node.appendChild(m);
      var br = btn.getBoundingClientRect(), nr = node.getBoundingClientRect();
      m.style.left = Math.max(0, Math.min(br.left - nr.left, nr.width - m.offsetWidth - 4)) + 'px';
      m.style.top = (br.bottom - nr.top + 4) + 'px';
      btn.setAttribute('aria-expanded', 'true');
      menu = { node: m, btn: btn };
      grow(m);
      m.addEventListener('keydown', function (e) {
        var its = $$('.dkd-mi', m), i = its.indexOf(document.activeElement);
        if (!its.length) return;
        if (e.key === 'ArrowDown') { e.preventDefault(); its[(i + 1) % its.length].focus(); }
        else if (e.key === 'ArrowUp') { e.preventDefault(); its[(i - 1 + its.length) % its.length].focus(); }
        else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); closeMenu(); btn.focus(); }
        else if (e.key === 'Tab') { closeMenu(); }
      });
      m.addEventListener('click', function (e) {
        var b = e.target.closest('.dkd-mi'); if (!b) return;
        var act = b.getAttribute('data-act');
        if (act === 'deal') { fill(true); return; }
        if (act === 'menu') { fill(false); return; }
        var hero = b.getAttribute('data-hero');
        closeMenu(true);
        menuAct(node, id, copy, btn, hero ? 'deal' : act, hero ? { id: hero, name: b.textContent } : null);
      });
    }
    function leaveCard(btn, how) {
      var lv = level();
      if (lv === 'off') return Promise.resolve();
      if (lv === 'calm') return fade(btn, 1, 0, 200, IN());
      var node = nodeOf(btn), thumb = node && $('.dkd-back', node);
      if (how === 'put-back' && thumb) return flyOut(btn, thumb.getBoundingClientRect(), 360);
      var w = $('.dk-wrap', btn);
      if (how === 'deal') return deckMove(w || btn, 'deal', 120, 0);
      return deckMove(w || btn, 'play', 0, -90);
    }
    function menuAct(node, id, copy, btn, act, hero) {
      if (act === 'open') {
        open({ campaignId: cid, deckId: id, from: rowOf(node), viewer: 'director', startCopy: copy, onClose: function () { resync(); } })
          .catch(function (e) { setErr(id, e.words); });
        return;
      }
      setErr(id, '');
      btn.disabled = true;
      var body = act === 'deal' ? { to: 'hand', heroId: hero.id } : {};
      send('POST', base + '/' + enc(id) + '/copies/' + enc(copy) + '/' + act, body).then(function () {
        return leaveCard(btn, act);
      }, function (e) {
        btn.disabled = false;
        setErr(id, e.offline ? OFFLINE : e.words);
        throw e;
      }).then(function () { return resync(); }, function () { /* said under the row */ });
    }

    // ---- Choose decks ----
    function savePicks(b) {
      var picker = $('[data-dkd-picker]', root);
      var ids = $$('input[type="checkbox"]', picker).filter(function (i) { return i.checked; }).map(function (i) { return i.value; });
      var say = $('[data-dkd-picks-err]', picker);
      if (!say) { say = el('<p class="dkd-line" role="alert" data-dkd-picks-err hidden></p>'); picker.appendChild(say); }
      say.hidden = true; say.textContent = '';
      b.setAttribute('data-working', ''); b.disabled = true;
      send('PUT', base + '/screen/picks', { deckIds: ids }).then(function () {
        var ch = $('[data-dkd-choose]', root);
        if (ch) ch.setAttribute('aria-expanded', 'false');
        return resync().then(function () { var c2 = $('[data-dkd-choose]', root); if (c2) c2.focus({ preventScroll: true }); });
      }, function (e) {
        b.removeAttribute('data-working'); b.disabled = false;
        say.textContent = e.offline ? OFFLINE : e.words; say.hidden = false;
      });
    }

    root.addEventListener('click', function (e) {
      var t = e.target;
      if (!t.closest) return;
      var b, node = nodeOf(t);
      if ((b = t.closest('[data-dkd-row]')) && node) { toggleRow(node); return; }
      if ((b = t.closest('[data-dkd-quick]')) && node) { quick(node, b); return; }
      if ((b = t.closest('[data-dkd-draw]')) && node) {
        closeMenu(true);
        setErr(deckId(node), '');
        open({ campaignId: cid, deckId: deckId(node), from: rowOf(node), viewer: 'director', onClose: function () { resync(); } })
          .catch(function (err) { setErr(deckId(node), err.offline ? OFFLINE : err.words); });
        return;
      }
      if ((b = t.closest('[data-dkd-copy]')) && node) {
        if (menu && menu.btn === b) closeMenu(); else openMenu(node, b);
        return;
      }
      if ((b = t.closest('[data-dkd-more]'))) {
        var openIt = b.getAttribute('aria-expanded') !== 'true';
        b.setAttribute('aria-expanded', openIt ? 'true' : 'false');
        var rest = $('[data-dkd-rest]', root); if (rest) fold(rest, openIt);
        return;
      }
      if ((b = t.closest('[data-dkd-choose]'))) {
        var pick = b.getAttribute('aria-expanded') !== 'true';
        b.setAttribute('aria-expanded', pick ? 'true' : 'false');
        var pk = $('[data-dkd-picker]', root); if (pk) fold(pk, pick);
        return;
      }
      if ((b = t.closest('[data-dkd-picks-save]'))) { savePicks(b); return; }
    });
    function onDocClick(e) {
      if (gone()) return;
      if (menu && !menu.node.contains(e.target) && !e.target.closest('[data-dkd-copy]')) closeMenu();
    }
    document.addEventListener('click', onDocClick, true);
    root.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && menu) { var b = menu.btn; closeMenu(); b.focus(); }
    });
    // The rows and the choose list start folded; the Fold is the move that opens them.
    $$('[data-dkd-row]', root).forEach(function (r) { r.setAttribute('aria-expanded', 'false'); });
    var mre = $('[data-dkd-more]', root); if (mre) mre.setAttribute('aria-expanded', 'false');
    var che = $('[data-dkd-choose]', root); if (che) che.setAttribute('aria-expanded', 'false');
    foldAll();
  }

  /** scan: mount every Card decks section on the page, now and whenever one is added. */
  function scan(rootNode) {
    $$('[data-dkd-screen]', rootNode || document).forEach(mountScreen);
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', function () { scan(); });
  else scan();
  document.addEventListener('htmx:load', function (e) { scan(e.detail && e.detail.elt && e.detail.elt.querySelectorAll ? e.detail.elt : document); });
  if (window.MutationObserver) {
    var pending = false;
    new MutationObserver(function () {
      if (pending) return;
      pending = true;
      (window.requestAnimationFrame || setTimeout)(function () { pending = false; scan(); });
    }).observe(document.documentElement, { childList: true, subtree: true });
  }

  window.DeckDraw = {
    open: open, arrive: arrive, scan: scan,
    close: function () { return current && current.panel ? current.close() : Promise.resolve(); },
    isOpen: function () { return !!(current && current.panel); },
    cap: cap, resize: resize
  };
})();
