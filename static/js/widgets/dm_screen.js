/*
 * dm_screen.js — the DM Screen opener.
 *
 * The top-bar button lives in app.templ; the panel is a server-rendered
 * fragment (GET /campaigns/:id/dm-screen) fetched fresh on every open. This
 * widget owns the overlay, the opening (a rune d20 from Chronicle.dice is
 * thrown from the button and lands on a 20, then bursts: the folded screen
 * pushes up out of it and the knocked-loose pieces fall, bounce and settle),
 * the closing (the leaves fold shut and the screen collapses where it
 * stands), tabs and the condition filter.
 *
 * Nothing loops, so there is nothing to ease to still. One click anywhere,
 * or Escape, skips the opening. Reduced motion gets a plain fade.
 *
 * Registered via Chronicle.register; boot.js mounts it on [data-widget="dm-screen"].
 */
(function () {
  'use strict';

  // The burst: the thrown die is swapped for ten flat facets, a hexagon outline
  // split around the centre face, in percentages of the die box, so each
  // facet can fall on its own.
  var H = [[50, 3], [93, 27], [93, 73], [50, 97], [7, 73], [7, 27]];
  var T = [[50, 24], [77, 67], [23, 67]];
  var FACETS = [
    { p: [T[0], T[1], T[2]], shade: 0, face: true },
    { p: [H[5], H[0], T[0]], shade: 18 }, { p: [H[0], H[1], T[0]], shade: 10 },
    { p: [H[1], T[1], T[0]], shade: -8 }, { p: [H[1], H[2], T[1]], shade: -18 },
    { p: [H[2], H[3], T[1]], shade: -24 }, { p: [H[3], T[2], T[1]], shade: -12 },
    { p: [H[3], H[4], T[2]], shade: -20 }, { p: [H[4], H[5], T[2]], shade: -4 },
    { p: [H[5], T[0], T[2]], shade: 6 }
  ];
  var DIE = 90;       // burst size in px, about the thrown die's size
  var BREAK = 1560;   // ms into the burst's clock when the pieces start to fall
  var FADE = BREAK + 1900;
  var HOLD = 450;     // ms the landed 20 stays lit before it bursts

  // Note state per campaign and note label, kept for the page's life so a draft
  // that failed to save is still there when the screen is reopened.
  var NOTES = {};

  var POS_KEY = 'chronicle.dmscreen.pos';

  function readPos() {
    try {
      var p = JSON.parse(localStorage.getItem(POS_KEY) || 'null');
      if (p && isFinite(p.x) && isFinite(p.y)) return { x: +p.x, y: +p.y };
    } catch (e) { /* storage blocked or corrupt: open at the default spot */ }
    return { x: 0, y: 0 };
  }

  function writePos(p) {
    try { localStorage.setItem(POS_KEY, JSON.stringify(p)); } catch (e) { /* not remembered */ }
  }

  // Asked at each move, so a Calm or Off pick in My view applies at once.
  function reduce() {
    return window.Chronicle && Chronicle.reducedMotion ? Chronicle.reducedMotion() :
      !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  function narrow() { return window.matchMedia('(max-width: 700px)').matches; }

  Chronicle.register('dm-screen', {
    init: function (el) {
      var openBtn = el.querySelector('[data-dms-open]');
      var url = el.getAttribute('data-url');
      if (!openBtn || !url) return;

      var st = { anims: [], fx: [], raf: 0, playing: false, overlay: null, opener: openBtn };

      function overlay() {
        if (st.overlay) return st.overlay;
        var o = document.createElement('div');
        o.className = 'dms-overlay';
        o.hidden = true;
        o.innerHTML = '<div class="dms-dim" data-dms-dim></div><div class="dms-stage" data-dms-stage></div>' +
          '<span class="dms-skip" hidden>Click anywhere to skip</span>';
        document.body.appendChild(o);
        o.addEventListener('click', function (e) {
          if (st.playing) { finish(); return; }
          if (e.target.closest('[data-dms-close]') || e.target.hasAttribute('data-dms-dim') || e.target.hasAttribute('data-dms-stage')) { close(); return; }
          var tab = e.target.closest('[data-dms-tab]');
          if (tab) selectTab(tab);
          // The downtime switch asks first; its yes button does the post.
          var ask = e.target.closest('[data-dms-ask]');
          if (ask) showConfirm(true);
          if (e.target.closest('[data-dms-cancel]')) showConfirm(false);
          var hrow = e.target.closest('[data-dms-hrow]');
          if (hrow) toggleHero(hrow);
        });
        // Once the opening is over the screen is a window: drag it by its title bar.
        o.addEventListener('pointerdown', function (e) {
          if (!st.windowed || e.button > 0 || e.target.closest('button, a, input, textarea')) return;
          var bar = e.target.closest('.dms-titlebar');
          if (bar) startDrag(e, bar);
        });
        o.addEventListener('scroll', function (e) {
          if (e.target.hasAttribute && e.target.hasAttribute('data-dms-party')) partyEdges(e.target);
        }, true);
        // A refresh after an Armory change swaps the panel in place.
        o.addEventListener('htmx:afterSettle', function () { refreshParty(); restoreKept(); if (st.windowed) placeWindow(false); });
        o.addEventListener('input', function (e) {
          if (e.target.hasAttribute('data-dms-filter')) filterConditions(e.target);
          if (e.target.hasAttribute('data-dms-note')) noteEdited(e.target);
        });
        // blur doesn't bubble; focusout does, and leaving the box saves now.
        o.addEventListener('focusout', function (e) {
          if (e.target.hasAttribute && e.target.hasAttribute('data-dms-note')) saveNote();
        });
        // A panel refetch (an Allow, a downtime switch) replaces the markup;
        // keep the open tab and any unsaved note text across it.
        o.addEventListener('htmx:beforeSwap', function (e) {
          if (!e.target.hasAttribute || !e.target.hasAttribute('data-dms-root')) return;
          var tab = e.target.querySelector('[data-dms-tab][aria-selected="true"]');
          st.keep = { tab: tab && tab.getAttribute('data-dms-tab') };
        });
        st.overlay = o;
        return o;
      }

      function showConfirm(on) {
        var box = st.overlay && st.overlay.querySelector('[data-dms-confirm]');
        if (!box) return;
        box.hidden = !on;
        if (on) { var c = box.querySelector('[data-dms-cancel]'); if (c) c.focus({ preventScroll: true }); }
      }

      function root() { return st.overlay && st.overlay.querySelector('[data-dms-root]'); }

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
        r.querySelectorAll('[data-dms-party]').forEach(partyEdges);
      }

      function selectTab(tab) {
        var r = root(); if (!r) return;
        var name = tab.getAttribute('data-dms-tab');
        r.querySelectorAll('[data-dms-tab]').forEach(function (t) {
          t.setAttribute('aria-selected', t === tab ? 'true' : 'false');
        });
        r.querySelectorAll('[data-dms-pane]').forEach(function (p) {
          p.hidden = p.getAttribute('data-dms-pane') !== name;
        });
      }

      var NOTE_OK = 'Saved \u00b7 only you and co-DMs can see this';

      function noteBox() { var r = root(); return r && r.querySelector('[data-dms-note]'); }

      function noteStatus(text) {
        var r = root(), el = r && r.querySelector('[data-dms-note-status]');
        if (el) el.textContent = text;
      }

      // The note's state lives outside the markup: a panel refetch, a close
      // during a save, or a failed save must never lose typed text. noteFor
      // adopts the box the first time it is seen and, on later mounts, pushes
      // an unsaved draft back into the fresh markup (with the version it was
      // typed against, so a save still notices that the note moved on).
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

      // The state of the box on screen, or the last one used when the markup is
      // already gone (closing). The box is the truth for the text while it exists: anything in it that
      // the state does not hold yet is an unsaved edit.
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

      // Autosave: ~800ms after typing stops, and on blur or close.
      function noteEdited() {
        var n = curNote();
        if (!n) return;
        n.dirty = true;
        if (n.conflict) return; // the choice comes first; the text is kept
        noteStatus('Unsaved changes');
        clearTimeout(st.noteTimer);
        st.noteTimer = setTimeout(saveNote, 800);
      }

      // Sends the note's state, not the box: close() has already torn the box
      // down by the time a queued or retried save runs.
      function saveNote() {
        clearTimeout(st.noteTimer);
        clearTimeout(st.noteRetry);
        var ta = noteBox(), n = curNote();
        if (!n || !n.dirty || n.conflict || (ta && ta.readOnly)) return;
        if (n.saving) { n.again = true; return; }
        var text = n.text;
        n.saving = true;
        noteStatus('Saving\u2026');
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
              noteStatus('Couldn\u2019t save. Your text is kept; retrying in a few seconds.');
              st.noteRetry = setTimeout(saveNote, 5000);
            } else if (retry) {
              noteStatus('Couldn\u2019t save. Your text is kept; it saves on your next edit.');
            } else {
              noteStatus('Couldn\u2019t save this note. Your text is kept here.');
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
          .catch(function () { noteStatus('Couldn\u2019t load the newer text. Your text is kept here.'); });
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
          .catch(function () { noteStatus('Couldn\u2019t reach the note. Your text is kept here.'); });
      }

      function restoreKept() {
        var k = st.keep; st.keep = null;
        var r = root(); if (!r) return;
        if (k && k.tab) {
          var t = r.querySelector('[data-dms-tab="' + k.tab + '"]');
          if (t) selectTab(t);
        }
        // A draft left by a failed save (or a refetch) comes back with the
        // fresh markup.
        var n = curNote(true);
        if (n && n.dirty) {
          if (n.conflict) showConflict(true);
          else { noteStatus('Unsaved changes'); st.noteTimer = setTimeout(saveNote, 800); }
        }
      }

      function filterConditions(input) {
        var q = input.value.trim().toLowerCase();
        var r = root(); if (!r) return;
        r.querySelectorAll('[data-dms-cond]').forEach(function (d) {
          d.hidden = q !== '' && d.getAttribute('data-dms-cond').toLowerCase().indexOf(q) < 0;
        });
      }

      function onKey(e) {
        if (e.key !== 'Escape' || !st.overlay || st.overlay.hidden) return;
        if (st.playing) { finish(); return; }
        // A window sits beside a usable page: Escape closes it only when focus
        // is in it (or nowhere), so it never swallows Escape meant for a page
        // dropdown or dialog.
        if (st.windowed) {
          var f = document.activeElement;
          if (e.defaultPrevented || (f && f !== document.body && !st.overlay.contains(f))) return;
        }
        close();
      }

      function anim(target, frames, opts) {
        opts.fill = 'both';
        if (!opts.easing) opts.easing = 'cubic-bezier(.2,.8,.2,1)';
        var a = target.animate(frames, opts);
        st.anims.push(a);
        return a;
      }

      function clearFx() {
        if (st.thrown) { st.thrown = false; if (Chronicle.dice) Chronicle.dice.stop(); }
        if (st.land) st.land(null);
        if (st.raf) { cancelAnimationFrame(st.raf); st.raf = 0; }
        st.fx.forEach(function (n) { n.remove(); });
        st.fx = [];
      }

      function stopAll() {
        clearFx();
        st.anims.forEach(function (a) { try { a.cancel(); } catch (e) { /* already gone */ } });
        st.anims = [];
        setPlaying(false);
      }

      // Skip: jump every animation to its end and clear the falling pieces.
      function finish() {
        clearFx();
        st.anims.forEach(function (a) { try { a.finish(); } catch (e) { /* already gone */ } });
        setPlaying(false);
      }

      function setPlaying(on) {
        st.playing = on;
        if (st.overlay) st.overlay.querySelector('.dms-skip').hidden = !on;
        if (!on) {
          refreshParty();
          if (st.willWindow && root()) enterWindow();
        }
      }

      // The window: after the opening the dim fades away and stops catching
      // clicks (the page behind is usable), the title bar drags the screen, and
      // the spot is remembered per browser. Narrow screens stay as they are.
      // The offset is the CSS `translate` property, which composes with the
      // opening's `transform` animations instead of fighting them.
      function enterWindow() {
        var o = st.overlay, r = root();
        if (st.windowed || !o || !r || narrow()) return;
        st.windowed = true;
        o.classList.add('dms-windowed');
        // The page behind is live now, so the dialog is no longer modal.
        r.setAttribute('aria-modal', 'false');
        var dim = o.querySelector('[data-dms-dim]');
        var fade = anim(dim, [{ opacity: 1 }, { opacity: 0 }], { duration: 300 });
        fade.onfinish = function () { o.classList.add('dms-dimmed-off'); };
        st.off = clampOffset(r, readPos());
        placeWindow(true);
        window.addEventListener('resize', onResize);
      }

      function exitWindow() {
        var o = st.overlay;
        window.removeEventListener('resize', onResize);
        st.windowed = false;
        st.willWindow = false;
        if (!o) return;
        o.classList.remove('dms-windowed', 'dms-dimmed-off', 'dms-dragging');
        var r = root();
        if (r) r.style.translate = '';
      }

      // Puts the root at the stored offset; settle eases it there once.
      function placeWindow(settle) {
        var r = root(); if (!r || !st.off) return;
        if (st.windowed) r.setAttribute('aria-modal', 'false'); // a refetch restores the template's value
        r.classList.toggle('dms-settle', !!settle && !reduce());
        r.style.translate = st.off.x + 'px ' + st.off.y + 'px';
      }

      // Keeps the title bar whole inside the viewport. offsetLeft/Top ignore
      // translate and transform, so the base spot is stable mid-move.
      function clampOffset(r, p) {
        var bar = r.querySelector('.dms-titlebar');
        var hh = bar ? bar.offsetHeight : 32;
        var minX = -r.offsetLeft, maxX = Math.max(minX, window.innerWidth - r.offsetWidth - r.offsetLeft);
        var minY = -r.offsetTop, maxY = Math.max(minY, window.innerHeight - hh - r.offsetTop);
        return { x: Math.min(maxX, Math.max(minX, p.x)), y: Math.min(maxY, Math.max(minY, p.y)) };
      }

      function onResize() {
        var r = root(); if (!r || !st.windowed) return;
        if (narrow()) { exitWindow(); st.willWindow = false; return; }
        st.off = clampOffset(r, st.off || { x: 0, y: 0 });
        placeWindow(false);
      }

      function startDrag(e, bar) {
        var r = root(); if (!r) return;
        var startX = e.clientX, startY = e.clientY, from = st.off || { x: 0, y: 0 };
        st.overlay.classList.add('dms-dragging');
        r.classList.remove('dms-settle');
        try { bar.setPointerCapture(e.pointerId); } catch (err) { /* older browsers: window listeners below still work */ }
        function move(ev) {
          st.off = clampOffset(r, { x: from.x + ev.clientX - startX, y: from.y + ev.clientY - startY });
          placeWindow(false);
        }
        function up() {
          bar.removeEventListener('pointermove', move);
          bar.removeEventListener('pointerup', up);
          bar.removeEventListener('pointercancel', up);
          if (st.overlay) st.overlay.classList.remove('dms-dragging');
          writePos(st.off);
        }
        bar.addEventListener('pointermove', move);
        bar.addEventListener('pointerup', up);
        bar.addEventListener('pointercancel', up);
        e.preventDefault();
      }

      function load() {
        return Chronicle.apiFetch(url, { headers: { 'Accept': 'text/html', 'HX-Request': 'true' } })
          .then(function (r) {
            if (!r.ok) throw new Error('DM Screen request failed: ' + r.status);
            return r.text();
          });
      }

      function mount(html) {
        var stage = st.overlay.querySelector('[data-dms-stage]');
        stage.innerHTML = html;
        if (window.htmx) window.htmx.process(stage);
        restoreKept();
        return root();
      }

      function buildDie(x, y) {
        var wrap = document.createElement('div');
        wrap.className = 'dms-die';
        wrap.style.left = (x - DIE / 2) + 'px';
        wrap.style.top = (y - DIE / 2) + 'px';
        var pieces = FACETS.map(function (f) {
          var p = document.createElement('div');
          p.className = 'dms-piece';
          p.style.clipPath = 'polygon(' + f.p.map(function (v) { return v[0] + '% ' + v[1] + '%'; }).join(',') + ')';
          p.style.filter = 'brightness(' + (100 + f.shade) + '%)';
          var cx = (f.p[0][0] + f.p[1][0] + f.p[2][0]) / 3, cy = (f.p[0][1] + f.p[1][1] + f.p[2][1]) / 3;
          p.style.transformOrigin = cx + '% ' + cy + '%';
          wrap.appendChild(p);
          return { el: p, cx: cx, cy: cy, face: !!f.face };
        });
        st.overlay.appendChild(wrap);
        st.fx.push(wrap);
        return { wrap: wrap, pieces: pieces };
      }

      // The faces are knocked loose by the rising screen, fall under gravity,
      // bounce on the bottom of the window, slide to rest, then fade.
      function shatter(die, start) {
        var W = window.innerWidth, Hh = window.innerHeight;
        var wl = parseFloat(die.wrap.style.left), wt = parseFloat(die.wrap.style.top);
        var bodies = die.pieces.map(function (p) {
          var px = p.cx / 100 * DIE, py = p.cy / 100 * DIE, r = p.face ? 16 : 11;
          return {
            el: p.el, x: 0, y: 0, a: 0,
            vx: (p.cx - 50) * 9 + (Math.random() - 0.5) * 90,
            vy: (p.cy - 50) * 7 - 420 - Math.random() * 260,
            va: (Math.random() < 0.5 ? -1 : 1) * (240 + Math.random() * 420),
            floor: Hh - 6 - r - (wt + py),
            xmin: -(wl + px) + r, xmax: W - (wl + px) - r
          };
        });
        var last = start;
        function step(now) {
          var t = now - start, dt = Math.min(0.033, (now - last) / 1000);
          last = now;
          if (t >= BREAK) {
            bodies.forEach(function (b) {
              b.vy += 2400 * dt; b.x += b.vx * dt; b.y += b.vy * dt; b.a += b.va * dt;
              if (b.y > b.floor) {
                b.y = b.floor;
                if (b.vy > 90) { b.vy = -b.vy * 0.42; b.vx *= 0.7; b.va = -b.va * 0.5 + b.vx * 0.6; }
                else { b.vy = 0; b.vx *= 0.86; b.va *= 0.8; }
              }
              if (b.x < b.xmin) { b.x = b.xmin; b.vx = -b.vx * 0.5; }
              if (b.x > b.xmax) { b.x = b.xmax; b.vx = -b.vx * 0.5; }
              b.el.style.transform = 'translate(' + b.x.toFixed(1) + 'px,' + b.y.toFixed(1) + 'px) rotate(' + b.a.toFixed(1) + 'deg)';
              b.el.style.opacity = t < FADE ? 1 : Math.max(0, 1 - (t - FADE) / 700);
            });
          }
          if (t < FADE + 700) st.raf = requestAnimationFrame(step);
          else clearFx();
        }
        st.raf = requestAnimationFrame(step);
      }

      // The screen rises out of where the die stood, stands, and swings open.
      function unfold(r, fromX, fromY, delay) {
        var rect = r.getBoundingClientRect();
        var dx = fromX - (rect.left + rect.width / 2), dy = fromY - (rect.top + rect.height / 2);
        var side = narrow() ? 'rotateX' : 'rotateY';
        var L = r.querySelector('[data-dms-leaf="l"]'), M = r.querySelector('[data-dms-leaf="m"]'), R = r.querySelector('[data-dms-leaf="r"]');
        anim(r, [
          { transform: 'translate(' + dx + 'px,' + dy + 'px) scale(.1)', opacity: 0 },
          { transform: 'translate(' + dx * 0.6 + 'px,' + dy * 0.6 + 'px) scale(.22)', opacity: 1, offset: 0.2 },
          { transform: 'translate(' + dx * 0.3 + 'px,' + dy * 0.3 + 'px) scale(.6)', opacity: 1, offset: 0.55 },
          { transform: 'none', opacity: 1 }
        ], { duration: 950, delay: delay, easing: 'cubic-bezier(.3,.5,.3,1)' });
        if (M) anim(M, [{ transform: 'rotateX(-80deg)' }, { transform: 'rotateX(-80deg)', offset: 0.15 }, { transform: 'rotateX(6deg)', offset: 0.8 }, { transform: 'none' }], { duration: 800, delay: delay });
        if (L) anim(L, [{ transform: side + '(' + (narrow() ? '-' : '') + '178deg)' }, { transform: side + '(' + (narrow() ? '8' : '-8') + 'deg)', offset: 0.75 }, { transform: 'none' }], { duration: 620, delay: delay + 650, easing: 'cubic-bezier(.3,.7,.3,1)' });
        var lastAnim = R ? anim(R, [{ transform: side + '(' + (narrow() ? '' : '-') + '178deg)' }, { transform: side + '(' + (narrow() ? '-8' : '8') + 'deg)', offset: 0.75 }, { transform: 'none' }], { duration: 620, delay: delay + 730, easing: 'cubic-bezier(.3,.7,.3,1)' }) : null;
        r.querySelectorAll('.dms-titlebar').forEach(function (n) {
          anim(n, [{ opacity: 0 }, { opacity: 1 }], { duration: 250, delay: delay + 1150 });
        });
        if (lastAnim) lastAnim.onfinish = function () { setPlaying(false); };
      }

      // Throws a rune d20 from the button and resolves with where it rests,
      // once the 20 has been lit for a beat. Anything that keeps the die from
      // being drawn (motion Calm, no WebGL, the dice files late) resolves at
      // once with no point, and the screen comes out of the middle instead.
      function throwDie() {
        return new Promise(function (resolve) {
          var done = false;
          st.land = function (p) {
            if (done) return;
            done = true; st.land = null;
            resolve(p ? { x: p.x, y: p.y } : null);
          };
          var dice = Chronicle.dice;
          if (!dice || !dice.roll) { st.land(null); return; }
          st.thrown = true;
          dice.roll([{ label: 'DM Screen', dice: [{ sides: 20, value: 20, crit: 'none' }] }], {
            from: openBtn,
            skippable: false,
            // The opening is a show, not a roll: nothing to read out.
            describe: function () { return ''; },
            onSettle: function (points) {
              var p = points && points[0];
              if (!p) { st.land(null); return; }
              setTimeout(function () { if (st.land) st.land(p); }, HOLD);
            }
          }).catch(function () { if (st.land) st.land(null); });
        });
      }

      // A flash of light where the die stood, under the flying pieces.
      function flash(x, y) {
        var f = document.createElement('div');
        f.className = 'dms-flash';
        f.style.left = x + 'px';
        f.style.top = y + 'px';
        st.overlay.appendChild(f);
        st.fx.push(f);
        anim(f, [{ transform: 'translate(-50%,-50%) scale(.2)', opacity: 1 }, { transform: 'translate(-50%,-50%) scale(2.6)', opacity: 0 }],
          { duration: 520, easing: 'cubic-bezier(.2,.8,.3,1)' });
      }

      function open() {
        var o = overlay();
        exitWindow();
        stopAll();
        st.closing = false;
        st.willWindow = !narrow();
        o.hidden = false;
        document.addEventListener('keydown', onKey);
        var dim = o.querySelector('[data-dms-dim]');
        anim(dim, [{ opacity: 0 }, { opacity: 1 }], { duration: 300 });
        var loaded = load();
        var me = st.seq = (st.seq || 0) + 1;

        if (reduce()) {
          loaded.then(function (html) {
            if (me !== st.seq) return;
            var r = mount(html);
            if (r) { anim(r, [{ opacity: 0 }, { opacity: 1 }], { duration: 200 }); focusClose(); if (st.willWindow) enterWindow(); }
          }).catch(failed);
          return;
        }

        setPlaying(true);
        var cx = window.innerWidth / 2, cy = Math.min(window.innerHeight * 0.42, 360);
        Promise.all([loaded, throwDie()]).then(function (got) {
          // Closed, or opened again, while the die was in the air.
          if (me !== st.seq || o.hidden) return;
          var r = mount(got[0]);
          if (!r) return;
          if (!st.playing) { focusClose(); if (st.willWindow) enterWindow(); return; } // skipped
          var p = got[1] || { x: cx, y: cy };
          // The thrown die gives way to its flat pieces in the same spot, and
          // they fly apart as the screen pushes up out of them.
          st.thrown = false;
          if (Chronicle.dice) Chronicle.dice.stop();
          flash(p.x, p.y);
          var now = performance.now();
          shatter(buildDie(p.x, p.y), now - BREAK);
          unfold(r, p.x, p.y, 0);
          focusClose();
        }).catch(failed);
      }

      function focusClose() {
        var c = st.overlay && st.overlay.querySelector('[data-dms-close]');
        if (c) c.focus({ preventScroll: true });
      }

      function failed() {
        stopAll();
        close();
        if (Chronicle.notify) Chronicle.notify('The DM Screen could not load. Try again in a moment.', 'error');
      }

      // Closing is the opening backwards, in place: the side leaves swing
      // shut over the middle one, the middle folds down, and the folded screen
      // collapses into a flash where it stands.
      function close() {
        if (!st.overlay || st.overlay.hidden || st.closing) return;
        // saveNote captures the box's text into the note state before the box
        // is torn down below; a save already in flight queues this one behind it.
        saveNote();
        st.willWindow = false;
        st.seq = (st.seq || 0) + 1;
        stopAll();
        document.removeEventListener('keydown', onKey);
        var r = root(), o = st.overlay;
        var done = function () { exitWindow(); o.hidden = true; o.querySelector('[data-dms-stage]').innerHTML = ''; openBtn.focus({ preventScroll: true }); };
        if (!r || reduce()) { done(); return; }
        st.closing = true;
        var side = narrow() ? 'rotateX' : 'rotateY';
        var L = r.querySelector('[data-dms-leaf="l"]'), M = r.querySelector('[data-dms-leaf="m"]'), R = r.querySelector('[data-dms-leaf="r"]');
        var shut = 'cubic-bezier(.5,0,.6,1)';
        if (!o.classList.contains('dms-dimmed-off')) anim(o.querySelector('[data-dms-dim]'), [{ opacity: 1 }, { opacity: 0 }], { duration: 300, delay: 420 });
        r.querySelectorAll('.dms-titlebar').forEach(function (n) { anim(n, [{ opacity: 1 }, { opacity: 0 }], { duration: 160 }); });
        if (R) anim(R, [{ transform: 'none' }, { transform: side + '(' + (narrow() ? '' : '-') + '178deg)' }], { duration: 340, easing: shut });
        if (L) anim(L, [{ transform: 'none' }, { transform: side + '(' + (narrow() ? '-' : '') + '178deg)' }], { duration: 340, delay: 70, easing: shut });
        if (M) anim(M, [{ transform: 'none' }, { transform: 'rotateX(-80deg)' }], { duration: 260, delay: 330, easing: shut });
        anim(r, [{ transform: 'none', opacity: 1 }, { transform: 'scale(.9)', opacity: 1, offset: 0.45 }, { transform: 'scale(.06)', opacity: 0 }],
          { duration: 360, delay: 420, easing: 'cubic-bezier(.55,0,.8,.3)' }).onfinish = function () {
          var rect = r.getBoundingClientRect();
          stopAll();
          st.closing = false;
          done();
          // The flash outlives the screen by a moment, on the page itself.
          var f = document.createElement('div');
          f.className = 'dms-flash dms-flash-page';
          f.style.left = (rect.left + rect.width / 2) + 'px';
          f.style.top = (rect.top + rect.height / 2) + 'px';
          document.body.appendChild(f);
          var a = f.animate([{ transform: 'translate(-50%,-50%) scale(.3)', opacity: .9 }, { transform: 'translate(-50%,-50%) scale(1.6)', opacity: 0 }],
            { duration: 380, easing: 'cubic-bezier(.2,.8,.3,1)', fill: 'both' });
          a.onfinish = function () { f.remove(); };
        };
      }

      openBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        open();
      });
      st.close = close;
      el._dmsState = st;
    },
    destroy: function (el) {
      // The overlay lives on <body>, outside the top bar; boosted navigation
      // keeps the top bar, so this only runs on a full teardown.
      if (el && el._dmsState && el._dmsState.close) el._dmsState.close();
    }
  });
})();
