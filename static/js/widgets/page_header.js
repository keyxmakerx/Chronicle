/**
 * page_header.js -- the header of a page a game-system renderer owns
 * (Chronicle.PageHeader): the more menu, Delete's inline confirm, and the
 * Claim button.
 *
 * The server renders the header (internal/plugins/entities/page_header.templ);
 * every control calls in here through an inline handler built Go-side, so
 * nothing depends on a script inside a swapped fragment. The Edit name fold is
 * page_name_fold.js, which shares the Chronicle.PageHeader namespace.
 *
 * The outside-click and Escape listeners live only while the more menu is open.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var H = (C.PageHeader = C.PageHeader || {});

  var openMenu = null;   // {menu, btn}

  function reduced() {
    return matchMedia('(prefers-reduced-motion: reduce)').matches ||
      !!document.querySelector('.nav-rm, html.rm, html[data-cz-reduce], html[data-view-motion="calm"]');
  }
  function toast(msg) { if (H._toast) H._toast(msg); }

  // ---- the more menu ----

  function outside(e) {
    if (!openMenu) return;
    if (!openMenu.menu.isConnected) { H.closeMore(); return; }
    if (openMenu.menu.contains(e.target) || openMenu.btn.contains(e.target)) return;
    H.closeMore();
  }
  function onKey(e) {
    if (e.key !== 'Escape' || !openMenu) return;
    var b = openMenu.btn;
    H.closeMore();
    if (b && b.isConnected) b.focus();
  }
  // A menu action that loads something (History) closes the menu behind it.
  function onMenuClick(e) {
    var t = e.target.closest && e.target.closest('button[hx-get]');
    if (t) H.closeMore();
  }
  function listen(on) {
    var m = on ? 'addEventListener' : 'removeEventListener';
    document[m]('mousedown', outside, true);
    document[m]('keydown', onKey, true);
  }

  H.closeMore = function () {
    if (!openMenu) return;
    var o = openMenu;
    openMenu = null;
    listen(false);
    o.menu.hidden = true;
    o.menu.removeEventListener('click', onMenuClick);
    o.btn.setAttribute('aria-expanded', 'false');
    var ask = o.menu.querySelector('[data-del-ask]');
    if (ask) ask.hidden = true;
  };

  H.toggleMore = function (btn) {
    var menu = btn.parentNode.querySelector('[data-more-menu]');
    if (!menu) return;
    if (openMenu && openMenu.menu === menu) { H.closeMore(); return; }
    H.closeMore();
    menu.hidden = false;
    btn.setAttribute('aria-expanded', 'true');
    openMenu = { menu: menu, btn: btn };
    menu.addEventListener('click', onMenuClick);
    listen(true);
  };

  // The phone's menu entry opens the same fold the wide button does.
  H.nameFoldFromMenu = function (el) {
    H.closeMore();
    var trigger = document.querySelector('.ph-acts .ph-wide[data-edit-name]') || el;
    if (H.nameFold) H.nameFold(trigger);
  };

  // ---- Delete's inline confirm ----

  H.askDelete = function (el) {
    var ask = el.closest('form').querySelector('[data-del-ask]');
    if (!ask) return;
    ask.hidden = false;
    var keep = ask.querySelector('.btn-secondary');
    if (keep) keep.focus();
  };

  H.keepDelete = function (el) {
    var ask = el.closest('[data-del-ask]');
    if (ask) ask.hidden = true;
  };

  // ---- Claim ----

  function warn(on) {
    var w = document.querySelector('[data-claim-warn]');
    if (!w) return;
    w.classList.remove('is-on');
    if (on) { void w.offsetWidth; w.classList.add('is-on'); }
  }

  function claimDone(btn) {
    var holder = btn.closest('.ph-claim');
    var pill = document.createElement('span');
    pill.className = 'ph-pill is-yours ag-ring';
    pill.setAttribute('data-claim-pill', '');
    pill.innerHTML = '<i class="fa-solid fa-circle-check"></i> Yours';
    if (holder) holder.replaceWith(pill);
    toast('Claimed. ' + (btn.getAttribute('data-name') || 'This character') + ' is on your My Characters page.');
    // The page's own widget learns it is claimed from what the server draws,
    // so show the pill, then draw the page again.
    setTimeout(function () { window.location.reload(); }, reduced() ? 600 : 1300);
  }

  H.claim = function (el) {
    var btn = document.querySelector('[data-claim]');
    if (!btn || btn.disabled) return;
    warn(false);
    var label = btn.querySelector('[data-claim-label]');
    var before = label ? label.textContent : '';
    btn.disabled = true;
    btn.classList.add('is-loading');
    if (label) label.textContent = 'Claiming…';
    C.apiFetch(btn.getAttribute('data-url'), { method: 'POST' })
      .then(function (res) {
        if (!res.ok) throw new Error('claim');
        claimDone(btn);
      })
      .catch(function () {
        btn.disabled = false;
        btn.classList.remove('is-loading');
        if (label) label.textContent = before;
        warn(true);
      });
  };
})();
