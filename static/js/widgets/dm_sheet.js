/**
 * dm_sheet.js - the DM Screen's Adventures tab: a page opened over the screen,
 * and the picker that pins a page to the tab.
 *
 * The sheet is its own layer on <body>, above the screen, so the screen
 * underneath is never navigated, re-rendered or moved. It grows out of the
 * card that was pressed (a clip-path from the card's box; reduced motion skips
 * the growth), traps Tab, closes on Escape, and hands focus back to the card.
 * Escape is taken in the capture phase so it closes the sheet only; the screen
 * stays open. The tab's buttons call in through inline handlers built Go-side
 * (dmscreen/onclick_handlers.go), because the panel is swapped on refresh.
 */
(function () {
  'use strict';

  window.Chronicle = window.Chronicle || {};
  var C = window.Chronicle;

  var FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), ' +
    'textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable="true"]';

  var cur = null; // the open sheet's state, or null

  function reduced() {
    return window.Chronicle && Chronicle.reducedMotion ? Chronicle.reducedMotion() :
      !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  function cards() {
    var o = document.querySelector('.dms-overlay');
    return o ? Array.prototype.slice.call(o.querySelectorAll('[data-dms-sheet]')) : [];
  }

  function visibleFocusables(root) {
    return Array.prototype.filter.call(root.querySelectorAll(FOCUSABLE), function (n) {
      return !n.closest('[hidden]') && (n.offsetWidth || n.offsetHeight || n.getClientRects().length);
    });
  }

  function build() {
    var s = document.createElement('div');
    s.className = 'dms-sheet';
    s.setAttribute('role', 'dialog');
    s.setAttribute('aria-modal', 'true');
    s.setAttribute('aria-label', 'Adventure page');
    s.innerHTML =
      '<div class="dms-sheet-bg" data-sheet-close></div>' +
      '<div class="dms-sheet-p" tabindex="-1">' +
      '<div class="dms-sheet-top">' +
      '<span class="dms-sheet-crumb" data-sheet-crumb></span>' +
      '<button type="button" class="dms-sheet-btn" data-sheet-prev>← Prev</button>' +
      '<button type="button" class="dms-sheet-btn" data-sheet-next>Next →</button>' +
      '<button type="button" class="dms-sheet-btn dms-sheet-x" data-sheet-close aria-label="Close the page">×</button>' +
      '</div>' +
      '<div class="dms-sheet-b" data-sheet-body aria-live="polite"></div>' +
      '</div>';
    return s;
  }

  // Start the clip-path at the pressed card's box so the page unfolds from it.
  function setOrigin(panel, from) {
    var p = panel.getBoundingClientRect();
    if (!from || !p.width || !p.height) return false;
    var r = from.getBoundingClientRect();
    panel.style.setProperty('--ct', Math.max(0, r.top - p.top) + 'px');
    panel.style.setProperty('--cl', Math.max(0, r.left - p.left) + 'px');
    panel.style.setProperty('--cr', Math.max(0, p.right - r.right) + 'px');
    panel.style.setProperty('--cb', Math.max(0, p.bottom - r.bottom) + 'px');
    return true;
  }

  function open(btn) {
    if (!btn || cur) return;
    var url = btn.getAttribute('data-sheet-url');
    if (!url) return;

    var sheet = build();
    var panel = sheet.querySelector('.dms-sheet-p');
    var still = reduced();
    var screen = document.querySelector('.dms-overlay');

    cur = { el: sheet, panel: panel, id: btn.getAttribute('data-dms-sheet'), opener: btn, token: 0, screen: screen };
    document.body.appendChild(sheet);
    // The screen stays as it is but is out of the tab order and the reader's
    // tree while the page is over it.
    if (screen) screen.inert = true;

    if (!still && setOrigin(panel, btn)) {
      // One frame at the card's box, then open to full size.
      requestAnimationFrame(function () { requestAnimationFrame(function () { sheet.classList.add('dms-sheet-open'); }); });
    } else {
      sheet.classList.add('dms-sheet-open', 'dms-sheet-still');
    }

    sheet.addEventListener('click', onClick);
    sheet.addEventListener('keydown', onSheetKey);
    window.addEventListener('keydown', onEscape, true);
    document.addEventListener('focusin', onFocusIn);

    load(url);
    panel.focus({ preventScroll: true });
  }

  function load(url) {
    var c = cur;
    if (!c) return;
    var token = ++c.token;
    var body = c.el.querySelector('[data-sheet-body]');
    body.setAttribute('aria-busy', 'true');
    body.innerHTML = '<p class="dms-sheet-wait">Opening the page…</p>';
    updateNav();

    C.apiFetch(url, { headers: { Accept: 'text/html' } }).then(function (r) {
      if (!r.ok) throw new Error('refused');
      return r.text();
    }).then(function (html) {
      if (cur !== c || c.token !== token) return;
      body.innerHTML = html;
      body.removeAttribute('aria-busy');
      // The page's own lazy loads (history, backlinks, files) are htmx.
      if (window.htmx) window.htmx.process(body);
      if (C.mountWidgets) C.mountWidgets(body);
      c.panel.scrollTop = 0;
    }).catch(function () {
      if (cur !== c || c.token !== token) return;
      body.removeAttribute('aria-busy');
      body.innerHTML = '<p class="dms-sheet-wait">That page could not be opened. It may have been moved or hidden.</p>';
    });
  }

  function updateNav() {
    var c = cur;
    if (!c) return;
    var list = cards();
    var i = indexOf(list, c.id);
    var crumb = c.el.querySelector('[data-sheet-crumb]');
    crumb.textContent = i >= 0 ? 'Adventures · ' + (i + 1) + ' of ' + list.length : 'Adventures';
    c.el.querySelector('[data-sheet-prev]').disabled = i <= 0;
    c.el.querySelector('[data-sheet-next]').disabled = i < 0 || i >= list.length - 1;
  }

  function indexOf(list, id) {
    for (var i = 0; i < list.length; i++) {
      if (list[i].getAttribute('data-dms-sheet') === id) return i;
    }
    return -1;
  }

  // Prev and next walk the pinned cards in the order they sit on the tab.
  function step(delta) {
    var c = cur;
    if (!c) return;
    var list = cards();
    var i = indexOf(list, c.id) + delta;
    if (i < 0 || i >= list.length) return;
    c.id = list[i].getAttribute('data-dms-sheet');
    c.opener = list[i];
    load(list[i].getAttribute('data-sheet-url'));
  }

  function close() {
    var c = cur;
    if (!c) return;
    cur = null;
    window.removeEventListener('keydown', onEscape, true);
    document.removeEventListener('focusin', onFocusIn);
    if (c.screen) c.screen.inert = false;

    var done = function () {
      c.el.remove();
      // The panel may have been redrawn while the sheet was open; look the card
      // up again, and fall back to the tab itself.
      var back = null;
      var o = document.querySelector('.dms-overlay');
      if (o) {
        back = Array.prototype.filter.call(o.querySelectorAll('[data-dms-sheet]'), function (n) {
          return n.getAttribute('data-dms-sheet') === c.id;
        })[0] || o.querySelector('[data-dms-tab="adventures"]');
      }
      if (back && back.focus) back.focus({ preventScroll: true });
    };

    if (reduced() || !c.el.classList.contains('dms-sheet-open')) { done(); return; }
    c.el.classList.remove('dms-sheet-open');
    setTimeout(done, 260);
  }

  function onClick(e) {
    if (e.target.closest('[data-sheet-close]')) { close(); return; }
    if (e.target.closest('[data-sheet-prev]')) { step(-1); return; }
    if (e.target.closest('[data-sheet-next]')) { step(1); }
  }

  // Capture phase and stopImmediatePropagation: the screen's own Escape handler
  // must not also run, or one press would close both layers.
  function onEscape(e) {
    if (e.key !== 'Escape' || !cur) return;
    // A dialog or menu inside the page (an editor popup) gets its own Escape.
    if (e.defaultPrevented) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    close();
  }

  function onSheetKey(e) {
    if (e.key !== 'Tab' || !cur) return;
    var f = visibleFocusables(cur.el);
    if (!f.length) { e.preventDefault(); cur.panel.focus(); return; }
    var first = f[0], last = f[f.length - 1], a = document.activeElement;
    if (e.shiftKey && (a === first || a === cur.panel)) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && a === last) { e.preventDefault(); first.focus(); }
  }

  // Focus that lands outside the sheet (a click on the page behind) comes back.
  function onFocusIn(e) {
    if (cur && !cur.el.contains(e.target)) cur.panel.focus({ preventScroll: true });
  }

  // ---- pin picker -------------------------------------------------------

  function refreshScreen() {
    if (window.htmx) window.htmx.trigger(document.body, 'dms-refresh');
  }

  function pick(btn) {
    var box = btn.parentNode.querySelector('[data-dms-pick]');
    if (!box) return;
    if (!box.hidden) {
      box.hidden = true;
      btn.setAttribute('aria-expanded', 'false');
      btn.focus();
      return;
    }
    var listUrl = btn.getAttribute('data-url');
    var pinUrl = btn.getAttribute('data-pin-url');
    box.textContent = '';
    var input = document.createElement('input');
    input.type = 'search';
    input.className = 'dms-search';
    input.placeholder = 'Find an adventure page';
    input.setAttribute('aria-label', 'Find an adventure page');
    var list = document.createElement('div');
    list.className = 'dms-pick-list';
    list.setAttribute('role', 'listbox');
    list.setAttribute('aria-label', 'Adventure pages');
    box.appendChild(input);
    box.appendChild(list);
    box.hidden = false;
    btn.setAttribute('aria-expanded', 'true');
    input.focus();

    var seq = 0, timer = 0;

    function say(text) {
      list.textContent = '';
      var p = document.createElement('p');
      p.className = 'dms-small';
      p.textContent = text;
      list.appendChild(p);
    }

    function render(res) {
      if (!res.has_category) {
        say('This campaign has no Adventures category yet. The owner can add it in Customize, under Categories.');
        return;
      }
      if (!res.pages || !res.pages.length) { say('No adventure pages found.'); return; }
      list.textContent = '';
      res.pages.forEach(function (p) {
        var b = document.createElement('button');
        b.type = 'button';
        b.className = 'dms-pick-item';
        b.setAttribute('role', 'option');
        var n = document.createElement('span');
        n.textContent = p.name;
        var t = document.createElement('span');
        t.className = 'dms-small';
        t.textContent = p.pinned ? 'On the tab' : p.type_name;
        b.appendChild(n);
        b.appendChild(t);
        if (p.pinned) b.disabled = true;
        b.addEventListener('click', function () {
          b.disabled = true;
          C.apiFetch(pinUrl, { method: 'POST', body: { entity_id: p.id } }).then(function (r) {
            if (!r.ok) throw new Error('refused');
            refreshScreen();
          }).catch(function () {
            b.disabled = false;
            if (C.notify) C.notify('That page could not be pinned. Try again.', 'error');
          });
        });
        list.appendChild(b);
      });
    }

    function search() {
      var mine = ++seq;
      C.apiFetch(listUrl + '?q=' + encodeURIComponent(input.value.trim())).then(function (r) {
        if (!r.ok) throw new Error('refused');
        return r.json();
      }).then(function (res) {
        if (mine === seq) render(res);
      }).catch(function () {
        if (mine === seq) say('The list could not be loaded. Try again.');
      });
    }

    input.addEventListener('input', function () {
      clearTimeout(timer);
      timer = setTimeout(search, 200);
    });
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && !box.hidden) {
        e.preventDefault();
        e.stopPropagation();
        box.hidden = true;
        btn.setAttribute('aria-expanded', 'false');
        btn.focus();
      }
    });
    search();
  }

  C.dmsSheet = { open: open, pick: pick };
})();
