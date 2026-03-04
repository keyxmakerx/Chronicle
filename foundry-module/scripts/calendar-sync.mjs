/**
 * Chronicle Sync - Calendar/Calendaria/SimpleCalendar Sync
 *
 * Bidirectional sync between Chronicle's calendar system and Foundry VTT
 * calendar modules. Supports both Calendaria and SimpleCalendar via an
 * adapter pattern. When neither is active, this module is a no-op.
 *
 * Sync flow:
 * - Chronicle → Foundry: Calendar changes arrive via WebSocket, update
 *   the active Foundry calendar module (date, events).
 * - Foundry → Chronicle: Calendar changes detected via Hooks, push to
 *   Chronicle API (PUT /calendar/date, POST/PUT/DELETE /calendar/events).
 *
 * Initial sync: On first connect, fetches Chronicle calendar structure and
 * optionally pushes to the active Foundry calendar module.
 */

import { getSetting } from './settings.mjs';

const FLAG_SCOPE = 'chronicle-sync';

/**
 * CalendarSync handles calendar ↔ Foundry calendar module synchronization.
 */
export class CalendarSync {
  constructor() {
    /** @type {import('./api-client.mjs').ChronicleAPI|null} */
    this._api = null;
    this._syncing = false;

    /** @type {'calendaria'|'simple-calendar'|null} */
    this._calendarModule = null;

    /** @type {object|null} Cached Chronicle calendar structure. */
    this._chronicleCalendar = null;

    // Bound hook handlers for cleanup.
    this._boundHandlers = {};
  }

  /**
   * Initialize calendar sync. Detects which Foundry calendar module is
   * active and registers appropriate hooks.
   * @param {import('./api-client.mjs').ChronicleAPI} api
   */
  async init(api) {
    this._api = api;

    if (!getSetting('syncCalendar')) return;

    // Detect active calendar module.
    if (game.modules.get('calendaria')?.active) {
      this._calendarModule = 'calendaria';
    } else if (game.modules.get('foundryvtt-simple-calendar')?.active) {
      this._calendarModule = 'simple-calendar';
    }

    if (!this._calendarModule) {
      console.log('Chronicle: No calendar module detected (Calendaria or SimpleCalendar). Calendar sync disabled.');
      return;
    }

    this._registerHooks();
    console.log(`Chronicle: Calendar sync initialized (${this._calendarModule} detected)`);
  }

  /**
   * Handle incoming WebSocket messages for calendar events.
   * @param {object} msg
   */
  async onMessage(msg) {
    if (!getSetting('syncCalendar') || !this._calendarModule) return;

    switch (msg.type) {
      case 'calendar.date.advanced':
        await this._onChronicaleDateAdvanced(msg.payload);
        break;
      case 'calendar.event.created':
        await this._onChronicleEventCreated(msg.payload);
        break;
      case 'calendar.event.updated':
        await this._onChronicleEventUpdated(msg.payload);
        break;
      case 'calendar.event.deleted':
        await this._onChronicleEventDeleted(msg.payload);
        break;
    }
  }

  /**
   * Perform initial calendar sync on WebSocket connect.
   * Fetches Chronicle calendar structure and syncs current date.
   */
  async onInitialSync() {
    if (!getSetting('syncCalendar') || !this._calendarModule) return;

    try {
      this._chronicleCalendar = await this._api.get('/calendar');
      if (!this._chronicleCalendar) {
        console.log('Chronicle: No calendar configured for this campaign');
        return;
      }

      // Sync the current date from Chronicle to the Foundry calendar module.
      await this._setLocalDate({
        year: this._chronicleCalendar.current_year,
        month: this._chronicleCalendar.current_month,
        day: this._chronicleCalendar.current_day,
        hour: this._chronicleCalendar.current_hour,
        minute: this._chronicleCalendar.current_minute,
      });

      console.log('Chronicle: Calendar initial sync complete');
    } catch (err) {
      console.error('Chronicle: Calendar initial sync failed', err);
    }
  }

