/**
 * calendar_view.js — the calendar's own page. Renders the month grid, the
 * day-card and era-card fold-outs, event icons/glance/full-detail, moons
 * (silhouette + moon view), and the calendar-switcher hub popover —
 * everything the read-only (Player) surface needs. Ported from the two
 * operator-signed mockups' CSS/markup contract
 * (internal/plugins/calendar/.ai.md documents the port and its honest gaps);
 * the JS itself is a fresh implementation against Chronicle's real API
 * (the mockups' own JS runs against synthetic sample data with a different
 * field vocabulary, so it is not something to copy statement-for-statement).
 *
 * Mount: data-widget="calendar_view"
 * Config (see view.templ's mount div):
 *   data-campaign-id, data-calendar-id, data-can-edit, data-can-author-dm-only,
 *   data-role (numeric campaigns.Role), data-api-base
 *
 * calendar_editor.js (loaded only when data-can-edit="true") augments this
 * widget after it mounts: it reads `el.calendarView` (the API this file
 * attaches to the mount element once init() finishes) or listens for the
 * 'calendarv5:ready' event if it runs first for any reason.
 */
(function () {
  'use strict';

  // ---------------------------------------------------------------------
  // Small date-math module, ported term-for-term from
  // internal/plugins/calendar/model.go so a recurring event's occurrence
  // days, the weekday grid column and moon phase can never disagree with
  // the server. dayIndex mirrors Calendar.absDayIndex exactly (a
  // real-time calendar's Gregorian JDN; the leap-unaware linear counter
  // for year <= 0, where AbsoluteDay's leap term is undefined; the
  // leap-aware absoluteDay otherwise) and is the one day counter every
  // caller here goes through — recurrence base/target, the weekday
  // column, span containment, past/today comparison — the same
  // reconciliation Calendar.absDayIndex's own doc comment describes
  // server-side. weekdayCol additionally mirrors WeekdayIndex's
  // MonthStartsNewWeek branch (day 1 of every month restarts the week; a
  // day inside an intercalary month on such a calendar belongs to no
  // week at all, signaled -1 — see view_helpers.go's buildMonthGrid for
  // the server's own "place it from the first column" handling of that).
  // ---------------------------------------------------------------------
  var CalDate = {
    mod: function (a, n) { return ((a % n) + n) % n; },

    usesRealTime: function (cal) {
      // Mirrors Calendar.UsesRealTime exactly: Mode == reallife AND
      // TracksRealTime, not mode alone. A manual (non-real-time) reallife
      // calendar keeps its stored month/weekday geometry server-side (its
      // own doc comment: "behaves exactly as before real-time support
      // existed"), so mode alone would show a 29 February and JDN-based
      // weekdays the server never produces for that calendar.
      return cal.mode === 'reallife' && !!cal.tracks_real_time;
    },

    monthDaysNative: function (year, month1) {
      return new Date(Date.UTC(year, month1, 0)).getUTCDate();
    },

    yearLength: function (cal) {
      var total = 0;
      (cal.months || []).forEach(function (m) { total += m.days; });
      return total;
    },

    isLeapYear: function (cal, year) {
      if (!cal.leap_year_every || cal.leap_year_every <= 0) return false;
      return CalDate.mod(year - cal.leap_year_offset, cal.leap_year_every) === 0;
    },

    yearLengthForYear: function (cal, year) {
      var total = 0, leap = CalDate.isLeapYear(cal, year);
      (cal.months || []).forEach(function (m) { total += m.days; if (leap) total += (m.leap_year_days || 0); });
      return total;
    },

    monthDays: function (cal, month0, year) {
      var months = cal.months || [];
      if (month0 < 0 || month0 >= months.length) return 0;
      if (CalDate.usesRealTime(cal)) return CalDate.monthDaysNative(year, month0 + 1);
      var days = months[month0].days;
      if (CalDate.isLeapYear(cal, year)) days += (months[month0].leap_year_days || 0);
      return days;
    },

    // constLenDayIndex mirrors Calendar.linearDayIndex (model.go): a
    // constant-length counter (no leap adjustment at all), month 1-based —
    // dayIndex's fallback for year <= 0, see its own doc comment.
    constLenDayIndex: function (cal, year, month1, day) {
      var months = cal.months || [];
      var abs = year * CalDate.yearLength(cal);
      for (var i = 0; i < month1 - 1 && i < months.length; i++) abs += months[i].days;
      return abs + day;
    },

    leapExtraDays: function (cal) {
      var total = 0;
      (cal.months || []).forEach(function (m) { total += (m.leap_year_days || 0); });
      return total;
    },

    leapYearsBefore: function (cal, year) {
      var e = cal.leap_year_every;
      if (!e || e <= 0 || year <= 0) return 0;
      var r = CalDate.mod(cal.leap_year_offset, e);
      if (year <= r) return 0;
      return Math.floor((year - 1 - r) / e) + 1;
    },

    // absoluteDay mirrors Calendar.AbsoluteDay: leap-aware, and dayIndex's
    // count for every year after 0. Moon phases go through dayIndex, like
    // weekdays, because a real-time calendar's real Moon is anchored to the
    // Julian Day Number, not to this count.
    absoluteDay: function (cal, year, month1, day) {
      var total = 0;
      if (year > 0) {
        total = year * CalDate.yearLength(cal);
        var extra = CalDate.leapExtraDays(cal);
        if (extra) total += extra * CalDate.leapYearsBefore(cal, year);
      }
      var months = cal.months || [];
      for (var i = 0; i < month1 - 1 && i < months.length; i++) total += CalDate.monthDays(cal, i, year);
      return total + day;
    },

    // gregorianJDN mirrors gregorian.go's gregorianJDN exactly (the
    // Fliegel-Van Flandern integer formula) — the real-time day counter
    // Calendar.absDayIndex switches to for a real-time-tracked calendar.
    // Math.floor mirrors Go's truncating integer division, which floors for
    // the non-negative operands every real calendar date produces here.
    gregorianJDN: function (y, m, d) {
      var a = Math.floor((14 - m) / 12);
      var yy = y + 4800 - a;
      var mm = m + 12 * a - 3;
      return d + Math.floor((153 * mm + 2) / 5) + 365 * yy + Math.floor(yy / 4) - Math.floor(yy / 100) + Math.floor(yy / 400) - 32045;
    },

    // dayIndex mirrors Calendar.absDayIndex exactly: the real-time branch
    // (JDN) for a real-time-tracked calendar; constLenDayIndex (leap-unaware)
    // for year <= 0, where AbsoluteDay's leap term is undefined (its own doc
    // comment: "a negative or zero year contributes nothing"); the leap-aware
    // absoluteDay for every other year. Every caller that needs "the" day
    // counter (recurrence base/target, the weekday column, span containment,
    // past/today comparison) goes through this, so none of them can drift
    // from what the server computes across an elapsed leap year.
    dayIndex: function (cal, year, month1, day) {
      if (CalDate.usesRealTime(cal)) return CalDate.gregorianJDN(year, month1, day);
      if (year <= 0) return CalDate.constLenDayIndex(cal, year, month1, day);
      return CalDate.absoluteDay(cal, year, month1, day);
    },

    weekLen: function (cal) { return (cal.weekdays || []).length || 7; },

    // monthIsIntercalary mirrors Calendar.monthIsIntercalary: whether the
    // 1-based month index refers to an intercalary (festival) month. An
    // out-of-range month is not intercalary, matching the Go guard.
    monthIsIntercalary: function (cal, month1) {
      var months = cal.months || [];
      var i = month1 - 1;
      if (i < 0 || i >= months.length) return false;
      return !!months[i].is_intercalary;
    },

    // weekdayCol mirrors Calendar.WeekdayIndex exactly, including its
    // MonthStartsNewWeek branch: on such a calendar, day 1 of every month
    // restarts the week (it never inherits an offset from the month
    // before), and a day inside an INTERCALARY month belongs to no week at
    // all — -1, which a grid renderer must place from its own first column
    // (view_helpers.go's buildMonthGrid does exactly that for the server's
    // preview grid; _paintMonth below mirrors it).
    weekdayCol: function (cal, year, month1, day) {
      var wl = CalDate.weekLen(cal);
      if (wl <= 0) return 0;
      if (cal.month_starts_new_week && !CalDate.usesRealTime(cal)) {
        if (CalDate.monthIsIntercalary(cal, month1)) return -1;
        return CalDate.mod(day - 1, wl);
      }
      return CalDate.mod(CalDate.dayIndex(cal, year, month1, day), wl);
    },

    monthCount: function (cal) { return (cal.months || []).length || 12; },

    // addDays walks {y,m,d} (month 1-based) forward/backward by delta real
    // days, using the leap-aware month lengths — the same "add N days"
    // Calendar.AbsoluteDay's callers would use, just inverted by stepping
    // rather than closed-form (delta is always UI-scale here: a bulk shift
    // of a bounded day range, never attacker-controlled).
    addDays: function (cal, date, delta) {
      var y = date.y, m = date.m, d = date.d, mc = CalDate.monthCount(cal);
      var n = Math.abs(delta), step = delta > 0 ? 1 : -1, i;
      for (i = 0; i < n; i++) {
        if (step > 0) {
          var len = CalDate.monthDays(cal, m - 1, y);
          d++;
          if (d > len) { d = 1; m++; if (m > mc) { m = 1; y++; } }
        } else {
          d--;
          if (d < 1) { m--; if (m < 1) { m = mc; y--; } d = CalDate.monthDays(cal, m - 1, y); }
        }
      }
      return { y: y, m: m, d: d };
    },

    shiftMonth: function (cal, year, month1, delta) {
      var mc = CalDate.monthCount(cal), m = month1 - 1 + delta, y = year;
      while (m < 0) { m += mc; y--; }
      while (m >= mc) { m -= mc; y++; }
      return { y: y, m: m + 1 };
    },

    RECUR_WEEKLY: 'weekly', RECUR_BIWEEKLY: 'biweekly', RECUR_MONTHLY: 'monthly',
    RECUR_CUSTOM: 'custom', RECUR_YEARLY: 'yearly',

    recurrenceWeeks: function (rtype, interval) {
      if (rtype === CalDate.RECUR_BIWEEKLY) return 2;
      if (rtype === CalDate.RECUR_CUSTOM && interval && interval > 0) return interval;
      return 1;
    },

    monthsBetween: function (cal, y1, m1, y2, m2) {
      var mc = CalDate.monthCount(cal);
      if (!mc) return 0;
      return (y2 - y1) * mc + (m2 - m1);
    },

    // The month read gives each repeating event its own dates in
    // `occurrences` (rules, skips and moves already applied by the server).
    // When they are there they are the whole truth for that month; the
    // arithmetic below is only the fallback for an event that came without.
    hasExpansion: function (e) {
      return Array.isArray(e.occurrences) || e.occurrences_truncated === true;
    },

    // The occurrence of e on (year, month, day), or null. A skipped one is
    // returned (flagged) only to viewers the server sent it to. When the
    // server could not finish counting (occurrences_truncated) and sent no
    // list at all, the event keeps its own start date. Once a list is given
    // (even empty) the start date is never put back: a player's list has
    // skipped dates stripped out, and a moved start date lives elsewhere.
    occurrenceOn: function (e, year, month, day) {
      var list = Array.isArray(e.occurrences) ? e.occurrences : [];
      for (var i = 0; i < list.length; i++) {
        var o = list[i];
        if (o.year === year && o.month === month && o.day === day) return o;
      }
      if (e.occurrences_truncated === true && !Array.isArray(e.occurrences) && e.year === year && e.month === month && e.day === day) {
        var moved = list.some(function (o) { return o.moved_from && o.moved_from.year === year && o.moved_from.month === month && o.moved_from.day === day; });
        if (!moved) return { year: year, month: month, day: day };
      }
      return null;
    },

    // occursOn mirrors Event.OccursOn exactly (model.go), including its
    // early-outs and its "recurrence only moves forward" rule, for an event
    // that arrived without occurrences.
    occursOn: function (cal, e, year, month, day) {
      if (CalDate.hasExpansion(e)) return CalDate.occurrenceOn(e, year, month, day) !== null;
      var onBase = e.year === year && e.month === month && e.day === day;
      if (!e.is_recurring || !e.recurrence_type) return onBase;
      var rt = e.recurrence_type;
      var known = [CalDate.RECUR_WEEKLY, CalDate.RECUR_BIWEEKLY, CalDate.RECUR_MONTHLY, CalDate.RECUR_CUSTOM, CalDate.RECUR_YEARLY];
      if (known.indexOf(rt) === -1) return onBase;

      var base = CalDate.dayIndex(cal, e.year, e.month, e.day);
      var target = CalDate.dayIndex(cal, year, month, day);
      if (target < base) return false;
      if (e.recurrence_end_year != null && e.recurrence_end_month != null && e.recurrence_end_day != null) {
        if (target > CalDate.dayIndex(cal, e.recurrence_end_year, e.recurrence_end_month, e.recurrence_end_day)) return false;
      }

      if (rt === CalDate.RECUR_YEARLY) {
        if (month !== e.month || day !== e.day || day > CalDate.monthDays(cal, month - 1, year)) return false;
        var stepY = (e.recurrence_interval && e.recurrence_interval > 1) ? e.recurrence_interval : 1;
        var nY = year - e.year;
        if (nY < 0 || nY % stepY !== 0) return false;
        if (e.recurrence_max_occurrences != null && Math.floor(nY / stepY) >= e.recurrence_max_occurrences) return false;
        return true;
      }
      if (rt === CalDate.RECUR_MONTHLY) {
        if (day !== e.day || day > CalDate.monthDays(cal, month - 1, year)) return false;
        var stepM = (e.recurrence_interval && e.recurrence_interval > 1) ? e.recurrence_interval : 1;
        var nM = CalDate.monthsBetween(cal, e.year, e.month, year, month);
        if (nM < 0 || nM % stepM !== 0) return false;
        if (e.recurrence_max_occurrences != null && Math.floor(nM / stepM) >= e.recurrence_max_occurrences) return false;
        return true;
      }
      var wl = CalDate.weekLen(cal);
      var stride = wl * CalDate.recurrenceWeeks(rt, e.recurrence_interval);
      if (stride <= 0) return onBase;
      var diff = target - base;
      if (diff % stride !== 0) return false;
      if (e.recurrence_max_occurrences != null && Math.floor(diff / stride) >= e.recurrence_max_occurrences) return false;
      return true;
    },

    // Whole-range containment for a (possibly non-recurring) multi-day
    // event: is (y,m,d) within [event start, event end] inclusive.
    withinSpan: function (cal, e, year, month, day) {
      if (e.end_year == null || e.end_month == null || e.end_day == null) return false;
      var t = CalDate.dayIndex(cal, year, month, day);
      var lo = CalDate.dayIndex(cal, e.year, e.month, e.day);
      var hi = CalDate.dayIndex(cal, e.end_year, e.end_month, e.end_day);
      return t >= lo && t <= hi;
    }
  };

  // -----------------------------------------------------------------
  // Moon phase math, ported from Moon.MoonPhase/MoonPhaseName/MoonPhaseIcon
  // (model.go) so the client's phase reads agree with the values the
  // server itself would compute for the same absolute day.
  // -----------------------------------------------------------------
  var MoonMath = {
    phase: function (moon, absoluteDay) {
      if (!moon.cycle_days || moon.cycle_days <= 0) return 0;
      var raw = (absoluteDay + (moon.phase_offset || 0)) / moon.cycle_days;
      var phase = raw - Math.floor(raw);
      return phase < 0 ? phase + 1 : phase;
    },
    // Mirrors Moon.MoonPhaseName verbatim: each name is centered on its
    // turning point, so the night before a full moon already reads "Full
    // Moon" (test/js/moon_phase_names.test.mjs keeps the copies in step).
    name: function (phase) {
      if (phase < 0.0625 || phase >= 0.9375) return 'New Moon';
      if (phase < 0.1875) return 'Waxing Crescent';
      if (phase < 0.3125) return 'First Quarter';
      if (phase < 0.4375) return 'Waxing Gibbous';
      if (phase < 0.5625) return 'Full Moon';
      if (phase < 0.6875) return 'Waning Gibbous';
      if (phase < 0.8125) return 'Last Quarter';
      return 'Waning Crescent';
    },
    litPct: function (phase) { return Math.round(((1 - Math.cos(2 * Math.PI * phase)) / 2) * 100); },
    // litPath mirrors MoonLitPath (moon_silhouette.go) exactly: the limb's
    // half-disc on the lit side, closed by the terminator's half-ellipse, for
    // a disc of radius r at the origin. "" within 0.004 of new.
    litPath: function (phase, r) {
      if (phase < 0.004 || phase > 0.996) return '';
      var num = function (f) { return f === Math.trunc(f) ? String(f) : f.toFixed(2); };
      var waxing = phase < 0.5, k = Math.cos(2 * Math.PI * phase), rx = Math.abs(k) * r;
      return 'M0,' + num(-r) + ' A' + num(r) + ',' + num(r) + ' 0 0 ' + (waxing ? 1 : 0) + ' 0,' + num(r) +
        ' A' + num(rx) + ',' + num(r) + ' 0 0 ' + (waxing === (k > 0) ? 0 : 1) + ' 0,' + num(-r) + ' Z';
    }
  };

  // -----------------------------------------------------------------
  // Small generic helpers.
  // -----------------------------------------------------------------
  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (ch) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch];
    });
  }
  function dayKey(y, m, d) { return y + '_' + m + '_' + d; }
  // The viewer's own date today, as YYYY-MM-DD.
  function localIso(t) { return t.getFullYear() + '-' + pad2(t.getMonth() + 1) + '-' + pad2(t.getDate()); }

  // A week of hours to paint: [col 0..6, Monday first][hour 0..23].
  function emptyWeekGrid() {
    var g = [];
    for (var c = 0; c < 7; c++) { g.push([]); for (var h = 0; h < 24; h++) g[c].push(''); }
    return g;
  }
  function copyWeekGrid(g) { return g.map(function (col) { return col.slice(); }); }
  function weekGridEmpty(g) { return g.every(function (col) { return col.every(function (v) { return !v; }); }); }

  // A painted week as the availability API's blocks: one block per run of
  // same-state hours, dayOfWeek 0 = Sunday.
  function weekGridBlocks(g, cadence) {
    var out = [];
    for (var c = 0; c < 7; c++) {
      var run = null;
      for (var h = 0; h <= 24; h++) {
        var st = h < 24 ? g[c][h] : '';
        if (run && run.st === st) continue;
        if (run) out.push({ dayOfWeek: (c + 1) % 7, startMinute: run.h * 60, endMinute: h * 60, state: run.st, weekCadence: cadence });
        run = st ? { st: st, h: h } : null;
      }
    }
    return out;
  }

  // The Sundays that name the two alternating weeks, from the week holding
  // iso: { 1: week A's, 2: week B's }, whichever comes first. Weeks are
  // counted from Sunday 1970-01-04, as the server's WeekCadenceFor counts
  // them, so both sides agree which week is which.
  function cadenceWeeks(iso) {
    var p = iso.split('-'), d = new Date(Date.UTC(+p[0], +p[1] - 1, +p[2]));
    d.setUTCDate(d.getUTCDate() - d.getUTCDay());
    var days = Math.round((d.getTime() - Date.UTC(1970, 0, 4)) / 86400000);
    var week = Math.floor(days / 7), first = d.toISOString().slice(0, 10);
    d.setUTCDate(d.getUTCDate() + 7);
    var next = d.toISOString().slice(0, 10);
    return ((week % 2) + 2) % 2 === 0 ? { 1: first, 2: next } : { 1: next, 2: first };
  }
  function parseDayKey(k) { var p = k.split('_'); return { y: +p[0], m: +p[1], d: +p[2] }; }
  function pad2(n) { return (n < 10 ? '0' : '') + n; }
  // realAnchor ties a world calendar to real dates by its own today: the
  // world's current date is the real today, and every other day's real
  // date follows by day count (AbsoluteDay). Game nights are real-life
  // dates, so tonight's night belongs on the world's today wherever the
  // story's clock has been moved to; a fixed link set once drifted off as
  // soon as the world's date moved faster or slower than the real one.
  // A real-time calendar needs none and returns null. now (ms) is for
  // tests; the real today is the viewer's own.
  function realAnchor(cal, now) {
    if (CalDate.usesRealTime(cal)) return null;
    if (!cal.current_year || !cal.current_month || !cal.current_day) return null;
    var t = new Date(now == null ? Date.now() : now);
    return { abs: CalDate.absoluteDay(cal, cal.current_year, cal.current_month, cal.current_day), ms: Date.UTC(t.getFullYear(), t.getMonth(), t.getDate()) };
  }

  // Game-night times: a night is stored as a wall time in the zone it was
  // set in; these turn it into the same moment in another zone with Intl,
  // which knows each zone's daylight-saving rules.
  function zoneParts(zone, ms) {
    var o = {};
    new Intl.DateTimeFormat('en-US', { timeZone: zone, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', timeZoneName: 'short' })
      .formatToParts(new Date(ms)).forEach(function (p) { o[p.type] = p.value; });
    return o;
  }
  function zoneOffsetMin(zone, ms) {
    var p = zoneParts(zone, ms);
    return Math.round((Date.UTC(+p.year, +p.month - 1, +p.day, +p.hour % 24, +p.minute) - Math.floor(ms / 60000) * 60000) / 60000);
  }
  // zonedToMs: the instant a wall time in zone names. The second pass
  // settles a guess that landed across a daylight-saving change.
  function zonedToMs(dateIso, hm, zone) {
    var d = dateIso.split('-'), t = hm.split(':');
    var guess = Date.UTC(+d[0], +d[1] - 1, +d[2], +t[0], +t[1]);
    var off = zoneOffsetMin(zone, guess), off2 = zoneOffsetMin(zone, guess - off * 60000);
    return guess - off2 * 60000;
  }
  // zonedShow: a night set at dateIso hm in src, as seen in ref:
  // {hm, abbr, date}, or null when a zone is unknown to this browser.
  function zonedShow(dateIso, hm, src, ref) {
    try {
      var p = zoneParts(ref, zonedToMs(dateIso, hm, src));
      return { hm: pad2(+p.hour % 24) + ':' + p.minute, abbr: p.timeZoneName || '', date: p.year + '-' + p.month + '-' + p.day };
    } catch (e) { return null; }
  }
  function browserZone() {
    try { return Intl.DateTimeFormat().resolvedOptions().timeZone || ''; } catch (e) { return ''; }
  }
  // "Thu Oct 8" for a YYYY-MM-DD real date.
  function realDateWords(iso) {
    try {
      return new Date(iso + 'T12:00:00Z').toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric', timeZone: 'UTC' }).replace(',', '');
    } catch (e) { return iso; }
  }
  // "6pm", "6:30pm", "12am": availability reads in the clock people say
  // out loud. m is minutes from midnight; 1440 is the next midnight.
  function ampm(m) {
    var h = Math.floor(m / 60) % 24, mm = m % 60, suf = h < 12 ? 'am' : 'pm', h12 = h % 12 || 12;
    return h12 + (mm ? ':' + pad2(mm) : '') + suf;
  }
  // "Oct 17", "Oct 17 and Oct 31", "Oct 3, Oct 17 and Oct 31".
  function listWords(a) {
    return a.length < 2 ? (a[0] || '') : a.slice(0, -1).join(', ') + ' and ' + a[a.length - 1];
  }
  // A player's whole days off from today on, as stretches of consecutive
  // dates: [{from, to}]. A date holding anything but one whole day off is a
  // day shaped by hand, not part of an away stretch.
  function awayStretches(excs, today) {
    var byDate = {};
    excs.forEach(function (e) { (byDate[e.onDate] = byDate[e.onDate] || []).push(e); });
    var days = Object.keys(byDate).filter(function (d) {
      var l = byDate[d];
      return d >= today && l.length === 1 && l[0].startMinute === 0 && l[0].endMinute === 1440 && l[0].state === 'unavailable';
    }).sort();
    var out = [];
    days.forEach(function (d) {
      var last = out[out.length - 1];
      if (last && isoAddDays(last.to, 1) === d) last.to = d;
      else out.push({ from: d, to: d });
    });
    return out;
  }
  // "19:30" as "7:30pm", the clock the rest of the calendar reads in.
  function hmWords(hm) {
    var p = /^(\d{1,2}):(\d{2})/.exec(String(hm || ''));
    return p ? ampm(+p[1] * 60 + +p[2]) : String(hm || '');
  }
  // Real-date arithmetic on YYYY-MM-DD strings, in UTC so no zone shifts
  // a day.
  function isoAddDays(iso, n) {
    var p = iso.split('-');
    return new Date(Date.UTC(+p[0], +p[1] - 1, +p[2] + n)).toISOString().slice(0, 10);
  }
  function isoMonday(iso) {
    var p = iso.split('-'), dow = new Date(Date.UTC(+p[0], +p[1] - 1, +p[2])).getUTCDay();
    return isoAddDays(iso, -((dow + 6) % 7));
  }
  // The short name of a zone on a date, like "CDT"; '' when unknown.
  function zoneAbbr(zone, iso) {
    if (!zone) return '';
    try { return zoneParts(zone, Date.parse(iso + 'T12:00:00Z')).timeZoneName || ''; } catch (e) { return ''; }
  }
  var FREE_KEY = 'chronicle.calendar.whosFree';
  // What the colours in a line mean, under every set of named lines.
  var FREE_LEGEND = '<div class="flegend" aria-hidden="true"><span><i class="f"></i>Free</span><span><i class="b"></i>Best time</span><span><i class="e"></i>Everyone free</span></div>';
  // The key under one day's lines, with "Off, usually free" when it is used.
  function freeLegend(members) {
    return members.some(function (mem) { return mem.off; }) ? FREE_LEGEND.replace('</div>', '<span><i class="o"></i>Off, usually free</span></div>') : FREE_LEGEND;
  }

  // Which zone a viewer reads game-night times in: 'mine' or the
  // calendar's. The browser remembers it; nothing breaks without storage.
  var GN_ZONE_KEY = 'chronicle.calendar.nightZone';
  function readZoneMode() {
    try { return window.localStorage.getItem(GN_ZONE_KEY) === 'mine' ? 'mine' : 'cal'; } catch (e) { return 'cal'; }
  }
  function writeZoneMode(mode) {
    try { window.localStorage.setItem(GN_ZONE_KEY, mode); } catch (e) { /* remembered for this page only */ }
  }
  function reducedMotion() { return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches; }

  // Event category icon: an event's own icon (FontAwesome-style token from
  // the mockup's monochrome symbol set) falls back to its kind's icon, then
  // a generic dot. Chronicle's EventKind.Icon is authored as a short glyph
  // (emoji or FA class per DefaultEventKinds) — render it as text, which
  // degrades gracefully for both.
  function eventGlyph(e) {
    return e.icon || e.kind_icon || '●';
  }

  // Allowlist for a color value read from server data (event/kind colors)
  // before it lands in a style="color:..." attribute: strips everything
  // but the characters a hex/rgb/oklch/named CSS color can legitimately
  // contain, so a crafted value can't close the attribute early or inject a
  // second declaration. Exposed on Chronicle so calendar_editor.js's kind
  // chips and era-manager rendering use this exact same allowlist rather
  // than a second copy that could drift from it — the same cross-file
  // sharing CalDate/growOpen/growClose already use below.
  function sanitizeColor(c) {
    return c ? String(c).replace(/[^#a-zA-Z0-9(),.% ]/g, '') : '';
  }

  // A weather reading's icon is one of the generator's six glyphs; anything
  // else (an older or imported reading) gets a neutral sun-and-cloud.
  var WEATHER_ICONS = { clear: 'fa-sun', cloud: 'fa-cloud', rain: 'fa-cloud-rain', snow: 'fa-snowflake', storm: 'fa-cloud-bolt', fog: 'fa-smog' };
  function weatherIcon(w) { return WEATHER_ICONS[w && w.icon] || 'fa-cloud-sun'; }
  function weatherLabel(w) { return w.preset_label || w.description || 'Weather recorded'; }
  // Colour is per reading, so supernatural weather (acid rain, an owner's
  // own fire rain) reads apart from plain rain with the same glyph.
  function weatherStyle(w) { var c = sanitizeColor(w.color); return c ? ' style="color:' + c + '"' : ''; }

  // The corner mark on a day cell. Future days only ever reach a Director
  // (the server filters them for everyone else), drawn faint.
  function weatherMarkHTML(w, future) {
    if (!w) return '';
    return '<span class="wxm' + (future ? ' fut' : '') + '" title="' + esc(weatherLabel(w)) + '"' + weatherStyle(w) + '>' +
      '<i class="fa-solid ' + weatherIcon(w) + '" aria-hidden="true"></i></span>';
  }

  // Edit mode's view of a day's stored reading (CSS shows it only while
  // .cal.wxon): the glyph, and a tint of the weather's own colour on a day
  // painted by hand. Generated readings show the glyph alone, so the
  // Director can tell what they set from what the generator set. ghost is a
  // Generate preview: faint, and not yet saved.
  function weatherPaintHTML(w, future, ghost) {
    if (!w) return '';
    var hand = !ghost && w.source !== 'generated', c = sanitizeColor(w.color), tone = hand ? ' hand' : (ghost ? ' ghost' : '');
    var locked = !ghost && w.locked === true;
    return ((hand || ghost) && c ? '<span class="cpt' + tone + '" style="--wxc:color-mix(in oklch,' + c + ' 26%,transparent)"></span>' : '') +
      '<span class="cwx' + tone + (future ? ' dir' : '') + (locked ? ' lkd' : '') + '"' + (c ? ' style="color:' + c + '"' : '') + (locked ? '' : ' aria-hidden="true"') + '>' +
      '<i class="fa-solid ' + weatherIcon(w) + ' i"' + (locked ? ' aria-hidden="true"' : '') + '></i>' +
      (locked ? '<i class="fa-solid fa-lock lk" aria-hidden="true"></i><span class="fcsr">locked</span>' : '') + '</span>';
  }

  // ---- Player forecast ----
  // A forecast entry's words (the server's own wording), or a neutral fallback.
  function forecastWords(e) { return String((e && e.words) || 'Forecast'); }

  // The grid/band mark for a forecast day: dashed outline always, faded when
  // the forecast is unsure. The words are visible; the sr text says it is a
  // forecast, not a reading.
  function forecastMarkHTML(e) {
    if (!e) return '';
    var words = forecastWords(e), faded = typeof e.confidence === 'number' && e.confidence < 0.55;
    return '<span class="wxm fc' + (faded ? ' faded' : '') + '" title="' + esc('Forecast: ' + words) + '">' +
      '<i class="fa-solid ' + weatherIcon({ icon: e.icon }) + '" aria-hidden="true"></i>' +
      '<span class="fcw" aria-hidden="true">' + esc(words) + '</span>' +
      '<span class="fcsr">' + esc('Forecast: ' + words) + '</span></span>';
  }

  // "8°C to 14°C · 70% chance of rain"; either part is left out when unknown.
  function forecastDetailText(e) {
    if (!e) return '';
    var parts = [], lo = e.temp_low, hi = e.temp_high;
    if (lo != null && hi != null) parts.push(lo + '°C to ' + hi + '°C');
    else if (lo != null || hi != null) parts.push((lo != null ? lo : hi) + '°C');
    var pc = Number(e.precip_chance);
    if (pc > 0) {
      var noun = e.icon === 'snow' ? 'snow' : (e.icon === 'storm' ? 'storms' : 'rain');
      parts.push(Math.round(pc) + '% chance of ' + noun);
    }
    return parts.join(' · ');
  }

  // The day card's forecast block for a player (never a real reading).
  function forecastFactHTML(e) {
    if (!e) return '';
    var detail = forecastDetailText(e);
    return '<div class="row wxf fcf"><i class="fa-solid ' + weatherIcon({ icon: e.icon }) + '" aria-hidden="true"></i><span class="tt"><b>' + esc(forecastWords(e)) + '</b>' +
      (detail ? '<small>' + esc(detail) + '</small>' : '') +
      '<small>A forecast, not a promise. The real weather shows on the day.</small></span></div>';
  }

  // The Director's one-line note of what players are told for the day.
  function playersSeeHTML(e) {
    return e ? '<div class="row wxf fcps"><span class="tt"><small>' + esc('Players see: ' + forecastWords(e)) + '</small></span></div>' : '';
  }

  var WIND_DIRS = { N: 'north', NNE: 'north', NE: 'northeast', ENE: 'east', E: 'east', ESE: 'east', SE: 'southeast', SSE: 'south',
    S: 'south', SSW: 'south', SW: 'southwest', WSW: 'west', W: 'west', WNW: 'west', NW: 'northwest', NNW: 'north' };
  function windWords(wind) {
    if (!wind || !wind.speed_tier) return '';
    var tier = String(wind.speed_tier);
    if (tier === 'calm') return 'Calm';
    var dir = WIND_DIRS[wind.direction];
    return tier.charAt(0).toUpperCase() + tier.slice(1) + ' wind' + (dir ? ' from the ' + dir : '');
  }

  // The day card's weather line. A future day is ghosted with a note that
  // players see it on the day.
  function weatherFactHTML(w, future) {
    if (!w) return '';
    var sub = windWords(w.wind);
    return '<div class="row wxf' + (future ? ' fut' : '') + '"><i class="fa-solid ' + weatherIcon(w) + '"' + weatherStyle(w) + '></i><span class="tt">' +
      esc(weatherLabel(w)) + (w.temperature_celsius != null ? ' · ' + esc(String(w.temperature_celsius)) + '°C' : '') +
      (sub ? '<small>' + esc(sub) + '</small>' : '') +
      (future ? '<span class="wxtag">Players see this on the day</span>' : '') +
      '</span></div>';
  }

  // Event category color: an event's own color falls back to its kind's.
  // Rendered as a plain inline `color:` declaration (not the mockup's --h
  // oklch hue variable, which would need a hex->oklch-hue conversion this
  // port doesn't do) — an inline style declaration already wins over the
  // stylesheet's `.mk{color:oklch(var(--icL) var(--icC) var(--h,260))}`
  // regardless, so this is a complete override, not a partial one.
  function eventColorStyle(e) {
    var c = sanitizeColor(e.color || e.kind_color);
    return c ? ('color:' + c + ';') : '';
  }

  // ---------------------------------------------------------------
  // Generic "grow from anchor" panel open/close, shared by every fold-out
  // (day wing, era manager, event detail, moon view, hub popover). The panel
  // is laid out at full size first (server/JS builds its final DOM before
  // any animation starts, scrollbars included per the motion rules), and
  // only transform + opacity animate — never width/height, so text never
  // reflows mid-animation. Reduced motion drops straight to a plain
  // crossfade. Every panel restores focus to whatever opened it on close.
  // ---------------------------------------------------------------
  // positionPanel sets a panel's left/top (and, in 'cover' mode, its size)
  // relative to containerEl before growOpen measures/animates it — the
  // panel is laid out at its FINAL position and size first (motion rule:
  // never animate a moving clip or a reflow), so only transform/opacity
  // animate afterward.
  function positionPanel(panel, anchorEl, containerEl, opts) {
    opts = opts || {};
    panel.style.left = '0px';
    panel.style.top = '0px';
    panel.style.width = '';
    panel.style.minHeight = '';
    var cr = containerEl.getBoundingClientRect();
    var ar = (anchorEl || containerEl).getBoundingClientRect();
    if (opts.cover) {
      panel.style.width = ar.width + 'px';
      panel.style.minHeight = ar.height + 'px';
      panel.style.left = (ar.left - cr.left) + 'px';
      panel.style.top = (ar.top - cr.top) + 'px';
      return;
    }
    var x, y;
    if (opts.center) {
      var w = Math.min(700, cr.width - 16);
      panel.style.width = w + 'px';
      var ph2 = panel.offsetHeight;
      x = Math.max(8, (cr.width - w) / 2);
      y = Math.max(8, (cr.height - ph2) / 2);
    } else {
      var pw = panel.offsetWidth, ph = panel.offsetHeight;
      x = Math.min(Math.max(8, ar.left - cr.left), Math.max(8, cr.width - pw - 8));
      y = ar.bottom - cr.top + 8;
      if (y + ph > cr.height - 8) y = Math.max(8, ar.top - cr.top - ph - 8);
    }
    panel.style.left = x + 'px';
    panel.style.top = y + 'px';
  }

  function growOpen(panel, anchorEl, containerEl, opts) {
    positionPanel(panel, anchorEl, containerEl, opts);
    panel.classList.add('live');
    // Force layout so the browser has committed the panel's final box
    // before we read its rect / start animating from it.
    void panel.offsetWidth;
    if (anchorEl) {
      var pr = panel.getBoundingClientRect(), ar = anchorEl.getBoundingClientRect();
      var ox = ar.left + ar.width / 2 - pr.left, oy = ar.top + ar.height / 2 - pr.top;
      panel.style.transformOrigin = ox + 'px ' + oy + 'px';
    }
    panel.classList.add('open');
    if (reducedMotion()) {
      var fa = panel.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 150, easing: 'ease-out', fill: 'both' });
      return fa.finished.catch(function () {});
    }
    var anim = panel.animate(
      [{ opacity: 0, transform: 'scale(.82)' }, { opacity: 1, transform: 'none' }],
      { duration: 220, easing: 'cubic-bezier(.22,.8,.24,1)', fill: 'both' }
    );
    return anim.finished.catch(function () {});
  }

  function growClose(panel, opener) {
    var had = panel.contains(document.activeElement);
    var frames = reducedMotion()
      ? [{ opacity: 1 }, { opacity: 0 }]
      : [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.88)' }];
    var anim = panel.animate(frames, { duration: reducedMotion() ? 120 : 180, easing: 'cubic-bezier(.22,.8,.24,1)', fill: 'both' });
    return anim.finished.catch(function () {}).then(function () {
      panel.classList.remove('open', 'live');
      panel.removeAttribute('style');
      panel.innerHTML = '';
      if (had && opener) opener.focus({ preventScroll: true });
    });
  }

  // ---------------------------------------------------------------
  // The day's card and the era's card (the signed calendar mockup,
  // #829). On a desktop each is two leaves of paper, laid out at its
  // final size and place: the leaf nearer what was pressed turns out
  // from behind its edge, the other unfolds from behind that one, and
  // closing runs the film backwards. What was pressed gives a little
  // as the card leaves it. A phone's card is a sheet that grows whole
  // out of the tapped day; reduced motion cross-fades. Only transform
  // and opacity move, and a leaf only shows while its face is towards
  // us, so every frame shows each leaf whole or not at all.
  // ---------------------------------------------------------------
  var FOLD = { d: 1100, lead: [30, 330], trail: [100, 390], rest: 490, fade: 80, cut: 76, shut: 450, back: 0.86, pressBack: 250, shade: 0.3, leafMin: 170, leafRatio: 1.5, letDown: 300, takeUp: 180, eye: 2.4 };
  // How long the held part of a long card takes to let down (or take up):
  // the base time for an ordinary card, more for a longer drop, so the
  // paper moves at one speed whatever the card's length.
  function leafTime(h, base) { return Math.round(clampN(base * (0.6 + (h.full - h.cap) / 500), base * 0.75, base * 1.6)); }
  var SPRING = 'cubic-bezier(.34,1.3,.4,1)';
  var EASE = 'cubic-bezier(.22,.8,.24,1)';
  function isPhone() { return window.matchMedia('(max-width:600px)').matches; }
  function anim(el, frames, o) { return el.animate(frames, Object.assign({ fill: 'forwards' }, o)); }
  function stopAnims(el, deep) { (deep ? el.getAnimations({ subtree: true }) : el.getAnimations()).forEach(function (a) { a.cancel(); }); }
  function settleAll(list) { return Promise.all(list.map(function (a) { return a.finished; })); }
  function clampN(v, a, b) { return Math.max(a, Math.min(b, v)); }
  function relRect(el, box) { var b = box.getBoundingClientRect(), r = el.getBoundingClientRect(); return { x: r.left - b.left, y: r.top - b.top, w: r.width, h: r.height }; }

  // The part of the calendar a card may use: what is on screen, inside
  // whatever scrolls it.
  // vbot is the visible edge alone, past the calendar's own bottom.
  function visBand(calEl) {
    var cr = calEl.getBoundingClientRect(), top = 0, bot = window.innerHeight;
    for (var p = calEl.parentElement; p && p !== document.body; p = p.parentElement) {
      var oy = getComputedStyle(p).overflowY;
      if (oy === 'auto' || oy === 'scroll') { var pr = p.getBoundingClientRect(); top = Math.max(top, pr.top); bot = Math.min(bot, pr.bottom); }
    }
    return { cr: cr, top: Math.max(8, top - cr.top + 8), bot: Math.min(cr.height - 8, bot - cr.top - 8), vbot: bot - cr.top - 8 };
  }
  // A day's card goes beside its day where there is room (right, else
  // left), else under or over it, and never past the visible band.
  function wingGeometry(card, btn, calEl) {
    var B = visBand(calEl), cr = B.cr, cell = relRect(btn, calEl), isBand = btn.classList.contains('band');
    if (isBand) { cell.x += 8; cell.w = Math.min(220, cell.w - 16); }
    var W = Math.min(340, cr.width - 24), maxH = Math.max(160, Math.min(540, B.bot - B.top));
    card.style.width = W + 'px';
    var H = Math.min(maxH, card.offsetHeight);
    card.style.width = '';
    var L = cell.x, R = L + cell.w, T = cell.y, Bt = T + cell.h, side = null, x, y;
    if (!isBand && R + W <= cr.width - 8) { side = 'right'; x = R; }
    else if (!isBand && L - W >= 8) { side = 'left'; x = L - W; }
    if (side) y = T + H <= B.bot ? T : (Bt - H >= B.top ? Bt - H : Math.max(B.top, B.bot - H));
    else {
      x = Math.max(8, Math.min(L + cell.w / 2 - W / 2, cr.width - W - 8));
      if (Bt + H <= B.bot) { side = 'down'; y = Bt; } else if (T - H >= B.top) { side = 'up'; y = T - H; } else { side = 'down'; y = Math.max(B.top, B.bot - H); }
    }
    return { cell: cell, side: side, final: { x: x, y: y, w: W, h: H, room: Math.max(H, B.vbot - y) } };
  }
  // The era's card drops out of the era label's bottom edge.
  function eraGeometry(card, btn, calEl) {
    var B = visBand(calEl), cr = B.cr, cell = relRect(btn, calEl), W = Math.min(392, cr.width - 16), y = cell.y + cell.h;
    var maxH = Math.max(160, Math.min(560, B.bot - y));
    card.style.width = W + 'px';
    var H = Math.min(maxH, card.offsetHeight);
    card.style.width = '';
    return { cell: cell, side: 'down', final: { x: Math.max(8, Math.min(cell.x, cr.width - W - 8)), y: y, w: W, h: H, room: Math.max(H, B.vbot - y) } };
  }
  // A card opens at its own height, inside the calendar; a form opened
  // in it later may grow it into the room below, to the visible edge.
  function placeCard(el, g) {
    el.dataset.side = g.side;
    el.style.left = g.final.x + 'px';
    el.style.top = g.final.y + 'px';
    el.style.width = g.final.w + 'px';
    el.style.maxHeight = g.final.room + 'px';
  }

  // Which leaf leads, on which hinge, and where the eye is: a card beside
  // its day turns out on the day's edge, a card under or over what was
  // pressed drops or rises from the edge nearer it. The eye is shared by
  // both leaves, so the crease between them stays joined. A card with no
  // second leaf to speak of folds out as one.
  function foldGeo(P, calEl) {
    var el = P.el, g = P.g, L1 = el.querySelector('.lf1'), L2 = el.querySelector('.lf2'), bx = relRect(el, calEl);
    if (L2 && !L2.offsetHeight) L2 = null;
    var W = el.offsetWidth, H = el.offsetHeight, h1 = L1.offsetHeight, cy = g.cell.y + g.cell.h / 2 - bx.y;
    var side = g.side, flat = side === 'right' || side === 'left', head = !L2 || (flat ? cy < h1 : side !== 'up' && cy <= H / 2);
    // The eye stands back in proportion to the span that turns, so a tall
    // card leans as far as a short one instead of ballooning towards us.
    var F = { el: el, d: Math.max(FOLD.d, Math.round(FOLD.eye * (flat ? W : H))), V: { x: W / 2, y: H / 2 }, ax: flat ? 'rotateY' : 'rotateX', B: { x: 0, y: h1 }, head: head, lead: head ? L1 : L2, trail: head ? L2 : L1, s2: head ? -1 : 1 };
    if (flat) { F.A = { x: side === 'left' ? W : 0, y: 0 }; F.s1 = side === 'right' ? 1 : -1; }
    else { F.A = { x: 0, y: head ? 0 : H }; F.s1 = head ? -1 : 1; }
    F.O = { lead: { x: 0, y: head ? 0 : h1 }, trail: { x: 0, y: head ? h1 : 0 } };
    return F;
  }
  // A card beside its day turns on a vertical hinge, and a tall second
  // leaf swinging through a turn reads as a stretched sheet rather than
  // paper. So a long card folds with its second leaf held to about the
  // head's proportions, then lets the rest down once it lies flat, and
  // takes it back up before folding away. Returns the leaf and its
  // heights, or null when the card is short enough to fold whole.
  function foldShort(P) {
    var g = P.g, L1 = P.el.querySelector('.lf1'), L2 = P.el.querySelector('.lf2');
    if (!g || (g.side !== 'right' && g.side !== 'left') || !L1 || !L2) return null;
    var cap = Math.round(Math.max(FOLD.leafMin, L1.offsetHeight * FOLD.leafRatio)), full = L2.offsetHeight;
    return full > cap + 24 ? { L2: L2, cap: cap, full: full } : null;
  }
  function holdLeaf(h, px) { h.L2.style.maxHeight = px + 'px'; h.L2.style.overflow = 'hidden'; }
  // Holding the leaf shortens the card from its foot, so a card that
  // hangs up from its day (its foot level with the day's) would fold out
  // well above the day and only reach it as the rest lets down. Holds
  // the leaf and returns how far the held card must sit lower to keep
  // its foot by the day; the card rises back as the leaf lets down.
  function heldLift(P, h) {
    var el = P.el, g = P.g, h0 = el.offsetHeight;
    holdLeaf(h, h.cap);
    var h1 = el.offsetHeight;
    return clampN(g.cell.y + g.cell.h - (g.final.y + h1), 0, h0 - h1);
  }
  function liftCard(P, from, to, dur) {
    P.el.style.top = (P.g.final.y + to) + 'px';
    return anim(P.el, [{ top: (P.g.final.y + from) + 'px' }, { top: (P.g.final.y + to) + 'px' }], { duration: dur, easing: EASE, fill: 'none' }).finished;
  }
  function freeLeaf(h) { h.L2.style.maxHeight = ''; h.L2.style.overflow = ''; }
  function slideLeaf(h, from, to, dur) {
    return anim(h.L2, [{ maxHeight: from + 'px' }, { maxHeight: to + 'px' }], { duration: dur, easing: EASE, fill: 'none' }).finished;
  }
  function foldEase(u) { u = clampN(u, 0, 1); return 1 - Math.pow(1 - u, 3); }
  function foldRot(ax, a, v) {
    var c = Math.cos(a), s = Math.sin(a);
    return ax === 'rotateY' ? [v[0] * c + v[2] * s, v[1], v[2] * c - v[0] * s] : [v[0], v[1] * c - v[2] * s, v[1] * s + v[2] * c];
  }
  // One moment of the open, t ms in: both leaves' angles (180 is folded
  // flat behind, 0 is open); whether each faces the eye (the second only
  // once the first does); how dark each is by how far it is turned from
  // us; and whose shadow shows: the leaves' own while they fold, the
  // card's once flat.
  function foldAt(F, t) {
    var r = Math.PI / 180, a1 = F.s1 * 180 * (1 - foldEase((t - FOLD.lead[0]) / FOLD.lead[1])), a2 = F.s2 * 180 * (1 - foldEase((t - FOLD.trail[0]) / FOLD.trail[1]));
    var A = F.A, B = F.B, V = F.V, n1 = foldRot(F.ax, a1 * r, [0, 0, 1]), n2 = foldRot(F.ax, a1 * r, foldRot('rotateX', a2 * r, [0, 0, 1])), q = foldRot(F.ax, a1 * r, [B.x - A.x, B.y - A.y, 0]);
    var f1 = n1[0] * (V.x - A.x) + n1[1] * (V.y - A.y) + n1[2] * F.d, f2 = n2[0] * (V.x - A.x - q[0]) + n2[1] * (V.y - A.y - q[1]) + n2[2] * (F.d - q[2]);
    var k = clampN((t - FOLD.rest) / FOLD.fade, 0, 1);
    k = k * k * (3 - 2 * k);
    return { a1: a1, a2: a2, v1: f1 > 0, v2: f1 > 0 && f2 > 0, d1: FOLD.shade * Math.pow(clampN(1 - n1[2], 0, 1), 0.8), d2: FOLD.shade * Math.pow(clampN(1 - n2[2], 0, 1), 0.8), leaf: 1 - k, card: k };
  }
  function pxs(x, y) { return x.toFixed(2) + 'px,' + y.toFixed(2) + 'px'; }
  // A leaf's transform about its own top-left corner: the eye's
  // perspective, the first leaf's hinge, and for the second leaf also the
  // crease, which turns with the first.
  function foldTf(F, which, s) {
    var O = F.O[which], V = F.V, A = F.A, B = F.B;
    var t = 'translate(' + pxs(V.x - O.x, V.y - O.y) + ') perspective(' + F.d + 'px) translate(' + pxs(A.x - V.x, A.y - V.y) + ') ' + F.ax + '(' + s.a1.toFixed(3) + 'deg) ';
    if (which === 'lead') return t + 'translate(' + pxs(O.x - A.x, O.y - A.y) + ')';
    return t + 'translate(' + pxs(B.x - A.x, B.y - A.y) + ') rotateX(' + s.a2.toFixed(3) + 'deg) translate(' + pxs(O.x - B.x, O.y - B.y) + ')';
  }
  // The film as keyframes, sampled from foldAt, with a sample at every
  // moment a leaf passes edge-on: a leaf's opacity steps between 0 and 1
  // only there, when it is a line. Backwards the film runs from where the
  // leaves have all but settled to where both are hidden again, a little
  // quicker, and the card's one shadow parts into the leaves' own at once.
  function foldRun(F, back, sp) {
    var T = back ? FOLD.shut : FOLD.rest + FOLD.fade, t0 = back ? FOLD.cut : 0, ts = [], flips = [], prev = foldAt(F, t0), i, t;
    for (i = 0; i <= 40; i++) ts.push(t0 + (T - t0) * i / 40);
    for (t = t0 + 0.5; t <= T; t += 0.5) { var st = foldAt(F, t); if (st.v1 !== prev.v1 || st.v2 !== prev.v2) { flips.push(t); ts.push(t); } prev = st; }
    ts.sort(function (a, b) { return a - b; });
    var off = function (tt) { var o = (tt - t0) / (T - t0); return back ? 1 - o : o; };
    var fr = { lead: [], trail: [], d1: [], d2: [], leaf: [], card: [], o1: [], o2: [] };
    ts.forEach(function (tt) {
      var s = foldAt(F, tt), o = off(tt);
      if (back) { var k = clampN(o / 0.12, 0, 1); k = k * k * (3 - 2 * k); s.leaf = k; s.card = 1 - k; }
      fr.lead.push({ offset: o, transform: foldTf(F, 'lead', s) });
      fr.trail.push({ offset: o, transform: foldTf(F, 'trail', s) });
      fr.d1.push({ offset: o, opacity: s.d1 });
      fr.d2.push({ offset: o, opacity: s.d2 });
      fr.leaf.push({ offset: o, opacity: s.leaf });
      fr.card.push({ offset: o, opacity: s.card });
    });
    [['o1', 'v1'], ['o2', 'v2']].forEach(function (q) {
      var at = function (tt) { return foldAt(F, tt)[q[1]] ? 1 : 0; }, list = fr[q[0]];
      list.push({ offset: off(t0), opacity: at(t0) });
      flips.forEach(function (tt) { var a = at(tt - 0.5), b = at(tt); if (a !== b) { list.push({ offset: off(tt), opacity: a }); list.push({ offset: off(tt), opacity: b }); } });
      list.push({ offset: off(T), opacity: at(T) });
    });
    if (back) Object.keys(fr).forEach(function (k) { fr[k].reverse(); });
    var o = { duration: (T - t0) * (back ? FOLD.back : 1) / sp, fill: 'both', easing: 'linear' };
    var pe = function (p) { return Object.assign({ pseudoElement: p }, o); };
    var list = [anim(F.lead, fr.lead, o), anim(F.lead, fr.o1, o), anim(F.lead, fr.d1, pe('::after')), anim(F.lead, fr.leaf, pe('::before')), anim(F.el, fr.card, pe('::before'))];
    if (F.trail) list.push(anim(F.trail, fr.trail, o), anim(F.trail, fr.o2, o), anim(F.trail, fr.d2, pe('::after')), anim(F.trail, fr.leaf, pe('::before')));
    return list;
  }
  // What was pressed sinks a little and springs back. Wide things sink
  // less, so every press moves about as far.
  function pressIn(list, o) {
    o = o || {};
    (list || []).forEach(function (t) {
      if (!t || !t.el || !t.el.isConnected) return;
      var d = t.depth || 0.94, opt = { duration: o.dur || 420, delay: o.delay || 0, fill: 'none' };
      if (o.soft) d = 1 - (1 - d) * 0.5;
      if (t.pseudo) opt.pseudoElement = t.pseudo;
      t.el.animate([{ transform: 'scale(1)', easing: 'cubic-bezier(.3,0,.6,1)' }, { transform: 'scale(' + d.toFixed(3) + ')', offset: 0.22, easing: SPRING }, { transform: 'scale(1)' }], opt);
    });
  }
  function pressDepth(el) { return 1 - Math.min(0.06, 6 / Math.max(1, el.getBoundingClientRect().width)); }
  function originOn(el, src) {
    var r = el.getBoundingClientRect(), s = src.getBoundingClientRect();
    el.style.transformOrigin = (s.left + s.width / 2 - r.left).toFixed(1) + 'px ' + (s.top + s.height / 2 - r.top).toFixed(1) + 'px';
  }
  // A day's marks travel between its cell and its card: a mark leaves
  // the cell as it sets off and the card's icon appears as it lands, so
  // an event is only ever in one place. Only the rows the card shows
  // receive their mark; the rest fade out of the cell and back.
  function inCardView(el, card) {
    var r = el.getBoundingClientRect(), sc = (!isPhone() && el.closest('.lscroll')) || card, v = sc.getBoundingClientRect();
    return r.top >= v.top && r.bottom <= v.bottom - 4;
  }
  function carryPairs(cell, card) {
    var out = [];
    $$('.mk[data-ev]', cell).forEach(function (m) {
      var to = card.querySelector('.evd[data-ev="' + m.dataset.ev + '"] .ric');
      if (to && m.getBoundingClientRect().width && inCardView(to, card)) out.push({ mk: m, ric: to, id: m.dataset.ev });
    });
    return out;
  }
  function packMarks(cell, pairs, back) {
    var going = pairs.map(function (p) { return p.mk; });
    $$('.mk, .more', cell).forEach(function (m) {
      stopAnims(m);
      if (going.indexOf(m) >= 0) { m.classList.add('away'); return; }
      anim(m, back ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 1 }, { opacity: 0 }], { duration: 200, delay: back ? 200 : 60, easing: 'ease-out', fill: 'both' });
    });
  }
  function unpackMarks(cell) {
    cell.classList.remove('packed');
    $$('.mk, .more', cell).forEach(function (m) { stopAnims(m); m.classList.remove('away'); });
  }
  // Three motions added together: the straight trip, an arc up and down,
  // and the growth to the size it lands at. Each is kept when it ends, so
  // a flyer that lands early holds its place until it is taken away.
  function fly(P, back, o, calEl) {
    var cr = calEl.getBoundingClientRect();
    return (P.pairs || []).map(function (p, i) {
      var a = (back ? p.ric : p.mk).getBoundingClientRect(), b = (back ? p.mk : p.ric).getBoundingClientRect(), f = document.createElement('span');
      f.className = 'flyer' + (p.mk.classList.contains('dir') ? ' dir' : '');
      f.setAttribute('style', (p.mk.getAttribute('style') || '') + ';width:' + a.width.toFixed(1) + 'px;height:' + a.height.toFixed(1) + 'px');
      f.textContent = p.mk.textContent;
      P.carry.appendChild(f);
      p.f = f;
      var sx = a.left - cr.left, sy = a.top - cr.top, ex = b.left - cr.left, ey = b.top - cr.top, k = b.width / a.width;
      var lift = Math.min(26, 8 + Math.hypot(ex - sx, ey - sy) * 0.07), t = { duration: o.dur, delay: o.delay + i * o.stagger, fill: 'both' };
      var trip = anim(f, [{ transform: 'translate(' + sx.toFixed(1) + 'px,' + sy.toFixed(1) + 'px)' }, { transform: 'translate(' + ex.toFixed(1) + 'px,' + ey.toFixed(1) + 'px)' }], Object.assign({ easing: 'cubic-bezier(.45,0,.2,1)' }, t));
      [trip,
        anim(f, [{ transform: 'translateY(0px)', easing: 'cubic-bezier(.25,.6,.45,1)' }, { transform: 'translateY(' + (-lift).toFixed(1) + 'px)', offset: 0.42, easing: 'cubic-bezier(.55,0,.75,.4)' }, { transform: 'translateY(0px)' }], Object.assign({ composite: 'add' }, t)),
        anim(f, [{ transform: 'scale(1)' }, { transform: 'scale(' + (Math.max(k, 1) * 1.16).toFixed(3) + ')', offset: 0.45 }, { transform: 'scale(' + k.toFixed(3) + ')' }], Object.assign({ composite: 'add', easing: 'ease-in-out' }, t))
      ].forEach(function (x) { x.persist(); });
      return trip;
    });
  }
  // Flyers land: on the way in the card's icons appear, on the way back
  // the cell's marks do.
  function land(P, back) {
    (P.pairs || []).forEach(function (p) {
      if (p.f) { stopAnims(p.f); p.f.remove(); p.f = null; }
      if (!back) p.ric.classList.remove('wait');
    });
    if (P.src && P.carried) {
      if (back) unpackMarks(P.src);
      else { P.src.classList.add('packed'); $$('.mk, .more', P.src).forEach(function (m) { stopAnims(m); }); }
    }
  }
  function clearCarry(P) {
    if (!P.carry) return;
    stopAnims(P.carry, true);
    P.carry.innerHTML = '';
    $$('.ric.wait', P.el).forEach(function (r) { r.classList.remove('wait'); });
  }

  // A card's state: which element, what opened it, where it lies, the
  // marks it carries, and a token so a later open or close supersedes an
  // earlier one.
  function cardState(el, carry) { return { el: el, carry: carry || null, state: 'closed', seq: 0, next: null, g: null, src: null, press: null, pairs: null, carried: false, closing: null }; }
  function resetCard(el) { stopAnims(el, true); el.classList.remove('live', 'open'); el.removeAttribute('style'); }
  // Resolves true once the card is fully open, false if something
  // superseded it.
  function openCard(P, calEl) {
    var tok = ++P.seq, el = P.el, run;
    P.state = 'opening';
    el.classList.add('live');
    if (reducedMotion()) {
      run = anim(el, [{ opacity: 0 }, { opacity: 1 }], { duration: 180, easing: 'ease-out' }).finished;
    } else if (isPhone() || !P.g) {
      if (P.src) originOn(el, P.src);
      pressIn(P.press);
      var fl = fly(P, false, { delay: 60, dur: 420, stagger: 26 }, calEl);
      run = settleAll([anim(el, [{ transform: 'scale(.08)' }, { transform: 'scale(1)' }], { duration: 360, delay: 40, easing: EASE, fill: 'both' })].concat(fl));
      anim(el, [{ opacity: 0 }, { opacity: 1, offset: 0.08 }, { opacity: 1 }], { duration: 360, delay: 40, easing: 'linear', fill: 'both' });
    } else {
      // The marks are aimed at the card as it will lie, before its leaves
      // turn; each lands after its leaf lies flat.
      var held = foldShort(P), lift = held ? heldLift(P, held) : 0;
      if (lift) el.style.top = (P.g.final.y + lift) + 'px';
      var F = foldGeo(P, calEl), marks = fly(P, false, { delay: 110, dur: 390, stagger: 30 }, calEl);
      pressIn(P.press);
      run = settleAll(foldRun(F, false, 1).concat(marks));
    }
    return run.then(function () {
      if (tok !== P.seq) throw new Error('superseded');
      stopAnims(el, true);
      land(P, false);
      if (held) {
        freeLeaf(held);
        slideLeaf(held, held.cap, held.full, leafTime(held, FOLD.letDown));
        if (lift) liftCard(P, lift, 0, leafTime(held, FOLD.letDown));
      }
      P.state = 'open';
      el.classList.add('open');
      return true;
    }).catch(function () { return false; });
  }
  function closeCard(P, calEl, o) {
    o = o || {};
    var tok = ++P.seq, el = P.el, sp = o.fast ? 1.8 : 1, run;
    P.state = 'closing';
    el.classList.remove('open');
    // Marks still flying in are dropped; the ones in the card fly back
    // from where they are.
    clearCarry(P);
    var moving = !o.instant && P.src && P.src.isConnected && !reducedMotion();
    if (moving && P.carried) {
      P.pairs = carryPairs(P.src, el);
      P.src.classList.remove('packed');
      packMarks(P.src, P.pairs, true);
      P.pairs.forEach(function (p) { p.ric.classList.add('wait'); });
    } else {
      P.pairs = [];
    }
    if (o.instant || !P.src || !P.src.isConnected) {
      run = Promise.resolve();
    } else if (reducedMotion()) {
      run = anim(el, [{ opacity: 1 }, { opacity: 0 }], { duration: 160, easing: 'ease-in' }).finished;
    } else if (isPhone() || !P.g) {
      originOn(el, P.src);
      var fl = fly(P, true, { delay: 0, dur: 340 / sp, stagger: 22 / sp }, calEl);
      run = settleAll([anim(el, [{ transform: 'scale(1)' }, { transform: 'scale(.08)' }], { duration: 280 / sp, delay: 60 / sp, easing: 'cubic-bezier(.5,0,.75,0)', fill: 'both' })].concat(fl));
      anim(el, [{ opacity: 1 }, { opacity: 1, offset: 0.9 }, { opacity: 0 }], { duration: 280 / sp, delay: 60 / sp, easing: 'linear', fill: 'both' });
      pressIn(P.press, { delay: 300 / sp, dur: 280 / sp, soft: true });
    } else {
      var marks = fly(P, true, { delay: 0, dur: 340 / sp, stagger: 24 / sp }, calEl), held = foldShort(P);
      var up = held ? leafTime(held, FOLD.takeUp) / sp : 0, lift = 0;
      if (held) { lift = heldLift(P, held); freeLeaf(held); }
      var folded = (held ? Promise.all([slideLeaf(held, held.full, held.cap, up), lift ? liftCard(P, 0, lift, up) : null]).then(function () { holdLeaf(held, held.cap); }) : Promise.resolve())
        .then(function () { return settleAll(foldRun(foldGeo(P, calEl), true, sp)); });
      run = Promise.all([folded, settleAll(marks)]);
      // The card tucks back into what it came from, which gives a little.
      pressIn(P.press, { delay: up + FOLD.pressBack / sp, dur: 300 / sp, soft: true });
    }
    return run.then(function () { return true; }, function () { return false; }).then(function () {
      if (tok !== P.seq) return false;
      land(P, true);
      clearCarry(P);
      if (P.src && P.src.isConnected) unpackMarks(P.src);
      resetCard(el);
      P.carried = false;
      P.pairs = null;
      P.state = 'closed';
      P.g = null;
      P.src = null;
      return true;
    });
  }

  // ================================================================
  // EraMath: where each era sits on the day counter, which era a day
  // belongs to, and how an era's span reads. Pure, so the node suite can
  // pin it. Eras arrive already filtered for the viewer by the server: a
  // player never has an era hidden until it begins, and the era before one
  // reads as still going.
  // ================================================================
  // A page, drawn inline so the era panel's lore row never shows an empty
  // box when the icon font is slow or blocked.
  var PAGE_GLYPH = '<svg width="13" height="15" viewBox="0 0 13 15" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round" stroke-linecap="round"><path d="M2 1h6l3 3v10H2z"/><path d="M8 1v3h3M4.5 7.5h4M4.5 10h4"/></svg>';

  var EraMath = {
    startOf: function (e) { return { y: e.start_year, m: e.start_month || 1, d: e.start_day || 1 }; },
    // An end year with no month/day is the whole year, as on the server.
    explicitEndOf: function (cal, e) {
      if (e.end_year == null) return null;
      if (e.end_month == null || e.end_day == null) {
        var mc = CalDate.monthCount(cal);
        return { y: e.end_year, m: mc, d: CalDate.monthDays(cal, mc - 1, e.end_year) };
      }
      return { y: e.end_year, m: e.end_month, d: e.end_day };
    },
    idx: function (cal, t) { return CalDate.dayIndex(cal, t.y, t.m, t.d); },
    // sorted returns the eras oldest first, each with its start and
    // (explicit or implied by the next era) end resolved.
    sorted: function (cal) {
      var list = (cal.eras || []).map(function (e) {
        var s = EraMath.startOf(e), x = EraMath.explicitEndOf(cal, e);
        return { era: e, start: s, startK: EraMath.idx(cal, s), end: x, endK: x ? EraMath.idx(cal, x) : null, explicitEnd: !!x };
      });
      list.sort(function (a, b) { return a.startK - b.startK || (a.era.sort_order || 0) - (b.era.sort_order || 0); });
      for (var i = 0; i < list.length; i++) {
        if (list[i].end || i + 1 >= list.length) continue;
        list[i].end = CalDate.addDays(cal, list[i + 1].start, -1);
        list[i].endK = list[i + 1].startK - 1;
      }
      return list;
    },
    // at returns the era a day belongs to: the latest-starting era that has
    // begun and has not explicitly ended (an implied end never cuts it off,
    // since the next era has taken over by then anyway).
    at: function (list, k) {
      var hit = null;
      for (var i = 0; i < list.length; i++) {
        var r = list[i];
        if (r.startK > k) continue;
        if (r.explicitEnd && r.endK < k) continue;
        hit = r;
      }
      return hit;
    },
    // month describes one month's eras: the one it opens in, the one it
    // closes in (when different, the day that one takes over), and any era
    // that begins inside it.
    month: function (cal, list, y, m) {
      var mc = CalDate.monthCount(cal), m0 = ((m - 1) % mc + mc) % mc;
      var days = CalDate.monthDays(cal, m0, y), k0 = CalDate.dayIndex(cal, y, m, 1), k1 = k0 + days - 1;
      var first = EraMath.at(list, k0), last = EraMath.at(list, k1);
      var info = { days: days, first: first, last: last, change: null, begins: [] };
      for (var i = 0; i < list.length; i++) {
        if (list[i].startK >= k0 && list[i].startK <= k1) info.begins.push({ day: list[i].startK - k0 + 1, row: list[i] });
      }
      if (first && last && first !== last) info.change = last.startK - k0 + 1;
      return info;
    },
    // span reads a day count as years, months and days of this calendar
    // (two parts at most; days only when under a year).
    span: function (cal, n) {
      var yl = CalDate.yearLength(cal) || 365, ml = yl / CalDate.monthCount(cal);
      var y = Math.floor(n / yl), rest = n - y * yl, mo = Math.floor(rest / ml), d = Math.round(rest - mo * ml), p = [];
      if (y) p.push(y.toLocaleString('en') + (y === 1 ? ' year' : ' years'));
      if (mo) p.push(mo + (mo === 1 ? ' month' : ' months'));
      if (d && !y) p.push(d + (d === 1 ? ' day' : ' days'));
      return p.slice(0, 2).join(', ') || 'one day';
    },
    fmt: function (cal, t) {
      var mo = (cal.months || [])[t.m - 1];
      return t.d + ' ' + (mo ? mo.name : 'Month ' + t.m) + ' ' + t.y;
    },
    // when is the era's dates and length, measured against the calendar's
    // current date ("today" in the world).
    when: function (cal, r) {
      var today = CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
      if (r.startK > today) return { range: 'From ' + EraMath.fmt(cal, r.start), len: 'Begins in ' + EraMath.span(cal, r.startK - today) };
      if (r.end && r.endK < today) return { range: EraMath.fmt(cal, r.start) + ' – ' + EraMath.fmt(cal, r.end), len: 'Lasted ' + EraMath.span(cal, r.endK - r.startK + 1) };
      return { range: EraMath.fmt(cal, r.start) + ' – today', len: EraMath.span(cal, today - r.startK + 1) + ' so far' };
    },
    // ribbon gives each era's share of the ribbon: square-root lengths with
    // a floor, so a short recent era is still easy to hit.
    ribbon: function (cal, list) {
      var today = CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day), yl = CalDate.yearLength(cal) || 365;
      var lens = list.map(function (r) {
        if (r.end) return Math.max(1, r.endK - r.startK + 1);
        return r.startK > today ? 5 * yl : Math.max(1, Math.round((today - r.startK + 1) / 0.72));
      });
      var roots = lens.map(Math.sqrt), sum = roots.reduce(function (a, b) { return a + b; }, 0) || 1, MIN = 18;
      var rest = Math.max(0, 100 - MIN * list.length);
      return list.map(function (r, i) {
        var here = r.startK <= today && (r.endK == null || r.endK >= today) ? Math.min(1, (today - r.startK + 1) / lens[i]) : null;
        return { flex: MIN + rest * roots[i] / sum, len: lens[i], here: here };
      });
    },
    // secret marks an era the Director sees but players don't yet.
    secret: function (cal, r) {
      return !!r.era.hidden_until_begins && r.startK > CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
    }
  };

  // Exposed for calendar_editor.js (a separate script/closure loaded only
  // for a CanEdit viewer — see that file's header) so the date-math port
  // is written once and both files stay in agreement with the server, and
  // so a canEdit override of view.showFlap/etc. can open/close a panel with
  // the exact same grow-from-anchor motion as every other panel here.
  Chronicle.calendarDate = CalDate;
  Chronicle.calendarEras = EraMath;
  Chronicle.calendarPanel = { growOpen: growOpen, growClose: growClose };
  Chronicle.calendarColor = sanitizeColor;
  Chronicle.calendarPageGlyph = PAGE_GLYPH;
  Chronicle.calendarWeatherIcon = weatherIcon;
  Chronicle.calendarWindWords = windWords;
  Chronicle.calendarRealAnchor = realAnchor;

  // ================================================================
  Chronicle.register('calendar_view', {
    init: function (el, config) {
      var self = this;
      this.el = el;
      this.campaignId = config.campaignId;
      this.calendarId = config.calendarId;
      this.canEdit = config.canEdit === true;
      this.canAuthorDmOnly = config.canAuthorDmOnly === true;
      this.role = typeof config.role === 'number' ? config.role : parseInt(config.role, 10) || 0;
      this.userId = config.userId || '';
      // Game nights have their own switch; with it off the calendar shows
      // no nights, RSVP or free hours, since those routes are closed.
      this.gameNights = config.gameNights === true;
      this.apiBase = config.apiBase;
      this.engineSrc = config.engineSrc; // chronicle_gen.js, loaded only when an editor paints weather

      var cfgEl = $('#calendar-config', el);
      try { this.cal = JSON.parse(cfgEl.dataset.calendar || '{}'); } catch (e) { this.cal = {}; }
      var initialEvents;
      try { initialEvents = JSON.parse(cfgEl.dataset.events || '[]'); } catch (e2) { initialEvents = []; }
      if (!Array.isArray(initialEvents)) initialEvents = [];

      this.eventsByMonth = {};
      this.forecastByDay = {};
      this.weatherByYear = {}; // year -> {'m_d': reading}, filled by fetchWeatherYear
      this.nightsByMonth = {}; // 'y_m' -> game nights, filled by fetchNights
      this._gnPages = {}; // session id -> its recap and linked pages
      // Game nights are real-world dates, so only a real-world calendar
      // shows them; a world calendar keeps to the story. Only members see
      // who is coming.
      this._anchor = realAnchor(this.cal);
      this.showNights = this.gameNights && this.role >= 1 && CalDate.usesRealTime(this.cal);
      this.calZone = cfgEl.dataset.zone || ''; // the real-world calendar's zone, members only
      this._gnZoneMode = readZoneMode();
      // Who's free: members' painted hours (the sessions plugin's overlay),
      // on a real-world calendar only, since hours are real dates. The
      // Director and co-Directors get the per-player lines and Best times;
      // everyone else sees counts in a day's card.
      this.freeCan = this.showNights && CalDate.usesRealTime(this.cal);
      this.freeDirector = this.freeCan && this.canAuthorDmOnly;
      this.freeByDate = {}; // 'YYYY-MM-DD' -> {total, hours[24], members[], detail}
      this._freeWeeks = {}; // week start -> Promise of that week's read
      var freeOn = false;
      // On unless the Director turned the lines off: they are the reason
      // to open the calendar when planning, and a switch inside a panel is
      // easy never to find.
      try { freeOn = window.localStorage.getItem(FREE_KEY) !== '0'; } catch (e3) { freeOn = true; }
      this.showFree = this.freeDirector && freeOn;
      this._gnNoteFor = null; // the night whose note is being written
      this.eventsByMonth[this.cal.current_year + '_' + this.cal.current_month] = initialEvents;

      this.view = { y: this.cal.current_year || 1, m: this.cal.current_month || 1 };
      this.rm = reducedMotion();
      this.wingFor = null; // key of the day whose wing is open, or null
      this._roll = null;   // the open "Roll one" roller in the day card, or null
      this._wingStale = null; // the open day's card, redrawn once it finishes unfolding
      this.selection = {}; // edit-mode multi-select, keyed by dayKey -> true

      this._buildShell();
      this._pw = cardState(this.wingEl, this.carryEl);
      this._pf = cardState(this.flapEl);
      this._bindEvents();
      this._dockSky();
      this.renderMonth();
      this.renderToday();
      this.renderLegend();
      this._openFromLink();

      // Hand the instance to calendar_editor.js (loaded only for an
      // Owner/co-Director viewer) without assuming load order.
      el.calendarView = this;
      el.dispatchEvent(new CustomEvent('calendarv5:ready', { detail: this, bubbles: false }));
    },

    destroy: function (el) {
      if (this.eraField) this.eraField.destroy();
      if (this._eraRO) this._eraRO.disconnect();
      // boot.js reuses this one object for every mount, so a remount must
      // build a fresh painter on the new stage, not reuse the dead one.
      this.eraField = null;
      this._eraRO = null;
      this._eraScene = null;
      this._eraOpen = null;
      this._eraOpener = null;
      if (this.skyDock) this.skyDock.destroy();
      if (this._eventDrawer) this._eventDrawer.destroy();
      if (window.Chronicle && Chronicle.calendarGameNight) Chronicle.calendarGameNight.destroy(this);
      if (this._resizeHandler) window.removeEventListener('resize', this._resizeHandler);
      if (this._escHandler) document.removeEventListener('keydown', this._escHandler);
      if (this._mvOffHandler) document.removeEventListener('pointerdown', this._mvOffHandler, true);
      if (this._plUp) document.removeEventListener('pointerup', this._plUp);
    },

    // --------------------------------------------------------------
    // Shell: the header, grid containers and every fold-out's mount
    // point. Built once; renderMonth()/renderToday()/etc. only touch
    // their own pieces afterward.
    // --------------------------------------------------------------
    _buildShell: function () {
      var canEditAttr = this.canEdit ? ' data-can-edit="true"' : '';
      this.el.innerHTML =
        '<section class="cal" id="cal5-cal" aria-label="Calendar"' + canEditAttr + '>' +
          '<div class="skywrap" id="cal5-skywrap" hidden><div class="sky" role="img" aria-label="The sky"><canvas></canvas></div>' +
            (this.canAuthorDmOnly ? '<div class="skyprev" id="cal5-skyprev" title="Only you see this. Players still see today’s sky." hidden><span class="spd"></span><input type="range" class="spt" aria-label="Time of day"><span class="sph"></span><button type="button" class="spx" data-sky-now>Back to now</button></div>' : '') +
          '</div>' +
          '<header class="head">' +
            '<div class="h-row h-top">' +
              '<button type="button" class="hub" id="cal5-hub" aria-haspopup="dialog" aria-expanded="false"><span>' + esc(this.cal.name || 'Calendar') + '</span><i class="fa-solid fa-chevron-down"></i></button>' +
              '<div class="todaypill" id="cal5-todaypill"></div>' +
              '<div class="h-acts">' +
                '<button type="button" class="skybtn" id="cal5-skybtn" aria-expanded="false" aria-controls="cal5-skywrap" title="Fold or open the sky" hidden><span class="sw" aria-hidden="true"></span><span>Sky</span></button>' +
                (this.freeCan ? '<button type="button" class="tbtn" id="cal5-freebtn" aria-haspopup="dialog" aria-expanded="false" title="' + (this.freeDirector ? 'When players are free, and the best times to play' : 'Give your hours, and see who is free') + '"><i class="fa-solid fa-user-clock"></i><span>Who’s free</span></button>' : '') +
                '<button type="button" class="tbtn" id="cal5-moonbtn" aria-haspopup="dialog" aria-expanded="false"><i class="fa-solid fa-moon"></i><span>Moons</span></button>' +
              '</div>' +
            '</div>' +
            '<div class="h-row h-mid">' +
              '<h2 class="mtitle" id="cal5-mtitle"></h2>' +
              '<nav class="mnav" aria-label="Change month">' +
                '<button type="button" class="ib" id="cal5-prev" aria-label="Previous month"><i class="fa-solid fa-chevron-left"></i></button>' +
                '<button type="button" class="tbtn plain" id="cal5-todaybtn">Today</button>' +
                '<button type="button" class="ib" id="cal5-next" aria-label="Next month"><i class="fa-solid fa-chevron-right"></i></button>' +
              '</nav>' +
            '</div>' +
            '<div class="h-row h-subw">' +
              '<div class="h-row h-sub" id="cal5-hsub">' +
                '<span class="erachips" id="cal5-erachips"></span>' +
                '<span class="season" id="cal5-season"></span>' +
              '</div>' +
            '</div>' +
          '</header>' +
          '<div class="eratip" id="cal5-eratip" role="tooltip" hidden></div>' +
          '<div class="erapanel" id="cal5-erapanel" role="region" aria-label="Era" hidden></div>' +
          '<div class="dow" id="cal5-dow" aria-hidden="true"></div>' +
          '<div class="stage" id="cal5-stage"></div>' +
          '<div class="hc" id="cal5-hc" aria-hidden="true"></div>' +
          '<div class="wing" id="cal5-wing" role="dialog" aria-label="Day" tabindex="-1"></div>' +
          '<div class="evp" id="cal5-evp" role="dialog" aria-label="Event" tabindex="-1"></div>' +
          '<div class="mv" id="cal5-mv" role="dialog" aria-label="Moons" tabindex="-1"></div>' +
          '<div class="mv fv" id="cal5-fv" role="dialog" aria-label="Who’s free" tabindex="-1"></div>' +
          '<div class="flap" id="cal5-flap" role="dialog" aria-label="Era" tabindex="-1"></div>' +
          '<div class="pop" id="cal5-pop" role="dialog" aria-label="Calendars you can see" tabindex="-1"></div>' +
          '<div class="carry" id="cal5-carry" aria-hidden="true"></div>' +
          '<div class="scrim" id="cal5-scrim"></div>' +
          '<div class="dock" id="cal5-dock"></div>' +
        '</section>' +
        '<div class="caption" id="cal5-caption">' +
          '<p>Hover a day’s marks for a glance; click a day to open its card, then an event’s box for everything about it. Arrows move between days, T jumps to today, Page Up/Down change month.</p>' +
          '<ul class="legend" id="cal5-legend" aria-label="What the marks mean"></ul>' +
        '</div>' +
        '<div class="sr" id="cal5-live" aria-live="polite"></div>';

      if (this.rm) document.documentElement.classList.add('rm');

      this.calEl = $('#cal5-cal', this.el);
      this.dowEl = $('#cal5-dow', this.el);
      this.stageEl = $('#cal5-stage', this.el);
      this.hcEl = $('#cal5-hc', this.el);
      this.wingEl = $('#cal5-wing', this.el);
      this.evpEl = $('#cal5-evp', this.el);
      this.mvEl = $('#cal5-mv', this.el);
      this.flapEl = $('#cal5-flap', this.el);
      this.popEl = $('#cal5-pop', this.el);
      this.scrimEl = $('#cal5-scrim', this.el);
      this.carryEl = $('#cal5-carry', this.el);
      this.dockEl = $('#cal5-dock', this.el);
      this.eraChipsEl = $('#cal5-erachips', this.el);
      this.eraTipEl = $('#cal5-eratip', this.el);
      this.eraPanelEl = $('#cal5-erapanel', this.el);
      this.fvEl = $('#cal5-fv', this.el);
    },

    // action, optional: {label, run} adds a button to the toast (Undo).
    say: function (msg, action) {
      this._announce(msg);
      this._toast(msg, action);
    },

    // Screen readers only: for things the eye already sees happen.
    _announce: function (msg) {
      var l = $('#cal5-live', this.el);
      l.textContent = '';
      setTimeout(function () { l.textContent = msg; }, 40);
    },

    // --------------------------------------------------------------
    // The sky over the month (#830): SkyPane.Dock (sky_pane.js) draws it
    // and folds it. Without the sky scripts or a canvas it stays missing.
    // --------------------------------------------------------------
    _dockSky: function () {
      var self = this, wrap = $('#cal5-skywrap', this.el), chip = $('#cal5-skybtn', this.el);
      if (!window.SkyPane || !SkyPane.Dock) return;
      wrap.hidden = false;
      try {
        this.skyDock = new SkyPane.Dock({
          card: this.calEl, wrap: wrap, chip: chip, seed: this.calendarId, name: this.cal.name,
          campaignId: this.campaignId, calendarId: this.calendarId,
          followers: function () {
            var out = [], n;
            for (n = self.calEl.nextElementSibling; n; n = n.nextElementSibling) out.push(n);
            for (n = self.el.nextElementSibling; n; n = n.nextElementSibling) out.push(n);
            return out;
          },
          // The chip is outside every card, so pressing it puts them away.
          before: function () { self.closeAllPanels({ instant: true }); },
          travel: function () { return !self._fixedShowing(); },
          say: function (msg) { self._announce(msg); }
        });
      } catch (e) {
        wrap.hidden = true;
        this.skyDock = null;
        return;
      }
      chip.hidden = false;
      // boot.js reuses this object for every mount, so a fresh dock starts
      // with no day drawn, whatever the last calendar showed.
      this._skySig = null;
      this._skyPrev = null;
      var bar = $('#cal5-skyprev', this.el);
      if (bar) {
        bar.addEventListener('click', function (e) { if (e.target.closest('[data-sky-now]')) self.endSkyPreview(); });
        $('.spt', bar).addEventListener('input', function () {
          var p = self._skyPrev, mph = self.cal.minutes_per_hour || 60, v = parseInt(this.value, 10) || 0;
          if (!p) return;
          p.h = Math.floor(v / mph); p.min = v % mph;
          self._skyDay();
        });
      }
      this._skyDay();
    },

    // Today's sky, redrawn only when the moment or today's events change.
    // The calendar and events here are already what this viewer may see.
    _skyDay: function () {
      var c = this.cal, p = this._skyPrev;
      // A preview shows only that day's own weather, never today's.
      if (p) c = Object.assign({}, c, { current_year: p.y, current_month: p.m, current_day: p.d, current_hour: p.h, current_minute: p.min, weather: null });
      this._skyPrevBar();
      if (!this.skyDock || !this.eventsByMonth[this.monthKey(c.current_year, c.current_month)]) return;
      var evs = this.eventsOnDay(c.current_year, c.current_month, c.current_day);
      var sig = [this.calendarId, c.current_year, c.current_month, c.current_day, c.current_hour, c.current_minute].join('/') + '|' +
        evs.map(function (e) { return e.id + ':' + (e.updated_at || ''); }).join(',');
      if (sig === this._skySig) return;
      this._skySig = sig;
      this.skyDock.setDay(c, evs);
    },

    // A private look at another day's sky, for viewers with DM access: only
    // this page's sky changes; the campaign's date and what players see stay.
    previewSky: function (y, m, d) {
      if (!this.skyDock || !this.canAuthorDmOnly) return;
      var self = this, c = this.cal, p = this._skyPrev;
      this._skyPrev = { y: y, m: m, d: d, h: p ? p.h : c.current_hour || 0, min: p ? p.min : c.current_minute || 0 };
      if (!this.skyDock.S.layout) this.skyDock.fold(true);
      this.fetchMonth(y, m).then(function () { if (self._skyPrev && self._skyPrev.d === d) self._skyDay(); });
      this._skyDay();
      this._announce('Showing the sky on ' + this._dateLabel(y, m, d) + '. Only you see this.');
    },
    endSkyPreview: function () {
      if (!this._skyPrev) return;
      this._skyPrev = null;
      this._skyDay();
      this._announce('Showing the sky now.');
    },
    // The strip's own line while a preview shows: which day, its time, and the way back.
    _skyPrevBar: function () {
      var bar = $('#cal5-skyprev', this.el), p = this._skyPrev, c = this.cal;
      if (!bar) return;
      bar.hidden = !p;
      if (!p) return;
      var mph = c.minutes_per_hour || 60, hpd = c.hours_per_day || 24, r = $('.spt', bar);
      $('.spd', bar).textContent = this._dateLabel(p.y, p.m, p.d);
      $('.sph', bar).textContent = pad2(p.h) + ':' + pad2(p.min);
      r.min = 0; r.max = hpd * mph - 1; r.step = Math.max(1, Math.round(mph / 6));
      r.value = p.h * mph + p.min;
    },

    // A transform on the card would carry its phone bars and sheets
    // (position:fixed) along with it, so while one shows the sky folds
    // without travelling.
    _fixedShowing: function () {
      var els = this.calEl.querySelectorAll('.bbar:not([hidden]), .btray:not([hidden]), .drawer.open, .toast.on');
      for (var i = 0; i < els.length; i++) {
        if (getComputedStyle(els[i]).position === 'fixed' && els[i].getClientRects().length) return true;
      }
      return false;
    },

    // Every say() call is also a visible, self-dismissing toast — calendar-view.css
    // already ships '.toast' (an "undo" style, scoped .cal-v5 .toast, with its
    // own reduced-motion handling) but nothing built or showed one, so a write's
    // outcome — success or failure, and a no-op explanation like "This calendar
    // has no moons." — was announced only to the #cal5-live aria-live region
    // above, invisible to a sighted viewer. aria-hidden here so it isn't a
    // second, redundant announcement on top of that live region.
    // A toast with an action keeps its button reachable and stays up
    // longer, so the button can be found and pressed.
    _toast: function (msg, action) {
      var el = this._toastEl;
      if (!el) {
        el = document.createElement('div');
        el.className = 'toast';
        el.innerHTML = '<span></span><button type="button" hidden></button>';
        this.calEl.appendChild(el);
        this._toastEl = el;
        el.querySelector('button').addEventListener('click', function () {
          var run = el._run;
          el._run = null;
          el.classList.remove('on');
          if (run) run();
        });
      }
      var btn = el.querySelector('button');
      el.querySelector('span').textContent = msg;
      btn.hidden = !action;
      btn.textContent = action ? action.label : '';
      el._run = action ? action.run : null;
      // The live region already reads the message; with an action only the
      // button is exposed, so nothing is heard twice.
      if (action) el.removeAttribute('aria-hidden'); else el.setAttribute('aria-hidden', 'true');
      el.querySelector('span').setAttribute('aria-hidden', 'true');
      el.classList.add('on');
      clearTimeout(this._toastTimer);
      this._toastTimer = setTimeout(function () { el.classList.remove('on'); el._run = null; }, action ? 8000 : 3200);
    },

    // --------------------------------------------------------------
    // Data access
    // --------------------------------------------------------------
    monthKey: function (y, m) { return y + '_' + m; },

    eventsFor: function (y, m) { return this.eventsByMonth[this.monthKey(y, m)] || []; },

    // fetchMonth loads (and caches) a month's events. Returns a Promise.
    fetchMonth: function (y, m) {
      var self = this, key = this.monthKey(y, m);
      if (this.eventsByMonth[key]) return Promise.resolve(this.eventsByMonth[key]);
      return Chronicle.apiFetch(this.apiBase + '/events?year=' + y + '&month=' + m)
        .then(function (resp) { return resp.ok ? resp.json() : []; })
        .then(function (list) {
          if (!Array.isArray(list)) list = [];
          self.eventsByMonth[key] = list;
          return list;
        })
        .catch(function () { return []; });
    },

    // fetchWeatherYear loads (and caches) a year's day weather. The server
    // already leaves out days a viewer may not see yet. Returns a Promise.
    fetchWeatherYear: function (y) {
      var self = this;
      if (this.weatherByYear[y]) return Promise.resolve(this.weatherByYear[y]);
      return Chronicle.apiFetch(this.apiBase + '/weather/days?year=' + y)
        .then(function (resp) { return resp.ok ? resp.json() : []; })
        .then(function (list) {
          var byDay = {};
          (Array.isArray(list) ? list : []).forEach(function (w) { byDay[w.month + '_' + w.day] = w; });
          self.weatherByYear[y] = byDay;
          return byDay;
        })
        .catch(function () { return {}; });
    },

    // The day's weather reading, or null. Today falls back to the calendar's
    // single current reading, which predates per-day weather.
    // fetchForecast loads the player forecast once (again when force). An
    // empty list means the forecast is off.
    fetchForecast: function (force) {
      var self = this;
      if (this._forecastP && !force) return this._forecastP;
      this._forecastP = Chronicle.apiFetch(this.apiBase + '/weather/forecast')
        .then(function (resp) { return resp.ok ? resp.json() : []; })
        .then(function (list) {
          var by = {};
          (Array.isArray(list) ? list : []).forEach(function (e) { by[e.year + '_' + e.month + '_' + e.day] = e; });
          self.forecastByDay = by;
          return by;
        })
        .catch(function () { self.forecastByDay = {}; return {}; });
      return this._forecastP;
    },

    // The forecast entry for a day, or null.
    forecastOn: function (y, m, d) {
      return (this.forecastByDay && this.forecastByDay[y + '_' + m + '_' + d]) || null;
    },

    // What a player sees on a future day with no visible reading: the
    // forecast entry. The Director never gets this mark (real weather shows).
    forecastMarkOn: function (y, m, d, future) {
      if (this.canAuthorDmOnly || !future || this.weatherOnDay(y, m, d)) return '';
      return forecastMarkHTML(this.forecastOn(y, m, d));
    },

    weatherOnDay: function (y, m, d) {
      var byDay = this.weatherByYear[y], w = byDay && byDay[m + '_' + d];
      if (w) return w;
      var cal = this.cal;
      return y === cal.current_year && m === cal.current_month && d === cal.current_day && cal.weather ? cal.weather : null;
    },

    // --------------------------------------------------------------
    // Game nights: a real-world calendar shows the campaign's game nights
    // (the sessions plugin's), and members answer Going / Maybe / Can't
    // right here, in the day's card and on the night's own page. Silence
    // shows as "no answer yet", never as a no. Each night of a repeating
    // game night keeps its own answers.
    // --------------------------------------------------------------
    nightsFor: function (y, m) { return this.nightsByMonth[this.monthKey(y, m)] || []; },

    // _realIso is the real date (YYYY-MM-DD) a calendar day falls on: the
    // day itself on a real-world calendar, the anchor's count on an anchored
    // world calendar, '' when the calendar has no real dates.
    _realIso: function (y, m, d) {
      if (CalDate.usesRealTime(this.cal)) return y + '-' + pad2(m) + '-' + pad2(d);
      if (!this._anchor) return '';
      try {
        var iso = new Date(this._anchor.ms + (CalDate.absoluteDay(this.cal, y, m, d) - this._anchor.abs) * 86400000).toISOString().slice(0, 10);
        return /^\d{4}-\d{2}-\d{2}$/.test(iso) ? iso : '';
      } catch (e) { return ''; }
    },

    // fetchNights loads (and caches) a month's game nights. o.fresh skips
    // the cache, after an answer is saved. A world month longer than one
    // read allows is read in pieces. Returns a Promise.
    fetchNights: function (y, m, o) {
      var self = this, key = this.monthKey(y, m);
      if (!this.showNights) return Promise.resolve([]);
      if (this.nightsByMonth[key] && !(o && o.fresh)) return Promise.resolve(this.nightsByMonth[key]);
      var last = CalDate.monthDays(this.cal, m - 1, y), reads = [];
      for (var start = 1; start <= last; start += 60) {
        var from = this._realIso(y, m, start), to = this._realIso(y, m, Math.min(last, start + 59));
        if (!from || !to) continue;
        reads.push(Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/sessions/nights?from=' + from + '&to=' + to)
          .then(function (resp) { return resp.ok ? resp.json() : []; })
          .then(function (list) { return Array.isArray(list) ? list : []; }));
      }
      return Promise.all(reads)
        .then(function (parts) {
          var list = [].concat.apply([], parts);
          self.nightsByMonth[key] = list;
          return list;
        })
        .catch(function () { return self.nightsByMonth[key] || []; });
    },

    nightsOnDay: function (y, m, d) {
      if (!this.showNights) return [];
      var iso = this._realIso(y, m, d);
      return iso ? this.nightsFor(y, m).filter(function (n) { return n.date === iso; }) : [];
    },

    // A game-night link (the sidebar's "Game nights", the RSVP card) lands
    // here with ?night=<session>&date=<day>, or ?night=next, and opens that
    // night's day with its box picked out. Only a real-world calendar is
    // linked to, so the date is the grid's own.
    _linkTarget: function () {
      if (!this.showNights || !CalDate.usesRealTime(this.cal)) return null;
      var q;
      try { q = new URLSearchParams(window.location.search); } catch (e) { return null; }
      var night = q.get('night'), date = q.get('date') || '';
      if (!night) return null;
      if (night === 'next') return { next: true };
      return /^\d{4}-\d{2}-\d{2}$/.test(date) ? { sessionId: night, date: date } : { next: true };
    },

    // _nextNight finds the first game night not yet played, from this month
    // through the two after it. Resolves to the night or null.
    _nextNight: function () {
      var self = this, y = this.cal.current_year, m = this.cal.current_month, months = [];
      for (var i = 0; i < 3; i++) {
        months.push({ y: y, m: m });
        m++;
        if (m > 12) { m = 1; y++; }
      }
      return Promise.all(months.map(function (o) { return self.fetchNights(o.y, o.m); })).then(function (lists) {
        var all = [].concat.apply([], lists).filter(function (n) { return !n.past; });
        all.sort(function (a, b) { return a.date < b.date ? -1 : a.date > b.date ? 1 : 0; });
        return all[0] || null;
      });
    },

    _openFromLink: function () {
      var self = this, t = this._linkTarget();
      if (!t) return;
      (t.next ? this._nextNight() : Promise.resolve(t)).then(function (n) {
        if (!n) return null;
        var p = n.date.split('-'), y = +p[0], m = +p[1], d = +p[2];
        if (self.view.y !== y || self.view.m !== m) { self.view = { y: y, m: m }; self.renderMonth(); }
        return self.fetchNights(y, m).then(function () {
          self._paintMonth();
          self.openWing(dayKey(y, m, d), 'gn:' + n.sessionId + ':' + n.date);
        });
      }).catch(function () { /* the calendar still shows; only the jump is lost */ });
    },

    // A game night's id in the grid and the cards: "gn:" + session + date,
    // so the day card's marks carry it like any event's.
    _gnId: function (n) { return 'gn:' + n.sessionId + ':' + n.date; },

    _findNight: function (id) {
      var self = this;
      return this.nightsFor(this.view.y, this.view.m).filter(function (n) { return self._gnId(n) === id; })[0] || null;
    },

    // _gnWhen is a night's start as this viewer reads it: in the calendar's
    // zone (or, on a calendar with none, the zone the night was set in),
    // labelled like "19:00 CDT", or in the viewer's own zone once they ask.
    // canSwitch is false when both read the same, so no switch is offered.
    // A night set without a zone shows its bare time: there is nothing to
    // convert from.
    _gnWhen: function (n) {
      if (!n.time) return null;
      var hm = String(n.time).slice(0, 5), src = n.tz || this.calZone;
      if (!src) return { text: hmWords(hm), canSwitch: false };
      var self = this, ref = this.calZone || src, mineZ = browserZone();
      function show(zone) {
        var o = zonedShow(n.date, hm, src, zone);
        if (!o) return null;
        var t = hmWords(o.hm) + (o.abbr ? ' ' + o.abbr : '');
        return o.date !== n.date ? realDateWords(o.date).split(' ')[0] + ' ' + t : t;
      }
      var calText = show(ref);
      if (calText == null) return { text: hmWords(hm), canSwitch: false };
      var mineText = mineZ ? show(mineZ) : null, canSwitch = mineText != null && mineText !== calText;
      var useMine = canSwitch && self._gnZoneMode === 'mine';
      return {
        text: useMine ? mineText : calText,
        mine: useMine,
        canSwitch: canSwitch,
        mineAbbr: canSwitch ? (zonedShow(n.date, hm, src, mineZ) || {}).abbr || '' : '',
        refLabel: this.calZone ? 'the calendar’s time' : 'the organiser’s time'
      };
    },

    _gnTime: function (n) {
      var w = this._gnWhen(n);
      return w ? w.text : '';
    },

    // The one-press switch between the calendar's time and the viewer's.
    _gnZoneHTML: function (n) {
      var w = this._gnWhen(n);
      if (!w || !w.canSwitch) return '';
      return '<div class="gnz">' + (w.mine
        ? '<span>Your time</span><button type="button" class="lnk" data-gn-zone="cal">Show ' + esc(w.refLabel) + '</button>'
        : '<span>' + esc(w.refLabel.charAt(0).toUpperCase() + w.refLabel.slice(1)) + '</span><button type="button" class="gnpill" data-gn-zone="mine">Show in my time' + (w.mineAbbr ? ' (' + esc(w.mineAbbr) + ')' : '') + '</button>') +
        '</div>';
    },

    _gnSetZoneMode: function (mode) {
      this._gnZoneMode = mode === 'mine' ? 'mine' : 'cal';
      writeZoneMode(this._gnZoneMode);
      var openId = this.evpEl.classList.contains('open') ? this.evpEl.dataset.ev : '';
      this.refreshWing();
      if (openId && openId.indexOf('gn:') === 0) {
        var n = this._findNight(openId);
        if (n) this.evpEl.innerHTML = this._gnPageHTML(n);
      }
      this._announce(this._gnZoneMode === 'mine' ? 'Times now show in your time.' : 'Times now show in the calendar’s time.');
    },

    // On an anchored world calendar the box says which real day the night
    // is, since the grid's date is the world's.
    _gnRealLine: function (n) {
      if (!this._anchor) return '';
      var t = this._gnTime(n);
      return '<div class="gnreal">Real date: ' + esc(realDateWords(n.date)) + (t ? ' · ' + esc(t) : '') + '</div>';
    },

    _gnTally: function (n) {
      var t = n.tally || {};
      return (t.going || 0) + ' going · ' + (t.maybe || 0) + ' maybe' + (t.cant ? ' · ' + t.cant + ' can’t' : '') + ' · ' + (t.noAnswer || 0) + ' no answer yet';
    },

    _gnAnswerWord: function (a) {
      if (a.excluded) return 'Not counted';
      if (a.carried) return 'Said yes to the time, not confirmed';
      return { yes: 'Going', maybe: 'Maybe', no: 'Can’t make it' }[a.answer] || 'No answer yet';
    },

    _gnRosterHTML: function (n) {
      var self = this;
      return '<ul class="roster">' + (n.roster || []).map(function (a) {
        var me = n.mine && n.mine.userId === a.userId;
        return '<li><span class="av" aria-hidden="true">' + esc((a.name || '?').charAt(0)) + '</span>' +
          '<span class="who">' + esc(a.name || 'A member') + (me ? ' (you)' : '') + (a.userId === n.organizerId ? ' · running it' : '') +
          (a.note ? '<span class="rn2">“' + esc(a.note) + '”</span>' : '') + '</span>' +
          '<span class="st' + (a.answer === 'yes' && !a.excluded ? ' yes' : '') + '">' + esc(self._gnAnswerWord(a)) + (a.recheck ? ' · to check' : '') + '</span></li>';
      }).join('') + '</ul>';
    },

    // Who answered what, as name chips: the day's card for whoever has no
    // answer of their own to give, and the full planner's list of nights.
    _gnChipsHTML: function (n) {
      var who = (n.roster || []).filter(function (a) { return !a.excluded; });
      if (!who.length) return '';
      return '<div class="plwho">' + who.map(function (a) {
        var k = a.carried ? 'q' : a.answer === 'yes' ? 'y' : a.answer === 'maybe' ? 'm' : a.answer === 'no' ? 'n' : 'q';
        return '<span class="plw ' + k + '" title="' + esc((a.name || 'A member') + ': ' + { y: 'going', m: 'maybe', n: 'can’t make it', q: 'no answer yet' }[k]) + '">' + esc(a.name || 'A member') + '</span>';
      }).join('') + '</div>';
    },

    // The answer block. The same markup serves the day's card (full=false:
    // buttons, note and count) and the night's page (full=true: plus who
    // answered what), so an answer looks and works the same in both.
    _gnRsvpHTML: function (n, full) {
      var id = this._gnId(n), mine = n.mine || {}, a = mine.carried ? '' : mine.answer, h = '<div class="rsvp" data-gnr="' + esc(id) + '">';
      if (!n.mine) return '<div class="rsvp glance" data-gnr="' + esc(id) + '"><div class="rcount">' + esc(this._gnTally(n)) + '</div>' + this._gnChipsHTML(n) + '</div>';
      if (mine.recheck && a) h += '<div class="rnote">The time changed after you answered. Is your answer still right?</div>';
      if (mine.carried) h += '<div class="rnote">You said yes to this time when it was proposed. Are you coming?</div>';
      h += '<div class="rq">' + (n.organizerId === mine.userId ? 'Are you playing?' : 'Are you coming?') + '</div>';
      h += '<div class="seg3" role="group" aria-label="' + esc('Your answer for ' + n.name + ', ' + n.date) + '">' + ['yes', 'maybe', 'no'].map(function (x) {
        return '<button type="button" data-ans="' + x + '" aria-pressed="' + (a === x) + '">' + { yes: 'Going', maybe: 'Maybe', no: 'Can’t' }[x] + '</button>';
      }).join('') + '</div>';
      if (this._gnNoteFor === id) {
        h += '<div class="gnnoteed"><label class="sr" for="gn-note-input">Your note for the table</label>' +
          '<input id="gn-note-input" data-gn-note-input maxlength="140" autocomplete="off" placeholder="Like “Might be late”" value="' + esc(mine.note || '') + '">' +
          '<button type="button" class="btn sm primary" data-gn-note="save">Save</button><button type="button" class="btn sm quiet" data-gn-note="cancel">Cancel</button></div>';
      } else if (mine.note) {
        h += '<div class="gnnote"><q>' + esc(mine.note) + '</q><button type="button" class="lnk" data-gn-note="edit">Edit note</button><button type="button" class="lnk" data-gn-note="clear">Clear note</button></div>';
      } else if (a) {
        h += '<div class="gnnote"><button type="button" class="lnk" data-gn-note="edit">Add a note</button></div>';
      }
      h += '<div class="rcount">' + esc(this._gnTally(n)) + '</div>';
      if (!full) return h + '</div>';
      h += this._gnRosterHTML(n) + '<div class="rnote">A missing answer is never counted as a no.</div>';
      if (n.canExclude) {
        h += '<div class="sw"><span>Count me in the answers<small>Off leaves you out of the numbers, as the one running it.</small></span>' +
          '<button type="button" class="tog" role="switch" aria-checked="' + !mine.excluded + '" aria-label="Count me in the answers" data-gn-count></button></div>';
      }
      return h + '</div>';
    },

    _gnBoxHTML: function (n, highlightId) {
      var id = this._gnId(n), hl = highlightId === id ? ' hl' : '', time = this._anchor ? '' : this._gnTime(n);
      var h = '<div class="evd go gnb' + hl + '" data-ev="' + esc(id) + '">' +
        '<div class="evh"><span class="ric"><i class="fa-solid fa-dice-d20"></i></span><span class="qt0">' + esc(n.name) + '</span></div>' +
        this._gnRealLine(n) +
        '<div class="evm">' + (n.past ? '<span class="mi">Played</span>' : '') + (time ? '<span class="mi">' + esc(time) + '</span>' : '') +
          (n.recurring ? '<span class="mi"><i class="fa-solid fa-rotate"></i> Repeats</span>' : '') + '</div>' +
        (n.past ? '' : this._gnZoneHTML(n));
      if (n.summary) h += '<p class="brief">' + esc(n.summary) + '</p>';
      if (!n.past) h += this._gnRsvpHTML(n, false);
      else h += '<div class="rsvp"><div class="rcount">' + esc((n.tally && n.tally.going) || 0) + ' said they’d come</div></div>';
      return h + '</div>';
    },

    _gnPageHTML: function (n) {
      var time = this._gnTime(n);
      var label = (this._anchor ? 'Real date: ' : '') + realDateWords(n.date) + (time ? ' · ' + time : '');
      return '<div class="grab" aria-hidden="true"></div>' +
        '<div class="ein"><div class="crease"><button type="button" class="back" data-close><i class="fa-solid fa-arrow-left"></i><span>Day</span></button>' +
          (this.role >= 2 ? '<button type="button" class="btn sm" data-gn-edit><i class="fa-solid fa-pen"></i> Edit</button>' : '') + '</div>' +
        '<div class="epb">' +
          '<h3 class="ept">' + esc(n.name) + ' <span class="dirnote">' + (n.past ? 'Played' : 'Game night') + '</span></h3>' +
          '<div class="espan">' + esc(label) + (n.recurring ? ' · repeats' : '') + '</div>' +
          (n.past ? '' : this._gnZoneHTML(n)) +
          (n.tz && time ? '<div class="espan">Set by ' + esc(n.organizerName || 'the organiser') + ' in ' + esc(n.tz.replace(/_/g, ' ')) + ' time.</div>' : '') +
          (n.summary ? '<div class="notes"><p>' + esc(n.summary) + '</p></div>' : '') +
          '<section class="eps"><h4>Who’s coming</h4>' + (n.past ? this._gnRosterHTML(n) : this._gnRsvpHTML(n, true)) + '</section>' +
          this._gnPageExtraHTML(n) +
        '</div></div>';
    },

    // A night's recap and linked pages sit below its answers. They are read
    // when the page opens; until then the page says so.
    _gnPageExtraHTML: function (n) {
      var sid = n.sessionId;
      if (!this._gnPages[sid]) this._gnLoadPage(sid);
      return '<div data-gn-extra="' + esc(sid) + '">' + this._gnExtraHTML(sid) + '</div>';
    },

    _gnLoadPage: function (sid) {
      var self = this;
      return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/sessions/' + encodeURIComponent(sid) + '/page')
        .then(function (resp) { return resp.ok ? resp.json() : Promise.reject(); })
        .then(function (p) {
          self._gnPages[sid] = { recap: p.recap || '', recapHtml: p.recapHtml || '', links: Array.isArray(p.links) ? p.links : [] };
        })
        .catch(function () { self._gnPages[sid] = { failed: true, recap: '', recapHtml: '', links: [] }; })
        .then(function () { self._gnRedrawExtra(sid); });
    },

    _gnRedrawExtra: function (sid, focusSel) {
      var box = this.evpEl.classList.contains('open') && this.evpEl.querySelector('[data-gn-extra="' + sid + '"]');
      if (!box) return;
      box.innerHTML = this._gnExtraHTML(sid);
      var f = focusSel && box.querySelector(focusSel);
      if (f) f.focus({ preventScroll: true });
    },

    _gnExtraHTML: function (sid) {
      var p = this._gnPages[sid], scribe = this.role >= 2, cid = encodeURIComponent(this.campaignId);
      if (!p) return '<section class="eps"><h4>Recap</h4><div class="none">Loading…</div></section>';
      if (p.failed) return '<section class="eps"><h4>Recap</h4><div class="none">The recap and linked pages could not be loaded.</div></section>';
      var h = '<section class="eps gnrecap"><h4>Recap</h4>';
      // Editing here is plain text, so a recap with formatting is shown but
      // never edited here: saving it would drop the formatting.
      var rich = !!p.recapHtml && /<(?!\/?p\b|br\b)[a-z]/i.test(p.recapHtml);
      if (this._gnRecapFor === sid && !rich) {
        h += '<label class="fld"><span class="sr">Recap</span><textarea data-gn-recap-input rows="6" placeholder="What happened this game night?">' + esc(p.recap) + '</textarea></label>' +
          '<p class="gperr" role="alert" hidden></p>' +
          '<div class="gpa"><button type="button" class="btn sm primary" data-gn-recap="save">Save the recap</button><button type="button" class="btn sm quiet" data-gn-recap="cancel">Cancel</button></div>';
      } else {
        // The server sanitizes recap HTML when it is saved.
        h += p.recapHtml ? '<div class="notes gnrtext">' + p.recapHtml + '</div>' : '<div class="none">No recap yet.</div>';
        if (scribe && rich) h += '<a class="lnk" href="/campaigns/' + cid + '/sessions/' + encodeURIComponent(sid) + '"><i class="fa-solid fa-pen"></i> Edit on the page</a>';
        else if (scribe) h += '<button type="button" class="lnk" data-gn-recap="edit"><i class="fa-solid fa-pen"></i> ' + (p.recapHtml ? 'Edit the recap' : 'Write the recap') + '</button>';
      }
      h += '</section><section class="eps gnlinks"><h4>Linked pages</h4>';
      h += p.links.length ? '<div class="gnchips">' + p.links.map(function (l) {
        return '<span class="gnchip"><a href="/campaigns/' + cid + '/entities/' + encodeURIComponent(l.slug) + '">' + esc(l.name) + '</a>' +
          (scribe ? '<button type="button" data-gn-unlink="' + esc(l.entityId) + '" aria-label="Unlink ' + esc(l.name) + '"><i class="fa-solid fa-xmark"></i></button>' : '') + '</span>';
      }).join('') + '</div>' : '<div class="none">No pages linked yet.</div>';
      if (scribe) {
        h += this._gnLinkFor === sid
          ? '<div class="gnfind"><input type="search" data-gn-link-q placeholder="Search pages to link" aria-label="Search pages to link" autocomplete="off"><ul class="lp-res" data-gn-link-res role="listbox" aria-label="Pages" hidden></ul></div>'
          : '<button type="button" class="lnk" data-gn-link-open><i class="fa-solid fa-plus"></i> Link a page</button>';
      }
      return h + '</section>';
    },

    _gnLinkSearch: function (input) {
      var self = this, q = input.value.trim(), res = input.parentNode.querySelector('[data-gn-link-res]'), seq = (this._gnLinkSeq || 0) + 1;
      this._gnLinkSeq = seq;
      if (q.length < 2) { res.hidden = true; res.innerHTML = ''; return; }
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/entities/search?q=' + encodeURIComponent(q), { headers: { Accept: 'application/json' } })
        .then(function (resp) { return resp.ok ? resp.json() : { results: [] }; })
        .then(function (body) {
          if (self._gnLinkSeq !== seq) return;
          // Only pages: the search also returns other kinds of result.
          var pages = (body.results || []).filter(function (r) { return r.id && /\/entities\//.test(r.url || ''); }).slice(0, 8);
          res.innerHTML = pages.length
            ? pages.map(function (r) { return '<li><button type="button" role="option" data-gn-link-id="' + esc(r.id) + '">' + esc(r.name) + (r.type_name ? '<small>' + esc(r.type_name) + '</small>' : '') + '</button></li>'; }).join('')
            : '<li class="none">No pages match.</li>';
          res.hidden = false;
        });
    },

    // Recap and link presses on a night's page. Returns true when handled.
    _gnExtraClick: function (e) {
      var self = this, box = e.target.closest('[data-gn-extra]');
      if (!box) return false;
      var sid = box.dataset.gnExtra, base = '/campaigns/' + encodeURIComponent(this.campaignId) + '/sessions/' + encodeURIComponent(sid);
      var t = e.target.closest('button');
      if (!t) return true;
      var reload = function (msg) { self.say(msg); return self._gnLoadPage(sid); };
      var fail = function (msg) {
        var err = box.querySelector('.gperr');
        if (err) { err.textContent = msg; err.hidden = false; } else self.say(msg);
        $$('button, textarea, input', box).forEach(function (c) { c.disabled = false; });
      };
      var act = t.dataset.gnRecap;
      if (act === 'edit') { this._gnRecapFor = sid; this._gnRedrawExtra(sid, '[data-gn-recap-input]'); }
      else if (act === 'cancel') { this._gnRecapFor = null; this._gnRedrawExtra(sid, '[data-gn-recap="edit"]'); }
      else if (act === 'save') {
        var text = box.querySelector('[data-gn-recap-input]').value;
        // The same shape the Sessions page saves: the text, and each line as
        // a paragraph.
        var html = text.split('\n').filter(function (l) { return l.trim(); }).map(function (l) { return '<p>' + esc(l) + '</p>'; }).join('');
        $$('button, textarea', box).forEach(function (c) { c.disabled = true; });
        Chronicle.apiFetch(base + '/recap', { method: 'PUT', body: { recap: text, recap_html: html } })
          .then(function (resp) {
            if (!resp.ok) return Promise.reject();
            self._gnRecapFor = null;
            return reload('Recap saved.');
          })
          .catch(function () { fail('The recap didn’t save. Try again.'); });
      } else if (t.hasAttribute('data-gn-link-open')) {
        this._gnLinkFor = sid; this._gnRedrawExtra(sid, '[data-gn-link-q]');
      } else if (t.dataset.gnLinkId) {
        $$('button, input', box).forEach(function (c) { c.disabled = true; });
        Chronicle.apiFetch(base + '/entities', { method: 'POST', body: { entity_id: t.dataset.gnLinkId, role: 'mentioned' } })
          .then(function (resp) {
            if (!resp.ok) return Promise.reject();
            self._gnLinkFor = null;
            return reload('Page linked.');
          })
          .catch(function () { fail('That page could not be linked. Try again.'); });
      } else if (t.dataset.gnUnlink) {
        $$('button, input', box).forEach(function (c) { c.disabled = true; });
        Chronicle.apiFetch(base + '/entities/' + encodeURIComponent(t.dataset.gnUnlink), { method: 'DELETE' })
          .then(function (resp) { return resp.ok ? reload('Page unlinked.') : Promise.reject(); })
          .catch(function () { fail('That page could not be unlinked. Try again.'); });
      }
      return true;
    },

    // Redraws every visible copy of a night's answer block (day card and
    // page) from the current data, keeping keyboard focus on the button
    // that was pressed.
    _gnRefresh: function (id, focusSel) {
      var n = this._findNight(id);
      if (!n) { this.refreshWing(); return; }
      var box = this.wingEl.querySelector('.evd[data-ev="' + id + '"]');
      if (box) {
        var tmp = document.createElement('div');
        tmp.innerHTML = this._gnBoxHTML(n, box.classList.contains('hl') ? id : null);
        box.replaceWith(tmp.firstChild);
      }
      if (this.evpEl.classList.contains('open') && this.evpEl.dataset.ev === id) {
        var old = this.evpEl.querySelector('.rsvp[data-gnr]');
        if (old) {
          var t2 = document.createElement('div');
          t2.innerHTML = this._gnRsvpHTML(n, true);
          old.replaceWith(t2.firstChild);
        }
      }
      if (focusSel) {
        var scope = this.evpEl.classList.contains('open') ? this.evpEl : this.wingEl;
        var f = scope.querySelector('.rsvp[data-gnr="' + id + '"] ' + focusSel);
        if (f) f.focus({ preventScroll: true });
      }
    },

    // Applies an answer locally (so the press answers at once), saves it,
    // then reloads the month's nights so counts match the server.
    _gnSave: function (id, change) {
      var self = this, n = this._findNight(id);
      if (!n || !n.mine) return;
      var mine = n.mine, body = {};
      var status = { yes: 'accepted', maybe: 'tentative', no: 'declined' };
      if (change.answer) { mine.answer = change.answer; mine.carried = false; mine.recheck = false; }
      if (!mine.answer) return;
      body.status = status[mine.answer];
      if ('note' in change) { mine.note = change.note; body.note = change.note === '' ? null : change.note; }
      if (n.recurring) body.occurrenceDate = n.date;
      (n.roster || []).forEach(function (a) { if (a.userId === mine.userId) { a.answer = mine.answer; a.note = mine.note; a.carried = false; a.recheck = false; } });
      this._gnRetally(n);
      this._gnRefresh(id, change.answer ? '[data-ans="' + change.answer + '"]' : null);
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/sessions/' + encodeURIComponent(n.sessionId) + '/rsvp', { method: 'POST', body: body })
        .then(function (resp) {
          self._announce(resp.ok ? 'Answer saved.' : 'Your answer could not be saved.');
          if (!resp.ok) self._toast('Your answer could not be saved. Try again.');
          return self.fetchNights(self.view.y, self.view.m, { fresh: true });
        })
        // A note being typed meanwhile is left alone; its own save redraws.
        .then(function () { if (self._gnNoteFor !== id) self._gnRefresh(id); self._paintMonth(); })
        .catch(function () { self._toast('Your answer could not be saved. Try again.'); });
    },

    _gnSetCounted: function (id, counted) {
      var self = this, n = this._findNight(id);
      if (!n || !n.mine || !n.canExclude) return;
      n.mine.excluded = !counted;
      (n.roster || []).forEach(function (a) { if (a.userId === n.mine.userId) a.excluded = !counted; });
      this._gnRetally(n);
      this._gnRefresh(id, '[data-gn-count]');
      var body = { excluded: !counted };
      if (n.recurring) body.occurrenceDate = n.date;
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/sessions/' + encodeURIComponent(n.sessionId) + '/rsvp-exclude', { method: 'PUT', body: body })
        .then(function (resp) {
          if (!resp.ok) self._toast('That could not be saved. Try again.');
          return self.fetchNights(self.view.y, self.view.m, { fresh: true });
        })
        .then(function () { self._gnRefresh(id); })
        .catch(function () { self._toast('That could not be saved. Try again.'); });
    },

    _gnRetally: function (n) {
      var t = { going: 0, maybe: 0, cant: 0, noAnswer: 0 };
      (n.roster || []).forEach(function (a) {
        if (a.excluded) return;
        if (a.carried || !a.answer) t.noAnswer++;
        else if (a.answer === 'yes') t.going++;
        else if (a.answer === 'maybe') t.maybe++;
        else t.cant++;
      });
      n.tally = t;
    },

    // Handles a press inside a night's answer block, in the day's card or
    // on its page. Returns true when the press was one of these controls.
    _gnHandleClick: function (e) {
      var zone = e.target.closest('[data-gn-zone]');
      if (zone) { this._gnSetZoneMode(zone.dataset.gnZone); return true; }
      var box = e.target.closest('.rsvp[data-gnr]');
      if (!box) return false;
      var id = box.dataset.gnr;
      var ans = e.target.closest('[data-ans]');
      var note = e.target.closest('[data-gn-note]');
      var count = e.target.closest('[data-gn-count]');
      if (ans) this._gnSave(id, { answer: ans.dataset.ans });
      else if (count) this._gnSetCounted(id, count.getAttribute('aria-checked') !== 'true');
      else if (note) {
        var act = note.dataset.gnNote;
        if (act === 'edit') { this._gnNoteFor = id; this._gnRefresh(id, '[data-gn-note-input]'); }
        else if (act === 'cancel') { this._gnNoteFor = null; this._gnRefresh(id); }
        else if (act === 'clear') { this._gnNoteFor = null; this._gnSave(id, { note: '' }); }
        else if (act === 'save') {
          var input = box.querySelector('[data-gn-note-input]');
          this._gnNoteFor = null;
          this._gnSave(id, { note: input ? input.value.trim().slice(0, 140) : '' });
        }
      }
      // Any press inside the block stays inside it: it never opens the page.
      return true;
    },

    // --------------------------------------------------------------
    // Who's free: the hours members painted (the sessions plugin's
    // availability overlay, read one week at a time in the calendar's
    // zone). The Director and co-Directors see one line per player in each
    // day, green where that player is free, with a soft band where everyone
    // is; hovering names them, and a "Best times" box suggests slots.
    // Others see only counts, in a day's card: the overlay itself leaves
    // names out for them.
    // --------------------------------------------------------------
    setShowFree: function (on) {
      var self = this;
      this.showFree = !!on && this.freeDirector;
      try { window.localStorage.setItem(FREE_KEY, this.showFree ? '1' : '0'); } catch (e) { /* this page only */ }
      var go = this.showFree ? this.fetchFreeMonth(this.view.y, this.view.m) : Promise.resolve();
      go.then(function () { self._paintMonth(); self.refreshFreeView(); self.refreshWing(); });
      this._announce(this.showFree ? 'Showing when players are free.' : 'Hiding when players are free.');
    },

    // fetchFreeWeek reads one week (Monday first) and files it by date. A
    // failed read files nothing and is not retried until the page reloads,
    // so a card never asks again in a loop.
    fetchFreeWeek: function (monday) {
      var self = this;
      if (this._freeWeeks[monday]) return this._freeWeeks[monday];
      var url = '/campaigns/' + encodeURIComponent(this.campaignId) + '/availability/overlay?week=' + monday +
        (this.calZone ? '&tz=' + encodeURIComponent(this.calZone) : '');
      this._freeWeeks[monday] = Chronicle.apiFetch(url)
        .then(function (resp) { return resp.ok ? resp.json() : null; })
        .then(function (ov) { if (ov) self._fileFreeWeek(ov); })
        .catch(function () { /* the calendar shows without it */ });
      return this._freeWeeks[monday];
    },

    _fileFreeWeek: function (ov) {
      var self = this, members = Array.isArray(ov.members) ? ov.members : [];
      (ov.days || []).forEach(function (day, i) {
        self.freeByDate[day.date] = {
          total: ov.totalMembers || 0,
          detail: !!ov.includeDetail,
          hours: (day.hours || []).map(function (h) { return h.free || 0; }),
          members: members.map(function (m) {
            return {
              userId: m.userId, name: m.name || 'A player', answered: !!m.hasAnswered,
              // [start, end, best]: a run the player marked as their best
              // time keeps that mark, so the lines can show it apart.
              segs: (m.lanes || []).filter(function (l) { return l.day === i; }).map(function (l) { return [l.start, l.end, l.state === 'preferred']; }),
              // Marked this day off although their usual hours cover it.
              off: (m.offDays || []).indexOf(i) >= 0
            };
          })
        };
      });
    },

    // The weeks a month's days fall in, Monday first.
    _freeMondays: function (y, m) {
      var first = this._realIso(y, m, 1), last = this._realIso(y, m, CalDate.monthDays(this.cal, m - 1, y)), out = [];
      if (!first || !last) return out;
      for (var w = isoMonday(first); w <= last && out.length < 7; w = isoAddDays(w, 7)) out.push(w);
      return out;
    },

    fetchFreeMonth: function (y, m) {
      if (!this.freeCan) return Promise.resolve();
      return Promise.all(this._freeMondays(y, m).map(this.fetchFreeWeek, this));
    },

    _freeOn: function (y, m, d) {
      var iso = this._realIso(y, m, d);
      return iso ? this.freeByDate[iso] || null : null;
    },

    // Runs of hours (start, end hour) where at least k are free.
    _freeRuns: function (hours, k) {
      var runs = [], start = -1;
      for (var h = 0; h <= 24; h++) {
        var ok = h < 24 && k > 0 && (hours[h] || 0) >= k;
        if (ok && start < 0) start = h;
        if (!ok && start >= 0) { runs.push([start, h]); start = -1; }
      }
      return runs;
    },

    // The day's strongest window at least minLen hours long: the most
    // players free, then the longest run. null when nobody is free that long.
    _bestWindow: function (data, minLen) {
      for (var k = data.total; k > 0; k--) {
        var best = null;
        this._freeRuns(data.hours, k).forEach(function (r) {
          if (r[1] - r[0] >= minLen && (!best || r[1] - r[0] > best[1] - best[0])) best = r;
        });
        if (best) return { start: best[0], end: best[1], free: k };
      }
      return null;
    },

    // Who is not free for the whole window, top-of-hour like the server.
    _freeMissing: function (data, w) {
      return data.members.filter(function (mem) {
        for (var h = w.start; h < w.end; h++) {
          var top = h * 60;
          if (!mem.segs.some(function (sg) { return sg[0] <= top && top < sg[1]; })) return true;
        }
        return false;
      });
    },

    // off: the player marked this day off although they are usually free,
    // drawn as a broken line so the DM sees the change, not a blank.
    _lineHTML: function (segs, off) {
      if (off) return '<span class="ln off"></span>';
      return '<span class="ln">' + segs.map(function (sg) {
        return '<b' + (sg[2] ? ' class="p"' : '') + ' style="left:' + (sg[0] / 14.4).toFixed(2) + '%;width:' + ((sg[1] - sg[0]) / 14.4).toFixed(2) + '%"></b>';
      }).join('') + '</span>';
    },

    // The band is "everyone who has painted hours is free": a player who
    // never answered would otherwise hide it on every day of the month.
    _bandHTML: function (data, cls) {
      var k = data.detail ? data.members.filter(function (mem) { return mem.answered; }).length : data.total;
      if (!k) return '';
      return this._freeRuns(data.hours, k).map(function (r) {
        return '<span class="' + cls + '" style="left:' + (r[0] / 0.24).toFixed(2) + '%;width:' + ((r[1] - r[0]) / 0.24).toFixed(2) + '%"></span>';
      }).join('');
    },

    // The lines in a day of the month: the same players in the same order
    // every day, so the rows are learnt once.
    _freeCellHTML: function (y, m, d) {
      if (!this.showFree) return '';
      var data = this._freeOn(y, m, d);
      if (!data || !data.detail || !data.members.length) return '';
      var self = this, rows = data.members.slice(0, 12);
      return '<span class="avl' + (rows.length > 8 ? ' many' : '') + '" data-avl="' + esc(this._realIso(y, m, d)) + '" aria-hidden="true">' +
        this._bandHTML(data, 'fband') + rows.map(function (mem) { return self._lineHTML(mem.segs, mem.off); }).join('') + '</span>';
    },

    _segsWords: function (mem) {
      if (mem.off) return 'off this day, usually free';
      if (!mem.segs.length) return mem.answered ? 'not free' : 'hasn’t painted hours yet';
      if (mem.segs.length === 1 && mem.segs[0][0] === 0 && mem.segs[0][1] >= 1440) return 'free all day';
      return mem.segs.map(function (sg) { return ampm(sg[0]) + ' to ' + ampm(sg[1]) + (sg[2] ? ' (best)' : ''); }).join(', ');
    },

    _freeGlanceHTML: function (iso) {
      var data = this.freeByDate[iso];
      if (!data || !data.detail) return '';
      var self = this;
      return '<div class="gh"><b>' + esc(realDateWords(iso)) + '</b></div>' +
        '<ul class="gfree">' + data.members.map(function (mem) {
          return '<li><span>' + esc(mem.name) + '</span><span>' + esc(self._segsWords(mem)) + '</span></li>';
        }).join('') + '</ul>';
    },

    _windowWords: function (w, iso) {
      var z = zoneAbbr(this.calZone, iso);
      return ampm(w.start * 60) + ' to ' + ampm(w.end * 60) + (z ? ' ' + z : '');
    },

    // "Who's free" in a day's card. The Director sees the lines large, with
    // names; everyone else sees the count and a link to their own hours.
    _freeWingHTML: function (d) {
      if (!this.freeCan) return '';
      var self = this, iso = this._realIso(d.y, d.m, d.d), data = iso ? this.freeByDate[iso] : null;
      if (!data) {
        // Read the week, then redraw this card if it is still open on it.
        var key = dayKey(d.y, d.m, d.d);
        if (iso) this.fetchFreeWeek(isoMonday(iso)).then(function () { if (self.wingFor === key && self.freeByDate[iso]) self._refreshWingOnceOpen(); });
        return '';
      }
      var mine = this._myDayHTML(iso) + '<button type="button" class="lnk" data-pl-mine>Change my usual hours</button>';
      var h = '<div class="sect">Who’s free</div><div class="free">';
      if (!data.total) return h + '<div class="none">Nobody in the campaign yet.</div></div>';
      var all = this._freeRuns(data.hours, data.total)[0], w = all ? { start: all[0], end: all[1], free: data.total } : this._bestWindow(data, 1);
      var summary = !w ? 'Nobody is free this day.'
        : (w.free === data.total ? 'Everyone free ' : w.free + ' of ' + data.total + ' free ') + this._windowWords(w, iso);
      h += '<div class="fsum">' + esc(summary) + '</div>';
      if (data.detail && data.members.length) {
        h += '<div class="flines"><span class="bandwrap" aria-hidden="true">' + this._bandHTML(data, 'band2') + '</span>' + data.members.map(function (mem) {
          return '<div class="fr"><span class="nm">' + esc(mem.name) + '</span>' + self._lineHTML(mem.segs, mem.off) +
            '<span class="sr">' + esc(mem.name + ': ' + self._segsWords(mem)) + '</span></div>';
        }).join('') + '</div><div class="fticks" aria-hidden="true"><span>12am</span><span>6am</span><span>noon</span><span>6pm</span><span>12am</span></div>' + freeLegend(data.members);
      }
      if (w && this.role >= 2 && iso >= this._todayIso() && !this.nightsOnDay(d.y, d.m, d.d).length) {
        h += this._planFor && this._planFor.iso === iso ? this._planFormHTML(iso, this._planFor.start)
          : '<button type="button" class="btn sm" data-plan="' + esc(iso) + '" data-plan-at="' + w.start + '"><i class="fa-solid fa-dice-d20"></i> Plan a game night at ' + esc(ampm(w.start * 60)) + '</button>';
      }
      return h + '<div class="fmine">' + mine + '</div></div>';
    },

    // Planning a game night, inside the day's card: a name, the start (in
    // the calendar's zone) and whether it repeats weekly. "More options"
    // carries what is filled in over to the game night editor.
    _planFormHTML: function (iso, startHour) {
      var z = zoneAbbr(this.calZone, iso);
      return '<form class="gnplan" data-plan-form="' + esc(iso) + '">' +
        '<div class="gph"><b>Plan a game night</b><span>' + esc(realDateWords(iso)) + '</span></div>' +
        '<label class="fld"><span>Name</span><input type="text" name="name" value="Game night" maxlength="200" required></label>' +
        '<div class="gprow"><label class="fld"><span>Starts' + (z ? ' (' + esc(z) + ')' : '') + '</span><input type="time" name="time" value="' + pad2(startHour) + ':00" required></label>' +
        '<label class="fld"><span>Repeats</span><select name="repeat"><option value="">Just this day</option><option value="weekly">Every week</option><option value="biweekly">Every 2 weeks</option></select></label></div>' +
        '<p class="gpn">Everyone in the campaign is asked if they can come.</p>' +
        '<div class="gpa"><button type="submit" class="btn sm primary">Plan it</button><button type="button" class="btn sm quiet" data-plan-cancel>Cancel</button>' +
        '<button type="button" class="lnk" data-plan-more>More options</button></div>' +
        '<p class="gperr" role="alert" hidden></p></form>';
    },

    // What a planned night sends: the New Session form's own fields, so the
    // server path is the one the Sessions page already uses.
    _planFields: function (iso, f) {
      var out = { name: (f.name || '').trim(), scheduled_date: iso, scheduled_time: f.time || '' };
      if (this.calZone && out.scheduled_time) out.scheduled_tz = this.calZone;
      if (f.repeat === 'weekly' || f.repeat === 'biweekly') { out.is_recurring = '1'; out.recurrence_type = f.repeat; }
      return out;
    },

    _planMore: function (form) {
      if (!form) return;
      var preset = { name: form.elements.name.value, time: form.elements.time.value, repeat: form.elements.repeat.value };
      this._planFor = null;
      this._gnOpenEditor(null, form.dataset.planForm, preset);
    },

    _openPlan: function (iso, startHour) {
      this._planFor = { iso: iso, start: startHour };
      this.refreshWing();
      var name = this.wingEl.querySelector('.gnplan input[name="name"]');
      if (name) { name.focus({ preventScroll: true }); name.select(); name.closest('.gnplan').scrollIntoView({ block: 'nearest' }); }
    },

    _closePlan: function () {
      this._planFor = null;
      var f = this.wingEl.querySelector('.gnplan');
      if (f) f.remove();
      this.refreshWing();
    },

    _submitPlan: function (form) {
      var self = this, iso = form.dataset.planForm, err = form.querySelector('.gperr');
      var fields = this._planFields(iso, { name: form.elements.name.value, time: form.elements.time.value, repeat: form.elements.repeat.value });
      if (!fields.name) { err.textContent = 'Give the game night a name.'; err.hidden = false; return; }
      var body = new FormData();
      Object.keys(fields).forEach(function (k) { body.append(k, fields[k]); });
      $$('button, input, select', form).forEach(function (c) { c.disabled = true; });
      // HX-Request asks the server for its no-redirect answer (204), so the
      // calendar stays where it is.
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/sessions', { method: 'POST', body: body, headers: { 'HX-Request': 'true' } })
        .then(function (resp) {
          if (!resp.ok) return resp.json().catch(function () { return {}; }).then(function (j) { return Promise.reject(j && j.error); });
          self._planFor = null;
          form.remove();
          self.say('Game night planned. Everyone has been asked if they can come.');
          return self.fetchNights(self.view.y, self.view.m, { fresh: true }).then(function () { self._paintMonth(); self.refreshWing(); });
        })
        .catch(function (msg) {
          $$('button, input, select', form).forEach(function (c) { c.disabled = false; });
          err.textContent = (typeof msg === 'string' && msg) || 'That game night could not be planned. Try again.';
          err.hidden = false;
        });
    },

    // Opens the game night editor drawer: night to change it, or null and
    // the day for a new one.
    _gnOpenEditor: function (night, iso, preset) {
      if (!window.Chronicle || !Chronicle.calendarGameNight) return;
      if (!night && !iso) return;
      Chronicle.calendarGameNight.open(this, night, iso, preset);
    },

    // "Plan a game night" in a day's card, for the Director team, on any
    // day from today on, whoever is free.
    _gnAddHTML: function (d) {
      if (!this.showNights || this.role < 2) return '';
      var iso = this._realIso(d.y, d.m, d.d);
      if (!iso || iso < (this._todayIso() || localIso(new Date()))) return '';
      return '<button type="button" class="addev" data-gn-new="' + esc(iso) + '"><i class="fa-solid fa-dice-d20"></i>Plan a game night</button>';
    },

    // reloadNights re-reads game nights after one is planned, changed or
    // cancelled. A change to a series reaches every month, so every month's
    // copy is dropped. On a real-world calendar the day card of iso (the
    // night's day) opens, so the result is in view.
    reloadNights: function (iso) {
      var self = this, openId = this.evpEl.classList.contains('open') ? this.evpEl.dataset.ev : '';
      this.nightsByMonth = {};
      this._gnPages = {};
      var target = null;
      if (iso && /^\d{4}-\d{2}-\d{2}$/.test(iso) && CalDate.usesRealTime(this.cal)) {
        var p = iso.split('-');
        target = { y: +p[0], m: +p[1], d: +p[2] };
        if (target.y !== this.view.y || target.m !== this.view.m) { this.view = { y: target.y, m: target.m }; this.renderMonth(); }
      }
      return this.fetchNights(this.view.y, this.view.m, { fresh: true }).then(function () {
        self._paintMonth();
        if (target && !self.anyPanelOpen()) { self.openWing(dayKey(target.y, target.m, target.d)); return; }
        if (self.wingFor) self.refreshWing();
        if (openId) {
          var n = self._findNight(openId);
          if (n) self.evpEl.innerHTML = self._gnPageHTML(n); else self.closeEventDetail();
        }
      });
    },

    // The real today: the real-world calendar's own date, or on a world
    // calendar the real day its today stands for.
    _todayIso: function () {
      var c = this.cal;
      if (CalDate.usesRealTime(c)) return c.current_year + '-' + pad2(c.current_month) + '-' + pad2(c.current_day);
      return this._anchor ? new Date(this._anchor.ms).toISOString().slice(0, 10) : '';
    },

    // Every day left in the month with a window of at least three hours,
    // each with its strongest window.
    _bestCandidates: function (y, m) {
      var out = [], today = this._todayIso(), last = CalDate.monthDays(this.cal, m - 1, y);
      for (var d = 1; d <= last; d++) {
        var iso = this._realIso(y, m, d), data = iso && this.freeByDate[iso];
        if (!data || !data.detail || iso < today) continue;
        var w = this._bestWindow(data, 3);
        if (w) out.push({ y: y, m: m, d: d, iso: iso, w: w, data: data, wd: new Date(iso + 'T12:00:00Z').getUTCDay() });
      }
      return out;
    },

    // How strong each weekday is across the month: the players free in its
    // best window, averaged over all of that weekday's days left, so a day
    // that is strong every week outranks one strong date among weak ones.
    _weekdayStrength: function (cands) {
      var sum = {}, n = {};
      cands.forEach(function (c) { sum[c.wd] = (sum[c.wd] || 0) + c.w.free; n[c.wd] = (n[c.wd] || 0) + 1; });
      var out = {};
      Object.keys(sum).forEach(function (k) { out[k] = sum[k] / n[k]; });
      return out;
    },

    // The three strongest slots of at least three hours left in the month:
    // most players first; then the window's length, counted only up to a
    // game night's four hours, so a whole free weekday never beats an
    // evening everyone can make; then the weekday that is strongest week
    // after week; then the soonest.
    bestTimes: function (y, m) {
      var cands = this._bestCandidates(y, m), str = this._weekdayStrength(cands);
      function len(c) { return Math.min(4, c.w.end - c.w.start); }
      cands.sort(function (a, b) {
        return (b.w.free - a.w.free) || (len(b) - len(a)) || ((str[b.wd] || 0) - (str[a.wd] || 0)) || (a.iso < b.iso ? -1 : a.iso > b.iso ? 1 : 0);
      });
      return cands.slice(0, 3);
    },

    // The weekday that is best overall, when the list of dates hides it:
    // a weekday that shows on most weeks but loses some dates to someone
    // who is away (often a player free every other week). Says who is
    // missing on which dates, so the shading and the list agree. null when
    // the list already leads with it, or no weekday stands out.
    bestWeekday: function (y, m) {
      var self = this, cands = this._bestCandidates(y, m), str = this._weekdayStrength(cands), top = this.bestTimes(y, m);
      // Only a weekday seen on two or more dates can be best "overall".
      var wd = null;
      Object.keys(str).forEach(function (k) {
        if (cands.filter(function (c) { return String(c.wd) === k; }).length < 2) return;
        if (wd == null || str[k] > str[wd]) wd = k;
      });
      if (wd == null) return null;
      var days = cands.filter(function (c) { return String(c.wd) === String(wd); });
      var bestFree = Math.max.apply(null, days.map(function (c) { return c.w.free; }));
      var weak = days.filter(function (c) { return c.w.free < bestFree; });
      var listed = top.filter(function (c) { return String(c.wd) === String(wd); }).length;
      if (listed >= Math.min(2, days.length) && !weak.length) return null;
      var strong = days.filter(function (c) { return c.w.free === bestFree; })[0];
      var away = {};
      weak.forEach(function (c) {
        self._freeMissing(c.data, strong.w).forEach(function (mem) {
          if (self._freeMissing(strong.data, strong.w).some(function (x) { return x.userId === mem.userId; })) return;
          (away[mem.name] = away[mem.name] || []).push(realDateWords(c.iso).replace(/^\w+ /, ''));
        });
      });
      var name = new Date(strong.iso + 'T12:00:00Z').toLocaleDateString('en-US', { weekday: 'long', timeZone: 'UTC' });
      return {
        weekday: name, window: strong.w, iso: strong.iso,
        away: Object.keys(away).map(function (k) { return { name: k, dates: away[k] }; })
      };
    },

    // --------------------------------------------------------------
    // The Who's free view: a panel over the month, opened from the
    // header, so everything about free hours stays inside the calendar
    // wherever it is shown. The Director gets Best times, who still has
    // to give hours (with a reminder), the switch for the lines on the
    // days and the way to the full planner; a player gets their own
    // hours, with the button that gives or changes them, and how many
    // are free at each game night this month.
    // --------------------------------------------------------------
    openFreeView: function () {
      var self = this, btn = $('#cal5-freebtn', this.el);
      if (!this.freeCan || !btn) return;
      if (this.wingFor) this.closeWing({ quiet: true });
      this.fvEl.innerHTML = this._fvHTML();
      growOpen(this.fvEl, btn, this.calEl, { center: true });
      btn.setAttribute('aria-expanded', 'true');
      this._updateScrim();
      this.fvEl.focus({ preventScroll: true });
      var reads = [this.fetchFreeMonth(this.view.y, this.view.m), this.fetchAnswers()];
      if (!this.freeDirector) reads.push(this.fetchMine(), this.fetchAway());
      Promise.all(reads).then(function () { self.refreshFreeView(); });
    },

    closeFreeView: function () {
      if (!this.fvEl.classList.contains('open')) return;
      var btn = $('#cal5-freebtn', this.el);
      if (btn) btn.setAttribute('aria-expanded', 'false');
      growClose(this.fvEl, btn);
      this._updateScrim();
    },

    refreshFreeView: function () {
      if (!this.fvEl || !this.fvEl.classList.contains('open')) return;
      var body = this.fvEl.querySelector('.fvbody'), top = body ? body.scrollTop : 0;
      this.fvEl.innerHTML = this._fvHTML();
      body = this.fvEl.querySelector('.fvbody');
      if (body) body.scrollTop = top;
    },

    // The viewer's own painted hours (the Availability page's pattern),
    // read once per page; a player's view says whether they have given
    // them yet.
    fetchMine: function () {
      var self = this;
      if (this._minePromise) return this._minePromise;
      this._minePromise = Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/availability/mine')
        .then(function (resp) { return resp.ok ? resp.json() : null; })
        .then(function (mine) { self.mine = mine; })
        .catch(function () { self.mine = null; });
      return this._minePromise;
    },

    // fetchMyDays loads (once) the viewer's own one-day changes. Returns a
    // Promise.
    fetchMyDays: function (o) {
      var self = this;
      if (this._myDaysPromise && !(o && o.fresh)) return this._myDaysPromise;
      this._myDaysPromise = Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/availability/exceptions')
        .then(function (resp) { return resp.ok ? resp.json() : []; })
        .then(function (list) { self.myDays = Array.isArray(list) ? list : []; })
        .catch(function () { self.myDays = self.myDays || []; });
      return this._myDaysPromise;
    },

    // How the viewer's own hours read on one day: their usual hours, a
    // different window, or not at all. A day changed on the Availability
    // page into something more than one window reads as "changed".
    _myDayState: function (iso) {
      var rows = (this.myDays || []).filter(function (x) { return x.onDate === iso; });
      if (!rows.length) return { mode: 'usual' };
      if (rows.length === 1 && rows[0].state === 'unavailable' && rows[0].startMinute === 0 && rows[0].endMinute >= 1440) return { mode: 'cant' };
      var free = rows.filter(function (x) { return x.state !== 'unavailable'; });
      if (rows.length === 1 && free.length === 1) return { mode: 'diff', start: free[0].startMinute, end: free[0].endMinute };
      return { mode: 'diff', start: free.length ? free[0].startMinute : 18 * 60, end: free.length ? free[0].endMinute : 22 * 60, mixed: true };
    },

    // "Your hours this day" in a day's card, with the one-day change: keep
    // the usual hours, play a different window, or not play at all. The
    // change is the Availability page's own (a day's exceptions), in the
    // member's zone.
    _myDayHTML: function (iso) {
      var self = this;
      if (!iso || iso < (this._todayIso() || localIso(new Date()))) return '';
      // The member's zone comes with their hours, so both are read first.
      if (!this.myDays || this.mine === undefined) {
        Promise.all([this.fetchMyDays(), this.fetchMine()]).then(function () { if (self.wingFor) self.refreshWing(); });
        return '';
      }
      var st = this._myDayState(iso), zone = (this.mine && this.mine.tz) || browserZone() || '';
      var words = st.mode === 'usual' ? 'Your usual hours' : st.mode === 'cant' ? 'You can’t play' : (st.mixed ? 'Changed, starting ' : '') + ampm(st.start) + ' to ' + ampm(st.end % 1440);
      if (this._myDayFor !== iso) {
        return '<div class="myday"><span>This day: <b>' + esc(words) + '</b></span><button type="button" class="lnk" data-myday-edit="' + esc(iso) + '">Change this day</button></div>';
      }
      var f = this._myDayForm || st;
      var modes = [['usual', 'Usual hours'], ['diff', 'Different this day'], ['cant', 'Can’t play']];
      var hhmm = function (min) { min = min % 1440; return pad2(Math.floor(min / 60)) + ':' + pad2(min % 60); };
      // A form, so the card stays open around it (see _mvOffHandler).
      return '<form class="myday edit" data-myday="' + esc(iso) + '"><b>Your hours on ' + esc(realDateWords(iso)) + '</b>' +
        '<div class="segs" role="group" aria-label="Your hours this day">' + modes.map(function (o) {
          return '<button type="button" data-myday-mode="' + o[0] + '" aria-pressed="' + (f.mode === o[0]) + '">' + o[1] + '</button>';
        }).join('') + '</div>' +
        (f.mode === 'diff' ? '<div class="gprow"><label class="fld"><span>From</span><input type="time" step="1800" data-myday-from value="' + hhmm(f.start == null ? 18 * 60 : f.start) + '"></label>' +
          '<label class="fld"><span>To</span><input type="time" step="1800" data-myday-to value="' + hhmm(f.end == null ? 22 * 60 : f.end) + '"></label></div>' : '') +
        (zone ? '<p class="gpn">In ' + esc(zone.replace(/_/g, ' ')) + ' time. Only this day changes.</p>' : '<p class="gpn">Only this day changes.</p>') +
        '<p class="gperr" role="alert" hidden></p>' +
        '<div class="gpa"><button type="button" class="btn sm primary" data-myday-save>Save</button><button type="button" class="btn sm quiet" data-myday-cancel>Cancel</button></div></form>';
    },

    // Redraws just the one-day block: the card won't redraw around a form.
    _myDayRedraw: function (iso, focusSel) {
      var old = this.wingEl.querySelector('.myday');
      if (!old) { this.refreshWing(); return; }
      var tmp = document.createElement('div');
      tmp.innerHTML = this._myDayHTML(iso);
      var next = tmp.firstChild;
      if (next) old.replaceWith(next); else old.remove();
      var f = focusSel && next && next.querySelector(focusSel);
      if (f) f.focus({ preventScroll: true });
    },

    _myDayRead: function (box) {
      var f = this._myDayForm, from = box.querySelector('[data-myday-from]'), to = box.querySelector('[data-myday-to]');
      var mins = function (v) { var p = (v || '').split(':'); return p.length === 2 ? (+p[0]) * 60 + (+p[1]) : null; };
      if (from) f.start = mins(from.value);
      if (to) { f.end = mins(to.value); if (f.end === 0) f.end = 1440; }
    },

    // Wing presses on the one-day change. Returns true when handled.
    _myDayClick: function (e) {
      var self = this, edit = e.target.closest('[data-myday-edit]');
      if (edit) {
        this._myDayFor = edit.dataset.mydayEdit;
        var st = this._myDayState(this._myDayFor);
        this._myDayForm = { mode: st.mode, start: st.start, end: st.end };
        this._myDayRedraw(this._myDayFor, '[aria-pressed="true"]');
        return true;
      }
      var box = e.target.closest('[data-myday]');
      if (!box) return false;
      var mode = e.target.closest('[data-myday-mode]');
      if (mode) { this._myDayRead(box); this._myDayForm.mode = mode.dataset.mydayMode; this._myDayRedraw(box.dataset.myday, '[aria-pressed="true"]'); return true; }
      if (e.target.closest('[data-myday-cancel]')) { var was = box.dataset.myday; this._myDayFor = null; this._myDayForm = null; this._myDayRedraw(was, '[data-myday-edit]'); return true; }
      if (!e.target.closest('[data-myday-save]')) return true;
      this._myDayRead(box);
      var f = this._myDayForm, iso = box.dataset.myday, err = box.querySelector('.gperr'), blocks = [];
      if (f.mode === 'cant') blocks = [{ startMinute: 0, endMinute: 1440, state: 'unavailable' }];
      else if (f.mode === 'diff') {
        if (f.start == null || f.end == null || f.end <= f.start) { err.textContent = 'The end has to be after the start.'; err.hidden = false; return true; }
        blocks = [{ startMinute: f.start, endMinute: f.end, state: 'available' }];
      }
      var zone = (this.mine && this.mine.tz) || browserZone() || 'UTC';
      $$('button, input', box).forEach(function (c) { c.disabled = true; });
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/availability/exceptions', { method: 'PUT', body: { onDate: iso, tz: zone, blocks: blocks } })
        .then(function (resp) {
          if (!resp.ok) return resp.json().catch(function () { return {}; }).then(function (j) { return Promise.reject(j && j.error); });
          self._myDayFor = null;
          self._myDayForm = null;
          self._myDayRedraw(iso);
          // Everyone's lines include this viewer's, so the weeks are read again.
          self._freeWeeks = {};
          self.freeByDate = {};
          return self.fetchMyDays({ fresh: true }).then(function () {
            self.say('Saved. Only ' + realDateWords(iso) + ' changed.');
            return Promise.all([self.fetchFreeWeek(isoMonday(iso)), self.fetchFreeMonth(self.view.y, self.view.m)]);
          }).then(function () { self._paintMonth(); self.refreshWing(); });
        })
        .catch(function (msg) {
          $$('button, input', box).forEach(function (c) { c.disabled = false; });
          err.textContent = (typeof msg === 'string' && msg) || 'That day didn’t save. Try again.';
          err.hidden = false;
        });
      return true;
    },

    _availURL: function (tab) {
      return '/campaigns/' + encodeURIComponent(this.campaignId) + '/availability' + (tab ? '?tab=' + tab : '');
    },

    _fvHTML: function () {
      var y = this.view.y, m = this.view.m, monthName = ((this.cal.months || [])[m - 1] || {}).name || '';
      var sub = [monthName + ' ' + y, this.calZone ? 'times in ' + zoneAbbr(this.calZone, y + '-' + pad2(m) + '-15') : ''].filter(Boolean).join(' · ');
      return '<div class="grab" aria-hidden="true"></div>' +
        '<div class="mvcrease"><div class="crease"><span class="mvt">Who’s free</span><span class="mvsub">' + esc(sub) + '</span><button type="button" class="x" data-close aria-label="Close">✕</button></div></div>' +
        '<div class="fvbody">' + (this.freeDirector ? this._fvDirectorHTML(y, m, monthName) : this._fvPlayerHTML(y, m)) + '</div>';
    },

    // The month's roster as the overlay names it: everyone, in the same
    // order as the lines, and whether each has painted hours. null until
    // a week has been read.
    _freeRoster: function (y, m) {
      for (var d = 1; d <= CalDate.monthDays(this.cal, m - 1, y); d++) {
        var data = this._freeOn(y, m, d);
        if (data && data.detail) return data.members.map(function (mem) { return { userId: mem.userId, name: mem.name, answered: mem.answered }; });
      }
      return null;
    },

    _fvDirectorHTML: function (y, m, monthName) {
      var self = this, slots = this.bestTimes(y, m), roster = this._freeRoster(y, m);
      var h = '<section class="fvsec fvbest"><h4>Best times' + (monthName ? ' in ' + esc(monthName) : '') + '</h4>';
      if (!roster) h += '<p class="none">Loading who’s free…</p>';
      else if (!slots.length) h += '<p class="none">No three-hour slot left this month when most players are free.</p>';
      else h += '<ul class="slots">' + slots.map(function (s) {
        var night = self.nightsOnDay(s.y, s.m, s.d)[0], missing = self._freeMissing(s.data, s.w);
        var who = s.w.free === s.data.total ? 'All ' + s.data.total + ' free' : s.w.free + ' of ' + s.data.total + ' free';
        if (missing.length && missing.length <= 2) who += ' · ' + missing.map(function (mem) { return mem.name + (mem.answered ? ' away' : ' hasn’t painted hours'); }).join(', ');
        if (night) who += ' · already a game night';
        var act = night ? '<button type="button" class="btn sm" data-best-open="' + esc(dayKey(s.y, s.m, s.d)) + '" data-best-night="' + esc(self._gnId(night)) + '">Open</button>'
          : (self.role >= 2 ? '<button type="button" class="btn sm" data-best-open="' + esc(dayKey(s.y, s.m, s.d)) + '" data-best-plan="' + esc(s.iso) + '" data-plan-at="' + s.w.start + '"><i class="fa-solid fa-dice-d20"></i> Plan it</button>'
            : '<button type="button" class="btn sm" data-best-open="' + esc(dayKey(s.y, s.m, s.d)) + '">Open</button>');
        return '<li><span><b>' + esc(realDateWords(s.iso)) + '</b>, ' + esc(self._windowWords(s.w, s.iso)) + '<small>' + esc(who) + '</small></span>' + act + '</li>';
      }).join('') + '</ul>';
      var wk = roster ? this.bestWeekday(y, m) : null;
      if (wk) {
        h += '<p class="fvwk"><b>' + esc(wk.weekday) + 's</b> are best overall, ' + esc(this._windowWords(wk.window, wk.iso)) + '.' +
          (wk.away.length ? ' ' + wk.away.map(function (a) { return esc(a.name) + ' is away ' + esc(listWords(a.dates)); }).join('. ') + '.' : '') + '</p>';
      }
      h += '</section><section class="fvsec fvwho"><h4>Players</h4>';
      if (roster) h += this._fvRosterHTML(roster);
      h += '</section><section class="fvsec fvfoot">' +
        '<label class="fvsw"><input type="checkbox" data-fv-lines' + (this.showFree ? ' checked' : '') + '><span>Show who’s free on the days</span></label>' +
        '<span class="fvlinks"><button type="button" class="lnk" data-fv-planner="mine">Change my hours</button><button type="button" class="btn sm" data-fv-planner="team"><i class="fa-solid fa-up-right-and-down-left-from-center"></i> Open the full planner</button></span></section>';
      return h;
    },

    // The Director's players: each one's state, with a reminder for that
    // one player, and the ask that has everyone confirm their times. After
    // an ask, a player's state is whether they have confirmed since.
    _fvRosterHTML: function (roster) {
      var self = this, ans = this.answers || {}, asked = ans.askedAt || '', byId = {};
      (ans.members || []).forEach(function (a) { byId[a.userId] = a; });
      var done = 0, askedOf = 0, silent = 0;
      var rows = roster.map(function (mem) {
        var a = byId[mem.userId] || {}, st, cls;
        var conf = !!(asked && a.answeredAt && a.answeredAt >= asked);
        if (!mem.answered) { st = 'No hours yet'; cls = 'wait'; silent++; }
        else if (asked) { st = conf ? 'Confirmed' : 'Not confirmed yet'; cls = conf ? 'ok' : 'wait'; }
        else { st = 'Hours given'; cls = 'ok'; }
        if (asked && mem.userId !== self.userId) { askedOf++; if (conf) done++; }
        var sent = self._reminded && self._reminded[mem.userId];
        var act = mem.userId === self.userId ? '<span class="fvme">You</span>'
          : '<button type="button" class="ib fvbell" data-remind="' + esc(mem.userId) + '"' + (sent ? ' disabled' : '') + ' aria-label="' + esc((sent ? 'Reminded ' : 'Remind ') + mem.name) + '" title="' + esc(sent ? 'Reminded' : 'Remind ' + mem.name + ' to check their times') + '"><i class="fa-solid ' + (sent ? 'fa-check' : 'fa-bell') + '"></i></button>';
        return '<li><span class="nm">' + esc(mem.name) + '</span><span class="' + cls + '">' + st + '</span>' + act + '</li>';
      });
      var h = '<ul class="fvroster">' + rows.join('') + '</ul>';
      h += '<div class="fvask">';
      if (asked) {
        h += '<p><b>' + done + ' of ' + askedOf + '</b> confirmed since you asked on ' + esc(realDateWords(asked.slice(0, 10)).replace(/^\w+ /, '')) + '.</p>';
      } else {
        h += '<p>Ask everyone to check their times are still right, on the site and by email.</p>';
      }
      h += '<span class="fvaskb"><button type="button" class="btn sm" data-ask-confirm><i class="fa-solid fa-clipboard-check"></i> ' + (asked ? 'Ask again' : 'Ask everyone to confirm') + '</button>' +
        (silent ? '<button type="button" class="btn sm quiet" data-best-nudge><i class="fa-solid fa-bell"></i> Remind ' + (silent === 1 ? 'the 1 without hours' : 'the ' + silent + ' without hours') + '</button>' : '') + '</span></div>';
      return h;
    },

    fetchAnswers: function (fresh) {
      var self = this;
      if (this._answersPromise && !fresh) return this._answersPromise;
      this._answersPromise = Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/availability/answers')
        .then(function (resp) { return resp.ok ? resp.json() : null; })
        .then(function (a) { self.answers = a || { members: [], askedAt: '' }; })
        .catch(function () { self.answers = self.answers || { members: [], askedAt: '' }; });
      return this._answersPromise;
    },

    // The player's side of an ask: the DM asked, their hours are saved, and
    // they have not confirmed since.
    _confirmDue: function () {
      var a = this.answers || {}, mine = this.mine;
      return !!(a.askedAt && mine && mine.answered && !(mine.answeredAt && mine.answeredAt >= a.askedAt));
    },

    // --- Away: whole days a player can't play, set ahead of time ---------
    // Stored as the Availability page's own days off, one per date; read
    // back here as stretches of consecutive days.
    fetchAway: function (fresh) {
      var self = this;
      if (this._awayPromise && !fresh) return this._awayPromise;
      this._awayPromise = Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/availability/exceptions')
        .then(function (resp) { return resp.ok ? resp.json() : []; })
        .then(function (list) { self.away = awayStretches(Array.isArray(list) ? list : [], self._todayIso() || localIso(new Date())); })
        .catch(function () { self.away = self.away || []; });
      return this._awayPromise;
    },

    _awayHTML: function () {
      var away = this.away, h = '<div class="fvaway"><h5>Away</h5>';
      if (away === undefined) return h + '<p class="none">Loading…</p></div>';
      if (away.length) {
        h += '<ul class="fvawl">' + away.map(function (a) {
          var words = a.from === a.to ? realDateWords(a.from) : realDateWords(a.from) + ' to ' + realDateWords(a.to);
          return '<li><span>' + esc(words) + '</span><button type="button" class="lnk" data-away-clear="' + esc(a.from + '_' + a.to) + '" aria-label="' + esc('Clear away ' + words) + '">Clear</button></li>';
        }).join('') + '</ul>';
      }
      if (this._awayForm) {
        var f = this._awayForm;
        h += '<form class="fvawf" data-away-form novalidate>' +
          '<label class="fld"><span>First day</span><input type="date" name="from" value="' + esc(f.from) + '" required></label>' +
          '<label class="fld"><span>Last day</span><input type="date" name="to" value="' + esc(f.to) + '" required></label>' +
          '<p class="gperr" role="alert"' + (f.error ? '' : ' hidden') + '>' + esc(f.error || '') + '</p>' +
          '<span class="fvawa"><button type="submit" class="btn sm primary">Mark me away</button><button type="button" class="btn sm quiet" data-away-cancel>Cancel</button></span></form>';
      } else {
        h += (away.length ? '' : '<p class="fnote">Going somewhere? Mark the days and game nights and Best times leave you out.</p>') +
          '<button type="button" class="btn sm" data-away-new><i class="fa-solid fa-plane"></i> I’m away…</button>';
      }
      return h + '</div>';
    },

    // A press on any of the time tools in the Who's free view or the
    // planner's My hours. Returns true when it handled the press.
    _timeToolClick: function (e, redraw) {
      var self = this, t = e.target, base = '/campaigns/' + encodeURIComponent(this.campaignId) + '/availability';
      var remind = t.closest('[data-remind]');
      if (remind) {
        remind.disabled = true;
        Chronicle.apiFetch(base + '/nudge', { method: 'POST', body: { userId: remind.dataset.remind } })
          .then(function (resp) { return resp.ok ? resp.json() : Promise.reject(); })
          .then(function (res) {
            self._reminded = self._reminded || {};
            self._reminded[remind.dataset.remind] = true;
            self.say('Reminded ' + ((res && res.notified || [])[0] || 'them') + ' to check their times.');
            redraw();
          })
          .catch(function () { remind.disabled = false; self.say('That reminder could not be sent. Try again.'); });
        return true;
      }
      var ask = t.closest('[data-ask-confirm]');
      if (ask) {
        ask.disabled = true;
        Chronicle.apiFetch(base + '/nudge', { method: 'POST', body: { ask: 'confirm' } })
          .then(function (resp) { return resp.ok ? resp.json() : Promise.reject(); })
          .then(function (res) {
            var n = (res && res.notified || []).length;
            self.say(n ? 'Asked ' + n + (n === 1 ? ' player' : ' players') + ' to confirm their times.' : 'There is nobody to ask yet.');
            return self.fetchAnswers(true);
          })
          .then(redraw)
          .catch(function () { ask.disabled = false; self.say('That ask could not be sent. Try again.'); });
        return true;
      }
      var conf = t.closest('[data-confirm-mine]');
      if (conf) {
        conf.disabled = true;
        Chronicle.apiFetch(base + '/confirm', { method: 'POST' })
          .then(function (resp) { return resp.ok ? resp.json() : Promise.reject(); })
          .then(function () {
            if (self.mine) self.mine.answeredAt = new Date().toISOString().replace(/\.\d+Z$/, 'Z');
            self.say('Thanks. Your times are confirmed.');
            redraw();
          })
          .catch(function () { conf.disabled = false; self.say('That could not be saved. Try again.'); });
        return true;
      }
      if (t.closest('[data-away-new]')) {
        var from = this._todayIso() || localIso(new Date());
        this._awayForm = { from: from, to: isoAddDays(from, 6) };
        redraw();
        var first = this._awayFocus(); if (first) first.focus();
        return true;
      }
      if (t.closest('[data-away-cancel]')) { this._awayForm = null; redraw(); return true; }
      var clear = t.closest('[data-away-clear]');
      if (clear) {
        var p = clear.dataset.awayClear.split('_');
        clear.disabled = true;
        this._awaySend('/away/clear', 'POST', p[0], p[1]).then(function (err) {
          if (err) { clear.disabled = false; self.say(err); return; }
          self.say('Cleared. Those days follow your usual hours again.');
          redraw();
        });
        return true;
      }
      return false;
    },

    // An away form left open holds its panel against a stray press outside,
    // and says why. Returns true when it held.
    _awayHolds: function (redraw) {
      if (!this._awayForm) return false;
      var root = this.plannerOpen() ? this.plEl : this.fvEl, form = root && root.querySelector('[data-away-form]');
      if (form) { this._awayForm.from = form.elements.from.value; this._awayForm.to = form.elements.to.value; }
      this._awayForm.error = 'You haven’t marked these days yet. Press Mark me away, or Cancel.';
      redraw();
      var err = root && root.querySelector('[data-away-form] .gperr');
      if (err) err.classList.add('shake');
      return true;
    },

    _awayFocus: function () {
      var root = this.plannerOpen() ? this.plEl : this.fvEl;
      return root && root.querySelector('[data-away-form] input');
    },

    _awaySubmit: function (form, redraw) {
      var self = this, from = form.elements.from.value, to = form.elements.to.value;
      var bad = !/^\d{4}-\d{2}-\d{2}$/.test(from) || !/^\d{4}-\d{2}-\d{2}$/.test(to) ? 'Pick the first and last day.' : to < from ? 'The last day can’t be before the first.' : '';
      if (bad) { this._awayForm = { from: from, to: to, error: bad }; redraw(); return; }
      form.querySelector('[type="submit"]').disabled = true;
      this._awaySend('/away', 'PUT', from, to).then(function (err) {
        if (err) { self._awayForm = { from: from, to: to, error: err }; redraw(); return; }
        self._awayForm = null;
        self.say('Marked away ' + realDateWords(from) + (to !== from ? ' to ' + realDateWords(to) : '') + '.');
        redraw();
      });
    },

    // Writes or clears a stretch, then rereads what it changes: the
    // player's own days off and the free hours of the weeks shown.
    _awaySend: function (path, method, from, to) {
      var self = this, zone = (this.mine && this.mine.tz) || browserZone() || 'UTC';
      return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/availability' + path, { method: method, body: { from: from, to: to, tz: zone } })
        .then(function (resp) {
          if (!resp.ok) return resp.json().then(function (j) { return (j && j.error) || 'That could not be saved.'; }, function () { return 'That could not be saved.'; });
          self.freeByDate = {};
          self._freeWeeks = {};
          return Promise.all([self.fetchAway(true), self.fetchFreeMonth(self.view.y, self.view.m)]).then(function () {
            if (self.showFree) self._paintMonth();
            return '';
          });
        })
        .catch(function () { return 'Could not reach Chronicle. Try again.'; });
    },

    _fvPlayerHTML: function (y, m) {
      var self = this, mine = this.mine, h = '<section class="fvsec fvmine"><h4>Your hours</h4>';
      if (mine === undefined) h += '<p class="none">Loading your hours…</p>';
      else if (!mine || !mine.answered) {
        h += '<p class="fvlead">You haven’t given your hours yet. The DM uses them to pick game nights.</p>' +
          '<button type="button" class="btn fvgo" data-fv-planner="mine"><i class="fa-solid fa-user-pen"></i> Give my hours</button>';
      } else {
        if (this._confirmDue()) h += '<p class="fvdue"><i class="fa-solid fa-clipboard-check"></i> The DM asked everyone to check their times are still right.</p>';
        h += this._mineWeekHTML(mine) + '<span class="fvmb">' +
          (this._confirmDue() ? '<button type="button" class="btn sm primary" data-confirm-mine><i class="fa-solid fa-check"></i> My times are still right</button>' : '') +
          '<button type="button" class="btn sm" data-fv-planner="mine"><i class="fa-solid fa-user-pen"></i> Change my hours</button></span>';
      }
      h += this._awayHTML();
      h += '</section><section class="fvsec fvnights"><h4>Game nights this month</h4>';
      var today = this._todayIso(), rows = [];
      for (var d = 1; d <= CalDate.monthDays(this.cal, m - 1, y); d++) {
        this.nightsOnDay(y, m, d).forEach(function (n) {
          if (n.date < today) return;
          var data = self.freeByDate[n.date], hr = /^\d{2}:\d{2}$/.test(n.time || '') ? parseInt(n.time, 10) : -1;
          var count = data && data.total && hr >= 0 ? (data.hours[hr] || 0) + ' of ' + data.total + ' free at ' + ampm(hr * 60) : '';
          rows.push('<li><button type="button" class="fvn" data-best-open="' + esc(dayKey(y, m, d)) + '" data-best-night="' + esc(self._gnId(n)) + '"><b>' + esc(realDateWords(n.date)) + '</b><span>' + esc(n.name) + '</span><small>' + esc(count) + '</small></button></li>');
        });
      }
      h += rows.length ? '<ul class="fvnl">' + rows.join('') + '</ul>' : '<p class="none">No game nights left this month.</p>';
      return h + '<p class="fnote">Click any day for how many are free that day.</p></section>';
    },

    // A player's week as they painted it, one line a day, Monday first, in
    // the zone they painted it in.
    _mineWeekHTML: function (mine) {
      var names = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'], dows = [1, 2, 3, 4, 5, 6, 0], self = this;
      var blocks = Array.isArray(mine.blocks) ? mine.blocks : [];
      return '<div class="flines">' + dows.map(function (dw, i) {
        var segs = blocks.filter(function (b) { return b.dayOfWeek === dw; }).map(function (b) { return [b.startMinute, b.endMinute]; });
        return '<div class="fr"><span class="nm">' + names[i] + '</span>' + self._lineHTML(segs) + '</div>';
      }).join('') + '</div><div class="fticks" aria-hidden="true"><span>12am</span><span>6am</span><span>noon</span><span>6pm</span><span>12am</span></div>' +
        (mine.tz ? '<p class="fnote">Repeats every week, in ' + esc(mine.tz.replace(/_/g, ' ')) + ' time.</p>' : '');
    },

    // --------------------------------------------------------------
    // The full planner: a drawer out of the calendar's right edge, like
    // the full event editor, so the whole of Who's free stays inside the
    // calendar. The Director sees a week of everyone's hours, day by day,
    // the month's game nights with who is coming, Best times and the
    // players; anyone can paint their own weekly hours here, which is the
    // Availability page's every-week pattern (alternating weeks and
    // one-off days stay on that page, and are kept as they are).
    // --------------------------------------------------------------
    openPlanner: function (o) {
      var self = this;
      o = o || {};
      if (!this.freeCan) return;
      var back = document.activeElement;
      this.closeAllPanels({ instant: true });
      if (!this.plEl) {
        var dock = this.dockEl;
        this.plScrim = $('.dscrim', dock);
        if (!this.plScrim) { this.plScrim = document.createElement('div'); this.plScrim.className = 'dscrim'; dock.appendChild(this.plScrim); }
        this.plEl = document.createElement('aside');
        this.plEl.className = 'drawer ded fvd';
        this.plEl.setAttribute('role', 'dialog');
        this.plEl.setAttribute('aria-label', 'Who’s free');
        this.plEl.tabIndex = -1;
        dock.appendChild(this.plEl);
        this.plEl.addEventListener('click', function (e) { self._plClick(e); });
        this.plEl.addEventListener('submit', function (e) {
          var form = e.target.closest('[data-away-form]');
          if (!form) return;
          e.preventDefault();
          self._awaySubmit(form, function () { self.refreshPlanner(); });
        });
        this.plEl.addEventListener('pointerdown', function (e) { self._paintStart(e); });
        this.plEl.addEventListener('pointerover', function (e) { self._paintMove(e); });
        this._plUp = function () { self._paintEnd(); };
        document.addEventListener('pointerup', this._plUp);
        this.plScrim.addEventListener('click', function () { if (self.plannerOpen() && !self._awayHolds(function () { self.refreshPlanner(); })) self.closePlanner(); });
      }
      this._plBack = back;
      this._plWeek = this._plWeek || isoMonday(this._todayIso() || (this.view.y + '-' + pad2(this.view.m) + '-01'));
      this._plPaint = o.paint || !this.freeDirector;
      this.plEl.innerHTML = this._plHTML();
      this.plEl.classList.add('open');
      this.dockEl.classList.add('on');
      setTimeout(function () { self.plEl.focus({ preventScroll: true }); }, reducedMotion() ? 20 : 260);
      var reads = [this.fetchFreeWeek(this._plWeek), this.fetchFreeMonth(this.view.y, this.view.m), this.fetchMine(), this.fetchAnswers(), this.fetchAway()];
      Promise.all(reads).then(function () { self._plLoadGrid(); self.refreshPlanner(); });
    },

    plannerOpen: function () { return !!(this.plEl && this.plEl.classList.contains('open')); },

    closePlanner: function (quiet) {
      if (!this.plannerOpen()) return;
      this.plEl.classList.remove('open');
      this.dockEl.classList.remove('on');
      this._painting = null;
      if (!quiet && this._plBack && this._plBack.isConnected) this._plBack.focus({ preventScroll: true });
    },

    refreshPlanner: function () {
      if (!this.plannerOpen()) return;
      var body = this.plEl.querySelector('.dbody'), top = body ? body.scrollTop : 0;
      this.plEl.innerHTML = this._plHTML();
      body = this.plEl.querySelector('.dbody');
      if (body) body.scrollTop = top;
    },

    _plWeekWords: function (monday) {
      var a = realDateWords(monday), b = realDateWords(isoAddDays(monday, 6));
      return a.replace(/^\w+ /, '') + ' – ' + b.replace(/^\w+ /, '');
    },

    _plHTML: function () {
      var dir = this.freeDirector, paint = this._plPaint;
      var tabs = dir ? '<div class="rseg fvtabs" role="tablist"><button type="button" role="tab" data-pl-tab="team" aria-pressed="' + !paint + '">Everyone</button>' +
        '<button type="button" role="tab" data-pl-tab="mine" aria-pressed="' + !!paint + '">My hours</button></div>' : '';
      return '<div class="crease"><span id="cal5-pltitle">Who’s free</span>' + tabs + '<button type="button" class="x" data-pl-close aria-label="Close">✕</button></div>' +
        '<div class="dbody">' + (paint ? this._plMineHTML() : this._plTeamHTML()) + '</div>';
    },

    _plTeamHTML: function () {
      var self = this, monday = this._plWeek, y = this.view.y, m = this.view.m, monthName = ((this.cal.months || [])[m - 1] || {}).name || '';
      var h = '<section class="dsec"><div class="plweek"><button type="button" class="ib" data-pl-week="-1" aria-label="Previous week"><i class="fa-solid fa-chevron-left"></i></button>' +
        '<h4>Week of ' + esc(this._plWeekWords(monday)) + '</h4><button type="button" class="ib" data-pl-week="1" aria-label="Next week"><i class="fa-solid fa-chevron-right"></i></button></div>';
      var any = false;
      for (var i = 0; i < 7; i++) {
        var iso = isoAddDays(monday, i), data = this.freeByDate[iso];
        if (!data || !data.detail) continue;
        any = true;
        var all = this._freeRuns(data.hours, data.members.filter(function (mem) { return mem.answered; }).length)[0];
        var w = all ? { start: all[0], end: all[1], free: data.total } : this._bestWindow(data, 1);
        var p = iso.split('-'), key = dayKey(+p[0], +p[1], +p[2]);
        h += '<div class="plday"><div class="pldh"><button type="button" class="lnk" data-pl-day="' + esc(key) + '"><b>' + esc(realDateWords(iso)) + '</b></button>' +
          '<span>' + esc(!w ? 'Nobody free' : (w.free === data.total ? 'Everyone ' : w.free + ' of ' + data.total + ' ') + this._windowWords(w, iso)) + '</span></div>' +
          '<div class="flines"><span class="bandwrap" aria-hidden="true">' + this._bandHTML(data, 'band2') + '</span>' + data.members.map(function (mem) {
            return '<div class="fr"><span class="nm">' + esc(mem.name) + '</span>' + self._lineHTML(mem.segs, mem.off) + '<span class="sr">' + esc(mem.name + ': ' + self._segsWords(mem)) + '</span></div>';
          }).join('') + '</div></div>';
      }
      if (!any) h += '<p class="none">Loading who’s free…</p>';
      else h += '<div class="fticks pltk" aria-hidden="true"><span>12am</span><span>6am</span><span>noon</span><span>6pm</span><span>12am</span></div>' + FREE_LEGEND;
      h += '</section>';
      // The month's game nights, with who is coming to each.
      var nights = [], today = this._todayIso();
      for (var d = 1; d <= CalDate.monthDays(this.cal, m - 1, y); d++) {
        this.nightsOnDay(y, m, d).forEach(function (n) { if (n.date >= today) nights.push({ n: n, key: dayKey(y, m, d) }); });
      }
      h += '<section class="dsec"><h4>Game nights in ' + esc(monthName) + '</h4>';
      h += nights.length ? '<ul class="plnights">' + nights.map(function (x) {
        var n = x.n, t = n.tally || {};
        return '<li><button type="button" class="plnb" data-pl-day="' + esc(x.key) + '" data-pl-night="' + esc(self._gnId(n)) + '"><b>' + esc(realDateWords(n.date)) + '</b> · ' + esc(n.name) +
          '<small>' + (t.going || 0) + ' going · ' + (t.maybe || 0) + ' maybe · ' + (t.cant || 0) + ' can’t · ' + (t.noAnswer || 0) + ' no answer</small></button>' + self._gnChipsHTML(n) + '</li>';
      }).join('') + '</ul>' : '<p class="none">No game nights left this month.</p>';
      h += '</section>';
      // Best times and the players, as in the quick view.
      var quick = this._fvDirectorHTML(y, m, monthName).replace(/<section class="fvsec fvfoot">[\s\S]*$/, '');
      h += '<div class="plquick">' + quick + '</div>';
      h += '<section class="dsec plfoot"><label class="fvsw"><input type="checkbox" data-fv-lines' + (this.showFree ? ' checked' : '') + '><span>Show who’s free on the days</span></label>' +
        '<a class="lnk" href="' + esc(this._availURL('overlay')) + '">Date polls</a></section>';
      return h;
    },

    // --- Painting your own weekly hours ---------------------------------
    // Three grids, one per week cadence (0 every week, 1 and 2 the two
    // alternating weeks), each [col 0..6, Monday first][hour 0..23] =
    // '' | available | preferred, the Availability page's layers at the same
    // one-hour grain. Alternating mode paints grids 1 and 2; otherwise
    // grid 0. Every-week hours in a member who also has alternating ones
    // apply on both weeks, so they are painted into both.
    _plLoadGrid: function () {
      var mine = this.mine || {}, grids = { 0: emptyWeekGrid(), 1: emptyWeekGrid(), 2: emptyWeekGrid() };
      var blocks = mine.blocks || [];
      var alt = blocks.some(function (b) { return b.weekCadence === 1 || b.weekCadence === 2; });
      blocks.forEach(function (b) {
        var cad = b.weekCadence === 1 || b.weekCadence === 2 ? b.weekCadence : 0;
        var targets = alt && cad === 0 ? [1, 2] : [cad];
        var col = (b.dayOfWeek + 6) % 7;
        targets.forEach(function (g) {
          for (var hh = Math.floor(b.startMinute / 60); hh < Math.ceil(b.endMinute / 60) && hh < 24; hh++) grids[g][col][hh] = b.state || 'available';
        });
      });
      this._plGrids = grids;
      this._plAlt = alt;
      this._plTool = this._plTool || 'available';
      this._plDirty = false;
    },

    _plBlocks: function () {
      var g = this._plGrids;
      return this._plAlt ? weekGridBlocks(g[1], 1).concat(weekGridBlocks(g[2], 2)) : weekGridBlocks(g[0], 0);
    },

    // Switching to alternating weeks starts both weeks from the usual
    // hours; switching back keeps the first week's hours as the usual ones.
    _plSetAlt: function (on) {
      if (on === this._plAlt) return;
      var g = this._plGrids;
      if (on) {
        if (weekGridEmpty(g[1]) && weekGridEmpty(g[2])) { g[1] = copyWeekGrid(g[0]); g[2] = copyWeekGrid(g[0]); }
      } else if (weekGridEmpty(g[0])) {
        g[0] = copyWeekGrid(g[1]);
      }
      this._plAlt = on;
      this._plDirty = true;
      this._plStatus = '';
      this.refreshPlanner();
    },

    _plGridHTML: function (g, label) {
      // An alternating week starts on Sunday (the server counts the weeks
      // from a Sunday), so its grid does too.
      var names = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'], order = g === 0 ? [0, 1, 2, 3, 4, 5, 6] : [6, 0, 1, 2, 3, 4, 5];
      var grid = this._plGrids[g];
      var h = '<div class="plgrid" role="grid" aria-label="' + esc(label) + '"><div class="plgh" aria-hidden="true"><span></span><span>12am</span><span>6am</span><span>noon</span><span>6pm</span></div>';
      order.forEach(function (c) {
        h += '<div class="plrow" role="row"><span class="nm">' + names[c] + '</span><span class="plcells">';
        for (var hr = 0; hr < 24; hr++) {
          var st = grid[c][hr];
          h += '<button type="button" class="plc" role="gridcell" data-g="' + g + '" data-c="' + c + '" data-h="' + hr + '" data-st="' + st + '" aria-label="' + names[c] + ' ' + ampm(hr * 60) + (st ? ', ' + (st === 'preferred' ? 'best' : 'free') : '') + '"></button>';
        }
        h += '</span></div>';
      });
      return h + '</div>';
    },

    _plMineHTML: function () {
      if (!this._plGrids) return '<section class="dsec"><p class="none">Loading your hours…</p></section>';
      var self = this, zone = (this.mine && this.mine.tz) || browserZone() || '', alt = this._plAlt;
      var tools = [['available', 'Free'], ['preferred', 'Best'], ['', 'Clear']];
      var inZone = zone ? ', in ' + esc(zone.replace(/_/g, ' ')) + ' time' : '';
      var h = '<section class="dsec"><h4>Your hours</h4>' +
        '<div class="rseg plalt" role="group" aria-label="How your hours repeat">' +
          '<button type="button" data-pl-alt="0" aria-pressed="' + !alt + '">Same every week</button>' +
          '<button type="button" data-pl-alt="1" aria-pressed="' + alt + '">Alternating weeks</button></div>' +
        '<p class="dnote">' + (alt ? 'Paint each of the two weeks. They take turns, every other week' : 'Drag across the hours you can usually play. They repeat every week') + inZone + '.</p>' +
        '<div class="rseg pltools" role="group" aria-label="Paint">' + tools.map(function (t) {
          return '<button type="button" data-pl-tool="' + t[0] + '" aria-pressed="' + (self._plTool === t[0]) + '"><span class="plsw ' + (t[0] || 'none') + '"></span>' + t[1] + '</button>';
        }).join('') + '</div>';
      if (alt) {
        var weeks = cadenceWeeks(this._todayIso() || localIso(new Date()));
        // The sooner week first.
        (weeks[1] < weeks[2] ? [1, 2] : [2, 1]).forEach(function (g) {
          var title = 'Week of ' + realDateWords(weeks[g]) + ' and every other week after';
          h += '<h5 class="plwk">' + esc(title) + '</h5>' + self._plGridHTML(g, title);
        });
      } else {
        h += this._plGridHTML(0, 'Your weekly hours');
      }
      h += '<div class="plsave"><button type="button" class="btn primary" data-pl-save' + (this._plDirty ? '' : ' disabled') + '>Save my hours</button><span class="plstat" role="status">' + esc(this._plStatus || '') + '</span></div>';
      h += '<p class="dnote">A day that’s different, or a day you can’t play: open that day in the calendar.</p></section>';
      h += '<section class="dsec">' + this._awayHTML() + '</section>';
      return h;
    },

    _paintStart: function (e) {
      var c = e.target.closest && e.target.closest('.plc');
      if (!c || !this._plGrids) return;
      e.preventDefault();
      var col = +c.dataset.c, hr = +c.dataset.h, cur = this._plGrids[+c.dataset.g || 0][col][hr];
      this._painting = { v: cur === this._plTool ? '' : this._plTool };
      this._paintCell(c);
    },
    _paintMove: function (e) {
      if (!this._painting) return;
      var c = e.target.closest && e.target.closest('.plc');
      if (c) this._paintCell(c);
    },
    _paintEnd: function () {
      if (!this._painting) return;
      this._painting = null;
      var btn = this.plEl && this.plEl.querySelector('[data-pl-save]');
      if (btn) btn.disabled = !this._plDirty;
    },
    _paintCell: function (c) {
      var col = +c.dataset.c, hr = +c.dataset.h, v = this._painting.v, grid = this._plGrids[+c.dataset.g || 0];
      if (grid[col][hr] === v) return;
      grid[col][hr] = v;
      c.dataset.st = v;
      this._plDirty = true;
      this._plStatus = '';
    },

    _plSave: function (btn) {
      var self = this, zone = (this.mine && this.mine.tz) || browserZone() || 'UTC';
      btn.disabled = true;
      this._setPlStatus('Saving…');
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/availability/mine', { method: 'PUT', body: { tz: zone, blocks: this._plBlocks() } })
        .then(function (resp) {
          if (!resp.ok) return Promise.reject();
          self._plDirty = false;
          self.mine = { answered: true, tz: zone, blocks: self._plBlocks() };
          // Everyone's lines include this viewer's, so the weeks are read again.
          self._freeWeeks = {};
          self.freeByDate = {};
          self._plStatus = 'Saved. The DM sees your new hours.';
          return Promise.all([self.fetchFreeWeek(self._plWeek), self.fetchFreeMonth(self.view.y, self.view.m)]);
        })
        .then(function () { self._paintMonth(); self.refreshPlanner(); self.say('Your hours are saved.'); })
        .catch(function () { btn.disabled = false; self._setPlStatus('Your hours could not be saved. Try again.'); });
    },
    _setPlStatus: function (msg) {
      this._plStatus = msg;
      var st = this.plEl && this.plEl.querySelector('.plstat');
      if (st) st.textContent = msg;
    },

    _plClick: function (e) {
      var self = this, t = e.target;
      if (t.closest('[data-pl-close]')) { this.closePlanner(); return; }
      var tab = t.closest('[data-pl-tab]');
      if (tab) { this._plPaint = tab.dataset.plTab === 'mine'; this.refreshPlanner(); return; }
      var tool = t.closest('[data-pl-tool]');
      if (tool) { this._plTool = tool.dataset.plTool; this.refreshPlanner(); return; }
      var alt = t.closest('[data-pl-alt]');
      if (alt) { this._plSetAlt(alt.dataset.plAlt === '1'); return; }
      if (t.closest('[data-pl-save]')) { this._plSave(t.closest('[data-pl-save]')); return; }
      var wk = t.closest('[data-pl-week]');
      if (wk) {
        this._plWeek = isoAddDays(this._plWeek, 7 * +wk.dataset.plWeek);
        this.refreshPlanner();
        this.fetchFreeWeek(this._plWeek).then(function () { self.refreshPlanner(); });
        return;
      }
      var lines = t.closest('[data-fv-lines]');
      if (lines) { this.setShowFree(lines.checked); return; }
      var nudge = t.closest('[data-best-nudge]');
      if (nudge) { this._fvHandleClick(e); return; }
      if (this._timeToolClick(e, function () { self.refreshPlanner(); })) return;
      var go = t.closest('[data-pl-day], [data-best-open]');
      if (go) {
        var key = go.dataset.plDay || go.dataset.bestOpen, night = go.dataset.plNight || go.dataset.bestNight || null;
        var plan = go.dataset.bestPlan ? { iso: go.dataset.bestPlan, start: parseInt(go.dataset.planAt, 10) || 19 } : null;
        var k = parseDayKey(key);
        this.closePlanner(true);
        if (k.y !== this.view.y || k.m !== this.view.m) { this.view = { y: k.y, m: k.m }; this.renderMonth(); }
        this._planFor = plan;
        setTimeout(function () { self._planFor = plan; self.openWing(key, night); }, reducedMotion() ? 20 : 300);
      }
    },

    _fvHandleClick: function (e) {
      var self = this, open = e.target.closest('[data-best-open]'), nudge = e.target.closest('[data-best-nudge]');
      if (e.target.closest('[data-close]')) { this.closeFreeView(); return; }
      var redraw = this.plannerOpen() ? function () { self.refreshPlanner(); } : function () { self.refreshFreeView(); };
      if (this._timeToolClick(e, redraw)) return;
      var pl = e.target.closest('[data-fv-planner]');
      if (pl) { this.openPlanner({ paint: pl.dataset.fvPlanner === 'mine' }); return; }
      if (open) {
        var key = open.dataset.bestOpen, night = open.dataset.bestNight || null;
        this._planFor = open.dataset.bestPlan ? { iso: open.dataset.bestPlan, start: parseInt(open.dataset.planAt, 10) || 19 } : null;
        this.closeFreeView();
        this.openWing(key, night);
        return;
      }
      if (!nudge) return;
      nudge.disabled = true;
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.campaignId) + '/availability/nudge', { method: 'POST' })
        .then(function (resp) { return resp.ok ? resp.json() : Promise.reject(); })
        .then(function (res) {
          var n = (res && res.notified || []).length;
          nudge.innerHTML = '<i class="fa-solid fa-check"></i> ' + (n ? 'Reminded' : 'Already asked');
          self.say(n ? 'Asked ' + n + (n === 1 ? ' player' : ' players') + ' to paint their hours.' : 'Everyone has already been asked.');
        })
        .catch(function () { nudge.disabled = false; self.say('That reminder could not be sent. Try again.'); });
    },

    // Every event touching (y,m,d): base-date match, recurrence expansion,
    // or a non-recurring multi-day span.
    eventsOnDay: function (y, m, d) {
      var cal = this.cal, list = this.eventsFor(y, m);
      // Events based in the previous/next month can still span into this
      // one, or recur onto it; a full sweep of just this month's list
      // covers the overwhelming majority (ListEventsForMonth's own
      // recurring/spanning WHERE clause already includes those
      // candidates), so no extra fetch is needed here.
      return list.filter(function (e) {
        if (CalDate.hasExpansion(e)) return CalDate.occurrenceOn(e, y, m, d) !== null;
        if (e.year === y && e.month === m && e.day === d) return true;
        if (e.is_recurring && CalDate.occursOn(cal, e, y, m, d)) return true;
        if (!e.is_recurring && CalDate.withinSpan(cal, e, y, m, d)) return true;
        return false;
      });
    },

    mainMoon: function () {
      var moons = this.cal.moons || [];
      var visible = moons.filter(function (mo) { return !mo.hidden_from_players; });
      return visible[0] || moons[0] || null;
    },

    // Moon.ID round-trips as a JSON number, but every caller here resolves a
    // moon from a DOM dataset attribute (always a string) — String()
    // coerces both sides so a numeric id and its dataset string still match.
    moonById: function (id) {
      return (this.cal.moons || []).filter(function (mo) { return String(mo.id) === String(id); })[0] || null;
    },

    seasonForDate: function (month, day) {
      var seasons = this.cal.seasons || [];
      for (var i = 0; i < seasons.length; i++) {
        var s = seasons[i], startVal = s.start_month * 100 + s.start_day, endVal = s.end_month * 100 + s.end_day, dateVal = month * 100 + day;
        var hit = startVal <= endVal ? (dateVal >= startVal && dateVal <= endVal) : (dateVal >= startVal || dateVal <= endVal);
        if (hit) return s;
      }
      return null;
    },

    // eraForDate is the era a day belongs to (EraMath.at): when eras
    // overlap, the one that began most recently.
    eraForDate: function (year, month, day) {
      var r = EraMath.at(EraMath.sorted(this.cal), CalDate.dayIndex(this.cal, year, month, day));
      return r ? r.era : null;
    },

    // --------------------------------------------------------------
    // Rendering: header + month grid
    // --------------------------------------------------------------
    renderHeader: function () {
      var cal = this.cal, mc = CalDate.monthCount(cal), m0 = ((this.view.m - 1) % mc + mc) % mc;
      var monthDef = (cal.months || [])[m0];
      var mtitle = $('#cal5-mtitle', this.el);
      mtitle.innerHTML = esc(monthDef ? monthDef.name : ('Month ' + this.view.m)) + ' <span class="myear">' + esc(this.view.y) + '</span>';

      this._renderEraChips();
      var season = this.seasonForDate(this.view.m, 15);
      $('#cal5-season', this.el).innerHTML = season ? ('<b>' + esc(season.name) + '</b>') : '';

      var dow = this.dowEl, weekdays = cal.weekdays || [];
      dow.innerHTML = weekdays.map(function (w) {
        return '<span><span class="f">' + esc(w.name) + '</span><span class="s">' + esc(w.name.slice(0, 1)) + '</span></span>';
      }).join('');
      this.calEl.style.setProperty('--cols', weekdays.length || 7);
    },

    renderToday: function (bump) {
      var cal = this.cal;
      var el = $('#cal5-todaypill', this.el);
      var mc = CalDate.monthCount(cal), m0 = ((cal.current_month - 1) % mc + mc) % mc;
      var monthDef = (cal.months || [])[m0];
      var label = (monthDef ? monthDef.name : cal.current_month) + ' ' + cal.current_day + ', ' + cal.current_year;
      if (!this._canSetToday()) {
        el.classList.remove('step');
        el.innerHTML = '<span class="tl">Today</span><span class="td">' + esc(label) + '</span>';
        return;
      }
      // The DM moves the world's today here: a day back or on with the
      // arrows, or any date and time from the date itself.
      var t = this._todayTimeWords();
      el.classList.add('step');
      el.innerHTML = '<span class="tl">Today</span>' +
        '<button type="button" class="sb" data-today-step="-1" aria-label="Move today back a day"><i class="fa-solid fa-chevron-left"></i></button>' +
        '<button type="button" class="td tdset' + (bump ? ' bump' : '') + '" data-today-set aria-haspopup="dialog" title="Set the world’s date and time">' + esc(label) + (t ? '<span class="tt">' + esc(t) + '</span>' : '') + '</button>' +
        '<button type="button" class="sb" data-today-step="1" aria-label="Move today on a day"><i class="fa-solid fa-chevron-right"></i></button>';
    },

    // --------------------------------------------------------------
    // Set today: the owner moves a world calendar's current date and time.
    // A real-world calendar follows the clock, so it has nothing to set.
    // --------------------------------------------------------------
    _canSetToday: function () {
      return this.role >= 3 && !!this.apiBase && !CalDate.usesRealTime(this.cal);
    },

    _hoursPerDay: function () { return this.cal.hours_per_day > 0 ? this.cal.hours_per_day : 24; },
    _minutesPerHour: function () { return this.cal.minutes_per_hour > 0 ? this.cal.minutes_per_hour : 60; },

    // "3:05pm" on a 24-hour day, "Hour 13:05" on any other.
    _todayTimeWords: function () {
      var c = this.cal, h = c.current_hour || 0, mm = c.current_minute || 0;
      if (c.current_hour == null) return '';
      return this._hoursPerDay() === 24 && this._minutesPerHour() === 60 ? ampm(h * 60 + mm) : 'Hour ' + h + ':' + pad2(mm);
    },

    showSetToday: function (btn) {
      var self = this, c = this.cal, mc = CalDate.monthCount(c);
      this._popBtn = btn;
      var months = (c.months || []).map(function (mo, i) {
        return '<option value="' + (i + 1) + '"' + (i + 1 === c.current_month ? ' selected' : '') + '>' + esc(mo.name || 'Month ' + (i + 1)) + '</option>';
      }).join('') || Array.from({ length: mc }, function (_, i) { return '<option value="' + (i + 1) + '">' + (i + 1) + '</option>'; }).join('');
      this.popEl.innerHTML = '<div class="crease"><span>Today in the world</span><button type="button" class="x" data-close aria-label="Close">✕</button></div>' +
        '<form class="settoday" data-today-form novalidate>' +
          '<div class="strow3"><label class="fld"><span>Month</span><select name="month">' + months + '</select></label>' +
          '<label class="fld"><span>Day</span><input type="number" name="day" inputmode="numeric" min="1" value="' + esc(String(c.current_day)) + '"></label>' +
          '<label class="fld"><span>Year</span><input type="number" name="year" inputmode="numeric" value="' + esc(String(c.current_year)) + '"></label></div>' +
          '<div class="strow2"><label class="fld"><span>Hour</span><input type="number" name="hour" inputmode="numeric" min="0" max="' + (this._hoursPerDay() - 1) + '" value="' + (c.current_hour || 0) + '"></label>' +
          '<label class="fld"><span>Minute</span><input type="number" name="minute" inputmode="numeric" min="0" max="' + (this._minutesPerHour() - 1) + '" value="' + (c.current_minute || 0) + '"></label></div>' +
          '<p class="stn">Players see the new date at once.</p>' +
          '<p class="sterr" role="alert" hidden></p>' +
          '<div class="sta"><button type="submit" class="btn sm primary">Set today</button><button type="button" class="btn sm quiet" data-close>Cancel</button></div>' +
        '</form>';
      growOpen(this.popEl, btn, this.calEl);
      this._updateScrim();
      setTimeout(function () { var f = self.popEl.querySelector('select'); if (f) f.focus({ preventScroll: true }); }, reducedMotion() ? 20 : 230);
    },

    _readTodayForm: function (form) {
      var n = function (k) { var v = form.elements[k].value.trim(); return /^-?\d+$/.test(v) ? parseInt(v, 10) : NaN; };
      var d = { y: n('year'), m: n('month'), d: n('day'), h: n('hour'), mi: n('minute') };
      if ([d.y, d.m, d.d, d.h, d.mi].some(isNaN)) return { error: 'Fill in every box with a number.' };
      var len = CalDate.monthDays(this.cal, d.m - 1, d.y);
      if (d.d < 1 || d.d > len) return { error: 'That month has ' + len + ' days.' };
      if (d.h < 0 || d.h >= this._hoursPerDay()) return { error: 'The hour runs from 0 to ' + (this._hoursPerDay() - 1) + '.' };
      if (d.mi < 0 || d.mi >= this._minutesPerHour()) return { error: 'The minute runs from 0 to ' + (this._minutesPerHour() - 1) + '.' };
      return d;
    },

    // _saveToday writes the world's new today and, once it is stored,
    // redraws everything that hangs off it. Resolves to an error message or ''.
    _saveToday: function (d) {
      var self = this;
      var body = { current_year: d.y, current_month: d.m, current_day: d.d, current_hour: d.h, current_minute: d.mi };
      return Chronicle.apiFetch(this.apiBase, { method: 'PUT', body: body })
        .then(function (resp) {
          if (!resp.ok) return resp.json().then(function (j) { return (j && (j.error || j.message)) || 'Could not set today.'; }, function () { return 'Could not set today.'; });
          Object.assign(self.cal, body);
          self._anchor = realAnchor(self.cal);
          self.nightsByMonth = {};
          self.renderToday(true);
          self.goToday();
          self.say('Today is now ' + $('#cal5-todaypill .tdset', self.el).textContent + '.');
          return '';
        })
        .catch(function () { return 'Could not reach Chronicle. Try again.'; });
    },

    _stepToday: function (delta) {
      if (this._todayBusy) return;
      var self = this, c = this.cal, next = CalDate.addDays(c, { y: c.current_year, m: c.current_month, d: c.current_day }, delta);
      this._todayBusy = true;
      this._saveToday({ y: next.y, m: next.m, d: next.d, h: c.current_hour || 0, mi: c.current_minute || 0 }).then(function (err) {
        self._todayBusy = false;
        if (err) self._toast(err);
      });
    },

    renderLegend: function () {
      var kinds = this.cal.event_kinds || [];
      $('#cal5-legend', this.el).innerHTML = kinds.slice(0, 8).map(function (k) {
        var color = sanitizeColor(k.color);
        return '<li><span class="mk" style="' + (color ? 'color:' + color + ';' : '') + '">' + esc(k.icon || '●') + '</span>' + esc(k.name) + '</li>';
      }).join('');
    },

    renderMonth: function () {
      var self = this, cal = this.cal;
      this.renderHeader();
      Promise.all([this.fetchMonth(this.view.y, this.view.m), this.fetchWeatherYear(this.view.y), this.fetchForecast(), this.fetchNights(this.view.y, this.view.m),
        this.showFree ? this.fetchFreeMonth(this.view.y, this.view.m) : null]).then(function () {
        self._paintMonth();
        self.refreshFreeView();
        // The open day's card shows what the fetch just brought.
        self.refreshWing();
      });
      // Paint immediately from cache too (avoids a blank grid while the
      // fetch above is in flight for a month already cached).
      this._paintMonth();
    },

    _paintMonth: function () {
      var cal = this.cal, y = this.view.y, m = this.view.m;
      var mc = CalDate.monthCount(cal), m0 = ((m - 1) % mc + mc) % mc;
      var monthDef = (cal.months || [])[m0];
      var weekLen = CalDate.weekLen(cal);
      var html = '';

      if (monthDef && monthDef.is_intercalary) {
        // An intercalary/festival month can be more than one day long
        // (Month.Days carries no upper bound) — a band per day, not just
        // day 1, or every day past the first would be unreachable: never
        // rendered, never clickable, and any event stored on it invisible.
        var interDays = CalDate.monthDays(cal, m0, y);
        for (var bd = 1; bd <= interDays; bd++) html += this._dayBandHTML(y, m, bd, monthDef, interDays);
      } else {
        var days = CalDate.monthDays(cal, m0, y);
        var firstCol = CalDate.weekdayCol(cal, y, m, 1);
        // -1 (MonthStartsNewWeek + an intercalary month) can't actually
        // reach here — the branch above already routes every intercalary
        // month to the band renderer — but weekdayCol's contract allows it,
        // and clamping keeps this file's handling the same as the server's
        // own preview grid (view_helpers.go's buildMonthGrid: "its days run
        // from the first column") rather than leaning on a guard elsewhere
        // that could change.
        if (firstCol < 0) firstCol = 0;
        this._gridOff = firstCol;
        var cells = [];
        for (var i = 0; i < firstCol; i++) cells.push(null);
        for (var d = 1; d <= days; d++) cells.push(d);
        while (cells.length % weekLen !== 0) cells.push(null);
        html += '<div class="wk">';
        for (var i2 = 0; i2 < cells.length; i2++) {
          if (i2 > 0 && i2 % weekLen === 0) html += '</div><div class="wk">';
          html += cells[i2] ? this._dayCellHTML(y, m, cells[i2]) : '<div class="day"></div>';
        }
        html += '</div>';
      }
      this.stageEl.innerHTML = '<div class="month">' + html + '</div>';
      // An open day's card follows its day into the redrawn grid (not one
      // on its way out: that one is put away with its old cell).
      var ps = this._pw.state, open = (ps === 'open' || ps === 'opening') && this.stageEl.querySelector('.day[data-key="' + this.wingFor + '"]');
      if (open) {
        this._pw.src = open;
        this._markOpenDay(open);
        if (this._pw.carried) open.classList.add('packed');
      }
      this._paintEraField(this._eraFade ? { crossfade: true } : null);
      this._eraFade = false;
      this._skyDay();
    },

    _dayBandHTML: function (y, m, d, monthDef, totalDays) {
      var cal = this.cal;
      var isToday = y === cal.current_year && m === cal.current_month && d === cal.current_day;
      var marks = this._marksHTML(y, m, d);
      var isFuture = CalDate.dayIndex(cal, y, m, d) > CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
      var label = esc(monthDef.name) + (totalDays > 1 ? ', day ' + d : '');
      return '<div class="band' + (isToday ? ' today' : '') + '">' +
        '<button type="button" class="day" data-key="' + dayKey(y, m, d) + '" style="width:100%">' +
          '<div class="dc"><span class="bn">' + label + '</span>' + weatherMarkHTML(this.weatherOnDay(y, m, d), isFuture) + this.forecastMarkOn(y, m, d, isFuture) + marks + this._eraStartHTML(y, m, d) + '</div>' +
          (this.canAuthorDmOnly ? this._paintMarkHTML(y, m, d, isFuture) : '') +
        '</button></div>';
    },

    // Edit mode's weather mark: a Generate preview (wxPreview, keyed like the
    // grid) shows as a faint ghost, except over a day painted by hand, which
    // a generated reading never replaces.
    _paintMarkHTML: function (y, m, d, future) {
      var stored = (this.weatherByYear[y] || {})[m + '_' + d];
      var pv = this.wxPreview && this.wxPreview[dayKey(y, m, d)];
      if (pv && !(stored && (stored.source !== 'generated' || stored.locked === true))) return weatherPaintHTML(pv, future, true);
      return weatherPaintHTML(stored, future);
    },

    _dayCellHTML: function (y, m, d) {
      var cal = this.cal, key = dayKey(y, m, d);
      var isToday = y === cal.current_year && m === cal.current_month && d === cal.current_day;
      var isPast = CalDate.dayIndex(cal, y, m, d) < CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
      var moon = this.mainMoon();
      var moonHTML = '';
      if (moon) {
        var abs = CalDate.dayIndex(cal, y, m, d);
        var phase = MoonMath.phase(moon, abs);
        moonHTML = '<span class="msil" data-moon-day="' + key + '" title="' + esc(moon.name + ', ' + MoonMath.name(phase)) + '">' + this._moonSilSVG(phase) + '</span>';
      }
      return '<button type="button" class="day' + (isToday ? ' today' : '') + (isPast ? ' past' : '') + '" data-key="' + key + '">' +
        moonHTML + weatherMarkHTML(this.weatherOnDay(y, m, d), !isPast && !isToday) + this.forecastMarkOn(y, m, d, !isPast && !isToday) +
        (this.canAuthorDmOnly ? this._paintMarkHTML(y, m, d, !isPast && !isToday) : '') +
        '<div class="dc"><span class="num">' + d + '</span>' + this._marksHTML(y, m, d) + '</div>' +
        this._eraStartHTML(y, m, d) +
        this._freeCellHTML(y, m, d) +
        '</button>';
    },

    _marksHTML: function (y, m, d) {
      var evs = this.eventsOnDay(y, m, d), nights = this.nightsOnDay(y, m, d);
      if (!evs.length && !nights.length) return '';
      var self = this;
      // Game nights come first: they are the marks a player acts on.
      var gn = nights.map(function (n) {
        return '<span class="mk gnm" data-ev="' + esc(self._gnId(n)) + '" title="' + esc(n.name) + '"><i class="fa-solid fa-dice-d20"></i></span>';
      });
      var cap = Math.max(0, 4 - gn.length), shown = evs.slice(0, cap), extra = evs.length - shown.length;
      var html = '<div class="marks">' + gn.join('') + shown.map(function (e) {
        var dirRing = self.canAuthorDmOnly && e.visibility === 'dm_only' ? ' dir' : '';
        var occ = CalDate.occurrenceOn(e, y, m, d), skipped = occ && occ.skipped;
        return '<span class="mk' + dirRing + (skipped ? ' skip' : '') + '" data-ev="' + esc(e.id) + '" style="' + eventColorStyle(e) + '" title="' + esc(e.name + (skipped ? ' (skipped)' : '')) + '">' + esc(eventGlyph(e)) + '</span>';
      }).join('');
      if (extra > 0) html += '<span class="more">+' + extra + '</span>';
      html += '</div>';
      return html;
    },

    _moonSilSVG: function (phase) {
      // A flat lunar-phase disc: a silhouette circle with a lit crescent
      // clipped by an ellipse whose horizontal radius encodes phase —
      // simpler than the mockups' seeded crater/sea texture (a real
      // reduction in fidelity, called out in .ai.md), same silhouette
      // idea and the same --sil* custom properties for blood-moon tint.
      // width/height keep it small until the stylesheet sizes it (see
      // moon_silhouette.templ).
      var path = MoonMath.litPath(phase, 6);
      return '<svg class="sil" width="14" height="14" viewBox="-7 -7 14 14" aria-hidden="true"><circle class="db" r="6"/>' + (path ? '<path class="dl" d="' + path + '"/>' : '') + '</svg>';
    },

    // --------------------------------------------------------------
    // The era field: calendar_era_blend.js paints the eras' colours behind
    // the shown month's days. Without that script it stays missing.
    // --------------------------------------------------------------
    _paintEraField: function (o) {
      var blend = Chronicle.calendarEraBlend, month = this.stageEl.querySelector('.month');
      if (!blend || !month) return;
      var self = this, cal = this.cal;
      if (!this.eraField) {
        this.eraField = blend.create(this.stageEl, {
          surface: function () { return getComputedStyle(self.calEl).backgroundColor; }
        });
        if ('ResizeObserver' in window) {
          this._eraRO = new ResizeObserver(function () { if (self.eraField && self._eraScene) self._paintEraField(); });
          this._eraRO.observe(this.stageEl);
        }
      }
      var info = EraMath.month(cal, EraMath.sorted(cal), this.view.y, this.view.m);
      var eras = [];
      if (info.first) eras.push(info.first.era);
      if (info.last && info.last !== info.first) eras.push(info.last.era);
      var band = !!month.querySelector('.band');
      var rowEls = $$(band ? '.band' : '.wk', month);
      var scene = {
        box: { left: month.offsetLeft, top: month.offsetTop, width: month.offsetWidth, height: month.offsetHeight },
        rows: rowEls.map(function (r) { return { top: r.offsetTop, height: r.offsetHeight }; }),
        cols: band ? 1 : CalDate.weekLen(cal),
        off: band ? 0 : (this._gridOff || 0),
        days: info.days,
        split: eras.length > 1 ? info.change : null,
        eras: eras.map(function (e) { return { key: e.id, color: e.color, color_2: e.color_2, style: e.style, feel: e.feel }; })
      };
      this._eraScene = scene;
      this.eraField.setLook(cal.era_look);
      this.eraField.setScene(scene, o);
    },

    // --------------------------------------------------------------
    // Eras in the header: the month's era is a chip (two, with an arrow,
    // when an era begins mid-month). Hover or focus shows a tooltip; a
    // press unfolds the era panel under the header, which pushes the grid
    // down rather than covering it.
    // --------------------------------------------------------------
    _eraRows: function () { return EraMath.sorted(this.cal); },
    _eraRow: function (id) { return this._eraRows().filter(function (r) { return String(r.era.id) === String(id); })[0] || null; },
    _eraSwatch: function (e) {
      var a = sanitizeColor(e.color) || '#888', b = sanitizeColor(e.color_2) || a;
      return 'background:linear-gradient(90deg,' + a + ',' + b + ')';
    },

    _renderEraChips: function () {
      var self = this, cal = this.cal, info = EraMath.month(cal, this._eraRows(), this.view.y, this.view.m);
      var chip = function (r, note) {
        var open = self._eraOpen != null && String(self._eraOpen) === String(r.era.id);
        return '<button type="button" class="erachip" data-era="' + esc(r.era.id) + '" aria-expanded="' + open + '" aria-controls="cal5-erapanel" aria-describedby="cal5-eratip">' +
          '<i class="esw" style="' + self._eraSwatch(r.era) + '"></i><span class="en">' + esc(r.era.name) + '</span>' +
          (EraMath.secret(cal, r) ? '<i class="fa-solid fa-eye-slash eh" aria-label="Hidden from players until it begins"></i>' : '') +
          (note ? '<small>' + esc(note) + '</small>' : '') +
          '<i class="fa-solid fa-chevron-down ev" aria-hidden="true"></i></button>';
      };
      var html = '';
      if (info.change) {
        html = chip(info.first, 'days 1–' + (info.change - 1)) + '<span class="earrow" aria-hidden="true">→</span>' + chip(info.last, 'from day ' + info.change);
      } else if (info.last) {
        html = chip(info.last, info.first ? '' : (info.begins.length ? 'from day ' + info.begins[0].day : ''));
      }
      this.eraChipsEl.innerHTML = html;
      this._markRibbon();
    },

    _bindEras: function () {
      var self = this, tipTimer = 0;
      var showTip = function (chip) {
        var r = self._eraRow(chip.dataset.era);
        if (!r) return;
        var w = EraMath.when(self.cal, r), tip = self.eraTipEl;
        tip.innerHTML = '<b>' + esc(r.era.name) + '</b><span>' + esc(w.range) + '</span><span>' + esc(w.len) +
          (EraMath.secret(self.cal, r) ? ' · hidden from players' : '') + '</span>';
        tip.hidden = false;
        var c = relRect(chip, self.calEl), cw = self.calEl.clientWidth, tw = tip.offsetWidth;
        tip.style.left = clampN(c.x, 8, Math.max(8, cw - tw - 8)) + 'px';
        tip.style.top = (c.y + c.h + 6) + 'px';
      };
      // A press or Escape puts the tooltip away until the pointer leaves the
      // chips: pressing re-renders the chips under a resting pointer, which
      // would otherwise bring it straight back over the opening panel.
      var tipMuted = false;
      this._hideEraTip = function (mute) { clearTimeout(tipTimer); self.eraTipEl.hidden = true; if (mute) tipMuted = true; };
      this.eraChipsEl.addEventListener('mouseover', function (e) {
        var chip = e.target.closest('.erachip');
        if (!chip || tipMuted) return;
        clearTimeout(tipTimer);
        tipTimer = setTimeout(function () { showTip(chip); }, 180);
      });
      this.eraChipsEl.addEventListener('mouseout', function (e) {
        if (!e.relatedTarget || !self.eraChipsEl.contains(e.relatedTarget)) { tipMuted = false; self._hideEraTip(); }
      });
      this.eraChipsEl.addEventListener('focusin', function (e) {
        var chip = e.target.closest('.erachip');
        // Focus handed back by Escape stays quiet; the next move shows it.
        if (tipMuted) { tipMuted = false; return; }
        if (chip && chip.matches(':focus-visible')) showTip(chip);
      });
      this.eraChipsEl.addEventListener('focusout', function () { self._hideEraTip(); });
      this.eraChipsEl.addEventListener('click', function (e) {
        var chip = e.target.closest('.erachip');
        if (!chip) return;
        self._hideEraTip(true);
        if (self._eraOpen != null && String(self._eraOpen) === chip.dataset.era) self.closeEraPanel({ refocus: chip });
        else self.openEraPanel(chip.dataset.era, chip);
      });
      this.eraPanelEl.addEventListener('click', function (e) {
        var t = e.target;
        if (t.closest('[data-era-close]')) { self.closeEraPanel({ refocus: true }); return; }
        var seg = t.closest('.rseg');
        if (seg) { self._eraJump(seg.dataset.era); return; }
        var ev = t.closest('[data-era-ev]');
        if (ev) { self._eraEventJump(ev); return; }
        if (t.closest('[data-era-all]')) { self._eraShowAll = true; self._renderEraEvents(); return; }
        if (t.closest('[data-era-manage]') && self.openEraManager) { self.closeEraPanel({ instant: true }); self.openEraManager(); }
      });
      var segTip = function (seg) {
        var r = self._eraRow(seg.dataset.era), tip = self.eraPanelEl.querySelector('.rtip');
        if (!r || !tip) return;
        tip.innerHTML = '<b>' + esc(r.era.name) + '</b><span>' + esc(EraMath.when(self.cal, r).range) + (EraMath.secret(self.cal, r) ? ' · hidden from players' : '') + '</span>';
        tip.hidden = false;
        var wrap = tip.parentNode, W = wrap.clientWidth, c = seg.offsetLeft + seg.offsetWidth / 2, tw = tip.offsetWidth;
        tip.style.left = clampN(c, tw / 2, Math.max(tw / 2, W - tw / 2)) + 'px';
      };
      var segHide = function () { var tip = self.eraPanelEl.querySelector('.rtip'); if (tip) tip.hidden = true; };
      this.eraPanelEl.addEventListener('mouseover', function (e) { var seg = e.target.closest('.rseg'); if (seg) segTip(seg); });
      this.eraPanelEl.addEventListener('mouseout', function (e) { if (e.target.closest('.rseg')) segHide(); });
      this.eraPanelEl.addEventListener('focusin', function (e) { var seg = e.target.closest('.rseg'); if (seg) segTip(seg); });
      this.eraPanelEl.addEventListener('focusout', segHide);
    },

    // openEraPanel unfolds the panel about one era, or switches an open
    // panel to it. The ribbon above the card shows every era the viewer may
    // see, with "you are here".
    openEraPanel: function (id, opener) {
      var r = this._eraRow(id), el = this.eraPanelEl, wasOpen = this._eraOpen != null;
      if (!r) return;
      this.hideGlance();
      if (this._hideEraTip) this._hideEraTip();
      this._eraOpen = r.era.id;
      this._eraOpener = opener || null;
      this._eraShowAll = false;
      var before = wasOpen ? el.offsetHeight : 0;
      el.innerHTML = this._eraPanelHTML(r);
      el.style.setProperty('--ea', sanitizeColor(r.era.color) || '#888');
      el.hidden = false;
      this._placeRibbonMarks();
      this._renderEraChips();
      this._loadEraEvents(r);
      var h = el.offsetHeight;
      stopAnims(el);
      if (!reducedMotion() && before !== h) {
        anim(el, [{ height: before + 'px', opacity: wasOpen ? 1 : 0 }, { height: h + 'px', opacity: 1 }], { duration: wasOpen ? 200 : 280, easing: 'cubic-bezier(.22,.8,.24,1)', fill: 'none' });
      }
      var head = el.querySelector('.ep-name');
      if (head && !wasOpen) head.focus({ preventScroll: true });
      if (this.eraField) this.eraField.wake();
      this._announce(r.era.name + ' era. ' + EraMath.when(this.cal, r).range + '.');
    },

    closeEraPanel: function (o) {
      o = o || {};
      var self = this, el = this.eraPanelEl;
      if (this._eraOpen == null) return;
      var opener = this._eraOpener, id = this._eraOpen;
      this._eraOpen = null;
      this._eraOpener = null;
      this._renderEraChips();
      var done = function () { el.hidden = true; el.innerHTML = ''; if (self.eraField) self._paintEraField(); };
      stopAnims(el);
      if (o.instant || reducedMotion()) done();
      else anim(el, [{ height: el.offsetHeight + 'px', opacity: 1 }, { height: '0px', opacity: 0 }], { duration: 220, easing: 'cubic-bezier(.22,.8,.24,1)' }).finished.then(done, done);
      if (o.refocus) {
        var back = o.refocus !== true ? o.refocus : (opener && opener.isConnected ? opener : this.eraChipsEl.querySelector('.erachip[data-era="' + id + '"]') || this.eraChipsEl.querySelector('.erachip'));
        if (back) back.focus({ preventScroll: true });
      }
    },

    _eraPanelHTML: function (r) {
      var self = this, cal = this.cal, list = this._eraRows(), shares = EraMath.ribbon(cal, list), w = EraMath.when(cal, r), e = r.era;
      var segs = list.map(function (x, i) {
        var a = sanitizeColor(x.era.color) || '#888', b = sanitizeColor(x.era.color_2) || a;
        return '<button type="button" class="rseg' + (EraMath.secret(cal, x) ? ' hid' : '') + (x === r ? ' cur' : '') + '" data-era="' + esc(x.era.id) + '" data-here="' + (shares[i].here == null ? '' : shares[i].here) + '"' +
          ' style="flex:' + shares[i].flex.toFixed(2) + ' 1 0px;--g:linear-gradient(90deg,' + a + ',' + b + ');--c:' + a + '"' +
          ' aria-label="' + esc(x.era.name + ', ' + EraMath.when(cal, x).range + '. Go to its first month.') + '"' + (x === r ? ' aria-current="true"' : '') + '></button>';
      }).join('');
      var desc = e.description ? String(e.description).split(/\n{2,}/).map(function (p) { return '<p>' + esc(p).replace(/\n/g, '<br>') + '</p>'; }).join('') : '';
      var lore = e.lore_entity_id && e.lore_entity_name
        ? '<a class="ep-lore" href="/campaigns/' + encodeURIComponent(this.campaignId) + '/entities/' + encodeURIComponent(e.lore_entity_id) + '"><span class="li" aria-hidden="true">' + PAGE_GLYPH + '</span><span class="lt">' + esc(e.lore_entity_name) + '<small>Lore page</small></span><span class="lg" aria-hidden="true">›</span></a>'
        : '';
      var note = e.dm_note ? '<div class="ep-note"><div class="nh"><i class="fa-solid fa-lock"></i>Director’s note, players never see this</div>' + esc(e.dm_note) + '</div>' : '';
      var manage = this.canAuthorDmOnly && this.openEraManager ? '<button type="button" class="ep-manage" data-era-manage><i class="fa-solid fa-pencil"></i>Edit eras</button>' : '';
      return '<div class="ep-ribbon"><div class="rtrack" role="group" aria-label="Eras, oldest to newest">' + segs + '</div><div class="raxis" aria-hidden="true"></div><div class="rtip" role="tooltip" hidden></div></div>' +
        '<div class="ep-card">' +
          '<div class="ep-head"><i class="esw" style="' + self._eraSwatch(e) + '"></i><div class="ep-main">' +
            '<h3 class="ep-name" tabindex="-1">' + esc(e.name) + '</h3>' +
            '<div class="ep-dates"><b>' + esc(w.range) + '</b><span>' + esc(w.len) + '</span></div>' +
            (EraMath.secret(cal, r) ? '<div class="ep-badge"><i class="fa-solid fa-eye-slash"></i>Hidden from players until it begins</div>' : '') +
          '</div><button type="button" class="ep-x" data-era-close aria-label="Close era"><i class="fa-solid fa-xmark"></i></button></div>' +
          '<div class="ep-body">' +
            '<div class="ep-col">' + (desc ? '<div class="ep-desc">' + desc + '</div>' : '') + lore + note + manage + '</div>' +
            '<div class="ep-col ep-events" aria-live="polite"><div class="ep-sh"><span>Key events</span><span class="cnt"></span></div><p class="ep-none">Loading…</p></div>' +
          '</div>' +
        '</div>';
    },

    // The ribbon's year labels and "you are here" need the laid-out widths.
    _placeRibbonMarks: function () {
      var wrap = this.eraPanelEl.querySelector('.ep-ribbon');
      if (!wrap) return;
      var cal = this.cal, ax = wrap.querySelector('.raxis'), W = wrap.clientWidth, prevRight = -99, self = this;
      ax.innerHTML = '';
      $$('.rhere', wrap).forEach(function (n) { n.remove(); });
      $$('.rseg', wrap).forEach(function (seg, p) {
        var r = self._eraRow(seg.dataset.era), x = seg.offsetLeft;
        if (!r) return;
        var lab = document.createElement('span');
        lab.textContent = (p === 0 ? 'Year ' : '') + r.start.y;
        ax.appendChild(lab);
        lab.style.left = x + 'px';
        if (x < prevRight + 8) lab.remove(); else prevRight = x + lab.offsetWidth;
        if (seg.dataset.here !== '') {
          var hx = x + seg.offsetWidth * +seg.dataset.here, h = document.createElement('div'), t = document.createElement('span');
          h.className = 'rhere'; t.textContent = 'you are here';
          h.appendChild(t); h.style.left = hx + 'px'; wrap.appendChild(h);
          t.style.left = Math.max(-hx, Math.min(W - hx - t.offsetWidth, -t.offsetWidth / 2)) + 'px';
        }
      });
      this._markRibbon();
    },
    // The eras the shown month belongs to are outlined on the ribbon.
    _markRibbon: function () {
      if (!this.eraPanelEl || this._eraOpen == null) return;
      var info = EraMath.month(this.cal, this._eraRows(), this.view.y, this.view.m), ids = {};
      if (info.first) ids[info.first.era.id] = 1;
      if (info.last) ids[info.last.era.id] = 1;
      $$('.rseg', this.eraPanelEl).forEach(function (s) { s.classList.toggle('showing', !!ids[s.dataset.era]); });
    },

    _eraJump: function (id) {
      var r = this._eraRow(id);
      if (!r) return;
      if (this.wingFor) this.closeWing({ instant: true, quiet: true });
      this.view = { y: r.start.y, m: r.start.m };
      this._eraFade = true;
      this.renderMonth();
      this.openEraPanel(r.era.id, this.eraChipsEl.querySelector('.erachip[data-era="' + r.era.id + '"]'));
      var seg = this.eraPanelEl.querySelector('.rseg[data-era="' + r.era.id + '"]');
      if (seg) seg.focus({ preventScroll: true });
    },

    _loadEraEvents: function (r) {
      var self = this, id = r.era.id;
      this._eraEvents = null;
      Chronicle.apiFetch(this.apiBase + '/eras/' + encodeURIComponent(id) + '/events')
        .then(function (resp) { return resp.ok ? resp.json() : null; })
        .then(function (body) {
          if (String(self._eraOpen) !== String(id)) return;
          self._eraEvents = body && Array.isArray(body.data) ? body : { data: [], total: 0, failed: !body };
          self._renderEraEvents();
        })
        .catch(function () {
          if (String(self._eraOpen) !== String(id)) return;
          self._eraEvents = { data: [], total: 0, failed: true };
          self._renderEraEvents();
        });
    },

    // Key events are the era's major-tier events; without any, its latest
    // events stand in. "Show all" lists everything the read returned.
    _renderEraEvents: function () {
      var box = this.eraPanelEl.querySelector('.ep-events'), body = this._eraEvents, cal = this.cal;
      if (!box || !body) return;
      var evs = body.data, keys = evs.filter(function (e) { return e.tier === 'major'; });
      var shown = this._eraShowAll ? evs : (keys.length ? keys : evs).slice(-4);
      var cnt = body.total + (body.total === 1 ? ' event' : ' events') + (keys.length ? ', ' + keys.length + ' key' : '');
      var h = '<div class="ep-sh"><span>Key events</span><span class="cnt">' + (body.failed ? '' : esc(cnt)) + '</span></div>';
      if (body.failed) h += '<p class="ep-none">Could not load this era’s events.</p>';
      else if (!evs.length) h += '<p class="ep-none">Nothing recorded in this era yet.</p>';
      else {
        h += '<ul class="ep-evs">' + shown.map(function (e) {
          return '<li><button type="button" data-era-ev="' + esc(e.id) + '" data-y="' + e.year + '" data-m="' + e.month + '" data-d="' + e.day + '">' +
            '<span class="t">' + esc(e.name) + (e.tier === 'major' ? '<span class="k" aria-label="key event">◆</span>' : '') + '</span>' +
            '<span class="w">' + esc(EraMath.fmt(cal, { y: e.year, m: e.month, d: e.day })) + '</span></button></li>';
        }).join('') + '</ul>';
        if (!this._eraShowAll && evs.length > shown.length) h += '<button type="button" class="ep-all" data-era-all>Show all ' + evs.length + ' events</button>';
        if (body.total > evs.length) h += '<p class="ep-none">Showing the first ' + evs.length + '.</p>';
      }
      box.innerHTML = h;
    },

    // An event in the panel goes to its day: the panel folds away and the
    // day's card opens on that event.
    _eraEventJump: function (btn) {
      var self = this, y = +btn.dataset.y, m = +btn.dataset.m, d = +btn.dataset.d, id = btn.dataset.eraEv;
      this.closeEraPanel({ instant: true });
      if (this.wingFor) this.closeWing({ instant: true, quiet: true });
      this.view = { y: y, m: m };
      this.renderMonth();
      this.fetchMonth(y, m).then(function () {
        if (self.view.y !== y || self.view.m !== m) return;
        self.openWing(dayKey(y, m, d), id);
      });
    },

    // The label on an era's first day: it opens the era panel too.
    _eraStartHTML: function (y, m, d) {
      var cal = this.cal, k = CalDate.dayIndex(cal, y, m, d);
      var r = this._eraRows().filter(function (x) { return x.startK === k; }).pop();
      if (!r) return '';
      var b = sanitizeColor(r.era.color_2) || sanitizeColor(r.era.color) || '#888';
      return '<span class="erastart" data-era-start="' + esc(r.era.id) + '" style="--es:' + b + '" title="' + esc(r.era.name) + ' begins"><span class="esl">' + esc(r.era.name) + ' begins</span><span class="ess" aria-hidden="true">New era</span></span>';
    },

    // --------------------------------------------------------------
    // Navigation
    // --------------------------------------------------------------
    goMonth: function (delta) {
      // A day's card belongs to its day; changing the month puts it away.
      if (this.wingFor) this.closeWing({ instant: true, quiet: true });
      var next = CalDate.shiftMonth(this.cal, this.view.y, this.view.m, delta);
      this.view = next;
      this._eraFade = true;
      this.renderMonth();
    },

    goToday: function () {
      if (this.wingFor) this.closeWing({ instant: true, quiet: true });
      this.view = { y: this.cal.current_year, m: this.cal.current_month };
      this._eraFade = true;
      this.renderMonth();
      this.say('Jumped to today.');
    },

    // --------------------------------------------------------------
    // Event wiring
    // --------------------------------------------------------------
    _bindEvents: function () {
      var self = this;
      $('#cal5-prev', this.el).addEventListener('click', function () { self.goMonth(-1); });
      $('#cal5-next', this.el).addEventListener('click', function () { self.goMonth(1); });
      $('#cal5-todaybtn', this.el).addEventListener('click', function () { self.goToday(); });
      $('#cal5-hub', this.el).addEventListener('click', function () { self.toggleHub(); });
      this._bindEras();
      $('#cal5-moonbtn', this.el).addEventListener('click', function () { self.openMoonView(); });
      var freeBtn = $('#cal5-freebtn', this.el);
      if (freeBtn) freeBtn.addEventListener('click', function () {
        if (self.fvEl.classList.contains('open')) self.closeFreeView(); else self.openFreeView();
      });
      this.fvEl.addEventListener('click', function (e) { self._fvHandleClick(e); });
      this.fvEl.addEventListener('submit', function (e) {
        var form = e.target.closest('[data-away-form]');
        if (!form) return;
        e.preventDefault();
        self._awaySubmit(form, function () { self.refreshFreeView(); });
      });
      this.fvEl.addEventListener('change', function (e) {
        if (e.target.matches('[data-fv-lines]')) self.setShowFree(e.target.checked);
      });
      this.scrimEl.addEventListener('click', function () { self.closeAllPanels(); });

      this.stageEl.addEventListener('click', function (e) {
        var es = e.target.closest('[data-era-start]');
        if (es && !self.calEl.classList.contains('editing')) {
          e.stopPropagation();
          self.openEraPanel(es.dataset.eraStart, es.closest('.day'));
          return;
        }
        var mk = e.target.closest('.mk');
        var msil = e.target.closest('.msil');
        var day = e.target.closest('.day');
        if (mk) { self.openWing(day.dataset.key, mk.dataset.ev); return; }
        if (msil) { self.openMoonView(); return; }
        if (day && day.dataset.key) self.openWing(day.dataset.key);
      });

      // Hover glance cards: ~140ms grow-from-anchor after a short rest,
      // gone the instant the pointer leaves (mousleave, not mouseout, so
      // moving onto a child doesn't retrigger). The card itself carries
      // pointer-events:none (calendar-view.css), so it can never "catch"
      // the pointer.
      var hoverTimer = null;
      this.stageEl.addEventListener('mouseover', function (e) {
        var mk = e.target.closest('.mk, .msil, .avl');
        if (!mk || mk === self._hoverAnchor) return;
        clearTimeout(hoverTimer);
        self._hoverAnchor = mk;
        hoverTimer = setTimeout(function () { self.showGlance(mk); }, 350);
      });
      this.stageEl.addEventListener('mouseleave', function () {
        clearTimeout(hoverTimer);
        self._hoverAnchor = null;
        self.hideGlance();
      }, true);

      this.wingEl.addEventListener('submit', function (e) {
        var form = e.target.closest('[data-plan-form]');
        if (form) { e.preventDefault(); self._submitPlan(form); return; }
        if (e.target.closest('[data-myday]')) e.preventDefault();
      });
      this.wingEl.addEventListener('click', function (e) {
        if (e.target.closest('[data-close]')) { if (!self._rollHolds()) self.closeWing(); return; }
        var roll = e.target.closest('[data-roll-one]');
        if (roll) { self._toggleRoll(roll); return; }
        var plan = e.target.closest('[data-plan]');
        if (plan) { self._openPlan(plan.dataset.plan, parseInt(plan.dataset.planAt, 10) || 19); return; }
        if (e.target.closest('[data-plan-cancel]')) { self._closePlan(); return; }
        if (e.target.closest('[data-pl-mine]')) { self.openPlanner({ paint: true }); return; }
        if (self._myDayClick(e)) return;
        var gnNew = e.target.closest('[data-gn-new]');
        if (gnNew) { self._gnOpenEditor(null, gnNew.dataset.gnNew); return; }
        var more = e.target.closest('[data-plan-more]');
        if (more) { self._planMore(more.closest('[data-plan-form]')); return; }
        if (self._gnHandleClick(e)) return;
        if (e.target.closest('[data-sky-day]')) { var sd = parseDayKey(self.wingFor); self.previewSky(sd.y, sd.m, sd.d); return; }
        var mr = e.target.closest('.mrow');
        if (mr) { self.mvMoonId = mr.dataset.moon; self.openMoonView(); return; }
        var evd = e.target.closest('.evd');
        if (evd) self.openEventDetail(evd.dataset.ev);
      });
      this.flapEl.addEventListener('click', function (e) {
        if (e.target.closest('[data-close]')) self.closeFlap();
      });
      // A desktop card scrolls in its second leaf, so the wheel over its
      // head and the scrolling keys on the card itself move that leaf.
      [this.wingEl, this.flapEl].forEach(function (card) {
        var scroller = function () { return (!isPhone() && card.querySelector('.lscroll')) || card; };
        card.addEventListener('wheel', function (e) {
          var sc = scroller();
          if (sc === card || e.ctrlKey || sc.contains(e.target) || sc.scrollHeight <= sc.clientHeight) return;
          e.preventDefault();
          sc.scrollTop += e.deltaY * (e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? sc.clientHeight : 1);
        }, { passive: false });
        card.addEventListener('keydown', function (e) {
          var sc = scroller();
          if (sc === card || e.target !== card || e.altKey || e.ctrlKey || e.metaKey) return;
          var k = e.key, pg = sc.clientHeight * 0.875;
          var d = k === 'ArrowDown' ? 40 : k === 'ArrowUp' ? -40 : k === 'PageDown' || (k === ' ' && !e.shiftKey) ? pg : k === 'PageUp' || k === ' ' ? -pg : k === 'End' ? sc.scrollHeight : k === 'Home' ? -sc.scrollHeight : 0;
          if (!d) return;
          e.preventDefault();
          sc.scrollBy({ top: d, behavior: reducedMotion() ? 'auto' : 'smooth' });
        });
      });
      this.evpEl.addEventListener('click', function (e) {
        if (e.target.closest('[data-close]')) { self.closeEventDetail(); return; }
        if (e.target.closest('[data-gn-edit]')) { self._gnOpenEditor(self._findNight(self.evpEl.dataset.ev)); return; }
        if (self._gnExtraClick(e)) return;
        self._gnHandleClick(e);
      });
      this.evpEl.addEventListener('input', function (e) {
        if (e.target.matches('[data-gn-link-q]')) self._gnLinkSearch(e.target);
      });
      // Enter saves a game night's note; Escape puts the note away without
      // closing the card around it.
      [this.wingEl, this.evpEl].forEach(function (card) {
        card.addEventListener('keydown', function (e) {
          if (!e.target.matches || !e.target.matches('[data-gn-note-input]')) return;
          var box = e.target.closest('.rsvp[data-gnr]');
          if (e.key === 'Enter') { e.preventDefault(); box.querySelector('[data-gn-note="save"]').click(); }
          else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); box.querySelector('[data-gn-note="cancel"]').click(); }
        });
      });
      this.mvEl.addEventListener('click', function (e) {
        if (e.target.closest('[data-close]')) self.closeMoonView();
        var strip = e.target.closest('.msi');
        if (strip) self.selectMoon(strip.dataset.moon);
      });
      this.popEl.addEventListener('click', function (e) {
        if (e.target.closest('[data-close]')) self.closePop();
      });
      this.popEl.addEventListener('submit', function (e) {
        var form = e.target.closest('[data-today-form]');
        if (!form) return;
        e.preventDefault();
        var d = self._readTodayForm(form), err = form.querySelector('.sterr'), go = form.querySelector('[type="submit"]');
        if (d.error) { err.textContent = d.error; err.hidden = false; return; }
        go.disabled = true;
        self._saveToday(d).then(function (msg) {
          go.disabled = false;
          if (msg) { err.textContent = msg; err.hidden = false; return; }
          self.closePop();
        });
      });
      $('#cal5-todaypill', this.el).addEventListener('click', function (e) {
        var step = e.target.closest('[data-today-step]');
        if (step) { self._stepToday(parseInt(step.dataset.todayStep, 10)); return; }
        var set = e.target.closest('[data-today-set]');
        if (!set) return;
        if (self.popEl.classList.contains('open')) { self.closePop(); return; }
        self.showSetToday(set);
      });

      // Keyboard: arrows move the roving day, T jumps to today, Page
      // Up/Down change month, Enter/Space open the focused day.
      this.stageEl.addEventListener('keydown', function (e) {
        var day = e.target.closest('.day');
        if (!day) return;
        var handled = true;
        switch (e.key) {
          case 'ArrowRight': self._moveRoving(1); break;
          case 'ArrowLeft': self._moveRoving(-1); break;
          case 'ArrowDown': self._moveRoving(CalDate.weekLen(self.cal)); break;
          case 'ArrowUp': self._moveRoving(-CalDate.weekLen(self.cal)); break;
          case 'Enter': case ' ': day.click(); break;
          case 't': case 'T': self.goToday(); break;
          case 'PageUp': self.goMonth(e.shiftKey ? -12 : -1); break;
          case 'PageDown': self.goMonth(e.shiftKey ? 12 : 1); break;
          default: handled = false;
        }
        if (handled) e.preventDefault();
      });

      this._resizeHandler = function () {
        if (self.wingFor) self.closeWing({ instant: true, quiet: true });
        if (self._pf.state !== 'closed') self.closeFlap({ instant: true, quiet: true });
        if (self._hideEraTip) self._hideEraTip();
        self._placeRibbonMarks();
      };
      window.addEventListener('resize', this._resizeHandler);

      // Escape closes whichever panel is open (motion rule #760: "every
      // panel works from the keyboard"), innermost first: the event detail
      // overlay before the day wing it grew out of, so Escape steps back
      // one layer at a time rather than closing everything at once.
      this._escHandler = function (e) {
        if (e.key !== 'Escape') return;
        if (self._hideEraTip) self._hideEraTip(true);
        if (self.plannerOpen()) { self.closePlanner(); return; }
        if (self.evpEl.classList.contains('open')) { self.closeEventDetail(); return; }
        if (self.mvEl.classList.contains('open')) { self.closeMoonView(); return; }
        if (self.fvEl.classList.contains('open')) { self.closeFreeView(); return; }
        if (self._pf.state !== 'closed') { self.closeFlap(); return; }
        if (self.popEl.classList.contains('open')) { self.closePop(); return; }
        if (self._roll) { if (!self._rollHolds()) self._roll.close(); return; }
        if (self.wingFor) { self.closeWing(); return; }
        if (self._eraOpen != null) { self.closeEraPanel({ refocus: true }); return; }
      };
      document.addEventListener('keydown', this._escHandler);

      // The moon and Who's free views sit over the month, so a press
      // anywhere else in the calendar closes them, the way the day and era
      // cards give way to a press on the grid; presses that open them again
      // are left to do that. The day card also gives way to a press
      // outside the calendar, so it never traps the page, unless it holds
      // a form being filled in. A press on another day is the grid's own:
      // it moves the card.
      this._mvOffHandler = function (e) {
        var t = e.target;
        if (!(t instanceof Element) || self.evpEl.contains(t)) return;
        var inside = self.el.contains(t);
        if (inside && self.mvEl.classList.contains('open') && !self.mvEl.contains(t) && !t.closest('#cal5-moonbtn, .msil, .mrow')) self.closeMoonView();
        if (inside && self.fvEl.classList.contains('open') && !self.fvEl.contains(t) && !t.closest('#cal5-freebtn') && !self._awayHolds(function () { self.refreshFreeView(); })) self.closeFreeView();
        if (self.popEl.classList.contains('open') && !self.popEl.contains(t) && !t.closest('#cal5-hub, #cal5-todaypill')) self._popOff();
        if (!self.wingFor || self.wingEl.contains(t) || self.wingEl.querySelector('form')) return;
        if ((inside ? !t.closest('.day[data-key], #cal5-freebtn') :
            !t.closest('dialog, [role="dialog"], [aria-modal="true"]')) && !self._rollHolds()) self.closeWing();
      };
      document.addEventListener('pointerdown', this._mvOffHandler, true);
    },

    _moveRoving: function (delta) {
      var buttons = $$('.day[data-key]', this.stageEl);
      var idx = buttons.findIndex(function (b) { return b === document.activeElement; });
      var next = buttons[Math.max(0, Math.min(buttons.length - 1, (idx < 0 ? 0 : idx) + delta))];
      if (next) next.focus({ preventScroll: false });
    },

    anyPanelOpen: function () {
      return !!this.wingFor || this._pf.state !== 'closed' || this.evpEl.classList.contains('open') ||
        this.mvEl.classList.contains('open') || this.popEl.classList.contains('open') || this.fvEl.classList.contains('open');
    },

    _updateScrim: function () {
      this.scrimEl.classList.toggle('on', this.anyPanelOpen() && window.matchMedia('(max-width:600px)').matches);
    },

    // o.instant takes them down without their motion.
    closeAllPanels: function (o) {
      if (this.wingFor) this.closeWing(o);
      this.closeEraPanel({ instant: o && o.instant });
      this.closeFlap(o);
      this.closeEventDetail();
      this.closeMoonView();
      this.closeFreeView();
      this.closePlanner(true);
      this.closePop();
      if (window.Chronicle && Chronicle.calendarGameNight) Chronicle.calendarGameNight.close(this, true);
    },

    // --------------------------------------------------------------
    // The day's card. A second press on its day folds it back; another
    // day's press folds this one away first. A press that comes while a
    // card is moving waits for the motion to finish.
    // --------------------------------------------------------------
    openWing: function (key, highlightEventId) {
      var self = this, P = this._pw;
      this.hideGlance();
      if (P.state === 'opening' || P.state === 'closing') { P.next = function () { self.openWing(key, highlightEventId); }; return; }
      if (this.wingFor === key) {
        if (highlightEventId) this._pickRow(highlightEventId); else if (!this._rollHolds()) this.closeWing();
        return;
      }
      if (this.wingFor && this._rollHolds()) return;
      var go = function () { if (!self.wingFor) self._showWing(key, highlightEventId); };
      if (this.wingFor) this.closeWing({ quiet: true, fast: true }).then(go);
      else if (this._pf.state !== 'closed') this.closeFlap({ quiet: true }).then(go);
      else go();
    },

    _showWing: function (key, highlightEventId) {
      var self = this, P = this._pw, btn = this.stageEl.querySelector('.day[data-key="' + key + '"]');
      if (!btn) return;
      resetCard(this.wingEl);
      this.wingEl.innerHTML = this._wingHTML(parseDayKey(key), highlightEventId);
      this.wingFor = key;
      this._markOpenDay(btn);
      var g = isPhone() ? null : wingGeometry(this.wingEl, btn, this.calEl);
      if (g) placeCard(this.wingEl, g);
      var row = highlightEventId ? this.wingEl.querySelector('.evd[data-ev="' + highlightEventId + '"]') : null;
      if (row) row.scrollIntoView({ block: 'nearest' });
      var depth = pressDepth(btn), dc = btn.querySelector('.dc');
      P.src = btn;
      P.g = g;
      P.press = [{ el: dc, depth: depth }, { el: btn, pseudo: '::before', depth: depth }];
      // The day's own marks go into its card, the pressed mark first; the
      // card's icons wait for them.
      P.pairs = [];
      P.carried = false;
      if (!reducedMotion() && btn.querySelector('.mk')) {
        P.pairs = carryPairs(btn, this.wingEl);
        if (highlightEventId) P.pairs.sort(function (a, b) { return (b.id === highlightEventId) - (a.id === highlightEventId); });
        P.pairs.forEach(function (p) { p.ric.classList.add('wait'); });
        packMarks(btn, P.pairs, false);
        P.carried = true;
      }
      this._updateScrim();
      openCard(P, this.calEl).then(function (ok) {
        if (!ok) return;
        if (self._wingStale === self.wingFor) { self._wingStale = null; self.refreshWing(); }
        self.wingEl.focus({ preventScroll: true });
        if (row && row.isConnected) row.classList.add('pulse');
        var n = P.next; P.next = null; if (n) n();
      });
    },

    // The open day turns to paper and the rest of the month recedes.
    _markOpenDay: function (btn) {
      btn.classList.add('sel');
      btn.setAttribute('aria-expanded', 'true');
      var m = btn.closest('.month');
      if (m) m.classList.add('dim');
    },

    // A mark of the day that is already open picks that event out.
    _pickRow: function (id) {
      $$('.evd.hl', this.wingEl).forEach(function (r) { r.classList.remove('hl', 'pulse'); });
      var row = this.wingEl.querySelector('.evd[data-ev="' + id + '"]');
      if (!row) return;
      row.classList.add('hl');
      row.scrollIntoView({ block: 'nearest' });
      void row.offsetWidth;
      row.classList.add('pulse');
    },

    // Redraws the open card's words in place (after an edit or a fetch
    // re-renders the month), keeping where its list was scrolled to.
    // A read that lands while the card is still unfolding would be dropped
    // by refreshWing, so it waits for the card to finish opening instead.
    _refreshWingOnceOpen: function () {
      if (this._pw.state === 'opening') this._wingStale = this.wingFor;
      else this.refreshWing();
    },

    refreshWing: function () {
      // Never under a form being filled in: its own save refreshes after.
      // Nor under an open roller: it refreshes when it closes.
      if (!this.wingFor || this._pw.state !== 'open' || this.wingEl.querySelector('form') || this._roll) return;
      var sc = this.wingEl.querySelector('.lscroll') || this.wingEl, top = sc.scrollTop;
      this.wingEl.innerHTML = this._wingHTML(parseDayKey(this.wingFor));
      (this.wingEl.querySelector('.lscroll') || this.wingEl).scrollTop = top;
      var btn = this.stageEl.querySelector('.day[data-key="' + this.wingFor + '"]');
      if (btn) { this._pw.src = btn; this._markOpenDay(btn); }
    },

    // Folds the card back into its day. o.fast when another day is
    // waiting, o.instant without motion, o.quiet to leave focus alone.
    closeWing: function (o) {
      o = o || {};
      var self = this, P = this._pw;
      if (!this.wingFor) return Promise.resolve();
      this._planFor = null;
      if (P.state === 'opening' && !o.instant) return new Promise(function (res) { P.next = function () { self.closeWing(o).then(res); }; });
      if (P.state === 'closing' && !o.instant) return P.closing || Promise.resolve();
      if (this.evpEl.classList.contains('open')) this.closeEventDetail();
      var btn = P.src, had = this.wingEl.contains(document.activeElement) || this.evpEl.contains(document.activeElement);
      P.closing = closeCard(P, this.calEl, o).then(function (ok) {
        if (!ok) return;
        if (btn) { btn.classList.remove('sel'); btn.removeAttribute('aria-expanded'); }
        $$('.day.sel', self.stageEl).forEach(function (d) { d.classList.remove('sel'); d.removeAttribute('aria-expanded'); });
        $$('.month.dim', self.stageEl).forEach(function (m) { m.classList.remove('dim'); });
        self.wingEl.innerHTML = '';
        self._roll = null;
        self.wingFor = null;
        P.closing = null;
        self._updateScrim();
        if (!o.quiet && had && btn && btn.isConnected) btn.focus({ preventScroll: true });
        var n = P.next; P.next = null; if (n) n();
      });
      this._updateScrim();
      return P.closing;
    },

    // What an event's repeat says in words: a rule as its sentence, a plain
    // repeat as its type.
    repeatText: function (ev) {
      var R = Chronicle.calendarRule;
      if (ev.recurrence_type === 'rule') {
        if (!ev.recurrence_rule || !R) return 'Repeats by a rule';
        return 'Repeats: ' + R.summaryInline(R.fromRule(ev.recurrence_rule), this.ruleEnv(ev.id));
      }
      return 'Repeats ' + ev.recurrence_type;
    },

    // What the rule editor and sentences may name on this calendar: its
    // moons, seasons, weekdays, months and the events seen so far (the
    // months visited; the API lists events a month at a time). An event
    // that repeats relative to another cannot be an anchor, so it is not
    // offered, and neither is the event being edited.
    ruleEnv: function (exceptId) {
      var self = this, seen = {}, all = [], names = this._eventNames || (this._eventNames = {});
      Object.keys(this.eventsByMonth).forEach(function (k) {
        (self.eventsByMonth[k] || []).forEach(function (e) {
          if (seen[e.id]) return;
          seen[e.id] = true;
          names[e.id] = e.name;
          all.push(e);
        });
      });
      var events = all.filter(function (e) {
        if (e.id === exceptId) return false;
        var r = e.recurrence_type === 'rule' && e.recurrence_rule;
        return !(r && (r.match || []).some(function (c) { return c.kind === 'relative_to_event' || c.kind === 'after_event'; }));
      }).map(function (e) { return { id: e.id, name: e.name }; });
      return { cal: this.cal, events: events, hiddenOk: this.canAuthorDmOnly === true, eventName: function (id) { return names[id]; } };
    },

    // "14 Harvestide" (with the year when it is not the calendar's own).
    _dateLabel: function (y, m, d) {
      var cal = this.cal, mo = (cal.months || [])[m - 1];
      return d + ' ' + (mo ? mo.name : m) + (y !== cal.current_year ? ' ' + y : '');
    },

    // "Add an event", with "Roll one" beside it for the DM team: rolling a
    // happening onto a day makes a DM-only event, so only they get it.
    _addRowHTML: function () {
      var add = '<button type="button" class="addev" data-add-event><i class="fa-solid fa-plus"></i>Add an event</button>';
      if (!this.canAuthorDmOnly || !(window.Chronicle && Chronicle.RollTables)) return add;
      return '<div class="rtd-open">' + add + '<button type="button" class="addev" data-roll-one aria-expanded="false"><i class="fa-solid fa-dice"></i>Roll one</button></div>';
    },

    // _rollHolds keeps the day card open while it holds a roll not yet put
    // on the day: the card shakes and says so instead of closing.
    _rollHolds: function () {
      if (this._roll && this._roll.isDirty()) { this._roll.nag(); return true; }
      return false;
    },

    // _toggleRoll opens the roller under the day card's "Roll one", or puts
    // it away again (unless it holds a roll).
    _toggleRoll: function (btn) {
      var self = this;
      if (this._roll) { if (!this._rollHolds()) this._roll.close(); return; }
      var d = parseDayKey(this.wingFor), cal = this.cal;
      var mc = CalDate.monthCount(cal), monthDef = (cal.months || [])[((d.m - 1) % mc + mc) % mc];
      var label = (monthDef ? monthDef.name : d.m) + ' ' + d.d;
      var host = document.createElement('div');
      btn.closest('.rtd-open').insertAdjacentElement('afterend', host);
      btn.setAttribute('aria-expanded', 'true');
      var handle = Chronicle.RollTables.mountDay(host, {
        campaignId: this.campaignId,
        calendar: cal,
        date: { year: d.y, month: d.m, day: d.d },
        why: function (r) {
          var abs = CalDate.dayIndex(cal, d.y, d.m, d.d), w = '';
          var moon = (r.moon && (cal.moons || []).filter(function (mo) { return mo.name === r.moon; })[0]) || self.mainMoon();
          if (moon) w += '<em><i class="fa-solid fa-moon"></i> ' + esc(moon.name) + ': ' + MoonMath.name(MoonMath.phase(moon, abs)) + '</em>';
          var season = self.seasonForDate(d.m, d.d);
          if (season) w += '<em><i class="fa-solid fa-leaf"></i> ' + esc(season.name) + '</em>';
          return w;
        },
        onPut: function (r) {
          var body = {
            name: r.name, description: r.brief || null,
            description_html: r.brief ? '<p>' + esc(r.brief) + '</p>' : null,
            year: d.y, month: d.m, day: d.d, all_day: true, visibility: 'dm_only'
          };
          return Chronicle.apiFetch(self.apiBase + '/events', { method: 'POST', body: body }).then(function (resp) {
            if (!resp.ok) throw new Error('not saved');
            self.eventsByMonth = {};
            self.renderMonth();
            self.say('On ' + label + '. Players won’t see it until you show it.');
          });
        },
        onClose: function () {
          host.remove();
          if (self._roll !== handle) return;
          self._roll = null;
          self.refreshWing();
        }
      });
      this._roll = handle;
    },

    _wingHTML: function (d, highlightEventId) {
      var self = this, cal = this.cal;
      var mc = CalDate.monthCount(cal), m0 = ((d.m - 1) % mc + mc) % mc, monthDef = (cal.months || [])[m0];
      var label = (monthDef ? monthDef.name : d.m) + ' ' + d.d + ', ' + d.y;
      var isToday = d.y === cal.current_year && d.m === cal.current_month && d.d === cal.current_day;
      var isFuture = CalDate.dayIndex(cal, d.y, d.m, d.d) > CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
      var reading = this.weatherOnDay(d.y, d.m, d.d), fc = this.forecastOn(d.y, d.m, d.d);
      var isForecast = !this.canAuthorDmOnly && isFuture && !reading && !!fc;
      var weatherFact = isForecast ? forecastFactHTML(fc) : weatherFactHTML(reading, isFuture) + (this.canAuthorDmOnly && fc ? playersSeeHTML(fc) : '');
      var moonRows = (cal.moons || []).map(function (mo) {
        var abs = CalDate.dayIndex(cal, d.y, d.m, d.d), phase = MoonMath.phase(mo, abs);
        return '<button type="button" class="mrow" data-moon="' + esc(mo.id) + '"><span class="mt"><b>' + esc(mo.name) + '</b><small>' + esc(MoonMath.name(phase)) + ' · ' + MoonMath.litPct(phase) + '% lit</small></span><i class="fa-solid fa-chevron-right chev"></i></button>';
      }).join('');

      var evs = this.eventsOnDay(d.y, d.m, d.d);
      var evHTML = evs.length ? evs.map(function (e) {
        var hl = highlightEventId && e.id === highlightEventId ? ' hl' : '';
        var time = e.all_day ? '' : (e.start_hour != null ? pad2(e.start_hour) + ':' + pad2(e.start_minute || 0) : '');
        var occ = CalDate.occurrenceOn(e, d.y, d.m, d.d) || {};
        var note = occ.skipped ? 'Skipped. Players don’t see it.' : (occ.moved_from ? 'Moved from ' + self._dateLabel(occ.moved_from.year, occ.moved_from.month, occ.moved_from.day) : '');
        return '<div class="evd go' + hl + (occ.skipped ? ' skip' : '') + '" data-ev="' + esc(e.id) + '">' +
          '<div class="evh"><span class="ric" style="' + eventColorStyle(e) + '">' + esc(eventGlyph(e)) + '</span><span class="qt0">' + esc(e.name) + '</span></div>' +
          (time || note ? '<div class="evm">' + (time ? '<span>' + esc(time) + '</span>' : '') + (note ? '<span>' + esc(note) + '</span>' : '') + '</div>' : '') +
          '</div>';
      }).join('') : '<div class="none">Nothing on the calendar today.</div>';
      var self = this, nights = this.nightsOnDay(d.y, d.m, d.d);
      var gnHTML = nights.length ? '<div class="sect">' + (nights.length > 1 ? nights.length + ' game nights' : 'Game night') + '</div>' +
        '<div class="evlist">' + nights.map(function (n) { return self._gnBoxHTML(n, highlightEventId); }).join('') + '</div>' : '';
      if (nights.length && !evs.length) evHTML = '';

      // Two leaves: the head (the date and the weather), which stays in
      // view, and the rest, which scrolls on a desktop: the day's events
      // first, then its moons.
      var moonCount = (cal.moons || []).length;
      return '<div class="leaf lf1"><div class="grab" aria-hidden="true"></div>' +
          '<div class="crease"><span>Day</span><button type="button" class="x" data-close aria-label="Close">✕</button></div>' +
          '<div class="wb wbh"><h3 class="wdate">' + esc(label) + '</h3>' + (isForecast ? '<span class="wxtag fctag">Forecast</span>' : '') +
          (this.canAuthorDmOnly && this.skyDock ? '<button type="button" class="skyday" data-sky-day><i class="fa-solid fa-cloud-sun"></i>See this day’s sky</button>' : '') +
          (weatherFact ? '<div class="facts">' + weatherFact + '</div>' : '') + '</div>' +
        '</div>' +
        '<div class="leaf lf2"><div class="lscroll"><div class="wb wbr">' + gnHTML + this._freeWingHTML(d) +
          (evHTML ? '<div class="sect">' + (evs.length > 1 ? evs.length + ' events' : 'Events') + '</div>' +
          '<div class="evlist">' + evHTML + '</div>' : '') +
          this._gnAddHTML(d) + (this.canEdit ? this._addRowHTML() : '') +
          (moonRows ? '<div class="sect">' + (moonCount > 1 ? 'Moons' : 'Moon') + '</div><div class="moonsec">' + moonRows + '</div>' : '') +
        '</div></div></div>';
    },

    // --------------------------------------------------------------
    // Event glance (hover) card
    // --------------------------------------------------------------
    showGlance: function (mk) {
      var self = this;
      var html;
      if (mk.classList.contains('msil')) {
        var moon = this.mainMoon();
        if (!moon) return;
        var d = parseDayKey(mk.dataset.moonDay);
        var abs = CalDate.dayIndex(this.cal, d.y, d.m, d.d), phase = MoonMath.phase(moon, abs);
        html = '<div class="gh"><b>' + esc(moon.name) + '</b></div><div class="gw">' + esc(MoonMath.name(phase)) + ' · ' + MoonMath.litPct(phase) + '% lit</div>';
      } else if (mk.classList.contains('avl')) {
        html = this._freeGlanceHTML(mk.dataset.avl);
        if (!html) return;
      } else if (String(mk.dataset.ev).indexOf('gn:') === 0) {
        var n = this._findNight(mk.dataset.ev);
        if (!n) return;
        var gt = this._gnTime(n);
        html = '<div class="gh"><b>' + esc(n.name) + '</b></div>' + (gt ? '<div class="gw">' + esc(gt) + '</div>' : '') +
          '<div class="gb">' + esc(n.past ? 'Played' : (n.mine && n.mine.answer ? 'You said ' + this._gnAnswerWord(n.mine) + '. ' : 'You haven’t answered yet. ') + this._gnTally(n)) + '</div>';
      } else {
        var ev = this._findEvent(mk.dataset.ev);
        if (!ev) return;
        var time = ev.all_day ? 'All day' : (ev.start_hour != null ? pad2(ev.start_hour) + ':' + pad2(ev.start_minute || 0) : '');
        var brief = (ev.description && !ev.description_html) ? ev.description : '';
        html = '<div class="gh"><b>' + esc(ev.name) + '</b></div>' +
          (time ? '<div class="gw">' + esc(time) + '</div>' : '') +
          (brief ? '<div class="gb">' + esc(brief) + '</div>' : '');
      }
      this.hcEl.innerHTML = html;
      var pr = this.calEl.getBoundingClientRect(), r = mk.getBoundingClientRect();
      this.hcEl.style.left = Math.max(8, r.left - pr.left) + 'px';
      this.hcEl.style.top = (r.bottom - pr.top + 6) + 'px';
      this.hcEl.style.transformOrigin = (r.left - pr.left + r.width / 2) + 'px 0px';
      this.hcEl.classList.add('on');
    },

    hideGlance: function () { this.hcEl.classList.remove('on'); },

    _findEvent: function (id) {
      var all = this.eventsFor(this.view.y, this.view.m);
      return all.filter(function (e) { return e.id === id; })[0] || null;
    },

    // --------------------------------------------------------------
    // Event full detail (evp)
    // --------------------------------------------------------------
    openEventDetail: function (id) {
      var self = this, night = String(id).indexOf('gn:') === 0 ? this._findNight(id) : null, ev = night ? null : this._findEvent(id);
      if (!ev && !night) return;
      this.evpEl.innerHTML = night ? this._gnPageHTML(night) : this._evpHTML(ev);
      // The full-detail overlay covers the day card it grew out of (the
      // motion this UI ports, per #741's rules): its final box matches the
      // wing's own rect, not the smaller event row that was clicked.
      growOpen(this.evpEl, this.wingEl, this.calEl, { cover: true });
      this.evpEl.dataset.ev = id;
      this._updateScrim();
    },

    closeEventDetail: function () {
      if (!this.evpEl.classList.contains('open')) return;
      var id = this.evpEl.dataset.ev;
      var anchor = id ? this.wingEl.querySelector('.evd[data-ev="' + id + '"]') : this.wingEl;
      growClose(this.evpEl, anchor);
      this._updateScrim();
    },

    _evpHTML: function (ev) {
      var cal = this.cal;
      var mc = CalDate.monthCount(cal), m0 = ((ev.month - 1) % mc + mc) % mc, monthDef = (cal.months || [])[m0];
      var label = (monthDef ? monthDef.name : ev.month) + ' ' + ev.day + ', ' + ev.year;
      if (ev.end_year != null) {
        var m0e = ((ev.end_month - 1) % mc + mc) % mc, monthDefE = (cal.months || [])[m0e];
        label += ' – ' + (monthDefE ? monthDefE.name : ev.end_month) + ' ' + ev.end_day + ', ' + ev.end_year;
      }
      var time = ev.all_day ? '' : (ev.start_hour != null ? pad2(ev.start_hour) + ':' + pad2(ev.start_minute || 0) +
        (ev.end_hour != null ? '–' + pad2(ev.end_hour) + ':' + pad2(ev.end_minute || 0) : '') : '');
      var recur = '';
      if (ev.is_recurring && ev.recurrence_type) {
        recur = '<div class="row"><i class="fa-solid fa-rotate"></i><span class="tt">' + esc(this.repeatText(ev)) + '</span></div>';
      }
      var entity = ev.entity_id ? '<div class="row"><i class="fa-solid fa-link"></i><a class="tt" href="/campaigns/' + esc(this.campaignId) + '/entities/' + esc(ev.entity_id) + '">' + esc(ev.entity_name || 'Linked page') + '</a></div>' : '';
      var visBadge = (this.canAuthorDmOnly && ev.visibility === 'dm_only') ? '<span class="dirnote"><i class="fa-solid fa-eye-slash"></i>Director only</span>' : '';
      var body = ev.description_html ? ev.description_html : (ev.description ? '<p>' + esc(ev.description) + '</p>' : '');

      return '<div class="grab" aria-hidden="true"></div>' +
        '<div class="ein"><div class="crease"><button type="button" class="back" data-close><i class="fa-solid fa-arrow-left"></i><span>Day</span></button></div>' +
        '<div class="epb">' +
          '<h3 class="ept">' + esc(ev.name) + (ev.kind_name ? ' <span class="dirnote">' + esc(ev.kind_name) + '</span>' : '') + '</h3>' +
          '<div class="espan">' + esc(label) + (time ? ' · ' + esc(time) : '') + ' ' + visBadge + '</div>' +
          (body ? '<div class="notes">' + body + '</div>' : '') +
          recur + entity +
        '<!--EVP_FOOT--></div></div>';
    },

    // --------------------------------------------------------------
    // The era manager's card. Everyone reads eras through the era panel
    // above; calendar_editor.js gives an Owner or co-Director an "Eras"
    // button in the edit strip (#cal5-erabtn) whose card manages them,
    // replacing showFlap below.
    // --------------------------------------------------------------
    toggleEra: function () {
      var self = this, P = this._pf;
      this.hideGlance();
      if (P.state === 'open') { this.closeFlap(); return; }
      if (P.state !== 'closed') return;
      if (this.wingFor) this.closeWing({ quiet: true }).then(function () { self.showFlap(); });
      else this.showFlap();
    },

    showFlap: function () {
      var era = this.eraForDate(this.view.y, this.view.m, 1);
      if (!era) { this.say('No era set for this month.'); return; }
      this.openEraPanel(era.id);
    },

    // Opens the era manager's card (calendar_editor.js) with the given
    // leaves, folding out of its Eras button like every other card here.
    _showFlapHTML: function (html) {
      var self = this, P = this._pf, btn = $('#cal5-erabtn', this.el);
      if (P.state !== 'closed' || !btn) return;
      resetCard(this.flapEl);
      this.flapEl.innerHTML = html;
      var g = isPhone() ? null : eraGeometry(this.flapEl, btn, this.calEl);
      if (g) placeCard(this.flapEl, g);
      P.src = btn;
      P.g = g;
      P.press = [{ el: btn, depth: pressDepth(btn) }];
      btn.setAttribute('aria-expanded', 'true');
      this._updateScrim();
      openCard(P, this.calEl).then(function (ok) {
        if (!ok) return;
        self.flapEl.focus({ preventScroll: true });
        var n = P.next; P.next = null; if (n) n();
      });
    },

    closeFlap: function (o) {
      o = o || {};
      var self = this, P = this._pf, btn = $('#cal5-erabtn', this.el);
      if (P.state === 'closed') return Promise.resolve();
      if (P.state === 'opening' && !o.instant) return new Promise(function (res) { P.next = function () { self.closeFlap(o).then(res); }; });
      if (P.state === 'closing' && !o.instant) return P.closing || Promise.resolve();
      var had = this.flapEl.contains(document.activeElement);
      P.closing = closeCard(P, this.calEl, o).then(function (ok) {
        if (!ok) return;
        btn.setAttribute('aria-expanded', 'false');
        self.flapEl.innerHTML = '';
        P.closing = null;
        self._updateScrim();
        if (!o.quiet && had) btn.focus({ preventScroll: true });
        var n = P.next; P.next = null; if (n) n();
      });
      this._updateScrim();
      return P.closing;
    },

    // --------------------------------------------------------------
    // Moon view (a simplified but functional stand-in for the mockups'
    // full ambient canvas graph — see .ai.md "Honest gaps").
    // --------------------------------------------------------------
    openMoonView: function () {
      var moons = this.cal.moons || [];
      if (!moons.length) { this.say('This calendar has no moons.'); return; }
      this.mvMoonId = this.mvMoonId || moons[0].id;
      this.mvRange = this.mvRange || 'month';
      var btn = $('#cal5-moonbtn', this.el);
      this.mvEl.innerHTML = this._mvHTML();
      growOpen(this.mvEl, btn, this.calEl, { center: true });
      btn.setAttribute('aria-expanded', 'true');
      this._observeMvGraph();
      this._updateScrim();
    },

    closeMoonView: function () {
      if (!this.mvEl.classList.contains('open')) return;
      var btn = $('#cal5-moonbtn', this.el);
      btn.setAttribute('aria-expanded', 'false');
      growClose(this.mvEl, btn);
      if (this._mvObserver) this._mvObserver.disconnect();
      this._updateScrim();
    },

    selectMoon: function (id) { this.mvMoonId = id; this._refreshMv(); },

    _refreshMv: function () {
      var scrollLeft = this.mvEl.querySelector('.gframe') ? this.mvEl.querySelector('.gframe').scrollLeft : 0;
      this.mvEl.innerHTML = this._mvHTML();
      var frame = this.mvEl.querySelector('.gframe');
      if (frame) frame.scrollLeft = scrollLeft;
      this._observeMvGraph();
    },

    _mvHTML: function () {
      var cal = this.cal, moon = this.moonById(this.mvMoonId) || (cal.moons || [])[0];
      var abs = CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
      var phase = moon ? MoonMath.phase(moon, abs) : 0;

      var strip = (cal.moons || []).map(function (mo) {
        var p = MoonMath.phase(mo, abs);
        return '<button type="button" class="msi' + (mo.id === (moon && moon.id) ? ' on' : '') + '" data-moon="' + esc(mo.id) + '" aria-pressed="' + (mo.id === (moon && moon.id)) + '">' +
          '<span class="msw">' + this._moonSilSVG(p) + '</span><span class="mst"><b>' + esc(mo.name) + '</b></span></button>';
      }.bind(this)).join('');

      var graph = this._mvGraphHTML(moon);

      return '<div class="grab" aria-hidden="true"></div>' +
        '<div class="mvcrease"><div class="crease"><span class="mvt">Moons</span><span class="mvsub" id="cal5-mvsub"></span><button type="button" class="x" data-close aria-label="Close">✕</button></div></div>' +
        '<div class="mvbody">' +
          '<div class="mvl">' +
            '<div class="mvbig"><div class="mvhead">' +
              '<h3 class="mvname">' + esc(moon ? moon.name : 'No moons') + '</h3>' +
              (moon ? '<div class="mvlit"><b>' + MoonMath.litPct(phase) + '%</b> lit · ' + esc(MoonMath.name(phase)) + '</div>' : '') +
            '</div></div>' +
            '<div class="mvstrip">' + strip + '</div>' +
          '</div>' +
          '<div class="mvr">' +
            '<div class="phead"><h4 class="plabel">This month</h4></div>' +
            graph +
          '</div>' +
        '</div>';
    },

    // A day-tick strip standing in for the mockups' ambient canvas graph:
    // each day gets a small phase disc, today is marked, and a moon-night
    // payload on that day gets a small coloured badge above its tick.
    // Reduced motion / off-screen both freeze the ambient sweep via the
    // .still class (calendar-view.css already guards .gframe.still).
    _mvGraphHTML: function (moon) {
      if (!moon) return '';
      var cal = this.cal, y = this.cal.current_year, m = this.cal.current_month;
      var mc = CalDate.monthCount(cal), m0 = ((m - 1) % mc + mc) % mc, days = CalDate.monthDays(cal, m0, y);
      var todayD = cal.current_day;
      var evs = this.eventsFor(y, m).filter(function (e) { return e.payload; });
      var self = this;
      var ticks = '';
      for (var d = 1; d <= days; d++) {
        var abs = CalDate.dayIndex(cal, y, m, d);
        var phase = MoonMath.phase(moon, abs);
        var badge = '';
        evs.forEach(function (e) {
          if (!CalDate.withinSpan(cal, e, y, m, d) && !(e.year === y && e.month === m && e.day === d)) return;
          var payload = self._parsePayload(e);
          if (payload && payload.moons && payload.moons.indexOf(moon.id) >= 0) badge = ' data-night="' + esc(payload.type) + '"';
        });
        // .gtick, not the mockup's .gx: .gx is an absolutely-positioned
        // LABEL meant to overlay a canvas graph (position:absolute, no
        // flow height of its own) — the ambient canvas it was designed for
        // is #794, not built here. Reusing it directly collapses .gframe
        // to zero height (every child pulled out of flow, nothing left to
        // size the container). .gtick is this strip's own flow-layout
        // class instead, styled fresh further down calendar-view.css.
        ticks += '<div class="gtick' + (d === todayD ? ' today' : '') + '"' + badge + ' title="' + esc(MoonMath.name(phase)) + '">' + this._moonSilSVG(phase) + '<span>' + d + '</span></div>';
      }
      return '<div class="gframe" tabindex="0">' + ticks + '</div>';
    },

    _parsePayload: function (e) {
      if (!e.payload) return null;
      try { return JSON.parse(e.payload); } catch (err) { return null; }
    },

    _observeMvGraph: function () {
      var self = this, frame = this.mvEl.querySelector('.gframe');
      if (!frame || !('IntersectionObserver' in window)) return;
      this._mvObserver = new IntersectionObserver(function (entries) {
        entries.forEach(function (en) { frame.classList.toggle('still', !en.isIntersecting); });
      });
      this._mvObserver.observe(frame);
    },

    // --------------------------------------------------------------
    // Calendar switcher (hub popover)
    // --------------------------------------------------------------
    toggleHub: function () {
      if (this.popEl.classList.contains('open')) { this.closePop(); return; }
      this.showHub();
    },

    showHub: function () {
      var self = this, btn = $('#cal5-hub', this.el);
      this._popBtn = btn;
      this.popEl.innerHTML = '<div class="crease"><span>Calendars</span><button type="button" class="x" data-close aria-label="Close">✕</button></div><div class="list"><p class="none" style="padding:10px">Loading…</p></div>';
      growOpen(this.popEl, btn, this.calEl);
      btn.setAttribute('aria-expanded', 'true');
      this._updateScrim();
      Chronicle.apiFetch('/campaigns/' + this.campaignId + '/calendars/list')
        .then(function (resp) { return resp.ok ? resp.json() : []; })
        .then(function (list) {
          if (!Array.isArray(list)) list = [];
          var body = self.popEl.querySelector('.list');
          if (!body) return;
          body.innerHTML = list.map(function (c) {
            var current = c.id === self.calendarId;
            return '<a class="hubi" href="/campaigns/' + esc(self.campaignId) + '/calendars/' + esc(c.id) + '/view"' + (current ? ' aria-current="true"' : '') + '>' +
              '<span class="hn">' + esc(c.name) + '</span>' +
              (c.is_default ? '<span class="hs">Default calendar</span>' : '') +
              '</a>';
          }).join('') || '<p class="none" style="padding:10px">No other calendars in this campaign.</p>';
        })
        .catch(function () {
          var body = self.popEl.querySelector('.list');
          if (body) body.innerHTML = '<p class="none" style="padding:10px">Could not load the calendar list.</p>';
        });
    },

    // A press outside the popover closes it, unless it holds a date being
    // set: then it stays and says so, so a stray press never loses it.
    _popOff: function () {
      var form = this.popEl.querySelector('[data-today-form]');
      if (form && this._todayFormDirty(form)) {
        var err = form.querySelector('.sterr');
        err.textContent = 'You haven’t set this date yet. Press Set today, or Cancel.';
        err.hidden = false;
        err.classList.remove('shake'); void err.offsetWidth; err.classList.add('shake');
        return;
      }
      this.closePop();
    },

    _todayFormDirty: function (form) {
      var c = this.cal, v = function (k) { return form.elements[k].value.trim(); };
      return v('year') !== String(c.current_year) || v('month') !== String(c.current_month) || v('day') !== String(c.current_day) ||
        v('hour') !== String(c.current_hour || 0) || v('minute') !== String(c.current_minute || 0);
    },

    closePop: function () {
      if (!this.popEl.classList.contains('open')) return;
      var btn = this._popBtn && this._popBtn.isConnected ? this._popBtn : $('#cal5-hub', this.el);
      $('#cal5-hub', this.el).setAttribute('aria-expanded', 'false');
      this._popBtn = null;
      growClose(this.popEl, btn);
      this._updateScrim();
    }
  });

  // Test-only hook: exposes the pure date/moon-phase math so the node:test
  // contract suite (test/js/calendar_math.test.mjs) can check it against the
  // Go model's own values without a browser DOM. `module` is undefined when
  // loaded via <script>, so this is a no-op in the browser.
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { CalDate: CalDate, MoonMath: MoonMath, EraMath: EraMath, weatherMarkHTML: weatherMarkHTML, weatherFactHTML: weatherFactHTML, weatherPaintHTML: weatherPaintHTML, forecastMarkHTML: forecastMarkHTML, forecastFactHTML: forecastFactHTML, forecastDetailText: forecastDetailText, playersSeeHTML: playersSeeHTML, weatherIcon: weatherIcon, awayStretches: awayStretches };
  }
})();
