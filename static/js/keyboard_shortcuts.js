/**
 * keyboard_shortcuts.js -- Global Keyboard Shortcuts
 *
 * Registers application-wide keyboard shortcuts:
 *   - Ctrl+N / Cmd+N: New entity (within current campaign)
 *   - Ctrl+E / Cmd+E: Edit current entity (on entity show pages)
 *   - Ctrl+S / Cmd+S: Save (triggers visible save button or fires chronicle:save)
 *
 * Shortcuts are suppressed when the user is typing in an input, textarea,
 * or contenteditable element (except Ctrl+S which always works).
 */
(function () {
  'use strict';

  /**
   * Extract the campaign ID from the current URL.
   * Pattern: /campaigns/:id/...
   */
  function getCampaignId() {
    var parts = window.location.pathname.split('/');
    if (parts.length >= 3 && parts[1] === 'campaigns' && parts[2] !== '' &&
        parts[2] !== 'new' && parts[2] !== 'picker') {
      return parts[2];
    }
    return '';
  }

  /**
   * Detect if the current page is an entity show page and return the edit URL.
   * Entity show pattern: /campaigns/:cid/:slug/:eid (4+ segments, not "entities")
   * Also matches: /campaigns/:cid/entities/:eid
   */
  function getEntityEditUrl() {
    // Look for an Edit link on the page (most reliable).
    var editLink = document.querySelector('a.btn-secondary[href*="/edit"]');
    if (editLink) {
      return editLink.getAttribute('href');
    }
    return '';
  }

  /**
   * Check if the active element is a text input where typing occurs.
   */
  function isTyping() {
    var el = document.activeElement;
    if (!el) return false;
    var tag = el.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
    if (el.isContentEditable) return true;
    return false;
  }

  /**
   * Find and click the most relevant save button on the page.
   * Priority: #te-save-btn (template editor) > .chronicle-editor__btn--save.has-changes
   * > form submit .btn-primary > custom chronicle:save event.
   */
  function triggerSave() {
    // Template editor save button.
    var teBtn = document.getElementById('te-save-btn');
    if (teBtn && teBtn.offsetParent !== null) {
      teBtn.click();
      return true;
    }

    // Editor save button with unsaved changes.
    var editorSave = document.querySelector('.chronicle-editor__btn--save.has-changes');
    if (editorSave && editorSave.offsetParent !== null) {
      editorSave.click();
      return true;
    }

    // Any visible form with a primary submit button.
    var forms = document.querySelectorAll('form');
    for (var i = 0; i < forms.length; i++) {
      var btn = forms[i].querySelector('button.btn-primary[type="submit"]');
      if (btn && btn.offsetParent !== null) {
        btn.click();
        return true;
      }
    }

    // Fire a custom event for widgets to handle.
    window.dispatchEvent(new CustomEvent('chronicle:save'));
    return false;
  }

  // --- Global Keyboard Listener ---

  document.addEventListener('keydown', function (e) {
    var mod = e.ctrlKey || e.metaKey;
    if (!mod) return;

    switch (e.key) {
      // Ctrl+N: New entity.
      case 'n':
        if (isTyping()) return;
        var cid = getCampaignId();
        if (!cid) return;
        e.preventDefault();
        window.location.href = '/campaigns/' + cid + '/entities/new';
        break;

      // Ctrl+E: Edit current entity.
      case 'e':
        if (isTyping()) return;
        var editUrl = getEntityEditUrl();
        if (!editUrl) return;
        e.preventDefault();
        window.location.href = editUrl;
        break;

      // Ctrl+S: Save.
      case 's':
        e.preventDefault();
        triggerSave();
        break;
    }
  });
})();
