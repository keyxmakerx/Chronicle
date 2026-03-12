/**
 * Chronicle Sync - Sync Dashboard
 *
 * Tabbed ApplicationV2 window for managing all Chronicle sync operations.
 * Provides visibility into sync state, per-entity/type controls, map linking,
 * calendar sync, and connection status with activity log.
 *
 * Accessed via the sidebar status indicator or scene controls button (GM only).
 */

import { getSetting, setSetting } from './settings.mjs';

const FLAG_SCOPE = 'chronicle-sync';
const { ApplicationV2, HandlebarsApplicationMixin } = foundry.applications.api;

/**
 * SyncDashboard is the main sync management UI.
 * Extends Foundry's ApplicationV2 with HandlebarsApplicationMixin for
 * template rendering and tab support.
 */
export class SyncDashboard extends HandlebarsApplicationMixin(ApplicationV2) {
  /** @type {SyncDashboard|null} Singleton instance. */
  static _instance = null;

  /** Get or create the singleton dashboard instance. */
  static get instance() {
    if (!SyncDashboard._instance) {
      SyncDashboard._instance = new SyncDashboard();
    }
    return SyncDashboard._instance;
  }

  static DEFAULT_OPTIONS = {
    id: 'chronicle-sync-dashboard',
    classes: ['chronicle-dashboard'],
    window: {
      title: 'Chronicle Sync',
      icon: 'fa-solid fa-rotate',
      resizable: true,
    },
    position: {
      width: 620,
      height: 700,
    },
    actions: {
      refresh: SyncDashboard.#onRefresh,
      'pull-entity': SyncDashboard.#onPullEntityAction,
      'push-journal': SyncDashboard.#onPushJournalAction,
      'pull-all': SyncDashboard.#onPullAllAction,
      'push-all': SyncDashboard.#onPushAllAction,
      'toggle-visibility': SyncDashboard.#onToggleVisibilityAction,
      'unlink-scene': SyncDashboard.#onUnlinkSceneAction,
      'pull-date': SyncDashboard.#onPullDateAction,
      'push-date': SyncDashboard.#onPushDateAction,
      reconnect: SyncDashboard.#onReconnectAction,
      'clear-log': SyncDashboard.#onClearLogAction,
      'open-settings': SyncDashboard.#onOpenSettingsAction,
      'open-shop': SyncDashboard.#onOpenShopAction,
      'push-actor': SyncDashboard.#onPushActorAction,
    },
  };

  static PARTS = {
    dashboard: {
      template: 'modules/chronicle-sync/templates/sync-dashboard.hbs',
    },
  };

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

