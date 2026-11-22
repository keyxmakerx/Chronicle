/*
 * dm_screen.js — the DM Screen dock.
 *
 * The top-bar button lives in app.templ; the panel is a server-rendered
 * fragment (GET /campaigns/:id/dm-screen) that refetches itself after an
 * Armory change. This widget docks it at the right edge of the page: a rail
 * of section icons and a column of section cards beside it. Several cards
 * can be open at once, each whole or folded to one line on its own, and a
 * card's grip (or its title, with a mouse) drags it within or between the
 * column and the leaves. The rail's top icon tucks the dock away to a small edge tab that
 * carries the waiting count.
 *
 * A rail icon dragged out toward the page opens its section on a leaf of
 * its own: the leaf swings open off the column's outer edge and hovers over
 * the page, two leaves at most beside the column. A leaf's bar merges it back
 * into the column or closes it, and dragging the bar onto the column or
 * another leaf merges it there.
 *
 * On a phone or touch screen there is no rail: all sections are always on
 * screen as one named line each (icon, name, what it holds), and a tap
 * unfolds or folds that line in place.
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

  // Phones and touch screens show named lines instead of the icon bar; the
  // same query drives the CSS.
  function phone() {
    return window.matchMedia('(max-width: 700px), (hover: none) and (pointer: coarse)').matches;
  }

  // The lines a phone starts with unfolded.
  var DEFAULT_LINES = ['world'];

  function readJSON(key, fallback) {
    try {
      var v = JSON.parse(localStorage.getItem(key) || 'null');
      return v === null ? fallback : v;
    } catch (e) { return fallback; } // storage blocked or corrupt
  }

  function writeJSON(key, v) {
    try { localStorage.setItem(key, JSON.stringify(v)); } catch (e) { /* not remembered */ }
  }

  var LEAF_W = 264;   // a leaf's width, the column's less its rail-side edge
  var MAX_LEAVES = 2; // beside the column, so three panels in all

  // A stored leaf list from storage, kept only if it is the shape we wrote.
  function cleanLeaves(v) {
    if (!Array.isArray(v)) return [];
    return v.filter(Array.isArray).map(function (l) {
      return l.filter(function (x) { return typeof x === 'string'; });
    }).filter(function (l) { return l.length; }).slice(0, MAX_LEAVES);
  }

  function cleanIds(v) {
    return Array.isArray(v) ? v.filter(function (x) { return typeof x === 'string'; }) : [];
  }

  // Off (and a device asking for less motion) shows the end state; Calm fades.
  function motionOff() {
    var h = document.documentElement;
    return h.getAttribute('data-motion') === 'off' ||
      !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  function standardMs() {
    return window.Chronicle && Chronicle.motionMs ? Chronicle.motionMs('--dur-standard', 200) : 200;
  }

  Chronicle.register('dm-screen', {
    init: function (el) {
      var toggleBtn = el.querySelector('[data-dms-toggle]');
      var url = el.getAttribute('data-url');
      if (!toggleBtn || !url) return;

      var cid = (url.match(/\/campaigns\/([^/]+)\//) || [])[1] || '';
      var OUT_KEY = 'chronicle.dms.out.' + cid;
      var OPEN_KEY = 'chronicle.dms.open.' + cid;
      // A phone keeps its own unfolded lines, so the computer's order of
      // open cards is never disturbed.
      var LINES_KEY = 'chronicle.dms.lines.' + cid;
      // Each leaf beside the column is a list of section ids, front card last.
      var LEAVES_KEY = 'chronicle.dms.leaves.' + cid;
      // Cards the person folded to their one line. Each card folds on its own,
      // so any number can sit open in the column or on a leaf.
      var FOLDED_KEY = 'chronicle.dms.folded.' + cid;

      var savedOpen = readJSON(OPEN_KEY, DEFAULT_OPEN);
      var savedLines = readJSON(LINES_KEY, DEFAULT_LINES);
      var st = {
        lines: Array.isArray(savedLines) ? savedLines.filter(function (x) { return typeof x === 'string'; }) : DEFAULT_LINES.slice(),
        out: readJSON(OUT_KEY, false) === true,
        open: Array.isArray(savedOpen) ? savedOpen.filter(function (s) { return typeof s === 'string'; }) : DEFAULT_OPEN.slice(),
        leaves: cleanLeaves(readJSON(LEAVES_KEY, [])),
        folded: cleanIds(readJSON(FOLDED_KEY, [])),
        dock: null, tab: null, loading: null, loaded: false, brewPv: false
      };

      function root() { return st.dock && st.dock.querySelector('[data-dms-root]'); }

      // The dock starts under the top bar, whatever sits above it. The tucked
      // tab instead hangs from the top of the page area, so it clears any strip
      // (call-to-action, guest, view-as, check-up) rather than sitting in its row.
      // A page opened beside has its close mark at that same edge, so there the
      // tab hangs below that page's header.
      function place() {
        var bar = document.getElementById('app-topbar');
        var top = bar ? Math.max(0, Math.round(bar.getBoundingClientRect().bottom)) : 56;
        var row = document.getElementById('beside-row');
        var tabTop = top;
        if (row) {
          var heads = row.querySelectorAll('.beside-head');
          var edge = heads.length ? heads[heads.length - 1].getBoundingClientRect().bottom : row.getBoundingClientRect().top;
          tabTop = Math.max(0, Math.round(edge));
        }
        var rs = document.documentElement.style;
        rs.setProperty('--dms-top', top + 'px');
        rs.setProperty('--dms-tab-top', tabTop + 'px');
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
        window.addEventListener('resize', onLeafRoom);
        d.addEventListener('pointerdown', dragDown);
        window.addEventListener('pointermove', dragMove);
        window.addEventListener('pointerup', dragUp);
        window.addEventListener('pointercancel', dragUp);
        // The click that ends a drag must not also open or fold a section.
        d.addEventListener('click', function (e) {
          if (st.eatClick) { e.stopPropagation(); e.preventDefault(); }
        }, true);
        // Crossing between phone lines and the icon bar redraws the cards.
        try {
          st.mq = window.matchMedia('(max-width: 700px), (hover: none) and (pointer: coarse)');
          st.mq.addEventListener('change', function () { applyOpen(); });
        } catch (e) { /* older browsers redraw on the next open */ }
        // Any strip above the page changing height resizes the flex-1 row, and a
        // page opening or closing beside changes its children; the custom
        // property only moves a fixed element, so neither can loop.
        try {
          var row = document.getElementById('beside-row');
          if (row && window.ResizeObserver) new ResizeObserver(place).observe(row);
          if (row && window.MutationObserver) new MutationObserver(place).observe(row, { childList: true });
        } catch (e) { /* the window resize handler still covers most cases */ }
        place();
      }

      function onClick(e) {
        var t = e.target;
        if (t.closest('[data-dms-tuck]')) { tuck(true); return; }
        var rail = t.closest('[data-dms-open]');
        if (rail) { railPick(rail.getAttribute('data-dms-open')); return; }
        var fold = t.closest('[data-dms-fold]');
        if (fold) {
          var fid = fold.closest('[data-dms-sec]').getAttribute('data-dms-sec');
          if (phone()) lineToggle(fid); else headerPick(fid);
          return;
        }
        var lmerge = t.closest('[data-dms-lmerge]');
        if (lmerge) { mergeLeaf(leafIndex(lmerge), 'col'); return; }
        var lshut = t.closest('[data-dms-lshut]');
        if (lshut) { closeLeaf(leafIndex(lshut)); return; }
        var shut = t.closest('[data-dms-shut]');
        if (shut) { closeSection(shut.closest('[data-dms-sec]').getAttribute('data-dms-sec')); return; }
        if (t.closest('[data-dms-closeall]')) { closeAll(); return; }
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
        // A section on a leaf closes there; the leaf folds away once empty.
        if (leafOf(id) >= 0) { removeFromLeaf(id); return; }
        var i = st.open.indexOf(id);
        if (i === st.open.length - 1 && i >= 0) st.open.pop();
        else {
          if (i >= 0) st.open.splice(i, 1);
          else setFolded(id, false);
          st.open.push(id);
        }
        saveOpen();
        applyOpen(id);
      }

      function isFolded(id) { return st.folded.indexOf(id) >= 0; }

      function setFolded(id, on) {
        var i = st.folded.indexOf(id);
        if (on && i < 0) st.folded.push(id);
        else if (!on && i >= 0) st.folded.splice(i, 1);
        else return;
        writeJSON(FOLDED_KEY, st.folded);
      }

      // A card's header folds it to its one line or opens it whole; the others
      // stay as they are.
      function headerPick(id) {
        setFolded(id, !isFolded(id));
        applyOpen();
      }

      function closeSection(id) {
        if (leafOf(id) >= 0) { removeFromLeaf(id); return; }
        var i = st.open.indexOf(id);
        if (i < 0) return;
        st.open.splice(i, 1);
        saveOpen();
        applyOpen();
        // Focus goes back to the section's rail icon rather than to nowhere.
        var r = root(), ic = r && r.querySelector('[data-dms-open="' + id + '"]');
        if (ic) ic.focus({ preventScroll: true });
      }

      // ---- Phone lines -------------------------------------------------

      function saveLines() { writeJSON(LINES_KEY, st.lines); }

      function sectionEl(id) {
        var r = root();
        return r && r.querySelector('[data-dms-sec="' + id + '"]');
      }

      // Draws st.lines onto the markup at once: every section on screen in
      // its fixed order, the unfolded ones whole and the rest one line.
      function applyLines() {
        var r = root(); if (!r) return;
        var have = {};
        r.querySelectorAll('[data-dms-sec]').forEach(function (s) { have[s.getAttribute('data-dms-sec')] = true; });
        st.lines = st.lines.filter(function (id, i, a) { return have[id] && a.indexOf(id) === i; });
        hideLeaves();
        r.querySelectorAll('[data-dms-sec]').forEach(function (s) {
          var open = st.lines.indexOf(s.getAttribute('data-dms-sec')) >= 0;
          homeCard(s);
          s.hidden = false;
          s.style.order = '';
          setLine(s, open);
          if (open && window.Chronicle && Chronicle.mountWidgets) Chronicle.mountWidgets(s);
        });
        refreshParty();
      }

      function setLine(s, open) {
        s.classList.toggle('dms-folded', !open);
        var fold = s.querySelector('[data-dms-fold]');
        if (fold) fold.setAttribute('aria-expanded', open ? 'true' : 'false');
        var body = s.querySelector('.dms-sec-b');
        if (body) body.hidden = !open;
      }

      // A tap on a line unfolds it where it sits, or folds it again. The
      // body unrolls downward from its heading and rolls back up; with
      // reduced motion (Calm or Off) it simply swaps.
      function lineToggle(id) {
        var s = sectionEl(id);
        if (!s) return;
        var i = st.lines.indexOf(id), opening = i < 0;
        if (opening) st.lines.push(id); else st.lines.splice(i, 1);
        saveLines();
        var body = s.querySelector('.dms-sec-b');
        if (body && body._dmsAnim) { body._dmsAnim.cancel(); body._dmsAnim = null; }
        if (reduce() || !body || !body.animate) { setLine(s, opening); if (opening) mountLine(s); refreshParty(); return; }
        if (opening) {
          setLine(s, true);
          mountLine(s);
          var cs = getComputedStyle(body), h = body.scrollHeight;
          body._dmsAnim = body.animate([
            { height: '0px', paddingTop: '0px', paddingBottom: '0px', opacity: 0 },
            { height: h + 'px', paddingTop: cs.paddingTop, paddingBottom: cs.paddingBottom, opacity: 1 }
          ], { duration: standardMs(), easing: 'cubic-bezier(.2,.8,.2,1)' });
          body._dmsAnim.onfinish = function () { body._dmsAnim = null; refreshParty(); };
        } else {
          var from = body.offsetHeight, cs2 = getComputedStyle(body);
          s.classList.add('dms-folded');
          var fold = s.querySelector('[data-dms-fold]');
          if (fold) fold.setAttribute('aria-expanded', 'false');
          var a = body._dmsAnim = body.animate([
            { height: from + 'px', paddingTop: cs2.paddingTop, paddingBottom: cs2.paddingBottom, opacity: 1 },
            { height: '0px', paddingTop: '0px', paddingBottom: '0px', opacity: 0 }
          ], { duration: standardMs(), easing: 'cubic-bezier(.4,0,1,1)' });
          a.onfinish = function () { body._dmsAnim = null; body.hidden = true; };
        }
      }

      // A line's widgets (Fight's tracker, the Homebrew lists) mount when it
      // is first unfolded.
      function mountLine(s) {
        if (window.Chronicle && Chronicle.mountWidgets) Chronicle.mountWidgets(s);
      }

      // Draws st.open onto the markup: open cards shown, the newest whole and
      // first in the column, the rest folded to their one-line summary.
      function applyOpen(focusId) {
        var r = root(); if (!r) return;
        if (phone()) { applyLines(); return; }
        var have = {};
        r.querySelectorAll('[data-dms-sec]').forEach(function (s) { have[s.getAttribute('data-dms-sec')] = true; });
        // A section can leave the screen (no fight source any more); forget it.
        st.open = st.open.filter(function (id, i, a) { return have[id] && a.indexOf(id) === i; });
        fitLeaves(have);
        var n = st.open.length;
        r.querySelectorAll('[data-dms-sec]').forEach(function (s) {
          var id = s.getAttribute('data-dms-sec'), i = st.open.indexOf(id);
          if (leafOf(id) >= 0) return; // drawn onto its leaf below
          homeCard(s);
          var front = i >= 0 && !isFolded(id);
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
        drawLeaves();
        r.querySelectorAll('[data-dms-open]').forEach(function (b) {
          var oid = b.getAttribute('data-dms-open');
          b.setAttribute('aria-pressed', st.open.indexOf(oid) >= 0 || leafOf(oid) >= 0 ? 'true' : 'false');
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

      // ---- Leaves ------------------------------------------------------
      // A section dragged out of the rail opens on a leaf beside the column,
      // hinged on the column's outer edge and lying over the page. Leaves are
      // drawn inside the panel root, so its lookups and a refetch reach their
      // cards; a refetch puts every card back in the column and this redraws
      // them onto their leaves.

      function saveLeaves() { writeJSON(LEAVES_KEY, st.leaves); }

      function leafOf(id) {
        for (var i = 0; i < st.leaves.length; i++) if (st.leaves[i].indexOf(id) >= 0) return i;
        return -1;
      }

      function leafIndex(node) {
        var l = node.closest('[data-dms-leaf]');
        return l ? parseInt(l.getAttribute('data-dms-leaf'), 10) : -1;
      }

      // How many leaves fit to the left of the dock without leaving the window.
      function leafRoom() {
        var dockW = st.dock ? st.dock.offsetWidth || 320 : 320;
        return Math.max(0, Math.min(MAX_LEAVES, Math.floor((window.innerWidth - dockW - 16) / LEAF_W)));
      }

      function onLeafRoom() {
        if (st.loaded && !phone() && st.leaves.length > leafRoom()) applyOpen();
      }

      // Drops sections that left the screen, keeps each section in one place
      // (a leaf wins over the column), and hands the cards of leaves that no
      // longer fit back to the column, folded behind its front card.
      function fitLeaves(have) {
        var seen = {};
        st.leaves = st.leaves.map(function (l) {
          return l.filter(function (id) { if (!have[id] || seen[id]) return false; seen[id] = true; return true; });
        }).filter(function (l) { return l.length; });
        var room = leafRoom();
        if (st.leaves.length > room) {
          st.leaves.splice(room).forEach(function (l) {
            l.forEach(function (id) { if (st.open.indexOf(id) < 0) st.open.unshift(id); });
          });
          saveOpen();
          saveLeaves();
        }
        st.open = st.open.filter(function (id) { return !seen[id]; });
      }

      function cardsBox() { var r = root(); return r && r.querySelector('[data-dms-cards]'); }

      // A card that sat on a leaf goes back into the column's list.
      function homeCard(s) {
        var box = cardsBox();
        if (box && s.parentNode !== box) box.appendChild(s);
      }

      function leavesBox(make) {
        var r = root(); if (!r) return null;
        var box = r.querySelector('[data-dms-leaves]');
        if (!box && make) {
          box = document.createElement('div');
          box.className = 'dms-leaves';
          box.setAttribute('data-dms-leaves', '');
          r.appendChild(box);
        }
        return box;
      }

      function hideLeaves() {
        var box = leavesBox(false);
        if (box) box.hidden = true;
      }

      function sectionTitle(id) {
        var s = sectionEl(id), t = s && s.querySelector('[id^="dms-h-"]');
        return t ? t.textContent.trim() : id;
      }

      function sectionIcon(id) {
        var s = sectionEl(id), i = s && s.querySelector('.dms-sec-t > i');
        return i ? i.className : 'fa-solid fa-table-columns';
      }

      function leafButton(attr, icon, label) {
        return '<button type="button" class="dms-ic dms-ic-sm" ' + attr + ' aria-label="' + label + '" title="' + label + '">' +
          '<i class="fa-solid ' + icon + '" aria-hidden="true"></i></button>';
      }

      function makeLeaf(i) {
        var l = document.createElement('div');
        l.className = 'dms-leaf';
        l.setAttribute('role', 'region');
        l.innerHTML =
          '<div class="dms-leafbar" data-dms-leafdrag title="Drag onto the screen or another leaf to merge">' +
            '<i class="fa-solid fa-grip-vertical dms-lgrip" aria-hidden="true"></i>' +
            '<span class="dms-leaf-t"></span>' +
            '<span class="dms-leaf-tools">' +
              leafButton('data-dms-lmerge', 'fa-right-to-bracket', 'Merge into the screen') +
              leafButton('data-dms-lshut', 'fa-xmark', 'Close this leaf') +
            '</span>' +
          '</div>' +
          '<div class="dms-leaf-b"></div>';
        setLeafIndex(l, i);
        return l;
      }

      function setLeafIndex(l, i) {
        l.setAttribute('data-dms-leaf', String(i));
        l.style.right = (i * LEAF_W) + 'px';
        l.querySelector('.dms-leaf-t').textContent = 'Leaf ' + (i + 2);
      }

      // Draws st.leaves: one leaf per list, its cards in order, the last one
      // whole and the others folded to their one line.
      function drawLeaves() {
        var box = leavesBox(st.leaves.length > 0);
        if (!box) return;
        box.hidden = false;
        var els = box.querySelectorAll('.dms-leaf:not(.dms-leaf-ghost)');
        for (var k = st.leaves.length; k < els.length; k++) els[k].remove();
        st.leaves.forEach(function (list, i) {
          var l = box.querySelector('.dms-leaf[data-dms-leaf="' + i + '"]:not(.dms-leaf-ghost)');
          if (!l) { l = makeLeaf(i); box.appendChild(l); }
          l.style.visibility = '';
          l.setAttribute('aria-label', 'Leaf ' + (i + 2) + ': ' + list.map(sectionTitle).join(', '));
          var body = l.querySelector('.dms-leaf-b');
          list.forEach(function (id) {
            var s = sectionEl(id); if (!s) return;
            var front = !isFolded(id);
            body.appendChild(s);
            s.hidden = false;
            s.style.order = '';
            s.classList.toggle('dms-folded', !front);
            var fold = s.querySelector('[data-dms-fold]');
            if (fold) fold.setAttribute('aria-expanded', front ? 'true' : 'false');
            var b = s.querySelector('.dms-sec-b');
            if (b) b.hidden = !front;
            if (front && window.Chronicle && Chronicle.mountWidgets) Chronicle.mountWidgets(s);
          });
        });
      }

      // Swings a leaf open on its hinge. Calm fades it in; Off shows it.
      function leafIn(l) {
        if (!l || !l.animate || motionOff()) return;
        if (reduce()) { l.animate([{ opacity: 0 }, { opacity: 1 }], { duration: window.Chronicle && Chronicle.motionMs ? Chronicle.motionMs('--dur-standard', 200) : 200, easing: 'ease-out' }); return; }
        l.animate([
          { transform: 'perspective(1400px) rotateY(-60deg)', opacity: 0 },
          { transform: 'perspective(1400px) rotateY(0)', opacity: 1 }
        ], { duration: window.Chronicle && Chronicle.motionMs ? Chronicle.motionMs('--dur-slide', 360) : 360, easing: 'cubic-bezier(.22,.8,.24,1)' });
      }

      // Folds a leaf shut on its hinge, like closing a brochure, then runs done.
      function leafOut(l, done) {
        if (!l || !l.animate || reduce()) { done(); return; }
        l.animate([
          { transform: 'perspective(1400px) rotateY(0)', filter: 'brightness(1)' },
          { transform: 'perspective(1400px) rotateY(-90deg)', filter: 'brightness(.7)' }
        ], { duration: standardMs(), easing: 'cubic-bezier(.4,0,1,1)' }).onfinish = function () {
          l.style.visibility = 'hidden';
          done();
        };
      }

      function leafEl(i) {
        var box = leavesBox(false);
        return box && box.querySelector('.dms-leaf[data-dms-leaf="' + i + '"]:not(.dms-leaf-ghost)');
      }

      // Leaves beyond a removed one slide in to close the gap, so the screen
      // stays one joined panel.
      function dropLeaf(i) {
        var box = leavesBox(false), gone = leafEl(i);
        st.leaves.splice(i, 1);
        if (gone) {
          // Its cards go home first: they live on in the column's list.
          gone.querySelectorAll('[data-dms-sec]').forEach(function (s) { homeCard(s); s.hidden = true; });
          gone.remove();
        }
        if (box) {
          box.querySelectorAll('.dms-leaf:not(.dms-leaf-ghost)').forEach(function (l) {
            var k = parseInt(l.getAttribute('data-dms-leaf'), 10);
            if (k <= i) return;
            setLeafIndex(l, k - 1);
            if (l.animate && !reduce()) {
              l.animate([{ transform: 'translateX(-' + LEAF_W + 'px)' }, { transform: 'none' }],
                { duration: standardMs(), easing: 'cubic-bezier(.2,.8,.2,1)' });
            }
          });
        }
        saveLeaves();
      }

      function removeFromLeaf(id) {
        var i = leafOf(id); if (i < 0) return;
        var rest = st.leaves[i].filter(function (x) { return x !== id; });
        var s = sectionEl(id);
        if (rest.length) {
          st.leaves[i] = rest;
          saveLeaves();
          if (s) { homeCard(s); s.hidden = true; }
          applyOpen();
          return;
        }
        leafOut(leafEl(i), function () {
          if (s) { homeCard(s); s.hidden = true; }
          dropLeaf(i);
          applyOpen();
        });
      }

      // Pulling a section out opens a new leaf with it; it leaves the column
      // and any other leaf.
      function unfold(id) {
        setFolded(id, false);
        st.open = st.open.filter(function (x) { return x !== id; });
        var was = leafOf(id);
        if (was >= 0) {
          st.leaves[was] = st.leaves[was].filter(function (x) { return x !== id; });
          if (!st.leaves[was].length) dropLeaf(was);
        }
        st.leaves.push([id]);
        saveOpen();
        saveLeaves();
        applyOpen();
      }

      // With every leaf open, a pulled section joins the leaf it is dropped on
      // as its front card.
      function dropOnLeaf(i, id) {
        setFolded(id, false);
        st.open = st.open.filter(function (x) { return x !== id; });
        st.leaves = st.leaves.map(function (l, k) { return k === i ? l : l.filter(function (x) { return x !== id; }); });
        st.leaves[i] = st.leaves[i].filter(function (x) { return x !== id; }).concat([id]);
        st.leaves = st.leaves.filter(function (l) { return l.length; });
        saveOpen();
        saveLeaves();
        applyOpen();
      }

      // Merging folds a leaf shut and hands its cards to the column (the last
      // one in front) or to another leaf. Closing folds it and closes them.
      function mergeLeaf(i, to) {
        var cards = (st.leaves[i] || []).slice();
        if (!cards.length) return;
        leafOut(leafEl(i), function () {
          dropLeaf(i);
          if (to === 'col') {
            cards.forEach(function (id) { if (st.open.indexOf(id) < 0) st.open.push(id); });
            saveOpen();
          } else {
            var j = to > i ? to - 1 : to;
            if (st.leaves[j]) st.leaves[j] = st.leaves[j].concat(cards);
            saveLeaves();
          }
          applyOpen(to === 'col' ? cards[cards.length - 1] : null);
        });
      }

      function closeLeaf(i) {
        var cards = (st.leaves[i] || []).slice();
        leafOut(leafEl(i), function () {
          cards.forEach(function (id) { var s = sectionEl(id); if (s) { homeCard(s); s.hidden = true; } });
          dropLeaf(i);
          applyOpen();
        });
      }

      // Close all folds the leaves shut, outermost first, then closes the column.
      function closeAll() {
        var i = st.leaves.length - 1;
        if (i < 0) { st.open = []; saveOpen(); applyOpen(); return; }
        closeLeaf(i);
        setTimeout(closeAll, reduce() ? 0 : standardMs() + 20);
      }

      // ---- Dragging ----------------------------------------------------
      // A rail icon dragged toward the page swings a new leaf open after the
      // pointer, past a third of the way it settles open, short of that it
      // swings shut. A leaf's bar dragged onto the column or another leaf
      // merges it there. A plain click is untouched: a drag starts only after
      // the pointer travels.

      function dragDown(e) {
        if (e.button !== 0 || phone() || st.drag) return;
        var ic = e.target.closest('.dms-rail [data-dms-open]');
        if (ic) {
          st.drag = { kind: 'sec', id: ic.getAttribute('data-dms-open'), x: e.clientX, y: e.clientY, live: false, p: 0 };
          return;
        }
        // A card moves by its grip (any pointer, since the grip never scrolls)
        // or, with a mouse or pen, by its title, past a short travel.
        var grip = e.target.closest('[data-dms-grip]');
        var title = !grip && e.pointerType !== 'touch' && e.target.closest('.dms-sec-t');
        var card = (grip || title) && (grip || title).closest('[data-dms-sec]');
        if (card && !phone()) {
          st.drag = { kind: 'card', id: card.getAttribute('data-dms-sec'), x: e.clientX, y: e.clientY, live: false };
          return;
        }
        var bar = e.target.closest('[data-dms-leafdrag]');
        if (bar && !e.target.closest('button')) {
          e.preventDefault();
          st.drag = { kind: 'leaf', i: leafIndex(bar), x: e.clientX, y: e.clientY, live: false };
        }
      }

      function makeChip(icon, text) {
        var c = document.createElement('div');
        c.className = 'dms-dragchip';
        c.setAttribute('aria-hidden', 'true');
        var i = document.createElement('i');
        i.className = icon;
        c.appendChild(i);
        c.appendChild(document.createTextNode(text));
        st.dock.appendChild(c);
        return c;
      }

      function placeChip(d, e) {
        d.chip.style.left = (e.clientX + 10) + 'px';
        d.chip.style.top = (e.clientY - 16) + 'px';
      }

      function clearHot() {
        st.dock.querySelectorAll('.dms-hot').forEach(function (x) { x.classList.remove('dms-hot'); });
      }

      // The pointer's target under the chip, which never catches it.
      function under(e) { return document.elementFromPoint(e.clientX, e.clientY); }

      function startSecDrag(d) {
        d.live = true;
        document.documentElement.classList.add('dms-dragging');
        d.chip = makeChip(sectionIcon(d.id), sectionTitle(d.id));
        var others = st.leaves.filter(function (l) { return !(l.length === 1 && l[0] === d.id); });
        // With nothing else open there is no screen to hinge on: it opens in
        // the column. With every leaf out it joins the leaf it is dropped on.
        d.toCol = !st.open.filter(function (x) { return x !== d.id; }).length && !others.length;
        d.full = others.length >= leafRoom();
        if (d.toCol || d.full) return;
        d.at = others.length;
        var g = makeLeaf(d.at);
        g.classList.add('dms-leaf-ghost');
        g.setAttribute('aria-hidden', 'true');
        g.removeAttribute('role');
        g.querySelector('.dms-leaf-tools').remove();
        var s = sectionEl(d.id);
        if (s) {
          var copy = s.cloneNode(true);
          copy.hidden = false;
          copy.classList.remove('dms-folded');
          copy.querySelectorAll('[id]').forEach(function (n) { n.removeAttribute('id'); });
          copy.removeAttribute('aria-labelledby');
          var cb = copy.querySelector('.dms-sec-b');
          if (cb) cb.hidden = false;
          g.querySelector('.dms-leaf-b').appendChild(copy);
        }
        leavesBox(true).hidden = false;
        leavesBox(true).appendChild(g);
        d.ghost = g;
        setProgress(g, 0);
      }

      function setProgress(g, p) {
        if (reduce()) { g.style.transform = ''; g.style.opacity = String(Math.max(0.15, p)); return; }
        g.style.opacity = '';
        g.style.transform = 'perspective(1400px) rotateY(' + (-90 * (1 - p)) + 'deg)';
      }

      function startLeafDrag(d) {
        d.el = leafEl(d.i);
        if (!d.el) { st.drag = null; return; }
        d.live = true;
        document.documentElement.classList.add('dms-dragging');
        d.el.classList.add('dms-lifting');
        var n = (st.leaves[d.i] || []).length;
        d.chip = makeChip('fa-solid fa-table-columns', 'Leaf ' + (d.i + 2) + ' · ' + n + (n === 1 ? ' card' : ' cards'));
      }

      function startCardDrag(d) {
        d.live = true;
        document.documentElement.classList.add('dms-dragging');
        var s = sectionEl(d.id);
        if (s) s.classList.add('dms-lifting');
        d.chip = makeChip(sectionIcon(d.id), sectionTitle(d.id));
      }

      function clearDrop() {
        st.dock.querySelectorAll('.dms-drop-before, .dms-drop-after').forEach(function (x) {
          x.classList.remove('dms-drop-before', 'dms-drop-after');
        });
      }

      // Where a dragged card would land under the pointer: next to another
      // card, at the end of a leaf, or at the end of the column.
      function cardTarget(e, id) {
        var o = under(e);
        var other = o && o.closest('[data-dms-sec]');
        // Over its own card a drop does nothing.
        if (other && other.getAttribute('data-dms-sec') === id) return null;
        if (other && !other.hidden) {
          var r = other.getBoundingClientRect();
          return { id: other.getAttribute('data-dms-sec'), after: e.clientY > r.top + r.height / 2, el: other };
        }
        var lf = o && o.closest('.dms-leaf:not(.dms-leaf-ghost)');
        if (lf) return { leaf: parseInt(lf.getAttribute('data-dms-leaf'), 10), el: lf };
        var col = o && o.closest('.dms-col');
        if (col) return { col: true, el: col };
        return null;
      }

      // Moves a card to the place the drop chose, folded or open as it was. The
      // column lists its newest card first, so its order is read reversed.
      function moveCard(id, t) {
        var toLeaf = t.leaf !== undefined ? t.leaf : (t.id ? leafOf(t.id) : -1);
        var targetList = toLeaf >= 0 ? st.leaves[toLeaf] : null;
        st.open = st.open.filter(function (x) { return x !== id; });
        st.leaves = st.leaves.map(function (l) { return l.filter(function (x) { return x !== id; }); });
        targetList = toLeaf >= 0 ? st.leaves[toLeaf] : null;
        if (targetList) {
          var at = t.id ? targetList.indexOf(t.id) + (t.after ? 1 : 0) : targetList.length;
          targetList.splice(at < 0 ? targetList.length : at, 0, id);
        } else {
          var disp = st.open.slice().reverse();
          var k = t.id ? disp.indexOf(t.id) + (t.after ? 1 : 0) : disp.length;
          disp.splice(k < 0 ? disp.length : k, 0, id);
          st.open = disp.reverse();
        }
        st.leaves = st.leaves.filter(function (l) { return l.length; });
        saveOpen();
        saveLeaves();
        applyOpen(id);
      }

      function dragMove(e) {
        var d = st.drag; if (!d) return;
        if (!d.live && d.kind === 'card') {
          if (Math.abs(e.clientX - d.x) + Math.abs(e.clientY - d.y) < 8) return;
          startCardDrag(d);
        }
        if (d.live && d.kind === 'card') {
          e.preventDefault();
          placeChip(d, e);
          clearHot();
          clearDrop();
          d.to = cardTarget(e, d.id);
          if (d.to) {
            if (d.to.id) d.to.el.classList.add(d.to.after ? 'dms-drop-after' : 'dms-drop-before');
            else d.to.el.classList.add('dms-hot');
          }
          return;
        }
        if (!d.live) {
          if (d.kind === 'sec' && Math.abs(e.clientX - d.x) < 10) return;
          if (d.kind === 'leaf' && Math.abs(e.clientX - d.x) + Math.abs(e.clientY - d.y) < 8) return;
          if (d.kind === 'sec') startSecDrag(d); else startLeafDrag(d);
          if (!st.drag) return;
        }
        e.preventDefault();
        placeChip(d, e);
        clearHot();
        if (d.kind === 'leaf') {
          var o = under(e);
          var lf = o && o.closest('.dms-leaf:not(.dms-lifting):not(.dms-leaf-ghost)');
          var col = !lf && o && o.closest('.dms-col');
          d.to = lf ? parseInt(lf.getAttribute('data-dms-leaf'), 10) : col ? 'col' : null;
          if (lf) lf.classList.add('dms-hot'); else if (col) col.classList.add('dms-hot');
          return;
        }
        if (d.ghost) {
          // The leaf starts to open once the pointer passes its hinge.
          var hinge = st.dock.getBoundingClientRect().left - d.at * LEAF_W;
          d.p = Math.max(0, Math.min(1, (hinge - e.clientX) / (LEAF_W * 0.8)));
          setProgress(d.ghost, d.p);
        }
        if (d.full) {
          var over = under(e), leaf = over && over.closest('.dms-leaf:not(.dms-leaf-ghost)');
          d.over = leaf ? parseInt(leaf.getAttribute('data-dms-leaf'), 10) : null;
          if (leaf) leaf.classList.add('dms-hot');
        }
      }

      // Puts the page back as it was before the drag, without acting on it.
      function dragEnd() {
        var d = st.drag; st.drag = null;
        document.documentElement.classList.remove('dms-dragging');
        if (!d || !st.dock) return d;
        if (d.chip) d.chip.remove();
        if (d.el) d.el.classList.remove('dms-lifting');
        if (d.kind === 'card') {
          var ls = sectionEl(d.id);
          if (ls) ls.classList.remove('dms-lifting');
          clearDrop();
        }
        clearHot();
        return d;
      }

      function dragUp(e) {
        var d = st.drag;
        if (!d) return;
        if (!d.live) { st.drag = null; return; }
        dragEnd();
        st.eatClick = true;
        setTimeout(function () { st.eatClick = false; }, 0);
        if (e.type === 'pointercancel') { if (d.ghost) d.ghost.remove(); return; }
        if (d.kind === 'card') {
          if (e.type !== 'pointercancel' && d.to) moveCard(d.id, d.to);
          return;
        }
        if (d.kind === 'leaf') {
          if (d.to !== null && d.to !== undefined) mergeLeaf(d.i, d.to);
          return;
        }
        if (d.toCol) {
          if (st.open.indexOf(d.id) < 0) railPick(d.id);
          return;
        }
        if (d.full) {
          if (d.over !== null && d.over !== undefined) dropOnLeaf(d.over, d.id);
          else if (Chronicle.notify) Chronicle.notify('The screen opens to ' + (leafRoom() + 1) + ' leaves at most here. Drop it on a leaf.', 'info');
          return;
        }
        var g = d.ghost;
        if (d.p < 0.35) {
          if (reduce() || !g.animate) { g.remove(); return; }
          g.animate([{ transform: g.style.transform }, { transform: 'perspective(1400px) rotateY(-90deg)' }],
            { duration: window.Chronicle && Chronicle.motionMs ? Chronicle.motionMs('--dur-standard', 200) : 200, easing: 'cubic-bezier(.4,0,1,1)' }).onfinish = function () { g.remove(); };
          return;
        }
        var finish = function () { g.remove(); unfold(d.id); };
        if (reduce() || !g.animate) { finish(); return; }
        g.animate([{ transform: g.style.transform }, { transform: 'perspective(1400px) rotateY(0)' }],
          { duration: Math.round(220 * (1 - d.p)) + 60, easing: 'cubic-bezier(.16,1,.3,1)', fill: 'forwards' }).onfinish = finish;
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
          // The rail's tuck mark is hidden on a phone, whose close mark is the
          // one in the heading.
          var ts = root() ? Array.prototype.slice.call(root().querySelectorAll('[data-dms-tuck]')) : [];
          var t = ts.filter(function (b) { return b.offsetParent !== null; })[0];
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
      st.onLeafRoom = onLeafRoom;
      st.dragMove = dragMove;
      st.dragUp = dragUp;
      st.dragEnd = dragEnd;
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
      window.removeEventListener('resize', st.onLeafRoom);
      window.removeEventListener('pointermove', st.dragMove);
      window.removeEventListener('pointerup', st.dragUp);
      window.removeEventListener('pointercancel', st.dragUp);
      if (st.dragEnd) st.dragEnd();
      if (window.Chronicle && Chronicle.destroyWidget) st.dock.querySelectorAll('[data-widget]').forEach(Chronicle.destroyWidget);
      st.dock.remove();
      if (st.tab) st.tab.remove();
      document.documentElement.classList.remove('dms-docked');
    }
  });
})();
