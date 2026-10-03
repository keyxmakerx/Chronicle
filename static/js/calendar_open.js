/**
 * calendar_open.js -- the Calendars page's cards, as the signed card-opening
 * design has them. Pointing at a card, or focusing it, slides its month out
 * from underneath (the peek itself is CSS, calendar_v5.css); on a touch
 * screen the first tap peeks and the second opens.
 *
 * Opening a card turns the page into the calendar's own page, in place: the
 * page is fetched from the card's link, mounted where the list was, and the
 * list fades back while motes of light fly from the card (or its peek) to the
 * calendar's pieces, each piece appearing as its first mote lands. The URL is
 * the calendar's own, so a reload or a shared link is just its page. "Back to
 * calendars", the browser's Back button, or Escape returns to the list.
 *
 * All of it is an enhancement: the card is a plain link, so without this
 * script, or when the fetch fails, it is ordinary navigation, and a direct
 * visit to a calendar never runs any of it.
 *
 * Loaded on every page (internal/app/routes.go's pluginBodyScripts) and idle
 * off the Calendars page. The pure helpers first are exported for
 * test/js/calendar_open.test.mjs.
 */
(function () {
  'use strict';

  // ---------------------------------------------------------------------
  // Timing, from the signed mockup. Durations in ms; FLY, FADE, SPREAD and
  // JITTER are fractions of DUR, the whole opening.
  // ---------------------------------------------------------------------
  var T = {
    DUR: 1000,       // the motes' run, start to last landing
    FLY: 0.45,       // one mote's flight
    FADE: 0.16,      // a landed mote's fade
    SPREAD: 0.36,    // the last piece leaves this much later than the first
    JITTER: 0.07,    // a piece's other motes leave up to this much after its first
    LIST_FADE: 240,  // the list stepping back
    PEEK_FADE: 200,  // the peek dissolving into the first motes
    POP: 240,        // a piece settling as its first mote lands
    GLASS: 950,      // the calendar's card gaining its paper
    LINES: 1000,     // the grid's hairlines, drawn as the days settle
    TIDY: 1300,      // finished animations dropped, the page left as plain markup
    CLOSE_OUT: 180,  // the calendar stepping back on the way home
    CLOSE_IN: 240,   // the list returning
    CLOSE_DELAY: 120,
    PLAIN: 160,      // reduced motion: one crossfade, no motes
    PREFETCH_DWELL: 120,
    PREFETCH_TTL: 15000
  };
  var MOTE_BUDGET = 360; // most motes in flight at once, however many days a month has
  var SPRITE = 48;       // the pre-drawn glow's size in px

  // A piece's motes: about one per 3000 px², at least 2, at most 12.
  function moteCount(w, h) {
    return Math.max(2, Math.min(12, Math.round((w * h) / 3000)));
  }

  // When the i-th of n pieces (in reading order) sends its first mote, as a
  // fraction of DUR.
  function pieceDelay(i, n) {
    return n > 0 ? (i / n) * T.SPREAD : 0;
  }

  // Reading order for pieces by their rects: rows top to bottom, a piece
  // joining the row above when its top is within tol px of that row's
  // first top, then left to right. Grouping by row (rather than comparing
  // tops pairwise) keeps the order stable for pieces a pixel or two apart.
  function readingOrder(rects, tol) {
    tol = tol == null ? 8 : tol;
    var byTop = rects.map(function (r, i) { return i; }).sort(function (a, b) {
      return rects[a].top - rects[b].top || a - b;
    });
    var rows = [], row = null, rowTop = 0;
    byTop.forEach(function (i) {
      if (!row || rects[i].top - rowTop > tol) { row = []; rowTop = rects[i].top; rows.push(row); }
      row.push(i);
    });
    var out = [];
    rows.forEach(function (r) {
      r.sort(function (a, b) { return rects[a].left - rects[b].left || a - b; });
      out.push.apply(out, r);
    });
    return out;
  }

  function easeOut(u) { return 1 - Math.pow(1 - u, 3); }

  // Every mote for one opening: from a random point in src to a random
  // point well inside its piece, on a curve bent to one side. rects must
  // already be in reading order; rand is injectable so tests are exact.
  function planMotes(src, rects, rand) {
    rand = rand || Math.random;
    var n = rects.length, counts = rects.map(function (r) { return moteCount(r.width, r.height); });
    var total = counts.reduce(function (a, b) { return a + b; }, 0);
    if (total > MOTE_BUDGET) counts = counts.map(function (c) { return Math.max(1, Math.floor((c * MOTE_BUDGET) / total)); });
    var P = [];
    rects.forEach(function (r, ri) {
      var base = pieceDelay(ri, n);
      for (var k = 0; k < counts[ri]; k++) {
        var sx = src.left + rand() * src.width, sy = src.top + rand() * src.height;
        var tx = r.left + r.width * (0.15 + rand() * 0.7), ty = r.top + r.height * (0.2 + rand() * 0.6);
        var dx = tx - sx, dy = ty - sy, len = Math.hypot(dx, dy) || 1;
        var bend = (rand() < 0.5 ? -1 : 1) * Math.min(160, len * 0.35) * (0.5 + rand() * 0.5);
        P.push({
          sx: sx, sy: sy, tx: tx, ty: ty,
          cx: (sx + tx) / 2 - (dy / len) * bend, cy: (sy + ty) / 2 + (dx / len) * bend,
          d: base + (k === 0 ? 0 : rand() * T.JITTER), c: (ri + k) % 2, r: 1.1 + rand() * 1.6, ri: ri
        });
      }
    });
    return P;
  }

  // Where one mote is at time t (a fraction of DUR): on its curve while it
  // flies, then swelling and fading where it landed. alive says whether it
  // still needs a frame; landed whether its piece may appear.
  function moteAt(p, t) {
    if (t < p.d) return { alive: true, landed: false, alpha: 0 };
    var u = Math.min(1, (t - p.d) / T.FLY), e = easeOut(u), a = 1 - e;
    var fade = u < 1 ? Math.min(1, u * 6) : Math.max(0, 1 - (t - p.d - T.FLY) / T.FADE);
    return {
      x: a * a * p.sx + 2 * a * e * p.cx + e * e * p.tx,
      y: a * a * p.sy + 2 * a * e * p.cy + e * e * p.ty,
      alpha: 0.95 * fade,
      radius: p.r * (u < 1 ? 1 : 1 + (1 - fade) * 2.5) * 4.5,
      landed: u >= 1,
      alive: u < 1 || fade > 0
    };
  }

  // A tap on a card that isn't peeking yet peeks; anything else opens.
  function tapAction(pointerType, peeking) {
    return pointerType === 'touch' && !peeking ? 'peek' : 'open';
  }

  // The calendar pages' own theme rule (calendar-view.css, calendar_v5.css).
  function isDark(themeAttr, prefersDark) {
    return themeAttr === 'dark' || (themeAttr !== 'light' && !!prefersDark);
  }

  // Light adds up to white on a dark page; on paper that washes out, so
  // there the motes draw normally.
  function blendFor(dark) {
    return dark ? 'lighter' : 'source-over';
  }

  // A click the browser would otherwise follow as a plain navigation.
  function isPlainClick(e) {
    return !e.defaultPrevented && (e.button === 0 || e.button == null) && !e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey;
  }

  // Escape returns to the list only from a freshly opened calendar with
  // nothing of its own open: a day, era, moon or event card, the event
  // drawer or weather sheet, edit mode, or a field being typed in all take
  // Escape first.
  function escapeReturns(o) {
    return !!o.open && !o.defaultPrevented && !o.panelOpen && !o.editing && !o.drawerOpen && !o.typing;
  }

  var api = {
    T: T, MOTE_BUDGET: MOTE_BUDGET, moteCount: moteCount, pieceDelay: pieceDelay, readingOrder: readingOrder,
    planMotes: planMotes, moteAt: moteAt, tapAction: tapAction, isDark: isDark, blendFor: blendFor,
    isPlainClick: isPlainClick, escapeReturns: escapeReturns
  };
  if (typeof window !== 'undefined') {
    window.Chronicle = window.Chronicle || {};
    window.Chronicle.calendarOpen = api;
  }
  // Test-only hook, as in calendar_rule.js: undefined in the browser.
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof document === 'undefined') return;

  // =====================================================================
  // The page.
  // =====================================================================

  // The calendar's pieces, each assembled by its own motes. Anything else
  // on the page (an editor's bar, say) is simply there when it lands.
  var PIECES = [
    '[data-cal-back]', '.cal5-crumbs nav',
    '.cal-v5 .skywrap:not([hidden])', '.cal-v5 .head .hub', '.cal-v5 .head .todaypill', '.cal-v5 .head .h-acts',
    '.cal-v5 .head .mtitle', '.cal-v5 .head .mnav', '.cal-v5 .head .h-subw', '.cal-v5 .dow',
    '.cal-v5 .stage .day[data-key]', '.cal-v5 .stage .band', '.cal-v5 .caption'
  ].join(',');

  // phase: idle (the list, or any other page), open (a calendar opened from
  // its card, list kept aside for the way back), closing.
  var S = { phase: 'idle', seq: 0, list: null, anims: [], timers: [], raf: 0, canvas: null, lifts: [] };
  var pre = null;           // the one prefetched page: {url, at, promise}
  var lastPointer = 'mouse';
  var touchPeek = null;     // the slot a tap is peeking
  var dwell = 0;            // the hover-prefetch timer

  function mainEl() { return document.getElementById('main-content'); }
  function reduced() { return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches); }
  function capable() { return !!(window.fetch && window.DOMParser && window.history && history.pushState && Element.prototype.animate); }
  function sameDoc(url) {
    try { return new URL(url, location.href).origin === location.origin; } catch (e) { return false; }
  }
  function pathOf(url) {
    try { return new URL(url, location.href).pathname; } catch (e) { return ''; }
  }

  function anim(el, frames, o) {
    var a = el.animate(frames, Object.assign({ fill: 'both' }, o));
    S.anims.push(a);
    return a;
  }
  function later(fn, ms) { S.timers.push(setTimeout(fn, ms)); }

  // Ends any running motion: no animations, timers, frame loop or canvas,
  // nothing left half-faded, no lifted copy left on the page.
  function stop() {
    S.anims.forEach(function (a) { try { a.cancel(); } catch (e) { /* already gone */ } });
    S.anims = [];
    S.timers.forEach(clearTimeout);
    S.timers = [];
    if (S.raf) { cancelAnimationFrame(S.raf); S.raf = 0; }
    if (S.canvas) { S.canvas.remove(); S.canvas = null; }
    S.lifts.forEach(function (l) { l.remove(); });
    S.lifts = [];
    if (S.veil) { S.veil.remove(); S.veil = null; }
  }

  // Lifts whatever #main-content shows into a fixed copy at the same place on
  // screen (the same nodes, moved), so the page can change underneath while
  // it fades. Returns the lift; the nodes stay in it after it is removed.
  // Stylesheet links stay where they are: a link taken out of the page drops
  // its sheet and reloads it when put back, and the page would flash
  // unstyled in between.
  function liftOut(main) {
    var r = main.getBoundingClientRect(), cs = getComputedStyle(main);
    var lift = document.createElement('div');
    lift.className = 'calv5-lift';
    lift.setAttribute('aria-hidden', 'true');
    lift.inert = true;
    // Inline styles: the list's own stylesheet leaves with the list.
    lift.style.cssText = 'position:fixed;z-index:30;overflow:hidden;pointer-events:none;box-sizing:border-box;margin:0;' +
      'left:' + r.left + 'px;top:' + r.top + 'px;width:' + r.width + 'px;height:' + r.height + 'px;padding:' + cs.padding;
    var inner = document.createElement('div');
    inner.style.transform = 'translateY(' + -main.scrollTop + 'px)';
    var nodes = Array.prototype.filter.call(main.childNodes, function (n) { return !isSheet(n); });
    nodes.forEach(function (n) { inner.appendChild(n); });
    lift.appendChild(inner);
    document.body.appendChild(lift);
    S.lifts.push(lift);
    lift.nodes = nodes;
    return lift;
  }
  function isSheet(n) {
    return n.nodeType === 1 && n.matches('link[rel~="stylesheet"]');
  }
  function dropLift(lift) {
    lift.remove();
    S.lifts = S.lifts.filter(function (l) { return l !== lift; });
  }

  // ---- Fetching the calendar's page ----

  function load(url) {
    var now = Date.now();
    if (pre && pre.url === url && now - pre.at < T.PREFETCH_TTL) return pre.promise;
    var p = fetch(url, { credentials: 'same-origin', headers: { Accept: 'text/html' } }).then(function (res) {
      // A redirect (to the login page, say) is somewhere else: let the
      // browser go there itself.
      if (!res.ok || pathOf(res.url) !== pathOf(url)) throw new Error('not the calendar');
      return res.text();
    });
    var entry = { url: url, at: now, promise: p };
    pre = entry;
    p.catch(function () { if (pre === entry) pre = null; });
    return p;
  }
  function prefetch(link) {
    if (!capable() || S.phase !== 'idle') return;
    var url = link.href;
    if (sameDoc(url)) load(url).catch(function () { /* the click will navigate */ });
  }

  // The fetched page's #main-content, or null when it isn't a calendar.
  function pageFrom(html) {
    var doc = new DOMParser().parseFromString(html, 'text/html');
    var main = doc.getElementById('main-content');
    if (!main || !main.querySelector('[data-widget="calendar_view"]')) return null;
    // Never run a script that came in a fetched page (boot.js's swap rule).
    Array.prototype.forEach.call(main.querySelectorAll('script'), function (s) { s.remove(); });
    return { main: main, title: doc.title };
  }

  // ---- Opening ----

  function open(link, fromHistory) {
    var main = mainEl(), url = link.href;
    if (!main || S.phase !== 'idle' || !main.contains(link)) return false;
    var seq = ++S.seq;
    S.phase = 'loading';
    load(url).then(function (html) {
      if (seq !== S.seq || !link.isConnected) return;
      var page = pageFrom(html);
      if (!page) throw new Error('not the calendar');
      mount(link, page, url, fromHistory);
    }).catch(function () {
      if (seq !== S.seq) return;
      S.phase = 'idle';
      window.location.assign(url);
    });
    return true;
  }

  function sourceRect(link) {
    var slot = link.closest('.calv5-slot'), peek = slot && slot.querySelector('.calv5-peek');
    if (peek && parseFloat(getComputedStyle(peek).opacity) > 0.5) return { el: peek, rect: peek.getBoundingClientRect() };
    var card = link.closest('.calv5-ccard') || link;
    return { el: null, rect: card.getBoundingClientRect() };
  }

  function glowColours(from) {
    var cs = getComputedStyle(from);
    return [cs.getPropertyValue('--glow').trim() || '#f0c060', cs.getPropertyValue('--glow2').trim() || '#a58ae0'];
  }

  function mount(link, page, url, fromHistory) {
    var main = mainEl(), root = link.closest('.calv5') || main;
    var src = sourceRect(link), colours = glowColours(root);
    var href = link.getAttribute('href');
    // The just-created announcement has been made; coming back must not make it again.
    root.removeAttribute('data-created-name');
    root.removeAttribute('data-created-id');
    clearTouchPeek();

    var listState = { scroll: main.scrollTop, title: document.title, url: location.href, href: href };
    var lift = liftOut(main);
    listState.nodes = lift.nodes;

    // A sheet already on the page (the list loads the calendar's own) is not
    // added again, so going back and forth never piles up links.
    var have = {};
    Array.prototype.forEach.call(main.querySelectorAll('link[rel~="stylesheet"]'), function (l) { have[l.href] = true; });
    Array.prototype.slice.call(page.main.childNodes).forEach(function (n) {
      if (!(isSheet(n) && have[n.href])) main.appendChild(n);
    });
    main.scrollTop = 0;
    if (page.title) document.title = page.title;
    if (!fromHistory) {
      try {
        history.replaceState({ calv5List: true }, '', listState.url);
        history.pushState({ calv5Open: true }, '', url);
      } catch (e) { /* the opening still works; Back just leaves the page */ }
    }
    if (window.htmx && htmx.process) htmx.process(main);
    if (window.Chronicle && Chronicle.mountWidgets) Chronicle.mountWidgets(main);
    var back = main.querySelector('[data-cal-back]');
    if (back) back.hidden = false;

    S.list = listState;
    S.phase = 'open';
    var focusTo = back || main.querySelector('.cal-v5 .mtitle');
    if (focusTo) {
      if (!back) focusTo.setAttribute('tabindex', '-1');
      focusTo.focus({ preventScroll: true });
    }

    if (reduced() || fromHistory) {
      anim(lift, [{ opacity: 1 }, { opacity: 0 }], { duration: T.PLAIN, fill: 'forwards' });
      anim(main, [{ opacity: 0 }, { opacity: 1 }], { duration: T.PLAIN });
      later(function () { stop(); }, T.PLAIN + 20);
      return;
    }
    assemble(main, lift, src, colours);
  }

  // A stable selector for a piece. The calendar repaints its month when its
  // first fetches return, replacing the day cells, so a piece is hidden and
  // shown by selector (a day by its date key), never by element.
  function pieceSelector(el) {
    if (el.matches('.cal-v5 .stage .day[data-key]')) {
      return '.cal-v5 .stage .day[data-key="' + CSS.escape(el.getAttribute('data-key')) + '"]';
    }
    var sels = PIECES.split(',');
    for (var i = 0; i < sels.length; i++) if (el.matches(sels[i])) return sels[i];
    return null;
  }

  // The list steps back; motes leave the card (or peek) and build the
  // calendar piece by piece.
  function assemble(main, lift, src, colours) {
    var vh = window.innerHeight, seen = {};
    var found = Array.prototype.filter.call(main.querySelectorAll(PIECES), function (el) {
      var r = el.getBoundingClientRect(), sel = pieceSelector(el);
      if (!sel || seen[sel] || !(r.width > 0 && r.height > 0 && r.top < vh && r.bottom > 0)) return false;
      seen[sel] = true;
      return true;
    });
    // Pieces nested in another piece ride with it.
    found = found.filter(function (el) {
      return !found.some(function (o) { return o !== el && o.contains(el); });
    });
    var rects = found.map(function (el) { return el.getBoundingClientRect(); });
    var order = readingOrder(rects);
    var sels = order.map(function (i) { return pieceSelector(found[i]); });
    rects = order.map(function (i) { return rects[i]; });

    // Every piece is hidden by one rule until its first mote lands.
    var waiting = sels.slice();
    var veil = document.createElement('style');
    veil.setAttribute('data-cal-veil', '');
    var paint = function () { veil.textContent = waiting.length ? waiting.join(',') + '{opacity:0!important}' : ''; };
    paint();
    document.head.appendChild(veil);
    S.veil = veil;

    anim(lift, [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.985)' }], { duration: T.LIST_FADE, easing: 'ease-in', fill: 'forwards' });
    if (src.el) anim(src.el, [{ opacity: 1 }, { opacity: 0, transform: 'scale(1.04)' }], { duration: T.PEEK_FADE, fill: 'forwards' });
    later(function () { dropLift(lift); }, T.LIST_FADE);

    // The calendar's card is glass first, gaining its paper as the days settle.
    var cal = main.querySelector('.cal-v5 .cal');
    if (cal) {
      var glass = { backgroundColor: 'transparent', borderColor: 'transparent', boxShadow: 'none' };
      anim(cal, [glass, Object.assign({ offset: 0.4, easing: 'ease-out' }, glass), {}], { duration: T.GLASS });
    }
    // The grid's hairlines come last, so the motes are the first thing seen.
    // Like the pieces they are veiled by rule, since the weeks are repainted.
    var lines = '.cal-v5 .stage .wk{border-top-color:transparent!important}.cal-v5 .stage .day:not([data-key]){border-left-color:transparent!important}';
    var drawn = false;
    paint = function () {
      veil.textContent = (waiting.length ? waiting.join(',') + '{opacity:0!important}' : '') + (drawn ? '' : lines);
    };
    paint();
    later(function () {
      drawn = true;
      paint();
      var ease = { duration: T.LINES * 0.45, easing: 'ease-out' };
      Array.prototype.forEach.call(main.querySelectorAll('.cal-v5 .stage .wk'), function (w) { anim(w, [{ borderTopColor: 'transparent' }, {}], ease); });
      Array.prototype.forEach.call(main.querySelectorAll('.cal-v5 .stage .day:not([data-key])'), function (d) { anim(d, [{ borderLeftColor: 'transparent' }, {}], ease); });
    }, T.LINES * 0.55);

    fly(src.rect, rects, colours, function (i) {
      waiting = waiting.filter(function (w) { return w !== sels[i]; });
      paint();
      var el = main.querySelector(sels[i]);
      if (el) anim(el, [{ opacity: 0, transform: 'scale(.94)' }, { opacity: 1, transform: 'scale(1.02)', offset: 0.6 }, { opacity: 1, transform: 'none' }], { duration: T.POP, easing: 'ease-out', fill: 'backwards' });
    });
    // Once everything has landed, the finished animations and the emptied
    // veil go, leaving the page as plain, static markup.
    later(function () {
      S.anims.forEach(function (a) { a.cancel(); });
      S.anims = [];
      if (S.veil) { S.veil.remove(); S.veil = null; }
    }, T.TIDY);
  }

  // A mote is a pre-drawn glow (bright core, soft halo) stamped with
  // drawImage: far cheaper per frame than canvas shadows or blur.
  var sprites = {};
  function sprite(col) {
    if (sprites[col]) return sprites[col];
    var c = document.createElement('canvas'), h = SPRITE / 2;
    c.width = c.height = SPRITE;
    var g = c.getContext('2d');
    // Gradient stops need a concrete colour, so a white glow is tinted with the token instead.
    var t = g.createRadialGradient(h, h, 0, h, h, h);
    t.addColorStop(0, 'rgba(255,255,255,1)'); t.addColorStop(0.2, 'rgba(255,255,255,.9)');
    t.addColorStop(0.5, 'rgba(255,255,255,.18)'); t.addColorStop(1, 'rgba(255,255,255,0)');
    g.fillStyle = t; g.fillRect(0, 0, SPRITE, SPRITE);
    g.globalCompositeOperation = 'source-atop'; g.fillStyle = col; g.globalAlpha = 0.85; g.fillRect(0, 0, SPRITE, SPRITE);
    g.globalCompositeOperation = 'source-over'; g.globalAlpha = 1;
    var core = g.createRadialGradient(h, h, 0, h, h, 5);
    core.addColorStop(0, 'rgba(255,255,255,.95)'); core.addColorStop(1, 'rgba(255,255,255,0)');
    g.fillStyle = core; g.fillRect(0, 0, SPRITE, SPRITE);
    sprites[col] = c;
    return c;
  }

  // The motes' one canvas over the window, drawn only while a mote is still
  // flying or fading, then removed: no frame loop outlives the opening.
  function fly(src, rects, colours, onLand) {
    var W = window.innerWidth, H = window.innerHeight, dpr = window.devicePixelRatio || 1;
    var cv = document.createElement('canvas');
    cv.className = 'calv5-motes';
    cv.setAttribute('aria-hidden', 'true');
    cv.style.cssText = 'position:fixed;left:0;top:0;width:' + W + 'px;height:' + H + 'px;z-index:60;pointer-events:none';
    cv.width = Math.round(W * dpr); cv.height = Math.round(H * dpr);
    document.body.appendChild(cv);
    S.canvas = cv;
    var ctx = cv.getContext('2d');
    if (!ctx) { rects.forEach(function (r, i) { onLand(i); }); return; }
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    var html = document.documentElement;
    var dark = isDark(html.getAttribute('data-theme'), window.matchMedia && matchMedia('(prefers-color-scheme: dark)').matches);
    var blend = blendFor(dark), sp = [sprite(colours[0]), sprite(colours[1])];
    var P = planMotes(src, rects), landed = {}, t0 = performance.now();
    function frame(now) {
      var t = (now - t0) / T.DUR, alive = false;
      ctx.clearRect(0, 0, W, H);
      ctx.globalCompositeOperation = blend;
      for (var i = 0; i < P.length; i++) {
        var m = moteAt(P[i], t);
        if (m.alive) alive = true;
        if (m.landed && !landed[P[i].ri]) { landed[P[i].ri] = true; onLand(P[i].ri); }
        if (!(m.alpha > 0)) continue;
        ctx.globalAlpha = m.alpha;
        ctx.drawImage(sp[P[i].c], m.x - m.radius, m.y - m.radius, m.radius * 2, m.radius * 2);
      }
      ctx.globalAlpha = 1;
      if (alive) { S.raf = requestAnimationFrame(frame); return; }
      S.raf = 0;
      if (S.canvas === cv) { cv.remove(); S.canvas = null; }
    }
    S.raf = requestAnimationFrame(frame);
  }

  // ---- Back to the list ----

  function close() {
    var main = mainEl(), list = S.list;
    if (!main || !list) return;
    stop();
    S.phase = 'closing';
    Array.prototype.forEach.call(main.querySelectorAll('[data-widget]'), function (el) {
      if (window.Chronicle && Chronicle.destroyWidget) Chronicle.destroyWidget(el);
    });
    var lift = liftOut(main);
    list.nodes.forEach(function (n) { main.appendChild(n); });
    main.scrollTop = list.scroll;
    document.title = list.title;
    if (window.htmx && htmx.process) htmx.process(main);
    S.list = null;
    var card = null;
    Array.prototype.some.call(main.querySelectorAll('a[data-cal-open]'), function (a) {
      if (a.getAttribute('href') === list.href) { card = a; return true; }
      return false;
    });
    if (card) card.focus({ preventScroll: true });
    var done = function () { stop(); S.phase = 'idle'; };
    if (reduced()) {
      anim(lift, [{ opacity: 1 }, { opacity: 0 }], { duration: T.PLAIN, fill: 'forwards' });
      anim(main, [{ opacity: 0 }, { opacity: 1 }], { duration: T.PLAIN });
      later(done, T.PLAIN + 20);
      return;
    }
    anim(lift, [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.985)' }], { duration: T.CLOSE_OUT, easing: 'ease-in', fill: 'forwards' });
    anim(main, [{ opacity: 0, transform: 'scale(.985)' }, { opacity: 1, transform: 'none' }], { duration: T.CLOSE_IN, delay: T.CLOSE_DELAY, easing: 'ease-out', fill: 'backwards' });
    later(done, T.CLOSE_DELAY + T.CLOSE_IN + 20);
  }

  // Someone else took the page (a sidebar link, htmx restoring history): drop
  // the opening and the list kept for the way back.
  function abandon() {
    S.seq++;
    stop();
    S.list = null;
    S.phase = 'idle';
    clearTouchPeek();
  }

  // ---- Touch peeks ----

  function clearTouchPeek() {
    if (touchPeek) touchPeek.classList.remove('peeking');
    touchPeek = null;
  }
  function setTouchPeek(slot) {
    clearTouchPeek();
    if (!slot) return;
    slot.classList.add('peeking');
    touchPeek = slot;
  }

  // ---- Listeners: document-level and delegated, so they hold no page DOM
  // and survive the list being swapped in and out. ----

  function onPointerDown(e) {
    lastPointer = e.pointerType || 'mouse';
    if (touchPeek && !(e.target instanceof Element && touchPeek.contains(e.target))) clearTouchPeek();
  }
  function onKeyFlag(e) {
    if (e.key !== 'Escape') lastPointer = 'key';
  }

  function onClick(e) {
    if (!(e.target instanceof Element)) return;
    var back = e.target.closest('[data-cal-back]');
    if (back) {
      if (S.phase === 'open' && history.state && history.state.calv5Open && isPlainClick(e)) {
        e.preventDefault();
        history.back();
      }
      return;
    }
    var link = e.target.closest('a[data-cal-open]');
    if (!link || !isPlainClick(e) || !sameDoc(link.href)) return;
    var slot = link.closest('.calv5-slot');
    if (tapAction(lastPointer, !!(slot && slot.classList.contains('peeking'))) === 'peek') {
      e.preventDefault();
      setTouchPeek(slot);
      prefetch(link);
      return;
    }
    if (!capable()) return;
    if (open(link, false)) e.preventDefault();
  }

  function onPointerOver(e) {
    if (e.pointerType === 'touch' || !(e.target instanceof Element)) return;
    var slot = e.target.closest('.calv5-slot');
    if (!slot || (e.relatedTarget instanceof Element && slot.contains(e.relatedTarget))) return;
    clearTimeout(dwell);
    var link = slot.querySelector('a[data-cal-open]');
    if (link) dwell = setTimeout(function () { prefetch(link); }, T.PREFETCH_DWELL);
  }
  function onPointerOut(e) {
    if (!(e.target instanceof Element)) return;
    var slot = e.target.closest('.calv5-slot');
    if (slot && !(e.relatedTarget instanceof Element && slot.contains(e.relatedTarget))) clearTimeout(dwell);
  }
  function onFocusIn(e) {
    var link = e.target instanceof Element && e.target.closest('a[data-cal-open]');
    if (link && lastPointer !== 'touch') prefetch(link);
  }

  function onKeyDown(e) {
    if (e.key !== 'Escape') return;
    var main = mainEl(), mount = main && main.querySelector('[data-widget="calendar_view"]');
    var view = mount && mount.calendarView, ae = document.activeElement;
    var drawer = !!(view && ((Chronicle.calendarEventDrawer && Chronicle.calendarEventDrawer.isOpen(view)) ||
      (Chronicle.calendarWeatherSheet && Chronicle.calendarWeatherSheet.isOpen(view))));
    var ok = escapeReturns({
      open: S.phase === 'open' && !!(history.state && history.state.calv5Open),
      defaultPrevented: e.defaultPrevented,
      panelOpen: !!(view && view.anyPanelOpen && view.anyPanelOpen()),
      editing: !!(main && main.querySelector('.cal-v5 .cal.editing')),
      drawerOpen: drawer,
      typing: !!(ae && ae.matches && ae.matches('input, textarea, select, [contenteditable=""], [contenteditable="true"]'))
    });
    if (!ok) return;
    e.preventDefault();
    history.back();
  }

  function onPopState(e) {
    var st = e.state || {};
    if (st.calv5List) {
      if (S.list && S.phase === 'open') close();
      else if (S.phase === 'idle' && !document.querySelector('#main-content .calv5-cards, #main-content .calv5-empty')) {
        // The list kept for the way back is gone (another page came in
        // between): load it as a page.
        window.location.reload();
      }
      return;
    }
    if (st.calv5Open) {
      if (S.phase !== 'idle') return;
      var link = null, main = mainEl();
      if (main) Array.prototype.some.call(main.querySelectorAll('a[data-cal-open]'), function (a) {
        if (pathOf(a.href) === location.pathname) { link = a; return true; }
        return false;
      });
      if (!link || !capable() || !open(link, true)) window.location.reload();
      return;
    }
    if (S.phase !== 'idle') abandon();
  }

  function onBeforeSwap(e) {
    var t = e.detail && e.detail.target, main = mainEl();
    if (!t || !main) return;
    if (t === main || t.contains(main)) {
      clearTimeout(dwell);
      pre = null;
      if (S.phase !== 'idle') abandon();
      else clearTouchPeek();
    }
  }

  document.addEventListener('pointerdown', onPointerDown, true);
  document.addEventListener('keydown', onKeyFlag, true);
  document.addEventListener('click', onClick);
  document.addEventListener('pointerover', onPointerOver);
  document.addEventListener('pointerout', onPointerOut);
  document.addEventListener('focusin', onFocusIn);
  document.addEventListener('keydown', onKeyDown);
  document.addEventListener('htmx:beforeSwap', onBeforeSwap);
  window.addEventListener('popstate', onPopState);
})();
