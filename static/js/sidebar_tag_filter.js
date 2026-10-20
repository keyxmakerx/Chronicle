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
    var searchDiv = document.querySelector('#sidebar-cat-content .nav-drill-search');
    if (!searchDiv) return;
    if (searchDiv.querySelector('.sidebar-tag-filter')) return;

    var campaignId = getCampaignID();
    if (!campaignId) return;

    Promise.all([fetchTags(campaignId), fetchSavedFilters(campaignId)]).then(function (results) {
      var tags = results[0];
      var savedFilters = results[1];
      if ((!tags || tags.length === 0) && (!savedFilters || savedFilters.length === 0)) return;

      var wrapper = document.createElement('div');
      wrapper.className = 'sidebar-tag-filter';

      // An icon in the search box opens a drop-down over the list, so the
      // filter costs no line; its count keeps a closed filter from being silent.
      wrapper.innerHTML = '<button type="button" class="nav-drill-tags-toggle" id="sidebar-tags-toggle" title="Filter by tag" aria-label="Tag filters"' +
        ' aria-controls="sidebar-tags-body" aria-expanded="false"><i class="fa-solid fa-tags" aria-hidden="true"></i>' +
        '<span class="nav-drill-tags-n"></span></button><div class="nav-drill-tags-menu" id="sidebar-tags-body">' +
        '<div class="nav-drill-tags-list"></div><div class="nav-drill-tags-foot" id="sidebar-save-filter" hidden>' +
        '<button type="button">Save as filter</button><button type="button">Clear</button></div></div>';
      var toggle = wrapper.firstChild;
      var list = wrapper.querySelector('.nav-drill-tags-list');
      var foot = list.nextSibling;
      toggle.addEventListener('click', function () {
        setOpen(toggle.getAttribute('aria-expanded') !== 'true');
      });

      // Saved filter presets row.
      if (savedFilters && savedFilters.length > 0) {
        var presetsRow = document.createElement('div');
        presetsRow.innerHTML = '<div class="nav-drill-tags-label">Saved filters</div>';
        savedFilters.forEach(function (filter) {
          var btn = document.createElement('button');
          btn.type = 'button';
          btn.className = 'nav-drill-tag-row';
          btn.title = 'Right-click to delete';
          btn.innerHTML = '<i class="fa-solid fa-bookmark" aria-hidden="true"></i><span>' + Chronicle.escapeHtml(filter.name) + '</span>';
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
        list.appendChild(presetsRow);
      }

      // Tag chips row.
      if (tags && tags.length > 0) {
        var chipsRow = document.createElement('div');
        chipsRow.innerHTML = '<div class="nav-drill-tags-label">Tags</div>';
        tags.forEach(function (tag) {
          var chip = document.createElement('button');
          chip.type = 'button';
          chip.className = 'nav-drill-tag-row';
          chip.innerHTML = '<i class="fa-solid fa-check" aria-hidden="true"></i><span></span>';
          chip.lastChild.textContent = tag.name || tag.slug;
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

        foot.lastChild.addEventListener('click', function () { applyPreset([]); });
        foot.firstChild.addEventListener('click', function () {
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
        list.appendChild(chipsRow);
      }

      searchDiv.appendChild(wrapper);
    });
  }

  function setOpen(open) {
    var toggle = document.getElementById('sidebar-tags-toggle');
    if (!toggle) return;
    toggle.setAttribute('aria-expanded', open);
    toggle.nextSibling.classList.toggle('is-open', open);
  }

  function updateToggleLabel() {
    var toggle = document.getElementById('sidebar-tags-toggle');
    if (!toggle) return;
    var n = activeTags.length;
    toggle.lastChild.textContent = n || '';
    toggle.setAttribute('aria-label', 'Tag filters' + (n ? ', ' + n + ' active' : ''));
  }

  function updateChipStyle(chip, active) {
    chip.setAttribute('aria-pressed', active ? 'true' : 'false');
  }

  /** Show/hide save button based on active tag count. */
  function updateSaveButton() {
    updateToggleLabel();
    var btn = document.getElementById('sidebar-save-filter');
    if (!btn) return;
    btn.hidden = activeTags.length === 0;
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

  // Like a menu, the drop-down closes on a click outside it or on Escape.
  function closeOn(e) {
    var toggle = document.getElementById('sidebar-tags-toggle');
    if (!toggle || toggle.getAttribute('aria-expanded') !== 'true') return;
    if (e.type === 'click' ? toggle.parentNode.contains(e.target) : e.key !== 'Escape') return;
    setOpen(false);
    if (e.type !== 'click') toggle.focus();
  }
  document.addEventListener('click', closeOn);
  document.addEventListener('keydown', closeOn);

  window.addEventListener('chronicle:navigated', reset);
})();
