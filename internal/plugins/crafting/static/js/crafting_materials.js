/**
 * crafting_materials.js -- crafting tags on Armory items (window.CraftingMaterials)
 *
 * The workshop's Materials tab: every Armory item the Director can see, with
 * the nine crafting tags as switches. A tag is what lets a recipe ask for
 * "any herb" and what tells a step whether an item fits (grinding takes
 * herbs and bones, pouring takes liquids). Each click saves that one item at
 * once; the items themselves stay the Armory's to edit.
 */
window.CraftingMaterials = (function () {
  'use strict';
  var L = window.CraftingLib, esc = L.esc, PAGE = 60;

  function mount(el, app) {
    var lib = app.lib, filter = 'all', query = '', shown = PAGE, busy = {};
    function $(s) { return el.querySelector(s); }

    /* How many recipes ask for each item, so a Director sees what a tag
       change touches before making it. */
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
    function icon(it) {
      var t = (it.tags || [])[0], CR = window.CraftingRecipes;
      return t ? CR.shape(CR.TAG_SHAPE[t], CR.TAG_COLOR[t], 30) : CR.shape('', '', 30);
    }
    function renderFilters() {
      var count = {all: app.itemList.length, none: 0};
      app.itemList.forEach(function (it) {
        if (!(it.tags || []).length) count.none++;
        (it.tags || []).forEach(function (t) { count[t] = (count[t] || 0) + 1; });
      });
      var list = [{key: 'all', label: 'All'}, {key: 'none', label: 'No tag yet'}].concat(lib.tags);
      $('.cra-mfilters').innerHTML = list.map(function (t) {
        return '<button type="button" class="cra-chip cra-chip-sm" data-filter="' + esc(t.key) + '" aria-pressed="' + (filter === t.key) + '">' + esc(t.label) +
          ' <small>' + (count[t.key] || 0) + '</small></button>';
      }).join('');
    }
    function row(it, n) {
      var tags = it.tags || [];
      return '<li class="cra-mrow" data-id="' + esc(it.id) + '">' + icon(it) + '<div class="cra-mname"><b>' + esc(it.name) + '</b><small>' +
        (n[it.id] ? 'In ' + n[it.id] + (n[it.id] > 1 ? ' recipes' : ' recipe') : 'Not in a recipe') + '</small></div>' +
        '<div class="cra-mtags" role="group" aria-label="Crafting tags for ' + esc(it.name) + '">' + lib.tags.map(function (t) {
          return '<button type="button" class="cra-tagbtn" data-tag="' + esc(t.key) + '" aria-pressed="' + (tags.indexOf(t.key) >= 0) + '"' + (busy[it.id] ? ' disabled' : '') + '>' + esc(t.label) + '</button>';
        }).join('') + '</div><span class="cra-mok" aria-hidden="true"></span></li>';
    }
    function renderList() {
      var n = uses(), hits = app.itemList.filter(matches), box = $('.cra-mlist');
      if (!app.itemList.length) {
        box.innerHTML = '<div class="cra-empty">The Armory has no items yet. Add the things your world is made of there, then tag them here.</div>';
        return;
      }
      if (!hits.length) {
        box.innerHTML = '<div class="cra-empty">Nothing matches. <button type="button" class="cra-link" data-act="clear">Show every item</button></div>';
        return;
      }
      box.innerHTML = '<ul class="cra-mrows">' + hits.slice(0, shown).map(function (it) { return row(it, n); }).join('') + '</ul>' +
        (hits.length > shown ? '<div class="cra-row"><button type="button" class="btn btn-secondary btn-sm" data-act="more">Show more (' + (hits.length - shown) + ' left)</button></div>' : '');
    }
    function save(it, next, li) {
      var was = it.tags || [];
      it.tags = next;
      busy[it.id] = true;
      app.status('saving');
      if (li) Array.prototype.forEach.call(li.querySelectorAll('.cra-tagbtn'), function (b) { b.setAttribute('aria-pressed', String(next.indexOf(b.getAttribute('data-tag')) >= 0)); b.disabled = true; });
      app.api('PUT', '/materials/' + encodeURIComponent(it.id), {tags: next}).then(function (got) {
        busy[it.id] = false;
        it.tags = got.tags || [];
        app.status('saved');
        redraw(it.id, true);
      }, function (err) {
        busy[it.id] = false;
        it.tags = was;
        app.status('error', err.message.charAt(0).toUpperCase() + err.message.slice(1));
        redraw(it.id, false);
      });
    }
    /* Only the row and the counts redraw, so focus stays on the tag the
       Director just pressed. */
    function redraw(id, ok) {
      renderFilters();
      var li = el.querySelector('.cra-mrow[data-id="' + CSS.escape(id) + '"]'), it = app.items[id];
      if (!li || !it) return;
      var focus = document.activeElement && li.contains(document.activeElement) ? document.activeElement.getAttribute('data-tag') : null;
      var tmp = document.createElement('ul');
      tmp.innerHTML = row(it, uses());
      var fresh = tmp.firstChild;
      li.parentNode.replaceChild(fresh, li);
      if (ok) { var m = fresh.querySelector('.cra-mok'); m.textContent = 'Saved'; setTimeout(function () { m.textContent = ''; }, 1500); }
      if (focus) { var b = fresh.querySelector('[data-tag="' + focus + '"]'); if (b) b.focus(); }
    }

    el.innerHTML = '<div class="cra-mats-pane"><div class="cra-mhead"><div><h3 class="cra-ph">Materials</h3><p class="cra-fine">Give Armory items a crafting tag, and a recipe can ask for “any herb” instead of one item. ' +
      'Steps use tags too: grinding takes herbs, bones, body parts and Remnant parts; pouring takes liquids and magic essences. Each tag saves as you click it.</p></div>' +
      '<label class="cra-search"><span class="sr-only">Find an item</span><input type="search" class="cra-in" data-f="q" placeholder="Find an item"></label></div>' +
      '<div class="cra-chips cra-mfilters" role="group" aria-label="Show"></div><div class="cra-mlist"></div></div>';

    el.addEventListener('input', function (e) {
      if (e.target.getAttribute('data-f') !== 'q') return;
      query = e.target.value.trim().toLowerCase(); shown = PAGE; renderList();
    });
    el.addEventListener('click', function (e) {
      var b = e.target.closest('button');
      if (!b || !el.contains(b)) return;
      if (b.hasAttribute('data-filter')) { filter = b.getAttribute('data-filter'); shown = PAGE; renderFilters(); renderList(); return; }
      var act = b.getAttribute('data-act');
      if (act === 'more') { shown += PAGE; renderList(); return; }
      if (act === 'clear') { filter = 'all'; query = ''; $('[data-f="q"]').value = ''; renderFilters(); renderList(); return; }
      if (b.hasAttribute('data-tag')) {
        var li = b.closest('.cra-mrow'), it = app.items[li.getAttribute('data-id')], t = b.getAttribute('data-tag');
        if (!it || busy[it.id]) return;
        var has = (it.tags || []).indexOf(t) >= 0;
        var next = has ? it.tags.filter(function (x) { return x !== t; }) : (it.tags || []).concat([t]);
        // The server keeps tags in the library's order; so does the page
        next = lib.tags.map(function (x) { return x.key; }).filter(function (k) { return next.indexOf(k) >= 0; });
        save(it, next, li);
      }
    });

    renderFilters();
    renderList();
    return {
      // Recipes may have changed while this tab was hidden
      shown: function () { renderFilters(); renderList(); },
      destroy: function () {}
    };
  }
  return {mount: mount};
})();
