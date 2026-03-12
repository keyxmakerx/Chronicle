/**
 * Chronicle Sync - Sync Dashboard
 *
 * Tabbed Application window for managing all Chronicle sync operations.
 * Provides visibility into sync state, per-entity/type controls, map linking,
 * calendar sync, and connection status with activity log.
 *
 * Accessed via the sidebar control button (GM only).
 */

import { getSetting, setSetting } from './settings.mjs';

const FLAG_SCOPE = 'chronicle-sync';

/**
 * SyncDashboard is the main sync management UI.
 * Extends Foundry's Application class with tab support.
 */
export class SyncDashboard extends Application {
  /** @type {SyncDashboard|null} Singleton instance. */
  static _instance = null;

  /** Get or create the singleton dashboard instance. */
  static get instance() {
    if (!SyncDashboard._instance) {
      SyncDashboard._instance = new SyncDashboard();
    }
    return SyncDashboard._instance;
  }

  static get defaultOptions() {
    return mergeObject(super.defaultOptions, {
      id: 'chronicle-sync-dashboard',
      title: 'Chronicle Sync',
      template: 'modules/chronicle-sync/templates/sync-dashboard.hbs',
      width: 620,
      height: 700,
      resizable: true,
      classes: ['chronicle-dashboard'],
      tabs: [{
        navSelector: '.dashboard-tabs',
        contentSelector: '.dashboard-content',
        initial: 'entities',
      }],
    });
  }

  constructor(options = {}) {
    super(options);

    /** @type {import('./sync-manager.mjs').SyncManager|null} */
    this._syncManager = null;

    /** @type {boolean} Whether data is currently loading. */
    this._loading = false;

    /** @type {object} Cached data for each tab. */
    this._cache = { entityTypes: null, entities: null, maps: null, calendar: null };

    /** @type {Set<number>} Collapsed entity type groups. */
    this._collapsedTypes = new Set();

    /** @type {string} Current search filter text. */
    this._searchFilter = '';
  }

  /**
   * Bind the sync manager to the dashboard.
   * @param {import('./sync-manager.mjs').SyncManager} syncManager
   */
  bind(syncManager) {
    this._syncManager = syncManager;
  }

  /** @returns {import('./api-client.mjs').ChronicleAPI|null} */
  get api() {
    return this._syncManager?.api ?? null;
  }

  // ---------------------------------------------------------------------------
  // Data
  // ---------------------------------------------------------------------------

  /** @override */
  async getData() {
    if (!this._syncManager || !this.api) {
      return { configured: false };
    }

    const exclusions = this._getExclusions();

    // Build entity tab data.
    let entityGroups = [];
    let foundryOnlyJournals = [];
    try {
      entityGroups = await this._buildEntityGroups(exclusions);
      foundryOnlyJournals = this._getFoundryOnlyJournals();
    } catch (err) {
      console.error('Chronicle Dashboard: Failed to load entities', err);
    }

    // Build map tab data.
    let mapData = { chronicleMaps: [], unlinkedScenes: [] };
    try {
      mapData = await this._buildMapData();
    } catch (err) {
      console.error('Chronicle Dashboard: Failed to load maps', err);
    }

    // Build calendar tab data.
    let calendarData = { available: false };
    try {
      calendarData = await this._buildCalendarData();
    } catch (err) {
      console.error('Chronicle Dashboard: Failed to load calendar', err);
    }

    // Build status tab data.
    const statusData = this._buildStatusData();

    return {
      configured: true,
      loading: this._loading,
      searchFilter: this._searchFilter,

      // Entities tab.
      entityGroups,
      foundryOnlyJournals,
      hasChronicleOnly: entityGroups.some(g => g.entities.some(e => e.status === 'chronicle-only')),
      hasFoundryOnly: foundryOnlyJournals.length > 0,

      // Maps tab.
      ...mapData,

      // Calendar tab.
      calendar: calendarData,

      // Status tab.
      ...statusData,
    };
  }

  // ---------------------------------------------------------------------------
  // Entity data
  // ---------------------------------------------------------------------------

