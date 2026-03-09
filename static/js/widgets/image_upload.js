/**
 * image_upload.js -- Chronicle Image Upload Widget
 *
 * Handles clicking on an image placeholder / overlay to trigger a file
 * upload. Uploads the file to the media endpoint, then sets the resulting
 * media path on the entity via the entity image API.
 *
 * Config (from data-* attributes):
 *   data-endpoint    - Entity image API endpoint (PUT), e.g. /campaigns/:id/entities/:eid/image
 *   data-upload-url  - Media upload endpoint (POST), e.g. /media/upload
 *   data-csrf-token  - CSRF token for mutating requests
 */
Chronicle.register('image-upload', {
  init: function (el, config) {
    // Create a hidden file input for the image picker.
    var fileInput = document.createElement('input');
    fileInput.type = 'file';
    fileInput.accept = 'image/jpeg,image/png,image/webp,image/gif';
    fileInput.style.display = 'none';
    el.appendChild(fileInput);

    // Prevent the file input's click from bubbling back up to el,
    // which would re-trigger the handler and cause Firefox to suppress
    // the file picker (recursive dispatch detected as non-user-gesture).
    fileInput.addEventListener('click', function (e) {
      e.stopPropagation();
    });

    // Clicking the widget area opens the file picker.
    el.addEventListener('click', function (e) {
      e.preventDefault();
      e.stopPropagation();
      fileInput.click();
    });

    // Handle file selection.
    fileInput.addEventListener('change', function () {
      var file = fileInput.files[0];
      if (!file) return;

      // Validate file type client-side.
      var allowed = ['image/jpeg', 'image/png', 'image/webp', 'image/gif'];
      if (allowed.indexOf(file.type) === -1) {
        Chronicle.notify('Please select a JPEG, PNG, WebP, or GIF image.', 'warning');
        fileInput.value = '';
        return;
      }

      // Validate file size client-side (10 MB max).
      if (file.size > 10 * 1024 * 1024) {
        Chronicle.notify('Image must be smaller than 10 MB.', 'warning');
        fileInput.value = '';
        return;
      }

      // Show upload feedback.
      el.style.opacity = '0.6';
      el.style.pointerEvents = 'none';

      // Step 1: Upload the file to the media endpoint.
      var formData = new FormData();
      formData.append('file', file);
      formData.append('usage_type', 'entity_image');

      // Extract campaign_id from the entity endpoint URL for quota enforcement.
      // Endpoint format: /campaigns/:id/entities/:eid/image
      var campMatch = (config.endpoint || '').match(/\/campaigns\/([^/]+)\//);
      if (campMatch) {
        formData.append('campaign_id', campMatch[1]);
      }

      // Use Chronicle.apiFetch so Accept: application/json is sent,
      // ensuring error responses come back as JSON (not HTML error pages).
      Chronicle.apiFetch(config.uploadUrl, {
        method: 'POST',
        body: formData,
      })
        .then(function (res) {
          if (!res.ok) {
            return res.json().catch(function () { return {}; }).then(function (body) {
              throw new Error(body.message || 'Upload failed: ' + res.status);
            });
          }
          return res.json();
        })
        .then(function (data) {
          // Step 2: Set the uploaded image path on the entity.
          return Chronicle.apiFetch(config.endpoint, {
            method: 'PUT',
            body: { image_path: data.id },
          });
        })
        .then(function (res) {
          if (!res.ok) throw new Error('Failed to set entity image: ' + res.status);
          // Reload the page to show the new image.
          window.location.reload();
        })
        .catch(function (err) {
          console.error('[image-upload] Error:', err);
          Chronicle.notify('Image upload failed: ' + (err.message || 'Unknown error'), 'error');
          el.style.opacity = '';
          el.style.pointerEvents = '';
        })
        .finally(function () {
          fileInput.value = '';
        });
    });
  },

  destroy: function (el) {
    el.innerHTML = '';
  }
});
