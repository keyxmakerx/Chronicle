/**
 * calendar_rule.js — the pure side of "repeat by a rule" in the event
 * drawer: the ready-made rules, the map between the drawer's form model and
 * the events API's recurrence_rule, the plain-sentence summary and the
 * checks run before saving. No DOM and no network, so test/js/
 * calendar_rule.test.mjs runs it as is; calendar_event_drawer.js owns the
 * markup and the preview request.
 *
 * Form model (what the drawer edits):
 *   { conds: [cond...], every: 1, shift: 0, dir: 'later' | 'earlier' }
 * with one cond per row:
 *   { k: 'moon',   ph: 'full', moon: <moon id> }
 *   { k: 'wd',     wds: [<0-based weekday>, ...] }
 *   { k: 'nth',    n: 1..5 | -1, wd: <weekday> }
 *   { k: 'dom',    d: 1.. | -1 }
 *   { k: 'month',  m: <1-based month> }
 *   { k: 'season', season: <season id> }
 *   { k: 'event',  event: <event id> }
 *   { k: 'raw',    raw: <API condition> }
 * 'raw' carries a condition the API accepts (importers write them) but the
 * rows have no fields for: it is shown as a sentence, can be removed, and
 * is sent back unchanged. The API's own kinds are never extended here.
 *
 * The API's weekday and month numbers are 0-based and 1-based; the form
 * keeps the same numbers so nothing is converted twice.
 */
