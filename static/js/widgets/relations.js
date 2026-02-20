/**
 * relations.js -- Chronicle Entity Relations Widget
 *
 * Displays bi-directional relations between entities, grouped by relation type.
 * Provides UI for adding and removing relations (Scribe+ only).
 * Auto-mounted by boot.js on elements with data-widget="relations".
 *
 * Config (from data-* attributes):
 *   data-relations-endpoint - Relations list/create endpoint (GET/POST),
 *                             e.g. /campaigns/:id/entities/:eid/relations
 *   data-relation-types-endpoint - Common relation types endpoint (GET),
 *                                  e.g. /campaigns/:id/relation-types
 *   data-entity-search-endpoint - Entity search endpoint (GET),
 *                                 e.g. /campaigns/:id/entities?q=...
 *   data-campaign-url - Base URL for entity links,
 *                       e.g. /campaigns/:id
 *   data-editable     - "true" if user can modify relations (Scribe+)
 *   data-csrf-token   - CSRF token for mutating requests
 */
Chronicle.register('relations', {
  init: function (el, config) {
    var state = {
      relations: [],
      relationTypes: [],
      isAdding: false,
      searchQuery: '',
      searchResults: [],
      selectedTarget: null,
      selectedType: '',
      customType: '',
      customReverseType: '',
      isSearching: false,
      isSubmitting: false,
      error: null
    };

    // Inject scoped styles once.
    if (!document.getElementById('relations-widget-styles')) {
      var style = document.createElement('style');
      style.id = 'relations-widget-styles';
      style.textContent = [
        '.rel-card { border: 1px solid #e5e7eb; border-radius: 8px; background: white; overflow: hidden; }',
        '.dark .rel-card { border-color: #374151; background: #1f2937; }',
        '.rel-group-header { display: flex; align-items: center; gap: 6px; padding: 8px 12px; font-size: 12px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #6b7280; border-bottom: 1px solid #f3f4f6; }',
        '.dark .rel-group-header { color: #9ca3af; border-bottom-color: #374151; }',
        '.rel-item { display: flex; align-items: center; gap: 8px; padding: 8px 12px; transition: background 0.15s; }',
        '.rel-item:hover { background: #f9fafb; }',
        '.dark .rel-item:hover { background: #374151; }',
        '.rel-item + .rel-item { border-top: 1px solid #f3f4f6; }',
        '.dark .rel-item + .rel-item { border-top-color: #374151; }',
        '.rel-icon { width: 28px; height: 28px; border-radius: 50%; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }',
        '.rel-icon i { font-size: 12px; color: white; }',
        '.rel-name { flex: 1; min-width: 0; }',
        '.rel-name a { font-size: 14px; font-weight: 500; color: #111827; text-decoration: none; }',
        '.dark .rel-name a { color: #f3f4f6; }',
        '.rel-name a:hover { color: #4f46e5; }',
        '.dark .rel-name a:hover { color: #818cf8; }',
        '.rel-type-badge { font-size: 11px; color: #9ca3af; }',
        '.rel-delete { opacity: 0; padding: 4px; cursor: pointer; color: #9ca3af; border: none; background: none; border-radius: 4px; transition: opacity 0.15s, color 0.15s; }',
        '.rel-item:hover .rel-delete { opacity: 1; }',
        '.rel-delete:hover { color: #ef4444; }',
        '.rel-add-btn { display: inline-flex; align-items: center; gap: 6px; padding: 6px 14px; font-size: 13px; font-weight: 500; color: #4f46e5; background: none; border: 1px dashed #c7d2fe; border-radius: 8px; cursor: pointer; transition: border-color 0.15s, background 0.15s; }',
        '.dark .rel-add-btn { color: #818cf8; border-color: #4338ca; }',
        '.rel-add-btn:hover { background: #eef2ff; border-color: #a5b4fc; }',
        '.dark .rel-add-btn:hover { background: #312e81; }',
        '.rel-modal { margin-top: 8px; border: 1px solid #e5e7eb; border-radius: 8px; background: white; box-shadow: 0 4px 12px rgba(0,0,0,0.1); padding: 12px; }',
        '.dark .rel-modal { border-color: #374151; background: #1f2937; }',
        '.rel-search { width: 100%; padding: 8px 10px; font-size: 13px; border: 1px solid #e5e7eb; border-radius: 6px; outline: none; background: transparent; color: inherit; }',
        '.dark .rel-search { border-color: #4b5563; color: #e5e7eb; }',
        '.rel-search:focus { border-color: #6366f1; box-shadow: 0 0 0 2px rgba(99,102,241,0.15); }',
        '.rel-results { max-height: 160px; overflow-y: auto; margin-top: 6px; }',
        '.rel-result { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 6px; cursor: pointer; font-size: 13px; color: #374151; }',
        '.dark .rel-result { color: #d1d5db; }',
        '.rel-result:hover { background: #f3f4f6; }',
        '.dark .rel-result:hover { background: #374151; }',
        '.rel-result.selected { background: #eef2ff; border: 1px solid #c7d2fe; }',
        '.dark .rel-result.selected { background: #312e81; border-color: #4338ca; }',
        '.rel-result-icon { width: 24px; height: 24px; border-radius: 50%; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }',
        '.rel-result-icon i { font-size: 10px; color: white; }',
        '.rel-type-select { width: 100%; padding: 8px 10px; font-size: 13px; border: 1px solid #e5e7eb; border-radius: 6px; outline: none; background: transparent; color: inherit; margin-top: 8px; }',
        '.dark .rel-type-select { border-color: #4b5563; color: #e5e7eb; background: #1f2937; }',
        '.rel-type-select:focus { border-color: #6366f1; box-shadow: 0 0 0 2px rgba(99,102,241,0.15); }',
        '.rel-custom-row { display: flex; gap: 6px; margin-top: 6px; }',
        '.rel-custom-row input { flex: 1; padding: 6px 8px; font-size: 12px; border: 1px solid #e5e7eb; border-radius: 6px; outline: none; background: transparent; color: inherit; }',
        '.dark .rel-custom-row input { border-color: #4b5563; color: #e5e7eb; }',
        '.rel-actions { display: flex; gap: 6px; margin-top: 10px; justify-content: flex-end; }',
        '.rel-submit { padding: 6px 16px; font-size: 13px; font-weight: 500; color: white; background: #4f46e5; border: none; border-radius: 6px; cursor: pointer; }',
        '.rel-submit:hover { background: #4338ca; }',
        '.rel-submit:disabled { opacity: 0.5; cursor: not-allowed; }',
        '.rel-cancel { padding: 6px 16px; font-size: 13px; font-weight: 500; color: #6b7280; background: none; border: 1px solid #e5e7eb; border-radius: 6px; cursor: pointer; }',
        '.dark .rel-cancel { color: #9ca3af; border-color: #4b5563; }',
        '.rel-cancel:hover { background: #f9fafb; }',
        '.dark .rel-cancel:hover { background: #374151; }',
        '.rel-empty { padding: 16px; text-align: center; font-size: 13px; color: #9ca3af; }',
        '.rel-error { padding: 12px; font-size: 13px; color: #ef4444; background: #fef2f2; border-radius: 6px; margin-bottom: 8px; }',
        '.dark .rel-error { background: #451a1a; }',
        '.rel-label { font-size: 12px; font-weight: 500; color: #6b7280; margin-top: 8px; margin-bottom: 4px; }',
        '.dark .rel-label { color: #9ca3af; }'
      ].join('\n');
      document.head.appendChild(style);
    }

    el._relationsState = state;

    var headers = { 'Accept': 'application/json' };
    if (config.csrfToken) {
      headers['X-CSRF-Token'] = config.csrfToken;
    }

    // Load relations and common types in parallel.
    var fetches = [
      fetch(config.relationsEndpoint, { headers: headers }).then(function (r) {
        if (!r.ok) throw new Error('Failed to load relations');
        return r.json();
      })
    ];

    // Only fetch relation types if editable (needed for the add UI).
    if (config.editable && config.relationTypesEndpoint) {
      fetches.push(
        fetch(config.relationTypesEndpoint, { headers: headers }).then(function (r) {
          if (!r.ok) throw new Error('Failed to load relation types');
          return r.json();
        })
      );
    }

    Promise.all(fetches).then(function (results) {
      state.relations = results[0] || [];
      if (results[1]) {
        state.relationTypes = results[1] || [];
      }
      render();
    }).catch(function (err) {
      console.error('[relations] Failed to load:', err);
      state.error = 'Failed to load relations. Please refresh the page.';
      render();
    });

    // --- Render ---

    function render() {
      el.innerHTML = '';

      // Error state.
      if (state.error && state.relations.length === 0) {
        var errorEl = document.createElement('div');
        errorEl.className = 'rel-error';
        errorEl.textContent = state.error;
        el.appendChild(errorEl);
        return;
      }

      // Group relations by type.
      var groups = {};
      var groupOrder = [];
      state.relations.forEach(function (rel) {
        if (!groups[rel.relationType]) {
          groups[rel.relationType] = [];
          groupOrder.push(rel.relationType);
        }
        groups[rel.relationType].push(rel);
      });

      // Render relation groups.
      if (groupOrder.length > 0) {
        var card = document.createElement('div');
        card.className = 'rel-card';

        groupOrder.forEach(function (type) {
          var headerEl = document.createElement('div');
          headerEl.className = 'rel-group-header';
          headerEl.innerHTML = '<i class="fa-solid fa-link" style="font-size:10px"></i> ' + escapeHtml(type);
          card.appendChild(headerEl);

          groups[type].forEach(function (rel) {
            card.appendChild(renderRelationItem(rel));
          });
        });

        el.appendChild(card);
      } else if (!config.editable) {
        // Read-only users see nothing when there are no relations.
        return;
      }

      // Add button (editable only).
      if (config.editable) {
        var addWrap = document.createElement('div');
        addWrap.style.marginTop = groupOrder.length > 0 ? '8px' : '0';

        if (!state.isAdding) {
          var addBtn = document.createElement('button');
          addBtn.type = 'button';
          addBtn.className = 'rel-add-btn';
          addBtn.innerHTML = '<i class="fa-solid fa-plus" style="font-size:10px"></i> Add Relation';
          addBtn.addEventListener('click', function () {
            state.isAdding = true;
            state.selectedTarget = null;
            state.selectedType = '';
            state.customType = '';
            state.customReverseType = '';
            state.searchQuery = '';
            state.searchResults = [];
            render();
          });
          addWrap.appendChild(addBtn);
        } else {
          addWrap.appendChild(renderAddModal());
        }

        el.appendChild(addWrap);
      }
    }

    // --- Render a single relation item ---

    function renderRelationItem(rel) {
      var item = document.createElement('div');
      item.className = 'rel-item';

      // Entity type icon.
      var icon = document.createElement('div');
      icon.className = 'rel-icon';
      icon.style.backgroundColor = rel.targetEntityColor || '#6b7280';
      icon.innerHTML = '<i class="fa-solid ' + escapeHtml(rel.targetEntityIcon || 'fa-file') + '"></i>';
      item.appendChild(icon);

      // Entity name link with tooltip preview support.
      var nameWrap = document.createElement('div');
      nameWrap.className = 'rel-name';
      var link = document.createElement('a');
      var entityHref = (config.campaignUrl || '') + '/entities/' + rel.targetEntityId;
      link.href = entityHref;
      link.textContent = rel.targetEntityName || 'Unknown Entity';
      link.setAttribute('data-entity-preview', entityHref + '/preview');
      nameWrap.appendChild(link);

      // Show entity type badge if available.
      if (rel.targetEntityType) {
        var badge = document.createElement('div');
        badge.className = 'rel-type-badge';
        badge.textContent = rel.targetEntityType;
        nameWrap.appendChild(badge);
      }
      item.appendChild(nameWrap);

      // Delete button (editable only).
      if (config.editable) {
        var delBtn = document.createElement('button');
        delBtn.type = 'button';
        delBtn.className = 'rel-delete';
        delBtn.title = 'Remove relation';
        delBtn.innerHTML = '<i class="fa-solid fa-xmark"></i>';
        delBtn.addEventListener('click', function (e) {
          e.stopPropagation();
          deleteRelation(rel.id);
        });
        item.appendChild(delBtn);
      }

      return item;
    }

    // --- Render add-relation modal ---

    function renderAddModal() {
      var modal = document.createElement('div');
      modal.className = 'rel-modal';

      // Step 1: Entity search.
      var searchLabel = document.createElement('div');
      searchLabel.className = 'rel-label';
      searchLabel.textContent = 'Search for an entity';
      modal.appendChild(searchLabel);

      var searchInput = document.createElement('input');
      searchInput.type = 'text';
      searchInput.className = 'rel-search';
      searchInput.placeholder = 'Type to search entities...';
      searchInput.value = state.searchQuery;

      var searchTimer = null;
      searchInput.addEventListener('input', function () {
        state.searchQuery = searchInput.value;
        clearTimeout(searchTimer);
        if (state.searchQuery.length >= 2) {
          searchTimer = setTimeout(function () {
            searchEntities(state.searchQuery);
          }, 300);
        } else {
          state.searchResults = [];
          renderSearchResults(resultsEl);
        }
      });
      searchInput.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') {
          state.isAdding = false;
          render();
        }
      });
      modal.appendChild(searchInput);

      // Search results.
      var resultsEl = document.createElement('div');
      resultsEl.className = 'rel-results';
      renderSearchResults(resultsEl);
      modal.appendChild(resultsEl);

      // Selected entity display.
      if (state.selectedTarget) {
        var selectedEl = document.createElement('div');
        selectedEl.className = 'rel-result selected';
        selectedEl.style.marginTop = '6px';

        var selIcon = document.createElement('div');
        selIcon.className = 'rel-result-icon';
        selIcon.style.backgroundColor = state.selectedTarget.type_color || '#6b7280';
        selIcon.innerHTML = '<i class="fa-solid ' + escapeHtml(state.selectedTarget.type_icon || 'fa-file') + '"></i>';
        selectedEl.appendChild(selIcon);

        var selName = document.createElement('span');
        selName.textContent = state.selectedTarget.name;
        selectedEl.appendChild(selName);

        modal.appendChild(selectedEl);
      }

      // Step 2: Relation type selector.
      var typeLabel = document.createElement('div');
      typeLabel.className = 'rel-label';
      typeLabel.textContent = 'Relation type';
      modal.appendChild(typeLabel);

      var typeSelect = document.createElement('select');
      typeSelect.className = 'rel-type-select';

      var defaultOpt = document.createElement('option');
      defaultOpt.value = '';
      defaultOpt.textContent = 'Choose a relation type...';
      typeSelect.appendChild(defaultOpt);

      // Add common types as options.
      state.relationTypes.forEach(function (rt) {
        var opt = document.createElement('option');
        opt.value = rt.forward + '|' + rt.reverse;
        opt.textContent = rt.forward + (rt.forward !== rt.reverse ? ' / ' + rt.reverse : '');
        if (state.selectedType === opt.value) opt.selected = true;
        typeSelect.appendChild(opt);
      });

      // "Custom" option.
      var customOpt = document.createElement('option');
      customOpt.value = '__custom__';
      customOpt.textContent = 'Custom...';
      if (state.selectedType === '__custom__') customOpt.selected = true;
      typeSelect.appendChild(customOpt);

      typeSelect.addEventListener('change', function () {
        state.selectedType = typeSelect.value;
        if (state.selectedType !== '__custom__') {
          state.customType = '';
          state.customReverseType = '';
        }
        renderCustomInputs();
      });
      modal.appendChild(typeSelect);

      // Custom type inputs (shown when "Custom..." is selected).
      var customContainer = document.createElement('div');
      customContainer.id = 'rel-custom-container';
      modal.appendChild(customContainer);

      function renderCustomInputs() {
        customContainer.innerHTML = '';
        if (state.selectedType !== '__custom__') return;

        var row = document.createElement('div');
        row.className = 'rel-custom-row';

        var fwdInput = document.createElement('input');
        fwdInput.type = 'text';
        fwdInput.placeholder = 'Forward type (e.g. "allied with")';
        fwdInput.value = state.customType;
        fwdInput.addEventListener('input', function () {
          state.customType = fwdInput.value;
        });
        row.appendChild(fwdInput);

        var revInput = document.createElement('input');
        revInput.type = 'text';
        revInput.placeholder = 'Reverse type (optional)';
        revInput.value = state.customReverseType;
        revInput.addEventListener('input', function () {
          state.customReverseType = revInput.value;
        });
        row.appendChild(revInput);

        customContainer.appendChild(row);
      }
      renderCustomInputs();

      // Action buttons.
      var actions = document.createElement('div');
      actions.className = 'rel-actions';

      var cancelBtn = document.createElement('button');
      cancelBtn.type = 'button';
      cancelBtn.className = 'rel-cancel';
      cancelBtn.textContent = 'Cancel';
      cancelBtn.addEventListener('click', function () {
        state.isAdding = false;
        render();
      });
      actions.appendChild(cancelBtn);

      var submitBtn = document.createElement('button');
      submitBtn.type = 'button';
      submitBtn.className = 'rel-submit';
      submitBtn.textContent = 'Add Relation';
      submitBtn.disabled = !canSubmit();
      submitBtn.addEventListener('click', function () {
        createRelation();
      });
      actions.appendChild(submitBtn);

      modal.appendChild(actions);

      // Focus search input after render.
      setTimeout(function () {
        searchInput.focus();
      }, 0);

      return modal;
    }

    function renderSearchResults(container) {
      container.innerHTML = '';

      if (state.isSearching) {
        var loading = document.createElement('div');
        loading.className = 'rel-empty';
        loading.textContent = 'Searching...';
        container.appendChild(loading);
        return;
      }

      if (state.searchResults.length === 0 && state.searchQuery.length >= 2) {
        var empty = document.createElement('div');
        empty.className = 'rel-empty';
        empty.textContent = 'No entities found';
        container.appendChild(empty);
        return;
      }

      state.searchResults.forEach(function (entity) {
        var result = document.createElement('div');
        result.className = 'rel-result';
        if (state.selectedTarget && state.selectedTarget.id === entity.id) {
          result.classList.add('selected');
        }

        var icon = document.createElement('div');
        icon.className = 'rel-result-icon';
        icon.style.backgroundColor = entity.type_color || '#6b7280';
        icon.innerHTML = '<i class="fa-solid ' + escapeHtml(entity.type_icon || 'fa-file') + '"></i>';
        result.appendChild(icon);

        var name = document.createElement('span');
        name.textContent = entity.name;
        result.appendChild(name);

        if (entity.type_name) {
          var typeBadge = document.createElement('span');
          typeBadge.style.cssText = 'font-size:11px; color:#9ca3af; margin-left:auto;';
          typeBadge.textContent = entity.type_name;
          result.appendChild(typeBadge);
        }

        result.addEventListener('click', function () {
          state.selectedTarget = entity;
          render();
        });

        container.appendChild(result);
      });
    }

    // --- API calls ---

    function searchEntities(query) {
      if (state.isSearching) return;
      state.isSearching = true;

      var searchHeaders = { 'Accept': 'application/json' };
      if (config.csrfToken) {
        searchHeaders['X-CSRF-Token'] = config.csrfToken;
      }

      // Use the entity search API endpoint which returns JSON when Accept
      // header includes application/json. Returns { results: [...], total: N }.
      var url = config.entitySearchEndpoint + '?q=' + encodeURIComponent(query);

      fetch(url, { headers: searchHeaders })
        .then(function (r) {
          if (!r.ok) throw new Error('Search failed');
          return r.json();
        })
        .then(function (data) {
          state.searchResults = data.results || data.entities || data || [];
          state.isSearching = false;
          var resultsEl = el.querySelector('.rel-results');
          if (resultsEl) renderSearchResults(resultsEl);
        })
        .catch(function (err) {
          console.error('[relations] Search failed:', err);
          state.searchResults = [];
          state.isSearching = false;
          var resultsEl = el.querySelector('.rel-results');
          if (resultsEl) renderSearchResults(resultsEl);
        });
    }

    function canSubmit() {
      if (!state.selectedTarget) return false;
      if (state.selectedType === '') return false;
      if (state.selectedType === '__custom__' && !state.customType.trim()) return false;
      return true;
    }

    function createRelation() {
      if (!canSubmit() || state.isSubmitting) return;
      state.isSubmitting = true;

      var relationType, reverseRelationType;
      if (state.selectedType === '__custom__') {
        relationType = state.customType.trim();
        reverseRelationType = state.customReverseType.trim() || relationType;
      } else {
        var parts = state.selectedType.split('|');
        relationType = parts[0];
        reverseRelationType = parts[1] || parts[0];
      }

      var reqHeaders = {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      };
      if (config.csrfToken) {
        reqHeaders['X-CSRF-Token'] = config.csrfToken;
      }

      fetch(config.relationsEndpoint, {
        method: 'POST',
        headers: reqHeaders,
        body: JSON.stringify({
          targetEntityId: state.selectedTarget.id,
          relationType: relationType,
          reverseRelationType: reverseRelationType
        })
      })
        .then(function (r) {
          if (!r.ok) {
            return r.json().then(function (err) {
              throw new Error(err.message || 'Failed to create relation');
            });
          }
          return r.json();
        })
        .then(function () {
          state.isSubmitting = false;
          state.isAdding = false;
          // Reload all relations to get the full joined data.
          return loadRelations();
        })
        .catch(function (err) {
          console.error('[relations] Create failed:', err);
          state.isSubmitting = false;
          state.error = err.message || 'Failed to create relation';
          render();
        });
    }

    function deleteRelation(relationId) {
      var reqHeaders = {
        'Accept': 'application/json'
      };
      if (config.csrfToken) {
        reqHeaders['X-CSRF-Token'] = config.csrfToken;
      }

      fetch(config.relationsEndpoint + '/' + relationId, {
        method: 'DELETE',
        headers: reqHeaders
      })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to delete relation');
          return loadRelations();
        })
        .catch(function (err) {
          console.error('[relations] Delete failed:', err);
        });
    }

    function loadRelations() {
      var loadHeaders = { 'Accept': 'application/json' };
      if (config.csrfToken) {
        loadHeaders['X-CSRF-Token'] = config.csrfToken;
      }

      return fetch(config.relationsEndpoint, { headers: loadHeaders })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to load relations');
          return r.json();
        })
        .then(function (data) {
          state.relations = data || [];
          state.error = null;
          render();
        })
        .catch(function (err) {
          console.error('[relations] Reload failed:', err);
        });
    }

    function escapeHtml(str) {
      var div = document.createElement('div');
      div.textContent = str;
      return div.innerHTML;
    }
  },

  destroy: function (el) {
    el.innerHTML = '';
    delete el._relationsState;
  }
});
