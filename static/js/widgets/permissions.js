/**
 * permissions.js -- Chronicle Per-Entity Permissions Widget
 *
 * "Permissions" trigger button opens a slide-in card with the
 * visibility-mode selector (Everyone / DM Only / Custom) and, in Custom
 * mode, per-role / per-user / per-group grant rows.
 *
 * Mount modes:
 *   1. `data-endpoint` present (entity show/edit): loads/saves via
 *      GET/PUT /permissions. On the page itself, `data-layout="corner"`
 *      reuses the server-rendered control as the trigger and unfolds the
 *      editor from it.
 *   2. `data-mode="draft"` + `data-draft-target` (create form): no
 *      endpoint yet, writes is_private into the target hidden input;
 *      Custom mode is disabled until the entity has an ID.
 *
 * Auto-mounted by boot.js on data-widget="permissions".
 */
// Role constant matching Go permissions.RoleOwner.
var ROLE_OWNER = 3;

Chronicle.register('permissions', {
  init: function (el, config) {
    var state = {
      visibility: 'default',
      isPrivate: false,
      members: [],
      groups: [],
      permissions: [],
      tagGrants: [],
      loading: true,
      // loadFailed (ADR-057): GET .../permissions can 403 (Owner-only
      // route) or fail on the network, leaving visibility/isPrivate at
      // their untouched init values. Without this flag getMode() cannot
      // tell "loaded and Everyone" apart from "never loaded, defaulted to
      // Everyone". renderTrigger()/renderBody() must check this BEFORE
      // calling getMode().
      loadFailed: false,
      saving: false,
      saved: false,
      error: null,
      errorCategory: null,
      open: false,
      abortController: null
    };
    var editable = config.editable === true;
    var draftMode = config.mode === 'draft';
    var draftTarget = config.draftTarget || null;
    // Inline layout: opt-in via data-layout="inline". Renders the editor
    // as a compact summary row that expands in place (animated) instead
    // of the right-edge slide-in card. Pure presentation — reuses
    // renderBody/load/save/grant UI verbatim.
    var inline = config.layout === 'inline';
    // Corner layout: the page's top-right visibility control. The trigger
    // is the server-rendered chip already inside the mount, and the editor
    // unfolds from it as a popover rather than sliding in from the screen
    // edge, so it opens inside the thing it belongs to.
    var corner = config.layout === 'corner';

    // Inject scoped styles once. Card uses CSS variables that flow from
    // the campaign theme (surface/edge/accent) so per-campaign retints
    // work.
    if (!document.getElementById('perm-widget-styles')) {
      var style = document.createElement('style');
      style.id = 'perm-widget-styles';
      style.textContent = [
        // Trigger button — small inline affordance rendered into the mount el.
        '.perm-trigger { display: inline-flex; align-items: center; gap: 6px; padding: 6px 12px; border-radius: 8px; border: 1px solid var(--color-edge, #d1d5db); background: var(--color-surface, #fff); color: var(--color-fg-body, #374151); font-size: 13px; cursor: pointer; transition: background 0.15s, border-color 0.15s; }',
        '.perm-trigger:hover { background: var(--color-surface-alt, #f3f4f6); border-color: var(--color-accent, #6366f1); }',
        '.perm-trigger:focus-visible { outline: 2px solid var(--color-accent, #6366f1); outline-offset: 2px; }',
        '.perm-trigger .perm-trigger-mode { color: var(--color-fg-muted, #9ca3af); font-size: 11px; padding-left: 4px; border-left: 1px solid var(--color-edge, #e5e7eb); margin-left: 4px; }',
        // Backdrop — fades in/out under the card.
        '.perm-backdrop { position: fixed; inset: 0; background: rgba(15, 23, 42, 0.4); opacity: 0; pointer-events: none; transition: opacity 280ms ease-out; z-index: 90; }',
        '.perm-backdrop.perm-open { opacity: 1; pointer-events: auto; }',
        // Card — fixed to the right edge, off-screen by default.
        '.perm-card { position: fixed; top: 0; right: 0; bottom: 0; width: min(100%, 420px); background: var(--color-surface, #fff); color: var(--color-fg-body, #374151); border-left: 1px solid var(--color-edge, #e5e7eb); box-shadow: -16px 0 32px -8px rgba(15, 23, 42, 0.18); display: flex; flex-direction: column; transform: translateX(100%); transition: transform 280ms ease-out; z-index: 100; }',
        '.perm-card.perm-open { transform: translateX(0); }',
        // Header / body / footer of the card.
        '.perm-card-header { display: flex; align-items: center; justify-content: space-between; padding: 16px 20px; border-bottom: 1px solid var(--color-edge, #e5e7eb); }',
        '.perm-card-title { font-size: 15px; font-weight: 600; color: var(--color-fg, #111827); display: flex; align-items: center; gap: 8px; }',
        '.perm-card-close { width: 32px; height: 32px; border-radius: 8px; border: none; background: transparent; color: var(--color-fg-muted, #6b7280); cursor: pointer; display: flex; align-items: center; justify-content: center; transition: background 0.15s; }',
        '.perm-card-close:hover { background: var(--color-surface-alt, #f3f4f6); color: var(--color-fg-body, #374151); }',
        '.perm-card-body { flex: 1 1 auto; overflow-y: auto; padding: 16px 20px; }',
        '.perm-card-footer { padding: 10px 20px; border-top: 1px solid var(--color-edge, #e5e7eb); font-size: 12px; color: var(--color-fg-muted, #6b7280); min-height: 20px; }',
        // Inline error region.
        '.perm-error { border-radius: 8px; padding: 10px 12px; font-size: 13px; margin-bottom: 12px; display: flex; align-items: flex-start; gap: 8px; }',
        '.perm-error .perm-error-dismiss { margin-left: auto; background: transparent; border: none; color: inherit; opacity: 0.7; cursor: pointer; font-size: 16px; line-height: 1; }',
        '.perm-error-validation { background: #fef3c7; color: #92400e; border: 1px solid #fde68a; }',
        '.perm-error-auth { background: #fee2e2; color: #991b1b; border: 1px solid #fecaca; }',
        '.perm-error-not_found { background: #e0e7ff; color: #3730a3; border: 1px solid #c7d2fe; }',
        '.perm-error-internal { background: #fee2e2; color: #991b1b; border: 1px solid #fecaca; }',
        '.dark .perm-error-validation { background: #451a03; color: #fbbf24; border-color: #92400e; }',
        '.dark .perm-error-auth, .dark .perm-error-internal { background: #450a0a; color: #fca5a5; border-color: #991b1b; }',
        '.dark .perm-error-not_found { background: #1e1b4b; color: #c7d2fe; border-color: #3730a3; }',
        // Mode picker — Material-ish card list.
        '.perm-mode-list { display: flex; flex-direction: column; gap: 8px; }',
        '.perm-mode-option { display: flex; align-items: flex-start; gap: 10px; padding: 12px 14px; border-radius: 10px; border: 1.5px solid var(--color-edge, #e5e7eb); background: var(--color-surface, #fff); cursor: pointer; transition: border-color 0.15s, background 0.15s, box-shadow 0.15s; }',
        '.perm-mode-option:hover { background: var(--color-surface-alt, #f9fafb); }',
        '.perm-mode-option.perm-mode-active { border-color: var(--color-accent, #6366f1); background: var(--color-accent-subtle, #eef2ff); box-shadow: 0 1px 3px rgba(99, 102, 241, 0.12); }',
        '.perm-mode-option.perm-mode-disabled { opacity: 0.6; cursor: not-allowed; }',
        '.perm-mode-option input[type=radio] { margin-top: 2px; accent-color: var(--color-accent, #6366f1); }',
        '.perm-mode-icon { width: 28px; height: 28px; flex-shrink: 0; display: flex; align-items: center; justify-content: center; border-radius: 8px; background: var(--color-surface-alt, #f3f4f6); color: var(--color-fg-muted, #6b7280); margin-top: 1px; }',
        '.perm-mode-option.perm-mode-active .perm-mode-icon { background: var(--color-accent, #6366f1); color: white; }',
        '.perm-mode-label { font-size: 13px; font-weight: 600; color: var(--color-fg, #111827); }',
        '.perm-mode-desc { font-size: 12px; color: var(--color-fg-muted, #6b7280); margin-top: 2px; line-height: 1.4; }',
        // Custom grants block.
        '.perm-section-header { font-size: 11px; font-weight: 600; color: var(--color-fg-muted, #6b7280); text-transform: uppercase; letter-spacing: 0.06em; margin: 16px 0 6px; }',
        '.perm-section-hint { font-size: 11px; color: var(--color-fg-muted, #9ca3af); margin-bottom: 8px; }',
        '.perm-grant-row { display: flex; align-items: center; gap: 8px; padding: 8px 0; border-bottom: 1px solid var(--color-edge, #f3f4f6); }',
        '.perm-grant-row:last-child { border-bottom: none; }',
        '.perm-grant-name { flex: 1; font-size: 13px; color: var(--color-fg-body, #374151); display: flex; align-items: center; gap: 6px; min-width: 0; }',
        '.perm-grant-name .perm-role-badge { font-size: 10px; padding: 1px 6px; border-radius: 9999px; background: var(--color-surface-alt, #f3f4f6); color: var(--color-fg-muted, #6b7280); }',
        '.perm-grant-name .perm-grant-icon { width: 20px; height: 20px; display: inline-flex; align-items: center; justify-content: center; color: var(--color-fg-muted, #6b7280); flex-shrink: 0; }',
        '.perm-toggle-group { display: inline-flex; }',
        '.perm-toggle { padding: 3px 10px; font-size: 12px; border: 1px solid var(--color-edge, #d1d5db); background: var(--color-surface, #fff); color: var(--color-fg-muted, #6b7280); cursor: pointer; transition: background 0.15s, color 0.15s, border-color 0.15s; }',
        '.perm-toggle:first-child { border-radius: 6px 0 0 6px; }',
        '.perm-toggle:last-child { border-radius: 0 6px 6px 0; border-left-width: 0; }',
        '.perm-toggle:not(:first-child):not(:last-child) { border-left-width: 0; }',
        '.perm-toggle:hover:not(.perm-toggle-active) { background: var(--color-surface-alt, #f3f4f6); }',
        '.perm-toggle-active { background: var(--color-accent, #6366f1); color: white; border-color: var(--color-accent, #6366f1); }',
        '.perm-warning { font-size: 12px; color: #b45309; padding: 8px 12px; background: #fffbeb; border: 1px solid #fde68a; border-radius: 8px; margin-top: 8px; display: flex; align-items: flex-start; gap: 8px; }',
        '.dark .perm-warning { background: #451a03; border-color: #92400e; color: #fbbf24; }',
        '.perm-loading { padding: 24px; text-align: center; color: var(--color-fg-muted, #9ca3af); font-size: 13px; }',
        '.perm-readonly-badge { display: inline-flex; align-items: center; gap: 6px; padding: 6px 12px; border-radius: 6px; font-size: 13px; background: var(--color-surface-alt, #f3f4f6); color: var(--color-fg-body, #374151); }',
        '.perm-draft-note { font-size: 11px; color: var(--color-fg-muted, #6b7280); margin-top: 10px; padding: 8px 10px; background: var(--color-surface-alt, #f9fafb); border-radius: 6px; border-left: 3px solid var(--color-accent, #6366f1); line-height: 1.4; }',
        // Tag-derived grants note: amber to distinguish a widening exposed
        // by a tag from the page-level permission controls.
        '.perm-tag-note { font-size: 12px; color: var(--color-fg-body, #374151); margin-bottom: 12px; padding: 8px 10px; background: var(--color-surface-alt, #f9fafb); border-radius: 6px; border-left: 3px solid #f59e0b; line-height: 1.5; }',
        '.perm-tag-note .perm-tag-note-title { font-weight: 600; display: block; margin-bottom: 2px; }',
        '.perm-tag-note .perm-tag-chip { color: var(--color-fg-muted, #6b7280); }',
        '.perm-trigger-widened { width: 6px; height: 6px; border-radius: 9999px; background: #f59e0b; margin-left: 4px; }',
        '@media (max-width: 480px) { .perm-card { width: 100%; box-shadow: none; } }',
        // Inline layout: a summary trigger above a collapsible panel in the
        // form flow. The panel animates open/closed via the
        // grid-template-rows 0fr↔1fr trick — pure CSS height animation, no JS
        // measurement, reduced-motion safe.
        '.perm-inline .perm-trigger { width: 100%; justify-content: flex-start; }',
        '.perm-inline .perm-trigger .perm-trigger-chevron { margin-left: auto; transition: transform 200ms ease-out; }',
        '.perm-inline.perm-inline-open .perm-trigger .perm-trigger-chevron { transform: rotate(180deg); }',
        '.perm-inline-panel { display: grid; grid-template-rows: 0fr; transition: grid-template-rows 280ms ease-out; border: 1px solid var(--color-edge, #e5e7eb); border-top: none; border-radius: 0 0 8px 8px; }',
        '.perm-inline-panel.perm-open { grid-template-rows: 1fr; }',
        '.perm-inline-inner { overflow: hidden; min-height: 0; }',
        '.perm-inline .perm-card-body { padding: 16px; }',
        '.perm-inline .perm-card-footer { border-top: 1px solid var(--color-edge, #e5e7eb); }',
        '.perm-inline-open .perm-trigger { border-radius: 8px 8px 0 0; }',
        '@media (prefers-reduced-motion: reduce) { .perm-inline-panel, .perm-inline .perm-trigger-chevron { transition: none; } }',
        // Corner layout: a popover anchored under the page's top-right
        // control, growing from that corner. Only transform and opacity
        // animate; visibility flips after the close transition ends.
        '.perm-corner-panel { position: absolute; top: calc(100% + 6px); right: 0; width: min(380px, calc(100vw - 32px)); max-height: min(70vh, 560px); display: flex; flex-direction: column; background: var(--color-card-bg, #fff); color: var(--color-fg-body, #374151); border: 1px solid var(--color-edge, #e5e7eb); border-radius: 12px; box-shadow: var(--elev-dragged, 0 16px 36px -8px rgba(0,0,0,0.22)); transform-origin: top right; transform: scale(0.92) translateY(-4px); opacity: 0; visibility: hidden; pointer-events: none; transition: transform var(--dur-standard, 200ms) var(--ease-out, ease-out), opacity var(--dur-standard, 200ms) var(--ease-out, ease-out), visibility 0s linear var(--dur-standard, 200ms); z-index: 50; text-align: left; }',
        '.perm-corner-panel.perm-open { transform: none; opacity: 1; visibility: visible; pointer-events: auto; transition-delay: 0s; }',
        '.perm-corner-panel .perm-card-header { padding: 12px 16px; }',
        '.perm-corner-panel .perm-card-body { padding: 12px 16px; }',
        '.perm-corner-open [data-perm-corner-tip] { display: none; }',
        '.perm-corner-open [data-perm-corner-trigger] { border-color: var(--color-accent, #6366f1); }',
        '.perm-unsaved { display: none; margin: 0 16px 12px; padding: 8px 10px; border-radius: 8px; font-size: 12px; background: #fffbeb; color: #92400e; border: 1px solid #fde68a; }',
        '.dark .perm-unsaved { background: #451a03; color: #fbbf24; border-color: #92400e; }',
        '.perm-unsaved.perm-unsaved-show { display: block; animation: perm-unsaved-in var(--dur-standard, 200ms) var(--ease-out, ease-out); }',
        '.perm-corner-panel.perm-nudge { animation: perm-nudge 360ms var(--ease-in-out, ease-in-out); }',
        '@keyframes perm-unsaved-in { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; transform: none; } }',
        '@keyframes perm-nudge { 20%, 60% { transform: translateX(-5px); } 40%, 80% { transform: translateX(5px); } }',
        '@media (prefers-reduced-motion: reduce) { .perm-corner-panel { transition: none; } .perm-corner-panel.perm-nudge, .perm-unsaved.perm-unsaved-show { animation: none; } }'
      ].join('\n');
      document.head.appendChild(style);
    }

    var roleNames = { 1: 'Player', 2: 'Scribe', 3: 'Owner' };

    function modeLabel(mode) {
      if (mode === 'everyone') return 'Everyone';
      if (mode === 'dm_only') return 'DM Only';
      if (mode === 'custom') return 'Custom';
      return 'Permissions';
    }

    function getMode() {
      if (state.visibility === 'custom') return 'custom';
      if (state.isPrivate) return 'dm_only';
      return 'everyone';
    }

    function setMode(mode) {
      if (mode === 'everyone') {
        state.visibility = 'default';
        state.isPrivate = false;
        state.permissions = [];
      } else if (mode === 'dm_only') {
        state.visibility = 'default';
        state.isPrivate = true;
        state.permissions = [];
      } else if (mode === 'custom') {
        state.visibility = 'custom';
        state.isPrivate = false;
      }
      syncDraftTarget();
    }

    function syncDraftTarget() {
      if (!draftMode || !draftTarget) return;
      var target = document.querySelector(draftTarget);
      if (target) {
        target.value = state.isPrivate ? 'true' : 'false';
      }
    }

    function findGrant(subjectType, subjectId) {
      for (var i = 0; i < state.permissions.length; i++) {
        var p = state.permissions[i];
        if (p.subject_type === subjectType && p.subject_id === subjectId) {
          return p.permission;
        }
      }
      return 'none';
    }

    function setGrant(subjectType, subjectId, permission) {
      state.permissions = state.permissions.filter(function (p) {
        return !(p.subject_type === subjectType && p.subject_id === subjectId);
      });
      if (permission !== 'none') {
        state.permissions.push({
          subject_type: subjectType,
          subject_id: subjectId,
          permission: permission
        });
      }
    }

    function hasAnyGrants() {
      return state.permissions.length > 0;
    }

    // DOM references — created once and re-rendered in place.
    var trigger = null;
    var backdrop = null;
    var card = null;
    var bodyEl = null;
    var statusEl = null;

    function renderTrigger() {
      if (!trigger) return;
      if (corner) {
        renderCornerTrigger();
        return;
      }
      trigger.innerHTML = '';
      // ADR-057: a load that hasn't resolved yet is just as unknown as one
      // that failed — getMode() would otherwise run against untouched init
      // defaults and paint "Everyone" until the response lands. Draft mode
      // is excluded: it has no endpoint and owns its mode locally from
      // init, so checking `!draftMode` here (rather than state.loading)
      // keeps it visible even on the very first renderTrigger() call, which
      // runs before load().
      var modeUnknown = state.loadFailed || (state.loading && !draftMode);
      if (modeUnknown) {
        var unknownIcon = document.createElement('i');
        unknownIcon.className = 'fa-solid fa-circle-question text-xs';
        trigger.appendChild(unknownIcon);
        var unknownLabel = document.createElement('span');
        unknownLabel.textContent = 'Permissions';
        trigger.appendChild(unknownLabel);
        // Inline mode's expand affordance must survive this branch too —
        // the panel still opens on click even though the mode is unknown.
        if (inline) {
          var unknownChevron = document.createElement('i');
          unknownChevron.className = 'fa-solid fa-chevron-down text-xs perm-trigger-chevron';
          trigger.appendChild(unknownChevron);
        }
        return;
      }
      var mode = getMode();
      var icon = mode === 'everyone' ? 'fa-globe' : mode === 'dm_only' ? 'fa-lock' : 'fa-shield-halved';
      var iconEl = document.createElement('i');
      iconEl.className = 'fa-solid ' + icon + ' text-xs';
      trigger.appendChild(iconEl);
      var labelEl = document.createElement('span');
      labelEl.textContent = 'Permissions';
      trigger.appendChild(labelEl);
      var modeBadge = document.createElement('span');
      modeBadge.className = 'perm-trigger-mode';
      modeBadge.textContent = modeLabel(mode);
      trigger.appendChild(modeBadge);
      // Tag-widening dot: an amber pip when an otherwise hidden page is
      // exposed by a tag grant, mirroring the show-page badge.
      if (mode !== 'everyone' && state.tagGrants && state.tagGrants.length > 0) {
        var widened = document.createElement('span');
        widened.className = 'perm-trigger-widened';
        widened.setAttribute('data-tag-widened', 'true');
        widened.setAttribute('title', 'Also visible via tag grants');
        trigger.appendChild(widened);
      }
      // Inline mode: a chevron affordance that rotates when expanded.
      if (inline) {
        var chevron = document.createElement('i');
        chevron.className = 'fa-solid fa-chevron-down text-xs perm-trigger-chevron';
        trigger.appendChild(chevron);
      }
    }

    // renderCornerTrigger refreshes the server-rendered corner control after
    // a load or save. Until a load succeeds the server's answer stands, so
    // the control never flashes an unknown or defaulted state.
    function renderCornerTrigger() {
      if (state.loading || state.loadFailed) return;
      var mode = getMode();
      var icon = trigger.querySelector('[data-perm-corner-icon]');
      if (icon) {
        icon.className = 'fa-solid ' + (mode === 'everyone' ? 'fa-globe' : mode === 'dm_only' ? 'fa-lock' : 'fa-shield-halved') + ' text-[11px]';
        icon.style.color = mode === 'custom' ? 'var(--color-accent)' : 'var(--color-fg-muted)';
      }
      var label = trigger.querySelector('[data-perm-corner-label]');
      if (label) label.textContent = mode === 'everyone' ? 'Everyone' : mode === 'dm_only' ? 'DM team only' : 'Custom';
      trigger.setAttribute('data-visibility-badge', mode);
      // Tags only widen a page that is otherwise hidden.
      var dot = trigger.querySelector('[data-perm-corner-dot]');
      if (dot) dot.style.display = mode === 'everyone' ? 'none' : '';
      var tip = el.querySelector('[data-perm-corner-tip]');
      if (tip && tip.getAttribute('data-perm-mode') !== mode && tip.hasAttribute('data-perm-mode')) {
        tip.textContent = cornerTip(mode);
      }
      if (tip) tip.setAttribute('data-perm-mode', mode);
    }

    function cornerTip(mode) {
      var base = mode === 'everyone' ? 'Visible to everyone in this campaign'
        : mode === 'dm_only' ? 'Visible only to the owner and members given DM access'
        : 'Custom permissions — specific people';
      if (mode !== 'everyone' && state.tagGrants && state.tagGrants.length > 0) {
        base += ' · Also visible via tags';
      }
      return base + ' · Click to change.';
    }

    // renderTagGrantsNote appends a read-only summary of the tag-derived
    // grants that widen this page's visibility. If a tag exposed the page,
    // the Owner managing permissions must see it, named.
    function renderTagGrantsNote(parent) {
      if (!state.tagGrants || state.tagGrants.length === 0) return;
      var note = document.createElement('div');
      note.className = 'perm-tag-note';
      note.setAttribute('data-tag-grants-note', 'true');
      var title = document.createElement('span');
      title.className = 'perm-tag-note-title';
      title.innerHTML = '<i class="fa-solid fa-tag text-xs"></i> Also visible via tags';
      note.appendChild(title);
      state.tagGrants.forEach(function (g) {
        var line = document.createElement('div');
        line.className = 'perm-tag-chip';
        var subject = g.subject_label || 'someone';
        var tag = g.tag_name || g.tag_slug || 'a tag';
        line.textContent = subject + ' ← ' + tag;
        note.appendChild(line);
      });
      parent.appendChild(note);
    }

    function renderBody() {
      if (!bodyEl) return;
      bodyEl.innerHTML = '';

      if (state.loading) {
        var loadingDiv = document.createElement('div');
        loadingDiv.className = 'perm-loading';
        loadingDiv.textContent = 'Loading permissions...';
        bodyEl.appendChild(loadingDiv);
        return;
      }

      // Inline error region — uses category to pick severity class.
      if (state.error) {
        var errDiv = document.createElement('div');
        var category = state.errorCategory || 'internal';
        // Defensive: an unknown category falls through to internal styling
        // rather than emitting an unstyled element.
        var knownCategories = { validation: 1, auth: 1, not_found: 1, internal: 1, config: 1 };
        var cssCategory = knownCategories[category] ? category : 'internal';
        errDiv.className = 'perm-error perm-error-' + cssCategory;
        var errIcon = document.createElement('i');
        errIcon.className = 'fa-solid fa-circle-exclamation';
        errIcon.style.marginTop = '2px';
        errDiv.appendChild(errIcon);
        var errText = document.createElement('div');
        errText.style.flex = '1';
        errText.textContent = state.error;
        errDiv.appendChild(errText);
        // A failed-load error is not offered a dismiss button: unlike a
        // save error, a failed load has no real content underneath it to
        // reveal once dismissed (the mode picker needs a real answer from
        // load()). Dismissing would leave the panel empty with no way back.
        if (!state.loadFailed) {
          var dismissBtn = document.createElement('button');
          dismissBtn.type = 'button';
          dismissBtn.className = 'perm-error-dismiss';
          dismissBtn.innerHTML = '&times;';
          dismissBtn.setAttribute('aria-label', 'Dismiss error');
          dismissBtn.addEventListener('click', function () {
            state.error = null;
            state.errorCategory = null;
            renderBody();
          });
          errDiv.appendChild(dismissBtn);
        }
        bodyEl.appendChild(errDiv);
      }

      // ADR-057: a failed load has already said everything it honestly can
      // via the perm-error region above. Rendering the mode picker below
      // this point would fall back to state's untouched init defaults
      // (getMode() reading visibility: 'default') and repaint a wrong
      // "Everyone" claim. Stop here instead.
      if (state.loadFailed) {
        return;
      }

      var mode = getMode();

      // Tag-derived widening note — always shown (read-only + editable) so the
      // exposure is never silent.
      renderTagGrantsNote(bodyEl);

      // Read-only mode for non-owners: small badge, no controls.
      if (!editable) {
        var badge = document.createElement('div');
        badge.className = 'perm-readonly-badge';
        var icon = mode === 'everyone' ? 'fa-globe' : mode === 'dm_only' ? 'fa-lock' : 'fa-shield-halved';
        var label = mode === 'everyone' ? 'Visible to everyone' : mode === 'dm_only' ? 'Private (GM only)' : 'Custom permissions';
        badge.innerHTML = '<i class="fa-solid ' + Chronicle.escapeHtml(icon) + ' text-xs"></i> ' + Chronicle.escapeHtml(label);
        bodyEl.appendChild(badge);
        return;
      }

      // Mode picker.
      var modeList = document.createElement('div');
      modeList.className = 'perm-mode-list';

      var modes = [
        { value: 'everyone', icon: 'fa-globe', label: 'Everyone', desc: 'All campaign members can view; Scribes can edit.' },
        { value: 'dm_only', icon: 'fa-lock', label: 'DM Only', desc: 'Only Scribes and the campaign owner can see this page.' },
        { value: 'custom', icon: 'fa-shield-halved', label: 'Custom', desc: 'Choose exactly who can view or edit this page.' }
      ];

      modes.forEach(function (m) {
        var disabled = draftMode && m.value === 'custom';
        var optionEl = document.createElement('label');
        optionEl.className = 'perm-mode-option' + (mode === m.value ? ' perm-mode-active' : '') + (disabled ? ' perm-mode-disabled' : '');

        var radio = document.createElement('input');
        radio.type = 'radio';
        radio.name = 'perm-mode-' + (config.endpoint || 'draft');
        radio.value = m.value;
        radio.checked = mode === m.value;
        radio.disabled = disabled;
        radio.addEventListener('change', function () {
          if (disabled) return;
          setMode(m.value);
          renderBody();
          renderTrigger();
          if (!draftMode) save();
        });

        var iconEl = document.createElement('span');
        iconEl.className = 'perm-mode-icon';
        iconEl.innerHTML = '<i class="fa-solid ' + m.icon + ' text-xs"></i>';

        var textWrap = document.createElement('div');
        textWrap.style.flex = '1';
        var labelEl = document.createElement('div');
        labelEl.className = 'perm-mode-label';
        labelEl.textContent = m.label;
        var descEl = document.createElement('div');
        descEl.className = 'perm-mode-desc';
        descEl.textContent = m.desc;
        if (disabled) {
          descEl.textContent += ' (Configure after creating the page.)';
        }
        textWrap.appendChild(labelEl);
        textWrap.appendChild(descEl);

        optionEl.appendChild(radio);
        optionEl.appendChild(iconEl);
        optionEl.appendChild(textWrap);
        modeList.appendChild(optionEl);
      });

      bodyEl.appendChild(modeList);

      // Custom grants panel.
      if (mode === 'custom' && !draftMode) {
        var roleHeader = document.createElement('div');
        roleHeader.className = 'perm-section-header';
        roleHeader.textContent = 'Role Permissions';
        bodyEl.appendChild(roleHeader);

        var roleHint = document.createElement('div');
        roleHint.className = 'perm-section-hint';
        roleHint.textContent = 'Granting access to Players also grants access to Scribes.';
        bodyEl.appendChild(roleHint);

        [1, 2].forEach(function (roleNum) {
          var row = createGrantRow(
            roleNames[roleNum], null, roleNum === 1 ? 'fa-user' : 'fa-pen',
            'role', String(roleNum),
            findGrant('role', String(roleNum))
          );
          bodyEl.appendChild(row);
        });

        var nonOwnerMembers = state.members.filter(function (m) { return m.role < ROLE_OWNER; });
        if (nonOwnerMembers.length > 0) {
          var userHeader = document.createElement('div');
          userHeader.className = 'perm-section-header';
          userHeader.textContent = 'Individual Permissions';
          bodyEl.appendChild(userHeader);

          nonOwnerMembers.forEach(function (member) {
            var row = createGrantRow(
              member.display_name || member.email, roleNames[member.role],
              'fa-user', 'user', member.user_id,
              findGrant('user', member.user_id)
            );
            bodyEl.appendChild(row);
          });
        }

        if (state.groups.length > 0) {
          var groupHeader = document.createElement('div');
          groupHeader.className = 'perm-section-header';
          groupHeader.textContent = 'Group Permissions';
          bodyEl.appendChild(groupHeader);

          state.groups.forEach(function (group) {
            var row = createGrantRow(
              group.name, null, 'fa-users',
              'group', String(group.id),
              findGrant('group', String(group.id))
            );
            bodyEl.appendChild(row);
          });
        }

        // Owner members (greyed out, always-on).
        var owners = state.members.filter(function (m) { return m.role >= ROLE_OWNER; });
        if (owners.length > 0) {
          owners.forEach(function (member) {
            var row = document.createElement('div');
            row.className = 'perm-grant-row';
            row.style.opacity = '0.6';

            var nameDiv = document.createElement('div');
            nameDiv.className = 'perm-grant-name';
            nameDiv.innerHTML = '<span class="perm-grant-icon"><i class="fa-solid fa-crown text-xs" style="color: #d97706;"></i></span>' +
              Chronicle.escapeHtml(member.display_name || member.email);
            row.appendChild(nameDiv);

            var accessLabel = document.createElement('span');
            accessLabel.style.fontSize = '12px';
            accessLabel.style.color = 'var(--color-fg-muted, #9ca3af)';
            accessLabel.textContent = 'Full access';
            row.appendChild(accessLabel);

            bodyEl.appendChild(row);
          });
        }

        if (!hasAnyGrants()) {
          var warning = document.createElement('div');
          warning.className = 'perm-warning';
          warning.innerHTML = '<i class="fa-solid fa-triangle-exclamation"></i><span>No grants set — only the campaign owner can see this page.</span>';
          bodyEl.appendChild(warning);
        }
      } else if (mode === 'custom' && draftMode) {
        var draftNote = document.createElement('div');
        draftNote.className = 'perm-draft-note';
        draftNote.textContent = 'Custom permissions are configured after the page is created. The page will be created as DM Only and you can choose grants from its profile.';
        bodyEl.appendChild(draftNote);
        // Map custom-in-draft to dm_only so created entities don't go public.
        state.isPrivate = true;
        syncDraftTarget();
      }
    }

    function createGrantRow(name, roleBadge, icon, subjectType, subjectId, currentPerm) {
      var row = document.createElement('div');
      row.className = 'perm-grant-row';

      var nameDiv = document.createElement('div');
      nameDiv.className = 'perm-grant-name';
      nameDiv.innerHTML = '<span class="perm-grant-icon"><i class="fa-solid ' + Chronicle.escapeHtml(icon) + ' text-xs"></i></span>' +
        '<span class="truncate" style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">' +
        Chronicle.escapeHtml(name) + '</span>';
      if (roleBadge) {
        nameDiv.innerHTML += '<span class="perm-role-badge">' + Chronicle.escapeHtml(roleBadge) + '</span>';
      }
      row.appendChild(nameDiv);

      var toggleGroup = document.createElement('div');
      toggleGroup.className = 'perm-toggle-group';

      ['none', 'view', 'edit'].forEach(function (perm) {
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'perm-toggle' + (currentPerm === perm ? ' perm-toggle-active' : '');
        btn.textContent = perm === 'none' ? 'None' : perm === 'view' ? 'View' : 'Edit';
        btn.addEventListener('click', function () {
          setGrant(subjectType, subjectId, perm);
          renderBody();
          save();
        });
        toggleGroup.appendChild(btn);
      });

      row.appendChild(toggleGroup);
      return row;
    }

    function showStatus(type, message) {
      if (!statusEl) return;
      statusEl.textContent = message || '';
      statusEl.style.color = type === 'saved' ? '#16a34a' : (type === 'error' ? '#dc2626' : 'var(--color-fg-muted, #6b7280)');
    }

    function save() {
      if (!editable || !config.endpoint) return;

      if (state.abortController) {
        state.abortController.abort();
      }
      state.abortController = new AbortController();

      state.saving = true;
      showStatus('saving', 'Saving...');

      var body = {
        visibility: state.visibility,
        is_private: state.isPrivate,
        permissions: state.permissions.map(function (p) {
          return {
            subject_type: p.subject_type,
            subject_id: p.subject_id,
            permission: p.permission
          };
        })
      };

      Chronicle.apiFetch(config.endpoint, {
        method: 'PUT',
        body: body,
        signal: state.abortController.signal
      })
        .then(function (resp) {
          if (!resp.ok) {
            // Surface the structured wire error so operators see the
            // actual cause (validation / auth / not_found) instead of
            // a generic "Failed to save".
            return resp.json().then(
              function (b) {
                var err = new Error((b && (b.message || b.error)) || ('Failed to save permissions (HTTP ' + resp.status + ')'));
                err.category = (b && b.category) || null;
                throw err;
              },
              function () {
                throw new Error('Failed to save permissions (HTTP ' + resp.status + ')');
              }
            );
          }
          state.saving = false;
          state.saved = true;
          state.error = null;
          state.errorCategory = null;
          showStatus('saved', 'Saved');
          setTimeout(function () {
            state.saved = false;
            showStatus('', '');
          }, 2000);
        })
        .catch(function (err) {
          if (err.name === 'AbortError') return;
          state.saving = false;
          state.error = err.message || 'Error saving permissions';
          state.errorCategory = err.category || 'internal';
          showStatus('error', '');
          renderBody();
          console.error('permissions: save error', err);
        });
    }

    function load() {
      if (draftMode) {
        // No endpoint to load from in draft mode; widget operates on
        // local state seeded with defaults (Everyone / is_private=false).
        state.loading = false;
        syncDraftTarget();
        renderTrigger();
        renderBody();
        return;
      }

      if (!config.endpoint) {
        state.loading = false;
        renderBody();
        return;
      }

      Chronicle.apiFetch(config.endpoint)
        .then(function (resp) {
          if (!resp.ok) {
            return resp.json().then(
              function (b) {
                var err = new Error((b && (b.message || b.error)) || 'Failed to load permissions');
                err.category = (b && b.category) || null;
                throw err;
              },
              function () { throw new Error('Failed to load permissions'); }
            );
          }
          return resp.json();
        })
        .then(function (data) {
          state.visibility = data.visibility || 'default';
          state.isPrivate = data.is_private || false;
          state.members = data.members || [];
          state.groups = data.groups || [];
          state.permissions = (data.permissions || []).map(function (p) {
            return {
              subject_type: p.subject_type,
              subject_id: p.subject_id,
              permission: p.permission
            };
          });
          // Tag-derived grants: read-only, additive. Surfaced so an Owner
          // editing permissions sees that a tag has also exposed this page
          // — a tag must never silently widen visibility.
          state.tagGrants = data.tag_grants || [];
          state.loading = false;
          renderTrigger();
          renderBody();
        })
        .catch(function (err) {
          state.loading = false;
          // Covers both a non-2xx response (the 403 a Scribe gets from the
          // Owner-only route) and an outright network failure -- either way
          // state.visibility/isPrivate were never replaced by a real answer,
          // so render must not treat them as one (ADR-057).
          state.loadFailed = true;
          state.error = err.message || 'Failed to load permissions';
          state.errorCategory = err.category || 'internal';
          renderTrigger();
          renderBody();
          console.error('permissions: load error', err);
        });
    }

    function openCard() {
      if (state.open) return;
      state.open = true;
      if (corner) {
        hideUnsaved();
        void card.offsetWidth;
        card.classList.add('perm-open');
        el.classList.add('perm-corner-open');
        trigger.setAttribute('aria-expanded', 'true');
        document.addEventListener('keydown', onEscape);
        document.addEventListener('pointerdown', onOutside, true);
        return;
      }
      // Inline layout: expand the in-flow panel via the grid-rows animation;
      // no backdrop, no body-scroll lock, no global Escape (it's part of the
      // form, not a modal).
      if (inline) {
        void card.offsetWidth;
        card.classList.add('perm-open');
        el.classList.add('perm-inline-open');
        return;
      }
      // Trigger reflow before adding the open class so the transform
      // transition actually fires (otherwise the browser collapses the
      // initial translateX(100%) → translateX(0) into the same frame).
      void card.offsetWidth;
      backdrop.classList.add('perm-open');
      card.classList.add('perm-open');
      document.addEventListener('keydown', onEscape);
      // Prevent body scroll while card is open.
      document.body.style.overflow = 'hidden';
    }

    function closeCard() {
      if (!state.open) return;
      state.open = false;
      if (corner) {
        card.classList.remove('perm-open');
        el.classList.remove('perm-corner-open');
        trigger.setAttribute('aria-expanded', 'false');
        document.removeEventListener('keydown', onEscape);
        document.removeEventListener('pointerdown', onOutside, true);
        hideUnsaved();
        return;
      }
      if (inline) {
        card.classList.remove('perm-open');
        el.classList.remove('perm-inline-open');
        return;
      }
      backdrop.classList.remove('perm-open');
      card.classList.remove('perm-open');
      document.removeEventListener('keydown', onEscape);
      document.body.style.overflow = '';
    }

    function onEscape(e) {
      if (e.key === 'Escape') {
        e.preventDefault();
        closeCard();
      }
    }

    // A change is unsaved while its save is in flight or after it failed.
    function hasUnsaved() {
      return state.saving || (!!state.error && !state.loadFailed);
    }

    // tryClose closes the corner editor unless a change is unsaved; then it
    // stays open and says so, with a small nudge so the warning is seen.
    function tryClose() {
      if (!hasUnsaved()) {
        closeCard();
        return;
      }
      unsavedEl.classList.remove('perm-unsaved-show');
      void unsavedEl.offsetWidth;
      unsavedEl.classList.add('perm-unsaved-show');
      card.classList.remove('perm-nudge');
      void card.offsetWidth;
      card.classList.add('perm-nudge');
    }

    function hideUnsaved() {
      if (unsavedEl) unsavedEl.classList.remove('perm-unsaved-show');
    }

    function onOutside(e) {
      if (!state.open || el.contains(e.target)) return;
      tryClose();
    }

    var unsavedEl = null;

    if (corner) {
      trigger = el.querySelector('[data-perm-corner-trigger]');
      if (!trigger) return;
      trigger.setAttribute('aria-haspopup', 'dialog');
      trigger.setAttribute('aria-expanded', 'false');
      trigger.addEventListener('click', function () {
        if (state.open) {
          tryClose();
        } else {
          openCard();
        }
      });

      card = document.createElement('div');
      card.className = 'perm-corner-panel';
      card.setAttribute('role', 'dialog');
      card.setAttribute('aria-label', 'Who can see this page');
      card.addEventListener('animationend', function () {
        card.classList.remove('perm-nudge');
      });

      var cHeader = document.createElement('div');
      cHeader.className = 'perm-card-header';
      var cTitle = document.createElement('div');
      cTitle.className = 'perm-card-title';
      cTitle.textContent = 'Who can see this page';
      cHeader.appendChild(cTitle);
      var cClose = document.createElement('button');
      cClose.type = 'button';
      cClose.className = 'perm-card-close';
      cClose.setAttribute('aria-label', 'Close');
      cClose.innerHTML = '<i class="fa-solid fa-xmark"></i>';
      cClose.addEventListener('click', tryClose);
      cHeader.appendChild(cClose);
      card.appendChild(cHeader);

      bodyEl = document.createElement('div');
      bodyEl.className = 'perm-card-body';
      card.appendChild(bodyEl);

      unsavedEl = document.createElement('div');
      unsavedEl.className = 'perm-unsaved';
      unsavedEl.setAttribute('role', 'status');
      unsavedEl.textContent = 'Not saved yet. Wait a moment, or press Esc to leave without it.';
      card.appendChild(unsavedEl);

      var cFooter = document.createElement('div');
      cFooter.className = 'perm-card-footer';
      statusEl = document.createElement('span');
      cFooter.appendChild(statusEl);
      card.appendChild(cFooter);

      el.appendChild(card);

      el._permState = state;
      el._permCard = card;
      el._permBackdrop = null;
      el._permOnEscape = onEscape;
      el._permOnOutside = onOutside;

      load();
      return;
    }

    // Build the trigger inside the mount element.
    el.innerHTML = '';
    trigger = document.createElement('button');
    trigger.type = 'button';
    trigger.className = 'perm-trigger';
    // Inline mode is a disclosure (expands in place), not a modal dialog.
    trigger.setAttribute('aria-haspopup', inline ? 'true' : 'dialog');
    trigger.setAttribute('aria-expanded', 'false');
    trigger.addEventListener('click', function () {
      if (state.open) {
        closeCard();
      } else {
        openCard();
      }
      trigger.setAttribute('aria-expanded', state.open ? 'true' : 'false');
    });

    if (inline) {
      // Inline layout: a summary trigger over a collapsible in-flow panel.
      // No backdrop, no body-attached fixed card — everything lives inside
      // the mount so it animates open within the form flow.
      el.classList.add('perm-inline');
      el.appendChild(trigger);

      card = document.createElement('div');
      card.className = 'perm-inline-panel';
      card.setAttribute('role', 'region');
      card.setAttribute('aria-label', 'Page permissions');

      var inner = document.createElement('div');
      inner.className = 'perm-inline-inner';

      bodyEl = document.createElement('div');
      bodyEl.className = 'perm-card-body';
      inner.appendChild(bodyEl);

      var inlineFooter = document.createElement('div');
      inlineFooter.className = 'perm-card-footer';
      statusEl = document.createElement('span');
      inlineFooter.appendChild(statusEl);
      inner.appendChild(inlineFooter);

      card.appendChild(inner);
      el.appendChild(card);

      backdrop = null;
      el._permState = state;
      el._permCard = card;
      el._permBackdrop = null;
      el._permOnEscape = null;

      renderTrigger();
      load();
      return;
    }

    el.appendChild(trigger);

    // Build the backdrop + card and attach to body so they overlay
    // everything regardless of where the mount is in the DOM tree.
    backdrop = document.createElement('div');
    backdrop.className = 'perm-backdrop';
    backdrop.addEventListener('click', closeCard);
    document.body.appendChild(backdrop);

    card = document.createElement('div');
    card.className = 'perm-card';
    card.setAttribute('role', 'dialog');
    card.setAttribute('aria-label', 'Page permissions');

    var header = document.createElement('div');
    header.className = 'perm-card-header';
    var titleEl = document.createElement('div');
    titleEl.className = 'perm-card-title';
    titleEl.innerHTML = '<i class="fa-solid fa-shield-halved" style="color: var(--color-accent, #6366f1);"></i><span>Page Permissions</span>';
    header.appendChild(titleEl);
    var closeBtn = document.createElement('button');
    closeBtn.type = 'button';
    closeBtn.className = 'perm-card-close';
    closeBtn.setAttribute('aria-label', 'Close permissions');
    closeBtn.innerHTML = '<i class="fa-solid fa-xmark"></i>';
    closeBtn.addEventListener('click', closeCard);
    header.appendChild(closeBtn);
    card.appendChild(header);

    bodyEl = document.createElement('div');
    bodyEl.className = 'perm-card-body';
    card.appendChild(bodyEl);

    var footer = document.createElement('div');
    footer.className = 'perm-card-footer';
    statusEl = document.createElement('span');
    footer.appendChild(statusEl);
    card.appendChild(footer);

    document.body.appendChild(card);

    el._permState = state;
    el._permCard = card;
    el._permBackdrop = backdrop;
    el._permOnEscape = onEscape;

    renderTrigger();
    load();
  },

  destroy: function (el) {
    if (el._permState && el._permState.abortController) {
      el._permState.abortController.abort();
    }
    if (el._permOnEscape) {
      document.removeEventListener('keydown', el._permOnEscape);
    }
    if (el._permOnOutside) {
      document.removeEventListener('pointerdown', el._permOnOutside, true);
    }
    if (el._permCard && el._permCard.parentNode) {
      el._permCard.parentNode.removeChild(el._permCard);
    }
    if (el._permBackdrop && el._permBackdrop.parentNode) {
      el._permBackdrop.parentNode.removeChild(el._permBackdrop);
    }
    document.body.style.overflow = '';
    delete el._permState;
    delete el._permCard;
    delete el._permBackdrop;
    delete el._permOnEscape;
    delete el._permOnOutside;
    // The corner mount's control is server-rendered and outlives the widget.
    if (el.getAttribute('data-layout') !== 'corner') el.innerHTML = '';
  }
});
