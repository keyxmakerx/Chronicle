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

      // An icon at the search box's right end opens a drop-down over the
      // list, so the filter costs no line of its own. The icon carries the
      // active count so a closed filter is never a silent one.
      var toggle = document.createElement('button');
      toggle.type = 'button';
      toggle.className = 'nav-drill-tags-toggle';
      toggle.id = 'sidebar-tags-toggle';
      toggle.setAttribute('aria-controls', 'sidebar-tags-body');
      var body = document.createElement('div');
      body.id = 'sidebar-tags-body';
      body.className = 'nav-drill-tags-menu';
      setOpen(toggle, body, false);
      toggle.addEventListener('click', function () {
        setOpen(toggle, body, toggle.getAttribute('aria-expanded') !== 'true');
      });
      // The list scrolls; Save and Clear stay pinned under it.
      var list = document.createElement('div');
      list.className = 'nav-drill-tags-list';
      body.appendChild(list);
      wrapper.appendChild(toggle);
      wrapper.appendChild(body);
      searchDiv.classList.add('has-tags');

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

        // Save and Clear are quiet words, shown while tags are active.
        var foot = document.createElement('div');
        foot.id = 'sidebar-save-filter';
        foot.className = 'nav-drill-tags-foot';
        foot.hidden = true;
        var saveBtn = document.createElement('button');
        saveBtn.type = 'button';
        saveBtn.textContent = 'Save as filter';
        var clearBtn = document.createElement('button');
        clearBtn.type = 'button';
        clearBtn.textContent = 'Clear';
        clearBtn.addEventListener('click', function () { applyPreset([]); });
        foot.appendChild(saveBtn);
        foot.appendChild(clearBtn);
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
        list.appendChild(chipsRow);
        body.appendChild(foot);
      }

      searchDiv.appendChild(wrapper);
      updateToggleLabel();
    });
  }

  function setOpen(toggle, body, open) {
    toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
    body.hidden = !open;
  }

  /** The tag icon, plus the count while tags filter the list. */
  function updateToggleLabel() {
    var toggle = document.getElementById('sidebar-tags-toggle');
    if (!toggle) return;
    toggle.innerHTML = '<i class="fa-solid fa-tags" aria-hidden="true"></i>' +
      (activeTags.length > 0 ? '<span class="nav-drill-tags-n">' + activeTags.length + '</span>' : '');
    toggle.title = 'Filter by tag';
    toggle.setAttribute('aria-label', activeTags.length > 0
      ? 'Tag filters, ' + activeTags.length + ' active' : 'Tag filters');
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

  // The drop-down closes on a click outside it or on Escape, like a menu.
  document.addEventListener('click', function (e) {
    var toggle = document.getElementById('sidebar-tags-toggle');
    var menu = document.getElementById('sidebar-tags-body');
    if (!toggle || !menu || menu.hidden) return;
    if (toggle.contains(e.target) || menu.contains(e.target)) return;
    setOpen(toggle, menu, false);
  });

  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape') return;
    var toggle = document.getElementById('sidebar-tags-toggle');
    var menu = document.getElementById('sidebar-tags-body');
    if (!toggle || !menu || menu.hidden) return;
    setOpen(toggle, menu, false);
    toggle.focus();
  });

  window.addEventListener('chronicle:navigated', reset);
})();
