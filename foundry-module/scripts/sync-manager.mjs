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
 * Maps Foundry game.system.id values to Chronicle system IDs.
 * Only systems with an explicit mapping can enable character sync.
 */
const SYSTEM_MAP = {
  dnd5e: 'dnd5e',
  pf2e: 'pathfinder2e',
  drawsteel: 'drawsteel',
};

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

    /** @type {Array<{time: number, type: string, message: string}>} Recent activity log. */
    this._activityLog = [];

    /** @type {number} Maximum activity log entries. */
    this._maxLogEntries = 100;

    /** @type {string|null} Matched Chronicle system ID, or null if no match. */
    this._matchedSystem = null;

    /** @type {string|null} Foundry's game.system.id. */
    this._foundrySystemId = null;
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

    // Detect game system and match against Chronicle systems.
    await this._detectSystem();

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
   * Returns the matched Chronicle system ID, or null if no match.
   * @returns {string|null}
   */
  getMatchedSystem() {
    return this._matchedSystem;
  }

  /**
   * Returns Foundry's game.system.id.
   * @returns {string|null}
   */
  getFoundrySystemId() {
    return this._foundrySystemId;
  }

  /**
   * Detect the Foundry game system and match it against Chronicle systems.
   * Queries the /systems API endpoint to verify the match is enabled
   * for this campaign. Stores the result in the detectedSystem setting.
   * @private
   */
  async _detectSystem() {
    this._foundrySystemId = game.system?.id || null;

    if (!this._foundrySystemId) {
      console.log('Chronicle: No Foundry game system detected');
      return;
    }

    const chronicleId = SYSTEM_MAP[this._foundrySystemId];
    if (!chronicleId) {
      console.log(`Chronicle: Foundry system "${this._foundrySystemId}" has no Chronicle mapping`);
      await setSetting('detectedSystem', '');
      return;
    }

    try {
      // Query Chronicle for available systems and check if the mapped one is enabled.
      const result = await this.api.get('/systems');
      const systems = result.data || [];
      const match = systems.find((s) => s.id === chronicleId && s.enabled);

      if (match) {
        this._matchedSystem = chronicleId;
        await setSetting('detectedSystem', chronicleId);
        this.logActivity('connect', `Game system matched: ${match.name}`);
        console.log(`Chronicle: System matched — Foundry "${this._foundrySystemId}" → Chronicle "${chronicleId}"`);
      } else {
        await setSetting('detectedSystem', '');
        console.log(`Chronicle: System "${chronicleId}" not enabled for this campaign`);
      }
    } catch (err) {
      console.warn('Chronicle: Failed to detect system match', err);
      // Fall back to local setting if API call fails.
      const cached = getSetting('detectedSystem');
      if (cached) {
        this._matchedSystem = cached;
      }
    }
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

      this.logActivity('connect', `Initial sync complete (${result.mappings?.length || 0} mappings)`);
      ui.notifications.info('Chronicle: Initial sync complete');
    } catch (err) {
      console.error('Chronicle: Initial sync failed', err);
      this.logActivity('error', `Initial sync failed: ${err.message || 'Unknown error'}`);
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
   * Add an entry to the activity log.
   * @param {string} type - Log type: 'pull', 'push', 'update', 'link', 'unlink', 'connect', 'error'.
   * @param {string} message - Human-readable description.
   */
  logActivity(type, message) {
    this._activityLog.unshift({
      time: Date.now(),
      type,
      message,
      timeFormatted: new Date().toLocaleTimeString(),
    });
    if (this._activityLog.length > this._maxLogEntries) {
      this._activityLog.length = this._maxLogEntries;
    }
  }

  /**
   * Get the activity log entries.
   * @returns {Array<{time: number, type: string, message: string, timeFormatted: string}>}
   */
  getActivityLog() {
    return this._activityLog;
  }

  /**
   * Clear the activity log.
   */
  clearActivityLog() {
    this._activityLog = [];
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
