/**
 * Chronicle Sync - Journal/Entity Sync
 *
 * Bidirectional sync between Chronicle entities and Foundry JournalEntries.
 * Supports standard text pages and Monk's Enhanced Journal pages.
 *
 * Sync flow:
 * - Chronicle → Foundry: Entity changes arrive via WebSocket, create/update JournalEntry.
 * - Foundry → Chronicle: JournalEntry changes detected via Hooks, push to Chronicle API.
 */

import { getSetting } from './settings.mjs';

// Flag namespace for Chronicle data stored on Foundry documents.
const FLAG_SCOPE = 'chronicle-sync';

/**
 * JournalSync handles entity ↔ JournalEntry synchronization.
 */
export class JournalSync {
  constructor() {
    /** @type {import('./api-client.mjs').ChronicleAPI|null} */
    this._api = null;

    /** @type {boolean} Suppress hook processing during sync-initiated changes. */
    this._syncing = false;

    // Bound hook handlers for cleanup.
    this._onCreateJournal = this._handleCreateJournal.bind(this);
    this._onUpdateJournal = this._handleUpdateJournal.bind(this);
    this._onDeleteJournal = this._handleDeleteJournal.bind(this);
  }

  /**
   * Initialize the journal sync module.
   * @param {import('./api-client.mjs').ChronicleAPI} api
   */
  async init(api) {
    this._api = api;

    if (!getSetting('syncJournals')) return;

    // Register Foundry hooks for JournalEntry changes.
    Hooks.on('createJournalEntry', this._onCreateJournal);
    Hooks.on('updateJournalEntry', this._onUpdateJournal);
    Hooks.on('deleteJournalEntry', this._onDeleteJournal);

    console.log('Chronicle: Journal sync initialized');
  }

  /**
   * Handle incoming WebSocket messages for entity events.
   * @param {object} msg
   */
  async onMessage(msg) {
    if (!getSetting('syncJournals')) return;

    switch (msg.type) {
      case 'entity.created':
        await this._onEntityCreated(msg.payload);
        break;
      case 'entity.updated':
        await this._onEntityUpdated(msg.payload);
        break;
      case 'entity.deleted':
        await this._onEntityDeleted(msg.payload);
        break;
    }
  }

  /**
   * Handle a sync mapping received during initial sync.
   * @param {object} mapping
   */
  async onSyncMapping(mapping) {
    if (mapping.chronicle_type !== 'entity') return;
    if (!getSetting('syncJournals')) return;

    // Check if the Foundry journal exists; if not, fetch and create it.
    const journal = game.journal.get(mapping.external_id);
    if (!journal) {
      try {
        const entity = await this._api.get(`/entities/${mapping.chronicle_id}`);
        if (entity) {
          await this._createJournalFromEntity(entity, mapping.external_id);
        }
      } catch (err) {
        console.warn(`Chronicle: Failed to sync entity ${mapping.chronicle_id}`, err);
      }
    }
  }

  /**
   * Clean up hooks on destroy.
   */
  destroy() {
    Hooks.off('createJournalEntry', this._onCreateJournal);
    Hooks.off('updateJournalEntry', this._onUpdateJournal);
    Hooks.off('deleteJournalEntry', this._onDeleteJournal);
  }

  // --- Chronicle → Foundry ---

  /**
   * Create a new JournalEntry from a Chronicle entity.
   * @param {object} entity - Chronicle entity data.
   * @private
   */
  async _onEntityCreated(entity) {
    if (!entity?.id) return;

    // Check if we already have a journal for this entity.
    const existing = game.journal.find(
      (j) => j.getFlag(FLAG_SCOPE, 'entityId') === entity.id
    );
    if (existing) return;

    await this._createJournalFromEntity(entity);
  }

  /**
   * Update an existing JournalEntry from a Chronicle entity change.
   * @param {object} entity
   * @private
   */
  async _onEntityUpdated(entity) {
    if (!entity?.id) return;

    const journal = game.journal.find(
      (j) => j.getFlag(FLAG_SCOPE, 'entityId') === entity.id
    );
    if (!journal) {
      // Entity was updated but we don't have a journal for it yet — create one.
      await this._createJournalFromEntity(entity);
      return;
    }

    this._syncing = true;
    try {
      // Update the journal name.
      const updates = { name: entity.name };

      // Update ownership based on privacy.
      if (entity.is_private) {
        updates.ownership = { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.NONE };
      } else {
        updates.ownership = { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER };
      }

      await journal.update(updates);

      // Update the text page content.
      const textPage = journal.pages.find((p) => p.type === 'text');
      if (textPage && entity.entry_html) {
        await textPage.update({
          'text.content': entity.entry_html,
        });
      }

      // Update flags with latest entity data.
      await journal.setFlag(FLAG_SCOPE, 'entityType', entity.type_name || '');
      await journal.setFlag(FLAG_SCOPE, 'fields', entity.fields_data || {});
      await journal.setFlag(FLAG_SCOPE, 'tags', entity.tags || []);
      await journal.setFlag(FLAG_SCOPE, 'lastSync', new Date().toISOString());

      console.log(`Chronicle: Updated journal "${journal.name}" from entity`);
    } finally {
      this._syncing = false;
    }
  }

  /**
   * Delete a JournalEntry when its Chronicle entity is deleted.
   * @param {object} data - { id: entityId }
   * @private
   */
  async _onEntityDeleted(data) {
    if (!data?.id) return;

    const journal = game.journal.find(
      (j) => j.getFlag(FLAG_SCOPE, 'entityId') === data.id
    );
    if (!journal) return;

    this._syncing = true;
    try {
      await journal.delete();
      console.log(`Chronicle: Deleted journal for entity ${data.id}`);
    } finally {
      this._syncing = false;
    }
  }

