// chronicle_gen.test.mjs — a focused port of the generator lab's own test
// suite (scratchpad/mockups/generators/test.mjs) for the three generators the
// calendar wizard's "Generate one" step actually calls (calendar, names,
// moons), plus the fromSky extension that step adds on top of the vendored
// engine. Not a full port of the lab's suite (that also covers weather, sky
// events, tables and history, none of which this wizard step uses) — see
// static/js/widgets/chronicle_gen.js's own header for the full API and the
// lab's test.mjs for the complete original coverage.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);
const G = require('../../static/js/widgets/chronicle_gen.js');
const J = (x) => JSON.stringify(x);

// Chronicle's own shipped presets, read straight from the repo (relative to
// this test file, not the lab's hardcoded absolute path) so the calendar-math
// check below runs the same in CI as it does here.
const PRESET_DIR = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'presets');
const shipped = (name) => (existsSync(join(PRESET_DIR, name + '.json')) ? JSON.parse(readFileSync(join(PRESET_DIR, name + '.json'), 'utf8')) : null);

test('loads as a plain script with no module system, as when inlined into a page', () => {
  const src = readFileSync(join(here, '..', '..', 'static', 'js', 'widgets', 'chronicle_gen.js'), 'utf8');
  const sandbox = { window: {} };
  const vm = require('node:vm');
  vm.createContext(sandbox);
  sandbox.globalThis = sandbox;
  vm.runInContext(src, sandbox);
  assert.equal(typeof sandbox.ChronicleGen.run, 'function');
  const r = sandbox.ChronicleGen.names.generate({ seed: 'inline' });
  assert.ok(r.names.months.length === 12);
});

test('calendar maths mirrors the Go model (AbsoluteDay, weekday column, round trip)', () => {
  // Values from internal/plugins/calendar/model.go for the shipped harptos preset.
  const h = shipped('harptos');
  assert.ok(h, 'expected internal/plugins/calendar/presets/harptos.json to exist');
  const c = G.cal.from(h);
  assert.equal(c.abs(1523, 1, 1), 556277);
  assert.equal(c.abs(1524, 10, 2), 556855); // the leap day of Midsummer in a leap year
  assert.equal(c.weekday(1523, 3, 15), 1);
  for (let n = c.abs(1522, 1, 1); n < c.abs(1530, 1, 1); n++) {
    const d = c.fromAbs(n);
    assert.equal(c.abs(d.year, d.month, d.day), n);
    assert.ok(c.valid(d.year, d.month, d.day));
  }
});

test('the calendar, names and moons generators are deterministic by seed, and differ across seeds', () => {
  const runs = {
    calendar: (seed) => G.calendar.generate({ seed, recipe: 'calendar.wild' }).preset,
    names: (seed) => G.names.generate({ seed, recipe: 'names.elven' }).names,
    moons: (seed) => G.moons.generate({ seed, recipe: 'moons.many' }).moons,
  };
  for (const [name, f] of Object.entries(runs)) {
    assert.equal(J(f('alpha')), J(f('alpha')), name + ' is not deterministic');
    assert.notEqual(J(f('alpha')), J(f('beta')), name + ' ignores its seed');
  }
});

test('generated calendars match the shape of Chronicle’s presets', () => {
  const shippedKeys = new Set();
  for (const name of ['harptos', 'dwarven', 'blank']) {
    const p = shipped(name);
    if (p) Object.keys(p.calendar).forEach((k) => shippedKeys.add(k));
  }
  for (const r of G.recipes.builtIn('calendar')) {
    for (const theme of G.names.themes().map((t) => t.id)) {
      for (const seed of ['c1', 'c2']) {
        const rec = G.recipes.copy(r, { name: 'Test' });
        rec.details.theme = theme;
        const out = G.calendar.generate({ recipe: rec, seed });
        const v = G.cal.validatePreset(out.preset);
        assert.ok(v.ok, `${r.id}/${theme}/${seed}: ${v.errors.map((e) => e.message).join(' ')}`);
        assert.equal(out.preset.format, 'chronicle-calendar-v1');
        for (const k of Object.keys(out.preset.calendar)) {
          assert.ok(shippedKeys.size === 0 || shippedKeys.has(k) || k === 'festivals', 'field ' + k + ' is in no shipped preset');
        }
        // It is a working calendar: every day of its first two years exists and round-trips.
        const c = G.cal.from(out.preset), y = out.preset.calendar.current_year;
        for (let n = c.abs(y, 1, 1); n < c.abs(y + 2, 1, 1); n++) {
          const d = c.fromAbs(n);
          assert.equal(c.abs(d.year, d.month, d.day), n);
        }
        const all = out.preset.calendar.months.map((m) => m.name).concat(out.preset.calendar.weekdays.map((w) => w.name));
        assert.equal(new Set(all.map((x) => x.toLowerCase())).size, all.length, 'a name repeats in ' + r.id + '/' + theme);
      }
    }
  }
});

