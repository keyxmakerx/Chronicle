#!/usr/bin/env node
// build-roll-table-library.mjs — rebuilds the rolling-table library
// (static/roll-tables/library.json) from pinned, openly licensed sources,
// so the licence check behind every table can be repeated.
//
//   node tools/build-roll-table-library.mjs            fetch the sources, write the library
//   node tools/build-roll-table-library.mjs --check    fetch, rebuild, fail if the file differs
//   node tools/build-roll-table-library.mjs --from DIR read sources already in DIR
//
// Each source is pinned by version and hash; a hash mismatch stops the build
// rather than letting changed text in unreviewed. A table is kept only when
// its licence is on ALLOWED. It also writes SOURCES.md, next to the library:
// each source, its licence and attribution, how the text was adapted, and
// every table.

import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const OUT = join(ROOT, 'static/roll-tables/library.json');
const DOC = join(ROOT, 'static/roll-tables/SOURCES.md');

const CC_BY = { licence: 'CC BY 4.0', licenceUrl: 'https://creativecommons.org/licenses/by/4.0/' };
// The only licences a library table may carry. Share-alike and non-commercial
// licences are deliberately absent: a campaign's copy of a table must stay free
// to change and keep.
const ALLOWED = { 'https://creativecommons.org/licenses/by/4.0': CC_BY };

const SRD_COMMIT = 'bf6ac2ae7e778397ca7326ca991706daf70c13c2';
const SOURCES = {
  classic: {
    url: 'https://registry.npmjs.org/@datasworn/ironsworn-classic/-/ironsworn-classic-0.0.10.tgz',
    integrity: 'sha512-yNoeKcL2qL6I3JH9XfCjMX1u7k83FXnyRzIxQLd32lO9If2Su4Oq6xe4FigQWHsZtkyuYWyj7/B3fMNW24H1xg==',
    file: 'package/json/classic.json',
  },
  delve: {
    url: 'https://registry.npmjs.org/@datasworn/ironsworn-classic-delve/-/ironsworn-classic-delve-0.0.10.tgz',
    integrity: 'sha512-Q7HScLdcjXHk/0FaleZd5F2RSicMnKfnb8HpulisdK+T7I80VIda330Ff8TWe3o4l/zCIF9+W0n94lM5M0f9WA==',
    file: 'package/json/delve.json',
  },
  srdBackgrounds: { url: srdURL('03 beyond1st'), sha256: '2a3046f27890720736d9cea284e1244d056a2a5c7a67ac141ab1956868d99dfe' },
  srdRunning: { url: srdURL('09 running'), sha256: 'e142ba4bf901e2c440069f841a2891355f009271d514b9370eb7c790570f1623' },
  srdMagic: { url: srdURL('10 magic items'), sha256: '87d4198ac30e262cbcdb0b9445a89c49e94cd055e3047cd5b2b2d574dc3a6981' },
};
function srdURL(name) {
  return 'https://raw.githubusercontent.com/BTMorton/dnd-5e-srd/' + SRD_COMMIT + '/markdown/' + encodeURIComponent(name) + '.md';
}

const CREDITS = {
  ironsworn: Object.assign({
    source: 'Ironsworn', author: 'Shawn Tomkin', url: 'https://www.ironswornrpg.com',
    notice: 'This work is based on Ironsworn (found at www.ironswornrpg.com), created by Shawn Tomkin, and licensed for our use under the Creative Commons Attribution 4.0 International license (creativecommons.org/licenses/by/4.0/).',
  }, CC_BY),
  delve: Object.assign({
    source: 'Ironsworn: Delve', author: 'Shawn Tomkin', url: 'https://www.ironswornrpg.com',
    notice: 'This work is based on Ironsworn: Delve (found at www.ironswornrpg.com), created by Shawn Tomkin, and licensed for our use under the Creative Commons Attribution 4.0 International license (creativecommons.org/licenses/by/4.0/).',
  }, CC_BY),
  srd: Object.assign({
    source: 'System Reference Document 5.1', author: 'Wizards of the Coast LLC', url: 'https://dnd.wizards.com/resources/systems-reference-document',
    notice: 'This work includes material taken from the System Reference Document 5.1 (“SRD 5.1”) by Wizards of the Coast LLC and available at https://dnd.wizards.com/resources/systems-reference-document. The SRD 5.1 is licensed under the Creative Commons Attribution 4.0 International License available at https://creativecommons.org/licenses/by/4.0/legalcode.',
  }, CC_BY),
};

