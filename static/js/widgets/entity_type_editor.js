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
 *   data-draft           -- with data-fields-only: write the fields into the
 *                           Customizer's draft (no Save button here)
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
    'fa-circle', 'fa-heart', 'fa-chess-knight', 'fa-compass', 'fa-anchor',
    'fa-feather', 'fa-paw', 'fa-horse', 'fa-ghost', 'fa-flask'
  ];

  // Valid field types for the field editor.
  var FIELD_TYPES = [
    { value: 'text', label: 'Text' },
    { value: 'number', label: 'Number' },
    { value: 'textarea', label: 'Textarea' },
    { value: 'select', label: 'Select' },
    { value: 'checkbox', label: 'Checkbox' },
    { value: 'url', label: 'URL' },
    { value: 'calendar_date', label: 'Calendar date' }
  ];
  var KEPT_TYPES = { link: 'Link to a page', rule: 'Worked out' };

  Chronicle.register('entity-type-editor', {
    destroy: function (el) {
      el.innerHTML = '';
    },
    init: function (el, config) {
      var endpoint = config.endpoint;
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

      // When data-fields-only is set, only show the field management section
      // (used on the unified config page where name/icon/color live in Nav Panel tab).
      var fieldsOnly = el.getAttribute('data-fields-only') === 'true';

      // Build the editor UI.
      el.innerHTML = '';

      var colorInput = null;

      if (!fieldsOnly) {
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

        colorInput = document.createElement('input');
        colorInput.type = 'color';
        colorInput.value = currentColor;
        colorInput.className = 'w-10 h-10 rounded border border-gray-200 dark:border-gray-700 cursor-pointer';
        colorContainer.appendChild(colorInput);

        iconColorRow.appendChild(colorContainer);
        el.appendChild(iconColorRow);
      }

      // Fields management section.
      var fieldsSection = document.createElement('div');
      fieldsSection.className = 'czf mb-4';

      var fieldsHeader = document.createElement('div');
      fieldsHeader.className = 'czf-bar flex items-center justify-between mb-2';

      var fieldsLabel = document.createElement('label');
      fieldsLabel.className = 'czf-title block text-sm font-medium text-gray-700 dark:text-gray-300';
      fieldsLabel.textContent = 'Fields';
      fieldsHeader.appendChild(fieldsLabel);

      var addFieldBtn = document.createElement('button');
      addFieldBtn.type = 'button';
      addFieldBtn.className = 'czf-add text-xs text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300';
      addFieldBtn.innerHTML = '<i class="fa-solid fa-plus word-ic mr-1"></i> Add a field';
      addFieldBtn.addEventListener('click', function () {
        fields.push({ key: '', label: '', type: 'text', section: 'Basics', options: [] });
        rerender();
      });
      fieldsHeader.appendChild(addFieldBtn);
      fieldsSection.appendChild(fieldsHeader);

      var fieldsList = document.createElement('div');
      fieldsList.className = 'czf-list space-y-2';
      fieldsSection.appendChild(fieldsList);
      el.appendChild(fieldsSection);

      // In draft mode (data-draft) the editor has no Save button: what is
      // edited goes into the Customizer's draft and saves with the rest.
      var inDraft = el.getAttribute('data-draft') === 'true' && fieldsOnly;
      var typeId = el.getAttribute('data-entity-type-id') || '';
      var savedKey = '';
      function complete() { return fields.filter(function (f) { return f.key && f.label; }); }
      // snapshot records the saved fields as the editor shows them (the
      // render fills in empty countdown lists), then lays the draft's over.
      function snapshot() {
        if (!inDraft) return;
        savedKey = JSON.stringify(complete());
        var d = Chronicle.customizeDraft ? Chronicle.customizeDraft() : null;
        var it = d && d.categories && d.categories.items && d.categories.items[typeId];
        if (it && Array.isArray(it.fields)) {
          fields = JSON.parse(JSON.stringify(it.fields));
          renderFields();
        }
      }
      function changed(step) {
        if (!inDraft || !Chronicle.customizeCategoryWrite) return;
        var now = complete();
        Chronicle.customizeCategoryWrite(typeId, { fields: JSON.stringify(now) === savedKey ? undefined : now }, step ? { step: true } : undefined);
      }
      function rerender() { renderFields(); changed(true); }

      function renderFields() {
        fieldsList.innerHTML = '';
        if (fields.length === 0) {
          var empty = document.createElement('p');
          empty.className = 'text-xs text-gray-400 dark:text-gray-500 italic';
          empty.textContent = 'No fields yet. Choose "Add a field" to make one.';
          fieldsList.appendChild(empty);
          return;
        }

        // Every column is named, so the third one is not a bare "Basics".
        var colHead = document.createElement('div');
        colHead.className = 'czf-cols';
        colHead.setAttribute('aria-hidden', 'true');
        ['Label', 'Kind', 'Section', ''].forEach(function (t) {
          var c = document.createElement('span');
          c.textContent = t;
          colHead.appendChild(c);
        });
        fieldsList.appendChild(colHead);

        fields.forEach(function (field, idx) {
          var row = document.createElement('div');
          row.className = 'czf-row flex items-center gap-2 p-2 bg-gray-50 dark:bg-gray-800 rounded border border-gray-200 dark:border-gray-700';
          var acts = document.createElement('span');
          acts.className = 'czf-acts flex items-center gap-2';

          // Label input.
          var labelInput = document.createElement('input');
          labelInput.type = 'text';
          labelInput.value = field.label || '';
          labelInput.placeholder = 'Label';
          labelInput.className = 'input text-xs flex-1 py-1';
          labelInput.setAttribute('aria-label', 'Label');
          labelInput.addEventListener('input', function () {
            fields[idx].label = this.value;
            // Auto-generate key from label.
            fields[idx].key = this.value.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '');
            changed();
          });
          row.appendChild(labelInput);

          // Type select.
          var typeSelect = document.createElement('select');
          typeSelect.className = 'input text-xs py-1 w-32';
          typeSelect.setAttribute('aria-label', 'Kind');
          // A link or worked-out field carries its category or rule, which
          // this editor can't choose, so its type is shown and not changed.
          var kept = KEPT_TYPES[field.type];
          (kept ? [{ value: field.type, label: kept }] : FIELD_TYPES).forEach(function (ft) {
            var opt = document.createElement('option');
            opt.value = ft.value;
            opt.textContent = ft.label;
            if (ft.value === field.type) opt.selected = true;
            typeSelect.appendChild(opt);
          });
          if (kept) typeSelect.disabled = true;
          typeSelect.addEventListener('change', function () {
            fields[idx].type = this.value;
            // A calendar date has settings of its own, shown under its row.
            rerender();
          });
          row.appendChild(typeSelect);

          // Section input.
          var sectionInput = document.createElement('input');
          sectionInput.type = 'text';
          sectionInput.value = field.section || 'Basics';
          sectionInput.placeholder = 'Section';
          sectionInput.className = 'input text-xs py-1 w-20';
          sectionInput.setAttribute('aria-label', 'Section');
          sectionInput.addEventListener('input', function () {
            fields[idx].section = this.value;
            changed();
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
              rerender();
            });
            acts.appendChild(upBtn);
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
              rerender();
            });
            acts.appendChild(downBtn);
          }

          // Remove button.
          var removeBtn = document.createElement('button');
          removeBtn.type = 'button';
          removeBtn.className = 'text-red-400 hover:text-red-600 dark:hover:text-red-300 text-xs';
          removeBtn.innerHTML = '<i class="fa-solid fa-xmark"></i>';
          removeBtn.title = 'Remove field';
          removeBtn.addEventListener('click', function () {
            fields.splice(idx, 1);
            rerender();
          });
          acts.appendChild(removeBtn);
          row.appendChild(acts);

          fieldsList.appendChild(row);
          if (field.type === 'calendar_date') fieldsList.appendChild(dateSettings(field));
        });
      }

      // dateSettings edits a calendar date field: who sees it, "how long
      // ago", and the countdowns that run from it (field_dates.go checks
      // them on save).
      function dateSettings(field) {
        var box = document.createElement('div');
        box.className = 'ml-4 p-3 rounded border border-gray-200 dark:border-gray-700 space-y-2 text-xs';
        box.appendChild(check('Only the ' + Chronicle.gmTerm() + ' sees this date', !!field.gm_only, function (v) { field.gm_only = v; }));
        box.appendChild(check('Say how long ago it was', !!field.date_ago, function (v) { field.date_ago = v; }));
        var head = document.createElement('div');
        head.className = 'font-semibold text-gray-700 dark:text-gray-300 pt-1';
        head.textContent = 'Countdowns from this date';
        box.appendChild(head);
        field.countdowns = field.countdowns || [];
        field.countdowns.forEach(function (cd, ci) {
          var cdBox = document.createElement('div');
          cdBox.className = 'p-2 rounded bg-gray-50 dark:bg-gray-800 space-y-1.5';
          var top = document.createElement('div');
          top.className = 'flex items-center gap-2';
          top.appendChild(text(cd.name, 'Countdown name, like Soul lingers', 'flex-1', function (v) { cd.name = v; }));
          top.appendChild(iconBtn('fa-trash-can', 'Remove this countdown', function () { field.countdowns.splice(ci, 1); rerender(); }));
          cdBox.appendChild(top);
          cd.lengths = cd.lengths && cd.lengths.length ? cd.lengths : [{ days: 3 }];
          cd.lengths.forEach(function (len, li) {
            var lr = document.createElement('div');
            lr.className = 'flex items-center gap-2';
            if (li === 0) {
              var usual = document.createElement('span');
              usual.className = 'flex-1 text-gray-500 dark:text-gray-400';
              usual.textContent = cd.lengths.length > 1 ? 'Usually' : 'Runs';
              lr.appendChild(usual);
            } else {
              lr.appendChild(text(len.name, 'When it runs longer, like Strong-willed', 'flex-1', function (v) { len.name = v; }));
            }
            var days = document.createElement('input');
            days.type = 'number';
            days.min = '1';
            days.max = '3650';
            days.value = len.days || '';
            days.className = 'input text-xs py-1 w-16';
            days.setAttribute('aria-label', 'Length in days');
            days.addEventListener('input', function () { len.days = parseInt(this.value, 10) || 0; changed(); });
            lr.appendChild(days);
            var dl = document.createElement('span');
            dl.textContent = 'days';
            lr.appendChild(dl);
            if (li > 0) lr.appendChild(iconBtn('fa-xmark', 'Remove this length', function () { cd.lengths.splice(li, 1); rerender(); }));
            cdBox.appendChild(lr);
          });
          var more = document.createElement('button');
          more.type = 'button';
          more.className = 'text-blue-600 dark:text-blue-400 hover:underline';
          more.textContent = 'Add another length';
          more.addEventListener('click', function () { cd.lengths.push({ name: '', days: cd.lengths[0].days || 3 }); rerender(); });
          cdBox.appendChild(more);
          cdBox.appendChild(check('Only the ' + Chronicle.gmTerm() + ' sees the countdown', !!cd.gm_only, function (v) { cd.gm_only = v; }));
          box.appendChild(cdBox);
        });
        var add = document.createElement('button');
        add.type = 'button';
        add.className = 'text-blue-600 dark:text-blue-400 hover:underline';
        add.innerHTML = '<i class="fa-solid fa-plus word-ic mr-1"></i> Add a countdown';
        add.addEventListener('click', function () { field.countdowns.push({ name: '', lengths: [{ days: 3 }] }); rerender(); });
        box.appendChild(add);
        var hint = document.createElement('p');
        hint.className = 'text-gray-500 dark:text-gray-400';
        hint.textContent = 'Each page picks one of a countdown\u2019s lengths. Pages count from the campaign calendar\u2019s today.';
        box.appendChild(hint);
        return box;
      }

      function check(label, on, set) {
        var l = document.createElement('label');
        l.className = 'flex items-center gap-2 text-gray-700 dark:text-gray-300';
        var c = document.createElement('input');
        c.type = 'checkbox';
        c.checked = on;
        c.className = 'h-4 w-4 rounded';
        c.addEventListener('change', function () { set(this.checked); changed(true); });
        l.appendChild(c);
        l.appendChild(document.createTextNode(label));
        return l;
      }

      function text(value, placeholder, cls, set) {
        var i = document.createElement('input');
        i.type = 'text';
        i.value = value || '';
        i.placeholder = placeholder;
        i.maxLength = 60;
        i.className = 'input text-xs py-1 ' + cls;
        i.setAttribute('aria-label', placeholder);
        i.addEventListener('input', function () { set(this.value); changed(); });
        return i;
      }

      function iconBtn(icon, label, onClick) {
        var b = document.createElement('button');
        b.type = 'button';
        b.className = 'text-gray-400 hover:text-red-500 text-xs';
        b.innerHTML = '<i class="fa-solid ' + icon + '"></i>';
        b.title = label;
        b.setAttribute('aria-label', label);
        b.addEventListener('click', onClick);
        return b;
      }

      renderFields();
      snapshot();

      // Save button row.
      var saveRow = document.createElement('div');
      saveRow.className = 'flex items-center gap-3';

      var saveBtn = document.createElement('button');
      saveBtn.type = 'button';
      saveBtn.className = 'btn-primary text-sm';
      saveBtn.textContent = 'Save changes';

      var statusSpan = document.createElement('span');
      statusSpan.className = 'text-xs text-gray-500 dark:text-gray-400';

      saveBtn.addEventListener('click', function () {
        saveBtn.disabled = true;
        statusSpan.textContent = 'Saving...';
        statusSpan.className = 'text-xs text-gray-500 dark:text-gray-400';

        var payload;

        if (fieldsOnly) {
          // Fields-only mode: send only fields, plus required name/icon/color
          // from data attributes so the backend doesn't reject the request.
          payload = {
            name: currentName,
            name_plural: currentNamePlural,
            icon: currentIcon,
            color: currentColor,
            fields: fields.filter(function (f) { return f.key && f.label; })
          };
        } else {
          var nameInput = el.querySelector('#et-edit-name');
          var pluralInput = el.querySelector('#et-edit-plural');

          if (!nameInput.value.trim()) {
            statusSpan.textContent = 'Name is required';
            statusSpan.className = 'text-xs text-red-500';
            saveBtn.disabled = false;
            return;
          }

          payload = {
            name: nameInput.value.trim(),
            name_plural: pluralInput.value.trim(),
            icon: selectedIcon,
            color: colorInput.value,
            fields: fields.filter(function (f) { return f.key && f.label; })
          };
        }

        Chronicle.apiFetch(endpoint, {
          method: 'PUT',
          body: payload
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
          saveBtn.disabled = false;
          if (!fieldsOnly) {
            // Reload the page to reflect name/icon/color changes in the card list.
            setTimeout(function () {
              window.location.reload();
            }, 500);
          }
        })
        .catch(function (err) {
          statusSpan.textContent = err.message;
          statusSpan.className = 'text-xs text-red-500';
          saveBtn.disabled = false;
        });
      });

      saveRow.appendChild(saveBtn);
      saveRow.appendChild(statusSpan);
      if (!inDraft) el.appendChild(saveRow);

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