  /**
   * Create a Foundry JournalEntry from Chronicle entity data.
   * @param {object} entity
   * @param {string} [forceId] - Optionally use a specific Foundry document ID.
   * @private
   */
  async _createJournalFromEntity(entity, forceId) {
    this._syncing = true;
    try {
      const isMonksActive = game.modules.get('monks-enhanced-journal')?.active;

      // Build journal pages.
      const pages = [];

      // Image page (if entity has an image).
      if (entity.image_path) {
        pages.push({
          name: 'Image',
          type: 'image',
          src: entity.image_path,
          sort: 0,
        });
      }

      // Text page with entity content.
      if (isMonksActive) {
        // Monk's Enhanced Journal uses enhanced page type.
        pages.push({
          name: entity.name,
          type: 'text',
          text: { content: entity.entry_html || '' },
          sort: 1,
          flags: {
            'monks-enhanced-journal': {
              type: 'base',
            },
          },
        });
      } else {
        pages.push({
          name: entity.name,
          type: 'text',
          text: { content: entity.entry_html || '' },
          sort: 1,
        });
      }

      // Determine ownership.
      const ownership = entity.is_private
        ? { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.NONE }
        : { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER };

      const journalData = {
        name: entity.name,
        pages,
        ownership,
        flags: {
          [FLAG_SCOPE]: {
            entityId: entity.id,
            entityType: entity.type_name || '',
            fields: entity.fields_data || {},
            tags: entity.tags || [],
            lastSync: new Date().toISOString(),
          },
        },
      };

      if (forceId) {
        journalData._id = forceId;
      }

      const journal = await JournalEntry.create(journalData);

      // Create sync mapping on Chronicle server.
      if (journal) {
        try {
          await this._api.post('/sync/mappings', {
            chronicle_type: 'entity',
            chronicle_id: entity.id,
            external_system: 'foundry',
            external_id: journal.id,
            sync_direction: 'both',
            sync_metadata: { foundry_type: 'JournalEntry' },
          });
        } catch (err) {
          console.warn('Chronicle: Failed to create sync mapping', err);
        }
      }

      console.log(`Chronicle: Created journal "${entity.name}" from entity`);
      return journal;
    } finally {
      this._syncing = false;
    }
  }

  // --- Foundry → Chronicle ---

  /**
   * Handle Foundry JournalEntry creation — push to Chronicle if not from sync.
   * @param {JournalEntry} journal
   * @param {object} options
   * @param {string} userId
   * @private
   */
  async _handleCreateJournal(journal, options, userId) {
    if (this._syncing) return;
    if (userId !== game.user.id) return;

    // Skip if this journal was created by Chronicle sync.
    if (journal.getFlag(FLAG_SCOPE, 'entityId')) return;

    // Create entity in Chronicle from this new journal.
    try {
      const textPage = journal.pages.find((p) => p.type === 'text');
      const imagePage = journal.pages.find((p) => p.type === 'image');

      const entity = await this._api.post('/entities', {
        name: journal.name,
        entity_type_id: 0, // Default type — will use first available.
        is_private: (journal.ownership?.default ?? 0) < CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER,
        entry: textPage?.text?.content || '',
      });

      if (entity) {
        this._syncing = true;
        try {
          await journal.setFlag(FLAG_SCOPE, 'entityId', entity.id);
          await journal.setFlag(FLAG_SCOPE, 'lastSync', new Date().toISOString());
        } finally {
          this._syncing = false;
        }

        // Create sync mapping.
        await this._api.post('/sync/mappings', {
          chronicle_type: 'entity',
          chronicle_id: entity.id,
          external_system: 'foundry',
          external_id: journal.id,
          sync_direction: 'both',
        });

        console.log(`Chronicle: Pushed new journal "${journal.name}" to Chronicle`);
      }
    } catch (err) {
      console.error('Chronicle: Failed to push journal to Chronicle', err);
    }
  }

  /**
   * Handle Foundry JournalEntry update — push changes to Chronicle.
   * @param {JournalEntry} journal
   * @param {object} change
   * @param {object} options
   * @param {string} userId
   * @private
   */
  async _handleUpdateJournal(journal, change, options, userId) {
    if (this._syncing) return;
    if (userId !== game.user.id) return;

    const entityId = journal.getFlag(FLAG_SCOPE, 'entityId');
    if (!entityId) return;

    try {
      const textPage = journal.pages.find((p) => p.type === 'text');

      await this._api.put(`/entities/${entityId}`, {
        name: journal.name,
        is_private: (journal.ownership?.default ?? 0) < CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER,
        entry: textPage?.text?.content || '',
      });

      this._syncing = true;
      try {
        await journal.setFlag(FLAG_SCOPE, 'lastSync', new Date().toISOString());
      } finally {
        this._syncing = false;
      }

      console.log(`Chronicle: Pushed journal update "${journal.name}" to Chronicle`);
    } catch (err) {
      console.error('Chronicle: Failed to push journal update', err);
    }
  }

  /**
   * Handle Foundry JournalEntry deletion — notify Chronicle.
   * @param {JournalEntry} journal
   * @param {object} options
   * @param {string} userId
   * @private
   */
  async _handleDeleteJournal(journal, options, userId) {
    if (this._syncing) return;
    if (userId !== game.user.id) return;

    const entityId = journal.getFlag(FLAG_SCOPE, 'entityId');
    if (!entityId) return;

    try {
      await this._api.delete(`/entities/${entityId}`);
      console.log(`Chronicle: Deleted entity ${entityId} from journal deletion`);
    } catch (err) {
      // Entity may already be deleted on Chronicle side — that's fine.
      console.warn('Chronicle: Failed to delete entity on Chronicle', err);
    }
  }
}
