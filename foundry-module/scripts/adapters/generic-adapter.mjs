/**
 * Chronicle Sync - Generic System Adapter
 *
 * A data-driven adapter that reads field definitions from the Chronicle
 * /systems/:id/character-fields API. This allows any game system — including
 * custom-uploaded ones — to sync character fields between Chronicle and Foundry
 * without a hand-written adapter, as long as the system manifest includes
 * foundry_path annotations on its character fields.
 *
 * Field definitions specify:
 *   - key:             Chronicle field key (e.g. "hp_current")
 *   - foundry_path:    dot-notation path on actor.system (e.g. "system.attributes.hp.value")
 *   - foundry_writable: whether Chronicle may write back to this Foundry path (default true)
 *   - type:            field type ("number", "string", etc.) for casting
 */

/**
 * Create a generic adapter instance by fetching field definitions from the API.
 *
 * @param {import('../api-client.mjs').ChronicleAPI} api - Chronicle API client.
 * @param {string} chronicleSystemId - The Chronicle system ID (e.g. "dnd5e").
 * @returns {Promise<{systemId: string, characterTypeSlug: string, toChronicleFields: function, fromChronicleFields: function}|null>}
 */
export async function createGenericAdapter(api, chronicleSystemId) {
  let fieldDefs;
  try {
    const resp = await api.get(`/systems/${chronicleSystemId}/character-fields`);
    if (!resp || !resp.fields || resp.fields.length === 0) {
      console.warn(`Chronicle: Generic adapter — no character fields for system "${chronicleSystemId}"`);
      return null;
    }
    fieldDefs = resp;
  } catch (err) {
    console.error(`Chronicle: Generic adapter — failed to load field defs for "${chronicleSystemId}"`, err);
    return null;
  }

  // Only include fields that have a foundry_path annotation.
  const mappedFields = fieldDefs.fields.filter((f) => f.foundry_path);
  if (mappedFields.length === 0) {
    console.warn(`Chronicle: Generic adapter — no fields with foundry_path for "${chronicleSystemId}"`);
    return null;
  }

  const writableFields = mappedFields.filter((f) => f.foundry_writable !== false);

  console.log(
    `Chronicle: Generic adapter loaded for "${chronicleSystemId}" — ` +
    `${mappedFields.length} fields mapped, ${writableFields.length} writable`
  );

  return {
    /** Chronicle system ID. */
    systemId: chronicleSystemId,

    /** Character entity type slug from the manifest. */
    characterTypeSlug: fieldDefs.preset_slug || `${chronicleSystemId}-character`,

    /**
     * Extract Chronicle-compatible fields_data from a Foundry Actor.
     * Reads each mapped field from the actor using its foundry_path.
     *
     * @param {Actor} actor - Foundry Actor document.
     * @returns {object} Chronicle fields_data object.
     */
    toChronicleFields(actor) {
      const result = {};
      for (const field of mappedFields) {
        const value = _getNestedValue(actor, field.foundry_path);
        result[field.key] = value ?? null;
      }
      return result;
    },

    /**
     * Convert Chronicle entity fields_data into a Foundry Actor update.
     * Only writes to fields marked as foundry_writable (or defaulting to true).
     * Returns dot-notation keys for actor.update().
     *
     * @param {object} entity - Chronicle entity with fields_data.
     * @returns {object} Foundry Actor update data.
     */
    fromChronicleFields(entity) {
      const f = entity.fields_data || {};
      const update = {};

      for (const field of writableFields) {
        const value = f[field.key];
        if (value == null) continue;

        // Cast to appropriate type.
        if (field.type === 'number') {
          update[field.foundry_path] = Number(value);
        } else {
          update[field.foundry_path] = value;
        }
      }

      // Name is synced at document level.
      if (entity.name) update.name = entity.name;

      return update;
    },
  };
}

/**
 * Read a nested value from an object using dot-notation path.
 * Supports both nested objects and Foundry's system data.
 * e.g., _getNestedValue(actor, "system.abilities.str.value")
 *
 * @param {object} obj
 * @param {string} path
 * @returns {*}
 */
function _getNestedValue(obj, path) {
  const keys = path.split('.');
  let current = obj;
  for (const key of keys) {
    if (current == null || typeof current !== 'object') return undefined;
    current = current[key];
  }
  return current;
}
