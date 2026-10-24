/*
 * callout.js — the player call-to-action banner.
 *
 * The banner is server-rendered and HTMX-polled (app.templ). This widget
 * only does what the server can't: (1) fill in the browser's real
 * timezone and reveal the hidden accept button — the server must never
 * guess a zone, so if the browser won't tell us the "Choose" link to
 * /account is the whole offer; (2) client-side dismissal, scoped to the
 * tab session (no prefs table exists) rather than permanent, since an
 * unset timezone is wrong every day and a permanent dismiss would hide
 * that forever.
 *
 * Auto-mounted by boot.js on [data-widget="callout"]; re-mounted after
 * every htmx settle since swapped-in script tags don't persist.
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

  // zoneLabel turns an IANA zone into words a player recognises:
  // "America/New_York" reads "New York time". The full zone stays in the
  // button's title for anyone who needs to tell two cities apart.
  function zoneLabel(zone) {
    var city = zone.split('/').pop().replace(/_/g, ' ');
    return city + ' time';
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

    var label = zoneLabel(zone);
    if (slot) slot.textContent = label;
    accept.title = zone;
    accept.hidden = false;
    // Guests get no "Choose" link (they cannot open /account), so the failure
    // toast only points at it when it is on screen.
    var failMsg = bar.querySelector('.cta-alt')
      ? 'That could not be saved. Try Choose instead.'
      : 'That could not be saved. Please try again.';

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
          if (Chronicle.notify) Chronicle.notify(failMsg, 'error');
          return;
        }
        if (Chronicle.notify) Chronicle.notify('Times will now show in ' + label + '.', 'success');
        // Not dismissed — SATISFIED. The next poll will find the zone set and
        // send nothing, so the banner goes on its own; removing it here just
        // avoids the up-to-60-second wait.
        if (bar.parentNode) bar.parentNode.removeChild(bar);
      }).catch(function () {
        accept.disabled = false;
        if (Chronicle.notify) Chronicle.notify(failMsg, 'error');
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
