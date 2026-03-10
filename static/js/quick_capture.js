/**
 * quick_capture.js -- Quick Capture Modal (Ctrl+Shift+N)
 *
 * Opens a lightweight modal for instant note creation within the current
 * campaign. Pressing Ctrl+Shift+N pops up a small form with a title
 * (pre-filled with a timestamp) and a text area. Submitting creates a
 * campaign-wide note via the existing notes API.
 *
 * Also provides Chronicle.openSessionJournal() for the topbar "Session
 * Journal" button, which creates or appends to today's dated journal note.
 *
 * Features:
 *   - Ctrl+Shift+N global shortcut
 *   - Title auto-filled with "Quick Note - YYYY-MM-DD HH:MM"
 *   - Creates note via POST /campaigns/:id/notes
 *   - After creation, shows success toast and optionally opens the notes panel
 *   - Session Journal: finds or creates "Session Journal - YYYY-MM-DD" note
 */
(function () {
  'use strict';

  window.Chronicle = window.Chronicle || {};

  var overlay = null;
  var isOpen = false;

  // --- Campaign ID Extraction ---

  function getCampaignId() {
    var parts = window.location.pathname.split('/');
    if (parts.length >= 3 && parts[1] === 'campaigns' && parts[2] !== '' &&
        parts[2] !== 'new' && parts[2] !== 'picker') {
      return parts[2];
    }
    return '';
  }

  // --- Date Formatting ---

  function formatDate(d) {
    var pad = function (n) { return n < 10 ? '0' + n : '' + n; };
    return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate());
  }

  function formatTime(d) {
    var pad = function (n) { return n < 10 ? '0' + n : '' + n; };
    return pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  // --- Modal Construction ---

  function buildModal() {
    overlay = document.createElement('div');
    overlay.className = 'fixed inset-0 z-[100] flex items-start justify-center pt-[20vh] bg-black/60';
    overlay.style.display = 'none';
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) close();
    });

    var modal = document.createElement('div');
    modal.className = 'w-full max-w-md bg-surface border border-edge rounded-lg shadow-2xl overflow-hidden';
    modal.addEventListener('click', function (e) { e.stopPropagation(); });

    // Header.
    var header = document.createElement('div');
    header.className = 'flex items-center justify-between px-4 py-3 border-b border-edge';
    header.innerHTML =
      '<div class="flex items-center gap-2">' +
        '<i class="fa-solid fa-bolt text-accent text-sm"></i>' +
        '<span class="text-sm font-semibold text-fg">Quick Capture</span>' +
      '</div>' +
      '<kbd class="text-[10px] text-fg-muted border border-edge rounded px-1.5 py-0.5 font-mono">ESC</kbd>';

    // Form body.
    var body = document.createElement('div');
    body.className = 'p-4 space-y-3';

    // Title input.
    var titleLabel = document.createElement('label');
    titleLabel.className = 'block text-xs font-medium text-fg-secondary mb-1';
    titleLabel.textContent = 'Title';

    var titleInput = document.createElement('input');
    titleInput.type = 'text';
    titleInput.className = 'input w-full text-sm';
    titleInput.setAttribute('autocomplete', 'off');
    titleInput.id = 'qc-title';

    // Content textarea.
    var contentLabel = document.createElement('label');
    contentLabel.className = 'block text-xs font-medium text-fg-secondary mb-1';
    contentLabel.textContent = 'Content';

    var contentArea = document.createElement('textarea');
    contentArea.className = 'input w-full text-sm resize-none';
    contentArea.rows = 4;
    contentArea.placeholder = 'Write your note...';
    contentArea.id = 'qc-content';

    body.appendChild(titleLabel);
    body.appendChild(titleInput);
    body.appendChild(contentLabel);
    body.appendChild(contentArea);

    // Footer with submit button.
    var footer = document.createElement('div');
    footer.className = 'flex items-center justify-end gap-2 px-4 py-3 border-t border-edge';

    var cancelBtn = document.createElement('button');
    cancelBtn.type = 'button';
    cancelBtn.className = 'btn-secondary text-sm px-3 py-1.5';
    cancelBtn.textContent = 'Cancel';
    cancelBtn.addEventListener('click', close);

    var submitBtn = document.createElement('button');
    submitBtn.type = 'button';
    submitBtn.className = 'btn-primary text-sm px-4 py-1.5';
    submitBtn.innerHTML = '<i class="fa-solid fa-check mr-1"></i> Create Note';
    submitBtn.id = 'qc-submit';
    submitBtn.addEventListener('click', submitNote);

    footer.appendChild(cancelBtn);
    footer.appendChild(submitBtn);

    modal.appendChild(header);
    modal.appendChild(body);
    modal.appendChild(footer);
    overlay.appendChild(modal);
    document.body.appendChild(overlay);

    // Keyboard handling.
    titleInput.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { e.preventDefault(); close(); }
      if (e.key === 'Enter') { e.preventDefault(); contentArea.focus(); }
    });

    contentArea.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { e.preventDefault(); close(); }
      // Ctrl+Enter to submit.
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        submitNote();
      }
    });
  }

  // --- Open / Close ---

  function open() {
    if (isOpen) return;
    var cid = getCampaignId();
    if (!cid) return;

    if (!overlay) buildModal();

    var now = new Date();
    var titleInput = document.getElementById('qc-title');
    var contentArea = document.getElementById('qc-content');

    titleInput.value = 'Quick Note - ' + formatDate(now) + ' ' + formatTime(now);
    contentArea.value = '';

    overlay.style.display = '';
    isOpen = true;

    requestAnimationFrame(function () {
      contentArea.focus();
    });
  }

  function close() {
    if (!isOpen) return;
    overlay.style.display = 'none';
    isOpen = false;
  }

  // --- Submit ---

  function submitNote() {
    var cid = getCampaignId();
    if (!cid) return;

    var titleInput = document.getElementById('qc-title');
    var contentArea = document.getElementById('qc-content');
    var submitBtn = document.getElementById('qc-submit');

    var title = titleInput.value.trim();
    var content = contentArea.value.trim();

    if (!title) {
      titleInput.focus();
      return;
    }

    // Build content blocks (text block with the note content).
    var blocks = [];
    if (content) {
      blocks.push({ type: 'text', value: content });
    }

    submitBtn.disabled = true;
    submitBtn.textContent = 'Creating...';

    Chronicle.apiFetch('/campaigns/' + encodeURIComponent(cid) + '/notes', {
      method: 'POST',
      body: {
        title: title,
        content: blocks,
      },
    })
      .then(function (res) {
        if (!res.ok) throw new Error('Failed to create note: ' + res.status);
        return res.json();
      })
      .then(function () {
        close();
        var journalUrl = '/campaigns/' + encodeURIComponent(cid) + '/journal';
        Chronicle.notify('Note created — <a href="' + journalUrl + '" style="color:inherit;text-decoration:underline;font-weight:500;">View in Journal</a>', 'success', { duration: 6000, html: true });
        // Dispatch event so the notes widget can refresh its list.
        window.dispatchEvent(new CustomEvent('chronicle:note-created'));
      })
      .catch(function (err) {
        console.error('[QuickCapture] Error:', err);
        Chronicle.notify('Failed to create note', 'error');
      })
      .finally(function () {
        submitBtn.disabled = false;
        submitBtn.innerHTML = '<i class="fa-solid fa-check mr-1"></i> Create Note';
      });
  }

  // --- Session Journal ---

  /**
   * Open or create today's session journal note. Finds an existing note
   * titled "Session Journal - YYYY-MM-DD" or creates one, then opens the
   * notes panel.
   */
  function openSessionJournal() {
    var cid = getCampaignId();
    if (!cid) return;

    var today = formatDate(new Date());
    var journalTitle = 'Session Journal - ' + today;

    // Fetch existing notes to find today's journal.
    Chronicle.apiFetch('/campaigns/' + encodeURIComponent(cid) + '/notes?scope=campaign')
      .then(function (res) {
        if (!res.ok) throw new Error('Failed to fetch notes');
        return res.json();
      })
      .then(function (notes) {
        // Look for existing journal note for today.
        var existing = null;
        for (var i = 0; i < notes.length; i++) {
          if (notes[i].title === journalTitle) {
            existing = notes[i];
            break;
          }
        }

        if (existing) {
          // Journal exists -- open notes panel and signal it to focus this note.
          window.dispatchEvent(new CustomEvent('chronicle:open-note', {
            detail: { noteId: existing.id },
          }));
          return null;
        }

        // Create new journal note.
        return Chronicle.apiFetch('/campaigns/' + encodeURIComponent(cid) + '/notes', {
          method: 'POST',
          body: {
            title: journalTitle,
            content: [{ type: 'text', value: '' }],
            color: '#7c3aed', // Purple to distinguish journal notes.
          },
        });
      })
      .then(function (res) {
        if (!res) return; // Already dispatched open event.
        if (!res.ok) throw new Error('Failed to create journal');
        return res.json();
      })
      .then(function (note) {
        if (!note) return;
        Chronicle.notify('Session journal created for ' + today, 'success');
        window.dispatchEvent(new CustomEvent('chronicle:note-created'));
        window.dispatchEvent(new CustomEvent('chronicle:open-note', {
          detail: { noteId: note.id },
        }));
      })
      .catch(function (err) {
        console.error('[SessionJournal] Error:', err);
        Chronicle.notify('Failed to open session journal', 'error');
      });
  }

  // --- Global Shortcut ---

  document.addEventListener('keydown', function (e) {
    // Ctrl+Shift+N: Quick capture.
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'N') {
      e.preventDefault();
      open();
    }
  });

  // Close on navigation.
  window.addEventListener('chronicle:navigated', function () {
    if (isOpen) close();
  });

  // --- Public API ---

  Chronicle.openQuickCapture = open;
  Chronicle.openSessionJournal = openSessionJournal;
})();