// The limits a campaign's own tables are held to (internal/plugins/rolltables),
// so "Make a copy" of any library table can always be saved.
const MAX_NAME = 200, MAX_BRIEF = 1000, MAX_ENTRIES = 500, MAX_WEIGHT = 100;

function fail(msg) { console.error('build-roll-table-library: ' + msg); process.exit(1); }

/* ── Sources ── */
async function download(url) {
  const r = await fetch(url);
  if (!r.ok) fail('could not fetch ' + url + ' (' + r.status + ')');
  return Buffer.from(await r.arrayBuffer());
}
function checkHash(buf, algo, want, what) {
  const got = algo === 'sha512' ? 'sha512-' + createHash('sha512').update(buf).digest('base64') : createHash('sha256').update(buf).digest('hex');
  if (got !== want) fail(what + ' changed upstream (' + algo + ' ' + got + '); review the new text and its licence before updating the pin');
}
async function loadSources(fromDir) {
  const dir = fromDir || mkdtempSync(join(tmpdir(), 'roll-library-'));
  const out = {};
  for (const [key, s] of Object.entries(SOURCES)) {
    const name = key + (s.integrity ? '.tgz' : '.md');
    const path = join(dir, name);
    const buf = fromDir ? readFileSync(path) : await download(s.url);
    if (!fromDir) writeFileSync(path, buf);
    if (s.integrity) {
      checkHash(buf, 'sha512', s.integrity, key);
      const unpack = join(dir, key);
      mkdirSync(unpack, { recursive: true });
      execFileSync('tar', ['-xzf', path, '-C', unpack, s.file]);
      out[key] = JSON.parse(readFileSync(join(unpack, s.file), 'utf8'));
    } else {
      checkHash(buf, 'sha256', s.sha256, key);
      out[key] = buf.toString('utf8');
    }
  }
  return out;
}

/* ── Text clean-up ── */
// Datasworn writes cross-references as [Label](id:...) links and bold as __x__.
function plain(s) {
  return String(s || '')
    .replace(/\[([^\]]+)\]\((?:id|datasworn):[^)]*\)/g, '$1')
    .replace(/__([^_]+)__/g, '$1').replace(/\*\*([^*]+)\*\*/g, '$1').replace(/\*([^*]+)\*/g, '$1')
    .replace(/\s+/g, ' ').trim();
}
/* One row as an entry. A short row is all name; a long one keeps its first
   sentence as the name and the rest as the brief, the shape a DM reads at a glance.
   A leading "Label:" becomes the name outright. */
function entry(text, weight, where) {
  let t = plain(text), name = t, brief = '';
  const label = /^([^.:]{2,40}):\s+(.+)$/.exec(t);
  if (label) { name = label[1]; brief = label[2]; }
  else if (t.length > 120) {
    const m = /^(.{20,200}?[.!?])\s+(.+)$/.exec(t);
    if (m) { name = m[1]; brief = m[2]; }
  }
  if (/[{}]/.test(t)) fail(where + ': braces in source text would read as a table roll');
  if (name.length > MAX_NAME) fail(where + ': name longer than ' + MAX_NAME + ' characters');
  if (brief.length > MAX_BRIEF) fail(where + ': brief longer than ' + MAX_BRIEF + ' characters');
  const e = { name: name, weight: weight };
  if (brief) e.brief = brief;
  return e;
}
function table(id, name, group, credit, entries, extra) {
  if (!/^[a-z][a-z0-9-]{0,63}$/.test(id)) fail(id + ': not a valid table id');
  if (entries.length < 1 || entries.length > MAX_ENTRIES) fail(id + ': ' + entries.length + ' entries');
  entries.forEach(function (e, i) { if (!(e.weight >= 0 && e.weight <= MAX_WEIGHT)) fail(id + ' entry ' + (i + 1) + ': weight ' + e.weight); });
  return Object.assign({ id: id, name: name, group: group, credit: credit }, extra || {}, { entries: entries });
}

