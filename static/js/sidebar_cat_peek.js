/**
 * sidebar_cat_peek.js -- a category's hover peek in the campaign sidebar
 *
 * Resting a mouse on a category row for PEEK_MS shows its five most
 * recently changed pages in a card that starts at the pointer, read from the
 * row's data-nav-peek URL. Touch and keyboard never see it: a tap or Enter
 * opens the drill panel, which leads with the category's own page.
 * sidebar_drill.js loads this file the first time a mouse reaches a category.
 */
(function () {
  'use strict';

  var PEEK_MS = 600, PEEK_GRACE = 300;
  var peekEl = null, peekRow = null, peekTimer = null, hideTimer = null, peekCache = {};

  function peekHide() {
    clearTimeout(peekTimer);
    clearTimeout(hideTimer);
    peekDrop();
  }

  function peekLink(href, text, cls) {
    var a = document.createElement('a');
    a.className = cls;
    a.href = href;
    a.textContent = text;
    return a;
  }

  function peekShow(row, x, items) {
    peekHide();
    var menu = window.Chronicle && Chronicle.navRowMenu;
    var home = menu ? menu.sitePath(row.getAttribute('data-nav-home')) : '';
    var name = (row.querySelector('.nav-lb') || row).textContent.trim();
    var card = document.createElement('div');
    card.className = 'nav-peek';
    card.setAttribute('role', 'group');
    card.setAttribute('aria-label', name + ': recently changed');
    var h = document.createElement('div');
    h.className = 'nav-peek-h';
    h.textContent = items.length ? 'Recently changed' : 'No pages yet';
    card.appendChild(h);
    items.forEach(function (it) {
      var href = menu ? menu.sitePath(it.url) : '';
      if (href) card.appendChild(peekLink(href, it.name, 'nav-peek-i'));
    });
    if (home) card.appendChild(peekLink(home, 'Open ' + name + ' page \u203a', 'nav-peek-open'));
    card.addEventListener('mouseenter', function () { clearTimeout(hideTimer); });
    card.addEventListener('mouseleave', peekLeave);
    document.body.appendChild(card);
    var r = row.getBoundingClientRect(), w = card.offsetWidth, hh = card.offsetHeight;
    card.style.left = Math.max(8, Math.min(x + 10, window.innerWidth - w - 8)) + 'px';
    card.style.top = Math.max(8, Math.min(r.top, window.innerHeight - hh - 8)) + 'px';
    peekEl = card;
    peekRow = row;
  }

  function peekDrop() {
    if (peekEl) peekEl.remove();
    peekEl = peekRow = null;
  }

  function peekLeave() {
    clearTimeout(peekTimer);
    clearTimeout(hideTimer);
    hideTimer = setTimeout(peekDrop, PEEK_GRACE);
  }

  function peekFetch(url) {
    if (!peekCache[url] || Date.now() - peekCache[url].at > 60000) {
      peekCache[url] = { at: Date.now(), p: fetch(url, { headers: { Accept: 'application/json' }, credentials: 'same-origin' })
        .then(function (res) { if (!res.ok) throw new Error(res.status); return res.json(); })
        .then(function (d) { return (d && d.results) || []; })
        .catch(function (err) { delete peekCache[url]; throw err; }) };
    }
    return peekCache[url].p;
  }

  document.addEventListener('pointerover', function (e) {
    if (e.pointerType !== 'mouse' || !e.target.closest) return;
    var row = e.target.closest('#sidebar-nav-list .nav-row[data-nav-peek]');
    if (!row) {
      if (peekRow && !(peekEl && peekEl.contains(e.target))) peekLeave();
      return;
    }
    if (row === peekRow) { clearTimeout(hideTimer); return; }
    // Crossing another row on the way to the card must not drop it at once:
    // it goes after the grace unless the pointer reaches it first.
    if (peekEl) peekLeave();
    clearTimeout(peekTimer);
    if (document.body.classList.contains('nav-editing') || row.closest('.sidebar-collapsed')) return;
    var x = e.clientX, url = row.getAttribute('data-nav-peek');
    peekTimer = setTimeout(function () {
      if (!row.matches(':hover')) return;
      peekFetch(url).then(function (items) {
        if (row.matches(':hover')) peekShow(row, x, items);
      }, function () { /* the peek is a convenience; the click still works */ });
    }, PEEK_MS);
  });
  document.addEventListener('pointerdown', function (e) {
    if (peekEl && !peekEl.contains(e.target)) peekHide();
  }, true);
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape' && peekEl) peekHide(); });
  document.addEventListener('scroll', function () { if (peekEl) peekHide(); }, true);
  document.addEventListener('htmx:beforeSwap', peekHide);

  // The pointer that loaded this file is already resting on a category.
  var first = document.querySelector('#sidebar-nav-list .nav-row[data-nav-peek]:hover');
  if (first) first.dispatchEvent(new PointerEvent('pointerover', { bubbles: true, pointerType: 'mouse', clientX: first.getBoundingClientRect().right - 40 }));
})();
