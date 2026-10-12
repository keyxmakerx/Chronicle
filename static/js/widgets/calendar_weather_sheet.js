/**
 * calendar_weather_sheet.js — the weather calendar: a compact month in the
 * calendar's side drawer that holds only weather. Each day shows its number,
 * its weather, a stripe in its season's colour and its sky (full and new
 * moon, eclipses and the other moon nights). The Director chooses days by
 * dragging across them or with All, then clicks a weather or drags one onto
 * a day, pins days, or rolls new weather that follows each day's season and
 * the climate. Opened from the edit strip's and the bulk bar's Weather
 * buttons (calendar_editor.js).
 *
 * Nothing is written until Save: the panel keeps a draft, and a close with
 * unsaved changes shakes the panel and says so (Discard leaves). Days painted
 * by hand and pinned days are never replaced by a roll; a pin is saved as the
 * day's lock. Saving and Undo belong to the editor (opts.save); this file only
 * shapes what would be saved. The weather itself is chronicle_gen.js, loaded
 * by the editor before this opens.
 */
(function () {
  'use strict';

  function $(sel, root) { return (root || document).querySelector(sel); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (ch) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch];
    });
  }
  function reducedMotion() { return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches; }

  // Six common kinds sit in the palette; More weather… opens the whole list,
  // and a kind picked there joins the palette while the panel is open.
  var PAINT_COMMON = ['clear', 'partly-cloudy', 'cloudy', 'rain', 'snow', 'fog'];
  var PAINT_GROUPS = [['Yours', 'Yours'], ['Standard', 'Common'], ['Severe', 'Stormy'], ['Environmental', 'Nature'], ['Fantasy', 'Magic']];

  // The moon nights a calendar stores (MoonNightPayload in model.go), most
  // striking first: one mark per day, so the strongest wins. weather is what
  // "Sky events bring their own weather" sets on that day, or null.
  var SKY = {
    eclipse: { label: 'Eclipse', icon: 'fa-circle-half-stroke', weather: 'black-sun' },
    blood: { label: 'Blood moon', icon: 'fa-circle', weather: 'black-sun' },
    magic: { label: 'Magic night', icon: 'fa-wand-sparkles', weather: 'ley-surge' },
    conjunction: { label: 'Moons meet', icon: 'fa-circle-nodes', weather: 'luminous-sky' },
    harvest: { label: 'Harvest moon', icon: 'fa-wheat-awn', weather: null },
    full: { label: 'Full moon', weather: null },
    'new': { label: 'New moon', weather: null }
  };
  var SKY_ORDER = ['eclipse', 'blood', 'magic', 'conjunction', 'harvest'];

  function dkey(y, m, d) { return y + '_' + m + '_' + d; }
  function parseKey(k) { var p = k.split('_').map(Number); return { year: p[0], month: p[1], day: p[2] }; }
  function painted(w) { return !!w && w.source !== 'generated'; }
  function randomSeed() { return Math.random().toString(36).slice(2, 8); }
  function plural(n, one) { return n + ' ' + (n === 1 ? one : one + 's'); }

  // A stored reading as the generator reads it: zone left off, so a day
  // generated under another climate still counts as the weather around.
  function asEngineDay(w) {
    var c = {};
    Object.keys(w).forEach(function (k) { c[k] = w[k]; });
    c.zone_id = null;
    c.zone_name = null;
    return c;
  }

  function continuityWords(v) {
    if (v < 0.25) return 'Changes daily';
    if (v < 0.5) return 'Changeable';
    if (v < 0.75) return 'Some spells';
    return 'Long spells';
  }

  // ownKinds keeps the kinds the generator accepts, so one bad stored kind
  // costs only itself rather than failing every run.
  function ownKinds(list) {
    if (!Array.isArray(list)) return [];
    var check = window.ChronicleGen.weather.validateKind;
    return list.filter(function (k) { return check(k, list).ok; });
  }

  // moonPeaks marks the one day nearest each full and new moon of a moon in
  // a month: a phase name covers several days of a long cycle, a mark only
  // its turning point. phaseAt(abs) is the moon's phase (0..1) on that day.
  function moonPeaks(phaseAt, absDays, cycle) {
    var out = {}, tol = 0.5 / Math.max(1, cycle);
    var dist = function (p, t) { var d = Math.abs(p - t); return Math.min(d, 1 - d); };
    absDays.forEach(function (abs, i) {
      ['full', 'new'].forEach(function (kind) {
        var t = kind === 'full' ? 0.5 : 0, here = dist(phaseAt(abs), t);
        if (here > tol + 1e-9) return;
        if (here < dist(phaseAt(abs - 1), t) && here <= dist(phaseAt(abs + 1), t)) out[i] = kind;
      });
    });
    return out;
  }

  function Panel(view) {
    this.view = view;
    var dock = view.dockEl;
    this.scrim = $('.dscrim', dock);
    if (!this.scrim) { this.scrim = document.createElement('div'); this.scrim.className = 'dscrim'; dock.appendChild(this.scrim); }
    this.el = document.createElement('aside');
    this.el.className = 'drawer ded gsheet wxcal';
    this.el.setAttribute('role', 'dialog');
    this.el.setAttribute('aria-labelledby', 'cal5-wxtitle');
    this.el.tabIndex = -1;
    dock.appendChild(this.el);
    this.state = null;
    this._climate = 'temperate';
    this._continuity = 0.55;
    this._skyOwn = true;
    // The owner's own kinds of weather, from Calendar settings.
    this._kinds = [];
    // Set once the owner changes the climate or continuity here, so the
    // calendar's own settings stop replacing them for this page.
    this._picked = false;
    this._bind();
  }

  // _useSettings starts the panel from the calendar's Weather settings
  // (climate, how long weather lasts, own kinds), or keeps the defaults when
  // they could not be read.
  Panel.prototype._useSettings = function (s) {
    if (!s) return;
    this._kinds = ownKinds(s.kinds);
    if (this._picked) return;
    var known = window.ChronicleGen.weather.climates().some(function (c) { return c.id === s.climate; });
    var cont = +s.continuity;
    if (known) this._climate = s.climate;
    if (isFinite(cont) && cont >= 0 && cont <= 1) this._continuity = cont;
  };

  Panel.prototype.isOpen = function () { return this.el.classList.contains('open'); };

  // opts: {year, month: the month to show; chosen: day keys ('y_m_d') to
  // start chosen; settings: the calendar's Weather settings or null;
  // save: function (changes) -> Promise<bool>}. The editor has already
  // loaded the shown month's weather year and events.
  Panel.prototype.open = function (opts) {
    var view = this.view, back = document.activeElement;
    view.closeAllPanels({ instant: true });
    var sel = {};
    (opts.chosen || []).forEach(function (k) { sel[k] = true; });
    this.state = {
      y: opts.year, m: opts.month, save: opts.save, back: back,
      draft: {}, pins: {}, sel: sel, added: [], last: null, more: false, query: '',
      focus: null, busy: false, nag: false, note: ''
    };
    this._useSettings(opts.settings);
    this._presets = {};
    var self = this;
    window.ChronicleGen.weather.presets(this._kinds).forEach(function (p) { self._presets[p.id] = p; });
    this._renderShell();
    this._renderMonth();
    void this.el.offsetWidth;
    this.el.classList.add('open');
    view.dockEl.classList.add('on');
    var first = $('.wxc-d[tabindex="0"]', this.el) || this.el;
    setTimeout(function () { first.focus({ preventScroll: true }); }, reducedMotion() ? 20 : 260);
  };

  // close puts the panel away and drops the draft. Callers that must not
  // lose work go through tryClose.
  Panel.prototype.close = function (quiet) {
    if (!this.isOpen()) return;
    var back = this.state && this.state.back;
    this.el.classList.remove('open', 'nag');
    this.view.dockEl.classList.remove('on');
    this.state = null;
    this._preview();
    if (!quiet && back && back.isConnected) back.focus({ preventScroll: true });
  };

  Panel.prototype.dirty = function () {
    var S = this.state;
    return !!S && (Object.keys(S.draft).length > 0 || Object.keys(S.pins).length > 0);
  };

  // tryClose closes when nothing is unsaved; otherwise the panel shakes and
  // shows "Not saved yet", and stays.
  Panel.prototype.tryClose = function () {
    if (!this.isOpen()) return;
    if (!this.dirty()) { this.close(); return; }
    var S = this.state, el = this.el;
    S.nag = true;
    this._renderFoot();
    if (!reducedMotion()) { el.classList.remove('nag'); void el.offsetWidth; el.classList.add('nag'); }
    var w = $('.wxc-warn', el);
    if (w) w.focus({ preventScroll: true });
  };

  // ---- the month and its days ----
  Panel.prototype._monthDef = function () {
    var cal = this.view.cal, CalDate = Chronicle.calendarDate, mc = CalDate.monthCount(cal);
    var m0 = ((this.state.m - 1) % mc + mc) % mc;
    return { m0: m0, def: (cal.months || [])[m0] || null, days: CalDate.monthDays(cal, m0, this.state.y) };
  };

  // _keys is every day of the shown month, in order.
  Panel.prototype._keys = function () {
    var S = this.state, n = this._monthDef().days, out = [];
    for (var d = 1; d <= n; d++) out.push(dkey(S.y, S.m, d));
    return out;
  };

  Panel.prototype._stored = function (k) {
    var p = parseKey(k), byDay = this.view.weatherByYear[p.year];
    return (byDay && byDay[p.month + '_' + p.day]) || null;
  };

  // _reading is what a day would hold after Save: the draft's reading (null
  // when cleared) or what is stored.
  Panel.prototype._reading = function (k) {
    var dr = this.state.draft[k];
    return dr ? dr.w : this._stored(k);
  };

  Panel.prototype._pinned = function (k) {
    var p = this.state.pins[k];
    if (p != null) return p;
    var s = this._stored(k);
    return !!(s && s.locked === true);
  };

  // A day a roll leaves alone: pinned, or holding weather painted by hand.
  Panel.prototype._stays = function (k) { return this._pinned(k) || painted(this._reading(k)); };

  Panel.prototype._chosen = function () {
    var S = this.state;
    return this._keys().filter(function (k) { return S.sel[k]; });
  };

  // _seasonAt is the calendar season a day falls in, or null.
  Panel.prototype._seasonAt = function (k) {
    var p = parseKey(k);
    return this.view.seasonForDate ? this.view.seasonForDate(p.month, p.day) : null;
  };

  // _sky is each shown day's sky mark: the strongest moon night stored on it
  // (only what this viewer's events carry), else the main moon's full or new
  // turning point. Keyed by day key.
  Panel.prototype._sky = function () {
    var view = this.view, cal = view.cal, S = this.state, CalDate = Chronicle.calendarDate, out = {};
    var keys = this._keys(), moon = view.mainMoon ? view.mainMoon() : null;
    if (moon && moon.cycle_days > 0) {
      var abs = keys.map(function (k) { var p = parseKey(k); return CalDate.dayIndex(cal, p.year, p.month, p.day); });
      var phaseAt = function (a) { var r = (a + (moon.phase_offset || 0)) / moon.cycle_days; return r - Math.floor(r); };
      var peaks = moonPeaks(phaseAt, abs, moon.cycle_days);
      Object.keys(peaks).forEach(function (i) { out[keys[i]] = { type: peaks[i], name: moon.name }; });
    }
    keys.forEach(function (k) {
      var p = parseKey(k), best = null;
      (view.eventsOnDay ? view.eventsOnDay(p.year, p.month, p.day) : []).forEach(function (e) {
        if (!e.payload) return;
        var pl;
        try { pl = JSON.parse(e.payload); } catch (err) { return; }
        var rank = pl ? SKY_ORDER.indexOf(pl.type) : -1;
        if (rank >= 0 && (!best || rank < best.rank)) best = { type: pl.type, name: e.name, rank: rank };
      });
      if (best) out[k] = { type: best.type, name: best.name };
    });
    S.skyCache = out;
    return out;
  };

  // ---- changing the draft ----
  // _setDraft records a day's new reading (null clears it), dropping the
  // entry again when it matches what is stored.
  Panel.prototype._setDraft = function (k, w) {
    var S = this.state, s = this._stored(k);
    if (!w && !s) { delete S.draft[k]; return; }
    S.draft[k] = { w: w };
  };

  Panel.prototype._setPin = function (k, on) {
    var S = this.state, s = this._stored(k);
    if (on === !!(s && s.locked === true)) delete S.pins[k];
    else S.pins[k] = on;
  };

  // paint sets one kind of weather on the given days as painted by hand.
  Panel.prototype.paint = function (id, keys) {
    var p = this._presets[id], self = this;
    if (!p || !keys.length) return 0;
    this.state.last = id;
    keys.forEach(function (k) {
      var d = parseKey(k);
      self._setDraft(k, { year: d.year, month: d.month, day: d.day, source: 'manual',
        preset_id: p.id, preset_label: p.label, icon: p.glyph, color: p.color });
    });
    return keys.length;
  };

  Panel.prototype.clear = function (keys) {
    var self = this, n = 0;
    keys.forEach(function (k) { if (self._reading(k)) { self._setDraft(k, null); n++; } });
    return n;
  };

  // togglePins pins the given days, or unpins them when all are pinned. A pin
  // is saved as the day's lock, and only a day with weather can be locked, so
  // days without weather are left out; returns null when none have any.
  Panel.prototype.togglePins = function (keys) {
    var self = this;
    keys = keys.filter(function (k) { return self._reading(k); });
    if (!keys.length) return null;
    var all = keys.every(function (k) { return self._pinned(k); });
    keys.forEach(function (k) { self._setPin(k, !all); });
    return !all;
  };

  // ---- rolling ----
  Panel.prototype._recipe = function () {
    return { generator: 'weather', details: { climate: this._climate, continuity: this._continuity } };
  };

  // _engineRun runs the generator over targets with the given locks, and the
  // rest of the year (stored, with the draft on top) as the weather around.
  Panel.prototype._engineRun = function (targets, locks, seed) {
    var S = this.state, view = this.view, self = this, inScope = {}, known = {};
    targets.concat(locks.map(function (d) { return dkey(d.year, d.month, d.day); })).forEach(function (k) { inScope[k] = true; });
    var context = [], byDay = view.weatherByYear[S.y] || {};
    Object.keys(byDay).forEach(function (md) { var k = S.y + '_' + md; if (!inScope[k] && !S.draft[k]) context.push(asEngineDay(byDay[md])); });
    Object.keys(S.draft).forEach(function (k) { var w = S.draft[k].w; if (w && !inScope[k] && parseKey(k).year === S.y) context.push(asEngineDay(w)); });
    window.ChronicleGen.weather.presets(this._kinds).forEach(function (p) { known[p.id] = true; });
    return window.ChronicleGen.run('weather', {
      calendar: view.cal, recipe: this._recipe(), seed: seed,
      scope: { days: targets.map(parseKey) },
      locked: locks.filter(function (d) { return !d.preset_id || known[d.preset_id]; }),
      context: { weather: context }, kinds: self._kinds
    });
  };

  // roll draws fresh weather for the chosen days (or the whole month when
  // none are chosen), leaving pinned and painted days as they are. With
  // "Sky events bring their own weather" on, a day under an eclipse or
  // another moon night takes that night's weather, and the days around it
  // ease into it. Returns {rolled: [keys], left: n} or {error}.
  Panel.prototype.roll = function (seed) {
    var S = this.state, self = this;
    var scope = this._chosen();
    if (!scope.length) scope = this._keys();
    var targets = scope.filter(function (k) { return !self._stays(k); });
    if (!targets.length) return { rolled: [], left: scope.length };
    var monthLocks = this._keys().filter(function (k) { return targets.indexOf(k) < 0 && self._reading(k); })
      .map(function (k) { return asEngineDay(self._reading(k)); });
    seed = seed || randomSeed();
    var res;
    try {
      res = this._engineRun(targets, monthLocks, seed);
      var sky = this._skyOwn ? (S.skyCache || this._sky()) : {}, overlays = [];
      targets.forEach(function (k) {
        var mark = sky[k], wx = mark && SKY[mark.type] && SKY[mark.type].weather, p = wx && self._presets[wx];
        if (!p) return;
        var base = res.days.filter(function (d) { return dkey(d.year, d.month, d.day) === k; })[0];
        if (!base) return;
        var o = asEngineDay(base);
        o.preset_id = p.id; o.preset_label = p.label; o.icon = p.glyph; o.color = p.color;
        o.description = p.label; o.precipitation = null;
        o.gen = {};
        overlays.push(o);
      });
      if (overlays.length) {
        var skyKeys = overlays.map(function (o) { return dkey(o.year, o.month, o.day); });
        var rest = targets.filter(function (k) { return skyKeys.indexOf(k) < 0; });
        var days = overlays.slice();
        if (rest.length) days = days.concat(this._engineRun(rest, monthLocks.concat(overlays), seed).days);
        res = { days: days, summary: res.summary };
      }
    } catch (e) {
      return { error: e && e.friendly ? e.message : 'The weather could not be generated for this calendar.' };
    }
    var rolled = [];
    res.days.forEach(function (d) {
      var k = dkey(d.year, d.month, d.day);
      if (targets.indexOf(k) < 0) return;
      var w = {};
      Object.keys(d).forEach(function (f) { if (f !== 'gen') w[f] = d[f]; });
      w.source = 'generated';
      self._setDraft(k, w);
      rolled.push(k);
    });
    return { rolled: rolled, left: scope.length - targets.length };
  };

  // ---- saving ----
  // changes turns the draft into the editor's writes: readings to put (the
  // per-day API's shape), days to clear, and days to lock or unlock. The
  // server keeps a painted or locked day when a generated reading is put on
  // it, so such a day is cleared first; a painted put clears a day's lock, so
  // a pinned day that is written is locked again after.
  Panel.prototype.changes = function () {
    var S = this.state, self = this, G = window.ChronicleGen;
    var put = [], clear = [], lock = [], unlock = [], written = {};
    Object.keys(S.draft).forEach(function (k) {
      var w = S.draft[k].w, d = parseKey(k);
      if (!w) { if (self._stored(k)) clear.push(d); return; }
      var body = w.source === 'generated' ? G.weather.toWeatherInput(w) : {
        preset_id: w.preset_id, preset_label: w.preset_label, icon: w.icon, color: w.color
      };
      body.year = d.year; body.month = d.month; body.day = d.day; body.source = w.source === 'generated' ? 'generated' : 'manual';
      var s = self._stored(k);
      if (body.source === 'generated' && s && (painted(s) || s.locked === true)) clear.push(d);
      put.push(body);
      written[k] = true;
    });
    var touched = {};
    Object.keys(S.draft).concat(Object.keys(S.pins)).forEach(function (k) { touched[k] = true; });
    Object.keys(touched).forEach(function (k) {
      var w = self._reading(k), s = self._stored(k), pin = self._pinned(k);
      if (pin && w && (written[k] || !(s && s.locked === true))) lock.push(parseKey(k));
      else if (!pin && s && s.locked === true && !written[k] && w) unlock.push(parseKey(k));
    });
    return { put: put, clear: clear, lock: lock, unlock: unlock };
  };

  // _preview hands the unsaved readings to the calendar behind as faint
  // marks, so the month there shows what Save would write.
  Panel.prototype._preview = function () {
    var view = this.view, S = this.state;
    if (!S) view.wxPreview = null;
    else {
      view.wxPreview = {};
      Object.keys(S.draft).forEach(function (k) { if (S.draft[k].w) view.wxPreview[k] = S.draft[k].w; });
    }
    if (view._paintMonth) view._paintMonth();
  };

  // ---- markup ----
  function moonSVG(full) {
    return '<svg width="8" height="8" viewBox="-5 -5 10 10" aria-hidden="true">' +
      (full ? '<circle r="4.2" fill="currentColor"/>' : '<circle r="3.8" fill="none" stroke="currentColor" stroke-width="1.4"/>') + '</svg>';
  }
  function skyGlyph(type) {
    if (type === 'full' || type === 'new') return moonSVG(type === 'full');
    return '<i class="fa-solid ' + SKY[type].icon + '" aria-hidden="true"></i>';
  }

  // _renderShell draws the parts that stay while the panel is open; the
  // month, the palette and the footer redraw on their own.
  Panel.prototype._renderShell = function () {
    var climates = window.ChronicleGen.weather.climates(), cur = this._climate;
    var opt = function (c) { return '<option value="' + esc(c.id) + '"' + (c.id === cur ? ' selected' : '') + '>' + esc(c.name) + '</option>'; };
    this.el.innerHTML = '<div class="grab" aria-hidden="true"></div>' +
      '<div class="crease gcr"><span id="cal5-wxtitle">Weather</span><span class="sub">ONLY WEATHER, DAY BY DAY</span>' +
        '<button type="button" class="x" data-wx-close aria-label="Close"><i class="fa-solid fa-xmark"></i></button></div>' +
      '<div class="gbody wxc">' +
        '<div class="wxc-mon"><h3 id="cal5-wxmon" aria-live="polite"></h3>' +
          '<button type="button" class="wxc-nav" data-wx-nav="-1" aria-label="Previous month"><i class="fa-solid fa-chevron-left"></i></button>' +
          '<button type="button" class="wxc-nav" data-wx-nav="1" aria-label="Next month"><i class="fa-solid fa-chevron-right"></i></button></div>' +
        '<div class="wxc-seas" id="cal5-wxseas"></div>' +
        '<div class="wxc-cal" id="cal5-wxcal" role="grid" aria-label="Days of the month" aria-multiselectable="true"></div>' +
        '<div class="wxc-key" id="cal5-wxkey"></div>' +
        '<div class="wxc-selrow" id="cal5-wxsel"></div>' +
        '<div class="wxc-lab" id="cal5-wxpall">Paint</div>' +
        '<div class="wxc-pal" id="cal5-wxpal" role="group" aria-labelledby="cal5-wxpall"></div>' +
        '<div class="wxc-more" id="cal5-wxmore" hidden><input type="search" class="wxq" id="cal5-wxq" placeholder="Search weather" aria-label="Search weather"><div class="wxl" role="list"></div></div>' +
        '<div class="wxc-lab">Roll</div>' +
        '<div class="wxc-roll">' +
          '<label class="fld">Climate<select id="cal5-wxclim">' +
            '<optgroup label="Natural">' + climates.filter(function (c) { return !c.magic; }).map(opt).join('') + '</optgroup>' +
            '<optgroup label="Magic">' + climates.filter(function (c) { return c.magic; }).map(opt).join('') + '</optgroup>' +
          '</select></label>' +
          '<label class="fld">How long weather lasts<span class="dv">' +
            '<input type="range" id="cal5-wxcont" min="0" max="1" step="0.05" value="' + this._continuity + '">' +
            '<output id="cal5-wxconto">' + continuityWords(this._continuity) + '</output></span></label>' +
          '<label class="wxc-chk"><input type="checkbox" id="cal5-wxsky"' + (this._skyOwn ? ' checked' : '') + '>' +
            '<span>Sky events bring their own weather: an eclipse darkens its day, a magic night or meeting moons light the sky</span></label>' +
          '<div class="wxc-go"><span class="note" id="cal5-wxnote" aria-live="polite"></span>' +
            '<button type="button" class="btn sm" data-wx-roll><i class="fa-solid fa-dice"></i> Roll</button></div>' +
        '</div>' +
      '</div>' +
      '<div class="wxc-warn" role="alert" tabindex="-1" hidden><i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i>' +
        '<span>Not saved yet</span><span class="sp"></span><button type="button" data-wx-discard>Discard</button></div>' +
      '<div class="efoot gfoot"><span class="gfa">' +
        '<button type="button" class="btn quiet" data-wx-cancel>Cancel</button>' +
        '<button type="button" class="btn primary" data-wx-save>Save weather</button></span></div>';
  };

  // _renderMonth redraws everything that follows the shown month.
  Panel.prototype._renderMonth = function () {
    var S = this.state, md = this._monthDef();
    $('#cal5-wxmon', this.el).innerHTML = esc(md.def ? md.def.name : 'Month ' + S.m) + ' <span>' + esc(S.y) + '</span>';
    this._sky();
    this._renderSeasons();
    this._renderGrid();
    this._renderKey();
    this._renderPalette();
    this._renderSel();
    this._renderFoot();
  };

  Panel.prototype._renderSeasons = function () {
    var self = this, runs = [];
    this._keys().forEach(function (k) {
      var s = self._seasonAt(k), last = runs[runs.length - 1], d = parseKey(k).day;
      if (last && last.s === s) last.to = d;
      else runs.push({ s: s, from: d, to: d });
    });
    $('#cal5-wxseas', this.el).innerHTML = runs.filter(function (r) { return r.s; }).map(function (r) {
      var c = Chronicle.calendarColor(r.s.color);
      return '<span' + (c ? ' style="--sc:' + c + '"' : '') + '><i></i>' + esc(r.s.name) + ' · ' + (r.from === r.to ? r.from : r.from + '–' + r.to) + '</span>';
    }).join('');
  };

  Panel.prototype._cellLabel = function (k, w, season, sky) {
    var md = this._monthDef(), parts = [parseKey(k).day + ' ' + (md.def ? md.def.name : '')];
    parts.push(w ? (w.preset_label || w.description || 'Weather') + (painted(w) ? ', painted by hand' : '') : 'no weather');
    if (season) parts.push(season.name);
    if (sky) parts.push(SKY[sky.type].label + (sky.name && SKY_ORDER.indexOf(sky.type) >= 0 ? ': ' + sky.name : ''));
    if (this._pinned(k)) parts.push('pinned');
    if (this.state.draft[k]) parts.push('not saved yet');
    return parts.join(' · ');
  };

  Panel.prototype._cellHTML = function (k, focusKey) {
    var S = this.state, cal = this.view.cal, w = this._reading(k), p = parseKey(k);
    var season = this._seasonAt(k), sky = (S.skyCache || {})[k];
    var wc = w ? Chronicle.calendarColor(w.color) : '', sc = season ? Chronicle.calendarColor(season.color) : '';
    var today = p.year === cal.current_year && p.month === cal.current_month && p.day === cal.current_day;
    var label = this._cellLabel(k, w, season, sky);
    var style = (wc ? '--wc:' + wc + ';' : '') + (sc ? '--sc:' + sc + ';' : '');
    return '<button type="button" role="gridcell" class="wxc-d' + (S.sel[k] ? ' sel' : '') + (this._pinned(k) ? ' kept' : '') +
      (today ? ' today' : '') + (w ? '' : ' empty') + (S.draft[k] ? ' chg' : '') + (sky && sky.type === 'blood' ? ' blood' : '') + '"' +
      ' data-key="' + k + '" tabindex="' + (k === focusKey ? '0' : '-1') + '" aria-selected="' + !!S.sel[k] + '"' +
      ' aria-label="' + esc(label) + '" title="' + esc(label) + '"' + (style ? ' style="' + style + '"' : '') + '>' +
      '<span class="n" aria-hidden="true">' + p.day + '</span>' +
      (w ? '<i class="fa-solid ' + Chronicle.calendarWeatherIcon(w) + ' wi" aria-hidden="true"></i>' : '') +
      (sky ? '<span class="sk" aria-hidden="true">' + skyGlyph(sky.type) + '</span>' : '') +
      '<i class="fa-solid fa-thumbtack pin" aria-hidden="true"></i></button>';
  };

  // _renderGrid draws the days under the calendar's weekday names, the first
  // day in its own weekday column. An intercalary month has no weekdays, so
  // its days simply run on.
  Panel.prototype._renderGrid = function (rolled) {
    var S = this.state, cal = this.view.cal, CalDate = Chronicle.calendarDate, md = this._monthDef();
    var grid = $('#cal5-wxcal', this.el), keys = this._keys(), self = this;
    var cols = Math.max(1, CalDate.weekLen(cal)), inter = !!(md.def && md.def.is_intercalary);
    var off = inter ? 0 : CalDate.gridLead(cal, S.y, S.m);
    if (!S.focus || keys.indexOf(S.focus) < 0) {
      var t = dkey(cal.current_year, cal.current_month, cal.current_day);
      S.focus = keys.indexOf(t) >= 0 ? t : (this._chosen()[0] || keys[0]);
    }
    var h = inter ? '' : CalDate.gridWeekdays(cal).map(function (w) {
      return '<div class="wxc-dow" role="columnheader" title="' + esc(w.name) + '">' + esc(String(w.name).slice(0, 3)) + '</div>';
    }).join('');
    for (var b = 0; b < off; b++) h += '<span class="wxc-d blank" aria-hidden="true"></span>';
    h += keys.map(function (k) { return self._cellHTML(k, S.focus); }).join('');
    grid.style.setProperty('--wxcols', cols);
    grid.innerHTML = h;
    if (rolled && !reducedMotion()) rolled.forEach(function (k) { var c = $('[data-key="' + k + '"]', grid); if (c) c.classList.add('roll'); });
  };

  // _refreshCells redraws only the day buttons, keeping keyboard focus.
  Panel.prototype._refreshCells = function (rolled) {
    var had = document.activeElement && document.activeElement.closest && document.activeElement.closest('.wxc-d[data-key]');
    this._renderGrid(rolled);
    if (had) { var again = $('.wxc-d[data-key="' + this.state.focus + '"]', this.el); if (again) again.focus({ preventScroll: true }); }
    this._renderKey();
    this._renderSel();
    this._renderFoot();
    this._preview();
  };

  // _renderKey lists only the marks this month shows.
  Panel.prototype._renderKey = function () {
    var S = this.state, self = this, seen = {};
    Object.keys(S.skyCache || {}).forEach(function (k) { seen[S.skyCache[k].type] = true; });
    var types = ['full', 'new'].concat(SKY_ORDER).filter(function (t) { return seen[t]; });
    var pins = this._keys().some(function (k) { return self._pinned(k); });
    $('#cal5-wxkey', this.el).innerHTML = types.map(function (t) {
      return '<span class="' + (t === 'blood' ? 'blood' : '') + '">' + skyGlyph(t) + SKY[t].label + '</span>';
    }).join('') + (pins ? '<span class="kp"><i class="fa-solid fa-thumbtack" aria-hidden="true"></i>Pinned</span>' : '');
  };

  Panel.prototype._renderSel = function () {
    var S = this.state, self = this, chosen = this._chosen();
    var held = chosen.filter(function (k) { return self._reading(k); });
    var pinned = held.length > 0 && held.every(function (k) { return self._pinned(k); });
    $('#cal5-wxsel', this.el).innerHTML = (chosen.length
      ? '<b>' + plural(chosen.length, 'day') + ' chosen</b>'
      : '<span>Drag across days to choose them, or drag a weather onto a day.</span>') +
      '<span class="sp"></span>' +
      (chosen.length ? '<button type="button" class="btn sm quiet" data-wx-pin title="Pinned days stay when you roll"><i class="fa-solid fa-thumbtack"></i> ' + (pinned ? 'Unpin' : 'Pin') + '</button>' : '') +
      '<button type="button" class="btn sm quiet" data-wx-all>All</button>' +
      (chosen.length ? '<button type="button" class="btn sm quiet" data-wx-none>None</button>' : '');
    var scope = chosen.length ? chosen : this._keys();
    var free = scope.filter(function (k) { return !self._stays(k); }).length;
    $('#cal5-wxnote', this.el).textContent = S.note || ('Rolls ' + plural(free, 'day') + (chosen.length ? ' you chose' : ' this month') +
      ', following each day’s season. Pinned and painted days stay.');
    var roll = $('[data-wx-roll]', this.el);
    if (roll) roll.disabled = !free || S.busy;
  };

  function paletteChip(p, on, added) {
    var c = Chronicle.calendarColor(p.color);
    return '<button type="button" data-wx="' + esc(p.id) + '" class="' + (p.category === 'Fantasy' ? 'mg' : '') + (added ? ' added' : '') + '"' +
      ' aria-pressed="' + !!on + '"' + (c ? ' style="--wc:' + c + '"' : '') + '>' +
      '<i class="fa-solid ' + Chronicle.calendarWeatherIcon({ icon: p.glyph }) + '" aria-hidden="true"></i>' + esc(p.label) + '</button>';
  }

  // _renderPalette: the owner's own kinds first, then the common ones and any
  // picked from More weather…, then Clear and More weather….
  Panel.prototype._renderPalette = function () {
    var S = this.state, self = this, own = this._kinds.map(function (k) { return k.id; });
    var ids = PAINT_COMMON.concat(S.added.filter(function (id) { return PAINT_COMMON.indexOf(id) < 0 && own.indexOf(id) < 0; }));
    var chip = function (id) { var p = self._presets[id]; return p ? paletteChip(p, id === S.last, S.added.indexOf(id) >= 0 && PAINT_COMMON.indexOf(id) < 0) : ''; };
    $('#cal5-wxpal', this.el).innerHTML = own.map(chip).join('') + ids.map(chip).join('') +
      '<button type="button" class="wxc-tool" data-wx-clear><i class="fa-solid fa-eraser" aria-hidden="true"></i>Erase</button>' +
      '<button type="button" class="wxc-tool" data-wx-more aria-expanded="' + !!S.more + '" aria-controls="cal5-wxmore"><i class="fa-solid fa-ellipsis" aria-hidden="true"></i>More weather…</button>';
  };

  // moreListHTML draws the whole weather list under its group headings,
  // filtered by the search text, or a no-match line.
  function moreListHTML(presets, query) {
    var needle = String(query || '').trim().toLowerCase(), html = '';
    var all = Object.keys(presets).map(function (k) { return presets[k]; });
    PAINT_GROUPS.forEach(function (g) {
      var items = all.filter(function (p) { return p.category === g[0] && (!needle || p.label.toLowerCase().indexOf(needle) >= 0); });
      if (!items.length) return;
      html += '<div class="wxgt">' + esc(g[1]) + '</div>' + items.map(function (p) {
        var c = Chronicle.calendarColor(p.color);
        return '<button type="button" class="wxi' + (p.category === 'Fantasy' ? ' mg' : '') + '" role="listitem" data-wx-pick="' + esc(p.id) + '"' + (c ? ' style="--wc:' + c + '"' : '') + '>' +
          '<i class="fa-solid ' + Chronicle.calendarWeatherIcon({ icon: p.glyph }) + '" aria-hidden="true"></i><span class="wxn">' + esc(p.label) + '</span>' +
          (p.category === 'Fantasy' ? '<span class="wmg">Magic</span>' : '') + '</button>';
      }).join('');
    });
    return html || '<p class="wxnone">No weather matches “' + esc(query) + '”.</p>';
  }

  Panel.prototype._renderMore = function () {
    $('#cal5-wxmore .wxl', this.el).innerHTML = moreListHTML(this._presets, this.state.query);
  };

  Panel.prototype._renderFoot = function () {
    var S = this.state, dirty = this.dirty();
    var save = $('[data-wx-save]', this.el), warn = $('.wxc-warn', this.el);
    if (save) { save.disabled = !dirty || S.busy; save.textContent = S.busy ? 'Saving…' : 'Save weather'; }
    if (!dirty) S.nag = false;
    if (warn) warn.hidden = !S.nag;
  };

  // ---- acting ----
  // _say puts a line where the roll note sits, and nudges the choose row
  // when the line asks for days first.
  Panel.prototype._say = function (msg, nudge) {
    var S = this.state;
    S.note = msg;
    this._renderSel();
    S.note = '';
    var row = nudge && $('#cal5-wxsel', this.el);
    if (row && row.animate && !reducedMotion()) row.animate([{ transform: 'translateX(-4px)' }, { transform: 'translateX(4px)' }, { transform: 'none' }], { duration: 260 });
  };

  Panel.prototype._paintChosen = function (id) {
    var chosen = this._chosen();
    if (!chosen.length) {
      this.state.last = id;
      this._renderPalette();
      this._say('Choose days first, or drag the weather onto a day.', true);
      return;
    }
    this.paint(id, chosen);
    this._renderPalette();
    this._refreshCells(chosen);
  };

  Panel.prototype._goMonth = function (step) {
    var S = this.state, view = this.view, self = this, mc = Chronicle.calendarDate.monthCount(view.cal);
    var m = S.m + step, y = S.y;
    if (m < 1) { m = mc; y--; } else if (m > mc) { m = 1; y++; }
    S.y = y; S.m = m; S.sel = {}; S.anchor = null; S.focus = null;
    this._renderMonth();
    Promise.all([view.fetchWeatherYear(y), view.fetchMonth(y, m)]).then(function () {
      if (self.state && self.state.y === y && self.state.m === m) self._renderMonth();
    });
  };

  Panel.prototype._save = function () {
    var S = this.state, self = this;
    if (!S || S.busy || !this.dirty()) return;
    S.busy = true;
    this._renderFoot();
    S.save(this.changes()).then(function (ok) {
      if (!self.state) return;
      if (ok) { self.state.draft = {}; self.state.pins = {}; self.close(); return; }
      self.state.busy = false;
      self._renderFoot();
    });
  };

  // ---- choosing days ----
  Panel.prototype._range = function (a, b) {
    var keys = this._keys(), i = keys.indexOf(a), j = keys.indexOf(b);
    if (i < 0 || j < 0) return [b];
    return keys.slice(Math.min(i, j), Math.max(i, j) + 1);
  };

  // _choose applies a gesture: replace the choice with keys, add them, or
  // (toggle) remove them when they are all chosen already.
  Panel.prototype._choose = function (keys, mode, base) {
    var S = this.state, next = {};
    if (mode !== 'replace') Object.keys(base || S.sel).forEach(function (k) { next[k] = true; });
    var remove = mode === 'toggle' && keys.every(function (k) { return (base || S.sel)[k]; });
    keys.forEach(function (k) { if (remove) delete next[k]; else next[k] = true; });
    S.sel = next;
  };

  Panel.prototype._cellAt = function (x, y) {
    var el = document.elementFromPoint(x, y), c = el && el.closest && el.closest('.wxc-d[data-key]');
    return c && this.el.contains(c) ? c : null;
  };

  // ---- events ----
  Panel.prototype._bind = function () {
    var self = this, el = this.el;
    this.scrim.addEventListener('click', function () { if (self.isOpen()) self.tryClose(); });
    el.addEventListener('animationend', function (e) {
      if (e.target === el) el.classList.remove('nag');
      if (e.target.classList && e.target.classList.contains('roll')) e.target.classList.remove('roll');
    });
    el.addEventListener('keydown', function (e) {
      if (!self.state) return;
      if (e.key === 'Escape') {
        e.stopPropagation();
        if (self.state.more) { self.state.more = false; $('#cal5-wxmore', el).hidden = true; self._renderPalette(); $('[data-wx-more]', el).focus(); return; }
        self.tryClose();
        return;
      }
      var cell = e.target.closest && e.target.closest('.wxc-d[data-key]');
      if (cell) self._gridKey(e, cell);
    });
    el.addEventListener('input', function (e) {
      var S = self.state, t = e.target;
      if (!S) return;
      if (t.id === 'cal5-wxcont') { $('#cal5-wxconto', el).textContent = continuityWords(+t.value); self._continuity = +t.value; self._picked = true; }
      else if (t.id === 'cal5-wxq') { S.query = t.value; self._renderMore(); }
    });
    el.addEventListener('change', function (e) {
      var t = e.target;
      if (!self.state) return;
      if (t.id === 'cal5-wxclim') { self._climate = t.value; self._picked = true; }
      else if (t.id === 'cal5-wxsky') self._skyOwn = t.checked;
    });
    el.addEventListener('click', function (e) {
      var S = self.state, t = e.target.closest('button');
      if (!S || !t || t.classList.contains('wxc-d')) return;
      if (t.hasAttribute('data-wx-close')) { self.tryClose(); return; }
      if (t.hasAttribute('data-wx-cancel') || t.hasAttribute('data-wx-discard')) { self.close(); return; }
      if (S.busy) return;
      if (t.hasAttribute('data-wx-save')) { self._save(); return; }
      if (t.dataset.wxNav) { self._goMonth(+t.dataset.wxNav); return; }
      if (t.hasAttribute('data-wx-all')) { S.sel = {}; self._keys().forEach(function (k) { S.sel[k] = true; }); self._refreshCells(); return; }
      if (t.hasAttribute('data-wx-none')) { S.sel = {}; self._refreshCells(); return; }
      if (t.hasAttribute('data-wx-pin')) {
        if (self.togglePins(self._chosen()) === null) { self._say('Only days with weather can be pinned. Paint or roll them first.'); return; }
        self._refreshCells();
        return;
      }
      if (t.hasAttribute('data-wx-clear')) {
        var chosen = self._chosen();
        if (!chosen.length) { self._say('Choose the days to clear first.', true); return; }
        if (!self.clear(chosen)) { self._say('The chosen days have no weather to clear.'); return; }
        self._refreshCells();
        return;
      }
      if (t.hasAttribute('data-wx-more')) {
        S.more = !S.more;
        $('#cal5-wxmore', el).hidden = !S.more;
        self._renderPalette();
        if (S.more) { self._renderMore(); $('#cal5-wxq', el).focus(); }
        return;
      }
      if (t.dataset.wxPick) {
        var id = t.dataset.wxPick;
        if (S.added.indexOf(id) < 0) S.added.push(id);
        S.more = false;
        $('#cal5-wxmore', el).hidden = true;
        self._paintChosen(id);
        var back = $('[data-wx="' + id + '"]', el);
        if (back) back.focus({ preventScroll: true });
        return;
      }
      if (t.dataset.wx) {
        if (self._dragged) { self._dragged = false; return; }
        self._paintChosen(t.dataset.wx);
        return;
      }
      if (t.hasAttribute('data-wx-roll')) {
        var out = self.roll();
        if (out.error) { self._say(out.error); return; }
        self._refreshCells(out.rolled);
        if (!out.rolled.length) self._say('Every one of those days is pinned or painted, so there is nothing to roll.');
      }
    });
    this._bindGridPointer();
    this._bindPaletteDrag();
  };

  // Press and drag across days to choose them. Shift extends from the last
  // day pressed, Ctrl/Cmd adds or removes; a plain press on the only chosen
  // day lets it go. The pointer is captured, so the drag follows it across
  // the grid without a document listener.
  Panel.prototype._bindGridPointer = function () {
    var self = this, grid = function () { return $('#cal5-wxcal', self.el); }, drag = null;
    this.el.addEventListener('pointerdown', function (e) {
      var S = self.state, c = e.target.closest && e.target.closest('.wxc-d[data-key]');
      if (!S || !c || S.busy || e.button > 0) return;
      e.preventDefault();
      var k = c.dataset.key, add = e.ctrlKey || e.metaKey;
      var only = !add && !e.shiftKey && S.sel[k] && self._chosen().length === 1;
      drag = { a: e.shiftKey && S.anchor ? S.anchor : k, add: add, base: Object.assign({}, S.sel), only: only, moved: false, z: k, id: e.pointerId };
      if (!e.shiftKey) S.anchor = k;
      S.focus = k;
      if (only) S.sel = {};
      else self._choose(self._range(drag.a, k), add ? 'toggle' : 'replace', drag.base);
      try { grid().setPointerCapture(e.pointerId); } catch (err) { /* capture is a nicety */ }
      self._refreshCells();
      var f = $('.wxc-d[data-key="' + k + '"]', self.el);
      if (f) f.focus({ preventScroll: true });
    });
    this.el.addEventListener('pointermove', function (e) {
      if (!drag || e.pointerId !== drag.id || !self.state) return;
      var c = self._cellAt(e.clientX, e.clientY);
      if (!c || c.dataset.key === drag.z) return;
      drag.z = c.dataset.key;
      drag.moved = true;
      self.state.focus = drag.z;
      self._choose(self._range(drag.a, drag.z), drag.add ? 'toggle' : 'replace', drag.base);
      self._refreshCells();
    });
    var end = function (e) { if (drag && e.pointerId === drag.id) drag = null; };
    this.el.addEventListener('pointerup', end);
    this.el.addEventListener('pointercancel', end);
  };

  // Arrow keys move between days, Home and End go to the month's ends, Space
  // or Enter chooses or lets go of a day, and Shift with an arrow extends.
  Panel.prototype._gridKey = function (e, cell) {
    var S = this.state, keys = this._keys(), i = keys.indexOf(cell.dataset.key);
    var cols = Math.max(1, Chronicle.calendarDate.weekLen(this.view.cal)), j = i;
    if (e.key === 'ArrowLeft') j = i - 1;
    else if (e.key === 'ArrowRight') j = i + 1;
    else if (e.key === 'ArrowUp') j = i - cols;
    else if (e.key === 'ArrowDown') j = i + cols;
    else if (e.key === 'Home') j = 0;
    else if (e.key === 'End') j = keys.length - 1;
    else if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      this._choose([keys[i]], 'toggle');
      S.anchor = keys[i];
      this._refreshCells();
      return;
    } else return;
    e.preventDefault();
    j = Math.max(0, Math.min(keys.length - 1, j));
    S.focus = keys[j];
    // A plain move carries the anchor along, so Shift extends from where the
    // keyboard is rather than from an older press.
    if (e.shiftKey) { if (!S.anchor) S.anchor = keys[i]; this._choose(this._range(S.anchor, keys[j]), 'replace'); }
    else S.anchor = keys[j];
    this._refreshCells();
  };

  // A weather can be dragged from the palette onto a day: onto a chosen day
  // it paints every chosen day, onto any other only that one.
  Panel.prototype._bindPaletteDrag = function () {
    var self = this, pd = null;
    var stop = function () {
      if (!pd) return;
      if (pd.ghost) pd.ghost.remove();
      if (pd.over) pd.over.classList.remove('drop');
      pd = null;
    };
    this.el.addEventListener('pointerdown', function (e) {
      var b = e.target.closest && e.target.closest('#cal5-wxpal [data-wx]');
      if (!b || !self.state || self.state.busy || e.button > 0) return;
      pd = { id: b.dataset.wx, x: e.clientX, y: e.clientY, on: false, el: b, pid: e.pointerId, over: null, ghost: null };
      self._dragged = false;
    });
    this.el.addEventListener('pointermove', function (e) {
      if (!pd || e.pointerId !== pd.pid) return;
      if (!pd.on) {
        if (Math.abs(e.clientX - pd.x) + Math.abs(e.clientY - pd.y) < 7) return;
        pd.on = true;
        try { pd.el.setPointerCapture(pd.pid); } catch (err) { /* the drag still works while over the panel */ }
        var p = self._presets[pd.id], c = p && Chronicle.calendarColor(p.color);
        pd.ghost = document.createElement('div');
        pd.ghost.className = 'wxc-ghost';
        pd.ghost.setAttribute('aria-hidden', 'true');
        pd.ghost.innerHTML = '<i class="fa-solid ' + Chronicle.calendarWeatherIcon({ icon: p ? p.glyph : '' }) + '"' + (c ? ' style="color:' + c + '"' : '') + '></i>' + esc(p ? p.label : '');
        self.el.appendChild(pd.ghost);
      }
      pd.ghost.style.left = e.clientX + 'px';
      pd.ghost.style.top = e.clientY + 'px';
      var cell = self._cellAt(e.clientX, e.clientY);
      if (pd.over && pd.over !== cell) pd.over.classList.remove('drop');
      pd.over = cell;
      if (cell) cell.classList.add('drop');
    });
    this.el.addEventListener('pointerup', function (e) {
      if (!pd || e.pointerId !== pd.pid) return;
      var p = pd;
      if (p.on) {
        self._dragged = true;
        var k = p.over && p.over.dataset.key;
        stop();
        if (k && self.state) {
          var keys = self.state.sel[k] ? self._chosen() : [k];
          self.paint(p.id, keys);
          self._renderPalette();
          self._refreshCells(keys);
        }
        return;
      }
      pd = null;
    });
    this.el.addEventListener('pointercancel', stop);
  };

  window.Chronicle = window.Chronicle || {};
  // calendarWeatherSettings reads the calendar's Weather settings once per
  // page for the weather calendar; resolves null (and asks again next time)
  // when they can't be read. Calendar settings is its own page, so a change
  // there reloads this one.
  Chronicle.calendarWeatherSettings = function (view) {
    if (!view._wxSettings) {
      view._wxSettings = Chronicle.apiFetch(view.apiBase + '/weather/settings').then(function (resp) {
        return resp.ok ? resp.json() : null;
      }).catch(function () { return null; }).then(function (s) {
        if (!s) view._wxSettings = null;
        return s;
      });
    }
    return view._wxSettings;
  };
  Chronicle.calendarWeatherOwnKinds = ownKinds;
  Chronicle.calendarWeatherSheet = {
    open: function (view, opts) {
      if (!view._weatherSheet) view._weatherSheet = new Panel(view);
      view._weatherSheet.open(opts);
    },
    isOpen: function (view) { return !!(view._weatherSheet && view._weatherSheet.isOpen()); }
  };
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { Panel: Panel, PAINT_COMMON: PAINT_COMMON, PAINT_GROUPS: PAINT_GROUPS, continuityWords: continuityWords, asEngineDay: asEngineDay, ownKinds: ownKinds, moonPeaks: moonPeaks, moreListHTML: moreListHTML, paletteChip: paletteChip, SKY: SKY };
  }
})();
