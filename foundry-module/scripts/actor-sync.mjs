/**
 * Chronicle Sync - Actor/Character Sync
 *
 * Bidirectional sync between Chronicle character entities and Foundry Actors.
 * Only active when a matching game system is detected (F-3) and the
 * syncCharacters setting is enabled.
 *
 * Sync flow:
 * - Chronicle → Foundry: Entity changes arrive via WebSocket, create/update Actor.
 * - Foundry → Chronicle: Actor changes detected via Hooks, push to Chronicle API.
 *
 * System-specific field mapping is delegated to adapter modules
 * (dnd5e-adapter.mjs, pf2e-adapter.mjs).
 */

import { getSetting } from './settings.mjs';

// Flag namespace for Chronicle data stored on Foundry documents.
const FLAG_SCOPE = 'chronicle-sync';

/**
 * ActorSync handles character entity ↔ Actor synchronization.
 */
export class ActorSync {
  constructor() {
    /** @type {import('./api-client.mjs').ChronicleAPI|null} */
    this._api = null;

    /** @type {import('./sync-manager.mjs').SyncManager|null} */
    this._syncManager = null;

    /** @type {boolean} Suppress hook processing during sync-initiated changes. */
    this._syncing = false;

    /** @type {object|null} Loaded system adapter module. */
    this._adapter = null;

    /** @type {number|null} Cached Chronicle entity type ID for characters. */
    this._characterTypeId = null;

    // Bound hook handlers for cleanup.
    this._onCreateActor = this._handleCreateActor.bind(this);
    this._onUpdateActor = this._handleUpdateActor.bind(this);
    this._onDeleteActor = this._handleDeleteActor.bind(this);
  }

  /**
   * Initialize the actor sync module.
   * Loads the appropriate system adapter and registers hooks.
   * @param {import('./api-client.mjs').ChronicleAPI} api
   */
  async init(api) {
    this._api = api;

    if (!getSetting('syncCharacters')) {
      console.log('Chronicle: Character sync disabled in settings');
      return;
    }

    // Load the system adapter based on matched system.
    this._adapter = await this._loadAdapter();
    if (!this._adapter) {
      console.log('Chronicle: No system adapter available, character sync inactive');
      return;
    }

    // Resolve the character entity type ID from Chronicle.
    await this._resolveCharacterTypeId();

    // Register Foundry hooks for Actor changes (character type only).
    Hooks.on('createActor', this._onCreateActor);
    Hooks.on('updateActor', this._onUpdateActor);
    Hooks.on('deleteActor', this._onDeleteActor);

    console.log(`Chronicle: Actor sync initialized (adapter: ${this._adapter.systemId})`);
  }

  /**
   * Handle incoming WebSocket messages for character entity events.
   * Filters to only process entities matching the character type.
   * @param {object} msg
   */
  async onMessage(msg) {
    if (!this._adapter || !getSetting('syncCharacters')) return;

    // Only handle entity events.
    if (!msg.type?.startsWith('entity.')) return;

    const entity = msg.payload;
    if (!entity) return;

    // Filter: only process character-type entities.
    if (!this._isCharacterEntity(entity)) return;

    switch (msg.type) {
      case 'entity.created':
        await this._onCharacterCreated(entity);
        break;
      case 'entity.updated':
        await this._onCharacterUpdated(entity);
        break;
      case 'entity.deleted':
        await this._onCharacterDeleted(entity);
        break;
    }
  }

  /**
   * Handle a sync mapping received during initial sync.
   * @param {object} mapping
   */
  async onSyncMapping(mapping) {
    if (mapping.chronicle_type !== 'actor') return;
    if (!this._adapter || !getSetting('syncCharacters')) return;

    // Check if the Foundry actor exists; if not, fetch and create it.
    const actor = game.actors.get(mapping.external_id);
    if (actor) return; // Already exists.

    try {
      const entity = await this._api.get(`/entities/${mapping.chronicle_id}`);
      if (entity && this._isCharacterEntity(entity)) {
        await this._onCharacterCreated(entity);
      }
    } catch (err) {
      console.warn('Chronicle: Failed to sync actor mapping', err);
    }
  }