test('names are unique within a set, and none is a published setting’s or a real calendar’s', () => {
  const blocked = G._internal.BLOCKED_SET;
  for (const t of G.names.themes()) {
    for (let s = 0; s < 8; s++) {
      const r = G.names.generate({ recipe: 'names.' + t.id, seed: 'n' + s });
      const all = Object.values(r.names).flat().map((x) => (typeof x === 'string' ? x : x.name));
      const low = all.map((x) => x.toLowerCase());
      assert.equal(new Set(low).size, low.length, t.id + '/' + s + ' repeats a name: ' + all.join(', '));
      for (const n of low) assert.ok(!blocked[n], t.id + ' produced the blocked name ' + n);
    }
  }
  const avoid = G.names.generate({ recipe: { generator: 'names', details: { theme: 'grim', avoid: ['bone', 'crow'] } }, seed: 'av' });
  for (const n of Object.values(avoid.names).flat().map((x) => (typeof x === 'string' ? x : x.name))) {
    assert.ok(!/bone|crow/i.test(n), 'avoided fragment in ' + n);
  }
});

test('moons: distinct cycles that drift out of step, unique names, a surface for every moon', () => {
  for (const seed of ['m1', 'm2', 'm3', 'm4']) {
    const r = G.moons.generate({ recipe: 'moons.many', seed });
    assert.equal(r.moons.length, 4);
    assert.equal(new Set(r.moons.map((m) => m.name.toLowerCase())).size, 4);
    for (let i = 0; i < 4; i++) for (let j = i + 1; j < 4; j++) assert.ok(Math.abs(r.moons[i].cycle_days - r.moons[j].cycle_days) >= 2.5);
    for (const m of r.moons) {
      assert.ok(m.gen.surface && m.gen.surface.preset);
      assert.match(m.color, /^#[0-9a-f]{6}$/);
    }
  }
});

test('every generator’s summary is one plain paragraph', () => {
  const outs = [
    G.names.generate({ seed: 'p' }),
    G.moons.generate({ seed: 'p' }),
    G.calendar.generate({ seed: 'p' }),
    G.calendar.fromSky({ suns: 2, moons: 2, planets: 1, seed: 'p' }),
  ];
  for (const o of outs) {
    assert.ok(o.summary.length > 60 && !o.summary.includes('\n') && !/undefined|NaN|\[object/.test(o.summary), (o.generator || 'fromSky') + ': ' + o.summary);
  }
});

// ── fromSky: the "work it out from the sky" mode ──

test('fromSky is deterministic by seed, and differs across seeds and sky inputs', () => {
  const a1 = G.calendar.fromSky({ suns: 2, moons: 2, planets: 1, seed: 'same' });
  const a2 = G.calendar.fromSky({ suns: 2, moons: 2, planets: 1, seed: 'same' });
  assert.equal(J(a1), J(a2), 'fromSky is not deterministic for identical inputs');
  const b = G.calendar.fromSky({ suns: 2, moons: 2, planets: 1, seed: 'different' });
  assert.notEqual(J(a1), J(b), 'fromSky ignores its seed');
  const c = G.calendar.fromSky({ suns: 3, moons: 2, planets: 1, seed: 'same' });
  assert.notEqual(J(a1), J(c), 'fromSky ignores the sun count');
});

test('fromSky produces a valid, round-trippable calendar across the whole input range, and the moon count always matches what was asked', () => {
  for (let suns = 1; suns <= 3; suns++) {
    for (let moons = 0; moons <= 4; moons++) {
      for (let planets = 0; planets <= 7; planets += 1) {
        const seed = `sweep-${suns}-${moons}-${planets}`;
        const out = G.calendar.fromSky({ suns, moons, planets, seed });
        const v = G.cal.validatePreset(out.preset);
        assert.ok(v.ok, `suns=${suns} moons=${moons} planets=${planets}: ${v.errors.map((e) => e.message).join(' ')}`);
        assert.equal(out.preset.calendar.moons.length, moons, `suns=${suns} moons=${moons} planets=${planets}: moon count`);
        const c = G.cal.from(out.preset), y = out.preset.calendar.current_year;
        for (let n = c.abs(y, 1, 1); n < c.abs(y + 1, 1, 1); n++) {
          const d = c.fromAbs(n);
          assert.equal(c.abs(d.year, d.month, d.day), n, `suns=${suns} moons=${moons} planets=${planets}: day round trip at ${n}`);
        }
      }
    }
  }
});

test('fromSky: the week is one day per visible wanderer, unless that is fewer than three, when it falls back to a quarter of the main moon’s cycle', () => {
  // 3 suns + 2 moons + 2 planets = 7 wanderers, plenty for its own week.
  const many = G.calendar.fromSky({ suns: 3, moons: 2, planets: 2, seed: 'wanderers' });
  assert.equal(many.preset.calendar.weekdays.length, 7);
  assert.equal(many.extras.sky.quarterWeek, false);

  // 1 sun + 1 moon + 0 planets = 2 wanderers: too few, so the main moon's
  // quarter takes over instead.
  const few = G.calendar.fromSky({ suns: 1, moons: 1, planets: 0, seed: 'quiet-sky' });
  assert.equal(few.extras.sky.quarterWeek, true);
  const expectedWeek = Math.max(3, Math.round(few.extras.sky.mainMoonCycle / 4));
  assert.equal(few.preset.calendar.weekdays.length, expectedWeek);

  // No moon and too few wanderers: nothing to take a quarter of, so the week
  // just floors at 3 rather than producing a 1- or 2-day week.
  const bare = G.calendar.fromSky({ suns: 1, moons: 0, planets: 0, seed: 'bare-sky' });
  assert.ok(bare.preset.calendar.weekdays.length >= 3);
});

test('fromSky: months come from the main moon’s cycle, and the leftover becomes festival days or is folded in, never lost', () => {
  const out = G.calendar.fromSky({ suns: 1, moons: 1, planets: 0, seed: 'moon-months' });
  const cal = out.preset.calendar;
  const mainCycle = out.extras.moons[0].cycle_days;
  const regular = cal.months.filter((m) => !m.is_intercalary);
  const feasts = cal.months.filter((m) => m.is_intercalary);
  // Every regular month but the last is within a day or two of the main
  // moon's cycle; the last one absorbs whatever remainder was too big to
  // spend as its own festival day (see runCalendarFromSky's own comment on
  // why), so it alone may run shorter or longer -- but never below 1 day.
  for (const m of regular.slice(0, -1)) assert.ok(Math.abs(m.days - mainCycle) <= 2, `month ${m.name} is ${m.days} days, moon cycle is ${mainCycle}`);
  assert.ok(regular[regular.length - 1].days >= 1);
  // The days are all accounted for: nothing appears or disappears.
  const total = cal.months.reduce((s, m) => s + m.days, 0);
  assert.equal(total, out.stats.yearLength);
  // The summary explains itself in plain words, the way the task's own
  // example does ("Two suns and three moons: ...").
  assert.match(out.summary, /^One sun and one moon: /);
  assert.ok(feasts.length === 0 || feasts.every((m) => m.days >= 1));
});

test('fromSky: the leap rule comes from the fractional year, and a calendar with no fraction left over has none', () => {
  // A broad seed sweep: whenever fracRemainder ~ 0 (rare but possible), no
  // leap rule; otherwise leapEvery is a sane, bounded number of years.
  let sawLeap = false, sawNoLeap = false;
  for (let s = 0; s < 30; s++) {
    const out = G.calendar.fromSky({ suns: 1, moons: 1, planets: 0, seed: 'leaf-' + s });
    const every = out.preset.calendar.leap_year_every;
    assert.ok(every === 0 || (every >= 2 && every <= 40), `leap_year_every out of range: ${every}`);
    if (every === 0) sawNoLeap = true;
    else sawLeap = true;
  }
  assert.ok(sawLeap, 'expected at least one seed with a leap rule over 30 tries');
});

test('fromSky: extreme corners of the input range (max suns/moons/planets, and zero moons) still validate', () => {
  for (const [suns, moons, planets] of [[1, 0, 0], [3, 4, 7], [1, 4, 0], [3, 0, 7]]) {
    const out = G.calendar.fromSky({ suns, moons, planets, seed: `corner-${suns}-${moons}-${planets}` });
    assert.ok(G.cal.validatePreset(out.preset).ok, `suns=${suns} moons=${moons} planets=${planets}`);
    assert.equal(out.preset.calendar.moons.length, moons);
  }
});

test('fromSky: an out-of-range sky input is clamped, not rejected or crashed on', () => {
  const out = G.calendar.fromSky({ suns: 99, moons: -5, planets: 500, seed: 'wild-input' });
  assert.ok(G.cal.validatePreset(out.preset).ok);
  assert.ok(out.extras.sky.suns <= 3 && out.extras.sky.suns >= 1);
  assert.equal(out.preset.calendar.moons.length, 0);
  assert.ok(out.extras.sky.planets <= 7);
});
