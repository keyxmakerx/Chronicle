/**
 * calendar_outcome.js — changing what became of events: the sheet that asks
 * about the events on the days the Director has just moved today past, and
 * the change form in an event's card. Kept out of calendar_view.js, which
 * every page loads, since both are wanted only now and then.
 *
 * calendar_view.js fetches this the first time today moves forward over
 * events (only the owner moves today, and Calendar settings can switch the
 * question off), then calls Chronicle.calendarOutcome.open(view, from, to,
 * kit); or the first time a button in a card's outcome part is pressed, then
 * calls cardClick(view, box, target, kit). The sheet reads GET .../events/passed?from&to, which lists each
 * one-off event still planned whose last day is now behind and each date of
 * a repeating event, as this viewer may see them. Nothing to ask, no sheet.
 *
 * Each row offers Happened (picked to start with, since an unanswered event
 * reads as happened), Didn't and Moved, which asks for the new day. A
 * one-off event also takes a line on what happened. Done keeps every one-off
 * answer in one POST .../events/batch (all or nothing, and Undo is the same
 * route the other way); a repeating date is answered with that date's skip
 * or move override, which Undo takes back. Not now, Escape or the close mark
 * leave everything unanswered.
 *
 * On a computer the sheet grows out of the today pill, where the date was
 * moved; on a phone it slides up from the bottom edge. Calm fades it, Off
 * and a key show it at once.
 *
 * The card's form offers the four states, a new day for Moved and the line,
 * and saves only what changed with PUT .../events/:id.
 */
