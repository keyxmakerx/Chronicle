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

/** @type {SyncManager|null} */
let syncManager = null;

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

  // Add sync status indicator to the UI.
  _addStatusIndicator();
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
 * Shows connection state (green/yellow/red dot).
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
  const updateState = () => {
    const state = syncManager.api.state;
    indicator.className = `chronicle-sync-status ${state}`;

    const text = indicator.querySelector('.status-text');
    switch (state) {
      case 'connected':
        text.textContent = 'Chronicle: Connected';
        break;
      case 'reconnecting':
        text.textContent = 'Chronicle: Reconnecting...';
        break;
      default:
        text.textContent = 'Chronicle: Disconnected';
    }
  };

  // Poll state every 2 seconds (lightweight, just checks a string).
  setInterval(updateState, 2000);
  updateState();

  // Append to sidebar header or player list.
  const sidebar = document.getElementById('sidebar');
  if (sidebar) {
    sidebar.prepend(indicator);
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
      getAPI: () => syncManager?.api,
    };
  }
});
