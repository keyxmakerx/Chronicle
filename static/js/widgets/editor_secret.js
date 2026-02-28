/**
 * editor_secret.js -- Chronicle Inline Secrets Extension
 *
 * TipTap mark extension for inline GM-only secrets. Text wrapped in this
 * mark is stored with a `data-secret` attribute and stripped server-side
 * for non-scribe users. Owners see secrets with a visual indicator;
 * players never receive the secret content.
 *
 * Uses TipTap.Underline.extend() to create a proper Mark type without
 * needing the raw Mark class (which isn't exported from the bundle).
 */
(function () {
  'use strict';

  if (!window.TipTap || !TipTap.Underline) {
    console.error('[Secret] TipTap Underline extension required.');
    return;
  }

  // Create a Secret mark by extending Underline (a Mark subclass).
  // Override name, parsing, rendering, keyboard shortcut, and commands.
  var SecretMark = TipTap.Underline.extend({
    name: 'secret',

    addOptions: function () {
      return {};
    },

    addAttributes: function () {
      return {};
    },

    parseHTML: function () {
      return [
        { tag: 'span[data-secret]' },
      ];
    },

    renderHTML: function (props) {
      return ['span', {
        'data-secret': 'true',
        'class': 'chronicle-secret',
      }, 0];
    },

    addCommands: function () {
      var self = this;
      return {
        toggleSecret: function () {
          return function (opts) {
            return opts.commands.toggleMark(self.name);
          };
        },
      };
    },

    addKeyboardShortcuts: function () {
      return {
        'Mod-Shift-s': function () {
          return this.editor.commands.toggleSecret();
        },
      };
    },
  });

  // Export for use in editor.js.
  window.Chronicle = window.Chronicle || {};
  Chronicle.SecretMark = SecretMark;
})();