  /**
   * Perform initial sync: pull all character entities and match to actors.
   */
  async onInitialSync() {
    if (!this._adapter || !getSetting('syncCharacters')) return;
    if (!this._characterTypeId) return;

    try {
      const result = await this._api.get(`/entities?type_id=${this._characterTypeId}&per_page=100`);
      const entities = result?.data || [];

      for (const entity of entities) {
        // Check if already linked to an actor.
        const existingActor = game.actors.find(
          (a) => a.getFlag(FLAG_SCOPE, 'entityId') === entity.id
        );

        if (existingActor) {
          // Update existing actor with latest data.
          await this._updateActorFromEntity(existingActor, entity);
        }
        // Don't auto-create actors during initial sync — only update existing links.
      }
    } catch (err) {
      console.warn('Chronicle: Actor initial sync failed', err);
    }
  }

  /**
   * Clean up hooks on destroy.
   */
  destroy() {
    Hooks.off('createActor', this._onCreateActor);
    Hooks.off('updateActor', this._onUpdateActor);
    Hooks.off('deleteActor', this._onDeleteActor);
  }

  // ---------------------------------------------------------------------------
  // Chronicle → Foundry
  // ---------------------------------------------------------------------------

  /**
   * Handle a new character entity from Chronicle.
   * Creates a new Foundry Actor if one isn't already linked.
   * @param {object} entity
   * @private
   */
  async _onCharacterCreated(entity) {
    // Check if an actor is already linked.
    const existing = game.actors.find(
      (a) => a.getFlag(FLAG_SCOPE, 'entityId') === entity.id
    );
    if (existing) return;

    try {
      this._syncing = true;

      const actorData = {
        name: entity.name,
        type: 'character',
        flags: {
          [FLAG_SCOPE]: {
            entityId: entity.id,
            lastSync: new Date().toISOString(),
          },
        },
      };

      // Apply adapter field mapping.
      const fieldUpdate = this._adapter.fromChronicleFields(entity);
      if (fieldUpdate) {
        // Merge dot-notation fields into nested structure for creation.
        for (const [key, value] of Object.entries(fieldUpdate)) {
          if (key === 'name') continue; // Already set above.
          _setNestedValue(actorData, key, value);
        }
      }

      const actor = await Actor.create(actorData);

      // Create sync mapping.
      await this._api.post('/sync/mappings', {
        chronicle_type: 'actor',
        chronicle_id: entity.id,
        external_system: 'foundry',
        external_id: actor.id,
        sync_direction: 'both',
      });

      console.log(`Chronicle: Created actor "${entity.name}" from character entity`);
    } catch (err) {
      console.error('Chronicle: Failed to create actor from entity', err);
    } finally {
      this._syncing = false;
    }
  }

  /**
   * Handle an updated character entity from Chronicle.
   * @param {object} entity
   * @private
   */
  async _onCharacterUpdated(entity) {
    const actor = game.actors.find(
      (a) => a.getFlag(FLAG_SCOPE, 'entityId') === entity.id
    );
    if (!actor) return;

    await this._updateActorFromEntity(actor, entity);
  }

  /**
   * Apply Chronicle entity data to a Foundry Actor.
   * @param {Actor} actor
   * @param {object} entity
   * @private
   */
  async _updateActorFromEntity(actor, entity) {
    try {
      this._syncing = true;

      const fieldUpdate = this._adapter.fromChronicleFields(entity);
      if (fieldUpdate && Object.keys(fieldUpdate).length > 0) {
        await actor.update(fieldUpdate);
      }

      // Update sync timestamp.
      await actor.setFlag(FLAG_SCOPE, 'lastSync', new Date().toISOString());

      console.log(`Chronicle: Updated actor "${actor.name}" from entity`);
    } catch (err) {
      console.error(`Chronicle: Failed to update actor "${actor.name}"`, err);
    } finally {
      this._syncing = false;
    }
  }