  /**
   * Build entity groups by type with sync status.
   * @param {object} exclusions
   * @returns {Promise<Array>}
   * @private
   */
  async _buildEntityGroups(exclusions) {
    // Fetch entity types from Chronicle.
    if (!this._cache.entityTypes) {
      this._cache.entityTypes = await this.api.get('/entity-types');
    }
    const types = this._cache.entityTypes || [];

    // Fetch all entities (paginated, up to 500 for now).
    if (!this._cache.entities) {
      const allEntities = [];
      let page = 1;
      let hasMore = true;
      while (hasMore && page <= 5) {
        const result = await this.api.get(`/entities?per_page=100&page=${page}`);
        const entities = result?.entities || result || [];
        if (Array.isArray(entities) && entities.length > 0) {
          allEntities.push(...entities);
          hasMore = entities.length === 100;
          page++;
        } else {
          hasMore = false;
        }
      }
      this._cache.entities = allEntities;
    }
    const chronicleEntities = this._cache.entities || [];

    // Index Foundry journals by entityId flag.
    const journalsByEntityId = new Map();
    for (const j of game.journal.contents) {
      const eid = j.getFlag(FLAG_SCOPE, 'entityId');
      if (eid) journalsByEntityId.set(eid, j);
    }

    // Build groups.
    const groups = [];
    for (const type of types) {
      const typeEntities = chronicleEntities.filter(e => e.entity_type_id === type.id);
      const isTypeExcluded = exclusions.excludedTypes.includes(type.id);

      const entities = typeEntities.map(entity => {
        const journal = journalsByEntityId.get(entity.id);
        const isExcluded = exclusions.excludedEntities.includes(entity.id) || isTypeExcluded;

        let status = 'chronicle-only';
        if (journal) {
          const lastSync = journal.getFlag(FLAG_SCOPE, 'lastSync');
          if (lastSync) {
            const syncTime = new Date(lastSync).getTime();
            const updateTime = new Date(entity.updated_at).getTime();
            if (Math.abs(updateTime - syncTime) < 5000) {
              status = 'synced';
            } else if (updateTime > syncTime) {
              status = 'modified';
            } else {
              status = 'synced';
            }
          } else {
            status = 'synced';
          }
        }

        return {
          id: entity.id,
          name: entity.name,
          typeName: type.name,
          status,
          isPrivate: entity.is_private ?? false,
          isExcluded,
          journalId: journal?.id ?? null,
          updatedAt: entity.updated_at,
        };
      });

      // Apply search filter.
      const filtered = this._searchFilter
        ? entities.filter(e => e.name.toLowerCase().includes(this._searchFilter.toLowerCase()))
        : entities;

      const syncedCount = filtered.filter(e => e.status === 'synced' || e.status === 'modified').length;
      const chronicleOnlyCount = filtered.filter(e => e.status === 'chronicle-only').length;

      groups.push({
        typeId: type.id,
        typeName: type.name,
        typeNamePlural: type.name_plural || type.name + 's',
        typeIcon: type.icon || 'fa-solid fa-file',
        typeColor: type.color || '#888',
        isCollapsed: this._collapsedTypes.has(type.id),
        isTypeExcluded,
        entities: filtered,
        syncedCount,
        chronicleOnlyCount,
        totalCount: filtered.length,
      });
    }

    // Only show groups that have entities (or chronicle-only).
    return groups.filter(g => g.totalCount > 0);
  }

  /**
   * Get Foundry journals that aren't linked to any Chronicle entity.
   * @returns {Array}
   * @private
   */
  _getFoundryOnlyJournals() {
    const result = [];
    const filter = this._searchFilter?.toLowerCase() || '';

    for (const j of game.journal.contents) {
      if (j.getFlag(FLAG_SCOPE, 'entityId')) continue;
      // Skip SimpleCalendar note journals.
      if (j.flags?.['foundryvtt-simple-calendar'] || j.flags?.['simple-calendar']) continue;

      if (filter && !j.name.toLowerCase().includes(filter)) continue;

      result.push({
        id: j.id,
        name: j.name,
        status: 'foundry-only',
        pageCount: j.pages?.size ?? 0,
      });
    }

    return result;
  }

  // ---------------------------------------------------------------------------
  // Map data
  // ---------------------------------------------------------------------------

