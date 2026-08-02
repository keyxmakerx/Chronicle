// daycard_block_immutability.test.mjs — C-CALV4-DAYCARD (R2-2a), §1 rule 1.
//
// THE BOUND THIS SLICE IS FENCED BY, MADE MECHANICAL.
//
// calendar-v4's Block has an interior law: no JS in the package, no motion in
// the grid, and the ONE sanctioned content change is the CSS-only answer
// ladder. Round 2 works AROUND that Block, and this slice opens no file in
// internal/widgets/calendar_block at all. That is a claim about a diff, and a
// diff-shaped claim decays: the next hand adds "just a class" from the module
// and nothing fails.
//
// So the rule is asserted at RUNTIME instead: boot the module against a
// Block-shaped fixture and require the Block host's serialised DOM to be
// BYTE-IDENTICAL before and after a full open + close. The module may query the
// Block and may listen to it; it may not insert a node inside .cal-block-host,
// may not add or remove a class on anything inside it, and may not animate
// anything inside it.
//
// THE ONE THING THAT LOOKS LIKE AN EXCEPTION AND IS NOT: the `Open in the
// Ledger` door calls .click() on the day's own radio. That activates a shipped
// control exactly as a pointer would — checkedness is IDL state, not a content
// attribute — so the serialised DOM is unchanged, and the assertion below
// covers that path too rather than exempting it.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './daycard_harness.mjs';

test('open + close leaves the Block host byte-identical', () => {
  const fx = boot();
  const before = fx.blockHost.outerHTML;
  assert.ok(before.length > 200, 'the fixture is too thin to prove anything');

  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.popoverOpen, true, 'the card never opened; the assertion would be vacuous');
  assert.equal(fx.blockHost.outerHTML, before, 'the module mutated the Block while OPENING');

  fx.fire('keydown', fx.card, { key: 'Escape' });
  fx.flush();
  assert.equal(fx.card.popoverOpen, false);
  assert.equal(fx.blockHost.outerHTML, before, 'the module mutated the Block while CLOSING');
});

test('every day, every opener, and the Ledger door all leave the Block unchanged', () => {
  const fx = boot();
  const before = fx.blockHost.outerHTML;

  for (const day of [3, 4, 5]) {
    fx.fire('click', fx.cells[day]);
    assert.equal(fx.blockHost.outerHTML, before, 'clicking day ' + day + ' mutated the Block');
    fx.fire('change', fx.cells[day].querySelector('input.daypick'));
    assert.equal(fx.blockHost.outerHTML, before, 'the radio path on day ' + day + ' mutated the Block');
  }

  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-ledger]'));
  fx.flush();
  assert.equal(fx.blockHost.outerHTML, before,
    'the `Open in the Ledger` door mutated the Block — activating the radio must change ' +
    'CHECKEDNESS (IDL state) and never a content attribute');
});

test('the card builds its rows in its OWN subtree and nowhere else', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  // The rows exist…
  assert.equal(fx.card.querySelector('[data-dc-rows]').children.length, 2);
  // …and the Block host gained no .dc-row anywhere.
  assert.equal(fx.blockHost.querySelectorAll('.dc-row').length, 0);
  assert.equal(fx.blockHost.querySelectorAll('[data-dc-rows]').length, 0);
});

test('the card is a page-level sibling, not a child of the Block host', () => {
  const fx = boot();
  assert.equal(fx.blockHost.querySelector('[data-cal-daycard]'), null,
    'the card must not be mounted inside .cal-block-host — the Block clips, and the ' +
    'top layer is what escapes that without touching it');
  assert.equal(fx.card.closest('[data-bench-block]'), null);
  assert.ok(fx.card.closest('[data-cal-bench]'),
    'the card must stay inside .cal-bench: the register lives there and every prelude ' +
    'in that sheet names it');
});

// ── C-CALV4-EDITOR-R2b: THE MORPH RUNS, AND THE BLOCK STILL DOES NOT MOVE ──
//
// EXTENDED per §4 bound 4 and [ER-7] SIGNED. The signature's last clause is
// "never touch the Block's interior", and the dispatch is explicit that this
// suite "must pass WITH THE MORPH RUNNING, not merely with the module loaded".
// The distinction is real: the morph is the first thing in this arc that
// MEASURES a rect and writes geometry in response, and a measurement is one
// refactor away from being a measurement of a cell.
test('the card→editor morph leaves the Block host byte-identical', () => {
  const fx = boot();
  const before = fx.blockHost.outerHTML;
  assert.ok(before.length > 200, 'the fixture is too thin to prove anything');

  fx.fire('click', fx.cells[3]);
  assert.equal(fx.blockHost.outerHTML, before, 'opening the card mutated the Block');

  fx.fire('click', fx.card.querySelector('[data-dc-new]'));
  assert.equal(fx.editor.classList.contains('edmorph'), true,
    'the morph never engaged; this assertion would be about a plain open');
  assert.equal(fx.blockHost.outerHTML, before, 'the MORPH mutated the Block while opening');

  fx.flush();
  assert.equal(fx.blockHost.outerHTML, before, 'the morph mutated the Block while settling');

  fx.fire('keydown', fx.editor, { key: 'Escape' });
  assert.equal(fx.blockHost.outerHTML, before, 'the REVERSE morph mutated the Block');
  fx.flush();
  assert.equal(fx.editor.popoverOpen, false);
  assert.equal(fx.blockHost.outerHTML, before, 'the morph mutated the Block while hiding');
});

test('the morph carries its class on the EDITOR and never on anything in the Block', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-new]'));

  // BOUND 4, at runtime rather than in the sheet: the class exists in exactly
  // one place on the page while it is in flight, and that place is the editor.
  assert.equal(fx.blockHost.querySelectorAll('.edmorph').length, 0);
  assert.equal(fx.host.querySelectorAll('.edmorph').length, 0);
  assert.equal(fx.root.querySelectorAll('.edmorph').length, 1);
  assert.equal(fx.root.querySelectorAll('.edmorph')[0], fx.editor);
});

test('the DRAG path leaves the Block host byte-identical, start to finish', () => {
  const fx = boot();
  const before = fx.blockHost.outerHTML;

  // EXTENDED for stage 4 ([DC-11] / [ER-8] SIGNED). The drag is the first thing
  // in this arc that follows the pointer ACROSS the Block's own cells, which is
  // exactly where "just a class, just during a drag" would go.
  fx.fire('pointerdown', fx.cells[3], { button: 0 });
  assert.equal(fx.blockHost.outerHTML, before, 'pointerdown mutated the Block');
  fx.fire('pointermove', fx.cells[4]);
  assert.equal(fx.blockHost.outerHTML, before, 'the preview marked a cell');
  fx.fire('pointermove', fx.cells[5]);
  assert.equal(fx.blockHost.outerHTML, before, 'the growing preview marked a cell');
  fx.fire('pointerup', fx.cells[5]);
  assert.equal(fx.blockHost.outerHTML, before, 'release mutated the Block');
  fx.flush();
  assert.equal(fx.blockHost.outerHTML, before, 'the editor the drag opened mutated the Block');
});
