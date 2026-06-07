// worldstate_provider.js — the worldState provider singleton
// (C-CAL-WORLDSTATE-WIDGETS). ONE source of truth per page for the campaign
// worldState: it fetches GET /campaigns/:id/calendar/world-state at most once
// (memoized promise), normalizes the CATALOG Part-8 seed, fans it out to every
// subscribed worldstate widget, and runs them off a single shared rAF. Reduced
// -motion safe; tears down cleanly when the last subscriber leaves.
//
// Why a singleton: multiple worldstate widgets can mount on one page (e.g. a
// sky-band + a hourglass shelf, or several entity embeds). Each fetching
// independently would hammer the API and desync. They all read this provider.
//
// Seed-or-fetch: if a server already embedded the seed (the entity_worldstate
// block does, on #cal-v2-worldstate), the provider uses it and never fetches —
// so the common entity-page case is zero network calls. A bare widget mount
// with no server seed triggers exactly one fetch, shared by all subscribers.
//
// window.ChronicleWorldState.get(campaignId) → the per-campaign instance.
(function () {
  'use strict';

  var instances = {};

  function reducedMotion() {
    try { return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches); }
    catch (e) { return false; }
  }

  // The API seed and the server-embedded seed are both already the CATALOG
  // Part-8 worldState shape (BuildWorldStateSeed emits it; #401 parity test
  // pins it), so normalize is a defensive pass-through for now.
  function normalize(seed) { return seed || null; }

  function Provider(campaignId) {
    this.campaignId = campaignId;
    this.seed = null;
    this.loaded = false;
    this.error = null;
    this._promise = null;     // memoized → guarantees one fetch per page
    this.fetchCount = 0;      // test/diagnostic seam
    this._subs = [];
    this._errSubs = [];
    this._frameSubs = [];
    this._raf = 0;
  }

  // load(opts) — kick the single fetch (or adopt a server seed). Returns the
  // memoized promise; calling it N times still fetches at most once.
  Provider.prototype.load = function (opts) {
    opts = opts || {};
    if (this._promise) return this._promise;
    var self = this;

    if (opts.seed) {
      this.seed = normalize(opts.seed);
      this.loaded = true;
      this._promise = Promise.resolve(this.seed);
      this._emit();
      return this._promise;
    }

    var url = '/campaigns/' + this.campaignId + '/calendar/world-state';
    var doFetch = ChronicleWorldState._fetch || (window.fetch && window.fetch.bind(window));
    if (!doFetch) {
      this.error = new Error('no fetch available');
      this._promise = Promise.reject(this.error);
      this._emitError(this.error);
      return this._promise;
    }
    this.fetchCount++;
    this._promise = doFetch(url, { headers: { Accept: 'application/json' }, credentials: 'same-origin' })
      .then(function (r) { if (!r.ok) throw new Error('world-state ' + r.status); return r.json(); })
      .then(function (j) { self.seed = normalize(j); self.loaded = true; self._emit(); return self.seed; })
      .catch(function (e) { self.error = e; self._emitError(e); throw e; });
    return this._promise;
  };

  // subscribe(fn) — fn(seed) on data (immediately if already loaded). Returns
  // an unsubscribe; the provider self-destroys when the last sub leaves.
  Provider.prototype.subscribe = function (fn) {
    if (typeof fn !== 'function') return function () {};
    this._subs.push(fn);
    if (this.loaded && this.seed) { try { fn(this.seed); } catch (e) {} }
    var self = this;
    return function () {
      var i = self._subs.indexOf(fn);
      if (i >= 0) self._subs.splice(i, 1);
      if (!self._subs.length) self.destroy();
    };
  };

  // onError(fn) — fn(err) on a load failure (immediately if already errored).
  Provider.prototype.onError = function (fn) {
    if (typeof fn !== 'function') return function () {};
    this._errSubs.push(fn);
    if (this.error) { try { fn(this.error); } catch (e) {} }
    var self = this;
    return function () { var i = self._errSubs.indexOf(fn); if (i >= 0) self._errSubs.splice(i, 1); };
  };

  Provider.prototype.current = function () { return this.seed; };

  // push(seed) — a live update (e.g. GM advanced time): re-fan to subscribers.
  Provider.prototype.push = function (seed) { this.seed = normalize(seed); this.loaded = true; this._emit(); };

  Provider.prototype._emit = function () {
    var s = this.seed;
    this._subs.slice().forEach(function (fn) { try { fn(s); } catch (e) {} });
  };
  Provider.prototype._emitError = function (e) {
    this._errSubs.slice().forEach(function (fn) { try { fn(e); } catch (ee) {} });
  };

  // onFrame(fn) — register a per-frame callback on the SHARED rAF loop (one
  // loop for all subscribers). Reduced-motion: no loop; fn runs once, static.
  Provider.prototype.onFrame = function (fn) {
    if (typeof fn !== 'function') return function () {};
    this._frameSubs.push(fn);
    if (reducedMotion()) { try { fn(0); } catch (e) {} return function () {}; }
    this._startLoop();
    var self = this;
    return function () {
      var i = self._frameSubs.indexOf(fn);
      if (i >= 0) self._frameSubs.splice(i, 1);
      if (!self._frameSubs.length) self._stopLoop();
    };
  };
  Provider.prototype._startLoop = function () {
    if (this._raf) return;
    var self = this, last = 0;
    var tick = function (t) {
      var dt = last ? (t - last) : 16; last = t;
      self._frameSubs.slice().forEach(function (fn) { try { fn(dt); } catch (e) {} });
      self._raf = window.requestAnimationFrame(tick);
    };
    this._raf = window.requestAnimationFrame(tick);
  };
  Provider.prototype._stopLoop = function () {
    if (this._raf) { try { window.cancelAnimationFrame(this._raf); } catch (e) {} this._raf = 0; }
  };

  Provider.prototype.destroy = function () {
    this._stopLoop();
    this._subs = [];
    this._errSubs = [];
    this._frameSubs = [];
    if (instances[this.campaignId] === this) delete instances[this.campaignId];
  };

  window.ChronicleWorldState = {
    get: function (campaignId) {
      campaignId = campaignId || '';
      return instances[campaignId] || (instances[campaignId] = new Provider(campaignId));
    },
    // Test seams — swap fetch, reset the registry between cases.
    _fetch: null,
    _reset: function () { Object.keys(instances).forEach(function (k) { instances[k].destroy(); }); instances = {}; },
  };
})();