    /** @type {string} Currently active tab. */
    this._activeTab = 'entities';
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
  async _prepareContext(options = {}) {
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

    // Build shops tab data.
    let shopData = { shops: [] };
    try {
      shopData = await this._buildShopData();
    } catch (err) {
      console.error('Chronicle Dashboard: Failed to load shops', err);
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

    // Build characters tab data.
    const characterData = this._buildCharacterData();

    return {
      configured: true,
      loading: this._loading,
      searchFilter: this._searchFilter,
      activeTab: this._activeTab,

      // Entities tab.
      entityGroups,
      foundryOnlyJournals,
      hasChronicleOnly: entityGroups.some(g => g.entities.some(e => e.status === 'chronicle-only')),
      hasFoundryOnly: foundryOnlyJournals.length > 0,

      // Shops tab.
      ...shopData,

      // Maps tab.
      ...mapData,

      // Characters tab.
      ...characterData,

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
  // Shop data
  // ---------------------------------------------------------------------------

  /**
   * Build shops tab data: all shop-type entities from Chronicle with
   * inventory counts and sync status.
   * @returns {Promise<object>}
   * @private
   */
  async _buildShopData() {
    // Ensure entity types are cached.
    if (!this._cache.entityTypes) {
      this._cache.entityTypes = await this.api.get('/entity-types');
    }
    const types = this._cache.entityTypes || [];

    // Find the shop entity type by slug or name.
    const shopType = types.find(t =>
      t.slug === 'shop' || t.name?.toLowerCase() === 'shop'
    );

    if (!shopType) {
      return { shops: [], shopTypeExists: false };
    }

    // Ensure entities are cached.
    if (!this._cache.entities) {
      // Trigger entity cache population via _buildEntityGroups path.
      await this._buildEntityGroups(this._getExclusions());
    }
    const allEntities = this._cache.entities || [];

    // Filter to shop entities.
    const shopEntities = allEntities.filter(e => e.entity_type_id === shopType.id);

    // Index Foundry journals by entityId flag.
    const journalsByEntityId = new Map();
    for (const j of game.journal.contents) {
      const eid = j.getFlag(FLAG_SCOPE, 'entityId');
      if (eid) journalsByEntityId.set(eid, j);
    }

    const shops = shopEntities.map(entity => {
      const journal = journalsByEntityId.get(entity.id);
      const fields = entity.fields_data || {};

      return {
        id: entity.id,
        name: entity.name,
        shopType: fields.shop_type || '',
        shopKeeper: fields.shop_keeper || '',
        synced: !!journal,
        journalId: journal?.id ?? null,
        isPrivate: entity.is_private ?? false,
      };
    });

    return {
      shops,
      shopTypeExists: true,
      shopTypeIcon: shopType.icon || 'fa-store',
      shopTypeColor: shopType.color || '#f97316',
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
  // Character data
  // ---------------------------------------------------------------------------

  /**
   * Build characters tab data from ActorSync module.
   * @returns {object}
   * @private
   */
  _buildCharacterData() {
    const actorSync = this._syncManager?._modules?.find(
      (m) => m.constructor?.name === 'ActorSync'
    );
    const characters = actorSync?.getSyncedActors?.() ?? [];

    return { characters };
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
    const activityLog = this._syncManager?.getActivityLog() ?? [];

    // Compute stats from Foundry documents.
    const syncedEntities = game.journal.contents.filter(
      j => j.getFlag(FLAG_SCOPE, 'entityId')
    ).length;

    const linkedScenes = game.scenes.contents.filter(
      s => s.getFlag(FLAG_SCOPE, 'mapId')
    ).length;

    // System detection info.
    const foundrySystem = this._syncManager?.getFoundrySystemId() || null;
    const matchedSystem = this._syncManager?.getMatchedSystem() || null;
    const syncCharacters = getSetting('syncCharacters');

    // Health metrics (F-QoL).
    const health = this.api?.health ?? {};
    const errorLog = this.api?.getErrorLog() ?? [];
    const retryQueueSize = this.api?.getRetryQueueSize() ?? 0;
    const uptimePercent = this.api?.getUptimePercent() ?? 0;

    // Field mapping info for debug view.
    const fieldMappingInfo = this._buildFieldMappingInfo(matchedSystem);

    return {
      connectionState: state,
      connectionLabel: this._connectionLabel(state),
      lastSyncTime: getSetting('lastSyncTime') || 'Never',
      syncedEntities,
      linkedScenes,
      foundrySystem,
      matchedSystem,
      systemMatched: !!matchedSystem,
      syncCharacters,
      characterSyncAvailable: !!matchedSystem,
      activityLog: activityLog.slice(0, 50),

      // Diagnostics (F-QoL).
      healthMetrics: {
        restSuccessCount: health.restSuccessCount ?? 0,
        restErrorCount: health.restErrorCount ?? 0,
        reconnectAttempts: health.reconnectAttempts ?? 0,
        uptimePercent,
        retryQueueSize,
        lastRestSuccess: health.lastRestSuccess
          ? new Date(health.lastRestSuccess).toLocaleTimeString()
          : 'Never',
        lastRestError: health.lastRestError
          ? new Date(health.lastRestError).toLocaleTimeString()
          : 'None',
      },
      errorLog: errorLog.slice(0, 20),
      hasErrors: errorLog.length > 0,
      fieldMappingInfo,
    };
  }

  /**
   * Build field mapping debug info for the current system adapter.
   * Shows which fields are mapped and their Foundry paths.
   * @param {string|null} matchedSystem
   * @returns {object|null}
   * @private
   */
  _buildFieldMappingInfo(matchedSystem) {
    if (!matchedSystem) return null;

    // Access the actor sync adapter if available.
    const actorSync = this._syncManager?._modules?.find(
      (m) => m.constructor?.name === 'ActorSync' || m._adapter
    );
    if (!actorSync?._adapter) return null;

    const adapter = actorSync._adapter;

    // For generic adapters, we can read the field definitions.
    // For built-in adapters, show the system ID and type slug.
    return {
      systemId: adapter.systemId || matchedSystem,
      characterTypeSlug: adapter.characterTypeSlug || 'unknown',
      adapterType: adapter._fieldDefs ? 'generic' : 'built-in',
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
  // Render hooks
  // ---------------------------------------------------------------------------

  /**
   * Post-render DOM setup: tabs, search input, select dropdowns,
   * type header collapse, checkbox toggles.
   * @param {object} context
   * @param {object} options
   */
  _onRender(context, options) {
    const el = this.element;
    if (!el) return;

    // --- Tab navigation ---
    this._initTabs(el);

    // --- Search input ---
    const search = el.querySelector('.dashboard-search');
    if (search) {
      search.addEventListener('input', (e) => {
        this._searchFilter = e.target.value;
        this.render({ force: true });
      });
    }

    // --- Entity type header collapse/expand ---
    el.querySelectorAll('.entity-type-header').forEach((header) => {
      header.addEventListener('click', (e) => {
        // Ignore clicks on the sync toggle button inside the header.
        if (e.target.closest('.type-sync-toggle')) return;

        const typeId = Number(header.dataset.typeId);
        if (this._collapsedTypes.has(typeId)) {
          this._collapsedTypes.delete(typeId);
        } else {
          this._collapsedTypes.add(typeId);
        }
        this.render({ force: true });
      });
    });

    // --- Type sync toggle ---
    el.querySelectorAll('.type-sync-toggle').forEach((btn) => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        const typeId = Number(btn.dataset.typeId);
        this._onToggleType(typeId);
      });
    });

    // --- Entity sync checkbox ---
    el.querySelectorAll('.entity-sync-toggle').forEach((checkbox) => {
      checkbox.addEventListener('change', (e) => {
        const entityId = e.currentTarget.dataset.entityId;
        this._onToggleEntity(entityId, e.currentTarget.checked);
      });
    });

    // --- Map link/unlink selects ---
    el.querySelectorAll('[data-action="link-scene"]').forEach((select) => {
      select.addEventListener('change', (e) => {
        const mapId = e.currentTarget.dataset.mapId;
        const sceneId = e.currentTarget.value;
        if (sceneId) this._onLinkScene(mapId, sceneId);
      });
    });

    el.querySelectorAll('[data-action="link-map"]').forEach((select) => {
      select.addEventListener('change', (e) => {
        const sceneId = e.currentTarget.dataset.sceneId;
        const mapId = e.currentTarget.value;
        if (mapId) this._onLinkScene(mapId, sceneId);
      });
    });
  }

  /**
   * Initialize tab navigation with CSS class toggling.
   * ApplicationV2 doesn't auto-manage CSS-based tabs from the template,
   * so we handle it manually.
   * @param {HTMLElement} el
   * @private
   */
  _initTabs(el) {
    const tabs = el.querySelectorAll('.dashboard-tabs .item');
    const panels = el.querySelectorAll('.dashboard-content .tab');

    // Apply the stored active tab.
    tabs.forEach((tab) => {
      const tabName = tab.dataset.tab;
      tab.classList.toggle('active', tabName === this._activeTab);
    });
    panels.forEach((panel) => {
      const tabName = panel.dataset.tab;
      panel.classList.toggle('active', tabName === this._activeTab);
    });

    // Listen for tab clicks.
    tabs.forEach((tab) => {
      tab.addEventListener('click', (e) => {
        e.preventDefault();
        const tabName = tab.dataset.tab;
        this._activeTab = tabName;

        tabs.forEach(t => t.classList.toggle('active', t.dataset.tab === tabName));
        panels.forEach(p => p.classList.toggle('active', p.dataset.tab === tabName));
      });
    });
  }

  // ---------------------------------------------------------------------------
  // Action handlers (ApplicationV2 actions pattern)
  // Called with `this` bound to the application instance by Foundry.
  // ---------------------------------------------------------------------------

  /** Refresh button: invalidate caches and re-render. */
  static #onRefresh() {
    this._cache = { entityTypes: null, entities: null, maps: null, calendar: null };
    this.render({ force: true });
  }

  /** Pull a single entity from Chronicle. */
  static #onPullEntityAction(event, target) {
    const entityId = target.dataset.entityId;
    this._onPullEntity(entityId);
  }

  /** Push a Foundry journal to Chronicle. */
  static #onPushJournalAction(event, target) {
    const journalId = target.dataset.journalId;
    this._onPushJournal(journalId);
  }

  /** Pull all Chronicle-only entities. */
  static #onPullAllAction() {
    this._onPullAll();
  }

