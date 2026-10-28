/**
 * crafting_book.js -- a player's recipe book (window.CraftingBook)
 *
 * What a player sees of crafting: the recipes their characters know, each
 * as the paper page the Director previews beside the editor. The server
 * sends only pages this player may read (no drafts, no secret rules, no
 * names of items they haven't seen), so this file only draws them.
 */
window.CraftingBook = (function () {
  'use strict';
  var L = window.CraftingLib, esc = L.esc;

  function mount(el, app, pages) {
    var lib = app.lib, sel = 0, query = '';
    function $(s) { return el.querySelector(s); }
    function hits() {
      return pages.filter(function (p) { return !query || p.name.toLowerCase().indexOf(query) >= 0; });
    }
    function renderList() {
      var list = hits(), ul = $('.cra-bkl');
      if (sel >= list.length) sel = 0;
      ul.innerHTML = list.length ? list.map(function (p, i) {
        return '<li><button type="button" class="cra-ritem" data-i="' + i + '" aria-current="' + (i === sel) + '"><span class="cra-rtxt"><b>' + esc(p.name) + '</b><small>' +
          esc(p.kind) + (p.bench ? ' · ' + esc(p.bench) : '') + '</small></span></button></li>';
      }).join('') : '<li class="cra-fine">Nothing matches.</li>';
      var p = list[sel];
      $('.cra-bkpage').innerHTML = p ? L.bookHTML(lib, p) : '';
    }

    if (!pages.length) {
      el.innerHTML = '<div class="cra"><div class="cra-empty cra-bkempty"><h3 class="cra-ph">No recipes yet</h3><p>Your characters don’t know any recipes yet. ' +
        'Your Director adds recipes here as your characters learn them.</p></div></div>';
      return {destroy: function () {}};
    }
    el.innerHTML = '<div class="cra"><div class="cra-bkc"><div class="cra-bk"><aside class="cra-rlist"><h3 class="cra-ph">Recipes</h3>' +
      (pages.length > 8 ? '<label class="cra-search"><span class="sr-only">Find a recipe</span><input type="search" class="cra-in" data-f="q" placeholder="Find a recipe"></label>' : '') +
      '<ul class="cra-rl cra-bkl"></ul></aside><div class="cra-bkpage" aria-live="polite"></div></div></div></div>';
    el.addEventListener('click', function (e) {
      var b = e.target.closest('.cra-ritem');
      if (!b) return;
      sel = +b.getAttribute('data-i');
      renderList();
      var again = el.querySelector('.cra-ritem[data-i="' + sel + '"]');
      if (again) again.focus();
    });
    el.addEventListener('input', function (e) {
      if (e.target.getAttribute('data-f') !== 'q') return;
      query = e.target.value.trim().toLowerCase(); sel = 0; renderList();
    });
    renderList();
    return {destroy: function () {}};
  }
  return {mount: mount};
})();