  /**
   * Build map tab data: Chronicle maps matched with Foundry scenes.
   * @returns {Promise<object>}
   * @private
   */
  async _buildMapData() {
    // Fetch Chronicle maps.
    if (!this._cache.maps) {
      this._cache.maps = await this.api.get('/maps').catch(() => []);
    }
    const chronicles = this._cache.maps || [];

    // Index Foundry scenes by linked mapId.
    const scenesByMapId = new Map();
    const unlinkedScenes = [];

    for (const scene of game.scenes.contents) {
      const mapId = scene.getFlag(FLAG_SCOPE, 'mapId');
      if (mapId) {
        scenesByMapId.set(mapId, scene);
      } else {
        unlinkedScenes.push({ id: scene.id, name: scene.name });
      }
    }

    // Build chronicle map entries.
    const chronicleMaps = chronicles.map(m => {
      const scene = scenesByMapId.get(m.id);
      return {
        id: m.id,
        name: m.name,
        linked: !!scene,
        sceneId: scene?.id ?? null,
        sceneName: scene?.name ?? null,
      };
    });

    return {
      chronicleMaps,
      unlinkedScenes,
      availableScenes: unlinkedScenes,
      availableMaps: chronicles.filter(m => !scenesByMapId.has(m.id)),
    };
  }

  // ---------------------------------------------------------------------------
  // Calendar data
  // ---------------------------------------------------------------------------

  /**
   * Build calendar tab data.
   * @returns {Promise<object>}
   * @private
   */
  async _buildCalendarData() {
    const calendarEnabled = getSetting('syncCalendar');
    const calModule = this._detectCalendarModule();

    if (!calendarEnabled || !calModule) {
      return { available: false, enabled: calendarEnabled, module: calModule };
    }

    let chronicle = null;
    try {
      chronicle = await this.api.get('/calendar');
    } catch {
      // No calendar configured.
    }

    if (!chronicle) {
      return { available: false, enabled: calendarEnabled, module: calModule, noCampaignCalendar: true };
    }

    // Get local Foundry calendar date.
    const localDate = this._getLocalCalendarDate(calModule);

    const inSync = chronicle.current_year === localDate?.year
      && chronicle.current_month === localDate?.month
      && chronicle.current_day === localDate?.day;

    return {
      available: true,
      enabled: calendarEnabled,
      module: calModule,
      chronicleDate: {
        year: chronicle.current_year,
        month: chronicle.current_month,
        day: chronicle.current_day,
        hour: chronicle.current_hour ?? 0,
        minute: chronicle.current_minute ?? 0,
        calendarName: chronicle.name || 'Campaign Calendar',
      },
      localDate,
      inSync,
    };
  }

  /**
   * Detect which Foundry calendar module is active.
   * @returns {string|null}
   * @private
   */
  _detectCalendarModule() {
    if (game.modules.get('calendaria')?.active) return 'Calendaria';
    if (game.modules.get('foundryvtt-simple-calendar')?.active) return 'Simple Calendar';
    return null;
  }

  /**
   * Get the current date from the active Foundry calendar module.
   * @param {string} calModule
   * @returns {object|null}
   * @private
   */
  _getLocalCalendarDate(calModule) {
    try {
      if (calModule === 'Calendaria' && game.Calendaria?.getDate) {
        const d = game.Calendaria.getDate();
        return { year: d.year, month: d.month, day: d.day, hour: d.hour ?? 0, minute: d.minute ?? 0 };
      }
      if (calModule === 'Simple Calendar' && typeof SimpleCalendar !== 'undefined') {
        const ts = SimpleCalendar.api?.currentDateTime?.();
        if (ts) {
          return {
            year: ts.year,
            month: (ts.month ?? 0) + 1, // SC is 0-indexed.
            day: (ts.day ?? 0) + 1,
            hour: ts.hour ?? 0,
            minute: ts.minute ?? 0,
          };
        }
      }
    } catch {
      // Calendar module API not available.
    }
    return null;
  }

  // ---------------------------------------------------------------------------
  // Status data
  // ---------------------------------------------------------------------------

  /**
   * Build status tab data.
   * @returns {object}
   * @private
   */
  _buildStatusData() {
    const state = this.api?.state ?? 'disconnected';
    const activityLog = this._syncManager?._activityLog ?? [];

    // Compute stats from Foundry documents.
    const syncedEntities = game.journal.contents.filter(
      j => j.getFlag(FLAG_SCOPE, 'entityId')
    ).length;

    const linkedScenes = game.scenes.contents.filter(
      s => s.getFlag(FLAG_SCOPE, 'mapId')
    ).length;

    return {
      connectionState: state,
      connectionLabel: this._connectionLabel(state),
      lastSyncTime: getSetting('lastSyncTime') || 'Never',
      syncedEntities,
      linkedScenes,
      activityLog: activityLog.slice(-50).reverse(),
    };
  }

