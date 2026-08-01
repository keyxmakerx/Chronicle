// daycard_open_close.test.mjs — C-CALV4-DAYCARD (R2-2a). The open/close state
// machine, the two openers, the empty state, and the `Open in the Ledger` door.
//
// These run against a BENCH-SHAPED fixture (test/js/daycard_harness.mjs) rather
// than a stub, because every claim here is about what the module does to a real
// tree — and the claim the whole slice turns on is what it does NOT do to the
// Block's half of it (daycard_block_immutability.test.mjs).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './daycard_harness.mjs';

function rowTitles(fx) {
  return fx.card.querySelector('[data-dc-rows]').children.map(
    (r) => r.querySelector('.nm').textContent
  );
}

test('clicking a day opens the card, headed and listed', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);

  assert.equal(fx.card.popoverOpen, true, 'the popover never opened');
  assert.equal(fx.card.hasAttribute('data-dc-shown'), true);
  assert.equal(fx.card.classList.contains('dcopen'), true, 'the register reveal never engaged');
  assert.equal(fx.card.querySelector('[data-dc-head]').textContent, '3 Deepwinter 1523 · Thirdday');
  // GUARD B4: the dated node carries the ANSWER key.
  assert.equal(fx.card.querySelector('[data-dc-head]').getAttribute('data-day'), 'harptos-3');
  assert.deepEqual(rowTitles(fx), ['Council of Wards', 'Barrow scouting']);
});

test('a row is the Ledger row field set and nothing more', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  const rows = fx.card.querySelector('[data-dc-rows]').children;

  const plain = rows[0];
  assert.equal(plain.getAttribute('data-event-id'), 'ev-1');
  assert.equal(plain.getAttribute('data-day'), 'harptos-3');
  assert.equal(plain.querySelector('.rail').className, 'rail p3');
  assert.equal(plain.style.getPropertyValue('--axis'), 'var(--own-1)');
  assert.equal(plain.querySelector('.gr'), null, 'a public event must not draw the gold rail');
  assert.equal(plain.querySelector('.badge'), null, 'a public event must not carry the GM badge');

  // dm_only: THE GOLD RAIL AND THE `GM` BADGE SPLIT ON dm_only, and both ride
  // the same flag the Ledger row splits on.
  const gm = rows[1];
  assert.ok(gm.querySelector('.gr'), 'the dm_only row is missing its gold rail');
  assert.equal(gm.querySelector('.badge.gm').textContent, 'GM');
  assert.equal(gm.querySelector('.audchip').textContent, 'GM only');
  assert.equal(gm.querySelector('.tok').textContent, '▲');
});

test('an untimed event drops the time element rather than printing an empty one', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.querySelector('[data-dc-rows]').children[0].querySelector('.tm'), null);
  fx.fire('click', fx.cells[5]);
  assert.equal(fx.card.querySelector('[data-dc-rows]').children[0].querySelector('.tm').textContent, '18:00');
});

test('a quiet day gets a real empty state, never a blank card', () => {
  const fx = boot();
  fx.fire('click', fx.cells[4]);
  assert.equal(fx.card.popoverOpen, true, 'the card must still open on a day with no events');
  assert.equal(fx.card.querySelector('[data-dc-rows]').children.length, 0);
  assert.equal(fx.card.querySelector('[data-dc-empty]').hidden, false);
  // …and it goes away again when the day has events.
  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.querySelector('[data-dc-empty]').hidden, true);
});

test('both openers are wired, and re-opening the same day is a no-op', () => {
  const fx = boot();
  // The radio's `change` is the KEYBOARD path ([DC-4]): it exists wherever the
  // radio does, and it costs nothing.
  const radio = fx.cells[5].querySelector('input.daypick');
  fx.fire('change', radio);
  assert.equal(fx.card.popoverOpen, true);
  assert.equal(fx.card.querySelector('[data-dc-head]').getAttribute('data-day'), 'harptos-5');

  // A pointer click on the stretched .dsel label fires BOTH openers in a real
  // browser, so opening the SAME day twice must be idempotent rather than a
  // second animation.
  const before = fx.card.outerHTML;
  fx.fire('click', fx.cells[5].querySelector('label.dsel'));
  assert.equal(fx.card.outerHTML, before, 're-opening the same day changed the card');
});

test('the card re-targets when another day is clicked while it is open', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.cells[5]);
  assert.equal(fx.card.querySelector('[data-dc-head]').textContent, '5 Deepwinter 1523 · Fifthday');
  assert.deepEqual(rowTitles(fx), ['Caravan due']);
  assert.equal(fx.card.popoverOpen, true);
});

