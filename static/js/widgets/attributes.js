/**
 * attributes.js -- Chronicle Attributes Widget
 *
 * Displays entity custom fields (attributes) with inline editing support.
 * Shows field values in a read-only card by default; clicking "Edit" reveals
 * editable inputs that match the field type definitions (text, number, select,
 * textarea, checkbox, url). Auto-mounted by boot.js on elements with
 * data-widget="attributes".
 *
 * Config (from data-* attributes):
 *   data-endpoint   - Fields API endpoint (GET/PUT),
 *                     e.g. /campaigns/:id/entities/:eid/fields
 *   data-editable   - "true" if user can modify fields (Scribe+)
 *   data-csrf-token - CSRF token for mutating requests
 */
Chronicle.register('attributes', {
  init: function (el, config) {
    var state = {
      fields: [],            // Effective (merged) field definitions
      typeFields: [],        // Original type-level fields (for override reference)
      fieldsData: {},        // Current field values
      fieldOverrides: null,  // Per-entity overrides (added, hidden, modified)
      isEditing: false,      // Whether the edit form is shown
      isCustomizing: false,  // Whether the field override panel is shown
      isSaving: false,
      error: null
    };

    el._attributesState = state;

    // Derive the overrides endpoint from the fields endpoint.
    var overridesEndpoint = config.endpoint.replace(/\/fields$/, '/field-overrides');

    // Load field definitions and current values.
    var headers = { 'Accept': 'application/json' };

    fetch(config.endpoint, { headers: headers, credentials: 'same-origin' })
      .then(function (r) {
        if (!r.ok) throw new Error('Failed to load fields');
        return r.json();
      })
      .then(function (data) {
        state.fields = data.fields || [];
        state.typeFields = data.type_fields || data.fields || [];
        state.fieldsData = data.fields_data || {};
        state.fieldOverrides = data.field_overrides || null;
        render();
      })
      .catch(function (err) {
        console.error('[attributes] Failed to load:', err);
        state.error = 'Failed to load attributes.';
        render();
      });

    // --- Render ---

    function render() {
      el.innerHTML = '';

      // Nothing to show if no fields defined.
      if (state.fields.length === 0 && !state.error) {
        return;
      }

      var card = document.createElement('div');
      card.className = 'card p-4';

      // Header with title and edit toggle.
      var header = document.createElement('div');
      header.className = 'flex items-center justify-between mb-3';

      var title = document.createElement('h3');
      title.className = 'text-xs font-semibold uppercase tracking-wider';
      title.style.color = 'var(--color-text-secondary)';
      title.textContent = 'Attributes';
      header.appendChild(title);

      if (config.editable) {
        var btnGroup = document.createElement('div');
        btnGroup.className = 'flex items-center gap-1.5';

        // Gear icon for field customization (per-entity overrides).
        var gearBtn = document.createElement('button');
        gearBtn.type = 'button';
        gearBtn.className = 'chronicle-editor__edit-btn';
        gearBtn.title = 'Customize fields for this page';
        gearBtn.innerHTML = '<i class="fa-solid fa-gear" style="font-size:11px"></i>';
        gearBtn.addEventListener('click', function () {
          state.isCustomizing = !state.isCustomizing;
          state.isEditing = false;
          render();
        });
        btnGroup.appendChild(gearBtn);

        var editBtn = document.createElement('button');
        editBtn.type = 'button';

        if (state.isEditing) {
          editBtn.className = 'chronicle-editor__edit-btn chronicle-editor__edit-btn--done';
          editBtn.innerHTML = '<i class="fa-solid fa-check" style="font-size:11px"></i> Done';
          editBtn.addEventListener('click', function () {
            saveFields();
          });
        } else {
          editBtn.className = 'chronicle-editor__edit-btn';
          editBtn.innerHTML = '<i class="fa-solid fa-pen" style="font-size:11px"></i> Edit';
          editBtn.addEventListener('click', function () {
            state.isEditing = true;
            state.isCustomizing = false;
            render();
          });
        }
        btnGroup.appendChild(editBtn);
        header.appendChild(btnGroup);
      }

      card.appendChild(header);

      // Error display.
      if (state.error) {
        var errorEl = document.createElement('div');
        errorEl.className = 'text-sm text-red-500 mb-2';
        errorEl.textContent = state.error;
        card.appendChild(errorEl);
      }

      // Fields content.
      var content = document.createElement('div');
      content.className = 'space-y-3';

      if (state.isCustomizing) {
        renderCustomizePanel(content);
      } else if (state.isEditing) {
        renderEditForm(content);
      } else {
        renderReadOnly(content);
      }

      card.appendChild(content);
      el.appendChild(card);
    }

    // --- Read-only view ---

    function renderReadOnly(container) {
      var hasValues = false;

      state.fields.forEach(function (field) {
        var val = state.fieldsData[field.key];
        if (val === undefined || val === null || val === '') return;

        hasValues = true;
        var row = document.createElement('div');

        var label = document.createElement('dt');
        label.className = 'text-xs font-medium uppercase tracking-wider';
        label.style.color = 'var(--color-text-secondary)';
        label.textContent = field.label;
        row.appendChild(label);

        var value = document.createElement('dd');
        value.className = 'text-sm mt-0.5';
        value.style.color = 'var(--color-text-primary)';

        // Format value based on field type.
        if (field.type === 'checkbox') {
          value.textContent = val === true || val === 'true' || val === 'on' ? 'Yes' : 'No';
        } else if (field.type === 'url' && val) {
          var link = document.createElement('a');
          link.href = String(val);
          link.textContent = String(val);
          link.className = 'text-accent hover:underline';
          link.target = '_blank';
          link.rel = 'noopener noreferrer';
          value.appendChild(link);
        } else {
          value.textContent = String(val);
        }

        row.appendChild(value);
        container.appendChild(row);
      });

      if (!hasValues) {
        var empty = document.createElement('div');
        empty.className = 'text-sm';
        empty.style.color = 'var(--color-text-muted)';
        empty.textContent = config.editable
          ? 'No attributes set. Click Edit to add values.'
          : 'No attributes set.';
        container.appendChild(empty);
      }
    }

    // --- Edit form ---

    function renderEditForm(container) {
      // Group fields by section.
      var sections = {};
      var sectionOrder = [];

      state.fields.forEach(function (field) {
        var sec = field.section || 'General';
        if (!sections[sec]) {
          sections[sec] = [];
          sectionOrder.push(sec);
        }
        sections[sec].push(field);
      });

      sectionOrder.forEach(function (sectionName) {
        // Section header (only if more than one section).
        if (sectionOrder.length > 1) {
          var secHeader = document.createElement('div');
          secHeader.className = 'text-xs font-semibold uppercase tracking-wider pt-2 pb-1';
          secHeader.style.color = 'var(--color-text-muted)';
          secHeader.textContent = sectionName;
          container.appendChild(secHeader);
        }

        sections[sectionName].forEach(function (field) {
          var row = document.createElement('div');

          var label = document.createElement('label');
          label.className = 'block text-xs font-medium mb-1';
          label.style.color = 'var(--color-text-secondary)';
          label.textContent = field.label;
          label.htmlFor = 'attr-' + field.key;
          row.appendChild(label);

          var currentVal = state.fieldsData[field.key];
          if (currentVal === undefined || currentVal === null) currentVal = '';

          var input;

          switch (field.type) {
            case 'textarea':
              input = document.createElement('textarea');
              input.className = 'input';
              input.rows = 3;
              input.value = String(currentVal);
              break;

            case 'select':
              input = document.createElement('select');
              input.className = 'input';
              var emptyOpt = document.createElement('option');
              emptyOpt.value = '';
              emptyOpt.textContent = 'Select...';
              input.appendChild(emptyOpt);
              (field.options || []).forEach(function (opt) {
                var option = document.createElement('option');
                option.value = opt;
                option.textContent = opt;
                if (String(currentVal) === opt) option.selected = true;
                input.appendChild(option);
              });
              break;

            case 'checkbox':
              input = document.createElement('input');
              input.type = 'checkbox';
              input.className = 'h-4 w-4 rounded border-edge text-accent focus:ring-accent';
              input.checked = currentVal === true || currentVal === 'true' || currentVal === 'on';
              break;

            default: // text, number, url
              input = document.createElement('input');
              input.type = field.type === 'number' ? 'number' : (field.type === 'url' ? 'url' : 'text');
              input.className = 'input';
              input.value = String(currentVal);
              break;
          }

          input.id = 'attr-' + field.key;
          input.setAttribute('data-field-key', field.key);
          row.appendChild(input);

          container.appendChild(row);
        });
      });
    }

    // --- Customize panel (per-entity field overrides) ---

    function renderCustomizePanel(container) {
      var overrides = state.fieldOverrides || { added: [], hidden: [], modified: {} };

      // Header.
      var info = document.createElement('div');
      info.className = 'text-xs mb-3';
      info.style.color = 'var(--color-text-muted)';
      info.textContent = 'Toggle fields on/off or add custom fields for this page only.';
      container.appendChild(info);

      // Toggle list for type-level fields.
      var toggleLabel = document.createElement('div');
      toggleLabel.className = 'text-[10px] font-semibold uppercase tracking-wider mb-1.5';
      toggleLabel.style.color = 'var(--color-text-muted)';
      toggleLabel.textContent = 'Category Fields';
      container.appendChild(toggleLabel);

      var hiddenSet = {};
      (overrides.hidden || []).forEach(function (k) { hiddenSet[k] = true; });

      state.typeFields.forEach(function (field) {
        var row = document.createElement('div');
        row.className = 'flex items-center justify-between py-1';

        var lbl = document.createElement('span');
        lbl.className = 'text-sm';
        lbl.style.color = hiddenSet[field.key] ? 'var(--color-text-muted)' : 'var(--color-text-primary)';
        lbl.textContent = field.label;
        if (hiddenSet[field.key]) {
          lbl.style.textDecoration = 'line-through';
        }
        row.appendChild(lbl);

        var toggle = document.createElement('button');
        toggle.type = 'button';
        toggle.className = 'text-xs px-2 py-0.5 rounded border';
        if (hiddenSet[field.key]) {
          toggle.textContent = 'Hidden';
          toggle.style.color = 'var(--color-text-muted)';
          toggle.style.borderColor = 'var(--color-border)';
        } else {
          toggle.textContent = 'Visible';
          toggle.style.color = 'var(--color-accent)';
          toggle.style.borderColor = 'var(--color-accent)';
        }
        toggle.addEventListener('click', function () {
          if (hiddenSet[field.key]) {
            delete hiddenSet[field.key];
          } else {
            hiddenSet[field.key] = true;
          }
          overrides.hidden = Object.keys(hiddenSet);
          render();
        });
        row.appendChild(toggle);
        container.appendChild(row);
      });

      // Added fields section.
      var addedLabel = document.createElement('div');
      addedLabel.className = 'text-[10px] font-semibold uppercase tracking-wider mb-1.5 mt-4';
      addedLabel.style.color = 'var(--color-text-muted)';
      addedLabel.textContent = 'Custom Fields (this page only)';
      container.appendChild(addedLabel);

      var addedFields = overrides.added || [];
      addedFields.forEach(function (f, idx) {
        var row = document.createElement('div');
        row.className = 'flex items-center gap-2 mb-1';

        var nameInput = document.createElement('input');
        nameInput.type = 'text';
        nameInput.className = 'input text-sm flex-1';
        nameInput.placeholder = 'Field name';
        nameInput.value = f.label || '';
        nameInput.addEventListener('input', function () {
          f.label = nameInput.value;
          f.key = nameInput.value.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '') || 'custom_field';
        });
        row.appendChild(nameInput);

        var typeSelect = document.createElement('select');
        typeSelect.className = 'input text-sm w-24';
        ['text', 'number', 'textarea', 'url', 'checkbox', 'select'].forEach(function (t) {
          var opt = document.createElement('option');
          opt.value = t;
          opt.textContent = t.charAt(0).toUpperCase() + t.slice(1);
          if (f.type === t) opt.selected = true;
          typeSelect.appendChild(opt);
        });
        typeSelect.addEventListener('change', function () { f.type = typeSelect.value; });
        row.appendChild(typeSelect);

        var delBtn = document.createElement('button');
        delBtn.type = 'button';
        delBtn.className = 'text-red-400 hover:text-red-600 text-xs p-1';
        delBtn.innerHTML = '<i class="fa-solid fa-xmark"></i>';
        delBtn.addEventListener('click', function () {
          addedFields.splice(idx, 1);
          overrides.added = addedFields;
          render();
        });
        row.appendChild(delBtn);

        container.appendChild(row);
      });

      // Add field button.
      var addBtn = document.createElement('button');
      addBtn.type = 'button';
      addBtn.className = 'text-xs text-accent hover:text-accent-hover mt-1';
      addBtn.innerHTML = '<i class="fa-solid fa-plus mr-1"></i> Add Field';
      addBtn.addEventListener('click', function () {
        addedFields.push({ key: '', label: '', type: 'text', section: 'Custom' });
        overrides.added = addedFields;
        render();
      });
      container.appendChild(addBtn);

      // Save button.
      var saveBtn = document.createElement('button');
      saveBtn.type = 'button';
      saveBtn.className = 'chronicle-editor__edit-btn chronicle-editor__edit-btn--done mt-4 w-full';
      saveBtn.innerHTML = '<i class="fa-solid fa-check" style="font-size:11px"></i> Save Customizations';
      saveBtn.addEventListener('click', function () {
        saveOverrides(overrides);
      });
      container.appendChild(saveBtn);
    }

    function saveOverrides(overrides) {
      if (state.isSaving) return;
      state.isSaving = true;

      var reqHeaders = {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      };
      if (config.csrfToken) {
        reqHeaders['X-CSRF-Token'] = config.csrfToken;
      }

      fetch(overridesEndpoint, {
        method: 'PUT',
        headers: reqHeaders,
        credentials: 'same-origin',
        body: JSON.stringify(overrides)
      })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to save overrides');
          state.fieldOverrides = overrides;
          state.isCustomizing = false;
          state.isSaving = false;
          // Reload to get merged fields.
          return fetch(config.endpoint, { headers: { 'Accept': 'application/json' }, credentials: 'same-origin' });
        })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to reload fields');
          return r.json();
        })
        .then(function (data) {
          state.fields = data.fields || [];
          state.fieldsData = data.fields_data || {};
          state.fieldOverrides = data.field_overrides || null;
          render();
        })
        .catch(function (err) {
          console.error('[attributes] Save overrides failed:', err);
          state.error = 'Failed to save customizations.';
          state.isSaving = false;
          render();
        });
    }

    // --- Save ---

    function saveFields() {
      if (state.isSaving) return;
      state.isSaving = true;
      state.error = null;

      // Collect values from form inputs.
      var newData = {};
      state.fields.forEach(function (field) {
        var input = el.querySelector('[data-field-key="' + field.key + '"]');
        if (!input) return;

        if (field.type === 'checkbox') {
          newData[field.key] = input.checked;
        } else {
          newData[field.key] = input.value;
        }
      });

      var reqHeaders = {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      };
      if (config.csrfToken) {
        reqHeaders['X-CSRF-Token'] = config.csrfToken;
      }

      fetch(config.endpoint, {
        method: 'PUT',
        headers: reqHeaders,
        credentials: 'same-origin',
        body: JSON.stringify({ fields_data: newData })
      })
        .then(function (r) {
          if (!r.ok) throw new Error('Failed to save fields');
          state.fieldsData = newData;
          state.isEditing = false;
          state.isSaving = false;
          render();
        })
        .catch(function (err) {
          console.error('[attributes] Save failed:', err);
          state.error = 'Failed to save. Please try again.';
          state.isSaving = false;
          render();
        });
    }
  },

  destroy: function (el) {
    el.innerHTML = '';
    delete el._attributesState;
  }
});
