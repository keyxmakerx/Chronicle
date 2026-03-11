/**
 * appearance_editor.js -- Campaign Appearance Editor Widget
 *
 * Mounts on a data-widget="appearance-editor" element. Provides live preview
 * for brand name, accent color, and topbar styling. Changes are held locally
 * until the user explicitly clicks "Save Changes".
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

  // Gradient direction values to CSS mappings.
  var GRADIENT_DIR_CSS = {
    'to-r': 'to right',
    'to-br': 'to bottom right',
    'to-b': 'to bottom'
  };

  Chronicle.register('appearance-editor', {
    destroy: function (el) {
      // No timers to clean up in draft mode.
    },
    init: function (el, config) {
      var campaignId = config.campaignId;
      var csrfToken = config.csrf;
      if (!campaignId) {
        console.error('[appearance-editor] Missing data-campaign-id');
        return;
      }

      // --- Saved state (what's currently on the server) ---
      var saved = {
        brandName: config.brandName || '',
        accentColor: config.accentColor || '',
        topbarStyle: { mode: '', color: '', gradient_from: '', gradient_to: '', gradient_dir: 'to-r' }
      };

      // Parse initial topbar style.
      try {
        var parsed = JSON.parse(el.getAttribute('data-topbar-style') || '{}');
        if (parsed && parsed.mode !== undefined) {
          saved.topbarStyle = parsed;
        }
      } catch (e) {
        console.warn('[appearance-editor] Invalid topbar-style JSON, using defaults');
      }

      // --- Draft state (local changes, not yet saved) ---
      var draft = {
        brandName: saved.brandName,
        accentColor: saved.accentColor,
        topbarStyle: {
          mode: saved.topbarStyle.mode || '',
          color: saved.topbarStyle.color || '',
          gradient_from: saved.topbarStyle.gradient_from || '',
          gradient_to: saved.topbarStyle.gradient_to || '',
          gradient_dir: saved.topbarStyle.gradient_dir || 'to-r'
        }
      };

      // --- DOM references ---
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
      var accentContainer = el.querySelector('#appearance-accent-colors');
      var accentLabel = el.querySelector('#appearance-accent-label');

      // Save bar lives outside the widget element (sibling above it).
      var saveBar = document.getElementById('appearance-save-bar');
      var saveBtn = document.getElementById('appearance-save-btn');

      // --- Initialization ---

      // Set initial topbar control values from state.
      if (solidColorInput && draft.topbarStyle.color) {
        solidColorInput.value = draft.topbarStyle.color;
      }
      if (gradFromInput && draft.topbarStyle.gradient_from) {
        gradFromInput.value = draft.topbarStyle.gradient_from;
      }
      if (gradToInput && draft.topbarStyle.gradient_to) {
        gradToInput.value = draft.topbarStyle.gradient_to;
      }
      if (gradDirSelect && draft.topbarStyle.gradient_dir) {
        gradDirSelect.value = draft.topbarStyle.gradient_dir;
      }

      // Set initial active mode and show correct panel.
      setActiveMode(draft.topbarStyle.mode);
      updateTopbarPreview();

      // --- Dirty tracking ---

      function isDirty() {
        return draft.brandName !== saved.brandName ||
               draft.accentColor !== saved.accentColor ||
               draft.topbarStyle.mode !== (saved.topbarStyle.mode || '') ||
               draft.topbarStyle.color !== (saved.topbarStyle.color || '') ||
               draft.topbarStyle.gradient_from !== (saved.topbarStyle.gradient_from || '') ||
               draft.topbarStyle.gradient_to !== (saved.topbarStyle.gradient_to || '') ||
               draft.topbarStyle.gradient_dir !== (saved.topbarStyle.gradient_dir || 'to-r');
      }

      function updateSaveBar() {
        if (!saveBar) return;
        if (isDirty()) {
          saveBar.classList.remove('hidden');
        } else {
          saveBar.classList.add('hidden');
        }
      }

      // --- Brand Name ---

      if (brandInput) {
        brandInput.addEventListener('input', function () {
          draft.brandName = brandInput.value;
          // Live preview.
          if (previewBrand) {
            previewBrand.textContent = brandInput.value || brandInput.placeholder;
          }
          updateSaveBar();
        });
      }

      if (brandClearBtn) {
        brandClearBtn.addEventListener('click', function () {
          if (brandInput) {
            brandInput.value = '';
            draft.brandName = '';
            if (previewBrand) {
              previewBrand.textContent = brandInput.placeholder;
            }
          }
          updateSaveBar();
        });
      }

      // --- Accent Color (JS-driven, no server calls) ---

      if (accentContainer) {
        var accentButtons = accentContainer.querySelectorAll('button[data-accent-color]');
        for (var i = 0; i < accentButtons.length; i++) {
          accentButtons[i].addEventListener('click', function () {
            var color = this.getAttribute('data-accent-color');
            draft.accentColor = color;
            updateAccentHighlight(color);
            updateSaveBar();
          });
        }
      }

      function updateAccentHighlight(selectedColor) {
        if (!accentContainer) return;
        var buttons = accentContainer.querySelectorAll('button[data-accent-color]');
        for (var j = 0; j < buttons.length; j++) {
          var btn = buttons[j];
          var btnColor = btn.getAttribute('data-accent-color');
          var isReset = btnColor === '';

          if (btnColor === selectedColor) {
            if (isReset) {
              btn.className = 'w-8 h-8 rounded-full border-2 border-dashed border-fg ring-2 ring-offset-2 ring-offset-surface ring-fg flex items-center justify-center transition-colors shrink-0';
            } else {
              btn.className = 'w-8 h-8 rounded-full border-2 border-white ring-2 ring-offset-2 ring-offset-surface ring-fg transition-transform hover:scale-110 shrink-0';
            }
          } else {
            if (isReset) {
              btn.className = 'w-8 h-8 rounded-full border-2 border-dashed border-edge flex items-center justify-center hover:border-fg-muted transition-colors shrink-0';
            } else {
              btn.className = 'w-8 h-8 rounded-full border-2 border-transparent hover:border-white/50 transition-transform hover:scale-110 shrink-0';
            }
          }
        }

        // Update label.
        if (accentLabel) {
          accentLabel.textContent = selectedColor ? 'Selected: ' + selectedColor : 'Using default theme color';
        }
      }

      // --- Topbar Mode Buttons ---

      if (modeContainer) {
        var modeButtons = modeContainer.querySelectorAll('button[data-mode]');
        for (var i = 0; i < modeButtons.length; i++) {
          modeButtons[i].addEventListener('click', function () {
            var mode = this.getAttribute('data-mode');
            draft.topbarStyle.mode = mode;
            setActiveMode(mode);
            updateTopbarPreview();
            updateSaveBar();
          });
        }
      }

      // --- Topbar Color Inputs ---

      if (solidColorInput) {
        solidColorInput.addEventListener('input', function () {
          draft.topbarStyle.color = this.value;
          updateTopbarPreview();
          updateSaveBar();
        });
      }

      if (gradFromInput) {
        gradFromInput.addEventListener('input', function () {
          draft.topbarStyle.gradient_from = this.value;
          updateTopbarPreview();
          updateSaveBar();
        });
      }

      if (gradToInput) {
        gradToInput.addEventListener('input', function () {
          draft.topbarStyle.gradient_to = this.value;
          updateTopbarPreview();
          updateSaveBar();
        });
      }

      if (gradDirSelect) {
        gradDirSelect.addEventListener('change', function () {
          draft.topbarStyle.gradient_dir = this.value;
          updateTopbarPreview();
          updateSaveBar();
        });
      }

      // --- Save Button ---

      if (saveBtn) {
        saveBtn.addEventListener('click', function () {
          saveBtn.disabled = true;
          saveBtn.innerHTML = '<i class="fa-solid fa-spinner fa-spin text-xs mr-1"></i> Saving...';

          var pending = 0;
          var failed = false;

          function onComplete() {
            pending--;
            if (pending <= 0) {
              saveBtn.disabled = false;
              if (failed) {
                saveBtn.innerHTML = '<i class="fa-solid fa-check text-xs mr-1"></i> Save Changes';
                Chronicle.notify('Some changes failed to save', 'error');
              } else {
                // Update saved state to match draft.
                saved.brandName = draft.brandName;
                saved.accentColor = draft.accentColor;
                saved.topbarStyle = {
                  mode: draft.topbarStyle.mode,
                  color: draft.topbarStyle.color,
                  gradient_from: draft.topbarStyle.gradient_from,
                  gradient_to: draft.topbarStyle.gradient_to,
                  gradient_dir: draft.topbarStyle.gradient_dir
                };
                updateSaveBar();
                Chronicle.notify('Appearance saved', 'success');
              }
            }
          }

          // Save branding if changed.
          if (draft.brandName !== saved.brandName) {
            pending++;
            Chronicle.apiFetch('/campaigns/' + campaignId + '/branding', {
              method: 'PUT',
              body: { brand_name: draft.brandName },
              csrfToken: csrfToken
            }).then(function (res) {
              if (!res.ok) { failed = true; }
              onComplete();
            }).catch(function () {
              failed = true;
              onComplete();
            });
          }

          // Save accent color if changed (form-encoded for c.FormValue).
          if (draft.accentColor !== saved.accentColor) {
            pending++;
            Chronicle.apiFetch('/campaigns/' + campaignId + '/accent-color', {
              method: 'PUT',
              body: 'accent_color=' + encodeURIComponent(draft.accentColor),
              headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
              csrfToken: csrfToken
            }).then(function (res) {
              if (!res.ok) { failed = true; }
              onComplete();
            }).catch(function () {
              failed = true;
              onComplete();
            });
          }

          // Save topbar style if changed.
          var topbarChanged = draft.topbarStyle.mode !== (saved.topbarStyle.mode || '') ||
                              draft.topbarStyle.color !== (saved.topbarStyle.color || '') ||
                              draft.topbarStyle.gradient_from !== (saved.topbarStyle.gradient_from || '') ||
                              draft.topbarStyle.gradient_to !== (saved.topbarStyle.gradient_to || '') ||
                              draft.topbarStyle.gradient_dir !== (saved.topbarStyle.gradient_dir || 'to-r');
          if (topbarChanged) {
            pending++;
            Chronicle.apiFetch('/campaigns/' + campaignId + '/topbar-style', {
              method: 'PUT',
              body: {
                mode: draft.topbarStyle.mode || '',
                color: draft.topbarStyle.color || '',
                gradient_from: draft.topbarStyle.gradient_from || '',
                gradient_to: draft.topbarStyle.gradient_to || '',
                gradient_dir: draft.topbarStyle.gradient_dir || ''
              },
              csrfToken: csrfToken
            }).then(function (res) {
              if (!res.ok) { failed = true; }
              onComplete();
            }).catch(function () {
              failed = true;
              onComplete();
            });
          }

          // If nothing changed, just hide the bar.
          if (pending === 0) {
            saveBtn.disabled = false;
            saveBtn.innerHTML = '<i class="fa-solid fa-check text-xs mr-1"></i> Save Changes';
            updateSaveBar();
          }
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

        var mode = draft.topbarStyle.mode;
        if (mode === 'solid' && draft.topbarStyle.color) {
          previewTopbar.style.background = draft.topbarStyle.color;
        } else if (mode === 'gradient' && draft.topbarStyle.gradient_from && draft.topbarStyle.gradient_to) {
          var dir = GRADIENT_DIR_CSS[draft.topbarStyle.gradient_dir] || 'to right';
          previewTopbar.style.background = 'linear-gradient(' + dir + ', ' + draft.topbarStyle.gradient_from + ', ' + draft.topbarStyle.gradient_to + ')';
        } else {
          previewTopbar.style.background = '';
        }
      }
    }
  });
})();
