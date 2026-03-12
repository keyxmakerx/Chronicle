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

import { getSetting, getSyncExclusions } from './settings.mjs';

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
   * Fetches full entity data from the API since WebSocket payloads may
   * not include content fields (entry_html, fields_data, tags).
   * @param {object} entity - Chronicle entity data (possibly partial).
   * @private
   */
  async _onEntityCreated(entity) {
    if (!entity?.id) return;

    // Skip if entity or its type is excluded from sync.
    if (this._isExcluded(entity)) return;

    // Check if we already have a journal for this entity.
    const existing = game.journal.find(
      (j) => j.getFlag(FLAG_SCOPE, 'entityId') === entity.id
    );
    if (existing) return;

    // Fetch full entity data (WS payload may be partial).
    try {
      const fullEntity = await this._api.get(`/entities/${entity.id}`);
      await this._createJournalFromEntity(fullEntity || entity);
    } catch (err) {
      // Fallback to WS payload data if fetch fails.
      console.warn('Chronicle: Failed to fetch full entity, using WS payload', err);
      await this._createJournalFromEntity(entity);
    }
  }

  /**
   * Update an existing JournalEntry from a Chronicle entity change.
   * @param {object} entity
   * @private
   */
  async _onEntityUpdated(entity) {
    if (!entity?.id) return;

    // Skip if entity or its type is excluded from sync.
    if (this._isExcluded(entity)) return;

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
      // Update the journal name and ownership from Chronicle permissions.
      const ownership = await this._buildOwnership(entity);
      await journal.update({ name: entity.name, ownership });

      // Split entity content into pages and sync them.
      await this._syncPagesToJournal(journal, entity.entry_html || '');

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

      // Split entity content into pages by top-level headings.
      const sections = this._splitByHeadings(entity.entry_html || '');

      let sortIndex = 1;
      for (const section of sections) {
        const pageData = {
          name: section.title,
          type: 'text',
          text: { content: section.content },
          sort: sortIndex++,
        };

        // Monk's Enhanced Journal uses enhanced page flags.
        if (isMonksActive) {
          pageData.flags = {
            'monks-enhanced-journal': { type: 'base' },
          };
        }

        pages.push(pageData);
      }

      // Determine ownership from Chronicle permissions.
      const ownership = await this._buildOwnership(entity);

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
      // Concatenate all text pages into a single entry for Chronicle.
      const entryHtml = this._collectTextPages(journal);

      const entity = await this._api.post('/entities', {
        name: journal.name,
        entity_type_id: 0, // Default type — will use first available.
        is_private: (journal.ownership?.default ?? 0) < CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER,
        entry: entryHtml,
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

        // Push initial permissions from Foundry ownership.
        const isPrivate =
          (journal.ownership?.default ?? 0) < CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER;
        await this._pushPermissions(entity.id, journal.ownership, isPrivate);

        console.log(`Chronicle: Pushed new journal "${journal.name}" to Chronicle`);
      }
    } catch (err) {
      console.error('Chronicle: Failed to push journal to Chronicle', err);
    }
  }

  /**
   * Handle Foundry JournalEntry update — push changes to Chronicle.
   * Detects name, content, and ownership changes and pushes all to Chronicle.
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
      // Concatenate all text pages into a single entry for Chronicle.
      const entryHtml = this._collectTextPages(journal);
      const isPrivate =
        (journal.ownership?.default ?? 0) < CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER;

      await this._api.put(`/entities/${entityId}`, {
        name: journal.name,
        is_private: isPrivate,
        entry: entryHtml,
      });

      // Push ownership changes as Chronicle permission updates.
      // Map Foundry default ownership level to Chronicle visibility mode.
      await this._pushPermissions(entityId, journal.ownership, isPrivate);

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

  /**
   * Check if an entity is excluded from auto-sync via dashboard settings.
   * @param {object} entity - Chronicle entity with id and optionally entity_type_id.
   * @returns {boolean}
   * @private
   */
  _isExcluded(entity) {
    const exclusions = getSyncExclusions();
    if (exclusions.excludedEntities.includes(entity.id)) return true;
    if (entity.entity_type_id && exclusions.excludedTypes.includes(entity.entity_type_id)) return true;
    return false;
  }

  // --- Permission Mapping Helpers ---

  /**
   * Build a Foundry ownership object from Chronicle entity permissions.
   * Fetches the entity's permission grants and maps them to Foundry ownership levels.
   *
   * Mapping:
   * - visibility "default" + is_private=true → { default: NONE }
   * - visibility "default" + is_private=false → { default: OBSERVER }
   * - visibility "custom" → uses role-based grants to determine default level,
   *   and maps user-specific grants to per-Foundry-user ownership where possible.
   *
   * @param {object} entity - Chronicle entity with id, is_private, visibility fields.
   * @returns {object} Foundry ownership object.
   * @private
   */
  async _buildOwnership(entity) {
    // Fallback for legacy or simple visibility.
    if (!entity.visibility || entity.visibility === 'default') {
      return entity.is_private
        ? { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.NONE }
        : { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER };
    }

    // Custom visibility — fetch permission grants from API.
    try {
      const permsData = await this._api.get(`/entities/${entity.id}/permissions`);
      if (!permsData?.permissions) {
        // Fallback if API call returns no data.
        return { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.NONE };
      }

      const ownership = { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.NONE };

      for (const grant of permsData.permissions) {
        if (grant.subject_type === 'role') {
          // Role "1" = Player. If players have a grant, set default ownership.
          if (grant.subject_id === '1') {
            ownership.default =
              grant.permission === 'edit'
                ? CONST.DOCUMENT_OWNERSHIP_LEVELS.OWNER
                : CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER;
          }
          // Role "2" = Scribe. Scribes get at least OBSERVER.
          // (Foundry doesn't have a "scribe" concept; handle via default level.)
        }
        // User-specific and group grants are stored in flags for reference
        // but can't be mapped to Foundry users without a user ID mapping table.
      }

      return ownership;
    } catch (err) {
      console.warn('Chronicle: Failed to fetch entity permissions, using fallback', err);
      return entity.is_private
        ? { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.NONE }
        : { default: CONST.DOCUMENT_OWNERSHIP_LEVELS.OBSERVER };
    }
  }

  /**
   * Push Foundry ownership changes to Chronicle as permission updates.
   * Maps Foundry default ownership level to Chronicle visibility mode:
   * - NONE → visibility "default", is_private=true
   * - OBSERVER → visibility "default", is_private=false
   * - Changes in per-user ownership are logged but not yet pushed
   *   (requires user ID mapping table).
   *
   * @param {string} entityId - Chronicle entity ID.
   * @param {object} ownership - Foundry ownership object.
   * @param {boolean} isPrivate - Derived privacy flag from default ownership.
   * @private
   */
  async _pushPermissions(entityId, ownership, isPrivate) {
    try {
      // Build permission grants from Foundry ownership.
      const permissions = [];

      if (!isPrivate) {
        // Entity is visible: grant view to player role.
        permissions.push({
          subject_type: 'role',
          subject_id: '1',
          permission: 'view',
        });
      }

      // Determine visibility mode based on whether there are per-user grants.
      // For now, use "default" mode since we can't map Foundry user IDs
      // to Chronicle user IDs without a mapping table.
      const hasPerUserGrants = Object.keys(ownership || {}).some(
        (key) => key !== 'default' && ownership[key] > CONST.DOCUMENT_OWNERSHIP_LEVELS.NONE
      );

      // Only push custom permissions if there are meaningful grants.
      if (hasPerUserGrants || permissions.length > 0) {
        await this._api.put(`/entities/${entityId}/permissions`, {
          visibility: hasPerUserGrants ? 'custom' : 'default',
          is_private: isPrivate,
          permissions,
        });
      }
    } catch (err) {
      // Permission push is best-effort — don't fail the sync.
      console.warn('Chronicle: Failed to push permissions update', err);
    }
  }

  // --- Multi-Page Helpers ---

  /**
   * Split HTML content by top-level headings (h1/h2) into named sections.
   * Each section becomes a separate Foundry journal page.
   * If no headings are found, returns a single section with the entity name.
   * @param {string} html - The entity entry_html content.
   * @returns {Array<{title: string, content: string}>}
   * @private
   */
  _splitByHeadings(html) {
    if (!html) return [{ title: 'Content', content: '' }];

    // Match h1 or h2 tags to use as page break points.
    const headingRegex = /<h[12][^>]*>(.*?)<\/h[12]>/gi;
    const matches = [...html.matchAll(headingRegex)];

    // No headings found — return as single page.
    if (matches.length === 0) {
      return [{ title: 'Content', content: html }];
    }

    const sections = [];

    // Content before the first heading (if any).
    const preContent = html.substring(0, matches[0].index).trim();
    if (preContent) {
      sections.push({ title: 'Overview', content: preContent });
    }

    // Each heading starts a new section, ending at the next heading or end of string.
    for (let i = 0; i < matches.length; i++) {
      const match = matches[i];
      const startAfterHeading = match.index + match[0].length;
      const endIndex = i + 1 < matches.length ? matches[i + 1].index : html.length;
      const sectionContent = html.substring(startAfterHeading, endIndex).trim();

      // Strip HTML tags from heading text for the page title.
      const title = match[1].replace(/<[^>]*>/g, '').trim() || `Section ${i + 1}`;

      // Include the heading in the page content for context.
      sections.push({
        title,
        content: match[0] + sectionContent,
      });
    }

    return sections;
  }

  /**
   * Collect all text pages from a Foundry JournalEntry and concatenate
   * them into a single HTML string for Chronicle. Pages are joined in
   * sort order.
   * @param {JournalEntry} journal
   * @returns {string} Combined HTML content.
   * @private
   */
  _collectTextPages(journal) {
    const textPages = journal.pages
      .filter((p) => p.type === 'text')
      .sort((a, b) => a.sort - b.sort);

    if (textPages.length === 0) return '';
    if (textPages.length === 1) return textPages[0].text?.content || '';

    // Multiple pages: concatenate with the page name as a heading separator.
    return textPages
      .map((page) => {
        const content = page.text?.content || '';
        // If the page content already starts with a heading, use it as-is.
        if (/^<h[12][^>]*>/i.test(content.trim())) return content;
        // Otherwise, wrap the page name as an h2 heading.
        return `<h2>${page.name}</h2>\n${content}`;
      })
      .join('\n');
  }

  /**
   * Sync entity HTML content to journal pages. Splits by headings and
   * updates existing pages or creates/removes pages as needed.
   * @param {JournalEntry} journal
   * @param {string} html - Entity entry_html content.
   * @private
   */
  async _syncPagesToJournal(journal, html) {
    const sections = this._splitByHeadings(html);
    const existingTextPages = journal.pages
      .filter((p) => p.type === 'text')
      .sort((a, b) => a.sort - b.sort);

    // Update existing pages and create new ones as needed.
    for (let i = 0; i < sections.length; i++) {
      const section = sections[i];

      if (i < existingTextPages.length) {
        // Update existing page.
        const page = existingTextPages[i];
        const updates = { 'text.content': section.content };
        if (page.name !== section.title) {
          updates.name = section.title;
        }
        await page.update(updates);
      } else {
        // Create new page.
        await journal.createEmbeddedDocuments('JournalEntryPage', [
          {
            name: section.title,
            type: 'text',
            text: { content: section.content },
            sort: (existingTextPages.length + i) * 100,
          },
        ]);
      }
    }

    // Remove excess pages if entity has fewer sections than journal has pages.
    if (sections.length < existingTextPages.length) {
      const pagesToDelete = existingTextPages
        .slice(sections.length)
        .map((p) => p.id);
      if (pagesToDelete.length > 0) {
        await journal.deleteEmbeddedDocuments('JournalEntryPage', pagesToDelete);
      }
    }
  }
}
