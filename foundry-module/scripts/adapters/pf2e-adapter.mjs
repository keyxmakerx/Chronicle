/**
 * Chronicle Sync - Pathfinder 2e System Adapter
 *
 * Maps fields between Foundry VTT pf2e Actor data and Chronicle entity
 * fields_data. Used by ActorSync when the matched system is "pathfinder2e".
 */

/**
 * The Chronicle system ID this adapter handles.
 * @type {string}
 */
export const systemId = 'pathfinder2e';

/**
 * The Chronicle entity type slug that represents characters for this system.
 * @type {string}
 */
export const characterTypeSlug = 'pf2e-character';

/**
 * Extract Chronicle-compatible fields_data from a Foundry pf2e Actor.
 * PF2e stores ability modifiers rather than raw scores. It uses
 * `ancestry` instead of `race` and has `heritage` as a separate field.
 *
 * @param {Actor} actor - Foundry Actor document (type: "character").
 * @returns {object} Chronicle fields_data object.
 */
export function toChronicleFields(actor) {
  const sys = actor.system || {};
  const abilities = sys.abilities || {};
  const attrs = sys.attributes || {};
  const details = sys.details || {};

  return {
    str_mod: abilities.str?.mod ?? null,
    dex_mod: abilities.dex?.mod ?? null,
    con_mod: abilities.con?.mod ?? null,
    int_mod: abilities.int?.mod ?? null,
    wis_mod: abilities.wis?.mod ?? null,
    cha_mod: abilities.cha?.mod ?? null,
    hp_current: attrs.hp?.value ?? null,
    hp_max: attrs.hp?.max ?? null,
    ac: attrs.ac?.value ?? null,
    perception: attrs.perception?.value ?? null,
    speed: attrs.speed?.value ?? null,
    level: details.level?.value ?? null,
    class: details.class?.name ?? '',
    ancestry: details.ancestry?.name ?? '',
    heritage: details.heritage?.name ?? '',
  };
}

/**
 * Convert Chronicle entity fields_data into a Foundry Actor update object.
 * Only includes fields that have non-null values.
 *
 * Note: PF2e calculates many values from items and rules. Only HP and
 * name are safe to push back. Ability mods and AC are derived values
 * in the PF2e system and should not be overwritten.
 *
 * @param {object} entity - Chronicle entity with fields_data.
 * @returns {object} Foundry Actor update data (dot-notation keys).
 */
export function fromChronicleFields(entity) {
  const f = entity.fields_data || {};
  const update = {};

  // HP is the primary safe-to-update field from Chronicle.
  if (f.hp_current != null) update['system.attributes.hp.value'] = Number(f.hp_current);
  if (f.hp_max != null) update['system.attributes.hp.max'] = Number(f.hp_max);

  // Name synced at document level.
  if (entity.name) update.name = entity.name;

  return update;
}
