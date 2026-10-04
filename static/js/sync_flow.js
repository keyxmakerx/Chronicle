/**
 * sync_flow.js -- the call flow (sync_flow.templ).
 *
 * 1. Opens a flow: under a failed row of Manage › Sync history (the row is a
 *    <details>) and under a campaign row of Admin › API & access. The flow
 *    is fetched once, the first time it opens, and folds open.
 * 2. Clicking a step opens what happened under it and lights its chain:
 *    every step it came from and led to, with a line in the gutter. The
 *    rest dim. "Caused by" and "Led to" jump along the chain.
 * 3. The chips hide a thing's steps, or show only the problems.
 * Times arrive in UTC and are shown in the reader's own time zone.
 */
(function () {
  'use strict';

  var STAGGER = 0.06;
  var STAGGER_MAX = 1.2;

  function pad(n, w) { n = String(n); while (n.length < (w || 2)) n = '0' + n; return n; }

  function localise(root) {
    root.querySelectorAll('time.sf-time[datetime]').forEach(function (t) {
      if (t.dataset.local) return;
      var d = new Date(t.getAttribute('datetime'));
      if (isNaN(d)) return;
      var hm = pad(d.getHours()) + ':' + pad(d.getMinutes());
      if (t.hasAttribute('data-short')) {
        t.textContent = hm;
      } else if (t.hasAttribute('data-ms')) {
        t.textContent = hm + ':' + pad(d.getSeconds());
        var ms = document.createElement('span');
        ms.textContent = '.' + pad(d.getMilliseconds(), 3);
        t.appendChild(ms);
      } else {
        t.textContent = hm + ':' + pad(d.getSeconds());
      }
      t.dataset.local = '1';
    });
  }

  function words(s) { return s ? s.split(' ').filter(Boolean) : []; }

  function stepRows(flow) { return Array.prototype.slice.call(flow.querySelectorAll('.sf-row[data-step]')); }

  function rowById(flow, id) { return flow.querySelector('.sf-row[data-step="' + id + '"]'); }

  // Every step linked to the selected one: its causes back, its results on.
  function chain(flow, id) {
    var out = {};
    function walk(i, attr) {
      if (out[i]) return;
      var r = rowById(flow, i);
      if (!r) return;
      out[i] = 1;
      words(r.getAttribute(attr)).forEach(function (j) { walk(j, attr); });
    }
    var me = rowById(flow, id);
    if (!me) return out;
    words(me.dataset.cause).forEach(function (j) { walk(j, 'data-cause'); });
    words(me.dataset.then).forEach(function (j) { walk(j, 'data-then'); });
    out[id] = 'me';
    return out;
  }

  function detOf(flow, row) { return flow.querySelector('[data-det="' + row.dataset.step + '"]'); }

  function render(flow) {
    var sel = flow.dataset.selected || '';
    var hide = words(flow.dataset.hidden);
    var prob = flow.dataset.prob === '1';
    var ch = sel ? chain(flow, sel) : {};
    var rows = stepRows(flow);
    var shown = [];
    rows.forEach(function (r) {
      var id = r.dataset.step;
      var vis = hide.indexOf(r.dataset.group) < 0 && (!prob || r.hasAttribute('data-fail') || !!ch[id]);
      var det = detOf(flow, r);
      r.hidden = !vis;
      if (det) det.hidden = !vis;
      var isSel = vis && id === sel;
      r.classList.toggle('sf-sel', isSel);
      r.classList.toggle('sf-dim', !!sel && !ch[id]);
      r.setAttribute('aria-expanded', String(isSel));
      if (det) det.classList.toggle('sf-open', isSel);
      if (vis) shown.push(r);
    });

    // A heading shows while any of its steps do.
    flow.querySelectorAll('.sf-ghead').forEach(function (h) {
      var n = h.nextElementSibling, any = false;
      while (n && !n.classList.contains('sf-ghead') && !n.classList.contains('sf-none')) {
        if (n.matches('.sf-row') && !n.hidden) { any = true; break; }
        n = n.nextElementSibling;
      }
      h.hidden = !any;
    });
    var none = flow.querySelector('.sf-none');
    if (none) none.hidden = shown.length > 0;

    // Gutter: a line from the first linked step to the last, a dot on each.
    var linked = shown.filter(function (r) { return ch[r.dataset.step]; });
    var first = linked[0], last = linked[linked.length - 1], inside = false;
    shown.forEach(function (r) {
      var g = r.querySelector('.sf-gut');
      if (!g) return;
      g.textContent = '';
      if (r === first) inside = true;
      if (sel && inside && first !== last) {
        var line = document.createElement('i');
        line.style.top = r === first ? '50%' : '0';
        line.style.bottom = r === last ? '50%' : '0';
        g.appendChild(line);
      }
      var mark = ch[r.dataset.step];
      if (mark) {
        var dot = document.createElement('b');
        if (mark === 'me') dot.className = 'sf-me';
        g.appendChild(dot);
      }
      if (r === last) inside = false;
    });
    rows.forEach(function (r) {
      if (r.hidden) { var g = r.querySelector('.sf-gut'); if (g) g.textContent = ''; }
    });

    flow.querySelectorAll('[data-hide]').forEach(function (b) {
      b.setAttribute('aria-pressed', String(hide.indexOf(b.dataset.hide) < 0));
    });
    var pb = flow.querySelector('[data-prob]');
    if (pb) pb.setAttribute('aria-pressed', String(prob));
  }

  // Set up a flow just put in the page: local times, the opening stagger,
  // and the selected step's chain.
  function setup(flow) {
    if (flow.dataset.ready) return;
    flow.dataset.ready = '1';
    localise(flow);
    stepRows(flow).forEach(function (r, i) {
      r.style.setProperty('--sf-dly', Math.min(i * STAGGER, STAGGER_MAX).toFixed(2) + 's');
    });
    render(flow);
  }

  // After the first change the rows stay put: showing a hidden row must not
  // replay its opening.
  function change(flow, fn) {
    flow.classList.add('sf-still');
    fn();
    render(flow);
  }

  function openFold(fold) {
    // Measure first so the fold animates from closed.
    void fold.offsetHeight;
    fold.classList.add('sf-open');
  }

  function load(fold, src) {
    var box = fold.firstElementChild;
    if (!box || fold.dataset.loaded) { openFold(fold); return; }
    fold.dataset.loaded = '1';
    box.textContent = 'Loading the call flow…';
    box.className = 'sf-empty';
    openFold(fold);
    fetch(src, { headers: { 'HX-Request': 'true' }, credentials: 'same-origin' })
      .then(function (r) { if (!r.ok) throw new Error(String(r.status)); return r.text(); })
      .then(function (html) {
        // Chronicle's own fragment, every value escaped by templ.
        box.className = '';
        box.innerHTML = html;
        var flow = box.querySelector('[data-sync-flow]');
        if (flow) setup(flow);
      })
      .catch(function () {
        delete fold.dataset.loaded;
        box.className = 'sf-empty';
        box.textContent = 'The call flow didn’t load. Close and open it to try again.';
      });
  }

  // Manage › Sync history: a failed row is a <details>; toggle doesn't
  // bubble, so listen in the capture phase.
  document.addEventListener('toggle', function (ev) {
    var row = ev.target;
    if (!row || !row.matches || !row.matches('details.sh-row')) return;
    var fold = row.querySelector(':scope > .sh-flow[data-flow-src]');
    if (!fold) return;
    if (row.open) load(fold, fold.dataset.flowSrc);
    else fold.classList.remove('sf-open');
  }, true);

  function toggleCampaign(tr) {
    var fold = document.getElementById(tr.dataset.flowTarget);
    if (!fold) return;
    var open = tr.getAttribute('aria-expanded') !== 'true';
    tr.setAttribute('aria-expanded', String(open));
    if (open) load(fold, tr.dataset.flowSrc);
    else fold.classList.remove('sf-open');
  }

  document.addEventListener('click', function (ev) {
    var t = ev.target;
    if (!t.closest) return;
    var camp = t.closest('tr.sf-camp[data-flow-src]');
    if (camp) { toggleCampaign(camp); return; }
    var flow = t.closest('[data-sync-flow]');
    if (!flow) return;

    var go = t.closest('[data-go]');
    if (go) {
      ev.preventDefault();
      change(flow, function () {
        flow.dataset.selected = go.dataset.go;
        flow.dataset.hidden = '';
        flow.dataset.prob = '';
      });
      var target = rowById(flow, go.dataset.go);
      if (target) target.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
      return;
    }
    var hide = t.closest('[data-hide]');
    if (hide) {
      change(flow, function () {
        var h = words(flow.dataset.hidden), k = hide.dataset.hide, i = h.indexOf(k);
        if (i < 0) h.push(k); else h.splice(i, 1);
        flow.dataset.hidden = h.join(' ');
        var s = flow.dataset.selected && rowById(flow, flow.dataset.selected);
        if (s && h.indexOf(s.dataset.group) >= 0) flow.dataset.selected = '';
      });
      return;
    }
    if (t.closest('[data-prob]')) {
      change(flow, function () { flow.dataset.prob = flow.dataset.prob === '1' ? '' : '1'; });
      return;
    }
    if (t.closest('.sf-det')) return;
    var step = t.closest('.sf-row[data-step]');
    if (step) {
      change(flow, function () {
        flow.dataset.selected = flow.dataset.selected === step.dataset.step ? '' : step.dataset.step;
      });
    }
  });

  document.addEventListener('keydown', function (ev) {
    if (ev.key !== 'Enter' && ev.key !== ' ') return;
    var t = ev.target;
    if (!t.matches || !t.matches('.sf-row[data-step], tr.sf-camp[data-flow-src]')) return;
    ev.preventDefault();
    t.click();
  });

  // A flow rendered with the page, rather than fetched, is set up here.
  function init() { document.querySelectorAll('[data-sync-flow]').forEach(setup); }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
  document.addEventListener('htmx:afterSettle', init);
})();
