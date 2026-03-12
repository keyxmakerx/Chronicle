/**
 * Chronicle Sync - D&D 5e System Adapter
 *
 * Maps fields between Foundry VTT dnd5e Actor data and Chronicle entity
 * fields_data. Used by ActorSync when the matched system is "dnd5e".
 */

/**
 * The Chronicle system ID this adapter handles.
 * @type {string}
 */
export const systemId = 'dnd5e';

/**
 * The Chronicle entity type slug that represents characters for this system.
 * @type {string}
 */
export const characterTypeSlug = 'dnd5e-character';

/**
 * Extract Chronicle-compatible fields_data from a Foundry dnd5e Actor.
 * Reads from the Actor's system data paths and returns a flat object
 * matching the dnd5e-character entity preset field keys.
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
    str: abilities.str?.value ?? null,
    dex: abilities.dex?.value ?? null,
    con: abilities.con?.value ?? null,
    int: abilities.int?.value ?? null,
    wis: abilities.wis?.value ?? null,
    cha: abilities.cha?.value ?? null,
    hp_current: attrs.hp?.value ?? null,
    hp_max: attrs.hp?.max ?? null,
    ac: attrs.ac?.value ?? null,
    speed: attrs.movement?.walk ?? null,
    level: details.level ?? null,
    class: details.class ?? '',
    race: details.race ?? '',
    alignment: details.alignment ?? '',
    proficiency_bonus: attrs.prof ?? null,
  };
}

/**
 * Convert Chronicle entity fields_data into a Foundry Actor update object.
 * Returns a flat object with dot-notation keys suitable for
 * `actor.update({ ...result })`.
 *
 * Only includes fields that have non-null values in the entity data
 * to avoid overwriting Foundry-calculated values with null.
 *
 * @param {object} entity - Chronicle entity with fields_data.
 * @returns {object} Foundry Actor update data (dot-notation keys).
 */
export function fromChronicleFields(entity) {
  const f = entity.fields_data || {};
  const update = {};

  // Ability scores.
  if (f.str != null) update['system.abilities.str.value'] = Number(f.str);
  if (f.dex != null) update['system.abilities.dex.value'] = Number(f.dex);
  if (f.con != null) update['system.abilities.con.value'] = Number(f.con);
  if (f.int != null) update['system.abilities.int.value'] = Number(f.int);
  if (f.wis != null) update['system.abilities.wis.value'] = Number(f.wis);
  if (f.cha != null) update['system.abilities.cha.value'] = Number(f.cha);

  // HP.
  if (f.hp_current != null) update['system.attributes.hp.value'] = Number(f.hp_current);
  if (f.hp_max != null) update['system.attributes.hp.max'] = Number(f.hp_max);

  // Name (synced at document level, not system level).
  if (entity.name) update.name = entity.name;

  return update;
}
