/**
 * shortcuts_help.js -- Keyboard Shortcuts Help Overlay
 *
 * Pressing "?" (when not typing in an input) shows an overlay listing all
 * available keyboard shortcuts. Pressing "?" again or Escape closes it.
 */
(function () {
  'use strict';

  var overlay = null;

  var isMac = /Mac|iPod|iPhone|iPad/.test(navigator.platform);
  var mod = isMac ? '⌘' : 'Ctrl';

  var shortcuts = [
    { keys: mod + '+K', desc: 'Quick search (pages, maps, events, sessions)' },
    { keys: mod + '+N', desc: 'New page' },
    { keys: mod + '+E', desc: 'Edit current page' },
    { keys: mod + '+S', desc: 'Save' },
    { keys: '?', desc: 'Show this help' },
  ];

  function buildOverlay() {
    var el = document.createElement('div');
    el.id = 'shortcuts-help';
    el.style.cssText = 'position:fixed;inset:0;z-index:9999;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,.5)';
    el.onclick = function (e) { if (e.target === el) close(); };

    var card = document.createElement('div');
    card.className = 'card';
    card.style.cssText = 'width:100%;max-width:360px;padding:1.5rem;';

    var h = document.createElement('h2');
    h.className = 'text-lg font-semibold text-fg mb-4';
    h.textContent = 'Keyboard Shortcuts';
    card.appendChild(h);

    var list = document.createElement('div');
    list.className = 'space-y-2';

    for (var i = 0; i < shortcuts.length; i++) {
      var row = document.createElement('div');
      row.className = 'flex items-center justify-between';

      var desc = document.createElement('span');
      desc.className = 'text-sm text-fg-body';
      desc.textContent = shortcuts[i].desc;

      var kbd = document.createElement('kbd');
      kbd.className = 'px-2 py-0.5 text-xs font-mono bg-surface-alt border border-edge rounded text-fg-secondary';
      kbd.textContent = shortcuts[i].keys;

      row.appendChild(desc);
      row.appendChild(kbd);
      list.appendChild(row);
    }

    card.appendChild(list);

    var footer = document.createElement('div');
    footer.className = 'mt-4 pt-3 border-t border-edge text-right';
    var closeBtn = document.createElement('button');
    closeBtn.className = 'btn-secondary text-xs';
    closeBtn.textContent = 'Close';
    closeBtn.onclick = close;
    footer.appendChild(closeBtn);
    card.appendChild(footer);

    el.appendChild(card);
    return el;
  }

  function open() {
    if (overlay) return;
    overlay = buildOverlay();
    document.body.appendChild(overlay);
  }

  function close() {
    if (!overlay) return;
    overlay.remove();
    overlay = null;
  }

  function isTyping() {
    var el = document.activeElement;
    if (!el) return false;
    var tag = el.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
    if (el.isContentEditable) return true;
    return false;
  }

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && overlay) {
      e.preventDefault();
      close();
      return;
    }
    if (e.key === '?' && !e.ctrlKey && !e.metaKey && !isTyping()) {
      e.preventDefault();
      if (overlay) close(); else open();
    }
  });
})();
