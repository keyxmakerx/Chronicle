/**
 * sidebar_tag_filter.js -- Tag Filtering + Saved Presets for Sidebar Drill Panel
 *
 * Adds tag filter chips and saved filter presets below the search input
 * in the sidebar drill panel. Selecting tags appends &tags=slug1,slug2
 * to the search HTMX URL. Saved presets let users store and recall
 * tag combos with one click.
 *
 * API endpoints:
 *   GET /campaigns/:id/tags               -- available tags
 *   GET /campaigns/:id/saved-filters      -- user's saved presets
 *   POST /campaigns/:id/saved-filters     -- create preset
 *   DELETE /campaigns/:id/saved-filters/:fid -- delete preset
 */
(function () {
  'use strict';

  var activeTags = [];
  var tagCache = null;
  var savedFiltersCache = null;

  function getCampaignID() {
    var parts = window.location.pathname.split('/');
    if (parts.length >= 3 && parts[1] === 'campaigns') return parts[2];
    return null;
  }

  function fetchTags(campaignId) {
    if (tagCache) return Promise.resolve(tagCache);
    return Chronicle.apiFetch('/campaigns/' + campaignId + '/tags')
      .then(function (res) { return res.ok ? res.json() : []; })
      .then(function (tags) { tagCache = tags || []; return tagCache; })
      .catch(function () { return []; });
  }

  function fetchSavedFilters(campaignId) {
    return Chronicle.apiFetch('/campaigns/' + campaignId + '/saved-filters')
      .then(function (res) { return res.ok ? res.json() : []; })
      .then(function (filters) { savedFiltersCache = filters || []; return savedFiltersCache; })
      .catch(function () { return []; });
  }

  /** Inject tag filter UI + saved presets into the drill panel. */
  function injectTagFilter() {
    var searchDiv = document.querySelector('#sidebar-cat-content .px-4.pb-2');
    if (!searchDiv) return;
    if (searchDiv.querySelector('.sidebar-tag-filter')) return;

    var campaignId = getCampaignID();
    if (!campaignId) return;

    Promise.all([fetchTags(campaignId), fetchSavedFilters(campaignId)]).then(function (results) {
      var tags = results[0];
      var savedFilters = results[1];
      if ((!tags || tags.length === 0) && (!savedFilters || savedFilters.length === 0)) return;

      var wrapper = document.createElement('div');
      wrapper.className = 'sidebar-tag-filter pt-1';

      // Saved filter presets row.
      if (savedFilters && savedFilters.length > 0) {
        var presetsRow = document.createElement('div');
        presetsRow.className = 'flex flex-wrap gap-1 mb-1';
        savedFilters.forEach(function (filter) {
          var btn = document.createElement('button');
          btn.type = 'button';
          btn.className = 'text-[11px] px-2 py-1 min-h-[32px] rounded-md bg-accent/10 text-accent border border-accent/30 hover:bg-accent/20 transition-colors flex items-center gap-1';
          btn.innerHTML = '<i class="fa-solid fa-bookmark text-[8px]"></i> ' + Chronicle.escapeHtml(filter.name);
          btn.addEventListener('click', function () {
            applyPreset(filter.tag_slugs, tags);
          });
          // Delete on right-click.
          btn.addEventListener('contextmenu', function (e) {
            e.preventDefault();
            if (!confirm('Delete saved filter "' + filter.name + '"?')) return;
            Chronicle.apiFetch('/campaigns/' + campaignId + '/saved-filters/' + filter.id, { method: 'DELETE' })
              .then(function () {
                savedFiltersCache = null;
                btn.remove();
                Chronicle.notify('Filter deleted', 'success');
              });
          });
          presetsRow.appendChild(btn);
        });
        wrapper.appendChild(presetsRow);
      }

      // Tag chips row.
      if (tags && tags.length > 0) {
        var chipsRow = document.createElement('div');
        chipsRow.className = 'flex flex-wrap gap-1 items-center';
        tags.forEach(function (tag) {
          var chip = document.createElement('button');
          chip.type = 'button';
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
            updateSaveButton();
            applyFilter();
          });
          chipsRow.appendChild(chip);
        });

        // Save button (appears when tags are active).
        var saveBtn = document.createElement('button');
        saveBtn.type = 'button';
        saveBtn.id = 'sidebar-save-filter';
        saveBtn.className = 'text-[11px] px-2 py-1 min-h-[32px] rounded-md text-accent hover:bg-accent/10 transition-colors hidden';
        saveBtn.innerHTML = '<i class="fa-solid fa-floppy-disk text-[8px] mr-0.5"></i> Save';
        saveBtn.addEventListener('click', function () {
          var name = prompt('Filter name:');
          if (!name || !name.trim()) return;
          Chronicle.apiFetch('/campaigns/' + campaignId + '/saved-filters', {
            method: 'POST',
            body: { name: name.trim(), tag_slugs: activeTags.slice() }
          }).then(function (res) {
            if (res.ok) {
              Chronicle.notify('Filter saved', 'success');
              savedFiltersCache = null;
              // Refresh the whole filter UI to show the new preset.
              wrapper.remove();
              injectTagFilter();
            }
          });
        });
        chipsRow.appendChild(saveBtn);
        wrapper.appendChild(chipsRow);
      }

      searchDiv.appendChild(wrapper);
    });
  }

  // Touch-friendly chip sizes: min-h-[32px] ensures adequate tap target
  // on mobile while staying compact on desktop.
  function updateChipStyle(chip, active) {
    if (active) {
      chip.className = 'text-[11px] px-2 py-1 min-h-[32px] rounded-full border border-accent bg-accent/20 text-accent transition-colors';
    } else {
      chip.className = 'text-[11px] px-2 py-1 min-h-[32px] rounded-full border border-gray-700 text-gray-500 hover:border-gray-500 transition-colors';
    }
  }

  /** Show/hide save button based on active tag count. */
  function updateSaveButton() {
    var btn = document.getElementById('sidebar-save-filter');
    if (!btn) return;
    if (activeTags.length > 0) {
      btn.classList.remove('hidden');
    } else {
      btn.classList.add('hidden');
    }
  }

  /** Apply a saved preset — activate its tags and trigger filter. */
  function applyPreset(tagSlugs, allTags) {
    activeTags = tagSlugs.slice();
    var chips = document.querySelectorAll('.sidebar-tag-filter [data-tag-slug]');
    chips.forEach(function (chip) {
      var isActive = activeTags.indexOf(chip.dataset.tagSlug) !== -1;
      updateChipStyle(chip, isActive);
    });
    updateSaveButton();
    applyFilter();
  }

  function applyFilter() {
    var searchInput = document.querySelector('#sidebar-cat-content input[name="q"]');
    if (!searchInput) return;

    var baseUrl = searchInput.getAttribute('hx-get') || '';
    baseUrl = baseUrl.replace(/&tags=[^&]*/g, '');

    if (activeTags.length > 0) {
      baseUrl += '&tags=' + activeTags.join(',');
    }

    searchInput.setAttribute('hx-get', baseUrl);
    if (window.htmx) htmx.process(searchInput);

    var results = document.getElementById('sidebar-cat-results');
    if (results) {
      htmx.ajax('GET', baseUrl + (searchInput.value ? '&q=' + encodeURIComponent(searchInput.value) : ''), {
        target: results, swap: 'innerHTML'
      });
    }
  }

  function reset() {
    activeTags = [];
    savedFiltersCache = null;
    var chips = document.querySelectorAll('.sidebar-tag-filter [data-tag-slug]');
    chips.forEach(function (chip) { updateChipStyle(chip, false); });
    updateSaveButton();
  }

  document.addEventListener('htmx:afterSettle', function (e) {
    if (e.detail && e.detail.target && (
      e.detail.target.id === 'sidebar-cat-content' ||
      e.detail.target.id === 'sidebar-cat-results'
    )) {
      injectTagFilter();
    }
  });

  window.addEventListener('chronicle:navigated', reset);
})();
