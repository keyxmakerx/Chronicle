/**
 * favorites.js -- Entity Favorites (Bookmarks)
 *
 * Manages per-user, per-campaign entity favorites backed by the database.
 * Renders a "Favorites" section in the sidebar drill panel and handles
 * the star toggle button on entity show pages.
 *
 * API endpoints:
 *   POST /campaigns/:id/entities/:eid/favorite  -- toggle favorite
 *   GET  /campaigns/:id/favorites               -- list favorites (JSON)
 *   GET  /campaigns/:id/favorite-ids             -- list favorited entity IDs
 */
(function () {
  'use strict';

  // In-memory cache of favorited entity IDs per campaign.
  var favoriteCache = {};

  /** Get campaign ID from the URL. */
  function getCampaignID() {
    var parts = window.location.pathname.split('/');
    if (parts.length >= 3 && parts[1] === 'campaigns') {
      return parts[2];
    }
    return null;
  }

  /** Fetch the set of favorited entity IDs and cache them. */
  function loadFavoriteIDs(campaignId) {
    if (favoriteCache[campaignId]) return Promise.resolve(favoriteCache[campaignId]);

    return Chronicle.apiFetch('/campaigns/' + campaignId + '/favorite-ids')
      .then(function (res) { return res.ok ? res.json() : []; })
      .then(function (ids) {
        var set = {};
        (ids || []).forEach(function (id) { set[id] = true; });
        favoriteCache[campaignId] = set;
        return set;
      })
      .catch(function () {
        favoriteCache[campaignId] = {};
        return {};
      });
  }

  /** Check if an entity is favorited (from cache). */
  function isFavorite(campaignId, entityId) {
    var cache = favoriteCache[campaignId];
    return cache ? !!cache[entityId] : false;
  }

  /** Toggle favorite via API. Returns new state. */
  function toggleFavorite(campaignId, entityId) {
    return Chronicle.apiFetch('/campaigns/' + campaignId + '/entities/' + entityId + '/favorite', {
      method: 'POST'
    })
    .then(function (res) { return res.ok ? res.json() : null; })
    .then(function (data) {
      if (!data) return false;
      // Update cache.
      if (!favoriteCache[campaignId]) favoriteCache[campaignId] = {};
      if (data.favorited) {
        favoriteCache[campaignId][entityId] = true;
      } else {
        delete favoriteCache[campaignId][entityId];
      }
      return data.favorited;
    });
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

      // Set initial state from cache.
      updateStarButton(btn, isFavorite(campaignId, entityId));

      btn.addEventListener('click', function (e) {
        e.preventDefault();
        toggleFavorite(campaignId, entityId).then(function (nowFav) {
          updateStarButton(btn, nowFav);
          renderFavorites(campaignId);
        });
      });
    });
  }

  /** Render favorites list in the sidebar drill panel. */
  function renderFavorites(campaignId) {
    var container = document.getElementById('sidebar-cat-favorites');
    var header = document.getElementById('sidebar-cat-favorites-header');
    if (!container) return;

    Chronicle.apiFetch('/campaigns/' + campaignId + '/favorites')
      .then(function (res) { return res.ok ? res.json() : []; })
      .then(function (items) {
        if (!items || items.length === 0) {
          container.innerHTML = '';
          if (header) header.style.display = 'none';
          return;
        }

        if (header) header.style.display = '';

        var html = '';
        items.forEach(function (item) {
          var href = '/campaigns/' + encodeURIComponent(campaignId) + '/entities/' + encodeURIComponent(item.id);
          html += '<a href="' + href + '" ' +
            'class="flex items-center px-4 py-1.5 text-[11px] transition-colors text-sidebar-text hover:bg-sidebar-hover hover:text-sidebar-active truncate">' +
            '<i class="fa-solid fa-star text-[9px] text-amber-400/70 mr-2 shrink-0"></i>' +
            '<span class="truncate">' + Chronicle.escapeHtml(item.name) + '</span>' +
            '</a>';
        });

        container.innerHTML = html;
      })
      .catch(function () {
        // Silently fail — favorites section just stays empty.
      });
  }

  /** Initialize favorites on page load. */
  function init() {
    var campaignId = getCampaignID();
    if (!campaignId) return;

    // Load favorite IDs, then bind star buttons.
    loadFavoriteIDs(campaignId).then(function () {
      bindToggleButtons(campaignId);
      renderFavorites(campaignId);
    });
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