(function () {
  'use strict';

  var MOVE_CHOICES = 30; // days after the event's own offered as "moved to"

  function dkey(d) { return d.y + '-' + d.m + '-' + d.d; }

  // "Hearthday, Firstfrost 12", with the year when it is not today's year.
  function longDate(cal, CalDate, d) {
    var mo = (cal.months || [])[d.m - 1], col = CalDate.weekdayCol(cal, d.y, d.m, d.d);
    var wd = col >= 0 ? (cal.weekdays || [])[col] : null;
    return (wd && wd.name ? wd.name + ', ' : '') + (mo ? mo.name : 'Month ' + d.m) + ' ' + d.d + (d.y !== cal.current_year ? ', ' + d.y : '');
  }

  // "Firstfrost 11 has passed" for one day, "Firstfrost 9 to 11 have
  // passed" for several; to is the new today, so the last passed day is
  // the one before it.
  function heading(cal, CalDate, from, to) {
    var last = CalDate.addDays(cal, to, -1), name = function (d) { var mo = (cal.months || [])[d.m - 1]; return mo ? mo.name : 'Month ' + d.m; };
    if (dkey(from) === dkey(last)) return name(last) + ' ' + last.d + ' has passed';
    var a = name(from) + ' ' + from.d + (from.y !== last.y ? ', ' + from.y : '');
    var b = from.m === last.m && from.y === last.y ? String(last.d) : name(last) + ' ' + last.d;
    return a + ' to ' + b + ' have passed';
  }

  // answers holds, per row key, {choice: 'happened'|'didnt'|'moved', to:
  // {y,m,d}, line}. requests turns them into the writes Done sends and the
  // writes Undo would send, so both are worked out before anything is sent.
  // A span that moves keeps its length, as a drag on the year track does.
  function requests(items, answers, cal, CalDate) {
    var update = [], undo = [], occ = [];
    items.forEach(function (it) {
      var a = answers[it.key] || { choice: 'happened' }, ev = it.ev, line = (a.line || '').trim();
      if (ev.repeating) {
        if (a.choice === 'didnt') occ.push({ id: ev.id, date: it.date, body: { action: 'skip' } });
        else if (a.choice === 'moved' && a.to && dkey(a.to) !== dkey(it.date)) occ.push({ id: ev.id, date: it.date, body: { action: 'move', year: a.to.y, month: a.to.m, day: a.to.d } });
        return;
      }
      var ch = { status: a.choice === 'didnt' ? 'called_off' : a.choice === 'moved' ? 'moved' : 'happened' };
      var back = { status: 'planned', outcome: ev.outcome || null };
      if (ch.status === 'moved') {
        if (!a.to || dkey(a.to) === dkey(it.date)) return; // not moved after all
        ch.year = a.to.y; ch.month = a.to.m; ch.day = a.to.d;
        back.year = ev.year; back.month = ev.month; back.day = ev.day;
        if (ev.end_year != null) {
          var delta = CalDate.dayIndex(cal, a.to.y, a.to.m, a.to.d) - CalDate.dayIndex(cal, ev.year, ev.month, ev.day);
          var end = CalDate.addDays(cal, { y: ev.end_year, m: ev.end_month, d: ev.end_day }, delta);
          ch.end_year = end.y; ch.end_month = end.m; ch.end_day = end.d;
          back.end_year = ev.end_year; back.end_month = ev.end_month; back.end_day = ev.end_day;
        }
      }
      if (a.line != null && line !== (ev.outcome || '')) ch.outcome = line || null; // no line given keeps the one there
      update.push({ id: ev.id, changes: ch });
      undo.push({ id: ev.id, changes: back });
    });
    return { update: update, undo: undo, occ: occ };
  }

  function rowKey(ev) { return ev.id + '|' + ev.year + '-' + ev.month + '-' + ev.day; }

  function monthName(cal, m) { var mo = (cal.months || [])[m - 1]; return mo && mo.name ? mo.name : 'Month ' + m; }

  // "Firstfrost 9", or "Firstfrost 9 to 10" for an event over several days.
  function spanWords(cal, ev) {
    var a = monthName(cal, ev.month) + ' ' + ev.day;
    if (ev.end_year == null || (ev.end_year === ev.year && ev.end_month === ev.month && ev.end_day === ev.day)) return a;
    return a + ' to ' + (ev.end_year === ev.year && ev.end_month === ev.month ? '' : monthName(cal, ev.end_month) + ' ') + ev.end_day;
  }

  var CHOICES = [['happened', 'Happened'], ['didnt', 'Didn’t'], ['moved', 'Moved']];
  var seq = 0;

  // The sheet itself. Returns the handle calendar_view.js keeps.
  function open(view, from, to, kit) {
    var cal = view.cal, CalDate = kit.CalDate, esc = kit.esc, uid = 'dp' + (++seq);
    var url = view.apiBase + '/events/passed?from=' + dkey(from) + '&to=' + dkey(to);
    var state = { open: false, gone: false, items: [], moves: [], el: null, scrim: null, opener: null };
    var handle = {
      isOpen: function () { return state.open; },
      notNow: function () { close(); },
      destroy: function () { state.gone = true; close(true); }
    };
    Chronicle.apiFetch(url).then(function (resp) { return resp.ok ? resp.json() : null; }).then(function (got) {
      if (state.gone || !got || !Array.isArray(got.events) || !got.events.length || !view.el.isConnected) return;
      state.items = got.events.map(function (ev) { return { key: rowKey(ev), ev: ev, date: { y: ev.year, m: ev.month, d: ev.day } }; });
      state.truncated = !!got.truncated;
      build();
    }).catch(function () { /* nothing asked; the events stay as they are */ });

    // Moved offers the month after the event's day, the new today first
    // picked when it falls in it; "Another day…" takes any date.
    function moveChoices(it) {
      var list = [], pick = 0, tk = dkey(to);
      for (var n = 1; n <= MOVE_CHOICES; n++) {
        var d = CalDate.addDays(cal, it.date, n);
        if (dkey(d) === tk) pick = n - 1;
        list.push(d);
      }
      return { list: list, pick: pick };
    }

    function rowHTML(it, i) {
      var ev = it.ev, dm = ev.visibility === 'dm_only', mv = state.moves[i], nid = uid + 'n' + i, had = !!ev.outcome;
      var months = (cal.months || []).map(function (mo, k) {
        return '<option value="' + (k + 1) + '"' + (k + 1 === it.date.m ? ' selected' : '') + '>' + esc(mo.name || 'Month ' + (k + 1)) + '</option>';
      }).join('');
      return '<li class="dprow' + (dm ? ' dm' : '') + '" data-dp-row="' + i + '">' +
        '<div class="dphd"><b id="' + nid + '">' + esc(ev.name) + '</b>' +
          (dm ? '<span class="dponly"><i class="fa-solid fa-eye-slash" aria-hidden="true"></i>Only you</span>' : '') + '</div>' +
        '<p class="dpwhen">' + esc(spanWords(cal, ev)) + (ev.repeating ? ' · one date of a repeating event' : '') + '</p>' +
        '<div class="dpch" role="group" aria-labelledby="' + nid + '">' + CHOICES.map(function (c) {
          return '<button type="button" class="dpchip" data-dp-ch="' + c[0] + '" aria-pressed="' + (c[0] === 'happened') + '">' + c[1] + '</button>';
        }).join('') + '</div>' +
        '<div class="dpmv" data-dp-mv hidden>' +
          '<label class="fld dpto"><span>Moved to</span><select data-dp-to>' + mv.list.map(function (d, k) {
            return '<option value="' + k + '"' + (k === mv.pick ? ' selected' : '') + '>' + esc(longDate(cal, CalDate, d)) + '</option>';
          }).join('') + '<option value="other">Another day…</option></select></label>' +
          '<div class="dpother" data-dp-other hidden>' +
            '<label class="fld"><span>Month</span><select name="month">' + months + '</select></label>' +
            '<label class="fld"><span>Day</span><input type="number" name="day" inputmode="numeric" min="1" value="' + it.date.d + '"></label>' +
            '<label class="fld"><span>Year</span><input type="number" name="year" inputmode="numeric" value="' + it.date.y + '"></label>' +
          '</div></div>' +
        (ev.repeating ? '' :
          '<button type="button" class="qword dpwhy" data-dp-why' + (had ? ' hidden' : '') + '>What happened, if you like</button>' +
          '<label class="fld dpln"' + (had ? '' : ' hidden') + '><span>What happened, if you like</span>' +
            '<input type="text" data-dp-line maxlength="1000" value="' + esc(ev.outcome || '') + '"></label>') +
        '</li>';
    }

    function build() {
      state.moves = state.items.map(moveChoices);
      var hid = uid + 'h', phone = kit.isPhone();
      var scrim = document.createElement('div');
      scrim.className = 'dpscrim';
      var el = document.createElement('div');
      el.className = 'dpsheet' + (phone ? ' phone' : '');
      el.setAttribute('role', 'dialog');
      el.setAttribute('aria-modal', 'true');
      el.setAttribute('aria-labelledby', hid);
      el.innerHTML = '<div class="dpgrip" aria-hidden="true"></div>' +
        '<div class="dphead"><div><h2 id="' + hid + '" tabindex="-1">' + esc(heading(cal, CalDate, from, to)) + '</h2>' +
          '<p>What became of ' + (state.items.length === 1 ? 'this?' : 'these?') + '</p></div>' +
          '<button type="button" class="x" data-dp-later aria-label="Not now">✕</button></div>' +
        '<ol class="dprows">' + state.items.map(rowHTML).join('') + '</ol>' +
        (state.truncated ? '<p class="dpmore">More passed than one sheet holds. The rest stay unanswered and read as happened; change any of them from its card.</p>' : '') +
        '<p class="sterr dperr" role="alert" hidden></p>' +
        '<div class="dpacts"><button type="button" class="btn quiet" data-dp-later>Not now</button><button type="button" class="btn primary" data-dp-done>Done</button></div>';
      state.opener = view.el.querySelector('#cal5-todaypill .tdset') || (document.activeElement !== document.body ? document.activeElement : null);
      view.calEl.appendChild(scrim);
      view.calEl.appendChild(el);
      state.el = el; state.scrim = scrim; state.open = true;
      el.addEventListener('click', onClick);
      el.addEventListener('change', onChange);
      el.addEventListener('keydown', onKey);
      if (!phone) place(el);
      show(el, scrim, phone);
      // The date form that moved today may still be closing and hand the
      // focus back to its button; the heading takes it once that is done.
      var head = el.querySelector('h2');
      head.focus({ preventScroll: true });
      setTimeout(function () { if (state.el === el && !el.contains(document.activeElement)) head.focus({ preventScroll: true }); }, kit.closeMs() + 40);
    }

    // On a computer the sheet hangs under the today pill, inside the
    // calendar, and grows out of the pill.
    function place(el) {
      var box = view.calEl.getBoundingClientRect(), pill = view.el.querySelector('#cal5-todaypill');
      var pr = pill ? pill.getBoundingClientRect() : { left: box.left + box.width / 2, width: 0, bottom: box.top + 56, top: box.top + 56, height: 0 };
      var w = Math.min(440, box.width - 32);
      var x = Math.max(16, Math.min(box.width - w - 16, pr.left + pr.width / 2 - box.left - w / 2));
      var y = Math.max(8, pr.bottom - box.top + 8);
      el.style.width = w + 'px';
      el.style.left = x + 'px';
      el.style.top = y + 'px';
      el.style.maxHeight = Math.max(280, Math.min(640, window.innerHeight - Math.max(0, box.top + y) - 16)) + 'px';
      el.style.transformOrigin = (pr.left + pr.width / 2 - box.left - x) + 'px ' + (pr.top + pr.height / 2 - box.top - y) + 'px';
    }

    // Motion: Off and a key show it at once, Calm fades, otherwise it
    // grows from the pill (a drawer from the bottom edge on a phone).
    function show(el, scrim, phone) {
      el.classList.add('on'); scrim.classList.add('on');
      if (kit.motionOff() || kit.byKey()) return;
      var ms = kit.openMs(), ease = 'cubic-bezier(0.16, 1, 0.3, 1)';
      scrim.animate([{ opacity: 0 }, { opacity: 1 }], { duration: ms, easing: ease });
      if (kit.reducedMotion()) { el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: ms, easing: ease }); return; }
      el.animate(phone ? [{ transform: 'translateY(100%)' }, { transform: 'none' }] : [{ opacity: 0, transform: 'scale(.86)' }, { opacity: 1, transform: 'none' }],
        { duration: phone ? Math.round(ms * 1.3) : ms, easing: ease });
    }

    function close(instant) {
      if (!state.el) return;
      var el = state.el, scrim = state.scrim, had = el.contains(document.activeElement), opener = state.opener;
      state.el = state.scrim = null; state.open = false;
      var finish = function () {
        el.remove(); scrim.remove();
        if (had && opener && opener.isConnected) opener.focus({ preventScroll: true });
      };
      if (instant || kit.motionOff() || kit.byKey()) { finish(); return; }
      var ms = kit.closeMs() * 2, ease = 'cubic-bezier(0.4, 0, 1, 1)', phone = el.classList.contains('phone');
      el.style.pointerEvents = 'none';
      scrim.animate([{ opacity: 1 }, { opacity: 0 }], { duration: ms, easing: ease, fill: 'forwards' });
      var frames = kit.reducedMotion() ? [{ opacity: 1 }, { opacity: 0 }] : phone ? [{ transform: 'none' }, { transform: 'translateY(100%)' }] : [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.92)' }];
      el.animate(frames, { duration: ms, easing: ease, fill: 'forwards' }).finished.then(finish, finish);
    }

    function rowOf(t) { var r = t.closest('[data-dp-row]'); return r ? { el: r, i: +r.dataset.dpRow } : null; }

    function onClick(e) {
      var t = e.target, r = rowOf(t), b;
      if (t.closest('[data-dp-later]')) { close(); return; }
      if ((b = t.closest('[data-dp-done]'))) { done(b); return; }
      if (!r) return;
      if ((b = t.closest('[data-dp-ch]'))) {
        r.el.querySelectorAll('[data-dp-ch]').forEach(function (c) { c.setAttribute('aria-pressed', String(c === b)); });
        r.el.querySelector('[data-dp-mv]').hidden = b.dataset.dpCh !== 'moved';
        return;
      }
      if ((b = t.closest('[data-dp-why]'))) {
        var ln = r.el.querySelector('.dpln');
        b.hidden = true; ln.hidden = false;
        ln.querySelector('input').focus();
      }
    }

    function onChange(e) {
      var sel = e.target.closest('[data-dp-to]'), r = sel && rowOf(sel);
      if (!r) return;
      var other = r.el.querySelector('[data-dp-other]');
      other.hidden = sel.value !== 'other';
      if (!other.hidden) other.querySelector('select').focus();
    }

    // Tab stays inside the sheet while it is open; Escape is the view's
    // (it asks the sheet first).
    function onKey(e) {
      if (e.key !== 'Tab') return;
      var f = Array.prototype.filter.call(state.el.querySelectorAll('button, select, input, [tabindex="0"]'), function (n) { return !n.disabled && n.offsetParent !== null; });
      if (!f.length) return;
      var first = f[0], last = f[f.length - 1], at = document.activeElement;
      if (e.shiftKey && (at === first || !state.el.contains(at) || at === state.el.querySelector('h2'))) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && at === last) { e.preventDefault(); first.focus(); }
    }

    // Reads every row; the first one that cannot be read says why.
    function readAnswers() {
      var out = {}, bad = null;
      state.items.forEach(function (it, i) {
        var row = state.el.querySelector('[data-dp-row="' + i + '"]');
        var on = row.querySelector('[data-dp-ch][aria-pressed="true"]'), a = { choice: on ? on.dataset.dpCh : 'happened' };
        var ln = row.querySelector('[data-dp-line]');
        if (ln) a.line = ln.value;
        if (a.choice === 'moved') {
          var sel = row.querySelector('[data-dp-to]');
          if (sel.value !== 'other') a.to = state.moves[i].list[+sel.value];
          else {
            var num = function (k) { var v = row.querySelector('[name="' + k + '"]').value.trim(); return /^-?\d+$/.test(v) ? parseInt(v, 10) : NaN; };
            var y = num('year'), m = num('month'), d = num('day'), len = isNaN(m) || isNaN(y) ? 0 : CalDate.monthDays(cal, m - 1, y);
            if (isNaN(y) || isNaN(m) || isNaN(d)) bad = bad || { row: row, msg: 'Fill in the day ' + it.ev.name + ' moved to.' };
            else if (d < 1 || d > len) bad = bad || { row: row, msg: monthName(cal, m) + ' has ' + len + ' days.' };
            else if (y === it.date.y && m === it.date.m && d === it.date.d) bad = bad || { row: row, msg: 'Pick another day for ' + it.ev.name + ', or say it happened.' };
            else a.to = { y: y, m: m, d: d };
          }
        }
        out[it.key] = a;
      });
      return { answers: out, bad: bad };
    }

    function occURL(o) {
      return view.apiBase + '/events/' + encodeURIComponent(o.id) + '/occurrences/' + o.date.y + '/' + o.date.m + '/' + o.date.d;
    }

    // Done: the one-off answers go in one batch, all or nothing; the dates
    // of repeating events follow, each its own override.
    function done(btn) {
      var err = state.el.querySelector('.dperr'), r = readAnswers();
      err.hidden = true;
      if (r.bad) {
        err.textContent = r.bad.msg; err.hidden = false;
        var f = r.bad.row.querySelector('[data-dp-other] input, [data-dp-other] select');
        if (f) f.focus();
        return;
      }
      var w = requests(state.items, r.answers, cal, CalDate);
      if (!w.update.length && !w.occ.length) { close(); return; }
      btn.disabled = true;
      var nothing = 'Nothing was saved. Try again.';
      var batch = w.update.length
        ? Chronicle.apiFetch(view.apiBase + '/events/batch', { method: 'POST', body: { update: w.update } })
          .then(function (resp) { return resp.ok ? '' : failWords(resp, nothing); }, function () { return 'Could not reach Chronicle. ' + nothing; })
        : Promise.resolve('');
      batch.then(function (msg) {
        if (msg) { btn.disabled = false; if (state.el) { err.textContent = msg; err.hidden = false; } return; }
        return Promise.all(w.occ.map(function (o) {
          return Chronicle.apiFetch(occURL(o), { method: 'PUT', body: o.body }).then(function (resp) { return resp.ok; }, function () { return false; });
        })).then(function (oks) {
          var kept = w.occ.filter(function (o, i) { return oks[i]; }), missed = w.occ.length - kept.length, n = w.update.length + kept.length;
          close();
          if (state.gone) return;
          view.eventsByMonth = {};
          view.renderMonth();
          var said = missed
            ? 'Saved, but ' + missed + (missed === 1 ? ' date' : ' dates') + ' of repeating events did not change. Change them from the calendar.'
            : 'Saved what became of ' + (n === 1 ? 'that event.' : n + ' events.');
          view.say(said, n ? { label: 'Undo', run: function () { undo(w.undo, kept); } } : null);
        });
      });
    }

    // Undo sends the same routes the other way: the batch puts the events
    // back to planned (and their days back), each override is taken off.
    function undo(back, occs) {
      var steps = [];
      if (back.length) steps.push(Chronicle.apiFetch(view.apiBase + '/events/batch', { method: 'POST', body: { update: back } }).then(function (resp) { return resp.ok; }, function () { return false; }));
      occs.forEach(function (o) {
        steps.push(Chronicle.apiFetch(occURL(o), { method: 'DELETE' }).then(function (resp) { return resp.ok; }, function () { return false; }));
      });
      Promise.all(steps).then(function (oks) {
        if (state.gone || !view.el.isConnected) return;
        view.eventsByMonth = {};
        view.renderMonth();
        view.say(oks.every(Boolean) ? 'Undone. Those events are unanswered again.' : 'Some of that could not be undone. Change those events from their cards.');
      });
    }

    return handle;
  }

  // ---- The event card's change form ----
  var WORDS = [['planned', 'Planned'], ['happened', 'Happened'], ['moved', 'Moved'], ['called_off', 'Called off']];
  var SAID = { planned: 'planned', happened: 'happened', moved: 'moved', called_off: 'called off' };
  function statusOf(ev) { return ev.status || 'planned'; }

  function cardFormHTML(view, ev, kit) {
    var st = statusOf(ev), c = view.cal, CalDate = kit.CalDate, esc = kit.esc, mc = CalDate.monthCount(c);
    var day = st === 'moved' ? { y: ev.year, m: ev.month, d: ev.day } : CalDate.addDays(c, { y: ev.year, m: ev.month, d: ev.day }, 1);
    var months = (c.months || []).map(function (mo, i) {
      return '<option value="' + (i + 1) + '"' + (i + 1 === day.m ? ' selected' : '') + '>' + esc(mo.name || 'Month ' + (i + 1)) + '</option>';
    }).join('') || Array.from({ length: mc }, function (_, i) { return '<option value="' + (i + 1) + '">' + (i + 1) + '</option>'; }).join('');
    return '<div class="evochips" role="group" aria-label="What became of it">' + WORDS.map(function (w) {
        return '<button type="button" class="evochip" data-evout-st="' + w[0] + '" aria-pressed="' + (w[0] === st) + '">' + w[1] + '</button>';
      }).join('') + '</div>' +
      '<div class="evomv strow3" data-evout-mv' + (st === 'moved' ? '' : ' hidden') + '>' +
        '<label class="fld"><span>Moved to</span><select name="month">' + months + '</select></label>' +
        '<label class="fld"><span>Day</span><input type="number" name="day" inputmode="numeric" min="1" value="' + day.d + '"></label>' +
        '<label class="fld"><span>Year</span><input type="number" name="year" inputmode="numeric" value="' + day.y + '"></label></div>' +
      '<label class="fld evoln"><span>What happened, if you like</span><textarea name="outcome" rows="2" maxlength="1000">' + esc(ev.outcome || '') + '</textarea></label>' +
      '<p class="sterr" role="alert" hidden></p>' +
      '<div class="sta"><button type="button" class="btn sm primary" data-evout-save>Save</button><button type="button" class="btn sm quiet" data-evout-cancel>Cancel</button></div>';
  }

  // A press on a button inside the card's outcome part (box).
  function cardClick(view, box, target, kit) {
    var ev = view._findEvent(box.dataset.evout), form = box.querySelector('[data-evout-form]'), opener = box.querySelector('[data-evout-open]');
    if (!ev || !form) return;
    if (target.closest('[data-evout-open]')) {
      var opening = form.hidden;
      if (opening) form.innerHTML = cardFormHTML(view, ev, kit);
      form.hidden = !opening;
      opener.setAttribute('aria-expanded', String(opening));
      if (opening) { var on = form.querySelector('[aria-pressed="true"]'); if (on) on.focus({ preventScroll: true }); }
      return;
    }
    var chip = target.closest('[data-evout-st]');
    if (chip) {
      form.querySelectorAll('[data-evout-st]').forEach(function (b) { b.setAttribute('aria-pressed', String(b === chip)); });
      form.querySelector('[data-evout-mv]').hidden = chip.dataset.evoutSt !== 'moved';
      return;
    }
    if (target.closest('[data-evout-cancel]')) {
      form.hidden = true; form.innerHTML = '';
      opener.setAttribute('aria-expanded', 'false');
      opener.focus({ preventScroll: true });
      return;
    }
    var save = target.closest('[data-evout-save]');
    if (save) saveCard(view, ev, form, save, kit);
  }

  // Sends only what changed, as the event route's partial update reads it,
  // then redraws the month and the card from the server. A span moves
  // whole, as a drag on the year track moves it.
  function saveCard(view, ev, form, btn, kit) {
    var CalDate = kit.CalDate, err = form.querySelector('.sterr'), was = statusOf(ev);
    var picked = form.querySelector('[data-evout-st][aria-pressed="true"]');
    var st = picked ? picked.dataset.evoutSt : was;
    var line = form.querySelector('[name="outcome"]').value.trim();
    var body = {}, fail = function (msg) { err.textContent = msg; err.hidden = false; };
    if (st !== was) body.status = st;
    if (line !== (ev.outcome || '')) body.outcome = line || null;
    if (st === 'moved') {
      var n = function (k) { var v = form.querySelector('[name="' + k + '"]').value.trim(); return /^-?\d+$/.test(v) ? parseInt(v, 10) : NaN; };
      var y = n('year'), m = n('month'), d = n('day');
      if ([y, m, d].some(isNaN)) { fail('Fill in the day it moved to.'); return; }
      var len = CalDate.monthDays(view.cal, m - 1, y);
      if (d < 1 || d > len) { fail('That month has ' + len + ' days.'); return; }
      if (y !== ev.year || m !== ev.month || d !== ev.day) {
        body.year = y; body.month = m; body.day = d; body.status = 'moved';
        if (ev.end_year != null) {
          var end = CalDate.addDays(view.cal, { y: ev.end_year, m: ev.end_month, d: ev.end_day }, CalDate.dayIndex(view.cal, y, m, d) - CalDate.dayIndex(view.cal, ev.year, ev.month, ev.day));
          body.end_year = end.y; body.end_month = end.m; body.end_day = end.d;
        }
      } else if (was !== 'moved') { fail('Pick the day it moved to.'); return; }
    }
    if (!Object.keys(body).length) { view.closeEventDetail(); return; }
    btn.disabled = true;
    Chronicle.apiFetch(view.apiBase + '/events/' + encodeURIComponent(ev.id), { method: 'PUT', body: body }).then(function (resp) {
      if (!resp.ok) return failWords(resp, 'Not saved. Try again.');
      view.eventsByMonth = {};
      view.closeEventDetail();
      view.renderMonth();
      view.say(ev.name + ': ' + SAID[st] + '.');
      return '';
    }, function () { return 'Could not reach Chronicle. Try again.'; }).then(function (msg) {
      btn.disabled = false;
      if (msg) fail(msg);
    });
  }

  function failWords(resp, fallback) {
    return resp.json().then(function (j) { return (j && j.message) || fallback; }, function () { return fallback; });
  }

  Chronicle.calendarOutcome = { open: open, cardClick: cardClick, requests: requests, heading: heading, longDate: longDate, spanWords: spanWords };
})();
