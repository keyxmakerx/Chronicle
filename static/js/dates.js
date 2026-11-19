/**
 * dates.js -- real-world dates in the reader's own way.
 *
 * The server renders <time datetime="..." data-date="FORM"> with a day-first,
 * month-as-a-word fallback (internal/timeutil/date.go). This rewrites each one
 * with Intl.DateTimeFormat in the reader's saved time zone (html[data-view-tz])
 * or the device's, in the device's language, so Chicago reads "Nov 18, 2026"
 * and London "18 Nov 2026". The month is always a word. Forms must match
 * timeutil's DateForm values.
 *
 * <time data-date-title> keeps its text (e.g. "Edited 2 days ago") and only
 * gets the exact date and time as its title.
 */
(function () {
  'use strict';

  var FORMS = {
    'short': { day: 'numeric', month: 'short', year: 'numeric' },
    'day': { day: 'numeric', month: 'short' },
    'long': { day: 'numeric', month: 'long', year: 'numeric' },
    'weekday': { weekday: 'long', day: 'numeric', month: 'long' },
    'with-time': { day: 'numeric', month: 'short', year: 'numeric', hour: 'numeric', minute: '2-digit' }
  };

  // format returns the text for an ISO instant, or null when it cannot be
  // read (the server's fallback text then stays).
  function format(iso, form, opts) {
    var o = FORMS[form];
    var d = new Date(iso);
    if (!o || isNaN(d.getTime())) return null;
    opts = opts || {};
    var spec = Object.assign({}, o);
    if (opts.timeZone) spec.timeZone = opts.timeZone;
    try {
      return new Intl.DateTimeFormat(opts.locale || undefined, spec).format(d);
    } catch (e) {
      // An unknown saved zone falls back to the device's.
      delete spec.timeZone;
      try { return new Intl.DateTimeFormat(opts.locale || undefined, spec).format(d); } catch (e2) { return null; }
    }
  }

  function viewZone() {
    var h = typeof document !== 'undefined' && document.documentElement;
    return (h && h.getAttribute('data-view-tz')) || undefined;
  }

  function apply(root) {
    var scope = root && root.querySelectorAll ? root : document;
    var zone = viewZone();
    var els = scope.querySelectorAll('time[data-date], time[data-date-title]');
    for (var i = 0; i < els.length; i++) {
      var el = els[i];
      var iso = el.getAttribute('datetime');
      var form = el.getAttribute('data-date');
      if (form) {
        var s = format(iso, form, { timeZone: zone });
        if (s) el.textContent = s;
      }
      if (el.hasAttribute('data-date-title')) {
        var t = format(iso, 'with-time', { timeZone: zone });
        if (t) el.title = t;
      }
    }
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { format: format, FORMS: FORMS };
  }
  if (typeof document === 'undefined') return;

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { apply(document); });
  } else {
    apply(document);
  }
  // A swapped fragment arrives with the server's fallback text.
  document.addEventListener('htmx:afterSwap', function (e) { apply(e.target); });
  document.addEventListener('htmx:load', function (e) { apply(e.target); });
})();
