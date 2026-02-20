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
      fields: [],       // Field definitions from entity type
      fieldsData: {},   // Current field values
      isEditing: false, // Whether the edit form is shown
      isSaving: false,
      error: null
    };

    el._attributesState = state;

    // Load field definitions and current values.
    var headers = { 'Accept': 'application/json' };

    fetch(config.endpoint, { headers: headers, credentials: 'same-origin' })
      .then(function (r) {
        if (!r.ok) throw new Error('Failed to load fields');
        return r.json();
      })
      .then(function (data) {
        state.fields = data.fields || [];
        state.fieldsData = data.fields_data || {};
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
            render();
          });
        }
        header.appendChild(editBtn);
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

      if (state.isEditing) {
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
