/*
 * dm_arrange.js — Arrange mode, saved layouts and the Style drawer for the
 * docked screen (dm_screen.js), on whichever screen the dock is showing.
 *
 * Arrange mode is entered from the rail. Every card then shows its grip, a
 * height (S, M or L, the body scrolling inside), a "two columns wide"
 * switch when the column is two wide, and an eye that hides it. Hidden
 * cards stay in view, tinted, until Done, and leave the rail otherwise. A
 * bar at the top of the column names the layout, sets one or two columns
 * and resets to the starting arrangement. A focused grip moves its card
 * with the arrow keys, said aloud in a live region.
 *
 * The layout menu on the rail switches between Default (the person's
 * everyday screen, always there) and up to eight saved layouts, saves the
 * screen as a new one, and renames or deletes saved ones. The Style tab at
 * the rail's foot opens a drawer for the kit (a member look pick, so the
 * owner's "who can change" setting holds), the surface and the density.
 *
 * Everything is saved to the person's account (GET and PUT
 * /campaigns/:id/screen-prefs/:screen, partial: only changed keys go), with
 * this browser's copy as the fast cache. The layout rules live in
 * dm_layout.js.
 */
(function () {
  'use strict';

  var KITS = [['material', 'Material'], ['sharp', 'Sharp'], ['ornate', 'Ornate'], ['flowing', 'Flowing'], ['lumen', 'Lumen'], ['sketchbook', 'Sketchbook']];
  var SURFACES = [['solid', 'Solid'], ['glass', 'Glass over the page'], ['felt', 'Table felt']];
  var DENSITIES = [['roomy', 'Roomy'], ['compact', 'Compact']];
  var SIZES = [['S', 'Short'], ['M', 'Medium'], ['L', 'Whole card']];
  // Two columns need this much window; the CSS uses the same width.
  var TWO_COLS = '(min-width: 1180px)';

  function esc(s) {
    return Chronicle.escapeHtml(String(s == null ? '' : s));
  }

  function readJSON(key) {
    try { return JSON.parse(localStorage.getItem(key) || 'null'); } catch (e) { return null; }
  }

  function writeJSON(key, v) {
    try { localStorage.setItem(key, JSON.stringify(v)); } catch (e) { /* not remembered */ }
  }

  function kitName(id) {
    for (var i = 0; i < KITS.length; i++) if (KITS[i][0] === id) return KITS[i][1];
    return 'Material';
  }

  // The motion level: Off (or a device asking for less) shows end states,
  // Calm fades in place.
  function motion() {
    var h = document.documentElement, m = h.getAttribute('data-motion');
    if (m === 'off' || (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches)) return 'off';
    return m === 'calm' ? 'calm' : 'full';
  }

  function token(name, fallback) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || fallback;
  }

  function ms(name, fallback) {
    return window.Chronicle && Chronicle.motionMs ? Chronicle.motionMs(name, fallback) : fallback;
  }

  function seg(label, attr, choices, cur) {
    return '<span class="dms-seg2" role="group" aria-label="' + esc(label) + '">' + choices.map(function (c) {
      return '<button type="button" ' + attr + '="' + c[0] + '" aria-pressed="' + (c[0] === cur) + '">' + esc(c[1]) + '</button>';
    }).join('') + '</span>';
  }

  window.Chronicle = window.Chronicle || {};

  /*
   * dmsArrange attaches to one dock. api is what dm_screen.js lends it:
   * the dock's state (st: open, folded, leaves, and the sizes, wide, hidden
   * and cols this file adds), its panel root, its redraw and its storage.
   */
  Chronicle.dmsArrange = function (api) {
    var L = Chronicle.dmsLayout;
    if (!L) return null;
    var st = api.st, dock = api.dock;
    var url = '/campaigns/' + encodeURIComponent(api.cid) + '/screen-prefs/' + encodeURIComponent(api.screen);
    var cached = readJSON(api.prefsKey);
    var P = L.cleanPrefs(cached);
    var kit = null;
    var arranging = false, menu = null, drawer = null;
    var dirty = {}, timer = null, inflight = false, again = false, loaded = false, told = false;

    // The cache gives heights, wide cards, hidden cards and columns at once;
    // open, folded and leaves come from the dock's own keys, the freshest.
    (function () {
      var a = L.shown(P, api.factoryOpen);
      st.sizes = a.sizes; st.wide = a.wide; st.hidden = a.hidden; st.cols = a.cols;
    })();

    function snapshot() {
      return L.clean({ open: st.open, folded: st.folded, leaves: st.leaves, sizes: st.sizes, wide: st.wide, hidden: st.hidden, cols: st.cols });
    }

    // Puts an arrangement on the dock and redraws it.
    function show(a) {
      a = L.clean(a);
      st.open = a.open.slice(); st.folded = a.folded.slice(); st.leaves = L.copy(a.leaves);
      st.sizes = L.copy(a.sizes); st.wide = a.wide.slice(); st.hidden = a.hidden.slice(); st.cols = a.cols;
      api.saveAll();
      api.applyOpen();
    }

    function root() { return api.root(); }
    function roomTwo() { return !!(window.matchMedia && window.matchMedia(TWO_COLS).matches); }

    // ---- Saving -------------------------------------------------------

    function mark(keys) {
      keys.forEach(function (k) { dirty[k] = true; });
      writeJSON(api.prefsKey, P);
      clearTimeout(timer);
      timer = setTimeout(flush, 600);
    }

    // Sends the changed keys. A failure keeps everything here and says so
    // once; the keys go again with the next change.
    function flush() {
      if (!loaded) return;
      if (inflight) { again = true; return; }
      var keys = Object.keys(dirty);
      if (!keys.length) return;
      var body = L.body(P, keys);
      dirty = {};
      inflight = true;
      Chronicle.apiFetch(url, { method: 'PUT', body: body })
        .then(function (r) { if (!r.ok) throw new Error('save ' + r.status); return r.json(); })
        .then(function (j) { if (j && j.kit) kit = j.kit; })
        .catch(function () {
          keys.forEach(function (k) { dirty[k] = true; });
          if (!told && Chronicle.notify) {
            told = true;
            Chronicle.notify('Your screen layout is kept in this browser for now. It could not be saved to your account.', 'info');
          }
        })
        .then(function () {
          inflight = false;
          if (again) { again = false; flush(); }
        });
    }

    // The server's copy wins over this browser's, except for what changed
    // here before it answered. With nothing stored yet, what this browser
    // remembers becomes Default's starting point.
    function load() {
      Chronicle.apiFetch(url, { headers: { Accept: 'application/json' } })
        .then(function (r) { if (!r.ok) throw new Error('load ' + r.status); return r.json(); })
        .then(function (j) {
          kit = j.kit || null;
          if (j.stored) {
            var srv = L.cleanPrefs(j.prefs);
            Object.keys(dirty).forEach(function (k) { srv[k] = P[k]; });
            P = srv;
            var a = L.shown(P, api.factoryOpen);
            if (!L.equal(a, snapshot())) show(a);
            writeJSON(api.prefsKey, P);
          } else {
            L.record(P, snapshot());
            if (!P.home) P.home = snapshot();
            ['current', 'home', 'live', 'layouts', 'style'].forEach(function (k) { dirty[k] = true; });
          }
          paintStyle();
          if (drawer) paintDrawer();
        })
        .catch(function () { /* this browser's copy stands */ })
        .then(function () { loaded = true; flush(); });
    }

    // The dock changed what it shows (a card opened, folded, moved).
    function changed() {
      var keys = L.record(P, snapshot());
      if (keys.length) mark(keys);
      paintBar();
    }

    // ---- Live region --------------------------------------------------

    var live = document.createElement('div');
    live.className = 'sr-only';
    live.setAttribute('aria-live', 'polite');
    live.setAttribute('data-dms-live', '');
    dock.appendChild(live);
    function say(text) {
      live.textContent = '';
      setTimeout(function () { live.textContent = text; }, 30);
    }

    // ---- What the panel needs ----------------------------------------

    var RAIL = '<hr class="dms-arr-hr">' +
      '<button type="button" class="dms-ic" data-dms-arrange aria-pressed="false" aria-label="Arrange the screen" title="Arrange">' +
        '<i class="fa-solid fa-table-cells-large" aria-hidden="true"></i></button>' +
      '<button type="button" class="dms-ic" data-dms-laymenu aria-haspopup="dialog" aria-expanded="false" aria-label="Layouts" title="Layouts">' +
        '<i class="fa-solid fa-bookmark" aria-hidden="true"></i></button>' +
      '<button type="button" class="dms-styletab" data-dms-styletab aria-expanded="false" aria-controls="dms-style-' + esc(api.screen) + '">Style</button>';

    var BAR = '<div class="dms-arr-top"><b>Arranging</b><span class="dms-arr-lay" data-dms-layname></span>' +
        '<button type="button" class="dms-arr-done" data-dms-done>Done</button></div>' +
      '<div class="dms-arr-row">' + seg('Screen width', 'data-dms-cols', [['1', 'One column'], ['2', 'Two columns']], '1') +
        '<button type="button" class="dms-quiet" data-dms-resetlay>Reset to default</button></div>';

    function tools(title) {
      return '<span class="dms-sz" role="group" aria-label="Height of ' + esc(title) + '">' + SIZES.map(function (s) {
        return '<button type="button" data-dms-sz="' + s[0] + '" aria-pressed="false" aria-label="' + s[1] + '" title="' + s[1] + '">' + s[0] + '</button>';
      }).join('') + '</span>' +
        '<button type="button" class="dms-ic dms-ic-sm" data-dms-wide aria-pressed="false" aria-label="Two columns wide" title="Two columns wide">' +
          '<i class="fa-solid fa-left-right" aria-hidden="true"></i></button>' +
        '<button type="button" class="dms-ic dms-ic-sm" data-dms-hide aria-label="Hide ' + esc(title) + '" title="Hide">' +
          '<i class="fa-solid fa-eye" aria-hidden="true"></i></button>';
    }

    // Adds the rail's icons, the bar and each card's tools to a freshly
    // mounted panel. A refetch replaces the panel, so this runs each time.
    function mounted() {
      var r = root(); if (!r) return;
      var rail = r.querySelector('.dms-rail');
      if (rail && !rail.querySelector('[data-dms-arrange]')) rail.insertAdjacentHTML('beforeend', RAIL);
      var head = r.querySelector('.dms-colhead');
      if (head && !r.querySelector('[data-dms-arrbar]')) {
        var bar = document.createElement('div');
        bar.className = 'dms-arrbar';
        bar.setAttribute('data-dms-arrbar', '');
        bar.innerHTML = BAR;
        head.parentNode.insertBefore(bar, head.nextSibling);
      }
      r.querySelectorAll('[data-dms-sec]').forEach(function (s) {
        var h = s.querySelector('.dms-sec-h');
        if (!h || h.querySelector('[data-dms-arrtools]')) return;
        var span = document.createElement('span');
        span.className = 'dms-arr';
        span.setAttribute('data-dms-arrtools', '');
        span.innerHTML = tools(api.sectionTitle(s.getAttribute('data-dms-sec')));
        h.insertBefore(span, h.querySelector('[data-dms-shut]'));
      });
      paintStyle();
    }

    // ---- Drawing ------------------------------------------------------

    function paintBar() {
      var r = root(); if (!r) return;
      var name = r.querySelector('[data-dms-layname]');
      if (name) name.textContent = (P.current || L.DEFAULT_NAME) + (L.edited(P) ? ', changed' : '');
      r.querySelectorAll('[data-dms-cols]').forEach(function (b) {
        var two = b.getAttribute('data-dms-cols') === '2';
        b.setAttribute('aria-pressed', String((st.cols === 2 && roomTwo()) === two));
        if (two) {
          b.disabled = !roomTwo();
          b.title = roomTwo() ? '' : 'Two columns need a wider window';
        }
      });
    }

    // Runs at the end of every redraw of the dock (dm_screen.js applyOpen).
    function paint() {
      var r = root(); if (!r) return;
      var phone = api.phone(), arr = arranging && !phone;
      dock.classList.toggle('dms-arranging', arr);
      var two = st.cols === 2 && roomTwo() && !phone;
      dock.classList.toggle('dms-cols2', two);
      document.documentElement.classList.toggle('dms-two', two);
      r.querySelectorAll('[data-dms-sec]').forEach(function (s) {
        var id = s.getAttribute('data-dms-sec');
        var size = st.sizes[id] || 'L';
        s.setAttribute('data-dms-size', size);
        s.classList.toggle('dms-wide', st.wide.indexOf(id) >= 0);
        var hid = !phone && st.hidden.indexOf(id) >= 0;
        s.classList.toggle('dms-hid', hid);
        if (hid) {
          if (arr) {
            api.homeCard(s);
            s.hidden = false;
            s.style.order = '9999';
            var b = s.querySelector('.dms-sec-b');
            if (b) b.hidden = true;
          } else {
            s.hidden = true;
          }
        }
        s.querySelectorAll('[data-dms-sz]').forEach(function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-dms-sz') === size)); });
        var wide = s.querySelector('[data-dms-wide]');
        if (wide) wide.setAttribute('aria-pressed', String(st.wide.indexOf(id) >= 0));
        var eye = s.querySelector('[data-dms-hide]');
        if (eye) {
          var title = api.sectionTitle(id);
          eye.setAttribute('aria-label', (hid ? 'Show ' : 'Hide ') + title);
          eye.title = hid ? 'Show' : 'Hide';
          eye.querySelector('i').className = 'fa-solid ' + (hid ? 'fa-eye-slash' : 'fa-eye');
        }
        // A grip is a control only while arranging, where the keys move it.
        var grip = s.querySelector('[data-dms-grip]');
        if (grip) {
          if (arr && !hid) {
            grip.setAttribute('tabindex', '0');
            grip.setAttribute('role', 'button');
            grip.setAttribute('aria-label', 'Move ' + api.sectionTitle(id) + '. Arrow keys move it.');
            grip.removeAttribute('aria-hidden');
          } else {
            grip.removeAttribute('tabindex');
            grip.removeAttribute('role');
            grip.removeAttribute('aria-label');
            grip.setAttribute('aria-hidden', 'true');
          }
        }
      });
      r.querySelectorAll('.dms-rail [data-dms-open]').forEach(function (b) {
        var hid = !phone && st.hidden.indexOf(b.getAttribute('data-dms-open')) >= 0;
        b.hidden = hid && !arr;
        b.classList.toggle('dms-hid', hid && arr);
      });
      var ab = r.querySelector('[data-dms-arrange]');
      if (ab) ab.setAttribute('aria-pressed', String(arr));
      // While arranging, a hidden card counts as on screen.
      var empty = r.querySelector('[data-dms-empty]');
      if (empty && arr && st.hidden.length) empty.hidden = true;
      paintSlot();
      paintBar();
    }

    // The dashed place beside the screen where a card dropped opens a leaf.
    var slot = null;
    function paintSlot() {
      var can = arranging && !api.phone() && st.leaves.length < api.leafRoom();
      if (!can) { if (slot) slot.hidden = true; return; }
      if (!slot) {
        slot = document.createElement('div');
        slot.className = 'dms-leafslot';
        slot.setAttribute('data-dms-leafslot', '');
        slot.setAttribute('aria-hidden', 'true');
        slot.innerHTML = '<span>Drop a card here to open it on a leaf</span>';
        dock.appendChild(slot);
      }
      slot.hidden = false;
      slot.style.right = 'calc(100% + ' + (st.leaves.length * 264 + 10) + 'px)';
    }

    function paintStyle() {
      var s = P.style;
      if (s.surface === 'solid') dock.removeAttribute('data-dms-surface'); else dock.setAttribute('data-dms-surface', s.surface);
      if (s.density === 'roomy') dock.removeAttribute('data-dms-density'); else dock.setAttribute('data-dms-density', s.density);
      document.documentElement.classList.toggle('dms-glass', s.surface === 'glass');
      var tab = root() && root().querySelector('[data-dms-styletab]');
      if (tab) tab.setAttribute('aria-expanded', String(!!(drawer && !drawer.hidden && drawer.getAttribute('data-open') === '1')));
    }

    // ---- Arrange mode -------------------------------------------------

    function setArranging(on) {
      if (on === arranging) return;
      arranging = on;
      closeMenu(false);
      var had = dock.contains(document.activeElement) && document.activeElement.closest('[data-dms-arrbar], [data-dms-arrtools], [data-dms-grip]');
      api.applyOpen();
      var r = root(), bar = r && r.querySelector('[data-dms-arrbar]');
      if (on && bar && bar.animate && motion() !== 'off') {
        bar.animate(motion() === 'calm' ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 0, transform: 'translateY(-4px)' }, { opacity: 1, transform: 'none' }],
          { duration: ms('--dur-standard', 200), easing: token('--ease-out', 'ease-out') });
      }
      if (!on && had) { var ab = r && r.querySelector('[data-dms-arrange]'); if (ab) ab.focus({ preventScroll: true }); }
      say(on ? 'Arranging ' + (P.current || L.DEFAULT_NAME) + '. Done or Escape leaves.' : 'Done arranging.');
    }

    function setSize(id, size) {
      if (size === 'L') delete st.sizes[id]; else st.sizes[id] = size;
      paint();
      changed();
    }

    function toggleWide(id) {
      var i = st.wide.indexOf(id);
      if (i >= 0) st.wide.splice(i, 1); else st.wide.push(id);
      if (i < 0 && st.cols !== 2 && roomTwo()) st.cols = 2;
      api.applyOpen();
      changed();
    }

    function setCols(n) {
      st.cols = n === 2 ? 2 : 1;
      api.applyOpen();
      changed();
      say(n === 2 ? 'Two columns.' : 'One column.');
    }

    function toggleHidden(id) {
      var i = st.hidden.indexOf(id), title = api.sectionTitle(id);
      if (i >= 0) {
        st.hidden.splice(i, 1);
        if (st.open.indexOf(id) < 0) st.open.push(id);
        say(title + ' shown, at the top of the column.');
      } else {
        st.hidden.push(id);
        st.open = st.open.filter(function (x) { return x !== id; });
        st.leaves = st.leaves.map(function (l) { return l.filter(function (x) { return x !== id; }); }).filter(function (l) { return l.length; });
        say(title + ' hidden. It leaves the rail when you are done.');
      }
      api.saveAll();
      api.applyOpen();
      changed();
      var s = api.sectionEl(id), eye = s && s.querySelector('[data-dms-hide]');
      if (eye) eye.focus({ preventScroll: true });
    }

    // A focused grip and an arrow key move the card one place.
    function keyMove(e, grip) {
      var dirs = { ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'out', ArrowRight: 'in' };
      var dir = dirs[e.key];
      if (!dir) return;
      e.preventDefault();
      var id = grip.closest('[data-dms-sec]').getAttribute('data-dms-sec');
      var m = L.move({ open: st.open, leaves: st.leaves }, id, dir, api.leafRoom());
      var title = api.sectionTitle(id);
      if (!m) {
        say(title + ' can’t move ' + ({ up: 'up', down: 'down', out: 'further out', 'in': 'further in' })[dir] + '.');
        return;
      }
      st.open = m.open;
      st.leaves = m.leaves;
      api.saveAll();
      api.applyOpen();
      changed();
      var s = api.sectionEl(id), g = s && s.querySelector('[data-dms-grip]');
      if (g) g.focus({ preventScroll: true });
      var w = m.where;
      say(title + ' is ' + w.pos + ' of ' + w.of + (w.box === 'col' ? ' in the column.' : ' on leaf ' + (w.box + 2) + '.'));
    }

    function resetDefault() {
      P.current = '';
      P.live = null;
      P.home = null;
      show(L.factory(api.factoryOpen));
      mark(['current', 'live', 'home']);
      paintBar();
      say('Back to the starting layout.');
    }

    // ---- Layout menu --------------------------------------------------

    function menuHTML() {
      var edited = L.edited(P);
      var rows = '<button type="button" class="dms-lm-pick" data-dms-lay="" aria-current="' + (!P.current) + '">' +
          '<i class="fa-solid fa-check dms-lm-ck" aria-hidden="true"></i><span>' + L.DEFAULT_NAME + '<small>Always there</small></span></button>';
      P.layouts.forEach(function (l, i) {
        var cur = P.current === l.name;
        rows += '<div class="dms-lm-row" data-dms-lrow="' + i + '">' +
          '<button type="button" class="dms-lm-pick" data-dms-lay="' + esc(l.name) + '" aria-current="' + cur + '">' +
            '<i class="fa-solid fa-check dms-lm-ck" aria-hidden="true"></i><span>' + esc(l.name) + (cur && edited ? '<small>Changed since saved</small>' : '') + '</span></button>' +
          '<button type="button" class="dms-ic dms-ic-sm" data-dms-lren="' + i + '" aria-label="Rename ' + esc(l.name) + '" title="Rename"><i class="fa-solid fa-pen" aria-hidden="true"></i></button>' +
          '<button type="button" class="dms-ic dms-ic-sm" data-dms-ldel="' + i + '" aria-label="Delete ' + esc(l.name) + '" title="Delete"><i class="fa-solid fa-trash-can" aria-hidden="true"></i></button>' +
        '</div>';
      });
      var full = P.layouts.length >= L.MAX_LAYOUTS;
      return '<div class="dms-lm-h" id="dms-lm-h">Layouts</div>' + rows +
        '<div class="dms-lm-acts">' +
          (edited ? '<button type="button" class="dms-quiet" data-dms-lupdate>Save changes to ' + esc(P.current) + '</button>' : '') +
          '<button type="button" class="dms-quiet" data-dms-lsave' + (full ? ' disabled' : '') + '>Save as a new layout</button>' +
          (full ? '<span class="dms-lm-note">Eight layouts at most. Delete one to save another.</span>' : '') +
        '</div>' +
        '<p class="dms-lm-foot">Kept with your account, so your other devices have them too.</p>';
    }

    function openMenu(btn, byKey) {
      closeDrawer(false);
      menu = document.createElement('div');
      menu.className = 'dms-laymenu';
      menu.setAttribute('role', 'dialog');
      menu.setAttribute('aria-labelledby', 'dms-lm-h');
      menu.setAttribute('data-dms-menu', '');
      menu.innerHTML = menuHTML();
      dock.appendChild(menu);
      var d = dock.getBoundingClientRect(), b = btn.getBoundingClientRect();
      menu.style.top = Math.max(8, Math.min(b.top - d.top, dock.clientHeight - menu.offsetHeight - 8)) + 'px';
      btn.setAttribute('aria-expanded', 'true');
      var m = motion();
      if (!byKey && m !== 'off' && menu.animate) {
        menu.animate(m === 'calm' ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 0, transform: 'scale(.94)' }, { opacity: 1, transform: 'none' }],
          { duration: ms('--dur-everyday', 120), easing: token('--ease-out', 'ease-out') });
      }
      var first = menu.querySelector('[aria-current="true"]') || menu.querySelector('button');
      if (first) first.focus({ preventScroll: true });
    }

    function closeMenu(refocus) {
      if (!menu) return;
      var m = menu;
      menu = null;
      var btn = root() && root().querySelector('[data-dms-laymenu]');
      if (btn) btn.setAttribute('aria-expanded', 'false');
      if (refocus && btn) btn.focus({ preventScroll: true });
      if (motion() === 'off' || !m.animate) { m.remove(); return; }
      m.style.pointerEvents = 'none';
      m.animate([{ opacity: 1 }, { opacity: 0 }], { duration: ms('--dur-everyday-out', 80), easing: token('--ease-in', 'ease-in') }).onfinish = function () { m.remove(); };
    }

    function redrawMenu(focusSel) {
      if (!menu) return;
      menu.innerHTML = menuHTML();
      var f = focusSel && menu.querySelector(focusSel);
      if (f) f.focus({ preventScroll: true });
    }

    // An inline name box in place of a menu row: Enter saves, Escape backs out.
    function nameForm(where, value, label, onSave) {
      var form = document.createElement('form');
      form.className = 'dms-lm-form';
      form.innerHTML = '<input type="text" maxlength="' + L.MAX_NAME + '" aria-label="' + esc(label) + '" placeholder="A name, like Fight night">' +
        '<button type="submit" class="dms-arr-done">Save</button><p class="dms-lm-err" role="alert" hidden></p>';
      where.replaceWith(form);
      var input = form.querySelector('input'), err = form.querySelector('.dms-lm-err');
      input.value = value || '';
      input.focus();
      input.select();
      form.addEventListener('submit', function (e) {
        e.preventDefault();
        var msg = onSave(input.value);
        if (msg) { err.textContent = msg; err.hidden = false; input.focus(); }
      });
      form.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); redrawMenu('[data-dms-lsave]'); }
      });
    }

    function pick(name) {
      show(L.pick(P, name, api.factoryOpen));
      mark(['current', 'live']);
      closeMenu(true);
      paintBar();
      say((P.current || L.DEFAULT_NAME) + ' is on.');
    }

    function menuClick(t) {
      var lay = t.closest('[data-dms-lay]');
      if (lay) { pick(lay.getAttribute('data-dms-lay')); return; }
      if (t.closest('[data-dms-lupdate]')) {
        var name = P.current;
        if (!L.update(P, snapshot())) { mark(['layouts', 'live']); say(name + ' saved.'); paintBar(); }
        closeMenu(true);
        return;
      }
      var save = t.closest('[data-dms-lsave]');
      if (save && !save.disabled) {
        nameForm(save, '', 'Name for the new layout', function (v) {
          var err = L.saveAs(P, v, snapshot());
          if (err) return err;
          mark(['layouts', 'current', 'live']);
          closeMenu(true);
          paintBar();
          say('Saved as ' + P.current + '.');
          return null;
        });
        return;
      }
      var ren = t.closest('[data-dms-lren]');
      if (ren) {
        var l = P.layouts[+ren.getAttribute('data-dms-lren')];
        if (!l) return;
        nameForm(ren.closest('[data-dms-lrow]'), l.name, 'New name for ' + l.name, function (v) {
          var from = l.name, err = L.rename(P, from, v);
          if (err) return err;
          mark(['layouts', 'current']);
          redrawMenu('[data-dms-lay="' + CSS.escape(l.name) + '"]');
          paintBar();
          say(from + ' is now ' + l.name + '.');
          return null;
        });
        return;
      }
      var del = t.closest('[data-dms-ldel]');
      if (del) {
        var dl = P.layouts[+del.getAttribute('data-dms-ldel')];
        if (!dl) return;
        var row = del.closest('[data-dms-lrow]');
        row.innerHTML = '<span class="dms-lm-ask">Delete ' + esc(dl.name) + '?</span>' +
          '<button type="button" class="dms-quiet dms-lm-yes" data-dms-ldelyes="' + esc(dl.name) + '">Delete</button>' +
          '<button type="button" class="dms-quiet" data-dms-ldelno>Keep</button>';
        row.querySelector('[data-dms-ldelno]').focus({ preventScroll: true });
        return;
      }
      var yes = t.closest('[data-dms-ldelyes]');
      if (yes) {
        var gone = yes.getAttribute('data-dms-ldelyes');
        if (!L.remove(P, gone)) { mark(['layouts', 'current', 'live']); say(gone + ' deleted.'); paintBar(); }
        redrawMenu('[data-dms-lsave]');
        return;
      }
      if (t.closest('[data-dms-ldelno]')) redrawMenu('[data-dms-lsave]');
    }

    // ---- Style drawer -------------------------------------------------

    function kitTile(value, label, look, cur) {
      return '<button type="button" class="dms-ktile" data-dms-kitpick="' + value + '" aria-pressed="' + (value === cur) + '">' +
        '<span class="dms-kt" data-dms-kit="' + look + '" aria-hidden="true"><span class="dms-kt-card"><span class="dms-kt-h">Aa</span>' +
        '<span class="dms-kt-bar"><i></i></span><span class="dms-kt-l"></span></span></span>' +
        '<span class="dms-kt-n">' + esc(label) + '</span></button>';
    }

    function kitRow() {
      if (!kit) return '';
      var head = '<div class="dms-lbl" id="dms-sd-kit">Kit</div>';
      if (!kit.canPick) {
        var line = kit.owner ?
          'Wears ' + esc(kitName(kit.campaign)) + ', the campaign’s choice. Change it in <a class="dms-link" href="/campaigns/' + encodeURIComponent(api.cid) + '/customize/look">Customize</a>.' :
          'Follows the campaign’s choice: ' + esc(kitName(kit.campaign)) + '. The owner decides who can change it.';
        return head + '<p class="dms-sd-note">' + line + '</p>';
      }
      var cur = kit.mine || '';
      return head + '<div class="dms-kt-grid" role="group" aria-labelledby="dms-sd-kit">' +
        kitTile('', 'Follow the campaign', kit.campaign, cur) +
        KITS.map(function (k) { return kitTile(k[0], k[1], k[0], cur); }).join('') + '</div>';
    }

    function paintDrawer() {
      if (!drawer) return;
      var row = drawer.querySelector('[data-dms-kitrow]');
      row.innerHTML = kitRow();
      row.hidden = !kit;
      drawer.querySelectorAll('[data-dms-surf]').forEach(function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-dms-surf') === P.style.surface)); });
      drawer.querySelectorAll('[data-dms-dens]').forEach(function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-dms-dens') === P.style.density)); });
    }

    function buildDrawer() {
      drawer = document.createElement('div');
      drawer.className = 'dms-sdrawer';
      drawer.id = 'dms-style-' + api.screen;
      drawer.setAttribute('role', 'dialog');
      drawer.setAttribute('aria-label', 'Style for this screen');
      drawer.hidden = true;
      drawer.innerHTML = '<div class="dms-sd-h"><b>Style</b><span>only for you</span>' +
          '<button type="button" class="dms-ic dms-ic-sm" data-dms-sdclose aria-label="Close the style drawer" title="Close"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>' +
        '<div class="dms-sd-sec" data-dms-kitrow></div>' +
        '<div class="dms-sd-sec"><div class="dms-lbl">Surface</div>' + seg('Surface', 'data-dms-surf', SURFACES, P.style.surface) + '</div>' +
        '<div class="dms-sd-sec"><div class="dms-lbl">Density</div>' + seg('Density', 'data-dms-dens', DENSITIES, P.style.density) + '</div>' +
        '<p class="dms-sd-foot">Kept with your account, on every device. The campaign’s pages keep their own look.</p>';
      dock.appendChild(drawer);
    }

    // The kits' faces load once, the first time the drawer can offer them.
    function loadKits() {
      if (!kit || !kit.canPick || document.querySelector('link[data-dms-kits]')) return;
      var link = document.createElement('link');
      link.rel = 'stylesheet';
      link.href = url + '/kits.css';
      link.setAttribute('data-dms-kits', '');
      document.head.appendChild(link);
    }

    // The drawer slides out from behind the screen, level with its tab.
    function openDrawer() {
      closeMenu(false);
      if (!drawer) buildDrawer();
      paintDrawer();
      loadKits();
      drawer.hidden = false;
      drawer.setAttribute('data-open', '1');
      paintStyle();
      var m = motion();
      if (m !== 'off' && drawer.animate) {
        drawer.animate(m === 'calm' ? [{ opacity: 0 }, { opacity: 1 }] : [{ transform: 'translateX(100%)' }, { transform: 'none' }],
          { duration: m === 'calm' ? ms('--dur-standard', 200) : ms('--dur-slide', 360), easing: token('--ease-slide', 'ease-out') });
      }
      var f = drawer.querySelector('[aria-pressed="true"]') || drawer.querySelector('button');
      if (f) f.focus({ preventScroll: true });
    }

    function closeDrawer(refocus) {
      if (!drawer || drawer.getAttribute('data-open') !== '1') return;
      drawer.setAttribute('data-open', '0');
      paintStyle();
      var tab = root() && root().querySelector('[data-dms-styletab]');
      if (refocus && tab) tab.focus({ preventScroll: true });
      var d = drawer, m = motion();
      var done = function () { if (d.getAttribute('data-open') !== '1') d.hidden = true; };
      if (m === 'off' || !d.animate) { done(); return; }
      d.animate(m === 'calm' ? [{ opacity: 1 }, { opacity: 0 }] : [{ transform: 'none' }, { transform: 'translateX(100%)' }],
        { duration: ms('--dur-standard', 200), easing: token('--ease-in', 'ease-in') }).onfinish = done;
    }

    function pickKit(value) {
      if (!kit || !kit.canPick) return;
      var prev = kit.mine || '';
      kit.mine = value;
      dock.setAttribute('data-dms-kit', value || kit.campaign);
      paintDrawer();
      var b = drawer.querySelector('[data-dms-kitpick="' + value + '"]');
      if (b) b.focus({ preventScroll: true });
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(api.cid) + '/my-look/' + encodeURIComponent(kit.part), { method: 'PUT', body: { value: value || null } })
        .then(function (r) { if (!r.ok) throw new Error('kit ' + r.status); })
        .catch(function () {
          kit.mine = prev;
          dock.setAttribute('data-dms-kit', prev || kit.campaign);
          paintDrawer();
          if (Chronicle.notify) Chronicle.notify('That kit could not be saved. Try again.', 'error');
        });
    }

    function setStyle(key, value) {
      P.style[key] = value;
      paintStyle();
      paintDrawer();
      mark(['style']);
    }

    // ---- Events -------------------------------------------------------

    dock.addEventListener('click', function (e) {
      var t = e.target;
      if (menu && menu.contains(t)) { menuClick(t); return; }
      if (t.closest('[data-dms-arrange]')) { setArranging(!arranging); return; }
      if (t.closest('[data-dms-done]')) { setArranging(false); return; }
      var lm = t.closest('[data-dms-laymenu]');
      if (lm) { if (menu) closeMenu(false); else openMenu(lm, e.detail === 0); return; }
      if (menu) closeMenu(false);
      if (t.closest('[data-dms-styletab]')) { if (drawer && drawer.getAttribute('data-open') === '1') closeDrawer(false); else openDrawer(); return; }
      if (t.closest('[data-dms-sdclose]')) { closeDrawer(true); return; }
      var kp = t.closest('[data-dms-kitpick]');
      if (kp) { pickKit(kp.getAttribute('data-dms-kitpick')); return; }
      var sf = t.closest('[data-dms-surf]');
      if (sf) { setStyle('surface', sf.getAttribute('data-dms-surf')); return; }
      var dn = t.closest('[data-dms-dens]');
      if (dn) { setStyle('density', dn.getAttribute('data-dms-dens')); return; }
      var cs = t.closest('[data-dms-cols]');
      if (cs && !cs.disabled) { setCols(+cs.getAttribute('data-dms-cols')); return; }
      if (t.closest('[data-dms-resetlay]')) { resetDefault(); return; }
      var sec = t.closest('[data-dms-sec]');
      var id = sec && sec.getAttribute('data-dms-sec');
      var sz = t.closest('[data-dms-sz]');
      if (sz && id) { setSize(id, sz.getAttribute('data-dms-sz')); return; }
      if (t.closest('[data-dms-wide]') && id) { toggleWide(id); return; }
      if (t.closest('[data-dms-hide]') && id) { toggleHidden(id); }
    });

    dock.addEventListener('keydown', function (e) {
      var g = arranging && e.target.closest && e.target.closest('[data-dms-grip]');
      if (g) keyMove(e, g);
    });

    // A click anywhere else closes the layout menu.
    function onDoc(e) {
      if (menu && !menu.contains(e.target) && !(e.target.closest && e.target.closest('[data-dms-laymenu]'))) closeMenu(false);
    }
    document.addEventListener('click', onDoc, true);

    var mq = window.matchMedia ? window.matchMedia(TWO_COLS) : null;
    function onRoom() { if (st.cols === 2) api.applyOpen(); else paintBar(); }
    try { if (mq) mq.addEventListener('change', onRoom); } catch (e) { /* the next redraw catches up */ }

    load();

    return {
      mounted: mounted,
      paint: paint,
      changed: changed,
      arranging: function () { return arranging && !api.phone(); },
      // Escape closes the innermost thing first: the menu, the drawer, then
      // arrange mode. True when it closed something.
      escape: function () {
        if (menu) { closeMenu(true); return true; }
        if (drawer && drawer.getAttribute('data-open') === '1') { closeDrawer(true); return true; }
        if (arranging) { setArranging(false); return true; }
        return false;
      },
      destroy: function () {
        document.removeEventListener('click', onDoc, true);
        try { if (mq) mq.removeEventListener('change', onRoom); } catch (e) { /* gone with the page */ }
        document.documentElement.classList.remove('dms-two', 'dms-glass');
      }
    };
  };
})();
