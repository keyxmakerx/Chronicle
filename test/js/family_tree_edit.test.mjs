// family_tree_edit.test.mjs — pins the Family tree's Director edit state: the
// pure changes to a model, the difference that Save sends, the picker's
// exclusions, the empty places the layout adds in edit mode, and the words the
// save bar uses when a save does not fully land. The browser flow is covered by
// test/e2e/family_tree_edit.spec.mjs.

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
// Values built inside the vm context carry another realm's prototypes, so compare as JSON.
const same = (a, b, msg) => assert.equal(JSON.stringify(a), JSON.stringify(b), msg);

const person = (id, name, parents = [], partners = [], extra = {}) => ({ id, name, sub: 'Character', url: '/campaigns/c/entities/' + id, parents, partners, read: true, ...extra });
const par = (id, kind = 'birth', dm = false) => ({ id, kind, dm });

// gran > mum, dad > maud (page) > tam; mum and dad are partners.
function fresh() {
  return F.buildModel({
    page: 'maud',
    people: [
      person('maud', 'Maud', [par('mum'), par('dad')]),
      person('mum', 'Ione', [par('gran')], ['dad']),
      person('dad', 'Corvin', [], ['mum']),
      person('gran', 'Edda'),
      person('tam', 'Tam', [par('maud')])
    ]
  });
}
const cand = (id, name, extra = {}) => ({ id, name, sub: 'Character', img: '', dm: false, ...extra });

test('diffModels: what Save sends is exactly what changed', () => {
  const tests = [
    { name: 'nothing changed', edit: () => {}, want: { newPages: [], edits: [] } },
    {
      name: 'a parent added (born to)',
      edit: (m) => { F.addPerson(m, F.personFromCandidate(cand('q', 'Quill'), '/p/')); F.addParentEdge(m, 'tam', 'q', 'birth'); },
      want: { newPages: [], edits: [{ op: 'add', edge: 'parent', a: 'tam', b: 'q', link: 'birth' }] }
    },
    {
      name: 'a child added as adopted: the child holds the parent link',
      edit: (m) => { F.addPerson(m, F.personFromCandidate(cand('q', 'Quill'), '/p/')); F.addParentEdge(m, 'q', 'maud', 'adopted'); },
      want: { newPages: [], edits: [{ op: 'add', edge: 'parent', a: 'q', b: 'maud', link: 'adopted' }] }
    },
    {
      name: 'a partner added',
      edit: (m) => { F.addPerson(m, F.personFromCandidate(cand('q', 'Quill'), '/p/')); F.addPartnerPair(m, 'maud', 'q'); },
      want: { newPages: [], edits: [{ op: 'add', edge: 'partner', a: 'maud', b: 'q' }] }
    },
    {
      name: 'a parent removed',
      edit: (m) => { F.removeParentEdge(m, 'maud', 'dad'); },
      want: { newPages: [], edits: [{ op: 'remove', edge: 'parent', a: 'maud', b: 'dad' }] }
    },
    {
      name: 'a partner removed is one edit, not two',
      edit: (m) => { F.removePartnerPair(m, 'mum', 'dad'); },
      want: { newPages: [], edits: [{ op: 'remove', edge: 'partner', a: 'dad', b: 'mum' }] }
    },
    {
      name: 'a link changed from born to adopted is a remove and an add',
      edit: (m) => { F.addParentEdge(m, 'maud', 'dad', 'adopted'); },
      want: { newPages: [], edits: [{ op: 'remove', edge: 'parent', a: 'maud', b: 'dad' }, { op: 'add', edge: 'parent', a: 'maud', b: 'dad', link: 'adopted' }] }
    },
    {
      name: 'a link added then removed again is no change',
      edit: (m) => { F.addPerson(m, F.personFromCandidate(cand('q', 'Quill'), '/p/')); F.addParentEdge(m, 'tam', 'q', 'birth'); F.removeParentEdge(m, 'tam', 'q'); },
      want: { newPages: [], edits: [] }
    },
    {
      name: 'a made-up page is created once, by ref, and used by both its links',
      edit: (m) => {
        const id = F.newPersonId(m);
        F.addPerson(m, { id, name: 'Odo', sub: 'New page' });
        F.addParentEdge(m, 'maud', id, 'guardian');
        F.addPartnerPair(m, 'tam', id);
      },
      want: {
        newPages: [{ ref: 'new:1', name: 'Odo' }],
        edits: [{ op: 'add', edge: 'parent', a: 'maud', b: 'new:1', link: 'guardian' }, { op: 'add', edge: 'partner', a: 'new:1', b: 'tam' }]
      }
    },
    {
      name: 'a made-up page left unlinked is never created',
      edit: (m) => { const id = F.newPersonId(m); F.addPerson(m, { id, name: 'Odo' }); F.addParentEdge(m, 'maud', id, 'birth'); F.removeParentEdge(m, 'maud', id); },
      want: { newPages: [], edits: [] }
    }
  ];
  for (const tt of tests) {
    const base = fresh(), cur = F.cloneModel(base);
    tt.edit(cur);
    same(F.diffModels(base, cur), tt.want, tt.name);
  }
});

