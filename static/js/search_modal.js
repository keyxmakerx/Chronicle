/**
 * search_modal.js -- Quick Search Modal (Ctrl+K / Cmd+K)
 *
 * Opens a centered search modal for finding entities within the current
 * campaign. Uses the existing /campaigns/:id/entities/search JSON endpoint.
 *
 * Features:
 *   - Ctrl+K / Cmd+K keyboard shortcut (global)
 *   - Click the topbar search trigger to open
 *   - Debounced search (200ms)
 *   - Keyboard navigation: Arrow keys, Enter to open, Escape to close
 *   - Results grouped by entity type with icon + color
 *   - Click outside or press Escape to dismiss
 */
(function () {
  'use strict';

  // --- State ---
  var overlay = null;
  var input = null;
  var resultsList = null;
  var footer = null;
  var activeIndex = -1;
  var results = [];
  var debounceTimer = null;
  var abortController = null;
  var isOpen = false;

  // --- Campaign ID Extraction ---

  /**
   * Extract the campaign ID from the current URL.
   * Pattern: /campaigns/:id/...
   * Returns empty string if not in a campaign context.
   */
  function getCampaignId() {
    var parts = window.location.pathname.split('/');
    if (parts.length >= 3 && parts[1] === 'campaigns' && parts[2] !== '' &&
        parts[2] !== 'new' && parts[2] !== 'picker') {
      return parts[2];
    }
    return '';
  }

  // --- DOM Construction ---

  /**
   * Build the modal DOM elements. Called once on first open, then reused.
   */
  function buildModal() {
    // Backdrop overlay.
    overlay = document.createElement('div');
    overlay.className = 'fixed inset-0 z-[100] flex items-start justify-center pt-[15vh] bg-black/60';
    overlay.style.display = 'none';
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) close();
    });

    // Modal container.
    var modal = document.createElement('div');
    modal.className = 'w-full max-w-lg bg-surface border border-edge rounded-lg shadow-2xl overflow-hidden';
    modal.addEventListener('click', function (e) {
      e.stopPropagation();
    });

    // Search input area.
    var inputWrap = document.createElement('div');
    inputWrap.className = 'flex items-center px-4 border-b border-edge';

    var icon = document.createElement('i');
    icon.className = 'fa-solid fa-magnifying-glass text-fg-secondary mr-3 shrink-0';

    input = document.createElement('input');
    input.type = 'text';
    input.placeholder = 'Search entities...';
    input.className = 'flex-1 py-3.5 bg-transparent text-fg text-sm outline-none placeholder:text-fg-muted';
    input.setAttribute('autocomplete', 'off');
    input.setAttribute('spellcheck', 'false');

    var kbd = document.createElement('kbd');
    kbd.className = 'ml-2 text-[10px] text-fg-muted border border-edge rounded px-1.5 py-0.5 font-mono shrink-0';
    kbd.textContent = 'ESC';

    inputWrap.appendChild(icon);
    inputWrap.appendChild(input);
    inputWrap.appendChild(kbd);

    // Results container (scrollable).
    resultsList = document.createElement('div');
    resultsList.className = 'max-h-[50vh] overflow-y-auto';

    // Footer with keyboard hints.
    footer = document.createElement('div');
    footer.className = 'px-4 py-2 border-t border-edge text-[11px] text-fg-muted flex items-center gap-4';
    footer.innerHTML =
      '<span><kbd class="border border-edge rounded px-1 py-0.5 font-mono text-[10px] mr-0.5">&uarr;</kbd>' +
      '<kbd class="border border-edge rounded px-1 py-0.5 font-mono text-[10px]">&darr;</kbd> Navigate</span>' +
      '<span><kbd class="border border-edge rounded px-1 py-0.5 font-mono text-[10px]">&crarr;</kbd> Open</span>' +
      '<span><kbd class="border border-edge rounded px-1 py-0.5 font-mono text-[10px]">Esc</kbd> Close</span>';

    modal.appendChild(inputWrap);
    modal.appendChild(resultsList);
    modal.appendChild(footer);
    overlay.appendChild(modal);
    document.body.appendChild(overlay);

    // Input event handlers.
    input.addEventListener('input', onInputChange);
    input.addEventListener('keydown', onInputKeydown);
  }

  // --- Open / Close ---

  function open() {
    if (isOpen) return;
    if (!getCampaignId()) return;

    if (!overlay) buildModal();

    overlay.style.display = '';
    input.value = '';
    results = [];
    activeIndex = -1;
    renderEmpty();
    isOpen = true;

    // Focus after repaint so transition works.
    requestAnimationFrame(function () {
      input.focus();
    });
  }

  function close() {
    if (!isOpen) return;
    overlay.style.display = 'none';
    isOpen = false;
    activeIndex = -1;
    results = [];

    // Cancel any pending request.
    if (abortController) {
      abortController.abort();
      abortController = null;
    }
    if (debounceTimer) {
      clearTimeout(debounceTimer);
      debounceTimer = null;
    }
  }

  // --- Search ---

  function onInputChange() {
    var query = input.value.trim();

    if (debounceTimer) clearTimeout(debounceTimer);

    if (query.length < 2) {
      results = [];
      activeIndex = -1;
      if (query.length === 0) {
        renderEmpty();
      } else {
        renderHint('Type at least 2 characters to search...');
      }
      return;
    }

    debounceTimer = setTimeout(function () {
      doSearch(query);
    }, 200);
  }

  function doSearch(query) {
    // Cancel previous request.
    if (abortController) abortController.abort();
    abortController = new AbortController();

    var campaignId = getCampaignId();
    if (!campaignId) return;

    var url = '/campaigns/' + encodeURIComponent(campaignId) +
              '/entities/search?q=' + encodeURIComponent(query);

    fetch(url, {
      headers: { 'Accept': 'application/json' },
      signal: abortController.signal
    })
      .then(function (res) { return res.json(); })
      .then(function (data) {
        results = data.results || [];
        activeIndex = results.length > 0 ? 0 : -1;
        renderResults(data.total || 0);
      })
      .catch(function (err) {
        if (err.name !== 'AbortError') {
          console.error('[Search] Fetch failed:', err);
          results = [];
          activeIndex = -1;
          renderHint('Search failed. Please try again.');
        }
      });
  }

  // --- Rendering ---

  function renderEmpty() {
    resultsList.innerHTML =
      '<div class="px-4 py-8 text-center text-sm text-fg-muted">' +
        '<i class="fa-solid fa-magnifying-glass text-2xl mb-2 block opacity-30"></i>' +
        'Search for entities by name' +
      '</div>';
  }

  function renderHint(msg) {
    resultsList.innerHTML =
      '<div class="px-4 py-4 text-sm text-fg-muted text-center">' + msg + '</div>';
  }

  function renderResults(total) {
    if (results.length === 0) {
      resultsList.innerHTML =
        '<div class="px-4 py-6 text-center text-sm text-fg-muted">' +
          '<i class="fa-solid fa-circle-xmark text-lg mb-1 block opacity-30"></i>' +
          'No entities found' +
        '</div>';
      return;
    }

    var html = '';
    for (var i = 0; i < results.length; i++) {
      var r = results[i];
      var isActive = (i === activeIndex);
      var icon = r.type_icon || '';
      var color = r.type_color || '#6b7280';

      html +=
        '<a href="' + escapeAttr(r.url) + '" ' +
          'class="search-result flex items-center px-4 py-2.5 text-sm transition-colors cursor-pointer ' +
          (isActive ? 'bg-accent/10 text-accent' : 'text-fg hover:bg-surface-alt') + '" ' +
          'data-index="' + i + '">' +
          '<span class="w-6 h-6 rounded flex items-center justify-center mr-3 shrink-0">' +
            (icon
              ? '<i class="fa-solid ' + escapeAttr(icon) + ' text-xs" style="color:' + escapeAttr(color) + '"></i>'
              : '<span class="w-2.5 h-2.5 rounded-full" style="background-color:' + escapeAttr(color) + '"></span>'
            ) +
          '</span>' +
          '<div class="min-w-0 flex-1">' +
            '<div class="font-medium truncate">' + escapeHtml(r.name) + '</div>' +
            '<div class="text-xs text-fg-secondary">' + escapeHtml(r.type_name) + '</div>' +
          '</div>' +
        '</a>';
    }

    if (total > results.length) {
      html +=
        '<div class="px-4 py-2 text-xs text-fg-muted border-t border-edge-light text-center">' +
          'Showing ' + results.length + ' of ' + total + ' results' +
        '</div>';
    }

    resultsList.innerHTML = html;

    // Attach click handlers via delegation.
    resultsList.onclick = function (e) {
      var link = e.target.closest('a.search-result');
      if (link) {
        e.preventDefault();
        navigateTo(link.getAttribute('href'));
      }
    };

    // Mouse hover highlights.
    resultsList.onmousemove = function (e) {
      var link = e.target.closest('a.search-result');
      if (link) {
        var idx = parseInt(link.getAttribute('data-index'), 10);
        if (idx !== activeIndex) {
          activeIndex = idx;
          updateActiveHighlight();
        }
      }
    };
  }

  function updateActiveHighlight() {
    var items = resultsList.querySelectorAll('a.search-result');
    for (var i = 0; i < items.length; i++) {
      if (i === activeIndex) {
        items[i].className = items[i].className
          .replace('text-fg hover:bg-surface-alt', '')
          .replace('bg-accent/10 text-accent', '') + ' bg-accent/10 text-accent';
      } else {
        items[i].className = items[i].className
          .replace('bg-accent/10 text-accent', '')
          .replace('text-fg hover:bg-surface-alt', '') + ' text-fg hover:bg-surface-alt';
      }
    }

    // Scroll active item into view.
    if (items[activeIndex]) {
      items[activeIndex].scrollIntoView({ block: 'nearest' });
    }
  }

  // --- Keyboard Navigation ---

  function onInputKeydown(e) {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        if (results.length > 0) {
          activeIndex = (activeIndex + 1) % results.length;
          updateActiveHighlight();
        }
        break;

      case 'ArrowUp':
        e.preventDefault();
        if (results.length > 0) {
          activeIndex = (activeIndex - 1 + results.length) % results.length;
          updateActiveHighlight();
        }
        break;

      case 'Enter':
        e.preventDefault();
        if (activeIndex >= 0 && activeIndex < results.length) {
          navigateTo(results[activeIndex].url);
        }
        break;

      case 'Escape':
        e.preventDefault();
        close();
        break;
    }
  }

  // --- Navigation ---

  function navigateTo(url) {
    close();
    window.location.href = url;
  }

  // --- Escape Helpers ---

  function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function escapeAttr(str) {
    if (!str) return '';
    return str.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  // --- Global Keyboard Shortcut ---

  document.addEventListener('keydown', function (e) {
    // Ctrl+K or Cmd+K opens the search modal.
    if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
      e.preventDefault();
      if (isOpen) {
        close();
      } else {
        open();
      }
      return;
    }
  });

  // Close on browser navigation (hx-boost).
  window.addEventListener('chronicle:navigated', function () {
    if (isOpen) close();
  });

  // Expose open/close for the topbar trigger button.
  window.Chronicle = window.Chronicle || {};
  Chronicle.openSearch = open;
  Chronicle.closeSearch = close;
})();
