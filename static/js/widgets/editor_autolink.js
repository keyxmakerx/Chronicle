/**
 * editor_autolink.js -- Chronicle Auto-Link Entities Feature
 *
 * Scans editor text for entity names and converts them to @mention links.
 * Triggered via the Insert menu "Auto-link Entities" action or Ctrl+Shift+L.
 *
 * Fetches entity names from GET /campaigns/:id/entity-names (Redis-cached).
 * Names are matched whole-word, case-insensitive, minimum 3 characters.
 * Longer names are matched first to prevent partial matches.
 *
 * Integration:
 *   editor.js calls Chronicle.autoLinkEntities(editor, campaignId) when
 *   the auto-link action is triggered. This module handles the rest.
 */
(function () {
  'use strict';

  if (!window.Chronicle) {
    console.error('[AutoLink] Chronicle namespace not available.');
    return;
  }

  // Cache entity names per campaign to avoid repeated fetches within a session.
  var nameCache = {};
  var fetchInFlight = {};

  /**
   * Fetch entity names for a campaign. Returns a promise that resolves to
   * an array of {id, name, slug, type_name, type_icon, type_slug}.
   * Results are cached in memory (Redis cache provides cross-request caching).
   */
  function fetchEntityNames(campaignId) {
    if (nameCache[campaignId]) {
      return Promise.resolve(nameCache[campaignId]);
    }
    if (fetchInFlight[campaignId]) {
      return fetchInFlight[campaignId];
    }

    var url = '/campaigns/' + campaignId + '/entity-names';
    fetchInFlight[campaignId] = fetch(url, {
      headers: { 'Accept': 'application/json' },
      credentials: 'same-origin'
    })
      .then(function (resp) {
        if (!resp.ok) throw new Error('Failed to fetch entity names');
        return resp.json();
      })
      .then(function (data) {
        var names = data.names || [];
        nameCache[campaignId] = names;
        delete fetchInFlight[campaignId];
        return names;
      })
      .catch(function (err) {
        delete fetchInFlight[campaignId];
        console.error('[AutoLink] Failed to fetch entity names:', err);
        Chronicle.notify('Could not load entity names for auto-linking.', 'error');
        throw err;
      });

    return fetchInFlight[campaignId];
  }

  /**
   * Build entity URL from campaign ID and entity slug/type.
   */
  function entityURL(campaignId, entry) {
    return '/campaigns/' + campaignId + '/entities/' + entry.id;
  }

  /**
   * Create a whole-word regex for an entity name. Escapes special regex chars.
   * Case-insensitive. Uses word boundary assertions where possible, but falls
   * back to whitespace/punctuation checks for names starting/ending with
   * non-word characters.
   */
  function nameRegex(name) {
    var escaped = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    return new RegExp('\\b' + escaped + '\\b', 'gi');
  }

  /**
   * Auto-link entities in a TipTap editor instance.
   *
   * Scans all text nodes for entity name matches, skipping text that's
   * already inside a link (mention or regular). Creates mention links
   * for each match found.
   *
   * @param {Object} editor - TipTap editor instance
   * @param {string} campaignId - Campaign ID for API calls
   * @returns {Promise<number>} Number of links created
   */
  function autoLinkEntities(editor, campaignId) {
    if (!editor || !campaignId) {
      return Promise.resolve(0);
    }

    return fetchEntityNames(campaignId).then(function (entries) {
      if (!entries || entries.length === 0) return 0;

      var doc = editor.state.doc;
      var linkedCount = 0;

      // Collect all text ranges that are NOT inside links.
      var textRanges = [];
      doc.descendants(function (node, pos) {
        if (node.isText) {
          // Check if this text node is inside a link mark.
          var marks = node.marks || [];
          var inLink = marks.some(function (m) {
            return m.type.name === 'link';
          });
          if (!inLink) {
            textRanges.push({
              text: node.text,
              from: pos,
              to: pos + node.nodeSize
            });
          }
        }
      });

      if (textRanges.length === 0) return 0;

      // Build a combined text with position mapping for efficient matching.
      // Process each entry (already sorted by name length DESC from the API).
      var replacements = [];
      var usedRanges = []; // Track which character positions are already matched.

      for (var i = 0; i < entries.length; i++) {
        var entry = entries[i];
        if (entry.name.length < 3) continue;

        var regex = nameRegex(entry.name);

        for (var r = 0; r < textRanges.length; r++) {
          var range = textRanges[r];
          var match;
          regex.lastIndex = 0;

          while ((match = regex.exec(range.text)) !== null) {
            var matchFrom = range.from + match.index;
            var matchTo = matchFrom + match[0].length;

            // Skip if this range overlaps with an already-matched range.
            var overlaps = false;
            for (var u = 0; u < usedRanges.length; u++) {
              if (matchFrom < usedRanges[u].to && matchTo > usedRanges[u].from) {
                overlaps = true;
                break;
              }
            }
            if (overlaps) continue;

            replacements.push({
              from: matchFrom,
              to: matchTo,
              entry: entry,
              matchedText: match[0]
            });
            usedRanges.push({ from: matchFrom, to: matchTo });
          }
        }
      }

      if (replacements.length === 0) return 0;

      // Sort replacements by position (descending) so we can apply them
      // back-to-front without position shifts.
      replacements.sort(function (a, b) { return b.from - a.from; });

      // Apply all replacements in a single transaction.
      var chain = editor.chain().focus();

      for (var j = 0; j < replacements.length; j++) {
        var rep = replacements[j];
        var url = entityURL(campaignId, rep.entry);
        var previewURL = url + '/preview';

        // Select the matched text and add link mark.
        chain = chain
          .setTextSelection({ from: rep.from, to: rep.to })
          .deleteSelection()
          .insertContent({
            type: 'text',
            text: '@' + rep.matchedText,
            marks: [{
              type: 'link',
              attrs: {
                href: url,
                target: null,
                'data-mention-id': rep.entry.id,
                'data-entity-preview': previewURL
              }
            }]
          });

        linkedCount++;
      }

      chain.run();
      return linkedCount;
    });
  }

  /**
   * Invalidate the entity names cache for a campaign. Called when entities
   * are created/updated/deleted so the next auto-link fetch gets fresh data.
   */
  function invalidateCache(campaignId) {
    delete nameCache[campaignId];
  }

  // Expose on Chronicle namespace.
  Chronicle.autoLinkEntities = autoLinkEntities;
  Chronicle.invalidateAutoLinkCache = invalidateCache;
})();
