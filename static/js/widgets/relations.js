/**
 * relations.js -- bi-directional entity relations widget, grouped by
 * relation type. Add/remove UI is Scribe+ only. Auto-mounted by boot.js on
 * data-widget="relations".
 *
 * Config (data-*): relations-endpoint (GET/POST list/create, e.g.
 * /campaigns/:id/entities/:eid/relations), relation-types-endpoint (GET),
 * entity-search-endpoint (GET), campaign-url (base for entity links),
 * editable ("true" for Scribe+).
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
      dmOnly: false,
      isSearching: false,
      isSubmitting: false,
      error: null
    };

    el._relationsState = state;

    // Load relations and common types in parallel.
    var fetches = [
      Chronicle.apiFetch(config.relationsEndpoint).then(function (r) {
        if (!r.ok) throw new Error('Failed to load relations');
        return r.json();
      })
    ];

    // Only fetch relation types if editable (needed for the add UI).
    if (config.editable && config.relationTypesEndpoint) {
      fetches.push(
        Chronicle.apiFetch(config.relationTypesEndpoint).then(function (r) {
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
      loaded();
    }).catch(function (err) {
      console.error('[relations] Failed to load:', err);
      Chronicle.notify('Failed to load relations', 'error');
      state.error = 'Failed to load relations. Please refresh the page.';
      render();
      loaded();
    });

    // Says the first rows are drawn, for a page move that waits on them
    // (annals_pull.js).
    function loaded() {
      el.dispatchEvent(new CustomEvent('chronicle:relations-rendered', { bubbles: true }));
    }

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

      // Group relations by type. While the owner views as a player, the
      // Director-only links are left out here rather than only hidden, so
      // the groups and their counts read as a player's would.
      var preview = document.documentElement.hasAttribute('data-view-as-player');
      var groups = {};
      var groupOrder = [];
      state.relations.forEach(function (rel) {
        if (preview && rel.dmOnly) return;
        if (!groups[rel.relationType]) {
          groups[rel.relationType] = [];
          groupOrder.push(rel.relationType);
        }
        groups[rel.relationType].push(rel);
      });

      // Render relation groups.
      if (groupOrder.length > 0) {
        var card = document.createElement('div');
        card.className = 'rel-card cb-card';

        groupOrder.forEach(function (type) {
          var headerEl = document.createElement('div');
          headerEl.className = 'rel-group-header cb-group';
          // The count is drawn only by a page look that shows it (cb-count).
          headerEl.innerHTML = '<i class="fa-solid fa-link" style="font-size:10px"></i> ' + Chronicle.escapeHtml(type) +
            ' <span class="cb-count rel-group-count">' + groups[type].length + '</span>';
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
            state.dmOnly = false;
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
      item.className = 'rel-item cb-link';
      if (rel.dmOnly && config.isDm) item.setAttribute('data-dm-only', '');

      // Entity type icon.
      var icon = document.createElement('div');
      icon.className = 'rel-icon';
      icon.style.backgroundColor = rel.targetEntityColor || 'var(--color-text-secondary, #6b7280)';
      icon.innerHTML = '<i class="fa-solid ' + Chronicle.escapeHtml(rel.targetEntityIcon || 'fa-file') + '"></i>';
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

      // Show DM-only badge for DM users viewing DM-only relations.
      if (rel.dmOnly && config.isDm) {
        var dmBadge = document.createElement('span');
        dmBadge.className = 'rel-dm-badge cb-dm';
        dmBadge.innerHTML = '<i class="fa-solid fa-lock" style="font-size:8px"></i> ' + (window.Chronicle && Chronicle.gmTerm ? Chronicle.gmTerm() : 'GM');
        nameWrap.appendChild(dmBadge);
      }
      item.appendChild(nameWrap);

      // Opens the page beside this one; beside.js shows it only when it fits.
      var besideLink = document.createElement('a');
      besideLink.href = entityHref;
      besideLink.className = 'rel-beside';
      besideLink.setAttribute('data-beside-open', '');
      besideLink.setAttribute('data-beside-name', rel.targetEntityName || 'Unknown Entity');
      besideLink.title = 'Open beside this page';
      besideLink.setAttribute('aria-label', 'Open ' + (rel.targetEntityName || 'page') + ' beside this page');
      besideLink.innerHTML = '<i class="fa-solid fa-table-columns" aria-hidden="true"></i>';
      item.appendChild(besideLink);

      // Delete button (editable only).
      if (config.editable) {
        var delBtn = document.createElement('button');
        delBtn.type = 'button';
        delBtn.className = 'rel-delete';
        delBtn.title = 'Remove relation';
        delBtn.innerHTML = '<i class="fa-solid fa-xmark"></i>';
        delBtn.setAttribute('aria-label', 'Remove relation');
        delBtn.addEventListener('click', function (e) {
          e.stopPropagation();
          askRemove(item, delBtn, rel);
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
        selIcon.style.backgroundColor = state.selectedTarget.type_color || 'var(--color-text-secondary, #6b7280)';
        selIcon.innerHTML = '<i class="fa-solid ' + Chronicle.escapeHtml(state.selectedTarget.type_icon || 'fa-file') + '"></i>';
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

      // DM-only toggle (only for DMs).
      if (config.isDm) {
        var dmToggle = document.createElement('label');
        dmToggle.className = 'rel-dm-toggle';
        var dmCheckbox = document.createElement('input');
        dmCheckbox.type = 'checkbox';
        dmCheckbox.checked = state.dmOnly;
        dmCheckbox.addEventListener('change', function () {
          state.dmOnly = dmCheckbox.checked;
        });
        dmToggle.appendChild(dmCheckbox);
        var dmLabel = document.createElement('span');
        dmLabel.innerHTML = '<i class="fa-solid fa-lock" style="font-size:10px"></i> ' + (window.Chronicle && Chronicle.gmTerm ? Chronicle.gmTerm() : 'GM') + '-only (hidden from players)';
        dmToggle.appendChild(dmLabel);
        modal.appendChild(dmToggle);
      }

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
        icon.style.backgroundColor = entity.type_color || 'var(--color-text-secondary, #6b7280)';
        icon.innerHTML = '<i class="fa-solid ' + Chronicle.escapeHtml(entity.type_icon || 'fa-file') + '"></i>';
        result.appendChild(icon);

        var name = document.createElement('span');
        name.textContent = entity.name;
        result.appendChild(name);

        if (entity.type_name) {
          var typeBadge = document.createElement('span');
          typeBadge.style.cssText = 'font-size:11px; color:var(--color-text-muted, #9ca3af); margin-left:auto;';
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

      // Use the entity search API endpoint which returns JSON when Accept
      // header includes application/json. Returns { results: [...], total: N }.
      var url = config.entitySearchEndpoint + '?q=' + encodeURIComponent(query);

      Chronicle.apiFetch(url)
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
          Chronicle.notify('Failed to search entities', 'error');
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

      Chronicle.apiFetch(config.relationsEndpoint, {
        method: 'POST',
        body: {
          targetEntityId: state.selectedTarget.id,
          relationType: relationType,
          reverseRelationType: reverseRelationType,
          dmOnly: state.dmOnly || false
        }
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
          Chronicle.notify('Failed to create relation', 'error');
          state.isSubmitting = false;
          state.error = err.message || 'Failed to create relation';
          render();
        });
    }

    // askRemove swaps a row's remove button for "Remove? Yes . Keep" so the
    // question and the answer stay on the row it is about. A failed removal
    // keeps the row and offers another try in the same place.
    function askRemove(item, delBtn, rel) {
      var name = rel.targetEntityName || 'this page';
      var ask = document.createElement('span');
      ask.className = 'fx-ask';
      ask.setAttribute('role', 'group');
      ask.setAttribute('aria-label', 'Remove the relation to ' + name);

      function setAsk(label, yes, danger) {
        ask.innerHTML = '';
        ask.appendChild(document.createTextNode(label + ' '));
        var y = document.createElement('button');
        y.type = 'button';
        y.className = danger ? 'danger' : '';
        y.textContent = yes;
        ask.appendChild(y);
        ask.appendChild(document.createTextNode('\u00b7'));
        var k = document.createElement('button');
        k.type = 'button';
        k.textContent = 'Keep';
        ask.appendChild(k);
        y.addEventListener('click', function (e) {
          e.stopPropagation();
          y.disabled = true;
          k.disabled = true;
          ask.firstChild.textContent = 'Removing\u2026 ';
          deleteRelation(rel.id).then(function (ok) {
            if (ok) {
              Chronicle.notify('Removed the relation to ' + name + '.', 'success');
              // The row (and its buttons) is gone after the reload; keep focus
              // in the widget rather than letting it fall to the page.
              var add = el.querySelector('.rel-add-btn');
              if (add) add.focus();
              return;
            }
            setAsk('Could not remove it.', 'Try again', true);
          });
        });
        k.addEventListener('click', function (e) {
          e.stopPropagation();
          ask.replaceWith(delBtn);
          delBtn.focus();
        });
        y.focus();
      }

      delBtn.replaceWith(ask);
      setAsk('Remove?', 'Yes', true);
      ask.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') {
          e.stopPropagation();
          ask.replaceWith(delBtn);
          delBtn.focus();
        }
      });
    }

    // deleteRelation resolves true once the relation is gone and the list has
    // reloaded, false when the server refused.
    function deleteRelation(relationId) {
      return Chronicle.apiFetch(config.relationsEndpoint + '/' + relationId, {
        method: 'DELETE'
      })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to delete relation');
          return loadRelations().then(function () { return true; });
        })
        .catch(function (err) {
          console.error('[relations] Delete failed:', err);
          return false;
        });
    }

    function loadRelations() {
      return Chronicle.apiFetch(config.relationsEndpoint)
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
          Chronicle.notify('Failed to load relations', 'error');
        });
    }

  },

  destroy: function (el) {
    el.innerHTML = '';
    delete el._relationsState;
  }
});
