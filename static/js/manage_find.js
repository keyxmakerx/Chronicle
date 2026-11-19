/**
 * manage_find.js -- Find on the owner's Manage pages (layouts.ManageFind)
 *
 * The Find word, or "/" outside a field (keyboard_shortcuts.js presses the
 * same button), opens a small field that finds a setting by name, including
 * ones inside a tool, and goes to it. The list comes from the server when Find
 * opens: it is built for the owner alone from the desk's own tool list, so it
 * never names a tool the campaign switched off. The field itself is the one
 * the Customizer uses (find_palette.js).
 *
 * The button's inline handler loads this file on first use, because the desk
 * and the frame arrive by boosted swaps where a script tag may never run. This
 * file loads the field's script and style the same way.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var opening = false;

  function loadScript(src) {
    if (C.findPalette) return Promise.resolve();
    return new Promise(function (resolve, reject) {
      var s = document.querySelector('script[data-find-palette-js]');
      if (!s) {
        s = document.createElement('script');
        s.src = src;
        s.setAttribute('data-find-palette-js', '');
        document.head.appendChild(s);
      }
      s.addEventListener('load', resolve);
      s.addEventListener('error', reject);
    });
  }

  function loadStyle(href) {
    if (!href || document.querySelector('link[data-find-palette-css]')) return;
    var l = document.createElement('link');
    l.rel = 'stylesheet';
    l.href = href;
    l.setAttribute('data-find-palette-css', '');
    document.head.appendChild(l);
  }

  function fail() {
    if (C.notify) C.notify('Find could not load. Try again in a moment.', 'error');
  }

  function open(btn) {
    if (!btn || opening) return Promise.resolve(false);
    if (C.findPalette && C.findPalette.isOpen()) return Promise.resolve(false);
    opening = true;
    btn.setAttribute('aria-expanded', 'true');
    loadStyle(btn.getAttribute('data-find-css'));
    var url = btn.getAttribute('data-find-url');
    return Promise.all([
      loadScript(btn.getAttribute('data-find-js')),
      fetch(url, { credentials: 'same-origin', headers: { Accept: 'application/json' } }).then(function (res) {
        if (!res.ok) throw new Error('find ' + res.status);
        return res.json();
      })
    ]).then(function (parts) {
      var entries = (parts[1] && parts[1].entries) || [];
      C.findPalette.open({
        finds: entries,
        label: 'Find a setting',
        placeholder: 'Find a setting, like “invite” or “Trash”',
        onClose: function () { btn.setAttribute('aria-expanded', 'false'); }
      });
      return true;
    }).catch(function () {
      btn.setAttribute('aria-expanded', 'false');
      fail();
      return false;
    }).then(function (ok) { opening = false; return ok; });
  }

  C.manageFind = { open: open };
})();
