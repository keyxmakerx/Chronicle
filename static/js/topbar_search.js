/**
 * topbar_search.js -- Inline Topbar Search
 *
 * When the search trigger in the topbar is clicked (or Ctrl+K pressed):
 * 1. The #topbar-content area slides up/fades out
 * 2. The #topbar-search area appears with an input field
 * 3. Before typing, it lists the pages this browser opened last
 * 4. Typing performs live search; matching commands from the command palette
 *    follow the pages, and a search with no page match offers to create one
 *    (for those who may create pages)
 * 5. Escape or clicking outside closes search and restores content
 *
 * Falls back to the existing search modal (search_modal.js) when the
 * topbar elements aren't present (e.g., non-campaign pages).
 */
(function () {
  'use strict';

  var content = null;   // #topbar-content
  var search = null;    // #topbar-search
  var input = null;     // #topbar-search-input
  var resultsCt = null; // #topbar-search-results
  var trigger = null;   // #topbar-search-trigger
  var isActive = false;
  var debounceTimer = null;
  var abortController = null;
  var results = [];
  var activeIndex = -1;

  function init() {
    content = document.getElementById('topbar-content');
    search = document.getElementById('topbar-search');
    input = document.getElementById('topbar-search-input');
    resultsCt = document.getElementById('topbar-search-results');
    trigger = document.getElementById('topbar-search-trigger');

    if (!content || !search || !input) return false;

    input.addEventListener('input', onInput);
    input.addEventListener('keydown', onKeydown);

    // Close when clicking outside the search area.
    document.addEventListener('click', function (e) {
      if (!isActive) return;
      if (e.target.closest('#topbar-search') || e.target.closest('#topbar-search-trigger')) return;
      closeSearch();
    });

    return true;
  }

  function openSearch() {
    if (isActive) {
      input.focus();
      return;
    }
    isActive = true;
    content.classList.add('hidden');
    search.classList.remove('hidden');
    search.classList.add('flex');
    search.parentElement.setAttribute('data-tb-searching', '');
    if (trigger) trigger.classList.add('hidden');
    input.value = '';
    results = [];
    activeIndex = -1;
    showRecent();
    // Small delay so the transition feels smooth.
    requestAnimationFrame(function () { input.focus(); });
  }

  function closeSearch() {
    if (!isActive) return;
    isActive = false;
    search.classList.add('hidden');
    search.classList.remove('flex');
    search.parentElement.removeAttribute('data-tb-searching');
    content.classList.remove('hidden');
    if (trigger) trigger.classList.remove('hidden');
    input.value = '';
    results = [];
    activeIndex = -1;
    hideResults();
    if (debounceTimer) clearTimeout(debounceTimer);
    if (abortController) abortController.abort();
  }

  function onInput() {
    var query = input.value.trim();
    if (debounceTimer) clearTimeout(debounceTimer);

    if (query.length < 2) {
      results = [];
      activeIndex = -1;
      if (query.length === 0) showRecent();
      else showHint('Type at least 2 characters...');
      return;
    }

    debounceTimer = setTimeout(function () { doSearch(query); }, 200);
  }

  function onKeydown(e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      closeSearch();
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (results.length > 0) {
        activeIndex = Math.min(activeIndex + 1, results.length - 1);
        renderResults();
      }
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (results.length > 0) {
        activeIndex = Math.max(activeIndex - 1, 0);
        renderResults();
      }
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      if (activeIndex >= 0 && activeIndex < results.length) {
        runResult(results[activeIndex]);
      }
    }
  }

  function getCampaignId() {
    var match = window.location.pathname.match(/\/campaigns\/([^/]+)/);
    return match ? match[1] : null;
  }

  function doSearch(query) {
    if (abortController) abortController.abort();
    abortController = new AbortController();

    var campaignId = getCampaignId();
    if (!campaignId) return;

    var url = '/campaigns/' + encodeURIComponent(campaignId) +
              '/entities/search?q=' + encodeURIComponent(query);
    var signal = abortController.signal;

    Chronicle.apiFetch(url, { signal: signal })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        var pages = (data.results || []).map(function (e) {
          return {
            name: e.name,
            type: e.type_name || '',
            icon: e.type_icon || 'fa-file',
            color: e.type_color || '#6b7280',
            url: resultUrl(e, campaignId),
          };
        });
        results = pages.concat(matchingCommands(query));
        if (!pages.length && canCreate()) results.push(createRow(query, campaignId));
        activeIndex = results.length > 0 ? 0 : -1;
        renderResults();
      })
      .catch(function (err) {
        if (err.name !== 'AbortError') {
          showHint('Search failed.');
        }
      });
  }

  // resultUrl is where a result opens. Search returns pages, calendar events,
  // timelines, maps, sessions and system content, and each carries its own
  // url; only a same-site path is followed, falling back to the page route.
  function resultUrl(e, campaignId) {
    var u = typeof e.url === 'string' ? e.url : '';
    if (u.charAt(0) === '/' && u.charAt(1) !== '/' && u.charAt(1) !== '\\') return u;
    return '/campaigns/' + encodeURIComponent(campaignId) + '/entities/' + encodeURIComponent(e.id);
  }

  // --- Rows that are not pages ---

  var ICON_RE = /^fa-[a-z0-9-]{1,40}$/;
  var MAX_RECENT = 5;
  var MAX_COMMANDS = 4;

  // The pages this browser opened last, as recent_entities.js keeps them
  // (key chronicle-recent-{campaignId}, newest first).
  function recentRows(campaignId) {
    var list = [];
    try { list = JSON.parse(localStorage.getItem('chronicle-recent-' + campaignId) || '[]'); } catch (e) { list = []; }
    if (!Array.isArray(list)) return [];
    return list.filter(function (item) { return item && item.id && item.name; }).slice(0, MAX_RECENT).map(function (item) {
      return {
        name: String(item.name),
        type: 'Recent',
        icon: 'fa-clock-rotate-left',
        color: '#6b7280',
        url: '/campaigns/' + encodeURIComponent(campaignId) + '/entities/' + encodeURIComponent(item.id),
      };
    });
  }

  function showRecent() {
    var campaignId = getCampaignId();
    results = campaignId ? recentRows(campaignId) : [];
    activeIndex = -1;
    if (!results.length) { hideResults(); return; }
    renderResults('Recent');
  }

  // Commands come from the command palette (command_palette.js), so search
  // offers exactly what the palette does, for this viewer and this page.
  function matchingCommands(query) {
    if (!window.Chronicle || typeof Chronicle.paletteCommands !== 'function') return [];
    var q = query.toLowerCase();
    var out = [];
    Chronicle.paletteCommands().forEach(function (c) {
      if (out.length >= MAX_COMMANDS || !c || !c.label) return;
      // Search is already open, and a new page needs the create right.
      if (c.id === 'search' || (c.id === 'new-page' && !canCreate())) return;
      if (c.label.toLowerCase().indexOf(q) === -1) return;
      out.push({
        name: c.label,
        type: 'Command',
        icon: ICON_RE.test(c.icon || '') ? c.icon : 'fa-terminal',
        color: '#6b7280',
        url: c.href || '',
        action: c.action || null,
      });
    });
    return out;
  }

  // Creating a page needs the scribe role or above; the server checks it again.
  function canCreate() {
    return !!(search && search.getAttribute('data-can-create') === 'true');
  }

  function createRow(query, campaignId) {
    return {
      name: 'Create a page named \u201c' + query + '\u201d',
      type: 'New page',
      icon: 'fa-plus',
      color: '#6b7280',
      url: '/campaigns/' + encodeURIComponent(campaignId) + '/entities/new?name=' + encodeURIComponent(query),
    };
  }

  function runResult(r) {
    if (!r) return;
    if (r.action) {
      closeSearch();
      r.action();
      return;
    }
    if (r.url) {
      window.location.href = r.url;
      closeSearch();
    }
  }

  function renderResults(heading) {
    if (results.length === 0) {
      showHint('No results found.');
      return;
    }

    var html = heading ? '<div class="px-4 pt-2.5 pb-1 text-[11px] font-semibold uppercase tracking-wider text-fg-muted">' + Chronicle.escapeHtml(heading) + '</div>' : '';
    for (var i = 0; i < results.length; i++) {
      var r = results[i];
      var active = i === activeIndex ? ' bg-surface-alt' : '';
      html += '<a href="' + Chronicle.escapeAttr(r.url || '#') + '" data-index="' + i + '" class="flex items-center gap-3 px-4 py-2.5 text-sm hover:bg-surface-alt transition-colors' + active + '">' +
        '<span class="w-5 h-5 rounded flex items-center justify-center text-[10px] shrink-0" style="background-color:' + Chronicle.escapeAttr(r.color) + '20;color:' + Chronicle.escapeAttr(r.color) + '">' +
          '<i class="fa-solid ' + Chronicle.escapeAttr(r.icon) + '"></i></span>' +
        '<span class="flex-1 truncate text-fg">' + Chronicle.escapeHtml(r.name) + '</span>' +
        '<span class="text-xs text-fg-muted shrink-0">' + Chronicle.escapeHtml(r.type) + '</span>' +
      '</a>';
    }

    // "View all results" link to the dedicated search page.
    var campaignId = getCampaignId();
    var q = input ? input.value.trim() : '';
    if (campaignId && q.length >= 2) {
      html += '<a href="/campaigns/' + Chronicle.escapeAttr(campaignId) + '/search?q=' + encodeURIComponent(q) + '" ' +
        'class="flex items-center justify-center gap-1.5 px-4 py-2 text-xs font-medium text-accent hover:bg-accent/10 border-t border-edge transition-colors">' +
        '<i class="fa-solid fa-arrow-right text-[10px]"></i> View all results</a>';
    }

    resultsCt.innerHTML = html;
    resultsCt.classList.remove('hidden');

    // Click handler for results.
    resultsCt.querySelectorAll('a').forEach(function (a) {
      a.addEventListener('click', function (e) {
        var idx = a.getAttribute('data-index');
        var r = idx === null ? null : results[parseInt(idx, 10)];
        if (r && r.action) {
          e.preventDefault();
          runResult(r);
          return;
        }
        closeSearch();
      });
    });
  }

  function showHint(msg) {
    resultsCt.innerHTML = '<div class="px-4 py-3 text-sm text-fg-muted">' + Chronicle.escapeHtml(msg) + '</div>';
    resultsCt.classList.remove('hidden');
  }

  function hideResults() {
    if (resultsCt) {
      resultsCt.innerHTML = '';
      resultsCt.classList.add('hidden');
    }
  }

  // --- Initialization ---

  // Wait for DOM, then try to init topbar search.
  document.addEventListener('DOMContentLoaded', function () {
    var topbarReady = init();

    if (topbarReady) {
      // Override Chronicle.openSearch to use the inline topbar search.
      window.Chronicle = window.Chronicle || {};
      Chronicle.openSearch = openSearch;
      Chronicle.closeSearch = closeSearch;
    }
    // If topbar elements don't exist, search_modal.js keeps its own
    // Chronicle.openSearch — the modal works as before.
  });

  // Close on navigation.
  window.addEventListener('chronicle:navigated', function () {
    if (isActive) closeSearch();
  });
})();
