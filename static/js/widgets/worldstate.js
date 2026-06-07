// worldstate.js — the worldState widget (C-CAL-WORLDSTATE-WIDGETS). Mounts on
// any [data-widget="worldstate"] element (boot.js auto-mount) and renders the
// ambient worldState — sky band + hourglass-on-shelf ("the mini shelf view") —
// by REUSING the shared engine (static/js/cal-almanac.js), not rebuilding it.
//
// Data flow: the widget reads the campaign worldState through the provider
// singleton (worldstate_provider.js) so N widgets on a page share ONE fetch.
// It adopts a server-embedded seed (#cal-v2-worldstate) when present (zero
// fetch), and drives the engine's live re-render via window.__calSetWorldState
// on every provider update. Clean destroy() unsubscribes + drops listeners.
//
// data attributes:
//   data-campaign-id  — required; which campaign's worldState to read
//   data-variant      — "hourglass" | "sky-band" | "full" (default "full");
//                       currently informational (the server scaffold decides
//                       which surfaces are present); the widget drives whatever
//                       engine surfaces the scaffold rendered.
(function () {
  'use strict';
  if (!window.Chronicle || typeof window.Chronicle.register !== 'function') return;

  function parseSeed(el) {
    if (!el) return null;
    try { return JSON.parse(el.getAttribute('data-cal-worldstate') || 'null'); }
    catch (e) { return null; }
  }

  // Drive the shared engine from a seed. The engine paints the first frame
  // from the #cal-v2-worldstate blob on its own init; subsequent provider
  // updates patch the live worldState (no-op until the engine has booted).
  function applySeed(seed) {
    if (!seed) return;
    try {
      if (window.__calSetWorldState && window.__calWorldState) {
        window.__calSetWorldState(seed);
      }
    } catch (e) { /* engine not ready yet — first paint already came from the blob */ }
  }

  function showError(el) {
    var err = el.querySelector('[data-worldstate-error]');
    if (err) { err.classList.remove('hidden'); err.classList.add('flex'); }
    el.removeAttribute('data-worldstate-loading');
  }

  window.Chronicle.register('worldstate', {
    init: function (el) {
      if (!window.ChronicleWorldState) { showError(el); return; }
      var campaignId = el.getAttribute('data-campaign-id') || '';
      var provider = window.ChronicleWorldState.get(campaignId);
      var serverSeed = parseSeed(document.getElementById('cal-v2-worldstate'));

      var unsub = provider.subscribe(function (seed) {
        applySeed(seed);
        el.removeAttribute('data-worldstate-loading');
      });
      var unsubErr = provider.onError(function () { showError(el); });

      // Adopt the server seed (zero fetch) when present; otherwise the
      // provider fetches once for the whole page.
      provider.load(serverSeed ? { seed: serverSeed } : {}).catch(function () { showError(el); });

      el._worldstate = { unsub: unsub, unsubErr: unsubErr };
    },
    destroy: function (el) {
      var s = el._worldstate;
      if (s) {
        if (s.unsub) s.unsub();
        if (s.unsubErr) s.unsubErr();
        el._worldstate = null;
      }
    },
  });
})();
