/**
 * Chronicle Sync - Sync Manager
 *
 * Orchestrates sync state across all sync modules (journals, maps, calendar).
 * Manages the API client lifecycle, initial sync on connect, and coordinates
 * conflict resolution.
 */

import { ChronicleAPI } from './api-client.mjs';
import { getSetting, setSetting, isConfigured, getSyncDirections, getExcludedTags } from './settings.mjs';

/**
 * Hardcoded fallback map for known Foundry→Chronicle system mappings.
 * Used only when the API doesn't provide foundry_system_id matches.
 * The preferred flow is API-driven: /systems returns foundry_system_id
 * on each system, and _detectSystem() matches against game.system.id.
 */
const SYSTEM_MAP_FALLBACK = {
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
   * Queries the /systems API endpoint which returns foundry_system_id on each
   * system. Matches are API-driven so custom-uploaded systems work automatically.
   * Falls back to SYSTEM_MAP_FALLBACK only when the API doesn't return a match.
   * @private
   */
  async _detectSystem() {
    this._foundrySystemId = game.system?.id || null;

    if (!this._foundrySystemId) {
      console.log('Chronicle: No Foundry game system detected');
      return;
    }

    try {
      // Query Chronicle for available systems. Each system may include
      // foundry_system_id for automatic matching.
      const result = await this.api.get('/systems');
      const systems = result.data || [];

      // Primary: match by foundry_system_id returned from the API.
      let match = systems.find(
        (s) => s.foundry_system_id === this._foundrySystemId && s.enabled
      );

      // Secondary: match by hardcoded fallback map (legacy support).
      if (!match) {
        const fallbackId = SYSTEM_MAP_FALLBACK[this._foundrySystemId];
        if (fallbackId) {
          match = systems.find((s) => s.id === fallbackId && s.enabled);
        }
      }

      if (match) {
        this._matchedSystem = match.id;
        await setSetting('detectedSystem', match.id);
        this.logActivity('connect', `Game system matched: ${match.name}`);
        console.log(`Chronicle: System matched — Foundry "${this._foundrySystemId}" → Chronicle "${match.id}"`);
      } else {
        await setSetting('detectedSystem', '');
        console.log(`Chronicle: No Chronicle system matches Foundry system "${this._foundrySystemId}"`);
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
   * Get the sync direction for a given sync type.
   * Returns "both", "pull", "push", or "off".
   * @param {string} syncType - One of: journals, maps, calendar, characters, shops.
   * @returns {string}
   */
  getSyncDirection(syncType) {
    const directions = getSyncDirections();
    return directions[syncType] || 'both';
  }

  /**
   * Check if a sync type allows pulling from Chronicle to Foundry.
   * @param {string} syncType
   * @returns {boolean}
   */
  canPull(syncType) {
    const dir = this.getSyncDirection(syncType);
    return dir === 'both' || dir === 'pull';
  }

  /**
   * Check if a sync type allows pushing from Foundry to Chronicle.
   * @param {string} syncType
   * @returns {boolean}
   */
  canPush(syncType) {
    const dir = this.getSyncDirection(syncType);
    return dir === 'both' || dir === 'push';
  }

  /**
   * Check if auto-sync on change is enabled.
   * @returns {boolean}
   */
  isAutoSync() {
    return getSetting('autoSync');
  }

  /**
   * Check if an entity should be excluded from sync based on tag or name rules.
   * @param {object} entity - Entity with name and tags array.
   * @returns {boolean} True if the entity should be excluded.
   */
  isExcludedByRules(entity) {
    // Tag-based exclusion.
    const excludedTags = getExcludedTags();
    if (excludedTags.length > 0 && entity.tags) {
      const entityTags = Array.isArray(entity.tags)
        ? entity.tags.map(t => (typeof t === 'string' ? t : t.name || '').toLowerCase())
        : [];
      const excluded = excludedTags.map(t => t.toLowerCase());
      if (entityTags.some(t => excluded.includes(t))) return true;
    }

    // Name pattern exclusion.
    const namePattern = getSetting('excludedNamePattern');
    if (namePattern && entity.name) {
      if (entity.name.toLowerCase().includes(namePattern.toLowerCase())) return true;
    }

    return false;
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
