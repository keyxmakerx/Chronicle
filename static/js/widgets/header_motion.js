/**
 * header_motion.js -- the header's moving background (data-widget="header-motion").
 *
 * The server renders an over-wide strip of the owner's two colours inside the
 * header (app.templ, topbarMovingStyle); this widget slides it one bar-length
 * along its axis, forever, with a transform only. Chronicle.restWake (the
 * shared helper) eases the slide to a stop when the person steps away and back
 * up when they return, so the page draws nothing while it rests.
 *
 * It does nothing at all, leaving the strip still, under prefers-reduced-motion
 * or the campaign's own reduce switch (html[data-cz-reduce]).
 */
(function () {
  'use strict';

  var LOOP_MS = 36000; // one bar-length of drift; slow enough to read as ambient

  function reduced() {
    var mq = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)');
    return !!(mq && mq.matches) || document.documentElement.hasAttribute('data-cz-reduce');
  }

  Chronicle.register('header-motion', {
    init: function (el) {
      if (!el.animate || !Chronicle.restWake) return;
      var vertical = el.getAttribute('data-axis') === 'y';
      var anim = el.animate(
        vertical
          ? [{ transform: 'translateY(0)' }, { transform: 'translateY(-33.3333%)' }]
          : [{ transform: 'translateX(0)' }, { transform: 'translateX(-33.3333%)' }],
        { duration: LOOP_MS, iterations: Infinity, easing: 'linear' }
      );
      // Held until the helper decides: reduced motion leaves it here for good.
      anim.pause();
      var rw = null; // not yet assigned when the helper's first wake fires
      rw = Chronicle.restWake.create({
        reduced: reduced,
        // A playback rate of exactly 0 is a stop in disguise; the helper
        // pauses at rest, so only positive rates are ever set.
        onLevel: function (level) { if (level > 0) anim.playbackRate = level; },
        onState: function (state) {
          if (state === 'active') {
            anim.playbackRate = Math.max(0.02, rw ? rw.level() : 0);
            anim.play();
          } else {
            anim.pause();
          }
        }
      });
      el._headerMotion = { anim: anim, rw: rw };
    },
    destroy: function (el) {
      var h = el._headerMotion;
      if (!h) return;
      h.rw.destroy();
      h.anim.cancel();
      el._headerMotion = null;
    }
  });
})();
