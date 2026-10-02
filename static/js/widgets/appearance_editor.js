/**
 * appearance_editor.js -- Campaign Appearance Editor Widget
 *
 * Mounts on data-widget="appearance-editor". Live-previews brand name/logo,
 * topbar styling, and five accent slots: data-accent-color (site-wide),
 * data-accent-action (primary buttons/hover/FABs), data-accent-app
 * (character pages, calendar app), and the legacy data-accent-surface-1/2
 * pair (content-surface primary/secondary). Every field stages into one
 * local draft until the user clicks "Save Changes"; a "Discard" button
 * reverts every control back to the last save. Save issues its PUTs one at a
 * time (runSaveStepsSequentially): every Update* service method is a
 * read-modify-write on the whole settings blob, so two in flight together
 * can silently drop whichever one's write lands first.
 */
(function () {
  'use strict';

  // Gradient direction values to CSS mappings.
  var GRADIENT_DIR_CSS = {
    'to-r': 'to right',
    'to-br': 'to bottom right',
    'to-b': 'to bottom'
  };

  // runSaveStepsSequentially(steps) -- pure (no DOM, no Chronicle): given an
  // array of { name, run }, where run() returns a promise resolving to
  // something with an `ok` boolean (matching Chronicle.apiFetch's response)
  // or rejecting, executes them ONE AT A TIME rather than concurrently.
  // Every Update* service method this widget's Save calls is a
  // read-modify-write on the campaign's whole settings blob (find, change
  // one field, write the whole thing back) with no locking, so two of them
  // in flight at once can race: the one whose write lands second overwrites
  // the first's change with a copy of the settings read before it happened.
  // Chaining removes the race by construction — each step's write is on
  // disk before the next step reads. Resolves with which step names
  // succeeded and which failed, never rejects, so the caller can apply only
  // the fields that actually saved.
  function runSaveStepsSequentially(steps) {
    var succeeded = [];
    var failed = [];
    return steps.reduce(function (chain, step) {
      return chain.then(function () {
        return step.run().then(function (res) {
          if (res && res.ok) {
            succeeded.push(step.name);
          } else {
            failed.push(step.name);
          }
        }).catch(function () {
          failed.push(step.name);
        });
      });
    }, Promise.resolve()).then(function () {
      return { succeeded: succeeded, failed: failed };
    });
  }

  // refreshTopbar() redraws the live header's background and centre content
  // from a fresh server render of this page. The header sits outside
  // #main-content, so boosted navigation never redraws it and a saved change
  // would otherwise only show after a full reload. Swapping the server's own
  // markup keeps link sanitising and image URLs in one place (Topbar()).
  function refreshTopbar() {
    return fetch(window.location.href, { credentials: 'same-origin', headers: { 'Accept': 'text/html' } })
      .then(function (res) { return res.ok ? res.text() : Promise.reject(res.status); })
      .then(function (html) {
        var doc = new DOMParser().parseFromString(html, 'text/html');
        ['topbar-bg', 'topbar-content'].forEach(function (id) {
          var fresh = doc.getElementById(id);
          var live = document.getElementById(id);
          if (fresh && live) live.innerHTML = fresh.innerHTML;
        });
      })
      .catch(function () { /* the next full load shows it; the save itself succeeded */ });
  }
  Chronicle.refreshTopbar = refreshTopbar;

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
        // Action highlight and App accent slots, read from the
        // data-accent-action/-app attributes; config keys are the camelCase
        // form of the dash-case data attrs (Chronicle.register's config
        // parser convention, same as accentColor).
        accentAction: config.accentAction || '',
        accentApp: config.accentApp || '',
        // Legacy surface-accent pair. Read via getAttribute, not config:
        // boot.js's kebab->camelCase conversion only folds a hyphen before a
        // LETTER, so "data-accent-surface-1" would not become
        // config.accentSurface1 (same reason data-topbar-style below is read
        // this way rather than through config).
        accentSurface1: el.getAttribute('data-accent-surface-1') || '',
        accentSurface2: el.getAttribute('data-accent-surface-2') || '',
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
        accentAction: saved.accentAction,
        accentApp: saved.accentApp,
        accentSurface1: saved.accentSurface1,
        accentSurface2: saved.accentSurface2,
        fontFamily: saved.fontFamily,
        topbarStyle: {
          mode: saved.topbarStyle.mode || '',
          color: saved.topbarStyle.color || '',
          gradient_from: saved.topbarStyle.gradient_from || '',
          gradient_to: saved.topbarStyle.gradient_to || '',
          gradient_dir: saved.topbarStyle.gradient_dir || 'to-r',
          // Carry image_path through the draft so the preview can show the
          // saved topbar image and a Save that includes topbar-style does
          // not blank it out.
          image_path: saved.topbarStyle.image_path || ''
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

      // The "Image" mode button and upload panel are rendered by the templ
      // (branding.templ Top Bar Style card + TopbarImageSection). Upload and
      // remove are HTMX-driven (hx-post/hx-delete swapping
      // #appearance-topbar-image-section in place); the only JS wiring left
      // is syncing draft/saved image state after each swap (see the
      // htmx:afterSwap listener further down).
      var solidColorInput = el.querySelector('#appearance-topbar-color');
      var gradFromInput = el.querySelector('#appearance-topbar-gradient-from');
      var gradToInput = el.querySelector('#appearance-topbar-gradient-to');
      var gradDirSelect = el.querySelector('#appearance-topbar-gradient-dir');

      // Preview elements for live accent/font/backdrop updates. The primary
      // button follows the Action slot (mirrors the .btn-primary CSS swap in
      // input.css) and the "Characters" category chip follows the App slot,
      // instead of both trailing the site accent like every other element.
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
      var discardBtn = document.getElementById('appearance-discard-btn');

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
      initTopbarImageSync();

      // --- Dirty tracking ---

      function isDirty() {
        return draft.brandName !== saved.brandName ||
               draft.accentColor !== saved.accentColor ||
               draft.accentAction !== saved.accentAction ||
               draft.accentApp !== saved.accentApp ||
               draft.accentSurface1 !== saved.accentSurface1 ||
               draft.accentSurface2 !== saved.accentSurface2 ||
               draft.fontFamily !== saved.fontFamily ||
               draft.topbarStyle.mode !== (saved.topbarStyle.mode || '') ||
               draft.topbarStyle.color !== (saved.topbarStyle.color || '') ||
               draft.topbarStyle.gradient_from !== (saved.topbarStyle.gradient_from || '') ||
               draft.topbarStyle.gradient_to !== (saved.topbarStyle.gradient_to || '') ||
               draft.topbarStyle.gradient_dir !== (saved.topbarStyle.gradient_dir || 'to-r') ||
               draft.topbarStyle.image_path !== (saved.topbarStyle.image_path || '') ||
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

      // --- Site accent / Action highlight / App accent pickers ---
      //
      // All three slots share this generic wiring, since their
      // swatch/reset/custom-picker markup and click behavior are identical
      // (semanticAccentPicker), just scoped to a different container id and
      // draft field. Returns a highlight(color) function so callers (Discard,
      // below) can re-sync the swatch ring when the value changes elsewhere.
      function wireSemanticSlotPicker(containerId, customId, labelId, getDraft, setDraft, onChange) {
        var container = el.querySelector('#' + containerId);
        var custom = el.querySelector('#' + customId);
        var label = el.querySelector('#' + labelId);

        function highlight(selectedColor) {
          if (!container) return;
          var buttons = container.querySelectorAll('button[data-slot-color]');
          for (var j = 0; j < buttons.length; j++) {
            var btn = buttons[j];
            var btnColor = btn.getAttribute('data-slot-color');
            var isReset = btnColor === '';
            if (btnColor === selectedColor) {
              btn.className = isReset
                ? 'w-8 h-8 rounded-full border-2 border-dashed border-fg ring-2 ring-offset-2 ring-offset-surface ring-fg flex items-center justify-center transition-colors shrink-0'
                : 'w-8 h-8 rounded-full border-2 border-white ring-2 ring-offset-2 ring-offset-surface ring-fg transition-transform hover:scale-110 shrink-0';
            } else {
              btn.className = isReset
                ? 'w-8 h-8 rounded-full border-2 border-dashed border-edge flex items-center justify-center hover:border-fg-muted transition-colors shrink-0'
                : 'w-8 h-8 rounded-full border-2 border-transparent hover:border-white/50 transition-transform hover:scale-110 shrink-0';
            }
          }
          if (label) {
            label.textContent = selectedColor ? 'Current: ' + selectedColor : 'Using default theme color';
          }
        }

        if (container) {
          var buttons = container.querySelectorAll('button[data-slot-color]');
          for (var i = 0; i < buttons.length; i++) {
            buttons[i].addEventListener('click', function () {
              var color = this.getAttribute('data-slot-color');
              setDraft(color);
              highlight(color);
              // Keep the native color-input swatch in sync in both
              // directions: a preset shows its own color, Reset shows the
              // templ-rendered default rather than the last custom value
              // (otherwise the swatch visually contradicts the "Using
              // default theme color" label right below it).
              if (custom) custom.value = color || '#6366f1';
              onChange(color);
              updateSaveBar();
            });
          }
        }
        if (custom) {
          custom.addEventListener('input', function () {
            setDraft(this.value);
            highlight(this.value);
            onChange(this.value);
            updateSaveBar();
          });
        }

        highlight(getDraft());
        return highlight;
      }

      var highlightAccent = wireSemanticSlotPicker(
        'appearance-accent-colors', 'appearance-accent-custom', 'appearance-accent-label',
        function () { return draft.accentColor; },
        function (v) { draft.accentColor = v; },
        updateAccentPreview
      );
      var highlightAction = wireSemanticSlotPicker(
        'appearance-action-colors', 'appearance-action-custom', 'appearance-action-label',
        function () { return draft.accentAction; },
        function (v) { draft.accentAction = v; },
        function () { updateActionPreview(); }
      );
      var highlightApp = wireSemanticSlotPicker(
        'appearance-app-colors', 'appearance-app-custom', 'appearance-app-label',
        function () { return draft.accentApp; },
        function (v) { draft.accentApp = v; },
        function () { updateAppPreview(); }
      );

      // --- Surface Accents (legacy pair) ---
      //
      // The card's preset/reset/custom controls apply their own instant
      // CSS-variable preview via an inline onclick/onchange (they must stay
      // inline, not a delegated <script>, since htmx strips a <script> tag
      // from a boosted sidebar swap — surface_accent_onclick.go), scoped to
      // this widget element so the "Sample character header" card inherits
      // it (it's a descendant) without the color leaking to the rest of the
      // page. They then hand the chosen value to this widget through a DOM
      // event, because the inline handler runs outside this closure, folding
      // the slot into the same staged draft/save/discard flow as every
      // other field. The widget-level preview is cleared — never carried to
      // <html> — on both Discard and a successful Save (Save applies the
      // real, site-wide property instead; see the Save button below).
      function clearSurfaceVarPreview(slot) {
        el.style.removeProperty('--color-accent-surface-' + slot);
      }

      // highlightSurfaceRow syncs one surface row's ring, custom-picker
      // swatch, and label to selectedColor. Reset never changes appearance
      // (surfaceAccentRow renders it with a single static class, unlike the
      // semantic pickers' reset button), so only preset buttons are touched.
      function highlightSurfaceRow(slot, selectedColor) {
        var row = el.querySelector('#appearance-surface-row-' + slot);
        if (row) {
          var buttons = row.querySelectorAll('button[data-slot-color]');
          for (var j = 0; j < buttons.length; j++) {
            var btn = buttons[j];
            var btnColor = btn.getAttribute('data-slot-color');
            if (btnColor === '') continue;
            btn.className = (btnColor === selectedColor)
              ? 'w-6 h-6 rounded-full border-2 border-white ring-2 ring-offset-1 ring-offset-surface ring-fg transition-transform hover:scale-110 shrink-0'
              : 'w-6 h-6 rounded-full border-2 border-transparent hover:border-white/50 transition-transform hover:scale-110 shrink-0';
          }
        }
        var custom = el.querySelector('#appearance-surface-custom-' + slot);
        if (custom) custom.value = selectedColor || '#6366f1';
        var label = el.querySelector('#appearance-surface-label-' + slot);
        if (label) label.textContent = selectedColor || 'follows Accent Color';
      }

      el.addEventListener('chronicle:surface-accent-change', function (evt) {
        var detail = evt.detail || {};
        var slot = detail.slot;
        var color = detail.color || '';
        if (slot === 1) {
          draft.accentSurface1 = color;
        } else if (slot === 2) {
          draft.accentSurface2 = color;
        }
        highlightSurfaceRow(slot, color);
        updateSaveBar();
      });

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

      // buildSaveSteps compares draft against saved and returns one
      // { name, apply, run } per changed field: apply() copies that field's
      // draft value into saved (called only for steps that actually
      // succeeded — see the click handler below), run() issues its PUT and
      // returns the apiFetch promise. Building this list is pure given
      // draft/saved, but stays a closure (not hoisted out with
      // runSaveStepsSequentially) since apply()/run() both read and write
      // this init() call's own draft/saved/campaignId/csrfToken.
      function buildSaveSteps() {
        var steps = [];

        if (draft.brandName !== saved.brandName) {
          steps.push({
            name: 'brandName',
            apply: function () { saved.brandName = draft.brandName; },
            run: function () {
              return Chronicle.apiFetch('/campaigns/' + campaignId + '/branding', {
                method: 'PUT',
                body: { brand_name: draft.brandName },
                csrfToken: csrfToken
              });
            }
          });
        }

        // Accent color if changed (form-encoded for c.FormValue).
        if (draft.accentColor !== saved.accentColor) {
          steps.push({
            name: 'accentColor',
            apply: function () { saved.accentColor = draft.accentColor; },
            run: function () {
              return Chronicle.apiFetch('/campaigns/' + campaignId + '/accent-color', {
                method: 'PUT',
                body: 'accent_color=' + encodeURIComponent(draft.accentColor),
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                csrfToken: csrfToken
              });
            }
          });
        }

        // Action highlight if changed (same endpoint + form-encoded shape as
        // the site accent, routed by slot).
        if (draft.accentAction !== saved.accentAction) {
          steps.push({
            name: 'accentAction',
            apply: function () { saved.accentAction = draft.accentAction; },
            run: function () {
              return Chronicle.apiFetch('/campaigns/' + campaignId + '/accent-color', {
                method: 'PUT',
                body: 'accent_color=' + encodeURIComponent(draft.accentAction) + '&slot=action',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                csrfToken: csrfToken
              });
            }
          });
        }

        // App accent if changed.
        if (draft.accentApp !== saved.accentApp) {
          steps.push({
            name: 'accentApp',
            apply: function () { saved.accentApp = draft.accentApp; },
            run: function () {
              return Chronicle.apiFetch('/campaigns/' + campaignId + '/accent-color', {
                method: 'PUT',
                body: 'accent_color=' + encodeURIComponent(draft.accentApp) + '&slot=app',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                csrfToken: csrfToken
              });
            }
          });
        }

        // Surface accent 1 if changed (same endpoint + form-encoded shape as
        // action/app, routed by the legacy numeric slot).
        if (draft.accentSurface1 !== saved.accentSurface1) {
          steps.push({
            name: 'accentSurface1',
            apply: function () { saved.accentSurface1 = draft.accentSurface1; },
            run: function () {
              return Chronicle.apiFetch('/campaigns/' + campaignId + '/accent-color', {
                method: 'PUT',
                body: 'accent_color=' + encodeURIComponent(draft.accentSurface1) + '&slot=1',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                csrfToken: csrfToken
              });
            }
          });
        }

        // Surface accent 2 if changed.
        if (draft.accentSurface2 !== saved.accentSurface2) {
          steps.push({
            name: 'accentSurface2',
            apply: function () { saved.accentSurface2 = draft.accentSurface2; },
            run: function () {
              return Chronicle.apiFetch('/campaigns/' + campaignId + '/accent-color', {
                method: 'PUT',
                body: 'accent_color=' + encodeURIComponent(draft.accentSurface2) + '&slot=2',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                csrfToken: csrfToken
              });
            }
          });
        }

        // Font family if changed.
        if (draft.fontFamily !== saved.fontFamily) {
          steps.push({
            name: 'fontFamily',
            apply: function () { saved.fontFamily = draft.fontFamily; },
            run: function () {
              return Chronicle.apiFetch('/campaigns/' + campaignId + '/font-family', {
                method: 'PUT',
                body: { font_family: draft.fontFamily },
                csrfToken: csrfToken
              });
            }
          });
        }

        // Topbar style if changed.
        var topbarChanged = draft.topbarStyle.mode !== (saved.topbarStyle.mode || '') ||
                            draft.topbarStyle.color !== (saved.topbarStyle.color || '') ||
                            draft.topbarStyle.gradient_from !== (saved.topbarStyle.gradient_from || '') ||
                            draft.topbarStyle.gradient_to !== (saved.topbarStyle.gradient_to || '') ||
                            draft.topbarStyle.gradient_dir !== (saved.topbarStyle.gradient_dir || 'to-r') ||
                            draft.topbarStyle.image_path !== (saved.topbarStyle.image_path || '');
        if (topbarChanged) {
          steps.push({
            name: 'topbarStyle',
            apply: function () {
              saved.topbarStyle = {
                mode: draft.topbarStyle.mode,
                color: draft.topbarStyle.color,
                gradient_from: draft.topbarStyle.gradient_from,
                gradient_to: draft.topbarStyle.gradient_to,
                gradient_dir: draft.topbarStyle.gradient_dir,
                image_path: draft.topbarStyle.image_path
              };
            },
            run: function () {
              return Chronicle.apiFetch('/campaigns/' + campaignId + '/topbar-style', {
                method: 'PUT',
                body: {
                  mode: draft.topbarStyle.mode || '',
                  color: draft.topbarStyle.color || '',
                  gradient_from: draft.topbarStyle.gradient_from || '',
                  gradient_to: draft.topbarStyle.gradient_to || '',
                  gradient_dir: draft.topbarStyle.gradient_dir || '',
                  // Preserve the uploaded image path so switching a non-image
                  // topbar field and saving never blanks an image-mode topbar.
                  image_path: draft.topbarStyle.image_path || ''
                },
                csrfToken: csrfToken
              });
            }
          });
        }

        // Topbar content if changed.
        var contentChanged = draft.topbarContent.mode !== (saved.topbarContent.mode || 'none') ||
                             draft.topbarContent.quote !== (saved.topbarContent.quote || '') ||
                             JSON.stringify(draft.topbarContent.links) !== JSON.stringify(saved.topbarContent.links || []);
        if (contentChanged) {
          steps.push({
            name: 'topbarContent',
            apply: function () {
              saved.topbarContent = {
                mode: draft.topbarContent.mode,
                links: JSON.parse(JSON.stringify(draft.topbarContent.links)),
                quote: draft.topbarContent.quote
              };
            },
            run: function () {
              return Chronicle.apiFetch('/campaigns/' + campaignId + '/topbar-content', {
                method: 'PUT',
                body: {
                  mode: draft.topbarContent.mode || 'none',
                  links: draft.topbarContent.links || [],
                  quote: draft.topbarContent.quote || ''
                },
                csrfToken: csrfToken
              });
            }
          });
        }

        return steps;
      }

      if (saveBtn) {
        saveBtn.addEventListener('click', function () {
          var steps = buildSaveSteps();
          if (steps.length === 0) {
            updateSaveBar();
            return;
          }

          saveBtn.disabled = true;
          saveBtn.innerHTML = '<i class="fa-solid fa-spinner fa-spin text-xs mr-1"></i> Saving...';

          runSaveStepsSequentially(steps).then(function (result) {
            saveBtn.disabled = false;
            saveBtn.innerHTML = '<i class="fa-solid fa-check text-xs mr-1"></i> Save Changes';

            // Copy only the fields that actually saved into `saved`, so a
            // partial failure leaves Discard (and the page) agreeing with
            // what the database now holds rather than what was attempted.
            for (var i = 0; i < steps.length; i++) {
              if (result.succeeded.indexOf(steps[i].name) !== -1) {
                steps[i].apply();
              }
            }
            updateSaveBar();
            if (result.succeeded.indexOf('topbarStyle') !== -1 || result.succeeded.indexOf('topbarContent') !== -1) {
              refreshTopbar();
            }

            if (result.failed.length > 0) {
              Chronicle.notify('Some changes failed to save', 'error');
            } else {
              // Apply every accent slot that just saved to page CSS custom
              // properties live (T-B3), so the change is visible site-wide
              // immediately. Surface 1/2 move from the widget-scoped preview
              // (surface_accent_onclick.go) to the real, page-wide property;
              // the widget-scoped override is cleared so nothing shadows it.
              applySlotToPage('--color-accent', saved.accentColor);
              applySlotToPage('--color-accent-action', saved.accentAction);
              applySlotToPage('--color-accent-app', saved.accentApp);
              applySlotToPage('--color-accent-surface-1', saved.accentSurface1);
              applySlotToPage('--color-accent-surface-2', saved.accentSurface2);
              clearSurfaceVarPreview(1);
              clearSurfaceVarPreview(2);
              Chronicle.notify('Appearance saved', 'success');
            }
          });
        });
      }

      // --- Discard Button ---
      //
      // The inverse of Save: reset every draft field to the last-saved
      // value, then push that back into each control and its live preview.
      // Named update*/highlight* functions already exist for every field
      // (Save uses none of them — it only reads draft — so they are reused
      // here rather than duplicated).

      if (discardBtn) {
        discardBtn.addEventListener('click', function () {
          draft.brandName = saved.brandName;
          draft.accentColor = saved.accentColor;
          draft.accentAction = saved.accentAction;
          draft.accentApp = saved.accentApp;
          draft.accentSurface1 = saved.accentSurface1;
          draft.accentSurface2 = saved.accentSurface2;
          draft.fontFamily = saved.fontFamily;
          draft.topbarStyle = {
            mode: saved.topbarStyle.mode || '',
            color: saved.topbarStyle.color || '',
            gradient_from: saved.topbarStyle.gradient_from || '',
            gradient_to: saved.topbarStyle.gradient_to || '',
            gradient_dir: saved.topbarStyle.gradient_dir || 'to-r',
            image_path: saved.topbarStyle.image_path || ''
          };
          draft.topbarContent = {
            mode: saved.topbarContent.mode || 'none',
            links: JSON.parse(JSON.stringify(saved.topbarContent.links || [])),
            quote: saved.topbarContent.quote || ''
          };

          // Brand name.
          if (brandInput) brandInput.value = draft.brandName;
          if (previewBrand) previewBrand.textContent = draft.brandName || (brandInput && brandInput.placeholder) || '';

          // Site / Action / App accent slots: swatch ring, custom-picker
          // swatch, and live preview.
          highlightAccent(draft.accentColor);
          updateAccentPreview(draft.accentColor);
          highlightAction(draft.accentAction);
          updateActionPreview();
          highlightApp(draft.accentApp);
          updateAppPreview();
          var accentCustom = el.querySelector('#appearance-accent-custom');
          if (accentCustom) accentCustom.value = draft.accentColor || '#6366f1';
          var actionCustom = el.querySelector('#appearance-action-custom');
          if (actionCustom) actionCustom.value = draft.accentAction || '#6366f1';
          var appCustom = el.querySelector('#appearance-app-custom');
          if (appCustom) appCustom.value = draft.accentApp || '#6366f1';

          // Surface accents: clear the widget-scoped preview override (it
          // falls back to whatever <html> already carries from the last
          // save) and resync the ring, custom swatch, and label.
          clearSurfaceVarPreview(1);
          clearSurfaceVarPreview(2);
          highlightSurfaceRow(1, draft.accentSurface1);
          highlightSurfaceRow(2, draft.accentSurface2);

          // Font.
          if (fontContainer) {
            var allFontBtns = fontContainer.querySelectorAll('button[data-font-family]');
            for (var fi = 0; fi < allFontBtns.length; fi++) {
              var fbtn = allFontBtns[fi];
              var fSelected = fbtn.getAttribute('data-font-family') === draft.fontFamily;
              fbtn.className = fSelected
                ? 'px-3 py-2 rounded-lg border border-accent bg-accent/10 text-accent font-medium text-sm'
                : 'px-3 py-2 rounded-lg border border-edge bg-surface hover:border-accent/30 text-fg-secondary text-sm';
            }
          }
          updateFontPreview(draft.fontFamily);

          // Topbar style (image mode/path is never dirty — see
          // initTopbarImageSync — so there is nothing to revert there).
          if (solidColorInput) solidColorInput.value = draft.topbarStyle.color || '#000000';
          if (gradFromInput) gradFromInput.value = draft.topbarStyle.gradient_from || '#000000';
          if (gradToInput) gradToInput.value = draft.topbarStyle.gradient_to || '#000000';
          if (gradDirSelect) gradDirSelect.value = draft.topbarStyle.gradient_dir || 'to-r';
          setActiveMode(draft.topbarStyle.mode);
          updateTopbarPreview();

          // Topbar content.
          setActiveContentMode(draft.topbarContent.mode);
          if (quoteTextarea) {
            quoteTextarea.value = draft.topbarContent.quote || '';
            if (quoteCounter) quoteCounter.textContent = quoteTextarea.value.length + ' / 200';
          }
          renderLinksList();

          updateSaveBar();
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
       * Update all site-accent-colored elements in the preview. The primary
       * button and "Characters" category chip follow the Action and App
       * slots instead (updateActionPreview/updateAppPreview below), which
       * this function re-triggers so their site-accent fallback stays in
       * sync when the site accent changes.
       */
      function updateAccentPreview(color) {
        var accent = color || '#6366f1'; // fallback to default indigo
        var accentLight = accent + '20'; // 12% opacity for backgrounds

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
        // Category icon circles (Characters/cat1 excluded — App accent owns it).
        var catEls = [previewCat2, previewCat3];
        for (var i = 0; i < catEls.length; i++) {
          if (catEls[i]) {
            catEls[i].style.backgroundColor = accentLight;
            catEls[i].style.color = accent;
          }
        }

        updateActionPreview();
        updateAppPreview();
      }

      /**
       * Update the preview's primary button (Action highlight slot). Unset
       * falls back to the draft site accent, mirroring the
       * var(--color-accent-action, var(--color-accent, ...)) chain the real
       * .btn-primary CSS uses (input.css).
       */
      function updateActionPreview() {
        if (!previewBtnPrimary) return;
        previewBtnPrimary.style.backgroundColor = draft.accentAction || draft.accentColor || '#6366f1';
      }

      /**
       * Update the preview's "Characters" category chip (App accent slot,
       * the character-pages/calendar-app identity slot). Unset falls back to
       * the draft site accent — a simplification of the real fallback chain,
       * which also passes through a legacy surface-1 accent this preview
       * does not wire up.
       */
      function updateAppPreview() {
        if (!previewCat1) return;
        var app = draft.accentApp || draft.accentColor || '#6366f1';
        previewCat1.style.backgroundColor = app + '20';
        previewCat1.style.color = app;
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
       * Keep the editor's draft/saved topbar-image state in sync with the
       * first-class TopbarImageSection fragment. Upload/remove are HTMX POST/
       * DELETE that swap #appearance-topbar-image-section in place (no reload);
       * the swapped fragment carries data-topbar-image-path. Because the server
       * persists the mode + path atomically on upload/remove, we mirror that
       * into BOTH draft and saved so (a) the Save bar doesn't show a phantom
       * "unsaved changes", and (b) a later Save won't re-send stale topbar data.
       */
      function initTopbarImageSync() {
        document.body.addEventListener('htmx:afterSwap', function (evt) {
          if (!evt.detail || !evt.detail.target) return;
          var target = evt.detail.target;
          // outerHTML swaps can fire afterSwap on the replaced node OR its
          // parent, so resolve the section either way; ignore swaps outside
          // this widget (e.g. the backdrop section).
          var section = null;
          if (target.id === 'appearance-topbar-image-section') {
            section = target;
          } else if (target.querySelector) {
            section = target.querySelector('#appearance-topbar-image-section');
          }
          if (!section || !el.contains(section)) return;
          var path = section.getAttribute('data-topbar-image-path') || '';
          var mode = path ? 'image' : '';
          draft.topbarStyle.image_path = path;
          draft.topbarStyle.mode = mode;
          saved.topbarStyle.image_path = path;
          saved.topbarStyle.mode = mode;
          // Re-fetch the (re-rendered) panel node so setActiveMode toggles the
          // correct element after the swap replaced the old one.
          imagePanel = el.querySelector('#appearance-topbar-image');
          setActiveMode(mode);
          updateTopbarPreview();
          updateSaveBar();
          // Upload and remove save on the server at once, so the live header follows now.
          refreshTopbar();
        });
      }

      /**
       * Apply one accent slot's color to the page's CSS custom properties so
       * the change is visible site-wide without a full page reload. varPrefix
       * is the base custom property name (e.g. "--color-accent",
       * "--color-accent-action", "--color-accent-app"). An empty hex removes
       * the inline override, handing control back to the var() fallback
       * chain — unlike the site slot, the two new slots have no static
       * input.css default to fall back to.
       */
      function applySlotToPage(varPrefix, hex) {
        var root = document.documentElement;
        if (!hex) {
          root.style.removeProperty(varPrefix);
          root.style.removeProperty(varPrefix + '-hover');
          root.style.removeProperty(varPrefix + '-light');
          root.style.removeProperty(varPrefix + '-rgb');
          root.style.removeProperty(varPrefix + '-hover-rgb');
          root.style.removeProperty(varPrefix + '-light-rgb');
          return;
        }
        var r = parseInt(hex.slice(1, 3), 16);
        var g = parseInt(hex.slice(3, 5), 16);
        var b = parseInt(hex.slice(5, 7), 16);
        if (isNaN(r)) return;
        // Base color as RGB triplet for Tailwind.
        root.style.setProperty(varPrefix, hex);
        // Hover: darken by ~12%.
        var hr = Math.max(0, Math.min(255, Math.round(r * 0.88)));
        var hg = Math.max(0, Math.min(255, Math.round(g * 0.88)));
        var hb = Math.max(0, Math.min(255, Math.round(b * 0.88)));
        root.style.setProperty(varPrefix + '-hover', '#' + toHex(hr) + toHex(hg) + toHex(hb));
        // Light: blend toward white by ~60%.
        var lr = Math.max(0, Math.min(255, Math.round(r + (255 - r) * 0.6)));
        var lg = Math.max(0, Math.min(255, Math.round(g + (255 - g) * 0.6)));
        var lb = Math.max(0, Math.min(255, Math.round(b + (255 - b) * 0.6)));
        root.style.setProperty(varPrefix + '-light', '#' + toHex(lr) + toHex(lg) + toHex(lb));
        // RGB triplets for Tailwind's color utilities (bg-accent, bg-action, bg-app, etc.)
        root.style.setProperty(varPrefix + '-rgb', r + ' ' + g + ' ' + b);
        root.style.setProperty(varPrefix + '-hover-rgb', hr + ' ' + hg + ' ' + hb);
        root.style.setProperty(varPrefix + '-light-rgb', lr + ' ' + lg + ' ' + lb);
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

  // Test-only hook: exposes the pure save-sequencing helper so the node:test
  // contract suite can assert steps run one at a time (never concurrently)
  // without a real browser DOM. `module` is undefined when loaded via
  // <script>, so this is a no-op in the browser.
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
      runSaveStepsSequentially: runSaveStepsSequentially
    };
  }
})();