  /** Push all Foundry-only journals. */
  static #onPushAllAction() {
    this._onPushAll();
  }

  /** Toggle entity visibility (public/private). */
  static #onToggleVisibilityAction(event, target) {
    const entityId = target.dataset.entityId;
    const isPrivate = target.dataset.isPrivate === 'true';
    this._onToggleVisibility(entityId, isPrivate);
  }

  /** Unlink a scene from its Chronicle map. */
  static #onUnlinkSceneAction(event, target) {
    const sceneId = target.dataset.sceneId;
    this._onUnlinkScene(sceneId);
  }

  /** Pull calendar date from Chronicle. */
  static #onPullDateAction() {
    this._onPullDate();
  }

  /** Push calendar date to Chronicle. */
  static #onPushDateAction() {
    this._onPushDate();
  }

  /** Reconnect WebSocket. */
  static #onReconnectAction() {
    this.api?.connect();
    setTimeout(() => this.render({ force: true }), 1000);
  }

  /** Clear the activity log. */
  static #onClearLogAction() {
    this._syncManager?.clearActivityLog();
    this.render({ force: true });
  }

  /** Open Foundry module settings. */
  static #onOpenSettingsAction() {
    game.settings.sheet.render(true);
  }

  /** Open a shop window via the ShopWidget module. */
  static #onOpenShopAction(event, target) {
    const entityId = target.dataset.entityId;
    const shopName = target.dataset.shopName || 'Shop';
    const shopWidget = this._syncManager?._modules?.find(
      m => m.constructor?.name === 'ShopWidget'
    );
    if (shopWidget) {
      shopWidget.openShop(entityId, shopName);
    }
  }

  /** Manually push an unlinked Foundry actor to Chronicle. */
  static #onPushActorAction(event, target) {
    const actorId = target.dataset.actorId;
    this._onPushActor(actorId);
  }

  // ---------------------------------------------------------------------------
  // Actions (business logic)
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
    this.render({ force: true });
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
      this.render({ force: true });
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
      this.render({ force: true });
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

    const data = await this._prepareContext();
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
      this.render({ force: true });
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
      this.render({ force: true });

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
    this.render({ force: true });

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
      this.render({ force: true });
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
      this.render({ force: true });
    } catch (err) {
      console.error('Chronicle Dashboard: Push date failed', err);
    }
  }

  /**
   * Manually push an unlinked actor to Chronicle.
   * Triggers the ActorSync create flow for an actor that wasn't
   * auto-pushed (e.g., existed before character sync was enabled).
   * @param {string} actorId
   * @private
   */
  async _onPushActor(actorId) {
    const actor = game.actors.get(actorId);
    if (!actor) return;

    const actorSync = this._syncManager?._modules?.find(
      (m) => m.constructor?.name === 'ActorSync'
    );
    if (!actorSync) {
      ui.notifications.warn('Character sync module not active.');
      return;
    }

    try {
      // Trigger the create flow by calling the hook handler directly.
      await actorSync._handleCreateActor(actor, {}, game.user.id);
      this._logActivity('push', `Pushed actor "${actor.name}" to Chronicle`);
      this.render({ force: true });
    } catch (err) {
      console.error('Chronicle Dashboard: Push actor failed', err);
      ui.notifications.error(`Failed to push actor: ${err.message}`);
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
    this._syncManager?.logActivity(type, message);
  }

  /** Force refresh all data and re-render. */
  refresh() {
    this._cache = { entityTypes: null, entities: null, maps: null, calendar: null };
    this.render({ force: true });
  }
}
