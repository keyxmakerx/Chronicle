/**
 * customize_shell.js: Customize's frame (customize_shell.templ).
 *
 * Customize keeps ONE draft for all its areas: a JSON object of named
 * sections ("look", "categories", ...). The server keeps it until the owner
 * presses Save or Discard (GET/PUT/DELETE /campaigns/:id/customize/draft),
 * so leaving the page loses nothing. Save asks the server to apply every
 * section (POST .../customize/publish); each section is applied by the
 * plugin that owns its data, and a section that fails stays in the draft.
 *
 * Areas talk to the frame through the page, not through each other:
 *   - An area changes its section with Chronicle.customizeSet(key, value)
 *     (value null removes the section). A value may carry `_changes`, a list
 *     of { label, href } the save bar lists; the server ignores `_`-keys.
 *   - The frame announces every change it did not get from that call (Undo,
 *     Redo, Discard, Save) as a `chronicle:customize-draft` event on
 *     document, detail { reason, draft, keys, applied }. Areas redraw from
 *     detail.draft.
 *   - The current draft is always on the frame's data-draft attribute, so an
 *     area can read it whenever it mounts (Chronicle.customizeDraft()).
 *   - Chronicle.customizeFlush() resolves once the draft is kept, before an
 *     area opens a page that reads it.
 *   - An area with its own Undo and Redo buttons (the Customizer) calls
 *     Chronicle.customizeUndo / customizeRedo and reads customizeHistory();
 *     customizeSet(key, value, { step: true }) makes that change its own Undo
 *     step rather than joining a quick run of edits to the same section.
 */
