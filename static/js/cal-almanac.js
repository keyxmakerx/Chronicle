// cal-almanac.js — C-CAL-SHOWCASE-DESIGN-1-ALMANAC.
//
// Per-interaction init blocks wrapped in their own try/catch so one
// handler failing can't kill the rest (the carry-forward lesson from
// the showcase saga). Mock data is read from the inline JSON written
// by the templ; nothing here talks to a backend.
//
// MUST tier modules:
//   - widget-drag      : drag the calendar widget by its header handle
//   - widget-resize    : drag the bottom-right corner to resize
//   - sky-scrubber     : range slider repaints the sky gradient + sun/moon arc
//   - event-click      : clicking an event chip opens the drawer
//   - drag-create      : drag across empty cells -> ghost -> create stub
//   - visibility-editor: drawer's Q-V2-7 chip-row builder
//   - month-nav        : prev/next/today buttons (mock — re-renders sky label only)
//   - drawer-close     : close on Escape and on close button

(function () {
  'use strict';

  var INIT_BLOCKS = [];
  function registerInitBlock(name, runner) {
    INIT_BLOCKS.push({ name: name, runner: runner });
  }
  function runAll() {
    var results = [];
    for (var i = 0; i < INIT_BLOCKS.length; i++) {
      var b = INIT_BLOCKS[i];
      try {
        b.runner();
        results.push({ name: b.name, status: 'OK' });
      } catch (err) {
        results.push({ name: b.name, status: 'FAILED', error: (err && err.message) || String(err) });
        try { console.error('[cal-almanac]', b.name, err); } catch (e) {}
      }
    }
    return results;
  }
  function init() {
    if (window.__calAlmanacInited) return;
    var results = runAll();
    window.__calAlmanacInited = true;
    window.__calAlmanacResults = results;
  }

  // Shared dataset (loaded by 'data' block).
  var DATA = null;

  // ============================================================
  // Block 1 — load mock data from the inline JSON.
  // ============================================================
  registerInitBlock('data', function () {
    var node = document.getElementById('cal-almanac-data');
    if (!node) throw new Error('cal-almanac-data JSON node missing');
    try { DATA = JSON.parse(node.textContent || '{}'); }
    catch (e) { throw new Error('cal-almanac-data JSON parse failed: ' + e.message); }
  });

  // ============================================================
  // Block 2 — widget drag (header handle moves the widget).
  // ============================================================
  registerInitBlock('widget-drag', function () {
    var widget = document.querySelector('[data-cal-widget]');
    var handle = document.querySelector('[data-cal-drag-handle]');
    if (!widget || !handle) return;
    var startX = 0, startY = 0, startLeft = 0, startTop = 0, dragging = false;
    handle.addEventListener('pointerdown', function (ev) {
      // Don't initiate drag from inside the action buttons.
      if (ev.target.closest('.cal-almanac-iconbtn')) return;
      dragging = true;
      widget.setAttribute('data-cal-dragging', 'true');
      try { handle.setPointerCapture(ev.pointerId); } catch (e) {}
      startX = ev.clientX; startY = ev.clientY;
      var rect = widget.getBoundingClientRect();
      startLeft = rect.left; startTop = rect.top;
      // Convert to absolute coords on first drag so transform-free
      // movement works against the document scroll.
      widget.style.left = startLeft + window.scrollX + 'px';
      widget.style.top  = startTop  + window.scrollY + 'px';
      ev.preventDefault();
    });
    handle.addEventListener('pointermove', function (ev) {
      if (!dragging) return;
      widget.style.left = (startLeft + window.scrollX + (ev.clientX - startX)) + 'px';
      widget.style.top  = (startTop  + window.scrollY + (ev.clientY - startY)) + 'px';
    });
    function end(ev) {
      if (!dragging) return;
      dragging = false;
      widget.removeAttribute('data-cal-dragging');
      try { handle.releasePointerCapture(ev.pointerId); } catch (e) {}
    }
    handle.addEventListener('pointerup', end);
    handle.addEventListener('pointercancel', end);
  });

  // ============================================================
  // Block 3 — widget resize (bottom-right corner).
  // ============================================================
  registerInitBlock('widget-resize', function () {
    var widget = document.querySelector('[data-cal-widget]');
    var grip = document.querySelector('[data-cal-resize]');
    if (!widget || !grip) return;
    var startX = 0, startY = 0, startW = 0, startH = 0, resizing = false;
    grip.addEventListener('pointerdown', function (ev) {
      resizing = true;
      try { grip.setPointerCapture(ev.pointerId); } catch (e) {}
      startX = ev.clientX; startY = ev.clientY;
      var rect = widget.getBoundingClientRect();
      startW = rect.width; startH = rect.height;
      ev.stopPropagation();
      ev.preventDefault();
    });
    grip.addEventListener('pointermove', function (ev) {
      if (!resizing) return;
      widget.style.width  = Math.max(520, startW + (ev.clientX - startX)) + 'px';
      widget.style.height = Math.max(460, startH + (ev.clientY - startY)) + 'px';
    });
    function end(ev) {
      if (!resizing) return;
      resizing = false;
      try { grip.releasePointerCapture(ev.pointerId); } catch (e) {}
    }
    grip.addEventListener('pointerup', end);
    grip.addEventListener('pointercancel', end);
  });

  // ============================================================
  // Block 4 — sky-band scrubber: repaints gradient + sun/moon arc.
  // ============================================================
  registerInitBlock('sky-scrubber', function () {
    var sky = document.querySelector('[data-cal-sky]');
    var scrub = document.querySelector('[data-cal-sky-scrub]');
    var sun = document.querySelector('[data-cal-sky-sun]');
    var moons = Array.prototype.slice.call(document.querySelectorAll('[data-cal-sky-moon]'));
    var label = document.querySelector('[data-cal-sky-label]');
    var time = document.querySelector('[data-cal-sky-time]');
    if (!sky || !scrub) return;

    // 5 keyframes: midnight / dawn / noon / dusk / midnight (wrap).
    var KEY = [
      { top: 'oklch(0.18 0.05 270)', bot: 'oklch(0.10 0.04 270)' },
      { top: 'oklch(0.55 0.13 30)',  bot: 'oklch(0.35 0.10 305)' },
      { top: 'oklch(0.78 0.13 220)', bot: 'oklch(0.62 0.10 230)' },
      { top: 'oklch(0.62 0.16 60)',  bot: 'oklch(0.38 0.12 350)' },
      { top: 'oklch(0.18 0.05 270)', bot: 'oklch(0.10 0.04 270)' },
    ];
    function gradAt(t) {
      // Find segment + lerp via color-mix percentage.
      if (t < 0) t = 0; if (t > 1) t = 1;
      var pos = t * 4;
      var i = Math.min(3, Math.floor(pos));
      var f = pos - i;
      var a = KEY[i], b = KEY[i + 1];
      var top = 'color-mix(in oklch, ' + a.top + ' ' + ((1 - f) * 100).toFixed(1) + '%, ' + b.top + ')';
      var bot = 'color-mix(in oklch, ' + a.bot + ' ' + ((1 - f) * 100).toFixed(1) + '%, ' + b.bot + ')';
      return 'linear-gradient(180deg, ' + top + ' 0%, ' + bot + ' 100%)';
    }
    function arcPos(t) {
      // Same math as the server-side arcPositionStyle.
      var wake = (t - 0.25) / 0.5;
      if (wake < -0.1 || wake > 1.1) return { left: 50, top: 120, opacity: 0 };
      var x = 8 + wake * 84;
      var y = 95 - 4 * wake * (1 - wake) * 85;
      return { left: x, top: y, opacity: (wake < 0 || wake > 1) ? 0.6 : 1 };
    }
    function labelFor(t) {
      if (t < 0.20) return 'Pre-dawn';
      if (t < 0.32) return 'Dawn';
      if (t < 0.45) return 'Morning';
      if (t < 0.55) return 'Midday';
      if (t < 0.70) return 'Afternoon';
      if (t < 0.82) return 'Dusk';
      return 'Night';
    }
    function clockFor(t) {
      var hpd = (DATA && DATA.calendar && DATA.calendar.hours_per_day) || 24;
      var total = Math.floor(t * hpd * 60);
      var h = Math.floor(total / 60);
      var m = total % 60;
      return String(h).padStart(2, '0') + ':' + String(m).padStart(2, '0');
    }
    function applyPos(el, p) {
      el.style.left = p.left.toFixed(1) + '%';
      el.style.top  = p.top.toFixed(1) + '%';
      el.style.opacity = p.opacity.toFixed(2);
    }
    function paint(t) {
      sky.style.background = gradAt(t);
      if (sun) applyPos(sun, arcPos(t));
      moons.forEach(function (m) {
        // Find the moon's offset from DATA.
        var id = parseInt(m.getAttribute('data-moon-id'), 10);
        var offset = 0;
        if (DATA && Array.isArray(DATA.moons)) {
          for (var i = 0; i < DATA.moons.length; i++) {
            if (DATA.moons[i].id === id) { offset = DATA.moons[i].phase_offset; break; }
          }
        }
        var mt = t - 0.5 + offset;
        while (mt < 0) mt += 1;
        while (mt > 1) mt -= 1;
        applyPos(m, arcPos(mt));
      });
      if (label) {
        var subText = labelFor(t);
        // Preserve the "· season · weather" suffix from the server render.
        var existing = label.textContent || '';
        var bullet = existing.indexOf(' · ');
        var suffix = bullet >= 0 ? existing.substring(bullet) : '';
        label.textContent = subText + suffix;
      }
      if (time) time.textContent = clockFor(t);
    }
    scrub.addEventListener('input', function () {
      var t = parseInt(scrub.value, 10) / 1000;
      paint(t);
    });
    // Initial paint (in case the server snap-to-quarter doesn't match
    // the exact data t).
    paint(parseInt(scrub.value, 10) / 1000);
  });

  // ============================================================
  // Block 5 — event/day click -> anchored popover.
  //
  // Replaces the prior right-edge drawer for quick interactions:
  // clicking a chip pops up FROM that chip; clicking a day cell pops
  // up FROM the cell. Heavy edits still open the side drawer via the
  // popover's "Open full editor →" footer button.
  // ============================================================
  function findEventById(id) {
    if (!DATA) return null;
    // Recurring instances have IDs like "rec-session@1492-4-14"; the
    // event isn't in DATA.events, but the popover can still display
    // the override + template description.
    if (id && id.indexOf('@') !== -1) {
      var parts = id.split('@');
      var recId = parts[0];
      var key = parts[1];
      var rec = (DATA.recurring || []).find(function (r) { return r.id === recId; });
      if (rec) {
        var name = (rec.overrides && rec.overrides[key]) || rec.name;
        var dParts = key.split('-');
        return {
          id: id,
          name: name,
          description: rec.description,
          year: parseInt(dParts[0], 10),
          month: parseInt(dParts[1], 10),
          day: parseInt(dParts[2], 10),
          hour: rec.hour,
          tier: rec.tier,
          category: rec.category,
          visibility: 'public',
          recurring_ref: rec.id,
        };
      }
    }
    return (DATA.events || []).find(function (e) { return e.id === id; }) || null;
  }
  function dayWeather(m, day) {
    if (!DATA || !DATA.day_weather) return null;
    var key = DATA.current_year + '-' + m + '-' + day;
    var wid = DATA.day_weather[key];
    if (!wid) return null;
    return (DATA.weather_types || []).find(function (w) { return w.id === wid; }) || null;
  }
  function dayNote(m, day) {
    if (!DATA || !DATA.day_notes) return '';
    return DATA.day_notes[DATA.current_year + '-' + m + '-' + day] || '';
  }
  function setDayNote(m, day, text) {
    if (!DATA) return;
    if (!DATA.day_notes) DATA.day_notes = {};
    DATA.day_notes[DATA.current_year + '-' + m + '-' + day] = text;
  }
  function eventsForDay(m, day) {
    if (!DATA) return [];
    var out = [];
    (DATA.events || []).forEach(function (e) {
      if (eventTouchesDay(e, m, day)) out.push(e);
    });
    (DATA.recurring || []).forEach(function (rec) {
      if (recurringTouchesDay(rec, DATA.current_year, m, day)) {
        out.push(findEventById(rec.id + '@' + DATA.current_year + '-' + m + '-' + day));
      }
    });
    return out;
  }
  function eventTouchesDay(e, m, day) {
    if (!e.end_month) return e.month === m && e.day === day;
    if (e.month === m && day >= e.day) {
      if (e.end_month === m && day <= e.end_day) return true;
      if (e.end_month > m) return true;
    }
    if (e.end_month === m && day <= e.end_day && e.month < m) return true;
    if (e.month < m && e.end_month > m) return true;
    return false;
  }
  function recurringTouchesDay(rec, y, m, day) {
    if (!rec.interval_days) return false;
    var start = (rec.start_month - 1) * 30 + rec.start_day;
    var cur = (m - 1) * 30 + day;
    if (cur < start) return false;
    return (cur - start) % rec.interval_days === 0;
  }
  function categoryById(id) {
    if (!DATA) return null;
    return (DATA.categories || []).find(function (c) { return c.id === id; }) || null;
  }
  function moonPhaseFor(m, day) {
    if (!DATA || !DATA.moons || !DATA.moons.length) return null;
    var moon = DATA.moons[0];
    var epochDays = DATA.current_year * 360 + (m - 1) * 30 + day;
    var phase = epochDays / moon.cycle_days + moon.phase_offset;
    while (phase < 0) phase += 1;
    while (phase > 1) phase -= 1;
    var pct = Math.floor(phase * 100);
    var named = (DATA.moon_phases || []).find(function (mp) {
      return mp.moon_id === moon.id && pct >= mp.start_pct && pct < mp.end_pct;
    });
    return { moon: moon, pct: pct, name: named ? named.name : null, glyph: named ? named.glyph : null };
  }

  registerInitBlock('event-click', function () {
    var pop = document.querySelector('[data-cal-pop]');
    if (!pop) return;
    document.addEventListener('click', function (ev) {
      // Ignore clicks inside the popover itself (tabs, notes, etc.).
      if (ev.target.closest('[data-cal-pop]')) return;
      // Ignore clicks on the timepiece widget so it doesn't open
      // an event-popover on top of itself.
      if (ev.target.closest('[data-cal-time]')) return;

      var chip = ev.target.closest('[data-cal-event-id]');
      var cell = ev.target.closest('[data-cal-cell]');
      if (chip) {
        openEventPopover(findEventById(chip.getAttribute('data-cal-event-id')), chip);
        return;
      }
      if (cell) {
        var m = parseInt(cell.getAttribute('data-cell-month'), 10);
        var day = parseInt(cell.getAttribute('data-cell-day'), 10);
        openDayPopover(m, day, cell);
        return;
      }
      // Click outside calendar surfaces → close popover.
      closePopover();
    });
  });

  function openEventPopover(ev, anchor) {
    if (!ev || !anchor) return;
    var cat = categoryById(ev.category) || {};
    setPopoverTitle(ev.name, monthName(ev.month) + ' ' + ev.day + ' · ' + cat.name + (ev.hour >= 0 ? ' · ' + String(ev.hour).padStart(2, '0') + ':00' : ''));
    setPopoverDesc(ev.description || '(No description.)');
    var note = ev.recurring_ref ? '<p style="margin:6px 0 0;font-size:11px;color:oklch(0.74 0.022 261)">↻ Recurring (' + ev.recurring_ref + ') — edits affect this instance only.</p>' : '';
    var eventsBox = document.querySelector('[data-cal-pop-events]');
    if (eventsBox) eventsBox.innerHTML = note;
    setPopoverMoonWeather(null, null);
    hydrateVisibility(ev);
    var noteEl = document.querySelector('[data-cal-pop-notes]');
    if (noteEl) noteEl.value = ''; // events don't carry per-day notes
    rememberPopoverContext({ kind: 'event', event: ev, day: null, month: null });
    positionAndShowPopover(anchor);
  }
  function openDayPopover(m, day, anchor) {
    var wd = (DATA && DATA.weekdays) ? DATA.weekdays[(day - 1) % DATA.weekdays.length] : null;
    setPopoverTitle(monthName(m) + ' ' + day, (wd ? wd.name : '') + ' · ' + DATA.current_year + ' ' + DATA.calendar.epoch_name);
    setPopoverDesc('Day at a glance — events, weather, moon phase, notes.');
    var events = eventsForDay(m, day);
    var eventsBox = document.querySelector('[data-cal-pop-events]');
    if (eventsBox) {
      eventsBox.innerHTML = '';
      events.forEach(function (e) {
        if (!e) return;
        var cat = categoryById(e.category) || {};
        var row = document.createElement('div');
        row.className = 'cal-almanac-pop__event';
        row.style.setProperty('--chip-cat', cat.color || 'oklch(0.62 0.18 240)');
        row.innerHTML =
          '<span class="cal-almanac-pop__event-icon">●</span>' +
          '<span class="cal-almanac-pop__event-name">' + escapeHTML(e.name) + '</span>' +
          (e.hour >= 0 ? '<span class="cal-almanac-pop__event-time">' + String(e.hour).padStart(2, '0') + ':00</span>' : '');
        row.addEventListener('click', function () { openEventPopover(e, anchor); });
        eventsBox.appendChild(row);
      });
      if (!events.length) {
        var emp = document.createElement('div');
        emp.style.cssText = 'font-size:11px;color:oklch(0.62 0.022 261);font-style:italic;padding:4px 0;';
        emp.textContent = 'No events on this day.';
        eventsBox.appendChild(emp);
      }
    }
    var w = dayWeather(m, day);
    var mp = moonPhaseFor(m, day);
    setPopoverMoonWeather(w, mp);
    var noteEl = document.querySelector('[data-cal-pop-notes]');
    if (noteEl) noteEl.value = dayNote(m, day);
    // Default day-popover visibility editor: public.
    hydrateVisibility({ visibility: 'public', allow_users: [], deny_users: [] });
    rememberPopoverContext({ kind: 'day', event: null, day: day, month: m });
    positionAndShowPopover(anchor);
  }
  function setPopoverTitle(t, meta) {
    var titleEl = document.querySelector('[data-cal-pop-title]');
    var metaEl = document.querySelector('[data-cal-pop-meta]');
    if (titleEl) titleEl.textContent = t;
    if (metaEl) metaEl.textContent = meta;
  }
  function setPopoverDesc(t) {
    var d = document.querySelector('[data-cal-pop-desc]');
    if (d) d.textContent = t;
  }
  function setPopoverMoonWeather(w, mp) {
    var mw = document.querySelector('[data-cal-pop-moon]');
    var ww = document.querySelector('[data-cal-pop-weather]');
    if (mw) {
      if (mp && mp.name) {
        mw.removeAttribute('hidden');
        mw.innerHTML = '<strong>' + (mp.glyph || '') + '</strong> ' + escapeHTML(mp.moon.name) + ' — ' + escapeHTML(mp.name) + ' <span style="opacity:.6;font-family:JetBrains Mono,monospace">(' + mp.pct + '%)</span>';
      } else {
        mw.setAttribute('hidden', '');
      }
    }
    if (ww) {
      if (w) {
        ww.removeAttribute('hidden');
        ww.style.setProperty('--w-color', w.color);
        ww.style.borderLeft = '3px solid ' + w.color;
        ww.innerHTML = '<strong>' + escapeHTML(w.name) + '</strong> <span style="opacity:.7">·</span> ' + w.temp_c + '°C <span style="opacity:.7">·</span> ' + escapeHTML(w.category);
      } else {
        ww.setAttribute('hidden', '');
      }
    }
  }
  var popoverCtx = null;
  function rememberPopoverContext(ctx) { popoverCtx = ctx; }
  function positionAndShowPopover(anchor) {
    var pop = document.querySelector('[data-cal-pop]');
    if (!pop || !anchor) return;
    pop.removeAttribute('hidden');
    pop.setAttribute('aria-hidden', 'false');
    // Position: prefer above the anchor; flip below if no room.
    var ar = anchor.getBoundingClientRect();
    var pw = 360;
    var ph = 320; // estimate; we re-measure after show
    var vw = window.innerWidth;
    var vh = window.innerHeight;
    var left = ar.left + (ar.width / 2) - (pw / 2);
    left = Math.max(8, Math.min(left, vw - pw - 8));
    var side = 'top';
    var top = ar.top - ph - 10;
    if (top < 8) {
      side = 'bottom';
      top = ar.bottom + 10;
    }
    pop.style.left = left + 'px';
    pop.style.top  = top + 'px';
    pop.setAttribute('data-cal-pop-side', side);
    pop.setAttribute('data-cal-pop-open', 'true');
    // Re-position arrow to point at the anchor's center within the popover's width.
    var arrow = pop.querySelector('[data-cal-pop-arrow]');
    if (arrow) {
      var anchorCenterAbs = ar.left + ar.width / 2;
      var arrowLeft = Math.max(12, Math.min(anchorCenterAbs - left - 5, pw - 18));
      arrow.style.left = arrowLeft + 'px';
      arrow.style.marginLeft = '0';
    }
  }
  function closePopover() {
    var pop = document.querySelector('[data-cal-pop]');
    if (!pop) return;
    pop.setAttribute('data-cal-pop-open', 'false');
    pop.setAttribute('aria-hidden', 'true');
    setTimeout(function () { pop.setAttribute('hidden', ''); }, 220);
  }
  function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  // ============================================================
  // Block 5b — popover tab switcher + close + escalate to drawer.
  // ============================================================
  registerInitBlock('popover-controls', function () {
    var pop = document.querySelector('[data-cal-pop]');
    if (!pop) return;
    // Tabs.
    pop.querySelectorAll('[data-cal-pop-tab]').forEach(function (t) {
      t.addEventListener('click', function () {
        var name = t.getAttribute('data-cal-pop-tab');
        pop.querySelectorAll('[data-cal-pop-tab]').forEach(function (o) {
          o.classList.toggle('cal-almanac-pop__tab--active', o === t);
          o.setAttribute('aria-selected', o === t ? 'true' : 'false');
        });
        pop.querySelectorAll('[data-cal-pop-panel]').forEach(function (p) {
          var active = p.getAttribute('data-cal-pop-panel') === name;
          p.classList.toggle('cal-almanac-pop__panel--active', active);
          if (active) p.removeAttribute('hidden');
          else p.setAttribute('hidden', '');
        });
      });
    });
    // Close.
    var close = pop.querySelector('[data-cal-pop-close]');
    if (close) close.addEventListener('click', closePopover);
    document.addEventListener('keydown', function (ev) {
      if (ev.key === 'Escape') closePopover();
    });
    // Notes — persist into in-memory mock on input.
    var notes = pop.querySelector('[data-cal-pop-notes]');
    if (notes) notes.addEventListener('input', function () {
      if (popoverCtx && popoverCtx.kind === 'day') {
        setDayNote(popoverCtx.month, popoverCtx.day, notes.value);
      }
    });
    // Escalate to the side drawer for heavy edits.
    var escal = pop.querySelector('[data-cal-pop-open-drawer]');
    if (escal) escal.addEventListener('click', function () {
      var drawer = document.querySelector('[data-cal-drawer]');
      if (!drawer) return;
      var titleEl = document.querySelector('[data-cal-drawer-title]');
      var metaEl  = document.querySelector('[data-cal-drawer-meta]');
      var descEl  = document.querySelector('[data-cal-drawer-desc]');
      if (popoverCtx && popoverCtx.event) {
        var e = popoverCtx.event;
        if (titleEl) titleEl.textContent = e.name;
        if (metaEl) metaEl.textContent = monthName(e.month) + ' ' + e.day + ' · tier ' + e.tier + ' · ' + e.category;
        if (descEl) descEl.textContent = e.description || '(no description)';
        hydrateVisibility(e);
      } else if (popoverCtx && popoverCtx.kind === 'day') {
        if (titleEl) titleEl.textContent = monthName(popoverCtx.month) + ' ' + popoverCtx.day;
        if (metaEl) metaEl.textContent = 'Day editor';
        if (descEl) descEl.textContent = 'Edit events, weather, moon override, and per-day notes for this date.';
        hydrateVisibility({ visibility: 'public' });
      }
      closePopover();
      drawer.setAttribute('data-cal-drawer-open', 'true');
      drawer.setAttribute('aria-hidden', 'false');
    });
  });

  function monthName(idx1) {
    if (!DATA || !DATA.months) return String(idx1);
    var m = DATA.months[idx1 - 1];
    return m ? m.name : String(idx1);
  }

  // ============================================================
  // Block 6 — drawer close (button + Escape).
  // ============================================================
  registerInitBlock('drawer-close', function () {
    var drawer = document.querySelector('[data-cal-drawer]');
    var btn = document.querySelector('[data-cal-drawer-close]');
    if (!drawer) return;
    function close() {
      drawer.setAttribute('data-cal-drawer-open', 'false');
      drawer.setAttribute('aria-hidden', 'true');
    }
    if (btn) btn.addEventListener('click', close);
    document.addEventListener('keydown', function (ev) {
      if (ev.key === 'Escape' && drawer.getAttribute('data-cal-drawer-open') === 'true') close();
    });
  });

  // ============================================================
  // Block 7 — visibility editor (Q-V2-7 chip-row builder).
  // Mock rules: editor lets operator add/remove allow/deny chips and
  // updates the effective-audience summary. No persistence.
  // ============================================================
  function hydrateVisibility(ev) {
    var radios = document.querySelectorAll('[data-cal-vis-mode]');
    var rules  = document.querySelector('[data-cal-vis-rules]');
    var chips  = document.querySelector('[data-cal-vis-chips]');
    if (!radios.length || !rules || !chips) return;
    var mode = (ev.visibility === 'specific') ? 'specific' : 'public';
    radios.forEach(function (r) { r.checked = (r.value === mode); });
    if (mode === 'specific') rules.removeAttribute('hidden');
    else rules.setAttribute('hidden', '');
    chips.innerHTML = '';
    (ev.allow_users || []).forEach(function (u) { chips.appendChild(visChip('allow', u)); });
    (ev.deny_users  || []).forEach(function (u) { chips.appendChild(visChip('deny', u)); });
    refreshVisSummary();
  }
  function visChip(kind, name) {
    var li = document.createElement('li');
    li.className = 'cal-almanac-vis-chip cal-almanac-vis-chip--' + kind;
    li.setAttribute('data-vis-kind', kind);
    li.setAttribute('data-vis-name', name);
    var icon = document.createElement('span');
    icon.className = 'cal-almanac-vis-chip__icon';
    icon.textContent = kind === 'allow' ? '✓' : '✗';
    var n = document.createElement('span');
    n.className = 'cal-almanac-vis-chip__name';
    n.textContent = '@' + name;
    var del = document.createElement('button');
    del.type = 'button';
    del.className = 'cal-almanac-vis-chip__del';
    del.setAttribute('aria-label', 'Remove rule');
    del.textContent = '×';
    del.addEventListener('click', function () {
      li.remove();
      refreshVisSummary();
    });
    li.appendChild(icon); li.appendChild(n); li.appendChild(del);
    return li;
  }
  function refreshVisSummary() {
    var summary = document.querySelector('[data-cal-vis-summary]');
    if (!summary) return;
    var anySpec = document.querySelector('[data-cal-vis-mode][value="specific"]');
    if (!anySpec || !anySpec.checked) {
      summary.textContent = 'Effective audience: everyone';
      return;
    }
    var chips = document.querySelectorAll('[data-cal-vis-chips] [data-vis-kind]');
    var allow = [], deny = [];
    chips.forEach(function (c) {
      var n = c.getAttribute('data-vis-name');
      if (c.getAttribute('data-vis-kind') === 'allow') allow.push(n);
      else deny.push(n);
    });
    var msg;
    if (!allow.length && !deny.length) msg = 'No rules: nobody can see this.';
    else if (!allow.length) msg = 'Everyone except: ' + deny.join(', ');
    else if (!deny.length)  msg = allow.length + ' people can see this: ' + allow.join(', ');
    else                    msg = allow.join(', ') + ' (denied: ' + deny.join(', ') + ')';
    summary.textContent = 'Effective audience — ' + msg;
  }
  registerInitBlock('visibility-editor', function () {
    var radios = document.querySelectorAll('[data-cal-vis-mode]');
    var rules  = document.querySelector('[data-cal-vis-rules]');
    if (!radios.length || !rules) return;
    radios.forEach(function (r) {
      r.addEventListener('change', function () {
        if (r.checked && r.value === 'specific') rules.removeAttribute('hidden');
        if (r.checked && r.value === 'public')   rules.setAttribute('hidden', '');
        refreshVisSummary();
      });
    });
    document.querySelectorAll('[data-cal-vis-add]').forEach(function (b) {
      b.addEventListener('click', function () {
        var kind = b.getAttribute('data-cal-vis-add');
        var name = window.prompt('Add ' + kind + ' rule for which user?');
        if (!name) return;
        var chips = document.querySelector('[data-cal-vis-chips]');
        if (chips) chips.appendChild(visChip(kind, name.replace(/^@/, '')));
        refreshVisSummary();
      });
    });
  });

  // ============================================================
  // Block 8 — drag-to-create (drag across empty cells -> ghost ->
  // open drawer for stub event).
  // ============================================================
  registerInitBlock('drag-create', function () {
    var grid = document.querySelector('[data-cal-grid]');
    var ghost = document.querySelector('[data-cal-drag-ghost]');
    if (!grid || !ghost) return;
    var dragging = false, startCell = null;

    function cellAt(x, y) {
      var el = document.elementFromPoint(x, y);
      return el && el.closest('[data-cal-cell]');
    }
    function positionGhost(a, b) {
      if (!a || !b) return;
      var ra = a.getBoundingClientRect();
      var rb = b.getBoundingClientRect();
      var gridRect = grid.getBoundingClientRect();
      var left = Math.min(ra.left, rb.left) - gridRect.left;
      var top  = Math.min(ra.top, rb.top)  - gridRect.top;
      var right = Math.max(ra.right, rb.right) - gridRect.left;
      var bottom = Math.max(ra.bottom, rb.bottom) - gridRect.top;
      ghost.style.left = left + 'px';
      ghost.style.top  = top + 'px';
      ghost.style.width  = (right - left) + 'px';
      ghost.style.height = (bottom - top) + 'px';
    }

    grid.addEventListener('pointerdown', function (ev) {
      // Don't start a drag-create from an event chip.
      if (ev.target.closest('[data-cal-event-id]')) return;
      var cell = ev.target.closest('[data-cal-cell]');
      if (!cell) return;
      dragging = true;
      startCell = cell;
      ghost.setAttribute('data-cal-ghost-active', 'true');
      positionGhost(startCell, startCell);
      try { grid.setPointerCapture(ev.pointerId); } catch (e) {}
      ev.preventDefault();
    });
    grid.addEventListener('pointermove', function (ev) {
      if (!dragging) return;
      var cell = cellAt(ev.clientX, ev.clientY);
      if (!cell) return;
      positionGhost(startCell, cell);
    });
    function end(ev) {
      if (!dragging) return;
      dragging = false;
      ghost.removeAttribute('data-cal-ghost-active');
      try { grid.releasePointerCapture(ev.pointerId); } catch (e) {}
      var endCell = cellAt(ev.clientX, ev.clientY) || startCell;
      // Open the drawer with a stub event for the selected range.
      var startDay = startCell.getAttribute('data-cell-day');
      var endDay = endCell.getAttribute('data-cell-day');
      openStubEvent(startDay, endDay);
    }
    grid.addEventListener('pointerup', end);
    grid.addEventListener('pointercancel', end);

    function openStubEvent(s, e) {
      var drawer = document.querySelector('[data-cal-drawer]');
      var titleEl = document.querySelector('[data-cal-drawer-title]');
      var metaEl  = document.querySelector('[data-cal-drawer-meta]');
      var descEl  = document.querySelector('[data-cal-drawer-desc]');
      if (!drawer || !titleEl) return;
      var range = (s === e) ? ('day ' + s) : ('days ' + s + '–' + e);
      titleEl.textContent = 'New event';
      if (metaEl) metaEl.textContent = 'tier: standard · ' + range;
      if (descEl) descEl.textContent = 'Drag-created event. Save would create it on the selected day(s); this showcase doesn’t persist.';
      hydrateVisibility({ visibility: 'public', allow_users: [], deny_users: [] });
      drawer.setAttribute('data-cal-drawer-open', 'true');
      drawer.setAttribute('aria-hidden', 'false');
    }
  });

  // ============================================================
  // Block 9 — month nav (mock: cycles the displayed month name in the
  // widget title; doesn't actually re-render the grid since this is a
  // showcase. A future real plugin port re-renders cells from data).
  // ============================================================
  registerInitBlock('month-nav', function () {
    var titleEl = document.querySelector('.cal-almanac-widget__title');
    var prev = document.querySelector('[data-cal-prev]');
    var next = document.querySelector('[data-cal-next]');
    var today = document.querySelector('[data-cal-today]');
    if (!titleEl || !DATA) return;
    var monthIdx = DATA.current_month - 1;
    function paint() {
      var m = DATA.months[monthIdx];
      titleEl.textContent = DATA.calendar.name + ' · ' + m.name + ' ' + DATA.current_year + ' ' + DATA.calendar.epoch_name;
    }
    if (prev)  prev.addEventListener('click',  function () { monthIdx = (monthIdx - 1 + DATA.months.length) % DATA.months.length; paint(); });
    if (next)  next.addEventListener('click',  function () { monthIdx = (monthIdx + 1) % DATA.months.length; paint(); });
    if (today) today.addEventListener('click', function () { monthIdx = DATA.current_month - 1; paint(); });
  });

  // ============================================================
  // Block 10 — timepiece widget. Draggable like the main widget;
  // its own state for the "tick +1h" button. Sky-band scrubber +
  // timepiece clock stay in sync (scrubbing sky updates the
  // timepiece, ticking the timepiece updates the sky).
  // ============================================================
  registerInitBlock('timepiece', function () {
    var widget = document.querySelector('[data-cal-time]');
    var handle = document.querySelector('[data-cal-time-drag]');
    var clockEl = document.querySelector('[data-cal-time-clock]');
    var tick = document.querySelector('[data-cal-time-tick]');
    var scrub = document.querySelector('[data-cal-sky-scrub]');
    if (!widget || !handle) return;

    // --- drag ---
    var dragging = false, sx = 0, sy = 0, sl = 0, st = 0;
    handle.addEventListener('pointerdown', function (ev) {
      dragging = true;
      widget.setAttribute('data-cal-time-dragging', 'true');
      try { handle.setPointerCapture(ev.pointerId); } catch (e) {}
      sx = ev.clientX; sy = ev.clientY;
      var rect = widget.getBoundingClientRect();
      sl = rect.left; st = rect.top;
      widget.style.left = sl + window.scrollX + 'px';
      widget.style.top  = st + window.scrollY + 'px';
      ev.preventDefault();
    });
    handle.addEventListener('pointermove', function (ev) {
      if (!dragging) return;
      widget.style.left = (sl + window.scrollX + (ev.clientX - sx)) + 'px';
      widget.style.top  = (st + window.scrollY + (ev.clientY - sy)) + 'px';
    });
    function dragEnd(ev) {
      if (!dragging) return;
      dragging = false;
      widget.removeAttribute('data-cal-time-dragging');
      try { handle.releasePointerCapture(ev.pointerId); } catch (e) {}
    }
    handle.addEventListener('pointerup', dragEnd);
    handle.addEventListener('pointercancel', dragEnd);

    // --- tick +1h: advances SkyTime by 1/hours_per_day; wraps at 1.0 ---
    if (tick && scrub) {
      tick.addEventListener('click', function () {
        var hpd = (DATA && DATA.calendar && DATA.calendar.hours_per_day) || 24;
        var step = 1 / hpd;
        var t = parseInt(scrub.value, 10) / 1000;
        t = (t + step) % 1.0;
        scrub.value = String(Math.round(t * 1000));
        scrub.dispatchEvent(new Event('input', { bubbles: true }));
      });
    }
    // Keep clock in sync with scrubber.
    if (scrub && clockEl) {
      scrub.addEventListener('input', function () {
        var hpd = (DATA && DATA.calendar && DATA.calendar.hours_per_day) || 24;
        var t = parseInt(scrub.value, 10) / 1000;
        var total = Math.floor(t * hpd * 60);
        var h = Math.floor(total / 60), m = total % 60;
        clockEl.textContent = String(h).padStart(2, '0') + ':' + String(m).padStart(2, '0');
      });
    }
  });

  // ============================================================
  // Trigger.
  // ============================================================
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
