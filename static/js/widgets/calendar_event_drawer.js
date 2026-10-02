/**
 * calendar_event_drawer.js — the full event editor: a drawer that slides out
 * of the calendar's right edge (a sheet from the bottom on phones), as the
 * signed calendar and owner-editing mockups draw it. It sets everything the
 * events API stores about an event's when, what and who: title, all-day or
 * start and end times, start and end dates, how it repeats and when that
 * stops, its kind, notes, who sees it and whether players learn of it ahead
 * of time.
 *
 * Repeating by a rule ("every full moon of Luna", "the 3rd Kingsday of each
 * month", "2 days after the Festival of Masks") is built here from rows of
 * conditions; calendar_rule.js holds the pure part (ready-made rules, the
 * form <-> recurrence_rule mapping, the plain sentence) and the "Next five"
 * dates come from the preview endpoint, never from a second implementation
 * of the rule in the browser.
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
    this._offs = [];
    this._previewTimer = null;
    this._previewAbort = null;
    this._previewSeq = 0;
    this._bind();
  }

  // Removes every listener this drawer added and stops its pending work, so
  // a calendar torn down (boosted navigation) leaves nothing behind.
  Drawer.prototype.destroy = function () {
    this._stopPreview();
    this._offs.forEach(function (o) { o[0].removeEventListener(o[1], o[2], o[3]); });
    this._offs = [];
    this.state = null;
    this.el.remove();
  };
  Drawer.prototype._on = function (target, type, fn, capture) {
    target.addEventListener(type, fn, !!capture);
    this._offs.push([target, type, fn, !!capture]);
  };

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
    var Rl = Chronicle.calendarRule, env = view.ruleEnv(!isNew ? ev.id : null), rule = null;
    // A rule event whose rule the server left out names something this
    // viewer may not see: it is kept as it is, never shown or rewritten.
    var locked = rtype === 'rule' && (!ev.recurrence_rule || !Rl);
    if (rtype === 'rule' && !locked) {
      var rf = Rl.fromRule(ev.recurrence_rule);
      rule = { form: rf, original: Rl.clone(rf), preset: Rl.presetFor(rf, env) || 'custom', more: rf.every > 1 || rf.shift > 0 };
    }
    this.state = {
      ev: ev, isNew: isNew, back: back,
      allDay: isNew ? true : (ev.all_day || ev.start_hour == null),
      kindId: !isNew && ev.kind_id != null ? ev.kind_id : null,
      vis: isNew ? 'everyone' : ev.visibility,
      announced: !isNew && ev.announced ? ev.announced : '',
      rule: rule,
      rep: {
        type: rtype,
        locked: locked,
        // A stored rule has to be cleared (null) if the event stops using it.
        hadRule: !isNew && rtype === 'rule' && !!ev.recurrence_rule,
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
    if (this.state.rep.type === 'rule' && !this.state.rep.locked) { this._renderSummary(); this._lookupEventNames(); this._schedulePreview(); }
  };

  Drawer.prototype.close = function (quiet) {
    if (!this.isOpen()) return;
    var back = this.state && this.state.back;
    this._stopPreview();
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
      '<label class="fld">Repeats<select class="erep" id="cal5-edRep">' + this._repeatOptions() + '</select></label>' +
      '<div class="rbox" id="cal5-edRule"' + (S.rep.type === 'rule' ? '' : ' hidden') + '>' + (S.rep.type === 'rule' ? this._ruleBoxHTML() : '') + '</div>' +
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

  // ---- repeating by a rule ----
  // gone: what to call a stored value no option matches (a deleted moon or
  // season). It is shown first and selected, so picking any real option is a
  // change the browser reports.
  var GONE = { ph: '(choose)', moon: 'a removed moon', season: 'a removed season', event: 'an unavailable event', wd: '(choose)', n: '(choose)', d: '(choose)', m: '(choose)', k: '(choose)' };
  function sel(f, opts, cur, label) {
    var known = opts.some(function (o) { return String(o[0]) === String(cur); });
    if (!known) opts = [['', GONE[f] || '(choose)']].concat(opts);
    return '<select data-f="' + f + '" aria-label="' + esc(label) + '">' + opts.map(function (o) {
      return '<option value="' + esc(o[0]) + '"' + ((known ? String(o[0]) === String(cur) : o[0] === '') ? ' selected' : '') + '>' + esc(o[1]) + '</option>';
    }).join('') + '</select>';
  }

  Drawer.prototype._env = function () {
    return this.view.ruleEnv(this.state && this.state.ev ? this.state.ev.id : null);
  };

  // The Repeats list: the plain repeats first, then the rules.
  Drawer.prototype._repeatOptions = function () {
    var S = this.state, Rl = Chronicle.calendarRule;
    var cur = S.rep.type === 'rule' ? (S.rep.locked ? 'rule:locked' : 'rule:' + S.rule.preset) : S.rep.type;
    var h = REPEATS.map(function (o) {
      return '<option value="' + o[0] + '"' + (o[0] === cur ? ' selected' : '') + '>' + o[1] + '</option>';
    }).join('');
    if (!Rl) return S.rep.type === 'rule' ? h + '<option value="rule:locked" selected disabled>Repeats by a rule</option>' : h;
    h += '<optgroup label="By a rule">' + Rl.presets(this._env()).map(function (p) {
      return '<option value="rule:' + esc(p.id) + '"' + ('rule:' + p.id === cur ? ' selected' : '') + '>' + esc(p.label) + '</option>';
    }).join('') + '<option value="rule:custom"' + (cur === 'rule:custom' ? ' selected' : '') + '>Make your own…</option>' +
      (S.rep.locked ? '<option value="rule:locked" selected disabled>A rule you can’t see</option>' : '') + '</optgroup>';
    return h;
  };

  Drawer.prototype._ruleBoxHTML = function () {
    var S = this.state;
    if (S.rep.locked) return '<p class="rlock">This event repeats by a rule that uses something you can’t see, so it can’t be shown or changed here. Pick another repeat to replace it.</p>';
    var Rl = Chronicle.calendarRule, f = S.rule.form;
    var everyOpts = [];
    for (var n = 1; n <= Math.max(12, f.every); n++) everyOpts.push([n, n === 1 ? 'every' : 'every ' + Rl.ordinal(n)]);
    return '<div class="rsum" id="cal5-rsum" aria-live="polite"></div>' +
      '<div class="rrows" id="cal5-rrows">' + this._rowsHTML() + '</div>' +
      '<button type="button" class="radd" id="cal5-radd" data-r="add"' + (f.conds.length >= Rl.MAX_CONDS ? ' hidden' : '') + '>+ Add a condition</button>' +
      '<details class="rmore" id="cal5-rmore"' + (S.rule.more ? ' open' : '') + '><summary>Every Nth time, or shift the date</summary>' +
        '<div class="rthen">Keep <select data-rf="every" aria-label="Which matches to keep">' + everyOpts.map(function (o) {
          return '<option value="' + o[0] + '"' + (o[0] === f.every ? ' selected' : '') + '>' + o[1] + '</option>';
        }).join('') + '</select> match</div>' +
        '<div class="rthen">then move it <input type="number" data-rf="shift" min="0" max="' + Rl.MAX_SHIFT + '" value="' + f.shift + '" aria-label="Days to shift"> days ' +
          '<select data-rf="dir" aria-label="Later or earlier">' + [['later', 'later'], ['earlier', 'earlier']].map(function (o) {
            return '<option value="' + o[0] + '"' + (o[0] === f.dir ? ' selected' : '') + '>' + o[1] + '</option>';
          }).join('') + '</select></div></details>' +
      '<div class="rnext" id="cal5-rnext" aria-busy="false"><b>Next five</b><ol></ol><p class="rhint" role="status"></p></div>';
  };

  Drawer.prototype._rowsHTML = function () {
    var self = this, S = this.state, Rl = Chronicle.calendarRule, env = this._env(), f = S.rule.form;
    var kinds = Rl.availableKinds(env, f).map(function (k) { return [k, Rl.KIND_LABELS[k]]; });
    var removable = f.conds.length > 1 || S.rule.preset === 'custom';
    return f.conds.map(function (c, i) {
      var head = '<span class="rand">' + (i ? 'and' : 'when') + '</span>';
      var body = c.k === 'raw'
        ? '<span class="rraw">' + esc(Rl.noun(c, env)) + ' <small>(set elsewhere)</small></span>'
        : sel('k', kinds, c.k, 'Kind of condition') + self._condFields(c, env);
      return '<div class="rrow" data-i="' + i + '">' + head + body +
        (removable ? '<button type="button" class="rrm" data-r="del" aria-label="Remove this condition"><span aria-hidden="true">×</span></button>' : '') + '</div>';
    }).join('');
  };

  Drawer.prototype._condFields = function (c, env) {
    var Rl = Chronicle.calendarRule, wds = (env.cal.weekdays || []).map(function (w, i) { return [i, w.name]; });
    if (!wds.length) wds = [[0, Rl.wdName(env, 0)]];
    switch (c.k) {
      case 'moon':
        return sel('ph', Rl.PHASES.map(function (p) { return [p[0], p[2]]; }), c.ph, 'Phase') + '<span>of</span>' +
          sel('moon', Rl.moonsOf(env).map(function (m) { return [m.id, m.name]; }), c.moon, 'Moon');
      case 'wd':
        if (c.wds.length > 1) {
          return '<span class="rchips" role="group" aria-label="Weekdays">' + wds.map(function (w) {
            return '<button type="button" data-r="wd" data-wd="' + w[0] + '" aria-pressed="' + (c.wds.indexOf(w[0]) >= 0) + '">' + esc(w[1]) + '</button>';
          }).join('') + '</span>';
        }
        return sel('wd', wds.concat([['many', 'Several days…']]), c.wds[0], 'Weekday');
      case 'nth':
        return sel('n', Rl.ORDINALS, c.n, 'Which') + sel('wd', wds, c.wd, 'Weekday') + '<span>of the month</span>';
      case 'dom':
        var days = [[-1, 'the last day']];
        for (var d = 1; d <= Rl.maxMonthDays(env); d++) days.push([d, 'the ' + Rl.ordinal(d)]);
        return sel('d', days, c.d, 'Day');
      case 'month':
        return sel('m', (env.cal.months || []).map(function (m, i) { return [i + 1, m.name]; }), c.m, 'Month');
      case 'season':
        return sel('season', [[0, 'any season']].concat(Rl.seasonsOf(env).map(function (s) { return [s.id, s.name]; })), c.season || 0, 'Season');
      case 'event':
        var evs = env.events.map(function (e) { return [e.id, e.name]; });
        if (c.event && !evs.some(function (e) { return e[0] === c.event; })) evs.unshift([c.event, Rl.eventName(env, c.event)]);
        return sel('event', evs, c.event, 'Event');
    }
    return '';
  };

  // The Repeats list was changed: a plain repeat, a ready-made rule or
  // "Make your own".
  Drawer.prototype._repeatChosen = function (v) {
    var S = this.state, el = this.el, Rl = Chronicle.calendarRule, rbox = $('#cal5-edRule', el);
    if (v.indexOf('rule:') !== 0) {
      S.rep.type = v;
      S.rep.locked = false;
      S.rule = null;
      this._stopPreview();
    } else {
      var id = v.slice(5), env = this._env(), form;
      if (id === 'custom') {
        // Starting from the rule already chosen, if any, so a ready-made
        // rule can be changed rather than rebuilt.
        form = S.rule ? S.rule.form : { conds: [Rl.defaultCond('wd', env, this._startInfo())], every: 1, shift: 0, dir: 'later' };
      } else {
        form = Rl.presetForm(id, env);
        if (!form) return;
      }
      S.rep.type = 'rule';
      S.rep.locked = false;
      S.rule = { form: form, preset: id, more: form.every > 1 || form.shift > 0 };
    }
    var re = $('#cal5-edRepEnd', el);
    re.hidden = !S.rep.type;
    re.innerHTML = S.rep.type ? this._repEndHTML() : '';
    rbox.hidden = S.rep.type !== 'rule';
    rbox.innerHTML = S.rep.type === 'rule' ? this._ruleBoxHTML() : '';
    if (S.rep.type === 'rule') { this._renderSummary(); this._lookupEventNames(); this._schedulePreview(); }
  };

  // {wd, m, d} of the start date, so a new row opens on something real.
  Drawer.prototype._startInfo = function () {
    var s = this._readDate('es'), wd = this.CalDate.weekdayCol(this.cal, s.y, s.m, s.d);
    return { wd: wd < 0 ? 0 : wd, m: s.m, d: s.d };
  };

  Drawer.prototype._renderSummary = function () {
    var S = this.state, Rl = Chronicle.calendarRule, box = $('#cal5-rsum', this.el);
    if (!box || !S.rule) return;
    var text = Rl.summary(S.rule.form, this._env());
    box.textContent = text;
    if (S.rule.preset === 'custom') {
      var b = document.createElement('span');
      b.className = 'rbadge';
      b.textContent = 'your own rule';
      box.appendChild(document.createTextNode(' '));
      box.appendChild(b);
    }
  };

  // Any edit makes the rule the author's own and refreshes what depends on it.
  Drawer.prototype._ruleEdited = function (redrawRows, focusSel) {
    var S = this.state, Rl = Chronicle.calendarRule;
    // Leaving a ready-made rule also gives its last row a remove button, so
    // the rows are drawn again, keeping focus on the control being used.
    if (S.rule.preset !== 'custom') {
      redrawRows = true;
      var a = document.activeElement, ar = a && a.closest && a.closest('.rrow');
      if (!focusSel && ar && a.dataset.f) focusSel = '.rrow[data-i="' + ar.dataset.i + '"] [data-f="' + a.dataset.f + '"]';
    }
    S.rule.preset = 'custom';
    var rep = $('#cal5-edRep', this.el);
    if (rep.value !== 'rule:custom') rep.value = 'rule:custom';
    if (redrawRows) {
      $('#cal5-rrows', this.el).innerHTML = this._rowsHTML();
      $('#cal5-radd', this.el).hidden = S.rule.form.conds.length >= Rl.MAX_CONDS;
      if (focusSel) { var f = $(focusSel, this.el); if (f) f.focus(); }
    }
    this._renderSummary();
    this._schedulePreview();
  };

  Drawer.prototype._ruleFieldChanged = function (t) {
    var S = this.state;
    if (!S || !S.rule) return;
    var Rl = Chronicle.calendarRule, f = S.rule.form, env = this._env();
    if (t.dataset.rf) {
      if (t.dataset.rf === 'every') f.every = Rl.intIn(t.value, 1, Rl.MAX_EVERY, 1);
      else if (t.dataset.rf === 'dir') f.dir = t.value === 'earlier' ? 'earlier' : 'later';
      else if (t.dataset.rf === 'shift') {
        f.shift = Rl.intIn(t.value, 0, Rl.MAX_SHIFT, 0);
        // Show what is stored: a typed 900 reads 365 at once.
        if (t.value !== '' && String(f.shift) !== t.value) t.value = f.shift;
      }
      this._ruleEdited(false);
      return;
    }
    var row = t.closest('.rrow');
    if (!row) return;
    var i = +row.dataset.i, c = f.conds[i], key = t.dataset.f, v = t.value;
    if (!c) return;
    var focus = '.rrow[data-i="' + i + '"] [data-f="' + key + '"]';
    if (key === 'k') { f.conds[i] = Rl.defaultCond(v, env, this._startInfo()); this._ruleEdited(true, focus); return; }
    if (key === 'ph') c.ph = v;
    else if (key === 'moon') c.moon = parseInt(v, 10);
    else if (key === 'season') c.season = parseInt(v, 10);
    else if (key === 'event') c.event = v;
    else if (key === 'n') c.n = parseInt(v, 10);
    else if (key === 'd') c.d = parseInt(v, 10);
    else if (key === 'm') c.m = parseInt(v, 10);
    else if (key === 'wd' && c.k === 'nth') c.wd = parseInt(v, 10);
    else if (key === 'wd' && c.k === 'wd') {
      if (v === 'many') {
        // Several weekdays: the chosen one plus the next, to start from.
        var wl = (env.cal.weekdays || []).length || 7;
        c.wds = [c.wds[0], (c.wds[0] + 1) % wl].sort(function (a, b) { return a - b; });
        this._ruleEdited(true, '.rrow[data-i="' + i + '"] [data-wd]');
        return;
      }
      c.wds = [parseInt(v, 10)];
    }
    this._ruleEdited(false);
  };

  Drawer.prototype._ruleClick = function (t) {
    var S = this.state, Rl = Chronicle.calendarRule;
    var act = t.dataset.r;
    if (act === 'retry') { this._schedulePreview(0); return; }
    if (!S.rule) return;
    var f = S.rule.form, env = this._env();
    if (act === 'add') {
      if (f.conds.length >= Rl.MAX_CONDS) return;
      var used = f.conds.map(function (c) { return c.k; });
      var order = ['wd', 'month', 'moon', 'dom', 'nth', 'season', 'event'];
      var avail = Rl.availableKinds(env, f);
      var kind = order.filter(function (k) { return avail.indexOf(k) >= 0 && used.indexOf(k) < 0; })[0] || 'wd';
      f.conds.push(Rl.defaultCond(kind, env, this._startInfo()));
      this._ruleEdited(true, '.rrow[data-i="' + (f.conds.length - 1) + '"] select');
    } else if (act === 'del') {
      var row = t.closest('.rrow');
      f.conds.splice(+row.dataset.i, 1);
      // Focus lands on the add button, which is always there to keep going.
      this._ruleEdited(true, '#cal5-radd');
    } else if (act === 'wd') {
      var c = f.conds[+t.closest('.rrow').dataset.i], w = +t.dataset.wd, at = c.wds.indexOf(w);
      if (at >= 0 && c.wds.length > 1) c.wds.splice(at, 1);
      else if (at < 0) c.wds.push(w);
      this._ruleEdited(true, '.rrow[data-i="' + t.closest('.rrow').dataset.i + '"] ' + (c.wds.length > 1 ? '[data-wd="' + w + '"]' : 'select'));
    }
  };

  // A rule may name an event not in the months seen so far; its name is
  // fetched once so the sentence and the picker can say it.
  Drawer.prototype._lookupEventNames = function () {
    var self = this, S = this.state, view = this.view;
    if (!S || !S.rule) return;
    var names = view._eventNames || (view._eventNames = {});
    var unknown = S.rule.form.conds.map(function (c) { return c.k === 'event' ? c.event : (c.k === 'raw' ? c.raw.event_id : null); })
      .filter(function (id, i, a) { return id && !names[id] && a.indexOf(id) === i; });
    unknown.forEach(function (id) {
      Chronicle.apiFetch(view.apiBase + '/events/' + encodeURIComponent(id)).then(function (resp) {
        return resp.ok ? resp.json() : null;
      }).then(function (e) {
        if (!e || !e.name || self.state !== S) return;
        names[id] = e.name;
        // Redrawing the rows must not drop the keyboard: put focus back on
        // the same control of the same row.
        var a = document.activeElement, row = a && a.closest && a.closest('.rrow'), at = null;
        if (row && self.el.contains(row)) at = { i: row.dataset.i, f: a.dataset.f, r: a.dataset.r, wd: a.dataset.wd };
        $('#cal5-rrows', self.el).innerHTML = self._rowsHTML();
        if (at) {
          var base = '.rrow[data-i="' + at.i + '"] ';
          var back = $(at.f ? base + '[data-f="' + at.f + '"]' : at.wd != null ? base + '[data-wd="' + at.wd + '"]' : at.r ? base + '[data-r="' + at.r + '"]' : base + 'select', self.el);
          if (back) back.focus();
        }
        self._renderSummary();
      }, function () { /* the sentence keeps saying "another event" */ });
    });
  };

  // ---- the "Next five" dates ----
  Drawer.prototype._stopPreview = function () {
    clearTimeout(this._previewTimer);
    this._previewTimer = null;
    if (this._previewAbort) { this._previewAbort.abort(); this._previewAbort = null; }
    this._previewSeq++;
  };

  Drawer.prototype._previewBox = function () { return $('#cal5-rnext', this.el); };

  Drawer.prototype._setNext = function (kind, dates, msg) {
    var box = this._previewBox();
    if (!box) return;
    var ol = $('ol', box), hint = $('.rhint', box), self = this;
    box.setAttribute('aria-busy', String(kind === 'loading'));
    box.classList.toggle('busy', kind === 'loading');
    if (kind === 'loading') {
      if (!ol.children.length) hint.textContent = 'Working out the dates…';
      return;
    }
    hint.textContent = '';
    var moon = this.state && this.state.rule && this.state.rule.form.conds.some(function (c) { return c.k === 'moon'; });
    ol.innerHTML = (dates || []).map(function (d) {
      return '<li' + (moon ? ' class="rphase"' : '') + '>' + esc(self._dateText(d)) + '</li>';
    }).join('');
    if (kind === 'error') {
      hint.textContent = msg;
      var retry = document.createElement('button');
      retry.type = 'button';
      retry.className = 'rretry';
      retry.dataset.r = 'retry';
      retry.textContent = 'Try again';
      hint.appendChild(document.createTextNode(' '));
      hint.appendChild(retry);
    } else if (kind === 'empty') hint.textContent = msg;
    else if (kind === 'idle') hint.textContent = msg;
    else if (kind === 'partial') hint.textContent = msg;
  };

  Drawer.prototype._dateText = function (d) {
    var cal = this.cal, col = this.CalDate.weekdayCol(cal, d.year, d.month, d.day), wd = col >= 0 ? (cal.weekdays || [])[col] : null;
    var mo = (cal.months || [])[d.month - 1];
    return (wd ? wd.name.slice(0, 3) + ' ' : '') + d.day + ' ' + (mo ? mo.name : d.month) + (d.year !== cal.current_year ? ' ' + d.year : '');
  };

  // Asks the server for the next dates after a short pause in typing. A
  // late answer for an older draft is dropped.
  Drawer.prototype._schedulePreview = function (delay) {
    var self = this, S = this.state;
    if (!S || S.rep.type !== 'rule' || S.rep.locked || !S.rule) return;
    var Rl = Chronicle.calendarRule;
    this._stopPreview();
    var bad = Rl.validate(S.rule.form, this._env());
    if (bad) { this._setNext('idle', [], 'Dates appear once the rule is complete.'); return; }
    this._setNext('loading');
    var seq = this._previewSeq;
    this._previewTimer = setTimeout(function () { self._runPreview(S, seq); }, delay == null ? 350 : delay);
  };

  Drawer.prototype._runPreview = function (S, seq) {
    var self = this, Rl = Chronicle.calendarRule, r = S.rep;
    if (this.state !== S || seq !== this._previewSeq) return;
    var s = this._readDate('es'), until = r.end === 'until' && $('#cal5-euM', this.el) ? this._readDate('eu') : null;
    var body = {
      recurrence_rule: Rl.toRule(S.rule.form),
      year: s.y, month: s.m, day: s.d, count: 5,
      recurrence_max_occurrences: r.end === 'count' ? Math.max(1, r.count || 1) : null,
      recurrence_end_year: until ? until.y : null, recurrence_end_month: until ? until.m : null, recurrence_end_day: until ? until.d : null
    };
    var ctl = typeof AbortController === 'function' ? new AbortController() : null;
    this._previewAbort = ctl;
    var fail = function (msg) { if (self.state === S && seq === self._previewSeq) self._setNext('error', [], msg); };
    Chronicle.apiFetch(this.view.apiBase + '/recurrence/preview', { method: 'POST', body: body, signal: ctl ? ctl.signal : undefined }).then(function (resp) {
      if (self.state !== S || seq !== self._previewSeq) return null;
      if (!resp.ok) {
        return resp.json().then(function (j) { fail((j && (j.message || j.error)) || 'The dates couldn’t be worked out.'); }, function () { fail('The dates couldn’t be worked out.'); });
      }
      return resp.json().then(function (j) {
        if (self.state !== S || seq !== self._previewSeq) return;
        if (!j || !Array.isArray(j.dates)) { fail('The dates couldn’t be worked out.'); return; }
        var dates = j.dates;
        if (!dates.length) self._setNext('empty', [], 'No dates found. Loosen a condition.');
        else if (j.truncated && dates.length < 5) self._setNext('partial', dates, 'No more dates found for a long while.');
        else self._setNext('ok', dates);
      }, function () { fail('The dates couldn’t be worked out.'); });
    }, function (err) {
      if (err && err.name === 'AbortError') return;
      fail('Couldn’t work out the dates. Check your connection.');
    });
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
    // Update is partial: a rule is sent as a value to replace it, null to
    // clear it, and left out to keep it (the case for a rule that is hidden
    // from this viewer, and for any event that never had one).
    if (r.type === 'rule' && !r.locked) {
      var Rl = Chronicle.calendarRule;
      var unchanged = !!(S.rule.original && Rl.sameRule(S.rule.original, S.rule.form));
      var bad = unchanged ? null : Rl.validate(S.rule.form, this._env());
      if (bad) { this._fail(bad, $('#cal5-radd', this.el) || $('#cal5-edRep', this.el)); return null; }
      body.is_recurring = true;
      body.recurrence_type = 'rule';
      body.recurrence_interval = null;
      // Unchanged since the drawer opened: leave it out so a rule naming
      // something since deleted does not block saving the title.
      if (!unchanged) body.recurrence_rule = Rl.toRule(S.rule.form);
    } else if (!r.locked) {
      body.is_recurring = !!r.type;
      body.recurrence_type = r.type || null;
      body.recurrence_interval = r.type === 'custom' ? Math.max(2, parseInt(($('#cal5-edRepI', this.el) || {}).value, 10) || 3) : null;
      if (r.hadRule) body.recurrence_rule = null;
    }
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
    this._on(this.scrim, 'click', function () { if (self.isOpen()) self.close(); });
    this._on(el, 'keydown', function (e) {
      if (e.key === 'Escape' && self.isOpen()) { e.stopPropagation(); self.close(); }
    });
    this._on(el, 'change', function (e) {
      var t = e.target, S = self.state;
      if (!S) return;
      // A month's days depend on its month (and year, for leap days).
      var mm = /^cal5-(es|ee|eu)M$/.exec(t.id);
      if (mm) {
        var p = mm[1], yEl = $('#cal5-' + (p === 'eu' ? 'euY' : p + 'Y'), el);
        var y = parseInt(yEl && yEl.value, 10) || self.cal.current_year;
        $('#cal5-' + p + 'D', el).innerHTML = self._dayOptions(y, +t.value, 1);
      }
      if (t.id === 'cal5-edRep') { self._repeatChosen(t.value); return; }
      if (t.dataset && (t.dataset.f || t.dataset.rf)) self._ruleFieldChanged(t);
      if (S.rep.type === 'rule' && /^cal5-(es|eu)[MDY]$/.test(t.id)) self._schedulePreview();
    });
    this._on(el, 'input', function (e) {
      if (!self.state) return;
      if (e.target.id === 'cal5-edRepI') self.state.rep.every = parseInt(e.target.value, 10) || 3;
      if (e.target.id === 'cal5-edRepN') { self.state.rep.count = parseInt(e.target.value, 10) || 1; self._schedulePreview(); }
      if (e.target.id === 'cal5-edT') $('#cal5-edErr', el).hidden = true;
      if (e.target.dataset && e.target.dataset.rf === 'shift') self._ruleFieldChanged(e.target);
      if (/^cal5-(es|eu)Y$/.test(e.target.id)) self._schedulePreview();
    });
    this._on(el, 'click', function (e) {
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
        self._schedulePreview();
      } else if (t.dataset.r) self._ruleClick(t);
    });
    // The "more" details keeps its own open state; remember it so a redraw
    // of the rule box does not fold it shut.
    this._on(el, 'toggle', function (e) {
      if (self.state && self.state.rule && e.target.id === 'cal5-rmore') self.state.rule.more = e.target.open;
    }, true);
  };

  // One drawer per mounted calendar, made the first time it is asked for.
  window.Chronicle = window.Chronicle || {};
  Chronicle.calendarEventDrawer = {
    open: function (view, ev, day) {
      if (!view._eventDrawer) view._eventDrawer = new Drawer(view);
      view._eventDrawer.open(ev, day);
    },
    isOpen: function (view) { return !!(view._eventDrawer && view._eventDrawer.isOpen()); },
    // The class itself, so the request-body logic can be unit tested.
    Drawer: Drawer
  };
})();
