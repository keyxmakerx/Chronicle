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
