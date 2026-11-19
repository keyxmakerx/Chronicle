/**
 * manage_pin.js -- the owner's pin for a Manage tool (layouts.ManagePin)
 *
 * A pin on a desk line or beside a page's title adds that tool to the owner's
 * own Pinned group in the sidebar, or takes it out. The pin is personal: it is
 * stored in the owner's own membership row, through the same
 * PUT /campaigns/:id/nav-pins every member's pin uses, and nobody else sees
 * it. The sidebar's own flow (static/js/sidebar_editor.js setPersonalPin) does
 * the saving, the redraw and the glide into place; this only hands the press
 * over and keeps the page's buttons in step.
 *
 * The control's inline handler loads this file on first use, because the desk
 * and the frame arrive by boosted swaps where a script tag may never run.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});

  // One press at a time per button: the sidebar refuses a second save while
  // one is in flight, and a double press must not read as pin then unpin.
  function toggle(btn) {
    if (!btn || btn.getAttribute('aria-busy') === 'true') return Promise.resolve(false);
    var ed = C.navEditor;
    if (!ed || !ed.setPersonalPin) {
      if (C.notify) C.notify('The sidebar is not ready yet. Try again in a moment.', 'error');
      return Promise.resolve(false);
    }
    var key = btn.getAttribute('data-manage-pin');
    var name = btn.getAttribute('data-pin-name') || '';
    var on = btn.getAttribute('aria-pressed') !== 'true';
    btn.setAttribute('aria-busy', 'true');
    return ed.setPersonalPin(key, on, name, { keepFocus: true }).then(function (ok) {
      btn.removeAttribute('aria-busy');
      if (ok && ed.syncManagePins) ed.syncManagePins();
      return ok;
    }, function () {
      btn.removeAttribute('aria-busy');
      return false;
    });
  }

  C.managePin = { toggle: toggle };
})();
