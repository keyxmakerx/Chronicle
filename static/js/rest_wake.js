/**
 * rest_wake.js -- Chronicle.restWake: ease a looping effect to a stop when
 * nobody is using the page, and back up when they return.
 *
 * Looping effects (the header's moving background, later others) cost power
 * and distract once the person has stepped away. restWake owns the "should it
 * be moving" decision so each effect only maps a level to its own drawing:
 *
 *   level 1  awake: the effect runs at full speed
 *   level 0  resting: the effect is stopped and draws nothing
 *
 * The level eases down over downMs (~2 s) and up over upMs (~1 s). It rests
 * when the tab is hidden, the window loses focus, the pointer leaves the
 * window, or idleMs (8 s) pass with no input; any pointer, key, wheel or
 * touch input wakes it. Under prefers-reduced-motion (or whatever
 * opts.reduced reports, such as the campaign's own reduce switch) the helper
 * stays inert: no listeners, no frames, level always 0.
 *
 *   var rw = Chronicle.restWake.create({
 *     reduced: function () { return false; },
 *     onLevel: function (level) { ... },   // each frame while easing (0..1, smoothed)
 *     onState: function (state) { ... }    // 'active' when level leaves 0, 'resting' when it returns
 *   });
 *   rw.destroy();
 *
 * Effects that keep their own frame loop start it on 'active' and stop it on
 * 'resting', reading rw.level() each frame. Effects driven by the Web
 * Animations API set playbackRate from onLevel and pause on 'resting'.
 */
(function () {
  'use strict';

  window.Chronicle = window.Chronicle || {};

  var INPUTS = ['pointermove', 'pointerdown', 'keydown', 'wheel', 'touchstart'];
  // Input events fire many times a second; the idle timer is re-checked
  // lazily instead of being re-armed on each one.
  var INPUT_NOTE_MS = 250;

  // Smoothstep: eases both ends so the effect neither jerks to a halt nor
  // lurches back to speed.
  function smooth(m) { return m * m * (3 - 2 * m); }

  function nowMs() {
    return (window.performance && window.performance.now) ? window.performance.now() : Date.now();
  }

  function create(opts) {
    opts = opts || {};
    var idleMs = opts.idleMs > 0 ? opts.idleMs : 8000;
    var downMs = opts.downMs > 0 ? opts.downMs : 2000;
    var upMs = opts.upMs > 0 ? opts.upMs : 1000;

    function isReduced() { return typeof opts.reduced === 'function' ? !!opts.reduced() : !!opts.reduced; }

    // Reduced motion: never move, never listen.
    if (isReduced()) {
      return {
        level: function () { return 0; },
        isResting: function () { return true; },
        wake: function () {}, rest: function () {}, suppress: function () {}, destroy: function () {}
      };
    }

    var m = 0;            // linear progress 0..1; level() is its smoothed form
    var target = 0;
    var active = false;   // true from the moment the level leaves 0 until it returns
    var raf = 0, idleTimer = 0, lastFrame = 0, lastInput = 0, lastNote = 0, suppressUntil = 0;
    var destroyed = false;

    function level() { return smooth(m); }

    function setActive(v) {
      if (active === v) return;
      active = v;
      if (opts.onState) opts.onState(v ? 'active' : 'resting');
    }

    function frame(t) {
      raf = 0;
      if (destroyed) return;
      // A frame's timestamp is its start time, which can precede the clock
      // read that scheduled it; a negative step would push the level below 0
      // and read as "arrived at rest" the instant the effect woke.
      var dt = Math.max(0, Math.min(0.05, (t - lastFrame) / 1000));
      lastFrame = t;
      if (m < target) m = Math.min(target, m + dt / (upMs / 1000));
      else if (m > target) m = Math.max(target, m - dt / (downMs / 1000));
      if (opts.onLevel) opts.onLevel(level(), dt);
      if (m <= 0 && target <= 0) setActive(false);
      if (m !== target) raf = window.requestAnimationFrame(frame);
    }

    function setTarget(v) {
      target = v;
      if (v > 0) setActive(true);
      if (m !== target && !raf) {
        lastFrame = nowMs();
        raf = window.requestAnimationFrame(frame);
      }
    }

    function armIdle(ms) {
      window.clearTimeout(idleTimer);
      idleTimer = window.setTimeout(function () {
        idleTimer = 0;
        var quiet = nowMs() - lastInput;
        if (quiet >= idleMs) rest(); else armIdle(idleMs - quiet);
      }, ms);
    }

    function wake() {
      if (destroyed || isReduced() || nowMs() < suppressUntil) return;
      lastInput = nowMs();
      if (target !== 1) setTarget(1);
      if (!idleTimer) armIdle(idleMs);
    }

    function rest() {
      window.clearTimeout(idleTimer);
      idleTimer = 0;
      if (!destroyed) setTarget(0);
    }

    function onInput() {
      var t = nowMs();
      // A burst of moves counts as one: skip the work unless something
      // needs waking or the note is stale.
      if (target === 1 && idleTimer && t - lastNote < INPUT_NOTE_MS) return;
      lastNote = t;
      wake();
    }
    function onBlur() { rest(); }
    function onVisibility() { if (document.hidden) rest(); }
    function onLeave() { rest(); }

    INPUTS.forEach(function (type) { window.addEventListener(type, onInput, { passive: true }); });
    window.addEventListener('blur', onBlur);
    document.addEventListener('visibilitychange', onVisibility);
    var docEl = document.documentElement;
    if (docEl) docEl.addEventListener('mouseleave', onLeave);

    // A page opened in a background tab starts at rest; any other starts awake.
    if (!document.hidden) wake();

    return {
      level: level,
      isResting: function () { return m <= 0 && target <= 0; },
      wake: wake,
      rest: rest,
      // Ignores wake-ups for ms, so a demo can show the rest without the
      // click that triggered it waking the effect again.
      suppress: function (ms) { suppressUntil = nowMs() + ms; },
      destroy: function () {
        destroyed = true;
        window.cancelAnimationFrame(raf);
        window.clearTimeout(idleTimer);
        raf = 0; idleTimer = 0;
        INPUTS.forEach(function (type) { window.removeEventListener(type, onInput); });
        window.removeEventListener('blur', onBlur);
        document.removeEventListener('visibilitychange', onVisibility);
        if (docEl) docEl.removeEventListener('mouseleave', onLeave);
      }
    };
  }

  window.Chronicle.restWake = { create: create };
})();
