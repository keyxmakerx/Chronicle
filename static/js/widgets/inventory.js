/**
 * inventory.js -- Chronicle Inventory Widget
 *
 * Displays a character's inventory as a list of items (entities linked via
 * "Has Item" relations) with quantity, equipped, and attuned metadata.
 * Items can be searched and added from existing item entities.
 *
 * Mount via:
 *   <div data-widget="inventory"
 *        data-relations-endpoint="/campaigns/:id/entities/:eid/relations"
 *        data-entity-search-endpoint="/campaigns/:id/entities/search"
 *        data-campaign-url="/campaigns/:id"
 *        data-relation-type="Has Item"
 *        data-reverse-relation-type="In Inventory Of"
 *        data-editable="true"
 *        data-csrf-token="..."
 *   ></div>
 */
Chronicle.register('inventory', {
  init: function (el) {
    var relationsEndpoint = el.dataset.relationsEndpoint;
    var entitySearchEndpoint = el.dataset.entitySearchEndpoint;
    var campaignUrl = el.dataset.campaignUrl;
    var relationType = el.dataset.relationType || 'Has Item';
    var reverseRelationType = el.dataset.reverseRelationType || 'In Inventory Of';
    var editable = el.dataset.editable === 'true';
    var csrfToken = el.dataset.csrfToken || '';

    // Internal state.
    var state = {
      items: [],
      loading: true,
      addMode: false,
      searchQuery: '',
      searchResults: [],
      searchTimer: null,
    };
    el._invState = state;

    // --- Styles ---

    var style = document.createElement('style');
    style.textContent = [
      '.inv { font-size: 0.875rem; }',
      '.inv-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem; }',
      '.inv-header h3 { font-size: 0.875rem; font-weight: 600; margin: 0; color: var(--text-primary, #1f2937); }',
      '.dark .inv-header h3 { color: #e5e7eb; }',
      '.inv-add-btn { font-size: 0.75rem; padding: 0.25rem 0.5rem; border-radius: 0.25rem; background: #3b82f6; color: #fff; border: none; cursor: pointer; }',
      '.inv-add-btn:hover { background: #2563eb; }',
      '.inv-empty { color: #9ca3af; font-style: italic; padding: 1rem; text-align: center; }',
      '.inv-list { display: flex; flex-direction: column; gap: 0.375rem; }',
      '.inv-item { display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem; border-radius: 0.375rem; background: var(--bg-secondary, #f9fafb); border: 1px solid var(--border, #e5e7eb); }',
      '.dark .inv-item { background: #1f2937; border-color: #374151; }',
      '.inv-item-icon { width: 1.75rem; height: 1.75rem; display: flex; align-items: center; justify-content: center; border-radius: 0.25rem; flex-shrink: 0; font-size: 0.75rem; }',
      '.inv-item-info { flex: 1; min-width: 0; }',
      '.inv-item-name { font-weight: 500; color: var(--text-primary, #1f2937); text-decoration: none; display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-size: 0.8125rem; }',
      '.inv-item-name:hover { text-decoration: underline; }',
      '.dark .inv-item-name { color: #e5e7eb; }',
      '.inv-item-meta { font-size: 0.6875rem; color: #6b7280; margin-top: 1px; }',
      '.inv-item-controls { display: flex; align-items: center; gap: 0.375rem; flex-shrink: 0; }',
      '.inv-qty { width: 2.5rem; text-align: center; font-size: 0.75rem; padding: 0.125rem; border: 1px solid #d1d5db; border-radius: 0.25rem; }',
      '.dark .inv-qty { background: #374151; border-color: #4b5563; color: #e5e7eb; }',
      '.inv-toggle { font-size: 0.5625rem; padding: 0.125rem 0.375rem; border-radius: 9999px; border: none; cursor: pointer; white-space: nowrap; }',
      '.inv-toggle-on { background: #d1fae5; color: #065f46; }',
      '.inv-toggle-off { background: #f3f4f6; color: #9ca3af; }',
      '.dark .inv-toggle-off { background: #374151; color: #6b7280; }',
      '.inv-remove { color: #ef4444; cursor: pointer; border: none; background: none; font-size: 0.75rem; padding: 0.125rem; }',
      '.inv-remove:hover { color: #dc2626; }',
      '.inv-search { width: 100%; padding: 0.375rem 0.5rem; border: 1px solid #d1d5db; border-radius: 0.25rem; font-size: 0.8125rem; margin-bottom: 0.5rem; }',
      '.dark .inv-search { background: #374151; border-color: #4b5563; color: #e5e7eb; }',
      '.inv-search-results { max-height: 10rem; overflow-y: auto; }',
      '.inv-search-item { display: flex; align-items: center; gap: 0.5rem; padding: 0.375rem 0.5rem; cursor: pointer; border-radius: 0.25rem; font-size: 0.8125rem; }',
      '.inv-search-item:hover { background: #f3f4f6; }',
      '.dark .inv-search-item:hover { background: #374151; }',
      '.inv-add-panel { border: 1px solid #e5e7eb; border-radius: 0.375rem; padding: 0.75rem; margin-bottom: 0.75rem; }',
      '.dark .inv-add-panel { border-color: #374151; }',
    ].join('\n');
    el.appendChild(style);

    // --- Render ---

    function render() {
      // Remove old content (keep style).
      Array.from(el.children).forEach(function (child) {
        if (child.tagName !== 'STYLE') el.removeChild(child);
      });

      var wrap = document.createElement('div');
      wrap.className = 'inv';

      // Header.
      var header = document.createElement('div');
      header.className = 'inv-header';
      var h3 = document.createElement('h3');
      h3.innerHTML = '<i class="fa-solid fa-shield-halved" style="margin-right:0.375rem;opacity:0.6;font-size:0.75rem"></i>Inventory';
      header.appendChild(h3);
      if (editable) {
        var addBtn = document.createElement('button');
        addBtn.className = 'inv-add-btn';
        addBtn.textContent = state.addMode ? 'Cancel' : '+ Add Item';
        addBtn.addEventListener('click', function () {
          state.addMode = !state.addMode;
          state.searchQuery = '';
          state.searchResults = [];
          render();
        });
        header.appendChild(addBtn);
      }
      wrap.appendChild(header);

      // Add panel.
      if (state.addMode) {
        wrap.appendChild(renderAddPanel());
      }

      // Items list.
      if (state.loading) {
        var loading = document.createElement('div');
        loading.className = 'inv-empty';
        loading.textContent = 'Loading inventory...';
        wrap.appendChild(loading);
      } else if (state.items.length === 0) {
        var empty = document.createElement('div');
        empty.className = 'inv-empty';
        empty.textContent = 'No items in inventory.';
        wrap.appendChild(empty);
      } else {
        var list = document.createElement('div');
        list.className = 'inv-list';
        state.items.forEach(function (item) {
          list.appendChild(renderItem(item));
        });
        wrap.appendChild(list);
      }

      el.appendChild(wrap);
    }

    function renderItem(item) {
      var meta = parseMetadata(item.metadata);
      var row = document.createElement('div');
      row.className = 'inv-item';

      // Icon.
      var icon = document.createElement('div');
      icon.className = 'inv-item-icon';
      icon.style.background = (item.targetEntityColor || '#6b7280') + '22';
      icon.style.color = item.targetEntityColor || '#6b7280';
      icon.innerHTML = '<i class="fa-solid ' + Chronicle.escapeHtml(item.targetEntityIcon || 'fa-gem') + '"></i>';
      row.appendChild(icon);

      // Info.
      var info = document.createElement('div');
      info.className = 'inv-item-info';
      var nameEl = document.createElement('a');
      nameEl.className = 'inv-item-name';
      nameEl.textContent = item.targetEntityName || 'Unknown';
      nameEl.href = campaignUrl + '/entities/' + item.targetEntityId;
      info.appendChild(nameEl);
      if (item.targetEntityType) {
        var metaEl = document.createElement('div');
        metaEl.className = 'inv-item-meta';
        metaEl.textContent = item.targetEntityType;
        info.appendChild(metaEl);
      }
      row.appendChild(info);

      // Controls.
      var controls = document.createElement('div');
      controls.className = 'inv-item-controls';

      // Quantity input.
      if (editable) {
        var qtyInput = document.createElement('input');
        qtyInput.type = 'number';
        qtyInput.className = 'inv-qty';
        qtyInput.value = meta.quantity || 1;
        qtyInput.min = 1;
        qtyInput.title = 'Quantity';
        qtyInput.addEventListener('change', function () {
          meta.quantity = parseInt(qtyInput.value) || 1;
          updateMetadata(item.id, meta);
        });
        controls.appendChild(qtyInput);
      } else {
        var qtySpan = document.createElement('span');
        qtySpan.style.fontSize = '0.75rem';
        qtySpan.style.color = '#6b7280';
        qtySpan.textContent = 'x' + (meta.quantity || 1);
        controls.appendChild(qtySpan);
      }

      // Equipped toggle.
      var eqBtn = document.createElement('button');
      eqBtn.className = 'inv-toggle ' + (meta.equipped ? 'inv-toggle-on' : 'inv-toggle-off');
      eqBtn.textContent = meta.equipped ? 'Equipped' : 'Stowed';
      eqBtn.title = meta.equipped ? 'Click to unequip' : 'Click to equip';
      if (editable) {
        eqBtn.addEventListener('click', function () {
          meta.equipped = !meta.equipped;
          updateMetadata(item.id, meta);
          render();
        });
      } else {
        eqBtn.style.cursor = 'default';
      }
      controls.appendChild(eqBtn);

      // Attuned toggle (only if metadata has attuned field).
      if (meta.attuned !== undefined) {
        var attBtn = document.createElement('button');
        attBtn.className = 'inv-toggle ' + (meta.attuned ? 'inv-toggle-on' : 'inv-toggle-off');
        attBtn.textContent = meta.attuned ? 'Attuned' : 'Not Attuned';
        if (editable) {
          attBtn.addEventListener('click', function () {
            meta.attuned = !meta.attuned;
            updateMetadata(item.id, meta);
            render();
          });
        } else {
          attBtn.style.cursor = 'default';
        }
        controls.appendChild(attBtn);
      }

      // Remove button.
      if (editable) {
        var removeBtn = document.createElement('button');
        removeBtn.className = 'inv-remove';
        removeBtn.innerHTML = '<i class="fa-solid fa-xmark"></i>';
        removeBtn.title = 'Remove from inventory';
        removeBtn.addEventListener('click', function () {
          removeItem(item.id);
        });
        controls.appendChild(removeBtn);
      }

      row.appendChild(controls);
      return row;
    }

    function renderAddPanel() {
      var panel = document.createElement('div');
      panel.className = 'inv-add-panel';

      var searchInput = document.createElement('input');
      searchInput.type = 'text';
      searchInput.className = 'inv-search';
      searchInput.placeholder = 'Search items to add...';
      searchInput.value = state.searchQuery;
      searchInput.addEventListener('input', function () {
        state.searchQuery = searchInput.value;
        clearTimeout(state.searchTimer);
        state.searchTimer = setTimeout(function () {
          searchEntities(state.searchQuery);
        }, 300);
      });
      panel.appendChild(searchInput);

      var results = document.createElement('div');
      results.className = 'inv-search-results';
      state.searchResults.forEach(function (entity) {
        var item = document.createElement('div');
        item.className = 'inv-search-item';
        item.innerHTML = '<i class="fa-solid ' + Chronicle.escapeHtml(entity.type_icon || 'fa-gem') + '" style="color:' + Chronicle.escapeAttr(entity.type_color || '#6b7280') + ';font-size:0.75rem"></i> ' +
          '<span>' + Chronicle.escapeHtml(entity.name) + '</span>';
        item.addEventListener('click', function () {
          addItem(entity.id);
        });
        results.appendChild(item);
      });
      panel.appendChild(results);

      return panel;
    }

    // --- API Calls ---

    function loadItems() {
      state.loading = true;
      render();

      fetch(relationsEndpoint, {
        headers: { 'Accept': 'application/json' },
        credentials: 'same-origin',
      })
        .then(function (res) { return res.json(); })
        .then(function (data) {
          // Filter to only "Has Item" type relations.
          state.items = (data || []).filter(function (r) {
            return r.relationType === relationType;
          });
          state.loading = false;
          render();
        })
        .catch(function (err) {
          console.error('Inventory: Failed to load items', err);
          state.loading = false;
          render();
        });
    }

    function searchEntities(query) {
      if (!query || query.length < 2) {
        state.searchResults = [];
        render();
        return;
      }
      fetch(entitySearchEndpoint + '?q=' + encodeURIComponent(query), {
        headers: { 'Accept': 'application/json' },
        credentials: 'same-origin',
      })
        .then(function (res) { return res.json(); })
        .then(function (data) {
          state.searchResults = data?.data || data || [];
          render();
        })
        .catch(function (err) {
          console.error('Inventory: Search failed', err);
        });
    }

    function addItem(targetEntityId) {
      var body = {
        targetEntityId: targetEntityId,
        relationType: relationType,
        reverseRelationType: reverseRelationType,
        metadata: JSON.stringify({ quantity: 1, equipped: false }),
      };

      fetch(relationsEndpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': csrfToken,
        },
        credentials: 'same-origin',
        body: JSON.stringify(body),
      })
        .then(function (res) {
          if (!res.ok) throw new Error('Failed to add item');
          state.addMode = false;
          state.searchQuery = '';
          state.searchResults = [];
          loadItems();
        })
        .catch(function (err) {
          console.error('Inventory: Failed to add item', err);
        });
    }

    function removeItem(relationId) {
      fetch(relationsEndpoint + '/' + relationId, {
        method: 'DELETE',
        headers: { 'X-CSRF-Token': csrfToken },
        credentials: 'same-origin',
      })
        .then(function (res) {
          if (!res.ok) throw new Error('Failed to remove item');
          loadItems();
        })
        .catch(function (err) {
          console.error('Inventory: Failed to remove item', err);
        });
    }

    function updateMetadata(relationId, meta) {
      fetch(relationsEndpoint + '/' + relationId + '/metadata', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': csrfToken,
        },
        credentials: 'same-origin',
        body: JSON.stringify({ metadata: JSON.stringify(meta) }),
      })
        .catch(function (err) {
          console.error('Inventory: Failed to update metadata', err);
        });
    }

    // --- Helpers ---

    function parseMetadata(raw) {
      if (!raw) return { quantity: 1, equipped: false };
      try {
        return typeof raw === 'string' ? JSON.parse(raw) : raw;
      } catch (e) {
        return { quantity: 1, equipped: false };
      }
    }

    // --- Init ---
    loadItems();
  },
});
