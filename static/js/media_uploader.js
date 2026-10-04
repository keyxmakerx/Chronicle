/**
 * media_uploader.js -- Alpine component behind the campaign Media page's
 * upload zone: drag-drop, per-file progress and multi-file upload.
 *
 * Loaded from the layout, before Alpine, rather than inline in the page: htmx
 * strips <script> tags from a boosted swap (allowScriptTags=false), so an
 * inline definition would be missing whenever the page is reached from the
 * sidebar or the Manage tabs.
 */
function mediaUploader(campaignId, csrfToken) {
  var ALLOWED = ['image/jpeg', 'image/png', 'image/webp', 'image/gif'];
  var MAX_SIZE = 10 * 1024 * 1024;

  function formatSize(b) {
    if (b >= 1048576) return (b / 1048576).toFixed(1) + ' MB';
    if (b >= 1024) return (b / 1024).toFixed(1) + ' KB';
    return b + ' B';
  }

  return {
    dragOver: false,
    queue: [],
    _processing: false,

    handleDrop(event) {
      this.dragOver = false;
      var files = event.dataTransfer ? event.dataTransfer.files : [];
      this.handleFiles(files);
    },

    handleFiles(files) {
      if (!files || !files.length) return;
      for (var i = 0; i < files.length; i++) {
        var f = files[i];
        if (!ALLOWED.includes(f.type)) {
          Chronicle.notify(f.name + ': unsupported file type.', 'error');
          continue;
        }
        if (f.size > MAX_SIZE) {
          Chronicle.notify(f.name + ': too large (max 10 MB).', 'error');
          continue;
        }
        this.queue.push({
          file: f,
          name: f.name,
          size: f.size,
          progress: 0,
          status: 'pending',
          statusText: formatSize(f.size)
        });
      }
      this._processQueue();
    },

    async _processQueue() {
      if (this._processing) return;
      this._processing = true;

      while (true) {
        var next = this.queue.find(function(q) { return q.status === 'pending'; });
        if (!next) break;
        await this._uploadOne(next);
      }

      this._processing = false;

      // If all succeeded, reload after a brief pause to show green checks.
      var allDone = this.queue.length > 0 && this.queue.every(function(q) { return q.status === 'done'; });
      if (allDone) {
        var count = this.queue.length;
        Chronicle.notify(count + ' file' + (count > 1 ? 's' : '') + ' uploaded.', 'success');
        setTimeout(function() { window.location.reload(); }, 800);
      }
    },

    _uploadOne(item) {
      var self = this;
      item.status = 'uploading';
      item.statusText = '0%';

      return new Promise(function(resolve) {
        var xhr = new XMLHttpRequest();
        var fd = new FormData();
        fd.append('file', item.file);
        fd.append('campaign_id', campaignId);
        fd.append('usage_type', 'attachment');

        xhr.upload.addEventListener('progress', function(e) {
          if (e.lengthComputable) {
            var pct = Math.round((e.loaded / e.total) * 100);
            item.progress = pct;
            item.statusText = pct + '%';
          }
        });

        xhr.addEventListener('load', function() {
          if (xhr.status >= 200 && xhr.status < 300) {
            item.progress = 100;
            item.status = 'done';
            item.statusText = 'Done';
            // ADR-058 decision 5: the server only ever sets
            // `deduplicated` when the merge was SAFE (the
            // uploader could already see every page using
            // the matched file) -- a refused merge looks
            // exactly like an ordinary upload, on purpose,
            // so there is nothing to detect or report here
            // for that case.
            try {
              var resp = JSON.parse(xhr.responseText);
              if (resp && resp.deduplicated) {
                var pages = (resp.used_by || []).map(function(r) { return r.entity_name; }).join(', ');
                item.statusText = pages ? ('Matched existing file (used on: ' + pages + ')') : 'Matched existing file';
              }
            } catch (_) { /* non-JSON or unexpected shape -- keep 'Done' */ }
          } else {
            item.status = 'error';
            try {
              var err = JSON.parse(xhr.responseText);
              item.statusText = err.message || 'Failed';
            } catch (_) {
              item.statusText = 'Failed';
            }
            Chronicle.notify(item.name + ': ' + item.statusText, 'error');
          }
          resolve();
        });

        xhr.addEventListener('error', function() {
          item.status = 'error';
          item.statusText = 'Network error';
          Chronicle.notify(item.name + ': network error.', 'error');
          resolve();
        });

        xhr.open('POST', '/media/upload');
        xhr.setRequestHeader('X-CSRF-Token', csrfToken);
        xhr.send(fd);
      });
    }
  };
}

