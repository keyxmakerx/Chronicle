// theater_block_immutability.test.mjs — C-CALV4-THEATER (R2-3), dispatch §12's
// "No Block DOM mutation" guard, and the second consumer of [DC-3]'s rule.
//
// The theater is a box around a Block the producer already rendered. The module
// may QUERY the Block and LISTEN to it; it may not insert a node inside
// `.cal-block-host`, add or remove a class inside it, set an attribute on it,
// or REPARENT it. Reparenting is covered by name because it is the one that
// does not look like a mutation: a Block that changes parents mid-session is a
// container query resolving against a box that is being animated, and the tier
// it lands on is whatever the reflow happened to produce.
//
// TWO BLOCKS, TWO ASSERTIONS. The entity page now carries the EMBED's Block and
// the THEATER's, and the interesting failure is asymmetric — a module that
// reached into the embed behind the backdrop would be the exact bug [TH-2]'s
// re-namespace exists to prevent, one level up. So both subtrees are checked.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './theater_harness.mjs';

function snapshot(fx) {
  return {
    embed: fx.embed.innerHTML,
    embedOuter: fx.embed.outerHTML,
    theater: fx.theaterBlock.innerHTML,
    theaterOuter: fx.theaterBlock.outerHTML,
    embedParent: fx.embed.parentNode,
    theaterParent: fx.theaterBlock.parentNode,
  };
}

function assertUnchanged(before, fx, when) {
  const after = snapshot(fx);
  assert.equal(after.embed, before.embed, 'the EMBED Block\'s innerHTML changed ' + when);
  assert.equal(after.embedOuter, before.embedOuter, 'the EMBED Block\'s own attributes changed ' + when);
  assert.equal(after.theater, before.theater, 'the THEATER Block\'s innerHTML changed ' + when);
  assert.equal(after.theaterOuter, before.theaterOuter, 'the THEATER Block\'s own attributes changed ' + when);
  assert.equal(after.embedParent, before.embedParent, 'the EMBED Block was REPARENTED ' + when);
  assert.equal(after.theaterParent, before.theaterParent, 'the THEATER Block was REPARENTED ' + when);
}

test('open + close leaves both Blocks byte-identical', () => {
  const fx = boot();
  const before = snapshot(fx);

  fx.fireOn('click', fx.opener);
  assertUnchanged(before, fx, 'on open');

  fx.fireOn('click', fx.closeBtn);
  fx.fireOn('transitionend', fx.dialog, { target: fx.dialog.querySelector('[data-theater-box]') });
  assertUnchanged(before, fx, 'on close');
});

test('the Escape, backdrop, watchdog and reduced-motion paths mutate nothing either', () => {
  for (const run of [
    (fx) => fx.fireOn('cancel', fx.dialog),
    (fx) => fx.fireOn('click', fx.dialog, { target: fx.dialog }),
    (fx) => { fx.fireOn('click', fx.closeBtn); fx.flush(); },
  ]) {
    const fx = boot();
    const before = snapshot(fx);
    fx.fireOn('click', fx.opener);
    run(fx);
    fx.fireOn('transitionend', fx.dialog, { target: fx.dialog.querySelector('[data-theater-box]') });
    assertUnchanged(before, fx, 'on one of the close paths');
  }

  const reduced = boot({ reduced: true });
  const before = snapshot(reduced);
  reduced.fireOn('click', reduced.opener);
  reduced.fireOn('cancel', reduced.dialog);
  assertUnchanged(before, reduced, 'under reduced motion');
});

test('the swap close mutates nothing it did not have to', () => {
  const fx = boot();
  const before = snapshot(fx);
  fx.fireOn('click', fx.opener);
  fx.fire('htmx:beforeSwap', { target: fx.host });
  assertUnchanged(before, fx, 'on the htmx:beforeSwap close');
});

test('the module writes only to the scaffold, the opener and the documentElement', () => {
  // The positive half of the claim above: this proves the assertions are about
  // a module that DOES something, rather than about a module that never ran.
  const fx = boot();
  fx.fireOn('click', fx.opener);

  assert.equal(fx.dialog.open, true, 'the module never opened anything, so every "unchanged" above is vacuous');
  assert.equal(fx.boxOpen(), true, 'the reveal class is the module\'s one write inside the scaffold');
  assert.equal(fx.opener.getAttribute('aria-expanded'), 'true');
  assert.equal(fx.lockedNow(), true);

  // …and none of those three writes is inside a .cal-block-host.
  assert.equal(fx.embed.querySelector('.tbopen'), null);
  assert.equal(fx.theaterBlock.querySelector('.tbopen'), null);
});
