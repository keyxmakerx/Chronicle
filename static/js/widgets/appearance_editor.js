/**
 * appearance_editor.js -- Campaign Appearance Editor Widget
 *
 * Mounts on a data-widget="appearance-editor" element. Provides live preview
 * and debounced auto-save for brand name, accent color, and topbar styling.
 *
 * Config attributes:
 *   data-campaign-id  -- Campaign ID
 *   data-csrf         -- CSRF token
 *   data-brand-name   -- Current brand name (may be empty)
 *   data-brand-logo   -- Current brand logo path (may be empty)
 *   data-accent-color -- Current accent color hex (may be empty)
 *   data-topbar-style -- Current topbar style JSON (default "{}")
 */
(function () {
  'use strict';

  var DEBOUNCE_MS = 500;

  // Gradient direction values to CSS mappings.
  var GRADIENT_DIR_CSS = {
    'to-r': 'to right',
    'to-br': 'to bottom right',
    'to-b': 'to bottom'
  };

  Chronicle.register('appearance-editor', {
    destroy: function (el) {
      // Clear any pending debounce timers to prevent saves after unmount.
      if (el._appearanceBrandTimer) {
        clearTimeout(el._appearanceBrandTimer);
      }
      if (el._appearanceTopbarTimer) {
        clearTimeout(el._appearanceTopbarTimer);
      }
    },
    init: function (el, config) {
      var campaignId = config.campaignId;
      var csrfToken = config.csrf;
      if (!campaignId) {
        console.error('[appearance-editor] Missing data-campaign-id');
        return;
      }

      // Parse initial topbar style.
      var topbarStyle = { mode: '', color: '', gradient_from: '', gradient_to: '', gradient_dir: 'to-r' };
      try {
        var parsed = JSON.parse(el.getAttribute('data-topbar-style') || '{}');
        if (parsed && parsed.mode) {
          topbarStyle = parsed;
        }
      } catch (e) {
        console.warn('[appearance-editor] Invalid topbar-style JSON, using defaults');
      }

      // DOM references.
      var brandInput = el.querySelector('#appearance-brand-name');
      var brandClearBtn = el.querySelector('#appearance-brand-clear');
      var previewBrand = el.querySelector('#appearance-preview-brand');
      var previewTopbar = el.querySelector('#appearance-preview-topbar');
      var modeContainer = el.querySelector('#appearance-topbar-mode');
      var solidPanel = el.querySelector('#appearance-topbar-solid');
      var gradientPanel = el.querySelector('#appearance-topbar-gradient');
      var solidColorInput = el.querySelector('#appearance-topbar-color');
      var gradFromInput = el.querySelector('#appearance-topbar-gradient-from');
      var gradToInput = el.querySelector('#appearance-topbar-gradient-to');
      var gradDirSelect = el.querySelector('#appearance-topbar-gradient-dir');

      // Timers for debounced saves (stored on element for destroy cleanup).
      el._appearanceBrandTimer = null;
      el._appearanceTopbarTimer = null;

      // --- Initialization ---

      // Set initial topbar control values from state.
      if (solidColorInput && topbarStyle.color) {
        solidColorInput.value = topbarStyle.color;
      }
      if (gradFromInput && topbarStyle.gradient_from) {
        gradFromInput.value = topbarStyle.gradient_from;
      }
      if (gradToInput && topbarStyle.gradient_to) {
        gradToInput.value = topbarStyle.gradient_to;
      }
      if (gradDirSelect && topbarStyle.gradient_dir) {
        gradDirSelect.value = topbarStyle.gradient_dir;
      }

      // Set initial active mode and show correct panel.
      setActiveMode(topbarStyle.mode || '');
      updateTopbarPreview();

      // --- Brand Name ---

      if (brandInput) {
        brandInput.addEventListener('input', function () {
          // Live preview.
          if (previewBrand) {
            previewBrand.textContent = brandInput.value || brandInput.placeholder;
          }
          // Debounced save.
          clearTimeout(el._appearanceBrandTimer);
          el._appearanceBrandTimer = setTimeout(function () {
            saveBranding(brandInput.value);
          }, DEBOUNCE_MS);
        });
      }

      if (brandClearBtn) {
        brandClearBtn.addEventListener('click', function () {
          if (brandInput) {
            brandInput.value = '';
            if (previewBrand) {
              previewBrand.textContent = brandInput.placeholder;
            }
          }
          clearTimeout(el._appearanceBrandTimer);
          saveBranding('');
        });
      }

      // --- Topbar Mode Buttons ---

      if (modeContainer) {
        var modeButtons = modeContainer.querySelectorAll('button[data-mode]');
        for (var i = 0; i < modeButtons.length; i++) {
          modeButtons[i].addEventListener('click', function () {
            var mode = this.getAttribute('data-mode');
            topbarStyle.mode = mode;
            setActiveMode(mode);
            updateTopbarPreview();
            debouncedSaveTopbar();
          });
        }
      }

      // --- Topbar Color Inputs ---

      if (solidColorInput) {
        solidColorInput.addEventListener('input', function () {
          topbarStyle.color = this.value;
          updateTopbarPreview();
          debouncedSaveTopbar();
        });
      }

      if (gradFromInput) {
        gradFromInput.addEventListener('input', function () {
          topbarStyle.gradient_from = this.value;
          updateTopbarPreview();
          debouncedSaveTopbar();
        });
      }

      if (gradToInput) {
        gradToInput.addEventListener('input', function () {
          topbarStyle.gradient_to = this.value;
          updateTopbarPreview();
          debouncedSaveTopbar();
        });
      }

      if (gradDirSelect) {
        gradDirSelect.addEventListener('change', function () {
          topbarStyle.gradient_dir = this.value;
          updateTopbarPreview();
          debouncedSaveTopbar();
        });
      }

      // --- Helper Functions ---

      /**
       * Set the active mode button and show/hide relevant panels.
       */
      function setActiveMode(mode) {
        if (!modeContainer) return;
        var buttons = modeContainer.querySelectorAll('button[data-mode]');
        for (var i = 0; i < buttons.length; i++) {
          var btn = buttons[i];
          if (btn.getAttribute('data-mode') === mode) {
            btn.classList.remove('btn-secondary');
            btn.classList.add('btn-primary');
          } else {
            btn.classList.remove('btn-primary');
            btn.classList.add('btn-secondary');
          }
        }

        // Show/hide panels.
        if (solidPanel) {
          solidPanel.classList.toggle('hidden', mode !== 'solid');
        }
        if (gradientPanel) {
          gradientPanel.classList.toggle('hidden', mode !== 'gradient');
        }
      }

      /**
       * Update the faux topbar preview element to reflect current style.
       */
      function updateTopbarPreview() {
        if (!previewTopbar) return;

        var mode = topbarStyle.mode;
        if (mode === 'solid' && topbarStyle.color) {
          previewTopbar.style.background = topbarStyle.color;
        } else if (mode === 'gradient' && topbarStyle.gradient_from && topbarStyle.gradient_to) {
          var dir = GRADIENT_DIR_CSS[topbarStyle.gradient_dir] || 'to right';
          previewTopbar.style.background = 'linear-gradient(' + dir + ', ' + topbarStyle.gradient_from + ', ' + topbarStyle.gradient_to + ')';
        } else {
          previewTopbar.style.background = '';
        }
      }

      /**
       * Debounced save for topbar style.
       */
      function debouncedSaveTopbar() {
        clearTimeout(el._appearanceTopbarTimer);
        el._appearanceTopbarTimer = setTimeout(function () {
          saveTopbarStyle();
        }, DEBOUNCE_MS);
      }

      /**
       * Save brand name to server.
       */
      function saveBranding(brandName) {
        Chronicle.apiFetch('/campaigns/' + campaignId + '/branding', {
          method: 'PUT',
          body: { brand_name: brandName },
          csrfToken: csrfToken
        }).then(function (res) {
          if (!res.ok) { Chronicle.notify('Failed to save brand name', 'error'); }
        }).catch(function () {
          Chronicle.notify('Failed to save brand name', 'error');
        });
      }

      /**
       * Save topbar style to server.
       */
      function saveTopbarStyle() {
        var body = {
          mode: topbarStyle.mode || '',
          color: topbarStyle.color || '',
          gradient_from: topbarStyle.gradient_from || '',
          gradient_to: topbarStyle.gradient_to || '',
          gradient_dir: topbarStyle.gradient_dir || ''
        };

        Chronicle.apiFetch('/campaigns/' + campaignId + '/topbar-style', {
          method: 'PUT',
          body: body,
          csrfToken: csrfToken
        }).then(function (res) {
          if (!res.ok) { Chronicle.notify('Failed to save topbar style', 'error'); }
        }).catch(function () {
          Chronicle.notify('Failed to save topbar style', 'error');
        });
      }
    }
  });
})();
