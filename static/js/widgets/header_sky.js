/**
 * header_sky.js -- the header's Sky background (data-widget="header-sky").
 *
 * The campaign's sky, the same one the calendar draws over the month, painted
 * across the header. It reads the default calendar (the server puts its id on
 * the element, already checked against what this viewer may see), that day's
 * events and that day's own weather, then paints with SkyPane.Scene from
 * sky_pane.js so both skies agree. Slow devices get the basic sky, as the
 * pane does.
 *
 * Time comes from MotionRest (static/js/motion_rest.js): the sky slows to a
 * stop when the person steps away, stops drawing while still, and picks up
 * where it stopped. Under prefers-reduced-motion or either reduce switch it
 * is painted once and held still. Until the sky is ready, or if it cannot be
 * read, the server's still night colours behind the canvas show instead.
 */
(function () {
  'use strict';

  // How stale the world's clock may get on a page left open; the header
  // outlives boosted navigation, so it re-reads the calendar this often.
  var REFRESH_MS = 5 * 60 * 1000;

  function clock() { return window.MotionRest ? window.MotionRest.now() : performance.now() / 1000; }
  function resting() { return !!(window.MotionRest && window.MotionRest.still()); }

  // Sky(el, o) draws into el's canvas. o: {campaignId, calendarId, still
  // (true to paint once and hold, on top of the page's own reduce rules)}.
  // Throws when there is nothing to draw with.
  function Sky(el, o) {
    var self = this;
    this.el = el;
    this.canvas = el.querySelector('canvas');
    this.ctx = this.canvas && this.canvas.getContext('2d');
    this.S = window.SkyPane && window.SkyPane.Scene;
    if (!this.ctx || !this.S || !window.SkyWorld || !window.Sky2D) throw new Error('header sky: nothing to draw with');
    this.cid = o.campaignId;
    this.calid = o.calendarId;
    this.dpr = Math.max(1, Math.min(2, window.devicePixelRatio || 1));
    this.rm = !!o.still || this.S.reduced();
    this.model = null; this.W = 0; this.H = 0; this.pace = 0; this.raf = 0; this.tmo = 0; this.onScreen = true;
    this._tick = function () { self.tick(); };
    this._kick = function () { self.maybeRefresh(); self.kick(); };
    this._glChange = function () { self.paint(clock()); self.kick(); };
    if (window.MotionRest) window.MotionRest.onWake(this._kick);
    if (window.SkyGL) window.SkyGL.onChange(this._glChange);
    document.addEventListener('visibilitychange', this._kick);
    if ('ResizeObserver' in window) {
      this.ro = new ResizeObserver(function () { if (self.measure()) self.paint(clock()); });
      this.ro.observe(el);
    }
    if ('IntersectionObserver' in window) {
      this.io = new IntersectionObserver(function (es) { es.forEach(function (e) { self.onScreen = e.isIntersecting; }); self.kick(); }, { threshold: 0 });
      this.io.observe(el);
    }
    this.load();
  }

  // load reads the calendar as this viewer may see it, the day's events and
  // the day's own weather. A failure leaves whatever is showing in place.
  Sky.prototype.load = function () {
    var self = this, S = this.S, base = '/campaigns/' + encodeURIComponent(this.cid) + '/calendars/' + encodeURIComponent(this.calid);
    this.loadedAt = Date.now();
    return S.getJSON(base).then(function (cal) {
      return Promise.all([
        S.getJSON(base + '/events?year=' + cal.current_year + '&month=' + cal.current_month).catch(function () { return []; }),
        S.dayWeather(self.cid, self.calid, cal)
      ]).then(function (got) {
        if (self.gone) return;
        var events = Array.isArray(got[0]) ? got[0] : (got[0] && got[0].data) || [];
        var prev = self.model;
        self.model = {
          calendar: cal,
          todayEvents: events.filter(function (e) { return e.year === cal.current_year && e.month === cal.current_month && e.day === cal.current_day; }),
          dayWeather: got[1], skym: window.SkyWorld.makeSkym(cal), landSeed: S.seed(self.calid),
          width: 0, height: 0, reduced: self.rm, glide: prev ? prev.glide : null
        };
        self.measure();
        self.paint(clock());
        self.el.classList.add('is-ready');
        self.kick();
      });
    }).catch(function (err) {
      if (window.console) console.warn('[header-sky] the sky could not be read', err);
    });
  };
  Sky.prototype.maybeRefresh = function () {
    if (!document.hidden && !this.gone && Date.now() - (this.loadedAt || 0) > REFRESH_MS) this.load();
  };

  Sky.prototype.measure = function () {
    var W = this.el.clientWidth, H = this.el.clientHeight;
    if (!W || !H) return false;
    var bw = Math.round(W * this.dpr), bh = Math.round(H * this.dpr);
    if (this.canvas.width !== bw || this.canvas.height !== bh) { this.canvas.width = bw; this.canvas.height = bh; }
    this.W = W; this.H = H;
    return true;
  };

  // paint draws one frame of the sky across the whole header.
  Sky.prototype.paint = function (t) {
    if (!this.model || (!this.W && !this.measure())) return;
    var dt = this.lastPaint ? t - this.lastPaint : 0;
    this.lastPaint = t;
    this.model.width = this.W; this.model.height = this.H;
    var st = this.S.state(this.model, this.rm ? 0 : t, dt);
    this.ctx.setTransform(1, 0, 0, 1, 0, 0);
    this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
    this.S.paint(this.ctx, this, st, this.dpr, 0, !!this.looping);
    this.pace = this.rm ? 0 : this.S.pace(st);
  };

  Sky.prototype.tick = function () {
    this.raf = 0;
    this.looping = true; this.paint(clock()); this.looping = false;
    this.kick();
  };
  // kick asks for the next frame: every frame while something visibly moves,
  // the idle rate while only clouds drift, none while still or out of sight.
  Sky.prototype.kick = function () {
    var self = this, live = !this.gone && !document.hidden && !this.rm && this.onScreen && !resting();
    if (live && this.pace === 2) { if (this.tmo) { clearTimeout(this.tmo); this.tmo = 0; } if (!this.raf) this.raf = requestAnimationFrame(this._tick); return; }
    if (this.raf) { cancelAnimationFrame(this.raf); this.raf = 0; }
    if (live && this.pace === 1) { if (!this.tmo) this.tmo = setTimeout(function () { self.tmo = 0; if (!self.raf) self.raf = requestAnimationFrame(self._tick); }, this.S.IDLE_S * 1000); }
    else if (this.tmo) { clearTimeout(this.tmo); this.tmo = 0; }
  };

  Sky.prototype.destroy = function () {
    this.gone = true;
    if (this.raf) cancelAnimationFrame(this.raf);
    if (this.tmo) clearTimeout(this.tmo);
    this.raf = 0; this.tmo = 0;
    if (window.MotionRest) window.MotionRest.offWake(this._kick);
    if (window.SkyGL) window.SkyGL.offChange(this._glChange);
    document.removeEventListener('visibilitychange', this._kick);
    if (this.ro) this.ro.disconnect();
    if (this.io) this.io.disconnect();
  };

  // mount draws the sky into el, or returns null when it cannot, leaving
  // whatever is behind el showing. The Customize page's example header
  // uses it too, so both draw the same sky.
  function mount(el, o) {
    try { return new Sky(el, o); }
    catch (e) { if (window.console) console.warn('[header-sky]', e.message); return null; }
  }
  Chronicle.headerSky = { mount: mount };

  Chronicle.register('header-sky', {
    init: function (el) {
      el._headerSky = mount(el, { campaignId: el.getAttribute('data-campaign-id'), calendarId: el.getAttribute('data-calendar-id') });
    },
    destroy: function (el) {
      if (el._headerSky) el._headerSky.destroy();
      el._headerSky = null;
    }
  });
})();
