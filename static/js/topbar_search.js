/**
 * topbar_search.js -- Inline Topbar Search
 *
 * When the search trigger in the topbar is clicked (or Ctrl+K pressed):
 * 1. The #topbar-content area slides up/fades out
 * 2. The #topbar-search area appears with an input field
 * 3. Typing performs live search with results in a dropdown
 * 4. Escape or clicking outside closes search and restores content
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
    if (trigger) trigger.classList.add('hidden');
    input.value = '';
    results = [];
    activeIndex = -1;
    hideResults();
    // Small delay so the transition feels smooth.
    requestAnimationFrame(function () { input.focus(); });
  }

  function closeSearch() {
    if (!isActive) return;
    isActive = false;
    search.classList.add('hidden');
    search.classList.remove('flex');
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
      if (query.length === 0) hideResults();
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
        var r = results[activeIndex];
        if (r.url) {
          window.location.href = r.url;
          closeSearch();
        }
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
        results = (data.results || []).map(function (e) {
          return {
            name: e.name,
            type: e.type_name || '',
            icon: e.type_icon || 'fa-file',
            color: e.type_color || '#6b7280',
            url: '/campaigns/' + campaignId + '/entities/' + e.id,
          };
        });
        activeIndex = results.length > 0 ? 0 : -1;
        renderResults();
      })
      .catch(function (err) {
        if (err.name !== 'AbortError') {
          showHint('Search failed.');
        }
      });
  }

  function renderResults() {
    if (results.length === 0) {
      showHint('No results found.');
      return;
    }

    var html = '';
    for (var i = 0; i < results.length; i++) {
      var r = results[i];
      var active = i === activeIndex ? ' bg-surface-alt' : '';
      html += '<a href="' + Chronicle.escapeAttr(r.url) + '" class="flex items-center gap-3 px-4 py-2.5 text-sm hover:bg-surface-alt transition-colors' + active + '">' +
        '<span class="w-5 h-5 rounded flex items-center justify-center text-[10px] shrink-0" style="background-color:' + Chronicle.escapeAttr(r.color) + '20;color:' + Chronicle.escapeAttr(r.color) + '">' +
          '<i class="fa-solid ' + Chronicle.escapeAttr(r.icon) + '"></i></span>' +
        '<span class="flex-1 truncate text-fg">' + Chronicle.escapeHtml(r.name) + '</span>' +
        '<span class="text-xs text-fg-muted shrink-0">' + Chronicle.escapeHtml(r.type) + '</span>' +
      '</a>';
    }

    // "View all results" link to the dedicated search page.
    var campaignId = getCampaignId();
    var q = input ? input.value.trim() : '';
    if (campaignId && q) {
      html += '<a href="/campaigns/' + Chronicle.escapeAttr(campaignId) + '/search?q=' + encodeURIComponent(q) + '" ' +
        'class="flex items-center justify-center gap-1.5 px-4 py-2 text-xs font-medium text-accent hover:bg-accent/10 border-t border-edge transition-colors">' +
        '<i class="fa-solid fa-arrow-right text-[10px]"></i> View all results</a>';
    }

    resultsCt.innerHTML = html;
    resultsCt.classList.remove('hidden');

    // Click handler for results.
    resultsCt.querySelectorAll('a').forEach(function (a) {
      a.addEventListener('click', function () {
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
