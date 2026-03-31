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
        fontFamily: config.fontFamily || '',
        topbarStyle: { mode: '', color: '', gradient_from: '', gradient_to: '', gradient_dir: 'to-r', image_path: '' },
        topbarContent: { mode: 'none', links: [], quote: '' }
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

      // Parse initial topbar content.
      try {
        var parsedContent = JSON.parse(el.getAttribute('data-topbar-content') || '{}');
        if (parsedContent && parsedContent.mode !== undefined) {
          saved.topbarContent = parsedContent;
        }
      } catch (e) {
        console.warn('[appearance-editor] Invalid topbar-content JSON, using defaults');
      }

      // --- Draft state (local changes, not yet saved) ---
      var draft = {
        brandName: saved.brandName,
        accentColor: saved.accentColor,
        fontFamily: saved.fontFamily,
        topbarStyle: {
          mode: saved.topbarStyle.mode || '',
          color: saved.topbarStyle.color || '',
          gradient_from: saved.topbarStyle.gradient_from || '',
          gradient_to: saved.topbarStyle.gradient_to || '',
          gradient_dir: saved.topbarStyle.gradient_dir || 'to-r'
        },
        topbarContent: {
          mode: saved.topbarContent.mode || 'none',
          links: JSON.parse(JSON.stringify(saved.topbarContent.links || [])),
          quote: saved.topbarContent.quote || ''
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
      var imagePanel = el.querySelector('#appearance-topbar-image');

      // Dynamically add "Image" button and upload panel if not in template.
      if (modeContainer && !modeContainer.querySelector('[data-mode="image"]')) {
        var imgBtn = document.createElement('button');
        imgBtn.type = 'button';
        imgBtn.className = 'btn-secondary text-xs px-3 py-1.5';
        imgBtn.setAttribute('data-mode', 'image');
        imgBtn.textContent = 'Image';
        modeContainer.appendChild(imgBtn);
      }
      if (!imagePanel && modeContainer) {
        imagePanel = document.createElement('div');
        imagePanel.id = 'appearance-topbar-image';
        imagePanel.className = 'hidden space-y-2';
        imagePanel.innerHTML =
          '<p class="text-xs text-fg-secondary">Upload a background image for the topbar.</p>' +
          '<div class="flex items-center gap-3">' +
          '  <label class="btn-secondary text-xs cursor-pointer">' +
          '    <i class="fa-solid fa-upload mr-1.5"></i>Choose Image' +
          '    <input type="file" accept="image/*" class="hidden" id="topbar-image-input"/>' +
          '  </label>' +
          '  <button type="button" id="topbar-image-remove" class="text-xs text-fg-muted hover:text-rose-400 transition-colors">' +
          '    <i class="fa-solid fa-trash mr-1"></i>Remove' +
          '  </button>' +
          '</div>';
        // Insert after gradient panel.
        if (gradientPanel) {
          gradientPanel.parentNode.insertBefore(imagePanel, gradientPanel.nextSibling);
        }

        // Wire upload handler.
        var imgInput = imagePanel.querySelector('#topbar-image-input');
        if (imgInput) {
          imgInput.addEventListener('change', function () {
            var file = imgInput.files[0];
            if (!file) return;
            var form = new FormData();
            form.append('file', file);
            fetch('/campaigns/' + config.campaignId + '/topbar-image', {
              method: 'POST', body: form, credentials: 'same-origin',
              headers: { 'X-CSRF-Token': Chronicle.getCsrf() }
            }).then(function (res) {
              if (res.ok) {
                Chronicle.notify('Topbar image uploaded — reloading...', 'success');
                setTimeout(function () { window.location.reload(); }, 600);
              } else {
                Chronicle.notify('Failed to upload image', 'error');
              }
            });
          });
        }
        var imgRemove = imagePanel.querySelector('#topbar-image-remove');
        if (imgRemove) {
          imgRemove.addEventListener('click', function () {
            Chronicle.apiFetch('/campaigns/' + config.campaignId + '/topbar-image', { method: 'DELETE' })
              .then(function (res) {
                if (res.ok) {
                  draft.topbarStyle.mode = '';
                  draft.topbarStyle.image_path = '';
                  setActiveMode('');
                  updateTopbarPreview();
                  Chronicle.notify('Topbar image removed', 'success');
                  setTimeout(function () { window.location.reload(); }, 600);
                }
              });
          });
        }
      }
      var solidColorInput = el.querySelector('#appearance-topbar-color');
      var gradFromInput = el.querySelector('#appearance-topbar-gradient-from');
      var gradToInput = el.querySelector('#appearance-topbar-gradient-to');
      var gradDirSelect = el.querySelector('#appearance-topbar-gradient-dir');
      var accentContainer = el.querySelector('#appearance-accent-colors');
      var accentLabel = el.querySelector('#appearance-accent-label');

      // Preview elements for live accent/font/backdrop updates.
      var previewBtnPrimary = el.querySelector('#appearance-preview-btn-primary');
      var previewLink = el.querySelector('#appearance-preview-link');
      var previewBadge = el.querySelector('#appearance-preview-badge');
      var previewAvatar = el.querySelector('#appearance-preview-avatar');
      var previewSidebarActive = el.querySelector('#appearance-preview-sidebar-active');
      var previewCat1 = el.querySelector('#appearance-preview-cat1');
      var previewCat2 = el.querySelector('#appearance-preview-cat2');
      var previewCat3 = el.querySelector('#appearance-preview-cat3');
      var previewContent = el.querySelector('#appearance-preview-content');
      var previewBackdrop = el.querySelector('#appearance-preview-backdrop');
      var previewBackdropImg = el.querySelector('#appearance-preview-backdrop-img');

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

      // Font family CSS map (mirrors Go fontFamilyPresets). Declared here so
      // it is initialized before updateFontPreview is called during init.
      var FONT_CSS_MAP = {
        '': 'inherit',
        'serif': "Georgia, 'Times New Roman', serif",
        'sans-serif': "'Inter', system-ui, sans-serif",
        'monospace': "'JetBrains Mono', 'Fira Code', monospace",
        'georgia': 'Georgia, Cambria, serif',
        'merriweather': "'Merriweather', Georgia, serif"
      };

      // Set initial active mode and show correct panel.
      setActiveMode(draft.topbarStyle.mode);
      updateTopbarPreview();
      updateAccentPreview(draft.accentColor);
      updateFontPreview(draft.fontFamily);
      initBackdropPreview();

      // --- Dirty tracking ---

      function isDirty() {
        return draft.brandName !== saved.brandName ||
               draft.accentColor !== saved.accentColor ||
               draft.fontFamily !== saved.fontFamily ||
               draft.topbarStyle.mode !== (saved.topbarStyle.mode || '') ||
               draft.topbarStyle.color !== (saved.topbarStyle.color || '') ||
               draft.topbarStyle.gradient_from !== (saved.topbarStyle.gradient_from || '') ||
               draft.topbarStyle.gradient_to !== (saved.topbarStyle.gradient_to || '') ||
               draft.topbarStyle.gradient_dir !== (saved.topbarStyle.gradient_dir || 'to-r') ||
               draft.topbarContent.mode !== (saved.topbarContent.mode || 'none') ||
               draft.topbarContent.quote !== (saved.topbarContent.quote || '') ||
               JSON.stringify(draft.topbarContent.links) !== JSON.stringify(saved.topbarContent.links || []);
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
            updateAccentPreview(color);
            // Sync the custom color picker value when a preset is chosen.
            var customInput = el.querySelector('#appearance-accent-custom');
            if (customInput && color) {
              customInput.value = color;
            }
            updateSaveBar();
          });
        }

        // Custom hex color picker input.
        var customColorInput = el.querySelector('#appearance-accent-custom');
        if (customColorInput) {
          customColorInput.addEventListener('input', function () {
            draft.accentColor = this.value;
            updateAccentHighlight(this.value);
            updateAccentPreview(this.value);
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

      // --- Font Family Buttons ---

      var fontContainer = el.querySelector('#appearance-font-family');
      if (fontContainer) {
        var fontButtons = fontContainer.querySelectorAll('button[data-font-family]');
        for (var i = 0; i < fontButtons.length; i++) {
          fontButtons[i].addEventListener('click', function () {
            draft.fontFamily = this.getAttribute('data-font-family');
            updateFontPreview(draft.fontFamily);
            // Update button highlighting.
            var allBtns = fontContainer.querySelectorAll('button[data-font-family]');
            for (var j = 0; j < allBtns.length; j++) {
              var btn = allBtns[j];
              var isSelected = btn.getAttribute('data-font-family') === draft.fontFamily;
              if (isSelected) {
                btn.className = btn.className.replace(/border-edge bg-surface hover:border-accent\/30 text-fg-secondary/, 'border-accent bg-accent/10 text-accent font-medium');
                if (btn.className.indexOf('border-accent') === -1) {
                  btn.className = 'px-3 py-2 rounded-lg border border-accent bg-accent/10 text-accent font-medium text-sm';
                  btn.style.fontFamily = btn.style.fontFamily; // preserve
                }
              } else {
                btn.className = 'px-3 py-2 rounded-lg border border-edge bg-surface hover:border-accent/30 text-fg-secondary text-sm';
                btn.style.fontFamily = btn.style.fontFamily; // preserve
              }
            }
            updateSaveBar();
          });
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

      // --- Topbar Content ---

      var contentModeContainer = el.querySelector('#appearance-topbar-content-mode');
      var linksPanel = el.querySelector('#appearance-topbar-links');
      var quotePanel = el.querySelector('#appearance-topbar-quote');
      var linksList = el.querySelector('#appearance-topbar-links-list');
      var addLinkBtn = el.querySelector('#appearance-add-topbar-link');
      var quoteTextarea = el.querySelector('#appearance-topbar-quote-text');
      var quoteCounter = el.querySelector('#appearance-topbar-quote-counter');

      function setActiveContentMode(mode) {
        if (!contentModeContainer) return;
        var buttons = contentModeContainer.querySelectorAll('button[data-content-mode]');
        for (var i = 0; i < buttons.length; i++) {
          var isActive = buttons[i].getAttribute('data-content-mode') === mode;
          buttons[i].className = isActive ? 'btn-primary text-xs px-3 py-1.5' : 'btn-secondary text-xs px-3 py-1.5';
        }
        if (linksPanel) linksPanel.classList.toggle('hidden', mode !== 'links');
        if (quotePanel) quotePanel.classList.toggle('hidden', mode !== 'quote');
      }

      function renderLinksList() {
        if (!linksList) return;
        var html = '';
        for (var i = 0; i < draft.topbarContent.links.length; i++) {
          var link = draft.topbarContent.links[i];
          html += '<div class="flex items-center gap-2" data-link-idx="' + i + '">' +
            '<input type="text" class="input text-xs flex-1" placeholder="Label" value="' + Chronicle.escapeAttr(link.label || '') + '" data-link-field="label"/>' +
            '<input type="text" class="input text-xs flex-1" placeholder="/path or URL" value="' + Chronicle.escapeAttr(link.url || '') + '" data-link-field="url"/>' +
            '<input type="text" class="input text-xs w-24" placeholder="fa-icon" value="' + Chronicle.escapeAttr(link.icon || '') + '" data-link-field="icon"/>' +
            '<button type="button" class="btn-ghost btn-icon text-red-500 hover:text-red-600 shrink-0" data-remove-link="' + i + '" title="Remove">' +
              '<i class="fa-solid fa-xmark text-xs"></i></button>' +
            '</div>';
        }
        linksList.innerHTML = html;

        // Bind input change listeners.
        linksList.querySelectorAll('input[data-link-field]').forEach(function (inp) {
          inp.addEventListener('input', function () {
            var idx = parseInt(inp.closest('[data-link-idx]').getAttribute('data-link-idx'));
            var field = inp.getAttribute('data-link-field');
            if (draft.topbarContent.links[idx]) {
              draft.topbarContent.links[idx][field] = inp.value;
              updateSaveBar();
            }
          });
        });

        // Bind remove buttons.
        linksList.querySelectorAll('button[data-remove-link]').forEach(function (btn) {
          btn.addEventListener('click', function () {
            var idx = parseInt(btn.getAttribute('data-remove-link'));
            draft.topbarContent.links.splice(idx, 1);
            renderLinksList();
            updateSaveBar();
          });
        });
      }

      // Mode selector buttons.
      if (contentModeContainer) {
        contentModeContainer.querySelectorAll('button[data-content-mode]').forEach(function (btn) {
          btn.addEventListener('click', function () {
            draft.topbarContent.mode = btn.getAttribute('data-content-mode');
            setActiveContentMode(draft.topbarContent.mode);
            updateSaveBar();
          });
        });
      }

      // Add link button.
      if (addLinkBtn) {
        addLinkBtn.addEventListener('click', function () {
          draft.topbarContent.links.push({ label: '', url: '', icon: '' });
          renderLinksList();
          updateSaveBar();
        });
      }

      // Quote textarea.
      if (quoteTextarea) {
        quoteTextarea.value = draft.topbarContent.quote || '';
        if (quoteCounter) quoteCounter.textContent = quoteTextarea.value.length + ' / 200';
        quoteTextarea.addEventListener('input', function () {
          draft.topbarContent.quote = quoteTextarea.value;
          if (quoteCounter) quoteCounter.textContent = quoteTextarea.value.length + ' / 200';
          updateSaveBar();
        });
      }

      // Initialize mode and link list.
      setActiveContentMode(draft.topbarContent.mode);
      renderLinksList();

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
              saveBtn.innerHTML = '<i class="fa-solid fa-check text-xs mr-1"></i> Save Changes';
              if (failed) {
                Chronicle.notify('Some changes failed to save', 'error');
              } else {
                // Update saved state to match draft.
                saved.brandName = draft.brandName;
                saved.accentColor = draft.accentColor;
                saved.fontFamily = draft.fontFamily;
                saved.topbarStyle = {
                  mode: draft.topbarStyle.mode,
                  color: draft.topbarStyle.color,
                  gradient_from: draft.topbarStyle.gradient_from,
                  gradient_to: draft.topbarStyle.gradient_to,
                  gradient_dir: draft.topbarStyle.gradient_dir
                };
                saved.topbarContent = {
                  mode: draft.topbarContent.mode,
                  links: JSON.parse(JSON.stringify(draft.topbarContent.links)),
                  quote: draft.topbarContent.quote
                };
                updateSaveBar();
                // Apply accent color to page CSS custom properties live.
                applyAccentToPage(draft.accentColor);
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

          // Save font family if changed.
          if (draft.fontFamily !== saved.fontFamily) {
            pending++;
            Chronicle.apiFetch('/campaigns/' + campaignId + '/font-family', {
              method: 'PUT',
              body: { font_family: draft.fontFamily },
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

          // Save topbar content if changed.
          var contentChanged = draft.topbarContent.mode !== (saved.topbarContent.mode || 'none') ||
                               draft.topbarContent.quote !== (saved.topbarContent.quote || '') ||
                               JSON.stringify(draft.topbarContent.links) !== JSON.stringify(saved.topbarContent.links || []);
          if (contentChanged) {
            pending++;
            Chronicle.apiFetch('/campaigns/' + campaignId + '/topbar-content', {
              method: 'PUT',
              body: {
                mode: draft.topbarContent.mode || 'none',
                links: draft.topbarContent.links || [],
                quote: draft.topbarContent.quote || ''
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
        if (imagePanel) {
          imagePanel.classList.toggle('hidden', mode !== 'image');
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
          // Use light text on dark topbar backgrounds.
          previewTopbar.style.color = isLightColor(draft.topbarStyle.color) ? '' : '#f9fafb';
        } else if (mode === 'gradient' && draft.topbarStyle.gradient_from && draft.topbarStyle.gradient_to) {
          var dir = GRADIENT_DIR_CSS[draft.topbarStyle.gradient_dir] || 'to right';
          previewTopbar.style.background = 'linear-gradient(' + dir + ', ' + draft.topbarStyle.gradient_from + ', ' + draft.topbarStyle.gradient_to + ')';
          previewTopbar.style.color = isLightColor(draft.topbarStyle.gradient_from) ? '' : '#f9fafb';
        } else if (mode === 'image' && draft.topbarStyle.image_path) {
          previewTopbar.style.background = 'url(/media/' + draft.topbarStyle.image_path + ') center/cover no-repeat';
          previewTopbar.style.color = '#f9fafb';
        } else {
          previewTopbar.style.background = '';
          previewTopbar.style.color = '';
        }
      }

      /**
       * Update all accent-colored elements in the preview.
       */
      function updateAccentPreview(color) {
        var accent = color || '#6366f1'; // fallback to default indigo
        var accentLight = accent + '20'; // 12% opacity for backgrounds

        if (previewBtnPrimary) {
          previewBtnPrimary.style.backgroundColor = accent;
        }
        if (previewLink) {
          previewLink.style.color = accent;
        }
        if (previewBadge) {
          previewBadge.style.backgroundColor = accentLight;
          previewBadge.style.color = accent;
        }
        if (previewAvatar) {
          previewAvatar.style.backgroundColor = accent;
        }
        if (previewSidebarActive) {
          previewSidebarActive.style.backgroundColor = '#2d2f3a';
          previewSidebarActive.style.borderLeft = '2px solid ' + accent;
        }
        // Category icon circles.
        var catEls = [previewCat1, previewCat2, previewCat3];
        for (var i = 0; i < catEls.length; i++) {
          if (catEls[i]) {
            catEls[i].style.backgroundColor = accentLight;
            catEls[i].style.color = accent;
          }
        }
      }

      /**
       * Update the preview content font family.
       */
      function updateFontPreview(family) {
        if (!previewContent) return;
        var css = FONT_CSS_MAP[family] || 'inherit';
        previewContent.style.fontFamily = css;
      }

      /**
       * Initialize backdrop preview from the existing backdrop section.
       */
      function initBackdropPreview() {
        if (!previewBackdrop) return;
        // Check if there's an existing backdrop image on the page.
        var backdropSection = el.querySelector('#backdrop-section img');
        if (backdropSection && backdropSection.src) {
          previewBackdropImg.src = backdropSection.src;
          previewBackdrop.style.display = '';
        }

        // Watch for HTMX swaps that update the backdrop section.
        document.body.addEventListener('htmx:afterSwap', function (evt) {
          if (!evt.detail || !evt.detail.target) return;
          if (evt.detail.target.id === 'backdrop-section' || (evt.detail.target.closest && evt.detail.target.closest('#backdrop-section'))) {
            var newImg = el.querySelector('#backdrop-section img');
            if (newImg && newImg.src) {
              previewBackdropImg.src = newImg.src;
              previewBackdrop.style.display = '';
            } else {
              previewBackdrop.style.display = 'none';
            }
          }
        });
      }

      /**
       * Apply accent color to the page's CSS custom properties so the change
       * is visible site-wide without a full page reload.
       */
      function applyAccentToPage(hex) {
        var root = document.documentElement;
        if (!hex) {
          // Reset to defaults (indigo-500).
          root.style.removeProperty('--color-accent');
          root.style.removeProperty('--color-accent-hover');
          root.style.removeProperty('--color-accent-light');
          root.style.removeProperty('--color-accent-rgb');
          root.style.removeProperty('--color-accent-hover-rgb');
          root.style.removeProperty('--color-accent-light-rgb');
          return;
        }
        var r = parseInt(hex.slice(1, 3), 16);
        var g = parseInt(hex.slice(3, 5), 16);
        var b = parseInt(hex.slice(5, 7), 16);
        if (isNaN(r)) return;
        // Base color as RGB triplet for Tailwind.
        root.style.setProperty('--color-accent', hex);
        // Hover: darken by ~12%.
        var hr = Math.max(0, Math.min(255, Math.round(r * 0.88)));
        var hg = Math.max(0, Math.min(255, Math.round(g * 0.88)));
        var hb = Math.max(0, Math.min(255, Math.round(b * 0.88)));
        root.style.setProperty('--color-accent-hover', '#' + toHex(hr) + toHex(hg) + toHex(hb));
        // Light: blend toward white by ~60%.
        var lr = Math.max(0, Math.min(255, Math.round(r + (255 - r) * 0.6)));
        var lg = Math.max(0, Math.min(255, Math.round(g + (255 - g) * 0.6)));
        var lb = Math.max(0, Math.min(255, Math.round(b + (255 - b) * 0.6)));
        root.style.setProperty('--color-accent-light', '#' + toHex(lr) + toHex(lg) + toHex(lb));
        // RGB triplets for Tailwind's color-accent utilities (bg-accent, text-accent, etc.)
        root.style.setProperty('--color-accent-rgb', r + ' ' + g + ' ' + b);
        root.style.setProperty('--color-accent-hover-rgb', hr + ' ' + hg + ' ' + hb);
        root.style.setProperty('--color-accent-light-rgb', lr + ' ' + lg + ' ' + lb);
      }

      function toHex(n) {
        var h = n.toString(16);
        return h.length < 2 ? '0' + h : h;
      }

      /**
       * Returns true if a hex color is light (should use dark text on it).
       */
      function isLightColor(hex) {
        if (!hex || hex.length < 7) return true;
        var r = parseInt(hex.slice(1, 3), 16);
        var g = parseInt(hex.slice(3, 5), 16);
        var b = parseInt(hex.slice(5, 7), 16);
        // Perceived luminance formula.
        var luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
        return luminance > 0.6;
      }
    }
  });
})();
