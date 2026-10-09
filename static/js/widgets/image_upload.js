/**
 * image_upload.js -- Chronicle Image Upload Widget
 *
 * Opens the file picker from the "Change" chip on an existing picture (or from
 * the whole placeholder when there is none), uploads the file to the media
 * endpoint, then sets the resulting media path on the entity via the entity
 * image API. While it works a progress bar shows; a refusal shows in the amber
 * bar with "Try again" and leaves the current picture alone.
 *
 * Config (from data-* attributes):
 *   data-endpoint    - Entity image API endpoint (PUT), e.g. /campaigns/:id/entities/:eid/image
 *   data-upload-url  - Media upload endpoint (POST), e.g. /media/upload
 *   data-csrf-token  - CSRF token for mutating requests
 */
Chronicle.register('image-upload', {
  init: function (el, config) {
    var ALLOWED = ['image/jpeg', 'image/png', 'image/webp', 'image/gif'];
    var MAX_BYTES = 10 * 1024 * 1024;
    var busy = false;
    var lastFile = null;
    // Only these nodes belong to the widget; the picture around them stays.
    var mine = [];

    // A hidden file input for the image picker.
    var fileInput = document.createElement('input');
    fileInput.type = 'file';
    fileInput.accept = ALLOWED.join(',');
    fileInput.style.display = 'none';
    el.appendChild(fileInput);
    mine.push(fileInput);

    // Prevent the file input's click from bubbling back up to the trigger,
    // which would re-trigger the handler and cause Firefox to suppress
    // the file picker (recursive dispatch detected as non-user-gesture).
    fileInput.addEventListener('click', function (e) {
      e.stopPropagation();
    });

    // Progress strip: a label and a bar along the bottom edge.
    var strip = document.createElement('div');
    strip.hidden = true;
    strip.setAttribute('role', 'status');
    strip.style.cssText = 'position:absolute;left:0;right:0;bottom:0;padding:10px 12px;' +
      'background:var(--color-card-bg, #fff);border-top:1px solid var(--color-border, #e5e7eb)';
    var label = document.createElement('div');
    label.className = 'ag-lbl';
    var track = document.createElement('div');
    track.className = 'ag-prog';
    track.style.marginTop = '6px';
    var bar = document.createElement('span');
    track.appendChild(bar);
    strip.appendChild(label);
    strip.appendChild(track);
    el.appendChild(strip);
    mine.push(strip);

    // The amber bar is the one place an error shows.
    var warn = document.createElement('div');
    warn.className = 'ag-warn';
    warn.setAttribute('role', 'alert');
    warn.style.cssText = 'position:absolute;left:0;right:0;bottom:0;margin:12px;';
    warn.innerHTML = '<i class="fa-solid fa-triangle-exclamation"></i><span></span>' +
      '<button type="button">Try again</button>';
    el.appendChild(warn);
    mine.push(warn);

    function showWarn(msg) {
      warn.querySelector('span').textContent = msg;
      warn.classList.remove('is-on');
      void warn.offsetWidth;
      warn.classList.add('is-on');
    }

    function setBusy(on, text, pct) {
      busy = on;
      strip.hidden = !on;
      if (on) {
        label.textContent = text;
        bar.style.width = (pct || 0) + '%';
        warn.classList.remove('is-on');
      }
      var chips = el.querySelectorAll('[data-pick]');
      for (var i = 0; i < chips.length; i++) chips[i].disabled = on;
      el.setAttribute('aria-busy', on ? 'true' : 'false');
    }

    // Upload with XMLHttpRequest because fetch cannot report how much of the
    // file has gone. The response is read as JSON so a refusal carries its
    // message.
    function upload(formData) {
      return new Promise(function (resolve, reject) {
        var x = new XMLHttpRequest();
        x.open('POST', config.uploadUrl);
        x.setRequestHeader('Accept', 'application/json');
        var csrf = (Chronicle.getCsrf && Chronicle.getCsrf()) || config.csrfToken || '';
        if (csrf) x.setRequestHeader('X-CSRF-Token', csrf);
        if (x.upload) {
          x.upload.onprogress = function (e) {
            if (!e.lengthComputable) return;
            var pct = Math.round(e.loaded / e.total * 100);
            bar.style.width = pct + '%';
            label.textContent = pct >= 100 ? 'Saving…' : 'Uploading… ' + pct + '%';
          };
        }
        x.onload = function () {
          var body = {};
          try { body = JSON.parse(x.responseText); } catch (e) { body = {}; }
          if (x.status >= 200 && x.status < 300) { resolve(body); return; }
          reject(new Error(body.message || 'The upload did not go through.'));
        };
        x.onerror = function () { reject(new Error('Network error. Check your connection.')); };
        x.send(formData);
      });
    }

    function send(file) {
      lastFile = file;
      setBusy(true, 'Uploading… 0%', 0);

      var formData = new FormData();
      formData.append('file', file);
      formData.append('usage_type', 'entity_image');

      // The campaign id comes from the entity endpoint URL for quota
      // enforcement. Endpoint format: /campaigns/:id/entities/:eid/image
      var campMatch = (config.endpoint || '').match(/\/campaigns\/([^/]+)\//);
      if (campMatch) {
        formData.append('campaign_id', campMatch[1]);
      }

      upload(formData)
        .then(function (data) {
          // Step 2: set the uploaded image path on the entity. Only the one
          // field is sent so nothing else on the page is touched.
          label.textContent = 'Saving…';
          return Chronicle.apiFetch(config.endpoint, {
            method: 'PUT',
            body: { image_path: data.id },
          });
        })
        .then(function (res) {
          if (!res.ok) {
            return res.json().catch(function () { return {}; }).then(function (body) {
              throw new Error(body.message || 'The picture could not be saved.');
            });
          }
          bar.style.width = '100%';
          label.textContent = 'Saved';
          Chronicle.notify('Picture changed.', 'success');
          // The flash shows before the page reloads to draw the new picture.
          setTimeout(function () { window.location.reload(); }, 700);
        })
        .catch(function (err) {
          console.error('[image-upload] Error:', err);
          setBusy(false);
          showWarn((err && err.message) || 'The upload did not go through.');
        });
    }

    function choose(file) {
      if (!file) return;
      if (ALLOWED.indexOf(file.type) === -1) {
        showWarn('Please choose a JPEG, PNG, WebP or GIF picture.');
        return;
      }
      if (file.size > MAX_BYTES) {
        showWarn('That picture is over 10 MB. Choose a smaller one.');
        return;
      }
      send(file);
    }

    // The chip opens the picker; with no chip (the empty placeholder) the
    // whole area does.
    function open(e) {
      e.preventDefault();
      e.stopPropagation();
      if (!busy) fileInput.click();
    }
    var chips = el.querySelectorAll('[data-pick]');
    if (chips.length) {
      for (var i = 0; i < chips.length; i++) chips[i].addEventListener('click', open);
    } else {
      el.addEventListener('click', function (e) {
        if (warn.contains(e.target)) return;
        open(e);
      });
    }

    // Try again re-sends the same file without asking for it a second time.
    warn.querySelector('button').addEventListener('click', function (e) {
      e.preventDefault();
      e.stopPropagation();
      if (lastFile && !busy) send(lastFile);
      else if (!busy) fileInput.click();
    });

    fileInput.addEventListener('change', function () {
      var file = fileInput.files[0];
      fileInput.value = '';
      choose(file);
    });

    el._imageUploadNodes = mine;
  },

  destroy: function (el) {
    (el._imageUploadNodes || []).forEach(function (n) { if (n.parentNode) n.parentNode.removeChild(n); });
    delete el._imageUploadNodes;
  }
});