test('cloneModel: an edit never reaches the saved model', () => {
  const base = fresh(), cur = F.cloneModel(base);
  F.removeParentEdge(cur, 'maud', 'mum');
  F.removePartnerPair(cur, 'mum', 'dad');
  assert.equal(base.P.maud.par.length, 2);
  same(base.P.mum.partners, ['dad']);
});

test('pickerExcluded: the picker never offers a link that is there or a loop', () => {
  const m = fresh();
  const keys = (kind, centre = 'maud') => Object.keys(F.pickerExcluded(m, centre, kind)).sort();
  same(keys('parent'), ['dad', 'maud', 'mum', 'tam'].sort(), 'a parent is not the page, a current parent or a descendant');
  same(keys('child'), ['dad', 'gran', 'maud', 'mum', 'tam'].sort(), 'a child is not the page, a current child or an ancestor');
  same(keys('partner'), ['maud'], 'a partner may be anyone but the page');
  same(keys('partner', 'mum'), ['dad', 'mum'], 'and not a current partner');
  // Pending edits count too: a parent just added is not offered again.
  F.addPerson(m, F.personFromCandidate(cand('q', 'Quill'), ''));
  F.addParentEdge(m, 'maud', 'q', 'birth');
  assert.ok(F.pickerExcluded(m, 'maud', 'parent').q);
});

test('addParentEdge and addPartnerPair refuse nonsense', () => {
  const m = fresh();
  assert.equal(F.addParentEdge(m, 'maud', 'maud', 'birth'), false, 'not itself');
  assert.equal(F.addParentEdge(m, 'maud', 'nobody', 'birth'), false, 'not an unknown page');
  assert.equal(F.addPartnerPair(m, 'maud', 'maud'), false);
  assert.equal(F.addPartnerPair(m, 'maud', 'nobody'), false);
});

test('personFromCandidate: the DM mark and the page address carry over', () => {
  const p = F.personFromCandidate(cand('q', 'Quill', { dm: true }), '/campaigns/c/entities/');
  assert.equal(p.hidden, true);
  assert.equal(p.url, '/campaigns/c/entities/q');
  assert.equal(p.read, false, 'their own relations are not read, so no Make centre');
  assert.equal(F.personFromCandidate(cand('q', 'Quill'), '').url, '');
});

test('renameId: a made-up page becomes the real one everywhere', () => {
  const m = fresh();
  F.addPerson(m, { id: 'new:1', name: 'Odo' });
  F.addParentEdge(m, 'maud', 'new:1', 'guardian');
  F.addPartnerPair(m, 'tam', 'new:1');
  F.renameId(m, 'new:1', 'real-9');
  assert.equal(m.P['new:1'], undefined);
  assert.equal(m.P['real-9'].id, 'real-9');
  assert.ok(m.order.includes('real-9') && !m.order.includes('new:1'));
  assert.ok(m.P.maud.par.some((e) => e.id === 'real-9' && e.t === 'guardian'));
  same(m.P.tam.partners, ['real-9']);
  same(m.P['real-9'].partners, ['tam']);
});

