/**
 * header_motion.js -- the header's moving background (data-widget="header-motion").
 *
 * The server renders an over-wide strip of the owner's two colours inside the
 * header (app.templ, topbarMovingStyle); this widget slides it one bar-length
 * along its axis, forever, with a transform only. The slide's speed follows
 * MotionRest (static/js/motion_rest.js), the clock every looping animation
 * shares, so it eases to a stop when the person steps away, stops drawing
 * while still, and eases back when they return.
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

  // drive slides el and keeps its rate in step with MotionRest. The
  // Customize page's example header uses it too, so both move the same way.
  function drive(el, vertical) {
    var MR = window.MotionRest;
    if (!el.animate || !MR) return null;
    var anim = el.animate(
      vertical
        ? [{ transform: 'translateY(0)' }, { transform: 'translateY(-33.3333%)' }]
        : [{ transform: 'translateX(0)' }, { transform: 'translateX(-33.3333%)' }],
      { duration: LOOP_MS, iterations: Infinity, easing: 'linear' }
    );
    var raf = 0, gone = false;
    // A playback rate of 0 is a stop in disguise, so at rest it pauses instead.
    function tick() {
      raf = 0;
      if (gone) return;
      if (MR.still()) { anim.pause(); return; }
      anim.playbackRate = Math.max(0.02, MR.speed());
      if (anim.playState !== 'running') anim.play();
      raf = requestAnimationFrame(tick);
    }
    function wake() { if (!raf && !gone) raf = requestAnimationFrame(tick); }
    MR.onWake(wake);
    tick();
    return {
      destroy: function () {
        gone = true;
        if (raf) cancelAnimationFrame(raf);
        MR.offWake(wake);
        anim.cancel();
      }
    };
  }

  Chronicle.headerMotion = { drive: drive };

  Chronicle.register('header-motion', {
    init: function (el) {
      if (reduced()) return;
      el._headerMotion = drive(el, el.getAttribute('data-axis') === 'y');
    },
    destroy: function (el) {
      if (el._headerMotion) el._headerMotion.destroy();
      el._headerMotion = null;
    }
  });
})();
