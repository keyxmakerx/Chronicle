// family_tree.test.mjs — pins the "Family tree" block's pure parts: motion,
// circles (initials, hue, safe addresses), relation words, what a viewer in
// the Director's "view as player" preview is left with, the layout of rows and
// the "more" bubble, and where the hover card may sit.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'entities', 'static', 'js', 'family_tree.js'), 'utf8');
const sandbox = { module: { exports: {} } };
vm.runInNewContext(src, sandbox);
const F = sandbox.module.exports;
// Arrays built inside the vm context have another realm's prototype, so compare as JSON.
const same = (a, b, msg) => assert.equal(JSON.stringify(a), JSON.stringify(b), msg);

const person = (id, name, parents = [], partners = [], extra = {}) => ({ id, name, sub: 'Character', url: '/campaigns/c/entities/' + id, parents, partners, read: true, ...extra });
const par = (id, kind = 'birth', dm = false) => ({ id, kind, dm });

// gran > mum, dad > maud (page) > tam; Corwin is a hidden half-brother; Kessa a ward.
const data = {
  page: 'maud',
  people: [
    person('maud', 'Maud Fenwick', [par('mum'), par('dad')]),
    person('mum', 'Ione Fenwick', [par('gran')], ['dad']),
    person('dad', 'Corvin Fenwick', [], ['mum']),
    person('gran', 'Edda Fenwick'),
    person('tam', 'Tam Ash', [par('maud')]),
    person('half', 'Corwin Mere', [par('dad')], [], { dm: true }),
    person('kessa', 'Kessa', [par('maud', 'guardian', true)], [], { read: false })
  ]
};

test('motionOf: reduced motion is always off; otherwise the page attribute decides', () => {
  const cases = [
    { attr: null, reduced: false, want: 'full' },
    { attr: 'calm', reduced: false, want: 'calm' },
    { attr: 'off', reduced: false, want: 'off' },
    { attr: 'calm', reduced: true, want: 'off' },
    { attr: 'bogus', reduced: false, want: 'full' }
  ];
  for (const c of cases) assert.equal(F.motionOf(c.attr, c.reduced), c.want, JSON.stringify(c));
});

test('circles: initials from first and last word, a stable hue, only safe addresses', () => {
  const initials = [['Maud Fenwick', 'MF'], ['Lady Seren Valcrest', 'LV'], ['Kessa', 'K'], ['  ', '?'], ['élodie roux', 'ÉR']];
  for (const [n, want] of initials) assert.equal(F.initialsOf(n), want, n);
  assert.equal(F.hueOf('abc'), F.hueOf('abc'));
  assert.ok(F.hueOf('abc') >= 0 && F.hueOf('abc') < 360);
  const urls = [['/media/x.png?sig=1', '/media/x.png?sig=1'], ['https://a.test/x.png', 'https://a.test/x.png'], ['//evil.test/x', ''], ['javascript:alert(1)', ''], [null, '']];
  for (const [u, want] of urls) assert.equal(F.safeUrl(u), want, String(u));
});

test('relLabel and rowLabel: neutral words, generations, kinds', () => {
  const cases = [
    ['up', 1, 'birth', 'Parent'], ['up', 2, 'birth', 'Grandparent'], ['up', 3, 'birth', 'Great-grandparent'], ['up', 4, 'birth', 'Great-great-grandparent'],
    ['up', 1, 'adopted', 'Adoptive parent'], ['up', 1, 'guardian', 'Guardian'],
    ['down', 1, 'birth', 'Child'], ['down', 2, 'birth', 'Grandchild'], ['down', 3, 'birth', 'Great-grandchild'],
    ['down', 1, 'adopted', 'Adopted child'], ['down', 1, 'guardian', 'Ward'], ['partner', 1, '', 'Partner'], ['sib', 1, '', 'Sibling']
  ];
  for (const [d, g, k, want] of cases) assert.equal(F.relLabel(d, g, k), want, `${d} ${g} ${k}`);
  assert.equal(F.rowLabel('up', 1), 'Parents');
  assert.equal(F.rowLabel('up', 3), 'Great-grandparents');
  assert.equal(F.rowLabel('down', 2), 'grandchildren');
});