  /**
   * Handle a deleted character entity from Chronicle.
   * Removes the sync link but keeps the actor (to avoid data loss).
   * @param {object} data - Deletion event payload (may only contain id).
   * @private
   */
  async _onCharacterDeleted(data) {
    const entityId = data.id || data.resourceId;
    if (!entityId) return;

    const actor = game.actors.find(
      (a) => a.getFlag(FLAG_SCOPE, 'entityId') === entityId
    );
    if (!actor) return;

    try {
      this._syncing = true;
      await actor.unsetFlag(FLAG_SCOPE, 'entityId');
      await actor.unsetFlag(FLAG_SCOPE, 'lastSync');
      console.log(`Chronicle: Unlinked actor "${actor.name}" (entity deleted)`);
    } catch (err) {
      console.error('Chronicle: Failed to unlink actor after entity deletion', err);
    } finally {
      this._syncing = false;
    }
  }

  // ---------------------------------------------------------------------------
  // Foundry → Chronicle
  // ---------------------------------------------------------------------------

  /**
   * Handle Foundry createActor hook.
   * Only processes character-type actors created by the current user.
   * @param {Actor} actor
   * @param {object} options
   * @param {string} userId
   * @private
   */
  async _handleCreateActor(actor, options, userId) {
    if (this._syncing) return;
    if (userId !== game.user.id) return;
    if (actor.type !== 'character') return;
    if (!this._adapter || !this._characterTypeId) return;

    // Skip if already linked (came from Chronicle).
    if (actor.getFlag(FLAG_SCOPE, 'entityId')) return;

    try {
      const fields = this._adapter.toChronicleFields(actor);

      const entity = await this._api.post('/entities', {
        name: actor.name,
        entity_type_id: this._characterTypeId,
        is_private: false,
        fields_data: fields,
      });

      if (entity) {
        this._syncing = true;
        await actor.setFlag(FLAG_SCOPE, 'entityId', entity.id);
        await actor.setFlag(FLAG_SCOPE, 'lastSync', new Date().toISOString());
        this._syncing = false;

        // Create sync mapping.
        await this._api.post('/sync/mappings', {
          chronicle_type: 'actor',
          chronicle_id: entity.id,
          external_system: 'foundry',
          external_id: actor.id,
          sync_direction: 'both',
        });

        console.log(`Chronicle: Pushed new actor "${actor.name}" to Chronicle`);
      }
    } catch (err) {
      this._syncing = false;
      console.error('Chronicle: Failed to push new actor to Chronicle', err);
    }
  }

  /**
   * Handle Foundry updateActor hook.
   * Pushes field changes to the linked Chronicle entity.
   * @param {Actor} actor
   * @param {object} change
   * @param {object} options
   * @param {string} userId
   * @private
   */
  async _handleUpdateActor(actor, change, options, userId) {
    if (this._syncing) return;
    if (userId !== game.user.id) return;
    if (actor.type !== 'character') return;
    if (!this._adapter) return;

    const entityId = actor.getFlag(FLAG_SCOPE, 'entityId');
    if (!entityId) return;

    // Only push if system data or name changed.
    if (!change.system && !change.name) return;

    try {
      const fields = this._adapter.toChronicleFields(actor);
      const updatePayload = { fields_data: fields };
      if (change.name) updatePayload.name = change.name;

      await this._api.put(`/entities/${entityId}/fields`, { fields_data: fields });

      // Update name separately if changed.
      if (change.name) {
        await this._api.put(`/entities/${entityId}`, { name: change.name });
      }

      this._syncing = true;
      await actor.setFlag(FLAG_SCOPE, 'lastSync', new Date().toISOString());
      this._syncing = false;

      console.log(`Chronicle: Pushed actor "${actor.name}" changes to Chronicle`);
    } catch (err) {
      this._syncing = false;
      console.error('Chronicle: Failed to push actor update to Chronicle', err);
    }
  }

