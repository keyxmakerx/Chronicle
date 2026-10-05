/**
 * calendar_game_night.js — the game night editor: the same drawer as the
 * event editor, sliding out of the calendar, for planning a game night,
 * changing it (name, first night, start time, how it repeats, when it
 * stops, the note for players) or cancelling it. Game nights belong to the
 * sessions plugin, so the drawer talks to its API: a new night posts the
 * New Session form's fields, a change sends only the fields that changed
 * (the sessions update is partial), and cancelling is the soft delete that
 * tells whoever had answered.
 *
 * calendar_view.js opens it ("Plan a game night" in a day's card, the quick
 * form's "Full editor", and Edit on a night's page) through
 * Chronicle.calendarGameNight.open(view, night, iso). The pure part (the
 * next nights and the request bodies) is on Chronicle.calendarGameNight too,
 * for test/js/calendar_game_night.test.mjs.
 */
(function () {
  'use strict';

  var NAME_MAX = 200;
  var SUMMARY_MAX = 500;

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (ch) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch];
    });
  }
  function pad2(n) { return (n < 10 ? '0' : '') + n; }
  function reducedMotion() { return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches; }
  function isIso(s) { return /^\d{4}-\d{2}-\d{2}$/.test(s || ''); }

  // How a game night can repeat: the sessions plugin's recurrence types.
  // "custom" is every N weeks.
  var REPEATS = [['', 'Does not repeat'], ['weekly', 'Every week'], ['biweekly', 'Every other week'],
    ['custom', 'Every few weeks'], ['monthly', 'Every month']];

  // The date after iso for a repeat, stepped exactly as the server's
  // computeNextOccurrence steps it (a month is Go's AddDate, which rolls
  // Jan 31 over into March, and JavaScript's setUTCMonth does the same).
  function stepDate(iso, type, every) {
    var p = iso.split('-'), d = new Date(Date.UTC(+p[0], +p[1] - 1, +p[2]));
    if (type === 'weekly') d.setUTCDate(d.getUTCDate() + 7);
    else if (type === 'biweekly') d.setUTCDate(d.getUTCDate() + 14);
    else if (type === 'custom') d.setUTCDate(d.getUTCDate() + 7 * Math.max(1, every || 1));
    else if (type === 'monthly') d.setUTCMonth(d.getUTCMonth() + 1);
    else return '';
    return d.getUTCFullYear() + '-' + pad2(d.getUTCMonth() + 1) + '-' + pad2(d.getUTCDate());
  }

  // The first n nights of a series starting on start, from the first one on
  // or after from, stopping at until (inclusive) when set.
  function nextNights(start, type, every, until, n, from) {
    if (!isIso(start)) return [];
    var out = [], cur = start;
    for (var i = 0; i < 520 && out.length < n; i++) {
      if (until && cur > until) break;
      if (!from || cur >= from) out.push(cur);
      if (!type) break;
      var next = stepDate(cur, type, every);
      if (!next || next <= cur) break;
      cur = next;
    }
    return out;
  }

  // The drawer's form state from a night (or a new one on iso).
  function stateFor(night, iso) {
    if (!night) return { name: 'Game night', date: iso, time: '19:00', repeat: '', every: 3, end: 'never', until: '', summary: '' };
    var type = night.recurring ? (night.recurrenceType || '') : '';
    var every = type === 'custom' ? Math.max(2, night.recurrenceInterval || 3) : 3;
    return {
      name: night.name || '',
      date: night.seriesDate || night.date,
      time: night.time || '',
      repeat: type,
      every: every,
      end: night.recurrenceEndDate ? 'until' : 'never',
      until: night.recurrenceEndDate || '',
      summary: night.summary || ''
    };
  }

  // What the form says is wrong, or ''.
  function problem(s) {
    if (!(s.name || '').trim()) return 'Give the game night a name.';
    if ((s.name || '').trim().length > NAME_MAX) return 'Keep the name under ' + NAME_MAX + ' characters.';
    if (!isIso(s.date)) return 'Pick the day it starts.';
    if (s.time && !/^\d{2}:\d{2}$/.test(s.time)) return 'Pick a start time.';
    if (s.repeat === 'custom' && !(s.every >= 2 && s.every <= 52)) return 'Repeat every 2 to 52 weeks.';
    if (s.repeat && s.end === 'until') {
      if (!isIso(s.until)) return 'Pick the last day it repeats.';
      if (s.until < s.date) return 'The last night can’t be before the first.';
    }
    if ((s.summary || '').length > SUMMARY_MAX) return 'Keep the note under ' + SUMMARY_MAX + ' characters.';
    return '';
  }

  // A new night: the New Session form's own fields.
  function createFields(s, zone) {
    var out = { name: s.name.trim(), scheduled_date: s.date };
    if (s.time) { out.scheduled_time = s.time; if (zone) out.scheduled_tz = zone; }
    if (s.summary.trim()) out.summary = s.summary.trim();
    if (s.repeat) {
      out.is_recurring = '1';
      out.recurrence_type = s.repeat;
      if (s.repeat === 'custom') out.recurrence_interval = String(s.every);
      if (s.end === 'until') out.recurrence_end_date = s.until;
    }
    return out;
  }

  // A change: only what differs from before (absent keeps, null clears).
  function updateBody(before, s, zone) {
    var out = {}, name = s.name.trim(), summary = s.summary.trim();
    if (name !== before.name) out.name = name;
    if (summary !== (before.summary || '').trim()) out.summary = summary === '' ? null : summary;
    if (s.date !== before.date) out.scheduled_date = s.date;
    if (s.time !== before.time) {
      out.scheduled_time = s.time === '' ? null : s.time;
      if (s.time && zone) out.scheduled_tz = zone;
    }
    var until = s.repeat && s.end === 'until' ? s.until : '';
    var beforeUntil = before.repeat && before.end === 'until' ? before.until : '';
    if (s.repeat !== before.repeat) {
      out.is_recurring = !!s.repeat;
      out.recurrence_type = s.repeat || null;
    }
    if (s.repeat === 'custom' && (s.every !== before.every || before.repeat !== 'custom')) out.recurrence_interval = s.every;
    if (until !== beforeUntil) out.recurrence_end_date = until || null;
    return out;
  }

  // "Sat Oct 10" for a real date.
  function shortDate(iso) {
    var p = iso.split('-'), d = new Date(Date.UTC(+p[0], +p[1] - 1, +p[2]));
    try { return d.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric', timeZone: 'UTC' }); } catch (e) { return iso; }
  }

  function zoneAbbr(zone, iso) {
    if (!zone) return '';
    try {
      var p = iso.split('-'), at = new Date(Date.UTC(+p[0], +p[1] - 1, +p[2], 12));
      var part = new Intl.DateTimeFormat('en-US', { timeZone: zone, timeZoneName: 'short' }).formatToParts(at)
        .filter(function (x) { return x.type === 'timeZoneName'; })[0];
      return part ? part.value : '';
    } catch (e) { return ''; }
  }

  function Drawer(view) {
    this.view = view;
    var dock = view.dockEl;
    this.scrim = $('.dscrim', dock);
    if (!this.scrim) { this.scrim = document.createElement('div'); this.scrim.className = 'dscrim'; dock.appendChild(this.scrim); }
    this.el = document.createElement('aside');
    this.el.className = 'drawer ded gnd';
    this.el.setAttribute('role', 'dialog');
    this.el.setAttribute('aria-labelledby', 'cal5-gntitle');
    this.el.tabIndex = -1;
    dock.appendChild(this.el);
    this.state = null;
    this._offs = [];
    this._bind();
  }

  Drawer.prototype._on = function (target, type, fn) {
    target.addEventListener(type, fn);
    this._offs.push([target, type, fn]);
  };
  Drawer.prototype.destroy = function () {
    this._offs.forEach(function (o) { o[0].removeEventListener(o[1], o[2]); });
    this._offs = [];
    this.state = null;
    this.el.remove();
  };
  Drawer.prototype.isOpen = function () { return this.el.classList.contains('open'); };

  // night: the night to change (null for a new one). iso: the day a new one
  // starts on. preset: fields a new one starts with ({ name, time, repeat }).
  Drawer.prototype.open = function (night, iso, preset) {
    var view = this.view, back = document.activeElement;
    if (view.wingFor && (view.wingEl.contains(back) || view.evpEl.contains(back))) back = $('.day[data-key="' + view.wingFor + '"]', view.stageEl) || back;
    view.closeAllPanels({ instant: true });
    var s = stateFor(night, iso);
    if (!night && preset) {
      if (preset.name) s.name = preset.name;
      if (preset.time) s.time = preset.time;
      if (preset.repeat) s.repeat = preset.repeat;
    }
    // A night keeps the zone its start time was set in; a new one uses the
    // calendar's.
    var zone = (night && night.tz) || view.calZone || '';
    this.state = { night: night, form: s, before: stateFor(night, iso), zone: zone, back: back, confirmDel: false, busy: false };
    this.el.innerHTML = this._html();
    void this.el.offsetWidth;
    this.el.classList.add('open');
    view.dockEl.classList.add('on');
    var first = $('#cal5-gnN', this.el);
    setTimeout(function () { if (first) first.focus({ preventScroll: true }); }, reducedMotion() ? 20 : 260);
  };

  Drawer.prototype.close = function (quiet) {
    if (!this.isOpen()) return;
    var back = this.state && this.state.back;
    this.el.classList.remove('open');
    this.view.dockEl.classList.remove('on');
    this.state = null;
    if (!quiet && back && back.isConnected) back.focus({ preventScroll: true });
  };

  Drawer.prototype._html = function () {
    var S = this.state, f = S.form, view = this.view, isNew = !S.night;
    var z = zoneAbbr(S.zone, f.date || '2026-01-15');
    var h = '<div class="grab" aria-hidden="true"></div><div class="crease"><span id="cal5-gntitle">' + (isNew ? 'New game night' : 'Edit game night') + '</span>' +
      '<button type="button" class="x" data-close aria-label="Close the editor"><i class="fa-solid fa-xmark"></i></button></div>';
    h += '<div class="ebody"><label class="fld"><span class="sr">Name</span><input class="etitle" id="cal5-gnN" maxlength="' + NAME_MAX + '" placeholder="What is the game night called?" value="' + esc(f.name) + '"></label>' +
      '<div class="err" id="cal5-gnErr" role="alert" hidden></div>';
    h += '<section class="esec"><h4>When</h4>' +
      '<div class="gnwhen"><label class="fld">' + (isNew || !f.repeat ? 'Day' : 'First night') + '<input type="date" id="cal5-gnD" value="' + esc(f.date) + '"></label>' +
      '<label class="fld">Starts' + (z ? ' (' + esc(z) + ')' : '') + '<input type="time" id="cal5-gnT" value="' + esc(f.time) + '"></label></div>' +
      '<label class="fld">Repeats<select class="erep" id="cal5-gnR">' + REPEATS.map(function (o) {
        return '<option value="' + o[0] + '"' + (o[0] === f.repeat ? ' selected' : '') + '>' + o[1] + '</option>';
      }).join('') + '</select></label>' +
      '<div class="repend" id="cal5-gnRepEnd"' + (f.repeat ? '' : ' hidden') + '>' + this._repEndHTML() + '</div>' +
      '<p class="gnnext" id="cal5-gnNext">' + this._nextHTML() + '</p>' +
      (!isNew && S.before.repeat ? '<p class="dnote">Changes apply to every night in the series. Answers already given stay with their nights.</p>' : '') +
      '</section>';
    h += '<section class="esec"><h4>For players</h4><label class="fld"><span class="sr">Note for players</span><textarea id="cal5-gnS" maxlength="' + SUMMARY_MAX + '" placeholder="Optional. Shows on the night in the calendar.">' + esc(f.summary) + '</textarea></label>' +
      (isNew ? '<p class="dnote">Everyone in the campaign is asked if they can come.</p>' : '') + '</section></div>';
    // The server lets the owner and co-Directors cancel a game night.
    var canDel = !isNew && view.canAuthorDmOnly;
    return h + '<div class="efoot">' + (canDel ? '<button type="button" class="btn danger" data-gn-del><i class="fa-solid fa-trash"></i> ' + (S.confirmDel ? 'Press again to cancel it' : 'Cancel this game night') + '</button>' : '') +
      '<span class="sp"></span><button type="button" class="btn quiet" data-close>Close</button><button type="button" class="btn primary" data-gn-save>' + (isNew ? 'Plan it' : 'Save') + '</button></div>';
  };

  Drawer.prototype._repEndHTML = function () {
    var f = this.state.form;
    return (f.repeat === 'custom' ? '<label class="rn">Every <input id="cal5-gnI" type="number" min="2" max="52" value="' + f.every + '"> weeks</label>' : '') +
      '<div class="segs" role="group" aria-label="Repeat ends">' + [['never', 'Never ends'], ['until', 'Ends on a date']].map(function (o) {
        return '<button type="button" data-gn-end="' + o[0] + '" aria-pressed="' + (f.end === o[0]) + '">' + o[1] + '</button>';
      }).join('') + '</div>' +
      (f.end === 'until' ? '<label class="fld">Last night<input type="date" id="cal5-gnU" value="' + esc(f.until || f.date) + '"></label>' : '');
  };

  Drawer.prototype._nextHTML = function () {
    var f = this.state.form;
    if (!isIso(f.date)) return '';
    var list = nextNights(f.date, f.repeat, f.every, f.repeat && f.end === 'until' ? f.until : '', 5, '');
    if (!list.length) return '';
    return (f.repeat ? 'Next nights: ' : 'On ') + list.map(function (iso, i) {
      return i === 0 ? '<b>' + esc(shortDate(iso)) + '</b>' : esc(shortDate(iso));
    }).join(' · ');
  };

  Drawer.prototype._read = function () {
    var f = this.state.form, el = this.el;
    f.name = $('#cal5-gnN', el).value;
    f.date = $('#cal5-gnD', el).value;
    f.time = $('#cal5-gnT', el).value;
    f.repeat = $('#cal5-gnR', el).value;
    var i = $('#cal5-gnI', el); if (i) f.every = parseInt(i.value, 10) || 0;
    var u = $('#cal5-gnU', el); if (u) f.until = u.value;
    f.summary = $('#cal5-gnS', el).value;
  };

  Drawer.prototype._redrawRepeat = function () {
    var f = this.state.form, box = $('#cal5-gnRepEnd', this.el);
    box.hidden = !f.repeat;
    box.innerHTML = f.repeat ? this._repEndHTML() : '';
    $('#cal5-gnNext', this.el).innerHTML = this._nextHTML();
  };

  Drawer.prototype._dirty = function () {
    if (!this.state) return false;
    this._read();
    var a = this.state.form, b = this.state.before;
    return ['name', 'date', 'time', 'repeat', 'every', 'end', 'until', 'summary'].some(function (k) { return String(a[k]) !== String(b[k]); });
  };

  Drawer.prototype._warnUnsaved = function () {
    var e = $('#cal5-gnErr', this.el);
    this._error('Not saved yet. Save it, or press Close to throw the changes away.');
    e.classList.remove('shake');
    void e.offsetWidth;
    if (!reducedMotion()) e.classList.add('shake');
  };

  Drawer.prototype._error = function (msg) {
    var e = $('#cal5-gnErr', this.el);
    e.textContent = msg;
    e.hidden = !msg;
  };

  Drawer.prototype._busy = function (on) {
    this.state.busy = on;
    $$('button, input, select, textarea', this.el).forEach(function (c) { c.disabled = on; });
  };

  Drawer.prototype._save = function () {
    var self = this, S = this.state, view = this.view;
    if (!S || S.busy) return;
    this._read();
    var bad = problem(S.form);
    if (bad) { this._error(bad); return; }
    var base = '/campaigns/' + encodeURIComponent(view.campaignId) + '/sessions';
    var req;
    if (!S.night) {
      var fields = createFields(S.form, S.zone), body = new FormData();
      Object.keys(fields).forEach(function (k) { body.append(k, fields[k]); });
      // HX-Request asks for the server's no-redirect answer (204).
      req = Chronicle.apiFetch(base, { method: 'POST', body: body, headers: { 'HX-Request': 'true' } });
    } else {
      var change = updateBody(S.before, S.form, S.zone);
      if (!Object.keys(change).length) { this.close(); return; }
      req = Chronicle.apiFetch(base + '/' + encodeURIComponent(S.night.sessionId), { method: 'PUT', body: change });
    }
    this._busy(true);
    req.then(function (resp) {
      if (!resp.ok) return resp.json().catch(function () { return {}; }).then(function (j) { return Promise.reject(j && j.error); });
      var isNew = !S.night;
      self.close(true);
      view.say(isNew ? 'Game night planned. Everyone has been asked if they can come.' : 'Game night saved.');
      return view.reloadNights(S.form.date);
    }).catch(function (msg) {
      if (!self.state) return;
      self._busy(false);
      self._error((typeof msg === 'string' && msg) || 'That didn’t save. Try again.');
    });
  };

  // Cancelling takes two presses, so a stray click can't cancel a game
  // night everyone has answered.
  Drawer.prototype._delete = function () {
    var self = this, S = this.state, view = this.view;
    if (!S || !S.night || S.busy) return;
    if (!S.confirmDel) {
      S.confirmDel = true;
      var b = $('[data-gn-del]', this.el);
      if (b) b.innerHTML = '<i class="fa-solid fa-trash"></i> Press again to cancel it';
      return;
    }
    this._busy(true);
    Chronicle.apiFetch('/campaigns/' + encodeURIComponent(view.campaignId) + '/sessions/' + encodeURIComponent(S.night.sessionId), { method: 'DELETE' })
      .then(function (resp) {
        if (!resp.ok) return resp.json().catch(function () { return {}; }).then(function (j) { return Promise.reject(j && j.error); });
        self.close(true);
        view.say('Game night cancelled. Anyone who had answered has been told.');
        return view.reloadNights();
      })
      .catch(function (msg) {
        if (!self.state) return;
        self._busy(false);
        self._error((typeof msg === 'string' && msg) || 'That game night could not be cancelled. Try again.');
      });
  };

  Drawer.prototype._bind = function () {
    var self = this, el = this.el;
    // A press outside closes the drawer only when nothing is unsaved;
    // otherwise it says so and stays open.
    this._on(this.scrim, 'click', function () {
      if (!self.isOpen()) return;
      if (self._dirty()) { self._warnUnsaved(); return; }
      self.close();
    });
    this._on(el, 'keydown', function (e) {
      if (e.key === 'Escape' && self.isOpen()) { e.stopPropagation(); self.close(); }
    });
    this._on(el, 'change', function (e) {
      if (!self.state) return;
      self._read();
      if (e.target.id === 'cal5-gnR') self._redrawRepeat();
      else $('#cal5-gnNext', el).innerHTML = self._nextHTML();
    });
    this._on(el, 'input', function (e) {
      if (!self.state) return;
      self._error('');
      if (e.target.id === 'cal5-gnI') { self._read(); $('#cal5-gnNext', el).innerHTML = self._nextHTML(); }
    });
    this._on(el, 'click', function (e) {
      var t = e.target.closest('button');
      if (!t || !self.state) return;
      if (t.hasAttribute('data-close')) self.close();
      else if (t.hasAttribute('data-gn-save')) self._save();
      else if (t.hasAttribute('data-gn-del')) self._delete();
      else if (t.dataset.gnEnd) {
        self._read();
        self.state.form.end = t.dataset.gnEnd;
        if (t.dataset.gnEnd === 'until' && !self.state.form.until) self.state.form.until = self.state.form.date;
        self._redrawRepeat();
      }
    });
  };

  // One drawer per calendar view, made on first use.
  function drawerFor(view) {
    if (!view._gnDrawer || !view._gnDrawer.el.isConnected) view._gnDrawer = new Drawer(view);
    return view._gnDrawer;
  }

  window.Chronicle = window.Chronicle || {};
  Chronicle.calendarGameNight = {
    open: function (view, night, iso, preset) { drawerFor(view).open(night, iso, preset); },
    isOpen: function (view) { return !!(view._gnDrawer && view._gnDrawer.isOpen()); },
    close: function (view, quiet) { if (view._gnDrawer) view._gnDrawer.close(quiet); },
    destroy: function (view) { if (view._gnDrawer) { view._gnDrawer.destroy(); view._gnDrawer = null; } },
    // Pure helpers, for the tests.
    nextNights: nextNights,
    stateFor: stateFor,
    problem: problem,
    createFields: createFields,
    updateBody: updateBody
  };
})();