  /**
   * @param {string} state
   * @returns {string}
   * @private
   */
  _connectionLabel(state) {
    switch (state) {
      case 'connected': return 'Connected';
      case 'connecting': return 'Connecting...';
      case 'reconnecting': return 'Reconnecting...';
      default: return 'Disconnected';
    }
  }

  // ---------------------------------------------------------------------------
  // Exclusions
  // ---------------------------------------------------------------------------

  /**
   * Get current sync exclusions.
   * @returns {{ excludedTypes: number[], excludedEntities: string[] }}
   * @private
   */
  _getExclusions() {
    try {
      const raw = getSetting('syncExclusions');
      if (raw) {
        const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw;
        return {
          excludedTypes: parsed.excludedTypes || [],
          excludedEntities: parsed.excludedEntities || [],
        };
      }
    } catch {
      // Ignore parse errors.
    }
    return { excludedTypes: [], excludedEntities: [] };
  }

  /**
   * Save sync exclusions.
   * @param {object} exclusions
   * @private
   */
  async _saveExclusions(exclusions) {
    await setSetting('syncExclusions', JSON.stringify(exclusions));
  }

  // ---------------------------------------------------------------------------
  // Event Handlers
  // ---------------------------------------------------------------------------

  /** @override */
  activateListeners(html) {
    super.activateListeners(html);

    // Search input.
    html.find('.dashboard-search').on('input', (e) => {
      this._searchFilter = e.target.value;
      this.render(false);
    });

    // Refresh button.
    html.find('[data-action="refresh"]').on('click', () => {
      this._cache = { entityTypes: null, entities: null, maps: null, calendar: null };
      this.render(false);
    });

    // Entity type header toggle (collapse/expand).
    html.find('.entity-type-header').on('click', (e) => {
      const typeId = Number(e.currentTarget.dataset.typeId);
      if (this._collapsedTypes.has(typeId)) {
        this._collapsedTypes.delete(typeId);
      } else {
        this._collapsedTypes.add(typeId);
      }
      this.render(false);
    });

    // Type sync toggle.
    html.find('.type-sync-toggle').on('click', (e) => {
      e.stopPropagation();
      const typeId = Number(e.currentTarget.dataset.typeId);
      this._onToggleType(typeId);
    });

    // Entity sync toggle.
    html.find('.entity-sync-toggle').on('change', (e) => {
      const entityId = e.currentTarget.dataset.entityId;
      this._onToggleEntity(entityId, e.currentTarget.checked);
    });

    // Pull entity from Chronicle.
    html.find('[data-action="pull-entity"]').on('click', (e) => {
      const entityId = e.currentTarget.dataset.entityId;
      this._onPullEntity(entityId);
    });

    // Push journal to Chronicle.
    html.find('[data-action="push-journal"]').on('click', (e) => {
      const journalId = e.currentTarget.dataset.journalId;
      this._onPushJournal(journalId);
    });

    // Pull all Chronicle-only entities.
    html.find('[data-action="pull-all"]').on('click', () => this._onPullAll());

    // Push all Foundry-only journals.
    html.find('[data-action="push-all"]').on('click', () => this._onPushAll());

    // Toggle entity visibility.
    html.find('[data-action="toggle-visibility"]').on('click', (e) => {
      const entityId = e.currentTarget.dataset.entityId;
      const isPrivate = e.currentTarget.dataset.isPrivate === 'true';
      this._onToggleVisibility(entityId, isPrivate);
    });

    // Map link/unlink.
    html.find('[data-action="link-scene"]').on('change', (e) => {
      const mapId = e.currentTarget.dataset.mapId;
      const sceneId = e.currentTarget.value;
      if (sceneId) this._onLinkScene(mapId, sceneId);
    });

    html.find('[data-action="link-map"]').on('change', (e) => {
      const sceneId = e.currentTarget.dataset.sceneId;
      const mapId = e.currentTarget.value;
      if (mapId) this._onLinkScene(mapId, sceneId);
    });

    html.find('[data-action="unlink-scene"]').on('click', (e) => {
      const sceneId = e.currentTarget.dataset.sceneId;
      this._onUnlinkScene(sceneId);
    });

    // Calendar actions.
    html.find('[data-action="pull-date"]').on('click', () => this._onPullDate());
    html.find('[data-action="push-date"]').on('click', () => this._onPushDate());

    // Status actions.
    html.find('[data-action="reconnect"]').on('click', () => {
      this.api?.connect();
      setTimeout(() => this.render(false), 1000);
    });

    html.find('[data-action="clear-log"]').on('click', () => {
      if (this._syncManager?._activityLog) {
        this._syncManager._activityLog = [];
      }
      this.render(false);
    });

    html.find('[data-action="open-settings"]').on('click', () => {
      game.settings.sheet.render(true);
    });
  }

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  /**
   * Toggle an entire entity type's sync exclusion.
   * @param {number} typeId
   * @private
   */
  async _onToggleType(typeId) {
    const exclusions = this._getExclusions();
    const idx = exclusions.excludedTypes.indexOf(typeId);
    if (idx >= 0) {
      exclusions.excludedTypes.splice(idx, 1);
    } else {
      exclusions.excludedTypes.push(typeId);
    }
    await this._saveExclusions(exclusions);
    this.render(false);
  }

