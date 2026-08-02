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

// ── C-CALV4-EDITOR-R2b: THE MORPH'S STATE MACHINE ─────────────────────────
//
// EXTENDED per §4 and [ER-6] / [ER-7] SIGNED. What is asserted here is the
// mechanism the operator signed: ONE BOX BECOMES THE OTHER, growing from the
// card's measured geometry rather than sliding in from elsewhere, with no
// scale, reversing faster than it arrived, and landing INSTANT AND COMPLETE
// under reduced motion.

function openEditorFromCard(fx, day) {
  fx.fire('click', fx.cells[day]);
  fx.fire('click', fx.card.querySelector('[data-dc-new]'));
}

test('the editor grows FROM the card’s measured geometry, not from nowhere', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  const card = fx.card.getBoundingClientRect();
  assert.ok(card.width > 0 && card.height > 0, 'the card has no rect to morph from');

  fx.fire('click', fx.card.querySelector('[data-dc-new]'));

  // THE START STATE: the editor sits at the card's rect, at the card's size,
  // transparent. The offset is a `translate`, so the box's own left/top stay
  // the placement law's answer and the morph never re-decides where it goes.
  const ed = fx.editor;
  assert.equal(ed.classList.contains('edmorph'), true, 'the carve-out class is not on the box');
  // The offset the START state carried, recomputed here from the two rects
  // rather than read back off the box: by the time this line runs the module
  // has already written the END state, and asserting on that would be asserting
  // that a value equals itself.
  const placedLeft = parseFloat(ed.style.left) || 0;
  const placedTop = parseFloat(ed.style.top) || 0;
  assert.ok(Math.round(card.left - placedLeft) !== 0 || Math.round(card.top - placedTop) !== 0,
    'the card and the editor were placed at the same point, so this fixture ' +
    'cannot tell a morph from a plain reveal');
  assert.equal(ed.style.getPropertyValue('translate'), '0px 0px',
    'the END state must be no offset — the box travels by `translate` and lands ' +
    'on the placement law\'s own answer, never on a second geometry');

  // THE END STATE, written in the same task: back to no offset, at the box's
  // own measured size, fully opaque.
  assert.equal(ed.style.getPropertyValue('inline-size'), '760px');
  assert.equal(ed.style.getPropertyValue('opacity'), '1');
  assert.equal(ed.classList.contains('dcopen'), true);
});

test('the morph does not scale — it writes a size, never a transform', () => {
  const fx = boot();
  openEditorFromCard(fx, 3);
  const ed = fx.editor;
  // A FLIP SCALE IS THE CHEAP WAY AND IT SQUASHES A FORM'S TEXT. `translate` is
  // named on its own precisely so this assertion can exist.
  assert.equal(ed.style.getPropertyValue('transform'), '');
  assert.equal(ed.style.getPropertyValue('scale'), '');
  assert.ok(/px/.test(ed.style.getPropertyValue('inline-size')),
    'the box grew by something other than its own size');
});

test('the morph settles and hands the box back its natural sizing', () => {
  const fx = boot();
  openEditorFromCard(fx, 3);
  assert.equal(fx.editor.classList.contains('edmorph'), true);

  fx.flush();

  // ONCE THE GEOMETRY HAS LANDED THE CLASS COMES OFF and the inline sizing goes
  // with it. Leaving the measured height pinned would make `overflow:hidden`
  // clip anything the form grew afterwards — the audience roster opening under
  // Restricted is exactly that case.
  assert.equal(fx.editor.classList.contains('edmorph'), false,
    'a resting editor still carries the carve-out class, so a later content ' +
    'change would animate something nobody signed');
  assert.equal(fx.editor.style.getPropertyValue('inline-size'), '');
  assert.equal(fx.editor.style.getPropertyValue('block-size'), '');
  assert.equal(fx.editor.style.getPropertyValue('translate'), '');
  assert.equal(fx.editor.popoverOpen, true, 'the editor closed instead of settling');
});

