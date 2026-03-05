/**
 * editor.js -- Chronicle Rich Text Editor Widget
 *
 * TipTap-based rich text editor for entity content. Mounts to elements
 * with data-widget="editor" and provides WYSIWYG editing with autosave.
 *
 * Configuration (via data-* attributes):
 *   data-endpoint    - API URL for loading/saving content (required)
 *   data-campaign-id - Campaign ID for @mention entity search (required for mentions)
 *   data-editable    - "true" to enable editing, "false" for read-only (default: false)
 *   data-autosave    - Autosave interval in seconds, 0 to disable (default: 30)
 *   data-csrf-token  - CSRF token for PUT requests
 *
 * Content is stored as ProseMirror JSON in the entity's `entry` column
 * and pre-rendered to HTML in `entry_html` for display performance.
 *
 * @mention support:
 *   When editor_mention.js is loaded and a campaign ID is available,
 *   typing @ in the editor triggers an entity search popup. Selecting
 *   an entity inserts a styled mention link.
 */
(function () {
  'use strict';

  // Ensure TipTap bundle is loaded.
  if (!window.TipTap) {
    console.error('[Editor] TipTap bundle not loaded. Include tiptap-bundle.min.js before editor.js.');
    return;
  }

  var Editor = TipTap.Editor;
  var StarterKit = TipTap.StarterKit;
  var Placeholder = TipTap.Placeholder;
  var Underline = TipTap.Underline;

  // Use MentionLink (extended Link with entity mention attributes) if
  // available, otherwise fall back to standard Link. MentionLink preserves
  // data-mention-id and data-entity-preview through the ProseMirror JSON
  // round-trip so hover preview cards work after save/reload.
  var Link = (Chronicle && Chronicle.MentionLink) || TipTap.Link;
  var Table = TipTap.Table;
  var TableRow = TipTap.TableRow;
  var TableCell = TipTap.TableCell;
  var TableHeader = TipTap.TableHeader;
  var CodeBlockLowlight = TipTap.CodeBlockLowlight;
  var lowlight = TipTap.lowlight;

  // Store editor instances for cleanup.
  var editors = new WeakMap();

  Chronicle.register('editor', {
    /**
     * Initialize the editor widget on a DOM element.
     *
     * The editor starts in read-only "view" mode by default, even when the
     * user has edit permissions. An "Edit" button lets them enter edit mode
     * which reveals the toolbar and enables typing. This prevents accidental
     * edits and provides a cleaner reading experience.
     *
     * @param {HTMLElement} el - Mount point element.
     * @param {Object} config - Parsed data-* attributes.
     */
    init: function (el, config) {
      var endpoint = config.endpoint;
      var campaignId = config.campaignId || '';
      var canEdit = config.editable === true; // user has permission to edit
      var autosaveInterval = config.autosave || 30;
      var csrfToken = config.csrfToken || '';

      // Create editor container structure.
      el.innerHTML = '';
      el.classList.add('chronicle-editor');

      // Header bar with title and edit/done toggle (visible when user can edit).
      var headerEl = null;
      if (canEdit) {
        headerEl = document.createElement('div');
        headerEl.className = 'chronicle-editor__header';
        el.appendChild(headerEl);
        renderHeader(headerEl, false);
      }

      var toolbar = null;
      var contentEl = document.createElement('div');
      contentEl.className = 'chronicle-editor__content';

      var statusEl = document.createElement('div');
      statusEl.className = 'chronicle-editor__status';
      statusEl.style.display = 'none'; // hidden in view mode

      // Toolbar is created but hidden until edit mode is activated.
      if (canEdit) {
        toolbar = createToolbar();
        toolbar.style.display = 'none'; // hidden in view mode
        el.appendChild(toolbar);
      }
      el.appendChild(contentEl);
      if (canEdit) {
        el.appendChild(statusEl);
      }

      // Configure TipTap extensions. StarterKit is configured to exclude Link
      // and Underline so we can provide our own configured versions without
      // triggering "Duplicate extension names" warnings.
      var extensions = [
        StarterKit.configure({
          heading: { levels: [1, 2, 3] },
          link: false,
          underline: false,
          codeBlock: false, // Replaced by CodeBlockLowlight for syntax highlighting.
        }),
        Placeholder.configure({
          placeholder: 'Begin writing your entry...',
        }),
        Link.configure({
          openOnClick: true, // always clickable in view mode, reconfigured in edit mode
          HTMLAttributes: { class: 'text-accent hover:underline' },
        }),
        Underline,
      ];

      // Add table extensions if available in the bundle.
      if (Table) {
        extensions.push(
          Table.configure({ resizable: true, HTMLAttributes: { class: 'chronicle-table' } }),
          TableRow,
          TableCell,
          TableHeader
        );
      }

      // Add code block with syntax highlighting if lowlight is available.
      if (CodeBlockLowlight && lowlight) {
        extensions.push(
          CodeBlockLowlight.configure({
            lowlight: lowlight,
            defaultLanguage: null, // Auto-detect.
          })
        );
      }

      // Add inline secrets mark if extension is loaded.
      if (Chronicle.SecretMark) {
        extensions.push(Chronicle.SecretMark);
      }

      // Build editor props. When mention extension is available and editor
      // is editable, intercept keydown events to let the mention popup
      // handle arrow keys, Enter, and Escape before ProseMirror processes them.
      var editorProps = {
        attributes: {
          class: 'prose prose-sm max-w-none focus:outline-none min-h-[200px] p-4 text-fg-body',
        },
      };

      // We store a reference to the mention extension here so the keydown
      // handler closure can access it. It will be set after editor creation.
      var mentionExtRef = { current: null };

      if (canEdit && campaignId && Chronicle.MentionExtension) {
        editorProps.handleKeyDown = function (view, event) {
          if (mentionExtRef.current) {
            return mentionExtRef.current.onKeyDown(null, event);
          }
          return false;
        };
      }

      // Create TipTap editor instance -- always starts read-only.
      var editor = new Editor({
        element: contentEl,
        extensions: extensions,
        editable: false, // start in view mode
        content: '<p></p>',
        editorProps: editorProps,
      });

      // --- @Mention Extension ---
      // Initialize mention support if the extension module is loaded and we
      // have a campaign ID to search against. The mention extension hooks into
      // editor events to detect the @ trigger and manage the popup lifecycle.
      var mentionExt = null;
      if (canEdit && campaignId && Chronicle.MentionExtension) {
        mentionExt = Chronicle.MentionExtension({ campaignId: campaignId });
        mentionExt.onCreate(editor);
        // Set the ref so the editorProps.handleKeyDown closure can access it.
        mentionExtRef.current = mentionExt;
      }

      // Track state.
      var state = {
        editor: editor,
        endpoint: endpoint,
        csrfToken: csrfToken,
        autosaveTimer: null,
        dirty: false,
        saving: false,
        statusEl: statusEl,
        toolbar: toolbar,
        headerEl: headerEl,
        mentionExt: mentionExt,
        canEdit: canEdit,
        isEditing: false, // tracks current edit mode state
        el: el,
        autosaveInterval: autosaveInterval,
      };

      editors.set(el, state);

      // Update toolbar active states on selection change.
      if (canEdit && toolbar) {
        editor.on('selectionUpdate', function () {
          updateToolbarState(editor, toolbar);
        });
        editor.on('transaction', function () {
          updateToolbarState(editor, toolbar);
        });
      }

      // Wire mention extension into editor update events so it can
      // detect the @ trigger and update the suggestion popup. Only fires
      // on content changes (update), NOT on cursor movement (selectionUpdate),
      // to prevent the popup from reappearing when clicking in the editor.
      if (mentionExt) {
        editor.on('update', function () {
          mentionExt.onUpdate(editor);
        });
      }

      // Track changes for autosave, highlight save button, and notify global dirty state.
      if (canEdit) {
        editor.on('update', function () {
          if (!state.isEditing) return; // ignore updates during content loading
          state.dirty = true;
          setStatus(statusEl, 'unsaved');
          updateSaveButton(toolbar, true);
          Chronicle.markDirty('editor');
        });
      }

      // Load initial content from API.
      if (endpoint) {
        loadContent(state);
      }
    },

    /**
     * Destroy the editor widget and clean up.
     *
     * @param {HTMLElement} el - Mount point element.
     */
    destroy: function (el) {
      var state = editors.get(el);
      if (!state) return;

      // Save unsaved changes before destroying.
      if (state.dirty && !state.saving) {
        saveContent(state);
      }

      if (state.autosaveTimer) {
        clearInterval(state.autosaveTimer);
      }

      // Clean up mention extension popup and listeners.
      if (state.mentionExt) {
        state.mentionExt.onDestroy();
      }

      // Clean up insert menu global click listener to prevent memory leaks.
      var insertMenu = el.querySelector('.chronicle-editor__insert-wrapper');
      if (insertMenu && insertMenu._closeDropdownHandler) {
        document.removeEventListener('click', insertMenu._closeDropdownHandler);
      }

      if (state.editor) {
        state.editor.destroy();
      }

      Chronicle.markClean('editor');
      editors.delete(el);
    },
  });

  // --- Edit Mode Toggle ---

  /**
   * Render the editor header bar with Edit/Done button.
   * @param {HTMLElement} headerEl - Header container.
   * @param {boolean} isEditing - Whether the editor is in edit mode.
   */
  function renderHeader(headerEl, isEditing) {
    headerEl.innerHTML = '';

    var label = document.createElement('span');
    label.className = 'chronicle-editor__header-label';
    label.textContent = 'Entry';
    headerEl.appendChild(label);

    var btn = document.createElement('button');
    btn.type = 'button';

    if (isEditing) {
      btn.className = 'chronicle-editor__edit-btn chronicle-editor__edit-btn--done';
      btn.innerHTML = '<i class="fa-solid fa-check" style="font-size:11px"></i> Done';
      btn.title = 'Exit edit mode';
    } else {
      btn.className = 'chronicle-editor__edit-btn';
      btn.innerHTML = '<i class="fa-solid fa-pen" style="font-size:11px"></i> Edit';
      btn.title = 'Enter edit mode';
    }

    btn.addEventListener('click', function (e) {
      e.preventDefault();
      var el = headerEl.closest('.chronicle-editor');
      var state = editors.get(el);
      if (!state) return;

      if (state.isEditing) {
        exitEditMode(state);
      } else {
        enterEditMode(state);
      }
    });

    headerEl.appendChild(btn);
  }

  /**
   * Enter edit mode: show toolbar, enable editing, start autosave.
   */
  function enterEditMode(state) {
    state.isEditing = true;
    state.editor.setEditable(true);

    // Show toolbar and status bar.
    if (state.toolbar) {
      state.toolbar.style.display = '';
    }
    if (state.statusEl) {
      state.statusEl.style.display = '';
    }

    // Update header to show "Done" button.
    if (state.headerEl) {
      renderHeader(state.headerEl, true);
    }

    // Add editing visual cue.
    state.el.classList.add('chronicle-editor--editing');

    // Start autosave timer.
    if (state.autosaveInterval > 0) {
      state.autosaveTimer = setInterval(function () {
        if (state.dirty && !state.saving) {
          saveContent(state);
        }
      }, state.autosaveInterval * 1000);
    }

    // Focus the editor.
    state.editor.commands.focus('end');
  }

  /**
   * Exit edit mode: save changes, hide toolbar, make read-only.
   */
  function exitEditMode(state) {
    // Save any unsaved changes first.
    if (state.dirty && !state.saving) {
      saveContent(state);
    }

    state.isEditing = false;
    state.editor.setEditable(false);

    // Hide toolbar and status bar.
    if (state.toolbar) {
      state.toolbar.style.display = 'none';
    }
    if (state.statusEl) {
      state.statusEl.style.display = 'none';
    }

    // Update header to show "Edit" button.
    if (state.headerEl) {
      renderHeader(state.headerEl, false);
    }

    // Remove editing visual cue.
    state.el.classList.remove('chronicle-editor--editing');

    // Stop autosave timer.
    if (state.autosaveTimer) {
      clearInterval(state.autosaveTimer);
      state.autosaveTimer = null;
    }
  }

  // --- Toolbar ---

  /**
   * Create the editor toolbar with formatting buttons.
   * @returns {HTMLElement}
   */
  function createToolbar() {
    var toolbar = document.createElement('div');
    toolbar.className = 'chronicle-editor__toolbar';

    var groups = [
      // Text formatting
      [
        { cmd: 'bold', icon: 'B', title: 'Bold (Ctrl+B)', style: 'font-weight:bold' },
        { cmd: 'italic', icon: 'I', title: 'Italic (Ctrl+I)', style: 'font-style:italic' },
        { cmd: 'underline', icon: 'U', title: 'Underline (Ctrl+U)', style: 'text-decoration:underline' },
        { cmd: 'strike', icon: 'S', title: 'Strikethrough', style: 'text-decoration:line-through' },
        { cmd: 'secret', icon: '<i class="fa-solid fa-eye-slash" style="font-size:11px"></i>', title: 'GM Secret (Ctrl+Shift+S)' },
      ],
      // Block formatting
      [
        { cmd: 'heading1', icon: 'H1', title: 'Heading 1' },
        { cmd: 'heading2', icon: 'H2', title: 'Heading 2' },
        { cmd: 'heading3', icon: 'H3', title: 'Heading 3' },
      ],
      // Lists
      [
        { cmd: 'bulletList', icon: '&#8226;', title: 'Bullet List' },
        { cmd: 'orderedList', icon: '1.', title: 'Numbered List' },
      ],
      // Misc
      [
        { cmd: 'blockquote', icon: '&#8220;', title: 'Quote' },
        { cmd: 'code', icon: '&lt;/&gt;', title: 'Code' },
        { cmd: 'horizontalRule', icon: '&#8212;', title: 'Horizontal Rule' },
      ],
      // Actions
      [
        { cmd: 'undo', icon: '&#x21B6;', title: 'Undo (Ctrl+Z)' },
        { cmd: 'redo', icon: '&#x21B7;', title: 'Redo (Ctrl+Shift+Z)' },
      ],
    ];

    groups.forEach(function (group, i) {
      if (i > 0) {
        var sep = document.createElement('span');
        sep.className = 'chronicle-editor__separator';
        toolbar.appendChild(sep);
      }
      group.forEach(function (btn) {
        var button = document.createElement('button');
        button.type = 'button';
        button.className = 'chronicle-editor__btn';
        button.innerHTML = btn.icon;
        button.title = btn.title;
        button.setAttribute('data-cmd', btn.cmd);
        if (btn.style) button.style.cssText = btn.style;
        toolbar.appendChild(button);
      });
    });

    // Handle toolbar button clicks.
    toolbar.addEventListener('click', function (e) {
      var button = e.target.closest('[data-cmd]');
      if (!button) return;
      e.preventDefault();

      var el = toolbar.closest('.chronicle-editor');
      var state = editors.get(el);
      if (!state || !state.editor) return;

      var cmd = button.getAttribute('data-cmd');
      executeCommand(state.editor, cmd);
    });

    // --- Insert menu (+ button with dropdown for discoverable features) ---
    var insertSep = document.createElement('span');
    insertSep.className = 'chronicle-editor__separator';
    toolbar.appendChild(insertSep);
    toolbar.appendChild(createInsertMenu());

    // Separator before save button.
    var saveSep = document.createElement('span');
    saveSep.className = 'chronicle-editor__separator';
    toolbar.appendChild(saveSep);

    // Save button -- prominent, highlights when there are unsaved changes.
    var saveBtn = document.createElement('button');
    saveBtn.type = 'button';
    saveBtn.className = 'chronicle-editor__btn chronicle-editor__btn--save';
    saveBtn.innerHTML = '&#128190; Save';
    saveBtn.title = 'Save (Ctrl+S)';
    saveBtn.setAttribute('data-cmd', 'save');
    toolbar.appendChild(saveBtn);

    return toolbar;
  }

  /**
   * Create the Insert menu -- a "+" dropdown that surfaces discoverable
   * editor features like @mentions, links, and horizontal rules. Users who
   * know the keyboard shortcuts can type them directly; this menu is for
   * those who don't.
   * @returns {HTMLElement}
   */
  function createInsertMenu() {
    var wrapper = document.createElement('div');
    wrapper.className = 'chronicle-editor__insert-menu';

    // Trigger button.
    var trigger = document.createElement('button');
    trigger.type = 'button';
    trigger.className = 'chronicle-editor__btn chronicle-editor__insert-trigger';
    trigger.innerHTML = '<i class="fa-solid fa-plus" style="font-size:11px"></i>';
    trigger.title = 'Insert...';
    wrapper.appendChild(trigger);

    // Dropdown panel (hidden by default).
    var dropdown = document.createElement('div');
    dropdown.className = 'chronicle-editor__insert-dropdown';
    dropdown.style.display = 'none';

    // Menu items: each has an action key, icon, label, and shortcut hint.
    var items = [
      { action: 'mention',        icon: 'fa-diagram-project', label: 'Link Entity',     hint: 'Type @' },
      { action: 'link',           icon: 'fa-link',            label: 'Insert Link',     hint: '' },
      { action: 'horizontalRule', icon: 'fa-minus',           label: 'Horizontal Rule', hint: '---' },
      { action: 'blockquote',     icon: 'fa-circle-info',     label: 'Callout Block',   hint: '>' },
      { action: 'code',           icon: 'fa-code',            label: 'Code Block',      hint: '```' },
      { action: 'table',          icon: 'fa-table',           label: 'Insert Table',    hint: '' },
    ];

    items.forEach(function (item) {
      var row = document.createElement('button');
      row.type = 'button';
      row.className = 'chronicle-editor__insert-item';
      row.setAttribute('data-insert', item.action);

      row.innerHTML =
        '<i class="fa-solid ' + item.icon + ' chronicle-editor__insert-icon"></i>' +
        '<span class="chronicle-editor__insert-label">' + item.label + '</span>' +
        (item.hint ? '<kbd class="chronicle-editor__insert-hint">' + item.hint + '</kbd>' : '');

      dropdown.appendChild(row);
    });

    wrapper.appendChild(dropdown);

    // Toggle dropdown on trigger click.
    trigger.addEventListener('click', function (e) {
      e.preventDefault();
      e.stopPropagation();
      var isOpen = dropdown.style.display !== 'none';
      dropdown.style.display = isOpen ? 'none' : '';
      trigger.classList.toggle('chronicle-editor__btn--active', !isOpen);
    });

    // Handle item clicks.
    dropdown.addEventListener('click', function (e) {
      var row = e.target.closest('[data-insert]');
      if (!row) return;
      e.preventDefault();
      e.stopPropagation();

      var action = row.getAttribute('data-insert');
      var el = wrapper.closest('.chronicle-editor');
      var state = editors.get(el);
      if (!state || !state.editor) return;

      executeInsert(state, action);

      // Close dropdown.
      dropdown.style.display = 'none';
      trigger.classList.remove('chronicle-editor__btn--active');
    });

    // Close dropdown when clicking outside. Store handler reference on the
    // wrapper element so it can be removed in destroy() to prevent memory leaks
    // when the widget is re-mounted via HTMX swaps.
    wrapper._closeDropdownHandler = function (e) {
      if (!wrapper.contains(e.target)) {
        dropdown.style.display = 'none';
        trigger.classList.remove('chronicle-editor__btn--active');
      }
    };
    document.addEventListener('click', wrapper._closeDropdownHandler);

    return wrapper;
  }

  /**
   * Execute an insert menu action. For mention, inserts @ at cursor to trigger
   * the mention popup. For others, delegates to standard TipTap commands.
   */
  function executeInsert(state, action) {
    var editor = state.editor;

    switch (action) {
      case 'mention':
        // Insert @ character at cursor position to trigger the mention popup.
        // The mention extension watches for @ and opens the search dropdown.
        editor.chain().focus().insertContent('@').run();
        // Manually nudge the mention extension to check for the trigger.
        if (state.mentionExt) {
          state.mentionExt.onUpdate(editor);
        }
        break;

      case 'link':
        // Prompt for URL and insert/update link on current selection.
        var url = prompt('Enter URL:');
        if (url) {
          // If there's a text selection, wrap it as a link. Otherwise insert the URL as text.
          if (editor.state.selection.empty) {
            editor.chain().focus().insertContent(
              '<a href="' + url + '">' + url + '</a>'
            ).run();
          } else {
            editor.chain().focus().setLink({ href: url }).run();
          }
        } else {
          editor.chain().focus().run();
        }
        break;

      case 'horizontalRule':
        editor.chain().focus().setHorizontalRule().run();
        break;

      case 'blockquote':
        editor.chain().focus().toggleBlockquote().run();
        break;

      case 'code':
        editor.chain().focus().toggleCodeBlock().run();
        break;

      case 'table':
        // Insert a 3x3 table with header row.
        if (editor.can().insertTable) {
          editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run();
        }
        break;
    }
  }

  /**
   * Execute a toolbar command on the editor.
   */
  function executeCommand(editor, cmd) {
    var chain = editor.chain().focus();

    switch (cmd) {
      case 'bold': chain.toggleBold().run(); break;
      case 'italic': chain.toggleItalic().run(); break;
      case 'underline': chain.toggleUnderline().run(); break;
      case 'strike': chain.toggleStrike().run(); break;
      case 'secret': if (editor.commands.toggleSecret) editor.commands.toggleSecret(); break;
      case 'heading1': chain.toggleHeading({ level: 1 }).run(); break;
      case 'heading2': chain.toggleHeading({ level: 2 }).run(); break;
      case 'heading3': chain.toggleHeading({ level: 3 }).run(); break;
      case 'bulletList': chain.toggleBulletList().run(); break;
      case 'orderedList': chain.toggleOrderedList().run(); break;
      case 'blockquote': chain.toggleBlockquote().run(); break;
      case 'code': chain.toggleCodeBlock().run(); break;
      case 'horizontalRule': chain.setHorizontalRule().run(); break;
      case 'undo': chain.undo().run(); break;
      case 'redo': chain.redo().run(); break;
      case 'save':
        var el = editor.options.element.closest('.chronicle-editor');
        var state = editors.get(el);
        if (state && state.dirty) saveContent(state);
        break;
    }
  }

  /**
   * Update toolbar button active states based on current editor state.
   */
  function updateToolbarState(editor, toolbar) {
    var buttons = toolbar.querySelectorAll('[data-cmd]');
    buttons.forEach(function (btn) {
      var cmd = btn.getAttribute('data-cmd');
      var active = false;

      switch (cmd) {
        case 'bold': active = editor.isActive('bold'); break;
        case 'italic': active = editor.isActive('italic'); break;
        case 'underline': active = editor.isActive('underline'); break;
        case 'strike': active = editor.isActive('strike'); break;
        case 'secret': active = editor.isActive('secret'); break;
        case 'heading1': active = editor.isActive('heading', { level: 1 }); break;
        case 'heading2': active = editor.isActive('heading', { level: 2 }); break;
        case 'heading3': active = editor.isActive('heading', { level: 3 }); break;
        case 'bulletList': active = editor.isActive('bulletList'); break;
        case 'orderedList': active = editor.isActive('orderedList'); break;
        case 'blockquote': active = editor.isActive('blockquote'); break;
        case 'code': active = editor.isActive('codeBlock'); break;
      }

      btn.classList.toggle('chronicle-editor__btn--active', active);
    });
  }

  // --- API ---

  /**
   * Load content from the API endpoint.
   */
  function loadContent(state) {
    fetch(state.endpoint, {
      method: 'GET',
      headers: { 'Accept': 'application/json' },
      credentials: 'same-origin',
    })
      .then(function (res) {
        if (!res.ok) throw new Error('Failed to load: ' + res.status);
        return res.json();
      })
      .then(function (data) {
        if (data.entry) {
          // entry is ProseMirror JSON stored as a string.
          var content = typeof data.entry === 'string' ? JSON.parse(data.entry) : data.entry;
          state.editor.commands.setContent(content);
        }
        state.dirty = false;
        if (state.editor.isEditable) {
          setStatus(state.statusEl, 'saved');
        }
      })
      .catch(function (err) {
        console.error('[Editor] Load error:', err);
        setStatus(state.statusEl, 'error', 'Failed to load content');
      });
  }

  /**
   * Save content to the API endpoint.
   */
  function saveContent(state) {
    if (state.saving) return;
    state.saving = true;
    setStatus(state.statusEl, 'saving');

    var json = state.editor.getJSON();
    var html = state.editor.getHTML();

    fetch(state.endpoint, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': state.csrfToken,
      },
      credentials: 'same-origin',
      body: JSON.stringify({
        entry: JSON.stringify(json),
        entry_html: html,
      }),
    })
      .then(function (res) {
        if (!res.ok) throw new Error('Save failed: ' + res.status);
        state.dirty = false;
        state.saving = false;
        setStatus(state.statusEl, 'saved');
        updateSaveButton(state.toolbar, false);
        Chronicle.markClean('editor');
      })
      .catch(function (err) {
        console.error('[Editor] Save error:', err);
        state.saving = false;
        setStatus(state.statusEl, 'error', 'Failed to save');
      });
  }

  // --- Status ---

  /**
   * Update the status bar message.
   */
  function setStatus(el, type, message) {
    if (!el) return;

    var text = '';
    var cls = 'chronicle-editor__status';

    switch (type) {
      case 'saved':
        text = 'All changes saved';
        cls += ' chronicle-editor__status--saved';
        break;
      case 'saving':
        text = 'Saving...';
        cls += ' chronicle-editor__status--saving';
        break;
      case 'unsaved':
        text = 'Unsaved changes';
        cls += ' chronicle-editor__status--unsaved';
        break;
      case 'error':
        text = message || 'Error';
        cls += ' chronicle-editor__status--error';
        break;
    }

    el.textContent = text;
    el.className = cls;
  }

  /**
   * Toggle the save button's visual highlight based on unsaved changes.
   */
  function updateSaveButton(toolbar, hasChanges) {
    if (!toolbar) return;
    var saveBtn = toolbar.querySelector('.chronicle-editor__btn--save');
    if (saveBtn) {
      saveBtn.classList.toggle('has-changes', hasChanges);
    }
  }

  // --- Keyboard Shortcuts ---

  // Ctrl+S to save (prevent browser default).
  document.addEventListener('keydown', function (e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      var editorEl = document.querySelector('.chronicle-editor');
      if (editorEl) {
        e.preventDefault();
        var state = editors.get(editorEl);
        if (state && state.dirty) saveContent(state);
      }
    }
  });

  // --- Find/Replace ---

  /**
   * Simple find/replace bar for the editor. Opens with Ctrl+F (find) or
   * Ctrl+H (find and replace). Uses ProseMirror's text search under the hood.
   */
  var findBarState = null;

  function openFindBar(editorEl, showReplace) {
    var state = editors.get(editorEl);
    if (!state || !state.isEditing) return;

    // If already open, just toggle replace visibility.
    if (findBarState && findBarState.bar.parentNode) {
      if (showReplace) {
        findBarState.replaceRow.style.display = '';
      }
      findBarState.findInput.focus();
      findBarState.findInput.select();
      return;
    }

    var bar = document.createElement('div');
    bar.className = 'chronicle-find-bar';

    // Find row.
    var findRow = document.createElement('div');
    findRow.className = 'chronicle-find-bar__row';

    var findInput = document.createElement('input');
    findInput.type = 'text';
    findInput.className = 'chronicle-find-bar__input';
    findInput.placeholder = 'Find...';
    findInput.setAttribute('autocomplete', 'off');

    var countLabel = document.createElement('span');
    countLabel.className = 'chronicle-find-bar__count';
    countLabel.textContent = '';

    var prevBtn = document.createElement('button');
    prevBtn.type = 'button';
    prevBtn.className = 'chronicle-find-bar__btn';
    prevBtn.innerHTML = '<i class="fa-solid fa-chevron-up"></i>';
    prevBtn.title = 'Previous (Shift+Enter)';

    var nextBtn = document.createElement('button');
    nextBtn.type = 'button';
    nextBtn.className = 'chronicle-find-bar__btn';
    nextBtn.innerHTML = '<i class="fa-solid fa-chevron-down"></i>';
    nextBtn.title = 'Next (Enter)';

    var closeBtn = document.createElement('button');
    closeBtn.type = 'button';
    closeBtn.className = 'chronicle-find-bar__btn chronicle-find-bar__close';
    closeBtn.innerHTML = '<i class="fa-solid fa-xmark"></i>';
    closeBtn.title = 'Close (Esc)';

    findRow.appendChild(findInput);
    findRow.appendChild(countLabel);
    findRow.appendChild(prevBtn);
    findRow.appendChild(nextBtn);
    findRow.appendChild(closeBtn);

    // Replace row.
    var replaceRow = document.createElement('div');
    replaceRow.className = 'chronicle-find-bar__row';
    if (!showReplace) replaceRow.style.display = 'none';

    var replaceInput = document.createElement('input');
    replaceInput.type = 'text';
    replaceInput.className = 'chronicle-find-bar__input';
    replaceInput.placeholder = 'Replace...';
    replaceInput.setAttribute('autocomplete', 'off');

    var replaceBtn = document.createElement('button');
    replaceBtn.type = 'button';
    replaceBtn.className = 'chronicle-find-bar__btn';
    replaceBtn.textContent = 'Replace';
    replaceBtn.title = 'Replace current match';

    var replaceAllBtn = document.createElement('button');
    replaceAllBtn.type = 'button';
    replaceAllBtn.className = 'chronicle-find-bar__btn';
    replaceAllBtn.textContent = 'All';
    replaceAllBtn.title = 'Replace all matches';

    replaceRow.appendChild(replaceInput);
    replaceRow.appendChild(replaceBtn);
    replaceRow.appendChild(replaceAllBtn);

    bar.appendChild(findRow);
    bar.appendChild(replaceRow);

    // Insert bar above the content area.
    var contentEl = editorEl.querySelector('.chronicle-editor__content');
    if (contentEl) {
      contentEl.parentNode.insertBefore(bar, contentEl);
    } else {
      editorEl.appendChild(bar);
    }

    // State for this find session.
    findBarState = {
      bar: bar,
      findInput: findInput,
      replaceInput: replaceInput,
      replaceRow: replaceRow,
      countLabel: countLabel,
      matches: [],
      currentIndex: -1,
      editorState: state,
    };

    // Event handlers.
    findInput.addEventListener('input', function () {
      doFind(findInput.value);
    });

    findInput.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') {
        e.preventDefault();
        if (e.shiftKey) { findPrev(); } else { findNext(); }
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        closeFindBar();
      }
    });

    replaceInput.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') {
        e.preventDefault();
        closeFindBar();
      }
    });

    nextBtn.addEventListener('click', findNext);
    prevBtn.addEventListener('click', findPrev);
    closeBtn.addEventListener('click', closeFindBar);
    replaceBtn.addEventListener('click', doReplace);
    replaceAllBtn.addEventListener('click', doReplaceAll);

    findInput.focus();
  }

  function closeFindBar() {
    if (!findBarState) return;
    clearHighlights();
    if (findBarState.bar.parentNode) {
      findBarState.bar.parentNode.removeChild(findBarState.bar);
    }
    if (findBarState.editorState && findBarState.editorState.editor) {
      findBarState.editorState.editor.commands.focus();
    }
    findBarState = null;
  }

  function doFind(query) {
    if (!findBarState) return;
    clearHighlights();
    findBarState.matches = [];
    findBarState.currentIndex = -1;

    if (!query) {
      findBarState.countLabel.textContent = '';
      return;
    }

    var editor = findBarState.editorState.editor;
    var doc = editor.state.doc;
    var queryLower = query.toLowerCase();
    var matches = [];

    // Walk through all text nodes in the ProseMirror document.
    doc.descendants(function (node, pos) {
      if (!node.isText) return;
      var text = node.text.toLowerCase();
      var idx = 0;
      while (true) {
        var found = text.indexOf(queryLower, idx);
        if (found === -1) break;
        matches.push({ from: pos + found, to: pos + found + query.length });
        idx = found + 1;
      }
    });

    findBarState.matches = matches;

    if (matches.length > 0) {
      findBarState.currentIndex = 0;
      highlightMatches();
      scrollToMatch(0);
    }

    findBarState.countLabel.textContent = matches.length > 0
      ? (findBarState.currentIndex + 1) + '/' + matches.length
      : 'No results';
  }

  function findNext() {
    if (!findBarState || findBarState.matches.length === 0) return;
    findBarState.currentIndex = (findBarState.currentIndex + 1) % findBarState.matches.length;
    highlightMatches();
    scrollToMatch(findBarState.currentIndex);
    findBarState.countLabel.textContent =
      (findBarState.currentIndex + 1) + '/' + findBarState.matches.length;
  }

  function findPrev() {
    if (!findBarState || findBarState.matches.length === 0) return;
    findBarState.currentIndex =
      (findBarState.currentIndex - 1 + findBarState.matches.length) % findBarState.matches.length;
    highlightMatches();
    scrollToMatch(findBarState.currentIndex);
    findBarState.countLabel.textContent =
      (findBarState.currentIndex + 1) + '/' + findBarState.matches.length;
  }

  function doReplace() {
    if (!findBarState || findBarState.matches.length === 0 || findBarState.currentIndex < 0) return;
    var match = findBarState.matches[findBarState.currentIndex];
    var replacement = findBarState.replaceInput.value;
    var editor = findBarState.editorState.editor;

    editor.chain().focus()
      .insertContentAt({ from: match.from, to: match.to }, replacement)
      .run();

    // Re-search after replacement.
    doFind(findBarState.findInput.value);
  }

  function doReplaceAll() {
    if (!findBarState || findBarState.matches.length === 0) return;
    var replacement = findBarState.replaceInput.value;
    var editor = findBarState.editorState.editor;
    var matches = findBarState.matches.slice().reverse(); // Replace from end to preserve positions.

    var chain = editor.chain().focus();
    matches.forEach(function (match) {
      chain = chain.insertContentAt({ from: match.from, to: match.to }, replacement);
    });
    chain.run();

    doFind(findBarState.findInput.value);
  }

  function highlightMatches() {
    // Use ProseMirror decorations via CSS class on the editor content.
    // For simplicity, we use the editor's setTextSelection to move to the
    // current match. Full decoration-based highlighting would require a
    // ProseMirror plugin; instead we just select the current match.
    if (!findBarState || findBarState.matches.length === 0) return;
    var match = findBarState.matches[findBarState.currentIndex];
    var editor = findBarState.editorState.editor;
    editor.commands.setTextSelection({ from: match.from, to: match.to });
  }

  function scrollToMatch(index) {
    if (!findBarState || !findBarState.matches[index]) return;
    var editor = findBarState.editorState.editor;
    var view = editor.view;
    var match = findBarState.matches[index];
    var coords = view.coordsAtPos(match.from);
    var editorEl = view.dom.closest('.chronicle-editor__content');
    if (editorEl && coords) {
      var rect = editorEl.getBoundingClientRect();
      if (coords.top < rect.top || coords.bottom > rect.bottom) {
        editorEl.scrollTop += coords.top - rect.top - rect.height / 3;
      }
    }
  }

  function clearHighlights() {
    // No-op: we use selection-based highlighting rather than decorations.
  }

  // Ctrl+F / Ctrl+H to open find/replace.
  document.addEventListener('keydown', function (e) {
    if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.key === 'h')) {
      var editorEl = document.querySelector('.chronicle-editor--editing');
      if (editorEl) {
        e.preventDefault();
        openFindBar(editorEl, e.key === 'h');
      }
    }
  });
})();