  /**
   * Toggle an individual entity's sync exclusion.
   * @param {string} entityId
   * @param {boolean} enabled - True if sync should be enabled (not excluded).
   * @private
   */
  async _onToggleEntity(entityId, enabled) {
    const exclusions = this._getExclusions();
    const idx = exclusions.excludedEntities.indexOf(entityId);
    if (enabled && idx >= 0) {
      exclusions.excludedEntities.splice(idx, 1);
    } else if (!enabled && idx < 0) {
      exclusions.excludedEntities.push(entityId);
    }
    await this._saveExclusions(exclusions);
  }

  /**
   * Pull a single entity from Chronicle into Foundry.
   * @param {string} entityId
   * @private
   */
  async _onPullEntity(entityId) {
    try {
      const entity = await this.api.get(`/entities/${entityId}`);
      if (!entity) return;

      // Find the JournalSync module.
      const journalSync = this._syncManager?._modules?.find(m => m.constructor?.name === 'JournalSync');
      if (journalSync) {
        await journalSync._createJournalFromEntity(entity);
        this._logActivity('pull', `Pulled "${entity.name}" from Chronicle`);
      }

      this._cache.entities = null; // Invalidate cache.
      this.render(false);
    } catch (err) {
      console.error('Chronicle Dashboard: Pull failed', err);
      ui.notifications.error(`Failed to pull entity: ${err.message}`);
    }
  }

  /**
   * Push a Foundry journal to Chronicle as a new entity.
   * @param {string} journalId
   * @private
   */
  async _onPushJournal(journalId) {
    const journal = game.journal.get(journalId);
    if (!journal) return;

    try {
      const textPage = journal.pages.find(p => p.type === 'text');
      const entity = await this.api.post('/entities', {
        name: journal.name,
        entity_type_id: 0,
        is_private: (journal.ownership?.default ?? 0) < CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER,
        entry: textPage?.text?.content || '',
      });

      if (entity) {
        await journal.setFlag(FLAG_SCOPE, 'entityId', entity.id);
        await journal.setFlag(FLAG_SCOPE, 'lastSync', new Date().toISOString());

        await this.api.post('/sync/mappings', {
          chronicle_type: 'entity',
          chronicle_id: entity.id,
          external_system: 'foundry',
          external_id: journal.id,
          sync_direction: 'both',
        });

        this._logActivity('push', `Pushed "${journal.name}" to Chronicle`);
      }

      this._cache.entities = null;
      this.render(false);
    } catch (err) {
      console.error('Chronicle Dashboard: Push failed', err);
      ui.notifications.error(`Failed to push journal: ${err.message}`);
    }
  }

  /**
   * Pull all Chronicle-only entities.
   * @private
   */
  async _onPullAll() {
    const confirmed = await Dialog.confirm({
      title: 'Pull All from Chronicle',
      content: '<p>Pull all Chronicle entities that don\'t have a Foundry journal yet?</p>',
    });
    if (!confirmed) return;

    const data = await this.getData();
    let count = 0;
    for (const group of data.entityGroups) {
      for (const entity of group.entities) {
        if (entity.status === 'chronicle-only') {
          await this._onPullEntity(entity.id);
          count++;
        }
      }
    }
    ui.notifications.info(`Chronicle: Pulled ${count} entities.`);
  }