test('close runs the register: the reveal is removed first, the popover hides after', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  fx.fire('keydown', fx.card, { key: 'Escape' });

  assert.equal(fx.card.classList.contains('dcopen'), false, 'the reveal must reverse immediately');
  assert.equal(fx.card.popoverOpen, true, 'the popover must survive the close animation');
  assert.equal(fx.timers.length, 1, 'exactly one close timer, at --disc-close');
  assert.equal(fx.timers[0].ms, 160);
  fx.flush();
  assert.equal(fx.card.popoverOpen, false);
  assert.equal(fx.card.hasAttribute('data-dc-shown'), false);
});

test('under reduced motion the close is INSTANT AND COMPLETE, with nothing awaited', () => {
  const fx = boot({ reduced: true });
  fx.fire('click', fx.cells[3]);
  fx.fire('keydown', fx.card, { key: 'Escape' });
  assert.equal(fx.timers.length, 0,
    'the module waited for an animation the sheet declares no rule for');
  assert.equal(fx.card.popoverOpen, false, 'the card must already be closed');
});

test('a pointerdown outside the card dismisses it (popover=manual owns dismissal)', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.root);
  assert.equal(fx.card.classList.contains('dcopen'), false);
  fx.flush();
  assert.equal(fx.card.popoverOpen, false);
});

test('a click INSIDE the card does not dismiss it', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-head]'));
  assert.equal(fx.card.popoverOpen, true);
  assert.equal(fx.card.classList.contains('dcopen'), true);
});

test('the Ledger door is emitted only when the payload says the column is docked', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  assert.ok(fx.card.querySelector('[data-dc-ledger]'), 'a docked Ledger must offer the door');

  const off = boot({
    payload: {
      calendars: [{
        id: 'cal-1', slug: 'harptos', ledgerDocked: false,
        days: [{ key: 'harptos-3', ord: '3', day: 3, label: '3 Deepwinter 1523', events: [] }],
      }],
    },
  });
  off.fire('click', off.cells[3]);
  assert.equal(off.card.popoverOpen, true,
    'with the Ledger off the card is the ONLY answer and must still open');
  assert.equal(off.card.querySelector('[data-dc-ledger]'), null,
    'a link to a column that is not on the page is the dishonesty this arc kills');
});

test('the Ledger door activates the shipped radio and scrolls the column into view', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  const radio = fx.cells[3].querySelector('input.daypick');
  fx.fire('click', fx.card.querySelector('[data-dc-ledger]'));

  assert.equal(radio.clicks, 1, 'the door must activate the day radio the server rendered');
  assert.equal(fx.ledger.scrolled, 1, 'the door must bring the Ledger column into view');
  assert.equal(fx.card.classList.contains('dcopen'), false, 'the card leaves as the ladder fires');
});

test('the module is inert on a page with no payload attribute', () => {
  const fx = boot({ payload: null });
  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.popoverOpen, false);
  assert.equal(fx.card.dataset.dcWired, undefined, 'nothing should have been wired');
});

test('re-init after a boosted navigation cannot double-bind', () => {
  const fx = boot();
  assert.ok(fx.document._listeners['htmx:afterSettle'], 'htmx:afterSettle is not wired');
  assert.ok(fx.document._listeners['htmx:load'], 'htmx:load is not wired');
  const clickHandlers = fx.document._listeners['click'].length;
  fx.document._listeners['htmx:afterSettle'].forEach((fn) => fn({}));
  fx.document._listeners['htmx:load'].forEach((fn) => fn({}));
  assert.equal(fx.document._listeners['click'].length, clickHandlers,
    'a re-init added a second click handler — the QA2 listener-leak class of bug');
});

// --- DC-CLEAR-1: the occlusion report, end to end ---------------------------

test('an opened card records on its own root whether it cleared the Ledger', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.getAttribute('data-dc-clear'), '1',
    'the Bench geometry clears the column and the card must say so');
});

test('a geometry that cannot clear the Ledger falls back to the sheet AND says so once', () => {
  const said = [];
  // A Ledger that fills most of the Bench in both axes, so neither below nor
  // above its day can clear it. Before DC3-STACKED-LEDGER-OCCLUSION-1 the card
  // was placed as a popover ON TOP of the column its own `Open in the Ledger`
  // door points at, and the only symptom was one console line. Now the card
  // degrades to [DC-3] bullet 4's signed sheet — and the warning survives,
  // because the geometry still ran out and that is the signed STOP-AND-FLAG.
  const fx = boot({ console: { warn: (m) => said.push(m) } });
  fx.ledger.rect = { left: 300, top: 200, right: 1200, bottom: 800, width: 900, height: 600 };

  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.classList.contains('dcsheet'), true,
    'the desktop last resort is the SHEET, never an occluding popover');
  assert.equal(fx.card.getAttribute('data-dc-clear'), '0',
    'an occluded Ledger must be recorded on the DOM, not swallowed');
  assert.equal(said.length, 1, 'the geometry running out must be reported exactly once');
  assert.match(said[0], /DC-3/);

  // Reopening on another day re-measures but does not re-announce.
  fx.fire('click', fx.cells[5]);
  assert.equal(fx.card.getAttribute('data-dc-clear'), '0');
  assert.equal(said.length, 1, 'one warning per session, not one per placement');
});

