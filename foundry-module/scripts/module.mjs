/**
 * Chronicle Sync - Foundry VTT Module Entry Point
 *
 * Registers settings, initializes the sync manager, and connects to the
 * Chronicle backend on the 'ready' hook. All sync features are coordinated
 * through the SyncManager which owns the WebSocket connection.
 */

import { registerSettings } from './settings.mjs';
import { SyncManager } from './sync-manager.mjs';
import { JournalSync } from './journal-sync.mjs';
import { MapSync } from './map-sync.mjs';
import { ShopWidget } from './shop-widget.mjs';
import { CalendarSync } from './calendar-sync.mjs';
import { SyncDashboard } from './sync-dashboard.mjs';

/** @type {SyncManager|null} */
let syncManager = null;

/** @type {SyncDashboard|null} */
let dashboard = null;

/**
 * Module initialization — register settings.
 * This runs before the game is fully ready.
 */
Hooks.once('init', () => {
  console.log('Chronicle Sync | Initializing');
  registerSettings();
});

/**
 * Game ready — start the sync manager and connect to Chronicle.
 * Only GMs run the full sync; players just get passive updates via
 * Foundry's native document sync.
 */
Hooks.once('ready', async () => {
  console.log('Chronicle Sync | Ready');

  syncManager = new SyncManager();

  // Register all sync modules.
  syncManager.registerModule(new JournalSync());
  syncManager.registerModule(new MapSync());
  syncManager.registerModule(new ShopWidget());
  syncManager.registerModule(new CalendarSync());

  // Start the sync manager (connects WebSocket, performs initial sync).
  await syncManager.start();

  // Create the sync dashboard (singleton, rendered on demand).
  dashboard = new SyncDashboard();
  dashboard.bind(syncManager);

  // Add sync status indicator to the UI.
  _addStatusIndicator();
});

/**
 * Add a Chronicle Sync button to Foundry's scene controls toolbar.
 * Visible only to GMs. Opens the Sync Dashboard on click.
 */
Hooks.on('getSceneControlButtons', (controls) => {
  if (!game.user.isGM) return;

  controls.push({
    name: 'chronicle-sync',
    title: 'Chronicle Sync',
    icon: 'fa-solid fa-rotate',
    layer: 'controls',
    visible: true,
    tools: [{
      name: 'dashboard',
      title: 'Open Chronicle Sync Dashboard',
      icon: 'fa-solid fa-rotate',
      button: true,
      onClick: () => {
        if (dashboard) dashboard.render(true);
      },
    }],
  });
});

/**
 * Clean up when the module is deactivated.
 */
Hooks.on('closeGame', () => {
  if (syncManager) {
    syncManager.stop();
    syncManager = null;
  }
});

/**
 * Add a sync status indicator to Foundry's sidebar.
 * Shows connection state (green/yellow/red dot) with click-to-reconnect
 * and event-driven updates (no polling).
 * @private
 */
function _addStatusIndicator() {
  if (!syncManager) return;

  const indicator = document.createElement('div');
  indicator.className = 'chronicle-sync-status disconnected';
  indicator.innerHTML = `
    <span class="status-dot"></span>
    <span class="status-text">Chronicle</span>
  `;

  // Update indicator based on WebSocket state.
  const updateState = (state) => {
    if (!state) state = syncManager.api.state;
    indicator.className = `chronicle-sync-status ${state}`;
    indicator.title = _statusTooltip(state);

    const text = indicator.querySelector('.status-text');
    switch (state) {
      case 'connected':
        text.textContent = 'Chronicle: Connected';
        break;
      case 'connecting':
        text.textContent = 'Chronicle: Connecting...';
        break;
      case 'reconnecting':
        text.textContent = 'Chronicle: Reconnecting...';
        break;
      default:
        text.textContent = 'Chronicle: Disconnected';
    }
  };

  // Event-driven state updates (no polling).
  syncManager.api.onStateChange(updateState);
  updateState();

  // Click to reconnect when disconnected.
  indicator.addEventListener('click', () => {
    const state = syncManager.api.state;
    if (state === 'disconnected') {
      syncManager.api.connect();
    }
  });

  // Flash the dot briefly when a WS message arrives (activity indicator).
  syncManager.api.on('*', () => {
    const dot = indicator.querySelector('.status-dot');
    if (dot && syncManager.api.state === 'connected') {
      dot.classList.add('activity');
      setTimeout(() => dot.classList.remove('activity'), 300);
    }
  });

  // Append to sidebar header or player list.
  const sidebar = document.getElementById('sidebar');
  if (sidebar) {
    sidebar.prepend(indicator);
  }
}

/**
 * Return tooltip text for the connection status indicator.
 * @param {string} state
 * @returns {string}
 * @private
 */
function _statusTooltip(state) {
  switch (state) {
    case 'connected':
      return 'Connected to Chronicle. Real-time sync active.';
    case 'connecting':
      return 'Connecting to Chronicle...';
    case 'reconnecting':
      return 'Connection lost. Reconnecting automatically...';
    default:
      return 'Disconnected from Chronicle. Click to reconnect.';
  }
}

/**
 * Expose the sync manager globally for debugging and integration.
 * Access via `game.modules.get('chronicle-sync').api`.
 */
Hooks.once('ready', () => {
  const module = game.modules.get('chronicle-sync');
  if (module) {
    module.api = {
      syncManager,
      dashboard,
      getAPI: () => syncManager?.api,
      openDashboard: () => dashboard?.render(true),
    };
  }
});