/* ── Ironsworn (Datasworn JSON) ── */
function datasworn(root, ruleset) {
  const byId = {};
  (function walk(o, lic) {
    if (Array.isArray(o)) { o.forEach(function (x) { walk(x, lic); }); return; }
    if (!o || typeof o !== 'object') return;
    const own = o._source && o._source.license;
    const l = own || lic;
    if (o._id && o.rows) byId[o._id] = { node: o, licence: l, page: (o._source && o._source.page) || null };
    for (const k of Object.keys(o)) if (k !== 'rows') walk(o[k], l);
  })(root.oracles, root.license);
  return function oracle(path) {
    const hit = byId[ruleset + '/oracles/' + path];
    if (!hit) fail('no oracle ' + ruleset + '/' + path);
    if (!ALLOWED[hit.licence]) fail(ruleset + '/' + path + ': licence ' + hit.licence + ' is not allowed');
    return hit;
  };
}
function rows(hit, where) {
  return hit.node.rows.map(function (r, i) {
    if (r.min == null || r.max == null) fail(where + ' row ' + (i + 1) + ' has no range');
    return entry(r.text, r.max - r.min + 1, where + ' row ' + (i + 1));
  });
}
function d100(hit) {
  const total = hit.node.rows.reduce(function (n, r) { return n + (r.max - r.min + 1); }, 0);
  return total === 100 ? { dice: 'd100' } : {};
}
function withPage(credit, hit) { return hit.page ? Object.assign({}, credit, { page: String(hit.page) }) : credit; }

function ironsworn(json) {
  const o = datasworn(json, 'classic'), C = CREDITS.ironsworn, G = 'Ironsworn oracles', out = [];
  function simple(id, path, name) { const h = o(path); out.push(table(id, name, G, withPage(C, h), rows(h, id), d100(h))); }
  simple('is-action', 'action_and_theme/action', 'Action');
  simple('is-theme', 'action_and_theme/theme', 'Theme');
  simple('is-character-role', 'character/role', 'Character role');
  simple('is-character-goal', 'character/goal', 'Character goal');
  simple('is-character-descriptor', 'character/descriptor', 'Character descriptor');
  simple('is-location', 'place/location', 'Location');
  simple('is-coastal-location', 'place/coastal_waters_location', 'Coastal waters location');
  simple('is-location-descriptor', 'place/descriptor', 'Location descriptor');
  simple('is-settlement-trouble', 'settlement/trouble', 'Settlement trouble');
  simple('is-combat-action', 'turning_point/combat_action', 'Combat action');
  simple('is-mystic-backlash', 'turning_point/mystic_backlash', 'Mystic backlash');
  simple('is-plot-twist', 'turning_point/major_plot_twist', 'Major plot twist');

  // Names. The two halves of the Ironlander list are one list of names.
  const a = o('name/ironlander/a'), b = o('name/ironlander/b');
  out.push(table('is-names-ironlander', 'Ironlander names', G, withPage(C, a), rows(a, 'ironlander a').concat(rows(b, 'ironlander b')).map(function (e) { e.weight = 1; return e; })));
  simple('is-names-elf', 'name/elf', 'Elf names');
  simple('is-names-giant', 'name/other/giants', 'Giant names');
  simple('is-names-varou', 'name/other/varou', 'Varou names');
  simple('is-names-troll', 'name/other/trolls', 'Troll names');

  // Settlement names: each row of the name oracle points at a list; here it rolls on it.
  const parts = [
    ['landscape_feature', 'is-settlement-landscape', 'Settlement name: landscape feature'],
    ['manmade_edifice', 'is-settlement-edifice', 'Settlement name: manmade edifice'],
    ['creature', 'is-settlement-creature', 'Settlement name: creature'],
    ['historical_event', 'is-settlement-event', 'Settlement name: historical event'],
    ['old_world_language', 'is-settlement-old-word', 'Settlement name: Old World word'],
    ['environmental_aspect', 'is-settlement-aspect', 'Settlement name: season or environment'],
    ['something_else', 'is-settlement-other', 'Settlement name: something else'],
  ];
  parts.forEach(function (p) { simple(p[1], 'settlement/name/' + p[0], p[2]); });
  const sn = o('settlement/name');
  if (sn.node.rows.length !== parts.length) fail('settlement name oracle changed shape');
  out.push(table('is-settlement-name', 'Settlement name', G, withPage(C, sn), sn.node.rows.map(function (r, i) {
    const e = entry(r.text, r.max - r.min + 1, 'settlement name row ' + (i + 1));
    return { name: '{' + parts[i][1] + '}', brief: plain(r.text), weight: e.weight };
  }), { dice: 'd100' }));
  const pre = o('settlement/quick_name/prefix'), suf = o('settlement/quick_name/suffix');
  simple('is-settlement-prefix', 'settlement/quick_name/prefix', 'Quick settlement name: prefix');
  simple('is-settlement-suffix', 'settlement/quick_name/suffix', 'Quick settlement name: suffix');
  out.push(table('is-settlement-quick-name', 'Quick settlement name', G, withPage(C, pre),
    [{ name: '{is-settlement-prefix}{is-settlement-suffix}', weight: 1 }]));
  void suf;
  return out;
}

