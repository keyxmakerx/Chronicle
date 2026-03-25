/**
 * template_picker.js -- Content Template Picker for Entity Create Form
 *
 * Automatically loads content templates from the API when the user selects an
 * entity type in the create form. Shows/hides the template picker dropdown
 * and populates it with available templates.
 *
 * Also adds "Insert Template" to the editor slash command menu for inserting
 * template content into an existing entity's editor.
 */
(function () {
  'use strict';

  // --- Template Picker in Create Form ---

  /**
   * Initialize the template picker on the entity create form.
   * Called on DOMContentLoaded and after HTMX swaps.
   */
  function initTemplatePicker() {
    var picker = document.getElementById('template-picker');
    var typeSelect = document.getElementById('entity_type_id');
    if (!picker || !typeSelect) return;

    var campaignId = picker.getAttribute('data-campaign-id');
    var templateSelect = document.getElementById('template_id');
    if (!campaignId || !templateSelect) return;

    // Avoid double-binding.
    if (typeSelect._templatePickerBound) return;
    typeSelect._templatePickerBound = true;

    typeSelect.addEventListener('change', function () {
      var etId = typeSelect.value;
      if (!etId || etId === '0') {
        picker.classList.add('hidden');
        templateSelect.innerHTML = '<option value="0">Blank — start from scratch</option>';
        return;
      }
      loadTemplates(campaignId, etId, templateSelect, picker);
    });

    // If entity type is pre-selected, load templates immediately.
    if (typeSelect.value && typeSelect.value !== '0') {
      loadTemplates(campaignId, typeSelect.value, templateSelect, picker);
    }
  }

  /**
   * Fetch templates for the given entity type and populate the select dropdown.
   */
  function loadTemplates(campaignId, entityTypeId, selectEl, pickerEl) {
    var url = '/campaigns/' + encodeURIComponent(campaignId) +
              '/content-templates?entity_type_id=' + encodeURIComponent(entityTypeId);

    Chronicle.apiFetch(url)
      .then(function (resp) { return resp.json(); })
      .then(function (templates) {
        var html = '<option value="0">Blank — start from scratch</option>';
        if (templates && templates.length > 0) {
          for (var i = 0; i < templates.length; i++) {
            var t = templates[i];
            var icon = t.icon || 'fa-file-lines';
            html += '<option value="' + t.id + '">' + Chronicle.escapeHtml(t.name) + '</option>';
          }
          pickerEl.classList.remove('hidden');
        } else {
          pickerEl.classList.add('hidden');
        }
        selectEl.innerHTML = html;
      })
      .catch(function () {
        // Silently fail — template picker is optional.
        pickerEl.classList.add('hidden');
      });
  }

  // --- Insert Template in Editor Slash Menu ---

  /**
   * Register a "Template" slash command that shows available templates
   * and inserts the selected template's content into the editor.
   */
  function registerTemplateSlashCommand() {
    if (!window.Chronicle || !Chronicle.SlashCommands) return;

    // Add the template command to the slash command list.
    Chronicle.SlashCommands.addCommand({
      id: 'insertTemplate',
      label: 'Insert Template',
      icon: 'fa-file-lines',
      keywords: 'template content prefill insert',
      description: 'Insert a content template',
      // Custom handler: fetch templates and show a sub-menu.
      customHandler: function (editor) {
        var campaignId = editor.options.element
          ? editor.options.element.closest('[data-campaign-id]')
          : null;
        if (campaignId) {
          campaignId = campaignId.getAttribute('data-campaign-id');
        }
        if (!campaignId) {
          // Try the parent widget element.
          var widget = editor.options.element
            ? editor.options.element.closest('[data-widget="editor"]')
            : null;
          if (widget) {
            campaignId = widget.getAttribute('data-campaign-id');
          }
        }
        if (!campaignId) return;

        showTemplateInsertMenu(editor, campaignId);
      }
    });
  }

  /**
   * Show a floating menu of available templates for insertion.
   */
  function showTemplateInsertMenu(editor, campaignId) {
    var url = '/campaigns/' + encodeURIComponent(campaignId) + '/content-templates';

    Chronicle.apiFetch(url)
      .then(function (resp) { return resp.json(); })
      .then(function (templates) {
        if (!templates || templates.length === 0) {
          Chronicle.notify('No content templates available.', 'info');
          return;
        }
        renderTemplateMenu(editor, templates);
      })
      .catch(function () {
        Chronicle.notify('Failed to load templates.', 'error');
      });
  }

  /**
   * Render and show the template selection floating menu.
   */
  function renderTemplateMenu(editor, templates) {
    // Remove any existing menu.
    var existing = document.getElementById('template-insert-menu');
    if (existing) existing.remove();

    var isDark = document.documentElement.classList.contains('dark');
    var menu = document.createElement('div');
    menu.id = 'template-insert-menu';
    menu.style.cssText =
      'position:fixed;z-index:9999;min-width:260px;max-width:340px;max-height:320px;' +
      'overflow-y:auto;border-radius:8px;box-shadow:0 4px 16px rgba(0,0,0,0.15);' +
      'padding:4px 0;' +
      (isDark
        ? 'background:#1f2937;border:1px solid #374151;color:#e5e7eb;'
        : 'background:#fff;border:1px solid #e5e7eb;color:#111827;');

    // Position near cursor.
    var coords = editor.view.coordsAtPos(editor.state.selection.from);
    menu.style.left = coords.left + 'px';
    menu.style.top = (coords.bottom + 4) + 'px';

    // Header.
    menu.innerHTML =
      '<div style="padding:4px 12px 2px;font-size:11px;font-weight:600;opacity:0.5;' +
      'text-transform:uppercase;letter-spacing:0.5px">Content Templates</div>';

    for (var i = 0; i < templates.length; i++) {
      var t = templates[i];
      var item = document.createElement('div');
      item.style.cssText =
        'display:flex;align-items:center;gap:8px;padding:8px 12px;cursor:pointer;' +
        'font-size:14px;transition:background 0.1s;';
      item.innerHTML =
        '<i class="fa-solid ' + Chronicle.escapeHtml(t.icon || 'fa-file-lines') +
        '" style="width:20px;text-align:center;opacity:0.6"></i>' +
        '<div><div style="font-weight:500">' + Chronicle.escapeHtml(t.name) + '</div>' +
        (t.description
          ? '<div style="font-size:12px;opacity:0.6">' + Chronicle.escapeHtml(t.description) + '</div>'
          : '') +
        '</div>';

      item.addEventListener('mouseenter', function () {
        this.style.backgroundColor = isDark ? '#374151' : '#f3f4f6';
      });
      item.addEventListener('mouseleave', function () {
        this.style.backgroundColor = '';
      });

      (function (tmpl) {
        item.addEventListener('mousedown', function (e) {
          e.preventDefault();
          e.stopPropagation();
          insertTemplateContent(editor, tmpl);
          menu.remove();
        });
      })(t);

      menu.appendChild(item);
    }

    document.body.appendChild(menu);

    // Close on click outside.
    function closeMenu(e) {
      if (!menu.contains(e.target)) {
        menu.remove();
        document.removeEventListener('mousedown', closeMenu);
      }
    }
    setTimeout(function () {
      document.addEventListener('mousedown', closeMenu);
    }, 50);
  }

  /**
   * Insert the template's content JSON into the editor at the current cursor.
   */
  function insertTemplateContent(editor, template) {
    if (!template.content_json) return;

    try {
      var doc = JSON.parse(template.content_json);
      if (doc.type === 'doc' && doc.content) {
        // Insert the template's content nodes at the current position.
        var nodes = [];
        for (var i = 0; i < doc.content.length; i++) {
          var node = editor.schema.nodeFromJSON(doc.content[i]);
          if (node) nodes.push(node);
        }

        if (nodes.length > 0) {
          var fragment = window.ProseMirrorModel
            ? window.ProseMirrorModel.Fragment.from(nodes)
            : editor.state.schema.node('doc', null, nodes).content;

          var tr = editor.state.tr;
          tr.insert(tr.selection.from, fragment);
          editor.view.dispatch(tr);
          Chronicle.notify('Template "' + template.name + '" inserted.', 'success');
        }
      }
    } catch (err) {
      Chronicle.notify('Failed to insert template.', 'error');
    }
  }

  // --- Initialization ---

  // Init on page load.
  document.addEventListener('DOMContentLoaded', function () {
    initTemplatePicker();
  });

  // Re-init after HTMX content swaps (e.g., navigating to create form).
  document.addEventListener('htmx:afterSettle', function () {
    initTemplatePicker();
  });

  // Register slash command when SlashCommands is available.
  if (window.Chronicle && Chronicle.SlashCommands && Chronicle.SlashCommands.addCommand) {
    registerTemplateSlashCommand();
  } else {
    // Wait for SlashCommands to be registered.
    document.addEventListener('DOMContentLoaded', function () {
      if (window.Chronicle && Chronicle.SlashCommands && Chronicle.SlashCommands.addCommand) {
        registerTemplateSlashCommand();
      }
    });
  }
})();
