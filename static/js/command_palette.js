/**
 * command_palette.js -- Command Palette (Ctrl+Shift+P / Cmd+Shift+P)
 *
 * VSCode-style action palette for fast navigation and actions within
 * Chronicle. Lists available commands filtered by context (campaign scope,
 * active page). Fuzzy substring matching on command labels.
 *
 * Features:
 *   - Ctrl+Shift+P / Cmd+Shift+P keyboard shortcut (global)
 *   - Keyboard navigation: Arrow keys, Enter to run, Escape to close
 *   - Context-aware: shows campaign commands only inside campaigns
 *   - Dispatches custom events for actions (quick capture, search, theme)
 *   - Click outside or press Escape to dismiss
 */
(function () {
  'use strict';

  // --- State ---
  var overlay = null;
  var input = null;
  var resultsList = null;
  var activeIndex = -1;
  var commands = [];
  var filtered = [];
  var isOpen = false;

  var isMac = /Mac|iPod|iPhone|iPad/.test(navigator.platform);
  var mod = isMac ? '⌘' : 'Ctrl';

  // --- Helpers ---

  function getCampaignId() {
    var parts = window.location.pathname.split('/');
    if (parts.length >= 3 && parts[1] === 'campaigns' && parts[2] !== '' &&
        parts[2] !== 'new' && parts[2] !== 'picker' && parts[2] !== 'import') {
      return parts[2];
    }
    // Also check data attribute on sidebar.
    var el = document.querySelector('[data-campaign-id]');
    if (el) return el.getAttribute('data-campaign-id');
    return '';
  }

  function isTyping() {
    var el = document.activeElement;
    if (!el) return false;
    var tag = el.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
    if (el.isContentEditable) return true;
    return false;
  }

  // --- Command Definitions ---

  function buildCommands() {
    var cid = getCampaignId();
    var cmds = [];

    // Global commands.
    cmds.push({ label: 'Go to Campaigns', icon: 'fa-dice-d20', href: '/campaigns', shortcut: '' });

    if (cid) {
      // Campaign navigation.
      cmds.push({ label: 'Go to Campaign Home', icon: 'fa-home', href: '/campaigns/' + cid, shortcut: '' });
      cmds.push({ label: 'Go to Calendar', icon: 'fa-calendar-days', href: '/campaigns/' + cid + '/calendar', shortcut: '' });
      cmds.push({ label: 'Go to Timelines', icon: 'fa-timeline', href: '/campaigns/' + cid + '/timelines', shortcut: '' });
      cmds.push({ label: 'Go to Maps', icon: 'fa-map', href: '/campaigns/' + cid + '/maps', shortcut: '' });
      cmds.push({ label: 'Go to Sessions', icon: 'fa-dice-d20', href: '/campaigns/' + cid + '/sessions', shortcut: '' });
      cmds.push({ label: 'Go to Journal', icon: 'fa-book', href: '/campaigns/' + cid + '/journal', shortcut: '' });
      cmds.push({ label: 'Go to Media', icon: 'fa-images', href: '/campaigns/' + cid + '/media', shortcut: '' });
      cmds.push({ label: 'Go to Members', icon: 'fa-users', href: '/campaigns/' + cid + '/members', shortcut: '' });
      cmds.push({ label: 'Go to NPCs', icon: 'fa-people-group', href: '/campaigns/' + cid + '/npcs', shortcut: '' });
      cmds.push({ label: 'Go to Relations Graph', icon: 'fa-diagram-project', href: '/campaigns/' + cid + '/relations-graph/page', shortcut: '' });
      cmds.push({ label: 'Go to Settings', icon: 'fa-gear', href: '/campaigns/' + cid + '/settings', shortcut: '' });
      cmds.push({ label: 'Go to Customize', icon: 'fa-palette', href: '/campaigns/' + cid + '/customize', shortcut: '' });
      cmds.push({ label: 'Go to Plugins', icon: 'fa-puzzle-piece', href: '/campaigns/' + cid + '/plugins', shortcut: '' });

      // Actions.
      cmds.push({ label: 'New Page', icon: 'fa-plus', href: '/campaigns/' + cid + '/entities/new', shortcut: mod + '+N' });
      cmds.push({
        label: 'Quick Capture Note', icon: 'fa-bolt', shortcut: mod + '+Shift+N',
        action: function () { window.dispatchEvent(new CustomEvent('chronicle:quick-capture')); }
      });
    }

    // Universal actions.
    cmds.push({
      label: 'Search Pages', icon: 'fa-search', shortcut: mod + '+K',
      action: function () {
        if (typeof Chronicle !== 'undefined' && Chronicle.openSearch) {
          Chronicle.openSearch();
        }
      }
    });
    cmds.push({
      label: 'Toggle Theme', icon: 'fa-circle-half-stroke', shortcut: '',
      action: function () {
        var html = document.documentElement;
        var isDark = html.classList.contains('dark');
        html.classList.toggle('dark', !isDark);
        try { localStorage.setItem('theme', isDark ? 'light' : 'dark'); } catch (e) { /* noop */ }
      }
    });
    cmds.push({
      label: 'Keyboard Shortcuts', icon: 'fa-keyboard', shortcut: '?',
      action: function () {
        // Simulate pressing ? to trigger shortcuts_help.js.
        document.dispatchEvent(new KeyboardEvent('keydown', { key: '?' }));
      }
    });

    return cmds;
  }

  // --- Fuzzy Filter ---

  function filterCommands(query) {
    if (!query) return commands.slice();
    var q = query.toLowerCase();
    var scored = [];
    for (var i = 0; i < commands.length; i++) {
      var label = commands[i].label.toLowerCase();
      var idx = label.indexOf(q);
      if (idx >= 0) {
        scored.push({ cmd: commands[i], score: idx });
      }
    }
    scored.sort(function (a, b) { return a.score - b.score; });
    return scored.map(function (s) { return s.cmd; });
  }

  // --- DOM Construction ---

  function buildModal() {
    overlay = document.createElement('div');
    overlay.className = 'fixed inset-0 z-[100] flex items-start justify-center pt-[15vh] bg-black/60';
    overlay.style.display = 'none';
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) close();
    });

    var modal = document.createElement('div');
    modal.className = 'card w-full max-w-lg shadow-2xl overflow-hidden';
    modal.style.maxHeight = '60vh';
    modal.style.display = 'flex';
    modal.style.flexDirection = 'column';

    // Search input.
    var inputWrap = document.createElement('div');
    inputWrap.className = 'flex items-center gap-2 px-4 py-3 border-b border-edge';

    var icon = document.createElement('i');
    icon.className = 'fa-solid fa-terminal text-fg-muted text-sm';
    inputWrap.appendChild(icon);

    input = document.createElement('input');
    input.type = 'text';
    input.placeholder = 'Type a command…';
    input.className = 'flex-1 bg-transparent text-fg text-sm outline-none placeholder:text-fg-muted';
    input.addEventListener('input', onInput);
    input.addEventListener('keydown', onKeydown);
    inputWrap.appendChild(input);

    var kbdHint = document.createElement('kbd');
    kbdHint.className = 'text-[10px] font-mono text-fg-muted bg-surface-alt border border-edge rounded px-1.5 py-0.5';
    kbdHint.textContent = 'Esc';
    inputWrap.appendChild(kbdHint);

    modal.appendChild(inputWrap);

    // Results list.
    resultsList = document.createElement('div');
    resultsList.className = 'overflow-y-auto flex-1';
    modal.appendChild(resultsList);

    overlay.appendChild(modal);
    document.body.appendChild(overlay);
  }

  // --- Rendering ---

  function renderResults() {
    var h = '';
    if (filtered.length === 0) {
      h = '<div class="px-4 py-6 text-center text-fg-muted text-sm">No matching commands</div>';
    } else {
      for (var i = 0; i < filtered.length; i++) {
        var cmd = filtered[i];
        var isActive = i === activeIndex;
        h += '<div class="cmd-item flex items-center justify-between px-4 py-2.5 cursor-pointer transition-colors '
          + (isActive ? 'bg-accent/10 text-accent' : 'text-fg hover:bg-surface-alt')
          + '" data-idx="' + i + '">';
        h += '<div class="flex items-center gap-3 min-w-0">';
        h += '<i class="fa-solid ' + (cmd.icon || 'fa-chevron-right') + ' text-xs w-4 text-center '
          + (isActive ? 'text-accent' : 'text-fg-muted') + '"></i>';
        h += '<span class="text-sm truncate">' + Chronicle.escapeHtml(cmd.label) + '</span>';
        h += '</div>';
        if (cmd.shortcut) {
          h += '<kbd class="text-[10px] font-mono ml-2 shrink-0 '
            + (isActive ? 'text-accent/70' : 'text-fg-muted')
            + ' bg-surface-alt border border-edge rounded px-1.5 py-0.5">'
            + Chronicle.escapeHtml(cmd.shortcut) + '</kbd>';
        }
        h += '</div>';
      }
    }
    resultsList.innerHTML = h;

    // Delegated click handler.
    var items = resultsList.querySelectorAll('.cmd-item');
    for (var j = 0; j < items.length; j++) {
      items[j].addEventListener('click', function () {
        var idx = parseInt(this.getAttribute('data-idx'), 10);
        executeCommand(idx);
      });
      items[j].addEventListener('mouseenter', function () {
        activeIndex = parseInt(this.getAttribute('data-idx'), 10);
        renderResults();
      });
    }

    // Scroll active into view.
    if (activeIndex >= 0) {
      var active = resultsList.querySelector('.cmd-item[data-idx="' + activeIndex + '"]');
      if (active) active.scrollIntoView({ block: 'nearest' });
    }
  }

  // --- Event Handlers ---

  function onInput() {
    filtered = filterCommands(input.value.trim());
    activeIndex = filtered.length > 0 ? 0 : -1;
    renderResults();
  }

  function onKeydown(e) {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        if (filtered.length > 0) {
          activeIndex = (activeIndex + 1) % filtered.length;
          renderResults();
        }
        break;
      case 'ArrowUp':
        e.preventDefault();
        if (filtered.length > 0) {
          activeIndex = activeIndex <= 0 ? filtered.length - 1 : activeIndex - 1;
          renderResults();
        }
        break;
      case 'Enter':
        e.preventDefault();
        executeCommand(activeIndex);
        break;
      case 'Escape':
        e.preventDefault();
        close();
        break;
    }
  }

  function executeCommand(idx) {
    if (idx < 0 || idx >= filtered.length) return;
    var cmd = filtered[idx];
    close();
    if (cmd.action) {
      cmd.action();
    } else if (cmd.href) {
      window.location.href = cmd.href;
    }
  }

  // --- Open / Close ---

  function open() {
    if (isOpen) return;
    isOpen = true;

    if (!overlay) buildModal();

    commands = buildCommands();
    filtered = commands.slice();
    activeIndex = 0;

    input.value = '';
    overlay.style.display = '';
    renderResults();

    requestAnimationFrame(function () { input.focus(); });
  }

  function close() {
    if (!isOpen) return;
    isOpen = false;
    if (overlay) overlay.style.display = 'none';
  }

  // --- Keyboard Shortcut ---

  document.addEventListener('keydown', function (e) {
    var mod = e.ctrlKey || e.metaKey;
    if (mod && e.shiftKey && e.key === 'P') {
      e.preventDefault();
      if (isOpen) close(); else open();
      return;
    }
    // Close on Escape if open.
    if (e.key === 'Escape' && isOpen) {
      e.preventDefault();
      close();
    }
  });

  // Close on HTMX navigation.
  document.addEventListener('chronicle:navigated', function () {
    if (isOpen) close();
  });

  // Public API.
  if (typeof Chronicle !== 'undefined') {
    Chronicle.openCommandPalette = open;
    Chronicle.closeCommandPalette = close;
  }
})();