  /**
   * Push all Foundry-only journals.
   * @private
   */
  async _onPushAll() {
    const confirmed = await Dialog.confirm({
      title: 'Push All to Chronicle',
      content: '<p>Push all Foundry journals that aren\'t linked to Chronicle yet?</p>',
    });
    if (!confirmed) return;

    const journals = this._getFoundryOnlyJournals();
    let count = 0;
    for (const j of journals) {
      await this._onPushJournal(j.id);
      count++;
    }
    ui.notifications.info(`Chronicle: Pushed ${count} journals.`);
  }

  /**
   * Toggle entity visibility (public/private) on Chronicle.
   * @param {string} entityId
   * @param {boolean} currentlyPrivate
   * @private
   */
  async _onToggleVisibility(entityId, currentlyPrivate) {
    try {
      await this.api.put(`/entities/${entityId}`, {
        is_private: !currentlyPrivate,
      });

      // Update the local journal's ownership.
      const journal = game.journal.find(j => j.getFlag(FLAG_SCOPE, 'entityId') === entityId);
      if (journal) {
        const newOwnership = currentlyPrivate
          ? { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER }
          : { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.NONE };
        await journal.update({ ownership: newOwnership });
      }

      this._cache.entities = null;
      this.render(false);
    } catch (err) {
      console.error('Chronicle Dashboard: Visibility toggle failed', err);
    }
  }

  /**
   * Link a Foundry scene to a Chronicle map.
   * @param {string} mapId
   * @param {string} sceneId
   * @private
   */
  async _onLinkScene(mapId, sceneId) {
    const scene = game.scenes.get(sceneId);
    if (!scene) return;

    try {
      await scene.setFlag(FLAG_SCOPE, 'mapId', mapId);

      await this.api.post('/sync/mappings', {
        chronicle_type: 'map',
        chronicle_id: mapId,
        external_system: 'foundry',
        external_id: sceneId,
        sync_direction: 'both',
      });

      this._logActivity('link', `Linked scene "${scene.name}" to map`);
      this._cache.maps = null;
      this.render(false);

      ui.notifications.info(`Chronicle: Scene "${scene.name}" linked to map.`);
    } catch (err) {
      console.error('Chronicle Dashboard: Link failed', err);
      ui.notifications.error(`Failed to link scene: ${err.message}`);
    }
  }

  /**
   * Unlink a Foundry scene from its Chronicle map.
   * @param {string} sceneId
   * @private
   */
  async _onUnlinkScene(sceneId) {
    const scene = game.scenes.get(sceneId);
    if (!scene) return;

    await scene.unsetFlag(FLAG_SCOPE, 'mapId');
    this._logActivity('unlink', `Unlinked scene "${scene.name}"`);
    this._cache.maps = null;
    this.render(false);

    ui.notifications.info(`Chronicle: Scene "${scene.name}" unlinked.`);
  }

  /**
   * Pull calendar date from Chronicle to Foundry.
   * @private
   */
  async _onPullDate() {
    const calSync = this._syncManager?._modules?.find(m => m.constructor?.name === 'CalendarSync');
    if (calSync && typeof calSync.onInitialSync === 'function') {
      await calSync.onInitialSync();
      this._logActivity('pull', 'Pulled calendar date from Chronicle');
      this.render(false);
    }
  }

  /**
   * Push calendar date from Foundry to Chronicle.
   * @private
   */
  async _onPushDate() {
    const calModule = this._detectCalendarModule();
    const localDate = this._getLocalCalendarDate(calModule);
    if (!localDate) return;

    try {
      await this.api.put('/calendar/date', {
        year: localDate.year,
        month: localDate.month,
        day: localDate.day,
        hour: localDate.hour || 0,
        minute: localDate.minute || 0,
      });
      this._logActivity('push', 'Pushed calendar date to Chronicle');
      this.render(false);
    } catch (err) {
      console.error('Chronicle Dashboard: Push date failed', err);
    }
  }

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  /**
   * Add an entry to the activity log.
   * @param {string} type
   * @param {string} message
   */
  _logActivity(type, message) {
    if (this._syncManager?._activityLog) {
      this._syncManager._activityLog.push({
        time: Date.now(),
        type,
        message,
      });
    }
  }

  /** Force refresh all data and re-render. */
  refresh() {
    this._cache = { entityTypes: null, entities: null, maps: null, calendar: null };
    this.render(false);
  }
}
