/**
 * topbar_search.js -- Inline Topbar Search
 *
 * When the search trigger in the topbar is clicked (or Ctrl+K pressed):
 * 1. The #topbar-content area slides up/fades out
 * 2. The #topbar-search area appears with an input field
 * 3. Before typing, it lists the pages this browser opened last
 * 4. Typing performs live search; the viewer's notes with a matching title
 *    and matching commands from the command palette follow the pages, and a
 *    search that finds no page with the words in its name offers to create
 *    one (for those who may create pages)
 * 5. Escape, Ctrl+K again or clicking outside closes search and restores
 *    content
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
  var notesLoad = null; // the viewer's notes, fetched once per opening

  function init() {
    content = document.getElementById('topbar-content');
    search = document.getElementById('topbar-search');
    input = document.getElementById('topbar-search-input');
    resultsCt = document.getElementById('topbar-search-results');
    trigger = document.getElementById('topbar-search-trigger');

    if (!content || !search || !input) return false;

    input.addEventListener('input', onInput);
    input.addEventListener('keydown', onKeydown);

    // Close when clicking outside the search area. The click that opened the
    // search bubbles here too, so every control that opens it is skipped:
    // the icon and the header's search box widget (data-search-open), or the
    // search would close in the same click that opened it.
    document.addEventListener('click', function (e) {
      if (!isActive) return;
      if (e.target.closest('#topbar-search, #topbar-search-trigger, [data-search-open]')) return;
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
    notesLoad = null;
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

    Promise.all([
      Chronicle.apiFetch(url, { signal: signal }).then(function (r) { return r.json(); }),
      loadNotes(campaignId),
    ])
      .then(function (loaded) {
        var data = loaded[0] || {};
        var pages = (data.results || []).map(function (e) {
          return {
            name: e.name,
            type: e.type_name || '',
            icon: e.type_icon || '',
            color: e.type_color || '',
            url: resultUrl(e, campaignId),
            // The page's own setting, or its category, hides it from players.
            locked: e.restricted === 'dm_only',
          };
        });
        results = pages.concat(matchingNotes(loaded[1], query, campaignId), matchingCommands(query));
        if (canCreate() && !pageNamed(pages, query)) results.push(createRow(query, campaignId));
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

  var MAX_NOTES = 5;
  var PAGE_URL_RE = /\/entities\/[^/?#]+$/;

  // The viewer's own notes, asked for once per opening and reused for each
  // keystroke. A campaign without notes, or a viewer who may not read them,
  // simply has none to list.
  function loadNotes(campaignId) {
    if (!notesLoad) {
      notesLoad = Chronicle.apiFetch('/campaigns/' + encodeURIComponent(campaignId) + '/notes')
        .then(function (r) { return r.json(); })
        .then(function (list) { return Array.isArray(list) ? list : []; })
        .catch(function () { return []; });
    }
    return notesLoad;
  }

  // Notes match on their title. Folders and archived notes stay out, as they
  // do everywhere a note is picked by name.
  function matchingNotes(notes, query, campaignId) {
    var q = query.toLowerCase();
    var out = [];
    (notes || []).forEach(function (n) {
      if (out.length >= MAX_NOTES || !n || n.isFolder || n.archived || !n.title) return;
      if (String(n.title).toLowerCase().indexOf(q) === -1) return;
      var journalUrl = '/campaigns/' + encodeURIComponent(campaignId) + '/journal/' + encodeURIComponent(n.id);
      out.push({
        name: String(n.title),
        type: 'Note',
        // A jot belongs to a page and opens in that page's notes panel, so
        // it has no address of its own.
        url: n.entityId ? '' : journalUrl,
        action: function () { openNote(n, journalUrl); },
      });
    });
    return out;
  }

  function openNote(n, journalUrl) {
    if (n.entityId) {
      window.dispatchEvent(new CustomEvent('chronicle:open-note', { detail: { noteId: n.id } }));
      return;
    }
    // Already in the Journal: open the note in place instead of reloading.
    if (Chronicle.openJournalNote && Chronicle.openJournalNote(n.id)) return;
    window.location.href = journalUrl;
  }

  // pageNamed says whether a page among the results carries the query in its
  // name. Search also matches what a page says, so a page that only mentions
  // the words is not the page the searcher was looking for, and must not hide
  // the offer to create it.
  function pageNamed(pages, query) {
    var q = query.toLowerCase();
    return pages.some(function (p) {
      return PAGE_URL_RE.test(p.url || '') && String(p.name || '').toLowerCase().indexOf(q) !== -1;
    });
  }

  // A command's other names (its words) find it as well as its label.
  function wordsFind(words, q) {
    var ws = String(words || '').toLowerCase().split(/\s+/);
    for (var i = 0; i < ws.length; i++) {
      if (ws[i] && ws[i].indexOf(q) === 0) return true;
    }
    return false;
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
      if (c.label.toLowerCase().indexOf(q) === -1 && !wordsFind(c.words, q)) return;
      out.push({
        name: c.label,
        type: 'Command',
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
    var url = '/campaigns/' + encodeURIComponent(campaignId) + '/entities/new?name=' + encodeURIComponent(query);
    return {
      name: 'Create a page named \u201c' + query + '\u201d',
      type: 'New page',
      url: url,
      // The small sheet over the results' page; the full screen if it can't open.
      action: function () {
        if (!(window.Chronicle && Chronicle.newPage && Chronicle.newPage.open(url))) window.location.href = url;
      },
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

  // The category's icon in front of a page's name. The row always carries
  // the slot: the site's CSS hides it unless the campaign shows icons beside
  // names, and a row with no icon (a note, a command) keeps an empty slot so
  // every name starts at the same edge. Icon and colour are checked here
  // because both land in markup.
  var KIND_ICON_RE = /^fa-[a-z0-9-]{1,40}$/;
  var KIND_COLOUR_RE = /^#[0-9a-fA-F]{6}$/;
  function kindIcon(icon, colour) {
    if (!KIND_ICON_RE.test(icon || '')) return '<span class="kind-ic" aria-hidden="true"></span>';
    return '<i class="fa-solid ' + icon + ' kind-ic"' +
      (KIND_COLOUR_RE.test(colour || '') ? ' style="--kind-colour:' + colour.toLowerCase() + '"' : '') + ' aria-hidden="true"></i>';
  }

  function renderResults(heading) {
    if (results.length === 0) {
      showHint('No results found.');
      return;
    }

    var html = heading ? '<div class="px-4 pt-2.5 pb-1 text-[11px] font-semibold uppercase tracking-wider text-fg-muted">' + Chronicle.escapeHtml(heading) + '</div>' : '';
    // A row is its name, then what kind of thing it is as a quiet word at the
    // end. A category's icon may stand in front when the campaign asks for it
    // (see kindIcon); by default the name is what tells two rows apart.
    // A page players can't see carries the same lock as the other page lists.
    var lockLabel = (Chronicle.gmTerm ? Chronicle.gmTerm() : 'GM') + ' team only';
    for (var i = 0; i < results.length; i++) {
      var r = results[i];
      var active = i === activeIndex ? ' bg-surface-alt' : '';
      html += '<a href="' + Chronicle.escapeAttr(r.url || '#') + '" data-index="' + i + '" class="flex items-baseline gap-3 px-4 py-2.5 text-sm hover:bg-surface-alt transition-colors' + active + '">' +
        '<span class="flex-1 min-w-0 flex items-baseline gap-1.5">' + kindIcon(r.icon, r.color) + '<span class="truncate text-fg">' + Chronicle.escapeHtml(r.name) + '</span>' +
          (r.locked ? '<i class="cb-dm fa-solid fa-lock shrink-0 text-[10px] text-fg-muted" role="img" aria-label="' + lockLabel + '" title="' + lockLabel + '"></i>' : '') + '</span>' +
        '<span class="text-xs text-fg-muted shrink-0 max-w-[45%] truncate">' + Chronicle.escapeHtml(r.type) + '</span>' +
      '</a>';
    }

    // "View all results" link to the dedicated search page.
    var campaignId = getCampaignId();
    var q = input ? input.value.trim() : '';
    if (campaignId && q.length >= 2) {
      html += '<a href="/campaigns/' + Chronicle.escapeAttr(campaignId) + '/search?q=' + encodeURIComponent(q) + '" ' +
        'class="flex items-center justify-center px-4 py-2 text-xs font-medium text-accent hover:bg-accent/10 border-t border-edge transition-colors">' +
        'View all results</a>';
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
      // Ctrl+K (search_modal.js owns the key) opens and closes this search
      // too, so the shortcut and the header button are one search.
      Chronicle.toggleSearch = function () { if (isActive) closeSearch(); else openSearch(); };
    }
    // If topbar elements don't exist, search_modal.js keeps its own
    // Chronicle.openSearch — the modal works as before.
  });

  // Close on navigation.
  window.addEventListener('chronicle:navigated', function () {
    if (isActive) closeSearch();
  });
})();
