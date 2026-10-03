/**
 * calendar_weather_sheet.js — the Generate sheet for day weather, opened
 * from the edit bar's Generate… button (calendar_editor.js) for the chosen
 * days. The signed recipe sheet, weather only: pick a climate and how long
 * weather lasts, and the result previews as cards here and as faint marks
 * on the grid (view.wxPreview). The owner keeps the days they like and
 * rerolls the rest, or one day at a time, then applies.
 *
 * Days painted by hand are never replaced: they go to the generator as
 * locked, so the days around them ease into them, and Apply leaves them out.
 * Saving and Undo belong to the editor (opts.apply); this file only shapes
 * what would be saved. The weather itself is chronicle_gen.js, already
 * loaded by the editor before this opens.
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

  // Cards shown before "Show all", so a long range stays scannable.
  var SHOWN = 8;

  function dkey(d) { return d.year + '_' + d.month + '_' + d.day; }
  function painted(w) { return !!w && w.source !== 'generated'; }
  function randomSeed() { return Math.random().toString(36).slice(2, 8); }

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

  function Sheet(view) {
    this.view = view;
    var dock = view.dockEl;
    this.scrim = $('.dscrim', dock);
    if (!this.scrim) { this.scrim = document.createElement('div'); this.scrim.className = 'dscrim'; dock.appendChild(this.scrim); }
    this.el = document.createElement('aside');
    this.el.className = 'drawer ded gsheet';
    this.el.setAttribute('role', 'dialog');
    this.el.setAttribute('aria-labelledby', 'cal5-gstitle');
    this.el.tabIndex = -1;
    dock.appendChild(this.el);
    this.state = null;
    this._climate = 'temperate';
    this._continuity = 0.55;
    this._bind();
  }

  Sheet.prototype.isOpen = function () { return this.el.classList.contains('open'); };

  // opts: {dates: [{year, month, day}], stored: {'y_m_d': reading} for
  // every loaded day of the years touched, apply: function (days) -> Promise}.
  Sheet.prototype.open = function (opts) {
    var view = this.view, back = document.activeElement;
    view.closeAllPanels({ instant: true });
    var CalDate = Chronicle.calendarDate, cal = view.cal;
    var dates = opts.dates.slice().sort(function (a, b) {
      return CalDate.dayIndex(cal, a.year, a.month, a.day) - CalDate.dayIndex(cal, b.year, b.month, b.day);
    });
    var inScope = {};
    dates.forEach(function (d) { inScope[dkey(d)] = true; });
    var stored = opts.stored || {};
    this.state = {
      dates: dates, stored: stored, apply: opts.apply, back: back,
      context: Object.keys(stored).filter(function (k) { return !inScope[k]; }).map(function (k) { return asEngineDay(stored[k]); }),
      seed: randomSeed(), nonce: 0, kept: {}, days: {}, summary: '', warnings: [], error: null, showAll: false, busy: false
    };
    this._generate();
    this._render();
    void this.el.offsetWidth;
    this.el.classList.add('open');
    view.dockEl.classList.add('on');
    var first = $('#cal5-gsClim', this.el) || this.el;
    setTimeout(function () { first.focus({ preventScroll: true }); }, reducedMotion() ? 20 : 260);
  };

  Sheet.prototype.close = function (quiet) {
    if (!this.isOpen()) return;
    var back = this.state && this.state.back;
    this.el.classList.remove('open');
    this.view.dockEl.classList.remove('on');
    this.state = null;
    this._preview();
    if (!quiet && back && back.isConnected) back.focus({ preventScroll: true });
  };

  // ---- generating ----
  Sheet.prototype._recipe = function () {
    return { generator: 'weather', details: { climate: this._climate, continuity: this._continuity } };
  };

  Sheet.prototype._isPainted = function (k) { return painted(this.state.stored[k]); };

  // Painted days, plus kept days when keepKept, as the generator's locks.
  // A painted kind the generator doesn't know is left out of the locks (it
  // would refuse the run); Apply still leaves that day alone.
  Sheet.prototype._locks = function (keepKept) {
    var S = this.state, self = this, known = {};
    window.ChronicleGen.weather.presets().forEach(function (p) { known[p.id] = true; });
    return S.dates.map(dkey).filter(function (k) {
      return self._isPainted(k) || (keepKept && S.kept[k] && S.days[k]);
    }).map(function (k) {
      return self._isPainted(k) ? asEngineDay(S.stored[k]) : S.days[k];
    }).filter(function (d) { return !d.preset_id || known[d.preset_id]; });
  };

  // _generate runs the whole range with the current recipe and seed, keeping
  // painted days and any kept ones.
  Sheet.prototype._generate = function () {
    var S = this.state, view = this.view;
    try {
      var res = window.ChronicleGen.run('weather', {
        calendar: view.cal, recipe: this._recipe(), seed: S.seed,
        scope: { days: S.dates }, locked: this._locks(true), context: { weather: S.context }
      });
      this._take(res.days);
      S.summary = res.summary;
      S.warnings = res.warnings || [];
      S.error = null;
    } catch (e) {
      S.days = {};
      S.summary = '';
      S.error = e && e.friendly ? e.message : 'The weather could not be generated for this calendar.';
    }
    this._preview();
  };

  // _reroll draws fresh weather for the given days (default: every day not
  // kept or painted), meeting the days either side without a jump.
  Sheet.prototype._reroll = function (keys) {
    var S = this.state, self = this, view = this.view;
    var targets = (keys || S.dates.map(dkey)).filter(function (k) { return !S.kept[k] && !self._isPainted(k); });
    if (!targets.length || S.error) return;
    var existing = S.dates.map(dkey).map(function (k) {
      var d = self._isPainted(k) ? asEngineDay(S.stored[k]) : S.days[k];
      if (!d) return null;
      var c = {};
      Object.keys(d).forEach(function (f) { c[f] = d[f]; });
      c.gen = {};
      Object.keys(d.gen || {}).forEach(function (f) { c.gen[f] = d.gen[f]; });
      c.gen.locked = targets.indexOf(k) < 0;
      return c;
    }).filter(Boolean).concat(S.context);
    try {
      var rr = window.ChronicleGen.mass.reroll({
        generator: 'weather', calendar: view.cal, recipe: this._recipe(), seed: S.seed,
        scope: { days: targets.map(function (k) { var p = k.split('_').map(Number); return { year: p[0], month: p[1], day: p[2] }; }) },
        existing: existing, nonce: 'r' + (++S.nonce)
      });
      var fresh = {};
      targets.forEach(function (k) { fresh[k] = true; });
      this._take(rr.items.filter(function (d) { return fresh[dkey(d)]; }));
      S.summary = rr.summary;
    } catch (e) {
      view.say('Couldn’t reroll those days. The preview is unchanged.');
    }
    this._preview();
  };

  Sheet.prototype._take = function (days) {
    var S = this.state;
    days.forEach(function (d) { S.days[dkey(d)] = d; });
  };

  // _preview hands the unsaved readings to the grid as faint marks.
  Sheet.prototype._preview = function () {
    var view = this.view, S = this.state, self = this;
    if (!S) { view.wxPreview = null; }
    else {
      view.wxPreview = {};
      S.dates.map(dkey).forEach(function (k) {
        if (S.days[k] && !self._isPainted(k)) view.wxPreview[k] = S.days[k];
      });
    }
    if (view._paintMonth) view._paintMonth();
  };

  // ---- applying ----
  Sheet.prototype._applyDays = function () {
    var S = this.state, self = this, G = window.ChronicleGen;
    return S.dates.map(dkey).filter(function (k) { return S.days[k] && !self._isPainted(k); }).map(function (k) {
      var d = S.days[k], body = G.weather.toWeatherInput(d);
      body.year = d.year; body.month = d.month; body.day = d.day; body.source = 'generated';
      return body;
    });
  };

  Sheet.prototype._apply = function () {
    var S = this.state, self = this;
    if (!S || S.busy) return;
    var days = this._applyDays();
    if (!days.length) { this.close(); return; }
    S.busy = true;
    this._render();
    S.apply(days).then(function (ok) {
      if (!self.state) return;
      if (ok) { self.close(); return; }
      self.state.busy = false;
      self._render();
    });
  };

  // ---- markup ----
  Sheet.prototype._dateText = function (d) {
    var mo = (this.view.cal.months || [])[d.month - 1];
    return d.day + ' ' + (mo ? mo.name : 'Month ' + d.month);
  };

  Sheet.prototype._cardHTML = function (k) {
    var S = this.state, view = this.view, cal = view.cal;
    var p = k.split('_').map(Number), date = { year: p[0], month: p[1], day: p[2] };
    var hand = this._isPainted(k), w = hand ? S.stored[k] : S.days[k];
    if (!w) return '';
    var c = Chronicle.calendarColor(w.color), kept = !hand && !!S.kept[k];
    var CalDate = Chronicle.calendarDate;
    var future = CalDate.dayIndex(cal, p[0], p[1], p[2]) > CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
    var label = w.preset_label || w.description || 'Weather', when = this._dateText(date);
    var facts = [when];
    if (w.temperature_celsius != null) facts.push(w.temperature_celsius + '°C');
    var wind = Chronicle.calendarWindWords(w.wind);
    if (wind) facts.push(wind);
    if (hand) facts.push('painted by hand, stays');
    return '<div class="rc' + (kept ? ' kept' : '') + (future ? ' dir' : '') + '" data-key="' + esc(k) + '">' +
      '<span class="rci"' + (c ? ' style="color:' + c + '"' : '') + '><i class="fa-solid ' + Chronicle.calendarWeatherIcon(w) + ' i" aria-hidden="true"></i></span>' +
      '<span class="rct"><b>' + esc(label) + '</b><small>' + esc(facts.join(' · ')) + '</small></span>' +
      '<span class="rca">' + (hand ? '' :
        '<button type="button" data-rc="keep" aria-pressed="' + kept + '" aria-label="Keep ' + esc(label) + ' on ' + esc(when) + '" title="Keep"><i class="fa-solid fa-thumbtack i" aria-hidden="true"></i></button>' +
        '<button type="button" data-rc="reroll" aria-label="Reroll ' + esc(when) + '" title="Reroll"' + (kept ? ' disabled' : '') + '><i class="fa-solid fa-dice i" aria-hidden="true"></i></button>') +
      '</span></div>';
  };

  Sheet.prototype._render = function () {
    var S = this.state, self = this;
    if (!S) return;
    var climates = window.ChronicleGen.weather.climates(), cur = this._climate;
    var blurb = (climates.filter(function (c) { return c.id === cur; })[0] || {}).blurb || '';
    var keys = S.dates.map(dkey), n = keys.length;
    var handN = keys.filter(function (k) { return self._isPainted(k); }).length;
    var keptN = keys.filter(function (k) { return S.kept[k] && !self._isPainted(k); }).length;
    var applyN = this._applyDays().length;
    var shown = S.showAll ? keys : keys.slice(0, SHOWN);
    var opt = function (c) { return '<option value="' + esc(c.id) + '"' + (c.id === cur ? ' selected' : '') + '>' + esc(c.name) + '</option>'; };
    var h = '<div class="grab" aria-hidden="true"></div><div class="crease gcr"><span id="cal5-gstitle">Generate weather</span>' +
      '<span class="sub">' + (n === 1 ? '1 DAY' : n + ' DAYS') + '</span>' +
      '<button type="button" class="x" data-close aria-label="Close"><i class="fa-solid fa-xmark"></i></button></div>';
    h += '<div class="gbody">' +
      '<div class="grec"><label class="fld">Climate<select id="cal5-gsClim">' +
        '<optgroup label="Natural">' + climates.filter(function (c) { return !c.magic; }).map(opt).join('') + '</optgroup>' +
        '<optgroup label="Magic">' + climates.filter(function (c) { return c.magic; }).map(opt).join('') + '</optgroup>' +
      '</select></label></div>' +
      (blurb ? '<p class="gdesc">' + esc(blurb) + '</p>' : '') +
      '<div class="gdet"><label class="fld">How long weather lasts<span class="dv">' +
        '<input type="range" id="cal5-gsCont" min="0" max="1" step="0.05" value="' + this._continuity + '">' +
        '<output id="cal5-gsContO">' + continuityWords(this._continuity) + '</output></span>' +
        '<span class="dh">Left changes every day; right settles into long spells.</span></label></div>';
    h += '<section class="gsec"><h4>Preview</h4>';
    if (S.error) {
      h += '<p class="gnone" role="alert">' + esc(S.error) + '</p>';
    } else {
      h += '<div class="gpv"><span><b>' + (n === 1 ? '1 day' : n + ' days') + '</b>' +
        (keptN ? ' · ' + keptN + ' kept' : '') + (handN ? ' · ' + handN + ' painted' : '') + '</span><span class="sp"></span>' +
        '<button type="button" class="btn sm quiet" data-gs="rest"' + (n - keptN - handN > 0 ? '' : ' disabled') + '><i class="fa-solid fa-dice"></i> Reroll the rest</button></div>' +
        '<div class="rcs" id="cal5-gsCards">' + shown.map(function (k) { return self._cardHTML(k); }).join('') + '</div>' +
        (n > SHOWN && !S.showAll ? '<button type="button" class="btn sm quiet rcmore" data-gs="all">Show all ' + n + ' days</button>' : '') +
        '<div class="gseed"><label for="cal5-gsSeed">Seed</label><input id="cal5-gsSeed" value="' + esc(S.seed) + '" maxlength="40" spellcheck="false">' +
        '<button type="button" class="x" data-gs="seed" aria-label="New seed" title="New seed"><i class="fa-solid fa-shuffle i"></i></button>' +
        '<span>The same seed gives the same weather.</span></div>';
    }
    h += '</section>';
    if (!S.error && S.summary) {
      h += '<p class="gsum">' + esc(S.summary) +
        (handN ? '<span class="gw">Days you painted by hand stay as they are.</span>' : '') +
        S.warnings.map(function (w) { return '<span class="gw">' + esc(w) + '</span>'; }).join('') + '</p>';
    }
    h += '</div><div class="efoot gfoot"><span class="gfa">' +
      '<button type="button" class="btn quiet" data-close>Cancel</button>' +
      '<button type="button" class="btn primary" data-gs="apply"' + (S.busy || S.error || !applyN ? ' disabled' : '') + '>' +
        (S.busy ? 'Saving…' : 'Apply to ' + (applyN === 1 ? '1 day' : applyN + ' days')) + '</button></span></div>';
    var focusId = document.activeElement && this.el.contains(document.activeElement) ? document.activeElement.id : null;
    var focusRc = document.activeElement && document.activeElement.closest && document.activeElement.closest('.rc [data-rc]');
    var focusRcSel = focusRc ? '.rc[data-key="' + focusRc.closest('.rc').dataset.key + '"] [data-rc="' + focusRc.dataset.rc + '"]' : null;
    this.el.innerHTML = h;
    // Re-rendering replaces the controls; keep the keyboard where it was.
    var again = (focusId && $('#' + focusId, this.el)) || (focusRcSel && $(focusRcSel, this.el));
    if (again && !again.disabled) again.focus({ preventScroll: true });
    else if (focusId || focusRcSel) this.el.focus({ preventScroll: true });
  };

  // ---- events ----
  Sheet.prototype._bind = function () {
    var self = this, el = this.el;
    this.scrim.addEventListener('click', function () { if (self.isOpen()) self.close(); });
    el.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && self.isOpen()) { e.stopPropagation(); self.close(); }
    });
    el.addEventListener('input', function (e) {
      if (e.target.id === 'cal5-gsCont') $('#cal5-gsContO', el).textContent = continuityWords(+e.target.value);
    });
    el.addEventListener('change', function (e) {
      var S = self.state, t = e.target;
      if (!S || S.busy) return;
      if (t.id === 'cal5-gsClim') self._climate = t.value;
      else if (t.id === 'cal5-gsCont') self._continuity = +t.value;
      else if (t.id === 'cal5-gsSeed') S.seed = t.value.trim() || randomSeed();
      else return;
      self._generate();
      self._render();
    });
    el.addEventListener('click', function (e) {
      var t = e.target.closest('button'), S = self.state;
      if (!t || !S) return;
      if (t.hasAttribute('data-close')) { self.close(); return; }
      if (S.busy) return;
      var a = t.dataset.gs, rc = t.dataset.rc, key = rc && t.closest('.rc').dataset.key;
      if (a === 'apply') { self._apply(); return; }
      if (a === 'rest') self._reroll();
      else if (a === 'all') S.showAll = true;
      else if (a === 'seed') { S.seed = randomSeed(); self._generate(); }
      else if (rc === 'keep') S.kept[key] = !S.kept[key];
      else if (rc === 'reroll') self._reroll([key]);
      else return;
      self._render();
      // A keep lands with a brief ring, so the pin reads as taken.
      var card = rc === 'keep' && S.kept[key] && $('.rc[data-key="' + key + '"]', el);
      if (card && !reducedMotion()) card.classList.add('pulse');
    });
  };

  window.Chronicle = window.Chronicle || {};
  Chronicle.calendarWeatherSheet = {
    open: function (view, opts) {
      if (!view._weatherSheet) view._weatherSheet = new Sheet(view);
      view._weatherSheet.open(opts);
    },
    isOpen: function (view) { return !!(view._weatherSheet && view._weatherSheet.isOpen()); }
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = { Sheet: Sheet, continuityWords: continuityWords, asEngineDay: asEngineDay };
})();
