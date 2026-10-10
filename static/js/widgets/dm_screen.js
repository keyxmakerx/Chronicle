/*
 * dm_screen.js — the DM Screen opener.
 *
 * The top-bar button lives in app.templ; the panel is a server-rendered
 * fragment (GET /campaigns/:id/dm-screen) fetched fresh on every open. This
 * widget owns the overlay, the opening (a d20 rolls out of the button, lands
 * on a 20, the folded screen pushes up out of it, and the knocked-loose faces
 * fall, bounce and settle), closing, tabs and the condition filter.
 *
 * Nothing loops, so there is nothing to ease to still. One click anywhere,
 * or Escape, skips the opening. Reduced motion gets a plain fade.
 *
 * Registered via Chronicle.register; boot.js mounts it on [data-widget="dm-screen"].
 */
(function () {
  'use strict';

  // The die is ten flat facets: a hexagon outline split around the centre
  // face, in percentages of the die box, so each facet can fall on its own.
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
  var DIE = 120;      // die size in px
  var BREAK = 1560;   // ms: the screen pushes out and the die comes apart
  var FADE = BREAK + 1900;

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

  var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

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
        // Once the opening is over the screen is a window: drag it by its head.
        o.addEventListener('pointerdown', function (e) {
          if (!st.windowed || e.button > 0 || e.target.closest('button, a, input, textarea')) return;
          var head = e.target.closest('.dms-head');
          if (head) startDrag(e, head);
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
          var ta = e.target.querySelector('[data-dms-note]');
          st.keep = { tab: tab && tab.getAttribute('data-dms-tab'), draft: st.noteDirty && ta ? ta.value : null };
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
        if (list.scrollTo) list.scrollTo({ top: target, behavior: reduce ? 'auto' : 'smooth' });
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

      // Autosave: ~800ms after typing stops, and on blur or close.
      function noteEdited() {
        st.noteDirty = true;
        noteStatus('Unsaved changes');
        clearTimeout(st.noteTimer);
        st.noteTimer = setTimeout(saveNote, 800);
      }

      function saveNote() {
        clearTimeout(st.noteTimer);
        var ta = noteBox();
        if (!st.noteDirty || !ta || ta.readOnly) return;
        if (st.noteSaving) { st.noteAgain = true; return; }
        var url = ta.getAttribute('data-dms-note-url'), text = ta.value;
        st.noteSaving = true;
        noteStatus('Saving\u2026');
        Chronicle.apiFetch(url, { method: 'PUT', body: { text: text, version: ta.getAttribute('data-dms-note-version') || '' } })
          .then(function (r) {
            if (r.status === 409) { var e = new Error('changed'); e.changed = true; throw e; }
            if (!r.ok) throw new Error('save failed: ' + r.status);
            return r.json();
          })
          .then(function (j) {
            // Typing during the save keeps the box dirty for the next round.
            var cur = noteBox();
            if (cur && j) cur.setAttribute('data-dms-note-version', j.version || '');
            if (!cur || cur.value === text) { st.noteDirty = false; noteStatus(NOTE_OK); }
            var r = root(), a = r && r.querySelector('[data-dms-note-link]');
            if (a && j && j.link) { a.setAttribute('href', j.link); a.hidden = false; }
          })
          .catch(function (err) {
            if (err && err.changed) { reloadNote(url); return; }
            noteStatus('Could not save. Your text is still here; it will retry.');
          })
          .then(function () {
            st.noteSaving = false;
            if (st.noteAgain) { st.noteAgain = false; saveNote(); }
          });
      }

      // Another save or an edit in Notes got there first: take its text.
      function reloadNote(url) {
        return Chronicle.apiFetch(url)
          .then(function (r) { if (!r.ok) throw new Error('reload failed'); return r.json(); })
          .then(function (j) {
            var ta = noteBox(); if (!ta) return;
            ta.value = j.text || '';
            ta.readOnly = !!j.read_only;
            ta.setAttribute('data-dms-note-version', j.version || '');
            st.noteDirty = false;
            noteStatus('Changed elsewhere; reloaded.');
          })
          .catch(function () { noteStatus('Changed elsewhere. Close and reopen the screen to reload.'); });
      }

      function restoreKept() {
        var k = st.keep; if (!k) return;
        st.keep = null;
        var r = root(); if (!r) return;
        if (k.tab) {
          var t = r.querySelector('[data-dms-tab="' + k.tab + '"]');
          if (t) selectTab(t);
        }
        var ta = noteBox();
        if (ta && k.draft !== null) { ta.value = k.draft; noteStatus('Unsaved changes'); st.noteTimer = setTimeout(saveNote, 800); }
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
        if (st.playing) finish(); else close();
      }

      function anim(target, frames, opts) {
        opts.fill = 'both';
        if (!opts.easing) opts.easing = 'cubic-bezier(.2,.8,.2,1)';
        var a = target.animate(frames, opts);
        st.anims.push(a);
        return a;
      }

      function clearFx() {
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
      // clicks (the page behind is usable), the head bar drags the screen, and
      // the spot is remembered per browser. Narrow screens stay as they are.
      // The offset is the CSS `translate` property, which composes with the
      // opening's `transform` animations instead of fighting them.
      function enterWindow() {
        var o = st.overlay, r = root();
        if (st.windowed || !o || !r || narrow()) return;
        st.windowed = true;
        o.classList.add('dms-windowed');
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
        r.classList.toggle('dms-settle', !!settle && !reduce);
        r.style.translate = st.off.x + 'px ' + st.off.y + 'px';
      }

      // Keeps the head bar whole inside the viewport. offsetLeft/Top ignore
      // translate and transform, so the base spot is stable mid-move.
      function clampOffset(r, p) {
        var head = r.querySelector('.dms-head');
        var hh = head ? head.offsetHeight : 32;
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

      function startDrag(e, head) {
        var r = root(); if (!r) return;
        var startX = e.clientX, startY = e.clientY, from = st.off || { x: 0, y: 0 };
        st.overlay.classList.add('dms-dragging');
        r.classList.remove('dms-settle');
        try { head.setPointerCapture(e.pointerId); } catch (err) { /* older browsers: window listeners below still work */ }
        function move(ev) {
          st.off = clampOffset(r, { x: from.x + ev.clientX - startX, y: from.y + ev.clientY - startY });
          placeWindow(false);
        }
        function up() {
          head.removeEventListener('pointermove', move);
          head.removeEventListener('pointerup', up);
          head.removeEventListener('pointercancel', up);
          if (st.overlay) st.overlay.classList.remove('dms-dragging');
          writePos(st.off);
        }
        head.addEventListener('pointermove', move);
        head.addEventListener('pointerup', up);
        head.addEventListener('pointercancel', up);
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
          if (f.face) p.innerHTML = '<b>20</b>';
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
        r.querySelectorAll('.dms-head, .dms-strip').forEach(function (n) {
          anim(n, [{ opacity: 0 }, { opacity: 1 }], { duration: 250, delay: delay + 1150 });
        });
        if (lastAnim) lastAnim.onfinish = function () { setPlaying(false); };
      }

      function open() {
        var o = overlay();
        exitWindow();
        stopAll();
        st.willWindow = !narrow();
        o.hidden = false;
        document.addEventListener('keydown', onKey);
        var dim = o.querySelector('[data-dms-dim]');
        anim(dim, [{ opacity: 0 }, { opacity: 1 }], { duration: 300 });
        var loaded = load();

        if (reduce) {
          loaded.then(function (html) {
            var r = mount(html);
            if (r) { anim(r, [{ opacity: 0 }, { opacity: 1 }], { duration: 200 }); focusClose(); if (st.willWindow) enterWindow(); }
          }).catch(failed);
          return;
        }

        setPlaying(true);
        var b = openBtn.getBoundingClientRect();
        var bx = b.left + b.width / 2, by = b.top + b.height / 2;
        var cx = window.innerWidth / 2, cy = Math.min(window.innerHeight * 0.42, 360);
        var die = buildDie(cx, cy), sx = bx - cx, sy = by - cy;
        anim(die.wrap, [
          { transform: 'translate(' + sx + 'px,' + sy + 'px) rotate(-30deg) scale(.25)', opacity: 0 },
          { transform: 'translate(' + sx * 0.55 + 'px,' + (sy * 0.55 - 70) + 'px) rotate(260deg) scale(.8)', opacity: 1, offset: 0.3 },
          { transform: 'translate(0,0) rotate(520deg) scale(1)', offset: 0.58 },
          { transform: 'translate(0,-34px) rotate(610deg) scale(1)', offset: 0.7 },
          { transform: 'translate(0,0) rotate(700deg) scale(1)', offset: 0.82 },
          { transform: 'translate(0,-8px) rotate(715deg) scale(1)', offset: 0.9 },
          { transform: 'translate(0,0) rotate(720deg) scale(1)', opacity: 1 }
        ], { duration: 1150, easing: 'cubic-bezier(.3,.6,.4,1)' });
        // A beat so the 20 reads, then a small tremor as it gives way.
        anim(die.wrap, [{ translate: '0 0' }, { translate: '-2px 0' }, { translate: '2px 1px' }, { translate: '-1px 0' }, { translate: '0 0' }],
          { duration: 200, delay: 1340, easing: 'linear', composite: 'add' });

        var start = performance.now();
        shatter(die, start);
        loaded.then(function (html) {
          var r = mount(html);
          if (!r) return;
          var wait = Math.max(0, BREAK - (performance.now() - start));
          if (!st.playing) { focusClose(); if (st.willWindow) enterWindow(); return; } // skipped while loading
          unfold(r, cx, cy, wait);
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

      function close() {
        if (!st.overlay || st.overlay.hidden) return;
        saveNote();
        st.willWindow = false;
        stopAll();
        document.removeEventListener('keydown', onKey);
        var r = root(), o = st.overlay;
        var done = function () { exitWindow(); o.hidden = true; o.querySelector('[data-dms-stage]').innerHTML = ''; openBtn.focus({ preventScroll: true }); };
        if (!r || reduce) { done(); return; }
        var b = openBtn.getBoundingClientRect(), rect = r.getBoundingClientRect();
        var dx = (b.left + b.width / 2) - (rect.left + rect.width / 2), dy = (b.top + b.height / 2) - (rect.top + rect.height / 2);
        anim(o.querySelector('[data-dms-dim]'), [{ opacity: 1 }, { opacity: 0 }], { duration: 300 });
        anim(r, [{ transform: 'none', opacity: 1 }, { transform: 'translate(' + dx + 'px,' + dy + 'px) scale(.1)', opacity: 0 }],
          { duration: 380, easing: 'cubic-bezier(.5,0,.75,0)' }).onfinish = function () { stopAll(); done(); };
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
