// theater_focus.test.mjs — C-CALV4-THEATER (R2-3), [TH-4] SIGNED: the full
// modal contract, and it is not negotiable down to a subset — a half-trapped
// overlay is worse than none.
//
// FOCUS CONTAINMENT ITSELF IS NATIVE. `<dialog>.showModal()` makes the rest of
// the document inert and traps the tab ring; that is exactly why [TH-1] chose it
// over `popover="manual"`, which traps nothing. What this suite pins is the part
// the UA does NOT give: where focus GOES on open, and that it comes BACK to the
// opener on EVERY close path. A modal that returns focus to <body> drops a
// keyboard user at the top of the page they were reading.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './theater_harness.mjs';

function settle(fx) {
  fx.fireOn('transitionend', fx.dialog, { target: fx.dialog.querySelector('[data-theater-box]') });
}

test('open moves focus INTO the theater, at the close control', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);

  assert.equal(fx.activeElement(), fx.closeBtn,
    'focus did not enter the theater at its close control — the first focusable descendant of the Block is a day cell deep in a grid, which is disorienting with a screen reader and offers no obvious way back out');
});

test('the close BUTTON returns focus to the opener', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);
  fx.fireOn('click', fx.closeBtn);
  settle(fx);

  assert.equal(fx.activeElement(), fx.opener);
  assert.equal(fx.opener.getAttribute('aria-expanded'), 'false');
});

test('ESCAPE returns focus to the opener', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);
  fx.fireOn('cancel', fx.dialog);
  settle(fx);

  assert.equal(fx.activeElement(), fx.opener);
  assert.equal(fx.opener.getAttribute('aria-expanded'), 'false');
});

test('a BACKDROP CLICK returns focus to the opener', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);
  fx.fireOn('click', fx.dialog, { target: fx.dialog });
  settle(fx);

  assert.equal(fx.activeElement(), fx.opener);
  assert.equal(fx.opener.getAttribute('aria-expanded'), 'false');
});

test('the WATCHDOG path returns focus to the opener', () => {
  // The path a browser that never fires transitionend takes. It is the one a
  // reader would meet on a broken engine, so it must land the same place.
  const fx = boot();
  fx.fireOn('click', fx.opener);
  fx.fireOn('click', fx.closeBtn);
  fx.flush();

  assert.equal(fx.activeElement(), fx.opener);
  assert.equal(fx.opener.getAttribute('aria-expanded'), 'false');
});

test('the REDUCED-MOTION path returns focus to the opener', () => {
  const fx = boot({ reduced: true });
  fx.fireOn('click', fx.opener);
  fx.fireOn('cancel', fx.dialog);

  assert.equal(fx.dialog.open, false);
  assert.equal(fx.activeElement(), fx.opener);
  assert.equal(fx.opener.getAttribute('aria-expanded'), 'false');
});

test('the opener is remembered on the dialog, not in a module variable', () => {
  // Two theaters on one page would share a module variable, and the second
  // close would return focus to the FIRST opener — a bug that is invisible on a
  // one-embed page, which is the same shape [TH-15] rules against for the mount.
  const fx = boot();
  fx.fireOn('click', fx.opener);
  assert.equal(fx.dialog._theaterOpener, fx.opener);

  fx.fireOn('click', fx.closeBtn);
  settle(fx);
  assert.equal(fx.dialog._theaterOpener, null, 'the reference outlived the close');
});

test('the ARIA shape is present and the accessible name resolves', () => {
  const fx = boot();
  assert.equal(fx.dialog.getAttribute('aria-modal'), 'true');
  const labelledBy = fx.dialog.getAttribute('aria-labelledby');
  assert.ok(labelledBy, 'no aria-labelledby');
  assert.ok(fx.dialog.querySelector('[id="' + labelledBy + '"]'), 'aria-labelledby names nothing');
  assert.equal(fx.opener.getAttribute('aria-haspopup'), 'dialog');
  assert.equal(fx.opener.getAttribute('aria-controls'), fx.scaffoldID);
});
