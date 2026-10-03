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
        });
        o.addEventListener('input', function (e) {
          if (e.target.hasAttribute('data-dms-filter')) filterConditions(e.target);
        });
        st.overlay = o;
        return o;
      }

      function root() { return st.overlay && st.overlay.querySelector('[data-dms-root]'); }

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
        stopAll();
        o.hidden = false;
        document.addEventListener('keydown', onKey);
        var dim = o.querySelector('[data-dms-dim]');
        anim(dim, [{ opacity: 0 }, { opacity: 1 }], { duration: 300 });
        var loaded = load();

        if (reduce) {
          loaded.then(function (html) {
            var r = mount(html);
            if (r) { anim(r, [{ opacity: 0 }, { opacity: 1 }], { duration: 200 }); focusClose(); }
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
          if (!st.playing) { focusClose(); return; } // skipped while loading
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
        stopAll();
        document.removeEventListener('keydown', onKey);
        var r = root(), o = st.overlay;
        var done = function () { o.hidden = true; o.querySelector('[data-dms-stage]').innerHTML = ''; openBtn.focus({ preventScroll: true }); };
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
