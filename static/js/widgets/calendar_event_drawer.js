/**
 * calendar_event_drawer.js — the full event editor: a drawer that slides out
 * of the calendar's right edge (a sheet from the bottom on phones), as the
 * signed calendar and owner-editing mockups draw it. It sets everything the
 * events API stores about an event's when, what and who: title, all-day or
 * start and end times, start and end dates, how it repeats and when that
 * stops, its kind, notes, who sees it and whether players learn of it ahead
 * of time.
 *
 * calendar_editor.js opens it (Edit on an event, and "Add an event") through Chronicle.calendarEventDrawer.open; it renders nothing
 * by itself. Loaded on every page with the other calendar scripts and inert
 * until opened.
 *
 * The mockup's linked pages and RSVPs are not here: the events API has no
 * write surface for either yet (see the calendar plugin's .ai.md, "Honest
 * gaps"), so the drawer leaves them out rather than show controls that
 * would not save.
 */
(function () {
  'use strict';

  var ROLE_OWNER = 3;

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (ch) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch];
    });
  }
  function reducedMotion() { return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches; }

  // The same plain-text rules as the compact form in calendar_editor.js: a
  // rich description is edited as its text, and saved back as paragraphs.
  function descriptionText(ev) {
    if (!ev) return '';
    if (ev.description_html) {
      var div = document.createElement('div');
      div.innerHTML = ev.description_html;
      return div.textContent || '';
    }
    return ev.description || '';
  }
  function bodyToHTML(text) {
    return String(text).trim().split(/\n{2,}/).map(function (p) {
      return '<p>' + esc(p).replace(/\n/g, '<br>') + '</p>';
    }).join('');
  }

  // The API's recurrence types, labelled as the mockup labels them.
  var REPEATS = [['', 'Does not repeat'], ['weekly', 'Every week'], ['biweekly', 'Every other week'],
    ['custom', 'Every few weeks'], ['monthly', 'Every month'], ['yearly', 'Every year']];

  function pad2(n) { return (n < 10 ? '0' : '') + n; }
  function timeText(h, m) { return h == null ? '' : pad2(h) + ':' + pad2(m || 0); }
  // "19", "19:30" or "7:05", bounded by this calendar's own hours and
  // minutes. Returns null for empty, false for something unreadable.
  function parseTime(cal, raw) {
    var s = String(raw || '').trim();
    if (!s) return null;
    var m = /^(\d{1,2})(?:[:.](\d{1,2}))?$/.exec(s);
    if (!m) return false;
    var h = +m[1], mi = m[2] ? +m[2] : 0;
    if (h >= (cal.hours_per_day || 24) || mi >= (cal.minutes_per_hour || 60)) return false;
    return { h: h, m: mi };
  }
  function ord(cal, CalDate, d) { return CalDate.dayIndex(cal, d.y, d.m, d.d); }

  function Drawer(view) {
    this.view = view;
    this.cal = view.cal;
    this.CalDate = Chronicle.calendarDate;
    var dock = view.dockEl;
    this.scrim = $('.dscrim', dock);
    if (!this.scrim) { this.scrim = document.createElement('div'); this.scrim.className = 'dscrim'; dock.appendChild(this.scrim); }
    this.el = document.createElement('aside');
    this.el.className = 'drawer ded';
    this.el.setAttribute('role', 'dialog');
    this.el.setAttribute('aria-labelledby', 'cal5-edtitle');
    this.el.tabIndex = -1;
    dock.appendChild(this.el);
    this.state = null;
    this._bind();
  }

  Drawer.prototype.isOpen = function () { return this.el.classList.contains('open'); };

  // ev: the event to edit (null for a new one). day: {y,m,d} a new event
  // starts on.
  Drawer.prototype.open = function (ev, day) {
    var view = this.view, cal = this.cal;
    var back = document.activeElement;
    if (view.wingFor && (view.wingEl.contains(back) || view.evpEl.contains(back))) back = $('.day[data-key="' + view.wingFor + '"]', view.stageEl) || back;
    view.closeAllPanels({ instant: true });
    var isNew = !ev;
    var s = isNew ? (day || { y: cal.current_year, m: cal.current_month, d: cal.current_day }) : { y: ev.year, m: ev.month, d: ev.day };
    var e = (!isNew && ev.end_year != null) ? { y: ev.end_year, m: ev.end_month, d: ev.end_day } : s;
    var rtype = !isNew && ev.is_recurring && ev.recurrence_type ? ev.recurrence_type : '';
    this.state = {
      ev: ev, isNew: isNew, back: back,
      allDay: isNew ? true : (ev.all_day || ev.start_hour == null),
      kindId: !isNew && ev.kind_id != null ? ev.kind_id : null,
      vis: isNew ? 'everyone' : ev.visibility,
      announced: !isNew && ev.announced ? ev.announced : '',
      rep: {
        type: rtype,
        every: !isNew && ev.recurrence_interval ? ev.recurrence_interval : 3,
        end: !isNew && ev.recurrence_max_occurrences ? 'count' : (!isNew && ev.recurrence_end_year != null ? 'until' : 'never'),
        count: !isNew && ev.recurrence_max_occurrences ? ev.recurrence_max_occurrences : 4,
        until: !isNew && ev.recurrence_end_year != null ? { y: ev.recurrence_end_year, m: ev.recurrence_end_month, d: ev.recurrence_end_day } : s
      }
    };
    this.el.innerHTML = this._html(ev, s, e);
    void this.el.offsetWidth;
    this.el.classList.add('open');
    view.dockEl.classList.add('on');
    var title = $('#cal5-edT', this.el);
    setTimeout(function () { title.focus({ preventScroll: true }); }, reducedMotion() ? 20 : 260);
  };

  Drawer.prototype.close = function (quiet) {
    if (!this.isOpen()) return;
    var back = this.state && this.state.back;
    this.el.classList.remove('open');
    this.view.dockEl.classList.remove('on');
    this.state = null;
    if (!quiet && back && back.isConnected) back.focus({ preventScroll: true });
  };

  // ---- markup ----
  Drawer.prototype._monthDayFields = function (d, p, label) {
    var cal = this.cal, CalDate = this.CalDate, months = '';
    (cal.months || []).forEach(function (mo, i) {
      months += '<option value="' + (i + 1) + '"' + (i + 1 === d.m ? ' selected' : '') + '>' + esc(mo.name) + '</option>';
    });
    return '<select id="cal5-' + p + 'M" aria-label="' + label + ' month">' + months + '</select>' +
      '<select id="cal5-' + p + 'D" aria-label="' + label + ' day">' + this._dayOptions(d.y, d.m, d.d) + '</select>';
  };
  Drawer.prototype._dayOptions = function (y, m, sel) {
    var n = this.CalDate.monthDays(this.cal, m - 1, y), h = '';
    for (var d = 1; d <= n; d++) h += '<option value="' + d + '"' + (d === sel ? ' selected' : '') + '>' + d + '</option>';
    return h;
  };
  Drawer.prototype._repEndHTML = function () {
    var r = this.state.rep;
    return (r.type === 'custom' ? '<label class="rn">Every <input id="cal5-edRepI" type="number" min="2" max="52" value="' + Math.max(2, r.every) + '"> weeks</label>' : '') +
      '<div class="segs" role="group" aria-label="Repeat ends">' + [['never', 'Never ends'], ['count', 'After'], ['until', 'On a date']].map(function (o) {
        return '<button type="button" data-rend="' + o[0] + '" aria-pressed="' + (r.end === o[0]) + '">' + o[1] + '</button>';
      }).join('') + '</div>' +
      (r.end === 'count' ? '<label class="rn">After <input id="cal5-edRepN" type="number" min="1" max="999" value="' + r.count + '"> times</label>' : '') +
      (r.end === 'until' ? '<div class="erow"><span class="rl">Until</span>' + this._monthDayFields(r.until, 'eu', 'Repeat until') +
        '<input id="cal5-euY" type="number" aria-label="Until year" value="' + r.until.y + '"></div>' : '');
  };
  Drawer.prototype._kindChips = function () {
    var cur = this.state.kindId, sanitize = Chronicle.calendarColor;
    return '<button type="button" data-kind="" aria-pressed="' + (cur == null) + '">None</button>' +
      (this.cal.event_kinds || []).map(function (k) {
        var color = sanitize(k.color);
        return '<button type="button" data-kind="' + k.id + '" aria-pressed="' + (k.id === cur) + '"' + (color ? ' style="color:' + color + '"' : '') + '>' + esc(k.icon || '') + ' ' + esc(k.name) + '</button>';
      }).join('');
  };
  Drawer.prototype._html = function (ev, s, e) {
    var S = this.state, view = this.view;
    var name = S.isNew ? '' : ev.name;
    var h = '<div class="grab" aria-hidden="true"></div><div class="crease"><span id="cal5-edtitle">' + (S.isNew ? 'New event' : 'Edit event') + '</span>' +
      '<button type="button" class="x" data-close aria-label="Close the editor"><i class="fa-solid fa-xmark"></i></button></div>';
    h += '<div class="ebody"><label class="fld"><span class="sr">Title</span><input class="etitle" id="cal5-edT" placeholder="What is happening?" maxlength="255" value="' + esc(name) + '"></label>' +
      '<div class="err" id="cal5-edErr" role="alert" hidden></div>';
    h += '<section class="esec' + (S.allDay ? ' allday' : '') + '" id="cal5-edWhen"><h4>When</h4>' +
      '<div class="swrow"><span>All day</span><button type="button" class="tog" role="switch" aria-checked="' + S.allDay + '" aria-label="All day" data-e="allday"></button></div>' +
      '<div class="erow"><span class="rl">Starts</span>' + this._monthDayFields(s, 'es', 'Start') +
        '<input class="etime" id="cal5-esT" placeholder="19:00" aria-label="Start time" value="' + esc(S.isNew ? '' : timeText(ev.start_hour, ev.start_minute)) + '"></div>' +
      '<div class="erow"><span class="rl">Ends</span>' + this._monthDayFields(e, 'ee', 'End') +
        '<input class="etime" id="cal5-eeT" placeholder="22:00" aria-label="End time" value="' + esc(S.isNew ? '' : timeText(ev.end_hour, ev.end_minute)) + '"></div>' +
      '<label class="fld">Repeats<select class="erep" id="cal5-edRep">' + REPEATS.map(function (o) {
        return '<option value="' + o[0] + '"' + (o[0] === S.rep.type ? ' selected' : '') + '>' + o[1] + '</option>';
      }).join('') + '</select></label>' +
      '<div class="repend" id="cal5-edRepEnd"' + (S.rep.type ? '' : ' hidden') + '>' + (S.rep.type ? this._repEndHTML() : '') + '</div></section>';
    h += '<section class="esec"><h4>Details</h4><div class="fld">Kind<div class="chips" id="cal5-edKinds">' + this._kindChips() + '</div></div>' +
      '<label class="fld">Notes<textarea id="cal5-edD" placeholder="Optional. Shows on the event’s card and page.">' + esc(S.isNew ? '' : descriptionText(ev)) + '</textarea></label></section>';
    if (view.canAuthorDmOnly) {
      h += '<section class="esec"><h4>Who</h4><div class="segs" role="group" aria-label="Who can see it">' + [['everyone', 'Everyone'], ['dm_only', 'Director only']].map(function (o) {
        return '<button type="button" data-vis="' + o[0] + '" aria-pressed="' + (S.vis === o[0]) + '">' + o[1] + '</button>';
      }).join('') + '</div>' +
      '<div class="fld eann">Players see it<div class="segs" role="group" aria-label="Players see it">' + [['', 'As its kind says'], ['ahead', 'Ahead of time'], ['on_day', 'On the day']].map(function (o) {
        return '<button type="button" data-ann="' + o[0] + '" aria-pressed="' + (S.announced === o[0]) + '">' + o[1] + '</button>';
      }).join('') + '</div></div></section>';
    }
    h += '<button type="button" class="moreb" data-e="more" aria-expanded="false" aria-controls="cal5-edMore">More <i class="fa-solid fa-chevron-down"></i></button>' +
      '<div class="moresec" id="cal5-edMore" hidden><div class="fields" style="margin-top:0">' +
      '<label class="fld" style="flex:1">Start year<input id="cal5-esY" type="number" value="' + s.y + '"></label>' +
      '<label class="fld" style="flex:1">End year<input id="cal5-eeY" type="number" value="' + e.y + '"></label></div></div>';
    var canDelete = !S.isNew && view.role >= ROLE_OWNER;
    return h + '</div><div class="efoot">' + (canDelete ? '<button type="button" class="btn danger" data-del><i class="fa-solid fa-trash"></i> Delete</button>' : '') +
      '<span class="sp"></span><button type="button" class="btn quiet" data-close>Cancel</button><button type="button" class="btn primary" data-save>Save</button></div>';
  };

  // ---- reading the form ----
  Drawer.prototype._readDate = function (p, yearId) {
    var y = parseInt($('#cal5-' + (yearId || p + 'Y'), this.el).value, 10);
    return { y: isNaN(y) ? this.cal.current_year : y, m: +$('#cal5-' + p + 'M', this.el).value, d: +$('#cal5-' + p + 'D', this.el).value };
  };
  Drawer.prototype._fail = function (msg, focusEl) {
    var err = $('#cal5-edErr', this.el);
    err.textContent = msg;
    err.hidden = false;
    if (focusEl) focusEl.focus();
  };

  // Builds the request body. Every field the drawer shows is sent, null
  // where it is cleared: an update is partial (absent keeps the stored
  // value), so leaving a cleared time or end date out would keep the old one.
  Drawer.prototype._body = function () {
    var S = this.state, cal = this.cal, CalDate = this.CalDate, view = this.view;
    var title = $('#cal5-edT', this.el);
    var name = title.value.trim();
    if (!name) { this._fail('Give the event a title first.', title); return null; }
    var s = this._readDate('es'), e = this._readDate('ee');
    if (ord(cal, CalDate, e) < ord(cal, CalDate, s)) e = s;
    var st = null, et = null;
    if (!S.allDay) {
      st = parseTime(cal, $('#cal5-esT', this.el).value);
      et = parseTime(cal, $('#cal5-eeT', this.el).value);
      if (st === false) { this._fail('Write the start time as hours and minutes, like 19:30.', $('#cal5-esT', this.el)); return null; }
      if (et === false) { this._fail('Write the end time as hours and minutes, like 22:00.', $('#cal5-eeT', this.el)); return null; }
    }
    var allDay = !st && !et;
    var oneDay = ord(cal, CalDate, e) === ord(cal, CalDate, s);
    var descText = $('#cal5-edD', this.el).value;
    var body = {
      name: name,
      year: s.y, month: s.m, day: s.d,
      all_day: allDay,
      start_hour: st ? st.h : null, start_minute: st ? st.m : null,
      end_year: oneDay && !et ? null : e.y, end_month: oneDay && !et ? null : e.m, end_day: oneDay && !et ? null : e.d,
      end_hour: et ? et.h : null, end_minute: et ? et.m : null,
      kind_id: S.kindId
    };
    // Untouched notes are left out so rich text written elsewhere (the
    // event's page, Foundry) keeps its formatting; edited notes are saved
    // as the plain paragraphs this box can show.
    if (S.isNew || descText !== descriptionText(S.ev)) {
      body.description = descText.trim() ? descText : null;
      body.description_html = descText.trim() ? bodyToHTML(descText) : null;
    }
    var r = S.rep;
    body.is_recurring = !!r.type;
    body.recurrence_type = r.type || null;
    body.recurrence_interval = r.type === 'custom' ? Math.max(2, parseInt(($('#cal5-edRepI', this.el) || {}).value, 10) || 3) : null;
    body.recurrence_max_occurrences = r.type && r.end === 'count' ? Math.max(1, parseInt(($('#cal5-edRepN', this.el) || {}).value, 10) || 1) : null;
    var until = r.type && r.end === 'until' ? this._readDate('eu') : null;
    body.recurrence_end_year = until ? until.y : null;
    body.recurrence_end_month = until ? until.m : null;
    body.recurrence_end_day = until ? until.d : null;
    if (view.canAuthorDmOnly) {
      body.visibility = S.vis;
      body.announced = S.announced || null;
    }
    return { body: body, start: s };
  };

  Drawer.prototype._save = function () {
    var self = this, S = this.state, view = this.view;
    var out = this._body();
    if (!out) return;
    var btn = $('[data-save]', this.el);
    btn.disabled = true;
    var req = S.isNew
      ? Chronicle.apiFetch(view.apiBase + '/events', { method: 'POST', body: out.body })
      : Chronicle.apiFetch(view.apiBase + '/events/' + S.ev.id, { method: 'PUT', body: out.body });
    req.then(function (resp) {
      if (!resp.ok) {
        btn.disabled = false;
        return resp.json().then(function (j) { self._fail((j && (j.message || j.error)) || 'That didn’t save. Try again.'); }, function () { self._fail('That didn’t save. Try again.'); });
      }
      self.close(true);
      view.eventsByMonth = {};
      if (out.start.y !== view.view.y || out.start.m !== view.view.m) view.view = { y: out.start.y, m: out.start.m };
      view.renderMonth();
      view.say('Saved ' + out.body.name + '.');
      var cell = $('.day[data-key="' + out.start.y + '_' + out.start.m + '_' + out.start.d + '"]', view.stageEl);
      if (cell) cell.focus({ preventScroll: true });
    }, function () {
      btn.disabled = false;
      self._fail('That didn’t save. Check your connection and try again.');
    });
  };

  Drawer.prototype._delete = function () {
    var self = this, view = this.view, ev = this.state.ev;
    if (!window.confirm('Delete this event? This cannot be undone.')) return;
    Chronicle.apiFetch(view.apiBase + '/events/' + ev.id, { method: 'DELETE' }).then(function (resp) {
      if (!resp.ok) { self._fail('Could not delete the event.'); return; }
      self.close(true);
      view.eventsByMonth = {};
      view.renderMonth();
      view.say('Event deleted.');
    });
  };

  Drawer.prototype._bind = function () {
    var self = this, el = this.el;
    this.scrim.addEventListener('click', function () { if (self.isOpen()) self.close(); });
    el.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && self.isOpen()) { e.stopPropagation(); self.close(); }
    });
    el.addEventListener('change', function (e) {
      var t = e.target, S = self.state;
      if (!S) return;
      // A month's days depend on its month (and year, for leap days).
      var mm = /^cal5-(es|ee|eu)M$/.exec(t.id);
      if (mm) {
        var p = mm[1], yEl = $('#cal5-' + (p === 'eu' ? 'euY' : p + 'Y'), el);
        var y = parseInt(yEl && yEl.value, 10) || self.cal.current_year;
        $('#cal5-' + p + 'D', el).innerHTML = self._dayOptions(y, +t.value, 1);
      }
      if (t.id === 'cal5-edRep') {
        S.rep.type = t.value;
        var re = $('#cal5-edRepEnd', el);
        re.hidden = !S.rep.type;
        re.innerHTML = S.rep.type ? self._repEndHTML() : '';
      }
    });
    el.addEventListener('input', function (e) {
      if (!self.state) return;
      if (e.target.id === 'cal5-edRepI') self.state.rep.every = parseInt(e.target.value, 10) || 3;
      if (e.target.id === 'cal5-edRepN') self.state.rep.count = parseInt(e.target.value, 10) || 1;
      if (e.target.id === 'cal5-edT') $('#cal5-edErr', el).hidden = true;
    });
    el.addEventListener('click', function (e) {
      var t = e.target.closest('button'), S = self.state;
      if (!t || !S) return;
      var press = function (sel) { $$(sel, el).forEach(function (b) { b.setAttribute('aria-pressed', String(b === t)); }); };
      if (t.hasAttribute('data-close')) self.close();
      else if (t.hasAttribute('data-save')) self._save();
      else if (t.hasAttribute('data-del')) self._delete();
      else if (t.hasAttribute('data-kind')) { S.kindId = t.dataset.kind === '' ? null : +t.dataset.kind; press('[data-kind]'); }
      else if (t.hasAttribute('data-vis')) { S.vis = t.dataset.vis; press('[data-vis]'); }
      else if (t.hasAttribute('data-ann')) { S.announced = t.dataset.ann; press('[data-ann]'); }
      else if (t.dataset.e === 'allday') {
        S.allDay = !S.allDay;
        t.setAttribute('aria-checked', String(S.allDay));
        $('#cal5-edWhen', el).classList.toggle('allday', S.allDay);
        if (!S.allDay) $('#cal5-esT', el).focus();
      } else if (t.dataset.e === 'more') {
        var m = $('#cal5-edMore', el), open = m.hidden;
        m.hidden = !open;
        t.setAttribute('aria-expanded', String(open));
      } else if (t.hasAttribute('data-rend')) {
        S.rep.end = t.dataset.rend;
        $('#cal5-edRepEnd', el).innerHTML = self._repEndHTML();
      }
    });
  };

  // One drawer per mounted calendar, made the first time it is asked for.
  window.Chronicle = window.Chronicle || {};
  Chronicle.calendarEventDrawer = {
    open: function (view, ev, day) {
      if (!view._eventDrawer) view._eventDrawer = new Drawer(view);
      view._eventDrawer.open(ev, day);
    },
    isOpen: function (view) { return !!(view._eventDrawer && view._eventDrawer.isOpen()); }
  };
})();
