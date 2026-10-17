/**
 * jot_place.js -- where the Jot notes sheet sits: the pure rules.
 *
 * The sheet floats where it was left, can be pinned to either side of the
 * page, or folded to its title bar. This file holds the arithmetic only (no
 * DOM, no storage), so it can be tested headless
 * (test/js/jot_place.test.mjs); notes.js reads and writes the place and draws
 * it.
 *
 * A place is { x, y, w, h, dock, folded }: x and y are the sheet's top-left
 * corner in viewport pixels, or null for "not yet moved" (the corner the tab
 * sits in); dock is 'l', 'r' or null.
 *
 * Exposes Chronicle.JotPlace.
 */
(function (root) {
  'use strict';

  var JP = {};

  // The sheet never gets smaller than this, nor nearer an edge than GAP.
  JP.MIN_W = 260;
  JP.MIN_H = 220;
  JP.GAP = 8;
  // The bottom-right margin the tab and a never-moved sheet share.
  JP.CORNER = 24;
  // How tall a folded sheet is: its title bar.
  JP.FOLDED_H = 44;
  // A pinned sheet is never wider than this, so the page keeps its room.
  JP.DOCK_MAX_W = 480;

  /** The place a first-time viewer gets: today's size, in the tab's corner. */
  JP.fresh = function () {
    return { x: null, y: null, w: 320, h: 480, dock: null, folded: false };
  };

  function num(v) { return typeof v === 'number' && isFinite(v) ? v : null; }

  /**
   * Reads a saved place, tolerating anything a browser may hand back. An
   * older saved size ({w, h} only) keeps its size and starts in the corner.
   */
  JP.read = function (saved) {
    var p = JP.fresh();
    if (!saved || typeof saved !== 'object') return p;
    if (num(saved.w)) p.w = saved.w;
    if (num(saved.h)) p.h = saved.h;
    if (num(saved.x) !== null && num(saved.y) !== null) { p.x = saved.x; p.y = saved.y; }
    if (saved.dock === 'l' || saved.dock === 'r') p.dock = saved.dock;
    p.folded = saved.folded === true;
    return p;
  };

  /**
   * Fits a floating place to a viewport of vw by vh: never smaller than the
   * minimum, never larger than the screen, and its title bar always on
   * screen so it can be grabbed again. Returns a new place.
   */
  JP.clamp = function (p, vw, vh) {
    var g = JP.GAP;
    var q = { x: p.x, y: p.y, w: p.w, h: p.h, dock: p.dock, folded: p.folded };
    q.w = Math.round(Math.max(JP.MIN_W, Math.min(q.w, vw - 2 * g)));
    q.h = Math.round(Math.max(JP.MIN_H, Math.min(q.h, vh - 2 * g)));
    var tall = q.folded ? JP.FOLDED_H : q.h;
    if (q.x === null || q.y === null) {
      q.x = vw - q.w - JP.CORNER;
      q.y = vh - tall - JP.CORNER;
    }
    q.x = Math.round(Math.max(g, Math.min(q.x, vw - q.w - g)));
    q.y = Math.round(Math.max(g, Math.min(q.y, vh - tall - g)));
    return q;
  };

  /** The width a pinned sheet takes from the page. */
  JP.dockWidth = function (p, vw) {
    return Math.round(Math.max(JP.MIN_W, Math.min(p.w, JP.DOCK_MAX_W, vw / 2)));
  };

  /** Whether two rectangles ({left, top, right, bottom}) share any area. */
  JP.overlaps = function (a, b) {
    return !!a && !!b && a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom;
  };

  /**
   * Where the sheet steps to while something under it is being edited:
   * folded in the bottom corner furthest from that thing, horizontally.
   */
  JP.asideFor = function (p, target, vw, vh) {
    var mid = (target.left + target.right) / 2;
    var q = { x: 0, y: 0, w: p.w, h: p.h, dock: null, folded: true };
    q.x = mid > vw / 2 ? JP.GAP : vw - p.w - JP.GAP;
    q.y = vh - JP.FOLDED_H - JP.GAP;
    return JP.clamp(q, vw, vh);
  };

  /** One arrow-key step: 8px, or 24px with Shift. */
  JP.step = function (key, shift) {
    var k = shift ? 24 : 8;
    return { ArrowLeft: [-k, 0], ArrowRight: [k, 0], ArrowUp: [0, -k], ArrowDown: [0, k] }[key] || null;
  };

  root.Chronicle = root.Chronicle || {};
  root.Chronicle.JotPlace = JP;
})(typeof window !== 'undefined' ? window : this);
