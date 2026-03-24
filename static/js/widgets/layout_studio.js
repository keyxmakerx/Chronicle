/**
 * layout_studio.js -- Unified Layout Studio Widget
 *
 * Combines campaign dashboard editing, page template editing, and category
 * management into one unified two-panel interface. Left sidebar provides
 * context navigation (campaign dashboards, per-category editors). Right
 * panel dynamically mounts the appropriate editor widget.
 *
 * Mount: data-widget="layout-studio"
 * Config:
 *   data-campaign-id   - Campaign UUID
 *   data-csrf-token    - CSRF token
 *   data-entity-types  - JSON array of entity types [{id, name, name_plural, icon, color}]
 */
(function () {
  'use strict';

  // Editor context types.
  var CTX_CAMPAIGN_DASH = 'campaign-dashboard';
  var CTX_OWNER_DASH = 'owner-dashboard';
  var CTX_PAGE_TEMPLATE = 'page-template';
  var CTX_CATEGORY_DASH = 'category-dashboard';

  Chronicle.register('layout-studio', {
    init: function (el, config) {
      this.el = el;
      this.campaignId = config.campaignId;
      this.csrfToken = config.csrfToken;
      this.entityTypes = [];
      this.activeContext = null; // { type, etid?, role? }
      this.activeWidget = null; // Reference to mounted widget element.
      this.dirty = false;

      try {
        this.entityTypes = JSON.parse(el.getAttribute('data-entity-types') || '[]');
      } catch (e) {
        this.entityTypes = [];
      }

      this.render();
      // Auto-select campaign dashboard on load.
      this.selectContext({ type: CTX_CAMPAIGN_DASH, role: 'default' });
    },

    destroy: function () {
      this.destroyActiveEditor();
    },

    /**
     * Render the full two-panel layout.
     */
    render: function () {
      var self = this;
      this.el.innerHTML = '';
      this.el.className = 'flex h-full overflow-hidden';

      // --- Left Sidebar ---
      var sidebar = document.createElement('div');
      sidebar.className = 'w-64 bg-surface border-r border-edge flex flex-col shrink-0 overflow-y-auto';
      sidebar.innerHTML = '<div class="px-3 pt-3 pb-2">' +
        '<h3 class="text-xs font-semibold text-fg-secondary uppercase tracking-wider">Layout Studio</h3>' +
        '</div>';

      // Campaign Dashboards section.
      var dashSection = this.createSection('Campaign Dashboards', 'fa-grip', [
        { label: 'Campaign Page', icon: 'fa-home', context: { type: CTX_CAMPAIGN_DASH, role: 'default' } },
        { label: 'Owner Dashboard', icon: 'fa-shield-halved', context: { type: CTX_OWNER_DASH } }
      ]);
      sidebar.appendChild(dashSection);

      // Categories section with add button.
      var catHeader = document.createElement('div');
      catHeader.className = 'flex items-center justify-between px-3 pt-4 pb-1';
      catHeader.innerHTML = '<span class="text-[10px] font-semibold text-fg-secondary uppercase tracking-wider">Categories</span>';
      var addBtn = document.createElement('button');
      addBtn.className = 'text-[10px] text-accent hover:text-accent-hover font-medium';
      addBtn.innerHTML = '<i class="fa-solid fa-plus mr-0.5"></i>Add';
      addBtn.addEventListener('click', function () { self.showCreateCategory(); });
      catHeader.appendChild(addBtn);
      sidebar.appendChild(catHeader);

      // Category items.
      this.catListEl = document.createElement('div');
      this.catListEl.className = 'px-1 pb-2';
      this.renderCategoryList();
      sidebar.appendChild(this.catListEl);

      // Empty state for categories.
      if (this.entityTypes.length === 0) {
        var empty = document.createElement('div');
        empty.className = 'px-3 py-4 text-center';
        empty.innerHTML = '<p class="text-xs text-fg-muted">No categories yet.</p>' +
          '<button class="text-xs text-accent hover:text-accent-hover font-medium mt-1">Create one</button>';
        empty.querySelector('button').addEventListener('click', function () { self.showCreateCategory(); });
        this.catListEl.appendChild(empty);
      }

      this.sidebarEl = sidebar;
      this.el.appendChild(sidebar);

      // --- Right Panel (Editor) ---
      var main = document.createElement('div');
      main.className = 'flex-1 flex flex-col min-w-0 overflow-hidden';

      // Context header bar.
      this.headerEl = document.createElement('div');
      this.headerEl.className = 'flex items-center justify-between px-4 py-2 bg-surface border-b border-edge shrink-0';
      main.appendChild(this.headerEl);

      // Editor mount point.
      this.editorEl = document.createElement('div');
      this.editorEl.className = 'flex-1 overflow-hidden';
      main.appendChild(this.editorEl);

      this.el.appendChild(main);
    },

    /**
     * Create a collapsible sidebar section.
     */
    createSection: function (title, icon, items) {
      var self = this;
      var section = document.createElement('div');

      var header = document.createElement('div');
      header.className = 'flex items-center gap-1.5 px-3 pt-3 pb-1 cursor-pointer select-none';
      header.innerHTML = '<i class="fa-solid ' + icon + ' text-[10px] text-fg-muted"></i>' +
        '<span class="text-[10px] font-semibold text-fg-secondary uppercase tracking-wider flex-1">' + Chronicle.escapeHtml(title) + '</span>' +
        '<i class="fa-solid fa-chevron-down text-[8px] text-fg-muted transition-transform section-chevron"></i>';

      var list = document.createElement('div');
      list.className = 'px-1 pb-1';

      header.addEventListener('click', function () {
        var hidden = list.style.display === 'none';
        list.style.display = hidden ? '' : 'none';
        header.querySelector('.section-chevron').style.transform = hidden ? '' : 'rotate(-90deg)';
      });

      items.forEach(function (item) {
        var btn = document.createElement('button');
        btn.className = 'ls-nav-item w-full flex items-center gap-2 px-2.5 py-1.5 rounded-md text-xs text-fg-secondary hover:bg-surface-alt hover:text-fg transition-colors text-left';
        btn.innerHTML = '<i class="fa-solid ' + item.icon + ' w-4 text-center text-fg-muted"></i>' +
          '<span>' + Chronicle.escapeHtml(item.label) + '</span>';
        btn.dataset.contextType = item.context.type;
        if (item.context.etid) btn.dataset.contextEtid = item.context.etid;
        btn.addEventListener('click', function () {
          self.selectContext(item.context);
        });
        list.appendChild(btn);
      });

      section.appendChild(header);
      section.appendChild(list);
      return section;
    },

    /**
     * Render the category list in the sidebar.
     */
    renderCategoryList: function () {
      var self = this;
      this.catListEl.innerHTML = '';

      this.entityTypes.forEach(function (et) {
        var group = document.createElement('div');
        group.className = 'mb-0.5';

        // Category header row.
        var row = document.createElement('div');
        row.className = 'flex items-center gap-1 px-1 group';

        // Expand/collapse toggle.
        var toggle = document.createElement('button');
        toggle.className = 'w-4 h-4 flex items-center justify-center text-[8px] text-fg-muted hover:text-fg transition-colors shrink-0';
        toggle.innerHTML = '<i class="fa-solid fa-chevron-down"></i>';
        var subItems = document.createElement('div');
        subItems.className = 'ml-5 pl-2 border-l border-edge/50';
        var expanded = true;
        toggle.addEventListener('click', function () {
          expanded = !expanded;
          subItems.style.display = expanded ? '' : 'none';
          toggle.querySelector('i').style.transform = expanded ? '' : 'rotate(-90deg)';
        });

        // Category icon + name.
        var catLabel = document.createElement('div');
        catLabel.className = 'flex items-center gap-1.5 flex-1 min-w-0 px-1.5 py-1 rounded-md text-xs font-medium text-fg truncate';
        catLabel.innerHTML = '<span class="w-4 h-4 rounded flex items-center justify-center text-[8px] shrink-0" style="background-color:' + et.color + '20;color:' + et.color + '">' +
          '<i class="fa-solid ' + (et.icon || 'fa-file') + '"></i></span>' +
          '<span class="truncate">' + Chronicle.escapeHtml(et.name_plural || et.name) + '</span>';

        // Category actions (edit/delete).
        var actions = document.createElement('div');
        actions.className = 'flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity shrink-0';
        var editBtn = document.createElement('button');
        editBtn.className = 'w-5 h-5 flex items-center justify-center rounded text-[9px] text-fg-muted hover:text-fg hover:bg-surface-alt';
        editBtn.innerHTML = '<i class="fa-solid fa-pen"></i>';
        editBtn.title = 'Edit category';
        editBtn.addEventListener('click', function (e) {
          e.stopPropagation();
          self.showEditCategory(et);
        });
        var delBtn = document.createElement('button');
        delBtn.className = 'w-5 h-5 flex items-center justify-center rounded text-[9px] text-fg-muted hover:text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-900/20';
        delBtn.innerHTML = '<i class="fa-solid fa-trash"></i>';
        delBtn.title = 'Delete category';
        delBtn.addEventListener('click', function (e) {
          e.stopPropagation();
          self.deleteCategory(et);
        });
        actions.appendChild(editBtn);
        actions.appendChild(delBtn);

        row.appendChild(toggle);
        row.appendChild(catLabel);
        row.appendChild(actions);
        group.appendChild(row);

        // Sub-items: Page Template + Category Dashboard.
        var pageBtn = document.createElement('button');
        pageBtn.className = 'ls-nav-item w-full flex items-center gap-2 px-2 py-1 rounded-md text-xs text-fg-secondary hover:bg-surface-alt hover:text-fg transition-colors text-left';
        pageBtn.innerHTML = '<i class="fa-solid fa-layer-group w-3 text-center text-fg-muted text-[9px]"></i><span>Page Template</span>';
        pageBtn.dataset.contextType = CTX_PAGE_TEMPLATE;
        pageBtn.dataset.contextEtid = et.id;
        pageBtn.addEventListener('click', function () {
          self.selectContext({ type: CTX_PAGE_TEMPLATE, etid: et.id, etName: et.name, etIcon: et.icon, etColor: et.color });
        });

        var dashBtn = document.createElement('button');
        dashBtn.className = 'ls-nav-item w-full flex items-center gap-2 px-2 py-1 rounded-md text-xs text-fg-secondary hover:bg-surface-alt hover:text-fg transition-colors text-left';
        dashBtn.innerHTML = '<i class="fa-solid fa-grip w-3 text-center text-fg-muted text-[9px]"></i><span>Category Dashboard</span>';
        dashBtn.dataset.contextType = CTX_CATEGORY_DASH;
        dashBtn.dataset.contextEtid = et.id;
        dashBtn.addEventListener('click', function () {
          self.selectContext({ type: CTX_CATEGORY_DASH, etid: et.id, etName: et.name, etIcon: et.icon, etColor: et.color });
        });

        subItems.appendChild(pageBtn);
        subItems.appendChild(dashBtn);
        group.appendChild(subItems);

        self.catListEl.appendChild(group);
      });
    },

    /**
     * Select a context and mount the appropriate editor.
     */
    selectContext: function (ctx) {
      // Warn about unsaved changes.
      if (this.dirty) {
        if (!confirm('You have unsaved changes. Switch and discard them?')) return;
      }

      this.activeContext = ctx;
      this.dirty = false;
      this.updateNavHighlight();
      this.updateHeader();
      this.mountEditor();
    },

    /**
     * Highlight the active nav item.
     */
    updateNavHighlight: function () {
      var ctx = this.activeContext;
      this.el.querySelectorAll('.ls-nav-item').forEach(function (btn) {
        var match = btn.dataset.contextType === ctx.type &&
          (!ctx.etid || btn.dataset.contextEtid == ctx.etid);
        if (match) {
          btn.classList.add('bg-accent/10', 'text-accent', 'font-medium');
          btn.classList.remove('text-fg-secondary');
        } else {
          btn.classList.remove('bg-accent/10', 'text-accent', 'font-medium');
          btn.classList.add('text-fg-secondary');
        }
      });
    },

    /**
     * Update the header bar for the current context.
     */
    updateHeader: function () {
      var ctx = this.activeContext;
      var h = '';

      if (ctx.type === CTX_CAMPAIGN_DASH) {
        h += '<div class="flex items-center gap-2">';
        h += '<i class="fa-solid fa-home text-xs text-fg-muted"></i>';
        h += '<span class="text-sm font-medium text-fg">Campaign Page</span>';
        h += '</div>';
        // Role toggle.
        h += '<div class="inline-flex rounded-lg bg-surface-alt p-0.5">';
        h += '<button type="button" class="ls-role-btn text-[11px] px-2.5 py-1 rounded-md font-medium transition-colors" data-role="default">Default</button>';
        h += '<button type="button" class="ls-role-btn text-[11px] px-2.5 py-1 rounded-md font-medium transition-colors" data-role="player">Player</button>';
        h += '<button type="button" class="ls-role-btn text-[11px] px-2.5 py-1 rounded-md font-medium transition-colors" data-role="scribe">Scribe</button>';
        h += '</div>';
      } else if (ctx.type === CTX_OWNER_DASH) {
        h += '<div class="flex items-center gap-2">';
        h += '<i class="fa-solid fa-shield-halved text-xs text-fg-muted"></i>';
        h += '<span class="text-sm font-medium text-fg">Owner Dashboard</span>';
        h += '<span class="text-[10px] text-fg-muted">(visible to owner & co-DMs only)</span>';
        h += '</div>';
      } else if (ctx.type === CTX_PAGE_TEMPLATE) {
        h += '<div class="flex items-center gap-2">';
        if (ctx.etColor) {
          h += '<span class="w-6 h-6 rounded flex items-center justify-center text-[10px]" style="background-color:' + ctx.etColor + '20;color:' + ctx.etColor + '">';
          h += '<i class="fa-solid ' + (ctx.etIcon || 'fa-file') + '"></i></span>';
        }
        h += '<span class="text-sm font-medium text-fg">' + Chronicle.escapeHtml(ctx.etName || 'Entity') + ' Page Template</span>';
        h += '</div>';
        h += '<div class="flex items-center gap-2">';
        h += '<span id="ls-te-save-status" class="text-xs text-fg-muted"></span>';
        h += '<button id="ls-te-save-btn" class="btn-primary text-sm">Save Layout</button>';
        h += '</div>';
      } else if (ctx.type === CTX_CATEGORY_DASH) {
        h += '<div class="flex items-center gap-2">';
        if (ctx.etColor) {
          h += '<span class="w-6 h-6 rounded flex items-center justify-center text-[10px]" style="background-color:' + ctx.etColor + '20;color:' + ctx.etColor + '">';
          h += '<i class="fa-solid ' + (ctx.etIcon || 'fa-file') + '"></i></span>';
        }
        h += '<span class="text-sm font-medium text-fg">' + Chronicle.escapeHtml(ctx.etName || 'Entity') + ' Category Dashboard</span>';
        h += '</div>';
      }

      this.headerEl.innerHTML = h;

      // Bind role toggle buttons.
      var self = this;
      if (ctx.type === CTX_CAMPAIGN_DASH) {
        var role = ctx.role || 'default';
        this.headerEl.querySelectorAll('.ls-role-btn').forEach(function (btn) {
          if (btn.dataset.role === role) {
            btn.classList.add('bg-surface', 'text-fg', 'shadow-sm');
            btn.classList.remove('text-fg-secondary', 'hover:text-fg');
          } else {
            btn.classList.remove('bg-surface', 'text-fg', 'shadow-sm');
            btn.classList.add('text-fg-secondary', 'hover:text-fg');
          }
          btn.addEventListener('click', function () {
            var newRole = btn.dataset.role;
            if (newRole !== self.activeContext.role) {
              self.activeContext.role = newRole;
              self.updateHeader();
              // Dispatch role change to the mounted editor.
              if (self.activeWidget) {
                self.activeWidget.setAttribute('data-role', newRole);
                self.activeWidget.dispatchEvent(new CustomEvent('role-change', { detail: { role: newRole } }));
              }
            }
          });
        });
      }
    },

    /**
     * Mount the unified layout-editor widget for the current context.
     * Passes context-appropriate features and data attributes.
     */
    mountEditor: function () {
      this.destroyActiveEditor();
      var ctx = this.activeContext;

      if (ctx.type === CTX_PAGE_TEMPLATE) {
        // Template context: fetch layout first since it's not a simple GET.
        this._mountTemplateEditor(ctx.etid);
        return;
      }

      var widgetEl = document.createElement('div');
      widgetEl.className = 'h-full';
      widgetEl.setAttribute('data-widget', 'layout-editor');
      widgetEl.setAttribute('data-campaign-id', this.campaignId);
      widgetEl.setAttribute('data-csrf-token', this.csrfToken);
      widgetEl.setAttribute('data-context', 'dashboard');

      if (ctx.type === CTX_CAMPAIGN_DASH) {
        widgetEl.setAttribute('data-endpoint', '/campaigns/' + this.campaignId + '/dashboard-layout');
        widgetEl.setAttribute('data-features', 'roles');
        widgetEl.setAttribute('data-role', ctx.role || 'default');
      } else if (ctx.type === CTX_OWNER_DASH) {
        widgetEl.setAttribute('data-endpoint', '/campaigns/' + this.campaignId + '/owner-dashboard-layout');
      } else if (ctx.type === CTX_CATEGORY_DASH) {
        widgetEl.setAttribute('data-endpoint', '/campaigns/' + this.campaignId + '/entity-types/' + ctx.etid + '/dashboard-layout');
      }

      this.editorEl.innerHTML = '';
      this.editorEl.appendChild(widgetEl);
      this.activeWidget = widgetEl;

      // Mount the widget via Chronicle's boot system.
      Chronicle.mountWidget(widgetEl);
    },

    /**
     * Mount the unified layout-editor for a page template context.
     * Fetches the layout from the API first, then creates the widget.
     */
    _mountTemplateEditor: function (etid) {
      var self = this;
      var endpoint = '/campaigns/' + this.campaignId + '/entity-types/' + etid + '/layout';

      // Show loading state.
      this.editorEl.innerHTML = '<div class="h-full flex items-center justify-center"><i class="fa-solid fa-spinner fa-spin text-fg-muted text-lg"></i></div>';

      Chronicle.apiFetch(endpoint)
        .then(function (res) { return res.ok ? res.json() : null; })
        .then(function (data) {
          var widgetEl = document.createElement('div');
          widgetEl.className = 'h-full';
          widgetEl.setAttribute('data-widget', 'layout-editor');
          widgetEl.setAttribute('data-endpoint', endpoint);
          widgetEl.setAttribute('data-campaign-id', self.campaignId);
          widgetEl.setAttribute('data-csrf-token', self.csrfToken);
          widgetEl.setAttribute('data-context', 'template');
          widgetEl.setAttribute('data-features', 'containers,visibility,height,presets,preview');
          if (data) {
            widgetEl.setAttribute('data-layout', JSON.stringify(data));
          }
          // Find the matching entity type for the name.
          var et = self.entityTypes.find(function (e) { return e.id == etid; });
          if (et) widgetEl.setAttribute('data-entity-type-name', et.name);

          self.editorEl.innerHTML = '';
          self.editorEl.appendChild(widgetEl);
          self.activeWidget = widgetEl;
          Chronicle.mountWidget(widgetEl);
        })
        .catch(function () {
          self.editorEl.innerHTML = '<div class="h-full flex items-center justify-center text-fg-muted text-sm">Failed to load layout.</div>';
        });
    },

    /**
     * Destroy the currently active editor widget.
     */
    destroyActiveEditor: function () {
      if (this.activeWidget) {
        Chronicle.destroyWidget(this.activeWidget);
        this.activeWidget = null;
      }
      if (this.editorEl) {
        this.editorEl.innerHTML = '';
      }
    },

    /**
     * Show inline create category form.
     */
    showCreateCategory: function () {
      var self = this;
      // Check if form already exists.
      if (this.catListEl.querySelector('.ls-cat-form')) return;

      var form = document.createElement('div');
      form.className = 'ls-cat-form mx-1 my-1 p-2 rounded-md border border-accent/30 bg-accent/5 space-y-2';
      form.innerHTML =
        '<input type="text" class="input text-xs w-full" placeholder="Category name (singular)" id="ls-cat-name"/>' +
        '<input type="text" class="input text-xs w-full" placeholder="Plural name" id="ls-cat-plural"/>' +
        '<div class="flex gap-2">' +
        '  <input type="text" class="input text-xs flex-1" placeholder="Icon (e.g. fa-user)" id="ls-cat-icon" value="fa-file"/>' +
        '  <input type="color" class="w-8 h-8 rounded border border-edge cursor-pointer shrink-0" id="ls-cat-color" value="#6366f1"/>' +
        '</div>' +
        '<div class="flex gap-1.5 justify-end">' +
        '  <button type="button" class="text-xs text-fg-muted hover:text-fg px-2 py-1" id="ls-cat-cancel">Cancel</button>' +
        '  <button type="button" class="btn-primary text-xs px-3 py-1" id="ls-cat-save">Create</button>' +
        '</div>';

      this.catListEl.insertBefore(form, this.catListEl.firstChild);

      form.querySelector('#ls-cat-cancel').addEventListener('click', function () {
        form.remove();
      });

      form.querySelector('#ls-cat-save').addEventListener('click', function () {
        var name = form.querySelector('#ls-cat-name').value.trim();
        var plural = form.querySelector('#ls-cat-plural').value.trim();
        var icon = form.querySelector('#ls-cat-icon').value.trim();
        var color = form.querySelector('#ls-cat-color').value;
        if (!name) { Chronicle.notify('Name is required', 'error'); return; }
        if (!plural) plural = name + 's';

        var saveBtn = form.querySelector('#ls-cat-save');
        saveBtn.disabled = true;
        saveBtn.textContent = 'Creating...';

        Chronicle.apiFetch('/campaigns/' + self.campaignId + '/entity-types', {
          method: 'POST',
          body: { name: name, name_plural: plural, icon: icon, color: color },
          csrfToken: self.csrfToken
        }).then(function (res) {
          if (!res.ok) throw new Error('Failed');
          return res.json();
        }).then(function (data) {
          // Add to local entity types list.
          self.entityTypes.push({
            id: data.id || data.ID,
            name: name,
            name_plural: plural,
            icon: icon,
            color: color
          });
          form.remove();
          self.renderCategoryList();
          Chronicle.notify('Category created', 'success');
        }).catch(function () {
          saveBtn.disabled = false;
          saveBtn.textContent = 'Create';
          Chronicle.notify('Failed to create category', 'error');
        });
      });

      form.querySelector('#ls-cat-name').focus();
    },

    /**
     * Show inline edit category form.
     */
    showEditCategory: function (et) {
      var self = this;
      var form = document.createElement('div');
      form.className = 'ls-cat-form mx-1 my-1 p-2 rounded-md border border-accent/30 bg-accent/5 space-y-2';
      form.innerHTML =
        '<input type="text" class="input text-xs w-full" placeholder="Name (singular)" id="ls-cat-name" value="' + Chronicle.escapeHtml(et.name) + '"/>' +
        '<input type="text" class="input text-xs w-full" placeholder="Plural name" id="ls-cat-plural" value="' + Chronicle.escapeHtml(et.name_plural) + '"/>' +
        '<div class="flex gap-2">' +
        '  <input type="text" class="input text-xs flex-1" placeholder="Icon" id="ls-cat-icon" value="' + Chronicle.escapeHtml(et.icon || 'fa-file') + '"/>' +
        '  <input type="color" class="w-8 h-8 rounded border border-edge cursor-pointer shrink-0" id="ls-cat-color" value="' + (et.color || '#6366f1') + '"/>' +
        '</div>' +
        '<div class="flex gap-1.5 justify-end">' +
        '  <button type="button" class="text-xs text-fg-muted hover:text-fg px-2 py-1" id="ls-cat-cancel">Cancel</button>' +
        '  <button type="button" class="btn-primary text-xs px-3 py-1" id="ls-cat-save">Save</button>' +
        '</div>';

      // Find the category row and replace with form.
      var rows = this.catListEl.querySelectorAll('.group');
      var targetRow = null;
      rows.forEach(function (row) {
        // Match by checking the sub-items' etid.
        var btn = row.querySelector('[data-context-etid="' + et.id + '"]');
        if (btn) targetRow = row;
      });
      if (targetRow) {
        targetRow.style.display = 'none';
        targetRow.parentNode.insertBefore(form, targetRow.nextSibling);
      } else {
        this.catListEl.appendChild(form);
      }

      form.querySelector('#ls-cat-cancel').addEventListener('click', function () {
        form.remove();
        if (targetRow) targetRow.style.display = '';
      });

      form.querySelector('#ls-cat-save').addEventListener('click', function () {
        var name = form.querySelector('#ls-cat-name').value.trim();
        var plural = form.querySelector('#ls-cat-plural').value.trim();
        var icon = form.querySelector('#ls-cat-icon').value.trim();
        var color = form.querySelector('#ls-cat-color').value;
        if (!name) { Chronicle.notify('Name is required', 'error'); return; }

        var saveBtn = form.querySelector('#ls-cat-save');
        saveBtn.disabled = true;
        saveBtn.textContent = 'Saving...';

        Chronicle.apiFetch('/campaigns/' + self.campaignId + '/entity-types/' + et.id, {
          method: 'PUT',
          body: { name: name, name_plural: plural, icon: icon, color: color },
          csrfToken: self.csrfToken
        }).then(function (res) {
          if (!res.ok) throw new Error('Failed');
          // Update local data.
          et.name = name;
          et.name_plural = plural;
          et.icon = icon;
          et.color = color;
          form.remove();
          if (targetRow) targetRow.remove();
          self.renderCategoryList();
          Chronicle.notify('Category updated', 'success');
        }).catch(function () {
          saveBtn.disabled = false;
          saveBtn.textContent = 'Save';
          Chronicle.notify('Failed to update category', 'error');
        });
      });

      form.querySelector('#ls-cat-name').focus();
    },

    /**
     * Delete a category after confirmation.
     */
    deleteCategory: function (et) {
      var self = this;
      if (!confirm('Delete "' + et.name_plural + '"? This will delete all entities in this category.')) return;

      Chronicle.apiFetch('/campaigns/' + self.campaignId + '/entity-types/' + et.id, {
        method: 'DELETE',
        csrfToken: self.csrfToken
      }).then(function (res) {
        if (!res.ok) throw new Error('Failed');
        // Remove from local list.
        self.entityTypes = self.entityTypes.filter(function (e) { return e.id !== et.id; });
        self.renderCategoryList();
        // If we were viewing this category, switch to campaign dashboard.
        if (self.activeContext && self.activeContext.etid == et.id) {
          self.selectContext({ type: CTX_CAMPAIGN_DASH, role: 'default' });
        }
        Chronicle.notify('Category deleted', 'success');
      }).catch(function () {
        Chronicle.notify('Failed to delete category', 'error');
      });
    }
  });
})();
