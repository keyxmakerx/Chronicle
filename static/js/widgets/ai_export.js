/**
 * ai_export.js — Copy-to-clipboard for the AI-Export modal.
 *
 * Mounts via data-widget="ai-export" on the modal returned by
 * GET /campaigns/:id/ai-export/generate. Wires the button with
 * `data-ai-export-copy` to write the contents of the
 * `data-ai-export-body` <pre> to the clipboard via the modern
 * navigator.clipboard API. Falls back to a document.execCommand
 * path on browsers that block clipboard write outside a secure
 * context.
 *
 * The "Copied" pulse + reset is handled by Alpine.js inside the
 * modal templ; this widget only owns the actual write.
 */
(function () {
  'use strict';

  Chronicle.register('ai-export', {
    init: function (el) {
      var btn = el.querySelector('[data-ai-export-copy]');
      var body = el.querySelector('[data-ai-export-body]');
      if (!btn || !body) {
        return;
      }
      btn.addEventListener('click', function () {
        var text = body.textContent || '';
        copyToClipboard(text);
      });
    },
  });

  function copyToClipboard(text) {
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).catch(function () {
        // Permission denied or transient — fall back below.
        legacyCopy(text);
      });
      return;
    }
    legacyCopy(text);
  }

  // Pre-Clipboard-API fallback. Uses a hidden, off-screen <textarea>
  // + document.execCommand('copy'). Works on insecure-origin dev hosts
  // and older browsers; same shape Chronicle's other copy buttons use.
  function legacyCopy(text) {
    var ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    ta.style.top = '-9999px';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    try {
      document.execCommand('copy');
    } catch (e) {
      // Last-resort silent failure; the user can still ctrl-A inside
      // the <pre> via the select-all class.
    }
    document.body.removeChild(ta);
  }
})();
