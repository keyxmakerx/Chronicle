/**
 * Chronicle Sync - Sync Manager
 *
 * Orchestrates sync state across all sync modules (journals, maps, calendar).
 * Manages the API client lifecycle, initial sync on connect, and coordinates
 * conflict resolution.
 */

import { ChronicleAPI } from './api-client.mjs';
import { getSetting, setSetting, isConfigured } from './settings.mjs';

/**
 * SyncManager coordinates all Chronicle sync operations.
 * It owns the API client and delegates to feature-specific sync modules.
 */
export class SyncManager {
  constructor() {
    /** @type {ChronicleAPI} */
    this.api = new ChronicleAPI();

    /** @type {Array<object>} Registered sync modules. */
    this._modules = [];

    /** @type {boolean} Whether initial sync has completed. */
    this._initialSyncDone = false;
  }

  /**
   * Register a sync module (e.g., JournalSync, MapSync).
   * Modules must implement: init(api), onMessage(msg), destroy().
   * @param {object} module
   */
  registerModule(module) {
    this._modules.push(module);
  }

  /**
   * Start the sync manager. Connects to Chronicle and performs initial sync.
   * Only runs for GMs when the module is configured.
   */
  async start() {
    if (!game.user.isGM) {
      console.log('Chronicle: Non-GM user, skipping sync');
      return;
    }

    if (!isConfigured()) {
      console.warn('Chronicle: Module not configured. Set URL, API key, and campaign ID in settings.');
      ui.notifications.warn('Chronicle Sync is not configured. Go to Module Settings to set up.');
      return;
    }

    if (!getSetting('syncEnabled')) {
      console.log('Chronicle: Sync disabled in settings');
      return;
    }

    // Initialize all registered modules.
    for (const mod of this._modules) {
      if (typeof mod.init === 'function') {
        await mod.init(this.api);
      }
    }

    // Listen for all WebSocket messages and route to modules.
    this.api.on('*', (msg) => this._routeMessage(msg));

    // Connect WebSocket.
    this.api.connect();

    // Listen for connection state changes.
    this.api.on('sync.status', async (msg) => {
      if (msg.payload?.status === 'connected' && !this._initialSyncDone) {
        await this._performInitialSync();
        this._initialSyncDone = true;
      }
    });

    console.log('Chronicle: Sync manager started');
  }

  /**
   * Stop the sync manager and disconnect.
   */
  stop() {
    this.api.disconnect();
    for (const mod of this._modules) {
      if (typeof mod.destroy === 'function') {
        mod.destroy();
      }
    }
    this._modules = [];
    this._initialSyncDone = false;
    console.log('Chronicle: Sync manager stopped');
  }

  /**
   * Perform initial sync: pull all changes since last sync time.
   * @private
   */
  async _performInitialSync() {
    const lastSync = getSetting('lastSyncTime') || '1970-01-01T00:00:00Z';

    try {
      console.log(`Chronicle: Initial sync from ${lastSync}`);

      // Pull sync mappings modified since last sync.
      const result = await this.api.get(`/sync/pull?since=${encodeURIComponent(lastSync)}`);

      if (result.mappings && result.mappings.length > 0) {
        console.log(`Chronicle: Received ${result.mappings.length} mapping updates`);

        // Route each mapping to the appropriate module.
        for (const mapping of result.mappings) {
          for (const mod of this._modules) {
            if (typeof mod.onSyncMapping === 'function') {
              await mod.onSyncMapping(mapping);
            }
          }
        }
      }

      // Let each module perform its own initial sync (e.g., calendar structure).
      for (const mod of this._modules) {
        if (typeof mod.onInitialSync === 'function') {
          await mod.onInitialSync();
        }
      }

      // Update last sync timestamp.
      await setSetting('lastSyncTime', result.server_time || new Date().toISOString());

      ui.notifications.info('Chronicle: Initial sync complete');
    } catch (err) {
      console.error('Chronicle: Initial sync failed', err);
      ui.notifications.error('Chronicle: Initial sync failed. Check console for details.');
    }
  }

  /**
   * Route an incoming WebSocket message to all registered modules.
   * @param {object} msg
   * @private
   */
  _routeMessage(msg) {
    for (const mod of this._modules) {
      if (typeof mod.onMessage === 'function') {
        try {
          mod.onMessage(msg);
        } catch (err) {
          console.error(`Chronicle: Module message handler error`, err);
        }
      }
    }
  }

  /**
   * Create or update a sync mapping on the server.
   * @param {object} mapping
   * @returns {Promise<object>}
   */
  async createMapping(mapping) {
    return this.api.post('/sync/mappings', mapping);
  }

  /**
   * Look up a sync mapping by Chronicle identity.
   * @param {string} chronicleType
   * @param {string} chronicleId
   * @returns {Promise<object|null>}
   */
  async findMapping(chronicleType, chronicleId) {
    try {
      return await this.api.get(
        `/sync/lookup?chronicle_type=${encodeURIComponent(chronicleType)}&chronicle_id=${encodeURIComponent(chronicleId)}`
      );
    } catch {
      return null;
    }
  }

  /**
   * Look up a sync mapping by Foundry document ID.
   * @param {string} externalId - Foundry document ID.
   * @returns {Promise<object|null>}
   */
  async findMappingByExternal(externalId) {
    try {
      return await this.api.get(
        `/sync/lookup?external_system=foundry&external_id=${encodeURIComponent(externalId)}`
      );
    } catch {
      return null;
    }
  }
}
