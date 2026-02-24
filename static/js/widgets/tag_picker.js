/**
 * tag_picker.js -- Chronicle Tag Picker Widget
 *
 * Displays entity tags as colored chips with a dropdown to add/remove/create tags.
 * Auto-mounted by boot.js on elements with data-widget="tag-picker".
 *
 * Config (from data-* attributes):
 *   data-tags-endpoint  - Tag list endpoint (GET), e.g. /campaigns/:id/tags
 *   data-entity-tags-endpoint - Entity tags endpoint (GET/PUT), e.g. /campaigns/:id/entities/:eid/tags
 *   data-editable        - "true" if user can modify tags (Scribe+)
 *   data-csrf-token      - CSRF token for mutating requests
 */
Chronicle.register('tag-picker', {
  init: function (el, config) {
    var state = {
      allTags: [],
      entityTags: [],
      isOpen: false,
      search: '',
      creating: false
    };

    // Inject scoped styles once.
    if (!document.getElementById('tag-picker-styles')) {
      var style = document.createElement('style');
      style.id = 'tag-picker-styles';
      style.textContent = [
        '.tp-chip { display: inline-flex; align-items: center; gap: 4px; padding: 2px 8px; border-radius: 9999px; font-size: 12px; font-weight: 500; line-height: 20px; white-space: nowrap; }',
        '.tp-chip-remove { cursor: pointer; opacity: 0.6; margin-left: 2px; font-size: 10px; }',
        '.tp-chip-remove:hover { opacity: 1; }',
        '.tp-wrap { position: relative; display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }',
        '.tp-add-btn { display: inline-flex; align-items: center; gap: 4px; padding: 2px 10px; border-radius: 9999px; font-size: 12px; color: #6b7280; border: 1px dashed #d1d5db; cursor: pointer; background: none; white-space: nowrap; }',
        '.tp-add-btn:hover { color: #374151; border-color: #9ca3af; }',
        '.tp-dropdown { position: absolute; top: 100%; left: 0; z-index: 50; margin-top: 4px; width: 240px; background: white; border: 1px solid #e5e7eb; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); padding: 4px; }',
        '.dark .tp-dropdown { background: #1f2937; border-color: #374151; }',
        '.tp-search { width: 100%; padding: 6px 8px; font-size: 13px; border: 1px solid #e5e7eb; border-radius: 6px; outline: none; background: transparent; color: inherit; }',
        '.dark .tp-search { border-color: #4b5563; color: #e5e7eb; }',
        '.tp-search:focus { border-color: #6366f1; box-shadow: 0 0 0 2px rgba(99,102,241,0.15); }',
        '.tp-list { max-height: 180px; overflow-y: auto; margin-top: 4px; }',
        '.tp-option { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 6px; cursor: pointer; font-size: 13px; color: #374151; }',
        '.dark .tp-option { color: #d1d5db; }',
        '.tp-option:hover { background: #f3f4f6; }',
        '.dark .tp-option:hover { background: #374151; }',
        '.tp-option-check { width: 14px; text-align: center; font-size: 11px; color: #6366f1; }',
        '.tp-option-dot { width: 10px; height: 10px; border-radius: 50%; flex-shrink: 0; }',
        '.tp-create { display: flex; align-items: center; gap: 6px; padding: 6px 8px; border-radius: 6px; cursor: pointer; font-size: 13px; color: #6366f1; border-top: 1px solid #e5e7eb; margin-top: 4px; padding-top: 8px; }',
        '.dark .tp-create { color: #818cf8; border-top-color: #374151; }',
        '.tp-create:hover { background: #f3f4f6; }',
        '.dark .tp-create:hover { background: #374151; }',
        '.tp-empty { padding: 12px 8px; font-size: 13px; color: #9ca3af; text-align: center; }'
      ].join('\n');
      document.head.appendChild(style);
    }

    el._tagPickerState = state;

    // Fetch both tag lists in parallel.
    var headers = { 'Accept': 'application/json' };
    if (config.csrfToken) {
      headers['X-CSRF-Token'] = config.csrfToken;
    }

    Promise.all([
      fetch(config.tagsEndpoint, { headers: headers }).then(function (r) { return r.json(); }),
      fetch(config.entityTagsEndpoint, { headers: headers }).then(function (r) { return r.json(); })
    ]).then(function (results) {
      state.allTags = results[0] || [];
      state.entityTags = results[1] || [];
      render();
    }).catch(function (err) {
      console.error('[tag-picker] Failed to load tags:', err);
    });

    function render() {
      el.innerHTML = '';

      var wrap = document.createElement('div');
      wrap.className = 'tp-wrap';

      // Render current tags as chips.
      state.entityTags.forEach(function (tag) {
        var chip = document.createElement('span');
        chip.className = 'tp-chip';
        chip.style.backgroundColor = tag.color + '22';
        chip.style.color = tag.color;
        chip.textContent = tag.name;

        if (config.editable) {
          var remove = document.createElement('span');
          remove.className = 'tp-chip-remove';
          remove.innerHTML = '&#10005;';
          remove.title = 'Remove tag';
          remove.addEventListener('click', function (e) {
            e.stopPropagation();
            removeTag(tag.id);
          });
          chip.appendChild(remove);
        }

        wrap.appendChild(chip);
      });

      // Add button (editable only).
      if (config.editable) {
        var addBtn = document.createElement('button');
        addBtn.className = 'tp-add-btn';
        addBtn.type = 'button';
        addBtn.innerHTML = '<i class="fa-solid fa-plus" style="font-size:10px"></i> Tag';
        addBtn.addEventListener('click', function (e) {
          e.stopPropagation();
          state.isOpen = !state.isOpen;
          state.search = '';
          render();
          if (state.isOpen) {
            var input = el.querySelector('.tp-search');
            if (input) input.focus();
          }
        });
        wrap.appendChild(addBtn);
      }

      el.appendChild(wrap);

      // Dropdown.
      if (state.isOpen && config.editable) {
        var dropdown = document.createElement('div');
        dropdown.className = 'tp-dropdown';

        var searchInput = document.createElement('input');
        searchInput.className = 'tp-search';
        searchInput.type = 'text';
        searchInput.placeholder = 'Search or create tag...';
        searchInput.value = state.search;
        searchInput.addEventListener('input', function () {
          state.search = searchInput.value;
          renderOptions();
        });
        searchInput.addEventListener('keydown', function (e) {
          if (e.key === 'Escape') {
            state.isOpen = false;
            render();
          } else if (e.key === 'Enter' && state.search.trim()) {
            e.preventDefault();
            // If search matches an existing tag, toggle it. Otherwise create.
            var match = filteredTags().find(function (t) {
              return t.name.toLowerCase() === state.search.trim().toLowerCase();
            });
            if (match) {
              toggleTag(match.id);
            } else {
              createTag(state.search.trim());
            }
          }
        });
        dropdown.appendChild(searchInput);

        var listEl = document.createElement('div');
        listEl.className = 'tp-list';
        dropdown.appendChild(listEl);

        wrap.appendChild(dropdown);

        renderOptions();

        function renderOptions() {
          listEl.innerHTML = '';
          var filtered = filteredTags();

          if (filtered.length === 0 && !state.search.trim()) {
            var empty = document.createElement('div');
            empty.className = 'tp-empty';
            empty.textContent = 'No tags yet';
            listEl.appendChild(empty);
          }

          filtered.forEach(function (tag) {
            var isSelected = state.entityTags.some(function (et) { return et.id === tag.id; });
            var option = document.createElement('div');
            option.className = 'tp-option';
            option.addEventListener('click', function () {
              toggleTag(tag.id);
            });

            var check = document.createElement('span');
            check.className = 'tp-option-check';
            check.innerHTML = isSelected ? '&#10003;' : '';
            option.appendChild(check);

            var dot = document.createElement('span');
            dot.className = 'tp-option-dot';
            dot.style.backgroundColor = tag.color;
            option.appendChild(dot);

            var label = document.createElement('span');
            label.textContent = tag.name;
            option.appendChild(label);

            listEl.appendChild(option);
          });

          // "Create" option if search doesn't match any existing tag.
          var searchTerm = state.search.trim();
          if (searchTerm) {
            var exists = state.allTags.some(function (t) {
              return t.name.toLowerCase() === searchTerm.toLowerCase();
            });
            if (!exists) {
              var createOption = document.createElement('div');
              createOption.className = 'tp-create';
              createOption.innerHTML = '<i class="fa-solid fa-plus" style="font-size:11px"></i> Create "' + escapeHtml(searchTerm) + '"';
              createOption.addEventListener('click', function () {
                createTag(searchTerm);
              });
              listEl.appendChild(createOption);
            }
          }
        }

        // Close dropdown when clicking outside.
        setTimeout(function () {
          document.addEventListener('click', closeHandler);
        }, 0);
      }
    }

    function closeHandler(e) {
      if (!el.contains(e.target)) {
        state.isOpen = false;
        document.removeEventListener('click', closeHandler);
        el._tagPickerCloseHandler = null;
        render();
      }
    }

    // Store reference so destroy() can remove it if the widget is
    // torn down while the dropdown is still open.
    el._tagPickerCloseHandler = closeHandler;

    function filteredTags() {
      var term = state.search.toLowerCase();
      if (!term) return state.allTags;
      return state.allTags.filter(function (t) {
        return t.name.toLowerCase().indexOf(term) !== -1;
      });
    }

    function toggleTag(tagId) {
      var isSelected = state.entityTags.some(function (t) { return t.id === tagId; });
      var newIds;
      if (isSelected) {
        newIds = state.entityTags.filter(function (t) { return t.id !== tagId; }).map(function (t) { return t.id; });
      } else {
        newIds = state.entityTags.map(function (t) { return t.id; }).concat([tagId]);
      }
      saveEntityTags(newIds);
    }

    function removeTag(tagId) {
      var newIds = state.entityTags.filter(function (t) { return t.id !== tagId; }).map(function (t) { return t.id; });
      saveEntityTags(newIds);
    }

    function saveEntityTags(tagIds) {
      var reqHeaders = {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      };
      if (config.csrfToken) {
        reqHeaders['X-CSRF-Token'] = config.csrfToken;
      }

      fetch(config.entityTagsEndpoint, {
        method: 'PUT',
        headers: reqHeaders,
        body: JSON.stringify({ tagIds: tagIds })
      })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to update tags');
          return r.json();
        })
        .then(function (updatedTags) {
          state.entityTags = updatedTags || [];
          render();
        })
        .catch(function (err) {
          console.error('[tag-picker] Failed to save tags:', err);
        });
    }

    function createTag(name) {
      if (state.creating) return;
      state.creating = true;

      // Pick a random muted color.
      var colors = ['#6366f1', '#8b5cf6', '#ec4899', '#f43f5e', '#f97316', '#eab308', '#22c55e', '#14b8a6', '#06b6d4', '#3b82f6'];
      var color = colors[Math.floor(Math.random() * colors.length)];

      var reqHeaders = {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      };
      if (config.csrfToken) {
        reqHeaders['X-CSRF-Token'] = config.csrfToken;
      }

      fetch(config.tagsEndpoint, {
        method: 'POST',
        headers: reqHeaders,
        body: JSON.stringify({ name: name, color: color })
      })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to create tag');
          return r.json();
        })
        .then(function (newTag) {
          state.allTags.push(newTag);
          state.search = '';
          state.creating = false;
          // Immediately add the new tag to the entity.
          var newIds = state.entityTags.map(function (t) { return t.id; }).concat([newTag.id]);
          saveEntityTags(newIds);
        })
        .catch(function (err) {
          console.error('[tag-picker] Failed to create tag:', err);
          state.creating = false;
        });
    }

    function escapeHtml(str) {
      var div = document.createElement('div');
      div.textContent = str;
      return div.innerHTML;
    }
  },

  destroy: function (el) {
    // Remove click-outside handler if dropdown was open during destroy.
    if (el._tagPickerCloseHandler) {
      document.removeEventListener('click', el._tagPickerCloseHandler);
      delete el._tagPickerCloseHandler;
    }
    el.innerHTML = '';
    delete el._tagPickerState;
  }
});
