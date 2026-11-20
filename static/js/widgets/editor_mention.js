/**
 * editor_mention.js -- Chronicle @Mention Extension for TipTap
 *
 * Typing `@` opens a dropdown of matching campaign entities (via GET
 * /campaigns/:id/entities/search?q=...); selecting one inserts a mention
 * node rendered as an <a data-mention-id>. Degrades gracefully: API
 * failures close the dropdown, deleted entities render as plain text.
 *
 * Self-contained module exporting window.Chronicle.MentionExtension,
 * which editor.js includes in its TipTap extensions array when present.
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
  /**
   * The picker's note for a page not everyone can read (the search marks it
   * "restricted"), or '' for an open page. Shared with the [[ picker.
   */
  Chronicle.linkRestrictionLabel = function (item) {
    if (!item || !item.restricted) return '';
    return item.restricted === 'custom' ? 'Only some players can see it' : 'Hidden from players';
  };

  // A link shows the page's name to everyone who reads the text, even a
  // player who can't open the page. Picking a restricted page for text that
  // players read therefore takes a second pick, after a warning.
  var LINK_WARNING = 'Players read this text and will see this name. Choose it again to link.';

  // The list's tallest height; Chronicle.placeAtCaret reads it back from the
  // CSS to keep the list inside the window.
  var MAX_HEIGHT = 240;

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

    this.el.style.display = 'block';
    this.place(coords);

    // Add click-outside listener.
    var self = this;
    setTimeout(function () {
      document.addEventListener('mousedown', self._onClickOutside);
    }, 0);
  };

  /**
   * Put the list beside the cursor and inside the window: under the cursor
   * when a full list fits there, above it when it does not, and clear of the
   * window's sides. On a phone the window is what the keyboard leaves showing.
   *
   * @param {Object} [coords] - {left, top, bottom} of the cursor. Left out,
   *   the list is placed again where it was, after its height changed.
   */
  MentionPopup.prototype.place = function (coords) {
    if (coords) this._at = coords;
    if (!this.el || !this._at) return;
    Chronicle.placeAtCaret(this.el, this._at);
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
      'position:fixed;z-index:9999;display:none;box-sizing:border-box;min-width:220px;max-width:320px;' +
      'max-height:' + MAX_HEIGHT + 'px;overflow-y:auto;border-radius:8px;' +
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

    Chronicle.apiFetch(url, { signal: this.abortController.signal })
      .then(function (res) {
        if (!res.ok) throw new Error('Search failed: ' + res.status);
        return res.json();
      })
      .then(function (data) {
        self.abortController = null;
        // Foundry scenes follow the pages, for those who may see them.
        var scenes = Chronicle.FoundryScenes ? Chronicle.FoundryScenes.pickerItems(self.campaignId, query) : [];
        var results = (data.results || []).concat(scenes);
        self.items = results;
        self.selectedIndex = 0;
        self._renderItems(results);
      })
      .catch(function (err) {
        // Ignore abort errors (expected when query changes rapidly).
        if (err.name === 'AbortError') return;
        console.warn('[Mention] Search error:', err);
        // No toast here -- mention search failures are transient and a popup
        // would be disruptive while the user is mid-sentence typing a mention.
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
      if (item.kind === 'scene') {
        if (i === 0 || items[i - 1].kind !== 'scene') {
          html += '<div class="mention-popup__group" style="padding:8px 12px 2px;font-size:11px;font-weight:600;' +
            'letter-spacing:.08em;text-transform:uppercase;opacity:.7">Foundry scenes</div>';
        }
        html += this._sceneItemHTML(item, i, isDark);
        continue;
      }
      // A page not everyone can read says so under its name, and so does the
      // warning shown before such a page is linked.
      var note = Chronicle.linkRestrictionLabel(item);
      var under = '';
      if (item._warned) {
        under = '<div style="font-size:12px;white-space:normal;color:#b45309">' + Chronicle.escapeHtml(LINK_WARNING) + '</div>';
      } else if (note) {
        under = '<div style="font-size:12px;opacity:0.6"><i class="fa-solid fa-lock" aria-hidden="true" style="font-size:10px"></i> ' +
          Chronicle.escapeHtml(note) + '</div>';
      }
      html += this._rowHTML(i, isDark, Chronicle.escapeHtml(item.name), Chronicle.escapeHtml(item.type_name || ''), under, kindIconHTML(item.type_icon, item.type_color));
    }

    this.el.innerHTML = html;
    this.place();

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
   * A page's category icon, in front of its name. The site's CSS hides it
   * unless the campaign shows icons beside names, so by default every name
   * starts at the row's edge. Icon and colour are checked because both land
   * in markup. Scene rows have none.
   */
  function kindIconHTML(icon, colour) {
    if (!/^fa-[a-z0-9-]{1,40}$/.test(icon || '')) return '';
    return '<i class="fa-solid ' + icon + ' kind-ic" style="margin-right:-4px' +
      (/^#[0-9a-fA-F]{6}$/.test(colour || '') ? ';--kind-colour:' + colour.toLowerCase() : '') + '" aria-hidden="true"></i>';
  }

  /**
   * One row of the list: the name, then a quiet word at the end saying what
   * it is, and under them an optional line. By default nothing is drawn in
   * front, so every name starts at the same edge and a long one keeps the most
   * room; leadHTML is the hidden-until-asked category icon.
   */
  MentionPopup.prototype._rowHTML = function (i, isDark, nameHTML, endHTML, underHTML, leadHTML) {
    var selected = i === this.selectedIndex;
    var bgSelected = selected ? (isDark ? 'background-color:#374151;' : 'background-color:#f3f4f6;') : '';
    return '<div class="mention-popup__item' + (selected ? ' mention-popup__item--selected' : '') + '" ' +
      'data-index="' + i + '" style="padding:8px 12px;cursor:pointer;transition:background-color 0.1s;' + bgSelected + '">' +
      '<div style="display:flex;align-items:baseline;gap:12px">' + (leadHTML || '') +
      '<span class="mention-popup__name" style="min-width:0;flex:1;font-weight:500;font-size:14px;' +
      'white-space:nowrap;overflow:hidden;text-overflow:ellipsis">' + nameHTML + '</span>' +
      (endHTML ? '<span class="mention-popup__kind" style="flex-shrink:0;max-width:45%;font-size:12px;opacity:0.6;' +
        'white-space:nowrap;overflow:hidden;text-overflow:ellipsis">' + endHTML + '</span>' : '') +
      '</div>' + (underHTML || '') + '</div>';
  };

  /**
   * One Foundry scene row: its name with its folder at the end, or, for a
   * name typed before the scene exists, the offer to mark it as needed.
   */
  MentionPopup.prototype._sceneItemHTML = function (item, i, isDark) {
    if (item.needed) {
      return this._rowHTML(i, isDark, '\u201c' + Chronicle.escapeHtml(item.name) + '\u201d', '',
        '<div style="font-size:12px;opacity:0.6">Not in Foundry yet: mark the map as needed</div>');
    }
    return this._rowHTML(i, isDark, Chronicle.escapeHtml(item.name), Chronicle.escapeHtml(item.folder || 'Top level'), '');
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
      Chronicle.escapeHtml(message) +
      '</div>';
    this.place();
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
    // Where the caret sat after the space a pick leaves behind the name,
    // kept until the next edit.
    var spaceAfterPick = null;

    // What a sentence never puts a space in front of.
    var CLOSING_MARKS = ',.;:!?)';

    /**
     * Takes back the space a pick left after the name when the very next
     * thing typed there is a closing mark, so "@Name ," reads "@Name,". It
     * reads what the edit left in the text, never the key, so a phone's
     * keyboard is served the same. True when it changed the text.
     *
     * @param {Object} editor - The TipTap editor instance.
     * @param {number} at - The caret position just after that space.
     */
    function closeUpAfterPick(editor, at) {
      var state = editor.state;
      if (editor.view.composing || !state.selection.empty || state.selection.from !== at + 1) return false;
      if (at < 1 || at + 1 > state.doc.content.size) return false;
      var typed = state.doc.textBetween(at - 1, at + 1, '\0', '\0');
      var space = typed.charAt(0);
      if (typed.length !== 2 || (space !== '\u00a0' && space !== ' ') || CLOSING_MARKS.indexOf(typed.charAt(1)) === -1) return false;
      editor.chain().deleteRange({ from: at - 1, to: at }).run();
      return true;
    }

    /**
     * Insert a mention node at the current @ trigger position.
     *
     * @param {Object} entity - The selected entity from search results.
     */
    function insertMention(entity) {
      if (!editorInstance || mentionStartPos === null) return;
      if (entity.kind === 'scene') {
        insertScene(entity);
        return;
      }
      if (entity.restricted && options.hostOpen && !entity._warned) {
        entity._warned = true;
        popup._renderItems(popup.items);
        return;
      }

      var editor = editorInstance;
      var from = mentionStartPos;
      var to = editor.state.selection.from;

      // Create the mention node content as HTML and insert it.
      // Include data-entity-preview for hover tooltip support.
      var previewAttr = entity.url ? ' data-entity-preview="' + Chronicle.escapeAttr(entity.url + '/preview') + '"' : '';
      var mentionHTML =
        '<a data-mention-id="' + Chronicle.escapeAttr(entity.id) + '" ' +
        'href="' + Chronicle.escapeAttr(entity.url) + '"' + previewAttr + ' ' +
        'class="mention-link text-accent font-medium hover:underline cursor-pointer" ' +
        'contenteditable="false">@' + Chronicle.escapeHtml(entity.name) + '</a>';

      // Delete the @query text and insert the mention link.
      editor
        .chain()
        .focus()
        .deleteRange({ from: from, to: to })
        .insertContent(mentionHTML + '&nbsp;')
        .run();

      spaceAfterPick = editor.state.selection.from;
      closeMention();
    }

    /**
     * Insert a Foundry scene mention in place of the @query. Without the
     * scene node in this editor's schema the name goes in as plain text.
     */
    function insertScene(item) {
      var editor = editorInstance;
      var range = { from: mentionStartPos, to: editor.state.selection.from };
      var content = editor.schema.nodes.foundryScene
        ? [{ type: 'foundryScene', attrs: { sceneId: item.sceneId || null, name: item.name } }, { type: 'text', text: ' ' }]
        : item.name + ' ';
      editor.chain().focus().deleteRange(range).insertContent(content).run();
      spaceAfterPick = editor.state.selection.from;
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
        // Load the scene list early, so the first @ already offers scenes.
        if (Chronicle.FoundryScenes) Chronicle.FoundryScenes.want(options.campaignId);
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

        if (spaceAfterPick !== null) {
          var afterSpace = spaceAfterPick;
          spaceAfterPick = null;
          // Its own edit comes back through here and does the rest.
          if (closeUpAfterPick(editor, afterSpace)) return;
        }

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

          // Skip if the @ is inside a link mark (an already-inserted mention).
          // Resolve the position just after @ and check its marks.
          var $atPos = state.doc.resolve(Math.min(startPos + 1, state.doc.content.size));
          var marks = $atPos.marks();
          var inLink = false;
          for (var mi = 0; mi < marks.length; mi++) {
            if (marks[mi].type.name === 'link') {
              inLink = true;
              break;
            }
          }
          if (inLink) {
            if (mentionActive) closeMention();
            return;
          }

          if (!mentionActive) {
            mentionActive = true;
            mentionStartPos = startPos;

            // Show popup near cursor.
            var coords = getCursorCoords(editor.view, from);
            popup.show(coords);
          }

          // Follow the cursor on each keystroke, in case the line wrapped.
          popup.place(getCursorCoords(editor.view, from));

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
        spaceAfterPick = null;
        editorInstance = null;
      },
    };
  }

  // --- Extended Link Mark for Entity Mentions ---

  // The kinds of address a link may carry.
  var LINK_PROTOCOLS = ['http:', 'https:', 'mailto:', 'tel:', 'sms:', 'ftp:', 'ftps:', 'callto:', 'cid:', 'xmpp:'];

  // What a link's address is to this site: 'inside' for a page of this site,
  // 'outside' for anywhere else a link may go, 'refused' for an address that
  // is not a link (one that runs code, or cannot be read). The browser's own
  // address parser decides, so "//other.example" or a tab tucked into a path
  // cannot pass for a page of this site.
  function linkKind(href, base) {
    if (typeof href !== 'string' || href === '') return 'outside';
    var url, here;
    try {
      url = new URL(href, base);
      here = new URL(base);
    } catch (e) {
      return 'refused';
    }
    if (LINK_PROTOCOLS.indexOf(url.protocol) === -1) return 'refused';
    var web = url.protocol === 'http:' || url.protocol === 'https:';
    return web && url.origin === here.origin ? 'inside' : 'outside';
  }

  /**
   * Chronicle.MentionLink - Extended TipTap Link mark that preserves
   * entity mention attributes through the ProseMirror JSON round-trip.
   *
   * TipTap's default Link mark only stores href/target. When a mention is
   * inserted as <a data-mention-id="..." data-entity-preview="...">, those
   * attributes are dropped during parse → JSON → render. This extended mark
   * keeps them in the schema so hover preview cards work after save/reload.
   *
   * Must replace TipTap.Link in the editor extensions array.
   */
  Chronicle.MentionLink = TipTap.Link.extend({
    addAttributes: function () {
      // Inherit parent Link attributes (href, target, etc.).
      var parentAttrs = {};
      if (this.parent) {
        try { parentAttrs = this.parent(); } catch (e) {}
      }
      // Ensure core Link attrs exist even if parent() failed.
      if (!parentAttrs.href) {
        parentAttrs.href = { default: null };
      }
      if (!parentAttrs.target) {
        parentAttrs.target = { default: null };
      }

      // Entity mention ID — identifies this link as an entity reference.
      parentAttrs['data-mention-id'] = {
        default: null,
        parseHTML: function (el) {
          return el.getAttribute('data-mention-id');
        },
        renderHTML: function (attributes) {
          if (!attributes['data-mention-id']) return {};
          return { 'data-mention-id': attributes['data-mention-id'] };
        },
      };

      // Entity preview URL — used by entity_tooltip.js for hover cards.
      parentAttrs['data-entity-preview'] = {
        default: null,
        parseHTML: function (el) {
          return el.getAttribute('data-entity-preview');
        },
        renderHTML: function (attributes) {
          if (!attributes['data-entity-preview']) return {};
          return { 'data-entity-preview': attributes['data-entity-preview'] };
        },
      };

      return parentAttrs;
    },

    // Override renderHTML to add entity-link class on mention links while
    // keeping regular links styled normally.
    renderHTML: function (props) {
      var attrs = props.HTMLAttributes;
      // Merge with configured HTMLAttributes (class, target, rel, etc.).
      var merged = {};
      var optAttrs = this.options.HTMLAttributes || {};
      var key;
      for (key in optAttrs) {
        if (optAttrs.hasOwnProperty(key)) merged[key] = optAttrs[key];
      }
      for (key in attrs) {
        if (attrs.hasOwnProperty(key)) merged[key] = attrs[key];
      }
      var kind = linkKind(merged.href, window.location.href);
      // Saved text is loaded as JSON, which skips the check made when a link
      // is typed or pasted, so the drawing repeats it: a refused address is
      // drawn as a link to nowhere.
      if (kind === 'refused') merged.href = '';
      // Inside the site a link opens in place, like every other link in the
      // app; only another website gets the new tab. The address alone
      // decides, whatever the saved link says about itself.
      if (kind === 'inside') {
        delete merged.target;
        delete merged.rel;
      } else {
        merged.target = '_blank';
        merged.rel = 'noopener noreferrer nofollow';
      }
      // Entity mention links get distinctive styling + non-editable behavior.
      if (merged['data-mention-id']) {
        merged['class'] = 'entity-link';
        merged['contenteditable'] = 'false';
      }
      return ['a', merged, 0];
    },
  });

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
   * @param {boolean} [options.hostOpen] - the text is read by players, so
   *   linking a restricted page asks first.
   * @returns {Object} Extension config with lifecycle hooks.
   */
  Chronicle.MentionExtension = createMentionExtensionConfig;
})();
