/**
 * shop_inventory.js -- Chronicle Shop Inventory Widget
 *
 * Displays a shop's inventory as a list of items with price, quantity, and
 * stock controls. Items can be entity relations (type "sells") or custom
 * items (name stored in relation metadata, no linked entity page).
 *
 * Mount via:
 *   <div data-widget="shop_inventory"
 *        data-relations-endpoint="/campaigns/:id/entities/:eid/relations"
 *        data-entity-search-endpoint="/campaigns/:id/entities/search"
 *        data-quick-create-endpoint="/campaigns/:id/entities/quick-create"
 *        data-campaign-url="/campaigns/:id"
 *        data-editable="true"
 *        data-csrf-token="..."
 *   ></div>
 */
Chronicle.register('shop_inventory', {
  init: function (el) {
    var relationsEndpoint = el.dataset.relationsEndpoint;
    var entitySearchEndpoint = el.dataset.entitySearchEndpoint;
    var quickCreateEndpoint = el.dataset.quickCreateEndpoint;
    var campaignUrl = el.dataset.campaignUrl;
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
      entityTypes: [],
    };
    el._shopState = state;

    // --- Styles ---

    var style = document.createElement('style');
    style.textContent = [
      '.shop-inv { font-size: 0.875rem; }',
      '.shop-inv-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem; }',
      '.shop-inv-header h3 { font-size: 0.875rem; font-weight: 600; margin: 0; color: var(--text-primary, #1f2937); }',
      '.dark .shop-inv-header h3 { color: #e5e7eb; }',
      '.shop-inv-add-btn { font-size: 0.75rem; padding: 0.25rem 0.5rem; border-radius: 0.25rem; background: #f97316; color: #fff; border: none; cursor: pointer; }',
      '.shop-inv-add-btn:hover { background: #ea580c; }',
      '.shop-inv-empty { color: #9ca3af; font-style: italic; padding: 1rem; text-align: center; }',
      '.shop-inv-list { display: flex; flex-direction: column; gap: 0.5rem; }',
      '.shop-inv-item { display: flex; align-items: center; gap: 0.75rem; padding: 0.5rem; border-radius: 0.375rem; background: var(--bg-secondary, #f9fafb); border: 1px solid var(--border, #e5e7eb); }',
      '.dark .shop-inv-item { background: #1f2937; border-color: #374151; }',
      '.shop-inv-item-icon { width: 2rem; height: 2rem; display: flex; align-items: center; justify-content: center; border-radius: 0.25rem; flex-shrink: 0; }',
      '.shop-inv-item-info { flex: 1; min-width: 0; }',
      '.shop-inv-item-name { font-weight: 500; color: var(--text-primary, #1f2937); text-decoration: none; display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }',
      '.shop-inv-item-name:hover { text-decoration: underline; }',
      '.shop-inv-item-name-plain { font-weight: 500; color: var(--text-primary, #1f2937); display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }',
      '.dark .shop-inv-item-name, .dark .shop-inv-item-name-plain { color: #e5e7eb; }',
      '.shop-inv-item-meta { font-size: 0.75rem; color: #6b7280; margin-top: 0.125rem; }',
      '.shop-inv-item-controls { display: flex; align-items: center; gap: 0.5rem; flex-shrink: 0; }',
      '.shop-inv-price { font-weight: 600; color: #d97706; white-space: nowrap; }',
      '.shop-inv-qty { display: flex; align-items: center; gap: 0.25rem; }',
      '.shop-inv-qty input { width: 3rem; text-align: center; font-size: 0.75rem; padding: 0.125rem; border: 1px solid #d1d5db; border-radius: 0.25rem; }',
      '.dark .shop-inv-qty input { background: #374151; border-color: #4b5563; color: #e5e7eb; }',
      '.shop-inv-stock-toggle { font-size: 0.625rem; padding: 0.125rem 0.375rem; border-radius: 9999px; border: none; cursor: pointer; }',
      '.shop-inv-stock-in { background: #d1fae5; color: #065f46; }',
      '.shop-inv-stock-out { background: #fee2e2; color: #991b1b; }',
      '.shop-inv-remove { color: #ef4444; cursor: pointer; border: none; background: none; font-size: 0.875rem; padding: 0.25rem; }',
      '.shop-inv-remove:hover { color: #dc2626; }',
      '.shop-inv-search { width: 100%; padding: 0.375rem 0.5rem; border: 1px solid #d1d5db; border-radius: 0.25rem; font-size: 0.8125rem; margin-bottom: 0.5rem; }',
      '.dark .shop-inv-search { background: #374151; border-color: #4b5563; color: #e5e7eb; }',
      '.shop-inv-search-results { max-height: 12rem; overflow-y: auto; }',
      '.shop-inv-search-item { display: flex; align-items: center; gap: 0.5rem; padding: 0.375rem 0.5rem; cursor: pointer; border-radius: 0.25rem; }',
      '.shop-inv-search-item:hover { background: #f3f4f6; }',
      '.dark .shop-inv-search-item:hover { background: #374151; }',
      '.shop-inv-price-input { width: 4rem; padding: 0.25rem; border: 1px solid #d1d5db; border-radius: 0.25rem; font-size: 0.75rem; margin-top: 0.25rem; }',
      '.dark .shop-inv-price-input { background: #374151; border-color: #4b5563; color: #e5e7eb; }',
      '.shop-inv-add-panel { border: 1px solid #e5e7eb; border-radius: 0.375rem; padding: 0.75rem; margin-bottom: 0.75rem; }',
      '.dark .shop-inv-add-panel { border-color: #374151; }',
      '.shop-inv-create-section { margin-top: 0.5rem; padding-top: 0.5rem; border-top: 1px solid #e5e7eb; }',
      '.dark .shop-inv-create-section { border-color: #374151; }',
      '.shop-inv-create-row { display: flex; gap: 0.375rem; align-items: center; margin-top: 0.375rem; }',
      '.shop-inv-create-input { flex: 1; padding: 0.3rem 0.5rem; border: 1px solid #d1d5db; border-radius: 0.25rem; font-size: 0.8125rem; }',
      '.dark .shop-inv-create-input { background: #374151; border-color: #4b5563; color: #e5e7eb; }',
      '.shop-inv-type-select { padding: 0.3rem 0.375rem; border: 1px solid #d1d5db; border-radius: 0.25rem; font-size: 0.75rem; max-width: 7rem; }',
      '.dark .shop-inv-type-select { background: #374151; border-color: #4b5563; color: #e5e7eb; }',
      '.shop-inv-action-btn { font-size: 0.75rem; padding: 0.3rem 0.625rem; border-radius: 0.25rem; color: #fff; border: none; cursor: pointer; white-space: nowrap; }',
      '.shop-inv-action-btn:disabled { opacity: 0.5; cursor: not-allowed; }',
      '.shop-inv-action-btn.green { background: #10b981; }',
      '.shop-inv-action-btn.green:hover:not(:disabled) { background: #059669; }',
      '.shop-inv-action-btn.blue { background: #3b82f6; }',
      '.shop-inv-action-btn.blue:hover:not(:disabled) { background: #2563eb; }',
      '.shop-inv-label { font-size: 0.6875rem; color: #6b7280; font-weight: 500; }',
    ].join('\n');
    el.appendChild(style);

    // --- Render ---

    function render() {
      // Remove old content (keep style).
      var children = el.children;
      for (var i = children.length - 1; i >= 0; i--) {
        if (children[i].tagName !== 'STYLE') el.removeChild(children[i]);
      }

      var wrapper = document.createElement('div');
      wrapper.className = 'shop-inv';

      // Header.
      var header = document.createElement('div');
      header.className = 'shop-inv-header';
      var title = document.createElement('h3');
      title.innerHTML = '<i class="fas fa-store"></i> Inventory';
      header.appendChild(title);

      if (editable) {
        var addBtn = document.createElement('button');
        addBtn.className = 'shop-inv-add-btn';
        addBtn.textContent = '+ Add Item';
        addBtn.onclick = function () {
          state.addMode = !state.addMode;
          state.searchQuery = '';
          state.searchResults = [];
          render();
        };
        header.appendChild(addBtn);
      }
      wrapper.appendChild(header);

      // Add item panel.
      if (state.addMode && editable) {
        wrapper.appendChild(renderAddPanel());
      }

      // Items list.
      if (state.loading) {
        var loading = document.createElement('div');
        loading.className = 'shop-inv-empty';
        loading.textContent = 'Loading inventory...';
        wrapper.appendChild(loading);
      } else if (state.items.length === 0) {
        var empty = document.createElement('div');
        empty.className = 'shop-inv-empty';
        empty.textContent = editable ? 'No items yet. Click "+ Add Item" to stock the shop.' : 'This shop has no items.';
        wrapper.appendChild(empty);
      } else {
        var list = document.createElement('div');
        list.className = 'shop-inv-list';
        for (var i = 0; i < state.items.length; i++) {
          list.appendChild(renderItem(state.items[i]));
        }
        wrapper.appendChild(list);
      }

      el.appendChild(wrapper);
    }

    function renderItem(item) {
      var meta = item.metadata || {};
      var isCustom = !item.targetEntityID;
      var row = document.createElement('div');
      row.className = 'shop-inv-item';

      // Icon.
      var iconWrap = document.createElement('div');
      iconWrap.className = 'shop-inv-item-icon';
      var iconColor = isCustom ? '#6b7280' : (item.targetEntityColor || '#6b7280');
      var iconClass = isCustom ? 'fa-box' : (item.targetEntityIcon || 'fa-box');
      iconWrap.style.backgroundColor = iconColor + '20';
      iconWrap.style.color = iconColor;
      iconWrap.innerHTML = '<i class="fas ' + Chronicle.escapeHtml(iconClass) + '"></i>';
      row.appendChild(iconWrap);

      // Info.
      var info = document.createElement('div');
      info.className = 'shop-inv-item-info';

      if (isCustom) {
        // Custom item (no entity page) — plain text name, editable inline.
        var nameSpan = document.createElement('span');
        nameSpan.className = 'shop-inv-item-name-plain';
        nameSpan.textContent = meta.custom_name || 'Unnamed item';
        info.appendChild(nameSpan);
        var customBadge = document.createElement('div');
        customBadge.className = 'shop-inv-item-meta';
        customBadge.textContent = 'Custom item';
        info.appendChild(customBadge);
      } else {
        var nameLink = document.createElement('a');
        nameLink.className = 'shop-inv-item-name';
        nameLink.href = campaignUrl + '/entities/' + item.targetEntitySlug;
        nameLink.setAttribute('data-hx-boost', 'true');
        nameLink.textContent = item.targetEntityName;
        info.appendChild(nameLink);

        if (item.targetEntityType) {
          var typeBadge = document.createElement('div');
          typeBadge.className = 'shop-inv-item-meta';
          typeBadge.textContent = item.targetEntityType;
          info.appendChild(typeBadge);
        }
      }
      row.appendChild(info);

      // Controls.
      var controls = document.createElement('div');
      controls.className = 'shop-inv-item-controls';

      // Price.
      if (editable) {
        var priceInput = document.createElement('input');
        priceInput.className = 'shop-inv-price-input';
        priceInput.type = 'number';
        priceInput.min = '0';
        priceInput.step = '0.01';
        priceInput.value = meta.price || 0;
        priceInput.title = 'Price';
        priceInput.onchange = function () {
          meta.price = parseFloat(priceInput.value) || 0;
          updateMetadata(item.id, meta);
        };
        var priceLabel = document.createElement('span');
        priceLabel.className = 'shop-inv-price';
        priceLabel.textContent = 'gp';
        controls.appendChild(priceInput);
        controls.appendChild(priceLabel);
      } else {
        var priceSpan = document.createElement('span');
        priceSpan.className = 'shop-inv-price';
        priceSpan.textContent = (meta.price || 0) + ' gp';
        controls.appendChild(priceSpan);
      }

      // Quantity.
      if (editable) {
        var qtyWrap = document.createElement('div');
        qtyWrap.className = 'shop-inv-qty';
        var qtyLabel = document.createElement('span');
        qtyLabel.textContent = 'Qty:';
        qtyLabel.style.fontSize = '0.75rem';
        qtyLabel.style.color = '#6b7280';
        var qtyInput = document.createElement('input');
        qtyInput.type = 'number';
        qtyInput.min = '0';
        qtyInput.value = meta.quantity != null ? meta.quantity : '';
        qtyInput.placeholder = '\u221E';
        qtyInput.onchange = function () {
          var val = qtyInput.value;
          meta.quantity = val === '' ? null : parseInt(val, 10);
          updateMetadata(item.id, meta);
        };
        qtyWrap.appendChild(qtyLabel);
        qtyWrap.appendChild(qtyInput);
        controls.appendChild(qtyWrap);
      } else if (meta.quantity != null) {
        var qtySpan = document.createElement('span');
        qtySpan.style.fontSize = '0.75rem';
        qtySpan.style.color = '#6b7280';
        qtySpan.textContent = 'Qty: ' + meta.quantity;
        controls.appendChild(qtySpan);
      }

      // Stock toggle.
      if (editable) {
        var stockBtn = document.createElement('button');
        stockBtn.className = 'shop-inv-stock-toggle ' + (meta.in_stock !== false ? 'shop-inv-stock-in' : 'shop-inv-stock-out');
        stockBtn.textContent = meta.in_stock !== false ? 'In Stock' : 'Out';
        stockBtn.onclick = function () {
          meta.in_stock = meta.in_stock === false;
          updateMetadata(item.id, meta);
          render();
        };
        controls.appendChild(stockBtn);

        // Remove button.
        var removeBtn = document.createElement('button');
        removeBtn.className = 'shop-inv-remove';
        removeBtn.innerHTML = '<i class="fas fa-trash-alt"></i>';
        removeBtn.title = 'Remove from shop';
        removeBtn.onclick = function () {
          removeItem(item.id);
        };
        controls.appendChild(removeBtn);
      } else if (meta.in_stock === false) {
        var outBadge = document.createElement('span');
        outBadge.className = 'shop-inv-stock-toggle shop-inv-stock-out';
        outBadge.textContent = 'Out of Stock';
        controls.appendChild(outBadge);
      }

      row.appendChild(controls);
      return row;
    }

    function renderAddPanel() {
      var panel = document.createElement('div');
      panel.className = 'shop-inv-add-panel';

      // Single search input for finding existing entities.
      var searchInput = document.createElement('input');
      searchInput.className = 'shop-inv-search';
      searchInput.type = 'text';
      searchInput.placeholder = 'Search for an item to add...';
      searchInput.value = state.searchQuery;
      searchInput.oninput = function () {
        state.searchQuery = searchInput.value;
        clearTimeout(state.searchTimer);
        if (state.searchQuery.length >= 2) {
          state.searchTimer = setTimeout(function () { searchItems(state.searchQuery); }, 300);
        } else {
          state.searchResults = [];
          renderSearchResults(resultsDiv);
        }
      };
      panel.appendChild(searchInput);

      // Search results.
      var resultsDiv = document.createElement('div');
      resultsDiv.className = 'shop-inv-search-results';
      renderSearchResults(resultsDiv);
      panel.appendChild(resultsDiv);

      // "Create new item" section — always visible below search results.
      // Creates a new entity page and adds it to the shop in one step.
      if (quickCreateEndpoint) {
        var createSection = document.createElement('div');
        createSection.className = 'shop-inv-create-section';

        var createLabel = document.createElement('div');
        createLabel.className = 'shop-inv-label';
        createLabel.textContent = 'Or create a new item:';
        createSection.appendChild(createLabel);

        var createRow = document.createElement('div');
        createRow.className = 'shop-inv-create-row';

        var nameInput = document.createElement('input');
        nameInput.className = 'shop-inv-create-input';
        nameInput.type = 'text';
        nameInput.placeholder = 'Item name...';

        // Entity type dropdown.
        var typeSelect = document.createElement('select');
        typeSelect.className = 'shop-inv-type-select';
        if (state.entityTypes.length === 0) {
          var defOpt = document.createElement('option');
          defOpt.value = '0';
          defOpt.textContent = 'Default';
          typeSelect.appendChild(defOpt);
        } else {
          for (var t = 0; t < state.entityTypes.length; t++) {
            var et = state.entityTypes[t];
            var opt = document.createElement('option');
            opt.value = et.id;
            opt.textContent = et.name;
            typeSelect.appendChild(opt);
          }
        }

        var createBtn = document.createElement('button');
        createBtn.className = 'shop-inv-action-btn green';
        createBtn.textContent = 'Create & Add';
        createBtn.onclick = function () {
          var name = nameInput.value.trim();
          if (!name) return;
          var typeId = parseInt(typeSelect.value, 10) || 0;
          createBtn.disabled = true;
          createBtn.textContent = 'Creating...';
          quickCreateItem(name, typeId, function () {
            createBtn.disabled = false;
            createBtn.textContent = 'Create & Add';
          });
        };

        nameInput.onkeydown = function (e) {
          if (e.key === 'Enter') { e.preventDefault(); createBtn.click(); }
        };

        createRow.appendChild(nameInput);
        createRow.appendChild(typeSelect);
        createRow.appendChild(createBtn);
        createSection.appendChild(createRow);
        panel.appendChild(createSection);
      }

      // Auto-focus the search input.
      setTimeout(function () { searchInput.focus(); }, 50);

      return panel;
    }

    function renderSearchResults(container) {
      container.innerHTML = '';
      if (state.searchResults.length === 0 && state.searchQuery.length >= 2) {
        container.innerHTML = '<div class="shop-inv-empty">No items found</div>';
        return;
      }
      for (var i = 0; i < state.searchResults.length; i++) {
        var result = state.searchResults[i];
        // Skip items already in inventory.
        var alreadyAdded = state.items.some(function (item) { return item.targetEntityID === result.id; });
        if (alreadyAdded) continue;

        var row = document.createElement('div');
        row.className = 'shop-inv-search-item';
        row.dataset.entityId = result.id;

        var icon = document.createElement('span');
        icon.style.color = result.type_color || '#6b7280';
        icon.innerHTML = '<i class="fas ' + Chronicle.escapeHtml(result.type_icon || 'fa-box') + '"></i>';
        row.appendChild(icon);

        var name = document.createElement('span');
        name.textContent = result.name;
        row.appendChild(name);

        if (result.type_name) {
          var badge = document.createElement('span');
          badge.style.fontSize = '0.625rem';
          badge.style.color = '#9ca3af';
          badge.style.marginLeft = '0.25rem';
          badge.textContent = '(' + result.type_name + ')';
          row.appendChild(badge);
        }

        (function (entityId) {
          row.onclick = function () { addItem(entityId); };
        })(result.id);

        container.appendChild(row);
      }
    }

    // --- API Calls ---

    function loadInventory() {
      Chronicle.apiFetch(relationsEndpoint, { method: 'GET' })
        .then(function (res) {
          if (!res.ok) throw new Error('Failed to load inventory: ' + res.status);
          return res.json();
        })
        .then(function (data) {
          // Filter to only "sells" relations.
          state.items = (data || []).filter(function (r) {
            return r.relationType === 'sells';
          }).map(function (r) {
            // Parse metadata if it's a string.
            if (typeof r.metadata === 'string') {
              try { r.metadata = JSON.parse(r.metadata); } catch (e) { r.metadata = {}; }
            }
            r.metadata = r.metadata || {};
            return r;
          });
          state.loading = false;
          render();
        })
        .catch(function (err) {
          console.error('Shop inventory: failed to load', err);
          state.loading = false;
          render();
        });
    }

    // Add an existing entity to the shop as an inventory item.
    function addItem(entityId) {
      return Chronicle.apiFetch(relationsEndpoint, {
        method: 'POST',
        body: {
          targetEntityId: entityId,
          relationType: 'sells',
          reverseRelationType: 'sold by',
          metadata: { price: 0, quantity: null, in_stock: true },
        },
      })
        .then(function (res) {
          if (!res.ok) throw new Error('Failed to add item: ' + res.status);
          state.addMode = false;
          state.searchQuery = '';
          state.searchResults = [];
          loadInventory();
        })
        .catch(function (err) {
          console.error('Shop inventory: failed to add item', err);
        });
    }

    function removeItem(relationId) {
      Chronicle.apiFetch(relationsEndpoint + '/' + relationId, {
        method: 'DELETE',
      })
        .then(function (res) {
          if (!res.ok) throw new Error('Failed to remove: ' + res.status);
          loadInventory();
        })
        .catch(function (err) {
          console.error('Shop inventory: failed to remove item', err);
        });
    }

    function updateMetadata(relationId, meta) {
      Chronicle.apiFetch(relationsEndpoint + '/' + relationId + '/metadata', {
        method: 'PUT',
        body: { metadata: meta },
      })
        .catch(function (err) {
          console.error('Shop inventory: failed to update metadata', err);
        });
    }

    function searchItems(query) {
      Chronicle.apiFetch(entitySearchEndpoint + '?q=' + encodeURIComponent(query), { method: 'GET' })
        .then(function (res) {
          if (!res.ok) throw new Error('Search failed: ' + res.status);
          return res.json();
        })
        .then(function (data) {
          state.searchResults = data.results || data || [];
          var resultsDiv = el.querySelector('.shop-inv-search-results');
          if (resultsDiv) renderSearchResults(resultsDiv);
        })
        .catch(function (err) {
          console.error('Shop inventory: search failed', err);
        });
    }

    // Create a new entity page and add it to the shop.
    function quickCreateItem(name, entityTypeId, onDone) {
      Chronicle.apiFetch(quickCreateEndpoint, {
        method: 'POST',
        body: { name: name, entity_type_id: entityTypeId },
      })
        .then(function (res) {
          if (!res.ok) {
            return res.json().catch(function () { return {}; }).then(function (body) {
              throw new Error(body.message || 'Create failed: ' + res.status);
            });
          }
          return res.json();
        })
        .then(function (data) {
          // Add the newly created entity to the shop.
          return addItem(data.id);
        })
        .then(function () {
          if (onDone) onDone();
        })
        .catch(function (err) {
          console.error('Shop inventory: failed to create item', err);
          Chronicle.notify('Failed to create item: ' + (err.message || 'Unknown error'), 'error');
          if (onDone) onDone();
        });
    }

    // Load entity types for the "Create entity page" dropdown.
    function loadEntityTypes() {
      // Use the search endpoint with empty query to discover available types.
      // The response includes type info per entity. We extract unique types.
      // Alternatively, fetch from a dedicated types endpoint if available.
      if (!quickCreateEndpoint) return;

      // Derive types endpoint from quick-create endpoint path.
      // quick-create: /campaigns/:id/entities/quick-create
      // types: /campaigns/:id/entities/types (if it exists), otherwise skip
      var typesUrl = quickCreateEndpoint.replace('/quick-create', '/types');
      Chronicle.apiFetch(typesUrl, { method: 'GET' })
        .then(function (res) {
          if (!res.ok) return;
          return res.json();
        })
        .then(function (data) {
          if (data && Array.isArray(data)) {
            state.entityTypes = data;
          }
        })
        .catch(function () {
          // Silently fail — dropdown will show "Default" fallback.
        });
    }

    // Initial load.
    loadInventory();
    loadEntityTypes();
  },

  destroy: function (el) {
    if (el._shopState && el._shopState.searchTimer) {
      clearTimeout(el._shopState.searchTimer);
    }
    el.innerHTML = '';
    delete el._shopState;
  }
});