test('close reverses the SAME geometry, and is never slower than open', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  // The card's rect, taken while it is still the open box — the same
  // measurement the module takes, computed INDEPENDENTLY here so the assertion
  // below is a check rather than an echo of whatever the module stored.
  const card = fx.card.getBoundingClientRect();
  fx.fire('click', fx.card.querySelector('[data-dc-new]'));
  const placedLeft = parseFloat(fx.editor.style.left) || 0;
  const placedTop = parseFloat(fx.editor.style.top) || 0;
  const want = Math.round(card.left - placedLeft) + 'px ' +
    Math.round(card.top - placedTop) + 'px';
  fx.flush();

  // THE LOG IS SCOPED TO THE CLOSE. It accumulates from the open phase too —
  // where `.edmorph` is added BEFORE `.dcopen`, which is correct and would make
  // the ordering assertion below pass vacuously on any implementation.
  fx.editor._ops.length = 0;
  fx.fire('keydown', fx.editor, { key: 'Escape' });

  // THE REVERSE IS THE SAME FOUR PROPERTIES ONTO THE SAME MEASURED RECT — not
  // a second signature, and not a fade-out standing in for one.
  assert.equal(fx.editor.classList.contains('edmorph'), true);
  assert.equal(fx.editor.style.getPropertyValue('opacity'), '0');
  assert.equal(fx.editor.style.getPropertyValue('translate'), want,
    'the reverse morph went somewhere other than the card it came from');

  // CLOSE FASTER THAN OPEN, READ FROM THE TOKENS RATHER THAN ASSERTED. The
  // harness reports --disc-close 160ms and --disc-open 200ms, which is what the
  // sheet declares, and `.dcopen` comes off BEFORE the reverse geometry is
  // written so the carve-out's open-state duration override is already gone.
  const P = fx.pure;
  const open = P.durationMS('200ms', 0);
  const close = P.durationMS('160ms', 0);
  assert.ok(close < open, 'leaving must never feel slower than arriving');
  assert.equal(fx.editor.classList.contains('dcopen'), false,
    'the open-state duration override is still on the box, so the close would ' +
    'run at 200ms');

  // THE ORDER IS THE CLAIM, NOT THE END STATE. `.dcopen` must come off BEFORE
  // the reverse geometry is written: the carve-out's open-state rule is the
  // only thing declaring --disc-open, so writing the geometry first would run
  // the close at 200ms with every end-state assertion in this file still green.
  // A mutation proved exactly that hole, which is why the fixture records an
  // operation log at all.
  const ops = fx.editor._ops;
  const droppedOpen = ops.findIndex((o) => o.startsWith('class:') && !/\bdcopen\b/.test(o));
  const wroteReverse = ops.findIndex((o) => /^style:translate=-?\d+px/.test(o) && o !== 'style:translate=0px 0px');
  assert.ok(droppedOpen >= 0, 'the log never recorded `.dcopen` coming off');
  assert.ok(wroteReverse >= 0, 'the log never recorded the reverse geometry');
  assert.ok(droppedOpen < wroteReverse,
    'the reverse geometry was written while `.dcopen` was still on the box, so ' +
    'the close would run at --disc-open — leaving must never feel slower than ' +
    'arriving (register clause 2, which the carve-out did not name and therefore ' +
    'did not lift)');

  fx.flush();
  assert.equal(fx.editor.popoverOpen, false);
  assert.equal(fx.editor.classList.contains('edmorph'), false);
});

test('REDUCED MOTION: the morph is instant AND COMPLETE, asserted on the END state', () => {
  const fx = boot({ reduced: true });
  openEditorFromCard(fx, 3);
  const ed = fx.editor;

  // INSTANT: nothing was seeded, so there is no start geometry for a
  // transition that will never fire to leave behind.
  assert.equal(ed.classList.contains('edmorph'), false,
    'the carve-out class is on the box under reduced motion; the sheet declares ' +
    'no rule there, so the seeded start geometry would simply STICK');
  assert.equal(ed.style.getPropertyValue('translate'), '');
  assert.equal(ed.style.getPropertyValue('opacity'), '');
  assert.equal(ed.style.getPropertyValue('block-size'), '');

  // COMPLETE, and this is the half the signature's second word is about: the
  // editor lands at full size, full opacity, correctly placed and open — never
  // instantly at a MID-MORPH geometry.
  assert.equal(ed.popoverOpen, true);
  assert.equal(ed.hasAttribute('data-dc-shown'), true);
  assert.equal(ed.classList.contains('dcopen'), true);
  const r = ed.getBoundingClientRect();
  assert.equal(r.width, 760, 'the editor did not land at full size');
  assert.ok(r.height > 0, 'the editor landed with no height');
  assert.ok(parseFloat(ed.style.left) >= 0 && parseFloat(ed.style.top) >= 0,
    'the editor landed without a resolved placement');

  // …and it does not WAIT for an animation that will never fire.
  assert.equal(fx.timers.length, 0,
    'a timer is queued under reduced motion; the module is waiting on a ' +
    'transition the sheet does not declare');

  // THE COUNTERFACTUAL: the same fixture WITHOUT the reduced-motion branch
  // seeds a start geometry and queues the settle. The number comes back, which
  // is what proves the branch is doing the work rather than the fixture being
  // inert.
  const normal = boot();
  openEditorFromCard(normal, 3);
  assert.equal(normal.editor.classList.contains('edmorph'), true);
  assert.match(normal.editor.style.getPropertyValue('translate'), /px/);
  assert.ok(normal.timers.length > 0,
    'the no-preference branch queued nothing either, so the zero above proves ' +
    'the fixture is inert rather than proving the branch');
});

