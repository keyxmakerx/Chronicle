/**
 * era_pages.js -- opens and shuts the Eras panel on entity pages
 * (Chronicle.EraPages).
 *
 * The server draws the panel (internal/plugins/entities/era_panel.templ) and
 * every era choice in it is a link; this file only shows and hides it. The
 * panel grows out of the Eras button and shrinks back on close (the Grow
 * move; Calm and Off reduce it in input.css). Closing the panel while an era
 * is picked goes back to the page as it is now.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var E = (C.EraPages = C.EraPages || {});

  function fold() { return document.getElementById('page-eras'); }
  function button() { return document.querySelector('[data-eras-btn]'); }

  function onKey(e) {
    if (e.key === 'Escape') E.close();
  }

  function open(f) {
    f.hidden = false;
    f.classList.remove('is-closing');
    void f.offsetWidth;
    f.classList.add('is-open');
    var b = button();
    if (b) b.setAttribute('aria-expanded', 'true');
    document.addEventListener('keydown', onKey);
  }

  // Back to the page as it is now: the same page with no era picked,
  // swapped in like the panel's own links.
  function backToNow() {
    var url = location.pathname;
    if (window.htmx) {
      window.htmx.ajax('GET', url, { target: '#main-content', select: '#main-content', swap: 'innerHTML show:none' })
        .then(function () { try { history.pushState({}, '', url); } catch (e) { /* stays on the old address */ } });
    } else {
      location.search = '';
    }
  }

  E.toggle = function () {
    var f = fold();
    if (!f) return;
    if (f.hidden || !f.classList.contains('is-open')) open(f);
    else E.close();
  };

  E.close = function () {
    var f = fold(), b = button();
    if (!f) return;
    document.removeEventListener('keydown', onKey);
    if (b) {
      b.setAttribute('aria-expanded', 'false');
      b.focus();
    }
    // An era picked in the panel is in the address; leaving the panel
    // leaves that era too.
    if (/[?&]era=/.test(location.search)) {
      backToNow();
      return;
    }
    f.classList.remove('is-open');
    f.classList.add('is-closing');
    setTimeout(function () {
      if (f.classList.contains('is-closing')) {
        f.hidden = true;
        f.classList.remove('is-closing');
      }
    }, 130);
  };

  // The panel's mount: one drawn open (an era is picked) closes on Escape.
  Chronicle.register('era-pages', {
    init: function (el) {
      if (!el.hidden) document.addEventListener('keydown', onKey);
    },
    destroy: function () {
      document.removeEventListener('keydown', onKey);
    }
  });
})();