  /**
   * Clean up hooks on destroy.
   */
  destroy() {
    if (this._calendarModule === 'calendaria') {
      Hooks.off('calendariaDateChange', this._boundHandlers.dateChange);
      Hooks.off('calendariaEventCreate', this._boundHandlers.eventCreate);
      Hooks.off('calendariaEventUpdate', this._boundHandlers.eventUpdate);
      Hooks.off('calendariaEventDelete', this._boundHandlers.eventDelete);
    } else if (this._calendarModule === 'simple-calendar') {
      Hooks.off('simple-calendar-date-time-change', this._boundHandlers.dateChange);
    }
    this._boundHandlers = {};
  }

  // --- Hook Registration (adapter pattern) ---

  /**
   * Register hooks for the detected calendar module.
   * @private
   */
  _registerHooks() {
    if (this._calendarModule === 'calendaria') {
      this._boundHandlers.dateChange = this._onLocalDateChange.bind(this);
      this._boundHandlers.eventCreate = this._onLocalEventCreate.bind(this);
      this._boundHandlers.eventUpdate = this._onLocalEventUpdate.bind(this);
      this._boundHandlers.eventDelete = this._onLocalEventDelete.bind(this);

      Hooks.on('calendariaDateChange', this._boundHandlers.dateChange);
      Hooks.on('calendariaEventCreate', this._boundHandlers.eventCreate);
      Hooks.on('calendariaEventUpdate', this._boundHandlers.eventUpdate);
      Hooks.on('calendariaEventDelete', this._boundHandlers.eventDelete);
    } else if (this._calendarModule === 'simple-calendar') {
      this._boundHandlers.dateChange = this._onSimpleCalendarDateChange.bind(this);
      Hooks.on('simple-calendar-date-time-change', this._boundHandlers.dateChange);
      // SimpleCalendar does not expose event create/update/delete hooks.
      // Events are managed through its internal note system (journal entries).
    }
  }

  // --- Chronicle → Foundry ---

  /**
   * Update the local Foundry calendar date from Chronicle.
   * @param {object} data - { year, month, day, hour, minute }
   * @private
   */
  async _onChronicaleDateAdvanced(data) {
    if (!data) return;
    await this._setLocalDate(data);
  }

  /**
   * Create a local calendar event from Chronicle data.
   * @param {object} data - Chronicle event object.
   * @private
   */
  async _onChronicleEventCreated(data) {
    if (!data) return;
    this._syncing = true;
    try {
      await this._createLocalEvent(data);
    } finally {
      this._syncing = false;
    }
  }

  /**
   * Update a local calendar event from Chronicle data.
   * @param {object} data - Chronicle event object.
   * @private
   */
  async _onChronicleEventUpdated(data) {
    if (!data) return;
    this._syncing = true;
    try {
      await this._updateLocalEvent(data);
    } finally {
      this._syncing = false;
    }
  }

  /**
   * Delete a local calendar event from Chronicle data.
   * @param {object} data - { id: eventId }
   * @private
   */
  async _onChronicleEventDeleted(data) {
    if (!data) return;
    this._syncing = true;
    try {
      await this._deleteLocalEvent(data);
    } finally {
      this._syncing = false;
    }
  }

  // --- Foundry → Chronicle ---

  /**
   * Push date change from Calendaria to Chronicle.
   * @param {object} dateData - { year, month, day }
   * @private
   */
  async _onLocalDateChange(dateData) {
    if (this._syncing) return;
    if (!game.user.isGM) return;

    try {
      await this._api.put('/calendar/date', {
        year: dateData.year,
        month: dateData.month,
        day: dateData.day,
        hour: dateData.hour || 0,
        minute: dateData.minute || 0,
      });
    } catch (err) {
      console.error('Chronicle: Failed to push date to Chronicle', err);
    }
  }

  /**
   * Push date change from SimpleCalendar to Chronicle.
   * SimpleCalendar provides a different hook payload format.
   * @param {object} data - SimpleCalendar date-time-change hook data.
   * @private
   */
  async _onSimpleCalendarDateChange(data) {
    if (this._syncing) return;
    if (!game.user.isGM) return;

    // SimpleCalendar uses different data shape depending on version.
    // The hook provides { date: { year, month, day, ... }, diff: N, ... }
    const date = data?.date || data;
    if (!date) return;

    try {
      await this._api.put('/calendar/date', {
        year: date.year,
        // SimpleCalendar months are 0-indexed; Chronicle is 1-indexed.
        month: (date.month ?? 0) + 1,
        day: (date.day ?? 0) + 1,
        hour: date.hour || 0,
        minute: date.minute || 0,
      });
    } catch (err) {
      console.error('Chronicle: Failed to push SimpleCalendar date to Chronicle', err);
    }
  }