test('the Director previewing as a player loses the hidden relative and the DM-only link, with no gap', () => {
  const m = F.buildModel(data);
  const cases = [
    { dm: true, kids: ['tam', 'kessa'], sibs: ['half'], reach: 6, parents: ['mum', 'dad'] },
    { dm: false, kids: ['tam'], sibs: [], reach: 4, parents: ['mum', 'dad'] }
  ];
  for (const c of cases) {
    same(F.kidsOf(m, c.dm, 'maud'), c.kids, 'kids dm=' + c.dm);
    same(F.sibsOf(m, c.dm, 'maud'), c.sibs, 'sibs dm=' + c.dm);
    assert.equal(F.reachable(m, c.dm, 'maud'), c.reach, 'reach dm=' + c.dm);
    same(F.parentsOf(m, c.dm, 'maud').map((e) => e.id), c.parents);
  }
  const lay = F.layoutTree(m, { dm: false, centre: 'maud', phone: false, width: 900, sib: true });
  const ids = Object.keys(lay.dots);
  assert.ok(!ids.includes('half') && !ids.includes('kessa'), 'no hidden dot in the player layout: ' + ids);
  const dmLay = F.layoutTree(m, { dm: true, centre: 'maud', phone: false, width: 900, sib: true });
  assert.ok(Object.keys(dmLay.dots).includes('half') && Object.keys(dmLay.dots).includes('kessa'));
});

test('layoutTree: rows run grandparents, parents, centre and partners, children; lines join them', () => {
  const m = F.buildModel(data);
  const lay = F.layoutTree(m, { dm: false, centre: 'maud', phone: false, width: 900 });
  same(lay.rows.map((r) => r.key), ['u2', 'u1', 'm', 'd1']);
  same(lay.rows[1].ents.map((e) => e.id), ['mum', 'dad']);
  assert.equal(lay.dots.maud.me, 1);
  assert.ok(lay.rows[1].y < lay.rows[2].y && lay.rows[2].y < lay.rows[3].y);
  const pairs = lay.links.map((l) => l.a + '>' + l.b).sort();
  same(pairs, ['dad>maud', 'gran>mum', 'maud>tam', 'mum>dad', 'mum>maud']);
  assert.match(F.pathD(lay.links.find((l) => l.a === 'mum' && l.b === 'maud')), /^M[\d.]+ [\d.]+V/);
});

test('layoutTree: a wide row folds into a "more" bubble until opened, and the bubble counts the rest', () => {
  const kids = Array.from({ length: 12 }, (_, i) => person('k' + i, 'Kid ' + i, [par('p')]));
  const m = F.buildModel({ page: 'p', people: [person('p', 'Parent'), ...kids] });
  const closed = F.layoutTree(m, { dm: false, centre: 'p', width: 500 });
  const row = closed.rows.find((r) => r.key === 'd1');
  const bub = row.ents.find((e) => e.kind === 'bub');
  assert.ok(bub, 'a bubble');
  assert.equal(row.ents.filter((e) => e.kind === 'dot').length + bub.n, 12);
  const open = F.layoutTree(m, { dm: false, centre: 'p', width: 500, openRows: { d1: true } });
  assert.equal(open.rows.find((r) => r.key === 'd1').ents.filter((e) => e.kind === 'bub').length, 0);
  assert.equal(open.rows.find((r) => r.key === 'd1').ents.length, 12);
});

test('layoutTree: two generations each way, "further back" counts the rest', () => {
  const chain = Array.from({ length: 5 }, (_, i) => person('a' + i, 'A' + i, i < 4 ? [par('a' + (i + 1))] : []));
  chain[4] = person('a4', 'A4');
  const m = F.buildModel({ page: 'a0', people: chain });
  const lay = F.layoutTree(m, { dm: false, centre: 'a0', width: 900 });
  same(lay.rows.map((r) => r.key), ['u2', 'u1', 'm']);
  const more = lay.rows[0].ents.find((e) => e.kind === 'bub');
  assert.equal(more.n, 2);
  assert.equal(more.act, 'fu');
  const wider = F.layoutTree(m, { dm: false, centre: 'a0', width: 900, xu: 2 });
  assert.equal(wider.rows.length, 5);
});

test('pickPeekSpot: the card never covers a parent circle or the lit lines when a clear spot exists', () => {
  // A child hovered under its parent: the spot above/below the child that would hide the parent is refused.
  const bounds = { left: 0, top: 0, right: 600, bottom: 400 };
  const parentCircle = { l: 250, t: 40, r: 310, b: 100 };
  const lit = [[280, 120], [280, 140], [280, 160]];
  const cands = [
    { i: 0, x: 240, y: 30, w: 236, h: 56, cost: 0, flat: false, fw: 0 },
    { i: 1, x: 330, y: 200, w: 236, h: 56, cost: 1, flat: false, fw: 0 },
    { i: 2, x: 20, y: 150, w: 236, h: 56, cost: 2, flat: true, fw: 0 }
  ];
  const best = F.pickPeekSpot(cands, [parentCircle], lit, bounds, 6);
  assert.equal(best.i, 1, 'the clear candidate wins');
  const crowded = F.pickPeekSpot(cands.slice(0, 1), [parentCircle], lit, bounds, 6);
  assert.ok(crowded.cost >= 1e6, 'a lone covering candidate is priced out, not silently accepted');
});
