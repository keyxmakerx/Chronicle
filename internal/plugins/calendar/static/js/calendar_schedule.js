/* calendar_schedule.js — the Painter's write driver (C-CALV4-RSVP-P8 Part B).
 *
 * WHAT THIS FILE IS, AND WHAT IT DELIBERATELY IS NOT.
 *
 * It is the ONLY script on the /schedule page, and it exists for exactly one
 * reason: the scheduler's availability endpoints take JSON over PUT, and a
 * <form> cannot send either. Everything else on this page is a link or a plain
 * form — the week stepper, the zoom and band segments, the candidate cards, the
 * scope segment, the RSVP tri-state and the suggestion dock are all GETs and
 * POSTs against routes that already exist, which is what makes the whole
 * surface state-addressable and reproducible in a render harness.
 *
 * IT IS NOT A SECOND WRITE PATH. Both targets below are the scheduler's own
 * shipped routes, read off data attributes the server emitted:
 *
 *   Every week      PUT …/availability/mine        the member's NORMAL HOURS
 *   This week only  PUT …/availability/exceptions  a DATE EXCEPTION, per day
 *
 * Forking a third endpoint would fork the composition invariant with it — "an
 * offer only ever adds, and never downgrades an hour already marked preferred"
 * is enforced in the scheduler's service, and a second writer is a second place
 * to get it wrong.
 *
 * NOTHING HERE DERIVES A DATE OR A WEEKDAY. Every day row carries data-day (the
 * ISO date) and data-weekday (0=Sun..6=Sat, the scheduler's own index) and every
 * tick carries data-hour, all emitted server-side. Re-deriving any of them from
 * a printed label is how a Monday becomes a Sunday at a locale boundary.
 *
 * PAGE-SIDE JS IS LEGAL HERE. The no-JS law is a statement about
 * internal/widgets/calendar_block/** and about HTMX-swapped fragments
 * (boot.js disables script tags inside them); it says nothing about a <script>
 * in a page templ, and calendar_daycard.js is the shipped precedent this copies
 * — AssetURL + defer, mounted beside its own scaffold.
 */
