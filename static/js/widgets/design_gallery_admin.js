/**
 * design_gallery_admin.js — the admin queue for the server gallery
 * (/admin/design-gallery). It lists entries held for review (and visible ones,
 * so a visible one can be taken down), shows what the checks or the reports
 * said, and posts Approve, Reject or Remove. The server is the authority: the
 * routes are site-admin only and refuse a decision that no longer applies.
 *
 * Nothing here is typed by the admin, so no text goes back through markup
 * except as escaped server data.
 */
(function () {
  'use strict';

  var TABS = [['pending', 'Waiting for review'], ['visible', 'Visible']];
  var REPORT_WORDS = { adult: 'Adult or vulgar', spam: 'Spam or advertising', other: 'Something else' };

  function esc(t) {
    t = String(t == null ? '' : t);
    return window.Chronicle && window.Chronicle.escapeHtml ? window.Chronicle.escapeHtml(t) : t.replace(/[^\w #.,:'’-]/g, '');
  }
  function nice(v) { v = String(v || '').replace(/[-_]+/g, ' ').trim(); return v.charAt(0).toUpperCase() + v.slice(1); }
  function when(iso) { var d = new Date(iso); return isNaN(d) ? '' : d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }); }
  function tabOf(v) { return v === 'visible' ? 'visible' : 'pending'; }
  function byId(list, id) { for (var i = 0; i < list.length; i++) if (list[i].id === id) return list[i]; return null; }

  function init(root) {
    var st = { status: 'pending', page: 1, data: null, err: '', msg: '', busy: false };

    function card(e) {
      var h = '<article class="card space-y-2" aria-label="' + esc(e.name) + '"><div class="flex flex-wrap items-baseline gap-2"><h2 class="text-base font-semibold text-fg">' + esc(e.name) + '</h2>' +
        '<span class="text-xs text-fg-muted">by ' + esc(e.makerName) + ' · ' + esc(nice(e.kind)) + ' · ' + esc(e.systemId || 'no game system') + ' · ' + esc(when(e.publishedAt)) + '</span></div>';
      if (e.note) h += '<p class="text-sm text-fg">' + esc(e.note) + '</p>';
      if (e.summary) h += '<p class="text-xs text-fg-muted">Parts: ' + esc(e.summary) + '</p>';
      if (e.reasons && e.reasons.length) {
        h += '<div><p class="text-xs font-medium text-fg">Why it is here</p><ul class="text-sm text-fg-secondary list-disc pl-5">' + e.reasons.map(function (r) { return '<li>' + esc(r) + '</li>'; }).join('') + '</ul></div>';
      }
      if (e.reports && e.reports.length) {
        h += '<div><p class="text-xs font-medium text-fg">Reports (' + e.reports.length + ')</p><ul class="text-sm text-fg-secondary list-disc pl-5">' +
          e.reports.map(function (r) { return '<li>' + esc(REPORT_WORDS[r.reason] || 'Something else') + (r.note ? ': ' + esc(r.note) : '') + '</li>'; }).join('') + '</ul></div>';
      }
      h += '<div class="flex flex-wrap gap-2">';
      if (e.status === 'pending') {
        h += '<button type="button" class="btn btn-primary btn-sm" data-ga="approve" data-id="' + esc(e.id) + '" aria-label="Approve ' + esc(e.name) + '">Approve</button>' +
          '<button type="button" class="btn btn-secondary btn-sm" data-ga="reject" data-id="' + esc(e.id) + '" aria-label="Reject ' + esc(e.name) + '">Reject</button>';
      } else {
        h += '<button type="button" class="btn btn-danger btn-sm" data-ga="remove" data-id="' + esc(e.id) + '" aria-label="Remove ' + esc(e.name) + '">Remove</button>';
      }
      return h + '</div></article>';
    }
    function view() {
      var d = st.data, h = '<div class="flex gap-2 mb-3" role="group" aria-label="Which entries">' + TABS.map(function (t) {
        return '<button type="button" class="btn ' + (st.status === t[0] ? 'btn-primary' : 'btn-secondary') + ' btn-sm" data-ga="tab" data-v="' + t[0] + '" aria-pressed="' + (st.status === t[0]) + '">' + t[1] + '</button>';
      }).join('') + '</div>';
      h += '<p class="text-sm text-fg" role="alert" data-ga-err></p><p class="text-sm text-fg-secondary" role="status" aria-live="polite" data-ga-msg></p>';
      if (!d) h += '<p class="text-sm text-fg-secondary">Loading…</p>';
      else if (!d.entries.length) h += '<div class="card text-center py-10"><p class="text-sm text-fg-secondary">' + (st.status === 'pending' ? 'Nothing is waiting for review.' : 'No designs are shown in the gallery yet.') + '</p></div>';
      else {
        h += '<div class="space-y-3">' + d.entries.map(card).join('') + '</div>';
        var pages = Math.max(1, Math.ceil(d.total / d.perPage));
        if (pages > 1) {
          h += '<div class="flex items-center justify-between mt-3"><button type="button" class="btn btn-secondary btn-sm" data-ga="prev"' + (d.page > 1 ? '' : ' disabled') + '>Previous</button>' +
            '<span class="text-xs text-fg-muted">Page ' + d.page + ' of ' + pages + '</span><button type="button" class="btn btn-secondary btn-sm" data-ga="next"' + (d.page < pages ? '' : ' disabled') + '>Next</button></div>';
        }
      }
      var a = document.activeElement, act = a && root.contains(a) && a.getAttribute('data-ga'), v = act && a.getAttribute('data-v');
      root.innerHTML = h;
      var e = root.querySelector('[data-ga-err]'), m = root.querySelector('[data-ga-msg]');
      if (e) e.textContent = st.err;
      if (m) m.textContent = st.msg;
      if (act === 'tab') { var tb = root.querySelectorAll('[data-ga="tab"]'); for (var i = 0; i < tb.length; i++) if (tb[i].getAttribute('data-v') === v) tb[i].focus(); }
    }
    function load() {
      var want = st.status;
      return window.Chronicle.apiAction('/admin/design-gallery/entries?status=' + encodeURIComponent(want) + '&page=' + st.page).then(function (d) {
        if (want !== st.status) return;
        st.data = d; st.err = ''; st.page = d.page;
        // A decision on the last entry of a page leaves it empty; step back.
        if (!d.entries.length && d.page > 1) { st.page = d.page - 1; return load(); }
        view();
      }).catch(function (e) { st.err = e.message || 'Could not load the queue. Reload the page.'; view(); });
    }
    function decide(act, id) {
      var e = byId((st.data && st.data.entries) || [], id); if (!e || st.busy) return;
      st.busy = true;
      window.Chronicle.apiAction('/admin/design-gallery/' + encodeURIComponent(e.id) + '/' + act, { method: 'POST' }).then(function () {
        st.busy = false; st.err = '';
        st.msg = act === 'approve' ? '“' + e.name + '” is now in the gallery.' : act === 'reject' ? '“' + e.name + '” was not approved.' : '“' + e.name + '” was taken down.';
        load();
      }).catch(function (x) {
        // Reload first so a stale page is replaced, then say why it was refused.
        st.busy = false;
        return load().then(function () { st.msg = ''; st.err = x.message || 'That did not work. Reload and try again.'; view(); });
      });
    }
    root.addEventListener('click', function (ev) {
      var b = ev.target.closest && ev.target.closest('button'); if (!b || !root.contains(b)) return;
      var act = b.getAttribute('data-ga'), id = b.getAttribute('data-id');
      if (act === 'tab') { st.status = tabOf(b.getAttribute('data-v')); st.page = 1; st.data = null; st.err = ''; st.msg = ''; view(); load(); }
      else if (act === 'prev' || act === 'next') { st.page = Math.max(1, st.page + (act === 'next' ? 1 : -1)); load(); }
      else if (act === 'approve' || act === 'reject' || act === 'remove') decide(act, id);
    });
    view(); load();
  }

  if (window.Chronicle && window.Chronicle.register) {
    window.Chronicle.register('design_gallery_admin', { init: init, destroy: function (root) { root.innerHTML = ''; } });
  }
  window.DesignGalleryAdmin = { tabOf: tabOf };
})();
