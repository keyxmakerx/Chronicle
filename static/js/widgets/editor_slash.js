/**
 * editor_slash.js -- TipTap Slash Command Menu
 *
 * Typing "/" at line start or after whitespace opens a floating dropdown
 * of block commands (headings, lists, quote, table, rule, code block);
 * typing filters, arrows navigate, Enter executes, Escape dismisses.
 *
 * Self-contained module exporting Chronicle.SlashCommands; editor.js
 * detects it and wires its lifecycle hooks into the TipTap editor
 * (same pattern as editor_mention.js).
 */
(function () {
  'use strict';

  window.Chronicle = window.Chronicle || {};

  // --- Slash Command Definitions ---

  var COMMANDS = [
    { id: 'heading1',       label: 'Heading 1',       icon: 'fa-heading',        keywords: 'heading h1 title',      description: 'Large heading' },
    { id: 'heading2',       label: 'Heading 2',       icon: 'fa-heading',        keywords: 'heading h2 subtitle',   description: 'Medium heading' },
    { id: 'heading3',       label: 'Heading 3',       icon: 'fa-heading',        keywords: 'heading h3',            description: 'Small heading' },
    { id: 'bulletList',     label: 'Bullet List',      icon: 'fa-list-ul',        keywords: 'bullet unordered list', description: 'Unordered list' },
    { id: 'orderedList',    label: 'Numbered List',    icon: 'fa-list-ol',        keywords: 'numbered ordered list', description: 'Ordered list' },
    { id: 'blockquote',     label: 'Callout',          icon: 'fa-quote-left',     keywords: 'callout quote block',   description: 'Callout block' },
    { id: 'table',          label: 'Table',            icon: 'fa-table',          keywords: 'table grid',            description: '3×3 table' },
    { id: 'horizontalRule', label: 'Horizontal Rule',  icon: 'fa-minus',          keywords: 'horizontal rule divider line separator', description: 'Divider line' },
    { id: 'codeBlock',      label: 'Code Block',       icon: 'fa-code',           keywords: 'code block snippet',    description: 'Code with syntax highlighting' },
  ];

  // --- Slash Popup UI ---

  /**
   * SlashPopup manages the floating dropdown for slash commands.
   * Handles positioning, filtering, keyboard navigation, and selection.
   *
   * @param {Object} options
   * @param {Function} options.onSelect - Callback when a command is selected.
   * @param {Function} options.onClose - Callback when the popup is dismissed.
   */
  function SlashPopup(options) {
    this.onSelect = options.onSelect;
    this.onClose = options.onClose;
    this.el = null;
    this.items = [];
    this.filteredItems = [];
    this.selectedIndex = 0;
    this.visible = false;
  }

  /**
   * Show the popup near the cursor position.
   * @param {Object} coords - {left, top, bottom} from cursor.
   */
  SlashPopup.prototype.show = function (coords) {
    if (!this.el) {
      this._createEl();
    }
    this.visible = true;
    this.selectedIndex = 0;
    this.filteredItems = COMMANDS.slice();

    this.el.style.display = 'block';
    this._at = coords;
    this._render();

    var self = this;
    setTimeout(function () {
      document.addEventListener('mousedown', self._onClickOutside);
    }, 0);
  };

  /** Hide and reset the popup. */
  SlashPopup.prototype.hide = function () {
    if (this.el) {
      this.el.style.display = 'none';
    }
    this.visible = false;
    this.filteredItems = [];
    this.selectedIndex = 0;
    document.removeEventListener('mousedown', this._onClickOutside);
  };

  /** Destroy the popup element. */
  SlashPopup.prototype.destroy = function () {
    this.hide();
    if (this.el && this.el.parentNode) {
      this.el.parentNode.removeChild(this.el);
    }
    this.el = null;
  };

  /**
   * Filter commands based on the query typed after "/".
   * @param {string} query - Text after the slash character.
   */
  SlashPopup.prototype.updateQuery = function (query) {
    var q = query.toLowerCase().trim();
    if (!q) {
      this.filteredItems = COMMANDS.slice();
    } else {
      this.filteredItems = COMMANDS.filter(function (cmd) {
        return cmd.label.toLowerCase().indexOf(q) !== -1 ||
               cmd.keywords.toLowerCase().indexOf(q) !== -1;
      });
    }
    this.selectedIndex = 0;

    if (this.filteredItems.length === 0) {
      this.onClose();
      return;
    }
    this._render();
  };

  /**
   * Handle keyboard navigation within the popup.
   * @param {string} key - Key name.
   * @returns {boolean} True if handled.
   */
  SlashPopup.prototype.handleKey = function (key) {
    if (!this.visible) return false;

    switch (key) {
      case 'ArrowDown':
        this.selectedIndex = Math.min(this.selectedIndex + 1, this.filteredItems.length - 1);
        this._updateSelection();
        return true;

      case 'ArrowUp':
        this.selectedIndex = Math.max(this.selectedIndex - 1, 0);
        this._updateSelection();
        return true;

      case 'Enter':
        if (this.filteredItems.length > 0 && this.selectedIndex >= 0) {
          this.onSelect(this.filteredItems[this.selectedIndex]);
          return true;
        }
        return false;

      case 'Escape':
        this.onClose();
        return true;

      case 'Tab':
        if (this.filteredItems.length > 0 && this.selectedIndex >= 0) {
          this.onSelect(this.filteredItems[this.selectedIndex]);
          return true;
        }
        return false;

      default:
        return false;
    }
  };

  // --- Private ---

  SlashPopup.prototype._createEl = function () {
    this.el = document.createElement('div');
    this.el.className = 'slash-command-popup';
    this.el.style.cssText =
      'position:fixed;z-index:9999;display:none;min-width:240px;max-width:320px;' +
      'max-height:300px;overflow-y:auto;border-radius:8px;' +
      'box-shadow:0 4px 16px rgba(0,0,0,0.15);';

    this._applyTheme();
    document.body.appendChild(this.el);

    var self = this;
    this._onClickOutside = function (e) {
      if (self.el && !self.el.contains(e.target)) {
        self.onClose();
      }
    };
  };

  SlashPopup.prototype._applyTheme = function () {
    if (!this.el) return;
    var isDark = document.documentElement.classList.contains('dark');
    if (isDark) {
      this.el.style.backgroundColor = '#1f2937';
      this.el.style.border = '1px solid #374151';
      this.el.style.color = '#e5e7eb';
    } else {
      this.el.style.backgroundColor = '#ffffff';
      this.el.style.border = '1px solid #e5e7eb';
      this.el.style.color = '#111827';
    }
  };

  SlashPopup.prototype._render = function () {
    if (!this.el) return;
    this._applyTheme();

    var html = '<div style="padding:4px 8px 2px;font-size:11px;font-weight:600;opacity:0.5;text-transform:uppercase;letter-spacing:0.5px">Commands</div>';
    var isDark = document.documentElement.classList.contains('dark');

    for (var i = 0; i < this.filteredItems.length; i++) {
      var cmd = this.filteredItems[i];
      var isSelected = (i === this.selectedIndex);
      var bg = isSelected ? (isDark ? 'background-color:#374151;' : 'background-color:#f3f4f6;') : '';
      // Heading level suffix shown as a badge.
      var badge = '';
      if (cmd.id === 'heading1') badge = '<span style="font-size:10px;opacity:0.5;margin-left:4px">H1</span>';
      if (cmd.id === 'heading2') badge = '<span style="font-size:10px;opacity:0.5;margin-left:4px">H2</span>';
      if (cmd.id === 'heading3') badge = '<span style="font-size:10px;opacity:0.5;margin-left:4px">H3</span>';

      html +=
        '<div class="slash-popup__item" data-index="' + i + '" ' +
        'style="display:flex;align-items:center;padding:8px 12px;cursor:pointer;' +
        'transition:background-color 0.1s;border-radius:4px;margin:2px 4px;' + bg + '">' +
        '<span style="display:flex;align-items:center;justify-content:center;width:28px;height:28px;' +
        'border-radius:6px;margin-right:10px;flex-shrink:0;font-size:13px;' +
        (isDark ? 'background:#374151;color:#9ca3af' : 'background:#f3f4f6;color:#6b7280') + '">' +
        '<i class="fa-solid ' + cmd.icon + '"></i>' +
        '</span>' +
        '<div style="min-width:0;flex:1">' +
        '<div style="font-weight:500;font-size:13px">' + Chronicle.escapeHtml(cmd.label) + badge + '</div>' +
        '<div style="font-size:11px;opacity:0.5">' + Chronicle.escapeHtml(cmd.description) + '</div>' +
        '</div>' +
        '</div>';
    }

    this.el.innerHTML = html;
    this.place();

    // Bind click/hover events.
    var self = this;
    var itemEls = this.el.querySelectorAll('.slash-popup__item');
    for (var j = 0; j < itemEls.length; j++) {
      (function (idx) {
        itemEls[idx].addEventListener('mousedown', function (e) {
          e.preventDefault();
          e.stopPropagation();
          self.selectedIndex = idx;
          self.onSelect(self.filteredItems[idx]);
        });
        itemEls[idx].addEventListener('mouseenter', function () {
          self.selectedIndex = idx;
          self._updateSelection();
        });
      })(j);
    }
  };

  /** Keep the menu beside the caret and inside the window. */
  SlashPopup.prototype.place = function (coords) {
    if (coords) this._at = coords;
    if (this.el && this._at) Chronicle.placeAtCaret(this.el, this._at);
  };

  SlashPopup.prototype._updateSelection = function () {
    if (!this.el) return;
    var isDark = document.documentElement.classList.contains('dark');
    var items = this.el.querySelectorAll('.slash-popup__item');
    for (var i = 0; i < items.length; i++) {
      if (i === this.selectedIndex) {
        items[i].style.backgroundColor = isDark ? '#374151' : '#f3f4f6';
        items[i].scrollIntoView({ block: 'nearest' });
      } else {
        items[i].style.backgroundColor = '';
      }
    }
  };

  // --- Slash Command Extension for TipTap ---

  /**
   * Creates a slash command extension configuration for the TipTap editor.
   * Follows the same lifecycle pattern as the mention extension.
   *
   * @returns {Object} Extension config with onCreate, onUpdate, onKeyDown, onDestroy.
   */
  function createSlashExtension() {
    var popup = null;
    var slashActive = false;
    var slashStartPos = null;
    var editorInstance = null;

    /**
     * Execute a slash command on the editor.
     * Deletes the /query text and applies the block command.
     *
     * @param {Object} cmd - Command definition from COMMANDS array.
     */
    function executeCommand(cmd) {
      if (!editorInstance || slashStartPos === null) return;

      var editor = editorInstance;
      var from = slashStartPos;
      var to = editor.state.selection.from;

      // Delete the /query text first.
      editor.chain().focus().deleteRange({ from: from, to: to }).run();

      // Custom handler for dynamically added commands (e.g., Insert Template).
      if (cmd.customHandler) {
        closeSlash();
        cmd.customHandler(editor);
        return;
      }

      // Apply the block command.
      switch (cmd.id) {
        case 'heading1':
          editor.chain().focus().toggleHeading({ level: 1 }).run();
          break;
        case 'heading2':
          editor.chain().focus().toggleHeading({ level: 2 }).run();
          break;
        case 'heading3':
          editor.chain().focus().toggleHeading({ level: 3 }).run();
          break;
        case 'bulletList':
          editor.chain().focus().toggleBulletList().run();
          break;
        case 'orderedList':
          editor.chain().focus().toggleOrderedList().run();
          break;
        case 'blockquote':
          editor.chain().focus().toggleBlockquote().run();
          break;
        case 'table':
          if (editor.can().insertTable) {
            editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run();
          }
          break;
        case 'horizontalRule':
          editor.chain().focus().setHorizontalRule().run();
          break;
        case 'codeBlock':
          editor.chain().focus().toggleCodeBlock().run();
          break;
      }

      closeSlash();
    }

    /** Close the slash popup and reset state. */
    function closeSlash() {
      slashActive = false;
      slashStartPos = null;
      if (popup) {
        popup.hide();
      }
    }

    /**
     * Get cursor coordinates for popup positioning.
     * @param {Object} view - ProseMirror view.
     * @param {number} pos - Document position.
     * @returns {Object} {left, top, bottom}
     */
    function getCursorCoords(view, pos) {
      try {
        return view.coordsAtPos(pos);
      } catch (e) {
        var rect = view.dom.getBoundingClientRect();
        return { left: rect.left, top: rect.top + 20, bottom: rect.top + 40 };
      }
    }

    return {
      onCreate: function (editor) {
        editorInstance = editor;
        popup = new SlashPopup({
          onSelect: executeCommand,
          onClose: closeSlash,
        });
      },

      onUpdate: function (editor) {
        if (!editor.isEditable) return;
        editorInstance = editor;

        var state = editor.state;
        var from = state.selection.from;

        // Get text before cursor in the current text block.
        var textBefore = '';
        var $pos = state.doc.resolve(from);
        var textNode = $pos.parent;

        if (textNode && textNode.isTextblock) {
          var startOfBlock = $pos.start();
          textBefore = state.doc.textBetween(startOfBlock, from, '\0', '\0');
        }

        // Match "/" preceded by nothing (start of block) or whitespace.
        // The slash must be followed by optional filter text (no spaces after
        // the command name since these are single-word filters).
        var slashMatch = textBefore.match(/(^|[\s\0])\/([\w]*)$/);

        if (slashMatch) {
          var query = slashMatch[2];
          var slashOffset = textBefore.length - slashMatch[0].length + slashMatch[1].length;
          var startPos = $pos.start() + slashOffset;

          if (!slashActive) {
            slashActive = true;
            slashStartPos = startPos;

            var coords = getCursorCoords(editor.view, from);
            popup.show(coords);
          }

          // Update popup position.
          var updatedCoords = getCursorCoords(editor.view, from);
          popup.place(updatedCoords);

          popup.updateQuery(query);
        } else if (slashActive) {
          closeSlash();
        }
      },

      onKeyDown: function (editor, event) {
        if (!slashActive || !popup || !popup.visible) return false;
        return popup.handleKey(event.key);
      },

      onDestroy: function () {
        if (popup) {
          popup.destroy();
          popup = null;
        }
        slashActive = false;
        slashStartPos = null;
        editorInstance = null;
      },
    };
  }

  // --- Public API ---

  /**
   * Chronicle.SlashCommands - Factory that creates a slash command extension
   * for the TipTap editor. The editor.js widget detects this and wires it
   * into the editor lifecycle alongside the mention extension.
   */
  Chronicle.SlashCommands = createSlashExtension;

  /**
   * Add a custom command to the slash menu. Commands added this way appear
   * after the built-in commands. Commands with a `customHandler` function
   * bypass the default executeCommand switch and call the handler directly.
   *
   * @param {Object} cmd - Command with id, label, icon, keywords, description.
   *   Optional: customHandler(editor) called instead of built-in switch.
   */
  Chronicle.SlashCommands.addCommand = function (cmd) {
    if (!cmd || !cmd.id) return;
    // Avoid duplicates.
    for (var i = 0; i < COMMANDS.length; i++) {
      if (COMMANDS[i].id === cmd.id) return;
    }
    COMMANDS.push(cmd);
  };
})();