(function () {
  'use strict';

  var form = document.querySelector('[data-schedule-paint]');
  if (!form) return;

  var saveURL = form.getAttribute('data-save-url') || '';
  var exceptionsURL = form.getAttribute('data-exceptions-url') || '';
  var scope = form.getAttribute('data-scope') || 'week';
  var zone = form.getAttribute('data-zone') || '';
  var csrf = (form.querySelector('input[name="csrf_token"]') || {}).value || '';
  var status = form.querySelector('[data-schedule-status]');

  /* say() is the ONLY feedback channel, and it is an aria-live region rather
     than a toast: the member has just pressed a button and is entitled to know
     what happened without watching for something that fades. */
  function say(text, bad) {
    if (!status) return;
    status.textContent = text;
    status.setAttribute('data-tone', bad ? 'bad' : 'ok');
  }

  /* runsFor collects the ticked hours of one day row into contiguous
     [start,end) MINUTE runs.

     Runs, not hours: the storage is a range table, and writing twelve one-hour
     rows where the member meant one four-hour window would burn their per-week
     row budget on an artefact of the grid's resolution. */
  function runsFor(row, name) {
    var ticks = row.parentNode.querySelectorAll(
      'input.sc-pick[name="' + name + '"][data-day="' + row.getAttribute('data-day') + '"]'
    );
    var hours = [];
    Array.prototype.forEach.call(ticks, function (t) {
      if (t.checked) hours.push(parseInt(t.getAttribute('data-hour'), 10));
    });
    hours.sort(function (a, b) { return a - b; });
    var runs = [];
    var i = 0;
    while (i < hours.length) {
      var start = hours[i];
      var end = hours[i] + 1;
      while (i + 1 < hours.length && hours[i + 1] === end) { end = hours[i + 1] + 1; i++; }
      runs.push({ start: start * 60, end: end * 60 });
      i++;
    }
    return runs;
  }

  function dayRows() {
    return form.querySelectorAll('.sc-paintgrid .sc-row[data-day]');
  }

  /* collect() walks the grid once and returns, per day, the available runs and
     the preferred runs.

     PREFERRED IS COMPOSED INTO AVAILABLE HERE TOO, not only on the server: the
     server's composition is the authority, and sending a preferred hour that is
     not also available would ask it to repair input this driver could simply not
     have malformed. Two agreeing halves, not one trusting the other. */
  function collect() {
    var days = [];
    var seen = {};
    Array.prototype.forEach.call(dayRows(), function (row) {
      var date = row.getAttribute('data-day');
      if (!date || seen[date]) return;
      seen[date] = true;
      var free = runsFor(row, 'free');
      var pref = runsFor(row, 'pref');
      var merged = free.slice();
      pref.forEach(function (p) {
        var covered = free.some(function (f) { return f.start <= p.start && p.end <= f.end; });
        if (!covered) merged.push({ start: p.start, end: p.end });
      });
      days.push({
        date: date,
        weekday: parseInt(row.getAttribute('data-weekday'), 10),
        free: merged,
        pref: pref
      });
    });
    return days;
  }

  function send(method, url, body) {
    return fetch(url, {
      method: method,
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
      body: JSON.stringify(body)
    }).then(function (res) {
      if (!res.ok) throw new Error('HTTP ' + res.status);
      return res;
    });
  }

  /* THE RECURRING PATH is one atomic replace-all: the endpoint takes the whole
     pattern and swaps it, so the driver sends the complete grid rather than a
     diff. A diff over a replace-all endpoint is a lost update waiting for two
     tabs. */
  function saveRecurring(days) {
    var blocks = [];
    days.forEach(function (d) {
      d.free.forEach(function (r) {
        blocks.push({
          dayOfWeek: d.weekday, startMinute: r.start, endMinute: r.end, state: 'available'
        });
      });
      d.pref.forEach(function (r) {
        blocks.push({
          dayOfWeek: d.weekday, startMinute: r.start, endMinute: r.end, state: 'preferred'
        });
      });
    });
    return send('PUT', saveURL, { tz: zone, blocks: blocks });
  }

  /* THE PER-DATE PATH is one call per day, because the endpoint replaces ONE
     date's overrides atomically and there is no whole-week variant.

     Sequential, not parallel: seven concurrent writes against one member's rows
     is a lock-ordering problem nobody asked for, and a week is seven requests
     once, on a button press. The first failure stops the run and SAYS SO — a
     partial save reported as success is worse than a failure. */
  function saveExceptions(days) {
    return days.reduce(function (chain, d) {
      return chain.then(function () {
        var blocks = [];
        d.free.forEach(function (r) {
          blocks.push({ startMinute: r.start, endMinute: r.end, state: 'available' });
        });
        d.pref.forEach(function (r) {
          blocks.push({ startMinute: r.start, endMinute: r.end, state: 'preferred' });
        });
        return send('PUT', exceptionsURL, { onDate: d.date, tz: zone, blocks: blocks });
      });
    }, Promise.resolve());
  }

  function save(days) {
    say('Saving…');
    var work = scope === 'recurring' ? saveRecurring(days) : saveExceptions(days);
    return work.then(function () {
      say(scope === 'recurring'
        ? 'Saved. These are your normal hours from now on.'
        : 'Saved for this week. Your usual pattern is untouched.');
    }).catch(function () {
      /* NO DETAIL, ON PURPOSE. The member cannot act on an HTTP status, and the
         one thing they need to know is whether their week is stored. */
      say('That did not save. Nothing has changed — try again.', true);
    });
  }

  var saveBtn = form.querySelector('[data-schedule-save]');
  if (saveBtn) {
    saveBtn.addEventListener('click', function () { save(collect()); });
  }

  /* CLEAR sends an EMPTY set through the same endpoints, which is the documented
     way to revert: an empty exception set puts the day's usual pattern back, and
     an empty recurring pattern is a member with no normal hours. It is not a
     delete route and it does not need one. */
  var clearBtn = form.querySelector('[data-schedule-clear]');
  if (clearBtn) {
    clearBtn.addEventListener('click', function () {
      Array.prototype.forEach.call(form.querySelectorAll('input.sc-pick'), function (t) {
        t.checked = false;
      });
      save(collect().map(function (d) { return { date: d.date, weekday: d.weekday, free: [], pref: [] }; }));
    });
  }

  /* "OUT JUST THIS WEEK" WRITES TWO THINGS, and the confirm popover says so
     before either happens. The RSVP goes first and the week second, so that
     when the scheduler cannot be reached the member's answer is still recorded
     — which is exactly what the popover's last line promises. */
  var outBtn = document.querySelector('[data-schedule-outweek]');
  if (outBtn) {
    outBtn.addEventListener('click', function () {
      var answer = document.querySelector('.sc-arow');
      var post = Promise.resolve();
      if (answer) {
        var body = new FormData();
        body.append('csrf_token', csrf);
        body.append('status', 'no');
        post = fetch(answer.getAttribute('action'), {
          method: 'POST', credentials: 'same-origin', body: body
        });
      }
      post.then(function () {
        var days = collect().map(function (d) {
          return { date: d.date, weekday: d.weekday, free: [], pref: [] };
        });
        return saveExceptions(days);
      }).then(function () {
        window.location.reload();
      }).catch(function () {
        say('Your answer was recorded; the week could not be marked. Try again.', true);
      });
    });
  }
})();
