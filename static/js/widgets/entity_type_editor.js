/**
 * entity_type_editor.js -- Inline Entity Type Editor Widget
 *
 * Renders an edit form for an entity type with name, plural name, icon picker,
 * color picker, and field management. Saves changes via PUT to the entity type
 * API endpoint.
 *
 * Config attributes:
 *   data-entity-type-id  -- Entity type ID
 *   data-endpoint        -- API endpoint for PUT (e.g., /campaigns/:id/entity-types/:etid)
 *   data-name            -- Current name
 *   data-name-plural     -- Current plural name
 *   data-icon            -- Current icon class
 *   data-color           -- Current color hex
 *   data-fields          -- JSON array of field definitions
 *   data-csrf-token      -- CSRF token
 */
(function () {
  'use strict';

  // Common Font Awesome icons for the picker.
  var ICONS = [
    'fa-user', 'fa-users', 'fa-map-pin', 'fa-building', 'fa-box', 'fa-sticky-note',
    'fa-calendar', 'fa-crown', 'fa-shield', 'fa-scroll', 'fa-book', 'fa-star',
    'fa-flag', 'fa-landmark', 'fa-globe', 'fa-mountain', 'fa-tree', 'fa-water',
    'fa-fire', 'fa-bolt', 'fa-gem', 'fa-skull', 'fa-dragon', 'fa-hat-wizard',
    'fa-dungeon', 'fa-chess-rook', 'fa-hand-fist', 'fa-wand-sparkles',
    'fa-circle', 'fa-heart', 'fa-swords', 'fa-compass', 'fa-anchor',
    'fa-feather', 'fa-paw', 'fa-horse', 'fa-ghost', 'fa-flask'
  ];

  // Valid field types for the field editor.
  var FIELD_TYPES = [
    { value: 'text', label: 'Text' },
    { value: 'number', label: 'Number' },
    { value: 'textarea', label: 'Textarea' },
    { value: 'select', label: 'Select' },
    { value: 'checkbox', label: 'Checkbox' },
    { value: 'url', label: 'URL' }
  ];

  Chronicle.register('entity-type-editor', {
    init: function (el, config) {
      var endpoint = config.endpoint;
      var csrfToken = config.csrfToken;
      var currentName = el.getAttribute('data-name') || '';
      var currentNamePlural = el.getAttribute('data-name-plural') || '';
      var currentIcon = el.getAttribute('data-icon') || 'fa-circle';
      var currentColor = el.getAttribute('data-color') || '#6b7280';
      var fields = [];

      try {
        fields = JSON.parse(el.getAttribute('data-fields') || '[]');
      } catch (e) {
        fields = [];
      }

      // Track selected icon.
      var selectedIcon = currentIcon;

      // Build the editor UI.
      el.innerHTML = '';

      // Name fields row.
      var nameRow = document.createElement('div');
      nameRow.className = 'grid grid-cols-1 md:grid-cols-2 gap-4 mb-4';

      var nameField = createInput('Name', 'et-edit-name', currentName, 'Name');
      var pluralField = createInput('Plural Name', 'et-edit-plural', currentNamePlural, 'Plural name');
      nameRow.appendChild(nameField);
      nameRow.appendChild(pluralField);
      el.appendChild(nameRow);

      // Icon + color row.
      var iconColorRow = document.createElement('div');
      iconColorRow.className = 'grid grid-cols-1 md:grid-cols-2 gap-4 mb-4';

      // Icon picker.
      var iconContainer = document.createElement('div');
      var iconLabel = document.createElement('label');
      iconLabel.className = 'block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1';
      iconLabel.textContent = 'Icon';
      iconContainer.appendChild(iconLabel);

      var iconGrid = document.createElement('div');
      iconGrid.className = 'flex flex-wrap gap-1.5 p-2 border border-gray-200 dark:border-gray-700 rounded-md max-h-28 overflow-y-auto bg-white dark:bg-gray-800';

      ICONS.forEach(function (icon) {
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'inline-flex items-center justify-center w-7 h-7 rounded border text-sm transition-colors ' +
          (icon === selectedIcon
            ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400'
            : 'border-gray-200 dark:border-gray-700 text-gray-500 dark:text-gray-400 hover:border-gray-300 dark:hover:border-gray-600');
        btn.title = icon;
        btn.innerHTML = '<i class="fa-solid ' + icon + '"></i>';
        btn.addEventListener('click', function () {
          selectedIcon = icon;
          // Update visual state.
          iconGrid.querySelectorAll('button').forEach(function (b) {
            b.className = 'inline-flex items-center justify-center w-7 h-7 rounded border text-sm transition-colors border-gray-200 dark:border-gray-700 text-gray-500 dark:text-gray-400 hover:border-gray-300 dark:hover:border-gray-600';
          });
          btn.className = 'inline-flex items-center justify-center w-7 h-7 rounded border text-sm transition-colors border-blue-500 bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400';
        });
        iconGrid.appendChild(btn);
      });

      iconContainer.appendChild(iconGrid);
      iconColorRow.appendChild(iconContainer);

      // Color picker.
      var colorContainer = document.createElement('div');
      var colorLabel = document.createElement('label');
      colorLabel.className = 'block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1';
      colorLabel.textContent = 'Color';
      colorContainer.appendChild(colorLabel);

      var colorInput = document.createElement('input');
      colorInput.type = 'color';
      colorInput.value = currentColor;
      colorInput.className = 'w-10 h-10 rounded border border-gray-200 dark:border-gray-700 cursor-pointer';
      colorContainer.appendChild(colorInput);

      iconColorRow.appendChild(colorContainer);
      el.appendChild(iconColorRow);

      // Fields management section.
      var fieldsSection = document.createElement('div');
      fieldsSection.className = 'mb-4';

      var fieldsHeader = document.createElement('div');
      fieldsHeader.className = 'flex items-center justify-between mb-2';

      var fieldsLabel = document.createElement('label');
      fieldsLabel.className = 'block text-sm font-medium text-gray-700 dark:text-gray-300';
      fieldsLabel.textContent = 'Custom Fields';
      fieldsHeader.appendChild(fieldsLabel);

      var addFieldBtn = document.createElement('button');
      addFieldBtn.type = 'button';
      addFieldBtn.className = 'text-xs text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300';
      addFieldBtn.innerHTML = '<i class="fa-solid fa-plus mr-1"></i> Add Field';
      addFieldBtn.addEventListener('click', function () {
        fields.push({ key: '', label: '', type: 'text', section: 'Basics', options: [] });
        renderFields();
      });
      fieldsHeader.appendChild(addFieldBtn);
      fieldsSection.appendChild(fieldsHeader);

      var fieldsList = document.createElement('div');
      fieldsList.className = 'space-y-2';
      fieldsSection.appendChild(fieldsList);
      el.appendChild(fieldsSection);

      function renderFields() {
        fieldsList.innerHTML = '';
        if (fields.length === 0) {
          var empty = document.createElement('p');
          empty.className = 'text-xs text-gray-400 dark:text-gray-500 italic';
          empty.textContent = 'No custom fields. Click "Add Field" to create one.';
          fieldsList.appendChild(empty);
          return;
        }

        fields.forEach(function (field, idx) {
          var row = document.createElement('div');
          row.className = 'flex items-center gap-2 p-2 bg-gray-50 dark:bg-gray-800 rounded border border-gray-200 dark:border-gray-700';

          // Label input.
          var labelInput = document.createElement('input');
          labelInput.type = 'text';
          labelInput.value = field.label || '';
          labelInput.placeholder = 'Label';
          labelInput.className = 'input text-xs flex-1 py-1';
          labelInput.addEventListener('input', function () {
            fields[idx].label = this.value;
            // Auto-generate key from label.
            fields[idx].key = this.value.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '');
          });
          row.appendChild(labelInput);

          // Type select.
          var typeSelect = document.createElement('select');
          typeSelect.className = 'input text-xs py-1 w-24';
          FIELD_TYPES.forEach(function (ft) {
            var opt = document.createElement('option');
            opt.value = ft.value;
            opt.textContent = ft.label;
            if (ft.value === field.type) opt.selected = true;
            typeSelect.appendChild(opt);
          });
          typeSelect.addEventListener('change', function () {
            fields[idx].type = this.value;
          });
          row.appendChild(typeSelect);

          // Section input.
          var sectionInput = document.createElement('input');
          sectionInput.type = 'text';
          sectionInput.value = field.section || 'Basics';
          sectionInput.placeholder = 'Section';
          sectionInput.className = 'input text-xs py-1 w-20';
          sectionInput.addEventListener('input', function () {
            fields[idx].section = this.value;
          });
          row.appendChild(sectionInput);

          // Move up button.
          if (idx > 0) {
            var upBtn = document.createElement('button');
            upBtn.type = 'button';
            upBtn.className = 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 text-xs';
            upBtn.innerHTML = '<i class="fa-solid fa-arrow-up"></i>';
            upBtn.title = 'Move up';
            upBtn.addEventListener('click', function () {
              var temp = fields[idx];
              fields[idx] = fields[idx - 1];
              fields[idx - 1] = temp;
              renderFields();
            });
            row.appendChild(upBtn);
          }

          // Move down button.
          if (idx < fields.length - 1) {
            var downBtn = document.createElement('button');
            downBtn.type = 'button';
            downBtn.className = 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 text-xs';
            downBtn.innerHTML = '<i class="fa-solid fa-arrow-down"></i>';
            downBtn.title = 'Move down';
            downBtn.addEventListener('click', function () {
              var temp = fields[idx];
              fields[idx] = fields[idx + 1];
              fields[idx + 1] = temp;
              renderFields();
            });
            row.appendChild(downBtn);
          }

          // Remove button.
          var removeBtn = document.createElement('button');
          removeBtn.type = 'button';
          removeBtn.className = 'text-red-400 hover:text-red-600 dark:hover:text-red-300 text-xs';
          removeBtn.innerHTML = '<i class="fa-solid fa-xmark"></i>';
          removeBtn.title = 'Remove field';
          removeBtn.addEventListener('click', function () {
            fields.splice(idx, 1);
            renderFields();
          });
          row.appendChild(removeBtn);

          fieldsList.appendChild(row);
        });
      }

      renderFields();

      // Save button row.
      var saveRow = document.createElement('div');
      saveRow.className = 'flex items-center gap-3';

      var saveBtn = document.createElement('button');
      saveBtn.type = 'button';
      saveBtn.className = 'btn-primary text-sm';
      saveBtn.textContent = 'Save Changes';

      var statusSpan = document.createElement('span');
      statusSpan.className = 'text-xs text-gray-500 dark:text-gray-400';

      saveBtn.addEventListener('click', function () {
        var nameInput = el.querySelector('#et-edit-name');
        var pluralInput = el.querySelector('#et-edit-plural');

        if (!nameInput.value.trim()) {
          statusSpan.textContent = 'Name is required';
          statusSpan.className = 'text-xs text-red-500';
          return;
        }

        saveBtn.disabled = true;
        statusSpan.textContent = 'Saving...';
        statusSpan.className = 'text-xs text-gray-500 dark:text-gray-400';

        var payload = {
          name: nameInput.value.trim(),
          name_plural: pluralInput.value.trim(),
          icon: selectedIcon,
          color: colorInput.value,
          fields: fields.filter(function (f) { return f.key && f.label; })
        };

        fetch(endpoint, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': csrfToken
          },
          body: JSON.stringify(payload)
        })
        .then(function (res) {
          if (!res.ok) {
            return res.json().then(function (data) {
              throw new Error(data.error || 'Failed to save');
            });
          }
          return res.json();
        })
        .then(function () {
          statusSpan.textContent = 'Saved!';
          statusSpan.className = 'text-xs text-green-600 dark:text-green-400';
          // Reload the page to reflect changes.
          setTimeout(function () {
            window.location.reload();
          }, 500);
        })
        .catch(function (err) {
          statusSpan.textContent = err.message;
          statusSpan.className = 'text-xs text-red-500';
          saveBtn.disabled = false;
        });
      });

      saveRow.appendChild(saveBtn);
      saveRow.appendChild(statusSpan);
      el.appendChild(saveRow);

      // --- Helpers ---

      function createInput(label, id, value, placeholder) {
        var wrapper = document.createElement('div');

        var lbl = document.createElement('label');
        lbl.htmlFor = id;
        lbl.className = 'block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1';
        lbl.textContent = label;
        wrapper.appendChild(lbl);

        var input = document.createElement('input');
        input.type = 'text';
        input.id = id;
        input.value = value;
        input.placeholder = placeholder;
        input.className = 'input w-full';
        input.maxLength = 100;
        wrapper.appendChild(input);

        return wrapper;
      }
    }
  });
})();