function delve(json) {
  const o = datasworn(json, 'delve'), C = CREDITS.delve, G = 'Ironsworn: Delve', out = [];
  function simple(id, path, name) { const h = o(path); out.push(table(id, name, G, withPage(C, h), rows(h, id), d100(h))); return h; }
  simple('delve-activity', 'character/activity', 'Character activity');
  simple('delve-disposition', 'character/disposition', 'Character disposition');
  simple('delve-combat-method', 'combat_event/method', 'Combat event: method');
  simple('delve-combat-target', 'combat_event/target', 'Combat event: target');
  simple('delve-feature-aspect', 'feature/aspect', 'Feature aspect');
  simple('delve-feature-focus', 'feature/focus', 'Feature focus');
  simple('delve-monster-size', 'monstrosity/size', 'Monstrosity: size');
  simple('delve-monster-form', 'monstrosity/primary_form', 'Monstrosity: primary form');
  simple('delve-monster-traits', 'monstrosity/characteristics', 'Monstrosity: characteristics');
  simple('delve-monster-abilities', 'monstrosity/abilities', 'Monstrosity: abilities');
  simple('delve-opportunity', 'moves/find_an_opportunity', 'Find an opportunity');
  simple('delve-danger', 'moves/reveal_a_danger', 'Reveal a danger');
  simple('delve-site-theme', 'site_nature/theme', 'Site theme');
  simple('delve-site-domain', 'site_nature/domain', 'Site domain');
  simple('delve-trap-event', 'trap/trap', 'Trap event');
  simple('delve-trap-component', 'trap/component', 'Trap component');

  // Site names: the format oracle's rows are templates over the name parts,
  // and each place row rolls on that domain's list of place words.
  simple('delve-site-description', 'site_name/description', 'Site name: description');
  simple('delve-site-detail', 'site_name/detail', 'Site name: detail');
  simple('delve-site-namesake', 'site_name/namesake', 'Site name: namesake');
  const domains = ['barrow', 'cavern', 'frozen_cavern', 'icereach', 'mine', 'pass', 'ruin', 'sea_cave', 'shadowfen', 'stronghold', 'tanglewood', 'underkeep'];
  domains.forEach(function (d) {
    const h = o('site_name/place/' + d), label = plain(h.node.name);
    out.push(table('delve-place-' + d.replace(/_/g, '-'), 'Site name: ' + label.toLowerCase() + ' place', G, withPage(C, h), rows(h, d), d100(h)));
  });
  const place = o('site_name/place');
  if (place.node.rows.length !== domains.length) fail('site name place oracle changed shape');
  out.push(table('delve-site-place', 'Site name: place', G, withPage(C, place), place.node.rows.map(function (r, i) {
    return { name: '{delve-place-' + domains[i].replace(/_/g, '-') + '}', brief: plain(r.text), weight: r.max - r.min + 1 };
  }), { dice: 'd100' }));
  const PARTS = { description: 'delve-site-description', place: 'delve-site-place', detail: 'delve-site-detail', namesake: 'delve-site-namesake' };
  const fmt = o('site_name/format');
  out.push(table('delve-site-name', 'Site name', G, withPage(C, fmt), fmt.node.rows.map(function (r, i) {
    const tpl = String(r.text).replace(/\[([^\]]+)\]\(id:delve\/oracles\/site_name\/(\w+)\)/g, function (_, label, part) {
      if (!PARTS[part]) fail('site name format row ' + (i + 1) + ': unknown part ' + part);
      return '{' + PARTS[part] + '}' + (/'s$/.test(label) ? '’s' : '');
    });
    if (/[[\]()]/.test(tpl)) fail('site name format row ' + (i + 1) + ' kept a link');
    return { name: tpl, weight: r.max - r.min + 1 };
  }), { dice: 'd100' }));

  // Threats: the category row rolls on its threat's list.
  const threats = ['burgeoning_conflict', 'cursed_site', 'environmental_calamity', 'malignant_plague', 'rampaging_creature', 'ravaging_horde', 'scheming_leader', 'power_hungry_mystic', 'zealous_cult'];
  threats.forEach(function (t) {
    const h = o('threat/' + t);
    out.push(table('delve-threat-' + t.replace(/_/g, '-'), 'Threat: ' + plain(h.node.name).toLowerCase(), G, withPage(C, h), rows(h, t), d100(h)));
  });
  const cat = o('threat/category');
  out.push(table('delve-threat', 'Threat', G, withPage(C, cat), cat.node.rows.map(function (r, i) {
    if (i < threats.length) return { name: plain(r.text), brief: '{delve-threat-' + threats[i].replace(/_/g, '-') + '}', weight: r.max - r.min + 1 };
    return entry(r.text, r.max - r.min + 1, 'threat category row ' + (i + 1));
  }), { dice: 'd100' }));
  return out;
}