(function () {
  'use strict';

  // The API's phase names, with how a sentence says each one.
  var PHASES = [
    ['new', 'new moon', 'a new moon'],
    ['first_quarter', 'first quarter', 'a first quarter'],
    ['full', 'full moon', 'a full moon'],
    ['last_quarter', 'last quarter', 'a last quarter']
  ];
  // The API's bounds (recurrence_rule.go): six conditions, every 1..99,
  // a shift within a year.
  var MAX_CONDS = 6;
  var MAX_EVERY = 99;
  var MAX_SHIFT = 365;

  var KIND_ORDER = ['moon', 'wd', 'nth', 'dom', 'month', 'season', 'event'];
  var KIND_LABELS = {
    moon: 'the moon is', wd: 'the weekday is', nth: 'it’s the', dom: 'the day of the month is',
    month: 'the month is', season: 'it’s the first day of', event: 'it’s the day of'
  };
  var ORDINALS = [[1, '1st'], [2, '2nd'], [3, '3rd'], [4, '4th'], [5, '5th'], [-1, 'last']];

  function ordinal(n) {
    var t = n % 100;
    if (t >= 11 && t <= 13) return n + 'th';
    return n + ({ 1: 'st', 2: 'nd', 3: 'rd' }[n % 10] || 'th');
  }
  function phaseRow(key) {
    for (var i = 0; i < PHASES.length; i++) if (PHASES[i][0] === key) return PHASES[i];
    return PHASES[2];
  }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }
  function intIn(v, lo, hi, dflt) {
    var n = parseInt(v, 10);
    return isNaN(n) ? dflt : Math.max(lo, Math.min(hi, n));
  }

  // ---- what the calendar offers -------------------------------------
  // env: { cal, events: [{id, name}], hiddenOk, eventName(id) }. hiddenOk is
  // whether the viewer may name a moon hidden from players, as the server
  // decides it when the rule is saved.
  function moonsOf(env) {
    return ((env.cal && env.cal.moons) || []).filter(function (m) { return env.hiddenOk || !m.hidden_from_players; });
  }
  function seasonsOf(env) { return (env.cal && env.cal.seasons) || []; }
  function weekdaysOf(env) { return (env.cal && env.cal.weekdays) || []; }
  function monthsOf(env) { return (env.cal && env.cal.months) || []; }
  function weekLen(env) { return weekdaysOf(env).length || 7; }
  function wdName(env, i) {
    var w = weekdaysOf(env)[i];
    return w ? w.name : 'day ' + (i + 1);
  }
  function monthName(env, m) {
    var mo = monthsOf(env)[m - 1];
    return mo ? mo.name : 'month ' + m;
  }
  function moonName(env, id) {
    var m = ((env.cal && env.cal.moons) || []).filter(function (x) { return String(x.id) === String(id); })[0];
    return m ? m.name : 'a moon';
  }
  function seasonName(env, id) {
    var s = seasonsOf(env).filter(function (x) { return String(x.id) === String(id); })[0];
    return s ? s.name : 'a season';
  }
  function eventName(env, id) {
    var e = (env.events || []).filter(function (x) { return x.id === id; })[0];
    if (e) return e.name;
    return (env.eventName && env.eventName(id)) || 'another event';
  }
  // The longest month, so the day-of-month list reaches every real day.
  function maxMonthDays(env) {
    var top = 0;
    monthsOf(env).forEach(function (m) { top = Math.max(top, (m.days || 0) + (m.leap_year_days || 0)); });
    return Math.max(top, 28);
  }

  // Which condition kinds the picker offers: moons and seasons only when
  // the calendar has some, events only when there is one to name, and any
  // kind the rule already uses so an existing row never loses its option.
  function availableKinds(env, form) {
    var used = {};
    ((form && form.conds) || []).forEach(function (c) { used[c.k] = true; });
    return KIND_ORDER.filter(function (k) {
      if (used[k]) return true;
      if (k === 'moon') return moonsOf(env).length > 0;
      if (k === 'season') return seasonsOf(env).length > 0;
      if (k === 'event') return (env.events || []).length > 0;
      return true;
    });
  }

  // A new row of a kind, filled with something valid for this calendar.
  // start: {y, m, d} the event starts on, so a weekday or month row opens
  // on the start date's own.
  function defaultCond(kind, env, start) {
    start = start || {};
    switch (kind) {
      case 'moon': return { k: 'moon', ph: 'full', moon: (moonsOf(env)[0] || {}).id };
      case 'wd': return { k: 'wd', wds: [Math.min(Math.max(start.wd || 0, 0), weekLen(env) - 1)] };
      case 'nth': return { k: 'nth', n: 1, wd: Math.min(Math.max(start.wd || 0, 0), weekLen(env) - 1) };
      case 'dom': return { k: 'dom', d: Math.min(Math.max(start.d || 1, 1), maxMonthDays(env)) };
      case 'month': return { k: 'month', m: Math.min(Math.max(start.m || 1, 1), monthsOf(env).length || 1) };
      case 'season': return { k: 'season', season: (seasonsOf(env)[0] || {}).id };
      case 'event': return { k: 'event', event: ((env.events || [])[0] || {}).id };
    }
    return { k: 'wd', wds: [0] };
  }

  // ---- form <-> API --------------------------------------------------
  function condFromAPI(c) {
    switch (c.kind) {
      case 'moon_phase': return { k: 'moon', ph: c.phase, moon: c.moon_id };
      case 'weekday': return { k: 'wd', wds: (c.weekdays ? c.weekdays.slice() : [c.weekday]).sort(function (a, b) { return a - b; }) };
      case 'nth_weekday': return { k: 'nth', n: c.n, wd: c.weekday };
      case 'day_of_month': return { k: 'dom', d: c.day };
      case 'month': return { k: 'month', m: c.month };
      case 'months': if (c.months && c.months.length === 1) return { k: 'month', m: c.months[0] }; break;
      case 'season_start': return { k: 'season', season: c.season_id };
      case 'relative_to_event': return { k: 'event', event: c.event_id };
    }
    return { k: 'raw', raw: clone(c) };
  }
  function condToAPI(c) {
    switch (c.k) {
      case 'moon': return { kind: 'moon_phase', moon_id: c.moon, phase: c.ph };
      case 'wd':
        var w = c.wds.slice().sort(function (a, b) { return a - b; });
        return w.length === 1 ? { kind: 'weekday', weekday: w[0] } : { kind: 'weekday', weekdays: w };
      case 'nth': return { kind: 'nth_weekday', n: c.n, weekday: c.wd };
      case 'dom': return { kind: 'day_of_month', day: c.d };
      case 'month': return { kind: 'month', month: c.m };
      case 'season': return { kind: 'season_start', season_id: c.season };
      case 'event': return { kind: 'relative_to_event', event_id: c.event };
      case 'raw': return clone(c.raw);
    }
    return null;
  }

  // The API's rule as a form. Never fails: whatever the form has no row for
  // becomes a 'raw' row.
  function fromRule(rule) {
    var off = rule && rule.offset_days ? rule.offset_days : 0;
    return {
      conds: ((rule && rule.match) || []).map(condFromAPI),
      every: rule && rule.every > 0 ? rule.every : 1,
      shift: Math.abs(off),
      dir: off < 0 ? 'earlier' : 'later'
    };
  }
  // The form as the API's rule. every and offset_days are left out at their
  // defaults, as the API stores them.
  function toRule(form) {
    var rule = { match: form.conds.map(condToAPI) };
    if (form.every > 1) rule.every = form.every;
    if (form.shift) rule.offset_days = form.dir === 'earlier' ? -form.shift : form.shift;
    return rule;
  }
  function sameRule(a, b) { return JSON.stringify(toRule(a)) === JSON.stringify(toRule(b)); }

  // ---- the plain sentence --------------------------------------------
  function joinOr(list) {
    if (list.length < 2) return list.join('');
    return list.slice(0, -1).join(', ') + ' or ' + list[list.length - 1];
  }
  function wdList(env, wds) {
    if (wds.length >= weekLen(env)) return 'day';
    return joinOr(wds.map(function (w) { return wdName(env, w); }));
  }
  // What a row says it matches, as the thing that repeats.
  function noun(c, env) {
    switch (c.k) {
      case 'moon': return phaseRow(c.ph)[1] + ' of ' + moonName(env, c.moon);
      case 'wd': return wdList(env, c.wds);
      case 'nth': return (c.n === -1 ? 'last' : ordinal(c.n)) + ' ' + wdName(env, c.wd) + ' of the month';
      case 'dom': return c.d === -1 ? 'last day of the month' : ordinal(c.d) + ' of the month';
      case 'month': return 'day in ' + monthName(env, c.m);
      case 'season': return 'first day of ' + seasonName(env, c.season);
      case 'event': return eventName(env, c.event);
      case 'raw': return rawNoun(c.raw, env);
    }
    return 'day';
  }
  function rawNoun(r, env) {
    switch (r.kind) {
      case 'months': return 'day in ' + joinOr((r.months || []).map(function (m) { return monthName(env, m); }));
      case 'season': return 'day in ' + seasonName(env, r.season_id);
      case 'after_event':
        var n = Math.abs(r.days || 0);
        return (r.days ? n + ' day' + (n === 1 ? '' : 's') + (r.days < 0 ? ' before ' : ' after ') : 'same day as ') + eventName(env, r.event_id);
    }
    return 'day';
  }
  // A later row only narrows the first: "on a Starday", "in Harvestide".
  function filter(c, env) {
    switch (c.k) {
      case 'moon': return 'on a ' + noun(c, env);
      case 'wd': return 'on a ' + wdList(env, c.wds);
      case 'month': return 'in ' + monthName(env, c.m);
      case 'event': return 'on the same day as ' + eventName(env, c.event);
      case 'raw':
        if (c.raw.kind === 'months' || c.raw.kind === 'season') return 'in ' + noun(c, env).replace(/^day in /, '');
        return 'on ' + noun(c, env);
    }
    return 'on the ' + noun(c, env);
  }
  function lowerFirst(s) { return s.charAt(0).toLowerCase() + s.slice(1); }
  function upperFirst(s) { return s.charAt(0).toUpperCase() + s.slice(1); }

  // "Every full moon of Luna.", "The 3rd Kingsday of every month.",
  // "2 days after every full moon of Luna, only when it falls on a Starday."
  function summary(form, env) {
    if (!form.conds.length) return 'Add a condition to start.';
    var ev = form.every > 1 ? form.every : 1, c0 = form.conds[0], s;
    var eventFirst = c0.k === 'event';
    if (eventFirst) {
      var name = noun(c0, env);
      if (ev > 1) s = 'every ' + ordinal(ev) + ' time ' + name + ' happens';
      else if (form.shift) s = name;
      else s = 'the same days as ' + name;
    } else if (ev === 1 && (c0.k === 'nth' || c0.k === 'dom' || c0.k === 'season')) {
      // "The 3rd Kingsday of the month" reads as one day; "of every month" repeats.
      s = 'the ' + noun(c0, env).replace(/of the month$/, 'of every month');
    } else {
      s = 'every ' + (ev === 1 ? '' : (ev === 2 ? 'other ' : ordinal(ev) + ' ')) + noun(c0, env);
    }
    if (form.conds.length > 1) s += ', only when it falls ' + form.conds.slice(1).map(function (c) { return filter(c, env); }).join(' and ');
    if (form.shift) s = form.shift + ' day' + (form.shift === 1 ? '' : 's') + ' ' + (form.dir === 'earlier' ? 'before' : 'after') + ' ' + s;
    return upperFirst(s) + '.';
  }
  // The same sentence for a sentence that follows "Repeats:".
  function summaryInline(form, env) {
    return lowerFirst(summary(form, env)).replace(/\.$/, '');
  }

  // ---- ready-made rules ------------------------------------------------
  // Each is built for this calendar's own names; one needing a moon, a
  // season-less week of six days or an event the calendar lacks is left
  // out. Choosing one just fills the form, so it can be edited like any.
  function presets(env) {
    var out = [], moons = moonsOf(env), wl = weekLen(env), cond;
    function add(id, label, conds, extra) {
      var f = { conds: conds, every: 1, shift: 0, dir: 'later' };
      if (extra) for (var k in extra) f[k] = extra[k];
      out.push({ id: id, label: label, form: f });
    }
    var m0 = moons[0], mn = m0 ? m0.name : '';
    var wdA = Math.min(3, wl - 1), wdB = Math.min(5, wl - 1), wdC = Math.min(4, wl - 1);
    if (m0) {
      add('full-moon', 'Every full moon of ' + mn, [{ k: 'moon', ph: 'full', moon: m0.id }]);
      add('new-moon', 'Every new moon of ' + mn, [{ k: 'moon', ph: 'new', moon: m0.id }]);
      add('before-full', 'The day before every full moon of ' + mn, [{ k: 'moon', ph: 'full', moon: m0.id }], { shift: 1, dir: 'earlier' });
      add('third-full', 'Every 3rd full moon of ' + mn, [{ k: 'moon', ph: 'full', moon: m0.id }], { every: 3 });
      add('full-on-wd', 'A full moon of ' + mn + ' on a ' + wdName(env, wdB), [{ k: 'moon', ph: 'full', moon: m0.id }, { k: 'wd', wds: [wdB] }]);
    }
    add('first-wd', 'The first ' + wdName(env, 0) + ' of every month', [{ k: 'nth', n: 1, wd: 0 }]);
    add('nth-wd', 'The 3rd ' + wdName(env, wdA) + ' of each month', [{ k: 'nth', n: 3, wd: wdA }]);
    add('last-wd', 'The last ' + wdName(env, wdB) + ' of each month', [{ k: 'nth', n: -1, wd: wdB }]);
    add('last-day', 'The last day of each month', [{ k: 'dom', d: -1 }]);
    if (wl > 5) add('weekdays-only', 'Weekdays only (first five weekdays)', [{ k: 'wd', wds: [0, 1, 2, 3, 4] }]);
    add('other-wd', 'Every other ' + wdName(env, wdC), [{ k: 'wd', wds: [wdC] }], { every: 2 });
    var dom = Math.min(13, maxMonthDays(env));
    add('dom-wd', 'The ' + ordinal(dom) + ', when it’s a ' + wdName(env, wdC), [{ k: 'dom', d: dom }, { k: 'wd', wds: [wdC] }]);
    if (monthsOf(env).length) {
      var mo = Math.min(monthsOf(env).length, 9);
      add('wd-in-month', 'Every ' + wdName(env, wdC) + ' in ' + monthName(env, mo), [{ k: 'wd', wds: [wdC] }, { k: 'month', m: mo }]);
    }
    var ev0 = (env.events || [])[0];
    if (ev0) add('after-event', '2 days after ' + ev0.name, [{ k: 'event', event: ev0.id }], { shift: 2 });
    return out;
  }
  // The preset a rule is exactly, or null for "your own rule".
  function presetFor(form, env) {
    var all = presets(env);
    for (var i = 0; i < all.length; i++) if (sameRule(all[i].form, form)) return all[i].id;
    return null;
  }
  function presetForm(id, env) {
    var all = presets(env);
    for (var i = 0; i < all.length; i++) if (all[i].id === id) return clone(all[i].form);
    return null;
  }

  // ---- checks before saving ------------------------------------------
  // The first thing wrong with a form, in words, or null. The server checks
  // again; this only saves a round trip for the common slips.
  function validate(form, env) {
    if (!form.conds.length) return 'Add at least one condition to the rule.';
    if (form.conds.length > MAX_CONDS) return 'A rule can have at most ' + MAX_CONDS + ' conditions.';
    for (var i = 0; i < form.conds.length; i++) {
      var c = form.conds[i];
      if (c.k === 'moon' && !moonsOf(env).some(function (m) { return String(m.id) === String(c.moon); })) return 'Pick a moon for condition ' + (i + 1) + '.';
      if (c.k === 'season' && !seasonsOf(env).some(function (s) { return String(s.id) === String(c.season); })) return 'Pick a season for condition ' + (i + 1) + '.';
      if (c.k === 'event' && !c.event) return 'Pick an event for condition ' + (i + 1) + '.';
      if (c.k === 'wd' && (!c.wds.length || c.wds.some(function (w) { return w < 0 || w >= weekLen(env); }))) return 'Pick a weekday for condition ' + (i + 1) + '.';
      if (c.k === 'nth' && (c.wd < 0 || c.wd >= weekLen(env))) return 'Pick a weekday for condition ' + (i + 1) + '.';
      if (c.k === 'month' && (c.m < 1 || c.m > monthsOf(env).length)) return 'Pick a month for condition ' + (i + 1) + '.';
    }
    if (!(form.every >= 1 && form.every <= MAX_EVERY)) return 'Keep every Nth match between 1 and ' + MAX_EVERY + '.';
    if (!(form.shift >= 0 && form.shift <= MAX_SHIFT)) return 'Shift the date by at most ' + MAX_SHIFT + ' days.';
    return null;
  }

  var api = {
    PHASES: PHASES, KIND_ORDER: KIND_ORDER, KIND_LABELS: KIND_LABELS, ORDINALS: ORDINALS,
    MAX_CONDS: MAX_CONDS, MAX_EVERY: MAX_EVERY, MAX_SHIFT: MAX_SHIFT,
    ordinal: ordinal, intIn: intIn, clone: clone,
    moonsOf: moonsOf, seasonsOf: seasonsOf, weekdaysOf: weekdaysOf, monthsOf: monthsOf, maxMonthDays: maxMonthDays,
    wdName: wdName, monthName: monthName, eventName: eventName,
    availableKinds: availableKinds, defaultCond: defaultCond,
    fromRule: fromRule, toRule: toRule, sameRule: sameRule,
    summary: summary, summaryInline: summaryInline, noun: noun,
    presets: presets, presetFor: presetFor, presetForm: presetForm,
    validate: validate
  };

  window.Chronicle = window.Chronicle || {};
  Chronicle.calendarRule = api;
  // Test-only hook, as in calendar_view.js: undefined in the browser.
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})();
