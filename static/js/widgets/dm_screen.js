/*
 * dm_screen.js — the DM Screen dock.
 *
 * The top-bar button lives in app.templ; the panel is a server-rendered
 * fragment (GET /campaigns/:id/dm-screen) that refetches itself after an
 * Armory change. This widget docks it at the right edge of the page: a rail
 * of section icons and a column of section cards beside it. Several cards
 * can be open at once; the newest is shown whole and the older ones fold to
 * one line. The rail's top icon tucks the dock away to a small edge tab that
 * carries the waiting count.
 *
 * Whether the dock is out, and which sections are open in which order, is
 * remembered per browser and per campaign. The dock lives on <body>, so a
 * boosted navigation (which swaps only #main-content) leaves it standing.
 *
 * Registered via Chronicle.register; boot.js mounts it on [data-widget="dm-screen"].
 */
(function () {
  'use strict';
  function gmTerm() { return (window.Chronicle && Chronicle.gmTerm) ? Chronicle.gmTerm() : 'GM'; }

  var UNDO_FOR = 10000; // ms a revealed row keeps its Undo
  var DEFAULT_OPEN = ['world', 'party'];

  // A revealed row's Undo lasts a few seconds, long enough to catch a
  // misclick at the table. Focus on it moves to the row's confirmation.
  document.addEventListener('htmx:load', function (e) {
    var root = e.detail && e.detail.elt;
    if (!root || !root.querySelectorAll) return;
    var undos = root.matches && root.matches('[data-dms-undo]') ? [root] : root.querySelectorAll('[data-dms-undo]');
    Array.prototype.forEach.call(undos, function (btn) {
      setTimeout(function () {
        if (!btn.isConnected) return;
        var done = btn.parentNode && btn.parentNode.querySelector('[data-dms-done]');
        var had = document.activeElement === btn;
        btn.remove();
        if (had && done) done.focus();
      }, UNDO_FOR);
    });
  });

  // Note state per campaign and note label, kept for the page's life so a draft
  // that failed to save is still there after a refetch.
  var NOTES = {};

  // Asked at each move, so a Calm or Off pick in My view applies at once.
  function reduce() {
    return window.Chronicle && Chronicle.reducedMotion ? Chronicle.reducedMotion() :
      !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  // On a narrow window the dock lies over the page instead of narrowing it.
  function narrow() { return window.matchMedia('(max-width: 700px)').matches; }

  function readJSON(key, fallback) {
    try {
      var v = JSON.parse(localStorage.getItem(key) || 'null');
      return v === null ? fallback : v;
    } catch (e) { return fallback; } // storage blocked or corrupt
  }

  function writeJSON(key, v) {
    try { localStorage.setItem(key, JSON.stringify(v)); } catch (e) { /* not remembered */ }
  }

  function standardMs() {
    var v = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--dur-standard'));
    return isFinite(v) && v > 0 ? v : 200;
  }

  Chronicle.register('dm-screen', {
    init: function (el) {
      var toggleBtn = el.querySelector('[data-dms-toggle]');
      var url = el.getAttribute('data-url');
      if (!toggleBtn || !url) return;

      var cid = (url.match(/\/campaigns\/([^/]+)\//) || [])[1] || '';
      var OUT_KEY = 'chronicle.dms.out.' + cid;
      var OPEN_KEY = 'chronicle.dms.open.' + cid;

      var savedOpen = readJSON(OPEN_KEY, DEFAULT_OPEN);
      var st = {
        out: readJSON(OUT_KEY, false) === true,
        open: Array.isArray(savedOpen) ? savedOpen.filter(function (s) { return typeof s === 'string'; }) : DEFAULT_OPEN.slice(),
        dock: null, tab: null, loading: null, loaded: false, brewPv: false
      };

      function root() { return st.dock && st.dock.querySelector('[data-dms-root]'); }

      // The dock starts under the top bar, whatever sits above it.
      function place() {
        var bar = document.getElementById('app-topbar');
        var top = bar ? Math.max(0, Math.round(bar.getBoundingClientRect().bottom)) : 56;
        document.documentElement.style.setProperty('--dms-top', top + 'px');
      }

      function build() {
        var d = document.createElement('div');
        d.className = 'dms-dock';
        d.hidden = true;
        d.innerHTML = '<div class="dms-stage" data-dms-stage></div>';
        var t = document.createElement('button');
        t.type = 'button';
        t.className = 'dms-tab';
        t.hidden = true;
        t.setAttribute('aria-label', 'Show the ' + gmTerm() + ' Screen');
        t.title = gmTerm() + ' Screen';
        t.innerHTML = '<i class="fa-solid fa-angles-left" aria-hidden="true"></i><span class="dms-tab-n" hidden></span>';
        t.addEventListener('click', function () { show(true); });
        document.body.appendChild(d);
        document.body.appendChild(t);
        st.dock = d;
        st.tab = t;

        d.addEventListener('click', onClick);
        d.addEventListener('scroll', function (e) {
          if (e.target.hasAttribute && e.target.hasAttribute('data-dms-party')) partyEdges(e.target);
        }, true);
        d.addEventListener('input', function (e) {
          if (e.target.hasAttribute('data-dms-filter')) filterConditions(e.target);
          if (e.target.hasAttribute('data-dms-note')) noteEdited();
        });
        // blur doesn't bubble; focusout does, and leaving the box saves now.
        d.addEventListener('focusout', function (e) {
          if (e.target.hasAttribute && e.target.hasAttribute('data-dms-note')) saveNote();
        });
        // A refetch (an Allow, a downtime switch) replaces the panel; the open
        // sections, the player view and any unsaved note come back on it.
        d.addEventListener('htmx:afterSettle', function (e) {
          if (e.target && e.target.hasAttribute && e.target.hasAttribute('data-dms-root')) afterMount();
        });
        document.addEventListener('keydown', onKey);
        document.addEventListener('click', onOutside);
        window.addEventListener('resize', place);
        place();
      }

      function onClick(e) {
        var t = e.target;
        if (t.closest('[data-dms-tuck]')) { tuck(true); return; }
        var rail = t.closest('[data-dms-open]');
        if (rail) { railPick(rail.getAttribute('data-dms-open')); return; }
        var fold = t.closest('[data-dms-fold]');
        if (fold) { headerPick(fold.closest('[data-dms-sec]').getAttribute('data-dms-sec')); return; }
        var shut = t.closest('[data-dms-shut]');
        if (shut) { closeSection(shut.closest('[data-dms-sec]').getAttribute('data-dms-sec')); return; }
        if (t.closest('[data-dms-closeall]')) { st.open = []; saveOpen(); applyOpen(); return; }
        // The downtime switch asks first; its yes button does the post.
        if (t.closest('[data-dms-ask]')) showConfirm(true);
        if (t.closest('[data-dms-cancel]')) showConfirm(false);
        var hrow = t.closest('[data-dms-hrow]');
        if (hrow) toggleHero(hrow);
        if (t.closest('[data-dms-brew-pv]')) { st.brewPv = !st.brewPv; applyBrew(); }
      }

      // ---- Sections ----------------------------------------------------

      function saveOpen() { writeJSON(OPEN_KEY, st.open); }

      // A rail icon opens its section as the newest, brings a folded one to
      // the front, or closes the one already in front.
      function railPick(id) {
        var i = st.open.indexOf(id);
        if (i === st.open.length - 1 && i >= 0) st.open.pop();
        else {
          if (i >= 0) st.open.splice(i, 1);
          st.open.push(id);
        }
        saveOpen();
        applyOpen(id);
      }

      // A folded card's header brings it to the front; the front card's
      // header folds it behind the others.
      function headerPick(id) {
        var i = st.open.indexOf(id);
        if (i < 0) return;
        st.open.splice(i, 1);
        if (i === st.open.length) st.open.unshift(id); // was the front card
        else st.open.push(id);
        saveOpen();
        applyOpen(st.open[st.open.length - 1]);
      }

      function closeSection(id) {
        var i = st.open.indexOf(id);
        if (i < 0) return;
        st.open.splice(i, 1);
        saveOpen();
        applyOpen();
        // Focus goes back to the section's rail icon rather than to nowhere.
        var r = root(), ic = r && r.querySelector('[data-dms-open="' + id + '"]');
        if (ic) ic.focus({ preventScroll: true });
      }

      // Draws st.open onto the markup: open cards shown, the newest whole and
      // first in the column, the rest folded to their one-line summary.
      function applyOpen(focusId) {
        var r = root(); if (!r) return;
        var have = {};
        r.querySelectorAll('[data-dms-sec]').forEach(function (s) { have[s.getAttribute('data-dms-sec')] = true; });
        // A section can leave the screen (no fight source any more); forget it.
        st.open = st.open.filter(function (id, i, a) { return have[id] && a.indexOf(id) === i; });
        var n = st.open.length;
        r.querySelectorAll('[data-dms-sec]').forEach(function (s) {
          var id = s.getAttribute('data-dms-sec'), i = st.open.indexOf(id);
          var front = i === n - 1 && i >= 0;
          s.hidden = i < 0;
          s.classList.toggle('dms-folded', i >= 0 && !front);
          s.style.order = i < 0 ? '' : String(n - i);
          var fold = s.querySelector('[data-dms-fold]');
          if (fold) fold.setAttribute('aria-expanded', front ? 'true' : 'false');
          var body = s.querySelector('.dms-sec-b');
          if (body) body.hidden = !front;
          // A card's widgets (Fight's tracker, the Homebrew lists) mount when
          // it is first shown, since a mount here doesn't run boot.js's pass.
          if (front && window.Chronicle && Chronicle.mountWidgets) Chronicle.mountWidgets(s);
        });
        r.querySelectorAll('[data-dms-open]').forEach(function (b) {
          b.setAttribute('aria-pressed', st.open.indexOf(b.getAttribute('data-dms-open')) >= 0 ? 'true' : 'false');
        });
        var count = r.querySelector('[data-dms-count]');
        if (count) count.textContent = n ? n + ' open' : 'Nothing open';
        var all = r.querySelector('[data-dms-closeall]');
        if (all) all.hidden = n === 0;
        var empty = r.querySelector('[data-dms-empty]');
        if (empty) empty.hidden = n > 0;
        if (focusId) {
          var cards = r.querySelector('[data-dms-cards]');
          if (cards) cards.scrollTop = 0;
        }
        refreshParty();
      }

      // ---- Out and tucked ----------------------------------------------

      function waiting() {
        var r = root(), b = r && r.querySelector('[data-dms-open="world"] .dms-badge');
        return b ? parseInt(b.textContent, 10) || 0 : 0;
      }

      function paintTab() {
        if (!st.tab) return;
        var n = waiting(), badge = st.tab.querySelector('.dms-tab-n');
        badge.hidden = n === 0;
        badge.textContent = n;
        st.tab.setAttribute('aria-label', 'Show the ' + gmTerm() + ' Screen' +
          (n ? ', ' + n + (n === 1 ? ' request' : ' requests') + ' waiting' : ''));
        st.tab.hidden = st.out;
      }

      function setDocked(on) {
        document.documentElement.classList.toggle('dms-docked', on);
        toggleBtn.setAttribute('aria-expanded', on ? 'true' : 'false');
      }

      function show(focus) {
        if (!st.dock) build();
        st.out = true;
        writeJSON(OUT_KEY, true);
        st.dock.hidden = false;
        setDocked(true);
        paintTab();
        if (!reduce()) {
          st.dock.animate([{ transform: 'translateX(100%)' }, { transform: 'none' }],
            { duration: standardMs(), easing: 'cubic-bezier(.2,.8,.2,1)' });
        }
        ensure(true).then(function () {
          if (!focus) return;
          var t = root() && root().querySelector('[data-dms-tuck]');
          if (t) t.focus({ preventScroll: true });
        });
      }

      function tuck(focus) {
        if (!st.dock || st.dock.hidden) return;
        saveNote();
        var had = st.dock.contains(document.activeElement);
        st.out = false;
        writeJSON(OUT_KEY, false);
        setDocked(false);
        var d = st.dock;
        var done = function () {
          if (st.out) return; // brought back out mid-slide
          d.hidden = true;
          paintTab();
          if (focus || had) (narrow() ? toggleBtn : st.tab).focus({ preventScroll: true });
        };
        if (reduce()) { done(); return; }
        d.animate([{ transform: 'none' }, { transform: 'translateX(100%)' }],
          { duration: standardMs(), easing: 'cubic-bezier(.4,0,1,1)' }).onfinish = done;
      }

      // Escape tucks the dock only when focus is in it, so it never swallows
      // Escape meant for a page dropdown or dialog.
      function onKey(e) {
        if (e.key !== 'Escape' || e.defaultPrevented || !st.out || !st.dock) return;
        if (!st.dock.contains(document.activeElement)) return;
        var confirm = st.dock.querySelector('[data-dms-confirm]:not([hidden])');
        if (confirm) { showConfirm(false); return; }
        tuck(true);
      }

      // On a narrow window the dock covers the page, so a tap outside it
      // tucks it away. Beside the page it stays until tucked.
      function onOutside(e) {
        if (!st.out || !st.dock || !narrow()) return;
        if (st.dock.contains(e.target) || el.contains(e.target) || (st.tab && st.tab.contains(e.target))) return;
        // The Adventures page sheet sits over the dock and belongs to it.
        if (e.target.closest && e.target.closest('.dms-sheet')) return;
        tuck(false);
      }

      // ---- Loading -----------------------------------------------------

      function load() {
        return Chronicle.apiFetch(url, { headers: { 'Accept': 'text/html', 'HX-Request': 'true' } })
          .then(function (r) {
            if (!r.ok) throw new Error('DM Screen request failed: ' + r.status);
            return r.text();
          });
      }

      // ensure fetches the panel once; asked marks a load the viewer is
      // waiting on, whose failure is worth telling them about.
      function ensure(asked) {
        if (st.loaded) return Promise.resolve();
        if (!st.loading) {
          st.loading = load().then(mount).catch(function (err) {
            st.loading = null;
            throw err;
          });
        }
        return st.loading.catch(function () {
          if (asked && Chronicle.notify) Chronicle.notify('The ' + gmTerm() + ' Screen could not load. Try again in a moment.', 'error');
        });
      }

      // clearStage tears the panel down, letting its widgets finish first
      // (the Fight card saves what was typed last).
      function clearStage(stage) {
        if (window.Chronicle && Chronicle.destroyWidget) stage.querySelectorAll('[data-widget]').forEach(Chronicle.destroyWidget);
        stage.innerHTML = '';
      }

      function mount(html) {
        var stage = st.dock.querySelector('[data-dms-stage]');
        clearStage(stage);
        stage.innerHTML = html;
        if (window.htmx) window.htmx.process(stage);
        st.loaded = true;
        afterMount();
      }

      function afterMount() {
        applyOpen();
        applyBrew();
        paintTab();
        // A draft left by a failed save (or a refetch) comes back with the
        // fresh markup.
        var n = curNote(true);
        if (n && n.dirty) {
          if (n.conflict) showConflict(true);
          else { noteStatus('Unsaved changes'); clearTimeout(st.noteTimer); st.noteTimer = setTimeout(saveNote, 800); }
        }
      }

      // ---- Section contents --------------------------------------------

      function showConfirm(on) {
        var box = st.dock && st.dock.querySelector('[data-dms-confirm]');
        if (!box) return;
        box.hidden = !on;
        if (on) { var c = box.querySelector('[data-dms-cancel]'); if (c) c.focus({ preventScroll: true }); }
      }

      // Heroes start folded to one line; a click folds the rest open.
      function toggleHero(row) {
        var hero = row.closest('[data-dms-hero]');
        var open = !hero.classList.contains('dms-open');
        hero.classList.toggle('dms-open', open);
        row.setAttribute('aria-expanded', open ? 'true' : 'false');
        setTimeout(function () {
          var list = hero.closest('[data-dms-party]');
          if (!list) return;
          fitOpenHeroes(list);
          if (open) showWhole(list, hero);
          partyEdges(list);
        }, 340);
      }

      // An open hero is always shown whole: the list grows past its usual
      // height when one hero alone wouldn't fit, and the others move aside.
      function fitOpenHeroes(list) {
        list.style.maxHeight = '';
        var cap = list.clientHeight, need = 0;
        list.querySelectorAll('[data-dms-hero].dms-open').forEach(function (h) {
          need = Math.max(need, h.offsetHeight + 8);
        });
        if (need > cap) list.style.maxHeight = need + 'px';
      }

      function showWhole(list, hero) {
        var top = hero.offsetTop, bottom = top + hero.offsetHeight;
        var target = list.scrollTop;
        if (bottom > list.scrollTop + list.clientHeight) target = bottom - list.clientHeight + 4;
        if (top < target) target = Math.max(0, top - 4);
        if (target === list.scrollTop) return;
        if (list.scrollTo) list.scrollTo({ top: target, behavior: reduce() ? 'auto' : 'smooth' });
        else list.scrollTop = target;
      }

      // Fades the list edge and counts the heroes still below it.
      function partyEdges(list) {
        var more = list.parentNode.querySelector('[data-dms-more]');
        var end = list.scrollTop + list.clientHeight >= list.scrollHeight - 2;
        list.classList.toggle('dms-at-end', end);
        list.classList.toggle('dms-scrolled', list.scrollTop > 2);
        var below = 0, bottom = list.getBoundingClientRect().bottom;
        list.querySelectorAll('[data-dms-hero]').forEach(function (h) {
          if (h.getBoundingClientRect().top > bottom - 12) below++;
        });
        if (!more) return;
        more.hidden = end || !below;
        more.textContent = below + ' more below';
      }

      function refreshParty() {
        var r = root(); if (!r) return;
        r.querySelectorAll('[data-dms-party]').forEach(function (list) {
          if (list.offsetParent) partyEdges(list);
        });
      }

      function filterConditions(input) {
        var q = input.value.trim().toLowerCase();
        var r = root(); if (!r) return;
        r.querySelectorAll('[data-dms-cond]').forEach(function (d) {
          d.hidden = q !== '' && d.getAttribute('data-dms-cond').toLowerCase().indexOf(q) < 0;
        });
      }

      // The Homebrew card's player view: what players would see of it. Kept
      // on st so a refetch finds it as it was left.
      function applyBrew() {
        var r = root(), pane = r && r.querySelector('[data-dms-brew-pane]');
        if (!pane) return;
        var pvb = pane.querySelector('[data-dms-brew-pv]');
        if (pvb) pvb.setAttribute('aria-pressed', st.brewPv ? 'true' : 'false');
        if (st.brewPv) pane.setAttribute('data-player-view', ''); else pane.removeAttribute('data-player-view');
      }

      // ---- Notes -------------------------------------------------------

      var NOTE_OK = 'Saved · only you and co-' + gmTerm() + 's can see this';

      function noteBox() { var r = root(); return r && r.querySelector('[data-dms-note]'); }

      function noteStatus(text) {
        var r = root(), s = r && r.querySelector('[data-dms-note-status]');
        if (s) s.textContent = text;
      }

      // The note's state lives outside the markup: a panel refetch or a failed
      // save must never lose typed text. noteFor adopts the box the first time
      // it is seen and, on later mounts, pushes an unsaved draft back into the
      // fresh markup (with the version it was typed against, so a save still
      // notices that the note moved on).
      function noteKey(ta) { return ta.getAttribute('data-dms-note-url') + '|' + (ta.getAttribute('aria-label') || ''); }
      function onScreen(n, ta) { return !!ta && noteKey(ta) === n.key; }

      function noteFor(ta, restore) {
        var key = noteKey(ta);
        var n = NOTES[key];
        if (!n) n = NOTES[key] = { key: key, dirty: false, saving: false, again: false, conflict: false, failures: 0 };
        n.url = ta.getAttribute('data-dms-note-url');
        if (restore && n.dirty && !ta.readOnly) {
          ta.value = n.text;
          ta.setAttribute('data-dms-note-version', n.version);
        } else if (!n.dirty) {
          n.text = ta.value;
          n.version = ta.getAttribute('data-dms-note-version') || '';
        }
        return n;
      }

      // The state of the box on screen, or the last one used. The box is the
      // truth for the text while it exists: anything in it that the state
      // does not hold yet is an unsaved edit.
      function curNote(restore) {
        var ta = noteBox();
        if (ta) {
          st.note = noteFor(ta, restore);
          if (!restore && !ta.readOnly && st.note.text !== ta.value) { st.note.text = ta.value; st.note.dirty = true; }
        }
        return st.note || null;
      }

      function conflictBox() {
        var r = root(), s = r && r.querySelector('[data-dms-note-status]');
        if (!s) return null;
        var box = r.querySelector('[data-dms-note-conflict]');
        if (!box) {
          box = document.createElement('span');
          box.setAttribute('data-dms-note-conflict', '');
          box.hidden = true;
          [['Use theirs', useTheirs], ['Keep mine', keepMine]].forEach(function (b) {
            var btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'dms-link';
            btn.textContent = b[0];
            btn.addEventListener('click', b[1]);
            box.appendChild(btn);
          });
          s.parentNode.insertBefore(box, s.nextSibling);
        }
        return box;
      }

      function showConflict(on) {
        var box = conflictBox();
        if (box) box.hidden = !on;
        if (on) noteStatus('Changed elsewhere. Your text is kept here.');
      }

      // Autosave: ~800ms after typing stops, and on blur or tuck.
      function noteEdited() {
        var n = curNote();
        if (!n) return;
        n.dirty = true;
        if (n.conflict) return; // the choice comes first; the text is kept
        noteStatus('Unsaved changes');
        clearTimeout(st.noteTimer);
        st.noteTimer = setTimeout(saveNote, 800);
      }

      // Sends the note's state, not the box: a refetch may have replaced the
      // box by the time a queued or retried save runs.
      function saveNote() {
        clearTimeout(st.noteTimer);
        clearTimeout(st.noteRetry);
        var ta = noteBox(), n = curNote();
        if (!n || !n.dirty || n.conflict || (ta && ta.readOnly)) return;
        if (n.saving) { n.again = true; return; }
        var text = n.text;
        n.saving = true;
        noteStatus('Saving…');
        Chronicle.apiFetch(n.url, { method: 'PUT', body: { text: text, version: n.version } })
          .then(function (r) {
            if (r.status === 409) { var e = new Error('changed'); e.changed = true; throw e; }
            if (!r.ok) { var f = new Error('save failed: ' + r.status); f.status = r.status; throw f; }
            return r.json();
          })
          .then(function (j) {
            n.failures = 0;
            if (j) n.version = j.version || '';
            // Typing during the save keeps the note dirty for the next round.
            if (n.text === text) n.dirty = false;
            var box = noteBox();
            if (box && onScreen(n, box)) {
              box.setAttribute('data-dms-note-version', n.version);
              noteStatus(n.dirty ? 'Unsaved changes' : NOTE_OK);
              var r = root(), a = r && r.querySelector('[data-dms-note-link]');
              if (a && j && j.link) { a.setAttribute('href', j.link); a.hidden = false; }
            }
          })
          .catch(function (err) {
            if (err && err.changed) {
              n.conflict = true;
              var box = noteBox();
              if (box && onScreen(n, box)) showConflict(true);
              return;
            }
            // A 4xx other than a conflict (too long, formatted note) will not
            // pass by trying again; anything else gets one more go in 5 s and
            // then waits for the next edit.
            var retry = !(err && err.status >= 400 && err.status < 500);
            n.failures++;
            if (retry && n.failures < 2) {
              noteStatus('Couldn’t save. Your text is kept; retrying in a few seconds.');
              st.noteRetry = setTimeout(saveNote, 5000);
            } else if (retry) {
              noteStatus('Couldn’t save. Your text is kept; it saves on your next edit.');
            } else {
              noteStatus('Couldn’t save this note. Your text is kept here.');
            }
          })
          .then(function () {
            n.saving = false;
            if (n.again) { n.again = false; saveNote(); }
          });
      }

      // The note moved on (another save, an edit in Notes, a new game night, or
      // it was deleted). Typed text is never replaced without a choice.
      function useTheirs() {
        var n = st.note; if (!n) return;
        Chronicle.apiFetch(n.url)
          .then(function (r) { if (!r.ok) throw new Error('reload failed'); return r.json(); })
          .then(function (j) {
            n.text = j.text || '';
            n.version = j.version || '';
            n.dirty = false;
            n.conflict = false;
            n.failures = 0;
            var ta = noteBox();
            if (ta && onScreen(n, ta)) {
              ta.value = n.text;
              ta.readOnly = !!j.read_only;
              ta.setAttribute('data-dms-note-version', n.version);
              showConflict(false);
              noteStatus('Loaded the newer text.');
            }
          })
          .catch(function () { noteStatus('Couldn’t load the newer text. Your text is kept here.'); });
      }

      // Re-saves the draft over the note as it is now: fetch its version, then
      // save with that.
      function keepMine() {
        var n = st.note; if (!n) return;
        Chronicle.apiFetch(n.url)
          .then(function (r) { if (!r.ok) throw new Error('reload failed'); return r.json(); })
          .then(function (j) {
            n.version = j.version || '';
            n.conflict = false;
            n.dirty = true;
            var ta = noteBox();
            if (ta && onScreen(n, ta)) {
              ta.setAttribute('data-dms-note-version', n.version);
              ta.readOnly = !!j.read_only;
              showConflict(false);
            }
            saveNote();
          })
          .catch(function () { noteStatus('Couldn’t reach the note. Your text is kept here.'); });
      }

      // ---- Start -------------------------------------------------------

      toggleBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        if (st.out) tuck(true); else show(true);
      });

      build();
      setDocked(false);
      if (st.out) {
        // Out last time: back out at once, without the slide, on a wide
        // window. A narrow window starts tucked so the page is readable.
        if (narrow()) { st.out = false; paintTab(); ensure(false); }
        else { st.dock.hidden = false; setDocked(true); ensure(false); }
      } else {
        // Tucked: the panel still loads so the tab can show the waiting count.
        paintTab();
        ensure(false);
      }

      st.onKey = onKey;
      st.onOutside = onOutside;
      st.place = place;
      el._dmsState = st;
    },
    destroy: function (el) {
      // The dock lives on <body>, outside the top bar; boosted navigation
      // keeps the top bar, so this only runs on a full teardown.
      var st = el && el._dmsState;
      if (!st || !st.dock) return;
      document.removeEventListener('keydown', st.onKey);
      document.removeEventListener('click', st.onOutside);
      window.removeEventListener('resize', st.place);
      if (window.Chronicle && Chronicle.destroyWidget) st.dock.querySelectorAll('[data-widget]').forEach(Chronicle.destroyWidget);
      st.dock.remove();
      if (st.tab) st.tab.remove();
      document.documentElement.classList.remove('dms-docked');
    }
  });
})();