/* ── SRD 5.1 (markdown) ── */
// The rows of the first table after a heading, as [range, text].
function mdTable(md, heading, where) {
  const lines = md.split('\n');
  let i = lines.findIndex(function (l) { return l.replace(/^#+\s*/, '').trim() === heading && /^#/.test(l); });
  if (i < 0) fail(where + ': heading "' + heading + '" not found');
  while (i < lines.length && !/^\|\s*d\d+/.test(lines[i])) i++;
  const head = lines[i];
  if (!head) fail(where + ': no table under "' + heading + '"');
  const out = [];
  for (i += 2; i < lines.length && /^\|/.test(lines[i]); i++) {
    const cells = lines[i].split('|').slice(1, -1).map(function (c) { return c.trim(); });
    out.push(cells);
  }
  return { die: /d(\d+)/.exec(head)[1] | 0, rows: out };
}
function rangeWeight(cell, where) {
  const m = /^(\d+)(?:\s*[–-]\s*(\d+))?$/.exec(cell);
  if (!m) fail(where + ': range "' + cell + '"');
  const lo = +m[1] === 0 ? 100 : +m[1], hi = m[2] == null ? lo : (+m[2] === 0 ? 100 : +m[2]);
  return hi - lo + 1;
}
function srd(src) {
  const C = CREDITS.srd, G = 'D&D 5e SRD', out = [];
  function add(id, name, md, heading, opts) {
    opts = opts || {};
    const t = mdTable(md, heading, id);
    if (opts.nth) {
      // Several tables share one heading (the Acolyte's characteristics); take the nth.
      const lines = md.split('\n'), start = lines.findIndex(function (l) { return /^#/.test(l) && l.replace(/^#+\s*/, '').trim() === heading; });
      let n = -1, i = start;
      for (; i < lines.length; i++) if (/^\|\s*d\d+/.test(lines[i]) && ++n === opts.nth) break;
      const sub = mdTable(['# x'].concat(lines.slice(i)).join('\n'), 'x', id);
      t.die = sub.die; t.rows = sub.rows;
    }
    let sum = 0;
    const entries = t.rows.map(function (cells, j) {
      const w = rangeWeight(cells[0], id + ' row ' + (j + 1));
      sum += w;
      return entry(cells[1], w, id + ' row ' + (j + 1));
    });
    if (sum !== t.die) fail(id + ': ranges cover ' + sum + ' of d' + t.die);
    out.push(table(id, name, G, Object.assign({}, C, opts.page ? { page: opts.page } : {}), entries, { dice: 'd' + t.die }));
  }
  add('srd-madness-short', 'Short-term madness', src.srdRunning, 'Short-Term Madness');
  add('srd-madness-long', 'Long-term madness', src.srdRunning, 'Long-Term Madness');
  add('srd-madness-indefinite', 'Indefinite madness', src.srdRunning, 'Indefinite Madness');
  add('srd-acolyte-trait', 'Acolyte: personality trait', src.srdBackgrounds, 'Suggested Characteristics', { nth: 0 });
  add('srd-acolyte-ideal', 'Acolyte: ideal', src.srdBackgrounds, 'Suggested Characteristics', { nth: 1 });
  add('srd-acolyte-bond', 'Acolyte: bond', src.srdBackgrounds, 'Suggested Characteristics', { nth: 2 });
  add('srd-acolyte-flaw', 'Acolyte: flaw', src.srdBackgrounds, 'Suggested Characteristics', { nth: 3 });
  add('srd-efreeti-bottle', 'Efreeti bottle', src.srdMagic, 'Efreeti Bottle');
  add('srd-sentient-communication', 'Sentient item: communication', src.srdMagic, 'Communication');
  add('srd-sentient-senses', 'Sentient item: senses', src.srdMagic, 'Senses');
  add('srd-sentient-alignment', 'Sentient item: alignment', src.srdMagic, 'Alignment');
  add('srd-sentient-purpose', 'Sentient item: special purpose', src.srdMagic, 'Special Purpose');
  return out;
}

/* ── SOURCES.md ── */
// Written from the same declarations as the library, so the two can't drift.
function sourcesDoc(tables) {
  const pin = function (k) { const x = SOURCES[k]; return '`' + x.url + '` (' + (x.integrity || 'sha256 ' + x.sha256) + ')'; };
  const L = [];
  L.push('# Rolling table library: sources', '');
  L.push('`library.json` holds the rolling tables Chronicle ships beyond its own starter tables. Every one comes from an openly licensed source, keeps its credit line (shown in the table picker and editor, and carried by "Make a copy"), and is listed below.', '');
  L.push('Do not edit `library.json` by hand. `node tools/build-roll-table-library.mjs` rebuilds it and this file from the pinned sources; `--check` fails when either is out of date. The build stops when a pinned source changes (its hash no longer matches) or a table\'s licence is not allowed, so a licence check is a re-run of the build.', '');
  L.push('## Licences a table may carry', '');
  L.push('- **CC BY 4.0** (https://creativecommons.org/licenses/by/4.0/): reuse, change and share with credit. Every library table today.', '');
  L.push('Share-alike and non-commercial licences are not allowed: a campaign\'s copy must stay free to change. CC0, OGL 1.0a and ORC content could be added, but each needs its own notice handling (the OGL requires its full text and a Section 15 notice) and an entry in the build\'s allow-list first.', '');
  L.push('## Sources', '');
  L.push('### Ironsworn and Ironsworn: Delve', '');
  L.push('- By Shawn Tomkin, https://www.ironswornrpg.com. Licence: CC BY 4.0.');
  L.push('- Text from the Datasworn transcriptions (https://github.com/rsek/datasworn), pinned: ' + pin('classic') + ' and ' + pin('delve') + '. Datasworn records a licence on each oracle; the build keeps only those it marks CC BY 4.0, inheriting a collection\'s licence for its columns. Both packages also hold CC BY-NC-SA material outside the oracles; none of it is used.');
  L.push('- Notices: "' + CREDITS.ironsworn.notice + '" and "' + CREDITS.delve.notice + '"');
  L.push('- Adapted: cross-reference links reduced to their words; rows over 120 characters split into a first-sentence name and a brief; d100 ranges kept as weights; the two Ironlander name tables joined into one list; the Settlement Name, Site Name, Site Place and Threat oracles turned into tables that roll on their part lists.');
  L.push('- Left out: oracles tied to Ironsworn\'s moves and rules (Ask the Oracle, Pay the Price, Endure Harm and Stress, Delve the Depths, Advance a Threat, Challenge Rank, the alternate Reveal a Danger) and the Ironlands Region list, which names the default setting\'s places.', '');
  L.push('### System Reference Document 5.1', '');
  L.push('- By Wizards of the Coast LLC, https://dnd.wizards.com/resources/systems-reference-document. Licence: CC BY 4.0.');
  L.push('- Text from a markdown transcription of the SRD 5.1 (https://github.com/BTMorton/dnd-5e-srd at `' + SRD_COMMIT + '`), pinned: ' + ['srdBackgrounds', 'srdRunning', 'srdMagic'].map(pin).join(', ') + '. Wizards of the Coast released the SRD 5.1 under CC BY 4.0, and that is the licence used here. This copy has not yet been compared word for word against the official CC BY PDF, which this build could not reach.');
  L.push('- Notice: "' + CREDITS.srd.notice + '"');
  L.push('- Adapted: a leading "Label:" becomes the line\'s name; rows over 120 characters split as above; italics dropped.');
  L.push('- Left out: Bag of Beans, Robe of Useful Items and Wand of Wonder, because the transcription merges two rows in each (the build checks every table\'s ranges cover its die exactly). They can come back from a cleaner copy.', '');
  L.push('## Tables', '');
  L.push('| Table | Name | Source | Die | Lines |', '|---|---|---|---|---|');
  tables.forEach(function (t) {
    const c = t.credit;
    L.push('| `' + t.id + '` | ' + t.name + ' | ' + c.source + (c.page ? ', p. ' + c.page : '') + ' (' + c.licence + ') | ' + (t.dice || '') + ' | ' + t.entries.length + ' |');
  });
  return L.join('\n') + '\n';
}

/* ── Build ── */
async function main() {
  const args = process.argv.slice(2);
  const fromIdx = args.indexOf('--from');
  const src = await loadSources(fromIdx >= 0 ? args[fromIdx + 1] : null);
  const tables = [].concat(ironsworn(src.classic), delve(src.delve), srd(src));
  const ids = {};
  tables.forEach(function (t) { if (ids[t.id]) fail('duplicate id ' + t.id); ids[t.id] = 1; });
  const lib = { format: 'chronicle-roll-tables', version: 1, id: 'library', name: 'Library tables', builtIn: true, tables: tables };
  // One table per line keeps a rebuild's diff readable.
  const head = JSON.stringify(Object.assign({}, lib, { tables: [] }));
  const text = head.slice(0, -2) + '\n' + tables.map(function (t) { return JSON.stringify(t); }).join(',\n') + '\n]}\n';
  JSON.parse(text);
  const doc = sourcesDoc(tables);
  if (args.includes('--check')) {
    const read = function (f) { return existsSync(f) ? readFileSync(f, 'utf8') : ''; };
    if (read(OUT) !== text || read(DOC) !== doc) fail('library.json or SOURCES.md is out of date with its sources; run tools/build-roll-table-library.mjs');
    console.log('library.json and SOURCES.md match their sources: ' + tables.length + ' tables');
    return;
  }
  mkdirSync(dirname(OUT), { recursive: true });
  writeFileSync(OUT, text);
  writeFileSync(DOC, doc);
  console.log('wrote ' + tables.length + ' tables to ' + OUT + ' and their sources to ' + DOC);
}
main().catch(function (e) { fail(e.stack || String(e)); });
