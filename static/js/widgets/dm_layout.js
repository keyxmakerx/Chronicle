/*
 * dm_layout.js — the docked screen's layouts, as plain data.
 *
 * An arrangement is how one screen is laid out: the column's open cards
 * (oldest first, the column shows the newest on top), folded cards, the
 * leaves beside the column, each card's height (S, M or L, L by default),
 * the cards that span both columns, the hidden ones, and one or two
 * columns. A person's preferences hold Default's arrangement (home, their
 * everyday screen), up to eight named layouts, the current one and what it
 * shows now (live, set once it is changed). Default is always there; a
 * saved layout can never take its name.
 *
 * Nothing here touches the page, so the rules can be tested on their own
 * (test/js/dm_layout.test.mjs); dm_arrange.js draws them. The server checks
 * the same rules again (internal/plugins/dmscreen/prefs.go).
 */
(function (root) {
  'use strict';

  var L = {
    MAX_LAYOUTS: 8,
    MAX_NAME: 40,
    MAX_LEAVES: 2,
    DEFAULT_NAME: 'Default',
    ID: /^[a-z][a-z0-9_-]{0,31}$/,
    SIZES: ['S', 'M', 'L']
  };

  function isArr(v) { return Array.isArray(v); }

  // A list of section ids, kept only if each is the shape the server allows,
  // repeats dropped.
  L.ids = function (v) {
    if (!isArr(v)) return [];
    var out = [];
    v.forEach(function (x) { if (typeof x === 'string' && L.ID.test(x) && out.indexOf(x) < 0) out.push(x); });
    return out;
  };

  // The screen's own starting arrangement: these cards open, nothing else set.
  L.factory = function (open) {
    return { open: L.ids(open), folded: [], leaves: [], sizes: {}, wide: [], hidden: [], cols: 1 };
  };

  // An arrangement in its one shape: a card in one place (a leaf wins over
  // the column), a hidden card neither open nor on a leaf, L heights left out.
  L.clean = function (a, fallbackOpen) {
    if (!a || typeof a !== 'object') return L.factory(fallbackOpen || []);
    var hidden = L.ids(a.hidden), seen = {};
    hidden.forEach(function (id) { seen[id] = true; });
    var leaves = (isArr(a.leaves) ? a.leaves : []).map(function (l) {
      return L.ids(l).filter(function (id) { if (seen[id]) return false; seen[id] = true; return true; });
    }).filter(function (l) { return l.length; }).slice(0, L.MAX_LEAVES);
    var open = L.ids(a.open).filter(function (id) { return !seen[id]; });
    var sizes = {};
    if (a.sizes && typeof a.sizes === 'object') {
      Object.keys(a.sizes).sort().forEach(function (id) {
        var s = a.sizes[id];
        if (L.ID.test(id) && (s === 'S' || s === 'M')) sizes[id] = s;
      });
    }
    return { open: open, folded: L.ids(a.folded), leaves: leaves, sizes: sizes, wide: L.ids(a.wide), hidden: hidden, cols: a.cols === 2 ? 2 : 1 };
  };

  L.copy = function (a) { return JSON.parse(JSON.stringify(a)); };

  // Two arrangements are the same screen when they would draw the same:
  // order matters for open cards and leaves, not for the sets.
  L.equal = function (a, b) {
    var x = L.clean(a), y = L.clean(b);
    function sorted(v) { return v.slice().sort().join(','); }
    return JSON.stringify(x.open) === JSON.stringify(y.open) &&
      JSON.stringify(x.leaves) === JSON.stringify(y.leaves) &&
      JSON.stringify(x.sizes) === JSON.stringify(y.sizes) &&
      sorted(x.folded) === sorted(y.folded) && sorted(x.wide) === sorted(y.wide) &&
      sorted(x.hidden) === sorted(y.hidden) && x.cols === y.cols;
  };

  // ---- Preferences ----------------------------------------------------

  // A preferences document from the server or this browser's cache, kept
  // only in the shape we write.
  L.cleanPrefs = function (p) {
    p = p && typeof p === 'object' ? p : {};
    var layouts = [], names = {};
    (isArr(p.layouts) ? p.layouts : []).forEach(function (l) {
      if (!l || typeof l.name !== 'string' || layouts.length >= L.MAX_LAYOUTS) return;
      var name = l.name.trim();
      if (L.nameError(name, layouts) !== null || names[name.toLowerCase()]) return;
      names[name.toLowerCase()] = true;
      var a = L.clean(l);
      a.name = name;
      layouts.push(a);
    });
    var cur = typeof p.current === 'string' ? L.find({ layouts: layouts }, p.current) : null;
    var style = p.style && typeof p.style === 'object' ? p.style : {};
    return {
      current: cur ? cur.name : '',
      home: p.home ? L.clean(p.home) : null,
      live: cur && p.live ? L.clean(p.live) : null,
      layouts: layouts,
      style: {
        surface: ['glass', 'felt'].indexOf(style.surface) >= 0 ? style.surface : 'solid',
        density: style.density === 'compact' ? 'compact' : 'roomy'
      }
    };
  };

  // The saved layout with that name, any case.
  L.find = function (p, name) {
    var n = String(name || '').trim().toLowerCase();
    if (!n) return null;
    for (var i = 0; i < (p.layouts || []).length; i++) if (p.layouts[i].name.toLowerCase() === n) return p.layouts[i];
    return null;
  };

  // Why a name can't be used, or null. except is the layout being renamed.
  L.nameError = function (name, layouts, except) {
    var n = String(name || '').trim();
    if (!n) return 'Give the layout a name.';
    if (n.length > L.MAX_NAME) return 'Keep the name to ' + L.MAX_NAME + ' characters.';
    if (/[\u0000-\u001f\u007f]/.test(n)) return 'Use letters, numbers and spaces.';
    if (n.toLowerCase() === L.DEFAULT_NAME.toLowerCase()) return 'Default is always there. Pick another name.';
    var low = n.toLowerCase(), ex = except ? String(except).toLowerCase() : null;
    for (var i = 0; i < (layouts || []).length; i++) {
      var o = layouts[i].name.toLowerCase();
      if (o === low && o !== ex) return 'A layout already has that name.';
    }
    return null;
  };

  // What the screen shows for these preferences: the current layout (as
  // changed since, when it was), or Default's arrangement.
  L.shown = function (p, factoryOpen) {
    var cur = p.current ? L.find(p, p.current) : null;
    if (cur) return L.clean(p.live || cur);
    return L.clean(p.home || L.factory(factoryOpen));
  };

  // True while the current saved layout shows something other than itself.
  L.edited = function (p) {
    var cur = p.current ? L.find(p, p.current) : null;
    return !!(cur && p.live && !L.equal(p.live, cur));
  };

  // Records what the screen now shows: Default's arrangement follows it;
  // a saved layout stays as saved and the change is held as live. Returns
  // the keys that changed, for a partial save.
  L.record = function (p, a) {
    var arr = L.clean(a), cur = p.current ? L.find(p, p.current) : null;
    if (!cur) {
      if (p.home && L.equal(p.home, arr)) return [];
      p.home = arr;
      return ['home'];
    }
    var live = L.equal(arr, cur) ? null : arr;
    if (JSON.stringify(live) === JSON.stringify(p.live || null)) return [];
    p.live = live;
    return ['live'];
  };

  // Switches to a layout ('' is Default) and returns what to show.
  L.pick = function (p, name, factoryOpen) {
    var cur = name ? L.find(p, name) : null;
    p.current = cur ? cur.name : '';
    p.live = null;
    return L.shown(p, factoryOpen);
  };

  // Saves what the screen shows as a new layout and switches to it. Returns
  // an error message, or null.
  L.saveAs = function (p, name, a) {
    if (p.layouts.length >= L.MAX_LAYOUTS) return 'Eight layouts at most. Delete one to save another.';
    var err = L.nameError(name, p.layouts);
    if (err) return err;
    var l = L.clean(a);
    l.name = String(name).trim();
    p.layouts.push(l);
    p.current = l.name;
    p.live = null;
    return null;
  };

  // Writes what the screen shows over the current saved layout.
  L.update = function (p, a) {
    var cur = p.current ? L.find(p, p.current) : null;
    if (!cur) return 'Pick a saved layout first.';
    var l = L.clean(a);
    l.name = cur.name;
    p.layouts[p.layouts.indexOf(cur)] = l;
    p.live = null;
    return null;
  };

  L.rename = function (p, from, to) {
    var l = L.find(p, from);
    if (!l) return 'That layout is gone.';
    var err = L.nameError(to, p.layouts, l.name);
    if (err) return err;
    var was = l.name;
    l.name = String(to).trim();
    if (p.current === was) p.current = l.name;
    return null;
  };

  // Deleting the current layout leaves the screen as it is, now on Default.
  L.remove = function (p, name) {
    var l = L.find(p, name);
    if (!l) return 'That layout is gone.';
    p.layouts.splice(p.layouts.indexOf(l), 1);
    if (p.current === l.name) { p.current = ''; p.live = null; }
    return null;
  };

  // The document for the server: only the named keys, so a save never
  // carries back keys another tab may have changed.
  L.body = function (p, keys) {
    var out = {};
    keys.forEach(function (k) {
      if (k === 'style') {
        out.style = {
          surface: p.style.surface === 'solid' ? '' : p.style.surface,
          density: p.style.density === 'roomy' ? '' : p.style.density
        };
      } else if (k === 'layouts') {
        out.layouts = p.layouts.map(L.copy);
      } else {
        out[k] = p[k] === undefined ? null : p[k];
      }
    });
    return out;
  };

  // ---- Moving a card with the keys -------------------------------------

  // Moves a card one step: up or down in its own list, out to the next leaf
  // (opening one when there is room), or in toward the column. The column
  // shows its newest card first, so its list is read reversed. Returns the
  // new open list and leaves and where the card now is, or null when it
  // cannot move that way.
  L.move = function (state, id, dir, room) {
    var open = state.open.slice(), leaves = state.leaves.map(function (l) { return l.slice(); });
    var leaf = -1;
    leaves.forEach(function (l, i) { if (l.indexOf(id) >= 0) leaf = i; });
    if (leaf < 0 && open.indexOf(id) < 0) return null;
    var max = Math.min(L.MAX_LEAVES, room === undefined ? L.MAX_LEAVES : room);
    if (dir === 'up' || dir === 'down') {
      var list = leaf >= 0 ? leaves[leaf] : open.slice().reverse();
      var i = list.indexOf(id), j = dir === 'up' ? i - 1 : i + 1;
      if (j < 0 || j >= list.length) return null;
      list.splice(i, 1);
      list.splice(j, 0, id);
      if (leaf < 0) open = list.slice().reverse();
    } else if (dir === 'out') {
      var to = leaf + 1;
      if (to >= leaves.length && to >= max) return null;
      // A card alone on the outermost leaf has nowhere further to go.
      if (leaf >= 0 && leaves[leaf].length === 1 && to >= leaves.length) return null;
      if (leaf >= 0) leaves[leaf].splice(leaves[leaf].indexOf(id), 1);
      else open.splice(open.indexOf(id), 1);
      if (to >= leaves.length) leaves.push([]);
      leaves[to].push(id);
    } else if (dir === 'in') {
      if (leaf < 0) return null;
      leaves[leaf].splice(leaves[leaf].indexOf(id), 1);
      if (leaf === 0) open.push(id); else leaves[leaf - 1].push(id);
    } else {
      return null;
    }
    leaves = leaves.filter(function (l) { return l.length; });
    var where = { box: 'col', pos: 0, of: 0 };
    leaves.forEach(function (l, i) {
      var k = l.indexOf(id);
      if (k >= 0) where = { box: i, pos: k + 1, of: l.length };
    });
    if (where.box === 'col') {
      var disp = open.slice().reverse();
      where = { box: 'col', pos: disp.indexOf(id) + 1, of: disp.length };
    }
    return { open: open, leaves: leaves, where: where };
  };

  if (typeof module !== 'undefined' && module.exports) module.exports = L;
  if (root) { root.Chronicle = root.Chronicle || {}; root.Chronicle.dmsLayout = L; }
})(typeof window !== 'undefined' ? window : null);