(function () {
  'use strict';

  var EVENT = 'chronicle:customize-draft';
  var SAVE_DELAY = 700;      // ms of quiet before the draft is sent
  var COALESCE = 1200;       // edits to one section this close share one Undo step
  var MAX_UNDO = 100;

  function parse(s, dflt) {
    try { var v = JSON.parse(s || ''); return v == null ? dflt : v; } catch (e) { return dflt; }
  }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function typing(t) {
    if (!t || !t.closest) return false;
    return !!t.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"]');
  }

  // Shared with the areas: read the draft, and change a section of it. An
  // area may load before the frame's script; its changes wait in a queue
  // the frame drains when it mounts.
  // Areas whose script loaded first queue their changes here.
  Chronicle._customizeQueue = Chronicle._customizeQueue || [];
  var active = null;
  Chronicle.customizeDraft = function () {
    if (active) return active.read();
    var el = document.querySelector('[data-widget="customize-shell"]');
    return el ? parse(el.getAttribute('data-draft'), {}) : {};
  };
  Chronicle.customizeSet = function (key, value, opts) {
    if (active) active.set(key, value, opts);
    else Chronicle._customizeQueue.push([key, value, opts]);
  };
  Chronicle.customizeUndo = function () { if (active) active.undo(); };
  Chronicle.customizeRedo = function () { if (active) active.redo(); };
  Chronicle.customizeHistory = function () {
    return active ? active.history() : { undo: false, redo: false };
  };
  // customizeToast shows a note in the frame's toast stack; undoFn adds an
  // Undo button.
  Chronicle.customizeToast = function (msg, bad, undoFn) { if (active) active.toast(msg, bad, undoFn); };
  // customizeFlush resolves once the draft as it stands is kept on the
  // server, for an area about to open a page that reads it.
  Chronicle.customizeFlush = function () {
    return active ? active.flush() : Promise.resolve(true);
  };

  function mount(el) {
    var CID = el.getAttribute('data-campaign-id');
    var CSRF = el.getAttribute('data-csrf');
    var BASE = '/campaigns/' + encodeURIComponent(CID) + '/customize';
    var areas = parse(el.getAttribute('data-areas'), []);
    var finds = parse(el.getAttribute('data-find'), []);
    var draft = parse(el.getAttribute('data-draft'), {});
    var version = Number(el.getAttribute('data-version')) || 0;
    var past = [], future = [];
    var lastKey = null, lastAt = 0;
    var rev = 0, sentRev = 0, saveT = 0, inflight = null, retryT = 0;
    var problem = null;        // 'conflict' | 'offline' | null
    var publishing = false;
    var failed = [];           // [{ key, message }] from the last Save
    var alive = true;
    var listeners = [];

    function on(t, type, fn, o) { t.addEventListener(type, fn, o); listeners.push([t, type, fn, o]); }
    function $(sel) { return el.querySelector(sel); }
    function $$(sel) { return Array.prototype.slice.call(el.querySelectorAll(sel)); }
    function snapshot() { return JSON.stringify(draft); }
    function sections() { return Object.keys(draft).filter(function (k) { return k.charAt(0) !== '_'; }); }
    function areaFor(key) { return areas.filter(function (a) { return a.section === key; })[0]; }

    /* ---------- Changing the draft ---------- */
    function announce(reason, keys, extra) {
      el.setAttribute('data-draft', snapshot());
      var detail = { reason: reason, draft: parse(snapshot(), {}), keys: keys || [] };
      if (extra) Object.keys(extra).forEach(function (k) { detail[k] = extra[k]; });
      document.dispatchEvent(new CustomEvent(EVENT, { detail: detail }));
      paint();
    }
    function changedKeys(a, b) {
      var A = parse(a, {}), B = parse(b, {}), out = [];
      Object.keys(A).concat(Object.keys(B)).forEach(function (k) {
        if (out.indexOf(k) < 0 && JSON.stringify(A[k]) !== JSON.stringify(B[k])) out.push(k);
      });
      return out;
    }
    function set(key, value, opts) {
      if (!alive || typeof key !== 'string' || !key) return;
      var before = snapshot();
      var empty = value == null || (typeof value === 'object' && !Object.keys(value).some(function (k) { return k.charAt(0) !== '_'; }));
      if (empty) delete draft[key];
      else draft[key] = parse(JSON.stringify(value), null);
      if (snapshot() === before) return;
      var now = Date.now();
      var step = !!(opts && opts.step);
      if (step || !(key === lastKey && now - lastAt < COALESCE && past.length)) {
        past.push(before);
        if (past.length > MAX_UNDO) past.shift();
      }
      future = [];
      lastKey = key; lastAt = now;
      failed = failed.filter(function (f) { return f.key !== key; });
      el.setAttribute('data-draft', snapshot());
      changed();
      paint();
    }
    function restore(to, reason) {
      var before = snapshot();
      draft = parse(to, {});
      lastKey = null;
      announce(reason, changedKeys(before, to));
      changed();
    }
    function undo() {
      if (!past.length || publishing) return;
      future.push(snapshot());
      restore(past.pop(), 'undo');
    }
    function redo() {
      if (!future.length || publishing) return;
      past.push(snapshot());
      restore(future.pop(), 'redo');
    }

    /* ---------- Keeping the draft on the server ---------- */
    function changed() {
      rev++;
      Chronicle.markDirty('customize-draft');
      clearTimeout(saveT);
      saveT = setTimeout(function () { send(false); }, SAVE_DELAY);
    }
    function api(method, path, body, keepalive) {
      var opts = { method: method, csrfToken: CSRF };
      if (body !== undefined) opts.body = body;
      if (keepalive) opts.keepalive = true;
      return Chronicle.apiFetch(BASE + path, opts).then(function (res) {
        return res.json().then(function (b) { return { ok: res.ok, status: res.status, body: b }; },
          function () { return { ok: res.ok, status: res.status, body: null }; });
      });
    }
    // send keeps the server's copy in step; it resolves once what was
    // current when it was called is stored (or has failed).
    function send(keepalive) {
      clearTimeout(saveT);
      if (problem === 'conflict') return Promise.resolve(false);
      if (inflight) return inflight.then(function () { return rev === sentRev ? true : send(keepalive); });
      if (rev === sentRev) return Promise.resolve(true);
      var mine = rev, body = { version: version, draft: parse(snapshot(), {}) };
      inflight = api('PUT', '/draft', body, keepalive).then(function (r) {
        inflight = null;
        if (r.ok) {
          version = (r.body && typeof r.body.version === 'number') ? r.body.version : version;
          el.setAttribute('data-version', String(version));
          sentRev = mine;
          if (problem === 'offline') problem = null;
          if (rev === sentRev) Chronicle.markClean('customize-draft');
          if (alive) paint();
          return true;
        }
        if (r.status === 409) { problem = 'conflict'; if (alive) paint(); return false; }
        return offline();
      }, function () { inflight = null; return offline(); });
      return inflight;
    }
    function offline() {
      problem = 'offline';
      if (alive) {
        paint();
        clearTimeout(retryT);
        retryT = setTimeout(function () { send(false); }, 5000);
      }
      return false;
    }

    /* ---------- Save and Discard ---------- */
    function labels() {
      var out = [];
      sections().forEach(function (k) {
        var sec = draft[k] || {}, ch = Array.isArray(sec._changes) ? sec._changes : [];
        if (ch.length) ch.forEach(function (c) { if (c && c.label) out.push({ key: k, label: String(c.label), href: c.href || '' }); });
        else {
          var a = areaFor(k);
          out.push({ key: k, label: a ? a.label : k, href: a ? a.href : '' });
        }
      });
      return out;
    }
    function publish() {
      if (publishing || problem === 'conflict' || !sections().length) return;
      publishing = true; paint();
      send(false).then(function (ok) {
        if (!ok) { publishing = false; paint(); toast('Not saved. ' + problemText(), true); return null; }
        return api('POST', '/publish', { version: version });
      }).then(function (r) {
        if (!r) return;
        publishing = false;
        if (!r.ok) {
          if (r.status === 409) problem = 'conflict';
          paint();
          toast('Not saved. ' + ((r.body && (r.body.message || r.body.error)) || problemText()), true);
          return;
        }
        var b = r.body || {};
        draft = b.draft && typeof b.draft === 'object' ? b.draft : {};
        version = typeof b.version === 'number' ? b.version : 0;
        el.setAttribute('data-version', String(version));
        rev = sentRev = 0;
        past = []; future = []; lastKey = null;
        failed = Array.isArray(b.failed) ? b.failed : [];
        Chronicle.markClean('customize-draft');
        announce('publish', b.applied || [], { applied: b.applied || [], failed: failed });
        if (failed.length) {
          var a = areaFor(failed[0].key);
          toast('Some changes were not saved. ' + (a ? a.label + ': ' : '') + failed[0].message, true);
        } else {
          toast('Saved. Everyone in the campaign now sees your changes.');
        }
      }, function () {
        publishing = false; paint();
        toast('Not saved. Chronicle could not be reached; your draft is kept.', true);
      });
    }
    function discard() {
      if (publishing || !sections().length) return;
      var kept = snapshot();
      clearTimeout(saveT);
      var go = inflight ? inflight.then(function () {}) : Promise.resolve();
      go.then(function () {
        return api('DELETE', '/draft?version=' + encodeURIComponent(version));
      }).then(function (r) {
        if (!r.ok && r.status !== 404) {
          if (r.status === 409) { problem = 'conflict'; paint(); }
          toast('Not thrown away. ' + problemText(), true);
          return;
        }
        version = 0; rev = sentRev = 0; failed = [];
        el.setAttribute('data-version', '0');
        past = []; future = [];
        Chronicle.markClean('customize-draft');
        restoreQuiet({}, 'discard');
        toast('Changes thrown away.', false, function () {
          // Bring the draft back as a new one.
          version = 0;
          restoreQuiet(parse(kept, {}), 'restore');
          rev++; send(false);
        });
      }, function () { toast('Not thrown away. Chronicle could not be reached.', true); });
    }
    function restoreQuiet(to, reason) {
      var before = snapshot();
      draft = to;
      lastKey = null;
      announce(reason, changedKeys(before, snapshot()));
    }
    function problemText() {
      if (problem === 'conflict') return 'This draft was changed in another tab or window. Reload to see the latest.';
      if (problem === 'offline') return 'Chronicle could not be reached. Your changes are kept on this page and sent again shortly.';
      return 'Try again.';
    }

    /* ---------- The save bar, the rail and toasts ---------- */
    var bar = $('[data-czs-savebar]');
    function paint() {
      var secs = sections();
      $$('[data-czs-area]').forEach(function (a) {
        var area = areas.filter(function (x) { return x.id === a.getAttribute('data-czs-area'); })[0];
        var dot = a.querySelector('.czs-dot');
        if (dot) dot.hidden = !(area && area.section && secs.indexOf(area.section) >= 0);
      });
      var dotsElsewhere = document.querySelectorAll('[data-czs-area-dot]');
      Array.prototype.forEach.call(dotsElsewhere, function (d) {
        var area = areas.filter(function (x) { return x.id === d.getAttribute('data-czs-area-dot'); })[0];
        d.hidden = !(area && area.section && secs.indexOf(area.section) >= 0);
      });
      var show = secs.length > 0 || publishing || problem === 'conflict';
      bar.hidden = !show;
      if (!show) return;
      var ls = labels(), n = ls.length;
      var what = $('[data-czs-what]'), sdot = bar.querySelector('.czs-sdot');
      sdot.classList.toggle('is-bad', !!problem || failed.length > 0);
      if (problem === 'conflict') what.innerHTML = '<b>Draft changed elsewhere.</b> Reload to see it.';
      else if (publishing) what.innerHTML = '<span class="czs-spin" aria-hidden="true"></span> <b>Saving…</b>';
      else {
        what.innerHTML = '<b>' + n + (n === 1 ? ' change' : ' changes') + '</b> waiting' +
          (problem === 'offline' ? ', not yet kept' : '') + ': ' + esc(ls.map(function (l) { return l.label; }).join(', '));
      }
      $('[data-czs-undo]').disabled = !past.length || publishing;
      $('[data-czs-redo]').disabled = !future.length || publishing;
      var dis = $('[data-czs-discard]'), save = $('[data-czs-save]');
      dis.disabled = publishing || !secs.length;
      save.disabled = publishing || problem === 'conflict' || !secs.length;
      save.textContent = problem === 'conflict' ? 'Reload' : (publishing ? 'Saving…' : 'Save changes');
      if (problem === 'conflict') { save.disabled = false; save.setAttribute('data-reload', ''); }
      else save.removeAttribute('data-reload');
    }
    var toasts = $('[data-czs-toasts]');
    function toast(msg, bad, undoFn) {
      if (!toasts) return;
      var t = document.createElement('div');
      t.className = 'czs-toast' + (bad ? ' is-bad' : '');
      t.innerHTML = '<span></span>' + (undoFn ? '<button type="button">Undo</button>' : '') +
        (bad ? '<button type="button" aria-label="Dismiss">Close</button>' : '');
      t.firstChild.textContent = msg;
      var btns = t.querySelectorAll('button');
      if (undoFn) btns[0].addEventListener('click', function () { undoFn(); t.remove(); });
      if (bad) btns[btns.length - 1].addEventListener('click', function () { t.remove(); });
      toasts.appendChild(t);
      // Errors stay until dismissed; everything else leaves on its own.
      if (!bad) setTimeout(function () { t.remove(); }, undoFn ? 6500 : 3200);
    }

    /* ---------- What is waiting ---------- */
    var pop = null;
    function closePop() {
      if (!pop) return;
      pop.remove(); pop = null;
      document.removeEventListener('pointerdown', popOff, true);
    }
    function popOff(e) { if (pop && !pop.contains(e.target) && !e.target.closest('[data-czs-what]')) closePop(); }
    function openChanges(anchor) {
      if (pop) { closePop(); return; }
      var ls = labels();
      pop = document.createElement('div');
      pop.className = 'czs-pop';
      pop.setAttribute('role', 'dialog');
      pop.setAttribute('aria-label', 'Changes waiting');
      var fail = {};
      failed.forEach(function (f) { fail[f.key] = f.message; });
      pop.innerHTML = '<div class="czs-changes"><h3>Waiting to be saved</h3>' + ls.map(function (l) {
        return '<a href="' + esc(l.href || '#') + '"><i class="fa-solid fa-circle-dot" aria-hidden="true"></i><span>' + esc(l.label) +
          (fail[l.key] ? '<span class="czs-fail">Not saved: ' + esc(fail[l.key]) + '</span>' : '') + '</span></a>';
      }).join('') + '</div>';
      document.body.appendChild(pop);
      var r = anchor.getBoundingClientRect();
      pop.style.left = Math.max(12, Math.min(r.left, innerWidth - pop.offsetWidth - 12)) + 'px';
      pop.style.top = Math.max(12, r.top - pop.offsetHeight - 10) + 'px';
      pop.style.transformOrigin = 'bottom left';
      setTimeout(function () { document.addEventListener('pointerdown', popOff, true); }, 0);
      var first = pop.querySelector('a');
      if (first) first.focus();
      pop.addEventListener('keydown', function (e) { if (e.key === 'Escape') { closePop(); anchor.focus(); } });
    }

    /* ---------- Find a setting ---------- */
    var palette = null;
    function words(s) { return String(s || '').toLowerCase(); }
    function openFind() {
      if (palette) return;
      var opener = document.activeElement;
      palette = document.createElement('div');
      palette.className = 'czs-scrim';
      palette.innerHTML = '<div class="czs-palette" role="dialog" aria-modal="true" aria-label="Find a setting">' +
        '<div class="czs-q"><i class="fa-solid fa-magnifying-glass" aria-hidden="true"></i>' +
        '<input type="text" placeholder="Find a setting, like “font” or “Places”" aria-label="Find a setting" autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true" aria-controls="czs-find-res"><kbd>Esc</kbd></div>' +
        '<div class="czs-res" id="czs-find-res" role="listbox"></div></div>';
      document.body.appendChild(palette);
      var input = palette.querySelector('input'), res = palette.querySelector('.czs-res'), sel = 0, shown = [];
      function draw() {
        var q = words(input.value).trim().split(/\s+/).filter(Boolean);
        // A hit in the label ranks above one only in the path or the
        // section's other words, so "colour" puts Colours first.
        shown = finds.map(function (f, i) {
          var hay = words(f.label + ' ' + f.path + ' ' + (f.words || ''));
          if (!q.every(function (w) { return hay.indexOf(w) >= 0; })) return null;
          var lab = words(f.label), rank = 2;
          if (q.length && q.every(function (w) { return lab.indexOf(w) >= 0; })) rank = lab.indexOf(q[0]) === 0 ? 0 : 1;
          return { f: f, rank: rank, i: i };
        }).filter(Boolean).sort(function (a, b) {
          return a.rank - b.rank || a.i - b.i;
        }).slice(0, 40).map(function (r) { return r.f; });
        sel = Math.min(sel, Math.max(0, shown.length - 1));
        res.innerHTML = shown.length ? shown.map(function (f, i) {
          var icon = String(f.icon || 'fa-sliders').replace(/^fa-solid\s+/, '');
          return '<a href="' + esc(f.href) + '" role="option" id="czs-f-' + i + '" aria-selected="' + (i === sel) + '">' +
            '<span class="czs-ic"><i class="fa-solid ' + esc(icon) + '" aria-hidden="true"></i></span>' +
            '<span><span class="czs-t">' + esc(f.label) + '</span><span class="czs-p">' + esc(f.path) + '</span></span></a>';
        }).join('') : '<p class="czs-none">Nothing matches. Try another word.</p>';
        input.setAttribute('aria-activedescendant', shown.length ? 'czs-f-' + sel : '');
      }
      function close() {
        if (!palette) return;
        palette.remove(); palette = null;
        if (opener && opener.focus) opener.focus();
      }
      function go(f) {
        close();
        var here = location.pathname, to = f.href.split('#');
        if (to[0] === here && to[1]) { location.hash = to[1]; return; }
        location.href = f.href;
      }
      input.addEventListener('input', function () { sel = 0; draw(); });
      input.addEventListener('keydown', function (e) {
        if (e.key === 'ArrowDown') { e.preventDefault(); sel = Math.min(sel + 1, shown.length - 1); draw(); scrollSel(); }
        else if (e.key === 'ArrowUp') { e.preventDefault(); sel = Math.max(sel - 1, 0); draw(); scrollSel(); }
        else if (e.key === 'Enter') { e.preventDefault(); if (shown[sel]) go(shown[sel]); }
      });
      function scrollSel() { var a = res.querySelector('[aria-selected="true"]'); if (a) a.scrollIntoView({ block: 'nearest' }); }
      res.addEventListener('click', function (e) {
        var a = e.target.closest('a');
        if (!a) return;
        e.preventDefault();
        var i = Number(a.id.replace('czs-f-', ''));
        if (shown[i]) go(shown[i]);
      });
      palette.addEventListener('pointerdown', function (e) { if (e.target === palette) close(); });
      palette.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(); }
        if (e.key === 'Tab') { e.preventDefault(); input.focus(); }
      });
      draw();
      input.focus();
    }

    /* ---------- Wiring ---------- */
    on(bar, 'click', function (e) {
      var b = e.target.closest('button');
      if (!b) return;
      if (b.hasAttribute('data-czs-undo')) undo();
      else if (b.hasAttribute('data-czs-redo')) redo();
      else if (b.hasAttribute('data-czs-discard')) discard();
      else if (b.hasAttribute('data-czs-save')) { if (b.hasAttribute('data-reload')) location.reload(); else publish(); }
      else if (b.hasAttribute('data-czs-what')) openChanges(b);
    });
    on($('[data-czs-find]'), 'click', openFind);
    on(document, 'keydown', function (e) {
      if (e.defaultPrevented || palette) return;
      var mod = e.ctrlKey || e.metaKey;
      if (!mod && !e.altKey && e.key === '/' && !typing(e.target)) { e.preventDefault(); openFind(); return; }
      if (!mod || typing(e.target)) return;
      var k = e.key.toLowerCase();
      if (k === 'z' && !e.shiftKey) { if (past.length) { e.preventDefault(); undo(); } }
      else if ((k === 'z' && e.shiftKey) || k === 'y') { if (future.length) { e.preventDefault(); redo(); } }
    });
    // Ctrl+S on a page with nothing else to save (keyboard_shortcuts.js).
    on(window, 'chronicle:save', publish);
    on(window, 'pagehide', function () { if (rev !== sentRev) send(true); });

    var api_ = {
      read: function () { return parse(snapshot(), {}); }, set: set, flush: function () { return send(false); },
      undo: undo, redo: redo, toast: toast,
      history: function () { return { undo: past.length > 0 && !publishing, redo: future.length > 0 && !publishing }; }
    };
    active = api_;
    Chronicle._customizeQueue.splice(0).forEach(function (q) { set(q[0], q[1], q[2]); });
    paint();
    if (location.hash === '#find') openFind();

    return function destroy() {
      alive = false;
      if (active === api_) active = null;
      // Whatever has not reached the server goes now, even as the page
      // changes under it.
      if (rev !== sentRev && problem !== 'conflict') send(true);
      clearTimeout(saveT); clearTimeout(retryT);
      closePop();
      if (palette) { palette.remove(); palette = null; }
      listeners.forEach(function (l) { l[0].removeEventListener(l[1], l[2], l[3]); });
    };
  }

  Chronicle.register('customize-shell', {
    init: function (el) { el._czsDestroy = mount(el); },
    destroy: function (el) {
      if (el._czsDestroy) el._czsDestroy();
      el._czsDestroy = null;
    }
  });
})();
