/**
 * page_layout_chip.js: the chip beside a page's name that says which of its
 * category's layouts the page wears (page_layout_chip.templ). The chip is a
 * <details>, so it opens and closes without this file; this adds Escape, the
 * menu's Close and a click outside to close it, and puts focus back on the
 * chip, and opens the menu toward the page when the chip sits near the right
 * edge. Picking a layout is an htmx PUT the server checks; it reloads the page.
 */
(function () {
  'use strict';

  function close(d, focus) {
    if (!d.open) return;
    d.open = false;
    var s = d.querySelector('summary');
    if (focus && s) s.focus();
  }

  // The menu hangs from the chip's left edge unless that would run it off
  // the screen; then it hangs from the right edge instead.
  function place(d) {
    var m = d.querySelector('.page-layout-menu');
    if (!m) return;
    m.classList.remove('is-end');
    if (m.getBoundingClientRect().right > document.documentElement.clientWidth - 8) {
      m.classList.add('is-end');
    }
  }

  Chronicle.register('page-layout-chip', {
    init: function (el) {
      function onToggle() { if (el.open) place(el); }
      function onKey(e) { if (e.key === 'Escape') close(el, true); }
      function onDown(e) { if (el.open && !el.contains(e.target)) close(el, false); }
      function onClick(e) { if (e.target.closest('[data-page-layout-close]')) close(el, true); }
      document.addEventListener('keydown', onKey);
      document.addEventListener('pointerdown', onDown, true);
      el.addEventListener('click', onClick);
      el.addEventListener('toggle', onToggle);
      el._plcOff = function () {
        document.removeEventListener('keydown', onKey);
        document.removeEventListener('pointerdown', onDown, true);
        el.removeEventListener('click', onClick);
        el.removeEventListener('toggle', onToggle);
      };
    },
    destroy: function (el) {
      if (el._plcOff) el._plcOff();
      el._plcOff = null;
    }
  });
})();
