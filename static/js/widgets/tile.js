/**
 * tile.js -- tells a widget in a tile section when its tile changes size.
 *
 * A tile section (internal/tilegrid) draws each widget inside a cell that is
 * itself a `data-widget="tile"` mount and carries data-tile-w / data-tile-h,
 * the size the tile is drawn at. A widget reads them when it mounts to decide
 * how much to show. This mount announces later changes: it dispatches a
 * bubbling `chronicle:tile-resize` event on the cell when either attribute
 * changes (the layout editor resizing a tile) and when the screen crosses the
 * narrow breakpoint, where the grid drops from 4 columns to 2 and a wider tile
 * spans the whole row.
 *
 * event.detail:
 *   w, h       the stored size, from data-tile-w / data-tile-h
 *   columns    the columns the tile spans right now (w, clamped to 2 on a
 *              narrow screen); use this to choose a layout
 *   narrow     true on the 2-column grid
 *   previous   { w, h } before the change, or null when the screen width
 *              (not the size) caused it
 *
 * Nothing fires for a tile's first appearance: the widget reads the
 * attributes when it mounts. Documented for widget authors in
 * docs/build/widget.md.
 */
(function () {
  'use strict';

  var EVENT = 'chronicle:tile-resize';
  var NARROW_QUERY = '(max-width: 639px)';
  var NARROW_COLUMNS = 2;

  /** Columns a tile of width w spans on the current grid. */
  function effectiveColumns(w, narrow) {
    return narrow ? Math.min(w, NARROW_COLUMNS) : w;
  }

  function isNarrow() {
    return !!(window.matchMedia && window.matchMedia(NARROW_QUERY).matches);
  }

  /** Reads a tile cell's size, or null when it has no valid size. */
  function readSize(el, narrow) {
    var w = parseInt(el.getAttribute('data-tile-w'), 10);
    var h = parseInt(el.getAttribute('data-tile-h'), 10);
    if (!(w >= 1) || !(h >= 1)) return null;
    return { w: w, h: h, columns: effectiveColumns(w, narrow), narrow: narrow };
  }

  function announce(el, previous) {
    var size = readSize(el, isNarrow());
    if (!size) return;
    size.previous = previous || null;
    el.dispatchEvent(new CustomEvent(EVENT, { bubbles: true, detail: size }));
  }

  /**
   * The size before a batch of attribute changes: the earliest recorded old
   * value of each attribute, the other one as it is now.
   */
  function previousOf(el, mutations) {
    var prev = {
      w: parseInt(el.getAttribute('data-tile-w'), 10),
      h: parseInt(el.getAttribute('data-tile-h'), 10),
    };
    var seen = {};
    for (var i = 0; i < mutations.length; i++) {
      var m = mutations[i];
      if (seen[m.attributeName]) continue;
      seen[m.attributeName] = true;
      var old = parseInt(m.oldValue, 10);
      if (m.attributeName === 'data-tile-w') prev.w = old;
      if (m.attributeName === 'data-tile-h') prev.h = old;
    }
    return prev;
  }

  // Per mounted cell: what to undo in destroy.
  var mounts = typeof WeakMap === 'function' ? new WeakMap() : null;

  Chronicle.register('tile', {
    init: function (el) {
      var state = { observer: null, mq: null, onBreakpoint: null };

      if (typeof MutationObserver === 'function') {
        state.observer = new MutationObserver(function (mutations) {
          announce(el, previousOf(el, mutations));
        });
        state.observer.observe(el, {
          attributes: true,
          attributeOldValue: true,
          attributeFilter: ['data-tile-w', 'data-tile-h'],
        });
      }

      if (window.matchMedia) {
        state.mq = window.matchMedia(NARROW_QUERY);
        state.onBreakpoint = function () {
          // Only a tile wider than the narrow grid changes its span.
          if (parseInt(el.getAttribute('data-tile-w'), 10) > NARROW_COLUMNS) {
            announce(el, null);
          }
        };
        if (state.mq.addEventListener) state.mq.addEventListener('change', state.onBreakpoint);
        else if (state.mq.addListener) state.mq.addListener(state.onBreakpoint);
      }

      if (mounts) mounts.set(el, state);
    },

    destroy: function (el) {
      var state = mounts && mounts.get(el);
      if (!state) return;
      if (state.observer) state.observer.disconnect();
      if (state.mq && state.onBreakpoint) {
        if (state.mq.removeEventListener) state.mq.removeEventListener('change', state.onBreakpoint);
        else if (state.mq.removeListener) state.mq.removeListener(state.onBreakpoint);
      }
      mounts.delete(el);
    },
  });
})();
