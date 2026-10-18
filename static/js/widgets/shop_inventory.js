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

    // Transaction endpoints derive from the campaign URL.
    var txEndpoint = campaignUrl + '/armory/transactions';

    // Internal state.
    var state = {
      items: [],
      loading: true,
      addMode: false,
      showTransactions: false,
      transactions: [],
      txLoading: false,
      searchQuery: '',
      searchResults: [],
      searchTimer: null,
      entityTypes: [],
      // The shop page's local words, so a reader sees the coin's local name.
      words: {},
    };
    el._shopState = state;

    // --- Render ---

    function render() {
      // Remove old content.
      while (el.firstChild) el.removeChild(el.firstChild);

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

      // Transaction history section.
      if (editable) {
        var txToggle = document.createElement('button');
        txToggle.className = 'shop-inv-tx-toggle';
        txToggle.innerHTML = '<i class="fas fa-receipt"></i> ' + (state.showTransactions ? 'Hide Transactions' : 'Transactions');
        txToggle.onclick = function () {
          state.showTransactions = !state.showTransactions;
          if (state.showTransactions && state.transactions.length === 0) {
            loadTransactions();
          }
          render();
        };
        header.appendChild(txToggle);
      }

      if (state.showTransactions) {
        wrapper.appendChild(renderTransactionSection());
      }

      el.appendChild(wrapper);
    }

    function renderTransactionSection() {
      var section = document.createElement('div');
      section.className = 'shop-inv-tx-section';

      var title = document.createElement('div');
      title.className = 'shop-inv-tx-title';
      title.textContent = 'Recent Transactions';
      section.appendChild(title);

      if (state.txLoading) {
        var loading = document.createElement('div');
        loading.className = 'shop-inv-empty';
        loading.textContent = 'Loading transactions...';
        section.appendChild(loading);
      } else if (state.transactions.length === 0) {
        var empty = document.createElement('div');
        empty.className = 'shop-inv-empty';
        empty.textContent = 'No transactions recorded.';
        section.appendChild(empty);
      } else {
        var list = document.createElement('div');
        list.className = 'shop-inv-tx-list';
        for (var i = 0; i < state.transactions.length; i++) {
          list.appendChild(renderTransactionRow(state.transactions[i]));
        }
        section.appendChild(list);
      }
      return section;
    }

    function renderTransactionRow(tx) {
      var row = document.createElement('div');
      row.className = 'shop-inv-tx-row';

      var typeSpan = document.createElement('span');
      typeSpan.className = 'shop-inv-tx-type ' + (tx.transaction_type || '');
      typeSpan.textContent = tx.transaction_type || 'unknown';
      row.appendChild(typeSpan);

      var detail = document.createElement('span');
      detail.className = 'shop-inv-tx-detail';
      var parts = [];
      if (tx.item_name) parts.push(tx.item_name);
      if (tx.quantity > 1) parts.push('x' + tx.quantity);
      if (tx.buyer_name) parts.push('\u2192 ' + tx.buyer_name);
      detail.textContent = parts.join(' ');
      row.appendChild(detail);

      if (tx.price_paid) {
        var price = document.createElement('span');
        price.className = 'shop-inv-tx-price';
        price.textContent = tx.price_paid;
        row.appendChild(price);
      }

      var date = document.createElement('span');
      date.className = 'shop-inv-tx-date';
      date.textContent = formatTxDate(tx.created_at);
      row.appendChild(date);

      return row;
    }

    function formatTxDate(isoStr) {
      if (!isoStr) return '';
      try {
        var d = new Date(isoStr);
        return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
      } catch (e) {
        return '';
      }
    }

    function renderItem(item) {
      var meta = item.metadata || {};
      var isCustom = !item.targetEntityId;
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
        nameLink.href = campaignUrl + '/entities/' + item.targetEntityId;
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
        // The coin as stored; the editor sees the usual word it will save.
        priceLabel.textContent = meta.currency || 'gp';
        controls.appendChild(priceInput);
        controls.appendChild(priceLabel);
      } else {
        var priceSpan = document.createElement('span');
        priceSpan.className = 'shop-inv-price';
        var cur = meta.currency || 'gp';
        var coin = Chronicle.localName ? Chronicle.localName(state.words, cur) : { text: cur, usual: '' };
        priceSpan.textContent = (meta.price || 0) + ' ' + coin.text;
        if (coin.usual) priceSpan.title = (meta.price || 0) + ' ' + coin.usual;
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
        var alreadyAdded = state.items.some(function (item) { return item.targetEntityId === result.id; });
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
      var ids = (relationsEndpoint || '').match(/\/campaigns\/([^/]+)\/entities\/([^/]+)\//) || [];
      if (Chronicle.localWords && !editable) {
        Chronicle.localWords(ids[1], ids[2]).then(function (w) { state.words = w || {}; if (!state.loading) render(); });
      }
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
          Chronicle.notify('Could not add the item to the shop.', 'error');
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
          Chronicle.notify('Could not remove the item from the shop.', 'error');
        });
    }

    function updateMetadata(relationId, meta) {
      Chronicle.apiFetch(relationsEndpoint + '/' + relationId + '/metadata', {
        method: 'PUT',
        body: { metadata: meta },
      })
        .then(function (res) {
          if (!res.ok) throw new Error('Failed to save: ' + res.status);
        })
        .catch(function (err) {
          console.error('Shop inventory: failed to update metadata', err);
          Chronicle.notify('Could not save the change to this item.', 'error');
        });
    }

    function searchItems(query) {
      Chronicle.apiFetch(entitySearchEndpoint + '?q=' + encodeURIComponent(query), { method: 'GET' })
        .then(function (res) {
          if (!res.ok) throw new Error('Search failed: ' + res.status);
          return res.json();
        })
        .then(function (data) {
          // Search also returns maps, timelines and events; only entity pages
          // can be stocked, since a relation needs an entity on both ends.
          var results = (data && data.results) || [];
          state.searchResults = results.filter(function (r) {
            return typeof r.url === 'string' && r.url.indexOf('/entities/') !== -1;
          });
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

    // Load transaction history for this shop entity.
    function loadTransactions() {
      // Extract entity ID from the relations endpoint.
      // Format: /campaigns/:id/entities/:eid/relations
      var parts = relationsEndpoint.split('/');
      var eidIdx = parts.indexOf('entities');
      var entityId = eidIdx >= 0 ? parts[eidIdx + 1] : '';
      if (!entityId) return;

      state.txLoading = true;
      render();

      Chronicle.apiFetch(txEndpoint + '?shop=' + encodeURIComponent(entityId) + '&per_page=20', { method: 'GET' })
        .then(function (res) {
          if (!res.ok) throw new Error('Failed to load transactions');
          return res.json();
        })
        .then(function (data) {
          state.transactions = data.data || [];
          state.txLoading = false;
          render();
        })
        .catch(function (err) {
          console.error('Shop inventory: failed to load transactions', err);
          state.txLoading = false;
          render();
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