  /**
   * Handle Foundry deleteActor hook.
   * Deletes the linked Chronicle entity if one exists.
   * @param {Actor} actor
   * @param {object} options
   * @param {string} userId
   * @private
   */
  async _handleDeleteActor(actor, options, userId) {
    if (this._syncing) return;
    if (userId !== game.user.id) return;

    const entityId = actor.getFlag(FLAG_SCOPE, 'entityId');
    if (!entityId) return;

    try {
      await this._api.delete(`/entities/${entityId}`);
      console.log(`Chronicle: Deleted entity for actor "${actor.name}"`);
    } catch (err) {
      console.error('Chronicle: Failed to delete entity for deleted actor', err);
    }
  }

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  /**
   * Load the appropriate system adapter based on the matched Chronicle system.
   * @returns {Promise<object|null>} Adapter module or null.
   * @private
   */
  async _loadAdapter() {
    const matchedSystem = getSetting('detectedSystem');
    if (!matchedSystem) return null;

    try {
      switch (matchedSystem) {
        case 'dnd5e':
          return await import('./adapters/dnd5e-adapter.mjs');
        case 'pathfinder2e':
          return await import('./adapters/pf2e-adapter.mjs');
        default:
          console.warn(`Chronicle: No adapter for system "${matchedSystem}"`);
          return null;
      }
    } catch (err) {
      console.error(`Chronicle: Failed to load adapter for "${matchedSystem}"`, err);
      return null;
    }
  }

  /**
   * Resolve the character entity type ID from Chronicle.
   * Queries entity types and finds one matching the adapter's character slug.
   * @private
   */
  async _resolveCharacterTypeId() {
    if (!this._adapter?.characterTypeSlug) return;

    try {
      const result = await this._api.get('/entity-types');
      const types = result?.data || result || [];
      const match = types.find(
        (t) => t.slug === this._adapter.characterTypeSlug
          || t.name?.toLowerCase().includes('character')
      );
      if (match) {
        this._characterTypeId = match.id;
        console.log(`Chronicle: Character type resolved — "${match.name}" (ID: ${match.id})`);
      } else {
        console.warn('Chronicle: No character entity type found in campaign');
      }
    } catch (err) {
      console.warn('Chronicle: Failed to resolve character type ID', err);
    }
  }

  /**
   * Check if an entity is a character type based on type_slug, type_name,
   * or entity_type_id matching.
   * @param {object} entity
   * @returns {boolean}
   * @private
   */
  _isCharacterEntity(entity) {
    // Match by type slug if available.
    if (entity.type_slug && this._adapter?.characterTypeSlug) {
      return entity.type_slug === this._adapter.characterTypeSlug;
    }
    // Match by type name (fallback).
    if (entity.type_name) {
      return entity.type_name.toLowerCase().includes('character');
    }
    // Match by type ID if resolved.
    if (this._characterTypeId && entity.entity_type_id) {
      return entity.entity_type_id === this._characterTypeId;
    }
    return false;
  }

  /**
   * Get all synced actors with their status for dashboard display.
   * @returns {Array<{id: string, name: string, entityId: string|null, synced: boolean, lastSync: string|null}>}
   */
  getSyncedActors() {
    if (!this._adapter) return [];

    return game.actors.contents
      .filter((a) => a.type === 'character')
      .map((a) => {
        const entityId = a.getFlag(FLAG_SCOPE, 'entityId') || null;
        const lastSync = a.getFlag(FLAG_SCOPE, 'lastSync') || null;
        return {
          id: a.id,
          name: a.name,
          entityId,
          synced: !!entityId,
          lastSync,
          img: a.img,
        };
      })
      .sort((a, b) => {
        // Synced actors first, then alphabetical.
        if (a.synced !== b.synced) return a.synced ? -1 : 1;
        return a.name.localeCompare(b.name);
      });
  }
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

/**
 * Set a nested value on an object using dot-notation key.
 * e.g., _setNestedValue(obj, 'system.abilities.str.value', 10)
 * @param {object} obj
 * @param {string} path
 * @param {*} value
 */
function _setNestedValue(obj, path, value) {
  const keys = path.split('.');
  let current = obj;
  for (let i = 0; i < keys.length - 1; i++) {
    if (!(keys[i] in current) || typeof current[keys[i]] !== 'object') {
      current[keys[i]] = {};
    }
    current = current[keys[i]];
  }
  current[keys[keys.length - 1]] = value;
}