  /**
   * Push new event from Calendaria to Chronicle.
   * @param {object} eventData
   * @private
   */
  async _onLocalEventCreate(eventData) {
    if (this._syncing) return;
    if (!game.user.isGM) return;

    try {
      const result = await this._api.post('/calendar/events', {
        name: eventData.name || 'Untitled Event',
        year: eventData.year,
        month: eventData.month,
        day: eventData.day,
        description: eventData.description || '',
        visibility: 'everyone',
      });

      // Store the Chronicle event ID in the local module's data for later sync.
      if (result?.id && eventData.id) {
        this._storeEventMapping(eventData.id, result.id);
      }
    } catch (err) {
      console.error('Chronicle: Failed to push calendar event', err);
    }
  }

  /**
   * Push event update from Calendaria to Chronicle.
   * @param {object} eventData
   * @private
   */
  async _onLocalEventUpdate(eventData) {
    if (this._syncing) return;
    if (!game.user.isGM) return;

    const chronicleId = this._getChronicleEventId(eventData.id);
    if (!chronicleId) {
      // Event was created outside Chronicle; create it instead.
      await this._onLocalEventCreate(eventData);
      return;
    }

    try {
      await this._api.put(`/calendar/events/${chronicleId}`, {
        name: eventData.name || 'Untitled Event',
        year: eventData.year,
        month: eventData.month,
        day: eventData.day,
        description: eventData.description || '',
      });
    } catch (err) {
      console.error('Chronicle: Failed to update calendar event', err);
    }
  }

  /**
   * Push event delete from Calendaria to Chronicle.
   * @param {object} eventData
   * @private
   */
  async _onLocalEventDelete(eventData) {
    if (this._syncing) return;
    if (!game.user.isGM) return;

    const chronicleId = this._getChronicleEventId(eventData.id);
    if (!chronicleId) return;

    try {
      await this._api.delete(`/calendar/events/${chronicleId}`);
      this._removeEventMapping(eventData.id);
    } catch (err) {
      console.warn('Chronicle: Failed to delete calendar event', err);
    }
  }

  // --- Adapter Methods (abstract over Calendaria vs SimpleCalendar) ---

  /**
   * Set the date on the active Foundry calendar module.
   * @param {object} data - { year, month, day, hour, minute }
   * @private
   */
  async _setLocalDate(data) {
    this._syncing = true;
    try {
      if (this._calendarModule === 'calendaria') {
        if (game.Calendaria?.setDate) {
          await game.Calendaria.setDate({
            year: data.year,
            month: data.month,
            day: data.day,
          });
        }
      } else if (this._calendarModule === 'simple-calendar') {
        const sc = SimpleCalendar?.api;
        if (sc?.setDate) {
          sc.setDate({
            year: data.year,
            // SimpleCalendar months are 0-indexed.
            month: (data.month || 1) - 1,
            day: (data.day || 1) - 1,
            hour: data.hour || 0,
            minute: data.minute || 0,
            seconds: 0,
          });
        }
      }
    } catch (err) {
      console.error('Chronicle: Failed to set local calendar date', err);
    } finally {
      this._syncing = false;
    }
  }

