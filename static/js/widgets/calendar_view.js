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
      // TracksRealTime itself is never serialized to the client (server-only
      // field, model.go). A manual (non-real-time) reallife calendar still
      // wants true Gregorian month lengths, so 'reallife' alone is the
      // client-visible signal to use native Date arithmetic — same
      // heuristic the mockup's own isG() uses for its Gregorian structure.
      return cal.mode === 'reallife';
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

    // absoluteDay mirrors Calendar.AbsoluteDay: leap-aware, used for moon
    // phase and for date-shift arithmetic (calendar_editor.js's bulk
    // "Shift events").
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
    // #768 (moon phase centering) is left to PR #775 — do not touch this
    // bucket table, it mirrors Moon.MoonPhaseName verbatim.
    name: function (phase) {
      if (phase < 0.125) return 'New Moon';
      if (phase < 0.25) return 'Waxing Crescent';
      if (phase < 0.375) return 'First Quarter';
      if (phase < 0.5) return 'Waxing Gibbous';
      if (phase < 0.625) return 'Full Moon';
      if (phase < 0.75) return 'Waning Gibbous';
      if (phase < 0.875) return 'Last Quarter';
      return 'Waning Crescent';
    },
    litPct: function (phase) { return Math.round(((1 - Math.cos(2 * Math.PI * phase)) / 2) * 100); }
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
      this.eventsByMonth[this.cal.current_year + '_' + this.cal.current_month] = initialEvents;

      this.view = { y: this.cal.current_year || 1, m: this.cal.current_month || 1 };
      this.rm = reducedMotion();
      this.wingFor = null; // key of the day whose wing is open, or null
      this.selection = {}; // edit-mode multi-select, keyed by dayKey -> true

      this._buildShell();
      this._bindEvents();
      this.renderMonth();
      this.renderToday();
      this.renderLegend();

      // Hand the instance to calendar_editor.js (loaded only for an
      // Owner/co-Director viewer) without assuming load order.
      el.calendarView = this;
      el.dispatchEvent(new CustomEvent('calendarv5:ready', { detail: this, bubbles: false }));
    },

    destroy: function (el) {
      if (this._resizeHandler) window.removeEventListener('resize', this._resizeHandler);
      if (this._escHandler) document.removeEventListener('keydown', this._escHandler);
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
          '<header class="head">' +
            '<div class="h-row h-top">' +
              '<button type="button" class="hub" id="cal5-hub" aria-haspopup="dialog" aria-expanded="false"><span>' + esc(this.cal.name || 'Calendar') + '</span><i class="fa-solid fa-chevron-down"></i></button>' +
              '<div class="todaypill" id="cal5-todaypill"></div>' +
              '<div class="h-acts">' +
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
      this.dockEl = $('#cal5-dock', this.el);
    },

    say: function (msg) {
      var l = $('#cal5-live', this.el);
      l.textContent = '';
      setTimeout(function () { l.textContent = msg; }, 40);
      this._toast(msg);
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
      this.fetchMonth(this.view.y, this.view.m).then(function () {
        self._paintMonth();
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
      this._paintDaySkyContext();
    },

    _dayBandHTML: function (y, m, d, monthDef, totalDays) {
      var cal = this.cal;
      var isToday = y === cal.current_year && m === cal.current_month && d === cal.current_day;
      var marks = this._marksHTML(y, m, d);
      var label = esc(monthDef.name) + (totalDays > 1 ? ', day ' + d : '');
      return '<div class="band' + (isToday ? ' today' : '') + '">' +
        '<button type="button" class="day" data-key="' + dayKey(y, m, d) + '" style="width:100%">' +
          '<div class="dc"><span class="bn">' + label + '</span>' + marks + '</div>' +
        '</button></div>';
    },

    _dayCellHTML: function (y, m, d) {
      var cal = this.cal, key = dayKey(y, m, d);
      var isToday = y === cal.current_year && m === cal.current_month && d === cal.current_day;
      var isPast = CalDate.dayIndex(cal, y, m, d) < CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
      var moon = this.mainMoon();
      var moonHTML = '';
      if (moon) {
        var abs = CalDate.absoluteDay(cal, y, m, d);
        var phase = MoonMath.phase(moon, abs);
        moonHTML = '<span class="msil" data-moon-day="' + key + '" title="' + esc(moon.name + ', ' + MoonMath.name(phase)) + '">' + this._moonSilSVG(phase) + '</span>';
      }
      return '<button type="button" class="day' + (isToday ? ' today' : '') + (isPast ? ' past' : '') + '" data-key="' + key + '">' +
        moonHTML +
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
      var lit = Math.cos(phase * 2 * Math.PI);
      var rx = Math.abs(lit) * 6;
      var large = lit > 0 ? 1 : 0;
      var path = phase === 0 ? '' :
        'M 0,-6 A 6,6 0 ' + (phase < 0.5 ? '1,1' : '0,1') + ' 0,6 A ' + rx.toFixed(2) + ',6 0 ' + large + ',' + (phase < 0.5 ? 0 : 1) + ' 0,-6 Z';
      return '<svg class="sil" viewBox="-7 -7 14 14" aria-hidden="true"><circle class="db" r="6"/>' + (path ? '<path class="dl" d="' + path + '"/>' : '') + '</svg>';
    },

    _paintDaySkyContext: function () {
      // No ambient backdrop-field animation is ported (the mockups' era/
      // season colour wash) — a deliberate simplification, see .ai.md.
    },

    // --------------------------------------------------------------
    // Navigation
    // --------------------------------------------------------------
    goMonth: function (delta) {
      var next = CalDate.shiftMonth(this.cal, this.view.y, this.view.m, delta);
      this.view = next;
      this.renderMonth();
    },

    goToday: function () {
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
        var evd = e.target.closest('.evd');
        if (evd) self.openEventDetail(evd.dataset.ev);
      });
      this.flapEl.addEventListener('click', function (e) {
        if (e.target.closest('[data-close]')) self.closeFlap();
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

      this._resizeHandler = function () { if (self.wingFor) self.closeWing(); };
      window.addEventListener('resize', this._resizeHandler);

      // Escape closes whichever panel is open (motion rule #760: "every
      // panel works from the keyboard"), innermost first: the event detail
      // overlay before the day wing it grew out of, so Escape steps back
      // one layer at a time rather than closing everything at once.
      this._escHandler = function (e) {
        if (e.key !== 'Escape') return;
        if (self.evpEl.classList.contains('open')) { self.closeEventDetail(); return; }
        if (self.mvEl.classList.contains('open')) { self.closeMoonView(); return; }
        if (self.flapEl.classList.contains('open')) { self.closeFlap(); return; }
        if (self.popEl.classList.contains('open')) { self.closePop(); return; }
        if (self.wingFor) { self.closeWing(); return; }
      };
      document.addEventListener('keydown', this._escHandler);
    },

    _moveRoving: function (delta) {
      var buttons = $$('.day[data-key]', this.stageEl);
      var idx = buttons.findIndex(function (b) { return b === document.activeElement; });
      var next = buttons[Math.max(0, Math.min(buttons.length - 1, (idx < 0 ? 0 : idx) + delta))];
      if (next) next.focus({ preventScroll: false });
    },

    anyPanelOpen: function () {
      return !!this.wingFor || this.flapEl.classList.contains('open') || this.evpEl.classList.contains('open') ||
        this.mvEl.classList.contains('open') || this.popEl.classList.contains('open');
    },

    _updateScrim: function () {
      this.scrimEl.classList.toggle('on', this.anyPanelOpen() && window.matchMedia('(max-width:600px)').matches);
    },

    closeAllPanels: function () {
      if (this.wingFor) this.closeWing();
      this.closeFlap();
      this.closeEventDetail();
      this.closeMoonView();
      this.closePop();
    },

    // --------------------------------------------------------------
    // Day wing (fold-out day card)
    // --------------------------------------------------------------
    openWing: function (key, highlightEventId) {
      var self = this;
      this.wingFor = key;
      var d = parseDayKey(key);
      var anchor = this.stageEl.querySelector('.day[data-key="' + key + '"]');
      this.wingEl.innerHTML = this._wingHTML(d, highlightEventId);
      growOpen(this.wingEl, anchor, this.calEl).then(function () {
        self.wingEl.focus({ preventScroll: true });
        if (highlightEventId) {
          var row = self.wingEl.querySelector('.evd[data-ev="' + highlightEventId + '"]');
          if (row) { row.scrollIntoView({ block: 'nearest' }); row.classList.add('pulse'); }
        }
      });
      this._updateScrim();
    },

    closeWing: function () {
      if (!this.wingFor) return;
      var anchor = this.stageEl.querySelector('.day[data-key="' + this.wingFor + '"]');
      this.wingFor = null;
      growClose(this.wingEl, anchor);
      this._updateScrim();
    },

    _wingHTML: function (d, highlightEventId) {
      var cal = this.cal;
      var mc = CalDate.monthCount(cal), m0 = ((d.m - 1) % mc + mc) % mc, monthDef = (cal.months || [])[m0];
      var label = (monthDef ? monthDef.name : d.m) + ' ' + d.d + ', ' + d.y;
      var isToday = d.y === cal.current_year && d.m === cal.current_month && d.d === cal.current_day;
      var weatherFact = '';
      if (isToday && cal.weather) {
        var w = cal.weather;
        weatherFact = '<div class="row"><i class="fa-solid fa-cloud-sun"></i><span class="tt">' + esc(w.preset_label || w.description || 'Weather recorded') +
          (w.temperature_celsius != null ? ' · ' + w.temperature_celsius + '°C' : '') + '</span></div>';
      }
      var moonRows = (cal.moons || []).map(function (mo) {
        var abs = CalDate.absoluteDay(cal, d.y, d.m, d.d), phase = MoonMath.phase(mo, abs);
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

      return '<div class="grab" aria-hidden="true"></div>' +
        '<div class="crease"><span>Day</span><button type="button" class="x" data-close aria-label="Close">✕</button></div>' +
        '<div class="wb">' +
          '<h3 class="wdate">' + esc(label) + '</h3>' +
          (weatherFact || moonRows ? '<div class="facts">' + weatherFact + moonRows + '</div>' : '') +
          '<div class="sect">' + (evs.length > 1 ? evs.length + ' events' : 'Events') + '</div>' +
          '<div class="evlist">' + evHTML + '</div>' +
          (this.canEdit ? '<button type="button" class="addev" data-add-event><i class="fa-solid fa-plus"></i>Add an event</button>' : '') +
        '</div>';
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
        var abs = CalDate.absoluteDay(this.cal, d.y, d.m, d.d), phase = MoonMath.phase(moon, abs);
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
      if (this.flapEl.classList.contains('open')) { this.closeFlap(); return; }
      this.showFlap();
    },

    showFlap: function () {
      var era = this.eraForDate(this.view.y, this.view.m, 1);
      if (!era) { this.say('No era set for this month.'); return; }
      var btn = $('#cal5-erabtn', this.el);
      this.flapEl.innerHTML = this._eraHTML(era);
      growOpen(this.flapEl, btn, this.calEl);
      btn.setAttribute('aria-expanded', 'true');
      this._updateScrim();
    },

    closeFlap: function () {
      if (!this.flapEl.classList.contains('open')) return;
      var btn = $('#cal5-erabtn', this.el);
      btn.setAttribute('aria-expanded', 'false');
      growClose(this.flapEl, btn);
      this._updateScrim();
    },

    _eraHTML: function (era) {
      var span = esc(era.start_year) + (era.end_year != null ? '–' + esc(era.end_year) : '–present');
      return '<div class="grab" aria-hidden="true"></div>' +
        '<div class="crease"><span>Era</span><button type="button" class="x" data-close aria-label="Close">✕</button></div>' +
        '<div class="fb">' +
          '<h3 class="ename">' + esc(era.name) + '</h3>' +
          '<div class="espan"><span>' + span + '</span></div>' +
          (era.description ? '<p class="edesc">' + esc(era.description) + '</p>' : '') +
        '</div>';
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
      var abs = CalDate.absoluteDay(cal, cal.current_year, cal.current_month, cal.current_day);
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
        var abs = CalDate.absoluteDay(cal, y, m, d);
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
    module.exports = { CalDate: CalDate, MoonMath: MoonMath };
  }
})();
