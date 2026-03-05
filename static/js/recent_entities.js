/**
 * recent_entities.js -- Recently Viewed Entities Tracker
 *
 * Tracks entity page visits in localStorage and renders a "Recent" list
 * in the sidebar drill panel. On entity pages, records the visit. On any
 * page with a sidebar, populates the #sidebar-cat-recent element.
 *
 * Storage key: chronicle-recent-{campaignId}
 * Format: JSON array of { id, name, type, slug, ts } (newest first, max 10).
 */
(function () {
  'use strict';

  var MAX_RECENT = 10;
  var STORAGE_PREFIX = 'chronicle-recent-';

  /**
   * Get the campaign ID from the current page URL.
   * Pattern: /campaigns/{id}/...
   */
  function getCampaignID() {
    var parts = window.location.pathname.split('/');
    if (parts.length >= 3 && parts[1] === 'campaigns') {
      return parts[2];
    }
    return null;
  }

  /**
   * Get the entity ID from the current page URL.
   * Pattern: /campaigns/{id}/entities/{eid}
   */
  function getEntityID() {
    var parts = window.location.pathname.split('/');
    if (parts.length >= 5 && parts[3] === 'entities') {
      var eid = parts[4];
      // Exclude non-entity routes.
      if (eid && eid !== 'new' && eid !== 'search' && eid !== '') {
        return eid;
      }
    }
    return null;
  }

  /**
   * Load recent entities from localStorage.
   */
  function loadRecent(campaignId) {
    try {
      var raw = localStorage.getItem(STORAGE_PREFIX + campaignId);
      if (raw) return JSON.parse(raw);
    } catch (e) {
      // Ignore parse errors.
    }
    return [];
  }

  /**
   * Save recent entities to localStorage.
   */
  function saveRecent(campaignId, list) {
    try {
      localStorage.setItem(STORAGE_PREFIX + campaignId, JSON.stringify(list));
    } catch (e) {
      // Ignore storage errors.
    }
  }

  /**
   * Record a visit to an entity page.
   */
  function recordVisit(campaignId, entityId) {
    // Get entity name from the page title or breadcrumb.
    var name = '';
    var typeSlug = '';

    // Try document title: "EntityName - CampaignName"
    var title = document.title || '';
    var dashIdx = title.lastIndexOf(' - ');
    if (dashIdx > 0) {
      name = title.substring(0, dashIdx).trim();
    }

    // Try to get entity type from breadcrumb or URL.
    var parts = window.location.pathname.split('/');
    // Check if there's a type slug in the URL: /campaigns/{id}/{typeSlug}s/{eid} is NOT
    // the pattern — entities use /campaigns/{id}/entities/{eid}. Type comes from breadcrumb.
    var breadcrumb = document.querySelector('nav[aria-label="Breadcrumb"]');
    if (breadcrumb) {
      var links = breadcrumb.querySelectorAll('a');
      // Pattern: Campaign / TypeNames / ... / Current
      if (links.length >= 2) {
        // The second link might be the type (e.g. "Characters", "Locations")
        var typeLink = links[1];
        if (typeLink && typeLink.href) {
          var m = typeLink.href.match(/\/campaigns\/[^/]+\/([^/]+)$/);
          if (m) typeSlug = m[1];
        }
      }
    }

    if (!name) return; // Can't identify the entity.

    var list = loadRecent(campaignId);

    // Remove existing entry for this entity.
    list = list.filter(function (item) { return item.id !== entityId; });

    // Add to front.
    list.unshift({
      id: entityId,
      name: name,
      type: typeSlug,
      ts: Date.now()
    });

    // Trim to max.
    if (list.length > MAX_RECENT) {
      list = list.slice(0, MAX_RECENT);
    }

    saveRecent(campaignId, list);
  }

  /**
   * Render recent entities into the sidebar element.
   */
  function renderRecent(campaignId) {
    var container = document.getElementById('sidebar-cat-recent');
    if (!container) return;

    var list = loadRecent(campaignId);
    if (list.length === 0) {
      container.innerHTML = '<div class="text-xs text-gray-500 px-4 py-2 italic">No recently viewed pages</div>';
      return;
    }

    var html = '';
    list.forEach(function (item) {
      var href = '/campaigns/' + encodeURIComponent(campaignId) + '/entities/' + encodeURIComponent(item.id);
      html += '<a href="' + href + '" ' +
        'class="flex items-center px-4 py-1.5 text-xs transition-colors text-sidebar-text hover:bg-sidebar-hover hover:text-sidebar-active truncate">' +
        '<i class="fa-solid fa-clock text-[10px] text-gray-600 mr-2 shrink-0"></i>' +
        '<span class="truncate">' + escapeHtml(item.name) + '</span>' +
        '</a>';
    });

    container.innerHTML = html;
  }

  /**
   * Escape HTML to prevent XSS from stored entity names.
   */
  function escapeHtml(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  /**
   * Initialize on DOM ready.
   */
  function init() {
    var campaignId = getCampaignID();
    if (!campaignId) return;

    var entityId = getEntityID();
    if (entityId) {
      // Small delay so the page title and breadcrumb are rendered.
      setTimeout(function () {
        recordVisit(campaignId, entityId);
        renderRecent(campaignId);
      }, 100);
    } else {
      renderRecent(campaignId);
    }
  }

  // Run on initial load.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  // Re-render after HTMX swaps (sidebar drill navigation).
  document.addEventListener('htmx:afterSettle', function () {
    var campaignId = getCampaignID();
    if (campaignId) {
      var entityId = getEntityID();
      if (entityId) {
        recordVisit(campaignId, entityId);
      }
      renderRecent(campaignId);
    }
  });
})();
