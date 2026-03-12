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

  // Character sync toggle (requires matching game system).
  game.settings.register(MODULE_ID, 'syncCharacters', {
    name: game.i18n.localize('CHRONICLE.Settings.SyncCharacters.Name'),
    hint: game.i18n.localize('CHRONICLE.Settings.SyncCharacters.Hint'),
    scope: 'world',
    config: true,
    type: Boolean,
    default: false,
  });

  // Internal: detected Chronicle system ID matched from Foundry's game.system.id.
  game.settings.register(MODULE_ID, 'detectedSystem', {
    scope: 'world',
    config: false,
    type: String,
    default: '',
  });

  // Internal: last sync timestamp (not shown in UI).
  game.settings.register(MODULE_ID, 'lastSyncTime', {
    scope: 'world',
    config: false,
    type: String,
    default: '',
  });

  // Internal: per-type and per-entity sync exclusions (not shown in settings UI).
  // Stored as JSON: { excludedTypes: [typeId, ...], excludedEntities: ["entityId", ...] }
  game.settings.register(MODULE_ID, 'syncExclusions', {
    scope: 'world',
    config: false,
    type: String,
    default: '{"excludedTypes":[],"excludedEntities":[]}',
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
 * Get sync exclusions (excluded types and entities).
 * @returns {{ excludedTypes: number[], excludedEntities: string[] }}
 */
export function getSyncExclusions() {
  try {
    return JSON.parse(getSetting('syncExclusions'));
  } catch {
    return { excludedTypes: [], excludedEntities: [] };
  }
}

/**
 * Save sync exclusions.
 * @param {{ excludedTypes: number[], excludedEntities: string[] }} exclusions
 */
export async function setSyncExclusions(exclusions) {
  await setSetting('syncExclusions', JSON.stringify(exclusions));
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

/**
 * Mask the API key input in the module settings dialog.
 * Foundry doesn't have a native password input type for settings,
 * so we convert it after the settings form renders.
 */
Hooks.on('renderSettingsConfig', (app, html) => {
  const keyInput = html[0]?.querySelector
    ? html[0].querySelector(`input[name="${MODULE_ID}.apiKey"]`)
    : html.find(`input[name="${MODULE_ID}.apiKey"]`)[0];
  if (keyInput) {
    keyInput.type = 'password';
    keyInput.autocomplete = 'off';
  }
});