test('layoutTree: the Edit tab adds the empty places, and only then', () => {
  const lone = F.buildModel({ page: 'solo', people: [person('solo', 'Solo')] });
  const lay = (m, edit, centre) => F.layoutTree(m, { dm: true, centre, phone: false, width: 900, edit });
  const slotsOf = (L) => Object.keys(L.slots).sort();

  const plain = lay(lone, false, 'solo');
  same(slotsOf(plain), [], 'no places outside edit mode');
  same(plain.rows.map((r) => r.key), ['m']);

  const edit = lay(lone, true, 'solo');
  same(slotsOf(edit), ['s:child', 's:parent', 's:partner']);
  same(edit.rows.map((r) => r.key), ['u1', 'm', 'd1'], 'an empty page gets a parents row and a children row to hold them');
  same(edit.links, [], 'a place draws no line');
  assert.equal(Object.keys(edit.dots).length, 1, 'a place is not a person');

  const m = fresh();
  const full = lay(m, true, 'maud');
  same(slotsOf(full), ['s:child', 's:parent', 's:partner'], 'always offered, even with two parents (adoptive and guardian links exist)');
  const u1 = full.rows.find((r) => r.key === 'u1');
  assert.equal(u1.ents[u1.ents.length - 1].kind, 'slot', 'the place comes last in its row');
  const mid = full.rows.find((r) => r.key === 'm');
  assert.equal(mid.ents[mid.ents.length - 1].slot, 'partner');
  same(lay(m, false, 'maud').rows.map((r) => r.key), ['u2', 'u1', 'm', 'd1']);
});

test('describeEdit: one change in words', () => {
  const m = fresh();
  const tests = [
    [{ op: 'add', edge: 'parent', a: 'maud', b: 'gran', link: 'birth' }, 'adding Edda as a parent of Maud'],
    [{ op: 'add', edge: 'parent', a: 'maud', b: 'gran', link: 'adopted' }, 'adding Edda as an adoptive parent of Maud'],
    [{ op: 'add', edge: 'parent', a: 'maud', b: 'gran', link: 'guardian' }, 'adding Edda as a guardian of Maud'],
    [{ op: 'remove', edge: 'parent', a: 'maud', b: 'dad' }, 'removing Corvin as a parent of Maud'],
    [{ op: 'add', edge: 'partner', a: 'maud', b: 'tam' }, 'adding Maud and Tam as partners'],
    [{ op: 'remove', edge: 'partner', a: 'mum', b: 'ghost' }, 'removing Ione and a page as partners']
  ];
  for (const [ed, want] of tests) assert.equal(F.describeEdit(m, ed), want);
});

test('saveReport: a failed save says which links did not save and what state that leaves', () => {
  const m = fresh();
  const edits = [
    { op: 'add', edge: 'parent', a: 'maud', b: 'gran', link: 'birth' },
    { op: 'remove', edge: 'partner', a: 'dad', b: 'mum' },
    { op: 'add', edge: 'partner', a: 'maud', b: 'tam' }
  ];
  const tests = [
    {
      name: 'rolled back: nothing saved, the one that failed is named',
      res: { failed: [{ index: 1, reason: 'x' }], rolledBack: true, written: [], created: [] },
      has: ['Nothing was saved.', 'Could not do: removing Corvin and Ione as partners.', 'press Save to try again'],
      not: ['half-written', 'Some changes were saved']
    },
    {
      name: 'a half-written pair is never silent',
      res: { failed: [{ index: 2, reason: 'x', halfWritten: true }], rolledBack: false, written: [2], created: [] },
      has: ['Some changes were saved and some were not.', 'Left half-written, finish or remove it in Relations: adding Maud and Tam as partners.'],
      not: ['Saved and not taken back']
    },
    {
      name: 'a takeback that failed lists what stayed saved',
      res: { failed: [{ index: 2, reason: 'x' }], rolledBack: false, written: [0], created: [] },
      has: ['Could not do: adding Maud and Tam as partners.', 'Saved and not taken back: adding Edda as a parent of Maud.']
    },
    {
      name: 'a page that was created stays and is reused',
      res: { failed: [{ index: 0, reason: 'x' }], rolledBack: true, written: [], created: [{ ref: 'new:1', id: 'r', name: 'Odo' }] },
      has: ['The new page Odo was created and will be used when you save again.']
    },
    {
      name: 'a page that could not be created',
      res: { failed: [{ index: -1, reason: 'x' }], rolledBack: true, written: [], created: [] },
      has: ['Could not do: creating a new page.']
    }
  ];
  for (const tt of tests) {
    const out = F.saveReport(m, edits, tt.res);
    for (const w of tt.has) assert.ok(out.includes(w), `${tt.name}: lacks ${JSON.stringify(w)} in ${JSON.stringify(out)}`);
    for (const w of tt.not || []) assert.ok(!out.includes(w), `${tt.name}: should not say ${JSON.stringify(w)}`);
  }
});
