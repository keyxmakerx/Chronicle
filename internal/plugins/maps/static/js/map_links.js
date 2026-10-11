/**
 * map_links.js -- pins that open another map.
 *
 * A pin may carry linked_map_id (and the target's linked_map_name). This module
 * draws what follows from that: the badge on the pin, the "Open <map>" button
 * and the who-can-follow line in the pin card, the "Opens map" chooser in the
 * pin form, the trail at the top of the viewer and the tree of linked maps.
 *
 * Who can follow a link is the pin's own visibility. The server already sent
 * this viewer only the pins they may see, and builds the tree from those same
 * lists (GET /campaigns/:id/maps/link-tree), so nothing here decides access;
 * the words about who can follow are shown to owners and co-DMs only.
 *
 * The trail is the ?from= list of maps the viewer came through, checked by the
 * server against the campaign's maps and used for display only. On a plain
 * visit it falls back to the current map's place in the tree.
 *
 * Names are user text: they only ever go through textContent and attributes,
 * never innerHTML. The pure helpers at the top are exported for the Node tests.
 */
(function () {
  'use strict';

  // ---- Pure helpers ----

  // foldTrail shortens a long trail by folding its middle into one gap entry:
  // past three maps it keeps the first and the last two, past two on a narrow
  // screen the first and the last. Entries are the steps themselves or
  // { gap: true }.
  function foldTrail(steps, narrow) {
    var list = Array.isArray(steps) ? steps.slice() : [];
    var max = narrow ? 2 : 3;
    if (list.length <= max) return list;
    if (narrow) return [list[0], { gap: true }, list[list.length - 1]];
    return [list[0], { gap: true }].concat(list.slice(-2));
  }

  function parseRules(raw) {
    if (!raw) return null;
    if (typeof raw === 'object') return raw;
    try { return JSON.parse(raw); } catch (e) { return null; }
  }

  // audience reads who can follow a pin's link from its visibility and rules,
  // the same two fields the server filters on: 'dm' (DM only), 'some' (only
  // the allowed players), 'except' (players but the denied ones) or 'everyone'.
  function audience(visibility, rules) {
    if (visibility === 'dm_only') return { kind: 'dm', users: [] };
    var r = parseRules(rules);
    var allowed = r && Array.isArray(r.allowed_users) ? r.allowed_users.filter(Boolean) : [];
    var denied = r && Array.isArray(r.denied_users) ? r.denied_users.filter(Boolean) : [];
    if (allowed.length) return { kind: 'some', users: allowed };
    if (denied.length) return { kind: 'except', users: denied };
    return { kind: 'everyone', users: [] };
  }

  // nameList joins names the way a sentence does: "Mira", "Mira and Kael",
  // "Mira, Kael and Tam".
  function nameList(names) {
    var n = (names || []).filter(Boolean);
    if (n.length <= 1) return n.join('');
    return n.slice(0, -1).join(', ') + ' and ' + n[n.length - 1];
  }

  // namesOf turns user ids into display names. An id with no known member (a
  // player who left) is counted rather than shown as an id.
  function namesOf(ids, members) {
    var known = [];
    var unknown = 0;
    (ids || []).forEach(function (id) {
      var nm = members && Object.prototype.hasOwnProperty.call(members, id) ? members[id] : '';
      if (nm) known.push(nm); else unknown++;
    });
    if (unknown) known.push(unknown === 1 ? (known.length ? '1 other player' : '1 player') : unknown + (known.length ? ' other players' : ' players'));
    return nameList(known);
  }

  // audienceWords is the who-can-follow line of the pin card and form.
  function audienceWords(aud, members) {
    if (aud.kind === 'dm') return 'Only you and members with DM access';
    if (aud.kind === 'some') return namesOf(aud.users, members) + ' can open this';
    if (aud.kind === 'except') return 'Players except ' + namesOf(aud.users, members) + ' can open this';
    return 'Players can open this';
  }

  // badgeTip is the badge's tooltip and accessible name. Players read where it
  // goes; owners and co-DMs also read who else can follow it.
  function badgeTip(mapName, aud, members, staff) {
    var t = 'Opens ' + mapName;
    if (!staff) return t;
    if (aud.kind === 'dm') return t + '. Only you and members with DM access can open it.';
    if (aud.kind === 'some') return t + '. ' + namesOf(aud.users, members) + ' can open it too.';
    if (aud.kind === 'except') return t + '. Players except ' + namesOf(aud.users, members) + ' can open it too.';
    return t + '. Players can open it too.';
  }

  // mapURL is the map page for id, carrying the maps the viewer came through.
  function mapURL(campaignID, id, fromIDs) {
    var u = '/campaigns/' + encodeURIComponent(campaignID) + '/maps/' + encodeURIComponent(id);
    var from = (fromIDs || []).filter(Boolean);
    if (from.length) u += '?from=' + from.map(encodeURIComponent).join(',');
    return u;
  }

  // pathTo returns the nodes from a root down to the map id, or [] when the
  // tree does not hold it.
  function pathTo(roots, id) {
    var out = [];
    function walk(nodes) {
      for (var i = 0; i < (nodes || []).length; i++) {
        out.push(nodes[i]);
        if (nodes[i].id === id || walk(nodes[i].children)) return true;
        out.pop();
      }
      return false;
    }
    walk(roots);
    return out.slice();
  }

  // treeKey decides what a key does in the tree. items are the visible rows in
  // order, each { level, expanded } where expanded is true, false, or null for
  // a map with no branch. The answer is an action for the DOM part to carry out.
  function treeKey(items, i, key) {
    var it = items[i];
    if (!it) return null;
    switch (key) {
      case 'ArrowDown': return { focus: Math.min(items.length - 1, i + 1) };
      case 'ArrowUp': return { focus: Math.max(0, i - 1) };
      case 'Home': return { focus: 0 };
      case 'End': return { focus: items.length - 1 };
      case 'ArrowRight':
        if (it.expanded === false) return { expand: i };
        if (it.expanded === true) return { focus: i + 1 };
        return { none: true };
      case 'ArrowLeft':
        if (it.expanded === true) return { collapse: i };
        for (var j = i - 1; j >= 0; j--) if (items[j].level < it.level) return { focus: j };
        return { none: true };
      case 'Enter':
      case ' ':
        return { open: i };
      case 'Escape': return { close: true, refocus: true };
      case 'Tab': return { close: true, refocus: false };
    }
    return null;
  }

  var api = {
    foldTrail: foldTrail,
    audience: audience,
    nameList: nameList,
    namesOf: namesOf,
    audienceWords: audienceWords,
    badgeTip: badgeTip,
    mapURL: mapURL,
    pathTo: pathTo,
    treeKey: treeKey
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof window === 'undefined') return;

  // ---- DOM ----
  // How much motion this person gets: html[data-motion] is 'calm' or 'off'
  // (absent is full), and a device asking for reduced motion is always off.
  function motionLevel() {
    var m = document.documentElement.getAttribute('data-motion');
    if (m === 'off' || (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches)) return 'off';
    return m === 'calm' ? 'calm' : 'full';
  }
  // dur reads a duration token so the owner's Motion speed retimes these too.
  function dur(name, fallback) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    var n = parseFloat(v);
    if (!isFinite(n)) return fallback;
    return /ms$/.test(v) ? n : n * 1000;
  }
  var EASE_OUT = 'cubic-bezier(.16,1,.3,1)';
  var EASE_IN = 'cubic-bezier(.4,0,1,1)';

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function fa(name) {
    var i = el('i', 'fa-solid ' + name);
    i.setAttribute('aria-hidden', 'true');
    return i;
  }
  // The "opens a map" glyph: a folded map with an arrow down into it. Fixed
  // markup, never user text.
  function glyph() {
    var s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    s.setAttribute('viewBox', '0 0 24 24');
    s.setAttribute('aria-hidden', 'true');
    s.setAttribute('class', 'mp-lglyph');
    ['M4 7l5-2 6 2 5-2v12l-5 2-6-2-5 2z', 'M12 9v6M9.5 12.5L12 15l2.5-2.5'].forEach(function (d) {
      var p = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      p.setAttribute('d', d);
      s.appendChild(p);
    });
    return s;
  }
  function thumb(cls, url) {
    var t = el('span', cls);
    if (url) {
      var img = el('img');
      img.alt = '';
      img.loading = 'lazy';
      img.decoding = 'async';
      // A picture that fails to load leaves the plain box, not a broken image.
      img.onerror = function () { img.remove(); };
      img.src = url;
      t.appendChild(img);
    }
    return t;
  }
  // grow plays the Grow move on a popover opening out of the thing clicked.
  function grow(node) {
    var lvl = motionLevel();
    if (lvl === 'off' || !node.animate) return;
    var d = dur('--dur-standard', 200);
    var kf = lvl === 'full'
      ? [{ opacity: 0, transform: 'scale(.96)' }, { opacity: 1, transform: 'none' }]
      : [{ opacity: 0 }, { opacity: 1 }];
    node.animate(kf, { duration: d, easing: EASE_OUT });
  }

  /**
   * attach wires the links of one mounted viewer.
   *
   * opts: { wrap, container, campaignID, mapID, mapName, canTree, staff, members,
   *         trail, linkMaps, pinPoint(mk) -> {x, y} in container pixels }
   */
  api.attach = function (opts) {
    var wrap = opts.wrap;
    var container = opts.container;
    var staff = !!opts.staff;
    var members = opts.members || {};
    var serverTrail = Array.isArray(opts.trail) ? opts.trail : [];
    var linkMaps = Array.isArray(opts.linkMaps) ? opts.linkMaps : [];
    var trailEl = wrap && wrap.querySelector('#mp-trail');
    var treeEl = wrap && wrap.querySelector('#mp-tree');
    var tree = null;            // { roots, count, truncated } once loaded
    var cleanups = [];
    var destroyed = false;
    var leaving = null;          // the running lean-and-fade, undone on pageshow

    function listen(t, evt, fn) { t.addEventListener(evt, fn); cleanups.push(function () { t.removeEventListener(evt, fn); }); }
    function audOf(mk) { return audience(mk.visibility, mk.visibility_rules); }
    function narrow() { return !!wrap && wrap.getBoundingClientRect().width < 560; }

    // The maps this view sits under: the trail the viewer came through, else
    // the current map's place in the tree, else just the current map.
    function currentPath() {
      if (serverTrail.length) return serverTrail.map(function (s) { return { id: s.id, name: s.name }; });
      if (tree) {
        var p = pathTo(tree.roots, opts.mapID);
        if (p.length) return p.map(function (n) { return { id: n.id, name: n.name }; });
      }
      return [];
    }
    function pathIDs() {
      var p = currentPath();
      return p.length ? p.map(function (s) { return s.id; }) : [opts.mapID];
    }

    // ---- Following a link: the view leans into the pin while it fades, then
    // the map page loads; the new map settles in on arrival. ----
    function go(url, point) {
      if (destroyed) return;
      closeTree(false);
      var lvl = motionLevel();
      if (lvl === 'off' || !container || !container.animate) { window.location.assign(url); return; }
      var d = lvl === 'full' ? dur('--dur-large', 280) : dur('--dur-standard', 200);
      var kf = [{ opacity: 1 }, { opacity: 0 }];
      if (lvl === 'full' && point) {
        container.style.transformOrigin = point.x + 'px ' + point.y + 'px';
        kf = [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(1.6)' }];
      }
      var a = container.animate(kf, { duration: d, easing: EASE_IN, fill: 'forwards' });
      leaving = a;
      var gone = false;
      function leave() { if (gone) return; gone = true; window.location.assign(url); }
      a.onfinish = leave;
      setTimeout(leave, d + 120);
    }
    // A page restored from the back/forward cache must not stay faded out.
    listen(window, 'pageshow', function () {
      if (leaving) { try { leaving.cancel(); } catch (e) { /* already gone */ } leaving = null; }
      if (container) container.style.transformOrigin = '';
    });
    function follow(mk) {
      if (!mk || !mk.linked_map_id) return;
      var point = opts.pinPoint ? opts.pinPoint(mk) : null;
      go(mapURL(opts.campaignID, mk.linked_map_id, pathIDs()), point);
    }

    // Settle: arriving through a link, the new map fades and rises into place.
    (function settle() {
      if (!container || !container.animate || !/[?&]from=/.test(window.location.search)) return;
      var lvl = motionLevel();
      if (lvl === 'off') return;
      var kf = lvl === 'full'
        ? [{ opacity: 0, transform: 'translateY(6px)' }, { opacity: 1, transform: 'none' }]
        : [{ opacity: 0 }, { opacity: 1 }];
      container.animate(kf, { duration: dur('--dur-large', 280), easing: EASE_OUT });
    })();

    // ---- The badge on a pin ----
    function badge(mk) {
      if (!mk.linked_map_id || !mk.linked_map_name) return null;
      var aud = audOf(mk);
      var dm = staff && aud.kind === 'dm';
      var tip = badgeTip(mk.linked_map_name, aud, members, staff);
      var b = el('button', 'mp-lbadge' + (dm ? ' dm' : ''));
      b.type = 'button';
      b.setAttribute('aria-label', tip);
      b.appendChild(glyph());
      if (staff) b.appendChild(fa(dm ? 'fa-eye-slash' : 'fa-eye'));
      if (dm) b.appendChild(el('span', 'mp-lbadge-l', 'DM only'));
      var t = el('span', 'mp-btip', tip);
      t.setAttribute('aria-hidden', 'true');
      b.appendChild(t);
      // The pin underneath opens its card and drags; the badge only follows.
      if (window.L && L.DomEvent) L.DomEvent.disableClickPropagation(b);
      b.addEventListener('click', function (e) { e.stopPropagation(); e.preventDefault(); follow(mk); });
      b.addEventListener('keydown', function (e) { if (e.key === 'Enter' || e.key === ' ') e.stopPropagation(); });
      b.addEventListener('keypress', function (e) { e.stopPropagation(); });
      return b;
    }

    // ---- The pin card ----
    function cardBits(mk) {
      if (!mk.linked_map_id || !mk.linked_map_name) return null;
      var meta = el('span', 'mp-badge mp-badge-acc');
      meta.appendChild(glyph());
      meta.appendChild(document.createTextNode('Opens a map'));
      var open = el('button', 'mp-cbtn mp-cbtn-primary mp-cbtn-ico');
      open.type = 'button';
      open.appendChild(glyph());
      open.appendChild(el('span', null, 'Open ' + mk.linked_map_name));
      open.addEventListener('click', function () { follow(mk); });
      var line = null;
      if (staff) line = visLine(audOf(mk), false);
      return { meta: meta, open: open, line: line };
    }
    // visLine says in words who can follow; with help set it is the pin form's
    // line under the chooser.
    function visLine(aud, help) {
      var can = aud.kind !== 'dm';
      var line = el('div', 'mp-vline' + (can ? '' : ' dm') + (help ? ' mp-vline-help' : ''));
      line.appendChild(fa(can ? 'fa-eye' : 'fa-eye-slash'));
      line.appendChild(el('span', null, audienceWords(aud, members)));
      return line;
    }

    // ---- The "Opens map" chooser in the pin form ----
    // getAud returns the audience the form currently describes (the visibility
    // select may change it). Returns null when no other map exists to offer.
    function chooser(mk, getAud) {
      var current = mk && mk.linked_map_id ? mk.linked_map_id : '';
      var currentName = mk && mk.linked_map_name ? mk.linked_map_name : '';
      if (!linkMaps.length && !current) return null;
      var chosen = current;
      var opts2 = [{ id: '', name: 'Nothing, just a pin' }].concat(linkMaps);
      if (current && !linkMaps.some(function (m) { return m.id === current; })) opts2.push({ id: current, name: currentName || 'Linked map' });
      function optOf(id) { for (var i = 0; i < opts2.length; i++) if (opts2[i].id === id) return opts2[i]; return opts2[0]; }

      var box = el('div', 'mp-field mp-omap');
      var lab = el('span', null, 'Opens map');
      lab.id = 'mp-omap-l-' + Math.random().toString(36).slice(2, 8);
      box.appendChild(lab);
      var sel = el('div', 'mp-mapsel');
      var btn = el('button', 'mp-input mp-omb');
      btn.type = 'button';
      btn.setAttribute('aria-haspopup', 'listbox');
      btn.setAttribute('aria-expanded', 'false');
      btn.setAttribute('aria-labelledby', lab.id);
      sel.appendChild(btn);
      box.appendChild(sel);
      var help = el('div', 'mp-vline-slot');
      help.setAttribute('aria-live', 'polite');
      box.appendChild(help);

      function optFace(o, cls) {
        var frag = document.createDocumentFragment();
        if (o.id) frag.appendChild(thumb(cls, o.thumb_url));
        else { var t = el('span', cls); t.appendChild(fa('fa-location-dot')); frag.appendChild(t); }
        frag.appendChild(el('span', 'mp-om-nm', o.name));
        return frag;
      }
      function paint() {
        btn.textContent = '';
        btn.appendChild(optFace(optOf(chosen), 'mp-om-th'));
        btn.appendChild(fa('fa-chevron-down'));
      }
      function setHelp() {
        help.textContent = '';
        if (!chosen) { help.appendChild(el('div', 'mp-vline mp-vline-help', 'Pick a map to make this pin open it.')); return; }
        help.appendChild(visLine(getAud(), true));
      }
      var menu = null;
      function closeMenu(refocus) {
        if (!menu) return false;
        menu.remove();
        menu = null;
        btn.setAttribute('aria-expanded', 'false');
        if (refocus) btn.focus();
        return true;
      }
      function pick(id) { chosen = id; paint(); setHelp(); closeMenu(true); }
      function openMenu() {
        menu = el('div', 'mp-om-menu');
        menu.setAttribute('role', 'listbox');
        menu.setAttribute('aria-labelledby', lab.id);
        opts2.forEach(function (o) {
          var b = el('button', 'mp-om-opt');
          b.type = 'button';
          b.setAttribute('role', 'option');
          b.setAttribute('aria-selected', o.id === chosen ? 'true' : 'false');
          b.dataset.id = o.id;
          b.appendChild(optFace(o, 'mp-om-th'));
          if (o.id === chosen) b.appendChild(fa('fa-check'));
          b.addEventListener('click', function () { pick(o.id); });
          menu.appendChild(b);
        });
        menu.addEventListener('keydown', function (e) {
          var items = Array.prototype.slice.call(menu.querySelectorAll('.mp-om-opt'));
          var i = items.indexOf(document.activeElement);
          var k = e.key;
          if (k === 'ArrowDown') items[Math.min(items.length - 1, i + 1)].focus();
          else if (k === 'ArrowUp') items[Math.max(0, i - 1)].focus();
          else if (k === 'Home') items[0].focus();
          else if (k === 'End') items[items.length - 1].focus();
          else if (k === 'Escape') closeMenu(true);
          else if (k === 'Tab') { closeMenu(false); return; }
          else return;
          e.preventDefault();
          e.stopPropagation();
        });
        sel.appendChild(menu);
        // Open upward when the map has no room below the button.
        var wr = wrap ? wrap.getBoundingClientRect() : null;
        var mr = menu.getBoundingClientRect();
        if (wr && mr.bottom > wr.bottom - 8) menu.classList.add('mp-om-up');
        btn.setAttribute('aria-expanded', 'true');
        grow(menu);
        var first = menu.querySelector('[aria-selected="true"]') || menu.firstChild;
        first.focus();
      }
      btn.addEventListener('click', function (e) {
        e.stopPropagation();
        if (menu) closeMenu(false); else openMenu();
      });
      btn.addEventListener('keydown', function (e) {
        if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && !menu) { e.preventDefault(); openMenu(); }
      });
      box.addEventListener('pointerdown', function (e) {
        if (menu && !e.target.closest('.mp-mapsel')) closeMenu(false);
      });
      paint();
      setHelp();
      return {
        el: box,
        value: function () { return chosen; },
        changed: function () { return chosen !== current; },
        nameOf: function (id) { return optOf(id).name; },
        refresh: setHelp,
        escape: function () { return closeMenu(true); }
      };
    }

    // ---- The trail and the tree ----
    var openSet = {};
    var expandAll = false;
    function closeTree(refocus) {
      if (!treeEl || treeEl.hidden) return false;
      treeEl.hidden = true;
      if (trailEl) trailEl.querySelectorAll('[aria-haspopup="tree"]').forEach(function (b) { b.setAttribute('aria-expanded', 'false'); });
      if (refocus && trailEl) { var t = trailEl.querySelector('.mp-tt'); if (t) t.focus(); }
      return true;
    }
    function goStep(steps, i) {
      // Back up the trail: the maps before this one stay the trail.
      var ids = steps.slice(0, i).map(function (s) { return s.id; });
      go(mapURL(opts.campaignID, steps[i].id, ids), null);
    }
    function renderTrail() {
      if (!trailEl || destroyed) return;
      var steps = currentPath();
      var hasTree = !!(tree && tree.count > 0);
      if (!steps.length && !hasTree) { trailEl.hidden = true; return; }
      if (!steps.length) steps = [{ id: opts.mapID, name: opts.mapName || '' }];
      trailEl.textContent = '';
      var treeOpen = !!(treeEl && !treeEl.hidden);
      if (hasTree) {
        var tt = el('button', 'mp-tt');
        tt.type = 'button';
        tt.setAttribute('aria-haspopup', 'tree');
        tt.setAttribute('aria-expanded', treeOpen ? 'true' : 'false');
        tt.setAttribute('aria-label', 'All linked maps');
        tt.title = 'All linked maps';
        tt.appendChild(fa('fa-sitemap'));
        var ch = fa('fa-chevron-down');
        ch.classList.add('mp-tt-sm');
        tt.appendChild(ch);
        tt.addEventListener('click', function () { if (treeEl.hidden) openTree(); else closeTree(false); });
        trailEl.appendChild(tt);
      }
      var all = steps.map(function (s, i) { return { id: s.id, name: s.name, i: i }; });
      var shown = expandAll && !hasTree ? all : foldTrail(all, narrow());
      shown.forEach(function (s, k) {
        if (k || hasTree) {
          var chev = el('span', 'mp-chev');
          chev.setAttribute('aria-hidden', 'true');
          chev.appendChild(fa('fa-chevron-right'));
          trailEl.appendChild(chev);
        }
        if (s.gap) {
          var g = el('button', 'mp-gap', '…');
          g.type = 'button';
          g.setAttribute('aria-label', 'Show every level');
          g.title = 'Show every level';
          if (hasTree) {
            g.setAttribute('aria-haspopup', 'tree');
            g.setAttribute('aria-expanded', treeOpen ? 'true' : 'false');
            g.addEventListener('click', function () { if (treeEl.hidden) openTree(); else closeTree(false); });
          } else {
            // Without the tree (a visitor who is not signed in) the gap
            // unfolds the trail in place.
            g.addEventListener('click', function () { expandAll = true; renderTrail(); });
          }
          trailEl.appendChild(g);
        } else if (s.i === steps.length - 1) {
          var cur = el('span', 'mp-cur', s.name);
          cur.setAttribute('aria-current', 'page');
          cur.title = s.name;
          trailEl.appendChild(cur);
        } else {
          var b = el('button', null, s.name);
          b.type = 'button';
          b.title = s.name;
          b.addEventListener('click', function () { goStep(steps, s.i); });
          trailEl.appendChild(b);
        }
      });
      trailEl.hidden = false;
    }

    function nodeEl(n, level) {
      var kids = n.children && n.children.length;
      var isCur = n.id === opts.mapID;
      var li = el('li');
      li.setAttribute('role', 'treeitem');
      li.setAttribute('aria-level', String(level));
      li.setAttribute('aria-selected', isCur ? 'true' : 'false');
      if (isCur) li.setAttribute('aria-current', 'page');
      if (kids) li.setAttribute('aria-expanded', openSet[n.id] ? 'true' : 'false');
      li.tabIndex = -1;
      li.dataset.id = n.id;
      li.__level = level;
      var row = el('div', 'mp-trow' + (isCur ? ' cur' : ''));
      row.style.paddingLeft = (4 + (level - 1) * 16) + 'px';
      var tw = el('span', 'mp-tw');
      if (kids) {
        tw.dataset.tw = '1';
        tw.appendChild(fa('fa-chevron-right'));
        tw.addEventListener('click', function (e) {
          e.stopPropagation();
          toggle(li, li.getAttribute('aria-expanded') !== 'true');
          focusItem(li);
        });
      }
      row.appendChild(tw);
      row.appendChild(thumb('mp-tth', n.thumb_url));
      row.appendChild(el('span', 'mp-tn', n.name));
      if (staff && n.via) {
        var aud = audience(n.via.visibility, { allowed_users: n.via.allowed_users, denied_users: n.via.denied_users });
        var tag = null;
        if (aud.kind === 'dm') {
          tag = el('span', 'mp-tvis dm');
          tag.title = 'Only you and members with DM access can open this link';
          tag.appendChild(fa('fa-eye-slash'));
          tag.appendChild(document.createTextNode('DM only'));
        } else if (aud.kind !== 'everyone') {
          tag = el('span', 'mp-tvis');
          var words = audienceWords(aud, members);
          tag.title = words + ' link';
          tag.appendChild(fa('fa-eye'));
          tag.appendChild(document.createTextNode(aud.kind === 'some' ? namesOf(aud.users, members) : 'Not ' + namesOf(aud.users, members)));
        }
        if (tag) row.appendChild(tag);
      }
      if (isCur) row.appendChild(el('span', 'mp-here', 'Here'));
      row.addEventListener('click', function () { openNode(n.id); });
      li.appendChild(row);
      if (kids) {
        var ul = el('ul');
        ul.setAttribute('role', 'group');
        if (!openSet[n.id]) ul.hidden = true;
        n.children.forEach(function (c) { ul.appendChild(nodeEl(c, level + 1)); });
        li.appendChild(ul);
      }
      return li;
    }
    function openNode(id) {
      if (id === opts.mapID) { closeTree(true); return; }
      var p = pathTo(tree.roots, id);
      go(mapURL(opts.campaignID, id, p.slice(0, -1).map(function (n) { return n.id; })), null);
    }
    function openTree() {
      if (!treeEl || !tree) return;
      // The way down to the current map starts open, and so does its branch.
      openSet = {};
      pathTo(tree.roots, opts.mapID).forEach(function (n) { openSet[n.id] = true; });
      treeEl.textContent = '';
      var head = el('div', 'mp-tth2', 'Linked maps');
      head.appendChild(el('span', 'mp-badge', String(tree.count)));
      treeEl.appendChild(head);
      var ul = el('ul');
      ul.setAttribute('role', 'tree');
      ul.setAttribute('aria-label', 'Linked maps');
      tree.roots.forEach(function (r) { ul.appendChild(nodeEl(r, 1)); });
      treeEl.appendChild(ul);
      var foot = el('p', 'mp-tfoot');
      if (staff) {
        foot.appendChild(fa('fa-eye-slash'));
        foot.appendChild(el('span', null, 'DM only: players can’t open that link. The pin’s “Who can see it” decides.'));
      } else {
        foot.appendChild(el('span', null, 'Maps you can open from pins.'));
      }
      treeEl.appendChild(foot);
      if (tree.truncated) treeEl.appendChild(el('p', 'mp-tfoot', 'Some links are left out: the tree is too large to show at once.'));
      treeEl.hidden = false;
      // Under the trail; across the map on a phone.
      var wr = wrap.getBoundingClientRect();
      var tr = trailEl.getBoundingClientRect();
      if (wr.width < 560) {
        treeEl.style.left = '12px'; treeEl.style.right = '12px'; treeEl.style.width = 'auto';
      } else {
        treeEl.style.right = 'auto'; treeEl.style.width = '';
        treeEl.style.left = Math.max(12, Math.min(tr.left - wr.left, wr.width - treeEl.offsetWidth - 12)) + 'px';
      }
      treeEl.style.top = (tr.bottom - wr.top + 6) + 'px';
      trailEl.querySelectorAll('[aria-haspopup="tree"]').forEach(function (b) { b.setAttribute('aria-expanded', 'true'); });
      grow(treeEl);
      focusItem(treeEl.querySelector('li[aria-current]') || treeEl.querySelector('li'));
    }
    function focusItem(li) {
      if (!li) return;
      treeEl.querySelectorAll('li[tabindex="0"]').forEach(function (x) { x.tabIndex = -1; });
      li.tabIndex = 0;
      li.focus();
    }
    // Branches fold to their real height; Calm and Off open them at once.
    function toggle(li, open) {
      var g = li.querySelector(':scope > ul');
      if (!g) return;
      li.setAttribute('aria-expanded', open ? 'true' : 'false');
      openSet[li.dataset.id] = open;
      if (motionLevel() !== 'full' || !g.animate) { g.hidden = !open; return; }
      g.style.overflow = 'hidden';
      if (open) {
        g.hidden = false;
        var h = g.scrollHeight;
        g.animate([{ height: '0px', opacity: 0 }, { height: h + 'px', opacity: 1 }], { duration: dur('--dur-standard', 200), easing: EASE_OUT })
          .onfinish = function () { g.style.overflow = ''; };
      } else {
        var h2 = g.scrollHeight;
        g.animate([{ height: h2 + 'px', opacity: 1 }, { height: '0px', opacity: 0 }], { duration: 160, easing: EASE_IN })
          .onfinish = function () { g.hidden = true; g.style.overflow = ''; };
      }
    }
    function visibleItems() {
      return Array.prototype.filter.call(treeEl.querySelectorAll('li'), function (li) {
        var g = li.parentNode;
        while (g && g !== treeEl) { if (g.hidden) return false; g = g.parentNode; }
        return true;
      });
    }
    if (treeEl) {
      listen(treeEl, 'keydown', function (e) {
        var li = e.target.closest && e.target.closest('li');
        if (!li) return;
        var lis = visibleItems();
        var items = lis.map(function (x) {
          var ex = x.getAttribute('aria-expanded');
          return { level: x.__level, expanded: ex === null ? null : ex === 'true' };
        });
        var act = treeKey(items, lis.indexOf(li), e.key);
        if (!act) return;
        if (act.close) {
          closeTree(act.refocus);
          if (!act.refocus) return; // Tab moves on as usual
        } else if (act.focus != null) focusItem(lis[act.focus]);
        else if (act.expand != null) toggle(lis[act.expand], true);
        else if (act.collapse != null) toggle(lis[act.collapse], false);
        else if (act.open != null) openNode(lis[act.open].dataset.id);
        e.preventDefault();
        e.stopPropagation();
      });
      listen(document, 'pointerdown', function (e) {
        if (!treeEl.hidden && !(e.target.closest && e.target.closest('#mp-tree, #mp-trail'))) closeTree(false);
      });
    }

    // Folding depends on the width the map has, which changes with the window
    // and with full screen.
    var lastNarrow = null;
    function onResize() {
      var n = narrow();
      if (n !== lastNarrow) { lastNarrow = n; renderTrail(); }
    }
    listen(window, 'resize', onResize);
    renderTrail();
    lastNarrow = narrow();

    // The tree is for campaign members (the endpoint is session only); a
    // visitor to a public campaign gets the trail the server resolved.
    if (opts.canTree && window.Chronicle && Chronicle.apiFetch) {
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(opts.campaignID) + '/maps/link-tree')
        .then(function (r) { return r.ok ? r.json() : null; })
        .then(function (t) {
          if (destroyed || !t || !Array.isArray(t.roots)) return;
          tree = t;
          renderTrail();
        })
        .catch(function () { /* the trail still works without the tree */ });
    }

    return {
      badge: badge,
      cardBits: cardBits,
      chooser: chooser,
      follow: follow,
      // escape closes the tree; true when it did.
      escape: function () { return closeTree(true); },
      destroy: function () {
        destroyed = true;
        cleanups.forEach(function (fn) { fn(); });
        closeTree(false);
      }
    };
  };

  window.ChronicleMapLinks = api;
})();
