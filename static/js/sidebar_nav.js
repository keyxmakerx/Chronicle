/**
 * sidebar_nav.js -- the campaign sidebar's behaviour (nav.templ)
 *
 * - Folds each section, and each category's sub-categories, under its
 *   heading. Folds are remembered per viewer in a per-campaign cookie that
 *   the server reads, so a page load paints them already right.
 * - Keeps the current row, its page name and the living ring in step with
 *   boosted navigation, which swaps only #main-content: the layout renders a
 *   [data-nav-current] marker there, and this reads it. While a category's
 *   panel (sidebar_drill.js) is open, the ring marks the panel's header.
 * - The phone drawer: out of the tab order while shut, focus kept inside
 *   while open. (Escape and the menu button live in app.templ's Alpine.)
 *
 * Motion moves only transform and opacity. Content is laid out at its final
 * size first and never revealed by a moving clip; the ring is one element
 * that moves between rows, never a copy, and fades in where it lands rather
 * than travelling: a glide from a far row swept it across every row between.
 * Under reduced motion everything crossfades.
 */
(function () {
  'use strict';

  var FOLD_MS = 280;
  // A closing section's rows fade out this fast, so none are left showing
  // below a follower shorter than the section (Manage above a folded Admin).
  var FADE_OUT_MS = 120;
  var EASE = 'cubic-bezier(.2,.8,.2,1)';
  var EASE_OUT = 'cubic-bezier(.16,1,.3,1)';
  var COOKIE = 'chronicle_nav_folds';
  var FOLD_ID = /^[A-Za-z0-9_-]{1,48}$/;

  var rmQuery = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
  var phoneQuery = window.matchMedia ? window.matchMedia('(max-width: 767px)') : null;

  function reduced() {
    return !!(rmQuery && rmQuery.matches) || document.documentElement.classList.contains('nav-rm');
  }

  // Element.animate, or nothing where it is missing: every caller copes with null.
  function play(el, frames, opts) {
    return el && el.animate ? el.animate(frames, opts) : null;
  }

  function whenDone(anim, fn) {
    if (!anim) { fn(); return; }
    anim.finished.then(fn, fn);
  }

  // The categories area's own list — exists on every campaign page, so it
  // doubles as the "is this a campaign sidebar" guard. campaignNavTop's own
  // rows (Dashboard, Pinned, Apps) live in a separate list; sidebar() below
  // is the shared ancestor searches that must span both use instead.
  function list() { return document.getElementById('sidebar-nav-list'); }

  function shown(el) { return !!el && el.offsetParent !== null; }

  // --- Fold memory ---------------------------------------------------------

  /** Parses the cookie's "id:1|id:0" form into {id: open}. */
  function parseFolds(raw) {
    var out = {};
    if (!raw || raw.length > 2048) return out;
    raw.split('|').slice(0, 64).forEach(function (part) {
      var i = part.indexOf(':');
      if (i < 1) return;
      var id = part.slice(0, i), v = part.slice(i + 1);
      if (FOLD_ID.test(id) && (v === '0' || v === '1')) out[id] = v === '1';
    });
    return out;
  }

  /** The inverse of parseFolds, in a stable order. */
  function serializeFolds(folds) {
    return Object.keys(folds).filter(function (id) { return FOLD_ID.test(id); }).sort()
      .map(function (id) { return id + ':' + (folds[id] ? '1' : '0'); }).join('|');
  }

  function readFolds() {
    var parts = (document.cookie || '').split(/;\s*/);
    for (var i = 0; i < parts.length; i++) {
      if (parts[i].indexOf(COOKIE + '=') === 0) return parseFolds(parts[i].slice(COOKIE.length + 1));
    }
    return {};
  }

  function writeFolds(folds) {
    var nav = document.getElementById('sidebar-nav');
    var base = nav && nav.getAttribute('data-nav-base');
    if (!base) return;
    document.cookie = COOKIE + '=' + serializeFolds(folds) + '; path=' + base + '; max-age=31536000; SameSite=Lax';
  }

  // --- Folding -------------------------------------------------------------

  /** Everything shown below body in its own list: what slides when it
   *  folds. Only Apps folds inside campaignNavTop, and it is always that
   *  list's last section, so this walk itself finds nothing there — the
   *  categories area below still has to slide to meet it, so a fold inside
   *  .nav-top carries that whole area along as one more follower (its real
   *  height depends on .nav-top's, since it is flex:1 next to it). */
  function followers(body) {
    var root = body.closest('.nav-list'), out = [];
    for (var node = body; node && node !== root; node = node.parentElement) {
      for (var sib = node.nextElementSibling; sib; sib = sib.nextElementSibling) {
        if (shown(sib)) out.push(sib);
      }
    }
    var topWrap = body.closest('.nav-top');
    var zone = topWrap && topWrap.parentElement && topWrap.parentElement.querySelector('.nav-cat-zone');
    if (shown(zone)) out.push(zone);
    return out;
  }

  // A fold that is still moving finishes at once when its heading is pressed
  // again, so two animations never fight over one body.
  function settle(body) {
    if (body._navSettle) body._navSettle();
  }

  /** What fades while a fold opens or closes: its rows, twisties, and any
   *  member pins that are showing (hidden ones stay hidden). */
  function foldContent(body) {
    return Array.prototype.filter.call(body.querySelectorAll('.nav-row, .nav-tw, .nav-mypin'), function (el) {
      return !el.classList.contains('nav-mypin') || getComputedStyle(el).opacity !== '0';
    });
  }

  function unfold(body) {
    body.hidden = false;
    if (reduced()) {
      play(body, [{ opacity: 0 }, { opacity: 1 }], { duration: 180, easing: 'linear' });
      return;
    }
    var h = body.offsetHeight;
    var after = followers(body), anims = [];
    // The rows below start where the closed section left them and slide down
    // over the opened rows, which are already drawn at full size underneath.
    after.forEach(function (el) {
      el.classList.add('nav-cover');
      anims.push(play(el, [{ transform: 'translateY(' + (-h) + 'px)' }, { transform: 'none' }], { duration: FOLD_MS, easing: EASE }));
    });
    // The opened rows fade in drawn whole, each settling from a few pixels
    // up, as the signed mockup draws them.
    var rows = foldContent(body);
    for (var i = 0; i < rows.length; i++) {
      anims.push(play(rows[i], [{ opacity: 0, transform: 'translateY(-8px)' }, { opacity: 1, transform: 'none' }],
        { duration: FOLD_MS, delay: Math.min(i, 8) * 16, easing: EASE_OUT, fill: 'backwards' }));
    }
    var cleanup = function () {
      body._navSettle = null;
      after.forEach(function (el) { el.classList.remove('nav-cover'); });
    };
    body._navSettle = function () { anims.forEach(function (a) { if (a) a.finish(); }); cleanup(); };
    whenDone(anims[0], cleanup);
  }

  function fold(body, onDone) {
    if (reduced()) {
      var fade = play(body, [{ opacity: 1 }, { opacity: 0 }], { duration: 140, easing: 'linear', fill: 'forwards' });
      whenDone(fade, function () { body.hidden = true; if (fade) fade.cancel(); if (onDone) onDone(); });
      return;
    }
    var h = body.offsetHeight;
    var after = followers(body), anims = [];
    var rows = foldContent(body);
    for (var i = 0; i < rows.length; i++) {
      anims.push(play(rows[i], [{ opacity: 1 }, { opacity: 0 }], { duration: FADE_OUT_MS, easing: 'ease-out', fill: 'forwards' }));
    }
    // The rows below slide up over the closing section, like a drawer shutting.
    var lead = null;
    after.forEach(function (el) {
      el.classList.add('nav-cover');
      var a = play(el, [{ transform: 'none' }, { transform: 'translateY(' + (-h) + 'px)' }], { duration: FOLD_MS, easing: EASE, fill: 'forwards' });
      anims.push(a);
      if (!lead) lead = a;
    });
    var finished = false;
    var finish = function () {
      if (finished) return;
      finished = true;
      body._navSettle = null;
      body.hidden = true;
      anims.forEach(function (a) { if (a) a.cancel(); });
      after.forEach(function (el) { el.classList.remove('nav-cover'); });
      if (onDone) onDone();
    };
    body._navSettle = finish;
    whenDone(lead || anims[anims.length - 1] || null, finish);
  }

  function toggleFold(btn) {
    var id = btn.getAttribute('data-nav-fold');
    var body = document.getElementById(btn.getAttribute('aria-controls') || '');
    if (!id || !body) return;
    settle(body);
    var open = btn.getAttribute('aria-expanded') !== 'true';
    btn.setAttribute('aria-expanded', String(open));
    var folds = readFolds();
    folds[id] = open;
    writeFolds(folds);

    var cur = currentKey();
    var holdsCurrent = cur && body.querySelector('.nav-row[data-nav-key="' + cssEscape(cur) + '"]');
    if (open) {
      unfold(body);
      // Opening a category's sub-categories hands the ring back to the sub row.
      if (holdsCurrent) moveRing(ringTargetFor(cur), true);
    } else if (holdsCurrent && body.classList.contains('nav-subs')) {
      // The ring leaves a closing sub row for its category's row at once.
      moveRing(body.parentElement.querySelector('.nav-row'), true);
      fold(body);
    } else {
      fold(body);
    }
  }

  // --- Where the viewer is -------------------------------------------------

  function cssEscape(s) {
    return window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/["\\]/g, '\\$&');
  }

  // A row can be in either part (campaignNavTop or the categories area), so
  // this and every other "where is X" lookup below searches the whole
  // sidebar, not just one list.
  function rowFor(key) {
    var root = sidebar();
    return key && root ? root.querySelector('.nav-row[data-nav-key="' + cssEscape(key) + '"]') : null;
  }

  function currentKey() {
    var m = document.querySelector('#main-content [data-nav-current]');
    return m ? m.getAttribute('data-nav-current') || '' : '';
  }

  /** The row the ring belongs on: the current row, or its category while
   *  its sub-categories are folded; none while its section is folded. */
  function ringTargetFor(key) {
    // An open category panel covers the categories, so while it is open the
    // ring marks its header: that category is where the reader now is.
    var head = document.querySelector('#sidebar-drill.is-open .nav-drill-head');
    if (shown(head)) return head;
    var row = rowFor(key);
    if (shown(row)) return row;
    var rw = row && row.parentElement && row.parentElement.closest('.nav-rw');
    var parent = rw && rw.querySelector('.nav-row');
    return shown(parent) ? parent : null;
  }

  function ringHTML() {
    return '<span class="nav-ring-tint"></span>' +
      '<svg class="nav-ring-svg" focusable="false"><rect class="nav-ring-t1" width="100%" height="100%" pathLength="100"></rect>' +
      '<rect class="nav-ring-t2" width="100%" height="100%" pathLength="100"></rect></svg><span class="nav-ring-j"></span>';
  }

  function ringEl() {
    var root = sidebar();
    var ring = root && root.querySelector('.nav-ring');
    if (!ring) {
      ring = document.createElement('span');
      ring.className = 'nav-ring';
      ring.setAttribute('aria-hidden', 'true');
      ring.innerHTML = ringHTML();
    }
    return ring;
  }

  // Moving the ring to another row restarts its CSS animations; setting each
  // trace's delay from the page's own clock keeps them on the phase a ring
  // placed at page load would have reached, so the traces never jump.
  function keepPhase(ring) {
    var now = window.performance ? performance.now() : 0;
    var t1 = ring.querySelector('.nav-ring-t1'), t2 = ring.querySelector('.nav-ring-t2');
    if (t1) t1.style.animationDelay = (-(now % 14000)) + 'ms';
    if (t2) t2.style.animationDelay = (-(now % 19000)) + 'ms';
  }

  function moveRing(target, animate) {
    var ring = ringEl();
    if (!target) {
      if (ring.parentElement) ring.parentElement.removeChild(ring);
      return;
    }
    if (ring.parentElement === target) return;
    target.appendChild(ring);
    keepPhase(ring);
    if (animate) play(ring, [{ opacity: 0 }, { opacity: 1 }], { duration: 200, easing: 'ease-out' });
  }

  function restoreTrail(row) {
    var tr = row.querySelector('.nav-tr');
    if (!tr) return;
    tr.textContent = row.getAttribute('data-nav-tr') || '';
    tr.classList.remove('is-page');
  }

  /** Names the current row's section on its heading, for when it is folded. */
  function markHeadings(key) {
    var root = sidebar();
    if (!root) return;
    var row = rowFor(key);
    var secs = root.querySelectorAll('.nav-sec');
    for (var i = 0; i < secs.length; i++) {
      var head = secs[i].querySelector('.nav-gh[data-nav-fold]');
      if (!head) continue;
      var inside = !!row && secs[i].contains(row);
      head.classList.toggle('has-cur', inside);
      var cur = head.querySelector('.nav-gh-cur');
      var label = inside ? row.querySelector('.nav-lb') : null;
      if (cur) cur.textContent = label ? label.textContent : '';
    }
  }

  /** Makes key the current row, showing page as its trailing name. */
  function setCurrent(key, page, animate) {
    var root = list();
    if (!root) return;
    var old = root.querySelectorAll('.nav-row[aria-current="page"]');
    for (var i = 0; i < old.length; i++) {
      old[i].removeAttribute('aria-current');
      restoreTrail(old[i]);
    }
    var row = rowFor(key);
    if (row) {
      row.setAttribute('aria-current', 'page');
      var tr = row.querySelector('.nav-tr');
      if (tr && page) {
        tr.textContent = page;
        tr.classList.add('is-page');
      }
    }
    markHeadings(key);
    moveRing(ringTargetFor(key), animate);
  }

  function syncFromMarker(animate) {
    var m = document.querySelector('#main-content [data-nav-current]');
    if (m) setCurrent(m.getAttribute('data-nav-current') || '', m.getAttribute('data-nav-page') || '', animate);
  }

  // --- The phone drawer ----------------------------------------------------

  function sidebar() { return document.getElementById('sidebar'); }

  function drawerOpen() {
    var sb = sidebar();
    return !!sb && sb.classList.contains('sidebar-mobile-open');
  }

  // Shut, the drawer is out of the tab order and hidden from assistive tech;
  // open on a phone, it is a modal dialog.
  function syncDrawer() {
    var sb = sidebar();
    if (!sb || !list()) return;
    var phone = !!(phoneQuery && phoneQuery.matches);
    if (phone && !drawerOpen()) {
      sb.setAttribute('inert', '');
      sb.removeAttribute('role');
      sb.removeAttribute('aria-modal');
    } else if (phone) {
      sb.removeAttribute('inert');
      sb.setAttribute('role', 'dialog');
      sb.setAttribute('aria-label', 'Navigation');
      // While the owner edits, the save bar outside the drawer is live too.
      if (editing()) sb.removeAttribute('aria-modal');
      else sb.setAttribute('aria-modal', 'true');
    } else {
      sb.removeAttribute('inert');
      sb.removeAttribute('role');
      sb.removeAttribute('aria-modal');
      sb.removeAttribute('aria-label');
    }
  }

  var wasOpen = false, wasCollapsed = false;
  function onSidebarClass() {
    // Folded to icons, sub-category rows are hidden: the ring moves to their
    // category's row, and back when the sidebar opens out again.
    var sb = sidebar();
    var collapsed = !!sb && sb.classList.contains('sidebar-collapsed');
    if (collapsed !== wasCollapsed) {
      wasCollapsed = collapsed;
      var key = currentKey();
      if (key) moveRing(ringTargetFor(key), false);
    }
    onDrawerChange();
  }

  function onDrawerChange() {
    var open = drawerOpen();
    if (open === wasOpen) return;
    wasOpen = open;
    syncDrawer();
    var sb = sidebar();
    if (!(phoneQuery && phoneQuery.matches)) return;
    if (reduced()) {
      // The slide is off under reduced motion; fade instead. A transform:none
      // keyframe holds the drawer in place while it fades out.
      play(sb, open ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'none' }],
        { duration: open ? 200 : 160, easing: 'ease-out' });
    }
    if (open) {
      var first = sb.querySelector('#sidebar-nav [aria-current="page"]') || sb.querySelector('a[href], button:not([disabled])');
      if (first) {
        first.focus({ preventScroll: true });
        if (first.scrollIntoView) first.scrollIntoView({ block: 'nearest' });
      }
    }
  }

  function trapTab(e) {
    var sel = 'a[href], button:not([disabled]), input, select, textarea, [tabindex]:not([tabindex="-1"])';
    var items = Array.prototype.filter.call(sidebar().querySelectorAll(sel), shown);
    // While the owner edits, Tab also reaches the save bar.
    var bar = document.getElementById('nav-ebar');
    if (bar) items = items.concat(Array.prototype.filter.call(bar.querySelectorAll(sel), shown));
    if (!items.length) return;
    var first = items[0], last = items[items.length - 1], cur = document.activeElement;
    if (items.indexOf(cur) < 0) { e.preventDefault(); first.focus(); }
    else if (e.shiftKey && cur === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && cur === last) { e.preventDefault(); first.focus(); }
  }

  // --- Wiring ----------------------------------------------------------------

  function editing() {
    return document.body.classList.contains('nav-editing');
  }

  // Watches the sidebar's classes for the drawer opening and the folding to
  // icons. It follows the sidebar element, which Back replaces (below).
  var observer = null;

  function init() {
    if (observer) observer.disconnect();
    observer = null;
    if (!list()) return;
    var sb = sidebar();
    if (sb && window.MutationObserver) {
      observer = new MutationObserver(onSidebarClass);
      observer.observe(sb, { attributes: true, attributeFilter: ['class'] });
    }
    wasOpen = drawerOpen();
    wasCollapsed = !!sb && sb.classList.contains('sidebar-collapsed');
    if (wasCollapsed && currentKey()) moveRing(ringTargetFor(currentKey()), false);
    syncDrawer();
  }

  if (phoneQuery && phoneQuery.addEventListener) phoneQuery.addEventListener('change', syncDrawer);

  document.addEventListener('click', function (e) {
    var t = e.target;
    if (!t || !t.closest || !list()) return;
    // Apps' own fold heading lives in campaignNavTop, Manage's and a
    // category's twisty in the categories area — both under #sidebar.
    var fold = t.closest('#sidebar [data-nav-fold]');
    if (fold) {
      e.preventDefault();
      if (!editing()) toggleFold(fold);
      return;
    }
    // A plain click on a boosted row: the ring leaves for it at once, and the
    // page's marker confirms the row when the page arrives. A category row
    // is a <button> that opens the drill panel (sidebar_drill.js), never an
    // <a>; the ring follows that panel through 'chronicle:drill' instead.
    var row = t.closest('#sidebar a.nav-row');
    if (!row || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    if (row.target === '_blank' || row.getAttribute('hx-boost') === 'false' || editing()) return;
    moveRing(row, true);
  });

  window.addEventListener('chronicle:nav-editing', syncDrawer);

  // sidebar_drill.js says when a category panel opens (once its content is
  // in) or closes; the ring follows it there and back.
  document.addEventListener('chronicle:drill', function () {
    moveRing(ringTargetFor(currentKey()), true);
  });

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Tab' && drawerOpen() && phoneQuery && phoneQuery.matches) trapTab(e);
  });

  document.addEventListener('htmx:afterSettle', function (e) {
    var target = e.detail && e.detail.target;
    if (target && target.id === 'main-content') syncFromMarker(true);
  });

  // Back and Forward swap in a whole new body (htmx fetches the page, since
  // boot.js keeps its history cache off), so the sidebar is a new element:
  // watch the new one, re-apply the drawer's state, and put the ring on the
  // row the restored page names.
  document.addEventListener('htmx:historyRestore', function () {
    init();
    syncFromMarker(false);
  });

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.nav = {
    setCurrent: setCurrent,
    parseFolds: parseFolds,
    serializeFolds: serializeFolds
  };
})();
