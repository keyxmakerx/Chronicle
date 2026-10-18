/**
 * calendar_event_roll.js — the Roll section of the event editor: an event
 * that rolls dice or draws from a rolling table each night it lands on
 * (the Bloodnight on every full moon), with results that say what players
 * are told and what only the Director sees, and a stored result for each
 * night that can be rolled ahead, locked, rerolled or rolled on the night.
 *
 * calendar_event_drawer.js renders its Roll section as this widget's mount
 * for the owner or a granted co-Director, on an event that has been saved
 * (a roll belongs to an event id). The roll's settings save with the drawer's Save; the night
 * buttons act at once, saving changed settings first. The rolling itself is
 * the server's (crypto random, like Chronicle.dice), never this file's.
 *
 * Loaded on sight: the first mount fetches it (ADR-063).
 */
(function () {
  'use strict';

  function $(sel, root) { return (root || document).querySelector(sel); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  var DICE_RE = /^\s*(\d{0,3})\s*d\s*(\d{1,4})\s*(?:([+-])\s*(\d{1,4}))?\s*$/i;

  // parseDice mirrors the server's ParseDice limits so a typo is caught
  // before Save; the server still checks.
  function parseDice(s) {
    var m = DICE_RE.exec(String(s || ''));
    if (!m) return null;
    var n = m[1] ? +m[1] : 1, sides = +m[2], mod = m[4] ? (m[3] === '-' ? -m[4] : +m[4]) : 0;
    if (n < 1 || n > 100 || sides < 2 || sides > 1000) return null;
    return { n: n, sides: sides, mod: mod, lo: n + mod, hi: n * sides + mod };
  }

  function clone(o) { return JSON.parse(JSON.stringify(o)); }

  // A new roll starts as two results over 2d10, ready to rename.
  function starter() {
    return {
      source: 'dice', dice: '2d10', on_the_night: true, table_id: '', tell_players: false,
      results: [
        { max: 11, name: 'Quiet', players: '', director: '', tell_players: false },
        { name: 'Stirring', players: '', director: '', tell_players: true }
      ]
    };
  }

  function Roll(host, view, ev) {
    this.host = host;
    this.view = view;
    this.ev = ev;
    this.year = view.cal.current_year;
    this.cfg = null; // the stored settings, null for none
    this.draft = null; // what the form holds, null when "no roll"
    this.dirty = false;
    this.data = null; // the last read: nights and upcoming dates
    this.tables = null;
    this.showPast = false;
    this.busy = false;
    this.err = '';
    this._seq = 0;
    var self = this;
    this._onClick = function (e) { self._click(e); };
    this._onInput = function (e) { self._input(e); };
    this._onChange = function (e) { self._change(e); };
    host.addEventListener('click', this._onClick);
    host.addEventListener('input', this._onInput);
    host.addEventListener('change', this._onChange);
    host.innerHTML = '<h4>Roll</h4><p class="rlhint">Loading…</p>';
    this._load();
  }

  Roll.prototype.destroy = function () {
    this.host.removeEventListener('click', this._onClick);
    this.host.removeEventListener('input', this._onInput);
    this.host.removeEventListener('change', this._onChange);
    this._seq++;
  };

  Roll.prototype._url = function (suffix) {
    return this.view.apiBase + '/events/' + encodeURIComponent(this.ev.id) + '/roll' + (suffix || '');
  };

  Roll.prototype._load = function () {
    var self = this, seq = ++this._seq;
    Chronicle.apiFetch(this._url('?year=' + this.year)).then(function (resp) {
      return resp.ok ? resp.json() : null;
    }).then(function (data) {
      if (seq !== self._seq) return;
      if (!data) { self.host.innerHTML = '<h4>Roll</h4><p class="rlhint">The roll couldn’t be loaded. Close the editor and try again.</p>'; return; }
      self._took(data, true);
    }, function () {
      if (seq === self._seq) self.host.innerHTML = '<h4>Roll</h4><p class="rlhint">The roll couldn’t be loaded. Check your connection.</p>';
    });
  };

  // _took keeps a read of the roll. reset replaces the form with the
  // stored settings; otherwise unsaved edits stay.
  Roll.prototype._took = function (data, reset) {
    this.data = data;
    this.cfg = data.config || null;
    if (reset || !this.dirty) { this.draft = this.cfg ? clone(this.cfg) : null; this.dirty = false; }
    if (this.draft && this.draft.source === 'table') this._loadTables();
    this.render();
  };

  Roll.prototype._loadTables = function () {
    var self = this;
    if (this.tables) return;
    this.tables = [];
    Chronicle.apiFetch('/campaigns/' + encodeURIComponent(this.view.campaignId) + '/roll-tables').then(function (resp) {
      return resp.ok ? resp.json() : null;
    }).then(function (doc) {
      self.tables = doc && Array.isArray(doc.tables) ? doc.tables : [];
      self.render();
    }, function () {});
  };

  // ---- markup ----
  Roll.prototype.render = function () {
    var d = this.draft, h = '<h4>Roll <span class="rldm"><i class="fa-solid fa-eye-slash" aria-hidden="true"></i> Only you see results</span></h4>';
    if (!d) {
      h += '<p class="rlhint">Roll dice or draw from a rolling table each night this event lands on, and keep each night’s result.</p>' +
        '<button type="button" class="rlq" data-r="add">Add a roll</button>';
      this.host.innerHTML = h;
      return;
    }
    h += '<div class="rlrow"><div class="fld">Roll with<div class="segs" role="group" aria-label="Roll with">' +
      [['dice', 'Dice'], ['table', 'A rolling table']].map(function (o) {
        return '<button type="button" data-with="' + o[0] + '" aria-pressed="' + (d.source === o[0]) + '">' + o[1] + '</button>';
      }).join('') + '</div></div>';
    if (d.source === 'dice') {
      h += '<label class="fld rldice">Dice<input id="cal5-rlDice" value="' + esc(d.dice) + '" spellcheck="false" autocomplete="off" aria-describedby="cal5-rlRange"></label>';
    } else {
      h += '<label class="fld rltable">Table<select id="cal5-rlTable">' + this._tableOptions() + '</select></label>';
    }
    h += '</div>';
    if (d.source === 'dice') {
      var p = parseDice(d.dice);
      h += '<p class="rlhint" id="cal5-rlRange">' + (p ? 'Totals run from ' + p.lo + ' to ' + p.hi + '.' : 'Write dice like 2d10, 1d20+3 or 3d6-1.') + '</p>';
      h += '<div class="rlsub">Results <span>lowest first</span></div><div class="rlbands">' + this._bandsHTML() + '</div>' +
        (d.results.length < 12 ? '<button type="button" class="rlq" data-r="band+">Add a result</button>' : '');
    } else {
      h += '<p class="rlhint">Each night draws one entry from the table. The entry’s brief is what you see.</p>' +
        '<label class="rltell"><input type="checkbox" id="cal5-rlTableTell"' + (d.tell_players ? ' checked' : '') + '> Tell players on the night</label>';
    }
    h += '<label class="rltell"><input type="checkbox" id="cal5-rlNight"' + (d.on_the_night ? ' checked' : '') + '> Roll on the night if it wasn’t rolled ahead</label>';
    h += '<div class="rlsub">Each night <span class="rlyear"><button type="button" class="rlic" data-r="y-" aria-label="Previous year"><i class="fa-solid fa-chevron-left"></i></button>' +
      '<b>' + esc(String(this.year)) + '</b><button type="button" class="rlic" data-r="y+" aria-label="Next year"><i class="fa-solid fa-chevron-right"></i></button></span></div>' +
      '<div class="rlacts"><button type="button" class="btn primary" data-r="year"' + (this.busy ? ' disabled' : '') + '><i class="fa-solid fa-dice" aria-hidden="true"></i> Roll the year</button>' +
      '<button type="button" class="rlq" data-r="clear"' + (this.busy ? ' disabled' : '') + '>Clear unlocked nights</button></div>' +
      '<p class="rlerr" role="alert"' + (this.err ? '' : ' hidden') + '>' + esc(this.err) + '</p>' +
      '<div class="rlnights" aria-live="polite">' + this._nightsHTML() + '</div>' +
      '<p class="rlhint">A locked night keeps its result when you roll the year again. Each night keeps its result, the way each day keeps its weather.</p>' +
      '<button type="button" class="rlq" data-r="ai" aria-expanded="false">What the AI page writes for this</button>' +
      '<pre class="rlcode" hidden>' + esc(this._aiText()) + '</pre>' +
      '<button type="button" class="rlq rlrm" data-r="remove">Remove the roll</button>';
    this.host.innerHTML = h;
    var tas = this.host.querySelectorAll('textarea');
    for (var i = 0; i < tas.length; i++) autosize(tas[i]);
  };

  function autosize(t) { t.style.height = 'auto'; t.style.height = t.scrollHeight + 'px'; }

  Roll.prototype._tableOptions = function () {
    var cur = this.draft.table_id, list = this.tables || [], found = false;
    var h = list.map(function (t) {
      if (t.id === cur) found = true;
      return '<option value="' + esc(t.id) + '"' + (t.id === cur ? ' selected' : '') + '>' + esc(t.name) + '</option>';
    }).join('');
    if (!found) h = '<option value=""' + (cur ? '' : ' selected') + '>' + (list.length ? '(choose a table)' : this.tables ? 'No rolling tables yet' : 'Loading…') + '</option>' + h;
    return h;
  };

  Roll.prototype._bandsHTML = function () {
    var d = this.draft, p = parseDice(d.dice), lo = p ? p.lo : 1, n = d.results.length;
    return d.results.map(function (b, i) {
      var last = i === n - 1;
      var inp = '<input type="number" inputmode="numeric" data-max="' + i + '" value="' + esc(b.max == null ? '' : b.max) + '" aria-label="Highest total for ' + esc(b.name || 'this result') + '">';
      var rng = i === 0 && !last ? inp + '<span>or less</span>' : '<span class="lo">' + rangeLead(i, n, lo) + '</span>' + (last ? '' : inp);
      if (!last && b.max != null) lo = b.max + 1;
      return '<div class="rlband" data-i="' + i + '"><div class="rng">' + rng + '</div><div class="bt">' +
        '<input class="nm" data-k="name" value="' + esc(b.name) + '" maxlength="80" aria-label="Result name">' +
        '<span class="lbl">Players see</span><textarea class="pl" rows="1" data-k="players" maxlength="1000" aria-label="What players see">' + esc(b.players || '') + '</textarea>' +
        '<span class="lbl dm"><i class="fa-solid fa-eye-slash" aria-hidden="true"></i> Only you</span><textarea class="gm" rows="1" data-k="director" maxlength="1000" aria-label="What only you see">' + esc(b.director || '') + '</textarea>' +
        '<label class="rltell"><input type="checkbox" data-tell="' + i + '"' + (b.tell_players ? ' checked' : '') + '> Tell players on the night</label></div>' +
        (n > 1 ? '<button type="button" class="rlic" data-r="band-" aria-label="Remove ' + esc(b.name || 'this result') + '"><i class="fa-solid fa-xmark"></i></button>' : '<span></span>') +
        '</div>';
    }).join('');
  };

  // rangeLead is the fixed part of a result's range: where it starts.
  function rangeLead(i, n, lo) {
    if (i === n - 1) return n === 1 ? 'Any total' : lo + ' or more';
    return lo + '–';
  }

  // _ranges rewrites only the range starts after a total or the dice change,
  // so the field the Director tabbed into keeps its focus and its typing.
  Roll.prototype._ranges = function () {
    var d = this.draft, p = parseDice(d.dice), lo = p ? p.lo : 1, n = d.results.length;
    var leads = this.host.querySelectorAll('.rlband');
    for (var i = 0; i < leads.length && i < n; i++) {
      var el = leads[i].querySelector('.rng .lo');
      if (el) el.textContent = rangeLead(i, n, lo);
      if (i < n - 1 && d.results[i].max != null) lo = d.results[i].max + 1;
    }
  };

  Roll.prototype._dateText = function (d) {
    var cal = this.view.cal, mo = (cal.months || [])[d.month - 1];
    return d.day + ' ' + (mo ? mo.name : d.month);
  };

  Roll.prototype._nightsHTML = function () {
    var self = this, data = this.data || {}, cal = this.view.cal, CalDate = Chronicle.calendarDate;
    var today = CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
    var rows = (data.nights || []).map(function (n) { return { d: n, n: n }; })
      .concat((data.upcoming || []).map(function (d) { return { d: d, n: null }; }));
    rows.forEach(function (r) { r.ix = CalDate.dayIndex(cal, r.d.year, r.d.month, r.d.day); });
    rows.sort(function (a, b) { return a.ix - b.ix; });
    if (!rows.length) return '<p class="rlhint">' + (data.truncated ? 'This event’s nights in ' + this.year + ' couldn’t all be worked out.' : 'This event has no nights in ' + this.year + '.') + '</p>';
    var past = rows.filter(function (r) { return r.ix < today; }).length, h = '';
    if (past) h += '<button type="button" class="rlq" data-r="past" aria-expanded="' + this.showPast + '">' + (this.showPast ? 'Hide' : 'Show') + ' ' + past + ' past night' + (past > 1 ? 's' : '') + '</button>';
    rows.forEach(function (r) {
      var isPast = r.ix < today;
      if (isPast && !self.showPast) return;
      var n = r.n, key = r.d.year + '_' + r.d.month + '_' + r.d.day, tick = n && n.band_index != null && n.band_index >= 0 ? ' t' + Math.min(n.band_index, 2) : '';
      var told = n && n.told && r.ix <= today;
      h += '<div class="rlnight' + (isPast ? ' past' : '') + '" data-d="' + key + '"><span class="d">' + esc(self._dateText(r.d)) + '</span>' +
        (n ? '<span class="tot">' + (n.total != null ? esc(String(n.total)) : '') + '</span><span class="bn"><i class="tick' + tick + '"></i><b>' + esc(n.result || '—') + '</b>' + (told ? '<span class="told">Players told</span>' : '') + '</span>'
           : '<span class="tot"></span><span class="bn wait">Not rolled yet</span>') +
        '<span class="acts">' +
        '<button type="button" class="rlic" data-r="lock" aria-pressed="' + !!(n && n.locked) + '" aria-label="' + (n && n.locked ? 'Unlock' : 'Lock') + ' ' + esc(self._dateText(r.d)) + '"' + (n ? '' : ' disabled') + '><i class="fa-solid ' + (n && n.locked ? 'fa-lock' : 'fa-lock-open') + '"></i></button>' +
        '<button type="button" class="rlic" data-r="reroll" aria-label="Roll ' + esc(self._dateText(r.d)) + '"' + (n && n.locked ? ' disabled' : '') + '><i class="fa-solid fa-rotate"></i></button></span></div>';
    });
    return h;
  };

  // _aiText is the AI page's record for this roll, so a Director can see
  // what to ask for there.
  Roll.prototype._aiText = function () {
    var d = this.draft, q = function (s) { return /^[\w .,'!?()-]*$/.test(s) && s.trim() === s && s !== '' ? s : JSON.stringify(s); };
    var t = '---\nkind: event\nname: ' + q(this.ev.name) + '\nroll:\n';
    if (d.source === 'table') {
      t += '  table: ' + q(d.table_id || '') + '\n  on_the_night: ' + !!d.on_the_night + '\n  tell_players: ' + !!d.tell_players + '\n';
    } else {
      t += '  dice: ' + q(d.dice) + '\n  on_the_night: ' + !!d.on_the_night + '\n  results:\n';
      d.results.forEach(function (b, i) {
        t += '    - ' + (i < d.results.length - 1 && b.max != null ? 'max: ' + b.max + '\n      ' : '') + 'name: ' + q(b.name) + '\n';
        if (b.players) t += '      players: ' + q(b.players) + '\n';
        if (b.director) t += '      director: ' + q(b.director) + '\n';
        if (b.tell_players) t += '      tell_players: true\n';
      });
    }
    return t + '---\n\n---\nkind: generator\ngenerator: rolls\nevent: ' + q(this.ev.name) + '\nyear: ' + this.year + '\nkeep_locked: true\n---';
  };

  // ---- editing ----
  Roll.prototype._edited = function () { this.dirty = true; this.err = ''; };

  Roll.prototype._click = function (e) {
    var t = e.target.closest('button');
    if (!t || !this.host.contains(t)) return;
    var d = this.draft, r = t.dataset.r;
    if (t.dataset.with && d) {
      d.source = t.dataset.with;
      if (d.source === 'dice' && !d.results.length) d.results = starter().results;
      if (d.source === 'table') this._loadTables();
      this._edited(); this.render(); return;
    }
    if (r === 'add') { this.draft = starter(); this._edited(); this.render(); var f = $('#cal5-rlDice', this.host); if (f) f.focus(); return; }
    if (r === 'remove') { this.draft = null; this._edited(); this.render(); return; }
    if (r === 'band+') {
      var n = d.results.length, last = d.results[n - 1], prev = n > 1 ? d.results[n - 2].max : null, p = parseDice(d.dice);
      last.max = prev != null ? prev + 2 : (p ? Math.floor((p.lo + p.hi) / 2) : 10);
      d.results.push({ name: 'New result', players: '', director: '', tell_players: false });
      this._edited(); this.render();
      var names = this.host.querySelectorAll('.rlband .nm'), nm = names[names.length - 1];
      if (nm) { nm.focus(); nm.select(); }
      return;
    }
    if (r === 'band-') {
      var i = +t.closest('.rlband').dataset.i;
      d.results.splice(i, 1);
      if (d.results.length) delete d.results[d.results.length - 1].max;
      this._edited(); this.render(); return;
    }
    if (r === 'y-' || r === 'y+') { this.year += r === 'y-' ? -1 : 1; this._load(); return; }
    if (r === 'past') { this.showPast = !this.showPast; $('.rlnights', this.host).innerHTML = this._nightsHTML(); return; }
    if (r === 'ai') {
      var pre = $('.rlcode', this.host), open = pre.hidden;
      pre.textContent = this._aiText();
      pre.hidden = !open;
      t.setAttribute('aria-expanded', String(open));
      return;
    }
    var row = t.closest('.rlnight'), date = null;
    if (row) { var k = row.dataset.d.split('_'); date = { year: +k[0], month: +k[1], day: +k[2] }; }
    if (r === 'year') this._act(this._url('/nights'), { year: this.year }, 'Rolled every night of ' + this.year + ' that isn’t locked.');
    else if (r === 'clear') {
      if (!window.confirm('Clear the results of every unlocked night from today on in ' + this.year + '?')) return;
      this._act(this._url('/nights/clear'), { year: this.year, keep_locked: true }, 'Cleared the unlocked nights.', true);
    } else if (r === 'reroll' && date) this._act(this._url('/nights'), { dates: [date] }, 'Rolled ' + this._dateText(date) + '.');
    else if (r === 'lock' && date) this._act(this._url('/nights/lock'), { days: [date], locked: t.getAttribute('aria-pressed') !== 'true' }, '', true);
  };

  Roll.prototype._input = function (e) {
    var t = e.target, d = this.draft;
    if (!d) return;
    if (t.id === 'cal5-rlDice') {
      d.dice = t.value; this._edited();
      var p = parseDice(d.dice), hint = $('#cal5-rlRange', this.host);
      if (hint) hint.textContent = p ? 'Totals run from ' + p.lo + ' to ' + p.hi + '.' : 'Write dice like 2d10, 1d20+3 or 3d6-1.';
      return;
    }
    var band = t.closest('.rlband');
    if (!band) return;
    var b = d.results[+band.dataset.i];
    if (t.dataset.k) { b[t.dataset.k] = t.value; this._edited(); }
    if (t.dataset.max != null) { var v = parseInt(t.value, 10); b.max = isNaN(v) ? null : v; this._edited(); }
    if (t.tagName === 'TEXTAREA') autosize(t);
  };

  Roll.prototype._change = function (e) {
    var t = e.target, d = this.draft;
    if (!d) return;
    if (t.id === 'cal5-rlNight') { d.on_the_night = t.checked; this._edited(); }
    else if (t.id === 'cal5-rlTableTell') { d.tell_players = t.checked; this._edited(); }
    else if (t.id === 'cal5-rlTable') { d.table_id = t.value; this._edited(); }
    else if (t.dataset.tell != null) { d.results[+t.dataset.tell].tell_players = t.checked; this._edited(); }
    else if (t.dataset.max != null || t.id === 'cal5-rlDice') this._ranges();
  };

  // check returns what is wrong with the draft, or ''.
  Roll.prototype.check = function () {
    var d = this.draft;
    if (!d) return '';
    if (d.source === 'table') return d.table_id ? '' : 'Choose the rolling table this roll draws from.';
    if (!parseDice(d.dice)) return 'Write the roll’s dice like 2d10, 1d20+3 or 3d6-1.';
    var prev = null;
    for (var i = 0; i < d.results.length; i++) {
      var b = d.results[i];
      if (!String(b.name || '').trim()) return 'Give every roll result a name.';
      if (i === d.results.length - 1) break;
      if (b.max == null) return 'Give “' + b.name + '” the highest total it covers.';
      if (prev != null && b.max <= prev) return 'List the roll’s results lowest first, each covering higher totals than the one before.';
      prev = b.max;
    }
    return '';
  };

  Roll.prototype.isDirty = function () { return this.dirty; };

  // save stores the settings if they changed. Resolves to '' or an error.
  Roll.prototype.save = function () {
    var self = this;
    if (!this.dirty) return Promise.resolve('');
    var bad = this.check();
    if (bad) return Promise.resolve(bad);
    var body = { config: this.draft ? this._body() : null, year: this.year };
    return Chronicle.apiFetch(this._url(), { method: 'PUT', body: body }).then(function (resp) {
      if (!resp.ok) return resp.json().then(function (j) { return (j && (j.message || j.error)) || 'The roll didn’t save.'; }, function () { return 'The roll didn’t save.'; });
      return resp.json().then(function (data) { self.dirty = false; self._took(data, true); return ''; });
    }, function () { return 'The roll didn’t save. Check your connection.'; });
  };

  Roll.prototype._body = function () {
    var d = this.draft;
    if (d.source === 'table') return { source: 'table', table_id: d.table_id, on_the_night: !!d.on_the_night, tell_players: !!d.tell_players };
    return {
      source: 'dice', dice: String(d.dice).trim(), on_the_night: !!d.on_the_night,
      results: d.results.map(function (b, i) {
        var o = { name: String(b.name).trim(), players: b.players || '', director: b.director || '', tell_players: !!b.tell_players };
        if (i < d.results.length - 1) o.max = b.max;
        return o;
      })
    };
  };

  // _act runs a night button: changed settings are saved first, since the
  // server rolls with what it has stored.
  Roll.prototype._act = function (url, body, said, reload) {
    var self = this;
    if (this.busy) return;
    if (!this.cfg && !this.draft) return;
    this.busy = true; this.err = ''; this.render();
    this.save().then(function (bad) {
      if (bad) throw new Error(bad);
      return Chronicle.apiFetch(url, { method: 'POST', body: body });
    }).then(function (resp) {
      if (!resp.ok) return resp.json().then(function (j) { throw new Error((j && (j.message || j.error)) || 'That didn’t work. Try again.'); }, function () { throw new Error('That didn’t work. Try again.'); });
      self.busy = false;
      self.view.rollsByMonth = {};
      if (said && self.view.say) self.view.say(said);
      if (reload) { self._load(); return null; }
      return resp.json().then(function (data) { self._took(data, false); });
    }).catch(function (err) {
      self.busy = false;
      self.err = err && err.message ? err.message : 'That didn’t work. Try again.';
      self.render();
    });
  };

  window.Chronicle = window.Chronicle || {};
  Chronicle.calendarEventRoll = { parseDice: parseDice, Roll: Roll };

  // The drawer renders the section with data-widget="calendar_event_roll"
  // and hands over its calendar and event on the element, so this file
  // loads only when a Director first opens a saved event (ADR-063).
  if (Chronicle.register) {
    Chronicle.register('calendar_event_roll', {
      init: function (el) {
        var ctx = el._calRoll;
        if (!ctx) return;
        el._calRollCtl = new Roll(el, ctx.view, ctx.ev);
        if (ctx.ready) ctx.ready(el._calRollCtl);
      },
      destroy: function (el) {
        if (el._calRollCtl) { el._calRollCtl.destroy(); el._calRollCtl = null; }
      }
    });
  }
})();
