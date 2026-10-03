/**
 * motion_rest.js — one clock for every looping animation on the page, so they
 * all come to rest together when the viewer steps away and wake together when
 * they come back.
 *
 * The viewer is away when the tab is hidden, the window loses focus, the
 * pointer leaves the window, or nothing has been touched for a while. Motion
 * then eases down to a standstill rather than stopping dead, and once still a
 * loop should stop asking for frames, to save battery. Any input eases it back.
 * One-shot transitions (a drawer opening, a fold) do not use this clock.
 *
 * A loop reads its time from MotionRest.now() instead of performance.now():
 * that clock runs at MotionRest.speed() (1 normally, 0 at rest, in between
 * while easing), so everything driven by it slows smoothly and resumes exactly
 * where it stopped. MotionRest.still() says when to stop requesting frames, and
 * MotionRest.onWake(fn) calls fn when motion starts again, to restart a loop.
 *
 * Under either reduce switch (the campaign's html[data-cz-reduce] or the
 * person's own html[data-view-motion="calm"]) the clock is held at rest for
 * good: speed 0 from the first frame, never waking, so every loop that reads
 * it stays still without needing its own check. Flipping the attribute live
 * (the My view card does) takes effect at once.
 */
(function () {
  'use strict';
  if (window.MotionRest) return;

  // How long without input counts as stepping away, and how long the easing takes each way.
  var IDLE_MS = 8000, DOWN_S = 2, UP_S = 1;

  // Either reduce switch holds everything still; read live so a toggle needs no reload.
  function calm() {
    var r = document.documentElement;
    return !!(r && r.hasAttribute && (r.hasAttribute('data-cz-reduce') || r.getAttribute('data-view-motion') === 'calm'));
  }

  // Starts at rest under a reduce switch, so nothing moves even for the first frames.
  var speed = calm() ? 0 : 1, from = speed, to = speed, easeT0 = 0, easeDur = 0;
  // The rest clock: its value at the last fold point and the real time it was taken.
  var base = 0, baseReal = now0(), raf = 0;
  var hidden = !!document.hidden, blurred = false, pointerOut = false, idle = false, forced = null;
  var idleTimer = 0, wakers = [];

  function now0() { return performance.now() / 1000; }
  function smooth(u) { u = Math.max(0, Math.min(1, u)); return u * u * (3 - 2 * u); }

  // The speed at real time r, and the rest time gained between two real times (the speed's integral).
  function speedAt(r) { return easeDur ? from + (to - from) * smooth((r - easeT0) / easeDur) : to; }
  function gained(r0, r1) {
    // Integrated in small steps: cheap, and exact enough for a clock nobody reads to the microsecond.
    var n = Math.max(1, Math.ceil((r1 - r0) / .02)), h = (r1 - r0) / n, s = 0;
    for (var i = 0; i < n; i++) s += speedAt(r0 + (i + .5) * h) * h;
    return s;
  }
  function fold(r) {
    base += gained(baseReal, r); baseReal = r;
    speed = speedAt(r);
    if (easeDur && r - easeT0 >= easeDur) { easeDur = 0; speed = to; from = to; }
  }
  function nowRest() { fold(now0()); return base; }

  function away() { return calm() || (forced != null ? forced : (hidden || blurred || pointerOut || idle)); }
  function update() {
    var r = now0(); fold(r);
    var target = away() ? 0 : 1;
    if (target === to) return;
    // A hidden tab draws nothing, so there is nothing to ease: it rests at once.
    if (target === 0 && hidden && forced == null) { from = to = speed = 0; easeDur = 0; return; }
    from = speed; to = target; easeT0 = r; easeDur = (target ? UP_S : DOWN_S) * Math.abs(target - speed);
    if (target === 1) wakers.slice().forEach(function (fn) { try { fn(); } catch (e) { if (window.console) console.error('[motion-rest] wake failed', e); } });
    // Keep the clock folding while it eases, even if no loop is reading it.
    if (!raf) raf = requestAnimationFrame(easeTick);
  }
  function easeTick() { raf = 0; fold(now0()); if (easeDur) raf = requestAnimationFrame(easeTick); }

  function poke() {
    if (idle) { idle = false; update(); }
    clearTimeout(idleTimer);
    idleTimer = setTimeout(function () { idle = true; update(); }, IDLE_MS);
  }
  ['pointerdown', 'pointermove', 'keydown', 'wheel', 'touchstart', 'scroll'].forEach(function (ev) {
    window.addEventListener(ev, function () { if (pointerOut && ev !== 'scroll') pointerOut = false; poke(); if (!pointerOut) update(); }, { passive: true, capture: true });
  });
  document.addEventListener('visibilitychange', function () { hidden = !!document.hidden; if (!hidden) poke(); update(); });
  window.addEventListener('blur', function () { blurred = true; update(); });
  window.addEventListener('focus', function () { blurred = false; poke(); update(); });
  // Only a mouse leaving the window counts; a lifted finger is not stepping away.
  document.documentElement.addEventListener('mouseleave', function () { pointerOut = true; update(); });
  document.documentElement.addEventListener('mouseenter', function () { pointerOut = false; poke(); update(); });
  poke();
  // The attribute can change after load (My view); re-evaluate when it does.
  if (typeof MutationObserver === 'function' && document.documentElement && document.documentElement.nodeType) {
    new MutationObserver(update).observe(document.documentElement, { attributes: true, attributeFilter: ['data-cz-reduce', 'data-view-motion'] });
  }

  window.MotionRest = {
    now: nowRest,
    speed: function () { fold(now0()); return speed; },
    // Still: at rest and done easing, so a loop can stop asking for frames.
    still: function () { fold(now0()); return speed === 0 && !easeDur; },
    onWake: function (fn) { if (wakers.indexOf(fn) < 0) wakers.push(fn); },
    offWake: function (fn) { var i = wakers.indexOf(fn); if (i >= 0) wakers.splice(i, 1); },
    // For mockups and tests: true rests as if the viewer stepped away; false hands back to the real signals, as if they returned.
    simulate: function (awayNow) { forced = awayNow ? true : null; poke(); update(); },
    IDLE_MS: IDLE_MS, DOWN_S: DOWN_S, UP_S: UP_S
  };
})();
