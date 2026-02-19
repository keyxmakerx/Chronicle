/**
 * editor.js -- Chronicle Rich Text Editor Widget
 *
 * TipTap-based rich text editor for entity content. Mounts to elements
 * with data-widget="editor" and provides WYSIWYG editing with autosave.
 *
 * Configuration (via data-* attributes):
 *   data-endpoint   - API URL for loading/saving content (required)
 *   data-editable   - "true" to enable editing, "false" for read-only (default: false)
 *   data-autosave   - Autosave interval in seconds, 0 to disable (default: 30)
 *   data-csrf-token - CSRF token for PUT requests
 *
 * Content is stored as ProseMirror JSON in the entity's `entry` column
 * and pre-rendered to HTML in `entry_html` for display performance.
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
  var Link = TipTap.Link;
  var Underline = TipTap.Underline;

  // Store editor instances for cleanup.
  var editors = new WeakMap();

  Chronicle.register('editor', {
    /**
     * Initialize the editor widget on a DOM element.
     *
     * @param {HTMLElement} el - Mount point element.
     * @param {Object} config - Parsed data-* attributes.
     */
    init: function (el, config) {
      var endpoint = config.endpoint;
      var editable = config.editable === true;
      var autosaveInterval = config.autosave || 30;
      var csrfToken = config.csrfToken || '';

      // Create editor container structure.
      el.innerHTML = '';
      el.classList.add('chronicle-editor');

      var toolbar = null;
      var contentEl = document.createElement('div');
      contentEl.className = 'chronicle-editor__content';

      var statusEl = document.createElement('div');
      statusEl.className = 'chronicle-editor__status';

      if (editable) {
        toolbar = createToolbar();
        el.appendChild(toolbar);
      }
      el.appendChild(contentEl);
      if (editable) {
        el.appendChild(statusEl);
      }

      // Configure TipTap extensions.
      var extensions = [
        StarterKit.configure({
          heading: { levels: [1, 2, 3] },
        }),
        Placeholder.configure({
          placeholder: 'Begin writing your entry...',
        }),
        Link.configure({
          openOnClick: !editable,
          HTMLAttributes: { class: 'text-accent hover:underline' },
        }),
        Underline,
      ];

      // Create TipTap editor instance.
      var editor = new Editor({
        element: contentEl,
        extensions: extensions,
        editable: editable,
        content: '<p></p>',
        editorProps: {
          attributes: {
            class: 'prose prose-sm max-w-none focus:outline-none min-h-[200px] p-4',
          },
        },
      });

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
      };

      editors.set(el, state);

      // Update toolbar active states on selection change.
      if (editable && toolbar) {
        editor.on('selectionUpdate', function () {
          updateToolbarState(editor, toolbar);
        });
        editor.on('transaction', function () {
          updateToolbarState(editor, toolbar);
        });
      }

      // Track changes for autosave and highlight the save button.
      if (editable) {
        editor.on('update', function () {
          state.dirty = true;
          setStatus(statusEl, 'unsaved');
          updateSaveButton(toolbar, true);
        });

        // Set up autosave interval.
        if (autosaveInterval > 0) {
          state.autosaveTimer = setInterval(function () {
            if (state.dirty && !state.saving) {
              saveContent(state);
            }
          }, autosaveInterval * 1000);
        }
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

      if (state.editor) {
        state.editor.destroy();
      }

      editors.delete(el);
    },
  });

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
   * Execute a toolbar command on the editor.
   */
  function executeCommand(editor, cmd) {
    var chain = editor.chain().focus();

    switch (cmd) {
      case 'bold': chain.toggleBold().run(); break;
      case 'italic': chain.toggleItalic().run(); break;
      case 'underline': chain.toggleUnderline().run(); break;
      case 'strike': chain.toggleStrike().run(); break;
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
})();