test('with no measurable card rect the editor opens exactly as stage 2 did', () => {
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  // A card with no rect is the degraded case — a browser that has not laid the
  // popover out yet, or a fixture that never gave it one. The morph DECLINES
  // rather than seeding a zero box, and the stage-2 open path is the fallback.
  Object.defineProperty(fx.card, 'rect', {
    configurable: true,
    get() { return { left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 }; },
  });
  fx.fire('click', fx.card.querySelector('[data-dc-new]'));

  assert.equal(fx.editor.classList.contains('edmorph'), false);
  assert.equal(fx.editor.classList.contains('dcopen'), true);
  assert.equal(fx.editor.popoverOpen, true);
  assert.equal(fx.editor.style.getPropertyValue('translate'), '');
});

// ── C-CALV4-EDITOR-R2b stage 6, FIX-FORWARD: THE STALE-GEOMETRY REOPEN ────
//
// THE DEFECT, AND IT WAS THE ROUND-3 BLOCKER ARRIVING THROUGH A STYLE
// ATTRIBUTE. edClose writes the REVERSE morph geometry as inline
// `inline-size` / `block-size` / `translate` / `opacity` — the CARD's measured
// rect, 420px wide — and edHide is the only thing that clears it, on the
// --disc-close timer that edShow cancels. So closing one day and opening
// another inside that 160ms window walked into edPosition with the card's width
// still pinned inline: `ed.root.offsetWidth` answered 420 for a box the sheet
// sizes at 760, placeCard found a clear position for a rectangle that does not
// exist, the 760px box drew there, and the module's own occlusion report said
// clear=true about it.
//
// MEASURED, NOT REASONED. daycard_geometry_probe_test.go (DAYCARD_GEOMETRY=1)
// caught it on 23 of the real-world calendar's day cells at viewport 900, up to
// 70,906 px² over the docked Ledger, at EVERY candidate width including the
// shipped one — because the stale rect is the CARD's and does not vary with
// --de-w. That probe is env-gated; this test is not, which is the point of
// writing it twice.
//
// `placeCard` IS NOT TOUCHED BY THE FIX ([ER-5]: a fourth geometry would be
// round 4's lesson unlearned). The law was always right; it was being handed a
// lie about the box.
test('reopening INSIDE the close window measures the box, not its last morph', () => {
  const fx = boot();
  openEditorFromCard(fx, 3);
  fx.flush(); // the morph settles; the editor is resting at its own size

  // CLOSE WITHOUT FLUSHING. The hide timer is now pending and the reverse
  // geometry — the card's rect — is sitting on the box as inline style.
  const cardW = Math.round(fx.card.getBoundingClientRect().width);
  fx.fire('keydown', fx.editor, { key: 'Escape' });
  assert.equal(fx.editor.style.getPropertyValue('inline-size'), cardW + 'px',
    'the reverse morph did not write the card geometry, so this test is not ' +
    'reproducing the condition it claims to');
  assert.notEqual(cardW, 760, 'the fixture cannot tell the two widths apart');
  assert.ok(fx.timers.length > 0, 'no hide timer is pending; the window under test is not open');

  // …and REOPEN on another day, inside that window, exactly as a user reading
  // two days in a row does.
  fx.editor._ops.length = 0;
  openEditorFromCard(fx, 5);

  // THE ORDERING IS THE CLAIM. The four morph properties must be CLEARED
  // before the placement writes `left`, because `left` is written by
  // applyPlacement from a size edPosition measured a moment earlier — if the
  // clear came after, the measurement already happened through the stale box.
  const ops = fx.editor._ops;
  const cleared = ops.indexOf('style:inline-size=');
  const placed = ops.findIndex((o) => o.startsWith('style:left='));
  assert.ok(cleared >= 0, 'the stale inline-size was never cleared on reopen');
  assert.ok(placed >= 0, 'the reopen never placed the box');
  assert.ok(cleared < placed,
    'the box was measured and placed BEFORE its stale geometry was cleared — ' +
    'placeCard is reasoning about a rectangle that does not render');

  // And the box really did land at its own width, not the card's.
  assert.equal(fx.editor.style.getPropertyValue('inline-size'), '760px');
  assert.equal(fx.editor.style.getPropertyValue('opacity'), '1');
});
