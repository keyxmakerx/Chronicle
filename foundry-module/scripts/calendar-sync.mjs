/**
 * Chronicle Sync - Calendar/Calendaria Sync
 *
 * Optional integration between Chronicle's calendar system and the Calendaria
 * Foundry VTT module. When Calendaria is active, syncs date changes and
 * calendar events bidirectionally.
 *
 * When Calendaria is not active, this module is a no-op.
 */

import { getSetting } from './settings.mjs';

/**
 * CalendarSync handles calendar ↔ Calendaria synchronization.
 */
export class CalendarSync {
  constructor() {
    /** @type {import('./api-client.mjs').ChronicleAPI|null} */
    this._api = null;
    this._syncing = false;
    this._calendariaActive = false;
  }

  /**
   * Initialize calendar sync.
   * @param {import('./api-client.mjs').ChronicleAPI} api
   */
  async init(api) {
    this._api = api;

    if (!getSetting('syncCalendar')) return;

    // Check if Calendaria is active.
    this._calendariaActive = !!game.modules.get('calendaria')?.active;

    if (!this._calendariaActive) {
      console.log('Chronicle: Calendaria not active, calendar sync disabled');
      return;
    }

    // Hook into Calendaria's date change events.
    // Calendaria exposes hooks for date changes.
    Hooks.on('calendariaDateChange', this._onCalendariaDateChange.bind(this));
    Hooks.on('calendariaEventCreate', this._onCalendariaEventCreate.bind(this));

    console.log('Chronicle: Calendar sync initialized (Calendaria detected)');
  }

  /**
   * Handle incoming WebSocket messages for calendar events.
   * @param {object} msg
   */
  async onMessage(msg) {
    if (!getSetting('syncCalendar') || !this._calendariaActive) return;

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
   * Clean up on destroy.
   */
  destroy() {
    // Calendaria hooks auto-clean on module unload.
  }

  // --- Chronicle → Calendaria ---

  /**
   * Update Calendaria's date when Chronicle date advances.
   * @param {object} data - { year, month, day, hour, minute }
   * @private
   */
  async _onChronicaleDateAdvanced(data) {
    if (!data) return;
    this._syncing = true;
    try {
      // Use Calendaria's API to set the date.
      // The Calendaria module exposes `game.Calendaria?.setDate()`.
      if (game.Calendaria?.setDate) {
        await game.Calendaria.setDate({
          year: data.year,
          month: data.month,
          day: data.day,
        });
        console.log('Chronicle: Updated Calendaria date from Chronicle');
      }
    } finally {
      this._syncing = false;
    }
  }

  async _onChronicleEventCreated(data) {
    // Future: create Calendaria event from Chronicle event.
    console.log('Chronicle: Calendar event created', data);
  }

  async _onChronicleEventUpdated(data) {
    console.log('Chronicle: Calendar event updated', data);
  }

  async _onChronicleEventDeleted(data) {
    console.log('Chronicle: Calendar event deleted', data);
  }

  // --- Calendaria → Chronicle ---

  /**
   * Push date change from Calendaria to Chronicle.
   * @param {object} dateData
   * @private
   */
  async _onCalendariaDateChange(dateData) {
    if (this._syncing) return;
    if (!game.user.isGM) return;

    try {
      await this._api.post('/calendar/advance', {
        year: dateData.year,
        month: dateData.month,
        day: dateData.day,
      });
      console.log('Chronicle: Pushed date change to Chronicle');
    } catch (err) {
      console.error('Chronicle: Failed to push date to Chronicle', err);
    }
  }

  /**
   * Push new event from Calendaria to Chronicle.
   * @param {object} eventData
   * @private
   */
  async _onCalendariaEventCreate(eventData) {
    if (this._syncing) return;
    if (!game.user.isGM) return;

    try {
      await this._api.post('/calendar/events', {
        name: eventData.name || 'Untitled Event',
        year: eventData.year,
        month: eventData.month,
        day: eventData.day,
        description: eventData.description || '',
      });
      console.log('Chronicle: Pushed calendar event to Chronicle');
    } catch (err) {
      console.error('Chronicle: Failed to push calendar event', err);
    }
  }
}
