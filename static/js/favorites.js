/**
 * favorites.js -- Entity Favorites (Bookmarks)
 *
 * Manages a localStorage-backed list of favorite entities per campaign.
 * Renders a "Favorites" section in the sidebar drill panel and handles
 * the star toggle button on entity show pages.
 *
 * Storage key: chronicle-favorites-{campaignId}
 * Format: JSON array of { id, name, ts } (newest first).
 */
(function () {
  'use strict';

  var MAX_FAVORITES = 50;
  var STORAGE_PREFIX = 'chronicle-favorites-';

  /** Get campaign ID from the URL. */
  function getCampaignID() {
    var parts = window.location.pathname.split('/');
    if (parts.length >= 3 && parts[1] === 'campaigns') {
      return parts[2];
    }
    return null;
  }

  /** Load favorites from localStorage. */
  function loadFavorites(campaignId) {
    try {
      var raw = localStorage.getItem(STORAGE_PREFIX + campaignId);
      if (raw) return JSON.parse(raw);
    } catch (e) { /* ignore */ }
    return [];
  }

  /** Save favorites to localStorage. */
  function saveFavorites(campaignId, list) {
    try {
      localStorage.setItem(STORAGE_PREFIX + campaignId, JSON.stringify(list));
    } catch (e) { /* ignore */ }
  }

  /** Check if an entity is favorited. */
  function isFavorite(campaignId, entityId) {
    var list = loadFavorites(campaignId);
    return list.some(function (item) { return item.id === entityId; });
  }

  /** Toggle favorite status for an entity. Returns new state. */
  function toggleFavorite(campaignId, entityId, entityName) {
    var list = loadFavorites(campaignId);
    var idx = -1;
    for (var i = 0; i < list.length; i++) {
      if (list[i].id === entityId) { idx = i; break; }
    }
    if (idx !== -1) {
      list.splice(idx, 1);
      saveFavorites(campaignId, list);
      return false;
    }
    list.unshift({ id: entityId, name: entityName, ts: Date.now() });
    if (list.length > MAX_FAVORITES) list = list.slice(0, MAX_FAVORITES);
    saveFavorites(campaignId, list);
    return true;
  }

  /** Update the star button icon to reflect current state. */
  function updateStarButton(btn, favorited) {
    var icon = btn.querySelector('i');
    if (!icon) return;
    if (favorited) {
      icon.className = 'fa-solid fa-star text-lg text-amber-400';
    } else {
      icon.className = 'fa-regular fa-star text-lg';
    }
  }

  /** Bind click handlers on star toggle buttons. */
  function bindToggleButtons(campaignId) {
    var buttons = document.querySelectorAll('[data-favorite-toggle]');
    buttons.forEach(function (btn) {
      if (btn._favBound) return;
      btn._favBound = true;

      var entityId = btn.dataset.favoriteToggle;
      var entityName = btn.dataset.entityName || '';

      // Set initial state.
      updateStarButton(btn, isFavorite(campaignId, entityId));

      btn.addEventListener('click', function (e) {
        e.preventDefault();
        var nowFav = toggleFavorite(campaignId, entityId, entityName);
        updateStarButton(btn, nowFav);
        renderFavorites(campaignId);
      });
    });
  }

  /** Render favorites list in the sidebar. */
  function renderFavorites(campaignId) {
    var container = document.getElementById('sidebar-cat-favorites');
    var header = document.getElementById('sidebar-cat-favorites-header');
    if (!container) return;

    var list = loadFavorites(campaignId);
    if (list.length === 0) {
      container.innerHTML = '';
      if (header) header.style.display = 'none';
      return;
    }

    if (header) header.style.display = '';

    var html = '';
    list.forEach(function (item) {
      var href = '/campaigns/' + encodeURIComponent(campaignId) + '/entities/' + encodeURIComponent(item.id);
      html += '<a href="' + href + '" ' +
        'class="flex items-center px-4 py-1.5 text-xs transition-colors text-sidebar-text hover:bg-sidebar-hover hover:text-sidebar-active truncate">' +
        '<i class="fa-solid fa-star text-[10px] text-amber-400 mr-2 shrink-0"></i>' +
        '<span class="truncate">' + Chronicle.escapeHtml(item.name) + '</span>' +
        '</a>';
    });

    container.innerHTML = html;
  }

  /** Initialize favorites on page load. */
  function init() {
    var campaignId = getCampaignID();
    if (!campaignId) return;

    bindToggleButtons(campaignId);
    renderFavorites(campaignId);
  }

  // Run on initial load.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  // Re-bind after HTMX swaps.
  document.addEventListener('htmx:afterSettle', function () {
    var campaignId = getCampaignID();
    if (campaignId) {
      bindToggleButtons(campaignId);
      renderFavorites(campaignId);
    }
  });
})();
