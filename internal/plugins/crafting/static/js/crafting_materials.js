/**
 * crafting_materials.js -- material kinds and pictures on Armory items (window.CraftingMaterials)
 *
 * The workshop's Materials tab: every Armory item the Director can see, with
 * its kinds and its picture. A kind is what lets a recipe ask for "any herb"
 * and what tells a step whether an item fits (grinding takes herbs, fungi,
 * bones; pouring takes liquids, venoms, oils). A picture has its own name,
 * never a material's: each kind wears one (the Pictures board swaps or
 * changes them), and an item follows its first kind's unless it is given
 * its own. Each click saves at once; the items themselves stay the
 * Armory's to edit.
 */
window.CraftingMaterials = (function () {
  'use strict';
  var L = window.CraftingLib, CP = window.CraftPictures, esc = L.esc, PAGE = 60;
  var TINTS = ['#6a9c3a', '#3f7d4f', '#7cc242', '#8fbbe0', '#4f86c6', '#4fb3c9', '#a77cf0', '#6d45c4', '#b3263a', '#d0682e', '#d99a2b', '#e3d6b8', '#9a6a3e', '#8d939c', '#3a3f48'];
  var X = '<svg viewBox="0 0 16 16" width="12" height="12" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8"/></svg>';

  function mount(el, app) {
    var lib = app.lib, filter = 'all', query = '', shown = PAGE, busy = {}, board = false, open = null, drag = null;
    function $(s) { return el.querySelector(s); }
    function kind(k) { return L.find(lib.tags, k); }
    function picLabel(k) { var p = L.find(lib.pictures, k); return p ? p.label : 'No picture'; }

    /* How many recipes ask for each item, so a Director sees what a change
       touches before making it. */
    function uses() {
      var n = {};
      app.recipes.forEach(function (r) {
        var seen = {};
        r.materials.forEach(function (m) { if (m.itemId && !seen[m.itemId]) { seen[m.itemId] = 1; n[m.itemId] = (n[m.itemId] || 0) + 1; } });
      });
      return n;
    }
    function matches(it) {
      if (query && it.name.toLowerCase().indexOf(query) < 0) return false;
      if (filter === 'all') return true;
      if (filter === 'none') return !(it.tags || []).length;
      return (it.tags || []).indexOf(filter) >= 0;
    }
    function why(it) {
      if (it.picture) return 'its own picture';
      var k = (it.tags || [])[0];
      return k ? 'follows ' + kind(k).label : 'no kind yet';
    }

    /* ---------- Drawing ---------- */
    function renderFilters() {
      var count = {all: app.itemList.length, none: 0};
      app.itemList.forEach(function (it) {
        if (!(it.tags || []).length) count.none++;
        (it.tags || []).forEach(function (t) { count[t] = (count[t] || 0) + 1; });
      });
      // Only the kinds in use: twenty chips would bury the list
      var list = [{key: 'all', label: 'All'}, {key: 'none', label: 'No kind yet'}].concat(lib.tags.filter(function (t) { return count[t.key] || filter === t.key; }));
      $('.cra-mfilters').innerHTML = list.map(function (t) {
        return '<button type="button" class="cra-chip cra-chip-sm" data-filter="' + esc(t.key) + '" aria-pressed="' + (filter === t.key) + '">' +
          (t.picture ? CP.svg(t.picture, t.tint, 20) : '') + esc(t.label) + ' <small>' + (count[t.key] || 0) + '</small></button>';
      }).join('');
    }
    function row(it, n) {
      var tags = it.tags || [], p = L.matPicture(lib, it);
      return '<li class="cra-mrow" data-id="' + esc(it.id) + '">' +
        '<button type="button" class="cra-pic cpx-touch" data-act="pick" aria-haspopup="dialog" aria-expanded="false"' + (busy[it.id] ? ' disabled' : '') +
        ' aria-label="Picture for ' + esc(it.name) + ': ' + esc(picLabel(p.picture)) + ', ' + why(it) + '">' + CP.svg(p.picture, p.tint, 34) +
        (it.picture ? '<span class="cra-own" aria-hidden="true"></span>' : '') + '</button>' +
        '<div class="cra-mname"><b>' + esc(it.name) + '</b><small>' +
        (n[it.id] ? 'In ' + n[it.id] + (n[it.id] > 1 ? ' recipes' : ' recipe') : 'Not in a recipe') + ' · ' + why(it) + '</small></div>' +
        '<div class="cra-mtags" role="group" aria-label="Kinds for ' + esc(it.name) + '">' + tags.map(function (k) {
          var t = kind(k);
          return '<span class="cra-kind">' + CP.svg(t.picture, t.tint, 20) + esc(t.label) +
            '<button type="button" class="cra-x" data-act="untag" data-tag="' + esc(k) + '"' + (busy[it.id] ? ' disabled' : '') + ' aria-label="Take ' + esc(t.label) + ' off ' + esc(it.name) + '">' + X + '</button></span>';
        }).join('') + '<button type="button" class="cra-addk" data-act="kinds" aria-haspopup="dialog" aria-expanded="false"' + (busy[it.id] ? ' disabled' : '') + '>' +
        (tags.length ? 'Add a kind' : 'Give it a kind') + '</button></div><span class="cra-mok" aria-hidden="true"></span></li>';
    }
    function renderList() {
      var n = uses(), hits = app.itemList.filter(matches), box = $('.cra-mlist');
      if (!app.itemList.length) {
        box.innerHTML = '<div class="cra-empty">The Armory has no items yet. Add the things your world is made of there, then give them kinds here.</div>';
        return;
      }
      if (!hits.length) {
        box.innerHTML = '<div class="cra-empty">Nothing matches. <button type="button" class="cra-link" data-act="clear">Show every item</button></div>';
        return;
      }
      var more = hits.length - shown;
      box.innerHTML = '<ul class="cra-mrows">' + hits.slice(0, shown).map(function (it) { return row(it, n); }).join('') + '</ul>' +
        (more > 0 ? '<div class="cra-row"><button type="button" class="cra-link" data-act="more">Show ' + Math.min(PAGE, more) + ' more (' + more + ' left)</button></div>' : '');
    }
    function changed(t) { return t.picture !== t.defaultPicture || t.tint !== t.defaultTint; }
    function renderBoard() {
      var box = $('.cra-boardslot');
      $('.cra-itab').setAttribute('aria-expanded', String(board));
      if (!board) { box.innerHTML = ''; return; }
      var any = lib.tags.some(changed);
      box.innerHTML = '<section class="cra-board" id="cra-board" aria-label="Each kind’s picture"><div class="cra-bh"><div><h4 class="cra-ph">Each kind’s picture</h4>' +
        '<p class="cra-fine">An item shows its first kind’s picture unless you give it its own. Drag one kind onto another to swap their pictures, or open one to choose.' +
        (any ? ' <button type="button" class="cra-link" data-act="resetall">Put every kind back</button>' : '') + '</p></div>' +
        '<button type="button" class="cra-ib" data-act="board" aria-label="Close the pictures">' + X + '</button></div>' +
        '<div class="cra-btiles">' + lib.tags.map(function (t) {
          return '<button type="button" class="cra-btile cpx-touch' + (changed(t) ? ' is-changed' : '') + '" data-tile="' + esc(t.key) + '" aria-haspopup="dialog" aria-expanded="false" aria-label="' +
            esc(t.label) + ': ' + esc(picLabel(t.picture)) + '. Open to choose, or drag onto another kind to swap.">' + CP.svg(t.picture, t.tint, 36) +
            '<span><b>' + esc(t.label) + '</b><small>' + esc(picLabel(t.picture)) + '</small></span></button>';
        }).join('') + '</div></section>';
    }
    function renderAll() { renderBoard(); renderFilters(); renderList(); }

    /* ---------- Popovers: the picture picker and the kinds menu ---------- */
    function close(back) {
      if (!open) return;
      var o = open; open = null;
      o.pop.remove();
      var from = o.from;
      // A kind saved from the menu redraws its row; go back to the new one
      if (!document.body.contains(from) && o.kinds) from = el.querySelector('.cra-mrow[data-id="' + CSS.escape(o.kinds) + '"] .cra-addk');
      if (from && document.body.contains(from)) {
        from.setAttribute('aria-expanded', 'false');
        if (back) from.focus();
      }
    }
    /* A popover grows out of what was clicked, inside the pane. */
    function place(pop, from) {
      var r = from.getBoundingClientRect(), a = el.getBoundingClientRect(), left = r.left - a.left, max = el.clientWidth - pop.offsetWidth;
      pop.style.left = Math.max(0, Math.min(left, max)) + 'px';
      pop.style.top = (r.bottom - a.top + 6) + 'px';
      pop.style.transformOrigin = left > max ? 'top right' : 'top left';
    }
    function show(from, cls, label, html, target) {
      close();
      var pop = document.createElement('div');
      pop.className = cls; pop.setAttribute('role', 'dialog'); pop.setAttribute('aria-label', label);
      pop.innerHTML = html;
      el.querySelector('.cra-mats-pane').appendChild(pop);
      place(pop, from);
      from.setAttribute('aria-expanded', 'true');
      open = {pop: pop, from: from, target: target};
      var f = pop.querySelector('[aria-pressed="true"]') || pop.querySelector('button');
      if (f) f.focus();
    }
    function current(t) {
      if (t.kind) { var k = kind(t.kind); return {picture: k.picture, tint: k.tint}; }
      return L.matPicture(lib, app.items[t.item]);
    }
    function grid(cur, q) {
      var list = lib.pictures.filter(function (p) { return !q || p.label.toLowerCase().indexOf(q) >= 0; });
      if (!list.length) return '<p class="cra-fine cra-pnone">No picture by that name.</p>';
      return list.map(function (p) {
        return '<button type="button" class="cpx-touch" data-pic="' + esc(p.key) + '" aria-pressed="' + (p.key === cur.picture) + '" aria-label="' + esc(p.label) + '" title="' + esc(p.label) + '">' + CP.svg(p.key, cur.tint, 30) + '</button>';
      }).join('');
    }
    function pickerHTML(t) {
      var cur = current(t), it = t.item && app.items[t.item], title, sub, follow = '', reset = '';
      if (t.kind) {
        var k = kind(t.kind);
        title = k.label; sub = 'Every item that follows ' + k.label;
        if (changed(k)) reset = '<button type="button" class="cra-link" data-act="kreset">Back to ' + esc(picLabel(k.defaultPicture)) + ', its first picture</button>';
      } else {
        title = it.name; sub = it.picture ? 'Its own picture' : (it.tags || []).length ? 'Following ' + kind(it.tags[0]).label : 'No kind yet';
        if ((it.tags || []).length) {
          var f = kind(it.tags[0]);
          follow = '<button type="button" class="cra-follow" data-act="follow" aria-pressed="' + !it.picture + '">' + CP.svg(f.picture, f.tint, 30) +
            '<span>' + esc(f.label) + '’s picture<small>Changes when the kind’s picture changes</small></span></button>';
        }
      }
      return '<header>' + CP.svg(cur.picture, cur.tint, 40) + '<div><b>' + esc(title) + '</b><small>' + esc(sub) + '</small></div>' +
        '<button type="button" class="cra-ib" data-act="pclose" aria-label="Close">' + X + '</button></header><div class="cra-pbody">' + follow +
        '<div class="cra-plab">Picture</div><label class="cra-search"><span class="sr-only">Find a picture</span><input type="search" class="cra-in" data-f="pq" placeholder="Find a picture"></label>' +
        '<div class="cra-pgrid" role="group" aria-label="Pictures">' + grid(cur, '') + '</div>' +
        '<div class="cra-plab">Tint</div><div class="cra-tints" role="group" aria-label="Tints">' + TINTS.map(function (c) {
          return '<button type="button" data-tint="' + c + '" aria-pressed="' + (c === cur.tint) + '" aria-label="Tint ' + c + '"><i style="background:' + c + '"></i></button>';
        }).join('') + '</div>' + reset + '</div>';
    }
    function kindsHTML(it) {
      return (lib.tagGroups || []).map(function (g) {
        return '<h5>' + esc(g.label) + '</h5><div class="cra-kg">' + lib.tags.filter(function (t) { return t.group === g.key; }).map(function (t) {
          return '<button type="button" data-ktoggle="' + esc(t.key) + '" aria-pressed="' + ((it.tags || []).indexOf(t.key) >= 0) + '">' + CP.svg(t.picture, t.tint, 24) + esc(t.label) + '</button>';
        }).join('') + '</div>';
      }).join('');
    }
    /* After a save the picker stays open on the redrawn row or tile, so a
       Director can try a few pictures in a row. */
    function reopen(t, focusSel) {
      if (!open) return;
      var q = open.pop.querySelector('[data-f="pq"]'), qv = q ? q.value : '';
      var from = t.kind ? el.querySelector('[data-tile="' + CSS.escape(t.kind) + '"]') : el.querySelector('.cra-mrow[data-id="' + CSS.escape(t.item) + '"] .cra-pic');
      if (!from) { close(); return; }
      open.from = from; from.setAttribute('aria-expanded', 'true');
      open.pop.innerHTML = pickerHTML(t);
      if (qv) { open.pop.querySelector('[data-f="pq"]').value = qv; open.pop.querySelector('.cra-pgrid').innerHTML = grid(current(t), qv.toLowerCase()); }
      place(open.pop, from);
      var f = focusSel && open.pop.querySelector(focusSel);
      if (f) f.focus();
    }

    /* ---------- Saving ---------- */
    function fail(err) {
      app.status('error', err.message.charAt(0).toUpperCase() + err.message.slice(1));
    }
    function saveTags(it, next) {
      var was = it.tags || [];
      it.tags = next;
      busy[it.id] = true;
      app.status('saving');
      redrawRow(it.id, false);
      app.api('PUT', '/materials/' + encodeURIComponent(it.id), {tags: next}).then(function (got) {
        busy[it.id] = false; it.tags = got.tags || []; app.status('saved'); redrawRow(it.id, true);
      }, function (err) {
        busy[it.id] = false; it.tags = was; fail(err); redrawRow(it.id, false);
      });
    }
    function saveItemPicture(it, pic, focusSel) {
      var was = it.picture;
      it.picture = pic;
      app.status('saving');
      redrawRow(it.id, false); reopen({item: it.id}, focusSel);
      app.api('PUT', '/materials/' + encodeURIComponent(it.id) + '/picture', {picture: pic}).then(function (got) {
        it.picture = got.picture || null; app.status('saved'); redrawRow(it.id, true); reopen({item: it.id}, focusSel);
      }, function (err) {
        it.picture = was; fail(err); redrawRow(it.id, false); reopen({item: it.id}, focusSel);
      });
    }
    /* kinds maps a kind to {picture, tint}, or null for its default. */
    function saveKinds(kinds, t, focusSel, glow) {
      var was = lib.tags.map(function (x) { return {key: x.key, picture: x.picture, tint: x.tint}; });
      Object.keys(kinds).forEach(function (k) {
        var x = kind(k), p = kinds[k] || {picture: x.defaultPicture, tint: x.defaultTint};
        x.picture = p.picture; x.tint = p.tint;
      });
      app.status('saving');
      renderAll(); if (t) reopen(t, focusSel);
      app.api('PUT', '/pictures', {kinds: kinds}).then(function (got) {
        lib.tags = got.tags; app.lib.tags = got.tags;
        app.status('saved');
        renderAll(); if (t) reopen(t, focusSel);
        (glow || []).forEach(function (k) { var b = el.querySelector('[data-tile="' + CSS.escape(k) + '"]'); if (b) b.classList.add('cra-glow'); });
      }, function (err) {
        was.forEach(function (w) { var x = kind(w.key); x.picture = w.picture; x.tint = w.tint; });
        fail(err); renderAll(); if (t) reopen(t, focusSel);
      });
    }
    /* Only the row and the counts redraw, so focus stays where it was. */
    function redrawRow(id, ok) {
      renderFilters();
      var li = el.querySelector('.cra-mrow[data-id="' + CSS.escape(id) + '"]'), it = app.items[id];
      if (!li || !it) return;
      var a = document.activeElement, focus = a && li.contains(a) ? (a.getAttribute('data-tag') ? '[data-tag="' + a.getAttribute('data-tag') + '"]' : a.getAttribute('data-act') ? '[data-act="' + a.getAttribute('data-act') + '"]' : null) : null;
      var tmp = document.createElement('ul');
      tmp.innerHTML = row(it, uses());
      var fresh = tmp.firstChild;
      li.parentNode.replaceChild(fresh, li);
      if (ok) { var m = fresh.querySelector('.cra-mok'); m.textContent = 'Saved'; setTimeout(function () { m.textContent = ''; }, 1500); }
      if (focus) { var b = fresh.querySelector(focus) || fresh.querySelector('.cra-addk'); if (b) b.focus(); }
    }

    /* ---------- Events ---------- */
    el.innerHTML = '<div class="cra-mats-pane"><div class="cra-mhead"><div><h3 class="cra-ph">Materials</h3><p class="cra-fine">Give Armory items a kind, and a recipe can ask for “any herb” instead of one item. ' +
      'Grinding takes herbs, fungi, bones, body parts, scales, gems and Remnant parts; pouring takes liquids, essences, venoms and resins. ' +
      'Pictures follow each kind; open <b>Pictures</b> to change them, or open any item’s picture to give it its own. Everything saves as you click.</p></div>' +
      '<div class="cra-mtools"><button type="button" class="cra-itab" data-act="board" aria-expanded="false" aria-controls="cra-board">Pictures</button>' +
      '<label class="cra-search"><span class="sr-only">Find an item</span><input type="search" class="cra-in" data-f="q" placeholder="Find an item"></label></div></div>' +
      '<div class="cra-boardslot"></div><div class="cra-chips cra-mfilters" role="group" aria-label="Show"></div><div class="cra-mlist"></div></div>';

    function onInput(e) {
      var f = e.target.getAttribute('data-f');
      if (f === 'q') { close(); query = e.target.value.trim().toLowerCase(); shown = PAGE; renderList(); }
      if (f === 'pq' && open && open.target) open.pop.querySelector('.cra-pgrid').innerHTML = grid(current(open.target), e.target.value.trim().toLowerCase());
    }
    function onClick(e) {
      var b = e.target.closest('button');
      if (open && !open.pop.contains(e.target) && b !== open.from) close();
      if (!b || !el.contains(b) || b.disabled) return;
      if (b.dataset.dragged) return;
      var t = open && open.target;
      if (b.hasAttribute('data-pic') && t) {
        var c = current(t), pic = {picture: b.getAttribute('data-pic'), tint: c.tint}, fs = '[data-pic="' + pic.picture + '"]';
        if (t.kind) { var k = {}; k[t.kind] = pic; saveKinds(k, t, fs); } else saveItemPicture(app.items[t.item], pic, fs);
        return;
      }
      if (b.hasAttribute('data-tint') && t) {
        var c2 = current(t), pic2 = {picture: c2.picture, tint: b.getAttribute('data-tint')}, fs2 = '[data-tint="' + pic2.tint + '"]';
        if (!pic2.picture) return;
        if (t.kind) { var k2 = {}; k2[t.kind] = pic2; saveKinds(k2, t, fs2); } else saveItemPicture(app.items[t.item], pic2, fs2);
        return;
      }
      if (b.hasAttribute('data-ktoggle') && open && open.kinds) {
        var it = app.items[open.kinds], tag = b.getAttribute('data-ktoggle'), has = (it.tags || []).indexOf(tag) >= 0;
        var next = has ? it.tags.filter(function (x) { return x !== tag; }) : (it.tags || []).concat([tag]);
        // The server keeps tags in the library's order; so does the page
        next = lib.tags.map(function (x) { return x.key; }).filter(function (x) { return next.indexOf(x) >= 0; });
        saveTags(it, next);
        open.pop.innerHTML = kindsHTML(it);
        open.pop.querySelector('[data-ktoggle="' + tag + '"]').focus();
        return;
      }
      if (b.hasAttribute('data-filter')) { filter = b.getAttribute('data-filter'); shown = PAGE; renderFilters(); renderList(); return; }
      if (b.hasAttribute('data-tile')) {
        var key = b.getAttribute('data-tile');
        if (open && open.from === b) { close(true); return; }
        show(b, 'cra-pick', 'Choose a picture for ' + kind(key).label, pickerHTML({kind: key}), {kind: key});
        return;
      }
      var act = b.getAttribute('data-act'), li = b.closest('.cra-mrow'), item = li && app.items[li.getAttribute('data-id')];
      if (act === 'pick') {
        if (open && open.from === b) { close(true); return; }
        show(b, 'cra-pick', 'Choose a picture for ' + item.name, pickerHTML({item: item.id}), {item: item.id});
        return;
      }
      if (act === 'kinds') {
        if (open && open.from === b) { close(true); return; }
        show(b, 'cra-kmenu', 'Kinds for ' + item.name, kindsHTML(item), null);
        open.kinds = item.id;
        return;
      }
      if (act === 'untag') { var tg = b.getAttribute('data-tag'); saveTags(item, item.tags.filter(function (x) { return x !== tg; })); return; }
      if (act === 'pclose') { close(true); return; }
      if (act === 'follow' && t && t.item) { saveItemPicture(app.items[t.item], null, '[data-act="follow"]'); return; }
      if (act === 'kreset' && t && t.kind) { var r = {}; r[t.kind] = null; saveKinds(r, t, '[data-act="pclose"]'); return; }
      if (act === 'resetall') { var all = {}; lib.tags.filter(changed).forEach(function (x) { all[x.key] = null; }); saveKinds(all); return; }
      if (act === 'board') { close(); board = !board; renderBoard(); var f = board ? el.querySelector('.cra-btile') : $('.cra-itab'); if (f) f.focus(); return; }
      if (act === 'more') { shown += PAGE; renderList(); return; }
      if (act === 'clear') { filter = 'all'; query = ''; $('[data-f="q"]').value = ''; renderFilters(); renderList(); }
    }
    function onKey(e) {
      if (e.key === 'Escape' && open) { e.stopPropagation(); close(true); }
    }
    /* Dragging one kind's tile onto another swaps their pictures in one save.
       The tile lifts and follows the pointer; the one under it shows a
       dashed edge where it will land. */
    function onDown(e) {
      if (e.button !== 0) return;
      var tile = e.target.closest('.cra-btile');
      if (!tile) return;
      drag = {tile: tile, x: e.clientX, y: e.clientY, on: false, id: e.pointerId};
    }
    function under(e) {
      var n = document.elementFromPoint(e.clientX, e.clientY), t = n && n.closest('.cra-btile');
      return t && t !== drag.tile && el.contains(t) ? t : null;
    }
    function onMove(e) {
      if (!drag || e.pointerId !== drag.id) return;
      if (!drag.on) {
        if (Math.abs(e.clientX - drag.x) + Math.abs(e.clientY - drag.y) < 6) return;
        drag.on = true; close();
        drag.tile.classList.add('is-lift', 'cpx-held');
        drag.tile.dataset.dragged = '1';
        try { drag.tile.setPointerCapture(e.pointerId); } catch (err) { /* the drag still works without capture */ }
      }
      e.preventDefault();
      drag.tile.style.transform = 'translate(' + (e.clientX - drag.x) + 'px,' + (e.clientY - drag.y) + 'px) rotate(-1.5deg)';
      Array.prototype.forEach.call(el.querySelectorAll('.cra-btile.is-drop'), function (n) { n.classList.remove('is-drop'); });
      var t = under(e); if (t) t.classList.add('is-drop');
    }
    function onUp(e) {
      if (!drag || e.pointerId !== drag.id) return;
      var d = drag; drag = null;
      if (!d.on) return;
      var t = under(e);
      d.tile.classList.remove('is-lift', 'cpx-held'); d.tile.style.transform = '';
      setTimeout(function () { delete d.tile.dataset.dragged; }, 0);
      if (!t) { renderBoard(); return; }
      var a = d.tile.getAttribute('data-tile'), b = t.getAttribute('data-tile'), ka = kind(a), kb = kind(b), kinds = {};
      kinds[a] = {picture: kb.picture, tint: kb.tint};
      kinds[b] = {picture: ka.picture, tint: ka.tint};
      saveKinds(kinds, null, null, [a, b]);
    }
    function onOutside(e) { if (open && !el.contains(e.target)) close(); }
    function onResize() { if (open) place(open.pop, open.from); }

    el.addEventListener('input', onInput);
    el.addEventListener('click', onClick);
    el.addEventListener('keydown', onKey);
    el.addEventListener('pointerdown', onDown);
    el.addEventListener('pointermove', onMove);
    el.addEventListener('pointerup', onUp);
    el.addEventListener('pointercancel', onUp);
    document.addEventListener('click', onOutside);
    window.addEventListener('resize', onResize);

    renderAll();
    return {
      // Recipes may have changed while this tab was hidden
      shown: function () { renderAll(); },
      destroy: function () {
        close();
        document.removeEventListener('click', onOutside);
        window.removeEventListener('resize', onResize);
      }
    };
  }
  return {mount: mount};
})();
