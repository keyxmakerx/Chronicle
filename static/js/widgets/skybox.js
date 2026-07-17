// skybox.js — the Skybox widget (C-SKYBOX-WIDGET). Mounts on any
// [data-widget="skybox"] element (boot.js auto-mount) and renders the
// ambient sky (starfield, sun, moons, weather + celestial particles — no
// hourglass) by REUSING the shared engine (static/js/cal-almanac.js), not
// rebuilding it. Sky-only sibling of the `worldstate` widget
// (static/js/widgets/worldstate.js), which is sky + hourglass.
//
// Data flow: the widget reads the campaign worldState through the SAME
// provider singleton (worldstate_provider.js) the worldstate widget uses,
// so N sky/worldstate widgets on one page still share ONE fetch. It adopts
// a server-embedded seed (any [data-cal-worldstate] element on the page —
// either its own, when ElementID was set server-side, or the almanac
// band's/entity embed's pre-existing one) when present (zero fetch), and
// drives the engine's live re-render via window.__calSetWorldState on every
// provider update. Clean destroy() unsubscribes + drops no timers (this
// widget never starts one — see .ai.md).
//
// data attributes:
//   data-campaign-id — required; which campaign's worldState to read
(function () {
  'use strict';
  if (!window.Chronicle || typeof window.Chronicle.register !== 'function') return;

  function parseSeed() {
    var el = document.querySelector('[data-cal-worldstate]');
    if (!el) return null;
    try { return JSON.parse(el.getAttribute('data-cal-worldstate') || 'null'); }
    catch (e) { return null; }
  }

  // Drive the shared engine from a seed. The engine paints the first frame
  // from the embedded blob on its own init; subsequent provider updates
  // patch the live worldState (no-op until the engine has booted — the
  // first paint already came from the blob, so this is never user-visible).
  function applySeed(seed) {
    if (!seed) return;
    try {
      if (window.__calSetWorldState && window.__calWorldState) {
        window.__calSetWorldState(seed);
      }
    } catch (e) { /* engine not ready yet — first paint already came from the blob */ }
  }

  function showError(el) {
    var err = el.querySelector('[data-skybox-error]');
    if (err) { err.classList.remove('hidden'); err.classList.add('flex'); }
    el.removeAttribute('data-skybox-loading');
  }

  window.Chronicle.register('skybox', {
    init: function (el) {
      if (!window.ChronicleWorldState) { showError(el); return; }
      var campaignId = el.getAttribute('data-campaign-id') || '';
      var provider = window.ChronicleWorldState.get(campaignId);
      var serverSeed = parseSeed();

      var unsub = provider.subscribe(function (seed) {
        applySeed(seed);
        el.removeAttribute('data-skybox-loading');
      });
      var unsubErr = provider.onError(function () { showError(el); });

      // Adopt the server seed (zero fetch) when present; otherwise the
      // provider fetches once for the whole page (shared with any other
      // worldstate/skybox widget already on it).
      provider.load(serverSeed ? { seed: serverSeed } : {}).catch(function () { showError(el); });

      el._skybox = { unsub: unsub, unsubErr: unsubErr };
    },
    destroy: function (el) {
      var s = el._skybox;
      if (s) {
        if (s.unsub) s.unsub();
        if (s.unsubErr) s.unsubErr();
        el._skybox = null;
      }
    },
  });
})();
