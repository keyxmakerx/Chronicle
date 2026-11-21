/**
 * calendar_year_track.js — the year track in the calendar's Edit drawer.
 *
 * "The year" folds open under the drawer's tool row and shows the whole
 * current year as a few quiet lines: the seasons, the months, a tick for each
 * day that holds a saved event and a band where weather is set. It is the way
 * in to the generator engine's mass tools (chronicle_gen.js, loaded on first
 * use through the editor's loadEngine): fill the rest of the year, copy a
 * month, repeat an event every month, roll generated events again, and write a
 * history of the years before now. This file is itself fetched by the editor
 * the first time Edit opens, so a page that is only looked at never loads it.
 *
 * Every tool makes a DRAFT first. A draft lives only in this page: it is drawn
 * as lighter marks on the lines and as hollow marks on the month grid (view.
 * evPreview, view.wxPreview), and nothing is saved until Keep. Keep writes
 * through the calendar's own routes (POST events, PUT weather/days) and offers
 * Undo, which deletes what Keep created and puts back what it replaced.
 *
 * WHO MAY DO WHAT. Anyone who can open Edit gets the track as a view of the
 * year. The generating tools need two things at once: authoring DM-only
 * content (everything generated is saved dm_only unless a fill is kept "On
 * their day", and weather writes are gated on it) and the Owner role (Undo and a re-roll delete events, which
 * routes.go gates on RoleOwner). The client only decides what to offer; the
 * server gates are the safety.
 *
 * WHAT GENERATED MEANS. A generated event carries a payload marker,
 * {"type":"generated","lane":"events|sky|history","sig":"..."}, where sig
 * hashes the event's name, date and description as generated. An event whose
 * current text no longer matches its sig was edited by a person and counts as
 * hand-made: no tool here replaces or deletes a hand-made event, and none
 * overwrites a day of weather that was painted by hand or locked.
 *
 * It is not a Chronicle widget of its own: calendar_editor.js attaches it
 * (editor.yearTrack) and calls the few methods listed on the returned object
 * of attach(). Loaded before calendar_editor.js, like the weather sheet.
 */
