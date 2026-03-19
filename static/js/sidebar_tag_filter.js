/**
 * sidebar_tag_filter.js -- Tag Filtering for Sidebar Drill Panel
 *
 * Adds a compact tag filter dropdown below the search input in the
 * sidebar drill panel. When tags are selected, appends &tags=slug1,slug2
 * to the search HTMX URL and re-triggers the entity list fetch.
 */
(function () {
  'use strict';

  var activeTags = [];
  var tagCache = null;

  /** Get campaign ID from the URL. */
  function getCampaignID() {
    var parts = window.location.pathname.split('/');
    if (parts.length >= 3 && parts[1] === 'campaigns') return parts[2];
    return null;
  }

  /** Fetch tags for the campaign (cached). */
  function fetchTags(campaignId) {
    if (tagCache) return Promise.resolve(tagCache);
    return Chronicle.apiFetch('/campaigns/' + campaignId + '/tags')
      .then(function (res) { return res.ok ? res.json() : []; })
      .then(function (tags) {
        tagCache = tags || [];
        return tagCache;
      })
      .catch(function () { return []; });
  }

  /** Inject tag filter UI into the drill panel. */
  function injectTagFilter() {
    var searchDiv = document.querySelector('#sidebar-cat-content .px-4.pb-2');
    if (!searchDiv) return;
    if (searchDiv.querySelector('.sidebar-tag-filter')) return; // Already injected.

    var campaignId = getCampaignID();
    if (!campaignId) return;

    fetchTags(campaignId).then(function (tags) {
      if (!tags || tags.length === 0) return;

      var container = document.createElement('div');
      container.className = 'sidebar-tag-filter flex flex-wrap gap-1 px-0 pt-1';

      tags.forEach(function (tag) {
        var chip = document.createElement('button');
        chip.type = 'button';
        chip.className = 'text-[10px] px-1.5 py-0.5 rounded-full border transition-colors';
        chip.textContent = tag.name || tag.slug;
        chip.dataset.tagSlug = tag.slug;
        updateChipStyle(chip, false);

        chip.addEventListener('click', function () {
          var idx = activeTags.indexOf(tag.slug);
          if (idx !== -1) {
            activeTags.splice(idx, 1);
            updateChipStyle(chip, false);
          } else {
            activeTags.push(tag.slug);
            updateChipStyle(chip, true);
          }
          applyFilter();
        });

        container.appendChild(chip);
      });

      searchDiv.appendChild(container);
    });
  }

  function updateChipStyle(chip, active) {
    if (active) {
      chip.className = 'text-[10px] px-1.5 py-0.5 rounded-full border border-accent bg-accent/20 text-accent transition-colors';
    } else {
      chip.className = 'text-[10px] px-1.5 py-0.5 rounded-full border border-gray-700 text-gray-500 hover:border-gray-500 transition-colors';
    }
  }

  /** Update the HTMX search URL and re-trigger. */
  function applyFilter() {
    var searchInput = document.querySelector('#sidebar-cat-content input[name="q"]');
    if (!searchInput) return;

    var baseUrl = searchInput.getAttribute('hx-get') || '';
    // Strip existing &tags= param.
    baseUrl = baseUrl.replace(/&tags=[^&]*/g, '');

    if (activeTags.length > 0) {
      baseUrl += '&tags=' + activeTags.join(',');
    }

    searchInput.setAttribute('hx-get', baseUrl);
    // Re-process the element so HTMX picks up the new URL.
    if (window.htmx) htmx.process(searchInput);

    // Trigger a fetch with current search value.
    var results = document.getElementById('sidebar-cat-results');
    if (results) {
      htmx.ajax('GET', baseUrl + (searchInput.value ? '&q=' + encodeURIComponent(searchInput.value) : ''), {
        target: results, swap: 'innerHTML'
      });
    }
  }

  /** Reset active tags when drilling out or switching categories. */
  function reset() {
    activeTags = [];
    var chips = document.querySelectorAll('.sidebar-tag-filter [data-tag-slug]');
    chips.forEach(function (chip) { updateChipStyle(chip, false); });
  }

  // Initialize after HTMX swaps (drill panel loads).
  document.addEventListener('htmx:afterSettle', function (e) {
    if (e.detail && e.detail.target && (
      e.detail.target.id === 'sidebar-cat-content' ||
      e.detail.target.id === 'sidebar-cat-results'
    )) {
      injectTagFilter();
    }
  });

  // Reset when drilling out.
  window.addEventListener('chronicle:navigated', reset);
})();
