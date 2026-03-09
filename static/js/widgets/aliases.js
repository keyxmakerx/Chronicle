/**
 * aliases.js -- Entity Aliases Widget
 *
 * Displays and manages alternative names (aliases) for an entity.
 * Aliases appear in auto-linking, search, and @mention results.
 *
 * Data attributes:
 *   data-widget="aliases"
 *   data-entity-id   -- Entity UUID
 *   data-campaign-id -- Campaign UUID
 *   data-editable    -- "true" if the user can add/remove aliases (Scribe+)
 */
(function () {
  'use strict';

  if (!window.Chronicle) return;

  Chronicle.register('aliases', {
    init: function (el, config) {
      var state = {
        el: el,
        entityId: config.entityId || el.dataset.entityId,
        campaignId: config.campaignId || el.dataset.campaignId,
        editable: el.dataset.editable === 'true',
        aliases: [],
        saving: false,
      };

      loadAliases(state);
      return { destroy: function () { el.innerHTML = ''; } };
    },
  });

  function apiUrl(state) {
    return '/campaigns/' + state.campaignId + '/entities/' + state.entityId + '/aliases';
  }

  function loadAliases(state) {
    Chronicle.apiFetch(apiUrl(state))
      .then(function (resp) {
        if (!resp.ok) throw new Error('Failed to load aliases');
        return resp.json();
      })
      .then(function (data) {
        state.aliases = (data.aliases || []).map(function (a) { return a.alias; });
        render(state);
      })
      .catch(function (err) {
        console.error('[Aliases] Load failed:', err);
      });
  }

  function saveAliases(state) {
    if (state.saving) return;
    state.saving = true;

    Chronicle.apiFetch(apiUrl(state), {
      method: 'PUT',
      body: { aliases: state.aliases },
    })
      .then(function (resp) {
        state.saving = false;
        if (!resp.ok) throw new Error('Failed to save aliases');
        // Invalidate auto-link cache so new aliases are picked up.
        if (Chronicle.invalidateAutoLinkCache) {
          Chronicle.invalidateAutoLinkCache(state.campaignId);
        }
      })
      .catch(function (err) {
        state.saving = false;
        console.error('[Aliases] Save failed:', err);
        Chronicle.notify('Failed to save aliases.', 'error');
      });
  }

  function render(state) {
    var el = state.el;

    // Don't render anything if no aliases and not editable.
    if (state.aliases.length === 0 && !state.editable) {
      el.innerHTML = '';
      return;
    }

    var html = '<div class="flex flex-wrap items-center gap-1.5 mt-1">';

    // Label.
    if (state.aliases.length > 0 || state.editable) {
      html += '<span class="text-xs text-fg-muted italic">aka</span>';
    }

    // Alias chips.
    for (var i = 0; i < state.aliases.length; i++) {
      html += '<span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-surface-alt border border-edge-light text-xs text-fg-secondary">';
      html += Chronicle.escapeHTML(state.aliases[i]);
      if (state.editable) {
        html += ' <button class="hover:text-red-400 transition-colors alias-remove" data-index="' + i + '" title="Remove alias">';
        html += '<i class="fa-solid fa-xmark text-[9px]"></i>';
        html += '</button>';
      }
      html += '</span>';
    }

    // Add button (Scribe+ only).
    if (state.editable && state.aliases.length < 10) {
      html += '<button class="alias-add inline-flex items-center gap-1 px-2 py-0.5 rounded-full border border-dashed border-edge text-xs text-fg-muted hover:text-fg hover:border-edge-light transition-colors">';
      html += '<i class="fa-solid fa-plus text-[9px]"></i> alias';
      html += '</button>';
    }

    html += '</div>';
    el.innerHTML = html;

    // Wire events.
    var removeButtons = el.querySelectorAll('.alias-remove');
    for (var j = 0; j < removeButtons.length; j++) {
      removeButtons[j].addEventListener('click', function (e) {
        e.preventDefault();
        e.stopPropagation();
        var idx = parseInt(this.dataset.index, 10);
        state.aliases.splice(idx, 1);
        saveAliases(state);
        render(state);
      });
    }

    var addBtn = el.querySelector('.alias-add');
    if (addBtn) {
      addBtn.addEventListener('click', function (e) {
        e.preventDefault();
        e.stopPropagation();
        showAddInput(state, addBtn);
      });
    }
  }

  function showAddInput(state, addBtn) {
    // Replace the add button with an input field.
    var wrapper = document.createElement('span');
    wrapper.className = 'inline-flex items-center';
    var input = document.createElement('input');
    input.type = 'text';
    input.className = 'border border-edge rounded px-2 py-0.5 text-xs bg-surface text-fg w-32 focus:outline-none focus:border-accent';
    input.placeholder = 'New alias…';
    input.maxLength = 200;
    wrapper.appendChild(input);
    addBtn.replaceWith(wrapper);
    input.focus();

    function commit() {
      var value = input.value.trim();
      if (value.length >= 2 && state.aliases.indexOf(value) === -1) {
        state.aliases.push(value);
        saveAliases(state);
      }
      render(state);
    }

    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') {
        e.preventDefault();
        commit();
      } else if (e.key === 'Escape') {
        render(state);
      }
    });

    input.addEventListener('blur', function () {
      commit();
    });
  }
})();