(function () {
  'use strict';

  var ROLE_OWNER = 3;
  var LANES = ['events', 'sky', 'history'];
  // What one batch may carry; the server refuses more (event_batch.go).
  var BATCH_MAX_EVENTS = 2000, BATCH_MAX_DAYS = 2000;
  // The server refuses a request body over 2 MB; stay under it.
  var BATCH_MAX_BYTES = 1900000;
  var WORDS = ['no', 'one', 'two', 'three', 'four', 'five', 'six', 'seven', 'eight', 'nine', 'ten', 'eleven', 'twelve'];

  // ------------------------------------------------------------------
  // Small helpers
  // ------------------------------------------------------------------
  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }
  function esc(s) { return Chronicle.escapeHtml(s == null ? '' : String(s)); }
  function el(html) { var t = document.createElement('template'); t.innerHTML = html.trim(); return t.content.firstChild; }
  function assign(a) {
    for (var i = 1; i < arguments.length; i++) {
      var b = arguments[i];
      if (b) Object.keys(b).forEach(function (k) { a[k] = b[k]; });
    }
    return a;
  }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }
  function count(n, one, many) { return n + ' ' + (n === 1 ? one : (many || one + 's')); }
  function listWords(parts) { return parts.length > 1 ? parts.slice(0, -1).join(', ') + ' and ' + parts[parts.length - 1] : parts[0] || ''; }
  function randomSeed() { return Math.random().toString(36).slice(2, 8); }
  function dkey(y, m, d) { return y + '_' + m + '_' + d; }
  function stamp(y, m, d) { return y * 10000 + m * 100 + d; }
  function CD() { return Chronicle.calendarDate; }

  // Calm and Off both stop the big moves; a device that asks for reduced
  // motion is always Off. Read at call time so a My view change applies at once.
  function motion() {
    var a = document.documentElement.getAttribute('data-motion');
    if (a === 'off' || (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches)) return 'off';
    return a === 'calm' ? 'calm' : 'full';
  }
  // A duration or curve token from input.css, for script-driven motion.
  function tok(name, fallback) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    if (!v) return fallback;
    return /ms$/.test(v) ? parseFloat(v) : /s$/.test(v) ? parseFloat(v) * 1000 : v;
  }
  function wait(ms) { return new Promise(function (r) { setTimeout(r, motion() === 'off' ? 0 : ms); }); }

  // A block opens downward from its own top edge to its measured height and
  // folds back the same way; Calm only fades, Off ends at once.
  function fold(node, open) {
    var m = motion();
    node.getAnimations().forEach(function (a) { a.cancel(); });
    if (open) node.hidden = false;
    if (m === 'off') { node.hidden = !open; return Promise.resolve(); }
    var h = node.scrollHeight;
    var kf = m === 'calm'
      ? [{ opacity: open ? 0 : 1 }, { opacity: open ? 1 : 0 }]
      : [{ height: (open ? 0 : h) + 'px', opacity: open ? 0 : 1 }, { height: (open ? h : 0) + 'px', opacity: open ? 1 : 0 }];
    node.style.overflow = 'hidden';
    var a = node.animate(kf, { duration: open ? tok('--dur-standard', 200) : tok('--dur-micro', 120), easing: open ? tok('--ease-out', 'ease-out') : tok('--ease-in', 'ease-in') });
    return a.finished.then(function () { node.style.overflow = ''; if (!open) node.hidden = true; }).catch(function () {});
  }

  // ------------------------------------------------------------------
  // The year's geometry. Months are not equal, so every position is in days:
  // starts[i] is how many days of the year come before month i + 1.
  // ------------------------------------------------------------------
  function geometry(days) {
    var starts = [], total = 0;
    days.forEach(function (n) { starts.push(total); total += n; });
    return {
      days: days, starts: starts, total: total,
      // Day of the year from 0, for a month (from 1) and a day (from 1).
      doy: function (m, d) { return starts[m - 1] + (d - 1); },
      fromDoy: function (n) {
        for (var i = days.length - 1; i >= 0; i--) if (n >= starts[i] && days[i] > 0) return { month: i + 1, day: n - starts[i] + 1 };
        return { month: 1, day: 1 };
      }
    };
  }
  // The lit month: where it starts and how wide it is, as fractions of the year.
  function windowFor(g, month) {
    if (!g.total || !g.days[month - 1]) return null;
    return { at: g.starts[month - 1] / g.total, span: g.days[month - 1] / g.total };
  }
  // Today's line sits in the middle of today's day.
  function nowAt(g, month, day) { return g.total ? (g.doy(month, day) + 0.5) / g.total : 0; }
  // The seasons as spans of the year (a season that wraps the year's end is
  // two spans), from the calendar's own start and end dates.
  function seasonSpans(seasons, g) {
    var out = [];
    (seasons || []).slice().sort(function (a, b) { return a.start_month - b.start_month || (a.start_day || 1) - (b.start_day || 1); }).forEach(function (s) {
      var sm = g.days[s.start_month - 1], em = g.days[s.end_month - 1];
      if (!sm || !em) return;
      var a = g.doy(s.start_month, Math.min(s.start_day || 1, sm)), b = g.doy(s.end_month, Math.min(s.end_day || em, em)) + 1;
      if (b > a) out.push({ name: s.name, from: a, to: b });
      else if (b < a) { out.push({ name: s.name, from: a, to: g.total }); if (b > 0) out.push({ name: s.name, from: 0, to: b }); }
    });
    return out;
  }
  // Runs of consecutive days, from a set of days of the year.
  function runsOf(set) {
    var ns = Object.keys(set).map(Number).sort(function (a, b) { return a - b; }), out = [], i = 0;
    while (i < ns.length) {
      var a = ns[i], b = a;
      while (i + 1 < ns.length && ns[i + 1] === b + 1) b = ns[++i];
      i++;
      out.push([a, b]);
    }
    return out;
  }
  // Every saved event of a year from the months the view holds (a repeating
  // event is in many months' lists but counts once), by the day it is based on.
  function yearEventsFrom(byMonth, y, monthCount) {
    var seen = {}, out = [];
    for (var m = 1; m <= monthCount; m++) {
      (byMonth[y + '_' + m] || []).forEach(function (e) {
        if (seen[e.id]) return;
        seen[e.id] = true;
        if (e.year === y) out.push(e);
      });
    }
    return out;
  }

  // ------------------------------------------------------------------
  // The marker that says an event was generated, and the edit check
  // ------------------------------------------------------------------
  // A small stable hash (FNV-1a, 32 bits) in base 36. It only has to tell
  // "as generated" from "edited since", not resist anyone.
  function hash(s) {
    var h = 0x811c9dc5;
    for (var i = 0; i < s.length; i++) {
      h ^= s.charCodeAt(i);
      h = (h + ((h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24))) >>> 0;
    }
    return h.toString(36);
  }
  function sigOf(name, y, m, d, description) {
    return hash([String(name == null ? '' : name).trim(), y, m, d, String(description == null ? '' : description)].join('␟'));
  }
  function markerFor(lane, body) {
    return JSON.stringify({ type: 'generated', lane: lane, sig: sigOf(body.name, body.year, body.month, body.day, body.description) });
  }
  function parsePayload(e) {
    if (!e || !e.payload) return null;
    try { var p = JSON.parse(e.payload); return p && typeof p === 'object' ? p : null; } catch (err) { return null; }
  }
  // The lane of an event the generator made and nobody has changed since, or
  // null for anything else. This is the one test every tool uses to decide
  // whether an event may be replaced.
  function generatedLane(e) {
    var p = parsePayload(e);
    if (!p || p.type !== 'generated' || LANES.indexOf(p.lane) < 0) return null;
    return p.sig === sigOf(e.name, e.year, e.month, e.day, e.description) ? p.lane : null;
  }
  function isHandMade(e) { return generatedLane(e) === null; }
  // What a new fill replaces: the happenings and sky events an earlier fill
  // put on or after today in this year that nobody has changed since, so
  // filling twice does not stack two years on top of each other.
  function fillReplaces(saved, now) {
    var from = stamp(now.year, now.month, now.day);
    return saved.filter(function (e) {
      var l = generatedLane(e);
      return (l === 'events' || l === 'sky') && e.year === now.year && stamp(e.year, e.month, e.day) >= from;
    });
  }

  // The repeats a fill makes and a later fill can end: a market that comes
  // every week, two weeks or month, with no cap on how many times. A yearly
  // event is left out because its next day is next year, past the stretch a
  // fill covers, so ending it would take that day away with nothing new put
  // in its place.
  var ENDABLE_REPEATS = ['weekly', 'biweekly', 'monthly'];
  // The day before `now`, within its year, or null on the year's first day.
  // monthDays(monthIndex, year) is the calendar's own count, so unequal and
  // intercalary months are right.
  function dayBefore(now, monthDays) {
    if (now.day > 1) return { year: now.year, month: now.month, day: now.day - 1 };
    for (var m = now.month - 1; m >= 1; m--) {
      var n = monthDays(m - 1, now.year);
      if (n > 0) return { year: now.year, month: m, day: n };
    }
    return null;
  }
  // What a new fill ends rather than deletes: an earlier fill's repeating
  // event that began before today and still has days ahead, untouched since.
  // Its first days stay as they were; the new fill starts its own repeat.
  function fillRetires(saved, now) {
    var from = stamp(now.year, now.month, now.day);
    return saved.filter(function (e) {
      var l = generatedLane(e);
      if ((l !== 'events' && l !== 'sky') || e.year !== now.year || !e.is_recurring) return false;
      if (ENDABLE_REPEATS.indexOf(e.recurrence_type) < 0 || e.recurrence_max_occurrences != null) return false;
      if (stamp(e.year, e.month, e.day) >= from) return false;
      var ended = e.recurrence_end_year != null && e.recurrence_end_month != null && e.recurrence_end_day != null;
      return !ended || stamp(e.recurrence_end_year, e.recurrence_end_month, e.recurrence_end_day) >= from;
    });
  }

  // ------------------------------------------------------------------
  // Weather that may be written
  // ------------------------------------------------------------------
  // A reading a person painted, or locked, is never replaced by a draw.
  function protectedDay(w) { return !!w && (w.source !== 'generated' || w.locked === true); }
  // Fill puts weather only where a day has none. readingOf(day) is the stored reading or null.
  function emptyDays(days, readingOf) {
    return days.filter(function (d) { return !readingOf(d); });
  }
  // A re-roll redraws only days that hold a generated, unlocked reading.
  function redrawable(days, readingOf) {
    return days.filter(function (d) { var w = readingOf(d); return !!w && !protectedDay(w); });
  }
  // A stored reading as the generator reads it: no zone, so a day drawn under
  // another climate still counts as the weather around. A painted or locked
  // day is passed as a lock, so the draw is built around it and never over it.
  function asEngineDay(w, known) {
    var c = {};
    Object.keys(w).forEach(function (k) { c[k] = w[k]; });
    c.zone_id = null; c.zone_name = null;
    if (protectedDay(w) && (!w.preset_id || !known || known[w.preset_id])) c.gen = { locked: true };
    return c;
  }

  // ------------------------------------------------------------------
  // The draft: what the generators made, before it is kept
  // ------------------------------------------------------------------
  var RECIPES = {
    events: [['events.frontier', 'A frontier village'], ['events.market-town', 'A market town'], ['events.festive', 'A festive year'], ['events.dark-omens', 'A year of omens']],
    sky: [['sky.classic', 'A natural sky'], ['sky.high-magic', 'A magic sky'], ['sky.omens', 'A sky full of omens'], ['sky.quiet', 'A quiet sky']],
    history: [['history.kingdom', 'A kingdom'], ['history.frontier', 'A frontier'], ['history.dark', 'A dark age']]
  };
  var BUSY = [['0.25', 'Quiet'], ['0.5', 'Steady'], ['0.9', 'Busy']];
  var SEE = [['later', 'When I reveal them'], ['day', 'On their day']];

  function blankDraft(kind, seed) {
    return {
      kind: kind, seed: seed, ev: [], wx: [], remove: [], retire: [], pinned: {}, dropped: {}, note: '',
      on: { events: true, sky: true, weather: true },
      recipe: { events: 'events.frontier', sky: 'sky.classic', history: 'history.kingdom' }, busy: '0.5',
      // When players learn of what a fill keeps: 'later' (a Director reveals each) or 'day' (each shows on its day).
      see: 'later',
      n: { events: 0, sky: 0, weather: 0, history: 0 }, wxPrev: {}
    };
  }
  function keyOf(e) { return e.gen && e.gen.key; }
  // The events that would be saved: a fill's parts that are switched on, less
  // any the owner left out of the list.
  function activeEvents(D) {
    if (!D) return [];
    return D.ev.filter(function (e) { return (D.kind !== 'fill' || D.on[e._lane]) && !D.dropped[keyOf(e)]; });
  }
  // One reading per day: the server refuses a day named twice in one call.
  // The saved events Keep would remove. A fill removes an earlier fill's
  // events only for the parts that are switched on.
  function removalsOf(D) {
    if (!D) return [];
    if (D.kind !== 'fill') return D.remove;
    return D.remove.filter(function (e) { return D.on[generatedLane(e)]; });
  }
  // The repeating events a fill ends, by the same parts-switched-on rule.
  function retiresOf(D) {
    if (!D || D.kind !== 'fill') return [];
    return (D.retire || []).filter(function (e) { return D.on[generatedLane(e)]; });
  }
  function activeWeather(D) {
    if (!D || (D.kind === 'fill' && !D.on.weather)) return [];
    var seen = {};
    return D.wx.filter(function (w) {
      var k = w.year + '_' + w.month + '_' + w.day;
      if (seen[k]) return false;
      return (seen[k] = true);
    });
  }
  function counts(D) {
    var a = activeEvents(D), c = { events: 0, sky: 0, history: 0, copy: 0, weather: activeWeather(D).length, all: a.length };
    a.forEach(function (e) { c[e._lane] = (c[e._lane] || 0) + 1; });
    return c;
  }
  // The draft in one plain sentence. ctx: {now: 'Firstfrost 8', year: '100 AS',
  // yearOf: function (n) { return '100 AS'; }}.
  function sentence(D, ctx) {
    var c = counts(D);
    if (D.kind === 'fill') {
      var parts = [];
      if (D.on.events) parts.push(count(c.events, 'happening'));
      if (D.on.sky) parts.push(count(c.sky, 'sky event'));
      // Weather is only drawn for days that have none.
      var wxSet = D.on.weather && !c.weather && (D.on.events || D.on.sky);
      if (D.on.weather && !wxSet) parts.push(count(c.weather, 'day') + ' of weather');
      if (!parts.length) return 'Every part of this draft is switched off.';
      var old = removalsOf(D).length + retiresOf(D).length;
      var onDay = D.see === 'day' && (D.on.events || D.on.sky);
      return 'From ' + ctx.now + ' to the end of ' + ctx.year + ': ' + listWords(parts) + '.' + (onDay ? ' Players see each event on its day.' : '') + (wxSet ? ' Every day already has its weather.' : '') + (old ? ' It replaces ' + count(old, 'event') + ' from an earlier fill.' : '');
    }
    if (D.kind === 'history') {
      var ys = activeEvents(D).map(function (e) { return e.year; });
      if (!ys.length) return 'Every event in this history is left out.';
      return 'A history for the years before now: ' + count(c.history, 'event') + ', from ' + ctx.yearOf(Math.min.apply(null, ys)) + ' to ' + ctx.yearOf(Math.max.apply(null, ys)) + '. Your eras stay as they are.';
    }
    return D.note;
  }
  function keepLabel(D) { return D.kind === 'history' ? 'Keep this history' : 'Keep'; }
  // What Keep says it did: "Kept 24 events and 120 days of weather."
  function keptWords(c, wxCount) {
    var parts = [];
    if (c) parts.push(count(c, 'event'));
    if (wxCount) parts.push(count(wxCount, 'day') + ' of weather');
    return 'Kept ' + (listWords(parts) || 'nothing') + '.';
  }
  // Keep is all or nothing, so a failed one says what is still true.
  function notKeptWords(status) {
    if (!status) return 'The server did not answer, so the draft may or may not have been saved. The calendar shows what is saved now. Check it, then fill again if it is not there.';
    if (status === 429) return 'The server is busy. Nothing was saved and the draft is still here. Try Keep again in a moment.';
    return 'Nothing was saved. The draft is still here.';
  }
  function notUndoneWords(status) {
    if (!status) return 'The server did not answer, so the undo may or may not have happened. The calendar shows what is saved now.';
    if (status === 429) return 'The server is busy. Nothing was undone. Press Undo again in a moment.';
    return 'Couldn’t undo. Nothing was changed. The calendar shows what is saved now.';
  }

  // ------------------------------------------------------------------
  // What an event is saved as
  // ------------------------------------------------------------------
  var ANNOUNCED = { 'ahead': 'ahead', 'on-the-day': 'on_day', 'on_day': 'on_day' };
  var COPY_FIELDS = ['name', 'description', 'description_html', 'entity_id', 'year', 'month', 'day', 'start_hour', 'start_minute',
    'end_year', 'end_month', 'end_day', 'end_hour', 'end_minute', 'is_recurring', 'recurrence_type', 'recurrence_interval',
    'recurrence_end_year', 'recurrence_end_month', 'recurrence_end_day', 'recurrence_max_occurrences', 'recurrence_rule',
    'visibility_rules', 'kind_id', 'tier', 'color', 'icon', 'all_day'];
  // The body POSTed to the events route. opts.lane saves a generated event: dm_only
  // and marked, or, with opts.onDay (never a history, which is already past), shown
  // to everyone but held until its day. Without a lane the event is a copy, so it keeps its own visibility and
  // payload, except a generated marker, which belongs to the event it was made for;
  // opts.keepPayload (an Undo putting an event back as it was) keeps that too.
  function createBody(e, opts) {
    opts = opts || {};
    var b = {};
    COPY_FIELDS.forEach(function (k) { if (e[k] !== undefined && e[k] !== null) b[k] = e[k]; });
    b.name = String(b.name == null ? '' : b.name).slice(0, 200);
    if (b.all_day === undefined) b.all_day = e.start_hour == null;
    var ann = ANNOUNCED[e.announced] || ANNOUNCED[e.announce];
    if (ann) b.announced = ann;
    if (opts.lane) {
      if (opts.onDay && opts.lane !== 'history') { b.visibility = 'everyone'; b.announced = 'on_day'; }
      else b.visibility = 'dm_only';
      b.payload = markerFor(opts.lane, b);
    } else {
      b.visibility = e.visibility === 'dm_only' ? 'dm_only' : 'everyone';
      var p = parsePayload(e);
      if (e.payload && (opts.keepPayload || !(p && p.type === 'generated'))) b.payload = e.payload;
    }
    return b;
  }

  // What the chosen days' row offers (the editor draws it). evs: the saved
  // events touching the chosen days; dates: the chosen days.
  function chosenActions(evs, dates, canGenerate) {
    if (!canGenerate) return { every: null, again: false };
    var one = dates.length === 1 && evs.length === 1 && !evs[0].is_recurring ? evs[0] : null;
    var again = evs.some(function (e) { var l = generatedLane(e); return !e.is_recurring && (l === 'events' || l === 'sky'); });
    return { every: one, again: again };
  }
  // Who may be offered the generating tools.
  function mayGenerate(view) { return !!view && view.canAuthorDmOnly === true && view.role >= ROLE_OWNER; }

  // ==================================================================
  // The track on the page
  // ==================================================================
  function YearTrack(editor) {
    this.editor = editor;
    this.view = editor.view;
    this.canGen = mayGenerate(this.view);
    this.D = null; // the draft, or null
    this.isOpen = false;
    this.zoom = 'year'; // 'year', or 'past' while a history is the draft
    this.picking = null; // {month, n} while "Copy this month" asks for a target
    this.node = null;
    this.folds = {}; // which of the draft's two folds are open
    this.busy = false; // Keep or Undo is writing
    this.glow = null; // what Keep just wrote, for one settle
    this.ink = null; // a draft inking in
    this.lastMonth = '';
    this._geo = {};
    this.mount();
  }
  var P = YearTrack.prototype;

  // ---- the calendar, as the track reads it ----
  P.cal = function () { return this.view.cal; };
  P.now = function () { var c = this.cal(); return { year: c.current_year, month: c.current_month, day: c.current_day }; };
  P.monthCount = function () { return (this.cal().months || []).length; };
  P.geo = function (y) {
    var c = this.cal(), key = y + ':' + this.monthCount();
    if (!this._geo[key]) this._geo[key] = geometry((c.months || []).map(function (M, i) { return CD().monthDays(c, i, y); }));
    return this._geo[key];
  };
  P.mName = function (m) { return ((this.cal().months || [])[m - 1] || {}).name || ''; };
  P.isFest = function (m) { return !!((this.cal().months || [])[m - 1] || {}).is_intercalary; };
  P.fmt = function (d) { return this.isFest(d.month) ? this.mName(d.month) : this.mName(d.month) + ' ' + d.day; };
  P.yearName = function (y) { var n = this.cal().epoch_name; return y + (n ? ' ' + n : ''); };
  P.erasOf = function () {
    return (this.cal().eras || []).slice().sort(function (a, b) {
      return stamp(a.start_year, a.start_month || 1, a.start_day || 1) - stamp(b.start_year, b.start_month || 1, b.start_day || 1);
    });
  };
  P.inEra = function (e, era) {
    var t = stamp(e.year, e.month, e.day), a = stamp(era.start_year, era.start_month || 1, era.start_day || 1);
    var b = era.end_year != null ? stamp(era.end_year, era.end_month || 99, era.end_day || 99) : Infinity;
    return t >= a && t <= b;
  };
  P.ctx = function () {
    var self = this, n = this.now();
    return { now: this.fmt(n), year: this.yearName(n.year), yearOf: function (y) { return self.yearName(y); } };
  };
  // The saved events of the year on show, from what the view already holds.
  P.savedEvents = function (y) { return yearEventsFrom(this.view.eventsByMonth, y, this.monthCount()); };
  P.monthEventsToCopy = function (y, m) {
    return this.view.eventsFor(y, m).filter(function (e) { return !e.is_recurring && e.year === y && e.month === m; });
  };
  // Loads the year the track draws: every month's events and the weather.
  P.ensureYear = function (y) {
    var v = this.view, ps = [], mc = this.monthCount();
    for (var m = 1; m <= mc; m++) ps.push(v.fetchMonth(y, m));
    ps.push(v.fetchWeatherYear(y));
    return Promise.all(ps);
  };

  // ---- mounting ----
  P.mount = function () {
    var self = this, drawer = this.editor.drawer;
    this.node = el('<section class="yt-year" id="cal5-year" hidden aria-label="The year">' +
      '<div class="yt-track"><div class="yt-pan"><div class="yt-rows"></div><div class="yt-over" aria-hidden="true"><i class="yt-win"></i><i class="yt-now"></i></div><div class="yt-peek" hidden></div></div></div>' +
      '<div class="yt-foot"></div></section>');
    drawer.insertBefore(this.node, $('.bbw', drawer));
    this.rows = $('.yt-rows', this.node); this.foot = $('.yt-foot', this.node); this.peekEl = $('.yt-peek', this.node);
    this.node.addEventListener('click', function (e) { self.click(e); });
    this.node.addEventListener('pointerover', function (e) { self.hover(e.target.closest('[data-peek]')); });
    this.node.addEventListener('pointerout', function (e) { if (!e.relatedTarget || !self.node.contains(e.relatedTarget) || !e.relatedTarget.closest('[data-peek]')) self.hover(null); });
    this.node.addEventListener('focusin', function (e) { self.hover(e.target.closest('[data-peek]')); });
    this.node.addEventListener('focusout', function () { self.hover(null); });
    this.node.addEventListener('keydown', function (e) { self.keys(e); });
    if (window.ResizeObserver) { this.ro = new ResizeObserver(function () { self.place(true); }); this.ro.observe(this.node); }
    this.view.afterPaint = function () { self.onPaint(); };
    this.bindBar();
  };
  P.button = function () { return $('[data-deck-year]', this.editor.deck); };

  // ---- the button and the panel ----
  P.toggle = function () { if (!this.D) this.setOpen(!this.isOpen); };
  P.setOpen = function (on) {
    var self = this;
    if (this.isOpen === on) return;
    this.isOpen = on;
    var b = this.button(); if (b) b.setAttribute('aria-expanded', String(on));
    if (on) {
      this.node.hidden = false; this.draw({}); this.node.hidden = true;
      // The lines read the whole year, so it is loaded the first time the track opens.
      this.ensureYear(this.now().year).then(function () { if (self.isOpen) self.draw({ quiet: true }); });
    }
    fold(this.node, on).then(function () { if (on) self.place(true); });
    if (on) this.place(true);
  };
  // The editor's own calls ------------------------------------------------
  P.onDeck = function () { this.syncButton(); this.paintSay(); };
  P.onEditing = function () {
    this.setPreview();
    this.view._paintMonth();
    if (this.editor.editing && this.isOpen) this.place(true);
    this.paintSay();
  };
  P.syncButton = function () {
    var b = this.button();
    if (!b) return;
    b.disabled = !!this.D;
    b.title = this.D ? 'Keep or discard the draft first' : '';
  };
  // Out of Edit the deck's line says a draft is still waiting there.
  P.paintSay = function () {
    var say = $('.dksay', this.editor.deck);
    if (say && this.D && !this.editor.editing) say.textContent = 'A draft is waiting in Edit. Nothing in it is saved yet.';
  };
  // The chosen days' row: the two buttons the track adds.
  P.barHTML = function () {
    var ed = this.editor, a = chosenActions(ed._eventsInSelection(), ed._selectedDates(), this.canGen), off = this.D ? ' disabled title="Keep or discard the draft first"' : '';
    return (a.every ? '<button type="button" class="bbb" data-yt-bb="every"' + off + '><i class="fa-solid fa-repeat" aria-hidden="true"></i><span>Repeat every month</span></button>' : '') +
      (a.again ? '<button type="button" class="bbb" data-yt-bb="again"' + off + '><i class="fa-solid fa-rotate" aria-hidden="true"></i><span>Roll again</span></button>' : '');
  };
  // The Eras card's row for a history.
  P.eraRowHTML = function () {
    if (!this.canGen) return '';
    return '<button type="button" class="addev" data-yt-history' + (this.D ? ' disabled title="Keep or discard the draft first"' : '') + '><i class="fa-solid fa-feather-pointed" aria-hidden="true"></i>Write a history</button>';
  };
  P.bindBar = function () {
    var self = this, ed = this.editor;
    ed.bbar.addEventListener('click', function (e) {
      var t = e.target.closest('[data-yt-bb]');
      if (!t || t.disabled || self.D || self.busy) return;
      if (t.dataset.ytBb === 'every') {
        var ev = ed._eventsInSelection()[0];
        if (ev) self.everyMonth(ev);
      } else self.rerollDays(ed._selectedDates());
    });
  };

  // ---- drawing the lines ----
  P.draw = function (o) {
    if (!this.node) return;
    o = o || {};
    var D = this.D, want = D && D.kind === 'history' ? 'past' : 'year';
    if (want !== this.zoom && this.isOpen && !this.node.hidden && motion() !== 'off') { this.zoomTo(want, o); return; }
    this.zoom = want;
    this.rows.innerHTML = want === 'past' ? this.pastHTML(D) : this.yearHTML(D);
    this.node.dataset.zoom = want;
    this.node.classList.toggle('yt-picking', !!this.picking);
    if (o.ink && motion() === 'full') this.inkIn(o);
    if (o.kept) this.settle();
    $$('.yt-sn', this.node).forEach(function (n) { n.classList.toggle('yt-tight', n.scrollWidth > n.clientWidth + 1); });
    this.place(!o.month);
    this.drawFoot(o);
  };
  // Pulls back from the year to the years before now, or returns, as one
  // move hinged on today at the right-hand edge.
  P.zoomTo = function (want, o) {
    var self = this, rows = this.rows, calm = motion() === 'calm', out = want === 'past';
    var a = rows.animate(calm ? [{ opacity: 1 }, { opacity: 0 }] : [{ opacity: 1, transform: 'scaleX(1)' }, { opacity: 0, transform: 'scaleX(' + (out ? 0.3 : 2.4) + ')' }],
      { duration: tok('--dur-micro', 120), easing: tok('--ease-in', 'ease-in'), fill: 'forwards' });
    a.finished.then(function () {
      a.cancel(); self.zoom = want;
      self.draw(assign({}, o, { ink: false }));
      rows.animate(calm ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 0, transform: 'scaleX(' + (out ? 2.4 : 0.3) + ')' }, { opacity: 1, transform: 'scaleX(1)' }],
        { duration: tok('--dur-large', 280), easing: tok('--ease-out', 'ease-out') });
    }).catch(function () {});
  };

  P.seasonsHTML = function (g) {
    var pct = function (n) { return (n / g.total * 100).toFixed(4) + '%'; };
    return seasonSpans(this.cal().seasons, g).map(function (s) {
      return '<span class="yt-sn" style="left:' + pct(s.from) + ';width:' + pct(s.to - s.from) + '">' + esc(s.name) + '</span>';
    }).join('');
  };
  P.yearHTML = function (D) {
    var self = this, v = this.view, y = this.now().year, g = this.geo(y), cal = this.cal();
    var pct = function (n) { return (n / g.total * 100).toFixed(4) + '%'; };
    var gone = {};
    removalsOf(D).forEach(function (e) { gone[e.id] = true; });
    // Months: every one a button; only the month on show carries its name.
    var months = g.days.map(function (n, i) {
      if (!n) return '';
      var cur = v.view.y === y && v.view.m === i + 1;
      return '<button type="button" class="yt-mo' + (cur ? ' cur' : '') + '" data-m="' + (i + 1) + '" style="left:' + pct(g.starts[i]) + ';width:' + pct(n) + '"' +
        (cur ? ' aria-current="true"' : '') + ' aria-label="' + esc(self.mName(i + 1)) + (self.picking ? ', copy here' : '') + '"><span>' + esc(self.mName(i + 1)) + '</span></button>';
    }).join('');
    // Events: one mark a day, saved in the calendar's event blue, a draft lighter.
    var days = {}, add = function (e, kind) {
      if (e.year !== y) return;
      var n = g.doy(e.month, e.day), d = days[n] || (days[n] = { saved: [], draft: [], pin: false });
      d[kind].push(e);
      if (kind === 'draft' && D.pinned[keyOf(e)]) d.pin = true;
    };
    this.savedEvents(y).forEach(function (e) { if (!gone[e.id]) add(e, 'saved'); });
    if (D) activeEvents(D).forEach(function (e) { add(e, 'draft'); });
    var ticks = '', runs = '';
    Object.keys(days).map(Number).sort(function (a, b) { return a - b; }).forEach(function (n) {
      var d = days[n], at = g.fromDoy(n), both = d.saved.length && d.draft.length;
      ['saved', 'draft'].forEach(function (kind) {
        var list = d[kind];
        if (!list.length) return;
        var names = list.slice(0, 3).map(function (e) { return e.name; }).join(' · ') + (list.length > 3 ? ' · and ' + (list.length - 3) + ' more' : '');
        ticks += '<button type="button" class="yt-tk ' + kind + (kind === 'draft' && d.pin ? ' pin' : '') + (kind === 'draft' && both ? ' beside' : '') + '" data-d="' + n + '" style="left:' + pct(n + 0.5) + '"' +
          ' data-peek="' + esc(names) + '" data-when="' + esc(self.fmt(at) + (kind === 'draft' ? ', not kept yet' : '')) + '" aria-label="' + esc(self.fmt(at) + ': ' + names) + '"></button>';
        list.forEach(function (e) {
          if (e.end_year == null || e.end_year !== y || (e.end_month === e.month && e.end_day === e.day)) return;
          var b = g.doy(e.end_month, e.end_day);
          if (b > n) runs += '<i class="yt-run ' + kind + '" style="left:' + pct(n + 0.5) + ';width:' + pct(b - n) + '"></i>';
        });
      });
    });
    // Weather: where days have weather set, as bands.
    var have = {}, draft = {}, stored = v.weatherByYear[y] || {};
    Object.keys(stored).forEach(function (k) { var w = stored[k]; if (w && w.month && g.days[w.month - 1]) have[g.doy(w.month, w.day)] = 1; });
    if (D) activeWeather(D).forEach(function (w) { if (w.year === y) draft[g.doy(w.month, w.day)] = 1; });
    var bands = '', band = function (map, cls) {
      runsOf(map).forEach(function (r) { bands += '<i class="yt-band ' + cls + '" style="left:' + pct(r[0]) + ';width:' + pct(r[1] - r[0] + 1) + '"></i>'; });
    };
    band(have, 'saved'); band(draft, 'draft');
    var label = function (t) { return '<span class="yt-ln">' + t + '</span>'; };
    return (cal.seasons && cal.seasons.length ? '<div class="yt-row yt-seasons">' + label('') + '<div class="yt-lb">' + this.seasonsHTML(g) + '</div></div>' : '') +
      '<div class="yt-row yt-months">' + label(esc(this.yearName(y))) + '<div class="yt-lb">' + months + '</div></div>' +
      '<div class="yt-row yt-lane" data-lane="events">' + label('Events') + '<div class="yt-lb">' + runs + ticks + '</div></div>' +
      '<div class="yt-row yt-lane" data-lane="weather">' + label('Weather') + '<div class="yt-lb">' + bands + '</div></div>';
  };
  // The past: the same lines, pulled back until the eras fit.
  P.pastHTML = function (D) {
    var self = this, y = this.now().year, g = this.geo(y), evs = activeEvents(D), ys = evs.map(function (e) { return e.year; }), end = y + 1;
    var first = ys.length ? Math.min.apply(null, ys) : y - 100, span = Math.max(20, end - first), start = end - span * 1.04;
    var x = function (yr) { return ((yr - start) / (end - start) * 100).toFixed(3) + '%'; };
    var w = function (a, b) { return ((b - a) / (end - start) * 100).toFixed(3) + '%'; };
    var frac = function (e) { return e.year + g.doy(e.month, e.day) / g.total; };
    var mc = this.monthCount();
    var eras = this.erasOf().map(function (era) {
      var a = Math.max(start, era.start_year + g.doy(era.start_month || 1, era.start_day || 1) / g.total);
      var b = Math.min(end, era.end_year != null ? era.end_year + (g.doy(era.end_month || mc, era.end_day || 1) + 1) / g.total : end);
      if (b <= a) return '';
      return '<span class="yt-sn yt-era" style="left:' + x(a) + ';width:' + w(a, b) + '">' + esc(era.name) + '</span>';
    }).join('');
    var step = [10, 20, 50, 100, 200, 500, 1000].filter(function (s) { return span / s <= 9; })[0] || 2000, marks = '';
    for (var yr = Math.ceil(start / step) * step; yr < end - step * 0.35; yr += step) marks += '<span class="yt-yr" style="left:' + x(yr) + '">' + yr + '</span>';
    var ticks = evs.map(function (e) {
      return '<button type="button" class="yt-tk draft' + (D.pinned[keyOf(e)] ? ' pin' : '') + '" data-y="' + e.year + '" data-ym="' + e.month + '" data-yd="' + e.day + '" style="left:' + x(frac(e)) + '"' +
        ' data-peek="' + esc(e.name) + '" data-when="' + esc(self.fmt(e) + ', ' + self.yearName(e.year)) + ', not kept yet" aria-label="' + esc(self.yearName(e.year) + ': ' + e.name) + '"></button>';
    }).join('');
    var label = function (t) { return '<span class="yt-ln">' + t + '</span>'; };
    return '<div class="yt-row yt-seasons">' + label('') + '<div class="yt-lb">' + eras + '</div></div>' +
      '<div class="yt-row yt-months yt-years">' + label('The past') + '<div class="yt-lb">' + marks + '</div></div>' +
      '<div class="yt-row yt-lane" data-lane="events">' + label('Events') + '<div class="yt-lb">' + ticks + '</div></div>';
  };

  // The lit month and today's line, placed in pixels so that they slide.
  P.place = function (jump) {
    if (!this.node || this.node.hidden) return;
    var body = $('.yt-months .yt-lb', this.node), over = $('.yt-over', this.node), win = $('.yt-win', this.node), now = $('.yt-now', this.node);
    if (!body) return;
    var v = this.view, n = this.now(), g = this.geo(n.year);
    var tr = $('.yt-pan', this.node).getBoundingClientRect(), br = body.getBoundingClientRect();
    over.style.left = (br.left - tr.left) + 'px'; over.style.width = br.width + 'px';
    over.style.top = (br.top - tr.top) + 'px'; over.style.height = (tr.bottom - br.top) + 'px';
    if (jump) { win.style.transition = 'none'; now.style.transition = 'none'; }
    if (this.zoom === 'past') { win.style.opacity = '0'; now.style.transform = 'translateX(' + (br.width - 1) + 'px)'; }
    else {
      var w = v.view.y === n.year ? windowFor(g, v.view.m) : null;
      win.style.opacity = w ? '1' : '0';
      if (w) { win.style.width = (w.span * br.width) + 'px'; win.style.transform = 'translateX(' + (w.at * br.width) + 'px)'; }
      now.style.transform = 'translateX(' + (nowAt(g, n.month, n.day) * br.width) + 'px)';
    }
    if (jump) { void win.offsetWidth; win.style.transition = ''; now.style.transition = ''; }
    // Where the lines pan sideways, the month being looked at stays in sight.
    var sc = $('.yt-track', this.node), room = sc.clientWidth, lead = br.left - tr.left;
    if (sc.scrollWidth > room + 1) {
      var win2 = windowFor(g, v.view.m), at = this.zoom === 'past' || !win2 ? br.width : (win2.at + win2.span / 2) * br.width;
      var to = Math.max(0, Math.min(sc.scrollWidth - room, at - (room - lead) / 2));
      if (jump || motion() !== 'full' || !sc.scrollTo) sc.scrollLeft = to; else sc.scrollTo({ left: to, behavior: 'smooth' });
    }
  };
  P.onPaint = function () {
    var v = this.view, key = v.view.y + '_' + v.view.m;
    this.decorate();
    if (this.lastMonth !== key) { this.lastMonth = key; this.onMonth(); }
    else this.syncRest();
  };
  P.onMonth = function () {
    if (!this.node) return;
    if (this.zoom === 'year') {
      var v = this.view, y = this.now().year;
      $$('.yt-mo', this.node).forEach(function (b) {
        var cur = v.view.y === y && +b.dataset.m === v.view.m;
        b.classList.toggle('cur', cur);
        if (cur) b.setAttribute('aria-current', 'true'); else b.removeAttribute('aria-current');
      });
      this.place(false);
    }
    this.syncRest();
    if (this.D) setTimeout(this.reveal.bind(this), 360);
  };

  // A new draft inks in from today, left to right, as one wave.
  P.inkIn = function (o) {
    var lane = o.lane, g = this.geo(this.now().year), today = g.doy(this.now().month, this.now().day);
    if (!lane || lane !== 'weather') $$('.yt-tk.draft, .yt-run.draft', this.node).forEach(function (t) {
      var n = t.dataset.d != null ? +t.dataset.d : 0, from = t.dataset.d != null ? today : 0;
      t.classList.add('yt-in'); t.style.animationDelay = Math.round(Math.max(0, n - from) / g.total * 420) + 'ms';
    });
    if (!lane || lane === 'weather') $$('.yt-band.draft', this.node).forEach(function (b) { b.classList.add('yt-in'); });
  };
  // What was just kept settles into the calendar's own ink.
  P.settle = function () {
    var gl = this.glow;
    if (!gl || motion() === 'off') return;
    var g = this.geo(this.now().year), y = this.now().year;
    $$('.yt-tk.saved', this.node).forEach(function (t) { var at = g.fromDoy(+t.dataset.d); if (gl.days[dkey(y, at.month, at.day)]) t.classList.add('kept'); });
    if (Object.keys(gl.wx).length) $$('.yt-band.saved', this.node).forEach(function (b) { b.classList.add('kept'); });
  };
  // The month grid's own marks: a new draft's ghosts ink in, kept marks glow once.
  P.decorate = function () {
    var stage = this.view.stageEl, m = motion(), self = this;
    if (!stage) return;
    var now = this.now(), vv = this.view.view;
    var from = vv.y === now.year && vv.m === now.month ? now.day : 1;
    var delayFor = function (n) {
      var day = n.closest('[data-key]'), d = day ? +day.dataset.key.split('_')[2] : 1;
      return Math.round(Math.min(320, Math.max(0, d - from) * 16) - (performance.now() - self.ink.t0)) + 'ms';
    };
    if (this.ink && m !== 'off') {
      var lane = this.ink.lane;
      if (lane !== 'weather') $$('.mk.ghost', stage).forEach(function (mk) { mk.classList.add('yt-in'); mk.style.animationDelay = delayFor(mk); });
      if (!lane || lane === 'weather') $$('.cpt.ghost, .cwx.ghost', stage).forEach(function (n) { n.classList.add('yt-in'); n.style.animationDelay = delayFor(n); });
    }
    if (this.glow) {
      var gl = this.glow;
      $$('.mk[data-ev]', stage).forEach(function (mk) { if (gl.ids[mk.dataset.ev]) mk.classList.add('yt-kept'); });
      if (Object.keys(gl.wx).length) $$('.cwx', stage).forEach(function (n) { var day = n.closest('[data-key]'); if (day && gl.wx[day.dataset.key]) n.classList.add('yt-kept'); });
    }
  };

  // ---- the foot: what you can do, or the draft ----
  P.drawFoot = function () {
    var foot = this.foot, rest = $('.yt-rest', foot), pick = $('.yt-pick', foot);
    if (this.D) { if (rest) rest.remove(); if (pick) pick.remove(); this.picking = null; this.node.classList.remove('yt-picking'); this.renderBar(foot); return; }
    this.renderBar(foot);
    if (!this.canGen) return;
    if (this.picking) {
      var from = this.picking.month;
      var html = '<div class="yt-pick"><span class="yt-pl">Copy ' + esc(this.mName(from)) + '’s ' + count(this.picking.n, 'event') + ' to</span><div class="yt-pm">' +
        (this.cal().months || []).map(function (M, i) { return i + 1 === from ? '' : '<button type="button" class="yt-chip" data-yt="copyto:' + (i + 1) + '">' + esc(M.name) + '</button>'; }).join('') +
        '</div><button type="button" class="yt-word" data-yt="copycancel">Cancel</button></div>';
      if (rest) rest.remove();
      if (pick) pick.outerHTML = html; else foot.insertAdjacentHTML('afterbegin', html);
      return;
    }
    if (pick) pick.remove();
    if (!rest) foot.insertAdjacentHTML('afterbegin', '<div class="yt-rest">' + this.word('fill', 'Fill the rest of the year') + '<button type="button" class="yt-word" data-yt="copy">Copy this month</button></div>');
    this.syncRest();
  };
  // "Copy this month" is quiet when the month on show has nothing to copy.
  P.syncRest = function () {
    var cp = $('[data-yt="copy"]', this.foot);
    if (!cp || !this.canGen) return;
    var v = this.view, here = v.view.y === this.now().year ? this.monthEventsToCopy(v.view.y, v.view.m).length : 0;
    cp.disabled = !here || this.busy;
    if (!here) cp.title = this.mName(v.view.m) + ' has no events to copy'; else cp.removeAttribute('title');
  };

  // ---- pointing and pressing ----
  P.hover = function (t) {
    var peek = this.peekEl;
    if (!t || !this.node.contains(t)) { peek.hidden = true; return; }
    peek.innerHTML = '<b>' + esc(t.dataset.peek) + '</b><span>' + esc(t.dataset.when) + '</span>';
    peek.hidden = false;
    var tr = $('.yt-pan', this.node).getBoundingClientRect(), r = t.getBoundingClientRect(), w = peek.offsetWidth;
    var x = Math.max(0, Math.min(tr.width - w, r.left + r.width / 2 - tr.left - w / 2));
    peek.style.transform = 'translate(' + Math.round(x) + 'px,' + Math.max(0, Math.round(r.top - tr.top - peek.offsetHeight - 4)) + 'px)';
  };
  P.click = function (e) {
    var mo = e.target.closest('.yt-mo'), tk = e.target.closest('.yt-tk'), g = e.target.closest('[data-yt]');
    if (mo) { this.goTo(this.now().year, +mo.dataset.m); return; }
    if (tk) {
      if (tk.dataset.y) this.showDay(+tk.dataset.y, +tk.dataset.ym, +tk.dataset.yd);
      else { var at = this.geo(this.now().year).fromDoy(+tk.dataset.d); this.showDay(this.now().year, at.month, at.day); }
      return;
    }
    if (!g || g.disabled || g.closest('.yt-barwrap')) return;
    var a = g.dataset.yt, v = this.view;
    if (a === 'fill') this.fill(g);
    else if (a === 'copy') {
      var n = this.monthEventsToCopy(v.view.y, v.view.m).length;
      this.picking = { month: v.view.m, n: n }; this.draw({});
      var first = $('.yt-chip', this.foot); if (first) first.focus({ preventScroll: true });
    }
    else if (a === 'copycancel') { this.picking = null; this.draw({}); }
    else if (a.indexOf('copyto:') === 0) { var from = this.picking; this.picking = null; this.goTo(this.now().year, from.month); this.copyMonth(+a.slice(7)); }
  };
  // Arrow keys walk the months and the marks; Escape leaves a choice.
  P.keys = function (e) {
    var t = e.target;
    if (e.key === 'Escape' && this.picking) { e.stopPropagation(); this.picking = null; this.draw({}); return; }
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
    var cls = t.classList.contains('yt-mo') ? '.yt-mo' : t.classList.contains('yt-tk') ? '.yt-tk' : null;
    if (!cls) return;
    var all = $$(cls, this.node), i = all.indexOf(t) + (e.key === 'ArrowRight' ? 1 : -1);
    if (all[i]) { e.preventDefault(); all[i].focus(); }
  };

  // ---- the month grid ----
  P.goTo = function (y, m) {
    var v = this.view;
    if (v.view.y === y && v.view.m === m) return;
    v.goMonth((y - v.view.y) * this.monthCount() + (m - v.view.m));
  };
  // Turns to a day's month and lights the day once.
  P.showDay = function (y, m, d) {
    var self = this;
    this.goTo(y, m);
    setTimeout(function () {
      var cell = $('.day[data-key="' + dkey(y, m, d) + '"]', self.view.stageEl);
      if (!cell) return;
      cell.classList.remove('yt-lit'); void cell.offsetWidth; cell.classList.add('yt-lit');
      setTimeout(function () { cell.classList.remove('yt-lit'); }, 1300);
    }, motion() === 'full' ? 300 : 30);
  };
  // A draft lands on the grid, so the grid moves clear of the drawer: the
  // first week that holds something new comes into view above it.
  P.reveal = function () {
    var ed = this.editor, mark = $('.mk.ghost, .cwx.ghost', this.view.stageEl), cell = mark && mark.closest('.day'), main = document.getElementById('main-content');
    if (!cell || !main || !ed.drawer) return;
    var top = Math.min(ed.deck.getBoundingClientRect().top, ed.drawer.getBoundingClientRect().top), r = cell.getBoundingClientRect(), over = r.bottom + 12 - top;
    // Only when the week still fits above the drawer; otherwise the track shows it.
    if (over > 0 && r.height * 1.5 + 24 < top - main.getBoundingClientRect().top) main.scrollBy({ top: over, behavior: motion() === 'full' ? 'smooth' : 'auto' });
  };
  // The draft on the grid, only while editing. Weather goes through the
  // weather sheet's own preview, events through evPreview.
  P.setPreview = function () {
    var D = this.D, v = this.view, on = D && this.editor.editing;
    if (!on) { v.evPreview = null; v.wxPreview = null; return; }
    var pv = {};
    activeWeather(D).forEach(function (w) { pv[dkey(w.year, w.month, w.day)] = w; });
    v.wxPreview = Object.keys(pv).length ? pv : null;
    var hide = {};
    removalsOf(D).forEach(function (e) { hide[e.id] = true; });
    v.evPreview = {
      events: activeEvents(D).map(function (e) { return previewEvent(e, D); }),
      hide: hide
    };
  };
  // A draft event as the grid draws one: no server id the grid might look up.
  function previewEvent(e, D) {
    var o = {};
    ['name', 'description', 'year', 'month', 'day', 'end_year', 'end_month', 'end_day', 'start_hour', 'start_minute', 'is_recurring', 'recurrence_type',
      'recurrence_interval', 'recurrence_end_year', 'recurrence_end_month', 'recurrence_end_day', 'all_day', 'icon', 'color'].forEach(function (k) { if (e[k] != null) o[k] = e[k]; });
    // A draft the engine made is drawn as a plain dot, as it is when saved.
    if (e._lane !== 'copy') { delete o.icon; delete o.color; }
    o.id = 'draft:' + keyOf(e);
    o.visibility = 'everyone';
    o._draft = true;
    if (D.pinned[keyOf(e)]) o._pin = true;
    return o;
  }
  // Redraws the grid and whatever the track shows for the draft.
  P.refresh = function (o) {
    o = o || {};
    if (o.ink) { var mine = this.ink = { t0: performance.now(), lane: o.lane || null }, self = this; setTimeout(function () { if (self.ink === mine) self.ink = null; }, 700); }
    this.setPreview();
    this.view._paintMonth();
    this.render(o);
  };
  // What the page shows for the draft: the track, the button, the chosen days' row.
  P.render = function (o) {
    o = o || {};
    if (this.D && !this.isOpen) this.setOpen(true);
    this.draw(o);
    this.syncButton();
    if (this.editor.editing) this.editor._renderBar();
    this.paintSay();
  };
  P.word = function (action, label, o) {
    o = o || {};
    return '<button type="button" class="yt-word' + (o.caret ? ' yt-caret' : '') + '" data-yt="' + action + '"' + (o.pressed != null ? ' aria-pressed="' + o.pressed + '"' : '') + (o.expanded != null ? ' aria-expanded="' + o.expanded + '"' : '') + '>' + esc(label) +
      (o.caret ? '<i class="fa-solid fa-chevron-down" aria-hidden="true"></i>' : '') + '</button>';
  };

  // ==================================================================
  // What the generators make, as drafts. Pure: they take the engine (G) and
  // the calendar, so they are tested without a page.
  // ==================================================================
  function sortEvents(D) { D.ev.sort(function (a, b) { return stamp(a.year, a.month, a.day) - stamp(b.year, b.month, b.day); }); }

  // The calendar's Weather settings as a run takes them: the climate, how long
  // weather lasts and the owner's own kinds; a calendar whose settings could
  // not be read is drawn from the defaults.
  function weatherSetup(G, settings, ownKinds) {
    var climate = 'temperate', continuity = 0.55, kinds = [];
    if (settings) {
      kinds = ownKinds ? ownKinds(settings.kinds) : [];
      if (G.weather.climates().some(function (c) { return c.id === settings.climate; })) climate = settings.climate;
      var cont = +settings.continuity;
      if (isFinite(cont) && cont >= 0 && cont <= 1) continuity = cont;
    }
    var known = {};
    G.weather.presets(kinds).forEach(function (p) { known[p.id] = true; });
    return { recipe: { generator: 'weather', details: { climate: climate, continuity: continuity } }, kinds: kinds, known: known };
  }
  // One lane of a fill (happenings or sky events): pinned events stay, the rest is drawn again.
  function rollFillLane(G, cal, D, lane) {
    D.n[lane]++;
    var pins = D.ev.filter(function (e) { return e._lane === lane && D.pinned[keyOf(e)]; }).map(function (e) {
      var c = assign({}, e); c.gen = assign({}, e.gen, { locked: true }); return c;
    });
    var recipe = G.recipes.get(D.recipe[lane]);
    if (lane === 'events') recipe.details = assign({}, recipe.details, { density: +D.busy });
    var res = G.mass.fillRestOfYear({ generator: lane, calendar: cal, seed: D.seed, recipe: recipe, nonce: 'f' + D.n[lane], existing: pins });
    D.ev = D.ev.filter(function (e) { return e._lane !== lane; }).concat(res.items.map(function (e) { return assign({}, e, { _lane: lane }); }));
  }
  // The fill's weather: today to the year's end, on days that have none.
  function rollFillWeather(G, cal, D, y, stored, setup) {
    D.n.weather++;
    var existing = Object.keys(stored).map(function (k) { return asEngineDay(stored[k], setup.known); });
    var r = G.mass.fillRestOfYear({ generator: 'weather', calendar: cal, seed: D.seed + '-' + D.n.weather, existing: existing, recipe: setup.recipe, kinds: setup.kinds });
    D.wx = emptyDays(r.result.days.filter(function (d) { return d.year === y; }), function (d) { return stored[d.month + '_' + d.day]; });
  }
  // A history of the years before now. Only its events are kept; the
  // calendar's eras are never part of a draft.
  function rollHistory(G, cal, D) {
    D.n.history++;
    var h = G.history.generate({ calendar: cal, seed: D.seed + (D.n.history > 1 ? '-' + D.n.history : ''), recipe: D.recipe.history });
    D.ev = h.events.map(function (e) { return assign({}, e, { _lane: 'history' }); });
  }
  // A month's own events, copied day for day into another month.
  function buildCopy(G, cal, src, from, to) {
    var res = G.mass.copyMonth(src, { calendar: cal, from: from, to: to });
    return res.added.map(function (e, i) {
      var c = assign({}, e, { _lane: 'copy' });
      delete c.id; delete c.occurrences;
      c.gen = assign({}, e.gen, { key: 'copy:' + i });
      return c;
    });
  }
  // One event, on the same day of every other month of its year.
  function buildEvery(G, cal, ev) {
    var pattern = assign({}, ev);
    ['id', 'end_year', 'end_month', 'end_day', 'occurrences'].forEach(function (k) { delete pattern[k]; });
    var res = G.mass.everyMonth(pattern, { calendar: cal, year: ev.year, rule: 'day', day: ev.day });
    return res.items.filter(function (e) { return e.month !== ev.month; }).map(function (e, i) {
      var c = assign({}, e, { _lane: 'copy' });
      c.gen = assign({}, e.gen, { key: 'every:' + i });
      return c;
    });
  }
  // The chosen days' generated events, drawn again. saved: the saved events of
  // the years those days fall in. Only an unedited generated event of the lane
  // is replaced; a hand-made event is not even offered to the engine.
  function buildReroll(G, cal, saved, dates, seed, nonce) {
    var out = { ev: [], remove: [], made: 0, gone: 0 };
    ['events', 'sky'].forEach(function (lane) {
      var byKey = {}, have = [];
      saved.forEach(function (e) {
        if (e.is_recurring || generatedLane(e) !== lane) return;
        var c = assign({}, e, { gen: { generator: lane, key: 'saved:' + e.id, locked: false } });
        byKey[c.gen.key] = e; have.push(c);
      });
      var res = G.mass.reroll({ generator: lane, calendar: cal, seed: seed, scope: { days: dates }, existing: have, nonce: nonce });
      res.items.forEach(function (e) {
        var k = e.gen && e.gen.key;
        if (byKey[k]) delete byKey[k];
        else { out.ev.push(assign({}, e, { _lane: lane })); out.made++; }
      });
      Object.keys(byKey).forEach(function (k) { out.remove.push(byKey[k]); out.gone++; });
    });
    return out;
  }
  // The chosen days' weather, drawn again, on generated days that are not locked.
  function buildWeatherReroll(G, cal, readingOf, stored, dates, seed, nonce, setup) {
    var chosen = {};
    dates.forEach(function (d) { chosen[dkey(d.year, d.month, d.day)] = true; });
    var r = G.mass.reroll({ generator: 'weather', calendar: cal, seed: seed + '-' + nonce, scope: { days: dates }, nonce: nonce, recipe: setup.recipe, kinds: setup.kinds,
      existing: stored.map(function (w) { return asEngineDay(w, setup.known); }) });
    return redrawable(r.result.days.filter(function (d) { return chosen[dkey(d.year, d.month, d.day)]; }), readingOf);
  }
  // True when the chosen days run unbroken from the first to the last.
  function contiguous(abs) {
    var seen = {}, n = 0;
    abs.forEach(function (a) { if (!seen[a]) { seen[a] = 1; n++; } });
    return abs.length ? Math.max.apply(null, abs) - Math.min.apply(null, abs) + 1 === n : true;
  }
  function rerollNote(fx, first, last, nDays, gone, made, days) {
    var what = nDays > 1 && !fx.contiguous ? 'the ' + nDays + ' chosen days' : fx.fmt(first) + (nDays > 1 ? ' to ' + fx.fmt(last) : '');
    return 'Rolls ' + what + ' again: ' + count(gone, 'generated event') + ' replaced by ' + (WORDS[made] || made) + ' new, and ' + count(days, 'day') + ' of weather redrawn. Events you made by hand stay.';
  }

  // ==================================================================
  // Starting a draft
  // ==================================================================
  // A tool that must load the generator shows it is working at once.
  P.setWorking = function (btn, on) {
    this.busy = on;
    if (btn) { btn.setAttribute('aria-busy', String(on)); btn.disabled = on; btn.classList.toggle('yt-working', on); }
    this.syncRest();
  };
  P.fail = function (msg) {
    var v = this.view;
    return function (err) { v.say(err && err.friendly && err.message ? err.message : msg); };
  };
  // The generator, the weather settings and the years' events and weather,
  // loaded; resolves with the settings (null if they could not be read).
  P.prepare = function (years) {
    var self = this, v = this.view;
    return new Promise(function (res, rej) {
      self.editor.kit().loadEngine(v.engineSrc, res, function () { rej({ friendly: true, message: 'The generator could not load. Check your connection and try again.' }); });
    }).then(function () {
      var settings = Chronicle.calendarWeatherSettings ? Chronicle.calendarWeatherSettings(v) : Promise.resolve(null);
      return Promise.all([Promise.all(years.map(function (y) { return self.ensureYear(y); })), settings]);
    }).then(function (r) { return r[1]; });
  };
  P.setup = function (settings) { return weatherSetup(window.ChronicleGen, settings, Chronicle.calendarWeatherOwnKinds); };
  P.fx = function () { var self = this; return { fmt: function (d) { return self.fmt(d); }, contiguous: true }; };

  P.begin = function (D, o) {
    this.D = D; sortEvents(D);
    if (this.view._toastEl) this.view._toastEl.classList.remove('on');
    this.refresh(assign({ ink: true, fresh: true }, o || {}));
    setTimeout(this.reveal.bind(this), 360);
  };
  P.fill = function (btn) {
    if (this.D || this.busy || !this.canGen) return;
    var self = this, now = this.now(), y = now.year;
    this.setWorking(btn, true);
    this.prepare([y]).then(function (settings) {
      var G = window.ChronicleGen, cal = self.cal(), D = blankDraft('fill', randomSeed()), setup = self.setup(settings);
      D.setup = setup;
      D.remove = fillReplaces(self.savedEvents(y), now);
      D.retire = fillRetires(self.savedEvents(y), now);
      rollFillLane(G, cal, D, 'events'); rollFillLane(G, cal, D, 'sky');
      rollFillWeather(G, cal, D, y, self.view.weatherByYear[y] || {}, setup);
      self.goTo(y, now.month);
      self.begin(D);
    }).catch(this.fail('The rest of the year could not be filled. Nothing was changed.')).then(function () { self.setWorking(btn, false); });
  };
  P.startHistory = function () {
    if (this.D || this.busy || !this.canGen) return;
    var self = this;
    this.busy = true;
    this.prepare([this.now().year]).then(function () {
      var D = blankDraft('history', randomSeed());
      rollHistory(window.ChronicleGen, self.cal(), D);
      self.begin(D);
    }).catch(this.fail('A history could not be written. Nothing was changed.')).then(function () { self.busy = false; self.syncRest(); });
  };
  P.copyMonth = function (to) {
    if (this.D || this.busy || !this.canGen) return;
    var self = this, v = this.view, from = { year: v.view.y, month: v.view.m };
    this.busy = true;
    this.prepare([from.year]).then(function () {
      var src = self.monthEventsToCopy(from.year, from.month), D = blankDraft('copy', randomSeed());
      D.ev = buildCopy(window.ChronicleGen, self.cal(), src, from, { year: from.year, month: to });
      D.note = 'Copies ' + count(D.ev.length, 'event') + ' from ' + self.mName(from.month) + ' to ' + self.mName(to) + '.';
      self.goTo(from.year, to);
      self.begin(D);
    }).catch(this.fail('That month could not be copied. Nothing was changed.')).then(function () { self.busy = false; self.syncRest(); });
  };
  P.everyMonth = function (ev) {
    if (this.D || this.busy || !this.canGen) return;
    var self = this;
    this.busy = true;
    this.prepare([ev.year]).then(function () {
      var D = blankDraft('every', randomSeed());
      D.ev = buildEvery(window.ChronicleGen, self.cal(), ev);
      D.note = 'Repeats ' + ev.name + ' on day ' + ev.day + ' of every other month of ' + self.yearName(ev.year) + ': ' + count(D.ev.length, 'more event') + '.';
      self.begin(D);
    }).catch(this.fail('That event could not be repeated. Nothing was changed.')).then(function () { self.busy = false; });
  };
  P.rerollDays = function (dates) {
    if (this.D || this.busy || !this.canGen || !dates.length) return;
    var self = this, years = [];
    dates.forEach(function (d) { if (years.indexOf(d.year) < 0) years.push(d.year); });
    this.busy = true;
    this.prepare(years).then(function (settings) {
      var G = window.ChronicleGen, calObj = G.cal.from(self.cal()), v = self.view, nonce = 'again' + Date.now().toString(36), seed = randomSeed();
      var abs = dates.map(function (d) { return calObj.abs(d.year, d.month, d.day); }).sort(function (a, b) { return a - b; });
      var saved = [];
      years.forEach(function (y) { saved = saved.concat(self.savedEvents(y)); });
      var D = blankDraft('reroll', seed), ev = buildReroll(G, self.cal(), saved, dates, seed, nonce), setup = self.setup(settings);
      var readingOf = function (d) { return (v.weatherByYear[d.year] || {})[d.month + '_' + d.day]; };
      var stored = [];
      years.forEach(function (y) { var by = v.weatherByYear[y] || {}; Object.keys(by).forEach(function (k) { stored.push(by[k]); }); });
      D.ev = ev.ev; D.remove = ev.remove;
      D.wx = buildWeatherReroll(G, self.cal(), readingOf, stored, dates, seed, nonce, setup);
      // What the redrawn days held, so Undo can put it back.
      D.wx.forEach(function (d) { D.wxPrev[dkey(d.year, d.month, d.day)] = self.editor.kit().dayInput(readingOf(d)); });
      var fx = assign(self.fx(), { contiguous: contiguous(abs) });
      D.note = rerollNote(fx, calObj.fromAbs(abs[0]), calObj.fromAbs(abs[abs.length - 1]), abs.length, ev.gone, ev.made, D.wx.length);
      self.begin(D);
    }).catch(this.fail('Those days could not be rolled again. Nothing was changed.')).then(function () { self.busy = false; });
  };

  // ---- changing a draft ----
  // Rolls the draft again: the old marks leave quickly, then the new ones ink in.
  P.reroll = function (lane) {
    var D = this.D, self = this;
    if (!D || this.busy) return;
    var G = window.ChronicleGen, cal = this.cal(), y = this.now().year;
    try {
      if (D.kind === 'history') rollHistory(G, cal, D);
      else if (D.kind === 'fill') {
        (lane ? [lane] : ['events', 'sky', 'weather']).forEach(function (l) {
          if (!D.on[l]) return;
          if (l === 'weather') rollFillWeather(G, cal, D, y, self.view.weatherByYear[y] || {}, D.setup);
          else rollFillLane(G, cal, D, l);
        });
      } else return;
    } catch (err) { this.fail('That could not be drawn again.')(err); return; }
    sortEvents(D);
    var root = document.documentElement;
    root.setAttribute('data-yt-roll', lane || 'all');
    wait(80).then(function () { root.removeAttribute('data-yt-roll'); self.refresh({ ink: true, lane: lane, again: true }); });
  };
  P.toggleLane = function (lane, on) { var D = this.D; if (!D) return; D.on[lane] = on; this.refresh({ lane: lane, toggled: true }); };
  P.pin = function (k) { var D = this.D; if (!D) return; if (D.pinned[k]) delete D.pinned[k]; else { D.pinned[k] = true; delete D.dropped[k]; } this.refresh({ soft: true }); };
  P.drop = function (k) { var D = this.D; if (!D) return; if (D.dropped[k]) delete D.dropped[k]; else { D.dropped[k] = true; delete D.pinned[k]; } this.refresh({ soft: true }); };
  P.discard = function () {
    var self = this;
    if (!this.D || this.busy) return;
    document.documentElement.setAttribute('data-yt-roll', 'all');
    wait(120).then(function () {
      document.documentElement.removeAttribute('data-yt-roll');
      self.D = null; self.setPreview(); self.view._paintMonth(); self.render({ discarded: true });
    });
  };

  // ==================================================================
  // The draft's own block: one sentence, what it makes, the reading list,
  // Roll again / Discard / Keep. It folds open with a draft and away when
  // the draft is kept or discarded.
  // ==================================================================
  function segHTML(label, action, pairs, current) {
    return '<div class="seg" role="group" aria-label="' + esc(label) + '">' + pairs.map(function (p) {
      return '<button type="button" data-yt="' + action + ':' + p[0] + '" aria-pressed="' + (p[0] === current) + '">' + esc(p[1]) + '</button>';
    }).join('') + '</div>';
  }
  function kindWord(e) {
    var k = e._lane === 'sky' ? 'In the sky' : '';
    if (e.is_recurring) k = (k ? k + ', ' : '') + (e.recurrence_type === 'weekly' ? 'every week' : e.recurrence_type === 'monthly' ? 'every month' : 'every year');
    return k;
  }
  P.monthWeather = function (D, m) {
    var days = activeWeather(D).filter(function (w) { return w.month === m; });
    if (!days.length) return '';
    var by = {}, lo = 99, hi = -99;
    days.forEach(function (w) { by[w.preset_label] = (by[w.preset_label] || 0) + 1; lo = Math.min(lo, w.temperature_celsius); hi = Math.max(hi, w.temperature_celsius); });
    var top = Object.keys(by).sort(function (a, b) { return by[b] - by[a]; });
    return (days.length > 1 ? 'Mostly ' + String(top[0]).toLowerCase() + (top[1] ? ', some ' + String(top[1]).toLowerCase() : '') : top[0]) + ', ' + Math.round(lo) + ' to ' + Math.round(hi) + '°C';
  };
  P.rowHTML = function (D, e) {
    var k = keyOf(e), out = !!D.dropped[k], pinned = !!D.pinned[k], can = D.kind === 'fill' || D.kind === 'history', kw = kindWord(e);
    return '<li class="yt-li' + (out ? ' out' : '') + (pinned ? ' pin' : '') + '">' +
      '<button type="button" class="yt-rd" data-yt="go:' + e.year + '_' + e.month + '_' + e.day + '" title="Show ' + esc(this.fmt(e)) + ' on the calendar">' +
        (D.kind === 'history' ? '<small>' + esc(e.year) + '</small>' : esc(e.day)) + '</button>' +
      '<div class="yt-rb"><b>' + esc(e.name) + '</b>' + (kw ? '<span class="yt-rk">' + esc(kw) + '</span>' : '') +
        (e.description ? '<p>' + esc(e.description) + '</p>' : '') + '</div>' +
      (can ? '<div class="yt-ra">' + (D.kind === 'fill' ? this.word('pin:' + k, pinned ? 'Pinned' : 'Pin', { pressed: pinned }) : '') + this.word('drop:' + k, out ? 'Put back' : 'Leave out') + '</div>' : '') +
    '</li>';
  };
  // The draft, read in order: month by month, or era by era for a history.
  P.listHTML = function (D) {
    var self = this, evs = D.ev.filter(function (e) { return D.kind !== 'fill' || D.on[e._lane]; }), html = '';
    var rows = function (list) { return '<ul>' + list.map(function (e) { return self.rowHTML(D, e); }).join('') + '</ul>'; };
    if (D.kind === 'history') {
      var seen = {};
      this.erasOf().forEach(function (era) {
        var inEra = evs.filter(function (e) { return self.inEra(e, era); });
        inEra.forEach(function (e) { seen[keyOf(e)] = 1; });
        if (!inEra.length) return;
        html += '<section><h4 class="yt-lh"><span>' + esc(era.name) + '</span><small>' + inEra.length + '</small></h4>' + rows(inEra) + '</section>';
      });
      var rest = evs.filter(function (e) { return !seen[keyOf(e)]; });
      if (rest.length) html = '<section><h4 class="yt-lh"><span>Before your eras</span><small>' + rest.length + '</small></h4>' + rows(rest) + '</section>' + html;
      return '<div class="yt-list">' + html + '</div>';
    }
    var y = this.now().year;
    (this.cal().months || []).forEach(function (M, i) {
      var m = i + 1, inMonth = evs.filter(function (e) { return e.month === m; }), wx = D.kind === 'fill' ? self.monthWeather(D, m) : '';
      if (!inMonth.length && !wx) return;
      var live = inMonth.filter(function (e) { return !D.dropped[keyOf(e)]; }).length;
      html += '<section><h4 class="yt-lh"><button type="button" class="yt-lm" data-yt="month:' + y + '_' + m + '" title="Turn the calendar to ' + esc(M.name) + '">' + esc(M.name) + '</button><small>' + (inMonth.length ? live : '') + '</small></h4>' +
        (wx ? '<p class="yt-ld">' + esc(wx) + '</p>' : '') + rows(inMonth) + '</section>';
    });
    return '<div class="yt-list">' + html + '</div>';
  };
  // Each line is one choice, and "None" is how a part is left out.
  P.makesHTML = function (D) {
    var row = function (label, body) { return '<div class="yt-mk"><span class="yt-ml">' + label + '</span>' + body + '</div>'; };
    if (D.kind === 'history') return row('The history of', segHTML('The history of', 'recipe:history', RECIPES.history, D.recipe.history));
    var none = [['none', 'None']];
    return row('Happenings', segHTML('Happenings', 'recipe:events', none.concat(RECIPES.events), D.on.events ? D.recipe.events : 'none')) +
      (D.on.events ? row('How busy', segHTML('How busy', 'busy', BUSY, D.busy)) : '') +
      row('Sky events', segHTML('Sky events', 'recipe:sky', none.concat(RECIPES.sky), D.on.sky ? D.recipe.sky : 'none')) +
      row('Weather', segHTML('Weather', 'recipe:weather', none.concat([['climate', 'From your climate']]), D.on.weather ? 'climate' : 'none')) +
      (D.on.events || D.on.sky ? row('Players see them', segHTML('Players see them', 'see', SEE, D.see)) : '');
  };
  P.actsHTML = function (D) {
    var canRoll = D.kind === 'fill' || D.kind === 'history', c = counts(D), none = !c.all && !c.weather && !removalsOf(D).length && !retiresOf(D).length;
    return (canRoll ? '<button type="button" class="btn sm" data-yt="roll">Roll again</button>' : '') +
      '<button type="button" class="btn sm quiet" data-yt="discard">Discard</button>' +
      '<button type="button" class="btn sm primary" data-yt="keep"' + (none ? ' disabled title="There is nothing in this draft to keep"' : '') + '>' + esc(keepLabel(D)) + '</button>';
  };
  P.barHTMLOf = function (D) {
    var open = this.folds, n = D.ev.filter(function (e) { return D.kind !== 'fill' || D.on[e._lane]; }).length, big = D.kind === 'fill' || D.kind === 'history';
    return '<div class="yt-draft" data-kind="' + D.kind + '">' +
      '<p class="yt-say">' + esc(sentence(D, this.ctx())) + ' <span class="yt-nosave">Nothing is saved until you keep it.</span></p>' +
      '<div class="yt-acts">' + (big ? this.word('fold:makes', 'Change what it makes', { caret: true, expanded: !!open.makes }) : '') +
        (n ? this.word('fold:list', 'Read all ' + n, { caret: true, expanded: !!open.list }) : '') +
        '<span class="yt-sp"></span>' + this.actsHTML(D) + '</div>' +
      '<div class="yt-fold yt-makes" data-fold="makes"' + (open.makes ? '' : ' hidden') + '>' + (big ? this.makesHTML(D) : '') + '</div>' +
      '<div class="yt-fold" data-fold="list"' + (open.list ? '' : ' hidden') + '>' + this.listHTML(D) + '</div>' +
    '</div>';
  };
  P.renderBar = function (host) {
    var D = this.D, self = this;
    if (!D) {
      if (this.barNode) { var n = this.barNode; this.barNode = null; this.folds = {}; n.inert = true; fold(n, false).then(function () { n.remove(); }); }
      return;
    }
    if (!this.barNode) {
      this.barNode = el('<div class="yt-barwrap" hidden></div>');
      host.appendChild(this.barNode);
      this.barNode.innerHTML = this.barHTMLOf(D);
      this.barNode.addEventListener('click', function (e) {
        var t = e.target.closest('[data-yt]');
        if (!t || t.disabled || self.busy) return;
        var a = t.dataset.yt;
        if (a.indexOf('fold:') === 0) { self.toggleFold(a.slice(5)); return; }
        self.act(a);
      });
      fold(this.barNode, true);
      return;
    }
    var list = $('.yt-list', this.barNode), top = list ? list.scrollTop : 0;
    var focus = document.activeElement && this.barNode.contains(document.activeElement) ? document.activeElement.dataset.yt : null;
    this.barNode.classList.remove('yt-working');
    this.barNode.innerHTML = this.barHTMLOf(D);
    list = $('.yt-list', this.barNode); if (list) list.scrollTop = top;
    if (focus) { var f = $('[data-yt="' + focus + '"]', this.barNode); if (f) f.focus({ preventScroll: true }); }
  };
  // One fold at a time, so the drawer never grows tall.
  P.toggleFold = function (name) {
    var self = this, on = !this.folds[name];
    ['makes', 'list'].forEach(function (k) {
      var want = k === name ? on : false;
      if (!!self.folds[k] === want) return;
      self.folds[k] = want;
      var node = $('[data-fold="' + k + '"]', self.barNode), btn = $('[data-yt="fold:' + k + '"]', self.barNode);
      if (btn) btn.setAttribute('aria-expanded', String(want));
      if (node) fold(node, want);
    });
    // The drawer just grew or shrank, so the new marks move clear of it again.
    setTimeout(this.reveal.bind(this), 360);
  };
  // One click handler for every draft control, wherever it is drawn.
  P.act = function (a) {
    var D = this.D, p = a.split(':');
    if (p[0] === 'keep') this.keep();
    else if (p[0] === 'discard') this.discard();
    else if (p[0] === 'roll') this.reroll(p[1]);
    else if (p[0] === 'pin') this.pin(p.slice(1).join(':'));
    else if (p[0] === 'drop') this.drop(p.slice(1).join(':'));
    else if (p[0] === 'go') { var d = p[1].split('_').map(Number); this.showDay(d[0], d[1], d[2]); }
    else if (p[0] === 'month') { var ym = p[1].split('_').map(Number); this.goTo(ym[0], ym[1]); }
    else if (p[0] === 'recipe' && D) {
      var lane = p[1], val = p.slice(2).join(':');
      if (lane === 'history') { if (D.recipe.history !== val) { D.recipe.history = val; this.reroll(); } }
      else if (val === 'none') { if (D.on[lane]) this.toggleLane(lane, false); }
      else if (!D.on[lane]) { if (lane !== 'weather') D.recipe[lane] = val; D.on[lane] = true; this.reroll(lane); }
      else if (lane !== 'weather' && D.recipe[lane] !== val) { D.recipe[lane] = val; this.reroll(lane); }
    }
    else if (p[0] === 'see' && D) { D.see = p[1] === 'day' ? 'day' : 'later'; this.refresh({ soft: true }); }
    else if (p[0] === 'busy' && D) { if (D.busy !== p[1]) { D.busy = p[1]; this.reroll('events'); } }
  };

  // ==================================================================
  // Keeping a draft, and undoing it
  // ==================================================================
  function laneOf(e) { return e._lane === 'copy' ? null : e._lane; }
  // The batch route takes a whole draft, or an Undo, and saves it in one
  // transaction: all of it or none. It counts as one write against the
  // server's per-minute budget.
  function batchError(resp) {
    var e = new Error('batch');
    e.status = resp.status;
    return e;
  }
  // What Keep sends: the new events, the repeating events a fill ends (each
  // is an update that sets its repeat-until date), the ones it replaces and
  // the weather.
  function repeatEnd(id, d) {
    return { id: id, changes: { recurrence_end_year: d ? d.year : null, recurrence_end_month: d ? d.month : null, recurrence_end_day: d ? d.day : null } };
  }
  function keepBatchBody(items, removed, wxPut, retired, endOn) {
    var b = { create: items.map(function (it) { return it.body; }), delete: removed.map(function (e) { return e.id; }) };
    if (retired && retired.length && endOn) b.update = retired.map(function (e) { return repeatEnd(e.id, endOn); });
    if (wxPut.length) b.weather = { set: wxPut };
    return b;
  }
  // What an Undo needs to put a retired event's repeat back: the end it had
  // (null where it had none, which the server reads as clearing it).
  function endOf(e) {
    return e.recurrence_end_year != null && e.recurrence_end_month != null && e.recurrence_end_day != null
      ? { year: e.recurrence_end_year, month: e.recurrence_end_month, day: e.recurrence_end_day } : null;
  }
  // What Undo sends: take away what Keep made, put back what it replaced,
  // give an ended repeat its old end again, and clear then restore the
  // weather days it wrote.
  function undoBatchBody(rec) {
    var b = {
      create: rec.removed.map(function (e) { return createBody(e, { keepPayload: true }); }),
      delete: rec.created.map(function (c) { return c.id; })
    };
    if (rec.retired && rec.retired.length) b.update = rec.retired.map(function (r) { return repeatEnd(r.id, r.was); });
    if (rec.wx.length) {
      var back = [];
      rec.wx.forEach(function (d) { var p = rec.prev[dkey(d.year, d.month, d.day)]; if (p) back.push(p); });
      b.weather = { clear: rec.wx, set: back };
    }
    return b;
  }
  P.api = function (path, method, body) { return Chronicle.apiFetch(this.view.apiBase + path, { method: method, body: body }); };
  P.batch = function (body) {
    return this.api('/events/batch', 'POST', body).then(function (resp) { if (!resp.ok) throw batchError(resp); return resp.json(); });
  };
  // Keep shows it is working at once: every control holds still and the button says so.
  P.workingBar = function () {
    var n = this.barNode;
    if (!n) return;
    n.classList.add('yt-working');
    $$('button', n).forEach(function (b) { b.disabled = true; });
    var k = $('[data-yt="keep"]', n);
    if (k) { k.setAttribute('aria-busy', 'true'); k.textContent = 'Keeping…'; }
  };
  // Forgets what the view holds, so the next read is what the server has.
  P.flush = function () {
    var v = this.view;
    v.eventsByMonth = {}; v.rollsByMonth = {}; v.weatherByYear = {};
    if (v.canAuthorDmOnly && v.fetchForecast) v.fetchForecast(true);
  };
  P.reload = function () {
    var v = this.view;
    this.flush();
    return Promise.all([this.ensureYear(this.now().year), v.fetchMonth(v.view.y, v.view.m), v.fetchWeatherYear(v.view.y)]);
  };

  P.keep = function () {
    var self = this, D = this.D, v = this.view, G = window.ChronicleGen;
    if (!D || this.busy) return;
    var evs = activeEvents(D), wx = activeWeather(D), removed = removalsOf(D).slice();
    // A repeat is ended the day before today; with no such day (today is the
    // year's first) nothing before today can be repeating, so none is ended.
    var now = this.now(), c = this.cal(), endOn = dayBefore(now, function (i, yr) { return CD().monthDays(c, i, yr); });
    var retired = endOn ? retiresOf(D).slice() : [];
    if (!evs.length && !wx.length && !removed.length && !retired.length) return;
    if (evs.length + removed.length + retired.length > BATCH_MAX_EVENTS || wx.length > BATCH_MAX_DAYS) {
      v.say('This draft is too big to keep in one go. Switch a part off, or discard it and start smaller.');
      return;
    }
    this.busy = true;
    this.workingBar();
    var rec = { created: [], removed: removed, retired: retired.map(function (e) { return { id: e.id, was: endOf(e) }; }), wx: [], prev: D.wxPrev || {} }, ok = false, status = 0;
    var items = evs.map(function (e) { return { e: e, body: createBody(e, { lane: laneOf(e), onDay: D.see === 'day' }) }; });
    var put = wx.map(function (w2) {
      var b = G.weather.toWeatherInput(w2);
      b.year = w2.year; b.month = w2.month; b.day = w2.day; b.source = 'generated';
      return b;
    });
    var body = keepBatchBody(items, removed, put, retired, endOn);
    if (JSON.stringify(body).length > BATCH_MAX_BYTES) {
      this.busy = false;
      this.refresh({ soft: true });
      v.say('This draft is too big to keep in one go. Switch a part off, or discard it and start smaller.');
      return;
    }
    this.batch(body).then(function (res) {
      ok = true;
      (res.created || []).forEach(function (id, i) {
        var it = items[i];
        rec.created.push({ id: id, key: keyOf(it.e), year: it.body.year, month: it.body.month, day: it.body.day });
      });
      rec.wx = wx.map(function (w2) { return { year: w2.year, month: w2.month, day: w2.day }; });
    }, function (err) { status = err && err.status || 0; })
      .then(function () { return self.reload(); })
      .then(function () { self.afterKeep(rec, ok, status); });
  };
  // What is left of the draft after Keep, and what to tell the owner. A kept
  // draft is gone; one that did not save is untouched.
  P.afterKeep = function (rec, ok, status) {
    var self = this, v = this.view;
    var msg;
    if (ok) {
      msg = keptWords(rec.created.length, rec.wx.length);
      this.D = null;
    } else {
      msg = notKeptWords(status);
      // With no answer from the server the draft may be saved already, and
      // keeping it again would save it twice; the calendar now shows the truth.
      if (!status) this.D = null;
    }
    var glow = { ids: {}, wx: {}, days: {} };
    rec.created.forEach(function (c) { glow.ids[c.id] = 1; glow.days[dkey(c.year, c.month, c.day)] = 1; });
    rec.wx.forEach(function (d) { glow.wx[dkey(d.year, d.month, d.day)] = 1; });
    this.glow = glow;
    setTimeout(function () { if (self.glow === glow) self.glow = null; }, 1700);
    this.busy = false;
    this.setPreview();
    this.render({ kept: ok });
    v.renderMonth();
    var did = ok && (rec.created.length || rec.removed.length || rec.retired.length || rec.wx.length);
    v.say(msg, did ? { label: 'Undo', run: function () { self.undo(rec); } } : null);
  };

  // Undo takes away what Keep created, puts back what it replaced, and clears
  // or restores the weather days it wrote, in one request.
  P.undo = function (rec) {
    var self = this, v = this.view;
    if (this.busy) { v.say('Wait a moment, then press Undo again.'); return; }
    this.busy = true;
    var failed = false, status = 0;
    this.batch(undoBatchBody(rec)).catch(function (err) { failed = true; status = err && err.status || 0; })
      .then(function () { return self.reload(); })
      .then(function () {
        self.busy = false;
        self.setPreview();
        self.render({});
        v.renderMonth();
        v.say(failed ? notUndoneWords(status) : 'Undone. The calendar is as it was.');
      });
  };

  // ==================================================================
  // Attaching to the editor
  // ==================================================================
  window.Chronicle = window.Chronicle || {};
  Chronicle.calendarYearTrack = {
    // calendar_editor.js calls this once per editor and keeps the result.
    attach: function (editor) { return new YearTrack(editor); }
  };

  // Pure helpers, exposed for test/js/calendar_year_track.test.mjs.
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
      geometry: geometry, windowFor: windowFor, nowAt: nowAt, seasonSpans: seasonSpans, runsOf: runsOf, yearEventsFrom: yearEventsFrom,
      hash: hash, sigOf: sigOf, markerFor: markerFor, generatedLane: generatedLane, isHandMade: isHandMade, fillReplaces: fillReplaces, fillRetires: fillRetires, dayBefore: dayBefore, retiresOf: retiresOf, removalsOf: removalsOf,
      protectedDay: protectedDay, emptyDays: emptyDays, redrawable: redrawable, asEngineDay: asEngineDay,
      blankDraft: blankDraft, activeEvents: activeEvents, activeWeather: activeWeather, counts: counts, sentence: sentence, keepLabel: keepLabel,
      keptWords: keptWords, notKeptWords: notKeptWords, notUndoneWords: notUndoneWords, keepBatchBody: keepBatchBody, undoBatchBody: undoBatchBody, createBody: createBody, chosenActions: chosenActions, mayGenerate: mayGenerate,
      weatherSetup: weatherSetup, rollFillLane: rollFillLane, rollFillWeather: rollFillWeather, rollHistory: rollHistory,
      buildCopy: buildCopy, buildEvery: buildEvery, buildReroll: buildReroll, buildWeatherReroll: buildWeatherReroll,
      contiguous: contiguous, rerollNote: rerollNote, BATCH_MAX_EVENTS: BATCH_MAX_EVENTS, BATCH_MAX_DAYS: BATCH_MAX_DAYS, YearTrack: YearTrack
    };
  }
})();
