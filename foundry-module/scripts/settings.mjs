/**
 * Chronicle Sync - Module Settings Registration
 *
 * Registers all module settings in Foundry's settings API.
 * Settings are stored per-world and only editable by GMs.
 */

const MODULE_ID = 'chronicle-sync';

/**
 * Register all Chronicle Sync module settings.
 * Called once during the 'init' hook.
 */
export function registerSettings() {
  // Chronicle instance URL.
  game.settings.register(MODULE_ID, 'apiUrl', {
    name: game.i18n.localize('CHRONICLE.Settings.ApiUrl.Name'),
    hint: game.i18n.localize('CHRONICLE.Settings.ApiUrl.Hint'),
    scope: 'world',
    config: true,
    type: String,
    default: '',
    requiresReload: true,
  });

  // API key (hidden from non-GMs).
  game.settings.register(MODULE_ID, 'apiKey', {
    name: game.i18n.localize('CHRONICLE.Settings.ApiKey.Name'),
    hint: game.i18n.localize('CHRONICLE.Settings.ApiKey.Hint'),
    scope: 'world',
    config: true,
    type: String,
    default: '',
    requiresReload: true,
  });

  // Campaign UUID.
  game.settings.register(MODULE_ID, 'campaignId', {
    name: game.i18n.localize('CHRONICLE.Settings.CampaignId.Name'),
    hint: game.i18n.localize('CHRONICLE.Settings.CampaignId.Hint'),
    scope: 'world',
    config: true,
    type: String,
    default: '',
    requiresReload: true,
  });

  // Master sync toggle.
  game.settings.register(MODULE_ID, 'syncEnabled', {
    name: game.i18n.localize('CHRONICLE.Settings.SyncEnabled.Name'),
    hint: game.i18n.localize('CHRONICLE.Settings.SyncEnabled.Hint'),
    scope: 'world',
    config: true,
    type: Boolean,
    default: true,
  });

  // Per-feature toggles.
  game.settings.register(MODULE_ID, 'syncJournals', {
    name: game.i18n.localize('CHRONICLE.Settings.SyncJournals.Name'),
    hint: game.i18n.localize('CHRONICLE.Settings.SyncJournals.Hint'),
    scope: 'world',
    config: true,
    type: Boolean,
    default: true,
  });

  game.settings.register(MODULE_ID, 'syncMaps', {
    name: game.i18n.localize('CHRONICLE.Settings.SyncMaps.Name'),
    hint: game.i18n.localize('CHRONICLE.Settings.SyncMaps.Hint'),
    scope: 'world',
    config: true,
    type: Boolean,
    default: true,
  });

  game.settings.register(MODULE_ID, 'syncCalendar', {
    name: game.i18n.localize('CHRONICLE.Settings.SyncCalendar.Name'),
    hint: game.i18n.localize('CHRONICLE.Settings.SyncCalendar.Hint'),
    scope: 'world',
    config: true,
    type: Boolean,
    default: false,
  });

  // Internal: last sync timestamp (not shown in UI).
  game.settings.register(MODULE_ID, 'lastSyncTime', {
    scope: 'world',
    config: false,
    type: String,
    default: '',
  });
}

/**
 * Get a Chronicle Sync setting value.
 * @param {string} key - Setting key without module prefix.
 * @returns {*} The setting value.
 */
export function getSetting(key) {
  return game.settings.get(MODULE_ID, key);
}

/**
 * Set a Chronicle Sync setting value.
 * @param {string} key - Setting key without module prefix.
 * @param {*} value - The value to set.
 */
export async function setSetting(key, value) {
  await game.settings.set(MODULE_ID, key, value);
}

/**
 * Check if the module is properly configured (URL + key + campaign).
 * @returns {boolean}
 */
export function isConfigured() {
  const url = getSetting('apiUrl');
  const key = getSetting('apiKey');
  const campaign = getSetting('campaignId');
  return !!(url && key && campaign);
}
