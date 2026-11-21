/**
 * new_page_sheet.js -- Create page as a small sheet over the place you are.
 *
 * Every place that starts Create page (New page on a list, the sidebar's plus,
 * "Create sub-page", Ctrl+N and quick search's "Create a page named ...") asks
 * Chronicle.newPage.open(href) with the address it would have followed. The
 * address is fetched with ?sheet=1, which answers with the form alone, and it
 * is shown in a native modal dialog with the app's shared dialog motion. The
 * form posts the way the full screen's does, so saving still lands on the new
 * page with the page editor open.
 *
 * open() answers false when it will not take the request (no dialog support,
 * the full form already on screen, an address that is not Create page), and
 * the caller then follows the link to the full screen as before.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var NEW_PATH = /^\/campaigns\/[^/]+\/entities\/new$/;
  var FORM_ID = 'form:entity-create';

  var dlg = null;
  var opener = null;
  var token = 0;

  function sheetUrl(href) {
    var u;
    try { u = new URL(href, window.location.href); } catch (e) { return null; }
    if (u.origin !== window.location.origin || !NEW_PATH.test(u.pathname)) return null;
    u.searchParams.set('sheet', '1');
    return u;
  }

  function teardown() {
    token++;
    if (!dlg) return;
    var d = dlg;
    dlg = null;
    if (C.markClean) C.markClean(FORM_ID);
    if (d.parentNode) d.parentNode.removeChild(d);
    var back = opener;
    opener = null;
    if (back && back.isConnected && back.focus) back.focus({ preventScroll: true });
  }

  // Escape and Cancel come here. A sheet with something typed asks first, in
  // place, with the shared slip; an empty one just closes.
  function requestClose() {
    if (!dlg) return;
    var box = dlg.querySelector('.np-sheet-in');
    if (box && C.askSlip && C.isDirty && C.isDirty()) {
      C.askSlip(box, {
        ask: 'Close without making the page?',
        detail: 'What you typed hasn’t been saved.',
        discard: 'Discard and close',
        onDiscard: function () { if (dlg) dlg.close(); }
      });
      return;
    }
    dlg.close();
  }

  function fill(d, html) {
    var box = d.querySelector('.np-sheet-in');
    box.innerHTML = html;
    box.removeAttribute('aria-busy');
    // The form arrived after the page's own scripts ran: let htmx wire its
    // post, the form's change tracking and the template picker find it.
    if (window.htmx && window.htmx.process) window.htmx.process(box);
    if (C.initFormTracking) C.initFormTracking(box);
    if (C.initTemplatePicker) C.initTemplatePicker();
    var name = box.querySelector('#name');
    if (name) name.focus({ preventScroll: true });
  }

  function open(href) {
    var u = sheetUrl(href);
    // The fallback goes to the address that was checked, never the raw one.
    var full = u ? u.pathname + u.search.replace(/([?&])sheet=1(&|$)/, '$1').replace(/[?&]$/, '') : '';
    if (!u || typeof HTMLDialogElement === 'undefined' || !HTMLDialogElement.prototype.showModal) return false;
    // The full form is on screen: its ids would clash with the sheet's.
    if (document.getElementById('entity-form')) return false;
    if (dlg) return true;

    opener = document.activeElement;
    var d = document.createElement('dialog');
    d.className = 'dlg-modal np-sheet';
    d.setAttribute('aria-labelledby', 'np-sheet-title');
    d.innerHTML = '<div class="np-sheet-in" aria-busy="true"></div>';
    d.addEventListener('cancel', function (e) { e.preventDefault(); requestClose(); });
    d.addEventListener('close', teardown);
    document.body.appendChild(d);
    dlg = d;
    d.showModal();

    var mine = ++token;
    fetch(u.toString(), { credentials: 'same-origin', headers: { 'X-Requested-With': 'XMLHttpRequest' } })
      .then(function (res) {
        if (!res.ok) throw new Error('new page ' + res.status);
        return res.text();
      })
      .then(function (html) { if (mine === token && dlg === d) fill(d, html); })
      .catch(function () {
        // The sheet could not be had: the full screen still can.
        if (mine !== token) return;
        teardown();
        window.location.assign(full);
      });
    return true;
  }

  C.newPage = { open: open, requestClose: requestClose };
})();
