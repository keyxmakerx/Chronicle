/**
 * signin_scene.js -- the scene behind the sign-in tabs (data-widget="signin-scene").
 *
 * The stage (site_doors.templ) holds all three scenes and shows the one named
 * by its data-door. A tab click sets data-door at once so the scenes crossfade
 * while the form swaps in; after the swap, and on browser back, the open tab
 * in the new markup says which door is open. The drift itself is CSS; this
 * only pauses it, on the shared rest clock (MotionRest), when the viewer is
 * away, the tab is hidden or the window is offscreen.
 */
(function () {
  'use strict';

  function doorOf(tab) { return tab && tab.getAttribute('data-door'); }

  Chronicle.register('signin-scene', {
    init: function (el) {
      var MR = window.MotionRest;
      var hidden = !!document.hidden;

      function set(door) { if (door) el.setAttribute('data-door', door); }

      // A tab click: crossfade now, the swap follows.
      function onClick(e) {
        var tab = e.target.closest && e.target.closest('[data-door]');
        if (tab && el.contains(tab) && tab.hasAttribute('role')) set(doorOf(tab));
      }

      // After the panel swap, trust what the server rendered as open.
      function onSettled() {
        var open = el.querySelector('#doors-tabs [aria-selected="true"]');
        if (open) set(doorOf(open));
      }

      function sync() {
        el.classList.toggle('is-still', hidden || !!(MR && MR.still()));
      }
      function onVis() { hidden = !!document.hidden; sync(); }

      el.addEventListener('click', onClick);
      document.body.addEventListener('htmx:afterSettle', onSettled);
      document.addEventListener('visibilitychange', onVis);
      if (MR) MR.onWake(sync);
      var poll = MR ? setInterval(sync, 1000) : 0;
      sync();

      el._signinScene = function () {
        el.removeEventListener('click', onClick);
        document.body.removeEventListener('htmx:afterSettle', onSettled);
        document.removeEventListener('visibilitychange', onVis);
        if (MR) MR.offWake(sync);
        if (poll) clearInterval(poll);
      };
    },
    destroy: function (el) {
      if (el._signinScene) el._signinScene();
      el._signinScene = null;
    }
  });
})();
