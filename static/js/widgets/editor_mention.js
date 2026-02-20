/**
 * editor_mention.js -- Chronicle @Mention Extension for TipTap
 *
 * Provides inline @mention functionality for the rich text editor.
 * When users type `@` followed by text, a dropdown appears showing matching
 * entities from the campaign. Selecting an entity inserts a styled mention
 * node that renders as a link to the entity's page.
 *
 * Architecture:
 *   - Self-contained module that exports a TipTap Node extension via
 *     window.Chronicle.MentionExtension.
 *   - Searches entities via GET /campaigns/:id/entities/search?q=... with
 *     Accept: application/json to receive JSON results.
 *   - Renders mention nodes as <a> links with data-mention-id attributes.
 *   - Gracefully degrades: API failures close the dropdown, deleted entities
 *     render as plain text.
 *
 * Integration:
 *   The editor.js widget reads Chronicle.MentionExtension and includes it
 *   in the TipTap extensions array when available.
 */
(function () {
  'use strict';

  // Ensure the Chronicle namespace exists.
  window.Chronicle = window.Chronicle || {};

  // Bail if TipTap is not loaded.
  if (!window.TipTap) {
    console.error('[Mention] TipTap bundle not loaded.');
    return;
  }

  // --- Mention Node Extension ---

  /**
   * Creates the TipTap Mention Node extension. This is a custom inline node
   * that stores entity ID, name, and URL as attributes, and renders as an
   * anchor tag in the editor.
   */
  function createMentionNode() {
    // Access ProseMirror Node type creator from the TipTap bundle internals.
    // TipTap's Node.create is available on the Editor's extensionManager,
    // but we can define a custom node using the Plugin approach from the
    // TipTap bundle's exported primitives.
    //
    // Since we only have StarterKit/Editor/etc. exported, we create the
    // mention node via the Editor extension API using inputRules and
    // a ProseMirror plugin for the suggestion popup.

    return {
      name: 'mention',
      group: 'inline',
      inline: true,
      selectable: false,
      atom: true,

      addAttributes: function () {
        return {
          id: { default: null },
          name: { default: null },
          url: { default: null },
        };
      },

      parseHTML: function () {
        return [
          {
            tag: 'a[data-mention-id]',
            getAttrs: function (dom) {
              return {
                id: dom.getAttribute('data-mention-id'),
                name: dom.textContent,
                url: dom.getAttribute('href'),
              };
            },
          },
        ];
      },

      renderHTML: function (props) {
        var node = props.node;
        var href = node.attrs.url || '#';
        // Derive preview URL from entity URL for tooltip support.
        var previewURL = href !== '#' ? href + '/preview' : '';
        var attrs = {
            'data-mention-id': node.attrs.id,
            'href': href,
            'class': 'mention-link text-accent font-medium hover:underline cursor-pointer',
            'contenteditable': 'false',
        };
        if (previewURL) {
          attrs['data-entity-preview'] = previewURL;
        }
        return ['a', attrs, '@' + (node.attrs.name || 'unknown')];
      },
    };
  }

  // --- Suggestion Popup Controller ---

  /**
   * MentionPopup manages the dropdown UI for entity search suggestions.
   * It handles positioning, keyboard navigation, and mouse interaction.
   *
   * @param {Object} options
   * @param {string} options.campaignId - Campaign ID for search API calls.
   * @param {Function} options.onSelect - Callback when an entity is selected.
   * @param {Function} options.onClose - Callback when the popup is dismissed.
   */
  function MentionPopup(options) {
    this.campaignId = options.campaignId;
    this.onSelect = options.onSelect;
    this.onClose = options.onClose;
    this.el = null;
    this.items = [];
    this.selectedIndex = 0;
    this.query = '';
    this.visible = false;
    this.abortController = null;
    this.debounceTimer = null;

    // Debounce delay in ms for search API calls.
    this.DEBOUNCE_MS = 200;
    // Minimum query length to trigger search (server requires >=2).
    this.MIN_QUERY_LEN = 2;
  }

  /**
   * Show the popup near the cursor position.
   *
   * @param {Object} coords - {left, top, bottom} from the editor's cursor position.
   */
  MentionPopup.prototype.show = function (coords) {
    if (!this.el) {
      this._createEl();
    }
    this.visible = true;
    this.selectedIndex = 0;

    // Position the popup below the cursor.
    this.el.style.left = coords.left + 'px';
    this.el.style.top = coords.bottom + 4 + 'px';
    this.el.style.display = 'block';

    // Add click-outside listener.
    var self = this;
    setTimeout(function () {
      document.addEventListener('mousedown', self._onClickOutside);
    }, 0);
  };

  /**
   * Hide and reset the popup.
   */
  MentionPopup.prototype.hide = function () {
    if (this.el) {
      this.el.style.display = 'none';
    }
    this.visible = false;
    this.items = [];
    this.query = '';
    this.selectedIndex = 0;

    // Cancel any pending search request.
    if (this.abortController) {
      this.abortController.abort();
      this.abortController = null;
    }
    if (this.debounceTimer) {
      clearTimeout(this.debounceTimer);
      this.debounceTimer = null;
    }

    document.removeEventListener('mousedown', this._onClickOutside);
  };

  /**
   * Destroy the popup element and clean up all listeners.
   */
  MentionPopup.prototype.destroy = function () {
    this.hide();
    if (this.el && this.el.parentNode) {
      this.el.parentNode.removeChild(this.el);
    }
    this.el = null;
  };

  /**
   * Update the search query and fetch results.
   *
   * @param {string} query - The text typed after @.
   */
  MentionPopup.prototype.updateQuery = function (query) {
    this.query = query;
    this.selectedIndex = 0;

    // Clear previous debounce.
    if (this.debounceTimer) {
      clearTimeout(this.debounceTimer);
    }

    // Show loading state if query is long enough.
    if (query.length < this.MIN_QUERY_LEN) {
      this._renderItems([]);
      this._renderHint(query.length === 0 ? 'Type to search entities...' : 'Keep typing...');
      return;
    }

    var self = this;
    this.debounceTimer = setTimeout(function () {
      self._fetchResults(query);
    }, this.DEBOUNCE_MS);
  };

  /**
   * Handle keyboard events for navigation within the popup.
   *
   * @param {string} key - The key pressed (ArrowUp, ArrowDown, Enter, Escape).
   * @returns {boolean} True if the key was handled.
   */
  MentionPopup.prototype.handleKey = function (key) {
    if (!this.visible) return false;

    switch (key) {
      case 'ArrowDown':
        this.selectedIndex = Math.min(this.selectedIndex + 1, this.items.length - 1);
        this._updateSelection();
        return true;

      case 'ArrowUp':
        this.selectedIndex = Math.max(this.selectedIndex - 1, 0);
        this._updateSelection();
        return true;

      case 'Enter':
        if (this.items.length > 0 && this.selectedIndex >= 0) {
          this.onSelect(this.items[this.selectedIndex]);
          return true;
        }
        return false;

      case 'Escape':
        this.onClose();
        return true;

      default:
        return false;
    }
  };

  // --- Private Methods ---

  /**
   * Create the popup DOM element.
   */
  MentionPopup.prototype._createEl = function () {
    this.el = document.createElement('div');
    this.el.className = 'mention-popup';
    this.el.style.cssText =
      'position:fixed;z-index:9999;display:none;min-width:220px;max-width:320px;' +
      'max-height:240px;overflow-y:auto;border-radius:8px;' +
      'box-shadow:0 4px 16px rgba(0,0,0,0.12);';

    // Apply theme-aware colors.
    this._applyTheme();

    document.body.appendChild(this.el);

    // Bind the click-outside handler with reference for removal.
    var self = this;
    this._onClickOutside = function (e) {
      if (self.el && !self.el.contains(e.target)) {
        self.onClose();
      }
    };
  };

  /**
   * Apply theme-aware styling to the popup element.
   */
  MentionPopup.prototype._applyTheme = function () {
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

  /**
   * Fetch search results from the entity search API.
   *
   * @param {string} query - Search query string.
   */
  MentionPopup.prototype._fetchResults = function (query) {
    // Cancel previous in-flight request.
    if (this.abortController) {
      this.abortController.abort();
    }

    // Use AbortController for clean cancellation.
    this.abortController = new AbortController();

    var self = this;
    var url =
      '/campaigns/' +
      encodeURIComponent(this.campaignId) +
      '/entities/search?q=' +
      encodeURIComponent(query);

    fetch(url, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      credentials: 'same-origin',
      signal: this.abortController.signal,
    })
      .then(function (res) {
        if (!res.ok) throw new Error('Search failed: ' + res.status);
        return res.json();
      })
      .then(function (data) {
        self.abortController = null;
        var results = data.results || [];
        self.items = results;
        self.selectedIndex = 0;
        self._renderItems(results);
      })
      .catch(function (err) {
        // Ignore abort errors (expected when query changes rapidly).
        if (err.name === 'AbortError') return;
        console.warn('[Mention] Search error:', err);
        self.abortController = null;
        self.items = [];
        self._renderHint('Search unavailable');
      });
  };

  /**
   * Render the list of search result items in the popup.
   *
   * @param {Array} items - Array of entity objects from the search API.
   */
  MentionPopup.prototype._renderItems = function (items) {
    if (!this.el) return;
    this._applyTheme();

    if (items.length === 0 && this.query.length >= this.MIN_QUERY_LEN) {
      this._renderHint('No entities found');
      return;
    }

    if (items.length === 0) {
      return;
    }

    var html = '';
    var isDark = document.documentElement.classList.contains('dark');

    for (var i = 0; i < items.length; i++) {
      var item = items[i];
      var selectedClass = i === this.selectedIndex ? ' mention-popup__item--selected' : '';
      var bgSelected = i === this.selectedIndex
        ? (isDark ? 'background-color:#374151;' : 'background-color:#f3f4f6;')
        : '';

      html +=
        '<div class="mention-popup__item' + selectedClass + '" ' +
        'data-index="' + i + '" ' +
        'style="display:flex;align-items:center;padding:8px 12px;cursor:pointer;' +
        'transition:background-color 0.1s;' + bgSelected + '">' +
        '<span style="display:inline-block;width:8px;height:8px;border-radius:50%;' +
        'margin-right:10px;flex-shrink:0;background-color:' +
        (item.type_color || '#6b7280') + '"></span>' +
        '<div style="min-width:0;flex:1">' +
        '<div style="font-weight:500;font-size:14px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">' +
        escapeHTML(item.name) + '</div>' +
        '<div style="font-size:12px;opacity:0.6">' +
        escapeHTML(item.type_name || '') + '</div>' +
        '</div>' +
        '</div>';
    }

    this.el.innerHTML = html;

    // Bind click events on items.
    var self = this;
    var itemEls = this.el.querySelectorAll('.mention-popup__item');
    for (var j = 0; j < itemEls.length; j++) {
      (function (idx) {
        itemEls[idx].addEventListener('mousedown', function (e) {
          e.preventDefault(); // Prevent editor blur.
          e.stopPropagation();
          self.selectedIndex = idx;
          self.onSelect(self.items[idx]);
        });
        itemEls[idx].addEventListener('mouseenter', function () {
          self.selectedIndex = idx;
          self._updateSelection();
        });
      })(j);
    }
  };

  /**
   * Render a hint message in the popup (e.g., "No results" or "Loading...").
   *
   * @param {string} message - The hint text to display.
   */
  MentionPopup.prototype._renderHint = function (message) {
    if (!this.el) return;
    this._applyTheme();
    this.el.innerHTML =
      '<div style="padding:10px 14px;font-size:13px;opacity:0.5">' +
      escapeHTML(message) +
      '</div>';
  };

  /**
   * Update the visual selection highlight without re-rendering all items.
   */
  MentionPopup.prototype._updateSelection = function () {
    if (!this.el) return;
    var isDark = document.documentElement.classList.contains('dark');
    var items = this.el.querySelectorAll('.mention-popup__item');
    for (var i = 0; i < items.length; i++) {
      if (i === this.selectedIndex) {
        items[i].classList.add('mention-popup__item--selected');
        items[i].style.backgroundColor = isDark ? '#374151' : '#f3f4f6';
        // Scroll selected item into view if needed.
        items[i].scrollIntoView({ block: 'nearest' });
      } else {
        items[i].classList.remove('mention-popup__item--selected');
        items[i].style.backgroundColor = '';
      }
    }
  };

  // --- Mention Plugin for TipTap ---

  /**
   * Creates a ProseMirror plugin that intercepts keystrokes to detect the
   * @mention trigger character and manages the suggestion popup lifecycle.
   *
   * This approach uses the TipTap Editor's event hooks instead of a raw
   * ProseMirror plugin, since the exported bundle doesn't expose the Plugin
   * constructor directly.
   *
   * @param {Object} options
   * @param {string} options.campaignId - Campaign ID for search API.
   * @returns {Object} Extension configuration object for TipTap.
   */
  function createMentionExtensionConfig(options) {
    var popup = null;
    var mentionActive = false;
    var mentionStartPos = null;
    var editorInstance = null;

    /**
     * Insert a mention node at the current @ trigger position.
     *
     * @param {Object} entity - The selected entity from search results.
     */
    function insertMention(entity) {
      if (!editorInstance || mentionStartPos === null) return;

      var editor = editorInstance;
      var from = mentionStartPos;
      var to = editor.state.selection.from;

      // Create the mention node content as HTML and insert it.
      // Include data-entity-preview for hover tooltip support.
      var previewAttr = entity.url ? ' data-entity-preview="' + escapeAttr(entity.url + '/preview') + '"' : '';
      var mentionHTML =
        '<a data-mention-id="' + escapeAttr(entity.id) + '" ' +
        'href="' + escapeAttr(entity.url) + '"' + previewAttr + ' ' +
        'class="mention-link text-accent font-medium hover:underline cursor-pointer" ' +
        'contenteditable="false">@' + escapeHTML(entity.name) + '</a>';

      // Delete the @query text and insert the mention link.
      editor
        .chain()
        .focus()
        .deleteRange({ from: from, to: to })
        .insertContent(mentionHTML + '&nbsp;')
        .run();

      closeMention();
    }

    /**
     * Close the mention popup and reset state.
     */
    function closeMention() {
      mentionActive = false;
      mentionStartPos = null;
      if (popup) {
        popup.hide();
      }
    }

    /**
     * Get the cursor coordinates from the editor view for popup positioning.
     *
     * @param {Object} view - ProseMirror editor view.
     * @param {number} pos - Document position.
     * @returns {Object} {left, top, bottom} coordinates.
     */
    function getCursorCoords(view, pos) {
      try {
        var coords = view.coordsAtPos(pos);
        return coords;
      } catch (e) {
        // Fallback: use the view's bounding rect.
        var rect = view.dom.getBoundingClientRect();
        return { left: rect.left, top: rect.top + 20, bottom: rect.top + 40 };
      }
    }

    return {
      campaignId: options.campaignId,

      /**
       * Called when the editor is created. Sets up event listeners for
       * detecting the @ trigger and managing the popup.
       *
       * @param {Object} editor - The TipTap editor instance.
       */
      onCreate: function (editor) {
        editorInstance = editor;
        popup = new MentionPopup({
          campaignId: options.campaignId,
          onSelect: insertMention,
          onClose: closeMention,
        });
      },

      /**
       * Called on every editor transaction (keystroke, selection change, etc.).
       * Detects the @ trigger character and manages mention state.
       *
       * @param {Object} editor - The TipTap editor instance.
       */
      onUpdate: function (editor) {
        if (!editor.isEditable) return;
        editorInstance = editor;

        var state = editor.state;
        var from = state.selection.from;

        // Get text before cursor to detect @ trigger.
        var textBefore = '';
        var $pos = state.doc.resolve(from);
        var textNode = $pos.parent;

        if (textNode && textNode.isTextblock) {
          // Get all text content from start of current text block to cursor.
          var startOfBlock = $pos.start();
          textBefore = state.doc.textBetween(startOfBlock, from, '\0', '\0');
        }

        // Look for an @ that starts a mention query.
        // Match @ preceded by a space, newline, or at start of block.
        var mentionMatch = textBefore.match(/(^|[\s\0])@([\w\s]*)$/);

        if (mentionMatch) {
          var query = mentionMatch[2];
          // Calculate the position of the @ character.
          var atOffset = textBefore.length - mentionMatch[0].length + (mentionMatch[1].length);
          var startPos = $pos.start() + atOffset;

          if (!mentionActive) {
            mentionActive = true;
            mentionStartPos = startPos;

            // Show popup near cursor.
            var coords = getCursorCoords(editor.view, from);
            popup.show(coords);
          }

          // Update popup position on each keystroke in case of line wrapping.
          var updatedCoords = getCursorCoords(editor.view, from);
          if (popup.el) {
            popup.el.style.left = updatedCoords.left + 'px';
            popup.el.style.top = updatedCoords.bottom + 4 + 'px';
          }

          popup.updateQuery(query);
        } else if (mentionActive) {
          closeMention();
        }
      },

      /**
       * Keyboard handler for intercepting navigation keys while the
       * mention popup is open.
       *
       * @param {Object} editor - The TipTap editor instance.
       * @param {KeyboardEvent} event - The keyboard event.
       * @returns {boolean} True if the event was consumed.
       */
      onKeyDown: function (editor, event) {
        if (!mentionActive || !popup || !popup.visible) return false;
        return popup.handleKey(event.key);
      },

      /**
       * Clean up popup when the editor is destroyed.
       */
      onDestroy: function () {
        if (popup) {
          popup.destroy();
          popup = null;
        }
        mentionActive = false;
        mentionStartPos = null;
        editorInstance = null;
      },
    };
  }

  // --- Utility Functions ---

  /**
   * Escape HTML special characters to prevent XSS in rendered content.
   *
   * @param {string} str - Raw string to escape.
   * @returns {string} HTML-escaped string.
   */
  function escapeHTML(str) {
    if (!str) return '';
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  /**
   * Escape a string for safe use in HTML attribute values.
   *
   * @param {string} str - Raw string to escape.
   * @returns {string} Attribute-safe string.
   */
  function escapeAttr(str) {
    if (!str) return '';
    return str
      .replace(/&/g, '&amp;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }

  // --- Public API ---

  /**
   * Chronicle.MentionExtension - Factory function that creates a mention
   * extension configuration for the TipTap editor.
   *
   * Usage in editor.js:
   *   var mentionConfig = Chronicle.MentionExtension({ campaignId: '...' });
   *   // Wire mentionConfig.onCreate, onUpdate, onKeyDown, onDestroy
   *   // into the TipTap editor lifecycle.
   *
   * @param {Object} options
   * @param {string} options.campaignId - Campaign ID for entity search.
   * @returns {Object} Extension config with lifecycle hooks.
   */
  Chronicle.MentionExtension = createMentionExtensionConfig;

  /**
   * Chronicle.MentionNode - The mention node schema definition.
   * Used by the editor to register the custom node type so that
   * mentions are properly parsed from and serialized to HTML.
   */
  Chronicle.MentionNode = createMentionNode;
})();