test('the STACKED Ledger no longer gets covered: the card flips above its day', () => {
  const said = [];
  // The shipped stacked-Ledger layout, measured at a ~884px .cal-bench content
  // width: the Ledger is a full-width band BELOW the grid, so the old
  // horizontal-only dodge had nowhere to go and the card sat on the band for
  // every day and every viewer. The vertical dodge clears it, and because the
  // card ends up clear there is nothing to warn about.
  const fx = boot({ viewportW: 1180, viewportH: 800, console: { warn: (m) => said.push(m) } });
  fx.ledger.rect = { left: 9, top: 595, right: 891, bottom: 751, width: 882, height: 156 };

  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.classList.contains('dcsheet'), false, 'a desktop width keeps the popover');
  assert.equal(fx.card.getAttribute('data-dc-clear'), '1',
    'the stacked band must be dodged, not covered and confessed to');
  assert.equal(said.length, 0, 'a clear placement has nothing to say');
});

test('the mobile bottom sheet records the overlap and stays silent about it', () => {
  const said = [];
  // 390px, the measured Bench: the sheet is full-width at the foot and the
  // narrow layout has stacked the Ledger below the grid. DC-3's own bullet 4
  // signs the bottom sheet and §12 scopes the STOP-AND-FLAG row to 1232px, so
  // this is the signed treatment — recorded, never announced.
  const fx = boot({ viewportW: 390, viewportH: 799, console: { warn: (m) => said.push(m) } });
  fx.ledger.rect = { left: 9, top: 594, right: 381, bottom: 740, width: 372, height: 146 };

  fx.fire('click', fx.cells[3]);
  assert.equal(fx.card.classList.contains('dcsheet'), true, 'below the breakpoint the card is a sheet');
  assert.equal(fx.card.getAttribute('data-dc-clear'), '0',
    'the overlap is real and the DOM records it at every width');
  assert.equal(said.length, 0,
    'warning on the signed mobile treatment would train the next hand to ignore the warning that matters');
});

test('the editor is placed through the same reader as the card', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-new]'));
  assert.equal(fx.editor.getAttribute('data-dc-clear'), '1',
    'the editor covers the same column and reports through the same path');
});

// ── DC3-DESKTOP-SHEET-OCCLUSION-R4, at the DOM ────────────────────────────────
//
// The two reachable ways a TALL box used to reach the desktop sheet and cover
// 100% of the stacked Ledger. The placement rule is pinned as a pure function in
// daycard_payload.test.mjs; these two run the whole path — click, measure, place,
// write the attribute, decide whether to warn — because the blocker was only
// visible end to end: the box's height comes from the rendered rows, and a pure
// test that is handed the height cannot notice that the rows produce it.

test('a tall EDITOR clears the stacked band instead of sheeting over it', () => {
  const said = [];
  const fx = boot({ viewportW: 944, viewportH: 900, console: { warn: (m) => said.push(m) } });
  // The measured ~884px content geometry: the Ledger is a full-width band
  // stacked below the grid, not a docked right-hand column.
  fx.ledger.rect = { left: 31, top: 595, right: 913, bottom: 717, width: 882, height: 122 };
  fx.editor.querySelector('[data-dc-box]').scrollHeight = 376; // a 400px editor

  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-new]'));

  assert.equal(fx.editor.classList.contains('dcsheet'), false,
    'a 944px viewport is nowhere near the mobile breakpoint — the desktop sheet ' +
    'here covered the whole Ledger while a clear popover position existed');
  assert.equal(fx.editor.getAttribute('data-dc-clear'), '1',
    'the flip must actually clear the band it dodged');
  assert.equal(said.length, 0,
    'and a placement that succeeded must not raise [DC-3]s STOP-AND-FLAG');
});

test('a BUSY day card clears the stacked band instead of sheeting over it', () => {
  const said = [];
  const fx = boot({ viewportW: 944, viewportH: 900, console: { warn: (m) => said.push(m) } });
  fx.ledger.rect = { left: 31, top: 595, right: 913, bottom: 717, width: 882, height: 122 };
  // 16 rows against `.dc-rows`s min(52vh,420px) cap — a festival day, not a
  // stress case.
  fx.card.querySelector('[data-dc-box]').scrollHeight = 457;

  fx.fire('click', fx.cells[3]);

  assert.equal(fx.card.classList.contains('dcsheet'), false);
  assert.equal(fx.card.getAttribute('data-dc-clear'), '1');
  assert.equal(said.length, 0);
});
