/*
 * vault_import.js -- the drop zone on Manage > Import.
 *
 * The page itself is server-rendered and HTMX-driven (the preview, the Import
 * button and the progress bar are fragments the server sends). This widget
 * only does what the server can't: take a dragged .zip, check its name and
 * size before spending an upload on it, submit the form, and draw the upload
 * progress bar. It writes nothing to the campaign.
 *
 * Mounted by boot.js on [data-widget="vault-import"] (the drop zone form), and
 * again whenever HTMX swaps a new drop zone in.
 */
(function () {
  'use strict';

  // isZipName: only a .zip is accepted; the server checks the contents again.
  function isZipName(name) {
    return /\.zip$/i.test(String(name || ''));
  }

  // formatBytes writes a size the way the rest of the site does.
  function formatBytes(n) {
    n = Number(n);
    if (!isFinite(n) || n < 0) return '';
    if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MB';
    if (n >= 1024) return (n / 1024).toFixed(1) + ' KB';
    return Math.round(n) + ' B';
  }

  // uploadPercent turns XHR progress into a whole percent between 0 and 100.
  function uploadPercent(loaded, total) {
    loaded = Number(loaded);
    total = Number(total);
    if (!isFinite(loaded) || !isFinite(total) || total <= 0) return 0;
    return Math.max(0, Math.min(100, Math.floor((loaded / total) * 100)));
  }

  // uploadLabel: once every byte is sent the server is still reading the zip.
  function uploadLabel(pct) {
    return pct >= 100 ? 'Reading the zip…' : 'Uploading…';
  }

  // checkFile returns a sentence for the owner when a file shouldn't be
  // uploaded, or '' when it is fine to send.
  function checkFile(file, maxBytes) {
    if (!file) return 'Choose a .zip file to import.';
    if (!isZipName(file.name)) return "That isn't a .zip file. Zip the vault folder and try again.";
    if (maxBytes > 0 && file.size > maxBytes) {
      return 'That zip is over ' + Math.floor(maxBytes / 1048576) + ' MB. Import a smaller part of the vault.';
    }
    return '';
  }

  window.Chronicle = window.Chronicle || {};
  Chronicle.VaultImport = {
    isZipName: isZipName,
    formatBytes: formatBytes,
    uploadPercent: uploadPercent,
    uploadLabel: uploadLabel,
    checkFile: checkFile,
  };

  if (!Chronicle.register) return;

  var handlers = new WeakMap();

  Chronicle.register('vault-import', {
    init: function (el, config) {
      var input = el.querySelector('[data-vi-file]');
      var drop = el.querySelector('[data-vi-drop]');
      var title = el.querySelector('[data-vi-title]');
      var sub = el.querySelector('[data-vi-sub]');
      var upload = el.querySelector('[data-vi-upload]');
      var upLabel = el.querySelector('[data-vi-upload-label]');
      var upPct = el.querySelector('[data-vi-upload-pct]');
      var upBar = el.querySelector('[data-vi-upload-bar]');
      if (!input || !drop) return;

      var maxBytes = parseInt(config && config.maxBytes, 10) || 0;
      var originalTitle = title ? title.textContent : '';
      var originalSub = sub ? sub.innerHTML : '';

      function say(text, isError) {
        if (title) title.textContent = text;
        if (sub) sub.textContent = '';
        drop.classList.toggle('is-error', !!isError);
      }

      function submit() {
        if (el.requestSubmit) el.requestSubmit();
        else if (window.htmx) window.htmx.trigger(el, 'submit');
      }

      function take(file) {
        var problem = checkFile(file, maxBytes);
        if (problem) {
          say(problem, true);
          try { input.value = ''; } catch (e) { /* older browsers */ }
          return;
        }
        say(file.name, false);
        if (sub) sub.textContent = formatBytes(file.size);
        submit();
      }

      var on = {
        change: function () { take(input.files && input.files[0]); },
        dragenter: function (e) { e.preventDefault(); drop.classList.add('is-over'); },
        dragover: function (e) { e.preventDefault(); drop.classList.add('is-over'); },
        dragleave: function (e) {
          if (e.relatedTarget && drop.contains(e.relatedTarget)) return;
          drop.classList.remove('is-over');
        },
        drop: function (e) {
          e.preventDefault();
          drop.classList.remove('is-over');
          var files = e.dataTransfer && e.dataTransfer.files;
          if (!files || !files.length) return;
          var problem = checkFile(files[0], maxBytes);
          if (problem) { say(problem, true); return; }
          // Hand the dropped file to the form's own input so the HTMX submit
          // sends it like a chosen one.
          try { input.files = files; } catch (err) { say("Your browser can't take a dropped file here. Use Choose a file.", true); return; }
          take(files[0]);
        },
        loadstart: function () {
          if (upload) upload.hidden = false;
          drop.classList.add('is-busy');
        },
        progress: function (e) {
          var d = e.detail || {};
          var pct = uploadPercent(d.loaded, d.total);
          if (upLabel) upLabel.textContent = uploadLabel(pct);
          if (upPct) upPct.textContent = pct + '%';
          if (upBar) {
            var fill = upBar.firstElementChild;
            if (fill) fill.style.setProperty('--to', String(pct / 100));
            upBar.setAttribute('aria-valuenow', String(pct));
          }
        },
        reset: function () {
          drop.classList.remove('is-busy');
          // Hide the bar and forget the file, so the same zip can be chosen
          // again (a change event does not fire for an unchanged selection).
          if (upload) upload.hidden = true;
          try { input.value = ''; } catch (e) { /* older browsers */ }
          if (title) title.textContent = originalTitle;
          if (sub) sub.innerHTML = originalSub;
        },
      };

      input.addEventListener('change', on.change);
      drop.addEventListener('dragenter', on.dragenter);
      drop.addEventListener('dragover', on.dragover);
      drop.addEventListener('dragleave', on.dragleave);
      drop.addEventListener('drop', on.drop);
      el.addEventListener('htmx:xhr:loadstart', on.loadstart);
      el.addEventListener('htmx:xhr:progress', on.progress);
      // A refused upload (HTMX shows the reason as a toast) puts the zone back.
      el.addEventListener('htmx:responseError', on.reset);
      el.addEventListener('htmx:sendError', on.reset);
      handlers.set(el, { input: input, drop: drop, on: on });
    },

    destroy: function (el) {
      var h = handlers.get(el);
      if (!h) return;
      h.input.removeEventListener('change', h.on.change);
      h.drop.removeEventListener('dragenter', h.on.dragenter);
      h.drop.removeEventListener('dragover', h.on.dragover);
      h.drop.removeEventListener('dragleave', h.on.dragleave);
      h.drop.removeEventListener('drop', h.on.drop);
      el.removeEventListener('htmx:xhr:loadstart', h.on.loadstart);
      el.removeEventListener('htmx:xhr:progress', h.on.progress);
      el.removeEventListener('htmx:responseError', h.on.reset);
      el.removeEventListener('htmx:sendError', h.on.reset);
      handlers.delete(el);
    },
  });
})();
