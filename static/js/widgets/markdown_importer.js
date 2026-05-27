/**
 * markdown_importer.js — drag/drop + multi-file upload widget for
 * AI Workspace > Import (V1 Phase 4).
 *
 * Mount: data-widget="markdown-importer". Looks for the
 * data-importer-dropzone (drag target), data-importer-file-input
 * (hidden <input type=file multiple>), and data-importer-file-list
 * (preview ul) elements inside the mount point.
 *
 * The widget owns the drag-active visual state + a per-file
 * preview list. It does NOT POST; the surrounding HTMX <form>
 * handles submission (the textarea + the file <input> are real
 * form fields that HTMX serializes).
 */
(function () {
  'use strict';

  Chronicle.register('markdown-importer', {
    init: function (el) {
      var zone = el.querySelector('[data-importer-dropzone]');
      var input = el.querySelector('[data-importer-file-input]');
      var list = el.querySelector('[data-importer-file-list]');
      if (!zone || !input || !list) {
        return;
      }

      ['dragenter', 'dragover'].forEach(function (evt) {
        zone.addEventListener(evt, function (e) {
          e.preventDefault();
          e.stopPropagation();
          zone.classList.add('ring-2', 'ring-accent', 'bg-accent/5');
        });
      });
      ['dragleave', 'drop'].forEach(function (evt) {
        zone.addEventListener(evt, function (e) {
          e.preventDefault();
          e.stopPropagation();
          zone.classList.remove('ring-2', 'ring-accent', 'bg-accent/5');
        });
      });

      zone.addEventListener('drop', function (e) {
        var dropped = Array.from(e.dataTransfer.files || []).filter(function (f) {
          var n = f.name.toLowerCase();
          return n.endsWith('.md') || n.endsWith('.markdown');
        });
        if (dropped.length === 0) {
          return;
        }
        // Replace the input's FileList with the dropped files.
        // FileList is read-only; build a DataTransfer to assign.
        var dt = new DataTransfer();
        dropped.forEach(function (f) { dt.items.add(f); });
        input.files = dt.files;
        renderList(dropped, list);
      });

      input.addEventListener('change', function () {
        renderList(Array.from(input.files || []), list);
      });
    },
  });

  function renderList(files, list) {
    list.innerHTML = '';
    if (files.length === 0) {
      return;
    }
    files.forEach(function (f) {
      var li = document.createElement('div');
      li.innerHTML = '<i class="fa-solid fa-file-code mr-1"></i> ' +
        escapeHtml(f.name) +
        ' <span class="text-fg-muted">(' + (f.size / 1024).toFixed(1) + ' KB)</span>';
      list.appendChild(li);
    });
  }

  function escapeHtml(s) {
    return s.replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
})();
