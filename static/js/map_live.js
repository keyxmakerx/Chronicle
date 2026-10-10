/**
 * map_live.js -- Chronicle map live refresh
 *
 * The server announces a change to a map's pins, drawings, tokens or shadows as
 * an id-only "map.items.changed" message ({map_id, kind}); it never carries item
 * content. The viewer answers by refetching the role-filtered list for that
 * kind, so what a person learns is exactly what their own read returns.
 *
 * This file is the pure scheduling core of that answer, kept apart from Leaflet
 * so it can be tested in Node:
 *   - bursts collapse into one refetch per kind (debounce);
 *   - a kind that is mid-edit (busy) is held back and retried, never fetched
 *     over the top of the edit;
 *   - a failed or malformed refetch changes nothing, so the layer on screen is
 *     never blanked by a hiccup;
 *   - a change that arrives while a refetch is in flight triggers one more
 *     afterwards, so the last write is never missed.
 *
 * Exposes window.ChronicleMapLive = { create, kindOf, DELAY_MS }.
 */
(function () {
  'use strict';

  var DELAY_MS = 300;

  // Shadows and pictures are drawings on the wire, so they share the drawings
  // list and one refetch covers all three. Tokens have no layer on the web map
  // viewer; a kind with no handler is ignored.
  var ALIAS = { shadows: 'drawings' };

  function kindOf(kind) {
    return Object.prototype.hasOwnProperty.call(ALIAS, kind) ? ALIAS[kind] : kind;
  }

  /**
   * create({ kinds, delay, setTimer, clearTimer }) returns { notify(kind),
   * destroy() }. kinds maps a kind to { fetch(), apply(list), busy() }:
   * fetch returns a promise of the list (rejecting on any failure), apply
   * redraws that layer from it, busy says an edit on that kind is in progress.
   */
  function create(opts) {
    var kinds = opts.kinds || {};
    var delay = opts.delay == null ? DELAY_MS : opts.delay;
    var setTimer = opts.setTimer || function (fn, ms) { return setTimeout(fn, ms); };
    var clearTimer = opts.clearTimer || function (id) { clearTimeout(id); };
    var pending = {};
    var inflight = {};
    var timer = null;
    var destroyed = false;

    function arm() {
      if (destroyed || timer !== null) return;
      timer = setTimer(run, delay);
    }

    function refetch(kind) {
      var h = kinds[kind];
      inflight[kind] = true;
      var p;
      try { p = Promise.resolve(h.fetch()); } catch (e) { p = Promise.reject(e); }
      p.then(function (list) {
        // Anything but a list is a failure: keep what is on screen.
        if (destroyed || !Array.isArray(list)) return;
        h.apply(list);
      }).catch(function () {
        /* keep what is on screen; the next change or reconnect tries again */
      }).then(function () {
        inflight[kind] = false;
        if (pending[kind]) arm();
      });
    }

    function run() {
      timer = null;
      if (destroyed) return;
      var again = false;
      Object.keys(pending).forEach(function (kind) {
        if (!pending[kind]) return;
        var h = kinds[kind];
        if (!h) { pending[kind] = false; return; }
        // The in-flight refetch re-arms the timer when it settles.
        if (inflight[kind]) return;
        var busy = false;
        try { busy = !!(h.busy && h.busy()); } catch (e) { busy = false; }
        if (busy) { again = true; return; }
        pending[kind] = false;
        refetch(kind);
      });
      if (again) arm();
    }

    return {
      notify: function (kind) {
        kind = kindOf(kind);
        if (destroyed || !kinds[kind]) return;
        pending[kind] = true;
        arm();
      },
      destroy: function () {
        destroyed = true;
        if (timer !== null) clearTimer(timer);
        timer = null;
        pending = {};
      }
    };
  }

  var api = { create: create, kindOf: kindOf, DELAY_MS: DELAY_MS };
  if (typeof window !== 'undefined') window.ChronicleMapLive = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})();
