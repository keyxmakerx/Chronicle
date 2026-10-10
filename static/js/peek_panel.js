/**
 * peek_panel.js -- the page peek: a read-only side panel for a linked page.
 *
 * Shift-click on a page link or mention, or the small peek icon that shows
 * beside one on hover, opens that page in a panel on the right (a bottom
 * sheet on a phone) without leaving the current page. One panel at a time:
 * another peek replaces its content. Close button, Escape, or "Open full
 * page" in the panel.
 *
 * The body comes from the server's /entities/:eid/peek fragment, which
 * applies the same permission and visibility checks as the full page, so
 * this file never decides who may see what. A page the viewer can't see
 * (or that is gone) gets one generic message that says nothing about it.
 *
 * Motion follows the UI standard: CSS owns it (.peek-panel in input.css), and
 * html[data-motion] calm/off already reduce it there. This file only waits
 * for the close transition when there is one.
 *
 * Exposes Chronicle.peek.open(anchorOrURL) / .close() / .isOpen().
 */
(function () {
  'use strict';

  var C = window.Chronicle = window.Chronicle || {};
  if (C.peek) return;

  var panel = null, bodyEl = null, closeBtn = null;
  var icon = null, iconFor = null, iconTimer = 0;
  var opener = null;     // link to return focus to on close
  var token = 0;         // an older answer never overwrites a newer peek
  var hideTimer = 0;
  var SELECTOR = 'a[data-entity-preview], a[data-mention-id]';

  function link(el) {
    return el && el.closest ? el.closest(SELECTOR) : null;
  }

  // The peek address for a link: the preview address with /peek in its place,
  // else the page address itself. Only same-site paths are ever fetched.
  function peekURL(a) {
    var p = a.getAttribute('data-entity-preview') || '';
    var u = '';
    if (/\/entities\/[^/?#]+\/preview$/.test(p)) {
      u = p.replace(/\/preview$/, '/peek');
    } else {
      var h = (a.getAttribute('href') || '').split(/[?#]/)[0];
      if (/^\/campaigns\/[^/]+\/entities\/[^/]+$/.test(h)) u = h + '/peek';
    }
    return /^\/(?!\/)/.test(u) ? u : '';
  }

  function build() {
    if (panel && panel.isConnected) return;
    panel = document.createElement('aside');
    panel.className = 'peek-panel';
    panel.id = 'peek-panel';
    panel.hidden = true;
    panel.setAttribute('role', 'complementary');
    panel.setAttribute('aria-label', 'Page peek');
    panel.tabIndex = -1;

    var bar = document.createElement('div');
    bar.className = 'peek-bar';
    var label = document.createElement('span');
    label.className = 'peek-bar__label';
    label.textContent = 'Peek';
    closeBtn = document.createElement('button');
    closeBtn.type = 'button';
    closeBtn.className = 'peek-close';
    closeBtn.setAttribute('aria-label', 'Close peek');
    closeBtn.innerHTML = '<i class="fa-solid fa-xmark" aria-hidden="true"></i>';
    closeBtn.addEventListener('click', function () { close(true); });
    bar.appendChild(label);
    bar.appendChild(closeBtn);

    bodyEl = document.createElement('div');
    bodyEl.className = 'peek-content';
    bodyEl.setAttribute('aria-live', 'polite');

    panel.appendChild(bar);
    panel.appendChild(bodyEl);
    document.body.appendChild(panel);
  }

  function message(text) {
    var p = document.createElement('p');
    p.className = 'peek-empty';
    p.textContent = text;
    bodyEl.textContent = '';
    bodyEl.appendChild(p);
  }

  function show() {
    clearTimeout(hideTimer);
    panel.hidden = false;
    void panel.offsetWidth; // so the first frame is the closed state
    panel.classList.add('is-open');
  }

  function open(target) {
    var a = typeof target === 'string' ? null : target;
    var url = typeof target === 'string' ? target : (a ? peekURL(a) : '');
    if (!url || !/^\/(?!\/)/.test(url)) return false;
    build();
    if (a) opener = a;
    var mine = ++token;
    var wasOpen = panel.classList.contains('is-open');
    bodyEl.setAttribute('aria-busy', 'true');
    if (!wasOpen) message('Loading…');
    show();
    if (!wasOpen) panel.focus({ preventScroll: true });

    fetch(url, { credentials: 'same-origin', headers: { 'Accept': 'text/html' } })
      .then(function (res) {
        if (!res.ok) throw new Error('peek ' + res.status);
        return res.text();
      })
      .then(function (html) {
        if (mine !== token) return;
        bodyEl.innerHTML = html;
        bodyEl.scrollTop = 0;
      })
      .catch(function () {
        if (mine !== token) return;
        message('This page isn’t available.');
      })
      .then(function () { if (mine === token) bodyEl.removeAttribute('aria-busy'); });
    return true;
  }

  function close(restoreFocus) {
    if (!panel || !panel.isConnected || !panel.classList.contains('is-open')) return;
    token++;
    panel.classList.remove('is-open');
    // Off (and a device that wants no motion) transitions in ~0ms: hide now.
    var ms = parseFloat(getComputedStyle(panel).transitionDuration) || 0;
    clearTimeout(hideTimer);
    hideTimer = setTimeout(function () { if (!panel.classList.contains('is-open')) panel.hidden = true; }, ms * 1000 + 40);
    if (restoreFocus && opener && opener.isConnected && opener.focus) opener.focus();
  }

  function isOpen() {
    return !!(panel && panel.isConnected && panel.classList.contains('is-open'));
  }

  // ---- The peek icon beside a link ----

  function ensureIcon() {
    if (icon && icon.isConnected) return icon;
    icon = document.createElement('button');
    icon.type = 'button';
    icon.className = 'peek-icon';
    icon.hidden = true;
    icon.setAttribute('aria-label', 'Peek at this page');
    icon.title = 'Peek at this page';
    icon.innerHTML = '<i class="fa-solid fa-up-right-and-down-left-from-center" aria-hidden="true"></i>';
    icon.addEventListener('pointerenter', function () { clearTimeout(iconTimer); });
    icon.addEventListener('pointerleave', hideIconSoon);
    icon.addEventListener('click', function (e) {
      e.preventDefault();
      e.stopPropagation();
      var a = iconFor;
      hideIcon();
      if (a) open(a);
    });
    document.body.appendChild(icon);
    return icon;
  }

  function showIcon(a) {
    if (!peekURL(a)) return;
    clearTimeout(iconTimer);
    ensureIcon();
    iconFor = a;
    var r = a.getBoundingClientRect();
    var size = 20;
    var left = Math.min(r.right + 2, window.innerWidth - size - 4);
    icon.style.left = Math.max(4, Math.round(left)) + 'px';
    icon.style.top = Math.round(r.top + (r.height - size) / 2) + 'px';
    icon.hidden = false;
  }

  function hideIcon() {
    clearTimeout(iconTimer);
    if (!icon) return;
    icon.hidden = true;
    iconFor = null;
  }

  function hideIconSoon() {
    clearTimeout(iconTimer);
    iconTimer = setTimeout(hideIcon, 200);
  }

  // ---- Wiring (this file loads once with the layout, so document-level
  // listeners are safe; nothing here lives inside a swapped fragment) ----

  document.addEventListener('mouseover', function (e) {
    if (icon && (e.target === icon || icon.contains(e.target))) return;
    var a = link(e.target);
    if (a) showIcon(a);
  });
  document.addEventListener('mouseout', function (e) {
    if (link(e.target) && !(e.relatedTarget && e.relatedTarget === icon)) hideIconSoon();
  });
  document.addEventListener('scroll', hideIcon, true);

  // Shift-click opens the peek instead of following the link. In an editor
  // you are typing in, Shift-click selects text, so it is left alone there.
  document.addEventListener('click', function (e) {
    if (!e.shiftKey || e.ctrlKey || e.metaKey || e.altKey || e.button !== 0) return;
    var a = link(e.target);
    if (!a || a.closest('[contenteditable="true"]') || !peekURL(a)) return;
    e.preventDefault();
    e.stopPropagation();
    open(a);
  }, true);

  // Escape closes; Shift+Enter on a focused link peeks, for the keyboard.
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && isOpen() && !e.defaultPrevented) {
      close(true);
      return;
    }
    if (e.key === 'Enter' && e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
      var a = link(document.activeElement);
      if (a && !a.closest('[contenteditable="true"]') && peekURL(a)) {
        e.preventDefault();
        open(a);
      }
    }
  });

  // A page change (boosted navigation or history) leaves the old peek behind.
  ['htmx:pushedIntoHistory', 'htmx:historyRestore'].forEach(function (n) {
    document.addEventListener(n, function () { close(false); hideIcon(); });
  });

  C.peek = { open: open, close: function () { close(true); }, isOpen: isOpen };
})();
