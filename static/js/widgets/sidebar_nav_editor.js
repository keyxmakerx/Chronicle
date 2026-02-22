/**
 * sidebar_nav_editor.js -- Custom Sections & Links Editor Widget
 *
 * Mounts on a data-widget="sidebar-nav-editor" element. Provides CRUD for
 * custom navigation sections (dividers/headers) and custom links that appear
 * in the campaign sidebar alongside entity types.
 *
 * Reads and writes to the same sidebar-config endpoint as the sidebar_config
 * widget. Custom sections and links are stored in the `custom_sections` and
 * `custom_links` arrays within the sidebar_config JSON column.
 *
 * Config attributes:
 *   data-endpoint="/campaigns/:id/sidebar-config"  -- API endpoint
 *   data-campaign-id="..."                         -- Campaign ID
 *   data-csrf-token="..."                          -- CSRF token (fallback)
 */
(function () {
  'use strict';

  Chronicle.register('sidebar-nav-editor', {
    init: function (el, config) {
      var endpoint = config.endpoint;
      if (!endpoint) {
        console.error('[sidebar-nav-editor] Missing data-endpoint');
        return;
      }

      // State: the full sidebar config (we only touch custom_sections + custom_links).
      var sidebarConfig = {
        entity_type_order: [],
        hidden_type_ids: [],
        custom_sections: [],
        custom_links: []
      };

      // Track which item is being edited (null = none).
      var editingId = null;

      // Load current config from server.
      fetch(endpoint, {
        headers: { 'Accept': 'application/json' },
        credentials: 'same-origin'
      })
        .then(function (res) {
          if (!res.ok) throw new Error('HTTP ' + res.status);
          return res.json();
        })
        .then(function (data) {
          sidebarConfig = data || sidebarConfig;
          if (!sidebarConfig.custom_sections) sidebarConfig.custom_sections = [];
          if (!sidebarConfig.custom_links) sidebarConfig.custom_links = [];
          if (!sidebarConfig.entity_type_order) sidebarConfig.entity_type_order = [];
          if (!sidebarConfig.hidden_type_ids) sidebarConfig.hidden_type_ids = [];
          render();
        })
        .catch(function () {
          render();
        });

      /**
       * Generate a short random ID for new sections/links.
       */
      function genId() {
        return 'nav_' + Math.random().toString(36).substr(2, 8);
      }

      /**
       * Get CSRF token from cookie.
       */
      function getCsrf() {
        var match = document.cookie.match('(?:^|; )chronicle_csrf=([^;]*)');
        return match ? decodeURIComponent(match[1]) : '';
      }

      /**
       * Save the full sidebar config to the server.
       */
      function save(callback) {
        fetch(endpoint, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': getCsrf()
          },
          credentials: 'same-origin',
          body: JSON.stringify(sidebarConfig)
        })
          .then(function (res) {
            if (!res.ok) {
              console.error('[sidebar-nav-editor] Save returned HTTP ' + res.status);
              Chronicle.notify('Failed to save', 'error');
            } else {
              Chronicle.notify('Navigation updated', 'success');
            }
            if (callback) callback();
          })
          .catch(function (err) {
            console.error('[sidebar-nav-editor] Save failed:', err);
            Chronicle.notify('Failed to save', 'error');
            if (callback) callback();
          });
      }

      /**
       * Render the full widget UI.
       */
      function render() {
        var sections = sidebarConfig.custom_sections || [];
        var links = sidebarConfig.custom_links || [];
        var html = '';

        // --- Custom Sections ---
        html += '<div class="space-y-3">';
        html += '<div class="flex items-center justify-between">';
        html += '<h4 class="text-xs font-semibold text-fg-secondary uppercase tracking-wider">Sections</h4>';
        html += '<button type="button" class="add-section-btn text-xs text-accent hover:text-accent/80 font-medium">';
        html += '<i class="fa-solid fa-plus mr-1"></i>Add Section';
        html += '</button>';
        html += '</div>';

        if (sections.length === 0) {
          html += '<p class="text-xs text-fg-muted py-2">No custom sections yet.</p>';
        } else {
          html += '<div class="space-y-1.5">';
          sections.forEach(function (s) {
            html += renderSectionItem(s);
          });
          html += '</div>';
        }
        html += '</div>';

        // Divider.
        html += '<hr class="border-edge my-4"/>';

        // --- Custom Links ---
        html += '<div class="space-y-3">';
        html += '<div class="flex items-center justify-between">';
        html += '<h4 class="text-xs font-semibold text-fg-secondary uppercase tracking-wider">Links</h4>';
        html += '<button type="button" class="add-link-btn text-xs text-accent hover:text-accent/80 font-medium">';
        html += '<i class="fa-solid fa-plus mr-1"></i>Add Link';
        html += '</button>';
        html += '</div>';

        if (links.length === 0) {
          html += '<p class="text-xs text-fg-muted py-2">No custom links yet.</p>';
        } else {
          html += '<div class="space-y-1.5">';
          links.forEach(function (l) {
            html += renderLinkItem(l);
          });
          html += '</div>';
        }
        html += '</div>';

        el.innerHTML = html;
        bindEvents();
      }

      /**
       * Render a single section item row.
       */
      function renderSectionItem(section) {
        var isEditing = editingId === section.id;
        var h = '';
        h += '<div class="flex items-center gap-2 px-3 py-2 rounded-md border border-edge bg-surface-raised text-sm" data-section-id="' + esc(section.id) + '">';

        if (isEditing) {
          h += '<input type="text" class="edit-section-label input text-sm flex-1 py-1" value="' + esc(section.label) + '" placeholder="Section label" />';
          h += '<button type="button" class="save-section-btn p-1 text-green-500 hover:text-green-400" title="Save"><i class="fa-solid fa-check text-xs"></i></button>';
          h += '<button type="button" class="cancel-edit-btn p-1 text-fg-muted hover:text-fg" title="Cancel"><i class="fa-solid fa-xmark text-xs"></i></button>';
        } else {
          h += '<i class="fa-solid fa-minus text-fg-muted text-xs mr-1"></i>';
          h += '<span class="flex-1 text-fg font-medium">' + esc(section.label || 'Untitled Section') + '</span>';
          h += '<button type="button" class="edit-item-btn p-1 text-fg-muted hover:text-fg" data-item-id="' + esc(section.id) + '" title="Edit"><i class="fa-solid fa-pen text-xs"></i></button>';
          h += '<button type="button" class="delete-section-btn p-1 text-red-400 hover:text-red-300" data-section-id="' + esc(section.id) + '" title="Delete"><i class="fa-solid fa-trash text-xs"></i></button>';
        }

        h += '</div>';
        return h;
      }

      /**
       * Render a single link item row.
       */
      function renderLinkItem(link) {
        var isEditing = editingId === link.id;
        var h = '';
        h += '<div class="flex items-center gap-2 px-3 py-2 rounded-md border border-edge bg-surface-raised text-sm" data-link-id="' + esc(link.id) + '">';

        if (isEditing) {
          h += '<div class="flex-1 space-y-1.5">';
          h += '<input type="text" class="edit-link-label input text-sm w-full py-1" value="' + esc(link.label) + '" placeholder="Link label" />';
          h += '<input type="text" class="edit-link-url input text-sm w-full py-1" value="' + esc(link.url) + '" placeholder="URL (e.g. /page or https://...)" />';
          h += '<input type="text" class="edit-link-icon input text-sm w-full py-1" value="' + esc(link.icon) + '" placeholder="Icon class (e.g. fa-globe)" />';

          // Section assignment dropdown.
          var sections = sidebarConfig.custom_sections || [];
          h += '<select class="edit-link-section input text-sm w-full py-1">';
          h += '<option value=""' + (!link.section ? ' selected' : '') + '>Top level (no section)</option>';
          sections.forEach(function (s) {
            h += '<option value="' + esc(s.id) + '"' + (link.section === s.id ? ' selected' : '') + '>' + esc(s.label) + '</option>';
          });
          h += '</select>';

          h += '</div>';
          h += '<div class="flex flex-col gap-1">';
          h += '<button type="button" class="save-link-btn p-1 text-green-500 hover:text-green-400" title="Save"><i class="fa-solid fa-check text-xs"></i></button>';
          h += '<button type="button" class="cancel-edit-btn p-1 text-fg-muted hover:text-fg" title="Cancel"><i class="fa-solid fa-xmark text-xs"></i></button>';
          h += '</div>';
        } else {
          h += '<span class="w-4 h-4 flex items-center justify-center shrink-0">';
          h += '<i class="fa-solid ' + esc(link.icon || 'fa-link') + ' text-xs text-fg-muted"></i>';
          h += '</span>';
          h += '<div class="flex-1 min-w-0">';
          h += '<div class="text-fg font-medium truncate">' + esc(link.label || 'Untitled Link') + '</div>';
          h += '<div class="text-xs text-fg-muted truncate">' + esc(link.url || 'No URL') + '</div>';
          h += '</div>';
          h += '<button type="button" class="edit-item-btn p-1 text-fg-muted hover:text-fg" data-item-id="' + esc(link.id) + '" title="Edit"><i class="fa-solid fa-pen text-xs"></i></button>';
          h += '<button type="button" class="delete-link-btn p-1 text-red-400 hover:text-red-300" data-link-id="' + esc(link.id) + '" title="Delete"><i class="fa-solid fa-trash text-xs"></i></button>';
        }

        h += '</div>';
        return h;
      }

      /**
       * Bind all click/submit events after render.
       */
      function bindEvents() {
        // Add section.
        var addSectionBtn = el.querySelector('.add-section-btn');
        if (addSectionBtn) {
          addSectionBtn.addEventListener('click', function () {
            var newSection = { id: genId(), label: 'New Section', after: '' };
            sidebarConfig.custom_sections.push(newSection);
            editingId = newSection.id;
            render();
            // Focus the input.
            var input = el.querySelector('.edit-section-label');
            if (input) { input.focus(); input.select(); }
          });
        }

        // Add link.
        var addLinkBtn = el.querySelector('.add-link-btn');
        if (addLinkBtn) {
          addLinkBtn.addEventListener('click', function () {
            var newLink = { id: genId(), label: 'New Link', url: '', icon: 'fa-link', section: '', position: (sidebarConfig.custom_links || []).length };
            sidebarConfig.custom_links.push(newLink);
            editingId = newLink.id;
            render();
            var input = el.querySelector('.edit-link-label');
            if (input) { input.focus(); input.select(); }
          });
        }

        // Edit buttons (sections and links).
        el.querySelectorAll('.edit-item-btn').forEach(function (btn) {
          btn.addEventListener('click', function () {
            editingId = btn.getAttribute('data-item-id');
            render();
          });
        });

        // Cancel edit.
        el.querySelectorAll('.cancel-edit-btn').forEach(function (btn) {
          btn.addEventListener('click', function () {
            editingId = null;
            render();
          });
        });

        // Save section.
        el.querySelectorAll('.save-section-btn').forEach(function (btn) {
          btn.addEventListener('click', function () {
            var row = btn.closest('[data-section-id]');
            if (!row) return;
            var sectionId = row.getAttribute('data-section-id');
            var label = row.querySelector('.edit-section-label').value.trim();
            if (!label) { Chronicle.notify('Section label is required', 'error'); return; }

            var section = findSection(sectionId);
            if (section) { section.label = label; }
            editingId = null;
            save(function () { render(); });
          });
        });

        // Save link.
        el.querySelectorAll('.save-link-btn').forEach(function (btn) {
          btn.addEventListener('click', function () {
            var row = btn.closest('[data-link-id]');
            if (!row) return;
            var linkId = row.getAttribute('data-link-id');
            var label = row.querySelector('.edit-link-label').value.trim();
            var url = row.querySelector('.edit-link-url').value.trim();
            var icon = row.querySelector('.edit-link-icon').value.trim() || 'fa-link';
            var section = row.querySelector('.edit-link-section').value;

            if (!label) { Chronicle.notify('Link label is required', 'error'); return; }
            if (!url) { Chronicle.notify('Link URL is required', 'error'); return; }

            var link = findLink(linkId);
            if (link) {
              link.label = label;
              link.url = url;
              link.icon = icon;
              link.section = section;
            }
            editingId = null;
            save(function () { render(); });
          });
        });

        // Delete section.
        el.querySelectorAll('.delete-section-btn').forEach(function (btn) {
          btn.addEventListener('click', function () {
            var sectionId = btn.getAttribute('data-section-id');
            if (!confirm('Delete this section? Links assigned to it will become top-level.')) return;

            // Unassign any links from this section.
            (sidebarConfig.custom_links || []).forEach(function (l) {
              if (l.section === sectionId) l.section = '';
            });

            sidebarConfig.custom_sections = (sidebarConfig.custom_sections || []).filter(function (s) {
              return s.id !== sectionId;
            });
            save(function () { render(); });
          });
        });

        // Delete link.
        el.querySelectorAll('.delete-link-btn').forEach(function (btn) {
          btn.addEventListener('click', function () {
            var linkId = btn.getAttribute('data-link-id');
            if (!confirm('Delete this link?')) return;

            sidebarConfig.custom_links = (sidebarConfig.custom_links || []).filter(function (l) {
              return l.id !== linkId;
            });
            save(function () { render(); });
          });
        });

        // Allow Enter key to submit in edit fields.
        el.querySelectorAll('.edit-section-label').forEach(function (input) {
          input.addEventListener('keydown', function (e) {
            if (e.key === 'Enter') {
              var saveBtn = input.closest('[data-section-id]').querySelector('.save-section-btn');
              if (saveBtn) saveBtn.click();
            }
          });
        });
      }

      /**
       * Find a section by ID in the current config.
       */
      function findSection(id) {
        var sections = sidebarConfig.custom_sections || [];
        for (var i = 0; i < sections.length; i++) {
          if (sections[i].id === id) return sections[i];
        }
        return null;
      }

      /**
       * Find a link by ID in the current config.
       */
      function findLink(id) {
        var links = sidebarConfig.custom_links || [];
        for (var i = 0; i < links.length; i++) {
          if (links[i].id === id) return links[i];
        }
        return null;
      }

      /**
       * Escape HTML to prevent XSS.
       */
      function esc(text) {
        var div = document.createElement('div');
        div.textContent = String(text || '');
        return div.innerHTML;
      }
    },

    destroy: function (el) {
      el.innerHTML = '';
    }
  });
})();