  /**
   * Create a calendar event in the active Foundry calendar module.
   * @param {object} data - Chronicle event data.
   * @private
   */
  async _createLocalEvent(data) {
    if (this._calendarModule === 'calendaria') {
      // Calendaria events: use its API if available, otherwise store as flag.
      if (game.Calendaria?.createEvent) {
        const localEvent = await game.Calendaria.createEvent({
          name: data.name,
          year: data.year,
          month: data.month,
          day: data.day,
          description: data.description || '',
        });
        if (localEvent?.id) {
          this._storeEventMapping(localEvent.id, data.id);
        }
      } else {
        // Fallback: store Chronicle event reference for display in our UI.
        console.log('Chronicle: Calendaria createEvent API not available, event stored as reference');
      }
    } else if (this._calendarModule === 'simple-calendar') {
      // SimpleCalendar events are journal entries with note flags.
      const sc = SimpleCalendar?.api;
      if (sc?.addNote) {
        const note = await sc.addNote(
          data.name || 'Event',
          data.description || '',
          {
            year: data.year,
            month: (data.month || 1) - 1,
            day: (data.day || 1) - 1,
            hour: 0,
            minute: 0,
            seconds: 0,
          },
          {
            year: data.end_year || data.year,
            month: ((data.end_month || data.month) || 1) - 1,
            day: ((data.end_day || data.day) || 1) - 1,
            hour: 0,
            minute: 0,
            seconds: 0,
          },
          true, // allDay
          0,    // repeats (none)
        );
        if (note?.id) {
          this._storeEventMapping(note.id, data.id);
          // Store Chronicle event ID on the journal entry.
          const journal = game.journal.get(note.id);
          if (journal) {
            await journal.setFlag(FLAG_SCOPE, 'calendarEventId', data.id);
          }
        }
      }
    }
  }

  /**
   * Update a calendar event in the active Foundry calendar module.
   * @param {object} data - Chronicle event data with id.
   * @private
   */
  async _updateLocalEvent(data) {
    if (this._calendarModule === 'calendaria') {
      if (game.Calendaria?.updateEvent) {
        const localId = this._getLocalEventId(data.id);
        if (localId) {
          await game.Calendaria.updateEvent(localId, {
            name: data.name,
            year: data.year,
            month: data.month,
            day: data.day,
            description: data.description || '',
          });
        }
      }
    } else if (this._calendarModule === 'simple-calendar') {
      // SimpleCalendar notes are journal entries — update name/content.
      const localId = this._getLocalEventId(data.id);
      if (localId) {
        const journal = game.journal.get(localId);
        if (journal) {
          await journal.update({ name: data.name || journal.name });
        }
      }
    }
  }

  /**
   * Delete a calendar event from the active Foundry calendar module.
   * @param {object} data - { id: chronicleEventId }
   * @private
   */
  async _deleteLocalEvent(data) {
    const localId = this._getLocalEventId(data.id);
    if (!localId) return;

    if (this._calendarModule === 'calendaria') {
      if (game.Calendaria?.deleteEvent) {
        await game.Calendaria.deleteEvent(localId);
      }
    } else if (this._calendarModule === 'simple-calendar') {
      // Delete the journal entry that represents this note.
      const journal = game.journal.get(localId);
      if (journal) {
        await journal.delete();
      }
    }

    this._removeEventMapping(localId);
  }

  // --- Event Mapping Helpers ---
  // Stores bidirectional mapping between local (Foundry) event IDs and
  // Chronicle event IDs using the GM's user flags for persistence.

  /**
   * Store a mapping between a local event ID and a Chronicle event ID.
   * @param {string} localId
   * @param {string} chronicleId
   * @private
   */
  _storeEventMapping(localId, chronicleId) {
    const mappings = this._getEventMappings();
    mappings[localId] = chronicleId;
    mappings[`_rev_${chronicleId}`] = localId;
    game.user.setFlag(FLAG_SCOPE, 'calendarEventMappings', mappings);
  }

  /**
   * Get the Chronicle event ID for a local event.
   * @param {string} localId
   * @returns {string|null}
   * @private
   */
  _getChronicleEventId(localId) {
    return this._getEventMappings()[localId] || null;
  }

  /**
   * Get the local event ID for a Chronicle event.
   * @param {string} chronicleId
   * @returns {string|null}
   * @private
   */
  _getLocalEventId(chronicleId) {
    return this._getEventMappings()[`_rev_${chronicleId}`] || null;
  }

  /**
   * Remove an event mapping.
   * @param {string} localId
   * @private
   */
  _removeEventMapping(localId) {
    const mappings = this._getEventMappings();
    const chronicleId = mappings[localId];
    delete mappings[localId];
    if (chronicleId) {
      delete mappings[`_rev_${chronicleId}`];
    }
    game.user.setFlag(FLAG_SCOPE, 'calendarEventMappings', mappings);
  }

  /**
   * Get all event mappings from user flags.
   * @returns {object}
   * @private
   */
  _getEventMappings() {
    return game.user.getFlag(FLAG_SCOPE, 'calendarEventMappings') || {};
  }
}
