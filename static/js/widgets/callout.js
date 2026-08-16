/*
 * callout.js — the player call-to-action banner (C-RSVP-P10).
 *
 * The banner itself is server-rendered and refreshed by an HTMX poll declared
 * on the host element (app.templ). This widget owns the two things the server
 * genuinely cannot do:
 *
 *   1. THE BROWSER'S TIMEZONE. The server does not know it and must not guess
 *      one into the sentence, so the timezone banner ships with a hidden accept
 *      button and this fills in the real zone and reveals it. If the browser
 *      will not tell us, the button stays hidden and the "Choose" link to
 *      /account is the whole offer — an honest degrade, not a broken control.
 *
 *   2. DISMISSAL. There is no user-preferences table in this product (one has
 *      been formally refused three times), so dismissal is client-side and
 *      lasts for the tab session. That is a deliberate choice and not a
 *      shortcut: an unset timezone is wrong EVERY day, so a permanent dismissal
 *      would let a player silently keep reading UTC times forever. Per tab is
 *      long enough to stop it nagging and short enough that it comes back.
 *
 * ES5 style to match the rest of static/js. Registered via Chronicle.register
 * and auto-mounted by boot.js on [data-widget="callout"], which also re-mounts
 * it after every htmx settle — the mechanism that exists precisely because
 * script tags in swapped fragments are removed.
 */
(function () {
  'use strict';

  // dismissKey is per-KIND, so dismissing the timezone ask does not also
  // swallow an RSVP that arrives ten minutes later. Getting this wrong is the
  // failure mode that makes a banner untrustworthy.
  function dismissKey(kind) { return 'chronicle:cta-dismissed:' + kind; }

  function isDismissed(kind) {
    try { return window.sessionStorage.getItem(dismissKey(kind)) === '1'; }
    catch (e) { return false; } // private mode / storage disabled: show it
  }

  function setDismissed(kind) {
    try { window.sessionStorage.setItem(dismissKey(kind), '1'); }
    catch (e) { /* non-fatal — the banner simply returns on the next poll */ }
  }

  // browserZone returns the IANA zone the browser reports, or '' when it will
  // not say. Wrapped because Intl is absent in some embedded webviews and a
  // throw here would take the whole widget down with it.
  function browserZone() {
    try {
      var tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
      return typeof tz === 'string' ? tz : '';
    } catch (e) { return ''; }
  }

  function wire(host) {
    var bar = host.querySelector('[data-cta]');
    if (!bar) return;
    var kind = bar.getAttribute('data-cta') || '';

    // A dismissed banner is removed rather than hidden: the poll re-renders the
    // host's innerHTML wholesale, so leaving a hidden node would just be
    // replaced anyway, and removing it keeps the DOM honest about what is on
    // screen.
    if (isDismissed(kind)) { bar.parentNode.removeChild(bar); return; }

    var x = bar.querySelector('[data-cta-dismiss]');
    if (x) {
      x.addEventListener('click', function () {
        setDismissed(kind);
        if (bar.parentNode) bar.parentNode.removeChild(bar);
      });
    }

    if (kind !== 'timezone') return;

    var accept = bar.querySelector('[data-cta-tz-accept]');
    var slot = bar.querySelector('[data-cta-zone]');
    var zone = browserZone();
    // No zone from the browser means no accept button. The offer has to be a
    // real one — a button labelled "Use my timezone" that submits an empty
    // string would clear the field it claims to set.
    if (!accept || !zone) return;

    if (slot) slot.textContent = zone;
    accept.hidden = false;

    accept.addEventListener('click', function () {
      accept.disabled = true;
      Chronicle.apiFetch('/account/timezone', {
        method: 'PUT',
        body: { timezone: zone },
      }).then(function (r) {
        if (!r || !r.ok) {
          // THE BUTTON COMES BACK. A control that disables itself and then
          // says nothing has told the player it worked.
          accept.disabled = false;
          if (Chronicle.notify) Chronicle.notify('That could not be saved. Try Choose instead.', 'error');
          return;
        }
        if (Chronicle.notify) Chronicle.notify('Times will now show in ' + zone + '.', 'success');
        // Not dismissed — SATISFIED. The next poll will find the zone set and
        // send nothing, so the banner goes on its own; removing it here just
        // avoids the up-to-60-second wait.
        if (bar.parentNode) bar.parentNode.removeChild(bar);
      }).catch(function () {
        accept.disabled = false;
        if (Chronicle.notify) Chronicle.notify('That could not be saved. Try Choose instead.', 'error');
      });
    });
  }

  Chronicle.register('callout', {
    init: function (el) {
      // The host starts empty and is filled by the poll, so wiring has to
      // happen on every settle rather than once at mount. htmx fires
      // htmx:afterSwap on the host element itself for its own hx-get.
      wire(el);
      el.addEventListener('htmx:afterSwap', function () { wire(el); });
    },
  });
})();
