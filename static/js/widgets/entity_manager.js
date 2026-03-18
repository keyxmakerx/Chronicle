/**
 * entity_manager.js -- Entity Manager Widget
 *
 * Sortable, filterable entity list with visibility controls. Can be placed
 * on entity pages, category dashboards, or the campaign dashboard via the
 * template block system.
 *
 * Config attributes (from data-* on mount element):
 *   data-campaign-id             -- Campaign ID
 *   data-entity-type-id          -- Entity type ID to show (0 = show type selector)
 *   data-role                    -- User's campaign role (1=player, 2=scribe, 3=owner)
 *   data-csrf                    -- CSRF token for mutations
 *   data-entities-endpoint       -- Base search endpoint
 *   data-reorder-endpoint        -- Base entity endpoint for reorder
 *   data-sidebar-config-endpoint -- Sidebar config endpoint for hide/unhide
 */
(function () {
  'use strict';

  Chronicle.register('entity_manager', {
    init: function (el, config) {
      var campaignId = config.campaignId;
      var entityTypeId = parseInt(config.entityTypeId || '0', 10);
      var role = parseInt(config.role || '1', 10);
      var csrfToken = config.csrf || '';
      var entitiesEndpoint = config.entitiesEndpoint;
      var reorderEndpoint = config.reorderEndpoint;
      var sidebarConfigEndpoint = config.sidebarConfigEndpoint;

      if (!campaignId || !entitiesEndpoint) {
        console.error('[entity_manager] Missing required config');
        return;
      }

      // Widget state.
      var entities = [];
      var sidebarConfig = null;
      var hiddenSet = {};
      var searchQuery = '';
      var sortMode = 'manual'; // manual | name | updated | created
      var searchTimer = null;
      var dragSrcId = null;

      // ---------------------------------------------------------------
      // Data fetching
      // ---------------------------------------------------------------

      function fetchEntities() {
        var url = entitiesEndpoint + '?sidebar=1&type=' + entityTypeId;
        if (searchQuery.length >= 2) {
          url = entitiesEndpoint + '?q=' + encodeURIComponent(searchQuery) + '&type=' + entityTypeId;
        }

        return Chronicle.apiFetch(url, {
          headers: { 'Accept': 'application/json' }
        })
          .then(function (res) { return res.ok ? res.json() : { results: [] }; })
          .then(function (data) {
            entities = data.results || [];
            sortEntities();
            render();
          })
          .catch(function (err) {
            console.error('[entity_manager] fetch failed', err);
          });
      }

      function fetchSidebarConfig() {
        if (!sidebarConfigEndpoint || role < 3) return Promise.resolve();

        return Chronicle.apiFetch(sidebarConfigEndpoint)
          .then(function (res) { return res.ok ? res.json() : {}; })
          .then(function (data) {
            sidebarConfig = data || {};
            if (!sidebarConfig.hidden_entity_ids) sidebarConfig.hidden_entity_ids = [];
            hiddenSet = {};
            sidebarConfig.hidden_entity_ids.forEach(function (id) {
              hiddenSet[id] = true;
            });
          })
          .catch(function () {
            sidebarConfig = { hidden_entity_ids: [] };
            hiddenSet = {};
          });
      }

      // ---------------------------------------------------------------
      // Sorting
      // ---------------------------------------------------------------

      function sortEntities() {
        if (sortMode === 'name') {
          entities.sort(function (a, b) {
            return (a.name || '').localeCompare(b.name || '');
          });
        }
        // manual sort uses server order (sort_order field)
        // updated/created would need timestamp fields from the API
      }

      // ---------------------------------------------------------------
      // Entity visibility toggle
      // ---------------------------------------------------------------

      function toggleEntityVisibility(entityId) {
        if (!sidebarConfig) return;
        var idx = sidebarConfig.hidden_entity_ids.indexOf(entityId);
        if (idx === -1) {
          sidebarConfig.hidden_entity_ids.push(entityId);
          hiddenSet[entityId] = true;
        } else {
          sidebarConfig.hidden_entity_ids.splice(idx, 1);
          delete hiddenSet[entityId];
        }
        render();
        saveSidebarConfig();
      }

      function saveSidebarConfig() {
        if (!sidebarConfigEndpoint || !sidebarConfig) return;
        Chronicle.apiFetch(sidebarConfigEndpoint, {
          method: 'PUT',
          body: {
            entity_type_order: sidebarConfig.entity_type_order || [],
            hidden_type_ids: sidebarConfig.hidden_type_ids || [],
            hidden_entity_ids: sidebarConfig.hidden_entity_ids || [],
            custom_sections: sidebarConfig.custom_sections || [],
            custom_links: sidebarConfig.custom_links || []
          }
        }).catch(function () {
          Chronicle.notify('Failed to save visibility', 'error');
        });
      }

      // ---------------------------------------------------------------
      // Drag and drop reorder (Owner/Scribe only)
      // ---------------------------------------------------------------

      function onDragStart(e) {
        dragSrcId = e.currentTarget.getAttribute('data-entity-id');
        e.currentTarget.classList.add('opacity-40');
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', dragSrcId);
      }

      function onDragOver(e) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        var row = e.currentTarget;
        row.classList.add('border-t-2', 'border-accent');
      }

      function onDragLeave(e) {
        e.currentTarget.classList.remove('border-t-2', 'border-accent');
      }

      function onDrop(e) {
        e.preventDefault();
        e.currentTarget.classList.remove('border-t-2', 'border-accent');
        var targetId = e.currentTarget.getAttribute('data-entity-id');
        if (!dragSrcId || dragSrcId === targetId) return;

        // Find indices.
        var srcIdx = -1, tgtIdx = -1;
        entities.forEach(function (ent, i) {
          if (ent.id === dragSrcId) srcIdx = i;
          if (ent.id === targetId) tgtIdx = i;
        });
        if (srcIdx === -1 || tgtIdx === -1) return;

        // Move in array.
        var moved = entities.splice(srcIdx, 1)[0];
        entities.splice(tgtIdx, 0, moved);
        render();

        // Save via reorder API.
        Chronicle.apiFetch(reorderEndpoint + '/' + dragSrcId + '/reorder', {
          method: 'PUT',
          body: { sort_order: tgtIdx, parent_id: null }
        }).catch(function () {
          Chronicle.notify('Failed to save order', 'error');
        });
      }

      function onDragEnd(e) {
        e.currentTarget.classList.remove('opacity-40');
        dragSrcId = null;
      }

      // ---------------------------------------------------------------
      // Rendering
      // ---------------------------------------------------------------

      function render() {
        el.innerHTML = '';

        // Toolbar: search + sort.
        var toolbar = document.createElement('div');
        toolbar.className = 'flex items-center gap-2 mb-3';

        // Search input.
        var searchWrap = document.createElement('div');
        searchWrap.className = 'relative flex-1';
        var searchIcon = document.createElement('i');
        searchIcon.className = 'fa-solid fa-search absolute left-2.5 top-1/2 -translate-y-1/2 text-[10px] text-fg-muted';
        var searchInput = document.createElement('input');
        searchInput.type = 'text';
        searchInput.placeholder = 'Search entities...';
        searchInput.value = searchQuery;
        searchInput.className = 'w-full pl-7 pr-3 py-1.5 text-xs bg-surface-alt border border-border rounded-lg text-fg placeholder:text-fg-muted focus:outline-none focus:ring-1 focus:ring-accent';
        searchInput.addEventListener('input', function () {
          searchQuery = searchInput.value;
          clearTimeout(searchTimer);
          searchTimer = setTimeout(fetchEntities, 300);
        });
        searchWrap.appendChild(searchIcon);
        searchWrap.appendChild(searchInput);
        toolbar.appendChild(searchWrap);

        // Sort dropdown.
        var sortSelect = document.createElement('select');
        sortSelect.className = 'text-xs bg-surface-alt border border-border rounded-lg px-2 py-1.5 text-fg focus:outline-none focus:ring-1 focus:ring-accent';
        var sortOptions = [
          { value: 'manual', label: 'Manual' },
          { value: 'name', label: 'Name' }
        ];
        sortOptions.forEach(function (opt) {
          var option = document.createElement('option');
          option.value = opt.value;
          option.textContent = opt.label;
          if (opt.value === sortMode) option.selected = true;
          sortSelect.appendChild(option);
        });
        sortSelect.addEventListener('change', function () {
          sortMode = sortSelect.value;
          sortEntities();
          render();
        });
        toolbar.appendChild(sortSelect);

        el.appendChild(toolbar);

        // Entity list.
        if (entities.length === 0) {
          var empty = document.createElement('div');
          empty.className = 'text-center py-8 text-fg-muted text-sm';
          empty.innerHTML = '<i class="fa-solid fa-file-circle-plus text-lg mb-2 block opacity-50"></i>No entities found';
          el.appendChild(empty);
          return;
        }

        var list = document.createElement('div');
        list.className = 'entity-manager-list divide-y divide-border/50';

        entities.forEach(function (entity) {
          var row = document.createElement('div');
          var isHidden = !!hiddenSet[entity.id];
          row.className = 'entity-manager-row flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-surface-alt transition-colors' +
            (isHidden ? ' opacity-40' : '');
          row.setAttribute('data-entity-id', entity.id);

          // Drag handle (Scribe+).
          if (role >= 2 && sortMode === 'manual') {
            var handle = document.createElement('span');
            handle.className = 'text-fg-muted cursor-grab shrink-0';
            handle.innerHTML = '<i class="fa-solid fa-grip-vertical text-[10px]"></i>';
            row.appendChild(handle);

            row.setAttribute('draggable', 'true');
            row.addEventListener('dragstart', onDragStart);
            row.addEventListener('dragover', onDragOver);
            row.addEventListener('dragleave', onDragLeave);
            row.addEventListener('drop', onDrop);
            row.addEventListener('dragend', onDragEnd);
          }

          // Entity icon.
          var icon = document.createElement('span');
          icon.className = 'w-5 h-5 flex items-center justify-center shrink-0 rounded text-[10px]';
          if (entity.type_color) {
            icon.style.color = entity.type_color;
          }
          icon.innerHTML = '<i class="fa-solid ' + (entity.type_icon || 'fa-file-lines') + '"></i>';
          row.appendChild(icon);

          // Entity name (clickable link).
          var link = document.createElement('a');
          link.href = '/campaigns/' + campaignId + '/entities/' + entity.id;
          link.className = 'flex-1 truncate text-xs text-fg hover:text-accent transition-colors';
          link.textContent = entity.name;
          row.appendChild(link);

          // Type badge.
          if (entity.type_name) {
            var badge = document.createElement('span');
            badge.className = 'text-[10px] text-fg-muted bg-surface-alt px-1.5 py-0.5 rounded shrink-0';
            badge.textContent = entity.type_name;
            row.appendChild(badge);
          }

          // Visibility toggle (Owner only).
          if (role >= 3) {
            var eyeBtn = document.createElement('button');
            eyeBtn.type = 'button';
            eyeBtn.className = 'p-1 text-[10px] rounded hover:bg-white/10 transition-colors shrink-0';
            eyeBtn.title = isHidden ? 'Show in sidebar' : 'Hide from sidebar';
            eyeBtn.innerHTML = '<i class="fa-solid ' + (isHidden ? 'fa-eye-slash text-fg-muted' : 'fa-eye text-fg-muted') + '"></i>';
            (function (eid) {
              eyeBtn.addEventListener('click', function (e) {
                e.preventDefault();
                e.stopPropagation();
                toggleEntityVisibility(eid);
              });
            })(entity.id);
            row.appendChild(eyeBtn);
          }

          list.appendChild(row);
        });

        el.appendChild(list);
      }

      // ---------------------------------------------------------------
      // Initialize
      // ---------------------------------------------------------------

      if (entityTypeId === 0) {
        // No type configured — show placeholder.
        el.innerHTML = '<div class="text-center py-8 text-fg-muted text-sm">' +
          '<i class="fa-solid fa-cog text-lg mb-2 block opacity-50"></i>' +
          'Configure this block to select an entity type.</div>';
        return;
      }

      // Fetch data in parallel.
      Promise.all([fetchEntities(), fetchSidebarConfig()])
        .then(function () {
          render();
        });
    },

    destroy: function (el) {
      el.innerHTML = '';
    }
  });
})();
