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
 *   data-quick-create-endpoint   -- Quick create endpoint for new folders
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
      var quickCreateEndpoint = config.quickCreateEndpoint;
      var sidebarConfigEndpoint = config.sidebarConfigEndpoint;

      if (!campaignId || !entitiesEndpoint) {
        console.error('[entity_manager] Missing required config');
        return;
      }

      // Widget state.
      var entities = [];
      var allTags = [];          // All tags in the campaign.
      var selectedTagName = '';  // Active tag filter (empty = show all).
      var sidebarConfig = null;
      var hiddenSet = {};
      var searchQuery = '';
      var sortMode = 'manual'; // manual | name
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
            // Build unique tag list from entity results.
            buildTagList();
            sortEntities();
            render();
          })
          .catch(function (err) {
            console.error('[entity_manager] fetch failed', err);
          });
      }

      function fetchAllTags() {
        return Chronicle.apiFetch('/campaigns/' + campaignId + '/tags')
          .then(function (res) { return res.ok ? res.json() : []; })
          .then(function (data) {
            allTags = data || [];
          })
          .catch(function () { allTags = []; });
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
      // Tag list from entity results
      // ---------------------------------------------------------------

      function buildTagList() {
        var seen = {};
        allTags = [];
        entities.forEach(function (ent) {
          if (ent.tags && ent.tags.length) {
            ent.tags.forEach(function (tag) {
              if (!seen[tag.name]) {
                seen[tag.name] = true;
                allTags.push(tag);
              }
            });
          }
        });
        allTags.sort(function (a, b) { return a.name.localeCompare(b.name); });
      }

      // ---------------------------------------------------------------
      // Filtering and sorting
      // ---------------------------------------------------------------

      function getFilteredEntities() {
        if (!selectedTagName) return entities;
        return entities.filter(function (ent) {
          if (!ent.tags || !ent.tags.length) return false;
          return ent.tags.some(function (t) { return t.name === selectedTagName; });
        });
      }

      function sortEntities() {
        if (sortMode === 'name') {
          entities.sort(function (a, b) {
            return (a.name || '').localeCompare(b.name || '');
          });
        }
        // manual sort uses server order (sort_order field)
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
      // Folder creation (Scribe+ only)
      // ---------------------------------------------------------------

      function createFolder() {
        if (!quickCreateEndpoint || role < 2 || !entityTypeId) return;

        var name = prompt('Folder name:');
        if (!name || !name.trim()) return;

        Chronicle.apiFetch(quickCreateEndpoint, {
          method: 'POST',
          body: { name: name.trim(), entity_type_id: entityTypeId }
        })
          .then(function (res) {
            if (!res.ok) throw new Error('Create failed');
            return res.json();
          })
          .then(function () {
            fetchEntities();
          })
          .catch(function () {
            Chronicle.notify('Failed to create folder', 'error');
          });
      }

      // ---------------------------------------------------------------
      // Tree helpers — build parent-child hierarchy from flat entity list
      // ---------------------------------------------------------------

      function buildTree(flatEntities) {
        var byId = {};
        var roots = [];
        flatEntities.forEach(function (ent) {
          byId[ent.id] = { entity: ent, children: [] };
        });
        flatEntities.forEach(function (ent) {
          var pid = ent.parent_id;
          if (pid && byId[pid]) {
            byId[pid].children.push(byId[ent.id]);
          } else {
            roots.push(byId[ent.id]);
          }
        });
        return roots;
      }

      // ---------------------------------------------------------------
      // Drag and drop reorder (Scribe+ only)
      // ---------------------------------------------------------------

      function onDragStart(e) {
        dragSrcId = e.currentTarget.getAttribute('data-entity-id');
        e.currentTarget.classList.add('opacity-40');
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', dragSrcId);
      }

      function onDragOver(e) {
        e.preventDefault();
        var row = e.currentTarget;
        var rect = row.getBoundingClientRect();
        var y = e.clientY - rect.top;

        // Clear previous indicators.
        row.classList.remove('border-t-2', 'border-accent', 'bg-accent/10');

        if (y < rect.height * 0.33) {
          // Top third: reorder above.
          e.dataTransfer.dropEffect = 'move';
          row.classList.add('border-t-2', 'border-accent');
        } else {
          // Bottom 2/3: reparent (nest inside).
          e.dataTransfer.dropEffect = 'move';
          row.classList.add('bg-accent/10');
        }
      }

      function onDragLeave(e) {
        e.currentTarget.classList.remove('border-t-2', 'border-accent', 'bg-accent/10');
      }

      function onDrop(e) {
        e.preventDefault();
        var row = e.currentTarget;
        row.classList.remove('border-t-2', 'border-accent', 'bg-accent/10');
        var targetId = row.getAttribute('data-entity-id');
        if (!dragSrcId || dragSrcId === targetId) return;

        var rect = row.getBoundingClientRect();
        var y = e.clientY - rect.top;
        var isReparent = y >= rect.height * 0.33;

        if (isReparent) {
          // Reparent: nest dragged entity inside target.
          Chronicle.apiFetch(reorderEndpoint + '/' + dragSrcId + '/reorder', {
            method: 'PUT',
            body: { sort_order: 0, parent_id: targetId }
          })
            .then(function () { fetchEntities(); })
            .catch(function () { Chronicle.notify('Failed to reparent', 'error'); });
        } else {
          // Reorder: move dragged entity before target (same parent level).
          var targetEntity = entities.find(function (ent) { return ent.id === targetId; });
          var targetParent = targetEntity ? (targetEntity.parent_id || null) : null;

          // Find indices among siblings.
          var srcIdx = -1, tgtIdx = -1;
          entities.forEach(function (ent, i) {
            if (ent.id === dragSrcId) srcIdx = i;
            if (ent.id === targetId) tgtIdx = i;
          });
          if (srcIdx === -1 || tgtIdx === -1) return;

          var moved = entities.splice(srcIdx, 1)[0];
          entities.splice(tgtIdx, 0, moved);
          render();

          Chronicle.apiFetch(reorderEndpoint + '/' + dragSrcId + '/reorder', {
            method: 'PUT',
            body: { sort_order: tgtIdx, parent_id: targetParent }
          }).catch(function () {
            Chronicle.notify('Failed to save order', 'error');
          });
        }
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

        // Toolbar: search + tag filter + sort.
        var toolbar = document.createElement('div');
        toolbar.className = 'flex items-center gap-2 mb-3 flex-wrap';

        // Search input.
        var searchWrap = document.createElement('div');
        searchWrap.className = 'relative flex-1 min-w-[140px]';
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

        // Tag filter dropdown (only if tags exist).
        if (allTags.length > 0) {
          var tagSelect = document.createElement('select');
          tagSelect.className = 'text-xs bg-surface-alt border border-border rounded-lg px-2 py-1.5 text-fg focus:outline-none focus:ring-1 focus:ring-accent';
          var allOpt = document.createElement('option');
          allOpt.value = '';
          allOpt.textContent = 'All tags';
          tagSelect.appendChild(allOpt);
          allTags.forEach(function (tag) {
            var opt = document.createElement('option');
            opt.value = tag.name;
            opt.textContent = tag.name;
            if (tag.name === selectedTagName) opt.selected = true;
            tagSelect.appendChild(opt);
          });
          tagSelect.addEventListener('change', function () {
            selectedTagName = tagSelect.value;
            render();
          });
          toolbar.appendChild(tagSelect);
        }

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

        // New entity + New Folder buttons (Scribe+).
        if (role >= 2) {
          var newBtn = document.createElement('a');
          newBtn.href = '/campaigns/' + campaignId + '/entities/new?type=' + entityTypeId;
          newBtn.className = 'inline-flex items-center gap-1 px-2 py-1.5 text-xs bg-accent/10 text-accent rounded-lg hover:bg-accent/20 transition-colors shrink-0';
          newBtn.innerHTML = '<i class="fa-solid fa-plus text-[9px]"></i> New';
          toolbar.appendChild(newBtn);

          if (quickCreateEndpoint) {
            var folderBtn = document.createElement('button');
            folderBtn.type = 'button';
            folderBtn.className = 'inline-flex items-center gap-1 px-2 py-1.5 text-xs bg-surface-alt border border-border text-fg-muted rounded-lg hover:bg-surface-alt/80 transition-colors shrink-0';
            folderBtn.innerHTML = '<i class="fa-solid fa-folder-plus text-[9px]"></i> Folder';
            folderBtn.addEventListener('click', createFolder);
            toolbar.appendChild(folderBtn);
          }
        }

        el.appendChild(toolbar);

        // Entity list — render as tree when not searching/filtering.
        var displayEntities = getFilteredEntities();

        if (displayEntities.length === 0) {
          var empty = document.createElement('div');
          empty.className = 'text-center py-8 text-fg-muted text-sm';
          empty.innerHTML = '<i class="fa-solid fa-file-circle-plus text-lg mb-2 block opacity-50"></i>' +
            (selectedTagName ? 'No entities with tag "' + selectedTagName + '"' : 'No entities found');
          el.appendChild(empty);
          return;
        }

        var list = document.createElement('div');
        list.className = 'entity-manager-list divide-y divide-border/50';

        // Use tree rendering when in manual sort with no search/filter active.
        var useTree = sortMode === 'manual' && !searchQuery && !selectedTagName;

        if (useTree) {
          var tree = buildTree(displayEntities);
          renderTreeNodes(list, tree, 0);
        } else {
          displayEntities.forEach(function (entity) {
            renderEntityRow(list, entity, 0, false);
          });
        }

        el.appendChild(list);
      }

      // Render tree nodes recursively with indentation.
      function renderTreeNodes(container, nodes, depth) {
        nodes.forEach(function (node) {
          renderEntityRow(container, node.entity, depth, node.children.length > 0);
          if (node.children.length > 0) {
            renderTreeNodes(container, node.children, depth + 1);
          }
        });
      }

      // Render a single entity row.
      function renderEntityRow(container, entity, depth, hasChildren) {
        var row = document.createElement('div');
        var isHidden = !!hiddenSet[entity.id];
        row.className = 'entity-manager-row flex items-center gap-2 py-1.5 rounded-md hover:bg-surface-alt transition-colors' +
          (isHidden ? ' opacity-40' : '');
        row.style.paddingLeft = (8 + depth * 16) + 'px';
        row.style.paddingRight = '8px';
        row.setAttribute('data-entity-id', entity.id);

        // Drag handle (Scribe+).
        if (role >= 2 && sortMode === 'manual' && !selectedTagName) {
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

        // Entity icon — folder icon for parents, type icon for leaves.
        var icon = document.createElement('span');
        icon.className = 'w-5 h-5 flex items-center justify-center shrink-0 rounded text-[10px]';
        if (hasChildren) {
          icon.innerHTML = '<i class="fa-solid fa-folder-open text-amber-400"></i>';
        } else {
          if (entity.type_color) icon.style.color = entity.type_color;
          icon.innerHTML = '<i class="fa-solid ' + (entity.type_icon || 'fa-file-lines') + '"></i>';
        }
        row.appendChild(icon);

        // Entity name (clickable link).
        var link = document.createElement('a');
        link.href = entity.url || ('/campaigns/' + campaignId + '/entities/' + entity.id);
        link.className = 'flex-1 truncate text-xs text-fg hover:text-accent transition-colors';
        link.textContent = entity.name;
        row.appendChild(link);

        // Tag chips (compact, inline).
        if (entity.tags && entity.tags.length) {
          var tagWrap = document.createElement('div');
          tagWrap.className = 'flex items-center gap-0.5 shrink-0';
          entity.tags.forEach(function (tag) {
            var chip = document.createElement('span');
            chip.className = 'inline-block px-1 py-0 text-[9px] rounded';
            chip.style.backgroundColor = tag.color + '22';
            chip.style.color = tag.color;
            chip.textContent = tag.name;
            tagWrap.appendChild(chip);
          });
          row.appendChild(tagWrap);
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

        container.appendChild(row);
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

      // Fetch data in parallel then render.
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
