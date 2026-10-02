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

    // occursOn mirrors Event.OccursOn exactly (model.go), including its
    // early-outs and its "recurrence only moves forward" rule.
    occursOn: function (cal, e, year, month, day) {
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
  function parseDayKey(k) { var p = k.split('_'); return { y: +p[0], m: +p[1], d: +p[2] }; }
  function pad2(n) { return (n < 10 ? '0' : '') + n; }
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
  // (day wing, era flap, event detail, moon view, hub popover). The panel
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
  var FOLD = { d: 1100, lead: [30, 330], trail: [100, 390], rest: 490, fade: 80, cut: 76, shut: 450, back: 0.86, pressBack: 250, shade: 0.3 };
  var SPRING = 'cubic-bezier(.34,1.3,.4,1)';
  var EASE = 'cubic-bezier(.22,.8,.24,1)';
  function isPhone() { return window.matchMedia('(max-width:600px)').matches; }
  function anim(el, frames, o) { return el.animate(frames, Object.assign({ fill: 'forwards' }, o)); }
  function stopAnims(el, deep) { (deep ? el.getAnimations({ subtree: true }) : el.getAnimations()).forEach(function (a) { a.cancel(); }); }
  function settleAll(list) { return Promise.all(list.map(function (a) { return a.finished; })); }
  function clampN(v, a, b) { return Math.max(a, Math.min(b, v)); }
  function relRect(el, box) { var b = box.getBoundingClientRect(), r = el.getBoundingClientRect(); return { x: r.left - b.left, y: r.top - b.top, w: r.width, h: r.height }; }

  // The part of the calendar a card may use: what is on screen, inside
  // whatever scrolls it (the page, or the almanac on the Calendars page).
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
    var F = { el: el, d: FOLD.d, V: { x: W / 2, y: H / 2 }, ax: flat ? 'rotateY' : 'rotateX', B: { x: 0, y: h1 }, head: head, lead: head ? L1 : L2, trail: head ? L2 : L1, s2: head ? -1 : 1 };
    if (flat) { F.A = { x: side === 'left' ? W : 0, y: 0 }; F.s1 = side === 'right' ? 1 : -1; }
    else { F.A = { x: 0, y: head ? 0 : H }; F.s1 = head ? -1 : 1; }
    F.O = { lead: { x: 0, y: head ? 0 : h1 }, trail: { x: 0, y: head ? h1 : 0 } };
    return F;
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
      var F = foldGeo(P, calEl), marks = fly(P, false, { delay: 110, dur: 390, stagger: 30 }, calEl);
      pressIn(P.press);
      run = settleAll(foldRun(F, false, 1).concat(marks));
    }
    return run.then(function () {
      if (tok !== P.seq) throw new Error('superseded');
      stopAnims(el, true);
      land(P, false);
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
      var marks = fly(P, true, { delay: 0, dur: 340 / sp, stagger: 24 / sp }, calEl);
      run = settleAll(foldRun(foldGeo(P, calEl), true, sp).concat(marks));
      // The card tucks back into what it came from, which gives a little.
      pressIn(P.press, { delay: FOLD.pressBack / sp, dur: 300 / sp, soft: true });
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

  // Exposed for calendar_editor.js (a separate script/closure loaded only
  // for a CanEdit viewer — see that file's header) so the date-math port
  // is written once and both files stay in agreement with the server, and
  // so a canEdit override of view.showFlap/etc. can open/close a panel with
  // the exact same grow-from-anchor motion as every other panel here.
  Chronicle.calendarDate = CalDate;
  Chronicle.calendarPanel = { growOpen: growOpen, growClose: growClose };
  Chronicle.calendarColor = sanitizeColor;

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
      this.apiBase = config.apiBase;

      var cfgEl = $('#calendar-config', el);
      try { this.cal = JSON.parse(cfgEl.dataset.calendar || '{}'); } catch (e) { this.cal = {}; }
      var initialEvents;
      try { initialEvents = JSON.parse(cfgEl.dataset.events || '[]'); } catch (e2) { initialEvents = []; }
      if (!Array.isArray(initialEvents)) initialEvents = [];

      this.eventsByMonth = {};
      this.weatherByYear = {}; // year -> {'m_d': reading}, filled by fetchWeatherYear
      this.eventsByMonth[this.cal.current_year + '_' + this.cal.current_month] = initialEvents;

      this.view = { y: this.cal.current_year || 1, m: this.cal.current_month || 1 };
      this.rm = reducedMotion();
      this.wingFor = null; // key of the day whose wing is open, or null
      this.selection = {}; // edit-mode multi-select, keyed by dayKey -> true

      this._buildShell();
      this._pw = cardState(this.wingEl, this.carryEl);
      this._pf = cardState(this.flapEl);
      this._bindEvents();
      this._dockSky();
      this.renderMonth();
      this.renderToday();
      this.renderLegend();

      // Hand the instance to calendar_editor.js (loaded only for an
      // Owner/co-Director viewer) without assuming load order.
      el.calendarView = this;
      el.dispatchEvent(new CustomEvent('calendarv5:ready', { detail: this, bubbles: false }));
    },

    destroy: function (el) {
      if (this.skyDock) this.skyDock.destroy();
      if (this._resizeHandler) window.removeEventListener('resize', this._resizeHandler);
      if (this._escHandler) document.removeEventListener('keydown', this._escHandler);
      if (this._mvOffHandler) document.removeEventListener('pointerdown', this._mvOffHandler, true);
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
          '<div class="skywrap" id="cal5-skywrap" hidden><div class="sky" role="img" aria-label="The sky"><canvas></canvas></div></div>' +
          '<header class="head">' +
            '<div class="h-row h-top">' +
              '<button type="button" class="hub" id="cal5-hub" aria-haspopup="dialog" aria-expanded="false"><span>' + esc(this.cal.name || 'Calendar') + '</span><i class="fa-solid fa-chevron-down"></i></button>' +
              '<div class="todaypill" id="cal5-todaypill"></div>' +
              '<div class="h-acts">' +
                '<button type="button" class="skybtn" id="cal5-skybtn" aria-expanded="false" aria-controls="cal5-skywrap" title="Fold or open the sky" hidden><span class="sw" aria-hidden="true"></span><span>Sky</span></button>' +
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
                '<button type="button" class="erabtn" id="cal5-erabtn" aria-haspopup="dialog" aria-expanded="false"><span id="cal5-eraname"></span><i class="fa-solid fa-chevron-down"></i></button>' +
                '<span class="season" id="cal5-season"></span>' +
              '</div>' +
            '</div>' +
          '</header>' +
          '<div class="dow" id="cal5-dow" aria-hidden="true"></div>' +
          '<div class="stage" id="cal5-stage"></div>' +
          '<div class="hc" id="cal5-hc" aria-hidden="true"></div>' +
          '<div class="wing" id="cal5-wing" role="dialog" aria-label="Day" tabindex="-1"></div>' +
          '<div class="evp" id="cal5-evp" role="dialog" aria-label="Event" tabindex="-1"></div>' +
          '<div class="mv" id="cal5-mv" role="dialog" aria-label="Moons" tabindex="-1"></div>' +
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
    },

    say: function (msg) {
      this._announce(msg);
      this._toast(msg);
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
      this._skyDay();
    },

    // Today's sky, redrawn only when today's events change. The calendar
    // and events here are already what this viewer may see.
    _skyDay: function () {
      var c = this.cal;
      if (!this.skyDock || !this.eventsByMonth[this.monthKey(c.current_year, c.current_month)]) return;
      var evs = this.eventsOnDay(c.current_year, c.current_month, c.current_day);
      var sig = evs.map(function (e) { return e.id + ':' + (e.updated_at || ''); }).join(',');
      if (sig === this._skySig) return;
      this._skySig = sig;
      this.skyDock.setDay(c, evs);
    },

    // A transform on the card would carry its phone bars and sheets
    // (position:fixed) along with it, so while one shows the sky folds
    // without travelling.
    _fixedShowing: function () {
      var els = this.calEl.querySelectorAll('.bbar:not([hidden]), .btray:not([hidden]), .bmore:not([hidden]), .drawer.open, .toast.on');
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
    _toast: function (msg) {
      var el = this._toastEl;
      if (!el) {
        el = document.createElement('div');
        el.className = 'toast';
        el.setAttribute('aria-hidden', 'true');
        el.innerHTML = '<span></span>';
        this.calEl.appendChild(el);
        this._toastEl = el;
      }
      el.querySelector('span').textContent = msg;
      el.classList.add('on');
      clearTimeout(this._toastTimer);
      this._toastTimer = setTimeout(function () { el.classList.remove('on'); }, 3200);
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
    weatherOnDay: function (y, m, d) {
      var byDay = this.weatherByYear[y], w = byDay && byDay[m + '_' + d];
      if (w) return w;
      var cal = this.cal;
      return y === cal.current_year && m === cal.current_month && d === cal.current_day && cal.weather ? cal.weather : null;
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

    eraForDate: function (year, month, day) {
      var eras = this.cal.eras || [];
      function less(y1, m1, d1, y2, m2, d2) { if (y1 !== y2) return y1 < y2; if (m1 !== m2) return m1 < m2; return d1 < d2; }
      for (var i = 0; i < eras.length; i++) {
        var e = eras[i];
        if (less(year, month, day, e.start_year, e.start_month, e.start_day)) continue;
        if (e.end_year == null) return e;
        if (e.end_month == null || e.end_day == null) { if (year <= e.end_year) return e; continue; }
        if (!less(e.end_year, e.end_month, e.end_day, year, month, day)) return e;
      }
      return null;
    },

    // --------------------------------------------------------------
    // Rendering: header + month grid
    // --------------------------------------------------------------
    renderHeader: function () {
      var cal = this.cal, mc = CalDate.monthCount(cal), m0 = ((this.view.m - 1) % mc + mc) % mc;
      var monthDef = (cal.months || [])[m0];
      var mtitle = $('#cal5-mtitle', this.el);
      mtitle.innerHTML = esc(monthDef ? monthDef.name : ('Month ' + this.view.m)) + ' <span class="myear">' + esc(this.view.y) + '</span>';

      var era = this.eraForDate(this.view.y, this.view.m, 1);
      $('#cal5-eraname', this.el).textContent = era ? era.name : 'No era set';
      var season = this.seasonForDate(this.view.m, 15);
      $('#cal5-season', this.el).innerHTML = season ? ('<b>' + esc(season.name) + '</b>') : '';

      var dow = this.dowEl, weekdays = cal.weekdays || [];
      dow.innerHTML = weekdays.map(function (w) {
        return '<span><span class="f">' + esc(w.name) + '</span><span class="s">' + esc(w.name.slice(0, 1)) + '</span></span>';
      }).join('');
      this.calEl.style.setProperty('--cols', weekdays.length || 7);
    },

    renderToday: function () {
      var cal = this.cal;
      var el = $('#cal5-todaypill', this.el);
      var mc = CalDate.monthCount(cal), m0 = ((cal.current_month - 1) % mc + mc) % mc;
      var monthDef = (cal.months || [])[m0];
      var label = (monthDef ? monthDef.name : cal.current_month) + ' ' + cal.current_day + ', ' + cal.current_year;
      el.innerHTML = '<span class="tl">Today</span><span class="td">' + esc(label) + '</span>';
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
      Promise.all([this.fetchMonth(this.view.y, this.view.m), this.fetchWeatherYear(this.view.y)]).then(function () {
        self._paintMonth();
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
      this._paintDaySkyContext();
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
          '<div class="dc"><span class="bn">' + label + '</span>' + weatherMarkHTML(this.weatherOnDay(y, m, d), isFuture) + marks + '</div>' +
        '</button></div>';
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
        moonHTML + weatherMarkHTML(this.weatherOnDay(y, m, d), !isPast && !isToday) +
        '<div class="dc"><span class="num">' + d + '</span>' + this._marksHTML(y, m, d) + '</div>' +
        '</button>';
    },

    _marksHTML: function (y, m, d) {
      var evs = this.eventsOnDay(y, m, d);
      if (!evs.length) return '';
      var cap = 4, shown = evs.slice(0, cap), extra = evs.length - shown.length;
      var self = this;
      var html = '<div class="marks">' + shown.map(function (e) {
        var dirRing = self.canAuthorDmOnly && e.visibility === 'dm_only' ? ' dir' : '';
        return '<span class="mk' + dirRing + '" data-ev="' + esc(e.id) + '" style="' + eventColorStyle(e) + '" title="' + esc(e.name) + '">' + esc(eventGlyph(e)) + '</span>';
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

    _paintDaySkyContext: function () {
      // No ambient backdrop-field animation is ported (the mockups' era/
      // season colour wash) — a deliberate simplification, see .ai.md.
    },

    // --------------------------------------------------------------
    // Navigation
    // --------------------------------------------------------------
    goMonth: function (delta) {
      // A day's card belongs to its day; changing the month puts it away.
      if (this.wingFor) this.closeWing({ instant: true, quiet: true });
      var next = CalDate.shiftMonth(this.cal, this.view.y, this.view.m, delta);
      this.view = next;
      this.renderMonth();
    },

    goToday: function () {
      if (this.wingFor) this.closeWing({ instant: true, quiet: true });
      this.view = { y: this.cal.current_year, m: this.cal.current_month };
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
      $('#cal5-erabtn', this.el).addEventListener('click', function () { self.toggleEra(); });
      $('#cal5-moonbtn', this.el).addEventListener('click', function () { self.openMoonView(); });
      this.scrimEl.addEventListener('click', function () { self.closeAllPanels(); });

      this.stageEl.addEventListener('click', function (e) {
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
        var mk = e.target.closest('.mk, .msil');
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

      this.wingEl.addEventListener('click', function (e) {
        if (e.target.closest('[data-close]')) { self.closeWing(); return; }
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
        if (e.target.closest('[data-close]')) self.closeEventDetail();
      });
      this.mvEl.addEventListener('click', function (e) {
        if (e.target.closest('[data-close]')) self.closeMoonView();
        var strip = e.target.closest('.msi');
        if (strip) self.selectMoon(strip.dataset.moon);
      });
      this.popEl.addEventListener('click', function (e) {
        if (e.target.closest('[data-close]')) self.closePop();
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
      };
      window.addEventListener('resize', this._resizeHandler);

      // Escape closes whichever panel is open (motion rule #760: "every
      // panel works from the keyboard"), innermost first: the event detail
      // overlay before the day wing it grew out of, so Escape steps back
      // one layer at a time rather than closing everything at once.
      this._escHandler = function (e) {
        if (e.key !== 'Escape') return;
        if (self.evpEl.classList.contains('open')) { self.closeEventDetail(); return; }
        if (self.mvEl.classList.contains('open')) { self.closeMoonView(); return; }
        if (self._pf.state !== 'closed') { self.closeFlap(); return; }
        if (self.popEl.classList.contains('open')) { self.closePop(); return; }
        if (self.wingFor) { self.closeWing(); return; }
      };
      document.addEventListener('keydown', this._escHandler);

      // The moon view sits over the month, so a press anywhere else in the
      // calendar closes it, the way the day and era cards give way to a
      // press on the grid. Presses outside the calendar are left to the
      // page (the almanac's click-off closes an open card itself), and
      // presses that open the moon view again are left to do that.
      this._mvOffHandler = function (e) {
        if (!self.mvEl.classList.contains('open')) return;
        var t = e.target;
        if (!(t instanceof Element) || !self.el.contains(t) || self.mvEl.contains(t) || self.evpEl.contains(t)) return;
        if (t.closest('#cal5-moonbtn, .msil, .mrow')) return;
        self.closeMoonView();
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
        this.mvEl.classList.contains('open') || this.popEl.classList.contains('open');
    },

    _updateScrim: function () {
      this.scrimEl.classList.toggle('on', this.anyPanelOpen() && window.matchMedia('(max-width:600px)').matches);
    },

    // o.instant takes them down without their motion (the almanac folding
    // the whole calendar away).
    closeAllPanels: function (o) {
      if (this.wingFor) this.closeWing(o);
      this.closeFlap(o);
      this.closeEventDetail();
      this.closeMoonView();
      this.closePop();
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
        if (highlightEventId) this._pickRow(highlightEventId); else this.closeWing();
        return;
      }
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
    refreshWing: function () {
      // Never under a form being filled in: its own save refreshes after.
      if (!this.wingFor || this._pw.state !== 'open' || this.wingEl.querySelector('form')) return;
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
        self.wingFor = null;
        P.closing = null;
        self._updateScrim();
        if (!o.quiet && had && btn && btn.isConnected) btn.focus({ preventScroll: true });
        var n = P.next; P.next = null; if (n) n();
      });
      this._updateScrim();
      return P.closing;
    },

    _wingHTML: function (d, highlightEventId) {
      var cal = this.cal;
      var mc = CalDate.monthCount(cal), m0 = ((d.m - 1) % mc + mc) % mc, monthDef = (cal.months || [])[m0];
      var label = (monthDef ? monthDef.name : d.m) + ' ' + d.d + ', ' + d.y;
      var isToday = d.y === cal.current_year && d.m === cal.current_month && d.d === cal.current_day;
      var isFuture = CalDate.dayIndex(cal, d.y, d.m, d.d) > CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
      var weatherFact = weatherFactHTML(this.weatherOnDay(d.y, d.m, d.d), isFuture);
      var moonRows = (cal.moons || []).map(function (mo) {
        var abs = CalDate.dayIndex(cal, d.y, d.m, d.d), phase = MoonMath.phase(mo, abs);
        return '<button type="button" class="mrow" data-moon="' + esc(mo.id) + '"><span class="mt"><b>' + esc(mo.name) + '</b><small>' + esc(MoonMath.name(phase)) + ' · ' + MoonMath.litPct(phase) + '% lit</small></span><i class="fa-solid fa-chevron-right chev"></i></button>';
      }).join('');

      var evs = this.eventsOnDay(d.y, d.m, d.d);
      var evHTML = evs.length ? evs.map(function (e) {
        var hl = highlightEventId && e.id === highlightEventId ? ' hl' : '';
        var time = e.all_day ? '' : (e.start_hour != null ? pad2(e.start_hour) + ':' + pad2(e.start_minute || 0) : '');
        return '<div class="evd go' + hl + '" data-ev="' + esc(e.id) + '">' +
          '<div class="evh"><span class="ric" style="' + eventColorStyle(e) + '">' + esc(eventGlyph(e)) + '</span><span class="qt0">' + esc(e.name) + '</span></div>' +
          (time ? '<div class="evm">' + esc(time) + '</div>' : '') +
          '</div>';
      }).join('') : '<div class="none">Nothing on the calendar today.</div>';

      // Two leaves: the head (the date and the weather), which stays in
      // view, and the rest, which scrolls on a desktop: the day's events
      // first, then its moons.
      var moonCount = (cal.moons || []).length;
      return '<div class="leaf lf1"><div class="grab" aria-hidden="true"></div>' +
          '<div class="crease"><span>Day</span><button type="button" class="x" data-close aria-label="Close">✕</button></div>' +
          '<div class="wb wbh"><h3 class="wdate">' + esc(label) + '</h3>' +
          (weatherFact ? '<div class="facts">' + weatherFact + '</div>' : '') + '</div>' +
        '</div>' +
        '<div class="leaf lf2"><div class="lscroll"><div class="wb wbr">' +
          '<div class="sect">' + (evs.length > 1 ? evs.length + ' events' : 'Events') + '</div>' +
          '<div class="evlist">' + evHTML + '</div>' +
          (this.canEdit ? '<button type="button" class="addev" data-add-event><i class="fa-solid fa-plus"></i>Add an event</button>' : '') +
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
      var self = this, ev = this._findEvent(id);
      if (!ev) return;
      this.evpEl.innerHTML = this._evpHTML(ev);
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
        recur = '<div class="row"><i class="fa-solid fa-rotate"></i><span class="tt">Repeats ' + esc(ev.recurrence_type) + '</span></div>';
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
    // Era flap (read-only timeline — the signed mockups never designed
    // Owner era editing; see .ai.md "Honest gaps").
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
      this._showFlapHTML(this._eraHTML(era));
    },

    // Opens the era's card with the given leaves; calendar_editor.js's era
    // manager opens through here too, so both fold the same way.
    _showFlapHTML: function (html) {
      var self = this, P = this._pf, btn = $('#cal5-erabtn', this.el);
      if (P.state !== 'closed') return;
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

    // The era's card: its name and span in the head leaf, its description
    // (when it has one) below.
    _eraHTML: function (era) {
      var span = esc(era.start_year) + (era.end_year != null ? '–' + esc(era.end_year) : '–present');
      return '<div class="leaf lf1"><div class="grab" aria-hidden="true"></div>' +
          '<div class="crease"><span>Era</span><button type="button" class="x" data-close aria-label="Close">✕</button></div>' +
          '<div class="fb fbh"><h3 class="ename">' + esc(era.name) + '</h3><div class="espan"><span>' + span + '</span></div></div>' +
        '</div>' +
        (era.description ? '<div class="leaf lf2"><div class="lscroll"><div class="fb fbr"><p class="edesc">' + esc(era.description) + '</p></div></div></div>' : '');
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

    closePop: function () {
      if (!this.popEl.classList.contains('open')) return;
      var btn = $('#cal5-hub', this.el);
      btn.setAttribute('aria-expanded', 'false');
      growClose(this.popEl, btn);
      this._updateScrim();
    }
  });

  // Test-only hook: exposes the pure date/moon-phase math so the node:test
  // contract suite (test/js/calendar_math.test.mjs) can check it against the
  // Go model's own values without a browser DOM, the same pattern
  // appearance_editor.js uses for its save-sequencing helper. `module` is
  // undefined when loaded via <script>, so this is a no-op in the browser.
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { CalDate: CalDate, MoonMath: MoonMath, weatherMarkHTML: weatherMarkHTML, weatherFactHTML: weatherFactHTML };
  }
})();
