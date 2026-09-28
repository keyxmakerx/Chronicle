/**
 * calendar_almanac.js -- a calendar opened from its card on the Calendars
 * page, as the signed setup design (#764) has it. The card is the almanac's
 * cover: it hinges open onto the preview (calendar_preview.templ), which
 * settles at its own size. "Open calendar" unfolds that page in place into
 * the calendar itself -- the same calendar_view widget the calendar's own
 * page mounts, waiting in the preview's <template> -- while the month grid
 * moves whole to where the calendar's grid lives and the calendar's header
 * unfolds from behind its top edge. The side page folds away behind the
 * month: the calendar carries today and its moons in its own header. Back
 * ("Preview") or Escape folds it up again; Escape once more closes it into
 * its card. Narrow screens get one sheet grown from the card; reduced motion
 * gets short cross-fades.
 *
 * Loaded on every page (internal/app/routes.go's pluginBodyScripts) and idle
 * until htmx swaps in a [data-almanac] preview.
 */
(function () {
  'use strict';

  var EASE = 'cubic-bezier(.22,.8,.24,1)';
  var SPRING = 'cubic-bezier(.34,1.3,.4,1)';
  var INFO_W = 272;     // the side page's width (the design's --iw)
  var GRID_PAD = 18;    // the month page's side padding
  var FULL_MAX_W = 1040;
  var COVER_SHUT = 'perspective(1100px) rotateY(0deg)';
  var COVER_OPEN = 'perspective(1100px) rotateY(-100deg)';
  var HOME = 'translate(0px,0px) scale(1)';

  var S = { state: 'closed', seq: 0 };

  function $(sel, root) { return (root || document).querySelector(sel); }
  function motion() { return !(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches); }
  // Only this file's own animations are ever cancelled: the calendar inside
  // the almanac runs animations of its own (its sky, its month sweeps).
  var mine = [];
  function anim(el, frames, o) {
    var a = el.animate(frames, Object.assign({ fill: 'forwards' }, o));
    mine.push(a);
    return a;
  }
  function stopMine() {
    mine.forEach(function (a) { a.cancel(); });
    mine = [];
  }
  function settle(list) { return Promise.all(list.map(function (a) { return a.finished; })); }
  function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }
  function fadeIn(el) { return anim(el, [{ opacity: 0 }, { opacity: 1 }], { duration: 180, easing: 'ease-out', fill: 'both' }).finished; }
  function fadeOut(el) { return anim(el, [{ opacity: 1 }, { opacity: 0 }], { duration: 150, easing: 'ease-in', fill: 'both' }).finished; }
  function tr(dx, dy, s) { return 'translate(' + dx.toFixed(1) + 'px,' + dy.toFixed(1) + 'px) scale(' + (s || 1).toFixed(4) + ')'; }

  function parts() {
    var a = S.alm;
    return {
      grid: $('#calv5-alm-grid', a), info: $('#calv5-alm-info', a), full: $('#calv5-alm-full', a),
      bar: $('.calv5-alm-bar', a), host: $('#calv5-alm-cal', a), mount: $('#calv5-alm-cal [data-widget="calendar_view"]', a)
    };
  }

  // A page lifted out of the layout and pinned where it was, so it can move
  // or fold while the almanac lays out its other state underneath it.
  function pin(el, r) {
    el.hidden = false;
    el.style.position = 'fixed';
    el.style.left = r.left + 'px';
    el.style.top = r.top + 'px';
    el.style.width = r.width + 'px';
    el.style.height = r.height + 'px';
    el.style.margin = '0';
    el.style.zIndex = '3';
  }
  function unpin(el) {
    ['position', 'left', 'top', 'width', 'height', 'margin', 'zIndex', 'transform', 'transformOrigin', 'opacity'].forEach(function (k) { el.style[k] = ''; });
  }

  // The move that lays the month page over the calendar's own grid: its
  // weekday row onto the calendar's weekday row, its weeks as wide as the
  // calendar's. at is where the page sits unmoved.
  function gridMatch(panel, at, mount) {
    var pd = $('.calv5-mg-dow', panel), rd = $('.dow', mount), rs = $('.stage', mount);
    if (!pd || !rd || !rs) return null;
    var d = pd.getBoundingClientRect(), r = rd.getBoundingClientRect(), s = rs.getBoundingClientRect().width / d.width;
    return tr(r.left - at.left - s * (d.left - at.left), r.top - at.top - s * (d.top - at.top), s);
  }

  // The page's own content column, beside the sidebar: the almanac opens
  // over it rather than over the whole window.
  function column() {
    var m = document.getElementById('main-content'), r = m && m.getBoundingClientRect();
    return r && r.width > 0 ? { left: r.left, width: r.width } : { left: 0, width: window.innerWidth };
  }

  // The folded page: the month sized for its week (never under 58px a day),
  // the side page beside it, centred and never closer than 16px to the top.
  // Too narrow for both side by side, it is one sheet instead.
  function layoutPreview() {
    var a = S.alm, p = parts(), grid = $('.calv5-mgrid', a);
    var cols = grid ? parseInt(grid.style.getPropertyValue('--cols'), 10) || 7 : 7;
    var cellW = clamp(Math.round(620 / cols), 58, 88), gw = cellW * cols + 2 * GRID_PAD;
    S.sheet = window.innerWidth < 640 || column().width - 32 < gw + INFO_W;
    a.classList.toggle('sheet', S.sheet);
    a.style.left = a.style.top = '';
    if (S.sheet) { S.pv = null; return; }
    a.style.setProperty('--cellW', cellW + 'px');
    a.style.setProperty('--gw', gw + 'px');
    a.style.setProperty('--rowH', 'auto');
    var rowH = Math.ceil(Math.max(p.grid.offsetHeight, p.info.offsetHeight));
    a.style.setProperty('--rowH', rowH + 'px');
    S.pv = { gw: gw, rowH: rowH };
    var box = previewBox();
    a.style.left = box.left + 'px';
    a.style.top = box.top + 'px';
  }
  function previewBox() {
    var W = S.pv.gw + INFO_W, c = column();
    return { left: Math.round(c.left + (c.width - W) / 2), top: Math.max(16, Math.round((window.innerHeight - S.pv.rowH) / 2)) };
  }
  // The two folded pages' places, worked out without showing them.
  function previewRects() {
    var b = previewBox();
    return {
      grid: { left: b.left, top: b.top, width: S.pv.gw, height: S.pv.rowH },
      info: { left: b.left + S.pv.gw, top: b.top, width: INFO_W, height: S.pv.rowH }
    };
  }

  // Opened out: the calendar on its own sheet, as wide as its page allows.
  function layoutFull() {
    var a = S.alm;
    a.style.left = a.style.top = '';
    if (S.sheet) return;
    var c = column(), W = Math.min(FULL_MAX_W, c.width - 32);
    a.style.setProperty('--fw', W + 'px');
    a.style.left = Math.round(c.left + (c.width - W) / 2) + 'px';
    a.style.top = '16px';
  }

  // The calendar the preview unfolds into, mounted once per open almanac and
  // kept through folding, so a second unfold is instant.
  function mountCalendar() {
    var p = parts();
    if (p.mount && p.mount.calendarView) return p.mount;
    var tpl = $('#calv5-alm-tpl', S.alm);
    if (!p.host || !tpl || !window.Chronicle || !Chronicle.mountWidget) return null;
    p.host.appendChild(tpl.content.cloneNode(true));
    var mount = $('[data-widget="calendar_view"]', p.host);
    if (mount) Chronicle.mountWidget(mount);
    return mount && mount.calendarView ? mount : null;
  }
  function unmountCalendar() {
    var p = S.alm && parts();
    if (p && p.mount && window.Chronicle && Chronicle.destroyWidget) Chronicle.destroyWidget(p.mount);
  }

  // The page at the size of its cover: a whole miniature of itself, centred
  // on the card.
  function coverFit(card) {
    var pr = S.alm.getBoundingClientRect(), cr = card.getBoundingClientRect();
    var s = Math.min(cr.width / pr.width, cr.height / pr.height);
    return tr(cr.left + (cr.width - pr.width * s) / 2 - pr.left, cr.top + (cr.height - pr.height * s) / 2 - pr.top, s);
  }
  // The cover is edge-on before the page moves, so the growing page never
  // passes under it.
  function hingeOpen(card) {
    var fit = coverFit(card);
    S.alm.style.transformOrigin = '0 0';
    card.classList.add('calv5-cover');
    anim(card, [{ transform: COVER_SHUT }, { transform: COVER_OPEN }], { duration: 260, easing: 'cubic-bezier(.45,0,.8,.45)', fill: 'both' });
    return anim(S.alm, [{ transform: fit, offset: 0 }, { transform: fit, offset: 0.42, easing: 'cubic-bezier(.2,.75,.25,1)' }, { transform: HOME, offset: 1 }], { duration: 640, fill: 'both' }).finished;
  }
  function hingeClose(card) {
    var fit = coverFit(card);
    S.alm.style.transformOrigin = '0 0';
    card.classList.remove('calv5-opened');
    card.classList.add('calv5-cover');
    anim(card, [{ transform: COVER_OPEN }, { transform: COVER_SHUT }], { duration: 280, delay: 340, easing: 'cubic-bezier(.2,.6,.3,1)', fill: 'both' });
    return anim(S.alm, [{ transform: HOME, offset: 0, easing: 'cubic-bezier(.5,0,.7,.3)' }, { transform: fit, offset: 0.52 }, { transform: fit, offset: 1 }], { duration: 620, fill: 'both' }).finished;
  }
  // A sheet grows from what was pressed, whole: solid once it is bigger than
  // a speck.
  function originOn(el, src) {
    var r = el.getBoundingClientRect(), s = src.getBoundingClientRect();
    el.style.transformOrigin = (s.left + s.width / 2 - r.left).toFixed(1) + 'px ' + (s.top + s.height / 2 - r.top).toFixed(1) + 'px';
  }
  function scaleOpen(el, src) {
    originOn(el, src);
    anim(el, [{ opacity: 0 }, { opacity: 1, offset: 0.08 }, { opacity: 1 }], { duration: 380, delay: 20, easing: 'linear', fill: 'both' });
    return anim(el, [{ transform: 'scale(.08)' }, { transform: 'scale(1)' }], { duration: 380, delay: 20, easing: EASE, fill: 'both' }).finished;
  }
  function scaleClose(el, src) {
    originOn(el, src);
    anim(el, [{ opacity: 1 }, { opacity: 1, offset: 0.9 }, { opacity: 0 }], { duration: 300, delay: 40, easing: 'linear', fill: 'both' });
    return anim(el, [{ transform: 'scale(1)' }, { transform: 'scale(.08)' }], { duration: 300, delay: 40, easing: 'cubic-bezier(.5,0,.75,0)', fill: 'both' }).finished;
  }

  function open(root) {
    if (S.state !== 'closed' && S.root && S.root.isConnected) return;
    var opener = window.__calv5Opener && window.__calv5Opener.isConnected ? window.__calv5Opener : null;
    var card = opener && opener.closest('.calv5-ccard');
    var tok = ++S.seq;
    S = { state: 'opening', seq: tok, root: root, alm: $('.calv5-alm', root), scrim: $('.calv5-alm-scrim', root), card: card, opener: opener };
    if (!S.alm || !S.scrim) { S = { state: 'closed', seq: tok }; return; }
    layoutPreview();
    S.alm.classList.add('live');
    S.scrim.classList.add('on');
    var hinge = motion() && !S.sheet && card;
    var run = !motion() || !card ? fadeIn(S.alm) : hinge ? hingeOpen(card) : scaleOpen(S.alm, opener);
    run.then(function () {
      if (tok !== S.seq) return;
      stopMine();
      S.alm.style.transformOrigin = '';
      if (card) {
        card.classList.remove('calv5-cover');
        if (hinge) card.classList.add('calv5-opened');
      }
      S.state = 'preview';
      S.alm.classList.add('open');
      var b = $('#calv5-alm-open', S.alm);
      if (b) b.focus({ preventScroll: true });
    }, function () {});
  }

  function close() {
    if (S.state !== 'preview') return Promise.resolve(false);
    var tok = ++S.seq, card = S.card && S.card.isConnected ? S.card : null;
    var hinge = motion() && !S.sheet && card && card.classList.contains('calv5-opened');
    S.state = 'closing';
    S.alm.classList.remove('open');
    S.scrim.classList.remove('on');
    var run = !motion() || !card ? fadeOut(S.alm) : hinge ? hingeClose(card) : scaleClose(S.alm, S.opener || card);
    return run.then(function () { return true; }, function () { return false; }).then(function () {
      if (tok !== S.seq) return false;
      finish();
      return true;
    });
  }

  // Closed: the calendar is taken down, the card is itself again, and the
  // preview's mount point waits for the next card.
  function finish() {
    stopMine();
    unmountCalendar();
    if (S.card) {
      S.card.classList.remove('calv5-cover', 'calv5-opened');
    }
    var fresh = document.createElement('div');
    fresh.id = 'calv5-preview-root';
    if (S.root && S.root.isConnected) S.root.replaceWith(fresh);
    var opener = S.opener;
    S = { state: 'closed', seq: S.seq + 1 };
    if (opener && opener.isConnected) opener.focus({ preventScroll: true });
  }

  // Opening out: the side page folds away behind the month, the month moves
  // whole to where the calendar's grid lives and becomes it, and the
  // calendar's header swings up from behind the grid's top edge.
  function unfold() {
    if (S.state !== 'preview') return Promise.resolve(false);
    var tok = ++S.seq, a = S.alm, p = parts();
    var g0 = p.grid.getBoundingClientRect(), i0 = p.info.getBoundingClientRect();
    var mount = mountCalendar();
    if (!mount) {
      // The calendar couldn't start here; its own page still can.
      var m = $('#calv5-alm-tpl', a) && $('#calv5-alm-tpl', a).content.querySelector('[data-api-base]');
      if (m) window.location.assign(m.getAttribute('data-api-base') + '/view');
      return Promise.resolve(false);
    }
    S.state = 'unfolding';
    a.classList.remove('open');
    var toFull = function (pinned) {
      if (pinned) { pin(p.grid, g0); pin(p.info, i0); } else { p.grid.hidden = p.info.hidden = true; }
      p.full.hidden = false;
      a.classList.remove('pv');
      a.classList.add('full');
      layoutFull();
    };
    var head = $('.head', mount), cap = $('.caption', mount), grid = [$('.dow', mount), $('.stage', mount)].filter(Boolean), run;
    if (!motion() || S.sheet || !head) {
      // A cross-fade: the folded pages go, then the calendar comes; on a
      // phone it drops open downwards.
      run = Promise.all([fadeOut(p.grid), fadeOut(p.info)]).then(function () {
        if (tok !== S.seq) return Promise.reject(new Error('superseded'));
        stopMine();
        toFull(false);
        if (!motion()) return fadeIn(p.full);
        p.full.style.transformOrigin = '50% 0';
        return anim(p.full, [{ opacity: 0, transform: 'perspective(600px) rotateX(-65deg)' }, { opacity: 1, transform: 'none' }], { duration: 340, easing: SPRING, fill: 'both' }).finished;
      });
    } else {
      toFull(true);
      var T = 820, at = gridMatch(p.grid, g0, mount) || HOME, list = [];
      p.info.style.transformOrigin = '0 50%';
      list.push(anim(p.info, [
        { transform: 'perspective(1100px) rotateY(0deg)', offset: 0, easing: 'cubic-bezier(.55,0,.75,.45)' },
        { transform: 'perspective(1100px) rotateY(180deg)', offset: 0.4 },
        { transform: 'perspective(1100px) rotateY(180deg)', offset: 1 }], { duration: T, fill: 'both' }));
      p.grid.style.transformOrigin = '0 0';
      list.push(anim(p.grid, [{ transform: HOME, offset: 0 }, { transform: HOME, offset: 0.3, easing: EASE }, { transform: at, offset: 0.66 }, { transform: at, offset: 1 }], { duration: T, fill: 'both' }));
      list.push(anim(p.grid, [{ opacity: 1, offset: 0 }, { opacity: 1, offset: 0.64 }, { opacity: 0, offset: 0.8 }, { opacity: 0, offset: 1 }], { duration: T, fill: 'both' }));
      list.push(anim(p.full, [{ opacity: 0, offset: 0 }, { opacity: 0, offset: 0.2 }, { opacity: 1, offset: 0.5 }, { opacity: 1, offset: 1 }], { duration: T, fill: 'both' }));
      grid.forEach(function (g) {
        list.push(anim(g, [{ opacity: 0, offset: 0 }, { opacity: 0, offset: 0.64 }, { opacity: 1, offset: 0.8 }, { opacity: 1, offset: 1 }], { duration: T, fill: 'both' }));
      });
      head.classList.add('calv5-flap');
      list.push(anim(head, [
        { transform: 'perspective(1500px) rotateX(180deg)', offset: 0 },
        { transform: 'perspective(1500px) rotateX(180deg)', offset: 0.6, easing: 'cubic-bezier(.3,.55,.25,1)' },
        { transform: 'perspective(1500px) rotateX(0deg)', offset: 1 }], { duration: T, fill: 'both' }));
      [p.bar, cap].forEach(function (el) {
        if (el) list.push(anim(el, [{ opacity: 0, offset: 0 }, { opacity: 0, offset: 0.8 }, { opacity: 1, offset: 1 }], { duration: T, fill: 'both' }));
      });
      run = settle(list);
    }
    return run.then(function () {
      if (tok !== S.seq) return false;
      stopMine();
      if (head) head.classList.remove('calv5-flap');
      p.full.style.transformOrigin = '';
      p.grid.hidden = p.info.hidden = true;
      unpin(p.grid);
      unpin(p.info);
      S.state = 'full';
      a.classList.add('open');
      var day = $('.stage [tabindex="0"]', mount) || $('[data-alm="fold"]', a);
      if (day) day.focus({ preventScroll: true });
      return true;
    }, function () { return false; });
  }

  // Folding up runs the same way back: the header folds down behind the
  // grid, the calendar's grid becomes the month page again, which moves
  // home, and the side page swings out from behind it.
  function fold() {
    if (S.state !== 'full') return Promise.resolve(false);
    var tok = ++S.seq, a = S.alm, p = parts(), mount = p.mount, view = mount && mount.calendarView;
    if (view && view.closeAllPanels) view.closeAllPanels();
    S.state = 'folding';
    a.classList.remove('open');
    var head = mount && $('.head', mount), cap = mount && $('.caption', mount);
    var grid = mount ? [$('.dow', mount), $('.stage', mount)].filter(Boolean) : [];
    var run;
    var toPreview = function () {
      stopMine();
      if (head) head.classList.remove('calv5-flap');
      p.full.hidden = true;
      unpin(p.grid);
      unpin(p.info);
      p.grid.hidden = p.info.hidden = false;
      a.classList.remove('full');
      a.classList.add('pv');
      layoutPreview();
    };
    if (!motion() || S.sheet || !head || !S.pv) {
      run = fadeOut(p.full).then(function () {
        if (tok !== S.seq) return Promise.reject(new Error('superseded'));
        toPreview();
        return Promise.all([fadeIn(p.grid), fadeIn(p.info)]);
      });
    } else {
      var T = 760, pv = previewRects(), list = [];
      pin(p.grid, pv.grid);
      pin(p.info, pv.info);
      var at = gridMatch(p.grid, pv.grid, mount) || HOME;
      [p.bar, cap].forEach(function (el) {
        if (el) list.push(anim(el, [{ opacity: 1 }, { opacity: 0, offset: 0.15 }, { opacity: 0 }], { duration: T, fill: 'both' }));
      });
      head.classList.add('calv5-flap');
      list.push(anim(head, [
        { transform: 'perspective(1500px) rotateX(0deg)', offset: 0, easing: 'cubic-bezier(.55,0,.75,.45)' },
        { transform: 'perspective(1500px) rotateX(180deg)', offset: 0.4 },
        { transform: 'perspective(1500px) rotateX(180deg)', offset: 1 }], { duration: T, fill: 'both' }));
      grid.forEach(function (g) {
        list.push(anim(g, [{ opacity: 1, offset: 0 }, { opacity: 1, offset: 0.35 }, { opacity: 0, offset: 0.5 }, { opacity: 0, offset: 1 }], { duration: T, fill: 'both' }));
      });
      list.push(anim(p.full, [{ opacity: 1, offset: 0 }, { opacity: 1, offset: 0.5 }, { opacity: 0, offset: 0.8 }, { opacity: 0, offset: 1 }], { duration: T, fill: 'both' }));
      p.grid.style.transformOrigin = '0 0';
      list.push(anim(p.grid, [{ opacity: 0, offset: 0 }, { opacity: 0, offset: 0.35 }, { opacity: 1, offset: 0.5 }, { opacity: 1, offset: 1 }], { duration: T, fill: 'both' }));
      list.push(anim(p.grid, [{ transform: at, offset: 0 }, { transform: at, offset: 0.5, easing: EASE }, { transform: HOME, offset: 0.85 }, { transform: HOME, offset: 1 }], { duration: T, fill: 'both' }));
      p.info.style.transformOrigin = '0 50%';
      list.push(anim(p.info, [
        { transform: 'perspective(1100px) rotateY(180deg)', offset: 0 },
        { transform: 'perspective(1100px) rotateY(180deg)', offset: 0.7, easing: 'cubic-bezier(.3,.55,.25,1)' },
        { transform: 'perspective(1100px) rotateY(0deg)', offset: 1 }], { duration: T, fill: 'both' }));
      run = settle(list).then(function () {
        if (tok !== S.seq) return Promise.reject(new Error('superseded'));
        toPreview();
      });
    }
    return run.then(function () {
      if (tok !== S.seq) return false;
      stopMine();
      S.state = 'preview';
      a.classList.add('open');
      var b = $('#calv5-alm-open', a);
      if (b) b.focus({ preventScroll: true });
      return true;
    }, function () { return false; });
  }

  // Escape, and a click on the dimmed page: an opened-out almanac folds up
  // (unless the calendar has a card of its own open, which it closes first);
  // a preview closes into its card.
  function escape() {
    if (S.state === 'full') {
      var m = parts().mount, v = m && m.calendarView;
      if (v && v.anyPanelOpen && v.anyPanelOpen()) return false;
      fold();
    } else if (S.state === 'preview') {
      close();
    }
    return true;
  }

  // Tab stays inside the almanac while it is open.
  function trapTab(e) {
    var nodes = Array.prototype.filter.call(
      S.alm.querySelectorAll('a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])'),
      function (n) { return n.getClientRects().length > 0 && !n.closest('[hidden],template'); });
    if (!nodes.length) return;
    var first = nodes[0], last = nodes[nodes.length - 1], cur = document.activeElement;
    if (!S.alm.contains(cur)) { e.preventDefault(); first.focus(); }
    else if (e.shiftKey && cur === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && cur === last) { e.preventDefault(); first.focus(); }
  }

  function live() { return S.state !== 'closed' && S.alm && S.alm.isConnected; }

  // htmx swaps a card's preview into #calv5-preview-root; that is the cue.
  document.addEventListener('htmx:load', function (e) {
    var el = e.detail && e.detail.elt;
    if (el && el.id === 'calv5-preview-root' && el.hasAttribute('data-almanac')) open(el);
  });

  document.addEventListener('click', function (e) {
    if (!live()) return;
    var t = e.target.closest && e.target.closest('[data-alm]');
    if (!t || !S.root.contains(t)) return;
    var act = t.getAttribute('data-alm');
    if (act === 'unfold') unfold();
    else if (act === 'fold') fold();
    else if (act === 'escape') escape();
    else if (act === 'close') {
      if (S.state === 'full') fold().then(function (ok) { if (ok) close(); });
      else close();
    }
  });

  // Capture phase, so the almanac decides before the calendar's own Escape.
  document.addEventListener('keydown', function (e) {
    if (!live()) return;
    if (e.key === 'Escape') {
      if (S.state === 'full' || S.state === 'preview') {
        if (escape()) { e.preventDefault(); e.stopPropagation(); }
      } else {
        e.preventDefault();
        e.stopPropagation();
      }
    } else if (e.key === 'Tab' && S.state !== 'closing') {
      trapTab(e);
    }
  }, true);

  var resizeTimer = 0;
  window.addEventListener('resize', function () {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(function () {
      if (!live()) return;
      if (S.state === 'preview') layoutPreview();
      else if (S.state === 'full') layoutFull();
    }, 120);
  });

  // Leaving the Calendars page (the sidebar swaps the page out from under
  // it) takes the almanac along; boot.js destroys the calendar inside.
  document.addEventListener('htmx:beforeSwap', function (e) {
    var t = e.detail && e.detail.target;
    if (live() && t && t !== S.root && t.contains(S.root)) {
      stopMine();
      S = { state: 'closed', seq: S.seq + 1 };
    }
  });
})();
